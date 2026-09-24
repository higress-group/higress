// Copyright (c) 2022 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/consts"
	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/protocol"
	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/utils"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	wasmtest "github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"gopkg.in/yaml.v3"
)

type validationTestServer struct {
	base BaseMCPServer
}

func newValidationTestServer() *validationTestServer {
	return &validationTestServer{base: NewBaseMCPServer()}
}

func (s *validationTestServer) AddMCPTool(name string, tool Tool) Server {
	s.base.AddMCPTool(name, tool)
	return s
}

func (s *validationTestServer) GetMCPTools() map[string]Tool { return s.base.GetMCPTools() }
func (s *validationTestServer) SetConfig(config []byte)      { s.base.SetConfig(config) }
func (s *validationTestServer) GetConfig(value any)          { s.base.GetConfig(value) }
func (s *validationTestServer) Clone() Server {
	return &validationTestServer{base: s.base.CloneBase()}
}

type validationToolCounters struct {
	create int
	call   int
}

type validationTestTool struct {
	counters    *validationToolCounters
	schema      map[string]any
	callErr     error
	schemaReads int
}

type validationOutputTool struct {
	*validationTestTool
}

type changingDescriptorTool struct {
	descriptionReads int
	schemaReads      int
	firstSchema      map[string]any
	secondSchema     map[string]any
}

func (t *changingDescriptorTool) Create(_ []byte) Tool               { return t }
func (t *changingDescriptorTool) Call(_ HttpContext, _ Server) error { return nil }
func (t *changingDescriptorTool) Description() string {
	t.descriptionReads++
	if t.descriptionReads == 1 {
		return "captured description"
	}
	return "drifted description"
}
func (t *changingDescriptorTool) InputSchema() map[string]any {
	t.schemaReads++
	if t.schemaReads == 1 {
		return t.firstSchema
	}
	return t.secondSchema
}

func (t *validationOutputTool) OutputSchema() map[string]any {
	return map[string]any{"type": "object"}
}

func (t *validationTestTool) Create(_ []byte) Tool {
	t.counters.create++
	return &validationTestTool{counters: t.counters, schema: t.schema, callErr: t.callErr}
}

func (t *validationTestTool) Call(ctx HttpContext, _ Server) error {
	t.counters.call++
	if t.callErr != nil {
		return t.callErr
	}
	utils.SendMCPToolTextResult(ctx, "called")
	return nil
}

func (t *validationTestTool) Description() string { return "validation test tool" }
func (t *validationTestTool) InputSchema() map[string]any {
	t.schemaReads++
	return t.schema
}

func modernToolHeaders(name string) [][2]string {
	return [][2]string{
		{":authority", "mcp.example.com"},
		{":method", "POST"},
		{":path", "/mcp"},
		{"content-type", "application/json"},
		{"accept", "application/json, text/event-stream"},
		{protocol.HeaderProtocolVersion, string(protocol.Version20260728)},
		{protocol.HeaderMethod, "tools/call"},
		{protocol.HeaderName, name},
	}
}

func modernToolListHeaders() [][2]string {
	return [][2]string{
		{":authority", "mcp.example.com"},
		{":method", "POST"},
		{":path", "/mcp"},
		{"content-type", "application/json"},
		{"accept", "application/json, text/event-stream"},
		{protocol.HeaderProtocolVersion, string(protocol.Version20260728)},
		{protocol.HeaderMethod, "tools/list"},
	}
}

func legacyToolHeaders() [][2]string {
	return [][2]string{
		{":authority", "mcp.example.com"},
		{":method", "POST"},
		{":path", "/mcp"},
		{"content-type", "application/json"},
		{"accept", "application/json, text/event-stream"},
	}
}

func modernToolCallBody(name, arguments string) []byte {
	return []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + name + `","arguments":` + arguments + `,"_meta":{"` +
		protocol.MetaProtocolVersion + `":"2026-07-28","` + protocol.MetaClientCapabilities + `":{}}}}`)
}

func newValidationHost(t *testing.T, config json.RawMessage) wasmtest.TestHost {
	t.Helper()
	host, status := wasmtest.NewTestHost(config)
	require.Equal(t, types.OnPluginStartStatusOK, status)
	t.Cleanup(host.Reset)
	return host
}

func installRegisteredValidationServer(t *testing.T, counters *validationToolCounters) {
	t.Helper()
	savedGlobalContext := globalContext
	globalContext = Context{servers: make(map[string]Server)}
	server := newValidationTestServer()
	server.AddMCPTool("count", &validationTestTool{
		counters: counters,
		schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"count": map[string]any{"type": "integer"},
			},
			"required":             []any{"count"},
			"additionalProperties": false,
		},
	})
	Load(AddMCPServer("registered", server))
	Initialize()
	t.Cleanup(func() { globalContext = savedGlobalContext })
}

func TestModernRegisteredToolValidatesBeforeCreateAndCall(t *testing.T) {
	counters := &validationToolCounters{}
	installRegisteredValidationServer(t, counters)
	host := newValidationHost(t, json.RawMessage(`{"server":{"name":"registered"}}`))

	require.Equal(t, types.ActionPause, host.CallOnHttpRequestHeaders(modernToolHeaders("count")))
	require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody(modernToolCallBody("count", `{"count":"wrong","token":"secret-value"}`)))
	response := host.GetLocalResponse()
	require.NotNil(t, response)
	assert.Equal(t, uint32(200), response.StatusCode)
	assert.False(t, gjson.GetBytes(response.Data, "error").Exists())
	assert.True(t, gjson.GetBytes(response.Data, "result.isError").Bool())
	assert.Equal(t, resultTypeComplete, gjson.GetBytes(response.Data, "result.resultType").String())
	assert.Equal(t, "registered", gjson.GetBytes(response.Data, "result._meta.io\\.modelcontextprotocol/serverInfo.name").String())
	assert.NotContains(t, string(response.Data), "secret-value")
	assert.Zero(t, counters.create)
	assert.Zero(t, counters.call)
}

func TestModernRegisteredToolInvokesAfterValidation(t *testing.T) {
	counters := &validationToolCounters{}
	installRegisteredValidationServer(t, counters)
	host := newValidationHost(t, json.RawMessage(`{"server":{"name":"registered"}}`))

	require.Equal(t, types.ActionPause, host.CallOnHttpRequestHeaders(modernToolHeaders("count")))
	require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody(modernToolCallBody("count", `{"count":2}`)))
	response := host.GetLocalResponse()
	require.NotNil(t, response)
	assert.False(t, gjson.GetBytes(response.Data, "result.isError").Bool())
	assert.Equal(t, resultTypeComplete, gjson.GetBytes(response.Data, "result.resultType").String())
	assert.Equal(t, "registered", gjson.GetBytes(response.Data, "result._meta.io\\.modelcontextprotocol/serverInfo.name").String())
	assert.Equal(t, 1, counters.create)
	assert.Equal(t, 1, counters.call)
}

func TestModernDegradedToolListsButReturnsExactServerErrorWithoutInvocation(t *testing.T) {
	savedGlobalContext := globalContext
	globalContext = Context{servers: make(map[string]Server)}
	counters := &validationToolCounters{}
	registered := newValidationTestServer()
	reproduced := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"businessType": map[string]any{"type": "array", "enum": []any{"A", "B"}},
		},
	}
	registered.AddMCPTool("degraded", &validationTestTool{counters: counters, schema: reproduced})
	registered.AddMCPTool("valid", &validationTestTool{counters: counters, schema: map[string]any{"type": "object"}})
	Load(AddMCPServer("registered", registered))
	Initialize()
	t.Cleanup(func() { globalContext = savedGlobalContext })

	t.Run("list preserves descriptor", func(t *testing.T) {
		listHost := newValidationHost(t, json.RawMessage(`{"server":{"name":"registered"}}`))
		require.Equal(t, types.ActionPause, listHost.CallOnHttpRequestHeaders(modernToolListHeaders()))
		listBody := []byte(`{"jsonrpc":"2.0","id":"list-1","method":"tools/list","params":{"_meta":{"` +
			protocol.MetaProtocolVersion + `":"2026-07-28","` + protocol.MetaClientCapabilities + `":{}}}}`)
		require.Equal(t, types.ActionContinue, listHost.CallOnHttpRequestBody(listBody))
		listResponse := listHost.GetLocalResponse()
		require.NotNil(t, listResponse)
		assert.Equal(t, "degraded", gjson.GetBytes(listResponse.Data, "result.tools.0.name").String())
		assert.Equal(t, "array", gjson.GetBytes(listResponse.Data, "result.tools.0.inputSchema.properties.businessType.type").String())
		assert.Equal(t, "A", gjson.GetBytes(listResponse.Data, "result.tools.0.inputSchema.properties.businessType.enum.0").String())
		assert.Equal(t, "valid", gjson.GetBytes(listResponse.Data, "result.tools.1.name").String())
	})

	t.Run("call is blocked", func(t *testing.T) {
		callHost := newValidationHost(t, json.RawMessage(`{"server":{"name":"registered"}}`))
		require.Equal(t, types.ActionPause, callHost.CallOnHttpRequestHeaders(modernToolHeaders("degraded")))
		require.Equal(t, types.ActionContinue, callHost.CallOnHttpRequestBody(modernToolCallBody("degraded", `{"businessType":["A"]}`)))
		response := callHost.GetLocalResponse()
		require.NotNil(t, response)
		assert.Equal(t, uint32(200), response.StatusCode)
		assert.Equal(t, int64(protocol.CodeInternalError), gjson.GetBytes(response.Data, "error.code").Int())
		assert.Equal(t, "tool input schema validation is unavailable", gjson.GetBytes(response.Data, "error.message").String())
		assert.Equal(t, "schema_validation_unavailable", gjson.GetBytes(response.Data, "error.data.reason").String())
		assert.Equal(t, int64(1), gjson.GetBytes(response.Data, "id").Int())
		assert.False(t, gjson.GetBytes(response.Data, "result").Exists())
		assert.Empty(t, callHost.GetHttpCalloutAttributes())
	})
	assert.Zero(t, counters.create)
	assert.Zero(t, counters.call)
}

func TestFormerDescriptorFatalToolIsBlockedOnlyForModernCalls(t *testing.T) {
	savedGlobalContext := globalContext
	globalContext = Context{servers: make(map[string]Server)}
	counters := &validationToolCounters{}
	deep := map[string]any{"type": "string"}
	for i := 0; i < maxSchemaDepth+2; i++ {
		deep = map[string]any{"type": "array", "items": deep}
	}
	registered := newValidationTestServer()
	registered.AddMCPTool("deep", &validationTestTool{counters: counters, schema: map[string]any{
		"type":       "object",
		"properties": map[string]any{"value": deep},
	}})
	Load(AddMCPServer("registered", registered))
	Initialize()
	t.Cleanup(func() { globalContext = savedGlobalContext })

	t.Run("modern blocked", func(t *testing.T) {
		modern := newValidationHost(t, json.RawMessage(`{"server":{"name":"registered"}}`))
		require.Equal(t, types.ActionPause, modern.CallOnHttpRequestHeaders(modernToolHeaders("deep")))
		require.Equal(t, types.ActionContinue, modern.CallOnHttpRequestBody(modernToolCallBody("deep", `{}`)))
		response := modern.GetLocalResponse()
		require.NotNil(t, response)
		assert.Equal(t, int64(protocol.CodeInternalError), gjson.GetBytes(response.Data, "error.code").Int())
		assert.Equal(t, "schema_validation_unavailable", gjson.GetBytes(response.Data, "error.data.reason").String())
		assert.Zero(t, counters.create)
		assert.Zero(t, counters.call)
	})

	t.Run("legacy bypass", func(t *testing.T) {
		legacy := newValidationHost(t, json.RawMessage(`{"server":{"name":"registered"}}`))
		require.Equal(t, types.ActionPause, legacy.CallOnHttpRequestHeaders(legacyToolHeaders()))
		require.Equal(t, types.ActionContinue, legacy.CallOnHttpRequestBody([]byte(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"deep","arguments":{"legacy":true}}}`)))
		legacyResponse := legacy.GetLocalResponse()
		require.NotNil(t, legacyResponse)
		assert.False(t, gjson.GetBytes(legacyResponse.Data, "result.isError").Bool())
	})
	assert.Equal(t, 1, counters.create)
	assert.Equal(t, 1, counters.call)
}

func TestNonSerializableRegisteredDescriptorIsRequestLocal(t *testing.T) {
	savedGlobalContext := globalContext
	globalContext = Context{servers: make(map[string]Server)}
	badCounters := &validationToolCounters{}
	validCounters := &validationToolCounters{}
	registered := newValidationTestServer()
	registered.AddMCPTool("bad", &validationTestTool{counters: badCounters, schema: map[string]any{
		"type":     "object",
		"callback": func() {},
	}})
	registered.AddMCPTool("valid", &validationTestTool{counters: validCounters, schema: map[string]any{"type": "object"}})
	Load(AddMCPServer("registered", registered))
	Initialize()
	t.Cleanup(func() { globalContext = savedGlobalContext })

	t.Run("list keeps serializable siblings", func(t *testing.T) {
		listHost := newValidationHost(t, json.RawMessage(`{"server":{"name":"registered"}}`))
		require.Equal(t, types.ActionPause, listHost.CallOnHttpRequestHeaders(modernToolListHeaders()))
		listBody := []byte(`{"jsonrpc":"2.0","id":"list","method":"tools/list","params":{"_meta":{"` +
			protocol.MetaProtocolVersion + `":"2026-07-28","` + protocol.MetaClientCapabilities + `":{}}}}`)
		require.Equal(t, types.ActionContinue, listHost.CallOnHttpRequestBody(listBody))
		listResponse := listHost.GetLocalResponse()
		require.NotNil(t, listResponse)
		assert.Equal(t, "valid", gjson.GetBytes(listResponse.Data, "result.tools.0.name").String())
		assert.Equal(t, int64(1), gjson.GetBytes(listResponse.Data, "result.tools.#").Int())
	})

	t.Run("call is unavailable", func(t *testing.T) {
		callHost := newValidationHost(t, json.RawMessage(`{"server":{"name":"registered"}}`))
		require.Equal(t, types.ActionPause, callHost.CallOnHttpRequestHeaders(modernToolHeaders("bad")))
		require.Equal(t, types.ActionContinue, callHost.CallOnHttpRequestBody(modernToolCallBody("bad", `{}`)))
		callResponse := callHost.GetLocalResponse()
		require.NotNil(t, callResponse)
		assert.Equal(t, "schema_validation_unavailable", gjson.GetBytes(callResponse.Data, "error.data.reason").String())
	})
	assert.Zero(t, badCounters.create)
	assert.Zero(t, badCounters.call)
}

func TestLegacyDegradedToolRetainsInvocationBehavior(t *testing.T) {
	savedGlobalContext := globalContext
	globalContext = Context{servers: make(map[string]Server)}
	counters := &validationToolCounters{}
	registered := newValidationTestServer()
	registered.AddMCPTool("degraded", &validationTestTool{counters: counters, schema: map[string]any{
		"type":  "object",
		"oneOf": []any{map[string]any{"type": "object"}},
	}})
	Load(AddMCPServer("registered", registered))
	Initialize()
	t.Cleanup(func() { globalContext = savedGlobalContext })
	host := newValidationHost(t, json.RawMessage(`{"server":{"name":"registered"}}`))

	require.Equal(t, types.ActionPause, host.CallOnHttpRequestHeaders(legacyToolHeaders()))
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"degraded","arguments":{"legacy":true}}}`)
	require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody(body))
	response := host.GetLocalResponse()
	require.NotNil(t, response)
	assert.False(t, gjson.GetBytes(response.Data, "result.isError").Bool())
	assert.Equal(t, 1, counters.create)
	assert.Equal(t, 1, counters.call)
}

func TestModernDegradedToolDoesNotBypassAllowToolsPrecedence(t *testing.T) {
	savedGlobalContext := globalContext
	globalContext = Context{servers: make(map[string]Server)}
	registered := newValidationTestServer()
	registered.AddMCPTool("degraded", &validationTestTool{counters: &validationToolCounters{}, schema: map[string]any{
		"type":  "object",
		"oneOf": []any{map[string]any{"type": "object"}},
	}})
	registered.AddMCPTool("valid", &validationTestTool{counters: &validationToolCounters{}, schema: map[string]any{"type": "object"}})
	Load(AddMCPServer("registered", registered))
	Initialize()
	t.Cleanup(func() { globalContext = savedGlobalContext })
	host := newValidationHost(t, json.RawMessage(`{"server":{"name":"registered"},"allowTools":["valid"]}`))

	require.Equal(t, types.ActionPause, host.CallOnHttpRequestHeaders(modernToolHeaders("degraded")))
	require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody(modernToolCallBody("degraded", `{}`)))
	response := host.GetLocalResponse()
	require.NotNil(t, response)
	assert.Equal(t, int64(protocol.CodeInvalidParams), gjson.GetBytes(response.Data, "error.code").Int())
	assert.Contains(t, gjson.GetBytes(response.Data, "error.message").String(), "Tool not allowed")
	assert.False(t, gjson.GetBytes(response.Data, "error.data.reason").Exists())
	assert.NotContains(t, string(response.Data), "schema_validation_unavailable")
	assert.Empty(t, host.GetHttpCalloutAttributes())
}

func TestModernRegisteredToolFailureIsToolExecutionError(t *testing.T) {
	savedGlobalContext := globalContext
	globalContext = Context{servers: make(map[string]Server)}
	counters := &validationToolCounters{}
	registered := newValidationTestServer()
	registered.AddMCPTool("fail", &validationTestTool{
		counters: counters,
		schema:   map[string]any{"type": "object"},
		callErr:  errors.New("tool failed"),
	})
	Load(AddMCPServer("registered", registered))
	Initialize()
	t.Cleanup(func() { globalContext = savedGlobalContext })
	host := newValidationHost(t, json.RawMessage(`{"server":{"name":"registered"}}`))

	require.Equal(t, types.ActionPause, host.CallOnHttpRequestHeaders(modernToolHeaders("fail")))
	require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody(modernToolCallBody("fail", `{}`)))
	response := host.GetLocalResponse()
	require.NotNil(t, response)
	assert.Equal(t, uint32(200), response.StatusCode)
	assert.False(t, gjson.GetBytes(response.Data, "error").Exists())
	assert.True(t, gjson.GetBytes(response.Data, "result.isError").Bool())
	assert.Equal(t, "tool failed", gjson.GetBytes(response.Data, "result.content.0.text").String())
	assert.Equal(t, resultTypeComplete, gjson.GetBytes(response.Data, "result.resultType").String())
	assert.Equal(t, "registered", gjson.GetBytes(response.Data, "result._meta.io\\.modelcontextprotocol/serverInfo.name").String())
	assert.Equal(t, 1, counters.create)
	assert.Equal(t, 1, counters.call)
}

func TestLegacyRegisteredToolKeepsExistingArgumentBehavior(t *testing.T) {
	counters := &validationToolCounters{}
	installRegisteredValidationServer(t, counters)
	host := newValidationHost(t, json.RawMessage(`{"server":{"name":"registered"}}`))

	require.Equal(t, types.ActionPause, host.CallOnHttpRequestHeaders(legacyToolHeaders()))
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"count","arguments":{"count":"legacy"}}}`)
	require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody(body))
	response := host.GetLocalResponse()
	require.NotNil(t, response)
	assert.False(t, gjson.GetBytes(response.Data, "result.isError").Bool())
	assert.False(t, gjson.GetBytes(response.Data, "result.resultType").Exists())
	assert.Equal(t, 1, counters.create)
	assert.Equal(t, 1, counters.call)
}

func TestModernRESTToolValidationPreventsUpstreamInvocation(t *testing.T) {
	savedGlobalContext := globalContext
	globalContext = Context{servers: make(map[string]Server)}
	Initialize()
	t.Cleanup(func() { globalContext = savedGlobalContext })

	config := json.RawMessage(`{
		"server":{"name":"rest"},
		"tools":[{
			"name":"lookup",
			"description":"lookup",
			"args":[{"name":"count","description":"count","type":"integer","required":true}],
			"requestTemplate":{"url":"http://backend.example/items","method":"POST","argsToJsonBody":true}
		}]
	}`)
	host := newValidationHost(t, config)
	require.Equal(t, types.ActionPause, host.CallOnHttpRequestHeaders(modernToolHeaders("lookup")))
	require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody(modernToolCallBody("lookup", `{"count":"wrong"}`)))
	response := host.GetLocalResponse()
	require.NotNil(t, response)
	assert.True(t, gjson.GetBytes(response.Data, "result.isError").Bool())
	assert.Empty(t, host.GetHttpCalloutAttributes())
}

func TestModernRESTToolAcceptsValidArguments(t *testing.T) {
	savedGlobalContext := globalContext
	globalContext = Context{servers: make(map[string]Server)}
	Initialize()
	t.Cleanup(func() { globalContext = savedGlobalContext })

	config := json.RawMessage(`{
		"server":{"name":"rest"},
		"tools":[{
			"name":"lookup",
			"description":"lookup",
			"args":[{"name":"count","description":"count","type":"integer","required":true}],
			"requestTemplate":{"url":"http://backend.example/items","method":"POST","argsToJsonBody":true}
		}]
	}`)
	host := newValidationHost(t, config)
	require.Equal(t, types.ActionPause, host.CallOnHttpRequestHeaders(modernToolHeaders("lookup")))
	require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody(modernToolCallBody("lookup", `{"count":2}`)))
	assert.Nil(t, host.GetLocalResponse(), "valid arguments must not be rejected at the protocol boundary")
}

func TestRESTAsyncResultAdapterPreservesToolResultShapes(t *testing.T) {
	transport := protocol.Transport{
		Method:          "POST",
		Authority:       "mcp.example.com",
		ContentType:     "application/json",
		Accept:          "application/json, text/event-stream",
		ProtocolVersion: string(protocol.Version20260728),
		MCPMethod:       "tools/call",
		MCPName:         "result",
	}
	request, protocolError := protocol.PrepareRequest(transport, modernToolCallBody("result", `{}`), func(method string) bool {
		return method == "tools/call"
	})
	require.Nil(t, protocolError)
	ctx := &protocolTestHTTPContext{values: map[string]any{consts.CtxProtocolRequest: request}}

	// Installation happens in the synchronous tools/call frame. Applying it
	// later models REST success/error callbacks after Tool.Call has returned.
	installDirectToolResultAdapter(ctx, "rest-result")
	results := []struct {
		name  string
		value map[string]any
	}{
		{name: "text", value: map[string]any{"content": []map[string]any{{"type": "text", "text": "plain"}}, "isError": false}},
		{name: "image", value: map[string]any{"content": []map[string]any{{"type": "image", "data": "AAEC", "mimeType": "image/png"}}, "isError": false}},
		{name: "structured", value: map[string]any{"content": []map[string]any{{"type": "text", "text": `{"answer":42}`}}, "structuredContent": json.RawMessage(`{"answer":42}`), "isError": false}},
		{name: "error", value: map[string]any{"content": []map[string]any{{"type": "text", "text": "upstream failed"}}, "isError": true}},
	}
	for _, result := range results {
		t.Run(result.name, func(t *testing.T) {
			shaped := utils.ApplyMCPResultAdapter(ctx, result.value)
			assert.Equal(t, result.value["content"], shaped["content"])
			assert.Equal(t, result.value["structuredContent"], shaped["structuredContent"])
			assert.Equal(t, result.value["isError"], shaped["isError"])
			assert.Equal(t, resultTypeComplete, shaped["resultType"])
			meta := shaped["_meta"].(map[string]any)
			serverInfo := meta["io.modelcontextprotocol/serverInfo"].(map[string]any)
			assert.Equal(t, "rest-result", serverInfo["name"])
		})
	}

	legacy := &protocolTestHTTPContext{}
	installDirectToolResultAdapter(legacy, "rest-result")
	legacyResult := map[string]any{"content": []map[string]any{{"type": "text", "text": "plain"}}, "isError": false}
	assert.Equal(t, legacyResult, utils.ApplyMCPResultAdapter(legacy, legacyResult))
}

func TestDirectToolSnapshotsKeepProfileCompatibleDescriptors(t *testing.T) {
	inputSchema := map[string]any{"type": "object"}
	newTool := func() Tool {
		return &validationOutputTool{&validationTestTool{
			counters: &validationToolCounters{},
			schema:   inputSchema,
		}}
	}

	registered := newValidationTestServer()
	registered.AddMCPTool("zeta", newTool())
	registered.AddMCPTool("alpha", newTool())

	rest := NewRestMCPServer("rest")
	for _, name := range []string{"zeta", "alpha"} {
		require.NoError(t, rest.AddRestTool(RestTool{
			Name:            name,
			Description:     name,
			OutputSchema:    map[string]any{"type": "object"},
			RequestTemplate: RestToolRequestTemplate{URL: "/" + name, Method: "GET"},
		}))
	}

	registry := &GlobalToolRegistry{}
	registry.Initialize()
	registry.RegisterTool("registered", "zeta", newTool())
	registry.RegisterTool("registered", "alpha", newTool())
	composed := NewComposedMCPServer("composed", []ServerToolConfig{{
		ServerName: "registered",
		Tools:      []string{"zeta", "alpha"},
	}}, registry)

	tests := []struct {
		name      string
		server    Server
		wantNames []string
	}{
		{name: "registered", server: registered, wantNames: []string{"alpha", "zeta"}},
		{name: "rest", server: rest, wantNames: []string{"alpha", "zeta"}},
		{name: "composed", server: composed, wantNames: []string{"registered___alpha", "registered___zeta"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := compileDirectToolSnapshot(test.server)
			modern := snapshot.buildModernToolList(nil)
			require.Len(t, modern, 2)
			for i, descriptor := range modern {
				assert.Equal(t, test.wantNames[i], descriptor["name"])
				assert.Equal(t, "object", descriptor["inputSchema"].(map[string]any)["type"])
				assert.NotContains(t, descriptor, "outputSchema")
			}

			legacy := buildToolList(test.server, nil, true)
			require.Len(t, legacy, 2)
			for _, descriptor := range legacy {
				assert.Contains(t, descriptor, "outputSchema")
			}
		})
	}

	t.Run("composed generation isolates a non-serializable registered descriptor", func(t *testing.T) {
		registry := &GlobalToolRegistry{}
		registry.Initialize()
		registry.RegisterTool("registered", "bad", &validationTestTool{counters: &validationToolCounters{}, schema: map[string]any{
			"type":     "object",
			"callback": func() {},
		}})
		registry.RegisterTool("registered", "valid", &validationTestTool{counters: &validationToolCounters{}, schema: map[string]any{"type": "object"}})
		config := &McpServerConfig{}
		err := ParseConfigCore(gjson.Parse(`{
			"toolSet":{"name":"composed","serverTools":[{"serverName":"registered","tools":["bad","valid"]}]}
		}`), config, &ConfigOptions{Servers: map[string]Server{}, ToolRegistry: registry})
		require.NoError(t, err)
		assert.Equal(t, directToolSchemaValidationUnavailable, config.directTools.byName["registered___bad"].schemaState)
		assert.False(t, config.directTools.byName["registered___bad"].serializable)
		listed := config.directTools.buildModernToolList(nil)
		require.Len(t, listed, 1)
		assert.Equal(t, "registered___valid", listed[0]["name"])
	})
}

func TestDirectToolSnapshotOwnsSerializableDescriptorAndPreparesOnce(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"value": map[string]any{"type": "string"},
		},
	}
	tool := &validationTestTool{counters: &validationToolCounters{}, schema: schema}
	server := newValidationTestServer()
	server.AddMCPTool("tool", tool)

	snapshot := compileDirectToolSnapshot(server)
	require.Equal(t, 1, tool.schemaReads)
	require.Equal(t, directToolSchemaValidated, snapshot.byName["tool"].schemaState)

	schema["properties"].(map[string]any)["value"].(map[string]any)["type"] = "integer"
	listed := snapshot.buildModernToolList(nil)
	require.Len(t, listed, 1)
	property := listed[0]["inputSchema"].(map[string]any)["properties"].(map[string]any)["value"].(map[string]any)
	assert.Equal(t, "string", property["type"], "published generation must retain its JSON-cloned descriptor")
	assert.Equal(t, 1, tool.schemaReads, "listing and invocation reuse the prepared generation")
}

func TestParseConfigRegistryPublicationReusesCapturedGenerationDescriptor(t *testing.T) {
	firstSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"value": map[string]any{"type": "string"},
		},
	}
	tool := &changingDescriptorTool{
		firstSchema: firstSchema,
		secondSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"value": map[string]any{"type": "number"},
			},
		},
	}
	registered := newValidationTestServer()
	registered.AddMCPTool("changing", tool)
	registry := &GlobalToolRegistry{}
	registry.Initialize()
	opts := &ConfigOptions{Servers: map[string]Server{"registered": registered}, ToolRegistry: registry}

	direct := &McpServerConfig{}
	require.NoError(t, ParseConfigCore(gjson.Parse(`{"server":{"name":"registered"}}`), direct, opts))
	require.Equal(t, 1, tool.descriptionReads)
	require.Equal(t, 1, tool.schemaReads)

	registryView, found := registry.GetToolInfo("registered", "changing")
	require.True(t, found)
	registryView.InputSchema["properties"].(map[string]any)["value"].(map[string]any)["type"] = "boolean"
	directListed := direct.directTools.buildModernToolList(nil)
	require.Len(t, directListed, 1)
	directProperty := directListed[0]["inputSchema"].(map[string]any)["properties"].(map[string]any)["value"].(map[string]any)
	assert.Equal(t, "string", directProperty["type"], "registry reads must not mutate the direct generation")
	registryView, found = registry.GetToolInfo("registered", "changing")
	require.True(t, found)
	registryProperty := registryView.InputSchema["properties"].(map[string]any)["value"].(map[string]any)
	assert.Equal(t, "string", registryProperty["type"], "registry reads must return independent clones")

	firstSchema["properties"].(map[string]any)["value"].(map[string]any)["type"] = "integer"
	composed := &McpServerConfig{}
	require.NoError(t, ParseConfigCore(gjson.Parse(`{
		"toolSet":{"name":"composed","serverTools":[{"serverName":"registered","tools":["changing"]}]}
	}`), composed, opts))

	require.Equal(t, 1, tool.descriptionReads, "registry publication must not reread Description")
	require.Equal(t, 1, tool.schemaReads, "registry publication and composed parsing must not reread InputSchema")
	directEntry := direct.directTools.byName["changing"]
	composedEntry := composed.directTools.byName["registered___changing"]
	assert.Equal(t, "captured description", directEntry.description)
	assert.Equal(t, directEntry.description, composedEntry.description)
	assert.Equal(t, directEntry.inputSchema, composedEntry.inputSchema)
	property := composedEntry.inputSchema["properties"].(map[string]any)["value"].(map[string]any)
	assert.Equal(t, "string", property["type"], "registry must retain the generation-owned clone after caller mutation")
	assert.Equal(t, directToolSchemaValidated, composedEntry.schemaState)
}

func TestComposedSnapshotPreservesLegacyOnlyRESTCompatibility(t *testing.T) {
	rest := NewRestMCPServer("rest")
	require.NoError(t, rest.AddRestTool(RestTool{
		Name:        "legacy",
		Description: "legacy",
		LegacyOnly:  true,
		Args: []RestToolArg{{
			Name:        "value",
			Description: "value",
			Type:        "array",
			Items:       map[string]any{"oneOf": []any{}},
		}},
		RequestTemplate: RestToolRequestTemplate{URL: "/legacy", Method: "POST"},
	}))
	registry := &GlobalToolRegistry{}
	registry.Initialize()
	registry.RegisterTool("rest", "legacy", rest.GetMCPTools()["legacy"])

	config := &McpServerConfig{}
	require.NoError(t, ParseConfigCore(gjson.Parse(`{
		"toolSet":{"name":"composed","serverTools":[{"serverName":"rest","tools":["legacy"]}]}
	}`), config, &ConfigOptions{Servers: map[string]Server{}, ToolRegistry: registry}))
	assert.True(t, config.isComposed)
	assert.Empty(t, config.directTools.buildModernToolList(nil))
	assert.Contains(t, config.directTools.legacyOnly["rest___legacy"], `unsupported schema keyword "oneOf"`)
	assert.Equal(t, directToolSchemaExplicitLegacyOnly, config.directTools.byName["rest___legacy"].schemaState)
	assert.False(t, modernMethodPolicy(*config, "tools/call").Available)

	legacy := buildToolList(config.server, nil, true)
	require.Len(t, legacy, 1)
	assert.Equal(t, "rest___legacy", legacy[0]["name"])
	inputSchema := legacy[0]["inputSchema"].(map[string]any)
	items := inputSchema["properties"].(map[string]any)["value"].(map[string]any)["items"].(map[string]any)
	assert.Contains(t, items, "oneOf", "legacy descriptor must retain unsupported schema semantics")

	clonedSnapshot := compileDirectToolSnapshot(config.server.Clone())
	assert.Empty(t, clonedSnapshot.buildModernToolList(nil))
	assert.Contains(t, clonedSnapshot.legacyOnly, "rest___legacy")
}

func TestAdmissibleUnsupportedDirectToolSchemasDegradePerTool(t *testing.T) {
	unsupported := map[string]any{
		"type":  "object",
		"oneOf": []any{map[string]any{"type": "object"}},
	}

	t.Run("registered", func(t *testing.T) {
		registered := newValidationTestServer()
		registered.AddMCPTool("bad", &validationTestTool{counters: &validationToolCounters{}, schema: unsupported})
		config := &McpServerConfig{}
		opts := &ConfigOptions{
			Servers:      map[string]Server{"registered": registered},
			ToolRegistry: &GlobalToolRegistry{},
		}
		opts.ToolRegistry.Initialize()
		require.NoError(t, ParseConfigCore(gjson.Parse(`{"server":{"name":"registered"}}`), config, opts))
		entry := config.directTools.byName["bad"]
		assert.Equal(t, directToolSchemaValidationUnavailable, entry.schemaState)
		assert.Nil(t, entry.validator)
		assert.Equal(t, unsupported, entry.inputSchema)
	})

	t.Run("rest", func(t *testing.T) {
		rest := NewRestMCPServer("rest")
		err := rest.AddRestTool(RestTool{
			Name:        "bad",
			Description: "bad",
			Args: []RestToolArg{{
				Name:        "items",
				Description: "items",
				Type:        "array",
				Items:       map[string]any{"oneOf": []any{}},
			}},
			RequestTemplate: RestToolRequestTemplate{URL: "/items", Method: "POST"},
		})
		require.NoError(t, err)
		snapshot := compileDirectToolSnapshot(rest)
		assert.Equal(t, directToolSchemaValidationUnavailable, snapshot.byName["bad"].schemaState)
		assert.Len(t, snapshot.buildModernToolList(nil), 1)
	})

	t.Run("composed", func(t *testing.T) {
		registry := &GlobalToolRegistry{}
		registry.Initialize()
		registry.RegisterTool("registered", "bad", &validationTestTool{counters: &validationToolCounters{}, schema: unsupported})
		config := &McpServerConfig{}
		opts := &ConfigOptions{Servers: map[string]Server{}, ToolRegistry: registry}
		err := ParseConfigCore(gjson.Parse(`{
			"toolSet":{"name":"composed","serverTools":[{"serverName":"registered","tools":["bad"]}]}
		}`), config, opts)
		require.NoError(t, err)
		entry := config.directTools.byName["registered___bad"]
		assert.Equal(t, directToolSchemaValidationUnavailable, entry.schemaState)
		assert.Equal(t, unsupported, entry.inputSchema)
	})
}

func schemaReasonForTool(t *testing.T, snapshot directToolSnapshot, name string) schemaDiagnosticReason {
	t.Helper()
	for _, diagnostic := range snapshot.degraded {
		if diagnostic.name == name {
			return diagnostic.reason
		}
	}
	require.FailNow(t, "missing degraded diagnostic", name)
	return 0
}

func TestEverySchemaPreparationFailurePublishesUnavailableState(t *testing.T) {
	deep := map[string]any{"type": "string"}
	for i := 0; i < maxSchemaDepth+2; i++ {
		deep = map[string]any{"type": "array", "items": deep}
	}
	manyProperties := make(map[string]any, maxSchemaCollectionSize+1)
	for i := 0; i < maxSchemaCollectionSize+1; i++ {
		manyProperties[fmt.Sprintf("property-%04d", i)] = map[string]any{"type": "string"}
	}
	largeEnum := make([]any, maxSchemaEnumSize+1)
	for i := range largeEnum {
		largeEnum[i] = fmt.Sprintf("value-%03d", i)
	}
	marshalCalls := 0
	opaque := opaqueSchemaValue{values: []string{"original"}, calls: &marshalCalls}
	var snapshotDeep any = "leaf"
	for i := 0; i <= maxSchemaSnapshotDepth; i++ {
		snapshotDeep = []any{snapshotDeep}
	}
	snapshotWide := make([]any, maxSchemaSnapshotItems/2)
	for index := range snapshotWide {
		snapshotWide[index] = []any{0, 1, 2, 3, 4, 5, 6, 7}
	}

	tests := []struct {
		name         string
		schema       map[string]any
		wantReason   schemaDiagnosticReason
		serializable bool
	}{
		{name: "nil", schema: nil, wantReason: schemaDiagnosticUnsupportedForm, serializable: true},
		{name: "invalid root", schema: map[string]any{"type": "string"}, wantReason: schemaDiagnosticUnsupportedForm, serializable: true},
		{name: "byte limit", schema: map[string]any{"type": "object", "description": strings.Repeat("x", maxToolInputSchemaBytes)}, wantReason: schemaDiagnosticResourceLimit, serializable: true},
		{name: "depth limit", schema: map[string]any{"type": "object", "properties": map[string]any{"value": deep}}, wantReason: schemaDiagnosticResourceLimit, serializable: true},
		{name: "node and collection limit", schema: map[string]any{"type": "object", "properties": manyProperties}, wantReason: schemaDiagnosticResourceLimit, serializable: true},
		{name: "enum limit", schema: map[string]any{"type": "object", "properties": map[string]any{"value": map[string]any{"type": "string", "enum": largeEnum}}}, wantReason: schemaDiagnosticResourceLimit, serializable: true},
		{name: "numeric comparison limit", schema: map[string]any{"type": "object", "properties": map[string]any{"value": map[string]any{"type": "number", "enum": []any{json.Number("1e5000")}}}}, wantReason: schemaDiagnosticResourceLimit, serializable: true},
		{name: "unsupported keyword", schema: map[string]any{"type": "object", "oneOf": []any{}}, wantReason: schemaDiagnosticUnsupportedKeyword, serializable: true},
		{name: "contradictory constraint", schema: map[string]any{"type": "object", "enum": []any{"x"}}, wantReason: schemaDiagnosticContradictoryConstraint, serializable: true},
		{name: "serialization failure", schema: map[string]any{"type": "object", "callback": func() {}}, wantReason: schemaDiagnosticSerializationFailure, serializable: false},
		{name: "opaque custom marshaler", schema: map[string]any{"type": "object", "opaque": opaque}, wantReason: schemaDiagnosticSerializationFailure, serializable: false},
		{name: "snapshot depth limit", schema: map[string]any{"type": "object", "opaque": snapshotDeep}, wantReason: schemaDiagnosticResourceLimit, serializable: false},
		{name: "snapshot node limit", schema: map[string]any{"type": "object", "opaque": snapshotWide}, wantReason: schemaDiagnosticResourceLimit, serializable: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			registered := newValidationTestServer()
			registered.AddMCPTool("bad", &validationTestTool{counters: &validationToolCounters{}, schema: test.schema})
			config := &McpServerConfig{}
			opts := &ConfigOptions{Servers: map[string]Server{"registered": registered}, ToolRegistry: &GlobalToolRegistry{}}
			opts.ToolRegistry.Initialize()
			require.NoError(t, ParseConfigCore(gjson.Parse(`{"server":{"name":"registered"}}`), config, opts))

			entry := config.directTools.byName["bad"]
			assert.Equal(t, directToolSchemaValidationUnavailable, entry.schemaState)
			assert.Nil(t, entry.validator)
			assert.Equal(t, test.serializable, entry.serializable)
			assert.Equal(t, test.wantReason, schemaReasonForTool(t, config.directTools, "bad"))
			_, published := opts.ToolRegistry.GetToolInfo("registered", "bad")
			assert.True(t, published, "schema preparation must not reject registry publication")
			if test.serializable {
				require.Len(t, config.directTools.buildModernToolList(nil), 1)
			} else {
				assert.Empty(t, config.directTools.buildModernToolList(nil))
			}
		})
	}
	assert.Zero(t, marshalCalls, "parseConfig must not execute a schema custom marshaler")
	opaque.values[0] = "mutated"
}

func restSchemaCompatibilityConfig(t *testing.T, arg RestToolArg) json.RawMessage {
	t.Helper()
	config := map[string]any{
		"server": map[string]any{"name": "rest"},
		"tools": []any{map[string]any{
			"name":        "compat",
			"description": "compat",
			"args":        []any{arg},
			"requestTemplate": map[string]any{
				"url":    "/compat",
				"method": "POST",
			},
		}},
	}
	raw, err := json.Marshal(config)
	require.NoError(t, err)
	return raw
}

func TestRESTSchemaPreparationLimitsDoNotChangeParseConfigAcceptance(t *testing.T) {
	deepItems := map[string]any{"type": "string"}
	for i := 0; i < maxSchemaDepth+2; i++ {
		deepItems = map[string]any{"type": "array", "items": deepItems}
	}
	manyProperties := make(map[string]any, maxSchemaCollectionSize+1)
	for i := 0; i < maxSchemaCollectionSize+1; i++ {
		manyProperties[fmt.Sprintf("property-%04d", i)] = map[string]any{"type": "string"}
	}
	largeEnum := make([]any, maxSchemaEnumSize+1)
	for i := range largeEnum {
		largeEnum[i] = fmt.Sprintf("value-%03d", i)
	}

	tests := []struct {
		name string
		arg  RestToolArg
	}{
		{name: "byte limit", arg: RestToolArg{Name: "value", Type: "string", Description: strings.Repeat("x", maxToolInputSchemaBytes)}},
		{name: "depth limit", arg: RestToolArg{Name: "value", Type: "array", Items: deepItems}},
		{name: "node and collection limit", arg: RestToolArg{Name: "value", Type: "object", Properties: manyProperties}},
		{name: "enum limit", arg: RestToolArg{Name: "value", Type: "string", Enum: largeEnum}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := parseRESTConfigForTest(t, restSchemaCompatibilityConfig(t, test.arg))
			entry := config.directTools.byName["compat"]
			assert.Equal(t, directToolSchemaValidationUnavailable, entry.schemaState)
			assert.Nil(t, entry.validator)
			assert.True(t, entry.serializable)
			require.Len(t, config.directTools.buildModernToolList(nil), 1)
			require.Len(t, buildToolList(config.server, nil, true), 1)
		})
	}

	t.Run("independent invalid template remains fatal", func(t *testing.T) {
		raw := restSchemaCompatibilityConfig(t, RestToolArg{Name: "value", Type: "string"})
		var config map[string]any
		require.NoError(t, json.Unmarshal(raw, &config))
		config["tools"].([]any)[0].(map[string]any)["requestTemplate"] = map[string]any{"url": "{{", "method": "GET"}
		encoded, err := json.Marshal(config)
		require.NoError(t, err)
		registry := &GlobalToolRegistry{}
		registry.Initialize()
		err = ParseConfigCore(gjson.ParseBytes(encoded), &McpServerConfig{}, &ConfigOptions{Servers: map[string]Server{}, ToolRegistry: registry})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "error parsing URL template")
	})
}

func TestDirectToolSnapshotReloadsDoNotRetainStaleValidatorState(t *testing.T) {
	serverFor := func(schema map[string]any) Server {
		registered := newValidationTestServer()
		registered.AddMCPTool("tool", &validationTestTool{counters: &validationToolCounters{}, schema: schema})
		return registered
	}
	valid := map[string]any{
		"type":       "object",
		"properties": map[string]any{"value": map[string]any{"type": "string"}},
	}
	unvalidated := map[string]any{
		"type":       "object",
		"properties": map[string]any{"value": map[string]any{"type": "array", "enum": []any{"A"}}},
	}

	first := compileDirectToolSnapshot(serverFor(valid))
	require.Equal(t, directToolSchemaValidated, first.byName["tool"].schemaState)
	require.NotNil(t, first.byName["tool"].validator)

	second := compileDirectToolSnapshot(serverFor(unvalidated))
	require.Equal(t, directToolSchemaValidationUnavailable, second.byName["tool"].schemaState)
	require.Nil(t, second.byName["tool"].validator)
	require.Equal(t, "array", second.byName["tool"].inputSchema["properties"].(map[string]any)["value"].(map[string]any)["type"])

	third := compileDirectToolSnapshot(serverFor(valid))
	require.Equal(t, directToolSchemaValidated, third.byName["tool"].schemaState)
	require.NotNil(t, third.byName["tool"].validator)
	assert.NotSame(t, first.byName["tool"].validator, third.byName["tool"].validator)
}

func TestDirectToolDegradedPublicationWarningIsBounded(t *testing.T) {
	registered := newValidationTestServer()
	for i := 0; i < maxSchemaDiagnosticRecords+3; i++ {
		name := fmt.Sprintf("tool-%02d", i)
		registered.AddMCPTool(name, &validationTestTool{counters: &validationToolCounters{}, schema: map[string]any{
			"type":  "object",
			"oneOf": []any{map[string]any{"type": "object"}},
		}})
	}
	snapshot := compileDirectToolSnapshot(registered)
	warning := snapshot.degradedPublicationWarning("registered")
	assert.Contains(t, warning, `server="registered" total=11`)
	assert.Contains(t, warning, `reason=unsupported_keyword`)
	assert.Contains(t, warning, `omitted=3`)
	assert.Contains(t, warning, "modern tools/call will be rejected; legacy calls remain available")
	assert.NotContains(t, warning, `tool-08`)
	assert.Len(t, []rune(boundedSchemaDiagnosticToolName(strings.Repeat("工", maxSchemaDiagnosticToolNameRunes+10))), maxSchemaDiagnosticToolNameRunes+3)
}

func checkedInRESTConfig(t *testing.T, name string) json.RawMessage {
	t.Helper()
	raw, err := os.ReadFile("../../../mcp-servers/" + name + "/mcp-server.yaml")
	require.NoError(t, err)
	var config map[string]any
	require.NoError(t, yaml.Unmarshal(raw, &config))
	encoded, err := json.Marshal(config)
	require.NoError(t, err)
	return encoded
}

func parseRESTConfigForTest(t *testing.T, raw json.RawMessage) *McpServerConfig {
	t.Helper()
	registry := &GlobalToolRegistry{}
	registry.Initialize()
	config := &McpServerConfig{}
	require.NoError(t, ParseConfigCore(gjson.ParseBytes(raw), config, &ConfigOptions{
		Servers:      map[string]Server{},
		ToolRegistry: registry,
	}))
	return config
}

func TestCheckedInRESTSchemasRemainLegacyCompatibleAndModernHonest(t *testing.T) {
	tests := []struct {
		name            string
		legacyOnlyTools []string
		reasonTool      string
		wantReason      string
	}{
		{
			name:            "mcp-e2bdev",
			legacyOnlyTools: []string{"create_sandbox"},
			reasonTool:      "create_sandbox",
			wantReason:      `unsupported type "int"`,
		},
		{
			name:            "mcp-firecrawl",
			legacyOnlyTools: []string{"scrape", "batch_scrape", "map", "extract", "search"},
			reasonTool:      "scrape",
			wantReason:      `unsupported schema keyword "oneOf"`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := parseRESTConfigForTest(t, checkedInRESTConfig(t, test.name))
			legacy := buildToolList(config.server, nil, true)
			modern := config.directTools.buildModernToolList(nil)
			require.NotEmpty(t, legacy)
			actualLegacyOnly := make([]string, 0, len(config.directTools.legacyOnly))
			for name := range config.directTools.legacyOnly {
				actualLegacyOnly = append(actualLegacyOnly, name)
			}
			assert.ElementsMatch(t, test.legacyOnlyTools, actualLegacyOnly)
			assert.Contains(t, config.directTools.legacyOnly[test.reasonTool], test.wantReason)
			assert.Len(t, modern, len(legacy)-len(config.directTools.legacyOnly))
			for _, descriptor := range modern {
				assert.NotContains(t, config.directTools.legacyOnly, descriptor["name"].(string))
			}
		})
	}
}

func TestModernCannotInvokeCheckedInLegacyOnlyRESTSchema(t *testing.T) {
	savedGlobalContext := globalContext
	globalContext = Context{servers: make(map[string]Server)}
	Initialize()
	t.Cleanup(func() { globalContext = savedGlobalContext })
	host := newValidationHost(t, checkedInRESTConfig(t, "mcp-e2bdev"))

	require.Equal(t, types.ActionPause, host.CallOnHttpRequestHeaders(modernToolHeaders("create_sandbox")))
	require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody(modernToolCallBody("create_sandbox", `{"timeout":300}`)))
	response := host.GetLocalResponse()
	require.NotNil(t, response)
	assert.True(t, gjson.GetBytes(response.Data, "result.isError").Bool())
	assert.Equal(t, resultTypeComplete, gjson.GetBytes(response.Data, "result.resultType").String())
	assert.Contains(t, gjson.GetBytes(response.Data, "result.content.0.text").String(), "unavailable in the modern profile")
	assert.Empty(t, host.GetHttpCalloutAttributes(), "legacy-only schema must not reach the REST upstream for modern calls")
}
