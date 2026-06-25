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
	"net/http"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

// 测试配置：基础配置
var basicConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"admin_consumer":   "admin",
		"redis_key_prefix": "chat_quota:",
		"admin_path":       "/quota",
		"enable_path_suffixes": []string{
			"/v1/chat/completions",
			"/v1/messages",
		},
		"redis": map[string]interface{}{
			"service_name": "redis.static",
			"service_port": 6379,
			"timeout":      1000,
			"database":     0,
		},
	})
	return data
}()

// 测试配置：缺少admin_consumer
var missingAdminConsumerConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"redis": map[string]interface{}{
			"service_name": "redis.static",
			"service_port": 6379,
		},
	})
	return data
}()

var defaultPathSuffixesConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"admin_consumer": "admin",
		"redis": map[string]interface{}{
			"service_name": "redis.static",
			"service_port": 6379,
		},
	})
	return data
}()

func TestParseConfig(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// 测试基础配置解析
		t.Run("basic config", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)
			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			quotaConfig := config.(*QuotaConfig)
			require.Equal(t, "admin", quotaConfig.AdminConsumer)
			require.Equal(t, "chat_quota:", quotaConfig.RedisKeyPrefix)
			require.Equal(t, "/quota", quotaConfig.AdminPath)
			require.Equal(t, []string{"/v1/chat/completions", "/v1/messages"}, quotaConfig.EnablePathSuffixes)
		})

		// 测试缺少admin_consumer的配置
		t.Run("missing admin_consumer", func(t *testing.T) {
			host, status := test.NewTestHost(missingAdminConsumerConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})

		t.Run("default path suffixes", func(t *testing.T) {
			host, status := test.NewTestHost(defaultPathSuffixesConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)
			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			quotaConfig := config.(*QuotaConfig)
			require.Equal(t, []string{"/v1/chat/completions", "/v1/messages"}, quotaConfig.EnablePathSuffixes)
		})
	})
}

func TestOnHttpRequestHeaders(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试聊天完成模式的请求头处理
		t.Run("chat completion mode", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，包含consumer信息
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"x-mse-consumer", "consumer1"},
			})

			// 由于需要调用Redis检查配额，应该返回HeaderStopAllIterationAndWatermark
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			// 模拟Redis调用响应（有足够配额）
			resp := test.CreateRedisResp(1000)
			host.CallOnRedisCall(0, resp)
			action = host.GetHttpStreamAction()
			require.Equal(t, types.ActionContinue, action)
			host.CompleteHttp()
		})

		// 测试聊天完成模式配额耗尽（legacy Phase 1 路径，group == ""）
		// 锁定 403 / ai-quota.noquota：body 含 "consumer quota exhausted" 细节
		t.Run("chat completion mode denied", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 不带 x-mse-consumer-group header，走 legacy 路径
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"x-mse-consumer", "consumer1"},
			})

			// legacy 路径同样 HeaderStopAllIterationAndWatermark
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			// 模拟 Redis 返回 0，触发 response.Integer() <= 0 拒绝分支
			resp := test.CreateRedisResp(0)
			host.CallOnRedisCall(0, resp)

			response := host.GetLocalResponse()
			require.NotNil(t, response)
			require.Equal(t, uint32(http.StatusForbidden), response.StatusCode)
			require.Contains(t, string(response.Data), "ai-quota.noquota")
			require.Contains(t, string(response.Data), "consumer quota exhausted")
			host.CompleteHttp()
		})

		// 测试管理员查询模式的请求头处理
		t.Run("admin query mode", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，包含admin consumer信息
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions/quota?consumer=consumer1"},
				{":method", "GET"},
				{"x-mse-consumer", "admin"},
			})

			// 管理员查询模式应该返回 ActionPause
			require.Equal(t, types.ActionPause, action)

			// 模拟Redis调用响应
			resp := test.CreateRedisResp(500)
			host.CallOnRedisCall(0, resp)

			response := host.GetLocalResponse()
			require.Equal(t, uint32(http.StatusOK), response.StatusCode)
			require.Equal(t, "{\"consumer\":\"consumer1\",\"quota\":500}", string(response.Data))
			host.CompleteHttp()
		})

		// 测试无consumer的情况
		t.Run("no consumer", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，不包含consumer信息
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			// 无consumer应该返回ActionContinue
			require.Equal(t, types.ActionContinue, action)
		})
	})
}

func TestOnHttpRequestBody(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试管理员刷新模式的请求体处理
		t.Run("admin refresh mode", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions/quota/refresh"},
				{":method", "POST"},
				{"x-mse-consumer", "admin"},
			})

			// 设置请求体
			body := "consumer=consumer1&quota=1000"
			action := host.CallOnHttpRequestBody([]byte(body))

			// 管理员刷新模式应该返回ActionPause
			require.Equal(t, types.ActionPause, action)

			// 模拟Redis调用响应
			resp := test.CreateRedisRespArray([]interface{}{"OK"})
			host.CallOnRedisCall(0, resp)

			response := host.GetLocalResponse()
			require.Equal(t, uint32(http.StatusOK), response.StatusCode)
			require.Equal(t, "refresh quota successful", string(response.Data))
			host.CompleteHttp()
		})

		// 测试聊天完成模式的请求体处理
		t.Run("chat completion mode", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"x-mse-consumer", "consumer1"},
			})

			// 设置请求体
			body := `{"model": "gpt-3.5-turbo", "messages": [{"role": "user", "content": "Hello"}]}`
			action := host.CallOnHttpRequestBody([]byte(body))

			// 聊天完成模式应该返回ActionContinue
			require.Equal(t, types.ActionContinue, action)
		})
	})
}

func TestOnHttpStreamingResponseBody(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试聊天完成模式的流式响应体处理
		t.Run("chat completion mode", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"x-mse-consumer", "consumer1"},
			})

			// 测试流式响应体处理
			data := []byte(`{"choices": [{"delta": {"content": "Hello"}}]}`)
			action := host.CallOnHttpStreamingResponseBody(data, false)

			require.Equal(t, types.ActionContinue, action)
			result := host.GetResponseBody()
			// 非结束流应该返回原始数据
			require.Equal(t, data, result)

			// 测试结束流
			action = host.CallOnHttpStreamingResponseBody(data, true)

			require.Equal(t, types.ActionContinue, action)
			result = host.GetResponseBody()
			// 结束流应该返回原始数据
			require.Equal(t, data, result)

			// 模拟Redis调用响应（减少配额）
			resp := test.CreateRedisRespArray([]interface{}{30})
			host.CallOnRedisCall(0, resp)

			host.CompleteHttp()
		})

		// 测试非聊天完成模式的流式响应体处理
		t.Run("non-chat completion mode", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/other/path"},
				{":method", "GET"},
				{"x-mse-consumer", "consumer1"},
			})

			// 测试流式响应体处理
			data := []byte("response data")
			action := host.CallOnHttpStreamingResponseBody(data, false)

			// 非聊天完成模式应该返回原始数据
			require.Equal(t, types.ActionContinue, action)
			result := host.GetResponseBody()
			require.Equal(t, data, result)
		})
	})
}

func TestGetOperationMode(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		adminPath string
		suffixes  []string
		chatMode  ChatMode
		adminMode AdminMode
	}{
		{
			name:      "chat completion mode",
			path:      "/v1/chat/completions",
			adminPath: "/quota",
			suffixes:  []string{"/v1/chat/completions", "/v1/messages"},
			chatMode:  ChatModeCompletion,
			adminMode: AdminModeNone,
		},
		{
			name:      "admin query mode",
			path:      "/v1/chat/completions/quota",
			adminPath: "/quota",
			suffixes:  []string{"/v1/chat/completions", "/v1/messages"},
			chatMode:  ChatModeAdmin,
			adminMode: AdminModeQuery,
		},
		{
			name:      "admin refresh mode",
			path:      "/v1/chat/completions/quota/refresh",
			adminPath: "/quota",
			suffixes:  []string{"/v1/chat/completions", "/v1/messages"},
			chatMode:  ChatModeAdmin,
			adminMode: AdminModeRefresh,
		},
		{
			name:      "admin delta mode",
			path:      "/v1/chat/completions/quota/delta",
			adminPath: "/quota",
			suffixes:  []string{"/v1/chat/completions", "/v1/messages"},
			chatMode:  ChatModeAdmin,
			adminMode: AdminModeDelta,
		},
		{
			name:      "anthropic messages completion mode",
			path:      "/v1/messages",
			adminPath: "/quota",
			suffixes:  []string{"/v1/chat/completions", "/v1/messages"},
			chatMode:  ChatModeCompletion,
			adminMode: AdminModeNone,
		},
		{
			name:      "custom suffix completion mode",
			path:      "/llm/invoke",
			adminPath: "/quota",
			suffixes:  []string{"/invoke"},
			chatMode:  ChatModeCompletion,
			adminMode: AdminModeNone,
		},
		{
			name:      "admin path fixed to chat completions",
			path:      "/v1/chat/completions/quota",
			adminPath: "/quota",
			suffixes:  []string{"/invoke"},
			chatMode:  ChatModeAdmin,
			adminMode: AdminModeQuery,
		},
		{
			name:      "messages admin path not supported",
			path:      "/v1/messages/quota",
			adminPath: "/quota",
			suffixes:  []string{"/v1/chat/completions", "/v1/messages"},
			chatMode:  ChatModeNone,
			adminMode: AdminModeNone,
		},
		{
			name:      "none mode",
			path:      "/other/path",
			adminPath: "/quota",
			suffixes:  []string{"/v1/chat/completions", "/v1/messages"},
			chatMode:  ChatModeNone,
			adminMode: AdminModeNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chatMode, adminMode := getOperationMode(tt.path, tt.adminPath, tt.suffixes)
			require.Equal(t, tt.chatMode, chatMode)
			require.Equal(t, tt.adminMode, adminMode)
		})
	}
}

// 验证两个 Lua 脚本常量存在且内容非空（编译期通过即可，运行时验证在后续 Task 中做）
func TestLuaScripts_Defined(t *testing.T) {
	require.NotEmpty(t, RequestPhaseQuotaReadScript)
	require.NotEmpty(t, ResponsePhaseQuotaDecrbyScript)
	require.Contains(t, RequestPhaseQuotaReadScript, "GET")
	require.Contains(t, ResponsePhaseQuotaDecrbyScript, "DECRBY")
}

// chat completion 模式 + group 非空 + 两池都 > 0 → ActionContinue
func TestOnHttpRequestHeaders_WithGroup_BothPositive(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(basicConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/v1/chat/completions"},
			{":method", "POST"},
			{"x-mse-consumer", "alice"},
			{"x-mse-consumer-group", "team-a"},
		})
		require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

		// Lua Eval 返回 {group_remaining, consumer_remaining}
		host.CallOnRedisCall(0, test.CreateRedisRespArray([]interface{}{1000, 500}))

		action = host.GetHttpStreamAction()
		require.Equal(t, types.ActionContinue, action)
		host.CompleteHttp()
	})
}

// group 非空 + group ≤ 0 → 拒绝（ai-quota.noquota: group quota exhausted）
func TestOnHttpRequestHeaders_WithGroup_GroupExhausted(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(basicConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/v1/chat/completions"},
			{":method", "POST"},
			{"x-mse-consumer", "alice"},
			{"x-mse-consumer-group", "team-a"},
		})
		require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

		host.CallOnRedisCall(0, test.CreateRedisRespArray([]interface{}{0, 500}))

		resp := host.GetLocalResponse()
		require.NotNil(t, resp)
		require.Equal(t, uint32(http.StatusForbidden), resp.StatusCode)
		require.Contains(t, string(resp.Data), "ai-quota.noquota")
		require.Contains(t, string(resp.Data), "group quota exhausted")
		host.CompleteHttp()
	})
}

// group 非空 + consumer ≤ 0 → 拒绝（ai-quota.noquota: consumer quota exhausted）
func TestOnHttpRequestHeaders_WithGroup_ConsumerExhausted(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(basicConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/v1/chat/completions"},
			{":method", "POST"},
			{"x-mse-consumer", "alice"},
			{"x-mse-consumer-group", "team-a"},
		})
		require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

		host.CallOnRedisCall(0, test.CreateRedisRespArray([]interface{}{1000, 0}))

		resp := host.GetLocalResponse()
		require.NotNil(t, resp)
		require.Equal(t, uint32(http.StatusForbidden), resp.StatusCode)
		require.Contains(t, string(resp.Data), "ai-quota.noquota")
		require.Contains(t, string(resp.Data), "consumer quota exhausted")
		host.CompleteHttp()
	})
}

// group 非空 + 两池都 ≤ 0 → 拒绝（ai-quota.noquota: group and consumer quota exhausted）
func TestOnHttpRequestHeaders_WithGroup_BothExhausted(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(basicConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/v1/chat/completions"},
			{":method", "POST"},
			{"x-mse-consumer", "alice"},
			{"x-mse-consumer-group", "team-a"},
		})
		require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

		host.CallOnRedisCall(0, test.CreateRedisRespArray([]interface{}{0, 0}))

		resp := host.GetLocalResponse()
		require.NotNil(t, resp)
		require.Equal(t, uint32(http.StatusForbidden), resp.StatusCode)
		require.Contains(t, string(resp.Data), "ai-quota.noquota")
		require.Contains(t, string(resp.Data), "group and consumer quota exhausted")
		host.CompleteHttp()
	})
}

// Phase 1 legacy Get 路径下 Redis 返回 nil（key 不存在）→ 403 ai-quota.noquota
// 锁定 main.go:235-237 的 IsNull() 分支，与 Integer() <= 0 分支独立。
// 业务语义：从未 refresh 过该 consumer 配额 → 视为无配额拒绝。
func TestOnHttpRequestHeaders_LegacyPhase1_NullKey_Denied(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(basicConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		// 不带 x-mse-consumer-group → legacy 单池路径
		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/v1/chat/completions"},
			{":method", "POST"},
			{"x-mse-consumer", "alice"},
		})
		require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

		// RESP nil reply：模拟 key 不存在
		host.CallOnRedisCall(0, test.CreateRedisRespNull())

		resp := host.GetLocalResponse()
		require.NotNil(t, resp)
		require.Equal(t, uint32(http.StatusForbidden), resp.StatusCode)
		require.Contains(t, string(resp.Data), "ai-quota.noquota")
		require.Contains(t, string(resp.Data), "consumer quota exhausted")
		host.CompleteHttp()
	})
}

// Phase 1 Lua Eval 路径下 Redis 返回错误 → 503 ai-quota.error
// 锁定 main.go:254-257 的 response.Error() 分支。
// 区分此路径（503）与 legacy 路径（429）的语义：Redis 故障不应被静默归类为配额拒绝。
func TestOnHttpRequestHeaders_LuaPhase1_RedisError_503(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(basicConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/v1/chat/completions"},
			{":method", "POST"},
			{"x-mse-consumer", "alice"},
			{"x-mse-consumer-group", "team-a"},
		})
		require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

		// RESP error reply：模拟 Redis 报错（如 network blip、AUTH 失败）
		host.CallOnRedisCall(0, test.CreateRedisRespError("connection refused"))

		resp := host.GetLocalResponse()
		require.NotNil(t, resp)
		require.Equal(t, uint32(http.StatusServiceUnavailable), resp.StatusCode)
		// 503 路径的 body 仅含 "redis error:%v"，错误码通过 SendResponse 的 statusCodeDetails
		// 字段（"ai-quota.error"）传给 Envoy，不在 response.Data 中。
		require.Contains(t, string(resp.Data), "redis error:")
		host.CompleteHttp()
	})
}

// 阶段 2 group 非空 → Eval 调用 group + consumer 双池 DECRBY
//
// 每个 CallOnRedisCall 的角色在内部注释说明，便于以后排查：
//   - 第 1 次 (idx=0)：模拟 Phase 1 Lua MGET 的响应 {group_remaining, consumer_remaining}
//   - 第 2 次 (idx=0)：模拟 Phase 2 Lua DECRBY 的响应 {group_remaining, consumer_remaining}
//
// Wasm filter 不暴露 Redis 调用参数（key 列表 / cost），框架也没有 RedisCallCount inspector。
// 为了让"实际走的是 Lua 双池路径而不是 legacy 单池路径"在重构时可见，单独有 sister 测试
// TestOnHttpStreamingResponseBody_LegacyDecrby_SinglePool 覆盖 group == "" 路径——
// 若有人把 Lua 分支误替换成 legacy 分支，本测试的 mock 值（数组）会让 DecrBy 解析失败或状态错位。
func TestOnHttpStreamingResponseBody_WithGroup_DecrbyBothPools(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(basicConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/v1/chat/completions"},
			{":method", "POST"},
			{"x-mse-consumer", "alice"},
			{"x-mse-consumer-group", "team-a"},
		})

		// Phase 1 Lua 响应：{group_rem=1000, consumer_rem=500} → 两池都 > 0，ResumeHttpRequest
		host.CallOnRedisCall(0, test.CreateRedisRespArray([]interface{}{1000, 500}))

		// 流式 chunk 携带 usage，触发 tokenusage 提取（必需，否则 onHttpStreamingResponseBody 早返回）
		data := []byte(`{"choices": [{"delta": {"content": "Hello"}}], "usage": {"prompt_tokens": 15, "completion_tokens": 15, "total_tokens": 30}}`)
		// 非结束流直接返回 data，不触发任何 Redis 操作
		result := host.CallOnHttpStreamingResponseBody(data, false)
		require.Equal(t, types.ActionContinue, result)

		// 结束流 → Phase 2 Lua DECRBY 双池
		result = host.CallOnHttpStreamingResponseBody(data, true)
		require.Equal(t, types.ActionContinue, result)

		// Phase 2 响应：{group_rem=970, consumer_rem=470}（cost=30 扣减后）
		// 回调是 nil，仅用于满足框架调度，本地无 response 可断言。
		host.CallOnRedisCall(0, test.CreateRedisRespArray([]interface{}{970, 470}))

		// post-state 断言：Wasm filter 必须原样透传 stream body（main.go:357-358 return data）
		require.Equal(t, data, host.GetResponseBody())
		host.CompleteHttp()
	})
}

// Sister 测试：Phase 2 legacy 路径（group == ""，无 x-mse-consumer-group header）→ 单 DecrBy。
// 与 WithGroup 版对照：若有人把 Lua 路径误换成 legacy 路径，WithGroup 测试的数组响应会让
// legacy DecrBy（只接受 integer reply）解析行为差异可见，反之亦然。
func TestOnHttpStreamingResponseBody_LegacyDecrby_SinglePool(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(basicConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		// 不带 x-mse-consumer-group → legacy 路径
		host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/v1/chat/completions"},
			{":method", "POST"},
			{"x-mse-consumer", "alice"},
		})

		// Phase 1 legacy Get 返回 integer reply
		host.CallOnRedisCall(0, test.CreateRedisResp(1000))

		data := []byte(`{"choices": [{"delta": {"content": "Hello"}}], "usage": {"prompt_tokens": 15, "completion_tokens": 15, "total_tokens": 30}}`)
		result := host.CallOnHttpStreamingResponseBody(data, false)
		require.Equal(t, types.ActionContinue, result)

		result = host.CallOnHttpStreamingResponseBody(data, true)
		require.Equal(t, types.ActionContinue, result)

		// Phase 2 legacy DecrBy 响应：单 integer（扣减后剩余），回调为 nil
		host.CallOnRedisCall(0, test.CreateRedisResp(970))

		// post-state 断言：data 透传
		require.Equal(t, data, host.GetResponseBody())
		host.CompleteHttp()
	})
}

// admin refresh with group=<name> → 写入 chat_quota:<group>
func TestAdminRefresh_Group(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(basicConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/v1/chat/completions/quota/refresh"},
			{":method", "POST"},
			{"x-mse-consumer", "admin"},
		})
		body := "group=team-a&quota=10000"
		action := host.CallOnHttpRequestBody([]byte(body))
		require.Equal(t, types.ActionPause, action)

		resp := test.CreateRedisRespArray([]interface{}{"OK"})
		host.CallOnRedisCall(0, resp)

		response := host.GetLocalResponse()
		require.Equal(t, uint32(http.StatusOK), response.StatusCode)
		require.Equal(t, "refresh quota successful", string(response.Data))
		host.CompleteHttp()
	})
}

// admin refresh 同时设置 consumer + group → 403 ai-quota.unauthorized
func TestAdminRefresh_BothConsumerAndGroup_Rejected(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(basicConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/v1/chat/completions/quota/refresh"},
			{":method", "POST"},
			{"x-mse-consumer", "admin"},
		})
		body := "consumer=alice&group=team-a&quota=100"
		host.CallOnHttpRequestBody([]byte(body))

		resp := host.GetLocalResponse()
		require.NotNil(t, resp)
		require.Equal(t, uint32(http.StatusForbidden), resp.StatusCode)
		require.Contains(t, string(resp.Data), "ai-quota.unauthorized")
		require.Contains(t, string(resp.Data), "consumer or group must be set")
		host.CompleteHttp()
	})
}

// admin query with group → GET chat_quota:<group>，响应 JSON 中 name=group 名
func TestAdminQuery_Group(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(basicConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/v1/chat/completions/quota?group=team-a"},
			{":method", "GET"},
			{"x-mse-consumer", "admin"},
		})
		require.Equal(t, types.ActionPause, action)

		// queryQuota 走 redisClient.Get，回调里调用 response.Integer() 解析单值。
		// 必须用 CreateRedisResp(int) 产生 RESP integer reply；
		// 若用 CreateRedisRespArray 会得到 RESP array，Integer() 返回 0 → 断言失败。
		host.CallOnRedisCall(0, test.CreateRedisResp(2000))

		response := host.GetLocalResponse()
		require.Equal(t, uint32(http.StatusOK), response.StatusCode)
		require.JSONEq(t, `{"group":"team-a","quota":2000}`, string(response.Data))
		host.CompleteHttp()
	})
}

// admin query with consumer → 响应含 consumer 字段（不出现 name 字段）。
// 与老 main 分支字节级一致 —— consumer 查询响应字段名保持 `consumer`，老 client 零迁移。
func TestAdminQuery_Consumer(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(basicConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/v1/chat/completions/quota?consumer=consumer1"},
			{":method", "GET"},
			{"x-mse-consumer", "admin"},
		})
		require.Equal(t, types.ActionPause, action)

		host.CallOnRedisCall(0, test.CreateRedisResp(500))

		response := host.GetLocalResponse()
		require.Equal(t, uint32(http.StatusOK), response.StatusCode)
		require.JSONEq(t, `{"consumer":"consumer1","quota":500}`, string(response.Data))
		// 防御性：响应里不出现 name 字段（spec §5.4.2 决策 —— 不创造 targetName 统一抽象）
		require.NotContains(t, string(response.Data), `"name"`)
		host.CompleteHttp()
	})
}

// admin delta with group → INCRBY/DECRBY chat_quota:<group>
func TestAdminDelta_Group(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(basicConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/v1/chat/completions/quota/delta"},
			{":method", "POST"},
			{"x-mse-consumer", "admin"},
		})
		body := "group=team-a&value=500"
		host.CallOnHttpRequestBody([]byte(body))

		// deltaQuota 走 IncrBy/DecrBy，回调只检查 .Error() 不解析值，
		// 用 CreateRedisResp(int) 产生标准 RESP integer reply 即可（与 Get 一致）。
		resp := test.CreateRedisResp(2500)
		host.CallOnRedisCall(0, resp)

		response := host.GetLocalResponse()
		require.Equal(t, uint32(http.StatusOK), response.StatusCode)
		require.Equal(t, "delta quota successful", string(response.Data))
		host.CompleteHttp()
	})
}

// 互斥校验：consumer 与 group 都未设置 → 403 ai-quota.unauthorized（refresh）
// 锁定 (a == "" && b == "") 分支，避免只测 "都设置" 时的偏覆盖。
func TestAdminRefresh_NeitherSet_Rejected(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(basicConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/v1/chat/completions/quota/refresh"},
			{":method", "POST"},
			{"x-mse-consumer", "admin"},
		})
		// 仅 quota，不带 consumer 与 group → 互斥校验应拦截
		host.CallOnHttpRequestBody([]byte("quota=1000"))

		resp := host.GetLocalResponse()
		require.NotNil(t, resp)
		require.Equal(t, uint32(http.StatusForbidden), resp.StatusCode)
		require.Contains(t, string(resp.Data), "ai-quota.unauthorized")
		require.Contains(t, string(resp.Data), "consumer or group must be set")
		host.CompleteHttp()
	})
}

// 互斥校验：consumer 与 group 都未设置 → 403 unauthorized（query，URL 参数版）
func TestAdminQuery_NeitherSet_Rejected(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(basicConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		// 不带 ?consumer= 也不带 ?group= → query 入口互斥校验拦截
		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/v1/chat/completions/quota"},
			{":method", "GET"},
			{"x-mse-consumer", "admin"},
		})
		// 校验失败走 ActionContinue（util.SendResponse 后立即返回）
		require.Equal(t, types.ActionContinue, action)

		resp := host.GetLocalResponse()
		require.NotNil(t, resp)
		require.Equal(t, uint32(http.StatusForbidden), resp.StatusCode)
		require.Contains(t, string(resp.Data), "ai-quota.unauthorized")
		require.Contains(t, string(resp.Data), "consumer or group must be set")
		host.CompleteHttp()
	})
}

// 互斥校验：consumer 与 group 都未设置 → 403 unauthorized（delta）
func TestAdminDelta_NeitherSet_Rejected(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(basicConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/v1/chat/completions/quota/delta"},
			{":method", "POST"},
			{"x-mse-consumer", "admin"},
		})
		// 仅 value，不带 consumer 与 group → 互斥校验应拦截
		host.CallOnHttpRequestBody([]byte("value=100"))

		resp := host.GetLocalResponse()
		require.NotNil(t, resp)
		require.Equal(t, uint32(http.StatusForbidden), resp.StatusCode)
		require.Contains(t, string(resp.Data), "ai-quota.unauthorized")
		require.Contains(t, string(resp.Data), "consumer or group must be set")
		host.CompleteHttp()
	})
}
