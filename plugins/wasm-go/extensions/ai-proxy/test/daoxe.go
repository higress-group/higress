package test

import (
	"encoding/json"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

// 测试配置：基本 DaoXE 配置
var basicDaoxeConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":      "daoxe",
			"apiTokens": []string{"sk-daoxe-test123456789"},
			"modelMapping": map[string]string{
				"*": "gpt-4o-mini",
			},
		},
	})
	return data
}()

// 测试配置：DaoXE 多模型配置
var daoxeMultiModelConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":      "daoxe",
			"apiTokens": []string{"sk-daoxe-multi-model"},
			"modelMapping": map[string]string{
				"gpt-4":         "claude-sonnet-4",
				"gpt-3.5-turbo": "gpt-4o-mini",
			},
		},
	})
	return data
}()

// 测试配置：无效 DaoXE 配置（缺少 apiToken）
var invalidDaoxeConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":         "daoxe",
			"apiTokens":    []string{},
			"modelMapping": map[string]string{},
		},
	})
	return data
}()

// 测试配置：完整 DaoXE 配置
var completeDaoxeConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":      "daoxe",
			"apiTokens": []string{"sk-daoxe-complete-test"},
			"timeout":   30000,
			"modelMapping": map[string]string{
				"gpt-4":         "claude-sonnet-4",
				"gpt-3.5-turbo": "gpt-4o-mini",
				"*":             "gpt-4o-mini",
			},
		},
	})
	return data
}()

// RunDaoxeParseConfigTests 测试 DaoXE 配置解析
func RunDaoxeParseConfigTests(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// 测试基本 DaoXE 配置解析
		t.Run("basic daoxe config", func(t *testing.T) {
			host, status := test.NewTestHost(basicDaoxeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试 DaoXE 多模型配置解析
		t.Run("daoxe multi model config", func(t *testing.T) {
			host, status := test.NewTestHost(daoxeMultiModelConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试无效 DaoXE 配置（缺少 apiToken）
		t.Run("invalid daoxe config - missing apiToken", func(t *testing.T) {
			host, status := test.NewTestHost(invalidDaoxeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})

		// 测试完整 DaoXE 配置解析
		t.Run("daoxe complete config", func(t *testing.T) {
			host, status := test.NewTestHost(completeDaoxeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})
	})
}

// RunDaoxeOnHttpRequestHeadersTests 测试 DaoXE 请求头处理
func RunDaoxeOnHttpRequestHeadersTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试 DaoXE 聊天完成请求头处理
		t.Run("daoxe chat completion request headers", func(t *testing.T) {
			host, status := test.NewTestHost(basicDaoxeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			// 应该返回 HeaderStopIteration，因为需要处理请求体
			require.Equal(t, types.HeaderStopIteration, action)

			requestHeaders := host.GetRequestHeaders()
			require.NotNil(t, requestHeaders)

			// 验证 Host 是否被改为 DaoXE 域名
			hostValue, hasHost := test.GetHeaderValue(requestHeaders, ":authority")
			require.True(t, hasHost, "Host header should exist")
			require.Equal(t, "api.daoxe.com", hostValue, "Host should be changed to DaoXE domain")

			// 验证 Authorization 是否被设置
			authValue, hasAuth := test.GetHeaderValue(requestHeaders, "Authorization")
			require.True(t, hasAuth, "Authorization header should exist")
			require.Contains(t, authValue, "Bearer sk-daoxe-test123456789", "Authorization should contain DaoXE API token with Bearer prefix")

			// 验证 Path 保持 OpenAI 兼容格式
			pathValue, hasPath := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, hasPath, "Path header should exist")
			require.Equal(t, "/v1/chat/completions", pathValue, "Path should remain OpenAI compatible")
		})

		// 测试 DaoXE 文本完成请求头处理
		t.Run("daoxe completion request headers", func(t *testing.T) {
			host, status := test.NewTestHost(basicDaoxeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			require.Equal(t, types.HeaderStopIteration, action)

			requestHeaders := host.GetRequestHeaders()
			require.NotNil(t, requestHeaders)

			hostValue, hasHost := test.GetHeaderValue(requestHeaders, ":authority")
			require.True(t, hasHost)
			require.Equal(t, "api.daoxe.com", hostValue)

			pathValue, hasPath := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, hasPath)
			require.Equal(t, "/v1/completions", pathValue, "Path should remain OpenAI compatible for completions")

			authValue, hasAuth := test.GetHeaderValue(requestHeaders, "Authorization")
			require.True(t, hasAuth, "Authorization header should exist for completions")
			require.Contains(t, authValue, "Bearer sk-daoxe-test123456789", "Authorization should contain DaoXE API token")
		})

		// 测试 DaoXE 模型列表请求头处理
		t.Run("daoxe models request headers", func(t *testing.T) {
			host, status := test.NewTestHost(basicDaoxeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/models"},
				{":method", "GET"},
			})

			// TODO: Due to the limitations of the test framework, we just treat it as a request with body here.
			require.Equal(t, types.HeaderStopIteration, action)

			requestHeaders := host.GetRequestHeaders()
			require.NotNil(t, requestHeaders)

			hostValue, hasHost := test.GetHeaderValue(requestHeaders, ":authority")
			require.True(t, hasHost)
			require.Equal(t, "api.daoxe.com", hostValue)

			pathValue, hasPath := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, hasPath)
			require.Equal(t, "/v1/models", pathValue, "Path should remain OpenAI compatible for models")

			authValue, hasAuth := test.GetHeaderValue(requestHeaders, "Authorization")
			require.True(t, hasAuth, "Authorization header should exist for models")
			require.Contains(t, authValue, "Bearer sk-daoxe-test123456789", "Authorization should contain DaoXE API token")
		})
	})
}

// RunDaoxeOnHttpRequestBodyTests 测试 DaoXE 请求体处理
func RunDaoxeOnHttpRequestBodyTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试 DaoXE 聊天完成请求体处理
		t.Run("daoxe chat completion request body", func(t *testing.T) {
			host, status := test.NewTestHost(basicDaoxeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			requestBody := `{
				"model": "gpt-3.5-turbo",
				"messages": [
					{"role": "user", "content": "Hello, world!"}
				],
				"stream": false
			}`

			action := host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			actualRequestBody := host.GetRequestBody()
			require.NotNil(t, actualRequestBody)

			// 验证模型映射
			require.Contains(t, string(actualRequestBody), "gpt-4o-mini",
				"Model should be mapped via modelMapping")
			require.Contains(t, string(actualRequestBody), "Hello, world!",
				"Request content should be preserved")
		})

		// 测试 DaoXE 多模型映射
		t.Run("daoxe multi model mapping", func(t *testing.T) {
			host, status := test.NewTestHost(daoxeMultiModelConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			requestBody := `{
				"model": "gpt-4",
				"messages": [
					{"role": "user", "content": "Write a poem about AI"}
				],
				"stream": true
			}`

			action := host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			actualRequestBody := host.GetRequestBody()
			require.NotNil(t, actualRequestBody)

			require.Contains(t, string(actualRequestBody), "claude-sonnet-4",
				"GPT-4 should be mapped to the configured target model")
			require.Contains(t, string(actualRequestBody), "Write a poem about AI",
				"Request content should be preserved")
			require.Contains(t, string(actualRequestBody), `"stream": true`,
				"Stream flag should be preserved")
		})

		// 测试不支持的 API（覆盖 OnRequestBody 错误路径）
		t.Run("daoxe unsupported api", func(t *testing.T) {
			host, status := test.NewTestHost(basicDaoxeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/unknown/endpoint"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			requestBody := `{"model": "gpt-3.5-turbo", "messages": [{"role": "user", "content": "test"}]}`

			action := host.CallOnHttpRequestBody([]byte(requestBody))
			// OnRequestBody 返回错误后由 main.go 的 ErrorHandler 处理
			require.Equal(t, types.ActionContinue, action)
		})
	})
}

// RunDaoxeOnHttpResponseHeadersTests 测试 DaoXE 响应头处理
func RunDaoxeOnHttpResponseHeadersTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("daoxe chat completion response headers", func(t *testing.T) {
			host, status := test.NewTestHost(basicDaoxeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})

			require.Equal(t, types.ActionContinue, action)

			responseHeaders := host.GetResponseHeaders()
			require.NotNil(t, responseHeaders)

			statusValue, hasStatus := test.GetHeaderValue(responseHeaders, ":status")
			require.True(t, hasStatus, "Status header should exist")
			require.Equal(t, "200", statusValue, "Status should be 200")

			contentTypeValue, hasContentType := test.GetHeaderValue(responseHeaders, "content-type")
			require.True(t, hasContentType, "Content-Type header should exist")
			require.Equal(t, "application/json", contentTypeValue, "Content-Type should be application/json")
		})
	})
}

// RunDaoxeOnHttpResponseBodyTests 测试 DaoXE 响应体处理
func RunDaoxeOnHttpResponseBodyTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("daoxe chat completion response body", func(t *testing.T) {
			host, status := test.NewTestHost(basicDaoxeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			responseBody := `{
				"id": "chatcmpl-test",
				"object": "chat.completion",
				"created": 1728558433,
				"model": "gpt-4o-mini",
				"choices": [
					{
						"index": 0,
						"message": {
							"role": "assistant",
							"content": "Hello! I am an AI assistant."
						},
						"finish_reason": "stop"
					}
				]
			}`

			action := host.CallOnHttpResponseBody([]byte(responseBody))
			require.Equal(t, types.ActionContinue, action)

			actualResponseBody := host.GetResponseBody()
			require.NotNil(t, actualResponseBody)

			bodyStr := string(actualResponseBody)
			require.Contains(t, bodyStr, `"object": "chat.completion"`, "Response should contain chat.completion object")
			require.Contains(t, bodyStr, `"Hello! I am an AI assistant."`, "Response should contain the assistant message")
		})
	})
}

// RunDaoxeOnStreamingResponseBodyTests 测试 DaoXE 流式响应体处理
func RunDaoxeOnStreamingResponseBodyTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("daoxe streaming response body", func(t *testing.T) {
			host, status := test.NewTestHost(basicDaoxeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			requestBody := `{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"test"}],"stream":true}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "text/event-stream"},
			})

			// DaoXE 流式响应使用 OpenAI 兼容的 SSE 格式，原样透传
			streamChunks := []string{
				`data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1699123456,"model":"gpt-4o-mini","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"},"finish_reason":null}]}`,
				`data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1699123456,"model":"gpt-4o-mini","choices":[{"index":0,"delta":{"content":" there!"},"finish_reason":null}]}`,
				`data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1699123456,"model":"gpt-4o-mini","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
				`data: [DONE]`,
			}

			for _, chunk := range streamChunks {
				streamChunk := chunk + "\n\n"
				action := host.CallOnHttpStreamingResponseBody([]byte(streamChunk), false)
				require.Equal(t, types.ActionContinue, action)
				require.Contains(t, string(host.GetResponseBody()), chunk)
			}

			action = host.CallOnHttpStreamingResponseBody([]byte{}, true)
			require.Equal(t, types.ActionContinue, action)
		})
	})
}
