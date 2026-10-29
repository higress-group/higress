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
	"bytes"
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
	create    int
	call      int
	arguments [][]byte
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

func (t *validationTestTool) Create(raw []byte) Tool {
	t.counters.create++
	t.counters.arguments = append(t.counters.arguments, append([]byte(nil), raw...))
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

func profileToolCall(modern bool, name, arguments string) ([][2]string, []byte) {
	headers := legacyToolHeaders()
	params := `"name":"` + name + `"`
	if arguments != "" {
		params += `,"arguments":` + arguments
	}
	if modern {
		headers = modernToolHeaders(name)
		params += `,"_meta":{"` + protocol.MetaProtocolVersion + `":"2026-07-28"}`
	}
	return headers, []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{` + params + `}}`)
}

func TestDirectToolProfilesPreserveOriginalCreateArguments(t *testing.T) {
	arguments := []string{
		`{"count":"wrong","extra":true}`,
		`{ "count": 9007199254740993, "count": 2, "nested": {"a":1,"a":2} }`,
		`{"count":1.50,"exponent":1e+300}`,
		`null`, `"opaque"`, ``,
	}
	for _, modern := range []bool{false, true} {
		for i, raw := range arguments {
			t.Run(fmt.Sprintf("modern=%t/case=%d", modern, i), func(t *testing.T) {
				counters := &validationToolCounters{}
				installRegisteredValidationServer(t, counters)
				host := newValidationHost(t, json.RawMessage(`{"server":{"name":"registered"}}`))
				headers, body := profileToolCall(modern, "count", raw)
				require.Equal(t, types.ActionPause, host.CallOnHttpRequestHeaders(headers))
				require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody(body))
				require.Equal(t, 1, counters.create)
				require.Equal(t, 1, counters.call)
				assert.Equal(t, raw, string(counters.arguments[0]), "gateway must not normalize direct Tool.Create input")
				response := host.GetLocalResponse()
				require.NotNil(t, response)
				assert.False(t, gjson.GetBytes(response.Data, "result.isError").Bool())
			})
		}
	}
}

func TestModernToolArgumentsDoNotRetainValidatorBudgets(t *testing.T) {
	arguments := []string{
		`{"padding":"` + strings.Repeat("x", 1024*1024+1) + `"}`,
		`{"nested":` + strings.Repeat("[", 70) + `0` + strings.Repeat("]", 70) + `}`,
		`{"values":[` + strings.Repeat("0,", 65536) + `0]}`,
	}
	for i, raw := range arguments {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			counters := &validationToolCounters{}
			installRegisteredValidationServer(t, counters)
			host := newValidationHost(t, json.RawMessage(`{"server":{"name":"registered"}}`))
			headers, body := profileToolCall(true, "count", raw)
			require.Equal(t, types.ActionPause, host.CallOnHttpRequestHeaders(headers))
			require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody(body))
			assert.Equal(t, 1, counters.create)
			assert.Equal(t, 1, counters.call)
			assert.Equal(t, raw, string(counters.arguments[0]))
		})
	}
}

func TestToolProfilesRetainAllowToolsAndLookupOrdering(t *testing.T) {
	for _, modern := range []bool{false, true} {
		for _, denied := range []string{"config", "header", "unknown"} {
			t.Run(fmt.Sprintf("modern=%t/%s", modern, denied), func(t *testing.T) {
				counters := &validationToolCounters{}
				installRegisteredValidationServer(t, counters)
				config := json.RawMessage(`{"server":{"name":"registered"}}`)
				if denied == "config" {
					config = json.RawMessage(`{"server":{"name":"registered"},"allowTools":[]}`)
				}
				host := newValidationHost(t, config)
				name := "count"
				if denied == "unknown" {
					name = "unknown"
				}
				headers, body := profileToolCall(modern, name, `{"count":"schema mismatch"}`)
				if denied == "header" {
					headers = append(headers, [2]string{"x-envoy-allow-mcp-tools", "other"})
				}
				require.Equal(t, types.ActionPause, host.CallOnHttpRequestHeaders(headers))
				require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody(body))
				response := host.GetLocalResponse()
				require.NotNil(t, response)
				assert.Equal(t, int64(utils.ErrInvalidParams), gjson.GetBytes(response.Data, "error.code").Int())
				assert.Zero(t, counters.create)
				assert.Zero(t, counters.call)
				assert.Empty(t, host.GetHttpCalloutAttributes())
			})
		}
	}
}

type descriptorMarshaler struct {
	state map[string]any
	calls *int
}

func (value descriptorMarshaler) MarshalJSON() ([]byte, error) {
	*value.calls++
	return json.Marshal(value.state)
}

type descriptorFixtureTool struct {
	input, output                             map[string]any
	description                               string
	inputReads, outputReads, descriptionReads int
}

func (tool *descriptorFixtureTool) Create([]byte) Tool             { return tool }
func (tool *descriptorFixtureTool) Call(HttpContext, Server) error { return nil }
func (tool *descriptorFixtureTool) InputSchema() map[string]any    { tool.inputReads++; return tool.input }
func (tool *descriptorFixtureTool) OutputSchema() map[string]any {
	tool.outputReads++
	return tool.output
}
func (tool *descriptorFixtureTool) Description() string {
	tool.descriptionReads++
	return tool.description
}

func TestDescriptorSnapshotsUseJSONAndPreservePublicViews(t *testing.T) {
	calls := 0
	state := map[string]any{"value": "original", "number": json.Number("9007199254740993")}
	raw := json.RawMessage(`{"value":"original","number":9007199254740993}`)
	input := map[string]any{
		"type": "object", "oneOf": []any{true, false}, "integer": int32(7), "required": []string{"value"},
		"raw": raw, "custom": descriptorMarshaler{state: state, calls: &calls},
		"typed": []int32{3, 4}, "bytes": []byte{1, 2, 3}, "keyed": map[int]string{1: "one"},
		"struct": struct {
			Name string `json:"name"`
		}{Name: "value"},
	}
	output := map[string]any{"type": "object", "custom": descriptorMarshaler{state: state, calls: &calls}}
	wantInput, err := json.Marshal(input)
	require.NoError(t, err)
	wantOutput, err := json.Marshal(output)
	require.NoError(t, err)
	tool := &descriptorFixtureTool{input: input, output: output, description: strings.Repeat("description ", 1024)}
	registered := newValidationTestServer()
	registered.AddMCPTool("tool", tool)
	registry := &GlobalToolRegistry{}
	registry.Initialize()
	opts := &ConfigOptions{Servers: map[string]Server{"registered": registered}, ToolRegistry: registry}
	direct := &McpServerConfig{}
	require.NoError(t, ParseConfigCore(gjson.Parse(`{"server":{"name":"registered"}}`), direct, opts))
	require.Equal(t, 1, tool.inputReads)
	require.Equal(t, 1, tool.outputReads)
	require.Equal(t, 1, tool.descriptionReads)
	frozenCalls := calls

	input["integer"] = int32(9)
	input["required"].([]string)[0] = "mutated"
	copy(raw, bytes.ReplaceAll(raw, []byte("original"), []byte("mutated!")))
	state["value"] = "mutated"
	tool.description = "mutated"

	composed := &McpServerConfig{}
	require.NoError(t, ParseConfigCore(gjson.Parse(`{"toolSet":{"name":"composed","serverTools":[{"serverName":"registered","tools":["tool"]}]}}`), composed, opts))
	for _, config := range []*McpServerConfig{direct, composed} {
		listed := config.directTools.buildToolList(nil)
		require.Len(t, listed, 1)
		gotInput, err := json.Marshal(listed[0]["inputSchema"])
		require.NoError(t, err)
		gotOutput, err := json.Marshal(listed[0]["outputSchema"])
		require.NoError(t, err)
		assert.Equal(t, string(wantInput), string(gotInput))
		assert.Equal(t, string(wantOutput), string(gotOutput))
		assert.Contains(t, string(gotInput), "9007199254740993")
		assert.NotEqual(t, "mutated", listed[0]["description"])
		listed[0]["inputSchema"].(map[string]any)["integer"] = 12
		listed[0]["outputSchema"].(map[string]any)["mutated"] = true
		fresh := config.directTools.buildToolList(nil)
		assert.Equal(t, int32(7), fresh[0]["inputSchema"].(map[string]any)["integer"])
		assert.NotContains(t, fresh[0]["outputSchema"], "mutated")
	}
	info, found := registry.GetToolInfo("registered", "tool")
	require.True(t, found)
	assert.IsType(t, int32(0), info.InputSchema["integer"])
	assert.Equal(t, []string{"value"}, info.InputSchema["required"])
	assert.Equal(t, []int32{3, 4}, info.InputSchema["typed"])
	assert.Equal(t, []byte{1, 2, 3}, info.InputSchema["bytes"])
	info.InputSchema["required"].([]string)[0] = "changed through registry"
	fresh, found := registry.GetToolInfo("registered", "tool")
	require.True(t, found)
	assert.Equal(t, []string{"value"}, fresh.InputSchema["required"])
	assert.Equal(t, frozenCalls, calls, "snapshots and reads use the owned JSON, not the original Marshaler")
	assert.Equal(t, 1, tool.inputReads)
	assert.Equal(t, 1, tool.outputReads)
	assert.Equal(t, 1, tool.descriptionReads)
}

func TestSerializableDescriptorSizesDoNotCreateAdmissionGates(t *testing.T) {
	deep := map[string]any{"value": true}
	for i := 0; i < 300; i++ {
		deep = map[string]any{"nested": deep}
	}
	wide := make([]any, 65537)
	for i := range wide {
		wide[i] = i
	}
	for i, schema := range []map[string]any{
		{"type": "int", "description": strings.Repeat("x", 1024*1024+1)},
		{"oneOf": []any{true}, "nested": deep},
		{"type": "object", "wide": wide},
	} {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			server := newValidationTestServer()
			tool := &validationTestTool{counters: &validationToolCounters{}, schema: schema}
			server.AddMCPTool(strings.Repeat("name", 2048), tool)
			snapshot := captureDirectToolSnapshot(server)
			require.Len(t, snapshot.buildToolList(nil), 1)
			assert.Equal(t, 1, tool.schemaReads)
		})
	}
}

func TestUnserializableDescriptorDoesNotBlockToolExecution(t *testing.T) {
	for _, modern := range []bool{false, true} {
		t.Run(fmt.Sprint(modern), func(t *testing.T) {
			saved := globalContext
			globalContext = Context{servers: make(map[string]Server)}
			t.Cleanup(func() { globalContext = saved })
			counters := &validationToolCounters{}
			server := newValidationTestServer()
			server.AddMCPTool("bad", &validationTestTool{counters: counters, schema: map[string]any{"callback": func() {}}})
			Load(AddMCPServer("registered", server))
			Initialize()
			host := newValidationHost(t, json.RawMessage(`{"server":{"name":"registered"}}`))
			headers, body := profileToolCall(modern, "bad", `{}`)
			require.Equal(t, types.ActionPause, host.CallOnHttpRequestHeaders(headers))
			require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody(body))
			assert.Equal(t, 1, counters.create)
			assert.Equal(t, 1, counters.call)
		})
	}
}

func TestRESTProfilesIgnoreLegacyOnlyAndKeepDeclaredConversions(t *testing.T) {
	saved := globalContext
	globalContext = Context{servers: make(map[string]Server)}
	Initialize()
	t.Cleanup(func() { globalContext = saved })
	for _, flag := range []string{"", `,"legacyOnly":false`, `,"legacyOnly":true`} {
		config := json.RawMessage(`{"server":{"name":"rest"},"tools":[{"name":"lookup","description":"lookup"` + flag + `,"args":[{"name":"count","type":"integer","required":true}],"outputSchema":{"oneOf":[true]},"requestTemplate":{"url":"http://backend.example/items","method":"POST","argsToJsonBody":true}}]}`)
		for _, modern := range []bool{false, true} {
			t.Run(fmt.Sprintf("flag=%s/modern=%t/list", flag, modern), func(t *testing.T) {
				host := newValidationHost(t, config)
				headers := legacyToolHeaders()
				body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
				if modern {
					headers = modernToolListHeaders()
					body = modernProxyListBody(1)
				}
				require.Equal(t, types.ActionPause, host.CallOnHttpRequestHeaders(headers))
				require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody(body))
				response := host.GetLocalResponse()
				require.NotNil(t, response)
				assert.Equal(t, "lookup", gjson.GetBytes(response.Data, "result.tools.0.name").String())
				assert.Equal(t, "[true]", gjson.GetBytes(response.Data, "result.tools.0.outputSchema.oneOf").Raw)
			})
			for _, args := range []struct{ raw, want string }{
				{`{"count":"wrong"}`, `"wrong"`},
				{`{"count":"3"}`, `3`},
				{`{"count":1,"count":"4"}`, `4`},
			} {
				t.Run(fmt.Sprintf("flag=%s/modern=%t/args=%s", flag, modern, args.want), func(t *testing.T) {
					host := newValidationHost(t, config)
					headers, body := profileToolCall(modern, "lookup", args.raw)
					require.Equal(t, types.ActionPause, host.CallOnHttpRequestHeaders(headers))
					require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody(body))
					assert.Nil(t, host.GetLocalResponse())
					assert.Equal(t, args.want, gjson.GetBytes(host.GetRequestBody(), "count").Raw)
				})
			}
		}
	}
}

func TestBuiltinRESTToolsRemainVisibleAndCallableWithoutFlags(t *testing.T) {
	for _, item := range []struct{ name, tool, args string }{
		{"mcp-e2bdev", "create_sandbox", `{"timeout":300}`},
		{"mcp-firecrawl", "scrape", `{"url":"https://example.com"}`},
	} {
		t.Run(item.name, func(t *testing.T) {
			raw := checkedInRESTConfig(t, item.name)
			assert.NotContains(t, string(raw), "legacyOnly")
			config := parseRESTConfigForTest(t, raw)
			require.Len(t, config.directTools.buildToolList(nil), len(config.server.GetMCPTools()))
			for _, modern := range []bool{false, true} {
				t.Run(fmt.Sprint(modern), func(t *testing.T) {
					host := newValidationHost(t, raw)
					headers, body := profileToolCall(modern, item.tool, item.args)
					require.Equal(t, types.ActionPause, host.CallOnHttpRequestHeaders(headers))
					require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody(body))
					assert.Nil(t, host.GetLocalResponse(), "formerly tagged tool must reach its ordinary REST route")
				})
			}
		})
	}
}

type descriptorHiddenCycle struct {
	Next    *descriptorHiddenCycle `json:"-"`
	Visible string                 `json:"visible"`
}

func TestDescriptorSnapshotUsesJSONForIgnoredStructState(t *testing.T) {
	value := &descriptorHiddenCycle{Visible: "original"}
	value.Next = value
	cloned, serializable := cloneRegistrySchema(map[string]any{"opaque": value})
	require.True(t, serializable, "a graph ignored by JSON must not introduce a private snapshot gate")
	value.Visible = "mutated"
	encoded, err := json.Marshal(cloned)
	require.NoError(t, err)
	assert.JSONEq(t, `{"opaque":{"visible":"original"}}`, string(encoded))
}

type descriptorPointerNumber int

func (value *descriptorPointerNumber) MarshalJSON() ([]byte, error) {
	captured := int(*value)
	*value++
	return json.Marshal(captured)
}

func TestDescriptorSnapshotFreezesAddressablePointerMarshalers(t *testing.T) {
	values := []descriptorPointerNumber{7}
	cloned, serializable := cloneRegistrySchema(map[string]any{"values": values})
	require.True(t, serializable)
	require.Equal(t, descriptorPointerNumber(8), values[0], "normal JSON serialization invokes an addressable pointer receiver")
	encoded, err := json.Marshal(cloned)
	require.NoError(t, err)
	assert.JSONEq(t, `{"values":[7]}`, string(encoded), "the snapshot must retain the first serialized output")
}

func TestDescriptorSnapshotPreservesJSONKeyNormalization(t *testing.T) {
	input := map[string]any{
		"bad\xff": json.RawMessage(`{"number":9007199254740993}`),
		"bad\xfe": json.RawMessage(`{"number":2}`),
	}
	want, err := json.Marshal(input)
	require.NoError(t, err)
	cloned, serializable := cloneRegistrySchema(input)
	require.True(t, serializable)
	got, err := json.Marshal(cloned)
	require.NoError(t, err)
	assert.Equal(t, string(want), string(got))
}
