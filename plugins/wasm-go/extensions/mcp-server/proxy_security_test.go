// Copyright (c) 2026 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	sdklog "github.com/higress-group/wasm-go/pkg/log"
	wasmtest "github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const securityInitResult = `{"protocolVersion":"2025-03-26","capabilities":{"tools":{}},"serverInfo":{"name":"fixture","version":"1"}}`
const securityDiscoverResult = `{"resultType":"complete","supportedVersions":["2026-07-28"],"capabilities":{"tools":{}},"ttlMs":0,"cacheScope":"private","_meta":{"io.modelcontextprotocol/serverInfo":{"name":"fixture","version":"1"}}}`

// This exercises the real plugin init, MCPServerLog and SDK RouteCall, including
// the later response callback. It intentionally never enables SafeLog itself.
func TestPluginSafeLogCoversRouteCallRequestAndAsyncResponse(t *testing.T) {
	require.True(t, sdklog.IsSafeLogEnabled(), "the plugin entrypoint must enable SafeLog")
	wasmtest.RunTest(t, func(t *testing.T) {
		config := json.RawMessage(`{"server":{"name":"safe-log","type":"mcp-proxy","transport":"http","protocolStrategy":"modern","mcpServerURL":"http://backend.example/synthetic-url-marker?key=synthetic-query-marker","securitySchemes":[{"id":"key","type":"apiKey","in":"header","name":"X-Upstream-Key","defaultCredential":"synthetic-credential-marker"}],"defaultUpstreamSecurity":{"id":"key"}}}`)
		host, status := wasmtest.NewTestHost(config)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		host.InitHttp()
		body := []byte(`{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"echo","arguments":{"value":"synthetic-argument-marker"},"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}`)
		headers := append(modernHeaders(body), [2]string{"Cookie", "synthetic-cookie-marker"})
		host.CallOnHttpRequestHeaders(headers)
		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody(body))
		require.Empty(t, host.GetHttpCalloutAttributes(), "actual SDK RouteCall must reuse the stream")
		requestLog := strings.Join(host.GetDebugLogs(), "\n")
		require.Contains(t, requestLog, "route call start")
		require.NotContains(t, requestLog, "route call end", "response is asynchronous")
		host.CallOnHttpResponseHeaders([][2]string{{":status", "200"}, {"content-type", "application/json"}, {"X-Result-Token", "synthetic-response-header-marker"}})
		host.CallOnHttpResponseBody([]byte(`{"jsonrpc":"2.0","id":9,"result":{"resultType":"complete","content":[{"type":"text","text":"synthetic-result-marker"}]}}`))
		debug := strings.Join(host.GetDebugLogs(), "\n")
		info := strings.Join(host.GetInfoLogs(), "\n")
		require.Contains(t, debug, "route call end")
		require.Contains(t, debug, "[mcp-server]", "retain the plugin logger")
		for _, marker := range []string{"synthetic-url-marker", "synthetic-query-marker", "synthetic-credential-marker", "synthetic-cookie-marker", "synthetic-argument-marker", "synthetic-response-header-marker", "synthetic-result-marker"} {
			require.NotContains(t, info, marker, "INFO must not expose SDK payload logs")
			require.Contains(t, debug, marker, "SafeLog lowers severity; it does not redact DEBUG")
		}
		require.Contains(t, string(host.GetResponseBody()), "synthetic-result-marker", "logging must not alter forwarding")
		host.CompleteHttp()
	})
}

func securityHeaderValues(headers [][2]string, name string) []string {
	var values []string
	for _, header := range headers {
		if strings.EqualFold(header[0], name) {
			values = append(values, header[1])
		}
	}
	return values
}

func TestProxyAuthorizationInheritanceAcrossTransports(t *testing.T) {
	for _, profile := range []string{"legacy", "modern", "auto-modern", "auto-legacy", "sse"} {
		for _, explicit := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/explicit=%v", profile, explicit), func(t *testing.T) {
				wasmtest.RunTest(t, func(t *testing.T) {
					strategy, transport := profile, "http"
					if strings.HasPrefix(profile, "auto-") {
						strategy = "auto"
					}
					if profile == "sse" {
						strategy, transport = "legacy", "sse"
					}
					security := ""
					if explicit {
						security = `,"securitySchemes":[{"id":"key","type":"apiKey","in":"header","name":"Proxy-Authorization","defaultCredential":"synthetic-configured"}],"defaultUpstreamSecurity":{"id":"key"}`
					}
					config := json.RawMessage(`{"server":{"name":"credential-boundary","type":"mcp-proxy","transport":"` + transport + `","protocolStrategy":"` + strategy + `","mcpServerURL":"http://backend.example/mcp"` + security + `}}`)
					host, status := wasmtest.NewTestHost(config)
					defer host.Reset()
					require.Equal(t, types.OnPluginStartStatusOK, status)
					host.InitHttp()
					params := `{}`
					if profile != "legacy" && profile != "sse" {
						params = `{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}`
					}
					body := []byte(`{"jsonrpc":"2.0","id":19,"method":"tools/list","params":` + params + `}`)
					headers := append(modernHeaders(body), [2]string{"Proxy-Authorization", "synthetic-client-first"}, [2]string{"pRoXy-aUtHoRiZaTiOn", "synthetic-client-second"}, [2]string{"X-Custom", "a"}, [2]string{"x-custom", "b"}, [2]string{"Cookie", "synthetic=ordinary"})
					if profile == "legacy" || profile == "sse" {
						headers = removeHeader(headers, "Mcp-Method")
					}
					assertHeaders := func(headers [][2]string, stage string) {
						t.Helper()
						if explicit {
							require.Equal(t, []string{"synthetic-configured"}, securityHeaderValues(headers, "proxy-authorization"))
						} else {
							require.Empty(t, securityHeaderValues(headers, "proxy-authorization"))
						}
						if stage == "sse/get" {
							// proxytest.addMapValue concatenates AddHttpRequestHeader
							// values without separators. The GET rebuild uses Add;
							// control POSTs below still assert both ordered pairs.
							require.Equal(t, []string{"ab"}, securityHeaderValues(headers, "x-custom"))
						} else {
							require.Equal(t, []string{"a", "b"}, securityHeaderValues(headers, "x-custom"))
						}
						require.Equal(t, []string{"synthetic=ordinary"}, securityHeaderValues(headers, "cookie"))
					}
					host.CallOnHttpRequestHeaders(headers)
					host.CallOnHttpRequestBody(body)
					if profile == "sse" {
						assertHeaders(host.GetRequestHeaders(), "sse/get") // main GET is rebuilt, not a sidecall
						require.Equal(t, []string{"GET"}, securityHeaderValues(host.GetRequestHeaders(), ":method"))
						host.CallOnHttpResponseHeaders([][2]string{{":status", "200"}, {"content-type", "text/event-stream"}})
						host.CallOnHttpStreamingResponseBody([]byte("event: endpoint\ndata: /messages\n\n"), false)
					}
					var operations []string
					for len(host.GetHttpCalloutAttributes()) > 0 {
						require.Less(t, len(operations), 4, "handshake must be bounded")
						call := host.GetHttpCalloutAttributes()[0]
						assertHeaders(call.Headers, "sidecall")
						op := gjson.GetBytes(call.Body, "method").String()
						operations = append(operations, op)
						result := securityInitResult
						switch op {
						case "server/discover":
							if profile == "auto-legacy" {
								completeCallout(host, call, "404", nil, nil)
								continue
							}
							result = securityDiscoverResult
						case "notifications/initialized":
							completeCallout(host, call, "202", nil, nil)
							continue
						case "tools/list":
							result = `{"tools":[]}`
						case "initialize":
						default:
							t.Fatalf("unexpected control operation %s", op)
						}
						response := []byte(`{"jsonrpc":"2.0","id":` + gjson.GetBytes(call.Body, "id").Raw + `,"result":` + result + `}`)
						completeCallout(host, call, "200", [][2]string{{"content-type", "application/json"}}, response)
						if profile == "sse" {
							host.CallOnHttpStreamingResponseBody(append(append([]byte("event: message\ndata: "), response...), []byte("\n\n")...), op == "tools/list")
						}
					}
					want := map[string][]string{"legacy": {"initialize", "notifications/initialized"}, "modern": nil, "auto-modern": {"server/discover"}, "auto-legacy": {"server/discover", "initialize", "notifications/initialized"}, "sse": {"initialize", "notifications/initialized", "tools/list"}}
					require.Equal(t, want[profile], operations, "assert every control and business leg, not an empty sequence")
					if profile != "sse" {
						require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())
						assertHeaders(host.GetRequestHeaders(), "business")
						host.CallOnHttpResponseHeaders([][2]string{{":status", "200"}, {"content-type", "application/json"}})
						host.CallOnHttpResponseBody([]byte(`{"jsonrpc":"2.0","id":` + gjson.GetBytes(host.GetRequestBody(), "id").Raw + `,"result":{"tools":[],"resultType":"complete"}}`))
					}
					host.CompleteHttp()
				})
			})
		}
	}
}
