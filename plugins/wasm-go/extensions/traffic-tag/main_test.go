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

// 测试配置：基本条件组配置
var basicConditionConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"conditionGroups": []map[string]interface{}{
			{
				"headerName":  "X-Traffic-Tag",
				"headerValue": "condition-match",
				"logic":       "and",
				"conditions": []map[string]interface{}{
					{
						"conditionType": "header",
						"key":           "User-Agent",
						"operator":      "prefix",
						"value":         []string{"Mozilla"},
					},
				},
			},
		},
	})
	return data
}()

// 测试配置：复杂条件组配置（多个条件，OR 逻辑）
var complexConditionConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"conditionGroups": []map[string]interface{}{
			{
				"headerName":  "X-Traffic-Tag",
				"headerValue": "complex-match",
				"logic":       "or",
				"conditions": []map[string]interface{}{
					{
						"conditionType": "header",
						"key":           "User-Agent",
						"operator":      "equal",
						"value":         []string{"Mobile-App"},
					},
					{
						"conditionType": "cookie",
						"key":           "session-type",
						"operator":      "in",
						"value":         []string{"premium", "vip"},
					},
				},
			},
		},
	})
	return data
}()

// 测试配置：权重组配置
var weightConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"weightGroups": []map[string]interface{}{
			{
				"headerName":  "X-Traffic-Tag",
				"headerValue": "weight-30",
				"weight":      30,
			},
			{
				"headerName":  "X-Traffic-Tag",
				"headerValue": "weight-70",
				"weight":      70,
			},
		},
		"defaultTagKey":   "X-Default-Tag",
		"defaultTagValue": "default-value",
	})
	return data
}()

// 测试配置：混合配置（条件组 + 权重组）
var mixedConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"conditionGroups": []map[string]interface{}{
			{
				"headerName":  "X-Traffic-Tag",
				"headerValue": "condition-match",
				"logic":       "and",
				"conditions": []map[string]interface{}{
					{
						"conditionType": "header",
						"key":           "X-Source",
						"operator":      "equal",
						"value":         []string{"mobile"},
					},
				},
			},
		},
		"weightGroups": []map[string]interface{}{
			{
				"headerName":  "X-Traffic-Tag",
				"headerValue": "weight-50",
				"weight":      50,
			},
			{
				"headerName":  "X-Traffic-Tag",
				"headerValue": "weight-50",
				"weight":      50,
			},
		},
		"defaultTagKey":   "X-Default-Tag",
		"defaultTagValue": "fallback",
	})
	return data
}()

// 测试配置：空配置
var emptyConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{})
	return data
}()

// 测试配置：无效条件组配置（缺少必需字段）
var invalidConditionConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"conditionGroups": []map[string]interface{}{
			{
				"headerName": "X-Traffic-Tag",
				// 缺少 headerValue 和 logic
				"conditions": []map[string]interface{}{
					{
						"conditionType": "header",
						"key":           "User-Agent",
						"operator":      "prefix",
						"value":         []string{"Mozilla"},
					},
				},
			},
		},
	})
	return data
}()

// 测试配置：无效条件配置（无效的操作符）
var invalidOperatorConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"conditionGroups": []map[string]interface{}{
			{
				"headerName":  "X-Traffic-Tag",
				"headerValue": "invalid-operator",
				"logic":       "and",
				"conditions": []map[string]interface{}{
					{
						"conditionType": "header",
						"key":           "User-Agent",
						"operator":      "invalid_operator",
						"value":         []string{"Mozilla"},
					},
				},
			},
		},
	})
	return data
}()

// 测试配置：正则表达式条件配置
var regexConditionConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"conditionGroups": []map[string]interface{}{
			{
				"headerName":  "X-Traffic-Tag",
				"headerValue": "regex-match",
				"logic":       "and",
				"conditions": []map[string]interface{}{
					{
						"conditionType": "header",
						"key":           "User-Agent",
						"operator":      "regex",
						"value":         []string{`.*Mobile.*`},
					},
				},
			},
		},
	})
	return data
}()

// 测试配置：百分比条件配置
var percentageConditionConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"conditionGroups": []map[string]interface{}{
			{
				"headerName":  "X-Traffic-Tag",
				"headerValue": "percentage-match",
				"logic":       "and",
				"conditions": []map[string]interface{}{
					{
						"conditionType": "header",
						"key":           "X-User-ID",
						"operator":      "percentage",
						"value":         []string{"30"},
					},
				},
			},
		},
	})
	return data
}()

// 测试配置：Exists 条件配置
var existsConditionConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"conditionGroups": []map[string]interface{}{
			{
				"headerName":  "X-Traffic-Tag",
				"headerValue": "exists-match",
				"logic":       "and",
				"conditions": []map[string]interface{}{
					{
						"conditionType": "header",
						"key":           "X-User-ID",
						"operator":      "exists",
						// exists 操作符不需要 value 字段
					},
				},
			},
		},
	})
	return data
}()

// 测试配置：NotExists 条件配置
var notExistsConditionConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"conditionGroups": []map[string]interface{}{
			{
				"headerName":  "X-Traffic-Tag",
				"headerValue": "not-exists-match",
				"logic":       "and",
				"conditions": []map[string]interface{}{
					{
						"conditionType": "header",
						"key":           "X-Debug-Mode",
						"operator":      "not_exists",
					},
				},
			},
		},
	})
	return data
}()

func TestParseConfig(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// 测试基本条件组配置解析
		t.Run("basic condition config", func(t *testing.T) {
			host, status := test.NewTestHost(basicConditionConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试复杂条件组配置解析
		t.Run("complex condition config", func(t *testing.T) {
			host, status := test.NewTestHost(complexConditionConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试权重组配置解析
		t.Run("weight config", func(t *testing.T) {
			host, status := test.NewTestHost(weightConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试混合配置解析
		t.Run("mixed config", func(t *testing.T) {
			host, status := test.NewTestHost(mixedConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试空配置解析
		t.Run("empty config", func(t *testing.T) {
			host, status := test.NewTestHost(emptyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试无效条件组配置解析
		t.Run("invalid condition config", func(t *testing.T) {
			host, status := test.NewTestHost(invalidConditionConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})

		// 测试无效操作符配置解析
		t.Run("invalid operator config", func(t *testing.T) {
			host, status := test.NewTestHost(invalidOperatorConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})

		// 测试正则表达式条件配置解析
		t.Run("regex condition config", func(t *testing.T) {
			host, status := test.NewTestHost(regexConditionConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试百分比条件配置解析
		t.Run("percentage condition config", func(t *testing.T) {
			host, status := test.NewTestHost(percentageConditionConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试存在/不存在配置解析
		t.Run("percentage condition config", func(t *testing.T) {
			host, status := test.NewTestHost(percentageConditionConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试 Exists 条件配置解析
		t.Run("exists condition config", func(t *testing.T) {
			host, status := test.NewTestHost(existsConditionConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试 NotExists 条件配置解析
		t.Run("not_exists condition config", func(t *testing.T) {
			host, status := test.NewTestHost(notExistsConditionConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})
	})
}

func TestOnHttpRequestHeaders(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试基本条件匹配 - 匹配成功
		t.Run("basic condition match - success", func(t *testing.T) {
			host, status := test.NewTestHost(basicConditionConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "GET"},
				{"User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"},
			})

			require.Equal(t, types.ActionContinue, action)

			// 验证是否添加了流量标签头
			requestHeaders := host.GetRequestHeaders()
			tagHeaderFound := false
			for _, header := range requestHeaders {
				if header[0] == "x-traffic-tag" && header[1] == "condition-match" {
					tagHeaderFound = true
					break
				}
			}
			require.True(t, tagHeaderFound, "Traffic tag header should be added")

			host.CompleteHttp()
		})

		// 测试基本条件匹配 - 匹配失败
		t.Run("basic condition match - failure", func(t *testing.T) {
			host, status := test.NewTestHost(basicConditionConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "GET"},
				{"User-Agent", "Custom-Client/1.0"},
			})

			require.Equal(t, types.ActionContinue, action)

			// 验证没有添加流量标签头
			requestHeaders := host.GetRequestHeaders()
			tagHeaderFound := false
			for _, header := range requestHeaders {
				if header[0] == "x-traffic-tag" {
					tagHeaderFound = true
					break
				}
			}
			require.False(t, tagHeaderFound, "Traffic tag header should not be added")

			host.CompleteHttp()
		})

		// 测试复杂条件匹配 - OR 逻辑，第一个条件匹配
		t.Run("complex condition match - OR logic, first condition matches", func(t *testing.T) {
			host, status := test.NewTestHost(complexConditionConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "GET"},
				{"User-Agent", "Mobile-App"},
			})

			require.Equal(t, types.ActionContinue, action)

			// 验证是否添加了流量标签头
			requestHeaders := host.GetRequestHeaders()
			tagHeaderFound := false
			for _, header := range requestHeaders {
				if header[0] == "x-traffic-tag" && header[1] == "complex-match" {
					tagHeaderFound = true
					break
				}
			}
			require.True(t, tagHeaderFound, "Traffic tag header should be added")

			host.CompleteHttp()
		})

		// 测试复杂条件匹配 - OR 逻辑，第二个条件匹配
		t.Run("complex condition match - OR logic, second condition matches", func(t *testing.T) {
			host, status := test.NewTestHost(complexConditionConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "GET"},
				{"Cookie", "session-type=premium; other=value"},
			})

			require.Equal(t, types.ActionContinue, action)

			// 验证是否添加了流量标签头
			requestHeaders := host.GetRequestHeaders()
			tagHeaderFound := false
			for _, header := range requestHeaders {
				if header[0] == "x-traffic-tag" && header[1] == "complex-match" {
					tagHeaderFound = true
					break
				}
			}
			require.True(t, tagHeaderFound, "Traffic tag header should be added")

			host.CompleteHttp()
		})

		// 测试权重分配
		t.Run("weight distribution", func(t *testing.T) {
			host, status := test.NewTestHost(weightConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "GET"},
			})

			require.Equal(t, types.ActionContinue, action)

			// 验证是否添加了流量标签头（权重分配是随机的，这里只验证行为）
			// 权重分配是随机的，可能添加也可能不添加
			// 这里只验证插件正常运行，不强制要求特定结果

			host.CompleteHttp()
		})

		// 测试默认标签设置
		t.Run("default tag setting", func(t *testing.T) {
			host, status := test.NewTestHost(weightConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "GET"},
			})

			require.Equal(t, types.ActionContinue, action)

			// 验证是否添加了默认标签头
			// 默认标签的设置取决于权重分配的结果
			// 这里只验证插件正常运行

			host.CompleteHttp()
		})

		// 测试正则表达式条件匹配
		t.Run("regex condition match", func(t *testing.T) {
			host, status := test.NewTestHost(regexConditionConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "GET"},
				{"User-Agent", "Mozilla/5.0 (Mobile; CPU iPhone OS 14_0 like Mac OS X) AppleWebKit/605.1.15"},
			})

			require.Equal(t, types.ActionContinue, action)

			// 验证是否添加了流量标签头
			requestHeaders := host.GetRequestHeaders()
			tagHeaderFound := false
			for _, header := range requestHeaders {
				if header[0] == "x-traffic-tag" && header[1] == "regex-match" {
					tagHeaderFound = true
					break
				}
			}
			require.True(t, tagHeaderFound, "Traffic tag header should be added for regex match")

			host.CompleteHttp()
		})

		// 测试百分比条件匹配
		t.Run("percentage condition match", func(t *testing.T) {
			host, status := test.NewTestHost(percentageConditionConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "GET"},
				{"X-User-ID", "user123"},
			})

			require.Equal(t, types.ActionContinue, action)

			// 百分比匹配是基于哈希值的，结果不确定
			// 这里只验证插件正常运行

			host.CompleteHttp()
		})

		// 测试混合配置 - 条件组优先
		t.Run("mixed config - condition group priority", func(t *testing.T) {
			host, status := test.NewTestHost(mixedConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "GET"},
				{"X-Source", "mobile"},
			})

			require.Equal(t, types.ActionContinue, action)

			// 验证是否添加了条件匹配的流量标签头
			requestHeaders := host.GetRequestHeaders()
			tagHeaderFound := false
			for _, header := range requestHeaders {
				if header[0] == "x-traffic-tag" && header[1] == "condition-match" {
					tagHeaderFound = true
					break
				}
			}
			require.True(t, tagHeaderFound, "Condition-based traffic tag header should be added")

			host.CompleteHttp()
		})
		// 测试 Exists 条件匹配：当 Header 存在时 (期望匹配成功)
		t.Run("exists condition match - header present", func(t *testing.T) {
			host, status := test.NewTestHost(existsConditionConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "GET"},
				{"X-User-ID", "user123"}, // 存在目标 Header
			})

			require.Equal(t, types.ActionContinue, action)
			requestHeaders := host.GetRequestHeaders()
			tagHeaderFound := false
			for _, header := range requestHeaders {
				if header[0] == "x-traffic-tag" && header[1] == "exists-match" {
					tagHeaderFound = true
					break
				}
			}
			require.True(t, tagHeaderFound, "Exists-based traffic tag header should be added")
			host.CompleteHttp()
		})

		// 测试 NotExists 条件匹配：当 Header 不存在时 (期望匹配成功)
		t.Run("not_exists condition match - header missing", func(t *testing.T) {
			host, status := test.NewTestHost(notExistsConditionConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "GET"},
			})

			require.Equal(t, types.ActionContinue, action)
			requestHeaders := host.GetRequestHeaders()
			tagHeaderFound := false
			for _, header := range requestHeaders {
				if header[0] == "x-traffic-tag" && header[1] == "not-exists-match" {
					tagHeaderFound = true
					break
				}
			}
			require.True(t, tagHeaderFound, "Exists-based traffic tag header should be added")
			host.CompleteHttp()
		})
	})
}

func TestCompleteFlow(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("complete request flow", func(t *testing.T) {
			host, status := test.NewTestHost(basicConditionConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 处理请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test?param1=value1"},
				{":method", "POST"},
				{"User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"},
				{"Content-Type", "application/json"},
			})

			require.Equal(t, types.ActionContinue, action)

			// 验证是否添加了流量标签头
			requestHeaders := host.GetRequestHeaders()
			tagHeaderFound := false
			for _, header := range requestHeaders {
				if header[0] == "x-traffic-tag" && header[1] == "condition-match" {
					tagHeaderFound = true
					break
				}
			}
			require.True(t, tagHeaderFound, "Traffic tag header should be added")

			host.CompleteHttp()
		})
	})
}
