package global_least_request

import (
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-load-balancer/utils"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
	"github.com/tidwall/resp"
)

const (
	RedisKeyFormat          = "higress:global_least_request_table:%s:%s"
	RedisLastCleanKeyFormat = "higress:global_least_request_table:last_clean_time:%s:%s"
	RedisLua                = `local seed = tonumber(KEYS[1])
local hset_key = KEYS[2]
local last_clean_key = KEYS[3]
local clean_interval = tonumber(KEYS[4])
local current_target = KEYS[5]
local healthy_count = tonumber(KEYS[6])
local enable_detail_log = KEYS[7]

math.randomseed(seed)

-- 1. Selection
local current_count = 0
local same_count_hits = 0

for i = 8, 8 + healthy_count - 1 do
    local host = KEYS[i]
    local count = 0
    local val = redis.call('HGET', hset_key, host)
    if val then
        count = tonumber(val) or 0
    end
    
    if same_count_hits == 0 or count < current_count then
        current_target = host
        current_count = count
        same_count_hits = 1
    elseif count == current_count then
        same_count_hits = same_count_hits + 1
        if math.random(same_count_hits) == 1 then
            current_target = host
        end
    end
end

redis.call("HINCRBY", hset_key, current_target, 1)
local new_count = redis.call("HGET", hset_key, current_target)

-- Collect host counts for logging
local host_details = {}
if enable_detail_log == "1" then
    local fields = {}
    for i = 8, #KEYS do
        table.insert(fields, KEYS[i])
    end
    if #fields > 0 then
        local values = redis.call('HMGET', hset_key, (table.unpack or unpack)(fields))
        for i, val in ipairs(values) do
            table.insert(host_details, fields[i])
            table.insert(host_details, tostring(val or 0))
        end
    end
end

-- 2. Cleanup
local current_time = math.floor(seed / 1000000)
local last_clean_time = tonumber(redis.call('GET', last_clean_key) or 0)

if current_time - last_clean_time >= clean_interval then
    local all_keys = redis.call('HKEYS', hset_key)
    if #all_keys > 0 then
        -- Create a lookup table for current hosts (from index 8 onwards)
        local current_hosts = {}
        for i = 8, #KEYS do
            current_hosts[KEYS[i]] = true
        end
        -- Remove keys not in current hosts
        for _, host in ipairs(all_keys) do
            if not current_hosts[host] then
                redis.call('HDEL', hset_key, host)
            end
        end
    end
    redis.call('SET', last_clean_key, current_time)
end

return {current_target, new_count, host_details}`
)

type GlobalLeastRequestLoadBalancer struct {
	redisClient             wrapper.RedisClient
	maxRequestCount         int64
	cleanInterval           int64 // seconds
	enableDetailLog         bool
	getRouteName            func() (string, error)
	getClusterName          func() (string, error)
	getUpstreamHosts        func() ([][2]string, error)
	setUpstreamOverrideHost func([]byte) error
	resumeHttpRequest       func() error
}

func NewGlobalLeastRequestLoadBalancer(json gjson.Result) (GlobalLeastRequestLoadBalancer, error) {
	lb := GlobalLeastRequestLoadBalancer{
		getRouteName:            utils.GetRouteName,
		getClusterName:          utils.GetClusterName,
		getUpstreamHosts:        proxywasm.GetUpstreamHosts,
		setUpstreamOverrideHost: proxywasm.SetUpstreamOverrideHost,
		resumeHttpRequest:       proxywasm.ResumeHttpRequest,
	}
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
	timeout := json.Get("timeout").Int()
	if timeout == 0 {
		timeout = 3000
	}
	// database default is 0
	database := json.Get("database").Int()
	lb.maxRequestCount = json.Get("maxRequestCount").Int()
	lb.cleanInterval = json.Get("cleanInterval").Int()
	if lb.cleanInterval == 0 {
		lb.cleanInterval = 60 * 60 // default 60 minutes
	} else {
		lb.cleanInterval = lb.cleanInterval * 60 // convert minutes to seconds
	}
	lb.enableDetailLog = true
	if val := json.Get("enableDetailLog"); val.Exists() {
		lb.enableDetailLog = val.Bool()
	}
	log.Infof("redis client init, serviceFQDN: %s, servicePort: %d, timeout: %d, database: %d, maxRequestCount: %d, cleanInterval: %d minutes, enableDetailLog: %v", serviceFQDN, servicePort, timeout, database, lb.maxRequestCount, lb.cleanInterval/60, lb.enableDetailLog)
	return lb, lb.redisClient.Init(username, password, int64(timeout), wrapper.WithDataBase(int(database)))
}

func (lb GlobalLeastRequestLoadBalancer) HandleHttpRequestHeaders(ctx wrapper.HttpContext) types.Action {
	if !ctx.HasRequestBody() {
		return lb.HandleHttpRequestBody(ctx, nil)
	}
	// If return types.ActionContinue, SetUpstreamOverrideHost will not take effect
	return types.HeaderStopIteration
}

func (lb GlobalLeastRequestLoadBalancer) HandleHttpRequestBody(ctx wrapper.HttpContext, body []byte) types.Action {
	routeName, err := lb.getRouteName()
	if err != nil || routeName == "" {
		ctx.SetContext("error", true)
		return types.ActionContinue
	} else {
		ctx.SetContext("routeName", routeName)
	}
	clusterName, err := lb.getClusterName()
	if err != nil || clusterName == "" {
		ctx.SetContext("error", true)
		return types.ActionContinue
	} else {
		ctx.SetContext("clusterName", clusterName)
	}
	hostInfos, err := lb.getUpstreamHosts()
	if err != nil {
		ctx.SetContext("error", true)
		return types.ActionContinue
	}
	allHostMap := make(map[string]struct{})
	// Only healthy host can be selected
	healthyHostArray := []string{}
	for _, hostInfo := range hostInfos {
		allHostMap[hostInfo[0]] = struct{}{}
		if gjson.Get(hostInfo[1], "health_status").String() == "Healthy" {
			healthyHostArray = append(healthyHostArray, hostInfo[0])
		}
	}
	if len(healthyHostArray) == 0 {
		ctx.SetContext("error", true)
		return types.ActionContinue
	}
	randomIndex := rand.Intn(len(healthyHostArray))
	hostSelected := healthyHostArray[randomIndex]

	// KEYS structure: [seed, hset_key, last_clean_key, clean_interval, host_selected, healthy_count, ...healthy_hosts, enableDetailLog, ...unhealthy_hosts]
	keys := []interface{}{
		time.Now().UnixMicro(),
		fmt.Sprintf(RedisKeyFormat, routeName, clusterName),
		fmt.Sprintf(RedisLastCleanKeyFormat, routeName, clusterName),
		lb.cleanInterval,
		hostSelected,
		len(healthyHostArray),
		"0",
	}
	if lb.enableDetailLog {
		keys[6] = "1"
	}
	for _, v := range healthyHostArray {
		keys = append(keys, v)
	}
	// Append unhealthy hosts (those in allHostMap but not in healthyHostArray)
	for host := range allHostMap {
		isHealthy := false
		for _, hh := range healthyHostArray {
			if host == hh {
				isHealthy = true
				break
			}
		}
		if !isHealthy {
			keys = append(keys, host)
		}
	}

	err = lb.redisClient.Eval(RedisLua, len(keys), keys, []interface{}{}, func(response resp.Value) {
		if err := response.Error(); err != nil {
			log.Errorf("HGetAll failed: %+v", err)
			ctx.SetContext("error", true)
			lb.resumeHttpRequest()
			return
		}
		valArray := response.Array()
		if len(valArray) < 2 {
			log.Errorf("redis eval lua result format error, expect at least [host, count], got: %+v", valArray)
			ctx.SetContext("error", true)
			lb.resumeHttpRequest()
			return
		}
		hostSelected = valArray[0].String()
		currentCount := valArray[1].Integer()

		// detail log
		if lb.enableDetailLog && len(valArray) >= 3 {
			detailLogStr := "host and count: "
			details := valArray[2].Array()
			for i := 0; i+1 < len(details); i += 2 {
				h := details[i].String()
				c := details[i+1].String()
				detailLogStr += fmt.Sprintf("{%s: %s}, ", h, c)
			}
			log.Debugf("host_selected: %s + 1, %s", hostSelected, detailLogStr)
		}

		// check rate limit
		if !lb.checkRateLimit(hostSelected, int64(currentCount), ctx, routeName, clusterName) {
			ctx.SetContext("error", true)
			log.Warnf("host_selected: %s, current_count: %d, exceed max request limit %d", hostSelected, currentCount, lb.maxRequestCount)
			// return 429
			proxywasm.SendHttpResponse(429, [][2]string{}, []byte("Exceeded maximum request limit from ai-load-balancer."), -1)
			ctx.DontReadResponseBody()
			return
		}

		if err := lb.setUpstreamOverrideHost([]byte(hostSelected)); err != nil {
			ctx.SetContext("error", true)
			log.Errorf("override upstream host failed, fallback to default lb policy, error informations: %+v", err)
			lb.resumeHttpRequest()
			return
		}

		log.Debugf("host_selected: %s", hostSelected)

		// finally resume the request
		ctx.SetContext("host_selected", hostSelected)
		lb.resumeHttpRequest()
	})
	if err != nil {
		ctx.SetContext("error", true)
		log.Errorf("redis eval failed, fallback to default lb policy, error informations: %+v", err)
		return types.ActionContinue
	}
	return types.ActionPause
}

func (lb GlobalLeastRequestLoadBalancer) HandleHttpResponseHeaders(ctx wrapper.HttpContext) types.Action {
	return types.ActionContinue
}

func (lb GlobalLeastRequestLoadBalancer) HandleHttpStreamingResponseBody(ctx wrapper.HttpContext, data []byte, endOfStream bool) []byte {
	return data
}

func (lb GlobalLeastRequestLoadBalancer) HandleHttpResponseBody(ctx wrapper.HttpContext, body []byte) types.Action {
	return types.ActionContinue
}

func (lb GlobalLeastRequestLoadBalancer) HandleHttpStreamDone(ctx wrapper.HttpContext) {
	isErr, _ := ctx.GetContext("error").(bool)
	if !isErr {
		routeName, _ := ctx.GetContext("routeName").(string)
		clusterName, _ := ctx.GetContext("clusterName").(string)
		host_selected, _ := ctx.GetContext("host_selected").(string)
		if host_selected == "" {
			log.Errorf("get host_selected failed")
		} else {
			err := lb.redisClient.HIncrBy(fmt.Sprintf(RedisKeyFormat, routeName, clusterName), host_selected, -1, nil)
			if err != nil {
				log.Errorf("host_selected: %s - 1, failed to update count from redis: %v", host_selected, err)
			}
		}
	}
}
