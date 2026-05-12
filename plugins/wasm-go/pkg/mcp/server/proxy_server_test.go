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
