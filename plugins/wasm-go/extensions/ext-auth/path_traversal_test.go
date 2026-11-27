// Copyright (c) 2025 Alibaba Group Holding Ltd.
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
	"net/http"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

// envoyConfig builds an envoy-mode config for a given path_prefix.
func envoyConfig(pathPrefix string, failureModeAllow bool) json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"http_service": map[string]interface{}{
			"endpoint_mode": "envoy",
			"endpoint": map[string]interface{}{
				"service_name": "ext-auth.backend.svc.cluster.local",
				"service_port": 8090,
				"path_prefix":  pathPrefix,
			},
			"timeout": 1000,
		},
		"failure_mode_allow": failureModeAllow,
	})
	return data
}

// A path_prefix without a leading slash passes config validation but leaves
// containment undecidable at request time.
var relativePrefixEnvoyConfig = envoyConfig("auth", false)

// path.Clean collapses this prefix to "/", which every absolute path is inside
// of, so containment would pass for any request path.
var collapsingPrefixEnvoyConfig = envoyConfig("/auth/..", false)

// failure_mode_allow accepts a request whose authorization call failed, so this
// is the configuration a malformed path must not be able to reach.
var failureModeAllowRootPrefixEnvoyConfig = envoyConfig("/", true)

// In envoy endpoint mode the authorization path is built from the raw :path
// header, which carries the query string. Joining it whole let dot segments
// inside a query value resolve as path segments and escape path_prefix.
func TestOnHttpRequestHeadersEnvoyModePathTraversal(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("dot segments in a query value never reach the joined path", func(t *testing.T) {
			host, status := test.NewTestHost(basicEnvoyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/users?next=/../../../admin/keys"},
				{":method", "GET"},
			})
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)
			require.Nil(t, host.GetLocalResponse())

			callouts := host.GetHttpCalloutAttributes()
			require.Len(t, callouts, 1)
			require.True(t,
				test.HasHeaderWithValue(callouts[0].Headers, ":path", "/auth/users?next=/../../../admin/keys"),
				"authorization callout headers = %v", callouts[0].Headers)

			host.CallOnHttpCall([][2]string{{":status", "200"}}, []byte(`{"authorized": true}`))
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())
			host.CompleteHttp()
		})

		t.Run("query is preserved verbatim for a clean path", func(t *testing.T) {
			host, status := test.NewTestHost(basicEnvoyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/users?a=1&b=2"},
				{":method", "GET"},
			})
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			callouts := host.GetHttpCalloutAttributes()
			require.Len(t, callouts, 1)
			require.True(t,
				test.HasHeaderWithValue(callouts[0].Headers, ":path", "/auth/users?a=1&b=2"),
				"authorization callout headers = %v", callouts[0].Headers)

			host.CallOnHttpCall([][2]string{{":status", "200"}}, []byte(`{"authorized": true}`))
			host.CompleteHttp()
		})

		t.Run("dot segments escaping path_prefix are rejected without calling out", func(t *testing.T) {
			host, status := test.NewTestHost(basicEnvoyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/users/../../admin/keys"},
				{":method", "GET"},
			})
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action, "a rejected request must stay paused")

			require.Empty(t, host.GetHttpCalloutAttributes(), "the authorization server must never see an escaped path")
			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			require.Equal(t, uint32(http.StatusForbidden), localResponse.StatusCode)
			require.Equal(t, "ext-auth.path_traversal", localResponse.StatusCodeDetail)
		})

		t.Run("dot segments inside a dot segment path are rejected even with a query", func(t *testing.T) {
			host, status := test.NewTestHost(basicEnvoyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/../admin/keys?x=1"},
				{":method", "GET"},
			})
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)
			require.Empty(t, host.GetHttpCalloutAttributes())
			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			require.Equal(t, uint32(http.StatusForbidden), localResponse.StatusCode)
		})

		t.Run("percent encoded dot dot in the path is rejected", func(t *testing.T) {
			host, status := test.NewTestHost(basicEnvoyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// wrapper.HttpCall dispatches url.Parse(...).Path, which is percent-decoded,
			// so %2E%2E reaches the authorization server as "..".
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/%2E%2E/admin/keys"},
				{":method", "GET"},
			})
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)
			require.Empty(t, host.GetHttpCalloutAttributes())
			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			require.Equal(t, uint32(http.StatusForbidden), localResponse.StatusCode)
			require.Equal(t, "ext-auth.path_traversal", localResponse.StatusCodeDetail)
		})

		t.Run("percent encoded dots in the query stay verbatim", func(t *testing.T) {
			host, status := test.NewTestHost(basicEnvoyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/users?next=%2E%2E%2F%2E%2E%2Fadmin"},
				{":method", "GET"},
			})
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			callouts := host.GetHttpCalloutAttributes()
			require.Len(t, callouts, 1)
			require.True(t,
				test.HasHeaderWithValue(callouts[0].Headers, ":path", "/auth/users?next=%2E%2E%2F%2E%2E%2Fadmin"),
				"authorization callout headers = %v", callouts[0].Headers)

			host.CallOnHttpCall([][2]string{{":status", "200"}}, []byte(`{"authorized": true}`))
			host.CompleteHttp()
		})

		t.Run("an undecodable percent escape is rejected instead of skipping the decoded check", func(t *testing.T) {
			host, status := test.NewTestHost(basicEnvoyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// url.PathUnescape fails atomically on %zz. Falling back to the raw path
			// only would let the %2E%2E next to it reach the authorization server,
			// which decodes it to ".." and resolves /admin/keys.
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/%zz%2E%2E/admin/keys"},
				{":method", "GET"},
			})
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)
			require.Empty(t, host.GetHttpCalloutAttributes())
			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			require.Equal(t, uint32(http.StatusForbidden), localResponse.StatusCode)
			require.Equal(t, "ext-auth.path_traversal", localResponse.StatusCodeDetail)
		})

		t.Run("a relative path_prefix fails closed", func(t *testing.T) {
			host, status := test.NewTestHost(relativePrefixEnvoyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// Containment is undecidable without a root: path_prefix "auth" would
			// produce the relative :path "auth/users", which is not a valid request
			// target. Rejecting beats forwarding a path nothing can be checked against.
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/users"},
				{":method", "GET"},
			})
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)
			require.Empty(t, host.GetHttpCalloutAttributes())
			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			require.Equal(t, uint32(http.StatusForbidden), localResponse.StatusCode)
			require.Equal(t, "ext-auth.path_traversal", localResponse.StatusCodeDetail)
		})

		t.Run("a raw backslash in the request path is rejected", func(t *testing.T) {
			host, status := test.NewTestHost(basicEnvoyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// path.Clean reads '\' as an ordinary byte, so this stays inside
			// path_prefix as the single segment `x\..\..\admin`. Envoy rewrites %5C
			// but never a raw backslash, and a consumer that does read it as a
			// separator would resolve /admin instead.
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", `/x\..\..\admin`},
				{":method", "GET"},
			})
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)
			require.Empty(t, host.GetHttpCalloutAttributes())
			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			require.Equal(t, uint32(http.StatusForbidden), localResponse.StatusCode)
			require.Equal(t, "ext-auth.path_traversal", localResponse.StatusCodeDetail)
		})

		t.Run("a backslash in the query is forwarded verbatim", func(t *testing.T) {
			host, status := test.NewTestHost(basicEnvoyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", `/users?file=a\b`},
				{":method", "GET"},
			})
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			callouts := host.GetHttpCalloutAttributes()
			require.Len(t, callouts, 1)
			require.True(t,
				test.HasHeaderWithValue(callouts[0].Headers, ":path", `/auth/users?file=a\b`),
				"authorization callout headers = %v", callouts[0].Headers)

			host.CallOnHttpCall([][2]string{{":status", "200"}}, []byte(`{"authorized": true}`))
			host.CompleteHttp()
		})

		t.Run("a path_prefix that collapses to root fails closed", func(t *testing.T) {
			host, status := test.NewTestHost(collapsingPrefixEnvoyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// path.Clean("/auth/..") is "/", so every request path would pass
			// containment while the operator configured a narrower prefix.
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/users"},
				{":method", "GET"},
			})
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)
			require.Empty(t, host.GetHttpCalloutAttributes())
			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			require.Equal(t, uint32(http.StatusForbidden), localResponse.StatusCode)
			require.Equal(t, "ext-auth.path_traversal", localResponse.StatusCodeDetail)
		})

		t.Run("an undispatchable escape is rejected even with failure_mode_allow", func(t *testing.T) {
			host, status := test.NewTestHost(failureModeAllowRootPrefixEnvoyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// Nothing can escape a root path_prefix, so pathutil forwards "%zz"
			// unchanged. wrapper.HttpCall then fails to parse the path and reports a
			// call error, which failure_mode_allow would accept: the request would
			// never be authorized. Rejecting it here keeps the malformed path off
			// that route.
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/users/%zz"},
				{":method", "GET"},
			})
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)
			require.Empty(t, host.GetHttpCalloutAttributes())
			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			require.Equal(t, uint32(http.StatusForbidden), localResponse.StatusCode)
			require.Equal(t, "ext-auth.path_traversal", localResponse.StatusCodeDetail)
		})
	})
}
