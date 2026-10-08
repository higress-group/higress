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

// 测试配置：启用请求转换
var requestTransformConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"request": map[string]interface{}{
			"enable": true,
			"prompt": "将请求转换为JSON格式",
		},
		"response": map[string]interface{}{
			"enable": false,
			"prompt": "",
		},
		"provider": map[string]interface{}{
			"apiKey":      "test-api-key",
			"serviceName": "ai-service",
			"domain":      "ai.example.com",
		},
	})
	return data
}()

// 测试配置：启用响应转换
var responseTransformConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"request": map[string]interface{}{
			"enable": false,
			"prompt": "",
		},
		"response": map[string]interface{}{
			"enable": true,
			"prompt": "将响应转换为XML格式",
		},
		"provider": map[string]interface{}{
			"apiKey":      "test-api-key",
			"serviceName": "ai-service",
			"domain":      "ai.example.com",
		},
	})
	return data
}()

// 测试配置：同时启用请求和响应转换
var bothTransformConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"request": map[string]interface{}{
			"enable": true,
			"prompt": "将请求转换为JSON格式",
		},
		"response": map[string]interface{}{
			"enable": true,
			"prompt": "将响应转换为XML格式",
		},
		"provider": map[string]interface{}{
			"apiKey":      "test-api-key",
			"serviceName": "ai-service",
			"domain":      "ai.example.com",
		},
	})
	return data
}()

// 测试配置：禁用所有转换
var noTransformConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"request": map[string]interface{}{
			"enable": false,
			"prompt": "",
		},
		"response": map[string]interface{}{
			"enable": false,
			"prompt": "",
		},
		"provider": map[string]interface{}{
			"apiKey":      "test-api-key",
			"serviceName": "ai-service",
			"domain":      "ai.example.com",
		},
	})
	return data
}()

// 测试配置：缺少API密钥
var missingAPIKeyConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"request": map[string]interface{}{
			"enable": true,
			"prompt": "将请求转换为JSON格式",
		},
		"response": map[string]interface{}{
			"enable": false,
			"prompt": "",
		},
		"provider": map[string]interface{}{
			"serviceName": "ai-service",
			"domain":      "ai.example.com",
		},
	})
	return data
}()

func TestParseConfig(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// 测试请求转换配置解析
		t.Run("request transform config", func(t *testing.T) {
			host, status := test.NewTestHost(requestTransformConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试响应转换配置解析
		t.Run("response transform config", func(t *testing.T) {
			host, status := test.NewTestHost(responseTransformConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试同时启用请求和响应转换的配置解析
		t.Run("both transform config", func(t *testing.T) {
			host, status := test.NewTestHost(bothTransformConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试禁用所有转换的配置解析
		t.Run("no transform config", func(t *testing.T) {
			host, status := test.NewTestHost(noTransformConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试缺少API密钥的配置解析
		t.Run("missing API key config", func(t *testing.T) {
			host, status := test.NewTestHost(missingAPIKeyConfig)
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
		// 测试启用请求转换时的请求头处理
		t.Run("request transform enabled", func(t *testing.T) {
			host, status := test.NewTestHost(requestTransformConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			// 应该返回 HeaderStopIteration，因为需要读取请求体
			require.Equal(t, types.HeaderStopIteration, action)
		})

		// 测试禁用请求转换时的请求头处理
		t.Run("request transform disabled", func(t *testing.T) {
			host, status := test.NewTestHost(noTransformConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
			})

			// 应该返回 ActionContinue，因为不需要转换
			require.Equal(t, types.ActionContinue, action)
		})

		// 测试启用请求转换但缺少提示词时的请求头处理
		t.Run("request transform enabled but no prompt", func(t *testing.T) {
			// 创建缺少提示词的配置
			noPromptConfig := func() json.RawMessage {
				data, _ := json.Marshal(map[string]interface{}{
					"request": map[string]interface{}{
						"enable": true,
						"prompt": "",
					},
					"response": map[string]interface{}{
						"enable": false,
						"prompt": "",
					},
					"provider": map[string]interface{}{
						"apiKey":      "test-api-key",
						"serviceName": "ai-service",
						"domain":      "ai.example.com",
					},
				})
				return data
			}()

			host, status := test.NewTestHost(noPromptConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
			})

			// 应该返回 ActionContinue，因为提示词为空
			require.Equal(t, types.ActionContinue, action)
		})
	})
}

func TestOnHttpRequestBody(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试请求体转换
		t.Run("request body transformation", func(t *testing.T) {
			host, status := test.NewTestHost(requestTransformConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			// 设置请求体
			requestBody := []byte(`{"name": "test", "value": "data"}`)
			action := host.CallOnHttpRequestBody(requestBody)

			// 应该返回 ActionPause，因为需要等待外部 AI 服务调用完成
			require.Equal(t, types.ActionPause, action)

			// 模拟 AI 服务的 HTTP 调用响应（仅包含头与空行，再跟随 body 的 HTTP 帧）
			// 注意：每个头部行必须有 key: value 格式，否则 extraceHttpFrame 会解析失败
			aiResponse := `{"output": {"text": "Host: example.com\nContent-Type: application/json\n\n{\"transformed\": true, \"data\": \"converted\"}"}}`
			host.CallOnHttpCall([][2]string{
				{"Content-Type", "application/json"},
				{":status", "200"},
			}, []byte(aiResponse))

			// 完成外呼回调后，应继续处理
			action = host.GetHttpStreamAction()
			require.Equal(t, types.ActionContinue, action)

			// 验证请求体已被替换为 AI 返回的内容
			expected := []byte(`{"transformed": true, "data": "converted"}`)
			got := host.GetRequestBody()
			require.Equal(t, expected, got)

			host.CompleteHttp()
		})

		// 测试 AI 服务返回无效 HTTP 帧的情况
		t.Run("invalid HTTP frame from AI service", func(t *testing.T) {
			host, status := test.NewTestHost(requestTransformConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			// 设置请求体
			requestBody := []byte(`{"name": "test", "value": "data"}`)
			action := host.CallOnHttpRequestBody(requestBody)

			// 应该返回 ActionPause
			require.Equal(t, types.ActionPause, action)

			// 模拟 AI 服务返回格式错误但不会导致 panic 的响应
			// 返回一个包含 \n\n 但格式不正确的响应，这样 extraceHttpFrame 会返回错误但不会 panic
			invalidResponse := `{"output": {"text": "invalid\n\nhttp frame"}}`
			host.CallOnHttpCall([][2]string{
				{"Content-Type", "application/json"},
				{":status", "200"},
			}, []byte(invalidResponse))

			// 完成外呼回调后，应继续处理
			action = host.GetHttpStreamAction()
			require.Equal(t, types.ActionContinue, action)

			// 由于解析失败，请求体应该保持原样
			expected := requestBody
			got := host.GetRequestBody()
			require.Equal(t, expected, got)

			host.CompleteHttp()
		})
	})
}

func TestOnHttpResponseHeaders(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试启用响应转换时的响应头处理
		t.Run("response transform enabled", func(t *testing.T) {
			host, status := test.NewTestHost(responseTransformConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置响应头
			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})

			// 应该返回 HeaderStopIteration，因为需要读取响应体
			require.Equal(t, types.HeaderStopIteration, action)
		})

		// 测试禁用响应转换时的响应头处理
		t.Run("response transform disabled", func(t *testing.T) {
			host, status := test.NewTestHost(noTransformConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置响应头
			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})

			// 应该返回 ActionContinue，因为不需要转换
			require.Equal(t, types.ActionContinue, action)
		})

		// 测试启用响应转换但缺少提示词时的响应头处理
		t.Run("response transform enabled but no prompt", func(t *testing.T) {
			// 创建缺少提示词的配置
			noPromptConfig := func() json.RawMessage {
				data, _ := json.Marshal(map[string]interface{}{
					"request": map[string]interface{}{
						"enable": false,
						"prompt": "",
					},
					"response": map[string]interface{}{
						"enable": true,
						"prompt": "",
					},
					"provider": map[string]interface{}{
						"apiKey":      "test-api-key",
						"serviceName": "ai-service",
						"domain":      "ai.example.com",
					},
				})
				return data
			}()

			host, status := test.NewTestHost(noPromptConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置响应头
			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})

			// 应该返回 ActionContinue，因为提示词为空
			require.Equal(t, types.ActionContinue, action)
		})
	})
}

func TestOnHttpResponseBody(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试响应体转换
		t.Run("response body transformation", func(t *testing.T) {
			host, status := test.NewTestHost(responseTransformConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置响应头
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})

			// 设置响应体
			responseBody := []byte(`{"status": "success", "data": "test"}`)
			action := host.CallOnHttpResponseBody(responseBody)

			// 应该返回 ActionPause，因为需要等待外部 AI 服务调用完成
			require.Equal(t, types.ActionPause, action)

			// 模拟 AI 服务的 HTTP 调用响应
			// 返回一个有效的 HTTP 帧格式，确保每个头部行都有 key: value 格式
			// 注意：不要包含状态行（如 HTTP/1.1 200 OK），只包含头部行
			aiResponse := `{"output": {"text": "Content-Type: application/xml\n\n<response><status>success</status><data>test</data></response>"}}`
			host.CallOnHttpCall([][2]string{
				{"Content-Type", "application/json"},
				{":status", "200"},
			}, []byte(aiResponse))

			// 完成外呼回调后，应继续处理
			action = host.GetHttpStreamAction()
			require.Equal(t, types.ActionContinue, action)

			// 验证响应体已被替换为 AI 返回的内容
			expected := []byte(`<response><status>success</status><data>test</data></response>`)
			got := host.GetResponseBody()
			require.Equal(t, expected, got)

			host.CompleteHttp()
		})

		// 测试 AI 服务返回无效 HTTP 帧的情况
		t.Run("invalid HTTP frame from AI service for response", func(t *testing.T) {
			host, status := test.NewTestHost(responseTransformConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置响应头
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})

			// 设置响应体
			responseBody := []byte(`{"status": "success", "data": "test"}`)
			action := host.CallOnHttpResponseBody(responseBody)

			// 应该返回 ActionPause
			require.Equal(t, types.ActionPause, action)

			// 模拟 AI 服务返回格式错误但不会导致 panic 的响应
			// 返回一个包含 \n\n 但格式不正确的响应，这样 extraceHttpFrame 会返回错误但不会 panic
			invalidResponse := `{"output": {"text": "invalid\n\nhttp frame"}}`
			host.CallOnHttpCall([][2]string{
				{"Content-Type", "application/json"},
				{":status", "200"},
			}, []byte(invalidResponse))

			// 完成外呼回调后，应继续处理
			action = host.GetHttpStreamAction()
			require.Equal(t, types.ActionContinue, action)

			// 由于解析失败，响应体应该保持原样
			expected := responseBody
			got := host.GetResponseBody()
			require.Equal(t, expected, got)

			host.CompleteHttp()
		})
	})
}

func TestCompleteFlow(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试完整的请求和响应转换流程
		t.Run("complete request and response transformation", func(t *testing.T) {
			host, status := test.NewTestHost(bothTransformConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 1. 处理请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			// 应该返回 HeaderStopIteration
			require.Equal(t, types.HeaderStopIteration, action)

			// 2. 处理请求体
			requestBody := []byte(`{"name": "test", "value": "data"}`)
			action = host.CallOnHttpRequestBody(requestBody)

			// 应该返回 ActionPause
			require.Equal(t, types.ActionPause, action)

			// 3. 模拟 AI 服务对请求的响应
			// 确保头部行格式正确，避免 extraceHttpFrame 解析失败
			requestAIResponse := `{"output": {"text": "Host: example.com\nContent-Type: application/json\n\n{\"transformed\": true, \"data\": \"converted\"}"}}`
			host.CallOnHttpCall([][2]string{
				{"Content-Type", "application/json"},
				{":status", "200"},
			}, []byte(requestAIResponse))

			// 4. 处理响应头
			action = host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})

			// 应该返回 HeaderStopIteration
			require.Equal(t, types.HeaderStopIteration, action)

			// 5. 处理响应体
			responseBody := []byte(`{"status": "success", "data": "test"}`)
			action = host.CallOnHttpResponseBody(responseBody)

			// 应该返回 ActionPause
			require.Equal(t, types.ActionPause, action)

			// 6. 模拟 AI 服务对响应的响应
			// 确保头部行格式正确，避免 extraceHttpFrame 解析失败
			// 注意：不要包含状态行，只包含头部行
			responseAIResponse := `{"output": {"text": "Content-Type: application/xml\n\n<response><status>success</status><data>test</data></response>"}}`
			host.CallOnHttpCall([][2]string{
				{"Content-Type", "application/json"},
				{":status", "200"},
			}, []byte(responseAIResponse))

			// 验证请求和响应都被正确转换
			// 检查请求体转换结果
			expectedRequestBody := []byte(`{"transformed": true, "data": "converted"}`)
			gotRequestBody := host.GetRequestBody()
			require.Equal(t, expectedRequestBody, gotRequestBody)

			// 检查响应体转换结果
			expectedResponseBody := []byte(`<response><status>success</status><data>test</data></response>`)
			gotResponseBody := host.GetResponseBody()
			require.Equal(t, expectedResponseBody, gotResponseBody)

			// 7. 完成请求
			host.CompleteHttp()
		})
	})
}

func TestExtraceHttpFrameNoSeparator(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("missing separator does not panic", func(t *testing.T) {
			require.NotPanics(t, func() {
				_, _, err := extraceHttpFrame("single line without double newline")
				require.Error(t, err)
			})
		})

		t.Run("empty frame does not panic", func(t *testing.T) {
			require.NotPanics(t, func() {
				_, _, err := extraceHttpFrame("")
				require.Error(t, err)
			})
		})

		t.Run("crlf separator parses properly", func(t *testing.T) {
			headers, body, err := extraceHttpFrame("Header: Value\r\n\r\nBody")
			require.NoError(t, err)
			require.Equal(t, [][2]string{{"Header", "Value"}}, headers)
			require.Equal(t, []byte("Body"), body)
		})
	})
}
