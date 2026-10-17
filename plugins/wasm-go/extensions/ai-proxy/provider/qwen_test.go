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
		Model: "qwen3.6-plus",
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

func TestBuildQwenTextGenerationRequestOmitsPreserveThinkingForUnsupportedModel(t *testing.T) {
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

	assert.False(t, gjson.GetBytes(body, "parameters.preserve_thinking").Exists())
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
		"model":"qwen3.6-plus",
		"messages":[
			{"role":"assistant","content":"visible answer","reasoning_content":"historical reasoning"}
		]
	}`)

	modifiedBody, err := provider.TransformRequestBodyHeaders(nil, ApiNameChatCompletion, body, http.Header{})
	require.NoError(t, err)
	assert.Equal(t, true, gjson.GetBytes(modifiedBody, "preserve_thinking").Bool())
}

func TestTransformRequestBodyHeadersCompatibleModeOmitsPreserveThinkingForUnsupportedModel(t *testing.T) {
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
	assert.False(t, gjson.GetBytes(modifiedBody, "preserve_thinking").Exists())
}

func TestTransformRequestBodyHeadersCompatibleModeEnablesPreserveThinkingAfterModelMapping(t *testing.T) {
	provider := &qwenProvider{
		config: ProviderConfig{
			qwenEnableCompatible: true,
			modelMapping: map[string]string{
				"alias-model": "qwen3.6-plus-2026-04-02",
			},
		},
	}

	body := []byte(`{
		"model":"alias-model",
		"messages":[
			{"role":"assistant","content":"visible answer","reasoning_content":"historical reasoning"}
		]
	}`)

	modifiedBody, err := provider.TransformRequestBodyHeaders(nil, ApiNameChatCompletion, body, http.Header{})
	require.NoError(t, err)
	assert.Equal(t, "qwen3.6-plus-2026-04-02", gjson.GetBytes(modifiedBody, "model").String())
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

// TestQwenDefaultCapabilitiesIncludeResponsesInBothModes guards the behaviour
// reported in the issue where the default Tongyi Qianwen provider did not
// support the Responses API. DashScope has no native Responses endpoint, so the
// capability must be present even when compatible mode is disabled, and it must
// always point at the OpenAI-compatible endpoint on the same host.
func TestQwenDefaultCapabilitiesIncludeResponsesInBothModes(t *testing.T) {
	initializer := &qwenProviderInitializer{}

	withCompatible := initializer.DefaultCapabilities(true)
	require.Equal(t, qwenCompatibleResponsesPath, withCompatible[string(ApiNameResponses)])
	require.Equal(t, qwenCompatibleChatCompletionPath, withCompatible[string(ApiNameChatCompletion)])

	withoutCompatible := initializer.DefaultCapabilities(false)
	require.Equal(t, qwenCompatibleResponsesPath, withoutCompatible[string(ApiNameResponses)],
		"the default provider must still route the Responses API to the compatible endpoint")
	// Every other API must keep using the native protocol so that the existing
	// request/response conversion behaviour is preserved.
	require.Equal(t, qwenChatCompletionPath, withoutCompatible[string(ApiNameChatCompletion)])
	require.Equal(t, qwenTextEmbeddingPath, withoutCompatible[string(ApiNameEmbeddings)])
	require.Len(t, withoutCompatible, 7, "non-compatible capabilities should be the native APIs plus Responses")
	// File/Batches/Conversations remain compatible-mode only because the native
	// protocol has no equivalent for them.
	require.NotContains(t, withoutCompatible, string(ApiNameFiles))
	require.NotContains(t, withoutCompatible, string(ApiNameQwenV1Conversations))
}

// TestQwenNonCompatibleProviderSupportsResponses verifies that a provider built
// without compatible mode reports the Responses API as supported, so requests to
// /v1/responses are no longer rejected with an "unsupported API name" error.
func TestQwenNonCompatibleProviderSupportsResponses(t *testing.T) {
	cfg := ProviderConfig{qwenEnableCompatible: false}
	cfg.setDefaultCapabilities((&qwenProviderInitializer{}).DefaultCapabilities(false))

	require.True(t, cfg.isSupportedAPI(ApiNameResponses))
	require.Equal(t, qwenCompatibleResponsesPath, cfg.capabilities[string(ApiNameResponses)])

	qwen := &qwenProvider{config: cfg}
	body := []byte(`{"id":"resp-1","object":"response","output":[{"type":"message","content":[{"type":"output_text","text":"hi"}]}]}`)
	// The compatible Responses endpoint already speaks OpenAI, so the response
	// body has to be forwarded untouched instead of being parsed as a native
	// DashScope response (which would fail to unmarshal and return an error).
	out, err := qwen.TransformResponseBody(nil, ApiNameResponses, body)
	require.NoError(t, err)
	require.Equal(t, body, out)
}

// TestQwenGetApiNameRecognizesResponsesPath makes sure the compatible Responses
// path used as the capability target is recognised by the provider so that
// "protocol: original" deployments keep working end to end.
func TestQwenGetApiNameRecognizesResponsesPath(t *testing.T) {
	qwen := &qwenProvider{}
	require.Equal(t, ApiNameResponses, qwen.GetApiName(qwenCompatibleResponsesPath))
	// Older Bailian deployments still expose the legacy prefixed path.
	require.Equal(t, ApiNameResponses, qwen.GetApiName("/api/v2/apps/protocols/compatible-mode/v1/responses"))
}

// TestQwenCompatibleCapabilitiesStillExposeCompatibleOnlyAPIs is a regression
// guard for the generalised Responses capability: enabling compatible mode must
// keep exposing the APIs that only exist on the compatible endpoint (files,
// batches and conversations), which the native protocol cannot serve.
func TestQwenCompatibleCapabilitiesStillExposeCompatibleOnlyAPIs(t *testing.T) {
	capabilities := (&qwenProviderInitializer{}).DefaultCapabilities(true)

	require.Equal(t, qwenCompatibleChatCompletionPath, capabilities[string(ApiNameChatCompletion)])
	require.Equal(t, qwenCompatibleResponsesPath, capabilities[string(ApiNameResponses)])
	require.Equal(t, qwenCompatibleFilesPath, capabilities[string(ApiNameFiles)])
	require.Equal(t, qwenCompatibleBatchesPath, capabilities[string(ApiNameBatches)])
	require.Equal(t, qwenCompatibleConversationsPath, capabilities[string(ApiNameQwenV1Conversations)])

	// The compatible provider has to report all of them as supported so that the
	// router does not reject the requests.
	cfg := ProviderConfig{qwenEnableCompatible: true}
	cfg.setDefaultCapabilities(capabilities)
	require.True(t, cfg.isSupportedAPI(ApiNameResponses))
	require.True(t, cfg.isSupportedAPI(ApiNameFiles))
}
