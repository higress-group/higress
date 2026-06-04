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
	Register(WasmPluginsRerouteMatchRule)
}

// WasmPluginsRerouteMatchRule tests that wasm plugin matchRules work correctly
// after a reroute triggered by a previous plugin (issue #3571).
//
// Scenario:
//   - Ingress "reroute-match-default": host reroute-match.com, path /, no header constraint
//   - Ingress "reroute-match-target": host reroute-match.com, path /, header x-user-id: 1
//   - Plugin A (transformer, priority 400): matchRule for "reroute-match-default",
//     maps query param "userId" to header "x-user-id", causing reroute
//   - Plugin B (custom-response, priority 200): matchRule for "reroute-match-target",
//     returns custom response {"hello":"world"}
//
// When sending GET /?userId=1, plugin A adds x-user-id:1 header and triggers reroute.
// Plugin B should see the NEW route "reroute-match-target" and return the custom response.
var WasmPluginsRerouteMatchRule = suite.ConformanceTest{
	ShortName:   "WasmPluginsRerouteMatchRule",
	Description: "Tests that wasm plugin matchRules work correctly after reroute triggered by a previous plugin (issue #3571).",
	Manifests:   []string{"tests/go-wasm-reroute-match-rule.yaml"},
	Features:    []suite.SupportedFeature{suite.WASMGoConformanceFeature},
	Test: func(t *testing.T, suite *suite.ConformanceTestSuite) {
		testcases := []http.Assertion{
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "case 1: matchRule should work after reroute - custom response expected",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host: "reroute-match.com",
						Path: "/?userId=1",
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 200,
						Headers: map[string]string{
							"x-matched": "rerouted",
						},
						ContentType: http.ContentTypeApplicationJson,
						Body:        []byte("{\"hello\":\"world\"}"),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 2: no reroute without userId param - normal backend response",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host: "reroute-match.com",
						Path: "/get",
					},
					ExpectedRequest: &http.ExpectedRequest{
						Request: http.Request{
							Host: "reroute-match.com",
							Path: "/get",
						},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 200,
					},
				},
			},
		}
		t.Run("WasmPlugins reroute match rule", func(t *testing.T) {
			for _, testcase := range testcases {
				http.MakeRequestAndExpectEventuallyConsistentResponse(t, suite.RoundTripper, suite.TimeoutConfig, suite.GatewayAddress, testcase)
			}
		})
	},
}
