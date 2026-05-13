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

	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/http"
	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/suite"
)

func init() {
	Register(WasmPluginsVMCrash)
}

// WasmPluginsVMCrash verifies that WasmPlugin failStrategy honors both
// FAIL_OPEN (request bypasses the crashed VM and reaches the backend) and
// FAIL_CLOSE (gateway returns 503 wasm_fail_stream). See higress issue
// #3814 — failStrategy=FAIL_OPEN was being silently dropped by istio's
// buildPluginConfig, causing 503 responses on wasm VM crashes.
var WasmPluginsVMCrash = suite.ConformanceTest{
	ShortName:   "WasmPluginsVMCrash",
	Description: "WasmPlugin failStrategy must apply when the wasm VM crashes during request_headers.",
	Manifests:   []string{"tests/go-wasm-vm-crash.yaml"},
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
						Host:             "fail-open.foo.com",
						Path:             "/",
						UnfollowRedirect: true,
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
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:             "fail-close.foo.com",
						Path:             "/",
						UnfollowRedirect: true,
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 503,
					},
				},
			},
		}
		t.Run("WasmPlugins vm-crash failStrategy", func(t *testing.T) {
			for _, testcase := range testcases {
				http.MakeRequestAndExpectEventuallyConsistentResponse(t, suite.RoundTripper, suite.TimeoutConfig, suite.GatewayAddress, testcase)
			}
		})
	},
}
