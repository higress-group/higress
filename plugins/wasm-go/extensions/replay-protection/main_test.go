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
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

// 测试配置：基本配置
var basicConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"redis": map[string]interface{}{
			"service_name": "redis.static",
			"service_port": 80,
		},
		"force_nonce":      true,
		"nonce_header":     "X-Higress-Nonce",
		"nonce_ttl":        900,
		"nonce_min_length": 8,
		"nonce_max_length": 128,
		"validate_base64":  true,
		"reject_code":      429,
		"reject_msg":       "Replay Attack Detected",
	})
	return data
}()

// 测试配置：自定义配置
var customConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"redis": map[string]interface{}{
			"service_name": "custom-redis.svc.cluster.local",
			"service_port": 6379,
			"username":     "admin",
			"password":     "password123",
			"timeout":      2000,
			"database":     1,
			"key_prefix":   "custom-prefix",
		},
		"force_nonce":      false,
		"nonce_header":     "X-Custom-Nonce",
		"nonce_ttl":        1800,
		"nonce_min_length": 16,
		"nonce_max_length": 64,
		"validate_base64":  false,
		"reject_code":      400,
		"reject_msg":       "Custom Reject Message",
	})
	return data
}()

// 测试配置：最小配置（使用默认值）
var minimalConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"redis": map[string]interface{}{
			"service_name": "redis.static",
		},
	})
	return data
}()

// 测试配置：无效配置（缺少 Redis 配置）
var invalidConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"force_nonce":  true,
		"nonce_header": "X-Higress-Nonce",
	})
	return data
}()

// 测试配置：无效配置（空的 Redis 服务名）
var invalidRedisConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"redis": map[string]interface{}{
			"service_name": "",
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
		})

		// 测试自定义配置解析
		t.Run("custom config", func(t *testing.T) {
			host, status := test.NewTestHost(customConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试最小配置解析（使用默认值）
		t.Run("minimal config", func(t *testing.T) {
			host, status := test.NewTestHost(minimalConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试无效配置（缺少 Redis 配置）
		t.Run("invalid config - missing redis", func(t *testing.T) {
			host, status := test.NewTestHost(invalidConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})

		// 测试无效配置（空的 Redis 服务名）
		t.Run("invalid config - empty redis service name", func(t *testing.T) {
			host, status := test.NewTestHost(invalidRedisConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})
	})
}

func TestParseConfigRejectsInvalidRanges(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		tests := []struct {
			name        string
			values      map[string]interface{}
			redisValues map[string]interface{}
		}{
			{name: "zero nonce ttl", values: map[string]interface{}{"nonce_ttl": 0}},
			{name: "negative nonce ttl", values: map[string]interface{}{"nonce_ttl": -1}},
			{name: "zero nonce min length", values: map[string]interface{}{"nonce_min_length": 0}},
			{name: "negative nonce min length", values: map[string]interface{}{"nonce_min_length": -1}},
			{name: "zero nonce max length", values: map[string]interface{}{"nonce_max_length": 0}},
			{name: "negative nonce max length", values: map[string]interface{}{"nonce_max_length": -1}},
			{
				name: "nonce min length exceeds max length",
				values: map[string]interface{}{
					"nonce_min_length": 129,
					"nonce_max_length": 128,
				},
			},
			{name: "reject code below HTTP range", values: map[string]interface{}{"reject_code": 99}},
			{name: "reject code above HTTP range", values: map[string]interface{}{"reject_code": 600}},
			{name: "zero redis timeout", redisValues: map[string]interface{}{"timeout": 0}},
			{name: "negative redis port", redisValues: map[string]interface{}{"service_port": -1}},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				redisConfig := map[string]interface{}{"service_name": "redis.static"}
				for key, value := range tt.redisValues {
					redisConfig[key] = value
				}
				config := map[string]interface{}{"redis": redisConfig}
				for key, value := range tt.values {
					config[key] = value
				}

				data, err := json.Marshal(config)
				require.NoError(t, err)
				host, status := test.NewTestHost(data)
				defer host.Reset()
				require.Equal(t, types.OnPluginStartStatusFailed, status)
			})
		}
	})
}

func TestOnHttpRequestHeaders(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试强制 nonce 模式 - 缺少 nonce 头
		t.Run("force nonce - missing nonce header", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "POST"},
			})

			require.Equal(t, types.ActionPause, action)

			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			require.Equal(t, uint32(400), localResponse.StatusCode)
			require.Equal(t, "Missing Required Header", string(localResponse.Data))

			host.CompleteHttp()
		})

		// 测试强制 nonce 模式 - 有效的 nonce
		t.Run("force nonce - valid nonce", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 模拟 Redis 客户端成功设置 nonce
			host.SetProperty([]string{"redis", "client", "mock"}, []byte("success"))

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "POST"},
				{"X-Higress-Nonce", "dGVzdC1ub25jZS12YWx1ZQ=="}, // base64 encoded "test-nonce-value"
			})

			// 由于 Redis 操作是异步的，这里会返回 ActionPause
			require.Equal(t, types.ActionPause, action)

			host.CompleteHttp()
		})

		// 测试非强制 nonce 模式 - 缺少 nonce 头（应该通过）
		t.Run("non-force nonce - missing nonce header", func(t *testing.T) {
			host, status := test.NewTestHost(customConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "GET"},
			})

			require.Equal(t, types.ActionContinue, action)

			localResponse := host.GetLocalResponse()
			require.Nil(t, localResponse, "Request should pass through when nonce is not required")

			host.CompleteHttp()
		})

		// 测试无效的 nonce 长度（太短）
		t.Run("invalid nonce - too short", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "POST"},
				{"X-Higress-Nonce", "short"}, // 长度只有 5，小于最小值 8
			})

			require.Equal(t, types.ActionPause, action)

			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			require.Equal(t, uint32(400), localResponse.StatusCode)
			require.Equal(t, "Invalid Nonce", string(localResponse.Data))

			host.CompleteHttp()
		})

		// 测试无效的 nonce 长度（太长）
		t.Run("invalid nonce - too long", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 创建一个超过最大长度的 nonce
			longNonce := "a"
			for i := 0; i < 130; i++ {
				longNonce += "a"
			}

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "POST"},
				{"X-Higress-Nonce", longNonce},
			})

			require.Equal(t, types.ActionPause, action)

			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			require.Equal(t, uint32(400), localResponse.StatusCode)
			require.Equal(t, "Invalid Nonce", string(localResponse.Data))

			host.CompleteHttp()
		})

		// 测试无效的 base64 格式（当启用验证时）
		t.Run("invalid nonce - invalid base64 format", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "POST"},
				{"X-Higress-Nonce", "invalid-base64!@#"}, // 包含无效字符
			})

			require.Equal(t, types.ActionPause, action)

			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			require.Equal(t, uint32(400), localResponse.StatusCode)
			require.Equal(t, "Invalid Nonce", string(localResponse.Data))

			host.CompleteHttp()
		})

		// 测试自定义 nonce 头名称
		t.Run("custom nonce header name", func(t *testing.T) {
			host, status := test.NewTestHost(customConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "POST"},
				{"X-Custom-Nonce", "dGVzdC1ub25jZS12YWx1ZQ=="}, // 使用自定义头名称
			})

			// 由于 Redis 操作是异步的，这里会返回 ActionPause
			require.Equal(t, types.ActionPause, action)

			host.CompleteHttp()
		})

		// 测试有效的 nonce（长度在范围内，格式正确）
		t.Run("valid nonce - correct format and length", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 模拟 Redis 客户端成功设置 nonce
			host.SetProperty([]string{"redis", "client", "mock"}, []byte("success"))

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "POST"},
				{"X-Higress-Nonce", "dGVzdC1ub25jZS12YWx1ZQ=="}, // base64 encoded "test-nonce-value"
			})

			// 由于 Redis 操作是异步的，这里会返回 ActionPause
			require.Equal(t, types.ActionPause, action)

			host.CompleteHttp()
		})
	})
}

func TestValidateNonce(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// 测试 nonce 长度验证
		t.Run("nonce length validation", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 测试太短的 nonce
			shortNonce := "short"
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "POST"},
				{"X-Higress-Nonce", shortNonce},
			})
			require.Equal(t, types.ActionPause, action)

			// 测试太长的 nonce
			longNonce := "a"
			for i := 0; i < 130; i++ {
				longNonce += "a"
			}
			action = host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "POST"},
				{"X-Higress-Nonce", longNonce},
			})
			require.Equal(t, types.ActionPause, action)

			// 测试长度在范围内的 nonce
			validNonce := "dGVzdC1ub25jZS12YWx1ZQ==" // base64 encoded "test-nonce-value"
			action = host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "POST"},
				{"X-Higress-Nonce", validNonce},
			})
			// 由于 Redis 操作是异步的，这里会返回 ActionPause
			require.Equal(t, types.ActionPause, action)
		})

		// 测试 base64 格式验证
		t.Run("base64 format validation", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 测试无效的 base64 格式
			invalidBase64 := "invalid-base64!@#"
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "POST"},
				{"X-Higress-Nonce", invalidBase64},
			})
			require.Equal(t, types.ActionPause, action)

			// 测试有效的 base64 格式
			validBase64 := "dGVzdC1ub25jZS12YWx1ZQ=="
			action = host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "POST"},
				{"X-Higress-Nonce", validBase64},
			})
			// 由于 Redis 操作是异步的，这里会返回 ActionPause
			require.Equal(t, types.ActionPause, action)
		})
	})
}

func TestCompleteFlow(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("complete request flow", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 1. 处理请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "POST"},
				{"X-Higress-Nonce", "dGVzdC1ub25jZS12YWx1ZQ=="},
			})

			// 由于 Redis 操作是异步的，这里会返回 ActionPause
			require.Equal(t, types.ActionPause, action)

			host.CompleteHttp()
		})
	})
}
