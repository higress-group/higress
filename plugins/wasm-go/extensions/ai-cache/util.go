package main

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-cache/config"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

func handleNonStreamChunk(ctx wrapper.HttpContext, c config.PluginConfig, chunk []byte, log log.Log) error {
	tempContentI := ctx.GetContext(CACHE_CONTENT_CONTEXT_KEY)
	if tempContentI == nil {
		ctx.SetContext(CACHE_CONTENT_CONTEXT_KEY, chunk)
		return nil
	}
	tempContent := tempContentI.([]byte)
	tempContent = append(tempContent, chunk...)
	ctx.SetContext(CACHE_CONTENT_CONTEXT_KEY, tempContent)
	return nil
}

func unifySSEChunk(data []byte) []byte {
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	data = bytes.ReplaceAll(data, []byte("\r"), []byte("\n"))
	return data
}

func handleStreamChunk(ctx wrapper.HttpContext, c config.PluginConfig, chunk []byte, log log.Log) error {
	var partialMessage []byte
	partialMessageI := ctx.GetContext(PARTIAL_MESSAGE_CONTEXT_KEY)
	log.Debugf("[handleStreamChunk] cache content: %v", ctx.GetContext(CACHE_CONTENT_CONTEXT_KEY))
	if partialMessageI != nil {
		partialMessage = append(partialMessageI.([]byte), chunk...)
	} else {
		partialMessage = chunk
	}
	messages := strings.Split(string(partialMessage), "\n\n")
	for i, msg := range messages {
		if i < len(messages)-1 {
			_, err := processSSEMessage(ctx, c, msg, log)
			if err != nil {
				return fmt.Errorf("[handleStreamChunk] processSSEMessage failed, error: %v", err)
			}
		}
	}
	if !strings.HasSuffix(string(partialMessage), "\n\n") {
		ctx.SetContext(PARTIAL_MESSAGE_CONTEXT_KEY, []byte(messages[len(messages)-1]))
	} else {
		ctx.SetContext(PARTIAL_MESSAGE_CONTEXT_KEY, nil)
	}
	return nil
}

func processNonStreamLastChunk(ctx wrapper.HttpContext, c config.PluginConfig, chunk []byte, log log.Log) (string, error) {
	var body []byte
	tempContentI := ctx.GetContext(CACHE_CONTENT_CONTEXT_KEY)
	if tempContentI != nil {
		body = append(tempContentI.([]byte), chunk...)
	} else {
		body = chunk
	}
	bodyJson := gjson.ParseBytes(body)
	value := bodyJson.Get(c.CacheValueFrom).String()
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("[processNonStreamLastChunk] parse value from response body failed, body:%s", body)
	}
	return value, nil
}

func processStreamLastChunk(ctx wrapper.HttpContext, c config.PluginConfig, chunk []byte, log log.Log) (string, error) {
	if len(chunk) > 0 {
		var lastMessage []byte
		partialMessageI := ctx.GetContext(PARTIAL_MESSAGE_CONTEXT_KEY)
		if partialMessageI != nil {
			lastMessage = append(partialMessageI.([]byte), chunk...)
		} else {
			lastMessage = chunk
		}
		if !strings.HasSuffix(string(lastMessage), "\n\n") {
			return "", fmt.Errorf("[processStreamLastChunk] invalid lastMessage:%s", lastMessage)
		}
		lastMessage = lastMessage[:len(lastMessage)-2]
		value, err := processSSEMessage(ctx, c, string(lastMessage), log)
		if err != nil {
			return "", fmt.Errorf("[processStreamLastChunk] processSSEMessage failed, error: %v", err)
		}
		// 兜底：[DONE] 或其它尾部 chunk 无 content 时，processSSEMessage 返回空，
		// 此时从 ctx 取已累积的缓存内容，避免缓存写空。
		if value == "" {
			if tempContentI := ctx.GetContext(CACHE_CONTENT_CONTEXT_KEY); tempContentI != nil {
				value = tempContentI.(string)
			}
		}
		return value, nil
	}
	tempContentI := ctx.GetContext(CACHE_CONTENT_CONTEXT_KEY)
	if tempContentI == nil {
		return "", nil
	}
	return tempContentI.(string), nil
}

func processSSEMessage(ctx wrapper.HttpContext, c config.PluginConfig, sseMessage string, log log.Log) (string, error) {
	content := ""
	// done 标记本次 sseMessage 是否遇到 [DONE]。当最后一段 content 与 [DONE]
	// 处于同一 buffer 时，必须跳出循环后由循环外的 merge 逻辑统一合并到
	// CACHE_CONTENT_CONTEXT_KEY，否则本次解析的最后一段 content 会被 [DONE]
	// 早 return 丢弃（PR #3962 review）。
	done := false
	for _, chunk := range strings.Split(sseMessage, "\n\n") {
		log.Debugf("single sse message: %s", chunk)
		subMessages := strings.Split(chunk, "\n")
		var message string
		for _, msg := range subMessages {
			if strings.HasPrefix(msg, "data:") {
				message = msg
				break
			}
		}
		if len(message) < 6 {
			return content, fmt.Errorf("[processSSEMessage] invalid message: %s", message)
		}

		// skip the prefix "data:"
		bodyJson := message[5:]

		if strings.TrimSpace(bodyJson) == "[DONE]" {
			// 跳出循环，把已解析的局部 content 留到循环外统一合并。
			done = true
			break
		}

		// Extract values from JSON fields
		responseBody := gjson.Get(bodyJson, c.CacheStreamValueFrom)
		toolCalls := gjson.Get(bodyJson, c.CacheToolCallsFrom)

		if toolCalls.Exists() {
			// TODO: Temporarily store the tool_calls value in the context for processing
			ctx.SetContext(TOOL_CALLS_CONTEXT_KEY, toolCalls.String())
		}

		// Check if the ResponseBody field exists
		if responseBody.Exists() {
			content += responseBody.String()
		}
	}

	// 本次 sseMessage 既没解析到 content 也没遇到 [DONE]：保持 ctx 不变，直接返回。
	if content == "" && !done {
		log.Debugf("[processSSEMessage] no content extracted; skipping cache update: %s", sseMessage)
		return "", nil
	}

	if content != "" {
		if v := ctx.GetContext(CACHE_CONTENT_CONTEXT_KEY); v == nil {
			ctx.SetContext(CACHE_CONTENT_CONTEXT_KEY, content)
		} else {
			ctx.SetContext(CACHE_CONTENT_CONTEXT_KEY, v.(string)+content)
		}
	}

	// handleStreamChunk 不使用返回值；processStreamLastChunk 把它作为 cacheResponse 的
	// SET value，必须是完整累积值（避免最后一段 content 因 [DONE] 早 return 被丢）。
	if v := ctx.GetContext(CACHE_CONTENT_CONTEXT_KEY); v != nil {
		return v.(string), nil
	}
	return "", nil
}
