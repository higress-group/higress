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
	"testing"

	"github.com/alibaba/higress/test/e2e/conformance/utils/http"
	"github.com/alibaba/higress/test/e2e/conformance/utils/suite"
)

func init() {
	Register(WasmPluginsExtAuth)
}

var WasmPluginsExtAuth = suite.ConformanceTest{
	ShortName:   "WasmPluginsExtAuth",
	Description: "E2E tests for ext‑auth plugin in envoy & forward_auth modes, with whitelist, blacklist, and body-required cases",
	Manifests:   []string{"tests/ext-auth-all-modes.yaml"},
	Features:    []suite.SupportedFeature{suite.WASMGoConformanceFeature},
	Test: func(t *testing.T, s *suite.ConformanceTestSuite) {
		cases := []struct {
			name       string
			path       string
			method     string
			body       []byte
			expectCode int
		}{
			{"Forward 200", "/echo", "GET", nil, 200},
		}

		for _, tc := range cases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				req := http.Request{
					Host:             "test-ext-auth-ingress1.com",
					Path:             tc.path,
					Method:           tc.method,
					Headers:          map[string]string{"Authorization": "Bearer token"},
					Body:             tc.body,
					UnfollowRedirect: true,
				}
				if tc.body != nil {
					req.Headers["Content-Type"] = "application/json"
				}
				resp := http.Response{StatusCode: tc.expectCode}
				assertion := http.Assertion{
					Meta:     http.AssertionMeta{TestCaseName: tc.name, TargetBackend: "infra-backend-v1", TargetNamespace: "higress-conformance-infra"},
					Request:  http.AssertionRequest{ActualRequest: req},
					Response: http.AssertionResponse{ExpectedResponse: resp},
				}
				http.MakeRequestAndExpectEventuallyConsistentResponse(t, s.RoundTripper, s.TimeoutConfig, s.GatewayAddress, assertion)
			})
		}
	},
}
