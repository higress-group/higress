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

// TestOnHttpRequestBody 验证请求体阶段的重放检查逻辑。
//
// 这是 issue #4601 的核心回归测试：带请求体的请求（如 POST）必须在
// 请求体阶段发起 Redis SETNX 并挂起请求等待回调，而非在请求头阶段
// 挂起（请求头阶段的 ActionPause 挂不住后续 body 阶段，会导致回调
// 来晚、重放漏放）。
func TestOnHttpRequestBody(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 带请求体的首次请求：header 阶段暂存 nonce，body 阶段发 SETNX，
		// Redis 返回 OK（写入成功）→ 放行。
		t.Run("POST with body - first request allowed", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 1. 请求头阶段：有 body，暂存 nonce，返回 ActionPause
			headerAction := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "POST"},
				{"content-type", "application/json"},
				{"X-Higress-Nonce", "dGVzdC1ub25jZS12YWx1ZQ=="},
			})
			require.Equal(t, types.ActionPause, headerAction)

			// 2. 请求体阶段：发起 SETNX，返回 ActionPause 等待回调
			bodyAction := host.CallOnHttpRequestBody([]byte(`{"key":"value"}`))
			require.Equal(t, types.ActionPause, bodyAction)

			// 3. 模拟 Redis SETNX 成功（首次写入）→ ResumeHttpRequest 放行
			host.CallOnRedisCall(0, test.CreateRedisRespString("OK"))

			host.CompleteHttp()
		})

		// 带请求体的重放请求：body 阶段发 SETNX，Redis 返回 nil
		//（key 已存在，写入失败）→ 应返回 429。
		t.Run("POST with body - replay request rejected", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 1. 请求头阶段
			headerAction := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "POST"},
				{"content-type", "application/json"},
				{"X-Higress-Nonce", "dGVzdC1ub25jZS12YWx1ZQ=="},
			})
			require.Equal(t, types.ActionPause, headerAction)

			// 2. 请求体阶段：发起 SETNX，返回 ActionPause 等待回调
			bodyAction := host.CallOnHttpRequestBody([]byte(`{"key":"value"}`))
			require.Equal(t, types.ActionPause, bodyAction)

			// 3. 模拟 Redis SETNX 失败（key 已存在，重放）→ SendHttpResponse(429)
			host.CallOnRedisCall(0, test.CreateRedisRespNull())

			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse, "replay request should be rejected with 429")
			require.Equal(t, uint32(429), localResponse.StatusCode)
			require.Equal(t, "Replay Attack Detected", string(localResponse.Data))

			host.CompleteHttp()
		})

		// 带请求体的请求，Redis 返回错误 → fail-open 放行
		t.Run("POST with body - redis error fail-open", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "POST"},
				{"content-type", "application/json"},
				{"X-Higress-Nonce", "dGVzdC1ub25jZS12YWx1ZQ=="},
			})

			bodyAction := host.CallOnHttpRequestBody([]byte(`{"key":"value"}`))
			require.Equal(t, types.ActionPause, bodyAction)

			// Redis 返回错误 → ResumeHttpRequest 放行（fail-open）
			host.CallOnRedisCall(0, test.CreateRedisRespError("connection refused"))

			host.CompleteHttp()
		})

		// 带请求体但无 nonce（非强制模式）→ 两阶段都直接放行
		t.Run("POST with body - no nonce non-force mode", func(t *testing.T) {
			host, status := test.NewTestHost(customConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			headerAction := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})
			require.Equal(t, types.ActionContinue, headerAction)

			// body 阶段：无 nonce 暂存，直接放行
			bodyAction := host.CallOnHttpRequestBody([]byte(`{"key":"value"}`))
			require.Equal(t, types.ActionContinue, bodyAction)

			host.CompleteHttp()
		})
	})
}

// TestCompleteFlowWithBody 验证带请求体的完整请求流程：
// header 阶段暂存 nonce → body 阶段发 SETNX → Redis 回调判决。
// 这直接覆盖 issue #4601 报告的 POST 重放漏放场景。
func TestCompleteFlowWithBody(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("POST complete flow - first then replay", func(t *testing.T) {
			nonce := "dGVzdC1ub25jZS12YWx1ZQ=="

			// --- 第一次请求（首次，应放行）---
			host, status := test.NewTestHost(basicConfig)
			require.Equal(t, types.OnPluginStartStatusOK, status)

			headerAction := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "POST"},
				{"content-type", "application/json"},
				{"X-Higress-Nonce", nonce},
			})
			require.Equal(t, types.ActionPause, headerAction)

			bodyAction := host.CallOnHttpRequestBody([]byte(`{"key":"value"}`))
			require.Equal(t, types.ActionPause, bodyAction)

			// SETNX 成功 → 放行
			host.CallOnRedisCall(0, test.CreateRedisRespString("OK"))
			host.CompleteHttp()
			host.Reset()

			// --- 第二次请求（重放，应 429）---
			host2, status2 := test.NewTestHost(basicConfig)
			defer host2.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status2)

			headerAction2 := host2.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "POST"},
				{"content-type", "application/json"},
				{"X-Higress-Nonce", nonce},
			})
			require.Equal(t, types.ActionPause, headerAction2)

			bodyAction2 := host2.CallOnHttpRequestBody([]byte(`{"key":"value"}`))
			require.Equal(t, types.ActionPause, bodyAction2)

			// SETNX 失败（key 已存在）→ 429
			host2.CallOnRedisCall(0, test.CreateRedisRespNull())

			localResponse := host2.GetLocalResponse()
			require.NotNil(t, localResponse, "replay POST should be rejected")
			require.Equal(t, uint32(429), localResponse.StatusCode)
			require.Equal(t, "Replay Attack Detected", string(localResponse.Data))

			host2.CompleteHttp()
		})
	})
}
