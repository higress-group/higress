// Copyright 2026 alibaba
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tests

import (
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/http"
	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/roundtripper"
	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/suite"
)

func init() { Register(WasmPluginsAiTokenRateLimit) }

var WasmPluginsAiTokenRateLimit = suite.ConformanceTest{
	ShortName:   "WasmPluginAiTokenRateLimit",
	Description: "The ai-token-ratelimit WASM plugin counts response token usage against per-minute budgets and rejects with 429 once a budget is exhausted.",
	Features:    []suite.SupportedFeature{suite.WASMGoConformanceFeature},
	Manifests:   []string{"tests/go-wasm-ai-token-ratelimit.yaml"},
	Test: func(t *testing.T, suite *suite.ConformanceTestSuite) {
		const (
			paramHost  = "ai-token-ratelimit.example.com"
			headerHost = "ai-token-ratelimit-header.example.com"
			// The llm-mock backend reports a fixed usage of 10 tokens
			// (prompt 9 + completion 1) for every non-streaming response.
			mockNonStreamBody = `{"id":"chatcmpl-llm-mock","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop","logprobs":null}],"created":10,"model":"gpt-3","object":"chat.completion","usage":{"prompt_tokens":9,"completion_tokens":1,"total_tokens":10}}`
		)

		// postChat sends one chat completion request and returns the raw
		// response. Rate-limit state is shared per key, so cases that consume
		// budget must not be retried; they use this one-shot helper instead
		// of MakeRequestAndExpectEventuallyConsistentResponse.
		postChat := func(t *testing.T, host string, query string, headers map[string]string, stream bool) *roundtripper.CapturedResponse {
			t.Helper()
			body := `{"model":"gpt-3","messages":[{"role":"user","content":"hi"}],"stream":false}`
			if stream {
				body = `{"model":"gpt-3","messages":[{"role":"user","content":"hi"}],"stream":true}`
			}
			req := roundtripper.Request{
				URL:        url.URL{Scheme: "http", Host: suite.GatewayAddress, Path: "/v1/chat/completions", RawQuery: query},
				Host:       host,
				Protocol:   "HTTP/1.1",
				Method:     "POST",
				Headers:    map[string][]string{},
				Body:       []byte(body),
				ContentType: http.ContentTypeApplicationJson,
			}
			for name, value := range headers {
				req.Headers[name] = []string{value}
			}
			_, res, err := suite.RoundTripper.CaptureRoundTrip(req)
			require.NoError(t, err)
			return res
		}

		hasResetHeader := func(res *roundtripper.CapturedResponse) bool {
			for name := range res.Headers {
				if strings.ToLower(name) == "x-tokenratelimit-reset" {
					return true
				}
			}
			return false
		}

		requireLimited := func(t *testing.T, res *roundtripper.CapturedResponse) {
			t.Helper()
			require.Equal(t, 429, res.StatusCode, string(res.Body))
			require.Equal(t, "Too many requests", string(res.Body))
			require.True(t, hasResetHeader(res), "expected X-TokenRateLimit-Reset header, got %v", res.Headers)
		}

		// Warm up the route, the plugin and redis with a generous budget
		// before the one-shot sequence below. This request is retried by the
		// framework until consistent, which also absorbs any readiness delay.
		warmup := http.Assertion{
			Meta: http.AssertionMeta{TestCaseName: "warmup", CompareTarget: http.CompareTargetResponse},
			Request: http.AssertionRequest{ActualRequest: http.Request{
				Host:        paramHost,
				Path:        "/v1/chat/completions?apikey=key-warm",
				Method:      "POST",
				ContentType: http.ContentTypeApplicationJson,
				Body:        []byte(`{"model":"gpt-3","messages":[{"role":"user","content":"hi"}],"stream":false}`),
			}},
			Response: http.AssertionResponse{ExpectedResponse: http.Response{
				StatusCode: 200,
				Body:       []byte(mockNonStreamBody),
			}},
		}
		http.MakeRequestAndExpectEventuallyConsistentResponse(t, suite.RoundTripper, suite.TimeoutConfig, suite.GatewayAddress, warmup)

		t.Run("non-streaming usage is counted and exhausts the budget", func(t *testing.T) {
			// key-a allows 5 tokens per minute; the mock response reports 10,
			// so the first request passes and the second is rejected.
			res := postChat(t, paramHost, "apikey=key-a", nil, false)
			require.Equal(t, 200, res.StatusCode, string(res.Body))
			require.JSONEq(t, mockNonStreamBody, string(res.Body))

			requireLimited(t, postChat(t, paramHost, "apikey=key-a", nil, false))
		})

		t.Run("streaming requests pass through", func(t *testing.T) {
			// The mock server does not emit an SSE usage event yet, so a
			// streaming response records 0 tokens and stays within budget.
			// Full streaming accounting coverage follows the mock-server
			// stream_options.include_usage support.
			res := postChat(t, paramHost, "apikey=key-s", nil, true)
			require.Equal(t, 200, res.StatusCode, string(res.Body))
			require.Contains(t, string(res.Body), "data: [DONE]")
		})

		t.Run("budgets are isolated per key", func(t *testing.T) {
			// key-a is exhausted from the previous case, while the
			// independent key-b budget is untouched.
			requireLimited(t, postChat(t, paramHost, "apikey=key-a", nil, false))

			res := postChat(t, paramHost, "apikey=key-b", nil, false)
			require.Equal(t, 200, res.StatusCode, string(res.Body))
			require.JSONEq(t, mockNonStreamBody, string(res.Body))
		})

		t.Run("header-based limits share the same contract", func(t *testing.T) {
			res := postChat(t, headerHost, "", map[string]string{"x-api-key": "key-h"}, false)
			require.Equal(t, 200, res.StatusCode, string(res.Body))
			require.JSONEq(t, mockNonStreamBody, string(res.Body))

			requireLimited(t, postChat(t, headerHost, "", map[string]string{"x-api-key": "key-h"}, false))
		})
	},
}
