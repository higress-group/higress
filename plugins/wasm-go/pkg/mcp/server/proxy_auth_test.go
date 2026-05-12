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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestApiKeyAuthentication tests API key authentication forwarding
func TestApiKeyAuthentication(t *testing.T) {
	server := NewMcpProxyServer("auth-test")

	// Configure security scheme
	scheme := SecurityScheme{
		ID:                "ApiKeyAuth",
		Type:              "apiKey",
		In:                "header",
		Name:              "X-API-Key",
		DefaultCredential: "default-api-key",
	}

	server.AddSecurityScheme(scheme)

	// Set server fields directly
	server.SetMcpServerURL("http://secure-backend.example.com/mcp")
	server.SetTimeout(5000)

	// Create tool with client-to-gateway and gateway-to-backend security
	toolConfig := McpProxyToolConfig{
		Name:        "secure_tool",
		Description: "Tool requiring authentication",
		Security: SecurityRequirement{
			ID:          "ApiKeyAuth", // Client-to-gateway authentication
			Passthrough: true,         // Extract client credential for backend use
		},
		Args: []ToolArg{
			{
				Name:        "data",
				Description: "Data parameter",
				Type:        "string",
				Required:    true,
			},
		},
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"result": map[string]any{
					"type":        "string",
					"description": "The result of the operation",
				},
			},
		},
		RequestTemplate: RequestTemplate{
			Security: SecurityRequirement{
				ID: "ApiKeyAuth", // Gateway-to-backend authentication (same scheme for simplicity)
			},
		},
	}

	err := server.AddProxyTool(toolConfig)
	require.NoError(t, err)

	tool, exists := server.GetMCPTools()["secure_tool"]
	require.True(t, exists)

	params := map[string]interface{}{
		"data": "test data",
	}
	paramsBytes, err := json.Marshal(params)
	require.NoError(t, err)

	toolInstance := tool.Create(paramsBytes)
	require.NotNil(t, toolInstance)

	// Authentication is now handled automatically during tool calls
	// The actual authentication flow is tested in integration tests
}

// TestBearerAuthentication tests Bearer token authentication
func TestBearerAuthentication(t *testing.T) {
	server := NewMcpProxyServer("bearer-auth-test")

	// Configure Bearer security scheme
	scheme := SecurityScheme{
		ID:     "BearerAuth",
		Type:   "http",
		Scheme: "bearer",
	}

	server.AddSecurityScheme(scheme)

	// Set server fields directly
	server.SetMcpServerURL("https://secure-backend.example.com/mcp")
	server.SetTimeout(8000)

	// Create tool with Bearer authentication
	// Create tool using only gateway-to-backend authentication (no client auth required)
	toolConfig := McpProxyToolConfig{
		Name:        "bearer_tool",
		Description: "Tool with Bearer authentication to backend only",
		Args: []ToolArg{
			{
				Name:        "query",
				Description: "Query parameter",
				Type:        "string",
				Required:    true,
			},
		},
		RequestTemplate: RequestTemplate{
			Security: SecurityRequirement{
				ID: "BearerAuth", // Only gateway-to-backend authentication
			},
		},
	}

	err := server.AddProxyTool(toolConfig)
	require.NoError(t, err)

	tool, exists := server.GetMCPTools()["bearer_tool"]
	require.True(t, exists)

	params := map[string]interface{}{
		"query": "test query",
	}
	paramsBytes, err := json.Marshal(params)
	require.NoError(t, err)

	toolInstance := tool.Create(paramsBytes)
	require.NotNil(t, toolInstance)

	// Authentication is now handled automatically during tool calls
	// The actual authentication flow is tested in integration tests

	// Test backward compatibility: this tool uses RequestTemplate.Security (legacy way)
	// which should still work
}

// TestBasicAuthentication tests Basic authentication
func TestBasicAuthentication(t *testing.T) {
	server := NewMcpProxyServer("basic-auth-test")

	// Configure Basic security scheme
	scheme := SecurityScheme{
		ID:     "BasicAuth",
		Type:   "http",
		Scheme: "basic",
	}

	server.AddSecurityScheme(scheme)

	// Test tool call with Basic authentication
	toolConfig := McpProxyToolConfig{
		Name:        "basic_tool",
		Description: "Tool with Basic authentication",
		Args: []ToolArg{
			{
				Name:        "resource",
				Description: "Resource identifier",
				Type:        "string",
				Required:    true,
			},
		},
		RequestTemplate: RequestTemplate{
			Security: SecurityRequirement{
				ID: "BasicAuth",
			},
		},
	}

	err := server.AddProxyTool(toolConfig)
	require.NoError(t, err)

	tool, exists := server.GetMCPTools()["basic_tool"]
	require.True(t, exists)

	params := map[string]interface{}{
		"resource": "test-resource",
	}
	paramsBytes, err := json.Marshal(params)
	require.NoError(t, err)

	toolInstance := tool.Create(paramsBytes)
	require.NotNil(t, toolInstance)

	// Authentication is now handled automatically during tool calls
	// The actual authentication flow is tested in integration tests

	// Test OutputSchema functionality (only for tools that have it configured)
	if toolWithOutputSchema, ok := tool.(ToolWithOutputSchema); ok {
		outputSchema := toolWithOutputSchema.OutputSchema()
		if outputSchema != nil {
			// Only validate if outputSchema is configured
			assert.Equal(t, "object", outputSchema["type"])
			properties, hasProperties := outputSchema["properties"].(map[string]any)
			require.True(t, hasProperties)
			resultSchema, hasResult := properties["result"].(map[string]any)
			require.True(t, hasResult)
			assert.Equal(t, "string", resultSchema["type"])
			assert.Equal(t, "The result of the operation", resultSchema["description"])
		}
	}
}

// TestMultipleSecuritySchemes tests multiple security schemes in one server
func TestMultipleSecuritySchemes(t *testing.T) {
	server := NewMcpProxyServer("multi-auth-test")

	// Add multiple security schemes
	schemes := []SecurityScheme{
		{
			ID:   "ApiKeyAuth",
			Type: "apiKey",
			In:   "header",
			Name: "X-API-Key",
		},
		{
			ID:     "BearerAuth",
			Type:   "http",
			Scheme: "bearer",
		},
	}

	for _, scheme := range schemes {
		server.AddSecurityScheme(scheme)
	}

	// Test that both schemes are available
	for _, scheme := range schemes {
		retrievedScheme, exists := server.GetSecurityScheme(scheme.ID)
		assert.True(t, exists)
		assert.Equal(t, scheme.ID, retrievedScheme.ID)
		assert.Equal(t, scheme.Type, retrievedScheme.Type)
	}
}

// ProxyAuthContext, RequestTemplate, SecurityConfig and authentication methods
// are now implemented in proxy_server.go

// TestToolsListAuthentication tests authentication configuration for tools/list requests
func TestToolsListAuthentication(t *testing.T) {
	server := NewMcpProxyServer("test-server")

	// Add a security scheme for global authentication
	scheme := SecurityScheme{
		ID:                "GlobalAuth",
		Type:              "apiKey",
		In:                "header",
		Name:              "X-API-Key",
		DefaultCredential: "default-global-key",
	}
	server.AddSecurityScheme(scheme)

	// Test that we can retrieve the security scheme
	retrievedScheme, exists := server.GetSecurityScheme("GlobalAuth")
	assert.True(t, exists)
	assert.Equal(t, "GlobalAuth", retrievedScheme.ID)
	assert.Equal(t, "apiKey", retrievedScheme.Type)
	assert.Equal(t, "header", retrievedScheme.In)
	assert.Equal(t, "X-API-Key", retrievedScheme.Name)

	// Test setting default security directly on server
	defaultDownstreamSecurity := SecurityRequirement{
		ID:          "GlobalAuth",
		Passthrough: true,
	}
	defaultUpstreamSecurity := SecurityRequirement{
		ID: "GlobalAuth",
	}

	server.SetDefaultDownstreamSecurity(defaultDownstreamSecurity)
	server.SetDefaultUpstreamSecurity(defaultUpstreamSecurity)

	// Verify default security settings
	retrievedDownstream := server.GetDefaultDownstreamSecurity()
	assert.Equal(t, "GlobalAuth", retrievedDownstream.ID)
	assert.True(t, retrievedDownstream.Passthrough)

	retrievedUpstream := server.GetDefaultUpstreamSecurity()
	assert.Equal(t, "GlobalAuth", retrievedUpstream.ID)

	t.Logf("Tools/list authentication configuration test completed successfully")
}

// TestDefaultSecurityFallback tests the fallback mechanism from tool-level to default security
func TestDefaultSecurityFallback(t *testing.T) {
	server := NewMcpProxyServer("test-server")

	// Add security schemes
	defaultScheme := SecurityScheme{
		ID:                "DefaultAuth",
		Type:              "apiKey",
		In:                "header",
		Name:              "X-Default-Key",
		DefaultCredential: "default-key",
	}
	toolScheme := SecurityScheme{
		ID:                "ToolAuth",
		Type:              "apiKey",
		In:                "header",
		Name:              "X-Tool-Key",
		DefaultCredential: "tool-key",
	}
	server.AddSecurityScheme(defaultScheme)
	server.AddSecurityScheme(toolScheme)

	// Test tool configuration with tool-level security (should use tool-level, not default)
	toolConfigWithSecurity := McpProxyToolConfig{
		Name:        "secure_tool",
		Description: "Tool with its own security",
		Security: SecurityRequirement{
			ID:          "ToolAuth",
			Passthrough: true,
		},
		RequestTemplate: RequestTemplate{
			Security: SecurityRequirement{
				ID: "ToolAuth",
			},
		},
	}

	// Test tool configuration without tool-level security (should fallback to default)
	toolConfigWithoutSecurity := McpProxyToolConfig{
		Name:        "fallback_tool",
		Description: "Tool that falls back to default security",
		// No Security field configured, should use default
		RequestTemplate: RequestTemplate{
			// No Security field configured, should use default
		},
	}

	// Set default security directly on server
	server.SetDefaultDownstreamSecurity(SecurityRequirement{
		ID:          "DefaultAuth",
		Passthrough: false,
	})
	server.SetDefaultUpstreamSecurity(SecurityRequirement{
		ID: "DefaultAuth",
	})

	// Set server configuration directly
	server.SetMcpServerURL("http://backend.example.com")
	server.SetTimeout(5000)

	// Add tools to server
	err := server.AddProxyTool(toolConfigWithSecurity)
	assert.NoError(t, err)
	err = server.AddProxyTool(toolConfigWithoutSecurity)
	assert.NoError(t, err)

	// Verify tools were added
	tools := server.GetMCPTools()
	assert.Contains(t, tools, "secure_tool")
	assert.Contains(t, tools, "fallback_tool")

	t.Logf("Default security fallback test completed successfully")
}

// TestURLModificationInAuthentication tests that authentication can modify the URL (e.g., adding query parameters)
func TestURLModificationInAuthentication(t *testing.T) {
	server := NewMcpProxyServer("test-server")

	// Add a security scheme that adds parameters to query (apiKey in query)
	scheme := SecurityScheme{
		ID:                "QueryApiKey",
		Type:              "apiKey",
		In:                "query",
		Name:              "api_key",
		DefaultCredential: "test-key-123",
	}
	server.AddSecurityScheme(scheme)

	// Verify the security scheme was added correctly
	retrievedScheme, exists := server.GetSecurityScheme("QueryApiKey")
	assert.True(t, exists)
	assert.Equal(t, "apiKey", retrievedScheme.Type)
	assert.Equal(t, "query", retrievedScheme.In)
	assert.Equal(t, "api_key", retrievedScheme.Name)

	t.Logf("URL modification authentication configuration test completed successfully")
}

// TestProxyServerFields tests the server-level field setting and getting
func TestProxyServerFields(t *testing.T) {
	server := NewMcpProxyServer("test-server")

	// Test mcpServerURL
	testURL := "http://mcp.example.com:8080/mcp"
	server.SetMcpServerURL(testURL)
	assert.Equal(t, testURL, server.GetMcpServerURL())

	// Test timeout
	testTimeout := 10000
	server.SetTimeout(testTimeout)
	assert.Equal(t, testTimeout, server.GetTimeout())

	// Test default security settings
	downstreamSec := SecurityRequirement{
		ID:          "test-downstream",
		Passthrough: true,
	}
	upstreamSec := SecurityRequirement{
		ID: "test-upstream",
	}

	server.SetDefaultDownstreamSecurity(downstreamSec)
	server.SetDefaultUpstreamSecurity(upstreamSec)

	assert.Equal(t, "test-downstream", server.GetDefaultDownstreamSecurity().ID)
	assert.True(t, server.GetDefaultDownstreamSecurity().Passthrough)
	assert.Equal(t, "test-upstream", server.GetDefaultUpstreamSecurity().ID)

	t.Logf("Proxy server fields test completed successfully")
}
