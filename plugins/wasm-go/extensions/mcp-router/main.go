// Copyright (c) 2022 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp"
	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/consts"
	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/protocol"
	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/utils"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func main() {}

func init() {
	mcp.LoadMCPFilter(
		mcp.FilterName("mcp-router"),
		mcp.SetConfigOverrideParser(ParseGlobalConfig, ParseOverrideConfig),
		mcp.SetJsonRpcRequestFilter(markToolDispatchEnabled),
		mcp.SetToolCallRequestFilter(ProcessRequest),
	)
	mcp.InitMCPFilter()
}

// ServerConfig represents the routing configuration for a single MCP server
type ServerConfig struct {
	Name   string `json:"name"`
	Domain string `json:"domain,omitempty"`
	Path   string `json:"path"`
}

// McpRouterGlobalConfig represents the global configuration for the mcp-router filter
type McpRouterGlobalConfig struct {
	Servers []ServerConfig `json:"servers"`
}

type McpRouterConfig struct {
	global *McpRouterGlobalConfig
	enable bool
}

func ParseGlobalConfig(configBytes []byte, globalConfig *any) error {
	var config McpRouterGlobalConfig
	if err := json.Unmarshal(configBytes, &config); err != nil {
		return fmt.Errorf("failed to parse mcp-router config: %v", err)
	}

	log.Infof("Parsed mcp-router config with %d servers", len(config.Servers))
	for _, server := range config.Servers {
		log.Debugf("Server: %s -> %s%s", server.Name, server.Domain, server.Path)
	}

	*globalConfig = config
	return nil
}

func ParseOverrideConfig(configBytes []byte, globalConfig any, ruleConfig *any) error {
	var config McpRouterConfig
	if globalConfig == nil {
		config.global = &McpRouterGlobalConfig{}
		config.enable = false
		*ruleConfig = config
		log.Error("globalConfig not found, mcp router will not work")
		return nil
	}
	parent, ok := globalConfig.(McpRouterGlobalConfig)
	if !ok {
		return fmt.Errorf("invalid globalConfig: %v", globalConfig)
	}
	config.global = &parent
	config.enable = gjson.GetBytes(configBytes, "enable").Bool()
	*ruleConfig = config
	return nil
}

// markToolDispatchEnabled tells the following mcp-server filter that the
// matched rule enables tool dispatch on this discovery request.
func markToolDispatchEnabled(_ wrapper.HttpContext, config any, _ utils.JsonRpcID, method string, _ gjson.Result, _ []byte) types.Action {
	routerConfig, ok := config.(McpRouterConfig)
	if method == "server/discover" && ok && routerConfig.enable {
		// If the host cannot set the flag, discovery treats dispatch as absent.
		_ = proxywasm.SetProperty([]string{consts.MCPRouterToolDispatchEnabled}, []byte("true"))
	}
	return types.ActionContinue
}

func ProcessRequest(context wrapper.HttpContext, config any, toolName string, _ gjson.Result, rawBody []byte) types.Action {
	routerConfig, ok := config.(McpRouterConfig)
	if !ok {
		log.Errorf("Invalid config type for mcp-router")
		return types.ActionContinue
	}
	if !routerConfig.enable {
		return types.ActionContinue
	}
	// Extract server name from tool name (format: "${serverName}___${toolName}")
	parts := strings.SplitN(toolName, consts.ToolSetNameSplitter, 2)
	if len(parts) != 2 {
		log.Debugf("Tool name '%s' does not contain server prefix, continuing without routing", toolName)
		return types.ActionContinue
	}

	serverName := parts[0]
	actualToolName := parts[1]

	log.Debugf("Routing tool call: server=%s, tool=%s", serverName, actualToolName)

	// Find the server configuration
	var targetServer *ServerConfig
	for _, server := range routerConfig.global.Servers {
		if server.Name == serverName {
			targetServer = &server
			break
		}
	}

	if targetServer == nil {
		log.Warnf("No routing configuration found for server '%s'", serverName)
		return types.ActionContinue
	}

	headers, err := proxywasm.GetHttpRequestHeaders()
	if err != nil {
		log.Errorf("Failed to read MCP request headers: %v", err)
		return types.ActionContinue
	}
	transport := protocol.NewTransport(context.Method(), context.Host(), headers)
	request, protocolError := protocol.PrepareModernToolCall(transport, rawBody)
	if protocolError != nil {
		var id protocol.ID
		if request != nil {
			id = request.Envelope.ID
		}
		utils.SendProtocolError(context, id, protocolError, "mcp_modern_request_rejected")
		return types.ActionContinue
	}
	var nameHeader string
	if request != nil {
		nameHeader = protocol.EncodeNameHeader(actualToolName)
		if nameHeader == "" {
			utils.SendProtocolError(context, request.Envelope.ID, protocol.InvalidParams("tools/call params.name is required"), "mcp_router_invalid_tool_name")
			return types.ActionContinue
		}
	}

	modifiedBody, err := sjson.SetBytes(rawBody, "params.name", actualToolName)
	if err != nil {
		log.Errorf("Failed to modify MCP tool name: %v", err)
		return types.ActionContinue
	}
	log.Infof("Routing to server '%s': domain=[%s], path=[%s]", serverName, targetServer.Domain, targetServer.Path)

	if err := forwardToolCall(targetServer, nameHeader, modifiedBody, proxywasm.ReplaceHttpRequestHeader); err != nil {
		log.Errorf("Failed to route MCP tool call: %v", err)
		if request != nil {
			// A partially rewritten Modern request must never reach a source.
			utils.SendProtocolError(context, request.Envelope.ID, &protocol.Error{
				HTTPStatus: 500, Code: protocol.CodeInternalError, Message: "Failed to route MCP tool call",
			}, "mcp_router_rewrite_failed")
		}
		return types.ActionContinue
	}

	log.Infof("Successfully routed request for tool '%s' to server '%s'. New tool name is '%s'.",
		toolName, serverName, actualToolName)
	return types.ActionContinue
}

// forwardToolCall applies the prepared route and name; Legacy calls have no
// replacement Mcp-Name header.
func forwardToolCall(target *ServerConfig, nameHeader string, body []byte, replaceHeader func(string, string) error) error {
	if target.Domain != "" {
		if err := replaceHeader(":authority", target.Domain); err != nil {
			return fmt.Errorf("set authority: %w", err)
		}
	}
	if err := replaceHeader("x-envoy-internal-route", "true"); err != nil {
		return fmt.Errorf("set internal route marker: %w", err)
	}
	if err := replaceHeader(":path", target.Path); err != nil {
		return fmt.Errorf("set path: %w", err)
	}
	if nameHeader != "" {
		if err := replaceHeader(protocol.HeaderName, nameHeader); err != nil {
			return fmt.Errorf("set Mcp-Name: %w", err)
		}
	}
	return proxywasm.ReplaceHttpRequestBody(body)
}
