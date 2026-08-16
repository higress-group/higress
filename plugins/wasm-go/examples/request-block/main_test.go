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

var testConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"blocked_code":      403,
		"blocked_message":   "Access denied",
		"case_sensitive":    false,
		"block_urls":        []string{"blocked", "forbidden"},
		"block_exact_urls":  []string{"/exact-block", "/admin"},
		"block_regexp_urls": []string{`/api/v\d+/blocked`},
		"block_headers":     []string{"blocked-header", "malicious"},
		"block_bodies":      []string{"blocked-content", "spam"},
	})
	return data
}()

func TestParseConfig(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(testConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		config, err := host.GetMatchConfig()
		require.NoError(t, err)
		require.NotNil(t, config)

		blockConfig := config.(*RequestBlockConfig)
		require.Equal(t, uint32(403), blockConfig.blockedCode)
		require.Equal(t, "Access denied", blockConfig.blockedMessage)
		require.False(t, blockConfig.caseSensitive)
		require.Contains(t, blockConfig.blockUrls, "blocked")
		require.Contains(t, blockConfig.blockUrls, "forbidden")
		require.Contains(t, blockConfig.blockExactUrls, "/exact-block")
		require.Contains(t, blockConfig.blockExactUrls, "/admin")
		require.Contains(t, blockConfig.blockHeaders, "blocked-header")
		require.Contains(t, blockConfig.blockHeaders, "malicious")
		require.Contains(t, blockConfig.blockBodies, "blocked-content")
		require.Contains(t, blockConfig.blockBodies, "spam")
	})
}

func TestBlockUrlByKeyword(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(testConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		// Test blocked URL by keyword
		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"},
			{":path", "/api/blocked/endpoint"},
		})
		require.Equal(t, types.ActionContinue, action)

		localResponse := host.GetLocalResponse()
		require.NotNil(t, localResponse)
		require.Equal(t, uint32(403), localResponse.StatusCode)
		require.Equal(t, "Access denied", string(localResponse.Data))
		host.CompleteHttp()
	})
}

func TestBlockUrlByExactMatch(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(testConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		// Test blocked URL by exact match
		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"},
			{":path", "/exact-block"},
		})
		require.Equal(t, types.ActionContinue, action)

		localResponse := host.GetLocalResponse()
		require.NotNil(t, localResponse)
		require.Equal(t, uint32(403), localResponse.StatusCode)
		require.Equal(t, "Access denied", string(localResponse.Data))
		host.CompleteHttp()
	})
}

func TestBlockUrlByRegexp(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(testConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		// Test blocked URL by regexp
		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"},
			{":path", "/api/v1/blocked"},
		})
		require.Equal(t, types.ActionContinue, action)

		localResponse := host.GetLocalResponse()
		require.NotNil(t, localResponse)
		require.Equal(t, uint32(403), localResponse.StatusCode)
		require.Equal(t, "Access denied", string(localResponse.Data))
		host.CompleteHttp()
	})
}

func TestBlockByHeaders(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(testConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		// Test blocked by headers
		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"},
			{":path", "/api/valid"},
			{"blocked-header", "some-value"},
		})
		require.Equal(t, types.ActionContinue, action)

		localResponse := host.GetLocalResponse()
		require.NotNil(t, localResponse)
		require.Equal(t, uint32(403), localResponse.StatusCode)
		require.Equal(t, "Access denied", string(localResponse.Data))
		host.CompleteHttp()
	})
}

func TestBlockByBody(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// Use a config that only has body blocking rules
		host, status := test.NewTestHost(testConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		// First call headers to set up context - use a path that won't be blocked by URL rules
		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"},
			{":path", "/api/safe/endpoint"},
		})
		require.Equal(t, types.ActionContinue, action)

		// Test blocked by body content
		action = host.CallOnHttpRequestBody([]byte("This is blocked-content in the body"))
		require.Equal(t, types.ActionContinue, action)

		localResponse := host.GetLocalResponse()
		require.NotNil(t, localResponse)
		require.Equal(t, uint32(403), localResponse.StatusCode)
		require.Equal(t, "Access denied", string(localResponse.Data))
		host.CompleteHttp()
	})
}

func TestAllowValidRequest(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(testConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		// Test valid request should be allowed
		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"},
			{":path", "/api/valid/endpoint"},
			{"valid-header", "valid-value"},
		})
		require.Equal(t, types.ActionContinue, action)

		localResponse := host.GetLocalResponse()
		require.Nil(t, localResponse, "Valid request should not be blocked")
		host.CompleteHttp()
	})
}

func TestCaseInsensitiveBlocking(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(testConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		// Test case insensitive blocking (config has case_sensitive: false)
		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"},
			{":path", "/API/BLOCKED/ENDPOINT"}, // Uppercase should still be blocked
		})
		require.Equal(t, types.ActionContinue, action)

		localResponse := host.GetLocalResponse()
		require.NotNil(t, localResponse)
		require.Equal(t, uint32(403), localResponse.StatusCode)
		host.CompleteHttp()
	})
}

func TestCustomBlockedCode(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		customConfig := func() json.RawMessage {
			data, _ := json.Marshal(map[string]interface{}{
				"blocked_code":    429,
				"blocked_message": "Too many requests",
				"case_sensitive":  false,
				"block_urls":      []string{"rate-limit"},
			})
			return data
		}()

		host, status := test.NewTestHost(customConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"},
			{":path", "/api/rate-limit/test"},
		})
		require.Equal(t, types.ActionContinue, action)

		localResponse := host.GetLocalResponse()
		require.NotNil(t, localResponse)
		require.Equal(t, uint32(429), localResponse.StatusCode)
		require.Equal(t, "Too many requests", string(localResponse.Data))
		host.CompleteHttp()
	})
}

// 测试配置解析中的边界情况
func TestParseConfigEdgeCases(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// 测试无效的blocked_code（使用默认值403）
		t.Run("invalid blocked_code", func(t *testing.T) {
			invalidCodeConfig := func() json.RawMessage {
				data, _ := json.Marshal(map[string]interface{}{
					"blocked_code":    999, // 无效状态码
					"blocked_message": "Invalid code",
					"block_urls":      []string{"test"},
				})
				return data
			}()

			host, status := test.NewTestHost(invalidCodeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			blockConfig := config.(*RequestBlockConfig)
			require.Equal(t, uint32(403), blockConfig.blockedCode) // 应该使用默认值
		})

		// 测试case_sensitive为true的情况
		t.Run("case sensitive true", func(t *testing.T) {
			caseSensitiveConfig := func() json.RawMessage {
				data, _ := json.Marshal(map[string]interface{}{
					"case_sensitive": true,
					"block_urls":     []string{"BLOCKED"},
					"block_headers":  []string{"BLOCKED-HEADER"},
					"block_bodies":   []string{"BLOCKED-CONTENT"},
				})
				return data
			}()

			host, status := test.NewTestHost(caseSensitiveConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			blockConfig := config.(*RequestBlockConfig)
			require.True(t, blockConfig.caseSensitive)
			require.Contains(t, blockConfig.blockUrls, "BLOCKED") // 保持大写
			require.Contains(t, blockConfig.blockHeaders, "BLOCKED-HEADER")
			require.Contains(t, blockConfig.blockBodies, "BLOCKED-CONTENT")
		})

		// 测试空字符串的处理
		t.Run("empty strings handling", func(t *testing.T) {
			emptyStringsConfig := func() json.RawMessage {
				data, _ := json.Marshal(map[string]interface{}{
					"block_urls":        []string{"valid", ""}, // 包含空字符串
					"block_exact_urls":  []string{"", "valid"}, // 包含空字符串
					"block_regexp_urls": []string{"", "valid"}, // 包含空字符串
					"block_headers":     []string{"", "valid"}, // 包含空字符串
					"block_bodies":      []string{"valid", ""}, // 包含空字符串
				})
				return data
			}()

			host, status := test.NewTestHost(emptyStringsConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			blockConfig := config.(*RequestBlockConfig)
			// 空字符串应该被过滤掉
			require.Contains(t, blockConfig.blockUrls, "valid")
			require.NotContains(t, blockConfig.blockUrls, "")
			require.Contains(t, blockConfig.blockExactUrls, "valid")
			require.NotContains(t, blockConfig.blockExactUrls, "")
		})

		// 测试没有block规则的情况（应该返回错误）
		t.Run("no block rules", func(t *testing.T) {
			noRulesConfig := func() json.RawMessage {
				data, _ := json.Marshal(map[string]interface{}{
					"blocked_message": "No rules",
					// 没有提供任何block规则
				})
				return data
			}()

			host, status := test.NewTestHost(noRulesConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})
	})
}

// 测试onHttpRequestHeaders中的错误处理路径
func TestOnHttpRequestHeadersErrorHandling(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试获取路径失败的情况
		t.Run("get path failed", func(t *testing.T) {
			host, status := test.NewTestHost(testConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 使用不包含:path的头部，模拟获取路径失败
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "test.com"},
				// 缺少 :path 头部
			})

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			localResponse := host.GetLocalResponse()
			require.Nil(t, localResponse)

			host.CompleteHttp()
		})

		// 测试获取头部失败的情况
		t.Run("get headers failed", func(t *testing.T) {
			// 创建一个只有block_headers的配置
			headerOnlyConfig := func() json.RawMessage {
				data, _ := json.Marshal(map[string]interface{}{
					"blocked_code":    403,
					"blocked_message": "Header blocked",
					"block_headers":   []string{"blocked-header"},
				})
				return data
			}()

			host, status := test.NewTestHost(headerOnlyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "test.com"},
				{":path", "/api/test"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			host.CompleteHttp()
		})

		// 测试只有block_bodies的情况（应该调用DontReadRequestBody）
		t.Run("only block bodies", func(t *testing.T) {
			bodyOnlyConfig := func() json.RawMessage {
				data, _ := json.Marshal(map[string]interface{}{
					"blocked_code":    403,
					"blocked_message": "Body blocked",
					"block_bodies":    []string{"blocked-content"},
				})
				return data
			}()

			host, status := test.NewTestHost(bodyOnlyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "test.com"},
				{":path", "/api/test"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			host.CompleteHttp()
		})
	})
}

// 测试onHttpRequestBody中的case_sensitive处理
func TestOnHttpRequestBodyCaseSensitive(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试case_sensitive为true的情况
		t.Run("case sensitive true", func(t *testing.T) {
			caseSensitiveConfig := func() json.RawMessage {
				data, _ := json.Marshal(map[string]interface{}{
					"case_sensitive":  true,
					"blocked_code":    403,
					"blocked_message": "Body blocked",
					"block_bodies":    []string{"BLOCKED"},
				})
				return data
			}()

			host, status := test.NewTestHost(caseSensitiveConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先调用头部处理
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "test.com"},
				{":path", "/api/test"},
			})
			require.Equal(t, types.ActionContinue, action)

			// 测试大写内容应该被阻止
			action = host.CallOnHttpRequestBody([]byte("This contains BLOCKED content"))
			require.Equal(t, types.ActionContinue, action)

			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			require.Equal(t, uint32(403), localResponse.StatusCode)

			host.CompleteHttp()
		})

		// 测试case_sensitive为false的情况（小写内容应该被阻止）
		t.Run("case sensitive false", func(t *testing.T) {
			caseInsensitiveConfig := func() json.RawMessage {
				data, _ := json.Marshal(map[string]interface{}{
					"case_sensitive": false,
					"block_bodies":   []string{"blocked"},
				})
				return data
			}()

			host, status := test.NewTestHost(caseInsensitiveConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先调用头部处理
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "test.com"},
				{":path", "/api/test"},
			})
			require.Equal(t, types.ActionContinue, action)

			// 测试大写内容应该被阻止（因为case_sensitive为false）
			action = host.CallOnHttpRequestBody([]byte("This contains BLOCKED content"))
			require.Equal(t, types.ActionContinue, action)

			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			require.Equal(t, uint32(403), localResponse.StatusCode)

			host.CompleteHttp()
		})
	})
}

// 测试正则表达式URL阻塞的边界情况
func TestBlockUrlByRegexpEdgeCases(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试复杂的正则表达式
		t.Run("complex regexp", func(t *testing.T) {
			complexRegexpConfig := func() json.RawMessage {
				data, _ := json.Marshal(map[string]interface{}{
					"case_sensitive":    true,
					"blocked_code":      403,
					"blocked_message":   "Blocked by regexp",
					"block_urls":        []string{"dummy"}, // 添加一个dummy规则以满足配置检查
					"block_regexp_urls": []string{`/api/v\d+/users/\d+/posts`},
				})
				return data
			}()

			host, status := test.NewTestHost(complexRegexpConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 测试匹配的URL
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "test.com"},
				{":path", "/api/v2/users/123/posts"},
			})
			require.Equal(t, types.ActionContinue, action)

			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			require.Equal(t, uint32(403), localResponse.StatusCode)

			// 确保请求完成
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())
			host.CompleteHttp()
		})

		// 测试不匹配的正则表达式
		t.Run("non-matching regexp", func(t *testing.T) {
			regexpConfig := func() json.RawMessage {
				data, _ := json.Marshal(map[string]interface{}{
					"case_sensitive":    true,
					"blocked_code":      403,
					"blocked_message":   "Blocked by regexp",
					"block_urls":        []string{"dummy"}, // 添加一个dummy规则以满足配置检查
					"block_regexp_urls": []string{`/api/v\d+/blocked`},
				})
				return data
			}()

			host, status := test.NewTestHost(regexpConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 测试不匹配的URL
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "test.com"},
				{":path", "/api/blocked"}, // 不匹配 /api/v\d+/blocked
			})
			require.Equal(t, types.ActionContinue, action)

			localResponse := host.GetLocalResponse()
			require.Nil(t, localResponse)

			// 确保请求完成
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())
			host.CompleteHttp()
		})
	})
}
