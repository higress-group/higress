package main

import (
	"encoding/json"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

var emptyConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{})
	return data
}()

func TestGetRequestID(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("prefers request-id header over x_request_id property", func(t *testing.T) {
			host, status := test.NewTestHost(emptyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{"request-id", "biz-req-123"},
				{"x-request-id", "envoy-req-123"},
			}))
			require.Equal(t, "biz-req-123", getRequestID())
			host.CompleteHttp()
		})

		t.Run("falls back to x_request_id property", func(t *testing.T) {
			host, status := test.NewTestHost(emptyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
			}))
			require.NoError(t, host.SetRequestId("x-req-123"))
			require.Equal(t, "x-req-123", getRequestID())
			host.CompleteHttp()
		})

		t.Run("falls back to selected request-id property", func(t *testing.T) {
			host, status := test.NewTestHost(emptyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{"x-request-id", "envoy-req-123"},
			}))
			require.NoError(t, host.SetProperty([]string{"wasm.requestId"}, []byte("biz-req-123")))
			require.Equal(t, "biz-req-123", getRequestID())
			host.CompleteHttp()
		})

		t.Run("falls back to x-request-id header", func(t *testing.T) {
			host, status := test.NewTestHost(emptyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{"x-request-id", "header-req-123"},
			}))
			require.NoError(t, host.SetRequestId("-"))
			require.Equal(t, "header-req-123", getRequestID())
			host.CompleteHttp()
		})

		t.Run("falls back to x_request_id when request-id is dash", func(t *testing.T) {
			host, status := test.NewTestHost(emptyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{"request-id", "-"},
				{"x-request-id", "envoy-req-456"},
			}))
			require.Equal(t, "envoy-req-456", getRequestID())
			host.CompleteHttp()
		})

		t.Run("returns empty for dash request ID", func(t *testing.T) {
			host, status := test.NewTestHost(emptyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
			}))
			require.NoError(t, host.SetRequestId("-"))
			require.Empty(t, getRequestID())
			host.CompleteHttp()
		})

		t.Run("trims whitespace", func(t *testing.T) {
			host, status := test.NewTestHost(emptyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
			}))
			require.NoError(t, host.SetRequestId(" x-req-456 "))
			require.Equal(t, "x-req-456", getRequestID())
			host.CompleteHttp()
		})
	})
}

func TestNormalizeRequestID(t *testing.T) {
	require.Equal(t, "req-123", normalizeRequestID("req-123"))
	require.Equal(t, "req-456", normalizeRequestID(" req-456 "))
	require.Empty(t, normalizeRequestID(""))
	require.Empty(t, normalizeRequestID("   "))
	require.Empty(t, normalizeRequestID("-"))
}
