// Copyright (c) 2023 Alibaba Group Holding Ltd.
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
	Register(WasmPluginsCSRF)
}

var WasmPluginsCSRF = suite.ConformanceTest{
	ShortName:   "WasmPluginsCSRF",
	Description: "The Ingress in the higress-conformance-infra namespace test the csrf WASM plugin.",
	Manifests:   []string{"tests/go-wasm-csrf.yaml"},
	Features:    []suite.SupportedFeature{suite.WASMGoConformanceFeature},
	Test: func(t *testing.T, suite *suite.ConformanceTestSuite) {
		testcases := []http.Assertion{
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 1: Successful authentication",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:    "foo.com",
						Path:    "/foo",
						Headers: map[string]string{"cookie": "higress-csrf-token=eyJyYW5kb20iOiIwLjY1NDA1MjkxOTUxNzIwNDIiLCJleHBpcmVzIjozNjAwLCJzaWduIjoiXHVmZmZkXHVmZmZkXHUwMDFmXHVmZmZkMnVcdWZmZmQ/XHVmZmZkXHVmZmZkZ2ZpVFVcdTAwM2NcdWZmZmRcdWZmZmTiiqtcdWZmZmRcdWZmZmRcdWZmZmQrdVx1ZmZmZFxcbFx1ZmZmZFx1ZmZmZFx1ZmZmZCJ9", "higress-csrf-token": "eyJyYW5kb20iOiIwLjY1NDA1MjkxOTUxNzIwNDIiLCJleHBpcmVzIjozNjAwLCJzaWduIjoiXHVmZmZkXHVmZmZkXHUwMDFmXHVmZmZkMnVcdWZmZmQ/XHVmZmZkXHVmZmZkZ2ZpVFVcdTAwM2NcdWZmZmRcdWZmZmTiiqtcdWZmZmRcdWZmZmRcdWZmZmQrdVx1ZmZmZFxcbFx1ZmZmZFx1ZmZmZFx1ZmZmZCJ9"},
					},
					ExpectedRequest: &http.ExpectedRequest{
						Request: http.Request{
							Host:    "foo.com",
							Path:    "/foo",
							Headers: map[string]string{"X-Mse-Consumer": "consumer1"},
						},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 200,
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 2: No header csrf token information found",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:    "foo.com",
						Path:    "/foo",
						Headers: map[string]string{"cookie": "higress-csrf-token=eyJyYW5kb20iOiIwLjY1NDA1MjkxOTUxNzIwNDIiLCJleHBpcmVzIjozNjAwLCJzaWduIjoiXHVmZmZkXHVmZmZkXHUwMDFmXHVmZmZkMnVcdWZmZmQ/XHVmZmZkXHVmZmZkZ2ZpVFVcdTAwM2NcdWZmZmRcdWZmZmTiiqtcdWZmZmRcdWZmZmRcdWZmZmQrdVx1ZmZmZFxcbFx1ZmZmZFx1ZmZmZFx1ZmZmZCJ9"},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 401,
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 3: No cookie higress csrf token information found",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:    "foo.com",
						Path:    "/foo",
						Headers: map[string]string{"higress-csrf-token": "eyJyYW5kb20iOiIwLjY1NDA1MjkxOTUxNzIwNDIiLCJleHBpcmVzIjozNjAwLCJzaWduIjoiXHVmZmZkXHVmZmZkXHUwMDFmXHVmZmZkMnVcdWZmZmQ/XHVmZmZkXHVmZmZkZ2ZpVFVcdTAwM2NcdWZmZmRcdWZmZmTiiqtcdWZmZmRcdWZmZmRcdWZmZmQrdVx1ZmZmZFxcbFx1ZmZmZFx1ZmZmZFx1ZmZmZCJ9"},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 401,
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 4: cookie higress csrf token  not equal header csrf token information",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:    "foo.com",
						Path:    "/foo",
						Headers: map[string]string{"higress-csrf-token": "eyJyYW5k", "cookie": "higress-csrf-token=kb20iO"},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 401,
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 5: cookie higress csrf token/header csrf token decoding error",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:    "foo.com",
						Path:    "/foo",
						Headers: map[string]string{"higress-csrf-token": "aaa", "cookie": "bbb"},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 401,
					},
				},
			},
		}
		t.Run("WasmPlugins csrf", func(t *testing.T) {
			for _, testcase := range testcases {
				http.MakeRequestAndExpectEventuallyConsistentResponse(t, suite.RoundTripper, suite.TimeoutConfig, suite.GatewayAddress, testcase)
			}
		})
	},
}
