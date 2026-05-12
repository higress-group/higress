// Copyright (c) 2022 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/utils"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

const (
	// Context keys for MCP proxy state management
	CtxMcpProxyInitialized = "mcp_proxy_initialized"
	CtxMcpProxySessionID   = "mcp_proxy_session_id"
	CtxMcpProxyToolName    = "mcp_proxy_tool_name"
	CtxMcpProxyToolArgs    = "mcp_proxy_tool_args"
	CtxMcpProxyOperation   = "mcp_proxy_operation"
)

// ProxyAuthInfo holds authentication information for proxy tool calls
type ProxyAuthInfo struct {
	SecuritySchemeID      string          // RequestTemplate.Security.ID for gateway-to-backend auth
	PassthroughCredential string          // Credential extracted from client request (if passthrough enabled)
	Server                *McpProxyServer // Server instance for accessing security schemes
}

// McpProxyOperation represents the current operation type
type McpProxyOperation string

const (
	OpToolsList McpProxyOperation = "tools/list"
	OpToolsCall McpProxyOperation = "tools/call"
)

// McpProtocolHandler handles MCP protocol initialization and communication
type McpProtocolHandler struct {
	backendURL string
	timeout    int
	sessionID  string
}

// NewMcpProtocolHandler creates a new MCP protocol handler
func NewMcpProtocolHandler(backendURL string, timeout int) *McpProtocolHandler {
	return &McpProtocolHandler{
		backendURL: backendURL,
		timeout:    timeout,
	}
}

// parseSSEResponse parses Server-Sent Events format and extracts data field content
func parseSSEResponse(sseData []byte) ([]byte, error) {
	scanner := bufio.NewScanner(bytes.NewReader(sseData))
	// Set max token size to 32MB to handle large messages
	maxTokenSize := 32 * 1024 * 1024 // 32MB
	scanner.Buffer(make([]byte, 0, 64*1024), maxTokenSize)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}

		// Look for data field
		if strings.HasPrefix(line, "data: ") {
			dataContent := strings.TrimPrefix(line, "data: ")
			return []byte(dataContent), nil
		}
	}

	if err := scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return nil, fmt.Errorf("SSE response line exceeds maximum token size (32MB): %w", err)
		}
		return nil, fmt.Errorf("error reading SSE data: %v", err)
	}

	return nil, fmt.Errorf("no data field found in SSE response")
}

// Initialize performs the MCP protocol initialization sequence asynchronously
func (h *McpProtocolHandler) Initialize(ctx wrapper.HttpContext, authInfo *ProxyAuthInfo) error {
	log.Infof("Starting MCP protocol initialization for %s", h.backendURL)

	// Check if already initialized for this context
	if initialized := ctx.GetContext(CtxMcpProxyInitialized); initialized != nil {
		if sessionID := ctx.GetContext(CtxMcpProxySessionID); sessionID != nil {
			h.sessionID = sessionID.(string)
			log.Debugf("MCP proxy already initialized with session ID: %s", h.sessionID)
			return nil
		}
	}

	// Step 1: Send initialize request
	initRequest := h.createInitializeRequest()
	requestBody, err := json.Marshal(initRequest)
	if err != nil {
		return fmt.Errorf("failed to marshal initialize request: %v", err)
	}

	// Send initialize request to backend asynchronously
	err = h.sendMcpRequest(ctx, requestBody, authInfo, func(statusCode int, responseHeaders [][2]string, responseBody []byte) {
		// Don't resume here - either OnMCPResponseError will send response directly,
		// or sendInitializedNotification will continue the async flow
		if statusCode != 200 {
			log.Errorf("Initialize request failed with status %d: %s", statusCode, string(responseBody))
			utils.OnMCPResponseError(ctx, fmt.Errorf("backend initialization failed"), utils.ErrInternalError, "mcp-proxy:initialize:backend_error")
			return
		}

		// Determine response content type and parse accordingly
		var jsonResponseBody []byte
		var contentType string

		// Find content-type header
		for _, header := range responseHeaders {
			if strings.ToLower(header[0]) == "content-type" {
				contentType = strings.ToLower(header[1])
				break
			}
		}

		// Parse response based on content type
		if strings.Contains(contentType, "text/event-stream") {
			// Handle SSE format
			log.Debugf("Processing SSE response for initialize request")
			parsedJSON, err := parseSSEResponse(responseBody)
			if err != nil {
				log.Errorf("Failed to parse SSE response: %v", err)
				utils.OnMCPResponseError(ctx, err, utils.ErrInternalError, "mcp-proxy:initialize:sse_parse_error")
				return
			}
			jsonResponseBody = parsedJSON
		} else {
			// Handle JSON format (default)
			log.Debugf("Processing JSON response for initialize request")
			jsonResponseBody = responseBody
		}

		// Parse initialize response
		var response map[string]interface{}
		if err := json.Unmarshal(jsonResponseBody, &response); err != nil {
			log.Errorf("Failed to parse initialize response: %v", err)
			utils.OnMCPResponseError(ctx, err, utils.ErrInternalError, "mcp-proxy:initialize:parse_error")
			return
		}

		// Check for protocol version compatibility
		if errorObj, exists := response["error"]; exists {
			log.Errorf("Backend initialization error: %v", errorObj)

			// Check if it's a version compatibility error
			if errorMap, ok := errorObj.(map[string]interface{}); ok {
				if code, codeOk := errorMap["code"]; codeOk && code == -32602 {
					// Protocol version not supported
					utils.OnMCPResponseError(ctx, fmt.Errorf("protocol version not supported by backend"), utils.ErrInvalidParams, "mcp-proxy:initialize:version_incompatible")
					return
				}
			}

			utils.OnMCPResponseError(ctx, fmt.Errorf("backend initialization failed"), utils.ErrInternalError, "mcp-proxy:initialize:backend_error")
			return
		}

		// Extract session ID from response headers if present
		for _, header := range responseHeaders {
			if header[0] == "Mcp-Session-Id" {
				h.sessionID = header[1]
				ctx.SetContext(CtxMcpProxySessionID, h.sessionID)
				log.Infof("Received MCP session ID: %s", h.sessionID)
				break
			}
		}

		// Step 2: Send notifications/initialized
		h.sendInitializedNotification(ctx, authInfo)
	})

	return err
}

// ForwardToolsList forwards tools/list request to backend MCP server
func (h *McpProtocolHandler) ForwardToolsList(ctx wrapper.HttpContext, cursor *string, authInfo *ProxyAuthInfo) error {
	log.Debugf("Forwarding tools/list request to %s", h.backendURL)

	// Store the cursor for later execution
	ctx.SetContext(CtxMcpProxyOperation, OpToolsList)
	if cursor != nil {
		ctx.SetContext("mcp_proxy_cursor", *cursor)
	}
	if authInfo != nil {
		ctx.SetContext("mcp_proxy_auth_info", authInfo)
	}

	// Check if MCP is already initialized
	if initialized := ctx.GetContext(CtxMcpProxyInitialized); initialized != nil {
		// Already initialized, execute directly
		return h.executeToolsList(ctx)
	}

	// Need to initialize first, which will execute tools/list in its callback
	return h.Initialize(ctx, authInfo)
}

// executeToolsList executes the actual tools/list request
func (h *McpProtocolHandler) executeToolsList(ctx wrapper.HttpContext) error {
	var cursor *string
	if cursorVal := ctx.GetContext("mcp_proxy_cursor"); cursorVal != nil {
		cursorStr := cursorVal.(string)
		cursor = &cursorStr
	}

	listRequest := h.createToolsListRequest(cursor)
	requestBody, err := json.Marshal(listRequest)
	if err != nil {
		return fmt.Errorf("failed to marshal tools/list request: %v", err)
	}

	headers := [][2]string{
		{"Content-Type", "application/json"},
		{"Accept", "application/json,text/event-stream"},
	}

	// Add session ID if we have one
	if h.sessionID != "" {
		headers = append(headers, [2]string{"Mcp-Session-Id", h.sessionID})
	}

	// Start with the original backend URL
	finalURL := h.backendURL

	// Apply authentication if auth info was provided
	if authInfoCtx := ctx.GetContext("mcp_proxy_auth_info"); authInfoCtx != nil {
		if authInfo, ok := authInfoCtx.(*ProxyAuthInfo); ok && authInfo.SecuritySchemeID != "" {
			// Apply authentication using shared utilities
			modifiedURL, err := h.applyProxyAuthentication(authInfo.Server, authInfo.SecuritySchemeID, authInfo.PassthroughCredential, &headers)
			if err != nil {
				log.Errorf("Failed to apply authentication for tools/list request: %v", err)
			} else {
				// Use the modified URL if authentication was applied successfully
				finalURL = modifiedURL
				log.Debugf("Using modified URL for tools/list request: %s", finalURL)
			}
		}
	}

	// Use RouteCall for the final tools/list request with potentially modified URL
	return ctx.RouteCall("POST", finalURL, headers, requestBody, func(statusCode int, responseHeaders [][2]string, responseBody []byte) {
		if statusCode != 200 {
			log.Errorf("Tools/list request failed with status %d: %s", statusCode, string(responseBody))
			utils.OnMCPResponseError(ctx, fmt.Errorf("backend tools/list failed"), utils.ErrInternalError, "mcp-proxy:tools/list:backend_error")
			return
		}

		// Determine response content type and parse accordingly
		var jsonResponseBody []byte
		var contentType string

		// Find content-type header
		for _, header := range responseHeaders {
			if strings.ToLower(header[0]) == "content-type" {
				contentType = strings.ToLower(header[1])
				break
			}
		}

		// Parse response based on content type
		if strings.Contains(contentType, "text/event-stream") {
			// Handle SSE format
			log.Debugf("Processing SSE response for tools/list request")
			parsedJSON, err := parseSSEResponse(responseBody)
			if err != nil {
				log.Errorf("Failed to parse SSE response: %v", err)
				utils.OnMCPResponseError(ctx, err, utils.ErrInternalError, "mcp-proxy:tools/list:sse_parse_error")
				return
			}
			jsonResponseBody = parsedJSON
		} else {
			// Handle JSON format (default)
			log.Debugf("Processing JSON response for tools/list request")
			jsonResponseBody = responseBody
		}

		// Parse response and forward to client
		var response map[string]interface{}
		if err := json.Unmarshal(jsonResponseBody, &response); err != nil {
			log.Errorf("Failed to parse tools/list response: %v", err)
			utils.OnMCPResponseError(ctx, err, utils.ErrInternalError, "mcp-proxy:tools/list:parse_error")
			return
		}

		// Forward the tools/list result with allowTools filtering
		if result, hasResult := response["result"]; hasResult {
			if resultMap, ok := result.(map[string]interface{}); ok {
				// Apply allowTools filtering if needed
				filteredResult := h.applyAllowToolsFilter(ctx, resultMap)
				utils.OnMCPResponseSuccess(ctx, filteredResult, "mcp-proxy:tools/list:success")
			} else {
				utils.OnMCPResponseError(ctx, fmt.Errorf("invalid tools/list result type"), utils.ErrInternalError, "mcp-proxy:tools/list:invalid_type")
			}
		} else {
			utils.OnMCPResponseError(ctx, fmt.Errorf("invalid tools/list response"), utils.ErrInternalError, "mcp-proxy:tools/list:invalid_response")
		}
	})
}

// ForwardToolsCall forwards tools/call request to backend MCP server
func (h *McpProtocolHandler) ForwardToolsCall(ctx wrapper.HttpContext, toolName string, arguments map[string]interface{}, authInfo *ProxyAuthInfo) error {
	log.Debugf("Forwarding tools/call request for tool %s to %s", toolName, h.backendURL)

	// Store the tool call parameters for later execution
	ctx.SetContext(CtxMcpProxyOperation, OpToolsCall)
	ctx.SetContext(CtxMcpProxyToolName, toolName)
	ctx.SetContext(CtxMcpProxyToolArgs, arguments)
	if authInfo != nil {
		ctx.SetContext("mcp_proxy_auth_info", authInfo)
	}

	// Check if MCP is already initialized
	if initialized := ctx.GetContext(CtxMcpProxyInitialized); initialized != nil {
		// Already initialized, execute directly
		return h.executeToolsCall(ctx)
	}

	// Need to initialize first, which will execute tools/call in its callback
	return h.Initialize(ctx, authInfo)
}

// executeToolsCall executes the actual tools/call request
func (h *McpProtocolHandler) executeToolsCall(ctx wrapper.HttpContext) error {
	toolName := ctx.GetContext(CtxMcpProxyToolName).(string)
	arguments := ctx.GetContext(CtxMcpProxyToolArgs).(map[string]interface{})

	callRequest := h.createToolsCallRequest(toolName, arguments)
	requestBody, err := json.Marshal(callRequest)
	if err != nil {
		return fmt.Errorf("failed to marshal tools/call request: %v", err)
	}

	headers := [][2]string{
		{"Content-Type", "application/json"},
		{"Accept", "application/json,text/event-stream"},
	}

	// Add session ID if we have one
	if h.sessionID != "" {
		headers = append(headers, [2]string{"Mcp-Session-Id", h.sessionID})
	}

	// Start with the original backend URL
	finalURL := h.backendURL

	// Apply authentication if auth info was provided
	if authInfoCtx := ctx.GetContext("mcp_proxy_auth_info"); authInfoCtx != nil {
		if authInfo, ok := authInfoCtx.(*ProxyAuthInfo); ok && authInfo.SecuritySchemeID != "" {
			// Apply authentication using shared utilities
			modifiedURL, err := h.applyProxyAuthentication(authInfo.Server, authInfo.SecuritySchemeID, authInfo.PassthroughCredential, &headers)
			if err != nil {
				log.Errorf("Failed to apply authentication for proxy tool call: %v", err)
			} else {
				// Use the modified URL if authentication was applied successfully
				finalURL = modifiedURL
				log.Debugf("Using modified URL for tools/call request: %s", finalURL)
			}
		}
	}

	// Use RouteCall for the final tools/call request with potentially modified URL
	return ctx.RouteCall("POST", finalURL, headers, requestBody, func(statusCode int, responseHeaders [][2]string, responseBody []byte) {
		if statusCode != 200 {
			log.Errorf("Tools/call request failed with status %d: %s", statusCode, string(responseBody))
			utils.OnMCPResponseError(ctx, fmt.Errorf("backend tools/call failed"), utils.ErrInternalError, "mcp-proxy:tools/call:backend_error")
			return
		}

		// Determine response content type and parse accordingly
		var jsonResponseBody []byte
		var contentType string

		// Find content-type header
		for _, header := range responseHeaders {
			if strings.ToLower(header[0]) == "content-type" {
				contentType = strings.ToLower(header[1])
				break
			}
		}

		// Parse response based on content type
		if strings.Contains(contentType, "text/event-stream") {
			// Handle SSE format
			log.Debugf("Processing SSE response for tools/call request")
			parsedJSON, err := parseSSEResponse(responseBody)
			if err != nil {
				log.Errorf("Failed to parse SSE response: %v", err)
				utils.OnMCPResponseError(ctx, err, utils.ErrInternalError, "mcp-proxy:tools/call:sse_parse_error")
				return
			}
			jsonResponseBody = parsedJSON
		} else {
			// Handle JSON format (default)
			log.Debugf("Processing JSON response for tools/call request")
			jsonResponseBody = responseBody
		}

		// Parse response and check for backend errors (single unmarshal)
		parsedResponse, isError, errorType := ParseBackendResponse(jsonResponseBody)
		if parsedResponse == nil {
			log.Errorf("Failed to parse tools/call response")
			utils.OnMCPResponseError(ctx, fmt.Errorf("invalid JSON response"), utils.ErrInternalError, "mcp-proxy:tools/call:parse_error")
			return
		}

		// Log backend errors for observability
		if isError {
			log.Warnf("Backend reported %s for %s", errorType, toolName)
		}

		// Forward the tools/call result (pass through both success and error responses)
		if result, hasResult := parsedResponse["result"]; hasResult {
			if resultMap, ok := result.(map[string]interface{}); ok {
				utils.OnMCPResponseSuccess(ctx, resultMap, "mcp-proxy:tools/call:success")
			} else {
				utils.OnMCPResponseError(ctx, fmt.Errorf("invalid tools/call result type"), utils.ErrInternalError, "mcp-proxy:tools/call:invalid_type")
			}
		} else if errorField, hasError := parsedResponse["error"]; hasError {
			// Pass through JSON-RPC error as MCP error
			if errorMap, ok := errorField.(map[string]interface{}); ok {
				errorMsg := "Backend error"
				if msg, hasMsg := errorMap["message"]; hasMsg {
					errorMsg = fmt.Sprintf("%v", msg)
				}
				utils.OnMCPResponseError(ctx, fmt.Errorf("%s", errorMsg), utils.ErrInternalError, "mcp-proxy:tools/call:backend_error")
			} else {
				utils.OnMCPResponseError(ctx, fmt.Errorf("backend error"), utils.ErrInternalError, "mcp-proxy:tools/call:backend_error")
			}
		} else {
			utils.OnMCPResponseError(ctx, fmt.Errorf("invalid tools/call response"), utils.ErrInternalError, "mcp-proxy:tools/call:invalid_response")
		}
	})
}

// sendMcpRequest sends an MCP request to the backend server using POST method
func (h *McpProtocolHandler) sendMcpRequest(ctx wrapper.HttpContext, body []byte, authInfo *ProxyAuthInfo, callback func(int, [][2]string, []byte)) error {
	// Copy headers from current request
	headers := copyHeadersForStreamableHTTP(ctx)

	// Override/ensure required headers for MCP request
	ensureHeader(&headers, "Content-Type", "application/json")
	ensureHeader(&headers, "Accept", "application/json,text/event-stream")

	// Add session ID if we have one
	if h.sessionID != "" {
		ensureHeader(&headers, "Mcp-Session-Id", h.sessionID)
	}

	// Start with the original backend URL
	finalURL := h.backendURL

	// Apply authentication if auth info was provided
	if authInfo != nil && authInfo.SecuritySchemeID != "" {
		modifiedURL, err := h.applyProxyAuthentication(authInfo.Server, authInfo.SecuritySchemeID, authInfo.PassthroughCredential, &headers)
		if err != nil {
			log.Errorf("Failed to apply authentication for MCP request: %v", err)
		} else {
			// Use the modified URL if authentication was applied successfully
			finalURL = modifiedURL
			log.Debugf("Using modified URL for MCP request: %s", finalURL)
		}
	}

	// Determine timeout
	timeout := uint32(h.timeout)
	if timeout == 0 {
		timeout = 5000 // Default 5 seconds
	}

	// Create HTTP client using RouteCluster
	client := wrapper.NewClusterClient(wrapper.RouteCluster{})

	// Convert callback to the expected format
	wrappedCallback := func(statusCode int, responseHeaders http.Header, responseBody []byte) {
		// Convert http.Header to [][2]string format
		headerSlice := make([][2]string, 0, len(responseHeaders))
		for key, values := range responseHeaders {
			if len(values) > 0 {
				headerSlice = append(headerSlice, [2]string{key, values[0]})
			}
		}
		callback(statusCode, headerSlice, responseBody)
	}

	// All MCP requests use POST method with potentially modified URL
	return client.Post(finalURL, headers, body, wrappedCallback, timeout)
}

// createInitializeRequest creates an MCP initialize request
func (h *McpProtocolHandler) createInitializeRequest() map[string]interface{} {
	return map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": "2025-03-26",
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]interface{}{
				"name":    "Higress-mcp-proxy",
				"version": "1.0.0",
			},
		},
	}
}

// sendInitializedNotification sends the notifications/initialized message
func (h *McpProtocolHandler) sendInitializedNotification(ctx wrapper.HttpContext, authInfo *ProxyAuthInfo) {
	notification := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
	}

	requestBody, err := json.Marshal(notification)
	if err != nil {
		log.Errorf("Failed to marshal initialized notification: %v", err)
		utils.OnMCPResponseError(ctx, err, utils.ErrInternalError, "mcp-proxy:notifications/initialized:marshal_error")
		return
	}

	// Send the notification (no response expected)
	err = h.sendMcpRequest(ctx, requestBody, authInfo, func(statusCode int, responseHeaders [][2]string, responseBody []byte) {
		// Always resume at the end, regardless of success or failure
		defer proxywasm.ResumeHttpRequest()

		if statusCode >= 300 {
			log.Warnf("Initialized notification failed with status %d: %s", statusCode, string(responseBody))
			// Even if notification fails, we can still proceed with the operation
			// The backend might still be functional for actual tool calls
		} else {
			log.Debugf("MCP initialization completed successfully")
		}

		// Mark initialization as complete
		ctx.SetContext(CtxMcpProxyInitialized, true)

		// Now execute the originally requested operation
		operation := ctx.GetContext(CtxMcpProxyOperation)
		if operation != nil {
			switch operation.(McpProxyOperation) {
			case OpToolsList:
				if err := h.executeToolsList(ctx); err != nil {
					log.Errorf("Failed to execute tools/list: %v", err)
					utils.OnMCPResponseError(ctx, err, utils.ErrInternalError, "mcp-proxy:tools/list:execution_error")
				}
			case OpToolsCall:
				if err := h.executeToolsCall(ctx); err != nil {
					log.Errorf("Failed to execute tools/call: %v", err)
					utils.OnMCPResponseError(ctx, err, utils.ErrInternalError, "mcp-proxy:tools/call:execution_error")
				}
			default:
				log.Warnf("Unknown MCP proxy operation: %v", operation)
				utils.OnMCPResponseError(ctx, fmt.Errorf("unknown operation"), utils.ErrInternalError, "mcp-proxy:unknown_operation")
			}
		} else {
			// No pending operation, just complete the initialization
			log.Debugf("MCP initialization completed, no pending operation")
		}
	})

	if err != nil {
		log.Errorf("Failed to send initialized notification: %v", err)
		utils.OnMCPResponseError(ctx, err, utils.ErrInternalError, "mcp-proxy:notifications/initialized:send_error")
	}
}

// createToolsListRequest creates a tools/list request
func (h *McpProtocolHandler) createToolsListRequest(cursor *string) map[string]interface{} {
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
		"params":  map[string]interface{}{},
	}

	if cursor != nil && *cursor != "" {
		request["params"].(map[string]interface{})["cursor"] = *cursor
	}

	return request
}

// createToolsCallRequest creates a tools/call request
func (h *McpProtocolHandler) createToolsCallRequest(toolName string, arguments map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name":      toolName,
			"arguments": arguments,
		},
	}
}

// ParseBackendResponse parses the response body and checks if it's a backend error
// Returns the parsed response, whether it's an error, and the error type
func ParseBackendResponse(responseBody []byte) (response map[string]interface{}, isError bool, errorType string) {
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return nil, false, ""
	}

	// Check for JSON-RPC 2.0 error format (top-level error field)
	if _, hasError := response["error"]; hasError {
		return response, true, "jsonrpc_error"
	}

	// Check for error in result.isError format
	if result, hasResult := response["result"]; hasResult {
		if resultMap, ok := result.(map[string]interface{}); ok {
			if isErr, hasIsError := resultMap["isError"]; hasIsError && isErr == true {
				return response, true, "result_isError"
			}
		}
	}

	return response, false, ""
}

// IsBackendError checks if the response is a backend error (JSON-RPC 2.0 error or result.isError)
// Returns true if it's an error response, and the error type ("jsonrpc_error" or "result_isError")
func IsBackendError(responseBody []byte) (isError bool, errorType string) {
	_, isError, errorType = ParseBackendResponse(responseBody)
	return isError, errorType
}

// McpSession represents a temporary MCP session
type McpSession struct {
	ID         string
	BackendURL string
	CreatedAt  time.Time
	LastUsed   time.Time
}

// McpSessionManagerImpl manages temporary MCP sessions
type McpSessionManagerImpl struct {
	sessions map[string]*McpSession
}

// NewMcpSessionManagerImpl creates a new session manager
func NewMcpSessionManagerImpl() *McpSessionManagerImpl {
	return &McpSessionManagerImpl{
		sessions: make(map[string]*McpSession),
	}
}

// CreateSession creates a new temporary session
func (m *McpSessionManagerImpl) CreateSession(backendURL string) (string, error) {
	sessionID := fmt.Sprintf("mcp-session-%d", time.Now().UnixNano())
	session := &McpSession{
		ID:         sessionID,
		BackendURL: backendURL,
		CreatedAt:  time.Now(),
		LastUsed:   time.Now(),
	}

	m.sessions[sessionID] = session
	log.Debugf("Created MCP session %s for %s", sessionID, backendURL)

	return sessionID, nil
}

// GetSession retrieves a session by ID
func (m *McpSessionManagerImpl) GetSession(sessionID string) (*McpSession, bool) {
	session, exists := m.sessions[sessionID]
	if exists {
		session.LastUsed = time.Now()
	}
	return session, exists
}

// CleanupSession removes a session
func (m *McpSessionManagerImpl) CleanupSession(sessionID string) {
	if _, exists := m.sessions[sessionID]; exists {
		delete(m.sessions, sessionID)
		log.Debugf("Cleaned up MCP session %s", sessionID)
	}
}

// CleanupExpiredSessions removes sessions older than specified duration
func (m *McpSessionManagerImpl) CleanupExpiredSessions(maxAge time.Duration) {
	now := time.Now()
	for sessionID, session := range m.sessions {
		if now.Sub(session.LastUsed) > maxAge {
			delete(m.sessions, sessionID)
			log.Debugf("Cleaned up expired MCP session %s", sessionID)
		}
	}
}

// CreateMcpProxyMethodHandlers creates JSON-RPC method handlers for MCP proxy operations
func CreateMcpProxyMethodHandlers(server *McpProxyServer, allowTools *map[string]struct{}) utils.MethodHandlers {
	return utils.MethodHandlers{
		"tools/list": func(ctx wrapper.HttpContext, id utils.JsonRpcID, params gjson.Result) error {
			// Check transport type
			if server.GetTransport() == TransportSSE {
				return handleSSEToolsList(ctx, id, params, server, allowTools)
			}

			// StreamableHTTP transport (original logic)
			// Extract cursor parameter if present
			var cursor *string
			if cursorResult := params.Get("cursor"); cursorResult.Exists() {
				cursorStr := cursorResult.String()
				cursor = &cursorStr
			}

			// Extract allowTools from header and compute effective allowTools
			allowToolsHeaderStr, _ := proxywasm.GetHttpRequestHeader("x-envoy-allow-mcp-tools")
			proxywasm.RemoveHttpRequestHeader("x-envoy-allow-mcp-tools")
			// Only consider header as "present" if it has non-empty value
			// Empty string means header is not set or explicitly empty, both treated as "no restriction"
			headerExists := allowToolsHeaderStr != ""
			effectiveAllowTools := computeEffectiveAllowToolsFromHeader(allowTools, allowToolsHeaderStr, headerExists)

			// Store server reference and effective allowTools in context for callback use
			ctx.SetContext("mcp_proxy_server", server)
			ctx.SetContext("mcp_proxy_effective_allow_tools", effectiveAllowTools)

			// This will trigger async initialization if needed
			if err := server.ForwardToolsList(ctx, cursor); err != nil {
				return err
			}

			// Signal that we need to pause and wait for async response
			ctx.SetContext(utils.CtxNeedPause, true)
			return nil
		},
		"tools/call": func(ctx wrapper.HttpContext, id utils.JsonRpcID, params gjson.Result) error {
			// Check transport type
			if server.GetTransport() == TransportSSE {
				return handleSSEToolsCall(ctx, id, params, server, allowTools)
			}

			// StreamableHTTP transport (original logic)
			// Extract tool name and arguments
			toolName := params.Get("name").String()
			if toolName == "" {
				return fmt.Errorf("missing tool name")
			}

			// Compute effective allowTools using helper function
			effectiveAllowTools := computeEffectiveAllowTools(allowTools)

			// Check if tool is allowed
			if effectiveAllowTools != nil {
				if _, allow := (*effectiveAllowTools)[toolName]; !allow {
					utils.OnMCPResponseError(ctx, fmt.Errorf("Tool not allowed: %s", toolName), utils.ErrInvalidParams, fmt.Sprintf("mcp-proxy:%s:tools/call:tool_not_allowed", server.Name))
					return nil
				}
			}

			// Extract arguments (optional)
			arguments := make(map[string]interface{})
			argsResult := params.Get("arguments")
			if argsResult.Exists() {
				if err := json.Unmarshal([]byte(argsResult.Raw), &arguments); err != nil {
					return fmt.Errorf("invalid arguments: %v", err)
				}
			}

			// Set properties for monitoring and debugging (consistent with default handler)
			proxywasm.SetProperty([]string{"mcp_server_name"}, []byte(server.Name))
			proxywasm.SetProperty([]string{"mcp_tool_name"}, []byte(toolName))

			// Create a tool instance and call it
			toolConfig, exists := server.GetToolConfig(toolName)
			if !exists {
				log.Warnf("tool not found: %s, will not use tool specifiy security config", toolName)
			}

			// Debug logging (consistent with default handler)
			log.Debugf("Tool call [%s] on server [%s] with arguments[%s]", toolName, server.Name, argsResult.Raw)

			tool := &McpProxyTool{
				serverName: server.Name,
				name:       toolName,
				toolConfig: toolConfig,
				arguments:  arguments,
			}

			// This will trigger async initialization if needed
			err := tool.Call(ctx, server)
			if err != nil {
				return err
			}

			// Signal that we need to pause and wait for async response
			ctx.SetContext(utils.CtxNeedPause, true)
			return nil
		},
	}
}

// applyAllowToolsFilter applies allowTools filtering to the tools/list response
func (h *McpProtocolHandler) applyAllowToolsFilter(ctx wrapper.HttpContext, resultMap map[string]interface{}) map[string]interface{} {
	// Get pre-computed effective allowTools from context
	var effectiveAllowTools *map[string]struct{}
	if allowToolsCtx := ctx.GetContext("mcp_proxy_effective_allow_tools"); allowToolsCtx != nil {
		if allowToolsPtr, ok := allowToolsCtx.(*map[string]struct{}); ok {
			effectiveAllowTools = allowToolsPtr
		}
	}

	// If no restrictions, return original result
	if effectiveAllowTools == nil {
		return resultMap
	}

	// Apply filtering to tools array
	if tools, hasTools := resultMap["tools"]; hasTools {
		if toolsArray, ok := tools.([]interface{}); ok {
			filteredTools := make([]interface{}, 0)

			for _, tool := range toolsArray {
				if toolMap, ok := tool.(map[string]interface{}); ok {
					if name, hasName := toolMap["name"]; hasName {
						if toolName, ok := name.(string); ok {
							// Check if tool is allowed
							if _, allow := (*effectiveAllowTools)[toolName]; !allow {
								continue
							}
							// Tool is allowed, add to filtered list
							filteredTools = append(filteredTools, tool)
						}
					}
				}
			}

			// Create new result with filtered tools
			filteredResult := make(map[string]interface{})
			for k, v := range resultMap {
				filteredResult[k] = v
			}
			filteredResult["tools"] = filteredTools
			return filteredResult
		}
	}

	// If tools array not found or invalid format, return original
	return resultMap
}

// applyProxyAuthentication applies authentication to the proxy request headers and URL
func (h *McpProtocolHandler) applyProxyAuthentication(server *McpProxyServer, schemeID string, passthroughCredential string, headers *[][2]string) (string, error) {
	// Parse the backend URL to create a proper URL object for the shared function
	parsedURL, err := url.Parse(h.backendURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse backend URL: %v", err)
	}

	// Create authentication context
	authCtx := AuthRequestContext{
		Method:                "POST",
		Headers:               *headers,
		ParsedURL:             parsedURL,
		RequestBody:           []byte{}, // Not used for header/query auth
		PassthroughCredential: passthroughCredential,
	}

	// Create security config for gateway-to-backend authentication
	// The passthrough credential (if any) comes from client-to-gateway authentication
	securityConfig := SecurityRequirement{
		ID:          schemeID,
		Credential:  "",                          // Will use passthrough credential or default credential from scheme
		Passthrough: passthroughCredential != "", // Use passthrough if we have a credential
	}

	// Apply authentication using shared utilities
	err = ApplySecurity(securityConfig, server, &authCtx)
	if err != nil {
		return "", err
	}

	// Update headers with authentication applied
	*headers = authCtx.Headers

	// Reconstruct URL from potentially modified ParsedURL (similar to rest_server.go logic)
	u := authCtx.ParsedURL
	encodedPath := u.EscapedPath()
	var urlStr string
	if u.Scheme != "" && u.Host != "" {
		urlStr = u.Scheme + "://" + u.Host + encodedPath
	} else {
		urlStr = "/" + strings.TrimPrefix(encodedPath, "/")
	}
	if u.RawQuery != "" {
		urlStr += "?" + u.RawQuery
	}
	if u.Fragment != "" {
		urlStr += "#" + u.Fragment
	}

	return urlStr, nil
}

// handleSSEToolsList handles tools/list request for SSE transport
func handleSSEToolsList(ctx wrapper.HttpContext, id utils.JsonRpcID, params gjson.Result, server *McpProxyServer, allowTools *map[string]struct{}) error {
	// Extract allowTools from header and compute effective allowTools
	allowToolsHeaderStr, _ := proxywasm.GetHttpRequestHeader("x-envoy-allow-mcp-tools")
	proxywasm.RemoveHttpRequestHeader("x-envoy-allow-mcp-tools")
	headerExists := allowToolsHeaderStr != ""
	effectiveAllowTools := computeEffectiveAllowToolsFromHeader(allowTools, allowToolsHeaderStr, headerExists)

	// Store server reference, effective allowTools, and JSON-RPC ID in context
	ctx.SetContext("mcp_proxy_server", server)
	ctx.SetContext("mcp_proxy_effective_allow_tools", effectiveAllowTools)

	// Prepare request body for tools/list
	listRequest := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
		"params":  map[string]interface{}{},
	}

	if cursorResult := params.Get("cursor"); cursorResult.Exists() {
		listRequest["params"].(map[string]interface{})["cursor"] = cursorResult.String()
	}

	requestBody, err := json.Marshal(listRequest)
	if err != nil {
		return fmt.Errorf("failed to marshal tools/list request: %v", err)
	}

	// Use common function to handle SSE request
	return handleSSERequest(ctx, id, requestBody, server, server.GetDefaultDownstreamSecurity(), server.GetDefaultUpstreamSecurity())
}

// handleSSEToolsCall handles tools/call request for SSE transport
func handleSSEToolsCall(ctx wrapper.HttpContext, id utils.JsonRpcID, params gjson.Result, server *McpProxyServer, allowTools *map[string]struct{}) error {
	// Extract tool name and arguments
	toolName := params.Get("name").String()
	if toolName == "" {
		return fmt.Errorf("missing tool name")
	}

	// Compute effective allowTools
	effectiveAllowTools := computeEffectiveAllowTools(allowTools)

	// Check if tool is allowed
	if effectiveAllowTools != nil {
		if _, allow := (*effectiveAllowTools)[toolName]; !allow {
			utils.OnMCPResponseError(ctx, fmt.Errorf("Tool not allowed: %s", toolName), utils.ErrInvalidParams, fmt.Sprintf("mcp-proxy:%s:tools/call:tool_not_allowed", server.Name))
			return nil
		}
	}

	// Store server reference in context
	ctx.SetContext("mcp_proxy_server", server)

	// Extract arguments
	arguments := make(map[string]interface{})
	argsResult := params.Get("arguments")
	if argsResult.Exists() {
		if err := json.Unmarshal([]byte(argsResult.Raw), &arguments); err != nil {
			return fmt.Errorf("invalid arguments: %v", err)
		}
	}

	// Set properties for monitoring
	proxywasm.SetProperty([]string{"mcp_server_name"}, []byte(server.Name))
	proxywasm.SetProperty([]string{"mcp_tool_name"}, []byte(toolName))

	log.Debugf("Tool call [%s] on server [%s] with arguments[%s]", toolName, server.Name, argsResult.Raw)

	// Prepare request body for tools/call
	// Use id: 2 because initialize uses id: 1, and we only send one tool request (list or call)
	callRequest := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name":      toolName,
			"arguments": arguments,
		},
	}

	requestBody, err := json.Marshal(callRequest)
	if err != nil {
		return fmt.Errorf("failed to marshal tools/call request: %v", err)
	}

	// Get tool config for tool-level security
	toolConfig, _ := server.GetToolConfig(toolName)

	// Determine downstream and upstream security (tool-level or server default)
	var downstreamSecurity SecurityRequirement
	if toolConfig.Security.ID != "" {
		downstreamSecurity = toolConfig.Security
	} else {
		downstreamSecurity = server.GetDefaultDownstreamSecurity()
	}

	var upstreamSecurity SecurityRequirement
	if toolConfig.RequestTemplate.Security.ID != "" {
		upstreamSecurity = toolConfig.RequestTemplate.Security
	} else {
		upstreamSecurity = server.GetDefaultUpstreamSecurity()
	}

	// Use common function to handle SSE request
	return handleSSERequest(ctx, id, requestBody, server, downstreamSecurity, upstreamSecurity)
}

// handleSSERequest is the common function to handle SSE requests for tools/list and tools/call
func handleSSERequest(ctx wrapper.HttpContext, id utils.JsonRpcID, requestBody []byte, server *McpProxyServer, downstreamSecurity SecurityRequirement, upstreamSecurity SecurityRequirement) error {
	// Store JSON-RPC ID in context
	ctx.SetContext(CtxSSEProxyJsonRpcID, id)

	// Store request body in context for later use
	ctx.SetContext(CtxSSEProxyRequestBody, requestBody)

	// Handle downstream security first (to extract and remove credentials before copying headers)
	passthroughCredential := ""
	if downstreamSecurity.ID != "" {
		clientScheme, schemeOk := server.GetSecurityScheme(downstreamSecurity.ID)
		if schemeOk {
			extractedCred, err := ExtractAndRemoveIncomingCredential(clientScheme)
			if err == nil && extractedCred != "" && downstreamSecurity.Passthrough {
				passthroughCredential = extractedCred
			}
		}
	} else {
		// Fallback: Remove Authorization header if no downstream security is defined
		// This prevents downstream credentials from being mistakenly passed to upstream
		// Unless passthroughAuthHeader is explicitly set to true
		if !server.GetPassthroughAuthHeader() {
			proxywasm.RemoveHttpRequestHeader("Authorization")
		}
	}

	// Prepare authentication info
	var authInfo *ProxyAuthInfo
	if upstreamSecurity.ID != "" {
		authInfo = &ProxyAuthInfo{
			SecuritySchemeID:      upstreamSecurity.ID,
			PassthroughCredential: passthroughCredential,
			Server:                server,
		}
	}

	// Store auth info in context (headers will be copied directly in response phase)
	ctx.SetContext(CtxSSEProxyAuthInfo, authInfo)

	// Convert current request to SSE GET request
	// The request will continue through the filter chain and be routed to backend
	// The response will be handled by onHttpResponseHeaders and onHttpStreamingResponseBody
	err := initiateSSEChannelInRequestPhase(ctx, server, authInfo)
	if err != nil {
		log.Errorf("Failed to convert request to SSE GET: %v", err)
		return err
	}

	// Explicitly set to NOT pause - let the request continue to establish SSE channel
	ctx.SetContext(utils.CtxNeedPause, false)
	return nil
}

// initiateSSEChannelInRequestPhase modifies the current request to be a GET request for establishing SSE channel
func initiateSSEChannelInRequestPhase(ctx wrapper.HttpContext, server *McpProxyServer, authInfo *ProxyAuthInfo) error {
	// Copy original request headers
	getHeaders := copyAndCleanHeadersForSSE(ctx)

	// Apply authentication to headers and URL
	finalURL := server.GetMcpServerURL()
	finalHeaders := getHeaders

	if authInfo != nil && authInfo.SecuritySchemeID != "" {
		modifiedURL, err := applyProxyAuthenticationForSSE(server, authInfo.SecuritySchemeID, authInfo.PassthroughCredential, &finalHeaders, finalURL)
		if err != nil {
			log.Errorf("Failed to apply authentication for SSE GET: %v", err)
		} else {
			finalURL = modifiedURL
		}
	}

	// Parse the target URL
	parsedURL, err := url.Parse(finalURL)
	if err != nil {
		return fmt.Errorf("failed to parse MCP server URL: %v", err)
	}

	// Store initial state
	ctx.SetContext(CtxSSEProxyState, SSEStateWaitingEndpoint)

	log.Infof("Converting request to SSE GET request for: %s", finalURL)

	// Modify the current request to be a GET request
	// Replace :method pseudo-header
	if err := proxywasm.ReplaceHttpRequestHeader(":method", "GET"); err != nil {
		log.Warnf("Failed to replace :method header: %v", err)
	}

	// Replace :path pseudo-header
	path := parsedURL.Path
	if parsedURL.RawQuery != "" {
		path += "?" + parsedURL.RawQuery
	}
	if path == "" {
		path = "/"
	}
	if err := proxywasm.ReplaceHttpRequestHeader(":path", path); err != nil {
		log.Warnf("Failed to replace :path header: %v", err)
	}

	// Replace :authority pseudo-header (host:port or just host)
	authority := parsedURL.Host
	if authority == "" {
		authority = parsedURL.Hostname()
		if parsedURL.Port() != "" {
			authority += ":" + parsedURL.Port()
		}
	}
	if err := proxywasm.ReplaceHttpRequestHeader(":authority", authority); err != nil {
		log.Warnf("Failed to replace :authority header: %v", err)
	}

	// Note: :scheme pseudo-header is managed by Envoy and should not be modified

	// Remove headers that are not appropriate for GET requests
	proxywasm.RemoveHttpRequestHeader("content-type")
	proxywasm.RemoveHttpRequestHeader("content-length")
	proxywasm.RemoveHttpRequestHeader("transfer-encoding")

	// Set Accept header for SSE
	if err := proxywasm.ReplaceHttpRequestHeader("accept", "text/event-stream"); err != nil {
		log.Warnf("Failed to set Accept header: %v", err)
	}

	// Apply any additional headers from authentication
	for _, header := range finalHeaders {
		// Skip pseudo-headers and headers already set
		headerName := strings.ToLower(header[0])
		if strings.HasPrefix(headerName, ":") {
			continue
		}
		if headerName == "accept" || headerName == "content-type" || headerName == "content-length" || headerName == "transfer-encoding" {
			continue
		}
		if err := proxywasm.ReplaceHttpRequestHeader(header[0], header[1]); err != nil {
			log.Warnf("Failed to set header %s: %v", header[0], err)
		}
	}

	log.Debugf("SSE GET request prepared: %s %s (authority: %s)", "GET", path, authority)
	return nil
}

// copyHeadersForStreamableHTTP copies headers from current request for StreamableHTTP requests
// This is used for initialize/notification requests in non-SSE mode
func copyHeadersForStreamableHTTP(ctx wrapper.HttpContext) [][2]string {
	headers := make([][2]string, 0)

	// Headers to skip
	skipHeaders := map[string]bool{
		"content-length":    true, // Will be set by the client
		"transfer-encoding": true, // Will be set by the client
		":path":             true, // Pseudo-header, not needed
		":method":           true, // Pseudo-header, not needed
		":scheme":           true, // Pseudo-header, not needed
		":authority":        true, // Pseudo-header, not needed
	}

	// Get all request headers
	headerMap, err := proxywasm.GetHttpRequestHeaders()
	if err != nil {
		log.Warnf("Failed to get request headers: %v", err)
		// Return minimal headers
		return [][2]string{}
	}

	// Copy headers, skipping unwanted ones
	for _, header := range headerMap {
		headerName := strings.ToLower(header[0])
		if skipHeaders[headerName] {
			continue
		}
		headers = append(headers, header)
	}

	return headers
}

// ensureHeader ensures a header is set to a specific value, replacing if it exists
func ensureHeader(headers *[][2]string, key, value string) {
	keyLower := strings.ToLower(key)
	// Check if header already exists
	for i, h := range *headers {
		if strings.ToLower(h[0]) == keyLower {
			// Replace existing header
			(*headers)[i] = [2]string{key, value}
			return
		}
	}
	// Header doesn't exist, add it
	*headers = append(*headers, [2]string{key, value})
}

// copyAndCleanHeadersForSSE copies original request headers and cleans them for SSE GET request
func copyAndCleanHeadersForSSE(ctx wrapper.HttpContext) [][2]string {
	headers := make([][2]string, 0)

	// Headers to skip for GET request
	skipHeaders := map[string]bool{
		"content-type":      true,
		"content-length":    true,
		"transfer-encoding": true,
		"accept":            true, // Will be set explicitly for SSE
		":path":             true,
		":method":           true,
		":scheme":           true,
		":authority":        true,
	}

	// Get all request headers
	headerMap, err := proxywasm.GetHttpRequestHeaders()
	if err != nil {
		log.Warnf("Failed to get request headers: %v", err)
		// Return minimal headers with Accept
		return [][2]string{{"Accept", "text/event-stream"}}
	}

	// Copy headers, skipping unwanted ones
	for _, header := range headerMap {
		headerName := strings.ToLower(header[0])
		if skipHeaders[headerName] {
			continue
		}
		headers = append(headers, header)
	}

	// Set/override Accept header for SSE
	headers = append(headers, [2]string{"Accept", "text/event-stream"})

	log.Debugf("Prepared %d headers for SSE GET request", len(headers))
	return headers
}
