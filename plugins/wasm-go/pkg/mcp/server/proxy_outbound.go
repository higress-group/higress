// Copyright (c) 2026 Alibaba Group Holding Ltd.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software distributed
// under the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
// CONDITIONS OF ANY KIND, either express or implied.

package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/protocol"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

// PreparedProxyRequest belongs to one exchange. All outgoing authentication and
// routing inputs are resolved before the first callout, including query auth.
type PreparedProxyRequest struct {
	target         proxyTarget
	authHeaders    [][2]string
	forwardHeaders [][2]string
	raw            json.RawMessage
	id             json.RawMessage
	params         map[string]json.RawMessage
	method         string
	toolNameHeader string
	timeout        uint32
	probeTimeout   uint32
}

type proxyTarget struct{ cluster, authority, path string }

// OutboundOperation determines wire identity without consulting the incoming
// method. A discover on behalf of tools/call must still identify as discover.
type OutboundOperation struct {
	method  string
	version protocol.Version
	session string
}

func (p *PreparedProxyRequest) headers(op OutboundOperation) [][2]string {
	headers := [][2]string{{"Content-Type", "application/json"}, {"Accept", "application/json,text/event-stream"}}
	modern := op.version == protocol.Version20260728
	for _, h := range p.forwardHeaders {
		_, trace := traceHeaderNames[strings.ToLower(h[0])]
		if trace || (modern && op.method == string(OpToolsCall) && validModernParamHeader(h[0], h[1])) {
			headers = append(headers, h)
		}
	}
	for _, h := range p.authHeaders {
		ensureHeader(&headers, h[0], h[1])
	}
	if modern {
		ensureHeader(&headers, protocol.HeaderProtocolVersion, string(op.version))
		ensureHeader(&headers, protocol.HeaderMethod, op.method)
		if op.method == string(OpToolsCall) {
			ensureHeader(&headers, protocol.HeaderName, p.toolNameHeader)
		}
	} else {
		if op.version == protocol.Version20250618 {
			ensureHeader(&headers, protocol.HeaderProtocolVersion, string(op.version))
		}
		if op.session != "" {
			ensureHeader(&headers, "Mcp-Session-Id", op.session)
		}
	}
	return headers
}

func (h *McpProtocolHandler) prepareAutoRequest(ctx wrapper.HttpContext, auth *ProxyAuthInfo) (*PreparedProxyRequest, error) {
	request, modern := ModernRequestContext(ctx)
	if !modern {
		return nil, errors.New("auto detection requires a modern request")
	}
	p := &PreparedProxyRequest{raw: bytes.Clone(request.Envelope.Raw), id: request.Envelope.ID.Raw(), method: request.Envelope.Method,
		toolNameHeader: request.Transport.MCPName, timeout: uint32(h.timeout), probeTimeout: h.autoDetection.ProbeTimeoutMs}
	if p.timeout == 0 {
		p.timeout = 5000
	}
	if p.probeTimeout == 0 {
		p.probeTimeout = 1000
	}
	if p.probeTimeout > p.timeout {
		p.probeTimeout = p.timeout
	}
	if err := json.Unmarshal(request.Envelope.Params, &p.params); err != nil {
		return nil, errors.New("invalid proxy request parameters")
	}
	if headers, ok := ctx.GetContext(CtxMcpProxyHeaders).([][2]string); ok {
		p.forwardHeaders = append([][2]string(nil), headers...)
	}
	finalURL := h.backendURL
	if auth != nil {
		if auth.SecuritySchemeID != "" {
			var err error
			finalURL, err = h.applyProxyAuthentication(auth.Server, auth.SecuritySchemeID, auth.PassthroughCredential, &p.authHeaders)
			if err != nil {
				return nil, errors.New("failed to prepare upstream authentication")
			}
		} else if auth.ForwardAuthorization != "" {
			p.authHeaders = append(p.authHeaders, [2]string{"Authorization", auth.ForwardAuthorization})
		}
	}
	for _, header := range p.authHeaders {
		name := strings.ToLower(header[0])
		if strings.HasPrefix(name, "mcp-param-") || name == "mcp-name" || name == "mcp-method" || name == "mcp-protocol-version" || name == "mcp-session-id" || name == "last-event-id" || name == "x-envoy-allow-mcp-tools" {
			return nil, errors.New("upstream authentication conflicts with MCP protocol headers")
		}
	}
	var err error
	p.target, err = resolveProxyTarget(finalURL)
	return p, err
}

func resolveProxyTarget(finalURL string) (proxyTarget, error) {
	parsed, err := url.Parse(finalURL)
	if err != nil {
		return proxyTarget{}, errors.New("failed to parse MCP upstream URL")
	}
	cluster := wrapper.RouteCluster{}
	authority := parsed.Host
	if authority == "" {
		authority = cluster.HostName()
	}
	if authority == "" {
		authority = "unknownhost"
	}
	path := "/" + strings.TrimPrefix(parsed.EscapedPath(), "/")
	if parsed.RawQuery != "" {
		path += "?" + parsed.RawQuery
	}
	return proxyTarget{cluster: cluster.ClusterName(), authority: authority, path: path}, nil
}

func (t proxyTarget) headers(headers [][2]string) [][2]string {
	clean := make([][2]string, 0, len(headers)+3)
	for _, h := range headers {
		if !strings.HasPrefix(h[0], ":") {
			clean = append(clean, h)
		}
	}
	return append(clean, [2]string{":method", http.MethodPost}, [2]string{":path", t.path}, [2]string{":authority", t.authority})
}
