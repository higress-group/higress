package main

import (
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

func TestDashScopeEmptyEmbeddingsDoesNotPanic(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(basicConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/v1/chat/completions"},
			{":method", "POST"},
		})

		action := host.CallOnHttpRequestBody([]byte(`{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"What is AI?"}]}`))
		require.Equal(t, types.ActionPause, action)

		emptyEmbeddingResponse := `{"output":{"embeddings":[]},"usage":{"total_tokens":0},"request_id":"req-empty"}`
		require.NotPanics(t, func() {
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(emptyEmbeddingResponse))
		})
	})
}
