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
	"testing"

	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/protocol"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	wasmtest "github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestInstructionsConfigurationMatrix(t *testing.T) {
	for _, tc := range []struct {
		name, server, extra string
		invalid             bool
	}{
		{"modern enabled", `"type":"mcp-proxy","transport":"http","protocolStrategy":"modern","mcpServerURL":"/mcp","passthroughInstructions":true`, "", false},
		{"legacy enabled", `"type":"mcp-proxy","transport":"http","mcpServerURL":"/mcp","passthroughInstructions":true`, "", true},
		{"SSE enabled", `"type":"mcp-proxy","transport":"sse","protocolStrategy":"legacy","mcpServerURL":"/sse","passthroughInstructions":true`, "", true},
		{"SSE false", `"type":"mcp-proxy","transport":"sse","protocolStrategy":"legacy","mcpServerURL":"/sse","passthroughInstructions":false`, "", false},
		{"auto enabled", `"type":"mcp-proxy","transport":"http","protocolStrategy":"auto","mcpServerURL":"/mcp","passthroughInstructions":true`, "", true},
		{"rest enabled", `"type":"rest","passthroughInstructions":true`, "", true},
		{"shadowed by toolSet", `"type":"mcp-proxy","transport":"http","protocolStrategy":"modern","mcpServerURL":"/mcp","passthroughInstructions":true`, `,"toolSet":{"name":"composed","serverTools":[]}`, true},
		{"rest false", `"type":"rest","passthroughInstructions":false`, "", false},
		{"legacy false", `"type":"mcp-proxy","transport":"http","mcpServerURL":"/mcp","passthroughInstructions":false`, "", false},
		{"auto false", `"type":"mcp-proxy","transport":"http","protocolStrategy":"auto","mcpServerURL":"/mcp","passthroughInstructions":false`, "", false},
		{"toolSet false", `"passthroughInstructions":false`, `,"toolSet":{"name":"composed","serverTools":[]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registry := &GlobalToolRegistry{}
			registry.Initialize()
			var config McpServerConfig
			err := ParseConfigCore(gjson.Parse(`{"server":{"name":"fixture",`+tc.server+`}`+tc.extra+`}`), &config, &ConfigOptions{Servers: map[string]Server{}, ToolRegistry: registry, SkipPreRegisteredServers: true})
			if tc.invalid {
				require.ErrorContains(t, err, "passthroughInstructions")
			} else {
				require.NoError(t, err)
			}
		})
	}
	for _, value := range []string{`null`, `"true"`, `0`, `1`, `[]`, `{}`} {
		t.Run(value, func(t *testing.T) {
			registry := &GlobalToolRegistry{}
			registry.Initialize()
			err := ParseConfigCore(gjson.Parse(`{"server":{"name":"fixture","passthroughInstructions":`+value+`}}`), &McpServerConfig{}, &ConfigOptions{ToolRegistry: registry, SkipPreRegisteredServers: true})
			require.ErrorContains(t, err, "must be a boolean")
		})
	}
	s := NewMcpProxyServer("clone")
	s.passthroughInstructions = true
	clone := s.Clone().(*McpProxyServer)
	require.True(t, clone.passthroughInstructions)
	clone.passthroughInstructions = false
	require.True(t, s.passthroughInstructions)
}

func newInstructionsHost(t *testing.T, flag, extra string) wasmtest.TestHost {
	t.Helper()
	saved := globalContext
	globalContext = Context{servers: make(map[string]Server)}
	Initialize()
	t.Cleanup(func() { globalContext = saved })
	config := `{"server":{"name":"instructions-proxy","type":"mcp-proxy","transport":"http","protocolStrategy":"modern","mcpServerURL":"http://backend.example/mcp?fixed=yes"` + flag + extra + `}}`
	host, status := wasmtest.NewTestHost(json.RawMessage(config))
	require.Equal(t, types.OnPluginStartStatusOK, status)
	t.Cleanup(host.Reset)
	return host
}

func instructionsBody() []byte {
	return []byte(`{"jsonrpc":"2.0","id":"original-discover","method":"server/discover","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientInfo":{"name":"real-client","version":"1"},"io.modelcontextprotocol/clientCapabilities":{},"example.com/opaque":{"preserve":true}}}}`)
}

func startInstructions(t *testing.T, host wasmtest.TestHost, extra ...[2]string) {
	t.Helper()
	headers := modernProxyListHeaders(extra...)
	for i := range headers {
		if strings.EqualFold(headers[i][0], protocol.HeaderMethod) {
			headers[i][1] = "server/discover"
		}
	}
	require.Equal(t, types.ActionPause, host.CallOnHttpRequestHeaders(headers))
	require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody(instructionsBody()))
}

func TestInstructionsDisabledKeepsLocalDiscovery(t *testing.T) {
	var baseline []byte
	for _, flag := range []string{"", `,"passthroughInstructions":false`} {
		t.Run(flag, func(t *testing.T) {
			host := newInstructionsHost(t, flag, "")
			startInstructions(t, host)
			require.Empty(t, host.GetHttpCalloutAttributes())
			response := host.GetLocalResponse()
			require.NotNil(t, response)
			require.Equal(t, uint32(200), response.StatusCode)
			if baseline == nil {
				baseline = append([]byte(nil), response.Data...)
			} else {
				require.Equal(t, baseline, response.Data)
			}
			require.False(t, gjson.GetBytes(response.Data, "result.instructions").Exists())
		})
	}
}

func TestInstructionsRoutedSuccessAndFieldIsolation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		value   any
		present bool
	}{
		{"unicode", "line one\n中文🌤️ \"quoted\"", true}, {"whitespace", " \n ", true},
		{"limit", strings.Repeat("界", 10922) + "xx", true}, {"oversize", strings.Repeat("x", 32769), false},
		{"missing", nil, false}, {"empty", "", false}, {"number", 9, false}, {"object", map[string]any{"no": "string"}, false}, {"null", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			host := newInstructionsHost(t, `,"passthroughInstructions":true`, "")
			startInstructions(t, host)
			req := routedRequest(t, host)
			require.Equal(t, instructionsBody(), req.Body)
			require.Equal(t, "server/discover", mustHeaderValue(t, req.Headers, protocol.HeaderMethod))
			for _, name := range []string{"Mcp-Session-Id", "Mcp-Name", "Mcp-Param-Secret"} {
				_, exists := findHeader(req.Headers, name)
				require.False(t, exists)
			}
			result := map[string]any{"instructions": tc.value, "supportedVersions": []string{"wrong"}, "capabilities": map[string]any{"resources": map[string]any{}}, "ttlMs": 0.5, "cacheScope": "public", "_meta": map[string]any{serverInfoMetaKey: map[string]any{"name": "upstream", "version": "99"}}}
			if tc.name == "missing" {
				delete(result, "instructions")
			}
			body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": "upstream-id-is-not-authoritative", "result": result})
			require.NoError(t, err)
			contentType := "application/json"
			if tc.name == "unicode" {
				contentType = "text/event-stream"
				body = []byte("data: " + string(body) + "\n\n")
			}
			completeCallout(host, req, "200", [][2]string{{"content-type", contentType}, {"X-Upstream-Only", "drop"}}, body)
			response := proxyTestResponse(host)
			require.NotNil(t, response)
			require.Equal(t, uint32(200), response.StatusCode)
			require.Equal(t, "original-discover", gjson.GetBytes(response.Data, "id").String())
			got := gjson.GetBytes(response.Data, "result.instructions")
			require.Equal(t, tc.present, got.Exists())
			if tc.present {
				require.Equal(t, tc.value, got.String())
			}
			require.Equal(t, int64(0), gjson.GetBytes(response.Data, "result.ttlMs").Int())
			require.Equal(t, "private", gjson.GetBytes(response.Data, "result.cacheScope").String())
			require.Equal(t, "complete", gjson.GetBytes(response.Data, "result.resultType").String())
			require.Equal(t, "instructions-proxy", gjson.GetBytes(response.Data, `result._meta.io\.modelcontextprotocol/serverInfo.name`).String())
			require.True(t, gjson.GetBytes(response.Data, "result.capabilities.tools").Exists())
			require.False(t, gjson.GetBytes(response.Data, "result.capabilities.resources").Exists())
			require.Contains(t, gjson.GetBytes(response.Data, "result.supportedVersions").Raw, "2026-07-28")
			_, leaked := findHeader(response.Headers, "X-Upstream-Only")
			require.False(t, leaked)
		})
	}
}

func TestInstructionsFailuresAndIncompleteResultsKeepProxyPaths(t *testing.T) {
	for _, tc := range []struct {
		name, status, body string
		incomplete         bool
	}{
		{"unauthorized", "401", `{"error":"no"}`, false}, {"forbidden", "403", `{}`, false}, {"unavailable", "503", `{}`, false},
		{"result array", "200", `{"jsonrpc":"2.0","result":[]}`, false}, {"null result", "200", `{"jsonrpc":"2.0","result":null}`, false},
		{"invalid JSON", "200", `not-json`, false}, {"RPC error", "200", `{"jsonrpc":"2.0","error":{"code":-32603,"message":"no"},"result":{"instructions":"must-not-merge"}}`, false},
		{"incomplete", "200", `{"jsonrpc":"2.0","result":{"resultType":"input_required","inputRequests":[],"instructions":"existing-result"}}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			host := newInstructionsHost(t, `,"passthroughInstructions":true`, "")
			startInstructions(t, host)
			completeCallout(host, routedRequest(t, host), tc.status, [][2]string{{"content-type", "application/json"}}, []byte(tc.body))
			response := proxyTestResponse(host)
			require.NotNil(t, response)
			if tc.incomplete {
				require.Equal(t, "input_required", gjson.GetBytes(response.Data, "result.resultType").String())
				require.False(t, gjson.GetBytes(response.Data, "result.supportedVersions").Exists())
			} else {
				require.True(t, gjson.GetBytes(response.Data, "error").Exists())
				require.False(t, gjson.GetBytes(response.Data, "result").Exists())
			}
			require.Empty(t, host.GetHttpCalloutAttributes())
		})
	}
}

func TestInstructionsServerAuthUsesExistingRules(t *testing.T) {
	for _, mode := range []string{"anonymous", "header", "query", "passthrough"} {
		t.Run(mode, func(t *testing.T) {
			extra := ""
			if mode == "header" || mode == "query" {
				extra = fmt.Sprintf(`,"securitySchemes":[{"id":"key","type":"apiKey","in":%q,"name":"X-Key","defaultCredential":"configured"}],"defaultUpstreamSecurity":{"id":"key"}`, mode)
			}
			if mode == "passthrough" {
				extra = `,"passthroughAuthHeader":true`
			}
			host := newInstructionsHost(t, `,"passthroughInstructions":true`, extra)
			startInstructions(t, host, [2]string{"Authorization", "Bearer current-client"})
			req := routedRequest(t, host)
			auth, _ := findHeader(req.Headers, "Authorization")
			if mode == "passthrough" {
				require.Equal(t, "Bearer current-client", auth)
			} else {
				require.Empty(t, auth)
			}
			if mode == "header" {
				require.Equal(t, "configured", mustHeaderValue(t, req.Headers, "X-Key"))
			}
			if mode == "query" {
				require.Contains(t, mustHeaderValue(t, req.Headers, ":path"), "X-Key=configured")
			}
		})
	}
}
