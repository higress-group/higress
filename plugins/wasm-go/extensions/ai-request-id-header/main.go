package main

import (
	"strings"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

const (
	responseRequestIDHeader = "request-id"
	upstreamRequestIDHeader = "request-id"
	requestIDPropertyKey    = "wasm.requestId"
	providerTypePropertyKey = "wasm.providerType"
	clusterNameProperty     = "cluster_name"
)

// defaultSkipClusterNamePatterns 默认跳过注入的 cluster_name 子串。
// 集群命名约定中 llm- 前缀用于 AI 提供商转发（如 llm-RightCodes.internal.dns），
// 这些上游不应收到 request-id header。
var defaultSkipClusterNamePatterns = []string{"llm-"}

type RequestIDHeaderConfig struct {
	SkipClusterNamePatterns []string
}

func main() {}

func init() {
	wrapper.SetCtx(
		"ai-request-id-header",
		wrapper.ParseConfig(parseConfig),
		wrapper.ProcessRequestHeaders(onHttpRequestHeaders),
		wrapper.ProcessResponseHeaders(onHttpResponseHeaders),
	)
}

func parseConfig(json gjson.Result, config *RequestIDHeaderConfig) error {
	config.SkipClusterNamePatterns = defaultSkipClusterNamePatterns
	if patterns := json.Get("skipClusterNamePatterns"); patterns.IsArray() {
		var got []string
		patterns.ForEach(func(_, v gjson.Result) bool {
			if s := strings.TrimSpace(v.String()); s != "" {
				got = append(got, s)
			}
			return true
		})
		if len(got) > 0 {
			config.SkipClusterNamePatterns = got
		}
	}
	return nil
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config RequestIDHeaderConfig) types.Action {
	requestID := getRequestID()
	if requestID != "" {
		_ = proxywasm.SetProperty([]string{requestIDPropertyKey}, []byte(requestID))
	}

	if shouldSkipForAIProvider(config) {
		// Only mutate if a non-empty request-id header is actually present; otherwise
		// there is nothing to remove and triggering a header mutation would unnecessarily
		// clear the route cache that ai-proxy already locked via DisableReroute.
		existing, err := proxywasm.GetHttpRequestHeader(upstreamRequestIDHeader)
		if err == nil && normalizeRequestID(existing) != "" {
			ctx.DisableReroute()
			if err := proxywasm.RemoveHttpRequestHeader(upstreamRequestIDHeader); err != nil {
				log.Warnf("ai-request-id-header: failed to remove upstream request header: %v", err)
			}
		}
		return types.ActionContinue
	}

	if requestID == "" {
		return types.ActionContinue
	}

	ctx.DisableReroute()
	if err := proxywasm.ReplaceHttpRequestHeader(upstreamRequestIDHeader, requestID); err != nil {
		log.Warnf("ai-request-id-header: failed to set upstream request header: %v", err)
	}

	return types.ActionContinue
}

// shouldSkipForAIProvider 判断当前请求是否转发给 AI 提供商，是则跳过 request-id 注入。
//
// 主信号：ai-proxy 在请求阶段写入 wasm.providerType 属性（需保证本插件执行优先级低于 ai-proxy，
// 与 ai-token-report 读取该属性的方式一致）。
// 兜底信号：cluster_name 命中配置的子串（覆盖绕开 ai-proxy 直连 AI 厂商的路由，如 right-codes.dns）。
func shouldSkipForAIProvider(config RequestIDHeaderConfig) bool {
	if v, err := proxywasm.GetProperty([]string{providerTypePropertyKey}); err == nil && len(v) > 0 {
		log.Debugf("ai-request-id-header: skip injection, providerType=%s", string(v))
		return true
	}

	if len(config.SkipClusterNamePatterns) == 0 {
		return false
	}

	clusterName, err := proxywasm.GetProperty([]string{clusterNameProperty})
	if err != nil || len(clusterName) == 0 {
		return false
	}

	for _, pattern := range config.SkipClusterNamePatterns {
		if pattern == "" {
			continue
		}
		if strings.Contains(string(clusterName), pattern) {
			log.Debugf("ai-request-id-header: skip injection, cluster_name=%s matches pattern=%s", string(clusterName), pattern)
			return true
		}
	}
	return false
}

func onHttpResponseHeaders(_ wrapper.HttpContext, _ RequestIDHeaderConfig) types.Action {
	requestID := getRequestID()
	if requestID == "" {
		return types.ActionContinue
	}

	if err := proxywasm.ReplaceHttpResponseHeader(responseRequestIDHeader, requestID); err != nil {
		log.Warnf("ai-request-id-header: failed to set response header: %v", err)
	}

	return types.ActionContinue
}

func getRequestID() string {
	if value, err := proxywasm.GetHttpRequestHeader(upstreamRequestIDHeader); err == nil {
		if requestID := normalizeRequestID(value); requestID != "" {
			return requestID
		}
	}

	if value, err := proxywasm.GetProperty([]string{requestIDPropertyKey}); err == nil {
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
		if requestID := normalizeRequestID(value); requestID != "" {
			return requestID
		}
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
