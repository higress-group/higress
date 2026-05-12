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
	"fmt"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

// McpProxyConfig represents the configuration for MCP proxy server
// Note: mcpServerURL, timeout, defaultDownstreamSecurity, and defaultUpstreamSecurity
// are now direct server fields, not part of this config structure
type McpProxyConfig struct {
	// This structure is kept for any additional server configuration that may be needed in the future
	// Currently, most configuration is handled as direct server fields
}

// TransportProtocol represents the transport protocol type for MCP proxy
type TransportProtocol string

const (
	TransportHTTP TransportProtocol = "http" // StreamableHTTP protocol
	TransportSSE  TransportProtocol = "sse"  // SSE protocol
)

// ToolArg represents an argument for a proxy tool
type ToolArg struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Type        string        `json:"type"`
	Required    bool          `json:"required"`
	Default     interface{}   `json:"default,omitempty"`
	Enum        []interface{} `json:"enum,omitempty"`
}

// McpProxyToolConfig represents a tool configuration for MCP proxy
type McpProxyToolConfig struct {
	Name            string              `json:"name"`
	Description     string              `json:"description"`
	Security        SecurityRequirement `json:"security,omitempty"` // Tool-level security for MCP Client to MCP Server
	Args            []ToolArg           `json:"args"`
	OutputSchema    map[string]any      `json:"outputSchema,omitempty"` // Output schema for MCP Protocol Version 2025-06-18
	RequestTemplate RequestTemplate     `json:"requestTemplate,omitempty"`
}

// RequestTemplate defines request template configuration for proxy tools
type RequestTemplate struct {
	Security SecurityRequirement `json:"security,omitempty"`
}

// McpProxyServer implements Server interface for MCP-to-MCP proxy
type McpProxyServer struct {
	Name                      string
	base                      BaseMCPServer
	toolsConfig               map[string]McpProxyToolConfig
	securitySchemes           map[string]SecurityScheme
	defaultDownstreamSecurity SecurityRequirement // Default client-to-gateway authentication
	defaultUpstreamSecurity   SecurityRequirement // Default gateway-to-backend authentication
	mcpServerURL              string              // Backend MCP server URL
	timeout                   int                 // Request timeout in milliseconds
	transport                 TransportProtocol   // Transport protocol (http or sse)
	passthroughAuthHeader     bool                // If true, pass through Authorization header even without downstream security
}

// NewMcpProxyServer creates a new MCP proxy server
func NewMcpProxyServer(name string) *McpProxyServer {
	return &McpProxyServer{
		Name:            name,
		base:            NewBaseMCPServer(),
		toolsConfig:     make(map[string]McpProxyToolConfig),
		securitySchemes: make(map[string]SecurityScheme),
	}
}

// AddSecurityScheme adds a security scheme to the server's map
func (s *McpProxyServer) AddSecurityScheme(scheme SecurityScheme) {
	if s.securitySchemes == nil {
		s.securitySchemes = make(map[string]SecurityScheme)
	}
	s.securitySchemes[scheme.ID] = scheme
}

// GetSecurityScheme retrieves a security scheme by its ID from the map
func (s *McpProxyServer) GetSecurityScheme(id string) (SecurityScheme, bool) {
	scheme, ok := s.securitySchemes[id]
	return scheme, ok
}

// SetDefaultDownstreamSecurity sets the default downstream security configuration
func (s *McpProxyServer) SetDefaultDownstreamSecurity(security SecurityRequirement) {
	s.defaultDownstreamSecurity = security
}

// GetDefaultDownstreamSecurity gets the default downstream security configuration
func (s *McpProxyServer) GetDefaultDownstreamSecurity() SecurityRequirement {
	return s.defaultDownstreamSecurity
}

// SetDefaultUpstreamSecurity sets the default upstream security configuration
func (s *McpProxyServer) SetDefaultUpstreamSecurity(security SecurityRequirement) {
	s.defaultUpstreamSecurity = security
}

// GetDefaultUpstreamSecurity gets the default upstream security configuration
func (s *McpProxyServer) GetDefaultUpstreamSecurity() SecurityRequirement {
	return s.defaultUpstreamSecurity
}

// SetMcpServerURL sets the backend MCP server URL
func (s *McpProxyServer) SetMcpServerURL(url string) {
	s.mcpServerURL = url
}

// GetMcpServerURL gets the backend MCP server URL
func (s *McpProxyServer) GetMcpServerURL() string {
	return s.mcpServerURL
}

// SetTimeout sets the request timeout in milliseconds
func (s *McpProxyServer) SetTimeout(timeout int) {
	s.timeout = timeout
}

// GetTimeout gets the request timeout in milliseconds
func (s *McpProxyServer) GetTimeout() int {
	return s.timeout
}

// SetTransport sets the transport protocol
func (s *McpProxyServer) SetTransport(transport TransportProtocol) {
	s.transport = transport
}

// GetTransport gets the transport protocol
func (s *McpProxyServer) GetTransport() TransportProtocol {
	return s.transport
}

// AddMCPTool implements Server interface
func (s *McpProxyServer) AddMCPTool(name string, tool Tool) Server {
	s.base.AddMCPTool(name, tool)
	return s
}

// AddProxyTool adds a proxy tool configuration
func (s *McpProxyServer) AddProxyTool(toolConfig McpProxyToolConfig) error {
	s.toolsConfig[toolConfig.Name] = toolConfig
	s.base.AddMCPTool(toolConfig.Name, &McpProxyTool{
		serverName: s.Name,
		name:       toolConfig.Name,
		toolConfig: toolConfig,
	})
	return nil
}

// GetMCPTools implements Server interface
func (s *McpProxyServer) GetMCPTools() map[string]Tool {
	return s.base.GetMCPTools()
}

// SetConfig implements Server interface
func (s *McpProxyServer) SetConfig(config []byte) {
	s.base.SetConfig(config)
}

// GetConfig implements Server interface
func (s *McpProxyServer) GetConfig(v any) {
	s.base.GetConfig(v)
}

// Clone implements Server interface
func (s *McpProxyServer) Clone() Server {
	newServer := &McpProxyServer{
		Name:            s.Name,
		base:            s.base.CloneBase(),
		toolsConfig:     make(map[string]McpProxyToolConfig),
		securitySchemes: make(map[string]SecurityScheme),
	}
	for k, v := range s.toolsConfig {
		newServer.toolsConfig[k] = v
	}
	// Deep copy securitySchemes
	if s.securitySchemes != nil {
		for k, v := range s.securitySchemes {
			newServer.securitySchemes[k] = v
		}
	}
	return newServer
}

// GetToolConfig returns the proxy tool configuration for a given tool name
func (s *McpProxyServer) GetToolConfig(name string) (McpProxyToolConfig, bool) {
	config, ok := s.toolsConfig[name]
	return config, ok
}

// SetPassthroughAuthHeader sets the passthrough auth header flag
func (s *McpProxyServer) SetPassthroughAuthHeader(passthrough bool) {
	s.passthroughAuthHeader = passthrough
}

// GetPassthroughAuthHeader gets the passthrough auth header flag
func (s *McpProxyServer) GetPassthroughAuthHeader() bool {
	return s.passthroughAuthHeader
}

// ForwardToolsList forwards tools/list request to backend MCP server
func (s *McpProxyServer) ForwardToolsList(ctx HttpContext, cursor *string) error {
	wrapperCtx := ctx.(wrapper.HttpContext)

	// Handle default downstream security for tools/list requests
	// tools/list requests use server-level default authentication configuration
	passthroughCredential := ""
	downstreamSecurity := s.GetDefaultDownstreamSecurity()
	if downstreamSecurity.ID != "" {
		clientScheme, schemeOk := s.GetSecurityScheme(downstreamSecurity.ID)
		if !schemeOk {
			log.Warnf("Default downstream security scheme ID '%s' not found for tools/list request.", downstreamSecurity.ID)
		} else {
			// Extract and remove the credential from the incoming request
			extractedCred, err := ExtractAndRemoveIncomingCredential(clientScheme)
			if err != nil {
				log.Warnf("Failed to extract/remove incoming credential for tools/list using scheme %s: %v", clientScheme.ID, err)
			} else if extractedCred == "" {
				log.Debugf("No incoming credential found for tools/list using scheme %s for extraction/removal.", clientScheme.ID)
			}

			// Only use passthrough if explicitly configured
			if downstreamSecurity.Passthrough && extractedCred != "" {
				passthroughCredential = extractedCred
				log.Debugf("Passthrough credential set for tools/list request.")
			}
		}
	} else {
		// Fallback: Remove Authorization header if no downstream security is defined
		// This prevents downstream credentials from being mistakenly passed to upstream
		// Unless passthroughAuthHeader is explicitly set to true
		if !s.GetPassthroughAuthHeader() {
			proxywasm.RemoveHttpRequestHeader("Authorization")
		}
	}

	// Create protocol handler using server fields
	handler := NewMcpProtocolHandler(s.GetMcpServerURL(), s.GetTimeout())

	// Prepare authentication information for gateway-to-backend communication
	var authInfo *ProxyAuthInfo
	upstreamSecurity := s.GetDefaultUpstreamSecurity()
	if upstreamSecurity.ID != "" {
		authInfo = &ProxyAuthInfo{
			SecuritySchemeID:      upstreamSecurity.ID,
			PassthroughCredential: passthroughCredential,
			Server:                s,
		}
	}

	// This will handle initialization asynchronously if needed and use ActionPause/Resume
	return handler.ForwardToolsList(wrapperCtx, cursor, authInfo)
}

// McpProxyTool implements Tool interface for MCP-to-MCP proxy
type McpProxyTool struct {
	serverName string
	name       string
	toolConfig McpProxyToolConfig
	arguments  map[string]interface{}
}

// Create implements Tool interface
func (t *McpProxyTool) Create(params []byte) Tool {
	newTool := &McpProxyTool{
		serverName: t.serverName,
		name:       t.name,
		toolConfig: t.toolConfig,
		arguments:  make(map[string]interface{}),
	}

	if len(params) > 0 {
		json.Unmarshal(params, &newTool.arguments)
	}

	return newTool
}

// Call implements Tool interface - this is where the MCP protocol handling happens
func (t *McpProxyTool) Call(httpCtx HttpContext, server Server) error {
	ctx := httpCtx.(wrapper.HttpContext)

	// Get proxy server instance to access configuration
	proxyServer, ok := server.(*McpProxyServer)
	if !ok {
		return fmt.Errorf("server is not a McpProxyServer")
	}

	// Handle tool-level or default downstream security: extract credential for passthrough if configured
	// toolConfig.Security represents client-to-gateway authentication, falls back to server's defaultDownstreamSecurity
	passthroughCredential := ""
	var downstreamSecurity SecurityRequirement
	if t.toolConfig.Security.ID != "" {
		// Use tool-level security if configured
		downstreamSecurity = t.toolConfig.Security
		log.Debugf("Using tool-level downstream security for tool %s: %s", t.name, downstreamSecurity.ID)
	} else {
		// Fall back to server's default downstream security
		downstreamSecurity = proxyServer.GetDefaultDownstreamSecurity()
		if downstreamSecurity.ID != "" {
			log.Debugf("Using default downstream security for tool %s: %s", t.name, downstreamSecurity.ID)
		}
	}

	if downstreamSecurity.ID != "" {
		clientScheme, schemeOk := proxyServer.GetSecurityScheme(downstreamSecurity.ID)
		if !schemeOk {
			log.Warnf("Downstream security scheme ID '%s' not found for tool %s.", downstreamSecurity.ID, t.name)
		} else {
			// Extract and remove the credential from the incoming request
			extractedCred, err := ExtractAndRemoveIncomingCredential(clientScheme)
			if err != nil {
				log.Warnf("Failed to extract/remove incoming credential for tool %s using scheme %s: %v", t.name, clientScheme.ID, err)
			} else if extractedCred == "" {
				log.Debugf("No incoming credential found for tool %s using scheme %s for extraction/removal.", t.name, clientScheme.ID)
			}

			// Only use passthrough if explicitly configured
			if downstreamSecurity.Passthrough && extractedCred != "" {
				passthroughCredential = extractedCred
				log.Debugf("Passthrough credential set for tool %s.", t.name)
			}
		}
	} else {
		// Fallback: Remove Authorization header if no downstream security is defined
		// This prevents downstream credentials from being mistakenly passed to upstream
		// Unless passthroughAuthHeader is explicitly set to true
		if !proxyServer.GetPassthroughAuthHeader() {
			proxywasm.RemoveHttpRequestHeader("Authorization")
		}
	}

	// Create protocol handler using server fields
	handler := NewMcpProtocolHandler(proxyServer.GetMcpServerURL(), proxyServer.GetTimeout())

	// Prepare authentication information for gateway-to-backend communication
	// toolConfig.RequestTemplate.Security represents gateway-to-backend authentication, falls back to server's defaultUpstreamSecurity
	var authInfo *ProxyAuthInfo
	var upstreamSecurity SecurityRequirement
	if t.toolConfig.RequestTemplate.Security.ID != "" {
		// Use tool-level upstream security if configured
		upstreamSecurity = t.toolConfig.RequestTemplate.Security
		log.Debugf("Using tool-level upstream security for tool %s: %s", t.name, upstreamSecurity.ID)
	} else {
		// Fall back to server's default upstream security
		upstreamSecurity = proxyServer.GetDefaultUpstreamSecurity()
		if upstreamSecurity.ID != "" {
			log.Debugf("Using default upstream security for tool %s: %s", t.name, upstreamSecurity.ID)
		}
	}

	if upstreamSecurity.ID != "" {
		authInfo = &ProxyAuthInfo{
			SecuritySchemeID:      upstreamSecurity.ID,
			PassthroughCredential: passthroughCredential,
			Server:                proxyServer,
		}
	}

	// This will handle initialization asynchronously if needed and use ActionPause/Resume
	return handler.ForwardToolsCall(ctx, t.name, t.arguments, authInfo)
}

// Description implements Tool interface
func (t *McpProxyTool) Description() string {
	return t.toolConfig.Description
}

// InputSchema implements Tool interface
func (t *McpProxyTool) InputSchema() map[string]any {
	schema := map[string]any{
		"type":       "object",
		"properties": make(map[string]any),
		"required":   []string{},
	}

	properties := schema["properties"].(map[string]any)
	var required []string

	for _, arg := range t.toolConfig.Args {
		argSchema := map[string]any{
			"type":        arg.Type,
			"description": arg.Description,
		}

		if arg.Default != nil {
			argSchema["default"] = arg.Default
		}

		if len(arg.Enum) > 0 {
			argSchema["enum"] = arg.Enum
		}

		properties[arg.Name] = argSchema

		if arg.Required {
			required = append(required, arg.Name)
		}
	}

	schema["required"] = required
	return schema
}

// OutputSchema implements Tool interface (MCP Protocol Version 2025-06-18)
func (t *McpProxyTool) OutputSchema() map[string]any {
	return t.toolConfig.OutputSchema
}

// ValidateSecurityScheme validates a security scheme configuration
func ValidateSecurityScheme(scheme SecurityScheme) error {
	if scheme.ID == "" {
		return fmt.Errorf("security scheme ID is required")
	}

	if scheme.Type != "apiKey" && scheme.Type != "http" {
		return fmt.Errorf("invalid security scheme type: %s", scheme.Type)
	}

	if scheme.Type == "apiKey" {
		if scheme.Name == "" {
			return fmt.Errorf("security scheme name is required for apiKey type")
		}
		if scheme.In != "header" && scheme.In != "query" && scheme.In != "cookie" {
			return fmt.Errorf("invalid security scheme location: %s", scheme.In)
		}
	}

	if scheme.Type == "http" {
		if scheme.Scheme == "" {
			return fmt.Errorf("security scheme scheme is required for http type")
		}
	}

	return nil
}

// ValidateToolConfig validates a tool configuration
func ValidateToolConfig(config McpProxyToolConfig) error {
	if config.Name == "" {
		return fmt.Errorf("tool name is required")
	}

	if config.Description == "" {
		return fmt.Errorf("tool description is required")
	}

	// Validate arguments
	argNames := make(map[string]bool)
	for _, arg := range config.Args {
		if arg.Name == "" {
			return fmt.Errorf("argument name is required")
		}

		if argNames[arg.Name] {
			return fmt.Errorf("duplicate argument name: %s", arg.Name)
		}
		argNames[arg.Name] = true

		if arg.Description == "" {
			return fmt.Errorf("argument description is required for %s", arg.Name)
		}

		validTypes := []string{"string", "number", "integer", "boolean", "array", "object"}
		validType := false
		for _, t := range validTypes {
			if arg.Type == t {
				validType = true
				break
			}
		}
		if !validType {
			return fmt.Errorf("invalid argument type %s for %s", arg.Type, arg.Name)
		}
	}

	return nil
}
