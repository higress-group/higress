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
)

// 测试配置：基础模板配置
var basicConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"templates": []map[string]interface{}{
			{
				"name":     "greeting",
				"template": "Hello {{name}}, welcome to {{company}}!",
			},
			{
				"name":     "summary",
				"template": "Here is a summary of {{topic}}: {{content}}",
			},
		},
	})
	return data
}()

// 测试配置：单个模板配置
var singleTemplateConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"templates": []map[string]interface{}{
			{
				"name":     "simple",
				"template": "This is a {{adjective}} {{noun}}.",
			},
		},
	})
	return data
}()

// 测试配置：空模板配置
var emptyTemplatesConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"templates": []map[string]interface{}{},
	})
	return data
}()

// 测试配置：复杂模板配置
var complexTemplateConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"templates": []map[string]interface{}{
			{
				"name":     "email",
				"template": "Dear {{recipient}},\n\n{{greeting}}\n\n{{body}}\n\nBest regards,\n{{sender}}",
			},
			{
				"name":     "report",
				"template": "Report: {{title}}\nDate: {{date}}\nAuthor: {{author}}\n\n{{content}}\n\nConclusion: {{conclusion}}",
			},
		},
	})
	return data
}()

func TestParseConfig(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// 测试基础模板配置解析
		t.Run("basic templates config", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)
			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			promptConfig := config.(*AIPromptTemplateConfig)
			require.NotNil(t, promptConfig.templates)
			require.Len(t, promptConfig.templates, 2)
			// 由于gjson.Get("template").Raw返回JSON原始值，包含引号
			require.Equal(t, "\"Hello {{name}}, welcome to {{company}}!\"", promptConfig.templates["greeting"])
			require.Equal(t, "\"Here is a summary of {{topic}}: {{content}}\"", promptConfig.templates["summary"])
		})

		// 测试单个模板配置解析
		t.Run("single template config", func(t *testing.T) {
			host, status := test.NewTestHost(singleTemplateConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)
			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			promptConfig := config.(*AIPromptTemplateConfig)
			require.NotNil(t, promptConfig.templates)
			require.Len(t, promptConfig.templates, 1)
			// 由于gjson.Get("template").Raw返回JSON原始值，包含引号
			require.Equal(t, "\"This is a {{adjective}} {{noun}}.\"", promptConfig.templates["simple"])
		})

		// 测试空模板配置解析
		t.Run("empty templates config", func(t *testing.T) {
			host, status := test.NewTestHost(emptyTemplatesConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)
			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			promptConfig := config.(*AIPromptTemplateConfig)
			require.NotNil(t, promptConfig.templates)
			require.Len(t, promptConfig.templates, 0)
		})

		// 测试复杂模板配置解析
		t.Run("complex templates config", func(t *testing.T) {
			host, status := test.NewTestHost(complexTemplateConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)
			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			promptConfig := config.(*AIPromptTemplateConfig)
			require.NotNil(t, promptConfig.templates)
			require.Len(t, promptConfig.templates, 2)
			// 由于gjson.Get("template").Raw返回JSON原始值，包含引号和转义字符
			require.Equal(t, "\"Dear {{recipient}},\\n\\n{{greeting}}\\n\\n{{body}}\\n\\nBest regards,\\n{{sender}}\"", promptConfig.templates["email"])
			require.Equal(t, "\"Report: {{title}}\\nDate: {{date}}\\nAuthor: {{author}}\\n\\n{{content}}\\n\\nConclusion: {{conclusion}}\"", promptConfig.templates["report"])
		})
	})
}

func TestOnHttpRequestHeaders(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试启用模板的情况
		t.Run("template enabled", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，启用模板
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"template-enable", "true"},
				{"content-length", "100"},
			})

			// 应该返回ActionContinue
			require.Equal(t, types.ActionContinue, action)
		})

		// 测试禁用模板的情况
		t.Run("template disabled", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，禁用模板
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"template-enable", "false"},
				{"content-length", "100"},
			})

			// 应该返回ActionContinue
			require.Equal(t, types.ActionContinue, action)
		})

		// 测试没有template-enable头的情况
		t.Run("no template-enable header", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，不包含template-enable
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"content-length", "100"},
			})

			// 应该返回ActionContinue
			require.Equal(t, types.ActionContinue, action)
		})
	})
}

func TestOnHttpRequestBody(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试基础模板替换
		t.Run("basic template replacement", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"template-enable", "true"},
			})

			// 设置请求体，包含模板和属性
			body := `{
				"template": "greeting",
				"properties": {
					"name": "Alice",
					"company": "TechCorp"
				}
			}`
			action := host.CallOnHttpRequestBody([]byte(body))

			// 应该返回ActionContinue
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})

		// 测试复杂模板替换
		t.Run("complex template replacement", func(t *testing.T) {
			host, status := test.NewTestHost(complexTemplateConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"template-enable", "true"},
			})

			// 设置请求体，包含复杂模板和属性
			body := `{
				"template": "email",
				"properties": {
					"recipient": "John Doe",
					"greeting": "I hope this email finds you well",
					"body": "Please find attached the quarterly report",
					"sender": "Jane Smith"
				}
			}`
			action := host.CallOnHttpRequestBody([]byte(body))

			// 应该返回ActionContinue
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})

		// 测试没有模板的情况
		t.Run("no template in body", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"template-enable", "true"},
			})

			// 设置请求体，不包含模板
			body := `{
				"messages": [
					{"role": "user", "content": "Hello"}
				]
			}`
			action := host.CallOnHttpRequestBody([]byte(body))

			// 应该返回ActionContinue
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})

		// 测试没有属性的情况
		t.Run("no properties in body", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"template-enable", "true"},
			})

			// 设置请求体，包含模板但不包含属性
			body := `{
				"template": "greeting"
			}`
			action := host.CallOnHttpRequestBody([]byte(body))

			// 应该返回ActionContinue
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})

		// 测试部分属性替换
		t.Run("partial properties replacement", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"template-enable", "true"},
			})

			// 设置请求体，只包含部分属性
			body := `{
				"template": "greeting",
				"properties": {
					"name": "Bob"
				}
			}`
			action := host.CallOnHttpRequestBody([]byte(body))

			// 应该返回ActionContinue
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})
	})
}

func TestStructs(t *testing.T) {
	// 测试AIPromptTemplateConfig结构体
	t.Run("AIPromptTemplateConfig struct", func(t *testing.T) {
		config := &AIPromptTemplateConfig{
			templates: map[string]string{
				"test": "This is a {{test}} template",
			},
		}
		require.NotNil(t, config.templates)
		require.Len(t, config.templates, 1)
		require.Equal(t, "This is a {{test}} template", config.templates["test"])
	})
}

func TestTemplateReplacementLogic(t *testing.T) {
	// 测试模板变量替换逻辑
	t.Run("template variable replacement", func(t *testing.T) {
		config := &AIPromptTemplateConfig{
			templates: map[string]string{
				"greeting": "Hello {{name}}, welcome to {{company}}!",
			},
		}

		// 模拟模板替换逻辑
		template := config.templates["greeting"]
		require.Equal(t, "Hello {{name}}, welcome to {{company}}!", template)

		// 测试变量替换
		properties := map[string]string{
			"name":    "Alice",
			"company": "TechCorp",
		}

		for key, value := range properties {
			template = strings.ReplaceAll(template, fmt.Sprintf("{{%s}}", key), value)
		}

		require.Equal(t, "Hello Alice, welcome to TechCorp!", template)
	})

	// 测试嵌套变量替换
	t.Run("nested variable replacement", func(t *testing.T) {
		config := &AIPromptTemplateConfig{
			templates: map[string]string{
				"nested": "{{greeting}} {{name}}, {{message}}",
			},
		}

		template := config.templates["nested"]
		require.Equal(t, "{{greeting}} {{name}}, {{message}}", template)

		// 测试嵌套替换
		properties := map[string]string{
			"greeting": "Hello",
			"name":     "World",
			"message":  "welcome!",
		}

		for key, value := range properties {
			template = strings.ReplaceAll(template, fmt.Sprintf("{{%s}}", key), value)
		}

		require.Equal(t, "Hello World, welcome!", template)
	})
}

func TestTemplatePropertiesPreserveJSON(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			value interface{}
			want  string
		}{
			{"quotes", `say "hello"`, `say "hello"`},
			{"newlines and tabs", "first\nsecond\tline", "first\nsecond\tline"},
			{"backslashes", `C:\users\example`, `C:\users\example`},
			{"control characters", "a\x00\x01b", "a\x00\x01b"},
			{"json object", map[string]interface{}{"name": "example"}, `{"name":"example"}`},
			{"unicode", "你好🌍", "你好🌍"},
			{"number", 42, "42"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				config := json.RawMessage(`{"templates":[{"name":"json","template":{"model":"example-model","messages":[{"role":"user","content":"before {{value}} after"}]}}]}`)
				host, status := test.NewTestHost(config)
				defer host.Reset()
				require.Equal(t, types.OnPluginStartStatusOK, status)
				host.CallOnHttpRequestHeaders([][2]string{{":authority", "example.com"}, {":path", "/v1/chat/completions"}, {":method", "POST"}, {"content-type", "application/json"}})
				body, err := json.Marshal(map[string]interface{}{"template": "json", "properties": map[string]interface{}{"value": tc.value}})
				require.NoError(t, err)
				require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody(body))
				var got struct {
					Model    string `json:"model"`
					Messages []struct {
						Role    string `json:"role"`
						Content string `json:"content"`
					} `json:"messages"`
				}
				require.NoError(t, json.Unmarshal(host.GetRequestBody(), &got))
				require.Equal(t, "example-model", got.Model)
				require.Len(t, got.Messages, 1)
				require.Equal(t, "user", got.Messages[0].Role)
				require.Equal(t, "before "+tc.want+" after", got.Messages[0].Content)
			})
		}
	})
}
