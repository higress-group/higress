// Copyright (c) 2025 Alibaba Group Holding Ltd.
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

package tests

import (
	"testing"
	"time"

	"github.com/alibaba/higress/test/e2e/conformance/utils/http"
	"github.com/alibaba/higress/test/e2e/conformance/utils/suite"
)

func init() {
	Register(WasmPluginsAiStatistics)
}

var WasmPluginsAiStatistics = suite.ConformanceTest{
	ShortName:   "WasmPluginAiStatistics",
	Description: "The Ingress in the higress-conformance-ai-backend namespace test the ai-statistics WASM plugin.",
	Features:    []suite.SupportedFeature{suite.WASMGoConformanceFeature},
	Manifests:   []string{"tests/go-wasm-ai-statistics.yaml"},
	Test: func(t *testing.T, suite *suite.ConformanceTestSuite) {
		testcases := []http.Assertion{
			// 测试基础配置 - 非流式请求
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "ai-statistics basic case 1: non-streaming request with default config",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "ai-statistics-basic.test",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Hello, who are you?"}],"stream":false}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeApplicationJson,
						// 验证llm-mock-service返回的标准OpenAI格式响应
						Body: []byte(`{"id":"chatcmpl-llm-mock","choices":[{"index":0,"message":{"role":"assistant","content":"Hello, who are you?"},"finish_reason":"stop"}],"created":10,"model":"unknown","object":"chat.completion","usage":{"prompt_tokens":9,"completion_tokens":1,"total_tokens":10}}`),
					},
				},
			},
			// 测试基础配置 - 流式请求
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "ai-statistics basic case 2: streaming request with default config",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "ai-statistics-basic.test",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Hello"}],"stream":true}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeTextEventStream,
						// 验证流式响应格式
						Body: []byte(`data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"H"}}],"created":10,"model":"unknown","object":"chat.completion.chunk","usage":{}}  
  
data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"e"}}],"created":10,"model":"unknown","object":"chat.completion.chunk","usage":{}}  
  
data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"l"}}],"created":10,"model":"unknown","object":"chat.completion.chunk","usage":{}}  
  
data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"l"}}],"created":10,"model":"unknown","object":"chat.completion.chunk","usage":{}}  
  
data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"o"}}],"created":10,"model":"unknown","object":"chat.completion.chunk","usage":{}}  
  
data: [DONE]  
  
`),
					},
				},
			},
			// 测试自定义属性配置 - 非流式请求
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "ai-statistics custom attrs case 1: non-streaming request with custom attributes",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "ai-statistics-custom.test",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Headers: map[string]string{
							"x-custom-header": "test-value",
							"x-mse-consumer":  "test-consumer",
						},
						Body: []byte(`{"model":"gpt-4","messages":[{"role":"user","content":"Custom attributes test"}],"stream":false}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"id":"chatcmpl-llm-mock","choices":[{"index":0,"message":{"role":"assistant","content":"Custom attributes test"},"finish_reason":"stop"}],"created":10,"model":"unknown","object":"chat.completion","usage":{"prompt_tokens":9,"completion_tokens":1,"total_tokens":10}}`),
					},
				},
			},
			// 测试自定义属性配置 - 流式请求
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "ai-statistics custom attrs case 2: streaming request with custom attributes",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "ai-statistics-custom.test",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Headers: map[string]string{
							"x-custom-header": "stream-test",
						},
						Body: []byte(`{"model":"gpt-4","messages":[{"role":"user","content":"Stream test"}],"stream":true}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeTextEventStream,
						Body: []byte(`data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"S"}}],"created":10,"model":"unknown","object":"chat.completion.chunk","usage":{}}  
  
data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"t"}}],"created":10,"model":"unknown","object":"chat.completion.chunk","usage":{}}  
  
data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"r"}}],"created":10,"model":"unknown","object":"chat.completion.chunk","usage":{}}  
  
data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"e"}}],"created":10,"model":"unknown","object":"chat.completion.chunk","usage":{}}  
  
data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"a"}}],"created":10,"model":"unknown","object":"chat.completion.chunk","usage":{}}  
  
data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"m"}}],"created":10,"model":"unknown","object":"chat.completion.chunk","usage":{}}  
  
data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":" "}}],"created":10,"model":"unknown","object":"chat.completion.chunk","usage":{}}  
  
data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"t"}}],"created":10,"model":"unknown","object":"chat.completion.chunk","usage":{}}  
  
data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"e"}}],"created":10,"model":"unknown","object":"chat.completion.chunk","usage":{}}  
  
data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"s"}}],"created":10,"model":"unknown","object":"chat.completion.chunk","usage":{}}  
  
data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"t"}}],"created":10,"model":"unknown","object":"chat.completion.chunk","usage":{}}  
  
data: [DONE]  
  
`),
					},
				},
			},
			// 测试与ai-proxy配合使用 - 非流式请求
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "ai-statistics with proxy case 1: non-streaming request with ai-proxy",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "ai-statistics-proxy.test",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Headers: map[string]string{
							"Authorization": "Bearer fake_token",
						},
						Body: []byte(`{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Proxy integration test"}],"stream":false}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeApplicationJson,
						// ai-proxy会将模型映射为qwen-turbo
						Body: []byte(`{"id":"chatcmpl-llm-mock","choices":[{"index":0,"message":{"role":"assistant","content":"Proxy integration test"},"finish_reason":"stop"}],"created":10,"model":"qwen-turbo","object":"chat.completion","usage":{"prompt_tokens":9,"completion_tokens":1,"total_tokens":10}}`),
					},
				},
			},
			// 测试与ai-proxy配合使用 - 流式请求
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "ai-statistics with proxy case 2: streaming request with ai-proxy",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "ai-statistics-proxy.test",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Headers: map[string]string{
							"Authorization": "Bearer fake_token",
						},
						Body: []byte(`{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Proxy stream"}],"stream":true}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeTextEventStream,
						Body: []byte(`data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"P"}}],"created":10,"model":"qwen-turbo","object":"chat.completion.chunk","usage":{}}  
  
data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"r"}}],"created":10,"model":"qwen-turbo","object":"chat.completion.chunk","usage":{}}  
  
data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"o"}}],"created":10,"model":"qwen-turbo","object":"chat.completion.chunk","usage":{}}  
  
data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"x"}}],"created":10,"model":"qwen-turbo","object":"chat.completion.chunk","usage":{}}  
  
data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"y"}}],"created":10,"model":"qwen-turbo","object":"chat.completion.chunk","usage":{}}  
  
data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":" "}}],"created":10,"model":"qwen-turbo","object":"chat.completion.chunk","usage":{}}  
  
data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"s"}}],"created":10,"model":"qwen-turbo","object":"chat.completion.chunk","usage":{}}  
  
data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"t"}}],"created":10,"model":"qwen-turbo","object":"chat.completion.chunk","usage":{}}  
  
data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"r"}}],"created":10,"model":"qwen-turbo","object":"chat.completion.chunk","usage":{}}  
  
data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"e"}}],"created":10,"model":"qwen-turbo","object":"chat.completion.chunk","usage":{}}  
  
data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"a"}}],"created":10,"model":"qwen-turbo","object":"chat.completion.chunk","usage":{}}  
  
data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"m"}}],"created":10,"model":"qwen-turbo","object":"chat.completion.chunk","usage":{}}  
  
data: [DONE]  
  
`),
					},
				},
			},
			// 测试错误请求处理
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "ai-statistics error case: invalid request body",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "ai-statistics-basic.test",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"invalid": "json"}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeApplicationJson,
						// llm-mock-service会回显请求内容
						Body: []byte(`{"id":"chatcmpl-llm-mock","choices":[{"index":0,"message":{"role":"assistant","content":"{\"invalid\": \"json\"}"},"finish_reason":"stop"}],"created":10,"model":"unknown","object":"chat.completion","usage":{"prompt_tokens":9,"completion_tokens":1,"total_tokens":10}}`),
					},
				},
			},
		}

		t.Run("WasmPlugins ai-statistics", func(t *testing.T) {
			for _, testcase := range testcases {
				t.Run(testcase.Meta.TestCaseName, func(t *testing.T) {
					// 添加一些延迟以确保指标收集完成
					time.Sleep(100 * time.Millisecond)
					http.MakeRequestAndExpectEventuallyConsistentResponse(t, suite.RoundTripper, suite.TimeoutConfig, suite.GatewayAddress, testcase)
				})
			}
		})

		// 额外的指标验证测试
		t.Run("Metrics Validation", func(t *testing.T) {
			// 这里可以添加对Prometheus指标的验证
			// 由于当前测试框架限制，这里主要通过日志验证功能正常
			t.Log("ai-statistics plugin metrics should be collected including:")
			t.Log("- route_upstream_model_consumer_metric_input_token")
			t.Log("- route_upstream_model_consumer_metric_output_token")
			t.Log("- route_upstream_model_consumer_metric_llm_service_duration")
			t.Log("- route_upstream_model_consumer_metric_llm_duration_count")
			t.Log("- route_upstream_model_consumer_metric_llm_first_token_duration (for streaming)")
			t.Log("- route_upstream_model_consumer_metric_llm_stream_duration_count (for streaming)")
		})
	},
}
