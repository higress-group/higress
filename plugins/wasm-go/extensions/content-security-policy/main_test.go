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

// enforcingConfig defines two enforcing policies.
var enforcingConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"policies": []string{
			"default-src 'self'",
			"script-src 'self' https://cdn.example.com",
		},
	})
	return data
}()

// reportOnlyConfig switches the header to the report-only variant.
var reportOnlyConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"policy":      "default-src 'self'; report-uri /csp-report",
		"report_only": true,
	})
	return data
}()

// singlePolicyConfig uses a single enforcing policy, used to verify that an
// upstream CSP header is replaced (not merged) with the gateway-owned one.
var singlePolicyConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"policy": "default-src 'self'",
	})
	return data
}()

func TestParseConfig(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		t.Run("enforcing policies", func(t *testing.T) {
			host, status := test.NewTestHost(enforcingConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			cspConfig := config.(*CSPConfig)
			require.False(t, cspConfig.reportOnly)
			require.Equal(t, []string{
				"default-src 'self'",
				"script-src 'self' https://cdn.example.com",
			}, cspConfig.policies)
		})

		t.Run("report-only single policy field", func(t *testing.T) {
			host, status := test.NewTestHost(reportOnlyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			cspConfig := config.(*CSPConfig)
			require.True(t, cspConfig.reportOnly)
			require.Equal(t, []string{"default-src 'self'; report-uri /csp-report"}, cspConfig.policies)
		})

		t.Run("blank policies are dropped", func(t *testing.T) {
			blankConfig := func() json.RawMessage {
				data, _ := json.Marshal(map[string]interface{}{
					"policies": []string{"  ", "default-src 'self'", ""},
				})
				return data
			}()

			host, status := test.NewTestHost(blankConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			cspConfig := config.(*CSPConfig)
			require.Equal(t, []string{"default-src 'self'"}, cspConfig.policies)
		})

		t.Run("missing policy fails plugin start", func(t *testing.T) {
			emptyConfig := func() json.RawMessage {
				data, _ := json.Marshal(map[string]interface{}{})
				return data
			}()

			host, status := test.NewTestHost(emptyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})
	})
}

func TestOnHttpResponseHeadersEnforcing(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(enforcingConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		action := host.CallOnHttpResponseHeaders([][2]string{
			{":status", "200"},
			{"content-type", "text/html"},
		})
		require.Equal(t, types.ActionContinue, action)

		headers := host.GetResponseHeaders()
		// Both policy blocks are combined into a single header, directives
		// separated by "; ".
		require.True(t, test.HasHeaderWithValue(headers, headerContentSecurityPolicy,
			"default-src 'self'; script-src 'self' https://cdn.example.com"))
		// Report-only header must not be present.
		require.False(t, test.HasHeader(headers, headerContentSecurityPolicyReport))

		host.CompleteHttp()
	})
}

func TestOnHttpResponseHeadersReportOnly(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(reportOnlyConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		action := host.CallOnHttpResponseHeaders([][2]string{
			{":status", "200"},
		})
		require.Equal(t, types.ActionContinue, action)

		headers := host.GetResponseHeaders()
		require.True(t, test.HasHeaderWithValue(headers, headerContentSecurityPolicyReport, "default-src 'self'; report-uri /csp-report"))
		// Enforcing header must not be present in report-only mode.
		require.False(t, test.HasHeader(headers, headerContentSecurityPolicy))

		host.CompleteHttp()
	})
}

func TestOnHttpResponseHeadersReplacesUpstreamHeader(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(singlePolicyConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		// The upstream already set a CSP header; the plugin should replace it
		// with the gateway-owned policy rather than leaving the original in place.
		action := host.CallOnHttpResponseHeaders([][2]string{
			{":status", "200"},
			{headerContentSecurityPolicy, "default-src 'unsafe-inline'"},
		})
		require.Equal(t, types.ActionContinue, action)

		headers := host.GetResponseHeaders()
		require.False(t, test.HasHeaderWithValue(headers, headerContentSecurityPolicy, "default-src 'unsafe-inline'"))
		require.True(t, test.HasHeaderWithValue(headers, headerContentSecurityPolicy, "default-src 'self'"))

		host.CompleteHttp()
	})
}
