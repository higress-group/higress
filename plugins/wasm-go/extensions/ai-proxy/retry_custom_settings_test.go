package main

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	wasmtest "github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestRetryPreservesCustomSettings(t *testing.T) {
	wasmtest.RunGoTest(t, func(t *testing.T) {
		for _, tt := range []struct {
			name              string
			providerType      string
			withSettings      bool
			overwrite         bool
			withRequestValues bool
			wantMaxTokens     int64
			wantTemperature   float64
			retries           int
		}{
			{"overwrite request values", "openai", true, true, true, 32, 0, 1},
			{"fill missing values", "openai", true, false, false, 32, 0, 1},
			{"retain supplied values", "openai", true, false, true, 4096, 0.9, 1},
			{"without custom settings", "openai", false, false, true, 4096, 0.9, 1},
			{"recursive retries", "openai", true, true, true, 32, 0, 2},
			{"before provider conversion", "claude", true, true, true, 32, 0.2, 1},
		} {
			t.Run(tt.name, func(t *testing.T) {
				providerConfig := map[string]interface{}{
					"type":      tt.providerType,
					"apiTokens": []string{"retry-token-a", "retry-token-b", "retry-token-c"},
					// A second mapping makes caching the provider-transformed body observable.
					"modelMapping": map[string]string{
						"client-model":   "upstream-model",
						"upstream-model": "mapped-twice",
					},
					"retryOnFailure": map[string]interface{}{
						"enabled": true, "maxRetries": tt.retries, "retryOnStatus": []string{"503"},
					},
				}
				if tt.withSettings {
					temperature := 0.0
					if tt.providerType == "claude" {
						temperature = 0.2
					}
					providerConfig["customSettings"] = []map[string]interface{}{
						{"name": "max_tokens", "value": 32, "overwrite": tt.overwrite},
						{"name": "temperature", "value": temperature, "overwrite": tt.overwrite},
					}
				}
				config, err := json.Marshal(map[string]interface{}{"provider": providerConfig})
				require.NoError(t, err)
				host, status := wasmtest.NewTestHost(config)
				defer host.Reset()
				require.Equal(t, types.OnPluginStartStatusOK, status)

				request := map[string]interface{}{
					"model": "client-model", "stream": false,
					"messages": []map[string]string{{"role": "user", "content": "hello"}},
				}
				if tt.withRequestValues {
					request["max_tokens"] = 4096
					request["temperature"] = 0.9
				}
				if tt.providerType == "claude" {
					request["messages"] = []map[string]string{
						{"role": "system", "content": "system instruction"},
						{"role": "user", "content": "hello"},
					}
				}
				body, err := json.Marshal(request)
				require.NoError(t, err)
				require.Equal(t, types.HeaderStopIteration, host.CallOnHttpRequestHeaders([][2]string{
					{":method", "POST"}, {":authority", "example.com"}, {":path", "/v1/chat/completions"},
					{"content-type", "application/json"}, {"content-length", strconv.Itoa(len(body))},
				}))
				require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody(body))
				initialBody := host.GetRequestBody()
				require.Equal(t, "upstream-model", gjson.GetBytes(initialBody, "model").String())
				require.Equal(t, tt.wantMaxTokens, gjson.GetBytes(initialBody, "max_tokens").Int())
				require.True(t, gjson.GetBytes(initialBody, "temperature").Exists())
				require.Equal(t, tt.wantTemperature, gjson.GetBytes(initialBody, "temperature").Float())
				if tt.providerType == "claude" {
					require.True(t, gjson.GetBytes(initialBody, "system").Exists())
					require.Len(t, gjson.GetBytes(initialBody, "messages").Array(), 1)
				}

				require.NoError(t, host.SetProperty([]string{"response", "code_details"}, []byte("via_upstream")))
				require.Equal(t, types.HeaderStopAllIterationAndWatermark, host.CallOnHttpResponseHeaders([][2]string{
					{":status", "503"}, {"content-type", "application/json"},
				}))
				tokenHeader := "authorization"
				if tt.providerType == "claude" {
					tokenHeader = "x-api-key"
				}
				usedTokens := []string{retryHeaderValue(host.GetRequestHeaders(), tokenHeader)}
				for attempt := 0; attempt < tt.retries; attempt++ {
					calls := host.GetHttpCalloutAttributes()
					require.Len(t, calls, 1)
					require.JSONEq(t, string(initialBody), string(calls[0].Body), "attempt %d must preserve the initial request parameters", attempt+1)
					token := retryHeaderValue(calls[0].Headers, tokenHeader)
					require.NotEmpty(t, token)
					require.NotContains(t, usedTokens, token)
					usedTokens = append(usedTokens, token)
					if attempt+1 < tt.retries {
						host.CallOnHttpCall([][2]string{{":status", "503"}}, []byte(`{"error":"retry again"}`))
					}
				}

				response := []byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`)
				if tt.providerType == "claude" {
					response = []byte(`{"id":"msg_retry","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
				}
				host.CallOnHttpCall([][2]string{{":status", "200"}, {"content-type", "application/json"}}, response)
				localResponse := host.GetLocalResponse()
				require.NotNil(t, localResponse)
				require.Equal(t, uint32(200), localResponse.StatusCode)
				require.Equal(t, "ok", gjson.GetBytes(localResponse.Data, "choices.0.message.content").String())
				require.Empty(t, host.GetHttpCalloutAttributes())
				host.CompleteHttp()
			})
		}
	})
}

func retryHeaderValue(headers [][2]string, name string) string {
	for _, header := range headers {
		if strings.EqualFold(header[0], name) {
			return header[1]
		}
	}
	return ""
}
