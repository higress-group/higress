package test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	wasmhost "github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

var edgeNoActiveProviderConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"providers": []map[string]interface{}{
			{"id": "a", "type": "generic", "genericHost": "http://127.0.0.1:8080", "apiTokens": []string{"t"}},
		},
	})
	return data
}()

var edgeGenericConfig = func() json.RawMessage {
	return LegacyProviderPluginJSON(map[string]interface{}{
		"type":         "generic",
		"genericHost":  "http://127.0.0.1:9999",
		"apiTokens":    []string{"tok"},
		"modelMapping": map[string]string{"*": "mapped-model"},
	})
}()

// RunMainEdgeCaseTests covers main.go branches: no active provider, unknown path, bad Content-Type, generic Claude path rewrite.
func RunMainEdgeCaseTests(t *testing.T) {
	wasmhost.RunGoTest(t, func(t *testing.T) {
		t.Run("no_active_provider_skips_body", func(t *testing.T) {
			host, status := wasmhost.NewTestHost(edgeNoActiveProviderConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.ActionContinue, action)
		})

		t.Run("unknown_path_logs_unsupported", func(t *testing.T) {
			host, status := wasmhost.NewTestHost(edgeGenericConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			_ = host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/not-a-real-openai-path"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			all := append(append([]string{}, host.GetDebugLogs()...), host.GetWarnLogs()...)
			found := false
			for _, line := range all {
				if strings.Contains(line, "unsupported path") {
					found = true
					break
				}
			}
			require.True(t, found, "logs: %v", all)
		})

		t.Run("multipart_on_chat_logs_unsupported_content_type", func(t *testing.T) {
			host, status := wasmhost.NewTestHost(edgeGenericConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			_ = host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "multipart/form-data; boundary=----abc"},
			})
			debugLogs := host.GetDebugLogs()
			found := false
			for _, line := range debugLogs {
				if strings.Contains(line, "unsupported content type") {
					found = true
					break
				}
			}
			require.True(t, found, "debug logs: %v", debugLogs)
		})
	})

	wasmhost.RunTest(t, func(t *testing.T) {
		t.Run("generic_claude_path_rewrites_to_chat_completions", func(t *testing.T) {
			host, status := wasmhost.NewTestHost(edgeGenericConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/messages"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			headers := host.GetRequestHeaders()
			pathVal, ok := wasmhost.GetHeaderValue(headers, ":path")
			require.True(t, ok)
			require.Equal(t, "/v1/chat/completions", pathVal)
		})

		t.Run("response_json_content_type_buffers_for_non_sse", func(t *testing.T) {
			host, status := wasmhost.NewTestHost(edgeGenericConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			host.CallOnHttpRequestBody([]byte(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`))

			host.SetProperty([]string{"response", "code_details"}, []byte("via_upstream"))
			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.ActionContinue, action)

			debugLogs := host.GetDebugLogs()
			found := false
			for _, line := range debugLogs {
				if strings.Contains(line, "onHttpResponseHeaders") {
					found = true
					break
				}
			}
			require.True(t, found, "expected onHttpResponseHeaders log, got: %v", debugLogs)
		})
	})
}
