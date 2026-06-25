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

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/resp"
)

// 测试配置：全局限流配置
var globalThresholdConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"rule_name": "ai-token-global-limit",
		"global_threshold": map[string]interface{}{
			"token_per_minute": 1000,
		},
		"redis": map[string]interface{}{
			"service_name": "redis.static",
			"service_port": 6379,
			"timeout":      1000,
		},
		"rejected_code": 429,
		"rejected_msg":  "Too many AI token requests",
	})
	return data
}()

// 测试配置：基于请求头的限流配置
var headerLimitConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"rule_name": "ai-token-header-limit",
		"rule_items": []map[string]interface{}{
			{
				"limit_by_header": "x-api-key",
				"limit_keys": []map[string]interface{}{
					{
						"key":              "test-key-123",
						"token_per_minute": 100,
					},
				},
			},
		},
		"redis": map[string]interface{}{
			"service_name": "redis.static",
			"service_port": 6379,
		},
		"rejected_code": 429,
		"rejected_msg":  "API key rate limit exceeded",
	})
	return data
}()

// 测试配置：基于请求参数的限流配置
var paramLimitConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"rule_name": "ai-token-param-limit",
		"rule_items": []map[string]interface{}{
			{
				"limit_by_param": "apikey",
				"limit_keys": []map[string]interface{}{
					{
						"key":              "param-key-456",
						"token_per_minute": 50,
					},
				},
			},
		},
		"redis": map[string]interface{}{
			"service_name": "redis.static",
			"service_port": 6379,
		},
		"rejected_code": 429,
		"rejected_msg":  "Parameter rate limit exceeded",
	})
	return data
}()

// 测试配置：基于 Consumer 的限流配置
var consumerLimitConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"rule_name": "ai-token-consumer-limit",
		"rule_items": []map[string]interface{}{
			{
				"limit_by_consumer": "",
				"limit_keys": []map[string]interface{}{
					{
						"key":              "consumer1",
						"token_per_minute": 200,
					},
				},
			},
		},
		"redis": map[string]interface{}{
			"service_name": "redis.static",
			"service_port": 6379,
		},
		"rejected_code": 429,
		"rejected_msg":  "Consumer rate limit exceeded",
	})
	return data
}()

// 测试配置：基于 Cookie 的限流配置
var cookieLimitConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"rule_name": "ai-token-cookie-limit",
		"rule_items": []map[string]interface{}{
			{
				"limit_by_cookie": "session-id",
				"limit_keys": []map[string]interface{}{
					{
						"key":              "session-789",
						"token_per_minute": 75,
					},
				},
			},
		},
		"redis": map[string]interface{}{
			"service_name": "redis.static",
			"service_port": 6379,
		},
		"rejected_code": 429,
		"rejected_msg":  "Session rate limit exceeded",
	})
	return data
}()

// 测试配置：基于 IP 的限流配置
var ipLimitConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"rule_name": "ai-token-ip-limit",
		"rule_items": []map[string]interface{}{
			{
				"limit_by_per_ip": "from-remote-addr",
				"limit_keys": []map[string]interface{}{
					{
						"key":              "192.168.1.0/24",
						"token_per_minute": 300,
					},
				},
			},
		},
		"redis": map[string]interface{}{
			"service_name": "redis.static",
			"service_port": 6379,
		},
		"rejected_code": 429,
		"rejected_msg":  "IP rate limit exceeded",
	})
	return data
}()

// 测试配置：正则表达式限流配置
var regexpLimitConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"rule_name": "ai-token-regexp-limit",
		"rule_items": []map[string]interface{}{
			{
				"limit_by_per_header": "x-user-id",
				"limit_keys": []map[string]interface{}{
					{
						"key":              "regexp:^user-\\d+$",
						"token_per_minute": 150,
					},
				},
			},
		},
		"redis": map[string]interface{}{
			"service_name": "redis.static",
			"service_port": 6379,
		},
		"rejected_code": 429,
		"rejected_msg":  "User ID rate limit exceeded",
	})
	return data
}()

// 测试配置：混合限流（global_threshold + rule_items 同时配置）
var hybridLimitConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"rule_name": "ai-token-hybrid",
		"global_threshold": map[string]interface{}{
			"token_per_minute": 10000,
		},
		"rule_items": []map[string]interface{}{
			{
				"limit_by_header": "x-api-key",
				"limit_keys": []map[string]interface{}{
					{
						"key":              "vip-key",
						"token_per_minute": 100,
					},
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

// 测试配置：多条 rule_items 同时命中
var multiRuleItemsConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"rule_name": "ai-token-multi-items",
		"rule_items": []map[string]interface{}{
			{
				"limit_by_header": "x-api-key",
				"limit_keys": []map[string]interface{}{
					{"key": "k1", "token_per_minute": 100},
				},
			},
			{
				"limit_by_param": "apikey",
				"limit_keys": []map[string]interface{}{
					{"key": "k2", "token_per_minute": 50},
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

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试基于请求头的限流配置解析
		t.Run("header limit config", func(t *testing.T) {
			host, status := test.NewTestHost(headerLimitConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试基于请求参数的限流配置解析
		t.Run("param limit config", func(t *testing.T) {
			host, status := test.NewTestHost(paramLimitConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试基于 Consumer 的限流配置解析
		t.Run("consumer limit config", func(t *testing.T) {
			host, status := test.NewTestHost(consumerLimitConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试基于 Cookie 的限流配置解析
		t.Run("cookie limit config", func(t *testing.T) {
			host, status := test.NewTestHost(cookieLimitConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试基于 IP 的限流配置解析
		t.Run("ip limit config", func(t *testing.T) {
			host, status := test.NewTestHost(ipLimitConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试正则表达式限流配置解析
		t.Run("regexp limit config", func(t *testing.T) {
			host, status := test.NewTestHost(regexpLimitConfig)
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
		// 测试全局限流请求头处理
		t.Run("global threshold request headers", func(t *testing.T) {
			host, status := test.NewTestHost(globalThresholdConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "POST"},
			})

			// 由于需要调用 Redis，应该返回 HeaderStopAllIterationAndWatermark
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			// 模拟 Redis 调用响应（允许请求）
			// 返回 [[threshold, current, ttl]] 嵌套格式
			resp := multiRuleResp([3]int{1000, 1, 60})
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
				{":method", "POST"},
				{"x-api-key", "test-key-123"},
			})

			// 由于需要调用 Redis，应该返回 HeaderStopAllIterationAndWatermark
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			// 模拟 Redis 调用响应（允许请求）
			resp := multiRuleResp([3]int{100, 1, 60})
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
				{":path", "/api/test?apikey=param-key-456"},
				{":method", "POST"},
			})

			// 由于需要调用 Redis，应该返回 HeaderStopAllIterationAndWatermark
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			// 模拟 Redis 调用响应（允许请求）
			resp := multiRuleResp([3]int{50, 1, 60})
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
				{":method", "POST"},
				{"x-mse-consumer", "consumer1"},
			})

			// 由于需要调用 Redis，应该返回 HeaderStopAllIterationAndWatermark
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			// 模拟 Redis 调用响应（允许请求）
			resp := multiRuleResp([3]int{200, 1, 60})
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
				{":method", "POST"},
				{"cookie", "session-id=session-789; other=value"},
			})

			// 由于需要调用 Redis，应该返回 HeaderStopAllIterationAndWatermark
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			// 模拟 Redis 调用响应（允许请求）
			resp := multiRuleResp([3]int{75, 1, 60})
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
				{":method", "POST"},
			})

			// 由于需要调用 Redis，应该返回 HeaderStopAllIterationAndWatermark
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			// 模拟 Redis 调用响应（触发限流）
			// 返回 [[threshold, current, ttl]] 嵌套格式，current > threshold 表示触发限流
			resp := multiRuleResp([3]int{1000, 1001, 60})
			host.CallOnRedisCall(0, resp)

			// 检查是否发送了限流响应
			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			require.Equal(t, uint32(429), localResponse.StatusCode)
			require.Contains(t, string(localResponse.Data), "Too many AI token requests")

			host.CompleteHttp()
		})

		// 测试没有匹配到限流规则的情况
		t.Run("no matching limit rule", func(t *testing.T) {
			host, status := test.NewTestHost(headerLimitConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，但不包含限流键
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "POST"},
				// 不包含 x-api-key 头
			})

			// 应该返回 ActionContinue，因为没有匹配到限流规则
			require.Equal(t, types.ActionContinue, action)
		})

		// 混合限流：global + rule_item 同时命中，返回 2-key Eval 响应
		t.Run("hybrid limit both match", func(t *testing.T) {
			host, status := test.NewTestHost(hybridLimitConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "POST"},
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
				{":method", "POST"},
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

		// 混合限流：global 触发（current > threshold），rule_item 未触发
		t.Run("hybrid limit global triggered", func(t *testing.T) {
			host, status := test.NewTestHost(hybridLimitConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "POST"},
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

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "POST"},
				{"x-api-key", "vip-key"},
			})
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			resp := multiRuleResp([3]int{10000, 1, 60})
			host.CallOnRedisCall(0, resp)

			require.Nil(t, host.GetLocalResponse())

			host.CompleteHttp()
		})

		// Redis 子数组长度异常（少于 3）
		t.Run("redis sub-array length mismatch", func(t *testing.T) {
			host, status := test.NewTestHost(hybridLimitConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "POST"},
				{"x-api-key", "vip-key"},
			})
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			badResp, err := resp.ArrayValue([]resp.Value{
				resp.ArrayValue([]resp.Value{
					resp.IntegerValue(10000),
					resp.IntegerValue(1),
				}),
			}).MarshalRESP()
			require.NoError(t, err)
			host.CallOnRedisCall(0, badResp)

			require.Nil(t, host.GetLocalResponse())

			host.CompleteHttp()
		})

		// 混合配置：global 命中，rule_items 不命中（请求头不含 x-api-key）
		t.Run("hybrid limit rule_items no match", func(t *testing.T) {
			host, status := test.NewTestHost(hybridLimitConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "POST"},
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

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "POST"},
				{"x-api-key", "vip-key"},
			})
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

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
				if strings.EqualFold(h[0], RateLimitResetHeader) && h[1] != "" {
					resetHeader = h[1]
					break
				}
			}
			require.Equal(t, "60", resetHeader, "应报告 global 规则的 reset 时间")

			host.CompleteHttp()
		})

		// 多规则未触发：ai-token-ratelimit 不对外暴露 tightest 选择（无 X-RateLimit-* 头），
		// 此处仅验证不触发拒绝；tightest 行为由 cluster-key-ratelimit 的测试覆盖。
		t.Run("multi-rule no trigger does not reject", func(t *testing.T) {
			host, status := test.NewTestHost(hybridLimitConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "POST"},
				{"x-api-key", "vip-key"},
			})
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			resp := multiRuleResp(
				[3]int{10000, 9000, 60},
				[3]int{100, 10, 60},
			)
			host.CallOnRedisCall(0, resp)

			require.Nil(t, host.GetLocalResponse())

			host.CompleteHttp()
		})
	})
}

func TestOnHttpStreamingBody(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试流式响应体处理（包含 token 统计）
		t.Run("streaming body with token usage", func(t *testing.T) {
			host, status := test.NewTestHost(globalThresholdConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先处理请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "POST"},
			})

			// 模拟 Redis 调用响应
			resp := multiRuleResp([3]int{1000, 1, 60})
			host.CallOnRedisCall(0, resp)

			// 处理流式响应体
			// 模拟包含 token 统计信息的响应体
			responseBody := []byte(`{"choices":[{"message":{"content":"Hello, how can I help you?"}}],"usage":{"prompt_tokens":10,"completion_tokens":15,"total_tokens":25}}`)
			action := host.CallOnHttpStreamingRequestBody(responseBody, false) // 不是最后一个块

			result := host.GetRequestBody()
			require.Equal(t, responseBody, result)
			// 应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			// 处理最后一个块
			lastChunk := []byte(`{"choices":[{"message":{"content":"How can I help you?"}}],"usage":{"prompt_tokens":10,"completion_tokens":15,"total_tokens":25}}`)
			action = host.CallOnHttpStreamingRequestBody(lastChunk, true) // 最后一个块

			result = host.GetRequestBody()
			require.Equal(t, lastChunk, result)

			// 应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})

		// 测试流式响应体处理（不包含 token 统计）
		t.Run("streaming body without token usage", func(t *testing.T) {
			host, status := test.NewTestHost(globalThresholdConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先处理请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "POST"},
			})

			// 模拟 Redis 调用响应
			resp := multiRuleResp([3]int{1000, 1, 60})
			host.CallOnRedisCall(0, resp)

			// 处理流式响应体
			// 模拟不包含 token 统计信息的响应体
			responseBody := []byte(`{"message": "Hello, world!"}`)
			action := host.CallOnHttpStreamingRequestBody(responseBody, true) // 最后一个块

			result := host.GetRequestBody()
			require.Equal(t, responseBody, result)
			// 应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})

		// 多规则下响应阶段应触发 2-key INCRBY。
		// Strengthened: 用 GetRedisCalloutAttributes() 断言响应阶段确实发起了一次 Eval。
		// Callout 计数模式：0 → 1 (请求阶段) → 0 (CallOnRedisCall 消费) → 1 (响应阶段)
		// 如果 response 阶段被改回 1-key 或不调用 Redis，最终计数会停在 0 而非 1。
		t.Run("streaming body multi-rule incrby", func(t *testing.T) {
			host, status := test.NewTestHost(hybridLimitConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			require.Equal(t, 0, len(host.GetRedisCalloutAttributes()),
				"请求前应无 Redis 调用")

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "POST"},
				{"x-api-key", "vip-key"},
			})
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)
			require.Equal(t, 1, len(host.GetRedisCalloutAttributes()),
				"请求阶段应发起 1 次多键 Eval")

			resp := multiRuleResp(
				[3]int{10000, 1, 60},
				[3]int{100, 1, 60},
			)
			host.CallOnRedisCall(0, resp)

			body := []byte(`{"choices":[{"message":{"content":"hi"}}],"usage":{"prompt_tokens":10,"completion_tokens":15,"total_tokens":25}}`)
			// 注意：ai-token 的 onHttpStreamingBody 注册为 ProcessStreamingResponseBody，
			// 对应测试方法 CallOnHttpStreamingResponseBody（不是 CallOnHttpStreamingRequestBody）。
			// 这是因为 ai-token 处理的是 LLM 的流式响应体（usage 字段在响应里）。
			streamAction := host.CallOnHttpStreamingResponseBody(body, true)
			require.Equal(t, types.ActionContinue, streamAction)
			// 响应阶段应再发起 1 次多键 INCRBY（callout 数 0 → 1）。
			// 如果 response 阶段被改回 1-key 或不调用 Redis，此断言会失败。
			require.Equal(t, 1, len(host.GetRedisCalloutAttributes()),
				"响应阶段应再发起 1 次多键 INCRBY（callout 数 0 → 1）")

			host.CompleteHttp()
		})

		// 流式响应体处理时无前置 context（覆盖 line 216-225, 227-234）：
		// 使用 headerLimitConfig（仅 x-api-key 规则），ensureContextInitialized 自动触发的
		// onHttpRequestHeaders 因为缺少 x-api-key 头而 matched 为空，返回 ActionContinue，
		// 此时 token context / LimitRedisContext 均未被设置。后续 streaming body 应通过
		// GetContext 判空分支安全返回 data，不 panic、不调用 Redis。
		t.Run("streaming body without prior context", func(t *testing.T) {
			host, status := test.NewTestHost(headerLimitConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 直接处理流式响应体（缺少 x-api-key，matched 为空，SetContext 未调用）
			body := []byte(`{"choices":[{"message":{"content":"hi"}}],"usage":{"prompt_tokens":10,"completion_tokens":15,"total_tokens":25}}`)
			streamAction := host.CallOnHttpStreamingResponseBody(body, true)

			// 应安全返回 ActionContinue
			require.Equal(t, types.ActionContinue, streamAction)

			// 不应触发任何 Redis callout（前置 context 缺失时跳过响应阶段）
			require.Equal(t, 0, len(host.GetRedisCalloutAttributes()),
				"missing token context should skip response-phase INCRBY")

			host.CompleteHttp()
		})
	})
}

func TestCompleteFlow(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试完整的限流流程
		t.Run("complete rate limit flow", func(t *testing.T) {
			host, status := test.NewTestHost(headerLimitConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 1. 处理请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "POST"},
				{"x-api-key", "test-key-123"},
			})

			// 由于需要调用 Redis，应该返回 HeaderStopAllIterationAndWatermark
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			// 2. 模拟 Redis 调用响应
			resp := multiRuleResp([3]int{100, 1, 60})
			host.CallOnRedisCall(0, resp)

			// 3. 处理流式响应体
			responseBody := []byte(`{"choices":[{"message":{"content":"AI response"}}],"usage":{"prompt_tokens":5,"completion_tokens":8,"total_tokens":13}}`)
			action = host.CallOnHttpStreamingRequestBody(responseBody, true)

			result := host.GetRequestBody()
			require.Equal(t, responseBody, result)

			// 应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			// 4. 完成请求
			host.CompleteHttp()
		})
	})
}

// multiRuleResp 构建多规则嵌套 Redis 响应（RESP wire format）。
// test.CreateRedisRespArray 不支持嵌套数组，因此直接用 resp.Writer 构造。
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
