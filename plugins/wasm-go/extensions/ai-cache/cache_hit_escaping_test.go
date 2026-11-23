package main

import (
	"encoding/json"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

// A cached answer is embedded into the response template with fmt.Sprintf, so it
// has to be escaped as a JSON string. Escaping with
// strings.Trim(strconv.Quote(s), `"`) strips *every* leading and trailing quote
// character, including the backslash-escaped ones, so an answer ending in a
// double quote produces a broken template.
func TestCacheHitReturnsValidJSONForQuotedAnswers(t *testing.T) {
	answers := map[string]string{
		"trailing quote":  `他说："你好"`,
		"single quote":    `"`,
		"quoted number":   `the answer is "42"`,
		"consecutive":     `a"b"c"`,
		"trailing other":  `back\slash`,
		"leading quote":   `"quoted start`,
		"newline and tab": "line1\nline2\ttab",
		"plain text":      "no quotes at all",
	}
	for name, answer := range answers {
		t.Run(name, func(t *testing.T) {
			test.RunTest(t, func(t *testing.T) {
				host, status := test.NewTestHost(basicRedisConfig)
				defer host.Reset()
				require.Equal(t, types.OnPluginStartStatusOK, status)

				host.CallOnHttpRequestHeaders([][2]string{
					{":authority", "example.com"},
					{":path", "/api/chat"},
					{":method", "POST"},
					{"content-type", "application/json"},
					{"x-mse-consumer", "test-consumer"},
				})
				host.CallOnHttpRequestBody([]byte(`{
					"model": "qwen-turbo",
					"messages": [{"role": "user", "content": "hello"}],
					"stream": false
				}`))

				host.CallOnRedisCall(0, test.CreateRedisRespString(answer))

				local := host.GetLocalResponse()
				require.NotNil(t, local, "cache hit produced no local response")
				require.True(t, json.Valid(local.Data),
					"cached answer %q produced invalid JSON: %s", answer, string(local.Data))

				var payload struct {
					Choices []struct {
						Message struct {
							Content string `json:"content"`
						} `json:"message"`
					} `json:"choices"`
				}
				require.NoError(t, json.Unmarshal(local.Data, &payload))
				require.Len(t, payload.Choices, 1)
				require.Equal(t, answer, payload.Choices[0].Message.Content,
					"cached answer must round-trip unchanged")

				host.CompleteHttp()
			})
		})
	}
}
