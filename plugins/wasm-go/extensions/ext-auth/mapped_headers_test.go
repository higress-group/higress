// Copyright (c) 2024 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

// mappedHeadersConfig exercises body_json / header / status_code extraction with a
// rename, plus a missing path and an empty value that must not be injected.
var mappedHeadersConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"http_service": map[string]interface{}{
			"endpoint_mode": "envoy",
			"endpoint": map[string]interface{}{
				"service_name": "ext-auth.backend.svc.cluster.local",
				"service_port": 8090,
				"path_prefix":  "/auth",
			},
			"timeout": 1000,
			"authorization_response": map[string]interface{}{
				"mapped_upstream_headers": []map[string]interface{}{
					{"source": "body_json", "key": "data.uid", "to_header": "x-auth-user-id"},
					{"source": "header", "key": "x-user-token", "to_header": "x-auth-token"},
					{"source": "status_code", "to_header": "x-auth-status"},
					{"source": "body_json", "key": "data.absent", "to_header": "x-missing-not-injected"},
					{"source": "body_json", "key": "data.empty", "to_header": "x-empty-not-injected"},
				},
			},
		},
	})
	return data
}()

// mappedAndAllowedHeadersConfig configures both allowed_upstream_headers (same-name
// forwarding) and mapped_upstream_headers (extract and rename) to prove coexistence.
var mappedAndAllowedHeadersConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"http_service": map[string]interface{}{
			"endpoint_mode": "envoy",
			"endpoint": map[string]interface{}{
				"service_name": "ext-auth.backend.svc.cluster.local",
				"service_port": 8090,
				"path_prefix":  "/auth",
			},
			"timeout": 1000,
			"authorization_response": map[string]interface{}{
				"allowed_upstream_headers": []map[string]interface{}{
					{"exact": "x-user-id"},
				},
				"mapped_upstream_headers": []map[string]interface{}{
					{"source": "body_json", "key": "data.tenant", "to_header": "x-auth-tenant"},
				},
			},
		},
	})
	return data
}()

func headerMap(headers [][2]string) map[string]string {
	m := make(map[string]string, len(headers))
	for _, h := range headers {
		m[strings.ToLower(h[0])] = h[1]
	}
	return m
}

func TestMappedUpstreamHeadersInjection(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("extract and rename; missing and empty not injected", func(t *testing.T) {
			host, status := test.NewTestHost(mappedHeadersConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/users"},
				{":method", "POST"},
			})
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"x-user-token", "tok-abc"},
				{"content-type", "application/json"},
			}, []byte(`{"data": {"uid": "user-42", "empty": ""}}`))

			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			got := headerMap(host.GetRequestHeaders())
			require.Equal(t, "user-42", got["x-auth-user-id"], "body_json value injected under renamed header")
			require.Equal(t, "tok-abc", got["x-auth-token"], "header value injected under renamed header")
			require.Equal(t, "200", got["x-auth-status"], "status_code value injected under renamed header")
			require.NotContains(t, got, "x-missing-not-injected", "missing path must not be injected")
			require.NotContains(t, got, "x-empty-not-injected", "empty value must not be injected")

			host.CompleteHttp()
		})

		t.Run("coexists with allowed_upstream_headers", func(t *testing.T) {
			host, status := test.NewTestHost(mappedAndAllowedHeadersConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/users"},
				{":method", "POST"},
			})
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"x-user-id", "u-1"},
				{"content-type", "application/json"},
			}, []byte(`{"data": {"tenant": "acme"}}`))

			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			got := headerMap(host.GetRequestHeaders())
			require.Equal(t, "u-1", got["x-user-id"], "allowed_upstream_headers same-name forwarding still works")
			require.Equal(t, "acme", got["x-auth-tenant"], "mapped_upstream_headers rename injection works")

			host.CompleteHttp()
		})
	})
}
