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
	"strings"

	"github.com/tidwall/gjson"
)

// extractResult 文本抽取结果
type extractResult struct {
	// Text 拼接后的所有可计 token 文本
	Text string
	// HasMultimodal 是否检测到非 text 类型 part（image_url/audio/...），命中即放行
	HasMultimodal bool
}

// extractPromptText 从请求体抽取所有需要计入 input tokens 的文本。
//
// 协议识别策略：通过检测 Anthropic 特有字段（tools[].input_schema、
// content type=tool_use/tool_result/thinking/redacted_thinking/document/search_result）
// 来判断是否为 Anthropic 协议请求。
// 普通纯文本请求即使是 Anthropic 格式，走 OpenAI 路径也能正确统计。
//
// 多模态降级：非文本二进制内容（image/audio/base64 document）视为多模态，
// 整个请求放行。
func extractPromptText(body []byte) extractResult {
	if hasAnthropicSpecificFields(body) {
		return extractAnthropicText(body)
	}
	return extractOpenAIText(body)
}

// hasAnthropicSpecificFields 保守识别 Anthropic 协议特有字段。
//
// 强信号（任一命中即判定为 Anthropic）：
//   - tools[].input_schema 存在（OpenAI 用 tools[].function.parameters）
//   - tools[] 中存在无 function 包装但有 name 的条目（Anthropic server tools）
//   - messages[].content[] 中含 type=tool_use/tool_result/thinking/redacted_thinking/document/search_result
//
// 不以 content array + type=text 判断（OpenAI 多模态也有此结构）。
func hasAnthropicSpecificFields(body []byte) bool {
	// 检查 tools[]
	tools := gjson.GetBytes(body, "tools").Array()
	for _, tool := range tools {
		if tool.Get("input_schema").Exists() {
			return true
		}
		if tool.Get("name").Exists() && !tool.Get("function").Exists() {
			return true
		}
	}
	// 检查 messages[].content[] 中的 Anthropic 特有 block types
	messages := gjson.GetBytes(body, "messages").Array()
	for _, msg := range messages {
		content := msg.Get("content")
		if !content.IsArray() {
			continue
		}
		for _, part := range content.Array() {
			switch part.Get("type").String() {
			case "tool_use", "tool_result", "thinking", "redacted_thinking", "document", "search_result":
				return true
			}
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// OpenAI Chat Completions extractor
// ---------------------------------------------------------------------------

// extractOpenAIText 从 OpenAI Chat Completions 请求体抽取文本。
//
// 协议参考：https://platform.openai.com/docs/api-reference/chat/create
//
// 抽取范围：
//   - messages[].role / name / content（string 或 text parts array）
//   - messages[].tool_calls[].function.{name, arguments}
//   - tools[].function.{name, description, parameters}
//   - response_format.json_schema.{name, description, schema}
//   - 顶层 system 字段（兼容将 system prompt 放在顶层的协议）
func extractOpenAIText(body []byte) extractResult {
	var sb strings.Builder
	result := extractResult{}

	// 1. messages[]
	messages := gjson.GetBytes(body, "messages").Array()
	for _, msg := range messages {
		if name := msg.Get("name").String(); name != "" {
			sb.WriteString(name)
			sb.WriteByte('\n')
		}
		if role := msg.Get("role").String(); role != "" {
			sb.WriteString(role)
			sb.WriteByte('\n')
		}
		content := msg.Get("content")
		switch {
		case content.Type == gjson.String:
			sb.WriteString(content.String())
			sb.WriteByte('\n')
		case content.IsArray():
			for _, part := range content.Array() {
				partType := part.Get("type").String()
				if partType == "text" {
					sb.WriteString(part.Get("text").String())
					sb.WriteByte('\n')
					continue
				}
				// 任意非 text part → 多模态，立即返回触发放行
				result.HasMultimodal = true
				return result
			}
		}
		// messages[].tool_calls[]（多轮对话中 assistant 的工具调用参数）
		toolCalls := msg.Get("tool_calls").Array()
		for _, tc := range toolCalls {
			fn := tc.Get("function")
			if !fn.Exists() {
				continue
			}
			if name := fn.Get("name").String(); name != "" {
				sb.WriteString(name)
				sb.WriteByte('\n')
			}
			if args := fn.Get("arguments").String(); args != "" {
				sb.WriteString(args)
				sb.WriteByte('\n')
			}
		}
	}

	// 2. tools[]
	tools := gjson.GetBytes(body, "tools").Array()
	for _, tool := range tools {
		fn := tool.Get("function")
		if !fn.Exists() {
			continue
		}
		if name := fn.Get("name").String(); name != "" {
			sb.WriteString(name)
			sb.WriteByte('\n')
		}
		if desc := fn.Get("description").String(); desc != "" {
			sb.WriteString(desc)
			sb.WriteByte('\n')
		}
		if params := fn.Get("parameters"); params.Exists() {
			sb.WriteString(params.Raw)
			sb.WriteByte('\n')
		}
	}

	// 3. response_format.json_schema（结构化输出 schema 计入 input tokens）
	jsonSchema := gjson.GetBytes(body, "response_format.json_schema")
	if jsonSchema.Exists() {
		if name := jsonSchema.Get("name").String(); name != "" {
			sb.WriteString(name)
			sb.WriteByte('\n')
		}
		if desc := jsonSchema.Get("description").String(); desc != "" {
			sb.WriteString(desc)
			sb.WriteByte('\n')
		}
		if schema := jsonSchema.Get("schema"); schema.Exists() {
			sb.WriteString(schema.Raw)
			sb.WriteByte('\n')
		}
	}

	// 4. 顶层 system 字段
	extractTopLevelSystem(body, &sb)

	result.Text = sb.String()
	return result
}

// ---------------------------------------------------------------------------
// Anthropic Messages extractor
// ---------------------------------------------------------------------------

// extractAnthropicText 从 Anthropic Messages 请求体抽取文本。
//
// 协议参考：https://docs.anthropic.com/en/api/messages
//
// 抽取范围：
//   - system：string 或 text block array
//   - messages[].role
//   - messages[].content：string 或 content block array
//   - type=text → text 字段
//   - type=tool_use → name + input（raw JSON）
//   - type=tool_result → content（string 或 content block array）
//   - type=thinking → thinking 字段
//   - type=redacted_thinking → data 字段
//   - type=document → source.type=text 时计入，其他视为多模态
//   - type=search_result → title + source + content text blocks
//   - tools[].name / description / type / input_schema（raw JSON）
func extractAnthropicText(body []byte) extractResult {
	var sb strings.Builder
	result := extractResult{}

	// 1. system（string 或 text block array）
	extractTopLevelSystem(body, &sb)

	// 2. messages[]
	messages := gjson.GetBytes(body, "messages").Array()
	for _, msg := range messages {
		if role := msg.Get("role").String(); role != "" {
			sb.WriteString(role)
			sb.WriteByte('\n')
		}
		content := msg.Get("content")
		switch {
		case content.Type == gjson.String:
			sb.WriteString(content.String())
			sb.WriteByte('\n')
		case content.IsArray():
			for _, part := range content.Array() {
				if extractAnthropicContentBlock(part, &sb) {
					result.HasMultimodal = true
					return result
				}
			}
		}
	}

	// 3. tools[]
	tools := gjson.GetBytes(body, "tools").Array()
	for _, tool := range tools {
		if tp := tool.Get("type").String(); tp != "" {
			sb.WriteString(tp)
			sb.WriteByte('\n')
		}
		if name := tool.Get("name").String(); name != "" {
			sb.WriteString(name)
			sb.WriteByte('\n')
		}
		if desc := tool.Get("description").String(); desc != "" {
			sb.WriteString(desc)
			sb.WriteByte('\n')
		}
		if schema := tool.Get("input_schema"); schema.Exists() {
			sb.WriteString(schema.Raw)
			sb.WriteByte('\n')
		}
	}

	result.Text = sb.String()
	return result
}

// extractAnthropicContentBlock 统一处理单个 Anthropic content block。
// 顶层 messages[].content[] 和 tool_result.content[] 均复用此函数。
// 返回 true 表示发现多模态内容（需放行）。
func extractAnthropicContentBlock(part gjson.Result, sb *strings.Builder) bool {
	t := part.Get("type").String()
	switch t {
	case "text":
		sb.WriteString(part.Get("text").String())
		sb.WriteByte('\n')
	case "tool_use":
		if name := part.Get("name").String(); name != "" {
			sb.WriteString(name)
			sb.WriteByte('\n')
		}
		if input := part.Get("input"); input.Exists() {
			sb.WriteString(input.Raw)
			sb.WriteByte('\n')
		}
	case "tool_result":
		content := part.Get("content")
		switch {
		case content.Type == gjson.String:
			sb.WriteString(content.String())
			sb.WriteByte('\n')
		case content.IsArray():
			for _, block := range content.Array() {
				if extractAnthropicContentBlock(block, sb) {
					return true
				}
			}
		}
	case "thinking":
		if thinking := part.Get("thinking").String(); thinking != "" {
			sb.WriteString(thinking)
			sb.WriteByte('\n')
		}
	case "redacted_thinking":
		if data := part.Get("data").String(); data != "" {
			sb.WriteString(data)
			sb.WriteByte('\n')
		}
	case "document":
		return extractAnthropicDocument(part, sb)
	case "search_result":
		extractAnthropicSearchResult(part, sb)
	default:
		// 真正的非文本 block（image/audio/等）视为多模态
		return true
	}
	return false
}

// extractAnthropicDocument 处理 Anthropic document content block。
// source.type=="text" 时抽取文本内容，其他类型（base64/url/file）视为多模态。
// 返回 true 表示多模态（需放行）。
func extractAnthropicDocument(part gjson.Result, sb *strings.Builder) bool {
	sourceType := part.Get("source.type").String()
	if sourceType == "text" {
		// 纯文本文档，计入 token
		if title := part.Get("title").String(); title != "" {
			sb.WriteString(title)
			sb.WriteByte('\n')
		}
		if data := part.Get("source.data").String(); data != "" {
			sb.WriteString(data)
			sb.WriteByte('\n')
		}
		return false
	}
	// base64/url/file 等非文本源 → 多模态
	return true
}

// extractAnthropicSearchResult 处理 Anthropic search_result content block。
// 抽取 title、source 和 content[] 中的 text blocks。
func extractAnthropicSearchResult(part gjson.Result, sb *strings.Builder) {
	if title := part.Get("title").String(); title != "" {
		sb.WriteString(title)
		sb.WriteByte('\n')
	}
	if source := part.Get("source").String(); source != "" {
		sb.WriteString(source)
		sb.WriteByte('\n')
	}
	// content 是 text blocks 数组
	contentArr := part.Get("content").Array()
	for _, block := range contentArr {
		if block.Get("type").String() == "text" {
			sb.WriteString(block.Get("text").String())
			sb.WriteByte('\n')
		}
	}
}

// ---------------------------------------------------------------------------
// 共用辅助函数
// ---------------------------------------------------------------------------

// extractTopLevelSystem 抽取顶层 system 字段（string 或 text block array）。
// OpenAI 和 Anthropic 均可能使用顶层 system。
func extractTopLevelSystem(body []byte, sb *strings.Builder) {
	sys := gjson.GetBytes(body, "system")
	if !sys.Exists() {
		return
	}
	switch {
	case sys.Type == gjson.String:
		sb.WriteString(sys.String())
		sb.WriteByte('\n')
	case sys.IsArray():
		for _, part := range sys.Array() {
			if part.Get("type").String() == "text" {
				sb.WriteString(part.Get("text").String())
				sb.WriteByte('\n')
			}
		}
	}
}
