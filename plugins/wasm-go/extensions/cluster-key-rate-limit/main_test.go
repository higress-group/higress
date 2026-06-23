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
	"fmt"
	"strings"
	"testing"

	"cluster-key-rate-limit/config"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/resp"
)

// 测试配置：全局限流配置
var globalThresholdConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"rule_name": "routeA-global-limit-rule",
		"global_threshold": map[string]interface{}{
			"query_per_minute": 1000,
		},
		"redis": map[string]interface{}{
			"service_name": "redis.static",
			"service_port": 6379,
			"timeout":      1000,
		},
		"show_limit_quota_header": true,
		"rejected_code":           429,
		"rejected_msg":            "Too many requests",
	})
	return data
}()

// 测试配置：基于请求参数的限流配置
var paramLimitConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"rule_name": "routeA-request-param-limit-rule",
		"rule_items": []map[string]interface{}{
			{
				"limit_by_param": "apikey",
				"limit_keys": []map[string]interface{}{
					{
						"key":              "9a342114-ba8a-11ec-b1bf-00163e1250b5",
						"query_per_minute": 10,
					},
					{
						"key":            "a6a6d7f2-ba8a-11ec-bec2-00163e1250b5",
						"query_per_hour": 100,
					},
				},
			},
		},
		"redis": map[string]interface{}{
			"service_name": "redis.static",
			"service_port": 6379,
		},
		"show_limit_quota_header": true,
	})
	return data
}()

// 测试配置：基于请求头的限流配置
var headerLimitConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"rule_name": "routeA-request-header-limit-rule",
		"rule_items": []map[string]interface{}{
			{
				"limit_by_header": "x-ca-key",
				"limit_keys": []map[string]interface{}{
					{
						"key":              "102234",
						"query_per_minute": 10,
					},
					{
						"key":            "308239",
						"query_per_hour": 10,
					},
				},
			},
		},
		"redis": map[string]interface{}{
			"service_name": "redis.static",
			"service_port": 6379,
		},
		"show_limit_quota_header": true,
	})
	return data
}()

// 测试配置：基于 Consumer 的限流配置
var consumerLimitConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"rule_name": "routeA-consumer-limit-rule",
		"rule_items": []map[string]interface{}{
			{
				"limit_by_consumer": "",
				"limit_keys": []map[string]interface{}{
					{
						"key":              "consumer1",
						"query_per_second": 10,
					},
					{
						"key":            "consumer2",
						"query_per_hour": 100,
					},
				},
			},
		},
		"redis": map[string]interface{}{
			"service_name": "redis.static",
			"service_port": 6379,
		},
		"show_limit_quota_header": true,
	})
	return data
}()

// 测试配置：基于 Cookie 的限流配置
var cookieLimitConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"rule_name": "routeA-cookie-limit-rule",
		"rule_items": []map[string]interface{}{
			{
				"limit_by_cookie": "key1",
				"limit_keys": []map[string]interface{}{
					{
						"key":              "value1",
						"query_per_minute": 10,
					},
					{
						"key":            "value2",
						"query_per_hour": 100,
					},
				},
			},
		},
		"redis": map[string]interface{}{
			"service_name": "redis.static",
			"service_port": 6379,
		},
		"show_limit_quota_header": true,
		"rejected_code":           200,
		"rejected_msg":            `{"code":-1,"msg":"Too many requests"}`,
	})
	return data
}()

// 测试配置：基于 IP 的限流配置
var ipLimitConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"rule_name": "routeA-client-ip-limit-rule",
		"rule_items": []map[string]interface{}{
			{
				"limit_by_per_ip": "from-header-x-forwarded-for",
				"limit_keys": []map[string]interface{}{
					{
						"key":           "1.1.1.1",
						"query_per_day": 10,
					},
					{
						"key":           "1.1.1.0/24",
						"query_per_day": 100,
					},
					{
						"key":           "0.0.0.0/0",
						"query_per_day": 1000,
					},
				},
			},
		},
		"redis": map[string]interface{}{
			"service_name": "redis.static",
			"service_port": 6379,
		},
		"show_limit_quota_header": true,
	})
	return data
}()

// 测试配置：正则表达式限流配置
var regexpLimitConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"rule_name": "routeA-regexp-limit-rule",
		"rule_items": []map[string]interface{}{
			{
				"limit_by_per_param": "apikey",
				"limit_keys": []map[string]interface{}{
					{
						"key":              "regexp:^a.*",
						"query_per_second": 10,
					},
					{
						"key":              "regexp:^b.*",
						"query_per_minute": 100,
					},
					{
						"key":            "*",
						"query_per_hour": 1000,
					},
				},
			},
		},
		"redis": map[string]interface{}{
			"service_name": "redis.static",
			"service_port": 6379,
		},
		"show_limit_quota_header": true,
	})
	return data
}()

// 测试配置：混合限流（global + rule_items 同时生效）
var hybridLimitConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"rule_name": "cluster-hybrid",
		"global_threshold": map[string]interface{}{
			"query_per_minute": 10000,
		},
		"rule_items": []map[string]interface{}{
			{
				"limit_by_header": "x-api-key",
				"limit_keys": []map[string]interface{}{
					{"key": "vip-key", "query_per_minute": 100},
				},
			},
		},
		"redis": map[string]interface{}{
			"service_name": "redis.static",
			"service_port": 6379,
		},
		"show_limit_quota_header": true,
	})
	return data
}()

// 测试配置：多条 rule_items 同时命中
var multiRuleItemsConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"rule_name": "cluster-multi-items",
		"rule_items": []map[string]interface{}{
			{
				"limit_by_header": "x-api-key",
				"limit_keys": []map[string]interface{}{
					{"key": "k1", "query_per_minute": 100},
				},
			},
			{
				"limit_by_param": "apikey",
				"limit_keys": []map[string]interface{}{
					{"key": "k2", "query_per_minute": 50},
				},
			},
		},
		"redis": map[string]interface{}{
			"service_name": "redis.static",
			"service_port": 6379,
		},
	})
	return data
}()

func TestParseConfig(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// 测试全局限流配置解析
		t.Run("global threshold config", func(t *testing.T) {
			host, status := test.NewTestHost(globalThresholdConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			cfg, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, cfg)

			// 验证配置内容
			parsedConfig := cfg.(*config.ClusterKeyRateLimitConfig)
			require.Equal(t, "routeA-global-limit-rule", parsedConfig.RuleName)
			require.NotNil(t, parsedConfig.GlobalThreshold)
			require.Equal(t, int64(1000), parsedConfig.GlobalThreshold.Count)
			require.Equal(t, int64(60), parsedConfig.GlobalThreshold.TimeWindow)
			require.True(t, parsedConfig.ShowLimitQuotaHeader)
			require.Equal(t, uint32(429), parsedConfig.RejectedCode)
			require.Equal(t, "Too many requests", parsedConfig.RejectedMsg)
		})

		// 测试基于请求参数的限流配置解析
		t.Run("param limit config", func(t *testing.T) {
			host, status := test.NewTestHost(paramLimitConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			cfg, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, cfg)

			// 验证配置内容
			parsedConfig := cfg.(*config.ClusterKeyRateLimitConfig)
			require.Equal(t, "routeA-request-param-limit-rule", parsedConfig.RuleName)
			require.Len(t, parsedConfig.RuleItems, 1)
			require.Equal(t, config.LimitByParamType, parsedConfig.RuleItems[0].LimitType)
			require.Equal(t, "apikey", parsedConfig.RuleItems[0].Key)
			require.Len(t, parsedConfig.RuleItems[0].ConfigItems, 2)
			require.True(t, parsedConfig.ShowLimitQuotaHeader)
		})

		// 测试基于请求头的限流配置解析
		t.Run("header limit config", func(t *testing.T) {
			host, status := test.NewTestHost(headerLimitConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			cfg, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, cfg)

			// 验证配置内容
			parsedConfig := cfg.(*config.ClusterKeyRateLimitConfig)
			require.Equal(t, "routeA-request-header-limit-rule", parsedConfig.RuleName)
			require.Len(t, parsedConfig.RuleItems, 1)
			require.Equal(t, config.LimitByHeaderType, parsedConfig.RuleItems[0].LimitType)
			require.Equal(t, "x-ca-key", parsedConfig.RuleItems[0].Key)
			require.Len(t, parsedConfig.RuleItems[0].ConfigItems, 2)
			require.True(t, parsedConfig.ShowLimitQuotaHeader)
		})

		// 测试基于 Consumer 的限流配置解析
		t.Run("consumer limit config", func(t *testing.T) {
			host, status := test.NewTestHost(consumerLimitConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			cfg, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, cfg)

			// 验证配置内容
			parsedConfig := cfg.(*config.ClusterKeyRateLimitConfig)
			require.Equal(t, "routeA-consumer-limit-rule", parsedConfig.RuleName)
			require.Len(t, parsedConfig.RuleItems, 1)
			require.Equal(t, config.LimitByConsumerType, parsedConfig.RuleItems[0].LimitType)
			require.Len(t, parsedConfig.RuleItems[0].ConfigItems, 2)
			require.True(t, parsedConfig.ShowLimitQuotaHeader)
		})

		// 测试基于 Cookie 的限流配置解析
		t.Run("cookie limit config", func(t *testing.T) {
			host, status := test.NewTestHost(cookieLimitConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			cfg, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, cfg)

			// 验证配置内容
			parsedConfig := cfg.(*config.ClusterKeyRateLimitConfig)
			require.Equal(t, "routeA-cookie-limit-rule", parsedConfig.RuleName)
			require.Len(t, parsedConfig.RuleItems, 1)
			require.Equal(t, config.LimitByCookieType, parsedConfig.RuleItems[0].LimitType)
			require.Equal(t, "key1", parsedConfig.RuleItems[0].Key)
			require.Len(t, parsedConfig.RuleItems[0].ConfigItems, 2)
			require.True(t, parsedConfig.ShowLimitQuotaHeader)
		})

		// 测试基于 IP 的限流配置解析
		t.Run("ip limit config", func(t *testing.T) {
			host, status := test.NewTestHost(ipLimitConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			cfg, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, cfg)

			// 验证配置内容
			parsedConfig := cfg.(*config.ClusterKeyRateLimitConfig)
			require.Equal(t, "routeA-client-ip-limit-rule", parsedConfig.RuleName)
			require.Len(t, parsedConfig.RuleItems, 1)
			require.Equal(t, config.LimitByPerIpType, parsedConfig.RuleItems[0].LimitType)
			require.NotNil(t, parsedConfig.RuleItems[0].LimitByPerIp)
			require.Equal(t, config.HeaderSourceType, parsedConfig.RuleItems[0].LimitByPerIp.SourceType)
			require.True(t, parsedConfig.ShowLimitQuotaHeader)
		})

		// 测试正则表达式限流配置解析
		t.Run("regexp limit config", func(t *testing.T) {
			host, status := test.NewTestHost(regexpLimitConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			cfg, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, cfg)

			// 验证配置内容
			parsedConfig := cfg.(*config.ClusterKeyRateLimitConfig)
			require.Equal(t, "routeA-regexp-limit-rule", parsedConfig.RuleName)
			require.Len(t, parsedConfig.RuleItems, 1)
			require.Equal(t, config.LimitByPerParamType, parsedConfig.RuleItems[0].LimitType)
			require.Equal(t, "apikey", parsedConfig.RuleItems[0].Key)
			require.Len(t, parsedConfig.RuleItems[0].ConfigItems, 3)
			require.Equal(t, config.RegexpType, parsedConfig.RuleItems[0].ConfigItems[0].ConfigType)
			require.True(t, parsedConfig.ShowLimitQuotaHeader)
		})
	})
}

func TestOnHttpRequestHeaders(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试全局限流请求头处理
		t.Run("global threshold request headers", func(t *testing.T) {
			host, status := test.NewTestHost(globalThresholdConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
			})

			// 由于需要调用 Redis，应该返回 HeaderStopAllIterationAndWatermark
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			resp := multiRuleResp([3]int{1000, 999, 60})
			// 模拟 Redis 调用响应（允许请求）
			host.CallOnRedisCall(0, resp)

			host.CompleteHttp()
		})

		// 测试基于请求参数的限流请求头处理
		t.Run("param limit request headers", func(t *testing.T) {
			host, status := test.NewTestHost(paramLimitConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，包含查询参数
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test?apikey=9a342114-ba8a-11ec-b1bf-00163e1250b5"},
				{":method", "GET"},
			})

			// 由于需要调用 Redis，应该返回 HeaderStopAllIterationAndWatermark
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			// 模拟 Redis 调用响应（允许请求）
			resp := multiRuleResp([3]int{10, 9, 60})
			host.CallOnRedisCall(0, resp)

			host.CompleteHttp()
		})

		// 测试基于请求头的限流请求头处理
		t.Run("header limit request headers", func(t *testing.T) {
			host, status := test.NewTestHost(headerLimitConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，包含限流键
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"x-ca-key", "102234"},
			})

			// 由于需要调用 Redis，应该返回 HeaderStopAllIterationAndWatermark
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			// 模拟 Redis 调用响应（允许请求）
			resp := multiRuleResp([3]int{10, 9, 60})
			host.CallOnRedisCall(0, resp)

			host.CompleteHttp()
		})

		// 测试基于 Consumer 的限流请求头处理
		t.Run("consumer limit request headers", func(t *testing.T) {
			host, status := test.NewTestHost(consumerLimitConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，包含 consumer 信息
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"x-mse-consumer", "consumer1"},
			})

			// 由于需要调用 Redis，应该返回 HeaderStopAllIterationAndWatermark
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			// 模拟 Redis 调用响应（允许请求）
			resp := multiRuleResp([3]int{10, 9, 1})
			host.CallOnRedisCall(0, resp)

			host.CompleteHttp()
		})

		// 测试基于 Cookie 的限流请求头处理
		t.Run("cookie limit request headers", func(t *testing.T) {
			host, status := test.NewTestHost(cookieLimitConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，包含 cookie
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"cookie", "key1=value1; other=value"},
			})

			// 由于需要调用 Redis，应该返回 HeaderStopAllIterationAndWatermark
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			// 模拟 Redis 调用响应（允许请求）
			resp := multiRuleResp([3]int{10, 9, 60})
			host.CallOnRedisCall(0, resp)

			host.CompleteHttp()
		})

		// 测试基于 IP 的限流请求头处理
		t.Run("ip limit request headers", func(t *testing.T) {
			host, status := test.NewTestHost(ipLimitConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，包含 IP 信息
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"x-forwarded-for", "1.1.1.1"},
			})

			// 由于需要调用 Redis，应该返回 HeaderStopAllIterationAndWatermark
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			// 模拟 Redis 调用响应（允许请求）
			resp := multiRuleResp([3]int{10, 9, 86400})
			host.CallOnRedisCall(0, resp)

			host.CompleteHttp()
		})

		// 测试限流触发的情况
		t.Run("rate limit exceeded", func(t *testing.T) {
			host, status := test.NewTestHost(globalThresholdConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
			})

			// 由于需要调用 Redis，应该返回 HeaderStopAllIterationAndWatermark
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			// 模拟 Redis 调用响应（触发限流）
			// 当前请求数(1001)超过阈值(1000)，触发限流
			resp := multiRuleResp([3]int{1000, 1001, 60})
			host.CallOnRedisCall(0, resp)

			// 检查是否发送了限流响应
			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			require.Equal(t, uint32(429), localResponse.StatusCode)
			require.Contains(t, string(localResponse.Data), "Too many requests")

			host.CompleteHttp()
		})

		// 混合限流：global + rule_item 同时命中
		t.Run("hybrid limit both match", func(t *testing.T) {
			host, status := test.NewTestHost(hybridLimitConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"x-api-key", "vip-key"},
			})
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			resp := multiRuleResp(
				[3]int{10000, 1, 60},
				[3]int{100, 1, 60},
			)
			host.CallOnRedisCall(0, resp)

			host.CompleteHttp()
		})

		// 多条 rule_items 同时命中
		t.Run("multi rule_items all match", func(t *testing.T) {
			host, status := test.NewTestHost(multiRuleItemsConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test?apikey=k2"},
				{":method", "GET"},
				{"x-api-key", "k1"},
			})
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			resp := multiRuleResp(
				[3]int{100, 1, 60},
				[3]int{50, 1, 60},
			)
			host.CallOnRedisCall(0, resp)

			host.CompleteHttp()
		})

		// 混合限流：global 触发
		t.Run("hybrid limit global triggered", func(t *testing.T) {
			host, status := test.NewTestHost(hybridLimitConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"x-api-key", "vip-key"},
			})
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			resp := multiRuleResp(
				[3]int{10000, 10001, 60},
				[3]int{100, 1, 60},
			)
			host.CallOnRedisCall(0, resp)

			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			require.Equal(t, uint32(429), localResponse.StatusCode)

			host.CompleteHttp()
		})

		// Redis 响应数组长度与规则数不匹配
		t.Run("redis response length mismatch", func(t *testing.T) {
			host, status := test.NewTestHost(hybridLimitConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"x-api-key", "vip-key"},
			})

			resp := multiRuleResp([3]int{10000, 1, 60}) // 期望 2 条，返回 1 条
			host.CallOnRedisCall(0, resp)

			require.Nil(t, host.GetLocalResponse())
			host.CompleteHttp()
		})

		// Redis 子数组长度异常（少于 3）
		t.Run("redis sub-array length mismatch", func(t *testing.T) {
			host, status := test.NewTestHost(hybridLimitConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"x-api-key", "vip-key"},
			})

			badResp, err := resp.ArrayValue([]resp.Value{
				resp.ArrayValue([]resp.Value{
					resp.IntegerValue(10000),
					resp.IntegerValue(1),
				}),
			}).MarshalRESP() // 关键：用 MarshalRESP() 而非 Bytes()
			require.NoError(t, err)
			host.CallOnRedisCall(0, badResp)

			require.Nil(t, host.GetLocalResponse())
			host.CompleteHttp()
		})

		// 混合配置：global 命中，rule_items 不命中
		t.Run("hybrid limit rule_items no match", func(t *testing.T) {
			host, status := test.NewTestHost(hybridLimitConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
			})
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			resp := multiRuleResp([3]int{10000, 1, 60})
			host.CallOnRedisCall(0, resp)

			require.Nil(t, host.GetLocalResponse())
			host.CompleteHttp()
		})

		// 多规则同时触发，验证 rejected 报告 global（reset=60 而非 rule_item 的 30）
		t.Run("hybrid limit both triggered reports global first", func(t *testing.T) {
			host, status := test.NewTestHost(hybridLimitConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"x-api-key", "vip-key"},
			})

			resp := multiRuleResp(
				[3]int{10000, 10001, 60},
				[3]int{100, 101, 30},
			)
			host.CallOnRedisCall(0, resp)

			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			require.Equal(t, uint32(429), localResponse.StatusCode)

			// LocalHttpResponse.Headers 类型为 [][2]string，按切片迭代查找目标头
			var resetHeader string
			for _, h := range localResponse.Headers {
				if strings.EqualFold(h[0], "X-RateLimit-Reset") {
					resetHeader = h[1]
					break
				}
			}
			require.Equal(t, "60", resetHeader, "应报告 global 规则的 reset 时间")

			host.CompleteHttp()
		})

		// 多规则未触发，剩余比例不同 → LimitContext 取 tightest
		// cluster-key 可以通过 X-RateLimit-* 响应头验证（ai-token 不能）
		t.Run("multi-rule no trigger takes tightest", func(t *testing.T) {
			host, status := test.NewTestHost(hybridLimitConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"x-api-key", "vip-key"},
			})

			resp := multiRuleResp(
				[3]int{10000, 9000, 60}, // global: 剩余 10%
				[3]int{100, 10, 60},     // rule_item: 剩余 90%
			)
			host.CallOnRedisCall(0, resp)

			require.Nil(t, host.GetLocalResponse())

			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})

			responseHeaders := host.GetResponseHeaders()
			require.True(t, test.HasHeader(responseHeaders, "x-ratelimit-limit"))
			require.True(t, test.HasHeader(responseHeaders, "x-ratelimit-remaining"))

			for _, h := range responseHeaders {
				if strings.EqualFold(h[0], "x-ratelimit-limit") {
					require.Equal(t, "10000", h[1], "X-RateLimit-Limit 应为 tightest(global) 的 threshold")
				}
				if strings.EqualFold(h[0], "x-ratelimit-remaining") {
					require.Equal(t, "1000", h[1], "X-RateLimit-Remaining 应为 tightest(global) 的 threshold-current")
				}
			}

			host.CompleteHttp()
		})
	})
}

func TestOnHttpResponseHeaders(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试显示限流配额的响应头处理
		t.Run("show limit quota headers", func(t *testing.T) {
			host, status := test.NewTestHost(globalThresholdConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先处理请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
			})

			// 模拟 Redis 调用响应
			resp := multiRuleResp([3]int{1000, 999, 60})
			host.CallOnRedisCall(0, resp)

			// 处理响应头
			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})

			// 应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			// 验证是否添加了限流配额响应头
			responseHeaders := host.GetResponseHeaders()
			require.True(t, test.HasHeader(responseHeaders, "x-ratelimit-limit"))
			require.True(t, test.HasHeader(responseHeaders, "x-ratelimit-remaining"))

			host.CompleteHttp()
		})

		// 测试不显示限流配额的响应头处理
		t.Run("hide limit quota headers", func(t *testing.T) {
			// 创建不显示限流配额的配置
			hideQuotaConfig := func() json.RawMessage {
				data, _ := json.Marshal(map[string]interface{}{
					"rule_name": "routeA-global-limit-rule",
					"global_threshold": map[string]interface{}{
						"query_per_minute": 1000,
					},
					"redis": map[string]interface{}{
						"service_name": "redis.static",
						"service_port": 6379,
					},
					"show_limit_quota_header": false,
				})
				return data
			}()

			host, status := test.NewTestHost(hideQuotaConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先处理请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
			})

			// 模拟 Redis 调用响应
			resp := multiRuleResp([3]int{1000, 999, 60})
			host.CallOnRedisCall(0, resp)

			// 处理响应头
			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})

			// 应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			// 验证是否没有添加限流配额响应头
			responseHeaders := host.GetResponseHeaders()
			require.False(t, test.HasHeader(responseHeaders, "x-ratelimit-limit"))
			require.False(t, test.HasHeader(responseHeaders, "x-ratelimit-remaining"))

			host.CompleteHttp()
		})
	})
}

func TestCompleteFlow(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("complete rate limit flow", func(t *testing.T) {
			host, status := test.NewTestHost(globalThresholdConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 1. 处理请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
			})

			// 由于需要调用 Redis，应该返回 HeaderStopAllIterationAndWatermark
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			// 2. 模拟 Redis 调用响应
			resp := multiRuleResp([3]int{1000, 1, 60})
			host.CallOnRedisCall(0, resp)

			// 3. 处理响应头
			action = host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})

			// 应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			// 验证完整的限流流程
			responseHeaders := host.GetResponseHeaders()

			// 验证是否添加了必要的限流响应头
			require.True(t, test.HasHeader(responseHeaders, "x-ratelimit-limit"))
			require.True(t, test.HasHeader(responseHeaders, "x-ratelimit-remaining"))

			host.CompleteHttp()
		})
	})
}

// multiRuleResp 构建多规则嵌套 Redis 响应（RESP wire format）。
// test.CreateRedisRespArray 不支持嵌套数组，因此直接用 resp 包构造。
// 每个 [3]int 元组为 {threshold, current, ttl}。
// 注意：不能使用 resp.Value.Bytes()——它返回的是显示用字符串，不是 RESP wire 格式。
// 必须用 resp.Writer.WriteArray 或 Value.MarshalRESP() 输出真正的 RESP 字节流。
func multiRuleResp(items ...[3]int) []byte {
	values := make([]resp.Value, len(items))
	for i, it := range items {
		values[i] = resp.ArrayValue([]resp.Value{
			resp.IntegerValue(it[0]),
			resp.IntegerValue(it[1]),
			resp.IntegerValue(it[2]),
		})
	}
	b, err := resp.ArrayValue(values).MarshalRESP()
	if err != nil {
		panic(fmt.Sprintf("failed to marshal multiRuleResp: %v", err))
	}
	return b
}
