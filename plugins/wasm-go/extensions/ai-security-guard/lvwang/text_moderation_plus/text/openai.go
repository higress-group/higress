package text

import (
	"encoding/json"
	"net/http"
	"time"

	cfg "github.com/alibaba/higress/plugins/wasm-go/extensions/ai-security-guard/config"
	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-security-guard/lvwang/common"
	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-security-guard/utils"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

// HandleTextGenerationRequestBody checks TextModerationPlus request text using
// the service selected by the resolver. If the request text fallback is
// disabled for this consumer, the request passes through without calling the
// global moderation service.
func HandleTextGenerationRequestBody(ctx wrapper.HttpContext, config cfg.AISecurityConfig, body []byte) types.Action {
	consumer, _ := ctx.GetContext("consumer").(string)
	startTime := time.Now().UnixMilli()
	content := gjson.GetBytes(body, config.RequestContentJsonPath).String()
	log.Debugf("Raw request content is: %s", content)
	if len(content) == 0 {
		log.Info("request content is empty. skip")
		return types.ActionContinue
	}
	contentIndex := 0
	sessionID, _ := utils.GenerateHexID(20)
	currentSubmissionIndex := 0
	var singleCall func()
	callback := func(statusCode int, responseHeaders http.Header, responseBody []byte) {
		log.Info(string(responseBody))
		if statusCode != 200 || gjson.GetBytes(responseBody, "Code").Int() != 200 {
			cfg.MarkGuardrailRequestError(ctx, currentSubmissionIndex, responseBody, startTime)
			proxywasm.ResumeHttpRequest()
			return
		}
		var response cfg.Response
		err := json.Unmarshal(responseBody, &response)
		if err != nil {
			log.Error("failed to unmarshal aliyun content security response at request phase")
			cfg.MarkGuardrailRequestError(ctx, currentSubmissionIndex, responseBody, startTime)
			proxywasm.ResumeHttpRequest()
			return
		}
		if cfg.IsRiskLevelAcceptable(config.Action, response.Data, config, consumer) {
			if contentIndex >= len(content) {
				endTime := time.Now().UnixMilli()
				ctx.SetUserAttribute("safecheck_request_rt", endTime-startTime)
				ctx.SetUserAttribute("safecheck_status", "request pass")
			}
			cfg.CompleteGuardrailSubmissionEvent(ctx, currentSubmissionIndex, responseBody, cfg.GuardrailResultPass)
			if contentIndex >= len(content) {
				cfg.WriteGuardrailLog(ctx)
				proxywasm.ResumeHttpRequest()
			} else {
				singleCall()
			}
			return
		}
		if err := cfg.SendDenyResponse(config, response, consumer, gjson.GetBytes(body, "stream").Bool()); err != nil {
			log.Errorf("failed to build deny response body: %v", err)
			cfg.MarkGuardrailRequestError(ctx, currentSubmissionIndex, responseBody, startTime)
			proxywasm.ResumeHttpRequest()
			return
		}
		ctx.DontReadResponseBody()
		config.IncrementCounter("ai_sec_request_deny", 1)
		endTime := time.Now().UnixMilli()
		ctx.SetUserAttribute("safecheck_request_rt", endTime-startTime)
		ctx.SetUserAttribute("safecheck_status", "reqeust deny")
		if len(response.Data.Result) > 0 {
			ctx.SetUserAttribute("safecheck_riskLabel", response.Data.Result[0].Label)
			ctx.SetUserAttribute("safecheck_riskWords", response.Data.Result[0].RiskWords)
		}
		cfg.CompleteGuardrailSubmissionEvent(ctx, currentSubmissionIndex, responseBody, cfg.GuardrailResultDeny)
		cfg.WriteGuardrailLog(ctx)
	}
	decision := config.ResolveRequestCheckService(consumer)
	if !decision.Enabled {
		log.Debugf("request text check disabled for consumer %s, source=%s", consumer, decision.Source)
		return types.ActionContinue
	}
	singleCall = func() {
		currentSubmissionIndex = cfg.BeginGuardrailSubmissionEvent(ctx, cfg.GuardrailPhaseRequest, cfg.GuardrailModalityText)
		var nextContentIndex int
		if contentIndex+cfg.LengthLimit >= len(content) {
			nextContentIndex = len(content)
		} else {
			nextContentIndex = contentIndex + cfg.LengthLimit
		}
		contentPiece := content[contentIndex:nextContentIndex]
		contentIndex = nextContentIndex
		// decision.Service carries either the matching consumer service or the
		// enabled default service; direct global lookup would break skip-only
		// consumer scenarios.
		path, headers, body := common.GenerateRequestForText(config, cfg.TextModerationPlus, decision.Service, contentPiece, sessionID)
		err := config.Client.Post(path, headers, body, callback, config.Timeout)
		if err != nil {
			log.Errorf("failed call the safe check service: %v", err)
			cfg.MarkGuardrailRequestError(ctx, currentSubmissionIndex, nil, startTime)
			proxywasm.ResumeHttpRequest()
		}
	}
	singleCall()
	return types.ActionPause
}
