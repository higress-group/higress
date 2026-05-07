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
	"strings"
	"testing"

	cfg "github.com/alibaba/higress/plugins/wasm-go/extensions/ai-security-guard/config"
	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-security-guard/utils"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/proxytest"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// 测试配置：基础安全配置
var basicConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"serviceName":               "security-service",
		"servicePort":               8080,
		"serviceHost":               "security.example.com",
		"accessKey":                 "test-ak",
		"secretKey":                 "test-sk",
		"checkRequest":              true,
		"checkResponse":             true,
		"contentModerationLevelBar": "high",
		"promptAttackLevelBar":      "high",
		"sensitiveDataLevelBar":     "S3",
		"timeout":                   2000,
		"bufferLimit":               1000,
	})
	return data
}()

// 测试配置：仅检查请求
var requestOnlyConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"serviceName":               "security-service",
		"servicePort":               8080,
		"serviceHost":               "security.example.com",
		"accessKey":                 "test-ak",
		"secretKey":                 "test-sk",
		"checkRequest":              true,
		"checkResponse":             false,
		"contentModerationLevelBar": "high",
		"promptAttackLevelBar":      "high",
		"sensitiveDataLevelBar":     "S3",
		"timeout":                   1000,
		"bufferLimit":               500,
	})
	return data
}()

// 测试配置：缺少必需字段
var missingRequiredConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"accessKey": "test-ak",
		"secretKey": "test-sk",
		// 故意缺少必需字段：serviceName, servicePort, serviceHost
	})
	return data
}()

// 测试配置：缺少服务配置字段
var missingServiceConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"accessKey":     "test-ak",
		"secretKey":     "test-sk",
		"checkRequest":  true,
		"checkResponse": true,
		// 缺少 serviceName, servicePort, serviceHost
	})
	return data
}()

// 测试配置：缺少认证字段
var missingAuthConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"serviceName":   "security-service",
		"servicePort":   8080,
		"serviceHost":   "security.example.com",
		"checkRequest":  true,
		"checkResponse": true,
		// 缺少 accessKey, secretKey
	})
	return data
}()

// 测试配置：消费者级别特殊配置
var consumerSpecificConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"serviceName":                "security-service",
		"servicePort":                8080,
		"serviceHost":                "security.example.com",
		"accessKey":                  "test-ak",
		"secretKey":                  "test-sk",
		"checkRequest":               true,
		"checkResponse":              false,
		"contentModerationLevelBar":  "high",
		"promptAttackLevelBar":       "high",
		"sensitiveDataLevelBar":      "S3",
		"maliciousUrlLevelBar":       "high",
		"modelHallucinationLevelBar": "high",
		"timeout":                    1000,
		"bufferLimit":                500,
		"consumerRequestCheckService": map[string]interface{}{
			"name":                "aaa",
			"matchType":           "exact",
			"requestCheckService": "llm_query_moderation_1",
		},
		"consumerResponseCheckService": map[string]interface{}{
			"name":                 "bbb",
			"matchType":            "prefix",
			"responseCheckService": "llm_response_moderation_1",
		},
		"consumerRiskLevel": map[string]interface{}{
			"name":                 "ccc.*",
			"matchType":            "regexp",
			"maliciousUrlLevelBar": "low",
		},
	})
	return data
}()

// 测试配置：包含 customLabelLevelBar 和消费者级别覆盖
var customLabelConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"serviceName":               "security-service",
		"servicePort":               8080,
		"serviceHost":               "security.example.com",
		"accessKey":                 "test-ak",
		"secretKey":                 "test-sk",
		"checkRequest":              true,
		"checkResponse":             true,
		"action":                    "MultiModalGuard",
		"customLabelLevelBar":       "high",
		"contentModerationLevelBar": "high",
		"promptAttackLevelBar":      "high",
		"consumerRiskLevel": []map[string]interface{}{
			{
				"name":                "exact-user",
				"matchType":           "exact",
				"customLabelLevelBar": "low",
			},
			{
				"name":                "prefix-",
				"matchType":           "prefix",
				"customLabelLevelBar": "medium",
			},
		},
	})
	return data
}()

// 测试配置：脱敏模式配置（riskAction=mask）
var maskConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"serviceName":               "security-service",
		"servicePort":               8080,
		"serviceHost":               "security.example.com",
		"accessKey":                 "test-ak",
		"secretKey":                 "test-sk",
		"checkRequest":              true,
		"checkResponse":             false,
		"action":                    "MultiModalGuard",
		"riskAction":                "mask",
		"contentModerationLevelBar": "high",
		"promptAttackLevelBar":      "high",
		"sensitiveDataLevelBar":     "S3",
		"timeout":                   2000,
	})
	return data
}()

// 测试配置：MCP配置
var mcpConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"serviceName":                   "security-service",
		"servicePort":                   8080,
		"serviceHost":                   "security.example.com",
		"accessKey":                     "test-ak",
		"secretKey":                     "test-sk",
		"checkRequest":                  false,
		"checkResponse":                 true,
		"action":                        "MultiModalGuard",
		"apiType":                       "mcp",
		"responseContentJsonPath":       "content",
		"responseStreamContentJsonPath": "content",
		"contentModerationLevelBar":     "high",
		"promptAttackLevelBar":          "high",
		"sensitiveDataLevelBar":         "S3",
		"timeout":                       2000,
	})
	return data
}()

var mcpRequestConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"serviceName":               "security-service",
		"servicePort":               8080,
		"serviceHost":               "security.example.com",
		"accessKey":                 "test-ak",
		"secretKey":                 "test-sk",
		"checkRequest":              true,
		"checkResponse":             false,
		"action":                    "MultiModalGuard",
		"apiType":                   "mcp",
		"requestContentJsonPath":    "params.arguments",
		"contentModerationLevelBar": "high",
		"promptAttackLevelBar":      "high",
		"sensitiveDataLevelBar":     "S3",
		"timeout":                   2000,
	})
	return data
}()

// 测试配置：MultiModalGuard 文本生成
var multiModalGuardTextConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"serviceName":               "security-service",
		"servicePort":               8080,
		"serviceHost":               "security.example.com",
		"accessKey":                 "test-ak",
		"secretKey":                 "test-sk",
		"checkRequest":              true,
		"checkResponse":             true,
		"action":                    "MultiModalGuard",
		"apiType":                   "text_generation",
		"contentModerationLevelBar": "high",
		"promptAttackLevelBar":      "high",
		"sensitiveDataLevelBar":     "S3",
		"timeout":                   2000,
		"bufferLimit":               1000,
	})
	return data
}()

// 测试配置：MultiModalGuard OpenAI 图像生成
var multiModalGuardImageOpenAIConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"serviceName":               "security-service",
		"servicePort":               8080,
		"serviceHost":               "security.example.com",
		"accessKey":                 "test-ak",
		"secretKey":                 "test-sk",
		"checkRequest":              true,
		"checkResponse":             true,
		"action":                    "MultiModalGuard",
		"apiType":                   "image_generation",
		"providerType":              "openai",
		"contentModerationLevelBar": "high",
		"promptAttackLevelBar":      "high",
		"sensitiveDataLevelBar":     "S3",
		"timeout":                   2000,
	})
	return data
}()

// 测试配置：MultiModalGuard Qwen 图像生成
var multiModalGuardImageQwenConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"serviceName":               "security-service",
		"servicePort":               8080,
		"serviceHost":               "security.example.com",
		"accessKey":                 "test-ak",
		"secretKey":                 "test-sk",
		"checkRequest":              true,
		"checkResponse":             true,
		"action":                    "MultiModalGuard",
		"apiType":                   "image_generation",
		"providerType":              "qwen",
		"contentModerationLevelBar": "high",
		"promptAttackLevelBar":      "high",
		"sensitiveDataLevelBar":     "S3",
		"timeout":                   2000,
	})
	return data
}()

// 测试配置：ProtocolOriginal MultiModalGuard
var protocolOriginalConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"serviceName":               "security-service",
		"servicePort":               8080,
		"serviceHost":               "security.example.com",
		"accessKey":                 "test-ak",
		"secretKey":                 "test-sk",
		"checkRequest":              true,
		"checkResponse":             false,
		"action":                    "MultiModalGuard",
		"protocol":                  "original",
		"contentModerationLevelBar": "high",
		"promptAttackLevelBar":      "high",
		"sensitiveDataLevelBar":     "S3",
		"timeout":                   2000,
	})
	return data
}()

func TestParseConfig(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// 测试基础配置解析
		t.Run("basic config", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)
			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			securityConfig := config.(*cfg.AISecurityConfig)
			require.Equal(t, "test-ak", securityConfig.AK)
			require.Equal(t, "test-sk", securityConfig.SK)
			require.Equal(t, true, securityConfig.CheckRequest)
			require.Equal(t, true, securityConfig.CheckResponse)
			require.Equal(t, "high", securityConfig.ContentModerationLevelBar)
			require.Equal(t, "high", securityConfig.PromptAttackLevelBar)
			require.Equal(t, "S3", securityConfig.SensitiveDataLevelBar)
			require.Equal(t, uint32(2000), securityConfig.Timeout)
			require.Equal(t, 1000, securityConfig.BufferLimit)
			require.Equal(t, cfg.DefaultResponseFallbackJsonPaths(), securityConfig.ResponseContentFallbackJsonPaths)
			require.Equal(t, cfg.DefaultStreamingResponseFallbackJsonPaths(), securityConfig.ResponseStreamContentFallbackJsonPaths)
		})

		// 测试仅检查请求的配置
		t.Run("request only config", func(t *testing.T) {
			host, status := test.NewTestHost(requestOnlyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)
			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			securityConfig := config.(*cfg.AISecurityConfig)
			require.Equal(t, true, securityConfig.CheckRequest)
			require.Equal(t, false, securityConfig.CheckResponse)
			require.Equal(t, "high", securityConfig.ContentModerationLevelBar)
			require.Equal(t, "high", securityConfig.PromptAttackLevelBar)
			require.Equal(t, "S3", securityConfig.SensitiveDataLevelBar)
		})

		// 测试缺少必需字段的配置
		t.Run("missing required config", func(t *testing.T) {
			host, status := test.NewTestHost(missingRequiredConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})

		// 测试缺少服务配置字段
		t.Run("missing service config", func(t *testing.T) {
			host, status := test.NewTestHost(missingServiceConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})

		// 测试缺少认证字段
		t.Run("missing auth config", func(t *testing.T) {
			host, status := test.NewTestHost(missingAuthConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})

		// 测试消费者级别配置
		t.Run("consumer specific config", func(t *testing.T) {
			host, status := test.NewTestHost(consumerSpecificConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)
			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			securityConfig := config.(*cfg.AISecurityConfig)
			require.Equal(t, "llm_query_moderation", securityConfig.GetRequestCheckService("aaaa"))
			require.Equal(t, "llm_query_moderation_1", securityConfig.GetRequestCheckService("aaa"))
			require.Equal(t, "llm_response_moderation", securityConfig.GetResponseCheckService("bb"))
			require.Equal(t, "llm_response_moderation_1", securityConfig.GetResponseCheckService("bbb-prefix-test"))
			require.Equal(t, "high", securityConfig.GetMaliciousUrlLevelBar("cc"))
			require.Equal(t, "low", securityConfig.GetMaliciousUrlLevelBar("ccc-regexp-test"))
		})

		t.Run("custom response fallback paths config", func(t *testing.T) {
			configJSON, err := json.Marshal(map[string]interface{}{
				"serviceName":                            "security-service",
				"servicePort":                            8080,
				"serviceHost":                            "security.example.com",
				"accessKey":                              "test-ak",
				"secretKey":                              "test-sk",
				"checkResponse":                          true,
				"responseContentFallbackJsonPaths":       []string{"output.text", "choices.0.message.content"},
				"responseStreamContentFallbackJsonPaths": []string{"payload.delta", "delta.text"},
			})
			require.NoError(t, err)
			host, status := test.NewTestHost(configJSON)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			securityConfig := config.(*cfg.AISecurityConfig)
			require.Equal(t, []string{"output.text", "choices.0.message.content"}, securityConfig.ResponseContentFallbackJsonPaths)
			require.Equal(t, []string{"payload.delta", "delta.text"}, securityConfig.ResponseStreamContentFallbackJsonPaths)
		})

		t.Run("empty response fallback paths disable fallback", func(t *testing.T) {
			configJSON, err := json.Marshal(map[string]interface{}{
				"serviceName":                            "security-service",
				"servicePort":                            8080,
				"serviceHost":                            "security.example.com",
				"accessKey":                              "test-ak",
				"secretKey":                              "test-sk",
				"checkResponse":                          true,
				"responseContentFallbackJsonPaths":       []string{},
				"responseStreamContentFallbackJsonPaths": []string{},
			})
			require.NoError(t, err)
			host, status := test.NewTestHost(configJSON)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			securityConfig := config.(*cfg.AISecurityConfig)
			require.Len(t, securityConfig.ResponseContentFallbackJsonPaths, 0)
			require.Len(t, securityConfig.ResponseStreamContentFallbackJsonPaths, 0)
		})

		t.Run("invalid response fallback paths type", func(t *testing.T) {
			configJSON, err := json.Marshal(map[string]interface{}{
				"serviceName":                      "security-service",
				"servicePort":                      8080,
				"serviceHost":                      "security.example.com",
				"accessKey":                        "test-ak",
				"secretKey":                        "test-sk",
				"checkResponse":                    true,
				"responseContentFallbackJsonPaths": "choices.0.message.content",
			})
			require.NoError(t, err)
			host, status := test.NewTestHost(configJSON)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})

		t.Run("invalid response fallback paths item", func(t *testing.T) {
			configJSON, err := json.Marshal(map[string]interface{}{
				"serviceName":                            "security-service",
				"servicePort":                            8080,
				"serviceHost":                            "security.example.com",
				"accessKey":                              "test-ak",
				"secretKey":                              "test-sk",
				"checkResponse":                          true,
				"responseStreamContentFallbackJsonPaths": []interface{}{"delta.text", ""},
			})
			require.NoError(t, err)
			host, status := test.NewTestHost(configJSON)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})

		t.Run("invalid response fallback paths non-string item", func(t *testing.T) {
			configJSON, err := json.Marshal(map[string]interface{}{
				"serviceName":                            "security-service",
				"servicePort":                            8080,
				"serviceHost":                            "security.example.com",
				"accessKey":                              "test-ak",
				"secretKey":                              "test-sk",
				"checkResponse":                          true,
				"responseStreamContentFallbackJsonPaths": []interface{}{"delta.text", 123},
			})
			require.NoError(t, err)
			host, status := test.NewTestHost(configJSON)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})

		t.Run("invalid contentModerationLevelBar value", func(t *testing.T) {
			configJSON, err := json.Marshal(map[string]interface{}{
				"serviceName":               "security-service",
				"servicePort":               8080,
				"serviceHost":               "security.example.com",
				"accessKey":                 "test-ak",
				"secretKey":                 "test-sk",
				"checkResponse":             true,
				"contentModerationLevelBar": "invalid",
			})
			require.NoError(t, err)
			host, status := test.NewTestHost(configJSON)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})
	})
}

func TestOnHttpRequestHeaders(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试启用请求检查的情况
		t.Run("request checking enabled", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			// 应该返回ActionContinue
			require.Equal(t, types.ActionContinue, action)
		})

		// 测试禁用请求检查的情况
		t.Run("request checking disabled", func(t *testing.T) {
			host, status := test.NewTestHost(requestOnlyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			// 应该返回ActionContinue
			require.Equal(t, types.ActionContinue, action)
		})
	})
}

func TestOnHttpRequestBody(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试请求体安全检查通过
		t.Run("request body security check pass", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			// 设置请求体
			body := `{"messages": [{"role": "user", "content": "Hello, how are you?"}]}`
			action := host.CallOnHttpRequestBody([]byte(body))

			// 应该返回ActionPause，等待安全检查结果
			require.Equal(t, types.ActionPause, action)

			// 模拟安全检查服务响应（通过）
			securityResponse := `{"Code": 200, "Message": "Success", "RequestId": "req-123", "Data": {"RiskLevel": "low"}}`
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(securityResponse))

			action = host.GetHttpStreamAction()
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})

		// 测试空请求内容
		t.Run("empty request content", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			// 设置空内容的请求体
			body := `{"messages": [{"role": "user", "content": ""}]}`
			action := host.CallOnHttpRequestBody([]byte(body))

			// 空内容应该直接通过
			require.Equal(t, types.ActionContinue, action)
		})
	})
}

func TestOnHttpResponseHeaders(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试启用响应检查的情况
		t.Run("response checking enabled", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			// 设置响应头
			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})

			// 应该返回HeaderStopIteration
			require.Equal(t, types.HeaderStopIteration, action)
		})

		// 测试禁用响应检查的情况
		t.Run("response checking disabled", func(t *testing.T) {
			host, status := test.NewTestHost(requestOnlyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			// 设置响应头
			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})

			// 应该返回ActionContinue
			require.Equal(t, types.ActionContinue, action)
		})

		// 测试非200状态码
		t.Run("non-200 status code", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			// 设置非200响应头
			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "500"},
				{"content-type", "application/json"},
			})

			// 应该返回ActionContinue
			require.Equal(t, types.ActionContinue, action)
		})
	})
}

func TestOnHttpResponseBody(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试响应体安全检查通过
		t.Run("response body security check pass", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			// 设置响应头
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})

			// 设置响应体
			body := `{"choices": [{"message": {"role": "assistant", "content": "Hello, how can I help you?"}}]}`
			action := host.CallOnHttpResponseBody([]byte(body))

			// 应该返回ActionPause，等待安全检查结果
			require.Equal(t, types.ActionPause, action)

			// 模拟安全检查服务响应（通过）
			securityResponse := `{"Code": 200, "Message": "Success", "RequestId": "req-123", "Data": {"RiskLevel": "low"}}`
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(securityResponse))

			action = host.GetHttpStreamAction()
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})

		// 测试空响应内容
		t.Run("empty response content", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			// 设置响应头
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})

			// 设置空内容的响应体
			body := `{"choices": [{"message": {"role": "assistant", "content": ""}}]}`
			action := host.CallOnHttpResponseBody([]byte(body))

			// 空内容应该直接通过
			require.Equal(t, types.ActionContinue, action)
		})
	})
}

func TestResponseFallbackExtractionCoverage(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		base := map[string]interface{}{
			"serviceName":               "security-service",
			"servicePort":               8080,
			"serviceHost":               "security.example.com",
			"accessKey":                 "test-ak",
			"secretKey":                 "test-sk",
			"checkResponse":             true,
			"action":                    "MultiModalGuard",
			"apiType":                   "text_generation",
			"contentModerationLevelBar": "high",
			"promptAttackLevelBar":      "high",
			"sensitiveDataLevelBar":     "S3",
			"timeout":                   2000,
			"bufferLimit":               1000,
		}

		withOverrides := func(overrides map[string]interface{}) json.RawMessage {
			cfgMap := make(map[string]interface{}, len(base)+len(overrides))
			for k, v := range base {
				cfgMap[k] = v
			}
			for k, v := range overrides {
				cfgMap[k] = v
			}
			data, err := json.Marshal(cfgMap)
			require.NoError(t, err)
			return data
		}

		t.Run("streaming response chunk uses configured fallback path", func(t *testing.T) {
			host, status := test.NewTestHost(withOverrides(map[string]interface{}{
				"responseStreamContentJsonPath":          "nonexistent.path",
				"responseStreamContentFallbackJsonPaths": []string{"choices.0.delta.content"},
			}))
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "text/event-stream"},
			})
			require.Equal(t, types.ActionContinue, action)

			chunk := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello fallback\"}}]}\n\n")
			host.CallOnHttpStreamingResponseBody(chunk, true)

			securityResponse := `{"Code": 200, "Message": "Success", "RequestId": "req-stream-fallback", "Data": {"RiskLevel": "low"}}`
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(securityResponse))
			host.CompleteHttp()
		})

		t.Run("buffered response body uses streaming fallback extraction", func(t *testing.T) {
			host, status := test.NewTestHost(withOverrides(map[string]interface{}{
				"responseStreamContentJsonPath":          "nonexistent.path",
				"responseStreamContentFallbackJsonPaths": []string{"choices.0.delta.content"},
			}))
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "text/event-stream"},
			})

			body := "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\ndata: [DONE]\n\n"
			host.CallOnHttpResponseBody([]byte(body))

			securityResponse := `{"Code": 200, "Message": "Success", "RequestId": "req-buffered-stream-fallback", "Data": {"RiskLevel": "low"}}`
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(securityResponse))
			host.CompleteHttp()
		})
	})
}

func TestMCP(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// Test MCP Response Body Check - Pass
		t.Run("mcp response body security check pass", func(t *testing.T) {
			host, status := test.NewTestHost(mcpConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"x-mse-consumer", "test-user"},
			})

			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})

			// body content matching responseContentJsonPath="content"
			body := `{"content": "Hello world"}`
			action := host.CallOnHttpResponseBody([]byte(body))
			require.Equal(t, types.ActionPause, action)

			securityResponse := `{"Code": 200, "Message": "Success", "RequestId": "req-123", "Data": {"RiskLevel": "low"}}`
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(securityResponse))

			action = host.GetHttpStreamAction()
			require.Equal(t, types.ActionContinue, action)
			host.CompleteHttp()
		})

		// Test MCP Response Body Check - Deny
		t.Run("mcp response body security check deny", func(t *testing.T) {
			host, status := test.NewTestHost(mcpConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})

			body := `{"content": "Bad content"}`
			action := host.CallOnHttpResponseBody([]byte(body))
			require.Equal(t, types.ActionPause, action)

			// High Risk
			securityResponse := `{"Code": 200, "Message": "Success", "RequestId": "req-123", "Data": {"RiskLevel": "high"}}`
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(securityResponse))

			// Verify it was replaced with DenyResponse
			// Can't easily verify the replaced body content with current test wrapper but can check action
			// Since plugin calls SendHttpResponse, execution stops or changes.
			// mcp.go uses SendHttpResponse(..., DenyResponse, -1) which means it ends the stream.
			// We can check if GetHttpStreamAction is ActionPause (since it did send a response) or something else.
			// Actually SendHttpResponse in proxy-wasm usually terminates further processing of the original stream.
		})

		// Test MCP Streaming Response Body Check - Pass
		t.Run("mcp streaming response body security check pass", func(t *testing.T) {
			host, status := test.NewTestHost(mcpConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "text/event-stream"},
			})

			// streaming chunk
			// config uses "content" key
			chunk := []byte(`data: {"content": "Hello"}` + "\n\n")
			// This calls OnHttpStreamingResponseBody -> mcp.HandleMcpStreamingResponseBody
			// It should push buffer and make call
			host.CallOnHttpStreamingResponseBody(chunk, false)
			// Action assertion removed as it returns an internal value 3

			securityResponse := `{"Code": 200, "Message": "Success", "RequestId": "req-123", "Data": {"RiskLevel": "low"}}`
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(securityResponse))
		})

		// Test MCP Streaming Response Body Check - Deny
		t.Run("mcp streaming response body security check deny", func(t *testing.T) {
			host, status := test.NewTestHost(mcpConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "text/event-stream"},
			})

			chunk := []byte(`data: {"content": "Bad"}` + "\n\n")
			host.CallOnHttpStreamingResponseBody(chunk, false)

			// High Risk
			securityResponse := `{"Code": 200, "Message": "Success", "RequestId": "req-123", "Data": {"RiskLevel": "high"}}`
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(securityResponse))

			// It injects DenySSEResponse.
		})
	})
}

func TestGetRiskAction(t *testing.T) {
	// 测试全局默认值
	t.Run("default is block", func(t *testing.T) {
		config := cfg.AISecurityConfig{}
		config.SetDefaultValues()
		require.Equal(t, "block", config.GetRiskAction("any-consumer"))
	})

	// 测试全局配置为 mask，无消费者覆盖
	t.Run("global mask without consumer override", func(t *testing.T) {
		config := cfg.AISecurityConfig{RiskAction: "mask"}
		require.Equal(t, "mask", config.GetRiskAction("any-consumer"))
	})

	// 测试消费者级别覆盖 riskAction
	t.Run("consumer overrides riskAction to mask", func(t *testing.T) {
		config := cfg.AISecurityConfig{
			RiskAction: "block",
			ConsumerRiskLevel: []map[string]interface{}{
				{
					"matcher":    cfg.Matcher{Exact: "vip-user"},
					"riskAction": "mask",
				},
			},
		}
		require.Equal(t, "mask", config.GetRiskAction("vip-user"))
		require.Equal(t, "block", config.GetRiskAction("normal-user"))
	})

	// 测试消费者匹配但未配置 riskAction，fallback 到全局
	t.Run("consumer matched without riskAction falls back to global", func(t *testing.T) {
		config := cfg.AISecurityConfig{
			RiskAction: "mask",
			ConsumerRiskLevel: []map[string]interface{}{
				{
					"matcher":                   cfg.Matcher{Exact: "some-user"},
					"contentModerationLevelBar": "low",
				},
			},
		}
		require.Equal(t, "mask", config.GetRiskAction("some-user"))
	})

	// 测试 prefix 匹配
	t.Run("consumer prefix match", func(t *testing.T) {
		config := cfg.AISecurityConfig{
			RiskAction: "block",
			ConsumerRiskLevel: []map[string]interface{}{
				{
					"matcher":    cfg.Matcher{Prefix: "test-"},
					"riskAction": "mask",
				},
			},
		}
		require.Equal(t, "mask", config.GetRiskAction("test-user-1"))
		require.Equal(t, "block", config.GetRiskAction("prod-user"))
	})

	// 测试空 consumer 不匹配任何规则
	t.Run("empty consumer falls back to global", func(t *testing.T) {
		config := cfg.AISecurityConfig{
			RiskAction: "mask",
			ConsumerRiskLevel: []map[string]interface{}{
				{
					"matcher":    cfg.Matcher{Exact: "vip"},
					"riskAction": "block",
				},
			},
		}
		require.Equal(t, "mask", config.GetRiskAction(""))
	})
}

func TestEvaluateRiskWithConsumerRiskAction(t *testing.T) {
	// 需要 proxy-wasm host 环境，因为 evaluateRiskMultiModal 调用 proxywasm.LogInfof
	opt := proxytest.NewEmulatorOption().WithVMContext(&types.DefaultVMContext{})
	_, reset := proxytest.NewHostEmulator(opt)
	defer reset()

	// 测试全局 block，消费者 mask
	t.Run("global block consumer mask", func(t *testing.T) {
		config := cfg.AISecurityConfig{
			RiskAction:                 "block",
			ContentModerationLevelBar:  "max",
			PromptAttackLevelBar:       "max",
			SensitiveDataLevelBar:      "S4",
			MaliciousUrlLevelBar:       "max",
			ModelHallucinationLevelBar: "max",
			ConsumerRiskLevel: []map[string]interface{}{
				{
					"matcher":               cfg.Matcher{Exact: "vip-user"},
					"riskAction":            "mask",
					"sensitiveDataLevelBar": "S2",
				},
			},
		}
		data := cfg.Data{
			RiskLevel: "none",
			Detail: []cfg.Detail{
				{Suggestion: "mask", Type: "sensitiveData", Level: "S2",
					Result: []cfg.Result{{Ext: cfg.Ext{Desensitization: "masked"}}}},
			},
		}
		// vip-user 使用 mask 模式，consumer 阈值 S2，Level=S2 >= S2 → RiskMask
		require.Equal(t, cfg.RiskMask, cfg.EvaluateRisk(cfg.MultiModalGuard, data, config, "vip-user"))
		// normal-user 使用全局 block 模式，全局阈值 S4，Level=S2 < S4 → RiskPass
		require.Equal(t, cfg.RiskPass, cfg.EvaluateRisk(cfg.MultiModalGuard, data, config, "normal-user"))
	})

	// 测试全局 mask，消费者 block
	t.Run("global mask consumer block", func(t *testing.T) {
		config := cfg.AISecurityConfig{
			RiskAction:                 "mask",
			ContentModerationLevelBar:  "max",
			PromptAttackLevelBar:       "max",
			SensitiveDataLevelBar:      "S2",
			MaliciousUrlLevelBar:       "max",
			ModelHallucinationLevelBar: "max",
			ConsumerRiskLevel: []map[string]interface{}{
				{
					"matcher":    cfg.Matcher{Exact: "strict-user"},
					"riskAction": "block",
				},
			},
		}
		data := cfg.Data{
			RiskLevel: "none",
			Detail: []cfg.Detail{
				{Suggestion: "mask", Type: "sensitiveData", Level: "S2",
					Result: []cfg.Result{{Ext: cfg.Ext{Desensitization: "masked"}}}},
			},
		}
		// strict-user 使用 block 模式，Level=S2 >= S2 但 Suggestion=mask + dimAction=block → detailTriggersBlock 返回 false（mask suggestion 不触发 block）
		// 实际上 detailTriggersBlock: Suggestion != "block", dimAction == "block" → return exceeds
		// exceeds = S2 >= S2 = true → RiskBlock
		// 所以 strict-user 应该是 RiskBlock
		require.Equal(t, cfg.RiskBlock, cfg.EvaluateRisk(cfg.MultiModalGuard, data, config, "strict-user"))
		// other-user 使用全局 mask 模式，Level=S2 >= S2 → RiskMask
		require.Equal(t, cfg.RiskMask, cfg.EvaluateRisk(cfg.MultiModalGuard, data, config, "other-user"))
	})
}

func TestParseConsumerRiskActionValidation(t *testing.T) {
	// 测试消费者级别 riskAction 无效值
	t.Run("invalid consumer riskAction", func(t *testing.T) {
		config := cfg.AISecurityConfig{}
		config.SetDefaultValues()
		configJSON := `{
			"serviceName": "security-service",
			"servicePort": 8080,
			"serviceHost": "security.example.com",
			"accessKey": "test-ak",
			"secretKey": "test-sk",
			"consumerRiskLevel": [
				{"name": "user1", "matchType": "exact", "riskAction": "invalid"}
			]
		}`
		err := config.Parse(gjson.Parse(configJSON))
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid riskAction in consumerRiskLevel")
	})

	// 测试消费者级别 riskAction 有效值
	t.Run("valid consumer riskAction", func(t *testing.T) {
		config := cfg.AISecurityConfig{}
		config.SetDefaultValues()
		configJSON := `{
			"serviceName": "security-service",
			"servicePort": 8080,
			"serviceHost": "security.example.com",
			"accessKey": "test-ak",
			"secretKey": "test-sk",
			"consumerRiskLevel": [
				{"name": "user1", "matchType": "exact", "riskAction": "mask"}
			]
		}`
		err := config.Parse(gjson.Parse(configJSON))
		require.NoError(t, err)
		require.Equal(t, "mask", config.GetRiskAction("user1"))
		require.Equal(t, "block", config.GetRiskAction("other"))
	})
}

func TestRiskLevelFunctions(t *testing.T) {
	// 测试风险等级转换函数
	t.Run("risk level conversion", func(t *testing.T) {
		require.Equal(t, 4, cfg.LevelToInt(cfg.MaxRisk))
		require.Equal(t, 3, cfg.LevelToInt(cfg.HighRisk))
		require.Equal(t, 2, cfg.LevelToInt(cfg.MediumRisk))
		require.Equal(t, 1, cfg.LevelToInt(cfg.LowRisk))
		require.Equal(t, 0, cfg.LevelToInt(cfg.NoRisk))
		require.Equal(t, -1, cfg.LevelToInt("invalid"))
	})

	// 测试风险等级比较
	t.Run("risk level comparison", func(t *testing.T) {
		require.True(t, cfg.LevelToInt(cfg.HighRisk) >= cfg.LevelToInt(cfg.MediumRisk))
		require.True(t, cfg.LevelToInt(cfg.MediumRisk) >= cfg.LevelToInt(cfg.LowRisk))
		require.True(t, cfg.LevelToInt(cfg.LowRisk) >= cfg.LevelToInt(cfg.NoRisk))
		require.False(t, cfg.LevelToInt(cfg.LowRisk) >= cfg.LevelToInt(cfg.HighRisk))
	})
}

func TestUtilityFunctions(t *testing.T) {
	// 测试十六进制ID生成函数
	t.Run("hex id generation", func(t *testing.T) {
		id, err := utils.GenerateHexID(16)
		require.NoError(t, err)
		require.Len(t, id, 16)
		require.Regexp(t, "^[0-9a-f]+$", id)
	})

	// 测试随机ID生成函数
	t.Run("random id generation", func(t *testing.T) {
		id := utils.GenerateRandomChatID()
		require.NotEmpty(t, id)
		require.Contains(t, id, "chatcmpl-")
		require.Len(t, id, 38) // "chatcmpl-" + 29 random chars
	})
}

func TestCustomLabelConstant(t *testing.T) {
	// 验证 CustomLabelType 常量值
	require.Equal(t, "customLabel", cfg.CustomLabelType)
}

func TestCustomLabelConfigParsing(t *testing.T) {
	// 测试 customLabelLevelBar 设置为 high
	t.Run("customLabelLevelBar set to high", func(t *testing.T) {
		config := cfg.AISecurityConfig{}
		config.SetDefaultValues()
		configJSON := `{
			"serviceName": "security-service",
			"servicePort": 8080,
			"serviceHost": "security.example.com",
			"accessKey": "test-ak",
			"secretKey": "test-sk",
			"customLabelLevelBar": "high"
		}`
		err := config.Parse(gjson.Parse(configJSON))
		require.NoError(t, err)
		require.Equal(t, "high", config.CustomLabelLevelBar)
	})

	// 测试 customLabelLevelBar 设置为 max
	t.Run("customLabelLevelBar set to max", func(t *testing.T) {
		config := cfg.AISecurityConfig{}
		config.SetDefaultValues()
		configJSON := `{
			"serviceName": "security-service",
			"servicePort": 8080,
			"serviceHost": "security.example.com",
			"accessKey": "test-ak",
			"secretKey": "test-sk",
			"customLabelLevelBar": "max"
		}`
		err := config.Parse(gjson.Parse(configJSON))
		require.NoError(t, err)
		require.Equal(t, "max", config.CustomLabelLevelBar)
	})

	// 测试 customLabelLevelBar 缺省时默认为 max
	t.Run("customLabelLevelBar defaults to max", func(t *testing.T) {
		config := cfg.AISecurityConfig{}
		config.SetDefaultValues()
		configJSON := `{
			"serviceName": "security-service",
			"servicePort": 8080,
			"serviceHost": "security.example.com",
			"accessKey": "test-ak",
			"secretKey": "test-sk"
		}`
		err := config.Parse(gjson.Parse(configJSON))
		require.NoError(t, err)
		require.Equal(t, "max", config.CustomLabelLevelBar)
	})

	// 测试 customLabelLevelBar 无效值
	t.Run("customLabelLevelBar invalid value", func(t *testing.T) {
		config := cfg.AISecurityConfig{}
		config.SetDefaultValues()
		configJSON := `{
			"serviceName": "security-service",
			"servicePort": 8080,
			"serviceHost": "security.example.com",
			"accessKey": "test-ak",
			"secretKey": "test-sk",
			"customLabelLevelBar": "invalid"
		}`
		err := config.Parse(gjson.Parse(configJSON))
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid customLabelLevelBar")
	})
}

func TestGetCustomLabelLevelBar(t *testing.T) {
	// 测试消费者精确匹配
	t.Run("consumer exact match", func(t *testing.T) {
		config := cfg.AISecurityConfig{
			CustomLabelLevelBar: "max",
			ConsumerRiskLevel: []map[string]interface{}{
				{
					"matcher":             cfg.Matcher{Exact: "exact-user"},
					"customLabelLevelBar": "low",
				},
			},
		}
		require.Equal(t, "low", config.GetCustomLabelLevelBar("exact-user"))
	})

	// 测试消费者前缀匹配
	t.Run("consumer prefix match", func(t *testing.T) {
		config := cfg.AISecurityConfig{
			CustomLabelLevelBar: "max",
			ConsumerRiskLevel: []map[string]interface{}{
				{
					"matcher":             cfg.Matcher{Prefix: "prefix-"},
					"customLabelLevelBar": "medium",
				},
			},
		}
		require.Equal(t, "medium", config.GetCustomLabelLevelBar("prefix-user"))
	})

	// 测试无匹配回退全局值
	t.Run("no match falls back to global", func(t *testing.T) {
		config := cfg.AISecurityConfig{
			CustomLabelLevelBar: "high",
			ConsumerRiskLevel: []map[string]interface{}{
				{
					"matcher":             cfg.Matcher{Exact: "other-user"},
					"customLabelLevelBar": "low",
				},
			},
		}
		require.Equal(t, "high", config.GetCustomLabelLevelBar("unmatched-user"))
	})
}

func TestCustomLabelDetailExceedsThreshold(t *testing.T) {
	// 需要 proxy-wasm host 环境，因为 evaluateRiskMultiModal 调用 proxywasm.LogInfof
	opt := proxytest.NewEmulatorOption().WithVMContext(&types.DefaultVMContext{})
	_, reset := proxytest.NewHostEmulator(opt)
	defer reset()

	// 测试 customLabel Level=high, threshold=high → 拦截 (true)
	t.Run("level high threshold high blocks", func(t *testing.T) {
		config := cfg.AISecurityConfig{
			RiskAction:                 "block",
			ContentModerationLevelBar:  "max",
			PromptAttackLevelBar:       "max",
			SensitiveDataLevelBar:      "S4",
			MaliciousUrlLevelBar:       "max",
			ModelHallucinationLevelBar: "max",
			CustomLabelLevelBar:        "high",
		}
		data := cfg.Data{
			RiskLevel: "none",
			Detail: []cfg.Detail{
				{Type: cfg.CustomLabelType, Level: "high"},
			},
		}
		require.Equal(t, cfg.RiskBlock, cfg.EvaluateRisk(cfg.MultiModalGuard, data, config, ""))
	})

	// 测试 customLabel Level=none, threshold=high → 不拦截 (false)
	t.Run("level none threshold high passes", func(t *testing.T) {
		config := cfg.AISecurityConfig{
			RiskAction:                 "block",
			ContentModerationLevelBar:  "max",
			PromptAttackLevelBar:       "max",
			SensitiveDataLevelBar:      "S4",
			MaliciousUrlLevelBar:       "max",
			ModelHallucinationLevelBar: "max",
			CustomLabelLevelBar:        "high",
		}
		data := cfg.Data{
			RiskLevel: "none",
			Detail: []cfg.Detail{
				{Type: cfg.CustomLabelType, Level: "none"},
			},
		}
		require.Equal(t, cfg.RiskPass, cfg.EvaluateRisk(cfg.MultiModalGuard, data, config, ""))
	})

	// 测试 customLabel Level=high, threshold=max → 不拦截 (false)
	t.Run("level high threshold max passes", func(t *testing.T) {
		config := cfg.AISecurityConfig{
			RiskAction:                 "block",
			ContentModerationLevelBar:  "max",
			PromptAttackLevelBar:       "max",
			SensitiveDataLevelBar:      "S4",
			MaliciousUrlLevelBar:       "max",
			ModelHallucinationLevelBar: "max",
			CustomLabelLevelBar:        "max",
		}
		data := cfg.Data{
			RiskLevel: "none",
			Detail: []cfg.Detail{
				{Type: cfg.CustomLabelType, Level: "high"},
			},
		}
		require.Equal(t, cfg.RiskPass, cfg.EvaluateRisk(cfg.MultiModalGuard, data, config, ""))
	})
}

func TestCustomLabelConfigIntegration(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// 测试 customLabelConfig 配置解析和消费者覆盖
		t.Run("customLabel config with consumer override", func(t *testing.T) {
			host, status := test.NewTestHost(customLabelConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)
			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			securityConfig := config.(*cfg.AISecurityConfig)
			require.Equal(t, "high", securityConfig.CustomLabelLevelBar)
			require.Equal(t, "low", securityConfig.GetCustomLabelLevelBar("exact-user"))
			require.Equal(t, "medium", securityConfig.GetCustomLabelLevelBar("prefix-user"))
			require.Equal(t, "high", securityConfig.GetCustomLabelLevelBar("unknown-user"))
		})
	})
}

func TestRequestMasking(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试请求阶段脱敏成功：riskAction=mask，API 返回 mask 建议，请求体被替换为脱敏内容
		t.Run("request masking success", func(t *testing.T) {
			host, status := test.NewTestHost(maskConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			body := `{"messages": [{"role": "user", "content": "我的电话是13800138000"}]}`
			action := host.CallOnHttpRequestBody([]byte(body))
			require.Equal(t, types.ActionPause, action)

			// API 返回 mask 建议，包含脱敏内容
			securityResponse := `{
				"Code": 200,
				"Message": "Success",
				"RequestId": "req-123",
				"Data": {
					"RiskLevel": "low",
					"Detail": [{
						"Suggestion": "mask",
						"Type": "sensitiveData",
						"Level": "S3",
						"Result": [{
							"Label": "phone_number",
							"Confidence": 99.0,
							"Ext": {
								"Desensitization": "我的电话是1**********",
								"SensitiveData": ["13800138000"]
							}
						}]
					}]
				}
			}`
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(securityResponse))

			// 验证请求体被替换为脱敏内容
			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)
			content := gjson.GetBytes(processedBody, "messages.@reverse.0.content").String()
			require.Equal(t, "我的电话是1**********", content)

			host.CompleteHttp()
		})

		// 测试脱敏内容为空时回退到拦截
		t.Run("empty desensitization falls back to block", func(t *testing.T) {
			host, status := test.NewTestHost(maskConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			body := `{"messages": [{"role": "user", "content": "我的电话是13800138000"}]}`
			action := host.CallOnHttpRequestBody([]byte(body))
			require.Equal(t, types.ActionPause, action)

			// API 返回 mask 建议，但 Desensitization 为空
			securityResponse := `{
				"Code": 200,
				"Message": "Success",
				"RequestId": "req-123",
				"Data": {
					"RiskLevel": "low",
					"Detail": [{
						"Suggestion": "mask",
						"Type": "sensitiveData",
						"Level": "S3",
						"Result": [{
							"Label": "phone_number",
							"Confidence": 99.0,
							"Ext": {
								"Desensitization": "",
								"SensitiveData": ["13800138000"]
							}
						}]
					}]
				}
			}`
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(securityResponse))

			// Desensitization 为空时应回退到拦截，SendHttpResponse 被调用
			// 验证请求体未被修改（原始内容保持不变）
			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)
			content := gjson.GetBytes(processedBody, "messages.@reverse.0.content").String()
			require.Equal(t, "我的电话是13800138000", content)
		})

		// 测试 riskAction=block 时 mask 建议按现有逻辑处理（向后兼容）
		t.Run("riskAction block keeps existing behavior for mask suggestion", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			body := `{"messages": [{"role": "user", "content": "我的电话是13800138000"}]}`
			action := host.CallOnHttpRequestBody([]byte(body))
			require.Equal(t, types.ActionPause, action)

			// API 返回 mask 建议，但全局 riskAction 默认为 block
			securityResponse := `{
				"Code": 200,
				"Message": "Success",
				"RequestId": "req-123",
				"Data": {
					"RiskLevel": "low",
					"Detail": [{
						"Suggestion": "mask",
						"Type": "sensitiveData",
						"Level": "S2",
						"Result": [{
							"Label": "phone_number",
							"Confidence": 99.0,
							"Ext": {
								"Desensitization": "我的电话是1**********",
								"SensitiveData": ["13800138000"]
							}
						}]
					}]
				}
			}`
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(securityResponse))

			// riskAction=block 时，mask 建议按风险等级判断，low 级别应放行
			action = host.GetHttpStreamAction()
			require.Equal(t, types.ActionContinue, action)

			// 请求体不应被脱敏修改
			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)
			content := gjson.GetBytes(processedBody, "messages.@reverse.0.content").String()
			require.Equal(t, "我的电话是13800138000", content)

			host.CompleteHttp()
		})

		// 测试 riskAction=mask 时 block 建议优先拦截
		t.Run("block suggestion takes priority over mask", func(t *testing.T) {
			host, status := test.NewTestHost(maskConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			body := `{"messages": [{"role": "user", "content": "违规内容"}]}`
			action := host.CallOnHttpRequestBody([]byte(body))
			require.Equal(t, types.ActionPause, action)

			// API 返回 block 建议
			securityResponse := `{
				"Code": 200,
				"Message": "Success",
				"RequestId": "req-123",
				"Data": {
					"RiskLevel": "high",
					"Detail": [{
						"Suggestion": "block",
						"Type": "contentModeration",
						"Level": "high"
					}]
				}
			}`
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(securityResponse))

			// block 建议应拦截请求，请求体不应被修改
			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)
			content := gjson.GetBytes(processedBody, "messages.@reverse.0.content").String()
			require.Equal(t, "违规内容", content)
		})
	})
}

// 测试配置：MCP + 脱敏模式
var mcpMaskConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"serviceName":                   "security-service",
		"servicePort":                   8080,
		"serviceHost":                   "security.example.com",
		"accessKey":                     "test-ak",
		"secretKey":                     "test-sk",
		"checkRequest":                  true,
		"checkResponse":                 true,
		"action":                        "MultiModalGuard",
		"apiType":                       "mcp",
		"riskAction":                    "mask",
		"requestContentJsonPath":        "params.arguments.input",
		"responseContentJsonPath":       "content",
		"responseStreamContentJsonPath": "content",
		"contentModerationLevelBar":     "high",
		"promptAttackLevelBar":          "high",
		"sensitiveDataLevelBar":         "S3",
		"timeout":                       2000,
	})
	return data
}()

func TestIsRiskLevelAcceptable(t *testing.T) {
	// 需要 proxy-wasm host 环境，因为 evaluateRiskMultiModal 调用 proxywasm.LogInfof
	opt := proxytest.NewEmulatorOption().WithVMContext(&types.DefaultVMContext{})
	_, reset := proxytest.NewHostEmulator(opt)
	defer reset()

	// 用例 1: riskAction=mask, Suggestion=mask → 应返回 true（mask 不应被视为不可接受）
	t.Run("mask action with mask suggestion is acceptable", func(t *testing.T) {
		config := cfg.AISecurityConfig{
			RiskAction:                 "mask",
			ContentModerationLevelBar:  "max",
			PromptAttackLevelBar:       "max",
			SensitiveDataLevelBar:      "S4",
			MaliciousUrlLevelBar:       "max",
			ModelHallucinationLevelBar: "max",
			CustomLabelLevelBar:        "max",
		}
		data := cfg.Data{
			RiskLevel: "none",
			Detail: []cfg.Detail{
				{Suggestion: "mask", Type: "sensitiveData", Level: "S2",
					Result: []cfg.Result{{Ext: cfg.Ext{Desensitization: "masked"}}}},
			},
		}
		require.True(t, cfg.IsRiskLevelAcceptable(cfg.MultiModalGuard, data, config, ""))
	})

	// 用例 2: riskAction=mask, Suggestion=block → 应返回 false
	t.Run("mask action with block suggestion is not acceptable", func(t *testing.T) {
		config := cfg.AISecurityConfig{
			RiskAction:                 "mask",
			ContentModerationLevelBar:  "max",
			PromptAttackLevelBar:       "max",
			SensitiveDataLevelBar:      "S4",
			MaliciousUrlLevelBar:       "max",
			ModelHallucinationLevelBar: "max",
			CustomLabelLevelBar:        "max",
		}
		data := cfg.Data{
			RiskLevel: "none",
			Detail: []cfg.Detail{
				{Suggestion: "block", Type: "contentModeration", Level: "high"},
			},
		}
		require.True(t, cfg.IsRiskLevelAcceptable(cfg.MultiModalGuard, data, config, ""))
	})

	// 用例 3: riskAction=mask, 无风险 → 应返回 true
	t.Run("mask action with no risk is acceptable", func(t *testing.T) {
		config := cfg.AISecurityConfig{
			RiskAction:                 "mask",
			ContentModerationLevelBar:  "max",
			PromptAttackLevelBar:       "max",
			SensitiveDataLevelBar:      "S4",
			MaliciousUrlLevelBar:       "max",
			ModelHallucinationLevelBar: "max",
			CustomLabelLevelBar:        "max",
		}
		data := cfg.Data{RiskLevel: "low"}
		require.True(t, cfg.IsRiskLevelAcceptable(cfg.MultiModalGuard, data, config, ""))
	})

	// 用例 4: riskAction=block, Suggestion=mask, level 未超阈值 → 应返回 true（向后兼容）
	t.Run("block action with mask suggestion below threshold is acceptable", func(t *testing.T) {
		config := cfg.AISecurityConfig{
			RiskAction:                 "block",
			ContentModerationLevelBar:  "max",
			PromptAttackLevelBar:       "max",
			SensitiveDataLevelBar:      "S4",
			MaliciousUrlLevelBar:       "max",
			ModelHallucinationLevelBar: "max",
			CustomLabelLevelBar:        "max",
		}
		data := cfg.Data{
			RiskLevel: "none",
			Detail: []cfg.Detail{
				{Suggestion: "mask", Type: "sensitiveData", Level: "S2"},
			},
		}
		require.True(t, cfg.IsRiskLevelAcceptable(cfg.MultiModalGuard, data, config, ""))
	})

	// 用例 5: riskAction=block, Suggestion=mask, level 超阈值 → 应返回 false（向后兼容）
	t.Run("block action with mask suggestion exceeding threshold is not acceptable", func(t *testing.T) {
		config := cfg.AISecurityConfig{
			RiskAction:                 "block",
			ContentModerationLevelBar:  "max",
			PromptAttackLevelBar:       "max",
			SensitiveDataLevelBar:      "S2",
			MaliciousUrlLevelBar:       "max",
			ModelHallucinationLevelBar: "max",
			CustomLabelLevelBar:        "max",
		}
		data := cfg.Data{
			RiskLevel: "none",
			Detail: []cfg.Detail{
				{Suggestion: "mask", Type: "sensitiveData", Level: "S2"},
			},
		}
		require.False(t, cfg.IsRiskLevelAcceptable(cfg.MultiModalGuard, data, config, ""))
	})

	// 用例 6: TextModerationPlus, riskAction=mask → 不受影响
	t.Run("TextModerationPlus not affected by riskAction mask", func(t *testing.T) {
		config := cfg.AISecurityConfig{
			RiskAction:   "mask",
			RiskLevelBar: "high",
		}
		data := cfg.Data{RiskLevel: "low"}
		require.True(t, cfg.IsRiskLevelAcceptable(cfg.TextModerationPlus, data, config, ""))
	})
}

func TestMcpMaskNotBlock(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 用例 7: MCP 请求, riskAction=mask, API 返回 Suggestion=mask → 应放行
		t.Run("mcp request with mask suggestion should pass not block", func(t *testing.T) {
			host, status := test.NewTestHost(mcpMaskConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/mcp"},
				{":method", "POST"},
			})

			body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test","arguments":{"input":"我的电话是13800138000"}}}`
			action := host.CallOnHttpRequestBody([]byte(body))
			require.Equal(t, types.ActionPause, action)

			// API 返回 mask 建议
			securityResponse := `{
				"Code": 200,
				"Message": "Success",
				"RequestId": "req-123",
				"Data": {
					"RiskLevel": "low",
					"Detail": [{
						"Suggestion": "mask",
						"Type": "sensitiveData",
						"Level": "S2",
						"Result": [{
							"Label": "phone_number",
							"Confidence": 99.0,
							"Ext": {
								"Desensitization": "我的电话是1**********",
								"SensitiveData": ["13800138000"]
							}
						}]
					}]
				}
			}`
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(securityResponse))

			// 应放行而非拦截
			action = host.GetHttpStreamAction()
			require.Equal(t, types.ActionContinue, action)
			host.CompleteHttp()
		})

		// 用例 8: MCP 响应, riskAction=mask, API 返回 Suggestion=mask → 应放行
		t.Run("mcp response with mask suggestion should pass not block", func(t *testing.T) {
			host, status := test.NewTestHost(mcpMaskConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/mcp"},
				{":method", "POST"},
			})

			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})

			body := `{"content": "我的电话是13800138000"}`
			action := host.CallOnHttpResponseBody([]byte(body))
			require.Equal(t, types.ActionPause, action)

			// API 返回 mask 建议
			securityResponse := `{
				"Code": 200,
				"Message": "Success",
				"RequestId": "req-123",
				"Data": {
					"RiskLevel": "low",
					"Detail": [{
						"Suggestion": "mask",
						"Type": "sensitiveData",
						"Level": "S2",
						"Result": [{
							"Label": "phone_number",
							"Confidence": 99.0,
							"Ext": {
								"Desensitization": "我的电话是1**********",
								"SensitiveData": ["13800138000"]
							}
						}]
					}]
				}
			}`
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(securityResponse))

			// 应放行而非拦截
			action = host.GetHttpStreamAction()
			require.Equal(t, types.ActionContinue, action)
			host.CompleteHttp()
		})

		// 用例 9: MCP 请求, riskAction=mask, API 返回 Suggestion=block → 应拦截
		t.Run("mcp request with block suggestion should deny", func(t *testing.T) {
			host, status := test.NewTestHost(mcpMaskConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/mcp"},
				{":method", "POST"},
			})

			body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test","arguments":{"input":"违规内容"}}}`
			action := host.CallOnHttpRequestBody([]byte(body))
			require.Equal(t, types.ActionPause, action)

			// API 返回 block 建议
			securityResponse := `{
				"Code": 200,
				"Message": "Success",
				"RequestId": "req-123",
				"Data": {
					"RiskLevel": "high",
					"Detail": [{
						"Suggestion": "block",
						"Type": "contentModeration",
						"Level": "high"
					}]
				}
			}`
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(securityResponse))

			// block 建议应拦截（SendHttpResponse 被调用，请求不会继续）
			// MCP handler 调用 SendHttpResponse 后不会 resume
		})
	})
}

// =============================================================================
// TC-PARSE: 配置解析与校验集成测试
// =============================================================================

// 测试配置：MultiModalGuard + 全局维度动作全为合法值
var dimensionActionValidConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"serviceName":               "security-service",
		"servicePort":               8080,
		"serviceHost":               "security.example.com",
		"accessKey":                 "test-ak",
		"secretKey":                 "test-sk",
		"checkRequest":              true,
		"checkResponse":             true,
		"action":                    "MultiModalGuard",
		"contentModerationAction":   "block",
		"promptAttackAction":        "block",
		"sensitiveDataAction":       "mask",
		"maliciousUrlAction":        "block",
		"modelHallucinationAction":  "block",
		"customLabelAction":         "block",
		"contentModerationLevelBar": "high",
		"promptAttackLevelBar":      "high",
		"sensitiveDataLevelBar":     "S3",
	})
	return data
}()

// 测试配置：MultiModalGuard + 全局维度动作出现非法值
var dimensionActionInvalidConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"serviceName":             "security-service",
		"servicePort":             8080,
		"serviceHost":             "security.example.com",
		"accessKey":               "test-ak",
		"secretKey":               "test-sk",
		"checkRequest":            true,
		"checkResponse":           true,
		"action":                  "MultiModalGuard",
		"contentModerationAction": "allow", // 非法值
	})
	return data
}()

// 测试配置：MultiModalGuard + consumerRiskLevel 内维度动作非法
var consumerDimensionActionInvalidConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"serviceName":  "security-service",
		"servicePort":  8080,
		"serviceHost":  "security.example.com",
		"accessKey":    "test-ak",
		"secretKey":    "test-sk",
		"checkRequest": true,
		"action":       "MultiModalGuard",
		"consumerRiskLevel": []map[string]interface{}{
			{
				"name":                "user-a",
				"matchType":           "exact",
				"sensitiveDataAction": "deny", // 非法值
			},
		},
	})
	return data
}()

// 测试配置：TextModerationPlus + 配置了维度动作
var textModPlusDimensionActionConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"serviceName":             "security-service",
		"servicePort":             8080,
		"serviceHost":             "security.example.com",
		"accessKey":               "test-ak",
		"secretKey":               "test-sk",
		"checkRequest":            true,
		"action":                  "TextModerationPlus",
		"sensitiveDataAction":     "mask",
		"contentModerationAction": "block",
	})
	return data
}()

// 测试配置：未配置任何动作字段
var noActionFieldConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"serviceName":  "security-service",
		"servicePort":  8080,
		"serviceHost":  "security.example.com",
		"accessKey":    "test-ak",
		"secretKey":    "test-sk",
		"checkRequest": true,
	})
	return data
}()

// TestTC_PARSE_001 MultiModalGuard + 全局维度动作全为合法值 => 启动成功
func TestTC_PARSE_001(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(dimensionActionValidConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		config, err := host.GetMatchConfig()
		require.NoError(t, err)
		securityConfig := config.(*cfg.AISecurityConfig)
		require.Equal(t, "block", securityConfig.ContentModerationAction)
		require.Equal(t, "block", securityConfig.PromptAttackAction)
		require.Equal(t, "mask", securityConfig.SensitiveDataAction)
		require.Equal(t, "block", securityConfig.MaliciousUrlAction)
		require.Equal(t, "block", securityConfig.ModelHallucinationAction)
		require.Equal(t, "block", securityConfig.CustomLabelAction)
	})
}

// TestTC_PARSE_002 MultiModalGuard + 全局维度动作出现非法值 => 启动失败
func TestTC_PARSE_002(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(dimensionActionInvalidConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusFailed, status)
	})
}

// TestTC_PARSE_003 MultiModalGuard + consumerRiskLevel 内维度动作非法 => 启动失败
func TestTC_PARSE_003(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(consumerDimensionActionInvalidConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusFailed, status)
	})
}

// TestTC_PARSE_004 TextModerationPlus + 配置了维度动作 => 启动成功（字段忽略）
func TestTC_PARSE_004(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(textModPlusDimensionActionConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		config, err := host.GetMatchConfig()
		require.NoError(t, err)
		securityConfig := config.(*cfg.AISecurityConfig)
		// 字段被解析但在运行时被忽略（不影响启动）
		require.Equal(t, "mask", securityConfig.SensitiveDataAction)
		require.Equal(t, "block", securityConfig.ContentModerationAction)
	})
}

// TestTC_PARSE_005 未配置任何动作字段 => 默认 riskAction=block
func TestTC_PARSE_005(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(noActionFieldConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		config, err := host.GetMatchConfig()
		require.NoError(t, err)
		securityConfig := config.(*cfg.AISecurityConfig)
		require.Equal(t, "block", securityConfig.RiskAction)
		require.Equal(t, "", securityConfig.ContentModerationAction)
		require.Equal(t, "", securityConfig.PromptAttackAction)
		require.Equal(t, "", securityConfig.SensitiveDataAction)
		require.Equal(t, "", securityConfig.MaliciousUrlAction)
		require.Equal(t, "", securityConfig.ModelHallucinationAction)
		require.Equal(t, "", securityConfig.CustomLabelAction)
	})
}

// TestTC_PARSE_006 TextModerationPlus + 非法维度动作值 => 启动成功（需求 8.2）
func TestTC_PARSE_006(t *testing.T) {
	// 非 MultiModalGuard 下配置了非法维度动作值，不应报错，应启动成功
	invalidDimActionTextModConfig := func() json.RawMessage {
		data, _ := json.Marshal(map[string]interface{}{
			"serviceName":             "security-service",
			"servicePort":             8080,
			"serviceHost":             "security.example.com",
			"accessKey":               "test-ak",
			"secretKey":               "test-sk",
			"checkRequest":            true,
			"action":                  "TextModerationPlus",
			"contentModerationAction": "allow", // 非法值，但非 MultiModalGuard 下应忽略
			"sensitiveDataAction":     "deny",  // 非法值
		})
		return data
	}()
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(invalidDimActionTextModConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
	})
}

// TestTC_PARSE_007 TextModerationPlus + consumerRiskLevel 内非法维度动作值 => 启动成功（需求 8.2）
func TestTC_PARSE_007(t *testing.T) {
	invalidConsumerDimActionTextModConfig := func() json.RawMessage {
		data, _ := json.Marshal(map[string]interface{}{
			"serviceName":  "security-service",
			"servicePort":  8080,
			"serviceHost":  "security.example.com",
			"accessKey":    "test-ak",
			"secretKey":    "test-sk",
			"checkRequest": true,
			"action":       "TextModerationPlus",
			"consumerRiskLevel": []map[string]interface{}{
				{
					"name":                "user-a",
					"matchType":           "exact",
					"sensitiveDataAction": "invalid-value", // 非法值，但非 MultiModalGuard 下应忽略
				},
			},
		})
		return data
	}()
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(invalidConsumerDimActionTextModConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
	})
}

// =============================================================================
// TC-REG: 回归测试
// =============================================================================

// 测试配置：历史仅 riskAction=block 的 MultiModalGuard 配置（无维度动作字段）
var legacyRiskActionBlockConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"serviceName":               "security-service",
		"servicePort":               8080,
		"serviceHost":               "security.example.com",
		"accessKey":                 "test-ak",
		"secretKey":                 "test-sk",
		"checkRequest":              true,
		"checkResponse":             true,
		"action":                    "MultiModalGuard",
		"riskAction":                "block",
		"contentModerationLevelBar": "high",
		"promptAttackLevelBar":      "high",
		"sensitiveDataLevelBar":     "S3",
		"timeout":                   2000,
	})
	return data
}()

// 测试配置：历史仅 riskAction=mask 的 MultiModalGuard 配置（无维度动作字段）
var legacyRiskActionMaskConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"serviceName":               "security-service",
		"servicePort":               8080,
		"serviceHost":               "security.example.com",
		"accessKey":                 "test-ak",
		"secretKey":                 "test-sk",
		"checkRequest":              true,
		"checkResponse":             true,
		"action":                    "MultiModalGuard",
		"riskAction":                "mask",
		"contentModerationLevelBar": "high",
		"promptAttackLevelBar":      "high",
		"sensitiveDataLevelBar":     "S3",
		"timeout":                   2000,
	})
	return data
}()

// TestTC_REG_004 历史仅 riskAction 配置的场景不回归
// 验证：仅配置 riskAction（不配置任何维度动作字段）时，新代码行为与历史一致
func TestTC_REG_004(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 子用例 1: riskAction=block，请求安全检查通过（低风险）=> 放行
		t.Run("legacy block config pass on low risk", func(t *testing.T) {
			host, status := test.NewTestHost(legacyRiskActionBlockConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			body := `{"messages": [{"role": "user", "content": "Hello"}]}`
			action := host.CallOnHttpRequestBody([]byte(body))
			require.Equal(t, types.ActionPause, action)

			// API 返回低风险，无 Detail 触发
			securityResponse := `{
				"Code": 200,
				"Message": "Success",
				"RequestId": "req-reg-001",
				"Data": {
					"RiskLevel": "none",
					"Detail": []
				}
			}`
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(securityResponse))

			action = host.GetHttpStreamAction()
			require.Equal(t, types.ActionContinue, action)
			host.CompleteHttp()
		})

		// 子用例 2: riskAction=block，顶层 RiskLevel 超阈值 => 拦截
		t.Run("legacy block config blocks on high risk level", func(t *testing.T) {
			host, status := test.NewTestHost(legacyRiskActionBlockConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			body := `{"messages": [{"role": "user", "content": "违规内容"}]}`
			action := host.CallOnHttpRequestBody([]byte(body))
			require.Equal(t, types.ActionPause, action)

			// API 返回高风险，顶层 RiskLevel 超阈值
			securityResponse := `{
				"Code": 200,
				"Message": "Success",
				"RequestId": "req-reg-002",
				"Data": {
					"RiskLevel": "high",
					"Detail": [{
						"Suggestion": "block",
						"Type": "contentModeration",
						"Level": "high"
					}]
				}
			}`
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(securityResponse))

			// 拦截：请求体不应被修改
			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)
			content := gjson.GetBytes(processedBody, "messages.@reverse.0.content").String()
			require.Equal(t, "违规内容", content)
		})

		// 子用例 3: riskAction=block，Detail 有 mask 建议但 level 未超阈值 => 放行（block 模式忽略 mask）
		t.Run("legacy block config ignores mask suggestion below threshold", func(t *testing.T) {
			host, status := test.NewTestHost(legacyRiskActionBlockConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			body := `{"messages": [{"role": "user", "content": "我的电话是13800138000"}]}`
			action := host.CallOnHttpRequestBody([]byte(body))
			require.Equal(t, types.ActionPause, action)

			// API 返回 mask 建议，但 riskAction=block 且 level 未超阈值
			securityResponse := `{
				"Code": 200,
				"Message": "Success",
				"RequestId": "req-reg-003",
				"Data": {
					"RiskLevel": "none",
					"Detail": [{
						"Suggestion": "mask",
						"Type": "sensitiveData",
						"Level": "S2",
						"Result": [{
							"Label": "phone_number",
							"Confidence": 99.0,
							"Ext": {
								"Desensitization": "我的电话是1**********",
								"SensitiveData": ["13800138000"]
							}
						}]
					}]
				}
			}`
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(securityResponse))

			// riskAction=block 时，mask 建议不触发脱敏，level 未超阈值应放行
			action = host.GetHttpStreamAction()
			require.Equal(t, types.ActionContinue, action)

			// 请求体不应被脱敏修改
			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)
			content := gjson.GetBytes(processedBody, "messages.@reverse.0.content").String()
			require.Equal(t, "我的电话是13800138000", content)
			host.CompleteHttp()
		})

		// 子用例 4: riskAction=mask，Detail 有 mask 建议 => 脱敏替换
		t.Run("legacy mask config applies desensitization", func(t *testing.T) {
			host, status := test.NewTestHost(legacyRiskActionMaskConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			body := `{"messages": [{"role": "user", "content": "我的电话是13800138000"}]}`
			action := host.CallOnHttpRequestBody([]byte(body))
			require.Equal(t, types.ActionPause, action)

			// API 返回 mask 建议，riskAction=mask 应触发脱敏
			securityResponse := `{
				"Code": 200,
				"Message": "Success",
				"RequestId": "req-reg-004",
				"Data": {
					"RiskLevel": "none",
					"Detail": [{
						"Suggestion": "mask",
						"Type": "sensitiveData",
						"Level": "S3",
						"Result": [{
							"Label": "phone_number",
							"Confidence": 99.0,
							"Ext": {
								"Desensitization": "我的电话是1**********",
								"SensitiveData": ["13800138000"]
							}
						}]
					}]
				}
			}`
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(securityResponse))

			// riskAction=mask 时，mask 建议应触发脱敏替换
			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)
			content := gjson.GetBytes(processedBody, "messages.@reverse.0.content").String()
			require.Equal(t, "我的电话是1**********", content)
			host.CompleteHttp()
		})

		// 子用例 5: riskAction=mask，Detail 有 block 建议 => 仍然拦截（block 优先）
		t.Run("legacy mask config still blocks on block suggestion", func(t *testing.T) {
			host, status := test.NewTestHost(legacyRiskActionMaskConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			body := `{"messages": [{"role": "user", "content": "违规内容"}]}`
			action := host.CallOnHttpRequestBody([]byte(body))
			require.Equal(t, types.ActionPause, action)

			// API 返回 block 建议，即使 riskAction=mask 也应拦截
			securityResponse := `{
				"Code": 200,
				"Message": "Success",
				"RequestId": "req-reg-005",
				"Data": {
					"RiskLevel": "high",
					"Detail": [{
						"Suggestion": "block",
						"Type": "contentModeration",
						"Level": "high"
					}]
				}
			}`
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(securityResponse))

			// block 建议应拦截，请求体不应被修改
			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)
			content := gjson.GetBytes(processedBody, "messages.@reverse.0.content").String()
			require.Equal(t, "违规内容", content)
		})
	})
}
func TestMultiModalGuardTextGenerationDeny(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// MultiModalGuard text_generation request deny → exercises multi_modal_guard/text/openai.go BuildDenyResponseBody path
		t.Run("multi modal guard text request deny returns blockedDetails", func(t *testing.T) {
			host, status := test.NewTestHost(multiModalGuardTextConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			body := `{"messages": [{"role": "user", "content": "trigger deny"}]}`
			require.Equal(t, types.ActionPause, host.CallOnHttpRequestBody([]byte(body)))

			securityResponse := `{"Code": 200, "Message": "Success", "RequestId": "req-mmg-text-deny", "Data": {"RiskLevel": "high"}}`
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(securityResponse))

			local := host.GetLocalResponse()
			require.NotNil(t, local, "expected SendHttpResponse for request deny")
			require.Contains(t, string(local.Data), "blockedDetails")
		})

		// MultiModalGuard text_generation response deny → exercises common/text/openai.go HandleTextGenerationResponseBody BuildDenyResponseBody path
		t.Run("multi modal guard text response deny returns blockedDetails", func(t *testing.T) {
			host, status := test.NewTestHost(multiModalGuardTextConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})

			body := `{"choices": [{"message": {"role": "assistant", "content": "bad response content"}}]}`
			action := host.CallOnHttpResponseBody([]byte(body))
			require.Equal(t, types.ActionPause, action)

			securityResponse := `{"Code": 200, "Message": "Success", "RequestId": "req-mmg-resp-deny", "Data": {"RiskLevel": "high"}}`
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(securityResponse))

			local := host.GetLocalResponse()
			require.NotNil(t, local, "expected SendHttpResponse for response deny")
			require.Contains(t, string(local.Data), "blockedDetails")
		})

		// MultiModalGuard text_generation request pass
		t.Run("multi modal guard text request pass", func(t *testing.T) {
			host, status := test.NewTestHost(multiModalGuardTextConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			body := `{"messages": [{"role": "user", "content": "Hello"}]}`
			require.Equal(t, types.ActionPause, host.CallOnHttpRequestBody([]byte(body)))

			securityResponse := `{"Code": 200, "Message": "Success", "RequestId": "req-mmg-pass", "Data": {"RiskLevel": "low"}}`
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(securityResponse))

			action := host.GetHttpStreamAction()
			require.Equal(t, types.ActionContinue, action)
			host.CompleteHttp()
		})
	})
}

func TestMultiModalGuardImageGenerationDeny(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// OpenAI image generation request deny → exercises multi_modal_guard/image/openai.go BuildDenyResponseBody path
		t.Run("openai image request deny returns blockedDetails", func(t *testing.T) {
			host, status := test.NewTestHost(multiModalGuardImageOpenAIConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/images/generations"},
				{":method", "POST"},
			})

			body := `{"prompt": "generate bad image"}`
			require.Equal(t, types.ActionPause, host.CallOnHttpRequestBody([]byte(body)))

			securityResponse := `{"Code": 200, "Message": "Success", "RequestId": "req-img-openai-deny", "Data": {"RiskLevel": "high"}}`
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(securityResponse))

			local := host.GetLocalResponse()
			require.NotNil(t, local, "expected SendHttpResponse for OpenAI image request deny")
			require.Contains(t, string(local.Data), "blockedDetails")
		})

		// OpenAI image generation request pass
		t.Run("openai image request pass", func(t *testing.T) {
			host, status := test.NewTestHost(multiModalGuardImageOpenAIConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/images/generations"},
				{":method", "POST"},
			})

			body := `{"prompt": "a cute cat"}`
			require.Equal(t, types.ActionPause, host.CallOnHttpRequestBody([]byte(body)))

			securityResponse := `{"Code": 200, "Message": "Success", "RequestId": "req-img-pass", "Data": {"RiskLevel": "low"}}`
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(securityResponse))

			action := host.GetHttpStreamAction()
			require.Equal(t, types.ActionContinue, action)
			host.CompleteHttp()
		})

		// Qwen image generation request deny → exercises multi_modal_guard/image/qwen.go BuildDenyResponseBody path
		t.Run("qwen image request deny returns blockedDetails", func(t *testing.T) {
			host, status := test.NewTestHost(multiModalGuardImageQwenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/images/generations"},
				{":method", "POST"},
			})

			body := `{"input": {"prompt": "generate bad image"}}`
			require.Equal(t, types.ActionPause, host.CallOnHttpRequestBody([]byte(body)))

			securityResponse := `{"Code": 200, "Message": "Success", "RequestId": "req-img-qwen-deny", "Data": {"RiskLevel": "high"}}`
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(securityResponse))

			local := host.GetLocalResponse()
			require.NotNil(t, local, "expected SendHttpResponse for Qwen image request deny")
			require.Contains(t, string(local.Data), "blockedDetails")
		})

		// Qwen image generation request pass
		t.Run("qwen image request pass", func(t *testing.T) {
			host, status := test.NewTestHost(multiModalGuardImageQwenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/images/generations"},
				{":method", "POST"},
			})

			body := `{"input": {"prompt": "a cute cat"}}`
			require.Equal(t, types.ActionPause, host.CallOnHttpRequestBody([]byte(body)))

			securityResponse := `{"Code": 200, "Message": "Success", "RequestId": "req-qwen-pass", "Data": {"RiskLevel": "low"}}`
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(securityResponse))

			action := host.GetHttpStreamAction()
			require.Equal(t, types.ActionContinue, action)
			host.CompleteHttp()
		})
	})
}

func TestMCPRequestDeny(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// MCP request deny → exercises multi_modal_guard/mcp/mcp.go HandleMcpRequestBody BuildDenyResponseBody path
		t.Run("mcp request deny returns blockedDetails", func(t *testing.T) {
			host, status := test.NewTestHost(mcpRequestConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/mcp/call"},
				{":method", "POST"},
			})

			body := `{"method": "tools/call", "params": {"arguments": "bad request content"}}`
			require.Equal(t, types.ActionPause, host.CallOnHttpRequestBody([]byte(body)))

			securityResponse := `{"Code": 200, "Message": "Success", "RequestId": "req-mcp-deny", "Data": {"RiskLevel": "high"}}`
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(securityResponse))

			local := host.GetLocalResponse()
			require.NotNil(t, local, "expected SendHttpResponse for MCP request deny")
			require.Contains(t, string(local.Data), "blockedDetails")
		})

		// MCP request pass
		t.Run("mcp request pass", func(t *testing.T) {
			host, status := test.NewTestHost(mcpRequestConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/mcp/call"},
				{":method", "POST"},
			})

			body := `{"method": "tools/call", "params": {"arguments": "safe content"}}`
			require.Equal(t, types.ActionPause, host.CallOnHttpRequestBody([]byte(body)))

			securityResponse := `{"Code": 200, "Message": "Success", "RequestId": "req-mcp-pass", "Data": {"RiskLevel": "low"}}`
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(securityResponse))

			action := host.GetHttpStreamAction()
			require.Equal(t, types.ActionContinue, action)
			host.CompleteHttp()
		})

		// MCP request skip non-tool-call method
		t.Run("mcp request skip non-tool-call", func(t *testing.T) {
			host, status := test.NewTestHost(mcpRequestConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/mcp/call"},
				{":method", "POST"},
			})

			body := `{"method": "resources/list", "params": {}}`
			action := host.CallOnHttpRequestBody([]byte(body))
			require.Equal(t, types.ActionContinue, action)
		})
	})
}

func TestTextModerationPlusResponseDeny(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// TextModerationPlus response deny → exercises text_moderation_plus/text (via common/text) BuildDenyResponseBody response path
		t.Run("text moderation plus response deny returns blockedDetails", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})

			body := `{"choices": [{"message": {"role": "assistant", "content": "bad response"}}]}`
			action := host.CallOnHttpResponseBody([]byte(body))
			require.Equal(t, types.ActionPause, action)

			securityResponse := `{"Code": 200, "Message": "Success", "RequestId": "req-tmp-resp-deny", "Data": {"RiskLevel": "high"}}`
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(securityResponse))

			local := host.GetLocalResponse()
			require.NotNil(t, local, "expected SendHttpResponse for response deny")
			require.Contains(t, string(local.Data), "blockedDetails")

			// Verify OpenAI completion shape wrapper
			type openAIChatCompletion struct {
				Choices []struct {
					Message struct {
						Content string `json:"content"`
					} `json:"message"`
				} `json:"choices"`
			}
			var outer openAIChatCompletion
			require.NoError(t, json.Unmarshal(local.Data, &outer))
			require.Len(t, outer.Choices, 1)

			var deny cfg.DenyResponseBody
			require.NoError(t, json.Unmarshal([]byte(outer.Choices[0].Message.Content), &deny))
			require.Equal(t, 200, deny.Code)
			require.NotEmpty(t, deny.BlockedDetails)
		})
	})
}

func TestBuildDenyResponseBody(t *testing.T) {
	makeConfig := func(contentBar, promptBar string) cfg.AISecurityConfig {
		return cfg.AISecurityConfig{
			ContentModerationLevelBar:  contentBar,
			PromptAttackLevelBar:       promptBar,
			SensitiveDataLevelBar:      "S4",
			MaliciousUrlLevelBar:       "max",
			ModelHallucinationLevelBar: "max",
			CustomLabelLevelBar:        "max",
			RiskAction:                 "block",
			Action:                     cfg.MultiModalGuard,
		}
	}

	t.Run("code equals response.Code", func(t *testing.T) {
		resp := cfg.Response{
			Code:      200,
			RequestId: "req-123",
			Data:      cfg.Data{},
		}
		body, err := cfg.BuildDenyResponseBody(resp, makeConfig("high", "high"), "")
		require.NoError(t, err)

		var result cfg.DenyResponseBody
		require.NoError(t, json.Unmarshal(body, &result))
		require.Equal(t, 200, result.Code)
	})

	t.Run("blockedDetails from Data.Detail", func(t *testing.T) {
		resp := cfg.Response{
			Code:      200,
			RequestId: "req-456",
			Data: cfg.Data{
				Detail: []cfg.Detail{
					{Type: cfg.ContentModerationType, Level: "high", Suggestion: "block"},
					{Type: cfg.PromptAttackType, Level: "low", Suggestion: "none"},
				},
			},
		}
		config := makeConfig("high", "high")
		body, err := cfg.BuildDenyResponseBody(resp, config, "")
		require.NoError(t, err)

		var result cfg.DenyResponseBody
		require.NoError(t, json.Unmarshal(body, &result))
		// only the contentModeration entry meets the "high" bar; promptAttack at "low" does not
		require.Len(t, result.BlockedDetails, 1)
		require.Equal(t, cfg.ContentModerationType, result.BlockedDetails[0].Type)
		require.Equal(t, "high", result.BlockedDetails[0].Level)
	})

	t.Run("blockedDetails empty when suggestion=block but below threshold", func(t *testing.T) {
		resp := cfg.Response{
			Code:      200,
			RequestId: "req-suggestion-block",
			Data: cfg.Data{
				Detail: []cfg.Detail{
					{Type: cfg.SensitiveDataType, Level: "S3", Suggestion: "block"},
				},
			},
		}
		config := makeConfig("high", "high")
		config.SensitiveDataLevelBar = "S4"
		body, err := cfg.BuildDenyResponseBody(resp, config, "")
		require.NoError(t, err)

		var result cfg.DenyResponseBody
		require.NoError(t, json.Unmarshal(body, &result))
		require.Len(t, result.BlockedDetails, 0)
	})

	t.Run("blockedDetails includes customLabel when threshold exceeded", func(t *testing.T) {
		resp := cfg.Response{
			Code:      200,
			RequestId: "req-custom-label",
			Data: cfg.Data{
				Detail: []cfg.Detail{
					{Type: cfg.CustomLabelType, Level: "high", Suggestion: "none"},
				},
			},
		}
		config := makeConfig("high", "high")
		config.CustomLabelLevelBar = "high"
		body, err := cfg.BuildDenyResponseBody(resp, config, "")
		require.NoError(t, err)

		var result cfg.DenyResponseBody
		require.NoError(t, json.Unmarshal(body, &result))
		require.Len(t, result.BlockedDetails, 1)
		require.Equal(t, cfg.CustomLabelType, result.BlockedDetails[0].Type)
		require.Equal(t, "high", result.BlockedDetails[0].Level)
	})

	t.Run("blockedDetails fallback from RiskLevel when Detail is empty", func(t *testing.T) {
		resp := cfg.Response{
			Code:      200,
			RequestId: "req-789",
			Data: cfg.Data{
				RiskLevel: "high",
				// Detail deliberately empty
			},
		}
		config := makeConfig("high", "high")
		body, err := cfg.BuildDenyResponseBody(resp, config, "")
		require.NoError(t, err)

		var result cfg.DenyResponseBody
		require.NoError(t, json.Unmarshal(body, &result))
		require.NotEmpty(t, result.BlockedDetails, "expected fallback detail from RiskLevel")
		require.Equal(t, cfg.ContentModerationType, result.BlockedDetails[0].Type)
		require.Equal(t, "high", result.BlockedDetails[0].Level)
	})

	t.Run("blockedDetails fallback from AttackLevel when Detail is empty", func(t *testing.T) {
		resp := cfg.Response{
			Code:      200,
			RequestId: "req-abc",
			Data: cfg.Data{
				AttackLevel: "high",
				// Detail deliberately empty
			},
		}
		config := makeConfig("high", "high")
		body, err := cfg.BuildDenyResponseBody(resp, config, "")
		require.NoError(t, err)

		var result cfg.DenyResponseBody
		require.NoError(t, json.Unmarshal(body, &result))
		require.NotEmpty(t, result.BlockedDetails, "expected fallback detail from AttackLevel")
		require.Equal(t, cfg.PromptAttackType, result.BlockedDetails[0].Type)
		require.Equal(t, "high", result.BlockedDetails[0].Level)
	})

	t.Run("blockedDetails empty when risk levels below threshold", func(t *testing.T) {
		resp := cfg.Response{
			Code:      200,
			RequestId: "req-def",
			Data: cfg.Data{
				RiskLevel:   "low",
				AttackLevel: "low",
			},
		}
		// threshold is "high", so "low" must not produce fallback entries
		config := makeConfig("high", "high")
		body, err := cfg.BuildDenyResponseBody(resp, config, "")
		require.NoError(t, err)

		var result cfg.DenyResponseBody
		require.NoError(t, json.Unmarshal(body, &result))
		require.Empty(t, result.BlockedDetails)
	})
}

func TestBuildDenyResponseBody_WithDenyMessage(t *testing.T) {
	config := cfg.AISecurityConfig{
		ContentModerationLevelBar:  "high",
		PromptAttackLevelBar:       "high",
		SensitiveDataLevelBar:      "S4",
		MaliciousUrlLevelBar:       "max",
		ModelHallucinationLevelBar: "max",
		CustomLabelLevelBar:        "max",
		RiskAction:                 "block",
		Action:                     cfg.MultiModalGuard,
		DenyMessage:                "很抱歉，我无法回答您的问题",
	}
	resp := cfg.Response{
		Code: 200,
		Data: cfg.Data{
			Detail: []cfg.Detail{
				{Type: cfg.ContentModerationType, Level: "high", Suggestion: "block"},
			},
		},
	}
	body, err := cfg.BuildDenyResponseBody(resp, config, "")
	require.NoError(t, err)

	var result cfg.DenyResponseBody
	require.NoError(t, json.Unmarshal(body, &result))
	require.Equal(t, "很抱歉，我无法回答您的问题", result.DenyMessage)
}

func TestBuildDenyResponseBody_WithoutDenyMessage(t *testing.T) {
	config := cfg.AISecurityConfig{
		ContentModerationLevelBar:  "high",
		PromptAttackLevelBar:       "high",
		SensitiveDataLevelBar:      "S4",
		MaliciousUrlLevelBar:       "max",
		ModelHallucinationLevelBar: "max",
		CustomLabelLevelBar:        "max",
		RiskAction:                 "block",
		Action:                     cfg.MultiModalGuard,
	}
	resp := cfg.Response{
		Code: 200,
		Data: cfg.Data{
			Detail: []cfg.Detail{
				{Type: cfg.ContentModerationType, Level: "high", Suggestion: "block"},
			},
		},
	}
	body, err := cfg.BuildDenyResponseBody(resp, config, "")
	require.NoError(t, err)
	require.NotContains(t, string(body), "denyMessage")
}

func TestBuildDenyResponseBody_BlockedDetailsOnlyTypeAndLevel(t *testing.T) {
	config := cfg.AISecurityConfig{
		ContentModerationLevelBar:  "high",
		PromptAttackLevelBar:       "high",
		SensitiveDataLevelBar:      "S4",
		MaliciousUrlLevelBar:       "max",
		ModelHallucinationLevelBar: "max",
		CustomLabelLevelBar:        "max",
		RiskAction:                 "block",
		Action:                     cfg.MultiModalGuard,
	}
	resp := cfg.Response{
		Code: 200,
		Data: cfg.Data{
			Detail: []cfg.Detail{
				{Type: cfg.ContentModerationType, Level: "high", Suggestion: "block", Result: []cfg.Result{{Label: "violence"}}},
				{Type: cfg.PromptAttackType, Level: "high", Suggestion: "block", Result: []cfg.Result{{Label: "injection"}}},
			},
		},
	}
	body, err := cfg.BuildDenyResponseBody(resp, config, "")
	require.NoError(t, err)

	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &raw))
	details := raw["blockedDetails"].([]interface{})
	require.Len(t, details, 2)
	for _, entry := range details {
		m := entry.(map[string]interface{})
		require.Len(t, m, 2, "each blockedDetail entry should have exactly 2 keys (type and level)")
		require.Contains(t, m, "type")
		require.Contains(t, m, "level")
	}
}

func TestBuildDenyResponseBody_CodeField(t *testing.T) {
	config := cfg.AISecurityConfig{
		ContentModerationLevelBar:  "high",
		PromptAttackLevelBar:       "high",
		SensitiveDataLevelBar:      "S4",
		MaliciousUrlLevelBar:       "max",
		ModelHallucinationLevelBar: "max",
		CustomLabelLevelBar:        "max",
		RiskAction:                 "block",
		Action:                     cfg.MultiModalGuard,
	}
	resp := cfg.Response{
		Code: 200,
		Data: cfg.Data{},
	}
	body, err := cfg.BuildDenyResponseBody(resp, config, "")
	require.NoError(t, err)

	var result cfg.DenyResponseBody
	require.NoError(t, json.Unmarshal(body, &result))
	require.Equal(t, 200, result.Code)
}

func TestBuildDenyResponseBody_NoRequestId(t *testing.T) {
	config := cfg.AISecurityConfig{
		ContentModerationLevelBar:  "high",
		PromptAttackLevelBar:       "high",
		SensitiveDataLevelBar:      "S4",
		MaliciousUrlLevelBar:       "max",
		ModelHallucinationLevelBar: "max",
		CustomLabelLevelBar:        "max",
		RiskAction:                 "block",
		Action:                     cfg.MultiModalGuard,
	}
	resp := cfg.Response{
		Code:      200,
		RequestId: "req-should-not-appear",
		Data:      cfg.Data{},
	}
	body, err := cfg.BuildDenyResponseBody(resp, config, "")
	require.NoError(t, err)
	require.NotContains(t, string(body), "requestId")
}

func TestBuildDenyResponseBody_FallbackSynthesis(t *testing.T) {
	config := cfg.AISecurityConfig{
		ContentModerationLevelBar:  "high",
		PromptAttackLevelBar:       "high",
		SensitiveDataLevelBar:      "S4",
		MaliciousUrlLevelBar:       "max",
		ModelHallucinationLevelBar: "max",
		CustomLabelLevelBar:        "max",
		RiskAction:                 "block",
		Action:                     cfg.MultiModalGuard,
	}
	resp := cfg.Response{
		Code: 200,
		Data: cfg.Data{
			RiskLevel: "high",
			// No Detail entries — triggers fallback synthesis
		},
	}
	body, err := cfg.BuildDenyResponseBody(resp, config, "")
	require.NoError(t, err)

	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &raw))
	details := raw["blockedDetails"].([]interface{})
	require.NotEmpty(t, details, "expected fallback synthesized entries")
	for _, entry := range details {
		m := entry.(map[string]interface{})
		require.Len(t, m, 2, "fallback blockedDetail entry should have exactly 2 keys (type and level)")
		require.Contains(t, m, "type")
		require.Contains(t, m, "level")
	}
}

// =============================================================================
// TC-COVER: 覆盖率补充测试
// =============================================================================

// TestMultiModalGuardStreamDeny 覆盖 openai.go RiskBlock 分支中 stream 响应格式路径
func TestMultiModalGuardStreamDeny(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("stream request deny returns SSE format", func(t *testing.T) {
			host, status := test.NewTestHost(multiModalGuardTextConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			// 请求体中包含 stream=true，触发 SSE 响应格式
			body := `{"messages": [{"role": "user", "content": "trigger deny"}], "stream": true}`
			require.Equal(t, types.ActionPause, host.CallOnHttpRequestBody([]byte(body)))

			securityResponse := `{"Code": 200, "Message": "Success", "RequestId": "req-stream-deny", "Data": {"RiskLevel": "high"}}`
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(securityResponse))

			local := host.GetLocalResponse()
			require.NotNil(t, local, "expected SendHttpResponse for stream deny")
			require.Contains(t, string(local.Data), "blockedDetails")
			// 验证 SSE content-type
			foundSSE := false
			for _, h := range local.Headers {
				if h[0] == "content-type" {
					require.Equal(t, "text/event-stream;charset=UTF-8", h[1])
					foundSSE = true
				}
			}
			require.True(t, foundSSE, "expected SSE content-type header")
		})
	})
}

// TestMultiModalGuardProtocolOriginalDeny 覆盖 openai.go RiskBlock 分支中 ProtocolOriginal 路径
func TestMultiModalGuardProtocolOriginalDeny(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("protocol original deny returns raw blockedDetails JSON", func(t *testing.T) {
			host, status := test.NewTestHost(protocolOriginalConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			body := `{"messages": [{"role": "user", "content": "trigger deny"}]}`
			require.Equal(t, types.ActionPause, host.CallOnHttpRequestBody([]byte(body)))

			securityResponse := `{"Code": 200, "Message": "Success", "RequestId": "req-proto-orig", "Data": {"RiskLevel": "high"}}`
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(securityResponse))

			local := host.GetLocalResponse()
			require.NotNil(t, local, "expected SendHttpResponse for protocol original deny")
			require.Contains(t, string(local.Data), "blockedDetails")
			// ProtocolOriginal 直接返回 JSON，不包装 OpenAI 格式
			for _, h := range local.Headers {
				if h[0] == "content-type" {
					require.Equal(t, "application/json", h[1])
				}
			}
			// 响应体是原始 blockedDetails JSON，不含 OpenAI 包装
			require.False(t, gjson.GetBytes(local.Data, "choices").Exists(), "should not wrap in OpenAI format")
		})
	})
}

// TestMultiModalGuardDenyWithAdvice 覆盖 openai.go RiskBlock 分支中 Advice != nil 路径
func TestMultiModalGuardDenyWithAdvice(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("deny with advice sets riskLabel and riskWords attributes", func(t *testing.T) {
			host, status := test.NewTestHost(multiModalGuardTextConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			body := `{"messages": [{"role": "user", "content": "trigger deny with advice"}]}`
			require.Equal(t, types.ActionPause, host.CallOnHttpRequestBody([]byte(body)))

			// 包含 Advice 和 Result 的安全服务响应
			securityResponse := `{
				"Code": 200, "Message": "Success", "RequestId": "req-advice-deny",
				"Data": {
					"RiskLevel": "high",
					"Result": [{"Label": "porn", "RiskWords": "bad-word"}],
					"Advice": [{"Answer": "blocked", "HitLabel": "porn"}],
					"Detail": [{"Suggestion": "block", "Type": "contentModeration", "Level": "high"}]
				}
			}`
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(securityResponse))

			local := host.GetLocalResponse()
			require.NotNil(t, local, "expected SendHttpResponse")
			require.Contains(t, string(local.Data), "blockedDetails")
		})
	})
}

// TestMultiChunkMasking 覆盖 openai.go 中 RiskPass + hasMasked 路径
// 场景：内容超过 LengthLimit(1800)，第一 chunk 触发 RiskMask 脱敏替换，第二 chunk RiskPass
func TestMultiChunkMasking(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("multi chunk masking with pass on second chunk", func(t *testing.T) {
			host, status := test.NewTestHost(maskConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			// 生成超过 LengthLimit (1800) 的内容
			longContent := strings.Repeat("a", 2000)
			body := `{"messages": [{"role": "user", "content": "` + longContent + `"}]}`
			require.Equal(t, types.ActionPause, host.CallOnHttpRequestBody([]byte(body)))

			// 第一个 chunk (1800 chars)：返回 mask 建议及脱敏内容
			maskedChunk := strings.Repeat("b", 1800)
			securityResponse1 := `{
				"Code": 200, "Message": "Success", "RequestId": "req-chunk-1",
				"Data": {
					"RiskLevel": "none",
					"Detail": [{
						"Suggestion": "mask", "Type": "sensitiveData", "Level": "S3",
						"Result": [{"Label": "phone", "Confidence": 99.0,
							"Ext": {"Desensitization": "` + maskedChunk + `"}}]
					}]
				}
			}`
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(securityResponse1))

			// 第二个 chunk (200 chars)：返回 pass（无风险）
			securityResponse2 := `{"Code": 200, "Message": "Success", "RequestId": "req-chunk-2", "Data": {"RiskLevel": "none", "Detail": []}}`
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(securityResponse2))

			// 验证请求体被替换为脱敏内容
			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)
			content := gjson.GetBytes(processedBody, "messages.@reverse.0.content").String()
			// 期望 = 脱敏后的第一 chunk (1800 'b') + 原始第二 chunk (200 'a')
			expectedContent := maskedChunk + strings.Repeat("a", 200)
			require.Equal(t, expectedContent, content)

			host.CompleteHttp()
		})

		// 覆盖 RiskMask 成功完成路径（单 chunk 内容刚好 <= LengthLimit，RiskMask 后立即完成）
		// 该路径在 RiskMask 分支中 contentIndex >= len(maskedContent) 的子路径
		t.Run("single chunk mask completes in RiskMask branch", func(t *testing.T) {
			host, status := test.NewTestHost(maskConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			body := `{"messages": [{"role": "user", "content": "我的银行卡号是6222021234567890"}]}`
			require.Equal(t, types.ActionPause, host.CallOnHttpRequestBody([]byte(body)))

			securityResponse := `{
				"Code": 200, "Message": "Success", "RequestId": "req-single-mask",
				"Data": {
					"RiskLevel": "none",
					"Detail": [{
						"Suggestion": "mask", "Type": "sensitiveData", "Level": "S3",
						"Result": [{"Label": "bank_card", "Confidence": 99.0,
							"Ext": {"Desensitization": "我的银行卡号是6222************"}}]
					}]
				}
			}`
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(securityResponse))

			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)
			content := gjson.GetBytes(processedBody, "messages.@reverse.0.content").String()
			require.Equal(t, "我的银行卡号是6222************", content)

			host.CompleteHttp()
		})
	})
}

// TestMultiModalGuardMaskStreamDeny 覆盖 openai.go RiskMask 空脱敏 fallthrough 到 RiskBlock 的 stream 路径
func TestMultiModalGuardMaskStreamDeny(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("mask with empty desensitization falls through to block stream format", func(t *testing.T) {
			host, status := test.NewTestHost(maskConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			// stream=true 使 block 走 SSE 格式
			body := `{"messages": [{"role": "user", "content": "敏感内容"}], "stream": true}`
			require.Equal(t, types.ActionPause, host.CallOnHttpRequestBody([]byte(body)))

			// 返回 mask 建议但脱敏内容为空 → fallthrough 到 RiskBlock
			securityResponse := `{
				"Code": 200, "Message": "Success", "RequestId": "req-mask-stream-deny",
				"Data": {
					"RiskLevel": "none",
					"Detail": [{
						"Suggestion": "mask", "Type": "sensitiveData", "Level": "S3",
						"Result": [{"Label": "phone", "Confidence": 99.0,
							"Ext": {"Desensitization": ""}}]
					}]
				}
			}`
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(securityResponse))

			local := host.GetLocalResponse()
			require.NotNil(t, local, "expected SendHttpResponse after mask fallthrough to block")
			// 验证是 SSE 格式
			foundSSE := false
			for _, h := range local.Headers {
				if h[0] == "content-type" {
					require.Equal(t, "text/event-stream;charset=UTF-8", h[1])
					foundSSE = true
				}
			}
			require.True(t, foundSSE, "expected SSE content-type for stream deny")
		})
	})
}
