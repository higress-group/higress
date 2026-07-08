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
	Register(WasmPluginCSP)
}

var WasmPluginCSP = suite.ConformanceTest{
	ShortName:   "WasmPluginCSP",
	Description: "The Ingress in the higress-conformance-infra namespace tests the CSP WASM plugin",
	Manifests:   []string{"tests/go-wasm-csp.yaml"},
	Features:    []suite.SupportedFeature{suite.WASMGoConformanceFeature},
	Test: func(t *testing.T, suite *suite.ConformanceTestSuite) {
		testcases := []http.Assertion{
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 1: enforcing CSP header is injected",
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
							"Content-Security-Policy": "default-src 'self'; img-src *",
						},
						AbsentHeaders: []string{"Content-Security-Policy-Report-Only"},
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 2: report-only mode uses the report-only header",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host: "foo.com",
						Path: "/report",
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 200,
						Headers: map[string]string{
							"Content-Security-Policy-Report-Only": "default-src 'self'",
						},
						AbsentHeaders: []string{"Content-Security-Policy"},
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 3: report_only_policy injects both headers alongside the enforced policy",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host: "foo.com",
						Path: "/dual",
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 200,
						Headers: map[string]string{
							"Content-Security-Policy":             "default-src 'self'; img-src *",
							"Content-Security-Policy-Report-Only": "default-src 'self'; img-src 'self'",
						},
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 4: override (default) strips upstream CSP headers before injecting",
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
					AdditionalResponseHeaders: map[string]string{
						"Content-Security-Policy":             "default-src https://upstream.example",
						"Content-Security-Policy-Report-Only": "default-src https://upstream.example",
					},
					ExpectedResponse: http.Response{
						StatusCode: 200,
						Headers: map[string]string{
							"Content-Security-Policy": "default-src 'self'; img-src *",
						},
						AbsentHeaders: []string{"Content-Security-Policy-Report-Only"},
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 5: override false keeps the upstream CSP header",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host: "foo.com",
						Path: "/keep",
					},
				},
				Response: http.AssertionResponse{
					AdditionalResponseHeaders: map[string]string{
						"Content-Security-Policy": "default-src https://upstream.example",
					},
					ExpectedResponse: http.Response{
						StatusCode: 200,
						Headers: map[string]string{
							"Content-Security-Policy": "default-src https://upstream.example",
						},
						AbsentHeaders: []string{"Content-Security-Policy-Report-Only"},
					},
				},
			},
		}
		t.Run("WasmPlugins csp", func(t *testing.T) {
			for _, testcase := range testcases {
				http.MakeRequestAndExpectEventuallyConsistentResponse(t, suite.RoundTripper, suite.TimeoutConfig, suite.GatewayAddress, testcase)
			}
		})
	},
}
