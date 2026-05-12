package image

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

func parseImage(body []byte, jsonPath string) *ImageItem {
	if gjson.GetBytes(body, jsonPath).Exists() {
		imgContent := gjson.GetBytes(body, jsonPath).String()
		if strings.HasPrefix(imgContent, "data:image") {
			return &ImageItem{
				Content: imgContent,
				Type:    "BASE64",
			}
		} else {
			return &ImageItem{
				Content: imgContent,
				Type:    "URL",
			}
		}
	}
	return nil
}

func parseImageArray(body []byte, jsonPath string) []ImageItem {
	result := []ImageItem{}
	if gjson.GetBytes(body, jsonPath).Exists() {
		for _, item := range gjson.GetBytes(body, jsonPath).Array() {
			imgContent := item.String()
			if strings.HasPrefix(imgContent, "data:image") {
				result = append(result, ImageItem{
					Content: imgContent,
					Type:    "BASE64",
				})
			} else {
				result = append(result, ImageItem{
					Content: imgContent,
					Type:    "URL",
				})
			}
		}
	}
	return result
}

func parseQwenRequest(body []byte) (text string, images []ImageItem) {
	// qwen api: https://bailian.console.aliyun.com/?tab=api#/api/?type=model&url=2975126
	images = []ImageItem{}
	// 文生图/文生图v1/文生图v2
	if gjson.GetBytes(body, "input.prompt").Exists() {
		text += gjson.GetBytes(body, "input.prompt").String()
	}
	// 图像背景生成
	if gjson.GetBytes(body, "input.ref_prompt").Exists() {
		text += gjson.GetBytes(body, "input.ref_prompt").String()
	}
	if gjson.GetBytes(body, "input.reference_edge.foreground_edge_prompt").Exists() {
		for _, item := range gjson.GetBytes(body, "input.reference_edge.foreground_edge_prompt").Array() {
			text += item.String()
		}
	}
	if gjson.GetBytes(body, "input.reference_edge.background_edge_prompt").Exists() {
		for _, item := range gjson.GetBytes(body, "input.reference_edge.background_edge_prompt").Array() {
			text += item.String()
		}
	}
	// 创意文字
	if gjson.GetBytes(body, "input.text").Exists() {
		text += gjson.GetBytes(body, "input.text").String()
	}
	if gjson.GetBytes(body, "input.negative_prompt").Exists() {
		text += gjson.GetBytes(body, "input.negative_prompt").String()
	}
	// 图像编辑
	if gjson.GetBytes(body, "input.messages.0.content").Exists() {
		for _, item := range gjson.GetBytes(body, "input.messages.0.content").Array() {
			if item.Get("text").Exists() {
				text += item.Get("text").String()
			} else if item.Get("image").Exists() {
				imgContent := item.Get("image").String()
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
	}
	// image json path
	imageJsonPath := []string{
		"input.image_url",          // 图像翻译/人像风格重绘/图像画面扩展/人物实例分割/图像擦除补全
		"input.base_image_url",     // 通用图像编辑2.1/图像局部重绘/虚拟模特
		"input.mask_image_url",     // 通用图像编辑2.1/图像局部重绘/虚拟模特
		"input.sketch_image_url",   // 涂鸦作画
		"input.template_image_url", // 鞋靴模特
		"input.shoe_image_url",     // 鞋靴模特
		"input.base_image_url",     // 图像背景生成
		"input.ref_image_url",      // 图像背景生成
		"input.mask_url",           // 图像擦除补全
		"input.foreground_url",     // 图像擦除补全
		"input.person_image_url",   // AI试衣
		"input.top_garment_url",    // AI试衣
		"input.bottom_garment_url", // AI试衣
		"input.coarse_image_url",   // AI试衣
		"input.template_url",       // 人物写真生成
	}
	for _, jsonPath := range imageJsonPath {
		tmpImage := parseImage(body, jsonPath)
		if tmpImage != nil {
			images = append(images, *tmpImage)
		}
	}
	// image array json path
	imageArrayJsonPath := []string{
		"input.images",                         // 通用图像编辑2.5/人物图像检测
		"input.reference_edge.foreground_edge", // 图像背景生成
		"input.reference_edge.background_edge", // 图像背景生成
		"input.user_urls",                      // 人物写真生成
	}
	for _, jsonPath := range imageArrayJsonPath {
		tmpImageArray := parseImageArray(body, jsonPath)
		images = append(images, tmpImageArray...)
	}
	return text, images
}

func parseQwenResponse(body []byte) []string {
	// qwen api: https://bailian.console.aliyun.com/?tab=api#/api/?type=model&url=2975126
	result := []string{}
	// 文生图/文生图v1/文生图v2/通用图像编辑2.5/通用图像编辑2.1/涂鸦作画/图像局部重绘/人像风格重绘
	// 虚拟模特/图像背景生成/人物写真FaceChain/文生图StableDiffusion/文生图FLUX/文字纹理生成API
	for _, part := range gjson.GetBytes(body, "output.results").Array() {
		if url := part.Get("url").String(); url != "" {
			result = append(result, url)
		}
	}
	// 图像编辑
	for _, part := range gjson.GetBytes(body, "output.choices.0.message.content").Array() {
		if url := part.Get("image").String(); url != "" {
			result = append(result, url)
		}
	}
	// 图像翻译/AI试衣OutfitAnyone
	if url := gjson.GetBytes(body, "output.image_url").String(); url != "" {
		result = append(result, url)
	}
	// 图像画面扩展/(part of)人物实例分割/图像擦除补全
	if url := gjson.GetBytes(body, "output.output_image_url").String(); url != "" {
		result = append(result, url)
	}
	// 鞋靴模特
	if url := gjson.GetBytes(body, "output.result_url").String(); url != "" {
		result = append(result, url)
	}
	// 创意海报生成
	for _, part := range gjson.GetBytes(body, "output.render_urls").Array() {
		if url := part.String(); url != "" {
			result = append(result, url)
		}
	}
	for _, part := range gjson.GetBytes(body, "output.bg_urls").Array() {
		if url := part.String(); url != "" {
			result = append(result, url)
		}
	}
	// 人物实例分割
	if url := gjson.GetBytes(body, "output.output_vis_image_url").String(); url != "" {
		result = append(result, url)
	}
	// 文字变形API
	for _, part := range gjson.GetBytes(body, "output.results").Array() {
		if url := part.Get("png_url").String(); url != "" {
			result = append(result, url)
		}
		if url := part.Get("svg_url").String(); url != "" {
			result = append(result, url)
		}
	}
	return result
}

func HandleQwenImageGenerationRequestBody(ctx wrapper.HttpContext, config cfg.AISecurityConfig, body []byte) types.Action {
	consumer, _ := ctx.GetContext("consumer").(string)
	checkService := config.GetRequestCheckService(consumer)
	checkImageService := config.GetRequestImageCheckService(consumer)
	startTime := time.Now().UnixMilli()
	// content := gjson.GetBytes(body, config.RequestContentJsonPath).String()
	content, images := parseQwenRequest(body)
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

func HandleQwenImageGenerationResponseBody(ctx wrapper.HttpContext, config cfg.AISecurityConfig, body []byte) types.Action {
	consumer, _ := ctx.GetContext("consumer").(string)
	log.Debugf("checking response body...")
	checkImageService := config.GetResponseImageCheckService(consumer)
	startTime := time.Now().UnixMilli()
	imgUrls := parseQwenResponse(body)
	if len(imgUrls) == 0 {
		return types.ActionContinue
	}
	imageIndex := 0
	var singleCall func()
	callback := func(statusCode int, responseHeaders http.Header, responseBody []byte) {
		imageIndex += 1
		log.Info(string(responseBody))
		if statusCode != 200 || gjson.GetBytes(responseBody, "Code").Int() != 200 {
			if imageIndex < len(imgUrls) {
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
			if imageIndex < len(imgUrls) {
				singleCall()
			} else {
				proxywasm.ResumeHttpResponse()
			}
			return
		}
		endTime := time.Now().UnixMilli()
		if cfg.IsRiskLevelAcceptable(config.Action, response.Data, config, consumer) {
			if imageIndex >= len(imgUrls) {
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
		imgUrl := imgUrls[imageIndex]
		path, headers, body := common.GenerateRequestForImage(config, cfg.MultiModalGuardForBase64, checkImageService, imgUrl, "")
		err := config.Client.Post(path, headers, body, callback, config.Timeout)
		if err != nil {
			log.Errorf("failed call the safe check service: %v", err)
			proxywasm.ResumeHttpResponse()
		}
	}
	singleCall()
	return types.ActionPause
}
