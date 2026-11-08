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
	"net/url"
	"testing"

	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/http"
	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/suite"
)

func init() {
	Register(WasmPluginsGeoIPPlugin)
}

var WasmPluginsGeoIPPlugin = suite.ConformanceTest{
	ShortName:   "WasmPluginsGeoIPPlugin",
	Description: "The geo-ip wasm plugin finds the client's geographic information according to the client's ip address.",
	Manifests:   []string{"tests/go-wasm-geo-ip.yaml"},
	Features:    []suite.SupportedFeature{suite.WASMGoConformanceFeature},
	Test: func(t *testing.T, suite *suite.ConformanceTestSuite) {
		testcases := []http.Assertion{
			{
				Meta: http.AssertionMeta{
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:             "foo.com",
						Path:             "/get",
						UnfollowRedirect: true,
						Headers: map[string]string{
							"X-Forwarded-For": "70.155.208.224,10.1.1.1",
						},
					},
					ExpectedRequest: &http.ExpectedRequest{
						Request: http.Request{
							Path: "/get",
							Host: "foo.com",
							Headers: map[string]string{
								"X-Higress-Geo-Isp":      url.QueryEscape("美国电话电报"),
								"X-Higress-Geo-City":     "",
								"X-Higress-Geo-Province": url.QueryEscape("密西西比"),
								"X-Higress-Geo-Country":  url.QueryEscape("美国"),
							},
						},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 200,
					},
					ExpectedResponseNoRequest: false,
				},
			},
			{
				Meta: http.AssertionMeta{
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:             "foo.com",
						Path:             "/get",
						UnfollowRedirect: true,
						Headers: map[string]string{
							"X-Forwarded-For": "2.2.128.100,10.1.1.2",
						},
					},
					ExpectedRequest: &http.ExpectedRequest{
						Request: http.Request{
							Path: "/get",
							Host: "foo.com",
							Headers: map[string]string{
								"X-Higress-Geo-Isp":      url.QueryEscape("橘子电信"),
								"X-Higress-Geo-City":     "",
								"X-Higress-Geo-Province": url.QueryEscape("Var"),
								"X-Higress-Geo-Country":  url.QueryEscape("法国"),
							},
						},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 200,
					},
					ExpectedResponseNoRequest: false,
				},
			},
		}
		t.Run("WasmPlugins geo-ip", func(t *testing.T) {
			for _, testcase := range testcases {
				http.MakeRequestAndExpectEventuallyConsistentResponse(t, suite.RoundTripper, suite.TimeoutConfig, suite.GatewayAddress, testcase)
			}
		})
	},
}
