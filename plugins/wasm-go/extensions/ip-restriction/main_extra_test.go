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
	"github.com/tidwall/gjson"
)

// === helpers =============================================================

func mustConfigBytes(t *testing.T, m map[string]interface{}) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(m)
	require.NoError(t, err)
	return b
}

// === Module A — parseIPNets edges =======================================
//
// parseIPNets is 70.0% in baseline. utils_test.go exercises only the
// happy-path CIDR list and the empty-array case; both error branches
// (`AddByString` failure on bad input + `ErrNodeBusy` log-and-continue on
// duplicate) are unreached. They share the function's only error-handling
// chain at utils.go:23-29 so they must be pinned together.

// Duplicate IP entries → second `AddByString` returns nradix.ErrNodeBusy →
// the function logs the duplicate and continues, eventually returning a
// well-formed tree. Driven through ParseConfig+NewTestHost rather than
// calling parseIPNets directly because the function calls log.Warnf which
// panics without a host-initialized logger; a passing host start with
// duplicate allow entries proves the same contract.
func TestParseConfig_DuplicateAllowIP_StartsOK(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		cfg := mustConfigBytes(t, map[string]interface{}{
			"allow": []string{"10.0.0.1", "10.0.0.1"},
		})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		conf, err := host.GetMatchConfig()
		require.NoError(t, err)
		rc := conf.(*RestrictionConfig)
		require.NotNil(t, rc.Allow)
	})
}

// Bogus string (not an IP, not CIDR) → AddByString returns an error that
// is NOT ErrNodeBusy → function returns nil + wrapped error. Distinct from
// the duplicate case above: a structurally invalid entry is fatal, while
// a duplicate is tolerated. Direct call works because the non-busy error
// path does NOT log — it returns the wrapped error before any log call.
func TestParseIPNets_InvalidEntry(t *testing.T) {
	tree, err := parseIPNets(gjson.Parse(`["not-an-ip"]`).Array())
	require.Error(t, err)
	require.Nil(t, tree)
	require.Contains(t, err.Error(), "not-an-ip")
}

// === Module B — parseConfig edges =======================================
//
// parseConfig is 86.1%. Three uncovered branches:
//   - `default:` switch arm at main.go:52-54 — unknown ip_source_type value
//     falls back to OriginSourceType (distinct from the unset/empty path
//     covered by defaultConfig in main_test.go).
//   - allow parseIPNets error propagation at main.go:78-81.
//   - deny  parseIPNets error propagation at main.go:83-86.

// Unknown ip_source_type value (neither "header" nor "origin-source") must
// fall through to OriginSourceType — this is the safety default so a
// typo'd config doesn't accidentally trust an arbitrary header.
func TestParseConfig_UnknownSourceType_FallsToOrigin(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		cfg := mustConfigBytes(t, map[string]interface{}{
			"ip_source_type": "totally-unknown-mode",
			"allow":          []string{"127.0.0.1"},
		})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		conf, err := host.GetMatchConfig()
		require.NoError(t, err)
		rc := conf.(*RestrictionConfig)
		require.Equal(t, OriginSourceType, rc.IPSourceType)
	})
}

// Bad allow IP propagates parseIPNets's error → parseConfig returns the
// err → host plugin start status = Failed. Mirrors the dual contract on
// the deny side below.
func TestParseConfig_InvalidAllowIP_StartFails(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		cfg := mustConfigBytes(t, map[string]interface{}{
			"allow": []string{"not-an-ip"},
		})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusFailed, status)
	})
}

func TestParseConfig_InvalidDenyIP_StartFails(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		cfg := mustConfigBytes(t, map[string]interface{}{
			"deny": []string{"not-an-ip"},
		})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusFailed, status)
	})
}

// === Module C — getDownStreamIp / onHttpRequestHeaders error paths ======
//
// onHttpRequestHeaders is 77.8% and getDownStreamIp is 92.3%. Existing
// tests always either set source/address (origin mode) or pass the IP
// header (header mode). The "header missing" and "origin property
// missing" paths are unreached — both must funnel into the
// `deniedUnauthorized(get_ip_failed)` early return at main.go:126-128.

// Header source mode + IP header absent ⇒ GetHttpRequestHeader returns
// err → onHttpRequestHeaders 403s with reason "get_ip_failed". Distinct
// from "deny list - IP not denied" which sends a valid header.
func TestOnHttpRequestHeaders_HeaderSource_HeaderMissing_403(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// Use deny mode with header source. Don't pass X-Real-IP.
		cfg := mustConfigBytes(t, map[string]interface{}{
			"ip_source_type": "header",
			"ip_header_name": "X-Real-IP",
			"deny":           []string{"10.0.0.1"},
			"status":         403,
			"message":        "blocked",
		})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/test"},
			{":method", "GET"},
		})
		require.Equal(t, types.ActionContinue, action)

		resp := host.GetLocalResponse()
		require.NotNil(t, resp)
		require.Equal(t, uint32(403), resp.StatusCode)
		// Detail string is "key-auth.<reason>" from deniedUnauthorized.
		require.Contains(t, resp.StatusCodeDetail, "get_ip_failed")
	})
}

// === Module D — DefaultDenyMessage default constant ====================
//
// defaultConfig in main_test.go runs ParseConfig only; the default Status
// (403) and Message ("Your IP address is blocked.") are never observed
// through an actual deny path. Pin both via a deny verdict using a
// minimal config that omits status and message.
func TestOnHttpRequestHeaders_DefaultStatusAndMessage_OnDeny(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		cfg := mustConfigBytes(t, map[string]interface{}{
			"ip_source_type": "origin-source",
			"allow":          []string{"127.0.0.1"},
			// status + message intentionally omitted.
		})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		// IP NOT in allow list → blocked with default 403 + default msg.
		host.SetProperty([]string{"source", "address"}, []byte("8.8.8.8:1234"))
		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/test"},
			{":method", "GET"},
		})
		require.Equal(t, types.ActionContinue, action)

		resp := host.GetLocalResponse()
		require.NotNil(t, resp)
		require.Equal(t, uint32(DefaultDenyStatus), resp.StatusCode)
		var body map[string]string
		require.NoError(t, json.Unmarshal(resp.Data, &body))
		require.Equal(t, DefaultDenyMessage, body["message"])
	})
}
