package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	DefaultMaxBodyBytes uint32 = 100 * 1024 * 1024
)

type Config struct {
	promptTemplate    string
	ocrProvider       Provider
	ocrProviderConfig *ProviderConfig
}

func main() {}

func init() {
	wrapper.SetCtx(
		"ai-image-reader",
		wrapper.ParseConfig(parseConfig),
		wrapper.ProcessRequestHeaders(onHttpRequestHeaders),
		wrapper.ProcessRequestBody(onHttpRequestBody),
	)
}

func parseConfig(json gjson.Result, config *Config) error {
	config.promptTemplate = `# 用户发送的图片解析得到的文字内容如下:
{image_content}
在回答时，请注意以下几点：
- 请你回答问题时结合用户图片的文字内容回答。
- 除非用户要求，否则你回答的语言需要和用户提问的语言保持一致。

# 用户消息为：
{question}`
	config.ocrProviderConfig = &ProviderConfig{}
	config.ocrProviderConfig.FromJson(json)
	if err := config.ocrProviderConfig.Validate(); err != nil {
		return err
	}
	var err error
	config.ocrProvider, err = CreateProvider(*config.ocrProviderConfig)
	if err != nil {
		return errors.New("create ocr provider failed")
	}
	return nil
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config Config) types.Action {
	ctx.DisableReroute()
	contentType, _ := proxywasm.GetHttpRequestHeader("content-type")
	if contentType == "" {
		return types.ActionContinue
	}
	if !strings.Contains(contentType, "application/json") {
		log.Warnf("content is not json, can't process: %s", contentType)
		ctx.DontReadRequestBody()
		return types.ActionContinue
	}
	ctx.SetRequestBodyBufferLimit(DefaultMaxBodyBytes)
	_ = proxywasm.RemoveHttpRequestHeader("Accept-Encoding")
	return types.ActionContinue
}

func onHttpRequestBody(ctx wrapper.HttpContext, config Config, body []byte) types.Action {
	var queryIndex int
	var query string
	messages := gjson.GetBytes(body, "messages").Array()
	var imageUrls []string
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Get("role").String() == "user" {
			queryIndex = i
			content := messages[i].Get("content").Array()
			for j := len(content) - 1; j >= 0; j-- {
				contentType := content[j].Get("type").String()
				if contentType == "image_url" {
					imageUrls = append(imageUrls, content[j].Get("image_url.url").String())
				} else if contentType == "text" {
					query = content[j].Get("text").String()
				}
			}
			break
		}
	}
	if len(imageUrls) == 0 {
		return types.ActionContinue
	}
	return executeReadImage(imageUrls, config, query, queryIndex, body)
}

func executeReadImage(imageUrls []string, config Config, query string, queryIndex int, body []byte) types.Action {
	imageContents := make([]string, len(imageUrls))
	var totalImages int
	var finished int
	for i, imageUrl := range imageUrls {
		err := config.ocrProvider.DoOCR(imageUrl, func(imageContent string, err error) {
			defer func() {
				finished++
				if totalImages == finished {
					var processedContents []string
					for idx := len(imageContents) - 1; idx >= 0; idx-- {
						processedContents = append(processedContents, fmt.Sprintf("第%d张图片内容为 %s", totalImages-idx, imageContents[idx]))
					}
					imageSummary := fmt.Sprintf("总共有 %d 张图片。\n", totalImages)
					prompt := strings.Replace(config.promptTemplate, "{image_content}", imageSummary+strings.Join(processedContents, "\n"), 1)
					prompt = strings.Replace(prompt, "{question}", query, 1)
					modifiedBody, err := sjson.SetBytes(body, fmt.Sprintf("messages.%d.content", queryIndex), prompt)
					if err != nil {
						log.Errorf("modify request message content failed, err:%v, body:%s", err, body)
					} else {
						log.Debugf("modified body:%s", modifiedBody)
						proxywasm.ReplaceHttpRequestBody(modifiedBody)
					}
					proxywasm.ResumeHttpRequest()
				}
			}()
			if err != nil {
				log.Errorf("do ocr failed, err:%v", err)
				return
			}
			imageContents[i] = imageContent
		})
		if err != nil {
			log.Errorf("ocr call failed, err:%v", err)
			continue
		}
		totalImages++
	}
	if totalImages > 0 {
		return types.ActionPause
	}
	return types.ActionContinue
}
