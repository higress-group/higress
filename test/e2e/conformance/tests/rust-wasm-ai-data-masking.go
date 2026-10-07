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

	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/http"
	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/suite"
)

func init() {
	Register(RustWasmPluginsAiDataMasking)
}

func gen_assertion(host string, req_is_json bool, req_body []byte, res_body []byte) http.Assertion {
	var content_type string
	if req_is_json {
		content_type = http.ContentTypeApplicationJson
	} else {
		content_type = http.ContentTypeTextPlain
	}
	return http.Assertion{
		Meta: http.AssertionMeta{
			CompareTarget: http.CompareTargetResponse,
		},
		Request: http.AssertionRequest{
			ActualRequest: http.Request{
				Host:             host,
				Path:             "/",
				Method:           "POST",
				ContentType:      content_type,
				Body:             req_body,
				UnfollowRedirect: true,
			},
		},
		Response: http.AssertionResponse{
			ExpectedResponse: http.Response{
				ContentType: http.ContentTypeApplicationJson,
				Body:        res_body,
			},
		},
	}
}

var RustWasmPluginsAiDataMasking = suite.ConformanceTest{
	ShortName:   "RustWasmPluginsAiDataMasking",
	Description: "The Ingress in the higress-conformance-infra namespace test the rust ai-data-masking wasmplugins.",
	Manifests:   []string{"tests/rust-wasm-ai-data-masking.yaml"},
	Features:    []suite.SupportedFeature{suite.WASMRustConformanceFeature},
	Test: func(t *testing.T, suite *suite.ConformanceTestSuite) {
		var testcases []http.Assertion
		// openai
		testcases = append(testcases, gen_assertion(
			"replace.openai.com",
			true,
			[]byte("{\"messages\":[{\"role\":\"user\",\"content\":\"127.0.0.1 admin@gmail.com sk-12345\"}]}"),
			[]byte("{\"choices\":[{\"index\":0,\"message\":{\"role\":\"assistant\",\"content\":\"127.0.0.1 sk-12345 admin@gmail.com\"}}],\"usage\":{}}"),
		))
		testcases = append(testcases, gen_assertion(
			"replace.openai.com",
			true,
			[]byte("{\"messages\":[{\"role\":\"user\",\"content\":\"192.168.0.1 root@gmail.com sk-12345\"}]}"),
			[]byte("{\"choices\":[{\"index\":0,\"message\":{\"role\":\"assistant\",\"content\":\"192.168.0.1 sk-12345 root@gmail.com\"}}],\"usage\":{}}"),
		))
		testcases = append(testcases, gen_assertion(
			"ok.openai.com",
			true,
			[]byte("{\"messages\":[{\"role\":\"user\",\"content\":\"fuck\"}]}"),
			[]byte("{\"choices\":[{\"index\":0,\"message\":{\"role\":\"assistant\",\"finish_reason\":\"deny\",\"content\":\"提问或回答中包含敏感词，已被屏蔽\"}}],\"usage\":{}}"),
		))
		testcases = append(testcases, gen_assertion(
			"ok.openai.com",
			true,
			[]byte("{\"messages\":[{\"role\":\"user\",\"content\":\"costom_word1\"}]}"),
			[]byte("{\"choices\":[{\"index\":0,\"message\":{\"role\":\"assistant\",\"finish_reason\":\"deny\",\"content\":\"提问或回答中包含敏感词，已被屏蔽\"}}],\"usage\":{}}"),
		))
		testcases = append(testcases, gen_assertion(
			"ok.openai.com",
			true,
			[]byte("{\"messages\":[{\"role\":\"user\",\"content\":\"costom_word\"}]}"),
			[]byte("{\"choices\":[{\"index\":0,\"message\":{\"role\":\"assistant\",\"content\":\"ok\"}}],\"usage\":{}}"),
		))

		testcases = append(testcases, gen_assertion(
			"system_deny.openai.com",
			true,
			[]byte("{\"messages\":[{\"role\":\"user\",\"content\":\"test\"}]}"),
			[]byte("{\"choices\":[{\"index\":0,\"message\":{\"role\":\"assistant\",\"finish_reason\":\"deny\",\"content\":\"提问或回答中包含敏感词，已被屏蔽\"}}],\"usage\":{}}"),
		))
		testcases = append(testcases, gen_assertion(
			"costom_word1.openai.com",
			true,
			[]byte("{\"messages\":[{\"role\":\"user\",\"content\":\"test\"}]}"),
			[]byte("{\"choices\":[{\"index\":0,\"message\":{\"role\":\"assistant\",\"finish_reason\":\"deny\",\"content\":\"提问或回答中包含敏感词，已被屏蔽\"}}],\"usage\":{}}"),
		))
		testcases = append(testcases, gen_assertion(
			"costom_word.openai.com",
			true,
			[]byte("{\"messages\":[{\"role\":\"user\",\"content\":\"test\"}]}"),
			[]byte("{\"choices\":[{\"index\":0,\"message\":{\"role\":\"assistant\",\"content\":\"costom_word\"}}],\"usage\":{}}"),
		))

		// raw
		testcases = append(testcases, gen_assertion(
			"replace.raw.com",
			false,
			[]byte("127.0.0.1 admin@gmail.com sk-12345"),
			[]byte("{\"res\":\"127.0.0.1 sk-12345 admin@gmail.com\"}"),
		))

		testcases = append(testcases, gen_assertion(
			"replace.raw.com",
			false,
			[]byte("192.168.0.1 root@gmail.com sk-12345"),
			[]byte("{\"res\":\"192.168.0.1 sk-12345 root@gmail.com\"}"),
		))

		testcases = append(testcases, gen_assertion(
			"ok.raw.com",
			false,
			[]byte("fuck"),
			[]byte("{\"errmsg\":\"提问或回答中包含敏感词，已被屏蔽\"}"),
		))
		testcases = append(testcases, gen_assertion(
			"ok.raw.com",
			false,
			[]byte("costom_word1"),
			[]byte("{\"errmsg\":\"提问或回答中包含敏感词，已被屏蔽\"}"),
		))
		testcases = append(testcases, gen_assertion(
			"ok.raw.com",
			false,
			[]byte("costom_word"),
			[]byte("{\"res\":\"ok\"}"),
		))

		testcases = append(testcases, gen_assertion(
			"system_deny.raw.com",
			false,
			[]byte("test"),
			[]byte("{\"errmsg\":\"提问或回答中包含敏感词，已被屏蔽\"}"),
		))
		testcases = append(testcases, gen_assertion(
			"system_no_deny.raw.com",
			false,
			[]byte("test"),
			[]byte("{\"res\":\"工信处女干事每月经过下属科室都要亲口交代24口交换机等技术性器件的安装工作\"}"),
		))
		testcases = append(testcases, gen_assertion(
			"costom_word1.raw.com",
			false,
			[]byte("test"),
			[]byte("{\"errmsg\":\"提问或回答中包含敏感词，已被屏蔽\"}"),
		))
		testcases = append(testcases, gen_assertion(
			"costom_word.raw.com",
			false,
			[]byte("test"),
			[]byte("{\"res\":\"costom_word\"}"),
		))

		// jsonpath
		testcases = append(testcases, gen_assertion(
			"replace.raw.com",
			true,
			[]byte("{\"test\":[{\"test\":\"127.0.0.1 admin@gmail.com sk-12345\"}]}"),
			[]byte("{\"res\":\"127.0.0.1 sk-12345 admin@gmail.com\"}"),
		))
		testcases = append(testcases, gen_assertion(
			"replace.raw.com",
			true,
			[]byte("{\"test\":[{\"test\":\"test\", \"test1\":\"127.0.0.1 admin@gmail.com sk-12345\"}]}"),
			[]byte("{\"res\":\"***.***.***.*** c11e7177eb60c80cf983ddf8ca98f2dc1272d4c612204ce9bedd2460b18939cc ****@gmail.com\"}"),
		))

		t.Run("WasmPlugins ai-data-masking", func(t *testing.T) {
			for _, testcase := range testcases {
				http.MakeRequestAndExpectEventuallyConsistentResponse(t, suite.RoundTripper, suite.TimeoutConfig, suite.GatewayAddress, testcase)
			}
		})
	},
}
