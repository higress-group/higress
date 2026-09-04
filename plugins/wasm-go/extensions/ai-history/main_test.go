// Copyright (c) 2024 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package main

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

// 测试配置：基本Redis配置
var basicRedisConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"redis": map[string]interface{}{
			"serviceName": "redis.static",
			"servicePort": 6379,
			"timeout":     1000,
			"database":    0,
		},
		"questionFrom": map[string]interface{}{
			"requestBody": "messages.@reverse.0.content",
		},
		"answerValueFrom": map[string]interface{}{
			"responseBody": "choices.0.message.content",
		},
		"answerStreamValueFrom": map[string]interface{}{
			"responseBody": "choices.0.delta.content",
		},
		"cacheKeyPrefix": "higress-ai-history:",
		"identityHeader": "Authorization",
		"fillHistoryCnt": 3,
		"cacheTTL":       3600,
	})
	return data
}()

// 测试配置：最小Redis配置（使用默认值）
var minimalRedisConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"redis": map[string]interface{}{
			"serviceName": "redis.static",
		},
		"questionFrom": map[string]interface{}{
			"requestBody": "messages.@reverse.0.content",
		},
		"answerValueFrom": map[string]interface{}{
			"responseBody": "choices.0.message.content",
		},
		"answerStreamValueFrom": map[string]interface{}{
			"responseBody": "choices.0.delta.content",
		},
	})
	return data
}()

// 测试配置：自定义Redis配置
var customRedisConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"redis": map[string]interface{}{
			"serviceName": "custom-redis.dns",
			"servicePort": 6380,
			"username":    "admin",
			"password":    "password123",
			"timeout":     2000,
			"database":    1,
		},
		"questionFrom": map[string]interface{}{
			"requestBody": "query.text",
		},
		"answerValueFrom": map[string]interface{}{
			"responseBody": "response.content",
		},
		"answerStreamValueFrom": map[string]interface{}{
			"responseBody": "response.delta.content",
		},
		"cacheKeyPrefix": "custom-history:",
		"identityHeader": "X-User-ID",
		"fillHistoryCnt": 5,
		"cacheTTL":       7200,
	})
	return data
}()

// 测试配置：带认证的Redis配置
var authRedisConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"redis": map[string]interface{}{
			"serviceName": "auth-redis.static",
			"servicePort": 6379,
			"username":    "user",
			"password":    "pass",
			"timeout":     1500,
			"database":    2,
		},
		"questionFrom": map[string]interface{}{
			"requestBody": "messages.@reverse.0.content",
		},
		"answerValueFrom": map[string]interface{}{
			"responseBody": "choices.0.message.content",
		},
		"answerStreamValueFrom": map[string]interface{}{
			"responseBody": "choices.0.delta.content",
		},
		"cacheKeyPrefix": "auth-history:",
		"identityHeader": "X-Auth-Token",
		"fillHistoryCnt": 4,
		"cacheTTL":       1800,
	})
	return data
}()

func TestDistinctChat(t *testing.T) {
	type args struct {
		chat        []ChatHistory
		currMessage []ChatHistory
	}
	firstChat := []ChatHistory{{Role: "user", Content: "userInput1"}, {Role: "assistant", Content: "assistantOutput1"}}
	sendUser := []ChatHistory{{Role: "user", Content: "userInput2"}}
	tests := []struct {
		name string
		args args
		want []ChatHistory
	}{
		{name: "填充历史", args: args{
			chat:        append([]ChatHistory{}, firstChat...),
			currMessage: append([]ChatHistory{}, sendUser...)},
			want: append(append([]ChatHistory{}, firstChat...), sendUser...)},
		{name: "无需填充", args: args{
			chat:        append([]ChatHistory{}, firstChat...),
			currMessage: append(append([]ChatHistory{}, firstChat...), sendUser...)},
			want: append(append([]ChatHistory{}, firstChat...), sendUser...)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fillHistory(tt.args.chat, tt.args.currMessage, 3); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("fillHistory() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetIntQueryParameter(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		defaultValue int
		want         int
	}{
		{name: "positive value", path: "/api/chat?cnt=2", defaultValue: 3, want: 2},
		{name: "zero value", path: "/api/chat?cnt=0", defaultValue: 3, want: 0},
		{name: "negative value", path: "/api/chat?cnt=-1", defaultValue: 3, want: 3},
		{name: "invalid value", path: "/api/chat?cnt=invalid", defaultValue: 3, want: 3},
		{name: "missing value", path: "/api/chat", defaultValue: 3, want: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, getIntQueryParameter("cnt", tt.path, tt.defaultValue))
		})
	}
}

func TestParseConfig(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// 测试基本Redis配置解析
		t.Run("basic redis config", func(t *testing.T) {
			host, status := test.NewTestHost(basicRedisConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			// 类型断言
			pluginConfig, ok := config.(*PluginConfig)
			require.True(t, ok, "config should be *PluginConfig")

			// 验证Redis配置字段
			require.Equal(t, "redis.static", pluginConfig.RedisInfo.ServiceName)
			require.Equal(t, 6379, pluginConfig.RedisInfo.ServicePort)
			require.Equal(t, 1000, pluginConfig.RedisInfo.Timeout)
			require.Equal(t, 0, pluginConfig.RedisInfo.Database)
			require.Equal(t, "", pluginConfig.RedisInfo.Username)
			require.Equal(t, "", pluginConfig.RedisInfo.Password)

			// 验证问题提取配置
			require.Equal(t, "messages.@reverse.0.content", pluginConfig.QuestionFrom.RequestBody)
			require.Equal(t, "", pluginConfig.QuestionFrom.ResponseBody)

			// 验证答案提取配置
			require.Equal(t, "", pluginConfig.AnswerValueFrom.RequestBody)
			require.Equal(t, "choices.0.message.content", pluginConfig.AnswerValueFrom.ResponseBody)

			// 验证流式答案提取配置
			require.Equal(t, "", pluginConfig.AnswerStreamValueFrom.RequestBody)
			require.Equal(t, "choices.0.delta.content", pluginConfig.AnswerStreamValueFrom.ResponseBody)

			// 验证其他配置字段
			require.Equal(t, "higress-ai-history:", pluginConfig.CacheKeyPrefix)
			require.Equal(t, "Authorization", pluginConfig.IdentityHeader)
			require.Equal(t, 3, pluginConfig.FillHistoryCnt)
			require.Equal(t, 3600, pluginConfig.CacheTTL)
		})

		// 测试最小Redis配置解析（使用默认值）
		t.Run("minimal redis config", func(t *testing.T) {
			host, status := test.NewTestHost(minimalRedisConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			// 类型断言
			pluginConfig, ok := config.(*PluginConfig)
			require.True(t, ok, "config should be *PluginConfig")

			// 验证Redis配置字段（使用默认值）
			require.Equal(t, "redis.static", pluginConfig.RedisInfo.ServiceName)
			require.Equal(t, 80, pluginConfig.RedisInfo.ServicePort) // 对于.static服务，默认端口是80
			require.Equal(t, 1000, pluginConfig.RedisInfo.Timeout)   // 默认超时
			require.Equal(t, 0, pluginConfig.RedisInfo.Database)     // 默认数据库
			require.Equal(t, "", pluginConfig.RedisInfo.Username)
			require.Equal(t, "", pluginConfig.RedisInfo.Password)

			// 验证问题提取配置（使用默认值）
			require.Equal(t, "messages.@reverse.0.content", pluginConfig.QuestionFrom.RequestBody)
			require.Equal(t, "", pluginConfig.QuestionFrom.ResponseBody)

			// 验证答案提取配置（使用默认值）
			require.Equal(t, "", pluginConfig.AnswerValueFrom.RequestBody)
			require.Equal(t, "choices.0.message.content", pluginConfig.AnswerValueFrom.ResponseBody)

			// 验证流式答案提取配置（使用默认值）
			require.Equal(t, "", pluginConfig.AnswerStreamValueFrom.RequestBody)
			require.Equal(t, "choices.0.delta.content", pluginConfig.AnswerStreamValueFrom.ResponseBody)

			// 验证其他配置字段（使用默认值）
			require.Equal(t, "higress-ai-history:", pluginConfig.CacheKeyPrefix)
			require.Equal(t, "Authorization", pluginConfig.IdentityHeader)
			require.Equal(t, 3, pluginConfig.FillHistoryCnt)
			require.Equal(t, 0, pluginConfig.CacheTTL) // 默认永不过期
		})

		// 测试自定义Redis配置解析
		t.Run("custom redis config", func(t *testing.T) {
			host, status := test.NewTestHost(customRedisConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			// 类型断言
			pluginConfig, ok := config.(*PluginConfig)
			require.True(t, ok, "config should be *PluginConfig")

			// 验证Redis配置字段
			require.Equal(t, "custom-redis.dns", pluginConfig.RedisInfo.ServiceName)
			require.Equal(t, 6380, pluginConfig.RedisInfo.ServicePort)
			require.Equal(t, 2000, pluginConfig.RedisInfo.Timeout)
			require.Equal(t, 1, pluginConfig.RedisInfo.Database)
			require.Equal(t, "admin", pluginConfig.RedisInfo.Username)
			require.Equal(t, "password123", pluginConfig.RedisInfo.Password)

			// 验证问题提取配置（插件硬编码，不从配置读取）
			require.Equal(t, "messages.@reverse.0.content", pluginConfig.QuestionFrom.RequestBody)
			require.Equal(t, "", pluginConfig.QuestionFrom.ResponseBody)

			// 验证答案提取配置（插件硬编码，不从配置读取）
			require.Equal(t, "", pluginConfig.AnswerValueFrom.RequestBody)
			require.Equal(t, "choices.0.message.content", pluginConfig.AnswerValueFrom.ResponseBody)

			// 验证流式答案提取配置（插件硬编码，不从配置读取）
			require.Equal(t, "", pluginConfig.AnswerStreamValueFrom.RequestBody)
			require.Equal(t, "choices.0.delta.content", pluginConfig.AnswerStreamValueFrom.ResponseBody)

			// 验证其他配置字段
			require.Equal(t, "custom-history:", pluginConfig.CacheKeyPrefix)
			require.Equal(t, "X-User-ID", pluginConfig.IdentityHeader)
			require.Equal(t, 5, pluginConfig.FillHistoryCnt)
			require.Equal(t, 7200, pluginConfig.CacheTTL)
		})

		// 测试带认证的Redis配置解析
		t.Run("auth redis config", func(t *testing.T) {
			host, status := test.NewTestHost(authRedisConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			// 类型断言
			pluginConfig, ok := config.(*PluginConfig)
			require.True(t, ok, "config should be *PluginConfig")

			// 验证Redis配置字段
			require.Equal(t, "auth-redis.static", pluginConfig.RedisInfo.ServiceName)
			require.Equal(t, 6379, pluginConfig.RedisInfo.ServicePort)
			require.Equal(t, 1500, pluginConfig.RedisInfo.Timeout)
			require.Equal(t, 2, pluginConfig.RedisInfo.Database)
			require.Equal(t, "user", pluginConfig.RedisInfo.Username)
			require.Equal(t, "pass", pluginConfig.RedisInfo.Password)

			// 验证问题提取配置
			require.Equal(t, "messages.@reverse.0.content", pluginConfig.QuestionFrom.RequestBody)
			require.Equal(t, "", pluginConfig.QuestionFrom.ResponseBody)

			// 验证答案提取配置
			require.Equal(t, "", pluginConfig.AnswerValueFrom.RequestBody)
			require.Equal(t, "choices.0.message.content", pluginConfig.AnswerValueFrom.ResponseBody)

			// 验证流式答案提取配置
			require.Equal(t, "", pluginConfig.AnswerStreamValueFrom.RequestBody)
			require.Equal(t, "choices.0.delta.content", pluginConfig.AnswerStreamValueFrom.ResponseBody)

			// 验证其他配置字段
			require.Equal(t, "auth-history:", pluginConfig.CacheKeyPrefix)
			require.Equal(t, "X-Auth-Token", pluginConfig.IdentityHeader)
			require.Equal(t, 4, pluginConfig.FillHistoryCnt)
			require.Equal(t, 1800, pluginConfig.CacheTTL)
		})
	})
}

func TestOnHttpRequestHeaders(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试JSON内容类型的请求头处理
		t.Run("JSON content type headers", func(t *testing.T) {
			host, status := test.NewTestHost(basicRedisConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置JSON内容类型的请求头，包含身份标识
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"content-type", "application/json"},
				{"authorization", "Bearer user123"},
			})

			// 应该返回HeaderStopIteration，因为需要读取请求体
			require.Equal(t, types.HeaderStopIteration, action)
		})

		// 测试非JSON内容类型的请求头处理
		t.Run("non-JSON content type headers", func(t *testing.T) {
			host, status := test.NewTestHost(basicRedisConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置非JSON内容类型的请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"content-type", "text/plain"},
				{"authorization", "Bearer user123"},
			})

			// 应该返回ActionContinue，但不会读取请求体
			require.Equal(t, types.ActionContinue, action)
		})

		// 测试缺少身份标识的请求头处理
		t.Run("missing identity header", func(t *testing.T) {
			host, status := test.NewTestHost(basicRedisConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置缺少身份标识的请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			// 应该返回ActionContinue，因为缺少身份标识
			require.Equal(t, types.ActionContinue, action)
		})

		// 测试自定义身份标识头
		t.Run("custom identity header", func(t *testing.T) {
			host, status := test.NewTestHost(customRedisConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置自定义身份标识头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"content-type", "application/json"},
				{"x-user-id", "user456"},
			})

			// 应该返回HeaderStopIteration
			require.Equal(t, types.HeaderStopIteration, action)
		})
	})
}

func TestOnHttpRequestBody(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试缓存命中的请求体处理
		t.Run("cache hit request body", func(t *testing.T) {
			host, status := test.NewTestHost(basicRedisConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"content-type", "application/json"},
				{"authorization", "Bearer user123"},
			})

			// 构造请求体
			requestBody := `{
				"messages": [
					{
						"role": "user",
						"content": "你好，请介绍一下自己"
					}
				]
			}`

			// 调用请求体处理
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			// 应该返回ActionPause，因为需要等待Redis响应
			require.Equal(t, types.ActionPause, action)

			// 模拟Redis缓存命中响应
			cacheResponse := `[{"role":"user","content":"之前的问题"},{"role":"assistant","content":"之前的回答"}]`
			resp := test.CreateRedisRespString(cacheResponse)
			host.CallOnRedisCall(0, resp)

			// 完成HTTP请求
			host.CompleteHttp()
		})

		// 测试缓存未命中的请求体处理
		t.Run("cache miss request body", func(t *testing.T) {
			host, status := test.NewTestHost(basicRedisConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"content-type", "application/json"},
				{"authorization", "Bearer user123"},
			})

			// 构造请求体
			requestBody := `{
				"messages": [
					{
						"role": "user",
						"content": "今天天气怎么样？"
					}
				]
			}`

			// 调用请求体处理
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			// 应该返回ActionPause，因为需要等待Redis响应
			require.Equal(t, types.ActionPause, action)

			// 模拟Redis缓存未命中响应
			resp := test.CreateRedisRespNull()
			host.CallOnRedisCall(0, resp)

			// 完成HTTP请求
			host.CompleteHttp()
		})

		// 测试流式请求的请求体处理
		t.Run("streaming request body", func(t *testing.T) {
			host, status := test.NewTestHost(basicRedisConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"content-type", "application/json"},
				{"authorization", "Bearer user123"},
			})

			// 构造流式请求体
			requestBody := `{
				"stream": true,
				"messages": [
					{
						"role": "user",
						"content": "请用流式方式回答"
					}
				]
			}`

			// 调用请求体处理
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			// 应该返回ActionPause，因为需要等待Redis响应
			require.Equal(t, types.ActionPause, action)

			// 模拟Redis缓存未命中响应
			resp := test.CreateRedisRespNull()
			host.CallOnRedisCall(0, resp)

			// 完成HTTP请求
			host.CompleteHttp()
		})

		// 测试查询历史请求的请求体处理
		t.Run("query history request body", func(t *testing.T) {
			host, status := test.NewTestHost(basicRedisConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/ai-history/query?cnt=2"},
				{":method", "GET"},
				{"content-type", "application/json"},
				{"authorization", "Bearer user123"},
			})

			// 构造请求体（需要包含messages字段，因为插件会尝试提取问题）
			requestBody := `{
				"messages": [
					{
						"role": "user",
						"content": "查询历史"
					}
				]
			}`

			// 调用请求体处理
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			// 应该返回ActionPause，因为需要等待Redis响应
			require.Equal(t, types.ActionPause, action)

			// 模拟Redis缓存命中响应
			cacheResponse := `[{"role":"user","content":"问题1"},{"role":"assistant","content":"回答1"},{"role":"user","content":"问题2"},{"role":"assistant","content":"回答2"}]`
			resp := test.CreateRedisRespString(cacheResponse)
			host.CallOnRedisCall(0, resp)

			// 完成HTTP请求
			host.CompleteHttp()
		})
	})
}

func TestOnHttpResponseHeaders(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试流式响应头处理
		t.Run("streaming response headers", func(t *testing.T) {
			host, status := test.NewTestHost(basicRedisConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 必须先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"content-type", "application/json"},
				{"authorization", "Bearer user123"},
			})

			// 设置流式响应头
			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "text/event-stream"},
			})

			// 应该返回ActionContinue
			require.Equal(t, types.ActionContinue, action)
		})

		// 测试非流式响应头处理
		t.Run("non-streaming response headers", func(t *testing.T) {
			host, status := test.NewTestHost(basicRedisConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 必须先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"content-type", "application/json"},
				{"authorization", "Bearer user123"},
			})

			// 设置非流式响应头
			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})

			// 应该返回ActionContinue
			require.Equal(t, types.ActionContinue, action)
		})
	})
}

func TestOnHttpStreamResponseBody(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试流式响应体处理 - 非流式模式
		t.Run("non-streaming mode", func(t *testing.T) {
			host, status := test.NewTestHost(basicRedisConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"content-type", "application/json"},
				{"authorization", "Bearer user123"},
			})

			// 设置请求体
			requestBody := `{
				"messages": [
					{
						"role": "user",
						"content": "测试问题"
					}
				]
			}`

			// 调用请求体处理，设置必要的上下文
			host.CallOnHttpRequestBody([]byte(requestBody))

			// 模拟Redis缓存未命中，设置QuestionContextKey
			resp := test.CreateRedisRespNull()
			host.CallOnRedisCall(0, resp)

			// 测试非流式响应体处理
			chunk := []byte(`{"choices":[{"message":{"content":"测试回答"}}]}`)
			action := host.CallOnHttpStreamingResponseBody(chunk, true)

			// 应该返回ActionContinue
			require.Equal(t, types.ActionContinue, action)
		})

		// 测试流式响应体处理 - 流式模式
		t.Run("streaming mode", func(t *testing.T) {
			host, status := test.NewTestHost(basicRedisConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"content-type", "application/json"},
				{"authorization", "Bearer user123"},
			})

			// 设置流式请求体
			requestBody := `{
				"stream": true,
				"messages": [
					{
						"role": "user",
						"content": "测试流式问题"
					}
				]
			}`

			// 调用请求体处理，设置必要的上下文
			host.CallOnHttpRequestBody([]byte(requestBody))

			// 模拟Redis缓存未命中，设置QuestionContextKey
			resp := test.CreateRedisRespNull()
			host.CallOnRedisCall(0, resp)

			// 设置流式响应头
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "text/event-stream"},
			})

			// 测试流式响应体处理 - 非最后一个chunk
			chunk1 := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n")
			action1 := host.CallOnHttpStreamingResponseBody(chunk1, false)
			require.Equal(t, types.ActionContinue, action1)

			// 测试流式响应体处理 - 最后一个chunk
			chunk2 := []byte("data: {\"choices\":[{\"delta\":{\"content\":\" World\"}}]}\n\n")
			action2 := host.CallOnHttpStreamingResponseBody(chunk2, true)
			require.Equal(t, types.ActionContinue, action2)
		})

		// 测试查询历史路径的流式响应体处理
		t.Run("query history path", func(t *testing.T) {
			host, status := test.NewTestHost(basicRedisConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/ai-history/query?cnt=2"},
				{":method", "GET"},
				{"content-type", "application/json"},
				{"authorization", "Bearer user123"},
			})

			// 设置请求体
			requestBody := `{
				"messages": [
					{
						"role": "user",
						"content": "查询历史"
					}
				]
			}`

			// 调用请求体处理，设置必要的上下文
			host.CallOnHttpRequestBody([]byte(requestBody))

			// 模拟Redis缓存命中，设置QuestionContextKey
			cacheResponse := `[{"role":"user","content":"问题1"},{"role":"assistant","content":"回答1"}]`
			resp := test.CreateRedisRespString(cacheResponse)
			host.CallOnRedisCall(0, resp)

			// 测试查询历史路径的响应体处理
			chunk := []byte("test chunk")
			action := host.CallOnHttpStreamingResponseBody(chunk, true)

			// 应该直接返回chunk，不进行处理
			require.Equal(t, types.ActionContinue, action)
		})

		// 测试没有QuestionContextKey的情况
		t.Run("no question context", func(t *testing.T) {
			host, status := test.NewTestHost(basicRedisConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"content-type", "application/json"},
				{"authorization", "Bearer user123"},
			})

			// 不调用请求体处理，所以没有QuestionContextKey

			// 测试没有QuestionContextKey的响应体处理
			chunk := []byte("test chunk")
			action := host.CallOnHttpStreamingResponseBody(chunk, true)

			// 应该返回ActionContinue
			require.Equal(t, types.ActionContinue, action)
		})
	})
}
