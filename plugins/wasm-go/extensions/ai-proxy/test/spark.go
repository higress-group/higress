package test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

var basicSparkConfig = func() json.RawMessage {
	return LegacyProviderPluginJSON(map[string]interface{}{
		"type":      "spark",
		"apiTokens": []string{"spark-test-token"},
		"modelMapping": map[string]string{
			"*": "generalv3.5",
		},
	})
}()

// RunSparkParseConfigTests exercises Spark plugin config loading (ValidateConfig is a no-op).
func RunSparkParseConfigTests(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		t.Run("basic spark config", func(t *testing.T) {
			host, status := test.NewTestHost(basicSparkConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)
			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})
	})
}

// RunSparkOnHttpRequestHeadersTests exercises Spark request header transforms.
func RunSparkOnHttpRequestHeadersTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("spark chat completions headers", func(t *testing.T) {
			host, status := test.NewTestHost(basicSparkConfig)
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
			require.Equal(t, "spark-api-open.xf-yun.com", hostValue)

			authValue, ok := test.GetHeaderValue(requestHeaders, "Authorization")
			require.True(t, ok)
			require.Contains(t, authValue, "Bearer spark-test-token")

			pathValue, ok := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, ok)
			require.Equal(t, "/v1/chat/completions", pathValue)

			debugLogs := host.GetDebugLogs()
			found := false
			for _, log := range debugLogs {
				if strings.Contains(log, "spark") || strings.Contains(log, "ai-proxy") {
					found = true
					break
				}
			}
			require.True(t, found, "expected ai-proxy or spark debug logs")
		})
	})
}
