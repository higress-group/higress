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
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/protocol"
	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/utils"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/proxytest"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	pb "github.com/higress-group/wasm-go/pkg/protos"
	wasmtest "github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"google.golang.org/protobuf/proto"
)

func uniqueBusinessJSON(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	switch token {
	case json.Delim('{'):
		seen := map[string]bool{}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return errors.New("duplicate or invalid JSON key")
			}
			seen[name] = true
			if err := uniqueBusinessJSON(decoder); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
	case json.Delim('['):
		for decoder.More() {
			if err := uniqueBusinessJSON(decoder); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
	}
	return err
}

// A strict peer checks the actual routed request, including every nested key,
// gateway metadata, media headers and correlation ID, before returning a result.
func strictProxyOutbound(t *testing.T, request proxytest.HttpCalloutAttribute, method string, modern bool) map[string]json.RawMessage {
	t.Helper()
	require.True(t, json.Valid(request.Body))
	decoder := json.NewDecoder(bytes.NewReader(request.Body))
	decoder.UseNumber()
	require.NoError(t, uniqueBusinessJSON(decoder))
	require.Equal(t, "POST", mustHeaderValue(t, request.Headers, ":method"))
	require.Equal(t, "application/json", mustHeaderValue(t, request.Headers, "content-type"))
	require.Contains(t, mustHeaderValue(t, request.Headers, "accept"), "application/json")
	require.Contains(t, mustHeaderValue(t, request.Headers, "accept"), "text/event-stream")
	var envelope map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(request.Body, &envelope))
	require.Len(t, envelope, 4)
	require.Equal(t, `"2.0"`, string(envelope["jsonrpc"]))
	require.Equal(t, strconv.Quote(method), string(envelope["method"]))
	require.NotEmpty(t, envelope["id"])
	require.NotEqual(t, "null", string(envelope["id"]))
	require.NotEqual(t, "-0", string(envelope["id"]))
	var text string
	var integer int64
	require.True(t, json.Unmarshal(envelope["id"], &text) == nil || json.Unmarshal(envelope["id"], &integer) == nil,
		"a strict peer accepts only string/integer request IDs")
	if modern {
		require.Equal(t, string(protocol.Version20260728), mustHeaderValue(t, request.Headers, protocol.HeaderProtocolVersion))
		require.Equal(t, method, mustHeaderValue(t, request.Headers, protocol.HeaderMethod))
		require.Equal(t, string(protocol.Version20260728), gjson.GetBytes(request.Body, `params._meta.io\.modelcontextprotocol/protocolVersion`).String())
		require.Equal(t, "{}", gjson.GetBytes(request.Body, `params._meta.io\.modelcontextprotocol/clientCapabilities`).Raw)
		require.Equal(t, "Higress-mcp-proxy", gjson.GetBytes(request.Body, `params._meta.io\.modelcontextprotocol/clientInfo.name`).String())
		if method == string(OpToolsCall) {
			require.Equal(t, "echo", mustHeaderValue(t, request.Headers, protocol.HeaderName))
		} else {
			_, hasName := findHeader(request.Headers, protocol.HeaderName)
			require.False(t, hasName)
		}
	} else {
		_, hasMethod := findHeader(request.Headers, protocol.HeaderMethod)
		_, hasName := findHeader(request.Headers, protocol.HeaderName)
		require.False(t, hasMethod)
		require.False(t, hasName)
		require.False(t, gjson.GetBytes(request.Body, "params._meta").Exists())
	}
	return envelope
}

func looseProxyRequest(modern bool, method, token string) ([][2]string, []byte) {
	headers := legacyToolHeaders()
	id := ""
	if token != "" {
		id = `,"id":` + token + `,"id":999`
	}
	params := `"cursor":"first","cursor":"last"`
	if method == string(OpToolsCall) {
		params = `"name":"echo","arguments":{"number":900719925474099312345,"choice":1,"choice":2,"nested":{"value":"first","value":"last"}},"arguments":{"ignored":true}`
	}
	if modern {
		headers = modernProxyListHeaders()
		for index := range headers {
			if headers[index][0] == protocol.HeaderMethod {
				headers[index][1] = method
			}
		}
		if method == string(OpToolsCall) {
			headers = append(headers, [2]string{protocol.HeaderName, "echo"})
		}
		params += `,"ordinary":{"number":900719925474099312345,"choice":1,"choice":2,"nested":{"value":"first","value":"last"}},"ordinary":{"ignored":true},"_meta":{"` + protocol.MetaProtocolVersion + `":"2026-07-28","` + protocol.MetaClientCapabilities + `":false,"` + protocol.MetaClientInfo + `":7,"vendor":{"ignored":true},"vendor":{"number":900719925474099312345,"choice":1,"choice":2,"nested":{"value":"first","value":"last"}}},"_meta":{"ignored":true}`
	}
	for index := range headers {
		if headers[index][0] == ":method" {
			headers[index][1] = "PUT"
		}
		if headers[index][0] == "content-type" {
			headers[index][1] = "text/plain"
		}
	}
	headers = append(headers, [2]string{"X-Custom", "first"}, [2]string{"X-Custom", "second"})
	return headers, []byte(`{"jsonrpc":"2.0"` + id + `,"method":"` + method + `","params":{` + params + `},"params":{"ignored":true}}`)
}

func finishProxyControls(t *testing.T, host wasmtest.TestHost, strategy ProtocolStrategy, modernDownstream, modernUpstream bool) int {
	t.Helper()
	count := 0
	if strategy == ProtocolStrategyAuto && modernDownstream {
		probe := calloutAt(t, host, 0)
		count++
		if modernUpstream {
			completeAutoResult(host, probe, validAutoDiscoverResult)
			return count
		}
		completeCallout(host, probe, "404", nil, nil)
	}
	if !modernUpstream {
		initialize := calloutAt(t, host, 0)
		completeAutoResult(host, initialize, validAutoInitResult)
		notification := calloutAt(t, host, 0)
		completeCallout(host, notification, "202", nil, nil)
		count += 2
	}
	return count
}

func TestProxyLooseClientIDsReachStrictUpstreamsAndRestoreClientSemantics(t *testing.T) {
	modes := []struct {
		name           string
		strategy       ProtocolStrategy
		modernUpstream bool
	}{
		{"explicit-modern", ProtocolStrategyModern, true}, {"explicit-legacy", ProtocolStrategyLegacy, false},
		{"auto-modern", ProtocolStrategyAuto, true}, {"auto-legacy", ProtocolStrategyAuto, false},
	}
	ids := []string{`"client\u002did"`, `9007199254740993`, `9223372036854775808`, `1.50`, `1E+03`, `-0`, `null`, ``}
	for _, mode := range modes {
		for _, modernDownstream := range []bool{false, true} {
			if !modernDownstream && mode.modernUpstream {
				continue
			}
			for idIndex, token := range ids {
				for _, operation := range []string{string(OpToolsList), string(OpToolsCall)} {
					for _, rpcError := range []bool{false, true} {
						t.Run(fmt.Sprintf("%s/downstream=%t/id=%d/%s/error=%t", mode.name, modernDownstream, idIndex, operation, rpcError), func(t *testing.T) {
							var host wasmtest.TestHost
							if mode.strategy == ProtocolStrategyAuto {
								host = newAutoTestHost(t, "")
							} else {
								host = newProxyBridgeHost(t, mode.strategy)
							}
							headers, body := looseProxyRequest(modernDownstream, operation, token)
							require.Equal(t, types.ActionPause, host.CallOnHttpRequestHeaders(headers))
							host.CallOnHttpRequestBody(body)
							controls := finishProxyControls(t, host, mode.strategy, modernDownstream, mode.modernUpstream)
							wantControls := 0
							if !mode.modernUpstream {
								wantControls = 2
							}
							if mode.strategy == ProtocolStrategyAuto && modernDownstream {
								wantControls++
							}
							require.Equal(t, wantControls, controls)
							business := routedRequest(t, host)
							envelope := strictProxyOutbound(t, business, operation, mode.modernUpstream)
							assert.Equal(t, 2, countHeader(business.Headers, "X-Custom"))
							assert.False(t, gjson.GetBytes(business.Body, "params.ignored").Exists())
							if mode.modernUpstream {
								for _, path := range []string{"params.ordinary", "params._meta.vendor"} {
									assert.Equal(t, "900719925474099312345", gjson.GetBytes(business.Body, path+".number").Raw)
									assert.Equal(t, "2", gjson.GetBytes(business.Body, path+".choice").Raw)
									assert.Equal(t, "last", gjson.GetBytes(business.Body, path+".nested.value").String())
									assert.False(t, gjson.GetBytes(business.Body, path+".ignored").Exists())
								}
							}
							if operation == string(OpToolsCall) {
								assert.Equal(t, "900719925474099312345", gjson.GetBytes(business.Body, "params.arguments.number").Raw)
								assert.Equal(t, "2", gjson.GetBytes(business.Body, "params.arguments.choice").Raw)
								assert.Equal(t, "last", gjson.GetBytes(business.Body, "params.arguments.nested.value").String())
								assert.False(t, gjson.GetBytes(business.Body, "params.arguments.ignored").Exists())
							} else {
								assert.Equal(t, "first", gjson.GetBytes(business.Body, "params.cursor").String())
							}
							if modernDownstream && token == "" {
								assert.Nil(t, host.GetLocalResponse(), "notification acknowledgement must wait for the routed business response")
							}
							responseBody := autoResult(string(envelope["id"]), `{"tools":[],"content":[],"number":900719925474099312345,"choice":1,"choice":2}`)
							if rpcError {
								responseBody = autoError(string(envelope["id"]), -32099, `{"discard":true}`)
							}
							completeCallout(host, business, "200", nil, responseBody)
							response := proxyTestResponse(host)
							require.NotNil(t, response)
							if modernDownstream && token == "" {
								assert.Equal(t, uint32(202), response.StatusCode)
								assert.Empty(t, response.Data)
								return
							}
							wantID := token
							if wantID == "" {
								wantID = "0"
							}
							assert.Equal(t, wantID, gjson.GetBytes(response.Data, "id").Raw)
							assert.Equal(t, uint32(200), response.StatusCode)
							if rpcError {
								assert.Equal(t, int64(utils.ErrInternalError), gjson.GetBytes(response.Data, "error.code").Int())
								assert.False(t, gjson.GetBytes(response.Data, "error.data").Exists())
							} else {
								assert.Equal(t, "900719925474099312345", gjson.GetBytes(response.Data, "result.number").Raw)
								assert.Equal(t, "2", gjson.GetBytes(response.Data, "result.choice").Raw)
							}
							assert.Empty(t, host.GetHttpCalloutAttributes())
						})
					}
				}
			}
		}
	}
}

func TestHTTPBusinessResponsePolicyDoesNotInheritControlRestrictions(t *testing.T) {
	deep := strings.Repeat("[", 70) + "0" + strings.Repeat("]", 70)
	for _, strategy := range []ProtocolStrategy{ProtocolStrategyLegacy, ProtocolStrategyModern, ProtocolStrategyAuto} {
		for _, operation := range []string{string(OpToolsList), string(OpToolsCall)} {
			for _, status := range []string{"200", "400", "404", "429", "500", "503", "504", "401", "403"} {
				t.Run(fmt.Sprintf("%s/%s/%s", strategy, operation, status), func(t *testing.T) {
					var host wasmtest.TestHost
					if strategy == ProtocolStrategyAuto {
						host = newAutoTestHost(t, "")
					} else {
						host = newProxyBridgeHost(t, strategy)
					}
					headers, body := looseProxyRequest(true, operation, "1.50")
					require.Equal(t, types.ActionPause, host.CallOnHttpRequestHeaders(headers))
					host.CallOnHttpRequestBody(body)
					finishProxyControls(t, host, strategy, true, strategy != ProtocolStrategyLegacy)
					business := routedRequest(t, host)
					responseBody := autoResult(gjson.GetBytes(business.Body, "id").Raw, `{"tools":[],"content":[],"deep":`+deep+`,"padding":"`+strings.Repeat("x", 1024*1024+1)+`","number":900719925474099312345,"value":1,"value":2}`)
					responseHeaders := [][2]string{{"Content-Type", "application/x-custom"}, {"Retry-After", "5"}, {"WWW-Authenticate", "Bearer realm=upstream"}}
					completeCallout(host, business, status, responseHeaders, responseBody)
					response := proxyTestResponse(host)
					require.NotNil(t, response)
					assert.Equal(t, "1.50", gjson.GetBytes(response.Data, "id").Raw)
					wantStatus := uint32(200)
					if status == "401" || status == "403" {
						value, _ := strconv.Atoi(status)
						wantStatus = uint32(value)
						assert.Equal(t, "Bearer realm=upstream", mustHeaderValue(t, response.Headers, "WWW-Authenticate"))
					}
					assert.Equal(t, wantStatus, response.StatusCode)
					_, retryAfter := findHeader(response.Headers, "Retry-After")
					assert.False(t, retryAfter)
					if status == "200" {
						assert.Equal(t, "900719925474099312345", gjson.GetBytes(response.Data, "result.number").Raw)
						assert.Equal(t, "2", gjson.GetBytes(response.Data, "result.value").Raw)
						assert.Equal(t, 1024*1024+1, len(gjson.GetBytes(response.Data, "result.padding").String()))
					} else {
						assert.Equal(t, int64(utils.ErrInternalError), gjson.GetBytes(response.Data, "error.code").Int())
					}
				})
			}
		}
	}
}

func TestExplicitDiscoveryUsesStrictOutboundAndOriginalClientIDs(t *testing.T) {
	for index, token := range []string{`1.50`, `1E+03`, `-0`, `9223372036854775808`, `null`, ``} {
		for _, rpcError := range []bool{false, true} {
			t.Run(fmt.Sprintf("id=%d/error=%t", index, rpcError), func(t *testing.T) {
				host := newInstructionsHost(t, `,"passthroughInstructions":true`, "")
				headers, body := looseProxyRequest(true, "server/discover", token)
				require.Equal(t, types.ActionPause, host.CallOnHttpRequestHeaders(headers))
				require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody(body))
				business := routedRequest(t, host)
				envelope := strictProxyOutbound(t, business, "server/discover", true)
				responseBody := autoResult(string(envelope["id"]), `{"resultType":"complete","instructions":"instructions","capabilities":{"unimplemented":{}},"ttlMs":99}`)
				if rpcError {
					responseBody = autoError(string(envelope["id"]), -32099, "")
				}
				completeCallout(host, business, "200", nil, responseBody)
				response := proxyTestResponse(host)
				require.NotNil(t, response)
				if token == "" {
					assert.Equal(t, uint32(202), response.StatusCode)
					assert.Empty(t, response.Data)
					return
				}
				assert.Equal(t, token, gjson.GetBytes(response.Data, "id").Raw)
				if !rpcError {
					assert.Equal(t, "instructions", gjson.GetBytes(response.Data, "result.instructions").String())
					assert.False(t, gjson.GetBytes(response.Data, "result.capabilities.unimplemented").Exists())
				}
			})
		}
	}
}

func TestBusinessAndControlJSONCodecsRemainSeparate(t *testing.T) {
	for _, body := range [][]byte{
		autoResult(`"control"`, `{"tools":[],"value":1,"value":2}`),
		autoResult(`"control"`, `{"deep":`+strings.Repeat("[", 70)+`0`+strings.Repeat("]", 70)+`}`),
	} {
		_, err := decodeBusinessJSONObject(body)
		require.NoError(t, err)
		_, err = decodeAutoResponse(jsonAutoResponse(200, body), []byte(`"control"`))
		require.Error(t, err)
	}
	_, err := decodeBusinessJSONObject([]byte(`{"result":{}}{}`))
	require.Error(t, err, "the historical Unmarshal path rejected concatenated business responses")
}

func TestProxyNotificationsSuppressBusinessAuthAndControlErrors(t *testing.T) {
	for _, strategy := range []ProtocolStrategy{ProtocolStrategyModern, ProtocolStrategyAuto} {
		for _, controlFailure := range []bool{false, true} {
			if strategy != ProtocolStrategyAuto && controlFailure {
				continue
			}
			t.Run(fmt.Sprintf("%s/control=%t", strategy, controlFailure), func(t *testing.T) {
				var host wasmtest.TestHost
				if strategy == ProtocolStrategyAuto {
					host = newAutoTestHost(t, "")
				} else {
					host = newProxyBridgeHost(t, strategy)
				}
				headers, body := looseProxyRequest(true, string(OpToolsList), "")
				require.Equal(t, types.ActionPause, host.CallOnHttpRequestHeaders(headers))
				host.CallOnHttpRequestBody(body)
				if controlFailure {
					probe := calloutAt(t, host, 0)
					completeCallout(host, probe, "401", [][2]string{{"WWW-Authenticate", "Bearer realm=upstream"}}, nil)
				} else {
					finishProxyControls(t, host, strategy, true, true)
					business := routedRequest(t, host)
					strictProxyOutbound(t, business, string(OpToolsList), true)
					assert.Nil(t, host.GetLocalResponse(), "routed notifications must not close the main request before its upstream response")
					host.RegisterForeignFunction("inject_encoded_data_to_filter_chain_on_header", func(raw []byte) []byte {
						args := &pb.InjectEncodedDataToFilterChainArguments{}
						require.NoError(t, proto.Unmarshal(raw, args))
						require.True(t, args.Endstream)
						require.Empty(t, args.Body)
						return []byte{0}
					})
					host.CallOnHttpResponseHeaders([][2]string{{":status", "401"}, {"WWW-Authenticate", "Bearer realm=upstream"}}, wasmtest.WithEndOfStream(true))
				}
				response := proxyTestResponse(host)
				require.NotNil(t, response)
				assert.Equal(t, uint32(401), response.StatusCode)
				assert.Equal(t, "Bearer realm=upstream", mustHeaderValue(t, response.Headers, "WWW-Authenticate"))
				assert.Empty(t, response.Data)
				assert.Empty(t, host.GetHttpCalloutAttributes())
			})
		}
	}
}

func TestProxyAuthenticationCannotOverrideSelectedProtocolIdentity(t *testing.T) {
	for _, strategy := range []ProtocolStrategy{ProtocolStrategyModern, ProtocolStrategyAuto} {
		for _, name := range []string{"Content-Type", "Mcp-Method"} {
			t.Run(string(strategy)+"/"+name, func(t *testing.T) {
				saved := globalContext
				globalContext = Context{servers: make(map[string]Server)}
				Initialize()
				t.Cleanup(func() { globalContext = saved })
				config := json.RawMessage(fmt.Sprintf(`{"server":{"name":"proxy","type":"mcp-proxy","transport":"http","protocolStrategy":%q,"mcpServerURL":"http://backend.example/mcp","securitySchemes":[{"id":"reserved","type":"apiKey","in":"header","name":%q,"defaultCredential":"configured-value"}],"defaultUpstreamSecurity":{"id":"reserved"}}}`, strategy, name))
				host := newValidationHost(t, config)
				headers, body := looseProxyRequest(true, string(OpToolsList), "1.50")
				require.Equal(t, types.ActionPause, host.CallOnHttpRequestHeaders(headers))
				host.CallOnHttpRequestBody(body)
				response := host.GetLocalResponse()
				require.NotNil(t, response)
				assert.Equal(t, "1.50", gjson.GetBytes(response.Data, "id").Raw)
				assert.Empty(t, host.GetHttpCalloutAttributes())
				assert.Contains(t, host.GetRequestHeaders(), [2]string{":path", "/mcp"})
			})
		}
	}
}

func TestPreparedLegacyBusinessDefaults(t *testing.T) {
	prepared := &PreparedProxyRequest{id: json.RawMessage(`null`), params: map[string]json.RawMessage{
		"name": json.RawMessage(`"echo"`), "arguments": json.RawMessage(`null`),
	}}
	body, err := prepared.body(OutboundOperation{method: string(OpToolsCall), version: protocol.Version20250326})
	require.NoError(t, err)
	assert.Equal(t, "3", gjson.GetBytes(body, "id").Raw)
	assert.Equal(t, "null", gjson.GetBytes(body, "params.arguments").Raw)
	prepared.params = map[string]json.RawMessage{}
	body, err = prepared.body(OutboundOperation{method: string(OpToolsList), version: protocol.Version20250326})
	require.NoError(t, err)
	assert.Equal(t, "2", gjson.GetBytes(body, "id").Raw)
	assert.Equal(t, "{}", gjson.GetBytes(body, "params").Raw)
}

func TestNotificationAuthenticationExceptionAcrossHTTPStages(t *testing.T) {
	for _, stage := range []string{"business", "discovery", "auto-discover", "auto-initialize", "auto-initialized"} {
		for _, status := range []string{"401", "403"} {
			t.Run(stage+"/"+status, func(t *testing.T) {
				var host wasmtest.TestHost
				method := string(OpToolsList)
				switch stage {
				case "business":
					host = newProxyBridgeHost(t, ProtocolStrategyModern)
				case "discovery":
					host = newInstructionsHost(t, `,"passthroughInstructions":true`, "")
					method = "server/discover"
				default:
					host = newAutoTestHost(t, "")
				}
				headers, body := looseProxyRequest(true, method, "")
				require.Equal(t, types.ActionPause, host.CallOnHttpRequestHeaders(headers))
				host.CallOnHttpRequestBody(body)
				responseHeaders := [][2]string{{"WWW-Authenticate", "Bearer realm=upstream"}}
				if strings.HasPrefix(stage, "auto-") {
					call := calloutAt(t, host, 0)
					if stage != "auto-discover" {
						completeCallout(host, call, "404", nil, nil)
						call = calloutAt(t, host, 0)
					}
					if stage == "auto-initialized" {
						completeAutoResult(host, call, validAutoInitResult)
						call = calloutAt(t, host, 0)
					}
					completeCallout(host, call, status, responseHeaders, nil)
				} else {
					routedRequest(t, host)
					host.RegisterForeignFunction("inject_encoded_data_to_filter_chain_on_header", func(raw []byte) []byte {
						args := &pb.InjectEncodedDataToFilterChainArguments{}
						require.NoError(t, proto.Unmarshal(raw, args))
						require.True(t, args.Endstream)
						require.Empty(t, args.Body)
						return []byte{0}
					})
					host.CallOnHttpResponseHeaders(append([][2]string{{":status", status}}, responseHeaders...), wasmtest.WithEndOfStream(true))
				}
				response := proxyTestResponse(host)
				require.NotNil(t, response)
				code, _ := strconv.Atoi(status)
				assert.Equal(t, uint32(code), response.StatusCode)
				assert.Equal(t, "Bearer realm=upstream", mustHeaderValue(t, response.Headers, "WWW-Authenticate"))
				assert.Empty(t, response.Data)
				assert.Empty(t, host.GetHttpCalloutAttributes(), "authentication failures never advance to another control/business call")
			})
		}
	}
}
