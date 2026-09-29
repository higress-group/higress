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
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

// 测试配置：基本 envoy 模式配置
var basicEnvoyConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"http_service": map[string]interface{}{
			"endpoint_mode": "envoy",
			"endpoint": map[string]interface{}{
				"service_name": "ext-auth.backend.svc.cluster.local",
				"service_port": 8090,
				"path_prefix":  "/auth",
			},
			"timeout": 1000,
		},
	})
	return data
}()

// 测试配置：forward_auth 模式配置
var forwardAuthConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"http_service": map[string]interface{}{
			"endpoint_mode": "forward_auth",
			"endpoint": map[string]interface{}{
				"service_name":   "ext-auth.backend.svc.cluster.local",
				"service_port":   8090,
				"path":           "/auth",
				"request_method": "POST",
			},
			"timeout": 1000,
		},
	})
	return data
}()

// 测试配置：带请求头过滤的配置
var headersConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"http_service": map[string]interface{}{
			"endpoint_mode": "envoy",
			"endpoint": map[string]interface{}{
				"service_name": "ext-auth.backend.svc.cluster.local",
				"service_port": 8090,
				"path_prefix":  "/auth",
			},
			"timeout": 1000,
			"authorization_request": map[string]interface{}{
				"allowed_headers": []map[string]interface{}{
					{"exact": "x-auth-version"},
					{"prefix": "x-custom"},
				},
				"headers_to_add": map[string]interface{}{
					"x-envoy-header": "true",
				},
			},
			"authorization_response": map[string]interface{}{
				"allowed_upstream_headers": []map[string]interface{}{
					{"exact": "x-user-id"},
					{"exact": "x-auth-version"},
				},
				"allowed_client_headers": []map[string]interface{}{
					{"exact": "x-auth-failed"},
				},
			},
		},
	})
	return data
}()

// 测试配置：带请求体的配置
var withRequestBodyConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"http_service": map[string]interface{}{
			"endpoint_mode": "envoy",
			"endpoint": map[string]interface{}{
				"service_name": "ext-auth.backend.svc.cluster.local",
				"service_port": 8090,
				"path_prefix":  "/auth",
			},
			"timeout": 1000,
			"authorization_request": map[string]interface{}{
				"with_request_body":      true,
				"max_request_body_bytes": 1024,
			},
		},
	})
	return data
}()

// 测试配置：带黑白名单的配置
var matchRulesConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"http_service": map[string]interface{}{
			"endpoint_mode": "envoy",
			"endpoint": map[string]interface{}{
				"service_name": "ext-auth.backend.svc.cluster.local",
				"service_port": 8090,
				"path_prefix":  "/auth",
			},
			"timeout": 1000,
		},
		"match_type": "whitelist",
		"match_list": []map[string]interface{}{
			{
				"match_rule_domain": "api.example.com",
				"match_rule_path":   "/public",
				"match_rule_type":   "prefix",
			},
			{
				"match_rule_method": []string{"GET"},
				"match_rule_path":   "/health",
				"match_rule_type":   "exact",
			},
		},
	})
	return data
}()

// 测试配置：失败模式配置
var failureModeConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"http_service": map[string]interface{}{
			"endpoint_mode": "envoy",
			"endpoint": map[string]interface{}{
				"service_name": "ext-auth.backend.svc.cluster.local",
				"service_port": 8090,
				"path_prefix":  "/auth",
			},
			"timeout": 1000,
		},
		"failure_mode_allow":            true,
		"failure_mode_allow_header_add": true,
		"status_on_error":               500,
	})
	return data
}()

func TestParseConfig(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// 测试基本 envoy 模式配置解析
		t.Run("basic envoy config", func(t *testing.T) {
			host, status := test.NewTestHost(basicEnvoyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试 forward_auth 模式配置解析
		t.Run("forward auth config", func(t *testing.T) {
			host, status := test.NewTestHost(forwardAuthConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试带请求头过滤的配置解析
		t.Run("headers config", func(t *testing.T) {
			host, status := test.NewTestHost(headersConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试带请求体的配置解析
		t.Run("with request body config", func(t *testing.T) {
			host, status := test.NewTestHost(withRequestBodyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试带黑白名单的配置解析
		t.Run("match rules config", func(t *testing.T) {
			host, status := test.NewTestHost(matchRulesConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试失败模式配置解析
		t.Run("failure mode config", func(t *testing.T) {
			host, status := test.NewTestHost(failureModeConfig)
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
		// 测试基本 envoy 模式请求头处理
		t.Run("basic envoy request headers", func(t *testing.T) {
			host, status := test.NewTestHost(basicEnvoyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/users"},
				{":method", "POST"},
				{"authorization", "Bearer token123"},
				{"x-custom-header", "value"},
			})

			// 由于需要调用外部认证服务，应该返回 HeaderStopAllIterationAndWatermark
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			// 模拟外部认证服务的HTTP调用响应
			// 模拟成功响应（200状态码）
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"x-user-id", "user123"},
				{"x-auth-version", "1.0"},
				{"content-type", "application/json"},
			}, []byte(`{"authorized": true, "user": "user123"}`))

			// 验证请求是否被恢复
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			host.CompleteHttp()
		})

		// 测试 forward_auth 模式请求头处理
		t.Run("forward auth request headers", func(t *testing.T) {
			host, status := test.NewTestHost(forwardAuthConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/users"},
				{":method", "GET"},
				{"authorization", "Bearer token123"},
				{"x-custom-header", "value"},
			})

			// 由于需要调用外部认证服务，应该返回 HeaderStopAllIterationAndWatermark
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			// 模拟外部认证服务的HTTP调用响应
			// 模拟成功响应（200状态码）
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"x-user-id", "user456"},
				{"x-auth-version", "1.0"},
				{"content-type", "application/json"},
			}, []byte(`{"authorized": true, "user": "user456"}`))

			// 验证请求是否被恢复
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			host.CompleteHttp()
		})

		// 测试带请求头过滤的请求头处理
		t.Run("headers filtered request headers", func(t *testing.T) {
			host, status := test.NewTestHost(headersConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/users"},
				{":method", "POST"},
				{"authorization", "Bearer token123"},
				{"x-auth-version", "1.0"},
				{"x-custom-header", "value"},
				{"x-ignored-header", "ignored"},
			})

			// 由于需要调用外部认证服务，应该返回 HeaderStopAllIterationAndWatermark
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			host.CompleteHttp()
		})

		// 测试带请求体的请求头处理
		t.Run("with request body request headers", func(t *testing.T) {
			host, status := test.NewTestHost(withRequestBodyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/users"},
				{":method", "POST"},
				{"authorization", "Bearer token123"},
				{"content-type", "application/json"},
			})

			// 由于需要读取请求体，应该返回 HeaderStopIteration
			require.Equal(t, types.HeaderStopIteration, action)

			host.CompleteHttp()
		})

		// 测试黑白名单匹配的请求头处理
		t.Run("match rules request headers", func(t *testing.T) {
			host, status := test.NewTestHost(matchRulesConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 测试白名单匹配的请求（应该跳过认证）
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "api.example.com"},
				{":path", "/public/users"},
				{":method", "GET"},
			})

			// 白名单匹配的请求应该直接通过
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})

		// 测试黑白名单不匹配的请求头处理
		t.Run("match rules no match request headers", func(t *testing.T) {
			host, status := test.NewTestHost(matchRulesConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 测试不在白名单中的请求（应该进行认证）
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "api.example.com"},
				{":path", "/private/users"},
				{":method", "POST"},
			})

			// 不在白名单中的请求应该进行认证
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			// 模拟外部认证服务的HTTP调用响应
			// 模拟认证失败响应（401状态码）
			host.CallOnHttpCall([][2]string{
				{":status", "401"},
				{"x-auth-failed", "true"},
				{"content-type", "application/json"},
			}, []byte(`{"authorized": false, "message": "Invalid token"}`))

			host.CompleteHttp()
		})

		// 测试认证失败的情况
		t.Run("authentication failed", func(t *testing.T) {
			host, status := test.NewTestHost(basicEnvoyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/users"},
				{":method", "POST"},
				{"authorization", "Bearer invalid-token"},
			})

			// 由于需要调用外部认证服务，应该返回 HeaderStopAllIterationAndWatermark
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			// 模拟外部认证服务的HTTP调用响应
			// 模拟认证失败响应（403状态码）
			host.CallOnHttpCall([][2]string{
				{":status", "403"},
				{"x-auth-failed", "true"},
				{"content-type", "application/json"},
			}, []byte(`{"authorized": false, "message": "Access denied"}`))

			host.CompleteHttp()
		})

		// 测试认证服务返回5xx错误的情况
		t.Run("authentication service error", func(t *testing.T) {
			host, status := test.NewTestHost(basicEnvoyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/users"},
				{":method", "POST"},
				{"authorization", "Bearer token123"},
			})

			// 由于需要调用外部认证服务，应该返回 HeaderStopAllIterationAndWatermark
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			// 模拟外部认证服务的HTTP调用响应
			// 模拟服务错误响应（500状态码）
			host.CallOnHttpCall([][2]string{
				{":status", "500"},
				{"x-auth-error", "true"},
				{"content-type", "application/json"},
			}, []byte(`{"error": "Internal server error"}`))

			host.CompleteHttp()
		})

		// 测试失败模式允许的情况
		t.Run("failure mode allow", func(t *testing.T) {
			host, status := test.NewTestHost(failureModeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/users"},
				{":method", "POST"},
				{"authorization", "Bearer token123"},
			})

			// 由于需要调用外部认证服务，应该返回 HeaderStopAllIterationAndWatermark
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			// 模拟外部认证服务的HTTP调用响应
			// 模拟服务错误响应（500状态码），但由于配置了失败模式允许，请求应该通过
			host.CallOnHttpCall([][2]string{
				{":status", "500"},
				{"x-auth-error", "true"},
				{"content-type", "application/json"},
			}, []byte(`{"error": "Internal server error"}`))

			// 验证请求是否被恢复（失败模式允许的情况下）
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			host.CompleteHttp()
		})
	})
}

func TestOnHttpRequestBody(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试带请求体的请求体处理
		t.Run("with request body", func(t *testing.T) {
			host, status := test.NewTestHost(withRequestBodyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先处理请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/users"},
				{":method", "POST"},
				{"authorization", "Bearer token123"},
				{"content-type", "application/json"},
			})

			// 处理请求体
			requestBody := `{"username": "test", "password": "password123"}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			// 由于需要调用外部认证服务，应该返回 DataStopIterationAndBuffer
			require.Equal(t, types.DataStopIterationAndBuffer, action)

			host.CompleteHttp()
		})

		// 测试不带请求体的请求体处理
		t.Run("without request body", func(t *testing.T) {
			host, status := test.NewTestHost(basicEnvoyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先处理请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/users"},
				{":method", "POST"},
				{"authorization", "Bearer token123"},
			})

			// 处理请求体
			requestBody := `{"username": "test", "password": "password123"}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			// 不带请求体配置的请求应该直接通过
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})
	})
}
