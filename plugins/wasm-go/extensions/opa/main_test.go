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
	"net/http"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

// 测试配置：基本配置
var basicConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"policy":        "example1",
		"timeout":       "5s",
		"serviceSource": "k8s",
		"serviceName":   "opa",
		"servicePort":   "8181",
		"namespace":     "higress-backend",
	})
	return data
}()

// 测试配置：IP 服务配置
var ipConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"policy":        "example2",
		"timeout":       "3s",
		"serviceSource": "ip",
		"host":          "192.168.1.100",
		"servicePort":   "8181",
	})
	return data
}()

// 测试配置：Nacos 服务配置
var nacosConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"policy":        "example3",
		"timeout":       "10s",
		"serviceSource": "nacos",
		"serviceName":   "opa-service",
		"servicePort":   "8181",
		"namespace":     "public",
	})
	return data
}()

// 测试配置：Route 服务配置
var routeConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"policy":        "example4",
		"timeout":       "2s",
		"serviceSource": "route",
		"host":          "example.com",
	})
	return data
}()

// 测试配置：无效配置（缺少 policy）
var invalidConfigMissingPolicy = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"timeout":       "5s",
		"serviceSource": "k8s",
		"serviceName":   "opa",
		"servicePort":   "8181",
	})
	return data
}()

// 测试配置：无效配置（缺少 timeout）
var invalidConfigMissingTimeout = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"policy":        "example1",
		"serviceSource": "k8s",
		"serviceName":   "opa",
		"servicePort":   "8181",
	})
	return data
}()

// 测试配置：无效配置（无效的 timeout 格式）
var invalidConfigInvalidTimeout = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"policy":        "example1",
		"timeout":       "invalid-timeout",
		"serviceSource": "k8s",
		"serviceName":   "opa",
		"servicePort":   "8181",
	})
	return data
}()

var synchronousCallErrorConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"policy":        "%zz",
		"timeout":       "5s",
		"serviceSource": "k8s",
		"serviceName":   "opa",
		"servicePort":   "8181",
		"namespace":     "higress-backend",
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
		})

		// 测试 IP 服务配置解析
		t.Run("ip service config", func(t *testing.T) {
			host, status := test.NewTestHost(ipConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试 Nacos 服务配置解析
		t.Run("nacos service config", func(t *testing.T) {
			host, status := test.NewTestHost(nacosConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试 Route 服务配置解析
		t.Run("route service config", func(t *testing.T) {
			host, status := test.NewTestHost(routeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试无效配置 - 缺少 policy
		t.Run("invalid config - missing policy", func(t *testing.T) {
			host, status := test.NewTestHost(invalidConfigMissingPolicy)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})

		// 测试无效配置 - 缺少 timeout
		t.Run("invalid config - missing timeout", func(t *testing.T) {
			host, status := test.NewTestHost(invalidConfigMissingTimeout)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})

		// 测试无效配置 - 无效的 timeout 格式
		t.Run("invalid config - invalid timeout format", func(t *testing.T) {
			host, status := test.NewTestHost(invalidConfigInvalidTimeout)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})
	})
}

func TestOnHttpRequestHeaders(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("synchronous opa call error fails closed", func(t *testing.T) {
			host, status := test.NewTestHost(synchronousCallErrorConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "GET"},
				{"Content-Type", "application/json"},
			})

			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			response := host.GetLocalResponse()
			require.NotNil(t, response)
			require.Equal(t, uint32(http.StatusServiceUnavailable), response.StatusCode)
			require.Equal(t, "opa.dispatch_fail", response.StatusCodeDetail)
			require.Equal(t, "opa dispatch failed", string(response.Data))

			host.CompleteHttp()
		})

		// 测试基本请求头处理
		t.Run("basic request headers", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "GET"},
				{"Content-Type", "application/json"},
			})

			// 由于 OPA 调用是异步的，这里会返回 HeaderStopAllIterationAndWatermark
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			// 模拟外部 OPA 服务的 HTTP 调用响应
			// 模拟成功响应 - 允许访问
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(`{"result": true}`))

			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())
			host.CompleteHttp()
		})

		// 测试 OPA 服务拒绝访问
		t.Run("opa service denies access", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			// 由于 OPA 调用是异步的，这里会返回 HeaderStopAllIterationAndWatermark
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			// 模拟外部 OPA 服务的 HTTP 调用响应
			// 模拟成功响应 - 拒绝访问
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(`{"result": false}`))

			response := host.GetLocalResponse()
			require.Equal(t, uint32(http.StatusUnauthorized), response.StatusCode)
			require.Equal(t, "opa.server_not_allowed", response.StatusCodeDetail)
			require.Equal(t, "opa server not allowed", string(response.Data))
			host.CompleteHttp()
		})

		// 测试 OPA 服务返回非 200 状态码
		t.Run("opa service returns non-200 status", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "GET"},
				{"Content-Type", "application/json"},
			})

			// 由于 OPA 调用是异步的，这里会返回 HeaderStopAllIterationAndWatermark
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			// 模拟外部 OPA 服务的 HTTP 调用响应
			// 模拟 500 错误响应
			host.CallOnHttpCall([][2]string{
				{":status", "500"},
				{"content-type", "application/json"},
			}, []byte(`{"error": "internal error"}`))

			response := host.GetLocalResponse()
			require.Equal(t, uint32(http.StatusInternalServerError), response.StatusCode)
			require.Equal(t, "opa.status_ne_200", response.StatusCodeDetail)
			require.Equal(t, "opa state not is 200", string(response.Data))
			host.CompleteHttp()
		})

		// 测试 OPA 服务返回无效响应
		t.Run("opa service returns invalid response", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "GET"},
				{"Content-Type", "application/json"},
			})

			// 由于 OPA 调用是异步的，这里会返回 HeaderStopAllIterationAndWatermark
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			// 模拟外部 OPA 服务的 HTTP 调用响应
			// 模拟无效 JSON 响应
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(`invalid json`))

			response := host.GetLocalResponse()
			require.Equal(t, uint32(http.StatusInternalServerError), response.StatusCode)
			require.Equal(t, "opa.bad_response_body", response.StatusCodeDetail)
			host.CompleteHttp()
		})

		// 测试 OPA 服务返回缺少 result 字段的响应
		t.Run("opa service returns response without result field", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "GET"},
				{"Content-Type", "application/json"},
			})

			// 由于 OPA 调用是异步的，这里会返回 HeaderStopAllIterationAndWatermark
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			// 模拟外部 OPA 服务的 HTTP 调用响应
			// 模拟缺少 result 字段的响应
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(`{"status": "ok"}`))

			response := host.GetLocalResponse()
			require.Equal(t, uint32(http.StatusInternalServerError), response.StatusCode)
			require.Equal(t, "opa.conversion_fail", response.StatusCodeDetail)
			require.Equal(t, "rsp type conversion fail", string(response.Data))
			host.CompleteHttp()
		})
	})
}

func TestOnHttpRequestBody(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试带请求体的请求处理
		t.Run("request with body", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先处理请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			// 处理请求体
			requestBody := []byte(`{"key": "value", "data": "test"}`)
			action = host.CallOnHttpRequestBody(requestBody)

			// 由于 OPA 调用是异步的，这里会返回 ActionPause
			require.Equal(t, types.ActionPause, action)

			// 模拟外部 OPA 服务的 HTTP 调用响应
			// 模拟成功响应 - 允许访问
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(`{"result": true}`))

			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())
			host.CompleteHttp()
		})
	})
}
