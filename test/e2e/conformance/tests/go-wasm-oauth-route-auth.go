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
	Register(WasmPluginsOAuthRouteAuth)
}

var WasmPluginsOAuthRouteAuth = suite.ConformanceTest{
	ShortName:   "WasmPluginsOAuthRouteAuth",
	Description: "The Ingress in the higress-conformance-infra namespace test the oauth WASM plugin, disabling credentials globally",
	Manifests:   []string{"tests/go-wasm-oauth-route-auth.yaml"},
	Features:    []suite.SupportedFeature{suite.WASMGoConformanceFeature},
	Test: func(t *testing.T, suite *suite.ConformanceTestSuite) {
		testcases := []http.Assertion{
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 1: audience not match this route",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host: "foo.com",
						Path: "/foo",
						Headers: map[string]string{"Authorization": "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6ImFwcGxpY2F0aW9uL2F0K2p3dCJ9.eyJhdWQiOiJkZWZhdWx0IiwiY2xpZW50X2lkIjoiOTUxNWI1NjQtMGIxZC0xMWVlLTljNGMtMDAxNjNlMTI1MGI1IiwiZXhwIjoxNzAxNTI2MDY2LCJpYXQiOjE3MDE1MTg4NjYsImlzcyI6IkhpZ3Jlc3MtR2F0ZXdheSIsImp0aSI6Ijc4MDYwYjVmLWRiY2EtNDljZi04MmM2LWEwNzRiY2UyY2QzZCIsInN1YiI6ImNvbnN1bWVyMSJ9.NOu7nDL7ebRoV7AnBx2L_JS4dp-G7b9ERk-s2YPS2BI"},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 403,
					},
					ExpectedResponseNoRequest: true,
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 2: audience match this route",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host: "foo.com",
						Path: "/foo",
						Headers: map[string]string{"Authorization": "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6ImFwcGxpY2F0aW9uL2F0K2p3dCJ9.eyJhdWQiOiJoaWdyZXNzLWNvbmZvcm1hbmNlLWluZnJhL3dhc21wbHVnaW4tb2F1dGgiLCJjbGllbnRfaWQiOiI5NTE1YjU2NC0wYjFkLTExZWUtOWM0Yy0wMDE2M2UxMjUwYjUiLCJleHAiOjE3MDE1MjYwNjYsImlhdCI6MTcwMTUxODg2NiwiaXNzIjoiSGlncmVzcy1HYXRld2F5IiwianRpIjoiNzgwNjBiNWYtZGJjYS00OWNmLTgyYzYtYTA3NGJjZTJjZDNkIiwic3ViIjoiY29uc3VtZXIxIn0.Hml1oK6Vkzd_G5AAAQFFHJ_O20JjeXts8XZxBvGjheA"},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 200,
					},
					ExpectedResponseNoRequest: true,
				},
			},
		}

		t.Run("WasmPlugins oauth route auth", func(t *testing.T) {
			for _, testcase := range testcases {
				http.MakeRequestAndExpectEventuallyConsistentResponse(t, suite.RoundTripper, suite.TimeoutConfig, suite.GatewayAddress, testcase)
			}
		})
	},
}
