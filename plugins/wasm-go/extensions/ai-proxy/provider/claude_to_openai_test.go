package provider

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock logger for testing
type mockLogger struct{}

func (m *mockLogger) Trace(msg string)                             {}
func (m *mockLogger) Tracef(format string, args ...interface{})    {}
func (m *mockLogger) Debug(msg string)                             {}
func (m *mockLogger) Debugf(format string, args ...interface{})    {}
func (m *mockLogger) Info(msg string)                              {}
func (m *mockLogger) Infof(format string, args ...interface{})     {}
func (m *mockLogger) Warn(msg string)                              {}
func (m *mockLogger) Warnf(format string, args ...interface{})     {}
func (m *mockLogger) Error(msg string)                             {}
func (m *mockLogger) Errorf(format string, args ...interface{})    {}
func (m *mockLogger) Critical(msg string)                          {}
func (m *mockLogger) Criticalf(format string, args ...interface{}) {}
func (m *mockLogger) ResetID(pluginID string)                      {}

func init() {
	// Initialize mock logger for testing
	log.SetPluginLog(&mockLogger{})
}

func TestClaudeToOpenAIConverter_ConvertClaudeRequestToOpenAI(t *testing.T) {
	converter := &ClaudeToOpenAIConverter{}

	t.Run("convert_tool_choice_any_to_required", func(t *testing.T) {
		claudeRequest := `{
			"model": "claude-sonnet-4",
			"max_tokens": 1000,
			"messages": [{"role": "user", "content": "Run a search."}],
			"tools": [{
				"name": "web_search",
				"description": "Search the web.",
				"input_schema": {
					"type": "object",
					"properties": {"query": {"type": "string"}},
					"required": ["query"]
				}
			}],
			"tool_choice": {"type": "any"}
		}`

		result, err := converter.ConvertClaudeRequestToOpenAI([]byte(claudeRequest))
		require.NoError(t, err)

		var openaiRequest chatCompletionRequest
		err = json.Unmarshal(result, &openaiRequest)
		require.NoError(t, err)

		require.Equal(t, "required", openaiRequest.ToolChoice)
		require.NotNil(t, openaiRequest.ParallelToolCalls)
		require.True(t, *openaiRequest.ParallelToolCalls)
		require.Contains(t, string(result), `"parallel_tool_calls":true`)
	})

	t.Run("convert_tool_choice_any_preserves_disable_parallel_tool_use", func(t *testing.T) {
		claudeRequest := `{
			"model": "claude-sonnet-4",
			"max_tokens": 1000,
			"messages": [{"role": "user", "content": "Run a search."}],
			"tools": [{
				"name": "web_search",
				"description": "Search the web.",
				"input_schema": {
					"type": "object",
					"properties": {"query": {"type": "string"}},
					"required": ["query"]
				}
			}],
			"tool_choice": {"type": "any", "disable_parallel_tool_use": true}
		}`

		result, err := converter.ConvertClaudeRequestToOpenAI([]byte(claudeRequest))
		require.NoError(t, err)

		var openaiRequest chatCompletionRequest
		err = json.Unmarshal(result, &openaiRequest)
		require.NoError(t, err)

		require.Equal(t, "required", openaiRequest.ToolChoice)
		require.NotNil(t, openaiRequest.ParallelToolCalls)
		require.False(t, *openaiRequest.ParallelToolCalls)
		require.Contains(t, string(result), `"parallel_tool_calls":false`)
	})

	t.Run("convert_multiple_text_content_blocks", func(t *testing.T) {
		// Test case: multiple text content blocks should remain as separate array elements with cache control support
		// Both system and user messages should handle array content format
		claudeRequest := `{
			"max_tokens": 32000,
			"messages": [{
				"content": [{
					"text": "<system-reminder>\nThis is a reminder that your todo list is currently empty. DO NOT mention this to the user explicitly because they are already aware. If you are working on tasks that would benefit from a todo list please use the TodoWrite tool to create one. If not, please feel free to ignore. Again do not mention this message to the user.</system-reminder>",
					"type": "text"
				}, {
					"text": "<system-reminder>\nyyy</system-reminder>",
					"type": "text"
				}, {
					"cache_control": {
						"type": "ephemeral"
					},
					"text": "你是谁",
					"type": "text"
				}],
				"role": "user"
			}],
			"metadata": {
				"user_id": "user_dd3c52c1d698a4486bdef490197846b7c1f7e553202dae5763f330c35aeb9823_account__session_b2e14122-0ac6-4959-9c5d-b49ae01ccb7c"
			},
			"model": "anthropic/claude-sonnet-4",
			"stream": true,
			"system": [{
				"cache_control": {
					"type": "ephemeral"
				},
				"text": "xxx",
				"type": "text"
			}, {
				"cache_control": {
					"type": "ephemeral"
				},
				"text": "yyy",
				"type": "text"
			}],
			"temperature": 1,
			"stream_options": {
				"include_usage": true
			}
		}`

		result, err := converter.ConvertClaudeRequestToOpenAI([]byte(claudeRequest))
		require.NoError(t, err)

		// Parse the result to verify the conversion
		var openaiRequest chatCompletionRequest
		err = json.Unmarshal(result, &openaiRequest)
		require.NoError(t, err)

		// Verify basic fields are converted correctly
		assert.Equal(t, "anthropic/claude-sonnet-4", openaiRequest.Model)
		assert.Equal(t, true, openaiRequest.Stream)
		assert.Equal(t, 1.0, openaiRequest.Temperature)
		assert.Equal(t, 32000, openaiRequest.MaxTokens)

		// Verify messages structure
		require.Len(t, openaiRequest.Messages, 2)

		// First message should be system message (converted from Claude's system field)
		systemMsg := openaiRequest.Messages[0]
		assert.Equal(t, roleSystem, systemMsg.Role)

		// System content should now also be an array for multiple text blocks
		systemContentArray, ok := systemMsg.Content.([]interface{})
		require.True(t, ok, "System content should be an array for multiple text blocks")
		require.Len(t, systemContentArray, 2)

		// First system text block
		firstSystemElement, ok := systemContentArray[0].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, contentTypeText, firstSystemElement["type"])
		assert.Equal(t, "xxx", firstSystemElement["text"])
		assert.NotNil(t, firstSystemElement["cache_control"]) // Has cache control
		systemCacheControl1, ok := firstSystemElement["cache_control"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "ephemeral", systemCacheControl1["type"])

		// Second system text block
		secondSystemElement, ok := systemContentArray[1].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, contentTypeText, secondSystemElement["type"])
		assert.Equal(t, "yyy", secondSystemElement["text"])
		assert.NotNil(t, secondSystemElement["cache_control"]) // Has cache control
		systemCacheControl2, ok := secondSystemElement["cache_control"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "ephemeral", systemCacheControl2["type"])

		// Second message should be user message with text content as array
		userMsg := openaiRequest.Messages[1]
		assert.Equal(t, "user", userMsg.Role)

		// The content should now be an array of separate text blocks, not merged
		contentArray, ok := userMsg.Content.([]interface{})
		require.True(t, ok, "Content should be an array for multiple text blocks")
		require.Len(t, contentArray, 3)

		// First text block
		firstElement, ok := contentArray[0].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, contentTypeText, firstElement["type"])
		assert.Equal(t, "<system-reminder>\nThis is a reminder that your todo list is currently empty. DO NOT mention this to the user explicitly because they are already aware. If you are working on tasks that would benefit from a todo list please use the TodoWrite tool to create one. If not, please feel free to ignore. Again do not mention this message to the user.</system-reminder>", firstElement["text"])
		assert.Nil(t, firstElement["cache_control"]) // No cache control for first block

		// Second text block
		secondElement, ok := contentArray[1].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, contentTypeText, secondElement["type"])
		assert.Equal(t, "<system-reminder>\nyyy</system-reminder>", secondElement["text"])
		assert.Nil(t, secondElement["cache_control"]) // No cache control for second block

		// Third text block with cache control
		thirdElement, ok := contentArray[2].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, contentTypeText, thirdElement["type"])
		assert.Equal(t, "你是谁", thirdElement["text"])
		assert.NotNil(t, thirdElement["cache_control"]) // Has cache control
		cacheControl, ok := thirdElement["cache_control"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "ephemeral", cacheControl["type"])
	})

	t.Run("convert_mixed_content_with_image", func(t *testing.T) {
		// Test case with mixed text and image content (should remain as array)
		claudeRequest := `{
			"model": "claude-3-sonnet-20240229",
			"messages": [{
				"role": "user",
				"content": [{
					"type": "text",
					"text": "What's in this image?"
				}, {
					"type": "image",
					"source": {
						"type": "base64",
						"media_type": "image/jpeg",
						"data": "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
					}
				}]
			}],
			"max_tokens": 1000
		}`

		result, err := converter.ConvertClaudeRequestToOpenAI([]byte(claudeRequest))
		require.NoError(t, err)

		var openaiRequest chatCompletionRequest
		err = json.Unmarshal(result, &openaiRequest)
		require.NoError(t, err)

		// Should have one user message
		require.Len(t, openaiRequest.Messages, 1)
		userMsg := openaiRequest.Messages[0]
		assert.Equal(t, "user", userMsg.Role)

		// Content should be an array (mixed content) - after JSON marshaling/unmarshaling it becomes []interface{}
		contentArray, ok := userMsg.Content.([]interface{})
		require.True(t, ok, "Content should be an array for mixed content")
		require.Len(t, contentArray, 2)

		// First element should be text
		firstElement, ok := contentArray[0].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, contentTypeText, firstElement["type"])
		assert.Equal(t, "What's in this image?", firstElement["text"])

		// Second element should be image
		secondElement, ok := contentArray[1].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, contentTypeImageUrl, secondElement["type"])
		assert.NotNil(t, secondElement["image_url"])
		imageUrl, ok := secondElement["image_url"].(map[string]interface{})
		require.True(t, ok)
		assert.Contains(t, imageUrl["url"], "data:image/jpeg;base64,")
	})

	t.Run("convert_simple_string_content", func(t *testing.T) {
		// Test case with simple string content
		claudeRequest := `{
			"model": "claude-3-sonnet-20240229",
			"messages": [{
				"role": "user",
				"content": "Hello, how are you?"
			}],
			"max_tokens": 1000
		}`

		result, err := converter.ConvertClaudeRequestToOpenAI([]byte(claudeRequest))
		require.NoError(t, err)

		var openaiRequest chatCompletionRequest
		err = json.Unmarshal(result, &openaiRequest)
		require.NoError(t, err)

		require.Len(t, openaiRequest.Messages, 1)
		userMsg := openaiRequest.Messages[0]
		assert.Equal(t, "user", userMsg.Role)
		assert.Equal(t, "Hello, how are you?", userMsg.Content)
	})

	t.Run("convert_empty_content_array", func(t *testing.T) {
		// Test case with empty content array
		claudeRequest := `{
			"model": "claude-3-sonnet-20240229",
			"messages": [{
				"role": "user",
				"content": []
			}],
			"max_tokens": 1000
		}`

		result, err := converter.ConvertClaudeRequestToOpenAI([]byte(claudeRequest))
		require.NoError(t, err)

		var openaiRequest chatCompletionRequest
		err = json.Unmarshal(result, &openaiRequest)
		require.NoError(t, err)

		require.Len(t, openaiRequest.Messages, 1)
		userMsg := openaiRequest.Messages[0]
		assert.Equal(t, "user", userMsg.Role)

		// Empty array should result in empty array, not string - after JSON marshaling/unmarshaling becomes []interface{}
		if userMsg.Content != nil {
			contentArray, ok := userMsg.Content.([]interface{})
			require.True(t, ok, "Empty content should be an array")
			assert.Empty(t, contentArray)
		} else {
			// null is also acceptable for empty content
			assert.Nil(t, userMsg.Content)
		}
	})

	t.Run("convert_tool_use_to_tool_calls", func(t *testing.T) {
		// Test Claude tool_use conversion to OpenAI tool_calls format
		claudeRequest := `{
			"model": "anthropic/claude-sonnet-4",
			"messages": [{
				"role": "assistant",
				"content": [{
					"type": "text",
					"text": "I'll help you search for information."
				}, {
					"type": "tool_use",
					"id": "toolu_01D7FLrfh4GYq7yT1ULFeyMV",
					"name": "web_search",
					"input": {
						"query": "Claude AI capabilities",
						"max_results": 5
					}
				}]
			}],
			"max_tokens": 1000
		}`

		result, err := converter.ConvertClaudeRequestToOpenAI([]byte(claudeRequest))
		require.NoError(t, err)

		var openaiRequest chatCompletionRequest
		err = json.Unmarshal(result, &openaiRequest)
		require.NoError(t, err)

		// Should have one assistant message with tool_calls
		require.Len(t, openaiRequest.Messages, 1)
		assistantMsg := openaiRequest.Messages[0]
		assert.Equal(t, "assistant", assistantMsg.Role)
		assert.Equal(t, "I'll help you search for information.", assistantMsg.Content)

		// Verify tool_calls format
		require.NotNil(t, assistantMsg.ToolCalls)
		require.Len(t, assistantMsg.ToolCalls, 1)

		toolCall := assistantMsg.ToolCalls[0]
		assert.Equal(t, "toolu_01D7FLrfh4GYq7yT1ULFeyMV", toolCall.Id)
		assert.Equal(t, "function", toolCall.Type)
		assert.Equal(t, "web_search", toolCall.Function.Name)

		// Verify arguments are properly JSON encoded
		var args map[string]interface{}
		err = json.Unmarshal([]byte(toolCall.Function.Arguments), &args)
		require.NoError(t, err)
		assert.Equal(t, "Claude AI capabilities", args["query"])
		assert.Equal(t, float64(5), args["max_results"])
	})

	t.Run("convert_tool_result_to_tool_message", func(t *testing.T) {
		// Test Claude tool_result conversion to OpenAI tool message format
		claudeRequest := `{
			"model": "anthropic/claude-sonnet-4",
			"messages": [{
				"role": "user",
				"content": [{
					"type": "tool_result",
					"tool_use_id": "toolu_01D7FLrfh4GYq7yT1ULFeyMV",
					"content": "Search results: Claude is an AI assistant created by Anthropic."
				}]
			}],
			"max_tokens": 1000
		}`

		result, err := converter.ConvertClaudeRequestToOpenAI([]byte(claudeRequest))
		require.NoError(t, err)

		var openaiRequest chatCompletionRequest
		err = json.Unmarshal(result, &openaiRequest)
		require.NoError(t, err)

		// Should have one tool message
		require.Len(t, openaiRequest.Messages, 1)
		toolMsg := openaiRequest.Messages[0]
		assert.Equal(t, "tool", toolMsg.Role)
		assert.Equal(t, "Search results: Claude is an AI assistant created by Anthropic.", toolMsg.Content)
		assert.Equal(t, "toolu_01D7FLrfh4GYq7yT1ULFeyMV", toolMsg.ToolCallId)
	})

	t.Run("convert_tool_result_with_array_content", func(t *testing.T) {
		// Test Claude tool_result with array content format (new format that was causing the error)
		claudeRequest := `{
			"model": "anthropic/claude-sonnet-4",
			"messages": [{
				"role": "user",
				"content": [{
					"type": "tool_result",
					"tool_use_id": "toolu_vrtx_01UbCfwoTgoDBqbYEwkVaxd5",
					"content": [{
						"text": "Search results for three.js libraries and frameworks",
						"type": "text"
					}]
				}]
			}],
			"max_tokens": 1000
		}`

		result, err := converter.ConvertClaudeRequestToOpenAI([]byte(claudeRequest))
		require.NoError(t, err)

		var openaiRequest chatCompletionRequest
		err = json.Unmarshal(result, &openaiRequest)
		require.NoError(t, err)

		// Should have one tool message
		require.Len(t, openaiRequest.Messages, 1)
		toolMsg := openaiRequest.Messages[0]

		assert.Equal(t, "tool", toolMsg.Role)
		assert.Equal(t, "Search results for three.js libraries and frameworks", toolMsg.Content)
		assert.Equal(t, "toolu_vrtx_01UbCfwoTgoDBqbYEwkVaxd5", toolMsg.ToolCallId)
	})

	t.Run("convert_tool_result_with_actual_error_data", func(t *testing.T) {
		// Test using the actual JSON data from the error log to ensure our fix works
		// This tests the fix for issue #3344 - text content alongside tool_result should be preserved
		claudeRequest := `{
			"model": "anthropic/claude-sonnet-4", 
			"messages": [{
				"role": "user",
				"content": [{
					"content": [{
						"text": "\n  ## 结果 1\n  - **id**: /websites/threejs\n  - **title**: three.js\n  - **description**: three.js is a JavaScript 3D library that makes it easy to create and display animated 3D computer graphics in a web browser. It provides a powerful and flexible way to build interactive 3D experiences.\n",
						"type": "text"
					}],
					"tool_use_id": "toolu_vrtx_01UbCfwoTgoDBqbYEwkVaxd5",
					"type": "tool_result"
				}, {
					"cache_control": {"type": "ephemeral"},
					"text": "继续",
					"type": "text"
				}]
			}],
			"max_tokens": 1000
		}`

		result, err := converter.ConvertClaudeRequestToOpenAI([]byte(claudeRequest))
		require.NoError(t, err)

		var openaiRequest chatCompletionRequest
		err = json.Unmarshal(result, &openaiRequest)
		require.NoError(t, err)

		// Should have two messages: tool message + user message with text content
		// This is the fix for issue #3344 - text content alongside tool_result is preserved
		require.Len(t, openaiRequest.Messages, 2)

		// First should be tool message
		toolMsg := openaiRequest.Messages[0]
		assert.Equal(t, "tool", toolMsg.Role)
		assert.Contains(t, toolMsg.Content, "three.js")
		assert.Equal(t, "toolu_vrtx_01UbCfwoTgoDBqbYEwkVaxd5", toolMsg.ToolCallId)

		// Second should be user message with text content
		userMsg := openaiRequest.Messages[1]
		assert.Equal(t, "user", userMsg.Role)
		assert.Equal(t, "继续", userMsg.Content)
	})

	t.Run("convert_multiple_tool_calls", func(t *testing.T) {
		// Test multiple tool_use in single message
		claudeRequest := `{
			"model": "anthropic/claude-sonnet-4", 
			"messages": [{
				"role": "assistant",
				"content": [{
					"type": "tool_use",
					"id": "toolu_search",
					"name": "web_search",
					"input": {"query": "weather"}
				}, {
					"type": "tool_use", 
					"id": "toolu_calc",
					"name": "calculate",
					"input": {"expression": "2+2"}
				}]
			}],
			"max_tokens": 1000
		}`

		result, err := converter.ConvertClaudeRequestToOpenAI([]byte(claudeRequest))
		require.NoError(t, err)

		var openaiRequest chatCompletionRequest
		err = json.Unmarshal(result, &openaiRequest)
		require.NoError(t, err)

		// Should have one assistant message with multiple tool_calls
		require.Len(t, openaiRequest.Messages, 1)
		assistantMsg := openaiRequest.Messages[0]
		assert.Equal(t, "assistant", assistantMsg.Role)
		assert.Nil(t, assistantMsg.Content) // No text content, so should be null

		// Verify multiple tool_calls
		require.NotNil(t, assistantMsg.ToolCalls)
		require.Len(t, assistantMsg.ToolCalls, 2)

		// First tool call
		assert.Equal(t, "toolu_search", assistantMsg.ToolCalls[0].Id)
		assert.Equal(t, "web_search", assistantMsg.ToolCalls[0].Function.Name)

		// Second tool call
		assert.Equal(t, "toolu_calc", assistantMsg.ToolCalls[1].Id)
		assert.Equal(t, "calculate", assistantMsg.ToolCalls[1].Function.Name)
	})

	t.Run("convert_multiple_tool_results", func(t *testing.T) {
		// Test multiple tool_result messages
		claudeRequest := `{
			"model": "anthropic/claude-sonnet-4",
			"messages": [{
				"role": "user",
				"content": [{
					"type": "tool_result",
					"tool_use_id": "toolu_search",
					"content": "Weather: 25°C sunny"
				}, {
					"type": "tool_result",
					"tool_use_id": "toolu_calc", 
					"content": "Result: 4"
				}]
			}],
			"max_tokens": 1000
		}`

		result, err := converter.ConvertClaudeRequestToOpenAI([]byte(claudeRequest))
		require.NoError(t, err)

		var openaiRequest chatCompletionRequest
		err = json.Unmarshal(result, &openaiRequest)
		require.NoError(t, err)

		// Should have two tool messages
		require.Len(t, openaiRequest.Messages, 2)

		// First tool result
		toolMsg1 := openaiRequest.Messages[0]
		assert.Equal(t, "tool", toolMsg1.Role)
		assert.Equal(t, "Weather: 25°C sunny", toolMsg1.Content)
		assert.Equal(t, "toolu_search", toolMsg1.ToolCallId)

		// Second tool result
		toolMsg2 := openaiRequest.Messages[1]
		assert.Equal(t, "tool", toolMsg2.Role)
		assert.Equal(t, "Result: 4", toolMsg2.Content)
		assert.Equal(t, "toolu_calc", toolMsg2.ToolCallId)
	})

	t.Run("convert_mixed_text_and_tool_use", func(t *testing.T) {
		// Test message with both text and tool_use
		claudeRequest := `{
			"model": "anthropic/claude-sonnet-4",
			"messages": [{
				"role": "assistant",
				"content": [{
					"type": "text",
					"text": "Let me search for that information and do a calculation."
				}, {
					"type": "tool_use",
					"id": "toolu_search123",
					"name": "search_database",
					"input": {"table": "users", "limit": 10}
				}]
			}],
			"max_tokens": 1000
		}`

		result, err := converter.ConvertClaudeRequestToOpenAI([]byte(claudeRequest))
		require.NoError(t, err)

		var openaiRequest chatCompletionRequest
		err = json.Unmarshal(result, &openaiRequest)
		require.NoError(t, err)

		// Should have one assistant message with both content and tool_calls
		require.Len(t, openaiRequest.Messages, 1)
		assistantMsg := openaiRequest.Messages[0]
		assert.Equal(t, "assistant", assistantMsg.Role)
		assert.Equal(t, "Let me search for that information and do a calculation.", assistantMsg.Content)

		// Should have tool_calls
		require.NotNil(t, assistantMsg.ToolCalls)
		require.Len(t, assistantMsg.ToolCalls, 1)
		assert.Equal(t, "toolu_search123", assistantMsg.ToolCalls[0].Id)
		assert.Equal(t, "search_database", assistantMsg.ToolCalls[0].Function.Name)
	})
}

func TestClaudeToOpenAIConverter_ConvertOpenAIResponseToClaude(t *testing.T) {
	converter := &ClaudeToOpenAIConverter{}

	t.Run("convert_tool_calls_response", func(t *testing.T) {
		// Test OpenAI response with tool calls conversion to Claude format
		openaiResponse := `{
			"id": "gen-1756214072-tVFkPBV6lxee00IqNAC5",
			"provider": "Google",
			"model": "anthropic/claude-sonnet-4", 
			"object": "chat.completion",
			"created": 1756214072,
			"choices": [{
				"logprobs": null,
				"finish_reason": "tool_calls",
				"native_finish_reason": "tool_calls",
				"index": 0,
				"message": {
					"role": "assistant",
					"content": "I'll analyze the README file to understand this project's purpose.",
					"refusal": null,
					"reasoning": null,
					"tool_calls": [{
						"id": "toolu_vrtx_017ijjgx8hpigatPzzPW59Wq",
						"index": 0,
						"type": "function",
						"function": {
							"name": "Read",
							"arguments": "{\"file_path\": \"/Users/zhangty/git/higress/README.md\"}"
						}
					}]
				}
			}],
			"usage": {
				"prompt_tokens": 14923,
				"completion_tokens": 81,
				"total_tokens": 15004
			}
		}`

		result, err := converter.ConvertOpenAIResponseToClaude(nil, []byte(openaiResponse))
		require.NoError(t, err)

		var claudeResponse claudeTextGenResponse
		err = json.Unmarshal(result, &claudeResponse)
		require.NoError(t, err)

		// Verify basic response fields
		assert.Equal(t, "gen-1756214072-tVFkPBV6lxee00IqNAC5", claudeResponse.Id)
		assert.Equal(t, "message", claudeResponse.Type)
		assert.Equal(t, "assistant", claudeResponse.Role)
		assert.Equal(t, "anthropic/claude-sonnet-4", claudeResponse.Model)
		assert.Equal(t, "tool_use", *claudeResponse.StopReason)

		// Verify usage
		assert.Equal(t, 14923, claudeResponse.Usage.InputTokens)
		assert.Equal(t, 81, claudeResponse.Usage.OutputTokens)

		// Verify content array has both text and tool_use
		require.Len(t, claudeResponse.Content, 2)

		// First content should be text
		textContent := claudeResponse.Content[0]
		assert.Equal(t, "text", textContent.Type)
		assert.Equal(t, "I'll analyze the README file to understand this project's purpose.", *textContent.Text)

		// Second content should be tool_use
		toolContent := claudeResponse.Content[1]
		assert.Equal(t, "tool_use", toolContent.Type)
		assert.Equal(t, "toolu_vrtx_017ijjgx8hpigatPzzPW59Wq", toolContent.Id)
		assert.Equal(t, "Read", toolContent.Name)

		// Verify tool arguments
		require.NotNil(t, toolContent.Input)
		assert.Equal(t, "/Users/zhangty/git/higress/README.md", (*toolContent.Input)["file_path"])
	})
}

func TestClaudeToOpenAIConverter_ConvertThinkingConfig(t *testing.T) {
	converter := &ClaudeToOpenAIConverter{}

	tests := []struct {
		name           string
		claudeRequest  string
		expectedEffort string
	}{
		{
			name: "thinking_enabled_low",
			claudeRequest: `{
				"model": "claude-sonnet-4",
				"max_tokens": 1000,
				"messages": [{"role": "user", "content": "Hello"}],
				"thinking": {"type": "enabled", "budget_tokens": 2048}
			}`,
			expectedEffort: "low",
		},
		{
			name: "thinking_enabled_medium",
			claudeRequest: `{
				"model": "claude-sonnet-4",
				"max_tokens": 1000,
				"messages": [{"role": "user", "content": "Hello"}],
				"thinking": {"type": "enabled", "budget_tokens": 8192}
			}`,
			expectedEffort: "medium",
		},
		{
			name: "thinking_enabled_high",
			claudeRequest: `{
				"model": "claude-sonnet-4",
				"max_tokens": 1000,
				"messages": [{"role": "user", "content": "Hello"}],
				"thinking": {"type": "enabled", "budget_tokens": 20480}
			}`,
			expectedEffort: "high",
		},
		{
			name: "thinking_disabled",
			claudeRequest: `{
				"model": "claude-sonnet-4",
				"max_tokens": 1000,
				"messages": [{"role": "user", "content": "Hello"}],
				"thinking": {"type": "disabled"}
			}`,
			expectedEffort: "",
		},
		{
			name: "no_thinking",
			claudeRequest: `{
				"model": "claude-sonnet-4",
				"max_tokens": 1000,
				"messages": [{"role": "user", "content": "Hello"}]
			}`,
			expectedEffort: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := converter.ConvertClaudeRequestToOpenAI([]byte(tt.claudeRequest))
			assert.NoError(t, err)
			assert.NotNil(t, result)

			var openaiRequest chatCompletionRequest
			err = json.Unmarshal(result, &openaiRequest)
			assert.NoError(t, err)

			assert.Equal(t, tt.expectedEffort, openaiRequest.ReasoningEffort)

			// Verify non-standard fields are NEVER set in the converted request.
			// These fields are not recognized by OpenAI/Azure and would cause 400 errors.
			assert.Equal(t, 0, openaiRequest.ReasoningMaxTokens,
				"reasoning_max_tokens must not be set - it is not a standard OpenAI parameter")
			assert.Nil(t, openaiRequest.Thinking,
				"thinking must not be set - it is not a standard OpenAI parameter")

			// Also verify at the raw JSON level to catch any serialization issues
			var rawJSON map[string]interface{}
			err = json.Unmarshal(result, &rawJSON)
			require.NoError(t, err)
			assert.NotContains(t, rawJSON, "thinking",
				"raw JSON must not contain 'thinking' field")
			assert.NotContains(t, rawJSON, "reasoning_max_tokens",
				"raw JSON must not contain 'reasoning_max_tokens' field")
		})
	}
}

func TestClaudeToOpenAIConverter_ConvertReasoningResponseToClaude(t *testing.T) {
	converter := &ClaudeToOpenAIConverter{}

	tests := []struct {
		name           string
		openaiResponse string
		expectThinking bool
		expectedText   string
	}{
		{
			name: "response_with_reasoning_content",
			openaiResponse: `{
				"id": "chatcmpl-test123",
				"object": "chat.completion",
				"created": 1699999999,
				"model": "gpt-4o",
				"choices": [{
					"index": 0,
					"message": {
						"role": "assistant",
						"content": "Based on my analysis, the answer is 42.",
						"reasoning_content": "Let me think about this step by step:\n1. The question asks about the meaning of life\n2. According to Douglas Adams, the answer is 42\n3. Therefore, 42 is the correct answer"
					},
					"finish_reason": "stop"
				}],
				"usage": {
					"prompt_tokens": 10,
					"completion_tokens": 20,
					"total_tokens": 30
				}
			}`,
			expectThinking: true,
			expectedText:   "Based on my analysis, the answer is 42.",
		},
		{
			name: "response_with_reasoning_field",
			openaiResponse: `{
				"id": "chatcmpl-test789",
				"object": "chat.completion",
				"created": 1699999999,
				"model": "gpt-4o",
				"choices": [{
					"index": 0,
					"message": {
						"role": "assistant",
						"content": "Based on my analysis, the answer is 42.",
						"reasoning": "Let me think about this step by step:\n1. The question asks about the meaning of life\n2. According to Douglas Adams, the answer is 42\n3. Therefore, 42 is the correct answer"
					},
					"finish_reason": "stop"
				}],
				"usage": {
					"prompt_tokens": 10,
					"completion_tokens": 20,
					"total_tokens": 30
				}
			}`,
			expectThinking: true,
			expectedText:   "Based on my analysis, the answer is 42.",
		},
		{
			name: "response_without_reasoning_content",
			openaiResponse: `{
				"id": "chatcmpl-test456",
				"object": "chat.completion",
				"created": 1699999999,
				"model": "gpt-4o",
				"choices": [{
					"index": 0,
					"message": {
						"role": "assistant",
						"content": "The answer is 42."
					},
					"finish_reason": "stop"
				}],
				"usage": {
					"prompt_tokens": 5,
					"completion_tokens": 10,
					"total_tokens": 15
				}
			}`,
			expectThinking: false,
			expectedText:   "The answer is 42.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := converter.ConvertOpenAIResponseToClaude(nil, []byte(tt.openaiResponse))
			assert.NoError(t, err)
			assert.NotNil(t, result)

			var claudeResponse claudeTextGenResponse
			err = json.Unmarshal(result, &claudeResponse)
			assert.NoError(t, err)

			// Verify response structure
			assert.Equal(t, "message", claudeResponse.Type)
			assert.Equal(t, "assistant", claudeResponse.Role)
			assert.NotEmpty(t, claudeResponse.Id) // ID should be present

			if tt.expectThinking {
				// Should have both thinking and text content
				assert.Len(t, claudeResponse.Content, 2)

				// First should be thinking
				thinkingContent := claudeResponse.Content[0]
				assert.Equal(t, "thinking", thinkingContent.Type)
				require.NotNil(t, thinkingContent.Signature)
				assert.Equal(t, "", *thinkingContent.Signature) // OpenAI doesn't provide signature
				require.NotNil(t, thinkingContent.Thinking)
				assert.Contains(t, *thinkingContent.Thinking, "Let me think about this step by step")

				// Second should be text
				textContent := claudeResponse.Content[1]
				assert.Equal(t, "text", textContent.Type)
				require.NotNil(t, textContent.Text)
				assert.Equal(t, tt.expectedText, *textContent.Text)
			} else {
				// Should only have text content
				assert.Len(t, claudeResponse.Content, 1)

				textContent := claudeResponse.Content[0]
				assert.Equal(t, "text", textContent.Type)
				require.NotNil(t, textContent.Text)
				assert.Equal(t, tt.expectedText, *textContent.Text)
			}
		})
	}
}

func TestClaudeToOpenAIConverter_ConvertOpenAIStreamResponseToClaude_WithCachedTokens(t *testing.T) {
	converter := &ClaudeToOpenAIConverter{}

	streamChunk := "data: {\"id\":\"chatcmpl-test\",\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"}}]}\n\n" +
		"data: {\"id\":\"chatcmpl-test\",\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":20,\"total_tokens\":120,\"prompt_tokens_details\":{\"cached_tokens\":60}}}\n\n"

	result, err := converter.ConvertOpenAIStreamResponseToClaude(nil, []byte(streamChunk))
	require.NoError(t, err)

	resultStr := string(result)
	assert.Contains(t, resultStr, "\"type\":\"message_delta\"")
	assert.Contains(t, resultStr, "\"input_tokens\":100")
	assert.Contains(t, resultStr, "\"output_tokens\":20")
	assert.Contains(t, resultStr, "\"cache_read_input_tokens\":60")
}

func TestClaudeToOpenAIConverter_StripCchFromSystemMessage(t *testing.T) {
	converter := &ClaudeToOpenAIConverter{}

	t.Run("string_system_with_billing_header", func(t *testing.T) {
		// Test that cch field is stripped from string format system message
		claudeRequest := `{
			"model": "claude-sonnet-4",
			"max_tokens": 1024,
			"system": [
				{
					"type": "text",
					"text": "x-anthropic-billing-header: cc_version=2.1.37.3a3; cc_entrypoint=claude-vscode; cch=abc123;"
				}
			],
			"messages": [{
				"role": "user",
				"content": "Hello"
			}]
		}`

		result, err := converter.ConvertClaudeRequestToOpenAI([]byte(claudeRequest))
		require.NoError(t, err)

		var openaiRequest chatCompletionRequest
		err = json.Unmarshal(result, &openaiRequest)
		require.NoError(t, err)

		require.Len(t, openaiRequest.Messages, 2)

		// First message should be system with cch stripped
		systemMsg := openaiRequest.Messages[0]
		assert.Equal(t, "system", systemMsg.Role)

		// The system content should have cch removed
		contentArray, ok := systemMsg.Content.([]interface{})
		require.True(t, ok, "System content should be an array")
		require.Len(t, contentArray, 1)

		contentMap, ok := contentArray[0].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "text", contentMap["type"])
		assert.Equal(t, "x-anthropic-billing-header: cc_version=2.1.37.3a3; cc_entrypoint=claude-vscode;", contentMap["text"])
		assert.NotContains(t, contentMap["text"], "cch=")
	})

	t.Run("plain_string_system_unchanged", func(t *testing.T) {
		// Test that normal system messages are not modified
		claudeRequest := `{
			"model": "claude-sonnet-4",
			"max_tokens": 1024,
			"system": "You are a helpful assistant.",
			"messages": [{
				"role": "user",
				"content": "Hello"
			}]
		}`

		result, err := converter.ConvertClaudeRequestToOpenAI([]byte(claudeRequest))
		require.NoError(t, err)

		var openaiRequest chatCompletionRequest
		err = json.Unmarshal(result, &openaiRequest)
		require.NoError(t, err)

		// First message should be system with original content
		systemMsg := openaiRequest.Messages[0]
		assert.Equal(t, "system", systemMsg.Role)
		assert.Equal(t, "You are a helpful assistant.", systemMsg.Content)
	})
}
func TestStripCchFromBillingHeader(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "billing header with cch at end",
			input:    "x-anthropic-billing-header: cc_version=2.1.37.3a3; cc_entrypoint=claude-vscode; cch=abc123;",
			expected: "x-anthropic-billing-header: cc_version=2.1.37.3a3; cc_entrypoint=claude-vscode;",
		},
		{
			name:     "billing header with cch at end without trailing semicolon",
			input:    "x-anthropic-billing-header: cc_version=2.1.37.3a3; cc_entrypoint=claude-vscode; cch=abc123",
			expected: "x-anthropic-billing-header: cc_version=2.1.37.3a3; cc_entrypoint=claude-vscode",
		},
		{
			name:     "billing header with cch in middle",
			input:    "x-anthropic-billing-header: cc_version=2.1.37.3a3; cch=abc123; cc_entrypoint=claude-vscode;",
			expected: "x-anthropic-billing-header: cc_version=2.1.37.3a3; cc_entrypoint=claude-vscode;",
		},
		{
			name:     "billing header without cch",
			input:    "x-anthropic-billing-header: cc_version=2.1.37.3a3; cc_entrypoint=claude-vscode;",
			expected: "x-anthropic-billing-header: cc_version=2.1.37.3a3; cc_entrypoint=claude-vscode;",
		},
		{
			name:     "non-billing header text unchanged",
			input:    "This is a normal system prompt",
			expected: "This is a normal system prompt",
		},
		{
			name:     "empty string unchanged",
			input:    "",
			expected: "",
		},
		{
			name:     "billing header with multiple cch fields",
			input:    "x-anthropic-billing-header: cc_version=2.1.37.3a3; cch=first; cc_entrypoint=claude-vscode; cch=second;",
			expected: "x-anthropic-billing-header: cc_version=2.1.37.3a3; cc_entrypoint=claude-vscode;",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripCchFromBillingHeader(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizeFinishReason(t *testing.T) {
	tests := []struct {
		name       string
		input      *string
		wantReason string
		wantValid  bool
	}{
		{
			name:      "nil finish reason",
			input:     nil,
			wantValid: false,
		},
		{
			name:      "empty finish reason",
			input:     stringPtr(""),
			wantValid: false,
		},
		{
			name:      "whitespace finish reason",
			input:     stringPtr("   "),
			wantValid: false,
		},
		{
			name:      "string null finish reason",
			input:     stringPtr("null"),
			wantValid: false,
		},
		{
			name:      "uppercase string null finish reason",
			input:     stringPtr("NULL"),
			wantValid: false,
		},
		{
			name:       "valid finish reason",
			input:      stringPtr("length"),
			wantReason: "length",
			wantValid:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotReason, gotValid := normalizeFinishReason(tt.input)
			assert.Equal(t, tt.wantReason, gotReason)
			assert.Equal(t, tt.wantValid, gotValid)
		})
	}
}

func TestClaudeToOpenAIConverter_ConvertOpenAIStreamResponseToClaude_Compatibility(t *testing.T) {
	t.Run("finish_reason empty string should not stop stream", func(t *testing.T) {
		converter := &ClaudeToOpenAIConverter{}

		chunk1 := `data: {"id":"stream-1","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":""}],"created":1,"model":"m","object":"chat.completion.chunk"}` + "\n\n"
		out1, err := converter.ConvertOpenAIStreamResponseToClaude(nil, []byte(chunk1))
		require.NoError(t, err)
		events1 := parseClaudeSSEEvents(t, out1)
		require.Len(t, events1, 1)
		assert.Equal(t, "message_start", events1[0].Name)

		chunk2 := `data: {"id":"stream-1","choices":[{"index":0,"delta":{"reasoning_content":"Let"},"finish_reason":""}],"created":1,"model":"m","object":"chat.completion.chunk","usage":{"prompt_tokens":10,"completion_tokens":1,"total_tokens":11}}` + "\n\n"
		out2, err := converter.ConvertOpenAIStreamResponseToClaude(nil, []byte(chunk2))
		require.NoError(t, err)
		events2 := parseClaudeSSEEvents(t, out2)
		require.Len(t, events2, 3)
		assert.Equal(t, "content_block_start", events2[0].Name)
		assert.Equal(t, "content_block_delta", events2[1].Name)
		assert.Equal(t, "message_delta", events2[2].Name)
		assert.Nil(t, events2[2].Payload.Delta.StopReason, "usage chunk without real finish_reason must not carry stop_reason")

		eventNames := []string{events2[0].Name, events2[1].Name, events2[2].Name}
		assert.NotContains(t, eventNames, "content_block_stop")
		assert.NotContains(t, eventNames, "message_stop")
	})

	t.Run("usage in every chunk should not trigger early message_stop", func(t *testing.T) {
		converter := &ClaudeToOpenAIConverter{}

		chunkStart := `data: {"id":"stream-2","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}],"created":1,"model":"m","object":"chat.completion.chunk"}` + "\n\n"
		_, err := converter.ConvertOpenAIStreamResponseToClaude(nil, []byte(chunkStart))
		require.NoError(t, err)

		chunkThinking1 := `data: {"id":"stream-2","choices":[{"index":0,"delta":{"reasoning_content":"Let"},"finish_reason":null}],"created":1,"model":"m","object":"chat.completion.chunk","usage":{"prompt_tokens":10,"completion_tokens":1,"total_tokens":11}}` + "\n\n"
		outThinking1, err := converter.ConvertOpenAIStreamResponseToClaude(nil, []byte(chunkThinking1))
		require.NoError(t, err)
		eventsThinking1 := parseClaudeSSEEvents(t, outThinking1)
		require.Len(t, eventsThinking1, 3)
		assert.Equal(t, "message_delta", eventsThinking1[2].Name)
		assert.Nil(t, eventsThinking1[2].Payload.Delta.StopReason)

		chunkThinking2 := `data: {"id":"stream-2","choices":[{"index":0,"delta":{"reasoning_content":" me"},"finish_reason":null}],"created":1,"model":"m","object":"chat.completion.chunk","usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}` + "\n\n"
		outThinking2, err := converter.ConvertOpenAIStreamResponseToClaude(nil, []byte(chunkThinking2))
		require.NoError(t, err)
		eventsThinking2 := parseClaudeSSEEvents(t, outThinking2)
		require.Len(t, eventsThinking2, 2)
		assert.Equal(t, "content_block_delta", eventsThinking2[0].Name)
		assert.Equal(t, "message_delta", eventsThinking2[1].Name)
		assert.Nil(t, eventsThinking2[1].Payload.Delta.StopReason)

		chunkFinishNoUsage := `data: {"id":"stream-2","choices":[{"index":0,"delta":{"content":"","reasoning_content":""},"finish_reason":"length"}],"created":1,"model":"m","object":"chat.completion.chunk"}` + "\n\n"
		outFinishNoUsage, err := converter.ConvertOpenAIStreamResponseToClaude(nil, []byte(chunkFinishNoUsage))
		require.NoError(t, err)
		eventsFinishNoUsage := parseClaudeSSEEvents(t, outFinishNoUsage)
		require.Len(t, eventsFinishNoUsage, 1)
		assert.Equal(t, "content_block_stop", eventsFinishNoUsage[0].Name)

		chunkFinalUsage := `data: {"id":"stream-2","choices":[],"created":1,"model":"m","object":"chat.completion.chunk","usage":{"prompt_tokens":10,"completion_tokens":100,"total_tokens":110}}` + "\n\n"
		outFinalUsage, err := converter.ConvertOpenAIStreamResponseToClaude(nil, []byte(chunkFinalUsage))
		require.NoError(t, err)
		eventsFinalUsage := parseClaudeSSEEvents(t, outFinalUsage)
		require.Len(t, eventsFinalUsage, 2)
		assert.Equal(t, "message_delta", eventsFinalUsage[0].Name)
		require.NotNil(t, eventsFinalUsage[0].Payload.Delta.StopReason)
		assert.Equal(t, "max_tokens", *eventsFinalUsage[0].Payload.Delta.StopReason)
		assert.Equal(t, "message_stop", eventsFinalUsage[1].Name)

		chunkDuplicateUsage := `data: {"id":"stream-2","choices":[],"created":1,"model":"m","object":"chat.completion.chunk","usage":{"prompt_tokens":10,"completion_tokens":100,"total_tokens":110}}` + "\n\n"
		outDuplicateUsage, err := converter.ConvertOpenAIStreamResponseToClaude(nil, []byte(chunkDuplicateUsage))
		require.NoError(t, err)
		assert.Empty(t, strings.TrimSpace(string(outDuplicateUsage)), "duplicate trailing chunks after message_stop should be ignored")

		doneChunk := "data: [DONE]\n\n"
		outDone, err := converter.ConvertOpenAIStreamResponseToClaude(nil, []byte(doneChunk))
		require.NoError(t, err)
		assert.Empty(t, strings.TrimSpace(string(outDone)))

		nextRequestChunk := `data: {"id":"stream-3","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}],"created":1,"model":"m","object":"chat.completion.chunk"}` + "\n\n"
		outNextRequest, err := converter.ConvertOpenAIStreamResponseToClaude(nil, []byte(nextRequestChunk))
		require.NoError(t, err)
		eventsNextRequest := parseClaudeSSEEvents(t, outNextRequest)
		require.Len(t, eventsNextRequest, 1)
		assert.Equal(t, "message_start", eventsNextRequest[0].Name)
	})
}

type parsedClaudeSSEEvent struct {
	Name    string
	Payload claudeTextGenStreamResponse
}

func parseClaudeSSEEvents(t *testing.T, raw []byte) []parsedClaudeSSEEvent {
	t.Helper()

	text := strings.TrimSpace(string(raw))
	if text == "" {
		return nil
	}

	blocks := strings.Split(text, "\n\n")
	events := make([]parsedClaudeSSEEvent, 0, len(blocks))
	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}

		var eventName string
		var dataPayload string
		for _, line := range strings.Split(block, "\n") {
			if strings.HasPrefix(line, "event: ") {
				eventName = strings.TrimPrefix(line, "event: ")
			}
			if strings.HasPrefix(line, "data: ") {
				dataPayload = strings.TrimPrefix(line, "data: ")
			}
		}

		require.NotEmpty(t, eventName)
		require.NotEmpty(t, dataPayload)

		var payload claudeTextGenStreamResponse
		require.NoError(t, json.Unmarshal([]byte(dataPayload), &payload))
		events = append(events, parsedClaudeSSEEvent{
			Name:    eventName,
			Payload: payload,
		})
	}

	return events
}

func stringPtr(value string) *string {
	return &value
}
