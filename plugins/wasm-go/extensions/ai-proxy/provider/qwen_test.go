package provider

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestChatMessage2QwenMessagePreservesReasoningContent(t *testing.T) {
	t.Run("string content", func(t *testing.T) {
		msg := chatMessage{
			Role:             "assistant",
			Content:          "visible answer",
			ReasoningContent: "preserved reasoning",
			ToolCalls: []toolCall{
				{
					Id:   "call_1",
					Type: "function",
					Function: functionCall{
						Name:      "lookup",
						Arguments: `{"q":"weather"}`,
					},
				},
			},
		}

		qwenMsg := chatMessage2QwenMessage(msg)

		assert.Equal(t, "assistant", qwenMsg.Role)
		assert.Equal(t, "visible answer", qwenMsg.Content)
		assert.Equal(t, "preserved reasoning", qwenMsg.ReasoningContent)
		require.Len(t, qwenMsg.ToolCalls, 1)
		assert.Equal(t, "call_1", qwenMsg.ToolCalls[0].Id)
	})

	t.Run("array content", func(t *testing.T) {
		msg := chatMessage{
			Role: "assistant",
			Content: []any{
				map[string]any{
					"type": "text",
					"text": "visible answer",
				},
			},
			ReasoningContent: "preserved reasoning",
		}

		qwenMsg := chatMessage2QwenMessage(msg)

		assert.Equal(t, "assistant", qwenMsg.Role)
		assert.Equal(t, "preserved reasoning", qwenMsg.ReasoningContent)
		contents, ok := qwenMsg.Content.([]qwenVlMessageContent)
		require.True(t, ok)
		require.Len(t, contents, 1)
		assert.Equal(t, "visible answer", contents[0].Text)
	})

	t.Run("array image content", func(t *testing.T) {
		msg := chatMessage{
			Role: "assistant",
			Content: []any{
				map[string]any{
					"type": "image_url",
					"image_url": map[string]any{
						"url": "https://example.com/image.png",
					},
				},
			},
			ReasoningContent: "preserved reasoning",
		}

		qwenMsg := chatMessage2QwenMessage(msg)

		assert.Equal(t, "preserved reasoning", qwenMsg.ReasoningContent)
		contents, ok := qwenMsg.Content.([]qwenVlMessageContent)
		require.True(t, ok)
		require.Len(t, contents, 1)
		assert.Equal(t, "https://example.com/image.png", contents[0].Image)
	})
}

func TestBuildQwenTextGenerationRequestEnablesPreserveThinkingForReasoningHistory(t *testing.T) {
	provider := &qwenProvider{}
	request := &chatCompletionRequest{
		Model: "qwen-plus",
		Messages: []chatMessage{
			{Role: "assistant", Content: "visible answer", ReasoningContent: "historical reasoning"},
		},
		MaxTokens: 256,
	}

	body, err := provider.buildQwenTextGenerationRequest(nil, request, false)
	require.NoError(t, err)

	var qwenRequest qwenTextGenRequest
	require.NoError(t, json.Unmarshal(body, &qwenRequest))
	assert.True(t, qwenRequest.Parameters.PreserveThinking)
}

func TestBuildQwenTextGenerationRequestOmitsPreserveThinkingWithoutReasoningHistory(t *testing.T) {
	provider := &qwenProvider{}
	request := &chatCompletionRequest{
		Model: "qwen-plus",
		Messages: []chatMessage{
			{Role: "assistant", Content: "visible answer"},
		},
		MaxTokens: 256,
	}

	body, err := provider.buildQwenTextGenerationRequest(nil, request, false)
	require.NoError(t, err)

	assert.False(t, gjson.GetBytes(body, "parameters.preserve_thinking").Exists())
}

func TestTransformRequestBodyHeadersCompatibleModeEnablesPreserveThinkingForReasoningHistory(t *testing.T) {
	provider := &qwenProvider{
		config: ProviderConfig{
			qwenEnableCompatible: true,
		},
	}

	body := []byte(`{
		"model":"qwen-plus",
		"messages":[
			{"role":"assistant","content":"visible answer","reasoning_content":"historical reasoning"}
		]
	}`)

	modifiedBody, err := provider.TransformRequestBodyHeaders(nil, ApiNameChatCompletion, body, http.Header{})
	require.NoError(t, err)
	assert.Equal(t, true, gjson.GetBytes(modifiedBody, "preserve_thinking").Bool())
}

func TestTransformRequestBodyHeadersCompatibleModeOmitsPreserveThinkingWithoutReasoningHistory(t *testing.T) {
	provider := &qwenProvider{
		config: ProviderConfig{
			qwenEnableCompatible: true,
		},
	}

	body := []byte(`{
		"model":"qwen-plus",
		"messages":[
			{"role":"assistant","content":"visible answer"}
		]
	}`)

	modifiedBody, err := provider.TransformRequestBodyHeaders(nil, ApiNameChatCompletion, body, http.Header{})
	require.NoError(t, err)
	assert.False(t, gjson.GetBytes(modifiedBody, "preserve_thinking").Exists())
}
