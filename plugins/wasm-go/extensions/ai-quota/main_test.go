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
