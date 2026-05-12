package mcp

import (
	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/filter"
	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/server"
)

var _ server.Server = &MCPServer{}

// MCPServer implements the Server interface using BaseMCPServer
type MCPServer struct {
	base server.BaseMCPServer
}

// NewMCPServer creates a new MCPServer
func NewMCPServer() *MCPServer {
	return &MCPServer{
		base: server.NewBaseMCPServer(),
	}
}

// Clone implements Server interface
func (s *MCPServer) Clone() server.Server {
	return &MCPServer{
		base: s.base.CloneBase(),
	}
}

// AddMCPTool implements Server interface
func (s *MCPServer) AddMCPTool(name string, tool server.Tool) server.Server {
	s.base.AddMCPTool(name, tool)
	return s
}

// GetConfig implements Server interface
func (s *MCPServer) GetConfig(v any) {
	s.base.GetConfig(v)
}

// GetMCPTools implements Server interface
func (s *MCPServer) GetMCPTools() map[string]server.Tool {
	return s.base.GetMCPTools()
}

// SetConfig implements Server interface
func (s *MCPServer) SetConfig(config []byte) {
	s.base.SetConfig(config)
}

// mcp server function
var (
	LoadMCPServer = server.Load

	InitMCPServer = server.Initialize

	AddMCPServer = server.AddMCPServer
)

// mcp filter function
var (
	LoadMCPFilter = filter.Load

	InitMCPFilter = filter.Initialize

	SetConfigParser = filter.SetConfigParser

	SetConfigOverrideParser = filter.SetConfigOverrideParser

	FilterName = filter.FilterName

	SetJsonRpcRequestFilter = filter.SetJsonRpcRequestFilter

	SetJsonRpcResponseFilter = filter.SetJsonRpcResponseFilter

	SetFallbackHTTPRequestFilter = filter.SetFallbackHTTPRequestFilter

	SetFallbackHTTPResponseFilter = filter.SetFallbackHTTPResponseFilter

	SetToolCallRequestFilter = filter.SetToolCallRequestFilter

	SetToolCallResponseFilter = filter.SetToolCallResponseFilter

	SetToolListResponseFilter = filter.SetToolListResponseFilter
)
