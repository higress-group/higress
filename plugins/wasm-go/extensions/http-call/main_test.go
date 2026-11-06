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

// 测试k8s服务源配置
var k8sTestConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"bodyHeader":    "x-response-body",
		"tokenHeader":   "x-auth-token",
		"requestPath":   "/api/auth",
		"serviceSource": "k8s",
		"serviceName":   "auth-service",
		"servicePort":   8080,
		"namespace":     "default",
	})
	return data
}()

// 测试nacos服务源配置
var nacosTestConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"bodyHeader":    "x-response-body",
		"tokenHeader":   "x-auth-token",
		"requestPath":   "/api/auth",
		"serviceSource": "nacos",
		"serviceName":   "auth-service",
		"servicePort":   8080,
		"namespace":     "public",
	})
	return data
}()

// 测试ip服务源配置
var ipTestConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"bodyHeader":    "x-response-body",
		"tokenHeader":   "x-auth-token",
		"requestPath":   "/api/auth",
		"serviceSource": "ip",
		"serviceName":   "auth-service",
		"servicePort":   8080,
	})
	return data
}()

// 测试dns服务源配置
var dnsTestConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"bodyHeader":    "x-response-body",
		"tokenHeader":   "x-auth-token",
		"requestPath":   "/api/auth",
		"serviceSource": "dns",
		"serviceName":   "auth-service",
		"servicePort":   8080,
		"domain":        "auth.example.com",
	})
	return data
}()

// 测试缺少bodyHeader的配置
var missingBodyHeaderConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"tokenHeader":   "x-auth-token",
		"requestPath":   "/api/auth",
		"serviceSource": "k8s",
		"serviceName":   "auth-service",
		"servicePort":   8080,
		"namespace":     "default",
	})
	return data
}()

// 测试缺少tokenHeader的配置
var missingTokenHeaderConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"bodyHeader":    "x-response-body",
		"requestPath":   "/api/auth",
		"serviceSource": "k8s",
		"serviceName":   "auth-service",
		"servicePort":   8080,
		"namespace":     "default",
	})
	return data
}()

// 测试缺少requestPath的配置
var missingRequestPathConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"bodyHeader":    "x-response-body",
		"tokenHeader":   "x-auth-token",
		"serviceSource": "k8s",
		"serviceName":   "auth-service",
		"servicePort":   8080,
		"namespace":     "default",
	})
	return data
}()

// 测试无效服务源的配置
var invalidServiceSourceConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"bodyHeader":    "x-response-body",
		"tokenHeader":   "x-auth-token",
		"requestPath":   "/api/auth",
		"serviceSource": "invalid",
		"serviceName":   "auth-service",
		"servicePort":   8080,
		"namespace":     "default",
	})
	return data
}()

func TestParseConfig(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// 测试k8s服务源配置
		t.Run("k8s service source", func(t *testing.T) {
			host, status := test.NewTestHost(k8sTestConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)
			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			httpCallConfig := config.(*HttpCallConfig)
			require.Equal(t, "x-response-body", httpCallConfig.bodyHeader)
			require.Equal(t, "x-auth-token", httpCallConfig.tokenHeader)
			require.Equal(t, "/api/auth", httpCallConfig.requestPath)
			require.NotNil(t, httpCallConfig.client)
		})

		// 测试nacos服务源配置
		t.Run("nacos service source", func(t *testing.T) {
			host, status := test.NewTestHost(nacosTestConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)
			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			httpCallConfig := config.(*HttpCallConfig)
			require.Equal(t, "x-response-body", httpCallConfig.bodyHeader)
			require.Equal(t, "x-auth-token", httpCallConfig.tokenHeader)
			require.Equal(t, "/api/auth", httpCallConfig.requestPath)
			require.NotNil(t, httpCallConfig.client)
		})

		// 测试ip服务源配置
		t.Run("ip service source", func(t *testing.T) {
			host, status := test.NewTestHost(ipTestConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)
			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			httpCallConfig := config.(*HttpCallConfig)
			require.Equal(t, "x-response-body", httpCallConfig.bodyHeader)
			require.Equal(t, "x-auth-token", httpCallConfig.tokenHeader)
			require.Equal(t, "/api/auth", httpCallConfig.requestPath)
			require.NotNil(t, httpCallConfig.client)
		})

		// 测试dns服务源配置
		t.Run("dns service source", func(t *testing.T) {
			host, status := test.NewTestHost(dnsTestConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)
			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			httpCallConfig := config.(*HttpCallConfig)
			require.Equal(t, "x-response-body", httpCallConfig.bodyHeader)
			require.Equal(t, "x-auth-token", httpCallConfig.tokenHeader)
			require.Equal(t, "/api/auth", httpCallConfig.requestPath)
			require.NotNil(t, httpCallConfig.client)
		})

		// 测试缺少bodyHeader的配置
		t.Run("missing bodyHeader", func(t *testing.T) {
			host, status := test.NewTestHost(missingBodyHeaderConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
			config, err := host.GetMatchConfig()
			require.NoError(t, err) // 框架不会返回错误，而是返回nil配置
			require.Nil(t, config)  // 配置解析失败时返回nil
		})

		// 测试缺少tokenHeader的配置
		t.Run("missing tokenHeader", func(t *testing.T) {
			host, status := test.NewTestHost(missingTokenHeaderConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
			config, err := host.GetMatchConfig()
			require.NoError(t, err) // 框架不会返回错误，而是返回nil配置
			require.Nil(t, config)  // 配置解析失败时返回nil
		})

		// 测试缺少requestPath的配置
		t.Run("missing requestPath", func(t *testing.T) {
			host, status := test.NewTestHost(missingRequestPathConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
			config, err := host.GetMatchConfig()
			require.NoError(t, err) // 框架不会返回错误，而是返回nil配置
			require.Nil(t, config)  // 配置解析失败时返回nil
		})

		// 测试无效服务源的配置
		t.Run("invalid service source", func(t *testing.T) {
			host, status := test.NewTestHost(invalidServiceSourceConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
			config, err := host.GetMatchConfig()
			require.NoError(t, err) // 框架不会返回错误，而是返回nil配置
			require.Nil(t, config)  // 配置解析失败时返回nil
		})
	})
}

func TestK8sOnHttpRequestHeaders(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 使用k8s配置进行测试
		host, status := test.NewTestHost(k8sTestConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		// 模拟HTTP请求头
		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"},
			{":path", "/api/test"},
			{":method", "GET"},
		})

		// 验证返回的action
		require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

		// 模拟外部服务的HTTP调用响应
		// 模拟成功响应
		host.CallOnHttpCall([][2]string{
			{":status", "200"},
			{"x-auth-token", "test-token-123"},
			{"content-type", "application/json"},
		}, []byte(`{"message": "success", "data": "test-data"}`))

		// 验证请求头是否正确设置
		requestHeaders := host.GetRequestHeaders()

		// 查找bodyHeader
		bodyHeaderFound := false
		tokenHeaderFound := false

		for _, header := range requestHeaders {
			if header[0] == "x-response-body" {
				bodyHeaderFound = true
				// 验证响应体内容（换行符被替换为#）
				expectedBody := `{"message": "success", "data": "test-data"}`
				require.Equal(t, expectedBody, header[1])
			}
			if header[0] == "x-auth-token" {
				tokenHeaderFound = true
				require.Equal(t, "test-token-123", header[1])
			}
		}

		require.True(t, bodyHeaderFound, "bodyHeader should be set")
		require.True(t, tokenHeaderFound, "tokenHeader should be set")
		require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())
		host.CompleteHttp()
	})
}

func TestK8sOnHttpRequestHeadersWithError(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 使用k8s配置进行测试
		host, status := test.NewTestHost(k8sTestConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		// 模拟HTTP请求头
		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"},
			{":path", "/api/test"},
			{":method", "GET"},
		})

		// 验证返回的action
		require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

		// 模拟外部服务返回错误状态码
		host.CallOnHttpCall([][2]string{
			{":status", "500"},
			{"content-type", "application/json"},
		}, []byte(`{"error": "internal server error"}`))

		// 验证请求头不应该被设置（因为状态码不是200）
		requestHeaders := host.GetRequestHeaders()

		bodyHeaderFound := false
		tokenHeaderFound := false

		for _, header := range requestHeaders {
			if header[0] == "x-response-body" {
				bodyHeaderFound = true
			}
			if header[0] == "x-auth-token" {
				tokenHeaderFound = true
			}
		}

		require.False(t, bodyHeaderFound, "bodyHeader should not be set when status code is not 200")
		require.False(t, tokenHeaderFound, "tokenHeader should not be set when status code is not 200")
		require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())
		host.CompleteHttp()
	})
}

func TestK8sOnHttpRequestHeadersWithNewlines(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 使用k8s配置进行测试
		host, status := test.NewTestHost(k8sTestConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		// 模拟HTTP请求头
		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"},
			{":path", "/api/test"},
			{":method", "GET"},
		})

		// 验证返回的action
		require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

		// 模拟外部服务响应包含换行符
		responseBody := `{"message": "success",
"data": "test-data",
"description": "multi-line response"}`

		host.CallOnHttpCall([][2]string{
			{":status", "200"},
			{"x-auth-token", "test-token-456"},
			{"content-type", "application/json"},
		}, []byte(responseBody))

		// 验证请求头是否正确设置，换行符应该被替换为#
		requestHeaders := host.GetRequestHeaders()

		bodyHeaderFound := false
		expectedBody := `{"message": "success",#"data": "test-data",#"description": "multi-line response"}`

		for _, header := range requestHeaders {
			if header[0] == "x-response-body" {
				bodyHeaderFound = true
				require.Equal(t, expectedBody, header[1])
			}
		}

		require.True(t, bodyHeaderFound, "bodyHeader should be set with newlines replaced by #")
		require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())
		host.CompleteHttp()
	})
}

// A dispatch that fails synchronously (here: a requestPath that makes the
// wrapper's URL parsing fail) never invokes the callback, so the request
// must not be left paused at the watermark.
func TestSyncDispatchFailureDoesNotHang(t *testing.T) {
	syncFailConfig := func() json.RawMessage {
		data, _ := json.Marshal(map[string]interface{}{
			"bodyHeader":  "x-response-body",
			"tokenHeader": "x-auth-token",
			// %zz is not a valid percent-encoding, so constructing the
			// request URL fails before DispatchHttpCall is issued.
			"requestPath":   "/%zz",
			"serviceSource": "k8s",
			"serviceName":   "auth-service",
			"servicePort":   8080,
			"namespace":     "default",
		})
		return data
	}

	test.RunTest(t, func(t *testing.T) {
		t.Run("sync dispatch failure continues the request", func(t *testing.T) {
			host, status := test.NewTestHost(syncFailConfig())
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "test.com"},
				{":path", "/api/test"},
				{":method", "GET"},
			})

			// Must not be HeaderStopAllIterationAndWatermark: the callback
			// will never fire, so nothing would ever resume the request.
			require.Equal(t, types.ActionContinue, action)

			// No headers should have been added by the failed call.
			for _, header := range host.GetRequestHeaders() {
				require.NotEqual(t, "x-response-body", header[0])
				require.NotEqual(t, "x-auth-token", header[0])
			}
		})
	})
}
