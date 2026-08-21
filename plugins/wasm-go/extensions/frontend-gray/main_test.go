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

// 测试配置：基本灰度配置
var basicGrayConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"grayKey": "userid",
		"rules": []map[string]interface{}{
			{
				"name": "inner-user",
				"grayKeyValue": []string{
					"00000001",
					"00000005",
				},
			},
			{
				"name": "beta-user",
				"grayKeyValue": []string{
					"00000002",
					"00000003",
				},
				"grayTagKey": "level",
				"grayTagValue": []string{
					"level3",
					"level5",
				},
			},
		},
		"baseDeployment": map[string]interface{}{
			"version":        "base",
			"backendVersion": "base-backend",
		},
		"grayDeployments": []map[string]interface{}{
			{
				"name":           "inner-user",
				"version":        "gray",
				"enabled":        true,
				"backendVersion": "gray-backend",
			},
		},
	})
	return data
}()

// 测试配置：带 404 HTML 回退页面的配置
var htmlFallbackConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"grayKey": "userid",
		"html":    "<html><body>fallback</body></html>",
		"baseDeployment": map[string]interface{}{
			"version":        "base",
			"backendVersion": "base-backend",
		},
	})
	return data
}()

// 测试配置：按比例灰度配置
var weightGrayConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"grayKey": "userid",
		"rules": []map[string]interface{}{
			{
				"name": "inner-user",
				"grayKeyValue": []string{
					"00000001",
					"00000005",
				},
			},
		},
		"baseDeployment": map[string]interface{}{
			"version":        "base",
			"backendVersion": "base-backend",
		},
		"grayDeployments": []map[string]interface{}{
			{
				"name":           "inner-user",
				"version":        "gray",
				"enabled":        true,
				"backendVersion": "gray-backend",
				"weight":         80,
			},
		},
	})
	return data
}()

// 测试配置：带重写的配置
var rewriteConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"grayKey": "userid",
		"rules": []map[string]interface{}{
			{
				"name": "inner-user",
				"grayKeyValue": []string{
					"00000001",
					"00000005",
				},
			},
		},
		"rewrite": map[string]interface{}{
			"host": "frontend-gray.example.com",
			"indexRouting": map[string]interface{}{
				"/app1": "/mfe/app1/{version}/index.html",
				"/":     "/mfe/app1/{version}/index.html",
			},
			"fileRouting": map[string]interface{}{
				"/":      "/mfe/app1/{version}",
				"/app1/": "/mfe/app1/{version}",
			},
		},
		"baseDeployment": map[string]interface{}{
			"version":        "base",
			"backendVersion": "base-backend",
		},
		"grayDeployments": []map[string]interface{}{
			{
				"name":           "inner-user",
				"version":        "gray",
				"enabled":        true,
				"backendVersion": "gray-backend",
			},
		},
	})
	return data
}()

// 测试配置：带注入的配置
var injectionConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"grayKey": "userid",
		"rules": []map[string]interface{}{
			{
				"name": "inner-user",
				"grayKeyValue": []string{
					"00000001",
					"00000005",
				},
			},
		},
		"baseDeployment": map[string]interface{}{
			"version":        "base",
			"backendVersion": "base-backend",
		},
		"grayDeployments": []map[string]interface{}{
			{
				"name":           "inner-user",
				"version":        "gray",
				"enabled":        true,
				"backendVersion": "gray-backend",
			},
		},
		"injection": map[string]interface{}{
			"head": []string{
				"<script>console.log('Header')</script>",
			},
			"body": map[string]interface{}{
				"first": []string{
					"<script>console.log('hello world before')</script>",
				},
				"last": []string{
					"<script>console.log('hello world after')</script>",
				},
			},
			"globalConfig": map[string]interface{}{
				"enabled":    true,
				"key":        "TEST_CONFIG",
				"featureKey": "FEATURE_STATUS",
				"value":      "testValue",
			},
		},
	})
	return data
}()

// 测试配置：带跳过路径的配置
var skippedPathsConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"grayKey": "userid",
		"rules": []map[string]interface{}{
			{
				"name": "inner-user",
				"grayKeyValue": []string{
					"00000001",
					"00000005",
				},
			},
		},
		"skippedPaths": []string{
			"/api/**",
			"/static/**",
		},
		"indexPaths": []string{
			"/app1/**",
			"/index.html",
		},
		"baseDeployment": map[string]interface{}{
			"version":        "base",
			"backendVersion": "base-backend",
		},
		"grayDeployments": []map[string]interface{}{
			{
				"name":           "inner-user",
				"version":        "gray",
				"enabled":        true,
				"backendVersion": "gray-backend",
			},
		},
	})
	return data
}()

func TestParseConfig(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// 测试基本灰度配置解析
		t.Run("basic gray config", func(t *testing.T) {
			host, status := test.NewTestHost(basicGrayConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试按比例灰度配置解析
		t.Run("weight gray config", func(t *testing.T) {
			host, status := test.NewTestHost(weightGrayConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试带重写的配置解析
		t.Run("rewrite config", func(t *testing.T) {
			host, status := test.NewTestHost(rewriteConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试带注入的配置解析
		t.Run("injection config", func(t *testing.T) {
			host, status := test.NewTestHost(injectionConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试带跳过路径的配置解析
		t.Run("skipped paths config", func(t *testing.T) {
			host, status := test.NewTestHost(skippedPathsConfig)
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
		// 测试基本灰度请求头处理
		t.Run("basic gray request headers", func(t *testing.T) {
			host, status := test.NewTestHost(basicGrayConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，包含灰度用户 ID
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/"},
				{":method", "GET"},
				{"cookie", "userid=00000001"},
			})

			require.Equal(t, types.ActionContinue, action)

			// 验证是否添加了版本标签头
			requestHeaders := host.GetRequestHeaders()
			require.True(t, test.HasHeader(requestHeaders, "x-higress-tag"))

			host.CompleteHttp()
		})

		// 测试按比例灰度请求头处理
		t.Run("weight gray request headers", func(t *testing.T) {
			host, status := test.NewTestHost(weightGrayConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/"},
				{":method", "GET"},
				{"cookie", "userid=00000001"},
			})

			require.Equal(t, types.ActionContinue, action)

			// 验证是否添加了版本标签头
			requestHeaders := host.GetRequestHeaders()
			require.True(t, test.HasHeader(requestHeaders, "x-higress-tag"))

			host.CompleteHttp()
		})

		// 测试带重写的请求头处理
		t.Run("rewrite request headers", func(t *testing.T) {
			host, status := test.NewTestHost(rewriteConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/app1"},
				{":method", "GET"},
				{"cookie", "userid=00000001"},
			})

			require.Equal(t, types.ActionContinue, action)

			// 验证是否添加了版本标签头
			requestHeaders := host.GetRequestHeaders()
			require.True(t, test.HasHeader(requestHeaders, "x-higress-tag"))

			host.CompleteHttp()
		})

		// 测试跳过路径的请求头处理
		t.Run("skipped paths request headers", func(t *testing.T) {
			host, status := test.NewTestHost(skippedPathsConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 测试跳过路径
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/users"},
				{":method", "GET"},
				{"cookie", "userid=00000001"},
			})

			require.Equal(t, types.ActionContinue, action)

			// 跳过路径不应该添加版本标签头
			requestHeaders := host.GetRequestHeaders()
			require.False(t, test.HasHeader(requestHeaders, "x-higress-tag"))

			host.CompleteHttp()
		})

		// 测试非 HTML 请求的请求头处理
		t.Run("non-html request headers", func(t *testing.T) {
			host, status := test.NewTestHost(basicGrayConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/data"},
				{":method", "GET"},
				{"cookie", "userid=00000001"},
			})

			require.Equal(t, types.ActionContinue, action)

			// 非 HTML 请求也应该添加版本标签头
			requestHeaders := host.GetRequestHeaders()
			require.True(t, test.HasHeader(requestHeaders, "x-higress-tag"))

			host.CompleteHttp()
		})
	})
}

func TestOnHttpResponseHeader(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试基本灰度响应头处理
		t.Run("basic gray response headers", func(t *testing.T) {
			host, status := test.NewTestHost(basicGrayConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先处理请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/"},
				{":method", "GET"},
				{"cookie", "userid=00000001"},
			})

			// 处理响应头
			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "text/html"},
			})

			require.Equal(t, types.ActionContinue, action)

			// 验证是否添加了 Set-Cookie 头
			responseHeaders := host.GetResponseHeaders()
			require.True(t, test.HasHeader(responseHeaders, "Set-Cookie"))

			host.CompleteHttp()
		})

		// 测试 404 状态码的响应头处理
		t.Run("404 status response headers", func(t *testing.T) {
			host, status := test.NewTestHost(basicGrayConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先处理请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/"},
				{":method", "GET"},
				{"cookie", "userid=00000001"},
			})

			// 处理响应头
			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "404"},
				{"content-type", "text/plain"},
			})

			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})

		// 上游 404 响应可能不携带 content-type，回退页面仍应正常生效。
		t.Run("404 html fallback without content type", func(t *testing.T) {
			host, status := test.NewTestHost(htmlFallbackConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/"},
				{":method", "GET"},
				{"cookie", "userid=00000001"},
			})

			var action types.Action
			require.NotPanics(t, func() {
				action = host.CallOnHttpResponseHeaders([][2]string{
					{":status", "404"},
				})
			})
			require.Equal(t, types.ActionContinue, action)

			responseHeaders := host.GetResponseHeaders()
			require.True(t, test.HasHeaderWithValue(responseHeaders, ":status", "200"))
			require.True(t, test.HasHeaderWithValue(responseHeaders, "content-type", "text/html"))

			host.CompleteHttp()
		})

		// 测试非首页请求的响应头处理
		t.Run("non-index response headers", func(t *testing.T) {
			host, status := test.NewTestHost(basicGrayConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先处理请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/data"},
				{":method", "GET"},
				{"cookie", "userid=00000001"},
			})

			// 处理响应头
			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})

			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})
	})
}

func TestOnHttpResponseBody(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试基本灰度响应体处理
		t.Run("basic gray response body", func(t *testing.T) {
			host, status := test.NewTestHost(basicGrayConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先处理请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/"},
				{":method", "GET"},
				{"cookie", "userid=00000001"},
			})

			// 处理响应头
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "text/html"},
			})

			// 处理响应体
			htmlBody := "<html><head><title>Test</title></head><body><h1>Hello World</h1></body></html>"
			action := host.CallOnHttpResponseBody([]byte(htmlBody))

			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})

		// 测试带注入的响应体处理
		t.Run("injection response body", func(t *testing.T) {
			host, status := test.NewTestHost(injectionConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先处理请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/"},
				{":method", "GET"},
				{"cookie", "userid=00000001"},
			})

			// 处理响应头
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "text/html"},
			})

			// 处理响应体
			htmlBody := "<html><head><title>Test</title></head><body><h1>Hello World</h1></body></html>"
			action := host.CallOnHttpResponseBody([]byte(htmlBody))

			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})

		// 测试非 HTML 请求的响应体处理
		t.Run("non-html response body", func(t *testing.T) {
			host, status := test.NewTestHost(basicGrayConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先处理请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/data"},
				{":method", "GET"},
				{"cookie", "userid=00000001"},
			})

			// 处理响应头
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})

			// 处理响应体
			jsonBody := `{"message": "Hello World"}`
			action := host.CallOnHttpResponseBody([]byte(jsonBody))

			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})
	})
}
