package test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

var basicTogetherAIConfig = func() json.RawMessage {
	return LegacyProviderPluginJSON(map[string]interface{}{
		"type":      "together-ai",
		"apiTokens": []string{"together-test-key"},
		"modelMapping": map[string]string{
			"*": "meta-llama/Llama-3-8b-chat-hf",
		},
	})
}()

var invalidTogetherAIConfig = func() json.RawMessage {
	return LegacyProviderPluginJSON(map[string]interface{}{
		"type":         "together-ai",
		"apiTokens":    []string{},
		"modelMapping": map[string]string{"*": "meta-llama/Llama-3-8b-chat-hf"},
	})
}()

// RunTogetherAIParseConfigTests exercises Together AI plugin config loading.
func RunTogetherAIParseConfigTests(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		t.Run("basic together-ai config", func(t *testing.T) {
			host, status := test.NewTestHost(basicTogetherAIConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)
			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})
		t.Run("invalid together-ai config missing apiToken", func(t *testing.T) {
			host, status := test.NewTestHost(invalidTogetherAIConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})
	})
}

// RunTogetherAIOnHttpRequestHeadersTests exercises Together AI request header transforms.
func RunTogetherAIOnHttpRequestHeadersTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("together-ai chat completions headers", func(t *testing.T) {
			host, status := test.NewTestHost(basicTogetherAIConfig)
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
			require.Equal(t, "api.together.xyz", hostValue)

			authValue, ok := test.GetHeaderValue(requestHeaders, "Authorization")
			require.True(t, ok)
			require.Contains(t, authValue, "Bearer together-test-key")

			pathValue, ok := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, ok)
			require.Equal(t, "/v1/chat/completions", pathValue)

			debugLogs := host.GetDebugLogs()
			found := false
			for _, log := range debugLogs {
				if strings.Contains(log, "together") || strings.Contains(log, "ai-proxy") {
					found = true
					break
				}
			}
			require.True(t, found, "expected ai-proxy or together debug logs")
		})
	})
}
