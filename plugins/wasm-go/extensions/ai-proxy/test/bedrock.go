package test

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"strings"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

// Test config: Basic Bedrock config with AWS Access Key/Secret Key (AWS Signature V4)
var basicBedrockConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":         "bedrock",
			"awsAccessKey": "test-ak-for-unit-test",
			"awsSecretKey": "test-sk-for-unit-test",
			"awsRegion":    "us-east-1",
			"modelMapping": map[string]string{
				"*": "anthropic.claude-3-5-haiku-20241022-v1:0",
			},
		},
	})
	return data
}()

// Test config: Bedrock original protocol config with AWS Access Key/Secret Key
var bedrockOriginalAkSkConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":         "bedrock",
			"protocol":     "original",
			"awsAccessKey": "test-ak-for-unit-test",
			"awsSecretKey": "test-sk-for-unit-test",
			"awsRegion":    "us-east-1",
		},
	})
	return data
}()

// Test config: Bedrock original protocol config with api token
var bedrockOriginalApiTokenConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":      "bedrock",
			"protocol":  "original",
			"awsRegion": "us-east-1",
			"apiTokens": []string{
				"test-token-for-unit-test",
			},
		},
	})
	return data
}()

// Test config: Bedrock original protocol config with AWS Access Key/Secret Key and custom settings
var bedrockOriginalAkSkWithCustomSettingsConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":         "bedrock",
			"protocol":     "original",
			"awsAccessKey": "test-ak-for-unit-test",
			"awsSecretKey": "test-sk-for-unit-test",
			"awsRegion":    "us-east-1",
			"customSettings": []map[string]interface{}{
				{
					"name":      "foo",
					"value":     "\"bar\"",
					"mode":      "raw",
					"overwrite": true,
				},
			},
		},
	})
	return data
}()

// Test config: Bedrock config with embeddings capability to verify generic SigV4 flow
var bedrockEmbeddingsCapabilityConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":         "bedrock",
			"awsAccessKey": "test-ak-for-unit-test",
			"awsSecretKey": "test-sk-for-unit-test",
			"awsRegion":    "us-east-1",
			"capabilities": map[string]string{
				"openai/v1/embeddings": "/model/amazon.titan-embed-text-v2:0/invoke",
			},
			"modelMapping": map[string]string{
				"*": "amazon.titan-embed-text-v2:0",
			},
		},
	})
	return data
}()

// Test config: Bedrock config with Bearer Token authentication
var bedrockApiTokenConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type": "bedrock",
			"apiTokens": []string{
				"test-token-for-unit-test",
			},
			"awsRegion": "us-east-1",
			"modelMapping": map[string]string{
				"*": "anthropic.claude-3-5-haiku-20241022-v1:0",
			},
		},
	})
	return data
}()

func bedrockApiTokenConfigWithCachePointPositions(positions map[string]bool) json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type": "bedrock",
			"apiTokens": []string{
				"test-token-for-unit-test",
			},
			"awsRegion": "us-east-1",
			"modelMapping": map[string]string{
				"*": "anthropic.claude-3-5-haiku-20241022-v1:0",
			},
			"bedrockPromptCachePointPositions": positions,
		},
	})
	return data
}

func bedrockApiTokenConfigWithPromptCacheRetention(promptCacheRetention string) json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type": "bedrock",
			"apiTokens": []string{
				"test-token-for-unit-test",
			},
			"awsRegion": "us-east-1",
			"modelMapping": map[string]string{
				"*": "anthropic.claude-3-5-haiku-20241022-v1:0",
			},
			"promptCacheRetention": promptCacheRetention,
		},
	})
	return data
}

func bedrockApiTokenConfigWithModelAndPromptCache(mappedModel, promptCacheRetention string, positions map[string]bool) json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type": "bedrock",
			"apiTokens": []string{
				"test-token-for-unit-test",
			},
			"awsRegion": "us-east-1",
			"modelMapping": map[string]string{
				"*": mappedModel,
			},
			"promptCacheRetention":             promptCacheRetention,
			"bedrockPromptCachePointPositions": positions,
		},
	})
	return data
}

// Test config: Bedrock config with multiple Bearer Tokens
var bedrockMultiTokenConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type": "bedrock",
			"apiTokens": []string{
				"test-token-1-for-unit-test",
				"test-token-2-for-unit-test",
			},
			"awsRegion": "us-west-2",
			"modelMapping": map[string]string{
				"gpt-4": "anthropic.claude-3-opus-20240229-v1:0",
				"*":     "anthropic.claude-3-haiku-20240307-v1:0",
			},
		},
	})
	return data
}()

// Test config: Bedrock config with additional fields
var bedrockWithAdditionalFieldsConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":         "bedrock",
			"awsAccessKey": "test-ak-for-unit-test",
			"awsSecretKey": "test-sk-for-unit-test",
			"awsRegion":    "us-east-1",
			"bedrockAdditionalFields": map[string]interface{}{
				"top_k": 200,
			},
			"modelMapping": map[string]string{
				"*": "anthropic.claude-3-5-haiku-20241022-v1:0",
			},
		},
	})
	return data
}()

// Test config: Invalid config - missing both apiTokens and ak/sk
var bedrockInvalidConfigMissingAuth = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":      "bedrock",
			"awsRegion": "us-east-1",
			"modelMapping": map[string]string{
				"*": "anthropic.claude-3-5-haiku-20241022-v1:0",
			},
		},
	})
	return data
}()

// Test config: Invalid config - missing region
var bedrockInvalidConfigMissingRegion = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type": "bedrock",
			"apiTokens": []string{
				"test-token-for-unit-test",
			},
			"modelMapping": map[string]string{
				"*": "anthropic.claude-3-5-haiku-20241022-v1:0",
			},
		},
	})
	return data
}()

// Test config: Invalid config - only has access key without secret key
var bedrockInvalidConfigPartialAkSk = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":         "bedrock",
			"awsAccessKey": "test-ak-for-unit-test",
			"awsRegion":    "us-east-1",
			"modelMapping": map[string]string{
				"*": "anthropic.claude-3-5-haiku-20241022-v1:0",
			},
		},
	})
	return data
}()

func RunBedrockParseConfigTests(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// Test basic Bedrock config with AWS Signature V4 authentication
		t.Run("basic bedrock config with ak/sk", func(t *testing.T) {
			host, status := test.NewTestHost(basicBedrockConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// Test Bedrock config with Bearer Token authentication
		t.Run("bedrock config with api token", func(t *testing.T) {
			host, status := test.NewTestHost(bedrockApiTokenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// Test Bedrock config with multiple tokens
		t.Run("bedrock config with multiple tokens", func(t *testing.T) {
			host, status := test.NewTestHost(bedrockMultiTokenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// Test Bedrock config with additional fields
		t.Run("bedrock config with additional fields", func(t *testing.T) {
			host, status := test.NewTestHost(bedrockWithAdditionalFieldsConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// Test invalid config - missing authentication
		t.Run("bedrock invalid config missing auth", func(t *testing.T) {
			host, status := test.NewTestHost(bedrockInvalidConfigMissingAuth)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})

		// Test invalid config - missing region
		t.Run("bedrock invalid config missing region", func(t *testing.T) {
			host, status := test.NewTestHost(bedrockInvalidConfigMissingRegion)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})

		// Test invalid config - partial ak/sk (only access key, no secret key)
		t.Run("bedrock invalid config partial ak/sk", func(t *testing.T) {
			host, status := test.NewTestHost(bedrockInvalidConfigPartialAkSk)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})
	})
}

func RunBedrockOnHttpRequestHeadersTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// Test Bedrock request headers processing with AWS Signature V4
		t.Run("bedrock chat completion request headers with ak/sk", func(t *testing.T) {
			host, status := test.NewTestHost(basicBedrockConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// Set request headers
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			require.Equal(t, types.HeaderStopIteration, action)

			// Verify request headers
			requestHeaders := host.GetRequestHeaders()
			require.NotNil(t, requestHeaders)

			// Verify Host is changed to Bedrock service domain
			hostValue, hasHost := test.GetHeaderValue(requestHeaders, ":authority")
			require.True(t, hasHost, "Host header should exist")
			require.Contains(t, hostValue, "bedrock-runtime.us-east-1.amazonaws.com", "Host should be changed to Bedrock service domain")
		})

		// Test Bedrock request headers processing with Bearer Token
		t.Run("bedrock chat completion request headers with api token", func(t *testing.T) {
			host, status := test.NewTestHost(bedrockApiTokenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// Set request headers
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			require.Equal(t, types.HeaderStopIteration, action)

			// Verify request headers
			requestHeaders := host.GetRequestHeaders()
			require.NotNil(t, requestHeaders)

			// Verify Host is changed to Bedrock service domain
			hostValue, hasHost := test.GetHeaderValue(requestHeaders, ":authority")
			require.True(t, hasHost, "Host header should exist")
			require.Contains(t, hostValue, "bedrock-runtime.us-east-1.amazonaws.com", "Host should be changed to Bedrock service domain")
		})
	})
}

func RunBedrockOnHttpRequestBodyTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// Test Bedrock request body processing with Bearer Token authentication
		t.Run("bedrock chat completion request body with api token", func(t *testing.T) {
			host, status := test.NewTestHost(bedrockApiTokenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// Set request headers
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			// Set request body
			requestBody := `{
				"model": "gpt-4",
				"messages": [
					{
						"role": "user",
						"content": "Hello, how are you?"
					}
				],
				"temperature": 0.7
			}`

			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			// Verify request headers for Bearer Token authentication
			requestHeaders := host.GetRequestHeaders()
			require.NotNil(t, requestHeaders)

			// Verify Authorization header uses Bearer token
			authValue, hasAuth := test.GetHeaderValue(requestHeaders, "Authorization")
			require.True(t, hasAuth, "Authorization header should exist")
			require.Contains(t, authValue, "Bearer ", "Authorization should use Bearer token")
			require.Contains(t, authValue, "test-token-for-unit-test", "Authorization should contain the configured token")

			// Verify path is transformed to Bedrock format
			pathValue, hasPath := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, hasPath, "Path header should exist")
			require.Contains(t, pathValue, "/model/", "Path should contain Bedrock model path")
			require.Contains(t, pathValue, "/converse", "Path should contain converse endpoint")
		})

		t.Run("bedrock request body prompt cache in-memory should inject system cache point only by default", func(t *testing.T) {
			host, status := test.NewTestHost(bedrockApiTokenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			requestBody := `{
				"model": "gpt-4",
				"prompt_cache_retention": "in-memory",
				"prompt_cache_key": "session-001",
				"messages": [
					{
						"role": "system",
						"content": "You are a helpful assistant."
					},
					{
						"role": "user",
						"content": "Hello"
					}
				]
			}`
			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)

			var bodyMap map[string]interface{}
			err := json.Unmarshal(processedBody, &bodyMap)
			require.NoError(t, err)

			_, hasPromptCacheRetention := bodyMap["prompt_cache_retention"]
			require.False(t, hasPromptCacheRetention, "prompt_cache_retention should not be forwarded to Bedrock")
			_, hasPromptCacheKey := bodyMap["prompt_cache_key"]
			require.False(t, hasPromptCacheKey, "prompt_cache_key should not be forwarded to Bedrock")

			systemBlocks, ok := bodyMap["system"].([]interface{})
			require.True(t, ok, "system should be an array")
			require.Len(t, systemBlocks, 2, "system should contain text block and cachePoint block")
			systemCachePointBlock := systemBlocks[len(systemBlocks)-1].(map[string]interface{})
			systemCachePoint, ok := systemCachePointBlock["cachePoint"].(map[string]interface{})
			require.True(t, ok, "system tail block should contain cachePoint")
			require.Equal(t, "default", systemCachePoint["type"])
			_, hasTTL := systemCachePoint["ttl"]
			require.False(t, hasTTL, "ttl should be omitted for in_memory to use Bedrock default 5m")

			messages := bodyMap["messages"].([]interface{})
			require.NotEmpty(t, messages, "messages should not be empty")
			lastMessage := messages[len(messages)-1].(map[string]interface{})
			lastMessageContent := lastMessage["content"].([]interface{})
			require.Len(t, lastMessageContent, 1, "last message should keep original content only by default")
			_, hasMessageCachePoint := lastMessageContent[0].(map[string]interface{})["cachePoint"]
			require.False(t, hasMessageCachePoint, "last message should not include cachePoint by default")
		})

		t.Run("bedrock request body should use provider promptCacheRetention in-memory when request omits prompt_cache_retention", func(t *testing.T) {
			host, status := test.NewTestHost(bedrockApiTokenConfigWithPromptCacheRetention("in-memory"))
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			requestBody := `{
				"model": "gpt-4",
				"messages": [
					{
						"role": "system",
						"content": "You are a helpful assistant."
					},
					{
						"role": "user",
						"content": "Hello"
					}
				]
			}`
			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)

			var bodyMap map[string]interface{}
			err := json.Unmarshal(processedBody, &bodyMap)
			require.NoError(t, err)

			systemBlocks := bodyMap["system"].([]interface{})
			require.Len(t, systemBlocks, 2, "provider promptCacheRetention should trigger cachePoint injection")
			systemCachePoint := systemBlocks[len(systemBlocks)-1].(map[string]interface{})["cachePoint"].(map[string]interface{})
			_, hasTTL := systemCachePoint["ttl"]
			require.False(t, hasTTL, "provider promptCacheRetention=in-memory should omit ttl and use Bedrock default 5m")
		})

		t.Run("bedrock request body prompt_cache_retention should override provider promptCacheRetention", func(t *testing.T) {
			host, status := test.NewTestHost(bedrockApiTokenConfigWithPromptCacheRetention("in_memory"))
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			requestBody := `{
				"model": "gpt-4",
				"prompt_cache_retention": "24h",
				"messages": [
					{
						"role": "system",
						"content": "You are a helpful assistant."
					},
					{
						"role": "user",
						"content": "Hello"
					}
				]
			}`
			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)

			var bodyMap map[string]interface{}
			err := json.Unmarshal(processedBody, &bodyMap)
			require.NoError(t, err)

			systemBlocks := bodyMap["system"].([]interface{})
			systemCachePoint := systemBlocks[len(systemBlocks)-1].(map[string]interface{})["cachePoint"].(map[string]interface{})
			require.Equal(t, "1h", systemCachePoint["ttl"], "request prompt_cache_retention should override provider promptCacheRetention")
		})

		t.Run("bedrock request body prompt cache 24h should map to 1h ttl on system cache point by default", func(t *testing.T) {
			host, status := test.NewTestHost(bedrockApiTokenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			requestBody := `{
				"model": "gpt-4",
				"prompt_cache_retention": "24h",
				"messages": [
					{
						"role": "system",
						"content": "You are a helpful assistant."
					},
					{
						"role": "user",
						"content": "Hello"
					}
				]
			}`
			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)

			var bodyMap map[string]interface{}
			err := json.Unmarshal(processedBody, &bodyMap)
			require.NoError(t, err)

			systemBlocks := bodyMap["system"].([]interface{})
			systemCachePointBlock := systemBlocks[len(systemBlocks)-1].(map[string]interface{})
			systemCachePoint := systemCachePointBlock["cachePoint"].(map[string]interface{})
			require.Equal(t, "1h", systemCachePoint["ttl"])

			messages := bodyMap["messages"].([]interface{})
			lastMessage := messages[len(messages)-1].(map[string]interface{})
			lastMessageContent := lastMessage["content"].([]interface{})
			require.Len(t, lastMessageContent, 1, "last message should keep original content only by default")
			_, hasMessageCachePoint := lastMessageContent[0].(map[string]interface{})["cachePoint"]
			require.False(t, hasMessageCachePoint, "last message should not include cachePoint by default")
		})

		t.Run("bedrock request body prompt cache should insert cache points based on configured positions", func(t *testing.T) {
			host, status := test.NewTestHost(bedrockApiTokenConfigWithCachePointPositions(map[string]bool{
				"systemPrompt":    true,
				"lastUserMessage": true,
				"lastMessage":     false,
			}))
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			requestBody := `{
				"model": "gpt-4",
				"prompt_cache_retention": "in_memory",
				"messages": [
					{
						"role": "system",
						"content": "You are a helpful assistant."
					},
					{
						"role": "user",
						"content": "Question from user"
					},
					{
						"role": "assistant",
						"content": "Previous assistant answer"
					}
				]
			}`
			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)

			var bodyMap map[string]interface{}
			err := json.Unmarshal(processedBody, &bodyMap)
			require.NoError(t, err)

			systemBlocks := bodyMap["system"].([]interface{})
			require.Len(t, systemBlocks, 2, "system should include cachePoint due to systemPrompt=true")
			systemCachePoint := systemBlocks[len(systemBlocks)-1].(map[string]interface{})["cachePoint"].(map[string]interface{})
			_, hasSystemTTL := systemCachePoint["ttl"]
			require.False(t, hasSystemTTL, "ttl should be omitted for in_memory cachePoint")

			messages := bodyMap["messages"].([]interface{})
			require.Len(t, messages, 2, "system message should not be in messages array")

			lastUserMessageContent := messages[0].(map[string]interface{})["content"].([]interface{})
			require.Len(t, lastUserMessageContent, 2, "last user message should include one cachePoint")
			lastUserMessageCachePoint := lastUserMessageContent[len(lastUserMessageContent)-1].(map[string]interface{})["cachePoint"].(map[string]interface{})
			_, hasLastUserTTL := lastUserMessageCachePoint["ttl"]
			require.False(t, hasLastUserTTL, "ttl should be omitted for in_memory cachePoint")

			lastMessageContent := messages[1].(map[string]interface{})["content"].([]interface{})
			require.Len(t, lastMessageContent, 1, "last message should not include cachePoint when lastMessage=false")
		})

		t.Run("bedrock request body prompt cache should avoid duplicate insertion when lastUserMessage and lastMessage overlap", func(t *testing.T) {
			host, status := test.NewTestHost(bedrockApiTokenConfigWithCachePointPositions(map[string]bool{
				"systemPrompt":    false,
				"lastUserMessage": true,
				"lastMessage":     true,
			}))
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			requestBody := `{
				"model": "gpt-4",
				"prompt_cache_retention": "in_memory",
				"messages": [
					{
						"role": "user",
						"content": "Hello"
					}
				]
			}`
			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)

			var bodyMap map[string]interface{}
			err := json.Unmarshal(processedBody, &bodyMap)
			require.NoError(t, err)

			_, hasSystem := bodyMap["system"]
			require.False(t, hasSystem, "system should not include cachePoint when systemPrompt=false and no system messages")

			messages := bodyMap["messages"].([]interface{})
			require.Len(t, messages, 1, "only one message should exist")
			messageContent := messages[0].(map[string]interface{})["content"].([]interface{})
			require.Len(t, messageContent, 2, "overlap positions should still insert only one cachePoint")
			cachePoint := messageContent[len(messageContent)-1].(map[string]interface{})["cachePoint"].(map[string]interface{})
			_, hasTTL := cachePoint["ttl"]
			require.False(t, hasTTL, "ttl should be omitted for in_memory cachePoint")
		})

		t.Run("bedrock request body with empty prompt cache retention should not inject cache points", func(t *testing.T) {
			host, status := test.NewTestHost(bedrockApiTokenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			requestBody := `{
				"model": "gpt-4",
				"prompt_cache_retention": "",
				"messages": [
					{
						"role": "system",
						"content": "You are a helpful assistant."
					},
					{
						"role": "user",
						"content": "Hello"
					}
				]
			}`
			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)

			var bodyMap map[string]interface{}
			err := json.Unmarshal(processedBody, &bodyMap)
			require.NoError(t, err)

			systemBlocks := bodyMap["system"].([]interface{})
			require.Len(t, systemBlocks, 1, "system should only contain the original text block")
			_, hasSystemCachePoint := systemBlocks[0].(map[string]interface{})["cachePoint"]
			require.False(t, hasSystemCachePoint, "system block should not include cachePoint when retention is empty")

			messages := bodyMap["messages"].([]interface{})
			lastMessage := messages[len(messages)-1].(map[string]interface{})
			lastMessageContent := lastMessage["content"].([]interface{})
			require.Len(t, lastMessageContent, 1, "message should only contain original text block")
			_, hasMessageCachePoint := lastMessageContent[0].(map[string]interface{})["cachePoint"]
			require.False(t, hasMessageCachePoint, "message block should not include cachePoint when retention is empty")
		})

		t.Run("bedrock request body with unsupported prompt cache retention should not inject cache points", func(t *testing.T) {
			host, status := test.NewTestHost(bedrockApiTokenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			requestBody := `{
				"model": "gpt-4",
				"prompt_cache_retention": "2h",
				"messages": [
					{
						"role": "system",
						"content": "You are a helpful assistant."
					},
					{
						"role": "user",
						"content": "Hello"
					}
				]
			}`
			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)

			var bodyMap map[string]interface{}
			err := json.Unmarshal(processedBody, &bodyMap)
			require.NoError(t, err)

			systemBlocks := bodyMap["system"].([]interface{})
			require.Len(t, systemBlocks, 1, "system should only contain the original text block")
			_, hasSystemCachePoint := systemBlocks[0].(map[string]interface{})["cachePoint"]
			require.False(t, hasSystemCachePoint, "system block should not include cachePoint when retention is unsupported")

			messages := bodyMap["messages"].([]interface{})
			lastMessage := messages[len(messages)-1].(map[string]interface{})
			lastMessageContent := lastMessage["content"].([]interface{})
			require.Len(t, lastMessageContent, 1, "message should only contain original text block")
			_, hasMessageCachePoint := lastMessageContent[0].(map[string]interface{})["cachePoint"]
			require.False(t, hasMessageCachePoint, "message block should not include cachePoint when retention is unsupported")
		})

		t.Run("bedrock request body should skip prompt cache for unsupported model even when enabled", func(t *testing.T) {
			host, status := test.NewTestHost(bedrockApiTokenConfigWithModelAndPromptCache(
				"meta.llama3-70b-instruct-v1:0",
				"in_memory",
				map[string]bool{
					"systemPrompt": true,
					"lastMessage":  true,
				},
			))
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			requestBody := `{
				"model": "gpt-4",
				"prompt_cache_retention": "24h",
				"messages": [
					{
						"role": "system",
						"content": "You are a helpful assistant."
					},
					{
						"role": "user",
						"content": "Hello"
					}
				]
			}`
			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)

			var bodyMap map[string]interface{}
			err := json.Unmarshal(processedBody, &bodyMap)
			require.NoError(t, err)

			systemBlocks := bodyMap["system"].([]interface{})
			require.Len(t, systemBlocks, 1, "unsupported model should skip system cachePoint injection")
			_, hasSystemCachePoint := systemBlocks[0].(map[string]interface{})["cachePoint"]
			require.False(t, hasSystemCachePoint, "unsupported model should not contain system cachePoint")

			messages := bodyMap["messages"].([]interface{})
			require.Len(t, messages, 1, "system message should not be in messages array")
			lastMessageContent := messages[0].(map[string]interface{})["content"].([]interface{})
			require.Len(t, lastMessageContent, 1, "unsupported model should skip message cachePoint injection")
			_, hasMessageCachePoint := lastMessageContent[0].(map[string]interface{})["cachePoint"]
			require.False(t, hasMessageCachePoint, "unsupported model should not contain message cachePoint")
		})

		t.Run("bedrock request body without system should not inject cache point by default", func(t *testing.T) {
			host, status := test.NewTestHost(bedrockApiTokenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			requestBody := `{
				"model": "gpt-4",
				"prompt_cache_retention": "in_memory",
				"messages": [
					{
						"role": "user",
						"content": "Hello"
					}
				]
			}`
			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)

			var bodyMap map[string]interface{}
			err := json.Unmarshal(processedBody, &bodyMap)
			require.NoError(t, err)

			_, hasSystem := bodyMap["system"]
			require.False(t, hasSystem, "system should be omitted when original request has no system prompts")

			messages := bodyMap["messages"].([]interface{})
			require.Len(t, messages, 1, "messages should keep original one user message")
			lastMessage := messages[0].(map[string]interface{})
			lastMessageContent := lastMessage["content"].([]interface{})
			require.Len(t, lastMessageContent, 1, "message should keep original text block only by default")
			_, hasMessageCachePoint := lastMessageContent[0].(map[string]interface{})["cachePoint"]
			require.False(t, hasMessageCachePoint, "message should not include cachePoint by default")
		})

		// Test Bedrock request body processing with AWS Signature V4 authentication
		t.Run("bedrock chat completion request body with ak/sk", func(t *testing.T) {
			host, status := test.NewTestHost(basicBedrockConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// Set request headers
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			// Set request body
			requestBody := `{
				"model": "gpt-4",
				"messages": [
					{
						"role": "user",
						"content": "Hello, how are you?"
					}
				],
				"temperature": 0.7
			}`

			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			// Verify request headers for AWS Signature V4 authentication
			requestHeaders := host.GetRequestHeaders()
			require.NotNil(t, requestHeaders)

			// Verify Authorization header uses AWS Signature
			authValue, hasAuth := test.GetHeaderValue(requestHeaders, "Authorization")
			require.True(t, hasAuth, "Authorization header should exist")
			require.Contains(t, authValue, "AWS4-HMAC-SHA256", "Authorization should use AWS4-HMAC-SHA256 signature")
			require.Contains(t, authValue, "Credential=", "Authorization should contain Credential")
			require.Contains(t, authValue, "Signature=", "Authorization should contain Signature")

			// Verify X-Amz-Date header exists
			dateValue, hasDate := test.GetHeaderValue(requestHeaders, "X-Amz-Date")
			require.True(t, hasDate, "X-Amz-Date header should exist for AWS Signature V4")
			require.NotEmpty(t, dateValue, "X-Amz-Date should not be empty")

			// Verify path is transformed to Bedrock format
			pathValue, hasPath := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, hasPath, "Path header should exist")
			require.Contains(t, pathValue, "/model/", "Path should contain Bedrock model path")
			require.Contains(t, pathValue, "/converse", "Path should contain converse endpoint")
		})

		// Test Bedrock generic request body processing with AWS Signature V4 authentication
		t.Run("bedrock embeddings request body with ak/sk should use sigv4", func(t *testing.T) {
			host, status := test.NewTestHost(bedrockEmbeddingsCapabilityConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/embeddings"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			requestBody := `{
				"model": "text-embedding-3-small",
				"input": "Hello from embeddings"
			}`
			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			requestHeaders := host.GetRequestHeaders()
			require.NotNil(t, requestHeaders)

			authValue, hasAuth := test.GetHeaderValue(requestHeaders, "Authorization")
			require.True(t, hasAuth, "Authorization header should exist")
			require.Contains(t, authValue, "AWS4-HMAC-SHA256", "Authorization should use AWS4-HMAC-SHA256 signature")
			require.Contains(t, authValue, "Credential=", "Authorization should contain Credential")
			require.Contains(t, authValue, "Signature=", "Authorization should contain Signature")

			dateValue, hasDate := test.GetHeaderValue(requestHeaders, "X-Amz-Date")
			require.True(t, hasDate, "X-Amz-Date header should exist for AWS Signature V4")
			require.NotEmpty(t, dateValue, "X-Amz-Date should not be empty")
		})

		// Test Bedrock original converse-stream path with AWS Signature V4 authentication
		t.Run("bedrock original converse-stream with ak/sk should use sigv4", func(t *testing.T) {
			host, status := test.NewTestHost(bedrockOriginalAkSkConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			originalPath := "/model/anthropic.claude-3-5-haiku-20241022-v1%3A0/converse-stream"
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", originalPath},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			requestBody := `{
				"messages": [
					{
						"role": "user",
						"content": [{"text": "Hello from original bedrock path"}]
					}
				],
				"inferenceConfig": {
					"maxTokens": 64
				}
			}`
			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			requestHeaders := host.GetRequestHeaders()
			require.NotNil(t, requestHeaders)

			authValue, hasAuth := test.GetHeaderValue(requestHeaders, "Authorization")
			require.True(t, hasAuth, "Authorization header should exist")
			require.Contains(t, authValue, "AWS4-HMAC-SHA256", "Authorization should use AWS4-HMAC-SHA256 signature")
			require.Contains(t, authValue, "Credential=", "Authorization should contain Credential")
			require.Contains(t, authValue, "Signature=", "Authorization should contain Signature")

			dateValue, hasDate := test.GetHeaderValue(requestHeaders, "X-Amz-Date")
			require.True(t, hasDate, "X-Amz-Date header should exist for AWS Signature V4")
			require.NotEmpty(t, dateValue, "X-Amz-Date should not be empty")

			pathValue, hasPath := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, hasPath, "Path header should exist")
			require.Equal(t, originalPath, pathValue, "Original Bedrock path should be kept unchanged")
		})

		// Test Bedrock original converse-stream path with Bearer Token authentication
		t.Run("bedrock original converse-stream with api token should pass bearer auth", func(t *testing.T) {
			host, status := test.NewTestHost(bedrockOriginalApiTokenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			originalPath := "/model/anthropic.claude-3-5-haiku-20241022-v1%3A0/converse-stream"
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", originalPath},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			requestBody := `{
				"messages": [
					{
						"role": "user",
						"content": [{"text": "Hello from original bedrock path"}]
					}
				]
			}`
			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			requestHeaders := host.GetRequestHeaders()
			require.NotNil(t, requestHeaders)

			authValue, hasAuth := test.GetHeaderValue(requestHeaders, "Authorization")
			require.True(t, hasAuth, "Authorization header should exist")
			require.Contains(t, authValue, "Bearer ", "Authorization should use Bearer token")
			require.Contains(t, authValue, "test-token-for-unit-test", "Authorization should contain configured token")

			_, hasDate := test.GetHeaderValue(requestHeaders, "X-Amz-Date")
			require.False(t, hasDate, "X-Amz-Date should not be set in Bearer token mode")

			pathValue, hasPath := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, hasPath, "Path header should exist")
			require.Equal(t, originalPath, pathValue, "Original Bedrock path should be kept unchanged")
		})

		// Test Bedrock original converse-stream path keeps signed body consistent with custom settings
		t.Run("bedrock original converse-stream with custom settings should replace body before forwarding", func(t *testing.T) {
			host, status := test.NewTestHost(bedrockOriginalAkSkWithCustomSettingsConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			originalPath := "/model/amazon.nova-2-lite-v1:0/converse-stream"
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", originalPath},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			requestBody := `{
				"messages": [
					{
						"role": "user",
						"content": [{"text": "Hello"}]
					}
				]
			}`
			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)

			var bodyMap map[string]interface{}
			err := json.Unmarshal(processedBody, &bodyMap)
			require.NoError(t, err)
			require.Equal(t, "\"bar\"", bodyMap["foo"], "Custom settings should be applied to forwarded body")

			authValue, hasAuth := test.GetHeaderValue(host.GetRequestHeaders(), "Authorization")
			require.True(t, hasAuth, "Authorization header should exist")
			require.Contains(t, authValue, "AWS4-HMAC-SHA256", "Authorization should use AWS4-HMAC-SHA256 signature")
		})

		// Test Bedrock streaming request
		t.Run("bedrock streaming request", func(t *testing.T) {
			host, status := test.NewTestHost(bedrockApiTokenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// Set request headers
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			// Set streaming request body
			requestBody := `{
				"model": "gpt-4",
				"messages": [
					{
						"role": "user",
						"content": "Hello"
					}
				],
				"stream": true
			}`

			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			// Verify path is transformed to Bedrock streaming format
			requestHeaders := host.GetRequestHeaders()
			pathValue, hasPath := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, hasPath, "Path header should exist")
			require.Contains(t, pathValue, "/model/", "Path should contain Bedrock model path")
			require.Contains(t, pathValue, "/converse-stream", "Path should contain converse-stream endpoint for streaming")
		})
	})
}

func RunBedrockOnHttpResponseHeadersTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// Test Bedrock response headers processing
		t.Run("bedrock response headers", func(t *testing.T) {
			host, status := test.NewTestHost(bedrockApiTokenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// Set request headers
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			// Set request body
			requestBody := `{
				"model": "gpt-4",
				"messages": [
					{
						"role": "user",
						"content": "Hello"
					}
				]
			}`
			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			// Process response headers
			action = host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"Content-Type", "application/json"},
				{"X-Amzn-Requestid", "test-request-id-12345"},
			})
			require.Equal(t, types.ActionContinue, action)

			// Verify response headers
			responseHeaders := host.GetResponseHeaders()
			require.NotNil(t, responseHeaders)

			// Verify status code
			statusValue, hasStatus := test.GetHeaderValue(responseHeaders, ":status")
			require.True(t, hasStatus, "Status header should exist")
			require.Equal(t, "200", statusValue, "Status should be 200")
		})
	})
}

func RunBedrockToolCallTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// Test single tool call conversion (regression test)
		t.Run("bedrock single tool call conversion", func(t *testing.T) {
			host, status := test.NewTestHost(bedrockApiTokenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			requestBody := `{
				"model": "gpt-4",
				"messages": [
					{"role": "user", "content": "What is the weather in Beijing?"},
					{"role": "assistant", "content": "Let me check the weather for you.", "tool_calls": [{"id": "call_001", "type": "function", "function": {"name": "get_weather", "arguments": "{\"city\":\"Beijing\"}"}}]},
					{"role": "tool", "content": "Sunny, 25°C", "tool_call_id": "call_001"}
				],
				"tools": [{"type": "function", "function": {"name": "get_weather", "description": "Get weather info", "parameters": {"type": "object", "properties": {"city": {"type": "string"}}}}}]
			}`

			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)

			var bodyMap map[string]interface{}
			err := json.Unmarshal(processedBody, &bodyMap)
			require.NoError(t, err)

			messages := bodyMap["messages"].([]interface{})
			// messages[0] = user, messages[1] = assistant with toolUse, messages[2] = user with toolResult
			require.Len(t, messages, 3, "Should have 3 messages: user, assistant, user(toolResult)")

			// Verify assistant message has exactly 1 toolUse
			assistantMsg := messages[1].(map[string]interface{})
			require.Equal(t, "assistant", assistantMsg["role"])
			assistantContent := assistantMsg["content"].([]interface{})
			require.Len(t, assistantContent, 1, "Assistant should have 1 content block")
			toolUseBlock := assistantContent[0].(map[string]interface{})
			require.Contains(t, toolUseBlock, "toolUse", "Content block should contain toolUse")

			// Verify tool result message
			toolResultMsg := messages[2].(map[string]interface{})
			require.Equal(t, "user", toolResultMsg["role"])
			toolResultContent := toolResultMsg["content"].([]interface{})
			require.Len(t, toolResultContent, 1, "Tool result message should have 1 content block")
			require.Contains(t, toolResultContent[0].(map[string]interface{}), "toolResult", "Content block should contain toolResult")
		})

		// Test multiple parallel tool calls conversion
		t.Run("bedrock multiple parallel tool calls conversion", func(t *testing.T) {
			host, status := test.NewTestHost(bedrockApiTokenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			requestBody := `{
				"model": "gpt-4",
				"messages": [
					{"role": "user", "content": "What is the weather in Beijing and Shanghai?"},
					{"role": "assistant", "content": "Let me check both cities.", "tool_calls": [{"id": "call_001", "type": "function", "function": {"name": "get_weather", "arguments": "{\"city\":\"Beijing\"}"}}, {"id": "call_002", "type": "function", "function": {"name": "get_weather", "arguments": "{\"city\":\"Shanghai\"}"}}]},
					{"role": "tool", "content": "Sunny, 25°C", "tool_call_id": "call_001"},
					{"role": "tool", "content": "Cloudy, 22°C", "tool_call_id": "call_002"}
				],
				"tools": [{"type": "function", "function": {"name": "get_weather", "description": "Get weather info", "parameters": {"type": "object", "properties": {"city": {"type": "string"}}}}}]
			}`

			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)

			var bodyMap map[string]interface{}
			err := json.Unmarshal(processedBody, &bodyMap)
			require.NoError(t, err)

			messages := bodyMap["messages"].([]interface{})
			// messages[0] = user, messages[1] = assistant with 2 toolUse, messages[2] = user with 2 toolResult
			require.Len(t, messages, 3, "Should have 3 messages: user, assistant, user(toolResults merged)")

			// Verify assistant message has 2 toolUse blocks
			assistantMsg := messages[1].(map[string]interface{})
			require.Equal(t, "assistant", assistantMsg["role"])
			assistantContent := assistantMsg["content"].([]interface{})
			require.Len(t, assistantContent, 2, "Assistant should have 2 content blocks for parallel tool calls")

			firstToolUse := assistantContent[0].(map[string]interface{})["toolUse"].(map[string]interface{})
			require.Equal(t, "get_weather", firstToolUse["name"])
			require.Equal(t, "call_001", firstToolUse["toolUseId"])

			secondToolUse := assistantContent[1].(map[string]interface{})["toolUse"].(map[string]interface{})
			require.Equal(t, "get_weather", secondToolUse["name"])
			require.Equal(t, "call_002", secondToolUse["toolUseId"])

			// Verify tool results are merged into a single user message
			toolResultMsg := messages[2].(map[string]interface{})
			require.Equal(t, "user", toolResultMsg["role"])
			toolResultContent := toolResultMsg["content"].([]interface{})
			require.Len(t, toolResultContent, 2, "Tool results should be merged into 2 content blocks in one user message")

			firstResult := toolResultContent[0].(map[string]interface{})["toolResult"].(map[string]interface{})
			require.Equal(t, "call_001", firstResult["toolUseId"])

			secondResult := toolResultContent[1].(map[string]interface{})["toolResult"].(map[string]interface{})
			require.Equal(t, "call_002", secondResult["toolUseId"])
		})

		t.Run("bedrock maps any tool choice to converse any tool choice", func(t *testing.T) {
			host, status := test.NewTestHost(bedrockApiTokenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			requestBody := `{
				"model": "gpt-4",
				"messages": [{"role": "user", "content": "Run a search."}],
				"tool_choice": "any",
				"tools": [{
					"type": "function",
					"function": {
						"name": "web_search",
						"description": "Search the web.",
						"parameters": {
							"type": "object",
							"properties": {"query": {"type": "string"}},
							"required": ["query"]
						}
					}
				}]
			}`

			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)

			var bodyMap map[string]interface{}
			err := json.Unmarshal(processedBody, &bodyMap)
			require.NoError(t, err)

			toolConfig := bodyMap["toolConfig"].(map[string]interface{})
			toolChoice := toolConfig["toolChoice"].(map[string]interface{})
			require.Contains(t, toolChoice, "any")
			require.Equal(t, map[string]interface{}{}, toolChoice["any"])
		})

		t.Run("bedrock maps object tool choice to converse tool choice", func(t *testing.T) {
			host, status := test.NewTestHost(bedrockApiTokenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			requestBody := `{
				"model": "gpt-4",
				"messages": [{"role": "user", "content": "Run a search."}],
				"tool_choice": {"type":"function","function":{"name":"web_search"}},
				"tools": [{
					"type": "function",
					"function": {
						"name": "web_search",
						"description": "Search the web.",
						"parameters": {
							"type": "object",
							"properties": {"query": {"type": "string"}},
							"required": ["query"]
						}
					}
				}]
			}`

			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)

			var bodyMap map[string]interface{}
			err := json.Unmarshal(processedBody, &bodyMap)
			require.NoError(t, err)

			toolConfig := bodyMap["toolConfig"].(map[string]interface{})
			toolChoice := toolConfig["toolChoice"].(map[string]interface{})
			tool := toolChoice["tool"].(map[string]interface{})
			require.Equal(t, "web_search", tool["name"])
			require.Len(t, tool, 1, "Bedrock specific tool choice should only include name")
		})

		t.Run("bedrock maps object any tool choice to converse any tool choice", func(t *testing.T) {
			host, status := test.NewTestHost(bedrockApiTokenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			requestBody := `{
				"model": "gpt-4",
				"messages": [{"role": "user", "content": "Run a search."}],
				"tool_choice": {"type":"any"},
				"tools": [{
					"type": "function",
					"function": {
						"name": "web_search",
						"description": "Search the web.",
						"parameters": {"type": "object"}
					}
				}]
			}`

			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			var bodyMap map[string]interface{}
			err := json.Unmarshal(host.GetRequestBody(), &bodyMap)
			require.NoError(t, err)

			toolConfig := bodyMap["toolConfig"].(map[string]interface{})
			toolChoice := toolConfig["toolChoice"].(map[string]interface{})
			require.Contains(t, toolChoice, "any")
			require.NotContains(t, toolChoice, "tool")
		})

		t.Run("bedrock maps none tool choice by omitting tools", func(t *testing.T) {
			host, status := test.NewTestHost(bedrockApiTokenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			requestBody := `{
				"model": "gpt-4",
				"messages": [{"role": "user", "content": "Answer without tools."}],
				"tool_choice": {"type":"none"},
				"tools": [{
					"type": "function",
					"function": {
						"name": "web_search",
						"description": "Search the web.",
						"parameters": {"type": "object"}
					}
				}]
			}`

			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			var bodyMap map[string]interface{}
			err := json.Unmarshal(host.GetRequestBody(), &bodyMap)
			require.NoError(t, err)
			require.NotContains(t, bodyMap, "toolConfig")
		})

		// Test tool call with text content mixed
		t.Run("bedrock tool call with text content mixed", func(t *testing.T) {
			host, status := test.NewTestHost(bedrockApiTokenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			requestBody := `{
				"model": "gpt-4",
				"messages": [
					{"role": "user", "content": "What is the weather in Beijing?"},
					{"role": "assistant", "content": "Let me check.", "tool_calls": [{"id": "call_001", "type": "function", "function": {"name": "get_weather", "arguments": "{\"city\":\"Beijing\"}"}}]},
					{"role": "tool", "content": "Sunny, 25°C", "tool_call_id": "call_001"},
					{"role": "assistant", "content": "The weather in Beijing is sunny with 25°C."},
					{"role": "user", "content": "Thanks!"}
				],
				"tools": [{"type": "function", "function": {"name": "get_weather", "description": "Get weather info", "parameters": {"type": "object", "properties": {"city": {"type": "string"}}}}}]
			}`

			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)

			var bodyMap map[string]interface{}
			err := json.Unmarshal(processedBody, &bodyMap)
			require.NoError(t, err)

			messages := bodyMap["messages"].([]interface{})
			// messages[0] = user, messages[1] = assistant(toolUse), messages[2] = user(toolResult),
			// messages[3] = assistant(text), messages[4] = user(text)
			require.Len(t, messages, 5, "Should have 5 messages in mixed tool call and text scenario")

			// Verify message roles alternate correctly
			require.Equal(t, "user", messages[0].(map[string]interface{})["role"])
			require.Equal(t, "assistant", messages[1].(map[string]interface{})["role"])
			require.Equal(t, "user", messages[2].(map[string]interface{})["role"])
			require.Equal(t, "assistant", messages[3].(map[string]interface{})["role"])
			require.Equal(t, "user", messages[4].(map[string]interface{})["role"])

			// Verify assistant text message (messages[3]) has text content
			assistantTextMsg := messages[3].(map[string]interface{})
			assistantTextContent := assistantTextMsg["content"].([]interface{})
			require.Len(t, assistantTextContent, 1)
			require.Contains(t, assistantTextContent[0].(map[string]interface{}), "text", "Text assistant message should have text content")
			require.Contains(t, assistantTextContent[0].(map[string]interface{})["text"], "sunny", "Text content should contain weather info")
		})
	})
}

func RunBedrockOnHttpResponseBodyTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// Test Bedrock response body processing
		t.Run("bedrock response body", func(t *testing.T) {
			host, status := test.NewTestHost(bedrockApiTokenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// Set request headers
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			// Set request body
			requestBody := `{
				"model": "gpt-4",
				"messages": [
					{
						"role": "user",
						"content": "Hello"
					}
				]
			}`
			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			// Set response property to ensure IsResponseFromUpstream() returns true
			host.SetProperty([]string{"response", "code_details"}, []byte("via_upstream"))

			// Process response headers (must include :status 200 for body processing)
			action = host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.ActionContinue, action)

			// Process response body (Bedrock format)
			responseBody := `{
				"output": {
					"message": {
						"role": "assistant",
						"content": [
							{
								"text": "Hello! How can I help you today?"
							}
						]
					}
				},
				"stopReason": "end_turn",
				"usage": {
					"inputTokens": 10,
					"outputTokens": 15,
					"totalTokens": 25,
					"cacheReadInputTokens": 6,
					"cacheWriteInputTokens": 12
				}
			}`

			action = host.CallOnHttpResponseBody([]byte(responseBody))
			require.Equal(t, types.ActionContinue, action)

			// Verify response body is transformed to OpenAI format
			transformedResponseBody := host.GetResponseBody()
			require.NotNil(t, transformedResponseBody)

			var responseMap map[string]interface{}
			err := json.Unmarshal(transformedResponseBody, &responseMap)
			require.NoError(t, err)

			// Verify choices exist in transformed response
			choices, exists := responseMap["choices"]
			require.True(t, exists, "Choices should exist in response body")
			require.NotNil(t, choices, "Choices should not be nil")

			// Verify usage exists
			usage, exists := responseMap["usage"]
			require.True(t, exists, "Usage should exist in response body")
			require.NotNil(t, usage, "Usage should not be nil")
			usageMap := usage.(map[string]interface{})
			promptTokensDetails, hasPromptTokensDetails := usageMap["prompt_tokens_details"].(map[string]interface{})
			require.True(t, hasPromptTokensDetails, "prompt_tokens_details should exist when cacheReadInputTokens is present")
			require.Equal(t, float64(18), promptTokensDetails["cached_tokens"], "cached_tokens should sum cacheReadInputTokens and cacheWriteInputTokens")
			_, hasCacheWriteTokens := promptTokensDetails["cache_write_tokens"]
			require.False(t, hasCacheWriteTokens, "cache_write_tokens should not exist in OpenAI-compatible usage")
		})

		t.Run("bedrock response body with zero cache read tokens should omit prompt_tokens_details", func(t *testing.T) {
			host, status := test.NewTestHost(bedrockApiTokenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			requestBody := `{
				"model": "gpt-4",
				"messages": [
					{
						"role": "user",
						"content": "Hello"
					}
				]
			}`
			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			host.SetProperty([]string{"response", "code_details"}, []byte("via_upstream"))

			action = host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.ActionContinue, action)

			responseBody := `{
				"output": {
					"message": {
						"role": "assistant",
						"content": [
							{
								"text": "Hello! How can I help you today?"
							}
						]
					}
				},
				"stopReason": "end_turn",
				"usage": {
					"inputTokens": 10,
					"outputTokens": 15,
					"totalTokens": 25,
					"cacheReadInputTokens": 0
				}
			}`

			action = host.CallOnHttpResponseBody([]byte(responseBody))
			require.Equal(t, types.ActionContinue, action)

			transformedResponseBody := host.GetResponseBody()
			require.NotNil(t, transformedResponseBody)

			var responseMap map[string]interface{}
			err := json.Unmarshal(transformedResponseBody, &responseMap)
			require.NoError(t, err)

			usageMap := responseMap["usage"].(map[string]interface{})
			_, hasPromptTokensDetails := usageMap["prompt_tokens_details"]
			require.False(t, hasPromptTokensDetails, "prompt_tokens_details should be omitted when cacheReadInputTokens is zero")
		})

		t.Run("bedrock response body with only cache write tokens should map to cached_tokens", func(t *testing.T) {
			host, status := test.NewTestHost(bedrockApiTokenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			requestBody := `{
				"model": "gpt-4",
				"messages": [
					{
						"role": "user",
						"content": "Hello"
					}
				]
			}`
			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			host.SetProperty([]string{"response", "code_details"}, []byte("via_upstream"))
			action = host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.ActionContinue, action)

			responseBody := `{
				"output": {
					"message": {
						"role": "assistant",
						"content": [
							{
								"text": "Hello! How can I help you today?"
							}
						]
					}
				},
				"stopReason": "end_turn",
				"usage": {
					"inputTokens": 10,
					"outputTokens": 15,
					"totalTokens": 25,
					"cacheReadInputTokens": 0,
					"cacheWriteInputTokens": 9
				}
			}`
			action = host.CallOnHttpResponseBody([]byte(responseBody))
			require.Equal(t, types.ActionContinue, action)

			transformedResponseBody := host.GetResponseBody()
			require.NotNil(t, transformedResponseBody)

			var responseMap map[string]interface{}
			err := json.Unmarshal(transformedResponseBody, &responseMap)
			require.NoError(t, err)

			usageMap := responseMap["usage"].(map[string]interface{})
			promptTokensDetails, hasPromptTokensDetails := usageMap["prompt_tokens_details"].(map[string]interface{})
			require.True(t, hasPromptTokensDetails, "prompt_tokens_details should exist when cacheWriteInputTokens is present")
			require.Equal(t, float64(9), promptTokensDetails["cached_tokens"], "cached_tokens should map from cacheWriteInputTokens when cacheReadInputTokens is zero")
			_, hasCacheWriteTokens := promptTokensDetails["cache_write_tokens"]
			require.False(t, hasCacheWriteTokens, "cache_write_tokens should not exist in OpenAI-compatible usage")
		})
	})
}

func RunBedrockOnStreamingResponseBodyTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		extractFirstDataPayload := func(body []byte) string {
			for _, line := range strings.Split(string(body), "\n") {
				if strings.HasPrefix(line, "data: ") && line != "data: [DONE]" {
					return strings.TrimPrefix(line, "data: ")
				}
			}
			return ""
		}

		t.Run("extract first data payload should return empty when no data line", func(t *testing.T) {
			payload := extractFirstDataPayload([]byte("event: ping\n\n"))
			require.Equal(t, "", payload)
		})

		t.Run("bedrock streaming parallel tool calls should use dense OpenAI indexes", func(t *testing.T) {
			host, status := test.NewTestHost(bedrockApiTokenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			requestBody := `{
				"model": "gpt-4",
				"messages": [{"role": "user", "content": "Run three independent searches."}],
				"stream": true,
				"tools": [{
					"type": "function",
					"function": {
						"name": "web_search",
						"description": "Search the web.",
						"parameters": {
							"type": "object",
							"properties": {"query": {"type": "string"}},
							"required": ["query"]
						}
					}
				}]
			}`
			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			host.SetProperty([]string{"response", "code_details"}, []byte("via_upstream"))
			action = host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"Content-Type", "application/vnd.amazon.eventstream"},
			})
			require.Equal(t, types.ActionContinue, action)

			extractFirstToolCallIndex := func(body []byte) int {
				dataPayload := extractFirstDataPayload(body)
				require.NotEmpty(t, dataPayload, "streaming chunk should contain one SSE data payload")

				var responseMap map[string]interface{}
				err := json.Unmarshal([]byte(dataPayload), &responseMap)
				require.NoError(t, err)

				choices := responseMap["choices"].([]interface{})
				require.Len(t, choices, 1, "streaming chunk should contain one choice")
				delta := choices[0].(map[string]interface{})["delta"].(map[string]interface{})
				toolCalls := delta["tool_calls"].([]interface{})
				require.Len(t, toolCalls, 1, "streaming chunk should contain one tool call delta")

				index, ok := toolCalls[0].(map[string]interface{})["index"].(float64)
				require.True(t, ok, "tool call delta should include an index")
				return int(index)
			}
			extractFirstToolCall := func(body []byte) map[string]interface{} {
				dataPayload := extractFirstDataPayload(body)
				require.NotEmpty(t, dataPayload, "streaming chunk should contain one SSE data payload")

				var responseMap map[string]interface{}
				err := json.Unmarshal([]byte(dataPayload), &responseMap)
				require.NoError(t, err)

				choices := responseMap["choices"].([]interface{})
				require.Len(t, choices, 1, "streaming chunk should contain one choice")
				delta := choices[0].(map[string]interface{})["delta"].(map[string]interface{})
				toolCalls := delta["tool_calls"].([]interface{})
				require.Len(t, toolCalls, 1, "streaming chunk should contain one tool call delta")

				toolCall, ok := toolCalls[0].(map[string]interface{})
				require.True(t, ok, "tool call delta should be an object")
				return toolCall
			}

			for expectedIndex, item := range []struct {
				contentBlockIndex int
				toolUseId         string
			}{
				{contentBlockIndex: 1, toolUseId: "tooluse_first"},
				{contentBlockIndex: 3, toolUseId: "tooluse_second"},
				{contentBlockIndex: 4, toolUseId: "tooluse_third"},
			} {
				toolCallStart := buildBedrockEventStreamMessage(t, map[string]interface{}{
					"contentBlockIndex": item.contentBlockIndex,
					"start": map[string]interface{}{
						"toolUse": map[string]interface{}{
							"toolUseId": item.toolUseId,
							"name":      "web_search",
						},
					},
				})
				action = host.CallOnHttpStreamingResponseBody(toolCallStart, false)
				require.Equal(t, types.ActionContinue, action)
				toolCall := extractFirstToolCall(host.GetResponseBody())
				require.Equal(t, expectedIndex, extractFirstToolCallIndex(host.GetResponseBody()))
				require.Equal(t, item.toolUseId, toolCall["id"])
				require.Equal(t, "function", toolCall["type"])
				function := toolCall["function"].(map[string]interface{})
				require.Equal(t, "web_search", function["name"])
				require.Equal(t, "", function["arguments"])
			}

			for expectedIndex, item := range []struct {
				contentBlockIndex int
				query             string
			}{
				{contentBlockIndex: 1, query: "first synthetic query"},
				{contentBlockIndex: 3, query: "second synthetic query"},
				{contentBlockIndex: 4, query: "third synthetic query"},
			} {
				toolCallDelta := buildBedrockEventStreamMessage(t, map[string]interface{}{
					"contentBlockIndex": item.contentBlockIndex,
					"delta": map[string]interface{}{
						"toolUse": map[string]interface{}{
							"input": "{\"query\":\"" + item.query + "\"}",
						},
					},
				})
				action = host.CallOnHttpStreamingResponseBody(toolCallDelta, false)
				require.Equal(t, types.ActionContinue, action)
				toolCall := extractFirstToolCall(host.GetResponseBody())
				require.Equal(t, expectedIndex, extractFirstToolCallIndex(host.GetResponseBody()))
				require.NotContains(t, toolCall, "id")
				function := toolCall["function"].(map[string]interface{})
				require.NotContains(t, function, "name")
				require.Equal(t, "{\"query\":\""+item.query+"\"}", function["arguments"])
			}
		})

		t.Run("bedrock streaming usage should map cached_tokens", func(t *testing.T) {
			host, status := test.NewTestHost(bedrockApiTokenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			requestBody := `{
				"model": "gpt-4",
				"messages": [
					{
						"role": "user",
						"content": "Hello"
					}
				],
				"stream": true
			}`
			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			host.SetProperty([]string{"response", "code_details"}, []byte("via_upstream"))

			action = host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"Content-Type", "application/vnd.amazon.eventstream"},
			})
			require.Equal(t, types.ActionContinue, action)

			streamingChunk := buildBedrockEventStreamMessage(t, map[string]interface{}{
				"usage": map[string]interface{}{
					"inputTokens":           10,
					"outputTokens":          2,
					"totalTokens":           12,
					"cacheReadInputTokens":  7,
					"cacheWriteInputTokens": 3,
				},
			})
			action = host.CallOnHttpStreamingResponseBody(streamingChunk, true)
			require.Equal(t, types.ActionContinue, action)

			transformedResponseBody := host.GetResponseBody()
			require.NotNil(t, transformedResponseBody)

			var dataPayload string
			for _, line := range strings.Split(string(transformedResponseBody), "\n") {
				if strings.HasPrefix(line, "data: ") && line != "data: [DONE]" {
					dataPayload = strings.TrimPrefix(line, "data: ")
					break
				}
			}
			require.NotEmpty(t, dataPayload, "should have at least one SSE data payload")

			var responseMap map[string]interface{}
			err := json.Unmarshal([]byte(dataPayload), &responseMap)
			require.NoError(t, err)
			usageMap := responseMap["usage"].(map[string]interface{})
			promptTokensDetails := usageMap["prompt_tokens_details"].(map[string]interface{})
			require.Equal(t, float64(10), promptTokensDetails["cached_tokens"], "cached_tokens should sum cacheReadInputTokens and cacheWriteInputTokens in streaming usage event")
			_, hasCacheWriteTokens := promptTokensDetails["cache_write_tokens"]
			require.False(t, hasCacheWriteTokens, "cache_write_tokens should not exist in OpenAI-compatible streaming usage")
		})

		t.Run("bedrock streaming text chunk then usage chunk format is stable", func(t *testing.T) {
			host, status := test.NewTestHost(bedrockApiTokenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			requestBody := `{
				"model": "gpt-4",
				"messages": [
					{
						"role": "user",
						"content": "Hello"
					}
				],
				"stream": true
			}`
			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			host.SetProperty([]string{"response", "code_details"}, []byte("via_upstream"))
			action = host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"Content-Type", "application/vnd.amazon.eventstream"},
			})
			require.Equal(t, types.ActionContinue, action)

			textChunk := buildBedrockEventStreamMessage(t, map[string]interface{}{
				"delta": map[string]interface{}{
					"text": "Hello from Bedrock",
				},
			})
			action = host.CallOnHttpStreamingResponseBody(textChunk, false)
			require.Equal(t, types.ActionContinue, action)

			firstResponseBody := host.GetResponseBody()
			require.NotNil(t, firstResponseBody)
			firstDataPayload := extractFirstDataPayload(firstResponseBody)
			require.NotEmpty(t, firstDataPayload, "first chunk should contain one SSE data payload")

			var firstResponseMap map[string]interface{}
			err := json.Unmarshal([]byte(firstDataPayload), &firstResponseMap)
			require.NoError(t, err)
			firstChoices := firstResponseMap["choices"].([]interface{})
			require.Len(t, firstChoices, 1, "text chunk should contain one choice")

			usageChunk := buildBedrockEventStreamMessage(t, map[string]interface{}{
				"usage": map[string]interface{}{
					"inputTokens":  10,
					"outputTokens": 2,
					"totalTokens":  12,
				},
			})
			action = host.CallOnHttpStreamingResponseBody(usageChunk, true)
			require.Equal(t, types.ActionContinue, action)

			secondResponseBody := host.GetResponseBody()
			require.NotNil(t, secondResponseBody)
			require.Contains(t, string(secondResponseBody), "data: [DONE]", "last chunk should append [DONE]")
			secondDataPayload := extractFirstDataPayload(secondResponseBody)
			require.NotEmpty(t, secondDataPayload, "usage chunk should contain one SSE data payload")

			var secondResponseMap map[string]interface{}
			err = json.Unmarshal([]byte(secondDataPayload), &secondResponseMap)
			require.NoError(t, err)
			secondChoices := secondResponseMap["choices"].([]interface{})
			require.Len(t, secondChoices, 0, "usage chunk should contain empty choices by design")
			_, hasUsage := secondResponseMap["usage"]
			require.True(t, hasUsage, "usage chunk should include usage field")
		})

		t.Run("bedrock empty intermediate callback should not affect next usage event", func(t *testing.T) {
			host, status := test.NewTestHost(bedrockApiTokenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			requestBody := `{
				"model": "gpt-4",
				"messages": [
					{
						"role": "user",
						"content": "Hello"
					}
				],
				"stream": true
			}`
			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			host.SetProperty([]string{"response", "code_details"}, []byte("via_upstream"))
			action = host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"Content-Type", "application/vnd.amazon.eventstream"},
			})
			require.Equal(t, types.ActionContinue, action)

			action = host.CallOnHttpStreamingResponseBody([]byte{}, false)
			require.Equal(t, types.ActionContinue, action)
			emptyResponseBody := host.GetResponseBody()
			require.Equal(t, 0, len(emptyResponseBody), "empty intermediate callback should output empty payload")

			usageChunk := buildBedrockEventStreamMessage(t, map[string]interface{}{
				"usage": map[string]interface{}{
					"inputTokens":  10,
					"outputTokens": 2,
					"totalTokens":  12,
				},
			})
			action = host.CallOnHttpStreamingResponseBody(usageChunk, true)
			require.Equal(t, types.ActionContinue, action)

			finalResponseBody := host.GetResponseBody()
			require.NotNil(t, finalResponseBody)
			require.Contains(t, string(finalResponseBody), "data: [DONE]", "last chunk should append [DONE]")
			finalDataPayload := extractFirstDataPayload(finalResponseBody)
			require.NotEmpty(t, finalDataPayload, "final usage event should still be parsed")

			var finalResponseMap map[string]interface{}
			err := json.Unmarshal([]byte(finalDataPayload), &finalResponseMap)
			require.NoError(t, err)
			finalChoices := finalResponseMap["choices"].([]interface{})
			require.Len(t, finalChoices, 0, "usage chunk should still keep empty choices")
		})
	})
}

func buildBedrockEventStreamMessage(t *testing.T, payload map[string]interface{}) []byte {
	payloadBytes, err := json.Marshal(payload)
	require.NoError(t, err)

	totalLength := uint32(16 + len(payloadBytes))
	headersLength := uint32(0)

	var message bytes.Buffer
	prelude := make([]byte, 8)
	binary.BigEndian.PutUint32(prelude[0:4], totalLength)
	binary.BigEndian.PutUint32(prelude[4:8], headersLength)
	message.Write(prelude)

	preludeCRC := crc32.ChecksumIEEE(prelude)
	preludeCRCBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(preludeCRCBytes, preludeCRC)
	message.Write(preludeCRCBytes)

	message.Write(payloadBytes)

	messageCRC := crc32.ChecksumIEEE(message.Bytes())
	messageCRCBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(messageCRCBytes, messageCRC)
	message.Write(messageCRCBytes)

	return message.Bytes()
}
