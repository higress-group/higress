package provider

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeConsecutiveMessages(t *testing.T) {
	t.Run("no_consecutive_messages", func(t *testing.T) {
		input := chatCompletionRequest{
			Messages: []chatMessage{
				{Role: "user", Content: "你好"},
				{Role: "assistant", Content: "你好！"},
				{Role: "user", Content: "再见"},
			},
		}
		body, err := json.Marshal(input)
		require.NoError(t, err)

		result, err := mergeConsecutiveMessages(body)
		assert.NoError(t, err)
		// No merging needed, returned body should be identical
		assert.Equal(t, body, result)
	})

	t.Run("merges_consecutive_user_messages", func(t *testing.T) {
		input := chatCompletionRequest{
			Messages: []chatMessage{
				{Role: "user", Content: "第一条"},
				{Role: "user", Content: "第二条"},
				{Role: "assistant", Content: "回复"},
			},
		}
		body, err := json.Marshal(input)
		require.NoError(t, err)

		result, err := mergeConsecutiveMessages(body)
		assert.NoError(t, err)

		var output chatCompletionRequest
		require.NoError(t, json.Unmarshal(result, &output))

		assert.Len(t, output.Messages, 2)
		assert.Equal(t, "user", output.Messages[0].Role)
		assert.Equal(t, "第一条\n\n第二条", output.Messages[0].Content)
		assert.Equal(t, "assistant", output.Messages[1].Role)
	})

	t.Run("merges_consecutive_assistant_messages", func(t *testing.T) {
		input := chatCompletionRequest{
			Messages: []chatMessage{
				{Role: "user", Content: "问题"},
				{Role: "assistant", Content: "第一段"},
				{Role: "assistant", Content: "第二段"},
			},
		}
		body, err := json.Marshal(input)
		require.NoError(t, err)

		result, err := mergeConsecutiveMessages(body)
		assert.NoError(t, err)

		var output chatCompletionRequest
		require.NoError(t, json.Unmarshal(result, &output))

		assert.Len(t, output.Messages, 2)
		assert.Equal(t, "user", output.Messages[0].Role)
		assert.Equal(t, "assistant", output.Messages[1].Role)
		assert.Equal(t, "第一段\n\n第二段", output.Messages[1].Content)
	})

	t.Run("merges_consecutive_assistant_reasoning_content", func(t *testing.T) {
		input := chatCompletionRequest{
			Messages: []chatMessage{
				{Role: "user", Content: "问题"},
				{Role: "assistant", Content: "第一段", ReasoningContent: "第一段推理"},
				{Role: "assistant", Content: "第二段", ReasoningContent: "第二段推理"},
			},
		}
		body, err := json.Marshal(input)
		require.NoError(t, err)

		result, err := mergeConsecutiveMessages(body)
		assert.NoError(t, err)

		var output chatCompletionRequest
		require.NoError(t, json.Unmarshal(result, &output))

		assert.Len(t, output.Messages, 2)
		assert.Equal(t, "assistant", output.Messages[1].Role)
		assert.Equal(t, "第一段\n\n第二段", output.Messages[1].Content)
		assert.Equal(t, "第一段推理\n\n第二段推理", output.Messages[1].ReasoningContent)
	})

	t.Run("does_not_merge_assistant_messages_with_tool_calls", func(t *testing.T) {
		input := chatCompletionRequest{
			Messages: []chatMessage{
				{Role: "user", Content: "问题"},
				{Role: "assistant", Content: "先解释"},
				{
					Role:    "assistant",
					Content: "",
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
				},
			},
		}
		body, err := json.Marshal(input)
		require.NoError(t, err)

		result, err := mergeConsecutiveMessages(body)
		assert.NoError(t, err)

		var output chatCompletionRequest
		require.NoError(t, json.Unmarshal(result, &output))

		assert.Len(t, output.Messages, 3)
		assert.Equal(t, "assistant", output.Messages[1].Role)
		assert.Equal(t, "先解释", output.Messages[1].Content)
		require.Len(t, output.Messages[2].ToolCalls, 1)
		assert.Equal(t, "call_1", output.Messages[2].ToolCalls[0].Id)
	})

	t.Run("does_not_merge_assistant_messages_with_legacy_function_call", func(t *testing.T) {
		input := chatCompletionRequest{
			Messages: []chatMessage{
				{Role: "user", Content: "问题"},
				{Role: "assistant", Content: "先解释"},
				{
					Role:    "assistant",
					Content: "",
					FunctionCall: &functionCall{
						Name:      "lookup",
						Arguments: `{"q":"weather"}`,
					},
				},
			},
		}
		body, err := json.Marshal(input)
		require.NoError(t, err)

		result, err := mergeConsecutiveMessages(body)
		assert.NoError(t, err)

		var output chatCompletionRequest
		require.NoError(t, json.Unmarshal(result, &output))

		assert.Len(t, output.Messages, 3)
		assert.Equal(t, "assistant", output.Messages[1].Role)
		assert.Equal(t, "先解释", output.Messages[1].Content)
		require.NotNil(t, output.Messages[2].FunctionCall)
		assert.Equal(t, "lookup", output.Messages[2].FunctionCall.Name)
	})

	t.Run("merges_multiple_consecutive_same_role", func(t *testing.T) {
		input := chatCompletionRequest{
			Messages: []chatMessage{
				{Role: "user", Content: "A"},
				{Role: "user", Content: "B"},
				{Role: "user", Content: "C"},
				{Role: "assistant", Content: "回复"},
			},
		}
		body, err := json.Marshal(input)
		require.NoError(t, err)

		result, err := mergeConsecutiveMessages(body)
		assert.NoError(t, err)

		var output chatCompletionRequest
		require.NoError(t, json.Unmarshal(result, &output))

		assert.Len(t, output.Messages, 2)
		assert.Equal(t, "A\n\nB\n\nC", output.Messages[0].Content)
	})

	t.Run("system_messages_not_merged", func(t *testing.T) {
		input := chatCompletionRequest{
			Messages: []chatMessage{
				{Role: "system", Content: "系统提示1"},
				{Role: "system", Content: "系统提示2"},
				{Role: "user", Content: "问题"},
			},
		}
		body, err := json.Marshal(input)
		require.NoError(t, err)

		result, err := mergeConsecutiveMessages(body)
		assert.NoError(t, err)
		// system messages are not merged, body unchanged
		assert.Equal(t, body, result)
	})

	t.Run("single_message_unchanged", func(t *testing.T) {
		input := chatCompletionRequest{
			Messages: []chatMessage{
				{Role: "user", Content: "只有一条"},
			},
		}
		body, err := json.Marshal(input)
		require.NoError(t, err)

		result, err := mergeConsecutiveMessages(body)
		assert.NoError(t, err)
		assert.Equal(t, body, result)
	})

	t.Run("invalid_json_body", func(t *testing.T) {
		body := []byte(`invalid json`)
		result, err := mergeConsecutiveMessages(body)
		assert.Error(t, err)
		assert.Equal(t, body, result)
	})
}

func TestCleanupContextMessages(t *testing.T) {
	t.Run("empty_cleanup_commands", func(t *testing.T) {
		body := []byte(`{"messages":[{"role":"user","content":"hello"}]}`)
		result, err := cleanupContextMessages(body, []string{})
		assert.NoError(t, err)
		assert.Equal(t, body, result)
	})

	t.Run("no_matching_command", func(t *testing.T) {
		body := []byte(`{"messages":[{"role":"system","content":"你是助手"},{"role":"user","content":"hello"}]}`)
		result, err := cleanupContextMessages(body, []string{"清理上下文", "/clear"})
		assert.NoError(t, err)
		assert.Equal(t, body, result)
	})

	t.Run("cleanup_with_single_command", func(t *testing.T) {
		input := chatCompletionRequest{
			Messages: []chatMessage{
				{Role: "system", Content: "你是一个助手"},
				{Role: "user", Content: "你好"},
				{Role: "assistant", Content: "你好！"},
				{Role: "user", Content: "清理上下文"},
				{Role: "user", Content: "新问题"},
			},
		}
		body, err := json.Marshal(input)
		require.NoError(t, err)

		result, err := cleanupContextMessages(body, []string{"清理上下文"})
		assert.NoError(t, err)

		var output chatCompletionRequest
		err = json.Unmarshal(result, &output)
		require.NoError(t, err)

		assert.Len(t, output.Messages, 2)
		assert.Equal(t, "system", output.Messages[0].Role)
		assert.Equal(t, "你是一个助手", output.Messages[0].Content)
		assert.Equal(t, "user", output.Messages[1].Role)
		assert.Equal(t, "新问题", output.Messages[1].Content)
	})

	t.Run("cleanup_with_multiple_commands_match_first", func(t *testing.T) {
		input := chatCompletionRequest{
			Messages: []chatMessage{
				{Role: "system", Content: "你是一个助手"},
				{Role: "user", Content: "你好"},
				{Role: "assistant", Content: "你好！"},
				{Role: "user", Content: "/clear"},
				{Role: "user", Content: "新问题"},
			},
		}
		body, err := json.Marshal(input)
		require.NoError(t, err)

		result, err := cleanupContextMessages(body, []string{"清理上下文", "/clear", "重新开始"})
		assert.NoError(t, err)

		var output chatCompletionRequest
		err = json.Unmarshal(result, &output)
		require.NoError(t, err)

		assert.Len(t, output.Messages, 2)
		assert.Equal(t, "system", output.Messages[0].Role)
		assert.Equal(t, "user", output.Messages[1].Role)
		assert.Equal(t, "新问题", output.Messages[1].Content)
	})

	t.Run("cleanup_removes_tool_messages", func(t *testing.T) {
		input := chatCompletionRequest{
			Messages: []chatMessage{
				{Role: "system", Content: "你是一个助手"},
				{Role: "user", Content: "查天气"},
				{Role: "assistant", Content: ""},
				{Role: "tool", Content: "北京 25°C"},
				{Role: "assistant", Content: "北京今天25度"},
				{Role: "user", Content: "清理上下文"},
				{Role: "user", Content: "新问题"},
			},
		}
		body, err := json.Marshal(input)
		require.NoError(t, err)

		result, err := cleanupContextMessages(body, []string{"清理上下文"})
		assert.NoError(t, err)

		var output chatCompletionRequest
		err = json.Unmarshal(result, &output)
		require.NoError(t, err)

		assert.Len(t, output.Messages, 2)
		assert.Equal(t, "system", output.Messages[0].Role)
		assert.Equal(t, "user", output.Messages[1].Role)
	})

	t.Run("cleanup_keeps_multiple_system_messages", func(t *testing.T) {
		input := chatCompletionRequest{
			Messages: []chatMessage{
				{Role: "system", Content: "系统提示1"},
				{Role: "system", Content: "系统提示2"},
				{Role: "user", Content: "你好"},
				{Role: "assistant", Content: "你好！"},
				{Role: "user", Content: "清理上下文"},
				{Role: "user", Content: "新问题"},
			},
		}
		body, err := json.Marshal(input)
		require.NoError(t, err)

		result, err := cleanupContextMessages(body, []string{"清理上下文"})
		assert.NoError(t, err)

		var output chatCompletionRequest
		err = json.Unmarshal(result, &output)
		require.NoError(t, err)

		assert.Len(t, output.Messages, 3)
		assert.Equal(t, "system", output.Messages[0].Role)
		assert.Equal(t, "系统提示1", output.Messages[0].Content)
		assert.Equal(t, "system", output.Messages[1].Role)
		assert.Equal(t, "系统提示2", output.Messages[1].Content)
		assert.Equal(t, "user", output.Messages[2].Role)
	})

	t.Run("cleanup_finds_last_matching_command", func(t *testing.T) {
		input := chatCompletionRequest{
			Messages: []chatMessage{
				{Role: "system", Content: "你是一个助手"},
				{Role: "user", Content: "清理上下文"},
				{Role: "user", Content: "中间问题"},
				{Role: "assistant", Content: "中间回答"},
				{Role: "user", Content: "清理上下文"},
				{Role: "user", Content: "最后问题"},
			},
		}
		body, err := json.Marshal(input)
		require.NoError(t, err)

		result, err := cleanupContextMessages(body, []string{"清理上下文"})
		assert.NoError(t, err)

		var output chatCompletionRequest
		err = json.Unmarshal(result, &output)
		require.NoError(t, err)

		// 应该匹配最后一个清理命令，保留 system 和 "最后问题"
		assert.Len(t, output.Messages, 2)
		assert.Equal(t, "system", output.Messages[0].Role)
		assert.Equal(t, "user", output.Messages[1].Role)
		assert.Equal(t, "最后问题", output.Messages[1].Content)
	})

	t.Run("cleanup_at_end_of_messages", func(t *testing.T) {
		input := chatCompletionRequest{
			Messages: []chatMessage{
				{Role: "system", Content: "你是一个助手"},
				{Role: "user", Content: "你好"},
				{Role: "assistant", Content: "你好！"},
				{Role: "user", Content: "清理上下文"},
			},
		}
		body, err := json.Marshal(input)
		require.NoError(t, err)

		result, err := cleanupContextMessages(body, []string{"清理上下文"})
		assert.NoError(t, err)

		var output chatCompletionRequest
		err = json.Unmarshal(result, &output)
		require.NoError(t, err)

		// 清理命令在最后，只保留 system
		assert.Len(t, output.Messages, 1)
		assert.Equal(t, "system", output.Messages[0].Role)
	})

	t.Run("cleanup_without_system_message", func(t *testing.T) {
		input := chatCompletionRequest{
			Messages: []chatMessage{
				{Role: "user", Content: "你好"},
				{Role: "assistant", Content: "你好！"},
				{Role: "user", Content: "清理上下文"},
				{Role: "user", Content: "新问题"},
			},
		}
		body, err := json.Marshal(input)
		require.NoError(t, err)

		result, err := cleanupContextMessages(body, []string{"清理上下文"})
		assert.NoError(t, err)

		var output chatCompletionRequest
		err = json.Unmarshal(result, &output)
		require.NoError(t, err)

		// 没有 system 消息，只保留清理命令之后的消息
		assert.Len(t, output.Messages, 1)
		assert.Equal(t, "user", output.Messages[0].Role)
		assert.Equal(t, "新问题", output.Messages[0].Content)
	})

	t.Run("cleanup_with_empty_messages", func(t *testing.T) {
		input := chatCompletionRequest{
			Messages: []chatMessage{},
		}
		body, err := json.Marshal(input)
		require.NoError(t, err)

		result, err := cleanupContextMessages(body, []string{"清理上下文"})
		assert.NoError(t, err)

		var output chatCompletionRequest
		err = json.Unmarshal(result, &output)
		require.NoError(t, err)

		assert.Len(t, output.Messages, 0)
	})

	t.Run("cleanup_command_partial_match_not_triggered", func(t *testing.T) {
		input := chatCompletionRequest{
			Messages: []chatMessage{
				{Role: "system", Content: "你是一个助手"},
				{Role: "user", Content: "请清理上下文吧"},
				{Role: "assistant", Content: "好的"},
			},
		}
		body, err := json.Marshal(input)
		require.NoError(t, err)

		result, err := cleanupContextMessages(body, []string{"清理上下文"})
		assert.NoError(t, err)

		// 部分匹配不应触发清理
		assert.Equal(t, body, result)
	})

	t.Run("invalid_json_body", func(t *testing.T) {
		body := []byte(`invalid json`)
		result, err := cleanupContextMessages(body, []string{"清理上下文"})
		assert.Error(t, err)
		assert.Equal(t, body, result)
	})
}
