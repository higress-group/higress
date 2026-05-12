package image

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

func parseOpenAIRequest(body []byte) (text string, images []ImageItem) {
	text = gjson.GetBytes(body, "prompt").String()
	return text, images
}

func parseOpenAIResponse(body []byte) []ImageItem {
	// qwen api: https://bailian.console.aliyun.com/?tab=api#/api/?type=model&url=2975126
	result := []ImageItem{}
	for _, part := range gjson.GetBytes(body, "data").Array() {
		if url := part.Get("url").String(); url != "" {
			result = append(result, ImageItem{
				Content: url,
				Type:    "URL",
			})
		}
		if b64 := part.Get("b64_json").String(); b64 != "" {
			result = append(result, ImageItem{
				Content: b64,
				Type:    "BASE64",
			})
		}
	}
	return result
}

func HandleOpenAIImageGenerationRequestBody(ctx wrapper.HttpContext, config cfg.AISecurityConfig, body []byte) types.Action {
	consumer, _ := ctx.GetContext("consumer").(string)
	checkService := config.GetRequestCheckService(consumer)
	checkImageService := config.GetRequestImageCheckService(consumer)
	startTime := time.Now().UnixMilli()
	content, images := parseOpenAIRequest(body)
	log.Debugf("Raw request content is: %s", content)
	if len(content) == 0 && len(images) == 0 {
		log.Info("request content is empty. skip")
		return types.ActionContinue
	}
	contentIndex := 0
	imageIndex := 0
	sessionID, _ := utils.GenerateHexID(20)
	var singleCall func()
	var singleCallForImage func()
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
				if len(images) > 0 && config.CheckRequestImage {
					singleCallForImage()
				} else {
					proxywasm.ResumeHttpRequest()
				}
			} else {
				singleCall()
			}
			return
		}
		denyBody, err := cfg.BuildDenyResponseBody(response, config, consumer)
		if err != nil {
			log.Errorf("failed to build deny response body: %v", err)
			proxywasm.ResumeHttpRequest()
			return
		}
		proxywasm.SendHttpResponse(403, [][2]string{{"content-type", "application/json"}}, denyBody, -1)
		ctx.DontReadResponseBody()
		config.IncrementCounter("ai_sec_request_deny", 1)
		endTime := time.Now().UnixMilli()
		ctx.SetUserAttribute("safecheck_request_rt", endTime-startTime)
		ctx.SetUserAttribute("safecheck_status", "reqeust deny")
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

	callbackForImage := func(statusCode int, responseHeaders http.Header, responseBody []byte) {
		imageIndex += 1
		log.Info(string(responseBody))
		if statusCode != 200 || gjson.GetBytes(responseBody, "Code").Int() != 200 {
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
				ctx.WriteUserAttributeToLogWithKey(wrapper.AILogKey)
				proxywasm.ResumeHttpRequest()
			} else {
				singleCallForImage()
			}
			return
		}

		denyBody, err := cfg.BuildDenyResponseBody(response, config, consumer)
		if err != nil {
			log.Errorf("failed to build deny response body: %v", err)
			proxywasm.ResumeHttpRequest()
			return
		}
		proxywasm.SendHttpResponse(403, [][2]string{{"content-type", "application/json"}}, denyBody, -1)
		ctx.DontReadResponseBody()
		config.IncrementCounter("ai_sec_request_deny", 1)
		ctx.SetUserAttribute("safecheck_request_rt", endTime-startTime)
		ctx.SetUserAttribute("safecheck_status", "reqeust deny")
		if response.Data.Advice != nil {
			ctx.SetUserAttribute("safecheck_riskLabel", response.Data.Result[0].Label)
			ctx.SetUserAttribute("safecheck_riskWords", response.Data.Result[0].RiskWords)
		}
		ctx.WriteUserAttributeToLogWithKey(wrapper.AILogKey)
	}
	singleCallForImage = func() {
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

func HandleOpenAIImageGenerationResponseBody(ctx wrapper.HttpContext, config cfg.AISecurityConfig, body []byte) types.Action {
	consumer, _ := ctx.GetContext("consumer").(string)
	log.Debugf("checking response body...")
	checkImageService := config.GetResponseImageCheckService(consumer)
	startTime := time.Now().UnixMilli()
	imgResults := parseOpenAIResponse(body)
	if len(imgResults) == 0 {
		return types.ActionContinue
	}
	imageIndex := 0
	var singleCall func()
	callback := func(statusCode int, responseHeaders http.Header, responseBody []byte) {
		imageIndex += 1
		log.Info(string(responseBody))
		if statusCode != 200 || gjson.GetBytes(responseBody, "Code").Int() != 200 {
			if imageIndex < len(imgResults) {
				singleCall()
			} else {
				proxywasm.ResumeHttpResponse()
			}
			return
		}
		var response cfg.Response
		err := json.Unmarshal(responseBody, &response)
		if err != nil {
			log.Errorf("%+v", err)
			if imageIndex < len(imgResults) {
				singleCall()
			} else {
				proxywasm.ResumeHttpResponse()
			}
			return
		}
		endTime := time.Now().UnixMilli()
		if cfg.IsRiskLevelAcceptable(config.Action, response.Data, config, consumer) {
			if imageIndex >= len(imgResults) {
				ctx.SetUserAttribute("safecheck_request_rt", endTime-startTime)
				ctx.SetUserAttribute("safecheck_status", "request pass")
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
		proxywasm.SendHttpResponse(403, [][2]string{{"content-type", "application/json"}}, denyBody, -1)
		config.IncrementCounter("ai_sec_request_deny", 1)
		ctx.SetUserAttribute("safecheck_request_rt", endTime-startTime)
		ctx.SetUserAttribute("safecheck_status", "reqeust deny")
		ctx.WriteUserAttributeToLogWithKey(wrapper.AILogKey)
	}
	singleCall = func() {
		img := imgResults[imageIndex]
		imgUrl := ""
		imgBase64 := ""
		if img.Type == "BASE64" {
			imgBase64 = img.Content
		} else {
			imgUrl = img.Content
		}
		path, headers, body := common.GenerateRequestForImage(config, cfg.MultiModalGuardForBase64, checkImageService, imgUrl, imgBase64)
		err := config.Client.Post(path, headers, body, callback, config.Timeout)
		if err != nil {
			log.Errorf("failed call the safe check service: %v", err)
			proxywasm.ResumeHttpResponse()
		}
	}
	singleCall()
	return types.ActionPause
}
