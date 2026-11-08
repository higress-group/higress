// Copyright (c) 2025 Alibaba Group Holding Ltd.
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
	"crypto/rand"
	"encoding/base64"
	"net/url"
	"testing"

	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/http"
	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/roundtripper"
	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/suite"
)

func init() {
	Register(WasmPluginsReplayProtection)
}

func generateBase64Nonce(length int) string {
	bytes := make([]byte, length)
	rand.Read(bytes)
	return base64.StdEncoding.EncodeToString(bytes)
}

var WasmPluginsReplayProtection = suite.ConformanceTest{
	ShortName:   "WasmPluginsReplayProtection",
	Description: "The replay protection wasm plugin prevents replay attacks by validating request nonce.",
	Manifests:   []string{"tests/go-wasm-replay-protection.yaml"},
	Features:    []suite.SupportedFeature{suite.WASMGoConformanceFeature},
	Test: func(t *testing.T, suite *suite.ConformanceTestSuite) {
		replayNonce := generateBase64Nonce(32)
		testcases := []http.Assertion{
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "Missing nonce header",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:   "foo.com",
						Path:   "/",
						Method: "GET",
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  400,
						ContentType: http.ContentTypeTextPlain,
						Body:        []byte(`Missing Required Header`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "Invalid nonce not base64 encoded",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:   "foo.com",
						Path:   "/",
						Method: "GET",
						Headers: map[string]string{
							"X-Higress-Nonce": "invalid nonce",
						},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  400,
						ContentType: http.ContentTypeTextPlain,
						Body:        []byte(`Invalid Nonce`),
					},
				},
			},
			{
				Meta: http.AssertionMeta{
					TestCaseName:  "First request with unique nonce returns 200",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:   "foo.com",
						Path:   "/",
						Method: "GET",
						Headers: map[string]string{
							"X-Higress-Nonce": replayNonce,
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
					TestCaseName:  "Second request with repeated nonce returns 429",
					CompareTarget: http.CompareTargetResponse,
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:   "foo.com",
						Path:   "/",
						Method: "GET",
						Headers: map[string]string{
							"X-Higress-Nonce": replayNonce,
						},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  429,
						ContentType: http.ContentTypeTextPlain,
						Body:        []byte(`Replay Attack Detected`),
					},
				},
			},
		}

		t.Run("WasmPlugins replay-protection", func(t *testing.T) {
			for i, testcase := range testcases {
				if i == 2 {
					// A nonce returns 200 only on its first request; later retries are duplicates.
					request := roundtripper.Request{
						Method:   "GET",
						Host:     "foo.com",
						URL:      url.URL{Scheme: "http", Host: suite.GatewayAddress, Path: "/"},
						Protocol: "HTTP/1.1",
						Headers: map[string][]string{
							"X-Higress-Nonce": {replayNonce},
						},
					}
					http.WaitForConsistentResponse(t, suite.RoundTripper, request, testcase, 1, suite.TimeoutConfig.MaxTimeToConsistency)
					continue
				}
				http.MakeRequestAndExpectEventuallyConsistentResponse(t, suite.RoundTripper, suite.TimeoutConfig, suite.GatewayAddress, testcase)
			}
		})
	},
}
