// Copyright (c) 2024 Alibaba Group Holding Ltd.
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

// 测试配置：基本配置（默认值）
var basicConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{})
	return data
}()

// 测试配置：自定义阻止状态码和消息
var customBlockConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"blocked_code":    429,
		"blocked_message": "Too Many Requests - Bot Detected",
	})
	return data
}()

// 测试配置：允许规则配置
var allowRulesConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"allow": []string{
			".*Go-http-client.*",
			".*Python-requests.*",
			".*curl.*",
		},
	})
	return data
}()

// 测试配置：拒绝规则配置
var denyRulesConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"deny": []string{
			"spd-tools.*",
			"malicious-bot.*",
			".*scraper.*",
		},
	})
	return data
}()

// 测试配置：混合规则配置
var mixedRulesConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"allow": []string{
			".*Go-http-client.*",
			".*Python-requests.*",
		},
		"deny": []string{
			"spd-tools.*",
			"malicious-bot.*",
		},
		"blocked_code":    418,
		"blocked_message": "I'm a teapot - Bot Detected",
	})
	return data
}()

// 测试配置：无效正则表达式配置
var invalidRegexConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"deny": []string{
			"[invalid-regex",
		},
	})
	return data
}()

func TestParseConfig(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// 测试基本配置解析（默认值）
		t.Run("basic config", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试自定义阻止状态码和消息配置解析
		t.Run("custom block config", func(t *testing.T) {
			host, status := test.NewTestHost(customBlockConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试允许规则配置解析
		t.Run("allow rules config", func(t *testing.T) {
			host, status := test.NewTestHost(allowRulesConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试拒绝规则配置解析
		t.Run("deny rules config", func(t *testing.T) {
			host, status := test.NewTestHost(denyRulesConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试混合规则配置解析
		t.Run("mixed rules config", func(t *testing.T) {
			host, status := test.NewTestHost(mixedRulesConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试无效正则表达式配置解析
		t.Run("invalid regex config", func(t *testing.T) {
			host, status := test.NewTestHost(invalidRegexConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})
	})
}

func TestOnHttpRequestHeaders(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试正常 User-Agent 请求头处理
		t.Run("normal user agent", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，包含正常的 User-Agent
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"user-agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"},
			})

			// 应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})

		// 测试默认爬虫检测（Googlebot）
		t.Run("default bot detection - googlebot", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，包含 Googlebot User-Agent
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"user-agent", "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)"},
			})

			// 应该返回 ActionPause，因为被识别为爬虫
			require.Equal(t, types.ActionPause, action)

			// 验证是否发送了阻止响应
			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			require.Equal(t, uint32(403), localResponse.StatusCode)
			require.Equal(t, "Invalid User-Agent", string(localResponse.Data))

			host.CompleteHttp()
		})

		// 测试默认爬虫检测（BaiduSpider）
		t.Run("default bot detection - baiduspider", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，包含 BaiduSpider User-Agent
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"user-agent", "Mozilla/5.0 (compatible; Baiduspider/2.0; +http://www.baidu.com/search/spider.html)"},
			})

			// 应该返回 ActionPause，因为被识别为爬虫
			require.Equal(t, types.ActionPause, action)

			// 验证是否发送了阻止响应
			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			require.Equal(t, uint32(403), localResponse.StatusCode)
			require.Equal(t, "Invalid User-Agent", string(localResponse.Data))

			host.CompleteHttp()
		})

		// 测试允许规则（Go-http-client）
		t.Run("allow rule - go-http-client", func(t *testing.T) {
			host, status := test.NewTestHost(allowRulesConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，包含 Go-http-client User-Agent
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"user-agent", "Go-http-client/1.1"},
			})

			// 应该返回 ActionContinue，因为被允许规则匹配
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})

		// 测试允许规则（Python-requests）
		t.Run("allow rule - python-requests", func(t *testing.T) {
			host, status := test.NewTestHost(allowRulesConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，包含 Python-requests User-Agent
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"user-agent", "python-requests/2.28.1"},
			})

			// 应该返回 ActionContinue，因为被允许规则匹配
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})

		// 测试拒绝规则（spd-tools）
		t.Run("deny rule - spd-tools", func(t *testing.T) {
			host, status := test.NewTestHost(denyRulesConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，包含 spd-tools User-Agent
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"user-agent", "spd-tools/1.1"},
			})

			// 应该返回 ActionPause，因为被拒绝规则匹配
			require.Equal(t, types.ActionPause, action)

			// 验证是否发送了阻止响应
			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			require.Equal(t, uint32(403), localResponse.StatusCode)
			require.Equal(t, "Invalid User-Agent", string(localResponse.Data))

			host.CompleteHttp()
		})

		// 测试拒绝规则（malicious-bot）
		t.Run("deny rule - malicious-bot", func(t *testing.T) {
			host, status := test.NewTestHost(denyRulesConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，包含 malicious-bot User-Agent
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"user-agent", "malicious-bot/2.0"},
			})

			// 应该返回 ActionPause，因为被拒绝规则匹配
			require.Equal(t, types.ActionPause, action)

			// 验证是否发送了阻止响应
			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			require.Equal(t, uint32(403), localResponse.StatusCode)
			require.Equal(t, "Invalid User-Agent", string(localResponse.Data))

			host.CompleteHttp()
		})

		// 测试混合规则配置
		t.Run("mixed rules config", func(t *testing.T) {
			host, status := test.NewTestHost(mixedRulesConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 测试允许规则（Go-http-client）
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"user-agent", "Go-http-client/1.1"},
			})

			// 应该返回 ActionContinue，因为被允许规则匹配
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()

			// 测试拒绝规则（spd-tools）
			action = host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"user-agent", "spd-tools/1.1"},
			})

			// 应该返回 ActionPause，因为被拒绝规则匹配
			require.Equal(t, types.ActionPause, action)

			// 验证是否发送了自定义阻止响应
			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			require.Equal(t, uint32(418), localResponse.StatusCode)
			require.Equal(t, "I'm a teapot - Bot Detected", string(localResponse.Data))

			host.CompleteHttp()
		})

		// 测试缺少 User-Agent 的情况
		t.Run("missing user agent", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，不包含 User-Agent
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
			})

			// 应该返回 ActionPause，因为缺少 User-Agent
			require.Equal(t, types.ActionPause, action)

			host.CompleteHttp()
		})

		// 测试空 User-Agent 的情况
		t.Run("empty user agent", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，包含空的 User-Agent
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"user-agent", ""},
			})

			// 应该返回 ActionPause，因为 User-Agent 为空
			require.Equal(t, types.ActionPause, action)

			host.CompleteHttp()
		})
	})
}

func TestCompleteFlow(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("complete bot detection flow", func(t *testing.T) {
			host, status := test.NewTestHost(mixedRulesConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 1. 测试正常请求通过
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"user-agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"},
			})

			// 应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()

			// 2. 测试爬虫请求被阻止
			action = host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"user-agent", "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)"},
			})

			// 应该返回 ActionPause，因为被识别为爬虫
			require.Equal(t, types.ActionPause, action)

			// 验证是否发送了阻止响应
			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			require.Equal(t, uint32(418), localResponse.StatusCode)
			require.Equal(t, "I'm a teapot - Bot Detected", string(localResponse.Data))

			host.CompleteHttp()
		})
	})
}
