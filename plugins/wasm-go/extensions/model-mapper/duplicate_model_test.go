package main

import (
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/proxytest"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

const duplicateModelRejectBody = `duplicate "model" member in request body`

func callMapperRequestBody(t *testing.T, body string) (types.Action, *proxytest.LocalHttpResponse) {
	t.Helper()
	host, status := test.NewTestHost(basicConfig)
	defer host.Reset()
	require.Equal(t, types.OnPluginStartStatusOK, status)

	host.CallOnHttpRequestHeaders([][2]string{
		{":authority", "example.com"},
		{":path", "/v1/chat/completions"},
		{":method", "POST"},
		{"content-type", "application/json"},
	})

	action := host.CallOnHttpRequestBody([]byte(body))
	return action, host.GetLocalResponse()
}

func TestDuplicateModelMemberRejected(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		action, local := callMapperRequestBody(t, `{"model":"gpt-3.5-turbo","model":"gpt-4o","messages":[]}`)
		require.Equal(t, types.ActionPause, action)
		require.NotNil(t, local, "duplicate model member must be answered locally")
		require.Equal(t, uint32(400), local.StatusCode)
		require.Equal(t, duplicateModelRejectBody, string(local.Data))
	})
}

func TestDuplicateModelMemberRejectedRegardlessOfValue(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// Identical values are still ambiguous to rewrite, so they are rejected too.
		action, local := callMapperRequestBody(t, `{"model":"gpt-3.5-turbo","stream":true,"model":"gpt-3.5-turbo"}`)
		require.Equal(t, types.ActionPause, action)
		require.NotNil(t, local)
		require.Equal(t, uint32(400), local.StatusCode)
	})
}

func TestSingleModelMemberUnaffected(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		action, local := callMapperRequestBody(t, `{"model":"gpt-3.5-turbo","messages":[]}`)
		require.Equal(t, types.ActionContinue, action)
		require.Nil(t, local, "a single model member must not be rejected")
	})
}

func TestModelWordInsideStringValueNotRejected(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// The count must be structural: "model" spelled inside a string value is
		// prompt text, not a member name.
		body := `{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"repeat \"model\" and \"model\" twice"}]}`
		action, local := callMapperRequestBody(t, body)
		require.Equal(t, types.ActionContinue, action)
		require.Nil(t, local)
	})
}

func TestNestedModelMemberNotRejected(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// Only the top level is counted, so a tool definition carrying its own
		// "model" keys stays valid.
		body := `{"model":"gpt-3.5-turbo","tools":[{"type":"function","function":{"name":"model","parameters":{"model":"a","model":"b"}}}]}`
		action, local := callMapperRequestBody(t, body)
		require.Equal(t, types.ActionContinue, action)
		require.Nil(t, local)
	})
}
