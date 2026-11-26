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
	defaultReportTimeout int32 = 5000       // 5 seconds
	maxAccumulatedSize   int   = 100 * 1024 // 100KB

	// Context keys
	CtxKeyModel        = "token_model"
	CtxKeyInputToken   = "token_input"
	CtxKeyOutputToken  = "token_output"
	CtxKeyTotalToken   = "token_total"
	CtxRequestStart    = "request_start_time"
	CtxFirstToken      = "first_token_time"
	CtxTokenUsage      = "token_usage_raw"
	CtxAccumulatedBody = "accumulated_body"
	CtxSkipPlugin      = "skip_token_report"
	CtxStatusCode      = "response_status_code"
	CtxProductSource   = "token_report_product_source"
	providerTypeKey    = "providerType"

	// HTTP headers
	OriginalAPIKey = "X-Original-Api-Key"
	teamUUIDHeader = "X-Wujie-Team-Id"

	// WujieTaskID is the business task id header (OPE-8474): forwarded by wujie backend,
	// a plain-numeric task id (e.g. "999"). Deliberately NOT x-request-id, which Envoy overwrites.
	WujieTaskID = "wujie_task_id"

	// Metrics
	LLMServiceDuration = "llm_service_duration"

	// Default disableOpenaiUsage value
	defaultDisableOpenaiUsage = false
)

// TokenUsageReportRequest represents the request body for token usage reporting
type TokenUsageReportRequest struct {
	ProductSource string `json:"productSource"`
	RequestId     string `json:"requestId"`
	Model         string `json:"model"`
	UserKey       string `json:"userKey"`
	InputToken    int64  `json:"inputToken"`
	OutputToken   int64  `json:"outputToken"`
	TotalToken    int64  `json:"totalToken"`
	Duration      int64  `json:"duration"`
	Timestamp     int64  `json:"timestamp"`
	StartTime     int64  `json:"startTime"`
	EndTime       int64  `json:"endTime"`
	TokenUsage    string `json:"tokenUsage"`
	// Wujie business task id (OPE-8474), plain numeric string read from wujie_task_id header.
	// omitempty: header absent/empty -> field not sent (Onepiece falls back to requestId dedup).
	TaskId           string `json:"taskId,omitempty"`
	TeamID           int64  `json:"teamId,omitempty"`
	PersonalOwnerUID int64  `json:"personalOwnerUid,omitempty"`
}

// TokenUsageReportConfig holds the configuration for token usage reporting
type TokenUsageReportConfig struct {
	reportClient       wrapper.HttpClient
	reportApiUrl       string
	reportServiceName  string
	reportTimeout      int32
	disableOpenaiUsage bool
	teamReportCluster  *wrapper.FQDNCluster
	requireTeamID      bool
	unlistedConfig     *TokenUsageReportConfig
	source             sourceConfig
}

func main() {}

func init() {
	wrapper.SetCtx(
		"ai-token-report",
		wrapper.ParseConfig(parseConfig),
		wrapper.ProcessRequestHeaders(onHttpRequestHeaders),
		wrapper.ProcessRequestBody(onHttpRequestBody),
		wrapper.ProcessResponseHeaders(onHttpResponseHeaders),
		wrapper.ProcessStreamingResponseBody(onHttpStreamingBody),
		wrapper.ProcessResponseBody(onHttpResponseBody),
	)
}

func parseConfig(json gjson.Result, config *TokenUsageReportConfig) error {
	if previous := json.Get("teamUnlistedPathConfig"); previous.Exists() {
		if !previous.IsObject() || previous.Get("teamUnlistedPathConfig").Exists() {
			return fmt.Errorf("teamUnlistedPathConfig must be a flat original policy")
		}
		config.unlistedConfig = new(TokenUsageReportConfig)
		if err := parseConfig(previous, config.unlistedConfig); err != nil {
			return fmt.Errorf("teamUnlistedPathConfig: %w", err)
		}
	}
	// Parse reportApiUrl (required)
	config.reportApiUrl = json.Get("reportApiUrl").String()
	config.requireTeamID = json.Get("requireTeamId").Bool()
	if service := json.Get("teamReportService").String(); service != "" {
		if !strings.HasSuffix(service, ".dns") {
			return fmt.Errorf("teamReportService must be a Higress DNS upstream name ending in .dns")
		}
		config.teamReportCluster = &wrapper.FQDNCluster{FQDN: service, Port: 8080}
	}
	if config.requireTeamID && (config.reportApiUrl == "" || config.teamReportCluster == nil) {
		return fmt.Errorf("reportApiUrl and teamReportService are required on team billing routes")
	}
	if config.reportApiUrl == "" {
		log.Infof("ai-token-report: reportApiUrl not configured, plugin will be disabled")
		return nil
	}
	var err error
	config.source, err = parseSourceConfig(json.Get("source"))
	if err != nil {
		return fmt.Errorf("source: %w", err)
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

// Use the balance filter's stream decision even after an upstream rewrites the path.
func (cfg TokenUsageReportConfig) forStream(ctx wrapper.HttpContext) TokenUsageReportConfig {
	if selected, ok := ctx.GetContext("unlistedReportConfig").(*TokenUsageReportConfig); ok {
		return *selected
	}
	return cfg
}

func rejectTeamUsage(status int) types.Action {
	body := `{"error":{"type":"team_billing_unavailable","code":"team_billing_unavailable","message":"Team billing unavailable"}}`
	if status == http.StatusBadRequest {
		body = `{"error":{"type":"invalid_team","code":"invalid_team","message":"Unsupported billing version"}}`
	}
	_ = proxywasm.SendHttpResponse(uint32(status), [][2]string{{"Content-Type", "application/json"}}, []byte(body), -1)
	return types.HeaderStopAllIterationAndWatermark
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config TokenUsageReportConfig) types.Action {
	if config.unlistedConfig != nil {
		if value, err := proxywasm.GetProperty([]string{"wasm.teamBillingUnlistedPath"}); err == nil && string(value) == "1" {
			ctx.SetContext("unlistedReportConfig", config.unlistedConfig)
			config = *config.unlistedConfig
		}
	}
	// Check if plugin is enabled
	if config.reportApiUrl == "" {
		return types.ActionContinue
	}
	version, versionErr := proxywasm.GetHttpRequestHeader("X-Wujie-Billing-Version")
	marked := versionErr == nil
	if !marked && versionErr != types.ErrorStatusNotFound {
		return rejectTeamUsage(http.StatusServiceUnavailable)
	}
	if marked && version != "2" {
		return rejectTeamUsage(http.StatusBadRequest)
	}
	// Trusted submission/forward routes settle at execution, including when this plugin is global.
	if settlement, err := proxywasm.GetProperty([]string{"wasm.teamBillingSettlement"}); err == nil && len(settlement) != 0 {
		ctx.SetContext(CtxSkipPlugin, true)
		ctx.DontReadRequestBody()
		ctx.DontReadResponseBody()
		return types.ActionContinue
	}
	// Snapshot attribution before later request rewrites. Async reporting consumes
	// this request context, never the (possibly modified) source header.
	source, _ := proxywasm.GetHttpRequestHeader(config.source.headerName)
	ctx.SetContext(CtxProductSource, config.source.normalize(source))

	// 检查 providerType，如果为空则跳过本插件（非 AI 路由）
	perCallModel, _ := proxywasm.GetProperty([]string{"wasm.teamBillingPerCallModel"})
	providerType := getProviderType()
	if providerType == "" && len(perCallModel) == 0 {
		if marked {
			return rejectTeamUsage(http.StatusServiceUnavailable)
		}
		log.Debugf("ai-token-report: providerType is empty, skipping (not an AI route)")
		ctx.DontReadRequestBody()
		ctx.DontReadResponseBody()
		ctx.SetContext(CtxSkipPlugin, true)
		return types.ActionContinue
	}

	// Freeze the billing decision while admission properties and the original headers are available.
	value, admissionErr := proxywasm.GetProperty([]string{"wasm.teamId"})
	if admissionErr == nil {
		teamID, err := strconv.ParseInt(string(value), 10, 64)
		if err != nil || teamID <= 0 || config.teamReportCluster == nil || (marked && config.disableOpenaiUsage) {
			return rejectTeamUsage(http.StatusServiceUnavailable)
		}
		owner, ownerErr := proxywasm.GetProperty([]string{"wasm.teamPersonalOwnerUid"})
		if ownerErr != types.ErrorStatusNotFound {
			uid, err := strconv.ParseInt(string(owner), 10, 64)
			if marked || ownerErr != nil || err != nil || uid <= 0 {
				return rejectTeamUsage(http.StatusServiceUnavailable)
			}
			ctx.SetContext("billingPersonalOwnerUid", uid)
		}
		ctx.SetContext("billingTeamId", teamID)
	} else {
		_, selectorErr := proxywasm.GetHttpRequestHeader(teamUUIDHeader)
		if admissionErr != types.ErrorStatusNotFound || marked || config.requireTeamID || (config.teamReportCluster != nil && selectorErr != types.ErrorStatusNotFound) {
			return rejectTeamUsage(http.StatusServiceUnavailable)
		}
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
	if len(perCallModel) != 0 {
		requestModel = string(perCallModel)
	}
	ctx.SetContext(CtxKeyModel, requestModel)

	log.Debugf("ai-token-report: processing AI request, providerType=%s, requestModel=%s", providerType, requestModel)

	return types.ActionContinue
}

func onHttpRequestBody(ctx wrapper.HttpContext, config TokenUsageReportConfig, body []byte) types.Action {
	config = config.forStream(ctx)
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

func onHttpResponseHeaders(ctx wrapper.HttpContext, config TokenUsageReportConfig) types.Action {
	// Check if plugin is enabled
	if config.reportApiUrl == "" {
		return types.ActionContinue
	}

	// 检查是否设置了跳过标记
	if ctx.GetContext(CtxSkipPlugin) != nil {
		return types.ActionContinue
	}

	// Capture response status code for later use in response body processing
	statusCode, err := proxywasm.GetHttpResponseHeader(":status")
	if err != nil {
		log.Debugf("ai-token-report: failed to get response status code: %v", err)
	} else {
		ctx.SetContext(CtxStatusCode, statusCode)
		log.Debugf("ai-token-report: captured response status code: %s", statusCode)
		if statusCode != "200" {
			ctx.SetContext(CtxSkipPlugin, true)
			ctx.DontReadResponseBody()
		}
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
	// 	log.Debugf("ai-token-report: accumulated body length=%d", len(accumulatedBody))
	return true
}

func getRequestID() string {
	if value, err := proxywasm.GetHttpRequestHeader("request-id"); err == nil {
		if requestID := normalizeRequestID(value); requestID != "" {
			return requestID
		}
	}

	if value, err := proxywasm.GetProperty([]string{"wasm.requestId"}); err == nil {
		if requestID := normalizeRequestID(string(value)); requestID != "" {
			return requestID
		}
	}

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
	config = config.forStream(ctx)
	// 检查是否设置了跳过标记
	if ctx.GetContext(CtxSkipPlugin) != nil {
		return data
	}

	// Check if plugin is enabled
	if config.reportApiUrl == "" {
		return data
	}

	if sku, err := proxywasm.GetProperty([]string{"wasm.teamBillingPerCallModel"}); err == nil {
		result, _ := ctx.GetContext("teamResultJSON").(*resultJSON)
		if result == nil {
			result = &resultJSON{search: string(sku) == "deepseek-search"}
			ctx.SetContext("teamResultJSON", result)
		}
		result.feed(data)
		if endOfStream {
			ctx.SetContext("teamHasBillableResult", result.billable())
			ctx.SetContext("teamResultJSON", nil)
		}
		ctx.SetContext(CtxKeyModel, string(sku))
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
	config = config.forStream(ctx)
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

	// Dedicated image routes may rewrite the provider model and also use per-call pricing.
	// A successful image result is chargeable even when the provider omits token counts.
	if perCallModel, err := proxywasm.GetProperty([]string{"wasm.teamBillingPerCallModel"}); err == nil {
		result := resultJSON{search: string(perCallModel) == "deepseek-search"}
		result.feed(body)
		if !result.billable() {
			return types.ActionContinue
		}
		ctx.SetContext(CtxKeyModel, string(perCallModel))
		ctx.SetContext("teamHasBillableResult", true)
	}
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

	if sku, err := proxywasm.GetProperty([]string{"wasm.teamBillingPerCallModel"}); err == nil {
		if !ctx.GetBoolContext("teamHasBillableResult", false) {
			return
		}
		model = string(sku)
	}
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
	// Get wujie business task id from request header (OPE-8474).
	// Plain numeric string only; the Onepiece receiver deserializes it into a Long taskId.
	taskId, err := proxywasm.GetHttpRequestHeader(WujieTaskID)
	if err != nil || taskId == "" {
		taskId = ""
	} else {
		log.Debugf("ai-token-report: wujie_task_id header found: %s", taskId)
	}

	// Get Envoy request ID from property
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
	selectedRequestID := getRequestID()
	if selectedRequestID != "" {
		requestID = selectedRequestID
		log.Debugf("reportTokenUsage using request ID: %s", selectedRequestID)
	} else {
		randomSuffix := generateRandomString(8)
		requestID = fmt.Sprintf("%s%d", randomSuffix, timestamp)
		log.Debugf("reportTokenUsage using generated ID: %s", requestID)
	}

	teamID, _ := ctx.GetContext("billingTeamId").(int64)
	personalOwnerUID, _ := ctx.GetContext("billingPersonalOwnerUid").(int64)
	if config.requireTeamID && teamID <= 0 {
		log.Errorf("Missing team admission for usage report, requestId=%s", requestID)
		return
	}

	// Build request body
	reportReq := TokenUsageReportRequest{
		ProductSource:    ctx.GetStringContext(CtxProductSource, config.source.fallbackValue),
		RequestId:        requestID,
		Model:            model,
		UserKey:          userKey,
		InputToken:       int64(inputTokens),
		OutputToken:      int64(outputTokens),
		TotalToken:       int64(totalTokens),
		Duration:         duration,
		Timestamp:        timestamp,
		StartTime:        startTime,
		EndTime:          endTime,
		TokenUsage:       tokenUsage,
		TaskId:           taskId,
		TeamID:           teamID,
		PersonalOwnerUID: personalOwnerUID,
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
	log.Infof("Reporting token usage to %s: requestId=%s, model=%s, inputToken=%d, outputToken=%d, totalToken=%d, duration=%d",
		config.reportApiUrl, requestID, model, inputTokens, outputTokens, totalTokens, duration)

	if teamID > 0 {
		err = dispatchTeamReport(config, requestID, jsonData)
	} else {
		err = config.reportClient.Post(
			requestPath,
			[][2]string{{"Content-Type", "application/json"}},
			jsonData,
			func(statusCode int, _ http.Header, _ []byte) {
				log.Debugf("Token usage report callback: status=%d", statusCode)
			},
			uint32(config.reportTimeout),
		)
	}

	if err != nil {
		log.Errorf("Failed to dispatch token usage report call: %v", err)
		return
	}
}

// isPendingAsyncResponse recognizes the async acknowledgement, including JSON
// strings and data envelopes. Inspect only envelope fields, never generated text.
func isPendingAsyncResponse(body []byte) bool {
	for depth := 0; depth < 8 && gjson.ValidBytes(body); depth++ {
		result := gjson.ParseBytes(body)
		if result.Type == gjson.String {
			body = []byte(result.String())
			continue
		}
		if !result.IsObject() {
			return false
		}
		if result.Get("accepted").Type == gjson.True &&
			result.Get("status").String() == "processing_or_existing" {
			return true
		}
		body = []byte(result.Get("data").Raw)
	}
	return false
}

func dispatchTeamReport(config TokenUsageReportConfig, requestID string, body []byte) error {
	// Dispatch directly to the cluster service and keep the report body out of logs.
	_, err := proxywasm.DispatchHttpCall(config.teamReportCluster.ClusterName(), [][2]string{
		{":method", http.MethodPost},
		{":authority", config.teamReportCluster.HostName()},
		{":path", "/internal/model/usage/team-report"},
		{"Content-Type", "application/json"},
	}, body, nil, uint32(config.reportTimeout), func(_, _, _ int) {
		headers, _ := proxywasm.GetHttpCallResponseHeaders()
		status := http.StatusBadGateway
		for _, h := range headers {
			if h[0] == ":status" {
				status, _ = strconv.Atoi(h[1])
				break
			}
		}
		if status < http.StatusOK || status >= http.StatusMultipleChoices {
			log.Errorf("Team usage report rejected, requestId=%s, status=%d", requestID, status)
		}
	})
	return err
}

func reportTokenUsageFromContext(ctx wrapper.HttpContext, config TokenUsageReportConfig, responseBody []byte) {
	// Add panic recovery to ensure this function never interrupts the main flow
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("Panic in reportTokenUsageFromContext (recovered): %v", r)
		}
	}()

	// Both JSON and SSE use this reporting boundary. Zero tokens may be a
	// successful image, so only the HTTP response status decides eligibility.
	// Re-read the current header: another filter may have changed it since the
	// response-header callback. The saved status remains a fallback.
	statusCode, ok := ctx.GetContext(CtxStatusCode).(string)
	if currentStatus, err := proxywasm.GetHttpResponseHeader(":status"); err == nil {
		statusCode, ok = currentStatus, true
	}
	log.Debugf("response status code is %s", statusCode)
	if !ok || statusCode != "200" {
		if !ok {
			log.Debugf("response status code not found in context, skipping token usage report")
		} else {
			log.Debugf("response status code is %s (not 200), skipping token usage report", statusCode)
		}
		return
	}

	// An accepted async task is not a completed model result, even when an
	// adapter wraps the acknowledgement in data or normalizes HTTP 201 to 200.
	if isPendingAsyncResponse(responseBody) ||
		isPendingAsyncResponse(ctx.GetByteSliceContext(CtxAccumulatedBody, nil)) {
		log.Infof("ai-token-report: async task is processing_or_existing, skipping token usage report")
		return
	}

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
