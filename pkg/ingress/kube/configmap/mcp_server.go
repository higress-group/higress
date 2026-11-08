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
	"sync/atomic"

	networking "istio.io/api/networking/v1alpha3"
	"istio.io/istio/pkg/config"
	"istio.io/istio/pkg/config/schema/gvk"

	_struct "github.com/golang/protobuf/ptypes/struct"

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
	// Name of the MCP server that owns this rule. When empty, mcp-session
	// binds sessions to the rule's own match key.
	ServerName string `json:"server_name,omitempty"`
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
}

func NewDefaultMcpServer() *McpServer {
	return &McpServer{
		Enable:    false,
		Servers:   make([]*SSEServer, 0),
		MatchList: make([]*MatchRule, 0),
	}
}

const (
	higressMcpServerEnvoyFilterName = "higress-config-mcp-server"

	golangFilterNamePrefix  = "golang-filter-"
	golangFilterConfigType  = "type.googleapis.com/envoy.extensions.filters.http.golang.v3alpha.Config"
	golangFilterLibraryPath = "/var/lib/istio/envoy/golang-filter.so"
	typedStructType         = "type.googleapis.com/xds.type.v3.TypedStruct"

	mcpSessionPluginName = "mcp-session"
	mcpServerPluginName  = "mcp-server"
)

// golangFilterRef is the HTTP_FILTER patch that makes Envoy fetch the golang
// filter configuration over ECDS.
type golangFilterRef struct {
	Name            string                   `json:"name"`
	ConfigDiscovery golangFilterDiscoveryRef `json:"config_discovery"`
}

type golangFilterDiscoveryRef struct {
	ConfigSource golangFilterADSConfig `json:"config_source"`
	TypeURLs     []string              `json:"type_urls"`
}

type golangFilterADSConfig struct {
	ADS struct{} `json:"ads"`
}

// golangFilterExtensionConfig is the EXTENSION_CONFIG patch that carries the
// golang filter configuration itself.
type golangFilterExtensionConfig struct {
	Name        string          `json:"name"`
	TypedConfig json.RawMessage `json:"typed_config"`
}

type golangFilterConfig struct {
	Type         string             `json:"@type"`
	LibraryID    string             `json:"library_id"`
	LibraryPath  string             `json:"library_path"`
	PluginName   string             `json:"plugin_name"`
	PluginConfig golangPluginConfig `json:"plugin_config"`
}

type golangPluginConfig struct {
	Type  string      `json:"@type"`
	Value interface{} `json:"value"`
}

// mcpSessionMatchRule is the wire form of a match_list entry. Match fields are
// always emitted, matching what mcp-session's ParseMatchList expects;
// server_name is omitted when the rule has no server identity.
type mcpSessionMatchRule struct {
	ServerName        string `json:"server_name,omitempty"`
	MatchRuleDomain   string `json:"match_rule_domain"`
	MatchRulePath     string `json:"match_rule_path"`
	MatchRuleType     string `json:"match_rule_type"`
	UpstreamType      string `json:"upstream_type"`
	EnablePathRewrite bool   `json:"enable_path_rewrite"`
	PathRewritePrefix string `json:"path_rewrite_prefix"`
}

type mcpSessionRedisConfig struct {
	Address  string `json:"address"`
	Username string `json:"username"`
	Password string `json:"password"`
	DB       int    `json:"db"`
}

type mcpSessionPluginValue struct {
	Redis         *mcpSessionRedisConfig `json:"redis"`
	SSEPathSuffix string                 `json:"sse_path_suffix"`
	MatchList     []mcpSessionMatchRule  `json:"match_list"`
}

// mcpServerEntry is the wire form of a servers entry. domain_list and config
// are omitted when empty, matching the previously emitted shape.
type mcpServerEntry struct {
	Name       string                 `json:"name"`
	Path       string                 `json:"path"`
	Type       string                 `json:"type"`
	DomainList []string               `json:"domain_list,omitempty"`
	Config     map[string]interface{} `json:"config,omitempty"`
}

type mcpServerPluginValue struct {
	Servers []mcpServerEntry `json:"servers"`
}

func newGolangFilterConfig(pluginName string, value interface{}) golangFilterConfig {
	return golangFilterConfig{
		Type:        golangFilterConfigType,
		LibraryID:   pluginName,
		LibraryPath: golangFilterLibraryPath,
		PluginName:  pluginName,
		PluginConfig: golangPluginConfig{
			Type:  typedStructType,
			Value: value,
		},
	}
}

func newMcpSessionMatchRule(rule *MatchRule) mcpSessionMatchRule {
	return mcpSessionMatchRule{
		ServerName:        rule.ServerName,
		MatchRuleDomain:   rule.MatchRuleDomain,
		MatchRulePath:     rule.MatchRulePath,
		MatchRuleType:     rule.MatchRuleType,
		UpstreamType:      rule.UpstreamType,
		EnablePathRewrite: rule.EnablePathRewrite,
		PathRewritePrefix: rule.PathRewritePrefix,
	}
}

// buildGolangFilterPatches returns the HTTP_FILTER patch that references the
// ECDS-delivered config and the EXTENSION_CONFIG patch carrying it.
func buildGolangFilterPatches(pluginName string, typedConfig json.RawMessage) (*_struct.Struct, *_struct.Struct, error) {
	filterRef, err := json.Marshal(golangFilterRef{
		Name: golangFilterNamePrefix + pluginName,
		ConfigDiscovery: golangFilterDiscoveryRef{
			TypeURLs: []string{golangFilterConfigType},
		},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal %s http filter reference: %v", pluginName, err)
	}
	filterRefValue, err := util.BuildPatchStruct(string(filterRef))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build %s http filter patch: %v", pluginName, err)
	}

	extensionConfig, err := json.Marshal(golangFilterExtensionConfig{
		Name:        golangFilterNamePrefix + pluginName,
		TypedConfig: typedConfig,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal %s extension config: %v", pluginName, err)
	}
	extensionConfigValue, err := util.BuildPatchStruct(string(extensionConfig))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build %s extension config patch: %v", pluginName, err)
	}

	return filterRefValue, extensionConfigValue, nil
}

var (
	validMatchRuleTypes = map[string]bool{
		"exact":    true,
		"prefix":   true,
		"suffix":   true,
		"contains": true,
		"regex":    true,
	}
	validUpstreamTypes = map[string]bool{
		"rest":       true,
		"sse":        true,
		"streamable": true,
	}
)

// validMatchRule validates a single match rule. It is applied to ConfigMap
// MatchList entries and to rules derived from Ingress annotations so that both
// sources are held to the same constraints before reaching the filter config.
func validMatchRule(rule *MatchRule) error {
	if rule == nil {
		return errors.New("match rule cannot be nil")
	}
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
	if err := mcpserver.ValidateRuleValue("server_name", rule.ServerName); err != nil {
		return err
	}
	// An empty domain is allowed and matches any host, so only the character
	// constraints are enforced here.
	return mcpserver.ValidateRuleContent([]string{rule.MatchRuleDomain}, rule.MatchRulePath, rule.PathRewritePrefix)
}

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

	// Validate match rule types
	for _, rule := range m.MatchList {
		if err := validMatchRule(rule); err != nil {
			return err
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
	newMcp.SSEPathSuffix = mcp.SSEPathSuffix

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
				ServerName:        rule.ServerName,
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
	mcpSessionStruct, err := m.constructMcpSessionStruct(mcpServer)
	if err != nil {
		return nil, err
	}
	if mcpSessionStruct != "" {
		sessionFilterRef, sessionExtensionConfig, err := buildGolangFilterPatches(mcpSessionPluginName, json.RawMessage(mcpSessionStruct))
		if err != nil {
			return nil, err
		}

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
							Value:     sessionFilterRef,
						},
					},
					{
						ApplyTo: networking.EnvoyFilter_EXTENSION_CONFIG,
						Patch: &networking.EnvoyFilter_Patch{
							Operation: networking.EnvoyFilter_Patch_ADD,
							Value:     sessionExtensionConfig,
						},
					},
				},
			},
		}
		configs = append(configs, sessionConfig)
	}

	// mcp-server envoy filter with ECDS
	mcpServerStruct, err := m.constructMcpServerStruct(mcpServer)
	if err != nil {
		return nil, err
	}
	if mcpServerStruct != "" {
		serverFilterRef, serverExtensionConfig, err := buildGolangFilterPatches(mcpServerPluginName, json.RawMessage(mcpServerStruct))
		if err != nil {
			return nil, err
		}

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
							Value:     serverFilterRef,
						},
					},
					{
						ApplyTo: networking.EnvoyFilter_EXTENSION_CONFIG,
						Patch: &networking.EnvoyFilter_Patch{
							Operation: networking.EnvoyFilter_Patch_ADD,
							Value:     serverExtensionConfig,
						},
					},
				},
			},
		}
		configs = append(configs, serverConfig)
	}

	return configs, nil
}

func (m *McpServerController) constructMcpSessionStruct(mcp *McpServer) (string, error) {
	// Build match_list configuration
	matchList := make([]mcpSessionMatchRule, 0, len(mcp.MatchList))
	for _, rule := range mcp.MatchList {
		matchList = append(matchList, newMcpSessionMatchRule(rule))
	}
	for provider := range m.mcpServerProviders {
		servers := provider.GetMcpServers()
		if len(servers) == 0 {
			continue
		}
		for _, server := range servers {
			// Provider rules are derived from Ingress annotations, so they go
			// through the same content validation as ConfigMap MatchList entries.
			// A rejected server is skipped rather than rendered into the filter
			// config.
			if err := mcpserver.ValidateRuleContent(server.Domains, server.PathMatchValue, server.PathRewritePrefix); err != nil {
				IngressLog.Errorf("mcp server %s: skip invalid match rule: %v", server.Name, err)
				continue
			}
			domains := server.Domains
			if len(domains) == 0 {
				// Preserve the existing rule generated for providers without domains.
				domains = []string{""}
			}
			for _, domain := range domains {
				matchList = append(matchList, mcpSessionMatchRule{
					ServerName:        server.Name,
					MatchRuleDomain:   domain,
					MatchRulePath:     server.PathMatchValue,
					MatchRuleType:     server.PathMatchType,
					UpstreamType:      server.UpstreamType,
					EnablePathRewrite: server.EnablePathRewrite,
					PathRewritePrefix: server.PathRewritePrefix,
				})
			}
		}
	}

	value := mcpSessionPluginValue{
		SSEPathSuffix: mcp.SSEPathSuffix,
		MatchList:     matchList,
	}

	// Build redis configuration
	if mcp.Redis != nil {
		value.Redis = &mcpSessionRedisConfig{
			Address:  mcp.Redis.Address,
			Username: mcp.Redis.Username,
			Password: m.redisPasswordValue(mcp.Redis),
			DB:       mcp.Redis.DB,
		}
	}

	// Build complete configuration structure for EXTENSION_CONFIG
	return marshalGolangFilterConfig(mcpSessionPluginName, value)
}

// redisPasswordValue resolves the redis password, preferring an indirection to
// the referenced Secret over the inline password.
func (m *McpServerController) redisPasswordValue(redis *RedisConfig) string {
	if redis.PasswordSecret == nil || redis.PasswordSecret.Name == "" || redis.PasswordSecret.Key == "" {
		return redis.Password
	}
	namespace := redis.PasswordSecret.Namespace
	if namespace == "" {
		namespace = m.Namespace
	}
	if namespace == "" {
		return fmt.Sprintf("${secret.%s.%s}", redis.PasswordSecret.Name, redis.PasswordSecret.Key)
	}
	return fmt.Sprintf("${secret.%s/%s.%s}", namespace, redis.PasswordSecret.Name, redis.PasswordSecret.Key)
}

func (m *McpServerController) constructMcpServerStruct(mcp *McpServer) (string, error) {
	// if no servers, return empty string
	if mcp == nil || len(mcp.Servers) == 0 {
		return "", nil
	}

	// Build servers configuration
	servers := make([]mcpServerEntry, 0, len(mcp.Servers))
	for _, server := range mcp.Servers {
		servers = append(servers, mcpServerEntry{
			Name:       server.Name,
			Path:       server.Path,
			Type:       server.Type,
			DomainList: server.DomainList,
			Config:     server.Config,
		})
	}

	// Build complete configuration structure for EXTENSION_CONFIG
	return marshalGolangFilterConfig(mcpServerPluginName, mcpServerPluginValue{Servers: servers})
}

func marshalGolangFilterConfig(pluginName string, value interface{}) (string, error) {
	data, err := json.Marshal(newGolangFilterConfig(pluginName, value))
	if err != nil {
		return "", fmt.Errorf("failed to marshal %s filter config: %v", pluginName, err)
	}
	return string(data), nil
}
