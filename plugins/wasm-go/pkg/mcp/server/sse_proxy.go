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

	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/utils"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

const (
	// Context keys for SSE proxy state management
	CtxSSEProxyState       = "sse_proxy_state"
	CtxSSEProxyEndpointURL = "sse_proxy_endpoint_url"
	CtxSSEProxyBuffer      = "sse_proxy_buffer"
	CtxSSEProxyAuthInfo    = "sse_proxy_auth_info"
	CtxSSEProxyRequestBody = "sse_proxy_request_body"
	CtxSSEProxyRequestID   = "sse_proxy_request_id"
	CtxSSEProxyFirstChunk  = "sse_proxy_first_chunk"
	CtxSSEProxyJsonRpcID   = "sse_proxy_jsonrpc_id"
	CtxSSEProxyPrepared    = "sse_proxy_prepared"

	// SSE proxy state values
	SSEStateWaitingEndpoint   = "waiting_endpoint"
	SSEStateWaitingInitResp   = "waiting_init_resp"
	SSEStateWaitingNotifyResp = "waiting_notify_resp"
	SSEStateWaitingToolResp   = "waiting_tool_resp"
	SSEStateComplete          = "complete"

	// Buffer size limit: 100MB
	MaxSSEBufferSize = 100 * 1024 * 1024
)

// SSE responses keep the existing streaming injection mechanism. Only the
// response identity and notification decision are shared with other transports.
func injectSSEResponse(ctx wrapper.HttpContext, extras map[string]any) {
	id, ok := ctx.GetContext(CtxSSEProxyJsonRpcID).(utils.JsonRpcID)
	if !ok {
		log.Errorf("JSON-RPC ID not found in context for SSE response")
		return
	}
	if exchange, ok := ctx.GetContext(CtxMcpProxyCancel).(*proxyExchange); ok {
		exchange.mu.Lock()
		if exchange.cancelled && !exchange.closed {
			exchange.mu.Unlock()
			finishSSERequest(ctx)
			return
		}
		if exchange.closed || exchange.responding {
			exchange.mu.Unlock()
			return
		}
		exchange.responding = true
		exchange.mu.Unlock()
	}
	response := map[string]any{"jsonrpc": "2.0", "id": utils.ClientResponseID(ctx, id)}
	for key, value := range extras {
		response[key] = value
	}
	body, err := json.Marshal(response)
	if err != nil {
		log.Errorf("Failed to marshal SSE JSON-RPC response")
		return
	}
	if notification, _ := ctx.GetContext(utils.CtxJsonRpcNotification).(bool); notification {
		body = nil
		status, _ := proxywasm.GetHttpResponseHeader(":status")
		if status != "401" && status != "403" {
			proxywasm.ReplaceHttpResponseHeader(":status", "202")
		}
	}
	ctx.SetContext(utils.CtxJsonRpcResponded, true)
	proxywasm.InjectEncodedDataToFilterChain(body, true)
	finishSSERequest(ctx)
}

func injectSSEResponseSuccess(ctx wrapper.HttpContext, result map[string]any) {
	injectSSEResponse(ctx, map[string]any{"result": adaptProxyResult(ctx, false, result)})
}

func injectSSEResponseError(ctx wrapper.HttpContext, err error, errorCode int) {
	if err == nil {
		err = errors.New("internal error")
	}
	injectSSEResponse(ctx, map[string]any{"error": map[string]any{"code": errorCode, "message": err.Error()}})
}

func finishSSERequest(ctx wrapper.HttpContext) {
	finishProxyRequest(ctx)
	// Keep a stream-local terminal marker so later chunks are discarded rather
	// than exposed as raw SSE after the other request-owned state is cleared.
	ctx.SetContext(CtxSSEProxyState, SSEStateComplete)
}

func handleSSEUpstreamAuthFailure(ctx wrapper.HttpContext, statusCode int, responseHeaders [][2]string) bool {
	if statusCode != http.StatusUnauthorized && statusCode != http.StatusForbidden {
		return false
	}
	proxywasm.ReplaceHttpResponseHeader(":status", fmt.Sprintf("%d", statusCode))
	proxywasm.RemoveHttpResponseHeader("WWW-Authenticate")
	for _, header := range responseHeaders {
		if strings.EqualFold(header[0], "WWW-Authenticate") && !strings.ContainsAny(header[1], "\r\n") {
			proxywasm.AddHttpResponseHeader("WWW-Authenticate", header[1])
		}
	}
	injectSSEResponseError(ctx, errors.New("upstream authorization failed"), utils.ErrInternalError)
	return true
}

// SSEMessage represents a parsed SSE message
type SSEMessage struct {
	Event string
	Data  string
	ID    string
}

// ParseSSEMessage parses SSE format data and returns complete messages
// Returns the parsed message and the remaining unparsed data
func ParseSSEMessage(data []byte) (*SSEMessage, []byte, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	// Set max token size to 32MB to handle large messages
	maxTokenSize := 32 * 1024 * 1024 // 32MB
	scanner.Buffer(make([]byte, 0, 64*1024), maxTokenSize)
	msg := &SSEMessage{}
	lineCount := 0
	lastPos := 0

	for scanner.Scan() {
		line := scanner.Text()
		lineCount++
		lastPos += len(line) + 1 // +1 for newline

		// Empty line indicates end of message
		if strings.TrimSpace(line) == "" {
			if msg.Event != "" || msg.Data != "" || msg.ID != "" {
				// Found a complete message
				return msg, data[lastPos:], nil
			}
			// Empty message, continue
			continue
		}

		// Skip comment lines (lines starting with ':')
		if strings.HasPrefix(line, ":") {
			continue
		}

		// Parse field
		parts := strings.SplitN(line, ":", 2)
		if len(parts) < 2 {
			continue
		}

		field := parts[0]
		value := strings.TrimSpace(parts[1])

		switch field {
		case "event":
			msg.Event = value
		case "data":
			if msg.Data != "" {
				msg.Data += "\n" + value
			} else {
				msg.Data = value
			}
		case "id":
			msg.ID = value
		}
	}

	if err := scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return nil, nil, fmt.Errorf("SSE message line exceeds maximum token size (32MB): %w", err)
		}
		return nil, nil, fmt.Errorf("error scanning SSE data: %v", err)
	}

	// No complete message found, return all data as remaining
	return nil, data, nil
}

// ExtractEndpointURL extracts the endpoint URL from an SSE endpoint message
// It handles two cases:
// 1. endpointData is a full URL (e.g., http://example.com/sse) - return as-is
// 2. endpointData is a path - if baseURL has scheme and host, combine them; otherwise return the path as-is
func ExtractEndpointURL(endpointData string, baseURL string) (string, error) {
	// Case 1: endpointData is a full URL
	if strings.HasPrefix(endpointData, "http://") || strings.HasPrefix(endpointData, "https://") {
		return endpointData, nil
	}

	// endpointData is a path
	parsedBase, err := url.Parse(baseURL)
	if err != nil {
		return "", errors.New("failed to parse base URL")
	}

	// Case 2: baseURL has scheme and host, combine them
	if parsedBase.Scheme != "" && parsedBase.Host != "" {
		// Combine scheme, host, and the new path
		// Ensure endpointData starts with "/"
		if !strings.HasPrefix(endpointData, "/") {
			endpointData = "/" + endpointData
		}
		result := parsedBase.Scheme + "://" + parsedBase.Host + endpointData
		return result, nil
	}

	// Case 3: baseURL is also just a path, return endpointData as-is
	return endpointData, nil
}

func prepareSSEUpstreamRequest(server *McpProxyServer, authInfo *ProxyAuthInfo, headers *[][2]string, targetURL string) (string, error) {
	if authInfo == nil {
		return targetURL, nil
	}
	var authHeaders [][2]string
	if authInfo.SecuritySchemeID != "" {
		modifiedURL, err := applyProxyAuthenticationForSSE(server, authInfo.SecuritySchemeID, authInfo.PassthroughCredential, &authHeaders, targetURL)
		if err != nil {
			return "", fmt.Errorf("failed to apply proxy authentication: %v", err)
		}
		targetURL = modifiedURL
	} else if authInfo.ForwardAuthorization != "" {
		if strings.ContainsAny(authInfo.ForwardAuthorization, "\r\n") {
			return "", errors.New("invalid forwarded authorization header")
		}
		authHeaders = append(authHeaders, [2]string{"Authorization", authInfo.ForwardAuthorization})
	}
	for _, header := range authHeaders {
		if reservedProxyHeader(strings.ToLower(header[0])) {
			return "", errors.New("upstream authentication conflicts with MCP protocol headers")
		}
		ensureHeader(headers, header[0], header[1])
	}
	return targetURL, nil
}

// SSE POSTs retain the original route cluster while the old endpoint URL rules
// choose Authority/path/query. No endpoint-domain DNS client is introduced.
func postSSEUpstream(ctx wrapper.HttpContext, targetURL string, headers [][2]string, body []byte, callback func(int, [][2]string, []byte)) error {
	prepared, ok := ctx.GetContext(CtxSSEProxyPrepared).(*PreparedProxyRequest)
	if !ok {
		return errors.New("missing SSE proxy request")
	}
	target, err := resolveProxyTarget(targetURL)
	if err != nil {
		return err
	}
	target.cluster = prepared.target.cluster
	exchange, _ := ctx.GetContext(CtxMcpProxyCancel).(*proxyExchange)
	if exchange == nil {
		return errors.New("missing SSE proxy exchange")
	}
	exchange.mu.Lock()
	closed, cancelled := exchange.closed, exchange.cancelled
	exchange.mu.Unlock()
	if closed {
		return nil
	}
	if cancelled {
		finishSSERequest(ctx)
		return nil
	}
	completed := false
	return dispatchPreparedMCP(target, prepared.timeout, headers, body, 0, func(response autoHTTPResponse) {
		exchange.mu.Lock()
		closed, cancelled, duplicate := exchange.closed, exchange.cancelled, completed
		completed = true
		exchange.mu.Unlock()
		if closed || duplicate {
			return
		}
		if cancelled {
			finishSSERequest(ctx)
			return
		}
		if response.status == 0 {
			response.status = http.StatusBadGateway
		}
		callback(response.status, response.headers, response.body)
	})
}

// sendSSEInitialize sends the initialize request for SSE protocol
func sendSSEInitialize(ctx wrapper.HttpContext, endpointURL string, authInfo *ProxyAuthInfo, proxyServer *McpProxyServer) error {
	initRequest := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-03-26",
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
				"name":    "Higress-mcp-proxy",
				"title":   "Higress MCP Proxy",
				"version": "1.0.0",
			},
		},
	}

	requestBody, err := json.Marshal(initRequest)
	if err != nil {
		return fmt.Errorf("failed to marshal initialize request: %v", err)
	}

	// Copy headers from current request (now supported in response phase by Envoy)
	finalHeaders := copyHeadersForSSERequest(ctx)

	// Override required headers for SSE initialize
	ensureHeader(&finalHeaders, "Content-Type", "application/json")
	ensureHeader(&finalHeaders, "Accept", "application/json,text/event-stream")

	// Apply the existing credential policy to the endpoint-selected URL.
	finalURL, err := prepareSSEUpstreamRequest(proxyServer, authInfo, &finalHeaders, endpointURL)
	if err != nil {
		return err
	}

	// Note: headers are already copied from the current request (which has server-level headers applied)
	// via copyHeadersForSSERequest, so no need to apply them again

	// Store state for tracking
	ctx.SetContext(CtxSSEProxyState, SSEStateWaitingInitResp)
	ctx.SetContext(CtxSSEProxyRequestID, json.RawMessage(`1`))

	return postSSEUpstream(ctx, finalURL, finalHeaders, requestBody, func(statusCode int, responseHeaders [][2]string, responseBody []byte) {
		if handleSSEUpstreamAuthFailure(ctx, statusCode, responseHeaders) {
			return
		}
		if statusCode != 200 && statusCode != 202 {
			log.Errorf("SSE initialize request failed with status %d", statusCode)
			// At this point, we're in streaming response phase, must use injectSSEResponseError
			injectSSEResponseError(ctx, fmt.Errorf("SSE initialize failed with status %d", statusCode), utils.ErrInternalError)
			return
		}

		log.Debugf("SSE initialize request sent successfully")
		// The response will be received through SSE channel and processed in streaming response handler
		// State has already been set to SSEStateWaitingInitResp before this POST request
		// No need to change state here
	})
}

// sendSSENotification sends the notifications/initialized message for SSE protocol
func sendSSENotification(ctx wrapper.HttpContext, endpointURL string, authInfo *ProxyAuthInfo, proxyServer *McpProxyServer) error {
	notification := map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
	}

	requestBody, err := json.Marshal(notification)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %v", err)
	}

	// Copy headers from current request (now supported in response phase by Envoy)
	finalHeaders := copyHeadersForSSERequest(ctx)

	// Override required headers for SSE notification
	ensureHeader(&finalHeaders, "Content-Type", "application/json")
	ensureHeader(&finalHeaders, "Accept", "application/json,text/event-stream")

	// Apply the existing credential policy to the endpoint-selected URL.
	finalURL, err := prepareSSEUpstreamRequest(proxyServer, authInfo, &finalHeaders, endpointURL)
	if err != nil {
		return err
	}

	// Note: headers are already copied from the current request (which has server-level headers applied)
	// via copyHeadersForSSERequest, so no need to apply them again

	// Store state for tracking
	ctx.SetContext(CtxSSEProxyState, SSEStateWaitingNotifyResp)

	return postSSEUpstream(ctx, finalURL, finalHeaders, requestBody, func(statusCode int, responseHeaders [][2]string, responseBody []byte) {
		if handleSSEUpstreamAuthFailure(ctx, statusCode, responseHeaders) {
			return
		}
		if statusCode != 200 && statusCode != 202 {
			log.Warnf("SSE notification request failed with status %d", statusCode)
			// Even if notification fails, we should try to continue
			// Some servers may not strictly require notification success
		}

		log.Debugf("SSE notification sent successfully")

		// Now we can send the actual tool request
		// Get stored context
		endpointURLRaw := ctx.GetContext(CtxSSEProxyEndpointURL)
		authInfoRaw := ctx.GetContext(CtxSSEProxyAuthInfo)
		proxyServerRaw := ctx.GetContext("mcp_proxy_server")
		requestBodyRaw := ctx.GetContext(CtxSSEProxyRequestBody)

		if endpointURLRaw == nil || proxyServerRaw == nil || requestBodyRaw == nil {
			log.Errorf("Missing context for sending tool request")
			// At this point, we're in streaming response phase, must use injectSSEResponseError
			injectSSEResponseError(ctx, fmt.Errorf("internal error: missing context"), utils.ErrInternalError)
			return
		}

		endpointURL, ok := endpointURLRaw.(string)
		if !ok {
			log.Errorf("Invalid endpoint URL type in context")
			injectSSEResponseError(ctx, fmt.Errorf("internal error: invalid endpoint URL"), utils.ErrInternalError)
			return
		}
		proxyServer, ok := proxyServerRaw.(*McpProxyServer)
		if !ok {
			log.Errorf("Invalid proxy server type in context")
			injectSSEResponseError(ctx, fmt.Errorf("internal error: invalid proxy server"), utils.ErrInternalError)
			return
		}
		requestBody, ok := requestBodyRaw.([]byte)
		if !ok {
			log.Errorf("Invalid request body type in context")
			injectSSEResponseError(ctx, fmt.Errorf("internal error: invalid request body"), utils.ErrInternalError)
			return
		}

		var authInfo *ProxyAuthInfo
		if authInfoRaw != nil {
			authInfo, ok = authInfoRaw.(*ProxyAuthInfo)
			if !ok {
				log.Errorf("Invalid auth info type in context")
				injectSSEResponseError(ctx, fmt.Errorf("internal error: invalid auth info"), utils.ErrInternalError)
				return
			}
		}

		if ctx.GetContext(CtxSSEProxyState) != SSEStateWaitingNotifyResp {
			return
		}
		if err := sendSSEToolRequest(ctx, endpointURL, authInfo, proxyServer, requestBody); err != nil {
			log.Errorf("Failed to send SSE tool request: %v", err)
			injectSSEResponseError(ctx, err, utils.ErrInternalError)
		}
	})
}

// sendSSEToolRequest sends the tools/list or tools/call request for SSE protocol
func sendSSEToolRequest(ctx wrapper.HttpContext, endpointURL string, authInfo *ProxyAuthInfo, proxyServer *McpProxyServer, requestBody []byte) error {
	// Copy headers from current request (now supported in response phase by Envoy)
	finalHeaders := copyHeadersForSSERequest(ctx)

	// Override required headers for SSE tool request
	ensureHeader(&finalHeaders, "Content-Type", "application/json")
	ensureHeader(&finalHeaders, "Accept", "application/json,text/event-stream")

	// Apply the existing credential policy to the endpoint-selected URL.
	finalURL, err := prepareSSEUpstreamRequest(proxyServer, authInfo, &finalHeaders, endpointURL)
	if err != nil {
		return err
	}

	// Note: headers are already copied from the current request (which has server-level headers applied)
	// via copyHeadersForSSERequest, so no need to apply them again

	// Store state for tracking
	ctx.SetContext(CtxSSEProxyState, SSEStateWaitingToolResp)
	ctx.SetContext(CtxSSEProxyRequestID, json.RawMessage(gjson.GetBytes(requestBody, "id").Raw))

	return postSSEUpstream(ctx, finalURL, finalHeaders, requestBody, func(statusCode int, responseHeaders [][2]string, responseBody []byte) {
		if handleSSEUpstreamAuthFailure(ctx, statusCode, responseHeaders) {
			return
		}
		if statusCode != 200 && statusCode != 202 {
			log.Errorf("SSE tool request failed with status %d", statusCode)
			// At this point, we're in streaming response phase, must use injectSSEResponseError
			injectSSEResponseError(ctx, fmt.Errorf("SSE tool request failed with status %d", statusCode), utils.ErrInternalError)
			return
		}

		log.Debugf("SSE tool request sent successfully")
		// The response will be received through SSE channel and processed in streaming response handler
	})
}

// copyHeadersForSSERequest copies headers from current request for SSE RouteCluster calls
// This leverages Envoy's new capability to access request headers in response phase
func copyHeadersForSSERequest(ctx wrapper.HttpContext) [][2]string {
	if captured, ok := ctx.GetContext(CtxMcpProxyHeaders).([][2]string); ok {
		return inheritedProxyHeaders(captured, false)
	}
	return captureForwardHeaders(ctx, false)
}

// applyProxyAuthenticationForSSE applies authentication for SSE proxy requests
func applyProxyAuthenticationForSSE(server *McpProxyServer, schemeID string, passthroughCredential string, headers *[][2]string, targetURL string) (string, error) {
	// Parse the target URL
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return "", errors.New("failed to parse target URL")
	}

	// Create authentication context
	authCtx := AuthRequestContext{
		Method:                "POST",
		Headers:               nil,
		ParsedURL:             parsedURL,
		RequestBody:           []byte{},
		PassthroughCredential: passthroughCredential,
	}

	// Create security config
	securityConfig := SecurityRequirement{
		ID:          schemeID,
		Credential:  "",
		Passthrough: passthroughCredential != "",
	}

	// Apply authentication
	err = ApplySecurity(securityConfig, server, &authCtx)
	if err != nil {
		return "", err
	}

	// Update headers
	for _, header := range authCtx.Headers {
		ensureHeader(headers, header[0], header[1])
	}

	// Reconstruct URL
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

// handleSSEStreamingResponse handles the streaming SSE response
func handleSSEStreamingResponse(ctx wrapper.HttpContext, config McpServerConfig, data []byte, endOfStream bool) []byte {
	if ctx.GetContext(CtxSSEProxyState) == SSEStateComplete {
		return []byte{}
	}
	if proxyRequestCancelled(ctx) {
		finishSSERequest(ctx)
		return []byte{}
	}
	// Get the first chunk flag
	isFirstChunk := ctx.GetBoolContext(CtxSSEProxyFirstChunk, true)
	if isFirstChunk {
		ctx.SetContext(CtxSSEProxyFirstChunk, false)
	}
	log.Debugf("Handling SSE response chunk (%d bytes)", len(data))
	// On first chunk, validate content-type and modify headers
	if isFirstChunk {
		// Validate that backend returned text/event-stream
		contentType, err := proxywasm.GetHttpResponseHeader("content-type")
		if err != nil || !strings.Contains(strings.ToLower(contentType), "text/event-stream") {
			log.Errorf("Backend did not return text/event-stream content-type, got: %s", contentType)
			// Return JSON-RPC error
			injectSSEResponseError(ctx, fmt.Errorf("invalid content-type, expected text/event-stream but got: %s", contentType), utils.ErrInternalError)
			return []byte{}
		}

		// Remove content-length and modify content-type
		proxywasm.RemoveHttpResponseHeader("content-length")
		proxywasm.ReplaceHttpResponseHeader("content-type", "application/json; charset=utf-8")
		proxywasm.ReplaceHttpResponseHeader(":status", "200")
	}

	// Get or initialize buffer
	var buffer []byte
	if bufferRaw := ctx.GetContext(CtxSSEProxyBuffer); bufferRaw != nil {
		var ok bool
		buffer, ok = bufferRaw.([]byte)
		if !ok {
			log.Errorf("Invalid buffer type in context")
			injectSSEResponseError(ctx, fmt.Errorf("internal error: invalid buffer"), utils.ErrInternalError)
			return []byte{}
		}
	}

	// Append new data to buffer
	buffer = append(buffer, data...)

	// Check buffer size limit
	if len(buffer) > MaxSSEBufferSize {
		log.Errorf("SSE buffer exceeded maximum size of %d bytes", MaxSSEBufferSize)
		injectSSEResponseError(ctx, errors.New("response too large"), utils.ErrInternalError)
		return []byte{}
	}

	// Store buffer back
	ctx.SetContext(CtxSSEProxyBuffer, buffer)

	// Get current state
	state := ctx.GetContext(CtxSSEProxyState)
	if state == nil {
		state = SSEStateWaitingEndpoint
		ctx.SetContext(CtxSSEProxyState, state)
	}

	stateStr, ok := state.(string)
	if !ok {
		log.Errorf("Invalid state type in context")
		injectSSEResponseError(ctx, fmt.Errorf("internal error: invalid state"), utils.ErrInternalError)
		return []byte{}
	}

	log.Debugf("SSE proxy state: %s, buffered bytes: %d", stateStr, len(buffer))

	// Process based on state
	switch stateStr {
	case SSEStateWaitingEndpoint:
		return handleWaitingEndpoint(ctx, config, &buffer)

	case SSEStateWaitingInitResp:
		return handleWaitingInitResp(ctx, config, &buffer)

	case SSEStateWaitingNotifyResp:
		return handleWaitingNotifyResp(ctx, config, &buffer)

	case SSEStateWaitingToolResp:
		return handleWaitingToolResp(ctx, config, &buffer)

	default:
		log.Warnf("Unknown SSE proxy state: %v", state)
		return []byte{}
	}
}

// handleWaitingEndpoint processes SSE messages waiting for endpoint message
func handleWaitingEndpoint(ctx wrapper.HttpContext, config McpServerConfig, buffer *[]byte) []byte {
	for {
		msg, remaining, err := ParseSSEMessage(*buffer)
		if err != nil {
			log.Errorf("Failed to parse SSE message: %v", err)
			injectSSEResponseError(ctx, err, utils.ErrInternalError)
			return []byte{}
		}

		if msg == nil {
			// No complete message yet
			*buffer = remaining
			return []byte{}
		}

		// Update buffer
		*buffer = remaining
		ctx.SetContext(CtxSSEProxyBuffer, *buffer)

		// Check for endpoint message
		if msg.Event == "endpoint" {
			// Extract and store endpoint URL
			proxyServerRaw := ctx.GetContext("mcp_proxy_server")
			if proxyServerRaw == nil {
				log.Errorf("mcp_proxy_server not found in context")
				injectSSEResponseError(ctx, errors.New("internal error"), utils.ErrInternalError)
				return []byte{}
			}
			proxyServer, ok := proxyServerRaw.(*McpProxyServer)
			if !ok {
				log.Errorf("Invalid proxy server type in context")
				injectSSEResponseError(ctx, fmt.Errorf("internal error: invalid proxy server"), utils.ErrInternalError)
				return []byte{}
			}

			endpointURL, err := ExtractEndpointURL(msg.Data, proxyServer.GetMcpServerURL())
			if err != nil {
				log.Errorf("Failed to extract endpoint URL: %v", err)
				injectSSEResponseError(ctx, err, utils.ErrInternalError)
				return []byte{}
			}

			log.Debugf("Received request-scoped SSE endpoint")
			ctx.SetContext(CtxSSEProxyEndpointURL, endpointURL)

			// Get stored auth info
			authInfoRaw := ctx.GetContext(CtxSSEProxyAuthInfo)

			var authInfo *ProxyAuthInfo
			if authInfoRaw != nil {
				var ok bool
				authInfo, ok = authInfoRaw.(*ProxyAuthInfo)
				if !ok {
					log.Errorf("Invalid auth info type in context")
					injectSSEResponseError(ctx, fmt.Errorf("internal error: invalid auth info"), utils.ErrInternalError)
					return []byte{}
				}
			}

			// Send initialize request
			if err := sendSSEInitialize(ctx, endpointURL, authInfo, proxyServer); err != nil {
				log.Errorf("Failed to send SSE initialize: %v", err)
				injectSSEResponseError(ctx, err, utils.ErrInternalError)
				return []byte{}
			}

			// State has been changed to SSEStateWaitingInitResp in sendSSEInitialize
			// Return immediately to allow next chunk to be processed in the new state
			return []byte{}
		}

		// Skip other message types (like ping) while waiting for endpoint
		// Continue to process next message in buffer
		log.Debugf("Skipping SSE message with event '%s' while waiting for endpoint", msg.Event)
		continue
	}
}

// handleWaitingInitResp processes SSE messages waiting for initialize response
func handleWaitingInitResp(ctx wrapper.HttpContext, config McpServerConfig, buffer *[]byte) []byte {
	requestID, _ := ctx.GetContext(CtxSSEProxyRequestID).(json.RawMessage)

	for {
		msg, remaining, err := ParseSSEMessage(*buffer)
		if err != nil {
			log.Errorf("Failed to parse SSE message: %v", err)
			injectSSEResponseError(ctx, err, utils.ErrInternalError)
			return []byte{}
		}

		if msg == nil {
			// No complete message yet
			*buffer = remaining
			return []byte{}
		}

		// Update buffer
		*buffer = remaining
		ctx.SetContext(CtxSSEProxyBuffer, *buffer)

		// Check for message event
		if msg.Event == "message" {
			// Parse JSON-RPC response
			// This explicit Legacy control path historically used Unmarshal,
			// independently of the strict auto probe/handshake envelope decoder.
			var jsonRpcResp map[string]any
			err := decodeBusinessJSON([]byte(msg.Data), &jsonRpcResp)
			if err != nil {
				log.Errorf("Failed to parse JSON-RPC response: %v", err)
				continue
			}

			// Check if this is the initialize response
			respID, _ := json.Marshal(jsonRpcResp["id"])
			if sameRPCID(respID, requestID) {
				// Check for errors
				if _, hasError := jsonRpcResp["error"]; hasError {
					log.Errorf("Backend initialize returned a JSON-RPC error")
					injectSSEResponseError(ctx, fmt.Errorf("backend initialize failed"), utils.ErrInternalError)
					return []byte{}
				}

				log.Debugf("Received initialize response, sending notification")

				// Get endpoint URL and auth info
				endpointURL, ok := ctx.GetContext(CtxSSEProxyEndpointURL).(string)
				if !ok {
					log.Errorf("Endpoint URL not found in context during init response handling")
					injectSSEResponseError(ctx, fmt.Errorf("internal error: missing endpoint URL"), utils.ErrInternalError)
					return []byte{}
				}
				authInfoRaw := ctx.GetContext(CtxSSEProxyAuthInfo)
				proxyServerRaw := ctx.GetContext("mcp_proxy_server")

				var authInfo *ProxyAuthInfo
				if authInfoRaw != nil {
					authInfo, ok = authInfoRaw.(*ProxyAuthInfo)
					if !ok {
						log.Errorf("Invalid auth info type in context")
						injectSSEResponseError(ctx, fmt.Errorf("internal error: invalid auth info"), utils.ErrInternalError)
						return []byte{}
					}
				}

				proxyServer, ok := proxyServerRaw.(*McpProxyServer)
				if !ok {
					log.Errorf("Invalid proxy server type in context")
					injectSSEResponseError(ctx, fmt.Errorf("internal error: invalid proxy server"), utils.ErrInternalError)
					return []byte{}
				}

				// Send notification
				// The notification callback will send the tool request after notification succeeds
				if err := sendSSENotification(ctx, endpointURL, authInfo, proxyServer); err != nil {
					log.Errorf("Failed to send SSE notification: %v", err)
					injectSSEResponseError(ctx, err, utils.ErrInternalError)
					return []byte{}
				}

				// State has been changed to SSEStateWaitingNotifyResp in sendSSENotification
				// The tool request will be sent in the notification callback
				// Return immediately to allow next chunk to be processed in the new state
				return []byte{}
			}
		}

		// Skip other message types (like ping) while waiting for init response
		// Continue to process next message in buffer
		log.Debugf("Skipping SSE message with event '%s' while waiting for init response", msg.Event)
		continue
	}
}

// handleWaitingNotifyResp processes SSE messages waiting for notification response
func handleWaitingNotifyResp(ctx wrapper.HttpContext, config McpServerConfig, buffer *[]byte) []byte {
	// For notifications, we don't expect a response in SSE channel
	// Just continue to send tool request
	// This state should be very brief
	return []byte{}
}

// handleWaitingToolResp processes SSE messages waiting for tool response
func handleWaitingToolResp(ctx wrapper.HttpContext, config McpServerConfig, buffer *[]byte) []byte {
	requestID, ok := ctx.GetContext(CtxSSEProxyRequestID).(json.RawMessage)
	if !ok {
		injectSSEResponseError(ctx, errors.New("internal error"), utils.ErrInternalError)
		return []byte{}
	}
	for {
		msg, remaining, err := ParseSSEMessage(*buffer)
		if err != nil {
			injectSSEResponseError(ctx, err, utils.ErrInternalError)
			return []byte{}
		}
		*buffer = remaining
		ctx.SetContext(CtxSSEProxyBuffer, *buffer)
		if msg == nil {
			return []byte{}
		}
		if msg.Event != "message" {
			continue
		}
		response, err := decodeBusinessJSONObject([]byte(msg.Data))
		if err != nil {
			continue
		}
		responseID, _ := json.Marshal(response["id"])
		if !sameRPCID(responseID, requestID) {
			continue
		}
		if _, hasError := response["error"]; hasError {
			injectSSEResponseError(ctx, errors.New("backend tool call failed"), utils.ErrInternalError)
			return []byte{}
		}
		if result, ok := response["result"].(map[string]any); ok {
			// The SSE oracle filtered any result carrying tools, independently
			// of the requested method. Reuse the existing effective ACL helper.
			handler := &McpProtocolHandler{}
			injectSSEResponseSuccess(ctx, handler.applyAllowToolsFilter(ctx, result))
			return []byte{}
		}
		injectSSEResponseError(ctx, errors.New("invalid response format"), utils.ErrInternalError)
		return []byte{}
	}
}
