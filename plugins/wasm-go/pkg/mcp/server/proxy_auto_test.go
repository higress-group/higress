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
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/protocol"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/proxytest"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	wasmtest "github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func newAutoTestHost(t *testing.T, extra string) wasmtest.TestHost {
	t.Helper()
	saved := globalContext
	globalContext = Context{servers: make(map[string]Server)}
	Initialize()
	t.Cleanup(func() { globalContext = saved })
	config := json.RawMessage(`{"server":{"name":"proxy","type":"mcp-proxy","transport":"http","protocolStrategy":"auto","mcpServerURL":"http://backend.example/a%2Fb?tenant=one"` + extra + `}}`)
	host, status := wasmtest.NewTestHost(config)
	require.Equal(t, types.OnPluginStartStatusOK, status)
	t.Cleanup(host.Reset)
	return host
}

func autoStartList(t *testing.T, host wasmtest.TestHost, id int, extra ...[2]string) proxytest.HttpCalloutAttribute {
	t.Helper()
	host.InitHttp()
	require.Equal(t, types.ActionPause, host.CallOnHttpRequestHeaders(modernProxyListHeaders(extra...)))
	require.Equal(t, types.ActionPause, host.CallOnHttpRequestBody(modernProxyListBody(id)))
	return calloutAt(t, host, 0)
}

func completeAutoResult(host wasmtest.TestHost, call proxytest.HttpCalloutAttribute, result string, headers ...[2]string) {
	responseHeaders := append([][2]string{{"Content-Type", "application/json"}}, headers...)
	completeCallout(host, call, "200", responseHeaders, autoResult(gjson.GetBytes(call.Body, "id").Raw, result))
}

func TestAutoModernFreshProbeAndOpaqueBusiness(t *testing.T) {
	original := dispatchPreparedMCP
	var timeouts []uint32
	dispatchPreparedMCP = func(target proxyTarget, timeout uint32, headers [][2]string, body []byte, maxBody int, callback func(autoHTTPResponse)) error {
		timeouts = append(timeouts, timeout)
		return original(target, timeout, headers, body, maxBody, callback)
	}
	t.Cleanup(func() { dispatchPreparedMCP = original })
	host := newAutoTestHost(t, `,"timeout":500,"autoDetection":{"probeTimeoutMs":700}`)
	for _, id := range []int{71, 72} {
		probe := autoStartList(t, host, id, [2]string{"traceparent", "trace"}, [2]string{"Mcp-Param-Future", "param"}, [2]string{"Cookie", "private=1"})
		assert.Equal(t, uint32(500), timeouts[len(timeouts)-1])
		assert.Equal(t, "server/discover", gjson.GetBytes(probe.Body, "method").String())
		assert.Equal(t, "server/discover", mustHeaderValue(t, probe.Headers, protocol.HeaderMethod))
		assert.Equal(t, "/a%2Fb?tenant=one", mustHeaderValue(t, probe.Headers, ":path"))
		_, param := findHeader(probe.Headers, "Mcp-Param-Future")
		assert.False(t, param)
		completeAutoResult(host, probe, validAutoDiscoverResult)
		business := calloutAt(t, host, 0)
		assert.Equal(t, string(modernProxyListBody(id)), string(business.Body))
		assert.Equal(t, "tools/list", gjson.GetBytes(business.Body, "method").String())
		assert.Equal(t, probe.Upstream, business.Upstream)
		completeAutoResult(host, business, `{"resultType":"complete","tools":[],"opaque":900719925474099312345}`)
		response := host.GetLocalResponse()
		require.NotNil(t, response)
		assert.Equal(t, int64(id), gjson.GetBytes(response.Data, "id").Int())
		assert.Equal(t, "900719925474099312345", gjson.GetBytes(response.Data, "result.opaque").Raw)
		assert.Empty(t, host.GetHttpCalloutAttributes())
		host.CompleteHttp()
	}
}

func TestAutoLegacyHandshakeAndSessionIsolation(t *testing.T) {
	host := newAutoTestHost(t, "")
	for _, version := range []string{"2025-03-26", "2025-06-18"} {
		probe := autoStartList(t, host, 81)
		completeCallout(host, probe, "404", nil, nil)
		initialize := calloutAt(t, host, 0)
		assert.Equal(t, "initialize", gjson.GetBytes(initialize.Body, "method").String())
		assert.Equal(t, "2025-03-26", gjson.GetBytes(initialize.Body, "params.protocolVersion").String())
		sessionHeaders := [][2]string{}
		if version == "2025-06-18" {
			sessionHeaders = append(sessionHeaders, [2]string{"Mcp-Session-Id", "one-request"})
		}
		completeAutoResult(host, initialize, strings.Replace(validAutoInitResult, "2025-03-26", version, 1), sessionHeaders...)
		notification := calloutAt(t, host, 0)
		assert.Equal(t, "notifications/initialized", gjson.GetBytes(notification.Body, "method").String())
		assert.False(t, gjson.GetBytes(notification.Body, "id").Exists())
		completeCallout(host, notification, "202", nil, nil)
		business := calloutAt(t, host, 0)
		assert.False(t, gjson.GetBytes(business.Body, "params._meta").Exists())
		if version == "2025-06-18" {
			assert.Equal(t, version, mustHeaderValue(t, business.Headers, protocol.HeaderProtocolVersion))
			assert.Equal(t, "one-request", mustHeaderValue(t, business.Headers, "Mcp-Session-Id"))
		}
		completeAutoResult(host, business, `{"tools":[],"opaque":900719925474099312345}`)
		response := host.GetLocalResponse()
		require.NotNil(t, response)
		assert.Equal(t, "complete", gjson.GetBytes(response.Data, "result.resultType").String())
		assert.Equal(t, "900719925474099312345", gjson.GetBytes(response.Data, "result.opaque").Raw)
		assert.Empty(t, host.GetHttpCalloutAttributes())
		host.CompleteHttp()
	}
}

func TestAutoCancellationAtEveryStage(t *testing.T) {
	for _, phase := range []string{"discover", "initialize", "initialized", "business"} {
		t.Run(phase, func(t *testing.T) {
			host := newAutoTestHost(t, "")
			call := autoStartList(t, host, 91)
			if phase != "discover" {
				completeCallout(host, call, "404", nil, nil)
				call = calloutAt(t, host, 0)
			}
			if phase == "initialized" || phase == "business" {
				completeAutoResult(host, call, validAutoInitResult)
				call = calloutAt(t, host, 0)
			}
			if phase == "business" {
				completeCallout(host, call, "202", nil, nil)
				call = calloutAt(t, host, 0)
			}
			host.CompleteHttp()
			completeAutoResult(host, call, validAutoDiscoverResult)
			assert.Nil(t, host.GetLocalResponse())
			assert.Empty(t, host.GetHttpCalloutAttributes())
		})
	}
}

func TestAutoFailureNeverDispatchesBusinessAgain(t *testing.T) {
	for _, phase := range []string{"discover", "initialize", "initialized", "business"} {
		t.Run(phase, func(t *testing.T) {
			host := newAutoTestHost(t, "")
			call := autoStartList(t, host, 92)
			if phase != "discover" {
				completeCallout(host, call, "404", nil, nil)
				call = calloutAt(t, host, 0)
			}
			if phase == "initialized" || phase == "business" {
				completeAutoResult(host, call, validAutoInitResult)
				call = calloutAt(t, host, 0)
			}
			if phase == "business" {
				completeCallout(host, call, "202", nil, nil)
				call = calloutAt(t, host, 0)
			}
			// At the business stage this simulates a response lost after backend work.
			// Runtime verification supplies the real backend execution ledger evidence.
			completeCallout(host, call, "503", nil, nil)
			require.NotNil(t, host.GetLocalResponse())
			assert.Empty(t, host.GetHttpCalloutAttributes())
		})
	}
}

func TestAutoCallWithoutListKeepsIdentityAndRejectsLegacyContinuation(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(fmt.Sprint(legacy), func(t *testing.T) {
			host := newAutoTestHost(t, "")
			host.InitHttp()
			body := []byte(`{"jsonrpc":"2.0","id":"business","method":"tools/call","params":{"name":"echo","arguments":{"value":900719925474099312345},"requestState":{"opaque":1},"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}`)
			headers := modernProxyListHeaders([2]string{protocol.HeaderName, "echo"}, [2]string{"Mcp-Param-Future", "value"})
			for i := range headers {
				if headers[i][0] == protocol.HeaderMethod {
					headers[i][1] = "tools/call"
				}
			}
			require.Equal(t, types.ActionPause, host.CallOnHttpRequestHeaders(headers))
			require.Equal(t, types.ActionPause, host.CallOnHttpRequestBody(body))
			probe := calloutAt(t, host, 0)
			_, name := findHeader(probe.Headers, protocol.HeaderName)
			assert.False(t, name)
			if legacy {
				completeCallout(host, probe, "404", nil, nil)
				initialize := calloutAt(t, host, 0)
				completeAutoResult(host, initialize, validAutoInitResult)
				notification := calloutAt(t, host, 0)
				completeCallout(host, notification, "202", nil, nil)
				response := host.GetLocalResponse()
				require.NotNil(t, response)
				assert.Equal(t, int64(-32602), gjson.GetBytes(response.Data, "error.code").Int())
				assert.Empty(t, host.GetHttpCalloutAttributes())
				return
			}
			completeAutoResult(host, probe, validAutoDiscoverResult)
			business := calloutAt(t, host, 0)
			assert.Equal(t, string(body), string(business.Body))
			assert.Equal(t, "value", mustHeaderValue(t, business.Headers, "Mcp-Param-Future"))
			completeCallout(host, business, "400", [][2]string{{"Content-Type", "application/json"}}, autoError(`"business"`, -32020, `{"opaque":900719925474099312345}`))
			response := host.GetLocalResponse()
			require.NotNil(t, response)
			assert.Equal(t, uint32(400), response.StatusCode)
			assert.Equal(t, int64(-32020), gjson.GetBytes(response.Data, "error.code").Int())
			assert.Empty(t, host.GetHttpCalloutAttributes())
		})
	}
}

func TestAutoAuthenticationSnapshotAndTarget(t *testing.T) {
	for _, in := range []string{"header", "query", "cookie"} {
		t.Run(in, func(t *testing.T) {
			location, name := in, "upstream-key"
			if in == "cookie" {
				location, name = "header", "Cookie"
			}
			host := newAutoTestHost(t, `,"securitySchemes":[{"id":"downstream","type":"apiKey","in":"header","name":"X-User"},{"id":"upstream","type":"apiKey","in":"`+location+`","name":"`+name+`"}],"defaultDownstreamSecurity":{"id":"downstream","passthrough":true},"defaultUpstreamSecurity":{"id":"upstream"}`)
			for _, user := range []string{"user-a", "user-b"} {
				credential := user
				if in == "cookie" {
					credential = "upstream-key=" + user
				}
				probe := autoStartList(t, host, 101, [2]string{"X-User", credential})
				_, exists := findHeader(host.GetRequestHeaders(), "X-User")
				assert.False(t, exists)
				completeAutoResult(host, probe, validAutoDiscoverResult)
				business := calloutAt(t, host, 0)
				for _, call := range []proxytest.HttpCalloutAttribute{probe, business} {
					if in == "query" {
						assert.Contains(t, mustHeaderValue(t, call.Headers, ":path"), "upstream-key="+user)
					} else if in == "cookie" {
						assert.Contains(t, mustHeaderValue(t, call.Headers, "Cookie"), "upstream-key="+user)
					} else {
						assert.Equal(t, user, mustHeaderValue(t, call.Headers, "upstream-key"))
					}
					assert.Equal(t, "backend.example", mustHeaderValue(t, call.Headers, ":authority"))
				}
				completeAutoResult(host, business, `{"tools":[],"resultType":"complete"}`)
				host.CompleteHttp()
			}
		})
	}
}

func TestAutoTerminalArbitrationAndDuplicateCallbacks(t *testing.T) {
	t.Run("cancel before dispatch", func(t *testing.T) {
		e := &AutoExchange{phase: autoReady, prepared: &PreparedProxyRequest{raw: []byte("secret")}}
		e.cancel()
		_, ok := e.take(autoReady, autoDispatching)
		assert.False(t, ok)
		assert.False(t, e.businessDispatched)
		assert.Nil(t, e.prepared)
	})
	t.Run("dispatch before cancel", func(t *testing.T) {
		e := &AutoExchange{phase: autoReady, prepared: &PreparedProxyRequest{}}
		_, ok := e.take(autoReady, autoDispatching)
		require.True(t, ok)
		e.cancel()
		_, ok = e.take(autoDispatching, autoResponding)
		assert.False(t, ok)
		assert.True(t, e.businessDispatched)
	})
	t.Run("parallel arbitration", func(t *testing.T) {
		for i := 0; i < 200; i++ {
			e := &AutoExchange{phase: autoReady, prepared: &PreparedProxyRequest{}}
			var wg sync.WaitGroup
			wg.Add(2)
			go func() { defer wg.Done(); e.cancel() }()
			go func() { defer wg.Done(); e.take(autoReady, autoDispatching) }()
			wg.Wait()
			assert.True(t, e.terminal)
			assert.Nil(t, e.prepared)
			_, ok := e.take(autoDispatching, autoResponding)
			assert.False(t, ok)
		}
	})
	for _, phase := range []string{autoProbing, autoInitializing, autoNotifying, autoDispatching} {
		e := &AutoExchange{phase: phase}
		_, ok := e.take(phase, "received")
		require.True(t, ok)
		_, ok = e.take(phase, "received")
		assert.False(t, ok)
		e.terminate("received", "completed")
		_, ok = e.take("received", autoReady)
		assert.False(t, ok)
	}
}

func TestAutoProbeLimitAndOperationalHeaders(t *testing.T) {
	for _, status := range []string{"401", "429", "200"} {
		t.Run(status, func(t *testing.T) {
			host := newAutoTestHost(t, "")
			probe := autoStartList(t, host, 111)
			body := []byte("sensitive upstream body")
			if status == "200" {
				body = []byte(strings.Repeat("x", autoProbeBodyLimit+1))
			}
			completeCallout(host, probe, status, [][2]string{{"WWW-Authenticate", `Bearer realm="test"`}, {"Retry-After", "3"}, {"Set-Cookie", "secret"}}, body)
			response := host.GetLocalResponse()
			require.NotNil(t, response)
			assert.NotContains(t, string(response.Data), "sensitive")
			assert.Empty(t, host.GetHttpCalloutAttributes())
			if status == "200" {
				assert.Contains(t, string(response.Data), "probe_response_too_large")
			} else {
				assert.Equal(t, "3", mustHeaderValue(t, response.Headers, "Retry-After"))
			}
		})
	}
}

func TestAutoEligibilityAndPermissionBoundary(t *testing.T) {
	for _, strategy := range []ProtocolStrategy{"", ProtocolStrategyLegacy, ProtocolStrategyModern, ProtocolStrategyAuto} {
		for _, transport := range []TransportProtocol{TransportHTTP, TransportSSE} {
			handler := NewMcpProtocolHandler("/mcp", 5000)
			handler.SetProtocolStrategy(strategy)
			handler.transport = transport
			assert.Equal(t, strategy == ProtocolStrategyAuto && transport == TransportHTTP, handler.eligibleAuto(modernProxyTestContext("tools/list", nil)))
			assert.False(t, handler.eligibleAuto(&protocolTestHTTPContext{values: map[string]any{}}))
		}
	}
	host := newAutoTestHost(t, "")
	host.InitHttp()
	headers := modernProxyListHeaders([2]string{protocol.HeaderName, "denied"}, [2]string{"x-envoy-allow-mcp-tools", `["allowed"]`})
	for i := range headers {
		if headers[i][0] == protocol.HeaderMethod {
			headers[i][1] = "tools/call"
		}
	}
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"denied","arguments":{},"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}`)
	host.CallOnHttpRequestHeaders(headers)
	host.CallOnHttpRequestBody(body)
	require.NotNil(t, host.GetLocalResponse())
	assert.Empty(t, host.GetHttpCalloutAttributes())
}

func TestAutoDuplicateCallbacksAndDispatchFailure(t *testing.T) {
	// Capture actual callback closures and inject duplicates; a real Envoy never
	// invokes a completed callout callback twice, so this is a lifecycle harness.
	original := dispatchPreparedMCP
	var callbacks []func(autoHTTPResponse)
	var bodies [][]byte
	dispatchPreparedMCP = func(target proxyTarget, timeout uint32, headers [][2]string, body []byte, maxBody int, callback func(autoHTTPResponse)) error {
		callbacks = append(callbacks, callback)
		bodies = append(bodies, append([]byte(nil), body...))
		return nil
	}
	t.Cleanup(func() { dispatchPreparedMCP = original })
	host := newAutoTestHost(t, "")
	host.InitHttp()
	host.CallOnHttpRequestHeaders(modernProxyListHeaders())
	host.CallOnHttpRequestBody(modernProxyListBody(121))
	require.Len(t, callbacks, 1)
	response := jsonAutoResponse(200, autoResult(gjson.GetBytes(bodies[0], "id").Raw, validAutoDiscoverResult))
	callbacks[0](response)
	require.Len(t, callbacks, 2)
	callbacks[0](response)
	require.Len(t, callbacks, 2)
	callbacks[1](jsonAutoResponse(200, autoResult(`121`, `{"tools":[],"resultType":"complete"}`)))
	first := host.GetLocalResponse()
	require.NotNil(t, first)
	callbacks[1](jsonAutoResponse(500, nil))
	callbacks[0](response)
	assert.Equal(t, first, host.GetLocalResponse())
	require.Len(t, callbacks, 2)
}

func TestAutoImmediateDispatchFailures(t *testing.T) {
	for _, failedMethod := range []string{"server/discover", "initialize", "notifications/initialized", "tools/list"} {
		t.Run(failedMethod, func(t *testing.T) {
			original := dispatchPreparedMCP
			dispatchPreparedMCP = func(target proxyTarget, timeout uint32, headers [][2]string, body []byte, maxBody int, callback func(autoHTTPResponse)) error {
				if gjson.GetBytes(body, "method").String() == failedMethod {
					return assert.AnError
				}
				return original(target, timeout, headers, body, maxBody, callback)
			}
			t.Cleanup(func() { dispatchPreparedMCP = original })
			host := newAutoTestHost(t, "")
			host.InitHttp()
			host.CallOnHttpRequestHeaders(modernProxyListHeaders())
			host.CallOnHttpRequestBody(modernProxyListBody(131))
			if failedMethod != "server/discover" {
				completeCallout(host, calloutAt(t, host, 0), "404", nil, nil)
			}
			if failedMethod == "notifications/initialized" || failedMethod == "tools/list" {
				completeAutoResult(host, calloutAt(t, host, 0), validAutoInitResult)
			}
			if failedMethod == "tools/list" {
				completeCallout(host, calloutAt(t, host, 0), "202", nil, nil)
			}
			response := host.GetLocalResponse()
			require.NotNil(t, response)
			assert.Contains(t, string(response.Data), "dispatch_failed")
			assert.Empty(t, host.GetHttpCalloutAttributes())
		})
	}
}

func TestAutoCancellationRegistrationRace(t *testing.T) {
	for i := 0; i < 100; i++ {
		request, protocolErr := protocol.PrepareRequest(protocol.NewTransport("POST", "mcp.example.com", modernProxyListHeaders()), modernProxyListBody(1), func(string) bool { return true })
		require.Nil(t, protocolErr)
		e := &AutoExchange{phase: autoPrepared, prepared: &PreparedProxyRequest{raw: []byte("private")}}
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); e.register(request) }()
		go func() { defer wg.Done(); request.Cancel() }()
		wg.Wait()
		assert.True(t, e.terminal)
		assert.Nil(t, e.prepared)
		assert.Nil(t, e.unregister)
	}
}

func TestAutoLegacyDownstreamDoesNotProbe(t *testing.T) {
	host := newAutoTestHost(t, "")
	host.InitHttp()
	headers := [][2]string{{":method", "POST"}, {":path", "/mcp"}, {":authority", "mcp.example.com"}, {"Content-Type", "application/json"}, {"Accept", "application/json,text/event-stream"}}
	host.CallOnHttpRequestHeaders(headers)
	host.CallOnHttpRequestBody([]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`))
	call := calloutAt(t, host, 0)
	assert.Equal(t, "initialize", gjson.GetBytes(call.Body, "method").String())
	assert.Equal(t, int64(1), gjson.GetBytes(call.Body, "id").Int(), "legacy uses its original handshake")
}

func TestAutoFixedAuthenticationIsPreparedOnce(t *testing.T) {
	host := newAutoTestHost(t, `,"securitySchemes":[{"id":"fixed","type":"apiKey","in":"header","name":"X-Fixed","defaultCredential":"first"}],"defaultUpstreamSecurity":{"id":"fixed"}`)
	probe := autoStartList(t, host, 141)
	config, err := host.GetMatchConfig()
	require.NoError(t, err)
	mcpConfig := config.(*McpServerConfig)
	server := mcpConfig.server.(*McpProxyServer)
	server.AddSecurityScheme(SecurityScheme{ID: "fixed", Type: "apiKey", In: "header", Name: "X-Fixed", DefaultCredential: "changed-after-probe"})
	completeAutoResult(host, probe, validAutoDiscoverResult)
	business := calloutAt(t, host, 0)
	assert.Equal(t, "first", mustHeaderValue(t, business.Headers, "X-Fixed"))
	completeAutoResult(host, business, `{"tools":[],"resultType":"complete"}`)
}

func TestAutoMalformedProtocolCandidatesNeverInitialize(t *testing.T) {
	for _, mode := range []string{"modern-header", "trailing-json", "batch"} {
		t.Run(mode, func(t *testing.T) {
			host := newAutoTestHost(t, "")
			probe := autoStartList(t, host, 151)
			body := autoError(gjson.GetBytes(probe.Body, "id").Raw, -32602, "")
			headers := [][2]string{{"Content-Type", "application/json"}}
			switch mode {
			case "modern-header":
				headers = append(headers, [2]string{protocol.HeaderProtocolVersion, "2026-07-28"})
			case "trailing-json":
				body = append(body, []byte(` {}`)...)
			case "batch":
				body = append(append([]byte("["), body...), ']')
			}
			completeCallout(host, probe, "400", headers, body)
			require.NotNil(t, host.GetLocalResponse())
			assert.Empty(t, host.GetHttpCalloutAttributes(), "malformed/modern errors must not authorize a handshake")
		})
	}
}
