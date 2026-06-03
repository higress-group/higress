package main

import (
	cfg "github.com/alibaba/higress/plugins/wasm-go/extensions/ai-security-guard/config"
	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-security-guard/lvwang/multi_modal_guard"
	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-security-guard/lvwang/text_moderation_plus"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

func main() {}

func init() {
	wrapper.SetCtx(
		"ai-security-guard",
		wrapper.ParseConfig(parseConfig),
		wrapper.ProcessRequestHeaders(onHttpRequestHeaders),
		wrapper.ProcessRequestBody(onHttpRequestBody),
		wrapper.ProcessResponseHeaders(onHttpResponseHeaders),
		wrapper.ProcessStreamingResponseBody(onHttpStreamingResponseBody),
		wrapper.ProcessResponseBody(onHttpResponseBody),
		wrapper.WithRebuildAfterRequests[cfg.AISecurityConfig](1000),
	)
}

func parseConfig(json gjson.Result, config *cfg.AISecurityConfig) error {
	return config.Parse(json)
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config cfg.AISecurityConfig) types.Action {
	consumer, _ := proxywasm.GetHttpRequestHeader("x-mse-consumer")
	ctx.SetContext("consumer", consumer)
	ctx.DisableReroute()
	if !config.CheckRequest {
		log.Debugf("request checking is disabled")
		ctx.DontReadRequestBody()
	}
	return types.ActionContinue
}

func onHttpRequestBody(ctx wrapper.HttpContext, config cfg.AISecurityConfig, body []byte) types.Action {
	log.Debugf("checking request body...")
	switch config.Action {
	case cfg.MultiModalGuard:
		return multi_modal_guard.OnHttpRequestBody(ctx, config, body)
	case cfg.TextModerationPlus:
		return text_moderation_plus.OnHttpRequestBody(ctx, config, body)
	default:
		log.Warnf("Unknown action %s", config.Action)
		return types.ActionContinue
	}
}

func onHttpResponseHeaders(ctx wrapper.HttpContext, config cfg.AISecurityConfig) types.Action {
	if !config.CheckResponse {
		log.Debugf("response checking is disabled")
		ctx.DontReadResponseBody()
		return types.ActionContinue
	}
	statusCode, _ := proxywasm.GetHttpResponseHeader(":status")
	// For embedding API, we need to check error.message in non-200 responses
	// so we don't skip response body check for embedding apiType
	if statusCode != "200" && config.ApiType != cfg.ApiEmbedding {
		log.Debugf("response is not 200, skip response body check")
		ctx.DontReadResponseBody()
		return types.ActionContinue
	}
	switch config.Action {
	case cfg.MultiModalGuard:
		return multi_modal_guard.OnHttpResponseHeaders(ctx, config)
	case cfg.TextModerationPlus:
		return text_moderation_plus.OnHttpResponseHeaders(ctx, config)
	default:
		log.Warnf("Unknown action %s", config.Action)
		return types.ActionContinue
	}
}

func onHttpStreamingResponseBody(ctx wrapper.HttpContext, config cfg.AISecurityConfig, data []byte, endOfStream bool) []byte {
	log.Debugf("checking streaming response body...")
	switch config.Action {
	case cfg.MultiModalGuard:
		return multi_modal_guard.OnHttpStreamingResponseBody(ctx, config, data, endOfStream)
	case cfg.TextModerationPlus:
		return text_moderation_plus.OnHttpStreamingResponseBody(ctx, config, data, endOfStream)
	default:
		log.Warnf("Unknown action %s", config.Action)
		return data
	}
}

func onHttpResponseBody(ctx wrapper.HttpContext, config cfg.AISecurityConfig, body []byte) types.Action {
	log.Debugf("checking response body...")
	switch config.Action {
	case cfg.MultiModalGuard:
		return multi_modal_guard.OnHttpResponseBody(ctx, config, body)
	case cfg.TextModerationPlus:
		return text_moderation_plus.OnHttpResponseBody(ctx, config, body)
	default:
		log.Warnf("Unknown action %s", config.Action)
		return types.ActionContinue
	}
}
