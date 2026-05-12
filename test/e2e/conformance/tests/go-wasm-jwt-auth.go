// Copyright (c) 2024 Alibaba Group Holding Ltd.
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

const (
	ES256Allow   string = "eyJhbGciOiJFUzI1NiIsImtpZCI6InAyNTYiLCJ0eXAiOiJKV1QifQ.eyJhdWQiOlsiZm9vIiwiYmFyIl0sImV4cCI6MjAxOTY4NjQwMCwiaXNzIjoiaGlncmVzcy10ZXN0IiwibmJmIjoxNzA0MDY3MjAwLCJzdWIiOiJoaWdyZXNzLXRlc3QifQ.hm71YWfjALshUAgyOu-r9W2WBG_zfqIZZacAbc7oIH1r7dbB0sGQn3wKMWMmOzmxX0UyaVZ0KMk-HFTA1hDnBQ"
	ES256Expried string = "eyJhbGciOiJFUzI1NiIsImtpZCI6InAyNTYiLCJ0eXAiOiJKV1QifQ.eyJhdWQiOlsiZm9vIiwiYmFyIl0sImV4cCI6MTcwNDA2NzIwMCwiaXNzIjoiaGlncmVzcy10ZXN0IiwibmJmIjoxNzA0MDY3MjAwLCJzdWIiOiJoaWdyZXNzLXRlc3QifQ.9AnXd2rZ6FirHZQAoabyL4xZNz0jr-3LmcV4-pFV3JrdtUT4386Mw5Qan125fUB-rZf_ZBlv0Bft2tWY149fyg"
	RS256Allow   string = "eyJhbGciOiJSUzI1NiIsImtpZCI6InJzYSIsInR5cCI6IkpXVCJ9.eyJhdWQiOlsiZm9vIiwiYmFyIl0sImV4cCI6MjAxOTY4NjQwMCwiaXNzIjoiaGlncmVzcy10ZXN0IiwibmJmIjoxNzA0MDY3MjAwLCJzdWIiOiJoaWdyZXNzLXRlc3QifQ.iO0wPY91b_VNGUMZ1n-Ub-SRmEkDQMFLSi77z49tEzll3UZXwmBraP5udM_OPUAdk9ZO3dbb_fOgdcN9V1H9p5kiTr-l-pZTFTJHrPJj8wC519sYRcCk3wrZ9aXR5tNMwOsMdQb7waTBatDQLmHPWzAoTNBc8mwXkRcv1dmJLvsJgxyCl1I9CMOMPq0fYj1NBvaUDIdVSL1o7GGiriD8-0UIOmS72-I3mbaoCIyVb0h3wx7gnIW3zr0yYWaYoiIgmHLag-eEGxHp4-BjtCqcokU4QVMS91qpH7Mkl1iv2WHEkuDQRJ-nLzYGwXb7Dncx9K5tNWHJuZ-DihIU2oT0aA"
	RS256Expried string = "eyJhbGciOiJSUzI1NiIsImtpZCI6InJzYSIsInR5cCI6IkpXVCJ9.eyJhdWQiOlsiZm9vIiwiYmFyIl0sImV4cCI6MTcwNDA2NzIwMCwiaXNzIjoiaGlncmVzcy10ZXN0IiwibmJmIjoxNzA0MDY3MjAwLCJzdWIiOiJoaWdyZXNzLXRlc3QifQ.jqzlhBPk9mmvtTT5aCYf-_5uXXSEU5bQ32fx78XeboCnjR9K1CsI4KYUIkXEX3bk66XJQUeSes7lz3gA4Yzkd-v9oADHTgpKnIxzv_5mD0_afIwEFjcalqVbSvCmro4PessQZDnmU7AIzoo3RPSqbmq8xbPVYUH9I-OO8aUu2ATd1HozgxJH1XnRU8k9KMkVW8XhvJXLKZJmnqe3Tu6pCU_tawFlBfBC4fAhMf0yX2CGE0ABAHubcdiI6JXObQmQQ9Or2a-g2a8g_Bw697PoPOsAn0YpTrHst9GcyTpkbNTAq9X8fc5EM7hiDM1FGeMYcaQTdMnOh4HBhP0p4YEhvA"
)

func init() {
	Register(WasmPluginsJWTAuthAllow)
	Register(WasmPluginsJWTAuthExpried)
	Register(WasmPluginsJWTAuthDeny)
	Register(WasmPluginsJWTAuthSingleConsumer)
}

var WasmPluginsJWTAuthAllow = suite.ConformanceTest{
	ShortName:   "WasmPluginsJWTAuth",
	Description: "The Ingress in the higress-conformance-infra namespace test the jwt-auth WASM plugin.",
	Manifests:   []string{"tests/go-wasm-jwt-auth-allow.yaml"},
	Features:    []suite.SupportedFeature{suite.WASMGoConformanceFeature},
	Test: func(t *testing.T, suite *suite.ConformanceTestSuite) {
		testcases := []http.Assertion{
			{
				Meta: http.AssertionMeta{
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
					TestCaseName:    "1. Default header with ES256",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:             "foo.com",
						Path:             "/info",
						UnfollowRedirect: true,
						Headers:          map[string]string{"Authorization": "Bearer " + ES256Allow},
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
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
					TestCaseName:    "2. Default header with RS256",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:             "foo.com",
						Path:             "/info",
						UnfollowRedirect: true,
						Headers:          map[string]string{"Authorization": "Bearer " + RS256Allow},
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
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
					TestCaseName:    "3. Default params with ES256",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:             "foo.com",
						Path:             "/info?access_token=" + ES256Allow,
						UnfollowRedirect: true,
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
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
					TestCaseName:    "4. Default params with RS256",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:             "foo.com",
						Path:             "/info?access_token=" + RS256Allow,
						UnfollowRedirect: true,
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
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
					TestCaseName:    "5. Custom header with ES256",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:             "foo.com",
						Path:             "/info",
						UnfollowRedirect: true,
						Headers:          map[string]string{"jwt": "Bearer " + ES256Allow},
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
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
					TestCaseName:    "6. Custom header with RS256",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:             "foo.com",
						Path:             "/info",
						UnfollowRedirect: true,
						Headers:          map[string]string{"jwt": "Bearer " + RS256Allow},
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
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
					TestCaseName:    "7. Custom params with ES256",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:             "foo.com",
						Path:             "/info?jwt_token=" + ES256Allow,
						UnfollowRedirect: true,
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
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
					TestCaseName:    "8. Custom params with RS256",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:             "foo.com",
						Path:             "/info?jwt_token=" + RS256Allow,
						UnfollowRedirect: true,
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
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
					TestCaseName:    "9. Custom cookies with ES256",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:             "foo.com",
						Path:             "/info",
						Headers:          map[string]string{"Cookie": "jwt_token=" + ES256Allow},
						UnfollowRedirect: true,
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
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
					TestCaseName:    "10. Custom cookies with RS256",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:             "foo.com",
						Path:             "/info",
						Headers:          map[string]string{"Cookie": "jwt_token=" + RS256Allow},
						UnfollowRedirect: true,
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
		t.Run("WasmPlugins jwt-auth", func(t *testing.T) {
			for _, testcase := range testcases {
				http.MakeRequestAndExpectEventuallyConsistentResponse(t, suite.RoundTripper, suite.TimeoutConfig, suite.GatewayAddress, testcase)
			}
		})
	},
}

var WasmPluginsJWTAuthExpried = suite.ConformanceTest{
	ShortName:   "WasmPluginsJWTAuthExpried",
	Description: "The Ingress in the higress-conformance-infra namespace test the jwt-auth WASM plugin.",
	Manifests:   []string{"tests/go-wasm-jwt-auth-deny.yaml"},
	Features:    []suite.SupportedFeature{suite.WASMGoConformanceFeature},
	Test: func(t *testing.T, suite *suite.ConformanceTestSuite) {
		testcases := []http.Assertion{
			{
				Meta: http.AssertionMeta{
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
					TestCaseName:    "1. Default header with expired ES256",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:             "foo.com",
						Path:             "/info",
						UnfollowRedirect: true,
						Headers:          map[string]string{"Authorization": "Bearer " + ES256Expried},
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
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
					TestCaseName:    "2. Default header with expired RS256",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:             "foo.com",
						Path:             "/info",
						UnfollowRedirect: true,
						Headers:          map[string]string{"Authorization": "Bearer " + RS256Expried},
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
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
					TestCaseName:    "3. Default params with expired ES256",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:             "foo.com",
						Path:             "/info?access_token=" + ES256Expried,
						UnfollowRedirect: true,
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
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
					TestCaseName:    "4. Default params with expired RS256",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:             "foo.com",
						Path:             "/info?access_token=" + RS256Expried,
						UnfollowRedirect: true,
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
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
					TestCaseName:    "5. Custom header with expired ES256",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:             "foo.com",
						Path:             "/info",
						UnfollowRedirect: true,
						Headers:          map[string]string{"jwt": "Bearer " + ES256Expried},
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
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
					TestCaseName:    "6. Custom header with expired RS256",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:             "foo.com",
						Path:             "/info",
						UnfollowRedirect: true,
						Headers:          map[string]string{"jwt": "Bearer " + RS256Expried},
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
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
					TestCaseName:    "7. Custom params with expired ES256",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:             "foo.com",
						Path:             "/info?jwt_token=" + ES256Expried,
						UnfollowRedirect: true,
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
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
					TestCaseName:    "8. Custom params with expired RS256",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:             "foo.com",
						Path:             "/info?jwt_token=" + RS256Expried,
						UnfollowRedirect: true,
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
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
					TestCaseName:    "9. Custom cookies with expired ES256",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:             "foo.com",
						Path:             "/info",
						Headers:          map[string]string{"Cookie": "jwt_token=" + ES256Expried},
						UnfollowRedirect: true,
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
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
					TestCaseName:    "10. Custom cookies with expired RS256",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:             "foo.com",
						Path:             "/info",
						Headers:          map[string]string{"Cookie": "jwt_token=" + RS256Expried},
						UnfollowRedirect: true,
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 403,
					},
					ExpectedResponseNoRequest: true,
				},
			},
		}
		t.Run("WasmPlugins jwt-auth", func(t *testing.T) {
			for _, testcase := range testcases {
				http.MakeRequestAndExpectEventuallyConsistentResponse(t, suite.RoundTripper, suite.TimeoutConfig, suite.GatewayAddress, testcase)
			}
		})
	},
}

var WasmPluginsJWTAuthDeny = suite.ConformanceTest{
	ShortName:   "WasmPluginsJWTAuthDeny",
	Description: "The Ingress in the higress-conformance-infra namespace test the jwt-auth WASM plugin.",
	Manifests:   []string{"tests/go-wasm-jwt-auth-deny.yaml"},
	Features:    []suite.SupportedFeature{suite.WASMGoConformanceFeature},
	Test: func(t *testing.T, suite *suite.ConformanceTestSuite) {
		testcases := []http.Assertion{
			{
				Meta: http.AssertionMeta{
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
					TestCaseName:    "1. Default header with RS256 but unauthorized consumer",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:             "foo.com",
						Path:             "/info",
						UnfollowRedirect: true,
						Headers:          map[string]string{"Authorization": "Bearer " + RS256Allow},
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
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
					TestCaseName:    "2. No token",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:             "foo.com",
						Path:             "/info",
						UnfollowRedirect: true,
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
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
					TestCaseName:    "3. Default header with no token",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:             "foo.com",
						Path:             "/info",
						UnfollowRedirect: true,
						Headers:          map[string]string{"Authorization": "Bearer " + ""},
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
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
					TestCaseName:    "4. Default params with no token",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:             "foo.com",
						Path:             "/info?access_token=" + "",
						UnfollowRedirect: true,
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
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
					TestCaseName:    "5. Custom header with no token",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:             "foo.com",
						Path:             "/info",
						UnfollowRedirect: true,
						Headers:          map[string]string{"jwt": "Bearer " + ""},
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
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
					TestCaseName:    "6. Custom params with no token",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:             "foo.com",
						Path:             "/info?jwt_token=" + "",
						UnfollowRedirect: true,
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
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
					TestCaseName:    "7. Custom cookies with no token",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:             "foo.com",
						Path:             "/info",
						Headers:          map[string]string{"Cookie": "jwt_token=" + ""},
						UnfollowRedirect: true,
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
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
					TestCaseName:    "8. Default header with fake token",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:             "foo.com",
						Path:             "/info",
						UnfollowRedirect: true,
						Headers:          map[string]string{"Authorization": "Bearer " + "faketoken"},
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
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
					TestCaseName:    "9. Default params with fake token",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:             "foo.com",
						Path:             "/info?access_token=" + "faketoken",
						UnfollowRedirect: true,
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
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
					TestCaseName:    "10. Custom header with fake token",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:             "foo.com",
						Path:             "/info",
						UnfollowRedirect: true,
						Headers:          map[string]string{"jwt": "Bearer " + "faketoken"},
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
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
					TestCaseName:    "11. Custom params with fake token",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:             "foo.com",
						Path:             "/info?jwt_token=" + "faketoken",
						UnfollowRedirect: true,
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
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
					TestCaseName:    "12. Custom cookies with fake token",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:             "foo.com",
						Path:             "/info",
						Headers:          map[string]string{"Cookie": "jwt_token=" + "faketoken"},
						UnfollowRedirect: true,
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 403,
					},
					ExpectedResponseNoRequest: true,
				},
			},
		}
		t.Run("WasmPlugins jwt-auth", func(t *testing.T) {
			for _, testcase := range testcases {
				http.MakeRequestAndExpectEventuallyConsistentResponse(t, suite.RoundTripper, suite.TimeoutConfig, suite.GatewayAddress, testcase)
			}
		})
	},
}

var WasmPluginsJWTAuthSingleConsumer = suite.ConformanceTest{
	ShortName:   "WasmPluginsJWTAuthSingleConsumer",
	Description: "The Ingress in the higress-conformance-infra namespace test the jwt-auth WASM plugin.",
	Manifests:   []string{"tests/go-wasm-jwt-auth-single-consumer.yaml"},
	Features:    []suite.SupportedFeature{suite.WASMGoConformanceFeature},
	Test: func(t *testing.T, suite *suite.ConformanceTestSuite) {
		testcases := []http.Assertion{
			{
				Meta: http.AssertionMeta{
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
					TestCaseName:    "1. Default hedaer with ES256 by single consumer_EC",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:             "foo.com",
						Path:             "/info",
						UnfollowRedirect: true,
						Headers:          map[string]string{"Authorization": "Bearer " + ES256Allow},
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
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
					TestCaseName:    "2. Default hedaer with expired ES256 by single consumer_EC",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:             "foo.com",
						Path:             "/info",
						UnfollowRedirect: true,
						Headers:          map[string]string{"Authorization": "Bearer " + ES256Expried},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 401,
					},
					ExpectedResponseNoRequest: true,
				},
			},
			{
				Meta: http.AssertionMeta{
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
					TestCaseName:    "3. Default hedaer with fake token by single consumer_EC",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:             "foo.com",
						Path:             "/info",
						UnfollowRedirect: true,
						Headers:          map[string]string{"Authorization": "Bearer " + "faketoken"},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 401,
					},
					ExpectedResponseNoRequest: true,
				},
			},
			{
				Meta: http.AssertionMeta{
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
					TestCaseName:    "4. No token by single consumer_EC",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:             "foo.com",
						Path:             "/info",
						UnfollowRedirect: true,
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 401,
					},
					ExpectedResponseNoRequest: true,
				},
			},
			{
				Meta: http.AssertionMeta{
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
					TestCaseName:    "5. Default header with RS256 by single consumer_EC",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:             "foo.com",
						Path:             "/info",
						UnfollowRedirect: true,
						Headers:          map[string]string{"Authorization": "Bearer " + RS256Allow},
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
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
					TestCaseName:    "6. Default params with ES256 by single consumer_EC",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:             "foo.com",
						Path:             "/info?access_token=" + ES256Allow,
						UnfollowRedirect: true,
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
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
					TestCaseName:    "7. Default params with expired ES256 by single consumer_EC",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:             "foo.com",
						Path:             "/info?access_token=" + ES256Expried,
						UnfollowRedirect: true,
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 401,
					},
					ExpectedResponseNoRequest: true,
				},
			},
			{
				Meta: http.AssertionMeta{
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
					TestCaseName:    "8. Default params with fake token by single consumer_EC",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:             "foo.com",
						Path:             "/info?access_token=" + "faketoken",
						UnfollowRedirect: true,
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 401,
					},
					ExpectedResponseNoRequest: true,
				},
			},

			{
				Meta: http.AssertionMeta{
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
					TestCaseName:    "9. Default params with RS256 by single consumer_EC",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:             "foo.com",
						Path:             "/info?access_token=" + RS256Allow,
						UnfollowRedirect: true,
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 403,
					},
					ExpectedResponseNoRequest: true,
				},
			},
		}
		t.Run("WasmPlugins jwt-auth", func(t *testing.T) {
			for _, testcase := range testcases {
				http.MakeRequestAndExpectEventuallyConsistentResponse(t, suite.RoundTripper, suite.TimeoutConfig, suite.GatewayAddress, testcase)
			}
		})
	},
}
