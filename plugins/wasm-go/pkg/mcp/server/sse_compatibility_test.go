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
	"compress/gzip"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/utils"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/proxytest"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	pb "github.com/higress-group/wasm-go/pkg/protos"
	wasmtest "github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"google.golang.org/protobuf/proto"
)

const sseOriginalCluster = "custom-original-cluster"

func newSSECompatibilityHost(t *testing.T, configuredURL, extra string) wasmtest.TestHost {
	t.Helper()
	saved := globalContext
	globalContext = Context{servers: make(map[string]Server)}
	Initialize()
	t.Cleanup(func() { globalContext = saved })
	host := newValidationHost(t, json.RawMessage(`{"server":{"name":"proxy","type":"mcp-proxy","transport":"sse","mcpServerURL":`+strconv.Quote(configuredURL)+`,"timeout":731`+extra+`}}`))
	host.RegisterForeignFunction("inject_encoded_data_to_filter_chain_on_header", func(raw []byte) []byte {
		args := &pb.InjectEncodedDataToFilterChainArguments{}
		require.NoError(t, proto.Unmarshal(raw, args))
		require.True(t, args.Endstream)
		require.NoError(t, proxywasm.InjectEncodedDataToFilterChain([]byte(args.Body), true))
		return []byte{0}
	})
	return host
}

func startSSECompatibility(t *testing.T, host wasmtest.TestHost, modern bool, method, id string, extra ...[2]string) {
	t.Helper()
	host.InitHttp()
	require.NoError(t, host.SetClusterName(sseOriginalCluster))
	headers, body := looseProxyRequest(modern, method, id)
	headers = append(headers, extra...)
	require.Equal(t, types.ActionPause, host.CallOnHttpRequestHeaders(headers))
	require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody(body))
	require.Nil(t, host.GetLocalResponse(), "the main GET must remain open for its SSE response")
	require.Empty(t, host.GetHttpCalloutAttributes(), "the initial GET uses the main request")
	require.Equal(t, "GET", mustHeaderValue(t, host.GetRequestHeaders(), ":method"))
	require.Equal(t, "text/event-stream", mustHeaderValue(t, host.GetRequestHeaders(), "accept"))
}

func sendSSEMessage(host wasmtest.TestHost, event, data string) {
	host.CallOnHttpStreamingResponseBody([]byte("event: "+event+"\ndata: "+data+"\n\n"), false)
}

func openSSECompatibility(t *testing.T, host wasmtest.TestHost, endpoint string) proxytest.HttpCalloutAttribute {
	t.Helper()
	require.Equal(t, types.ActionPause, host.CallOnHttpResponseHeaders([][2]string{{":status", "200"}, {"content-type", "text/event-stream"}}))
	sendSSEMessage(host, "endpoint", endpoint)
	initialize := calloutAt(t, host, 0)
	strictProxyOutbound(t, initialize, "initialize", false)
	require.Equal(t, "initialize", gjson.GetBytes(initialize.Body, "method").String())
	require.Equal(t, "{}", gjson.GetBytes(initialize.Body, "params.capabilities").Raw,
		"the gateway implements no roots/sampling/elicitation callbacks")
	require.Equal(t, sseOriginalCluster, initialize.Upstream)
	require.Equal(t, "POST", mustHeaderValue(t, initialize.Headers, ":method"))
	require.Equal(t, "application/json", mustHeaderValue(t, initialize.Headers, "content-type"))
	require.Contains(t, mustHeaderValue(t, initialize.Headers, "accept"), "application/json")
	return initialize
}

func advanceSSECompatibility(t *testing.T, host wasmtest.TestHost, initialize proxytest.HttpCalloutAttribute) (proxytest.HttpCalloutAttribute, proxytest.HttpCalloutAttribute) {
	t.Helper()
	completeCallout(host, initialize, "202", nil, nil)
	sendSSEMessage(host, "message", string(autoResult(`1`, `{"protocolVersion":"2025-03-26","capabilities":{}}`)))
	notification := calloutAt(t, host, 0)
	require.Equal(t, "notifications/initialized", gjson.GetBytes(notification.Body, "method").String())
	require.False(t, gjson.GetBytes(notification.Body, "id").Exists())
	completeCallout(host, notification, "202", nil, nil)
	business := calloutAt(t, host, 0)
	return notification, business
}

func TestSSEHistoricalEndpointURLsKeepOriginalCluster(t *testing.T) {
	cases := []struct {
		name, configured, endpoint, getAuthority, getPath, postAuthority, postPath string
	}{
		{"relative", "http://backend.example:8080/sse?initial=1", "messages?session=one", "backend.example:8080", "/sse?initial=1", "backend.example:8080", "/messages?session=one"},
		{"absolute-path", "https://backend.example/sse", "/messages?session=two", "backend.example", "/sse", "backend.example", "/messages?session=two"},
		{"cross-Authority", "http://backend.example/sse", "https://other.example:8443/messages?session=three", "backend.example", "/sse", "other.example:8443", "/messages?session=three"},
		{"path-config-full-endpoint", "/sse?initial=1", "https://other.example/messages", "", "/sse?initial=1", "other.example", "/messages"},
		{"path-config-path-endpoint-old-limit", "/sse", "/messages", "", "/sse", "unknownhost", "/messages"},
		{"network-path-with-full-base", "http://backend.example/sse", "//other.example/messages", "backend.example", "/sse", "backend.example", "//other.example/messages"},
		{"network-path-with-path-base", "/sse", "//other.example/messages", "", "/sse", "other.example", "/messages"},
		{"escaped-paths", "http://backend.example/a%2Fb?initial=1", "https://other.example/c%2Fd?q=a%2Bb", "backend.example", "/a/b?initial=1", "other.example", "/c%2Fd?q=a%2Bb"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			host := newSSECompatibilityHost(t, item.configured, "")
			startSSECompatibility(t, host, true, string(OpToolsList), `1.50`, [2]string{"Origin", "null"})
			assert.Equal(t, item.getAuthority, mustHeaderValue(t, host.GetRequestHeaders(), ":authority"))
			assert.Equal(t, item.getPath, mustHeaderValue(t, host.GetRequestHeaders(), ":path"))
			// Even a changed property must not make endpoint POSTs switch away
			// from the original cached route's cluster.
			require.NoError(t, host.SetClusterName("changed-later-cluster"))
			initialize := openSSECompatibility(t, host, item.endpoint)
			notification, business := advanceSSECompatibility(t, host, initialize)
			for _, request := range []proxytest.HttpCalloutAttribute{initialize, notification, business} {
				assert.Equal(t, sseOriginalCluster, request.Upstream)
				assert.Equal(t, item.postAuthority, mustHeaderValue(t, request.Headers, ":authority"))
				assert.Equal(t, item.postPath, mustHeaderValue(t, request.Headers, ":path"))
			}
			strictProxyOutbound(t, business, string(OpToolsList), false)
			completeCallout(host, business, "202", nil, nil)
			sendSSEMessage(host, "message", string(autoResult(gjson.GetBytes(business.Body, "id").Raw, `{"tools":[]}`)))
			assert.Equal(t, `1.50`, gjson.GetBytes(host.GetResponseBody(), "id").Raw)
		})
	}
}

func TestSSELosslessIDsNumbersAndHistoricalBusinessErrors(t *testing.T) {
	ids := []string{`"client\u002did"`, `9007199254740993`, `9223372036854775808`, `1.50`, `1E+03`, `-0`, `null`, ``}
	for _, modern := range []bool{false, true} {
		for index, id := range ids {
			for _, method := range []string{string(OpToolsList), string(OpToolsCall)} {
				for _, rpcError := range []bool{false, true} {
					t.Run(fmt.Sprintf("modern=%t/id=%d/%s/error=%t", modern, index, method, rpcError), func(t *testing.T) {
						host := newSSECompatibilityHost(t, "http://backend.example/sse", "")
						startSSECompatibility(t, host, modern, method, id)
						initialize := openSSECompatibility(t, host, "https://other.example/messages")
						_, business := advanceSSECompatibility(t, host, initialize)
						strictProxyOutbound(t, business, method, false)
						wantUpstreamID := "2"
						if method == string(OpToolsCall) {
							wantUpstreamID = "3"
						}
						assert.Equal(t, wantUpstreamID, gjson.GetBytes(business.Body, "id").Raw)
						if method == string(OpToolsCall) {
							assert.Equal(t, "900719925474099312345", gjson.GetBytes(business.Body, "params.arguments.number").Raw)
							assert.Equal(t, "last", gjson.GetBytes(business.Body, "params.arguments.nested.value").String())
						}
						completeCallout(host, business, "202", nil, nil)
						body := autoResult(gjson.GetBytes(business.Body, "id").Raw, `{"tools":[],"content":[],"number":900719925474099312345,"value":1,"value":2,"deep":`+strings.Repeat("[", 70)+`0`+strings.Repeat("]", 70)+`}`)
						if rpcError {
							body = autoError(gjson.GetBytes(business.Body, "id").Raw, -32099, `{"discard":true}`)
						}
						sendSSEMessage(host, "message", string(body))
						response := proxyTestResponse(host)
						require.NotNil(t, response)
						if modern && id == "" {
							assert.Equal(t, uint32(202), response.StatusCode)
							assert.Empty(t, response.Data)
						} else {
							wantID := id
							if wantID == "" {
								wantID = "0"
							}
							assert.Equal(t, wantID, gjson.GetBytes(response.Data, "id").Raw)
							if rpcError {
								assert.Equal(t, "backend tool call failed", gjson.GetBytes(response.Data, "error.message").String())
								assert.Equal(t, int64(utils.ErrInternalError), gjson.GetBytes(response.Data, "error.code").Int())
								assert.False(t, gjson.GetBytes(response.Data, "error.data").Exists())
							} else {
								assert.Equal(t, "900719925474099312345", gjson.GetBytes(response.Data, "result.number").Raw)
								assert.Equal(t, "2", gjson.GetBytes(response.Data, "result.value").Raw)
							}
						}
						first := string(response.Data)
						sendSSEMessage(host, "message", string(body))
						assert.Equal(t, first, string(host.GetResponseBody()), "terminal SSE chunks must not append a second response")
						assert.Empty(t, host.GetHttpCalloutAttributes())
					})
				}
			}
		}
	}
}

func TestSSEEndpointSelectedAuthorityReceivesOnlyExistingCredentials(t *testing.T) {
	cases := []struct {
		name, extra, header, value, query string
		incoming                          [][2]string
	}{
		{"static-bearer", `,"securitySchemes":[{"id":"up","type":"http","scheme":"bearer","defaultCredential":"synthetic-static"}],"defaultUpstreamSecurity":{"id":"up"}`, "Authorization", "Bearer synthetic-static", "", [][2]string{{"Authorization", "Bearer synthetic-downstream"}}},
		{"forward-authorization", `,"passthroughAuthHeader":true`, "Authorization", "Bearer synthetic-forward", "", [][2]string{{"Authorization", "Bearer synthetic-forward"}}},
		{"removed-authorization", ``, "", "", "", [][2]string{{"Authorization", "Bearer synthetic-removed"}}},
		{"removed-proxy-authorization", ``, "", "", "", [][2]string{{"Proxy-Authorization", "synthetic-removed"}}},
		{"explicit-proxy-authorization", `,"securitySchemes":[{"id":"up","type":"apiKey","in":"header","name":"Proxy-Authorization","defaultCredential":"synthetic-static"}],"defaultUpstreamSecurity":{"id":"up"}`, "Proxy-Authorization", "synthetic-static", "", [][2]string{{"Proxy-Authorization", "synthetic-old"}, {"Proxy-Authorization", "synthetic-duplicate"}}},
		{"header-api-key", `,"securitySchemes":[{"id":"up","type":"apiKey","in":"header","name":"X-Upstream-Key","defaultCredential":"synthetic-static"}],"defaultUpstreamSecurity":{"id":"up"}`, "X-Upstream-Key", "synthetic-static", "", [][2]string{{"X-Upstream-Key", "synthetic-old"}, {"X-Upstream-Key", "synthetic-duplicate"}}},
		{"query-api-key", `,"securitySchemes":[{"id":"up","type":"apiKey","in":"query","name":"key","defaultCredential":"synthetic-query"}],"defaultUpstreamSecurity":{"id":"up"}`, "", "", "key=synthetic-query", nil},
		{"passthrough-credential", `,"securitySchemes":[{"id":"down","type":"apiKey","in":"header","name":"X-Client-Key"},{"id":"up","type":"http","scheme":"bearer"}],"defaultDownstreamSecurity":{"id":"down","passthrough":true},"defaultUpstreamSecurity":{"id":"up"}`, "Authorization", "Bearer synthetic-pass", "", [][2]string{{"X-Client-Key", "synthetic-pass"}}},
		{"passthrough-query", `,"securitySchemes":[{"id":"down","type":"apiKey","in":"header","name":"X-Client-Key"},{"id":"up","type":"apiKey","in":"query","name":"key"}],"defaultDownstreamSecurity":{"id":"down","passthrough":true},"defaultUpstreamSecurity":{"id":"up"}`, "", "", "key=synthetic-pass", [][2]string{{"X-Client-Key", "synthetic-pass"}}},
		{"removed-client-credential", `,"securitySchemes":[{"id":"down","type":"apiKey","in":"header","name":"X-Client-Key"},{"id":"up","type":"http","scheme":"bearer","defaultCredential":"synthetic-static"}],"defaultDownstreamSecurity":{"id":"down"},"defaultUpstreamSecurity":{"id":"up"}`, "Authorization", "Bearer synthetic-static", "", [][2]string{{"X-Client-Key", "synthetic-removed"}}},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			host := newSSECompatibilityHost(t, "http://backend.example/sse?initial=1", item.extra)
			startSSECompatibility(t, host, true, string(OpToolsCall), `null`, item.incoming...)
			getHeaders := host.GetRequestHeaders()
			initialize := openSSECompatibility(t, host, "https://other.example:8443/messages?session=one")
			notification, business := advanceSSECompatibility(t, host, initialize)
			for index, headers := range [][][2]string{getHeaders, initialize.Headers, notification.Headers, business.Headers} {
				if index == 0 {
					// The native SDK emulator coalesces AddHeaderMapValue calls.
					// Real GET header lines are verified by the runtime ledger.
					assert.Equal(t, "firstsecond", mustHeaderValue(t, headers, "X-Custom"))
				} else {
					assert.Equal(t, 2, countHeader(headers, "X-Custom"))
				}
				_, clientKey := findHeader(headers, "X-Client-Key")
				assert.False(t, clientKey, "removed downstream credentials must never be restored")
				if item.header != "Proxy-Authorization" {
					_, inherited := findHeader(headers, "Proxy-Authorization")
					assert.False(t, inherited)
				}
				if item.header != "" {
					assert.Equal(t, item.value, mustHeaderValue(t, headers, item.header))
					assert.Equal(t, 1, countHeader(headers, item.header), "explicit credentials replace inherited duplicates")
				} else {
					_, authorization := findHeader(headers, "Authorization")
					assert.False(t, authorization)
				}
				path := mustHeaderValue(t, headers, ":path")
				if item.query != "" {
					assert.Contains(t, path, item.query)
				}
				if index > 0 {
					assert.Equal(t, "other.example:8443", mustHeaderValue(t, headers, ":authority"))
					assert.Contains(t, path, "session=one")
				}
			}
			for _, request := range []proxytest.HttpCalloutAttribute{initialize, notification, business} {
				assert.Equal(t, sseOriginalCluster, request.Upstream)
			}
		})
	}
}

func TestSSEAuthenticationFailuresRetainStatusAndChallengeAtEveryStage(t *testing.T) {
	for _, stage := range []string{"get", "initialize", "initialized", "business"} {
		for _, status := range []string{"401", "403"} {
			for _, notification := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/notification=%t", stage, status, notification), func(t *testing.T) {
					host := newSSECompatibilityHost(t, "http://backend.example/sse", "")
					id := `1.50`
					if notification {
						id = ""
					}
					startSSECompatibility(t, host, true, string(OpToolsList), id)
					headers := [][2]string{{"WWW-Authenticate", "Bearer realm=upstream"}, {"Retry-After", "5"}}
					if stage == "get" {
						// The real header-only failure runs before SSE content-type/body processing.
						host.CallOnHttpResponseHeaders(append([][2]string{{":status", status}}, headers...), wasmtest.WithEndOfStream(true))
					} else {
						call := openSSECompatibility(t, host, "https://other.example/messages")
						if stage != "initialize" {
							completeCallout(host, call, "202", nil, nil)
							sendSSEMessage(host, "message", string(autoResult(`1`, `{}`)))
							call = calloutAt(t, host, 0)
						}
						if stage == "business" {
							completeCallout(host, call, "202", nil, nil)
							call = calloutAt(t, host, 0)
						}
						completeCallout(host, call, status, headers, []byte(`{"private":true}`))
					}
					response := proxyTestResponse(host)
					require.NotNil(t, response)
					code, _ := strconv.Atoi(status)
					assert.Equal(t, uint32(code), response.StatusCode)
					assert.Equal(t, "Bearer realm=upstream", mustHeaderValue(t, response.Headers, "WWW-Authenticate"))
					if notification {
						assert.Empty(t, response.Data)
					} else {
						assert.Equal(t, id, gjson.GetBytes(response.Data, "id").Raw)
						assert.Equal(t, "upstream authorization failed", gjson.GetBytes(response.Data, "error.message").String())
					}
					assert.NotContains(t, string(response.Data), "private")
					assert.Empty(t, host.GetHttpCalloutAttributes(), "terminal auth failures dispatch no subsequent request")
					first := string(response.Data)
					if stage != "get" {
						sendSSEMessage(host, "endpoint", "https://another.example/late")
						assert.Empty(t, host.GetHttpCalloutAttributes())
						assert.Equal(t, first, string(host.GetResponseBody()))
					}
				})
			}
		}
	}
}

func TestSSEControlCorrelationAndLateCallbacks(t *testing.T) {
	host := newSSECompatibilityHost(t, "http://backend.example/sse", "")
	startSSECompatibility(t, host, true, string(OpToolsCall), `-0`)
	initialize := openSSECompatibility(t, host, "/messages")
	completeCallout(host, initialize, "202", nil, nil)
	for _, id := range []string{`1.5`, `"1"`, `11`} {
		sendSSEMessage(host, "message", string(autoResult(id, `{}`)))
		assert.Empty(t, host.GetHttpCalloutAttributes(), "unassociated initialization messages do not advance controls")
	}
	sendSSEMessage(host, "message", string(autoResult(`1`, `{}`)))
	notification := calloutAt(t, host, 0)
	completeCallout(host, notification, "202", nil, nil)
	business := calloutAt(t, host, 0)
	completeCallout(host, business, "202", nil, nil)
	for _, id := range []string{`1`, `3.5`, `"3"`, `33`} {
		sendSSEMessage(host, "message", string(autoResult(id, `{"number":0}`)))
		assert.Empty(t, host.GetResponseBody(), "an unrelated business message is never emitted as the result")
	}
	sendSSEMessage(host, "message", string(autoResult(`3`, `{"number":900719925474099312345}`)))
	assert.Equal(t, `-0`, gjson.GetBytes(host.GetResponseBody(), "id").Raw)
	assert.Equal(t, `900719925474099312345`, gjson.GetBytes(host.GetResponseBody(), "result.number").Raw)
}

func TestSSEStreamDoneSuppressesEveryPendingPOSTCallback(t *testing.T) {
	for _, stage := range []string{"initialize", "initialized", "business"} {
		t.Run(stage, func(t *testing.T) {
			host := newSSECompatibilityHost(t, "http://backend.example/sse", "")
			startSSECompatibility(t, host, true, string(OpToolsList), `null`)
			call := openSSECompatibility(t, host, "/messages")
			if stage != "initialize" {
				completeCallout(host, call, "202", nil, nil)
				sendSSEMessage(host, "message", string(autoResult(`1`, `{}`)))
				call = calloutAt(t, host, 0)
			}
			if stage == "business" {
				completeCallout(host, call, "202", nil, nil)
				call = calloutAt(t, host, 0)
			}
			host.CompleteHttp()
			completeCallout(host, call, "401", nil, nil)
			assert.Nil(t, host.GetLocalResponse())
			assert.Empty(t, host.GetHttpCalloutAttributes())
		})
	}
}

func TestSSEPOSTStatusPolicyAndTerminalFailures(t *testing.T) {
	for _, stage := range []string{"initialize", "initialized", "business"} {
		for _, status := range []string{"400", "404", "429", "500", "503", "504"} {
			t.Run(stage+"/"+status, func(t *testing.T) {
				host := newSSECompatibilityHost(t, "http://backend.example/sse", "")
				startSSECompatibility(t, host, true, string(OpToolsCall), `1E+03`)
				call := openSSECompatibility(t, host, "/messages")
				if stage != "initialize" {
					completeCallout(host, call, "202", nil, nil)
					sendSSEMessage(host, "message", string(autoResult(`1`, `{}`)))
					call = calloutAt(t, host, 0)
				}
				if stage == "business" {
					completeCallout(host, call, "202", nil, nil)
					call = calloutAt(t, host, 0)
				}
				completeCallout(host, call, status, [][2]string{{"Retry-After", "5"}, {"Content-Type", "application/x-custom"}}, autoError(`1`, -32099, `{"discard":true}`))
				if stage == "initialized" {
					// The explicit Legacy SSE oracle continued after ordinary
					// notification failure; authentication remains terminal.
					business := calloutAt(t, host, 0)
					assert.Equal(t, string(OpToolsCall), gjson.GetBytes(business.Body, "method").String())
					return
				}
				response := proxyTestResponse(host)
				require.NotNil(t, response)
				assert.Equal(t, uint32(200), response.StatusCode)
				assert.Equal(t, `1E+03`, gjson.GetBytes(response.Data, "id").Raw)
				message := "SSE initialize failed with status " + status
				if stage == "business" {
					message = "SSE tool request failed with status " + status
				}
				assert.Equal(t, message, gjson.GetBytes(response.Data, "error.message").String())
				assert.False(t, gjson.GetBytes(response.Data, "error.data").Exists())
				_, retryAfter := findHeader(response.Headers, "Retry-After")
				assert.False(t, retryAfter)
				assert.Empty(t, host.GetHttpCalloutAttributes())
			})
		}
	}
}

func TestSSEPreparationAndCorrelationBoundaries(t *testing.T) {
	t.Run("malformed endpoint stops before any POST", func(t *testing.T) {
		host := newSSECompatibilityHost(t, "http://backend.example/sse", "")
		startSSECompatibility(t, host, true, string(OpToolsList), `null`)
		host.CallOnHttpResponseHeaders([][2]string{{":status", "200"}, {"content-type", "text/event-stream"}})
		sendSSEMessage(host, "endpoint", "http://%invalid/messages")
		assert.Empty(t, host.GetHttpCalloutAttributes())
		assert.Equal(t, `null`, gjson.GetBytes(host.GetResponseBody(), "id").Raw)
		assert.Equal(t, int64(utils.ErrInternalError), gjson.GetBytes(host.GetResponseBody(), "error.code").Int())
	})
	t.Run("empty cursor follows historical SSE carrier", func(t *testing.T) {
		host := newSSECompatibilityHost(t, "http://backend.example/sse", "")
		host.InitHttp()
		require.NoError(t, host.SetClusterName(sseOriginalCluster))
		headers, _ := looseProxyRequest(true, string(OpToolsList), `1`)
		body := []byte(strings.Replace(string(modernProxyListBody(1)), `"_meta":`, `"cursor":"","_meta":`, 1))
		require.Equal(t, types.ActionPause, host.CallOnHttpRequestHeaders(headers))
		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody(body))
		initialize := openSSECompatibility(t, host, "/messages")
		_, business := advanceSSECompatibility(t, host, initialize)
		assert.Equal(t, `""`, gjson.GetBytes(business.Body, "params.cursor").Raw)
	})
	t.Run("concatenated control messages remain invalid", func(t *testing.T) {
		host := newSSECompatibilityHost(t, "http://backend.example/sse", "")
		startSSECompatibility(t, host, true, string(OpToolsList), `1`)
		initialize := openSSECompatibility(t, host, "/messages")
		completeCallout(host, initialize, "202", nil, nil)
		sendSSEMessage(host, "message", string(autoResult(`1`, `{}`))+`{}`)
		assert.Empty(t, host.GetHttpCalloutAttributes())
	})
}

func TestSSEDuplicatePOSTCallbacksCannotRepeatBusinessOrResponse(t *testing.T) {
	original := dispatchPreparedMCP
	var callbacks []func(autoHTTPResponse)
	dispatchPreparedMCP = func(target proxyTarget, timeout uint32, headers [][2]string, body []byte, maxBody int, callback func(autoHTTPResponse)) error {
		callbacks = append(callbacks, callback)
		require.Equal(t, uint32(731), timeout)
		require.Zero(t, maxBody, "SSE responses do not inherit autoProbeBodyLimit")
		return original(target, timeout, headers, body, maxBody, callback)
	}
	t.Cleanup(func() { dispatchPreparedMCP = original })
	host := newSSECompatibilityHost(t, "http://backend.example/sse", "")
	startSSECompatibility(t, host, true, string(OpToolsList), `1`)
	initialize := openSSECompatibility(t, host, "/messages")
	notification, business := advanceSSECompatibility(t, host, initialize)
	require.Len(t, callbacks, 3)
	callbacks[1](autoHTTPResponse{status: 202})
	assert.Len(t, host.GetHttpCalloutAttributes(), 1, "the repeated initialized callback cannot send another tool RPC")
	assert.Equal(t, business.CalloutID, calloutAt(t, host, 0).CalloutID)
	completeCallout(host, business, "202", nil, nil)
	sendSSEMessage(host, "message", string(autoResult(`2`, `{"tools":[]}`)))
	first := string(host.GetResponseBody())
	for _, callback := range callbacks {
		callback(autoHTTPResponse{status: 401})
	}
	assert.Equal(t, first, string(host.GetResponseBody()))
	assert.Empty(t, host.GetHttpCalloutAttributes())
	assert.Equal(t, "notifications/initialized", gjson.GetBytes(notification.Body, "method").String())
}

func TestSSENotificationOrdinaryFailuresAcknowledgeWithoutRPCBody(t *testing.T) {
	for _, stage := range []string{"initialize", "business"} {
		t.Run(stage, func(t *testing.T) {
			host := newSSECompatibilityHost(t, "http://backend.example/sse", "")
			startSSECompatibility(t, host, true, string(OpToolsList), "")
			call := openSSECompatibility(t, host, "/messages")
			if stage == "business" {
				_, call = advanceSSECompatibility(t, host, call)
			}
			completeCallout(host, call, "500", nil, nil)
			response := proxyTestResponse(host)
			require.NotNil(t, response)
			assert.Equal(t, uint32(202), response.StatusCode)
			assert.Empty(t, response.Data)
			assert.Empty(t, host.GetHttpCalloutAttributes())
		})
	}
}

func TestSSEInitialGETAuthenticationResponseIsolation(t *testing.T) {
	challenges := []string{`Bearer realm="compat-primary"`, `Basic realm="compat-secondary"`}
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, err := writer.Write([]byte(`{"error":"synthetic-upstream-auth-body"}`))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	for _, profile := range []string{"legacy", "modern", "notification"} {
		for _, method := range []string{string(OpToolsList), string(OpToolsCall)} {
			for _, status := range []string{"401", "403"} {
				for _, headerOnly := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/%s/%s/headerOnly=%t", profile, method, status, headerOnly), func(t *testing.T) {
						host := newSSECompatibilityHost(t, "http://backend.example/sse", "")
						id := `"client\u002did"`
						if profile == "modern" {
							id = "null"
							if method == string(OpToolsCall) {
								id = "1.50"
							}
						}
						if profile == "notification" {
							id = ""
						}
						startSSECompatibility(t, host, profile != "legacy", method, id)
						writes := 0
						host.RegisterForeignFunction("inject_encoded_data_to_filter_chain_on_header", func(raw []byte) []byte {
							writes++
							args := &pb.InjectEncodedDataToFilterChainArguments{}
							require.NoError(t, proto.Unmarshal(raw, args))
							require.True(t, args.Endstream)
							require.NoError(t, proxywasm.InjectEncodedDataToFilterChain([]byte(args.Body), true))
							return []byte{0}
						})
						headers := [][2]string{{":status", status}, {"Content-Type", "application/json"}, {"Content-Encoding", "gzip"},
							{"Content-Length", strconv.Itoa(compressed.Len())}, {"Set-Cookie", "synthetic-cookie=private"},
							{"Mcp-Session-Id", "synthetic-session"}, {"X-Upstream-Private", "synthetic-private"}, {"Retry-After", "7"},
							{"WWW-Authenticate", challenges[0]}, {"WWW-Authenticate", challenges[1]}}
						if headerOnly {
							headers[3][1] = "0"
						}
						require.Equal(t, types.HeaderStopAllIterationAndWatermark, host.CallOnHttpResponseHeaders(headers, wasmtest.WithEndOfStream(headerOnly)))
						response := proxyTestResponse(host)
						require.NotNil(t, response)
						code, _ := strconv.Atoi(status)
						assert.Equal(t, uint32(code), response.StatusCode)
						for _, name := range []string{"Content-Encoding", "Set-Cookie", "Mcp-Session-Id", "X-Upstream-Private", "Retry-After", "Content-Length"} {
							_, present := findHeader(response.Headers, name)
							assert.False(t, present, name)
						}
						var retainedChallenges string
						for _, header := range response.Headers {
							if strings.EqualFold(header[0], "WWW-Authenticate") {
								retainedChallenges += header[1]
							}
						}
						// The native mock can coalesce Add values; the real Envoy
						// regression records raw fields and validates their semantics.
						for _, challenge := range challenges {
							assert.Equal(t, 1, strings.Count(retainedChallenges, challenge))
						}
						assert.Contains(t, mustHeaderValue(t, response.Headers, "Content-Type"), "application/json")
						if profile == "notification" {
							assert.Empty(t, response.Data)
						} else {
							require.True(t, json.Valid(response.Data))
							if profile == "legacy" {
								assert.Equal(t, "client-id", gjson.GetBytes(response.Data, "id").String())
							} else {
								assert.Equal(t, id, gjson.GetBytes(response.Data, "id").Raw)
							}
							assert.Equal(t, int64(-32603), gjson.GetBytes(response.Data, "error.code").Int())
							assert.Equal(t, "upstream authorization failed", gjson.GetBytes(response.Data, "error.message").String())
							assert.NotContains(t, string(response.Data), "synthetic-upstream-auth-body")
						}
						first := bytes.Clone(response.Data)
						host.CallOnHttpStreamingResponseBody(compressed.Bytes(), true)
						sendSSEMessage(host, "endpoint", "http://later.example/messages")
						assert.Empty(t, host.GetHttpCalloutAttributes(), "terminal GET authentication must not dispatch later phases")
						assert.Equal(t, first, host.GetResponseBody())
						assert.Equal(t, 1, writes, "late chunks must not write another response")
					})
				}
			}
		}
	}
}
