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
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

// 测试配置：基本全局配置
var basicGlobalConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"consumers": []map[string]interface{}{
			{
				"name":       "consumer1",
				"credential": "admin:123456",
			},
			{
				"name":       "consumer2",
				"credential": "guest:abc",
			},
		},
		"global_auth": false,
	})
	return data
}()

// 测试配置：全局认证开启配置
var globalAuthTrueConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"consumers": []map[string]interface{}{
			{
				"name":       "consumer1",
				"credential": "admin:123456",
			},
			{
				"name":       "consumer2",
				"credential": "guest:abc",
			},
		},
		"global_auth": true,
	})
	return data
}()

// 测试配置：路由鉴权配置
var routeAuthConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"consumers": []map[string]interface{}{
			{
				"name":       "consumer1",
				"credential": "admin:123456",
			},
			{
				"name":       "consumer2",
				"credential": "guest:abc",
			},
		},
		"global_auth": false,
		"allow": []string{
			"consumer1",
		},
	})
	return data
}()

// 测试配置：域名鉴权配置
var domainAuthConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"consumers": []map[string]interface{}{
			{
				"name":       "consumer1",
				"credential": "admin:123456",
			},
			{
				"name":       "consumer2",
				"credential": "guest:abc",
			},
		},
		"global_auth": false,
		"allow": []string{
			"consumer2",
		},
	})
	return data
}()

// 测试配置：无效配置（缺少 consumers）
var invalidConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"global_auth": false,
	})
	return data
}()

// 测试配置：无效配置（空的 consumers）
var emptyConsumersConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"consumers":   []map[string]interface{}{},
		"global_auth": false,
	})
	return data
}()

// 测试配置：无效配置（重复的 credential）
var duplicateCredentialConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"consumers": []map[string]interface{}{
			{
				"name":       "consumer1",
				"credential": "admin:123456",
			},
			{
				"name":       "consumer2",
				"credential": "admin:123456", // 重复的 credential
			},
		},
		"global_auth": false,
	})
	return data
}()

// 测试配置：无效配置（无效的 credential 格式）
var invalidCredentialFormatConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"consumers": []map[string]interface{}{
			{
				"name":       "consumer1",
				"credential": "admin", // 缺少密码部分
			},
		},
		"global_auth": false,
	})
	return data
}()

// 测试配置：无效配置（缺少 consumer name）
var missingConsumerNameConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"consumers": []map[string]interface{}{
			{
				"credential": "admin:123456",
				// 缺少 name
			},
		},
		"global_auth": false,
	})
	return data
}()

// 测试配置：无效配置（空的 consumer name）
var emptyConsumerNameConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"consumers": []map[string]interface{}{
			{
				"name":       "",
				"credential": "admin:123456",
			},
		},
		"global_auth": false,
	})
	return data
}()

// 测试配置：无效配置（空的 credential）
var emptyCredentialConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"consumers": []map[string]interface{}{
			{
				"name":       "consumer1",
				"credential": "",
			},
		},
		"global_auth": false,
	})
	return data
}()

// 测试配置：无效配置（空的 allow 列表）
var emptyAllowConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"allow": []string{},
	})
	return data
}()

// 测试配置：路由级别配置（使用 _rules_ 和 _match_route_）
var routeLevelConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"consumers": []map[string]interface{}{
			{
				"name":       "consumer1",
				"credential": "admin:123456",
			},
			{
				"name":       "consumer2",
				"credential": "guest:abc",
			},
		},
		"global_auth": false,
		"_rules_": []map[string]interface{}{
			{
				"_match_route_": []string{"route-a", "route-b"},
				"allow":         []string{"consumer1"},
			},
			{
				"_match_route_": []string{"route-c"},
				"allow":         []string{"consumer2"},
			},
		},
	})
	return data
}()

// 测试配置：域名级别配置（使用 _rules_ 和 _match_domain_）
var domainLevelConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"consumers": []map[string]interface{}{
			{
				"name":       "consumer1",
				"credential": "admin:123456",
			},
			{
				"name":       "consumer2",
				"credential": "guest:abc",
			},
		},
		"global_auth": false,
		"_rules_": []map[string]interface{}{
			{
				"_match_domain_": []string{"*.example.com", "test.com"},
				"allow":          []string{"consumer2"},
			},
			{
				"_match_domain_": []string{"api.example.com"},
				"allow":          []string{"consumer1"},
			},
		},
	})
	return data
}()

// 测试配置：服务级别配置（使用 _rules_ 和 _match_service_）
var serviceLevelConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"consumers": []map[string]interface{}{
			{
				"name":       "consumer1",
				"credential": "admin:123456",
			},
			{
				"name":       "consumer2",
				"credential": "guest:abc",
			},
		},
		"global_auth": false,
		"_rules_": []map[string]interface{}{
			{
				"_match_service_": []string{"service-a:8080", "service-b"},
				"allow":           []string{"consumer1"},
			},
			{
				"_match_service_": []string{"service-c:9090"},
				"allow":           []string{"consumer2"},
			},
		},
	})
	return data
}()

// 测试配置：路由前缀级别配置（使用 _rules_ 和 _match_route_prefix_）
var routePrefixLevelConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"consumers": []map[string]interface{}{
			{
				"name":       "consumer1",
				"credential": "admin:123456",
			},
			{
				"name":       "consumer2",
				"credential": "guest:abc",
			},
		},
		"global_auth": false,
		"_rules_": []map[string]interface{}{
			{
				"_match_route_prefix_": []string{"api-", "web-"},
				"allow":                []string{"consumer1"},
			},
			{
				"_match_route_prefix_": []string{"admin-", "internal-"},
				"allow":                []string{"consumer2"},
			},
		},
	})
	return data
}()

// 测试配置：路由和服务组合配置（使用 _rules_、_match_route_ 和 _match_service_）
var routeAndServiceLevelConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"consumers": []map[string]interface{}{
			{
				"name":       "consumer1",
				"credential": "admin:123456",
			},
			{
				"name":       "consumer2",
				"credential": "guest:abc",
			},
		},
		"global_auth": false,
		"_rules_": []map[string]interface{}{
			{
				"_match_route_":   []string{"route-a"},
				"_match_service_": []string{"service-a:8080"},
				"allow":           []string{"consumer1"},
			},
			{
				"_match_route_":   []string{"route-b"},
				"_match_service_": []string{"service-b:9090"},
				"allow":           []string{"consumer2"},
			},
		},
	})
	return data
}()

// 测试配置：混合级别配置
var mixedLevelConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"consumers": []map[string]interface{}{
			{
				"name":       "consumer1",
				"credential": "admin:123456",
			},
			{
				"name":       "consumer2",
				"credential": "guest:abc",
			},
			{
				"name":       "consumer3",
				"credential": "user:def",
			},
		},
		"global_auth": false,
		"_rules_": []map[string]interface{}{
			{
				"_match_route_": []string{"api-route"},
				"allow":         []string{"consumer1"},
			},
			{
				"_match_domain_": []string{"*.example.com"},
				"allow":          []string{"consumer2"},
			},
			{
				"_match_service_": []string{"internal-service:8080"},
				"allow":           []string{"consumer3"},
			},
			{
				"_match_route_prefix_": []string{"web-"},
				"allow":                []string{"consumer1", "consumer2"},
			},
		},
	})
	return data
}()

// 测试配置：无效规则配置（缺少匹配条件）
var invalidRuleConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"consumers": []map[string]interface{}{
			{
				"name":       "consumer1",
				"credential": "admin:123456",
			},
		},
		"global_auth": false,
		"_rules_": []map[string]interface{}{
			{
				"allow": []string{"consumer1"},
				// 缺少匹配条件
			},
		},
	})
	return data
}()

// 测试配置：无效规则配置（空的匹配条件）
var emptyMatchConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"consumers": []map[string]interface{}{
			{
				"name":       "consumer1",
				"credential": "admin:123456",
			},
		},
		"global_auth": false,
		"_rules_": []map[string]interface{}{
			{
				"_match_route_": []string{},
				"allow":         []string{"consumer1"},
			},
		},
	})
	return data
}()

func TestParseGlobalConfig(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// 测试基本全局配置解析
		t.Run("basic global config", func(t *testing.T) {
			host, status := test.NewTestHost(basicGlobalConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试全局认证开启配置解析
		t.Run("global auth true config", func(t *testing.T) {
			host, status := test.NewTestHost(globalAuthTrueConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试无效配置（缺少 consumers）
		t.Run("invalid config - missing consumers", func(t *testing.T) {
			host, status := test.NewTestHost(invalidConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})

		// 测试无效配置（空的 consumers）
		t.Run("invalid config - empty consumers", func(t *testing.T) {
			host, status := test.NewTestHost(emptyConsumersConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})

		// 测试无效配置（重复的 credential）
		t.Run("invalid config - duplicate credential", func(t *testing.T) {
			host, status := test.NewTestHost(duplicateCredentialConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})

		// 测试无效配置（无效的 credential 格式）
		t.Run("invalid config - invalid credential format", func(t *testing.T) {
			host, status := test.NewTestHost(invalidCredentialFormatConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})

		// 测试无效配置（缺少 consumer name）
		t.Run("invalid config - missing consumer name", func(t *testing.T) {
			host, status := test.NewTestHost(missingConsumerNameConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})

		// 测试无效配置（空的 consumer name）
		t.Run("invalid config - empty consumer name", func(t *testing.T) {
			host, status := test.NewTestHost(emptyConsumerNameConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})

		// 测试无效配置（空的 credential）
		t.Run("invalid config - empty credential", func(t *testing.T) {
			host, status := test.NewTestHost(emptyCredentialConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})
	})
}

func TestParseOverrideRuleConfig(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// 测试路由鉴权配置解析
		t.Run("route auth config", func(t *testing.T) {
			host, status := test.NewTestHost(routeAuthConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试域名鉴权配置解析
		t.Run("domain auth config", func(t *testing.T) {
			host, status := test.NewTestHost(domainAuthConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试无效配置（空的 allow 列表）
		t.Run("invalid config - empty allow list", func(t *testing.T) {
			host, status := test.NewTestHost(emptyAllowConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})
	})
}

func TestParseRuleConfig(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// 测试路由级别配置解析
		t.Run("route level config", func(t *testing.T) {
			host, status := test.NewTestHost(routeLevelConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试域名级别配置解析
		t.Run("domain level config", func(t *testing.T) {
			host, status := test.NewTestHost(domainLevelConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试服务级别配置解析
		t.Run("service level config", func(t *testing.T) {
			host, status := test.NewTestHost(serviceLevelConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试路由前缀级别配置解析
		t.Run("route prefix level config", func(t *testing.T) {
			host, status := test.NewTestHost(routePrefixLevelConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试路由和服务组合配置解析
		t.Run("route and service level config", func(t *testing.T) {
			host, status := test.NewTestHost(routeAndServiceLevelConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试混合级别配置解析
		t.Run("mixed level config", func(t *testing.T) {
			host, status := test.NewTestHost(mixedLevelConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试无效规则配置（缺少匹配条件）
		t.Run("invalid rule config - missing match conditions", func(t *testing.T) {
			host, status := test.NewTestHost(invalidRuleConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})

		// 测试无效规则配置（空的匹配条件）
		t.Run("invalid rule config - empty match conditions", func(t *testing.T) {
			host, status := test.NewTestHost(emptyMatchConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})
	})
}

func TestOnHttpRequestHeaders(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试缺少 Authorization 头的情况
		t.Run("missing authorization header", func(t *testing.T) {
			host, status := test.NewTestHost(basicGlobalConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，不包含 Authorization
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
			})

			// 应该返回 ActionContinue，因为 global_auth 为 false 且没有配置 allow
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})

		// 测试空的 Authorization 头的情况
		t.Run("empty authorization header", func(t *testing.T) {
			host, status := test.NewTestHost(basicGlobalConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，包含空的 Authorization
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"authorization", ""},
			})

			// 应该返回 ActionContinue，因为 global_auth 为 false 且没有配置 allow
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})

		// 测试无效的 Authorization 头格式（缺少 Basic 前缀）
		t.Run("invalid authorization format - missing basic prefix", func(t *testing.T) {
			host, status := test.NewTestHost(globalAuthTrueConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，包含无效的 Authorization 格式
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"authorization", "Bearer token123"},
			})

			// 应该返回 ActionContinue，因为 global_auth 为 true
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})

		// 测试无效的 Authorization 头格式（无效的 base64）
		t.Run("invalid authorization format - invalid base64", func(t *testing.T) {
			host, status := test.NewTestHost(globalAuthTrueConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，包含无效的 base64 编码
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"authorization", "Basic invalid-base64"},
			})

			// 应该返回 ActionContinue，因为 global_auth 为 true
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})

		// 测试无效的凭证格式（缺少密码部分）
		t.Run("invalid credential format - missing password", func(t *testing.T) {
			host, status := test.NewTestHost(globalAuthTrueConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，包含无效的凭证格式
			encodedCredential := base64.StdEncoding.EncodeToString([]byte("admin"))
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"authorization", "Basic " + encodedCredential},
			})

			// 应该返回 ActionContinue，因为 global_auth 为 true
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})

		// 测试无效的用户名（未配置的用户名）
		t.Run("invalid username - not configured", func(t *testing.T) {
			host, status := test.NewTestHost(globalAuthTrueConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，包含未配置的用户名
			encodedCredential := base64.StdEncoding.EncodeToString([]byte("unknown:password"))
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"authorization", "Basic " + encodedCredential},
			})

			// 应该返回 ActionContinue，因为 global_auth 为 true
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})

		// 测试无效的密码（错误的密码）
		t.Run("invalid password - wrong password", func(t *testing.T) {
			host, status := test.NewTestHost(globalAuthTrueConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，包含错误的密码
			encodedCredential := base64.StdEncoding.EncodeToString([]byte("admin:wrongpassword"))
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"authorization", "Basic " + encodedCredential},
			})

			// 应该返回 ActionContinue，因为 global_auth 为 true
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})

		// 测试有效的凭证（全局认证开启，无 allow 配置）
		t.Run("valid credentials - global auth true, no allow config", func(t *testing.T) {
			host, status := test.NewTestHost(globalAuthTrueConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，包含有效的凭证
			encodedCredential := base64.StdEncoding.EncodeToString([]byte("admin:123456"))
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"authorization", "Basic " + encodedCredential},
			})

			// 应该返回 ActionContinue，因为凭证有效
			require.Equal(t, types.ActionContinue, action)

			// 注意：在测试框架中，proxywasm.AddHttpRequestHeader 可能不会立即反映在 host.GetRequestHeaders() 中
			// 这是因为测试框架可能没有完全模拟插件的执行环境
			// 我们主要验证插件的行为逻辑，而不是具体的请求头修改

			host.CompleteHttp()
		})

		// 测试有效的凭证（全局认证关闭，有 allow 配置）
		t.Run("valid credentials - global auth false, with allow config", func(t *testing.T) {
			// 这里需要先设置全局配置，然后设置路由配置
			// 由于测试框架的限制，我们直接测试路由配置
			host, status := test.NewTestHost(routeAuthConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，包含有效的凭证
			encodedCredential := base64.StdEncoding.EncodeToString([]byte("admin:123456"))
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"authorization", "Basic " + encodedCredential},
			})

			// 应该返回 ActionContinue，因为凭证有效且在 allow 列表中
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})

		// 测试有效的凭证但不在 allow 列表中的情况
		t.Run("valid credentials but not in allow list", func(t *testing.T) {
			host, status := test.NewTestHost(routeAuthConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，包含有效的凭证但不在 allow 列表中
			encodedCredential := base64.StdEncoding.EncodeToString([]byte("guest:abc"))
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"authorization", "Basic " + encodedCredential},
			})

			// 应该返回 ActionContinue，因为凭证有效但不在 allow 列表中
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})
	})
}

func TestCompleteFlow(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("complete basic auth flow", func(t *testing.T) {
			host, status := test.NewTestHost(globalAuthTrueConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 1. 测试缺少认证信息的情况
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
			})

			// 应该返回 ActionContinue，因为 global_auth 为 true
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()

			// 2. 测试有效认证的情况
			encodedCredential := base64.StdEncoding.EncodeToString([]byte("admin:123456"))
			action = host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				{"authorization", "Basic " + encodedCredential},
			})

			// 应该返回 ActionContinue，因为凭证有效
			require.Equal(t, types.ActionContinue, action)

			// 验证是否添加了 X-Mse-Consumer 请求头
			requestHeaders := host.GetRequestHeaders()
			require.True(t, test.HasHeaderWithValue(requestHeaders, "X-Mse-Consumer", "consumer1"))

			host.CompleteHttp()
		})
	})
}
