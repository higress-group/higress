// Copyright (c) 2026 Alibaba Group Holding Ltd.
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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tidwall/gjson"
)

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantMax   int
		wantCode  int
		wantRatio float64
		wantOk    bool
		wantErr   bool
	}{
		{
			name:      "完整配置",
			input:     `{"max_context_tokens":128000,"error_status_code":413,"buffer_ratio":1.2}`,
			wantMax:   128000,
			wantCode:  413,
			wantRatio: 1.2,
			wantOk:    true,
		},
		{
			name:      "仅必填字段，其余取默认值",
			input:     `{"max_context_tokens":32000}`,
			wantMax:   32000,
			wantCode:  defaultErrorStatusCode,
			wantRatio: defaultBufferRatio,
			wantOk:    true,
		},
		{
			name:      "缺失阈值不抛错，IsEnabled=false",
			input:     `{}`,
			wantMax:   0,
			wantCode:  0,
			wantRatio: 0,
			wantOk:    false,
		},
		{
			name:      "阈值为 0 视为未启用",
			input:     `{"max_context_tokens":0}`,
			wantMax:   0,
			wantCode:  0,
			wantRatio: 0,
			wantOk:    false,
		},
		{
			name:    "max_context_tokens 负数拒绝",
			input:   `{"max_context_tokens":-1}`,
			wantErr: true,
		},
		{
			name:    "buffer_ratio 负数拒绝",
			input:   `{"max_context_tokens":1000,"buffer_ratio":-1}`,
			wantErr: true,
		},
		{
			name:    "error_status_code=200 拒绝",
			input:   `{"max_context_tokens":1000,"error_status_code":200}`,
			wantErr: true,
		},
		{
			name:    "error_status_code=600 拒绝",
			input:   `{"max_context_tokens":1000,"error_status_code":600}`,
			wantErr: true,
		},
		{
			name:    "buffer_ratio=11 拒绝",
			input:   `{"max_context_tokens":1000,"buffer_ratio":11}`,
			wantErr: true,
		},
		{
			name:      "buffer_ratio=10 边界允许",
			input:     `{"max_context_tokens":1000,"buffer_ratio":10}`,
			wantMax:   1000,
			wantCode:  defaultErrorStatusCode,
			wantRatio: 10,
			wantOk:    true,
		},
		{
			name:      "error_status_code=599 边界允许",
			input:     `{"max_context_tokens":1000,"error_status_code":599}`,
			wantMax:   1000,
			wantCode:  599,
			wantRatio: defaultBufferRatio,
			wantOk:    true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var cfg Config
			err := parseConfig(gjson.Parse(tc.input), &cfg)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.wantMax, cfg.MaxContextTokens)
			assert.Equal(t, tc.wantCode, cfg.ErrorStatusCode)
			assert.InDelta(t, tc.wantRatio, cfg.BufferRatio, 1e-9)
			assert.Equal(t, tc.wantOk, cfg.IsEnabled())
		})
	}
}

// TestLightweightE2E 轻量端到端验证：
// 用低阈值跑完 extract + CountTokens + 判定，确认新增字段真实影响拦截/放行决策。
func TestLightweightE2E(t *testing.T) {
	require := assert.New(t)
	require.NoError(initEncoder())

	cases := []struct {
		name      string
		body      []byte
		maxTokens int
		wantBlock bool
	}{
		{
			name: "OpenAI tool_calls.arguments 超阈值 → 400",
			body: []byte(`{
				"messages": [
					{"role": "user", "content": "go"},
					{"role": "assistant", "tool_calls": [{"id": "c1", "type": "function", "function": {
						"name": "big_query",
						"arguments": "{\"sql\":\"SELECT a]very long query that should push tokens over the low threshold we set for this test, including multiple columns like id, name, email, phone, address, city, state, zip, country, created_at, updated_at, deleted_at FROM users WHERE status = active AND region IN (us-east-1, us-west-2, eu-west-1, ap-southeast-1) ORDER BY created_at DESC LIMIT 1000\"}"
					}}]}
				]
			}`),
			maxTokens: 5,
			wantBlock: true,
		},
		{
			name: "OpenAI response_format.json_schema 超阈值 → 400",
			body: []byte(`{
				"messages": [{"role": "user", "content": "x"}],
				"response_format": {"type": "json_schema", "json_schema": {
					"name": "big_schema",
					"description": "A very detailed schema for structured extraction of complex nested data",
					"schema": {"type": "object", "properties": {"a": {"type": "string"}, "b": {"type": "integer"}, "c": {"type": "array", "items": {"type": "object", "properties": {"d": {"type": "string"}}}}}}
				}}
			}`),
			maxTokens: 5,
			wantBlock: true,
		},
		{
			name: "Anthropic tools.input_schema 超阈值 → 400",
			body: []byte(`{
				"messages": [{"role": "user", "content": "hi"}],
				"tools": [{"name": "search", "description": "Search the database with complex filters", "input_schema": {"type": "object", "properties": {"query": {"type": "string"}, "filters": {"type": "array", "items": {"type": "object", "properties": {"field": {"type": "string"}, "op": {"type": "string"}, "value": {"type": "string"}}}}}}}]
			}`),
			maxTokens: 5,
			wantBlock: true,
		},
		{
			name: "Anthropic 短文本 → 放行",
			body: []byte(`{
				"system": "ok",
				"messages": [{"role": "user", "content": "hi"}],
				"tools": [{"name": "t", "input_schema": {"type": "object"}}]
			}`),
			maxTokens: 100,
			wantBlock: false,
		},
		{
			name: "Anthropic image block → 放行",
			body: []byte(`{
				"messages": [{"role": "user", "content": [
					{"type": "text", "text": "describe"},
					{"type": "image", "source": {"type": "base64", "data": "..."}}
				]}],
				"tools": [{"name": "x", "input_schema": {}}]
			}`),
			maxTokens: 1,
			wantBlock: false, // 多模态放行，不管阈值多低
		},
		{
			name: "Anthropic thinking block 超阈值 → 400（无 tools）",
			body: []byte(`{
				"messages": [
					{"role": "user", "content": "solve this"},
					{"role": "assistant", "content": [
						{"type": "thinking", "thinking": "Let me reason through this carefully. First, I need to analyze the problem from multiple angles. The key insight is that we need to consider all boundary conditions and edge cases before arriving at a solution. This requires systematic decomposition of the constraints."},
						{"type": "text", "text": "The answer is 42."}
					]}
				]
			}`),
			maxTokens: 5,
			wantBlock: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := extractPromptText(tc.body)
			tokens := CountTokens(r.Text)
			estimated := int(float64(tokens) * 1.10)
			blocked := !r.HasMultimodal && estimated > tc.maxTokens

			t.Logf("multimodal=%v tokens=%d estimated=%d threshold=%d blocked=%v",
				r.HasMultimodal, tokens, estimated, tc.maxTokens, blocked)

			assert.Equal(t, tc.wantBlock, blocked)
		})
	}
}

// ---------------------------------------------------------------------------
// Anthropic 协议场景测试
// ---------------------------------------------------------------------------

func TestAnthropicDetection(t *testing.T) {
	// OpenAI 请求不应触发 Anthropic 路径
	openaiBody := []byte(`{
		"messages": [{"role": "user", "content": [{"type": "text", "text": "hello"}]}],
		"tools": [{"type": "function", "function": {"name": "foo", "parameters": {}}}]
	}`)
	assert.False(t, hasAnthropicSpecificFields(openaiBody), "OpenAI content array + type=text 不应误判为 Anthropic")

	// Anthropic tools[].input_schema
	anthropicTools := []byte(`{
		"messages": [{"role": "user", "content": "hi"}],
		"tools": [{"name": "get_weather", "input_schema": {"type": "object"}}]
	}`)
	assert.True(t, hasAnthropicSpecificFields(anthropicTools), "tools[].input_schema 必须识别为 Anthropic")

	// Anthropic tool_use content block
	toolUseBody := []byte(`{
		"messages": [{"role": "assistant", "content": [
			{"type": "tool_use", "id": "tu_1", "name": "calc", "input": {"expr": "1+1"}}
		]}]
	}`)
	assert.True(t, hasAnthropicSpecificFields(toolUseBody), "content type=tool_use 必须识别为 Anthropic")

	// Anthropic tool_result content block
	toolResultBody := []byte(`{
		"messages": [{"role": "user", "content": [
			{"type": "tool_result", "tool_use_id": "tu_1", "content": "2"}
		]}]
	}`)
	assert.True(t, hasAnthropicSpecificFields(toolResultBody), "content type=tool_result 必须识别为 Anthropic")

	// 仅含 thinking block（无 tools）也应识别为 Anthropic
	thinkingOnly := []byte(`{
		"messages": [{"role": "assistant", "content": [
			{"type": "thinking", "thinking": "reasoning..."}
		]}]
	}`)
	assert.True(t, hasAnthropicSpecificFields(thinkingOnly), "thinking block 必须识别为 Anthropic")

	// 仅含 redacted_thinking block
	redactedOnly := []byte(`{
		"messages": [{"role": "assistant", "content": [
			{"type": "redacted_thinking", "data": "xxx"}
		]}]
	}`)
	assert.True(t, hasAnthropicSpecificFields(redactedOnly), "redacted_thinking block 必须识别为 Anthropic")

	// 仅含 document block
	docOnly := []byte(`{
		"messages": [{"role": "user", "content": [
			{"type": "document", "source": {"type": "text", "data": "..."}}
		]}]
	}`)
	assert.True(t, hasAnthropicSpecificFields(docOnly), "document block 必须识别为 Anthropic")

	// 仅含 search_result block
	searchOnly := []byte(`{
		"messages": [{"role": "user", "content": [
			{"type": "search_result", "title": "t", "content": []}
		]}]
	}`)
	assert.True(t, hasAnthropicSpecificFields(searchOnly), "search_result block 必须识别为 Anthropic")
}

func TestExtractAnthropicText_ToolUseAndResult(t *testing.T) {
	body := []byte(`{
		"system": "You are a helpful assistant",
		"messages": [
			{"role": "user", "content": "What is 2+2?"},
			{"role": "assistant", "content": [
				{"type": "text", "text": "Let me calculate that."},
				{"type": "tool_use", "id": "tu_1", "name": "calculator", "input": {"expression": "2+2"}}
			]},
			{"role": "user", "content": [
				{"type": "tool_result", "tool_use_id": "tu_1", "content": "4"}
			]},
			{"role": "assistant", "content": "The answer is 4."}
		],
		"tools": [
			{"name": "calculator", "description": "Evaluates math expressions", "input_schema": {"type": "object", "properties": {"expression": {"type": "string"}}}}
		]
	}`)

	r := extractPromptText(body)
	assert.False(t, r.HasMultimodal)
	// system
	assert.Contains(t, r.Text, "You are a helpful assistant")
	// messages content string
	assert.Contains(t, r.Text, "What is 2+2?")
	assert.Contains(t, r.Text, "The answer is 4.")
	// tool_use: name + input
	assert.Contains(t, r.Text, "calculator")
	assert.Contains(t, r.Text, "expression")
	assert.Contains(t, r.Text, "2+2")
	// tool_result: content string
	assert.Contains(t, r.Text, "4")
	// tools[].input_schema
	assert.Contains(t, r.Text, "Evaluates math expressions")
	// text block in assistant
	assert.Contains(t, r.Text, "Let me calculate that.")
}

func TestExtractAnthropicText_ToolResultContentArray(t *testing.T) {
	body := []byte(`{
		"messages": [
			{"role": "user", "content": [
				{"type": "tool_result", "tool_use_id": "tu_1", "content": [
					{"type": "text", "text": "Result line 1"},
					{"type": "text", "text": "Result line 2"}
				]}
			]}
		],
		"tools": [{"name": "dummy", "input_schema": {"type": "object"}}]
	}`)
	r := extractPromptText(body)
	assert.False(t, r.HasMultimodal)
	assert.Contains(t, r.Text, "Result line 1")
	assert.Contains(t, r.Text, "Result line 2")
}

func TestExtractAnthropicText_ImageMultimodal(t *testing.T) {
	body := []byte(`{
		"messages": [{"role": "user", "content": [
			{"type": "text", "text": "describe this"},
			{"type": "image", "source": {"type": "base64", "data": "..."}}
		]}],
		"tools": [{"name": "x", "input_schema": {}}]
	}`)
	r := extractPromptText(body)
	assert.True(t, r.HasMultimodal, "Anthropic image block 必须触发多模态放行")
}

func TestExtractAnthropicText_UnknownBlock(t *testing.T) {
	// 未知非文本 block（如 audio、unknown_binary）应触发多模态放行
	body := []byte(`{
		"messages": [{"role": "user", "content": [
			{"type": "text", "text": "listen to this"},
			{"type": "audio", "source": {"type": "base64", "data": "..."}}
		]}],
		"tools": [{"name": "x", "input_schema": {}}]
	}`)
	r := extractPromptText(body)
	assert.True(t, r.HasMultimodal, "未知非文本 block 必须触发多模态放行")
}

func TestExtractAnthropicText_ToolResultWithImage(t *testing.T) {
	// tool_result.content array 中包含非 text block 应触发多模态放行
	body := []byte(`{
		"messages": [
			{"role": "user", "content": [
				{"type": "tool_result", "tool_use_id": "tu_1", "content": [
					{"type": "text", "text": "here is the result"},
					{"type": "image", "source": {"type": "base64", "data": "..."}}
				]}
			]}
		],
		"tools": [{"name": "screenshot", "input_schema": {"type": "object"}}]
	}`)
	r := extractPromptText(body)
	assert.True(t, r.HasMultimodal, "tool_result 含非 text block 必须触发多模态放行")
}

func TestExtractAnthropicText_StringContent(t *testing.T) {
	// Anthropic 也支持 content 为纯字符串
	body := []byte(`{
		"system": [{"type": "text", "text": "system prompt"}],
		"messages": [{"role": "user", "content": "hello world"}],
		"tools": [{"name": "t1", "input_schema": {"type": "object"}}]
	}`)
	r := extractPromptText(body)
	assert.False(t, r.HasMultimodal)
	assert.Contains(t, r.Text, "system prompt")
	assert.Contains(t, r.Text, "hello world")
}

func TestExtractAnthropicText_ThinkingBlock(t *testing.T) {
	// Extended thinking block 应被计入，不触发多模态
	body := []byte(`{
		"messages": [
			{"role": "user", "content": "solve this"},
			{"role": "assistant", "content": [
				{"type": "thinking", "thinking": "Let me think about this step by step. First I need to consider the constraints and then work through the logic carefully."},
				{"type": "text", "text": "The answer is 42."}
			]}
		],
		"tools": [{"name": "x", "input_schema": {}}]
	}`)
	r := extractPromptText(body)
	assert.False(t, r.HasMultimodal, "thinking block 不应触发多模态")
	assert.Contains(t, r.Text, "step by step")
	assert.Contains(t, r.Text, "The answer is 42.")
}

func TestExtractAnthropicText_RedactedThinking(t *testing.T) {
	// Redacted thinking block 的 data 应被保守计入
	body := []byte(`{
		"messages": [
			{"role": "assistant", "content": [
				{"type": "redacted_thinking", "data": "abc123encrypteddatahere456"}
			]}
		],
		"tools": [{"name": "x", "input_schema": {}}]
	}`)
	r := extractPromptText(body)
	assert.False(t, r.HasMultimodal, "redacted_thinking 不应触发多模态")
	assert.Contains(t, r.Text, "abc123encrypteddatahere456")
}

func TestExtractAnthropicText_DocumentText(t *testing.T) {
	// document source.type=text 应被计入
	body := []byte(`{
		"messages": [{"role": "user", "content": [
			{"type": "document", "title": "report.txt", "source": {"type": "text", "data": "This is a very long document content that should be counted as input tokens."}}
		]}],
		"tools": [{"name": "x", "input_schema": {}}]
	}`)
	r := extractPromptText(body)
	assert.False(t, r.HasMultimodal, "text document 不应触发多模态")
	assert.Contains(t, r.Text, "report.txt")
	assert.Contains(t, r.Text, "very long document content")
}

func TestExtractAnthropicText_DocumentBase64(t *testing.T) {
	// document source.type=base64 应触发多模态放行
	body := []byte(`{
		"messages": [{"role": "user", "content": [
			{"type": "document", "title": "file.pdf", "source": {"type": "base64", "media_type": "application/pdf", "data": "..."}}
		]}],
		"tools": [{"name": "x", "input_schema": {}}]
	}`)
	r := extractPromptText(body)
	assert.True(t, r.HasMultimodal, "base64 document 应触发多模态放行")
}

func TestExtractAnthropicText_SearchResult(t *testing.T) {
	// search_result 的 title/source/content text blocks 应被计入
	body := []byte(`{
		"messages": [{"role": "user", "content": [
			{"type": "search_result", "title": "Higress Documentation", "source": "https://higress.io/docs", "content": [
				{"type": "text", "text": "Higress is a cloud-native API gateway."},
				{"type": "text", "text": "It supports WASM plugins for extensibility."}
			]}
		]}],
		"tools": [{"name": "x", "input_schema": {}}]
	}`)
	r := extractPromptText(body)
	assert.False(t, r.HasMultimodal, "search_result 不应触发多模态")
	assert.Contains(t, r.Text, "Higress Documentation")
	assert.Contains(t, r.Text, "https://higress.io/docs")
	assert.Contains(t, r.Text, "cloud-native API gateway")
	assert.Contains(t, r.Text, "WASM plugins")
}

// TestVerifyToolCallsAndResponseFormat 端到端验证：
// 真实场景请求体中的 tool_calls.arguments 和 response_format.json_schema
// 确实被纳入 token 统计，不会被漏算。
func TestVerifyToolCallsAndResponseFormat(t *testing.T) {
	require := assert.New(t)
	require.NoError(initEncoder())

	// 构造包含大量 tool_calls arguments 的多轮对话
	bodyWithToolCalls := []byte(`{
		"messages": [
			{"role": "user", "content": "help"},
			{"role": "assistant", "tool_calls": [{
				"id": "call_1", "type": "function",
				"function": {
					"name": "search_database",
					"arguments": "{\"query\":\"SELECT id, name, email, phone, address, created_at, updated_at FROM users WHERE status = active AND region IN (us-east, us-west, eu-west) ORDER BY created_at DESC LIMIT 100\"}"
				}
			}]},
			{"role": "tool", "content": "found 100 rows", "tool_call_id": "call_1"}
		]
	}`)

	// 同样的请求但不带 tool_calls（模拟修复前的漏算场景）
	bodyWithoutToolCalls := []byte(`{
		"messages": [
			{"role": "user", "content": "help"},
			{"role": "assistant"},
			{"role": "tool", "content": "found 100 rows", "tool_call_id": "call_1"}
		]
	}`)

	rWith := extractPromptText(bodyWithToolCalls)
	rWithout := extractPromptText(bodyWithoutToolCalls)

	tokensWithToolCalls := CountTokens(rWith.Text)
	tokensWithoutToolCalls := CountTokens(rWithout.Text)

	t.Logf("含 tool_calls: text_bytes=%d, tokens=%d", len(rWith.Text), tokensWithToolCalls)
	t.Logf("不含 tool_calls: text_bytes=%d, tokens=%d", len(rWithout.Text), tokensWithoutToolCalls)
	t.Logf("tool_calls 贡献的额外 tokens: %d", tokensWithToolCalls-tokensWithoutToolCalls)

	// tool_calls.arguments 包含大段 SQL，必须贡献显著的额外 token
	require.Greater(tokensWithToolCalls, tokensWithoutToolCalls+10,
		"tool_calls.arguments 必须被计入 token 统计")

	// 验证 response_format.json_schema 被统计
	bodyWithSchema := []byte(`{
		"messages": [{"role": "user", "content": "extract"}],
		"response_format": {
			"type": "json_schema",
			"json_schema": {
				"name": "extraction_result",
				"description": "A comprehensive schema for extracting structured order information including customer details and line items",
				"schema": {"type": "object", "properties": {"customer_name": {"type": "string"}, "order_id": {"type": "string"}, "items": {"type": "array", "items": {"type": "object", "properties": {"sku": {"type": "string"}, "qty": {"type": "integer"}, "price": {"type": "number"}}}}}}
			}
		}
	}`)

	bodyWithoutSchema := []byte(`{
		"messages": [{"role": "user", "content": "extract"}]
	}`)

	rSchema := extractPromptText(bodyWithSchema)
	rNoSchema := extractPromptText(bodyWithoutSchema)

	tokensWithSchema := CountTokens(rSchema.Text)
	tokensNoSchema := CountTokens(rNoSchema.Text)

	t.Logf("含 json_schema: text_bytes=%d, tokens=%d", len(rSchema.Text), tokensWithSchema)
	t.Logf("不含 json_schema: text_bytes=%d, tokens=%d", len(rNoSchema.Text), tokensNoSchema)
	t.Logf("json_schema 贡献的额外 tokens: %d", tokensWithSchema-tokensNoSchema)

	// json_schema 包含大段 schema 定义，必须贡献显著的额外 token
	require.Greater(tokensWithSchema, tokensNoSchema+20,
		"response_format.json_schema 必须被计入 token 统计")
}

func TestExtractPromptText_StringContent(t *testing.T) {
	body := []byte(`{
		"model": "gpt-4o",
		"messages": [
			{"role": "system", "content": "你是一个助手"},
			{"role": "user", "content": "Hello world"}
		]
	}`)
	r := extractPromptText(body)
	assert.False(t, r.HasMultimodal)
	assert.Contains(t, r.Text, "你是一个助手")
	assert.Contains(t, r.Text, "Hello world")
	assert.Contains(t, r.Text, "system")
	assert.Contains(t, r.Text, "user")
}

func TestExtractPromptText_ArrayContent(t *testing.T) {
	body := []byte(`{
		"messages": [
			{"role": "user", "content": [
				{"type": "text", "text": "describe this"},
				{"type": "text", "text": "in detail"}
			]}
		]
	}`)
	r := extractPromptText(body)
	assert.False(t, r.HasMultimodal)
	assert.Contains(t, r.Text, "describe this")
	assert.Contains(t, r.Text, "in detail")
}

func TestExtractPromptText_Multimodal(t *testing.T) {
	body := []byte(`{
		"messages": [
			{"role": "user", "content": [
				{"type": "text", "text": "what is in this image?"},
				{"type": "image_url", "image_url": {"url": "https://example.com/cat.jpg"}}
			]}
		]
	}`)
	r := extractPromptText(body)
	assert.True(t, r.HasMultimodal, "image_url 必须触发多模态放行")
}

func TestExtractPromptText_Tools(t *testing.T) {
	body := []byte(`{
		"messages": [{"role": "user", "content": "查询天气"}],
		"tools": [
			{
				"type": "function",
				"function": {
					"name": "get_weather",
					"description": "获取指定城市的天气信息",
					"parameters": {"type": "object", "properties": {"city": {"type": "string"}}}
				}
			}
		]
	}`)
	r := extractPromptText(body)
	assert.False(t, r.HasMultimodal)
	assert.Contains(t, r.Text, "查询天气")
	assert.Contains(t, r.Text, "get_weather")
	assert.Contains(t, r.Text, "获取指定城市的天气信息")
	// parameters 整体序列化进入计数
	assert.Contains(t, r.Text, "city")
}

func TestExtractPromptText_TopLevelSystem(t *testing.T) {
	body := []byte(`{
		"system": "你是有帮助的助手",
		"messages": [{"role": "user", "content": "hi"}]
	}`)
	r := extractPromptText(body)
	assert.Contains(t, r.Text, "你是有帮助的助手")
	assert.Contains(t, r.Text, "hi")
}

func TestExtractPromptText_Empty(t *testing.T) {
	r := extractPromptText([]byte(`{}`))
	assert.False(t, r.HasMultimodal)
	assert.Equal(t, "", r.Text)
}

func TestExtractPromptText_ToolCalls(t *testing.T) {
	body := []byte(`{
		"messages": [
			{"role": "user", "content": "查询订单"},
			{"role": "assistant", "tool_calls": [
				{"id": "call_1", "type": "function", "function": {
					"name": "lookup_order",
					"arguments": "{\"order_id\":\"12345\",\"detail\":true}"
				}}
			]},
			{"role": "tool", "content": "订单已发货", "tool_call_id": "call_1"}
		]
	}`)
	r := extractPromptText(body)
	assert.False(t, r.HasMultimodal)
	assert.Contains(t, r.Text, "lookup_order")
	assert.Contains(t, r.Text, "order_id")
	assert.Contains(t, r.Text, "12345")
	assert.Contains(t, r.Text, "订单已发货")
}

func TestExtractPromptText_ResponseFormat(t *testing.T) {
	body := []byte(`{
		"messages": [{"role": "user", "content": "extract info"}],
		"response_format": {
			"type": "json_schema",
			"json_schema": {
				"name": "order_schema",
				"description": "Schema for order extraction",
				"schema": {"type": "object", "properties": {"id": {"type": "string"}}}
			}
		}
	}`)
	r := extractPromptText(body)
	assert.False(t, r.HasMultimodal)
	assert.Contains(t, r.Text, "order_schema")
	assert.Contains(t, r.Text, "Schema for order extraction")
	assert.Contains(t, r.Text, "properties")
}

// TestCountTokens 只做基本可用性断言，避免在单测中绑定具体词表细节。
func TestCountTokens(t *testing.T) {
	require := assert.New(t)
	require.NoError(initEncoder())

	require.Equal(0, CountTokens(""), "空字符串返回 0")
	require.Greater(CountTokens("Hello world"), 0)
	require.Greater(CountTokens("中文测试"), 0)

	// 重复文本 token 数应近似线性
	once := CountTokens("hello")
	thrice := CountTokens("hello hello hello")
	require.Greater(thrice, once)
}

// TestBlockDecision 拦截判定逻辑（×buffer_ratio 与阈值比较）
// 直接用真实编码器，构造 prompt 控制估算值的相对位置
func TestBlockDecision(t *testing.T) {
	require := assert.New(t)
	require.NoError(initEncoder())

	// 构造一段已知 token 数的文本
	prompt := "Hello world. This is a test prompt for token counting."
	rawTokens := CountTokens(prompt)
	require.Greater(rawTokens, 0)

	cases := []struct {
		name        string
		bufferRatio float64
		threshold   int
		shouldBlock bool
	}{
		{"远低于阈值 → 放行", 1.10, 100000, false},
		{"略低于阈值 → 放行", 1.10, rawTokens * 2, false},
		{"恰好等于阈值 → 放行（>不>=）", 1.0, rawTokens, false},
		{"略超阈值 → 拦截", 1.10, 1, true},
		{"buffer_ratio 抬高致超阈值", 10.0, rawTokens + 1, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			estimated := int(float64(rawTokens) * tc.bufferRatio)
			got := estimated > tc.threshold
			assert.Equal(t, tc.shouldBlock, got,
				"raw=%d ratio=%.2f estimated=%d threshold=%d",
				rawTokens, tc.bufferRatio, estimated, tc.threshold)
		})
	}
}
