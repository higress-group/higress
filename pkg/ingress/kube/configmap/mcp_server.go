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

package configmap

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"

	networking "istio.io/api/networking/v1alpha3"
	"istio.io/istio/pkg/config"
	"istio.io/istio/pkg/config/schema/gvk"

	"github.com/alibaba/higress/v2/pkg/ingress/kube/mcpserver"
	"github.com/alibaba/higress/v2/pkg/ingress/kube/util"
	. "github.com/alibaba/higress/v2/pkg/ingress/log"
)

// RedisConfig defines the configuration for Redis connection
type RedisConfig struct {
	// The address of Redis server in the format of "host:port"
	Address string `json:"address,omitempty"`
	// The username for Redis authentication
	Username string `json:"username,omitempty"`
	// The password for Redis authentication
	Password string `json:"password,omitempty"`
	// Reference to a secret containing the password
	PasswordSecret *SecretKeyReference `json:"passwordSecret,omitempty"`
	// The database index to use
	DB int `json:"db,omitempty"`
}

// SecretKeyReference defines a reference to a key within a Kubernetes secret
type SecretKeyReference struct {
	// The namespace of the secret. Defaults to the higress system namespace.
	Namespace string `json:"namespace,omitempty"`
	// The name of the secret
	Name string `json:"name,omitempty"`
	// The key within the secret data
	Key string `json:"key,omitempty"`
}

// MCPRatelimitConfig defines the configuration for rate limit
type MCPRatelimitConfig struct {
	// The limit of the rate limit
	Limit int64 `json:"limit,omitempty"`
	// The window of the rate limit
	Window int64 `json:"window,omitempty"`
	// The white list of the rate limit
	WhiteList []string `json:"white_list,omitempty"`
}

// SSEServer defines the configuration for Server-Sent Events (SSE) server
type SSEServer struct {
	// The name of the SSE server
	Name string `json:"name,omitempty"`
	// The path where the SSE server will be mounted, the full path is (PATH + SSEPathSuffix)
	Path string `json:"path,omitempty"`
	// The type of the SSE server
	Type string `json:"type,omitempty"`
	// Additional Config parameters for the real MCP server implementation
	Config map[string]interface{} `json:"config,omitempty"`
	// The domain list of the SSE server
	DomainList []string `json:"domain_list,omitempty"`
}

// MatchRule defines a rule for matching requests
type MatchRule struct {
	// Domain pattern, supports wildcards
	MatchRuleDomain string `json:"match_rule_domain,omitempty"`
	// Path pattern to match
	MatchRulePath string `json:"match_rule_path,omitempty"`
	// Type of match rule: exact, prefix, suffix, contains, regex
	MatchRuleType string `json:"match_rule_type,omitempty"`
	// Type of upstream(s) matched by the rule: rest (default), sse
	UpstreamType string `json:"upstream_type"`
	// Enable request path rewrite for matched routes
	EnablePathRewrite bool `json:"enable_path_rewrite"`
	// Prefix the request path would be rewritten to.
	PathRewritePrefix string `json:"path_rewrite_prefix"`
}

// McpServer defines the configuration for MCP (Model Context Protocol) server
type McpServer struct {
	// Flag to control whether MCP server is enabled
	Enable bool `json:"enable,omitempty"`
	// Redis Config for MCP server
	Redis *RedisConfig `json:"redis,omitempty"`
	// The suffix to be appended to SSE paths, default is "/sse"
	SSEPathSuffix string `json:"sse_path_suffix,omitempty"`
	// List of SSE servers Configs
	Servers []*SSEServer `json:"servers,omitempty"`
	// List of match rules for filtering requests
	MatchList []*MatchRule `json:"match_list,omitempty"`
	// Flag to control whether user level server is enabled
	EnableUserLevelServer bool `json:"enable_user_level_server,omitempty"`
	// Rate limit config for MCP server
	Ratelimit *MCPRatelimitConfig `json:"rate_limit,omitempty"`
}

func NewDefaultMcpServer() *McpServer {
	return &McpServer{
		Enable:                false,
		Servers:               make([]*SSEServer, 0),
		MatchList:             make([]*MatchRule, 0),
		EnableUserLevelServer: false,
	}
}

const (
	higressMcpServerEnvoyFilterName = "higress-config-mcp-server"
)

func validMcpServer(m *McpServer) error {
	if m == nil {
		return nil
	}

	if m.Redis != nil && m.Redis.PasswordSecret != nil {
		if m.Redis.PasswordSecret.Name == "" {
			return errors.New("redis passwordSecret.name cannot be empty")
		}
		if m.Redis.PasswordSecret.Key == "" {
			return errors.New("redis passwordSecret.key cannot be empty")
		}
	}

	if m.EnableUserLevelServer && m.Redis == nil {
		return errors.New("redis config cannot be empty when user level server is enabled")
	}

	// Validate match rule types
	if m.MatchList != nil {
		validMatchRuleTypes := map[string]bool{
			"exact":    true,
			"prefix":   true,
			"suffix":   true,
			"contains": true,
			"regex":    true,
		}
		validUpstreamTypes := map[string]bool{
			"rest":       true,
			"sse":        true,
			"streamable": true,
		}

		for _, rule := range m.MatchList {
			if rule.MatchRuleType == "" {
				return errors.New("match_rule_type cannot be empty, must be one of: exact, prefix, suffix, contains, regex")
			}
			if !validMatchRuleTypes[rule.MatchRuleType] {
				return fmt.Errorf("invalid match_rule_type: %s, must be one of: exact, prefix, suffix, contains, regex", rule.MatchRuleType)
			}
			if rule.UpstreamType != "" && !validUpstreamTypes[rule.UpstreamType] {
				return fmt.Errorf("invalid upstream_type: %s, must be one of: rest, sse, streamable", rule.UpstreamType)
			}
			if rule.EnablePathRewrite && rule.UpstreamType != "sse" {
				return errors.New("path rewrite is only supported for SSE upstream type")
			}
		}
	}

	return nil
}

func compareMcpServer(old *McpServer, new *McpServer) (Result, error) {
	if old == nil && new == nil {
		return ResultNothing, nil
	}

	if new == nil {
		return ResultDelete, nil
	}

	if !reflect.DeepEqual(old, new) {
		return ResultReplace, nil
	}

	return ResultNothing, nil
}

func deepCopyMcpServer(mcp *McpServer) (*McpServer, error) {
	newMcp := NewDefaultMcpServer()
	newMcp.Enable = mcp.Enable

	if mcp.Redis != nil {
		newMcp.Redis = &RedisConfig{
			Address:  mcp.Redis.Address,
			Username: mcp.Redis.Username,
			Password: mcp.Redis.Password,
			DB:       mcp.Redis.DB,
		}
		if mcp.Redis.PasswordSecret != nil {
			newMcp.Redis.PasswordSecret = &SecretKeyReference{
				Namespace: mcp.Redis.PasswordSecret.Namespace,
				Name:      mcp.Redis.PasswordSecret.Name,
				Key:       mcp.Redis.PasswordSecret.Key,
			}
		}
	}
	if mcp.Ratelimit != nil {
		newMcp.Ratelimit = &MCPRatelimitConfig{
			Limit:     mcp.Ratelimit.Limit,
			Window:    mcp.Ratelimit.Window,
			WhiteList: mcp.Ratelimit.WhiteList,
		}
	}
	newMcp.SSEPathSuffix = mcp.SSEPathSuffix

	newMcp.EnableUserLevelServer = mcp.EnableUserLevelServer

	if len(mcp.Servers) > 0 {
		newMcp.Servers = make([]*SSEServer, len(mcp.Servers))
		for i, server := range mcp.Servers {
			newServer := &SSEServer{
				Name:       server.Name,
				Path:       server.Path,
				Type:       server.Type,
				DomainList: server.DomainList,
			}
			if server.Config != nil {
				newServer.Config = make(map[string]interface{})
				for k, v := range server.Config {
					newServer.Config[k] = v
				}
			}
			newMcp.Servers[i] = newServer
		}
	}

	if len(mcp.MatchList) > 0 {
		newMcp.MatchList = make([]*MatchRule, len(mcp.MatchList))
		for i, rule := range mcp.MatchList {
			newMcp.MatchList[i] = &MatchRule{
				MatchRuleDomain:   rule.MatchRuleDomain,
				MatchRulePath:     rule.MatchRulePath,
				MatchRuleType:     rule.MatchRuleType,
				UpstreamType:      rule.UpstreamType,
				EnablePathRewrite: rule.EnablePathRewrite,
				PathRewritePrefix: rule.PathRewritePrefix,
			}
		}
	}

	return newMcp, nil
}

type McpServerController struct {
	Namespace          string
	mcpServer          atomic.Value
	Name               string
	eventHandler       ItemEventHandler
	mcpServerProviders map[mcpserver.McpServerProvider]bool
}

func NewMcpServerController(namespace string) *McpServerController {
	mcpController := &McpServerController{
		Namespace:          namespace,
		Name:               "mcpServer",
		mcpServer:          atomic.Value{},
		mcpServerProviders: make(map[mcpserver.McpServerProvider]bool),
	}
	mcpController.SetMcpServer(NewDefaultMcpServer())
	return mcpController
}

func (m *McpServerController) GetName() string {
	return m.Name
}

func (m *McpServerController) SetMcpServer(mcp *McpServer) {
	m.mcpServer.Store(mcp)
}

func (m *McpServerController) GetMcpServer() *McpServer {
	value := m.mcpServer.Load()
	if value != nil {
		if mcp, ok := value.(*McpServer); ok {
			return mcp
		}
	}
	return nil
}

func (m *McpServerController) AddOrUpdateHigressConfig(name util.ClusterNamespacedName, old *HigressConfig, new *HigressConfig) error {
	if err := validMcpServer(new.McpServer); err != nil {
		IngressLog.Errorf("data:%+v convert to mcp server, error: %+v", new.McpServer, err)
		return nil
	}

	result, _ := compareMcpServer(old.McpServer, new.McpServer)

	switch result {
	case ResultReplace:
		if newMcp, err := deepCopyMcpServer(new.McpServer); err != nil {
			IngressLog.Infof("mcp server deepcopy error:%v", err)
		} else {
			m.SetMcpServer(newMcp)
			IngressLog.Infof("AddOrUpdate Higress config mcp server")
			m.eventHandler(higressMcpServerEnvoyFilterName)
			IngressLog.Infof("send event with filter name:%s", higressMcpServerEnvoyFilterName)
		}
	case ResultDelete:
		m.SetMcpServer(NewDefaultMcpServer())
		IngressLog.Infof("Delete Higress config mcp server")
		m.eventHandler(higressMcpServerEnvoyFilterName)
		IngressLog.Infof("send event with filter name:%s", higressMcpServerEnvoyFilterName)
	}

	return nil
}

func (m *McpServerController) ValidHigressConfig(higressConfig *HigressConfig) error {
	if higressConfig == nil {
		return nil
	}
	if higressConfig.McpServer == nil {
		return nil
	}

	return validMcpServer(higressConfig.McpServer)
}

func (m *McpServerController) RegisterItemEventHandler(eventHandler ItemEventHandler) {
	m.eventHandler = eventHandler
}

func (m *McpServerController) RegisterMcpServerProvider(provider mcpserver.McpServerProvider) {
	if m.mcpServerProviders == nil {
		m.mcpServerProviders = make(map[mcpserver.McpServerProvider]bool)
	}
	m.mcpServerProviders[provider] = true
}

func (m *McpServerController) ConstructEnvoyFilters() ([]*config.Config, error) {
	configs := make([]*config.Config, 0)
	mcpServer := m.GetMcpServer()
	namespace := m.Namespace

	if mcpServer == nil || !mcpServer.Enable {
		return configs, nil
	}

	// mcp-session envoy filter with ECDS
	mcpSessionStruct := m.constructMcpSessionStruct(mcpServer)
	if mcpSessionStruct != "" {
		// HTTP_FILTER configuration with config_discovery reference
		sessionFilterRef := `{
			"name": "golang-filter-mcp-session",
			"config_discovery": {
				"config_source": {
					"ads": {}
				},
				"type_urls": ["type.googleapis.com/envoy.extensions.filters.http.golang.v3alpha.Config"]
			}
		}`

		// EXTENSION_CONFIG configuration with actual filter config
		sessionExtensionConfig := fmt.Sprintf(`{
			"name": "golang-filter-mcp-session",
			"typed_config": %s
		}`, mcpSessionStruct)

		sessionConfig := &config.Config{
			Meta: config.Meta{
				GroupVersionKind: gvk.EnvoyFilter,
				Name:             higressMcpServerEnvoyFilterName,
				Namespace:        namespace,
			},
			Spec: &networking.EnvoyFilter{
				ConfigPatches: []*networking.EnvoyFilter_EnvoyConfigObjectPatch{
					{
						ApplyTo: networking.EnvoyFilter_HTTP_FILTER,
						Match: &networking.EnvoyFilter_EnvoyConfigObjectMatch{
							Context: networking.EnvoyFilter_GATEWAY,
							ObjectTypes: &networking.EnvoyFilter_EnvoyConfigObjectMatch_Listener{
								Listener: &networking.EnvoyFilter_ListenerMatch{
									FilterChain: &networking.EnvoyFilter_ListenerMatch_FilterChainMatch{
										Filter: &networking.EnvoyFilter_ListenerMatch_FilterMatch{
											Name: "envoy.filters.network.http_connection_manager",
											SubFilter: &networking.EnvoyFilter_ListenerMatch_SubFilterMatch{
												Name: "envoy.filters.http.cors",
											},
										},
									},
								},
							},
						},
						Patch: &networking.EnvoyFilter_Patch{
							Operation: networking.EnvoyFilter_Patch_INSERT_AFTER,
							Value:     util.BuildPatchStruct(sessionFilterRef),
						},
					},
					{
						ApplyTo: networking.EnvoyFilter_EXTENSION_CONFIG,
						Patch: &networking.EnvoyFilter_Patch{
							Operation: networking.EnvoyFilter_Patch_ADD,
							Value:     util.BuildPatchStruct(sessionExtensionConfig),
						},
					},
				},
			},
		}
		configs = append(configs, sessionConfig)
	}

	// mcp-server envoy filter with ECDS
	mcpServerStruct := m.constructMcpServerStruct(mcpServer)
	if mcpServerStruct != "" {
		// HTTP_FILTER configuration with config_discovery reference
		serverFilterRef := `{
			"name": "golang-filter-mcp-server",
			"config_discovery": {
				"config_source": {
					"ads": {}
				},
				"type_urls": ["type.googleapis.com/envoy.extensions.filters.http.golang.v3alpha.Config"]
			}
		}`

		// EXTENSION_CONFIG configuration with actual filter config
		serverExtensionConfig := fmt.Sprintf(`{
			"name": "golang-filter-mcp-server",
			"typed_config": %s
		}`, mcpServerStruct)

		serverConfig := &config.Config{
			Meta: config.Meta{
				GroupVersionKind: gvk.EnvoyFilter,
				Name:             higressMcpServerEnvoyFilterName + "-server",
				Namespace:        namespace,
			},
			Spec: &networking.EnvoyFilter{
				ConfigPatches: []*networking.EnvoyFilter_EnvoyConfigObjectPatch{
					{
						ApplyTo: networking.EnvoyFilter_HTTP_FILTER,
						Match: &networking.EnvoyFilter_EnvoyConfigObjectMatch{
							Context: networking.EnvoyFilter_GATEWAY,
							ObjectTypes: &networking.EnvoyFilter_EnvoyConfigObjectMatch_Listener{
								Listener: &networking.EnvoyFilter_ListenerMatch{
									FilterChain: &networking.EnvoyFilter_ListenerMatch_FilterChainMatch{
										Filter: &networking.EnvoyFilter_ListenerMatch_FilterMatch{
											Name: "envoy.filters.network.http_connection_manager",
											SubFilter: &networking.EnvoyFilter_ListenerMatch_SubFilterMatch{
												Name: "envoy.filters.http.router",
											},
										},
									},
								},
							},
						},
						Patch: &networking.EnvoyFilter_Patch{
							Operation: networking.EnvoyFilter_Patch_INSERT_BEFORE,
							Value:     util.BuildPatchStruct(serverFilterRef),
						},
					},
					{
						ApplyTo: networking.EnvoyFilter_EXTENSION_CONFIG,
						Patch: &networking.EnvoyFilter_Patch{
							Operation: networking.EnvoyFilter_Patch_ADD,
							Value:     util.BuildPatchStruct(serverExtensionConfig),
						},
					},
				},
			},
		}
		configs = append(configs, serverConfig)
	}

	return configs, nil
}

func (m *McpServerController) constructMcpSessionStruct(mcp *McpServer) string {
	// Build match_list configuration
	var matchList []*MatchRule
	matchList = append(matchList, mcp.MatchList...)
	for provider := range m.mcpServerProviders {
		servers := provider.GetMcpServers()
		if len(servers) == 0 {
			continue
		}
		for _, server := range servers {
			domains := server.Domains
			if len(domains) == 0 {
				// Preserve the existing rule generated for providers without domains.
				domains = []string{""}
			}
			for _, domain := range domains {
				matchList = append(matchList, &MatchRule{
					MatchRuleDomain:   domain,
					MatchRuleType:     server.PathMatchType,
					MatchRulePath:     server.PathMatchValue,
					UpstreamType:      server.UpstreamType,
					EnablePathRewrite: server.EnablePathRewrite,
					PathRewritePrefix: server.PathRewritePrefix,
				})
			}
		}
	}
	matchListConfig := "[]"
	if len(matchList) > 0 {
		matchConfigs := make([]string, 0, len(matchList))
		for _, rule := range matchList {
			matchConfigs = append(matchConfigs, fmt.Sprintf(`{
				"match_rule_domain": "%s",
				"match_rule_path": "%s",
				"match_rule_type": "%s",
				"upstream_type": "%s",
				"enable_path_rewrite": %t,
				"path_rewrite_prefix": "%s"
			}`, rule.MatchRuleDomain, rule.MatchRulePath, rule.MatchRuleType, rule.UpstreamType, rule.EnablePathRewrite, rule.PathRewritePrefix))
		}
		matchListConfig = fmt.Sprintf("[%s]", strings.Join(matchConfigs, ","))
	}

	// Build redis configuration
	redisConfig := "null"
	if mcp.Redis != nil {
		passwordValue := mcp.Redis.Password
		if mcp.Redis.PasswordSecret != nil && mcp.Redis.PasswordSecret.Name != "" && mcp.Redis.PasswordSecret.Key != "" {
			ns := mcp.Redis.PasswordSecret.Namespace
			if ns == "" {
				ns = m.Namespace
			}
			if ns != "" {
				passwordValue = fmt.Sprintf("${secret.%s/%s.%s}", ns, mcp.Redis.PasswordSecret.Name, mcp.Redis.PasswordSecret.Key)
			} else {
				passwordValue = fmt.Sprintf("${secret.%s.%s}", mcp.Redis.PasswordSecret.Name, mcp.Redis.PasswordSecret.Key)
			}
		}
		redisConfig = fmt.Sprintf(`{
							"address": "%s",
							"username": "%s",
							"password": "%s",
							"db": %d
						}`, mcp.Redis.Address, mcp.Redis.Username, passwordValue, mcp.Redis.DB)
	}

	// Build rate limit configuration
	rateLimitConfig := "null"
	if mcp.Ratelimit != nil {
		whiteList := "[]"
		if len(mcp.Ratelimit.WhiteList) > 0 {
			whiteList = fmt.Sprintf(`["%s"]`, strings.Join(mcp.Ratelimit.WhiteList, `","`))
		}
		rateLimitConfig = fmt.Sprintf(`{
							"limit": %d,
							"window": %d,
							"white_list": %s
						}`, mcp.Ratelimit.Limit, mcp.Ratelimit.Window, whiteList)
	}

	// Build complete configuration structure for EXTENSION_CONFIG
	return fmt.Sprintf(`{
		"@type": "type.googleapis.com/envoy.extensions.filters.http.golang.v3alpha.Config",
		"library_id": "mcp-session",
		"library_path": "/var/lib/istio/envoy/golang-filter.so",
		"plugin_name": "mcp-session",
		"plugin_config": {
			"@type": "type.googleapis.com/xds.type.v3.TypedStruct",
			"value": {
				"redis": %s,
				"rate_limit": %s,
				"sse_path_suffix": "%s",
				"match_list": %s,
				"enable_user_level_server": %t
			}
		}
	}`,
		redisConfig,
		rateLimitConfig,
		mcp.SSEPathSuffix,
		matchListConfig,
		mcp.EnableUserLevelServer)
}

func (m *McpServerController) constructMcpServerStruct(mcp *McpServer) string {
	// if no servers, return empty string
	if mcp == nil || len(mcp.Servers) == 0 {
		return ""
	}

	// Build servers configuration
	servers := "[]"
	if len(mcp.Servers) > 0 {
		serverConfigs := make([]string, len(mcp.Servers))
		for i, server := range mcp.Servers {
			serverConfig := fmt.Sprintf(`{
				"name": "%s",
				"path": "%s",
				"type": "%s"`,
				server.Name, server.Path, server.Type)
			if len(server.DomainList) > 0 {
				domainList := fmt.Sprintf(`["%s"]`, strings.Join(server.DomainList, `","`))
				serverConfig += fmt.Sprintf(`,
				"domain_list": %s`, domainList)
			}
			if len(server.Config) > 0 {
				config, _ := json.Marshal(server.Config)
				serverConfig += fmt.Sprintf(`,
				"config": %s`, string(config))
			}
			serverConfig += "}"
			serverConfigs[i] = serverConfig
		}
		servers = fmt.Sprintf("[%s]", strings.Join(serverConfigs, ","))
	}

	// Build complete configuration structure for EXTENSION_CONFIG
	return fmt.Sprintf(`{
		"@type": "type.googleapis.com/envoy.extensions.filters.http.golang.v3alpha.Config",
		"library_id": "mcp-server",
		"library_path": "/var/lib/istio/envoy/golang-filter.so",
		"plugin_name": "mcp-server",
		"plugin_config": {
			"@type": "type.googleapis.com/xds.type.v3.TypedStruct",
			"value": {
				"servers": %s
			}
		}
	}`, servers)
}
