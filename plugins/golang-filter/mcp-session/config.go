package mcp_session

import (
	"fmt"

	_ "net/http/pprof"

	xds "github.com/cncf/xds/go/xds/type/v3"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/alibaba/higress/plugins/golang-filter/mcp-session/common"
	"github.com/envoyproxy/envoy/contrib/golang/common/go/api"
)

const (
	Name              = "mcp-session"
	Version           = "1.0.0"
	DefaultServerName = "higress-mcp-server"
)

var GlobalSSEPathSuffix = "/sse"

type config struct {
	matchList       []common.MatchRule
	redisClient     *common.RedisClient
	sessionRegistry common.SessionRegistry
	sharedMCPServer *common.MCPServer // Created once, thread-safe with sync.RWMutex
}

func (c *config) Destroy() {
	if c.redisClient != nil {
		api.LogDebug("Closing Redis client")
		c.redisClient.Close()
	}
}

type Parser struct{}

// Parse the filter configuration
func (p *Parser) Parse(any *anypb.Any, callbacks api.ConfigCallbackHandler) (interface{}, error) {
	configStruct := &xds.TypedStruct{}
	if err := any.UnmarshalTo(configStruct); err != nil {
		return nil, err
	}
	v := configStruct.Value

	conf := &config{
		matchList: make([]common.MatchRule, 0),
	}

	// Parse match_list if exists
	if matchList, ok := v.AsMap()["match_list"].([]interface{}); ok {
		conf.matchList = common.ParseMatchList(matchList)
	}

	// Redis configuration is optional
	if redisConfigMap, ok := v.AsMap()["redis"].(map[string]interface{}); ok {
		redisConfig, err := common.ParseRedisConfig(redisConfigMap)
		if err != nil {
			return nil, fmt.Errorf("failed to parse redis config: %w", err)
		}

		redisClient, err := common.NewRedisClient(redisConfig)
		if err != nil {
			api.LogErrorf("Failed to initialize Redis client: %v", err)
		} else {
			api.LogDebug("Redis client initialized")
		}
		conf.redisClient = redisClient
	} else {
		api.LogDebug("Redis configuration not provided, running without Redis")
	}

	ssePathSuffix, ok := v.AsMap()["sse_path_suffix"].(string)
	if !ok || ssePathSuffix == "" {
		return nil, fmt.Errorf("sse path suffix is not set or empty")
	}
	GlobalSSEPathSuffix = ssePathSuffix

	// Create shared MCPServer once during config parsing (thread-safe with sync.RWMutex).
	// Its name is protocol metadata only: sessions are bound to the per-rule
	// identity carried by the matched match_list entry.
	conf.sharedMCPServer = common.NewMCPServer(DefaultServerName, Version)
	if conf.redisClient != nil {
		conf.sessionRegistry = common.NewRedisSessionRegistry(conf.redisClient)
	}

	return conf, nil
}

func (p *Parser) Merge(parent interface{}, child interface{}) interface{} {
	parentConfig, ok := parent.(*config)
	if !ok || parentConfig == nil {
		return child
	}
	childConfig, ok := child.(*config)
	if !ok || childConfig == nil {
		return parentConfig
	}

	newConfig := *parentConfig
	if childConfig.matchList != nil {
		newConfig.matchList = childConfig.matchList
	}
	return &newConfig
}

func FilterFactory(c interface{}, callbacks api.FilterCallbackHandler) api.StreamFilter {
	conf, ok := c.(*config)
	if !ok {
		panic("unexpected config type")
	}
	return &filter{
		callbacks: callbacks,
		config:    conf,
		stopChan:  make(chan struct{}),
	}
}
