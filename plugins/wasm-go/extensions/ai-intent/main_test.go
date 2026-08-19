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
	"fmt"
	"strings"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type recordingConfigLog struct {
	entries []string
}

func (l *recordingConfigLog) append(format string, args ...interface{}) {
	l.entries = append(l.entries, fmt.Sprintf(format, args...))
}

func (l *recordingConfigLog) Trace(msg string)                             { l.append("%s", msg) }
func (l *recordingConfigLog) Tracef(format string, args ...interface{})    { l.append(format, args...) }
func (l *recordingConfigLog) Debug(msg string)                             { l.append("%s", msg) }
func (l *recordingConfigLog) Debugf(format string, args ...interface{})    { l.append(format, args...) }
func (l *recordingConfigLog) Info(msg string)                              { l.append("%s", msg) }
func (l *recordingConfigLog) Infof(format string, args ...interface{})     { l.append(format, args...) }
func (l *recordingConfigLog) Warn(msg string)                              { l.append("%s", msg) }
func (l *recordingConfigLog) Warnf(format string, args ...interface{})     { l.append(format, args...) }
func (l *recordingConfigLog) Error(msg string)                             { l.append("%s", msg) }
func (l *recordingConfigLog) Errorf(format string, args ...interface{})    { l.append(format, args...) }
func (l *recordingConfigLog) Critical(msg string)                          { l.append("%s", msg) }
func (l *recordingConfigLog) Criticalf(format string, args ...interface{}) { l.append(format, args...) }
func (l *recordingConfigLog) ResetID(string)                               {}

func TestParseConfigDoesNotLogProxyAPIKey(t *testing.T) {
	const (
		apiKey       = "fixture-api-key-must-not-be-logged"
		secretPrompt = "fixture-private-prompt-must-not-be-logged"
		secretURL    = "user:password@"
	)
	configJSON, err := json.Marshal(map[string]interface{}{
		"scene": map[string]interface{}{
			"category": "test",
			"prompt":   secretPrompt,
		},
		"llm": map[string]interface{}{
			"proxyServiceName": "ai-service",
			"proxyUrl":         "https://" + secretURL + "ai.example.com/v1/chat/completions",
			"proxyApiKey":      apiKey,
		},
	})
	require.NoError(t, err)

	logger := &recordingConfigLog{}
	config := &PluginConfig{}
	require.NoError(t, parseConfig(gjson.ParseBytes(configJSON), config, logger))
	logs := strings.Join(logger.entries, "\n")
	require.NotContains(t, logs, apiKey)
	require.NotContains(t, logs, secretPrompt)
	require.NotContains(t, logs, secretURL)
}

// 测试配置：基本意图识别配置
var basicIntentConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"scene": map[string]interface{}{
			"category": "金融|电商|法律|Higress",
			"prompt":   "你是一个智能类别识别助手，负责根据用户提出的问题和预设的类别，确定问题属于哪个预设的类别，并给出相应的类别。用户提出的问题为:'%s',预设的类别为'%s'，直接返回一种具体类别，如果没有找到就返回'NotFound'。",
		},
		"llm": map[string]interface{}{
			"proxyServiceName": "ai-service",
			"proxyUrl":         "http://ai.example.com/v1/chat/completions",
			"proxyModel":       "qwen-long",
			"proxyPort":        80,
			"proxyDomain":      "ai.example.com",
			"proxyTimeout":     10000,
			"proxyApiKey":      "test-api-key",
		},
		"keyFrom": map[string]interface{}{
			"requestBody":  "messages.@reverse.0.content",
			"responseBody": "choices.0.message.content",
		},
	})
	return data
}()

// 测试配置：自定义提示词配置
var customPromptConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"scene": map[string]interface{}{
			"category": "技术|产品|运营|设计",
			"prompt":   "请分析以下问题属于哪个技术领域：%s，可选领域：%s，请直接返回领域名称。",
		},
		"llm": map[string]interface{}{
			"proxyServiceName": "ai-service",
			"proxyUrl":         "https://ai.example.com/v1/chat/completions",
			"proxyModel":       "gpt-3.5-turbo",
			"proxyPort":        443,
			"proxyDomain":      "ai.example.com",
			"proxyTimeout":     15000,
			"proxyApiKey":      "custom-api-key",
		},
		"keyFrom": map[string]interface{}{
			"requestBody":  "query",
			"responseBody": "result",
		},
	})
	return data
}()

// 测试配置：最小配置（使用默认值）
var minimalConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"scene": map[string]interface{}{
			"category": "A|B|C",
		},
		"llm": map[string]interface{}{
			"proxyServiceName": "ai-service",
			"proxyUrl":         "http://ai.example.com/v1/chat/completions",
		},
		"keyFrom": map[string]interface{}{},
	})
	return data
}()

// 测试配置：HTTPS配置
var httpsConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"scene": map[string]interface{}{
			"category": "客服|销售|技术支持",
		},
		"llm": map[string]interface{}{
			"proxyServiceName": "ai-service",
			"proxyUrl":         "https://ai.example.com:8443/v1/chat/completions",
			"proxyModel":       "claude-3",
			"proxyTimeout":     20000,
			"proxyApiKey":      "https-api-key",
		},
		"keyFrom": map[string]interface{}{
			"requestBody":  "input.text",
			"responseBody": "output.classification",
		},
	})
	return data
}()

func TestParseConfig(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// 测试基本意图识别配置解析
		t.Run("basic intent config", func(t *testing.T) {
			host, status := test.NewTestHost(basicIntentConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试自定义提示词配置解析
		t.Run("custom prompt config", func(t *testing.T) {
			host, status := test.NewTestHost(customPromptConfig)
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

		// 测试HTTPS配置解析
		t.Run("https config", func(t *testing.T) {
			host, status := test.NewTestHost(httpsConfig)
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
		// 测试请求头处理
		t.Run("request headers processing", func(t *testing.T) {
			host, status := test.NewTestHost(basicIntentConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			// 应该返回HeaderStopIteration，因为禁用了重路由
			require.Equal(t, types.HeaderStopIteration, action)
		})
	})
}

func TestOnHttpRequestBody(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试请求体处理 - 金融类问题
		t.Run("financial question processing", func(t *testing.T) {
			host, status := test.NewTestHost(basicIntentConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			// 构造请求体 - 金融类问题
			requestBody := `{
				"messages": [
					{"role": "user", "content": "今天股市怎么样？"}
				]
			}`

			// 调用请求体处理
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			// 应该返回ActionPause，因为需要等待LLM响应
			require.Equal(t, types.ActionPause, action)

			// 模拟LLM响应 - 返回"金融"类别
			llmResponse := `{
				"choices": [
					{
						"message": {
							"content": "金融"
						}
					}
				]
			}`

			// 模拟HTTP调用响应
			host.CallOnHttpCall([][2]string{
				{"content-type", "application/json"},
				{":status", "200"},
			}, []byte(llmResponse))

			// 验证插件是否正确处理了LLM响应
			// 插件应该将"金融"类别设置到Property中
			// 通过host.GetProperty验证意图类别是否被正确设置
			intentCategory, err := host.GetProperty([]string{"intent_category"})
			require.NoError(t, err)
			require.Equal(t, "金融", string(intentCategory))

			// 完成HTTP请求
			host.CompleteHttp()
		})

		// 测试请求体处理 - 电商类问题
		t.Run("ecommerce question processing", func(t *testing.T) {
			host, status := test.NewTestHost(basicIntentConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			// 构造请求体 - 电商类问题
			requestBody := `{
				"messages": [
					{"role": "user", "content": "这个商品什么时候发货？"}
				]
			}`

			// 调用请求体处理
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			// 应该返回ActionPause
			require.Equal(t, types.ActionPause, action)

			// 模拟LLM响应 - 返回"电商"类别
			llmResponse := `{
				"choices": [
					{
						"message": {
							"content": "电商"
						}
					}
				]
			}`

			// 模拟HTTP调用响应
			host.CallOnHttpCall([][2]string{
				{"content-type", "application/json"},
				{":status", "200"},
			}, []byte(llmResponse))

			// 验证插件是否正确处理了LLM响应
			// 插件应该将"电商"类别设置到Property中
			// 通过host.GetProperty验证意图类别是否被正确设置
			intentCategory, err := host.GetProperty([]string{"intent_category"})
			require.NoError(t, err)
			require.Equal(t, "电商", string(intentCategory))

			// 完成HTTP请求
			host.CompleteHttp()
		})

		// 测试请求体处理 - 未找到类别
		t.Run("category not found processing", func(t *testing.T) {
			host, status := test.NewTestHost(basicIntentConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			// 构造请求体 - 不相关的问题
			requestBody := `{
				"messages": [
					{"role": "user", "content": "今天天气怎么样？"}
				]
			}`

			// 调用请求体处理
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			// 应该返回ActionPause
			require.Equal(t, types.ActionPause, action)

			// 模拟LLM响应 - 返回"NotFound"
			llmResponse := `{
				"choices": [
					{
						"message": {
							"content": "NotFound"
						}
					}
				]
			}`

			// 模拟HTTP调用响应
			host.CallOnHttpCall([][2]string{
				{"content-type", "application/json"},
				{":status", "200"},
			}, []byte(llmResponse))

			_, err := host.GetProperty([]string{"intent_category"})
			// 应该返回错误，因为没有设置该Property
			require.Error(t, err)

			// 完成HTTP请求
			host.CompleteHttp()
		})
	})
}

// 测试配置验证
func TestConfigValidation(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// 测试缺少scene.category配置
		t.Run("missing scene.category", func(t *testing.T) {
			invalidConfig := func() json.RawMessage {
				data, _ := json.Marshal(map[string]interface{}{
					"scene": map[string]interface{}{
						"prompt": "test prompt",
					},
					"llm": map[string]interface{}{
						"proxyServiceName": "ai-service",
						"proxyUrl":         "http://ai.example.com/v1/chat/completions",
					},
					"keyFrom": map[string]interface{}{},
				})
				return data
			}()

			host, status := test.NewTestHost(invalidConfig)
			defer host.Reset()
			// 应该返回错误状态，因为缺少必需的scene.category
			require.NotEqual(t, types.OnPluginStartStatusOK, status)
		})

		// 测试缺少llm.proxyServiceName配置
		t.Run("missing llm.proxyServiceName", func(t *testing.T) {
			invalidConfig := func() json.RawMessage {
				data, _ := json.Marshal(map[string]interface{}{
					"scene": map[string]interface{}{
						"category": "A|B|C",
					},
					"llm": map[string]interface{}{
						"proxyUrl": "http://ai.example.com/v1/chat/completions",
					},
					"keyFrom": map[string]interface{}{},
				})
				return data
			}()

			host, status := test.NewTestHost(invalidConfig)
			defer host.Reset()
			// 应该返回错误状态，因为缺少必需的llm.proxyServiceName
			require.NotEqual(t, types.OnPluginStartStatusOK, status)
		})

		// 测试缺少llm.proxyUrl配置
		t.Run("missing llm.proxyUrl", func(t *testing.T) {
			invalidConfig := func() json.RawMessage {
				data, _ := json.Marshal(map[string]interface{}{
					"scene": map[string]interface{}{
						"category": "A|B|C",
					},
					"llm": map[string]interface{}{
						"proxyServiceName": "ai-service",
					},
					"keyFrom": map[string]interface{}{},
				})
				return data
			}()

			host, status := test.NewTestHost(invalidConfig)
			defer host.Reset()
			// 应该返回错误状态，因为缺少必需的llm.proxyUrl
			require.NotEqual(t, types.OnPluginStartStatusOK, status)
		})

		// 测试缺少必需字段的配置
		t.Run("missing required fields", func(t *testing.T) {
			invalidConfig := func() json.RawMessage {
				data, _ := json.Marshal(map[string]interface{}{
					"scene": map[string]interface{}{
						"category": "A|B|C",
					},
					"llm": map[string]interface{}{
						"proxyServiceName": "ai-service",
						// 故意不设置proxyUrl，这是必需的
					},
					"keyFrom": map[string]interface{}{},
				})
				return data
			}()

			host, status := test.NewTestHost(invalidConfig)
			defer host.Reset()
			// 应该返回错误状态，因为缺少必需的proxyUrl
			require.NotEqual(t, types.OnPluginStartStatusOK, status)
		})
	})
}

// 测试边界情况
func TestEdgeCases(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {

		// 测试无效JSON请求体
		t.Run("invalid JSON request body", func(t *testing.T) {
			host, status := test.NewTestHost(basicIntentConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			// 调用请求体处理 - 无效JSON
			invalidJSON := []byte(`{"messages": [{"role": "user", "content": "test"}`)
			action := host.CallOnHttpRequestBody(invalidJSON)

			// 应该返回ActionPause，因为需要等待LLM响应
			require.Equal(t, types.ActionPause, action)

			// 模拟LLM响应
			llmResponse := `{
				"choices": [
					{
						"message": {
							"content": "NotFound"
						}
					}
				]
			}`

			host.CallOnHttpCall([][2]string{
				{"content-type", "application/json"},
				{":status", "200"},
			}, []byte(llmResponse))

			// 验证插件是否正确处理了LLM响应
			// 由于返回"NotFound"，插件不会设置任何意图类别到Property中
			// 验证没有设置意图类别Property
			_, err := host.GetProperty([]string{"intent_category"})
			// 应该返回错误，因为没有设置该Property
			require.Error(t, err)

			host.CompleteHttp()
		})

		// 测试LLM服务错误响应
		t.Run("LLM service error response", func(t *testing.T) {
			host, status := test.NewTestHost(basicIntentConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			// 构造请求体
			requestBody := `{
				"messages": [
					{"role": "user", "content": "今天股市怎么样？"}
				]
			}`

			// 调用请求体处理
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			// 应该返回ActionPause
			require.Equal(t, types.ActionPause, action)

			// 模拟LLM服务错误响应
			errorResponse := `{
				"error": "Service unavailable",
				"message": "LLM service is down"
			}`

			host.CallOnHttpCall([][2]string{
				{"content-type", "application/json"},
				{":status", "503"},
			}, []byte(errorResponse))

			// 验证插件是否正确处理了LLM错误响应
			// 由于状态码不是200，插件不会设置任何意图类别到Property中
			host.CompleteHttp()
		})
	})
}
