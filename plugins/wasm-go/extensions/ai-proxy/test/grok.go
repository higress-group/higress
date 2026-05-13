package test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

var basicGrokConfig = func() json.RawMessage {
	return LegacyProviderPluginJSON(map[string]interface{}{
		"type":      "grok",
		"apiTokens": []string{"xai-grok-test-key"},
		"modelMapping": map[string]string{
			"*": "grok-2-latest",
		},
	})
}()

var invalidGrokConfig = func() json.RawMessage {
	return LegacyProviderPluginJSON(map[string]interface{}{
		"type":         "grok",
		"apiTokens":    []string{},
		"modelMapping": map[string]string{"*": "grok-2-latest"},
	})
}()

// RunGrokParseConfigTests exercises Grok plugin config loading.
func RunGrokParseConfigTests(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		t.Run("basic grok config", func(t *testing.T) {
			host, status := test.NewTestHost(basicGrokConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)
			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})
		t.Run("invalid grok config missing apiToken", func(t *testing.T) {
			host, status := test.NewTestHost(invalidGrokConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})
	})
}

// RunGrokOnHttpRequestHeadersTests exercises Grok request header transforms.
func RunGrokOnHttpRequestHeadersTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("grok chat completions headers", func(t *testing.T) {
			host, status := test.NewTestHost(basicGrokConfig)
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
			require.Equal(t, "api.x.ai", hostValue)

			authValue, ok := test.GetHeaderValue(requestHeaders, "Authorization")
			require.True(t, ok)
			require.Contains(t, authValue, "Bearer xai-grok-test-key")

			pathValue, ok := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, ok)
			require.Equal(t, "/v1/chat/completions", pathValue)

			debugLogs := host.GetDebugLogs()
			found := false
			for _, log := range debugLogs {
				if strings.Contains(log, "grok") || strings.Contains(log, "ai-proxy") {
					found = true
					break
				}
			}
			require.True(t, found, "expected ai-proxy or grok debug logs")
		})
	})
}
