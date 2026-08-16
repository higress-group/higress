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

// === helpers =============================================================

// mustConfig marshals m to JSON and fails the test on error.
func mustConfig(t *testing.T, m map[string]interface{}) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(m)
	require.NoError(t, err)
	return b
}

// routeRuleConfig produces a config with two consumers and a single _rules_ entry
// scoped to "route-a". globalAuth=nil leaves global_auth unset; allow controls
// who is permitted on route-a.
func routeRuleConfig(t *testing.T, globalAuth interface{}, allow []string) json.RawMessage {
	t.Helper()
	cfg := map[string]interface{}{
		"consumers": []map[string]interface{}{
			{"name": "consumer1", "credential": "token1"},
			{"name": "consumer2", "credential": "token2"},
		},
		"keys":      []string{"x-api-key"},
		"in_header": true,
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
//
// `contains` ships at 0% in the baseline; it is reachable only through the
// allow-list branches of onHttpRequestHeaders, which Module B drives. A direct
// unit test pins behavior independently in case the wasm dispatch path
// regresses or the helper is reused elsewhere (e.g. a future plugin pulling
// in the same util pattern, mirroring basic-auth's contains).

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

// === Module B — onHttpRequestHeaders allow-list branches ================
//
// Baseline onHttpRequestHeaders is 68.9%. Existing main_test.go only drives
// the early-exit and "no allow list" paths plus token-extraction failures.
// The four allow-list dispatches at main.go:344-363 — global_auth=true vs
// global_auth=false, consumer in vs not in allow — are uncovered. Each of
// these reaches `contains`, so this module also exercises the helper through
// its production caller.

// global_auth=true + route-scoped allow + consumer in allow → authenticated
// + X-Mse-Consumer header injected (main.go:344-351 success branch).
func TestOnHttpRequestHeaders_GlobalAuthTrue_RouteAllow_ConsumerAllowed(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(routeRuleConfig(t, true, []string{"consumer1"}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		require.NoError(t, host.SetRouteName("route-a"))

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/api/test"},
			{":method", "GET"},
			{"x-api-key", "token1"},
		})
		require.Equal(t, types.ActionContinue, action)
		require.Nil(t, host.GetLocalResponse())
		require.True(t, test.HasHeaderWithValue(host.GetRequestHeaders(), "X-Mse-Consumer", "consumer1"))
	})
}

// global_auth=true + route-scoped allow + consumer not in allow → 403 via
// deniedUnauthorizedConsumer (main.go:344-348). The credential decodes and
// authenticates against credential2Name — `consumer2` exists but is not
// permitted on route-a — distinct from the "credential not configured"
// variant already covered by main_test.go's "invalid api key" case.
func TestOnHttpRequestHeaders_GlobalAuthTrue_RouteAllow_ConsumerNotAllowed(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(routeRuleConfig(t, true, []string{"consumer1"}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		require.NoError(t, host.SetRouteName("route-a"))

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/api/test"},
			{":method", "GET"},
			{"x-api-key", "token2"},
		})
		require.Equal(t, types.ActionContinue, action)

		resp := host.GetLocalResponse()
		require.NotNil(t, resp)
		require.Equal(t, uint32(http.StatusForbidden), resp.StatusCode)
		require.True(t, test.HasHeader(resp.Headers, "WWW-Authenticate"))
	})
}

// global_auth=false + route-scoped allow + consumer in allow → authenticated
// path through the case-3 branch (main.go:354-362 success). Verifies
// X-Mse-Consumer is still injected when auth is enabled per-route only.
func TestOnHttpRequestHeaders_GlobalAuthFalse_RouteAllow_ConsumerAllowed(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(routeRuleConfig(t, false, []string{"consumer1"}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		require.NoError(t, host.SetRouteName("route-a"))

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/api/test"},
			{":method", "GET"},
			{"x-api-key", "token1"},
		})
		require.Equal(t, types.ActionContinue, action)
		require.Nil(t, host.GetLocalResponse())
		require.True(t, test.HasHeaderWithValue(host.GetRequestHeaders(), "X-Mse-Consumer", "consumer1"))
	})
}

// global_auth=false + route-scoped allow + consumer not in allow → 403 via
// deniedUnauthorizedConsumer (main.go:354-359 reject). Mirror of the
// global_auth=true rejection but exercises the case-3 entry condition
// `(globalAuthSetFalse || (globalAuthNoSet && ruleSet)) && !noAllow`.
func TestOnHttpRequestHeaders_GlobalAuthFalse_RouteAllow_ConsumerNotAllowed(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(routeRuleConfig(t, false, []string{"consumer1"}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		require.NoError(t, host.SetRouteName("route-a"))

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/api/test"},
			{":method", "GET"},
			{"x-api-key", "token2"},
		})
		require.Equal(t, types.ActionContinue, action)

		resp := host.GetLocalResponse()
		require.NotNil(t, resp)
		require.Equal(t, uint32(http.StatusForbidden), resp.StatusCode)
	})
}

// global_auth unset + at least one route configured + current route NOT
// configured → noAllow short-circuit through `(globalAuthNoSet && ruleSet)`
// at main.go:288-293. Existing tests use global_auth=false for this branch;
// this drives the unset path so the boolean expression's other operand is
// covered.
func TestOnHttpRequestHeaders_GlobalAuthUnset_RuleSet_OtherRoute_PassThrough(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// global_auth omitted; _rules_ entry on route-a only.
		host, status := test.NewTestHost(routeRuleConfig(t, nil, []string{"consumer1"}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		// Drive a request on a different route — current rule context has no
		// allow list, so auth must be skipped entirely.
		require.NoError(t, host.SetRouteName("route-b"))

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/api/test"},
			{":method", "GET"},
		})
		require.Equal(t, types.ActionContinue, action)
		require.Nil(t, host.GetLocalResponse())
	})
}

// === Module C — parse-time edge rejects =================================
//
// parseGlobalConfig is 93.2% and parseOverrideRuleConfig is 90.9%. The
// missing branches are the empty-string credential rejects (singular and
// inside the credentials array) and the "allow key absent entirely" branch
// at parseOverrideRuleConfig:251 which existing tests don't reach because
// every route-rule fixture supplies `allow` (possibly empty).

// credential: "" must be rejected at parseGlobalConfig:206-208 — distinct
// from "credential field absent" already covered by mixed-credential
// fixtures in main_test.go.
func TestParseGlobalConfig_EmptyCredentialString(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		cfg := mustConfig(t, map[string]interface{}{
			"consumers": []map[string]interface{}{
				{"name": "consumer1", "credential": ""},
			},
			"keys":      []string{"x-api-key"},
			"in_header": true,
		})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusFailed, status)
	})
}

// An empty string inside the credentials array must be rejected at
// parseGlobalConfig:215-217 — separate branch from the "credentials array
// empty" reject already covered by invalidEmptyPluralCredentialsConfig.
func TestParseGlobalConfig_EmptyCredentialInArray(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		cfg := mustConfig(t, map[string]interface{}{
			"consumers": []map[string]interface{}{
				{"name": "consumer1", "credentials": []string{"token1", ""}},
			},
			"keys":      []string{"x-api-key"},
			"in_header": true,
		})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusFailed, status)
	})
}

// _rules_ entry without an `allow` key at all — distinct from
// invalidRuleConfig which supplies `allow: []`. Hits the
// `if !allow.Exists()` branch at parseOverrideRuleConfig:251.
func TestParseOverrideRuleConfig_AllowMissing(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		cfg := mustConfig(t, map[string]interface{}{
			"consumers": []map[string]interface{}{
				{"name": "consumer1", "credential": "token1"},
			},
			"keys":      []string{"x-api-key"},
			"in_header": true,
			"_rules_": []map[string]interface{}{
				{
					"_match_route_": []string{"route-a"},
					// no "allow" key at all
				},
			},
		})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusFailed, status)
	})
}
