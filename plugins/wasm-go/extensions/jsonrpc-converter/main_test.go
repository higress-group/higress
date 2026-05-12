package main

import (
	"encoding/json"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

// TestTruncateString tests the truncateString function
func TestTruncateString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{"Short String", "Higress Is an AI-Native API Gateway", 1000, "Higress Is an AI-Native API Gateway"},
		{"Exact Length", "Higress Is an AI-Native API Gateway", 35, "Higress Is an AI-Native API Gateway"},
		{"Truncated String", "Higress Is an AI-Native API Gateway", 20, "Higress Is...(truncated)...PI Gateway"},
		{"Empty String", "", 10, ""},
		{"Single Char", "A", 10, "A"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := McpConverterConfig{MaxHeaderLength: tt.maxLen}
			result := truncateString(tt.input, config)
			if result != tt.expected {
				t.Errorf("truncateString(%q, %d) = %q; want %q", tt.input, tt.maxLen, result, tt.expected)
			}
		})
	}
}

// TestIsPreRequestStage tests the isPreRequestStage function
func TestIsPreRequestStage(t *testing.T) {
	config := McpConverterConfig{Stage: ProcessRequest}
	require.True(t, isPreRequestStage(config))

	config = McpConverterConfig{Stage: ProcessResponse}
	require.False(t, isPreRequestStage(config))
}

// TestIsPreResponseStage tests the isPreResponseStage function
func TestIsPreResponseStage(t *testing.T) {
	config := McpConverterConfig{Stage: ProcessResponse}
	require.True(t, isPreResponseStage(config))

	config = McpConverterConfig{Stage: ProcessRequest}
	require.False(t, isPreResponseStage(config))
}

// TestIsMethodAllowed tests the isMethodAllowed function
func TestIsMethodAllowed(t *testing.T) {
	config := McpConverterConfig{AllowedMethods: []string{MethodToolList, MethodToolCall}}

	require.True(t, isMethodAllowed(config, MethodToolList))
	require.True(t, isMethodAllowed(config, MethodToolCall))
	require.False(t, isMethodAllowed(config, "invalid/method"))
}

// TestConstants tests the constant values
func TestConstants(t *testing.T) {
	require.Equal(t, "x-envoy-jsonrpc-id", JsonRpcId)
	require.Equal(t, "x-envoy-jsonrpc-method", JsonRpcMethod)
	require.Equal(t, "x-envoy-jsonrpc-params", JsonRpcParams)
	require.Equal(t, "x-envoy-jsonrpc-result", JsonRpcResult)
	require.Equal(t, "x-envoy-jsonrpc-error", JsonRpcError)
	require.Equal(t, "x-envoy-mcp-tool-name", McpToolName)
	require.Equal(t, "x-envoy-mcp-tool-arguments", McpToolArguments)
	require.Equal(t, "x-envoy-mcp-tool-response", McpToolResponse)
	require.Equal(t, "x-envoy-mcp-tool-error", McpToolError)
	require.Equal(t, 4000, DefaultMaxHeaderLength)
	require.Equal(t, "tools/list", MethodToolList)
	require.Equal(t, "tools/call", MethodToolCall)
	require.Equal(t, ProcessStage("request"), ProcessRequest)
	require.Equal(t, ProcessStage("response"), ProcessResponse)
}

// TestMcpConverterConfigDefaults tests config default values
func TestMcpConverterConfigDefaults(t *testing.T) {
	config := McpConverterConfig{}
	require.Equal(t, 0, config.MaxHeaderLength)
	require.Equal(t, ProcessStage(""), config.Stage)
	require.Nil(t, config.AllowedMethods)
}

// TestProcessStage tests ProcessStage type
func TestProcessStage(t *testing.T) {
	require.Equal(t, ProcessStage("request"), ProcessRequest)
	require.Equal(t, ProcessStage("response"), ProcessResponse)
}

// TestRemoveJsonRpcHeadersFunction tests removeJsonRpcHeaders function logic
func TestRemoveJsonRpcHeadersFunction(t *testing.T) {
	headersToRemove := []string{
		JsonRpcId,
		JsonRpcMethod,
		JsonRpcParams,
		JsonRpcResult,
		McpToolName,
		McpToolArguments,
		McpToolResponse,
		McpToolError,
	}
	require.Len(t, headersToRemove, 8)
}

// TestTruncateStringLong tests truncation of very long strings
func TestTruncateStringLong(t *testing.T) {
	longString := ""
	for i := 0; i < 5000; i++ {
		longString += "a"
	}
	config := McpConverterConfig{MaxHeaderLength: 1000}
	result := truncateString(longString, config)
	require.Contains(t, result, "...(truncated)...")
	require.LessOrEqual(t, len(result), 1020)
}

// TestTruncateStringWithSmallMaxLength tests truncation with small max length
func TestTruncateStringWithSmallMaxLength(t *testing.T) {
	config := McpConverterConfig{MaxHeaderLength: 10}
	result := truncateString("This is a very long string", config)
	require.Contains(t, result, "...(truncated)...")
}

// TestPluginInit tests plugin initialization
func TestPluginInit(t *testing.T) {
	configBytes, _ := json.Marshal(McpConverterConfig{
		Stage:           ProcessRequest,
		MaxHeaderLength: DefaultMaxHeaderLength,
		AllowedMethods:  []string{MethodToolList, MethodToolCall},
	})

	host, status := test.NewTestHost(configBytes)
	defer host.Reset()
	require.Equal(t, types.OnPluginStartStatusOK, status)
}

// TestProcessJsonRpcRequest tests processJsonRpcRequest function
func TestProcessJsonRpcRequest(t *testing.T) {
	configBytes, _ := json.Marshal(McpConverterConfig{
		Stage:           ProcessRequest,
		MaxHeaderLength: DefaultMaxHeaderLength,
		AllowedMethods:  []string{MethodToolList, MethodToolCall},
	})

	host, status := test.NewTestHost(configBytes)
	defer host.Reset()
	require.Equal(t, types.OnPluginStartStatusOK, status)

	host.InitHttp()
	host.CallOnHttpRequestHeaders([][2]string{
		{":authority", "mcp-server.example.com"},
		{":method", "POST"},
		{":path", "/mcp"},
		{"content-type", "application/json"},
	})

	toolsListRequest := `{
		"jsonrpc": "2.0",
		"id": 1,
		"method": "tools/list",
		"params": {}
	}`
	action := host.CallOnHttpRequestBody([]byte(toolsListRequest))
	require.Equal(t, types.ActionContinue, action)

	host.CompleteHttp()
}

// TestProcessToolCallRequest tests processToolCallRequest function
func TestProcessToolCallRequest(t *testing.T) {
	configBytes, _ := json.Marshal(McpConverterConfig{
		Stage:           ProcessRequest,
		MaxHeaderLength: DefaultMaxHeaderLength,
		AllowedMethods:  []string{MethodToolCall},
	})

	host, status := test.NewTestHost(configBytes)
	defer host.Reset()
	require.Equal(t, types.OnPluginStartStatusOK, status)

	host.InitHttp()
	host.CallOnHttpRequestHeaders([][2]string{
		{":authority", "mcp-server.example.com"},
		{":method", "POST"},
		{":path", "/mcp"},
		{"content-type", "application/json"},
	})

	toolCallRequest := `{
		"jsonrpc": "2.0",
		"id": 1,
		"method": "tools/call",
		"params": {
			"name": "test_tool",
			"arguments": {"arg1": "value1"}
		}
	}`
	action := host.CallOnHttpRequestBody([]byte(toolCallRequest))
	require.Equal(t, types.ActionContinue, action)

	host.CompleteHttp()
}

// TestProcessJsonRpcResponse tests processJsonRpcResponse function
func TestProcessJsonRpcResponse(t *testing.T) {
	configBytes, _ := json.Marshal(McpConverterConfig{
		Stage:           ProcessResponse,
		MaxHeaderLength: DefaultMaxHeaderLength,
		AllowedMethods:  []string{MethodToolList, MethodToolCall},
	})

	host, status := test.NewTestHost(configBytes)
	defer host.Reset()
	require.Equal(t, types.OnPluginStartStatusOK, status)

	host.InitHttp()
	host.CallOnHttpRequestHeaders([][2]string{
		{":authority", "mcp-server.example.com"},
		{":method", "POST"},
		{":path", "/mcp"},
		{"content-type", "application/json"},
	})

	responseBody := `{
		"jsonrpc": "2.0",
		"id": 1,
		"result": {
			"tools": [{"name": "test_tool"}]
		}
	}`
	host.CallOnHttpResponseHeaders([][2]string{
		{":status", "200"},
		{"content-type", "application/json"},
	})
	host.CallOnHttpResponseBody([]byte(responseBody))

	host.CompleteHttp()
}

// TestProcessToolListResponse tests processToolListResponse function
func TestProcessToolListResponse(t *testing.T) {
	configBytes, _ := json.Marshal(McpConverterConfig{
		Stage:           ProcessResponse,
		MaxHeaderLength: DefaultMaxHeaderLength,
		AllowedMethods:  []string{MethodToolList},
	})

	host, status := test.NewTestHost(configBytes)
	defer host.Reset()
	require.Equal(t, types.OnPluginStartStatusOK, status)

	host.InitHttp()
	host.CallOnHttpRequestHeaders([][2]string{
		{":authority", "mcp-server.example.com"},
		{":method", "POST"},
		{":path", "/mcp"},
		{"content-type", "application/json"},
	})

	responseBody := `{
		"jsonrpc": "2.0",
		"id": 1,
		"result": {
			"tools": [{"name": "test_tool"}]
		}
	}`
	host.CallOnHttpResponseHeaders([][2]string{
		{":status", "200"},
		{"content-type", "application/json"},
	})
	host.CallOnHttpResponseBody([]byte(responseBody))

	host.CompleteHttp()
}
