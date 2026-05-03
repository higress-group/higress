package provider

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

// ClaudeToOpenAIConverter converts Claude protocol requests to OpenAI protocol
type ClaudeToOpenAIConverter struct {
	// State tracking for streaming conversion
	messageStartSent bool
	messageStopSent  bool
	messageId        string
	// Cache stop_reason until we get usage info
	pendingStopReason *string
	// Content block tracking with dynamic index allocation
	nextContentIndex     int
	thinkingBlockIndex   int
	thinkingBlockStarted bool
	thinkingBlockStopped bool
	textBlockIndex       int
	textBlockStarted     bool
	textBlockStopped     bool
	toolBlockIndex       int
	toolBlockStarted     bool
	toolBlockStopped     bool
	// Tool call state tracking
	toolCallStates  map[int]*toolCallInfo // Map of OpenAI index to tool call state
	activeToolIndex *int                  // Currently active tool call index (for Claude serialization)
}

// toolCallInfo tracks tool call state
type toolCallInfo struct {
	id                  string // Tool call ID
	name                string // Tool call name
	claudeContentIndex  int    // Claude content block index
	contentBlockStarted bool   // Whether content_block_start has been sent
	contentBlockStopped bool   // Whether content_block_stop has been sent
	cachedArguments     string // Cache arguments for this tool call
}

// contentConversionResult represents the result of converting Claude content to OpenAI format
type contentConversionResult struct {
	textParts          []string
	reasoningParts     []string
	toolCalls          []toolCall
	toolResults        []claudeChatMessageContent
	openaiContents     []chatMessageContent
	hasReasoningBlocks bool
}

type ClaudeToOpenAIConvertOptions struct {
	// PreserveMessageReasoningContent enables the non-standard message-level
	// reasoning_content field for providers that explicitly support it.
	PreserveMessageReasoningContent bool
}

func (r *contentConversionResult) reasoningContent() string {
	return strings.Join(r.reasoningParts, "\n\n")
}

func (r *contentConversionResult) setReasoningContent(message *chatMessage, options ClaudeToOpenAIConvertOptions) {
	if options.PreserveMessageReasoningContent && len(r.reasoningParts) > 0 {
		message.ReasoningContent = r.reasoningContent()
	}
}

// ConvertClaudeRequestToOpenAI converts a Claude chat completion request to strict OpenAI format.
// Use ConvertClaudeRequestToOpenAIWithOptions for providers that support non-standard message reasoning fields.
func (c *ClaudeToOpenAIConverter) ConvertClaudeRequestToOpenAI(body []byte) ([]byte, error) {
	return c.ConvertClaudeRequestToOpenAIWithOptions(body, ClaudeToOpenAIConvertOptions{
		PreserveMessageReasoningContent: false,
	})
}

// ConvertClaudeRequestToOpenAIWithOptions converts a Claude chat completion request to OpenAI format.
func (c *ClaudeToOpenAIConverter) ConvertClaudeRequestToOpenAIWithOptions(body []byte, options ClaudeToOpenAIConvertOptions) ([]byte, error) {
	log.Debugf("[Claude->OpenAI] Original Claude request body: %s", string(body))

	var claudeRequest claudeTextGenRequest
	if err := json.Unmarshal(body, &claudeRequest); err != nil {
		return nil, fmt.Errorf("unable to unmarshal claude request: %v", err)
	}

	// Convert Claude request to OpenAI format
	openaiRequest := chatCompletionRequest{
		Model:       claudeRequest.Model,
		Stream:      claudeRequest.Stream,
		Temperature: claudeRequest.Temperature,
		TopP:        claudeRequest.TopP,
		MaxTokens:   claudeRequest.MaxTokens,
		Stop:        claudeRequest.StopSequences,
	}

	if openaiRequest.Stream {
		openaiRequest.StreamOptions = &streamOptions{
			IncludeUsage: true,
		}
	}

	// Convert messages from Claude format to OpenAI format
	for _, claudeMsg := range claudeRequest.Messages {
		// Handle different content types using the type-safe wrapper
		if claudeMsg.Content.IsString {
			// Simple text content
			openaiMsg := chatMessage{
				Role:    claudeMsg.Role,
				Content: claudeMsg.Content.GetStringValue(),
			}
			openaiRequest.Messages = append(openaiRequest.Messages, openaiMsg)
		} else {
			// Multi-modal content - process with convertContentArray
			conversionResult := c.convertContentArray(claudeMsg.Content.GetArrayValue())

			// Handle tool calls if present
			if len(conversionResult.toolCalls) > 0 {
				// Use tool_calls format (current OpenAI standard)
				openaiMsg := chatMessage{
					Role:      claudeMsg.Role,
					ToolCalls: conversionResult.toolCalls,
				}
				conversionResult.setReasoningContent(&openaiMsg, options)

				// Add text content if present, otherwise set to null
				if len(conversionResult.textParts) > 0 {
					openaiMsg.Content = strings.Join(conversionResult.textParts, "\n\n")
				} else {
					openaiMsg.Content = nil
				}

				openaiRequest.Messages = append(openaiRequest.Messages, openaiMsg)
			}

			// Handle tool results if present
			if len(conversionResult.toolResults) > 0 {
				for _, toolResult := range conversionResult.toolResults {
					toolMsg := chatMessage{
						Role:       "tool",
						Content:    toolResult.Content.GetStringValue(),
						ToolCallId: toolResult.ToolUseId,
					}
					openaiRequest.Messages = append(openaiRequest.Messages, toolMsg)
				}
				// Also add visible text content if present alongside tool results.
				// This companion message intentionally does not carry reasoning_content:
				// tool_result content is user/tool-side data, while thinking belongs to assistant turns.
				if len(conversionResult.textParts) > 0 {
					textMsg := chatMessage{
						Role:    claudeMsg.Role,
						Content: strings.Join(conversionResult.textParts, "\n\n"),
					}
					openaiRequest.Messages = append(openaiRequest.Messages, textMsg)
				}
			}

			// Handle regular content if no tool calls or tool results
			if len(conversionResult.toolCalls) == 0 && len(conversionResult.toolResults) == 0 {
				var content any
				if len(conversionResult.openaiContents) > 0 {
					content = conversionResult.openaiContents
				}
				openaiMsg := chatMessage{
					Role:    claudeMsg.Role,
					Content: content,
				}
				conversionResult.setReasoningContent(&openaiMsg, options)
				if openaiMsg.Content == nil && openaiMsg.ReasoningContent == "" && conversionResult.hasReasoningBlocks {
					// Strict OpenAI-style providers reject role-only messages. When Claude turns
					// contain only non-portable reasoning blocks, degrade them to an empty visible
					// message instead of emitting an invalid assistant/user turn.
					openaiMsg.Content = ""
				}
				openaiRequest.Messages = append(openaiRequest.Messages, openaiMsg)
			}
		}
	}

	// Handle system message - Claude has separate system field
	if claudeRequest.System != nil {
		systemMsg := chatMessage{Role: roleSystem}
		if !claudeRequest.System.IsArray {
			// Strip dynamic cch field from billing header to enable caching
			systemMsg.Content = stripCchFromBillingHeader(claudeRequest.System.StringValue)
		} else {
			conversionResult := c.convertContentArray(claudeRequest.System.ArrayValue)
			systemMsg.Content = conversionResult.openaiContents
		}
		// Insert system message at the beginning
		openaiRequest.Messages = append([]chatMessage{systemMsg}, openaiRequest.Messages...)
	}

	// Convert tools if present
	for _, claudeTool := range claudeRequest.Tools {
		openaiTool := tool{
			Type: "function",
			Function: function{
				Name:        claudeTool.Name,
				Description: claudeTool.Description,
				Parameters:  claudeTool.InputSchema,
			},
		}
		openaiRequest.Tools = append(openaiRequest.Tools, openaiTool)
	}

	// Convert tool choice if present
	if claudeRequest.ToolChoice != nil {
		if claudeRequest.ToolChoice.Type == "tool" && claudeRequest.ToolChoice.Name != "" {
			openaiRequest.ToolChoice = &toolChoice{
				Type: "function",
				Function: function{
					Name: claudeRequest.ToolChoice.Name,
				},
			}
		} else {
			// For other types like "auto", "none", etc.
			openaiRequest.ToolChoice = claudeRequest.ToolChoice.Type
		}

		// Handle parallel tool calls
		openaiRequest.ParallelToolCalls = !claudeRequest.ToolChoice.DisableParallelToolUse
	}

	// Convert thinking configuration if present
	// Only set standard OpenAI fields (reasoning_effort). Non-standard fields like
	// "thinking" and "reasoning_max_tokens" are NOT set here because they are not
	// recognized by OpenAI/Azure and will cause 400 errors. Providers that need
	// these non-standard fields (e.g., ZhipuAI) should handle them in their own
	// OnRequestBody implementation.
	if claudeRequest.Thinking != nil {
		log.Debugf("[Claude->OpenAI] Found thinking config: type=%s, budget_tokens=%d",
			claudeRequest.Thinking.Type, claudeRequest.Thinking.BudgetTokens)

		if claudeRequest.Thinking.Type == "enabled" {
			// Set ReasoningEffort based on budget_tokens
			// low: <4096, medium: >=4096 and <16384, high: >=16384
			if claudeRequest.Thinking.BudgetTokens < 4096 {
				openaiRequest.ReasoningEffort = "low"
			} else if claudeRequest.Thinking.BudgetTokens < 16384 {
				openaiRequest.ReasoningEffort = "medium"
			} else {
				openaiRequest.ReasoningEffort = "high"
			}

			log.Debugf("[Claude->OpenAI] Converted thinking config: budget_tokens=%d, reasoning_effort=%s",
				claudeRequest.Thinking.BudgetTokens, openaiRequest.ReasoningEffort)
		}
	}

	result, err := json.Marshal(openaiRequest)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal openai request: %v", err)
	}

	log.Debugf("[Claude->OpenAI] Converted OpenAI request body: %s", string(result))
	return result, nil
}

// ConvertOpenAIResponseToClaude converts an OpenAI response back to Claude format
func (c *ClaudeToOpenAIConverter) ConvertOpenAIResponseToClaude(ctx wrapper.HttpContext, body []byte) ([]byte, error) {
	log.Debugf("[OpenAI->Claude] Original OpenAI response body: %s", string(body))

	var openaiResponse chatCompletionResponse
	if err := json.Unmarshal(body, &openaiResponse); err != nil {
		return nil, fmt.Errorf("unable to unmarshal openai response: %v", err)
	}

	// Convert OpenAI response to Claude format
	claudeResponse := claudeTextGenResponse{
		Id:    openaiResponse.Id,
		Type:  "message",
		Role:  "assistant",
		Model: openaiResponse.Model,
	}

	// Only include usage if it's available
	if openaiResponse.Usage != nil {
		claudeResponse.Usage = claudeTextGenUsage{
			InputTokens:  openaiResponse.Usage.PromptTokens,
			OutputTokens: openaiResponse.Usage.CompletionTokens,
		}
		if openaiResponse.Usage.PromptTokensDetails != nil {
			claudeResponse.Usage.CacheReadInputTokens = openaiResponse.Usage.PromptTokensDetails.CachedTokens
		}
	}

	// Convert the first choice content
	if len(openaiResponse.Choices) > 0 {
		choice := openaiResponse.Choices[0]
		if choice.Message != nil {
			var contents []claudeTextGenContent

			// Add reasoning content (thinking) if present - check both reasoning and reasoning_content fields
			var reasoningText string
			if choice.Message.Reasoning != "" {
				reasoningText = choice.Message.Reasoning
			} else if choice.Message.ReasoningContent != "" {
				reasoningText = choice.Message.ReasoningContent
			}

			if reasoningText != "" {
				emptySignature := ""
				contents = append(contents, claudeTextGenContent{
					Type:      "thinking",
					Signature: &emptySignature, // Use pointer for empty string
					Thinking:  &reasoningText,
				})
				log.Debugf("[OpenAI->Claude] Added thinking content: %s", reasoningText)
			}

			// Add text content if present
			if choice.Message.StringContent() != "" {
				textContent := choice.Message.StringContent()
				contents = append(contents, claudeTextGenContent{
					Type: "text",
					Text: &textContent,
				})
			}

			// Add tool calls if present
			if len(choice.Message.ToolCalls) > 0 {
				for _, toolCall := range choice.Message.ToolCalls {
					if !toolCall.Function.IsEmpty() {
						// Parse arguments from JSON string to map
						var input map[string]interface{}
						if toolCall.Function.Arguments != "" {
							if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &input); err != nil {
								log.Errorf("Failed to parse tool call arguments: %v, arguments: %s", err, toolCall.Function.Arguments)
								input = map[string]interface{}{}
							}
						} else {
							input = map[string]interface{}{}
						}

						contents = append(contents, claudeTextGenContent{
							Type:  "tool_use",
							Id:    toolCall.Id,
							Name:  toolCall.Function.Name,
							Input: &input,
						})
					}
				}
			}

			claudeResponse.Content = contents
		}

		// Convert finish reason
		if choice.FinishReason != nil {
			claudeFinishReason := openAIFinishReasonToClaude(*choice.FinishReason)
			claudeResponse.StopReason = &claudeFinishReason
		}
	}

	result, err := json.Marshal(claudeResponse)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal claude response: %v", err)
	}

	log.Debugf("[OpenAI->Claude] Converted Claude response body: %s", string(result))
	return result, nil
}

// ConvertOpenAIStreamResponseToClaude converts OpenAI streaming response to Claude format
func (c *ClaudeToOpenAIConverter) ConvertOpenAIStreamResponseToClaude(ctx wrapper.HttpContext, chunk []byte) ([]byte, error) {
	log.Debugf("[OpenAI->Claude] Original OpenAI streaming chunk: %s", string(chunk))

	// For streaming responses, we need to handle the Server-Sent Events format
	lines := strings.Split(string(chunk), "\n")
	var result strings.Builder

	for _, line := range lines {
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")

			// Handle [DONE] messages
			if data == "[DONE]" {
				log.Debugf("[OpenAI->Claude] Processing [DONE] message, finalizing stream")

				// Send final content_block_stop events for any active blocks
				if c.thinkingBlockStarted && !c.thinkingBlockStopped {
					c.thinkingBlockStopped = true
					log.Debugf("[OpenAI->Claude] Sending final thinking content_block_stop event at index %d", c.thinkingBlockIndex)
					stopEvent := &claudeTextGenStreamResponse{
						Type:  "content_block_stop",
						Index: &c.thinkingBlockIndex,
					}
					stopData, _ := json.Marshal(stopEvent)
					result.WriteString(fmt.Sprintf("event: %s\ndata: %s\n\n", stopEvent.Type, stopData))
				}
				if c.textBlockStarted && !c.textBlockStopped {
					c.textBlockStopped = true
					log.Debugf("[OpenAI->Claude] Sending final text content_block_stop event at index %d", c.textBlockIndex)
					stopEvent := &claudeTextGenStreamResponse{
						Type:  "content_block_stop",
						Index: &c.textBlockIndex,
					}
					stopData, _ := json.Marshal(stopEvent)
					result.WriteString(fmt.Sprintf("event: %s\ndata: %s\n\n", stopEvent.Type, stopData))
				}
				// Send final content_block_stop events for any remaining unclosed tool calls
				for index, toolCall := range c.toolCallStates {
					if toolCall.contentBlockStarted && !toolCall.contentBlockStopped {
						log.Debugf("[OpenAI->Claude] Sending final tool content_block_stop event for index %d at Claude index %d",
							index, toolCall.claudeContentIndex)
						stopEvent := &claudeTextGenStreamResponse{
							Type:  "content_block_stop",
							Index: &toolCall.claudeContentIndex,
						}
						stopData, _ := json.Marshal(stopEvent)
						result.WriteString(fmt.Sprintf("event: %s\ndata: %s\n\n", stopEvent.Type, stopData))
					}
				}

				// If we have a pending stop_reason but no usage, send message_delta with just stop_reason
				if c.pendingStopReason != nil {
					log.Debugf("[OpenAI->Claude] Sending final message_delta with pending stop_reason: %s", *c.pendingStopReason)
					messageDelta := &claudeTextGenStreamResponse{
						Type: "message_delta",
						Delta: &claudeTextGenDelta{
							StopReason:   c.pendingStopReason,
							StopSequence: json.RawMessage("null"),
						},
					}
					stopData, _ := json.Marshal(messageDelta)
					result.WriteString(fmt.Sprintf("event: %s\ndata: %s\n\n", messageDelta.Type, stopData))
					c.pendingStopReason = nil
				}

				if c.messageStartSent && !c.messageStopSent {
					c.messageStopSent = true
					log.Debugf("[OpenAI->Claude] Sending final message_stop event")
					messageStopEvent := &claudeTextGenStreamResponse{
						Type: "message_stop",
					}
					stopData, _ := json.Marshal(messageStopEvent)
					result.WriteString(fmt.Sprintf("event: %s\ndata: %s\n\n", messageStopEvent.Type, stopData))
				}

				// Reset all state for next request
				c.messageStartSent = false
				c.messageStopSent = false
				c.messageId = ""
				c.pendingStopReason = nil
				c.nextContentIndex = 0
				c.thinkingBlockIndex = -1
				c.thinkingBlockStarted = false
				c.thinkingBlockStopped = false
				c.textBlockIndex = -1
				c.textBlockStarted = false
				c.textBlockStopped = false
				c.toolBlockIndex = -1
				c.toolBlockStarted = false
				c.toolBlockStopped = false
				c.toolCallStates = make(map[int]*toolCallInfo)
				c.activeToolIndex = nil
				log.Debugf("[OpenAI->Claude] Reset converter state for next request")

				continue
			}

			// Some providers keep sending duplicate usage chunks after the stream
			// has already been finalized. Ignore those trailing chunks.
			if c.messageStopSent {
				log.Debugf("[OpenAI->Claude] Ignoring chunk after message_stop: %s", data)
				continue
			}

			var openaiStreamResponse chatCompletionResponse
			if err := json.Unmarshal([]byte(data), &openaiStreamResponse); err != nil {
				log.Debugf("unable to unmarshal openai stream response: %v, data: %s", err, data)
				continue
			}

			// Convert to Claude streaming format
			claudeStreamResponses := c.buildClaudeStreamResponse(ctx, &openaiStreamResponse)
			log.Debugf("[OpenAI->Claude] Generated %d Claude stream events from OpenAI chunk", len(claudeStreamResponses))

			for i, claudeStreamResponse := range claudeStreamResponses {
				responseData, err := json.Marshal(claudeStreamResponse)
				if err != nil {
					log.Errorf("unable to marshal claude stream response: %v", err)
					continue
				}
				log.Debugf("[OpenAI->Claude] Stream event [%d/%d]: %s", i+1, len(claudeStreamResponses), string(responseData))
				result.WriteString(fmt.Sprintf("event: %s\ndata: %s\n\n", claudeStreamResponse.Type, responseData))
			}
		}
	}

	claudeChunk := []byte(result.String())
	log.Debugf("[OpenAI->Claude] Converted Claude streaming chunk: %s", string(claudeChunk))
	return claudeChunk, nil
}

func normalizeFinishReason(finishReason *string) (string, bool) {
	if finishReason == nil {
		return "", false
	}

	normalized := strings.TrimSpace(*finishReason)
	if normalized == "" || strings.EqualFold(normalized, "null") {
		return "", false
	}

	return normalized, true
}

// buildClaudeStreamResponse builds Claude streaming responses from OpenAI streaming response
func (c *ClaudeToOpenAIConverter) buildClaudeStreamResponse(ctx wrapper.HttpContext, openaiResponse *chatCompletionResponse) []*claudeTextGenStreamResponse {
	var choice chatCompletionChoice
	if len(openaiResponse.Choices) == 0 {
		choice = chatCompletionChoice{
			Index: 0,
			Delta: &chatMessage{
				Content: "",
			},
		}
	} else {
		choice = openaiResponse.Choices[0]
	}
	var responses []*claudeTextGenStreamResponse

	// Log what we're processing
	hasRole := choice.Delta != nil && choice.Delta.Role != ""
	hasContent := choice.Delta != nil && choice.Delta.Content != ""
	finishReason, hasFinishReason := normalizeFinishReason(choice.FinishReason)
	hasUsage := openaiResponse.Usage != nil

	log.Debugf("[OpenAI->Claude] Processing OpenAI chunk - Role: %v, Content: %v, FinishReason: %v, Usage: %v",
		hasRole, hasContent, hasFinishReason, hasUsage)

	// Handle message start (only once)
	// Note: OpenRouter may send multiple messages with role but empty content at the start
	// We only send message_start for the first one
	if choice.Delta != nil && choice.Delta.Role != "" && !c.messageStartSent {
		c.messageId = openaiResponse.Id
		c.messageStartSent = true

		message := &claudeTextGenResponse{
			Id:      openaiResponse.Id,
			Type:    "message",
			Role:    "assistant",
			Model:   openaiResponse.Model,
			Content: []claudeTextGenContent{},
		}

		// Only include usage if it's available
		if openaiResponse.Usage != nil {
			message.Usage = claudeTextGenUsage{
				InputTokens:  openaiResponse.Usage.PromptTokens,
				OutputTokens: 0,
			}
		}

		responses = append(responses, &claudeTextGenStreamResponse{
			Type:    "message_start",
			Message: message,
		})

		log.Debugf("[OpenAI->Claude] Generated message_start event for id: %s", openaiResponse.Id)
	} else if choice.Delta != nil && choice.Delta.Role != "" && c.messageStartSent {
		// Skip duplicate role messages from OpenRouter
		log.Debugf("[OpenAI->Claude] Skipping duplicate role message for id: %s", openaiResponse.Id)
	}

	// Handle reasoning content (thinking) first - check both reasoning and reasoning_content fields
	var reasoningText string
	if choice.Delta != nil {
		if choice.Delta.Reasoning != "" {
			reasoningText = choice.Delta.Reasoning
		} else if choice.Delta.ReasoningContent != "" {
			reasoningText = choice.Delta.ReasoningContent
		}
	}

	if reasoningText != "" {
		log.Debugf("[OpenAI->Claude] Processing reasoning content delta: %s", reasoningText)

		// Send content_block_start for thinking only once with dynamic index
		if !c.thinkingBlockStarted {
			c.thinkingBlockIndex = c.nextContentIndex
			c.nextContentIndex++
			c.thinkingBlockStarted = true
			log.Debugf("[OpenAI->Claude] Generated content_block_start event for thinking at index %d", c.thinkingBlockIndex)
			emptyStr := ""
			responses = append(responses, &claudeTextGenStreamResponse{
				Type:  "content_block_start",
				Index: &c.thinkingBlockIndex,
				ContentBlock: &claudeTextGenContent{
					Type:      "thinking",
					Signature: &emptyStr, // Use pointer for empty string output
					Thinking:  &emptyStr, // Use pointer for empty string output
				},
			})
		}

		// Send content_block_delta for thinking
		log.Debugf("[OpenAI->Claude] Generated content_block_delta event with thinking: %s", reasoningText)
		responses = append(responses, &claudeTextGenStreamResponse{
			Type:  "content_block_delta",
			Index: &c.thinkingBlockIndex,
			Delta: &claudeTextGenDelta{
				Type:     "thinking_delta",
				Thinking: reasoningText, // Use Thinking field, not Text
			},
		})
	}

	// Handle content
	if choice.Delta != nil && choice.Delta.Content != nil && choice.Delta.Content != "" {
		deltaContent, ok := choice.Delta.Content.(string)
		if !ok {
			log.Debugf("[OpenAI->Claude] Content is not a string: %T", choice.Delta.Content)
			return responses
		}

		log.Debugf("[OpenAI->Claude] Processing content delta: %s", deltaContent)

		// Close thinking content block if it's still open
		if c.thinkingBlockStarted && !c.thinkingBlockStopped {
			c.thinkingBlockStopped = true
			log.Debugf("[OpenAI->Claude] Closing thinking content block before text")
			responses = append(responses, &claudeTextGenStreamResponse{
				Type:  "content_block_stop",
				Index: &c.thinkingBlockIndex,
			})
		}

		// Send content_block_start only once for text content with dynamic index
		if !c.textBlockStarted {
			c.textBlockIndex = c.nextContentIndex
			c.nextContentIndex++
			c.textBlockStarted = true
			log.Debugf("[OpenAI->Claude] Generated content_block_start event for text at index %d", c.textBlockIndex)
			emptyText := ""
			responses = append(responses, &claudeTextGenStreamResponse{
				Type:  "content_block_start",
				Index: &c.textBlockIndex,
				ContentBlock: &claudeTextGenContent{
					Type: "text",
					Text: &emptyText,
				},
			})
		}

		// Send content_block_delta
		log.Debugf("[OpenAI->Claude] Generated content_block_delta event with text: %s", deltaContent)
		responses = append(responses, &claudeTextGenStreamResponse{
			Type:  "content_block_delta",
			Index: &c.textBlockIndex,
			Delta: &claudeTextGenDelta{
				Type: "text_delta",
				Text: deltaContent,
			},
		})
	}

	// Handle tool calls in streaming response
	if choice.Delta != nil && len(choice.Delta.ToolCalls) > 0 {
		// Ensure message_start is sent before any content blocks
		if !c.messageStartSent {
			c.messageId = openaiResponse.Id
			c.messageStartSent = true
			message := &claudeTextGenResponse{
				Id:      openaiResponse.Id,
				Type:    "message",
				Role:    "assistant",
				Model:   openaiResponse.Model,
				Content: []claudeTextGenContent{},
			}
			if openaiResponse.Usage != nil {
				message.Usage = claudeTextGenUsage{
					InputTokens:  openaiResponse.Usage.PromptTokens,
					OutputTokens: 0,
				}
			}
			responses = append(responses, &claudeTextGenStreamResponse{
				Type:    "message_start",
				Message: message,
			})
			log.Debugf("[OpenAI->Claude] Generated message_start event before tool calls for id: %s", openaiResponse.Id)
		}

		// Initialize toolCallStates if needed
		if c.toolCallStates == nil {
			c.toolCallStates = make(map[int]*toolCallInfo)
		}

		for _, toolCall := range choice.Delta.ToolCalls {
			log.Debugf("[OpenAI->Claude] Processing tool call delta: index=%d, id=%s, name=%s, args=%s",
				toolCall.Index, toolCall.Id, toolCall.Function.Name, toolCall.Function.Arguments)

			// Handle new tool call (has id and name)
			if toolCall.Id != "" && toolCall.Function.Name != "" {
				// Create or update tool call state
				if _, exists := c.toolCallStates[toolCall.Index]; !exists {
					c.toolCallStates[toolCall.Index] = &toolCallInfo{
						id:                  toolCall.Id,
						name:                toolCall.Function.Name,
						contentBlockStarted: false,
						contentBlockStopped: false,
						cachedArguments:     "",
					}
				}

				toolState := c.toolCallStates[toolCall.Index]

				// Check if we can start this tool call (Claude requires serialization)
				if c.activeToolIndex == nil {
					// No active tool call, start this one
					c.activeToolIndex = &toolCall.Index
					toolCallResponses := c.startToolCall(toolState)
					responses = append(responses, toolCallResponses...)
				}
				// If there's already an active tool call, we'll start this one when the current one finishes
			}

			// Handle arguments for any tool call - cache all arguments regardless of active state
			if toolCall.Function.Arguments != "" {
				if toolState, exists := c.toolCallStates[toolCall.Index]; exists {
					// Always cache arguments for this tool call
					toolState.cachedArguments += toolCall.Function.Arguments
					log.Debugf("[OpenAI->Claude] Cached arguments for tool index %d: %s (total: %s)",
						toolCall.Index, toolCall.Function.Arguments, toolState.cachedArguments)

					// Send input_json_delta event only if this tool is currently active and content block started
					if c.activeToolIndex != nil && *c.activeToolIndex == toolCall.Index && toolState.contentBlockStarted {
						log.Debugf("[OpenAI->Claude] Generated input_json_delta event for active tool index %d: %s",
							toolCall.Index, toolCall.Function.Arguments)
						responses = append(responses, &claudeTextGenStreamResponse{
							Type:  "content_block_delta",
							Index: &toolState.claudeContentIndex,
							Delta: &claudeTextGenDelta{
								Type:        "input_json_delta",
								PartialJson: toolCall.Function.Arguments,
							},
						})
					}
				}
			}
		}
	}

	// Handle finish reason
	if hasFinishReason {
		claudeFinishReason := openAIFinishReasonToClaude(finishReason)
		log.Debugf("[OpenAI->Claude] Processing finish_reason: %s -> %s", finishReason, claudeFinishReason)

		// Send content_block_stop for any active content blocks
		if c.thinkingBlockStarted && !c.thinkingBlockStopped {
			c.thinkingBlockStopped = true
			log.Debugf("[OpenAI->Claude] Generated thinking content_block_stop event at index %d", c.thinkingBlockIndex)
			responses = append(responses, &claudeTextGenStreamResponse{
				Type:  "content_block_stop",
				Index: &c.thinkingBlockIndex,
			})
		}
		if c.textBlockStarted && !c.textBlockStopped {
			c.textBlockStopped = true
			log.Debugf("[OpenAI->Claude] Generated text content_block_stop event at index %d", c.textBlockIndex)
			responses = append(responses, &claudeTextGenStreamResponse{
				Type:  "content_block_stop",
				Index: &c.textBlockIndex,
			})
		}

		// First, start any remaining unstarted tool calls (they may have no arguments)
		// Process in order to maintain Claude's sequential requirement
		var sortedIndices []int
		for index := range c.toolCallStates {
			sortedIndices = append(sortedIndices, index)
		}

		// Sort indices to process in order
		for i := 0; i < len(sortedIndices)-1; i++ {
			for j := i + 1; j < len(sortedIndices); j++ {
				if sortedIndices[i] > sortedIndices[j] {
					sortedIndices[i], sortedIndices[j] = sortedIndices[j], sortedIndices[i]
				}
			}
		}

		for _, index := range sortedIndices {
			toolCall := c.toolCallStates[index]
			if !toolCall.contentBlockStarted {
				log.Debugf("[OpenAI->Claude] Starting remaining tool call at finish: index=%d, id=%s, name=%s",
					index, toolCall.id, toolCall.name)
				c.activeToolIndex = &index
				toolCallResponses := c.startToolCall(toolCall)
				responses = append(responses, toolCallResponses...)
				c.activeToolIndex = nil // Clear immediately since tool is now fully started
			}
		}

		// Then send content_block_stop for all started tool calls in order
		for _, index := range sortedIndices {
			toolCall := c.toolCallStates[index]
			if toolCall.contentBlockStarted && !toolCall.contentBlockStopped {
				log.Debugf("[OpenAI->Claude] Generated content_block_stop for tool at index %d, Claude index %d",
					index, toolCall.claudeContentIndex)
				responses = append(responses, &claudeTextGenStreamResponse{
					Type:  "content_block_stop",
					Index: &toolCall.claudeContentIndex,
				})
				toolCall.contentBlockStopped = true
			}
		}

		// Clear active tool index
		c.activeToolIndex = nil

		// Cache stop_reason until we get usage info (Claude protocol requires them together)
		c.pendingStopReason = &claudeFinishReason
		log.Debugf("[OpenAI->Claude] Cached stop_reason: %s, waiting for usage", claudeFinishReason)
	}

	// Handle usage information
	// Note: Some providers may send usage in the same chunk as finish_reason,
	// so we check for usage regardless of whether finish_reason is present
	if openaiResponse.Usage != nil {
		stopReasonIncluded := false
		log.Debugf("[OpenAI->Claude] Processing usage info - input: %d, output: %d",
			openaiResponse.Usage.PromptTokens, openaiResponse.Usage.CompletionTokens)

		// Send message_delta with both stop_reason and usage (Claude protocol requirement)
		messageDelta := &claudeTextGenStreamResponse{
			Type: "message_delta",
			Delta: &claudeTextGenDelta{
				StopSequence: json.RawMessage("null"), // Explicit null per Claude spec
			},
			Usage: &claudeTextGenUsage{
				InputTokens:  openaiResponse.Usage.PromptTokens,
				OutputTokens: openaiResponse.Usage.CompletionTokens,
			},
		}

		// Include cached stop_reason if available
		if c.pendingStopReason != nil {
			log.Debugf("[OpenAI->Claude] Combining cached stop_reason %s with usage", *c.pendingStopReason)
			messageDelta.Delta.StopReason = c.pendingStopReason
			c.pendingStopReason = nil // Clear cache
			stopReasonIncluded = true
		}

		log.Debugf("[OpenAI->Claude] Generated message_delta event with usage and stop_reason")
		responses = append(responses, messageDelta)

		// Send message_stop only when this usage chunk carries a real stop_reason.
		// Some providers report incremental usage in every chunk before finishing.
		if stopReasonIncluded && !c.messageStopSent {
			c.messageStopSent = true
			log.Debugf("[OpenAI->Claude] Generated message_stop event")
			responses = append(responses, &claudeTextGenStreamResponse{
				Type: "message_stop",
			})
		}
	}

	return responses
}

// openAIFinishReasonToClaude converts OpenAI finish reason to Claude format
func openAIFinishReasonToClaude(reason string) string {
	switch reason {
	case finishReasonStop:
		return "end_turn"
	case finishReasonLength:
		return "max_tokens"
	case finishReasonToolCall:
		return "tool_use"
	default:
		return reason
	}
}

// convertContentArray converts an array of Claude content to OpenAI content format
func (c *ClaudeToOpenAIConverter) convertContentArray(claudeContents []claudeChatMessageContent) *contentConversionResult {
	result := &contentConversionResult{
		textParts:          []string{},
		reasoningParts:     []string{},
		toolCalls:          []toolCall{},
		toolResults:        []claudeChatMessageContent{},
		openaiContents:     []chatMessageContent{},
		hasReasoningBlocks: false,
	}

	for _, claudeContent := range claudeContents {
		switch claudeContent.Type {
		case "text":
			if claudeContent.Text != "" {
				// Strip dynamic cch field from billing header to enable caching
				processedText := stripCchFromBillingHeader(claudeContent.Text)
				result.textParts = append(result.textParts, processedText)
				result.openaiContents = append(result.openaiContents, chatMessageContent{
					Type:         contentTypeText,
					Text:         processedText,
					CacheControl: claudeContent.CacheControl,
				})
			}
		case "thinking":
			result.hasReasoningBlocks = true
			if claudeContent.Thinking != "" {
				result.reasoningParts = append(result.reasoningParts, claudeContent.Thinking)
			}
		case "redacted_thinking":
			result.hasReasoningBlocks = true
			// data is an opaque Claude blob, not portable reasoning text.
		case "image":
			if claudeContent.Source != nil {
				if claudeContent.Source.Type == "base64" {
					// Convert base64 image to OpenAI format
					dataUrl := fmt.Sprintf("data:%s;base64,%s", claudeContent.Source.MediaType, claudeContent.Source.Data)
					result.openaiContents = append(result.openaiContents, chatMessageContent{
						Type: contentTypeImageUrl,
						ImageUrl: &chatMessageContentImageUrl{
							Url: dataUrl,
						},
					})
				} else if claudeContent.Source.Type == "url" {
					result.openaiContents = append(result.openaiContents, chatMessageContent{
						Type: contentTypeImageUrl,
						ImageUrl: &chatMessageContentImageUrl{
							Url: claudeContent.Source.Url,
						},
					})
				}
			}
		case "tool_use":
			// Convert Claude tool_use to OpenAI tool_calls format
			if claudeContent.Id != "" && claudeContent.Name != "" {
				// Convert input to JSON string for OpenAI format
				var argumentsStr string
				if claudeContent.Input != nil {
					if argBytes, err := json.Marshal(claudeContent.Input); err == nil {
						argumentsStr = string(argBytes)
					}
				}

				toolCall := toolCall{
					Id:   claudeContent.Id,
					Type: "function",
					Function: functionCall{
						Name:      claudeContent.Name,
						Arguments: argumentsStr,
					},
				}
				result.toolCalls = append(result.toolCalls, toolCall)
				log.Debugf("[Claude->OpenAI] Converted tool_use to tool_call: %s", claudeContent.Name)
			}
		case "tool_result":
			// Store tool results for processing
			result.toolResults = append(result.toolResults, claudeContent)
			log.Debugf("[Claude->OpenAI] Found tool_result for tool_use_id: %s", claudeContent.ToolUseId)
		}
	}

	return result
}

// startToolCall starts a new tool call content block
func (c *ClaudeToOpenAIConverter) startToolCall(toolState *toolCallInfo) []*claudeTextGenStreamResponse {
	var responses []*claudeTextGenStreamResponse

	// Close thinking content block if it's still open
	if c.thinkingBlockStarted && !c.thinkingBlockStopped {
		c.thinkingBlockStopped = true
		log.Debugf("[OpenAI->Claude] Closing thinking content block before tool use")
		responses = append(responses, &claudeTextGenStreamResponse{
			Type:  "content_block_stop",
			Index: &c.thinkingBlockIndex,
		})
	}

	// Close text content block if it's still open
	if c.textBlockStarted && !c.textBlockStopped {
		c.textBlockStopped = true
		log.Debugf("[OpenAI->Claude] Closing text content block before tool use")
		responses = append(responses, &claudeTextGenStreamResponse{
			Type:  "content_block_stop",
			Index: &c.textBlockIndex,
		})
	}

	// Assign Claude content index
	toolState.claudeContentIndex = c.nextContentIndex
	c.nextContentIndex++
	toolState.contentBlockStarted = true

	log.Debugf("[OpenAI->Claude] Started tool call: Claude index=%d, id=%s, name=%s",
		toolState.claudeContentIndex, toolState.id, toolState.name)

	// Send content_block_start
	emptyInput := map[string]interface{}{}
	responses = append(responses, &claudeTextGenStreamResponse{
		Type:  "content_block_start",
		Index: &toolState.claudeContentIndex,
		ContentBlock: &claudeTextGenContent{
			Type:  "tool_use",
			Id:    toolState.id,
			Name:  toolState.name,
			Input: &emptyInput, // Empty input as per Claude spec
		},
	})

	// Send any cached arguments as input_json_delta events
	if toolState.cachedArguments != "" {
		log.Debugf("[OpenAI->Claude] Outputting cached arguments for tool: %s", toolState.cachedArguments)
		responses = append(responses, &claudeTextGenStreamResponse{
			Type:  "content_block_delta",
			Index: &toolState.claudeContentIndex,
			Delta: &claudeTextGenDelta{
				Type:        "input_json_delta",
				PartialJson: toolState.cachedArguments,
			},
		})
	}

	return responses
}

// stripCchFromBillingHeader removes the dynamic cch field from x-anthropic-billing-header text
// to enable caching. The cch value changes on every request, which would break prompt caching.
// Example input:  "x-anthropic-billing-header: cc_version=2.1.37.3a3; cc_entrypoint=claude-vscode; cch=abc123;"
// Example output: "x-anthropic-billing-header: cc_version=2.1.37.3a3; cc_entrypoint=claude-vscode;"
func stripCchFromBillingHeader(text string) string {
	const billingHeaderPrefix = "x-anthropic-billing-header:"

	// Check if this is a billing header
	if !strings.HasPrefix(text, billingHeaderPrefix) {
		return text
	}

	// Remove cch=xxx pattern (may appear with or without trailing semicolon)
	// Pattern: ; cch=<any-non-semicolon-chars> followed by ; or end of string
	result := text

	// Try to find and remove ; cch=... pattern
	// We need to handle both "; cch=xxx;" and "; cch=xxx" (at end)
	for {
		cchIdx := strings.Index(result, "; cch=")
		if cchIdx == -1 {
			break
		}

		// Find the end of cch value (next semicolon or end of string)
		start := cchIdx + 2 // skip "; "
		end := strings.Index(result[start:], ";")
		if end == -1 {
			// cch is at the end, remove from "; cch=" to end
			result = result[:cchIdx]
		} else {
			// cch is followed by more content, remove "; cch=xxx" part
			result = result[:cchIdx] + result[start+end:]
		}
	}

	return result
}
