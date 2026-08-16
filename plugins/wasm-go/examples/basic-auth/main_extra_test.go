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
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

// === helpers =============================================================

// mustConfig marshals m to JSON and fails the test on error.
func mustConfig(t *testing.T, m map[string]interface{}) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(m)
	require.NoError(t, err)
	return b
}

// basicAuthHeader returns an Authorization header pair carrying base64-encoded
// "user:pwd". Cuts boilerplate across module B/C credential cases.
func basicAuthHeader(user, pwd string) [2]string {
	enc := base64.StdEncoding.EncodeToString([]byte(user + ":" + pwd))
	return [2]string{"authorization", "Basic " + enc}
}

// ruleConfig produces a config with two consumers and a single _rules_ entry
// scoped to "route-a"; pass globalAuth as nil to leave the field unset.
func ruleConfig(t *testing.T, globalAuth interface{}, allow []string) json.RawMessage {
	t.Helper()
	cfg := map[string]interface{}{
		"consumers": []map[string]interface{}{
			{"name": "consumer1", "credential": "admin:123456"},
			{"name": "consumer2", "credential": "guest:abc"},
		},
		"_rules_": []map[string]interface{}{
			{
				"_match_route_": []string{"route-a"},
				"allow":         allow,
			},
		},
	}
	if globalAuth != nil {
		cfg["global_auth"] = globalAuth
	}
	return mustConfig(t, cfg)
}

// === Module A — contains helper =========================================

// contains is a tiny package-private helper but ships at 0% in the baseline
// coverage; the wasm entry path that calls it is exercised only through the
// allow-list branches of onHttpRequestHeaders, which themselves are not
// covered (Module B fixes that). A direct unit test pins behavior in case
// the wasm path regresses or the helper is reused elsewhere.

func TestContains_Hit(t *testing.T) {
	require.True(t, contains([]string{"a", "b", "c"}, "b"))
}

func TestContains_Miss(t *testing.T) {
	require.False(t, contains([]string{"a", "b", "c"}, "z"))
}

func TestContains_EmptySlice(t *testing.T) {
	require.False(t, contains([]string{}, "x"))
}

func TestContains_NilSlice(t *testing.T) {
	require.False(t, contains(nil, "x"))
}

// === Module B — denied-unauthorized-consumer paths ======================
//
// deniedUnauthorizedConsumer is 0% in baseline coverage. It is reached when
// credentials decode and authenticate, but the consumer's name is missing
// from a route-scoped allow list. Documented at main.go:310-314 (RFC 7617
// §2 "realm" + Higress 403 contract). Both global_auth=true and
// global_auth=false branches converge on the same denial helper.

func TestOnHttpRequestHeaders_GlobalAuthTrue_ConsumerNotAllowed(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(ruleConfig(t, true, []string{"consumer1"}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		require.NoError(t, host.SetRouteName("route-a"))

		// consumer2 (guest:abc) authenticates successfully but is not in allow.
		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/api/test"},
			{":method", "GET"},
			basicAuthHeader("guest", "abc"),
		})
		require.Equal(t, types.ActionContinue, action)

		resp := host.GetLocalResponse()
		require.NotNil(t, resp)
		require.Equal(t, uint32(http.StatusForbidden), resp.StatusCode)
		require.True(t, test.HasHeader(resp.Headers, "WWW-Authenticate"))
	})
}

func TestOnHttpRequestHeaders_GlobalAuthFalse_ConsumerNotAllowed(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(ruleConfig(t, false, []string{"consumer1"}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		require.NoError(t, host.SetRouteName("route-a"))

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/api/test"},
			{":method", "GET"},
			basicAuthHeader("guest", "abc"),
		})
		require.Equal(t, types.ActionContinue, action)

		resp := host.GetLocalResponse()
		require.NotNil(t, resp)
		require.Equal(t, uint32(http.StatusForbidden), resp.StatusCode)
	})
}

// === Module C — onHttpRequestHeaders branch coverage ====================
//
// Baseline onHttpRequestHeaders is 66.0% and only covers the early-exit and
// "no allow list / no rule" branches. Sub-cases below drive each remaining
// branch documented in the comment block at main.go:182-193:
// authenticated case 2 (global_auth=true with allow), authenticated case 3
// (global_auth=false with allow), no-rules + globalAuth-unset path, and the
// three failure helpers that produce 401 (no auth data / decode error / bad
// format) under global_auth=true (so the early-exit guard does not fire).

// authenticated case 2: global_auth=true + route-scoped allow + consumer in allow
func TestOnHttpRequestHeaders_GlobalAuthTrue_ConsumerAllowed(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(ruleConfig(t, true, []string{"consumer1"}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		require.NoError(t, host.SetRouteName("route-a"))

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/api/test"},
			{":method", "GET"},
			basicAuthHeader("admin", "123456"),
		})
		require.Equal(t, types.ActionContinue, action)
		require.Nil(t, host.GetLocalResponse())
		require.True(t, test.HasHeaderWithValue(host.GetRequestHeaders(), "X-Mse-Consumer", "consumer1"))
	})
}

// authenticated case 3: global_auth=false + route-scoped allow + consumer in allow
func TestOnHttpRequestHeaders_GlobalAuthFalse_ConsumerAllowed(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(ruleConfig(t, false, []string{"consumer1"}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		require.NoError(t, host.SetRouteName("route-a"))

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/api/test"},
			{":method", "GET"},
			basicAuthHeader("admin", "123456"),
		})
		require.Equal(t, types.ActionContinue, action)
		require.Nil(t, host.GetLocalResponse())
		require.True(t, test.HasHeaderWithValue(host.GetRequestHeaders(), "X-Mse-Consumer", "consumer1"))
	})
}

// authenticated case 1 fallback: global_auth unset, no rules anywhere, valid
// credentials → consumer authenticated and X-Mse-Consumer injected.
func TestOnHttpRequestHeaders_GlobalAuthUnset_NoRules_Authenticated(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		cfg := mustConfig(t, map[string]interface{}{
			"consumers": []map[string]interface{}{
				{"name": "consumer1", "credential": "admin:123456"},
			},
		})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/api/test"},
			{":method", "GET"},
			basicAuthHeader("admin", "123456"),
		})
		require.Equal(t, types.ActionContinue, action)
		require.Nil(t, host.GetLocalResponse())
		require.True(t, test.HasHeaderWithValue(host.GetRequestHeaders(), "X-Mse-Consumer", "consumer1"))
	})
}

// missing Authorization header under global_auth=true must hit
// deniedNoBasicAuthData (401).
func TestOnHttpRequestHeaders_GlobalAuthTrue_MissingAuthorization(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(ruleConfig(t, true, []string{"consumer1"}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		require.NoError(t, host.SetRouteName("route-a"))

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/api/test"},
			{":method", "GET"},
		})
		require.Equal(t, types.ActionContinue, action)

		resp := host.GetLocalResponse()
		require.NotNil(t, resp)
		require.Equal(t, uint32(http.StatusUnauthorized), resp.StatusCode)
	})
}

// invalid base64 payload (not just a missing prefix) — exercises the decode
// error branch which is distinct from the format-prefix branch already
// covered by main_test.go.
func TestOnHttpRequestHeaders_GlobalAuthTrue_DecodeError(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(ruleConfig(t, true, []string{"consumer1"}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		require.NoError(t, host.SetRouteName("route-a"))

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/api/test"},
			{":method", "GET"},
			{"authorization", "Basic !!!notbase64"},
		})
		require.Equal(t, types.ActionContinue, action)

		resp := host.GetLocalResponse()
		require.NotNil(t, resp)
		require.Equal(t, uint32(http.StatusUnauthorized), resp.StatusCode)
	})
}

// payload decodes but does not split on ':' — exercises the
// "len(userAndPasswd) != 2" branch which has different status-code-detail
// "basic-auth.bad_credential" semantics from the no-data branch.
func TestOnHttpRequestHeaders_GlobalAuthTrue_BadCredentialFormat(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(ruleConfig(t, true, []string{"consumer1"}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		require.NoError(t, host.SetRouteName("route-a"))

		// "adminonly" decodes successfully but contains no colon.
		bad := base64.StdEncoding.EncodeToString([]byte("adminonly"))
		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/api/test"},
			{":method", "GET"},
			{"authorization", "Basic " + bad},
		})
		require.Equal(t, types.ActionContinue, action)

		resp := host.GetLocalResponse()
		require.NotNil(t, resp)
		require.Equal(t, uint32(http.StatusUnauthorized), resp.StatusCode)
	})
}
