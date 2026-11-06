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
	Register(WasmPluginsOAuthExpiredToken)
}

var WasmPluginsOAuthExpiredToken = suite.ConformanceTest{
	ShortName:   "WasmPluginsOAuthExpiredToken",
	Description: "The Ingress in the higress-conformance-infra namespace test the oauth WASM plugin.",
	Manifests:   []string{"tests/go-wasm-oauth-expired-token.yaml"},
	Features:    []suite.SupportedFeature{suite.WASMGoConformanceFeature},
	Test: func(t *testing.T, suite *suite.ConformanceTestSuite) {
		testcases := []http.Assertion{
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 1: expired token",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host: "foo.com",
						Path: "/foo",
						Headers: map[string]string{"Authorization": "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6ImFwcGxpY2F0aW9uL2F0K2p3dCJ9.eyJhdWQiOiJkZWZhdWx0IiwiY2xpZW50X2lkIjoiOTUxNWI1NjQtMGIxZC0xMWVlLTljNGMtMDAxNjNlMTI1MGI1IiwiZXhwIjoxNzAxNDE4NzU5LCJpYXQiOjE3MDE0MTE1NTksImlzcyI6IkhpZ3Jlc3MtR2F0ZXdheSIsImp0aSI6IjQ0YTMzYjc4LWNmYWItNGYzYS1iZDQ3LTQ1Y2Y5ZjM0YjVmZSIsInN1YiI6ImNvbnN1bWVyMSJ9.EIDCTVx4Wt6u5fRngFwgRo-qfDSKp6sUg4fKA7MYpuE"},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 401,
					},
					ExpectedResponseNoRequest: true,
				},
			},
		
		}

		t.Run("WasmPlugins oauth expired token", func(t *testing.T) {
			for _, testcase := range testcases {
				http.MakeRequestAndExpectEventuallyConsistentResponse(t, suite.RoundTripper, suite.TimeoutConfig, suite.GatewayAddress, testcase)
			}
		})
	},
}
