// Copyright (c) 2022 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
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
	Register(WasmPluginsAiLoadBalancer)
}

// WasmPluginsAiLoadBalancer verifies that cluster_metrics writes the selected
// cluster to x-higress-target-cluster and that Envoy routes to it.
var WasmPluginsAiLoadBalancer = suite.ConformanceTest{
	ShortName:   "WasmPluginsAiLoadBalancer",
	Description: "The ai-load-balancer cluster_metrics WASM plugin routes to the cluster selected by its cluster header.",
	Features:    []suite.SupportedFeature{suite.WASMGoConformanceFeature},
	Manifests:   []string{"tests/go-wasm-ai-load-balancer.yaml"},
	Test: func(t *testing.T, suite *suite.ConformanceTestSuite) {
		testcase := http.Assertion{
			Meta: http.AssertionMeta{
				TestCaseName:    "cluster_metrics routes to the selected cluster",
				TargetBackend:   "infra-backend-v2",
				TargetNamespace: "higress-conformance-infra",
			},
			Request: http.AssertionRequest{
				ActualRequest: http.Request{
					Host: "ai-load-balancer.example.com",
					Path: "/get",
				},
				ExpectedRequest: &http.ExpectedRequest{
					Request: http.Request{
						Host: "ai-load-balancer.example.com",
						Path: "/get",
					},
				},
			},
			Response: http.AssertionResponse{
				ExpectedResponse: http.Response{StatusCode: 200},
			},
		}

		t.Run("WasmPlugins ai-load-balancer", func(t *testing.T) {
			http.MakeRequestAndExpectEventuallyConsistentResponse(t, suite.RoundTripper, suite.TimeoutConfig, suite.GatewayAddress, testcase)
		})
	},
}
