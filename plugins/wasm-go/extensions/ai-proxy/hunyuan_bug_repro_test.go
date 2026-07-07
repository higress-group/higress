package main

import (
	"encoding/json"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	wasmhost "github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

func TestHunyuanStreamingLastChunkWithoutTerminatorDoesNotPanic(t *testing.T) {
	wasmhost.RunTest(t, func(t *testing.T) {
		cfg, _ := json.Marshal(map[string]interface{}{
			"provider": map[string]interface{}{
				"type":           "hunyuan",
				"hunyuanAuthId":  "12345678-1234-1234-1234-123456789012",
				"hunyuanAuthKey": "12345678901234567890123456789012",
			},
		})

		host, status := wasmhost.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/v1/chat/completions"},
			{":method", "POST"},
			{"Content-Type", "application/json"},
		})
		host.CallOnHttpRequestBody([]byte(`{"model":"hunyuan-lite","messages":[{"role":"user","content":"hi"}],"stream":true}`))
		host.SetProperty([]string{"response", "code_details"}, []byte("via_upstream"))
		host.CallOnHttpResponseHeaders([][2]string{
			{":status", "200"},
			{"Content-Type", "text/event-stream"},
		})

		lastChunkWithoutTerminator := `data: {"Choices":[{"Delta":{"Role":"assistant","Content":"hello"},"FinishReason":"stop"}],"Created":1716359713,"Id":"hunyuan-test","Usage":{"PromptTokens":1,"CompletionTokens":1,"TotalTokens":2}}`

		require.NotPanics(t, func() {
			action := host.CallOnHttpStreamingResponseBody([]byte(lastChunkWithoutTerminator), true)
			require.Equal(t, types.ActionContinue, action)
		})
	})
}
