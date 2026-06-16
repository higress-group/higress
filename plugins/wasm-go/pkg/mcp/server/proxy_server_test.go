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
)

// TestMcpProxyServerBasicInterface tests that McpProxyServer implements the Server interface
func TestMcpProxyServerBasicInterface(t *testing.T) {
	// This test will fail until we implement McpProxyServer
	server := NewMcpProxyServer("test-proxy")

	// Test Server interface implementation
	assert.NotNil(t, server)
	assert.Equal(t, "test-proxy", server.Name)

	// Test that it implements all required methods
	tools := server.GetMCPTools()
	assert.NotNil(t, tools)
	assert.Equal(t, 0, len(tools))

	// Test Clone method
	cloned := server.Clone()
	assert.NotNil(t, cloned)
}

// TestMcpProxyServerConfiguration tests configuration setting and getting
func TestMcpProxyServerConfiguration(t *testing.T) {
	server := NewMcpProxyServer("test-proxy")

	// Set server fields directly
	server.SetMcpServerURL("http://backend.example.com/mcp")
	server.SetTimeout(5000)

	// Add security scheme
	scheme := SecurityScheme{
		ID:   "test-auth",
		Type: "apiKey",
		In:   "header",
		Name: "X-API-Key",
	}
	server.AddSecurityScheme(scheme)

	// Verify server fields
	assert.Equal(t, "http://backend.example.com/mcp", server.GetMcpServerURL())
	assert.Equal(t, 5000, server.GetTimeout())

	// Verify security scheme
	retrievedScheme, exists := server.GetSecurityScheme("test-auth")
	assert.True(t, exists)
	assert.Equal(t, "test-auth", retrievedScheme.ID)
	assert.Equal(t, "apiKey", retrievedScheme.Type)
}

// TestMcpProxyServerAddTool tests adding proxy tools
func TestMcpProxyServerAddTool(t *testing.T) {
	server := NewMcpProxyServer("test-proxy")

	toolConfig := McpProxyToolConfig{
		Name:        "test-tool",
		Description: "Test tool for proxy",
		Args: []ToolArg{
			{
				Name:        "input",
				Description: "Test input",
				Type:        "string",
				Required:    true,
			},
		},
	}

	err := server.AddProxyTool(toolConfig)
	assert.NoError(t, err)

	tools := server.GetMCPTools()
	assert.Len(t, tools, 1)
	assert.Contains(t, tools, "test-tool")
}

// TestMcpProxyServerSecuritySchemes tests security scheme management
func TestMcpProxyServerSecuritySchemes(t *testing.T) {
	server := NewMcpProxyServer("test-proxy")

	scheme := SecurityScheme{
		ID:   "test-auth",
		Type: "apiKey",
		In:   "header",
		Name: "X-API-Key",
	}

	server.AddSecurityScheme(scheme)

	retrievedScheme, exists := server.GetSecurityScheme("test-auth")
	assert.True(t, exists)
	assert.Equal(t, scheme.ID, retrievedScheme.ID)
	assert.Equal(t, scheme.Type, retrievedScheme.Type)
}

// -----------------------------------------------------------------------------
// SetDefaultDownstreamSecurity / SetDefaultUpstreamSecurity / PassthroughAuth
// -----------------------------------------------------------------------------

func TestMcpProxyServer_SetGetDefaultDownstreamSecurity(t *testing.T) {
	s := NewMcpProxyServer("p")
	assert.Equal(t, "", s.GetDefaultDownstreamSecurity().ID, "fresh server has empty default")
	s.SetDefaultDownstreamSecurity(SecurityRequirement{ID: "K", Passthrough: true})
	got := s.GetDefaultDownstreamSecurity()
	assert.Equal(t, "K", got.ID)
	assert.True(t, got.Passthrough)
}

func TestMcpProxyServer_SetGetDefaultUpstreamSecurity(t *testing.T) {
	s := NewMcpProxyServer("p")
	s.SetDefaultUpstreamSecurity(SecurityRequirement{ID: "U", Credential: "c"})
	got := s.GetDefaultUpstreamSecurity()
	assert.Equal(t, "U", got.ID)
	assert.Equal(t, "c", got.Credential)
}

func TestMcpProxyServer_PassthroughAuthHeaderGetterAndSetter(t *testing.T) {
	s := NewMcpProxyServer("p")
	assert.False(t, s.GetPassthroughAuthHeader(), "default is false")
	s.SetPassthroughAuthHeader(true)
	assert.True(t, s.GetPassthroughAuthHeader())
	s.SetPassthroughAuthHeader(false)
	assert.False(t, s.GetPassthroughAuthHeader())
}

// -----------------------------------------------------------------------------
// AddSecurityScheme — nil-map branch
// -----------------------------------------------------------------------------

func TestMcpProxyServer_AddSecurityScheme_InitializesNilMap(t *testing.T) {
	// Skip the constructor so we can hit the `securitySchemes == nil` branch.
	s := &McpProxyServer{Name: "p"}
	s.AddSecurityScheme(SecurityScheme{ID: "K", Type: "apiKey", In: "header", Name: "X"})
	got, ok := s.GetSecurityScheme("K")
	require.True(t, ok)
	assert.Equal(t, "K", got.ID)
}

// -----------------------------------------------------------------------------
// AddMCPTool — delegates to BaseMCPServer
// -----------------------------------------------------------------------------

func TestMcpProxyServer_AddMCPTool_StoresInBaseAndReturnsSelf(t *testing.T) {
	s := NewMcpProxyServer("p")
	stub := &stubTool{desc: "d"}
	ret := s.AddMCPTool("custom", stub)
	assert.Same(t, s, ret, "AddMCPTool returns receiver for chaining")
	tools := s.GetMCPTools()
	got, ok := tools["custom"]
	require.True(t, ok)
	assert.Same(t, stub, got)
}

// -----------------------------------------------------------------------------
// AddProxyTool — overrides on duplicate name
// -----------------------------------------------------------------------------

func TestMcpProxyServer_AddProxyTool_DuplicateNameOverwrites(t *testing.T) {
	s := NewMcpProxyServer("p")
	require.NoError(t, s.AddProxyTool(McpProxyToolConfig{Name: "t", Description: "first"}))
	require.NoError(t, s.AddProxyTool(McpProxyToolConfig{Name: "t", Description: "second"}))

	tools := s.GetMCPTools()
	assert.Len(t, tools, 1, "duplicate AddProxyTool should overwrite, not duplicate")
	cfg, ok := s.GetToolConfig("t")
	require.True(t, ok)
	assert.Equal(t, "second", cfg.Description, "later AddProxyTool wins")
}

// -----------------------------------------------------------------------------
// GetToolConfig — hit and miss
// -----------------------------------------------------------------------------

func TestMcpProxyServer_GetToolConfig_HitAndMiss(t *testing.T) {
	s := NewMcpProxyServer("p")
	require.NoError(t, s.AddProxyTool(McpProxyToolConfig{Name: "t", Description: "d"}))

	cfg, ok := s.GetToolConfig("t")
	require.True(t, ok)
	assert.Equal(t, "d", cfg.Description)

	_, missOK := s.GetToolConfig("missing")
	assert.False(t, missOK)
}

// -----------------------------------------------------------------------------
// Clone — deep copy of toolsConfig and securitySchemes
// -----------------------------------------------------------------------------

func TestMcpProxyServer_Clone_DeepCopiesToolsConfigAndSchemes(t *testing.T) {
	orig := NewMcpProxyServer("orig")
	orig.SetMcpServerURL("http://b")
	orig.SetTimeout(1234)
	orig.SetTransport(TransportSSE)
	orig.SetPassthroughAuthHeader(true)
	orig.SetDefaultDownstreamSecurity(SecurityRequirement{ID: "K"})
	orig.AddSecurityScheme(SecurityScheme{ID: "K", Type: "apiKey", In: "header", Name: "X"})
	require.NoError(t, orig.AddProxyTool(McpProxyToolConfig{Name: "t", Description: "d"}))

	clonedI := orig.Clone()
	cloned, ok := clonedI.(*McpProxyServer)
	require.True(t, ok)
	require.NotSame(t, orig, cloned, "Clone must return a fresh struct")

	// Surface fields are copied.
	assert.Equal(t, orig.Name, cloned.Name)
	// NOTE: Clone does not propagate mcpServerURL/timeout/transport/passthrough
	// nor defaultDownstream/upstreamSecurity. That is intentional today (see
	// proxy_server.go:188): cloning is used for per-request isolation of
	// tool/security registries only. This test pins that contract — if Clone
	// starts copying those fields, update here and document the change.
	assert.Equal(t, "", cloned.GetMcpServerURL())

	// toolsConfig: deep copy — adding to clone doesn't bleed back to orig.
	require.NoError(t, cloned.AddProxyTool(McpProxyToolConfig{Name: "extra", Description: "x"}))
	_, origHasExtra := orig.GetToolConfig("extra")
	assert.False(t, origHasExtra, "tool added to clone must not appear in original")

	// securitySchemes: deep copy — replacing scheme on clone doesn't touch orig.
	cloned.AddSecurityScheme(SecurityScheme{ID: "K", Type: "http", Scheme: "bearer"})
	origScheme, _ := orig.GetSecurityScheme("K")
	clonedScheme, _ := cloned.GetSecurityScheme("K")
	assert.Equal(t, "apiKey", origScheme.Type, "original scheme must remain apiKey")
	assert.Equal(t, "http", clonedScheme.Type, "clone reflects the override")
}

// -----------------------------------------------------------------------------
// McpProxyTool — Description / InputSchema / OutputSchema / Create
// -----------------------------------------------------------------------------

func TestMcpProxyTool_DescriptionAndOutputSchema(t *testing.T) {
	tool := &McpProxyTool{
		toolConfig: McpProxyToolConfig{
			Description:  "describe me",
			OutputSchema: map[string]any{"type": "string"},
		},
	}
	assert.Equal(t, "describe me", tool.Description())
	assert.Equal(t, map[string]any{"type": "string"}, tool.OutputSchema())
}

func TestMcpProxyTool_InputSchema_RequiredAndOptionalAndEnumAndDefault(t *testing.T) {
	tool := &McpProxyTool{
		toolConfig: McpProxyToolConfig{
			Args: []ToolArg{
				{Name: "must", Type: "string", Description: "required", Required: true},
				{Name: "opt", Type: "integer", Description: "optional", Default: 7},
				{Name: "pick", Type: "string", Description: "enum", Enum: []interface{}{"a", "b"}},
			},
		},
	}
	schema := tool.InputSchema()
	assert.Equal(t, "object", schema["type"])
	required, ok := schema["required"].([]string)
	require.True(t, ok)
	assert.Equal(t, []string{"must"}, required, "only Required:true args land in required[]")

	props := schema["properties"].(map[string]any)
	mustProp := props["must"].(map[string]any)
	assert.Equal(t, "string", mustProp["type"])

	optProp := props["opt"].(map[string]any)
	assert.Equal(t, 7, optProp["default"])

	pickProp := props["pick"].(map[string]any)
	assert.Equal(t, []interface{}{"a", "b"}, pickProp["enum"])
}

func TestMcpProxyTool_InputSchema_NoArgs(t *testing.T) {
	tool := &McpProxyTool{toolConfig: McpProxyToolConfig{}}
	schema := tool.InputSchema()
	props, ok := schema["properties"].(map[string]any)
	require.True(t, ok)
	assert.Empty(t, props)
	required, ok := schema["required"].([]string)
	require.True(t, ok)
	assert.Empty(t, required)
}

func TestMcpProxyTool_Create_NewInstanceWithBoundArgs(t *testing.T) {
	orig := &McpProxyTool{
		serverName: "srv",
		name:       "t",
		toolConfig: McpProxyToolConfig{Name: "t"},
	}
	created := orig.Create([]byte(`{"q":"hello","n":7}`))
	require.NotSame(t, orig, created, "Create returns a fresh instance")
	cloned := created.(*McpProxyTool)
	assert.Equal(t, "srv", cloned.serverName)
	assert.Equal(t, "t", cloned.name)
	assert.Equal(t, "hello", cloned.arguments["q"])
	// JSON unmarshals numbers as float64.
	assert.Equal(t, float64(7), cloned.arguments["n"])
}

func TestMcpProxyTool_Create_EmptyParamsStillReturnsInstance(t *testing.T) {
	orig := &McpProxyTool{serverName: "s", name: "t"}
	created := orig.Create(nil)
	cloned := created.(*McpProxyTool)
	assert.Equal(t, "s", cloned.serverName)
	assert.Equal(t, "t", cloned.name)
	require.NotNil(t, cloned.arguments)
	assert.Empty(t, cloned.arguments, "no params → empty arguments map, not nil")
}

func TestMcpProxyTool_Create_MalformedJSON_PreservesEmptyArgs(t *testing.T) {
	orig := &McpProxyTool{serverName: "s", name: "t"}
	created := orig.Create([]byte(`{not json`))
	cloned := created.(*McpProxyTool)
	// json.Unmarshal silently fails; arguments stays empty.
	assert.Empty(t, cloned.arguments)
}

// -----------------------------------------------------------------------------
// ValidateSecurityScheme — full matrix
// -----------------------------------------------------------------------------

func TestValidateSecurityScheme(t *testing.T) {
	cases := []struct {
		name    string
		scheme  SecurityScheme
		wantErr string
	}{
		{"missing ID", SecurityScheme{Type: "apiKey", In: "header", Name: "X"}, "ID is required"},
		{"invalid type", SecurityScheme{ID: "k", Type: "oauth2"}, "invalid security scheme type"},
		{"apiKey missing name", SecurityScheme{ID: "k", Type: "apiKey", In: "header"}, "name is required"},
		{"apiKey invalid in", SecurityScheme{ID: "k", Type: "apiKey", In: "body", Name: "X"}, "invalid security scheme location"},
		{"apiKey ok header", SecurityScheme{ID: "k", Type: "apiKey", In: "header", Name: "X"}, ""},
		{"apiKey ok query", SecurityScheme{ID: "k", Type: "apiKey", In: "query", Name: "X"}, ""},
		{"apiKey ok cookie", SecurityScheme{ID: "k", Type: "apiKey", In: "cookie", Name: "X"}, ""},
		{"http missing scheme", SecurityScheme{ID: "k", Type: "http"}, "scheme is required for http"},
		{"http bearer ok", SecurityScheme{ID: "k", Type: "http", Scheme: "bearer"}, ""},
		{"http basic ok", SecurityScheme{ID: "k", Type: "http", Scheme: "basic"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateSecurityScheme(c.scheme)
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
// ValidateToolConfig — full matrix
// -----------------------------------------------------------------------------

func TestValidateToolConfig(t *testing.T) {
	cases := []struct {
		name    string
		config  McpProxyToolConfig
		wantErr string
	}{
		{
			"missing name",
			McpProxyToolConfig{Description: "d"},
			"tool name is required",
		},
		{
			"missing description",
			McpProxyToolConfig{Name: "t"},
			"tool description is required",
		},
		{
			"arg missing name",
			McpProxyToolConfig{Name: "t", Description: "d", Args: []ToolArg{{Type: "string", Description: "x"}}},
			"argument name is required",
		},
		{
			"arg duplicate names",
			McpProxyToolConfig{Name: "t", Description: "d", Args: []ToolArg{
				{Name: "a", Type: "string", Description: "x"},
				{Name: "a", Type: "string", Description: "y"},
			}},
			"duplicate argument name",
		},
		{
			"arg missing description",
			McpProxyToolConfig{Name: "t", Description: "d", Args: []ToolArg{{Name: "a", Type: "string"}}},
			"argument description is required",
		},
		{
			"arg invalid type",
			McpProxyToolConfig{Name: "t", Description: "d", Args: []ToolArg{{Name: "a", Type: "money", Description: "x"}}},
			"invalid argument type",
		},
		{
			"happy path with multiple typed args",
			McpProxyToolConfig{Name: "t", Description: "d", Args: []ToolArg{
				{Name: "s", Type: "string", Description: "x"},
				{Name: "n", Type: "number", Description: "x"},
				{Name: "i", Type: "integer", Description: "x"},
				{Name: "b", Type: "boolean", Description: "x"},
				{Name: "a", Type: "array", Description: "x"},
				{Name: "o", Type: "object", Description: "x"},
			}},
			"",
		},
		{
			"happy path no args",
			McpProxyToolConfig{Name: "t", Description: "d"},
			"",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateToolConfig(c.config)
			if c.wantErr == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), c.wantErr)
			}
		})
	}
}
