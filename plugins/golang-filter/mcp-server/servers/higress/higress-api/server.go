package higress_ops

import (
	"errors"

	"github.com/alibaba/higress/plugins/golang-filter/mcp-server/servers/higress"
	"github.com/alibaba/higress/plugins/golang-filter/mcp-server/servers/higress/higress-api/tools"
	"github.com/alibaba/higress/plugins/golang-filter/mcp-server/servers/higress/higress-api/tools/plugins"
	"github.com/alibaba/higress/plugins/golang-filter/mcp-session/common"
	"github.com/envoyproxy/envoy/contrib/golang/common/go/api"
)

const Version = "1.0.0"

func init() {
	common.GlobalRegistry.RegisterServer("higress-api", &HigressConfig{})
}

type HigressConfig struct {
	higressURL  string
	description string
}

func (c *HigressConfig) Clone() common.Server {
	cloned := *c
	return &cloned
}

func (c *HigressConfig) ParseConfig(config map[string]interface{}) error {
	higressURL, ok := config["higressURL"].(string)
	if !ok {
		return errors.New("missing higressURL")
	}
	c.higressURL = higressURL

	if desc, ok := config["description"].(string); ok {
		c.description = desc
	} else {
		c.description = "Higress API MCP Server, which invokes Higress Console APIs to manage resources such as routes, services, and plugins."
	}

	api.LogInfof("Higress MCP Server configuration parsed successfully. URL: %s",
		c.higressURL)

	return nil
}

func (c *HigressConfig) NewServer(serverName string) (*common.MCPServer, error) {
	mcpServer := common.NewMCPServer(
		serverName,
		Version,
		common.WithInstructions("This is a Higress API MCP Server"),
	)

	// Initialize Higress API client
	client := higress.NewHigressClient(c.higressURL)

	// Register all tools
	tools.RegisterRouteTools(mcpServer, client)
	tools.RegisterServiceTools(mcpServer, client)
	tools.RegisterAiRouteTools(mcpServer, client)
	tools.RegisterAiProviderTools(mcpServer, client)
	tools.RegisterMcpServerTools(mcpServer, client)
	plugins.RegisterCommonPluginTools(mcpServer, client)
	plugins.RegisterRequestBlockPluginTools(mcpServer, client)
	plugins.RegisterCustomResponsePluginTools(mcpServer, client)

	api.LogInfof("Higress MCP Server initialized: %s", serverName)

	return mcpServer, nil
}
