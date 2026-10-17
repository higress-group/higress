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
	"net/http"
	"slices"
	"strings"
	"sync"

	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/utils"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

// proxyExchange outlives the context keys cleared on completion. Cancellation
// only changes this memory; stream callbacks own all context access/hostcalls.
type proxyExchange struct {
	mu                                                  sync.Mutex
	cancelled, closed, preparing, submitted, responding bool
	unregister                                          func()
}

func (e *proxyExchange) controlActive(ctx wrapper.HttpContext) bool {
	e.mu.Lock()
	closed, cancelled := e.closed, e.cancelled
	e.mu.Unlock()
	if closed {
		return false
	}
	if cancelled {
		sendProxyCancelled(ctx)
		finishProxyRequest(ctx)
		return false
	}
	return true
}

func (e *proxyExchange) finish() {
	e.mu.Lock()
	e.closed = true
	unregister := e.unregister
	e.unregister = nil
	e.mu.Unlock()
	if unregister != nil {
		unregister()
	}
}

func sendProxyCancelled(ctx wrapper.HttpContext) {
	utils.WriteHTTPResponse(ctx, 499, "mcp-proxy:cancelled", [][2]string{{"content-type", "application/json"}}, []byte(`{"error":"MCP request cancelled"}`))
}

// routeProxyBusiness adds only MCP operation/lifecycle handling. The unchanged
// SDK RouteCall owns rewriting, buffering and header/body response callbacks.
func routeProxyBusiness(ctx wrapper.HttpContext, target string, headers [][2]string, body []byte, callback func(int, [][2]string, []byte)) error {
	registerProxyCancellation(ctx)
	e := ctx.GetContext(CtxMcpProxyCancel).(*proxyExchange)
	e.mu.Lock()
	if e.closed || e.cancelled || e.preparing {
		e.mu.Unlock()
		return errors.New("MCP business request is no longer available for submission")
	}
	e.preparing = true
	e.mu.Unlock()
	// RouteCall replaces supplied headers but otherwise keeps the original
	// request. Remove consumed protocol identity before installing this profile.
	original, err := proxywasm.GetHttpRequestHeaders()
	if err != nil {
		return errors.New("failed to read MCP request headers")
	}
	headers = routedHeaderOverrides(original, headers)
	for _, header := range original {
		name := strings.ToLower(header[0])
		if !strings.HasPrefix(name, ":") && name != "host" && reservedProxyHeader(name) {
			if err := proxywasm.RemoveHttpRequestHeader(header[0]); err != nil {
				return errors.New("failed to remove consumed MCP header")
			}
		}
	}
	// Remove all inherited instances of fields that are intentionally replaced,
	// including credential overrides. Unchanged ordinary headers stay in place.
	for _, header := range headers {
		if err := proxywasm.RemoveHttpRequestHeader(header[0]); err != nil {
			return errors.New("failed to replace MCP outbound header")
		}
	}
	err = ctx.RouteCall(http.MethodPost, target, headers, body, func(status int, headers [][2]string, body []byte) {
		e.mu.Lock()
		if e.closed || e.responding {
			e.mu.Unlock()
			return
		}
		e.responding = true
		cancelled := e.cancelled
		e.mu.Unlock()
		ctx.SetContext(utils.CtxRoutedProxyResponse, true)
		defer func() {
			finishProxyRequest(ctx)
			if request, modern := ModernRequestContext(ctx); modern {
				request.Cancel()
			}
		}()
		if cancelled {
			sendProxyCancelled(ctx)
			return
		}
		ctx.SetContext(utils.CtxJsonRpcResponded, false)
		callback(status, headers, body)
		// A concurrent cancellation can win the auto exchange after this
		// callback started. Never release an untreated upstream response.
		if responded, _ := ctx.GetContext(utils.CtxJsonRpcResponded).(bool); !responded {
			sendProxyCancelled(ctx)
		}
	})
	if err != nil {
		return errors.New("failed to prepare routed MCP request")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cancelled || e.closed {
		return errors.New("MCP request cancelled before submission")
	}
	e.submitted = true
	ctx.SetContext(utils.CtxNeedPause, false)
	return nil
}

// RouteCall applies Replace for each pair. Ordinary headers already live on the
// original request, so passing them again would collapse repeated values. Only
// operation identity and values changed by upstream authentication need a set.
func routedHeaderOverrides(original, outbound [][2]string) [][2]string {
	values := func(headers [][2]string, name string) []string {
		var result []string
		for _, header := range headers {
			if strings.EqualFold(header[0], name) {
				result = append(result, header[1])
			}
		}
		return result
	}
	var overrides [][2]string
	for _, header := range outbound {
		if reservedProxyHeader(strings.ToLower(header[0])) || !slices.Equal(values(original, header[0]), values(outbound, header[0])) {
			overrides = append(overrides, header)
		}
	}
	return overrides
}
