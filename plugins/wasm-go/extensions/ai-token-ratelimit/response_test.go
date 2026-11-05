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

package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/resp"
)

func TestResponseWithoutBodyDoesNotBuffer(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		cases := []struct {
			name, method, status, contentLength string
		}{
			{"no-content", "POST", "204", ""},
			{"empty-json", "POST", "200", "0"},
			{"head-with-content-length", "HEAD", "200", "89"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				host, status := test.NewTestHost(globalThresholdConfig)
				defer host.Reset()
				require.Equal(t, types.OnPluginStartStatusOK, status)
				contextID := host.InitializeHttpContext()
				defer host.CompleteHttpContext(contextID)
				require.Equal(t, types.HeaderStopAllIterationAndWatermark, host.CallOnRequestHeaders(contextID, [][2]string{
					{":authority", "example.com"}, {":path", "/v1/chat/completions"}, {":method", tc.method},
				}, false))
				calls := host.GetRedisCalloutAttributesFromContext(contextID)
				require.Len(t, calls, 1)
				host.CallOnRedisCallResponse(calls[0].CalloutID, 0, multiRuleResp([3]int{1000, 0, 60}))

				headers := [][2]string{{":status", tc.status}, {"content-type", "application/json"}}
				if tc.contentLength != "" {
					headers = append(headers, [2]string{"content-length", tc.contentLength})
				}
				// Use the host API directly to deliver endOfStream in the header phase.
				require.Equal(t, types.ActionContinue, host.CallOnResponseHeaders(contextID, headers, true))
				limit, err := host.GetProperty([]string{"set_encoder_buffer_limit"})
				require.Error(t, err, "a response without a body must not enable JSON buffering")
				require.Empty(t, limit)
				require.Empty(t, host.GetRedisCalloutAttributesFromContext(contextID))
			})
		}
	})
}

func TestJSONResponseAccountsSplitUsageOnce(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(globalThresholdConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		require.Equal(t, types.HeaderStopAllIterationAndWatermark, host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"}, {":path", "/v1/chat/completions"}, {":method", "POST"},
		}))
		host.CallOnRedisCall(0, multiRuleResp([3]int{1000, 0, 60}))
		host.CallOnHttpResponseHeaders([][2]string{{":status", "200"}, {"content-type", "application/json"}})

		body := []byte(`{"choices":[{"message":{"content":"hello"}}],"usage":{"prompt_tokens":10,"completion_tokens":15,"total_tokens":25}}`)
		split := bytes.Index(body, []byte(`"usage"`)) + 4
		host.CallOnHttpStreamingResponseBody(body[:split], false)
		require.Empty(t, host.GetRedisCalloutAttributes(), "usage must not be accounted before EOS")
		host.CallOnHttpStreamingResponseBody(body[split:], false)
		require.Empty(t, host.GetRedisCalloutAttributes(), "usage must not be accounted before EOS")
		require.Equal(t, types.ActionContinue, host.CallOnHttpStreamingResponseBody(nil, true))

		calls := host.GetRedisCalloutAttributes()
		require.Len(t, calls, 1, "a complete JSON response must be accounted exactly once")
		query, _, err := resp.NewReader(bytes.NewReader(calls[0].Query)).ReadValue()
		require.NoError(t, err)
		args := query.Array()
		require.Equal(t, "25", args[len(args)-1].String(), "account for the full prompt + completion sum")
		require.Equal(t, body, host.GetResponseBody(), "buffering must preserve response bytes")
		host.CompleteHttp()
	})
}

func TestJSONResponseAllowsBlankLines(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host := startResponseTest(t, globalThresholdConfig, "application/json")
		defer host.Reset()
		body := []byte(`{
  "model": "fixture",
  "usage": {
    "prompt_tokens": 20,

    "completion_tokens": 30,
    "total_tokens": 50
  }
}`)
		// A complete non-streaming JSON body is one document, including blank lines.
		require.Equal(t, types.ActionContinue, host.CallOnHttpStreamingResponseBody(body, true))
		assertResponseIncrement(t, host, 50, 1)
		require.Equal(t, body, host.GetResponseBody())
		host.CompleteHttp()
	})
}

func TestJSONResponseUsageFormats(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		cases := []struct {
			name, body string
			count      int
		}{
			{"openai-chat", `{"model":"fixture","usage":{"prompt_tokens":3,"completion_tokens":7,"total_tokens":10}}`, 10},
			{"openai-images", `{"model":"fixture","usage":{"input_tokens":5,"output_tokens":7,"total_tokens":12}}`, 12},
			{"openai-responses", `{"response":{"model":"fixture","usage":{"input_tokens":13,"output_tokens":17,"total_tokens":30}}}`, 30},
			{"anthropic", `{"message":{"model":"fixture","usage":{"input_tokens":3}},"usage":{"output_tokens":7,"cache_creation_input_tokens":2,"cache_read_input_tokens":5}}`, 10},
			{"gemini", `{"modelVersion":"fixture","usageMetadata":{"promptTokenCount":9,"candidatesTokenCount":11,"totalTokenCount":20}}`, 20},
			{"escaped-usage-key", `{"us\u0061ge":{"prompt_tokens":20,"completion_tokens":30,"total_tokens":50}}`, 50},
			{"crlf-blank-lines", "{\r\n\"usage\":{\r\n\"prompt_tokens\":20,\r\n\r\n\"completion_tokens\":30,\r\n\"total_tokens\":50}}", 50},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				host := startResponseTest(t, globalThresholdConfig, "application/json")
				defer host.Reset()
				body := []byte(tc.body)
				host.CallOnHttpStreamingResponseBody(body, true)
				assertResponseIncrement(t, host, tc.count, 1)
				require.Equal(t, body, host.GetResponseBody())
				host.CompleteHttp()
			})
		}
	})
}

func TestJSONResponseAccountsAtEverySplit(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		body := []byte(`{"choices":[{"message":{"content":"hello"}}],"usage":{"prompt_tokens":10,"completion_tokens":15,"total_tokens":25}}`)
		for split := 0; split <= len(body); split++ {
			t.Run(fmt.Sprintf("split-%d", split), func(t *testing.T) {
				host := startResponseTest(t, globalThresholdConfig, "application/json; charset=utf-8")
				defer host.Reset()
				host.CallOnHttpStreamingResponseBody(body[:split], false)
				host.CallOnHttpStreamingResponseBody(body[split:], false)
				require.Empty(t, host.GetRedisCalloutAttributes())
				host.CallOnHttpStreamingResponseBody(nil, true)
				assertResponseIncrement(t, host, 25, 1)
				require.Equal(t, string(body), string(host.GetResponseBody()))
				host.CompleteHttp()
			})
		}
	})
}

func TestJSONResponseVariants(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		cases := []struct {
			name, contentType, body string
			count                   int
		}{
			{"single-chunk", "application/json", `{"usage":{"prompt_tokens":3,"completion_tokens":7,"total_tokens":10}}`, 10},
			{"media-type-case", "Application/JSON; Charset=utf-8", `{"usage":{"prompt_tokens":3,"completion_tokens":7,"total_tokens":10}}`, 10},
			{"quoted-parameter", `application/json; profile="test"`, `{"usage":{"prompt_tokens":3,"completion_tokens":7,"total_tokens":10}}`, 10},
			{"missing-usage", "application/json", `{"error":{"message":"upstream error"}}`, 0},
			{"zero-usage", "application/json", `{"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}}`, 0},
			{"empty", "application/json", "", 0},
			{"large-json", "application/json", `{"content":"` + strings.Repeat("x", 256*1024) + `","usage":{"prompt_tokens":3,"completion_tokens":7,"total_tokens":10}}`, 10},
			{"two-megabyte-json", "application/json", `{"content":"` + strings.Repeat("x", 2*1024*1024) + `","usage":{"prompt_tokens":3,"completion_tokens":7,"total_tokens":10}}`, 10},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				host := startResponseTest(t, globalThresholdConfig, tc.contentType)
				defer host.Reset()
				body := []byte(tc.body)
				mid := len(body) / 2
				host.CallOnHttpStreamingResponseBody(body[:mid], false)
				host.CallOnHttpStreamingResponseBody(body[mid:], true)
				if tc.count == 0 {
					require.Empty(t, host.GetRedisCalloutAttributes())
				} else {
					assertResponseIncrement(t, host, tc.count, 1)
				}
				require.Equal(t, tc.body, string(host.GetResponseBody()))
				host.CompleteHttp()
			})
		}
	})
}

func TestSSEResponseRemainsStreaming(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host := startResponseTest(t, globalThresholdConfig, "text/event-stream")
		defer host.Reset()
		content := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n")
		require.Equal(t, types.ActionContinue, host.CallOnHttpStreamingResponseBody(content, false))
		require.Equal(t, content, host.GetResponseBody())
		require.Empty(t, host.GetRedisCalloutAttributes())
		usage := []byte("data: {\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":15,\"total_tokens\":25}}\n\n")
		require.Equal(t, types.ActionContinue, host.CallOnHttpStreamingResponseBody(usage, false))
		require.Equal(t, usage, host.GetResponseBody())
		require.Empty(t, host.GetRedisCalloutAttributes())
		done := []byte("data: [DONE]\n\n")
		require.Equal(t, types.ActionContinue, host.CallOnHttpStreamingResponseBody(done, true))
		require.Equal(t, done, host.GetResponseBody())
		assertResponseIncrement(t, host, 25, 1)
		host.CompleteHttp()
	})
}

func TestNonJSONMediaTypesRemainStreaming(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		for _, contentType := range []string{"application/jsonp", ""} {
			t.Run(contentType, func(t *testing.T) {
				host := startResponseTest(t, globalThresholdConfig, contentType)
				defer host.Reset()
				body := []byte(`{"usage":{"prompt_tokens":3,"completion_tokens":7,"total_tokens":10}}`)
				require.Equal(t, types.ActionContinue, host.CallOnHttpStreamingResponseBody(body, false))
				require.Equal(t, body, host.GetResponseBody())
				require.Empty(t, host.GetRedisCalloutAttributes())
				host.CallOnHttpStreamingResponseBody(nil, true)
				assertResponseIncrement(t, host, 10, 1)
				host.CompleteHttp()
			})
		}
	})
}

func TestUnmatchedJSONResponseRemainsStreaming(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(headerLimitConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"}, {":path", "/v1/chat/completions"}, {":method", "POST"},
		}))
		require.Empty(t, host.GetRedisCalloutAttributes())
		host.CallOnHttpResponseHeaders([][2]string{{":status", "200"}, {"content-type", "application/json"}})
		first := []byte(`{"content":"hello",`)
		require.Equal(t, types.ActionContinue, host.CallOnHttpStreamingResponseBody(first, false), "unmatched requests must not start buffering")
		require.Equal(t, first, host.GetResponseBody())
		last := []byte(`"usage":{"prompt_tokens":10,"completion_tokens":15,"total_tokens":25}}`)
		require.Equal(t, types.ActionContinue, host.CallOnHttpStreamingResponseBody(last, true))
		require.Equal(t, last, host.GetResponseBody())
		require.Empty(t, host.GetRedisCalloutAttributes())
		host.CompleteHttp()
	})
}

func TestResponseContextIsolationAndMultipleRules(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(hybridLimitConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		for _, count := range []int{13, 0, 29, 0, 7} {
			host.CallOnHttpRequestHeaders([][2]string{{":authority", "example.com"}, {":path", "/v1/chat/completions"}, {":method", "POST"}, {"x-api-key", "vip-key"}})
			host.CallOnRedisCall(0, multiRuleResp([3]int{10000, 0, 60}, [3]int{100, 0, 60}))
			host.CallOnHttpResponseHeaders([][2]string{{":status", "200"}, {"content-type", "application/json"}})
			body := []byte(fmt.Sprintf(`{"usage":{"prompt_tokens":%d,"completion_tokens":0,"total_tokens":%d}}`, count, count))
			host.CallOnHttpStreamingResponseBody(body[:len(body)/2], false)
			host.CallOnHttpStreamingResponseBody(body[len(body)/2:], true)
			if count == 0 {
				require.Empty(t, host.GetRedisCalloutAttributes(), "a prior request must not supply usage")
			} else {
				assertResponseIncrement(t, host, count, 2)
				host.CallOnRedisCall(0, []byte(":2\r\n"))
			}
			host.CompleteHttp()
		}
	})
}

func startResponseTest(t *testing.T, cfg []byte, contentType string) test.TestHost {
	t.Helper()
	host, status := test.NewTestHost(cfg)
	require.Equal(t, types.OnPluginStartStatusOK, status)
	host.CallOnHttpRequestHeaders([][2]string{{":authority", "example.com"}, {":path", "/v1/chat/completions"}, {":method", "POST"}})
	host.CallOnRedisCall(0, multiRuleResp([3]int{1000, 0, 60}))
	host.CallOnHttpResponseHeaders([][2]string{{":status", "200"}, {"content-type", contentType}})
	return host
}

func assertResponseIncrement(t *testing.T, host test.TestHost, count, rules int) {
	t.Helper()
	calls := host.GetRedisCalloutAttributes()
	require.Len(t, calls, 1)
	query, _, err := resp.NewReader(bytes.NewReader(calls[0].Query)).ReadValue()
	require.NoError(t, err)
	args := query.Array()
	require.Equal(t, rules, args[2].Integer())
	require.Len(t, args, 3+rules+3*rules)
	for i := 0; i < rules; i++ {
		require.Equal(t, fmt.Sprint(count), args[3+rules+3*i+2].String())
	}
}
