package endpoint_hash

import (
	"errors"
	"fmt"
	"hash/fnv"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-load-balancer/utils"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
	"github.com/tidwall/resp"
)

const (
	DefaultHashHeader = "x-mse-consumer"
	// RedisKeyFormat: higress:endpoint_hash_table:{routeName}:{clusterName}:{stateKey}
	// value = the bound upstream host address ("ip:port").
	RedisKeyFormat = "higress:endpoint_hash_table:%s:%s:%s"

	ctxStateKey = "endpoint_hash_state_key"
	ctxNeedSave = "endpoint_hash_need_save"
	ctxHashKey  = "endpoint_hash_hash_key"
	ctxRecorded = "endpoint_hash_recorded"

	logTag = "[ai-load-balancer/endpoint_hash]"
)

// EndpointHashLoadBalancer implements stateful sticky routing:
//   - With a hash key, it looks up the key's bound host in Redis. On a hit
//     (and the host still healthy) the request is pinned to that host.
//   - On a miss (first time the key is seen, or the bound host is gone), it
//     does NOT override and lets the service's own default LB pick a host,
//     then persists key -> chosen host to Redis for subsequent requests.
//   - Without a hash key, it never overrides — the request just uses the
//     default LB and no state is recorded.
type EndpointHashLoadBalancer struct {
	redisClient   wrapper.RedisClient
	HashHeader    string
	stickyTimeout int // TTL of a sticky entry, in seconds
}

func NewEndpointHashLoadBalancer(json gjson.Result) (EndpointHashLoadBalancer, error) {
	lb := EndpointHashLoadBalancer{}

	lb.HashHeader = json.Get("hash_header").String()
	if lb.HashHeader == "" {
		lb.HashHeader = DefaultHashHeader
	}

	// stickyTimeout is configured in minutes; default 60 minutes.
	stickyTimeout := json.Get("stickyTimeout").Int()
	if stickyTimeout == 0 {
		stickyTimeout = 60
	}
	lb.stickyTimeout = int(stickyTimeout) * 60

	serviceFQDN := json.Get("serviceFQDN").String()
	servicePort := json.Get("servicePort").Int()
	if serviceFQDN == "" || servicePort == 0 {
		log.Errorf("invalid redis service, serviceFQDN: %s, servicePort: %d", serviceFQDN, servicePort)
		return lb, errors.New("invalid redis service config")
	}
	lb.redisClient = wrapper.NewRedisClusterClient(wrapper.FQDNCluster{
		FQDN: serviceFQDN,
		Port: servicePort,
	})
	username := json.Get("username").String()
	password := json.Get("password").String()
	if password == "" {
		log.Warnf("%s redis password is empty; if your Redis requires authentication, every read/write will fail with NOAUTH and stickiness will not work", logTag)
	}
	timeout := json.Get("timeout").Int()
	if timeout == 0 {
		timeout = 3000
	}
	database := json.Get("database").Int()
	log.Infof("endpoint_hash redis init, serviceFQDN: %s, servicePort: %d, timeout: %d, database: %d, hashHeader: %s, stickyTimeout: %d minutes",
		serviceFQDN, servicePort, timeout, database, lb.HashHeader, lb.stickyTimeout/60)
	return lb, lb.redisClient.Init(username, password, int64(timeout), wrapper.WithDataBase(int(database)))
}

// stateKey derives the Redis key for a hash header value using FNV-1a, scoped
// by route + cluster so different routes/clusters keep independent mappings.
func stateKey(routeName, clusterName, hashKey string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(hashKey))
	return fmt.Sprintf(RedisKeyFormat, routeName, clusterName, fmt.Sprintf("%08x", h.Sum32()))
}

// readUpstreamAddress returns the address ("ip:port") of the host actually
// chosen by the upstream (default LB), or "" if it is not available yet.
func readUpstreamAddress() string {
	if addr, err := proxywasm.GetProperty([]string{"upstream", "address"}); err == nil && len(addr) > 0 {
		return string(addr)
	}
	return ""
}

// record persists key -> chosen host with TTL. It MUST be called from a live
// request/response phase (e.g. response headers), NOT from HandleHttpStreamDone:
// in the stream-done phase the filter context is being torn down and the async
// Redis dispatch never actually reaches Redis (the local dispatch still returns
// OK, which silently loses the write). The callback logs the real Redis reply
// so a lost/failed write is traceable.
func (lb EndpointHashLoadBalancer) record(ctx wrapper.HttpContext, hashKey, sKey, host string) {
	err := lb.redisClient.SetEx(sKey, host, lb.stickyTimeout, func(response resp.Value) {
		if response.Error() != nil {
			log.Errorf("%s record_reply_err key=%q skey=%s host=%s err=%v", logTag, hashKey, sKey, host, response.Error())
			return
		}
		ctx.SetContext(ctxRecorded, true)
		log.Infof("%s record_ok key=%q skey=%s host=%s ttl=%ds reply=%s", logTag, hashKey, sKey, host, lb.stickyTimeout, response.String())
	})
	if err != nil {
		log.Errorf("%s record_dispatch_failed key=%q skey=%s host=%s err=%v", logTag, hashKey, sKey, host, err)
	}
}

func (lb EndpointHashLoadBalancer) HandleHttpRequestHeaders(ctx wrapper.HttpContext) types.Action {
	// Defer to the body phase: SetUpstreamOverrideHost only takes effect after
	// the route/cluster is resolved (same constraint as global_least_request).
	return types.HeaderStopIteration
}

func (lb EndpointHashLoadBalancer) HandleHttpRequestBody(ctx wrapper.HttpContext, body []byte) types.Action {
	hashKey, err := proxywasm.GetHttpRequestHeader(lb.HashHeader)
	if err != nil || hashKey == "" {
		// No hash key -> never override, no state recorded, use default LB.
		log.Debugf("%s decision=NO_KEY header=%q -> default lb (no stickiness)", logTag, lb.HashHeader)
		return types.ActionContinue
	}

	routeName, err := utils.GetRouteName()
	if err != nil || routeName == "" {
		log.Warnf("%s decision=DEGRADE key=%q reason=get_route_name_failed err=%v -> default lb", logTag, hashKey, err)
		return types.ActionContinue
	}
	clusterName, err := utils.GetClusterName()
	if err != nil || clusterName == "" {
		log.Warnf("%s decision=DEGRADE key=%q reason=get_cluster_name_failed err=%v -> default lb", logTag, hashKey, err)
		return types.ActionContinue
	}

	// Snapshot the current healthy host set; used to validate a stored host.
	hostInfos, err := proxywasm.GetUpstreamHosts()
	if err != nil {
		log.Warnf("%s decision=DEGRADE key=%q route=%s cluster=%s reason=get_upstream_hosts_failed err=%v -> default lb", logTag, hashKey, routeName, clusterName, err)
		return types.ActionContinue
	}
	healthySet := make(map[string]struct{}, len(hostInfos))
	for _, hostInfo := range hostInfos {
		if gjson.Get(hostInfo[1], "health_status").String() == "Healthy" {
			healthySet[hostInfo[0]] = struct{}{}
		}
	}
	if len(healthySet) == 0 {
		log.Warnf("%s decision=DEGRADE key=%q route=%s cluster=%s reason=no_healthy_host -> default lb", logTag, hashKey, routeName, clusterName)
		return types.ActionContinue
	}

	sKey := stateKey(routeName, clusterName, hashKey)
	ctx.SetContext(ctxStateKey, sKey)
	ctx.SetContext(ctxHashKey, hashKey)

	err = lb.redisClient.Get(sKey, func(response resp.Value) {
		storedHost := ""
		if response.Error() != nil {
			log.Errorf("%s redis_get_err key=%q skey=%s err=%v -> treat as miss", logTag, hashKey, sKey, response.Error())
		} else {
			storedHost = response.String()
		}

		if storedHost != "" {
			if _, ok := healthySet[storedHost]; ok {
				// HIT: pin to the stored host.
				if err := proxywasm.SetUpstreamOverrideHost([]byte(storedHost)); err != nil {
					log.Errorf("%s decision=HIT_FALLBACK key=%q skey=%s host=%s reason=override_failed err=%v -> default lb, will record", logTag, hashKey, sKey, storedHost, err)
				} else {
					// Refresh TTL so active keys don't expire (best-effort).
					_ = lb.redisClient.Expire(sKey, lb.stickyTimeout, nil)
					log.Infof("%s decision=HIT key=%q skey=%s host=%s ttl_refreshed=%ds (override)", logTag, hashKey, sKey, storedHost, lb.stickyTimeout)
					proxywasm.ResumeHttpRequest()
					return
				}
			} else {
				log.Infof("%s decision=STALE key=%q skey=%s stored_host=%s reason=unhealthy -> default lb, will record", logTag, hashKey, sKey, storedHost)
			}
		} else {
			log.Infof("%s decision=MISS key=%q skey=%s -> default lb, will record", logTag, hashKey, sKey)
		}

		// MISS / STALE / override-failed: let the default LB pick, then persist
		// the chosen host in the response/done phase.
		ctx.SetContext(ctxNeedSave, true)
		proxywasm.ResumeHttpRequest()
	})
	if err != nil {
		// Redis unavailable -> degrade to default LB without stickiness.
		log.Warnf("%s decision=DEGRADE key=%q skey=%s reason=redis_get_dispatch_failed err=%v -> default lb", logTag, hashKey, sKey, err)
		return types.ActionContinue
	}
	return types.ActionPause
}

func (lb EndpointHashLoadBalancer) HandleHttpResponseHeaders(ctx wrapper.HttpContext) types.Action {
	needSave, _ := ctx.GetContext(ctxNeedSave).(bool)
	if !needSave {
		return types.ActionContinue
	}
	// Persist the default-LB choice HERE (a live phase), not in StreamDone — the
	// upstream host is already selected, the context is still alive, so the async
	// SETEX actually reaches Redis.
	sKey, _ := ctx.GetContext(ctxStateKey).(string)
	hashKey, _ := ctx.GetContext(ctxHashKey).(string)
	host := readUpstreamAddress()
	if sKey == "" || host == "" {
		// upstream.address not ready yet -> StreamDone will retry as a fallback.
		log.Warnf("%s record_deferred key=%q skey=%s reason=upstream_address_not_ready_at_response_headers", logTag, hashKey, sKey)
		return types.ActionContinue
	}
	lb.record(ctx, hashKey, sKey, host)
	return types.ActionContinue
}

func (lb EndpointHashLoadBalancer) HandleHttpStreamingResponseBody(ctx wrapper.HttpContext, data []byte, endOfStream bool) []byte {
	return data
}

func (lb EndpointHashLoadBalancer) HandleHttpResponseBody(ctx wrapper.HttpContext, body []byte) types.Action {
	return types.ActionContinue
}

func (lb EndpointHashLoadBalancer) HandleHttpStreamDone(ctx wrapper.HttpContext) {
	needSave, _ := ctx.GetContext(ctxNeedSave).(bool)
	if !needSave {
		return
	}
	// If the write already happened in the response-header phase, nothing to do.
	if recorded, _ := ctx.GetContext(ctxRecorded).(bool); recorded {
		return
	}
	// Fallback path: the write was deferred because upstream.address was not
	// ready at response headers. This is best-effort only — async Redis calls in
	// the stream-done phase may not reach Redis — so we log it as a fallback.
	sKey, _ := ctx.GetContext(ctxStateKey).(string)
	hashKey, _ := ctx.GetContext(ctxHashKey).(string)
	host := readUpstreamAddress()
	if sKey == "" || host == "" {
		log.Warnf("%s record_skipped key=%q skey=%s reason=upstream_address_unavailable", logTag, hashKey, sKey)
		return
	}
	log.Warnf("%s record_fallback_in_stream_done key=%q skey=%s host=%s (may not persist)", logTag, hashKey, sKey, host)
	lb.record(ctx, hashKey, sKey, host)
}
