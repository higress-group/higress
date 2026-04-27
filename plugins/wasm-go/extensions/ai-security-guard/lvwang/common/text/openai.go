package text

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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

const (
	responseFallbackPathsCtxKey       = "response_fallback_paths"
	responseStreamFallbackPathsCtxKey = "response_stream_fallback_paths"
)

func HandleTextGenerationResponseHeader(ctx wrapper.HttpContext, config cfg.AISecurityConfig) types.Action {
	contentType, _ := proxywasm.GetHttpResponseHeader("content-type")
	ctx.SetContext("end_of_stream_received", false)
	ctx.SetContext("during_call", false)
	ctx.SetContext("risk_detected", false)
	ctx.SetContext(responseFallbackPathsCtxKey, buildEffectiveFallbackPaths(config.ResponseContentJsonPath, config.ResponseContentFallbackJsonPaths))
	ctx.SetContext(responseStreamFallbackPathsCtxKey, buildEffectiveFallbackPaths(config.ResponseStreamContentJsonPath, config.ResponseStreamContentFallbackJsonPaths))
	sessionID, _ := utils.GenerateHexID(20)
	ctx.SetContext("sessionID", sessionID)
	if strings.Contains(contentType, "text/event-stream") {
		ctx.NeedPauseStreamingResponse()
		return types.ActionContinue
	} else {
		ctx.BufferResponseBody()
		return types.HeaderStopIteration
	}
}

func HandleTextGenerationStreamingResponseBody(ctx wrapper.HttpContext, config cfg.AISecurityConfig, data []byte, endOfStream bool) []byte {
	consumer, _ := ctx.GetContext("consumer").(string)
	streamFallbackPaths := getEffectiveFallbackPathsFromContext(ctx, responseStreamFallbackPathsCtxKey, config.ResponseStreamContentJsonPath, config.ResponseStreamContentFallbackJsonPaths)
	var sessionID string
	if ctx.GetContext("sessionID") == nil {
		sessionID, _ = utils.GenerateHexID(20)
		ctx.SetContext("sessionID", sessionID)
	} else {
		sessionID, _ = ctx.GetContext("sessionID").(string)
	}
	var bufferQueue [][]byte
	var singleCall func()
	callback := func(statusCode int, responseHeaders http.Header, responseBody []byte) {
		log.Info(string(responseBody))
		if statusCode != 200 || gjson.GetBytes(responseBody, "Code").Int() != 200 {
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
			if ctx.GetContext("end_of_stream_received").(bool) {
				proxywasm.ResumeHttpResponse()
			}
			ctx.SetContext("during_call", false)
			return
		}
		if !cfg.IsRiskLevelAcceptable(config.Action, response.Data, config, consumer) {
			denyBody, err := cfg.BuildDenyResponseBody(response, config, consumer)
			if err != nil {
				log.Errorf("failed to build deny response body: %v", err)
				endStream := ctx.GetContext("end_of_stream_received").(bool) && ctx.BufferQueueSize() == 0
				proxywasm.InjectEncodedDataToFilterChain(bytes.Join(bufferQueue, []byte("")), endStream)
				bufferQueue = [][]byte{}
				if !endStream {
					ctx.SetContext("during_call", false)
					singleCall()
				}
				return
			}
			marshalledDenyMessage := wrapper.MarshalStr(string(denyBody))
			randomID := utils.GenerateRandomChatID()
			jsonData := []byte(fmt.Sprintf(cfg.OpenAIStreamResponseFormat, randomID, marshalledDenyMessage, randomID))
			proxywasm.InjectEncodedDataToFilterChain(jsonData, true)
			return
		}
		endStream := ctx.GetContext("end_of_stream_received").(bool) && ctx.BufferQueueSize() == 0
		proxywasm.InjectEncodedDataToFilterChain(bytes.Join(bufferQueue, []byte("")), endStream)
		bufferQueue = [][]byte{}
		if !endStream {
			ctx.SetContext("during_call", false)
			singleCall()
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
				bufferQueue = append(bufferQueue, front)
				msg := gjson.GetBytes(front, config.ResponseStreamContentJsonPath).String()
				if len(msg) == 0 {
					msg = autoExtractStreamingResponseContent(front, streamFallbackPaths)
				}
				buffer += msg
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
			checkService := config.GetResponseCheckService(consumer)
			path, headers, body := common.GenerateRequestForText(config, config.Action, checkService, buffer, sessionID)
			err := config.Client.Post(path, headers, body, callback, config.Timeout)
			if err != nil {
				log.Errorf("failed call the safe check service: %v", err)
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

func HandleTextGenerationResponseBody(ctx wrapper.HttpContext, config cfg.AISecurityConfig, body []byte) types.Action {
	consumer, _ := ctx.GetContext("consumer").(string)
	responseFallbackPaths := getEffectiveFallbackPathsFromContext(ctx, responseFallbackPathsCtxKey, config.ResponseContentJsonPath, config.ResponseContentFallbackJsonPaths)
	streamFallbackPaths := getEffectiveFallbackPathsFromContext(ctx, responseStreamFallbackPathsCtxKey, config.ResponseStreamContentJsonPath, config.ResponseStreamContentFallbackJsonPaths)
	log.Debugf("checking response body...")
	startTime := time.Now().UnixMilli()
	contentType, _ := proxywasm.GetHttpResponseHeader("content-type")
	isStreamingResponse := strings.Contains(contentType, "event-stream")
	var content string
	if isStreamingResponse {
		content = utils.ExtractMessageFromStreamingBody(body, config.ResponseStreamContentJsonPath)
		if len(content) == 0 {
			content = autoExtractStreamingResponseFromSSE(body, streamFallbackPaths)
		}
	} else {
		content = gjson.GetBytes(body, config.ResponseContentJsonPath).String()
		if len(content) == 0 {
			content = autoExtractResponseContent(body, responseFallbackPaths)
		}
	}
	log.Debugf("Raw response content is: %s", content)
	if len(content) == 0 {
		log.Info("response content is empty. skip")
		return types.ActionContinue
	}
	contentIndex := 0
	sessionID, _ := utils.GenerateHexID(20)
	var singleCall func()
	callback := func(statusCode int, responseHeaders http.Header, responseBody []byte) {
		log.Info(string(responseBody))
		if statusCode != 200 || gjson.GetBytes(responseBody, "Code").Int() != 200 {
			proxywasm.ResumeHttpResponse()
			return
		}
		var response cfg.Response
		err := json.Unmarshal(responseBody, &response)
		if err != nil {
			log.Error("failed to unmarshal aliyun content security response at response phase")
			proxywasm.ResumeHttpResponse()
			return
		}
		if cfg.IsRiskLevelAcceptable(config.Action, response.Data, config, consumer) {
			if contentIndex >= len(content) {
				endTime := time.Now().UnixMilli()
				ctx.SetUserAttribute("safecheck_response_rt", endTime-startTime)
				ctx.SetUserAttribute("safecheck_status", "response pass")
				ctx.WriteUserAttributeToLogWithKey(wrapper.AILogKey)
				proxywasm.ResumeHttpResponse()
			} else {
				singleCall()
			}
			return
		}
		denyBody, err := cfg.BuildDenyResponseBody(response, config, consumer)
		if err != nil {
			log.Errorf("failed to build deny response body: %v", err)
			proxywasm.ResumeHttpResponse()
			return
		}
		if config.ProtocolOriginal {
			proxywasm.SendHttpResponse(uint32(config.DenyCode), [][2]string{{"content-type", "application/json"}}, denyBody, -1)
		} else if isStreamingResponse {
			randomID := utils.GenerateRandomChatID()
			marshalledDenyMessage := wrapper.MarshalStr(string(denyBody))
			jsonData := []byte(fmt.Sprintf(cfg.OpenAIStreamResponseFormat, randomID, marshalledDenyMessage, randomID))
			proxywasm.SendHttpResponse(uint32(config.DenyCode), [][2]string{{"content-type", "text/event-stream;charset=UTF-8"}}, jsonData, -1)
		} else {
			randomID := utils.GenerateRandomChatID()
			marshalledDenyMessage := wrapper.MarshalStr(string(denyBody))
			jsonData := []byte(fmt.Sprintf(cfg.OpenAIResponseFormat, randomID, marshalledDenyMessage))
			proxywasm.SendHttpResponse(uint32(config.DenyCode), [][2]string{{"content-type", "application/json"}}, jsonData, -1)
		}
		config.IncrementCounter("ai_sec_response_deny", 1)
		endTime := time.Now().UnixMilli()
		ctx.SetUserAttribute("safecheck_response_rt", endTime-startTime)
		ctx.SetUserAttribute("safecheck_status", "response deny")
		if response.Data.Advice != nil {
			ctx.SetUserAttribute("safecheck_riskLabel", response.Data.Result[0].Label)
			ctx.SetUserAttribute("safecheck_riskWords", response.Data.Result[0].RiskWords)
		}
		ctx.WriteUserAttributeToLogWithKey(wrapper.AILogKey)
	}
	singleCall = func() {
		var nextContentIndex int
		if contentIndex+cfg.LengthLimit >= len(content) {
			nextContentIndex = len(content)
		} else {
			nextContentIndex = contentIndex + cfg.LengthLimit
		}
		contentPiece := content[contentIndex:nextContentIndex]
		contentIndex = nextContentIndex
		log.Debugf("current content piece: %s", contentPiece)
		checkService := config.GetResponseCheckService(consumer)
		path, headers, body := common.GenerateRequestForText(config, config.Action, checkService, contentPiece, sessionID)
		err := config.Client.Post(path, headers, body, callback, config.Timeout)
		if err != nil {
			log.Errorf("failed call the safe check service: %v", err)
			proxywasm.ResumeHttpResponse()
		}
	}
	singleCall()
	return types.ActionPause
}

// autoExtractResponseContent tries configured fallback paths to extract text content.
func autoExtractResponseContent(body []byte, fallbackPaths []string) string {
	if len(fallbackPaths) == 0 {
		return ""
	}
	parsed := gjson.ParseBytes(body)
	return extractTextByPaths(parsed, fallbackPaths)
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
	parsed := gjson.ParseBytes(payload)
	return extractTextByPaths(parsed, fallbackPaths)
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
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if len(path) == 0 {
			continue
		}
		result := parsed.Get(path)
		if !result.Exists() {
			continue
		}
		if text := extractTextFromResult(result); len(text) > 0 {
			log.Debugf("response fallback path matched: %s", path)
			return text
		}
	}
	return ""
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
