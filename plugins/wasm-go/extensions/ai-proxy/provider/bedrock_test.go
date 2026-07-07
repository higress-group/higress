package provider

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"net/http"
	"testing"

	"github.com/higress-group/wasm-go/pkg/iface"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-proxy/util"
)

// mockBedrockHttpContext is a minimal mock for wrapper.HttpContext used in bedrock tests.
type mockBedrockHttpContext struct {
	contextMap map[string]interface{}
}

func newMockBedrockHttpContext() *mockBedrockHttpContext {
	return &mockBedrockHttpContext{contextMap: make(map[string]interface{})}
}

func (m *mockBedrockHttpContext) SetContext(key string, value interface{})          { m.contextMap[key] = value }
func (m *mockBedrockHttpContext) GetContext(key string) interface{}                 { return m.contextMap[key] }
func (m *mockBedrockHttpContext) GetBoolContext(key string, def bool) bool          { return def }
func (m *mockBedrockHttpContext) GetStringContext(key, def string) string           { return def }
func (m *mockBedrockHttpContext) GetByteSliceContext(key string, def []byte) []byte { return def }
func (m *mockBedrockHttpContext) Scheme() string                                    { return "" }
func (m *mockBedrockHttpContext) Host() string                                      { return "" }
func (m *mockBedrockHttpContext) Path() string                                      { return "" }
func (m *mockBedrockHttpContext) Method() string                                    { return "" }
func (m *mockBedrockHttpContext) GetUserAttribute(key string) interface{}           { return nil }
func (m *mockBedrockHttpContext) SetUserAttribute(key string, value interface{})    {}
func (m *mockBedrockHttpContext) SetUserAttributeMap(kvmap map[string]interface{})  {}
func (m *mockBedrockHttpContext) GetUserAttributeMap() map[string]interface{}       { return nil }
func (m *mockBedrockHttpContext) WriteUserAttributeToLog() error                    { return nil }
func (m *mockBedrockHttpContext) WriteUserAttributeToLogWithKey(key string) error   { return nil }
func (m *mockBedrockHttpContext) WriteUserAttributeToTrace() error                  { return nil }
func (m *mockBedrockHttpContext) DontReadRequestBody()                              {}
func (m *mockBedrockHttpContext) DontReadResponseBody()                             {}
func (m *mockBedrockHttpContext) BufferRequestBody()                                {}
func (m *mockBedrockHttpContext) BufferResponseBody()                               {}
func (m *mockBedrockHttpContext) NeedPauseStreamingResponse()                       {}
func (m *mockBedrockHttpContext) PushBuffer(buffer []byte)                          {}
func (m *mockBedrockHttpContext) PopBuffer() []byte                                 { return nil }
func (m *mockBedrockHttpContext) BufferQueueSize() int                              { return 0 }
func (m *mockBedrockHttpContext) DisableReroute()                                   {}
func (m *mockBedrockHttpContext) SetRequestBodyBufferLimit(byteSize uint32)         {}
func (m *mockBedrockHttpContext) SetResponseBodyBufferLimit(byteSize uint32)        {}
func (m *mockBedrockHttpContext) RouteCall(method, url string, headers [][2]string, body []byte, callback iface.RouteResponseCallback) error {
	return nil
}
func (m *mockBedrockHttpContext) GetExecutionPhase() iface.HTTPExecutionPhase { return 0 }
func (m *mockBedrockHttpContext) HasRequestBody() bool                        { return false }
func (m *mockBedrockHttpContext) HasResponseBody() bool                       { return false }
func (m *mockBedrockHttpContext) IsWebsocket() bool                           { return false }
func (m *mockBedrockHttpContext) IsBinaryRequestBody() bool                   { return false }
func (m *mockBedrockHttpContext) IsBinaryResponseBody() bool                  { return false }

// ==================== ValidateConfig Tests ====================

func TestBedrockValidateConfig_MissingBothAuth(t *testing.T) {
	init := &bedrockProviderInitializer{}
	config := &ProviderConfig{awsRegion: "us-east-1"}
	err := init.ValidateConfig(config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing bedrock access authentication parameters")
}

func TestBedrockValidateConfig_MissingRegion(t *testing.T) {
	init := &bedrockProviderInitializer{}
	config := &ProviderConfig{awsAccessKey: "ak", awsSecretKey: "sk"}
	err := init.ValidateConfig(config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing bedrock region")
}

func TestBedrockValidateConfig_ValidWithAkSk(t *testing.T) {
	init := &bedrockProviderInitializer{}
	config := &ProviderConfig{awsAccessKey: "ak", awsSecretKey: "sk", awsRegion: "us-east-1"}
	err := init.ValidateConfig(config)
	assert.NoError(t, err)
}

func TestBedrockValidateConfig_ValidWithApiTokens(t *testing.T) {
	init := &bedrockProviderInitializer{}
	config := &ProviderConfig{apiTokens: []string{"token1"}, awsRegion: "us-east-1"}
	err := init.ValidateConfig(config)
	assert.NoError(t, err)
}

// ==================== DefaultCapabilities & CreateProvider Tests ====================

func TestBedrockDefaultCapabilities(t *testing.T) {
	caps := (&bedrockProviderInitializer{}).DefaultCapabilities()
	assert.Equal(t, bedrockChatCompletionPath, caps[string(ApiNameChatCompletion)])
	assert.Equal(t, bedrockMantleMessagesPath, caps[string(ApiNameAnthropicMessages)])
	assert.Equal(t, bedrockInvokeModelPath, caps[string(ApiNameImageGeneration)])
}

func TestBedrockCreateProvider(t *testing.T) {
	init := &bedrockProviderInitializer{}
	config := ProviderConfig{awsAccessKey: "ak", awsSecretKey: "sk", awsRegion: "us-east-1"}
	provider, err := init.CreateProvider(config)
	require.NoError(t, err)
	require.NotNil(t, provider)
	bp, ok := provider.(*bedrockProvider)
	require.True(t, ok)
	assert.Equal(t, "us-east-1", bp.config.awsRegion)
}

// ==================== GetApiName Tests ====================

func TestBedrockGetApiName(t *testing.T) {
	p := &bedrockProvider{}
	tests := []struct {
		name     string
		path     string
		expected ApiName
	}{
		{name: "converse", path: "/model/anthropic.claude-3-sonnet/converse", expected: ApiNameChatCompletion},
		{name: "converse-stream", path: "/model/anthropic.claude-3-sonnet/converse-stream", expected: ApiNameChatCompletion},
		{name: "invoke", path: "/model/stability.stable-diffusion-xl/invoke", expected: ApiNameImageGeneration},
		{name: "invoke-stream", path: "/model/some-model/invoke-with-response-stream", expected: ApiNameImageGeneration},
		{name: "unknown", path: "/unknown/path", expected: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { assert.Equal(t, tt.expected, p.GetApiName(tt.path)) })
	}
}

// ==================== stopReasonBedrock2OpenAI Tests ====================

func TestStopReasonBedrock2OpenAI(t *testing.T) {
	tests := []struct {
		name     string
		reason   string
		expected string
	}{
		{name: "end_turn", reason: "end_turn", expected: finishReasonStop},
		{name: "stop_sequence", reason: "stop_sequence", expected: finishReasonStop},
		{name: "max_tokens", reason: "max_tokens", expected: finishReasonLength},
		{name: "tool_use", reason: "tool_use", expected: finishReasonToolCall},
		{name: "unknown", reason: "unknown_reason", expected: "unknown_reason"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { assert.Equal(t, tt.expected, stopReasonBedrock2OpenAI(tt.reason)) })
	}
}

// ==================== mapPromptCacheRetentionToBedrockTTL Tests ====================

func TestMapPromptCacheRetentionToBedrockTTL(t *testing.T) {
	tests := []struct {
		name        string
		retention   string
		expectedTTL string
		expectedOk  bool
	}{
		{name: "empty", retention: "", expectedTTL: "", expectedOk: false},
		{name: "in_memory", retention: "in_memory", expectedTTL: "", expectedOk: true},
		{name: "24h", retention: "24h", expectedTTL: bedrockCacheTTL1h, expectedOk: true},
		{name: "unsupported", retention: "1h", expectedTTL: "", expectedOk: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ttl, ok := mapPromptCacheRetentionToBedrockTTL(tt.retention)
			assert.Equal(t, tt.expectedTTL, ttl)
			assert.Equal(t, tt.expectedOk, ok)
		})
	}
}

// ==================== resolvePromptCacheRetention Tests ====================

func TestResolvePromptCacheRetention(t *testing.T) {
	p := &bedrockProvider{config: ProviderConfig{promptCacheRetention: "24h"}}
	t.Run("request level priority", func(t *testing.T) { assert.Equal(t, "in_memory", p.resolvePromptCacheRetention("in_memory")) })
	t.Run("config fallback", func(t *testing.T) { assert.Equal(t, "24h", p.resolvePromptCacheRetention("")) })
	t.Run("both empty", func(t *testing.T) {
		p2 := &bedrockProvider{config: ProviderConfig{}}
		assert.Equal(t, "", p2.resolvePromptCacheRetention(""))
	})
}

// ==================== getPromptCachePointPositions Tests ====================

func TestGetPromptCachePointPositions_Default(t *testing.T) {
	p := &bedrockProvider{config: ProviderConfig{}}
	pos := p.getPromptCachePointPositions()
	assert.True(t, pos[bedrockCachePointPositionSystemPrompt])
	assert.False(t, pos[bedrockCachePointPositionLastMessage])
	_, hasLastUser := pos[bedrockCachePointPositionLastUserMessage]
	assert.False(t, hasLastUser)
}

func TestGetPromptCachePointPositions_Custom(t *testing.T) {
	p := &bedrockProvider{config: ProviderConfig{bedrockPromptCachePointPositions: map[string]bool{
		"systemPrompt": false, "lastUserMessage": true, "lastMessage": true,
	}}}
	pos := p.getPromptCachePointPositions()
	assert.False(t, pos[bedrockCachePointPositionSystemPrompt])
	assert.True(t, pos[bedrockCachePointPositionLastUserMessage])
	assert.True(t, pos[bedrockCachePointPositionLastMessage])
}

func TestGetPromptCachePointPositions_UnsupportedKey(t *testing.T) {
	p := &bedrockProvider{config: ProviderConfig{bedrockPromptCachePointPositions: map[string]bool{
		"invalidKey": true, "systemPrompt": true,
	}}}
	pos := p.getPromptCachePointPositions()
	assert.True(t, pos[bedrockCachePointPositionSystemPrompt])
	assert.False(t, pos[bedrockCachePointPositionLastUserMessage])
	assert.False(t, pos[bedrockCachePointPositionLastMessage])
}

// ==================== buildPromptTokensDetails Tests ====================

func TestBuildPromptTokensDetails(t *testing.T) {
	t.Run("no cached tokens", func(t *testing.T) { assert.Nil(t, buildPromptTokensDetails(0, 0)) })
	t.Run("only read tokens", func(t *testing.T) {
		r := buildPromptTokensDetails(100, 0); require.NotNil(t, r); assert.Equal(t, 100, r.CachedTokens)
	})
	t.Run("only write tokens", func(t *testing.T) {
		r := buildPromptTokensDetails(0, 50); require.NotNil(t, r); assert.Equal(t, 50, r.CachedTokens)
	})
	t.Run("both tokens", func(t *testing.T) {
		r := buildPromptTokensDetails(100, 50); require.NotNil(t, r); assert.Equal(t, 150, r.CachedTokens)
	})
}

// ==================== bedrockThinkingFromClaudeConfig Tests ====================

func TestBedrockThinkingFromClaudeConfig(t *testing.T) {
	tests := []struct {
		name     string
		thinking *claudeThinkingConfig
		expected map[string]interface{}
	}{
		{name: "nil", thinking: nil, expected: nil},
		{name: "empty type", thinking: &claudeThinkingConfig{Type: ""}, expected: nil},
		{name: "disabled", thinking: &claudeThinkingConfig{Type: "disabled"}, expected: nil},
		{name: "enabled with budget", thinking: &claudeThinkingConfig{Type: "enabled", BudgetTokens: 10000}, expected: map[string]interface{}{"type": "enabled", "budget_tokens": 10000}},
		{name: "enabled no budget", thinking: &claudeThinkingConfig{Type: "enabled"}, expected: map[string]interface{}{"type": "enabled"}},
		{name: "enabled with display", thinking: &claudeThinkingConfig{Type: "enabled", Display: "streaming"}, expected: map[string]interface{}{"type": "enabled", "display": "streaming"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { assert.Equal(t, tt.expected, bedrockThinkingFromClaudeConfig(tt.thinking)) })
	}
}

// ==================== bedrockSupportsAdaptiveEffort Tests ====================

func TestBedrockSupportsAdaptiveEffort(t *testing.T) {
	tests := []struct {
		name     string
		effort   string
		expected bool
	}{
		{name: "low", effort: "low", expected: true},
		{name: "medium", effort: "medium", expected: true},
		{name: "high", effort: "high", expected: true},
		{name: "invalid", effort: "invalid", expected: false},
		{name: "empty", effort: "", expected: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { assert.Equal(t, tt.expected, bedrockSupportsAdaptiveEffort(tt.effort)) })
	}
}

// ==================== chatMessage2BedrockMessage Tests ====================

func TestChatMessage2BedrockMessage_StringContent(t *testing.T) {
	msg := chatMessage{Role: roleUser, Content: "Hello, how are you?"}
	result := chatMessage2BedrockMessage(msg)
	assert.Equal(t, roleUser, result.Role)
	require.Len(t, result.Content, 1)
	assert.Equal(t, "Hello, how are you?", result.Content[0].Text)
}

func TestChatMessage2BedrockMessage_ToolCalls(t *testing.T) {
	msg := chatMessage{
		Role: roleAssistant,
		ToolCalls: []toolCall{{
			Id: "call_123", Type: "function",
			Function: functionCall{Name: "get_weather", Arguments: `{"city": "Beijing"}`},
		}},
	}
	result := chatMessage2BedrockMessage(msg)
	assert.Equal(t, roleAssistant, result.Role)
	require.Len(t, result.Content, 1)
	require.NotNil(t, result.Content[0].ToolUse)
	assert.Equal(t, "call_123", result.Content[0].ToolUse.ToolUseId)
	assert.Equal(t, "get_weather", result.Content[0].ToolUse.Name)
	assert.Equal(t, "Beijing", result.Content[0].ToolUse.Input["city"])
}

func TestChatMessage2BedrockMessage_ImageContent(t *testing.T) {
	msg := chatMessage{
		Role: roleUser,
		Content: []any{map[string]any{
			"type": "image_url",
			"image_url": map[string]any{"url": "data:image/png;base64,iVBORw0KGgo="},
		}},
	}
	result := chatMessage2BedrockMessage(msg)
	assert.Equal(t, roleUser, result.Role)
	require.Len(t, result.Content, 1)
	require.NotNil(t, result.Content[0].Image)
	assert.Equal(t, "png", result.Content[0].Image.Format)
	assert.Equal(t, "iVBORw0KGgo=", result.Content[0].Image.Source.Bytes)
}

func TestChatMessage2BedrockMessage_TextAndImageContent(t *testing.T) {
	msg := chatMessage{
		Role: roleUser,
		Content: []any{
			map[string]any{"type": "text", "text": "What is in this image?"},
			map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/jpeg;base64,/9j/4AAQ"}},
		},
	}
	result := chatMessage2BedrockMessage(msg)
	assert.Equal(t, roleUser, result.Role)
	require.Len(t, result.Content, 2)
	assert.Equal(t, "What is in this image?", result.Content[0].Text)
	require.NotNil(t, result.Content[1].Image)
	assert.Equal(t, "jpeg", result.Content[1].Image.Format)
}

func TestChatMessage2BedrockMessage_ClaudeContentBlocks(t *testing.T) {
	msg := chatMessage{
		Role: roleUser,
		ClaudeContentBlocks: []claudeChatMessageContent{
			{Type: "text", Text: "Hello"},
			{Type: "image", Source: &claudeChatMessageContentSource{Type: "base64", MediaType: "image/png", Data: "iVBORw0KGgo="}},
		},
	}
	result := chatMessage2BedrockMessage(msg)
	assert.Equal(t, roleUser, result.Role)
	require.Len(t, result.Content, 2)
	assert.Equal(t, "Hello", result.Content[0].Text)
	require.NotNil(t, result.Content[1].Image)
	assert.Equal(t, "png", result.Content[1].Image.Format)
	assert.Equal(t, "iVBORw0KGgo=", result.Content[1].Image.Source.Bytes)
}

// ==================== claudeContentBlocksToBedrockContents Tests ====================

func TestClaudeContentBlocksToBedrockContents(t *testing.T) {
	tests := []struct {
		name     string
		blocks   []claudeChatMessageContent
		expected int
		check    func(t *testing.T, result []bedrockMessageContent)
	}{
		{name: "text block", blocks: []claudeChatMessageContent{{Type: "text", Text: "Hello"}}, expected: 1,
			check: func(t *testing.T, r []bedrockMessageContent) { assert.Equal(t, "Hello", r[0].Text) }},
		{name: "image block", blocks: []claudeChatMessageContent{{Type: "image", Source: &claudeChatMessageContentSource{Type: "base64", MediaType: "image/png", Data: "base64data"}}}, expected: 1,
			check: func(t *testing.T, r []bedrockMessageContent) { require.NotNil(t, r[0].Image); assert.Equal(t, "png", r[0].Image.Format) }},
		{name: "tool_use block", blocks: []claudeChatMessageContent{{Type: "tool_use", Id: "tool_123", Name: "search", Input: util.Ptr(map[string]interface{}{"query": "test"})}}, expected: 1,
			check: func(t *testing.T, r []bedrockMessageContent) { require.NotNil(t, r[0].ToolUse); assert.Equal(t, "tool_123", r[0].ToolUse.ToolUseId) }},
		{name: "tool_use nil input", blocks: []claudeChatMessageContent{{Type: "tool_use", Id: "tool_456", Name: "compute", Input: nil}}, expected: 1,
			check: func(t *testing.T, r []bedrockMessageContent) { require.NotNil(t, r[0].ToolUse); assert.Empty(t, r[0].ToolUse.Input) }},
		{name: "thinking block", blocks: []claudeChatMessageContent{{Type: "thinking", Thinking: "I need to think", Signature: "sig_123"}}, expected: 1,
			check: func(t *testing.T, r []bedrockMessageContent) { require.NotNil(t, r[0].ReasoningContent); require.NotNil(t, r[0].ReasoningContent.ReasoningText); assert.Equal(t, "sig_123", r[0].ReasoningContent.ReasoningText.Signature) }},
		{name: "redacted_thinking block", blocks: []claudeChatMessageContent{{Type: "redacted_thinking", Data: "redacted_data"}}, expected: 1,
			check: func(t *testing.T, r []bedrockMessageContent) { require.NotNil(t, r[0].ReasoningContent); assert.Equal(t, "redacted_data", r[0].ReasoningContent.RedactedContent) }},
		{name: "tool_result block", blocks: []claudeChatMessageContent{{Type: "tool_result", ToolUseId: "tool_123", Content: &claudeChatMessageContentWr{StringValue: "result text", IsString: true}}}, expected: 1,
			check: func(t *testing.T, r []bedrockMessageContent) { require.NotNil(t, r[0].ToolResult); assert.Equal(t, "tool_123", r[0].ToolResult.ToolUseId) }},
		{name: "image non-base64 skipped", blocks: []claudeChatMessageContent{{Type: "image", Source: &claudeChatMessageContentSource{Type: "url", Url: "https://example.com/image.png"}}}, expected: 0},
		{name: "image nil source skipped", blocks: []claudeChatMessageContent{{Type: "image"}}, expected: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := claudeContentBlocksToBedrockContents(tt.blocks)
			require.Len(t, result, tt.expected)
			if tt.check != nil {
				tt.check(t, result)
			}
		})
	}
}

// ==================== claudeToolResultBlockToBedrock Tests ====================

func TestClaudeToolResultBlockToBedrock_StringContent(t *testing.T) {
	block := claudeChatMessageContent{Type: "tool_result", ToolUseId: "tool_123",
		Content: &claudeChatMessageContentWr{StringValue: "result text", IsString: true}}
	result := claudeToolResultBlockToBedrock(block)
	assert.Equal(t, "tool_123", result.ToolUseId)
	require.Len(t, result.Content, 1)
	require.NotNil(t, result.Content[0].Text)
	assert.Equal(t, "result text", *result.Content[0].Text)
}

func TestClaudeToolResultBlockToBedrock_NilContent(t *testing.T) {
	block := claudeChatMessageContent{Type: "tool_result", ToolUseId: "tool_456", Content: nil}
	result := claudeToolResultBlockToBedrock(block)
	assert.Equal(t, "tool_456", result.ToolUseId)
	require.Len(t, result.Content, 1)
	require.NotNil(t, result.Content[0].Text)
	assert.Equal(t, "", *result.Content[0].Text)
}

func TestClaudeToolResultBlockToBedrock_ErrorStatus(t *testing.T) {
	block := claudeChatMessageContent{Type: "tool_result", ToolUseId: "tool_789", IsError: true,
		Content: &claudeChatMessageContentWr{StringValue: "error occurred", IsString: true}}
	result := claudeToolResultBlockToBedrock(block)
	assert.Equal(t, "tool_789", result.ToolUseId)
	assert.Equal(t, "error", result.Status)
}

func TestClaudeToolResultBlockToBedrock_ArrayContent(t *testing.T) {
	block := claudeChatMessageContent{Type: "tool_result", ToolUseId: "tool_arr",
		Content: &claudeChatMessageContentWr{IsString: false, ArrayValue: []claudeChatMessageContent{
			{Type: "text", Text: "text result"},
			{Type: "image", Source: &claudeChatMessageContentSource{Type: "base64", MediaType: "image/png", Data: "base64img"}},
		}}}
	result := claudeToolResultBlockToBedrock(block)
	assert.Equal(t, "tool_arr", result.ToolUseId)
	require.Len(t, result.Content, 2)
	require.NotNil(t, result.Content[0].Text)
	assert.Equal(t, "text result", *result.Content[0].Text)
	require.NotNil(t, result.Content[1].Image)
	assert.Equal(t, "png", result.Content[1].Image.Format)
}

func TestClaudeToolResultBlockToBedrock_EmptyArrayContent(t *testing.T) {
	block := claudeChatMessageContent{Type: "tool_result", ToolUseId: "tool_empty",
		Content: &claudeChatMessageContentWr{IsString: false, ArrayValue: []claudeChatMessageContent{}}}
	result := claudeToolResultBlockToBedrock(block)
	assert.Equal(t, "tool_empty", result.ToolUseId)
	require.Len(t, result.Content, 1)
	require.NotNil(t, result.Content[0].Text)
	assert.Equal(t, "", *result.Content[0].Text)
}

// ==================== buildBedrockImageGenerationRequest Tests ====================

func TestBuildBedrockImageGenerationRequest(t *testing.T) {
	p := &bedrockProvider{config: ProviderConfig{}}
	request := &imageGenerationRequest{
		Model:  "stability.stable-diffusion-xl",
		Prompt: "a beautiful sunset over mountains",
		Size:   "1024x1024",
		N:      1,
	}
	body, err := p.buildBedrockImageGenerationRequest(request, http.Header{})
	require.NoError(t, err)
	var parsed bedrockImageGenerationRequest
	require.NoError(t, json.Unmarshal(body, &parsed))
	assert.Equal(t, "TEXT_IMAGE", parsed.TaskType)
	require.NotNil(t, parsed.TextToImageParams)
	assert.Equal(t, "a beautiful sunset over mountains", parsed.TextToImageParams.Text)
	require.NotNil(t, parsed.ImageGenerationConfig)
	assert.Equal(t, 1, parsed.ImageGenerationConfig.NumberOfImages)
	assert.Equal(t, 1024, parsed.ImageGenerationConfig.Width)
	assert.Equal(t, 1024, parsed.ImageGenerationConfig.Height)
}

// ==================== buildBedrockImageGenerationResponse Tests ====================

func TestBuildBedrockImageGenerationResponse(t *testing.T) {
	p := &bedrockProvider{config: ProviderConfig{}}
	bedrockResp := &bedrockImageGenerationResponse{
		Images: []string{"base64imagedata=="},
	}
	resp := p.buildBedrockImageGenerationResponse(bedrockResp)
	require.NotNil(t, resp)
	require.Len(t, resp.Data, 1)
	assert.Equal(t, "base64imagedata==", resp.Data[0].B64)
}

// ==================== buildChatCompletionResponse Tests ====================

func TestBuildChatCompletionResponse_TextOnly(t *testing.T) {
	p := &bedrockProvider{config: ProviderConfig{}}
	bedrockResp := &bedrockConverseResponse{
		Output: converseOutputMemberMessage{
			Message: message{
				Role:    roleAssistant,
				Content: []contentBlock{{Text: "Hello from Bedrock"}},
			},
		},
		StopReason: "end_turn",
		Usage: tokenUsage{
			InputTokens:  10,
			OutputTokens: 20,
			TotalTokens:  30,
		},
	}
	result := p.buildChatCompletionResponse(newMockBedrockHttpContext(), bedrockResp)
	require.NotNil(t, result)
	require.Len(t, result.Choices, 1)
	assert.Equal(t, "Hello from Bedrock", result.Choices[0].Message.Content)
	assert.Equal(t, finishReasonStop, *result.Choices[0].FinishReason)
	require.NotNil(t, result.Usage)
	assert.Equal(t, 10, result.Usage.PromptTokens)
	assert.Equal(t, 20, result.Usage.CompletionTokens)
	assert.Equal(t, 30, result.Usage.TotalTokens)
}

func TestBuildChatCompletionResponse_WithToolUse(t *testing.T) {
	p := &bedrockProvider{config: ProviderConfig{}}
	bedrockResp := &bedrockConverseResponse{
		Output: converseOutputMemberMessage{
			Message: message{
				Role: roleAssistant,
				Content: []contentBlock{
					{Text: "Let me check the weather."},
					{ToolUse: &bedrockToolUse{ToolUseId: "tool_123", Name: "get_weather", Input: map[string]interface{}{"city": "NYC"}}},
				},
			},
		},
		StopReason: "tool_use",
		Usage:      tokenUsage{InputTokens: 15, OutputTokens: 25, TotalTokens: 40},
	}
	result := p.buildChatCompletionResponse(newMockBedrockHttpContext(), bedrockResp)
	require.Len(t, result.Choices, 1)
	assert.Equal(t, finishReasonToolCall, *result.Choices[0].FinishReason)
	require.Len(t, result.Choices[0].Message.ToolCalls, 1)
	assert.Equal(t, "tool_123", result.Choices[0].Message.ToolCalls[0].Id)
	assert.Equal(t, "get_weather", result.Choices[0].Message.ToolCalls[0].Function.Name)
}

func TestBuildChatCompletionResponse_WithReasoningContent(t *testing.T) {
	p := &bedrockProvider{config: ProviderConfig{}}
	bedrockResp := &bedrockConverseResponse{
		Output: converseOutputMemberMessage{
			Message: message{
				Role: roleAssistant,
				Content: []contentBlock{
					{ReasoningContent: &reasoningContent{ReasoningText: &reasoningText{
						Text:      "I need to think about this",
						Signature: "sig_abc",
					}}},
					{Text: "Here is my answer"},
				},
			},
		},
		StopReason: "end_turn",
		Usage:      tokenUsage{InputTokens: 5, OutputTokens: 10, TotalTokens: 15},
	}
	result := p.buildChatCompletionResponse(newMockBedrockHttpContext(), bedrockResp)
	require.Len(t, result.Choices, 1)
	assert.Equal(t, "I need to think about this", result.Choices[0].Message.ReasoningContent)
	assert.Equal(t, "Here is my answer", result.Choices[0].Message.Content)
}

func TestBuildChatCompletionResponse_WithCacheUsage(t *testing.T) {
	p := &bedrockProvider{config: ProviderConfig{}}
	bedrockResp := &bedrockConverseResponse{
		Output: converseOutputMemberMessage{
			Message: message{
				Role:    roleAssistant,
				Content: []contentBlock{{Text: "Cached response"}},
			},
		},
		StopReason: "end_turn",
		Usage: tokenUsage{
			InputTokens:       100,
			OutputTokens:      50,
			TotalTokens:       150,
			CacheReadInputTokens:  80,
			CacheWriteInputTokens: 20,
		},
	}
	result := p.buildChatCompletionResponse(newMockBedrockHttpContext(), bedrockResp)
	require.NotNil(t, result.Usage)
	require.NotNil(t, result.Usage.PromptTokensDetails)
	assert.Equal(t, 100, result.Usage.PromptTokensDetails.CachedTokens)
}

func TestBuildChatCompletionResponse_MaxTokens(t *testing.T) {
	p := &bedrockProvider{config: ProviderConfig{}}
	bedrockResp := &bedrockConverseResponse{
		Output: converseOutputMemberMessage{
			Message: message{
				Role:    roleAssistant,
				Content: []contentBlock{{Text: "Truncated"}},
			},
		},
		StopReason: "max_tokens",
		Usage:      tokenUsage{InputTokens: 5, OutputTokens: 10, TotalTokens: 15},
	}
	result := p.buildChatCompletionResponse(newMockBedrockHttpContext(), bedrockResp)
	assert.Equal(t, finishReasonLength, *result.Choices[0].FinishReason)
}

func TestBuildChatCompletionResponse_EmptyContent(t *testing.T) {
	p := &bedrockProvider{config: ProviderConfig{}}
	bedrockResp := &bedrockConverseResponse{
		Output: converseOutputMemberMessage{
			Message: message{
				Role:    roleAssistant,
				Content: []contentBlock{},
			},
		},
		StopReason: "end_turn",
		Usage:      tokenUsage{InputTokens: 5, OutputTokens: 0, TotalTokens: 5},
	}
	result := p.buildChatCompletionResponse(newMockBedrockHttpContext(), bedrockResp)
	require.Len(t, result.Choices, 1)
	assert.Equal(t, "", result.Choices[0].Message.Content)
}

// ==================== Event Stream decodeMessage Tests ====================

// buildEventStreamMessage creates a valid Amazon Event Stream encoded message for testing
func buildEventStreamMessage(headers map[string]string, payload []byte) []byte {
	var headersBuf bytes.Buffer
	for k, v := range headers {
		headerName := []byte(k)
		headerValue := []byte(v)
		headersBuf.WriteByte(byte(len(headerName)))
		headersBuf.Write(headerName)
		headersBuf.WriteByte(7) // String type
		headersBuf.Write([]byte{0, byte(len(headerValue))})
		headersBuf.Write(headerValue)
	}

	prelude := make([]byte, 12)
	totalLen := uint32(16 + headersBuf.Len() + len(payload))
	binary.BigEndian.PutUint32(prelude[0:4], totalLen)
	binary.BigEndian.PutUint32(prelude[4:8], uint32(headersBuf.Len()))
	preludeCRC := crc32.ChecksumIEEE(prelude[0:8])
	binary.BigEndian.PutUint32(prelude[8:12], preludeCRC)

	var msg bytes.Buffer
	msg.Write(prelude)
	msg.Write(headersBuf.Bytes())
	msg.Write(payload)

	messageCRC := crc32.ChecksumIEEE(msg.Bytes())
	var crcBuf [4]byte
	binary.BigEndian.PutUint32(crcBuf[:], messageCRC)
	msg.Write(crcBuf[:])

	return msg.Bytes()
}

func TestDecodeMessage_ValidMessage(t *testing.T) {
	headers := map[string]string{":event-type": "message-start"}
	payload, _ := json.Marshal(map[string]interface{}{"role": "assistant"})
	msg := buildEventStreamMessage(headers, payload)

	reader := bytes.NewReader(msg)
	m, err := decodeMessage(reader, make([]byte, 1024))
	require.NoError(t, err)
	assert.Equal(t, payload, m.Payload)
	// Verify the event-type header is present
	require.NotEmpty(t, m.Headers)
	found := false
	for _, h := range m.Headers {
		if h.Name == ":event-type" {
			found = true
			val, ok := h.Value.Get().(string)
			require.True(t, ok)
			assert.Equal(t, "message-start", val)
		}
	}
	assert.True(t, found, "expected :event-type header to be found")
}

func TestDecodeMessage_CRCMismatch(t *testing.T) {
	msg := buildEventStreamMessage(map[string]string{":event-type": "test"}, []byte("data"))
	msg[len(msg)-1] ^= 0xFF // Corrupt the message CRC
	_, err := decodeMessage(bytes.NewReader(msg), make([]byte, 1024))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checksum")
}

func TestDecodeMessage_TooShort(t *testing.T) {
	_, err := decodeMessage(bytes.NewReader([]byte{0x00, 0x00, 0x00, 0x10}), make([]byte, 1024))
	require.Error(t, err)
}

func TestDecodeMessage_EmptyPayload(t *testing.T) {
	headers := map[string]string{":event-type": "content-block-stop"}
	msg := buildEventStreamMessage(headers, []byte{})

	reader := bytes.NewReader(msg)
	m, err := decodeMessage(reader, make([]byte, 1024))
	require.NoError(t, err)
	assert.Empty(t, m.Payload)
}

// ==================== decodeHeaders Tests ====================

func TestDecodeHeaders_StringHeader(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteByte(4) // header name length
	buf.Write([]byte("type"))
	buf.WriteByte(7) // string type
	buf.Write([]byte{0, 5}) // value length
	buf.Write([]byte("event"))

	hs, err := decodeHeaders(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)
	require.Len(t, hs, 1)
	assert.Equal(t, "type", hs[0].Name)
	val, ok := hs[0].Value.Get().(string)
	require.True(t, ok)
	assert.Equal(t, "event", val)
}

func TestDecodeHeaders_EmptyHeaders(t *testing.T) {
	hs, err := decodeHeaders(bytes.NewReader([]byte{}))
	require.NoError(t, err)
	assert.Empty(t, hs)
}

func TestDecodeHeaders_MultipleHeaders(t *testing.T) {
	var buf bytes.Buffer
	// Header 1: :event-type = content-block-delta
	buf.WriteByte(11)
	buf.Write([]byte(":event-type"))
	buf.WriteByte(7)
	buf.Write([]byte{0, 19})
	buf.Write([]byte("content-block-delta"))
	// Header 2: :message-type = event
	buf.WriteByte(13)
	buf.Write([]byte(":message-type"))
	buf.WriteByte(7)
	buf.Write([]byte{0, 5})
	buf.Write([]byte("event"))

	hs, err := decodeHeaders(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)
	require.Len(t, hs, 2)
	assert.Equal(t, ":event-type", hs[0].Name)
	assert.Equal(t, ":message-type", hs[1].Name)
}

// ==================== amazonEventType Tests ====================

func TestAmazonEventType(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		hs := headers{
			{Name: ":event-type", Value: StringValue("content-block-delta")},
			{Name: ":message-type", Value: StringValue("event")},
		}
		eventType, ok := amazonEventType(hs)
		assert.True(t, ok)
		assert.Equal(t, "content-block-delta", eventType)
	})
	t.Run("not found", func(t *testing.T) {
		hs := headers{
			{Name: ":message-type", Value: StringValue("event")},
		}
		_, ok := amazonEventType(hs)
		assert.False(t, ok)
	})
	t.Run("empty headers", func(t *testing.T) {
		hs := headers{}
		_, ok := amazonEventType(hs)
		assert.False(t, ok)
	})
}

// ==================== overwriteRequestPathHeader Tests ====================

func TestOverwriteRequestPathHeader(t *testing.T) {
	p := &bedrockProvider{config: ProviderConfig{}}
	headers := http.Header{}
	p.overwriteRequestPathHeader(headers, "/model/%s/converse", "anthropic.claude-3-sonnet")
	path := headers.Get(":path")
	assert.Contains(t, path, "anthropic.claude-3-sonnet")
	assert.Contains(t, path, "/converse")
}

// ==================== stopReasonBedrock2OpenAI additional coverage ====================

func TestStopReasonBedrock2OpenAI_AllKnownReasons(t *testing.T) {
	assert.Equal(t, finishReasonStop, stopReasonBedrock2OpenAI("end_turn"))
	assert.Equal(t, finishReasonStop, stopReasonBedrock2OpenAI("stop_sequence"))
	assert.Equal(t, finishReasonLength, stopReasonBedrock2OpenAI("max_tokens"))
	assert.Equal(t, finishReasonToolCall, stopReasonBedrock2OpenAI("tool_use"))
	assert.Equal(t, "custom_reason", stopReasonBedrock2OpenAI("custom_reason"))
}