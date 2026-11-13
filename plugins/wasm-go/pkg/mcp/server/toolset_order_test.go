// Copyright (c) 2026 Alibaba Group Holding Ltd.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software distributed
// under the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
// CONDITIONS OF ANY KIND, either express or implied.

package server

import (
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"sync"
	"testing"
)

// parseToolSetGeneration 按指定顺序加载独立配置代；缺失来源不注册，绝不复用旧代registry模拟重载。
func parseToolSetGeneration(t *testing.T, order []string, kind string, keepSecond bool) (*McpServerConfig, *GlobalToolRegistry) {
	t.Helper()
	registry := &GlobalToolRegistry{}
	registry.Initialize()
	registered := newValidationTestServer()
	registered.AddMCPTool("lookup_record", &descriptorFixtureTool{description: "registered source", input: map[string]any{"type": "object", "properties": map[string]any{}}, output: map[string]any{"type": "object"}})
	opts := &ConfigOptions{Servers: map[string]Server{"source-b": registered}, ToolRegistry: registry}
	sourceB := `{"server":{"name":"source-b"},"tools":[{"name":"lookup_record","requestTemplate":{"url":"https://example.invalid/b","method":"GET"}}]}`
	if kind == "proxy" {
		sourceB = `{"server":{"name":"source-b","type":"mcp-proxy","transport":"http","mcpServerURL":"https://example.invalid/mcp"},"tools":[{"name":"lookup_record","inputSchema":{"type":"object","properties":{"count":{"type":"integer","minimum":9007199254740993}},"required":["count"]},"outputSchema":{"type":"object"}}]}`
	}
	if kind == "registered" {
		sourceB = `{"server":{"name":"source-b"}}`
	}
	rules := map[string]string{
		"a":   `{"server":{"name":"source-a"},"tools":[{"name":"list_records","description":"first source","outputSchema":{"type":"object"},"requestTemplate":{"url":"https://example.invalid/a","method":"GET"}}]}`,
		"b":   sourceB,
		"set": `{"toolSet":{"name":"combined","serverTools":[{"serverName":"source-a","tools":["list_records","missing","list_records"]},{"serverName":"source-b","tools":["lookup_record"]},{"serverName":"absent","tools":["unknown"]}]}}`,
	}
	var combined *McpServerConfig
	for _, name := range order {
		if name == "b" && !keepSecond {
			continue
		}
		config := &McpServerConfig{}
		require.NoError(t, ParseConfigCore(gjson.Parse(rules[name]), config, opts))
		if name == "set" {
			combined = config
		}
	}
	require.NotNil(t, combined)
	return combined, registry
}

// TestToolSetListIgnoresRegistrationOrder 验证六种加载顺序和三类来源；排序/去重/缺失跳过及代理大整数必须保持。
func TestToolSetListIgnoresRegistrationOrder(t *testing.T) {
	orders := [][]string{{"a", "set", "b"}, {"a", "b", "set"}, {"set", "a", "b"}, {"set", "b", "a"}, {"b", "a", "set"}, {"b", "set", "a"}}
	for _, kind := range []string{"rest", "proxy", "registered"} {
		for _, order := range orders {
			t.Run(fmt.Sprint(kind, order), func(t *testing.T) {
				config, registry := parseToolSetGeneration(t, order, kind, true)
				_, found := registry.GetToolInfo("source-b", "lookup_record")
				require.True(t, found)
				listed := config.buildToolList(nil)
				require.Len(t, listed, 2)
				require.Equal(t, "source-a___list_records", listed[0]["name"])
				require.Equal(t, "source-b___lookup_record", listed[1]["name"])
				require.Equal(t, map[string]any{"type": "object"}, listed[0]["outputSchema"])
				if kind == "proxy" {
					raw, err := json.Marshal(listed[1])
					require.NoError(t, err)
					require.Contains(t, string(raw), `"minimum":9007199254740993`)
					require.Equal(t, map[string]any{"type": "object"}, listed[1]["outputSchema"])
				}
				allowed := map[string]struct{}{"source-b___lookup_record": {}}
				filtered := config.buildToolList(&allowed)
				require.Len(t, filtered, 1)
				require.Equal(t, listed[1], filtered[0])
				empty := map[string]struct{}{}
				require.Equal(t, []map[string]any{}, config.buildToolList(&empty))
			})
		}
	}
}

// TestToolSetListOwnsDescriptorsAcrossGenerations 保留旧代对象检验删除/重载隔离；修改响应副本不得污染后续列表。
func TestToolSetListOwnsDescriptorsAcrossGenerations(t *testing.T) {
	old, oldRegistry := parseToolSetGeneration(t, []string{"a", "set", "b"}, "proxy", true)
	current, currentRegistry := parseToolSetGeneration(t, []string{"set", "b", "a"}, "proxy", false)
	require.NotSame(t, oldRegistry, currentRegistry)
	before := old.buildToolList(nil)
	require.Len(t, before, 2)
	before[0]["description"] = "changed response"
	before[0]["outputSchema"].(map[string]any)["mutated"] = true
	before[1]["inputSchema"].(map[string]any)["properties"].(map[string]any)["mutated"] = true
	fresh := old.buildToolList(nil)
	require.Equal(t, "first source", fresh[0]["description"])
	require.NotContains(t, fresh[0]["outputSchema"], "mutated")
	require.NotContains(t, fresh[1]["inputSchema"].(map[string]any)["properties"], "mutated")
	require.Len(t, current.buildToolList(nil), 1)
	require.Len(t, old.buildToolList(nil), 2)
	// 缺失引用已在上面串行验证；并发阶段只读取有效引用，避免单线程 proxytest 日志宿主
	// 在 Warnf 中并发追加日志切片。仍通过解析器绑定同两个 registry，验证完整列表的隔离。
	resolvedConfig := func(registry *GlobalToolRegistry, includeSecond bool) *McpServerConfig {
		references := []ServerToolConfig{{ServerName: "source-a", Tools: []string{"list_records"}}}
		if includeSecond {
			references = append(references, ServerToolConfig{ServerName: "source-b", Tools: []string{"lookup_record"}})
		}
		raw, err := json.Marshal(map[string]any{"toolSet": ToolSetConfig{Name: "concurrent", ServerTools: references}})
		require.NoError(t, err)
		config := &McpServerConfig{}
		require.NoError(t, ParseConfigCore(gjson.ParseBytes(raw), config, &ConfigOptions{Servers: map[string]Server{}, ToolRegistry: registry}))
		return config
	}
	oldConcurrent, currentConcurrent := resolvedConfig(oldRegistry, true), resolvedConfig(currentRegistry, false)
	// 并发读取使用同一已发布配置，但每次快照和返回值独立；由 race 检查潜在的跨请求写入。
	var wg sync.WaitGroup
	failures := make(chan string, 8)
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for repeat := 0; repeat < 20; repeat++ {
				if len(oldConcurrent.buildToolList(nil)) != 2 || len(currentConcurrent.buildToolList(nil)) != 1 {
					failures <- "generation changed"
					return
				}
			}
		}()
	}
	wg.Wait()
	close(failures)
	for failure := range failures {
		t.Error(failure)
	}
}

// TestEmptyToolSetListIsArray 保持空工具集的JSON空数组语义，不将其转换成null。
func TestEmptyToolSetListIsArray(t *testing.T) {
	registry := &GlobalToolRegistry{}
	registry.Initialize()
	config := &McpServerConfig{}
	require.NoError(t, ParseConfigCore(gjson.Parse(`{"toolSet":{"name":"empty","serverTools":[]}}`), config, &ConfigOptions{Servers: map[string]Server{}, ToolRegistry: registry}))
	raw, err := json.Marshal(config.buildToolList(nil))
	require.NoError(t, err)
	require.Equal(t, "[]", string(raw))
}
