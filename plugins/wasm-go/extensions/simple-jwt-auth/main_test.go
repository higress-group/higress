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
	"encoding/base64"
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

// buildRawToken 把 header/payload/signature 三段分别做 base64url 编码后拼成
// 一个 Token 字符串。它不保证结果一定合法，正是用来构造"分段数正确但内容
// 畸形"（例如 alg=none）的对抗性输入；签名段传空字符串即可得到未签名 Token。
func buildRawToken(header, payload, signature string) string {
	encode := func(segment string) string {
		if segment == "" {
			return ""
		}
		return base64.RawURLEncoding.EncodeToString([]byte(segment))
	}
	return encode(header) + "." + encode(payload) + "." + encode(signature)
}

// TestParseTokenValidInvalidInputs 覆盖 ParseTokenValid 的各种畸形输入。
//
// github.com/dgrijalva/jwt-go 在输入分段数不是三段时会返回 (nil, error)，
// 旧实现忽略 error 直接读取 token.Valid，于是任何非三段式输入都会触发
// nil pointer panic；在 WASM 数据面中这会 trap 掉插件实例，让整条路由在
// 后续流量上不可用，一条未认证请求就足以造成故障。
// 因此这里逐项断言"返回 false 且不 panic"，作为该缺陷的回归测试。
func TestParseTokenValidInvalidInputs(t *testing.T) {
	const secretKey = "test-secret-key-123"
	validToken := generateTestToken(secretKey)

	cases := []struct {
		name        string
		tokenString string
		wantValid   bool
		wantErr     bool
	}{
		// 以下用例在旧实现下都会 panic：jwt-go 返回的 token 为 nil。
		{name: "empty string", tokenString: "", wantValid: false, wantErr: true},
		{name: "bearer scheme only", tokenString: "Bearer", wantValid: false, wantErr: true},
		{name: "bearer scheme with trailing space", tokenString: "Bearer ", wantValid: false, wantErr: true},
		{name: "single segment", tokenString: "abc", wantValid: false, wantErr: true},
		{name: "two segments", tokenString: "a.b", wantValid: false, wantErr: true},
		{name: "four segments", tokenString: "a.b.c.d", wantValid: false, wantErr: true},
		{name: "segments that are not base64", tokenString: "!!!.???.###", wantValid: false, wantErr: true},
		// 分段数正确但没有有效签名：必须判为无效，而不是被放行。
		{name: "garbage signature", tokenString: buildRawToken(`{"alg":"HS256","typ":"JWT"}`, `{"sub":"1234567890"}`, "not-a-signature"), wantValid: false, wantErr: true},
		// 声明 alg=none 的未签名 Token 必须被算法白名单拒绝，不能交给 keyFunc 处理。
		{name: "alg none token", tokenString: buildRawToken(`{"alg":"none","typ":"JWT"}`, `{"sub":"1234567890"}`, ""), wantValid: false, wantErr: true},
		{name: "alg none token with bearer scheme", tokenString: "Bearer " + buildRawToken(`{"alg":"none","typ":"JWT"}`, `{"sub":"1234567890"}`, ""), wantValid: false, wantErr: true},
		// 使用其它密钥签发的 Token 也必须被拒绝。
		{name: "signed with another secret", tokenString: generateTestToken("another-secret"), wantValid: false, wantErr: true},
		// 合法 Token 仍需被接受，"Bearer " 前缀大小写不敏感。
		{name: "valid token", tokenString: validToken, wantValid: true, wantErr: false},
		{name: "valid token with bearer scheme", tokenString: "Bearer " + validToken, wantValid: true, wantErr: false},
		{name: "valid token with lowercase bearer scheme", tokenString: "bearer " + validToken, wantValid: true, wantErr: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// 调用刻意不包在 require.NotPanics 里：一旦回归成 nil 解引用，
			// 测试会直接 panic 失败，这正是要守住的底线。
			valid, err := ParseTokenValid(tc.tokenString, secretKey)
			require.Equal(t, tc.wantValid, valid, "unexpected validate result for %q", tc.tokenString)
			if tc.wantErr {
				require.Error(t, err, "expected a parse/validation error for %q", tc.tokenString)
			} else {
				require.NoError(t, err, "expected no error for %q", tc.tokenString)
			}
		})
	}
}

// 测试 JWT token 生成和验证
func TestJWTTokenGeneration(t *testing.T) {
	secretKey := "test-secret-key-123"
	tokenString := generateTestToken(secretKey)

	// 验证生成的 token 是有效的
	valid, err := ParseTokenValid(tokenString, secretKey)
	require.NoError(t, err, "Generated token should be parseable")
	require.True(t, valid, "Generated token should be valid")

	// 验证使用错误密钥时 token 无效
	valid, err = ParseTokenValid(tokenString, "wrong-secret")
	require.Error(t, err, "Token signed with another secret should report an error")
	require.False(t, valid, "Token should be invalid with wrong secret")
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

			// 使用一个格式正确但签名无效的 token：分段数正确时解析器会返回
			// 非 nil 的 token 加上签名校验错误，用于覆盖"返回 false 并报错"的分支
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

			// 仍然使用分段数正确的 token，验证自定义请求头名称走的是同一套校验逻辑
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

		// 测试请求头存在但值为空的 token：无论宿主把空值当成"缺失"返回错误，
		// 还是当成 "" 交给插件解析，都必须稳定返回 401 而不是 panic
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

			// 超长 token 也走同一条校验路径：修复后不再需要用"安全格式"
			// 去绕开 panic，这里刻意保留超长的非法输入作为边界用例
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

// TestOnHttpRequestHeadersMalformedToken 断言"请求头存在但不是三段式 JWT"
// 不会让插件崩溃：旧实现中这些输入会让 jwt-go 返回 nil token，
// onHttpRequestHeaders 读取 token.Valid 触发 nil pointer panic，
// 表现为插件实例 trap、后续流量全部失败。
// 修复后每条请求都必须稳定返回 401 simple-jwt-auth.auth_failed。
func TestOnHttpRequestHeadersMalformedToken(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		cases := []struct {
			name  string
			value string
		}{
			{name: "present but empty header", value: ""},
			{name: "bearer scheme only", value: "Bearer"},
			{name: "bearer scheme with trailing space", value: "Bearer "},
			{name: "single segment", value: "abc"},
			{name: "two segments", value: "a.b"},
			{name: "four segments", value: "a.b.c.d"},
			{name: "segments that are not base64", value: "!!!.???.###"},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				host, status := test.NewTestHost(validConfig)
				defer host.Reset()
				require.Equal(t, types.OnPluginStartStatusOK, status)

				action := host.CallOnHttpRequestHeaders([][2]string{
					{":authority", "test.com"},
					{":path", "/api/test"},
					{":method", "GET"},
					{"authorization", tc.value},
				})

				require.Equal(t, types.ActionContinue, action)
				require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

				localResponse := host.GetLocalResponse()
				require.NotNil(t, localResponse)
				require.Equal(t, uint32(401), localResponse.StatusCode)
				require.Equal(t, "simple-jwt-auth.auth_failed", localResponse.StatusCodeDetail)

				host.CompleteHttp()
			})
		}
	})
}

// TestOnHttpRequestHeadersBearerScheme 断言带 "Bearer " 前缀的合法 Token
// 会被正常放行，即插件对 "Bearer <token>" 与裸 token 两种写法一致对待。
func TestOnHttpRequestHeadersBearerScheme(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(validConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		validToken := generateTestToken("test-secret-key-123")

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"},
			{":path", "/api/test"},
			{":method", "GET"},
			{"authorization", "Bearer " + validToken},
		})

		require.Equal(t, types.ActionContinue, action)
		require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

		localResponse := host.GetLocalResponse()
		require.Nil(t, localResponse, "Bearer-prefixed valid token should not be rejected")

		host.CompleteHttp()
	})
}
