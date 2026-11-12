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
	Register(WasmPluginsOAuthEmptyConsumer)
	
}

var WasmPluginsOAuthEmptyConsumer = suite.ConformanceTest{
	ShortName:   "WasmPluginsOAuthEmptyConsumer",
	Description: "The Ingress in the higress-conformance-infra namespace test the oauth WASM plugin, with empty consumer config",
	Manifests:   []string{"tests/go-wasm-oauth-empty-consumer.yaml"},
	Features:    []suite.SupportedFeature{suite.WASMGoConformanceFeature},
	Test: func(t *testing.T, suite *suite.ConformanceTestSuite) {
		testcases := []http.Assertion{
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 1: empty consumer",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host: "foo.com",
						Path: "/foo",
						Headers: map[string]string{"Authorization": "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6ImFwcGxpY2F0aW9uL2F0K2p3dCJ9.eyJhdWQiOiJkZWZhdWx0IiwiY2xpZW50X2lkIjoiOTUxNTVhNjQtMGIxZC0xcWVlLTljNGMtMDAxcXdlMTI1MGI1IiwiZXhwIjoxNzAxMzYzODc0LCJpYXQiOjE3MDEzNTY2NzQsImlzcyI6IkhpZ3Jlc3MtR2F0ZXdheSIsImp0aSI6IjYyM2UyMmQ5LTc1MTctNGEwOC04ZDc2LTliZjBlNDljYjEyYyIsInN1YiI6IiJ9.IB2-T_v9aHRfOyd_QQcNIMtdjA8q5pHfCeixMi5-b0E"},
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

		t.Run("WasmPlugins oauth empty consumer", func(t *testing.T) {
			for _, testcase := range testcases {
				http.MakeRequestAndExpectEventuallyConsistentResponse(t, suite.RoundTripper, suite.TimeoutConfig, suite.GatewayAddress, testcase)
			}
		})
	},
}
