package main

import (
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

func TestIssue2918SplitJSONResponse(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(globalThresholdConfig)
		defer host.Reset()

		require.Equal(t, types.OnPluginStartStatusOK, status)

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/v1/chat/completions"},
			{":method", "POST"},
		})
		require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

		host.CallOnRedisCall(0, multiRuleResp(
			[3]int{1000, 1, 60},
		))
		require.Empty(t, host.GetRedisCalloutAttributes())

		action = host.CallOnHttpResponseHeaders([][2]string{
			{":status", "200"},
			{"content-type", "application/json; charset=utf-8"},
		})
		require.Equal(t, types.ActionContinue, action)

		chunk1 := []byte(`{"choices":[{"message":{"content":"hello"}}],"usa`)
		chunk2 := []byte(`ge":{"prompt_tokens":10,"completion_tokens":15,"total_tokens":25}}`)

		host.CallOnHttpStreamingResponseBody(chunk1, false)
		host.CallOnHttpStreamingResponseBody(chunk2, true)

		require.Len(t, host.GetRedisCalloutAttributes(), 1)
	})
}

func TestIssue2918SSEStaysStreaming(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(globalThresholdConfig)
		defer host.Reset()

		require.Equal(t, types.OnPluginStartStatusOK, status)

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/v1/chat/completions"},
			{":method", "POST"},
		})
		require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

		host.CallOnRedisCall(0, multiRuleResp(
			[3]int{1000, 1, 60},
		))

		action = host.CallOnHttpResponseHeaders([][2]string{
			{":status", "200"},
			{"content-type", "text/event-stream"},
		})
		require.Equal(t, types.ActionContinue, action)

		action = host.CallOnHttpStreamingResponseBody(
			[]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n"),
			false,
		)
		require.Equal(t, types.ActionContinue, action)
	})
}
