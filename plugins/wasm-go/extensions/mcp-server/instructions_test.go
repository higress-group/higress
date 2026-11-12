// Copyright (c) 2026 Alibaba Group Holding Ltd.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software distributed
// under the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
// CONDITIONS OF ANY KIND, either express or implied.

package main

import (
	"encoding/json"
	"testing"

	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/protocol"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func restInstructionsConfig(t *testing.T, name string) map[string]any {
	t.Helper()
	var config map[string]any
	require.NoError(t, json.Unmarshal(restMCPServerWithVersionConfig, &config))
	config["server"].(map[string]any)["name"] = name
	return config
}

func restInstructionsExchange(t *testing.T, host test.TestHost, authority, method, version string) gjson.Result {
	t.Helper()
	host.InitHttp()
	defer host.CompleteHttp()
	headers := [][2]string{
		{":authority", authority}, {":method", "POST"}, {":path", "/mcp"},
		{"content-type", "application/json"}, {"accept", "application/json, text/event-stream"},
	}
	params := map[string]any{"protocolVersion": version}
	if method == "server/discover" {
		headers = append(headers,
			[2]string{protocol.HeaderProtocolVersion, version},
			[2]string{protocol.HeaderMethod, method},
		)
		params = map[string]any{"_meta": map[string]any{
			protocol.MetaProtocolVersion: version, protocol.MetaClientCapabilities: map[string]any{},
		}}
	}
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": "instructions-request", "method": method, "params": params})
	require.NoError(t, err)
	require.Equal(t, types.HeaderStopIteration, host.CallOnHttpRequestHeaders(headers))
	require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody(body))
	response := proxyTestResponse(host)
	require.NotNil(t, response)
	require.Equal(t, uint32(200), response.StatusCode)
	require.True(t, gjson.ValidBytes(response.Data))
	envelope := gjson.ParseBytes(response.Data)
	require.False(t, envelope.Get("error").Exists(), string(response.Data))
	require.Equal(t, "instructions-request", envelope.Get("id").String())
	require.Empty(t, host.GetHttpCalloutAttributes(), "instructions are served locally")
	return envelope.Get("result")
}

func TestRestMCPServerInstructions(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		for _, tc := range []struct {
			name, value string
			configured  bool
			defaultType bool
		}{
			{name: "omitted"},
			{name: "empty", configured: true},
			{name: "multiline unicode and escaping", value: "  查询订单前先确认用户。\nUse \"get_order\" with C:\\orders. 🛒\n", configured: true, defaultType: true},
			{name: "whitespace preserved", value: " \n\t ", configured: true},
		} {
			t.Run(tc.name, func(t *testing.T) {
				config := restInstructionsConfig(t, "instructions-rest")
				server := config["server"].(map[string]any)
				// Exercise the default REST type as well as the explicit type.
				if tc.defaultType {
					delete(server, "type")
				}
				if tc.configured {
					server["instructions"] = tc.value
				}
				raw, err := json.Marshal(config)
				require.NoError(t, err)
				host, status := test.NewTestHost(raw)
				defer host.Reset()
				require.Equal(t, types.OnPluginStartStatusOK, status)
				for _, version := range []string{"2024-11-05", "2025-03-26", "2025-06-18", "2025-11-25", "2026-07-28"} {
					t.Run(version, func(t *testing.T) {
						method := "initialize"
						if version == "2026-07-28" {
							method = "server/discover"
						}
						result := restInstructionsExchange(t, host, "mcp.example.com", method, version)
						instructions := result.Get("instructions")
						require.Equal(t, tc.value != "", instructions.Exists())
						if instructions.Exists() {
							require.Equal(t, gjson.String, instructions.Type)
							require.Equal(t, tc.value, instructions.String())
						}
						require.JSONEq(t, `{"tools":{}}`, result.Get("capabilities").Raw)
						if method == "initialize" {
							require.Equal(t, version, result.Get("protocolVersion").String())
							require.JSONEq(t, `{"name":"instructions-rest","version":"2.5.0"}`, result.Get("serverInfo").Raw)
						} else {
							require.Equal(t, "complete", result.Get("resultType").String())
							require.Equal(t, "instructions-rest", result.Get(`_meta.io\.modelcontextprotocol/serverInfo.name`).String())
							require.JSONEq(t, `["2024-11-05","2025-03-26","2025-06-18","2025-11-25","2026-07-28"]`, result.Get("supportedVersions").Raw)
							require.Equal(t, "0", result.Get("ttlMs").Raw)
							require.Equal(t, "private", result.Get("cacheScope").String())
						}
					})
				}
			})
		}
	})
}

func TestRestMCPServerInstructionsRuleIsolation(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		config := restInstructionsConfig(t, "global")
		config["server"].(map[string]any)["instructions"] = "global instructions"
		var rules []map[string]any
		for _, name := range []string{"first", "second"} {
			rule := restInstructionsConfig(t, name)
			rule["_match_domain_"] = []string{name + ".example.com"}
			if name == "first" {
				rule["server"].(map[string]any)["instructions"] = "first instructions"
			}
			rules = append(rules, rule)
		}
		config["_rules_"] = rules
		raw, err := json.Marshal(config)
		require.NoError(t, err)
		host, status := test.NewTestHost(raw)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		for _, name := range []string{"first", "second", "global", "first", "second"} {
			for _, method := range []string{"initialize", "server/discover"} {
				version := "2025-11-25"
				if method == "server/discover" {
					version = "2026-07-28"
				}
				result := restInstructionsExchange(t, host, name+".example.com", method, version)
				if name == "second" {
					require.False(t, result.Get("instructions").Exists(), "an omitted rule field must not inherit another server's instructions")
				} else {
					require.Equal(t, name+" instructions", result.Get("instructions").String())
				}
			}
		}
	})
}
