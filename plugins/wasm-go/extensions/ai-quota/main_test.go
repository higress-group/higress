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
	"errors"
	"net/http"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/higress-group/wasm-go/pkg/wrapper"
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

type redisDecrement struct {
	key   string
	delta int
}

type recordingRedisClient struct {
	wrapper.RedisClient
	decrements []redisDecrement
	err        error
}

func (c *recordingRedisClient) DecrBy(key string, delta int, _ wrapper.RedisResponseCallback) error {
	c.decrements = append(c.decrements, redisDecrement{key: key, delta: delta})
	return c.err
}

type recordingHTTPContext struct {
	wrapper.HttpContext
	context        map[string]interface{}
	userAttributes map[string]interface{}
	bufferedBody   bool
	skippedBody    bool
}

func newRecordingHTTPContext(chatMode ChatMode, consumer string) *recordingHTTPContext {
	ctx := &recordingHTTPContext{
		context:        map[string]interface{}{"chatMode": chatMode},
		userAttributes: map[string]interface{}{},
	}
	if consumer != "" {
		ctx.context["consumer"] = consumer
	}
	return ctx
}

func (c *recordingHTTPContext) SetContext(key string, value interface{}) {
	c.context[key] = value
}

func (c *recordingHTTPContext) GetContext(key string) interface{} {
	return c.context[key]
}

func (c *recordingHTTPContext) SetUserAttribute(key string, value interface{}) {
	c.userAttributes[key] = value
}

func (c *recordingHTTPContext) GetUserAttribute(key string) interface{} {
	return c.userAttributes[key]
}

func (c *recordingHTTPContext) GetBoolContext(key string, defaultValue bool) bool {
	value, ok := c.context[key].(bool)
	if !ok {
		return defaultValue
	}
	return value
}

func (c *recordingHTTPContext) GetStringContext(key, defaultValue string) string {
	value, ok := c.context[key].(string)
	if !ok {
		return defaultValue
	}
	return value
}

func (c *recordingHTTPContext) GetByteSliceContext(key string, defaultValue []byte) []byte {
	value, ok := c.context[key].([]byte)
	if !ok {
		return defaultValue
	}
	return value
}

func (c *recordingHTTPContext) BufferResponseBody() {
	c.bufferedBody = true
}

func (c *recordingHTTPContext) DontReadResponseBody() {
	c.skippedBody = true
}

func allowCompletionRequest(t *testing.T, host test.TestHost) {
	t.Helper()

	action := host.CallOnHttpRequestHeaders([][2]string{
		{":authority", "example.com"},
		{":path", "/v1/chat/completions"},
		{":method", "POST"},
		{"x-mse-consumer", "consumer1"},
	})
	require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

	host.CallOnRedisCall(0, test.CreateRedisResp(1000))
	require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())
}

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

func TestOnHttpResponseBody(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("chunked non-streaming response is buffered before decrement", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			allowCompletionRequest(t, host)
			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})
			require.Equal(t, types.ActionContinue, action)

			firstChunk := []byte(`{"model":"gpt-4","choices":[],"usage":{"prompt_tokens":10,`)
			lastChunk := []byte(`"completion_tokens":15,"total_tokens":25}}`)
			action = host.CallOnHttpStreamingResponseBody(firstChunk, false)
			require.Equal(t, types.ActionPause, action)
			action = host.CallOnHttpStreamingResponseBody(lastChunk, true)
			require.Equal(t, types.ActionContinue, action)
			expectedBody := append(append([]byte{}, firstChunk...), lastChunk...)
			require.Equal(t, expectedBody, host.GetResponseBody())

			// Consume the pending response from the quota-decrement Redis callout.
			host.CallOnRedisCall(0, test.CreateRedisResp(975))

			host.CompleteHttp()
		})
	})
}

func TestOnHttpStreamingResponseBody(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("streaming response decrements only after end of stream", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			allowCompletionRequest(t, host)
			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "Text/Event-Stream; charset=utf-8"},
			})
			require.Equal(t, types.ActionContinue, action)

			contentChunk := []byte("data: {\"model\":\"gpt-4\",\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n")
			usageChunk := []byte("data: {\"model\":\"gpt-4\",\"choices\":[],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":15,\"total_tokens\":25}}\n\n")
			doneChunk := []byte("data: [DONE]\n\n")

			action = host.CallOnHttpStreamingResponseBody(contentChunk, false)
			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, contentChunk, host.GetResponseBody())
			action = host.CallOnHttpStreamingResponseBody(usageChunk, false)
			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, usageChunk, host.GetResponseBody())
			action = host.CallOnHttpStreamingResponseBody(doneChunk, true)
			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, doneChunk, host.GetResponseBody())

			host.CallOnRedisCall(0, test.CreateRedisResp(975))
			host.CompleteHttp()
		})

		t.Run("non-completion response body remains unchanged", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/other/path"},
				{":method", "GET"},
				{"x-mse-consumer", "consumer1"},
			})
			require.Equal(t, types.ActionContinue, action)
			action = host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})
			require.Equal(t, types.ActionContinue, action)

			data := []byte("response data")
			action = host.CallOnHttpStreamingResponseBody(data, false)
			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, data, host.GetResponseBody())
			host.CompleteHttp()
		})
	})
}

func TestProcessResponseUsage(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(basicConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		body := []byte(`{"model":"gpt-4","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":15,"total_tokens":25}}`)

		t.Run("dispatches one decrement for repeated callbacks", func(t *testing.T) {
			redisClient := &recordingRedisClient{}
			ctx := newRecordingHTTPContext(ChatModeCompletion, "consumer1")
			config := QuotaConfig{RedisKeyPrefix: "chat_quota:", redisClient: redisClient}

			processResponseUsage(ctx, config, body, true)
			processResponseUsage(ctx, config, body, true)

			require.Equal(t, []redisDecrement{{key: "chat_quota:consumer1", delta: 25}}, redisClient.decrements)
			require.Equal(t, true, ctx.GetContext(ctxKeyQuotaDeducted))
		})

		t.Run("streaming usage waits for end of stream", func(t *testing.T) {
			redisClient := &recordingRedisClient{}
			ctx := newRecordingHTTPContext(ChatModeCompletion, "consumer1")
			config := QuotaConfig{RedisKeyPrefix: "chat_quota:", redisClient: redisClient}

			processResponseUsage(ctx, config, body, false)
			require.Empty(t, redisClient.decrements)
			processResponseUsage(ctx, config, []byte("data: [DONE]\n\n"), true)

			require.Equal(t, []redisDecrement{{key: "chat_quota:consumer1", delta: 25}}, redisClient.decrements)
		})

		t.Run("synchronous dispatch failure can retry", func(t *testing.T) {
			redisClient := &recordingRedisClient{err: errors.New("dispatch failed")}
			ctx := newRecordingHTTPContext(ChatModeCompletion, "consumer1")
			config := QuotaConfig{RedisKeyPrefix: "chat_quota:", redisClient: redisClient}

			processResponseUsage(ctx, config, body, true)
			require.Equal(t, []redisDecrement{{key: "chat_quota:consumer1", delta: 25}}, redisClient.decrements)
			require.Equal(t, false, ctx.GetContext(ctxKeyQuotaDeducted))

			redisClient.err = nil
			processResponseUsage(ctx, config, body, true)
			require.Equal(t, []redisDecrement{
				{key: "chat_quota:consumer1", delta: 25},
				{key: "chat_quota:consumer1", delta: 25},
			}, redisClient.decrements)
			require.Equal(t, true, ctx.GetContext(ctxKeyQuotaDeducted))
		})

		t.Run("does not decrement without usage", func(t *testing.T) {
			redisClient := &recordingRedisClient{}
			ctx := newRecordingHTTPContext(ChatModeCompletion, "consumer1")
			config := QuotaConfig{RedisKeyPrefix: "chat_quota:", redisClient: redisClient}

			processResponseUsage(ctx, config, []byte(`{"choices":[]}`), true)

			require.Empty(t, redisClient.decrements)
			require.Nil(t, ctx.GetContext(ctxKeyQuotaDeducted))
		})

		t.Run("does not decrement without consumer", func(t *testing.T) {
			redisClient := &recordingRedisClient{}
			ctx := newRecordingHTTPContext(ChatModeCompletion, "")
			config := QuotaConfig{RedisKeyPrefix: "chat_quota:", redisClient: redisClient}

			processResponseUsage(ctx, config, body, true)

			require.Empty(t, redisClient.decrements)
			require.Nil(t, ctx.GetContext(ctxKeyQuotaDeducted))
		})
	})
}

func TestOnHttpResponseHeadersSkipsNonCompletionBody(t *testing.T) {
	ctx := newRecordingHTTPContext(ChatModeNone, "consumer1")

	action := onHttpResponseHeaders(ctx, QuotaConfig{})

	require.Equal(t, types.ActionContinue, action)
	require.True(t, ctx.skippedBody)
	require.False(t, ctx.bufferedBody)
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
