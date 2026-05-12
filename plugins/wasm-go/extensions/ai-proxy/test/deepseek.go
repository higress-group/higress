package test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

var basicDeepSeekConfig = func() json.RawMessage {
	return LegacyProviderPluginJSON(map[string]interface{}{
		"type":      "deepseek",
		"apiTokens": []string{"sk-deepseek-test"},
		"modelMapping": map[string]string{
			"*": "deepseek-chat",
		},
	})
}()

var invalidDeepSeekConfig = func() json.RawMessage {
	return LegacyProviderPluginJSON(map[string]interface{}{
		"type":         "deepseek",
		"apiTokens":    []string{},
		"modelMapping": map[string]string{"*": "deepseek-chat"},
	})
}()

// RunDeepSeekParseConfigTests exercises DeepSeek plugin config loading.
func RunDeepSeekParseConfigTests(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		t.Run("basic deepseek config", func(t *testing.T) {
			host, status := test.NewTestHost(basicDeepSeekConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)
			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})
		t.Run("invalid deepseek config missing apiToken", func(t *testing.T) {
			host, status := test.NewTestHost(invalidDeepSeekConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})
	})
}

// RunDeepSeekOnHttpRequestHeadersTests exercises request header transforms for DeepSeek.
func RunDeepSeekOnHttpRequestHeadersTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("deepseek chat completions headers", func(t *testing.T) {
			host, status := test.NewTestHost(basicDeepSeekConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			requestHeaders := host.GetRequestHeaders()
			require.NotNil(t, requestHeaders)

			hostValue, ok := test.GetHeaderValue(requestHeaders, ":authority")
			require.True(t, ok)
			require.Equal(t, "api.deepseek.com", hostValue)

			authValue, ok := test.GetHeaderValue(requestHeaders, "Authorization")
			require.True(t, ok)
			require.Contains(t, authValue, "Bearer sk-deepseek-test")

			pathValue, ok := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, ok)
			require.Equal(t, "/v1/chat/completions", pathValue)

			debugLogs := host.GetDebugLogs()
			found := false
			for _, log := range debugLogs {
				if strings.Contains(log, "deepseek") || strings.Contains(log, "ai-proxy") {
					found = true
					break
				}
			}
			require.True(t, found, "expected ai-proxy or deepseek debug logs")
		})
	})
}
