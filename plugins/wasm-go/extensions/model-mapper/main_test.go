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
