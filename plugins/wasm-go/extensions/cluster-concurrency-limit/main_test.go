// Copyright (c) 2026 Alibaba Group Holding Ltd.
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

var defaultRedis = map[string]interface{}{
	"service_name": "redis.static",
	"service_port": 6379,
	"timeout":      1000,
}

// 测试配置:仅全局兜底
var globalOnlyConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"rule_name":        "global-only",
		"global_threshold": map[string]interface{}{"concurrency": 100},
		"redis":            defaultRedis,
	})
	return data
}()

// 测试配置:三级层级限额(global + 租户 + 用户 + apikey),all-match 并存
var threeLevelConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"rule_name":        "three-level",
		"global_threshold": map[string]interface{}{"concurrency": 200},
		"rule_items": []map[string]interface{}{
			{
				"limit_by_consumer_group": "",
				"limit_keys": []map[string]interface{}{
					{"key": "tenant-a", "concurrency": 10},
					{"key": "*", "concurrency": 5},
				},
			},
			{
				"limit_by_consumer": "",
				"limit_keys": []map[string]interface{}{
					{"key": "alice", "concurrency": 7},
					{"key": "regexp:.*", "concurrency": 3},
				},
			},
			{
				"limit_by_header": "x-api-key",
				"limit_keys": []map[string]interface{}{
					{"key": "sk-aaa111", "concurrency": 2},
					{"key": "*", "concurrency": 1},
				},
			},
		},
		"redis":                   defaultRedis,
		"show_limit_quota_header": true,
	})
	return data
}()

// 测试配置:自定义拒绝码与消息
var customRejectConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"rule_name":        "custom-reject",
		"global_threshold": map[string]interface{}{"concurrency": 1},
		"redis":            defaultRedis,
		"rejected_code":    503,
		"rejected_msg":     "server busy",
	})
	return data
}()

// 测试配置:per_ip CIDR 匹配
var perIpConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"rule_name": "per-ip",
		"rule_items": []map[string]interface{}{
			{
				"limit_by_per_ip": "from-header-x-real-ip",
				"limit_keys": []map[string]interface{}{
					{"key": "192.168.0.0/16", "concurrency": 3},
				},
			},
		},
		"redis": defaultRedis,
	})
	return data
}()

// 测试配置:严格名单(无 "*" 兜底),用于验证"未配置级跳过、已配置级生效"
var whitelistConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"rule_name": "whitelist",
		"rule_items": []map[string]interface{}{
			{
				"limit_by_consumer_group": "",
				"limit_keys": []map[string]interface{}{
					{"key": "tenant-a", "concurrency": 10},
				},
			},
			{
				"limit_by_consumer": "",
				"limit_keys": []map[string]interface{}{
					{"key": "alice", "concurrency": 7},
				},
			},
			{
				"limit_by_header": "x-api-key",
				"limit_keys": []map[string]interface{}{
					{"key": "sk-aaa111", "concurrency": 2},
				},
			},
		},
		"redis": defaultRedis,
	})
	return data
}()

func TestParseConfig(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		t.Run("global only config", func(t *testing.T) {
			host, status := test.NewTestHost(globalOnlyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)
			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		t.Run("three level config", func(t *testing.T) {
			host, status := test.NewTestHost(threeLevelConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)
			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		t.Run("per ip config", func(t *testing.T) {
			host, status := test.NewTestHost(perIpConfig)
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
		// 有命中维度:暂停迭代等待 Redis 结果
		t.Run("global threshold hit", func(t *testing.T) {
			host, status := test.NewTestHost(globalOnlyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "POST"},
			})
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			// 模拟 Redis 放行响应:{0, cur_1},cur 为占槽后计数
			resp := test.CreateRedisRespArray([]interface{}{0, 1})
			host.CallOnRedisCall(0, resp)

			host.CompleteHttp()
		})

		// 三级层级全量命中:global + consumer_group + consumer + header 共 4 个维度
		t.Run("three level all-match", func(t *testing.T) {
			host, status := test.NewTestHost(threeLevelConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"x-mse-consumer-group", "tenant-a"},
				{"x-mse-consumer", "alice"},
				{"x-api-key", "sk-aaa111"},
			})
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			// 4 个维度全部占槽成功
			resp := test.CreateRedisRespArray([]interface{}{0, 10, 3, 2, 1})
			host.CallOnRedisCall(0, resp)

			host.CompleteHttp()
		})

		// 方案A(配了才限):身份头缺失,该级跳过;仅 global 生效,仍计数
		t.Run("no identity header skips rule dimensions", func(t *testing.T) {
			host, status := test.NewTestHost(threeLevelConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				// 无 x-mse-consumer-group / x-mse-consumer / x-api-key
			})
			// 仅 global 命中,仍需 Redis 调用
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			resp := test.CreateRedisRespArray([]interface{}{0, 1})
			host.CallOnRedisCall(0, resp)

			host.CompleteHttp()
		})

		// 方案A:apikey 未配置 → 该级跳过不限,已配置的租户/用户级照常计数(严格名单,无 "*")
		t.Run("unconfigured apikey skips, configured levels apply", func(t *testing.T) {
			host, status := test.NewTestHost(whitelistConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"x-mse-consumer-group", "tenant-a"},
				{"x-mse-consumer", "alice"},
				{"x-api-key", "sk-unknown"}, // 未配置的 apikey,该级跳过
			})
			// 命中租户+用户 2 个维度
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			resp := test.CreateRedisRespArray([]interface{}{0, 1, 1})
			host.CallOnRedisCall(0, resp)

			host.CompleteHttp()
		})

		// 方案A 边界:租户未配置跳过,但用户已配置 → 用户级仍然生效(各级独立)
		t.Run("unconfigured tenant skips, configured user applies", func(t *testing.T) {
			host, status := test.NewTestHost(whitelistConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"x-mse-consumer-group", "tenant-x"}, // 未配置的租户,该级跳过
				{"x-mse-consumer", "alice"},
				{"x-api-key", "sk-aaa111"},
			})
			// 命中用户+apikey 2 个维度
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			resp := test.CreateRedisRespArray([]interface{}{0, 1, 1})
			host.CallOnRedisCall(0, resp)

			host.CompleteHttp()
		})

		// 全部身份已配置:三级全命中
		t.Run("all levels configured", func(t *testing.T) {
			host, status := test.NewTestHost(whitelistConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"x-mse-consumer-group", "tenant-a"},
				{"x-mse-consumer", "alice"},
				{"x-api-key", "sk-aaa111"},
			})
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			resp := test.CreateRedisRespArray([]interface{}{0, 1, 1, 1})
			host.CallOnRedisCall(0, resp)

			host.CompleteHttp()
		})

		// "*" 兜底项:有身份但未列名,命中 "*" 放行并限到对应额度(threeLevelConfig 租户/ apikey 级有 "*")
		t.Run("star fallback allows unlisted identity", func(t *testing.T) {
			host, status := test.NewTestHost(threeLevelConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"x-mse-consumer-group", "tenant-other"}, // 未列名,命中 "*"
				{"x-mse-consumer", "bob"},                // 未列名,命中 "regexp:.*"
				{"x-api-key", "sk-other"},                // 未列名,命中 "*"
			})
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			resp := test.CreateRedisRespArray([]interface{}{0, 1, 1, 1, 1})
			host.CallOnRedisCall(0, resp)

			host.CompleteHttp()
		})

		// 仅 global_threshold:不做身份校验,无身份头也放行计数
		t.Run("global only allows any identity", func(t *testing.T) {
			host, status := test.NewTestHost(globalOnlyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "POST"},
			})
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			resp := test.CreateRedisRespArray([]interface{}{0, 1})
			host.CallOnRedisCall(0, resp)

			host.CompleteHttp()
		})

		// 无任何命中维度:直接放行,不发起 Redis 调用
		t.Run("no matching rule passes directly", func(t *testing.T) {
			host, status := test.NewTestHost(perIpConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "POST"},
				{"x-real-ip", "10.0.0.1"}, // 不在 192.168.0.0/16 内
			})
			require.Equal(t, types.ActionContinue, action)
		})

		// per_ip CIDR 命中
		t.Run("per ip cidr hit", func(t *testing.T) {
			host, status := test.NewTestHost(perIpConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "POST"},
				{"x-real-ip", "192.168.1.100"},
			})
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			resp := test.CreateRedisRespArray([]interface{}{0, 1})
			host.CallOnRedisCall(0, resp)

			host.CompleteHttp()
		})

		// 超限拒绝:第 2 个维度(consumer_group)超限
		t.Run("rejected by second dimension", func(t *testing.T) {
			host, status := test.NewTestHost(threeLevelConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"x-mse-consumer-group", "tenant-a"},
				{"x-mse-consumer", "alice"},
				{"x-api-key", "sk-aaa111"},
			})
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			// {reject=2, cur=10}:consumer_group 维度达到上限
			resp := test.CreateRedisRespArray([]interface{}{2, 10})
			host.CallOnRedisCall(0, resp)

			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			require.Equal(t, uint32(429), localResponse.StatusCode)
			require.Contains(t, string(localResponse.Data), "concurrency limit exceeded")

			host.CompleteHttp()
		})

		// 自定义拒绝码与消息
		t.Run("custom rejected code and msg", func(t *testing.T) {
			host, status := test.NewTestHost(customRejectConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "POST"},
			})
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			resp := test.CreateRedisRespArray([]interface{}{1, 1})
			host.CallOnRedisCall(0, resp)

			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			require.Equal(t, uint32(503), localResponse.StatusCode)
			require.Contains(t, string(localResponse.Data), "server busy")

			host.CompleteHttp()
		})
	})
}
