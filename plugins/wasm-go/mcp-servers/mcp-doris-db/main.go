package main

import (
	"plugins/wasm-go/mcp-servers/mcp-doris-db/tools"
	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp"
)

func main() {}

func init() {
	mcp.LoadMCPServer(mcp.AddMCPServer("mcp-doris-db",
		tools.LoadTools(mcp.NewMCPServer())))
	mcp.InitMCPServer()
} 