// Copyright (c) 2026 Alibaba Group Holding Ltd.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software distributed
// under the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
// CONDITIONS OF ANY KIND, either express or implied.

package server

import (
	"errors"
	"strings"

	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/protocol"
	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/utils"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

const maxDiscoveryInstructionsBytes = 32 * 1024

// forwardDiscovery adds only an optional field on the existing HTTP proxy path.
// RouteCall continues to own routing, buffering, timeout/retry and lifecycle.
func (s *McpProxyServer) forwardDiscovery(ctx wrapper.HttpContext, localResult map[string]any) error {
	_, modern := ModernRequestContext(ctx)
	if !modern || s.GetProtocolStrategy() != ProtocolStrategyModern || s.GetTransport() != TransportHTTP {
		return errors.New("instructions passthrough requires modern HTTP discovery")
	}
	auth := s.prepareServerProxyAuth(ctx)
	handler := NewMcpProtocolHandler(s.GetMcpServerURL(), s.GetTimeout())
	prepared, err := handler.prepareProxyRequest(ctx, auth)
	if err != nil {
		return err
	}
	op := OutboundOperation{method: "server/discover", version: protocol.Version20260728}
	body, err := prepared.body(op)
	if err != nil {
		return err
	}
	err = routeProxyBusiness(ctx, prepared.url, prepared.headers(op), body, func(status int, responseHeaders [][2]string, responseBody []byte) {
		if sendUpstreamAuthFailure(ctx, status, responseHeaders, "server/discover") {
			return
		}
		if status != 200 {
			utils.OnMCPResponseError(ctx, errors.New("backend server/discover failed"), utils.ErrInternalError, "mcp-proxy:server/discover:backend_error")
			return
		}
		jsonBody := responseBody
		for _, header := range responseHeaders {
			if strings.EqualFold(header[0], "content-type") && strings.Contains(strings.ToLower(header[1]), "text/event-stream") {
				var err error
				jsonBody, err = parseSSEResponse(responseBody)
				if err != nil {
					utils.OnMCPResponseError(ctx, errors.New("invalid server/discover SSE response"), utils.ErrInternalError, "mcp-proxy:server/discover:parse_error")
					return
				}
				break
			}
		}
		response, err := decodeBackendJSONObject(jsonBody)
		if err != nil {
			utils.OnMCPResponseError(ctx, errors.New("invalid server/discover response"), utils.ErrInternalError, "mcp-proxy:server/discover:parse_error")
			return
		}
		result, ok := response["result"].(map[string]any)
		_, hasError := response["error"]
		if !ok || hasError {
			utils.OnMCPResponseError(ctx, errors.New("invalid server/discover result"), utils.ErrInternalError, "mcp-proxy:server/discover:backend_error")
			return
		}
		// Explicit incomplete results retain the existing modern proxy result
		// path rather than being converted into a complete local discovery.
		if resultType, present := result["resultType"]; present && resultType != resultTypeComplete {
			utils.OnMCPResponseSuccess(ctx, result, "mcp-proxy:server/discover:incomplete")
			return
		}
		if instructions, ok := result["instructions"].(string); ok && instructions != "" && len(instructions) <= maxDiscoveryInstructionsBytes {
			localResult["instructions"] = instructions
		}
		utils.OnMCPResponseSuccess(ctx, localResult, "mcp-proxy:server/discover:success")
	})
	if err != nil {
		finishProxyRequest(ctx)
	}
	return err
}
