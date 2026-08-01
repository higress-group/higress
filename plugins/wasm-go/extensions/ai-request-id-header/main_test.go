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

func configWithSkipPatterns(patterns []string) json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"skipClusterNamePatterns": patterns,
	})
	return data
}

const clearRouteCacheSentinel = "__no_reroute__"

func assertClearRouteCacheOff(t *testing.T, host test.TestHost) {
	t.Helper()
	val, err := host.GetProperty([]string{"clear_route_cache"})
	require.NoError(t, err, "expected clear_route_cache to be set (DisableReroute must be called before header mutation)")
	require.Equal(t, "off", string(val))
}

// seedNonMutation pre-seeds clear_route_cache with a sentinel before calling onHttpRequestHeaders.
// Use assertClearRouteCacheNotMutated after the call to verify DisableReroute was not invoked.
func seedNonMutation(t *testing.T, host test.TestHost) {
	t.Helper()
	require.NoError(t, host.SetProperty([]string{"clear_route_cache"}, []byte(clearRouteCacheSentinel)))
}

func assertClearRouteCacheNotMutated(t *testing.T, host test.TestHost) {
	t.Helper()
	val, err := host.GetProperty([]string{"clear_route_cache"})
	require.NoError(t, err)
	require.Equal(t, clearRouteCacheSentinel, string(val), "DisableReroute must not be called when no header mutation occurs")
}

func TestOnHttpRequestHeaders(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("no mutation when request-id normalizes to empty", func(t *testing.T) {
			host, status := test.NewTestHost(emptyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// "-" normalizes to empty: getRequestID returns "" → no header mutation.
			require.NoError(t, host.SetRequestId("-"))
			seedNonMutation(t, host)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
			})

			require.Equal(t, types.ActionContinue, action)
			assertClearRouteCacheNotMutated(t, host)
			host.CompleteHttp()
		})

		t.Run("injects request-id from x_request_id", func(t *testing.T) {
			host, status := test.NewTestHost(emptyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			require.NoError(t, host.SetRequestId("x-req-123"))

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.True(t, test.HasHeaderWithValue(host.GetRequestHeaders(), upstreamRequestIDHeader, "x-req-123"))
			assertClearRouteCacheOff(t, host)
			host.CompleteHttp()
		})

		t.Run("preserves existing request-id over x_request_id", func(t *testing.T) {
			host, status := test.NewTestHost(emptyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			require.NoError(t, host.SetRequestId("x-req-456"))
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{upstreamRequestIDHeader, "old-req"},
			})

			require.Equal(t, types.ActionContinue, action)
			value, ok := test.GetHeaderValue(host.GetRequestHeaders(), upstreamRequestIDHeader)
			require.True(t, ok)
			require.Equal(t, "old-req", value)
			propertyValue, err := host.GetProperty([]string{requestIDPropertyKey})
			require.NoError(t, err)
			require.Equal(t, "old-req", string(propertyValue))
			assertClearRouteCacheOff(t, host)
			host.CompleteHttp()
		})

		t.Run("skips dash request ID without mutating headers", func(t *testing.T) {
			host, status := test.NewTestHost(emptyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			require.NoError(t, host.SetRequestId("-"))
			seedNonMutation(t, host)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{upstreamRequestIDHeader, "-"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.True(t, test.HasHeaderWithValue(host.GetRequestHeaders(), upstreamRequestIDHeader, "-"))
			assertClearRouteCacheNotMutated(t, host)
			host.CompleteHttp()
		})

		t.Run("no mutation when providerType skip and no request-id header", func(t *testing.T) {
			host, status := test.NewTestHost(emptyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			require.NoError(t, host.SetRequestId("x-req-ai"))
			require.NoError(t, host.SetProperty([]string{providerTypePropertyKey}, []byte("openai")))
			seedNonMutation(t, host)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "api.openai.com"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.False(t, test.HasHeader(host.GetRequestHeaders(), upstreamRequestIDHeader))
			assertClearRouteCacheNotMutated(t, host)
			host.CompleteHttp()
		})

		t.Run("removes existing request-id and disables reroute when providerType skip", func(t *testing.T) {
			host, status := test.NewTestHost(emptyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			require.NoError(t, host.SetRequestId("x-req-ai"))
			require.NoError(t, host.SetProperty([]string{providerTypePropertyKey}, []byte("openai")))

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "api.openai.com"},
				{upstreamRequestIDHeader, "biz-req-ai"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.False(t, test.HasHeader(host.GetRequestHeaders(), upstreamRequestIDHeader))
			propertyValue, err := host.GetProperty([]string{requestIDPropertyKey})
			require.NoError(t, err)
			require.Equal(t, "biz-req-ai", string(propertyValue))
			assertClearRouteCacheOff(t, host)
			host.CompleteHttp()
		})

		t.Run("no mutation when cluster_name skip and no request-id header", func(t *testing.T) {
			host, status := test.NewTestHost(emptyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			require.NoError(t, host.SetRequestId("x-req-llm"))
			require.NoError(t, host.SetClusterName("outbound|443||llm-RightCodes.internal.dns"))
			seedNonMutation(t, host)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "www.right.codes"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.False(t, test.HasHeader(host.GetRequestHeaders(), upstreamRequestIDHeader))
			assertClearRouteCacheNotMutated(t, host)
			host.CompleteHttp()
		})

		t.Run("removes existing request-id and disables reroute when cluster_name skip", func(t *testing.T) {
			host, status := test.NewTestHost(emptyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			require.NoError(t, host.SetRequestId("x-req-llm"))
			require.NoError(t, host.SetClusterName("outbound|443||llm-RightCodes.internal.dns"))

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "www.right.codes"},
				{upstreamRequestIDHeader, "biz-req-llm"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.False(t, test.HasHeader(host.GetRequestHeaders(), upstreamRequestIDHeader))
			propertyValue, err := host.GetProperty([]string{requestIDPropertyKey})
			require.NoError(t, err)
			require.Equal(t, "biz-req-llm", string(propertyValue))
			assertClearRouteCacheOff(t, host)
			host.CompleteHttp()
		})

		t.Run("no mutation when configured cluster_name skip and no request-id header", func(t *testing.T) {
			host, status := test.NewTestHost(configWithSkipPatterns([]string{"right-codes.dns"}))
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			require.NoError(t, host.SetRequestId("x-req-direct"))
			require.NoError(t, host.SetClusterName("outbound|443||right-codes.dns"))
			seedNonMutation(t, host)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "www.right.codes"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.False(t, test.HasHeader(host.GetRequestHeaders(), upstreamRequestIDHeader))
			assertClearRouteCacheNotMutated(t, host)
			host.CompleteHttp()
		})

		t.Run("still injects and disables reroute when cluster_name does not match patterns", func(t *testing.T) {
			host, status := test.NewTestHost(emptyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			require.NoError(t, host.SetRequestId("x-req-biz"))
			require.NoError(t, host.SetClusterName("outbound|80||onepiece-open-platform.onepiece.svc.cluster.local"))

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "biz.example.com"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.True(t, test.HasHeaderWithValue(host.GetRequestHeaders(), upstreamRequestIDHeader, "x-req-biz"))
			assertClearRouteCacheOff(t, host)
			host.CompleteHttp()
		})
	})
}

func TestOnHttpResponseHeaders(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("sets request-id from existing request-id", func(t *testing.T) {
			host, status := test.NewTestHost(emptyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{upstreamRequestIDHeader, "biz-req-123"},
				{"x-request-id", "x-req-123"},
			}))

			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.True(t, test.HasHeaderWithValue(host.GetResponseHeaders(), responseRequestIDHeader, "biz-req-123"))
			host.CompleteHttp()
		})

		t.Run("sets request-id from x_request_id", func(t *testing.T) {
			host, status := test.NewTestHost(emptyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			require.NoError(t, host.SetRequestId("x-req-123"))

			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.True(t, test.HasHeaderWithValue(host.GetResponseHeaders(), responseRequestIDHeader, "x-req-123"))
			host.CompleteHttp()
		})

		t.Run("falls back to legacy x-request-id header", func(t *testing.T) {
			host, status := test.NewTestHost(emptyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{"x-request-id", "header-req-123"},
			}))
			require.NoError(t, host.SetRequestId("-"))

			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.True(t, test.HasHeaderWithValue(host.GetResponseHeaders(), responseRequestIDHeader, "header-req-123"))
			host.CompleteHttp()
		})

		t.Run("skips dash request ID", func(t *testing.T) {
			host, status := test.NewTestHost(emptyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			require.NoError(t, host.SetRequestId("-"))

			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.False(t, test.HasHeader(host.GetResponseHeaders(), responseRequestIDHeader))
			host.CompleteHttp()
		})
	})
}

func TestGetRequestID(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("returns existing request-id before x_request_id", func(t *testing.T) {
			host, status := test.NewTestHost(emptyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{upstreamRequestIDHeader, "biz-req-123"},
				{"x-request-id", "x-req-123"},
			}))
			require.Equal(t, "biz-req-123", getRequestID())
			host.CompleteHttp()
		})

		t.Run("returns x_request_id", func(t *testing.T) {
			host, status := test.NewTestHost(emptyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{"x-request-id", "x-req-123"},
			}))
			require.Equal(t, "x-req-123", getRequestID())
			host.CompleteHttp()
		})

		t.Run("falls back to legacy x-request-id header when property is unusable", func(t *testing.T) {
			host, status := test.NewTestHost(emptyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{"x-request-id", "header-req-456"},
			}))
			require.Equal(t, "header-req-456", getRequestID())
			host.CompleteHttp()
		})

		t.Run("trims whitespace", func(t *testing.T) {
			host, status := test.NewTestHost(emptyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{"x-request-id", " x-req-456 "},
			}))
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
