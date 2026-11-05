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
	"strings"
	"testing"

	"github.com/dgrijalva/jwt-go"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

// 生成测试用的有效 JWT token
func generateTestToken(secretKey string) string {
	token := jwt.New(jwt.SigningMethodHS256)
	claims := token.Claims.(jwt.MapClaims)
	claims["sub"] = "1234567890"
	claims["name"] = "John Doe"
	claims["iat"] = 1516239022

	tokenString, _ := token.SignedString([]byte(secretKey))
	return tokenString
}

// 测试 JWT token 生成和验证
func TestJWTTokenGeneration(t *testing.T) {
	secretKey := "test-secret-key-123"
	tokenString := generateTestToken(secretKey)

	// 验证生成的 token 是有效的
	require.True(t, ParseTokenValid(tokenString, secretKey), "Generated token should be valid")

	// 验证使用错误密钥时 token 无效
	require.False(t, ParseTokenValid(tokenString, "wrong-secret"), "Token should be invalid with wrong secret")
}

// 测试配置：完整的有效配置
var validConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"token_secret_key": "test-secret-key-123",
		"token_headers":    "authorization",
	})
	return data
}()

// 测试配置：缺少 token_secret_key
var missingSecretKeyConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"token_headers": "authorization",
	})
	return data
}()

// 测试配置：缺少 token_headers
var missingTokenHeadersConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"token_secret_key": "test-secret-key-123",
	})
	return data
}()

// 测试配置：空字符串配置
var emptyStringConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"token_secret_key": "",
		"token_headers":    "",
	})
	return data
}()

// 测试配置：使用不同的请求头名称
var customHeaderConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"token_secret_key": "custom-secret-key",
		"token_headers":    "x-auth-token",
	})
	return data
}()

func TestParseConfig(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// 测试有效配置
		t.Run("valid config", func(t *testing.T) {
			host, status := test.NewTestHost(validConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			jwtConfig := config.(*Config)
			require.Equal(t, "test-secret-key-123", jwtConfig.TokenSecretKey)
			require.Equal(t, "authorization", jwtConfig.TokenHeaders)
		})

		// 测试缺少 token_secret_key 的配置
		t.Run("missing token_secret_key", func(t *testing.T) {
			host, status := test.NewTestHost(missingSecretKeyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			jwtConfig := config.(*Config)
			require.Equal(t, "", jwtConfig.TokenSecretKey)
			require.Equal(t, "authorization", jwtConfig.TokenHeaders)
		})

		// 测试缺少 token_headers 的配置
		t.Run("missing token_headers", func(t *testing.T) {
			host, status := test.NewTestHost(missingTokenHeadersConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			jwtConfig := config.(*Config)
			require.Equal(t, "test-secret-key-123", jwtConfig.TokenSecretKey)
			require.Equal(t, "", jwtConfig.TokenHeaders)
		})

		// 测试空字符串配置
		t.Run("empty string config", func(t *testing.T) {
			host, status := test.NewTestHost(emptyStringConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			jwtConfig := config.(*Config)
			require.Equal(t, "", jwtConfig.TokenSecretKey)
			require.Equal(t, "", jwtConfig.TokenHeaders)
		})

		// 测试自定义请求头配置
		t.Run("custom header config", func(t *testing.T) {
			host, status := test.NewTestHost(customHeaderConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			jwtConfig := config.(*Config)
			require.Equal(t, "custom-secret-key", jwtConfig.TokenSecretKey)
			require.Equal(t, "x-auth-token", jwtConfig.TokenHeaders)
		})
	})
}

func TestOnHttpRequestHeaders(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试有效配置下的有效 JWT token
		t.Run("valid config with valid token", func(t *testing.T) {
			host, status := test.NewTestHost(validConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 生成有效的 JWT token
			validToken := generateTestToken("test-secret-key-123")

			// 模拟带有有效 JWT token 的请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "test.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"authorization", validToken},
			})

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			localResponse := host.GetLocalResponse()
			require.Nil(t, localResponse, "Valid token should not be rejected")

			host.CompleteHttp()
		})

		// 测试缺少 token_secret_key 的配置
		t.Run("missing token_secret_key", func(t *testing.T) {
			host, status := test.NewTestHost(missingSecretKeyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "test.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"authorization", "valid-token"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			require.Equal(t, uint32(401), localResponse.StatusCode)
			require.Equal(t, "simple-jwt-auth.bad_config", localResponse.StatusCodeDetail)

			// 验证响应体
			var responseBody map[string]interface{}
			err := json.Unmarshal(localResponse.Data, &responseBody)
			require.NoError(t, err)
			require.Equal(t, float64(400), responseBody["code"])
			require.Equal(t, "token or secret 不允许为空", responseBody["msg"])

			host.CompleteHttp()
		})

		// 测试缺少 token_headers 的配置
		t.Run("missing token_headers", func(t *testing.T) {
			host, status := test.NewTestHost(missingTokenHeadersConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "test.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"authorization", "valid-token"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			require.Equal(t, uint32(401), localResponse.StatusCode)
			require.Equal(t, "simple-jwt-auth.bad_config", localResponse.StatusCodeDetail)

			host.CompleteHttp()
		})

		// 测试空字符串配置
		t.Run("empty string config", func(t *testing.T) {
			host, status := test.NewTestHost(emptyStringConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "test.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"authorization", "valid-token"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			require.Equal(t, uint32(401), localResponse.StatusCode)
			require.Equal(t, "simple-jwt-auth.bad_config", localResponse.StatusCodeDetail)

			host.CompleteHttp()
		})

		// 测试缺少请求头的情况
		t.Run("missing token header", func(t *testing.T) {
			host, status := test.NewTestHost(validConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "test.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				// 缺少 authorization 头部
			})

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			require.Equal(t, uint32(401), localResponse.StatusCode)
			require.Equal(t, "simple-jwt-auth.auth_failed", localResponse.StatusCodeDetail)

			// 验证响应体
			var responseBody map[string]interface{}
			err := json.Unmarshal(localResponse.Data, &responseBody)
			require.NoError(t, err)
			require.Equal(t, float64(401), responseBody["code"])
			require.Equal(t, "认证失败", responseBody["msg"])

			host.CompleteHttp()
		})

		// 测试无效的 JWT token
		t.Run("invalid JWT token", func(t *testing.T) {
			host, status := test.NewTestHost(validConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 使用一个格式正确但签名无效的 token，避免 panic
			// 这个 token 格式正确，但签名不匹配
			invalidToken := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.invalid_signature_part"

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "test.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"authorization", invalidToken},
			})

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			require.Equal(t, uint32(401), localResponse.StatusCode)
			require.Equal(t, "simple-jwt-auth.auth_failed", localResponse.StatusCodeDetail)

			// 验证响应体
			var responseBody map[string]interface{}
			err := json.Unmarshal(localResponse.Data, &responseBody)
			require.NoError(t, err)
			require.Equal(t, float64(401), responseBody["code"])
			require.Equal(t, "认证失败", responseBody["msg"])

			host.CompleteHttp()
		})

		// 测试自定义请求头名称
		t.Run("custom header name", func(t *testing.T) {
			host, status := test.NewTestHost(customHeaderConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 使用一个格式正确但签名无效的 token，避免 panic
			invalidToken := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.invalid_signature_part"

			// 使用自定义请求头名称
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "test.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"x-auth-token", invalidToken},
			})

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			require.Equal(t, uint32(401), localResponse.StatusCode)
			require.Equal(t, "simple-jwt-auth.auth_failed", localResponse.StatusCodeDetail)

			host.CompleteHttp()
		})

		// 测试空 token 值
		t.Run("empty token value", func(t *testing.T) {
			host, status := test.NewTestHost(validConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "test.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"authorization", ""}, // 空 token 值
			})

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			require.Equal(t, uint32(401), localResponse.StatusCode)
			require.Equal(t, "simple-jwt-auth.auth_failed", localResponse.StatusCodeDetail)

			host.CompleteHttp()
		})
	})
}

// 测试边界情况和错误处理
func TestEdgeCases(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试非常长的 token
		t.Run("very long token", func(t *testing.T) {
			host, status := test.NewTestHost(validConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 创建一个非常长的 token，但使用安全的格式避免 panic
			// 使用重复的字符而不是随机字节
			longToken := "Bearer " + strings.Repeat("a", 1000) + ".eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.invalid_signature"

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "test.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"authorization", longToken},
			})

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			require.Equal(t, uint32(401), localResponse.StatusCode)
			require.Equal(t, "simple-jwt-auth.auth_failed", localResponse.StatusCodeDetail)

			host.CompleteHttp()
		})

		// 测试特殊字符的 token
		t.Run("special characters in token", func(t *testing.T) {
			host, status := test.NewTestHost(validConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			specialToken := "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "test.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"authorization", specialToken},
			})

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			require.Equal(t, uint32(401), localResponse.StatusCode)
			require.Equal(t, "simple-jwt-auth.auth_failed", localResponse.StatusCodeDetail)

			host.CompleteHttp()
		})

		// 测试没有 Bearer 前缀的 token
		t.Run("token without Bearer prefix", func(t *testing.T) {
			host, status := test.NewTestHost(validConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "test.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"authorization", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			require.Equal(t, uint32(401), localResponse.StatusCode)
			require.Equal(t, "simple-jwt-auth.auth_failed", localResponse.StatusCodeDetail)

			host.CompleteHttp()
		})
	})
}
