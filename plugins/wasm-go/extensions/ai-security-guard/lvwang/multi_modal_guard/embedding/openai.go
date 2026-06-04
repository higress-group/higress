package embedding

import (
	"encoding/json"
	"fmt"
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

// OpenAI Embedding error response format
const EmbeddingErrorResponseFormat = `{"error": {"message": "%s", "type": "invalid_request_error", "param": null, "code": "content_policy_violation"}}`

// parseInput extracts text from the input field of an Embedding request.
// input can be:
// - A string: returns the string directly
// - An array of strings: returns all strings joined
// - An array of integers (token IDs): returns empty with unsupportedType=true
func parseInput(json gjson.Result) (text string, unsupportedType bool) {
	if json.IsArray() {
		// Check if it's an array of strings or token IDs
		arr := json.Array()
		if len(arr) == 0 {
			return "", false
		}

		// Check first element type
		if arr[0].Type == gjson.String {
			// Array of strings
			var texts []string
			for _, item := range arr {
				if item.Type == gjson.String {
					texts = append(texts, item.String())
				}
			}
			return joinTexts(texts), false
		} else if arr[0].Type == gjson.Number {
			// Array of token IDs - not supported for text detection
			log.Info("embedding input is token ID array, not supported for text detection")
			return "", true
		}
	} else if json.Type == gjson.String {
		// Single string
		return json.String(), false
	}

	// Unknown type
	log.Warnf("embedding input has unsupported type: %v", json.Type)
	return "", true
}

// joinTexts joins multiple text strings with newline separator
func joinTexts(texts []string) string {
	result := ""
	for i, t := range texts {
		if i > 0 {
			result += "\n"
		}
		result += t
	}
	return result
}

// structuralFields contains field names that should be skipped when extracting text content
// These are structural/metadata fields, not user content
var structuralFields = map[string]bool{
	"object":    true, // JSON structure identifier
	"model":     true, // Model name
	"index":     true, // Array index marker
	"encoding":  true, // Encoding format
	"id":        true, // Response ID
	"requestId": true, // Request ID
}

// extractStringLeaves recursively extracts string values from a JSON structure
// Skips structural/metadata fields that are not user content
func extractStringLeaves(json gjson.Result, texts *[]string) {
	if json.Type == gjson.String {
		*texts = append(*texts, json.String())
		return
	}

	if json.IsArray() {
		for _, item := range json.Array() {
			extractStringLeaves(item, texts)
		}
		return
	}

	if json.IsObject() {
		json.ForEach(func(key, value gjson.Result) bool {
			// Skip structural/metadata fields
			if structuralFields[key.String()] {
				return true
			}
			// Skip embedding vectors (numeric arrays or base64 strings)
			if key.String() == "embedding" {
				return true
			}
			extractStringLeaves(value, texts)
			return true
		})
	}
}

// HandleEmbeddingRequestBody handles request body for Embedding API
func HandleEmbeddingRequestBody(ctx wrapper.HttpContext, config cfg.AISecurityConfig, body []byte) types.Action {
	consumer, _ := ctx.GetContext("consumer").(string)
	checkService := config.GetRequestCheckService(consumer)
	startTime := time.Now().UnixMilli()

	// Extract text from input field
	input := gjson.GetBytes(body, config.RequestContentJsonPath)
	content, unsupportedType := parseInput(input)

	log.Debugf("Embedding request content: %s, unsupportedType: %v", content, unsupportedType)

	// Handle unsupported input types (e.g., token ID arrays)
	if unsupportedType {
		log.Info("embedding request has unsupported input type, skipping text detection")
		ctx.SetUserAttribute("safecheck_status", "request skip - unsupported input type")
		ctx.WriteUserAttributeToLogWithKey(wrapper.AILogKey)
		return types.ActionContinue
	}

	if len(content) == 0 {
		log.Info("embedding request content is empty, skip")
		return types.ActionContinue
	}

	contentIndex := 0
	sessionID, _ := utils.GenerateHexID(20)
	var singleCall func()

	callback := func(statusCode int, responseHeaders http.Header, responseBody []byte) {
		log.Info(string(responseBody))
		if statusCode != 200 || gjson.GetBytes(responseBody, "Code").Int() != 200 {
			proxywasm.ResumeHttpRequest()
			return
		}

		var response cfg.Response
		err := json.Unmarshal(responseBody, &response)
		if err != nil {
			log.Errorf("%+v", err)
			proxywasm.ResumeHttpRequest()
			return
		}

		if cfg.IsRiskLevelAcceptable(config.Action, response.Data, config, consumer) {
			if contentIndex >= len(content) {
				endTime := time.Now().UnixMilli()
				ctx.SetUserAttribute("safecheck_request_rt", endTime-startTime)
				ctx.SetUserAttribute("safecheck_status", "request pass")
				ctx.WriteUserAttributeToLogWithKey(wrapper.AILogKey)
				proxywasm.ResumeHttpRequest()
			} else {
				singleCall()
			}
			return
		}

		// Risk detected - send Embedding-compatible error response
		denyBody, err := cfg.BuildDenyResponseBody(response, config, consumer)
		if err != nil {
			log.Errorf("failed to build deny response body: %v", err)
			proxywasm.ResumeHttpRequest()
			return
		}

		// Use Embedding-specific error response format
		marshalledDenyMessage := wrapper.MarshalStr(string(denyBody))
		jsonData := []byte(fmt.Sprintf(EmbeddingErrorResponseFormat, marshalledDenyMessage))
		proxywasm.SendHttpResponse(uint32(config.DenyCode), [][2]string{{"content-type", "application/json"}}, jsonData, -1)

		ctx.DontReadResponseBody()
		config.IncrementCounter("ai_sec_request_deny", 1)
		endTime := time.Now().UnixMilli()
		ctx.SetUserAttribute("safecheck_request_rt", endTime-startTime)
		ctx.SetUserAttribute("safecheck_status", "request deny")
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
		path, headers, body := common.GenerateRequestForText(config, cfg.MultiModalGuard, checkService, contentPiece, sessionID)
		err := config.Client.Post(path, headers, body, callback, config.Timeout)
		if err != nil {
			log.Errorf("failed call the safe check service: %v", err)
			proxywasm.ResumeHttpRequest()
		}
	}

	singleCall()
	return types.ActionPause
}

// HandleEmbeddingResponseHeaders handles response headers for Embedding API
func HandleEmbeddingResponseHeaders(ctx wrapper.HttpContext, config cfg.AISecurityConfig) types.Action {
	ctx.BufferResponseBody()
	return types.HeaderStopIteration
}

// HandleEmbeddingResponseBody handles response body for Embedding API
func HandleEmbeddingResponseBody(ctx wrapper.HttpContext, config cfg.AISecurityConfig, body []byte) types.Action {
	consumer, _ := ctx.GetContext("consumer").(string)
	log.Debugf("checking embedding response body...")
	startTime := time.Now().UnixMilli()

	// Priority 1: Check error.message for error responses
	var content string
	if config.ResponseErrorContentJsonPath != "" {
		content = gjson.GetBytes(body, config.ResponseErrorContentJsonPath).String()
	}

	// Priority 2: Extract string leaves from data field
	if len(content) == 0 {
		data := gjson.GetBytes(body, config.ResponseContentJsonPath)
		var texts []string
		extractStringLeaves(data, &texts)
		if len(texts) > 0 {
			content = joinTexts(texts)
		}
	}

	log.Debugf("Embedding response content length: %d", len(content))

	if len(content) == 0 {
		// No text found - this is normal for standard embedding responses that only contain vectors
		log.Info("embedding response has no text content (likely vector-only response), skipping text detection")
		ctx.SetUserAttribute("safecheck_status", "response skip - no text content")
		ctx.WriteUserAttributeToLogWithKey(wrapper.AILogKey)
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

		// Risk detected - send Embedding-compatible error response
		denyBody, err := cfg.BuildDenyResponseBody(response, config, consumer)
		if err != nil {
			log.Errorf("failed to build deny response body: %v", err)
			proxywasm.ResumeHttpResponse()
			return
		}

		// Use Embedding-specific error response format
		marshalledDenyMessage := wrapper.MarshalStr(string(denyBody))
		jsonData := []byte(fmt.Sprintf(EmbeddingErrorResponseFormat, marshalledDenyMessage))
		proxywasm.SendHttpResponse(uint32(config.DenyCode), [][2]string{{"content-type", "application/json"}}, jsonData, -1)

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