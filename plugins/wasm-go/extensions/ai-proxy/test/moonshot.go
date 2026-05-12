package test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

var basicMoonshotConfig = func() json.RawMessage {
	return LegacyProviderPluginJSON(map[string]interface{}{
		"type":      "moonshot",
		"apiTokens": []string{"sk-moonshot-test"},
		"modelMapping": map[string]string{
			"*": "moonshot-v1-8k",
		},
	})
}()

var invalidMoonshotConfig = func() json.RawMessage {
	return LegacyProviderPluginJSON(map[string]interface{}{
		"type":         "moonshot",
		"apiTokens":    []string{},
		"modelMapping": map[string]string{"*": "moonshot-v1-8k"},
	})
}()

// RunMoonshotParseConfigTests exercises Moonshot plugin config loading.
func RunMoonshotParseConfigTests(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		t.Run("basic moonshot config", func(t *testing.T) {
			host, status := test.NewTestHost(basicMoonshotConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)
			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})
		t.Run("invalid moonshot config missing apiToken", func(t *testing.T) {
			host, status := test.NewTestHost(invalidMoonshotConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})
	})
}

// RunMoonshotOnHttpRequestHeadersTests exercises Moonshot request header transforms.
func RunMoonshotOnHttpRequestHeadersTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("moonshot chat completions headers", func(t *testing.T) {
			host, status := test.NewTestHost(basicMoonshotConfig)
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
			require.Equal(t, "api.moonshot.cn", hostValue)

			authValue, ok := test.GetHeaderValue(requestHeaders, "Authorization")
			require.True(t, ok)
			require.Contains(t, authValue, "Bearer sk-moonshot-test")

			pathValue, ok := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, ok)
			require.Equal(t, "/v1/chat/completions", pathValue)

			debugLogs := host.GetDebugLogs()
			found := false
			for _, log := range debugLogs {
				if strings.Contains(log, "moonshot") || strings.Contains(log, "ai-proxy") {
					found = true
					break
				}
			}
			require.True(t, found, "expected ai-proxy or moonshot debug logs")
		})
	})
}
