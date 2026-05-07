package provider

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBedrockResponsePreservesClaudeNativeThinkingSignature(t *testing.T) {
	provider := &bedrockProvider{}
	ctx := newMockMultipartHttpContext()
	ctx.SetContext("needClaudeResponseConversion", true)

	response := provider.buildChatCompletionResponse(ctx, &bedrockConverseResponse{
		Output: converseOutputMemberMessage{Message: message{
			Role: roleAssistant,
			Content: []contentBlock{
				{ReasoningContent: &reasoningContent{ReasoningText: &reasoningText{Text: "reasoning", Signature: "sig"}}},
				{Text: "answer"},
			},
		}},
		StopReason: "end_turn",
	})
	body, err := json.Marshal(response)
	require.NoError(t, err)

	converted, err := (&ClaudeToOpenAIConverter{}).ConvertOpenAIResponseToClaude(ctx, body)
	require.NoError(t, err)

	var claudeResponse claudeTextGenResponse
	require.NoError(t, json.Unmarshal(converted, &claudeResponse))
	require.Len(t, claudeResponse.Content, 2)
	assert.Equal(t, "thinking", claudeResponse.Content[0].Type)
	require.NotNil(t, claudeResponse.Content[0].Thinking)
	require.NotNil(t, claudeResponse.Content[0].Signature)
	assert.Equal(t, "reasoning", *claudeResponse.Content[0].Thinking)
	assert.Equal(t, "sig", *claudeResponse.Content[0].Signature)
	assert.Equal(t, "text", claudeResponse.Content[1].Type)
	require.NotNil(t, claudeResponse.Content[1].Text)
	assert.Equal(t, "answer", *claudeResponse.Content[1].Text)
}

func TestBedrockStreamPreservesClaudeNativeThinkingSignature(t *testing.T) {
	provider := &bedrockProvider{}
	ctx := newMockMultipartHttpContext()
	ctx.SetContext("needClaudeResponseConversion", true)
	converter := &ClaudeToOpenAIConverter{}

	textChunk, err := provider.convertEventFromBedrockToOpenAI(ctx, ConverseStreamEvent{
		ContentBlockIndex: 0,
		Delta: &converseStreamEventContentBlockDelta{
			ReasoningContent: &reasoningContentDelta{Text: "reasoning"},
		},
	})
	require.NoError(t, err)
	_, err = converter.ConvertOpenAIStreamResponseToClaude(ctx, textChunk)
	require.NoError(t, err)

	signatureChunk, err := provider.convertEventFromBedrockToOpenAI(ctx, ConverseStreamEvent{
		ContentBlockIndex: 0,
		Delta: &converseStreamEventContentBlockDelta{
			ReasoningContent: &reasoningContentDelta{Signature: "sig"},
		},
	})
	require.NoError(t, err)
	converted, err := converter.ConvertOpenAIStreamResponseToClaude(ctx, signatureChunk)
	require.NoError(t, err)

	events := parseClaudeSSEEvents(t, converted)
	require.Len(t, events, 1)
	assert.Equal(t, "content_block_delta", events[0].Name)
	require.NotNil(t, events[0].Payload.Delta)
	assert.Equal(t, "signature_delta", events[0].Payload.Delta.Type)
	assert.Equal(t, "sig", events[0].Payload.Delta.Signature)
}

func TestBedrockStreamPreservesClaudeNativeIndexesAndStops(t *testing.T) {
	provider := &bedrockProvider{}
	ctx := newMockMultipartHttpContext()
	ctx.SetContext("needClaudeResponseConversion", true)
	converter := &ClaudeToOpenAIConverter{}

	chunk, err := provider.convertEventFromBedrockToOpenAI(ctx, ConverseStreamEvent{
		ContentBlockIndex: 2,
		Delta: &converseStreamEventContentBlockDelta{
			ReasoningContent: &reasoningContentDelta{Text: "reasoning"},
		},
	})
	require.NoError(t, err)
	converted, err := converter.ConvertOpenAIStreamResponseToClaude(ctx, chunk)
	require.NoError(t, err)
	events := parseClaudeSSEEvents(t, converted)
	require.Len(t, events, 2)
	assert.Equal(t, "content_block_start", events[0].Name)
	require.NotNil(t, events[0].Payload.Index)
	assert.Equal(t, 2, *events[0].Payload.Index)
	assert.Equal(t, "content_block_delta", events[1].Name)
	require.NotNil(t, events[1].Payload.Index)
	assert.Equal(t, 2, *events[1].Payload.Index)

	chunk, err = provider.convertEventFromBedrockToOpenAI(ctx, ConverseStreamEvent{
		ContentBlockIndex: 2,
		ContentBlockStop:  &contentBlockStop{ContentBlockIndex: 2},
	})
	require.NoError(t, err)
	converted, err = converter.ConvertOpenAIStreamResponseToClaude(ctx, chunk)
	require.NoError(t, err)
	events = parseClaudeSSEEvents(t, converted)
	require.Len(t, events, 1)
	assert.Equal(t, "content_block_stop", events[0].Name)
	require.NotNil(t, events[0].Payload.Index)
	assert.Equal(t, 2, *events[0].Payload.Index)
}

func TestBedrockResponsePreservesClaudeNativeRedactedThinking(t *testing.T) {
	provider := &bedrockProvider{}
	ctx := newMockMultipartHttpContext()
	ctx.SetContext("needClaudeResponseConversion", true)

	response := provider.buildChatCompletionResponse(ctx, &bedrockConverseResponse{
		Output: converseOutputMemberMessage{Message: message{
			Role: roleAssistant,
			Content: []contentBlock{
				{ReasoningContent: &reasoningContent{RedactedContent: "opaque-base64"}},
				{Text: "answer"},
			},
		}},
		StopReason: "end_turn",
	})
	body, err := json.Marshal(response)
	require.NoError(t, err)

	converted, err := (&ClaudeToOpenAIConverter{}).ConvertOpenAIResponseToClaude(ctx, body)
	require.NoError(t, err)

	var claudeResponse claudeTextGenResponse
	require.NoError(t, json.Unmarshal(converted, &claudeResponse))
	require.Len(t, claudeResponse.Content, 2)
	assert.Equal(t, "redacted_thinking", claudeResponse.Content[0].Type)
	assert.Equal(t, "opaque-base64", claudeResponse.Content[0].Data)
}

func TestBedrockResponsePreservesClaudeNativeToolUseWithThinking(t *testing.T) {
	provider := &bedrockProvider{}
	ctx := newMockMultipartHttpContext()
	ctx.SetContext("needClaudeResponseConversion", true)

	response := provider.buildChatCompletionResponse(ctx, &bedrockConverseResponse{
		Output: converseOutputMemberMessage{Message: message{
			Role: roleAssistant,
			Content: []contentBlock{
				{ReasoningContent: &reasoningContent{ReasoningText: &reasoningText{Text: "reasoning", Signature: "sig"}}},
				{ToolUse: &bedrockToolUse{ToolUseId: "toolu_1", Name: "lookup", Input: map[string]interface{}{"query": "q"}}},
			},
		}},
		StopReason: "tool_use",
	})
	body, err := json.Marshal(response)
	require.NoError(t, err)

	converted, err := (&ClaudeToOpenAIConverter{}).ConvertOpenAIResponseToClaude(ctx, body)
	require.NoError(t, err)

	var claudeResponse claudeTextGenResponse
	require.NoError(t, json.Unmarshal(converted, &claudeResponse))
	require.Len(t, claudeResponse.Content, 2)
	assert.Equal(t, "thinking", claudeResponse.Content[0].Type)
	assert.Equal(t, "tool_use", claudeResponse.Content[1].Type)
	assert.Equal(t, "toolu_1", claudeResponse.Content[1].Id)
	assert.Equal(t, "lookup", claudeResponse.Content[1].Name)
}

func TestBedrockStreamRedactedThinkingStopsOnce(t *testing.T) {
	provider := &bedrockProvider{}
	ctx := newMockMultipartHttpContext()
	ctx.SetContext("needClaudeResponseConversion", true)
	converter := &ClaudeToOpenAIConverter{}

	chunk, err := provider.convertEventFromBedrockToOpenAI(ctx, ConverseStreamEvent{
		ContentBlockIndex: 1,
		Delta: &converseStreamEventContentBlockDelta{
			ReasoningContent: &reasoningContentDelta{RedactedContent: "opaque-base64"},
		},
	})
	require.NoError(t, err)
	converted, err := converter.ConvertOpenAIStreamResponseToClaude(ctx, chunk)
	require.NoError(t, err)
	events := parseClaudeSSEEvents(t, converted)
	require.Len(t, events, 1)
	assert.Equal(t, "content_block_start", events[0].Name)
	assert.Equal(t, "redacted_thinking", events[0].Payload.ContentBlock.Type)

	chunk, err = provider.convertEventFromBedrockToOpenAI(ctx, ConverseStreamEvent{
		ContentBlockIndex: 1,
		ContentBlockStop:  &contentBlockStop{ContentBlockIndex: 1},
	})
	require.NoError(t, err)
	converted, err = converter.ConvertOpenAIStreamResponseToClaude(ctx, chunk)
	require.NoError(t, err)
	events = parseClaudeSSEEvents(t, converted)
	require.Len(t, events, 1)
	assert.Equal(t, "content_block_stop", events[0].Name)
}

func TestBedrockRequestPreservesClaudeNativeThinkingAndToolResult(t *testing.T) {
	provider := &bedrockProvider{}
	openaiBody, err := (&ClaudeToOpenAIConverter{}).ConvertClaudeRequestToOpenAI([]byte(`{
		"model":"claude",
		"system":"system prompt",
		"messages":[{
			"role":"assistant",
			"content":[
				{"type":"thinking","thinking":"reasoning","signature":"sig"},
				{"type":"tool_use","id":"toolu_1","name":"lookup","input":{"query":"q"}}
			]
		},{
			"role":"user",
			"content":[{
				"type":"tool_result",
				"tool_use_id":"toolu_1",
				"is_error":true,
				"content":[{"type":"text","text":"failed"}]
			}]
		}]
	}`))
	require.NoError(t, err)
	var openaiRequest chatCompletionRequest
	require.NoError(t, json.Unmarshal(openaiBody, &openaiRequest))

	body, err := provider.buildBedrockTextGenerationRequest(&openaiRequest, nil)
	require.NoError(t, err)

	var request bedrockTextGenRequest
	require.NoError(t, json.Unmarshal(body, &request))
	require.Len(t, request.System, 1)
	assert.Equal(t, "system prompt", request.System[0].Text)
	require.Len(t, request.Messages, 2)
	require.Len(t, request.Messages[0].Content, 2)
	require.NotNil(t, request.Messages[0].Content[0].ReasoningContent)
	require.NotNil(t, request.Messages[0].Content[0].ReasoningContent.ReasoningText)
	assert.Equal(t, "reasoning", request.Messages[0].Content[0].ReasoningContent.ReasoningText.Text)
	assert.Equal(t, "sig", request.Messages[0].Content[0].ReasoningContent.ReasoningText.Signature)
	require.NotNil(t, request.Messages[0].Content[1].ToolUse)
	assert.Equal(t, "toolu_1", request.Messages[0].Content[1].ToolUse.ToolUseId)
	require.NotNil(t, request.Messages[1].Content[0].ToolResult)
	assert.Equal(t, "error", request.Messages[1].Content[0].ToolResult.Status)
	require.NotNil(t, request.Messages[1].Content[0].ToolResult.Content[0].Text)
	assert.Equal(t, "failed", *request.Messages[1].Content[0].ToolResult.Content[0].Text)
}

func TestBedrockRequestToolResultWithTrailingTextDoesNotDuplicateToolResult(t *testing.T) {
	provider := &bedrockProvider{}
	openaiBody, err := (&ClaudeToOpenAIConverter{}).ConvertClaudeRequestToOpenAI([]byte(`{
		"model":"claude",
		"messages":[{
			"role":"user",
			"content":[
				{"type":"tool_result","tool_use_id":"toolu_1","content":"ok"},
				{"type":"text","text":"continue"}
			]
		}]
	}`))
	require.NoError(t, err)
	var openaiRequest chatCompletionRequest
	require.NoError(t, json.Unmarshal(openaiBody, &openaiRequest))
	require.Len(t, openaiRequest.Messages, 2)
	assert.Empty(t, openaiRequest.Messages[1].ClaudeContentBlocks)

	body, err := provider.buildBedrockTextGenerationRequest(&openaiRequest, nil)
	require.NoError(t, err)

	var request bedrockTextGenRequest
	require.NoError(t, json.Unmarshal(body, &request))
	require.Len(t, request.Messages, 2)
	require.Len(t, request.Messages[0].Content, 1)
	require.NotNil(t, request.Messages[0].Content[0].ToolResult)
	require.Len(t, request.Messages[1].Content, 1)
	assert.Nil(t, request.Messages[1].Content[0].ToolResult)
	assert.Equal(t, "continue", request.Messages[1].Content[0].Text)
}

func TestBedrockRequestRedactedThinkingUsesSingleUnionArm(t *testing.T) {
	contents := claudeContentBlocksToBedrockContents([]claudeChatMessageContent{
		{Type: "redacted_thinking", Data: "opaque-base64"},
	})

	body, err := json.Marshal(contents[0])
	require.NoError(t, err)
	assert.JSONEq(t, `{"reasoningContent":{"redactedContent":"opaque-base64"}}`, string(body))
	assert.NotContains(t, string(body), "reasoningText")
}

func TestBedrockRequestToolResultDefaultsEmptyContent(t *testing.T) {
	result := claudeToolResultBlockToBedrock(claudeChatMessageContent{
		Type:      "tool_result",
		ToolUseId: "toolu_1",
	})

	require.Len(t, result.Content, 1)
	require.NotNil(t, result.Content[0].Text)
	assert.Equal(t, "", *result.Content[0].Text)
	body, err := json.Marshal(result)
	require.NoError(t, err)
	assert.Contains(t, string(body), `"text":""`)
	assert.NotContains(t, string(body), `[{}]`)
}

func TestBedrockRequestPreservesEmptyThinkingTextWithSignature(t *testing.T) {
	contents := claudeContentBlocksToBedrockContents([]claudeChatMessageContent{
		{Type: "thinking", Thinking: "", Signature: "sig"},
	})

	body, err := json.Marshal(contents[0])
	require.NoError(t, err)
	assert.JSONEq(t, `{"reasoningContent":{"reasoningText":{"text":"","signature":"sig"}}}`, string(body))
}

func TestBedrockRequestPreservesClaudeNativeThinkingBudget(t *testing.T) {
	provider := &bedrockProvider{}
	openaiBody, err := (&ClaudeToOpenAIConverter{}).ConvertClaudeRequestToOpenAI([]byte(`{
		"model":"claude",
		"max_tokens":32000,
		"thinking":{"type":"enabled","budget_tokens":8192},
		"messages":[{"role":"user","content":"hello"}]
	}`))
	require.NoError(t, err)
	var openaiRequest chatCompletionRequest
	require.NoError(t, json.Unmarshal(openaiBody, &openaiRequest))

	body, err := provider.buildBedrockTextGenerationRequest(&openaiRequest, nil)
	require.NoError(t, err)

	var request bedrockTextGenRequest
	require.NoError(t, json.Unmarshal(body, &request))
	assert.Equal(t, float64(8192), request.AdditionalModelRequestFields["thinking"].(map[string]interface{})["budget_tokens"])
}

func TestBedrockRequestMapsAdaptiveOutputEffortIntoThinking(t *testing.T) {
	provider := &bedrockProvider{}
	openaiBody, err := (&ClaudeToOpenAIConverter{}).ConvertClaudeRequestToOpenAI([]byte(`{
		"model":"claude",
		"thinking":{"type":"adaptive"},
		"output_config":{"effort":"high"},
		"messages":[{"role":"user","content":"hello"}]
	}`))
	require.NoError(t, err)
	var openaiRequest chatCompletionRequest
	require.NoError(t, json.Unmarshal(openaiBody, &openaiRequest))

	body, err := provider.buildBedrockTextGenerationRequest(&openaiRequest, nil)
	require.NoError(t, err)

	var request bedrockTextGenRequest
	require.NoError(t, json.Unmarshal(body, &request))
	thinking := request.AdditionalModelRequestFields["thinking"].(map[string]interface{})
	assert.Equal(t, "adaptive", thinking["type"])
	assert.Equal(t, "high", thinking["effort"])
	assert.NotContains(t, request.AdditionalModelRequestFields, "output_config")
	assert.NotContains(t, request.AdditionalModelRequestFields, "anthropic_beta")
}

func TestBedrockRequestDropsOutputEffortWithoutAdaptiveThinking(t *testing.T) {
	provider := &bedrockProvider{}
	openaiBody, err := (&ClaudeToOpenAIConverter{}).ConvertClaudeRequestToOpenAI([]byte(`{
		"model":"claude",
		"output_config":{"effort":"high"},
		"messages":[{"role":"user","content":"hello"}]
	}`))
	require.NoError(t, err)
	var openaiRequest chatCompletionRequest
	require.NoError(t, json.Unmarshal(openaiBody, &openaiRequest))

	body, err := provider.buildBedrockTextGenerationRequest(&openaiRequest, nil)
	require.NoError(t, err)

	var request bedrockTextGenRequest
	require.NoError(t, json.Unmarshal(body, &request))
	assert.NotContains(t, request.AdditionalModelRequestFields, "output_config")
	assert.NotContains(t, request.AdditionalModelRequestFields, "thinking")
}

func TestBedrockRequestDropsUnsupportedAdaptiveOutputEffort(t *testing.T) {
	provider := &bedrockProvider{}
	openaiBody, err := (&ClaudeToOpenAIConverter{}).ConvertClaudeRequestToOpenAI([]byte(`{
		"model":"claude",
		"thinking":{"type":"adaptive"},
		"output_config":{"effort":"xhigh"},
		"messages":[{"role":"user","content":"hello"}]
	}`))
	require.NoError(t, err)
	var openaiRequest chatCompletionRequest
	require.NoError(t, json.Unmarshal(openaiBody, &openaiRequest))

	body, err := provider.buildBedrockTextGenerationRequest(&openaiRequest, nil)
	require.NoError(t, err)

	var request bedrockTextGenRequest
	require.NoError(t, json.Unmarshal(body, &request))
	thinking := request.AdditionalModelRequestFields["thinking"].(map[string]interface{})
	assert.Equal(t, "adaptive", thinking["type"])
	assert.NotContains(t, thinking, "effort")
	assert.NotContains(t, request.AdditionalModelRequestFields, "output_config")
}

func TestBedrockRequestMapsClaudeOutputFormatToTextFormat(t *testing.T) {
	provider := &bedrockProvider{}
	openaiBody, err := (&ClaudeToOpenAIConverter{}).ConvertClaudeRequestToOpenAI([]byte(`{
		"model":"claude",
		"output_config":{
			"format":{
				"type":"json_schema",
				"schema":{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"]}
			}
		},
		"messages":[{"role":"user","content":"hello"}]
	}`))
	require.NoError(t, err)
	var openaiRequest chatCompletionRequest
	require.NoError(t, json.Unmarshal(openaiBody, &openaiRequest))

	body, err := provider.buildBedrockTextGenerationRequest(&openaiRequest, nil)
	require.NoError(t, err)

	var request map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &request))
	outputConfig := request["outputConfig"].(map[string]interface{})
	textFormat := outputConfig["textFormat"].(map[string]interface{})
	assert.Equal(t, "json_schema", textFormat["type"])
}

func TestBedrockRequestDowngradesForcedToolChoiceWhenThinkingEnabled(t *testing.T) {
	provider := &bedrockProvider{}
	openaiBody, err := (&ClaudeToOpenAIConverter{}).ConvertClaudeRequestToOpenAI([]byte(`{
		"model":"claude",
		"thinking":{"type":"enabled","budget_tokens":8192},
		"tools":[{"name":"lookup","input_schema":{"type":"object"}}],
		"tool_choice":{"type":"any"},
		"messages":[{"role":"user","content":"hello"}]
	}`))
	require.NoError(t, err)
	var openaiRequest chatCompletionRequest
	require.NoError(t, json.Unmarshal(openaiBody, &openaiRequest))

	body, err := provider.buildBedrockTextGenerationRequest(&openaiRequest, nil)
	require.NoError(t, err)

	var request bedrockTextGenRequest
	require.NoError(t, json.Unmarshal(body, &request))
	require.NotNil(t, request.ToolConfig)
	assert.NotNil(t, request.ToolConfig.ToolChoice.Auto)
	assert.Nil(t, request.ToolConfig.ToolChoice.Any)
	assert.Nil(t, request.ToolConfig.ToolChoice.Tool)
}

func TestBedrockStreamSkipsOrphanClaudeContentBlockStop(t *testing.T) {
	provider := &bedrockProvider{}
	ctx := newMockMultipartHttpContext()
	ctx.SetContext("needClaudeResponseConversion", true)
	converter := &ClaudeToOpenAIConverter{}

	chunk, err := provider.convertEventFromBedrockToOpenAI(ctx, ConverseStreamEvent{
		ContentBlockIndex: 3,
		ContentBlockStop:  &contentBlockStop{ContentBlockIndex: 3},
	})
	require.NoError(t, err)
	converted, err := converter.ConvertOpenAIStreamResponseToClaude(ctx, chunk)
	require.NoError(t, err)
	assert.Empty(t, parseClaudeSSEEvents(t, converted))
}

func TestBedrockStreamBatchedEventsKeepClaudeMessageStartFirst(t *testing.T) {
	provider := &bedrockProvider{}
	ctx := newMockMultipartHttpContext()
	ctx.SetContext("needClaudeResponseConversion", true)
	converter := &ClaudeToOpenAIConverter{}
	role := roleAssistant

	roleChunk, err := provider.convertEventFromBedrockToOpenAI(ctx, ConverseStreamEvent{Role: &role})
	require.NoError(t, err)
	reasoningChunk, err := provider.convertEventFromBedrockToOpenAI(ctx, ConverseStreamEvent{
		ContentBlockIndex: 0,
		Delta: &converseStreamEventContentBlockDelta{
			ReasoningContent: &reasoningContentDelta{Text: "reasoning"},
		},
	})
	require.NoError(t, err)

	converted, err := converter.ConvertOpenAIStreamResponseToClaude(ctx, append(roleChunk, reasoningChunk...))
	require.NoError(t, err)
	events := parseClaudeSSEEvents(t, converted)
	require.Len(t, events, 3)
	assert.Equal(t, "message_start", events[0].Name)
	assert.Equal(t, "content_block_start", events[1].Name)
	assert.Equal(t, "content_block_delta", events[2].Name)
}

func TestBedrockResponseUsesReasoningContentInsteadOfThinkTags(t *testing.T) {
	provider := &bedrockProvider{}

	response := provider.buildChatCompletionResponse(newMockMultipartHttpContext(), &bedrockConverseResponse{
		Output: converseOutputMemberMessage{Message: message{
			Role: roleAssistant,
			Content: []contentBlock{
				{ReasoningContent: &reasoningContent{ReasoningText: &reasoningText{Text: "reasoning"}}},
				{Text: "answer"},
			},
		}},
		StopReason: "end_turn",
	})

	require.Len(t, response.Choices, 1)
	require.NotNil(t, response.Choices[0].Message)
	assert.Equal(t, "reasoning", response.Choices[0].Message.ReasoningContent)
	assert.Equal(t, "answer", response.Choices[0].Message.Content)
	assert.NotContains(t, response.Choices[0].Message.StringContent(), "<think>")
}

func TestBedrockStreamUsesReasoningContentInsteadOfThinkTags(t *testing.T) {
	provider := &bedrockProvider{}
	ctx := newMockMultipartHttpContext()

	chunk, err := provider.convertEventFromBedrockToOpenAI(ctx, ConverseStreamEvent{
		ContentBlockIndex: 0,
		Delta: &converseStreamEventContentBlockDelta{
			ReasoningContent: &reasoningContentDelta{Text: "reasoning"},
		},
	})
	require.NoError(t, err)

	body := strings.TrimPrefix(strings.TrimSpace(string(chunk)), ssePrefix)
	var response chatCompletionResponse
	require.NoError(t, json.Unmarshal([]byte(body), &response))
	require.Len(t, response.Choices, 1)
	require.NotNil(t, response.Choices[0].Delta)
	assert.Equal(t, "reasoning", response.Choices[0].Delta.ReasoningContent)
	assert.Nil(t, response.Choices[0].Delta.Content)
}
