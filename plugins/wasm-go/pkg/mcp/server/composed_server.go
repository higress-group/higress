package server

import (
	"encoding/json"
	"fmt"

	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/consts"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/wasm-go/pkg/log"
)

// ComposedMCPServer represents a server composed of tools from other servers.
type ComposedMCPServer struct {
	name        string              // Name of the composed server (from toolSet.name)
	serverTools []ServerToolConfig  // Configuration of which tools to include
	registry    *GlobalToolRegistry // Reference to the global tool registry
	config      []byte              // Configuration for the composed server itself (if any)
}

// NewComposedMCPServer creates a new ComposedMCPServer.
func NewComposedMCPServer(name string, serverToolsConfig []ServerToolConfig, registry *GlobalToolRegistry) *ComposedMCPServer {
	return &ComposedMCPServer{
		name:        name,
		serverTools: serverToolsConfig,
		registry:    registry,
	}
}

// GetName returns the name of the composed server.
func (cs *ComposedMCPServer) GetName() string {
	return cs.name
}

// AddMCPTool for ComposedMCPServer is a no-op as tools are defined by toolSet.
func (cs *ComposedMCPServer) AddMCPTool(name string, tool Tool) Server {
	log.Warnf("AddMCPTool called on ComposedMCPServer '%s'; this is a no-op.", cs.name)
	return cs
}

// GetMCPTools constructs and returns the map of tools exposed by this composed server.
// The tool names are prefixed with their original server name, e.g., "${originalServer}___${toolName}".
// The Tool instances are DescriptiveTool, only providing Description and InputSchema.
func (cs *ComposedMCPServer) GetMCPTools() map[string]Tool {
	composedTools := make(map[string]Tool)
	for _, stc := range cs.serverTools {
		originalServerName := stc.ServerName
		for _, originalToolName := range stc.Tools {
			toolInfo, found := cs.registry.GetToolInfo(originalServerName, originalToolName)
			if !found {
				log.Warnf("Tool %s/%s not found in global registry for composed server %s", originalServerName, originalToolName, cs.name)
				continue
			}

			composedToolName := fmt.Sprintf("%s%s%s", originalServerName, consts.ToolSetNameSplitter, originalToolName)
			composedTools[composedToolName] = &DescriptiveTool{
				description:             toolInfo.Description,
				inputSchema:             toolInfo.InputSchema,
				inputSchemaSerializable: toolInfo.inputSchemaSerializable,
				outputSchema:            toolInfo.OutputSchema, // New field for MCP Protocol Version 2025-06-18
			}
		}
	}
	return composedTools
}

// hasRoutedTool requires both a published descriptor in this configuration
// generation and a source route advertised by mcp-router on this request.
// A standalone toolSet can list descriptors but has no call dispatcher.
func (cs *ComposedMCPServer) hasRoutedTool() bool {
	raw, err := proxywasm.GetProperty([]string{consts.MCPRouterDiscoverySources})
	if err != nil {
		return false
	}
	var sources map[string]bool
	if err := json.Unmarshal(raw, &sources); err != nil {
		return false
	}
	for _, source := range cs.serverTools {
		if !sources[source.ServerName] {
			continue
		}
		for _, name := range source.Tools {
			if tool, found := cs.registry.GetToolInfo(source.ServerName, name); found && tool.inputSchemaSerializable {
				return true
			}
		}
	}
	return false
}

// SetConfig sets the configuration for the composed server itself.
func (cs *ComposedMCPServer) SetConfig(config []byte) {
	cs.config = config
}

// GetConfig retrieves the configuration of the composed server itself.
func (cs *ComposedMCPServer) GetConfig(v any) {
	if len(cs.config) == 0 {
		return
	}
	if ptrBytes, ok := v.(*[]byte); ok {
		*ptrBytes = cs.config
	} else {
		// If you need to unmarshal to a struct, you'd do it here.
		// For now, keeping it simple as per previous discussions.
		log.Warnf("ComposedMCPServer.GetConfig called with unhandled type for v. Config not set.")
	}
}

// Clone creates a new instance of the ComposedMCPServer with the same configuration.
func (cs *ComposedMCPServer) Clone() Server {
	cloned := NewComposedMCPServer(cs.name, cs.serverTools, cs.registry)
	cloned.SetConfig(cs.config)
	return cloned
}

// DescriptiveTool is a placeholder Tool implementation for ComposedMCPServer.
// Its Call and Create methods should never be invoked.
type DescriptiveTool struct {
	description             string
	inputSchema             map[string]any
	inputSchemaSerializable bool
	outputSchema            map[string]any // New field for MCP Protocol Version 2025-06-18
}

// Create for DescriptiveTool should not be called.
func (dt *DescriptiveTool) Create(params []byte) Tool {
	log.Errorf("DescriptiveTool.Create called for tool used in ComposedMCPServer. This should not happen.")
	// Return a new instance to fulfill the interface, though it's an error state.
	return &DescriptiveTool{
		description:             dt.description,
		inputSchema:             dt.inputSchema,
		inputSchemaSerializable: dt.inputSchemaSerializable,
		outputSchema:            dt.outputSchema,
	}
}

// Call for DescriptiveTool should not be called.
func (dt *DescriptiveTool) Call(httpCtx HttpContext, server Server) error {
	log.Errorf("DescriptiveTool.Call called for tool used in ComposedMCPServer. This should not happen.")
	return fmt.Errorf("DescriptiveTool.Call should not be invoked on a ComposedMCPServer's tool")
}

// Description returns the tool's description.
func (dt *DescriptiveTool) Description() string {
	return dt.description
}

// InputSchema returns the tool's input schema.
func (dt *DescriptiveTool) InputSchema() map[string]any {
	return dt.inputSchema
}

func (dt *DescriptiveTool) capturedInputSchema() (map[string]any, bool) {
	return dt.inputSchema, dt.inputSchemaSerializable
}

// OutputSchema returns the tool's output schema (MCP Protocol Version 2025-06-18).
func (dt *DescriptiveTool) OutputSchema() map[string]any {
	return dt.outputSchema
}
