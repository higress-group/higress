// Copyright (c) 2023 Alibaba Group Holding Ltd.
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
	"strings"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

// 测试配置：基本配置 - 只启用请求头部日志
var basicConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"request": map[string]interface{}{
			"headers": map[string]interface{}{
				"enabled": true,
			},
			"body": map[string]interface{}{
				"enabled": false,
			},
		},
		"response": map[string]interface{}{
			"headers": map[string]interface{}{
				"enabled": false,
			},
			"body": map[string]interface{}{
				"enabled": false,
			},
		},
	})
	return data
}()

// 测试配置：完整配置 - 启用所有日志功能
var fullConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"request": map[string]interface{}{
			"headers": map[string]interface{}{
				"enabled": true,
			},
			"body": map[string]interface{}{
				"enabled":      true,
				"maxSize":      1024,
				"contentTypes": []string{"application/json", "text/plain"},
			},
		},
		"response": map[string]interface{}{
			"headers": map[string]interface{}{
				"enabled": true,
			},
			"body": map[string]interface{}{
				"enabled":      true,
				"maxSize":      2048,
				"contentTypes": []string{"application/json", "text/html"},
			},
		},
	})
	return data
}()

// 测试配置：自定义内容类型配置
var customContentTypesConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"request": map[string]interface{}{
			"headers": map[string]interface{}{
				"enabled": true,
			},
			"body": map[string]interface{}{
				"enabled":      true,
				"maxSize":      512,
				"contentTypes": []string{"application/xml", "text/csv"},
			},
		},
		"response": map[string]interface{}{
			"headers": map[string]interface{}{
				"enabled": true,
			},
			"body": map[string]interface{}{
				"enabled":      true,
				"maxSize":      512,
				"contentTypes": []string{"application/xml", "text/csv"},
			},
		},
	})
	return data
}()

// 测试配置：大文件配置 - 测试大小限制
var largeFileConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"request": map[string]interface{}{
			"headers": map[string]interface{}{
				"enabled": true,
			},
			"body": map[string]interface{}{
				"enabled":      true,
				"maxSize":      100,
				"contentTypes": []string{"text/plain"},
			},
		},
		"response": map[string]interface{}{
			"headers": map[string]interface{}{
				"enabled": true,
			},
			"body": map[string]interface{}{
				"enabled":      true,
				"maxSize":      100,
				"contentTypes": []string{"text/plain"},
			},
		},
	})
	return data
}()

// 测试配置：默认值配置 - 不指定 maxSize 和 contentTypes
var defaultValuesConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"request": map[string]interface{}{
			"headers": map[string]interface{}{
				"enabled": true,
			},
			"body": map[string]interface{}{
				"enabled": true,
			},
		},
		"response": map[string]interface{}{
			"headers": map[string]interface{}{
				"enabled": true,
			},
			"body": map[string]interface{}{
				"enabled": true,
			},
		},
	})
	return data
}()

// 测试配置：最小配置 - 只启用必要的功能
var minimalConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"request": map[string]interface{}{
			"headers": map[string]interface{}{
				"enabled": false,
			},
			"body": map[string]interface{}{
				"enabled": false,
			},
		},
		"response": map[string]interface{}{
			"headers": map[string]interface{}{
				"enabled": false,
			},
			"body": map[string]interface{}{
				"enabled": false,
			},
		},
	})
	return data
}()

// 测试配置：状态码条件 - 只记录 >= 400 的错误请求（开启全部日志项）
var errorOnlyConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"statusCode": map[string]interface{}{
			"operator": ">=",
			"value":    400,
		},
		"request": map[string]interface{}{
			"headers": map[string]interface{}{
				"enabled": true,
			},
			"body": map[string]interface{}{
				"enabled": true,
			},
		},
		"response": map[string]interface{}{
			"headers": map[string]interface{}{
				"enabled": true,
			},
			"body": map[string]interface{}{
				"enabled": true,
			},
		},
	})
	return data
}()

// 测试配置：状态码条件 - 只记录 404（显式使用 = 操作符）
var notFoundOnlyConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"statusCode": map[string]interface{}{
			"operator": "=",
			"value":    404,
		},
		"request": map[string]interface{}{
			"headers": map[string]interface{}{
				"enabled": true,
			},
			"body": map[string]interface{}{
				"enabled": true,
			},
		},
		"response": map[string]interface{}{
			"headers": map[string]interface{}{
				"enabled": true,
			},
			"body": map[string]interface{}{
				"enabled": true,
			},
		},
	})
	return data
}()

// 测试配置：状态码条件 - 省略 operator（默认使用 >=）
var defaultOperatorConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"statusCode": map[string]interface{}{
			"value": 500,
		},
		"request": map[string]interface{}{
			"headers": map[string]interface{}{
				"enabled": false,
			},
			"body": map[string]interface{}{
				"enabled": false,
			},
		},
		"response": map[string]interface{}{
			"headers": map[string]interface{}{
				"enabled": false,
			},
			"body": map[string]interface{}{
				"enabled": false,
			},
		},
	})
	return data
}()

// 测试配置：非法操作符 - 应导致插件加载失败
var invalidOperatorConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"statusCode": map[string]interface{}{
			"operator": "~",
			"value":    400,
		},
	})
	return data
}()

// 测试配置：非法状态码取值 - 应导致插件加载失败
var invalidValueConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"statusCode": map[string]interface{}{
			"operator": ">=",
			"value":    99,
		},
	})
	return data
}()

func TestParseConfig(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// 测试基本配置解析
		t.Run("basic config", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			pluginConfig := config.(*PluginConfig)
			require.True(t, pluginConfig.Request.Headers.Enabled)
			require.False(t, pluginConfig.Request.Body.Enabled)
			require.False(t, pluginConfig.Response.Headers.Enabled)
			require.False(t, pluginConfig.Response.Body.Enabled)
		})

		// 测试完整配置解析
		t.Run("full config", func(t *testing.T) {
			host, status := test.NewTestHost(fullConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			pluginConfig := config.(*PluginConfig)
			require.True(t, pluginConfig.Request.Headers.Enabled)
			require.True(t, pluginConfig.Request.Body.Enabled)
			require.Equal(t, 1024, pluginConfig.Request.Body.MaxSize)
			require.Len(t, pluginConfig.Request.Body.ContentTypes, 2)
			require.Equal(t, "application/json", pluginConfig.Request.Body.ContentTypes[0])
			require.Equal(t, "text/plain", pluginConfig.Request.Body.ContentTypes[1])

			require.True(t, pluginConfig.Response.Headers.Enabled)
			require.True(t, pluginConfig.Response.Body.Enabled)
			require.Equal(t, 2048, pluginConfig.Response.Body.MaxSize)
			require.Len(t, pluginConfig.Response.Body.ContentTypes, 2)
			require.Equal(t, "application/json", pluginConfig.Response.Body.ContentTypes[0])
			require.Equal(t, "text/html", pluginConfig.Response.Body.ContentTypes[1])
		})

		// 测试自定义内容类型配置
		t.Run("custom content types config", func(t *testing.T) {
			host, status := test.NewTestHost(customContentTypesConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			pluginConfig := config.(*PluginConfig)
			require.Len(t, pluginConfig.Request.Body.ContentTypes, 2)
			require.Equal(t, "application/xml", pluginConfig.Request.Body.ContentTypes[0])
			require.Equal(t, "text/csv", pluginConfig.Request.Body.ContentTypes[1])

			require.Len(t, pluginConfig.Response.Body.ContentTypes, 2)
			require.Equal(t, "application/xml", pluginConfig.Response.Body.ContentTypes[0])
			require.Equal(t, "text/csv", pluginConfig.Response.Body.ContentTypes[1])
		})

		// 测试大文件配置
		t.Run("large file config", func(t *testing.T) {
			host, status := test.NewTestHost(largeFileConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			pluginConfig := config.(*PluginConfig)
			require.Equal(t, 100, pluginConfig.Request.Body.MaxSize)
			require.Equal(t, 100, pluginConfig.Response.Body.MaxSize)
		})

		// 测试默认值配置
		t.Run("default values config", func(t *testing.T) {
			host, status := test.NewTestHost(defaultValuesConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			pluginConfig := config.(*PluginConfig)
			// 默认 maxSize 应该是 10KB
			require.Equal(t, 10*1024, pluginConfig.Request.Body.MaxSize)
			require.Equal(t, 10*1024, pluginConfig.Response.Body.MaxSize)

			// 默认内容类型
			require.Len(t, pluginConfig.Request.Body.ContentTypes, 4)
			require.Contains(t, pluginConfig.Request.Body.ContentTypes, "application/json")
			require.Contains(t, pluginConfig.Request.Body.ContentTypes, "text/plain")

			require.Len(t, pluginConfig.Response.Body.ContentTypes, 4)
			require.Contains(t, pluginConfig.Response.Body.ContentTypes, "application/json")
			require.Contains(t, pluginConfig.Response.Body.ContentTypes, "text/html")
		})

		// 测试最小配置
		t.Run("minimal config", func(t *testing.T) {
			host, status := test.NewTestHost(minimalConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			pluginConfig := config.(*PluginConfig)
			require.False(t, pluginConfig.Request.Headers.Enabled)
			require.False(t, pluginConfig.Request.Body.Enabled)
			require.False(t, pluginConfig.Response.Headers.Enabled)
			require.False(t, pluginConfig.Response.Body.Enabled)
		})
	})
}

func TestOnHttpRequestHeaders(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试请求头部日志 - 启用
		t.Run("request headers logging enabled", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":method", "GET"},
				{":path", "/test"},
				{":scheme", "https"},
				{"content-type", "application/json"},
				{"user-agent", "test-agent"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			host.CompleteHttp()
		})

		// 测试请求头部日志 - 禁用
		t.Run("request headers logging disabled", func(t *testing.T) {
			host, status := test.NewTestHost(minimalConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":method", "GET"},
				{":path", "/test"},
				{":scheme", "https"},
				{"content-type", "application/json"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			host.CompleteHttp()
		})

		// 测试请求体日志 - POST 请求，内容类型匹配
		t.Run("request body logging enabled - POST with matching content type", func(t *testing.T) {
			host, status := test.NewTestHost(fullConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":method", "POST"},
				{":path", "/test"},
				{":scheme", "https"},
				{"content-type", "application/json"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			host.CompleteHttp()
		})

		// 测试请求体日志 - POST 请求，内容类型不匹配
		t.Run("request body logging enabled - POST with non-matching content type", func(t *testing.T) {
			host, status := test.NewTestHost(fullConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":method", "POST"},
				{":path", "/test"},
				{":scheme", "https"},
				{"content-type", "image/png"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			host.CompleteHttp()
		})

		// 测试请求体日志 - GET 请求（不应该读取请求体）
		t.Run("request body logging enabled - GET request", func(t *testing.T) {
			host, status := test.NewTestHost(fullConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":method", "GET"},
				{":path", "/test"},
				{":scheme", "https"},
				{"content-type", "application/json"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			host.CompleteHttp()
		})

		// 测试请求体日志 - PUT 请求，内容类型匹配
		t.Run("request body logging enabled - PUT with matching content type", func(t *testing.T) {
			host, status := test.NewTestHost(fullConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":method", "PUT"},
				{":path", "/test"},
				{":scheme", "https"},
				{"content-type", "text/plain"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			host.CompleteHttp()
		})

		// 测试请求体日志 - PATCH 请求，内容类型匹配
		t.Run("request body logging enabled - PATCH with matching content type", func(t *testing.T) {
			host, status := test.NewTestHost(fullConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":method", "PATCH"},
				{":path", "/test"},
				{":scheme", "https"},
				{"content-type", "application/json"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			host.CompleteHttp()
		})
	})
}

func TestOnHttpResponseHeaders(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试响应头部日志 - 启用
		t.Run("response headers logging enabled", func(t *testing.T) {
			host, status := test.NewTestHost(fullConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
				{"content-length", "123"},
				{"server", "test-server"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			host.CompleteHttp()
		})

		// 测试响应头部日志 - 禁用
		t.Run("response headers logging disabled", func(t *testing.T) {
			host, status := test.NewTestHost(minimalConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
				{"content-length", "123"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			host.CompleteHttp()
		})

		// 测试响应体日志 - 内容类型匹配
		t.Run("response body logging enabled - matching content type", func(t *testing.T) {
			host, status := test.NewTestHost(fullConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
				{"content-length", "123"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			host.CompleteHttp()
		})

		// 测试响应体日志 - 内容类型不匹配
		t.Run("response body logging enabled - non-matching content type", func(t *testing.T) {
			host, status := test.NewTestHost(fullConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "image/png"},
				{"content-length", "123"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			host.CompleteHttp()
		})

		// 测试响应体日志 - 没有 content-type
		t.Run("response body logging enabled - no content type", func(t *testing.T) {
			host, status := test.NewTestHost(fullConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-length", "123"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			host.CompleteHttp()
		})
	})
}

func TestOnStreamingRequestBody(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试流式请求体处理 - 小数据
		t.Run("streaming request body - small data", func(t *testing.T) {
			host, status := test.NewTestHost(fullConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":method", "POST"},
				{":path", "/test"},
				{":scheme", "https"},
				{"content-type", "application/json"},
			})

			// 测试流式请求体
			testData := []byte(`{"key": "value"}`)
			action := host.CallOnHttpStreamingRequestBody(testData, true)
			require.Equal(t, types.ActionContinue, action)
			result := host.GetRequestBody()
			require.Equal(t, testData, result, "Request body should be returned unchanged")
		})

		// 测试流式请求体处理 - 大数据（超过限制）
		t.Run("streaming request body - large data", func(t *testing.T) {
			host, status := test.NewTestHost(largeFileConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":method", "POST"},
				{":path", "/test"},
				{":scheme", "https"},
				{"content-type", "text/plain"},
			})

			// 测试大数据（超过 100 字节限制）
			largeData := []byte(strings.Repeat("a", 200))
			action := host.CallOnHttpStreamingRequestBody(largeData, true)
			require.Equal(t, types.ActionContinue, action)
			result := host.GetRequestBody()
			require.Equal(t, largeData, result, "Request body should be returned unchanged even if large")
			host.CompleteHttp()
		})

		// 测试流式请求体处理 - 禁用
		t.Run("streaming request body - disabled", func(t *testing.T) {
			host, status := test.NewTestHost(minimalConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":method", "POST"},
				{":path", "/test"},
				{":scheme", "https"},
				{"content-type", "application/json"},
			})

			// 测试流式请求体
			testData := []byte(`{"key": "value"}`)
			action := host.CallOnHttpStreamingRequestBody(testData, true)
			require.Equal(t, types.ActionContinue, action)
			result := host.GetRequestBody()
			require.Equal(t, testData, result, "Request body should be returned unchanged when disabled")
			host.CompleteHttp()
		})
	})
}

func TestOnStreamingResponseBody(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试流式响应体处理 - 小数据
		t.Run("streaming response body - small data", func(t *testing.T) {
			host, status := test.NewTestHost(fullConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置响应头
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
				{"content-length", "123"},
			})

			// 测试流式响应体
			testData := []byte(`{"status": "success"}`)
			action := host.CallOnHttpStreamingResponseBody(testData, true)
			require.Equal(t, types.ActionContinue, action)
			result := host.GetResponseBody()
			require.Equal(t, testData, result, "Response body should be returned unchanged")
		})

		// 测试流式响应体处理 - 大数据（超过限制）
		t.Run("streaming response body - large data", func(t *testing.T) {
			host, status := test.NewTestHost(largeFileConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置响应头
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "text/plain"},
				{"content-length", "123"},
			})

			// 测试大数据（超过 100 字节限制）
			largeData := []byte(strings.Repeat("b", 200))
			action := host.CallOnHttpStreamingResponseBody(largeData, true)
			require.Equal(t, types.ActionContinue, action)
			result := host.GetResponseBody()
			require.Equal(t, largeData, result, "Response body should be returned unchanged even if large")
			host.CompleteHttp()
		})

		// 测试流式响应体处理 - 禁用
		t.Run("streaming response body - disabled", func(t *testing.T) {
			host, status := test.NewTestHost(minimalConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置响应头
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
				{"content-length", "123"},
			})

			// 测试流式响应体
			testData := []byte(`{"status": "success"}`)
			action := host.CallOnHttpStreamingResponseBody(testData, true)
			require.Equal(t, types.ActionContinue, action)
			result := host.GetResponseBody()
			require.Equal(t, testData, result, "Response body should be returned unchanged when disabled")
			host.CompleteHttp()
		})
	})
}

func TestCompleteFlow(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试完整的请求-响应流程
		t.Run("complete request-response flow", func(t *testing.T) {
			host, status := test.NewTestHost(fullConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 1. 处理请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":method", "POST"},
				{":path", "/test"},
				{":scheme", "https"},
				{"content-type", "application/json"},
				{"user-agent", "test-agent"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			// 2. 处理请求体
			requestBody := []byte(`{"name": "test", "value": "data"}`)
			action = host.CallOnHttpStreamingRequestBody(requestBody, true)
			require.Equal(t, types.ActionContinue, action)
			body := host.GetRequestBody()
			require.Equal(t, requestBody, body)

			// 3. 处理响应头
			action = host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
				{"content-length", "45"},
				{"server", "test-server"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			// 4. 处理响应体
			responseBody := []byte(`{"status": "success", "message": "ok"}`)
			action = host.CallOnHttpStreamingResponseBody(responseBody, true)
			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())
			responseBodyResult := host.GetResponseBody()
			require.Equal(t, responseBody, responseBodyResult)

			host.CompleteHttp()
		})
	})
}

// getFilterState 读取插件写入的 filter state 属性，返回内容和是否存在。
// 插件写入时为了 JSON 安全对特殊字符做了转义，这里还原为原始内容
func getFilterState(t *testing.T, host test.TestHost, key string) (string, bool) {
	t.Helper()
	raw, err := host.GetProperty([]string{key})
	if err != nil {
		return "", false
	}
	var value string
	require.NoError(t, json.Unmarshal([]byte(`"`+string(raw)+`"`), &value))
	return value, true
}

func TestParseStatusCodeConfig(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// 测试状态码条件配置解析
		t.Run("status code condition config", func(t *testing.T) {
			host, status := test.NewTestHost(errorOnlyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			pluginConfig := config.(*PluginConfig)
			require.True(t, pluginConfig.StatusCode.Enabled)
			require.Equal(t, ">=", pluginConfig.StatusCode.Operator)
			require.Equal(t, 400, pluginConfig.StatusCode.Value)
		})

		// 测试省略 operator 时默认使用 >=
		t.Run("status code condition defaults to >= when operator omitted", func(t *testing.T) {
			host, status := test.NewTestHost(defaultOperatorConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			pluginConfig := config.(*PluginConfig)
			require.True(t, pluginConfig.StatusCode.Enabled)
			require.Equal(t, ">=", pluginConfig.StatusCode.Operator)
			require.Equal(t, 500, pluginConfig.StatusCode.Value)
		})

		// 测试非法操作符导致插件加载失败
		t.Run("invalid operator fails plugin start", func(t *testing.T) {
			host, status := test.NewTestHost(invalidOperatorConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})

		// 测试非法状态码取值导致插件加载失败
		t.Run("invalid status value fails plugin start", func(t *testing.T) {
			host, status := test.NewTestHost(invalidValueConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})
	})
}

func TestStatusCodeCondition(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 未配置条件：请求头在请求阶段立即写入（保持原有行为）
		t.Run("no condition - request headers logged immediately", func(t *testing.T) {
			host, status := test.NewTestHost(fullConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":method", "GET"},
				{":path", "/test"},
				{":scheme", "https"},
			})

			_, ok := getFilterState(t, host, logKeyRequestHeaders)
			require.True(t, ok, "request headers should be logged immediately without a status code condition")

			host.CompleteHttp()
		})

		// 配置 >= 400：请求阶段先暂存，不写入
		t.Run("condition >= 400 - request data deferred before response", func(t *testing.T) {
			host, status := test.NewTestHost(errorOnlyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":method", "POST"},
				{":path", "/test"},
				{":scheme", "https"},
				{"content-type", "application/json"},
			})

			_, ok := getFilterState(t, host, logKeyRequestHeaders)
			require.False(t, ok, "deferred request headers should not be logged before the response")

			host.CompleteHttp()
		})

		// 配置 >= 400：200 响应不记录任何内容
		t.Run("condition >= 400 - 200 response skips all logging", func(t *testing.T) {
			host, status := test.NewTestHost(errorOnlyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":method", "POST"},
				{":path", "/test"},
				{":scheme", "https"},
				{"content-type", "application/json"},
			})
			host.CallOnHttpStreamingRequestBody([]byte(`{"name": "test"}`), true)
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})
			host.CallOnHttpStreamingResponseBody([]byte(`{"status": "ok"}`), true)
			host.CompleteHttp()

			for _, key := range []string{logKeyRequestHeaders, logKeyRequestBody, logKeyResponseHeaders, logKeyResponseBody} {
				_, ok := getFilterState(t, host, key)
				require.False(t, ok, "no content should be logged when status does not match, key: "+key)
			}
		})

		// 配置 >= 400：500 响应记录全部内容
		t.Run("condition >= 400 - 500 response logs everything", func(t *testing.T) {
			host, status := test.NewTestHost(errorOnlyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":method", "POST"},
				{":path", "/test"},
				{":scheme", "https"},
				{"content-type", "application/json"},
			})
			host.CallOnHttpStreamingRequestBody([]byte(`{"name": "test"}`), true)
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "500"},
				{"content-type", "application/json"},
			})
			host.CallOnHttpStreamingResponseBody([]byte(`{"error": "boom"}`), true)
			host.CompleteHttp()

			requestHeaders, ok := getFilterState(t, host, logKeyRequestHeaders)
			require.True(t, ok, "request headers should be logged when status matches")
			require.Contains(t, requestHeaders, `"method":"POST"`)

			requestBody, ok := getFilterState(t, host, logKeyRequestBody)
			require.True(t, ok, "request body should be logged when status matches")
			require.Equal(t, `{"name": "test"}`, requestBody)

			responseHeaders, ok := getFilterState(t, host, logKeyResponseHeaders)
			require.True(t, ok, "response headers should be logged when status matches")
			require.Contains(t, responseHeaders, `"status":"500"`)

			responseBody, ok := getFilterState(t, host, logKeyResponseBody)
			require.True(t, ok, "response body should be logged when status matches")
			require.Equal(t, `{"error": "boom"}`, responseBody)
		})

		// 配置 = 404：404 记录，200 不记录
		t.Run("condition = 404 - only 404 responses are logged", func(t *testing.T) {
			host, status := test.NewTestHost(notFoundOnlyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 404 响应命中条件
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":method", "GET"},
				{":path", "/missing"},
				{":scheme", "https"},
			})
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "404"},
				{"content-type", "application/json"},
			})
			_, ok := getFilterState(t, host, logKeyRequestHeaders)
			require.True(t, ok, "request headers should be logged for a 404 response")
			host.CompleteHttp()
		})

		t.Run("condition = 404 - non-404 responses are skipped", func(t *testing.T) {
			host, status := test.NewTestHost(notFoundOnlyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":method", "GET"},
				{":path", "/found"},
				{":scheme", "https"},
			})
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})
			host.CompleteHttp()

			_, ok := getFilterState(t, host, logKeyRequestHeaders)
			require.False(t, ok, "request headers should not be logged for a 200 response")
		})

		// 早响应场景：响应先于请求体结束到达，请求体流结束时用累计内容覆盖写入
		t.Run("condition >= 400 - early response logs cumulative request body", func(t *testing.T) {
			host, status := test.NewTestHost(errorOnlyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":method", "POST"},
				{":path", "/test"},
				{":scheme", "https"},
				{"content-type", "application/json"},
			})
			// 响应先到达（HTTP/2 早响应）
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "502"},
				{"content-type", "application/json"},
			})
			host.CallOnHttpStreamingRequestBody([]byte(`{"part1"`), false)
			host.CallOnHttpStreamingRequestBody([]byte(`: "part2"}`), true)
			host.CallOnHttpStreamingResponseBody([]byte(`{"error": "bad gateway"}`), true)
			host.CompleteHttp()

			requestBody, ok := getFilterState(t, host, logKeyRequestBody)
			require.True(t, ok, "request body should be logged when status matches")
			require.Equal(t, `{"part1": "part2"}`, requestBody)

			responseBody, ok := getFilterState(t, host, logKeyResponseBody)
			require.True(t, ok, "response body should be logged when status matches")
			require.Equal(t, `{"error": "bad gateway"}`, responseBody)
		})
	})
}
