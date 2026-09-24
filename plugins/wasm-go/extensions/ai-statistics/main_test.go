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
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/higress-group/wasm-go/pkg/tokenusage"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/stretchr/testify/require"
)

// 测试配置：基本统计配置
var basicConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"attributes": []map[string]interface{}{
			{
				"key":                   "request_id",
				"value_source":          "request_header",
				"value":                 "x-request-id",
				"apply_to_log":          true,
				"apply_to_span":         false,
				"as_separate_log_field": false,
			},
			{
				"key":                   "api_version",
				"value_source":          "fixed_value",
				"value":                 "v1",
				"apply_to_log":          true,
				"apply_to_span":         true,
				"as_separate_log_field": false,
			},
			{
				"key":                   "model",
				"value_source":          "request_body",
				"value":                 "model",
				"apply_to_log":          true,
				"apply_to_span":         true,
				"as_separate_log_field": false,
			},
			{
				"key":                   "input_token",
				"value_source":          "response_body",
				"value":                 "usage.prompt_tokens",
				"apply_to_log":          true,
				"apply_to_span":         true,
				"as_separate_log_field": false,
			},
			{
				"key":                   "output_token",
				"value_source":          "response_body",
				"value":                 "usage.completion_tokens",
				"apply_to_log":          true,
				"apply_to_span":         true,
				"as_separate_log_field": false,
			},
			{
				"key":                   "total_token",
				"value_source":          "response_body",
				"value":                 "usage.total_tokens",
				"apply_to_log":          true,
				"apply_to_span":         true,
				"as_separate_log_field": false,
			},
		},
		"disable_openai_usage": false,
	})
	return data
}()

// 测试配置：流式响应体属性配置
var streamingBodyConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"attributes": []map[string]interface{}{
			{
				"key":                   "response_content",
				"value_source":          "response_streaming_body",
				"value":                 "choices.0.message.content",
				"rule":                  "first",
				"apply_to_log":          true,
				"apply_to_span":         false,
				"as_separate_log_field": false,
			},
			{
				"key":                   "model_name",
				"value_source":          "response_streaming_body",
				"value":                 "model",
				"rule":                  "replace",
				"apply_to_log":          true,
				"apply_to_span":         true,
				"as_separate_log_field": false,
			},
		},
		"disable_openai_usage": false,
	})
	return data
}()

var streamingModelExtractionConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"attributes": []map[string]interface{}{
			{
				"key":                   "first_model",
				"value_source":          "response_streaming_body",
				"value":                 "model",
				"rule":                  "first",
				"apply_to_log":          true,
				"apply_to_span":         false,
				"as_separate_log_field": false,
			},
			{
				"key":                   "replace_model",
				"value_source":          "response_streaming_body",
				"value":                 "model",
				"rule":                  "replace",
				"apply_to_log":          true,
				"apply_to_span":         false,
				"as_separate_log_field": false,
			},
		},
		"disable_openai_usage": true,
	})
	return data
}()

// 测试配置：请求体属性配置
var requestBodyConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"attributes": []map[string]interface{}{
			{
				"key":                   "user_message_count",
				"value_source":          "request_body",
				"value":                 "messages.#(role==\"user\")",
				"apply_to_log":          true,
				"apply_to_span":         false,
				"as_separate_log_field": false,
			},
			{
				"key":                   "request_model",
				"value_source":          "request_body",
				"value":                 "model",
				"apply_to_log":          true,
				"apply_to_span":         true,
				"as_separate_log_field": false,
			},
		},
		"disable_openai_usage": false,
	})
	return data
}()

// 测试配置：响应体属性配置
var responseBodyConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"attributes": []map[string]interface{}{
			{
				"key":                   "response_status",
				"value_source":          "response_body",
				"value":                 "status",
				"apply_to_log":          true,
				"apply_to_span":         false,
				"as_separate_log_field": false,
			},
			{
				"key":                   "response_message",
				"value_source":          "response_body",
				"value":                 "message",
				"apply_to_log":          true,
				"apply_to_span":         true,
				"as_separate_log_field": false,
			},
		},
		"disable_openai_usage": false,
	})
	return data
}()

// 测试配置：禁用 OpenAI 使用统计
var disableOpenaiUsageConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"attributes": []map[string]interface{}{
			{
				"key":                   "custom_attribute",
				"value_source":          "fixed_value",
				"value":                 "custom_value",
				"apply_to_log":          true,
				"apply_to_span":         false,
				"as_separate_log_field": false,
			},
		},
		"disable_openai_usage": true,
	})
	return data
}()

// 测试配置：空属性配置
var emptyAttributesConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"attributes":           []map[string]interface{}{},
		"disable_openai_usage": false,
	})
	return data
}()

func TestParseConfig(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// 测试基本统计配置解析
		t.Run("basic config", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试流式响应体属性配置解析
		t.Run("streaming body config", func(t *testing.T) {
			host, status := test.NewTestHost(streamingBodyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试请求体属性配置解析
		t.Run("request body config", func(t *testing.T) {
			host, status := test.NewTestHost(requestBodyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试响应体属性配置解析
		t.Run("response body config", func(t *testing.T) {
			host, status := test.NewTestHost(responseBodyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试禁用 OpenAI 使用统计配置解析
		t.Run("disable openai usage config", func(t *testing.T) {
			host, status := test.NewTestHost(disableOpenaiUsageConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试空属性配置解析
		t.Run("empty attributes config", func(t *testing.T) {
			host, status := test.NewTestHost(emptyAttributesConfig)
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
		// 测试基本请求头处理
		t.Run("basic request headers", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"x-request-id", "req-123"},
				{"x-mse-consumer", "consumer1"},
			})

			// 应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})

		// 测试包含 consumer 的请求头处理
		t.Run("request headers with consumer", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"x-request-id", "req-456"},
				{"x-mse-consumer", "consumer2"},
			})

			// 应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})

		// 测试不包含 consumer 的请求头处理
		t.Run("request headers without consumer", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"x-request-id", "req-789"},
			})

			// 应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})
	})
}

func TestOnHttpRequestBody(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试基本请求体处理
		t.Run("basic request body", func(t *testing.T) {
			host, status := test.NewTestHost(requestBodyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
			})

			// 设置请求体
			requestBody := []byte(`{
				"model": "gpt-3.5-turbo",
				"messages": [
					{"role": "user", "content": "Hello"},
					{"role": "assistant", "content": "Hi there"},
					{"role": "user", "content": "How are you?"}
				]
			}`)
			action := host.CallOnHttpRequestBody(requestBody)

			// 应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})

		// 测试 Google Gemini 格式的请求体处理
		t.Run("gemini request body", func(t *testing.T) {
			host, status := test.NewTestHost(requestBodyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/models/gemini-pro:generateContent"},
				{":method", "POST"},
			})

			// 设置请求体
			requestBody := []byte(`{
				"contents": [
					{"role": "user", "parts": [{"text": "Hello"}]},
					{"parts": [{"text": "Hi there"}]}
				]
			}`)
			action := host.CallOnHttpRequestBody(requestBody)

			// 应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})

		// 测试不包含消息的请求体处理
		t.Run("request body without messages", func(t *testing.T) {
			host, status := test.NewTestHost(requestBodyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
			})

			// 设置请求体
			requestBody := []byte(`{
				"model": "gpt-3.5-turbo",
				"temperature": 0.7
			}`)
			action := host.CallOnHttpRequestBody(requestBody)

			// 应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})
	})
}

func TestOnHttpResponseHeaders(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试基本响应头处理
		t.Run("basic response headers", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
			})

			// 设置响应头
			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})

			// 应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})

		// 测试流式响应头处理
		t.Run("streaming response headers", func(t *testing.T) {
			host, status := test.NewTestHost(streamingBodyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
			})

			// 设置流式响应头
			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "text/event-stream"},
			})

			// 应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})
	})
}

func getAILogAttributes(t *testing.T, host test.TestHost) map[string]interface{} {
	t.Helper()

	raw, err := host.GetProperty([]string{wrapper.AILogKey})
	require.NoError(t, err)

	var attrs map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(wrapper.UnmarshalStr(`"`+string(raw)+`"`)), &attrs))
	return attrs
}

func TestOnHttpStreamingBody(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试流式响应体处理
		t.Run("streaming response body", func(t *testing.T) {
			host, status := test.NewTestHost(streamingBodyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
			})

			// 设置流式响应头
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "text/event-stream"},
			})

			// 处理第一个流式块
			firstChunk := []byte(`data: {"choices":[{"message":{"content":"Hello"}}],"model":"gpt-3.5-turbo"}`)
			action := host.CallOnHttpStreamingResponseBody(firstChunk, false)

			result := host.GetResponseBody()
			require.Equal(t, firstChunk, result)

			// 应该返回原始数据
			require.Equal(t, types.ActionContinue, action)

			// 处理最后一个流式块
			lastChunk := []byte(`data: {"choices":[{"message":{"content":"How can I help you?"}}],"model":"gpt-3.5-turbo"}`)
			action = host.CallOnHttpStreamingResponseBody(lastChunk, true)

			// 应该返回原始数据
			require.Equal(t, types.ActionContinue, action)

			result = host.GetResponseBody()
			require.Equal(t, lastChunk, result)

			host.CompleteHttp()
		})

		t.Run("streaming first and replace skip empty model chunks", func(t *testing.T) {
			host, status := test.NewTestHost(streamingModelExtractionConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})
			require.Equal(t, types.ActionContinue, action)

			action = host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "text/event-stream"},
			})
			require.Equal(t, types.ActionContinue, action)

			action = host.CallOnHttpStreamingResponseBody([]byte("data: {\"model\":\"\"}\n\n"), false)
			require.Equal(t, types.ActionContinue, action)
			action = host.CallOnHttpStreamingResponseBody([]byte("data: {\"model\":null}\n\n"), false)
			require.Equal(t, types.ActionContinue, action)
			action = host.CallOnHttpStreamingResponseBody([]byte("data: {\"model\":\"gpt-4o\"}\n\n"), true)
			require.Equal(t, types.ActionContinue, action)

			attrs := getAILogAttributes(t, host)
			require.Equal(t, "gpt-4o", attrs["first_model"])
			require.Equal(t, "gpt-4o", attrs["replace_model"])

			host.CompleteHttp()
		})

		t.Run("streaming first and replace return nil when model path is missing", func(t *testing.T) {
			host, status := test.NewTestHost(streamingModelExtractionConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})
			require.Equal(t, types.ActionContinue, action)

			action = host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "text/event-stream"},
			})
			require.Equal(t, types.ActionContinue, action)

			action = host.CallOnHttpStreamingResponseBody([]byte("data: {\"choices\":[]}\n\n"), false)
			require.Equal(t, types.ActionContinue, action)
			action = host.CallOnHttpStreamingResponseBody([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n"), true)
			require.Equal(t, types.ActionContinue, action)

			attrs := getAILogAttributes(t, host)
			require.Nil(t, attrs["first_model"])
			require.Nil(t, attrs["replace_model"])

			host.CompleteHttp()
		})

		// 测试不包含 token 统计的流式响应体处理
		t.Run("streaming body without token usage", func(t *testing.T) {
			host, status := test.NewTestHost(streamingBodyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
			})

			// 设置流式响应头
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "text/event-stream"},
			})

			// 处理流式响应体
			chunk := []byte(`data: {"message": "Hello world"}`)
			action := host.CallOnHttpStreamingResponseBody(chunk, true)

			// 应该返回原始数据
			require.Equal(t, types.ActionContinue, action)

			result := host.GetResponseBody()
			require.Equal(t, chunk, result)

			host.CompleteHttp()
		})
	})
}

func TestOnHttpResponseBody(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试基本响应体处理
		t.Run("basic response body", func(t *testing.T) {
			host, status := test.NewTestHost(responseBodyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
			})

			// 设置响应头
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})

			// 设置响应体
			responseBody := []byte(`{
				"status": "success",
				"message": "Hello, how can I help you?",
				"choices": [{"message": {"content": "Hello"}}],
				"usage": {"prompt_tokens": 10, "completion_tokens": 15, "total_tokens": 25},
				"model": "gpt-3.5-turbo"
			}`)
			action := host.CallOnHttpResponseBody(responseBody)

			// 应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})

		// 测试不包含 token 统计的响应体处理
		t.Run("response body without token usage", func(t *testing.T) {
			host, status := test.NewTestHost(responseBodyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
			})

			// 设置响应头
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})

			// 设置响应体
			responseBody := []byte(`{
				"status": "success",
				"message": "Hello world"
			}`)
			action := host.CallOnHttpResponseBody(responseBody)

			// 应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})
	})
}

func TestMetrics(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试指标收集
		t.Run("test token usage metrics", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置路由和集群名称
			host.SetRouteName("api-v1")
			host.SetClusterName("cluster-1")

			// 1. 处理请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"x-mse-consumer", "user1"},
			})

			// 2. 处理请求体
			requestBody := []byte(`{
				"model": "gpt-3.5-turbo",
				"messages": [{"role": "user", "content": "Hello"}]
			}`)
			host.CallOnHttpRequestBody(requestBody)

			// 添加延迟，确保有足够的时间间隔来计算 llm_service_duration
			time.Sleep(10 * time.Millisecond)

			// 2.5 处理响应头（非流式 JSON，走缓冲响应体路径）
			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})
			require.Equal(t, types.ActionContinue, action)

			// 3. 处理响应体
			responseBody := []byte(`{
				"choices": [{"message": {"content": "Hello, how can I help you?"}}],
				"usage": {"prompt_tokens": 5, "completion_tokens": 8, "total_tokens": 13},
				"model": "gpt-3.5-turbo"
			}`)
			host.CallOnHttpResponseBody(responseBody)

			// 4. 完成请求
			host.CompleteHttp()

			// 5. 验证指标值
			// 检查输入 token 指标
			inputTokenMetric := "route.api-v1.upstream.cluster-1.model.gpt-3.5-turbo.consumer.user1.metric.input_token"
			inputTokenValue, err := host.GetCounterMetric(inputTokenMetric)
			require.NoError(t, err)
			require.Equal(t, uint64(5), inputTokenValue)

			// 检查输出 token 指标
			outputTokenMetric := "route.api-v1.upstream.cluster-1.model.gpt-3.5-turbo.consumer.user1.metric.output_token"
			outputTokenValue, err := host.GetCounterMetric(outputTokenMetric)
			require.NoError(t, err)
			require.Equal(t, uint64(8), outputTokenValue)

			// 检查总 token 指标
			totalTokenMetric := "route.api-v1.upstream.cluster-1.model.gpt-3.5-turbo.consumer.user1.metric.total_token"
			totalTokenValue, err := host.GetCounterMetric(totalTokenMetric)
			require.NoError(t, err)
			require.Equal(t, uint64(13), totalTokenValue)

			// 检查服务时长指标
			serviceDurationMetric := "route.api-v1.upstream.cluster-1.model.gpt-3.5-turbo.consumer.user1.metric.llm_service_duration"
			serviceDurationValue, err := host.GetCounterMetric(serviceDurationMetric)
			require.NoError(t, err)
			require.Greater(t, serviceDurationValue, uint64(0))

			// 检查请求计数指标
			durationCountMetric := "route.api-v1.upstream.cluster-1.model.gpt-3.5-turbo.consumer.user1.metric.llm_duration_count"
			durationCountValue, err := host.GetCounterMetric(durationCountMetric)
			require.NoError(t, err)
			require.Equal(t, uint64(1), durationCountValue)
		})

		// 测试流式响应指标
		t.Run("test streaming metrics", func(t *testing.T) {
			host, status := test.NewTestHost(streamingBodyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置路由和集群名称
			host.SetRouteName("api-v1")
			host.SetClusterName("cluster-1")

			// 1. 处理请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"x-mse-consumer", "user2"},
			})

			// 2. 处理请求体
			requestBody := []byte(`{
				"model": "gpt-4",
				"messages": [
					{"role": "user", "content": "Hello"}
				]
			}`)
			action := host.CallOnHttpRequestBody(requestBody)

			// 应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			// 添加延迟，确保有足够的时间间隔来计算 llm_service_duration
			time.Sleep(10 * time.Millisecond)

			// 3. 处理流式响应头
			action = host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "text/event-stream"},
			})

			// 应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			// 4. 处理流式响应体 - 添加 usage 信息（SSE 事件以 \n\n 结尾）
			firstChunk := []byte("data: {\"choices\":[{\"message\":{\"content\":\"Hello\"}}],\"model\":\"gpt-4\",\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":3,\"total_tokens\":8}}\n\n")
			action = host.CallOnHttpStreamingResponseBody(firstChunk, false)

			// 应该返回原始数据
			require.Equal(t, types.ActionContinue, action)

			result := host.GetResponseBody()
			require.Equal(t, firstChunk, result)

			// 5. 处理最后一个流式块 - 添加 usage 信息（SSE 事件以 \n\n 结尾）
			lastChunk := []byte("data: {\"choices\":[{\"message\":{\"content\":\"How can I help you?\"}}],\"model\":\"gpt-4\",\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":8,\"total_tokens\":13}}\n\n")
			action = host.CallOnHttpStreamingResponseBody(lastChunk, true)

			// 应该返回原始数据
			require.Equal(t, types.ActionContinue, action)

			result = host.GetResponseBody()
			require.Equal(t, lastChunk, result)

			// 添加延迟，确保有足够的时间间隔来计算 llm_service_duration
			time.Sleep(10 * time.Millisecond)

			// 6. 完成请求
			host.CompleteHttp()

			// 7. 验证流式响应指标
			// 检查首 token 延迟指标
			firstTokenDurationMetric := "route.api-v1.upstream.cluster-1.model.gpt-4.consumer.user2.metric.llm_first_token_duration"
			firstTokenDurationValue, err := host.GetCounterMetric(firstTokenDurationMetric)
			require.NoError(t, err)
			require.Greater(t, firstTokenDurationValue, uint64(0))

			// 检查流式请求计数指标
			streamDurationCountMetric := "route.api-v1.upstream.cluster-1.model.gpt-4.consumer.user2.metric.llm_stream_duration_count"
			streamDurationCountValue, err := host.GetCounterMetric(streamDurationCountMetric)
			require.NoError(t, err)
			require.Equal(t, uint64(1), streamDurationCountValue)

			// 检查服务时长指标
			serviceDurationMetric := "route.api-v1.upstream.cluster-1.model.gpt-4.consumer.user2.metric.llm_service_duration"
			serviceDurationValue, err := host.GetCounterMetric(serviceDurationMetric)
			require.NoError(t, err)
			require.Greater(t, serviceDurationValue, uint64(0))

			// 检查 token 指标
			inputTokenMetric := "route.api-v1.upstream.cluster-1.model.gpt-4.consumer.user2.metric.input_token"
			inputTokenValue, err := host.GetCounterMetric(inputTokenMetric)
			require.NoError(t, err)
			require.Equal(t, uint64(5), inputTokenValue)

			outputTokenMetric := "route.api-v1.upstream.cluster-1.model.gpt-4.consumer.user2.metric.output_token"
			outputTokenValue, err := host.GetCounterMetric(outputTokenMetric)
			require.NoError(t, err)
			require.Equal(t, uint64(8), outputTokenValue)

			totalTokenMetric := "route.api-v1.upstream.cluster-1.model.gpt-4.consumer.user2.metric.total_token"
			totalTokenValue, err := host.GetCounterMetric(totalTokenMetric)
			require.NoError(t, err)
			require.Equal(t, uint64(13), totalTokenValue)
		})
	})
}

func TestCompleteFlow(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试完整的统计流程
		t.Run("complete statistics flow", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置路由和集群名称
			host.SetRouteName("api-v1")
			host.SetClusterName("cluster-1")

			// 1. 处理请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"x-request-id", "req-123"},
				{"x-mse-consumer", "consumer1"},
			})

			// 应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			// 2. 处理请求体
			requestBody := []byte(`{
				"model": "gpt-3.5-turbo",
				"messages": [
					{"role": "user", "content": "Hello"}
				]
			}`)
			action = host.CallOnHttpRequestBody(requestBody)

			// 应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			// 添加延迟，确保有足够的时间间隔来计算 llm_service_duration
			time.Sleep(10 * time.Millisecond)

			// 3. 处理响应头
			action = host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})

			// 应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			// 4. 处理响应体
			responseBody := []byte(`{
				"choices": [{"message": {"content": "Hello, how can I help you?"}}],
				"usage": {"prompt_tokens": 5, "completion_tokens": 8, "total_tokens": 13},
				"model": "gpt-3.5-turbo"
			}`)
			action = host.CallOnHttpResponseBody(responseBody)

			// 应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			// 5. 完成请求
			host.CompleteHttp()

			// 6. 验证指标值
			// 检查输入 token 指标
			inputTokenMetric := "route.api-v1.upstream.cluster-1.model.gpt-3.5-turbo.consumer.consumer1.metric.input_token"
			inputTokenValue, err := host.GetCounterMetric(inputTokenMetric)
			require.NoError(t, err)
			require.Equal(t, uint64(5), inputTokenValue)

			// 检查输出 token 指标
			outputTokenMetric := "route.api-v1.upstream.cluster-1.model.gpt-3.5-turbo.consumer.consumer1.metric.output_token"
			outputTokenValue, err := host.GetCounterMetric(outputTokenMetric)
			require.NoError(t, err)
			require.Equal(t, uint64(8), outputTokenValue)

			// 检查总 token 指标
			totalTokenMetric := "route.api-v1.upstream.cluster-1.model.gpt-3.5-turbo.consumer.consumer1.metric.total_token"
			totalTokenValue, err := host.GetCounterMetric(totalTokenMetric)
			require.NoError(t, err)
			require.Equal(t, uint64(13), totalTokenValue)

			// 检查服务时长指标
			serviceDurationMetric := "route.api-v1.upstream.cluster-1.model.gpt-3.5-turbo.consumer.consumer1.metric.llm_service_duration"
			serviceDurationValue, err := host.GetCounterMetric(serviceDurationMetric)
			require.NoError(t, err)
			require.Greater(t, serviceDurationValue, uint64(0))

			// 检查请求计数指标
			durationCountMetric := "route.api-v1.upstream.cluster-1.model.gpt-3.5-turbo.consumer.consumer1.metric.llm_duration_count"
			durationCountValue, err := host.GetCounterMetric(durationCountMetric)
			require.NoError(t, err)
			require.Equal(t, uint64(1), durationCountValue)
		})

		// 测试流式响应的完整流程
		t.Run("complete streaming flow", func(t *testing.T) {
			host, status := test.NewTestHost(streamingBodyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置路由和集群名称
			host.SetRouteName("api-v1")
			host.SetClusterName("cluster-1")

			// 1. 处理请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"x-mse-consumer", "consumer2"},
			})

			// 应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			// 2. 处理请求体
			requestBody := []byte(`{
				"model": "gpt-4",
				"messages": [
					{"role": "user", "content": "Hello"}
				]
			}`)
			action = host.CallOnHttpRequestBody(requestBody)

			// 应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			// 添加延迟，确保有足够的时间间隔来计算 llm_service_duration
			time.Sleep(10 * time.Millisecond)

			// 3. 处理流式响应头
			action = host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "text/event-stream"},
			})

			// 应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			// 4. 处理流式响应体 - 添加 usage 信息（SSE 事件以 \n\n 结尾）
			firstChunk := []byte("data: {\"choices\":[{\"message\":{\"content\":\"Hello\"}}],\"model\":\"gpt-4\",\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":3,\"total_tokens\":8}}\n\n")
			action = host.CallOnHttpStreamingResponseBody(firstChunk, false)

			// 应该返回原始数据
			require.Equal(t, types.ActionContinue, action)

			result := host.GetResponseBody()
			require.Equal(t, firstChunk, result)

			// 5. 处理最后一个流式块 - 添加 usage 信息（SSE 事件以 \n\n 结尾）
			lastChunk := []byte("data: {\"choices\":[{\"message\":{\"content\":\"How can I help you?\"}}],\"model\":\"gpt-4\",\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":8,\"total_tokens\":13}}\n\n")
			action = host.CallOnHttpStreamingResponseBody(lastChunk, true)

			// 应该返回原始数据
			require.Equal(t, types.ActionContinue, action)

			result = host.GetResponseBody()
			require.Equal(t, lastChunk, result)

			// 添加延迟，确保有足够的时间间隔来计算 llm_service_duration
			time.Sleep(10 * time.Millisecond)

			// 6. 完成请求
			host.CompleteHttp()

			// 7. 验证流式响应指标
			// 检查首 token 延迟指标
			firstTokenDurationMetric := "route.api-v1.upstream.cluster-1.model.gpt-4.consumer.consumer2.metric.llm_first_token_duration"
			firstTokenDurationValue, err := host.GetCounterMetric(firstTokenDurationMetric)
			require.NoError(t, err)
			require.Greater(t, firstTokenDurationValue, uint64(0))

			// 检查流式请求计数指标
			streamDurationCountMetric := "route.api-v1.upstream.cluster-1.model.gpt-4.consumer.consumer2.metric.llm_stream_duration_count"
			streamDurationCountValue, err := host.GetCounterMetric(streamDurationCountMetric)
			require.NoError(t, err)
			require.Equal(t, uint64(1), streamDurationCountValue)

			// 检查服务时长指标
			serviceDurationMetric := "route.api-v1.upstream.cluster-1.model.gpt-4.consumer.consumer2.metric.llm_service_duration"
			serviceDurationValue, err := host.GetCounterMetric(serviceDurationMetric)
			require.NoError(t, err)
			require.Greater(t, serviceDurationValue, uint64(0))
		})
	})
}

// ==================== Built-in Attributes Tests ====================

// 测试配置：历史兼容配置（显式配置 value_source 和 value）
var legacyQuestionAnswerConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"attributes": []map[string]interface{}{
			{
				"key":          "question",
				"value_source": "request_body",
				"value":        "messages.@reverse.0.content",
				"apply_to_log": true,
			},
			{
				"key":          "answer",
				"value_source": "response_streaming_body",
				"value":        "choices.0.delta.content",
				"rule":         "append",
				"apply_to_log": true,
			},
			{
				"key":          "answer",
				"value_source": "response_body",
				"value":        "choices.0.message.content",
				"apply_to_log": true,
			},
		},
	})
	return data
}()

// 测试配置：内置属性简化配置（不配置 value_source 和 value）
var builtinAttributesConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"attributes": []map[string]interface{}{
			{
				"key":          "question",
				"apply_to_log": true,
			},
			{
				"key":          "answer",
				"apply_to_log": true,
			},
			{
				"key":          "reasoning",
				"apply_to_log": true,
			},
			{
				"key":          "tool_calls",
				"apply_to_log": true,
			},
		},
	})
	return data
}()

// 测试配置：session_id 配置
var sessionIdConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"session_id_header": "x-custom-session",
		"attributes": []map[string]interface{}{
			{
				"key":          "question",
				"apply_to_log": true,
			},
			{
				"key":          "answer",
				"apply_to_log": true,
			},
		},
	})
	return data
}()

// TestLegacyConfigCompatibility 测试历史配置兼容性
func TestLegacyConfigCompatibility(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试使用显式 value_source 和 value 配置的 question/answer
		t.Run("legacy question answer config", func(t *testing.T) {
			host, status := test.NewTestHost(legacyQuestionAnswerConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 1. 处理请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			// 2. 处理请求体
			requestBody := []byte(`{
				"model": "gpt-4",
				"messages": [
					{"role": "system", "content": "You are a helpful assistant."},
					{"role": "user", "content": "What is 2+2?"}
				]
			}`)
			action := host.CallOnHttpRequestBody(requestBody)
			require.Equal(t, types.ActionContinue, action)

			// 3. 处理响应头 (非流式)
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})

			// 4. 处理响应体
			responseBody := []byte(`{
				"choices": [{"message": {"role": "assistant", "content": "2+2 equals 4."}}],
				"model": "gpt-4",
				"usage": {"prompt_tokens": 20, "completion_tokens": 10, "total_tokens": 30}
			}`)
			action = host.CallOnHttpResponseBody(responseBody)
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})

		// 测试使用显式配置的流式响应
		t.Run("legacy streaming answer config", func(t *testing.T) {
			host, status := test.NewTestHost(legacyQuestionAnswerConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 1. 处理请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			// 2. 处理请求体
			requestBody := []byte(`{
				"model": "gpt-4",
				"stream": true,
				"messages": [{"role": "user", "content": "Hello"}]
			}`)
			host.CallOnHttpRequestBody(requestBody)

			// 3. 处理流式响应头
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "text/event-stream"},
			})

			// 4. 处理流式响应体
			chunk1 := []byte(`data: {"choices":[{"delta":{"content":"Hello"}}]}`)
			host.CallOnHttpStreamingResponseBody(chunk1, false)

			chunk2 := []byte(`data: {"choices":[{"delta":{"content":" there!"}}]}`)
			host.CallOnHttpStreamingResponseBody(chunk2, true)

			host.CompleteHttp()
		})
	})
}

// TestBuiltinAttributesDefaultSource 测试内置属性的默认 value_source
func TestBuiltinAttributesDefaultSource(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试不配置 value_source 的内置属性（非流式响应）
		t.Run("builtin attributes non-streaming", func(t *testing.T) {
			host, status := test.NewTestHost(builtinAttributesConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 1. 处理请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			// 2. 处理请求体 - question 应该自动从 request_body 提取
			requestBody := []byte(`{
				"model": "deepseek-reasoner",
				"messages": [
					{"role": "user", "content": "What is the capital of France?"}
				]
			}`)
			action := host.CallOnHttpRequestBody(requestBody)
			require.Equal(t, types.ActionContinue, action)

			// 3. 处理响应头 (非流式)
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})

			// 4. 处理响应体 - answer, reasoning, tool_calls 应该自动从 response_body 提取
			responseBody := []byte(`{
				"choices": [{
					"message": {
						"role": "assistant",
						"content": "The capital of France is Paris.",
						"reasoning_content": "The user is asking about geography. France is a country in Europe, and its capital city is Paris."
					}
				}],
				"model": "deepseek-reasoner",
				"usage": {"prompt_tokens": 15, "completion_tokens": 25, "total_tokens": 40}
			}`)
			action = host.CallOnHttpResponseBody(responseBody)
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})

		// 测试不配置 value_source 的内置属性（流式响应）
		t.Run("builtin attributes streaming", func(t *testing.T) {
			host, status := test.NewTestHost(builtinAttributesConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 1. 处理请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			// 2. 处理请求体
			requestBody := []byte(`{
				"model": "deepseek-reasoner",
				"stream": true,
				"messages": [{"role": "user", "content": "Tell me a joke"}]
			}`)
			host.CallOnHttpRequestBody(requestBody)

			// 3. 处理流式响应头
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "text/event-stream"},
			})

			// 4. 处理流式响应体 - answer, reasoning 应该自动从 response_streaming_body 提取
			chunk1 := []byte(`data: {"choices":[{"delta":{"reasoning_content":"Let me think of a good joke..."}}]}`)
			host.CallOnHttpStreamingResponseBody(chunk1, false)

			chunk2 := []byte(`data: {"choices":[{"delta":{"content":"Why did the chicken"}}]}`)
			host.CallOnHttpStreamingResponseBody(chunk2, false)

			chunk3 := []byte(`data: {"choices":[{"delta":{"content":" cross the road?"}}]}`)
			host.CallOnHttpStreamingResponseBody(chunk3, true)

			host.CompleteHttp()
		})
	})
}

// TestStreamingToolCalls 测试流式 tool_calls 解析
func TestStreamingToolCalls(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试流式 tool_calls 拼接
		t.Run("streaming tool calls assembly", func(t *testing.T) {
			host, status := test.NewTestHost(builtinAttributesConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 1. 处理请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			// 2. 处理请求体
			requestBody := []byte(`{
				"model": "gpt-4",
				"stream": true,
				"messages": [{"role": "user", "content": "What's the weather in Beijing?"}],
				"tools": [{"type": "function", "function": {"name": "get_weather"}}]
			}`)
			host.CallOnHttpRequestBody(requestBody)

			// 3. 处理流式响应头
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "text/event-stream"},
			})

			// 4. 处理流式响应体 - 模拟分片的 tool_calls
			// 第一个 chunk: tool call 的 id 和 function name
			chunk1 := []byte(`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_abc123","type":"function","function":{"name":"get_weather","arguments":""}}]}}]}`)
			host.CallOnHttpStreamingResponseBody(chunk1, false)

			// 第二个 chunk: arguments 的第一部分
			chunk2 := []byte(`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"locat"}}]}}]}`)
			host.CallOnHttpStreamingResponseBody(chunk2, false)

			// 第三个 chunk: arguments 的第二部分
			chunk3 := []byte(`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"ion\": \"Bei"}}]}}]}`)
			host.CallOnHttpStreamingResponseBody(chunk3, false)

			// 第四个 chunk: arguments 的最后部分
			chunk4 := []byte(`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"jing\"}"}}]}}]}`)
			host.CallOnHttpStreamingResponseBody(chunk4, false)

			// 最后一个 chunk: 结束
			chunk5 := []byte(`data: {"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`)
			host.CallOnHttpStreamingResponseBody(chunk5, true)

			host.CompleteHttp()
		})

		// 测试多个 tool_calls 的流式拼接
		t.Run("multiple streaming tool calls", func(t *testing.T) {
			host, status := test.NewTestHost(builtinAttributesConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 1. 处理请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			// 2. 处理请求体
			requestBody := []byte(`{
				"model": "gpt-4",
				"stream": true,
				"messages": [{"role": "user", "content": "Compare weather in Beijing and Shanghai"}]
			}`)
			host.CallOnHttpRequestBody(requestBody)

			// 3. 处理流式响应头
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "text/event-stream"},
			})

			// 4. 处理流式响应体 - 模拟多个 tool_calls
			// 第一个 tool call
			chunk1 := []byte(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_001","type":"function","function":{"name":"get_weather","arguments":""}}]}}]}`)
			host.CallOnHttpStreamingResponseBody(chunk1, false)

			// 第二个 tool call
			chunk2 := []byte(`data: {"choices":[{"delta":{"tool_calls":[{"index":1,"id":"call_002","type":"function","function":{"name":"get_weather","arguments":""}}]}}]}`)
			host.CallOnHttpStreamingResponseBody(chunk2, false)

			// 第一个 tool call 的 arguments
			chunk3 := []byte(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"location\":\"Beijing\"}"}}]}}]}`)
			host.CallOnHttpStreamingResponseBody(chunk3, false)

			// 第二个 tool call 的 arguments
			chunk4 := []byte(`data: {"choices":[{"delta":{"tool_calls":[{"index":1,"function":{"arguments":"{\"location\":\"Shanghai\"}"}}]}}]}`)
			host.CallOnHttpStreamingResponseBody(chunk4, false)

			// 结束
			chunk5 := []byte(`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`)
			host.CallOnHttpStreamingResponseBody(chunk5, true)

			host.CompleteHttp()
		})

		// 测试非流式 tool_calls
		t.Run("non-streaming tool calls", func(t *testing.T) {
			host, status := test.NewTestHost(builtinAttributesConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 1. 处理请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			// 2. 处理请求体
			requestBody := []byte(`{
				"model": "gpt-4",
				"messages": [{"role": "user", "content": "What's the weather?"}]
			}`)
			host.CallOnHttpRequestBody(requestBody)

			// 3. 处理响应头
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})

			// 4. 处理响应体 - 非流式 tool_calls
			responseBody := []byte(`{
				"choices": [{
					"message": {
						"role": "assistant",
						"content": null,
						"tool_calls": [{
							"id": "call_abc123",
							"type": "function",
							"function": {
								"name": "get_weather",
								"arguments": "{\"location\": \"Beijing\"}"
							}
						}]
					},
					"finish_reason": "tool_calls"
				}],
				"model": "gpt-4",
				"usage": {"prompt_tokens": 20, "completion_tokens": 15, "total_tokens": 35}
			}`)
			action := host.CallOnHttpResponseBody(responseBody)
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})
	})
}

// TestSessionIdExtraction 测试 session_id 提取
func TestSessionIdExtraction(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试自定义 session_id header
		t.Run("custom session id header", func(t *testing.T) {
			host, status := test.NewTestHost(sessionIdConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 处理请求头 - 带自定义 session header
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"x-custom-session", "sess_custom_123"},
			})
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})

		// 测试默认 session_id headers 优先级
		t.Run("default session id headers priority", func(t *testing.T) {
			host, status := test.NewTestHost(builtinAttributesConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 处理请求头 - 带多个默认 session headers，应该使用优先级最高的
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"x-agent-session", "sess_agent_456"},
				{"x-clawdbot-session-key", "sess_clawdbot_789"},
				{"x-openclaw-session-key", "sess_openclaw_123"}, // 最高优先级
			})
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})

		// 测试 fallback 到次优先级 header
		t.Run("session id fallback", func(t *testing.T) {
			host, status := test.NewTestHost(builtinAttributesConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 处理请求头 - 只有低优先级的 session header
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"x-agent-session", "sess_agent_only"},
			})
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})
	})
}

// TestExtractStreamingBodyByJsonPath 单独测试流式响应 body 的 JSONPath 提取规则
func TestExtractStreamingBodyByJsonPath(t *testing.T) {
	t.Run("first skips empty string chunk", func(t *testing.T) {
		// Azure/OpenAI 兼容流可能先返回带空 model 的过滤结果 chunk，后续 chunk 才有真实模型名。
		chunks := []byte(`data: {"choices":[],"created":0,"id":"","model":"","object":""}

data: {"choices":[{"delta":{"content":""}}],"created":1777444731,"id":"chatcmpl-1","model":"gpt-5.4-2026-03-05","object":"chat.completion.chunk"}`)

		value := extractStreamingBodyByJsonPath(chunks, "model", RuleFirst)

		require.Equal(t, "gpt-5.4-2026-03-05", value)
	})

	t.Run("replace skips trailing empty string chunk", func(t *testing.T) {
		chunks := []byte(`data: {"model":"gpt-4o"}

data: {"model":""}`)

		value := extractStreamingBodyByJsonPath(chunks, "model", RuleReplace)

		require.Equal(t, "gpt-4o", value)
	})

	t.Run("first returns nil when path is missing in all chunks", func(t *testing.T) {
		chunks := []byte(`data: {"choices":[]}

data: {"choices":[{"delta":{"content":"hello"}}]}`)

		value := extractStreamingBodyByJsonPath(chunks, "model", RuleFirst)

		require.Nil(t, value)
	})

	t.Run("first skips explicit null chunk", func(t *testing.T) {
		chunks := []byte(`data: {"model":null}

data: {"model":"gpt-4o"}`)

		value := extractStreamingBodyByJsonPath(chunks, "model", RuleFirst)

		require.Equal(t, "gpt-4o", value)
	})

	t.Run("zero and false remain valid values", func(t *testing.T) {
		numberValue := extractStreamingBodyByJsonPath([]byte(`data: {"usage":{"total_tokens":0}}`), "usage.total_tokens", RuleFirst)
		boolValue := extractStreamingBodyByJsonPath([]byte(`data: {"filtered":false}`), "filtered", RuleFirst)

		require.Equal(t, float64(0), numberValue)
		require.Equal(t, false, boolValue)
	})
}

// TestExtractStreamingToolCalls 单独测试 extractStreamingToolCalls 函数
func TestExtractStreamingToolCalls(t *testing.T) {
	t.Run("single tool call assembly", func(t *testing.T) {
		// 模拟流式 chunks
		chunks := [][]byte{
			[]byte(`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_123","type":"function","function":{"name":"get_weather","arguments":""}}]}}]}`),
			[]byte(`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"loc"}}]}}]}`),
			[]byte(`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"ation"}}]}}]}`),
			[]byte(`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\":\"Beijing\"}"}}]}}]}`),
		}

		var buffer *StreamingToolCallsBuffer
		for _, chunk := range chunks {
			buffer = extractStreamingToolCalls(chunk, buffer)
		}

		toolCalls := getToolCallsFromBuffer(buffer)
		require.Len(t, toolCalls, 1)
		require.Equal(t, "call_123", toolCalls[0].ID)
		require.Equal(t, "function", toolCalls[0].Type)
		require.Equal(t, "get_weather", toolCalls[0].Function.Name)
		require.Equal(t, `{"location":"Beijing"}`, toolCalls[0].Function.Arguments)
	})

	t.Run("multiple tool calls assembly", func(t *testing.T) {
		chunks := [][]byte{
			[]byte(`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_001","type":"function","function":{"name":"get_weather","arguments":""}}]}}]}`),
			[]byte(`{"choices":[{"delta":{"tool_calls":[{"index":1,"id":"call_002","type":"function","function":{"name":"get_time","arguments":""}}]}}]}`),
			[]byte(`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"city\":\"Beijing\"}"}}]}}]}`),
			[]byte(`{"choices":[{"delta":{"tool_calls":[{"index":1,"function":{"arguments":"{\"timezone\":\"UTC+8\"}"}}]}}]}`),
		}

		var buffer *StreamingToolCallsBuffer
		for _, chunk := range chunks {
			buffer = extractStreamingToolCalls(chunk, buffer)
		}

		toolCalls := getToolCallsFromBuffer(buffer)
		require.Len(t, toolCalls, 2)

		// 验证第一个 tool call
		require.Equal(t, "call_001", toolCalls[0].ID)
		require.Equal(t, "get_weather", toolCalls[0].Function.Name)
		require.Equal(t, `{"city":"Beijing"}`, toolCalls[0].Function.Arguments)

		// 验证第二个 tool call
		require.Equal(t, "call_002", toolCalls[1].ID)
		require.Equal(t, "get_time", toolCalls[1].Function.Name)
		require.Equal(t, `{"timezone":"UTC+8"}`, toolCalls[1].Function.Arguments)
	})

	t.Run("empty chunks", func(t *testing.T) {
		chunks := [][]byte{
			[]byte(`{"choices":[{"delta":{}}]}`),
			[]byte(`{"choices":[{"delta":{"content":"Hello"}}]}`),
		}

		var buffer *StreamingToolCallsBuffer
		for _, chunk := range chunks {
			buffer = extractStreamingToolCalls(chunk, buffer)
		}

		toolCalls := getToolCallsFromBuffer(buffer)
		require.Len(t, toolCalls, 0)
	})
}

// TestBuiltinAttributeHelpers 测试内置属性辅助函数
func TestBuiltinAttributeHelpers(t *testing.T) {
	t.Run("isBuiltinAttribute", func(t *testing.T) {
		require.True(t, isBuiltinAttribute("question"))
		require.True(t, isBuiltinAttribute("answer"))
		require.True(t, isBuiltinAttribute("tool_calls"))
		require.True(t, isBuiltinAttribute("reasoning"))
		require.False(t, isBuiltinAttribute("custom_key"))
		require.False(t, isBuiltinAttribute("model"))
	})

	t.Run("getBuiltinAttributeDefaultSources", func(t *testing.T) {
		// question 应该默认从 request_body 提取
		questionSources := getBuiltinAttributeDefaultSources("question")
		require.Equal(t, []string{RequestBody}, questionSources)

		// answer 应该支持 streaming 和 non-streaming
		answerSources := getBuiltinAttributeDefaultSources("answer")
		require.Contains(t, answerSources, ResponseStreamingBody)
		require.Contains(t, answerSources, ResponseBody)

		// tool_calls 应该支持 streaming 和 non-streaming
		toolCallsSources := getBuiltinAttributeDefaultSources("tool_calls")
		require.Contains(t, toolCallsSources, ResponseStreamingBody)
		require.Contains(t, toolCallsSources, ResponseBody)

		// reasoning 应该支持 streaming 和 non-streaming
		reasoningSources := getBuiltinAttributeDefaultSources("reasoning")
		require.Contains(t, reasoningSources, ResponseStreamingBody)
		require.Contains(t, reasoningSources, ResponseBody)

		// 非内置属性应该返回 nil
		customSources := getBuiltinAttributeDefaultSources("custom_key")
		require.Nil(t, customSources)
	})

	t.Run("shouldProcessBuiltinAttribute", func(t *testing.T) {
		// 配置了 value_source 时，应该精确匹配
		require.True(t, shouldProcessBuiltinAttribute("question", RequestBody, RequestBody))
		require.False(t, shouldProcessBuiltinAttribute("question", RequestBody, ResponseBody))

		// 没有配置 value_source 时，内置属性应该使用默认 source
		require.True(t, shouldProcessBuiltinAttribute("question", "", RequestBody))
		require.False(t, shouldProcessBuiltinAttribute("question", "", ResponseBody))

		require.True(t, shouldProcessBuiltinAttribute("answer", "", ResponseBody))
		require.True(t, shouldProcessBuiltinAttribute("answer", "", ResponseStreamingBody))
		require.False(t, shouldProcessBuiltinAttribute("answer", "", RequestBody))

		// 非内置属性没有配置 value_source 时，不应该处理
		require.False(t, shouldProcessBuiltinAttribute("custom_key", "", RequestBody))
		require.False(t, shouldProcessBuiltinAttribute("custom_key", "", ResponseBody))
	})
}

// TestSessionIdDebugOutput 演示session_id的debug日志输出
func TestSessionIdDebugOutput(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("session id with full flow", func(t *testing.T) {
			host, status := test.NewTestHost(sessionIdConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 1. 处理请求头 - 带 session_id
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"x-custom-session", "sess_abc123xyz"},
			})

			// 2. 处理请求体
			requestBody := []byte(`{
				"model": "gpt-4",
				"messages": [
					{"role": "user", "content": "What is 2+2?"}
				]
			}`)
			host.CallOnHttpRequestBody(requestBody)

			// 3. 处理响应头
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})

			// 4. 处理响应体
			responseBody := []byte(`{
				"choices": [{"message": {"role": "assistant", "content": "2+2 equals 4."}}],
				"model": "gpt-4",
				"usage": {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}
			}`)
			host.CallOnHttpResponseBody(responseBody)

			host.CompleteHttp()
		})
	})
}

// 测试配置：Token Details 配置
var tokenDetailsConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"attributes": []map[string]interface{}{
			{
				"key":          "reasoning_tokens",
				"apply_to_log": true,
			},
			{
				"key":          "cached_tokens",
				"apply_to_log": true,
			},
			{
				"key":          "input_token_details",
				"apply_to_log": true,
			},
			{
				"key":          "output_token_details",
				"apply_to_log": true,
			},
		},
		"disable_openai_usage": false,
	})
	return data
}()

// TestTokenDetails 测试 token details 功能
func TestTokenDetails(t *testing.T) {
	t.Run("test builtin token details attributes", func(t *testing.T) {
		host, status := test.NewTestHost(tokenDetailsConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		// 设置路由和集群名称
		host.SetRouteName("api-v1")
		host.SetClusterName("cluster-1")

		// 1. 处理请求头
		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/v1/chat/completions"},
			{":method", "POST"},
		})
		require.Equal(t, types.ActionContinue, action)

		// 2. 处理请求体
		requestBody := []byte(`{
			"model": "gpt-4o",
			"messages": [
				{"role": "user", "content": "Test question"}
			]
		}`)
		action = host.CallOnHttpRequestBody(requestBody)
		require.Equal(t, types.ActionContinue, action)

		// 3. 处理响应头
		action = host.CallOnHttpResponseHeaders([][2]string{
			{":status", "200"},
			{"content-type", "application/json"},
		})
		require.Equal(t, types.ActionContinue, action)

		// 4. 处理响应体（包含 token details）
		responseBody := []byte(`{
			"id": "chatcmpl-123",
			"object": "chat.completion",
			"created": 1677652288,
			"model": "gpt-4o",
			"usage": {
				"prompt_tokens": 100,
				"completion_tokens": 50,
				"total_tokens": 150,
				"completion_tokens_details": {
					"reasoning_tokens": 25
				},
				"prompt_tokens_details": {
					"cached_tokens": 80
				}
			},
			"choices": [{
				"message": {
					"role": "assistant",
					"content": "Test answer"
				},
				"finish_reason": "stop"
			}]
		}`)
		action = host.CallOnHttpResponseBody(responseBody)
		require.Equal(t, types.ActionContinue, action)

		// 5. 完成请求
		host.CompleteHttp()
	})
}

func TestUnmatchedPathsAndContentTypes(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		restrictiveConfig := func() json.RawMessage {
			data, _ := json.Marshal(map[string]interface{}{
				"enable_path_suffixes": []string{"/allowed_path"},
				"enable_content_types": []string{"application/json"},
				"attributes": []map[string]interface{}{
					{
						"key":          "test_attr",
						"value_source": "response_body",
						"value":        "data",
						"apply_to_log": true,
					},
				},
				"disable_openai_usage": true,
			})
			return data
		}()

		t.Run("skip request for unenabled path", func(t *testing.T) {
			host, status := test.NewTestHost(restrictiveConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/disallowed_path"},
				{":method", "POST"},
			})
			require.Equal(t, types.ActionContinue, action)
			host.CompleteHttp()
		})

		t.Run("skip response for unenabled content type", func(t *testing.T) {
			host, status := test.NewTestHost(restrictiveConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/allowed_path"},
				{":method", "POST"},
			})

			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "text/plain"},
			})
			require.Equal(t, types.ActionContinue, action)
			host.CompleteHttp()
		})
	})
}

func TestSetSpanAttributeAndLoggingEdgeCases(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		configBytes := []byte(`{
			"attributes": [
				{
					"key": "test_attr1",
					"value_source": "fixed_value",
					"value": "",
					"apply_to_span": true
				},
				{
					"key": "test_attr2",
					"value_source": "fixed_value",
					"value": "long_value_that_exceeds_limit_long_value_that_exceeds_limit_long_value_that_exceeds_limit",
					"apply_to_log": true
				}
			],
			"value_length_limit": 20
		}`)

		t.Run("span attribute edge cases", func(t *testing.T) {
			host, status := test.NewTestHost(configBytes)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// Setting fixed value attribute to empty should just print a debug log and skip setting span
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
			})
			require.Equal(t, types.ActionContinue, action)
			host.CompleteHttp()
		})
	})
}

func TestGetRouteAndClusterNameEdgeCases(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("properties absence", func(t *testing.T) {
			host, status := test.NewTestHost([]byte(`{}`))
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// Host doesn't have route_name implicitly by default without SetRouteName, but getRouteName handles err
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
			})
			host.CompleteHttp()
		})

		t.Run("api name with @", func(t *testing.T) {
			host, status := test.NewTestHost([]byte(`{}`))
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.SetRouteName("api@v1@service@extra") // @ has special handling in getAPIName
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
			})
			host.CompleteHttp()
		})
	})
}

func TestExtractClaudeStreamingToolCallsMissingInput(t *testing.T) {
	t.Run("claude missing partial_json", func(t *testing.T) {
		chunks := [][]byte{
			[]byte(`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"tool_123","name":"get_weather","input":{}}}`),
			[]byte(`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta"}}`),
			[]byte(`data: {"type":"content_block_stop","index":0}`),
		}

		var buffer *StreamingToolCallsBuffer
		for _, chunk := range chunks {
			buffer = extractClaudeStreamingToolCalls(chunk, buffer)
		}

		toolCalls := getToolCallsFromBuffer(buffer)
		require.Len(t, toolCalls, 1)
		require.Equal(t, "tool_123", toolCalls[0].ID)
		require.Equal(t, "tool_use", toolCalls[0].Type)
		require.Equal(t, "get_weather", toolCalls[0].Function.Name)
		// partial_json absence means arguments might be empty
	})
}

func TestWriteMetricEdgeCases(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("disable_openai_usage true", func(t *testing.T) {
			configBytes := []byte(`{
				"disable_openai_usage": true
			}`)
			host, status := test.NewTestHost(configBytes)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.SetRouteName("api-v1")
			host.SetClusterName("cluster-1")

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
			})

			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})

			responseBody := []byte(`{
				"usage": {"prompt_tokens": 5, "completion_tokens": 8, "total_tokens": 13},
				"model": "gpt-3.5-turbo"
			}`)
			host.CallOnHttpResponseBody(responseBody)
			host.CompleteHttp()
		})
	})
}

func TestIsPathEnabled(t *testing.T) {
	require.True(t, isPathEnabled("/v1/chat/completions", nil))
	require.True(t, isPathEnabled("/v1/chat/completions", []string{}))
	require.True(t, isPathEnabled("/v1/chat/completions", []string{"/completions", "/messages"}))
	require.True(t, isPathEnabled("/v1/messages", []string{"/completions", "/messages"}))
	require.False(t, isPathEnabled("/v1/embeddings", []string{"/completions", "/messages"}))

	// test query params
	require.True(t, isPathEnabled("/v1/chat/completions?stream=true", []string{"/completions"}))
	require.False(t, isPathEnabled("/v1/embeddings?stream=true", []string{"/completions"}))
}

func TestIsContentTypeEnabled(t *testing.T) {
	require.True(t, isContentTypeEnabled("application/json", nil))
	require.True(t, isContentTypeEnabled("application/json", []string{}))
	require.True(t, isContentTypeEnabled("application/json", []string{"application/json", "text/event-stream"}))
	require.True(t, isContentTypeEnabled("text/event-stream; charset=utf-8", []string{"application/json", "text/event-stream"}))
	require.False(t, isContentTypeEnabled("text/html", []string{"application/json", "text/event-stream"}))
}

func TestConvertToUInt(t *testing.T) {
	val, ok := convertToUInt(int32(10))
	require.True(t, ok)
	require.Equal(t, uint64(10), val)

	val, ok = convertToUInt(int64(10))
	require.True(t, ok)
	require.Equal(t, uint64(10), val)

	val, ok = convertToUInt(uint32(10))
	require.True(t, ok)
	require.Equal(t, uint64(10), val)

	val, ok = convertToUInt(uint64(10))
	require.True(t, ok)
	require.Equal(t, uint64(10), val)

	val, ok = convertToUInt(float32(10.0))
	require.True(t, ok)
	require.Equal(t, uint64(10), val)

	val, ok = convertToUInt(float64(10.0))
	require.True(t, ok)
	require.Equal(t, uint64(10), val)

	_, ok = convertToUInt("10")
	require.False(t, ok)
}

func TestExtractClaudeStreamingToolCalls(t *testing.T) {
	t.Run("claude tool use assembly", func(t *testing.T) {
		chunks := [][]byte{
			[]byte(`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"tool_123","name":"get_weather"}}`),
			[]byte(`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"loc"}}}`),
			[]byte(`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"ation\":\"Bei"}}}`),
			[]byte(`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"jing\"}"}}}`),
			[]byte(`data: {"type":"content_block_stop","index":0}`),
		}

		var buffer *StreamingToolCallsBuffer
		for _, chunk := range chunks {
			buffer = extractClaudeStreamingToolCalls(chunk, buffer)
		}

		toolCalls := getToolCallsFromBuffer(buffer)
		require.Len(t, toolCalls, 1)
		require.Equal(t, "tool_123", toolCalls[0].ID)
		require.Equal(t, "tool_use", toolCalls[0].Type)
		require.Equal(t, "get_weather", toolCalls[0].Function.Name)
		require.Equal(t, `{"location":"Beijing"}`, toolCalls[0].Function.Arguments)
	})

	t.Run("claude empty chunks", func(t *testing.T) {
		chunks := [][]byte{
			[]byte(`data: {"type":"ping"}`),
			[]byte(`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`),
		}
		var buffer *StreamingToolCallsBuffer
		for _, chunk := range chunks {
			buffer = extractClaudeStreamingToolCalls(chunk, buffer)
		}
		toolCalls := getToolCallsFromBuffer(buffer)
		require.Len(t, toolCalls, 0)
	})

	t.Run("claude tool use with initial input", func(t *testing.T) {
		chunks := [][]byte{
			[]byte(`data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"tool_456","name":"get_time","input":{"timezone":"UTC+8"}}}`),
			[]byte(`data: {"type":"content_block_stop","index":1}`),
		}

		var buffer *StreamingToolCallsBuffer
		for _, chunk := range chunks {
			buffer = extractClaudeStreamingToolCalls(chunk, buffer)
		}

		toolCalls := getToolCallsFromBuffer(buffer)
		require.Len(t, toolCalls, 1)
		require.Equal(t, "tool_456", toolCalls[0].ID)
		require.Equal(t, "tool_use", toolCalls[0].Type)
		require.Equal(t, "get_time", toolCalls[0].Function.Name)
		require.Equal(t, `{"timezone":"UTC+8"}`, toolCalls[0].Function.Arguments)
	})
}

func TestConfigWithDefaultAttributes(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("use default attributes config", func(t *testing.T) {
			defaultConfig := []byte(`{
				"use_default_attributes": true
			}`)
			host, status := test.NewTestHost(defaultConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)
		})

		t.Run("use default response attributes config", func(t *testing.T) {
			defaultRespConfig := []byte(`{
				"use_default_response_attributes": true
			}`)
			host, status := test.NewTestHost(defaultRespConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)
		})
	})
}

func TestIsErrorResponse(t *testing.T) {
	// Test error body detection (OpenAI/Anthropic format)
	t.Run("error object in body", func(t *testing.T) {
		body := []byte(`{"error": {"type": "api_error", "message": "Unauthorized"}}`)
		require.True(t, isErrorResponse(body))
	})

	t.Run("error object with only code", func(t *testing.T) {
		body := []byte(`{"error": {"code": "invalid_api_key"}}`)
		require.True(t, isErrorResponse(body))
	})

	t.Run("error string in body", func(t *testing.T) {
		body := []byte(`{"error": "Something went wrong"}`)
		require.True(t, isErrorResponse(body))
	})

	t.Run("empty error string is not counted", func(t *testing.T) {
		body := []byte(`{"error": ""}`)
		require.False(t, isErrorResponse(body))
	})

	t.Run("null error is not counted", func(t *testing.T) {
		body := []byte(`{"error": null}`)
		require.False(t, isErrorResponse(body))
	})

	t.Run("no error field in body", func(t *testing.T) {
		body := []byte(`{"choices": [{"message": {"content": "Hi"}}], "model": "gpt-4"}`)
		require.False(t, isErrorResponse(body))
	})

	t.Run("nested error field not at root", func(t *testing.T) {
		body := []byte(`{"choices": [{"error": "x"}]}`)
		require.False(t, isErrorResponse(body))
	})

	// Empty body with HTTP status fallback is tested in TestIsErrorResponseWithHTTPStatus
	// (requires wasm test environment for proxywasm.GetHttpResponseHeader)
}

func TestIsErrorResponseWithHTTPStatus(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("empty body with 401 status returns true", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
			})
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "401"},
				{"content-type", "application/json"},
			})

			require.True(t, isErrorResponse([]byte{}))
			host.CompleteHttp()
		})

		t.Run("empty body with 200 status returns false", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
			})
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})

			require.False(t, isErrorResponse([]byte{}))
			host.CompleteHttp()
		})
	})
}

func TestFailureCountMetric(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("error response body increments failure count only", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.SetRouteName("api-v1")
			host.SetClusterName("cluster-1")

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"x-mse-consumer", "user1"},
			})

			requestBody := []byte(`{"model": "gpt-3.5-turbo", "messages": [{"role": "user", "content": "Hello"}]}`)
			host.CallOnHttpRequestBody(requestBody)

			time.Sleep(10 * time.Millisecond)

			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "401"},
				{"content-type", "application/json"},
			})
			errorBody := []byte(`{"error": {"type": "authentication_error", "message": "Invalid API key"}}`)
			host.CallOnHttpResponseBody(errorBody)

			host.CompleteHttp()

			// Verify llm_failure_count is incremented
			failureMetric := "route.api-v1.upstream.cluster-1.model.gpt-3.5-turbo.consumer.user1.metric.llm_failure_count"
			failureValue, err := host.GetCounterMetric(failureMetric)
			require.NoError(t, err)
			require.Equal(t, uint64(1), failureValue)

			// Verify success metrics are NOT present (usage info unavailable in error response)
			durationCountMetric := "route.api-v1.upstream.cluster-1.model.gpt-3.5-turbo.consumer.user1.metric.llm_duration_count"
			_, err = host.GetCounterMetric(durationCountMetric)
			require.Error(t, err, "llm_duration_count should not exist for error response without usage")
		})

		t.Run("success response does not increment failure count", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.SetRouteName("api-v1")
			host.SetClusterName("cluster-1")

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"x-mse-consumer", "user1"},
			})

			requestBody := []byte(`{"model": "gpt-4", "messages": [{"role": "user", "content": "Hello"}]}`)
			host.CallOnHttpRequestBody(requestBody)

			time.Sleep(10 * time.Millisecond)

			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})
			responseBody := []byte(`{
				"choices": [{"message": {"content": "Hello!"}}],
				"usage": {"prompt_tokens": 5, "completion_tokens": 3, "total_tokens": 8},
				"model": "gpt-4"
			}`)
			host.CallOnHttpResponseBody(responseBody)

			host.CompleteHttp()

			// Verify llm_failure_count is NOT present
			failureMetric := "route.api-v1.upstream.cluster-1.model.gpt-4.consumer.user1.metric.llm_failure_count"
			_, err := host.GetCounterMetric(failureMetric)
			require.Error(t, err, "llm_failure_count should not exist for successful response")

			// Verify success metrics exist
			durationCountMetric := "route.api-v1.upstream.cluster-1.model.gpt-4.consumer.user1.metric.llm_duration_count"
			durationCountValue, err := host.GetCounterMetric(durationCountMetric)
			require.NoError(t, err)
			require.Equal(t, uint64(1), durationCountValue)
		})
	})
}

func TestStreamingFailureCountMetric(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("sse error in middle chunk before done", func(t *testing.T) {
			host, status := test.NewTestHost(streamingBodyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.SetRouteName("api-v1")
			host.SetClusterName("cluster-1")

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"x-mse-consumer", "user1"},
			})

			requestBody := []byte(`{"model": "gpt-4", "messages": [{"role": "user", "content": "Hello"}]}`)
			host.CallOnHttpRequestBody(requestBody)

			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "text/event-stream"},
			})

			// Simulate SSE stream: error appears in middle chunk, last chunk is [DONE]
			// (events are terminated with the SSE delimiter \n\n).
			chunk1 := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n")
			host.CallOnHttpStreamingResponseBody(chunk1, false)

			chunk2 := []byte("data: {\"error\":{\"type\":\"api_error\",\"message\":\"Something went wrong\"}}\n\n")
			host.CallOnHttpStreamingResponseBody(chunk2, false)

			chunk3 := []byte("data: [DONE]\n\n")
			host.CallOnHttpStreamingResponseBody(chunk3, true)

			host.CompleteHttp()

			// Verify llm_failure_count is incremented despite [DONE] being the last chunk
			failureMetric := "route.api-v1.upstream.cluster-1.model.gpt-4.consumer.user1.metric.llm_failure_count"
			failureValue, err := host.GetCounterMetric(failureMetric)
			require.NoError(t, err)
			require.Equal(t, uint64(1), failureValue)
		})
	})
}

// ==================== SSE framer integration tests (Design #4249) ====================
//
// These tests drive onHttpStreamingBody through the proxy-wasm test host with
// controlled byte boundaries and pin the consumer-level rows of the Design
// test matrix: token usage and stream-error detection consume reassembled
// complete SSE events from the request-scoped framer, so split and intact
// delivery are observably equivalent, and original response bytes pass through
// unchanged on every callback. Scanner-level rows (T-05..T-08, T-13..T-15,
// T-22, T-23, T-25, T-26 byte-boundary details) are covered in
// sse_framer_test.go; the tests below assert the end-to-end outcomes.

// setupStreamingHost starts a request scoped to a streaming (SSE) response:
// request headers + body (model gpt-4) + text/event-stream response headers,
// with route api-v1 / cluster cluster-1 / consumer user1 for metric labels.
// The caller must defer host.Reset().
func setupStreamingHost(t *testing.T, config json.RawMessage) test.TestHost {
	t.Helper()
	host, status := test.NewTestHost(config)
	require.Equal(t, types.OnPluginStartStatusOK, status)

	host.SetRouteName("api-v1")
	host.SetClusterName("cluster-1")

	action := host.CallOnHttpRequestHeaders([][2]string{
		{":authority", "example.com"},
		{":path", "/v1/chat/completions"},
		{":method", "POST"},
		{"x-mse-consumer", "user1"},
	})
	require.Equal(t, types.ActionContinue, action)

	action = host.CallOnHttpRequestBody([]byte(`{"model": "gpt-4", "messages": [{"role": "user", "content": "Hello"}]}`))
	require.Equal(t, types.ActionContinue, action)

	action = host.CallOnHttpResponseHeaders([][2]string{
		{":status", "200"},
		{"content-type", "text/event-stream"},
	})
	require.Equal(t, types.ActionContinue, action)
	return host
}

// sseEvent wraps a payload in one complete SSE event with an LF delimiter.
func sseEvent(payload string) []byte {
	return []byte("data: " + payload + "\n\n")
}

// concatStreamBytes concatenates byte slices into a fresh slice.
func concatStreamBytes(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// deliverStreamChunk feeds one host callback and asserts the fail-open
// passthrough contract: the plugin must return the original bytes unchanged.
func deliverStreamChunk(t *testing.T, host test.TestHost, chunk []byte, endOfStream bool) {
	t.Helper()
	action := host.CallOnHttpStreamingResponseBody(chunk, endOfStream)
	require.Equal(t, types.ActionContinue, action)
	require.Equal(t, chunk, host.GetResponseBody(), "original response bytes must pass through unchanged")
}

// deliverStreamChunks feeds each chunk as one host callback, with endOfStream
// set on the last chunk, asserting passthrough on every callback.
func deliverStreamChunks(t *testing.T, host test.TestHost, chunks ...[]byte) {
	t.Helper()
	for i, chunk := range chunks {
		deliverStreamChunk(t, host, chunk, i == len(chunks)-1)
	}
}

// aiLogInt64 reads a numeric attribute from the parsed ai_log property.
func aiLogInt64(attrs map[string]interface{}, key string) (int64, bool) {
	v, ok := attrs[key]
	if !ok {
		return 0, false
	}
	f, ok := v.(float64)
	if !ok {
		return 0, false
	}
	return int64(f), true
}

// assertTokenAttrs asserts the final recorded token usage in ai_log.
func assertTokenAttrs(t *testing.T, attrs map[string]interface{}, input, output, total int64) {
	t.Helper()
	in, ok := aiLogInt64(attrs, tokenusage.CtxKeyInputToken)
	require.True(t, ok, "input_token missing from ai_log")
	require.Equal(t, input, in)
	out, ok := aiLogInt64(attrs, tokenusage.CtxKeyOutputToken)
	require.True(t, ok, "output_token missing from ai_log")
	require.Equal(t, output, out)
	tot, ok := aiLogInt64(attrs, tokenusage.CtxKeyTotalToken)
	require.True(t, ok, "total_token missing from ai_log")
	require.Equal(t, total, tot)
}

// assertNoTokenAttrs asserts no token usage was recorded (zero-fabrication).
func assertNoTokenAttrs(t *testing.T, attrs map[string]interface{}) {
	t.Helper()
	for _, key := range []string{tokenusage.CtxKeyInputToken, tokenusage.CtxKeyOutputToken, tokenusage.CtxKeyTotalToken} {
		_, ok := aiLogInt64(attrs, key)
		require.False(t, ok, "%s must not be fabricated", key)
	}
}

// getSpanValue reads an ARMS span property written by setSpanAttribute.
func getSpanValue(host test.TestHost, key string) (string, bool) {
	raw, err := host.GetProperty([]string{wrapper.TraceSpanTagPrefix + key})
	if err != nil || len(raw) == 0 {
		return "", false
	}
	return string(raw), true
}

// streamingMetricName builds the counter name for the standard test flow.
func streamingMetricName(model, metric string) string {
	return fmt.Sprintf("route.api-v1.upstream.cluster-1.model.%s.consumer.user1.metric.%s", model, metric)
}

// assertTokenMetrics asserts the recorded token counter metrics.
func assertTokenMetrics(t *testing.T, host test.TestHost, model string, input, output, total uint64) {
	t.Helper()
	for metric, want := range map[string]uint64{
		tokenusage.CtxKeyInputToken:  input,
		tokenusage.CtxKeyOutputToken: output,
		tokenusage.CtxKeyTotalToken:  total,
	} {
		got, err := host.GetCounterMetric(streamingMetricName(model, metric))
		require.NoError(t, err)
		require.Equal(t, want, got, "metric %s", metric)
	}
}

// TestStreamingIntactUsageEventParity covers T-01: a single intact usage event
// in one callback recovers the upstream values, and the ai_log content is
// byte-equivalent to the pre-change behavior (dynamic duration fields aside).
func TestStreamingIntactUsageEventParity(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("intact usage event", func(t *testing.T) {
			host := setupStreamingHost(t, emptyAttributesConfig)
			defer host.Reset()

			contentEvent := sseEvent(`{"id":"chatcmpl-parity","choices":[{"delta":{"content":"Hello"}}],"model":"gpt-4"}`)
			usageEvent := sseEvent(`{"choices":[],"model":"gpt-4","usage":{"prompt_tokens":5,"completion_tokens":8,"total_tokens":13}}`)
			deliverStreamChunks(t, host, contentEvent, usageEvent)

			attrs := getAILogAttributes(t, host)
			// Durations are time-based; assert presence and exclude them from
			// the exact-content comparison.
			_, ok := aiLogInt64(attrs, LLMFirstTokenDuration)
			require.True(t, ok, "llm_first_token_duration missing")
			_, ok = aiLogInt64(attrs, LLMServiceDuration)
			require.True(t, ok, "llm_service_duration missing")
			delete(attrs, LLMFirstTokenDuration)
			delete(attrs, LLMServiceDuration)

			// Byte-equivalence with pre-change behavior: the framed complete
			// event is exactly the bytes the raw-chunk path used to see, and an
			// event without details leaves empty detail maps in the log.
			require.Equal(t, map[string]interface{}{
				"api":                  "-",
				"chat_round":           float64(1),
				"response_type":        "stream",
				"chat_id":              "chatcmpl-parity",
				"model":                "gpt-4",
				"input_token":          float64(5),
				"output_token":         float64(8),
				"total_token":          float64(13),
				"input_token_details":  map[string]interface{}{},
				"output_token_details": map[string]interface{}{},
			}, attrs)

			spanTotal, ok := getSpanValue(host, ArmsTotalToken)
			require.True(t, ok)
			require.Equal(t, "13", spanTotal)
			spanInput, ok := getSpanValue(host, ArmsInputToken)
			require.True(t, ok)
			require.Equal(t, "5", spanInput)
			spanOutput, ok := getSpanValue(host, ArmsOutputToken)
			require.True(t, ok)
			require.Equal(t, "8", spanOutput)
			spanModel, ok := getSpanValue(host, ArmsModelName)
			require.True(t, ok)
			require.Equal(t, "gpt-4", spanModel)

			assertTokenMetrics(t, host, "gpt-4", 5, 8, 13)
			host.CompleteHttp()
		})
	})
}

// TestStreamingSplitUsageEventRecovery covers T-02, T-03, T-17 and T-23: a
// usage event split across host callbacks at arbitrary byte boundaries is
// reassembled by the framer and recovers exactly the same tokens as intact
// delivery, including with shouldBufferStreamingBody disabled (the config used
// here buffers nothing) and with one-byte callbacks.
func TestStreamingSplitUsageEventRecovery(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		payload := `{"id":"chatcmpl-split","object":"chat.completion.chunk","created":1710000000,"model":"gpt-4","choices":[{"index":0,"delta":{}}],"usage":{"prompt_tokens":11,"completion_tokens":22,"total_tokens":33}}`
		event := sseEvent(payload)
		require.Greater(t, len(event), 141, "pinned split offsets must be inside the event")

		// assertRecovered runs one full request over the given chunks and
		// asserts the recovered final usage matches the intact-delivery values.
		assertRecovered := func(t *testing.T, chunks ...[]byte) {
			host := setupStreamingHost(t, emptyAttributesConfig)
			defer host.Reset()
			deliverStreamChunks(t, host, chunks...)
			attrs := getAILogAttributes(t, host)
			assertTokenAttrs(t, attrs, 11, 22, 33)
			require.Equal(t, "gpt-4", attrs["model"])
			host.CompleteHttp()
		}

		// T-02: splits inside the JSON body at the Design-pinned offsets.
		for _, off := range []int{50, 100, 120, 130, 140} {
			t.Run(fmt.Sprintf("split_at_offset_%d", off), func(t *testing.T) {
				assertRecovered(t, event[:off], event[off:])
			})
		}

		// T-03: a split inside the "data:" prefix reassembles by byte shape.
		t.Run("split_inside_data_prefix", func(t *testing.T) {
			assertRecovered(t, event[:2], event[2:])
		})

		// T-23: one event delivered across many 1-byte callbacks is emitted on
		// the completing callback and recovers the same tokens.
		t.Run("one_byte_per_callback", func(t *testing.T) {
			chunks := make([][]byte, 0, len(event))
			for i := 0; i < len(event); i++ {
				chunks = append(chunks, event[i:i+1])
			}
			assertRecovered(t, chunks...)
		})

		// T-17: with shouldBufferStreamingBody == false (emptyAttributesConfig
		// configures no streaming-body attribute), token recovery still works
		// via the framer's bounded tail, end to end down to the metrics.
		t.Run("buffering_disabled_still_recovers_usage", func(t *testing.T) {
			host := setupStreamingHost(t, emptyAttributesConfig)
			defer host.Reset()

			deliverStreamChunks(t, host, event[:100], event[100:])
			attrs := getAILogAttributes(t, host)
			assertTokenAttrs(t, attrs, 11, 22, 33)
			assertTokenMetrics(t, host, "gpt-4", 11, 22, 33)
			host.CompleteHttp()
		})
	})
}

// TestStreamingDelimiterSplitRecovery covers T-04 and T-05 at integration
// level: LF and CRLF delimiters split across callbacks (at every byte
// boundary) are recognized only once complete, and the event recovers the same
// tokens as intact delivery. (Exact emission counts per split point are
// asserted in sse_framer_test.go.)
func TestStreamingDelimiterSplitRecovery(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		payload := `{"choices":[],"model":"gpt-4","usage":{"prompt_tokens":3,"completion_tokens":4,"total_tokens":7}}`

		// T-04: an LF delimiter split across callbacks emits the event only
		// after the second \n arrives; usage is recovered mid-stream, before
		// end-of-stream.
		t.Run("lf_delimiter_split", func(t *testing.T) {
			host := setupStreamingHost(t, emptyAttributesConfig)
			defer host.Reset()

			deliverStreamChunk(t, host, []byte("data: "+payload+"\n"), false)
			attrs := getAILogAttributes(t, host)
			assertNoTokenAttrs(t, attrs)

			deliverStreamChunk(t, host, []byte("\n"), false)
			attrs = getAILogAttributes(t, host)
			assertTokenAttrs(t, attrs, 3, 4, 7)

			deliverStreamChunks(t, host, sseEvent("[DONE]"))
			host.CompleteHttp()
		})

		// T-05: a CRLF delimiter split at every byte boundary recovers the
		// event; intact CRLF and mixed delimiter forms behave the same.
		crlfEvent := []byte("data: " + payload + "\r\n\r\n")
		n := len(crlfEvent)
		cases := []struct {
			name  string
			part1 []byte
			part2 []byte
		}{
			{"crlf_intact", crlfEvent, nil},
			{"cr_then_lf_crlf", crlfEvent[:n-3], crlfEvent[n-3:]}, // "\r" | "\n\r\n"
			{"crlf_then_crlf", crlfEvent[:n-2], crlfEvent[n-2:]},  // "\r\n" | "\r\n"
			{"crlf_cr_then_lf", crlfEvent[:n-1], crlfEvent[n-1:]}, // "\r\n\r" | "\n"
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				host := setupStreamingHost(t, emptyAttributesConfig)
				defer host.Reset()
				if tc.part2 == nil {
					deliverStreamChunks(t, host, tc.part1)
				} else {
					deliverStreamChunks(t, host, tc.part1, tc.part2)
				}
				attrs := getAILogAttributes(t, host)
				assertTokenAttrs(t, attrs, 3, 4, 7)
				host.CompleteHttp()
			})
		}

		// Mixed delimiter forms terminate an event exactly like pure LF.
		for _, delim := range []string{"\n\r\n", "\r\n\n"} {
			t.Run(fmt.Sprintf("mixed_delimiter_%q", delim), func(t *testing.T) {
				host := setupStreamingHost(t, emptyAttributesConfig)
				defer host.Reset()
				deliverStreamChunks(t, host, []byte("data: "+payload+delim))
				attrs := getAILogAttributes(t, host)
				assertTokenAttrs(t, attrs, 3, 4, 7)
				host.CompleteHttp()
			})
		}
	})
}

// TestStreamingMultipleUsageEvents covers T-09 and the T-20 consistency rows:
// with several usage events the final complete event wins (values are never
// summed), repeated span-property writes overwrite to the final value, and
// ai_log / span / metrics all reflect the same final usage state.
func TestStreamingMultipleUsageEvents(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		first := sseEvent(`{"model":"gpt-4","usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}}`)
		final := sseEvent(`{"model":"gpt-4","usage":{"prompt_tokens":5,"completion_tokens":8,"total_tokens":13}}`)

		assertFinalState := func(t *testing.T, host test.TestHost) {
			attrs := getAILogAttributes(t, host)
			assertTokenAttrs(t, attrs, 5, 8, 13)
			spanTotal, ok := getSpanValue(host, ArmsTotalToken)
			require.True(t, ok)
			require.Equal(t, "13", spanTotal, "span property must hold the last event's value")
			assertTokenMetrics(t, host, "gpt-4", 5, 8, 13)
		}

		t.Run("separate_callbacks", func(t *testing.T) {
			host := setupStreamingHost(t, emptyAttributesConfig)
			defer host.Reset()

			deliverStreamChunk(t, host, first, false)
			attrs := getAILogAttributes(t, host)
			assertTokenAttrs(t, attrs, 5, 3, 8)
			spanTotal, ok := getSpanValue(host, ArmsTotalToken)
			require.True(t, ok)
			require.Equal(t, "8", spanTotal)

			deliverStreamChunks(t, host, final)
			assertFinalState(t, host)
			host.CompleteHttp()
		})

		t.Run("both_events_in_one_callback", func(t *testing.T) {
			host := setupStreamingHost(t, emptyAttributesConfig)
			defer host.Reset()
			deliverStreamChunks(t, host, concatStreamBytes(first, final))
			assertFinalState(t, host)
			host.CompleteHttp()
		})

		t.Run("second_event_split_across_callbacks", func(t *testing.T) {
			host := setupStreamingHost(t, emptyAttributesConfig)
			defer host.Reset()
			stream := concatStreamBytes(first, final)
			split := len(first) + len(final)/2
			deliverStreamChunks(t, host, stream[:split], stream[split:])
			assertFinalState(t, host)
			host.CompleteHttp()
		})

		// A final event silent on total never regresses the recorded total to
		// 0: tokenusage computes it from the effective components (5+8).
		t.Run("final_event_without_explicit_total", func(t *testing.T) {
			host := setupStreamingHost(t, emptyAttributesConfig)
			defer host.Reset()
			partial := sseEvent(`{"model":"gpt-4","usage":{"prompt_tokens":5,"completion_tokens":8}}`)
			deliverStreamChunks(t, host, first, partial)
			attrs := getAILogAttributes(t, host)
			assertTokenAttrs(t, attrs, 5, 8, 13)
			host.CompleteHttp()
		})
	})
}

// TestStreamingTokenDetailsPolicy covers T-10 (and the details part of T-20):
// details present in event N and absent in usage-bearing event N+1 are
// retained in both the context layer (EOS builtin attributes) and the
// user-attribute layer (per-callback ai_log write); details present in N+1
// replace the whole map and are never summed.
func TestStreamingTokenDetailsPolicy(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("details_retained_when_later_event_omits_them", func(t *testing.T) {
			host := setupStreamingHost(t, tokenDetailsConfig)
			defer host.Reset()

			withDetails := sseEvent(`{"model":"gpt-4o","usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15,"prompt_tokens_details":{"cached_tokens":4},"completion_tokens_details":{"reasoning_tokens":2}}}`)
			withoutDetails := sseEvent(`{"model":"gpt-4o","usage":{"prompt_tokens":10,"completion_tokens":7,"total_tokens":17}}`)

			deliverStreamChunk(t, host, withDetails, false)
			attrs := getAILogAttributes(t, host)
			require.Equal(t, map[string]interface{}{"cached_tokens": float64(4)}, attrs["input_token_details"])
			require.Equal(t, map[string]interface{}{"reasoning_tokens": float64(2)}, attrs["output_token_details"])

			// The usage-bearing event without details must not wipe the
			// recorded details: the user-attribute layer (this per-callback
			// ai_log write) retains the previous maps.
			deliverStreamChunk(t, host, withoutDetails, false)
			attrs = getAILogAttributes(t, host)
			assertTokenAttrs(t, attrs, 10, 7, 17)
			require.Equal(t, map[string]interface{}{"cached_tokens": float64(4)}, attrs["input_token_details"],
				"user-attribute layer must retain previous details")
			require.Equal(t, map[string]interface{}{"reasoning_tokens": float64(2)}, attrs["output_token_details"],
				"user-attribute layer must retain previous details")

			deliverStreamChunks(t, host, sseEvent("[DONE]"))
			// At EOS the builtin attributes read the context layer, which must
			// hold the same retained maps (layers synchronized), rendered as
			// their JSON string form.
			attrs = getAILogAttributes(t, host)
			assertTokenAttrs(t, attrs, 10, 7, 17)
			require.Equal(t, `{"cached_tokens":4}`, attrs["input_token_details"])
			require.Equal(t, `{"reasoning_tokens":2}`, attrs["output_token_details"])
			cached, ok := aiLogInt64(attrs, BuiltinCachedTokens)
			require.True(t, ok)
			require.Equal(t, int64(4), cached)
			reasoning, ok := aiLogInt64(attrs, BuiltinReasoningTokens)
			require.True(t, ok)
			require.Equal(t, int64(2), reasoning)

			// Output consistency (T-20): span and metrics reflect the same
			// final usage as the log attributes.
			spanTotal, ok := getSpanValue(host, ArmsTotalToken)
			require.True(t, ok)
			require.Equal(t, "17", spanTotal)
			assertTokenMetrics(t, host, "gpt-4o", 10, 7, 17)
			host.CompleteHttp()
		})

		t.Run("details_replaced_when_later_event_carries_them", func(t *testing.T) {
			host := setupStreamingHost(t, tokenDetailsConfig)
			defer host.Reset()

			first := sseEvent(`{"model":"gpt-4o","usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15,"prompt_tokens_details":{"cached_tokens":4},"completion_tokens_details":{"reasoning_tokens":2}}}`)
			// Output details present (replace), input details absent (retain).
			second := sseEvent(`{"model":"gpt-4o","usage":{"prompt_tokens":10,"completion_tokens":7,"total_tokens":17,"completion_tokens_details":{"reasoning_tokens":6}}}`)
			deliverStreamChunks(t, host, first, second)

			attrs := getAILogAttributes(t, host)
			assertTokenAttrs(t, attrs, 10, 7, 17)
			require.Equal(t, `{"cached_tokens":4}`, attrs["input_token_details"], "absent input details must be retained")
			require.Equal(t, `{"reasoning_tokens":6}`, attrs["output_token_details"], "present output details must replace the whole map")
			reasoning, ok := aiLogInt64(attrs, BuiltinReasoningTokens)
			require.True(t, ok)
			require.Equal(t, int64(6), reasoning, "details are replaced, never summed")
			host.CompleteHttp()
		})

		t.Run("details_never_summed_across_events", func(t *testing.T) {
			host := setupStreamingHost(t, tokenDetailsConfig)
			defer host.Reset()

			first := sseEvent(`{"model":"gpt-4o","usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15,"prompt_tokens_details":{"cached_tokens":4}}}`)
			second := sseEvent(`{"model":"gpt-4o","usage":{"prompt_tokens":10,"completion_tokens":7,"total_tokens":17,"prompt_tokens_details":{"cached_tokens":9}}}`)
			deliverStreamChunks(t, host, first, second)

			attrs := getAILogAttributes(t, host)
			require.Equal(t, `{"cached_tokens":9}`, attrs["input_token_details"], "details maps replace; values are never summed")
			cached, ok := aiLogInt64(attrs, BuiltinCachedTokens)
			require.True(t, ok)
			require.Equal(t, int64(9), cached)
			host.CompleteHttp()
		})
	})
}

// TestStreamingSplitErrorEvent covers T-11: an SSE error event split across
// callbacks is reassembled and detected exactly as an intact one, and one
// request produces exactly one llm_failure_count increment regardless of how
// many error events matched. The config used here buffers nothing, so the
// increment can only come from the framed-event hasStreamError flag (the EOS
// bodyForMetric fallback sees only the final [DONE] chunk).
func TestStreamingSplitErrorEvent(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		failureMetric := streamingMetricName("gpt-4", LLMFailureCount)
		contentEvent := sseEvent(`{"choices":[{"delta":{"content":"Hello"}}]}`)
		errorPayload := `{"error":{"type":"api_error","message":"Something went wrong"}}`
		doneEvent := sseEvent("[DONE]")

		assertOneFailure := func(t *testing.T, chunks ...[]byte) {
			host := setupStreamingHost(t, emptyAttributesConfig)
			defer host.Reset()
			deliverStreamChunks(t, host, chunks...)
			value, err := host.GetCounterMetric(failureMetric)
			require.NoError(t, err)
			require.Equal(t, uint64(1), value, "exactly one request-level failure increment")
			host.CompleteHttp()
		}

		t.Run("intact_error_event", func(t *testing.T) {
			assertOneFailure(t, contentEvent, sseEvent(errorPayload), doneEvent)
		})

		t.Run("split_error_event_is_equivalent", func(t *testing.T) {
			event := sseEvent(errorPayload)
			assertOneFailure(t, contentEvent, event[:12], event[12:len(event)-2], event[len(event)-2:], doneEvent)
		})

		t.Run("two_error_events_still_one_increment", func(t *testing.T) {
			assertOneFailure(t, sseEvent(errorPayload), sseEvent(errorPayload), doneEvent)
		})
	})
}

// TestStreamingNoUpstreamUsage covers T-12 and T-18: when the upstream never
// emits usage (including an abrupt end of stream mid-event, the #4145 shape),
// recorded tokens remain zero and nothing is fabricated.
func TestStreamingNoUpstreamUsage(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		assertZeroTokens := func(t *testing.T, host test.TestHost) {
			attrs := getAILogAttributes(t, host)
			assertNoTokenAttrs(t, attrs)
			_, err := host.GetCounterMetric(streamingMetricName("gpt-4", tokenusage.CtxKeyTotalToken))
			require.Error(t, err, "no token metric may be recorded without upstream usage")
			_, err = host.GetCounterMetric(streamingMetricName("gpt-4", LLMFailureCount))
			require.Error(t, err, "no failure may be recorded for a clean stream")
		}

		t.Run("no_usage_events", func(t *testing.T) {
			host := setupStreamingHost(t, emptyAttributesConfig)
			defer host.Reset()
			deliverStreamChunks(t, host,
				sseEvent(`{"choices":[{"delta":{"content":"Hello"}}],"model":"gpt-4"}`),
				sseEvent(`{"choices":[{"delta":{"content":" world"}}],"model":"gpt-4"}`),
				sseEvent("[DONE]"))
			assertZeroTokens(t, host)
			host.CompleteHttp()
		})

		// #4145 shape: the stream ends abruptly with an incomplete tail; the
		// tail is discarded at EOS and no usage is fabricated.
		t.Run("abrupt_end_of_stream_mid_event", func(t *testing.T) {
			host := setupStreamingHost(t, emptyAttributesConfig)
			defer host.Reset()
			deliverStreamChunks(t, host,
				sseEvent(`{"choices":[{"delta":{"content":"Hello"}}],"model":"gpt-4"}`),
				[]byte(`data: {"choices":[{"delta":{"content":"Hel`))
			assertZeroTokens(t, host)
			host.CompleteHttp()
		})
	})
}

// TestStreamingToolCallsSplit covers T-16: tool-call extraction stays on the
// end-of-stream accumulated-body path, so tool_calls content split across
// callbacks yields exactly the intact-delivery result and arguments are not
// double-appended.
func TestStreamingToolCallsSplit(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		events := [][]byte{
			sseEvent(`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_abc123","type":"function","function":{"name":"get_weather","arguments":""}}]}}]}`),
			sseEvent(`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"locat"}}]}}]}`),
			sseEvent(`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"ion\": \"Bei"}}]}}]}`),
			sseEvent(`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"jing\"}"}}]}}]}`),
			sseEvent(`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`),
		}

		readToolCalls := func(t *testing.T, chunks ...[]byte) interface{} {
			host := setupStreamingHost(t, builtinAttributesConfig)
			defer host.Reset()
			deliverStreamChunks(t, host, chunks...)
			attrs := getAILogAttributes(t, host)
			host.CompleteHttp()
			return attrs["tool_calls"]
		}

		t.Run("split_delivery_matches_intact_delivery", func(t *testing.T) {
			intact := readToolCalls(t, events...)

			// Re-chunk the identical byte stream into 13-byte pieces that cut
			// across event boundaries, JSON tokens and the data: prefixes.
			stream := concatStreamBytes(events...)
			var pieces [][]byte
			for i := 0; i < len(stream); i += 13 {
				end := i + 13
				if end > len(stream) {
					end = len(stream)
				}
				pieces = append(pieces, stream[i:end])
			}
			split := readToolCalls(t, pieces...)

			require.Equal(t, intact, split, "tool_calls must equal intact delivery")
			calls, ok := split.([]interface{})
			require.True(t, ok)
			require.Len(t, calls, 1)
			call := calls[0].(map[string]interface{})
			require.Equal(t, "call_abc123", call["id"])
			require.Equal(t, "function", call["type"])
			fn := call["function"].(map[string]interface{})
			require.Equal(t, "get_weather", fn["name"])
			require.Equal(t, `{"location": "Beijing"}`, fn["arguments"],
				"arguments must be assembled exactly once, not double-appended")
		})
	})
}

// TestStreamingEOSIncompleteTail covers T-24: an unterminated event at
// end-of-stream is discarded; no usage is fabricated for the partial event
// and previously recorded values are preserved.
func TestStreamingEOSIncompleteTail(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		usageEvent := sseEvent(`{"model":"gpt-4","usage":{"prompt_tokens":5,"completion_tokens":8,"total_tokens":13}}`)
		partialUsage := []byte(`data: {"model":"gpt-4","usage":{"prompt_tokens":99,"completion_tokens":99,"total_tokens":198}}`)

		t.Run("partial_final_event_discarded", func(t *testing.T) {
			host := setupStreamingHost(t, emptyAttributesConfig)
			defer host.Reset()
			deliverStreamChunks(t, host, usageEvent, partialUsage)
			attrs := getAILogAttributes(t, host)
			assertTokenAttrs(t, attrs, 5, 8, 13)
			assertTokenMetrics(t, host, "gpt-4", 5, 8, 13)
			host.CompleteHttp()
		})

		t.Run("complete_event_then_partial_tail_same_callback", func(t *testing.T) {
			host := setupStreamingHost(t, emptyAttributesConfig)
			defer host.Reset()
			deliverStreamChunks(t, host, concatStreamBytes(usageEvent, partialUsage))
			attrs := getAILogAttributes(t, host)
			assertTokenAttrs(t, attrs, 5, 8, 13)
			host.CompleteHttp()
		})
	})
}

// TestStreamingFailOpenOnPanic covers T-19: a panic in an observability
// consumer is contained by the local recovery boundary in onHttpStreamingBody
// and the original response bytes pass through unchanged. Wrapper-level
// recovery is disabled via WASM_DISABLE_PANIC_RECOVERY so only the local
// boundary can contain the panic in go mode; in wasm mode the injected
// override has no effect and the passthrough assertions still hold.
func TestStreamingFailOpenOnPanic(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("injected_consumer_panic_returns_original_data", func(t *testing.T) {
			host := setupStreamingHost(t, emptyAttributesConfig)
			defer host.Reset()

			os.Setenv("WASM_DISABLE_PANIC_RECOVERY", "true")
			defer os.Unsetenv("WASM_DISABLE_PANIC_RECOVERY")

			orig := getTokenUsage
			getTokenUsage = func(ctx wrapper.HttpContext, body []byte) tokenusage.TokenUsage {
				panic("injected consumer panic")
			}
			defer func() { getTokenUsage = orig }()

			usageChunk := sseEvent(`{"model":"gpt-4","usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}}`)
			deliverStreamChunk(t, host, usageChunk, false)

			// The request-scoped framer state is not corrupted by the panic: a
			// later event is still processed and the EOS path completes.
			getTokenUsage = orig
			finalChunk := sseEvent(`{"model":"gpt-4","usage":{"prompt_tokens":5,"completion_tokens":8,"total_tokens":13}}`)
			deliverStreamChunks(t, host, finalChunk)
			attrs := getAILogAttributes(t, host)
			assertTokenAttrs(t, attrs, 5, 8, 13)
			host.CompleteHttp()
		})
	})
}

// TestStreamingFramerResyncIntegration drives the RESYNC paths end to end:
// T-13 (complete events emitted before an oversized suffix in the same
// callback are still delivered), T-14 (after an overflow the framer resumes
// within the callback that carries the next delimiter), T-25/T-26 (the exact
// 1 MiB boundary), and exercises drain() with a non-zero overflow count at EOS
// (T-21; the exact count is asserted in sse_framer_test.go).
func TestStreamingFramerResyncIntegration(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// paddedUsageEvent builds a usage event larger than 1 MiB whose padding
		// contains no line endings (so no delimiter appears inside it).
		paddedUsageEvent := func(prompt, completion, total int) []byte {
			payload := fmt.Sprintf(`{"pad":"%s","model":"gpt-4","usage":{"prompt_tokens":%d,"completion_tokens":%d,"total_tokens":%d}}`,
				strings.Repeat("x", maxIncompleteSSEEventBytes), prompt, completion, total)
			return sseEvent(payload)
		}

		// T-25: an incomplete suffix of exactly 1 MiB is retained as the tail
		// (no RESYNC), so the event completes on the next callback and usage is
		// recovered.
		t.Run("suffix_exactly_one_mib_retained_and_recovered", func(t *testing.T) {
			host := setupStreamingHost(t, emptyAttributesConfig)
			defer host.Reset()
			event := paddedUsageEvent(7, 8, 15)
			deliverStreamChunks(t, host, event[:maxIncompleteSSEEventBytes], event[maxIncompleteSSEEventBytes:])
			attrs := getAILogAttributes(t, host)
			assertTokenAttrs(t, attrs, 7, 8, 15)
			host.CompleteHttp()
		})

		// T-26: a 1 MiB+1 incomplete suffix enters RESYNC and is dropped; the
		// next delimiter-terminated event is still recovered.
		t.Run("suffix_one_mib_plus_one_resyncs", func(t *testing.T) {
			host := setupStreamingHost(t, emptyAttributesConfig)
			defer host.Reset()
			event := paddedUsageEvent(99, 99, 198)
			deliverStreamChunk(t, host, event[:maxIncompleteSSEEventBytes+1], false)
			// The remainder carries the oversized event's own terminating
			// delimiter; RESYNC discards everything through it.
			deliverStreamChunk(t, host, event[maxIncompleteSSEEventBytes+1:], false)
			attrs := getAILogAttributes(t, host)
			assertNoTokenAttrs(t, attrs)

			final := sseEvent(`{"model":"gpt-4","usage":{"prompt_tokens":5,"completion_tokens":8,"total_tokens":13}}`)
			deliverStreamChunks(t, host, final)
			attrs = getAILogAttributes(t, host)
			assertTokenAttrs(t, attrs, 5, 8, 13)
			host.CompleteHttp()
		})

		// T-13 + T-14: a complete usage event emitted before the overflow is
		// still delivered, and after the overflow the framer resumes processing
		// bytes after the next delimiter within the same callback.
		t.Run("events_before_overflow_and_same_callback_resume", func(t *testing.T) {
			host := setupStreamingHost(t, emptyAttributesConfig)
			defer host.Reset()

			first := sseEvent(`{"model":"gpt-4","usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}}`)
			oversizedTail := []byte(`data: {"pad":"` + strings.Repeat("x", maxIncompleteSSEEventBytes) + `"`)
			deliverStreamChunk(t, host, concatStreamBytes(first, oversizedTail), false)
			attrs := getAILogAttributes(t, host)
			assertTokenAttrs(t, attrs, 5, 3, 8)

			second := sseEvent(`{"model":"gpt-4","usage":{"prompt_tokens":5,"completion_tokens":8,"total_tokens":13}}`)
			deliverStreamChunk(t, host, concatStreamBytes([]byte("\n\n"), second), false)
			attrs = getAILogAttributes(t, host)
			assertTokenAttrs(t, attrs, 5, 8, 13)

			// EOS: drain() reports the single RESYNC entry via one debug log
			// line; previously recorded values are preserved.
			deliverStreamChunks(t, host, sseEvent("[DONE]"))
			attrs = getAILogAttributes(t, host)
			assertTokenAttrs(t, attrs, 5, 8, 13)
			host.CompleteHttp()
		})
	})
}
