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
	"net/url"
	"reflect"
	"slices"
	"strings"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/invopop/jsonschema"
	"github.com/tidwall/gjson"

	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/utils"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

const (
	DefaultMaxBodyBytes   uint32 = 100 * 1024 * 1024
	GlobalToolRegistryKey        = "GlobalToolRegistry"
)

// SupportedMCPVersions contains all supported MCP protocol versions
var SupportedMCPVersions = []string{"2024-11-05", "2025-03-26", "2025-06-18"}

// validateURL validates that the given string is a valid URL
func validateURL(urlStr string) error {
	if urlStr == "" {
		return errors.New("url cannot be empty")
	}

	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid URL format: %v", err)
	}

	// Allow both full URLs (with scheme and host) and path-only URLs
	// Path-only URLs will be resolved against the cluster's base URL
	if parsedURL.Scheme != "" {
		// If scheme is provided, host must also be provided
		if parsedURL.Host == "" {
			return errors.New("url with scheme must include a host")
		}

		// Only allow http and https schemes for security
		if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
			return fmt.Errorf("unsupported URL scheme '%s', only http and https are allowed", parsedURL.Scheme)
		}
	}

	return nil
}

// setupMcpProxyServer creates and configures an MCP proxy server
func setupMcpProxyServer(serverName string, serverJson gjson.Result, serverConfigJsonForInstance string) (*McpProxyServer, error) {
	proxyServer := NewMcpProxyServer(serverName)
	proxyServer.SetConfig([]byte(serverConfigJsonForInstance))

	// Parse and validate transport (required for mcp-proxy)
	transportStr := serverJson.Get("transport").String()
	if transportStr == "" {
		return nil, errors.New("transport field is required for mcp-proxy server type")
	}
	transport := TransportProtocol(transportStr)
	if transport != TransportHTTP && transport != TransportSSE {
		return nil, fmt.Errorf("invalid transport value: %s, must be 'http' or 'sse'", transportStr)
	}
	proxyServer.SetTransport(transport)

	// Parse and validate mcpServerURL (required for mcp-proxy)
	mcpServerURL := serverJson.Get("mcpServerURL").String()
	if mcpServerURL == "" {
		return nil, errors.New("mcpServerURL is required for mcp-proxy server type")
	}
	if err := validateURL(mcpServerURL); err != nil {
		return nil, fmt.Errorf("invalid mcpServerURL: %v", err)
	}
	proxyServer.SetMcpServerURL(mcpServerURL)

	// Parse timeout (optional)
	timeout := serverJson.Get("timeout").Int()
	if timeout > 0 {
		proxyServer.SetTimeout(int(timeout))
	}

	// Parse passthroughAuthHeader (optional, defaults to false)
	passthroughAuthHeader := serverJson.Get("passthroughAuthHeader").Bool()
	proxyServer.SetPassthroughAuthHeader(passthroughAuthHeader)

	// Parse security schemes
	securitySchemesJson := serverJson.Get("securitySchemes")
	if securitySchemesJson.Exists() {
		for _, schemeJson := range securitySchemesJson.Array() {
			var scheme SecurityScheme
			if err := json.Unmarshal([]byte(schemeJson.Raw), &scheme); err != nil {
				return nil, fmt.Errorf("failed to parse security scheme config: %v", err)
			}
			proxyServer.AddSecurityScheme(scheme)
		}
	}

	// Parse default downstream security
	defaultDownstreamSecurityJson := serverJson.Get("defaultDownstreamSecurity")
	if defaultDownstreamSecurityJson.Exists() {
		var defaultDownstreamSecurity SecurityRequirement
		if err := json.Unmarshal([]byte(defaultDownstreamSecurityJson.Raw), &defaultDownstreamSecurity); err != nil {
			return nil, fmt.Errorf("failed to parse defaultDownstreamSecurity config: %v", err)
		}
		proxyServer.SetDefaultDownstreamSecurity(defaultDownstreamSecurity)
	}

	// Parse default upstream security
	defaultUpstreamSecurityJson := serverJson.Get("defaultUpstreamSecurity")
	if defaultUpstreamSecurityJson.Exists() {
		var defaultUpstreamSecurity SecurityRequirement
		if err := json.Unmarshal([]byte(defaultUpstreamSecurityJson.Raw), &defaultUpstreamSecurity); err != nil {
			return nil, fmt.Errorf("failed to parse defaultUpstreamSecurity config: %v", err)
		}
		proxyServer.SetDefaultUpstreamSecurity(defaultUpstreamSecurity)
	}

	return proxyServer, nil
}

type HttpContext wrapper.HttpContext

type Context struct {
	servers map[string]Server
}

type CtxOption interface {
	Apply(*Context)
}

var globalContext Context

// ToolInfo stores information about a tool for the global registry.
type ToolInfo struct {
	Name         string
	Description  string
	InputSchema  map[string]any
	OutputSchema map[string]any // New field for MCP Protocol Version 2025-06-18
	ServerName   string         // Original server name
	Tool         Tool           // The actual tool instance for cloning
}

// GlobalToolRegistry holds all tools from all servers.
type GlobalToolRegistry struct {
	// serverName -> toolName -> toolInfo
	serverTools map[string]map[string]ToolInfo
}

// Initialize initializes the GlobalToolRegistry
func (r *GlobalToolRegistry) Initialize() {
	r.serverTools = make(map[string]map[string]ToolInfo)
}

// RegisterTool registers a tool into the global registry.
func (r *GlobalToolRegistry) RegisterTool(serverName string, toolName string, tool Tool) {
	if _, ok := r.serverTools[serverName]; !ok {
		r.serverTools[serverName] = make(map[string]ToolInfo)
	}
	toolInfo := ToolInfo{
		Name:        toolName,
		Description: tool.Description(),
		InputSchema: tool.InputSchema(),
		ServerName:  serverName,
		Tool:        tool,
	}
	// Check if tool implements OutputSchema (MCP Protocol Version 2025-06-18)
	if toolWithSchema, ok := tool.(ToolWithOutputSchema); ok {
		toolInfo.OutputSchema = toolWithSchema.OutputSchema()
	}
	r.serverTools[serverName][toolName] = toolInfo
	log.Debugf("Registered tool %s/%s", serverName, toolName)
}

// GetToolInfo retrieves tool information from the global registry.
func (r *GlobalToolRegistry) GetToolInfo(serverName string, toolName string) (ToolInfo, bool) {
	if serverTools, ok := r.serverTools[serverName]; ok {
		toolInfo, found := serverTools[toolName]
		return toolInfo, found
	}
	return ToolInfo{}, false
}

func onPluginStartOrReload(context wrapper.PluginContext) error {
	toolRegistry := &GlobalToolRegistry{}
	toolRegistry.Initialize()
	context.SetContext(GlobalToolRegistryKey, toolRegistry)
	context.EnableRuleLevelConfigIsolation()
	return nil
}

// GetServer retrieves a server instance from the global context.
// This is needed by ComposedMCPServer to get original server instances.
func GetServerFromGlobalContext(serverName string) (Server, bool) {
	server, exist := globalContext.servers[serverName]
	return server, exist
}

type Server interface {
	AddMCPTool(name string, tool Tool) Server
	GetMCPTools() map[string]Tool // For single server, returns its tools. For composed, returns composed tools.
	SetConfig(config []byte)
	GetConfig(v any)
	Clone() Server
	// GetName() string // Returns the server name - REMOVED
}

type Tool interface {
	Create(params []byte) Tool
	Call(httpCtx HttpContext, server Server) error
	Description() string
	InputSchema() map[string]any
}

// ToolWithOutputSchema is an optional interface for tools that support output schema
// (MCP Protocol Version 2025-06-18). Tools can optionally implement this interface
// to provide output schema information.
type ToolWithOutputSchema interface {
	Tool
	OutputSchema() map[string]any
}

// ToolSetConfig defines the configuration for a toolset.
type ToolSetConfig struct {
	Name        string             `json:"name"`
	ServerTools []ServerToolConfig `json:"serverTools"`
}

// ServerToolConfig specifies which tools from a server to include in a toolset.
type ServerToolConfig struct {
	ServerName string   `json:"serverName"`
	Tools      []string `json:"tools"`
}

// ConfigOptions contains the dependencies needed for config parsing
type ConfigOptions struct {
	Servers      map[string]Server
	ToolRegistry *GlobalToolRegistry
	// Skip validation for pre-registered Go-based servers
	SkipPreRegisteredServers bool
}

type McpServerConfig struct {
	serverName     string // Store the server name directly
	server         Server // Can be a single server or a composed server
	methodHandlers utils.MethodHandlers
	toolSet        *ToolSetConfig // Parsed toolset configuration
	isComposed     bool
}

// GetServerName returns the server name for external access
func (c *McpServerConfig) GetServerName() string {
	return c.serverName
}

// GetIsComposed returns whether this is a composed server for external access
func (c *McpServerConfig) GetIsComposed() bool {
	return c.isComposed
}

// computeEffectiveAllowTools computes the effective allowTools by taking the intersection
// of config allowTools and request header allowTools.
// Returns nil if no restrictions (allow all), otherwise returns a pointer to the effective set.
func computeEffectiveAllowTools(configAllowTools *map[string]struct{}) *map[string]struct{} {
	// Get allowTools from request header
	allowToolsHeaderStr, _ := proxywasm.GetHttpRequestHeader("x-envoy-allow-mcp-tools")
	proxywasm.RemoveHttpRequestHeader("x-envoy-allow-mcp-tools")
	// Only consider header as "present" if it has non-empty value
	// Empty string means header is not set or explicitly empty, both treated as "no restriction"
	headerExists := allowToolsHeaderStr != ""
	return computeEffectiveAllowToolsFromHeader(configAllowTools, allowToolsHeaderStr, headerExists)
}

// computeEffectiveAllowToolsFromHeader computes the effective allowTools by taking the intersection
// of config allowTools and header allowTools string.
// This is useful when the header string is already extracted (e.g., in async callbacks).
// Returns nil if no restrictions (allow all), otherwise returns a pointer to the effective set.
func computeEffectiveAllowToolsFromHeader(configAllowTools *map[string]struct{}, allowToolsHeaderStr string, headerExists bool) *map[string]struct{} {
	var allowToolsFromHeader *map[string]struct{}
	if headerExists {
		// Header is present (even if empty string), parse it
		headerMap := make(map[string]struct{})
		for tool := range strings.SplitSeq(allowToolsHeaderStr, ",") {
			trimmedTool := strings.TrimSpace(tool)
			if trimmedTool == "" {
				continue
			}
			headerMap[trimmedTool] = struct{}{}
		}
		// Always create pointer even if map is empty, to distinguish from "not configured"
		allowToolsFromHeader = &headerMap
	}

	// Compute effective allowTools (intersection of config and header)
	if configAllowTools == nil && allowToolsFromHeader == nil {
		// Both not configured, allow all tools
		return nil
	} else if configAllowTools == nil {
		// Only header restrictions
		return allowToolsFromHeader
	} else if allowToolsFromHeader == nil {
		// Only config restrictions
		return configAllowTools
	} else {
		// Both restrictions exist, compute intersection
		intersection := make(map[string]struct{})
		for tool := range *configAllowTools {
			if _, exists := (*allowToolsFromHeader)[tool]; exists {
				intersection[tool] = struct{}{}
			}
		}
		return &intersection
	}
}

// parseConfigCore contains the core config parsing logic with dependency injection
func parseConfigCore(configJson gjson.Result, config *McpServerConfig, opts *ConfigOptions) error {
	toolSetJson := configJson.Get("toolSet")
	serverJson := configJson.Get("server")                        // This is for single server or REST server definition
	pluginServerConfigJson := configJson.Get("server.config").Raw // Config for the plugin instance itself, if any.

	// serverConfigJsonForInstance is the config passed to the specific server instance (single or REST)
	// It's distinct from pluginServerConfigJson which might be for the mcp-server plugin itself.
	var serverConfigJsonForInstance string

	if toolSetJson.Exists() {
		config.isComposed = true
		var tsConfig ToolSetConfig
		if err := json.Unmarshal([]byte(toolSetJson.Raw), &tsConfig); err != nil {
			return fmt.Errorf("failed to parse toolSet config: %v", err)
		}
		config.toolSet = &tsConfig
		config.serverName = tsConfig.Name // Use toolSet name as the server name for composed server
		log.Infof("Parsing toolSet configuration: %s", config.serverName)

		composedServer := NewComposedMCPServer(config.serverName, tsConfig.ServerTools, opts.ToolRegistry)
		// A composed server itself might have a config block, e.g. for shared settings, though not typical.
		composedServer.SetConfig([]byte(pluginServerConfigJson))
		config.server = composedServer
	} else if serverJson.Exists() {
		config.isComposed = false
		config.serverName = serverJson.Get("name").String()
		if config.serverName == "" {
			return errors.New("server.name field is missing for single server config")
		}
		// This is the config for the specific server being defined (e.g. REST server's own config)
		serverConfigJsonForInstance = serverJson.Get("config").Raw
		log.Infof("Parsing single server configuration: %s", config.serverName)

		// Check server type to determine which type of server to create
		serverType := serverJson.Get("type").String()
		if serverType == "" {
			serverType = "rest" // Default to REST server type
		}

		toolsJson := configJson.Get("tools") // These are REST tools for this server instance or MCP proxy tools

		if serverType == "mcp-proxy" {
			// Create MCP proxy server
			proxyServer, err := setupMcpProxyServer(config.serverName, serverJson, serverConfigJsonForInstance)
			if err != nil {
				return err
			}

			// Handle tools configuration (optional for MCP proxy)
			if toolsJson.Exists() && len(toolsJson.Array()) > 0 {
				for _, toolJson := range toolsJson.Array() {
					var proxyTool McpProxyToolConfig
					if err := json.Unmarshal([]byte(toolJson.Raw), &proxyTool); err != nil {
						return fmt.Errorf("failed to parse proxy tool config: %v", err)
					}

					if err := proxyServer.AddProxyTool(proxyTool); err != nil {
						return fmt.Errorf("failed to add proxy tool %s: %v", proxyTool.Name, err)
					}
					// Register tool to registry
					opts.ToolRegistry.RegisterTool(config.serverName, proxyTool.Name, proxyServer.GetMCPTools()[proxyTool.Name])
				}
			}
			// Set the proxy server regardless of whether tools are configured
			config.server = proxyServer
		} else if toolsJson.Exists() && len(toolsJson.Array()) > 0 {
			// Handle REST-to-MCP server (requires tools configuration)
			// Create REST-to-MCP server (default behavior)
			restServer := NewRestMCPServer(config.serverName)         // Pass the server name
			restServer.SetConfig([]byte(serverConfigJsonForInstance)) // Pass the server's specific config

			securitySchemesJson := serverJson.Get("securitySchemes")
			if securitySchemesJson.Exists() {
				for _, schemeJson := range securitySchemesJson.Array() {
					var scheme SecurityScheme
					if err := json.Unmarshal([]byte(schemeJson.Raw), &scheme); err != nil {
						return fmt.Errorf("failed to parse security scheme config: %v", err)
					}
					restServer.AddSecurityScheme(scheme)
				}
			}

			// Parse default downstream security
			defaultDownstreamSecurityJson := serverJson.Get("defaultDownstreamSecurity")
			if defaultDownstreamSecurityJson.Exists() {
				var defaultDownstreamSecurity SecurityRequirement
				if err := json.Unmarshal([]byte(defaultDownstreamSecurityJson.Raw), &defaultDownstreamSecurity); err != nil {
					return fmt.Errorf("failed to parse defaultDownstreamSecurity config: %v", err)
				}
				restServer.SetDefaultDownstreamSecurity(defaultDownstreamSecurity)
			}

			// Parse default upstream security
			defaultUpstreamSecurityJson := serverJson.Get("defaultUpstreamSecurity")
			if defaultUpstreamSecurityJson.Exists() {
				var defaultUpstreamSecurity SecurityRequirement
				if err := json.Unmarshal([]byte(defaultUpstreamSecurityJson.Raw), &defaultUpstreamSecurity); err != nil {
					return fmt.Errorf("failed to parse defaultUpstreamSecurity config: %v", err)
				}
				restServer.SetDefaultUpstreamSecurity(defaultUpstreamSecurity)
			}

			// Parse passthroughAuthHeader (optional, defaults to false)
			passthroughAuthHeader := serverJson.Get("passthroughAuthHeader").Bool()
			restServer.SetPassthroughAuthHeader(passthroughAuthHeader)

			for _, toolJson := range toolsJson.Array() {
				var restTool RestTool
				if err := json.Unmarshal([]byte(toolJson.Raw), &restTool); err != nil {
					return fmt.Errorf("failed to parse tool config: %v", err)
				}

				if err := restServer.AddRestTool(restTool); err != nil {
					return fmt.Errorf("failed to add tool %s: %v", restTool.Name, err)
				}
				// Register tool to registry
				opts.ToolRegistry.RegisterTool(config.serverName, restTool.Name, restServer.GetMCPTools()[restTool.Name])
			}
			config.server = restServer
		} else {
			// Logic for pre-registered Go-based servers (non-REST)
			if opts.SkipPreRegisteredServers {
				// In validation mode, skip pre-registered servers validation
				// Just validate the basic structure without actual server instance
				config.server = nil // Will be handled appropriately in validation context
			} else {
				if serverInstance, exist := opts.Servers[config.serverName]; exist {
					clonedServer := serverInstance.Clone()
					clonedServer.SetConfig([]byte(serverConfigJsonForInstance)) // Pass the server's specific config
					config.server = clonedServer
					// Register tools from this server to registry
					for toolName, toolInstance := range clonedServer.GetMCPTools() {
						opts.ToolRegistry.RegisterTool(config.serverName, toolName, toolInstance)
					}
				} else {
					return fmt.Errorf("mcp server type '%s' not registered", config.serverName)
				}
			}
		}
	} else {
		return errors.New("either 'server' or 'toolSet' field must be present in the configuration")
	}

	// Parse allowTools - this might need adjustment for composed servers
	// Use pointer to distinguish between "not configured" (nil) and "configured as empty" (empty map)
	var allowTools *map[string]struct{} // For single server, tool name. For composed, serverName/toolName.
	allowToolsResult := configJson.Get("allowTools")
	if allowToolsResult.Exists() {
		// allowTools is configured, create the map
		toolsMap := make(map[string]struct{})
		allowToolsArray := allowToolsResult.Array()
		for _, toolJson := range allowToolsArray {
			toolsMap[toolJson.String()] = struct{}{}
		}
		allowTools = &toolsMap
	}
	// If allowTools is nil, it means not configured (allow all)

	config.methodHandlers = make(utils.MethodHandlers)
	// Use config.serverName which is now reliably set
	currentServerNameForHandlers := config.serverName

	config.methodHandlers["ping"] = func(ctx wrapper.HttpContext, id utils.JsonRpcID, params gjson.Result) error {
		utils.OnMCPResponseSuccess(ctx, map[string]any{}, fmt.Sprintf("mcp:%s:ping", currentServerNameForHandlers))
		return nil
	}
	config.methodHandlers["notifications/initialized"] = func(ctx wrapper.HttpContext, id utils.JsonRpcID, params gjson.Result) error {
		proxywasm.SendHttpResponseWithDetail(202, fmt.Sprintf("mcp:%s:notifications/initialized", currentServerNameForHandlers), nil, nil, -1)
		return nil
	}
	config.methodHandlers["notifications/cancelled"] = func(ctx wrapper.HttpContext, id utils.JsonRpcID, params gjson.Result) error {
		proxywasm.SendHttpResponseWithDetail(202, fmt.Sprintf("mcp:%s:notifications/cancelled", currentServerNameForHandlers), nil, nil, -1)
		return nil
	}
	config.methodHandlers["initialize"] = func(ctx wrapper.HttpContext, id utils.JsonRpcID, params gjson.Result) error {
		requestedVersion := params.Get("protocolVersion").String()
		if requestedVersion == "" {
			utils.OnMCPResponseError(ctx, errors.New("protocolVersion is required"), utils.ErrInvalidParams, fmt.Sprintf("mcp:%s:initialize:error", currentServerNameForHandlers))
			return nil
		}

		// MCP specification compliant version negotiation:
		// If the server supports the requested protocol version, it MUST respond with the same version.
		// Otherwise, the server MUST respond with another protocol version it supports.
		// This SHOULD be the latest version supported by the server.
		negotiatedVersion := requestedVersion
		if !slices.Contains(SupportedMCPVersions, requestedVersion) {
			// Return the latest supported version instead of rejecting the request
			negotiatedVersion = SupportedMCPVersions[len(SupportedMCPVersions)-1]
			log.Warnf("Client requested unsupported version %s, responding with latest supported version %s",
				requestedVersion, negotiatedVersion)
		}

		utils.OnMCPResponseSuccess(ctx, map[string]any{
			"protocolVersion": negotiatedVersion,
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
			"serverInfo": map[string]any{
				"name":    currentServerNameForHandlers, // Use the actual server name (single or composed)
				"version": "1.0.0",
			},
		}, fmt.Sprintf("mcp:%s:initialize", currentServerNameForHandlers))
		return nil
	}

	// Override tools/list and tools/call handlers for MCP proxy servers first
	if config.server != nil {
		if proxyServer, ok := config.server.(*McpProxyServer); ok {
			// Use MCP proxy specific handlers that support ActionPause
			proxyHandlers := CreateMcpProxyMethodHandlers(proxyServer, allowTools)
			config.methodHandlers["tools/list"] = proxyHandlers["tools/list"]
			config.methodHandlers["tools/call"] = proxyHandlers["tools/call"]
		}
	}

	// Default tools/list handler for non-proxy servers
	if config.methodHandlers["tools/list"] == nil {
		config.methodHandlers["tools/list"] = func(ctx wrapper.HttpContext, id utils.JsonRpcID, params gjson.Result) error {
			var listedTools []map[string]any
			// GetMCPTools() will return appropriately formatted tools for both single and composed servers
			allTools := config.server.GetMCPTools() // For composed, keys are "serverName/toolName"

			// Compute effective allowTools using helper function
			effectiveAllowTools := computeEffectiveAllowTools(allowTools)

			for toolFullName, tool := range allTools {
				// For composed server, toolFullName is "originalServerName/originalToolName"
				// For single server, toolFullName is "originalToolName"
				// The allowTools map should use the same format as toolFullName
				if effectiveAllowTools != nil {
					if _, allow := (*effectiveAllowTools)[toolFullName]; !allow {
						continue
					}
				}
				toolDef := map[string]any{
					"name":        toolFullName,
					"description": tool.Description(),
					"inputSchema": tool.InputSchema(),
				}
				// Add outputSchema if tool implements ToolWithOutputSchema (MCP Protocol Version 2025-06-18)
				if toolWithSchema, ok := tool.(ToolWithOutputSchema); ok {
					if outputSchema := toolWithSchema.OutputSchema(); len(outputSchema) > 0 {
						toolDef["outputSchema"] = outputSchema
					}
				}
				listedTools = append(listedTools, toolDef)
			}
			utils.OnMCPResponseSuccess(ctx, map[string]any{
				"tools": listedTools,
			}, fmt.Sprintf("mcp:%s:tools/list", currentServerNameForHandlers))
			return nil
		}
	}

	// Default tools/call handler for non-proxy servers
	if config.methodHandlers["tools/call"] == nil {
		config.methodHandlers["tools/call"] = func(ctx wrapper.HttpContext, id utils.JsonRpcID, params gjson.Result) error {
			if config.isComposed {
				// This endpoint is for a composed server (toolSet).
				// Actual tool calls should be routed by mcp-router to individual servers.
				// If a tools/call request reaches here, it's a misconfiguration or unexpected.
				errMsg := fmt.Sprintf("tools/call is not supported on a composed toolSet endpoint ('%s'). It should be routed by mcp-router to the target server.", currentServerNameForHandlers)
				log.Errorf(errMsg)
				utils.OnMCPResponseError(ctx, errors.New(errMsg), utils.ErrMethodNotFound, fmt.Sprintf("mcp:%s:tools/call:not_supported_on_toolset", currentServerNameForHandlers))
				return nil
			}

			// Logic for single (non-composed) server
			toolName := params.Get("name").String() // For single server, this is the direct tool name
			args := params.Get("arguments")

			// Compute effective allowTools using helper function
			effectiveAllowTools := computeEffectiveAllowTools(allowTools)

			// Check if tool is allowed
			if effectiveAllowTools != nil {
				if _, allow := (*effectiveAllowTools)[toolName]; !allow {
					utils.OnMCPResponseError(ctx, fmt.Errorf("Tool not allowed: %s", toolName), utils.ErrInvalidParams, fmt.Sprintf("mcp:%s:tools/call:tool_not_allowed", currentServerNameForHandlers))
					return nil
				}
			}

			proxywasm.SetProperty([]string{"mcp_server_name"}, []byte(currentServerNameForHandlers))
			proxywasm.SetProperty([]string{"mcp_tool_name"}, []byte(toolName))

			toolToCall, ok := config.server.GetMCPTools()[toolName]
			if !ok {
				utils.OnMCPResponseError(ctx, fmt.Errorf("unknown tool: %s", toolName), utils.ErrInvalidParams, fmt.Sprintf("mcp:%s:tools/call:invalid_tool_name", currentServerNameForHandlers))
				return nil
			}

			log.Debugf("Tool call [%s] on server [%s] with arguments[%s]", toolName, currentServerNameForHandlers, args.Raw)
			toolInstance := toolToCall.Create([]byte(args.Raw))
			err := toolInstance.Call(ctx, config.server) // Pass the single server instance
			if err != nil {
				utils.OnMCPToolCallError(ctx, err)
				return nil
			}
			return nil
		}
	}

	return nil
}

// ParseConfigCore exports the core parsing logic for external use (e.g., validation)
func ParseConfigCore(configJson gjson.Result, config *McpServerConfig, opts *ConfigOptions) error {
	return parseConfigCore(configJson, config, opts)
}

func parseConfig(context wrapper.PluginContext, configJson gjson.Result, config *McpServerConfig) error {
	registryI := context.GetContext(GlobalToolRegistryKey)
	if registryI == nil {
		return errors.New("GlobalToolRegistry not found")
	}
	registry, ok := registryI.(*GlobalToolRegistry)
	if !ok {
		return errors.New("invalid GlobalToolRegistry")
	}
	// Build runtime dependencies using global variables
	opts := &ConfigOptions{
		Servers:      globalContext.servers,
		ToolRegistry: registry,
	}

	// Call the core parsing logic
	return parseConfigCore(configJson, config, opts)
}

func Load(options ...CtxOption) {
	for _, opt := range options {
		opt.Apply(&globalContext)
	}
}

func Initialize() {
	if globalContext.servers == nil {
		panic("At least one mcpserver needs to be added.")
	}
	wrapper.SetCtx(
		"mcp-server",
		wrapper.PrePluginStartOrReload[McpServerConfig](onPluginStartOrReload),
		wrapper.ParseConfigWithContext(parseConfig),
		wrapper.WithLogger[McpServerConfig](&utils.MCPServerLog{}),
		wrapper.ProcessRequestHeaders(onHttpRequestHeaders),
		wrapper.ProcessRequestBody(onHttpRequestBody),
		wrapper.ProcessResponseHeaders(onHttpResponseHeaders),
		wrapper.ProcessStreamingResponseBody(onHttpStreamingResponseBody),
		wrapper.WithRebuildMaxMemBytes[McpServerConfig](200*1024*1024),
	)
}

type addMCPServerOption struct {
	name   string
	server Server
}

func AddMCPServer(name string, server Server) CtxOption {
	return &addMCPServerOption{
		name:   name,
		server: server,
	}
}

func (o *addMCPServerOption) Apply(ctx *Context) {
	if ctx.servers == nil {
		ctx.servers = make(map[string]Server)
	}
	if _, exist := ctx.servers[o.name]; exist {
		panic(fmt.Sprintf("Conflict! There is a mcp server with the same name:%s",
			o.name))
	}
	ctx.servers[o.name] = o.server
}

func ToInputSchema(v any) map[string]any {
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	inputSchema := jsonschema.Reflect(v).Definitions[t.Name()]
	inputSchemaBytes, _ := json.Marshal(inputSchema)
	var result map[string]any
	json.Unmarshal(inputSchemaBytes, &result)
	return result
}

func StoreServerState(ctx wrapper.HttpContext, config any) {
	if utils.IsStatefulSession(ctx) {
		log.Warnf("There is no session ID, unable to store state.")
		return
	}
	configBytes, err := json.Marshal(config)
	if err != nil {
		log.Errorf("Server config marshal failed:%v, config:%s", err, configBytes)
		return
	}
	proxywasm.SetProperty([]string{"mcp_server_config"}, configBytes)
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config McpServerConfig) types.Action {
	ctx.DisableReroute()
	ctx.SetRequestBodyBufferLimit(DefaultMaxBodyBytes)
	ctx.SetResponseBodyBufferLimit(DefaultMaxBodyBytes)

	// Remove accept-encoding header to prevent backend from compressing the response
	// This ensures we can properly process and modify the response body
	proxywasm.RemoveHttpRequestHeader("accept-encoding")

	// Parse MCP-Protocol-Version header and store in context
	// This allows clients to specify the MCP protocol version via HTTP header
	// instead of only through the JSON-RPC initialize method
	protocolVersion, _ := proxywasm.GetHttpRequestHeader("MCP-Protocol-Version")
	if protocolVersion != "" {
		// Validate the protocol version against supported versions
		if slices.Contains(SupportedMCPVersions, protocolVersion) {
			log.Debugf("MCP Protocol Version set from header: %s", protocolVersion)
		} else {
			log.Warnf("Unsupported MCP Protocol Version in header: %s", protocolVersion)
		}

		// Remove the header from the request to prevent it from being forwarded
		proxywasm.RemoveHttpRequestHeader("MCP-Protocol-Version")
	}

	if ctx.Method() == "GET" {
		proxywasm.SendHttpResponseWithDetail(405, "not_support_sse_on_this_endpoint", nil, nil, -1)
		return types.HeaderStopAllIterationAndWatermark
	}
	// Handle DELETE request for session termination (MCP 2025-06-18 spec)
	// Per spec: "Clients that no longer need a particular session SHOULD send an HTTP DELETE
	// to the MCP endpoint with the Mcp-Session-Id header, to explicitly terminate the session."
	// Per spec: "The server MAY respond to this request with HTTP 405 Method Not Allowed,
	// indicating that the server does not allow clients to terminate sessions."
	if ctx.Method() == "DELETE" {
		proxywasm.SendHttpResponseWithDetail(405, "session_termination_not_supported", nil, nil, -1)
		return types.HeaderStopAllIterationAndWatermark
	}
	if !ctx.HasRequestBody() {
		proxywasm.SendHttpResponseWithDetail(400, "missing_body_in_mcp_request", nil, nil, -1)
		return types.HeaderStopAllIterationAndWatermark
	}
	return types.HeaderStopIteration
}

func onHttpRequestBody(ctx wrapper.HttpContext, config McpServerConfig, body []byte) types.Action {
	return utils.HandleJsonRpcMethod(ctx, body, config.methodHandlers)
}

func onHttpResponseHeaders(ctx wrapper.HttpContext, config McpServerConfig) types.Action {
	// Check if this request initiated SSE channel (tools/list or tools/call with SSE transport)
	// Only these requests need special SSE streaming response processing
	if ctx.GetContext(CtxSSEProxyState) != nil {
		// Check if response has a body
		if ctx.HasResponseBody() {
			// Pause streaming response for processing
			// Content-type validation will be done in onHttpStreamingResponseBody
			ctx.NeedPauseStreamingResponse()
			return types.HeaderStopIteration
		} else {
			// No body, return error
			utils.OnMCPResponseError(ctx, fmt.Errorf("no response body in SSE response"), utils.ErrInternalError, "mcp-proxy:sse:no_body")
			return types.HeaderStopAllIterationAndWatermark
		}
	}

	// For non-SSE streaming requests, continue normally
	return types.HeaderContinue
}

func onHttpStreamingResponseBody(ctx wrapper.HttpContext, config McpServerConfig, data []byte, endOfStream bool) []byte {
	// Check if this request initiated SSE channel (tools/list or tools/call with SSE transport)
	// Only these requests need special SSE streaming response processing
	if ctx.GetContext(CtxSSEProxyState) != nil {
		return handleSSEStreamingResponse(ctx, config, data, endOfStream)
	}

	// For non-SSE streaming requests, return data as-is
	return data
}
