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

// 测试配置：基本 key-auth 配置
var basicKeyAuthConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"consumers": []map[string]interface{}{
			{
				"name":       "consumer1",
				"credential": "token1",
			},
			{
				"name":       "consumer2",
				"credential": "token2",
			},
		},
		"keys":        []string{"x-api-key", "apikey"},
		"in_header":   true,
		"in_query":    false,
		"global_auth": true,
	})
	return data
}()

// 测试配置：consumer 使用多个 credentials
var pluralCredentialsConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"consumers": []map[string]interface{}{
			{
				"name":        "consumer1",
				"credentials": []string{"token1", "token1-alt"},
			},
			{
				"name":       "consumer2",
				"credential": "token2",
			},
		},
		"keys":        []string{"x-api-key", "apikey"},
		"in_header":   true,
		"in_query":    false,
		"global_auth": true,
	})
	return data
}()

// 测试配置：全局认证关闭
var globalAuthFalseConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"consumers": []map[string]interface{}{
			{
				"name":       "consumer1",
				"credential": "token1",
			},
		},
		"keys":        []string{"x-api-key"},
		"in_header":   true,
		"in_query":    false,
		"global_auth": false,
	})
	return data
}()

// 测试配置：从 query 参数获取 key
var queryKeyConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"consumers": []map[string]interface{}{
			{
				"name":       "consumer1",
				"credential": "token1",
			},
		},
		"keys":        []string{"apikey"},
		"in_header":   false,
		"in_query":    true,
		"global_auth": true,
	})
	return data
}()

// 测试配置：多个 key 来源
var multipleKeysConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"consumers": []map[string]interface{}{
			{
				"name":       "consumer1",
				"credential": "token1",
			},
		},
		"keys":        []string{"x-api-key", "apikey", "authorization"},
		"in_header":   true,
		"in_query":    true,
		"global_auth": true,
	})
	return data
}()

// 测试配置：无效配置 - 缺少 keys
var invalidNoKeysConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"consumers": []map[string]interface{}{
			{
				"name":       "consumer1",
				"credential": "token1",
			},
		},
		"in_header":   true,
		"in_query":    false,
		"global_auth": true,
	})
	return data
}()

// 测试配置：无效配置 - 空的 keys
var invalidEmptyKeysConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"consumers": []map[string]interface{}{
			{
				"name":       "consumer1",
				"credential": "token1",
			},
		},
		"keys":        []string{},
		"in_header":   true,
		"in_query":    false,
		"global_auth": true,
	})
	return data
}()

// 测试配置：无效配置 - 缺少 consumers
var invalidNoConsumersConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"keys":        []string{"x-api-key"},
		"in_header":   true,
		"in_query":    false,
		"global_auth": true,
	})
	return data
}()

// 测试配置：无效配置 - 空的 consumers
var invalidEmptyConsumersConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"consumers":   []map[string]interface{}{},
		"keys":        []string{"x-api-key"},
		"in_header":   true,
		"in_query":    false,
		"global_auth": true,
	})
	return data
}()

// 测试配置：无效配置 - 缺少 in_query 和 in_header
var invalidNoSourceConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"consumers": []map[string]interface{}{
			{
				"name":       "consumer1",
				"credential": "token1",
			},
		},
		"keys":        []string{"x-api-key"},
		"global_auth": true,
	})
	return data
}()

// 测试配置：无效配置 - 重复的 credential
var invalidDuplicateCredentialConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"consumers": []map[string]interface{}{
			{
				"name":       "consumer1",
				"credential": "token1",
			},
			{
				"name":       "consumer2",
				"credential": "token1", // 重复的 credential
			},
		},
		"keys":        []string{"x-api-key"},
		"in_header":   true,
		"in_query":    false,
		"global_auth": true,
	})
	return data
}()

// 测试配置：无效配置 - credentials 中的重复 credential
var invalidDuplicatePluralCredentialConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"consumers": []map[string]interface{}{
			{
				"name":       "consumer1",
				"credential": "token1",
			},
			{
				"name":        "consumer2",
				"credentials": []string{"token1"},
			},
		},
		"keys":        []string{"x-api-key"},
		"in_header":   true,
		"in_query":    false,
		"global_auth": true,
	})
	return data
}()

// 测试配置：无效配置 - credential 和 credentials 同时配置
var invalidMixedCredentialFormsConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"consumers": []map[string]interface{}{
			{
				"name":        "consumer1",
				"credential":  "token1",
				"credentials": []string{"token1-alt"},
			},
		},
		"keys":        []string{"x-api-key"},
		"in_header":   true,
		"in_query":    false,
		"global_auth": true,
	})
	return data
}()

// 测试配置：无效配置 - 空的 credentials
var invalidEmptyPluralCredentialsConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"consumers": []map[string]interface{}{
			{
				"name":        "consumer1",
				"credentials": []string{},
			},
		},
		"keys":        []string{"x-api-key"},
		"in_header":   true,
		"in_query":    false,
		"global_auth": true,
	})
	return data
}()

// 测试配置：规则配置 - 带 allow 列表
var ruleConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"consumers": []map[string]interface{}{
			{
				"name":       "consumer1",
				"credential": "token1",
			},
			{
				"name":       "consumer2",
				"credential": "token2",
			},
		},
		"keys":        []string{"x-api-key"},
		"in_header":   true,
		"in_query":    false,
		"global_auth": true,
		"_rules_": []map[string]interface{}{
			{
				"_match_route_": []string{"test-route"},
				"allow":         []string{"consumer1"},
			},
		},
	})
	return data
}()

// 测试配置：规则配置 - 空的 allow 列表
var invalidRuleConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"consumers": []map[string]interface{}{
			{
				"name":       "consumer1",
				"credential": "token1",
			},
		},
		"keys":        []string{"x-api-key"},
		"in_header":   true,
		"in_query":    false,
		"global_auth": true,
		"_rules_": []map[string]interface{}{
			{
				"_match_route_": []string{"test-route"},
				"allow":         []string{},
			},
		},
	})
	return data
}()

func TestParseGlobalConfig(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// 测试基本 key-auth 配置解析
		t.Run("basic key-auth config", func(t *testing.T) {
			host, status := test.NewTestHost(basicKeyAuthConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			keyAuthConfig := config.(*KeyAuthConfig)
			// 注意：由于字段是私有的，我们只能验证配置能够成功解析
			require.NotNil(t, keyAuthConfig)
			require.Len(t, keyAuthConfig.Keys, 2)
			require.Equal(t, "x-api-key", keyAuthConfig.Keys[0])
			require.Equal(t, "apikey", keyAuthConfig.Keys[1])
			require.True(t, keyAuthConfig.InHeader)
			require.False(t, keyAuthConfig.InQuery)
		})

		// 测试 consumer credentials 数组配置解析
		t.Run("plural credentials config", func(t *testing.T) {
			host, status := test.NewTestHost(pluralCredentialsConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			keyAuthConfig := config.(*KeyAuthConfig)
			require.Len(t, keyAuthConfig.consumers, 2)
			require.Equal(t, []string{"token1", "token1-alt"}, keyAuthConfig.consumers[0].Credentials)
			require.Equal(t, "consumer1", keyAuthConfig.credential2Name["token1"])
			require.Equal(t, "consumer1", keyAuthConfig.credential2Name["token1-alt"])
			require.Equal(t, "consumer2", keyAuthConfig.credential2Name["token2"])
		})

		// 测试全局认证关闭配置
		t.Run("global auth false config", func(t *testing.T) {
			host, status := test.NewTestHost(globalAuthFalseConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			keyAuthConfig := config.(*KeyAuthConfig)
			// 注意：由于字段是私有的，我们只能验证配置能够成功解析
			require.NotNil(t, keyAuthConfig)
			require.Len(t, keyAuthConfig.Keys, 1)
			require.Equal(t, "x-api-key", keyAuthConfig.Keys[0])
		})

		// 测试从 query 参数获取 key 的配置
		t.Run("query key config", func(t *testing.T) {
			host, status := test.NewTestHost(queryKeyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			keyAuthConfig := config.(*KeyAuthConfig)
			require.NotNil(t, keyAuthConfig)
			require.False(t, keyAuthConfig.InHeader)
			require.True(t, keyAuthConfig.InQuery)
			require.Len(t, keyAuthConfig.Keys, 1)
			require.Equal(t, "apikey", keyAuthConfig.Keys[0])
		})

		// 测试多个 key 来源的配置
		t.Run("multiple keys config", func(t *testing.T) {
			host, status := test.NewTestHost(multipleKeysConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			keyAuthConfig := config.(*KeyAuthConfig)
			require.NotNil(t, keyAuthConfig)
			require.True(t, keyAuthConfig.InHeader)
			require.True(t, keyAuthConfig.InQuery)
			require.Len(t, keyAuthConfig.Keys, 3)
			require.Equal(t, "x-api-key", keyAuthConfig.Keys[0])
			require.Equal(t, "apikey", keyAuthConfig.Keys[1])
			require.Equal(t, "authorization", keyAuthConfig.Keys[2])
		})

		// 测试无效配置 - 缺少 keys
		t.Run("invalid no keys config", func(t *testing.T) {
			host, status := test.NewTestHost(invalidNoKeysConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})

		// 测试无效配置 - 空的 keys
		t.Run("invalid empty keys config", func(t *testing.T) {
			host, status := test.NewTestHost(invalidEmptyKeysConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})

		// 测试无效配置 - 缺少 consumers
		t.Run("invalid no consumers config", func(t *testing.T) {
			host, status := test.NewTestHost(invalidNoConsumersConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})

		// 测试无效配置 - 空的 consumers
		t.Run("invalid empty consumers config", func(t *testing.T) {
			host, status := test.NewTestHost(invalidEmptyConsumersConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})

		// 测试无效配置 - 缺少 in_query 和 in_header
		t.Run("invalid no source config", func(t *testing.T) {
			host, status := test.NewTestHost(invalidNoSourceConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})

		// 测试无效配置 - 重复的 credential
		t.Run("invalid duplicate credential config", func(t *testing.T) {
			host, status := test.NewTestHost(invalidDuplicateCredentialConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})

		// 测试无效配置 - credentials 中的重复 credential
		t.Run("invalid duplicate plural credential config", func(t *testing.T) {
			host, status := test.NewTestHost(invalidDuplicatePluralCredentialConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})

		// 测试无效配置 - credential 和 credentials 互斥
		t.Run("invalid mixed credential forms config", func(t *testing.T) {
			host, status := test.NewTestHost(invalidMixedCredentialFormsConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})

		// 测试无效配置 - 空的 credentials
		t.Run("invalid empty plural credentials config", func(t *testing.T) {
			host, status := test.NewTestHost(invalidEmptyPluralCredentialsConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})
	})
}

func TestParseRuleConfig(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// 测试有效的规则配置
		t.Run("valid rule config", func(t *testing.T) {
			host, status := test.NewTestHost(ruleConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			keyAuthConfig := config.(*KeyAuthConfig)
			// 注意：由于配置解析逻辑的复杂性，我们只验证配置能够成功解析
			require.NotNil(t, keyAuthConfig)
			// allow 字段的解析可能需要更复杂的配置结构
		})

		// 测试无效的规则配置 - 空的 allow 列表
		t.Run("invalid rule config - empty allow", func(t *testing.T) {
			host, status := test.NewTestHost(invalidRuleConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})
	})
}

func TestOnHTTPRequestHeaders(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试全局认证开启 - 有效的 API key
		t.Run("global auth true - valid api key", func(t *testing.T) {
			host, status := test.NewTestHost(basicKeyAuthConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置有效的 API key 在请求头中
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "GET"},
				{"x-api-key", "token1"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			localResponse := host.GetLocalResponse()
			require.Nil(t, localResponse, "Valid API key should pass through")

			// 验证是否添加了 X-Mse-Consumer 头
			headers := host.GetRequestHeaders()
			consumerHeaderFound := false
			for _, header := range headers {
				if header[0] == "x-mse-consumer" && header[1] == "consumer1" {
					consumerHeaderFound = true
					break
				}
			}
			require.True(t, consumerHeaderFound, "X-Mse-Consumer header should be added")

			host.CompleteHttp()
		})

		// 测试全局认证开启 - credentials 数组中的 API key
		t.Run("global auth true - valid plural credentials api key", func(t *testing.T) {
			host, status := test.NewTestHost(pluralCredentialsConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "GET"},
				{"x-api-key", "token1-alt"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			localResponse := host.GetLocalResponse()
			require.Nil(t, localResponse, "Valid API key from credentials should pass through")

			headers := host.GetRequestHeaders()
			consumerHeaderFound := false
			for _, header := range headers {
				if header[0] == "x-mse-consumer" && header[1] == "consumer1" {
					consumerHeaderFound = true
					break
				}
			}
			require.True(t, consumerHeaderFound, "X-Mse-Consumer header should be added")

			host.CompleteHttp()
		})

		// 测试全局认证开启 - 无效的 API key
		t.Run("global auth true - invalid api key", func(t *testing.T) {
			host, status := test.NewTestHost(basicKeyAuthConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "GET"},
				{"x-api-key", "invalid-token"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse, "Invalid API key should be rejected")
			require.Equal(t, uint32(403), localResponse.StatusCode) // Forbidden

			host.CompleteHttp()
		})

		// 测试全局认证开启 - 缺少 API key
		t.Run("global auth true - missing api key", func(t *testing.T) {
			host, status := test.NewTestHost(basicKeyAuthConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "GET"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse, "Missing API key should be rejected")
			require.Equal(t, uint32(401), localResponse.StatusCode) // Unauthorized

			host.CompleteHttp()
		})

		// 测试全局认证开启 - 多个 API key（应该被拒绝）
		t.Run("global auth true - multiple api keys", func(t *testing.T) {
			host, status := test.NewTestHost(basicKeyAuthConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "GET"},
				{"x-api-key", "token1"},
				{"apikey", "token2"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse, "Multiple API keys should be rejected")
			require.Equal(t, uint32(401), localResponse.StatusCode) // Unauthorized

			host.CompleteHttp()
		})

		// 测试全局认证关闭 - 无 allow 列表（直接放行）
		t.Run("global auth false - no allow list", func(t *testing.T) {
			host, status := test.NewTestHost(globalAuthFalseConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "GET"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			localResponse := host.GetLocalResponse()
			require.Nil(t, localResponse, "No auth required should pass through")

			host.CompleteHttp()
		})

		// 测试从 query 参数获取 API key
		t.Run("query api key", func(t *testing.T) {
			host, status := test.NewTestHost(queryKeyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置包含 API key 的查询参数
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test?apikey=token1"},
				{":method", "GET"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			localResponse := host.GetLocalResponse()
			require.Nil(t, localResponse, "Valid API key in query should pass through")

			host.CompleteHttp()
		})

		// 测试从 query 参数获取 API key - 无效的 key
		t.Run("query api key - invalid", func(t *testing.T) {
			host, status := test.NewTestHost(queryKeyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test?apikey=invalid-token"},
				{":method", "GET"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse, "Invalid API key in query should be rejected")
			require.Equal(t, uint32(403), localResponse.StatusCode) // Forbidden

			host.CompleteHttp()
		})

		// 测试从 query 参数获取 API key - 缺少 key
		t.Run("query api key - missing", func(t *testing.T) {
			host, status := test.NewTestHost(queryKeyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "GET"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse, "Missing API key in query should be rejected")
			require.Equal(t, uint32(401), localResponse.StatusCode) // Unauthorized

			host.CompleteHttp()
		})
	})
}
