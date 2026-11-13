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
	"fmt"
	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/protocol"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"testing"
)

// toolSetOrderConfig 构造独立来源名称的六种规则顺序；URL仅满足配置约束，测试不能访问后端。
func toolSetOrderConfig(t *testing.T, keepSecond bool) json.RawMessage {
	t.Helper()
	orders := [][]string{{"a", "set", "b"}, {"a", "b", "set"}, {"set", "a", "b"}, {"set", "b", "a"}, {"b", "a", "set"}, {"b", "set", "a"}}
	rules := make([]any, 0, len(orders)*3)
	for index, order := range orders {
		a, b := fmt.Sprintf("source-a-%d", index), fmt.Sprintf("source-b-%d", index)
		source := func(name, tool string, keep bool) map[string]any {
			tools := []any{}
			if keep {
				tools = append(tools, map[string]any{"name": tool, "description": name, "requestTemplate": map[string]any{"method": "GET", "url": "https://example.invalid/records"}})
			}
			return map[string]any{"_match_domain_": []string{name + ".example.test"}, "server": map[string]any{"name": name}, "tools": tools}
		}
		byName := map[string]any{"a": source(a, "list_records", true), "b": source(b, "lookup_record", keepSecond), "set": map[string]any{
			"_match_domain_": []string{fmt.Sprintf("set-%d.example.test", index)}, "toolSet": map[string]any{"name": fmt.Sprintf("combined-%d", index), "serverTools": []any{
				map[string]any{"serverName": a, "tools": []string{"list_records", "missing"}}, map[string]any{"serverName": b, "tools": []string{"lookup_record"}},
			}},
		}}
		for _, name := range order {
			if name == "b" && !keepSecond {
				continue
			}
			rules = append(rules, byName[name])
		}
	}
	raw, err := json.Marshal(map[string]any{"toolSet": map[string]any{"name": "empty", "serverTools": []any{}}, "_rules_": rules})
	require.NoError(t, err)
	return raw
}

// toolSetOrderList 经真实插件请求入口列举工具；验证两种协议、请求过滤和无后端调用，不只检查HTTP状态。
func toolSetOrderList(t *testing.T, host test.TestHost, domain string, modern bool, allowed string) []string {
	t.Helper()
	host.InitHttp()
	defer host.CompleteHttp()
	require.NoError(t, host.SetDomainName(domain))
	headers := [][2]string{{":authority", domain}, {":method", "POST"}, {":path", "/mcp"}, {"content-type", "application/json"}, {"accept", "application/json, text/event-stream"}}
	params := map[string]any{}
	if modern {
		headers = append(headers, [2]string{protocol.HeaderProtocolVersion, "2026-07-28"}, [2]string{protocol.HeaderMethod, "tools/list"})
		params["_meta"] = map[string]any{protocol.MetaProtocolVersion: "2026-07-28", protocol.MetaClientCapabilities: map[string]any{}}
	}
	if allowed != "" {
		headers = append(headers, [2]string{"x-envoy-allow-mcp-tools", allowed})
	}
	require.Equal(t, types.HeaderStopIteration, host.CallOnHttpRequestHeaders(headers))
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": "order", "method": "tools/list", "params": params})
	require.NoError(t, err)
	host.CallOnHttpRequestBody(body)
	response := proxyTestResponse(host)
	require.NotNil(t, response)
	require.EqualValues(t, 200, response.StatusCode)
	envelope := gjson.ParseBytes(response.Data)
	require.False(t, envelope.Get("error").Exists(), string(response.Data))
	require.Equal(t, "order", envelope.Get("id").String())
	if modern {
		require.Equal(t, "complete", envelope.Get("result.resultType").String())
	}
	names := make([]string, 0)
	for _, tool := range envelope.Get("result.tools").Array() {
		names = append(names, tool.Get("name").String())
	}
	require.Empty(t, host.GetHttpCalloutAttributes())
	return names
}

// TestToolSetRuleOrderIndependent 在Go/Wasm各验证六种加载顺序；缺失引用跳过，来源工具和组合工具均须可见。
func TestToolSetRuleOrderIndependent(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(toolSetOrderConfig(t, true))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		for index := 0; index < 6; index++ {
			for _, modern := range []bool{false, true} {
				t.Run(fmt.Sprintf("order-%d/modern-%t", index, modern), func(t *testing.T) {
					a, b := fmt.Sprintf("source-a-%d", index), fmt.Sprintf("source-b-%d", index)
					require.Equal(t, []string{"list_records"}, toolSetOrderList(t, host, a+".example.test", modern, ""))
					require.Equal(t, []string{"lookup_record"}, toolSetOrderList(t, host, b+".example.test", modern, ""))
					domain := fmt.Sprintf("set-%d.example.test", index)
					want := []string{a + "___list_records", b + "___lookup_record"}
					for repeat := 0; repeat < 2; repeat++ {
						require.Equal(t, want, toolSetOrderList(t, host, domain, modern, ""))
					}
					require.Equal(t, want[1:], toolSetOrderList(t, host, domain, modern, want[1]))
					require.Empty(t, toolSetOrderList(t, host, domain, modern, "unlisted"))
				})
			}
		}
	})
}

// TestToolSetFreshConfigurationDropsRemovedTools 验证新插件配置不继承旧代来源；两代使用相同服务名且工具先有后删。
func TestToolSetFreshConfigurationDropsRemovedTools(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		for _, keepSecond := range []bool{true, false, true} {
			t.Run(fmt.Sprintf("second-%t", keepSecond), func(t *testing.T) {
				host, status := test.NewTestHost(toolSetOrderConfig(t, keepSecond))
				defer host.Reset()
				require.Equal(t, types.OnPluginStartStatusOK, status)
				for _, modern := range []bool{false, true} {
					want := []string{"source-a-0___list_records"}
					if keepSecond {
						want = append(want, "source-b-0___lookup_record")
					}
					require.Equal(t, want, toolSetOrderList(t, host, "set-0.example.test", modern, ""))
				}
			})
		}
	})
}
