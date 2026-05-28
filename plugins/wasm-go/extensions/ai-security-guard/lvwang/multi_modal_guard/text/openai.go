package text

import (
	"encoding/json"
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

type ImageItem struct {
	Content string
	Type    string // URL or BASE64
}

func parseContent(json gjson.Result) (text string, images []ImageItem) {
	images = []ImageItem{}
	if json.IsArray() {
		for _, item := range json.Array() {
			switch item.Get("type").String() {
			case "text":
				text += item.Get("text").String()
			case "image_url":
				imgContent := item.Get("image_url.url").String()
				if strings.HasPrefix(imgContent, "data:image") {
					images = append(images, ImageItem{
						Content: imgContent,
						Type:    "BASE64",
					})
				} else {
					images = append(images, ImageItem{
						Content: imgContent,
						Type:    "URL",
					})
				}
			}
		}
	} else {
		text = json.String()
	}
	return text, images
}

func HandleTextGenerationRequestBody(ctx wrapper.HttpContext, config cfg.AISecurityConfig, body []byte) types.Action {
	consumer, _ := ctx.GetContext("consumer").(string)
	checkService := config.GetRequestCheckService(consumer)
	checkImageService := config.GetRequestImageCheckService(consumer)
	startTime := time.Now().UnixMilli()
	// content := gjson.GetBytes(body, config.RequestContentJsonPath).String()
	content, images := parseContent(gjson.GetBytes(body, config.RequestContentJsonPath))
	log.Debugf("Raw request content is: %s", content)
	if len(content) == 0 && len(images) == 0 {
		log.Info("request content is empty. skip")
		return types.ActionContinue
	}
	contentIndex := 0
	imageIndex := 0
	hasMasked := false
	maskedContent := []byte(content)
	sessionID, _ := utils.GenerateHexID(20)
	currentSubmissionIndex := 0
	currentImageSubmissionIndex := 0
	var singleCall func()
	var singleCallForImage func()
	// prevContentIndex tracks the start of the current chunk for masking replacement
	prevContentIndex := 0
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
			log.Errorf("%+v", err)
			cfg.MarkGuardrailRequestError(ctx, currentSubmissionIndex, responseBody, startTime)
			proxywasm.ResumeHttpRequest()
			return
		}
		riskResult := cfg.EvaluateRisk(config.Action, response.Data, config, consumer)
		proxywasm.LogInfof("safecheck_resolved_action=%v", riskResult)
		switch riskResult {
		case cfg.RiskPass:
			if contentIndex >= len(maskedContent) {
				endTime := time.Now().UnixMilli()
				ctx.SetUserAttribute("safecheck_request_rt", endTime-startTime)
				if hasMasked {
					// All chunks processed, some had masking — replace the content in request body
					newBody, replaceErr := utils.ReplaceJsonFieldTextContent(body, config.RequestContentJsonPath, string(maskedContent))
					if replaceErr != nil {
						log.Errorf("failed to replace request body content, falling back to block: %v", replaceErr)
						// Fall back to block to prevent leaking sensitive data
						if sendErr := cfg.SendFallbackDenyResponse(config, gjson.GetBytes(body, "stream").Bool()); sendErr != nil {
							log.Errorf("failed to build deny response body: %v", sendErr)
							cfg.MarkGuardrailRequestError(ctx, currentSubmissionIndex, responseBody, startTime)
							proxywasm.ResumeHttpRequest()
							return
						}
						ctx.DontReadResponseBody()
						config.IncrementCounter("ai_sec_request_deny", 1)
						ctx.SetUserAttribute("safecheck_status", "reqeust deny")
						cfg.CompleteGuardrailSubmissionEvent(ctx, currentSubmissionIndex, responseBody, cfg.GuardrailResultDeny)
						cfg.WriteGuardrailLog(ctx)
						return
					}
					proxywasm.ReplaceHttpRequestBody(newBody)
					config.IncrementCounter("ai_sec_request_mask", 1)
					ctx.SetUserAttribute("safecheck_status", "request mask")
				} else {
					ctx.SetUserAttribute("safecheck_status", "request pass")
				}
			}
			cfg.CompleteGuardrailSubmissionEvent(ctx, currentSubmissionIndex, responseBody, cfg.GuardrailResultPass)
			if contentIndex >= len(maskedContent) {
				if len(images) > 0 && config.CheckRequestImage {
					singleCallForImage()
				} else {
					cfg.WriteGuardrailLog(ctx)
					proxywasm.ResumeHttpRequest()
				}
			} else {
				singleCall()
			}
			return
		case cfg.RiskMask:
			desensitization := cfg.ExtractDesensitization(response.Data)
			if desensitization == "" {
				proxywasm.LogInfof("safecheck_action_source=mask_fallback_to_block, reason=empty_desensitization")
				log.Warnf("desensitization content is empty, falling back to block logic")
				// Keep this fallback separate from RiskBlock: legacy reuses the
				// original deny body in content, while structured emits an empty
				// fallback guardrail object.
				isStream := gjson.GetBytes(body, "stream").Bool()
				var sendErr error
				if !config.ProtocolOriginal && config.OpenAIDenyResponseFormat != cfg.OpenAIDenyResponseFormatStructured {
					sendErr = cfg.SendDenyResponse(config, response, consumer, isStream)
				} else {
					sendErr = cfg.SendFallbackDenyResponse(config, isStream)
				}
				if sendErr != nil {
					log.Errorf("failed to build deny response body: %v", sendErr)
					cfg.MarkGuardrailRequestError(ctx, currentSubmissionIndex, responseBody, startTime)
					proxywasm.ResumeHttpRequest()
					return
				}
				ctx.DontReadResponseBody()
				config.IncrementCounter("ai_sec_request_deny", 1)
				endTime := time.Now().UnixMilli()
				ctx.SetUserAttribute("safecheck_request_rt", endTime-startTime)
				ctx.SetUserAttribute("safecheck_status", "reqeust deny")
				cfg.CompleteGuardrailSubmissionEvent(ctx, currentSubmissionIndex, responseBody, cfg.GuardrailResultDeny)
				cfg.WriteGuardrailLog(ctx)
				return
			} else {
				// Replace only the current chunk portion in maskedContent
				chunkStart := prevContentIndex
				chunkEnd := contentIndex
				maskedContent = append(maskedContent[:chunkStart], append([]byte(desensitization), maskedContent[chunkEnd:]...)...)
				// Adjust contentIndex for the length difference
				lengthDiff := len(desensitization) - (chunkEnd - chunkStart)
				contentIndex += lengthDiff
				hasMasked = true
				// Continue checking remaining chunks
				if contentIndex >= len(maskedContent) {
					// All chunks done, apply the masked content
					newBody, replaceErr := utils.ReplaceJsonFieldTextContent(body, config.RequestContentJsonPath, string(maskedContent))
					if replaceErr != nil {
						log.Errorf("failed to replace request body content, falling back to block: %v", replaceErr)
						// Fall back to block to prevent leaking sensitive data
						if sendErr := cfg.SendFallbackDenyResponse(config, gjson.GetBytes(body, "stream").Bool()); sendErr != nil {
							log.Errorf("failed to build deny response body: %v", sendErr)
							cfg.MarkGuardrailRequestError(ctx, currentSubmissionIndex, responseBody, startTime)
							proxywasm.ResumeHttpRequest()
							return
						}
						ctx.DontReadResponseBody()
						config.IncrementCounter("ai_sec_request_deny", 1)
						endTime := time.Now().UnixMilli()
						ctx.SetUserAttribute("safecheck_request_rt", endTime-startTime)
						ctx.SetUserAttribute("safecheck_status", "reqeust deny")
						cfg.CompleteGuardrailSubmissionEvent(ctx, currentSubmissionIndex, responseBody, cfg.GuardrailResultDeny)
						cfg.WriteGuardrailLog(ctx)
						return
					}
					proxywasm.ReplaceHttpRequestBody(newBody)
					config.IncrementCounter("ai_sec_request_mask", 1)
					endTime := time.Now().UnixMilli()
					ctx.SetUserAttribute("safecheck_request_rt", endTime-startTime)
					ctx.SetUserAttribute("safecheck_status", "request mask")
					cfg.CompleteGuardrailSubmissionEvent(ctx, currentSubmissionIndex, responseBody, cfg.GuardrailResultMask)
					if len(images) > 0 && config.CheckRequestImage {
						singleCallForImage()
					} else {
						cfg.WriteGuardrailLog(ctx)
						proxywasm.ResumeHttpRequest()
					}
				} else {
					cfg.CompleteGuardrailSubmissionEvent(ctx, currentSubmissionIndex, responseBody, cfg.GuardrailResultMask)
					singleCall()
				}
				return
			}
		case cfg.RiskBlock:
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
	}
	singleCall = func() {
		currentSubmissionIndex = cfg.BeginGuardrailSubmissionEvent(ctx, cfg.GuardrailPhaseRequest, cfg.GuardrailModalityText)
		prevContentIndex = contentIndex
		var nextContentIndex int
		if contentIndex+cfg.LengthLimit >= len(maskedContent) {
			nextContentIndex = len(maskedContent)
		} else {
			nextContentIndex = contentIndex + cfg.LengthLimit
		}
		contentPiece := string(maskedContent[contentIndex:nextContentIndex])
		contentIndex = nextContentIndex
		log.Debugf("current content piece: %s", contentPiece)
		path, headers, body := common.GenerateRequestForText(config, cfg.MultiModalGuard, checkService, contentPiece, sessionID)
		err := config.Client.Post(path, headers, body, callback, config.Timeout)
		if err != nil {
			log.Errorf("failed call the safe check service: %v", err)
			cfg.MarkGuardrailRequestError(ctx, currentSubmissionIndex, nil, startTime)
			proxywasm.ResumeHttpRequest()
		}
	}

	callbackForImage := func(statusCode int, responseHeaders http.Header, responseBody []byte) {
		imageIndex += 1
		log.Info(string(responseBody))
		if statusCode != 200 || gjson.GetBytes(responseBody, "Code").Int() != 200 {
			cfg.MarkGuardrailRequestError(ctx, currentImageSubmissionIndex, responseBody, startTime)
			if imageIndex < len(images) {
				singleCallForImage()
			} else {
				proxywasm.ResumeHttpRequest()
			}
			return
		}
		var response cfg.Response
		err := json.Unmarshal(responseBody, &response)
		if err != nil {
			log.Errorf("%+v", err)
			cfg.MarkGuardrailRequestError(ctx, currentImageSubmissionIndex, responseBody, startTime)
			if imageIndex < len(images) {
				singleCallForImage()
			} else {
				proxywasm.ResumeHttpRequest()
			}
			return
		}
		endTime := time.Now().UnixMilli()
		if cfg.IsRiskLevelAcceptable(config.Action, response.Data, config, consumer) {
			if imageIndex >= len(images) {
				ctx.SetUserAttribute("safecheck_request_rt", endTime-startTime)
				ctx.SetUserAttribute("safecheck_status", "request pass")
			}
			cfg.CompleteGuardrailSubmissionEvent(ctx, currentImageSubmissionIndex, responseBody, cfg.GuardrailResultPass)
			if imageIndex >= len(images) {
				cfg.WriteGuardrailLog(ctx)
				proxywasm.ResumeHttpRequest()
			} else {
				singleCallForImage()
			}
			return
		}

		if err := cfg.SendDenyResponse(config, response, consumer, gjson.GetBytes(body, "stream").Bool()); err != nil {
			log.Errorf("failed to build deny response body: %v", err)
			cfg.MarkGuardrailRequestError(ctx, currentImageSubmissionIndex, responseBody, startTime)
			proxywasm.ResumeHttpRequest()
			return
		}
		ctx.DontReadResponseBody()
		config.IncrementCounter("ai_sec_request_deny", 1)
		ctx.SetUserAttribute("safecheck_request_rt", endTime-startTime)
		ctx.SetUserAttribute("safecheck_status", "reqeust deny")
		if len(response.Data.Result) > 0 {
			ctx.SetUserAttribute("safecheck_riskLabel", response.Data.Result[0].Label)
			ctx.SetUserAttribute("safecheck_riskWords", response.Data.Result[0].RiskWords)
		}
		cfg.CompleteGuardrailSubmissionEvent(ctx, currentImageSubmissionIndex, responseBody, cfg.GuardrailResultDeny)
		cfg.WriteGuardrailLog(ctx)
	}
	singleCallForImage = func() {
		currentImageSubmissionIndex = cfg.BeginGuardrailSubmissionEvent(ctx, cfg.GuardrailPhaseRequest, cfg.GuardrailModalityImage)
		img := images[imageIndex]
		imgUrl := ""
		imgBase64 := ""
		if img.Type == "BASE64" {
			imgBase64 = img.Content
		} else {
			imgUrl = img.Content
		}
		path, headers, body := common.GenerateRequestForImage(config, cfg.MultiModalGuardForBase64, checkImageService, imgUrl, imgBase64)
		err := config.Client.Post(path, headers, body, callbackForImage, config.Timeout)
		if err != nil {
			log.Errorf("failed call the safe check service: %v", err)
			cfg.MarkGuardrailRequestError(ctx, currentImageSubmissionIndex, nil, startTime)
			proxywasm.ResumeHttpRequest()
		}
	}
	if len(content) > 0 {
		singleCall()
	} else {
		singleCallForImage()
	}
	return types.ActionPause
}
