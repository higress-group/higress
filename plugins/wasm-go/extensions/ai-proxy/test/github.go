package test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

var basicGithubConfig = func() json.RawMessage {
	return LegacyProviderPluginJSON(map[string]interface{}{
		"type":      "github",
		"apiTokens": []string{"github_models_pat_test"},
		"modelMapping": map[string]string{
			"*": "gpt-4o",
		},
	})
}()

var invalidGithubConfig = func() json.RawMessage {
	return LegacyProviderPluginJSON(map[string]interface{}{
		"type":         "github",
		"apiTokens":    []string{},
		"modelMapping": map[string]string{"*": "gpt-4o"},
	})
}()

// RunGithubParseConfigTests exercises GitHub Models plugin config loading.
func RunGithubParseConfigTests(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		t.Run("basic github config", func(t *testing.T) {
			host, status := test.NewTestHost(basicGithubConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)
			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})
		t.Run("invalid github config missing apiToken", func(t *testing.T) {
			host, status := test.NewTestHost(invalidGithubConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})
	})
}

// RunGithubOnHttpRequestHeadersTests exercises GitHub Models request header transforms.
func RunGithubOnHttpRequestHeadersTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("github chat completions headers", func(t *testing.T) {
			host, status := test.NewTestHost(basicGithubConfig)
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
			require.Equal(t, "models.inference.ai.azure.com", hostValue)

			authValue, ok := test.GetHeaderValue(requestHeaders, "Authorization")
			require.True(t, ok)
			// GitHub provider sets raw token without "Bearer " prefix
			require.Equal(t, "github_models_pat_test", authValue)

			pathValue, ok := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, ok)
			require.Equal(t, "/chat/completions", pathValue)

			debugLogs := host.GetDebugLogs()
			found := false
			for _, log := range debugLogs {
				if strings.Contains(log, "github") || strings.Contains(log, "ai-proxy") {
					found = true
					break
				}
			}
			require.True(t, found, "expected ai-proxy or github debug logs")
		})
	})
}
