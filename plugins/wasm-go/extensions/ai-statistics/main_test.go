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
	"time"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
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

			// 4. 处理流式响应体 - 添加 usage 信息
			firstChunk := []byte(`data: {"choices":[{"message":{"content":"Hello"}}],"model":"gpt-4","usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}}`)
			action = host.CallOnHttpStreamingResponseBody(firstChunk, false)

			// 应该返回原始数据
			require.Equal(t, types.ActionContinue, action)

			result := host.GetResponseBody()
			require.Equal(t, firstChunk, result)

			// 5. 处理最后一个流式块 - 添加 usage 信息
			lastChunk := []byte(`data: {"choices":[{"message":{"content":"How can I help you?"}}],"model":"gpt-4","usage":{"prompt_tokens":5,"completion_tokens":8,"total_tokens":13}}`)
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

			// 4. 处理流式响应体 - 添加 usage 信息
			firstChunk := []byte(`data: {"choices":[{"message":{"content":"Hello"}}],"model":"gpt-4","usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}}`)
			action = host.CallOnHttpStreamingResponseBody(firstChunk, false)

			// 应该返回原始数据
			require.Equal(t, types.ActionContinue, action)

			result := host.GetResponseBody()
			require.Equal(t, firstChunk, result)

			// 5. 处理最后一个流式块 - 添加 usage 信息
			lastChunk := []byte(`data: {"choices":[{"message":{"content":"How can I help you?"}}],"model":"gpt-4","usage":{"prompt_tokens":5,"completion_tokens":8,"total_tokens":13}}`)
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
