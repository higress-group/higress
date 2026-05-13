package test

import (
	"encoding/json"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var basicOpenRouterConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":      "openrouter",
			"apiTokens": []string{"sk-openrouter-test"},
		},
	})
	return data
}()

func RunOpenRouterClaudeAutoConversionTests(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		t.Run("claude thinking budget_tokens is converted to reasoning.max_tokens", func(t *testing.T) {
			host, status := test.NewTestHost(basicOpenRouterConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// Send request with Claude /v1/messages path to trigger auto-conversion
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/messages"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			// Claude request body with thinking enabled
			requestBody := `{
				"model": "anthropic/claude-sonnet-4",
				"max_tokens": 8000,
				"messages": [{"role": "user", "content": "Hello"}],
				"thinking": {"type": "enabled", "budget_tokens": 10000}
			}`

			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			transformedBody := host.GetRequestBody()
			require.NotNil(t, transformedBody)

			var bodyMap map[string]interface{}
			err := json.Unmarshal(transformedBody, &bodyMap)
			require.NoError(t, err)

			// reasoning.max_tokens should be set from budget_tokens
			reasoning, ok := bodyMap["reasoning"].(map[string]interface{})
			require.True(t, ok, "reasoning field should be present")
			assert.Equal(t, float64(10000), reasoning["max_tokens"],
				"reasoning.max_tokens should preserve the original budget_tokens value")

			// reasoning_effort should be removed (OpenRouter uses reasoning.max_tokens instead)
			assert.NotContains(t, bodyMap, "reasoning_effort",
				"reasoning_effort should be removed")

			// Non-standard fields should not be present
			assert.NotContains(t, bodyMap, "thinking",
				"thinking should not be in the final request")
			assert.NotContains(t, bodyMap, "reasoning_max_tokens",
				"reasoning_max_tokens should not be in the final request")
		})

		t.Run("claude without thinking uses default transformation", func(t *testing.T) {
			host, status := test.NewTestHost(basicOpenRouterConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/messages"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			requestBody := `{
				"model": "anthropic/claude-sonnet-4",
				"max_tokens": 1000,
				"messages": [{"role": "user", "content": "Hello"}]
			}`

			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			transformedBody := host.GetRequestBody()
			require.NotNil(t, transformedBody)

			var bodyMap map[string]interface{}
			err := json.Unmarshal(transformedBody, &bodyMap)
			require.NoError(t, err)

			// No reasoning fields should be present
			assert.NotContains(t, bodyMap, "reasoning")
			assert.NotContains(t, bodyMap, "reasoning_effort")
			assert.NotContains(t, bodyMap, "thinking")
			assert.NotContains(t, bodyMap, "reasoning_max_tokens")
		})

		t.Run("claude thinking disabled does not set reasoning", func(t *testing.T) {
			host, status := test.NewTestHost(basicOpenRouterConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/messages"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			// thinking disabled with budget_tokens (dirty input)
			requestBody := `{
				"model": "anthropic/claude-sonnet-4",
				"max_tokens": 1000,
				"messages": [{"role": "user", "content": "Hello"}],
				"thinking": {"type": "disabled", "budget_tokens": 5000}
			}`

			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			transformedBody := host.GetRequestBody()
			require.NotNil(t, transformedBody)

			var bodyMap map[string]interface{}
			err := json.Unmarshal(transformedBody, &bodyMap)
			require.NoError(t, err)

			// Should NOT have reasoning.max_tokens since thinking was disabled
			assert.NotContains(t, bodyMap, "reasoning",
				"reasoning should not be set when thinking is disabled")
			assert.NotContains(t, bodyMap, "thinking")
			assert.NotContains(t, bodyMap, "reasoning_max_tokens")
		})
	})
}
