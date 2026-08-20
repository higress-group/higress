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

	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/http"
	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/suite"
)

// The llm-mock service response has a fixed id of `chatcmpl-llm-mock`.
// The created field is fixed to 10.
// The response content is echoed back as the request content.
// The usage field is fixed to `{"prompt_tokens":9,"completion_tokens":1,"total_tokens":10}` (specific values may vary based on the corresponding response fields).

func init() {
	Register(WasmPluginsAiProxy)
}

var WasmPluginsAiProxy = suite.ConformanceTest{
	ShortName:   "WasmPluginAiProxy",
	Description: "The Ingress in the higress-conformance-ai-backend namespace test the ai-proxy WASM plugin.",
	Features:    []suite.SupportedFeature{suite.WASMGoConformanceFeature},
	Manifests:   []string{"tests/go-wasm-ai-proxy.yaml"},
	Test: func(t *testing.T, suite *suite.ConformanceTestSuite) {
		testcases := []http.Assertion{
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "ai360 case 1: non-streaming request",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "api.360.cn",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-3","messages":[{"role":"user","content":"你好，你是谁？"}],"stream":false}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"id":"chatcmpl-llm-mock","choices":[{"index":0,"message":{"role":"assistant","content":"你好，你是谁？"},"finish_reason":"stop","logprobs":null}],"created":10,"model":"360gpt-turbo","object":"chat.completion","usage":{"prompt_tokens":9,"completion_tokens":1,"total_tokens":10}}`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "ai360 case 2: streaming request",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "api.360.cn",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-3","messages":[{"role":"user","content":"你好，你是谁？"}],"stream":true}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeTextEventStream,
						Body: []byte(`data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"你"},"finish_reason":null,"logprobs":null}],"created":10,"model":"360gpt-turbo","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"好"},"finish_reason":null,"logprobs":null}],"created":10,"model":"360gpt-turbo","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"，"},"finish_reason":null,"logprobs":null}],"created":10,"model":"360gpt-turbo","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"你"},"finish_reason":null,"logprobs":null}],"created":10,"model":"360gpt-turbo","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"是"},"finish_reason":null,"logprobs":null}],"created":10,"model":"360gpt-turbo","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"谁"},"finish_reason":null,"logprobs":null}],"created":10,"model":"360gpt-turbo","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"？"},"finish_reason":"stop","logprobs":null}],"created":10,"model":"360gpt-turbo","object":"chat.completion.chunk","usage":null}

data: [DONE]

`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "baichuan case 1: non-streaming request",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "api.baichuan-ai.com",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-3","messages":[{"role":"user","content":"你好，你是谁？"}],"stream":false}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"id":"chatcmpl-llm-mock","choices":[{"index":0,"message":{"role":"assistant","content":"你好，你是谁？"},"finish_reason":"stop","logprobs":null}],"created":10,"model":"baichuan2-13b-chat-v1","object":"chat.completion","usage":{"prompt_tokens":9,"completion_tokens":1,"total_tokens":10}}`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "baichuan case 2: streaming request",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "api.baichuan-ai.com",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-3","messages":[{"role":"user","content":"你好，你是谁？"}],"stream":true}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeTextEventStream,
						Body: []byte(`data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"你"},"finish_reason":null,"logprobs":null}],"created":10,"model":"baichuan2-13b-chat-v1","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"好"},"finish_reason":null,"logprobs":null}],"created":10,"model":"baichuan2-13b-chat-v1","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"，"},"finish_reason":null,"logprobs":null}],"created":10,"model":"baichuan2-13b-chat-v1","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"你"},"finish_reason":null,"logprobs":null}],"created":10,"model":"baichuan2-13b-chat-v1","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"是"},"finish_reason":null,"logprobs":null}],"created":10,"model":"baichuan2-13b-chat-v1","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"谁"},"finish_reason":null,"logprobs":null}],"created":10,"model":"baichuan2-13b-chat-v1","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"？"},"finish_reason":"stop","logprobs":null}],"created":10,"model":"baichuan2-13b-chat-v1","object":"chat.completion.chunk","usage":null}

data: [DONE]

`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "baidu case 1: non-streaming request",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "qianfan.baidubce.com",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-3","messages":[{"role":"user","content":"你好，你是谁？"}],"stream":false}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"id":"chatcmpl-llm-mock","choices":[{"index":0,"message":{"role":"assistant","content":"你好，你是谁？"},"finish_reason":"stop","logprobs":null}],"created":10,"model":"ernie-3.5-8k","object":"chat.completion","usage":{"prompt_tokens":9,"completion_tokens":1,"total_tokens":10}}`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "baidu case 2: streaming request",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "qianfan.baidubce.com",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-3","messages":[{"role":"user","content":"你好，你是谁？"}],"stream":true}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeTextEventStream,
						Body: []byte(`data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"你"},"finish_reason":null,"logprobs":null}],"created":10,"model":"ernie-3.5-8k","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"好"},"finish_reason":null,"logprobs":null}],"created":10,"model":"ernie-3.5-8k","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"，"},"finish_reason":null,"logprobs":null}],"created":10,"model":"ernie-3.5-8k","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"你"},"finish_reason":null,"logprobs":null}],"created":10,"model":"ernie-3.5-8k","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"是"},"finish_reason":null,"logprobs":null}],"created":10,"model":"ernie-3.5-8k","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"谁"},"finish_reason":null,"logprobs":null}],"created":10,"model":"ernie-3.5-8k","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"？"},"finish_reason":"stop","logprobs":null}],"created":10,"model":"ernie-3.5-8k","object":"chat.completion.chunk","usage":null}

data: [DONE]

`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "deepseek case 1: non-streaming request",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "api.deepseek.com",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-3","messages":[{"role":"user","content":"你好，你是谁？"}],"stream":false}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"id":"chatcmpl-llm-mock","choices":[{"index":0,"message":{"role":"assistant","content":"你好，你是谁？"},"finish_reason":"stop","logprobs":null}],"created":10,"model":"deepseek-reasoner","object":"chat.completion","usage":{"prompt_tokens":9,"completion_tokens":1,"total_tokens":10}}`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "deepseek case 2: streaming request",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "api.deepseek.com",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-3","messages":[{"role":"user","content":"你好，你是谁？"}],"stream":true}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeTextEventStream,
						Body: []byte(`data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"你"},"finish_reason":null,"logprobs":null}],"created":10,"model":"deepseek-reasoner","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"好"},"finish_reason":null,"logprobs":null}],"created":10,"model":"deepseek-reasoner","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"，"},"finish_reason":null,"logprobs":null}],"created":10,"model":"deepseek-reasoner","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"你"},"finish_reason":null,"logprobs":null}],"created":10,"model":"deepseek-reasoner","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"是"},"finish_reason":null,"logprobs":null}],"created":10,"model":"deepseek-reasoner","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"谁"},"finish_reason":null,"logprobs":null}],"created":10,"model":"deepseek-reasoner","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"？"},"finish_reason":"stop","logprobs":null}],"created":10,"model":"deepseek-reasoner","object":"chat.completion.chunk","usage":null}

data: [DONE]

`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "doubao case 1: non-streaming request",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "ark.cn-beijing.volces.com",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-3","messages":[{"role":"user","content":"你好，你是谁？"}],"stream":false}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"id":"chatcmpl-llm-mock","choices":[{"index":0,"message":{"role":"assistant","content":"你好，你是谁？"},"finish_reason":"stop","logprobs":null}],"created":10,"model":"fake_doubao_endpoint","object":"chat.completion","usage":{"prompt_tokens":9,"completion_tokens":1,"total_tokens":10}}`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "doubao case 2: streaming request",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "ark.cn-beijing.volces.com",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-3","messages":[{"role":"user","content":"你好，你是谁？"}],"stream":true}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeTextEventStream,
						Body: []byte(`data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"你"},"finish_reason":null,"logprobs":null}],"created":10,"model":"fake_doubao_endpoint","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"好"},"finish_reason":null,"logprobs":null}],"created":10,"model":"fake_doubao_endpoint","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"，"},"finish_reason":null,"logprobs":null}],"created":10,"model":"fake_doubao_endpoint","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"你"},"finish_reason":null,"logprobs":null}],"created":10,"model":"fake_doubao_endpoint","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"是"},"finish_reason":null,"logprobs":null}],"created":10,"model":"fake_doubao_endpoint","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"谁"},"finish_reason":null,"logprobs":null}],"created":10,"model":"fake_doubao_endpoint","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"？"},"finish_reason":"stop","logprobs":null}],"created":10,"model":"fake_doubao_endpoint","object":"chat.completion.chunk","usage":null}

data: [DONE]

`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "github case 1: non-streaming request",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "models.inference.ai.azure.com",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-3","messages":[{"role":"user","content":"你好，你是谁？"}],"stream":false}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"id":"chatcmpl-llm-mock","choices":[{"index":0,"message":{"role":"assistant","content":"你好，你是谁？"},"finish_reason":"stop","logprobs":null}],"created":10,"model":"cohere-command-r-08-2024","object":"chat.completion","usage":{"prompt_tokens":9,"completion_tokens":1,"total_tokens":10}}`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "github case 2: streaming request",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "models.inference.ai.azure.com",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-3","messages":[{"role":"user","content":"你好，你是谁？"}],"stream":true}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeTextEventStream,
						Body: []byte(`data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"你"},"finish_reason":null,"logprobs":null}],"created":10,"model":"cohere-command-r-08-2024","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"好"},"finish_reason":null,"logprobs":null}],"created":10,"model":"cohere-command-r-08-2024","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"，"},"finish_reason":null,"logprobs":null}],"created":10,"model":"cohere-command-r-08-2024","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"你"},"finish_reason":null,"logprobs":null}],"created":10,"model":"cohere-command-r-08-2024","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"是"},"finish_reason":null,"logprobs":null}],"created":10,"model":"cohere-command-r-08-2024","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"谁"},"finish_reason":null,"logprobs":null}],"created":10,"model":"cohere-command-r-08-2024","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"？"},"finish_reason":"stop","logprobs":null}],"created":10,"model":"cohere-command-r-08-2024","object":"chat.completion.chunk","usage":null}

data: [DONE]

`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "groq case 1: non-streaming request",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "api.groq.com",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-3","messages":[{"role":"user","content":"你好，你是谁？"}],"stream":false}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"id":"chatcmpl-llm-mock","choices":[{"index":0,"message":{"role":"assistant","content":"你好，你是谁？"},"finish_reason":"stop","logprobs":null}],"created":10,"model":"llama3-8b-8192","object":"chat.completion","usage":{"prompt_tokens":9,"completion_tokens":1,"total_tokens":10}}`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "groq case 2: streaming request",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "api.groq.com",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-3","messages":[{"role":"user","content":"你好，你是谁？"}],"stream":true}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeTextEventStream,
						Body: []byte(`data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"你"},"finish_reason":null,"logprobs":null}],"created":10,"model":"llama3-8b-8192","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"好"},"finish_reason":null,"logprobs":null}],"created":10,"model":"llama3-8b-8192","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"，"},"finish_reason":null,"logprobs":null}],"created":10,"model":"llama3-8b-8192","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"你"},"finish_reason":null,"logprobs":null}],"created":10,"model":"llama3-8b-8192","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"是"},"finish_reason":null,"logprobs":null}],"created":10,"model":"llama3-8b-8192","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"谁"},"finish_reason":null,"logprobs":null}],"created":10,"model":"llama3-8b-8192","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"？"},"finish_reason":"stop","logprobs":null}],"created":10,"model":"llama3-8b-8192","object":"chat.completion.chunk","usage":null}

data: [DONE]

`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "minimax case 1: proxy completion V2 API, non-streaming request",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "api.minimax.chat-v2-api",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-3","messages":[{"role":"user","content":"你好，你是谁？"}],"stream":false}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"id":"chatcmpl-llm-mock","choices":[{"index":0,"message":{"role":"assistant","content":"你好，你是谁？"},"finish_reason":"stop","logprobs":null}],"created":10,"model":"abab6.5s-chat","object":"chat.completion","usage":{"prompt_tokens":9,"completion_tokens":1,"total_tokens":10}}`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "minimax case 2: proxy completion V2 API, streaming request",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "api.minimax.chat-v2-api",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-3","messages":[{"role":"user","content":"你好，你是谁？"}],"stream":true}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeTextEventStream,
						Body: []byte(`data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"你"},"finish_reason":null,"logprobs":null}],"created":10,"model":"abab6.5s-chat","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"好"},"finish_reason":null,"logprobs":null}],"created":10,"model":"abab6.5s-chat","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"，"},"finish_reason":null,"logprobs":null}],"created":10,"model":"abab6.5s-chat","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"你"},"finish_reason":null,"logprobs":null}],"created":10,"model":"abab6.5s-chat","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"是"},"finish_reason":null,"logprobs":null}],"created":10,"model":"abab6.5s-chat","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"谁"},"finish_reason":null,"logprobs":null}],"created":10,"model":"abab6.5s-chat","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"？"},"finish_reason":"stop","logprobs":null}],"created":10,"model":"abab6.5s-chat","object":"chat.completion.chunk","usage":null}

data: [DONE]

`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "minimax case 3: proxy completion Pro API, non-streaming request",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "api.minimax.chat-pro-api",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-3","messages":[{"role":"user","content":"你好，你是谁？"}],"stream":false}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"id":"chatcmpl-llm-mock","choices":[{"index":0,"message":{"name":"MM智能助理","role":"assistant","content":"你好，你是谁？"},"finish_reason":"stop","logprobs":null}],"created":10,"model":"abab6.5s-chat","object":"chat.completion","usage":{"prompt_tokens":9,"completion_tokens":1,"total_tokens":10}}`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "minimax case 4: proxy completion Pro API, streaming request",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "api.minimax.chat-pro-api",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-3","messages":[{"role":"user","content":"你好，你是谁？"}],"stream":true}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeTextEventStream,
						Body: []byte(`data: {"choices":[{"index":0,"message":{"name":"MM智能助理","role":"assistant","content":"你"},"finish_reason":"","logprobs":null}],"created":10,"model":"abab6.5s-chat","object":"chat.completion","usage":{}}

data: {"choices":[{"index":0,"message":{"name":"MM智能助理","role":"assistant","content":"好"},"finish_reason":"","logprobs":null}],"created":10,"model":"abab6.5s-chat","object":"chat.completion","usage":{}}

data: {"choices":[{"index":0,"message":{"name":"MM智能助理","role":"assistant","content":"，"},"finish_reason":"","logprobs":null}],"created":10,"model":"abab6.5s-chat","object":"chat.completion","usage":{}}

data: {"choices":[{"index":0,"message":{"name":"MM智能助理","role":"assistant","content":"你"},"finish_reason":"","logprobs":null}],"created":10,"model":"abab6.5s-chat","object":"chat.completion","usage":{}}

data: {"choices":[{"index":0,"message":{"name":"MM智能助理","role":"assistant","content":"是"},"finish_reason":"","logprobs":null}],"created":10,"model":"abab6.5s-chat","object":"chat.completion","usage":{}}

data: {"choices":[{"index":0,"message":{"name":"MM智能助理","role":"assistant","content":"谁"},"finish_reason":"","logprobs":null}],"created":10,"model":"abab6.5s-chat","object":"chat.completion","usage":{}}

data: {"choices":[{"index":0,"message":{"name":"MM智能助理","role":"assistant","content":"？"},"finish_reason":"","logprobs":null}],"created":10,"model":"abab6.5s-chat","object":"chat.completion","usage":{}}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"message":{"name":"MM智能助理","role":"assistant","content":"你好，你是谁？"},"finish_reason":"stop","logprobs":null}],"created":10,"model":"abab6.5s-chat","object":"chat.completion","usage":{"prompt_tokens":9,"completion_tokens":1,"total_tokens":10}}

`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "mistral case 1: non-streaming request",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "api.mistral.ai",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-3","messages":[{"role":"user","content":"你好，你是谁？"}],"stream":false}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"id":"chatcmpl-llm-mock","choices":[{"index":0,"message":{"role":"assistant","content":"你好，你是谁？"},"finish_reason":"stop","logprobs":null}],"created":10,"model":"mistral-tiny","object":"chat.completion","usage":{"prompt_tokens":9,"completion_tokens":1,"total_tokens":10}}`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "mistral case 2: streaming request",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "api.mistral.ai",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-3","messages":[{"role":"user","content":"你好，你是谁？"}],"stream":true}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeTextEventStream,
						Body: []byte(`data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"你"},"finish_reason":null,"logprobs":null}],"created":10,"model":"mistral-tiny","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"好"},"finish_reason":null,"logprobs":null}],"created":10,"model":"mistral-tiny","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"，"},"finish_reason":null,"logprobs":null}],"created":10,"model":"mistral-tiny","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"你"},"finish_reason":null,"logprobs":null}],"created":10,"model":"mistral-tiny","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"是"},"finish_reason":null,"logprobs":null}],"created":10,"model":"mistral-tiny","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"谁"},"finish_reason":null,"logprobs":null}],"created":10,"model":"mistral-tiny","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"？"},"finish_reason":"stop","logprobs":null}],"created":10,"model":"mistral-tiny","object":"chat.completion.chunk","usage":null}

data: [DONE]

`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "qwen case 1: compatible mode, non-streaming request",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "dashscope.aliyuncs.com-compatible-mode",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-3","messages":[{"role":"user","content":"你好，你是谁？"}],"stream":false}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"id":"chatcmpl-llm-mock","choices":[{"index":0,"message":{"role":"assistant","content":"你好，你是谁？"},"finish_reason":"stop","logprobs":null}],"created":10,"model":"qwen-turbo","object":"chat.completion","usage":{"prompt_tokens":9,"completion_tokens":1,"total_tokens":10}}`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "qwen case 2: compatible mode, streaming request",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "dashscope.aliyuncs.com-compatible-mode",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-3","messages":[{"role":"user","content":"你好，你是谁？"}],"stream":true}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeTextEventStream,
						Body: []byte(`data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"你"},"finish_reason":null,"logprobs":null}],"created":10,"model":"qwen-turbo","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"好"},"finish_reason":null,"logprobs":null}],"created":10,"model":"qwen-turbo","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"，"},"finish_reason":null,"logprobs":null}],"created":10,"model":"qwen-turbo","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"你"},"finish_reason":null,"logprobs":null}],"created":10,"model":"qwen-turbo","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"是"},"finish_reason":null,"logprobs":null}],"created":10,"model":"qwen-turbo","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"谁"},"finish_reason":null,"logprobs":null}],"created":10,"model":"qwen-turbo","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"？"},"finish_reason":"stop","logprobs":null}],"created":10,"model":"qwen-turbo","object":"chat.completion.chunk","usage":null}

data: [DONE]

`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "qwen case 3: non-streaming request",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "dashscope.aliyuncs.com",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-3","messages":[{"role":"user","content":"你好，你是谁？"}],"stream":false}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeApplicationJson,
						// Since the "created" field is generated by the ai-proxy plugin based on the current timestamp, it is ignored during comparison
						JsonBodyIgnoreFields: []string{"created"},
						Body:                 []byte(`{"id":"chatcmpl-llm-mock","choices":[{"index":0,"message":{"role":"assistant","content":"你好，你是谁？"},"finish_reason":"stop","logprobs":null}],"created":1738218357,"model":"qwen-turbo","object":"chat.completion","usage":{"prompt_tokens":9,"completion_tokens":1,"total_tokens":10}}`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "stepfun case 1: non-streaming request",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "api.stepfun.com",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-3","messages":[{"role":"user","content":"你好，你是谁？"}],"stream":false}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"id":"chatcmpl-llm-mock","choices":[{"index":0,"message":{"role":"assistant","content":"你好，你是谁？"},"finish_reason":"stop","logprobs":null}],"created":10,"model":"step-1-8k","object":"chat.completion","usage":{"prompt_tokens":9,"completion_tokens":1,"total_tokens":10}}`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "stepfun case 2: streaming request",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "api.stepfun.com",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-3","messages":[{"role":"user","content":"你好，你是谁？"}],"stream":true}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeTextEventStream,
						Body: []byte(`data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"你"},"finish_reason":null,"logprobs":null}],"created":10,"model":"step-1-8k","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"好"},"finish_reason":null,"logprobs":null}],"created":10,"model":"step-1-8k","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"，"},"finish_reason":null,"logprobs":null}],"created":10,"model":"step-1-8k","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"你"},"finish_reason":null,"logprobs":null}],"created":10,"model":"step-1-8k","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"是"},"finish_reason":null,"logprobs":null}],"created":10,"model":"step-1-8k","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"谁"},"finish_reason":null,"logprobs":null}],"created":10,"model":"step-1-8k","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"？"},"finish_reason":"stop","logprobs":null}],"created":10,"model":"step-1-8k","object":"chat.completion.chunk","usage":null}

data: [DONE]

`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "cloudflare case 1: non-streaming request",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "api.cloudflare.com",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-3","messages":[{"role":"user","content":"你好，你是谁？"}],"stream":false}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"id":"chatcmpl-llm-mock","choices":[{"index":0,"message":{"role":"assistant","content":"你好，你是谁？"},"finish_reason":"stop","logprobs":null}],"created":10,"model":"meta/llama-3.1-8b-instruct","object":"chat.completion","usage":{"prompt_tokens":9,"completion_tokens":1,"total_tokens":10}}`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "cloudflare case 2: streaming request",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "api.cloudflare.com",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-3","messages":[{"role":"user","content":"你好，你是谁？"}],"stream":true}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeTextEventStream,
						Body: []byte(`data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"你"},"finish_reason":null,"logprobs":null}],"created":10,"model":"meta/llama-3.1-8b-instruct","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"好"},"finish_reason":null,"logprobs":null}],"created":10,"model":"meta/llama-3.1-8b-instruct","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"，"},"finish_reason":null,"logprobs":null}],"created":10,"model":"meta/llama-3.1-8b-instruct","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"你"},"finish_reason":null,"logprobs":null}],"created":10,"model":"meta/llama-3.1-8b-instruct","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"是"},"finish_reason":null,"logprobs":null}],"created":10,"model":"meta/llama-3.1-8b-instruct","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"谁"},"finish_reason":null,"logprobs":null}],"created":10,"model":"meta/llama-3.1-8b-instruct","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"？"},"finish_reason":"stop","logprobs":null}],"created":10,"model":"meta/llama-3.1-8b-instruct","object":"chat.completion.chunk","usage":null}

data: [DONE]

`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "together-ai case 1: non-streaming request",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "api.together.xyz",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-3","messages":[{"role":"user","content":"你好，你是谁？"}],"stream":false}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"id":"chatcmpl-llm-mock","choices":[{"index":0,"message":{"role":"assistant","content":"你好，你是谁？"},"finish_reason":"stop","logprobs":null}],"created":10,"model":"meta-llama/Meta-Llama-3-8B-Instruct-Turbo","object":"chat.completion","usage":{"prompt_tokens":9,"completion_tokens":1,"total_tokens":10}}`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "together-ai case 2: streaming request",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "api.together.xyz",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-3","messages":[{"role":"user","content":"你好，你是谁？"}],"stream":true}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeTextEventStream,
						Body: []byte(`data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"你"},"finish_reason":null,"logprobs":null}],"created":10,"model":"meta-llama/Meta-Llama-3-8B-Instruct-Turbo","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"好"},"finish_reason":null,"logprobs":null}],"created":10,"model":"meta-llama/Meta-Llama-3-8B-Instruct-Turbo","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"，"},"finish_reason":null,"logprobs":null}],"created":10,"model":"meta-llama/Meta-Llama-3-8B-Instruct-Turbo","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"你"},"finish_reason":null,"logprobs":null}],"created":10,"model":"meta-llama/Meta-Llama-3-8B-Instruct-Turbo","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"是"},"finish_reason":null,"logprobs":null}],"created":10,"model":"meta-llama/Meta-Llama-3-8B-Instruct-Turbo","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"谁"},"finish_reason":null,"logprobs":null}],"created":10,"model":"meta-llama/Meta-Llama-3-8B-Instruct-Turbo","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"？"},"finish_reason":"stop","logprobs":null}],"created":10,"model":"meta-llama/Meta-Llama-3-8B-Instruct-Turbo","object":"chat.completion.chunk","usage":null}

data: [DONE]

`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "yi case 1: non-streaming request",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "api.lingyiwanwu.com",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-3","messages":[{"role":"user","content":"你好，你是谁？"}],"stream":false}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"id":"chatcmpl-llm-mock","choices":[{"index":0,"message":{"role":"assistant","content":"你好，你是谁？"},"finish_reason":"stop","logprobs":null}],"created":10,"model":"Yi-Medium","object":"chat.completion","usage":{"prompt_tokens":9,"completion_tokens":1,"total_tokens":10}}`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "yi case 2: streaming request",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "api.lingyiwanwu.com",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-3","messages":[{"role":"user","content":"你好，你是谁？"}],"stream":true}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeTextEventStream,
						Body: []byte(`data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"你"},"finish_reason":null,"logprobs":null}],"created":10,"model":"Yi-Medium","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"好"},"finish_reason":null,"logprobs":null}],"created":10,"model":"Yi-Medium","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"，"},"finish_reason":null,"logprobs":null}],"created":10,"model":"Yi-Medium","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"你"},"finish_reason":null,"logprobs":null}],"created":10,"model":"Yi-Medium","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"是"},"finish_reason":null,"logprobs":null}],"created":10,"model":"Yi-Medium","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"谁"},"finish_reason":null,"logprobs":null}],"created":10,"model":"Yi-Medium","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"？"},"finish_reason":"stop","logprobs":null}],"created":10,"model":"Yi-Medium","object":"chat.completion.chunk","usage":null}

data: [DONE]

`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "zhipuai case 1: non-streaming request",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "open.bigmodel.cn",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-3","messages":[{"role":"user","content":"你好，你是谁？"}],"stream":false}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"id":"chatcmpl-llm-mock","choices":[{"index":0,"message":{"role":"assistant","content":"你好，你是谁？"},"finish_reason":"stop","logprobs":null}],"created":10,"model":"glm-4-plus","object":"chat.completion","usage":{"prompt_tokens":9,"completion_tokens":1,"total_tokens":10}}`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "zhipuai case 2: streaming request",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "open.bigmodel.cn",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-3","messages":[{"role":"user","content":"你好，你是谁？"}],"stream":true}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeTextEventStream,
						Body: []byte(`data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"你"},"finish_reason":null,"logprobs":null}],"created":10,"model":"glm-4-plus","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"好"},"finish_reason":null,"logprobs":null}],"created":10,"model":"glm-4-plus","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"，"},"finish_reason":null,"logprobs":null}],"created":10,"model":"glm-4-plus","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"你"},"finish_reason":null,"logprobs":null}],"created":10,"model":"glm-4-plus","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"是"},"finish_reason":null,"logprobs":null}],"created":10,"model":"glm-4-plus","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"谁"},"finish_reason":null,"logprobs":null}],"created":10,"model":"glm-4-plus","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"？"},"finish_reason":"stop","logprobs":null}],"created":10,"model":"glm-4-plus","object":"chat.completion.chunk","usage":null}

data: [DONE]

`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "ollama case 1: non-streaming request",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "ollama.e2e.host",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-3","messages":[{"role":"user","content":"你好，你是谁？"}],"stream":false}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"id":"chatcmpl-llm-mock","choices":[{"index":0,"message":{"role":"assistant","content":"你好，你是谁？"},"finish_reason":"stop","logprobs":null}],"created":10,"model":"gemma3","object":"chat.completion","usage":{"prompt_tokens":9,"completion_tokens":1,"total_tokens":10}}`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "ollama case 2: streaming request",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "ollama.e2e.host",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-3","messages":[{"role":"user","content":"你好，你是谁？"}],"stream":true}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeTextEventStream,
						Body: []byte(`data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"你"},"finish_reason":null,"logprobs":null}],"created":10,"model":"gemma3","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"好"},"finish_reason":null,"logprobs":null}],"created":10,"model":"gemma3","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"，"},"finish_reason":null,"logprobs":null}],"created":10,"model":"gemma3","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"你"},"finish_reason":null,"logprobs":null}],"created":10,"model":"gemma3","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"是"},"finish_reason":null,"logprobs":null}],"created":10,"model":"gemma3","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"谁"},"finish_reason":null,"logprobs":null}],"created":10,"model":"gemma3","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"？"},"finish_reason":"stop","logprobs":null}],"created":10,"model":"gemma3","object":"chat.completion.chunk","usage":null}

data: [DONE]

`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "dify case 1: non-streaming completion request",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "api.dify.ai",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-3","messages":[{"role":"user","content":"你好"}],"stream":false}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"id":"chatcmpl-llm-mock","choices":[{"index":0,"message":{"role":"assistant","content":"USER: \n你好\n"},"finish_reason":"stop","logprobs":null}],"created":10,"model":"dify","object":"chat.completion","usage":{"prompt_tokens":9,"completion_tokens":1,"total_tokens":10}}`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "dify case 2: streaming completion request",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "api.dify.ai",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-3","messages":[{"role":"user","content":"你好"}],"stream":true}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeTextEventStream,
						Body: []byte(`data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"role":"assistant","content":"U"},"finish_reason":null,"logprobs":null}],"created":10,"model":"dify","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"role":"assistant","content":"S"},"finish_reason":null,"logprobs":null}],"created":10,"model":"dify","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"role":"assistant","content":"E"},"finish_reason":null,"logprobs":null}],"created":10,"model":"dify","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"role":"assistant","content":"R"},"finish_reason":null,"logprobs":null}],"created":10,"model":"dify","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"role":"assistant","content":":"},"finish_reason":null,"logprobs":null}],"created":10,"model":"dify","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"role":"assistant","content":" "},"finish_reason":null,"logprobs":null}],"created":10,"model":"dify","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"role":"assistant","content":"\n"},"finish_reason":null,"logprobs":null}],"created":10,"model":"dify","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"role":"assistant","content":"你"},"finish_reason":null,"logprobs":null}],"created":10,"model":"dify","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"role":"assistant","content":"好"},"finish_reason":null,"logprobs":null}],"created":10,"model":"dify","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"role":"assistant","content":"\n"},"finish_reason":null,"logprobs":null}],"created":10,"model":"dify","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"role":"assistant","content":"USER: \n你好\n"},"finish_reason":"stop","logprobs":null}],"model":"dify","object":"chat.completion.chunk","usage":{"prompt_tokens":9,"completion_tokens":1,"total_tokens":10}}

`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "grok case 1: non-streaming request",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "api.x.ai",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-3","messages":[{"role":"user","content":"你好，你是谁？"}],"stream":false}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"id":"chatcmpl-llm-mock","choices":[{"index":0,"message":{"role":"assistant","content":"你好，你是谁？"},"finish_reason":"stop","logprobs":null}],"created":10,"model":"grok-beta","object":"chat.completion","usage":{"prompt_tokens":9,"completion_tokens":1,"total_tokens":10}}`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "grok case 2: streaming request",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "api.x.ai",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-3","messages":[{"role":"user","content":"你好，你是谁？"}],"stream":true}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeTextEventStream,
						Body: []byte(`data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"你"},"finish_reason":null,"logprobs":null}],"created":10,"model":"grok-beta","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"好"},"finish_reason":null,"logprobs":null}],"created":10,"model":"grok-beta","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"，"},"finish_reason":null,"logprobs":null}],"created":10,"model":"grok-beta","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"你"},"finish_reason":null,"logprobs":null}],"created":10,"model":"grok-beta","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"是"},"finish_reason":null,"logprobs":null}],"created":10,"model":"grok-beta","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"谁"},"finish_reason":null,"logprobs":null}],"created":10,"model":"grok-beta","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"？"},"finish_reason":"stop","logprobs":null}],"created":10,"model":"grok-beta","object":"chat.completion.chunk","usage":null}

data: [DONE]

`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "openai case 1: non-streaming request",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "api.openai.com",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-3","messages":[{"role":"user","content":"你好，你是谁？"}],"stream":false}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"id":"chatcmpl-llm-mock","choices":[{"index":0,"message":{"role":"assistant","content":"你好，你是谁？"},"finish_reason":"stop","logprobs":null}],"created":10,"model":"gpt-3","object":"chat.completion","usage":{"prompt_tokens":9,"completion_tokens":1,"total_tokens":10}}`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "openai case 2: streaming request",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "api.openai.com",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-3","messages":[{"role":"user","content":"你好，你是谁？"}],"stream":true}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeTextEventStream,
						Body: []byte(`data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"你"},"finish_reason":null,"logprobs":null}],"created":10,"model":"gpt-3","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"好"},"finish_reason":null,"logprobs":null}],"created":10,"model":"gpt-3","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"，"},"finish_reason":null,"logprobs":null}],"created":10,"model":"gpt-3","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"你"},"finish_reason":null,"logprobs":null}],"created":10,"model":"gpt-3","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"是"},"finish_reason":null,"logprobs":null}],"created":10,"model":"gpt-3","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"谁"},"finish_reason":null,"logprobs":null}],"created":10,"model":"gpt-3","object":"chat.completion.chunk","usage":null}

data: {"id":"chatcmpl-llm-mock","choices":[{"index":0,"delta":{"content":"？"},"finish_reason":"stop","logprobs":null}],"created":10,"model":"gpt-3","object":"chat.completion.chunk","usage":null}

data: [DONE]

`),
					},
				},
			},
		}
		t.Run("WasmPlugins ai-proxy", func(t *testing.T) {
			for _, testcase := range testcases {
				http.MakeRequestAndExpectEventuallyConsistentResponse(t, suite.RoundTripper, suite.TimeoutConfig, suite.GatewayAddress, testcase)
			}
		})
	},
}
