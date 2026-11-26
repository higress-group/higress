package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

const (
	// 上下文键
	RouteName     = "route"
	ClusterName   = "cluster"
	ModelName     = "ai_model"
	ProviderName  = "ai_cluster"
	ConsumerKey   = "x-mse-consumer"
	StartTimeKey  = "ai-route-monitor-start-time"
	StatusCodeKey = "status_code"
	UrlKey        = "url"
	APIName       = "api"
)

const (
	// 监控指标名称后缀
	MetricRequestTotal    = "request_total"
	MetricRequestSuccess  = "request_success"
	MetricRequestError4xx = "request_error_4xx"
	MetricRequestError5xx = "request_error_5xx"
	MetricDurationMs      = "llm_service_duration"
	MetricDurationCount   = "llm_duration_count"

	// 非200错误异常监控指标
	MetricErrorCount = "error_count"
	MetricErrorCode  = "error_code"
	MetricErrorType  = "error_type"
)

const (
	// 错误分类类别
	ErrorCategoryRateLimit         = "rate_limit"
	ErrorCategoryAuth              = "auth"
	ErrorCategoryInvalidRequest    = "invalid_request"
	ErrorCategoryServerError       = "server_error"
	ErrorCategoryTimeout           = "timeout"
	ErrorCategoryContentFilter     = "content_filter"
	ErrorCategoryModelOverloaded   = "model_overloaded"
	ErrorCategoryInsufficientQuota = "insufficient_quota"
	ErrorCategoryContextLength     = "context_length"
	ErrorCategoryUnknown           = "unknown"
)

func main() {}

func init() {
	wrapper.SetCtx(
		"ai-route-monitor",
		wrapper.ParseConfigBy(parseConfig),
		wrapper.ProcessRequestHeadersBy(onHttpRequestHeaders),
		wrapper.ProcessResponseHeadersBy(onHttpResponseHeaders),
		wrapper.ProcessResponseBodyBy(onHttpResponseBody),
		wrapper.ProcessStreamingResponseBodyBy(onStreamingResponseBody),
		wrapper.ProcessStreamDoneBy(onStreamDone),
	)
}

// RouteMonitorConfig 路由监控插件配置
type RouteMonitorConfig struct {
	// Prometheus 监控指标计数器（懒初始化）
	counterMetrics       map[string]proxywasm.MetricCounter
	legacyMetrics        bool
	terminalMetrics      bool
	source               dimensionConfig
	models               dimensionConfig
	asyncSubmissionPaths map[string]bool
}

// parseConfig 解析插件配置
func parseConfig(json gjson.Result, config *RouteMonitorConfig, log log.Log) error {
	if !json.IsObject() {
		return fmt.Errorf("configuration must be a JSON object")
	}
	config.counterMetrics = make(map[string]proxywasm.MetricCounter)
	return parseTerminalConfig(json, config)
}

// onHttpRequestHeaders 处理请求头，提取维度信息并记录开始时间
func onHttpRequestHeaders(ctx wrapper.HttpContext, config RouteMonitorConfig, log log.Log) types.Action {
	startTerminalRequest(ctx, config)
	ctx.DisableReroute()

	// 获取路由名称
	route := getRouteName()
	ctx.SetContext(RouteName, route)

	api, apiError := getAPIName()
	if apiError == nil {
		route = api
	}
	ctx.SetUserAttribute(APIName, api)

	// 	// 获取 upstream cluster 名称（原始值，用于 metric 维度中的 cluster 字段）
	cluster := getRawClusterName()
	// 	ctx.SetContext(ClusterName, cluster)

	// 获取服务提供者名称（从 cluster_name 解析，用于 metric 维度中的 upstream 字段）
	provider := parseProviderName(cluster)
	ctx.SetContext(ProviderName, provider)

	// 获取模型名称
	model := getModelFromRequest()
	ctx.SetContext(ModelName, model)

	// 获取 URI
	uri := "unknown"
	if pathHeader, err := proxywasm.GetHttpRequestHeader(":path"); err == nil && pathHeader != "" {
		uri = pathHeader
	}
	ctx.SetContext(UrlKey, uri)

	// 获取消费者标识
	consumer := "none"
	if consumerHeader, err := proxywasm.GetHttpRequestHeader(ConsumerKey); err == nil && consumerHeader != "" {
		consumer = consumerHeader
	}
	ctx.SetContext(ConsumerKey, consumer)

	// 记录请求开始时间
	ctx.SetContext(StartTimeKey, time.Now())

	log.Debugf("route=%s, provider=%s, model=%s, consumer=%s", route, provider, model, consumer)
	return types.ActionContinue
}

// onHttpResponseHeaders 处理响应头，记录监控指标
func onHttpResponseHeaders(ctx wrapper.HttpContext, config RouteMonitorConfig, log log.Log) types.Action {
	observeResponseHeaders(ctx)
	if !config.legacyMetrics {
		if terminalState(ctx) == nil {
			ctx.DontReadResponseBody()
		}
		return types.ActionContinue
	}
	if !wrapper.IsResponseFromUpstream() {
		if terminalState(ctx) == nil {
			ctx.DontReadResponseBody()
		}
		return types.ActionContinue
	}

	// 获取 HTTP 状态码
	statusCodeStr, err := proxywasm.GetHttpResponseHeader(":status")
	if err != nil {
		log.Debugf("failed to get response status code: %v", err)
		if terminalState(ctx) == nil {
			ctx.DontReadResponseBody()
		}
		return types.ActionContinue
	}

	statusCode, err := strconv.Atoi(statusCodeStr)
	if err != nil {
		log.Debugf("failed to parse status code: %v, value: %q", err, statusCodeStr)
		if terminalState(ctx) == nil {
			ctx.DontReadResponseBody()
		}
		return types.ActionContinue
	}

	// 记录监控指标
	writeMetric(ctx, config, statusCode, log)

	// 非200错误需要读取响应体提取错误信息分类
	if isErrorCode(statusCode) {
		ctx.SetContext(StatusCodeKey, statusCode)
		// Preserve the legacy buffered error-classification path. Successful SSE
		// stays incremental; error responses were already buffered by this plugin.
		ctx.BufferResponseBody()
		ctx.SetResponseBodyBufferLimit(10 * 1024) // 限制 10KB
		return types.ActionContinue
	}

	// 正常响应不需要读取响应体
	if terminalState(ctx) == nil {
		ctx.DontReadResponseBody()
	}
	return types.ActionContinue
}

// writeMetric 写入监控指标（参考 ai-statistics 指标上报模式）
func writeMetric(ctx wrapper.HttpContext, config RouteMonitorConfig, statusCode int, log log.Log) {
	// 从上下文获取维度信息
	route, _ := ctx.GetContext(RouteName).(string)
	if route == "" {
		route = "none"
	}
	provider, _ := ctx.GetContext(ProviderName).(string)
	if provider == "" {
		provider = "unknown"
	}
	model, _ := ctx.GetContext(ModelName).(string)
	if model == "" {
		model = "unknown"
	}
	consumer, _ := ctx.GetContext(ConsumerKey).(string)
	if consumer == "" {
		consumer = "none"
	}
	uri, _ := ctx.GetContext(UrlKey).(string)
	if uri == "" {
		uri = "unknown"
	}

	log.Debugf("ai-route-monitor writeMetric: route=%s provider=%s model=%s consumer=%s uri=%s statusCode=%d",
		route, provider, model, consumer, uri, statusCode)

	// 记录请求计数
	config.incrementCounter(generateMetricName(route, provider, model, consumer, MetricRequestTotal, uri), 1)

	// 根据状态码分类记录
	if statusCode >= 200 && statusCode < 400 {
		config.incrementCounter(generateMetricName(route, provider, model, consumer, MetricRequestSuccess, uri), 1)
	} else if statusCode >= 400 && statusCode < 500 {
		config.incrementCounter(generateMetricName(route, provider, model, consumer, MetricRequestError4xx, uri), 1)
	} else if statusCode >= 500 && statusCode < 600 {
		config.incrementCounter(generateMetricName(route, provider, model, consumer, MetricRequestError5xx, uri), 1)
	}

	// 记录请求耗时（毫秒）
	if startTimeCtx := ctx.GetContext(StartTimeKey); startTimeCtx != nil {
		if startTime, ok := startTimeCtx.(time.Time); ok {
			elapsedMs := uint64(time.Since(startTime).Milliseconds())
			config.incrementCounter(generateMetricName(route, provider, model, consumer, MetricDurationMs, uri), elapsedMs)
			config.incrementCounter(generateMetricName(route, provider, model, consumer, MetricDurationCount, uri), 1)
		}
	}
}

// onHttpResponseBody 处理错误响应体，提取错误信息并分类
func onHttpResponseBody(ctx wrapper.HttpContext, config RouteMonitorConfig, body []byte, log log.Log) types.Action {
	if state := terminalState(ctx); state != nil {
		state.observeBody(body, true)
	}
	if !config.legacyMetrics {
		return types.ActionContinue
	}
	if !wrapper.IsResponseFromUpstream() {
		return types.ActionContinue
	}

	statusCodeCtx := ctx.GetContext(StatusCodeKey)
	if statusCodeCtx == nil {
		// 非错误响应不会到这里
		return types.ActionContinue
	}

	statusCode, ok := statusCodeCtx.(int)
	if !ok {
		return types.ActionContinue
	}

	if !isErrorCode(statusCode) {
		return types.ActionContinue
	}

	// 记录错误分类指标
	writeErrorMetrics(ctx, config, statusCode, body, log)

	return types.ActionContinue
}

// writeErrorMetrics 写入非200错误异常监控指标（按 route+provider+model+consumer+error 维度分类）
func writeErrorMetrics(ctx wrapper.HttpContext, config RouteMonitorConfig, statusCode int, body []byte, log log.Log) {
	route, _ := ctx.GetContext(RouteName).(string)
	if route == "" {
		route = "none"
	}
	provider, _ := ctx.GetContext(ProviderName).(string)
	if provider == "" {
		provider = "unknown"
	}
	model, _ := ctx.GetContext(ModelName).(string)
	if model == "" {
		model = "unknown"
	}
	consumer, _ := ctx.GetContext(ConsumerKey).(string)
	if consumer == "" {
		consumer = "none"
	}
	uri, _ := ctx.GetContext(UrlKey).(string)
	if uri == "" {
		uri = "unknown"
	}

	// 提取错误消息
	errorMsg := extractErrorMessage(body)

	// 记录错误总数
	config.incrementCounter(generateMetricName(route, provider, model, consumer, MetricErrorCount, uri), 1)

	// 记录按错误码分类 (如 error_code_429_count)
	errorCodeMetric := fmt.Sprintf("%s_count_%d", MetricErrorCode, statusCode)
	config.incrementCounter(generateMetricName(route, provider, model, consumer, errorCodeMetric, uri), 1)

	// 记录按错误类别分类 (如 error_type_rate_limit_count)
	category := classifyError(statusCode, errorMsg)
	errorCategoryMetric := fmt.Sprintf("%s_count_%s", MetricErrorType, category)
	config.incrementCounter(generateMetricName(route, provider, model, consumer, errorCategoryMetric, uri), 1)

	log.Debugf("error metrics: route=%s provider=%s model=%s consumer=%s uri=%s code=%d category=%s msg=%s",
		route, provider, model, consumer, uri, statusCode, category, truncateString(errorMsg, 100))
}

// classifyError 根据 HTTP 状态码和响应体错误消息进行分类
func classifyError(statusCode int, errorMsg string) string {
	msg := strings.ToLower(errorMsg)

	// 内容过滤
	if statusCode == 400 && (strings.Contains(msg, "content") && strings.Contains(msg, "filter") ||
		strings.Contains(msg, "safety") || strings.Contains(msg, "moderation") ||
		strings.Contains(msg, "blocked") || strings.Contains(msg, "content_filter")) {
		return ErrorCategoryContentFilter
	}

	// 上下文长度超限
	if statusCode == 400 && (strings.Contains(msg, "context length") ||
		strings.Contains(msg, "token limit") || strings.Contains(msg, "max token") ||
		strings.Contains(msg, "too long") || strings.Contains(msg, "maximum context") ||
		strings.Contains(msg, "reduce the length")) {
		return ErrorCategoryContextLength
	}

	// 无效请求
	if statusCode >= 400 && statusCode < 500 {
		if strings.Contains(msg, "invalid") || strings.Contains(msg, "bad request") ||
			strings.Contains(msg, "parameter") || strings.Contains(msg, "malformed") {
			return ErrorCategoryInvalidRequest
		}
	}

	// 认证错误
	if statusCode == 401 || statusCode == 403 {
		return ErrorCategoryAuth
	}
	if strings.Contains(msg, "auth") || strings.Contains(msg, "unauthorized") ||
		strings.Contains(msg, "forbidden") || strings.Contains(msg, "permission") ||
		strings.Contains(msg, "invalid api key") || strings.Contains(msg, "incorrect api key") ||
		strings.Contains(msg, "token") && (strings.Contains(msg, "invalid") || strings.Contains(msg, "expired")) {
		return ErrorCategoryAuth
	}

	// 配额不足
	if statusCode == 429 || strings.Contains(msg, "insufficient_quota") ||
		strings.Contains(msg, "billing") || strings.Contains(msg, "exceeded your current quota") ||
		strings.Contains(msg, "quota") && strings.Contains(msg, "exceed") {
		return ErrorCategoryInsufficientQuota
	}

	// 频率限制
	if statusCode == 429 || strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "rate_limit") || strings.Contains(msg, "too many requests") ||
		strings.Contains(msg, "throttl") {
		return ErrorCategoryRateLimit
	}

	// 模型过载
	if statusCode == 503 && (strings.Contains(msg, "overload") ||
		strings.Contains(msg, "overloaded") || strings.Contains(msg, "capacity") ||
		strings.Contains(msg, "busy")) {
		return ErrorCategoryModelOverloaded
	}
	if strings.Contains(msg, "model is overloaded") || strings.Contains(msg, "high traffic") {
		return ErrorCategoryModelOverloaded
	}

	// 超时
	if statusCode == 504 || statusCode == 408 ||
		strings.Contains(msg, "timeout") || strings.Contains(msg, "timed out") ||
		strings.Contains(msg, "deadline") {
		return ErrorCategoryTimeout
	}

	// 服务端错误
	if statusCode >= 500 {
		return ErrorCategoryServerError
	}
	if strings.Contains(msg, "internal") || strings.Contains(msg, "server error") {
		return ErrorCategoryServerError
	}

	// 4xx 但未被上述规则匹配
	if statusCode >= 400 && statusCode < 500 {
		return ErrorCategoryInvalidRequest
	}

	return ErrorCategoryUnknown
}

// extractErrorMessage 从响应体中提取错误消息
func extractErrorMessage(body []byte) string {
	if len(body) == 0 {
		return ""
	}

	// 限制处理的 body 大小
	maxBodySize := 10 * 1024
	if len(body) > maxBodySize {
		body = body[:maxBodySize]
	}

	// 尝试解析 JSON 获取错误消息
	jsonData := gjson.ParseBytes(body)
	errorFields := []string{"error.message", "error.code", "message", "error", "msg", "error.msg", "detail"}
	for _, field := range errorFields {
		if value := jsonData.Get(field); value.Exists() && value.String() != "" {
			msg := value.String()
			if len(msg) > 500 {
				return msg[:500]
			}
			return msg
		}
	}

	// 返回原始内容（限制长度）
	bodyStr := string(body)
	if len(bodyStr) > 500 {
		return bodyStr[:500]
	}
	return bodyStr
}

// isErrorCode 判断是否为错误状态码
func isErrorCode(statusCode int) bool {
	return statusCode >= 400 && statusCode < 600
}

// truncateString 截断字符串
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// generateMetricName 生成 Prometheus 指标名称
// 完全对齐 ai-statistics 的 route.{}.upstream.{}.model.{}.consumer.{}.metric.{} 格式
//
// Higress Envoy 内置的 stats tag 会将此格式自动解析为 Prometheus label：
//
//	route.RightCodes.upstream.RightCodes.model.gpt-image-2.consumer.none.metric.request_success_v1_images_generations
//	  → metric_request_success_v1_images_generations{route="RightCodes", upstream="RightCodes", model="gpt-image-2", consumer="none"}
//
// 维度对应关系：provider → upstream tag, model → model tag, uri → 拼入 metric 名
func generateMetricName(route, provider, model, consumer, metricType, uri string) string {
	if model == "unknown" {
		// 普通路由：model 填 none 保持格式一致
		return fmt.Sprintf("route.%s.upstream.%s.model.none.consumer.%s.metric.%s_%s",
			sanitizeMetricLabel(route),
			sanitizeMetricLabel(provider),
			sanitizeMetricLabel(consumer),
			metricType,
			sanitizeMetricLabel(uri))
	}
	return fmt.Sprintf("route.%s.upstream.%s.model.%s.consumer.%s.metric.%s_%s",
		sanitizeMetricLabel(route),
		sanitizeMetricLabel(provider),
		sanitizeMetricLabel(model),
		sanitizeMetricLabel(consumer),
		metricType,
		sanitizeMetricLabel(uri))
}

// sanitizeMetricLabel 清洗动态标签值，仅保留 [a-zA-Z0-9_\-]
// 将所有非安全字符替换为下划线
func sanitizeMetricLabel(s string) string {
	if s == "" {
		return "unknown"
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-' {
			b.WriteRune(c)
		} else {
			b.WriteRune('_')
		}
	}
	result := b.String()
	if result == "" {
		return "unknown"
	}
	return result
}

// incrementCounter 懒初始化并递增监控指标计数器
func (config *RouteMonitorConfig) incrementCounter(metricName string, inc uint64) {
	if inc == 0 {
		return
	}
	counter, ok := config.counterMetrics[metricName]
	if !ok {
		counter = proxywasm.DefineCounterMetric(metricName)
		config.counterMetrics[metricName] = counter
	}
	counter.Increment(inc)
}

func getAPIName() (string, error) {
	if raw, err := proxywasm.GetProperty([]string{"route_name"}); err != nil {
		return "-", err
	} else {
		parts := strings.Split(string(raw), "@")
		if len(parts) < 3 {
			return "-", errors.New("not api type")
		} else {
			return strings.Join(parts[:3], "@"), nil
		}
	}
}

// getRouteName 从 Envoy 属性获取路由名称
func getRouteName() string {
	if raw, err := proxywasm.GetProperty([]string{"route_name"}); err == nil {
		name := string(raw)
		if name != "" {
			return name
		}
	}
	return "none"
}

// getRawClusterName 获取原始 cluster_name（不修剪），用于 metric 维度
func getRawClusterName() string {
	if raw, err := proxywasm.GetProperty([]string{"cluster_name"}); err == nil {
		name := string(raw)
		if name != "" {
			return name
		}
	}
	return "none"
}

// parseProviderName 从 cluster_name 解析服务提供者名称
// 与 ai-success-rate-monitor 的 getClusterName 逻辑一致
// cluster_name 格式: outbound|443||llm-RightCodes.internal.dns
func parseProviderName(clusterName string) string {
	if clusterName == "none" || clusterName == "" {
		return "unknown"
	}
	parts := strings.Split(clusterName, "||")
	if len(parts) >= 2 {
		serviceName := parts[1]
		// 移除 llm- 前缀
		serviceName = strings.TrimPrefix(serviceName, "llm-")
		// 提取第一个点之前的部分
		if idx := strings.Index(serviceName, "."); idx > 0 {
			serviceName = serviceName[:idx]
		}
		if serviceName != "" {
			return serviceName
		}
	}
	return "unknown"
}

// getModelFromRequest 从 Envoy 属性获取模型名称
// 与 ai-success-rate-monitor 的 getModelFromRequest 逻辑一致
func getModelFromRequest() string {
	if requestModel, err := proxywasm.GetProperty([]string{"wasm.requestModel"}); err == nil {
		model := string(requestModel)
		if model != "" {
			return model
		}
	}
	return "unknown"
}
