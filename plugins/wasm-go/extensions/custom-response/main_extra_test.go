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
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

// === helpers =============================================================

func mustConfigBytes(t *testing.T, m map[string]interface{}) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(m)
	require.NoError(t, err)
	return b
}

// === Module A — fuzzyMatchCode early-exit and skip branches =============
//
// fuzzyMatchCode is 88.2% in baseline. Test_prefixMatchCode in main_test.go
// drives the matching matrix but never calls the function with an empty
// map, an empty status code, a length-mismatched code, or a pure-numeric
// pattern that must be skipped via the `!strings.Contains(pattern, "x")`
// guard at main.go:234-236.

func TestFuzzyMatchCode_EmptyMap(t *testing.T) {
	rule, ok := fuzzyMatchCode(map[string]*CustomResponseRule{}, "404")
	require.False(t, ok)
	require.Nil(t, rule)
}

func TestFuzzyMatchCode_EmptyStatusCode(t *testing.T) {
	m := map[string]*CustomResponseRule{"4xx": {}}
	rule, ok := fuzzyMatchCode(m, "")
	require.False(t, ok)
	require.Nil(t, rule)
}

// Code is longer than every pattern in the map ⇒ `len(pattern) != codeLen`
// is true on every iter → continue → no match. Pins the per-iter length
// guard at main.go:230-232 (the existing tests use 3-char codes against a
// 3-char-only map so the guard's body never fires).
func TestFuzzyMatchCode_LengthMismatch(t *testing.T) {
	m := map[string]*CustomResponseRule{"4xx": {}, "5xx": {}}
	rule, ok := fuzzyMatchCode(m, "4040")
	require.False(t, ok)
	require.Nil(t, rule)
}

// Pure-numeric pattern (no `x`) is skipped — pins the
// `!strings.Contains(pattern, "x")` short-circuit. Without the skip the
// fuzzy block would shadow the upstream exact-match dispatch.
func TestFuzzyMatchCode_PureNumberPatternSkipped(t *testing.T) {
	m := map[string]*CustomResponseRule{
		"500": {}, // pure-numeric — must be skipped by fuzzy logic
		"4xx": {}, // x-containing — would match if reached
	}
	// "500" matches the first map entry exactly (in length) but fuzzy
	// logic skips numeric patterns; "4xx" doesn't match "500"; result:
	// no fuzzy match.
	rule, ok := fuzzyMatchCode(m, "500")
	require.False(t, ok)
	require.Nil(t, rule)
}

// === Module B — parseRuleItem error paths ===============================
//
// parseRuleItem is 92.3%. Three uncovered branches:
//   - content-length header skip at main.go:105-106
//   - header without `=` returning error at main.go:110-112
//   - status_code value not parseable as int at main.go:128-131

// Driven through ParseConfig + NewTestHost so the wasm logger is set up.
// Old-style (single-rule) config with content-length in headers — the
// header must be silently dropped, NOT round-tripped, since that field is
// recomputed by the proxy.
func TestParseRuleItem_ContentLengthHeaderSkipped(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		cfg := mustConfigBytes(t, map[string]interface{}{
			"status_code": 200,
			"headers": []string{
				"Content-Length=999", // must be dropped
				"X-Other=keep",
			},
			"body": "ok",
		})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		conf, err := host.GetMatchConfig()
		require.NoError(t, err)
		rc := conf.(*CustomResponseConfig)
		require.NotNil(t, rc.defaultRule)
		// content-length must NOT appear in the rule's headers.
		for _, h := range rc.defaultRule.headers {
			require.NotEqual(t, "Content-Length", h[0])
			require.NotEqual(t, "content-length", h[0])
		}
	})
}

// Header missing `=` ⇒ parseRuleItem returns "invalid header pair format"
// → parseConfig propagates → plugin start fails.
func TestParseRuleItem_HeaderMissingEquals_StartFails(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		cfg := mustConfigBytes(t, map[string]interface{}{
			"status_code": 200,
			"headers":     []string{"no_equals_sign"},
			"body":        "x",
		})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusFailed, status)
	})
}

// status_code that doesn't parse as int ⇒ strconv.Atoi error →
// parseRuleItem returns wrapped error → start fails.
func TestParseRuleItem_InvalidStatusCode_StartFails(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		cfg := mustConfigBytes(t, map[string]interface{}{
			"status_code": "not-a-number",
			"body":        "x",
		})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusFailed, status)
	})
}

// === Module C — parseConfig error paths =================================
//
// parseConfig is 84.0%. Three reachable uncovered branches:
//   - rules-array path: parseRuleItem error propagation at main.go:62-64
//     (the existing `invalidConfig` exercises ONLY single-rule path; the
//     array form's error propagation is distinct)
//   - duplicate enable_on_status across rules at main.go:82-85
//   - rules-version with no defaultRule and empty enableOnStatusRuleMap
//     at main.go:89-91 — reachable only via empty rules array

// Two rules sharing the same exact enable_on_status entry must fail with
// "enableOnStatus can only use once" — pins single-rule-per-status
// uniqueness invariant.
func TestParseConfig_DuplicateEnableOnStatus_StartFails(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		cfg := mustConfigBytes(t, map[string]interface{}{
			"rules": []map[string]interface{}{
				{
					"status_code":      200,
					"body":             "a",
					"enable_on_status": []string{"404"},
				},
				{
					"status_code":      200,
					"body":             "b",
					"enable_on_status": []string{"404"}, // duplicate
				},
			},
		})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusFailed, status)
	})
}

// One rule in the array has a malformed header ⇒ parseRuleItem returns
// err → parseConfig propagates from the rules-array branch (distinct from
// the single-rule branch already covered by `invalidConfig`).
func TestParseConfig_RuleInArrayInvalid_StartFails(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		cfg := mustConfigBytes(t, map[string]interface{}{
			"rules": []map[string]interface{}{
				{
					"status_code":      200,
					"headers":          []string{"no_equals"},
					"body":             "x",
					"enable_on_status": []string{"200"},
				},
			},
		})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusFailed, status)
	})
}

// Rules array is present but empty ⇒ rulesVersion=true, no rules iterated,
// defaultRule stays nil, enableOnStatusRuleMap stays empty → returns
// "no valid config is found".
func TestParseConfig_EmptyRulesArray_StartFails(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		cfg := mustConfigBytes(t, map[string]interface{}{
			"rules": []map[string]interface{}{},
		})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusFailed, status)
	})
}

// === Module D — onHttpResponseHeaders missing :status ===================
//
// onHttpResponseHeaders is 73.3%. The early-exit at main.go:200-204 —
// `:status` retrieval failure → log + ActionContinue — is unreached
// because every existing fixture passes a `:status` header. This is the
// fail-soft contract under malformed upstream responses.
func TestOnHttpResponseHeaders_MissingStatusHeader_PassThrough(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(statusMatchConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/test"},
			{":method", "GET"},
		})

		// No :status in response headers.
		action := host.CallOnHttpResponseHeaders([][2]string{
			{"content-type", "text/plain"},
		})
		require.Equal(t, types.ActionContinue, action)
		// No local response sent — the rule path didn't fire because the
		// status retrieval failed first.
		require.Nil(t, host.GetLocalResponse())
	})
}
