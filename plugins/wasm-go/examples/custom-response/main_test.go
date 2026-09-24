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

func Test_prefixMatchCode(t *testing.T) {
	rules := map[string]*CustomResponseRule{
		"x01": {},
		"2x3": {},
		"45x": {},
		"6xx": {},
		"x7x": {},
		"xx8": {},
	}

	tests := []struct {
		code      string
		expectHit bool
	}{
		{"101", true},  // 匹配x01
		{"201", true},  // 匹配x01
		{"111", false}, // 不匹配
		{"203", true},  // 匹配2x3
		{"213", true},  // 匹配2x3
		{"450", true},  // 匹配45x
		{"451", true},  // 匹配45x
		{"600", true},  // 匹配6xx
		{"611", true},  // 匹配6xx
		{"612", true},  // 匹配6xx
		{"171", true},  // 匹配x7x
		{"161", false}, // 不匹配
		{"228", true},  // 匹配xx8
		{"229", false}, // 不匹配
		{"123", false}, // 不匹配
	}

	for _, tt := range tests {
		_, found := fuzzyMatchCode(rules, tt.code)
		if found != tt.expectHit {
			t.Errorf("code:%s expect:%v got:%v", tt.code, tt.expectHit, found)
		}
	}
}

func TestIsValidPrefixString(t *testing.T) {
	tests := []struct {
		input    string
		expected string
		hasError bool
	}{
		{"x1x", "x1x", false},
		{"X2X", "x2x", false},
		{"xx1", "xx1", false},
		{"x12", "x12", false},
		{"1x2", "1x2", false},
		{"12x", "12x", false},
		{"123", "", true},  // 缺少x
		{"xxx", "", true},  // 缺少数字
		{"xYx", "", true},  // 非法字符
		{"x1", "", true},   // 长度不足
		{"x123", "", true}, // 长度超限
	}

	for _, tt := range tests {
		result, err := isValidFuzzyMatchString(tt.input)
		if tt.hasError {
			if err == nil {
				t.Errorf("%q: expected error but got none", tt.input)
			}
		} else {
			if err != nil {
				t.Errorf("%q: unexpected error: %v", tt.input, err)
			}
			if result != tt.expected {
				t.Errorf("%q: expected %q, got %q", tt.input, tt.expected, result)
			}
		}
	}
}

// 测试配置：基本配置（老版本）
var basicConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"status_code": 200,
		"headers": []string{
			"Content-Type=application/json",
			"Hello=World",
		},
		"body": `{"hello":"world"}`,
	})
	return data
}()

// 测试配置：带状态码匹配的配置（老版本）
var statusMatchConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"status_code": 302,
		"headers": []string{
			"Location=https://example.com",
		},
		"body": "Redirect to example.com",
		"enable_on_status": []string{
			"429",
		},
	})
	return data
}()

// 测试配置：新版本多规则配置
var multiRulesConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"rules": []map[string]interface{}{
			{
				"body": `{"hello":"world 200"}`,
				"enable_on_status": []string{
					"200",
					"201",
				},
				"headers": []string{
					"key1=value1",
					"key2=value2",
				},
				"status_code": 200,
			},
			{
				"body": `{"hello":"world 404"}`,
				"enable_on_status": []string{
					"404",
				},
				"headers": []string{
					"key1=value1",
					"key2=value2",
				},
				"status_code": 200,
			},
		},
	})
	return data
}()

// 测试配置：模糊匹配配置
var fuzzyMatchConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"rules": []map[string]interface{}{
			{
				"body": `{"hello":"world 200"}`,
				"enable_on_status": []string{
					"200",
				},
				"headers": []string{
					"key1=value1",
					"key2=value2",
				},
				"status_code": 200,
			},
			{
				"body": `{"hello":"world 40x"}`,
				"enable_on_status": []string{
					"40x",
				},
				"headers": []string{
					"key1=value1",
					"key2=value2",
				},
				"status_code": 200,
			},
			{
				"body": `{"hello":"world 4xx"}`,
				"enable_on_status": []string{
					"4xx",
				},
				"headers": []string{
					"key1=value1",
					"key2=value2",
				},
				"status_code": 200,
			},
		},
	})
	return data
}()

// 测试配置：带默认规则的配置
var defaultRuleConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"rules": []map[string]interface{}{
			{
				"body": `{"hello":"world default"}`,
				"headers": []string{
					"key1=value1",
					"key2=value2",
				},
				"status_code": 200,
			},
			{
				"body": `{"hello":"world 404"}`,
				"enable_on_status": []string{
					"404",
				},
				"headers": []string{
					"key1=value1",
					"key2=value2",
				},
				"status_code": 200,
			},
		},
	})
	return data
}()

// 测试配置：纯默认规则配置（没有 enable_on_status）
var pureDefaultRuleConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"rules": []map[string]interface{}{
			{
				"body": `{"hello":"world pure default"}`,
				"headers": []string{
					"key1=value1",
					"key2=value2",
				},
				"status_code": 200,
			},
		},
	})
	return data
}()

// 测试配置：无效配置
var invalidConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"rules": []map[string]interface{}{
			{
				"body": `{"hello":"world"}`,
				"enable_on_status": []string{
					"invalid",
				},
				"headers": []string{
					"key1=value1",
				},
				"status_code": 200,
			},
		},
	})
	return data
}()

func TestParseConfig(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// 测试基本配置解析（老版本）
		t.Run("basic config", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试状态码匹配配置解析（老版本）
		t.Run("status match config", func(t *testing.T) {
			host, status := test.NewTestHost(statusMatchConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试多规则配置解析（新版本）
		t.Run("multi rules config", func(t *testing.T) {
			host, status := test.NewTestHost(multiRulesConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试模糊匹配配置解析
		t.Run("fuzzy match config", func(t *testing.T) {
			host, status := test.NewTestHost(fuzzyMatchConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试带默认规则的配置解析
		t.Run("default rule config", func(t *testing.T) {
			host, status := test.NewTestHost(defaultRuleConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试无效配置解析
		t.Run("invalid config", func(t *testing.T) {
			host, status := test.NewTestHost(invalidConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.Nil(t, config)
		})
	})
}

func TestOnHttpRequestHeaders(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试基本配置的请求头处理（应该使用默认规则）
		t.Run("basic config request headers", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "GET"},
			})

			// 由于没有 enable_on_status 规则，应该使用默认规则并返回 ActionPause
			require.Equal(t, types.ActionPause, action)

			host.CompleteHttp()
		})

		// 测试带状态码匹配的请求头处理（不应该在请求头阶段处理）
		t.Run("status match config request headers", func(t *testing.T) {
			host, status := test.NewTestHost(statusMatchConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "GET"},
			})

			// 由于有 enable_on_status 规则，应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})

		// 测试多规则配置的请求头处理（不应该在请求头阶段处理）
		t.Run("multi rules config request headers", func(t *testing.T) {
			host, status := test.NewTestHost(multiRulesConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "GET"},
			})

			// 由于有 enable_on_status 规则，应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})

		// 测试带默认规则的请求头处理（由于有 enable_on_status 规则，应该返回 ActionContinue）
		t.Run("default rule config request headers", func(t *testing.T) {
			host, status := test.NewTestHost(defaultRuleConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "GET"},
			})

			// 由于有 enable_on_status 规则，应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})

		// 测试纯默认规则的请求头处理（应该使用默认规则并返回 ActionPause）
		t.Run("pure default rule config request headers", func(t *testing.T) {
			host, status := test.NewTestHost(pureDefaultRuleConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "GET"},
			})

			// 由于没有 enable_on_status 规则，应该使用默认规则并返回 ActionPause
			require.Equal(t, types.ActionPause, action)

			host.CompleteHttp()
		})
	})
}

func TestOnHttpResponseHeaders(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试状态码匹配的响应头处理
		t.Run("status match response headers", func(t *testing.T) {
			host, status := test.NewTestHost(statusMatchConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先处理请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "GET"},
			})

			// 处理响应头，状态码为 429（应该匹配规则）
			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "429"},
				{"content-type", "text/plain"},
			})

			// 应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})

		// 测试多规则配置的响应头处理
		t.Run("multi rules response headers", func(t *testing.T) {
			host, status := test.NewTestHost(multiRulesConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先处理请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "GET"},
			})

			// 处理响应头，状态码为 200（应该匹配第一个规则）
			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "text/plain"},
			})

			// 应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})

		// 测试模糊匹配的响应头处理
		t.Run("fuzzy match response headers", func(t *testing.T) {
			host, status := test.NewTestHost(fuzzyMatchConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先处理请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "GET"},
			})

			// 处理响应头，状态码为 404（应该匹配 4xx 规则）
			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "404"},
				{"content-type", "text/plain"},
			})

			// 应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})

		// 测试不匹配状态码的响应头处理
		t.Run("no match response headers", func(t *testing.T) {
			host, status := test.NewTestHost(multiRulesConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先处理请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "GET"},
			})

			// 处理响应头，状态码为 500（不应该匹配任何规则）
			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "500"},
				{"content-type", "text/plain"},
			})

			// 应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})
	})
}
