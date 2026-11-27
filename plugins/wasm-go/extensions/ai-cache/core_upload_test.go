package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-cache/config"
	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-cache/vector"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	logs "github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestUploadEmbeddingAndAnswerProviders(t *testing.T) {
	for _, providerType := range []string{"dashvector", "chroma", "elasticsearch", "milvus", "pinecone", "qdrant", "weaviate"} {
		t.Run(providerType, func(t *testing.T) {
			test.RunGoTest(t, func(t *testing.T) {
				host, cfg, logger := newUploadTestHost(t, providerType, "fixture")
				defer host.Reset()
				ctx := newUploadTestContext()
				uploadEmbeddingAndAnswer(ctx, cfg, "question", "private-answer", logger)
				calls := host.GetHttpCalloutAttributes()
				require.Len(t, calls, 1)
				require.Contains(t, string(calls[0].Body), "private-answer")
				require.NotPanics(t, func() {
					host.CallOnHttpCall([][2]string{{":status", "200"}}, []byte(`{"status":"ok"}`))
				})
				require.Empty(t, host.GetHttpCalloutAttributes())
				require.Empty(t, logger.warnings)
			})
		})
	}
}

func TestUploadEmbeddingAndAnswerAsyncError(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, cfg, logger := newUploadTestHost(t, "dashvector", "fixture")
		defer host.Reset()
		uploadEmbeddingAndAnswer(newUploadTestContext(), cfg, "question", "private-answer", logger)
		require.Len(t, host.GetHttpCalloutAttributes(), 1)
		require.NotPanics(t, func() {
			host.CallOnHttpCall([][2]string{{":status", "503"}}, []byte(`{"error":"unavailable"}`))
		})
		require.Len(t, logger.warnings, 1)
		require.Contains(t, logger.warnings[0], "failed to upload embedding: 503")
		require.NotContains(t, logger.warnings[0], "private-answer")
		require.Empty(t, host.GetHttpCalloutAttributes(), "an asynchronous error must not start a fallback or retry")
	})
}

func TestUploadEmbeddingAndAnswerSyncFallback(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, cfg, logger := newUploadTestHost(t, "dashvector", "fixture")
		defer host.Reset()
		ctx := newUploadTestContext()
		// Fail the answer upload's consumer lookup once. The public caller's
		// initial check and the subsequent embedding upload still see a consumer.
		// This injects a synchronous provider error without a production test hook.
		ctx.failConsumerRead = 2
		uploadEmbeddingAndAnswer(ctx, cfg, "question", "private-answer", logger)
		require.Len(t, logger.warnings, 1)
		require.Contains(t, logger.warnings[0], "authenticated consumer is required")
		calls := host.GetHttpCalloutAttributes()
		require.Len(t, calls, 1)
		require.Equal(t, "", gjson.GetBytes(calls[0].Body, "docs.0.fields.answer").String())
		require.Equal(t, "question", gjson.GetBytes(calls[0].Body, "docs.0.fields.query").String())
		require.NotPanics(t, func() {
			host.CallOnHttpCall([][2]string{{":status", "200"}}, []byte(`{"code":0}`))
		})
		require.Empty(t, host.GetHttpCalloutAttributes())
		require.Len(t, logger.warnings, 1)
	})
}

func TestUploadEmbeddingAndAnswerSyncErrors(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, cfg, logger := newUploadTestHost(t, "dashvector", "bad%zz")
		defer host.Reset()
		// Both real upload methods return a URL parsing error before dispatch.
		uploadEmbeddingAndAnswer(newUploadTestContext(), cfg, "question", "answer", logger)
		require.Empty(t, host.GetHttpCalloutAttributes())
		require.Len(t, logger.warnings, 2)
		require.Contains(t, logger.warnings[0], "failed to upload answer embedding")
		require.Contains(t, logger.warnings[1], "failed to upload embedding")
		for _, warning := range logger.warnings {
			require.Contains(t, warning, "invalid URL escape")
		}
	})
}

func TestUploadEmbeddingAndAnswerSkips(t *testing.T) {
	for _, name := range []string{"no consumer", "no embedding", "wrong embedding type", "no vector provider"} {
		t.Run(name, func(t *testing.T) {
			test.RunGoTest(t, func(t *testing.T) {
				host, cfg, logger := newUploadTestHost(t, "qdrant", "fixture")
				defer host.Reset()
				ctx := newUploadTestContext()
				switch name {
				case "no consumer":
					ctx.values[vector.ConsumerContextKey] = ""
				case "no embedding":
					delete(ctx.values, CACHE_KEY_EMBEDDING_KEY)
				case "wrong embedding type":
					ctx.values[CACHE_KEY_EMBEDDING_KEY] = "invalid"
				case "no vector provider":
					cfg = config.PluginConfig{}
				}
				uploadEmbeddingAndAnswer(ctx, cfg, "question", "answer", logger)
				require.Empty(t, host.GetHttpCalloutAttributes())
			})
		})
	}
}

func TestUploadEmbeddingAndAnswerRequestPath(t *testing.T) {
	for _, providerType := range []string{"dashvector", "qdrant"} {
		t.Run(providerType, func(t *testing.T) {
			test.RunTest(t, func(t *testing.T) {
				host, status := test.NewTestHost(uploadTestConfig(providerType, "fixture"))
				defer host.Reset()
				require.Equal(t, types.OnPluginStartStatusOK, status)
				host.CallOnHttpRequestHeaders([][2]string{
					{":method", "POST"}, {":path", "/v1/chat/completions"}, {":authority", "fixture"},
					{"content-type", "application/json"}, {"x-mse-consumer", "fixture-consumer"},
				})
				require.Equal(t, types.ActionPause, host.CallOnHttpRequestBody([]byte(`{"messages":[{"role":"user","content":"question"}],"stream":false}`)))
				require.Len(t, host.GetHttpCalloutAttributes(), 1)
				host.CallOnHttpCall([][2]string{{":status", "200"}}, []byte(`{"output":{"embeddings":[{"embedding":[0.1,0.2]}]}}`))
				require.Len(t, host.GetHttpCalloutAttributes(), 1)
				miss := `{"code":0,"output":[]}`
				if providerType == "qdrant" {
					miss = `{"status":"ok","result":[]}`
				}
				host.CallOnHttpCall([][2]string{{":status", "200"}}, []byte(miss))
				require.Empty(t, host.GetHttpCalloutAttributes())
				host.CallOnHttpResponseHeaders([][2]string{{":status", "200"}, {"content-type", "application/json"}})
				answer := []byte(`{"choices":[{"message":{"role":"assistant","content":"answer"}}]}`)
				require.Equal(t, types.ActionContinue, host.CallOnHttpResponseBody(answer))
				require.Len(t, host.GetHttpCalloutAttributes(), 1)
				require.NotPanics(t, func() {
					host.CallOnHttpCall([][2]string{{":status", "200"}}, []byte(`{"status":"ok"}`))
				})
				require.Empty(t, host.GetHttpCalloutAttributes())
				require.Equal(t, answer, host.GetResponseBody())
				require.Nil(t, host.GetLocalResponse())
			})
		})
	}
}

func uploadTestConfig(providerType, collection string) json.RawMessage {
	body, _ := json.Marshal(map[string]interface{}{
		"embedding": map[string]interface{}{
			"type": "dashscope", "apiKey": "fixture-key", "serviceName": "embedding.static", "servicePort": 8080,
		},
		"vector": map[string]interface{}{
			"type": providerType, "apiKey": "fixture-key", "collectionID": collection,
			"serviceName": "vector.static", "serviceHost": "vector.fixture", "servicePort": 8081,
			"threshold": 0.8, "thresholdRelation": "gt",
		},
		"enableSemanticCache": true,
	})
	return body
}

func newUploadTestHost(t *testing.T, providerType, collection string) (test.TestHost, config.PluginConfig, *uploadTestLog) {
	t.Helper()
	body := uploadTestConfig(providerType, collection)
	host, status := test.NewTestHost(body)
	require.Equal(t, types.OnPluginStartStatusOK, status)
	host.CallOnHttpRequestHeaders([][2]string{
		{":method", "POST"}, {":path", "/v1/chat/completions"}, {":authority", "fixture"},
		{"content-type", "application/json"}, {"x-mse-consumer", "fixture-consumer"},
	})
	logger := &uploadTestLog{}
	var cfg config.PluginConfig
	cfg.FromJson(gjson.ParseBytes(body), logger)
	require.NoError(t, cfg.Validate())
	require.NoError(t, cfg.Complete(logger))
	return host, cfg, logger
}

type uploadTestContext struct {
	wrapper.HttpContext
	values           map[string]interface{}
	consumerReads    int
	failConsumerRead int
}

func newUploadTestContext() *uploadTestContext {
	return &uploadTestContext{values: map[string]interface{}{
		vector.ConsumerContextKey: "fixture-consumer",
		CACHE_KEY_EMBEDDING_KEY:   []float64{0.1, 0.2},
	}}
}

func (c *uploadTestContext) GetContext(key string) interface{} {
	if key == vector.ConsumerContextKey {
		c.consumerReads++
		if c.consumerReads == c.failConsumerRead {
			return ""
		}
	}
	return c.values[key]
}

type uploadTestLog struct {
	logs.Log
	warnings []string
}

func (*uploadTestLog) Debugf(string, ...interface{}) {}
func (*uploadTestLog) Info(string)                   {}
func (*uploadTestLog) Infof(string, ...interface{})  {}
func (*uploadTestLog) Errorf(string, ...interface{}) {}
func (l *uploadTestLog) Warnf(format string, args ...interface{}) {
	l.warnings = append(l.warnings, strings.TrimSpace(fmt.Sprintf(format, args...)))
}
