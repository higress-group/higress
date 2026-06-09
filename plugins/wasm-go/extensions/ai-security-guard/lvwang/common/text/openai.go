package text

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	cfg "github.com/alibaba/higress/plugins/wasm-go/extensions/ai-security-guard/config"
	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-security-guard/lvwang/common"
	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-security-guard/utils"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	responseFallbackPathsCtxKey       = "response_fallback_paths"
	responseStreamFallbackPathsCtxKey = "response_stream_fallback_paths"
	responseStartTimeCtxKey           = "response_start_time"
	responseMaskedCtxKey              = "response_masked"
	responseMaskCounterEmittedCtxKey  = "response_mask_counter_emitted"
)

type responseContentTarget struct {
	Path string
	Text string
}

type responseContentMatch struct {
	Text    string
	Targets []responseContentTarget
}

type streamingBufferedChunk struct {
	Raw   []byte
	Match responseContentMatch
}

// HandleTextGenerationResponseHeader decides whether response text guardrail
// should own the response body. The resolver check must happen before buffering
// or streaming pause so consumers with disabled response text fallback pass
// through without extra body reads.
func HandleTextGenerationResponseHeader(ctx wrapper.HttpContext, config cfg.AISecurityConfig) types.Action {
	consumer, _ := ctx.GetContext("consumer").(string)
	decision := config.ResolveResponseCheckService(consumer)
	if !decision.Enabled {
		log.Debugf("response text check disabled for consumer %s, source=%s", consumer, decision.Source)
		ctx.DontReadResponseBody()
		return types.ActionContinue
	}
	contentType, _ := proxywasm.GetHttpResponseHeader("content-type")
	ctx.SetContext("end_of_stream_received", false)
	ctx.SetContext("during_call", false)
	ctx.SetContext("risk_detected", false)
	ctx.SetContext(responseStartTimeCtxKey, time.Now().UnixMilli())
	ctx.SetContext(responseFallbackPathsCtxKey, buildEffectiveFallbackPaths(config.ResponseContentJsonPath, config.ResponseContentFallbackJsonPaths))
	ctx.SetContext(responseStreamFallbackPathsCtxKey, buildEffectiveFallbackPaths(config.ResponseStreamContentJsonPath, config.ResponseStreamContentFallbackJsonPaths))
	sessionID, _ := utils.GenerateHexID(20)
	ctx.SetContext("sessionID", sessionID)
	if strings.Contains(contentType, "text/event-stream") {
		ctx.NeedPauseStreamingResponse()
		return types.ActionContinue
	} else {
		if err := proxywasm.RemoveHttpResponseHeader("content-length"); err != nil {
			log.Debugf("failed to remove response content-length before buffering: %v", err)
		}
		ctx.BufferResponseBody()
		return types.HeaderStopIteration
	}
}

// HandleTextGenerationStreamingResponseBody submits buffered SSE text chunks to
// the response text service chosen by the resolver. The header handler already
// filters disabled consumers, and this function keeps the resolved service in
// one decision so every async submission uses the same consumer/default source.
func HandleTextGenerationStreamingResponseBody(ctx wrapper.HttpContext, config cfg.AISecurityConfig, data []byte, endOfStream bool) []byte {
	consumer, _ := ctx.GetContext("consumer").(string)
	responseDecision := config.ResolveResponseCheckService(consumer)
	streamFallbackPaths := getEffectiveFallbackPathsFromContext(ctx, responseStreamFallbackPathsCtxKey, config.ResponseStreamContentJsonPath, config.ResponseStreamContentFallbackJsonPaths)
	var sessionID string
	if ctx.GetContext("sessionID") == nil {
		sessionID, _ = utils.GenerateHexID(20)
		ctx.SetContext("sessionID", sessionID)
	} else {
		sessionID, _ = ctx.GetContext("sessionID").(string)
	}
	var bufferQueue []streamingBufferedChunk
	currentSubmissionIndex := 0
	var singleCall func()
	callback := func(statusCode int, responseHeaders http.Header, responseBody []byte) {
		log.Info(string(responseBody))
		startTime, _ := ctx.GetContext(responseStartTimeCtxKey).(int64)
		passBufferedAfterBuildFailure := func(buildErr error, response *cfg.Response) {
			cfg.CompleteGuardrailSubmissionEvent(ctx, currentSubmissionIndex, responseBody, cfg.GuardrailResultError)
			log.Errorf("failed to build deny response body: %v", buildErr)
			endStream := ctx.GetContext("end_of_stream_received").(bool) && ctx.BufferQueueSize() == 0
			proxywasm.InjectEncodedDataToFilterChain(joinBufferedRaw(bufferQueue), endStream)
			bufferQueue = []streamingBufferedChunk{}
			config.IncrementCounter("ai_sec_response_deny_buildfail", 1)
			ctx.SetUserAttribute("safecheck_response_rt", time.Now().UnixMilli()-startTime)
			ctx.SetUserAttribute("safecheck_status", "build_fallback_pass")
			if response != nil {
				setResponseRiskAttributes(ctx, *response)
			}
			cfg.WriteGuardrailLog(ctx)
			if !endStream {
				ctx.SetContext("during_call", false)
				singleCall()
			}
		}
		sendStreamingOpenAIFallbackDeny := func(reason string, response cfg.Response) {
			proxywasm.LogInfof("safecheck_action_source=response_mask_fallback_to_block, reason=%s", reason)
			jsonData, err := buildOpenAIFallbackDenyData(config, true)
			if err != nil {
				passBufferedAfterBuildFailure(err, &response)
				return
			}
			cfg.CompleteGuardrailSubmissionEvent(ctx, currentSubmissionIndex, responseBody, cfg.GuardrailResultDeny)
			proxywasm.InjectEncodedDataToFilterChain(jsonData, true)
			bufferQueue = []streamingBufferedChunk{}
			ctx.SetContext("risk_detected", true)
			ctx.SetContext("during_call", false)
			config.IncrementCounter("ai_sec_response_deny", 1)
			ctx.SetUserAttribute("safecheck_response_rt", time.Now().UnixMilli()-startTime)
			ctx.SetUserAttribute("safecheck_status", "response deny")
			setResponseRiskAttributes(ctx, response)
			ctx.WriteUserAttributeToLogWithKey(wrapper.AILogKey)
		}
		if statusCode != 200 || gjson.GetBytes(responseBody, "Code").Int() != 200 {
			cfg.MarkGuardrailResponseError(ctx, currentSubmissionIndex, responseBody, startTime)
			if ctx.GetContext("end_of_stream_received").(bool) {
				proxywasm.ResumeHttpResponse()
			}
			ctx.SetContext("during_call", false)
			return
		}
		var response cfg.Response
		err := json.Unmarshal(responseBody, &response)
		if err != nil {
			log.Error("failed to unmarshal aliyun content security response at response phase")
			cfg.MarkGuardrailResponseError(ctx, currentSubmissionIndex, responseBody, startTime)
			if ctx.GetContext("end_of_stream_received").(bool) {
				proxywasm.ResumeHttpResponse()
			}
			ctx.SetContext("during_call", false)
			return
		}
		riskResult := cfg.EvaluateRisk(config.Action, response.Data, config, consumer)
		passStreamingResponse := func() {
			cfg.CompleteGuardrailSubmissionEvent(ctx, currentSubmissionIndex, responseBody, cfg.GuardrailResultPass)
			cfg.WriteGuardrailLog(ctx)
			endStream := ctx.GetContext("end_of_stream_received").(bool) && ctx.BufferQueueSize() == 0
			proxywasm.InjectEncodedDataToFilterChain(joinBufferedRaw(bufferQueue), endStream)
			bufferQueue = []streamingBufferedChunk{}
			if !endStream {
				ctx.SetContext("during_call", false)
				singleCall()
			}
		}
		switch riskResult {
		case cfg.RiskPass:
			passStreamingResponse()
			return
		case cfg.RiskMask:
			if config.ProtocolOriginal {
				jsonData, err := buildOriginalProtocolStreamingDenyData(config, response, consumer)
				if err != nil {
					passBufferedAfterBuildFailure(err, &response)
					return
				}
				cfg.CompleteGuardrailSubmissionEvent(ctx, currentSubmissionIndex, responseBody, cfg.GuardrailResultDeny)
				proxywasm.InjectEncodedDataToFilterChain(jsonData, true)
				bufferQueue = []streamingBufferedChunk{}
				ctx.SetContext("risk_detected", true)
				ctx.SetContext("during_call", false)
				config.IncrementCounter("ai_sec_response_deny", 1)
				ctx.SetUserAttribute("safecheck_response_rt", time.Now().UnixMilli()-startTime)
				ctx.SetUserAttribute("safecheck_status", "response deny")
				setResponseRiskAttributes(ctx, response)
				ctx.WriteUserAttributeToLogWithKey(wrapper.AILogKey)
				return
			}
			desensitization := cfg.ExtractDesensitizationForRisk(response.Data, config, consumer)
			if desensitization == "" {
				sendStreamingOpenAIFallbackDeny("empty_desensitization", response)
				return
			}
			rewritten, err := rewriteStreamingBuffer(bufferQueue, desensitization)
			if err != nil {
				log.Errorf("failed to rewrite streaming response body, falling back to block: %v", err)
				sendStreamingOpenAIFallbackDeny("rewrite_failed", response)
				return
			}
			cfg.CompleteGuardrailSubmissionEvent(ctx, currentSubmissionIndex, responseBody, cfg.GuardrailResultMask)
			markResponseMaskSuccess(ctx, config, startTime)
			cfg.WriteGuardrailLog(ctx)
			endStream := ctx.GetContext("end_of_stream_received").(bool) && ctx.BufferQueueSize() == 0
			proxywasm.InjectEncodedDataToFilterChain(rewritten, endStream)
			bufferQueue = []streamingBufferedChunk{}
			if !endStream {
				ctx.SetContext("during_call", false)
				singleCall()
			}
			return
		case cfg.RiskBlock:
			jsonData, err := buildStreamingDenyData(config, response, consumer)
			if err != nil {
				// Build failure → fail-open: inject the buffered upstream content as-is.
				// Make this path observable so operators can spot the silent passthrough
				// instead of mistakenly attributing observed denies-only to the success
				// path's metrics. Symmetric with the success path's observability suite
				// (counter / safecheck_response_rt / safecheck_status / log / risk_detected).
				passBufferedAfterBuildFailure(err, &response)
				return
			}
			cfg.CompleteGuardrailSubmissionEvent(ctx, currentSubmissionIndex, responseBody, cfg.GuardrailResultDeny)
			proxywasm.InjectEncodedDataToFilterChain(jsonData, true)
			bufferQueue = []streamingBufferedChunk{}
			ctx.SetContext("risk_detected", true)
			ctx.SetContext("during_call", false)
			config.IncrementCounter("ai_sec_response_deny", 1)
			ctx.SetUserAttribute("safecheck_response_rt", time.Now().UnixMilli()-startTime)
			ctx.SetUserAttribute("safecheck_status", "response deny")
			setResponseRiskAttributes(ctx, response)
			ctx.WriteUserAttributeToLogWithKey(wrapper.AILogKey)
			return
		default:
			log.Warnf("unknown response risk result %v at streaming response phase, treating as pass", riskResult)
			passStreamingResponse()
			return
		}
	}
	singleCall = func() {
		if ctx.GetContext("during_call").(bool) {
			return
		}
		if ctx.BufferQueueSize() >= config.BufferLimit || ctx.GetContext("end_of_stream_received").(bool) {
			var buffer string
			for ctx.BufferQueueSize() > 0 {
				front := ctx.PopBuffer()
				match := extractStreamingChunkContentWithTargets(front, config.ResponseStreamContentJsonPath, streamFallbackPaths)
				bufferQueue = append(bufferQueue, streamingBufferedChunk{Raw: front, Match: match})
				buffer += match.Text
				if len([]rune(buffer)) >= config.BufferLimit {
					break
				}
			}
			// case 1: streaming body has reasoning_content, part of buffer maybe empty
			// case 2: streaming body has toolcall result, part of buffer maybe empty
			log.Debugf("current content piece: %s", buffer)
			if len(buffer) == 0 {
				buffer = "[empty content]"
			}
			ctx.SetContext("during_call", true)
			log.Debugf("current content piece: %s", buffer)
			currentSubmissionIndex = cfg.BeginGuardrailSubmissionEvent(ctx, cfg.GuardrailPhaseResponse, cfg.GuardrailModalityText)
			// responseDecision.Service may be consumer-specific or the global
			// fallback service; handlers should not reimplement that selection.
			path, headers, body := common.GenerateRequestForText(config, config.Action, responseDecision.Service, buffer, sessionID)
			err := config.Client.Post(path, headers, body, callback, config.Timeout)
			if err != nil {
				log.Errorf("failed call the safe check service: %v", err)
				startTime, _ := ctx.GetContext(responseStartTimeCtxKey).(int64)
				cfg.MarkGuardrailResponseError(ctx, currentSubmissionIndex, nil, startTime)
				if ctx.GetContext("end_of_stream_received").(bool) {
					proxywasm.ResumeHttpResponse()
				}
			}
		}
	}
	if !ctx.GetContext("risk_detected").(bool) {
		unifiedChunk := wrapper.UnifySSEChunk(data)
		hasTrailingSeparator := bytes.HasSuffix(unifiedChunk, []byte("\n\n"))
		trimmedChunk := bytes.TrimSpace(unifiedChunk)
		chunks := bytes.Split(trimmedChunk, []byte("\n\n"))
		// Filter out empty chunks
		nonEmptyChunks := make([][]byte, 0, len(chunks))
		for _, chunk := range chunks {
			if len(chunk) > 0 {
				nonEmptyChunks = append(nonEmptyChunks, chunk)
			}
		}
		// Restore separators
		for i := range len(nonEmptyChunks) - 1 {
			nonEmptyChunks[i] = append(nonEmptyChunks[i], []byte("\n\n")...)
		}
		if hasTrailingSeparator && len(nonEmptyChunks) > 0 {
			nonEmptyChunks[len(nonEmptyChunks)-1] = append(nonEmptyChunks[len(nonEmptyChunks)-1], []byte("\n\n")...)
		}
		for _, chunk := range nonEmptyChunks {
			ctx.PushBuffer(chunk)
		}
		// for _, chunk := range bytes.Split(bytes.TrimSpace(wrapper.UnifySSEChunk(data)), []byte("\n\n")) {
		// 	ctx.PushBuffer([]byte(string(chunk) + "\n\n"))
		// }
		ctx.SetContext("end_of_stream_received", endOfStream)
		if !ctx.GetContext("during_call").(bool) {
			singleCall()
		}
	} else if endOfStream {
		proxywasm.ResumeHttpResponse()
	}
	return []byte{}
}

// HandleTextGenerationResponseBody applies non-streaming response text checks
// using the resolved consumer/default service. The explicit disabled guard is a
// defensive mirror of the header path, so direct body invocations do not call
// Lvwang with an empty fallback service.
func HandleTextGenerationResponseBody(ctx wrapper.HttpContext, config cfg.AISecurityConfig, body []byte) types.Action {
	consumer, _ := ctx.GetContext("consumer").(string)
	responseDecision := config.ResolveResponseCheckService(consumer)
	if !responseDecision.Enabled {
		return types.ActionContinue
	}
	responseFallbackPaths := getEffectiveFallbackPathsFromContext(ctx, responseFallbackPathsCtxKey, config.ResponseContentJsonPath, config.ResponseContentFallbackJsonPaths)
	streamFallbackPaths := getEffectiveFallbackPathsFromContext(ctx, responseStreamFallbackPathsCtxKey, config.ResponseStreamContentJsonPath, config.ResponseStreamContentFallbackJsonPaths)
	log.Debugf("checking response body...")
	startTime := time.Now().UnixMilli()
	contentType, _ := proxywasm.GetHttpResponseHeader("content-type")
	isStreamingResponse := strings.Contains(contentType, "event-stream")
	var content string
	var contentMatch responseContentMatch
	var streamingChunks []streamingBufferedChunk
	if isStreamingResponse {
		streamingChunks, content = extractStreamingBodyChunksWithTargets(body, config.ResponseStreamContentJsonPath, streamFallbackPaths)
	} else {
		contentMatch = extractResponseContentWithTargets(body, config.ResponseContentJsonPath, responseFallbackPaths)
		content = contentMatch.Text
	}
	log.Debugf("Raw response content is: %s", content)
	if len(content) == 0 {
		log.Info("response content is empty. skip")
		return types.ActionContinue
	}
	contentIndex := 0
	prevContentIndex := 0
	maskedContent := content
	hasMasked := false
	sessionID, _ := utils.GenerateHexID(20)
	currentSubmissionIndex := 0
	var singleCall func()
	rewriteMaskedResponseBody := func() ([]byte, error) {
		if isStreamingResponse {
			return rewriteStreamingBuffer(streamingChunks, maskedContent)
		}
		return rewriteNonStreamingResponseBody(body, contentMatch, maskedContent)
	}
	completeResponseMask := func(responseBody []byte) bool {
		newBody, replaceErr := rewriteMaskedResponseBody()
		if replaceErr != nil {
			log.Errorf("failed to replace response body content, falling back to block: %v", replaceErr)
			if sendErr := cfg.SendFallbackDenyResponse(config, isStreamingResponse); sendErr != nil {
				log.Errorf("failed to build deny response body: %v", sendErr)
				cfg.MarkGuardrailResponseError(ctx, currentSubmissionIndex, responseBody, startTime)
				proxywasm.ResumeHttpResponse()
				return false
			}
			config.IncrementCounter("ai_sec_response_deny", 1)
			ctx.SetUserAttribute("safecheck_response_rt", time.Now().UnixMilli()-startTime)
			ctx.SetUserAttribute("safecheck_status", "response deny")
			cfg.CompleteGuardrailSubmissionEvent(ctx, currentSubmissionIndex, responseBody, cfg.GuardrailResultDeny)
			cfg.WriteGuardrailLog(ctx)
			return false
		}
		proxywasm.ReplaceHttpResponseBody(newBody)
		markResponseMaskSuccess(ctx, config, startTime)
		return true
	}
	sendResponseFallbackDeny := func(reason string, response cfg.Response, responseBody []byte) {
		proxywasm.LogInfof("safecheck_action_source=response_mask_fallback_to_block, reason=%s", reason)
		if sendErr := cfg.SendFallbackDenyResponse(config, isStreamingResponse); sendErr != nil {
			log.Errorf("failed to build deny response body: %v", sendErr)
			cfg.MarkGuardrailResponseError(ctx, currentSubmissionIndex, responseBody, startTime)
			proxywasm.ResumeHttpResponse()
			return
		}
		config.IncrementCounter("ai_sec_response_deny", 1)
		ctx.SetUserAttribute("safecheck_response_rt", time.Now().UnixMilli()-startTime)
		ctx.SetUserAttribute("safecheck_status", "response deny")
		setResponseRiskAttributes(ctx, response)
		cfg.CompleteGuardrailSubmissionEvent(ctx, currentSubmissionIndex, responseBody, cfg.GuardrailResultDeny)
		cfg.WriteGuardrailLog(ctx)
	}
	resumeAfterGuardrailResponseError := func(responseBody []byte) {
		if !hasMasked {
			cfg.MarkGuardrailResponseError(ctx, currentSubmissionIndex, responseBody, startTime)
			proxywasm.ResumeHttpResponse()
			return
		}

		cfg.CompleteGuardrailSubmissionEvent(ctx, currentSubmissionIndex, responseBody, cfg.GuardrailResultError)
		newBody, replaceErr := rewriteMaskedResponseBody()
		if replaceErr != nil {
			log.Errorf("failed to apply accumulated response mask after guardrail error, falling back to block: %v", replaceErr)
			if sendErr := cfg.SendFallbackDenyResponse(config, isStreamingResponse); sendErr != nil {
				log.Errorf("failed to build deny response body: %v", sendErr)
				ctx.SetUserAttribute("safecheck_response_rt", time.Now().UnixMilli()-startTime)
				ctx.SetUserAttribute("safecheck_status", "response error")
				cfg.WriteGuardrailLog(ctx)
				proxywasm.ResumeHttpResponse()
				return
			}
			config.IncrementCounter("ai_sec_response_deny", 1)
			ctx.SetUserAttribute("safecheck_response_rt", time.Now().UnixMilli()-startTime)
			ctx.SetUserAttribute("safecheck_status", "response deny")
			cfg.WriteGuardrailLog(ctx)
			return
		}

		proxywasm.ReplaceHttpResponseBody(newBody)
		markResponseMaskSuccess(ctx, config, startTime)
		cfg.WriteGuardrailLog(ctx)
		proxywasm.ResumeHttpResponse()
	}
	callback := func(statusCode int, responseHeaders http.Header, responseBody []byte) {
		log.Info(string(responseBody))
		if statusCode != 200 || gjson.GetBytes(responseBody, "Code").Int() != 200 {
			resumeAfterGuardrailResponseError(responseBody)
			return
		}
		var response cfg.Response
		err := json.Unmarshal(responseBody, &response)
		if err != nil {
			log.Error("failed to unmarshal aliyun content security response at response phase")
			resumeAfterGuardrailResponseError(responseBody)
			return
		}
		riskResult := cfg.EvaluateRisk(config.Action, response.Data, config, consumer)
		passNonStreamingResponse := func() {
			if contentIndex >= len(maskedContent) {
				if hasMasked {
					if !completeResponseMask(responseBody) {
						return
					}
				} else {
					markResponsePassIfNotMasked(ctx, startTime)
				}
				cfg.CompleteGuardrailSubmissionEvent(ctx, currentSubmissionIndex, responseBody, cfg.GuardrailResultPass)
				cfg.WriteGuardrailLog(ctx)
				proxywasm.ResumeHttpResponse()
			} else {
				cfg.CompleteGuardrailSubmissionEvent(ctx, currentSubmissionIndex, responseBody, cfg.GuardrailResultPass)
				singleCall()
			}
		}
		switch riskResult {
		case cfg.RiskPass:
			passNonStreamingResponse()
			return
		case cfg.RiskMask:
			if config.ProtocolOriginal {
				if err := cfg.SendDenyResponse(config, response, consumer, isStreamingResponse); err != nil {
					log.Errorf("failed to build deny response body: %v", err)
					cfg.MarkGuardrailResponseError(ctx, currentSubmissionIndex, responseBody, startTime)
					proxywasm.ResumeHttpResponse()
					return
				}
				config.IncrementCounter("ai_sec_response_deny", 1)
				ctx.SetUserAttribute("safecheck_response_rt", time.Now().UnixMilli()-startTime)
				ctx.SetUserAttribute("safecheck_status", "response deny")
				setResponseRiskAttributes(ctx, response)
				cfg.CompleteGuardrailSubmissionEvent(ctx, currentSubmissionIndex, responseBody, cfg.GuardrailResultDeny)
				cfg.WriteGuardrailLog(ctx)
				return
			}
			desensitization := cfg.ExtractDesensitizationForRisk(response.Data, config, consumer)
			if desensitization == "" {
				sendResponseFallbackDeny("empty_desensitization", response, responseBody)
				return
			}
			chunkStart := prevContentIndex
			chunkEnd := contentIndex
			maskedContent = replaceStringRange(maskedContent, chunkStart, chunkEnd, desensitization)
			contentIndex += len(desensitization) - (chunkEnd - chunkStart)
			hasMasked = true
			cfg.CompleteGuardrailSubmissionEvent(ctx, currentSubmissionIndex, responseBody, cfg.GuardrailResultMask)
			if contentIndex >= len(maskedContent) {
				if !completeResponseMask(responseBody) {
					return
				}
				cfg.WriteGuardrailLog(ctx)
				proxywasm.ResumeHttpResponse()
			} else {
				singleCall()
			}
			return
		case cfg.RiskBlock:
			if err := cfg.SendDenyResponse(config, response, consumer, isStreamingResponse); err != nil {
				log.Errorf("failed to build deny response body: %v", err)
				cfg.MarkGuardrailResponseError(ctx, currentSubmissionIndex, responseBody, startTime)
				proxywasm.ResumeHttpResponse()
				return
			}
			config.IncrementCounter("ai_sec_response_deny", 1)
			ctx.SetUserAttribute("safecheck_response_rt", time.Now().UnixMilli()-startTime)
			ctx.SetUserAttribute("safecheck_status", "response deny")
			setResponseRiskAttributes(ctx, response)
			cfg.CompleteGuardrailSubmissionEvent(ctx, currentSubmissionIndex, responseBody, cfg.GuardrailResultDeny)
			cfg.WriteGuardrailLog(ctx)
		default:
			log.Warnf("unknown response risk result %v at non-streaming response phase, treating as pass", riskResult)
			passNonStreamingResponse()
			return
		}
	}
	singleCall = func() {
		prevContentIndex = contentIndex
		var nextContentIndex int
		if contentIndex+cfg.LengthLimit >= len(maskedContent) {
			nextContentIndex = len(maskedContent)
		} else {
			nextContentIndex = nextUTF8ChunkEnd(maskedContent, contentIndex, cfg.LengthLimit)
		}
		contentPiece := maskedContent[contentIndex:nextContentIndex]
		contentIndex = nextContentIndex
		log.Debugf("current content piece: %s", contentPiece)
		currentSubmissionIndex = cfg.BeginGuardrailSubmissionEvent(ctx, cfg.GuardrailPhaseResponse, cfg.GuardrailModalityText)
		// Use the service chosen once at function entry so every chunk follows
		// the same resolver decision even when content is split into many calls.
		path, headers, body := common.GenerateRequestForText(config, config.Action, responseDecision.Service, contentPiece, sessionID)
		err := config.Client.Post(path, headers, body, callback, config.Timeout)
		if err != nil {
			log.Errorf("failed call the safe check service: %v", err)
			resumeAfterGuardrailResponseError(nil)
		}
	}
	singleCall()
	return types.ActionPause
}

func markResponseMaskSuccess(ctx wrapper.HttpContext, config cfg.AISecurityConfig, startTime int64) {
	ctx.SetContext(responseMaskedCtxKey, true)
	if emitted, _ := ctx.GetContext(responseMaskCounterEmittedCtxKey).(bool); !emitted {
		config.IncrementCounter("ai_sec_response_mask", 1)
		ctx.SetContext(responseMaskCounterEmittedCtxKey, true)
	}
	ctx.SetUserAttribute("safecheck_response_rt", time.Now().UnixMilli()-startTime)
	ctx.SetUserAttribute("safecheck_status", "response mask")
}

func markResponsePassIfNotMasked(ctx wrapper.HttpContext, startTime int64) {
	if masked, _ := ctx.GetContext(responseMaskedCtxKey).(bool); masked {
		ctx.SetUserAttribute("safecheck_response_rt", time.Now().UnixMilli()-startTime)
		return
	}
	ctx.SetUserAttribute("safecheck_response_rt", time.Now().UnixMilli()-startTime)
	ctx.SetUserAttribute("safecheck_status", "response pass")
}

func setResponseRiskAttributes(ctx wrapper.HttpContext, response cfg.Response) {
	if len(response.Data.Result) == 0 {
		return
	}
	ctx.SetUserAttribute("safecheck_riskLabel", response.Data.Result[0].Label)
	ctx.SetUserAttribute("safecheck_riskWords", response.Data.Result[0].RiskWords)
}

func joinBufferedRaw(chunks []streamingBufferedChunk) []byte {
	parts := make([][]byte, 0, len(chunks))
	for _, chunk := range chunks {
		parts = append(parts, chunk.Raw)
	}
	return bytes.Join(parts, []byte(""))
}

// autoExtractResponseContent tries configured fallback paths to extract text content.
func autoExtractResponseContent(body []byte, fallbackPaths []string) string {
	if len(fallbackPaths) == 0 {
		return ""
	}
	return extractTextMatchByPaths(gjson.ParseBytes(body), fallbackPaths).Text
}

// autoExtractStreamingResponseContent tries configured fallback paths to extract text content.
// It handles both bare JSON and SSE "data:" payloads, including multi-line data events.
func autoExtractStreamingResponseContent(chunk []byte, fallbackPaths []string) string {
	if len(fallbackPaths) == 0 {
		return ""
	}
	payload := bytes.TrimSpace(chunk)
	if len(payload) == 0 {
		return ""
	}
	if !isJSONPayload(payload) {
		payload = extractSSEDataPayload(payload)
		if len(payload) == 0 {
			return ""
		}
	}
	if !json.Valid(payload) {
		return ""
	}
	return extractTextMatchByPaths(gjson.ParseBytes(payload), fallbackPaths).Text
}

func isJSONPayload(payload []byte) bool {
	return len(payload) > 0 && (payload[0] == '{' || payload[0] == '[')
}

// extractSSEDataPayload concatenates all "data:" lines in one SSE event.
// SSE specifies multi-line data fields should be joined with '\n'.
func extractSSEDataPayload(chunk []byte) []byte {
	lines := bytes.Split(chunk, []byte("\n"))
	dataLines := make([][]byte, 0, len(lines))
	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		data := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if len(data) == 0 {
			continue
		}
		if bytes.Equal(data, []byte("[DONE]")) {
			return nil
		}
		dataLines = append(dataLines, data)
	}
	if len(dataLines) == 0 {
		return nil
	}
	return bytes.TrimSpace(bytes.Join(dataLines, []byte("\n")))
}

func buildEffectiveFallbackPaths(primaryPath string, fallbackPaths []string) []string {
	primaryPath = strings.TrimSpace(primaryPath)
	if len(fallbackPaths) == 0 {
		return []string{}
	}
	deduped := make([]string, 0, len(fallbackPaths))
	seen := make(map[string]struct{}, len(fallbackPaths))
	for _, path := range fallbackPaths {
		path = strings.TrimSpace(path)
		if len(path) == 0 || path == primaryPath {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		deduped = append(deduped, path)
	}
	if len(deduped) == 0 {
		return []string{}
	}
	return deduped
}

type fallbackPathContext interface {
	GetContext(key string) interface{}
	SetContext(key string, value interface{})
}

func getEffectiveFallbackPathsFromContext(ctx fallbackPathContext, ctxKey string, primaryPath string, fallbackPaths []string) []string {
	if cached, ok := ctx.GetContext(ctxKey).([]string); ok {
		return cached
	}
	effective := buildEffectiveFallbackPaths(primaryPath, fallbackPaths)
	ctx.SetContext(ctxKey, effective)
	return effective
}

func extractTextByPaths(parsed gjson.Result, paths []string) string {
	return extractTextMatchByPaths(parsed, paths).Text
}

func extractResponseContentWithTargets(body []byte, primaryPath string, fallbackPaths []string) responseContentMatch {
	paths := prependPrimaryPath(primaryPath, fallbackPaths)
	if len(paths) == 0 {
		return responseContentMatch{}
	}
	return extractTextMatchByPaths(gjson.ParseBytes(body), paths)
}

func extractStreamingChunkContentWithTargets(chunk []byte, primaryPath string, fallbackPaths []string) responseContentMatch {
	paths := prependPrimaryPath(primaryPath, fallbackPaths)
	if len(paths) == 0 {
		return responseContentMatch{}
	}
	payload := bytes.TrimSpace(chunk)
	if len(payload) == 0 {
		return responseContentMatch{}
	}
	if !isJSONPayload(payload) {
		payload = extractSSEDataPayload(payload)
		if len(payload) == 0 {
			return responseContentMatch{}
		}
	}
	if !json.Valid(payload) {
		return responseContentMatch{}
	}
	return extractTextMatchByPaths(gjson.ParseBytes(payload), paths)
}

func extractStreamingBodyChunksWithTargets(data []byte, primaryPath string, fallbackPaths []string) ([]streamingBufferedChunk, string) {
	unified := wrapper.UnifySSEChunk(data)
	hasTrailingSeparator := bytes.HasSuffix(unified, []byte("\n\n"))
	chunks := bytes.Split(bytes.TrimSpace(unified), []byte("\n\n"))
	buffered := make([]streamingBufferedChunk, 0, len(chunks))
	var parts []string
	for i, chunk := range chunks {
		if len(bytes.TrimSpace(chunk)) == 0 {
			continue
		}
		raw := append([]byte(nil), chunk...)
		if i < len(chunks)-1 || hasTrailingSeparator {
			raw = append(raw, []byte("\n\n")...)
		}
		match := extractStreamingChunkContentWithTargets(raw, primaryPath, fallbackPaths)
		if match.Text != "" {
			parts = append(parts, match.Text)
		}
		buffered = append(buffered, streamingBufferedChunk{Raw: raw, Match: match})
	}
	return buffered, strings.Join(parts, "")
}

func prependPrimaryPath(primaryPath string, fallbackPaths []string) []string {
	paths := make([]string, 0, len(fallbackPaths)+1)
	primaryPath = strings.TrimSpace(primaryPath)
	if primaryPath != "" {
		paths = append(paths, primaryPath)
	}
	for _, path := range fallbackPaths {
		path = strings.TrimSpace(path)
		if path != "" {
			paths = append(paths, path)
		}
	}
	return paths
}

func extractTextMatchByPaths(parsed gjson.Result, paths []string) responseContentMatch {
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if len(path) == 0 {
			continue
		}
		match := extractTextMatchForPath(parsed, path)
		if match.Text != "" {
			log.Debugf("response fallback path matched: %s", path)
			return match
		}
	}
	return responseContentMatch{}
}

func extractTextMatchForPath(parsed gjson.Result, path string) responseContentMatch {
	if targets, ok := resolveFilteredTextTargets(parsed, path); ok {
		return targetsToMatch(targets)
	}

	result := parsed.Get(path)
	if !result.Exists() {
		return responseContentMatch{}
	}
	text := extractTextFromResult(result)
	if text == "" {
		return responseContentMatch{}
	}
	if result.Type == gjson.String && isDirectWritablePath(path) {
		return responseContentMatch{
			Text: text,
			Targets: []responseContentTarget{{
				Path: path,
				Text: text,
			}},
		}
	}
	return responseContentMatch{Text: text}
}

func isDirectWritablePath(path string) bool {
	return !strings.Contains(path, "#") && !strings.Contains(path, "@")
}

func resolveFilteredTextTargets(parsed gjson.Result, path string) ([]responseContentTarget, bool) {
	const suffix = `.#(type=="text")#.text`
	if !strings.HasSuffix(path, suffix) {
		return nil, false
	}
	arrayPath := strings.TrimSuffix(path, suffix)
	arrayResult := parsed.Get(arrayPath)
	if !arrayResult.IsArray() {
		return nil, true
	}
	targets := make([]responseContentTarget, 0)
	for i, item := range arrayResult.Array() {
		if item.Get("type").String() != "text" {
			continue
		}
		textResult := item.Get("text")
		if !textResult.Exists() || textResult.Type != gjson.String {
			continue
		}
		text := textResult.String()
		if text == "" {
			continue
		}
		targets = append(targets, responseContentTarget{
			Path: fmt.Sprintf("%s.%d.text", arrayPath, i),
			Text: text,
		})
	}
	return targets, true
}

func targetsToMatch(targets []responseContentTarget) responseContentMatch {
	if len(targets) == 0 {
		return responseContentMatch{}
	}
	parts := make([]string, 0, len(targets))
	for _, target := range targets {
		parts = append(parts, target.Text)
	}
	return responseContentMatch{
		Text:    strings.Join(parts, ""),
		Targets: targets,
	}
}

func extractTextFromResult(result gjson.Result) string {
	if result.IsArray() {
		var parts []string
		for _, item := range result.Array() {
			if s := item.String(); len(s) > 0 {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, "")
	}
	return result.String()
}

func rewriteNonStreamingResponseBody(body []byte, match responseContentMatch, maskedContent string) ([]byte, error) {
	return rewriteJSONContentTargets(body, match.Targets, maskedContent)
}

func rewriteJSONContentTargets(body []byte, targets []responseContentTarget, replacement string) ([]byte, error) {
	if len(targets) == 0 {
		return nil, fmt.Errorf("no writable response content targets")
	}
	parts := splitReplacementByTargets(replacement, targets)
	rewritten := append([]byte(nil), body...)
	var err error
	for i, target := range targets {
		if strings.TrimSpace(target.Path) == "" {
			return nil, fmt.Errorf("empty writable response content target")
		}
		rewritten, err = sjson.SetBytes(rewritten, target.Path, parts[i])
		if err != nil {
			return nil, err
		}
		if got := gjson.GetBytes(rewritten, target.Path); !got.Exists() || got.String() != parts[i] {
			return nil, fmt.Errorf("failed to rewrite response content target %q", target.Path)
		}
	}
	return rewritten, nil
}

func rewriteStreamingBuffer(chunks []streamingBufferedChunk, replacement string) ([]byte, error) {
	originals := make([]string, 0)
	for _, chunk := range chunks {
		if chunk.Match.Text != "" {
			if len(chunk.Match.Targets) == 0 {
				return nil, fmt.Errorf("content-bearing SSE chunk has no writable response content targets")
			}
			originals = append(originals, chunk.Match.Text)
		}
	}
	if len(originals) == 0 {
		return nil, fmt.Errorf("no content-bearing SSE chunks to rewrite")
	}
	replacements := splitReplacementByOriginalTexts(replacement, originals)
	rewrittenChunks := make([][]byte, 0, len(chunks))
	replacementIndex := 0
	for _, chunk := range chunks {
		if chunk.Match.Text == "" {
			rewrittenChunks = append(rewrittenChunks, chunk.Raw)
			continue
		}
		rewritten, err := rewriteStreamingChunk(chunk.Raw, chunk.Match, replacements[replacementIndex])
		if err != nil {
			return nil, err
		}
		replacementIndex++
		rewrittenChunks = append(rewrittenChunks, rewritten)
	}
	return bytes.Join(rewrittenChunks, []byte("")), nil
}

func rewriteStreamingChunk(chunk []byte, match responseContentMatch, replacement string) ([]byte, error) {
	payload := bytes.TrimSpace(chunk)
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty SSE chunk")
	}
	if isJSONPayload(payload) {
		rewritten, err := rewriteJSONContentTargets(payload, match.Targets, replacement)
		if err != nil {
			return nil, err
		}
		rewritten = append(rewritten, trailingEventSeparator(chunk)...)
		return rewritten, nil
	}
	payload = extractSSEDataPayload(payload)
	if len(payload) == 0 || !json.Valid(payload) {
		return nil, fmt.Errorf("SSE chunk has no writable JSON data payload")
	}
	rewrittenPayload, err := rewriteJSONContentTargets(payload, match.Targets, replacement)
	if err != nil {
		return nil, err
	}
	rewrittenPayload = compactJSONPayload(rewrittenPayload)
	return replaceSSEDataPayload(chunk, rewrittenPayload), nil
}

func compactJSONPayload(payload []byte) []byte {
	var compacted bytes.Buffer
	if err := json.Compact(&compacted, payload); err != nil {
		return payload
	}
	return compacted.Bytes()
}

func replaceSSEDataPayload(chunk []byte, payload []byte) []byte {
	lines := bytes.Split(bytes.TrimRight(chunk, "\r\n"), []byte("\n"))
	outputLines := make([][]byte, 0, len(lines)+1)
	for _, line := range lines {
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 || bytes.HasPrefix(trimmed, []byte("data:")) {
			continue
		}
		outputLines = append(outputLines, append([]byte(nil), line...))
	}
	outputLines = append(outputLines, append([]byte("data: "), payload...))
	rewritten := bytes.Join(outputLines, []byte("\n"))
	if bytes.HasSuffix(chunk, []byte("\n\n")) {
		rewritten = append(rewritten, []byte("\n\n")...)
	}
	return rewritten
}

func trailingEventSeparator(chunk []byte) []byte {
	if bytes.HasSuffix(chunk, []byte("\r\n\r\n")) {
		return []byte("\r\n\r\n")
	}
	if bytes.HasSuffix(chunk, []byte("\n\n")) {
		return []byte("\n\n")
	}
	return nil
}

func splitReplacementByTargets(replacement string, targets []responseContentTarget) []string {
	originals := make([]string, 0, len(targets))
	for _, target := range targets {
		originals = append(originals, target.Text)
	}
	return splitReplacementByOriginalTexts(replacement, originals)
}

func splitReplacementByOriginalTexts(replacement string, originals []string) []string {
	if len(originals) == 0 {
		return []string{}
	}
	if len(originals) == 1 {
		return []string{replacement}
	}
	totalOriginalRunes := 0
	for _, original := range originals {
		totalOriginalRunes += utf8.RuneCountInString(original)
	}
	if totalOriginalRunes == 0 {
		parts := make([]string, len(originals))
		parts[len(parts)-1] = replacement
		return parts
	}
	replacementRunes := []rune(replacement)
	parts := make([]string, len(originals))
	previousCut := 0
	cumulativeOriginalRunes := 0
	for i, original := range originals {
		if i == len(originals)-1 {
			parts[i] = string(replacementRunes[previousCut:])
			break
		}
		cumulativeOriginalRunes += utf8.RuneCountInString(original)
		nextCut := len(replacementRunes) * cumulativeOriginalRunes / totalOriginalRunes
		if nextCut < previousCut {
			nextCut = previousCut
		}
		parts[i] = string(replacementRunes[previousCut:nextCut])
		previousCut = nextCut
	}
	return parts
}

func replaceStringRange(content string, start, end int, replacement string) string {
	return content[:start] + replacement + content[end:]
}

func nextUTF8ChunkEnd(content string, start, limit int) int {
	end := start + limit
	if end >= len(content) {
		return len(content)
	}
	for end > start && !utf8.RuneStart(content[end]) {
		end--
	}
	if end == start {
		end = start + limit
		for end < len(content) && !utf8.RuneStart(content[end]) {
			end++
		}
	}
	return end
}

func buildStreamingDenyData(config cfg.AISecurityConfig, response cfg.Response, consumer string) ([]byte, error) {
	if config.ProtocolOriginal {
		return buildOriginalProtocolStreamingDenyData(config, response, consumer)
	}
	return cfg.BuildOpenAIDenyData(config, response, consumer, true)
}

func buildOpenAIFallbackDenyData(config cfg.AISecurityConfig, isStream bool) ([]byte, error) {
	marshalledDenyMessage := wrapper.MarshalStr(cfg.ResolveDenyMessage(config))
	randomID := utils.GenerateRandomChatID()
	createdTs := time.Now().Unix()
	if config.OpenAIDenyResponseFormat == cfg.OpenAIDenyResponseFormatStructured {
		guardrailBody, err := cfg.BuildOpenAIFallbackDenyResponseBody(config)
		if err != nil {
			return nil, err
		}
		if isStream {
			return []byte(fmt.Sprintf(cfg.OpenAIStreamResponseFormatStructured, randomID, createdTs, marshalledDenyMessage, randomID, createdTs, string(guardrailBody))), nil
		}
		return []byte(fmt.Sprintf(cfg.OpenAIResponseFormatStructured, randomID, createdTs, marshalledDenyMessage, string(guardrailBody))), nil
	}
	if isStream {
		return []byte(fmt.Sprintf(cfg.OpenAIStreamResponseFormatLegacy, randomID, createdTs, marshalledDenyMessage, randomID, createdTs)), nil
	}
	return []byte(fmt.Sprintf(cfg.OpenAIResponseFormatLegacy, randomID, createdTs, marshalledDenyMessage)), nil
}

func buildOriginalProtocolStreamingDenyData(config cfg.AISecurityConfig, response cfg.Response, consumer string) ([]byte, error) {
	denyBody, err := cfg.BuildDenyResponseBody(response, config, consumer)
	if err != nil {
		return nil, err
	}
	data := make([]byte, 0, len("data: ")+len(denyBody)+len("\n\n"))
	data = append(data, []byte("data: ")...)
	data = append(data, denyBody...)
	data = append(data, []byte("\n\n")...)
	return data, nil
}

// autoExtractStreamingResponseFromSSE tries configured fallback paths on a full SSE body.
func autoExtractStreamingResponseFromSSE(data []byte, fallbackPaths []string) string {
	if len(fallbackPaths) == 0 {
		return ""
	}
	chunks := bytes.Split(bytes.TrimSpace(wrapper.UnifySSEChunk(data)), []byte("\n\n"))
	var parts []string
	for _, chunk := range chunks {
		if s := autoExtractStreamingResponseContent(chunk, fallbackPaths); len(s) > 0 {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "")
}
