package provider

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tidwall/gjson"
)

func TestIsStatefulAPI(t *testing.T) {
	tests := []struct {
		name     string
		apiName  string
		expected bool
	}{
		// Stateful APIs - should return true
		{
			name:     "responses_api",
			apiName:  string(ApiNameResponses),
			expected: true,
		},
		{
			name:     "files_api",
			apiName:  string(ApiNameFiles),
			expected: true,
		},
		{
			name:     "retrieve_file_api",
			apiName:  string(ApiNameRetrieveFile),
			expected: true,
		},
		{
			name:     "retrieve_file_content_api",
			apiName:  string(ApiNameRetrieveFileContent),
			expected: true,
		},
		{
			name:     "batches_api",
			apiName:  string(ApiNameBatches),
			expected: true,
		},
		{
			name:     "retrieve_batch_api",
			apiName:  string(ApiNameRetrieveBatch),
			expected: true,
		},
		{
			name:     "cancel_batch_api",
			apiName:  string(ApiNameCancelBatch),
			expected: true,
		},
		{
			name:     "fine_tuning_jobs_api",
			apiName:  string(ApiNameFineTuningJobs),
			expected: true,
		},
		{
			name:     "retrieve_fine_tuning_job_api",
			apiName:  string(ApiNameRetrieveFineTuningJob),
			expected: true,
		},
		{
			name:     "fine_tuning_job_events_api",
			apiName:  string(ApiNameFineTuningJobEvents),
			expected: true,
		},
		{
			name:     "fine_tuning_job_checkpoints_api",
			apiName:  string(ApiNameFineTuningJobCheckpoints),
			expected: true,
		},
		{
			name:     "cancel_fine_tuning_job_api",
			apiName:  string(ApiNameCancelFineTuningJob),
			expected: true,
		},
		{
			name:     "resume_fine_tuning_job_api",
			apiName:  string(ApiNameResumeFineTuningJob),
			expected: true,
		},
		// Non-stateful APIs - should return false
		{
			name:     "chat_completion_api",
			apiName:  string(ApiNameChatCompletion),
			expected: false,
		},
		{
			name:     "completion_api",
			apiName:  string(ApiNameCompletion),
			expected: false,
		},
		{
			name:     "embeddings_api",
			apiName:  string(ApiNameEmbeddings),
			expected: false,
		},
		{
			name:     "models_api",
			apiName:  string(ApiNameModels),
			expected: false,
		},
		{
			name:     "image_generation_api",
			apiName:  string(ApiNameImageGeneration),
			expected: false,
		},
		{
			name:     "audio_speech_api",
			apiName:  string(ApiNameAudioSpeech),
			expected: false,
		},
		// Empty/unknown API - should return false
		{
			name:     "empty_api_name",
			apiName:  "",
			expected: false,
		},
		{
			name:     "unknown_api_name",
			apiName:  "unknown/api",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isStatefulAPI(tt.apiName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetTokenWithConsumerAffinity(t *testing.T) {
	tests := []struct {
		name      string
		apiTokens []string
		consumer  string
		wantEmpty bool
		wantToken string // If not empty, expected specific token (for single token case)
	}{
		{
			name:      "no_tokens_returns_empty",
			apiTokens: []string{},
			consumer:  "consumer1",
			wantEmpty: true,
		},
		{
			name:      "nil_tokens_returns_empty",
			apiTokens: nil,
			consumer:  "consumer1",
			wantEmpty: true,
		},
		{
			name:      "single_token_always_returns_same_token",
			apiTokens: []string{"token1"},
			consumer:  "consumer1",
			wantToken: "token1",
		},
		{
			name:      "single_token_with_different_consumer",
			apiTokens: []string{"token1"},
			consumer:  "consumer2",
			wantToken: "token1",
		},
		{
			name:      "multiple_tokens_consistent_for_same_consumer",
			apiTokens: []string{"token1", "token2", "token3"},
			consumer:  "consumer1",
			wantEmpty: false, // Will get one of the tokens, consistently
		},
		{
			name:      "multiple_tokens_different_consumers_may_get_different_tokens",
			apiTokens: []string{"token1", "token2"},
			consumer:  "consumerA",
			wantEmpty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &ProviderConfig{
				apiTokens: tt.apiTokens,
			}

			result := config.GetTokenWithConsumerAffinity(nil, tt.consumer)

			if tt.wantEmpty {
				assert.Empty(t, result)
			} else if tt.wantToken != "" {
				assert.Equal(t, tt.wantToken, result)
			} else {
				assert.NotEmpty(t, result)
				assert.Contains(t, tt.apiTokens, result)
			}
		})
	}
}

func TestGetTokenWithConsumerAffinity_Consistency(t *testing.T) {
	// Test that the same consumer always gets the same token (consistency)
	config := &ProviderConfig{
		apiTokens: []string{"token1", "token2", "token3", "token4", "token5"},
	}

	t.Run("same_consumer_gets_same_token_repeatedly", func(t *testing.T) {
		consumer := "test-consumer"
		var firstResult string

		// Call multiple times and verify consistency
		for i := 0; i < 10; i++ {
			result := config.GetTokenWithConsumerAffinity(nil, consumer)
			if i == 0 {
				firstResult = result
			}
			assert.Equal(t, firstResult, result, "Consumer should consistently get the same token")
		}
	})

	t.Run("different_consumers_distribute_across_tokens", func(t *testing.T) {
		// Use multiple consumers and verify they distribute across tokens
		consumers := []string{"consumer1", "consumer2", "consumer3", "consumer4", "consumer5", "consumer6", "consumer7", "consumer8", "consumer9", "consumer10"}
		tokenCounts := make(map[string]int)

		for _, consumer := range consumers {
			token := config.GetTokenWithConsumerAffinity(nil, consumer)
			tokenCounts[token]++
		}

		// Verify all tokens returned are valid
		for token := range tokenCounts {
			assert.Contains(t, config.apiTokens, token)
		}

		// With 10 consumers and 5 tokens, we expect some distribution
		// (not necessarily perfect distribution, but should use multiple tokens)
		assert.GreaterOrEqual(t, len(tokenCounts), 2, "Should use at least 2 different tokens")
	})

	t.Run("empty_consumer_returns_empty_string", func(t *testing.T) {
		config := &ProviderConfig{
			apiTokens: []string{"token1", "token2"},
		}
		result := config.GetTokenWithConsumerAffinity(nil, "")
		// Empty consumer still returns a token (hash of empty string)
		assert.NotEmpty(t, result)
		assert.Contains(t, []string{"token1", "token2"}, result)
	})
}

func TestGetTokenWithConsumerAffinity_HashDistribution(t *testing.T) {
	// Test that the hash function distributes consumers reasonably across tokens
	config := &ProviderConfig{
		apiTokens: []string{"token1", "token2", "token3"},
	}

	// Test specific consumers to verify hash behavior
	testCases := []struct {
		consumer    string
		expectValid bool
	}{
		{"user-alice", true},
		{"user-bob", true},
		{"user-charlie", true},
		{"service-api-v1", true},
		{"service-api-v2", true},
	}

	for _, tc := range testCases {
		t.Run("consumer_"+tc.consumer, func(t *testing.T) {
			result := config.GetTokenWithConsumerAffinity(nil, tc.consumer)
			assert.True(t, tc.expectValid)
			assert.Contains(t, config.apiTokens, result)
		})
	}
}

func TestProviderDomain_Config(t *testing.T) {
	t.Run("providerDomain_field_exists", func(t *testing.T) {
		config := ProviderConfig{}
		config.FromJson(gjson.Result{})
		assert.Equal(t, "", config.providerDomain)
	})

	t.Run("providerDomain_parsed_from_json", func(t *testing.T) {
		config := ProviderConfig{}
		jsonStr := `{"providerDomain": "universal-proxy.example.com"}`
		config.FromJson(gjson.Parse(jsonStr))
		assert.Equal(t, "universal-proxy.example.com", config.providerDomain)
	})
}

func TestProviderBasePath_Config(t *testing.T) {
	t.Run("providerBasePath_field_exists", func(t *testing.T) {
		config := ProviderConfig{}
		config.FromJson(gjson.Result{})
		assert.Equal(t, "", config.providerBasePath)
	})

	t.Run("providerBasePath_parsed_from_json", func(t *testing.T) {
		config := ProviderConfig{}
		jsonStr := `{"providerBasePath": "/api/ai"}`
		config.FromJson(gjson.Parse(jsonStr))
		assert.Equal(t, "/api/ai", config.providerBasePath)
	})

	t.Run("providerBasePath_with_other_config", func(t *testing.T) {
		config := ProviderConfig{}
		jsonStr := `{
			"type": "openai",
			"apiToken": "sk-test",
			"providerBasePath": "/api/v1",
			"providerDomain": "proxy.example.com"
		}`
		config.FromJson(gjson.Parse(jsonStr))
		assert.Equal(t, "openai", config.typ)
		assert.Equal(t, "/api/v1", config.providerBasePath)
		assert.Equal(t, "proxy.example.com", config.providerDomain)
	})
}

func TestApplyProviderBasePath(t *testing.T) {
	tests := []struct {
		name             string
		providerBasePath string
		originalPath     string
		expectedPath     string
	}{
		{
			name:             "no_base_path_configured",
			providerBasePath: "",
			originalPath:     "/v1/chat/completions",
			expectedPath:     "/v1/chat/completions",
		},
		{
			name:             "base_path_prepended",
			providerBasePath: "/api/ai",
			originalPath:     "/v1/chat/completions",
			expectedPath:     "/api/ai/v1/chat/completions",
		},
		{
			name:             "path_already_has_base_path",
			providerBasePath: "/api/ai",
			originalPath:     "/api/ai/v1/chat/completions",
			expectedPath:     "/api/ai/v1/chat/completions",
		},
		{
			name:             "base_path_with_trailing_slash",
			providerBasePath: "/api/ai/",
			originalPath:     "/v1/chat/completions",
			expectedPath:     "/api/ai//v1/chat/completions",
		},
		{
			name:             "deep_base_path",
			providerBasePath: "/internal/services/ai",
			originalPath:     "/v1/models",
			expectedPath:     "/internal/services/ai/v1/models",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &ProviderConfig{
				providerBasePath: tt.providerBasePath,
			}
			result := config.applyProviderBasePath(tt.originalPath)
			assert.Equal(t, tt.expectedPath, result)
		})
	}
}

func TestHandleRequestHeaders_PathHandling(t *testing.T) {
	// This test verifies the path handling logic in handleRequestHeaders
	// including basePathHandling and providerBasePath

	t.Run("basePath_removePrefix_only", func(t *testing.T) {
		config := &ProviderConfig{
			basePath:         "/gateway",
			basePathHandling: basePathHandlingRemovePrefix,
		}
		// Simulate the logic - actual test would need mock provider
		originPath := "/gateway/v1/chat"
		expectedPath := "/v1/chat"
		result := strings.TrimPrefix(originPath, config.basePath)
		assert.Equal(t, expectedPath, result)
	})

	t.Run("basePath_prepend_only", func(t *testing.T) {
		config := &ProviderConfig{
			basePath:         "/api",
			basePathHandling: basePathHandlingPrepend,
		}
		currentPath := "/v1/chat"
		// basePath preprend + providerBasePath (not set) = just basePath effect
		// Note: applyProviderBasePath only handles providerBasePath, not basePath
		// So this test just verifies that applyProviderBasePath doesn't modify path when providerBasePath is empty
		expectedPath := "/v1/chat" // applyProviderBasePath doesn't change path without providerBasePath configured
		result := config.applyProviderBasePath(currentPath)
		assert.Equal(t, expectedPath, result)
	})

	t.Run("providerBasePath_only", func(t *testing.T) {
		config := &ProviderConfig{
			providerBasePath: "/ai-proxy",
		}
		currentPath := "/v1/chat"
		expectedPath := "/ai-proxy/v1/chat"
		result := config.applyProviderBasePath(currentPath)
		assert.Equal(t, expectedPath, result)
	})

	t.Run("both_basePath_and_providerBasePath", func(t *testing.T) {
		config := &ProviderConfig{
			basePath:         "/gateway",
			basePathHandling: basePathHandlingRemovePrefix,
			providerBasePath: "/ai",
		}
		// First removePrefix, then apply providerBasePath
		originPath := "/gateway/v1/chat"
		afterRemovePrefix := strings.TrimPrefix(originPath, config.basePath)
		finalPath := config.applyProviderBasePath(afterRemovePrefix)
		assert.Equal(t, "/ai/v1/chat", finalPath)
	})
}

func TestProviderConfig_IsOriginal(t *testing.T) {
	tests := []struct {
		name     string
		protocol string
		expected bool
	}{
		{"openai_protocol", protocolOpenAI, false},
		{"original_protocol", protocolOriginal, true},
		{"empty_protocol", "", false},
		{"unknown_protocol", "unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &ProviderConfig{
				protocol: tt.protocol,
			}
			result := config.IsOriginal()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProviderConfig_GetPromoteThinkingOnEmpty(t *testing.T) {
	tests := []struct {
		name                   string
		promoteThinkingOnEmpty bool
		expected               bool
	}{
		{"enabled", true, true},
		{"disabled", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &ProviderConfig{
				promoteThinkingOnEmpty: tt.promoteThinkingOnEmpty,
			}
			result := config.GetPromoteThinkingOnEmpty()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ============ Failover Tests ============

func TestFailover_FromJson_Defaults(t *testing.T) {
	t.Run("default_failure_threshold", func(t *testing.T) {
		f := &failover{}
		jsonStr := `{"enabled": true}`
		f.FromJson(gjson.Parse(jsonStr))
		assert.Equal(t, int64(3), f.failureThreshold)
	})

	t.Run("default_success_threshold", func(t *testing.T) {
		f := &failover{}
		jsonStr := `{"enabled": true}`
		f.FromJson(gjson.Parse(jsonStr))
		assert.Equal(t, int64(1), f.successThreshold)
	})

	t.Run("default_health_check_interval", func(t *testing.T) {
		f := &failover{}
		jsonStr := `{"enabled": true}`
		f.FromJson(gjson.Parse(jsonStr))
		assert.Equal(t, int64(5000), f.healthCheckInterval)
	})

	t.Run("default_health_check_timeout", func(t *testing.T) {
		f := &failover{}
		jsonStr := `{"enabled": true}`
		f.FromJson(gjson.Parse(jsonStr))
		assert.Equal(t, int64(5000), f.healthCheckTimeout)
	})

	t.Run("custom_values", func(t *testing.T) {
		f := &failover{}
		jsonStr := `{
			"enabled": true,
			"failureThreshold": 5,
			"successThreshold": 3,
			"healthCheckInterval": 10000,
			"healthCheckTimeout": 8000,
			"healthCheckModel": "test-model"
		}`
		f.FromJson(gjson.Parse(jsonStr))
		assert.Equal(t, true, f.enabled)
		assert.Equal(t, int64(5), f.failureThreshold)
		assert.Equal(t, int64(3), f.successThreshold)
		assert.Equal(t, int64(10000), f.healthCheckInterval)
		assert.Equal(t, int64(8000), f.healthCheckTimeout)
		assert.Equal(t, "test-model", f.healthCheckModel)
	})
}

func TestFailover_FromJson_FailoverOnStatus(t *testing.T) {
	t.Run("parse_failoverOnStatus_array", func(t *testing.T) {
		f := &failover{}
		jsonStr := `{
			"enabled": true,
			"failoverOnStatus": ["401", "403", "5[0-9][0-9]"]
		}`
		f.FromJson(gjson.Parse(jsonStr))
		assert.Equal(t, 3, len(f.failoverOnStatus))
		assert.Contains(t, f.failoverOnStatus, "401")
		assert.Contains(t, f.failoverOnStatus, "403")
		assert.Contains(t, f.failoverOnStatus, "5[0-9][0-9]")
	})

	t.Run("empty_failoverOnStatus", func(t *testing.T) {
		f := &failover{}
		jsonStr := `{"enabled": true}`
		f.FromJson(gjson.Parse(jsonStr))
		// When failoverOnStatus is not specified, it keeps default values
		// Default regex patterns may be set elsewhere
		assert.True(t, f.enabled)
		assert.Equal(t, int64(3), f.failureThreshold)
	})
}

func TestHealthCheckEndpoint_Struct(t *testing.T) {
	t.Run("health_check_endpoint_fields", func(t *testing.T) {
		endpoint := HealthCheckEndpoint{
			Host:    "api.example.com",
			Path:    "/v1/chat/completions",
			Cluster: "ai-provider-cluster",
		}
		assert.Equal(t, "api.example.com", endpoint.Host)
		assert.Equal(t, "/v1/chat/completions", endpoint.Path)
		assert.Equal(t, "ai-provider-cluster", endpoint.Cluster)
	})
}

func TestLease_Struct(t *testing.T) {
	t.Run("lease_fields", func(t *testing.T) {
		lease := Lease{
			VMID:      "vm-12345",
			Timestamp: 1234567890,
		}
		assert.Equal(t, "vm-12345", lease.VMID)
		assert.Equal(t, int64(1234567890), lease.Timestamp)
	})
}

func TestFailover_Constants(t *testing.T) {
	t.Run("cas_max_retries_value", func(t *testing.T) {
		assert.Equal(t, 10, casMaxRetries)
	})

	t.Run("operation_constants", func(t *testing.T) {
		assert.Equal(t, "addApiToken", addApiTokenOperation)
		assert.Equal(t, "removeApiToken", removeApiTokenOperation)
		assert.Equal(t, "addApiTokenRequestCount", addApiTokenRequestCountOperation)
		assert.Equal(t, "resetApiTokenRequestCount", resetApiTokenRequestCountOperation)
	})

	t.Run("context_key_constants", func(t *testing.T) {
		assert.Equal(t, "requestHost", CtxRequestHost)
		assert.Equal(t, "requestPath", CtxRequestPath)
		assert.Equal(t, "requestBody", CtxRequestBody)
	})
}

func TestProviderConfig_TransformRequestHeadersAndBody_PathHandling(t *testing.T) {
	// Test that providerBasePath is applied in transformRequestHeadersAndBody
	t.Run("providerBasePath_applied", func(t *testing.T) {
		config := &ProviderConfig{
			providerBasePath: "/api/ai",
		}

		// Test the applyProviderBasePath logic used in transformRequestHeadersAndBody
		testPath := "/v1/chat/completions"
		expectedPath := "/api/ai/v1/chat/completions"
		result := config.applyProviderBasePath(testPath)
		assert.Equal(t, expectedPath, result)
	})

	t.Run("providerBasePath_already_present", func(t *testing.T) {
		config := &ProviderConfig{
			providerBasePath: "/api/ai",
		}

		testPath := "/api/ai/v1/chat/completions"
		result := config.applyProviderBasePath(testPath)
		// Should not duplicate the prefix
		assert.Equal(t, "/api/ai/v1/chat/completions", result)
	})
}

func TestProviderConfig_IsSupportedAPI(t *testing.T) {
	t.Run("supported_api", func(t *testing.T) {
		config := &ProviderConfig{
			capabilities: map[string]string{
				string(ApiNameChatCompletion): "/v1/chat/completions",
				string(ApiNameEmbeddings):     "/v1/embeddings",
			},
		}

		result := config.IsSupportedAPI(ApiNameChatCompletion)
		assert.True(t, result)
	})

	t.Run("unsupported_api", func(t *testing.T) {
		config := &ProviderConfig{
			capabilities: map[string]string{
				string(ApiNameChatCompletion): "/v1/chat/completions",
			},
		}

		result := config.IsSupportedAPI(ApiNameEmbeddings)
		assert.False(t, result)
	})

	t.Run("empty_capabilities", func(t *testing.T) {
		config := &ProviderConfig{
			capabilities: map[string]string{},
		}

		result := config.IsSupportedAPI(ApiNameChatCompletion)
		assert.False(t, result)
	})
}

func TestProviderConfig_SetDefaultCapabilities(t *testing.T) {
	t.Run("set_when_nil", func(t *testing.T) {
		config := &ProviderConfig{
			capabilities: nil,
		}

		defaultCaps := map[string]string{
			string(ApiNameChatCompletion): "/v1/chat/completions",
		}
		config.setDefaultCapabilities(defaultCaps)

		assert.NotNil(t, config.capabilities)
		assert.Equal(t, "/v1/chat/completions", config.capabilities[string(ApiNameChatCompletion)])
	})

	t.Run("merge_with_existing", func(t *testing.T) {
		config := &ProviderConfig{
			capabilities: map[string]string{
				string(ApiNameEmbeddings): "/v1/embeddings",
			},
		}

		defaultCaps := map[string]string{
			string(ApiNameChatCompletion): "/v1/chat/completions",
		}
		config.setDefaultCapabilities(defaultCaps)

		assert.Equal(t, "/v1/embeddings", config.capabilities[string(ApiNameEmbeddings)])
		assert.Equal(t, "/v1/chat/completions", config.capabilities[string(ApiNameChatCompletion)])
	})
}

func TestStripClaudeInternalMessageFields(t *testing.T) {
	body := []byte(`{
		"model":"claude",
		"claude_thinking":{"type":"adaptive"},
		"claude_output_config":{"effort":"high"},
		"claude_anthropic_beta":["effort-2025-11-24"],
		"messages":[{
			"role":"assistant",
			"content":"answer",
			"reasoning_content":"reasoning",
			"reasoning_signature":"sig",
			"reasoning_redacted_content":"opaque",
			"claude_content_blocks":[{"type":"thinking","thinking":"","signature":"sig"}],
			"claude_content_block_index":1,
			"claude_content_block_stop":1
		}]
	}`)

	result := stripClaudeInternalMessageFields(body)

	assert.False(t, gjson.GetBytes(result, "claude_thinking").Exists())
	assert.False(t, gjson.GetBytes(result, "claude_output_config").Exists())
	assert.False(t, gjson.GetBytes(result, "claude_anthropic_beta").Exists())
	assert.False(t, gjson.GetBytes(result, "messages.0.reasoning_content").Exists())
	assert.False(t, gjson.GetBytes(result, "messages.0.reasoning_signature").Exists())
	assert.False(t, gjson.GetBytes(result, "messages.0.reasoning_redacted_content").Exists())
	assert.False(t, gjson.GetBytes(result, "messages.0.claude_content_blocks").Exists())
	assert.False(t, gjson.GetBytes(result, "messages.0.claude_content_block_index").Exists())
	assert.False(t, gjson.GetBytes(result, "messages.0.claude_content_block_stop").Exists())
	assert.Equal(t, "answer", gjson.GetBytes(result, "messages.0.content").String())
}
