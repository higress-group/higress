package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

const (
	defaultAPITimeout int32  = 5000
	OriginalAPIKey    string = "X-Original-Api-Key"
)

const (
	CtxKeyBalanceChecked = "balance_checked"
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
	checkClient           wrapper.HttpClient
	apiUrl                string
	serviceName           string
	apiTimeout            int32
	failMessage           string
	failCode              int32
	teamEndpoint          *url.URL
	teamRouteMode         string
	teamSettlement        string
	teamVersionGuardOnly  bool
	teamPaidPaths         map[string]bool
	unlistedConfig        *BalanceCheckConfig
	teamFallback          string
	teamSigningKey        string
	teamImageBillingModel string
}

func main() {}

func init() {
	wrapper.SetCtx(
		"ai-balance-check",
		wrapper.ParseConfig(parseConfig),
		wrapper.ProcessRequestHeaders(onHttpRequestHeaders),
	)
}

func parseConfig(json gjson.Result, config *BalanceCheckConfig) error {
	config.teamVersionGuardOnly = json.Get("teamVersionGuardOnly").Bool()
	if config.teamVersionGuardOnly {
		return nil
	}
	if paths := json.Get("teamPaidPaths"); paths.Exists() {
		if !paths.IsArray() || len(paths.Array()) == 0 {
			return fmt.Errorf("teamPaidPaths requires exact paid paths")
		}
		config.teamPaidPaths = make(map[string]bool)
		for _, item := range paths.Array() {
			path := item.String()
			if !strings.HasPrefix(path, "/") {
				return fmt.Errorf("invalid paid path")
			}
			config.teamPaidPaths[path] = true
		}
	}
	if previous := json.Get("teamUnlistedPathConfig"); previous.Exists() {
		if config.teamPaidPaths == nil || !previous.IsObject() || previous.Get("teamPaidPaths").Exists() || previous.Get("teamUnlistedPathConfig").Exists() {
			return fmt.Errorf("teamUnlistedPathConfig must be a flat original policy alongside teamPaidPaths")
		}
		config.unlistedConfig = new(BalanceCheckConfig)
		if err := parseConfig(previous, config.unlistedConfig); err != nil {
			return fmt.Errorf("teamUnlistedPathConfig: %w", err)
		}
	}

	teamBilling := json.Get("teamBilling").Bool()
	config.teamRouteMode = json.Get("teamRouteMode").String()
	config.teamSettlement = json.Get("teamSettlement").String()
	config.teamFallback = json.Get("teamFallback").String()
	if config.teamFallback != "" {
		if (config.teamFallback != "source" && config.teamFallback != "target") || config.teamRouteMode == "" || config.teamSettlement != "usage" {
			return fmt.Errorf("native team fallback requires a migrated usage source or target")
		}
	}
	if signedOperation(config.teamSettlement) || config.teamFallback != "" {
		config.teamSigningKey = os.Getenv("WUJIE_TEAM_SIGNING_KEY")
		if config.teamSigningKey == "" {
			config.teamSigningKey = json.Get("teamSigningKey").String()
		}
		if len(config.teamSigningKey) < 32 {
			return fmt.Errorf("signed operations require a team signing key of at least 32 bytes")
		}
	}
	switch config.teamRouteMode {
	case "":
		if config.teamSettlement != "" {
			return fmt.Errorf("teamSettlement requires a migrated team route")
		}
	case "strict", "compat":
		if !teamBilling {
			return fmt.Errorf("teamRouteMode requires teamBilling")
		}
		switch config.teamSettlement {
		case "usage", "forward", "unavailable":
		case "image":
			config.teamImageBillingModel = json.Get("teamImageBillingModel").String()
			if config.teamImageBillingModel == "" {
				return fmt.Errorf("image settlement requires the route's price catalog model")
			}
		case "search":
			config.teamImageBillingModel = "deepseek-search"
		case "task_create", "task_create_sync", "mirror_create":
		default:
			return fmt.Errorf("migrated team routes require explicit teamSettlement")
		}
	default:
		return fmt.Errorf("invalid teamRouteMode")
	}
	config.apiUrl = json.Get("apiUrl").String()
	if config.apiUrl == "" {
		if teamBilling {
			return fmt.Errorf("the legacy apiUrl is required when team billing is enabled")
		}
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

	if teamBilling {
		endpoint, err := url.Parse(json.Get("teamApiUrl").String())
		if err != nil || endpoint.Host != parsedUrl.Host || endpoint.Scheme != parsedUrl.Scheme || endpoint.Path == "" {
			return fmt.Errorf("teamApiUrl must be an admission URL on the existing balance API origin")
		}
		config.teamEndpoint = endpoint
	}

	// Create HTTP client for balance check API
	config.checkClient = wrapper.NewClusterClient(wrapper.DnsCluster{
		ServiceName: config.serviceName,
		Domain:      actualHost,
		Port:        port,
	})

	log.Infof("ai-balance-check: configured, url=%s, service=%s, timeout=%dms, failMessage=%s",
		config.apiUrl, config.serviceName, config.apiTimeout, config.failMessage)
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

	_ = proxywasm.SendHttpResponse(
		402,
		[][2]string{{"Content-Type", "application/json"}},
		jsonData,
		-1,
	)
	return types.HeaderStopAllIterationAndWatermark
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config BalanceCheckConfig) types.Action {
	version, versionErr := proxywasm.GetHttpRequestHeader("X-Wujie-Billing-Version")
	if versionErr != nil && versionErr != types.ErrorStatusNotFound {
		return teamAdmissionError(http.StatusServiceUnavailable)
	}
	marked := versionErr == nil
	if marked {
		if version != "2" {
			return teamAdmissionError(http.StatusBadRequest)
		}
		if config.teamRouteMode == "" {
			return teamAdmissionError(http.StatusServiceUnavailable)
		}
	}
	if config.teamVersionGuardOnly {
		return types.ActionContinue
	}
	if config.teamPaidPaths != nil {
		raw, err := proxywasm.GetHttpRequestHeader(":path")
		if err != nil || raw == "" {
			return teamAdmissionError(http.StatusServiceUnavailable)
		}
		path, _, _ := strings.Cut(raw, "?")
		if !config.teamPaidPaths[path] {
			if marked || config.teamRouteMode == "strict" {
				return teamAdmissionError(http.StatusServiceUnavailable)
			}
			if config.unlistedConfig != nil {
				config = *config.unlistedConfig
			} else {
				// Older prefix configs contain the legacy balance URL but no policy snapshot.
				config.teamEndpoint = nil
			}
			if _, err := proxywasm.GetHttpRequestHeader(teamUUIDHeader); err != types.ErrorStatusNotFound && (err != nil || config.teamEndpoint == nil) {
				return teamAdmissionError(http.StatusServiceUnavailable)
			}
			if proxywasm.SetProperty([]string{"wasm.teamBillingUnlistedPath"}, []byte("1")) != nil {
				return teamAdmissionError(http.StatusServiceUnavailable)
			}
		}
	}
	if config.apiUrl == "" {
		log.Debugf("ai-balance-check: plugin disabled (no apiUrl)")
		return types.ActionContinue
	}

	// Check if already validated to avoid duplicate checks
	if ctx.GetContext(CtxKeyBalanceChecked) != nil {
		return types.ActionContinue
	}

	if config.teamEndpoint != nil {
		if config.teamSettlement == "unavailable" {
			return teamAdmissionError(http.StatusServiceUnavailable)
		}
		for _, header := range []string{admittedTeamHeader, admittedTeamUUIDHeader, signatureHeader, timestampHeader} {
			if err := proxywasm.RemoveHttpRequestHeader(header); err != nil && err != types.ErrorStatusNotFound {
				return teamAdmissionError(http.StatusServiceUnavailable)
			}
		}
		proof, _ := proxywasm.GetHttpRequestHeader(fallbackProofHeader)
		if err := proxywasm.RemoveHttpRequestHeader(fallbackProofHeader); err != nil && err != types.ErrorStatusNotFound {
			return teamAdmissionError(http.StatusServiceUnavailable)
		}
		redirectOrigin, _ := proxywasm.GetHttpRequestHeader("x-higress-fallback-from")
		if config.teamRouteMode != "" && config.teamFallback != "target" && (proof != "" || redirectOrigin != "") {
			return teamAdmissionError(http.StatusServiceUnavailable)
		}
		if config.teamFallback == "target" {
			team, _ := proxywasm.GetHttpRequestHeader(teamUUIDHeader)
			key, _ := proxywasm.GetHttpRequestHeader(OriginalAPIKey)
			requestID, _ := proxywasm.GetHttpRequestHeader("request-id")
			model, _ := proxywasm.GetHttpRequestHeader("x-higress-llm-model")
			if !validFallbackProof(config.teamSigningKey, requestID, team, key, model, proof) {
				return teamAdmissionError(http.StatusServiceUnavailable)
			}
			return checkTeamAdmission(ctx, config, &team)
		}
		teamUUID, err := proxywasm.GetHttpRequestHeader(teamUUIDHeader)
		switch err {
		case nil:
			return checkTeamAdmission(ctx, config, &teamUUID)
		case types.ErrorStatusNotFound:
			switch config.teamRouteMode {
			case "strict":
				return teamAdmissionError(http.StatusBadRequest)
			case "compat":
				if marked {
					return teamAdmissionError(http.StatusBadRequest)
				}
				return checkTeamAdmission(ctx, config, nil)
			}
		default:
			return teamAdmissionError(http.StatusServiceUnavailable)
		}
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

	// Mark as checked to avoid duplicate validation
	ctx.SetContext(CtxKeyBalanceChecked, true)

	// Perform balance check
	return checkBalance(ctx, config, apiKey)
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
		return types.ActionContinue
	}

	// Pause the request until callback completes
	return types.ActionPause
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
		_ = proxywasm.SendHttpResponse(
			402,
			[][2]string{{"Content-Type", "application/json"}},
			jsonData,
			-1,
		)
		return
	}

	// Balance check passed, resume the request
	log.Infof("ai-balance-check: balance check passed")
	_ = proxywasm.ResumeHttpRequest()
}
