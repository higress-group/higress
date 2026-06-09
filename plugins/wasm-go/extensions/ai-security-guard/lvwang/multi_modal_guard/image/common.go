package image

import (
	"strings"

	cfg "github.com/alibaba/higress/plugins/wasm-go/extensions/ai-security-guard/config"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

type ImageItem struct {
	Content string
	Type    string // URL or BASE64
}

// HandleImageGenerationResponseHeader decides whether generated images should
// be buffered for response-image checks. Response image fallback is disabled by
// default, so most image responses pass through unless a consumer rule or
// explicit defaultResponseImageCheckEnabled turns the modality on.
func HandleImageGenerationResponseHeader(ctx wrapper.HttpContext, config cfg.AISecurityConfig) types.Action {
	consumer, _ := ctx.GetContext("consumer").(string)
	decision := config.ResolveResponseImageCheckService(consumer)
	if !decision.Enabled {
		log.Debugf("response image check disabled for consumer %s, source=%s", consumer, decision.Source)
		ctx.DontReadResponseBody()
		return types.ActionContinue
	}
	contentType, _ := proxywasm.GetHttpResponseHeader("content-type")
	ctx.SetContext("risk_detected", false)
	if strings.Contains(contentType, "text/event-stream") {
		ctx.DontReadResponseBody()
		return types.ActionContinue
	} else {
		ctx.BufferResponseBody()
		return types.HeaderStopIteration
	}
}
