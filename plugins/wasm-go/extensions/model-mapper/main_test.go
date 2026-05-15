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

func getHeader(headers [][2]string, key string) (string, bool) {
	for _, h := range headers {
		if strings.EqualFold(h[0], key) {
			return h[1], true
		}
	}
	return "", false
}

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

	headerSyncConfig = func() json.RawMessage {
		data, _ := json.Marshal(map[string]interface{}{
			"modelKey": "model",
			"modelMapping": map[string]string{
				"gpt-3.5-turbo": "gpt-4",
			},
			"modelToHeader": "x-final-model",
			"enableOnPathSuffix": []string{
				"/v1/chat/completions",
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

		t.Run("modelToHeader default and custom", func(t *testing.T) {
			var cfgDefault Config
			require.NoError(t, parseConfig(gjson.ParseBytes([]byte(`{"modelMapping":{}}`)), &cfgDefault))
			require.Equal(t, "x-higress-llm-model-final", cfgDefault.modelToHeader)

			var cfgCustom Config
			err := parseConfig(gjson.ParseBytes([]byte(`{
				"modelToHeader": "x-my-model",
				"modelMapping": {}
			}`)), &cfgCustom)
			require.NoError(t, err)
			require.Equal(t, "x-my-model", cfgCustom.modelToHeader)
		})

		t.Run("empty modelMapping", func(t *testing.T) {
			var cfg Config
			err := parseConfig(gjson.ParseBytes([]byte(`{"modelMapping": {}}`)), &cfg)
			require.NoError(t, err)
			require.Empty(t, cfg.exactModelMapping)
			require.Empty(t, cfg.prefixModelMapping)
			require.Equal(t, "", cfg.defaultModel)
		})

		t.Run("prefix rules sorted by key for stable iteration", func(t *testing.T) {
			var cfg Config
			// Object key order in JSON is z then a; after sort, prefix "a" is tried before "z".
			jsonData := []byte(`{
				"modelMapping": {
					"z*": "Z",
					"a*": "A"
				}
			}`)
			require.NoError(t, parseConfig(gjson.ParseBytes(jsonData), &cfg))
			require.Len(t, cfg.prefixModelMapping, 2)
			require.Equal(t, "a", cfg.prefixModelMapping[0].Prefix)
			require.Equal(t, "A", cfg.prefixModelMapping[0].Target)
			require.Equal(t, "z", cfg.prefixModelMapping[1].Prefix)
			require.Equal(t, "Z", cfg.prefixModelMapping[1].Target)
		})

		t.Run("exact mapping wins over prefix", func(t *testing.T) {
			var cfg Config
			jsonData := []byte(`{
				"modelKey": "model",
				"modelMapping": {
					"gpt-3.5*": "from-prefix",
					"gpt-3.5-turbo": "from-exact"
				}
			}`)
			require.NoError(t, parseConfig(gjson.ParseBytes(jsonData), &cfg))
			require.Equal(t, "from-exact", cfg.exactModelMapping["gpt-3.5-turbo"])
			require.Len(t, cfg.prefixModelMapping, 1)
			require.Equal(t, "gpt-3.5", cfg.prefixModelMapping[0].Prefix)
		})
	})
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
			_, foundContentLength := getHeader(newHeaders, "content-length")
			require.True(t, foundContentLength, "content-length should be kept when path is not enabled")
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
			_, foundCL := getHeader(newHeaders, "content-length")
			require.False(t, foundCL, "content-length should be removed when buffering body")
		})

		t.Run("path with query string still matches suffix", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions?trace=1"},
				{":method", "POST"},
				{"content-type", "application/json"},
				{"content-length", "99"},
			})
			require.Equal(t, types.HeaderStopIteration, action)
			_, foundCL := getHeader(host.GetRequestHeaders(), "content-length")
			require.False(t, foundCL)
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
			v, ok := getHeader(host.GetRequestHeaders(), "x-higress-llm-model-final")
			require.True(t, ok)
			require.Equal(t, "gpt-4", v)
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
			v, ok := getHeader(host.GetRequestHeaders(), "x-higress-llm-model-final")
			require.True(t, ok)
			require.Equal(t, "gpt-4o", v)
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
			v, ok := getHeader(host.GetRequestHeaders(), "x-higress-llm-model-final")
			require.True(t, ok)
			require.Equal(t, "gpt-4-mini", v)
		})

		t.Run("exact mapping beats prefix for same family", func(t *testing.T) {
			host, status := test.NewTestHost(customConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/embeddings"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			origBody := []byte(`{
				"request": {
					"model": "gpt-3.5-t1",
					"input": "hello"
				}
			}`)
			action := host.CallOnHttpRequestBody(origBody)
			require.Equal(t, types.ActionContinue, action)

			processed := host.GetRequestBody()
			require.NotNil(t, processed)
			require.Equal(t, "gpt-4-turbo-1", gjson.GetBytes(processed, "request.model").String())
			v, ok := getHeader(host.GetRequestHeaders(), "x-higress-llm-model-final")
			require.True(t, ok)
			require.Equal(t, "gpt-4-turbo-1", v)
		})

		t.Run("empty request body is a no-op", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			action := host.CallOnHttpRequestBody(nil)
			require.Equal(t, types.ActionContinue, action)
			require.Nil(t, host.GetRequestBody())

			action = host.CallOnHttpRequestBody([]byte{})
			require.Equal(t, types.ActionContinue, action)
		})

		t.Run("invalid json body is skipped", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"content-type", "application/json"},
				{"x-higress-llm-model-final", "should-not-change"},
			})

			bad := []byte(`not json`)
			action := host.CallOnHttpRequestBody(bad)
			require.Equal(t, types.ActionContinue, action)
			out := host.GetRequestBody()
			if out != nil {
				require.Equal(t, string(bad), string(out))
			}
			v, ok := getHeader(host.GetRequestHeaders(), "x-higress-llm-model-final")
			require.True(t, ok)
			require.Equal(t, "should-not-change", v, "invalid JSON must not refresh model header")
		})

		t.Run("no body rewrite when already mapped target but header still refreshed", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			origBody := []byte(`{"model":"gpt-4","messages":[]}`)
			action := host.CallOnHttpRequestBody(origBody)
			require.Equal(t, types.ActionContinue, action)
			out := host.GetRequestBody()
			if out != nil {
				require.Equal(t, string(origBody), string(out))
			}
			v, ok := getHeader(host.GetRequestHeaders(), "x-higress-llm-model-final")
			require.True(t, ok)
			require.Equal(t, "gpt-4", v)
		})

		t.Run("modelToHeader always set to resolved model", func(t *testing.T) {
			host, status := test.NewTestHost(headerSyncConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"content-type", "application/json"},
				{"x-final-model", "gpt-3.5-turbo"},
			})

			origBody := []byte(`{"model":"gpt-3.5-turbo"}`)
			require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody(origBody))

			processed := host.GetRequestBody()
			require.NotNil(t, processed)
			require.Equal(t, "gpt-4", gjson.GetBytes(processed, "model").String())

			v, ok := getHeader(host.GetRequestHeaders(), "x-final-model")
			require.True(t, ok)
			require.Equal(t, "gpt-4", v)
		})

		t.Run("modelToHeader refreshed even when it already matches resolved model", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"content-type", "application/json"},
				{"x-higress-llm-model-final", "gpt-4"},
			})

			origBody := []byte(`{"model":"gpt-3.5-turbo","messages":[]}`)
			require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody(origBody))

			processed := host.GetRequestBody()
			require.NotNil(t, processed)
			require.Equal(t, "gpt-4", gjson.GetBytes(processed, "model").String())
			v, ok := getHeader(host.GetRequestHeaders(), "x-higress-llm-model-final")
			require.True(t, ok)
			require.Equal(t, "gpt-4", v)
		})
	})
}
