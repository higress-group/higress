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
	"github.com/stretchr/testify/require"
)

// 测试配置：基本DashScope OCR配置
var basicDashScopeConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"type":        "dashscope",
		"apiKey":      "test-api-key-123",
		"serviceName": "ocr-service",
		"serviceHost": "dashscope.aliyuncs.com",
		"servicePort": 443,
		"timeout":     10000,
		"model":       "qwen-vl-ocr",
	})
	return data
}()

// 测试配置：最小DashScope配置（使用默认值）
var minimalDashScopeConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"type":        "dashscope",
		"apiKey":      "minimal-api-key",
		"serviceName": "ocr-service",
	})
	return data
}()

// 测试配置：自定义端口和超时配置
var customPortTimeoutConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"type":        "dashscope",
		"apiKey":      "custom-api-key",
		"serviceName": "ocr-service",
		"serviceHost": "custom.dashscope.com",
		"servicePort": 8443,
		"timeout":     30000,
		"model":       "qwen-vl-ocr",
	})
	return data
}()

// 测试配置：自定义模型配置
var customModelConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"type":        "dashscope",
		"apiKey":      "model-api-key",
		"serviceName": "ocr-service",
		"serviceHost": "dashscope.aliyuncs.com",
		"servicePort": 443,
		"timeout":     15000,
		"model":       "custom-ocr-model",
	})
	return data
}()

func TestParseConfig(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// 测试基本DashScope配置解析
		t.Run("basic dashscope config", func(t *testing.T) {
			host, status := test.NewTestHost(basicDashScopeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试最小DashScope配置解析（使用默认值）
		t.Run("minimal dashscope config", func(t *testing.T) {
			host, status := test.NewTestHost(minimalDashScopeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试自定义端口和超时配置解析
		t.Run("custom port timeout config", func(t *testing.T) {
			host, status := test.NewTestHost(customPortTimeoutConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试自定义模型配置解析
		t.Run("custom model config", func(t *testing.T) {
			host, status := test.NewTestHost(customModelConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})
	})
}

func TestOnHttpRequestHeaders(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试JSON内容类型的请求头处理
		t.Run("JSON content type headers", func(t *testing.T) {
			host, status := test.NewTestHost(basicDashScopeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置JSON内容类型的请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			// 需要读取并处理请求体，因此暂停后续请求头处理
			require.Equal(t, types.HeaderStopIteration, action)
		})

		// 测试非JSON内容类型的请求头处理
		t.Run("non-JSON content type headers", func(t *testing.T) {
			host, status := test.NewTestHost(basicDashScopeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置非JSON内容类型的请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"content-type", "text/plain"},
			})

			// 应该返回ActionContinue，但不会读取请求体
			require.Equal(t, types.ActionContinue, action)
		})

		// 测试缺少content-type的请求头处理
		t.Run("missing content type headers", func(t *testing.T) {
			host, status := test.NewTestHost(basicDashScopeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置缺少content-type的请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
			})

			// 应该返回ActionContinue
			require.Equal(t, types.ActionContinue, action)
		})
	})
}

func TestOnHttpRequestBody(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试包含单张图片的请求体处理
		t.Run("single image request body", func(t *testing.T) {
			host, status := test.NewTestHost(basicDashScopeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			// 构造包含单张图片的请求体
			requestBody := `{
				"messages": [
					{
						"role": "user",
						"content": [
							{
								"type": "text",
								"text": "这张图片里有什么？"
							},
							{
								"type": "image_url",
								"image_url": {
									"url": "https://example.com/image1.jpg"
								}
							}
						]
					}
				]
			}`

			// 调用请求体处理
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			// 应该返回ActionPause，因为需要等待OCR响应
			require.Equal(t, types.ActionPause, action)

			// 模拟OCR服务响应
			ocrResponse := `{
				"choices": [
					{
						"message": {
							"content": "图片中包含一些文字内容"
						}
					}
				]
			}`

			// 模拟HTTP调用响应
			host.CallOnHttpCall([][2]string{
				{"content-type", "application/json"},
				{":status", "200"},
			}, []byte(ocrResponse))

			modifiedBody := host.GetRequestBody()
			require.NotNil(t, modifiedBody)
			require.Contains(t, string(modifiedBody), "图片中包含一些文字内容")

			// 完成HTTP请求
			host.CompleteHttp()
		})

		// 测试包含多张图片的请求体处理
		t.Run("multiple images request body", func(t *testing.T) {
			host, status := test.NewTestHost(basicDashScopeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			// 构造包含多张图片的请求体
			requestBody := `{
				"messages": [
					{
						"role": "user",
						"content": [
							{
								"type": "text",
								"text": "这些图片里有什么？"
							},
							{
								"type": "image_url",
								"image_url": {
									"url": "https://example.com/image1.jpg"
								}
							},
							{
								"type": "image_url",
								"image_url": {
									"url": "https://example.com/image2.jpg"
								}
							}
						]
					}
				]
			}`

			// 调用请求体处理
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			// 应该返回ActionPause，因为需要等待OCR响应
			require.Equal(t, types.ActionPause, action)

			// 模拟第一张图片的OCR响应
			ocrResponse1 := `{
				"choices": [
					{
						"message": {
							"content": "第一张图片包含文字A"
						}
					}
				]
			}`

			// 模拟第二张图片的OCR响应
			ocrResponse2 := `{
				"choices": [
					{
						"message": {
							"content": "第二张图片包含文字B"
						}
					}
				]
			}`

			// 模拟第一个HTTP调用响应
			host.CallOnHttpCall([][2]string{
				{"content-type", "application/json"},
				{":status", "200"},
			}, []byte(ocrResponse1))

			// 模拟第二个HTTP调用响应
			host.CallOnHttpCall([][2]string{
				{"content-type", "application/json"},
				{":status", "200"},
			}, []byte(ocrResponse2))

			modifiedBody := host.GetRequestBody()
			require.NotNil(t, modifiedBody)
			require.Contains(t, string(modifiedBody), "第一张图片包含文字A")
			require.Contains(t, string(modifiedBody), "第二张图片包含文字B")

			// 完成HTTP请求
			host.CompleteHttp()
		})

		// 测试不包含图片的请求体处理
		t.Run("no image request body", func(t *testing.T) {
			host, status := test.NewTestHost(basicDashScopeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			// 构造不包含图片的请求体
			requestBody := `{
				"messages": [
					{
						"role": "user",
						"content": [
							{
								"type": "text",
								"text": "你好，请介绍一下自己"
							}
						]
					}
				]
			}`

			// 调用请求体处理
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			// 应该返回ActionContinue，因为没有图片需要处理
			require.Equal(t, types.ActionContinue, action)
		})
	})
}

// 测试配置验证
func TestConfigValidation(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// 测试缺少type配置
		t.Run("missing type", func(t *testing.T) {
			invalidConfig := func() json.RawMessage {
				data, _ := json.Marshal(map[string]interface{}{
					"apiKey":      "test-api-key",
					"serviceName": "ocr-service",
					"serviceHost": "dashscope.aliyuncs.com",
					"servicePort": 443,
				})
				return data
			}()

			host, status := test.NewTestHost(invalidConfig)
			defer host.Reset()
			// 应该返回错误状态，因为缺少必需的type
			require.NotEqual(t, types.OnPluginStartStatusOK, status)
		})

		// 测试缺少apiKey配置
		t.Run("missing apiKey", func(t *testing.T) {
			invalidConfig := func() json.RawMessage {
				data, _ := json.Marshal(map[string]interface{}{
					"type":        "dashscope",
					"serviceName": "ocr-service",
					"serviceHost": "dashscope.aliyuncs.com",
					"servicePort": 443,
					"timeout":     10000,
					"model":       "qwen-vl-ocr",
				})
				return data
			}()

			host, status := test.NewTestHost(invalidConfig)
			defer host.Reset()
			// 应该返回错误状态，因为缺少必需的apiKey
			require.NotEqual(t, types.OnPluginStartStatusOK, status)
		})

		// 测试缺少serviceName配置
		t.Run("missing serviceName", func(t *testing.T) {
			invalidConfig := func() json.RawMessage {
				data, _ := json.Marshal(map[string]interface{}{
					"type":        "dashscope",
					"apiKey":      "test-api-key",
					"serviceHost": "dashscope.aliyuncs.com",
					"servicePort": 443,
					"timeout":     10000,
					"model":       "qwen-vl-ocr",
				})
				return data
			}()

			host, status := test.NewTestHost(invalidConfig)
			defer host.Reset()
			// 应该返回错误状态，因为缺少必需的serviceName
			require.NotEqual(t, types.OnPluginStartStatusOK, status)
		})

		// 测试未知的provider类型
		t.Run("unknown provider type", func(t *testing.T) {
			invalidConfig := func() json.RawMessage {
				data, _ := json.Marshal(map[string]interface{}{
					"type":        "unknown-provider",
					"apiKey":      "test-api-key",
					"serviceName": "ocr-service",
					"serviceHost": "example.com",
					"servicePort": 443,
				})
				return data
			}()

			host, status := test.NewTestHost(invalidConfig)
			defer host.Reset()
			// 应该返回错误状态，因为provider类型未知
			require.NotEqual(t, types.OnPluginStartStatusOK, status)
		})
	})
}

// 测试边界情况
func TestEdgeCases(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试空请求体
		t.Run("empty request body", func(t *testing.T) {
			host, status := test.NewTestHost(basicDashScopeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			// 调用请求体处理 - 空请求体
			action := host.CallOnHttpRequestBody([]byte{})

			// 应该返回ActionContinue，因为没有图片需要处理
			require.Equal(t, types.ActionContinue, action)
		})

		// 测试无效JSON请求体
		t.Run("invalid JSON request body", func(t *testing.T) {
			host, status := test.NewTestHost(basicDashScopeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			// 调用请求体处理 - 无效JSON
			invalidJSON := []byte(`{"messages": [{"role": "user", "content": "test"}`)
			action := host.CallOnHttpRequestBody(invalidJSON)

			// 应该返回ActionContinue，因为JSON解析失败
			require.Equal(t, types.ActionContinue, action)
		})

		// 测试OCR服务错误响应
		t.Run("OCR service error response", func(t *testing.T) {
			host, status := test.NewTestHost(basicDashScopeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			// 构造包含图片的请求体
			requestBody := `{
				"messages": [
					{
						"role": "user",
						"content": [
							{
								"type": "text",
								"text": "这张图片里有什么？"
							},
							{
								"type": "image_url",
								"image_url": {
									"url": "https://example.com/image1.jpg"
								}
							}
						]
					}
				]
			}`

			// 调用请求体处理
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			// 应该返回ActionPause
			require.Equal(t, types.ActionPause, action)

			// 模拟OCR服务错误响应
			errorResponse := `{
				"error": "Service unavailable",
				"message": "OCR service is down"
			}`

			host.CallOnHttpCall([][2]string{
				{"content-type", "application/json"},
				{":status", "503"},
			}, []byte(errorResponse))

			host.CompleteHttp()
		})

		// 测试OCR服务返回空结果
		t.Run("OCR service empty response", func(t *testing.T) {
			host, status := test.NewTestHost(basicDashScopeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			// 构造包含图片的请求体
			requestBody := `{
				"messages": [
					{
						"role": "user",
						"content": [
							{
								"type": "text",
								"text": "这张图片里有什么？"
							},
							{
								"type": "image_url",
								"image_url": {
									"url": "https://example.com/image1.jpg"
								}
							}
						]
					}
				]
			}`

			// 调用请求体处理
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			// 应该返回ActionPause
			require.Equal(t, types.ActionPause, action)

			// 模拟OCR服务返回空结果
			emptyResponse := `{
				"choices": []
			}`

			host.CallOnHttpCall([][2]string{
				{"content-type", "application/json"},
				{":status", "200"},
			}, []byte(emptyResponse))

			host.CompleteHttp()
		})
	})
}
