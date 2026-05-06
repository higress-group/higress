package provider

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-proxy/util"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

// claudeProvider is the provider for Claude service.
const (
	claudeDomain           = "api.anthropic.com"
	claudeDefaultVersion   = "2023-06-01"
	claudeDefaultMaxTokens = 4096

	// Claude Code mode constants
	claudeCodeUserAgent    = "claude-cli/2.1.2 (external, cli)"
	claudeCodeBetaFeatures = "oauth-2025-04-20,interleaved-thinking-2025-05-14,claude-code-20250219"
	claudeCodeSystemPrompt = "You are Claude Code, Anthropic's official CLI for Claude."
)

type claudeProviderInitializer struct{}

type claudeTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	InputSchema map[string]interface{} `json:"input_schema,omitempty"`
}

type claudeToolChoice struct {
	Type                   string `json:"type"`
	Name                   string `json:"name,omitempty"`
	DisableParallelToolUse bool   `json:"disable_parallel_tool_use,omitempty"`
}

type claudeChatMessage struct {
	Role    string                     `json:"role"`
	Content claudeChatMessageContentWr `json:"content"`
}

// claudeChatMessageContentWr wraps the content to handle both string and array formats
type claudeChatMessageContentWr struct {
	// StringValue holds simple text content
	StringValue string
	// ArrayValue holds multi-modal content
	ArrayValue []claudeChatMessageContent
	// IsString indicates whether this is a simple string or array
	IsString bool
}

type claudeChatMessageContentSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type,omitempty"`
	Data      string `json:"data,omitempty"`
	Url       string `json:"url,omitempty"`
	FileId    string `json:"file_id,omitempty"`
}

type claudeChatMessageContent struct {
	Type         string                          `json:"type"`
	Text         string                          `json:"text,omitempty"`
	Source       *claudeChatMessageContentSource `json:"source,omitempty"`
	CacheControl map[string]interface{}          `json:"cache_control,omitempty"`
	// Tool use fields
	Id    string                 `json:"id,omitempty"`    // For tool_use
	Name  string                 `json:"name,omitempty"`  // For tool_use
	Input map[string]interface{} `json:"input,omitempty"` // For tool_use
	// Tool result fields
	ToolUseId string                      `json:"tool_use_id,omitempty"` // For tool_result
	Content   *claudeChatMessageContentWr `json:"content,omitempty"`     // For tool_result - can be string or array
}

// UnmarshalJSON implements custom JSON unmarshaling for claudeChatMessageContentWr
func (ccw *claudeChatMessageContentWr) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as string first
	var stringValue string
	if err := json.Unmarshal(data, &stringValue); err == nil {
		ccw.StringValue = stringValue
		ccw.IsString = true
		return nil
	}

	// Try to unmarshal as array of content blocks
	var arrayValue []claudeChatMessageContent
	if err := json.Unmarshal(data, &arrayValue); err == nil {
		ccw.ArrayValue = arrayValue
		ccw.IsString = false
		return nil
	} else {
		log.Errorf("claude chat message unmarshal failed, data:%s, err:%v", data, err)
	}

	return fmt.Errorf("content field must be either a string or an array of content blocks")
}

// MarshalJSON implements custom JSON marshaling for claudeChatMessageContentWr
func (ccw claudeChatMessageContentWr) MarshalJSON() ([]byte, error) {
	if ccw.IsString {
		return json.Marshal(ccw.StringValue)
	}
	return json.Marshal(ccw.ArrayValue)
}

// GetStringValue returns the string representation if it's a string, or concatenated text from array blocks
func (ccw claudeChatMessageContentWr) GetStringValue() string {
	if ccw.IsString {
		return ccw.StringValue
	}
	// If it's an array, concatenate text content from all blocks
	var parts []string
	for _, block := range ccw.ArrayValue {
		if block.Text != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// GetArrayValue returns the array representation if it's an array, empty slice otherwise
func (ccw claudeChatMessageContentWr) GetArrayValue() []claudeChatMessageContent {
	if !ccw.IsString {
		return ccw.ArrayValue
	}
	return nil
}

// NewStringContent creates a new wrapper for string content
func NewStringContent(content string) claudeChatMessageContentWr {
	return claudeChatMessageContentWr{
		StringValue: content,
		IsString:    true,
	}
}

// NewArrayContent creates a new wrapper for array content
func NewArrayContent(content []claudeChatMessageContent) claudeChatMessageContentWr {
	return claudeChatMessageContentWr{
		ArrayValue: content,
		IsString:   false,
	}
}

// claudeSystemPrompt represents the system field which can be either a string or an array of text blocks
type claudeSystemPrompt struct {
	// Will be set to the string value if system is a simple string
	StringValue string
	// Will be set to the array value if system is an array of text blocks
	ArrayValue []claudeChatMessageContent
	// Indicates which type this represents
	IsArray bool
}

// UnmarshalJSON implements custom JSON unmarshaling for claudeSystemPrompt
func (csp *claudeSystemPrompt) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as string first
	var stringValue string
	if err := json.Unmarshal(data, &stringValue); err == nil {
		csp.StringValue = stringValue
		csp.IsArray = false
		return nil
	}

	// Try to unmarshal as array of text blocks
	var arrayValue []claudeChatMessageContent
	if err := json.Unmarshal(data, &arrayValue); err == nil {
		csp.ArrayValue = arrayValue
		csp.IsArray = true
		return nil
	}

	return fmt.Errorf("system field must be either a string or an array of text blocks")
}

// MarshalJSON implements custom JSON marshaling for claudeSystemPrompt
func (csp claudeSystemPrompt) MarshalJSON() ([]byte, error) {
	if csp.IsArray {
		return json.Marshal(csp.ArrayValue)
	}
	return json.Marshal(csp.StringValue)
}

// String returns the string representation of the system prompt
func (csp claudeSystemPrompt) String() string {
	if csp.IsArray {
		// Concatenate all text blocks
		var parts []string
		for _, block := range csp.ArrayValue {
			if block.Text != "" {
				parts = append(parts, block.Text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return csp.StringValue
}

// claudeThinkingConfig represents the thinking configuration for Claude
type claudeThinkingConfig struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens,omitempty"`
}

type claudeTextGenRequest struct {
	Model            string                `json:"model,omitempty"`
	Messages         []claudeChatMessage   `json:"messages"`
	System           *claudeSystemPrompt   `json:"system,omitempty"`
	MaxTokens        int                   `json:"max_tokens,omitempty"`
	StopSequences    []string              `json:"stop_sequences,omitempty"`
	Stream           bool                  `json:"stream,omitempty"`
	Temperature      float64               `json:"temperature,omitempty"`
	TopP             float64               `json:"top_p,omitempty"`
	TopK             int                   `json:"top_k,omitempty"`
	ToolChoice       *claudeToolChoice     `json:"tool_choice,omitempty"`
	Tools            []claudeTool          `json:"tools,omitempty"`
	ServiceTier      string                `json:"service_tier,omitempty"`
	Thinking         *claudeThinkingConfig `json:"thinking,omitempty"`
	AnthropicVersion string                `json:"anthropic_version,omitempty"`
}

type claudeTextGenResponse struct {
	Id           string                 `json:"id"`
	Type         string                 `json:"type"`
	Role         string                 `json:"role"`
	Content      []claudeTextGenContent `json:"content"`
	Model        string                 `json:"model"`
	StopReason   *string                `json:"stop_reason"`
	StopSequence *string                `json:"stop_sequence"`
	Usage        claudeTextGenUsage     `json:"usage"`
	Error        *claudeTextGenError    `json:"error"`
}

type claudeTextGenContent struct {
	Type      string                  `json:"type,omitempty"`
	Text      *string                 `json:"text,omitempty"`      // Use pointer: empty string outputs "text":"", nil omits field
	Id        string                  `json:"id,omitempty"`        // For tool_use
	Name      string                  `json:"name,omitempty"`      // For tool_use
	Input     *map[string]interface{} `json:"input,omitempty"`     // Use pointer: empty map outputs "input":{}, nil omits field
	Signature *string                 `json:"signature,omitempty"` // For thinking - use pointer for empty string output
	Thinking  *string                 `json:"thinking,omitempty"`  // For thinking - use pointer for empty string output
}

type claudeTextGenUsage struct {
	InputTokens              int    `json:"input_tokens,omitempty"`
	OutputTokens             int    `json:"output_tokens,omitempty"`
	CacheReadInputTokens     int    `json:"cache_read_input_tokens,omitempty"`
	CacheCreationInputTokens int    `json:"cache_creation_input_tokens,omitempty"`
	ServiceTier              string `json:"service_tier,omitempty"`
}

type claudeTextGenError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type claudeTextGenStreamResponse struct {
	Type         string                 `json:"type"`
	Message      *claudeTextGenResponse `json:"message,omitempty"`
	Index        *int                   `json:"index,omitempty"`
	ContentBlock *claudeTextGenContent  `json:"content_block,omitempty"`
	Delta        *claudeTextGenDelta    `json:"delta,omitempty"`
	Usage        *claudeTextGenUsage    `json:"usage,omitempty"`
}

type claudeTextGenDelta struct {
	Type         string          `json:"type,omitempty"`
	Text         string          `json:"text,omitempty"`
	Thinking     string          `json:"thinking,omitempty"`
	PartialJson  string          `json:"partial_json,omitempty"`
	StopReason   *string         `json:"stop_reason,omitempty"`
	StopSequence json.RawMessage `json:"stop_sequence,omitempty"` // Use RawMessage to output explicit null
}

func (c *claudeProviderInitializer) ValidateConfig(config *ProviderConfig) error {
	if len(config.apiTokens) == 0 {
		return errors.New("no apiToken found in provider config")
	}
	return nil
}

func (c *claudeProviderInitializer) DefaultCapabilities() map[string]string {
	return map[string]string{
		string(ApiNameChatCompletion):    PathAnthropicMessages,
		string(ApiNameCompletion):        PathAnthropicComplete,
		string(ApiNameAnthropicMessages): PathAnthropicMessages,
		// docs: https://docs.anthropic.com/en/docs/build-with-claude/embeddings#voyage-http-api
		string(ApiNameEmbeddings): PathOpenAIEmbeddings,
		string(ApiNameModels):     PathOpenAIModels,
	}
}

func (c *claudeProviderInitializer) CreateProvider(config ProviderConfig) (Provider, error) {
	config.setDefaultCapabilities(c.DefaultCapabilities())
	return &claudeProvider{
		config:       config,
		contextCache: createContextCache(&config),
	}, nil
}

type claudeProvider struct {
	config       ProviderConfig
	contextCache *contextCache

	messageId   string
	usage       usage
	serviceTier string
}

func (c *claudeProvider) GetProviderType() string {
	return providerTypeClaude
}

func (c *claudeProvider) OnRequestHeaders(ctx wrapper.HttpContext, apiName ApiName) error {
	c.config.handleRequestHeaders(c, ctx, apiName)
	return nil
}

func (c *claudeProvider) TransformRequestHeaders(ctx wrapper.HttpContext, apiName ApiName, headers http.Header) {
	util.OverwriteRequestPathHeaderByCapability(headers, string(apiName), c.config.capabilities)
	util.OverwriteRequestHostHeader(headers, claudeDomain)

	if c.config.apiVersion == "" {
		c.config.apiVersion = claudeDefaultVersion
	}
	headers.Set("anthropic-version", c.config.apiVersion)

	// Check if Claude Code mode is enabled
	if c.config.claudeCodeMode {
		// Claude Code mode: use OAuth token with Bearer authorization
		token := c.config.GetApiTokenInUse(ctx)
		headers.Set("authorization", "Bearer "+token)
		headers.Del("x-api-key")

		// Set Claude Code specific headers
		headers.Set("user-agent", claudeCodeUserAgent)
		headers.Set("x-app", "cli")
		headers.Set("anthropic-beta", claudeCodeBetaFeatures)

		// Add ?beta=true query parameter to the path
		currentPath := headers.Get(":path")
		if currentPath != "" && !strings.Contains(currentPath, "beta=true") {
			if strings.Contains(currentPath, "?") {
				headers.Set(":path", currentPath+"&beta=true")
			} else {
				headers.Set(":path", currentPath+"?beta=true")
			}
		}
	} else {
		// Standard mode: use x-api-key
		headers.Set("x-api-key", c.config.GetApiTokenInUse(ctx))
	}
}

func (c *claudeProvider) OnRequestBody(ctx wrapper.HttpContext, apiName ApiName, body []byte) (types.Action, error) {
	if !c.config.isSupportedAPI(apiName) {
		return types.ActionContinue, nil
	}
	return c.config.handleRequestBody(c, c.contextCache, ctx, apiName, body)
}

func (c *claudeProvider) TransformRequestBody(ctx wrapper.HttpContext, apiName ApiName, body []byte) ([]byte, error) {
	if apiName != ApiNameChatCompletion {
		return c.config.defaultTransformRequestBody(ctx, apiName, body)
	}
	request := &chatCompletionRequest{}
	if err := c.config.parseRequestAndMapModel(ctx, request, body); err != nil {
		return nil, err
	}
	claudeRequest := c.buildClaudeTextGenRequest(request)
	return json.Marshal(claudeRequest)
}

func (c *claudeProvider) TransformResponseBody(ctx wrapper.HttpContext, apiName ApiName, body []byte) ([]byte, error) {
	if apiName != ApiNameChatCompletion {
		return body, nil
	}
	claudeResponse := &claudeTextGenResponse{}
	if err := json.Unmarshal(body, claudeResponse); err != nil {
		return nil, fmt.Errorf("unable to unmarshal claude response: %v", err)
	}
	if claudeResponse.Error != nil {
		return nil, fmt.Errorf("claude response error, error_type: %s, error_message: %s", claudeResponse.Error.Type, claudeResponse.Error.Message)
	}
	response := c.responseClaude2OpenAI(ctx, claudeResponse)
	return json.Marshal(response)
}

func (c *claudeProvider) OnStreamingResponseBody(ctx wrapper.HttpContext, name ApiName, chunk []byte, isLastChunk bool) ([]byte, error) {
	if isLastChunk || len(chunk) == 0 {
		return nil, nil
	}
	// only process the response from chat completion, skip other responses
	if name != ApiNameChatCompletion {
		return chunk, nil
	}

	responseBuilder := &strings.Builder{}
	lines := strings.Split(string(chunk), "\n")
	for _, data := range lines {
		// only process the line starting with "data:"
		if strings.HasPrefix(data, "data:") {
			// extract json data from the line
			jsonData := strings.TrimPrefix(data, "data:")
			var claudeResponse claudeTextGenStreamResponse
			if err := json.Unmarshal([]byte(jsonData), &claudeResponse); err != nil {
				log.Errorf("unable to unmarshal claude response: %v", err)
				continue
			}
			response := c.streamResponseClaude2OpenAI(ctx, &claudeResponse)
			if response != nil {
				responseBody, err := json.Marshal(response)
				if err != nil {
					log.Errorf("unable to marshal response: %v", err)
					return nil, err
				}
				c.appendResponse(responseBuilder, string(responseBody))
			}
		}
	}
	modifiedResponseChunk := responseBuilder.String()
	log.Debugf("modified response chunk: %s", modifiedResponseChunk)
	return []byte(modifiedResponseChunk), nil
}

func (c *claudeProvider) buildClaudeTextGenRequest(origRequest *chatCompletionRequest) *claudeTextGenRequest {
	claudeRequest := claudeTextGenRequest{
		Model:         origRequest.Model,
		MaxTokens:     origRequest.getMaxTokens(),
		StopSequences: origRequest.Stop,
		Stream:        origRequest.Stream,
		Temperature:   origRequest.Temperature,
		TopP:          origRequest.TopP,
		// ServiceTier:   origRequest.ServiceTier,
	}
	if claudeRequest.MaxTokens == 0 {
		claudeRequest.MaxTokens = claudeDefaultMaxTokens
	}

	// Convert OpenAI reasoning parameters to Claude thinking configuration
	if origRequest.ReasoningEffort != "" || origRequest.ReasoningMaxTokens > 0 {
		var budgetTokens int
		if origRequest.ReasoningMaxTokens > 0 {
			budgetTokens = origRequest.ReasoningMaxTokens
		} else {
			// Convert reasoning_effort to budget_tokens
			switch origRequest.ReasoningEffort {
			case "low":
				budgetTokens = 1024 // Minimum required by Claude
			case "medium":
				budgetTokens = 8192
			case "high":
				budgetTokens = 16384
			default:
				budgetTokens = 8192 // Default to medium
			}
		}
		// Ensure minimum budget_tokens requirement
		if budgetTokens < 1024 {
			budgetTokens = 1024
		}
		claudeRequest.Thinking = &claudeThinkingConfig{
			Type:         "enabled",
			BudgetTokens: budgetTokens,
		}
	}

	// Track if system message exists in original request
	hasSystemMessage := false
	for _, message := range origRequest.Messages {
		if message.Role == roleSystem {
			hasSystemMessage = true
			// In Claude Code mode, use array format with cache_control
			if c.config.claudeCodeMode {
				claudeRequest.System = &claudeSystemPrompt{
					ArrayValue: []claudeChatMessageContent{
						{
							Type: contentTypeText,
							Text: message.StringContent(),
							CacheControl: map[string]interface{}{
								"type": "ephemeral",
							},
						},
					},
					IsArray: true,
				}
			} else {
				claudeRequest.System = &claudeSystemPrompt{
					StringValue: message.StringContent(),
					IsArray:     false,
				}
			}
			continue
		}

		// Handle OpenAI "tool" role messages - convert to Claude "user" role with tool_result content
		if message.Role == roleTool {
			toolResultContent := claudeChatMessageContent{
				Type:      "tool_result",
				ToolUseId: message.ToolCallId,
			}
			// Tool result content can be string or array
			if message.IsStringContent() {
				toolResultContent.Content = &claudeChatMessageContentWr{
					StringValue: message.StringContent(),
					IsString:    true,
				}
			} else {
				// For array content, extract text parts
				var textParts []string
				for _, part := range message.ParseContent() {
					if part.Type == contentTypeText {
						textParts = append(textParts, part.Text)
					}
				}
				toolResultContent.Content = &claudeChatMessageContentWr{
					StringValue: strings.Join(textParts, "\n"),
					IsString:    true,
				}
			}

			// Check if the last message is a user message with tool_result, merge if so
			if len(claudeRequest.Messages) > 0 {
				lastMsg := &claudeRequest.Messages[len(claudeRequest.Messages)-1]
				if lastMsg.Role == roleUser && !lastMsg.Content.IsString {
					// Check if last message contains tool_result
					hasToolResult := false
					for _, content := range lastMsg.Content.ArrayValue {
						if content.Type == "tool_result" {
							hasToolResult = true
							break
						}
					}
					if hasToolResult {
						// Merge with existing tool_result message
						lastMsg.Content.ArrayValue = append(lastMsg.Content.ArrayValue, toolResultContent)
						continue
					}
				}
			}

			// Create new user message with tool_result
			claudeMessage := claudeChatMessage{
				Role:    roleUser,
				Content: NewArrayContent([]claudeChatMessageContent{toolResultContent}),
			}
			claudeRequest.Messages = append(claudeRequest.Messages, claudeMessage)
			continue
		}

		claudeMessage := claudeChatMessage{
			Role: message.Role,
		}

		// Handle assistant messages with tool_calls - convert to Claude tool_use content blocks
		if message.Role == roleAssistant && len(message.ToolCalls) > 0 {
			chatMessageContents := make([]claudeChatMessageContent, 0)

			// Add text content if present
			if message.IsStringContent() && message.StringContent() != "" {
				chatMessageContents = append(chatMessageContents, claudeChatMessageContent{
					Type: contentTypeText,
					Text: message.StringContent(),
				})
			}

			// Convert tool_calls to tool_use content blocks
			for _, tc := range message.ToolCalls {
				var inputMap map[string]interface{}
				if tc.Function.Arguments != "" {
					if err := json.Unmarshal([]byte(tc.Function.Arguments), &inputMap); err != nil {
						log.Errorf("failed to parse tool call arguments: %v", err)
						inputMap = make(map[string]interface{})
					}
				} else {
					inputMap = make(map[string]interface{})
				}

				chatMessageContents = append(chatMessageContents, claudeChatMessageContent{
					Type:  "tool_use",
					Id:    tc.Id,
					Name:  tc.Function.Name,
					Input: inputMap,
				})
			}

			claudeMessage.Content = NewArrayContent(chatMessageContents)
			claudeRequest.Messages = append(claudeRequest.Messages, claudeMessage)
			continue
		}

		if message.IsStringContent() {
			claudeMessage.Content = NewStringContent(message.StringContent())
		} else {
			chatMessageContents := make([]claudeChatMessageContent, 0)
			for _, messageContent := range message.ParseContent() {
				switch messageContent.Type {
				case contentTypeText:
					chatMessageContents = append(chatMessageContents, claudeChatMessageContent{
						Type: contentTypeText,
						Text: messageContent.Text,
					})
				case contentTypeImageUrl:
					if strings.HasPrefix(messageContent.ImageUrl.Url, "data:") {
						parts := strings.SplitN(messageContent.ImageUrl.Url, ";", 2)
						if len(parts) != 2 {
							log.Errorf("invalid image url format: %s", messageContent.ImageUrl.Url)
							continue
						}
						chatMessageContents = append(chatMessageContents, claudeChatMessageContent{
							Type: "image",
							Source: &claudeChatMessageContentSource{
								Type:      "base64",
								MediaType: strings.TrimPrefix(parts[0], "data:"),
								Data:      strings.TrimPrefix(parts[1], "base64,"),
							},
						})
					} else {
						chatMessageContents = append(chatMessageContents, claudeChatMessageContent{
							Type: "image",
							Source: &claudeChatMessageContentSource{
								Type: "url",
								Url:  messageContent.ImageUrl.Url,
							},
						})
					}
				case contentTypeFile:
					chatMessageContents = append(chatMessageContents, claudeChatMessageContent{
						Type: "file",
						Source: &claudeChatMessageContentSource{
							Type:   "url",
							FileId: messageContent.File.FileId,
						},
					})
				default:
					log.Errorf("Unsupported content type: %s", messageContent.Type)
					continue
				}
			}
			claudeMessage.Content = NewArrayContent(chatMessageContents)
		}
		claudeRequest.Messages = append(claudeRequest.Messages, claudeMessage)
	}

	// In Claude Code mode, add default system prompt if not present
	if c.config.claudeCodeMode && !hasSystemMessage {
		claudeRequest.System = &claudeSystemPrompt{
			ArrayValue: []claudeChatMessageContent{
				{
					Type: contentTypeText,
					Text: claudeCodeSystemPrompt,
					CacheControl: map[string]interface{}{
						"type": "ephemeral",
					},
				},
			},
			IsArray: true,
		}
	}

	for _, tool := range origRequest.Tools {
		claudeTool := claudeTool{
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			InputSchema: tool.Function.Parameters,
		}
		claudeRequest.Tools = append(claudeRequest.Tools, claudeTool)
	}

	if origRequest.ToolChoice != nil {
		parallelToolCalls := true
		if origRequest.ParallelToolCalls != nil {
			parallelToolCalls = *origRequest.ParallelToolCalls
		}

		choiceType := origRequest.getToolChoiceType()
		if tc := origRequest.getToolChoiceObject(); tc != nil && tc.Type == "function" && tc.Function.Name != "" {
			claudeRequest.ToolChoice = &claudeToolChoice{
				Name:                   tc.Function.Name,
				Type:                   "tool",
				DisableParallelToolUse: !parallelToolCalls,
			}
		} else if choiceType != "" {
			switch choiceType {
			case "required":
				choiceType = "any"
			}
			claudeRequest.ToolChoice = &claudeToolChoice{
				Type: choiceType,
			}
			if choiceType != "none" {
				claudeRequest.ToolChoice.DisableParallelToolUse = !parallelToolCalls
			}
		}
	}

	return &claudeRequest
}

func (c *claudeProvider) responseClaude2OpenAI(ctx wrapper.HttpContext, origResponse *claudeTextGenResponse) *chatCompletionResponse {
	// Extract text content, thinking content, and tool calls from Claude response
	var textContent string
	var reasoningContent string
	var toolCalls []toolCall
	for _, content := range origResponse.Content {
		switch content.Type {
		case contentTypeText:
			if content.Text != nil {
				textContent = *content.Text
			}
		case "thinking":
			if content.Thinking != nil {
				reasoningContent = *content.Thinking
			}
		case "tool_use":
			var args []byte
			if content.Input != nil {
				args, _ = json.Marshal(*content.Input)
			} else {
				args = []byte("{}")
			}
			toolCalls = append(toolCalls, toolCall{
				Id:   content.Id,
				Type: "function",
				Function: functionCall{
					Name:      content.Name,
					Arguments: string(args),
				},
			})
		}
	}

	choice := chatCompletionChoice{
		Index:        0,
		Message:      &chatMessage{Role: roleAssistant, Content: textContent, ReasoningContent: reasoningContent, ToolCalls: toolCalls},
		FinishReason: util.Ptr(stopReasonClaude2OpenAI(origResponse.StopReason)),
	}

	response := &chatCompletionResponse{
		Id:                origResponse.Id,
		Created:           time.Now().UnixMilli() / 1000,
		Model:             ctx.GetStringContext(ctxKeyFinalRequestModel, ""),
		SystemFingerprint: "",
		Object:            objectChatCompletion,
		Choices:           []chatCompletionChoice{choice},
	}

	// Include usage information if available
	if origResponse.Usage.InputTokens > 0 || origResponse.Usage.OutputTokens > 0 {
		response.Usage = &usage{
			PromptTokens:     origResponse.Usage.InputTokens,
			CompletionTokens: origResponse.Usage.OutputTokens,
			TotalTokens:      origResponse.Usage.InputTokens + origResponse.Usage.OutputTokens,
		}
	}

	return response
}

func stopReasonClaude2OpenAI(reason *string) string {
	if reason == nil {
		return ""
	}
	switch *reason {
	case "end_turn":
		return finishReasonStop
	case "stop_sequence":
		return finishReasonStop
	case "max_tokens":
		return finishReasonLength
	case "tool_use":
		return finishReasonToolCall
	default:
		return *reason
	}
}

func (c *claudeProvider) streamResponseClaude2OpenAI(ctx wrapper.HttpContext, origResponse *claudeTextGenStreamResponse) *chatCompletionResponse {
	switch origResponse.Type {
	case "message_start":
		if origResponse.Message != nil {
			c.messageId = origResponse.Message.Id
			c.usage = usage{
				PromptTokens:     origResponse.Message.Usage.InputTokens,
				CompletionTokens: origResponse.Message.Usage.OutputTokens,
			}
			c.serviceTier = origResponse.Message.Usage.ServiceTier
		}
		var index int
		if origResponse.Index != nil {
			index = *origResponse.Index
		}
		choice := chatCompletionChoice{
			Index: index,
			Delta: &chatMessage{Role: roleAssistant, Content: ""},
		}
		return c.createChatCompletionResponse(ctx, origResponse, choice)

	case "content_block_start":
		// Handle tool_use content block start
		if origResponse.ContentBlock != nil && origResponse.ContentBlock.Type == "tool_use" {
			var index int
			if origResponse.Index != nil {
				index = *origResponse.Index
			}
			choice := chatCompletionChoice{
				Index: index,
				Delta: &chatMessage{
					ToolCalls: []toolCall{
						{
							Index: index,
							Id:    origResponse.ContentBlock.Id,
							Type:  "function",
							Function: functionCall{
								Name:      origResponse.ContentBlock.Name,
								Arguments: "",
							},
						},
					},
				},
			}
			return c.createChatCompletionResponse(ctx, origResponse, choice)
		}
		return nil

	case "content_block_delta":
		var index int
		if origResponse.Index != nil {
			index = *origResponse.Index
		}
		// Handle tool_use input_json_delta
		if origResponse.Delta != nil && origResponse.Delta.Type == "input_json_delta" {
			choice := chatCompletionChoice{
				Index: index,
				Delta: &chatMessage{
					ToolCalls: []toolCall{
						{
							Index: index,
							Function: functionCall{
								Arguments: origResponse.Delta.PartialJson,
							},
						},
					},
				},
			}
			return c.createChatCompletionResponse(ctx, origResponse, choice)
		}
		// Handle thinking_delta
		if origResponse.Delta != nil && origResponse.Delta.Type == "thinking_delta" {
			choice := chatCompletionChoice{
				Index: index,
				Delta: &chatMessage{Reasoning: origResponse.Delta.Thinking},
			}
			return c.createChatCompletionResponse(ctx, origResponse, choice)
		}
		// Handle text_delta
		choice := chatCompletionChoice{
			Index: index,
			Delta: &chatMessage{Content: origResponse.Delta.Text},
		}
		return c.createChatCompletionResponse(ctx, origResponse, choice)

	case "message_delta":
		if origResponse.Usage != nil {
			c.usage.CompletionTokens += origResponse.Usage.OutputTokens
			c.usage.TotalTokens = c.usage.PromptTokens + c.usage.CompletionTokens
		}

		var index int
		if origResponse.Index != nil {
			index = *origResponse.Index
		}
		choice := chatCompletionChoice{
			Index:        index,
			Delta:        &chatMessage{},
			FinishReason: util.Ptr(stopReasonClaude2OpenAI(origResponse.Delta.StopReason)),
		}
		return c.createChatCompletionResponse(ctx, origResponse, choice)
	case "message_stop":
		return &chatCompletionResponse{
			Id:          c.messageId,
			Created:     time.Now().UnixMilli() / 1000,
			Model:       ctx.GetStringContext(ctxKeyFinalRequestModel, ""),
			Object:      objectChatCompletionChunk,
			Choices:     []chatCompletionChoice{},
			ServiceTier: c.serviceTier,
			Usage: &usage{
				PromptTokens:     c.usage.PromptTokens,
				CompletionTokens: c.usage.CompletionTokens,
				TotalTokens:      c.usage.TotalTokens,
			},
		}
	case "content_block_stop", "ping":
		log.Debugf("skip processing response type: %s", origResponse.Type)
		return nil
	default:
		log.Errorf("Unexpected response type: %s", origResponse.Type)
		return nil
	}
}

func (c *claudeProvider) createChatCompletionResponse(ctx wrapper.HttpContext, response *claudeTextGenStreamResponse, choice chatCompletionChoice) *chatCompletionResponse {
	return &chatCompletionResponse{
		Id:          c.messageId,
		Created:     time.Now().UnixMilli() / 1000,
		Model:       ctx.GetStringContext(ctxKeyFinalRequestModel, ""),
		Object:      objectChatCompletionChunk,
		Choices:     []chatCompletionChoice{choice},
		ServiceTier: c.serviceTier,
	}
}

func (c *claudeProvider) appendResponse(responseBuilder *strings.Builder, responseBody string) {
	responseBuilder.WriteString(fmt.Sprintf("%s %s\n\n", streamDataItemKey, responseBody))
}

func (c *claudeProvider) insertHttpContextMessage(body []byte, content string, onlyOneSystemBeforeFile bool) ([]byte, error) {
	request := &claudeTextGenRequest{}
	if err := json.Unmarshal(body, request); err != nil {
		return nil, fmt.Errorf("unable to unmarshal request: %v", err)
	}

	systemStr := request.System.String()
	if systemStr == "" {
		request.System = &claudeSystemPrompt{
			StringValue: content,
			IsArray:     false,
		}
	} else {
		request.System = &claudeSystemPrompt{
			StringValue: content + "\n" + systemStr,
			IsArray:     false,
		}
	}

	return json.Marshal(request)
}

func (c *claudeProvider) GetApiName(path string) ApiName {
	if strings.Contains(path, PathAnthropicMessages) {
		return ApiNameChatCompletion
	}
	if strings.Contains(path, PathAnthropicComplete) {
		return ApiNameCompletion
	}
	if strings.Contains(path, PathOpenAIModels) {
		return ApiNameModels
	}
	if strings.Contains(path, PathOpenAIEmbeddings) {
		return ApiNameEmbeddings
	}
	return ""
}
