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
	Register(WasmPluginsModelRouter)
}

var WasmPluginsModelRouter = suite.ConformanceTest{
	ShortName:   "WasmPluginModelRouter",
	Description: "The Ingress in the higress-conformance-ai-backend namespace tests the model-router WASM plugin.",
	Features:    []suite.SupportedFeature{suite.WASMCPPConformanceFeature},
	Manifests:   []string{"tests/cpp-wasm-model-router.yaml"},
	Test: func(t *testing.T, suite *suite.ConformanceTestSuite) {
		testcases := []http.Assertion{
			{
				// 测试提取 provider 信息并添加到请求头
				Meta: http.AssertionMeta{
					TestCaseName:  "model router case 1: add provider header",
					CompareTarget: http.CompareTargetRequest,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "foo.com",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"qwen/qwen-long","messages":[{"role":"user","content":"测试消息"}]}`),
					},
					ExpectedRequest: &http.ExpectedRequest{
						Request: http.Request{
							Host:        "foo.com",
							Path:        "/v1/chat/completions",
							Method:      "POST",
							ContentType: http.ContentTypeApplicationJson,
							Body:        []byte(`{"model":"qwen-long","messages":[{"role":"user","content":"测试消息"}]}`),
							Headers: map[string]string{
								"x-higress-llm-provider": "qwen",
							},
						},
					},
				},
			},
			{
				// 测试将 model 参数直接添加到请求头
				Meta: http.AssertionMeta{
					TestCaseName:  "model router case 2: model to header",
					CompareTarget: http.CompareTargetRequest,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "foo.com",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"gpt-4","messages":[{"role":"user","content":"测试消息"}]}`),
					},
					ExpectedRequest: &http.ExpectedRequest{
						Request: http.Request{
							Host:        "foo.com",
							Path:        "/v1/chat/completions",
							Method:      "POST",
							ContentType: http.ContentTypeApplicationJson,
							Body:        []byte(`{"model":"gpt-4","messages":[{"role":"user","content":"测试消息"}]}`),
							Headers: map[string]string{
								"x-higress-llm-model": "gpt-4",
							},
						},
					},
				},
			},
			{
				// 测试自定义 modelKey 配置
				Meta: http.AssertionMeta{
					TestCaseName:  "model router case 3: custom model key",
					CompareTarget: http.CompareTargetRequest,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "foo.com",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"engine":"openai/gpt-3.5-turbo","messages":[{"role":"user","content":"测试消息"}]}`),
					},
					ExpectedRequest: &http.ExpectedRequest{
						Request: http.Request{
							Host:        "foo.com",
							Path:        "/v1/chat/completions",
							Method:      "POST",
							ContentType: http.ContentTypeApplicationJson,
							Body:        []byte(`{"engine":"gpt-3.5-turbo","messages":[{"role":"user","content":"测试消息"}]}`),
							Headers: map[string]string{
								"x-higress-llm-provider": "openai",
							},
						},
					},
				},
			},
			{
				// 测试路径匹配功能 - 匹配的路径
				Meta: http.AssertionMeta{
					TestCaseName:  "model router case 4: path suffix match",
					CompareTarget: http.CompareTargetRequest,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "foo.com",
						Path:        "/v1/embeddings",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"text-embedding-ada-002","input":"测试文本"}`),
					},
					ExpectedRequest: &http.ExpectedRequest{
						Request: http.Request{
							Host:        "foo.com",
							Path:        "/v1/embeddings",
							Method:      "POST",
							ContentType: http.ContentTypeApplicationJson,
							Body:        []byte(`{"model":"text-embedding-ada-002","input":"测试文本"}`),
							Headers: map[string]string{
								"x-higress-llm-model": "text-embedding-ada-002",
							},
						},
					},
				},
			},
			{
				// 测试路径匹配功能 - 不匹配的路径（不应该处理）
				Meta: http.AssertionMeta{
					TestCaseName:  "model router case 5: path suffix not match",
					CompareTarget: http.CompareTargetRequest,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "foo.com",
						Path:        "/v1/models",
						Method:      "GET",
						ContentType: http.ContentTypeApplicationJson,
					},
					ExpectedRequest: &http.ExpectedRequest{
						Request: http.Request{
							Host:        "foo.com",
							Path:        "/v1/models",
							Method:      "GET",
							ContentType: http.ContentTypeApplicationJson,
							// 不应该添加任何额外的请求头
						},
					},
				},
			},
			{
				// 测试复合功能：同时添加 provider 和 model 头
				Meta: http.AssertionMeta{
					TestCaseName:  "model router case 6: both provider and model headers",
					CompareTarget: http.CompareTargetRequest,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "foo.com",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte(`{"model":"anthropic/claude-3","messages":[{"role":"user","content":"测试消息"}]}`),
					},
					ExpectedRequest: &http.ExpectedRequest{
						Request: http.Request{
							Host:        "foo.com",
							Path:        "/v1/chat/completions",
							Method:      "POST",
							ContentType: http.ContentTypeApplicationJson,
							Body:        []byte(`{"model":"claude-3","messages":[{"role":"user","content":"测试消息"}]}`),
							Headers: map[string]string{
								"x-higress-llm-provider": "anthropic",
								"x-higress-llm-model":    "claude-3",
							},
						},
					},
				},
			},
		}
		t.Run("WasmPlugins model-router", func(t *testing.T) {
			for _, testcase := range testcases {
				http.MakeRequestAndExpectEventuallyConsistentResponse(t, suite.RoundTripper, suite.TimeoutConfig, suite.GatewayAddress, testcase)
			}
		})
	},
}
