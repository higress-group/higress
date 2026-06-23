// Copyright (c) 2026 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0

package tests

import (
	"testing"

	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/http"
	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/suite"
)

func init() {
	Register(WasmPluginsClusterKeyRateLimit)
}

// cluster-key uses request count (not token count); each request is +1 to counters.
// Config: global_threshold query_per_minute=3, vip-key rule_item query_per_minute=1.
//
// Lua semantics (MultiKeyFixedWindowScript):
//   - For each rule independently: read counter; if current <= threshold, atomic incr; else no-op.
//   - Lua's `current > threshold` check uses the PRE-incr value; Go's check uses the POST-incr value.
//   - Rule rejects if returned current > threshold (strict).
//   - §5.4: rejected requests still incr untriggered rule counters (per-key independence).
//
// Counter trajectory (initial state: global=0, vip_key=0):
//
//	Case 1 (vip-key):     global 0->1, vip_key 0->1   → 200  (both ≤ threshold)
//	Case 2 (vip-key):     global 1->2, vip_key 1->2   → 429  (vip_key 2 > 1 triggers rule_item;
//	                                                         global 2 ≤ 3 still ok, but rule_item
//	                                                         wins per multi-rule OR; §5.4: global
//	                                                         was incr'd to 2 even though rejected)
//	Case 3 (no x-api-key): global 2->3                 → 200  (rule_item not matched, only global;
//	                                                         global 3 not > 3 yet)
//	Case 4 (no x-api-key): global 3->4                 → 429  (global 4 > 3 triggers global)
//
// §5.4 indirect validation: if Case 2 had not incr'd global (i.e. rejection skipped ALL incrs),
// global would stay at 1 after Case 2. Then Case 3 would incr to 2 (allow), Case 4 to 3 (allow,
// 3 not > 3), Case 5 to 4 (reject). The fact that we see rejection at Case 4 (not Case 5)
// confirms that rejected requests still incr untriggered rule counters per §5.4.
var WasmPluginsClusterKeyRateLimit = suite.ConformanceTest{
	ShortName:   "WasmPluginsClusterKeyRateLimit",
	Description: "The cluster-key-rate-limit plugin enforces multi-rule OR overlay and exposes X-RateLimit-* headers.",
	Manifests:   []string{"tests/go-wasm-cluster-key-rate-limit.yaml"},
	Features:    []suite.SupportedFeature{suite.WASMGoConformanceFeature},
	Test: func(t *testing.T, suite *suite.ConformanceTestSuite) {
		testcases := []http.Assertion{
			// Case 1: First vip-key request → 200 (global: 0→1, vip_key: 0→1)
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 1: first vip-key request passes (global=1, vip_key=1)",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:    "foo.com",
						Path:    "/",
						Method:  "GET",
						Headers: map[string]string{"x-api-key": "vip-key"},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 200,
					},
				},
			},
			// Case 2: Second vip-key request → 429 (vip_key triggers: 2 > 1)
			// Validates multi-rule OR: rule_item triggers before global reaches its limit.
			// Also validates §5.4 indirectly: global is still incr'd to 2 (because global was not over).
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 2: second vip-key request rejected by rule_item (vip_key=2 > 1)",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:    "foo.com",
						Path:    "/",
						Method:  "GET",
						Headers: map[string]string{"x-api-key": "vip-key"},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  429,
						ContentType: http.ContentTypeTextPlain,
						Body:        []byte(`Too many requests`),
					},
				},
			},
			// Case 3: Request without vip-key — rule_item not evaluated, only global.
			// global was incr'd to 2 in Case 2 (even though Case 2 was rejected) per §5.4.
			// Now global: 2→3, Go: 3 > 3? No → allow.
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 3: request without vip-key bypasses rule_item, global still under threshold (global=3)",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
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
						StatusCode: 200,
					},
				},
			},
			// Case 4: Another no-key request → global triggers (global: 3→4, 4 > 3).
			// Confirms global finally rejects and confirms §5.4 timing: rejection happens
			// at Case 4 (not Case 5) because Case 2 incr'd global even when rejected.
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 4: global triggered (validates §5.4 — rejected Case 2 still incremented global)",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
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
						StatusCode:  429,
						ContentType: http.ContentTypeTextPlain,
						Body:        []byte(`Too many requests`),
					},
				},
			},
		}

		t.Run("WasmPlugins cluster-key-rate-limit", func(t *testing.T) {
			for _, testcase := range testcases {
				http.MakeRequestAndExpectEventuallyConsistentResponse(t, suite.RoundTripper, suite.TimeoutConfig, suite.GatewayAddress, testcase)
			}
		})
	},
}
