// Copyright (c) 2022 Alibaba Group Holding Ltd.
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

	"github.com/alibaba/higress/test/e2e/conformance/utils/http"
	"github.com/alibaba/higress/test/e2e/conformance/utils/suite"
)

func init() {
	Register(WasmPluginsAiSecurityGuard)
}

var WasmPluginsAiSecurityGuard = suite.ConformanceTest{
	ShortName:   "WasmPluginAiSecurityGuard",
	Description: "The Ingress in the higress-conformance-infra namespace test the ai-security-guard WASM plugin.",
	Features:    []suite.SupportedFeature{suite.WASMGoConformanceFeature},
	Manifests:   []string{"tests/go-wasm-ai-security-guard.yaml"},
	Test: func(t *testing.T, suite *suite.ConformanceTestSuite) {
		testcases := []http.Assertion{{Meta: http.AssertionMeta{
			TestCaseName:    "case 1: normal request passes through",
			TargetBackend:   "infra-backend-v1",
			TargetNamespace: "higress-conformance-infra",
		}, Request: http.AssertionRequest{ActualRequest: http.Request{
			Host:        "api.openai.com",
			Path:        "/v1/chat/completions",
			Method:      "POST",
			ContentType: http.ContentTypeApplicationJson,
			Body: []byte(`{  
                                                        "model": "gpt-4",  
                                                        "messages": [{"role":"user","content":"Hello, how are you?"}],  
                                                        "stream": false  
                                                }`),
		}, ExpectedRequest: &http.ExpectedRequest{Request: http.Request{
			Host:        "api.openai.com",
			Path:        "/v1/chat/completions",
			Method:      "POST",
			ContentType: http.ContentTypeApplicationJson,
			Body: []byte(`{  
                                                                "model": "gpt-4",  
                                                                "messages": [{"role":"user","content":"Hello, how are you?"}],  
                                                                "stream": false  
                                                        }`),
		}}}, Response: http.AssertionResponse{ExpectedResponse: http.Response{
			StatusCode: 200,
		}}}, {Meta: http.AssertionMeta{
			TestCaseName:  "case 2: malicious request blocked",
			CompareTarget: http.CompareTargetResponse}, Request: http.AssertionRequest{ActualRequest: http.Request{
			Host:        "api.openai.com",
			Path:        "/v1/chat/completions",
			Method:      "POST",
			ContentType: http.ContentTypeApplicationJson,
			Body: []byte(`{  
                                                        "model": "gpt-4",  
                                                        "messages": [{"role":"user","content":"这是一段非法内容"}],  
                                                        "stream": false  
                                                }`),
		}}, Response: http.AssertionResponse{ExpectedResponse: http.Response{
			StatusCode:  200,
			ContentType: http.ContentTypeApplicationJson,
			Body:        []byte(`{"id":"chatcmpl-mock","object":"chat.completion","model":"from-security-guard","choices":[{"index":0,"message":{"role":"assistant","content":"很抱歉，我无法回答您的问题"},"logprobs":null,"finish_reason":"stop"}],"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}}`),
		}}}, {Meta: http.AssertionMeta{
			TestCaseName:  "case 3: malicious streaming request blocked",
			CompareTarget: http.CompareTargetResponse}, Request: http.AssertionRequest{ActualRequest: http.Request{
			Host:        "api.openai.com",
			Path:        "/v1/chat/completions",
			Method:      "POST",
			ContentType: http.ContentTypeApplicationJson,
			Body: []byte(`{  
                                                        "model": "gpt-4",  
                                                        "messages": [{"role":"user","content":"这是一段非法内容"}],  
                                                        "stream": true  
                                                }`),
		}}, Response: http.AssertionResponse{ExpectedResponse: http.Response{
			StatusCode:  200,
			ContentType: http.ContentTypeTextEventStream,
			Body:        []byte(`data:{"id":"chatcmpl-mock","object":"chat.completion.chunk","model":"from-security-guard","choices":[{"index":0,"delta":{"role":"assistant","content":"很抱歉，我无法回答您的问题"},"logprobs":null,"finish_reason":null}]}\n\ndata:{"id":"chatcmpl-mock","object":"chat.completion.chunk","model":"from-security-guard","choices":[{"index":0,"delta":{},"logprobs":null,"finish_reason":"stop"}],"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}}\n\ndata: [DONE]`),
		}}}, {Meta: http.AssertionMeta{
			TestCaseName:  "case 4: custom protocol format",
			CompareTarget: http.CompareTargetResponse}, Request: http.AssertionRequest{ActualRequest: http.Request{
			Host:        "custom.ai.com",
			Path:        "/api/chat",
			Method:      "POST",
			ContentType: http.ContentTypeApplicationJson,
			Body: []byte(`{  
                                                        "input": {"prompt": "这是一段非法内容"},  
                                                        "model": "custom-model"  
                                                }`),
		}}, Response: http.AssertionResponse{ExpectedResponse: http.Response{
			StatusCode:  200,
			ContentType: http.ContentTypeApplicationJson,
			Body:        []byte(`"很抱歉，我无法回答您的问题"`),
		}}}}
		t.Run("WasmPlugins ai-security-guard", func(t *testing.T) {
			for _, testcase := range testcases {
				http.MakeRequestAndExpectEventuallyConsistentResponse(t, suite.RoundTripper, suite.TimeoutConfig, suite.GatewayAddress, testcase)

			}

		})

	},
}
