// Copyright (c) 2026 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tests

import (
	"testing"

	conformancehttp "github.com/alibaba/higress/v2/test/e2e/conformance/utils/http"
	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/suite"
)

func init() {
	Register(WasmPluginsA2AProtocol)
}

var WasmPluginsA2AProtocol = suite.ConformanceTest{
	ShortName:   "WasmPluginsA2AProtocol",
	Description: "The a2a-protocol WASM plugin enforces A2A 1.0 method and version semantics and publishes trusted metadata.",
	Manifests:   []string{"tests/go-wasm-a2a-protocol.yaml"},
	Features:    []suite.SupportedFeature{suite.WASMGoConformanceFeature},
	Test: func(t *testing.T, suite *suite.ConformanceTestSuite) {
		validBody := []byte(`{"jsonrpc":"2.0","id":"request-1","method":"GetTask","params":{"tenant":"tenant-a","id":"task-1"}}`)
		cases := []conformancehttp.Assertion{
			{
				Meta: conformancehttp.AssertionMeta{
					TestCaseName:    "canonical request publishes trusted metadata",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
					CompareTarget:   conformancehttp.CompareTargetRequest,
				},
				Request: conformancehttp.AssertionRequest{
					ActualRequest: conformancehttp.Request{
						Host: "a2a-protocol.example.com", Path: "/a2a", Method: "POST",
						ContentType: conformancehttp.ContentTypeApplicationJson, Body: validBody,
						Headers: map[string]string{
							"A2A-Version":          "1.0",
							"X-Higress-A2A-Method": "spoofed",
						},
					},
					ExpectedRequest: &conformancehttp.ExpectedRequest{Request: conformancehttp.Request{
						Host: "a2a-protocol.example.com", Path: "/a2a", Method: "POST",
						Headers: map[string]string{
							"X-Higress-A2A-Agent-Id":     "weather-agent",
							"X-Higress-A2A-Method":       "GetTask",
							"X-Higress-A2A-Task-Id":      "task-1",
							"X-Higress-A2A-Tenant":       "tenant-a",
							"X-Higress-A2A-Parse-Status": "parsed",
						},
					}},
				},
				Response: conformancehttp.AssertionResponse{ExpectedResponse: conformancehttp.Response{StatusCode: 200}},
			},
			{
				Meta: conformancehttp.AssertionMeta{TestCaseName: "missing version uses disabled 0.3 profile", CompareTarget: conformancehttp.CompareTargetResponse},
				Request: conformancehttp.AssertionRequest{ActualRequest: conformancehttp.Request{
					Host: "a2a-protocol.example.com", Path: "/a2a", Method: "POST",
					ContentType: conformancehttp.ContentTypeApplicationJson,
					Body:        []byte(`{"jsonrpc":"2.0","id":1,"method":"tasks/get","params":{"id":"task-1"}}`),
				}},
				Response: conformancehttp.AssertionResponse{ExpectedResponse: conformancehttp.Response{StatusCode: 400}},
			},
			{
				Meta: conformancehttp.AssertionMeta{TestCaseName: "slash method is rejected on the 1.0 profile", CompareTarget: conformancehttp.CompareTargetResponse},
				Request: conformancehttp.AssertionRequest{ActualRequest: conformancehttp.Request{
					Host: "a2a-protocol.example.com", Path: "/a2a", Method: "POST",
					ContentType: conformancehttp.ContentTypeApplicationJson,
					Headers:     map[string]string{"A2A-Version": "1.0"},
					Body:        []byte(`{"jsonrpc":"2.0","id":1,"method":"tasks/get","params":{"id":"task-1"}}`),
				}},
				Response: conformancehttp.AssertionResponse{ExpectedResponse: conformancehttp.Response{StatusCode: 400}},
			},
		}
		for _, testcase := range cases {
			t.Run(testcase.Meta.TestCaseName, func(t *testing.T) {
				conformancehttp.MakeRequestAndExpectEventuallyConsistentResponse(t, suite.RoundTripper, suite.TimeoutConfig, suite.GatewayAddress, testcase)
			})
		}
	},
}
