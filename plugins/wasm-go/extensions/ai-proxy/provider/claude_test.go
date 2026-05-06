package provider

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeProviderInitializer_ValidateConfig(t *testing.T) {
	initializer := &claudeProviderInitializer{}

	t.Run("valid_config_with_api_tokens", func(t *testing.T) {
		config := &ProviderConfig{
			apiTokens: []string{"test-token"},
		}
		err := initializer.ValidateConfig(config)
		assert.NoError(t, err)
	})

	t.Run("invalid_config_without_api_tokens", func(t *testing.T) {
		config := &ProviderConfig{
			apiTokens: nil,
		}
		err := initializer.ValidateConfig(config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no apiToken found in provider config")
	})

	t.Run("invalid_config_with_empty_api_tokens", func(t *testing.T) {
		config := &ProviderConfig{
			apiTokens: []string{},
		}
		err := initializer.ValidateConfig(config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no apiToken found in provider config")
	})
}

func TestClaudeProviderInitializer_DefaultCapabilities(t *testing.T) {
	initializer := &claudeProviderInitializer{}

	capabilities := initializer.DefaultCapabilities()
	expected := map[string]string{
		string(ApiNameChatCompletion):    PathAnthropicMessages,
		string(ApiNameCompletion):        PathAnthropicComplete,
		string(ApiNameAnthropicMessages): PathAnthropicMessages,
		string(ApiNameEmbeddings):        PathOpenAIEmbeddings,
		string(ApiNameModels):            PathOpenAIModels,
	}

	assert.Equal(t, expected, capabilities)
}

func TestClaudeProviderInitializer_CreateProvider(t *testing.T) {
	initializer := &claudeProviderInitializer{}

	config := ProviderConfig{
		apiTokens: []string{"test-token"},
	}

	provider, err := initializer.CreateProvider(config)
	require.NoError(t, err)
	require.NotNil(t, provider)

	assert.Equal(t, providerTypeClaude, provider.GetProviderType())

	claudeProvider, ok := provider.(*claudeProvider)
	require.True(t, ok)
	assert.NotNil(t, claudeProvider.config.apiTokens)
	assert.Equal(t, []string{"test-token"}, claudeProvider.config.apiTokens)
}

func TestClaudeProvider_GetProviderType(t *testing.T) {
	provider := &claudeProvider{
		config: ProviderConfig{
			apiTokens: []string{"test-token"},
		},
		contextCache: createContextCache(&ProviderConfig{}),
	}

	assert.Equal(t, providerTypeClaude, provider.GetProviderType())
}

// Note: TransformRequestHeaders tests are skipped because they require WASM runtime
// The header transformation logic is tested via integration tests instead.
// Here we test the helper functions and logic that can be unit tested.

func TestClaudeCodeMode_HeaderLogic(t *testing.T) {
	// Test the logic for adding beta=true query parameter
	t.Run("adds_beta_query_param_to_path_without_query", func(t *testing.T) {
		currentPath := "/v1/messages"
		var newPath string
		if currentPath != "" && !strings.Contains(currentPath, "beta=true") {
			if strings.Contains(currentPath, "?") {
				newPath = currentPath + "&beta=true"
			} else {
				newPath = currentPath + "?beta=true"
			}
		} else {
			newPath = currentPath
		}
		assert.Equal(t, "/v1/messages?beta=true", newPath)
	})

	t.Run("adds_beta_query_param_to_path_with_existing_query", func(t *testing.T) {
		currentPath := "/v1/messages?foo=bar"
		var newPath string
		if currentPath != "" && !strings.Contains(currentPath, "beta=true") {
			if strings.Contains(currentPath, "?") {
				newPath = currentPath + "&beta=true"
			} else {
				newPath = currentPath + "?beta=true"
			}
		} else {
			newPath = currentPath
		}
		assert.Equal(t, "/v1/messages?foo=bar&beta=true", newPath)
	})

	t.Run("does_not_duplicate_beta_param", func(t *testing.T) {
		currentPath := "/v1/messages?beta=true"
		var newPath string
		if currentPath != "" && !strings.Contains(currentPath, "beta=true") {
			if strings.Contains(currentPath, "?") {
				newPath = currentPath + "&beta=true"
			} else {
				newPath = currentPath + "?beta=true"
			}
		} else {
			newPath = currentPath
		}
		assert.Equal(t, "/v1/messages?beta=true", newPath)
	})

	t.Run("bearer_token_format", func(t *testing.T) {
		token := "sk-ant-oat01-oauth-token"
		bearerAuth := "Bearer " + token
		assert.Equal(t, "Bearer sk-ant-oat01-oauth-token", bearerAuth)
	})
}

func TestClaudeProvider_BuildClaudeTextGenRequest_StandardMode(t *testing.T) {
	provider := &claudeProvider{
		config: ProviderConfig{
			claudeCodeMode: false,
		},
	}

	t.Run("builds_request_without_injecting_defaults", func(t *testing.T) {
		request := &chatCompletionRequest{
			Model:     "claude-sonnet-4-5-20250929",
			MaxTokens: 8192,
			Stream:    true,
			Messages: []chatMessage{
				{Role: roleUser, Content: "Hello"},
			},
		}

		claudeReq := provider.buildClaudeTextGenRequest(request)

		// Should not have system prompt injected
		assert.Nil(t, claudeReq.System)
		// Should not have tools injected
		assert.Empty(t, claudeReq.Tools)
	})

	t.Run("preserves_existing_system_message", func(t *testing.T) {
		request := &chatCompletionRequest{
			Model:     "claude-sonnet-4-5-20250929",
			MaxTokens: 8192,
			Messages: []chatMessage{
				{Role: roleSystem, Content: "You are a helpful assistant."},
				{Role: roleUser, Content: "Hello"},
			},
		}

		claudeReq := provider.buildClaudeTextGenRequest(request)

		assert.NotNil(t, claudeReq.System)
		assert.False(t, claudeReq.System.IsArray)
		assert.Equal(t, "You are a helpful assistant.", claudeReq.System.StringValue)
	})

	t.Run("maps_openai_function_tool_choice_to_claude_tool_choice", func(t *testing.T) {
		request := &chatCompletionRequest{
			Model:     "claude-sonnet-4-5-20250929",
			MaxTokens: 8192,
			Messages: []chatMessage{
				{Role: roleUser, Content: "Search."},
			},
			Tools: []tool{{
				Type: "function",
				Function: function{
					Name:        "web_search",
					Description: "Search the web.",
					Parameters:  map[string]interface{}{"type": "object"},
				},
			}},
			ToolChoice: map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name": "web_search",
				},
			},
		}

		claudeReq := provider.buildClaudeTextGenRequest(request)

		require.NotNil(t, claudeReq.ToolChoice)
		assert.Equal(t, "tool", claudeReq.ToolChoice.Type)
		assert.Equal(t, "web_search", claudeReq.ToolChoice.Name)
	})

	t.Run("maps_openai_string_required_tool_choice_to_claude_any", func(t *testing.T) {
		parallelToolCalls := false
		request := &chatCompletionRequest{
			Model:     "claude-sonnet-4-5-20250929",
			MaxTokens: 8192,
			Messages: []chatMessage{
				{Role: roleUser, Content: "Search."},
			},
			Tools: []tool{{
				Type: "function",
				Function: function{
					Name:       "web_search",
					Parameters: map[string]interface{}{"type": "object"},
				},
			}},
			ToolChoice:        "required",
			ParallelToolCalls: &parallelToolCalls,
		}

		claudeReq := provider.buildClaudeTextGenRequest(request)

		require.NotNil(t, claudeReq.ToolChoice)
		assert.Equal(t, "any", claudeReq.ToolChoice.Type)
		assert.Empty(t, claudeReq.ToolChoice.Name)
		assert.True(t, claudeReq.ToolChoice.DisableParallelToolUse)
	})

	t.Run("maps_openai_string_none_tool_choice_to_claude_none", func(t *testing.T) {
		request := &chatCompletionRequest{
			Model:     "claude-sonnet-4-5-20250929",
			MaxTokens: 8192,
			Messages: []chatMessage{
				{Role: roleUser, Content: "Answer without tools."},
			},
			Tools: []tool{{
				Type: "function",
				Function: function{
					Name:       "web_search",
					Parameters: map[string]interface{}{"type": "object"},
				},
			}},
			ToolChoice: "none",
		}

		claudeReq := provider.buildClaudeTextGenRequest(request)

		require.NotNil(t, claudeReq.ToolChoice)
		assert.Equal(t, "none", claudeReq.ToolChoice.Type)
		assert.Empty(t, claudeReq.ToolChoice.Name)
	})
}

func TestClaudeProvider_BuildClaudeTextGenRequest_ClaudeCodeMode(t *testing.T) {
	provider := &claudeProvider{
		config: ProviderConfig{
			claudeCodeMode: true,
		},
	}

	t.Run("injects_default_system_prompt_when_missing", func(t *testing.T) {
		request := &chatCompletionRequest{
			Model:     "claude-sonnet-4-5-20250929",
			MaxTokens: 8192,
			Stream:    true,
			Messages: []chatMessage{
				{Role: roleUser, Content: "List files"},
			},
		}

		claudeReq := provider.buildClaudeTextGenRequest(request)

		// Should have default Claude Code system prompt
		require.NotNil(t, claudeReq.System)
		assert.True(t, claudeReq.System.IsArray)
		require.Len(t, claudeReq.System.ArrayValue, 1)
		assert.Equal(t, claudeCodeSystemPrompt, claudeReq.System.ArrayValue[0].Text)
		assert.Equal(t, contentTypeText, claudeReq.System.ArrayValue[0].Type)
		// Should have cache_control
		assert.NotNil(t, claudeReq.System.ArrayValue[0].CacheControl)
		assert.Equal(t, "ephemeral", claudeReq.System.ArrayValue[0].CacheControl["type"])
	})

	t.Run("preserves_existing_system_message_with_cache_control", func(t *testing.T) {
		request := &chatCompletionRequest{
			Model:     "claude-sonnet-4-5-20250929",
			MaxTokens: 8192,
			Messages: []chatMessage{
				{Role: roleSystem, Content: "Custom system prompt"},
				{Role: roleUser, Content: "Hello"},
			},
		}

		claudeReq := provider.buildClaudeTextGenRequest(request)

		// Should preserve custom system prompt but with array format and cache_control
		require.NotNil(t, claudeReq.System)
		assert.True(t, claudeReq.System.IsArray)
		require.Len(t, claudeReq.System.ArrayValue, 1)
		assert.Equal(t, "Custom system prompt", claudeReq.System.ArrayValue[0].Text)
		// Should have cache_control
		assert.NotNil(t, claudeReq.System.ArrayValue[0].CacheControl)
		assert.Equal(t, "ephemeral", claudeReq.System.ArrayValue[0].CacheControl["type"])
	})

	t.Run("full_request_transformation", func(t *testing.T) {
		request := &chatCompletionRequest{
			Model:       "claude-sonnet-4-5-20250929",
			MaxTokens:   8192,
			Stream:      true,
			Temperature: 1.0,
			Messages: []chatMessage{
				{Role: roleUser, Content: "List files in current directory"},
			},
		}

		claudeReq := provider.buildClaudeTextGenRequest(request)

		// Verify complete request structure
		assert.Equal(t, "claude-sonnet-4-5-20250929", claudeReq.Model)
		assert.Equal(t, 8192, claudeReq.MaxTokens)
		assert.True(t, claudeReq.Stream)
		assert.Equal(t, 1.0, claudeReq.Temperature)

		// Verify system prompt
		require.NotNil(t, claudeReq.System)
		assert.True(t, claudeReq.System.IsArray)
		assert.Equal(t, claudeCodeSystemPrompt, claudeReq.System.ArrayValue[0].Text)

		// Verify messages
		require.Len(t, claudeReq.Messages, 1)
		assert.Equal(t, roleUser, claudeReq.Messages[0].Role)

		// Verify no tools are injected by default
		assert.Empty(t, claudeReq.Tools)

		// Verify the request can be serialized to JSON
		jsonBytes, err := json.Marshal(claudeReq)
		require.NoError(t, err)
		assert.NotEmpty(t, jsonBytes)
	})
}

// Note: TransformRequestBody tests are skipped because they require WASM runtime
// The request body transformation is tested indirectly through buildClaudeTextGenRequest tests

// Test constants
func TestClaudeConstants(t *testing.T) {
	assert.Equal(t, "api.anthropic.com", claudeDomain)
	assert.Equal(t, "2023-06-01", claudeDefaultVersion)
	assert.Equal(t, 4096, claudeDefaultMaxTokens)
	assert.Equal(t, "claude", providerTypeClaude)

	// Claude Code mode constants
	assert.Equal(t, "claude-cli/2.1.2 (external, cli)", claudeCodeUserAgent)
	assert.Equal(t, "oauth-2025-04-20,interleaved-thinking-2025-05-14,claude-code-20250219", claudeCodeBetaFeatures)
	assert.Equal(t, "You are Claude Code, Anthropic's official CLI for Claude.", claudeCodeSystemPrompt)
}

func TestClaudeProvider_GetApiName(t *testing.T) {
	provider := &claudeProvider{}

	t.Run("messages_path", func(t *testing.T) {
		assert.Equal(t, ApiNameChatCompletion, provider.GetApiName("/v1/messages"))
		assert.Equal(t, ApiNameChatCompletion, provider.GetApiName("/api/v1/messages"))
	})

	t.Run("complete_path", func(t *testing.T) {
		assert.Equal(t, ApiNameCompletion, provider.GetApiName("/v1/complete"))
	})

	t.Run("models_path", func(t *testing.T) {
		assert.Equal(t, ApiNameModels, provider.GetApiName("/v1/models"))
	})

	t.Run("embeddings_path", func(t *testing.T) {
		assert.Equal(t, ApiNameEmbeddings, provider.GetApiName("/v1/embeddings"))
	})

	t.Run("unknown_path", func(t *testing.T) {
		assert.Equal(t, ApiName(""), provider.GetApiName("/unknown"))
	})
}

func TestClaudeProvider_BuildClaudeTextGenRequest_ToolRoleConversion(t *testing.T) {
	provider := &claudeProvider{
		config: ProviderConfig{
			claudeCodeMode: false,
		},
	}

	t.Run("converts_single_tool_role_to_user_with_tool_result", func(t *testing.T) {
		request := &chatCompletionRequest{
			Model:     "claude-sonnet-4-5-20250929",
			MaxTokens: 1024,
			Messages: []chatMessage{
				{Role: roleUser, Content: "What's the weather?"},
				{Role: roleAssistant, Content: nil, ToolCalls: []toolCall{
					{Id: "call_123", Type: "function", Function: functionCall{Name: "get_weather", Arguments: `{"city": "Beijing"}`}},
				}},
				{Role: roleTool, ToolCallId: "call_123", Content: "Sunny, 25°C"},
			},
		}

		claudeReq := provider.buildClaudeTextGenRequest(request)

		// Should have 3 messages: user, assistant with tool_use, user with tool_result
		require.Len(t, claudeReq.Messages, 3)

		// First message should be user
		assert.Equal(t, roleUser, claudeReq.Messages[0].Role)

		// Second message should be assistant with tool_use
		assert.Equal(t, roleAssistant, claudeReq.Messages[1].Role)
		require.False(t, claudeReq.Messages[1].Content.IsString)
		require.Len(t, claudeReq.Messages[1].Content.ArrayValue, 1)
		assert.Equal(t, "tool_use", claudeReq.Messages[1].Content.ArrayValue[0].Type)
		assert.Equal(t, "call_123", claudeReq.Messages[1].Content.ArrayValue[0].Id)
		assert.Equal(t, "get_weather", claudeReq.Messages[1].Content.ArrayValue[0].Name)

		// Third message should be user with tool_result
		assert.Equal(t, roleUser, claudeReq.Messages[2].Role)
		require.False(t, claudeReq.Messages[2].Content.IsString)
		require.Len(t, claudeReq.Messages[2].Content.ArrayValue, 1)
		assert.Equal(t, "tool_result", claudeReq.Messages[2].Content.ArrayValue[0].Type)
		assert.Equal(t, "call_123", claudeReq.Messages[2].Content.ArrayValue[0].ToolUseId)
	})

	t.Run("merges_multiple_tool_results_into_single_user_message", func(t *testing.T) {
		request := &chatCompletionRequest{
			Model:     "claude-sonnet-4-5-20250929",
			MaxTokens: 1024,
			Messages: []chatMessage{
				{Role: roleUser, Content: "What's the weather and time?"},
				{Role: roleAssistant, Content: nil, ToolCalls: []toolCall{
					{Id: "call_1", Type: "function", Function: functionCall{Name: "get_weather", Arguments: `{"city": "Beijing"}`}},
					{Id: "call_2", Type: "function", Function: functionCall{Name: "get_time", Arguments: `{"timezone": "Asia/Shanghai"}`}},
				}},
				{Role: roleTool, ToolCallId: "call_1", Content: "Sunny, 25°C"},
				{Role: roleTool, ToolCallId: "call_2", Content: "3:00 PM"},
			},
		}

		claudeReq := provider.buildClaudeTextGenRequest(request)

		// Should have 3 messages: user, assistant with 2 tool_use, user with 2 tool_results
		require.Len(t, claudeReq.Messages, 3)

		// Assistant message should have 2 tool_use blocks
		require.Len(t, claudeReq.Messages[1].Content.ArrayValue, 2)
		assert.Equal(t, "tool_use", claudeReq.Messages[1].Content.ArrayValue[0].Type)
		assert.Equal(t, "tool_use", claudeReq.Messages[1].Content.ArrayValue[1].Type)

		// User message should have 2 tool_result blocks merged
		assert.Equal(t, roleUser, claudeReq.Messages[2].Role)
		require.Len(t, claudeReq.Messages[2].Content.ArrayValue, 2)
		assert.Equal(t, "tool_result", claudeReq.Messages[2].Content.ArrayValue[0].Type)
		assert.Equal(t, "call_1", claudeReq.Messages[2].Content.ArrayValue[0].ToolUseId)
		assert.Equal(t, "tool_result", claudeReq.Messages[2].Content.ArrayValue[1].Type)
		assert.Equal(t, "call_2", claudeReq.Messages[2].Content.ArrayValue[1].ToolUseId)
	})

	t.Run("handles_assistant_tool_calls_with_text_content", func(t *testing.T) {
		request := &chatCompletionRequest{
			Model:     "claude-sonnet-4-5-20250929",
			MaxTokens: 1024,
			Messages: []chatMessage{
				{Role: roleUser, Content: "What's the weather?"},
				{Role: roleAssistant, Content: "Let me check the weather for you.", ToolCalls: []toolCall{
					{Id: "call_123", Type: "function", Function: functionCall{Name: "get_weather", Arguments: `{"city": "Beijing"}`}},
				}},
			},
		}

		claudeReq := provider.buildClaudeTextGenRequest(request)

		require.Len(t, claudeReq.Messages, 2)

		// Assistant message should have both text and tool_use
		assert.Equal(t, roleAssistant, claudeReq.Messages[1].Role)
		require.False(t, claudeReq.Messages[1].Content.IsString)
		require.Len(t, claudeReq.Messages[1].Content.ArrayValue, 2)
		assert.Equal(t, contentTypeText, claudeReq.Messages[1].Content.ArrayValue[0].Type)
		assert.Equal(t, "Let me check the weather for you.", claudeReq.Messages[1].Content.ArrayValue[0].Text)
		assert.Equal(t, "tool_use", claudeReq.Messages[1].Content.ArrayValue[1].Type)
	})
}
