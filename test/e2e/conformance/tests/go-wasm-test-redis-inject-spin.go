// Copyright (c) 2026 Alibaba Group Holding Ltd.
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
	Register(WasmPluginsRedisInjectSpin)
}

// WasmPluginsRedisInjectSpin reproduces issue #4034: a deferred async Redis
// failure callback synchronously calls injectEncodedDataToFilterChain, which
// (pre-fix) makes WasmBase::doAfterVmCallActions re-queue an action forever and
// spins a worker at 100% CPU. Redis is pointed at an unroutable endpoint so the
// failure callbacks fire under concurrency.
//
// With the Layer A drain-to-local fix in place, the after-vm-call drain
// terminates over a snapshot, deferred callbacks complete, and the gateway keeps
// serving requests. The test asserts the plugin route stays responsive (200)
// rather than hanging — a spinning worker would fail this via timeout.
var WasmPluginsRedisInjectSpin = suite.ConformanceTest{
	ShortName:   "WasmPluginsRedisInjectSpin",
	Description: "Reproduce #4034: redis-failure callback + inject must not spin the worker; gateway stays responsive.",
	Manifests:   []string{"tests/go-wasm-test-redis-inject-spin.yaml"},
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
						Host:             "redis-inject-spin.com",
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
		}
		t.Run("WasmPlugins redis-inject-spin (#4034 CPU spin repro)", func(t *testing.T) {
			// Drive repeated requests: on the pre-fix host the deferred callback
			// re-queues forever and a worker spins, so the route stops
			// responding; on the fixed host every request completes.
			for i := 0; i < 20; i++ {
				for _, testcase := range testcases {
					http.MakeRequestAndExpectEventuallyConsistentResponse(t, suite.RoundTripper, suite.TimeoutConfig, suite.GatewayAddress, testcase)
				}
			}
		})
	},
}
