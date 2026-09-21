package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

const (
	defaultAPITimeout int32 = 5000
	OriginalAPIKey    string = "X-Original-Api-Key"
)

const (
	CtxKeyBalanceChecked = "balance_checked"
	CtxKeyAPIKey         = "api_key"
)

// OpenAIError OpenAI 兼容的错误响应结构
type OpenAIError struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

type BalanceCheckConfig struct {
	checkClient  wrapper.HttpClient
	apiUrl      string
	serviceName string
	apiTimeout   int32
	failMessage  string
	failCode     int32
	skipPaths    []string
}

func main() {}

func init() {
	wrapper.SetCtx(
		"ai-balance-check",
		wrapper.ParseConfig(parseConfig),
		wrapper.ProcessRequestHeaders(onHttpRequestHeaders),
		wrapper.ProcessRequestBody(onHttpRequestBody),
	)
}

func parseConfig(json gjson.Result, config *BalanceCheckConfig) error {
	config.apiUrl = json.Get("apiUrl").String()
	if config.apiUrl == "" {
		log.Infof("ai-balance-check: apiUrl not configured, plugin will be disabled")
		return nil
	}

	config.serviceName = json.Get("serviceName").String()
	if config.serviceName == "" {
		return fmt.Errorf("serviceName is required")
	}

	if json.Get("apiTimeout").Exists() {
		config.apiTimeout = int32(json.Get("apiTimeout").Int())
	} else {
		config.apiTimeout = defaultAPITimeout
	}

	config.failMessage = json.Get("failMessage").String()
	if config.failMessage == "" {
		config.failMessage = "Insufficient balance, please recharge"
	}

	if json.Get("failCode").Exists() {
		config.failCode = int32(json.Get("failCode").Int())
	} else {
		config.failCode = 402 // Default to Payment Required
	}

	// Parse skipPaths - list of URI path prefixes that should skip balance check
	// 支持两种配置形式：
	//   "skipPaths": ["/v1/images/tasks", "/v1/async/"]   (数组形式)
	//   "skipPaths": "/v1/images/tasks,/v1/async/"         (逗号分隔字符串形式)
	skipPathsResult := json.Get("skipPaths")
	if skipPathsResult.IsArray() {
		skipPathsResult.ForEach(func(_, value gjson.Result) bool {
			path := strings.TrimSpace(value.String())
			if path != "" {
				config.skipPaths = append(config.skipPaths, path)
			}
			return true
		})
	} else if skipPathsResult.String() != "" {
		for _, path := range strings.Split(skipPathsResult.String(), ",") {
			path = strings.TrimSpace(path)
			if path != "" {
				config.skipPaths = append(config.skipPaths, path)
			}
		}
	}

	// Parse URL to get host and port
	parsedUrl, err := url.Parse(config.apiUrl)
	if err != nil {
		return fmt.Errorf("invalid apiUrl: %v", err)
	}

	actualHost := parsedUrl.Hostname()
	if actualHost == "" {
		return fmt.Errorf("invalid apiUrl: missing hostname")
	}

	port := int64(80)
	if parsedUrl.Scheme == "https" {
		port = 443
	}
	if actualPort := parsedUrl.Port(); actualPort != "" {
		if p, err := strconv.Atoi(actualPort); err == nil {
			port = int64(p)
		}
	}

	// Create HTTP client for balance check API
	config.checkClient = wrapper.NewClusterClient(wrapper.DnsCluster{
		ServiceName: config.serviceName,
		Domain:      actualHost,
		Port:        port,
	})

	log.Infof("ai-balance-check: configured, url=%s, service=%s, timeout=%dms, failMessage=%s, skipPaths=%v",
		config.apiUrl, config.serviceName, config.apiTimeout, config.failMessage, config.skipPaths)
	return nil
}

func sendErrorResponse(ctx wrapper.HttpContext, config *BalanceCheckConfig, message string) types.Action {
	log.Warnf("ai-balance-check: sending error response: %s", message)
	// 构建 OpenAI 兼容的错误响应
	errorInner := struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	}{
		Message: message,
		Type:    "insufficient_balance",
		Code:    "insufficient_balance",
	}
	errorResponse := OpenAIError{Error: errorInner}
	jsonData, err := json.Marshal(errorResponse)
	if err != nil {
		log.Errorf("ai-balance-check: failed to marshal error response: %v", err)
		return types.ActionContinue
	}

	if err := proxywasm.SendHttpResponse(
		402,
		[][2]string{{"Content-Type", "application/json"}},
		jsonData,
		-1,
	); err != nil {
		log.Errorf("ai-balance-check: SendHttpResponse failed in sendErrorResponse: %v", err)
	}
	return types.HeaderStopAllIterationAndWatermark
}

// shouldSkipPath 判断请求路径是否应跳过余额校验。
// path 是 HTTP 请求路径（可能包含 query string）。
// skipPaths 是需要跳过的路径前缀列表。
// 匹配规则：去除 query 后做前缀匹配。
func shouldSkipPath(path string, skipPaths []string) bool {
	if len(skipPaths) == 0 {
		return false
	}
	// 去掉 query string
	pathOnly := path
	if idx := strings.IndexByte(path, '?'); idx >= 0 {
		pathOnly = path[:idx]
	}
	for _, prefix := range skipPaths {
		if prefix == "" {
			continue
		}
		if strings.HasPrefix(pathOnly, prefix) {
			return true
		}
	}
	return false
}

// onHttpRequestHeaders 在请求 header 阶段执行异步余额校验。
//
// 为什么放在 header 阶段(而非 body 阶段):
//   Envoy filter chain 按 "阶段(frame type)" 推进。所有 filter 的 decodeHeaders 先同步
//   跑完,然后再依次跑 decodeData。body 阶段的 ActionPause(=StopIterationAndBuffer) 只能
//   暂停 "当前 filter 自己" 的迭代,无法阻止排在后面的 filter(例如 ai-proxy)的 decodeData
//   被调用,从而导致 ai-proxy 改写 body、router filter 把请求发给 upstream。
//   header 阶段返回 HeaderStopAllIterationAndWatermark 则会暂停整条 filter chain 的
//   header + body + trailers,直到本插件显式调用 ResumeHttpRequest() / SendHttpResponse()。
func onHttpRequestHeaders(ctx wrapper.HttpContext, config BalanceCheckConfig) types.Action {
	// Skip if plugin is disabled
	if config.apiUrl == "" {
		log.Debugf("ai-balance-check: plugin disabled (no apiUrl)")
		return types.ActionContinue
	}

	// Check if already validated to avoid duplicate checks
	if ctx.GetContext(CtxKeyBalanceChecked) != nil {
		return types.ActionContinue
	}

	// 获取请求路径，检查是否在 skipPaths 白名单内（前缀匹配）
	// 跳过余额校验的 URI（例如：作画任务结果轮询接口）
	requestPath, _ := proxywasm.GetHttpRequestHeader(":path")
	if requestPath != "" && shouldSkipPath(requestPath, config.skipPaths) {
		log.Infof("ai-balance-check: skipping balance check for path (matched skipPaths): %s", requestPath)
		return types.ActionContinue
	}

	// Get API key from X-Original-Api-Key header
	apiKey, err := proxywasm.GetHttpRequestHeader(OriginalAPIKey)
	if err != nil {
		log.Errorf("ai-balance-check: failed to get header %s: %v", OriginalAPIKey, err)
		return sendErrorResponse(ctx, &config, config.failMessage)
	}

	if apiKey == "" {
		log.Warnf("ai-balance-check: missing header: %s", OriginalAPIKey)
		return sendErrorResponse(ctx, &config, config.failMessage)
	}

	// 标记为已校验，避免后续重复执行
	ctx.SetContext(CtxKeyBalanceChecked, true)
	ctx.SetContext(CtxKeyAPIKey, apiKey)

	// 余额校验在 header 阶段发起，不需要读取请求 body
	ctx.DontReadRequestBody()

	log.Infof("ai-balance-check: starting async balance check (header phase)")
	return checkBalance(ctx, config, apiKey)
}

// onHttpRequestBody 在请求 body 阶段不做任何业务处理。
// 余额校验已挪到 onHttpRequestHeaders 阶段；这里保留注册只是为了向后兼容，
// 实际不会被执行（header 阶段已 ctx.DontReadRequestBody()）。
func onHttpRequestBody(ctx wrapper.HttpContext, config BalanceCheckConfig, body []byte) types.Action {
	return types.ActionContinue
}

func checkBalance(ctx wrapper.HttpContext, config BalanceCheckConfig, apiKey string) types.Action {
	// Parse request path from apiUrl
	parsedUrl, err := url.Parse(config.apiUrl)
	if err != nil {
		log.Errorf("ai-balance-check: failed to parse apiUrl: %v", err)
		return sendErrorResponse(ctx, &config, "Balance check API error")
	}

	requestPath := parsedUrl.Path
	if requestPath == "" {
		requestPath = "/"
	}
	if parsedUrl.RawQuery != "" {
		requestPath += "?" + parsedUrl.RawQuery
	}

	// Make async call to balance check API
	log.Infof("ai-balance-check: checking balance for apiKey")

	err = config.checkClient.Get(
		requestPath,
		[][2]string{{"Authorization", "Bearer " + apiKey}},
		func(statusCode int, responseHeaders http.Header, responseBody []byte) {
			handleBalanceCheckResponse(ctx, config, statusCode, responseBody)
		},
		uint32(config.apiTimeout),
	)

	if err != nil {
		log.Errorf("ai-balance-check: failed to dispatch balance check request: %v - PASSING REQUEST DUE TO DISPATCH ERROR", err)
		// dispatch 失败时直接放行请求，不再暂停 filter chain
		return types.ActionContinue
	}

	// 暂停整条 filter chain（header + body + trailers），直到异步回调显式 ResumeHttpRequest 或 SendHttpResponse。
	// 注意：必须使用 HeaderStopAllIterationAndWatermark，而不是 ActionPause。
	// 因为 header 阶段的 ActionPause 等价于 HeaderStopIteration，只能拦住后续 filter 的 header，
	// 拦不住后续 filter 的 body。HeaderStopAllIterationAndWatermark 才能拦住所有阶段。
	return types.HeaderStopAllIterationAndWatermark
}

func handleBalanceCheckResponse(ctx wrapper.HttpContext, config BalanceCheckConfig, statusCode int, responseBody []byte) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("ai-balance-check: panic in handleBalanceCheckResponse (recovered): %v", r)
			_ = proxywasm.ResumeHttpRequest()
		}
	}()

	log.Infof("ai-balance-check: balance check response status=%d, body=%s", statusCode, string(responseBody))

	// Check HTTP status - 服务异常时记录日志但放行
	if statusCode < 200 || statusCode > 400 {
		log.Errorf("ai-balance-check: balance check API returned error status: %d, body=%s - PASSING REQUEST DUE TO SERVICE ERROR", statusCode, string(responseBody))
		// 服务异常，放行请求
		_ = proxywasm.ResumeHttpRequest()
		return
	}

	// Parse JSON response - API 直接返回 true/false
	var success bool
	if err := json.Unmarshal(responseBody, &success); err != nil {
		log.Errorf("ai-balance-check: failed to parse balance check response: %v, body=%s - PASSING REQUEST DUE TO PARSE ERROR", err, string(responseBody))
		// 解析失败，放行请求
		_ = proxywasm.ResumeHttpRequest()
		return
	}

	// Check balance result
	if !success {
		log.Warnf("ai-balance-check: balance check failed, API returned false - BLOCKING REQUEST")
		message := config.failMessage
		// 构建 OpenAI 兼容的错误响应
		errorInner := struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		}{
			Message: message,
			Type:    "insufficient_balance",
			Code:    "insufficient_balance",
		}
		errorResponse := OpenAIError{Error: errorInner}
		jsonData, err := json.Marshal(errorResponse)
		if err != nil {
			log.Errorf("ai-balance-check: failed to marshal error response: %v", err)
			_ = proxywasm.ResumeHttpRequest()
			return
		}
		if err := proxywasm.SendHttpResponse(
			402,
			[][2]string{{"Content-Type", "application/json"}},
			jsonData,
			-1,
		); err != nil {
			// SendHttpResponse 失败通常意味着 stream 状态不对（例如请求已经被上游处理）。
			// 此时再 ResumeHttpRequest 也无法纠正，但为避免请求一直被 pause 悬挂，这里放行。
			log.Errorf("ai-balance-check: SendHttpResponse failed in block path: %v - request may leak upstream", err)
			_ = proxywasm.ResumeHttpRequest()
		}
		return
	}

	// Balance check passed, resume the request
	log.Infof("ai-balance-check: balance check passed")
	_ = proxywasm.ResumeHttpRequest()
}
