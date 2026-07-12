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
	"strings"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

// enforcingConfig configures only the enforcing Content-Security-Policy header.
var enforcingConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"content_security_policy": "default-src 'self'; script-src 'self'",
	})
	return data
}()

// reportOnlyConfig configures only the report-only header.
var reportOnlyConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"content_security_policy_report_only": "default-src 'self'; report-uri /csp-report",
	})
	return data
}()

// bothConfig configures both the enforcing and report-only headers.
var bothConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"content_security_policy":             "default-src 'self'",
		"content_security_policy_report_only": "img-src 'self'; report-uri /csp-report",
	})
	return data
}()

// emptyConfig has neither field set, which must be rejected.
var emptyConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{})
	return data
}()

func TestParseConfig(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		t.Run("enforcing config", func(t *testing.T) {
			host, status := test.NewTestHost(enforcingConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
			cspConfig, ok := config.(*CSPConfig)
			require.True(t, ok)
			require.Equal(t, "default-src 'self'; script-src 'self'", cspConfig.contentSecurityPolicy)
			require.Empty(t, cspConfig.contentSecurityPolicyReportOnly)
		})

		t.Run("report-only config", func(t *testing.T) {
			host, status := test.NewTestHost(reportOnlyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
			cspConfig, ok := config.(*CSPConfig)
			require.True(t, ok)
			require.Empty(t, cspConfig.contentSecurityPolicy)
			require.Equal(t, "default-src 'self'; report-uri /csp-report", cspConfig.contentSecurityPolicyReportOnly)
		})

		t.Run("both enforcing and report-only config", func(t *testing.T) {
			host, status := test.NewTestHost(bothConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
			cspConfig, ok := config.(*CSPConfig)
			require.True(t, ok)
			require.Equal(t, "default-src 'self'", cspConfig.contentSecurityPolicy)
			require.Equal(t, "img-src 'self'; report-uri /csp-report", cspConfig.contentSecurityPolicyReportOnly)
		})

		t.Run("empty config should be rejected", func(t *testing.T) {
			host, status := test.NewTestHost(emptyConfig)
			defer host.Reset()
			// An invalid configuration must not let the plugin start successfully.
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})
	})
}

func TestOnHttpResponseHeaders(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("adds enforcing CSP header", func(t *testing.T) {
			host, status := test.NewTestHost(enforcingConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/index.html"},
				{":method", "GET"},
			})
			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "text/html"},
			})
			require.Equal(t, types.ActionContinue, action)

			responseHeaders := host.GetResponseHeaders()
			require.True(t, test.HasHeaderWithValue(responseHeaders,
				"content-security-policy", "default-src 'self'; script-src 'self'"))
			require.False(t, test.HasHeader(responseHeaders, "content-security-policy-report-only"))

			host.CompleteHttp()
		})

		t.Run("adds report-only CSP header", func(t *testing.T) {
			host, status := test.NewTestHost(reportOnlyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/index.html"},
				{":method", "GET"},
			})
			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "text/html"},
			})
			require.Equal(t, types.ActionContinue, action)

			responseHeaders := host.GetResponseHeaders()
			require.True(t, test.HasHeaderWithValue(responseHeaders,
				"content-security-policy-report-only", "default-src 'self'; report-uri /csp-report"))
			require.False(t, test.HasHeader(responseHeaders, "content-security-policy"))

			host.CompleteHttp()
		})

		t.Run("adds both enforcing and report-only headers", func(t *testing.T) {
			host, status := test.NewTestHost(bothConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/index.html"},
				{":method", "GET"},
			})
			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "text/html"},
			})
			require.Equal(t, types.ActionContinue, action)

			responseHeaders := host.GetResponseHeaders()
			require.True(t, test.HasHeaderWithValue(responseHeaders,
				"content-security-policy", "default-src 'self'"))
			require.True(t, test.HasHeaderWithValue(responseHeaders,
				"content-security-policy-report-only", "img-src 'self'; report-uri /csp-report"))

			host.CompleteHttp()
		})

		t.Run("replaces existing upstream CSP header", func(t *testing.T) {
			host, status := test.NewTestHost(enforcingConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/index.html"},
				{":method", "GET"},
			})
			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "text/html"},
				{"content-security-policy", "default-src *"},
			})
			require.Equal(t, types.ActionContinue, action)

			responseHeaders := host.GetResponseHeaders()
			// The gateway-configured policy must override the upstream value, and
			// only a single CSP header must remain.
			count := 0
			for _, h := range responseHeaders {
				if strings.EqualFold(h[0], "content-security-policy") {
					count++
					require.Equal(t, "default-src 'self'; script-src 'self'", h[1])
				}
			}
			require.Equal(t, 1, count)

			host.CompleteHttp()
		})
	})
}
