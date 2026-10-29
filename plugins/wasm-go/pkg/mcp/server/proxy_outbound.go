// Copyright (c) 2026 Alibaba Group Holding Ltd.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software distributed
// under the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
// CONDITIONS OF ANY KIND, either express or implied.

package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/protocol"
	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/utils"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

// PreparedProxyRequest belongs to one exchange. All outgoing authentication and
// routing inputs are resolved before the first callout, including query auth.
type PreparedProxyRequest struct {
	target         proxyTarget
	url            string
	authHeaders    [][2]string
	forwardHeaders [][2]string
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
	headers = append(headers, inheritedProxyHeaders(p.forwardHeaders, modern && op.method == string(OpToolsCall))...)
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

func (h *McpProtocolHandler) prepareProxyRequest(ctx wrapper.HttpContext, auth *ProxyAuthInfo) (*PreparedProxyRequest, error) {
	p := &PreparedProxyRequest{params: make(map[string]json.RawMessage), timeout: uint32(h.timeout), probeTimeout: h.autoDetection.ProbeTimeoutMs}
	if request, modern := ModernRequestContext(ctx); modern {
		p.id = request.Envelope.ID.Raw()
		p.method, p.toolNameHeader = request.Envelope.Method, request.Transport.MCPName
		var err error
		p.params, err = protocol.DecodeBusinessJSONObject(request.Envelope.Params)
		if err != nil {
			return nil, errors.New("invalid proxy request parameters")
		}
	} else {
		if id, ok := ctx.GetContext(utils.CtxJsonRpcClientID).(protocol.ID); ok {
			p.id = id.Raw()
		}
		operation, _ := ctx.GetContext(CtxMcpProxyOperation).(McpProxyOperation)
		p.method = string(operation)
	}
	// These are the effective fields already selected by the historical
	// handlers. Argument maps retain their recursive last-key/UseNumber reads.
	if p.method == string(OpToolsCall) {
		if name, ok := ctx.GetContext(CtxMcpProxyToolName).(string); ok {
			p.params["name"], _ = json.Marshal(name)
		}
		if arguments, ok := ctx.GetContext(CtxMcpProxyToolArgs).(map[string]any); ok {
			p.params["arguments"], _ = json.Marshal(arguments)
		}
		if _, present := p.params["arguments"]; !present {
			p.params["arguments"] = json.RawMessage(`{}`)
		}
	} else if p.method == string(OpToolsList) {
		cursor := gjson.ParseBytes(p.params["cursor"]).String()
		if selected, ok := ctx.GetContext(CtxMcpProxyCursor).(string); ok {
			cursor = selected
		}
		delete(p.params, "cursor")
		if cursor != "" {
			p.params["cursor"], _ = json.Marshal(cursor)
		}
	}
	if p.timeout == 0 {
		p.timeout = 5000
	}
	if p.probeTimeout == 0 {
		p.probeTimeout = 1000
	}
	if p.probeTimeout > p.timeout {
		p.probeTimeout = p.timeout
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
		if proxyAuthHeaderConflicts(name) {
			return nil, errors.New("upstream authentication conflicts with MCP protocol headers")
		}
	}
	var err error
	p.url = finalURL
	p.target, err = resolveProxyTarget(finalURL)
	return p, err
}

// body serializes the selected operation independently of the client's loose
// envelope. Only proxy boundaries decode argument maps; direct tools keep Raw.
func (p *PreparedProxyRequest) body(op OutboundOperation) ([]byte, error) {
	params := make(map[string]any)
	modern := op.version == protocol.Version20260728
	for name, raw := range p.params {
		if !modern && ((op.method == string(OpToolsList) && name != "cursor") ||
			(op.method == string(OpToolsCall) && name != "name" && name != "arguments")) {
			continue
		}
		value, err := decodeBusinessJSONValue(raw)
		if err != nil {
			return nil, errors.New("invalid proxy request parameters")
		}
		params[name] = value
	}
	if modern {
		meta, _ := params["_meta"].(map[string]any)
		if meta == nil {
			meta = make(map[string]any)
		}
		meta[protocol.MetaProtocolVersion] = string(op.version)
		meta[protocol.MetaClientCapabilities] = map[string]any{}
		meta[protocol.MetaClientInfo] = map[string]any{"name": "Higress-mcp-proxy", "version": serverImplementationVersion}
		params["_meta"] = meta
	}
	clientID, protocolError := protocol.ParseID(p.id)
	if protocolError != nil {
		return nil, protocolError
	}
	id := clientID.CorrelationID("higress-proxy-" + op.method)
	if !modern {
		// Legacy operations keep their historical internal IDs. Neither these
		// nor a Modern correlation replacement become the downstream identity.
		id = json.RawMessage("2")
		if op.method == string(OpToolsCall) {
			id = json.RawMessage("3")
		}
	}
	return json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": op.method, "params": params})
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
