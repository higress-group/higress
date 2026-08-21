package mcp_session

import (
	"fmt"

	_ "net/http/pprof"

	xds "github.com/cncf/xds/go/xds/type/v3"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/alibaba/higress/plugins/golang-filter/mcp-session/common"
	"github.com/alibaba/higress/plugins/golang-filter/mcp-session/handler"
	"github.com/envoyproxy/envoy/contrib/golang/common/go/api"
)

const (
	Name              = "mcp-session"
	Version           = "1.0.0"
	ConfigPathSuffix  = "/config"
	DefaultServerName = "higress-mcp-server"
)

var GlobalSSEPathSuffix = "/sse"

type config struct {
	matchList             []common.MatchRule
	enableUserLevelServer bool
	rateLimitConfig       *handler.MCPRatelimitConfig
	redisClient           *common.RedisClient
	sharedMCPServer       *common.MCPServer // Created once, thread-safe with sync.RWMutex
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

	enableUserLevelServer, ok := v.AsMap()["enable_user_level_server"].(bool)
	if !ok {
		enableUserLevelServer = false
		if conf.redisClient == nil {
			return nil, fmt.Errorf("redis configuration is not provided, enable_user_level_server is true")
		}
	}
	conf.enableUserLevelServer = enableUserLevelServer

	if rateLimit, ok := v.AsMap()["rate_limit"].(map[string]interface{}); ok {
		rateLimitConfig := &handler.MCPRatelimitConfig{}
		if limit, ok := rateLimit["limit"].(float64); ok {
			rateLimitConfig.Limit = int(limit)
		}
		if window, ok := rateLimit["window"].(float64); ok {
			rateLimitConfig.Window = int(window)
		}
		if whiteList, ok := rateLimit["white_list"].([]interface{}); ok {
			for _, item := range whiteList {
				if uid, ok := item.(string); ok {
					rateLimitConfig.Whitelist = append(rateLimitConfig.Whitelist, uid)
				}
			}
		}
		if errorText, ok := rateLimit["error_text"].(string); ok {
			rateLimitConfig.ErrorText = errorText
		}
		conf.rateLimitConfig = rateLimitConfig
	}

	ssePathSuffix, ok := v.AsMap()["sse_path_suffix"].(string)
	if !ok || ssePathSuffix == "" {
		return nil, fmt.Errorf("sse path suffix is not set or empty")
	}
	GlobalSSEPathSuffix = ssePathSuffix

	// Create shared MCPServer once during config parsing (thread-safe with sync.RWMutex)
	conf.sharedMCPServer = common.NewMCPServer(DefaultServerName, Version)

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
	newConfig.enableUserLevelServer = childConfig.enableUserLevelServer
	if childConfig.rateLimitConfig != nil {
		newConfig.rateLimitConfig = childConfig.rateLimitConfig
	}
	return &newConfig
}

func FilterFactory(c interface{}, callbacks api.FilterCallbackHandler) api.StreamFilter {
	conf, ok := c.(*config)
	if !ok {
		panic("unexpected config type")
	}
	return &filter{
		callbacks:           callbacks,
		config:              conf,
		stopChan:            make(chan struct{}),
		mcpConfigHandler:    handler.NewMCPConfigHandler(conf.redisClient, callbacks),
		mcpRatelimitHandler: handler.NewMCPRatelimitHandler(conf.redisClient, callbacks, conf.rateLimitConfig),
	}
}
