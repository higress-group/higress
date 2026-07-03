package mcp_server

import (
	"fmt"

	_ "github.com/alibaba/higress/plugins/golang-filter/mcp-server/registry/nacos"
	_ "github.com/alibaba/higress/plugins/golang-filter/mcp-server/servers/gorm"
	_ "github.com/alibaba/higress/plugins/golang-filter/mcp-server/servers/higress/higress-api"
	_ "github.com/alibaba/higress/plugins/golang-filter/mcp-server/servers/higress/higress-ops"
	_ "github.com/alibaba/higress/plugins/golang-filter/mcp-server/servers/rag"
	_ "github.com/alibaba/higress/plugins/golang-filter/mcp-server/servers/tool-search"
	mcp_session "github.com/alibaba/higress/plugins/golang-filter/mcp-session"
	"github.com/alibaba/higress/plugins/golang-filter/mcp-session/common"
	xds "github.com/cncf/xds/go/xds/type/v3"
	"github.com/envoyproxy/envoy/contrib/golang/common/go/api"
	"google.golang.org/protobuf/types/known/anypb"
)

const (
	Name    = "mcp-server"
	Version = "1.0.0"
)

type SSEServerWrapper struct {
	BaseServer   *common.SSEServer
	HostMatchers []common.HostMatcher // Pre-parsed host matchers for efficient matching
}

type config struct {
	servers []*SSEServerWrapper
}

func (c *config) Destroy() {
	for _, server := range c.servers {
		server.BaseServer.Close()
	}
}

type Parser struct{}

func (p *Parser) Parse(any *anypb.Any, callbacks api.ConfigCallbackHandler) (interface{}, error) {
	configStruct := &xds.TypedStruct{}
	if err := any.UnmarshalTo(configStruct); err != nil {
		return nil, err
	}
	v := configStruct.Value

	conf := &config{
		servers: make([]*SSEServerWrapper, 0),
	}

	serverConfigs, ok := v.AsMap()["servers"].([]interface{})
	if !ok {
		api.LogDebug("No servers are configured")
		return conf, nil
	}

	var parseErrors []string
	for index, rawServerConfig := range serverConfigs {
		serverConfigMap, ok := rawServerConfig.(map[string]interface{})
		if !ok {
			parseErrors = append(parseErrors, fmt.Sprintf("server config at index %d must be an object", index))
			continue
		}

		serverType, ok := serverConfigMap["type"].(string)
		if !ok {
			parseErrors = append(parseErrors, fmt.Sprintf("server config at index %d type is not set", index))
			continue
		}

		serverPath, ok := serverConfigMap["path"].(string)
		if !ok {
			parseErrors = append(parseErrors, fmt.Sprintf("server %s path is not set", serverType))
			continue
		}

		// Parse domain list directly into HostMatchers for efficient matching
		var hostMatchers []common.HostMatcher
		if domainList, ok := serverConfigMap["domain_list"].([]interface{}); ok {
			hostMatchers = make([]common.HostMatcher, 0, len(domainList))
			for _, domain := range domainList {
				if domainStr, ok := domain.(string); ok {
					hostMatchers = append(hostMatchers, common.ParseHostPattern(domainStr))
				}
			}
		} else {
			// Default to match all domains
			hostMatchers = []common.HostMatcher{common.ParseHostPattern("*")}
		}

		serverName, ok := serverConfigMap["name"].(string)
		if !ok {
			parseErrors = append(parseErrors, fmt.Sprintf("server %s name is not set", serverType))
			continue
		}
		server := common.GlobalRegistry.NewServerConfig(serverType)

		if server == nil {
			parseErrors = append(parseErrors, fmt.Sprintf("server %s is not registered", serverType))
			continue
		}
		serverConfig, ok := serverConfigMap["config"].(map[string]interface{})
		if !ok {
			api.LogDebug(fmt.Sprintf("No config provided for server %s", serverType))
		}
		api.LogDebug(fmt.Sprintf("Server config: %+v", serverConfig))

		err := server.ParseConfig(serverConfig)
		if err != nil {
			parseErrors = append(parseErrors, fmt.Sprintf("server %s failed to parse config: %v", serverName, err))
			continue
		}

		serverInstance, err := server.NewServer(serverName)
		if err != nil {
			parseErrors = append(parseErrors, fmt.Sprintf("server %s failed to initialize MCP Server: %v", serverName, err))
			continue
		}

		conf.servers = append(conf.servers, &SSEServerWrapper{
			BaseServer: common.NewSSEServer(serverInstance,
				common.WithSSEEndpoint(fmt.Sprintf("%s%s", serverPath, mcp_session.GlobalSSEPathSuffix)),
				common.WithMessageEndpoint(serverPath)),
			HostMatchers: hostMatchers,
		})
		api.LogDebug(fmt.Sprintf("Registered MCP Server: %s", serverType))
	}
	if len(parseErrors) > 0 {
		api.LogWarnf("mcp-server: some servers failed to load and were skipped: %v", parseErrors)
	}

	return conf, nil
}

func (p *Parser) Merge(parent interface{}, child interface{}) interface{} {
	parentConfig := parent.(*config)
	childConfig := child.(*config)

	newConfig := *parentConfig
	if childConfig.servers != nil {
		newConfig.servers = childConfig.servers
	}
	return &newConfig
}

func FilterFactory(c interface{}, callbacks api.FilterCallbackHandler) api.StreamFilter {
	conf, ok := c.(*config)
	if !ok {
		panic("unexpected config type")
	}
	return &filter{
		config:    conf,
		callbacks: callbacks,
	}
}
