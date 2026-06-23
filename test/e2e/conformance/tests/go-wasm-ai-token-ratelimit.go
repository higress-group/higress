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
	Register(WasmPluginsAiTokenRateLimit)
}

// llm-mock-service returns fixed total_tokens=10 per request.
// Config: global_threshold token_per_minute=100, vip-key rule_item token_per_minute=25.
// Counter trajectory (each vip-key request increments both):
//
//	Req1 后: vip=10,  global=10
//	Req2 后: vip=20,  global=20
//	Req3 后: vip=30,  global=30  (request check: vip=20 ≤ 25 OK)
//	Req4 检查: vip=30 > 25 → rule_item 触发 → 429
var WasmPluginsAiTokenRateLimit = suite.ConformanceTest{
	ShortName:   "WasmPluginsAiTokenRateLimit",
	Description: "The ai-token-ratelimit plugin enforces multi-rule OR overlay with real token usage from llm-mock backend.",
	Manifests:   []string{"tests/go-wasm-ai-token-ratelimit.yaml"},
	Features:    []suite.SupportedFeature{suite.WASMGoConformanceFeature},
	Test: func(t *testing.T, suite *suite.ConformanceTestSuite) {
		requestBody := []byte(`{"model":"gpt-3","messages":[{"role":"user","content":"hello"}],"stream":false}`)

		testcases := []http.Assertion{
			// Case 1: First vip-key request → 200 (vip=10, global=10)
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 1: first vip-key request passes (vip=10, global=10)",
					TargetBackend:   "llm-mock-service",
					TargetNamespace: "higress-conformance-ai-backend",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "ai-token-ratelimit.test.com",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        requestBody,
						Headers:     map[string]string{"x-api-key": "vip-key"},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeApplicationJson,
					},
				},
			},
			// Case 2: Second vip-key request → 200 (vip=20, global=20)
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 2: second vip-key request passes (vip=20, global=20)",
					TargetBackend:   "llm-mock-service",
					TargetNamespace: "higress-conformance-ai-backend",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "ai-token-ratelimit.test.com",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        requestBody,
						Headers:     map[string]string{"x-api-key": "vip-key"},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeApplicationJson,
					},
				},
			},
			// Case 3: Third vip-key request → 200 (check vip=20 ≤ 25, post: vip=30)
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 3: third vip-key request still passes (check vip=20 ≤ 25, post: vip=30)",
					TargetBackend:   "llm-mock-service",
					TargetNamespace: "higress-conformance-ai-backend",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "ai-token-ratelimit.test.com",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        requestBody,
						Headers:     map[string]string{"x-api-key": "vip-key"},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeApplicationJson,
					},
				},
			},
			// Case 4: Fourth vip-key request → 429 (vip=30 > 25)
			// Validates: rule_item triggers before global (vip=30 < global threshold 100), multi-rule OR semantics
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 4: fourth vip-key request rejected by rule_item (vip=30 > 25)",
					TargetBackend:   "llm-mock-service",
					TargetNamespace: "higress-conformance-ai-backend",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "ai-token-ratelimit.test.com",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        requestBody,
						Headers:     map[string]string{"x-api-key": "vip-key"},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  429,
						ContentType: http.ContentTypeTextPlain,
						Body:        []byte(`Too many AI token requests`),
					},
				},
			},
			// Case 5: Different x-api-key bypasses rule_item; global=30 still ≤ 100 → 200
			// Validates: rejected request doesn't enter response phase INCRBY (global stays at 30, not 40)
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 5: different x-api-key bypasses rule_item, global still under threshold",
					TargetBackend:   "llm-mock-service",
					TargetNamespace: "higress-conformance-ai-backend",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "ai-token-ratelimit.test.com",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body:        requestBody,
						Headers:     map[string]string{"x-api-key": "other-key"},
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode:  200,
						ContentType: http.ContentTypeApplicationJson,
					},
				},
			},
		}

		t.Run("WasmPlugins ai-token-ratelimit", func(t *testing.T) {
			for _, testcase := range testcases {
				http.MakeRequestAndExpectEventuallyConsistentResponse(t, suite.RoundTripper, suite.TimeoutConfig, suite.GatewayAddress, testcase)
			}
		})
	},
}
