package cluster_metrics

import (
	"testing"
	"time"

	"github.com/higress-group/wasm-go/pkg/iface"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

func init() {
	log.SetPluginLog(noopLog{})
}

func TestAdaptiveScoreConfigDefaults(t *testing.T) {
	lb, err := NewClusterEndpointLoadBalancer(gjson.Parse(`{
		"mode": "AdaptiveScore",
		"service_list": ["svc-a", "svc-b"]
	}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lb.EWMABeta != DefaultEWMABeta {
		t.Fatalf("expected default ewma_beta %.2f, got %.2f", DefaultEWMABeta, lb.EWMABeta)
	}
	if lb.P2CChoices != DefaultP2CChoices {
		t.Fatalf("expected default p2c_choices %d, got %d", DefaultP2CChoices, lb.P2CChoices)
	}
	if lb.TTFTWeight != DefaultTTFTWeight || lb.TotalRTWeight != DefaultTotalLatencyWeight {
		t.Fatalf("unexpected latency weights: ttft=%.2f total=%.2f", lb.TTFTWeight, lb.TotalRTWeight)
	}
	for _, svc := range lb.ServiceList {
		if lb.ServiceAdaptiveMetrics[svc] == nil {
			t.Fatalf("missing adaptive metrics for %s", svc)
		}
	}
}

func TestAdaptiveScoreNormalizesUnknownMetricsMissingPolicy(t *testing.T) {
	lb, err := NewClusterEndpointLoadBalancer(gjson.Parse(`{
		"mode": "AdaptiveScore",
		"metrics_missing_policy": "unknown",
		"service_list": ["svc-a", "svc-b"]
	}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lb.MetricsMissingPolicy != MetricsMissingPolicyLeast {
		t.Fatalf("expected unknown metrics_missing_policy to default to %q, got %q", MetricsMissingPolicyLeast, lb.MetricsMissingPolicy)
	}
}

func TestAdaptiveScoreRejectsInvalidConfig(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "invalid ewma beta",
			body: `{"mode":"AdaptiveScore","ewma_beta":1.2,"service_list":["svc-a"]}`,
		},
		{
			name: "zero latency weights",
			body: `{"mode":"AdaptiveScore","ttft_weight":0,"total_latency_weight":0,"service_list":["svc-a"]}`,
		},
		{
			name: "negative error penalty",
			body: `{"mode":"AdaptiveScore","error_penalty":-1,"service_list":["svc-a"]}`,
		},
		{
			name: "negative cooldown",
			body: `{"mode":"AdaptiveScore","failure_cooldown_ms":-1,"service_list":["svc-a"]}`,
		},
		{
			name: "empty service list",
			body: `{"mode":"AdaptiveScore","service_list":[]}`,
		},
		{
			name: "negative global inflight timeout",
			body: `{"mode":"AdaptiveScore","global_inflight_timeout":-1,"service_list":["svc-a"]}`,
		},
		{
			name: "negative global inflight key ttl",
			body: `{"mode":"AdaptiveScore","global_inflight_key_ttl":-1,"service_list":["svc-a"]}`,
		},
		{
			name: "missing redis service when global inflight is enabled",
			body: `{"mode":"AdaptiveScore","global_inflight_enabled":true,"service_list":["svc-a"]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewClusterEndpointLoadBalancer(gjson.Parse(tt.body)); err == nil {
				t.Fatal("expected config error")
			}
		})
	}
}

func TestAdaptiveScoreEWMAAndFailureState(t *testing.T) {
	lb := newAdaptiveScoreLB(t)

	lb.recordAdaptiveTTFT("svc-a", 100)
	lb.recordAdaptiveTTFT("svc-a", 300)
	if got := lb.ServiceAdaptiveMetrics["svc-a"].EWMATTFT; got != 200 {
		t.Fatalf("expected TTFT EWMA 200, got %.2f", got)
	}

	lb.recordAdaptiveTotalRT("svc-a", 800, true, 1000)
	metrics := lb.ServiceAdaptiveMetrics["svc-a"]
	if metrics.EWMATotalRT != 800 {
		t.Fatalf("expected total RT EWMA 800, got %.2f", metrics.EWMATotalRT)
	}
	if metrics.ErrorCount != 1 || metrics.SuccessCount != 0 || metrics.ConsecutiveErrors != 1 {
		t.Fatalf("unexpected failure counters: success=%d errors=%d consecutive=%d", metrics.SuccessCount, metrics.ErrorCount, metrics.ConsecutiveErrors)
	}
	if metrics.CooldownUntilUnixMs != 1000+DefaultFailureCooldownMs {
		t.Fatalf("unexpected cooldown deadline: %d", metrics.CooldownUntilUnixMs)
	}

	lb.recordAdaptiveTotalRT("svc-a", 400, false, 2000)
	if metrics.SuccessCount != 1 || metrics.ConsecutiveErrors != 0 {
		t.Fatalf("success should reset consecutive errors, success=%d consecutive=%d", metrics.SuccessCount, metrics.ConsecutiveErrors)
	}
	if got := metrics.EWMATotalRT; got != 600 {
		t.Fatalf("expected total RT EWMA 600, got %.2f", got)
	}
}

func TestAdaptiveScorePrefersLowerLatencyAndInflight(t *testing.T) {
	lb := newAdaptiveScoreLB(t)
	lb.ServiceAdaptiveMetrics["svc-a"] = &AdaptiveMetrics{
		EWMATTFT:     100,
		EWMATotalRT:  500,
		HasTTFT:      true,
		HasTotalRT:   true,
		SuccessCount: 10,
	}
	lb.ServiceAdaptiveMetrics["svc-b"] = &AdaptiveMetrics{
		EWMATTFT:     20,
		EWMATotalRT:  80,
		HasTTFT:      true,
		HasTotalRT:   true,
		SuccessCount: 10,
	}
	if got := lb.selectAdaptiveScore(1000); got != "svc-b" {
		t.Fatalf("expected lower latency service, got %q", got)
	}

	lb.ServiceRequestOngoing["svc-b"] = 100
	if got := lb.selectAdaptiveScore(1000); got != "svc-a" {
		t.Fatalf("expected lower inflight service after pressure increase, got %q", got)
	}
}

func TestAdaptiveScoreAvoidsCooldownWhenAlternativeExists(t *testing.T) {
	lb := newAdaptiveScoreLB(t)
	lb.ServiceAdaptiveMetrics["svc-a"] = &AdaptiveMetrics{
		EWMATTFT:            10,
		EWMATotalRT:         20,
		HasTTFT:             true,
		HasTotalRT:          true,
		ErrorCount:          1,
		ConsecutiveErrors:   1,
		CooldownUntilUnixMs: 2000,
	}
	lb.ServiceAdaptiveMetrics["svc-b"] = &AdaptiveMetrics{
		EWMATTFT:     300,
		EWMATotalRT:  600,
		HasTTFT:      true,
		HasTotalRT:   true,
		SuccessCount: 1,
	}
	if got := lb.selectAdaptiveScore(1000); got != "svc-b" {
		t.Fatalf("expected non-cooling service, got %q", got)
	}
	if got := lb.selectAdaptiveScore(3000); got != "svc-a" {
		t.Fatalf("expected recovered low-latency service, got %q", got)
	}
}

func TestAdaptiveScoreUsesLeastBusyForMissingMetrics(t *testing.T) {
	lb := newAdaptiveScoreLB(t)
	lb.ServiceRequestOngoing["svc-a"] = 3
	lb.ServiceRequestOngoing["svc-b"] = 1
	if got := lb.selectAdaptiveScore(1000); got != "svc-b" {
		t.Fatalf("expected least busy service for missing metrics, got %q", got)
	}
}

func TestAdaptiveScoreRespectsRateLimitWhenAlternativeExists(t *testing.T) {
	lb, err := NewClusterEndpointLoadBalancer(gjson.Parse(`{
		"mode": "AdaptiveScore",
		"p2c_choices": 2,
		"rate_limit": 0.6,
		"service_list": ["svc-a", "svc-b"]
	}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lb.ServiceRequestCount["svc-a"] = 7
	lb.ServiceRequestCount["svc-b"] = 3
	lb.ServiceAdaptiveMetrics["svc-a"] = &AdaptiveMetrics{
		EWMATTFT:     1,
		EWMATotalRT:  1,
		HasTTFT:      true,
		HasTotalRT:   true,
		SuccessCount: 10,
	}
	lb.ServiceAdaptiveMetrics["svc-b"] = &AdaptiveMetrics{
		EWMATTFT:     100,
		EWMATotalRT:  100,
		HasTTFT:      true,
		HasTotalRT:   true,
		SuccessCount: 10,
	}
	if got := lb.selectAdaptiveScore(1000); got != "svc-b" {
		t.Fatalf("expected service below rate limit, got %q", got)
	}
}

func TestAdaptiveScoreGlobalInflightConfigDefaults(t *testing.T) {
	lb, err := NewClusterEndpointLoadBalancer(gjson.Parse(`{
		"mode": "AdaptiveScore",
		"service_list": ["svc-a", "svc-b"]
	}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lb.GlobalInflightEnabled {
		t.Fatal("global inflight should be disabled by default")
	}
	if lb.GlobalInflightKeyPrefix != DefaultGlobalInflightKeyPrefix {
		t.Fatalf("expected default key prefix %q, got %q", DefaultGlobalInflightKeyPrefix, lb.GlobalInflightKeyPrefix)
	}
	if lb.GlobalInflightTimeoutMs != DefaultGlobalInflightTimeoutMs {
		t.Fatalf("expected default timeout %d, got %d", DefaultGlobalInflightTimeoutMs, lb.GlobalInflightTimeoutMs)
	}
	if lb.GlobalInflightKeyTTL != DefaultGlobalInflightKeyTTL {
		t.Fatalf("expected default key ttl %d, got %d", DefaultGlobalInflightKeyTTL, lb.GlobalInflightKeyTTL)
	}
}

func TestAdaptiveScoreBuildsGlobalInflightLuaKeys(t *testing.T) {
	lb := newAdaptiveScoreLB(t)
	lb.GlobalInflightKeyPrefix = "test-prefix"
	lb.ServiceAdaptiveMetrics["svc-a"] = &AdaptiveMetrics{
		EWMATTFT:     10,
		EWMATotalRT:  20,
		HasTTFT:      true,
		HasTotalRT:   true,
		SuccessCount: 1,
	}
	lb.ServiceAdaptiveMetrics["svc-b"] = &AdaptiveMetrics{
		EWMATTFT:     30,
		EWMATotalRT:  40,
		HasTTFT:      true,
		HasTotalRT:   true,
		SuccessCount: 1,
	}

	keys := lb.buildGlobalInflightLuaKeys(1234, "route-a")
	if got := keys[0]; got != int64(1234000) {
		t.Fatalf("unexpected seed: %v", got)
	}
	if got := keys[1]; got != "test-prefix:route-a:AdaptiveScore" {
		t.Fatalf("unexpected redis key: %v", got)
	}
	if got := keys[2]; got != 2 {
		t.Fatalf("unexpected service count: %v", got)
	}
	if got := keys[3]; got != DefaultGlobalInflightKeyTTL {
		t.Fatalf("unexpected key ttl: %v", got)
	}
	if keys[4] != "svc-a" || keys[6] != "svc-b" {
		t.Fatalf("unexpected service ordering in lua keys: %v", keys)
	}
	if _, ok := keys[5].(float64); !ok {
		t.Fatalf("expected svc-a score to be float64, got %T", keys[5])
	}
	if _, ok := keys[7].(float64); !ok {
		t.Fatalf("expected svc-b score to be float64, got %T", keys[7])
	}
}

func TestAdaptiveScoreGlobalInflightKeysUseFilteredCandidates(t *testing.T) {
	lb, err := NewClusterEndpointLoadBalancer(gjson.Parse(`{
		"mode": "AdaptiveScore",
		"p2c_choices": 3,
		"rate_limit": 0.6,
		"service_list": ["svc-a", "svc-b", "svc-c"]
	}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lb.ServiceRequestCount["svc-a"] = 7
	lb.ServiceRequestCount["svc-b"] = 2
	lb.ServiceRequestCount["svc-c"] = 1
	lb.ServiceAdaptiveMetrics["svc-a"].CooldownUntilUnixMs = 2000
	lb.ServiceAdaptiveMetrics["svc-b"].CooldownUntilUnixMs = 2000
	lb.ServiceAdaptiveMetrics["svc-c"] = &AdaptiveMetrics{
		EWMATTFT:     30,
		EWMATotalRT:  40,
		HasTTFT:      true,
		HasTotalRT:   true,
		SuccessCount: 1,
	}

	keys := lb.buildGlobalInflightLuaKeys(1000, "route-a")
	if got := keys[2]; got != 1 {
		t.Fatalf("expected only one eligible candidate, got %v in keys %v", got, keys)
	}
	if got := keys[4]; got != "svc-c" {
		t.Fatalf("expected only non-limited and non-cooling candidate svc-c, got %v", got)
	}
}

func TestAdaptiveScoreGlobalInflightKeysUseP2CSample(t *testing.T) {
	lb, err := NewClusterEndpointLoadBalancer(gjson.Parse(`{
		"mode": "AdaptiveScore",
		"p2c_choices": 2,
		"service_list": ["svc-a", "svc-b", "svc-c", "svc-d"]
	}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	keys := lb.buildGlobalInflightLuaKeys(1000, "route-a")
	if got := keys[2]; got != 2 {
		t.Fatalf("expected global lua keys to honor p2c_choices=2, got service count %v in keys %v", got, keys)
	}
	if len(keys) != 8 {
		t.Fatalf("expected 4 metadata entries plus 2 service/score pairs, got %d entries: %v", len(keys), keys)
	}
}

func TestAdaptiveScoreRejectsUnknownGlobalInflightCandidate(t *testing.T) {
	lb := newAdaptiveScoreLB(t)
	redis := &fakeGlobalInflightRedis{}
	lb.redisClient = redis
	ctx := newFakeHTTPContext()

	if ok := lb.applyGlobalInflightCandidate(ctx, "test-prefix:route-a:AdaptiveScore", "svc-unknown"); ok {
		t.Fatal("expected unknown global inflight candidate to be rejected")
	}
	if got := ctx.GetContext(lb.ClusterHeader); got != nil {
		t.Fatalf("unknown candidate should not be written to context, got %v", got)
	}
	if redis.hIncrByKey != "test-prefix:route-a:AdaptiveScore" {
		t.Fatalf("unexpected rollback redis key: %q", redis.hIncrByKey)
	}
	if redis.hIncrByField != "svc-unknown" {
		t.Fatalf("unexpected rollback redis field: %q", redis.hIncrByField)
	}
	if redis.hIncrByDelta != -1 {
		t.Fatalf("expected rollback decrement -1, got %d", redis.hIncrByDelta)
	}
}

func TestAdaptiveScoreStreamDoneDecrementsGlobalInflight(t *testing.T) {
	lb := newAdaptiveScoreLB(t)
	redis := &fakeGlobalInflightRedis{}
	lb.redisClient = redis
	ctx := newFakeHTTPContext()
	ctx.SetContext(lb.ClusterHeader, "svc-a")
	ctx.SetContext("global_inflight_selected", true)
	ctx.SetContext("global_inflight_key", "test-prefix:route-a:AdaptiveScore")
	ctx.SetContext("request_start", int64(1000))
	ctx.SetContext("statusCode", "200")
	lb.ServiceRequestOngoing["svc-a"] = 1

	lb.HandleHttpStreamDone(ctx)

	if redis.hIncrByKey != "test-prefix:route-a:AdaptiveScore" {
		t.Fatalf("unexpected redis key: %q", redis.hIncrByKey)
	}
	if redis.hIncrByField != "svc-a" {
		t.Fatalf("unexpected redis field: %q", redis.hIncrByField)
	}
	if redis.hIncrByDelta != -1 {
		t.Fatalf("expected decrement -1, got %d", redis.hIncrByDelta)
	}
	if got := lb.ServiceRequestOngoing["svc-a"]; got != 0 {
		t.Fatalf("expected local ongoing to decrement to 0, got %d", got)
	}
}

func TestAdaptiveScoreStreamingResponseSkipsMissingCandidate(t *testing.T) {
	lb := newAdaptiveScoreLB(t)
	ctx := newFakeHTTPContext()
	ctx.SetContext("request_start", timeNowMinus(10))
	ctx.SetContext("statusCode", "200")

	data := []byte("chunk")
	got := lb.HandleHttpStreamingResponseBody(ctx, data, false)

	if string(got) != string(data) {
		t.Fatalf("expected response body to be returned unchanged, got %q", string(got))
	}
	if got := lb.FirstTokenLatencyRequests["svc-a"].Size(); got != 0 {
		t.Fatalf("expected no TTFT metric to be recorded, got %d", got)
	}
}

func TestAdaptiveScoreStreamDoneSkipsMissingCandidate(t *testing.T) {
	lb := newAdaptiveScoreLB(t)
	ctx := newFakeHTTPContext()
	ctx.SetContext("request_start", timeNowMinus(10))
	ctx.SetContext("statusCode", "200")

	lb.HandleHttpStreamDone(ctx)

	if got := lb.ServiceRequestOngoing["svc-a"]; got != 0 {
		t.Fatalf("expected local ongoing to remain unchanged, got %d", got)
	}
	if got := lb.TotalLatencyRequests["svc-a"].Size(); got != 0 {
		t.Fatalf("expected no total latency metric to be recorded, got %d", got)
	}
}

func TestAdaptiveScoreStreamDoneRecordsMetricsWhenGlobalInflightKeyMissing(t *testing.T) {
	lb := newAdaptiveScoreLB(t)
	redis := &fakeGlobalInflightRedis{}
	lb.redisClient = redis
	ctx := newFakeHTTPContext()
	ctx.SetContext(lb.ClusterHeader, "svc-a")
	ctx.SetContext("global_inflight_selected", true)
	ctx.SetContext("request_start", timeNowMinus(10))
	ctx.SetContext("statusCode", "200")
	lb.ServiceRequestOngoing["svc-a"] = 1

	lb.HandleHttpStreamDone(ctx)

	if redis.hIncrByDelta != 0 {
		t.Fatalf("redis decrement should be skipped when key is missing, got delta %d", redis.hIncrByDelta)
	}
	if got := lb.TotalLatencyRequests["svc-a"].Size(); got != 1 {
		t.Fatalf("expected total latency to still be recorded, got queue size %d", got)
	}
	if !lb.ServiceAdaptiveMetrics["svc-a"].HasTotalRT {
		t.Fatal("expected adaptive total RT to still be recorded")
	}
}

func newAdaptiveScoreLB(t *testing.T) ClusterEndpointLoadBalancer {
	t.Helper()
	lb, err := NewClusterEndpointLoadBalancer(gjson.Parse(`{
		"mode": "AdaptiveScore",
		"p2c_choices": 2,
		"service_list": ["svc-a", "svc-b"]
	}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return lb
}

func timeNowMinus(deltaMs int64) int64 {
	return time.Now().UnixMilli() - deltaMs
}

type fakeGlobalInflightRedis struct {
	hIncrByKey   string
	hIncrByField string
	hIncrByDelta int
}

func (f *fakeGlobalInflightRedis) Ready() bool {
	return true
}

func (f *fakeGlobalInflightRedis) Eval(script string, numkeys int, keys, args []interface{}, callback wrapper.RedisResponseCallback) error {
	return nil
}

func (f *fakeGlobalInflightRedis) HIncrBy(key, field string, delta int, callback wrapper.RedisResponseCallback) error {
	f.hIncrByKey = key
	f.hIncrByField = field
	f.hIncrByDelta = delta
	return nil
}

type fakeHTTPContext struct {
	values map[string]interface{}
}

func newFakeHTTPContext() *fakeHTTPContext {
	return &fakeHTTPContext{values: map[string]interface{}{}}
}

func (f *fakeHTTPContext) Scheme() string                           { return "" }
func (f *fakeHTTPContext) Host() string                             { return "" }
func (f *fakeHTTPContext) Path() string                             { return "" }
func (f *fakeHTTPContext) Method() string                           { return "" }
func (f *fakeHTTPContext) SetContext(key string, value interface{}) { f.values[key] = value }
func (f *fakeHTTPContext) GetContext(key string) interface{}        { return f.values[key] }
func (f *fakeHTTPContext) GetBoolContext(key string, defaultValue bool) bool {
	if value, ok := f.values[key].(bool); ok {
		return value
	}
	return defaultValue
}
func (f *fakeHTTPContext) GetStringContext(key, defaultValue string) string {
	if value, ok := f.values[key].(string); ok {
		return value
	}
	return defaultValue
}
func (f *fakeHTTPContext) GetByteSliceContext(key string, defaultValue []byte) []byte {
	if value, ok := f.values[key].([]byte); ok {
		return value
	}
	return defaultValue
}
func (f *fakeHTTPContext) GetUserAttribute(key string) interface{}          { return nil }
func (f *fakeHTTPContext) SetUserAttribute(key string, value interface{})   {}
func (f *fakeHTTPContext) SetUserAttributeMap(kvmap map[string]interface{}) {}
func (f *fakeHTTPContext) GetUserAttributeMap() map[string]interface{}      { return nil }
func (f *fakeHTTPContext) WriteUserAttributeToLog() error                   { return nil }
func (f *fakeHTTPContext) WriteUserAttributeToLogWithKey(key string) error  { return nil }
func (f *fakeHTTPContext) WriteUserAttributeToTrace() error                 { return nil }
func (f *fakeHTTPContext) DontReadRequestBody()                             {}
func (f *fakeHTTPContext) DontReadResponseBody()                            {}
func (f *fakeHTTPContext) BufferRequestBody()                               {}
func (f *fakeHTTPContext) BufferResponseBody()                              {}
func (f *fakeHTTPContext) NeedPauseStreamingResponse()                      {}
func (f *fakeHTTPContext) PushBuffer(buffer []byte)                         {}
func (f *fakeHTTPContext) PopBuffer() []byte                                { return nil }
func (f *fakeHTTPContext) BufferQueueSize() int                             { return 0 }
func (f *fakeHTTPContext) DisableReroute()                                  {}
func (f *fakeHTTPContext) SetRequestBodyBufferLimit(byteSize uint32)        {}
func (f *fakeHTTPContext) SetResponseBodyBufferLimit(byteSize uint32)       {}
func (f *fakeHTTPContext) RouteCall(method, url string, headers [][2]string, body []byte, callback iface.RouteResponseCallback) error {
	return nil
}
func (f *fakeHTTPContext) GetExecutionPhase() iface.HTTPExecutionPhase { return iface.Done }

type noopLog struct{}

func (noopLog) Trace(msg string)                          {}
func (noopLog) Tracef(format string, args ...interface{}) {}
func (noopLog) Debug(msg string)                          {}
func (noopLog) Debugf(format string, args ...interface{}) {}
func (noopLog) Info(msg string)                           {}
func (noopLog) Infof(format string, args ...interface{})  {}
func (noopLog) Warn(msg string)                           {}
func (noopLog) Warnf(format string, args ...interface{})  {}
func (noopLog) Error(msg string)                          {}
func (noopLog) Errorf(format string, args ...interface{}) {}
func (noopLog) Critical(msg string)                       {}
func (noopLog) Criticalf(format string, args ...interface{}) {
}
func (noopLog) ResetID(pluginID string) {}
