package main

import (
	"encoding/json"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

var headerMatchConfig = json.RawMessage(`{
	"http_service": {
		"endpoint_mode": "envoy",
		"endpoint": {
			"service_name": "ext-auth.backend.svc.cluster.local",
			"service_port": 8090,
			"path_prefix": "/auth"
		},
		"authorization_request": {"with_request_body": true}
	},
	"match_type": "blacklist",
	"match_list": [{
		"match_rule_headers": [{"name": "X-Custom-Auth", "exists": true}]
	}]
}`)

func newHeaderMatchTestHost(t *testing.T) test.TestHost {
	t.Helper()
	host, status := test.NewTestHost(headerMatchConfig)
	require.Equal(t, types.OnPluginStartStatusOK, status)
	t.Cleanup(host.Reset)
	return host
}

func TestHeaderPresenceMatchRequestFlow(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("missing header bypasses before body buffering and callout", func(t *testing.T) {
			host := newHeaderMatchTestHost(t)
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"}, {":path", "/users"}, {":method", "POST"}, {"content-length", "7"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.Empty(t, host.GetHttpCalloutAttributes())
			host.CompleteHttp()
		})

		t.Run("present empty header executes authorization", func(t *testing.T) {
			host := newHeaderMatchTestHost(t)
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"}, {":path", "/users"}, {":method", "POST"}, {"x-custom-auth", ""},
			})

			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)
			require.Len(t, host.GetHttpCalloutAttributes(), 1)
			host.CompleteHttp()
		})
	})
}
