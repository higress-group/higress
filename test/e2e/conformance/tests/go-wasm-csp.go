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

func init() {
	Register(WasmPluginCsp)
}

var WasmPluginCsp = suite.ConformanceTest{
	ShortName:   "WasmPluginCsp",
	Description: "The Ingress in the higress-conformance-infra namespace test the content security policy WASM Plugin",
	Manifests:   []string{"tests/go-wasm-csp.yaml"},
	Features:    []suite.SupportedFeature{suite.WASMGoConformanceFeature},
	Test: func(t *testing.T, suite *suite.ConformanceTestSuite) {
		testcases := []http.Assertion{
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 1: enforcing Content-Security-Policy header is set",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host: "foo.com",
						Path: "/foo",
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 200,
						Headers: map[string]string{
							"Content-Security-Policy": "default-src 'self'",
						},
						AbsentHeaders: []string{"Content-Security-Policy-Report-Only"},
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 2: report-only Content-Security-Policy header is set",
					TargetBackend:   "infra-backend-v2",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host: "bar.com",
						Path: "/bar",
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 200,
						Headers: map[string]string{
							"Content-Security-Policy-Report-Only": "default-src 'self'; report-uri /csp-report",
						},
						AbsentHeaders: []string{"Content-Security-Policy"},
					},
				},
			},
		}
		t.Run("WasmPlugin csp", func(t *testing.T) {
			for _, testcase := range testcases {
				http.MakeRequestAndExpectEventuallyConsistentResponse(t, suite.RoundTripper, suite.TimeoutConfig, suite.GatewayAddress, testcase)
			}
		})
	},
}
