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
	Register(WasmPluginsOAuthGlobalAuth)
}

var WasmPluginsOAuthGlobalAuth = suite.ConformanceTest{
	ShortName:   "WasmPluginsOAuthGlobalAuth",
	Description: "The Ingress in the higress-conformance-infra namespace test the oauth WASM plugin, with global_auth false",
	Manifests:   []string{"tests/go-wasm-oauth-global-auth.yaml"},
	Features:    []suite.SupportedFeature{suite.WASMGoConformanceFeature},
	Test: func(t *testing.T, suite *suite.ConformanceTestSuite) {
		testcases := []http.Assertion{
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 1: with no ruleset under this route, oauth let every request pass",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host: "foo2.com",
						Path: "/foo",
						Headers: map[string]string{"Buthorization": "Aearer x.x.x"},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 200,
					},
					ExpectedResponseNoRequest: true,
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 2: with some ruleset under this route, oauth checks token and route rule. token verify success, client in the route's allowset",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host: "foo1.com",
						Path: "/foo",
						// consumer1
						Headers: map[string]string{"Authorization": "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6ImFwcGxpY2F0aW9uL2F0K2p3dCJ9.eyJhdWQiOiJkZWZhdWx0IiwiY2xpZW50X2lkIjoiOTUxNWI1NjQtMGIxZC0xMWVlLTljNGMtMDAxNjNlMTI1MGI1IiwiZXhwIjoxNzAxNDE4NzU5LCJpYXQiOjE3MDE0MTE1NTksImlzcyI6IkhpZ3Jlc3MtR2F0ZXdheSIsImp0aSI6IjQ0YTMzYjc4LWNmYWItNGYzYS1iZDQ3LTQ1Y2Y5ZjM0YjVmZSIsInN1YiI6ImNvbnN1bWVyMSJ9.EIDCTVx4Wt6u5fRngFwgRo-qfDSKp6sUg4fKA7MYpuE"},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 200,
					},
					ExpectedResponseNoRequest: true,
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 3: with some ruleset under this route, oauth checks token and route rule. token verify failed, client not in the route's allowset",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host: "foo1.com",
						Path: "/foo",
						// consumer2
						Headers: map[string]string{"Authorization": "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6ImFwcGxpY2F0aW9uL2F0K2p3dCJ9.eyJhdWQiOiJkZWZhdWx0IiwiY2xpZW50X2lkIjoiODUyMWI1NjQtMGIxZC0xMWVlLTljNGMtMDAxNjNlMTI1MGI1IiwiZXhwIjoxNzAxNDIzOTU1LCJpYXQiOjE3MDE0MTY3NTUsImlzcyI6IkhpZ3Jlc3MtR2F0ZXdheSIsImp0aSI6IjU1NDVkZDRhLWU4YjYtNDY2NC04ZDE4LWY3Yjk5YWVmYzQ1YyIsInN1YiI6ImNvbnN1bWVyMiJ9.FhxLbFFW0h3O3S8MH3vjFRj54xSmQIVVEC8IxGNpIcU"},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 403,
					},
				},
			},
		}

		t.Run("WasmPlugins oauth global auth", func(t *testing.T) {
			for _, testcase := range testcases {
				http.MakeRequestAndExpectEventuallyConsistentResponse(t, suite.RoundTripper, suite.TimeoutConfig, suite.GatewayAddress, testcase)
			}
		})
	},
}
