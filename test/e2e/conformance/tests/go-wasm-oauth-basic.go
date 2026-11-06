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
	Register(WasmPluginsOAuthBasic)

}

var WasmPluginsOAuthBasic = suite.ConformanceTest{
	ShortName:   "WasmPluginsOAuthBasic",
	Description: "The Ingress in the higress-conformance-infra namespace test the oauth WASM plugin.",
	Manifests:   []string{"tests/go-wasm-oauth-basic.yaml"},
	Features:    []suite.SupportedFeature{suite.WASMGoConformanceFeature},
	Test: func(t *testing.T, suite *suite.ConformanceTestSuite) {
		testcases := []http.Assertion{
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "scene 1: generate token, case 1: GET, path lacks <?>",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host: "foo.com",
						Path: "/foo/oauth2/token",
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 400,
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "scene 1: generate token, case 2: GET, path lacks grant_type",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host: "foo.com",
						Path: "/foo/oauth2/token?",
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 400,
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "scene 1: generate token, case 3: GET, path lacks client_id",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host: "foo.com",
						Path: "/foo/oauth2/token?grant_type=client_credentials",
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 400,
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "scene 1: generate token, case 4: GET, path lacks client_secret",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host: "foo.com",
						Path: "/foo/oauth2/token?grant_type=client_credentials&client_id=9515b564-0b1d-11ee-9c4c-00163e1250b5",
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 400,
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "scene 1: generate token, case 5: GET, consumer_id not found in configs",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host: "foo.com",
						Path: "/foo/oauth2/token?grant_type=client_credentials&client_id=c05&client_secret=xxxxx",
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 400,
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "scene 1: generate token, case 6: GET, Failed token service with consumerid and secret not matched",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host: "foo.com",
						Path: "/foo/oauth2/token?grant_type=client_credentials&client_id=9515b564-0b1d-11ee-9c4c-00163e1250b5&client_secret=c01xxxx",
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 400,
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "scene 1: generate token, case 7: success by GET method (consumer1)",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host: "foo.com",
						Path: "/foo/oauth2/token?grant_type=client_credentials&client_id=9515b564-0b1d-11ee-9c4c-00163e1250b5&client_secret=9e55de56-0b1d-11ee-b8ec-00163e1250b5",
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 200,
					},
					// TODO: CompareRequest不支持200且请求未到达backend的情况
					ExpectedResponseNoRequest: true,
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "scene 1: generate token, case 8: success by GET method (consumer2)",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host: "foo.com",
						Path: "/foo/oauth2/token?grant_type=client_credentials&client_id=8521b564-0b1d-11ee-9c4c-00163e1250b5&client_secret=8520b564-0b1d-11ee-9c4c-00163e1250b5",
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
					TestCaseName:    "scene 1: generate token, case 9:POST, body lacks grant_type",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "foo.com",
						Path:        "/foo/oauth2/token",
						Method:      "POST",
						ContentType: http.ContentTypeFormUrlencoded,
						Body:        []byte(`client_id=8521b564-0b1d-11ee-9c4c-00163e1250b5&client_secret=8520b564-0b1d-11ee-9c4c-00163e1250b5`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 400,
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "scene 1: generate token, case 10: POST, body lacks client_id",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "foo.com",
						Path:        "/foo/oauth2/token",
						Method:      "POST",
						ContentType: http.ContentTypeFormUrlencoded,
						Body:        []byte(`grant_type=client_credentials&client_secret=8520b564-0b1d-11ee-9c4c-00163e1250b5`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 400,
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "scene 1: generate token, case 11: POST, body lacks client_secret",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "foo.com",
						Path:        "/foo/oauth2/token",
						Method:      "POST",
						ContentType: http.ContentTypeFormUrlencoded,
						Body:        []byte(`grant_type=client_credentials&client_id=8521b564-0b1d-11ee-9c4c-00163e1250b5`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 400,
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "scene 1: generate token, case 12: POST, consumer_id not found in configs",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "foo.com",
						Path:        "/foo/oauth2/token",
						Method:      "POST",
						ContentType: http.ContentTypeFormUrlencoded,
						Body:        []byte(`grant_type=client_credentials&client_id=c05&client_secret=xxxxx`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 400,
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "scene 1: generate token, case 13: POST, Failed token service with consumerid and secret not matched",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "foo.com",
						Path:        "/foo/oauth2/token",
						Method:      "POST",
						ContentType: http.ContentTypeFormUrlencoded,
						Body:        []byte(`grant_type=client_credentials&client_id=9515b564-0b1d-11ee-9c4c-00163e1250b5&client_secret=c01xxxx`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 400,
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "scene 1: generate token, case 14: success by POST method, consumser info in request body (consumer2)",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "foo.com",
						Path:        "/foo/oauth2/token",
						Method:      "POST",
						ContentType: http.ContentTypeFormUrlencoded,
						Body:        []byte(`grant_type=client_credentials&client_id=8521b564-0b1d-11ee-9c4c-00163e1250b5&client_secret=8520b564-0b1d-11ee-9c4c-00163e1250b5`),
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
					TestCaseName:    "scene 2: invalid token, case 1: not a bearer token",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:    "foo.com",
						Path:    "/foo",
						Headers: map[string]string{"Authorization": "alksdjf"},
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
					TestCaseName:    "scene 2: invalid token, case 2: token not fit jwt's format",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:    "foo.com",
						Path:    "/foo",
						Headers: map[string]string{"Authorization": "Bearer alksdjf"},
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
					TestCaseName:    "scene 2: invalid token, case 3: invalid token",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:    "foo.com",
						Path:    "/foo",
						Headers: map[string]string{"Authorization": "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6ImFwcGxpY2F0aW9uL2F0K2p3dCJ9.eyJhdWQiOiJkZWZhdWx0IiwiY2xpZW50X2lkIjoiOTUxNTVhNjQtMGIxZC0xcWVlLTljNGMtMDAxcXdlMTI1MGI1IiwiZXhwIjoxNzAxMzYzODc0LCJpYXQiOjE3MDEzNTY2NzQsImlzcyI6IkhpZ3Jlc3MtR2F0ZXdheSIsImp0aSI6IjYyM2UyMmQ5LTc1MTctNGEwOC04ZDc2LTliZjBlNDljYjEyYyIsInN1YiI6IiJ9.IB2-T_v9aHRfOyd_QQcNIMtdjA8q5pHfCeixMi5-b0E"},
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
					TestCaseName:    "scene 4: AuthZ, case 1: token verify fail, client not in the route's allowset",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host: "foo.com",
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
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "scene 4: AuthZ, case 2: token verify success, client in the route's allowset",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host: "foo.com",
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
		}

		t.Run("WasmPlugins oauth basic", func(t *testing.T) {
			for _, testcase := range testcases {
				http.MakeRequestAndExpectEventuallyConsistentResponse(t, suite.RoundTripper, suite.TimeoutConfig, suite.GatewayAddress, testcase)
			}
		})
	},
}
