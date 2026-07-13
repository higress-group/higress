package main

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/tokenusage"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

const (
	// Report configuration
	defaultReportTimeout int32 = 5000 // 5 seconds
	maxAccumulatedSize   int    = 100 * 1024 // 100KB

	// Context keys
	CtxKeyModel       = "token_model"
	CtxKeyInputToken  = "token_input"
	CtxKeyOutputToken = "token_output"
	CtxKeyTotalToken  = "token_total"
	CtxRequestStart   = "request_start_time"
	CtxFirstToken     = "first_token_time"
	CtxTokenUsage     = "token_usage_raw"
	CtxAccumulatedBody = "accumulated_body"
	CtxSkipPlugin     = "skip_token_report"
	providerTypeKey   = "providerType"

	// HTTP headers
	OriginalAPIKey = "X-Original-Api-Key"

	// Metrics
	LLMServiceDuration = "llm_service_duration"

	// Default disableOpenaiUsage value
	defaultDisableOpenaiUsage = false
)

// TokenUsageReportRequest represents the request body for token usage reporting
type TokenUsageReportRequest struct {
	RequestId   string `json:"requestId"`
	Model       string `json:"model"`
	UserKey     string `json:"userKey"`
	InputToken  int64  `json:"inputToken"`
	OutputToken int64  `json:"outputToken"`
	TotalToken  int64  `json:"totalToken"`
	Duration    int64  `json:"duration"`
	Timestamp   int64  `json:"timestamp"`
	StartTime   int64  `json:"startTime"`
	EndTime     int64  `json:"endTime"`
	TokenUsage  string `json:"tokenUsage"`
}

// TokenUsageReportConfig holds the configuration for token usage reporting
type TokenUsageReportConfig struct {
	reportClient       wrapper.HttpClient
	reportApiUrl       string
	reportServiceName  string
	reportTimeout      int32
	disableOpenaiUsage bool
}

func main() {}

func init() {
	wrapper.SetCtx(
		"ai-token-report",
		wrapper.ParseConfig(parseConfig),
		wrapper.ProcessRequestHeaders(onHttpRequestHeaders),
		wrapper.ProcessRequestBody(onHttpRequestBody),
		wrapper.ProcessStreamingResponseBody(onHttpStreamingBody),
		wrapper.ProcessResponseBody(onHttpResponseBody),
	)
}

func parseConfig(json gjson.Result, config *TokenUsageReportConfig) error {
	// Parse reportApiUrl (required)
	config.reportApiUrl = json.Get("reportApiUrl").String()
	if config.reportApiUrl == "" {
		log.Infof("ai-token-report: reportApiUrl not configured, plugin will be disabled")
		return nil
	}

	// Parse reportServiceName (optional)
	config.reportServiceName = json.Get("reportServiceName").String()
	if config.reportServiceName == "" {
		config.reportServiceName = "default"
		log.Warnf("reportServiceName not configured, using default: %s", "default")
	}

	// Parse reportTimeout (optional)
	if json.Get("reportTimeout").Exists() {
		config.reportTimeout = int32(json.Get("reportTimeout").Int())
	} else {
		config.reportTimeout = defaultReportTimeout
		log.Infof("reportTimeout not configured, using default: %dms", defaultReportTimeout)
	}

	// Parse disableOpenaiUsage (optional)
	if json.Get("disableOpenaiUsage").Exists() {
		config.disableOpenaiUsage = json.Get("disableOpenaiUsage").Bool()
	} else {
		config.disableOpenaiUsage = defaultDisableOpenaiUsage
		log.Infof("disableOpenaiUsage not configured, using default: %v", defaultDisableOpenaiUsage)
	}

	// Parse URL and extract actual domain and port
	parsedUrl, err := url.Parse(config.reportApiUrl)
	if err != nil {
		log.Errorf("failed to parse reportApiUrl: %v", err)
		return fmt.Errorf("invalid reportApiUrl: %v", err)
	}

	// Extract actual hostname (for Host header and TLS SNI)
	actualHost := parsedUrl.Hostname()
	if actualHost == "" {
		log.Errorf("failed to extract hostname from reportApiUrl: %s", config.reportApiUrl)
		return fmt.Errorf("invalid reportApiUrl: missing hostname")
	}

	// Extract port, default 80 (HTTP) or 443 (HTTPS)
	port := int64(80)
	if parsedUrl.Scheme == "https" {
		port = 443
	}

	// If port is explicitly specified in URL, use the specified port
	if actualPort := parsedUrl.Port(); actualPort != "" {
		if p, err := strconv.Atoi(actualPort); err == nil {
			port = int64(p)
		}
	}

	// Create HTTP client using DnsCluster
	config.reportClient = wrapper.NewClusterClient(wrapper.DnsCluster{
		ServiceName: config.reportServiceName,
		Domain:      actualHost,
		Port:        port,
	})

	log.Infof("ai-token-report: token usage report configured")
	log.Infof("  reportApiUrl: %s", config.reportApiUrl)
	log.Infof("  reportServiceName (服务名): %s", config.reportServiceName)
	log.Infof("  DnsCluster.ServiceName (服务名): %s", config.reportServiceName)
	log.Infof("  DnsCluster.Domain (实际域名): %s", actualHost)
	log.Infof("  DnsCluster.Port (服务端口): %d", port)
	log.Infof("  disableOpenaiUsage: %v", config.disableOpenaiUsage)
	log.Infof("  reportTimeout: %dms", config.reportTimeout)

	return nil
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config TokenUsageReportConfig) types.Action {
	// Check if plugin is enabled
	if config.reportApiUrl == "" {
		return types.ActionContinue
	}

	// 检查 providerType，如果为空则跳过本插件（非 AI 路由）
	providerType := getProviderType()
	if providerType == "" {
		log.Debugf("ai-token-report: providerType is empty, skipping (not an AI route)")
		ctx.DontReadRequestBody()
		ctx.DontReadResponseBody()
		ctx.SetContext(CtxSkipPlugin, true)
		return types.ActionContinue
	}

	// Clean up any residual data from previous requests to prevent memory leaks
	ctx.SetContext(CtxAccumulatedBody, nil)
	ctx.SetContext(CtxKeyInputToken, nil)
	ctx.SetContext(CtxKeyOutputToken, nil)
	ctx.SetContext(CtxKeyTotalToken, nil)
	ctx.SetContext(CtxTokenUsage, nil)

	// Set request start time
	ctx.SetContext(CtxRequestStart, time.Now().UnixMilli())

	requestModel := getRequestModel()
	ctx.SetContext(CtxKeyModel, requestModel)

	log.Debugf("ai-token-report: processing AI request, providerType=%s, requestModel=%s", providerType, requestModel)

	return types.ActionContinue
}

func onHttpRequestBody(ctx wrapper.HttpContext, config TokenUsageReportConfig, body []byte) types.Action {
	// 检查是否设置了跳过标记
	if ctx.GetContext(CtxSkipPlugin) != nil {
		return types.ActionContinue
	}

	// 检查 plugin 是否启用
	if config.reportApiUrl == "" {
		return types.ActionContinue
	}

	// 如果 requestModel 为空，尝试从 body 中提取
	model, _ := ctx.GetContext(CtxKeyModel).(string)
	if model == "" {
		parsed := gjson.ParseBytes(body)
		model = parsed.Get("model").String()
		if model != "" {
			ctx.SetContext(CtxKeyModel, model)
			log.Debugf("ai-token-report: extracted model from request body: %s", model)
		}
	}

	// 如果 model 为空，则跳过读取响应体
	if model == "" {
		log.Debugf("ai-token-report: request model is empty, skipping response body read")
		ctx.DontReadResponseBody()
	}

	return types.ActionContinue
}

func getProviderType() string {
	propertyKey := "wasm." + providerTypeKey
	if providerType, err := proxywasm.GetProperty([]string{propertyKey}); err == nil {
		return string(providerType)
	}
	return ""
}

func getRequestModel() string {
	propertyKey := "wasm.requestModel"
	if requestModel, err := proxywasm.GetProperty([]string{propertyKey}); err == nil {
		return string(requestModel)
	}
	return ""
}

// setUsageToContext sets token usage values to context
func setUsageToContext(ctx wrapper.HttpContext, usage tokenusage.TokenUsage) {
	ctx.SetContext(CtxKeyInputToken, usage.InputToken)
	ctx.SetContext(CtxKeyOutputToken, usage.OutputToken)
	ctx.SetContext(CtxKeyTotalToken, usage.TotalToken)
	if usageJSON, err := json.Marshal(usage); err == nil {
		ctx.SetContext(CtxTokenUsage, string(usageJSON))
	}
}

// shouldAccumulateChunk checks if a chunk should be accumulated for later usage parsing
func shouldAccumulateChunk(data []byte) bool {
	return bytes.Contains(data, []byte(`"usage"`)) ||
		bytes.Contains(data, []byte(`"usageMetadata"`)) ||
		bytes.Contains(data, []byte(`total_tokens`)) ||
		bytes.Contains(data, []byte(`totalTokenCount`)) ||
		bytes.Contains(data, []byte(`"data"`))
}

// accumulateChunk accumulates a chunk for later usage parsing, with size limit
func accumulateChunk(ctx wrapper.HttpContext, data []byte) bool {
	existingBody := ctx.GetByteSliceContext(CtxAccumulatedBody, []byte{})
	if len(existingBody) >= maxAccumulatedSize {
		log.Warnf("ai-token-report: accumulated body exceeded max size %d, skipping accumulation", maxAccumulatedSize)
		ctx.SetContext(CtxAccumulatedBody, nil)
		return false
	}
	accumulatedBody := make([]byte, len(existingBody)+len(data))
	copy(accumulatedBody, existingBody)
	copy(accumulatedBody[len(existingBody):], data)
	ctx.SetContext(CtxAccumulatedBody, accumulatedBody)
	log.Debugf("ai-token-report: accumulated body length=%d", len(accumulatedBody))
	return true
}

func getRequestID() string {
	if value, err := proxywasm.GetProperty([]string{"x_request_id"}); err == nil {
		if requestID := normalizeRequestID(string(value)); requestID != "" {
			return requestID
		}
	}

	if value, err := proxywasm.GetHttpRequestHeader("x-request-id"); err == nil {
		return normalizeRequestID(value)
	}

	return ""
}

func normalizeRequestID(value string) string {
	requestID := strings.TrimSpace(value)
	if requestID == "" || requestID == "-" {
		return ""
	}
	return requestID
}

func onHttpStreamingBody(ctx wrapper.HttpContext, config TokenUsageReportConfig, data []byte, endOfStream bool) []byte {
	// 检查是否设置了跳过标记
	if ctx.GetContext(CtxSkipPlugin) != nil {
		return data
	}

	// Check if plugin is enabled
	if config.reportApiUrl == "" {
		return data
	}

	// Set first token time if not already set
	if ctx.GetContext(CtxFirstToken) == nil {
		ctx.SetContext(CtxFirstToken, time.Now().UnixMilli())
	}

	// Extract token usage from response body (only if not disabled)
	if !config.disableOpenaiUsage {
		usage := tokenusage.GetTokenUsage(ctx, data)
		if usage.TotalToken > 0 {
			setUsageToContext(ctx, usage)
			log.Debugf("ai-token-report: extracted usage via GetTokenUsage: total=%d", usage.TotalToken)
			ctx.SetContext(CtxAccumulatedBody, nil) // Clean up
		} else if len(data) > 0 && shouldAccumulateChunk(data) {
			accumulateChunk(ctx, data)
		}
	}

	// Report token usage at the end of stream
	if endOfStream {
		// Calculate request duration
		requestStartTime, ok := ctx.GetContext(CtxRequestStart).(int64)
		if !ok {
			log.Warn("ai-token-report: request start time not found in context (streaming)")
			requestStartTime = time.Now().UnixMilli()
		}
		duration := time.Now().UnixMilli() - requestStartTime
		ctx.SetContext(LLMServiceDuration, duration)

		// Try parsing from accumulated body if no TotalToken yet
		if !config.disableOpenaiUsage {
			totalTokenVal := ctx.GetContext(CtxKeyTotalToken)
			totalToken, _ := convertToUInt(totalTokenVal)
			if totalToken == 0 {
				accumulatedBody := ctx.GetByteSliceContext(CtxAccumulatedBody, []byte{})
				if len(accumulatedBody) > 0 {
					log.Debugf("ai-token-report: trying to parse usage from accumulated body, length=%d", len(accumulatedBody))
					usage := parseUsageDirectly(accumulatedBody)
					if usage.TotalToken > 0 {
						setUsageToContext(ctx, usage)
						log.Infof("ai-token-report: successfully parsed usage from accumulated body: total=%d, input=%d, output=%d",
							usage.TotalToken, usage.InputToken, usage.OutputToken)
					} else {
						log.Warnf("ai-token-report: failed to parse usage from accumulated body")
					}
				}
			}
		}

        log.Debugf("onHttpStreamingBody bodyLen=%d", len(data))

		// Report token usage
		reportTokenUsageFromContext(ctx, config, data)

		// Clean up accumulated body
		ctx.SetContext(CtxAccumulatedBody, nil)
	}

	return data
}

func onHttpResponseBody(ctx wrapper.HttpContext, config TokenUsageReportConfig, body []byte) types.Action {
	// 检查是否设置了跳过标记
	if ctx.GetContext(CtxSkipPlugin) != nil {
		return types.ActionContinue
	}

	// Check if plugin is enabled
	if config.reportApiUrl == "" {
		return types.ActionContinue
	}

	// Calculate request duration
	requestStartTime, ok := ctx.GetContext(CtxRequestStart).(int64)
	if !ok {
		log.Warn("ai-token-report: request start time not found in context, using current time")
		requestStartTime = time.Now().UnixMilli()
	}

	responseEndTime := time.Now().UnixMilli()
	duration := responseEndTime - requestStartTime
	ctx.SetContext(LLMServiceDuration, duration)

	// Extract token usage from response body
	if !config.disableOpenaiUsage {
		if usage := tokenusage.GetTokenUsage(ctx, body); usage.TotalToken > 0 {
			// Update model from response body if not already set (or override with response value)
			if usage.Model != "" && usage.Model != tokenusage.ModelUnknown {
				ctx.SetContext(CtxKeyModel, usage.Model)
			}

			// Supplement model from response body via direct gjson if GetTokenUsage missed it
			if usage.Model == "" || usage.Model == tokenusage.ModelUnknown {
				if model := gjson.GetBytes(body, "model"); model.Exists() && model.String() != "" {
					usage.Model = model.String()
					ctx.SetContext(CtxKeyModel, usage.Model)
				}
			}

			// Supplement token details from response body via direct gjson if GetTokenUsage missed them
			if len(usage.InputTokenDetails) == 0 {
				if inputDetails := gjson.GetBytes(body, "usage.prompt_tokens_details"); inputDetails.Exists() && inputDetails.IsObject() {
					for key, value := range inputDetails.Map() {
						usage.InputTokenDetails[key] = value.Int()
					}
				}
			}
			if len(usage.OutputTokenDetails) == 0 {
				if outputDetails := gjson.GetBytes(body, "usage.completion_tokens_details"); outputDetails.Exists() && outputDetails.IsObject() {
					for key, value := range outputDetails.Map() {
						usage.OutputTokenDetails[key] = value.Int()
					}
				}
			}

			ctx.SetContext(CtxKeyInputToken, usage.InputToken)
			ctx.SetContext(CtxKeyOutputToken, usage.OutputToken)
			ctx.SetContext(CtxKeyTotalToken, usage.TotalToken)
			// Store the supplemented tokenUsage JSON string
			usageJSON, err := json.Marshal(usage)
			if err == nil {
				ctx.SetContext(CtxTokenUsage, string(usageJSON))
			}
			log.Infof("ai-token-report: extracted token usage (non-streaming): model=%s, input=%d, output=%d, total=%d, inputDetails=%v, outputDetails=%v",
				usage.Model, usage.InputToken, usage.OutputToken, usage.TotalToken, usage.InputTokenDetails, usage.OutputTokenDetails)
		}
	}

    log.Debugf("onHttpResponseBody bodyLen=%d", len(body))

	// Report token usage
	reportTokenUsageFromContext(ctx, config, body)

	return types.ActionContinue
}

func generateRandomString(length int) string {
	if length > 32 {
		length = 32
	}
	if length <= 0 {
		length = 8
	}

	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, length)

	for i := range result {
		randomIndex, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		result[i] = charset[randomIndex.Int64()]
	}

	return string(result)
}

func convertToUInt(val interface{}) (uint64, bool) {
	switch v := val.(type) {
	case float32:
		return uint64(v), true
	case float64:
		return uint64(v), true
	case int32:
		return uint64(v), true
	case int64:
		return uint64(v), true
	case uint32:
		return uint64(v), true
	case uint64:
		return v, true
	default:
		return 0, false
	}
}

func reportTokenUsage(config TokenUsageReportConfig, ctx wrapper.HttpContext, model string, inputTokens, outputTokens,
	totalTokens uint64, duration int64, startTime, endTime int64, tokenUsage string) {
	// Add panic recovery to ensure reporting never crashes the main flow
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("Panic in reportTokenUsage (recovered): %v", r)
		}
	}()

	// Skip if report API client is not configured
	if config.reportClient == nil {
		log.Debugf("reportClient not configured, skipping token usage report")
		return
	}

	// Get userKey from X-Original-Api-Key header
	userKey, err := proxywasm.GetHttpRequestHeader(OriginalAPIKey)
	if err != nil || userKey == "" {
		log.Debugf("X-Original-Api-Key header not found, skipping token usage report")
		return
	}

	// Get Envoy request ID from x-request-id header
	envoyRequestID := ""
	if requestIDHeader, err := proxywasm.GetHttpRequestHeader("x-request-id"); err == nil {
		envoyRequestID = requestIDHeader
		if envoyRequestID != "" && envoyRequestID != "-" {
			log.Infof("Using Envoy request ID from x-request-id header: %s", envoyRequestID)
		} else {
			log.Debugf("x-request-id header is empty or '-', will generate one")
			envoyRequestID = ""
		}
	} else {
		log.Debugf("Failed to get x-request-id header: %v", err)
	}

	timestamp := time.Now().UnixMilli()
	var requestID string
	upstreamRequestID := getRequestID()
	if upstreamRequestID != "" {
		requestID = upstreamRequestID
		log.Debugf("reportTokenUsage using upstream request ID: %s", upstreamRequestID)
	} else {
		randomSuffix := generateRandomString(8)
		requestID = fmt.Sprintf("%s%d", randomSuffix, timestamp)
		log.Debugf("reportTokenUsage using generated ID: %s", requestID)
	}

	// Build request body
	reportReq := TokenUsageReportRequest{
		RequestId:   requestID,
		Model:       model,
		UserKey:     userKey,
		InputToken:  int64(inputTokens),
		OutputToken: int64(outputTokens),
		TotalToken:  int64(totalTokens),
		Duration:    duration,
		Timestamp:   timestamp,
		StartTime:   startTime,
		EndTime:     endTime,
		TokenUsage:  tokenUsage,
	}

	// Marshal request body
	jsonData, err := json.Marshal(reportReq)
	if err != nil {
		log.Errorf("failed to marshal token usage report request: %v", err)
		return
	}

	// Parse URL to get path
	parsedUrl, _ := url.Parse(config.reportApiUrl)
	requestPath := parsedUrl.Path
	if requestPath == "" {
		requestPath = "/"
	}

	// Add query parameters if present
	if parsedUrl.RawQuery != "" {
		requestPath = requestPath + "?" + parsedUrl.RawQuery
	}

	// Call the reporting API asynchronously
	log.Infof("Reporting token usage to %s: requestId=%s, model=%s, userKey=%s, inputToken=%d, outputToken=%d, totalToken=%d, duration=%d",
		config.reportApiUrl, requestID, model, userKey, inputTokens, outputTokens, totalTokens, duration)

	// Use a simple callback that doesn't capture large variables
	err = config.reportClient.Post(
		requestPath,
		[][2]string{
			{"Content-Type", "application/json"},
		},
		jsonData,
		func(statusCode int, responseHeaders http.Header, responseBody []byte) {
			log.Debugf("Token usage report callback: status=%d", statusCode)
		},
		uint32(config.reportTimeout),
	)

	if err != nil {
		log.Errorf("Failed to dispatch token usage report call: %v", err)
		return
	}
}

func reportTokenUsageFromContext(ctx wrapper.HttpContext, config TokenUsageReportConfig, responseBody []byte) {
	// Add panic recovery to ensure this function never interrupts the main flow
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("Panic in reportTokenUsageFromContext (recovered): %v", r)
		}
	}()

	// Skip if disableOpenaiUsage is enabled
	if config.disableOpenaiUsage {
		log.Debugf("disableOpenaiUsage is enabled, skipping token usage report")
		return
	}

	// Skip if report API client is not configured
	if config.reportClient == nil {
		return
	}

	// Get model from context attributes
	model, ok := ctx.GetContext(CtxKeyModel).(string)
	if !ok || model == "" {
		log.Debugf("Model not found in context, skipping token usage report")
		return
	}

	// Get token values from context
	inputTokenVal := ctx.GetContext(CtxKeyInputToken)
	outputTokenVal := ctx.GetContext(CtxKeyOutputToken)
	totalTokenVal := ctx.GetContext(CtxKeyTotalToken)

	// Convert token values to uint64, allowing input/output to be missing (default to 0)
	// but totalTokens must exist
	inputTokens, _ := convertToUInt(inputTokenVal)
	outputTokens, _ := convertToUInt(outputTokenVal)

	totalTokens, ok := convertToUInt(totalTokenVal)
	if !ok {
		// Some models (e.g., image generation) don't return token data in the response.
		// Check if the response has a "data" field, which indicates a valid non-text response.
		// For streaming, also check the accumulated body since responseBody may only be the last chunk.
		log.Debugf("ai-token-report: totalTokenVal is nil/not-set, responseBody len=%d, checking for data field", len(responseBody))
		hasData := false
		if len(responseBody) > 0 {
			hasData = gjson.GetBytes(responseBody, "data").Exists()
			log.Debugf("ai-token-report: responseBody hasData=%v, sample: %s", hasData, truncateSample(string(responseBody), 200))
		}
		if !hasData {
			if accBody := ctx.GetByteSliceContext(CtxAccumulatedBody, []byte{}); len(accBody) > 0 {
				hasData = gjson.GetBytes(accBody, "data").Exists()
				log.Debugf("ai-token-report: accBody len=%d, hasData=%v, sample: %s", len(accBody), hasData, truncateSample(string(accBody), 200))
			}
		}
		if hasData {
			log.Infof("ai-token-report: no token data but response contains data field, reporting with 0 tokens, model=%s", model)
			totalTokens = 0
		} else {
			log.Debugf("ai-token-report: no token data and no data field in response, result not ready yet, model=%s", model)
			return
		}
	}

	// Get duration from context
	durationVal := ctx.GetContext(LLMServiceDuration)
	duration, ok := convertToUInt(durationVal)
	if !ok {
		log.Debugf("LLMServiceDuration not found, skipping token usage report")
		return
	}

	// Get start time from context
	startTimeVal := ctx.GetContext(CtxRequestStart)
	startTime, ok := convertToUInt(startTimeVal)
	if !ok {
		startTime = 0
	}

	// Get end time (current time)
	endTime := time.Now().UnixMilli()

	// Get tokenUsage JSON from context
	tokenUsage := ""
	if tokenUsageVal := ctx.GetContext(CtxTokenUsage); tokenUsageVal != nil {
		if tokenUsageStr, ok := tokenUsageVal.(string); ok {
			// 将驼峰命名转换为下划线命名
			tokenUsage = convertCamelToSnakeJSON(tokenUsageStr)
		}
	}

	// Call the report function (in a goroutine-like manner to avoid blocking)
	reportTokenUsage(config, ctx, model, inputTokens, outputTokens, totalTokens, int64(duration), int64(startTime), endTime, tokenUsage)

	// Clean up context to prevent memory leaks
	ctx.SetContext(CtxRequestStart, nil)
	ctx.SetContext(CtxKeyModel, nil)
	ctx.SetContext(CtxKeyInputToken, nil)
	ctx.SetContext(CtxKeyOutputToken, nil)
	ctx.SetContext(CtxKeyTotalToken, nil)
	ctx.SetContext(CtxFirstToken, nil)
	ctx.SetContext(CtxTokenUsage, nil)
	ctx.SetContext(CtxAccumulatedBody, nil) // Also clean up accumulated body
	ctx.SetContext(LLMServiceDuration, nil)
}

// convertCamelToSnakeJSON 将 JSON 字符串中第一层的驼峰命名转换为下划线命名
// 例如：{"InputToken":123,"Inner":{"Key":"val"}} -> {"input_token":123,"Inner":{"Key":"val"}}
func convertCamelToSnakeJSON(jsonStr string) string {
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		// 解析失败，返回原字符串
		return jsonStr
	}

	// 只转换第一层的 key
	result := make(map[string]interface{})
	for key, value := range data {
		newKey := camelToSnake(key)
		result[newKey] = value
	}

	output, _ := json.Marshal(result)
	return string(output)
}

// camelToSnake 将驼峰命名转换为下划线命名
func camelToSnake(s string) string {
	var result []rune
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result = append(result, '_')
		}
		result = append(result, r)
	}
	return strings.ToLower(string(result))
}

// parseUsageDirectly directly parses usage from response body using gjson
// This is a fallback when tokenusage.GetTokenUsage fails to extract usage
func parseUsageDirectly(data []byte) tokenusage.TokenUsage {
	usage := tokenusage.TokenUsage{}

	// Add debug logging to see what we're working with
	dataStr := string(data)

	// Check if data starts with "data:" (SSE format)
	if strings.HasPrefix(dataStr, "data:") {
		log.Debugf("ai-token-report: data appears to be in SSE format")
		// Remove "data:" prefix and parse the JSON
		jsonStart := 5 // Skip "data:"
		// Skip whitespace
		for jsonStart < len(dataStr) && (dataStr[jsonStart] == ' ' || dataStr[jsonStart] == '\t') {
			jsonStart++
		}
		trimmedData := dataStr[jsonStart:]
		parsed := gjson.Parse(trimmedData)

		// Try to get usage from the SSE JSON
		if totalTokens := parsed.Get("usage.total_tokens"); totalTokens.Exists() {
			usage.TotalToken = totalTokens.Int()
			usage.InputToken = parsed.Get("usage.prompt_tokens").Int()
			usage.OutputToken = parsed.Get("usage.completion_tokens").Int()
			log.Infof("ai-token-report: found usage in SSE format: total=%d, input=%d, output=%d",
				usage.TotalToken, usage.InputToken, usage.OutputToken)
			return usage
		}
	}

	// Try to parse usage from the data using gjson
	parsed := gjson.ParseBytes(data)

	// Try OpenAI format first
	if totalTokens := parsed.Get("usage.total_tokens"); totalTokens.Exists() {
		usage.TotalToken = totalTokens.Int()
		usage.InputToken = parsed.Get("usage.prompt_tokens").Int()
		usage.OutputToken = parsed.Get("usage.completion_tokens").Int()
		log.Debugf("ai-token-report: parsed usage in OpenAI format: total=%d, input=%d, output=%d",
			usage.TotalToken, usage.InputToken, usage.OutputToken)
		return usage
	}

	// Try Gemini format
	if totalTokens := parsed.Get("usageMetadata.totalTokenCount"); totalTokens.Exists() {
		usage.TotalToken = totalTokens.Int()
		usage.InputToken = parsed.Get("usageMetadata.promptTokenCount").Int()
		usage.OutputToken = parsed.Get("usageMetadata.candidatesTokenCount").Int()
		log.Debugf("ai-token-report: parsed usage in Gemini format: total=%d, input=%d, output=%d",
			usage.TotalToken, usage.InputToken, usage.OutputToken)
		return usage
	}

	// Log a sample of the data for debugging
	sampleSize := 200
	if len(dataStr) > sampleSize {
		log.Debugf("ai-token-report: data sample (first %d chars): %s", sampleSize, dataStr[:sampleSize])
	} else {
		log.Debugf("ai-token-report: data sample: %s", dataStr)
	}

	// Try searching for usage in nested structures (for SSE format)
	// Look for any object containing usage field
	parsed.ForEach(func(key, value gjson.Result) bool {
		if value.IsObject() {
			// Check if this object has usage field
			if usageObj := value.Get("usage"); usageObj.Exists() && usageObj.IsObject() {
				if totalTokens := usageObj.Get("total_tokens"); totalTokens.Exists() {
					usage.TotalToken = totalTokens.Int()
					usage.InputToken = usageObj.Get("prompt_tokens").Int()
					usage.OutputToken = usageObj.Get("completion_tokens").Int()
					log.Infof("ai-token-report: found usage in nested structure: total=%d, input=%d, output=%d",
						usage.TotalToken, usage.InputToken, usage.OutputToken)
					return false // stop iteration
				}
			}
		}
		return true // continue iteration
	})

	if usage.TotalToken > 0 {
		return usage
	}

	// If still not found, try to extract usage using regex-like search for usage patterns
	// Look for usage patterns in the data: "usage":{"prompt_tokens":X,"completion_tokens":Y,"total_tokens":Z}
	// This is a simplified search for common usage patterns
	usageIdx := strings.Index(dataStr, `"usage":`)
	if usageIdx != -1 {
		// Find the opening brace after "usage:":
		braceIdx := strings.Index(dataStr[usageIdx:], "{")
		if braceIdx != -1 {
			// Try to extract the usage object
			usageStart := usageIdx + braceIdx
			// Find matching closing brace by counting open/close (handles nested objects)
			usageEnd := findMatchingBrace(dataStr, usageStart)
			if usageEnd != -1 {
				usageJSON := dataStr[usageStart : usageEnd+1]
				usageParsed := gjson.Parse(usageJSON)
				if totalTokens := usageParsed.Get("total_tokens"); totalTokens.Exists() {
					usage.TotalToken = totalTokens.Int()
					usage.InputToken = usageParsed.Get("prompt_tokens").Int()
					usage.OutputToken = usageParsed.Get("completion_tokens").Int()
					log.Infof("ai-token-report: found usage via string search: total=%d, input=%d, output=%d",
						usage.TotalToken, usage.InputToken, usage.OutputToken)
					return usage
				}
			}
		}
	}

	log.Debugf("ai-token-report: no usage found in data, length=%d", len(data))
	return usage
}

// findMatchingBrace finds the matching closing brace for the opening brace at startIdx.
// Uses brace counting to correctly handle nested objects.
func findMatchingBrace(s string, startIdx int) int {
	braceCount := 0
	for i := startIdx; i < len(s); i++ {
		switch s[i] {
		case '{':
			braceCount++
		case '}':
			braceCount--
			if braceCount == 0 {
				return i
			}
		}
	}
	return -1
}

// truncateSample truncates a string to maxLen characters for debug logging.
func truncateSample(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
