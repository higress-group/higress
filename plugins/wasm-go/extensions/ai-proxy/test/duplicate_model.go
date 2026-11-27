package test

import (
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

const duplicateModelRejectBody = `duplicate "model" member in request body`

// RunDuplicateModelMemberTests covers the reject that ai-proxy performs before
// any provider logic reads or rewrites the model.
func RunDuplicateModelMemberTests(t *testing.T) {
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
				require.Equal(t, duplicateModelRejectBody, string(local.Data))
			} else {
				require.Nil(t, local, "the request must not be rejected")
			}
			return action
		}

		t.Run("duplicate top level model member is rejected", func(t *testing.T) {
			// The wildcard model mapping would otherwise rewrite whichever
			// occurrence sjson keeps, hiding the divergence from the caller.
			require.Equal(t, types.ActionPause,
				callRequestBody(t, `{"model":"gpt-4o","model":"gpt-3.5-turbo","messages":[{"role":"user","content":"test"}]}`))
		})

		t.Run("duplicate model member with identical values is rejected", func(t *testing.T) {
			require.Equal(t, types.ActionPause,
				callRequestBody(t, `{"model":"gpt-4o","stream":false,"model":"gpt-4o","messages":[]}`))
		})

		t.Run("single model member is unaffected", func(t *testing.T) {
			require.Equal(t, types.ActionContinue,
				callRequestBody(t, `{"model":"gpt-4o","messages":[{"role":"user","content":"test"}]}`))
		})

		t.Run("model word inside a string value is not a member", func(t *testing.T) {
			require.Equal(t, types.ActionContinue,
				callRequestBody(t, `{"model":"gpt-4o","messages":[{"role":"user","content":"set \"model\" to gpt-4, then \"model\" again"}]}`))
		})

		t.Run("nested model members are not top level members", func(t *testing.T) {
			require.Equal(t, types.ActionContinue,
				callRequestBody(t, `{"model":"gpt-4o","tools":[{"type":"function","function":{"name":"model","parameters":{"model":"a","model":"b"}}}],"messages":[]}`))
		})

		t.Run("malformed body is left to the existing validation", func(t *testing.T) {
			require.Equal(t, types.ActionContinue, callRequestBody(t, `{"model":"gpt-4o"`))
		})
	})
}
