// Copyright (c) 2022 Alibaba Group Holding Ltd.
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

func TestSetsCSPHeader(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		config, _ := json.Marshal(map[string]any{
			"policy": "default-src 'self'; img-src *",
		})
		host, status := test.NewTestHost(config)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		action := host.CallOnHttpResponseHeaders([][2]string{
			{":status", "200"},
			{"content-type", "text/html"},
		})
		require.Equal(t, types.ActionContinue, action)

		v, ok := test.GetHeaderValue(host.GetResponseHeaders(), headerCSP)
		require.True(t, ok, "Content-Security-Policy header should be set")
		require.Equal(t, "default-src 'self'; img-src *", v)
	})
}

func TestReportOnlyHeader(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		config, _ := json.Marshal(map[string]any{
			"policy":      "default-src 'self'",
			"report_only": true,
		})
		host, status := test.NewTestHost(config)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		host.CallOnHttpResponseHeaders([][2]string{{":status", "200"}})

		headers := host.GetResponseHeaders()
		v, ok := test.GetHeaderValue(headers, headerCSPReportOnly)
		require.True(t, ok, "report-only header should be set")
		require.Equal(t, "default-src 'self'", v)

		require.False(t, test.HasHeader(headers, headerCSP),
			"enforcing header should not be set in report-only mode")
	})
}

func TestReportOnlyRemovesUpstreamEnforce(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		config, _ := json.Marshal(map[string]any{
			"policy":      "default-src 'self'",
			"report_only": true,
		})
		host, status := test.NewTestHost(config)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		host.CallOnHttpResponseHeaders([][2]string{
			{":status", "200"},
			{headerCSP, "default-src https://upstream.example"},
		})

		headers := host.GetResponseHeaders()
		v, ok := test.GetHeaderValue(headers, headerCSPReportOnly)
		require.True(t, ok, "report-only header should be set")
		require.Equal(t, "default-src 'self'", v)

		require.False(t, test.HasHeader(headers, headerCSP),
			"override must drop the upstream enforcing CSP in report-only mode")
	})
}

func TestOverrideReplacesUpstreamEnforce(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		config, _ := json.Marshal(map[string]any{
			"policy": "default-src 'self'",
		})
		host, status := test.NewTestHost(config)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		host.CallOnHttpResponseHeaders([][2]string{
			{":status", "200"},
			{headerCSP, "default-src https://upstream.example"},
		})

		v, ok := test.GetHeaderValue(host.GetResponseHeaders(), headerCSP)
		require.True(t, ok)
		require.Equal(t, "default-src 'self'", v,
			"upstream CSP must be replaced by the configured policy")
	})
}

func TestOverrideFalseKeepsExisting(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		config, _ := json.Marshal(map[string]any{
			"policy":   "default-src 'self'",
			"override": false,
		})
		host, status := test.NewTestHost(config)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		host.CallOnHttpResponseHeaders([][2]string{
			{":status", "200"},
			{headerCSP, "default-src https://upstream.example"},
		})

		v, ok := test.GetHeaderValue(host.GetResponseHeaders(), headerCSP)
		require.True(t, ok)
		require.Equal(t, "default-src https://upstream.example", v,
			"existing CSP header must be kept when override is false")
	})
}

func TestOverrideFalseEmptyValueTreatedAbsent(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		config, _ := json.Marshal(map[string]any{
			"policy":   "default-src 'self'",
			"override": false,
		})
		host, status := test.NewTestHost(config)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		// Mock host reports empty-value headers as not-found → injected here.
		// Real Envoy returns Ok+empty → treated as present, kept.
		host.CallOnHttpResponseHeaders([][2]string{
			{":status", "200"},
			{headerCSP, ""},
		})

		require.True(t,
			test.HasHeaderWithValue(host.GetResponseHeaders(), headerCSP, "default-src 'self'"),
			"the mock host surfaces empty-value headers as not-found, so the policy is injected")
	})
}

func TestOverrideFalseDualPolicyYieldsPerVariant(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		config, _ := json.Marshal(map[string]any{
			"policy":             "default-src 'self'",
			"report_only_policy": "default-src 'none'",
			"override":           false,
		})
		host, status := test.NewTestHost(config)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		host.CallOnHttpResponseHeaders([][2]string{
			{":status", "200"},
			{headerCSP, "default-src https://upstream.example"},
		})

		headers := host.GetResponseHeaders()
		v, ok := test.GetHeaderValue(headers, headerCSP)
		require.True(t, ok)
		require.Equal(t, "default-src https://upstream.example", v,
			"existing enforcing header must be kept when override is false")

		ro, ok := test.GetHeaderValue(headers, headerCSPReportOnly)
		require.True(t, ok, "the report-only candidate must still be injected")
		require.Equal(t, "default-src 'none'", ro)
	})
}

func TestOverrideFalseAddsWhenAbsent(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		config, _ := json.Marshal(map[string]any{
			"policy":   "default-src 'self'",
			"override": false,
		})
		host, status := test.NewTestHost(config)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		host.CallOnHttpResponseHeaders([][2]string{{":status", "200"}})

		v, ok := test.GetHeaderValue(host.GetResponseHeaders(), headerCSP)
		require.True(t, ok, "policy must be injected when no upstream CSP header exists")
		require.Equal(t, "default-src 'self'", v)
	})
}

func TestOverrideNullDefaultsTrue(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		config := []byte(`{"policy": "default-src 'self'", "override": null}`)
		host, status := test.NewTestHost(config)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		host.CallOnHttpResponseHeaders([][2]string{
			{":status", "200"},
			{headerCSP, "default-src https://upstream.example"},
		})

		v, ok := test.GetHeaderValue(host.GetResponseHeaders(), headerCSP)
		require.True(t, ok)
		require.Equal(t, "default-src 'self'", v,
			"explicit null override must fall back to the default (true)")
	})
}

func TestDualPolicyEmitsBoth(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		config, _ := json.Marshal(map[string]any{
			"policy":             "default-src 'self'",
			"report_only_policy": "default-src 'self'; require-trusted-types-for 'script'",
		})
		host, status := test.NewTestHost(config)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		host.CallOnHttpResponseHeaders([][2]string{{":status", "200"}})

		headers := host.GetResponseHeaders()
		enforce, ok := test.GetHeaderValue(headers, headerCSP)
		require.True(t, ok, "enforcing header should be set")
		require.Equal(t, "default-src 'self'", enforce)

		reportOnly, ok := test.GetHeaderValue(headers, headerCSPReportOnly)
		require.True(t, ok, "report-only candidate header should be set")
		require.Equal(t, "default-src 'self'; require-trusted-types-for 'script'", reportOnly)
	})
}

func TestReportOnlyPolicyConflictRejected(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		config, _ := json.Marshal(map[string]any{
			"policy":             "default-src 'self'",
			"report_only":        true,
			"report_only_policy": "default-src 'none'",
		})
		host, status := test.NewTestHost(config)
		defer host.Reset()
		require.NotEqual(t, types.OnPluginStartStatusOK, status,
			"report_only_policy combined with report_only:true should fail plugin start")
	})
}

func TestOverrideFalseInjectsOwnVariant(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		config, _ := json.Marshal(map[string]any{
			"policy":   "default-src 'self'",
			"override": false,
		})
		host, status := test.NewTestHost(config)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		// Upstream set only the report-only variant; our enforcing policy must
		// still be injected (different variant), and the upstream one preserved.
		host.CallOnHttpResponseHeaders([][2]string{
			{":status", "200"},
			{headerCSPReportOnly, "default-src https://upstream.example"},
		})

		headers := host.GetResponseHeaders()
		enforce, ok := test.GetHeaderValue(headers, headerCSP)
		require.True(t, ok, "enforcing policy must be injected when only the other variant exists")
		require.Equal(t, "default-src 'self'", enforce)

		reportOnly, ok := test.GetHeaderValue(headers, headerCSPReportOnly)
		require.True(t, ok)
		require.Equal(t, "default-src https://upstream.example", reportOnly,
			"upstream report-only header must be kept")
	})
}

func TestMainNoop(t *testing.T) {
	main()
}

func TestEmptyPolicyRejected(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		config, _ := json.Marshal(map[string]any{"policy": ""})
		host, status := test.NewTestHost(config)
		defer host.Reset()
		require.NotEqual(t, types.OnPluginStartStatusOK, status,
			"empty policy should fail plugin start")
	})
}
