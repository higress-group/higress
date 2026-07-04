package nacos

import (
	"errors"
	"fmt"
	"time"

	"github.com/alibaba/higress/plugins/golang-filter/mcp-server/registry"
	"github.com/alibaba/higress/plugins/golang-filter/mcp-session/common"
	"github.com/envoyproxy/envoy/contrib/golang/common/go/api"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/nacos-group/nacos-sdk-go/v2/clients"
	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
)

func init() {
	common.GlobalRegistry.RegisterServer("nacos-mcp-registry", &NacosConfig{})
}

type NacosConfig struct {
	ServerAddr     *string
	Ak             *string
	Sk             *string
	Namespace      *string
	RegionId       *string
	ServiceMatcher *map[string]string
}

type McpServerToolsChangeListener struct {
	mcpServer *common.MCPServer
}

func (l *McpServerToolsChangeListener) OnToolChanged(reg registry.McpServerRegistry) {
	resetToolsToMcpServer(l.mcpServer, reg)
}

func CreateNacosMcpRegistry(config *NacosConfig) (*NacosMcpRegistry, error) {
	sc := []constant.ServerConfig{
		*constant.NewServerConfig(*config.ServerAddr, 8848, constant.WithContextPath("/nacos")),
	}

	// create ClientConfig
	cc := *constant.NewClientConfig(
		constant.WithTimeoutMs(5000),
		constant.WithNotLoadCacheAtStart(true),
		constant.WithOpenKMS(true),
		constant.WithLogLevel("error"),
	)
	cc.AppendToStdout = true

	cc.DiableLog = true

	if config.Namespace != nil {
		cc.NamespaceId = *config.Namespace
	}

	if config.RegionId != nil {
		cc.RegionId = *config.RegionId
	}

	if config.Ak != nil {
		cc.AccessKey = *config.Ak
	}

	if config.Sk != nil {
		cc.SecretKey = *config.Sk
	}

	// create config client
	configClient, err := clients.NewConfigClient(
		vo.NacosClientParam{
			ClientConfig:  &cc,
			ServerConfigs: sc,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initial nacos config client: %w", err)
	}

	namingClient, err := clients.NewNamingClient(
		vo.NacosClientParam{
			ClientConfig:  &cc,
			ServerConfigs: sc,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initial naming config client: %w", err)
	}

	return &NacosMcpRegistry{
		configClient:             configClient,
		namingClient:             namingClient,
		serviceMatcher:           *config.ServiceMatcher,
		toolChangeEventListeners: []registry.ToolChangeEventListener{},
		toolsDescription:         map[string]*registry.ToolDescription{},
		toolsRpcContext:          map[string]*registry.RpcContext{},
		currentServiceSet:        map[string]bool{},
	}, nil
}

func (c *NacosConfig) ParseConfig(config map[string]any) error {
	serverAddr, ok := config["serverAddr"].(string)
	if !ok {
		return errors.New("missing serverAddr")
	}
	c.ServerAddr = &serverAddr

	serviceMatcher, ok := config["serviceMatcher"].(map[string]any)
	if !ok {
		return errors.New("missing serviceMatcher")
	}

	if namespace, ok := config["namespace"].(string); ok {
		c.Namespace = &namespace
	}

	matchers := map[string]string{}
	for key, value := range serviceMatcher {
		matchers[key] = value.(string)
	}

	c.ServiceMatcher = &matchers

	if ak, ok := config["accessKey"].(string); ok {
		c.Ak = &ak
	}

	if sk, ok := config["secretKey"].(string); ok {
		c.Sk = &sk
	}

	if region, ok := config["regionId"].(string); ok {
		c.RegionId = &region
	}
	return nil
}

func (c *NacosConfig) NewServer(serverName string) (*common.MCPServer, error) {
	mcpServer := common.NewMCPServer(
		serverName,
		"1.0.0",
	)

	nacosRegistry, err := CreateNacosMcpRegistry(c)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize NacosMcpRegistry: %w", err)
	}

	listener := McpServerToolsChangeListener{
		mcpServer: mcpServer,
	}
	nacosRegistry.RegisterToolChangeEventListener(&listener)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				api.LogErrorf("NacosToolsListRefresh recovered from panic: %v", r)
			}
		}()

		for {
			if nacosRegistry.refreshToolsList() {
				resetToolsToMcpServer(mcpServer, nacosRegistry)
			}
			time.Sleep(time.Second * 3)
		}
	}()
	return mcpServer, nil
}

func resetToolsToMcpServer(mcpServer *common.MCPServer, reg registry.McpServerRegistry) {
	wrappedTools := []common.ServerTool{}
	tools := reg.ListToolsDescription()
	for _, tool := range tools {
		wrappedTools = append(wrappedTools, common.ServerTool{
			Tool:    mcp.NewToolWithRawSchema(tool.Name, tool.Description, tool.InputSchema),
			Handler: registry.HandleRegistryToolsCall(reg),
		})
	}
	mcpServer.SetTools(wrappedTools...)
	api.LogInfof("Tools reset, new tools list len %d", len(wrappedTools))
}
