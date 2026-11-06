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
	Register(WasmPluginsNginxRewriteCompatible)
}

// The nginx-rewrite-compatible plugin rewrites the request path in place, so
// these cases use the default CompareTargetRequest mode: they assert on the
// request the backend actually received after the plugin ran, which is the only
// way to observe the effect of a rewrite from outside. ExpectedRequest.Path is
// therefore the rewritten path, not the path the client asked for.
//
// The plugin runs in one request callback with no cross-pass state, which is the
// whole point of the plugin: it replaces the Nginx rewrite+set combination that
// CVE-2026-42945 affects.
var WasmPluginsNginxRewriteCompatible = suite.ConformanceTest{
	ShortName:   "WasmPluginsNginxRewriteCompatible",
	Description: "The Ingress in the higress-conformance-infra namespace tests the nginx-rewrite-compatible WASM plugin.",
	Manifests:   []string{"tests/go-wasm-nginx-rewrite-compatible.yaml"},
	Features:    []suite.SupportedFeature{suite.WASMGoConformanceFeature},
	Test: func(t *testing.T, suite *suite.ConformanceTestSuite) {
		testcases := []http.Assertion{
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 1: a matching prefix is rewritten with its capture group before reaching the backend",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:    "rewrite.example.com",
						Path:    "/old/abc",
						Method:  stdHttp.MethodGet,
						Headers: map[string]string{"X-Case": "one"},
					},
					ExpectedRequest: &http.ExpectedRequest{
						Request: http.Request{
							Path: "/new/abc",
						},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: stdHttp.StatusOK,
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 2: a request that matches no rule keeps its original path",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:    "rewrite.example.com",
						Path:    "/untouched",
						Method:  stdHttp.MethodGet,
						Headers: map[string]string{"X-Case": "two"},
					},
					ExpectedRequest: &http.ExpectedRequest{
						Request: http.Request{
							Path: "/untouched",
						},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: stdHttp.StatusOK,
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 3: query_append adds a fragment while keeping the original query string",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:    "rewrite.example.com",
						Path:    "/api/v1?existing=1",
						Method:  stdHttp.MethodGet,
						Headers: map[string]string{"X-Case": "three"},
					},
					ExpectedRequest: &http.ExpectedRequest{
						Request: http.Request{
							Path: "/internal?existing=1&migrated=true",
						},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: stdHttp.StatusOK,
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 4: query_template replaces the query string using capture groups",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:    "rewrite.example.com",
						Path:    "/x/first/second?dropped=1",
						Method:  stdHttp.MethodGet,
						Headers: map[string]string{"X-Case": "four"},
					},
					ExpectedRequest: &http.ExpectedRequest{
						Request: http.Request{
							Path: "/y?a=first&b=second",
						},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: stdHttp.StatusOK,
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 5: pass_to_upstream propagates a captured variable as a request header",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:    "rewrite.example.com",
						Path:    "/tag/xyz",
						Method:  stdHttp.MethodGet,
						Headers: map[string]string{"X-Case": "five"},
					},
					ExpectedRequest: &http.ExpectedRequest{
						Request: http.Request{
							Path: "/tagged/xyz",
							Headers: map[string]string{
								"X-Higress-Rewrite-Var-Origin": "xyz",
							},
						},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: stdHttp.StatusOK,
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 6: a variable without pass_to_upstream is not exposed as a request header",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:    "rewrite.example.com",
						Path:    "/private/secret",
						Method:  stdHttp.MethodGet,
						Headers: map[string]string{"X-Case": "six"},
					},
					ExpectedRequest: &http.ExpectedRequest{
						Request: http.Request{
							Path: "/hidden/secret",
						},
						AbsentHeaders: []string{
							"X-Higress-Rewrite-Var-Internal",
						},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: stdHttp.StatusOK,
					},
				},
			},
		}
		t.Run("WasmPlugins nginx-rewrite-compatible", func(t *testing.T) {
			for _, testcase := range testcases {
				http.MakeRequestAndExpectEventuallyConsistentResponse(t, suite.RoundTripper, suite.TimeoutConfig, suite.GatewayAddress, testcase)
			}
		})
	},
}
