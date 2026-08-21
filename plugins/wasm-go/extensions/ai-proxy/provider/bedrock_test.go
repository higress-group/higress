package provider

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildBedrockTextGenerationRequestSkipsEmptyMalformedMessageBeforeToolResult(t *testing.T) {
	provider := &bedrockProvider{}
	origRequest := &chatCompletionRequest{
		Messages: []chatMessage{
			{
				Role: roleUser,
				Content: []any{
					map[string]any{
						"type":      contentTypeImageUrl,
						"image_url": map[string]any{},
					},
				},
			},
			{
				Role:       roleTool,
				ToolCallId: "call_1",
				Content:    "tool result",
			},
		},
	}

	body, err := provider.buildBedrockTextGenerationRequest(origRequest, nil)
	require.NoError(t, err)

	var request bedrockTextGenRequest
	require.NoError(t, json.Unmarshal(body, &request))
	require.Len(t, request.Messages, 1)
	assert.Equal(t, roleUser, request.Messages[0].Role)
	require.Len(t, request.Messages[0].Content, 1)
	require.NotNil(t, request.Messages[0].Content[0].ToolResult)
	assert.Equal(t, "call_1", request.Messages[0].Content[0].ToolResult.ToolUseId)
}
