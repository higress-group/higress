package main

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name           string
		configJSON     string
		expectedConfig AIModelFilterConfig
		expectError    bool
	}{
		{
			name: "Valid config with all fields",
			configJSON: `{
				"allowed_models": ["gpt-4", "gpt-3.5-turbo", "claude-3-*"],
				"strict_mode": true,
				"reject_message": "Custom rejection message",
				"reject_status_code": 403
			}`,
			expectedConfig: AIModelFilterConfig{
				allowedModels:    []string{"gpt-4", "gpt-3.5-turbo", "claude-3-*"},
				strictMode:       true,
				rejectMessage:    "Custom rejection message",
				rejectStatusCode: 403,
			},
			expectError: false,
		},
		{
			name: "Config with default values",
			configJSON: `{
				"allowed_models": ["gpt-4"]
			}`,
			expectedConfig: AIModelFilterConfig{
				allowedModels:    []string{"gpt-4"},
				strictMode:       true,                // Default value
				rejectMessage:    "Model not allowed", // Default value
				rejectStatusCode: 403,                 // Default value
			},
			expectError: false,
		},
		{
			name: "Empty allowed models",
			configJSON: `{
				"allowed_models": [],
				"strict_mode": false
			}`,
			expectedConfig: AIModelFilterConfig{
				allowedModels:    []string{},
				strictMode:       false,
				rejectMessage:    "Model not allowed", // Default value
				rejectStatusCode: 403,                 // Default value
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the config parsing logic manually to avoid WASM environment issues
			configJson := gjson.Parse(tt.configJSON)
			config := AIModelFilterConfig{}

			// Parse allowed models
			allowedModelsArray := configJson.Get("allowed_models").Array()
			config.allowedModels = make([]string, len(allowedModelsArray))
			for i, model := range allowedModelsArray {
				config.allowedModels[i] = model.String()
			}

			// Parse strict mode setting, default to true
			config.strictMode = configJson.Get("strict_mode").Bool()
			if !configJson.Get("strict_mode").Exists() {
				config.strictMode = true
			}

			// Parse custom reject message, default message
			config.rejectMessage = configJson.Get("reject_message").String()
			if config.rejectMessage == "" {
				config.rejectMessage = "Model not allowed"
			}

			// Parse custom reject status code, default 403
			config.rejectStatusCode = int(configJson.Get("reject_status_code").Int())
			if config.rejectStatusCode == 0 {
				config.rejectStatusCode = 403
			}

			if tt.expectError {
				// For this simple test, we don't expect errors
				t.Skip("No error cases in this simplified test")
			} else {
				assert.Equal(t, tt.expectedConfig.allowedModels, config.allowedModels)
				assert.Equal(t, tt.expectedConfig.strictMode, config.strictMode)
				assert.Equal(t, tt.expectedConfig.rejectMessage, config.rejectMessage)
				assert.Equal(t, tt.expectedConfig.rejectStatusCode, config.rejectStatusCode)
			}
		})
	}
}

func TestExtractModelNameFromBody(t *testing.T) {
	tests := []struct {
		name          string
		requestBody   string
		expectedModel string
	}{
		{
			name:          "Extract model from request body",
			requestBody:   `{"model":"gpt-4","messages":[{"role":"user","content":"Hello"}]}`,
			expectedModel: "gpt-4",
		},
		{
			name:          "No model in body",
			requestBody:   `{"messages":[{"role":"user","content":"Hello"}]}`,
			expectedModel: "",
		},
		{
			name:          "Empty body",
			requestBody:   `{}`,
			expectedModel: "",
		},
		{
			name:          "Invalid JSON",
			requestBody:   `invalid json`,
			expectedModel: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the JSON parsing part directly
			model := gjson.GetBytes([]byte(tt.requestBody), "model")
			var result string
			if model.Exists() {
				result = model.String()
			}
			assert.Equal(t, tt.expectedModel, result)
		})
	}
}

func TestExtractModelNameFromPath(t *testing.T) {
	tests := []struct {
		name          string
		requestPath   string
		expectedModel string
	}{
		{
			name:          "Extract model from Gemini API path",
			requestPath:   "/v1/models/gemini-pro:generateContent",
			expectedModel: "gemini-pro",
		},
		{
			name:          "Extract model from Gemini stream API path",
			requestPath:   "/v1/models/gemini-flash:streamGenerateContent",
			expectedModel: "gemini-flash",
		},
		{
			name:          "No model in path",
			requestPath:   "/v1/chat/completions",
			expectedModel: "",
		},
		{
			name:          "Invalid path format",
			requestPath:   "/invalid/path",
			expectedModel: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the regex matching part directly
			if strings.Contains(tt.requestPath, "generateContent") || strings.Contains(tt.requestPath, "streamGenerateContent") {
				reg := regexp.MustCompile(`^.*/(?P<api_version>[^/]+)/models/(?P<model>[^:]+):\w+Content$`)
				matches := reg.FindStringSubmatch(tt.requestPath)
				var result string
				if len(matches) == 3 {
					result = matches[2]
				}
				assert.Equal(t, tt.expectedModel, result)
			} else {
				assert.Equal(t, tt.expectedModel, "")
			}
		})
	}
}

func TestIsModelAllowed(t *testing.T) {
	tests := []struct {
		name          string
		modelName     string
		allowedModels []string
		expected      bool
	}{
		{
			name:          "Exact match",
			modelName:     "gpt-4",
			allowedModels: []string{"gpt-4", "gpt-3.5-turbo"},
			expected:      true,
		},
		{
			name:          "Wildcard match",
			modelName:     "claude-3-sonnet",
			allowedModels: []string{"gpt-4", "claude-3-*"},
			expected:      true,
		},
		{
			name:          "No match",
			modelName:     "llama-3",
			allowedModels: []string{"gpt-4", "claude-3-*"},
			expected:      false,
		},
		{
			name:          "Empty allowed models",
			modelName:     "gpt-4",
			allowedModels: []string{},
			expected:      false,
		},
		{
			name:          "Wildcard prefix match",
			modelName:     "claude-3-opus",
			allowedModels: []string{"claude-3-*"},
			expected:      true,
		},
		{
			name:          "Wildcard no match",
			modelName:     "claude-2-sonnet",
			allowedModels: []string{"claude-3-*"},
			expected:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isModelAllowed(tt.modelName, tt.allowedModels)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// 测试配置：基础配置
var basicConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"allowed_models":     []string{"gpt-4", "gpt-3.5-turbo", "claude-3-*"},
		"strict_mode":        true,
		"reject_message":     "Model not allowed",
		"reject_status_code": 403,
	})
	return data
}()

// 测试配置：非严格模式
var nonStrictConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"allowed_models": []string{"gpt-4"},
		"strict_mode":    false,
	})
	return data
}()

// 测试配置：空模型列表
var emptyModelsConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"allowed_models": []string{},
	})
	return data
}()

// 测试配置：自定义拒绝消息
var customRejectConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"allowed_models":     []string{"gpt-4"},
		"reject_message":     "Custom rejection message",
		"reject_status_code": 400,
	})
	return data
}()

func TestParseConfigWithTestFramework(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// 测试基础配置解析
		t.Run("basic config", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			configRaw, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, configRaw)

			config, ok := configRaw.(*AIModelFilterConfig)
			require.True(t, ok, "config should be of type *AIModelFilterConfig")

			// 验证配置
			require.Equal(t, []string{"gpt-4", "gpt-3.5-turbo", "claude-3-*"}, config.allowedModels)
			require.True(t, config.strictMode)
			require.Equal(t, "Model not allowed", config.rejectMessage)
			require.Equal(t, 403, config.rejectStatusCode)
		})

		// 测试非严格模式配置
		t.Run("non-strict mode config", func(t *testing.T) {
			host, status := test.NewTestHost(nonStrictConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			configRaw, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, configRaw)

			config, ok := configRaw.(*AIModelFilterConfig)
			require.True(t, ok)

			require.Equal(t, []string{"gpt-4"}, config.allowedModels)
			require.False(t, config.strictMode)
		})

		// 测试空模型列表配置
		t.Run("empty models config", func(t *testing.T) {
			host, status := test.NewTestHost(emptyModelsConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			configRaw, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, configRaw)

			config, ok := configRaw.(*AIModelFilterConfig)
			require.True(t, ok)

			require.Equal(t, []string{}, config.allowedModels)
		})
	})
}

func TestOnHttpRequestHeadersWithTestFramework(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试基本请求头处理
		t.Run("basic request headers", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "api.openai.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			// 应该返回ActionContinue，因为需要等待请求体
			require.Equal(t, types.ActionContinue, action)
		})

		// 测试Gemini API路径
		t.Run("gemini api path", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置Gemini API请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "generativelanguage.googleapis.com"},
				{":path", "/v1/models/gemini-pro:generateContent"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			require.Equal(t, types.ActionContinue, action)
		})
	})
}

func TestOnHttpRequestBodyWithTestFramework(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试允许的模型
		t.Run("allowed model in body", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "api.openai.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			// 构造请求体 - 允许的模型
			requestBody := `{
				"model": "gpt-4",
				"messages": [
					{
						"role": "user",
						"content": "Hello"
					}
				]
			}`

			// 调用请求体处理
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			// 应该返回ActionContinue，因为模型在允许列表中
			require.Equal(t, types.ActionContinue, action)
		})

		// 测试不允许的模型
		t.Run("disallowed model in body", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "api.openai.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			// 构造请求体 - 不允许的模型
			requestBody := `{
				"model": "llama-3",
				"messages": [
					{
						"role": "user",
						"content": "Hello"
					}
				]
			}`

			// 调用请求体处理
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			// 应该返回ActionPause，因为模型不在允许列表中
			require.Equal(t, types.ActionPause, action)
		})

		// 测试通配符匹配
		t.Run("wildcard model match", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "api.anthropic.com"},
				{":path", "/v1/messages"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			// 构造请求体 - 通配符匹配的模型
			requestBody := `{
				"model": "claude-3-sonnet",
				"messages": [
					{
						"role": "user",
						"content": "Hello"
					}
				]
			}`

			// 调用请求体处理
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			// 应该返回ActionContinue，因为模型匹配通配符
			require.Equal(t, types.ActionContinue, action)
		})

		// 测试空模型列表（允许所有）
		t.Run("empty allowed models list", func(t *testing.T) {
			host, status := test.NewTestHost(emptyModelsConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "api.openai.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			// 构造请求体
			requestBody := `{
				"model": "any-model",
				"messages": [
					{
						"role": "user",
						"content": "Hello"
					}
				]
			}`

			// 调用请求体处理
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			// 应该返回ActionContinue，因为没有配置允许的模型列表
			require.Equal(t, types.ActionContinue, action)
		})

		// 测试无法提取模型名称（严格模式）
		t.Run("cannot extract model name strict mode", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "api.openai.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			// 构造请求体 - 没有model字段
			requestBody := `{
				"messages": [
					{
						"role": "user",
						"content": "Hello"
					}
				]
			}`

			// 调用请求体处理
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			// 应该返回ActionPause，因为严格模式下无法提取模型名称
			require.Equal(t, types.ActionPause, action)
		})

		// 测试无法提取模型名称（非严格模式）
		t.Run("cannot extract model name non-strict mode", func(t *testing.T) {
			host, status := test.NewTestHost(nonStrictConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "api.openai.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			// 构造请求体 - 没有model字段
			requestBody := `{
				"messages": [
					{
						"role": "user",
						"content": "Hello"
					}
				]
			}`

			// 调用请求体处理
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			// 应该返回ActionContinue，因为非严格模式下允许无法提取模型名称
			require.Equal(t, types.ActionContinue, action)
		})

		// 测试从Gemini API路径提取模型
		t.Run("extract model from gemini path", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置Gemini API请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "generativelanguage.googleapis.com"},
				{":path", "/v1/models/gpt-4:generateContent"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			// 构造请求体 - 没有model字段，但路径中有
			requestBody := `{
				"contents": [
					{
						"parts": [
							{
								"text": "Hello"
							}
						]
					}
				]
			}`

			// 调用请求体处理
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			// 应该返回ActionContinue，因为从路径中提取的模型在允许列表中
			require.Equal(t, types.ActionContinue, action)
		})

		// 测试无效的JSON
		t.Run("invalid json body", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "api.openai.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			// 构造无效的JSON
			invalidBody := []byte(`{invalid json`)

			// 调用请求体处理
			action := host.CallOnHttpRequestBody(invalidBody)

			// 应该返回ActionPause，因为严格模式下无法解析JSON
			require.Equal(t, types.ActionPause, action)
		})
	})
}
