package main

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strconv"
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
	// 模型白名单：正则表达式列表，任意一个匹配就触发 OCR
	modelPatterns []string
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

	// 解析模型白名单（正则表达式列表）
	json.Get("modelPatterns").ForEach(func(k, v gjson.Result) bool {
		config.modelPatterns = append(config.modelPatterns, v.String())
		return true
	})

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

// imageBlock 记录某条消息 content 数组里一个 image_url 块的位置。
// msgIdx 为消息在 messages 中的索引，partIdx 为该块在 content 数组的索引。
// 全量扫描后统一就地替换，保证历史残留 image_url 也不会漏到后端。
type imageBlock struct {
	msgIdx   int
	partIdx  int
	url      string
	isLatest bool // 是否位于"最后一条 user 消息"：只有这些图需要本轮重新 OCR
}

// collectAllImageBlocks 全量扫描所有消息（deepseek-vision 思路），
// 收集每一条消息里所有 image_url 块的位置与 URL，并标记是否属于"最后一条 user"。
// query / queryIndex 顺带返回末条 user 的提问文本与其索引（供 OCR prompt 拼装）。
// 相比官方只扫末条 user，这能覆盖多轮对话时历史残留的 image_url。
func collectAllImageBlocks(body []byte) (
	blocks []imageBlock, query string, queryIndex int, hasImages bool) {
	messages := gjson.GetBytes(body, "messages").Array()
	queryIndex = -1
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Get("role").String() == "user" {
			queryIndex = i
			break
		}
	}
	for i := 0; i < len(messages); i++ {
		content := messages[i].Get("content")
		if !content.IsArray() {
			continue // 纯字符串 content 天然无 image_url
		}
		parts := content.Array()
		isLatest := (i == queryIndex && messages[i].Get("role").String() == "user")
		for j := 0; j < len(parts); j++ {
			if parts[j].Get("type").String() == "image_url" {
				blocks = append(blocks, imageBlock{
					msgIdx:   i,
					partIdx:  j,
					url:      parts[j].Get("image_url.url").String(),
					isLatest: isLatest,
				})
			} else if isLatest && parts[j].Get("type").String() == "text" {
				query = parts[j].Get("text").String()
			}
		}
	}
	return blocks, query, queryIndex, len(blocks) > 0
}

func onHttpRequestBody(ctx wrapper.HttpContext, config Config, body []byte) types.Action {
	// 模型白名单：只允许匹配的模型触发 OCR，多模态模型直接放行
	requestModel := gjson.GetBytes(body, "model").String()
	if !matchesModelPatterns(requestModel, config.modelPatterns) {
		return types.ActionContinue
	}

	blocks, query, queryIndex, hasImages := collectAllImageBlocks(body)
	log.Debugf("onHttpRequestBody: model=%s, hasImages=%v blocks=%d queryIndex=%d",
		requestModel, hasImages, len(blocks), queryIndex)
	if !hasImages {
		// 整个请求里没有任何图片（含历史），无需任何处理，直接放行。
		return types.ActionContinue
	}

	// 只对"最后一条 user"的图做本轮 OCR（这是用户本次想问的图）；
	// 历史消息里的残留图不重复 OCR，直接复用文本清理即可。
	var imageUrls []string
	for _, b := range blocks {
		if b.isLatest {
			imageUrls = append(imageUrls, b.url)
		}
	}
	if len(imageUrls) == 0 {
		// 只有历史消息残留 image_url，末条 user 无图：无需 OCR，
		// 全量剥离所有历史 image_url 后放行（fail-closed 校验由 stripAllImageUrls 保证）。
		strippedBody, changed := stripAllImageUrls(body)
		log.Debugf("strip legacy: changed=%v", changed)
		if changed {
			log.Debugf("  stripped body:%s", strippedBody)
			proxywasm.ReplaceHttpRequestBody(strippedBody)
		}
		return types.ActionContinue
	}
	// 末条 user 带图：OCR 后全量就地替换（含历史残留图一并清理）。
	return executeReadImage(blocks, config, query, queryIndex, body)
}

// matchesModelPatterns 检查模型名是否匹配任意一个正则表达式
func matchesModelPatterns(model string, patterns []string) bool {
	if model == "" {
		return false
	}
	// 空列表 = 不匹配任何模型（白名单模式：默认关闭）
	if len(patterns) == 0 {
		return false
	}
	for _, pattern := range patterns {
		if matched, _ := regexp.MatchString(pattern, model); matched {
			return true
		}
	}
	return false
}

// stripAllImageUrls 删除 body 中所有 messages 的 content 数组里的 image_url 部分，
// 防止多轮对话时历史消息残留 image_url 导致非多模态后端（如 DeepSeek）报错。
// - content 为纯字符串的消息天然不含 image_url，自动跳过
// - 若某条消息的 content 数组只含 image_url，删除后会变成空数组被后端拒绝，
//   此时整体替换为占位文本 "[图片]"
// 返回处理后的 body 以及是否发生了修改。
func stripAllImageUrls(body []byte) ([]byte, bool) {
	changed := false
	messages := gjson.GetBytes(body, "messages").Array()
	log.Debugf("stripAllImageUrls: messages count=%d", len(messages))
	for i := 0; i < len(messages); i++ {
		content := messages[i].Get("content")
		if !content.IsArray() {
			log.Debugf("  message[%d]: content is string (type=%v), skip", i, content.Type)
			continue
		}
		parts := content.Array()
		log.Debugf("  message[%d]: content is array with %d parts", i, len(parts))
		var imageIndices []int
		hasOtherContent := false
		for j := 0; j < len(parts); j++ {
			partType := parts[j].Get("type").String()
			log.Debugf("    part[%d]: type=%q", j, partType)
			if partType == "image_url" {
				imageIndices = append(imageIndices, j)
			} else {
				hasOtherContent = true
			}
		}
		if len(imageIndices) == 0 {
			continue
		}
		log.Debugf("  message[%d]: found %d image_url parts, stripping", i, len(imageIndices))
		if !hasOtherContent {
			// content 全是 image_url：替换为占位文本，避免产生空数组
			newBody, err := sjson.SetBytes(body, fmt.Sprintf("messages.%d.content", i), "[图片]")
			if err != nil {
				log.Warnf("replace image-only content failed, err:%v", err)
				continue
			}
			body = newBody
			changed = true
			continue
		}
		// 从后往前删除，避免索引偏移
		for k := len(imageIndices) - 1; k >= 0; k-- {
			newBody, err := sjson.DeleteBytes(body, fmt.Sprintf("messages.%d.content.%d", i, imageIndices[k]))
			if err != nil {
				log.Warnf("delete image_url content failed, err:%v", err)
				continue
			}
			body = newBody
			changed = true
		}
	}
	log.Debugf("stripAllImageUrls: finished, changed=%v", changed)
	return body, changed
}

func executeReadImage(blocks []imageBlock, config Config, query string, queryIndex int, body []byte) types.Action {
	// 本轮需要真正 OCR 的图：仅"最后一条 user"里的图（用户本次想问的图），
	// 历史残留的图不重复 OCR，交给 stripAllImageUrls 统一清理。
	var imageUrls []string
	for _, b := range blocks {
		if b.isLatest {
			imageUrls = append(imageUrls, b.url)
		}
	}
	// 按索引记录 OCR 结果：成功则非空，失败则留空（待完成分支补占位文本）
	results := make([]string, len(imageUrls))
	totalImages := 0
	finished := 0
	for i, imageUrl := range imageUrls {
		idx := i
		err := config.ocrProvider.DoOCR(imageUrl, func(imageContent string, err error) {
			defer func() {
				finished++
				if totalImages == finished {
					var processedContents []string
					// 正序生成，避免倒序导致图片顺序颠倒
					for k := 0; k < len(results); k++ {
						if results[k] != "" {
							processedContents = append(processedContents, fmt.Sprintf("第%d张图片内容为 %s", k+1, results[k]))
						} else {
							processedContents = append(processedContents, fmt.Sprintf("第%d张图片识别失败", k+1))
						}
					}
					imageSummary := fmt.Sprintf("总共有 %d 张图片。\n", len(imageUrls))
					prompt := strings.Replace(config.promptTemplate, "{image_content}", imageSummary+strings.Join(processedContents, "\n"), 1)
					prompt = strings.Replace(prompt, "{question}", query, 1)

					// 替换最后一条 user 消息的 content 为纯文本
					modifiedBody, err := sjson.SetBytes(body, fmt.Sprintf("messages.%d.content", queryIndex), prompt)
					if err != nil {
						log.Errorf("modify request message content failed, err:%v, body:%s", err, body)
						return
					}

					// 安全兜底：清除历史 messages 中残留的 image_url
					//（最后一条 user 消息已被替换为纯文本，IsArray 检查会自动跳过它）
					modifiedBody, _ = stripAllImageUrls(modifiedBody)

					// fail-closed：最终防线——若常规剥离后仍残留任何 image_url（如
					// 出现在 content 字符串内嵌 / 工具调用参数 / 其它异常形态），
					// 再做一次深度清除，确保外发请求绝不含多模态内容，避免后端 400。
					if hasImageUrlAnywhere(modifiedBody) {
						log.Warnf("[fail-closed] image_url still present after strip, force removing")
						modifiedBody = forceRemoveAllImages(modifiedBody, queryIndex)
					}

					log.Debugf("modified body:%s", modifiedBody)
					proxywasm.ReplaceHttpRequestBody(modifiedBody)
					proxywasm.ResumeHttpRequest()
				}
			}()
			if err != nil {
				log.Errorf("do ocr failed, err:%v", err)
				return
			}
			results[idx] = imageContent
		})
		if err != nil {
			log.Errorf("ocr call failed, err:%v", err)
			continue // 同步失败：不 totalImages++，回调不会触发
		}
		totalImages++
	}
	if totalImages > 0 {
		return types.ActionPause
	}
	// 关键兜底：OCR 调用【全部同步失败】（cluster 不可达/超时/限流等，回调从未触发）。
	// 此时绝不能把含 image_url 的原始 body 原样透传给非多模态主模型（否则报"不是多模态大模型" 400）。
	// 而是先剥离所有 image_url，并注入占位文本，保证外发请求是纯文本。
	// 这样即使 OCR 暂时不可用，对话也能以"图片无法解析"的方式继续，而不是直接报错。
	strippedBody, _ := stripAllImageUrls(body)
	// 把最后一条 user 消息替换为占位说明，避免其 content 数组被剥离后变成空数组被后端拒绝
	placeholderPrompt := "用户发送了 " + strconv.Itoa(len(imageUrls)) + " 张图片，但图片解析服务暂时不可用。请告诉用户图片暂时无法解析，并请 TA 稍后重试或以文字描述图片内容。"
	strippedBody, err := sjson.SetBytes(strippedBody, fmt.Sprintf("messages.%d.content", queryIndex), placeholderPrompt)
	if err != nil {
		log.Errorf("[ocr-all-fail] set placeholder failed, err:%v; fallback strip only", err)
	}
	if string(strippedBody) != string(body) {
		log.Errorf("[ocr-all-fail] OCR unreachable, stripping %d image_url and forwarding text-only body", len(imageUrls))
		proxywasm.ReplaceHttpRequestBody(strippedBody)
	}
	return types.ActionContinue
}

// hasImageUrlAnywhere 深度检查 body 里是否还残留任何 "image_url" 字段。
// fail-closed 语义：只要常规剥离后 body 里仍出现 image_url，就不允许原始透传，
// 必须再走 forceRemoveAllImages 彻底清除。
func hasImageUrlAnywhere(body []byte) bool {
	return bytes.Contains(body, []byte(`"image_url"`))
}

// forceRemoveAllImages 是 fail-closed 的最终防线：
// 1. 先复用 stripAllImageUrls 剥离所有 content 数组里的 image_url；
// 2. 若剥离后 body 里仍残留 "image_url"（说明它可能藏在 content 字符串、
//    工具调用参数等 stripAllImageUrls 覆盖不到的形态），则把"最后一条 user"
//    的 content 整体强制替换为占位提示文本，杜绝多模态内容透传给非多模态后端。
// queryIndex 为最后一条 user 消息的索引；若为 -1 则跳过占位替换。
func forceRemoveAllImages(body []byte, queryIndex int) []byte {
	out, _ := stripAllImageUrls(body)
	if hasImageUrlAnywhere(out) && queryIndex >= 0 {
		placeholder := "用户发送了图片，但图片内容未能解析。请告诉用户图片暂时无法解析，并请 TA 以文字描述图片内容。"
		var err error
		out, err = sjson.SetBytes(out, fmt.Sprintf("messages.%d.content", queryIndex), placeholder)
		if err != nil {
			log.Errorf("[fail-closed] force replace last user content failed, err:%v", err)
		}
	}
	return out
}
