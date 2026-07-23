package cluster_metrics

import (
	"fmt"
	"math"
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
	DefaultQueueSize               = 100
	DefaultClusterHeader           = "x-higress-target-cluster"
	ModeAdaptiveScore              = "AdaptiveScore"
	DefaultEWMABeta                = 0.5
	DefaultP2CChoices              = 2
	DefaultTTFTWeight              = 0.7
	DefaultTotalLatencyWeight      = 0.3
	DefaultErrorPenalty            = 3.0
	DefaultFailureCooldownMs       = 30000
	MetricsMissingPolicyLeast      = "least_busy"
	DefaultGlobalInflightKeyPrefix = "higress:adaptive_score_inflight"
	DefaultGlobalInflightTimeoutMs = 3000
	DefaultGlobalInflightKeyTTL    = 1800
	GlobalInflightKeyFormat        = "%s:%s:%s"
	GlobalInflightLua              = `local seed = tonumber(KEYS[1])
local hset_key = KEYS[2]
local service_count = tonumber(KEYS[3])
local ttl = tonumber(KEYS[4])

math.randomseed(seed)

local selected = ""
local selected_score = 0
local same_score_hits = 0

for i = 1, service_count do
    local offset = 4 + (i - 1) * 2
    local service = KEYS[offset + 1]
    local local_score = tonumber(KEYS[offset + 2])
    local inflight = 0
    local val = redis.call("HGET", hset_key, service)
    if val then
        inflight = tonumber(val) or 0
    end
    local score = local_score * (inflight + 1)

    if same_score_hits == 0 or score < selected_score then
        selected = service
        selected_score = score
        same_score_hits = 1
    elseif score == selected_score then
        same_score_hits = same_score_hits + 1
        if math.random(same_score_hits) == 1 then
            selected = service
            selected_score = score
        end
    end
end

redis.call("HINCRBY", hset_key, selected, 1)
if ttl > 0 then
    redis.call("EXPIRE", hset_key, ttl)
end
local new_count = redis.call("HGET", hset_key, selected)
return {selected, new_count, selected_score}`
)

type ClusterEndpointLoadBalancer struct {
	// Configurations
	Mode                    string
	ClusterHeader           string
	ServiceList             []string
	RateLimit               float64
	EWMABeta                float64
	P2CChoices              int
	TTFTWeight              float64
	TotalRTWeight           float64
	ErrorPenalty            float64
	FailureCooldownMs       int64
	MetricsMissingPolicy    string
	GlobalInflightEnabled   bool
	GlobalInflightKeyPrefix string
	GlobalInflightTimeoutMs int64
	GlobalInflightKeyTTL    int
	redisClient             globalInflightRedisClient
	// Statistic
	ServiceRequestOngoing     map[string]int
	ServiceRequestCount       map[string]int
	FirstTokenLatencyRequests map[string]*utils.FixedQueue[float64]
	TotalLatencyRequests      map[string]*utils.FixedQueue[float64]
	ServiceAdaptiveMetrics    map[string]*AdaptiveMetrics
}

type globalInflightRedisClient interface {
	Ready() bool
	Eval(script string, numkeys int, keys, args []interface{}, callback wrapper.RedisResponseCallback) error
	HIncrBy(key, field string, delta int, callback wrapper.RedisResponseCallback) error
}

type AdaptiveMetrics struct {
	EWMATTFT            float64
	EWMATotalRT         float64
	HasTTFT             bool
	HasTotalRT          bool
	SuccessCount        int
	ErrorCount          int
	ConsecutiveErrors   int
	CooldownUntilUnixMs int64
}

func NewClusterEndpointLoadBalancer(json gjson.Result) (ClusterEndpointLoadBalancer, error) {
	lb := ClusterEndpointLoadBalancer{}
	lb.ServiceRequestOngoing = make(map[string]int)
	lb.ServiceRequestCount = make(map[string]int)
	lb.FirstTokenLatencyRequests = make(map[string]*utils.FixedQueue[float64])
	lb.TotalLatencyRequests = make(map[string]*utils.FixedQueue[float64])
	lb.ServiceAdaptiveMetrics = make(map[string]*AdaptiveMetrics)

	lb.Mode = json.Get("mode").String()
	lb.ClusterHeader = json.Get("cluster_header").String()
	if lb.ClusterHeader == "" {
		lb.ClusterHeader = DefaultClusterHeader
	}
	if json.Get("rate_limit").Exists() {
		lb.RateLimit = json.Get("rate_limit").Float()
	} else {
		lb.RateLimit = 1.0
	}
	lb.EWMABeta = getFloatConfig(json, "ewma_beta", DefaultEWMABeta)
	if lb.EWMABeta < 0 || lb.EWMABeta > 1 {
		return lb, fmt.Errorf("ewma_beta must be in range [0, 1]")
	}
	lb.P2CChoices = int(json.Get("p2c_choices").Int())
	if lb.P2CChoices <= 0 {
		lb.P2CChoices = DefaultP2CChoices
	}
	lb.TTFTWeight = getFloatConfig(json, "ttft_weight", DefaultTTFTWeight)
	lb.TotalRTWeight = getFloatConfig(json, "total_latency_weight", DefaultTotalLatencyWeight)
	if lb.TTFTWeight < 0 || lb.TotalRTWeight < 0 {
		return lb, fmt.Errorf("ttft_weight and total_latency_weight must be greater than or equal to 0")
	}
	if lb.TTFTWeight == 0 && lb.TotalRTWeight == 0 {
		return lb, fmt.Errorf("ttft_weight and total_latency_weight cannot both be 0")
	}
	lb.ErrorPenalty = getFloatConfig(json, "error_penalty", DefaultErrorPenalty)
	if lb.ErrorPenalty < 0 {
		return lb, fmt.Errorf("error_penalty must be greater than or equal to 0")
	}
	lb.FailureCooldownMs = json.Get("failure_cooldown_ms").Int()
	if lb.FailureCooldownMs == 0 {
		lb.FailureCooldownMs = DefaultFailureCooldownMs
	}
	if lb.FailureCooldownMs < 0 {
		return lb, fmt.Errorf("failure_cooldown_ms must be greater than or equal to 0")
	}
	lb.MetricsMissingPolicy = json.Get("metrics_missing_policy").String()
	if !isValidMetricsMissingPolicy(lb.MetricsMissingPolicy) {
		lb.MetricsMissingPolicy = MetricsMissingPolicyLeast
	}
	lb.GlobalInflightEnabled = json.Get("global_inflight_enabled").Bool()
	lb.GlobalInflightKeyPrefix = json.Get("global_inflight_key_prefix").String()
	if lb.GlobalInflightKeyPrefix == "" {
		lb.GlobalInflightKeyPrefix = DefaultGlobalInflightKeyPrefix
	}
	lb.GlobalInflightTimeoutMs = json.Get("global_inflight_timeout").Int()
	if lb.GlobalInflightTimeoutMs == 0 {
		lb.GlobalInflightTimeoutMs = DefaultGlobalInflightTimeoutMs
	}
	if lb.GlobalInflightTimeoutMs < 0 {
		return lb, fmt.Errorf("global_inflight_timeout must be greater than or equal to 0")
	}
	lb.GlobalInflightKeyTTL = int(json.Get("global_inflight_key_ttl").Int())
	if lb.GlobalInflightKeyTTL == 0 {
		lb.GlobalInflightKeyTTL = DefaultGlobalInflightKeyTTL
	}
	if lb.GlobalInflightKeyTTL < 0 {
		return lb, fmt.Errorf("global_inflight_key_ttl must be greater than or equal to 0")
	}
	if lb.GlobalInflightEnabled {
		if err := lb.initGlobalInflightRedis(json); err != nil {
			return lb, err
		}
	}
	queueSize := int(json.Get("queue_size").Int())
	if queueSize == 0 {
		queueSize = DefaultQueueSize
	}

	for _, svc := range json.Get("service_list").Array() {
		serviceName := svc.String()
		lb.ServiceList = append(lb.ServiceList, serviceName)
		lb.ServiceRequestOngoing[serviceName] = 0
		lb.ServiceRequestCount[serviceName] = 0
		lb.FirstTokenLatencyRequests[serviceName] = utils.NewFixedQueue[float64](queueSize)
		lb.TotalLatencyRequests[serviceName] = utils.NewFixedQueue[float64](queueSize)
		lb.ServiceAdaptiveMetrics[serviceName] = &AdaptiveMetrics{}
	}
	if len(lb.ServiceList) == 0 {
		return lb, fmt.Errorf("service_list cannot be empty")
	}
	return lb, nil
}

func (lb *ClusterEndpointLoadBalancer) initGlobalInflightRedis(json gjson.Result) error {
	serviceFQDN := json.Get("serviceFQDN").String()
	servicePort := json.Get("servicePort").Int()
	if serviceFQDN == "" || servicePort == 0 {
		return fmt.Errorf("invalid redis service config")
	}
	redisClient := wrapper.NewRedisClusterClient(wrapper.FQDNCluster{
		FQDN: serviceFQDN,
		Port: servicePort,
	})
	username := json.Get("username").String()
	password := json.Get("password").String()
	database := json.Get("database").Int()
	if err := redisClient.Init(username, password, lb.GlobalInflightTimeoutMs, wrapper.WithDataBase(int(database))); err != nil {
		log.Errorf("adaptive score global inflight redis init failed, fallback to local adaptive score: %v", err)
		lb.redisClient = nil
		return nil
	}
	lb.redisClient = redisClient
	return nil
}

func getFloatConfig(json gjson.Result, path string, defaultValue float64) float64 {
	if json.Get(path).Exists() {
		return json.Get(path).Float()
	}
	return defaultValue
}

func isValidMetricsMissingPolicy(policy string) bool {
	return policy == MetricsMissingPolicyLeast
}

func (lb ClusterEndpointLoadBalancer) getRequestRate(serviceName string) float64 {
	totalRequestCount := 0
	for _, v := range lb.ServiceRequestCount {
		totalRequestCount += v
	}
	if totalRequestCount != 0 {
		return float64(lb.ServiceRequestCount[serviceName]) / float64(totalRequestCount)
	}
	return 0
}

func (lb ClusterEndpointLoadBalancer) getServiceTTFT(serviceName string) float64 {
	queue, ok := lb.FirstTokenLatencyRequests[serviceName]
	if !ok || queue.Size() == 0 {
		return 0
	}
	value := 0.0
	queue.ForEach(func(i int, item float64) {
		value += float64(item)
	})
	return value / float64(queue.Size())
}

func (lb ClusterEndpointLoadBalancer) getServiceTotalRT(serviceName string) float64 {
	queue, ok := lb.TotalLatencyRequests[serviceName]
	if !ok || queue.Size() == 0 {
		return 0
	}
	value := 0.0
	queue.ForEach(func(i int, item float64) {
		value += float64(item)
	})
	return value / float64(queue.Size())
}

func (lb ClusterEndpointLoadBalancer) updateEWMA(oldValue, sample float64, initialized bool) float64 {
	if !initialized {
		return sample
	}
	return lb.EWMABeta*oldValue + (1-lb.EWMABeta)*sample
}

func (lb ClusterEndpointLoadBalancer) recordAdaptiveTTFT(serviceName string, durationMs float64) {
	metrics, ok := lb.ServiceAdaptiveMetrics[serviceName]
	if !ok {
		return
	}
	metrics.EWMATTFT = lb.updateEWMA(metrics.EWMATTFT, durationMs, metrics.HasTTFT)
	metrics.HasTTFT = true
}

func (lb ClusterEndpointLoadBalancer) recordAdaptiveTotalRT(serviceName string, durationMs float64, failed bool, nowUnixMs int64) {
	metrics, ok := lb.ServiceAdaptiveMetrics[serviceName]
	if !ok {
		return
	}
	metrics.EWMATotalRT = lb.updateEWMA(metrics.EWMATotalRT, durationMs, metrics.HasTotalRT)
	metrics.HasTotalRT = true
	if failed {
		metrics.ErrorCount += 1
		metrics.ConsecutiveErrors += 1
		metrics.CooldownUntilUnixMs = nowUnixMs + lb.FailureCooldownMs
		return
	}
	metrics.SuccessCount += 1
	metrics.ConsecutiveErrors = 0
}

func (lb ClusterEndpointLoadBalancer) adaptiveScore(serviceName string, nowUnixMs int64) float64 {
	metrics, ok := lb.ServiceAdaptiveMetrics[serviceName]
	if !ok {
		return math.MaxFloat64
	}
	inflight := float64(lb.ServiceRequestOngoing[serviceName] + 1)
	if metrics.CooldownUntilUnixMs > nowUnixMs {
		inflight += float64(metrics.ConsecutiveErrors + 1)
	}
	if !metrics.HasTTFT && !metrics.HasTotalRT && lb.MetricsMissingPolicy == MetricsMissingPolicyLeast {
		return inflight
	}
	ttft := metrics.EWMATTFT
	if !metrics.HasTTFT {
		ttft = metrics.EWMATotalRT
	}
	totalRT := metrics.EWMATotalRT
	if !metrics.HasTotalRT {
		totalRT = metrics.EWMATTFT
	}
	latencyScore := lb.TTFTWeight*math.Sqrt(ttft+1) + lb.TotalRTWeight*math.Sqrt(totalRT+1)
	total := metrics.SuccessCount + metrics.ErrorCount
	errorRate := 0.0
	if total > 0 {
		errorRate = float64(metrics.ErrorCount) / float64(total)
	}
	reliabilityPenalty := 1 + lb.ErrorPenalty*errorRate
	return latencyScore * inflight * reliabilityPenalty
}

func (lb ClusterEndpointLoadBalancer) selectAdaptiveScore(nowUnixMs int64) string {
	if len(lb.ServiceList) == 1 {
		return lb.ServiceList[0]
	}
	return lb.selectLowestScore(lb.sampleAdaptiveCandidates(nowUnixMs), nowUnixMs)
}

func (lb ClusterEndpointLoadBalancer) adaptiveCandidateServices(nowUnixMs int64) []string {
	candidateServices := lb.ServiceList
	rateLimitedServices := make([]string, 0, len(lb.ServiceList))
	for _, svc := range lb.ServiceList {
		if lb.getRequestRate(svc) < lb.RateLimit {
			rateLimitedServices = append(rateLimitedServices, svc)
		}
	}
	if len(rateLimitedServices) > 0 {
		candidateServices = rateLimitedServices
	}
	availableServices := make([]string, 0, len(lb.ServiceList))
	for _, svc := range candidateServices {
		if metrics, ok := lb.ServiceAdaptiveMetrics[svc]; ok && metrics.CooldownUntilUnixMs <= nowUnixMs {
			availableServices = append(availableServices, svc)
		}
	}
	if len(availableServices) > 0 {
		candidateServices = availableServices
	}
	return candidateServices
}

func (lb ClusterEndpointLoadBalancer) sampleAdaptiveCandidates(nowUnixMs int64) []string {
	candidateServices := lb.adaptiveCandidateServices(nowUnixMs)
	if len(candidateServices) <= 1 || lb.P2CChoices >= len(candidateServices) {
		return candidateServices
	}
	choices := make([]string, 0, lb.P2CChoices)
	seen := make(map[string]struct{}, lb.P2CChoices)
	for len(choices) < lb.P2CChoices {
		svc := candidateServices[rand.Int()%len(candidateServices)]
		if _, ok := seen[svc]; ok {
			continue
		}
		seen[svc] = struct{}{}
		choices = append(choices, svc)
	}
	return choices
}

func (lb ClusterEndpointLoadBalancer) selectLowestScore(candidates []string, nowUnixMs int64) string {
	candidate := candidates[0]
	candidateScore := lb.adaptiveScore(candidate, nowUnixMs)
	for _, svc := range candidates[1:] {
		score := lb.adaptiveScore(svc, nowUnixMs)
		if score < candidateScore {
			candidate = svc
			candidateScore = score
		}
	}
	return candidate
}

func (lb ClusterEndpointLoadBalancer) hasService(serviceName string) bool {
	_, ok := lb.ServiceRequestOngoing[serviceName]
	return ok
}

func (lb ClusterEndpointLoadBalancer) getSelectedService(ctx wrapper.HttpContext) (string, bool) {
	candidate, ok := ctx.GetContext(lb.ClusterHeader).(string)
	if !ok || candidate == "" {
		log.Errorf("selected service is missing from context")
		return "", false
	}
	if !lb.hasService(candidate) {
		log.Errorf("selected service is not in service_list: %s", candidate)
		return "", false
	}
	return candidate, true
}

func (lb ClusterEndpointLoadBalancer) globalInflightKey(routeName string) string {
	return fmt.Sprintf(GlobalInflightKeyFormat, lb.GlobalInflightKeyPrefix, routeName, lb.Mode)
}

func (lb ClusterEndpointLoadBalancer) buildGlobalInflightLuaKeys(nowUnixMs int64, routeName string) []interface{} {
	candidates := lb.sampleAdaptiveCandidates(nowUnixMs)
	keys := []interface{}{
		nowUnixMs * 1000,
		lb.globalInflightKey(routeName),
		len(candidates),
		lb.GlobalInflightKeyTTL,
	}
	for _, svc := range candidates {
		keys = append(keys, svc, lb.adaptiveScore(svc, nowUnixMs))
	}
	return keys
}

func (lb ClusterEndpointLoadBalancer) applySelectedService(ctx wrapper.HttpContext, candidate string) {
	proxywasm.ReplaceHttpRequestHeader(lb.ClusterHeader, candidate)
	ctx.SetContext(lb.ClusterHeader, candidate)
	lb.ServiceRequestOngoing[candidate] += 1
	lb.ServiceRequestCount[candidate] += 1
}

func (lb ClusterEndpointLoadBalancer) selectLocalAdaptiveScore(ctx wrapper.HttpContext) string {
	nowUnixMs := time.Now().UnixMilli()
	candidate := lb.selectAdaptiveScore(nowUnixMs)
	lb.applySelectedService(ctx, candidate)
	return candidate
}

func (lb ClusterEndpointLoadBalancer) applyGlobalInflightCandidate(ctx wrapper.HttpContext, globalInflightKey, candidate string) bool {
	if !lb.hasService(candidate) {
		log.Errorf("adaptive score global inflight returned unknown service, fallback to local adaptive score: %s", candidate)
		if lb.redisClient != nil && globalInflightKey != "" && candidate != "" {
			if err := lb.redisClient.HIncrBy(globalInflightKey, candidate, -1, nil); err != nil {
				log.Errorf("adaptive score global inflight rollback failed, service: %s, error: %v", candidate, err)
			}
		}
		return false
	}
	lb.applySelectedService(ctx, candidate)
	ctx.SetContext("global_inflight_selected", true)
	return true
}

func (lb ClusterEndpointLoadBalancer) selectGlobalAdaptiveScore(ctx wrapper.HttpContext) types.Action {
	nowUnixMs := time.Now().UnixMilli()
	routeName, err := utils.GetRouteName()
	if err != nil || routeName == "" {
		log.Errorf("adaptive score global inflight route name unavailable, fallback to local adaptive score: %v", err)
		candidate := lb.selectLocalAdaptiveScore(ctx)
		log.Debugf("adaptive score fallback selected service: %s", candidate)
		return types.ActionContinue
	}
	globalInflightKey := lb.globalInflightKey(routeName)
	ctx.SetContext("global_inflight_key", globalInflightKey)
	keys := lb.buildGlobalInflightLuaKeys(nowUnixMs, routeName)
	err = lb.redisClient.Eval(GlobalInflightLua, len(keys), keys, []interface{}{}, func(response resp.Value) {
		if err := response.Error(); err != nil {
			log.Errorf("adaptive score global inflight eval failed, fallback to local adaptive score: %+v", err)
			candidate := lb.selectLocalAdaptiveScore(ctx)
			log.Debugf("adaptive score fallback selected service: %s", candidate)
			proxywasm.ResumeHttpRequest()
			return
		}
		valArray := response.Array()
		if len(valArray) < 1 || valArray[0].String() == "" {
			log.Errorf("adaptive score global inflight result format error, fallback to local adaptive score: %+v", valArray)
			candidate := lb.selectLocalAdaptiveScore(ctx)
			log.Debugf("adaptive score fallback selected service: %s", candidate)
			proxywasm.ResumeHttpRequest()
			return
		}
		candidate := valArray[0].String()
		if !lb.applyGlobalInflightCandidate(ctx, globalInflightKey, candidate) {
			candidate = lb.selectLocalAdaptiveScore(ctx)
			log.Debugf("adaptive score fallback selected service: %s", candidate)
			proxywasm.ResumeHttpRequest()
			return
		}
		log.Debugf("adaptive score global inflight selected service: %s", candidate)
		proxywasm.ResumeHttpRequest()
	})
	if err != nil {
		log.Errorf("adaptive score global inflight eval dispatch failed, fallback to local adaptive score: %+v", err)
		candidate := lb.selectLocalAdaptiveScore(ctx)
		log.Debugf("adaptive score fallback selected service: %s", candidate)
		return types.ActionContinue
	}
	return types.ActionPause
}

// Callbacks which are called in request path
func (lb ClusterEndpointLoadBalancer) HandleHttpRequestHeaders(ctx wrapper.HttpContext) types.Action {
	ctx.SetContext("request_start", time.Now().UnixMilli())
	candidate := lb.ServiceList[rand.Int()%len(lb.ServiceList)]
	var debugInfo string
	switch lb.Mode {
	case "LeastBusy":
		for svc, ongoingNum := range lb.ServiceRequestOngoing {
			if candidate == svc {
				continue
			}
			if lb.getRequestRate(candidate) >= lb.RateLimit {
				candidate = svc
			} else if ongoingNum < lb.ServiceRequestOngoing[candidate] && lb.getRequestRate(svc) < lb.RateLimit {
				candidate = svc
			}
		}
		for svc := range lb.ServiceRequestOngoing {
			debugInfo += fmt.Sprintf("[service: %s] {ongoing request: %d, total request: %d, request rate: %.2f}, ",
				svc, lb.ServiceRequestOngoing[svc], lb.ServiceRequestCount[svc], lb.getRequestRate(svc))
		}
	case "LeastFirstTokenLatency":
		candidateTTFT := lb.getServiceTTFT(candidate)
		for _, svc := range lb.ServiceList {
			if candidate == svc {
				continue
			}
			if lb.getRequestRate(candidate) >= lb.RateLimit {
				candidate = svc
				candidateTTFT = lb.getServiceTTFT(svc)
			} else if lb.getServiceTTFT(svc) < candidateTTFT && lb.getRequestRate(svc) < lb.RateLimit {
				candidate = svc
				candidateTTFT = lb.getServiceTTFT(svc)
			}
		}
		for _, svc := range lb.ServiceList {
			debugInfo += fmt.Sprintf("[service: %s] {average ttft: %.2f, total request: %d, request rate: %.2f}, ",
				svc, lb.getServiceTTFT(svc), lb.ServiceRequestCount[svc], lb.getRequestRate(svc))
		}
	case "LeastTotalLatency":
		candidateTotalRT := lb.getServiceTotalRT(candidate)
		for _, svc := range lb.ServiceList {
			if candidate == svc {
				continue
			}
			if lb.getRequestRate(candidate) >= lb.RateLimit {
				candidate = svc
				candidateTotalRT = lb.getServiceTotalRT(svc)
			} else if lb.getServiceTotalRT(svc) < candidateTotalRT && lb.getRequestRate(svc) < lb.RateLimit {
				candidate = svc
				candidateTotalRT = lb.getServiceTotalRT(svc)
			}
		}
		for _, svc := range lb.ServiceList {
			debugInfo += fmt.Sprintf("[service: %s] {average latency: %.2f, total request: %d, request rate: %.2f}, ",
				svc, lb.getServiceTotalRT(svc), lb.ServiceRequestCount[svc], lb.getRequestRate(svc))
		}
	case ModeAdaptiveScore:
		nowUnixMs := time.Now().UnixMilli()
		if lb.GlobalInflightEnabled && lb.redisClient != nil && lb.redisClient.Ready() {
			return lb.selectGlobalAdaptiveScore(ctx)
		}
		candidate = lb.selectAdaptiveScore(nowUnixMs)
		for _, svc := range lb.ServiceList {
			metrics := lb.ServiceAdaptiveMetrics[svc]
			debugInfo += fmt.Sprintf("[service: %s] {adaptive score: %.2f, ongoing request: %d, ewma ttft: %.2f, ewma latency: %.2f, errors: %d}, ",
				svc, lb.adaptiveScore(svc, nowUnixMs), lb.ServiceRequestOngoing[svc], metrics.EWMATTFT, metrics.EWMATotalRT, metrics.ErrorCount)
		}
	}
	debugInfo += fmt.Sprintf("final service: %s", candidate)
	log.Debug(debugInfo)
	lb.applySelectedService(ctx, candidate)
	return types.ActionContinue
}

func (lb ClusterEndpointLoadBalancer) HandleHttpRequestBody(ctx wrapper.HttpContext, body []byte) types.Action {
	return types.ActionContinue
}

func (lb ClusterEndpointLoadBalancer) HandleHttpResponseHeaders(ctx wrapper.HttpContext) types.Action {
	statusCode, _ := proxywasm.GetHttpResponseHeader(":status")
	ctx.SetContext("statusCode", statusCode)
	return types.ActionContinue
}

func (lb ClusterEndpointLoadBalancer) HandleHttpStreamingResponseBody(ctx wrapper.HttpContext, data []byte, endOfStream bool) []byte {
	if ctx.GetContext("ttft_recorded") == nil {
		candidate, ok := lb.getSelectedService(ctx)
		if !ok {
			return data
		}
		requestStart, ok := ctx.GetContext("request_start").(int64)
		if !ok {
			log.Errorf("request_start is missing from context")
			return data
		}
		statusCode, ok := ctx.GetContext("statusCode").(string)
		if !ok {
			statusCode = "200"
		}
		duration := float64(time.Now().UnixMilli() - requestStart)
		// punish failed request
		if statusCode != "200" {
			for _, svc := range lb.ServiceList {
				ttft := lb.getServiceTTFT(svc)
				if duration < ttft {
					duration = ttft
				}
			}
			duration *= 2
		}
		lb.FirstTokenLatencyRequests[candidate].Enqueue(duration)
		lb.recordAdaptiveTTFT(candidate, duration)
		ctx.SetContext("ttft_recorded", struct{}{})
	}
	return data
}

func (lb ClusterEndpointLoadBalancer) HandleHttpResponseBody(ctx wrapper.HttpContext, body []byte) types.Action {
	return types.ActionContinue
}

func (lb ClusterEndpointLoadBalancer) HandleHttpStreamDone(ctx wrapper.HttpContext) {
	candidate, ok := lb.getSelectedService(ctx)
	if !ok {
		return
	}
	lb.ServiceRequestOngoing[candidate] -= 1
	if globalSelected, _ := ctx.GetContext("global_inflight_selected").(bool); globalSelected && lb.redisClient != nil {
		globalInflightKey, _ := ctx.GetContext("global_inflight_key").(string)
		if globalInflightKey != "" {
			if err := lb.redisClient.HIncrBy(globalInflightKey, candidate, -1, nil); err != nil {
				log.Errorf("adaptive score global inflight decrement failed, service: %s, error: %v", candidate, err)
			}
		}
	}
	requestStart, ok := ctx.GetContext("request_start").(int64)
	if !ok {
		log.Errorf("request_start is missing from context")
		return
	}
	statusCode, ok := ctx.GetContext("statusCode").(string)
	if !ok {
		statusCode = "200"
	}
	duration := float64(time.Now().UnixMilli() - requestStart)
	// punish failed request
	if statusCode != "200" {
		for _, svc := range lb.ServiceList {
			rt := lb.getServiceTotalRT(svc)
			if duration < rt {
				duration = rt
			}
		}
		duration *= 2
	}
	lb.TotalLatencyRequests[candidate].Enqueue(duration)
	lb.recordAdaptiveTotalRT(candidate, duration, statusCode != "200", time.Now().UnixMilli())
}
