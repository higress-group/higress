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

package utils

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/higress-group/wasm-go/pkg/wrapper"
)

const ctxMCPResultAdapter = "mcp_result_adapter"

// MCPResultAdapter transforms a successful MCP response result before it is
// serialized. Servers install an adapter only for request profiles that need
// response shaping; asynchronous tool callbacks retain it on the HTTP context.
type MCPResultAdapter func(map[string]any) map[string]any

// SetMCPResultAdapter installs a request-scoped MCP success-result adapter.
func SetMCPResultAdapter(ctx wrapper.HttpContext, adapter MCPResultAdapter) {
	ctx.SetContext(ctxMCPResultAdapter, adapter)
}

// ApplyMCPResultAdapter applies the request-scoped adapter, if any. Keeping the
// adapter on the HTTP context allows REST callbacks to use it after Call has
// returned.
func ApplyMCPResultAdapter(ctx wrapper.HttpContext, result map[string]any) map[string]any {
	adapter, ok := ctx.GetContext(ctxMCPResultAdapter).(MCPResultAdapter)
	if !ok || adapter == nil {
		return result
	}
	return adapter(result)
}

func OnMCPResponseSuccess(ctx wrapper.HttpContext, result map[string]any, debugInfo string) {
	OnJsonRpcResponseSuccess(ctx, ApplyMCPResultAdapter(ctx, result), debugInfo)
	// TODO: support pub to redis when use POST + SSE
}

func OnMCPResponseError(ctx wrapper.HttpContext, err error, code int, debugInfo string) {
	OnJsonRpcResponseError(ctx, err, code, debugInfo)
	// TODO: support pub to redis when use POST + SSE
}

func OnMCPToolCallSuccess(ctx wrapper.HttpContext, content []map[string]any, debugInfo string) {
	OnMCPResponseSuccess(ctx, map[string]any{
		"content": content,
		"isError": false,
	}, debugInfo)
}

// OnMCPToolCallSuccessWithStructuredContent sends a successful MCP tool response with structured content
// According to MCP spec, structuredContent is a field in tool results, not a capability
func OnMCPToolCallSuccessWithStructuredContent(ctx wrapper.HttpContext, content []map[string]any, structuredContent json.RawMessage, debugInfo string) {
	response := map[string]any{
		"content": content,
		"isError": false,
	}
	if structuredContent != nil && len(structuredContent) > 0 {
		response["structuredContent"] = structuredContent
	}
	OnMCPResponseSuccess(ctx, response, debugInfo)
}

func OnMCPToolCallError(ctx wrapper.HttpContext, err error, debugInfo ...string) {
	responseDebugInfo := fmt.Sprintf("mcp:tools/call:error(%s)", err)
	if len(debugInfo) > 0 {
		responseDebugInfo = debugInfo[0]
	}
	OnMCPResponseSuccess(ctx, map[string]any{
		"content": []map[string]any{
			{
				"type": "text",
				"text": err.Error(),
			},
		},
		"isError": true,
	}, responseDebugInfo)
}

func SendMCPToolTextResult(ctx wrapper.HttpContext, result string, debugInfo ...string) {
	responseDebugInfo := "mcp:tools/call::result"
	if len(debugInfo) > 0 {
		responseDebugInfo = debugInfo[0]
	}
	OnMCPToolCallSuccess(ctx, []map[string]any{
		{
			"type": "text",
			"text": result,
		},
	}, responseDebugInfo)
}

func SendMCPToolImageResult(ctx wrapper.HttpContext, image []byte, contentType string, debugInfo ...string) {
	responseDebugInfo := "mcp:tools/call::result"
	if len(debugInfo) > 0 {
		responseDebugInfo = debugInfo[0]
	}

	content := []map[string]any{
		{
			"type":     "image",
			"data":     base64.StdEncoding.EncodeToString(image),
			"mimeType": contentType,
		},
	}

	// Use traditional response format since no structured data is provided
	OnMCPToolCallSuccess(ctx, content, responseDebugInfo)
}

// SendMCPToolTextResultWithStructuredContent sends a tool result with both text content and structured content
// According to MCP spec, for backward compatibility, tools that return structured content
// SHOULD also return the serialized JSON in a TextContent block
func SendMCPToolTextResultWithStructuredContent(ctx wrapper.HttpContext, textResult string, structuredContent json.RawMessage, debugInfo ...string) {
	responseDebugInfo := "mcp:tools/call::result"
	if len(debugInfo) > 0 {
		responseDebugInfo = debugInfo[0]
	}
	content := []map[string]any{
		{
			"type": "text",
			"text": textResult,
		},
	}
	OnMCPToolCallSuccessWithStructuredContent(ctx, content, structuredContent, responseDebugInfo)
}
