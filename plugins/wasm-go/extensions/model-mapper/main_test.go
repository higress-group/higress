package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Basic configs for wasm test host
var (
	basicConfig = func() json.RawMessage {
		data, _ := json.Marshal(map[string]interface{}{
			"modelKey": "model",
			"modelMapping": map[string]string{
				"gpt-3.5-turbo": "gpt-4",
			},
			"enableOnPathSuffix": []string{
				"/v1/chat/completions",
			},
		})
		return data
	}()

	customConfig = func() json.RawMessage {
		data, _ := json.Marshal(map[string]interface{}{
			"modelKey": "request.model",
			"modelMapping": map[string]string{
				"*":          "gpt-4o",
				"gpt-3.5*":   "gpt-4-mini",
				"gpt-3.5-t":  "gpt-4-turbo",
				"gpt-3.5-t1": "gpt-4-turbo-1",
			},
			"enableOnPathSuffix": []string{
				"/v1/chat/completions",
				"/v1/embeddings",
			},
		})
		return data
	}()
)

func TestParseConfig(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		t.Run("basic config with defaults", func(t *testing.T) {
			var cfg Config
			jsonData := []byte(`{
				"modelMapping": {
					"gpt-3.5-turbo": "gpt-4",
					"gpt-4*": "gpt-4o-mini",
					"*": "gpt-4o"
				}
			}`)
			err := parseConfig(gjson.ParseBytes(jsonData), &cfg)
			require.NoError(t, err)

			// default modelKey
			require.Equal(t, "model", cfg.modelKey)
			// exact mapping
			require.Equal(t, "gpt-4", cfg.exactModelMapping["gpt-3.5-turbo"])
			// prefix mapping
			require.Len(t, cfg.prefixModelMapping, 1)
			require.Equal(t, "gpt-4", cfg.prefixModelMapping[0].Prefix)
			// default model
			require.Equal(t, "gpt-4o", cfg.defaultModel)
			// default enabled path suffixes
			require.Contains(t, cfg.enableOnPathSuffix, "/completions")
			require.Contains(t, cfg.enableOnPathSuffix, "/embeddings")
		})

		t.Run("custom modelKey and enableOnPathSuffix", func(t *testing.T) {
			var cfg Config
			jsonData := []byte(`{
				"modelKey": "request.model",
				"modelMapping": {
					"gpt-3.5-turbo": "gpt-4",
					"gpt-3.5*": "gpt-4-mini"
				},
				"enableOnPathSuffix": ["/v1/chat/completions", "/v1/embeddings"]
			}`)
			err := parseConfig(gjson.ParseBytes(jsonData), &cfg)
			require.NoError(t, err)

			require.Equal(t, "request.model", cfg.modelKey)
			require.Equal(t, "gpt-4", cfg.exactModelMapping["gpt-3.5-turbo"])
			require.Len(t, cfg.prefixModelMapping, 1)
			require.Equal(t, "gpt-3.5", cfg.prefixModelMapping[0].Prefix)
			require.Equal(t, "gpt-4-mini", cfg.prefixModelMapping[0].Target)
			require.Equal(t, 2, len(cfg.enableOnPathSuffix))
			require.Contains(t, cfg.enableOnPathSuffix, "/v1/chat/completions")
			require.Contains(t, cfg.enableOnPathSuffix, "/v1/embeddings")
		})

		t.Run("enableResponseMapping defaults to false", func(t *testing.T) {
			var cfg Config
			jsonData := []byte(`{
				"modelMapping": {
					"gpt-3.5-turbo": "gpt-4"
				}
			}`)
			err := parseConfig(gjson.ParseBytes(jsonData), &cfg)
			require.NoError(t, err)
			require.False(t, cfg.enableResponseMapping)
		})

		t.Run("enableResponseMapping can be disabled", func(t *testing.T) {
			var cfg Config
			jsonData := []byte(`{
				"enableResponseMapping": false,
				"modelMapping": {
					"gpt-3.5-turbo": "gpt-4"
				}
			}`)
			err := parseConfig(gjson.ParseBytes(jsonData), &cfg)
			require.NoError(t, err)
			require.False(t, cfg.enableResponseMapping)
		})

		t.Run("enableResponseMapping must be boolean", func(t *testing.T) {
			var cfg Config
			jsonData := []byte(`{
				"enableResponseMapping": "false"
			}`)
			err := parseConfig(gjson.ParseBytes(jsonData), &cfg)
			require.Error(t, err)
		})

		t.Run("modelMapping must be object", func(t *testing.T) {
			var cfg Config
			jsonData := []byte(`{
				"modelMapping": "invalid"
			}`)
			err := parseConfig(gjson.ParseBytes(jsonData), &cfg)
			require.Error(t, err)
		})

		t.Run("enableOnPathSuffix must be array", func(t *testing.T) {
			var cfg Config
			jsonData := []byte(`{
				"enableOnPathSuffix": "not-array"
			}`)
			err := parseConfig(gjson.ParseBytes(jsonData), &cfg)
			require.Error(t, err)
		})

		t.Run("responseModelPaths defaults", func(t *testing.T) {
			var cfg Config
			jsonData := []byte(`{
				"modelMapping": {
					"gpt-3.5-turbo": "gpt-4"
				}
			}`)
			err := parseConfig(gjson.ParseBytes(jsonData), &cfg)
			require.NoError(t, err)
			require.Equal(t, 5, len(cfg.responseModelPaths))
			require.Equal(t, "model", cfg.responseModelPaths[0])
			require.Equal(t, "body.model", cfg.responseModelPaths[1])
			require.Equal(t, "message.model", cfg.responseModelPaths[3])
			require.Equal(t, "modelVersion", cfg.responseModelPaths[4])
		})

		t.Run("responseModelPaths append merges with defaults", func(t *testing.T) {
			var cfg Config
			jsonData := []byte(`{
				"modelMapping": {
					"gpt-3.5-turbo": "gpt-4"
				},
				"responseModelPaths": ["data.model", "model", "result.model"]
			}`)
			err := parseConfig(gjson.ParseBytes(jsonData), &cfg)
			require.NoError(t, err)
			require.Equal(t, 7, len(cfg.responseModelPaths))
			require.Equal(t, "model", cfg.responseModelPaths[0])
			require.Equal(t, "data.model", cfg.responseModelPaths[5])
			require.Equal(t, "result.model", cfg.responseModelPaths[6])
		})

		t.Run("responseModelPaths must be array", func(t *testing.T) {
			var cfg Config
			jsonData := []byte(`{
				"responseModelPaths": "invalid"
			}`)
			err := parseConfig(gjson.ParseBytes(jsonData), &cfg)
			require.Error(t, err)
		})
	})
}

func TestRewriteModelFieldInJSONBytes(t *testing.T) {
	t.Run("rewrite top-level model", func(t *testing.T) {
		payload := []byte(`{"model":"gpt-4","id":"x"}`)
		newPayload, rewritten, err := rewriteModelFieldInJSONBytes(payload, []string{"model"}, "gpt-4", "gpt-3.5-turbo")
		require.NoError(t, err)
		require.True(t, rewritten)
		require.Equal(t, "gpt-3.5-turbo", gjson.GetBytes(newPayload, "model").String())
	})

	t.Run("rewrite nested message.model", func(t *testing.T) {
		payload := []byte(`{"message":{"model":"gpt-4","id":"m1"}}`)
		newPayload, rewritten, err := rewriteModelFieldInJSONBytes(payload, []string{"message.model"}, "gpt-4", "gpt-3.5-turbo")
		require.NoError(t, err)
		require.True(t, rewritten)
		require.Equal(t, "gpt-3.5-turbo", gjson.GetBytes(newPayload, "message.model").String())
	})

	t.Run("invalid json does not fail", func(t *testing.T) {
		payload := []byte(`{"model":`)
		newPayload, rewritten, err := rewriteModelFieldInJSONBytes(payload, []string{"model"}, "gpt-4", "gpt-3.5-turbo")
		require.NoError(t, err)
		require.False(t, rewritten)
		require.Equal(t, payload, newPayload)
	})

	t.Run("first-match-wins across multiple paths", func(t *testing.T) {
		payload := []byte(`{"message":{"model":"gpt-4"},"model":"other"}`)
		// "model" matches first but value is "other", then "message.model" matches with correct value
		paths := []string{"model", "message.model"}
		newPayload, rewritten, err := rewriteModelFieldInJSONBytes(payload, paths, "gpt-4", "gpt-3.5-turbo")
		require.NoError(t, err)
		require.True(t, rewritten)
		require.Equal(t, "gpt-3.5-turbo", gjson.GetBytes(newPayload, "message.model").String())
		require.Equal(t, "other", gjson.GetBytes(newPayload, "model").String())
	})

	t.Run("no path matches returns unchanged", func(t *testing.T) {
		payload := []byte(`{"model":"unknown"}`)
		newPayload, rewritten, err := rewriteModelFieldInJSONBytes(payload, []string{"model", "message.model"}, "gpt-4", "gpt-3.5-turbo")
		require.NoError(t, err)
		require.False(t, rewritten)
		require.Equal(t, payload, newPayload)
	})
}

func TestRewriteSseEvent(t *testing.T) {
	t.Run("rewrite data json and keep done", func(t *testing.T) {
		raw := "event: message\n" +
			"data: {\"model\":\"gpt-4\",\"id\":\"1\"}\n" +
			"data: [DONE]\n"
		rewritten := rewriteSseEvent(raw, []string{"model"}, "gpt-4", "gpt-3.5-turbo")
		require.Contains(t, rewritten, `data: {"model":"gpt-3.5-turbo","id":"1"}`)
		require.Contains(t, rewritten, "data: [DONE]")
	})

	t.Run("invalid data line stays unchanged", func(t *testing.T) {
		raw := "data: not-json\n"
		rewritten := rewriteSseEvent(raw, []string{"model"}, "gpt-4", "gpt-3.5-turbo")
		require.Equal(t, raw, rewritten)
	})
}

func TestFindSseEventSeparator(t *testing.T) {
	pos, sep := findSseEventSeparator("data: 1\n\ndata: 2\n\n")
	require.Equal(t, 7, pos)
	require.Equal(t, 2, sep)

	pos, sep = findSseEventSeparator("data: 1\r\n\r\ndata: 2\r\n\r\n")
	require.Equal(t, 7, pos)
	require.Equal(t, 4, sep)

	pos, sep = findSseEventSeparator("data: 1\n")
	require.Equal(t, -1, pos)
	require.Equal(t, 0, sep)
}

func TestOnHttpRequestHeaders(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("skip when path not matched", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			originalHeaders := [][2]string{
				{":authority", "example.com"},
				{":path", "/v1/other"},
				{":method", "POST"},
				{"content-type", "application/json"},
				{"content-length", "123"},
			}
			action := host.CallOnHttpRequestHeaders(originalHeaders)
			require.Equal(t, types.ActionContinue, action)

			newHeaders := host.GetRequestHeaders()
			// content-length should still exist because path is not enabled
			foundContentLength := false
			for _, h := range newHeaders {
				if strings.ToLower(h[0]) == "content-length" {
					foundContentLength = true
					break
				}
			}
			require.True(t, foundContentLength)
		})

		t.Run("process when path and content-type match", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			originalHeaders := [][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"content-type", "application/json"},
				{"content-length", "123"},
			}
			action := host.CallOnHttpRequestHeaders(originalHeaders)
			require.Equal(t, types.HeaderStopIteration, action)

			newHeaders := host.GetRequestHeaders()
			// content-length should be removed
			for _, h := range newHeaders {
				require.NotEqual(t, strings.ToLower(h[0]), "content-length")
			}
		})
	})
}

func TestOnHttpRequestBody_ModelMapping(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("exact mapping", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			origBody := []byte(`{
				"model": "gpt-3.5-turbo",
				"messages": [{"role": "user", "content": "hello"}]
			}`)
			action := host.CallOnHttpRequestBody(origBody)
			require.Equal(t, types.ActionContinue, action)

			processed := host.GetRequestBody()
			require.NotNil(t, processed)
			require.Equal(t, "gpt-4", gjson.GetBytes(processed, "model").String())
		})

		t.Run("default model when key missing", func(t *testing.T) {
			// use customConfig where default model is set with "*"
			host, status := test.NewTestHost(customConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			origBody := []byte(`{
				"request": {
					"messages": [{"role": "user", "content": "hello"}]
				}
			}`)
			action := host.CallOnHttpRequestBody(origBody)
			require.Equal(t, types.ActionContinue, action)

			processed := host.GetRequestBody()
			require.NotNil(t, processed)
			// default model should be set at request.model
			require.Equal(t, "gpt-4o", gjson.GetBytes(processed, "request.model").String())
		})

		t.Run("prefix mapping takes effect", func(t *testing.T) {
			host, status := test.NewTestHost(customConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			origBody := []byte(`{
				"request": {
					"model": "gpt-3.5-turbo-16k",
					"messages": [{"role": "user", "content": "hello"}]
				}
			}`)
			action := host.CallOnHttpRequestBody(origBody)
			require.Equal(t, types.ActionContinue, action)

			processed := host.GetRequestBody()
			require.NotNil(t, processed)
			require.Equal(t, "gpt-4-mini", gjson.GetBytes(processed, "request.model").String())
		})
	})
}
