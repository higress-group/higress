package main

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp"
	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/utils"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

func main() {}

const (
	JsonRpcId        = "x-envoy-jsonrpc-id"
	JsonRpcMethod    = "x-envoy-jsonrpc-method"
	JsonRpcParams    = "x-envoy-jsonrpc-params"
	JsonRpcResult    = "x-envoy-jsonrpc-result"
	JsonRpcError     = "x-envoy-jsonrpc-error"
	McpToolName      = "x-envoy-mcp-tool-name"
	McpToolArguments = "x-envoy-mcp-tool-arguments"
	McpToolResponse  = "x-envoy-mcp-tool-response"
	McpToolError     = "x-envoy-mcp-tool-error"

	DefaultMaxHeaderLength = 4000         // default max length for truncation
	MethodToolList         = "tools/list" // default method for tool list
	MethodToolCall         = "tools/call" // default method for tool call
)

type ProcessStage string

const (
	ProcessRequest  ProcessStage = "request"
	ProcessResponse ProcessStage = "response"
)

type McpConverterConfig struct {
	Stage           ProcessStage `json:"stage"`
	MaxHeaderLength int          `json:"max_header_length,omitempty"`
	AllowedMethods  []string     `json:"allowed_methods,omitempty"` // optional, for future use
}

func init() {
	mcp.LoadMCPFilter(
		mcp.FilterName("jsonrpc-converter"),
		mcp.SetConfigParser(parseConfig),
		mcp.SetJsonRpcRequestFilter(processJsonRpcRequest),
		mcp.SetJsonRpcResponseFilter(processJsonRpcResponse),
		mcp.SetToolListResponseFilter(processToolListResponse),
		mcp.SetToolCallRequestFilter(processToolCallRequest),
		mcp.SetToolCallResponseFilter(processToolCallResponse),
	)
	mcp.InitMCPFilter()
}

func parseConfig(configBytes []byte, filterConfig *any) error {
	var config McpConverterConfig
	if err := json.Unmarshal(configBytes, &config); err != nil {
		return fmt.Errorf("failed to parse mcp-converter config: %v", err)
	}
	// validate stage
	if config.Stage != ProcessRequest && config.Stage != ProcessResponse {
		return fmt.Errorf("invalid mcp-converter stage: %s, must be 'request' or 'response'", config.Stage)
	}
	// validate length
	if config.MaxHeaderLength <= 0 {
		config.MaxHeaderLength = DefaultMaxHeaderLength
	}
	// validate allowed methods
	if len(config.AllowedMethods) == 0 {
		config.AllowedMethods = []string{MethodToolList, MethodToolCall}
	}
	log.Infof("MCP Converter config parsed successfully, stage: %s", config.Stage)
	*filterConfig = config
	return nil
}

func isPreRequestStage(config any) bool {
	return config.(McpConverterConfig).Stage == ProcessRequest
}

func isPreResponseStage(config any) bool {
	return config.(McpConverterConfig).Stage == ProcessResponse
}

func isMethodAllowed(config any, method string) bool {
	allowedMethods := config.(McpConverterConfig).AllowedMethods
	return slices.Contains(allowedMethods, method)
}

// Remove jsonrpc headers
func removeJsonRpcHeaders(isRequest bool) {
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
	for _, header := range headersToRemove {
		var err error
		if isRequest {
			err = proxywasm.RemoveHttpRequestHeader(header)
		} else {
			err = proxywasm.RemoveHttpResponseHeader(header)
		}
		if err != nil {
			log.Errorf("failed to remove header %s: %v", header, err)
		}
	}
}

// Insert jsonrpc headers
func insertJsonRpcHeaders(isRequest bool, config any, name string, value string) {
	if value == "" {
		log.Debugf("Skipping insertion of empty header %s", name)
		return
	}
	truncatedValue := truncateString(value, config)
	var err error
	if isRequest {
		err = proxywasm.ReplaceHttpRequestHeader(name, truncatedValue)
	} else {
		err = proxywasm.ReplaceHttpResponseHeader(name, truncatedValue)
	}
	if err != nil {
		log.Errorf("failed to insert header %s: %v", name, err)
	}
}

func printHeaders(stage ProcessStage, s string) {
	var err error
	var headersNow any
	switch stage {
	case ProcessRequest:
		headersNow, err = proxywasm.GetHttpRequestHeaders()
	case ProcessResponse:
		headersNow, err = proxywasm.GetHttpResponseHeaders()
	}
	if err != nil {
		log.Errorf("PrintHeaders %s: failed to get request headers: %v", s, err)
		return
	}
	log.Debugf("PrintHeaders %s: %v", s, headersNow)
}

// truncates a string to a maximum length of 4000 characters.
func truncateString(s string, config any) string {
	length := config.(McpConverterConfig).MaxHeaderLength
	if len(s) <= length {
		return s
	}
	prefix := s[:length/2]
	suffix := s[len(s)-length/2:]

	return fmt.Sprintf("%s...(truncated)...%s", prefix, suffix)
}

func processJsonRpcRequest(context wrapper.HttpContext, config any, id utils.JsonRpcID, method string, params gjson.Result, rawBody []byte) types.Action {
	if isPreResponseStage(config) {
		// pre-response removes request headers, which are added by pre-request
		removeJsonRpcHeaders(true)
		return types.ActionContinue
	}

	if !isMethodAllowed(config, method) {
		log.Debugf("[JsonRpcRequest] Method %s is not allowed, skipping processing", method)
		return types.ActionContinue
	}

	// Set common headers, JsonRpcId, JsonRpcMethod
	insertJsonRpcHeaders(true, config, JsonRpcId, id.StringValue)
	insertJsonRpcHeaders(true, config, JsonRpcMethod, method)

	// Set other headers based on the method
	// For MethodToolCall, we set the params in processToolCallRequest
	if method != MethodToolCall {
		// JsonRpcParams
		insertJsonRpcHeaders(true, config, JsonRpcParams, params.Raw)
	}

	return types.ActionContinue
}

func processJsonRpcResponse(context wrapper.HttpContext, config any, id utils.JsonRpcID, result, error gjson.Result, rawBody []byte) types.Action {
	if isPreRequestStage(config) {
		// pre-request removes response headers, which are added by pre-response
		removeJsonRpcHeaders(false)
		return types.ActionContinue
	}

	method := context.GetStringContext("JSONRPC_METHOD", "")
	if !isMethodAllowed(config, method) {
		log.Debugf("[JsonRpcResponse] Method %s is not allowed, skipping processing", method)
		return types.ActionContinue
	}

	// Set common headers, JsonRpcId, JsonRpcMethod
	insertJsonRpcHeaders(false, config, JsonRpcId, id.StringValue)
	insertJsonRpcHeaders(false, config, JsonRpcMethod, method)

	// Set other headers based on the method
	// For MethodToolList & MethodToolCall, we set the params in processToolCallResponse and processToolListResponse
	if method != MethodToolList && method != MethodToolCall {
		// JsonRpcResult
		insertJsonRpcHeaders(false, config, JsonRpcResult, result.Raw)
		// JsonRpcError
		insertJsonRpcHeaders(false, config, JsonRpcError, error.Raw)
	}

	return types.ActionContinue
}

func processToolListResponse(ctx wrapper.HttpContext, config any, tools gjson.Result, rawBody []byte) types.Action {
	if isPreRequestStage(config) {
		return types.ActionContinue
	}

	if !isMethodAllowed(config, MethodToolList) {
		log.Debugf("[ToolListResponse] Method %s is not allowed, skipping processing", MethodToolList)
		return types.ActionContinue
	}

	// JsonRpcResult
	insertJsonRpcHeaders(false, config, JsonRpcResult, tools.Raw)

	return types.ActionContinue
}

func processToolCallRequest(ctx wrapper.HttpContext, config any, toolName string, toolArgs gjson.Result, rawBody []byte) types.Action {
	if isPreResponseStage(config) {
		return types.ActionContinue
	}

	if !isMethodAllowed(config, MethodToolCall) {
		log.Debugf("[ToolCallRequest] Method %s is not allowed, skipping processing", MethodToolCall)
		return types.ActionContinue
	}

	// McpToolName, McpToolArguments
	insertJsonRpcHeaders(true, config, McpToolName, toolName)
	insertJsonRpcHeaders(true, config, McpToolArguments, toolArgs.Raw)

	return types.ActionContinue
}

func processToolCallResponse(ctx wrapper.HttpContext, config any, isError bool, content gjson.Result, rawBody []byte) types.Action {
	if isPreRequestStage(config) {
		return types.ActionContinue
	}

	if !isMethodAllowed(config, MethodToolCall) {
		log.Debugf("[ToolCallResponse] Method %s is not allowed, skipping processing", MethodToolCall)
		return types.ActionContinue
	}

	// McpToolResponse, McpToolError
	insertJsonRpcHeaders(false, config, McpToolResponse, content.Raw)
	insertJsonRpcHeaders(false, config, McpToolError, strconv.FormatBool(isError))

	return types.ActionContinue
}
