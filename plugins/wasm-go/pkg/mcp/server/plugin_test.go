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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// -----------------------------------------------------------------------------
// validateURL
// -----------------------------------------------------------------------------

func TestValidateURL(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		wantErr string // empty = expect no error
	}{
		{"empty string", "", "cannot be empty"},
		{"path only", "/api/foo", ""},
		{"http with host", "http://backend.example/mcp", ""},
		{"https with host", "https://backend.example/mcp", ""},
		{"http with userinfo", "http://user:pass@backend.example/mcp", ""},
		{"http with port", "http://backend.example:8080/mcp", ""},
		{"scheme without host", "http://", "must include a host"},
		{"unsupported scheme ftp", "ftp://example/x", "unsupported URL scheme"},
		{"unsupported scheme ws", "ws://example/x", "unsupported URL scheme"},
		{"contains space - parse error", "http://exa mple/x", "invalid URL format"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateURL(c.in)
			if c.wantErr == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), c.wantErr)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// computeEffectiveAllowToolsFromHeader (4-way matrix + edge cases)
// -----------------------------------------------------------------------------

func TestComputeEffectiveAllowToolsFromHeader_BothAbsent(t *testing.T) {
	got := computeEffectiveAllowToolsFromHeader(nil, "", false)
	assert.Nil(t, got, "no restrictions on either side → allow all (nil)")
}

func TestComputeEffectiveAllowToolsFromHeader_HeaderOnly(t *testing.T) {
	got := computeEffectiveAllowToolsFromHeader(nil, "a,b,c", true)
	require.NotNil(t, got)
	assert.Len(t, *got, 3)
	_, hasA := (*got)["a"]
	_, hasB := (*got)["b"]
	_, hasC := (*got)["c"]
	assert.True(t, hasA && hasB && hasC)
}

func TestComputeEffectiveAllowToolsFromHeader_ConfigOnly(t *testing.T) {
	cfg := map[string]struct{}{"x": {}, "y": {}}
	got := computeEffectiveAllowToolsFromHeader(&cfg, "", false)
	require.NotNil(t, got)
	assert.Equal(t, &cfg, got, "config restrictions returned as-is when header absent")
}

func TestComputeEffectiveAllowToolsFromHeader_BothPresent_Intersection(t *testing.T) {
	cfg := map[string]struct{}{"a": {}, "b": {}, "c": {}}
	got := computeEffectiveAllowToolsFromHeader(&cfg, "b,c,d", true)
	require.NotNil(t, got)
	assert.Len(t, *got, 2)
	_, hasB := (*got)["b"]
	_, hasC := (*got)["c"]
	_, hasD := (*got)["d"]
	_, hasA := (*got)["a"]
	assert.True(t, hasB && hasC, "intersection keeps common entries")
	assert.False(t, hasD, "header-only entries are dropped")
	assert.False(t, hasA, "config-only entries are dropped")
}

func TestComputeEffectiveAllowToolsFromHeader_HeaderEmptyStringButPresent(t *testing.T) {
	// headerExists=true with empty string → produces empty map (deny all)
	cfg := map[string]struct{}{"x": {}}
	got := computeEffectiveAllowToolsFromHeader(&cfg, "", true)
	require.NotNil(t, got)
	assert.Empty(t, *got, "empty header with headerExists=true intersects to empty set")
}

func TestComputeEffectiveAllowToolsFromHeader_HeaderWhitespaceAndDuplicates(t *testing.T) {
	got := computeEffectiveAllowToolsFromHeader(nil, "  a  , b ,  ,a, c  ,", true)
	require.NotNil(t, got)
	// "a", "b", "c" — duplicates and empties dropped
	assert.Len(t, *got, 3)
	for _, k := range []string{"a", "b", "c"} {
		_, ok := (*got)[k]
		assert.True(t, ok, "missing %q", k)
	}
}

// -----------------------------------------------------------------------------
// McpServerConfig accessors
// -----------------------------------------------------------------------------

func TestMcpServerConfig_GetServerName(t *testing.T) {
	c := &McpServerConfig{serverName: "my-server"}
	assert.Equal(t, "my-server", c.GetServerName())
}

func TestMcpServerConfig_GetIsComposed(t *testing.T) {
	c1 := &McpServerConfig{isComposed: false}
	c2 := &McpServerConfig{isComposed: true}
	assert.False(t, c1.GetIsComposed())
	assert.True(t, c2.GetIsComposed())
}

// -----------------------------------------------------------------------------
// GlobalToolRegistry — extra branches not covered elsewhere
// -----------------------------------------------------------------------------

// stubToolWithOutputSchema exercises the ToolWithOutputSchema dispatch in
// GlobalToolRegistry.RegisterTool.
type stubToolWithOutputSchema struct {
	desc   string
	input  map[string]any
	output map[string]any
}

func (s *stubToolWithOutputSchema) Create(_ []byte) Tool               { return s }
func (s *stubToolWithOutputSchema) Call(_ HttpContext, _ Server) error { return nil }
func (s *stubToolWithOutputSchema) Description() string                { return s.desc }
func (s *stubToolWithOutputSchema) InputSchema() map[string]any        { return s.input }
func (s *stubToolWithOutputSchema) OutputSchema() map[string]any       { return s.output }

func TestGlobalToolRegistry_RegisterTool_CapturesOutputSchema(t *testing.T) {
	r := &GlobalToolRegistry{}
	r.Initialize()
	r.RegisterTool("srv", "tool", &stubToolWithOutputSchema{
		desc:   "d",
		input:  map[string]any{"in": 1},
		output: map[string]any{"out": 2},
	})
	info, ok := r.GetToolInfo("srv", "tool")
	require.True(t, ok)
	assert.Equal(t, "d", info.Description)
	assert.Equal(t, map[string]any{"in": 1}, info.InputSchema)
	assert.Equal(t, map[string]any{"out": 2}, info.OutputSchema, "OutputSchema must be captured when tool implements ToolWithOutputSchema")
}

func TestGlobalToolRegistry_RegisterTool_PlainToolHasNoOutputSchema(t *testing.T) {
	r := &GlobalToolRegistry{}
	r.Initialize()
	r.RegisterTool("srv", "tool", &stubTool{desc: "d", input: map[string]any{"in": 1}})
	info, ok := r.GetToolInfo("srv", "tool")
	require.True(t, ok)
	assert.Nil(t, info.OutputSchema)
}

func TestGlobalToolRegistry_GetToolInfo_Misses(t *testing.T) {
	r := &GlobalToolRegistry{}
	r.Initialize()
	_, ok := r.GetToolInfo("missing", "any")
	assert.False(t, ok, "unknown server → not found")

	r.RegisterTool("srv", "real", &stubTool{desc: "d"})
	_, ok = r.GetToolInfo("srv", "missing")
	assert.False(t, ok, "unknown tool on known server → not found")
}

// -----------------------------------------------------------------------------
// AddMCPServer / addMCPServerOption.Apply
// -----------------------------------------------------------------------------

func TestAddMCPServer_FirstAndSecondAreStored(t *testing.T) {
	ctx := &Context{}
	AddMCPServer("alpha", &stubServer{}).Apply(ctx)
	AddMCPServer("beta", &stubServer{}).Apply(ctx)
	require.Len(t, ctx.servers, 2)
	_, hasA := ctx.servers["alpha"]
	_, hasB := ctx.servers["beta"]
	assert.True(t, hasA && hasB)
}

func TestAddMCPServer_DuplicateNamePanics(t *testing.T) {
	ctx := &Context{}
	AddMCPServer("dup", &stubServer{}).Apply(ctx)
	assert.PanicsWithValue(t,
		"Conflict! There is a mcp server with the same name:dup",
		func() { AddMCPServer("dup", &stubServer{}).Apply(ctx) })
}

// stubServer satisfies the Server interface for AddMCPServer tests.
type stubServer struct {
	cfg []byte
}

func (s *stubServer) AddMCPTool(_ string, _ Tool) Server { return s }
func (s *stubServer) GetMCPTools() map[string]Tool       { return map[string]Tool{} }
func (s *stubServer) SetConfig(c []byte)                 { s.cfg = c }
func (s *stubServer) GetConfig(_ any)                    {}
func (s *stubServer) Clone() Server                      { copy := *s; return &copy }

// -----------------------------------------------------------------------------
// ToInputSchema — exercises jsonschema.Reflect dispatch
// -----------------------------------------------------------------------------

type sampleStruct struct {
	Name  string   `json:"name"`
	Count int      `json:"count"`
	Tags  []string `json:"tags"`
}

func TestToInputSchema_StructByValue(t *testing.T) {
	out := ToInputSchema(sampleStruct{})
	require.NotNil(t, out, "must return a populated schema map")
	// Reflected schema always has a top-level "properties" map.
	props, ok := out["properties"].(map[string]any)
	require.True(t, ok, "schema should have a properties object: %v", out)
	for _, k := range []string{"name", "count", "tags"} {
		_, exists := props[k]
		assert.True(t, exists, "missing property %q", k)
	}
}

func TestToInputSchema_StructByPointer(t *testing.T) {
	// The function dereferences pointer types before name lookup.
	out := ToInputSchema(&sampleStruct{})
	require.NotNil(t, out)
	_, ok := out["properties"]
	assert.True(t, ok, "pointer-to-struct should resolve to the same schema")
}

// -----------------------------------------------------------------------------
// setupMcpProxyServer — error paths
// -----------------------------------------------------------------------------

func mustGJSON(t *testing.T, raw string) gjson.Result {
	t.Helper()
	r := gjson.Parse(raw)
	require.True(t, r.Exists(), "raw must parse: %s", raw)
	return r
}

func TestSetupMcpProxyServer_MissingTransport(t *testing.T) {
	j := mustGJSON(t, `{"name":"s","type":"mcp-proxy","mcpServerURL":"http://b"}`)
	_, err := setupMcpProxyServer("s", j, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "transport")
}

func TestSetupMcpProxyServer_InvalidTransport(t *testing.T) {
	j := mustGJSON(t, `{"transport":"grpc","mcpServerURL":"http://b"}`)
	_, err := setupMcpProxyServer("s", j, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid transport value")
}

func TestSetupMcpProxyServer_MissingMcpServerURL(t *testing.T) {
	j := mustGJSON(t, `{"transport":"http"}`)
	_, err := setupMcpProxyServer("s", j, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mcpServerURL is required")
}

func TestSetupMcpProxyServer_InvalidMcpServerURL(t *testing.T) {
	j := mustGJSON(t, `{"transport":"http","mcpServerURL":"ws://nope"}`)
	_, err := setupMcpProxyServer("s", j, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid mcpServerURL")
}

func TestSetupMcpProxyServer_BadSecuritySchemeJson(t *testing.T) {
	j := mustGJSON(t, `{
		"transport":"http",
		"mcpServerURL":"http://b",
		"securitySchemes":[123]
	}`)
	_, err := setupMcpProxyServer("s", j, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "security scheme")
}

func TestSetupMcpProxyServer_BadDefaultDownstreamSecurity(t *testing.T) {
	j := mustGJSON(t, `{
		"transport":"http",
		"mcpServerURL":"http://b",
		"defaultDownstreamSecurity": 42
	}`)
	_, err := setupMcpProxyServer("s", j, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "defaultDownstreamSecurity")
}

func TestSetupMcpProxyServer_BadDefaultUpstreamSecurity(t *testing.T) {
	j := mustGJSON(t, `{
		"transport":"http",
		"mcpServerURL":"http://b",
		"defaultUpstreamSecurity": "not-an-object"
	}`)
	_, err := setupMcpProxyServer("s", j, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "defaultUpstreamSecurity")
}

func TestSetupMcpProxyServer_HappyPath_AppliesAllFields(t *testing.T) {
	raw := `{
		"transport":"sse",
		"mcpServerURL":"https://backend.example/mcp",
		"timeout":7777,
		"passthroughAuthHeader":true,
		"securitySchemes":[
			{"id":"K","type":"apiKey","in":"header","name":"X-K","defaultCredential":"d"}
		],
		"defaultDownstreamSecurity":{"id":"K"},
		"defaultUpstreamSecurity":{"id":"K"}
	}`
	srv, err := setupMcpProxyServer("alpha", mustGJSON(t, raw), `{"cfg":1}`)
	require.NoError(t, err)
	require.NotNil(t, srv)

	assert.Equal(t, "alpha", srv.Name)
	assert.Equal(t, TransportSSE, srv.GetTransport())
	assert.Equal(t, "https://backend.example/mcp", srv.GetMcpServerURL())
	assert.Equal(t, 7777, srv.GetTimeout())
	assert.True(t, srv.GetPassthroughAuthHeader())
	_, ok := srv.GetSecurityScheme("K")
	assert.True(t, ok)
	assert.Equal(t, "K", srv.GetDefaultDownstreamSecurity().ID)
	assert.Equal(t, "K", srv.GetDefaultUpstreamSecurity().ID)
}

// -----------------------------------------------------------------------------
// parseConfigCore — error / branch paths via ParseConfigCore
// -----------------------------------------------------------------------------

func newValidationOpts() *ConfigOptions {
	r := &GlobalToolRegistry{}
	r.Initialize()
	return &ConfigOptions{
		Servers:                  make(map[string]Server),
		ToolRegistry:             r,
		SkipPreRegisteredServers: false,
	}
}

func TestParseConfigCore_NoServerOrToolSet(t *testing.T) {
	c := &McpServerConfig{}
	err := ParseConfigCore(gjson.Parse(`{}`), c, newValidationOpts())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "'server' or 'toolSet'")
}

func TestParseConfigCore_SingleServer_MissingName(t *testing.T) {
	c := &McpServerConfig{}
	err := ParseConfigCore(gjson.Parse(`{"server":{"type":"rest"}}`), c, newValidationOpts())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server.name")
}

func TestParseConfigCore_PreRegisteredNotInRegistry(t *testing.T) {
	// type=="" defaults to "rest", but with no tools and no entry in opts.Servers,
	// falls into the "pre-registered" branch which fails with "not registered".
	c := &McpServerConfig{}
	err := ParseConfigCore(gjson.Parse(`{"server":{"name":"ghost"}}`), c, newValidationOpts())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not registered")
}

func TestParseConfigCore_PreRegisteredSkipped(t *testing.T) {
	c := &McpServerConfig{}
	opts := newValidationOpts()
	opts.SkipPreRegisteredServers = true
	err := ParseConfigCore(gjson.Parse(`{"server":{"name":"ghost"}}`), c, opts)
	require.NoError(t, err, "skip flag should bypass the not-registered error")
	assert.Equal(t, "ghost", c.GetServerName())
	assert.Nil(t, c.server, "no server instance is constructed in skip mode")
}

func TestParseConfigCore_PreRegisteredFound(t *testing.T) {
	c := &McpServerConfig{}
	opts := newValidationOpts()
	opts.Servers["found"] = &stubServer{}
	err := ParseConfigCore(gjson.Parse(`{"server":{"name":"found"}}`), c, opts)
	require.NoError(t, err)
	require.NotNil(t, c.server, "Clone() of pre-registered server should be stored")
}

func TestParseConfigCore_McpProxy_BubblesUpSetupError(t *testing.T) {
	c := &McpServerConfig{}
	err := ParseConfigCore(gjson.Parse(`{
		"server":{"name":"p","type":"mcp-proxy","transport":"http"}
	}`), c, newValidationOpts())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mcpServerURL")
}

func TestParseConfigCore_McpProxy_HappyPath_NoTools(t *testing.T) {
	c := &McpServerConfig{}
	err := ParseConfigCore(gjson.Parse(`{
		"server":{
			"name":"p",
			"type":"mcp-proxy",
			"transport":"http",
			"mcpServerURL":"http://b"
		}
	}`), c, newValidationOpts())
	require.NoError(t, err)
	require.NotNil(t, c.server)
	assert.Equal(t, "p", c.GetServerName())
	assert.False(t, c.GetIsComposed())
	// Method handlers are populated for all servers.
	assert.NotNil(t, c.methodHandlers["ping"])
	assert.NotNil(t, c.methodHandlers["initialize"])
	assert.NotNil(t, c.methodHandlers["tools/list"])
	assert.NotNil(t, c.methodHandlers["tools/call"])
}

func TestParseConfigCore_McpProxy_BadProxyToolJson(t *testing.T) {
	c := &McpServerConfig{}
	err := ParseConfigCore(gjson.Parse(`{
		"server":{
			"name":"p",
			"type":"mcp-proxy",
			"transport":"http",
			"mcpServerURL":"http://b"
		},
		"tools":[42]
	}`), c, newValidationOpts())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "proxy tool")
}

func TestParseConfigCore_REST_BadToolJson(t *testing.T) {
	c := &McpServerConfig{}
	err := ParseConfigCore(gjson.Parse(`{
		"server":{"name":"r"},
		"tools":[42]
	}`), c, newValidationOpts())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tool config")
}

func TestParseConfigCore_REST_BadSecuritySchemeJson(t *testing.T) {
	c := &McpServerConfig{}
	err := ParseConfigCore(gjson.Parse(`{
		"server":{
			"name":"r",
			"securitySchemes":[42]
		},
		"tools":[
			{"name":"t","description":"d","requestTemplate":{"url":"/x","method":"GET"}}
		]
	}`), c, newValidationOpts())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "security scheme")
}

func TestParseConfigCore_REST_BadDefaultDownstreamSecurity(t *testing.T) {
	c := &McpServerConfig{}
	err := ParseConfigCore(gjson.Parse(`{
		"server":{
			"name":"r",
			"defaultDownstreamSecurity":42
		},
		"tools":[
			{"name":"t","description":"d","requestTemplate":{"url":"/x","method":"GET"}}
		]
	}`), c, newValidationOpts())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "defaultDownstreamSecurity")
}

func TestParseConfigCore_REST_BadDefaultUpstreamSecurity(t *testing.T) {
	c := &McpServerConfig{}
	err := ParseConfigCore(gjson.Parse(`{
		"server":{
			"name":"r",
			"defaultUpstreamSecurity":"oops"
		},
		"tools":[
			{"name":"t","description":"d","requestTemplate":{"url":"/x","method":"GET"}}
		]
	}`), c, newValidationOpts())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "defaultUpstreamSecurity")
}

func TestParseConfigCore_REST_AddRestToolError(t *testing.T) {
	// Setting two of {argsToJsonBody, argsToUrlParam, argsToFormBody} bubbles
	// the parseTemplates error out through AddRestTool.
	c := &McpServerConfig{}
	err := ParseConfigCore(gjson.Parse(`{
		"server":{"name":"r"},
		"tools":[
			{
				"name":"bad",
				"description":"d",
				"requestTemplate":{
					"url":"/x",
					"method":"POST",
					"argsToJsonBody":true,
					"argsToFormBody":true
				}
			}
		]
	}`), c, newValidationOpts())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "argsTo")
}

func TestParseConfigCore_REST_HappyPath_AllowToolsParsed(t *testing.T) {
	c := &McpServerConfig{}
	err := ParseConfigCore(gjson.Parse(`{
		"server":{"name":"r"},
		"tools":[
			{"name":"t1","description":"d","requestTemplate":{"url":"/x","method":"GET"}},
			{"name":"t2","description":"d","requestTemplate":{"url":"/y","method":"GET"}}
		],
		"allowTools":["t1"]
	}`), c, newValidationOpts())
	require.NoError(t, err)
	require.NotNil(t, c.server)
	assert.Equal(t, "r", c.GetServerName())
	assert.False(t, c.GetIsComposed())
}

func TestParseConfigCore_ToolSet_BadJson(t *testing.T) {
	c := &McpServerConfig{}
	err := ParseConfigCore(gjson.Parse(`{"toolSet":42}`), c, newValidationOpts())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "toolSet")
}

func TestParseConfigCore_ToolSet_HappyPath(t *testing.T) {
	opts := newValidationOpts()
	// Register one tool so the composed server has something to aggregate.
	opts.ToolRegistry.RegisterTool("alpha", "search", &stubTool{
		desc:  "alpha search",
		input: map[string]any{"type": "object"},
	})
	c := &McpServerConfig{}
	err := ParseConfigCore(gjson.Parse(`{
		"toolSet":{
			"name":"compound",
			"serverTools":[{"serverName":"alpha","tools":["search"]}]
		}
	}`), c, opts)
	require.NoError(t, err)
	assert.True(t, c.GetIsComposed(), "toolSet must produce a composed server")
	assert.Equal(t, "compound", c.GetServerName(), "composed server uses toolSet.name as serverName")
	require.NotNil(t, c.server)
}

// -----------------------------------------------------------------------------
// GetServerFromGlobalContext — exercises the package-level singleton
// -----------------------------------------------------------------------------

func TestGetServerFromGlobalContext(t *testing.T) {
	// Snapshot and restore globalContext to keep tests independent.
	saved := globalContext
	defer func() { globalContext = saved }()
	globalContext = Context{servers: map[string]Server{
		"existing": &stubServer{},
	}}

	got, ok := GetServerFromGlobalContext("existing")
	require.True(t, ok)
	assert.NotNil(t, got)

	_, miss := GetServerFromGlobalContext("missing")
	assert.False(t, miss)
}

// -----------------------------------------------------------------------------
// BaseMCPServer.Clone / CloneBase
// -----------------------------------------------------------------------------

func TestBaseMCPServer_Clone_PanicsByContract(t *testing.T) {
	// Derived types must implement Clone; BaseMCPServer panics to enforce that.
	b := NewBaseMCPServer()
	assert.PanicsWithValue(t,
		"Clone method must be implemented by derived types",
		func() { _ = b.Clone() })
}

func TestBaseMCPServer_CloneBase_DeepCopiesTools(t *testing.T) {
	b := NewBaseMCPServer()
	b.SetConfig([]byte(`{"k":1}`))
	stub := &stubTool{desc: "t"}
	b.AddMCPTool("a", stub)

	cloned := b.CloneBase()

	// Mutating clone's tools must not bleed into the original.
	cloned.AddMCPTool("b", &stubTool{desc: "x"})
	assert.Len(t, b.GetMCPTools(), 1, "original tools must remain untouched")
	assert.Len(t, cloned.GetMCPTools(), 2)

	// Same config bytes are preserved.
	got, ok := cloned.GetMCPTools()["a"]
	require.True(t, ok)
	assert.Same(t, stub, got, "existing tools are shared by reference (no deep clone of Tool itself)")
}
