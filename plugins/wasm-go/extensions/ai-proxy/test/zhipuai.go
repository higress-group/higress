package test

import (
	"encoding/json"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var basicZhipuAIConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":      "zhipuai",
			"apiTokens": []string{"sk-zhipuai-test"},
		},
	})
	return data
}()

func RunZhipuAIClaudeAutoConversionTests(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		t.Run("claude thinking enabled sets thinking enabled for zhipuai", func(t *testing.T) {
			host, status := test.NewTestHost(basicZhipuAIConfig)
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
				"model": "glm-4",
				"max_tokens": 1000,
				"messages": [{"role": "user", "content": "Hello"}],
				"thinking": {"type": "enabled", "budget_tokens": 8192}
			}`

			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			transformedBody := host.GetRequestBody()
			require.NotNil(t, transformedBody)

			var bodyMap map[string]interface{}
			err := json.Unmarshal(transformedBody, &bodyMap)
			require.NoError(t, err)

			// ZhipuAI should have thinking=enabled (converted from reasoning_effort)
			thinking, ok := bodyMap["thinking"].(map[string]interface{})
			require.True(t, ok, "thinking field should be present")
			assert.Equal(t, "enabled", thinking["type"])

			// reasoning_effort should be removed (ZhipuAI doesn't recognize it)
			assert.NotContains(t, bodyMap, "reasoning_effort")
		})

		t.Run("claude without thinking sets thinking disabled for zhipuai", func(t *testing.T) {
			host, status := test.NewTestHost(basicZhipuAIConfig)
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
				"model": "glm-4",
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

			// ZhipuAI should explicitly set thinking=disabled
			thinking, ok := bodyMap["thinking"].(map[string]interface{})
			require.True(t, ok, "thinking field should be present for disabled state")
			assert.Equal(t, "disabled", thinking["type"])
		})

		t.Run("claude thinking disabled sets thinking disabled for zhipuai", func(t *testing.T) {
			host, status := test.NewTestHost(basicZhipuAIConfig)
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
				"model": "glm-4",
				"max_tokens": 1000,
				"messages": [{"role": "user", "content": "Hello"}],
				"thinking": {"type": "disabled"}
			}`

			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			transformedBody := host.GetRequestBody()
			require.NotNil(t, transformedBody)

			var bodyMap map[string]interface{}
			err := json.Unmarshal(transformedBody, &bodyMap)
			require.NoError(t, err)

			// ZhipuAI should explicitly set thinking=disabled
			thinking, ok := bodyMap["thinking"].(map[string]interface{})
			require.True(t, ok, "thinking field should be present for disabled state")
			assert.Equal(t, "disabled", thinking["type"])

			// No reasoning fields
			assert.NotContains(t, bodyMap, "reasoning_effort")
		})
	})
}
