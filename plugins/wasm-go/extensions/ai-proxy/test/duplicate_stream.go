package test

import (
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

const duplicateStreamRejectBody = `duplicate "stream" member in request body`

// RunDuplicateStreamMemberTests covers the reject that ai-proxy performs before
// any provider logic reads the streaming flag or normalizes stream_options.
func RunDuplicateStreamMemberTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		callRequestBody := func(t *testing.T, body string) types.Action {
			t.Helper()
			host, status := test.NewTestHost(basicOpenAIConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			action := host.CallOnHttpRequestBody([]byte(body))
			local := host.GetLocalResponse()
			if action == types.ActionPause {
				require.NotNil(t, local, "a paused request must be answered locally")
				require.Equal(t, uint32(400), local.StatusCode)
				require.Equal(t, duplicateStreamRejectBody, string(local.Data))
			} else {
				require.Nil(t, local, "the request must not be rejected")
			}
			return action
		}

		t.Run("duplicate top level stream member is rejected", func(t *testing.T) {
			// The first occurrence says non-streaming while the upstream keeps the
			// last one and streams without stream_options.include_usage, so token
			// usage would never be reported back for accounting.
			require.Equal(t, types.ActionPause,
				callRequestBody(t, `{"model":"gpt-4o","stream":false,"messages":[{"role":"user","content":"test"}],"stream":true}`))
		})

		t.Run("duplicate stream member with identical values is rejected", func(t *testing.T) {
			require.Equal(t, types.ActionPause,
				callRequestBody(t, `{"model":"gpt-4o","stream":true,"stream":true,"messages":[]}`))
		})

		t.Run("single stream member is unaffected", func(t *testing.T) {
			require.Equal(t, types.ActionContinue,
				callRequestBody(t, `{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"test"}]}`))
		})

		t.Run("stream word inside a string value is not a member", func(t *testing.T) {
			require.Equal(t, types.ActionContinue,
				callRequestBody(t, `{"model":"gpt-4o","messages":[{"role":"user","content":"set \"stream\" to true, then \"stream\" again"}]}`))
		})

		t.Run("nested stream members are not top level members", func(t *testing.T) {
			require.Equal(t, types.ActionContinue,
				callRequestBody(t, `{"model":"gpt-4o","tools":[{"type":"function","function":{"name":"stream","parameters":{"stream":"a","stream":"b"}}}],"messages":[]}`))
		})

		t.Run("malformed body is left to the existing validation", func(t *testing.T) {
			require.Equal(t, types.ActionContinue, callRequestBody(t, `{"model":"gpt-4o","stream":`))
		})
	})
}
