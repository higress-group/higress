// Copyright (c) 2024 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"encoding/json"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/santhosh-tekuri/jsonschema"
	"github.com/stretchr/testify/require"
)

// 测试配置：基础配置
var basicConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"serviceName":              "ai-service",
		"serviceDomain":            "api.openai.com",
		"servicePort":              443,
		"servicePath":              "/v1/chat/completions",
		"apiKey":                   "sk-test123",
		"serviceTimeout":           30000,
		"maxRetry":                 3,
		"contentPath":              "choices.0.message.content",
		"enableContentDisposition": true,
		// 添加一个简单的JSON Schema，避免编译失败
		"jsonSchema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"content": map[string]interface{}{
					"type": "string",
				},
			},
		},
	})
	return data
}()

// 测试配置：使用serviceUrl的配置
var serviceUrlConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"serviceName":              "ai-service",
		"serviceUrl":               "https://api.openai.com/v1/chat/completions",
		"apiKey":                   "sk-test456",
		"serviceTimeout":           50000,
		"maxRetry":                 5,
		"contentPath":              "choices.0.message.content",
		"enableContentDisposition": false,
		// 添加一个简单的JSON Schema，避免编译失败
		"jsonSchema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"content": map[string]interface{}{
					"type": "string",
				},
			},
		},
	})
	return data
}()

// 测试配置：包含JSON Schema的配置
var jsonSchemaConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"serviceName":   "ai-service",
		"serviceDomain": "api.openai.com",
		"servicePort":   443,
		"apiKey":        "sk-test789",
		"jsonSchema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{
					"type": "string",
				},
				"age": map[string]interface{}{
					"type": "integer",
				},
			},
			"required": []string{"name"},
		},
		"enableSwagger": true,
		"enableOas3":    false,
	})
	return data
}()

// 测试配置：启用OAS3的配置
var oas3Config = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"serviceName":   "ai-service",
		"serviceDomain": "api.openai.com",
		"servicePort":   443,
		"apiKey":        "sk-test101",
		"jsonSchema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"title": map[string]interface{}{
					"type": "string",
				},
				"content": map[string]interface{}{
					"type": "string",
				},
			},
		},
		"enableSwagger": false,
		"enableOas3":    true,
	})
	return data
}()

// 测试配置：无效的JSON Schema配置
var invalidJsonSchemaConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"serviceName":   "ai-service",
		"serviceDomain": "api.openai.com",
		"servicePort":   443,
		"apiKey":        "sk-test303",
		"jsonSchema":    "invalid-schema",
	})
	return data
}()

// 测试配置：缺少必需字段的配置
var missingRequiredConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"apiKey":         "sk-test404",
		"serviceTimeout": 30000,
	})
	return data
}()

func TestParseConfig(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// 测试基础配置解析
		t.Run("basic config", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)
			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			pluginConfig := config.(*PluginConfig)
			require.Equal(t, "ai-service", pluginConfig.serviceName)
			require.Equal(t, "api.openai.com", pluginConfig.serviceDomain)
			require.Equal(t, 443, pluginConfig.servicePort)
			require.Equal(t, "/v1/chat/completions", pluginConfig.servicePath)
			require.Equal(t, "sk-test123", pluginConfig.apiKey)
			require.Equal(t, 30000, pluginConfig.serviceTimeout)
			require.Equal(t, 3, pluginConfig.maxRetry)
			require.Equal(t, "choices.0.message.content", pluginConfig.contentPath)
			require.True(t, pluginConfig.enableContentDisposition)
			require.NotNil(t, pluginConfig.jsonSchema)
			require.Equal(t, jsonschema.Draft7, pluginConfig.draft)
			require.True(t, pluginConfig.enableJsonSchemaValidation)
		})

		// 测试使用serviceUrl的配置解析
		t.Run("serviceUrl config", func(t *testing.T) {
			host, status := test.NewTestHost(serviceUrlConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)
			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			pluginConfig := config.(*PluginConfig)
			require.Equal(t, "ai-service", pluginConfig.serviceName)
			require.Equal(t, "api.openai.com", pluginConfig.serviceDomain)
			require.Equal(t, 443, pluginConfig.servicePort)
			require.Equal(t, "/v1/chat/completions", pluginConfig.servicePath)
			require.Equal(t, "sk-test456", pluginConfig.apiKey)
			require.Equal(t, 50000, pluginConfig.serviceTimeout)
			require.Equal(t, 5, pluginConfig.maxRetry)
			require.False(t, pluginConfig.enableContentDisposition)
		})

		// 测试包含JSON Schema的配置解析
		t.Run("jsonSchema config", func(t *testing.T) {
			host, status := test.NewTestHost(jsonSchemaConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)
			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			pluginConfig := config.(*PluginConfig)
			require.NotNil(t, pluginConfig.jsonSchema)
			require.Equal(t, jsonschema.Draft4, pluginConfig.draft)
			require.True(t, pluginConfig.enableJsonSchemaValidation)
			require.NotNil(t, pluginConfig.compile)
		})

		// 测试启用OAS3的配置解析
		t.Run("oas3 config", func(t *testing.T) {
			host, status := test.NewTestHost(oas3Config)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)
			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			pluginConfig := config.(*PluginConfig)
			require.Equal(t, jsonschema.Draft7, pluginConfig.draft)
			require.True(t, pluginConfig.enableJsonSchemaValidation)
		})

		// 测试无效的JSON Schema配置
		t.Run("invalid jsonSchema config", func(t *testing.T) {
			host, status := test.NewTestHost(invalidJsonSchemaConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)
			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			pluginConfig := config.(*PluginConfig)
			// 根据插件的实际行为，无效的JSON Schema会导致编译失败
			require.Equal(t, uint32(JSON_SCHEMA_COMPILE_FAILED_CODE), pluginConfig.rejectStruct.RejectCode)
		})

		// 测试缺少必需字段的配置
		t.Run("missing required config", func(t *testing.T) {
			host, status := test.NewTestHost(missingRequiredConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)
			config, _ := host.GetMatchConfig()
			require.NotNil(t, config)

			pluginConfig := config.(*PluginConfig)
			// 根据插件的实际行为，缺少serviceDomain会导致JSON Schema编译失败
			require.Equal(t, uint32(JSON_SCHEMA_COMPILE_FAILED_CODE), pluginConfig.rejectStruct.RejectCode)
			require.Contains(t, pluginConfig.rejectStruct.RejectMsg, "Json Schema compile failed")
		})
	})
}

func TestOnHttpRequestHeaders(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试正常请求头处理
		t.Run("normal request headers", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Authorization", "Bearer sk-user123"},
				{"Content-Type", "application/json"},
				{"Content-Length", "100"},
			})

			// 应该返回ActionContinue
			require.Equal(t, types.ActionContinue, action)
		})

		// 测试来自插件的请求头处理
		t.Run("request from this plugin", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置来自插件的请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{EXTEND_HEADER_KEY, "true"},
				{"Content-Type", "application/json"},
			})

			// 应该返回ActionContinue
			require.Equal(t, types.ActionContinue, action)
		})

		// 测试没有Authorization头的请求
		t.Run("no authorization header", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置没有Authorization的请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
				{"Content-Length", "100"},
			})

			// 应该返回ActionContinue
			require.Equal(t, types.ActionContinue, action)
		})

		// 测试配置错误的请求头处理
		t.Run("config error", func(t *testing.T) {
			host, status := test.NewTestHost(missingRequiredConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			// 应该返回ActionPause
			require.Equal(t, types.ActionPause, action)
		})
	})
}

func TestOnHttpRequestBody(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试来自插件的请求（应该直接继续）
		t.Run("request from this plugin", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，包含EXTEND_HEADER_KEY来标记请求来自插件
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
				{EXTEND_HEADER_KEY, "true"},
			})

			// 设置请求体
			body := `{
				"model": "gpt-3.5-turbo",
				"messages": [
					{"role": "user", "content": "Hello"}
				]
			}`

			action := host.CallOnHttpRequestBody([]byte(body))
			// 应该返回ActionContinue，因为请求来自插件
			require.Equal(t, types.ActionContinue, action)
		})

		// 测试配置错误的请求体处理
		t.Run("config error", func(t *testing.T) {
			host, status := test.NewTestHost(missingRequiredConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			// 设置请求体
			body := `{
				"model": "gpt-3.5-turbo",
				"messages": [
					{"role": "user", "content": "Hello"}
				]
			}`

			action := host.CallOnHttpRequestBody([]byte(body))
			// 应该返回ActionContinue，因为配置有错误
			require.Equal(t, types.ActionContinue, action)
		})

		// 测试正常请求体处理 - 成功响应
		t.Run("normal request with successful response", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			// 设置请求体
			body := `{
				"model": "gpt-3.5-turbo",
				"messages": [
					{"role": "user", "content": "What is AI?"}
				]
			}`

			action := host.CallOnHttpRequestBody([]byte(body))
			// 应该返回ActionPause，等待外部服务响应
			require.Equal(t, types.ActionPause, action)

			// 模拟外部服务返回成功响应
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(`{
				"id": "chatcmpl-123",
				"choices": [
					{
						"index": 0,
						"message": {
							"role": "assistant",
							"content": "{\"definition\": \"AI is artificial intelligence\", \"examples\": [\"machine learning\", \"natural language processing\"]}"
						}
					}
				]
			}`))

			response := host.GetLocalResponse()
			require.NotNil(t, response)
			require.Contains(t, string(response.Data), "definition")
			require.Contains(t, string(response.Data), "examples")

			// 完成HTTP请求
			host.CompleteHttp()
		})

		// 测试正常请求体处理 - 需要重试的响应
		t.Run("normal request with retry response", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			// 设置请求体
			body := `{
				"model": "gpt-3.5-turbo",
				"messages": [
					{"role": "user", "content": "What is AI?"}
				]
			}`

			action := host.CallOnHttpRequestBody([]byte(body))
			// 应该返回ActionPause，等待外部服务响应
			require.Equal(t, types.ActionPause, action)

			// 模拟外部服务返回需要重试的响应（content字段不是有效JSON）
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(`{
				"id": "chatcmpl-123",
				"choices": [
					{
						"index": 0,
						"message": {
							"role": "assistant",
							"content": "AI is artificial intelligence. It includes machine learning and natural language processing."
						}
					}
				]
			}`))

			// 由于content不是有效JSON，插件会进行重试
			// 模拟重试请求的响应
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(`{
				"id": "chatcmpl-456",
				"choices": [
					{
						"index": 0,
						"message": {
							"role": "assistant",
							"content": "{\"definition\": \"AI is artificial intelligence\", \"examples\": [\"machine learning\", \"natural language processing\"]}"
						}
					}
				]
			}`))

			// 验证最终响应体是提取的JSON内容
			response := host.GetLocalResponse()
			require.NotNil(t, response)
			require.Contains(t, string(response.Data), "definition")
			require.Contains(t, string(response.Data), "examples")

			// 完成HTTP请求
			host.CompleteHttp()
		})

		// 测试外部服务返回无效响应体
		t.Run("external service returns invalid response body", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			// 设置请求体
			body := `{
				"model": "gpt-3.5-turbo",
				"messages": [
					{"role": "user", "content": "What is AI?"}
				]
			}`

			action := host.CallOnHttpRequestBody([]byte(body))
			// 应该返回ActionPause，等待外部服务响应
			require.Equal(t, types.ActionPause, action)

			// 模拟外部服务返回无效的响应体
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(`invalid json response`))

			// 验证响应体包含错误信息
			response := host.GetLocalResponse()
			require.NotNil(t, response)
			require.Contains(t, string(response.Data), "invalid json response")

			// 完成HTTP请求
			host.CompleteHttp()
		})

		// 测试外部服务返回缺少content字段的响应
		t.Run("external service returns response without content field", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			// 设置请求体
			body := `{
				"model": "gpt-3.5-turbo",
				"messages": [
					{"role": "user", "content": "What is AI?"}
				]
			}`

			action := host.CallOnHttpRequestBody([]byte(body))
			// 应该返回ActionPause，等待外部服务响应
			require.Equal(t, types.ActionPause, action)

			// 模拟外部服务返回缺少content字段的响应
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(`{
				"id": "chatcmpl-123",
				"choices": [
					{
						"index": 0,
						"message": {
							"role": "assistant"
						}
					}
				]
			}`))

			// 验证响应体包含错误信息
			response := host.GetLocalResponse()
			require.NotNil(t, response)
			require.Contains(t, string(response.Data), "response body does not contain the content")

			// 完成HTTP请求
			host.CompleteHttp()
		})

		// 测试使用自定义servicePath的请求
		t.Run("request with custom service path", func(t *testing.T) {
			host, status := test.NewTestHost(serviceUrlConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/custom/chat"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			// 设置请求体
			body := `{
				"model": "gpt-3.5-turbo",
				"messages": [
					{"role": "user", "content": "What is AI?"}
				]
			}`

			action := host.CallOnHttpRequestBody([]byte(body))
			// 应该返回ActionPause，等待外部服务响应
			require.Equal(t, types.ActionPause, action)

			// 模拟外部服务返回成功响应
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(`{
				"id": "chatcmpl-123",
				"choices": [
					{
						"index": 0,
						"message": {
							"role": "assistant",
							"content": "{\"answer\": \"AI is artificial intelligence\"}"
						}
					}
				]
			}`))

			// 验证响应体是提取的JSON内容
			response := host.GetLocalResponse()
			require.NotNil(t, response)
			require.Contains(t, string(response.Data), "answer")

			// 完成HTTP请求
			host.CompleteHttp()
		})

		// 测试达到最大重试次数的情况
		t.Run("max retry count exceeded", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			// 设置请求体
			body := `{
				"model": "gpt-3.5-turbo",
				"messages": [
					{"role": "user", "content": "What is AI?"}
				]
			}`

			action := host.CallOnHttpRequestBody([]byte(body))
			// 应该返回ActionPause，等待外部服务响应
			require.Equal(t, types.ActionPause, action)

			// 模拟多次重试，每次都返回无效的content
			for i := 0; i < 4; i++ { // 超过最大重试次数3次
				host.CallOnHttpCall([][2]string{
					{":status", "200"},
					{"content-type", "application/json"},
				}, []byte(`{
					"id": "chatcmpl-123",
					"choices": [
						{
							"index": 0,
							"message": {
								"role": "assistant",
								"content": "AI is artificial intelligence"
							}
						}
					]
				}`))
			}

			// 验证最终响应体包含重试次数超限的错误信息
			response := host.GetLocalResponse()
			require.NotNil(t, response)
			require.Contains(t, string(response.Data), "retry count exceeds max retry count")

			// 完成HTTP请求
			host.CompleteHttp()
		})
	})
}

func TestRejectStruct(t *testing.T) {
	// 测试RejectStruct的GetBytes方法
	t.Run("GetBytes", func(t *testing.T) {
		reject := RejectStruct{
			RejectCode: 1001,
			RejectMsg:  "Test error message",
		}

		bytes := reject.GetBytes()
		require.NotNil(t, bytes)

		// 验证JSON格式
		var result RejectStruct
		err := json.Unmarshal(bytes, &result)
		require.NoError(t, err)
		require.Equal(t, uint32(1001), result.RejectCode)
		require.Equal(t, "Test error message", result.RejectMsg)
	})

	// 测试RejectStruct的GetShortMsg方法
	t.Run("GetShortMsg", func(t *testing.T) {
		reject := RejectStruct{
			RejectCode: 1001,
			RejectMsg:  "Json Schema is not valid: invalid format",
		}

		shortMsg := reject.GetShortMsg()
		require.Equal(t, "ai-json-resp.Json Schema is not valid", shortMsg)
	})

	// 测试RejectStruct的GetShortMsg方法 - 没有冒号的情况
	t.Run("GetShortMsg no colon", func(t *testing.T) {
		reject := RejectStruct{
			RejectCode: 1001,
			RejectMsg:  "Simple error message",
		}

		shortMsg := reject.GetShortMsg()
		require.Equal(t, "ai-json-resp.Simple error message", shortMsg)
	})
}

func TestValidateBody(t *testing.T) {
	// 创建测试配置
	config := &PluginConfig{
		contentPath:                "choices.0.message.content",
		jsonSchema:                 nil,   // 明确设置为nil，禁用JSON Schema验证
		enableJsonSchemaValidation: false, // 禁用JSON Schema验证
	}

	// 测试有效的响应体
	t.Run("valid response body", func(t *testing.T) {
		validBody := []byte(`{
			"id": "chatcmpl-123",
			"choices": [
				{
					"index": 0,
					"message": {
						"role": "assistant",
						"content": "Hello, how can I help you?"
					}
				}
			]
		}`)
		err := config.ValidateBody(validBody)
		require.NoError(t, err)
	})

	// 测试无效的JSON响应体
	t.Run("invalid JSON response body", func(t *testing.T) {
		invalidBody := []byte(`invalid json content`)

		err := config.ValidateBody(invalidBody)
		require.Error(t, err)
		require.Equal(t, uint32(SERVICE_UNAVAILABLE_CODE), config.rejectStruct.RejectCode)
		require.Contains(t, config.rejectStruct.RejectMsg, "service unavailable")
	})

	// 测试缺少content字段的响应体
	t.Run("missing content field", func(t *testing.T) {
		missingContentBody := []byte(`{
			"id": "chatcmpl-123",
			"choices": [
				{
					"index": 0,
					"message": {
						"role": "assistant"
					}
				}
			]
		}`)

		err := config.ValidateBody(missingContentBody)
		require.Error(t, err)
		require.Equal(t, uint32(SERVICE_UNAVAILABLE_CODE), config.rejectStruct.RejectCode)
		require.Contains(t, config.rejectStruct.RejectMsg, "response body does not contain the content")
	})

	// 测试空的响应体
	t.Run("empty response body", func(t *testing.T) {
		emptyBody := []byte{}

		err := config.ValidateBody(emptyBody)
		require.Error(t, err)
		require.Equal(t, uint32(SERVICE_UNAVAILABLE_CODE), config.rejectStruct.RejectCode)
	})
}

func TestExtractJson(t *testing.T) {
	// 创建测试配置
	config := &PluginConfig{
		jsonSchema:                 nil,   // 明确设置为nil，禁用JSON Schema验证
		enableJsonSchemaValidation: false, // 禁用JSON Schema验证
	}

	// 测试提取有效的JSON
	t.Run("extract valid JSON", func(t *testing.T) {
		content := `Here is the response: {"name": "John", "age": 30} and some other text`

		jsonStr, err := config.ExtractJson(content)
		require.NoError(t, err)
		require.Equal(t, `{"name": "John", "age": 30}`, jsonStr)
	})

	// 测试提取嵌套JSON
	t.Run("extract nested JSON", func(t *testing.T) {
		content := `Response: {"user": {"name": "John", "profile": {"age": 30, "city": "NYC"}}}`

		jsonStr, err := config.ExtractJson(content)
		require.NoError(t, err)
		require.Equal(t, `{"user": {"name": "John", "profile": {"age": 30, "city": "NYC"}}}`, jsonStr)
	})

	// 测试没有JSON的内容
	t.Run("no JSON in content", func(t *testing.T) {
		content := `This is just plain text without any JSON content`

		_, err := config.ExtractJson(content)
		require.Error(t, err)
		require.Contains(t, err.Error(), "cannot find json in the response body")
	})

	// 测试只有开始括号的内容
	t.Run("only opening brace", func(t *testing.T) {
		content := `Here is the start: { but no closing brace`

		_, err := config.ExtractJson(content)
		require.Error(t, err)
		require.Contains(t, err.Error(), "cannot find json in the response body")
	})

	// 测试只有结束括号的内容
	t.Run("only closing brace", func(t *testing.T) {
		content := `Here is the end: } but no opening brace`

		_, err := config.ExtractJson(content)
		require.Error(t, err)
		require.Contains(t, err.Error(), "cannot find json in the response body")
	})

	// 测试无效的JSON格式
	t.Run("invalid JSON format", func(t *testing.T) {
		content := `Here is invalid JSON: {"name": "John", "age": 30,}`

		_, err := config.ExtractJson(content)
		require.Error(t, err)
		// ExtractJson会提取到{"name": "John", "age": 30,}，但json.Unmarshal会失败
		// 因为JSON格式无效（末尾有多余的逗号）
		require.Contains(t, err.Error(), "invalid character '}' looking for beginning of object key string")
	})

	// 测试多个JSON对象（应该提取第一个完整的）
	t.Run("multiple JSON objects", func(t *testing.T) {
		content := `First: {"name": "John"} Second: {"age": 30}`

		_, err := config.ExtractJson(content)
		require.Error(t, err)
		// ExtractJson会提取到{"name": "John"} Second: {"age": 30}
		// 这不是有效的JSON，因为"Second: {"age": 30}"不是有效的JSON语法
		require.Contains(t, err.Error(), "invalid character 'S' after top-level value")
	})
}

type noopLog struct{}

func (noopLog) Trace(string)                     {}
func (noopLog) Tracef(string, ...interface{})    {}
func (noopLog) Debug(string)                     {}
func (noopLog) Debugf(string, ...interface{})    {}
func (noopLog) Info(string)                      {}
func (noopLog) Infof(string, ...interface{})     {}
func (noopLog) Warn(string)                      {}
func (noopLog) Warnf(string, ...interface{})     {}
func (noopLog) Error(string)                     {}
func (noopLog) Errorf(string, ...interface{})    {}
func (noopLog) Critical(string)                  {}
func (noopLog) Criticalf(string, ...interface{}) {}
func (noopLog) ResetID(string)                   {}

func TestSaveBodyToHistMsg_NilMessageInChoices(t *testing.T) {
	ctx := &RequestContext{}
	reqBody := []byte(`{"messages":[{"role":"user","content":"hello"}]}`)
	// choices without message field
	respBody := []byte(`{"id":"chatcmpl-1","choices":[{"index":0,"delta":{"content":"hi"}}]}`)

	require.NotPanics(t, func() {
		ctx.SaveBodyToHistMsg(noopLog{}, reqBody, respBody)
	})
	require.Len(t, ctx.HistoryMessages, 1)
	require.Equal(t, "user", ctx.HistoryMessages[0].Role)
	require.Equal(t, "hello", ctx.HistoryMessages[0].Content)

	// choices with valid message
	ctx2 := &RequestContext{}
	respBody2 := []byte(`{"id":"chatcmpl-2","choices":[{"index":0,"message":{"role":"assistant","content":"world"}}]}`)
	require.NotPanics(t, func() {
		ctx2.SaveBodyToHistMsg(noopLog{}, reqBody, respBody2)
	})
	require.Len(t, ctx2.HistoryMessages, 2)
	require.Equal(t, "world", ctx2.HistoryMessages[1].Content)
}
