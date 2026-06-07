// 这个文件中主要将OnHttpRequestHeaders、OnHttpRequestBody、OnHttpResponseHeaders、OnHttpResponseBody这四个函数实现
// 其中的缓存思路调用cache.go中的逻辑，然后cache.go中的逻辑会调用textEmbeddingProvider和vectorStoreProvider中的逻辑（实例）
package main

import (
	"strings"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-cache/config"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

const (
	PLUGIN_NAME                 = "ai-cache"
	CACHE_KEY_CONTEXT_KEY       = "cacheKey"
	CACHE_KEY_EMBEDDING_KEY     = "cacheKeyEmbedding"
	CACHE_CONTENT_CONTEXT_KEY   = "cacheContent"
	PARTIAL_MESSAGE_CONTEXT_KEY = "partialMessage"
	TOOL_CALLS_CONTEXT_KEY      = "toolCalls"
	STREAM_CONTEXT_KEY          = "stream"
	SKIP_CACHE_HEADER           = "x-higress-skip-ai-cache"
	ERROR_PARTIAL_MESSAGE_KEY   = "errorPartialMessage"

	DEFAULT_MAX_BODY_BYTES uint32 = 100 * 1024 * 1024
)

func main() {}

func init() {
	// CreateClient()
	wrapper.SetCtx(
		PLUGIN_NAME,
		wrapper.ParseConfigBy(parseConfig),
		wrapper.ProcessRequestHeadersBy(onHttpRequestHeaders),
		wrapper.ProcessRequestBodyBy(onHttpRequestBody),
		wrapper.ProcessResponseHeadersBy(onHttpResponseHeaders),
		wrapper.ProcessStreamingResponseBodyBy(onHttpResponseBody),
	)
}

func parseConfig(json gjson.Result, c *config.PluginConfig, log log.Log) error {
	// config.EmbeddingProviderConfig.FromJson(json.Get("embeddingProvider"))
	// config.VectorDatabaseProviderConfig.FromJson(json.Get("vectorBaseProvider"))
	// config.RedisConfig.FromJson(json.Get("redis"))
	c.FromJson(json, log)
	if err := c.Validate(); err != nil {
		return err
	}
	// Note that initializing the client during the parseConfig phase may cause errors, such as Redis not being usable in Docker Compose.
	if err := c.Complete(log); err != nil {
		log.Errorf("complete config failed: %v", err)
		return err
	}
	return nil
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, c config.PluginConfig, log log.Log) types.Action {
	ctx.DisableReroute()
	skipCache, _ := proxywasm.GetHttpRequestHeader(SKIP_CACHE_HEADER)
	if skipCache == "on" {
		ctx.SetContext(SKIP_CACHE_HEADER, struct{}{})
		ctx.DontReadRequestBody()
		return types.ActionContinue
	}
	contentType, _ := proxywasm.GetHttpRequestHeader("content-type")
	// The request does not have a body.
	if contentType == "" {
		return types.ActionContinue
	}
	if !strings.Contains(contentType, "application/json") {
		log.Warnf("content is not json, can't process: %s", contentType)
		ctx.DontReadRequestBody()
		return types.ActionContinue
	}
	ctx.SetRequestBodyBufferLimit(DEFAULT_MAX_BODY_BYTES)
	_ = proxywasm.RemoveHttpRequestHeader("Accept-Encoding")
	// The request has a body and requires delaying the header transmission until a cache miss occurs,
	// at which point the header should be sent.
	return types.HeaderStopIteration
}

func onHttpRequestBody(ctx wrapper.HttpContext, c config.PluginConfig, body []byte, log log.Log) types.Action {
	bodyJson := gjson.ParseBytes(body)
	// TODO: It may be necessary to support stream mode determination for different LLM providers.
	stream := false
	if bodyJson.Get("stream").Bool() {
		stream = true
		ctx.SetContext(STREAM_CONTEXT_KEY, struct{}{})
	}

	var key string
	if c.CacheKeyStrategy == config.CACHE_KEY_STRATEGY_LAST_QUESTION {
		log.Debugf("[onHttpRequestBody] cache key strategy is last question, cache key from: %s", c.CacheKeyFrom)
		key = bodyJson.Get(c.CacheKeyFrom).String()
	} else if c.CacheKeyStrategy == config.CACHE_KEY_STRATEGY_ALL_QUESTIONS {
		log.Debugf("[onHttpRequestBody] cache key strategy is all questions, cache key from: messages")
		messages := bodyJson.Get("messages").Array()
		var userMessages []string
		for _, msg := range messages {
			if msg.Get("role").String() == "user" {
				userMessages = append(userMessages, msg.Get("content").String())
			}
		}
		key = strings.Join(userMessages, "\n")
	} else if c.CacheKeyStrategy == config.CACHE_KEY_STRATEGY_DISABLED {
		log.Info("[onHttpRequestBody] cache key strategy is disabled")
		ctx.DontReadResponseBody()
		return types.ActionContinue
	} else {
		log.Warnf("[onHttpRequestBody] unknown cache key strategy: %s", c.CacheKeyStrategy)
		ctx.DontReadResponseBody()
		return types.ActionContinue
	}

	ctx.SetContext(CACHE_KEY_CONTEXT_KEY, key)
	log.Debugf("[onHttpRequestBody] key: %s", key)
	if key == "" {
		log.Debug("[onHttpRequestBody] parse key from request body failed")
		ctx.DontReadResponseBody()
		return types.ActionContinue
	}

	if err := CheckCacheForKey(key, ctx, c, log, stream, true); err != nil {
		log.Errorf("[onHttpRequestBody] check cache for key: %s failed, error: %v", key, err)
		return types.ActionContinue
	}

	return types.ActionPause
}

func onHttpResponseHeaders(ctx wrapper.HttpContext, c config.PluginConfig, log log.Log) types.Action {
	skipCache := ctx.GetContext(SKIP_CACHE_HEADER)
	if skipCache != nil {
		ctx.SetUserAttribute("cache_status", "skip")
		ctx.WriteUserAttributeToLogWithKey(wrapper.AILogKey)
		ctx.DontReadResponseBody()
		return types.ActionContinue
	}
	if ctx.GetContext(CACHE_KEY_CONTEXT_KEY) != nil {
		ctx.SetUserAttribute("cache_status", "miss")
		ctx.WriteUserAttributeToLogWithKey(wrapper.AILogKey)
	}
	contentType, _ := proxywasm.GetHttpResponseHeader("content-type")
	if strings.Contains(contentType, "text/event-stream") {
		ctx.SetContext(STREAM_CONTEXT_KEY, struct{}{})
	} else {
		ctx.SetResponseBodyBufferLimit(DEFAULT_MAX_BODY_BYTES)
	}

	return types.ActionContinue
}

func onHttpResponseBody(ctx wrapper.HttpContext, c config.PluginConfig, chunk []byte, isLastChunk bool, log log.Log) []byte {
	log.Debugf("[onHttpResponseBody] is last chunk: %v", isLastChunk)
	log.Debugf("[onHttpResponseBody] chunk: %s", string(chunk))

	if ctx.GetContext(TOOL_CALLS_CONTEXT_KEY) != nil || ctx.GetContext(ERROR_PARTIAL_MESSAGE_KEY) != nil {
		return chunk
	}

	key := ctx.GetContext(CACHE_KEY_CONTEXT_KEY)
	if key == nil {
		log.Debug("[onHttpResponseBody] key is nil, skip cache")
		return chunk
	}

	stream := ctx.GetContext(STREAM_CONTEXT_KEY)
	var err error
	if !isLastChunk {
		if stream == nil {
			err = handleNonStreamChunk(ctx, c, chunk, log)
		} else {
			err = handleStreamChunk(ctx, c, unifySSEChunk(chunk), log)
		}
		if err != nil {
			log.Errorf("[onHttpResponseBody] handle non last chunk failed, error: %v", err)
			// Set an empty struct in the context to indicate an error in processing the partial message
			ctx.SetContext(ERROR_PARTIAL_MESSAGE_KEY, struct{}{})
		}
		return chunk
	}
	var value string
	if stream == nil {
		value, err = processNonStreamLastChunk(ctx, c, chunk, log)
	} else {
		value, err = processStreamLastChunk(ctx, c, unifySSEChunk(chunk), log)
	}

	if err != nil {
		log.Errorf("[onHttpResponseBody] process last chunk failed, error: %v", err)
		return chunk
	}

	cacheResponse(ctx, c, key.(string), value, log)
	uploadEmbeddingAndAnswer(ctx, c, key.(string), value, log)
	return chunk
}
