// Copyright (c) 2026 Alibaba Group Holding Ltd.
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
	stdHttp "net/http"
	"testing"

	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/http"
	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/suite"
)

func init() {
	Register(WasmPluginsCors)
}

// The cors plugin enables CORS response headers for cross-origin requests.
// For an actual (non-preflight) CORS request whose Origin and Method are
// allowed, the plugin lets the request reach the backend and appends the CORS
// response headers on the way back.
// For a preflight (OPTIONS) request the plugin short-circuits the request and
// answers it directly with 204 No Content: a valid preflight carries the
// configured Access-Control-Allow-* headers, while an invalid one (origin,
// method or requested headers not allowed) carries none of them, and the
// browser is left to reject it.
var WasmPluginsCors = suite.ConformanceTest{
	ShortName:   "WasmPluginsCors",
	Description: "The Ingress in the higress-conformance-infra namespace tests the cors WASM plugin.",
	Manifests:   []string{"tests/go-wasm-cors.yaml"},
	Features:    []suite.SupportedFeature{suite.WASMGoConformanceFeature},
	Test: func(t *testing.T, suite *suite.ConformanceTestSuite) {
		testcases := []http.Assertion{
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 1: actual request with an allowed origin is forwarded and gets the CORS response header",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:    "cors.example.com",
						Path:    "/foo",
						Method:  stdHttp.MethodGet,
						Headers: map[string]string{"Origin": "http://cors.example.net"},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: stdHttp.StatusOK,
						Headers: map[string]string{
							"Access-Control-Allow-Origin": "http://cors.example.net",
						},
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 2: actual request with a disallowed origin is still forwarded but gets no CORS response header",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:    "cors.example.com",
						Path:    "/foo",
						Method:  stdHttp.MethodGet,
						Headers: map[string]string{"Origin": "http://evil.example.net"},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: stdHttp.StatusOK,
						AbsentHeaders: []string{
							"Access-Control-Allow-Origin",
							"Access-Control-Allow-Methods",
							"Access-Control-Allow-Headers",
							"Access-Control-Allow-Credentials",
							"Access-Control-Max-Age",
						},
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "case 3: valid preflight request is answered by the plugin with the configured headers",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:   "cors.example.com",
						Path:   "/foo",
						Method: stdHttp.MethodOptions,
						Headers: map[string]string{
							"Origin":                         "http://cors.example.net",
							"Access-Control-Request-Method":  stdHttp.MethodPost,
							"Access-Control-Request-Headers": "X-Requested-With, Content-Type",
						},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: stdHttp.StatusNoContent,
						Headers: map[string]string{
							"Access-Control-Allow-Origin":      "http://cors.example.net",
							"Access-Control-Allow-Methods":     "GET,POST",
							"Access-Control-Allow-Headers":     "Content-Type,X-Requested-With",
							"Access-Control-Allow-Credentials": "true",
							"Access-Control-Max-Age":           "3600",
							"Vary":                             "Origin",
						},
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "case 4: preflight request from a disallowed origin gets no CORS headers",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:   "cors.example.com",
						Path:   "/foo",
						Method: stdHttp.MethodOptions,
						Headers: map[string]string{
							"Origin":                        "http://evil.example.net",
							"Access-Control-Request-Method": stdHttp.MethodPost,
						},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: stdHttp.StatusNoContent,
						AbsentHeaders: []string{
							"Access-Control-Allow-Origin",
							"Access-Control-Allow-Methods",
							"Access-Control-Allow-Headers",
							"Access-Control-Allow-Credentials",
							"Access-Control-Max-Age",
						},
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "case 5: preflight request with a disallowed method gets no CORS headers",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:   "cors.example.com",
						Path:   "/foo",
						Method: stdHttp.MethodOptions,
						Headers: map[string]string{
							"Origin":                        "http://cors.example.net",
							"Access-Control-Request-Method": stdHttp.MethodDelete,
						},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: stdHttp.StatusNoContent,
						AbsentHeaders: []string{
							"Access-Control-Allow-Origin",
							"Access-Control-Allow-Methods",
							"Access-Control-Allow-Headers",
							"Access-Control-Allow-Credentials",
							"Access-Control-Max-Age",
						},
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "case 6: preflight request with a disallowed header gets no CORS headers",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:   "cors.example.com",
						Path:   "/foo",
						Method: stdHttp.MethodOptions,
						Headers: map[string]string{
							"Origin":                         "http://cors.example.net",
							"Access-Control-Request-Method":  stdHttp.MethodGet,
							"Access-Control-Request-Headers": "X-Not-Allowed",
						},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: stdHttp.StatusNoContent,
						AbsentHeaders: []string{
							"Access-Control-Allow-Origin",
							"Access-Control-Allow-Methods",
							"Access-Control-Allow-Headers",
							"Access-Control-Allow-Credentials",
							"Access-Control-Max-Age",
						},
					},
				},
			},
		}
		t.Run("WasmPlugins cors", func(t *testing.T) {
			for _, testcase := range testcases {
				http.MakeRequestAndExpectEventuallyConsistentResponse(t, suite.RoundTripper, suite.TimeoutConfig, suite.GatewayAddress, testcase)
			}
		})
	},
}
