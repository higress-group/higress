package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-cache/config"
	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-cache/vector"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	logs "github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/resp"
)

// CheckCacheForKey checks if the key is in the cache, or triggers similarity search if not found.
func CheckCacheForKey(key string, ctx wrapper.HttpContext, c config.PluginConfig, log logs.Log, stream bool, useSimilaritySearch bool) error {
	activeCacheProvider := c.GetCacheProvider()
	if activeCacheProvider == nil {
		log.Debugf("[%s] [CheckCacheForKey] no cache provider configured, performing similarity search", PLUGIN_NAME)
		return performSimilaritySearch(key, ctx, c, log, key, stream)
	}

	queryKey := activeCacheProvider.GetCacheKeyPrefix() + key
	log.Debugf("[%s] [CheckCacheForKey] querying cache with key: %s", PLUGIN_NAME, queryKey)

	err := activeCacheProvider.Get(queryKey, func(response resp.Value) {
		handleCacheResponse(key, response, ctx, log, stream, c, useSimilaritySearch)
	})
	if err != nil {
		log.Errorf("[%s] [CheckCacheForKey] failed to retrieve key: %s from cache, error: %v", PLUGIN_NAME, key, err)
		return err
	}

	return nil
}

// handleCacheResponse processes cache response and handles cache hits and misses.
func handleCacheResponse(key string, response resp.Value, ctx wrapper.HttpContext, log logs.Log, stream bool, c config.PluginConfig, useSimilaritySearch bool) {
	if err := response.Error(); err == nil && !response.IsNull() {
		log.Infof("[%s] cache hit for key: %s", PLUGIN_NAME, key)
		processCacheHit(key, response.String(), stream, ctx, c, log)
		return
	}

	log.Infof("[%s] [handleCacheResponse] cache miss for key: %s", PLUGIN_NAME, key)
	if err := response.Error(); err != nil {
		log.Errorf("[%s] [handleCacheResponse] error retrieving key: %s from cache, error: %v", PLUGIN_NAME, key, err)
	}

	if useSimilaritySearch && c.EnableSemanticCache {
		if err := performSimilaritySearch(key, ctx, c, log, key, stream); err != nil {
			log.Errorf("[%s] [handleCacheResponse] failed to perform similarity search for key: %s, error: %v", PLUGIN_NAME, key, err)
			proxywasm.ResumeHttpRequest()
		}
	} else {
		proxywasm.ResumeHttpRequest()
	}
}

// processCacheHit handles a successful cache hit.
func processCacheHit(key string, response string, stream bool, ctx wrapper.HttpContext, c config.PluginConfig, log logs.Log) {
	if strings.TrimSpace(response) == "" {
		log.Warnf("[%s] [processCacheHit] cached response for key %s is empty", PLUGIN_NAME, key)
		proxywasm.ResumeHttpRequest()
		return
	}

	log.Debugf("[%s] [processCacheHit] cached response for key %s: %s", PLUGIN_NAME, key, response)

	// Escape the response to ensure consistent formatting
	escapedResponse := strings.Trim(strconv.Quote(response), "\"")

	ctx.SetContext(CACHE_KEY_CONTEXT_KEY, nil)

	ctx.SetUserAttribute("cache_status", "hit")
	ctx.WriteUserAttributeToLogWithKey(wrapper.AILogKey)

	if stream {
		proxywasm.SendHttpResponseWithDetail(200, "ai-cache.hit", [][2]string{{"content-type", "text/event-stream; charset=utf-8"}}, []byte(fmt.Sprintf(c.StreamResponseTemplate, escapedResponse)), -1)
	} else {
		proxywasm.SendHttpResponseWithDetail(200, "ai-cache.hit", [][2]string{{"content-type", "application/json; charset=utf-8"}}, []byte(fmt.Sprintf(c.ResponseTemplate, escapedResponse)), -1)
	}
}

// performSimilaritySearch determines the appropriate similarity search method to use.
func performSimilaritySearch(key string, ctx wrapper.HttpContext, c config.PluginConfig, log logs.Log, queryString string, stream bool) error {
	activeVectorProvider := c.GetVectorProvider()
	if activeVectorProvider == nil {
		return logAndReturnError(log, "[performSimilaritySearch] no vector provider configured for similarity search")
	}

	// Check if the active vector provider implements the StringQuerier interface.
	if _, ok := activeVectorProvider.(vector.StringQuerier); ok {
		log.Debugf("[%s] [performSimilaritySearch] active vector provider implements StringQuerier interface, performing string query", PLUGIN_NAME)
		return performStringQuery(key, queryString, ctx, c, log, stream)
	}

	// Check if the active vector provider implements the EmbeddingQuerier interface.
	if _, ok := activeVectorProvider.(vector.EmbeddingQuerier); ok {
		log.Debugf("[%s] [performSimilaritySearch] active vector provider implements EmbeddingQuerier interface, performing embedding query", PLUGIN_NAME)
		return performEmbeddingQuery(key, ctx, c, log, stream)
	}

	return logAndReturnError(log, "[performSimilaritySearch] no suitable querier or embedding provider available for similarity search")
}

// performStringQuery executes the string-based similarity search.
func performStringQuery(key string, queryString string, ctx wrapper.HttpContext, c config.PluginConfig, log logs.Log, stream bool) error {
	stringQuerier, ok := c.GetVectorProvider().(vector.StringQuerier)
	if !ok {
		return logAndReturnError(log, "[performStringQuery] active vector provider does not implement StringQuerier interface")
	}

	return stringQuerier.QueryString(queryString, ctx, log, func(results []vector.QueryResult, ctx wrapper.HttpContext, log logs.Log, err error) {
		handleQueryResults(key, results, ctx, log, stream, c, err)
	})
}

// performEmbeddingQuery executes the embedding-based similarity search.
func performEmbeddingQuery(key string, ctx wrapper.HttpContext, c config.PluginConfig, log logs.Log, stream bool) error {
	embeddingQuerier, ok := c.GetVectorProvider().(vector.EmbeddingQuerier)
	if !ok {
		return logAndReturnError(log, fmt.Sprintf("[performEmbeddingQuery] active vector provider does not implement EmbeddingQuerier interface"))
	}

	activeEmbeddingProvider := c.GetEmbeddingProvider()
	if activeEmbeddingProvider == nil {
		return logAndReturnError(log, fmt.Sprintf("[performEmbeddingQuery] no embedding provider configured for similarity search"))
	}

	return activeEmbeddingProvider.GetEmbedding(key, ctx, func(textEmbedding []float64, err error) {
		log.Debugf("[%s] [performEmbeddingQuery] GetEmbedding success, length of embedding: %d, error: %v", PLUGIN_NAME, len(textEmbedding), err)
		if err != nil {
			handleInternalError(err, fmt.Sprintf("[%s] [performEmbeddingQuery] error getting embedding for key: %s", PLUGIN_NAME, key), log)
			return
		}
		ctx.SetContext(CACHE_KEY_EMBEDDING_KEY, textEmbedding)

		err = embeddingQuerier.QueryEmbedding(textEmbedding, ctx, log, func(results []vector.QueryResult, ctx wrapper.HttpContext, log logs.Log, err error) {
			handleQueryResults(key, results, ctx, log, stream, c, err)
		})
		if err != nil {
			handleInternalError(err, fmt.Sprintf("[%s] [performEmbeddingQuery] error querying vector database for key: %s", PLUGIN_NAME, key), log)
		}
	})
}

// handleQueryResults processes the results of similarity search and determines next actions.
func handleQueryResults(key string, results []vector.QueryResult, ctx wrapper.HttpContext, log logs.Log, stream bool, c config.PluginConfig, err error) {
	if err != nil {
		handleInternalError(err, fmt.Sprintf("[%s] [handleQueryResults] error querying vector database for key: %s", PLUGIN_NAME, key), log)
		return
	}

	if len(results) == 0 {
		log.Warnf("[%s] [handleQueryResults] no similar keys found for key: %s", PLUGIN_NAME, key)
		proxywasm.ResumeHttpRequest()
		return
	}

	mostSimilarData := results[0]
	log.Debugf("[%s] [handleQueryResults] for key: %s, the most similar key found: %s with score: %f", PLUGIN_NAME, key, mostSimilarData.Text, mostSimilarData.Score)
	simThreshold := c.GetVectorProviderConfig().Threshold
	simThresholdRelation := c.GetVectorProviderConfig().ThresholdRelation
	if compare(simThresholdRelation, mostSimilarData.Score, simThreshold) {
		log.Infof("[%s] key accepted: %s with score: %f", PLUGIN_NAME, mostSimilarData.Text, mostSimilarData.Score)
		if mostSimilarData.Answer != "" {
			// direct return the answer if available
			cacheResponse(ctx, c, key, mostSimilarData.Answer, log)
			processCacheHit(key, mostSimilarData.Answer, stream, ctx, c, log)
		} else {
			if c.GetCacheProvider() != nil {
				CheckCacheForKey(mostSimilarData.Text, ctx, c, log, stream, false)
			} else {
				// Otherwise, do not check the cache, directly return
				log.Infof("[%s] cache hit for key: %s, but no corresponding answer found in the vector database", PLUGIN_NAME, mostSimilarData.Text)
				proxywasm.ResumeHttpRequest()
			}
		}
	} else {
		log.Infof("[%s] score not meet the threshold %f: %s with score %f", PLUGIN_NAME, simThreshold, mostSimilarData.Text, mostSimilarData.Score)
		proxywasm.ResumeHttpRequest()
	}
}

// logAndReturnError logs an error and returns it.
func logAndReturnError(log logs.Log, message string) error {
	message = fmt.Sprintf("[%s] %s", PLUGIN_NAME, message)
	log.Errorf(message)
	return errors.New(message)
}

// handleInternalError logs an error and resumes the HTTP request.
func handleInternalError(err error, message string, log logs.Log) {
	if err != nil {
		log.Errorf("[%s] [handleInternalError] %s: %v", PLUGIN_NAME, message, err)
	} else {
		log.Errorf("[%s] [handleInternalError] %s", PLUGIN_NAME, message)
	}
	// proxywasm.SendHttpResponse(500, [][2]string{{"content-type", "text/plain"}}, []byte("Internal Server Error"), -1)
	proxywasm.ResumeHttpRequest()
}

// Caches the response value
func cacheResponse(ctx wrapper.HttpContext, c config.PluginConfig, key string, value string, log logs.Log) {
	if strings.TrimSpace(value) == "" {
		log.Warnf("[%s] [cacheResponse] cached value for key %s is empty", PLUGIN_NAME, key)
		return
	}

	activeCacheProvider := c.GetCacheProvider()
	if activeCacheProvider != nil {
		queryKey := activeCacheProvider.GetCacheKeyPrefix() + key
		_ = activeCacheProvider.Set(queryKey, value, nil)
		log.Debugf("[%s] [cacheResponse] cache set success, key: %s, length of value: %d", PLUGIN_NAME, queryKey, len(value))
	}
}

// Handles embedding upload if available
func uploadEmbeddingAndAnswer(ctx wrapper.HttpContext, c config.PluginConfig, key string, value string, log logs.Log) {
	embedding := ctx.GetContext(CACHE_KEY_EMBEDDING_KEY)
	if embedding == nil {
		return
	}

	emb, ok := embedding.([]float64)
	if !ok {
		log.Errorf("[%s] [uploadEmbeddingAndAnswer] embedding is not of expected type []float64", PLUGIN_NAME)
		return
	}

	activeVectorProvider := c.GetVectorProvider()
	if activeVectorProvider == nil {
		log.Debugf("[%s] [uploadEmbeddingAndAnswer] no vector provider configured for uploading embedding", PLUGIN_NAME)
		return
	}

	// Check if the cached value is empty, skip uploading embedding if so
	if strings.TrimSpace(value) == "" {
		log.Warnf("[%s] [uploadEmbeddingAndAnswer] cached value for key %s is empty, skip uploading embedding", PLUGIN_NAME, key)
		return
	}

	// Attempt to upload answer embedding first
	if ansEmbUploader, ok := activeVectorProvider.(vector.AnswerAndEmbeddingUploader); ok {
		log.Infof("[%s] uploading answer embedding for key: %s", PLUGIN_NAME, key)
		err := ansEmbUploader.UploadAnswerAndEmbedding(key, emb, value, ctx, log, nil)
		if err != nil {
			log.Warnf("[%s] [uploadEmbeddingAndAnswer] failed to upload answer embedding for key: %s, error: %v", PLUGIN_NAME, key, err)
		} else {
			return // If successful, return early
		}
	}

	// If answer embedding upload fails, attempt normal embedding upload
	if embUploader, ok := activeVectorProvider.(vector.EmbeddingUploader); ok {
		log.Infof("[%s] uploading embedding for key: %s", PLUGIN_NAME, key)
		err := embUploader.UploadEmbedding(key, emb, ctx, log, nil)
		if err != nil {
			log.Warnf("[%s] [uploadEmbeddingAndAnswer] failed to upload embedding for key: %s, error: %v", PLUGIN_NAME, key, err)
		}
	}
}

// 主要用于相似度/距离/点积判断
// 余弦相似度度量的是两个向量在方向上的相似程度。相似度越高，两个向量越接近。
// 距离度量的是两个向量在空间上的远近程度。距离越小，两个向量越接近。
// compare 函数根据操作符进行判断并返回结果
func compare(operator string, value1 float64, value2 float64) bool {
	switch operator {
	case "gt":
		return value1 > value2
	case "gte":
		return value1 >= value2
	case "lt":
		return value1 < value2
	case "lte":
		return value1 <= value2
	default:
		return false
	}
}
