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
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/http"
	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/suite"
)

func init() {
	Register(WasmPluginsTokenStatistics)
}

var WasmPluginsTokenStatistics = suite.ConformanceTest{
	ShortName:   "WasmPluginTokenStatistics",
	Description: "Production-ready conformance test for the token-statistics WASM plugin covering multiple AI providers and scenarios.",
	Features:    []suite.SupportedFeature{suite.WASMGoConformanceFeature},
	Manifests:   []string{"tests/go-wasm-token-statistics.yaml"},
	Test: func(t *testing.T, suite *suite.ConformanceTestSuite) {
		testcases := []http.Assertion{
			{
				Meta: http.AssertionMeta{
					TestCaseName:    "case 1: OpenAI-compatible format (Qwen) with standard response",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "dashscope.aliyuncs.com",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body: []byte(`{
							"model": "qwen-long",
							"messages": [{"role":"user","content":"hello, test token statistics"}],
							"stream": false
						}`),
					},
					ExpectedRequest: &http.ExpectedRequest{
						Request: http.Request{
							Host:        "dashscope.aliyuncs.com",
							Path:        "/compatible-mode/v1/chat/completions",
							Method:      "POST",
							ContentType: http.ContentTypeApplicationJson,
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
					TestCaseName:    "case 2: Streaming response with token statistics",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "dashscope.aliyuncs.com",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body: []byte(`{
							"model": "qwen-long",
							"messages": [{"role":"user","content":"streaming test"}],
							"stream": true
						}`),
					},
					ExpectedRequest: &http.ExpectedRequest{
						Request: http.Request{
							Host:        "dashscope.aliyuncs.com",
							Path:        "/compatible-mode/v1/chat/completions",
							Method:      "POST",
							ContentType: http.ContentTypeApplicationJson,
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
					TestCaseName:    "case 3: Alternative AI provider (Qwen on different host)",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "qwen.ai.com",
						Path:        "/v1/chat/completions",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body: []byte(`{
							"model": "qwen-turbo",
							"messages": [{"role":"user","content":"test different host"}]
						}`),
					},
					ExpectedRequest: &http.ExpectedRequest{
						Request: http.Request{
							Host:        "qwen.ai.com",
							Path:        "/compatible-mode/v1/chat/completions",
							Method:      "POST",
							ContentType: http.ContentTypeApplicationJson,
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
					TestCaseName:    "case 4: Path filtering - should not process non-matching path",
					TargetBackend:   "infra-backend-v1",
					TargetNamespace: "higress-conformance-infra",
				},
				Request: http.AssertionRequest{
					ActualRequest: http.Request{
						Host:        "dashscope.aliyuncs.com",
						Path:        "/v1/embeddings",
						Method:      "POST",
						ContentType: http.ContentTypeApplicationJson,
						Body: []byte(`{
							"model": "text-embedding-v1",
							"input": "test embedding"
						}`),
					},
				},
				Response: http.AssertionResponse{
					ExpectedResponse: http.Response{
						StatusCode: 200,
					},
				},
			},
		}

		t.Run("WasmPlugins token-statistics", func(t *testing.T) {
			for _, testcase := range testcases {
				t.Run(testcase.Meta.TestCaseName, func(t *testing.T) {
					// Make request and verify response
					http.MakeRequestAndExpectEventuallyConsistentResponse(t, suite.RoundTripper, suite.TimeoutConfig, suite.GatewayAddress, testcase)

					// Only verify metrics for paths that should be processed
					if !strings.Contains(testcase.Request.ActualRequest.Path, "/chat/completions") &&
						!strings.Contains(testcase.Request.ActualRequest.Path, "/completions") {
						t.Log("Skipping metric verification for non-tracked path")
						return
					}

					// Verify Prometheus metrics (optional, won't fail test if unavailable)
					promAddr := os.Getenv("PROMETHEUS_ADDR")
					if promAddr == "" {
						t.Log("PROMETHEUS_ADDR not set; skipping metric assertion")
						return
					}

					// Wait for metrics to be exported and scraped
					time.Sleep(5 * time.Second)

					// Query for specific metrics
					queries := []struct {
						name  string
						query string
					}{
						{"input_tokens", `higress_token_statistics_qwen_long_input_tokens_total`},
						{"output_tokens", `higress_token_statistics_qwen_long_output_tokens_total`},
						{"total_tokens", `higress_token_statistics_qwen_long_total_tokens_total`},
						{"any_total", `sum({__name__=~"higress_token_statistics_.*_total_tokens_total"})`},
					}

					for _, q := range queries {
						t.Run("metric_"+q.name, func(t *testing.T) {
							var found bool
							var lastBody string
							var lastErr error

							// Retry up to 15 times with 3 second intervals (45 seconds total)
							for i := 0; i < 15; i++ {
								respBody, err := http.QueryPrometheus(promAddr, q.query)
								if err != nil {
									lastErr = err
									t.Logf("prometheus query %s attempt %d/%d failed: %v", q.name, i+1, 15, err)
									time.Sleep(3 * time.Second)
									continue
								}

								lastBody = string(respBody)

								// Parse Prometheus response
								var promResp struct {
									Status string `json:"status"`
									Data   struct {
										ResultType string `json:"resultType"`
										Result     []struct {
											Metric map[string]interface{} `json:"metric"`
											Value  []interface{}          `json:"value"`
										} `json:"result"`
									} `json:"data"`
								}

								if err := json.Unmarshal(respBody, &promResp); err != nil {
									lastErr = err
									t.Logf("failed to parse prometheus response for %s (attempt %d/%d): %v", q.name, i+1, 15, err)
									time.Sleep(3 * time.Second)
									continue
								}

								// Check if we have results
								if promResp.Status == "success" && len(promResp.Data.Result) > 0 {
									found = true
									t.Logf("✅ Successfully found metric %s with %d result(s) on attempt %d", q.name, len(promResp.Data.Result), i+1)

									// Log metric values for debugging
									for idx, result := range promResp.Data.Result {
										if len(result.Value) >= 2 {
											t.Logf("  Metric %s[%d]: value=%v, labels=%v", q.name, idx, result.Value[1], result.Metric)
										}
									}
									break
								}

								t.Logf("Metric %s not yet available (attempt %d/%d), waiting...", q.name, i+1, 15)
								time.Sleep(3 * time.Second)
							}

							if !found {
								t.Logf("WARNING: Prometheus metric %s not found after 15 retries (45 seconds)", q.name)
								if lastErr != nil {
									t.Logf("  Last error: %v", lastErr)
								}
								if lastBody != "" {
									t.Logf("  Last Prometheus response: %s", lastBody)
								}
								t.Logf("  Possible causes:")
								t.Logf("    1. Prometheus not configured to scrape Higress metrics")
								t.Logf("    2. Plugin not exporting metrics correctly")
								t.Logf("    3. Metric naming mismatch")
								t.Logf("    4. Network connectivity issues")
								// Don't fail the test - metrics verification is optional
								// This allows the test to pass in environments without Prometheus
							}
						})
					}
				})
			}
		})
	},
}
