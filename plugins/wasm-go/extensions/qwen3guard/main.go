package main

import (
	"bytes"
	"net/http"
	"strings"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

const (
	ctxRequestPrompt = "qwen3guard.request_prompt"
	ctxStreamPartial = "qwen3guard.stream_partial"
	ctxStreamPending = "qwen3guard.stream_pending"
	ctxResponseText  = "qwen3guard.response_text"
	ctxUncheckedText = "qwen3guard.unchecked_text"
	ctxStreamDone    = "qwen3guard.stream_done"
	ctxStreamBlocked = "qwen3guard.stream_blocked"
	ctxStreamBypass  = "qwen3guard.stream_bypass"
	ctxDuringCall    = "qwen3guard.during_call"
)

func main() {}

func init() {
	wrapper.SetCtx(
		pluginName,
		wrapper.ParseConfigBy(parseConfig),
		wrapper.ProcessRequestHeadersBy(onHttpRequestHeaders),
		wrapper.ProcessRequestBodyBy(onHttpRequestBody),
		wrapper.ProcessResponseHeadersBy(onHttpResponseHeaders),
		wrapper.ProcessStreamingResponseBodyBy(onHttpStreamingResponseBody),
		wrapper.ProcessResponseBodyBy(onHttpResponseBody),
	)
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, c pluginConfig, log log.Log) types.Action {
	ctx.DisableReroute()
	if c.checkResponse {
		_ = proxywasm.RemoveHttpRequestHeader("Accept-Encoding")
	}
	if !c.checkRequest && !c.checkResponse {
		ctx.DontReadRequestBody()
		ctx.DontReadResponseBody()
		return types.ActionContinue
	}
	if !ctx.HasRequestBody() {
		if c.checkRequest {
			log.Warnf("qwen3guard: request body is empty, skip request moderation")
		}
		return types.ActionContinue
	}
	ctx.SetRequestBodyBufferLimit(c.maxBodyBytes)
	ctx.BufferRequestBody()
	return types.ActionContinue
}

func onHttpRequestBody(ctx wrapper.HttpContext, c pluginConfig, body []byte, log log.Log) types.Action {
	prompt, ok := extractJSONText(body, c.requestContentJSONPath)
	if ok {
		ctx.SetContext(ctxRequestPrompt, prompt)
	} else {
		log.Warnf("qwen3guard: request content path %q not found, skip request moderation", c.requestContentJSONPath)
	}
	if !c.checkRequest || !ok {
		return types.ActionContinue
	}

	requestBody, err := buildPromptModerationBody(c.model, prompt)
	if err != nil {
		log.Warnf("qwen3guard: build request moderation body failed, fail open: %v", err)
		return types.ActionContinue
	}
	if err := c.client.Post(c.requestPath, buildGuardHeaders(c.apiKey), requestBody,
		func(statusCode int, _ http.Header, responseBody []byte) {
			result, err := parseGuardHTTPResponse(statusCode, responseBody)
			if err != nil {
				log.Warnf("qwen3guard: request moderation failed, fail open: %v", err)
				proxywasm.ResumeHttpRequest()
				return
			}
			if shouldBlockRisk(result.Safety, c.riskLevelBar) {
				proxywasm.SendHttpResponse(
					uint32(c.denyCode),
					[][2]string{{"content-type", "application/json"}},
					buildChatDenyBody(c.denyMessage),
					-1,
				)
				ctx.DontReadResponseBody()
				return
			}
			proxywasm.ResumeHttpRequest()
		}, c.timeoutMS); err != nil {
		log.Warnf("qwen3guard: dispatch request moderation failed, fail open: %v", err)
		return types.ActionContinue
	}
	return types.ActionPause
}

func onHttpResponseHeaders(ctx wrapper.HttpContext, c pluginConfig, log log.Log) types.Action {
	if !c.checkResponse {
		ctx.DontReadResponseBody()
		return types.ActionContinue
	}
	status, _ := proxywasm.GetHttpResponseHeader(":status")
	if status != "200" {
		ctx.DontReadResponseBody()
		return types.ActionContinue
	}

	_ = proxywasm.RemoveHttpResponseHeader("content-length")
	ctx.SetResponseBodyBufferLimit(c.maxBodyBytes)
	contentType, _ := proxywasm.GetHttpResponseHeader("content-type")
	if strings.Contains(strings.ToLower(contentType), "text/event-stream") {
		ctx.NeedPauseStreamingResponse()
		ctx.SetContext(ctxStreamDone, false)
		ctx.SetContext(ctxStreamBlocked, false)
		ctx.SetContext(ctxStreamBypass, false)
		ctx.SetContext(ctxDuringCall, false)
		return types.ActionContinue
	}
	ctx.BufferResponseBody()
	return types.HeaderStopIteration
}

func onHttpResponseBody(ctx wrapper.HttpContext, c pluginConfig, body []byte, log log.Log) types.Action {
	responseText, ok := extractJSONText(body, c.responseContentJSONPath)
	if !ok {
		log.Warnf("qwen3guard: response content path %q not found, fail open", c.responseContentJSONPath)
		return types.ActionContinue
	}

	requestBody, err := buildResponseModerationBody(c.model, ctx.GetStringContext(ctxRequestPrompt, ""), responseText)
	if err != nil {
		log.Warnf("qwen3guard: build response moderation body failed, fail open: %v", err)
		return types.ActionContinue
	}
	if err := c.client.Post(c.requestPath, buildGuardHeaders(c.apiKey), requestBody,
		func(statusCode int, _ http.Header, responseBody []byte) {
			result, err := parseGuardHTTPResponse(statusCode, responseBody)
			if err != nil {
				log.Warnf("qwen3guard: response moderation failed, fail open: %v", err)
				proxywasm.ResumeHttpResponse()
				return
			}
			if shouldBlockRisk(result.Safety, c.riskLevelBar) {
				proxywasm.SendHttpResponse(
					uint32(c.denyCode),
					[][2]string{{"content-type", "application/json"}},
					buildChatDenyBody(c.denyMessage),
					-1,
				)
				return
			}
			proxywasm.ResumeHttpResponse()
		}, c.timeoutMS); err != nil {
		log.Warnf("qwen3guard: dispatch response moderation failed, fail open: %v", err)
		return types.ActionContinue
	}
	return types.ActionPause
}

func onHttpStreamingResponseBody(ctx wrapper.HttpContext, c pluginConfig, data []byte, endOfStream bool, log log.Log) []byte {
	if ctx.GetBoolContext(ctxStreamBlocked, false) {
		return []byte{}
	}
	if ctx.GetBoolContext(ctxStreamBypass, false) {
		return data
	}

	pending := ctx.GetStringContext(ctxStreamPending, "")
	combined := ctx.GetStringContext(ctxStreamPartial, "") + string(data)
	if exceedsByteLimit(len(pending), len(combined), c.maxBodyBytes) {
		return bypassStreamingResponse(ctx, pending+combined, c, log)
	}

	complete, leftover := splitCompleteSSE(combined, endOfStream)
	ctx.SetContext(ctxStreamPartial, leftover)
	if complete != "" {
		ctx.SetContext(ctxStreamPending, pending+complete)
		if !collectStreamingText(ctx, c, complete) {
			return bypassStreamingResponse(ctx, pending+complete+leftover, c, log)
		}
	}

	if ctx.GetBoolContext(ctxDuringCall, false) {
		return []byte{}
	}

	uncheckedText := ctx.GetStringContext(ctxUncheckedText, "")
	shouldCheck := uncheckedText != "" &&
		(charCount(uncheckedText) >= c.streamBufferChars || ctx.GetBoolContext(ctxStreamDone, false) || endOfStream)
	if shouldCheck {
		checkStreamingResponse(ctx, c, log)
		return []byte{}
	}

	pending = ctx.GetStringContext(ctxStreamPending, "")
	if endOfStream && pending != "" {
		ctx.SetContext(ctxStreamPending, "")
		return []byte(pending)
	}
	return []byte{}
}

func collectStreamingText(ctx wrapper.HttpContext, c pluginConfig, data string) bool {
	responseText := ctx.GetStringContext(ctxResponseText, "")
	uncheckedText := ctx.GetStringContext(ctxUncheckedText, "")
	for _, payload := range extractSSEDataPayloads(data) {
		if isDonePayload(payload) {
			ctx.SetContext(ctxStreamDone, true)
			continue
		}
		text, ok := extractJSONText([]byte(payload), c.streamingResponseContentJSONPath)
		if !ok {
			continue
		}
		if exceedsByteLimit(len(responseText), len(text), c.maxBodyBytes) {
			return false
		}
		responseText += text
		uncheckedText += text
	}
	ctx.SetContext(ctxResponseText, responseText)
	ctx.SetContext(ctxUncheckedText, uncheckedText)
	return true
}

func checkStreamingResponse(ctx wrapper.HttpContext, c pluginConfig, log log.Log) {
	requestBody, err := buildResponseModerationBody(
		c.model,
		ctx.GetStringContext(ctxRequestPrompt, ""),
		ctx.GetStringContext(ctxResponseText, ""),
	)
	if err != nil {
		log.Warnf("qwen3guard: build streaming response moderation body failed, fail open: %v", err)
		releasePendingStream(ctx, false)
		return
	}
	ctx.SetContext(ctxDuringCall, true)
	if err := c.client.Post(c.requestPath, buildGuardHeaders(c.apiKey), requestBody,
		func(statusCode int, _ http.Header, responseBody []byte) {
			ctx.SetContext(ctxDuringCall, false)
			if ctx.GetBoolContext(ctxStreamBypass, false) {
				return
			}
			result, err := parseGuardHTTPResponse(statusCode, responseBody)
			if err != nil {
				log.Warnf("qwen3guard: streaming response moderation failed, fail open: %v", err)
				ctx.SetContext(ctxUncheckedText, "")
				releasePendingStream(ctx, false)
				return
			}
			if shouldBlockRisk(result.Safety, c.riskLevelBar) {
				ctx.SetContext(ctxStreamBlocked, true)
				ctx.SetContext(ctxStreamPending, "")
				ctx.SetContext(ctxUncheckedText, "")
				proxywasm.InjectEncodedDataToFilterChain(buildStreamDenyBody(c.denyMessage), true)
				return
			}
			ctx.SetContext(ctxUncheckedText, "")
			releasePendingStream(ctx, false)
		}, c.timeoutMS); err != nil {
		ctx.SetContext(ctxDuringCall, false)
		log.Warnf("qwen3guard: dispatch streaming response moderation failed, fail open: %v", err)
		releasePendingStream(ctx, false)
	}
}

func releasePendingStream(ctx wrapper.HttpContext, forceEndStream bool) {
	pending := ctx.GetStringContext(ctxStreamPending, "")
	if pending == "" {
		if forceEndStream {
			proxywasm.ResumeHttpResponse()
		}
		return
	}
	ctx.SetContext(ctxStreamPending, "")
	endStream := forceEndStream || ctx.GetBoolContext(ctxStreamDone, false)
	proxywasm.InjectEncodedDataToFilterChain(bytes.Clone([]byte(pending)), endStream)
}

func bypassStreamingResponse(ctx wrapper.HttpContext, buffered string, c pluginConfig, log log.Log) []byte {
	log.Warnf("qwen3guard: streaming response exceeded max_body_bytes %d, fail open", c.maxBodyBytes)
	ctx.SetContext(ctxStreamBypass, true)
	ctx.SetContext(ctxStreamPartial, "")
	ctx.SetContext(ctxStreamPending, "")
	ctx.SetContext(ctxResponseText, "")
	ctx.SetContext(ctxUncheckedText, "")
	ctx.SetContext(ctxStreamDone, false)
	return []byte(buffered)
}
