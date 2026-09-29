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
	"strconv"
	"sync"

	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/protocol"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

const (
	autoPrepared     = "prepared"
	autoProbing      = "probing"
	autoProbed       = "probe_received"
	autoInitializing = "initializing"
	autoInitialized  = "initialize_received"
	autoNotifying    = "notifying"
	autoNotified     = "notification_received"
	autoReady        = "ready"
	autoDispatching  = "dispatching"
	autoResponding   = "business_received"
)

// AutoExchange is captured by every callback, including after context cleanup.
// A terminal marker must never be reconstructed from a cleared context key.
// The lock protects only in-memory state; no hostcall runs while it is held.
type AutoExchange struct {
	mu                 sync.Mutex
	phase              string
	terminal           bool
	businessDispatched bool
	profile            ProtocolStrategy
	version            protocol.Version
	session            string
	supported          []string
	prepared           *PreparedProxyRequest
	ctx                wrapper.HttpContext
	unregister         func()
}

type autoStep struct {
	prepared  *PreparedProxyRequest
	ctx       wrapper.HttpContext
	profile   ProtocolStrategy
	version   protocol.Version
	session   string
	supported []string
}

// take linearizes cancellation against acquiring a phase's submission right.
// Cancellation after this point cannot retract a hostcall, but prevents every
// subsequent phase and suppresses the response. Callers never hold the lock.
func (e *AutoExchange) take(expected, next string) (autoStep, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.terminal || e.phase != expected {
		return autoStep{}, false
	}
	if next == autoDispatching {
		if e.businessDispatched {
			return autoStep{}, false
		}
		e.businessDispatched = true
	}
	e.phase = next
	return autoStep{e.prepared, e.ctx, e.profile, e.version, e.session, e.supported}, true
}

func (e *AutoExchange) selectProfile(expected string, profile ProtocolStrategy, version protocol.Version, session string, supported []string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.terminal || e.phase != expected {
		return false
	}
	e.profile, e.version, e.session, e.supported = profile, version, session, supported
	return true
}

func (e *AutoExchange) terminate(expected, terminal string) (autoStep, bool) {
	e.mu.Lock()
	if e.terminal || expected != "" && e.phase != expected {
		e.mu.Unlock()
		return autoStep{}, false
	}
	step := autoStep{e.prepared, e.ctx, e.profile, e.version, e.session, e.supported}
	e.terminal = true
	e.phase = terminal
	e.prepared = nil
	e.ctx = nil
	e.session = ""
	e.supported = nil
	unregister := e.unregister
	e.unregister = nil
	e.mu.Unlock()
	if unregister != nil {
		unregister()
	}
	return step, true
}

func (e *AutoExchange) cancel() { e.terminate("", "cancelled") }

func (e *AutoExchange) register(request *protocol.RequestContext) {
	unregister := request.OnCancel(e.cancel)
	e.mu.Lock()
	if e.terminal {
		e.mu.Unlock()
		unregister()
		return
	}
	e.unregister = unregister
	e.mu.Unlock()
}

func (h *McpProtocolHandler) eligibleAuto(ctx wrapper.HttpContext) bool {
	_, modern := ModernRequestContext(ctx)
	return h.strategy == ProtocolStrategyAuto && h.transport == TransportHTTP && modern
}

func (h *McpProtocolHandler) startAuto(ctx wrapper.HttpContext, auth *ProxyAuthInfo) error {
	prepared, err := h.prepareAutoRequest(ctx, auth)
	if err != nil {
		return err
	}
	e := &AutoExchange{phase: autoPrepared, prepared: prepared, ctx: ctx}
	h.auto = e
	request, _ := ModernRequestContext(ctx)
	e.register(request)
	// Only the exchange now owns the credential/header snapshots. Cancellation
	// never touches HttpContext maps from a foreign goroutine.
	ctx.SetContext(CtxMcpProxyHeaders, nil)
	e.probe(h)
	return nil
}

func (e *AutoExchange) fail(phase string, failure *autoFailure) {
	step, ok := e.terminate(phase, "failed")
	if !ok {
		return
	}
	log.Debugf("MCP auto phase=%s profile=%s outcome=failed reason=%s", phase, step.profile, failure.reason)
	sendAutoFailure(step.ctx, step.prepared.id, phase, failure)
	finishProxyRequest(step.ctx)
}

func (e *AutoExchange) probe(h *McpProtocolHandler) {
	step, ok := e.take(autoPrepared, autoProbing)
	if !ok {
		return
	}
	id := json.RawMessage(`"higress-auto-discover"`)
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": "server/discover", "params": map[string]any{"_meta": map[string]any{
		protocol.MetaProtocolVersion: string(protocol.Version20260728), protocol.MetaClientCapabilities: map[string]any{},
		protocol.MetaClientInfo: map[string]any{"name": "Higress-mcp-proxy", "version": serverImplementationVersion},
	}}})
	op := OutboundOperation{method: "server/discover", version: protocol.Version20260728}
	err := dispatchPreparedMCP(step.prepared.target, step.prepared.probeTimeout, step.prepared.headers(op), body, autoProbeBodyLimit, func(r autoHTTPResponse) {
		if _, ok := e.take(autoProbing, autoProbed); !ok {
			return
		}
		decision := classifyAutoProbe(r, id)
		if decision.failure != nil {
			e.fail(autoProbed, decision.failure)
			return
		}
		log.Debugf("MCP auto phase=probing profile=%s outcome=%s", decision.profile, decision.reason)
		if !e.selectProfile(autoProbed, decision.profile, decision.version, "", decision.supported) {
			return
		}
		if decision.profile == ProtocolStrategyModern {
			e.ready(h, autoProbed)
		} else {
			e.initialize(h)
		}
	})
	if err != nil {
		e.fail(autoProbing, gatewayAutoFailure("dispatch_failed"))
	}
}

func (e *AutoExchange) initialize(h *McpProtocolHandler) {
	step, ok := e.take(autoProbed, autoInitializing)
	if !ok {
		return
	}
	id := json.RawMessage(`"higress-auto-initialize"`)
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": "initialize", "params": map[string]any{
		"protocolVersion": step.version, "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "Higress-mcp-proxy", "version": serverImplementationVersion},
	}})
	op := OutboundOperation{method: "initialize", version: step.version}
	err := dispatchPreparedMCP(step.prepared.target, step.prepared.timeout, step.prepared.headers(op), body, 0, func(r autoHTTPResponse) {
		current, ok := e.take(autoInitializing, autoInitialized)
		if !ok {
			return
		}
		if failure := operationalAutoFailure(r); failure != nil {
			e.fail(autoInitialized, failure)
			return
		}
		envelope, err := decodeAutoResponse(r, id)
		if err != nil {
			e.fail(autoInitialized, gatewayAutoFailure("invalid_initialize_response"))
			return
		}
		if envelope.rpcError != nil {
			e.fail(autoInitialized, protocolAutoFailure(r, envelope))
			return
		}
		if r.status != http.StatusOK {
			e.fail(autoInitialized, gatewayAutoFailure("initialize_rejected"))
			return
		}
		version, err := validateAutoInitialize(envelope, current.supported)
		if err != nil {
			e.fail(autoInitialized, gatewayAutoFailure(err.Error()))
			return
		}
		session, _ := autoHeader(r.headers, "Mcp-Session-Id")
		for _, c := range session {
			if c < 0x21 || c > 0x7e {
				e.fail(autoInitialized, gatewayAutoFailure("invalid_session_id"))
				return
			}
		}
		if !e.selectProfile(autoInitialized, ProtocolStrategyLegacy, version, session, current.supported) {
			return
		}
		e.notify(h)
	})
	if err != nil {
		e.fail(autoInitializing, gatewayAutoFailure("dispatch_failed"))
	}
}

func (e *AutoExchange) notify(h *McpProtocolHandler) {
	step, ok := e.take(autoInitialized, autoNotifying)
	if !ok {
		return
	}
	body := []byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	op := OutboundOperation{method: "notifications/initialized", version: step.version, session: step.session}
	err := dispatchPreparedMCP(step.prepared.target, step.prepared.timeout, step.prepared.headers(op), body, 0, func(r autoHTTPResponse) {
		if _, ok := e.take(autoNotifying, autoNotified); !ok {
			return
		}
		if failure := initializedAutoFailure(r); failure != nil {
			e.fail(autoNotified, failure)
			return
		}
		e.ready(h, autoNotified)
	})
	if err != nil {
		e.fail(autoNotifying, gatewayAutoFailure("dispatch_failed"))
	}
}

func (e *AutoExchange) ready(h *McpProtocolHandler, phase string) {
	step, ok := e.take(phase, autoReady)
	if !ok {
		return
	}
	if step.profile == ProtocolStrategyLegacy {
		for _, name := range []string{"requestState", "inputResponses"} {
			if _, exists := step.prepared.params[name]; exists {
				e.fail(autoReady, &autoFailure{status: 400, code: protocol.CodeInvalidParams, reason: "continuation_requires_modern"})
				return
			}
		}
	}
	// Retain the original business execution entry points and the same final
	// DispatchHttpCall boundary; auto does not fix or change #4597 routing.
	if step.prepared.method == string(OpToolsList) {
		h.executeToolsList(step.ctx)
	} else {
		h.executeToolsCall(step.ctx)
	}
}

func (e *AutoExchange) dispatchBusiness(h *McpProtocolHandler) error {
	step, ok := e.take(autoReady, autoDispatching)
	if !ok {
		return nil
	}
	body := step.prepared.raw
	if step.profile == ProtocolStrategyLegacy {
		params := map[string]json.RawMessage{}
		keys := []string{"cursor"}
		if step.prepared.method == string(OpToolsCall) {
			keys = []string{"name", "arguments"}
		}
		for _, key := range keys {
			if value, exists := step.prepared.params[key]; exists {
				params[key] = value
			}
		}
		body, _ = json.Marshal(map[string]any{"jsonrpc": "2.0", "id": step.prepared.id, "method": step.prepared.method, "params": params})
	}
	op := OutboundOperation{method: step.prepared.method, version: step.version, session: step.session}
	err := dispatchPreparedMCP(step.prepared.target, step.prepared.timeout, step.prepared.headers(op), body, 0, func(r autoHTTPResponse) {
		current, ok := e.take(autoDispatching, autoResponding)
		if !ok {
			return
		}
		envelope, err := decodeAutoResponse(r, current.prepared.id)
		if err != nil || r.status < 200 || (r.status >= 300 && r.status < 400) {
			failure := operationalAutoFailure(r)
			if failure == nil {
				failure = gatewayAutoFailure("invalid_business_response")
			}
			e.fail(autoResponding, failure)
			return
		}
		if envelope.result != nil && r.status != http.StatusOK {
			e.fail(autoResponding, gatewayAutoFailure("invalid_business_status"))
			return
		}
		// Only the winner of completion may access context, emit a response or
		// clean it. Cancellation callbacks do none of these operations.
		completed, ok := e.terminate(autoResponding, "completed")
		if !ok {
			return
		}
		if envelope.result != nil {
			result, _ := decodeBackendJSONObject(envelope.members["result"])
			if completed.prepared.method == string(OpToolsList) {
				result = h.applyAllowToolsFilter(completed.ctx, result)
			}
			result = adaptProxyResult(completed.ctx, completed.profile == ProtocolStrategyModern, result)
			envelope.members["result"], _ = json.Marshal(result)
		}
		// Rebind even a permitted no-ID HTTP error to the original business ID.
		envelope.members["id"] = completed.prepared.id
		responseBody, _ := json.Marshal(envelope.members)
		proxywasm.SendHttpResponseWithDetail(uint32(r.status), "mcp-proxy:auto:business", safeAutoResponseHeaders(r.headers), responseBody, -1)
		finishProxyRequest(completed.ctx)
	})
	if err != nil {
		e.fail(autoDispatching, gatewayAutoFailure("dispatch_failed"))
	}
	return nil
}

var dispatchPreparedMCP = postPreparedMCP

// The host buffers its response before invoking this callback. The probe cap
// prevents copying an oversized response into Wasm, not host-side buffering.
func postPreparedMCP(target proxyTarget, timeout uint32, headers [][2]string, body []byte, maxBody int, callback func(autoHTTPResponse)) error {
	_, err := proxywasm.DispatchHttpCall(target.cluster, target.headers(headers), body, nil, timeout, func(numHeaders, bodySize, numTrailers int) {
		headers, err := proxywasm.GetHttpCallResponseHeaders()
		response := autoHTTPResponse{headers: headers}
		if err != nil {
			response.err = errors.New("response_headers_unavailable")
			callback(response)
			return
		}
		status, _ := autoHeader(headers, ":status")
		response.status, _ = strconv.Atoi(status)
		if maxBody > 0 && bodySize > maxBody {
			response.err = errAutoResponseTooLarge
			callback(response)
			return
		}
		if bodySize < 0 {
			response.err = errors.New("invalid_body_size")
			callback(response)
			return
		}
		if bodySize > 0 {
			response.body, err = proxywasm.GetHttpCallResponseBody(0, bodySize)
			if err != nil {
				response.err = errors.New("response_body_unavailable")
			}
		}
		callback(response)
	})
	return err
}
