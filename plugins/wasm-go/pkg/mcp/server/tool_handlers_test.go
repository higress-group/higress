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
	"os"
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
	counters *validationToolCounters
	schema   map[string]any
	callErr  error
}

type validationOutputTool struct {
	*validationTestTool
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

func (t *validationTestTool) Description() string         { return "validation test tool" }
func (t *validationTestTool) InputSchema() map[string]any { return t.schema }

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
			snapshot, err := compileDirectToolSnapshot(test.server)
			require.NoError(t, err)
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
	assert.False(t, modernMethodPolicy(*config, "tools/call").Available)

	legacy := buildToolList(config.server, nil, true)
	require.Len(t, legacy, 1)
	assert.Equal(t, "rest___legacy", legacy[0]["name"])
	inputSchema := legacy[0]["inputSchema"].(map[string]any)
	items := inputSchema["properties"].(map[string]any)["value"].(map[string]any)["items"].(map[string]any)
	assert.Contains(t, items, "oneOf", "legacy descriptor must retain unsupported schema semantics")

	clonedSnapshot, err := compileDirectToolSnapshot(config.server.Clone())
	require.NoError(t, err)
	assert.Empty(t, clonedSnapshot.buildModernToolList(nil))
	assert.Contains(t, clonedSnapshot.legacyOnly, "rest___legacy")
}

func TestDirectToolSchemasAreRejectedBeforeServing(t *testing.T) {
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
		err := ParseConfigCore(gjson.Parse(`{"server":{"name":"registered"}}`), config, opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `unsupported schema keyword "oneOf"`)
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
		require.Error(t, err)
		assert.Contains(t, err.Error(), `unsupported schema keyword "oneOf"`)
		assert.Contains(t, err.Error(), "legacyOnly: true")
		assert.Empty(t, rest.GetMCPTools())
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
		require.Error(t, err)
		assert.Contains(t, err.Error(), `unsupported schema keyword "oneOf"`)
	})
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
