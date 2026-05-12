package server

import (
	"fmt"

	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/consts"
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
				description:  toolInfo.Description,
				inputSchema:  toolInfo.InputSchema,
				outputSchema: toolInfo.OutputSchema, // New field for MCP Protocol Version 2025-06-18
			}
		}
	}
	return composedTools
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
	description  string
	inputSchema  map[string]any
	outputSchema map[string]any // New field for MCP Protocol Version 2025-06-18
}

// Create for DescriptiveTool should not be called.
func (dt *DescriptiveTool) Create(params []byte) Tool {
	log.Errorf("DescriptiveTool.Create called for tool used in ComposedMCPServer. This should not happen.")
	// Return a new instance to fulfill the interface, though it's an error state.
	return &DescriptiveTool{
		description:  dt.description,
		inputSchema:  dt.inputSchema,
		outputSchema: dt.outputSchema,
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

// OutputSchema returns the tool's output schema (MCP Protocol Version 2025-06-18).
func (dt *DescriptiveTool) OutputSchema() map[string]any {
	return dt.outputSchema
}
