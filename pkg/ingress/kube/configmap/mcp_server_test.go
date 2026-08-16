// Copyright (c) 2022 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
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
	"testing"

	"github.com/alibaba/higress/v2/pkg/ingress/kube/mcpserver"
	"github.com/alibaba/higress/v2/pkg/ingress/kube/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_validMcpServer(t *testing.T) {
	tests := []struct {
		name    string
		mcp     *McpServer
		wantErr error
	}{
		{
			name: "default",
			mcp: &McpServer{
				Enable:    false,
				MatchList: []*MatchRule{},
				Servers:   []*SSEServer{},
			},
			wantErr: nil,
		},
		{
			name:    "nil",
			mcp:     nil,
			wantErr: nil,
		},
		{
			name: "enabled but no redis config",
			mcp: &McpServer{
				Enable:                true,
				EnableUserLevelServer: false,
				Redis:                 nil,
				MatchList:             []*MatchRule{},
				Servers:               []*SSEServer{},
			},
			wantErr: nil,
		},
		{
			name: "enabled but bad match_rule_type",
			mcp: &McpServer{
				Enable:                true,
				EnableUserLevelServer: false,
				Redis:                 nil,
				MatchList: []*MatchRule{
					{
						MatchRuleDomain: "*",
						MatchRulePath:   "/mcp",
						MatchRuleType:   "bad-type",
					},
				},
				Servers: []*SSEServer{},
			},
			wantErr: errors.New("invalid match_rule_type: bad-type, must be one of: exact, prefix, suffix, contains, regex"),
		},
		{
			name: "enabled but bad upstream_type",
			mcp: &McpServer{
				Enable:                true,
				EnableUserLevelServer: false,
				Redis:                 nil,
				MatchList: []*MatchRule{
					{
						MatchRuleDomain: "*",
						MatchRulePath:   "/mcp",
						MatchRuleType:   "prefix",
						UpstreamType:    "bad-type",
					},
				},
				Servers: []*SSEServer{},
			},
			wantErr: errors.New("invalid upstream_type: bad-type, must be one of: rest, sse, streamable"),
		},
		{
			name: "enabled but path rewrite with unsupported upstream type",
			mcp: &McpServer{
				Enable:                true,
				EnableUserLevelServer: false,
				Redis:                 nil,
				MatchList: []*MatchRule{
					{
						MatchRuleDomain:   "*",
						MatchRulePath:     "/mcp",
						MatchRuleType:     "prefix",
						UpstreamType:      "rest",
						EnablePathRewrite: true,
						PathRewritePrefix: "/",
					},
				},
				Servers: []*SSEServer{},
			},
			wantErr: errors.New("path rewrite is only supported for SSE upstream type"),
		},
		{
			name: "enabled with user level server but no redis config",
			mcp: &McpServer{
				Enable:                true,
				EnableUserLevelServer: true,
				Redis:                 nil,
				MatchList:             []*MatchRule{},
				Servers:               []*SSEServer{},
			},
			wantErr: errors.New("redis config cannot be empty when user level server is enabled"),
		},
		{
			name: "redis config with password secret missing name",
			mcp: &McpServer{
				Enable: true,
				Redis: &RedisConfig{
					PasswordSecret: &SecretKeyReference{
						Key: "password",
					},
				},
			},
			wantErr: errors.New("redis passwordSecret.name cannot be empty"),
		},
		{
			name: "redis config with password secret missing key",
			mcp: &McpServer{
				Enable: true,
				Redis: &RedisConfig{
					PasswordSecret: &SecretKeyReference{
						Name: "redis-credentials",
					},
				},
			},
			wantErr: errors.New("redis passwordSecret.key cannot be empty"),
		},
		{
			name: "valid config with redis",
			mcp: &McpServer{
				Enable:                true,
				EnableUserLevelServer: true,
				Redis: &RedisConfig{
					Address:  "localhost:6379",
					Username: "default",
					Password: "password",
					DB:       0,
				},
				SSEPathSuffix: "/sse",
				MatchList: []*MatchRule{
					{
						MatchRuleDomain: "*",
						MatchRulePath:   "*",
						MatchRuleType:   "exact",
					},
				},
				Servers: []*SSEServer{
					{
						Name: "test-server",
						Path: "/test",
						Type: "test",
						Config: map[string]interface{}{
							"key": "value",
						},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "valid config with redis password secret",
			mcp: &McpServer{
				Enable: true,
				Redis: &RedisConfig{
					Address: "localhost:6379",
					PasswordSecret: &SecretKeyReference{
						Name: "redis-credentials",
						Key:  "password",
					},
				},
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validMcpServer(tt.mcp)
			assert.Equal(t, tt.wantErr, err)
		})
	}
}

func Test_compareMcpServer(t *testing.T) {
	tests := []struct {
		name       string
		old        *McpServer
		new        *McpServer
		wantResult Result
		wantErr    error
	}{
		{
			name:       "compare both nil",
			old:        nil,
			new:        nil,
			wantResult: ResultNothing,
			wantErr:    nil,
		},
		{
			name: "compare result delete",
			old: &McpServer{
				Enable: true,
				Redis: &RedisConfig{
					Address: "localhost:6379",
				},
				MatchList: []*MatchRule{},
				Servers:   []*SSEServer{},
			},
			new:        nil,
			wantResult: ResultDelete,
			wantErr:    nil,
		},
		{
			name: "compare result equal",
			old: &McpServer{
				Enable: true,
				Redis: &RedisConfig{
					Address: "localhost:6379",
				},
				MatchList: []*MatchRule{},
				Servers:   []*SSEServer{},
			},
			new: &McpServer{
				Enable: true,
				Redis: &RedisConfig{
					Address: "localhost:6379",
				},
				MatchList: []*MatchRule{},
				Servers:   []*SSEServer{},
			},
			wantResult: ResultNothing,
			wantErr:    nil,
		},
		{
			name: "compare result replace",
			old: &McpServer{
				Enable: true,
				Redis: &RedisConfig{
					Address: "localhost:6379",
				},
				MatchList: []*MatchRule{},
				Servers:   []*SSEServer{},
			},
			new: &McpServer{
				Enable: true,
				Redis: &RedisConfig{
					Address: "redis:6379",
				},
				MatchList: []*MatchRule{
					{
						MatchRuleDomain: "*",
						MatchRulePath:   "/test",
						MatchRuleType:   "exact",
					},
				},
				Servers: []*SSEServer{},
			},
			wantResult: ResultReplace,
			wantErr:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := compareMcpServer(tt.old, tt.new)
			assert.Equal(t, tt.wantResult, result)
			assert.Equal(t, tt.wantErr, err)
		})
	}
}

func Test_deepCopyMcpServer(t *testing.T) {
	tests := []struct {
		name    string
		mcp     *McpServer
		wantMcp *McpServer
		wantErr error
	}{
		{
			name: "deep copy with redis only",
			mcp: &McpServer{
				Enable: true,
				Redis: &RedisConfig{
					Address:  "localhost:6379",
					Username: "default",
					Password: "password",
					PasswordSecret: &SecretKeyReference{
						Name: "redis-credentials",
						Key:  "password",
					},
					DB: 0,
				},
				MatchList: []*MatchRule{},
				Servers:   []*SSEServer{},
			},
			wantMcp: &McpServer{
				Enable: true,
				Redis: &RedisConfig{
					Address:  "localhost:6379",
					Username: "default",
					Password: "password",
					PasswordSecret: &SecretKeyReference{
						Name: "redis-credentials",
						Key:  "password",
					},
					DB: 0,
				},
				MatchList: []*MatchRule{},
				Servers:   []*SSEServer{},
			},
			wantErr: nil,
		},
		{
			name: "deep copy with full config",
			mcp: &McpServer{
				Enable: true,
				Redis: &RedisConfig{
					Address:  "localhost:6379",
					Username: "default",
					Password: "password",
					PasswordSecret: &SecretKeyReference{
						Name:      "redis-credentials",
						Namespace: "custom-ns",
						Key:       "password",
					},
					DB: 0,
				},
				SSEPathSuffix: "/sse",
				MatchList: []*MatchRule{
					{
						MatchRuleDomain: "*",
						MatchRulePath:   "*",
						MatchRuleType:   "exact",
					},
				},
				Servers: []*SSEServer{
					{
						Name: "test-server",
						Path: "/test",
						Type: "test",
						Config: map[string]interface{}{
							"key": "value",
						},
					},
				},
			},
			wantMcp: &McpServer{
				Enable: true,
				Redis: &RedisConfig{
					Address:  "localhost:6379",
					Username: "default",
					Password: "password",
					PasswordSecret: &SecretKeyReference{
						Name:      "redis-credentials",
						Namespace: "custom-ns",
						Key:       "password",
					},
					DB: 0,
				},
				SSEPathSuffix: "/sse",
				MatchList: []*MatchRule{
					{
						MatchRuleDomain: "*",
						MatchRulePath:   "*",
						MatchRuleType:   "exact",
					},
				},
				Servers: []*SSEServer{
					{
						Name: "test-server",
						Path: "/test",
						Type: "test",
						Config: map[string]interface{}{
							"key": "value",
						},
					},
				},
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mcp, err := deepCopyMcpServer(tt.mcp)
			assert.Equal(t, tt.wantMcp, mcp)
			assert.Equal(t, tt.wantErr, err)
		})
	}
}

func TestMcpServerController_AddOrUpdateHigressConfig(t *testing.T) {
	eventPush := "default"
	defaultHandler := func(name string) {
		eventPush = "push"
	}

	defaultName := util.ClusterNamespacedName{}

	tests := []struct {
		name          string
		old           *HigressConfig
		new           *HigressConfig
		wantErr       error
		wantEventPush string
		wantMcp       *McpServer
	}{
		{
			name: "default",
			old: &HigressConfig{
				McpServer: NewDefaultMcpServer(),
			},
			new: &HigressConfig{
				McpServer: NewDefaultMcpServer(),
			},
			wantErr:       nil,
			wantEventPush: "default",
			wantMcp:       NewDefaultMcpServer(),
		},
		{
			name: "replace and push - enable mcp server",
			old: &HigressConfig{
				McpServer: NewDefaultMcpServer(),
			},
			new: &HigressConfig{
				McpServer: &McpServer{
					Enable: true,
					Redis: &RedisConfig{
						Address:  "localhost:6379",
						Username: "default",
						Password: "password",
						DB:       0,
					},
					Servers:   []*SSEServer{},
					MatchList: []*MatchRule{},
				},
			},
			wantErr:       nil,
			wantEventPush: "push",
			wantMcp: &McpServer{
				Enable: true,
				Redis: &RedisConfig{
					Address:  "localhost:6379",
					Username: "default",
					Password: "password",
					DB:       0,
				},
				Servers:   []*SSEServer{},
				MatchList: []*MatchRule{},
			},
		},
		{
			name: "replace and push - update config",
			old: &HigressConfig{
				McpServer: &McpServer{
					Enable: true,
					Redis: &RedisConfig{
						Address: "localhost:6379",
					},
					Servers:   []*SSEServer{},
					MatchList: []*MatchRule{},
				},
			},
			new: &HigressConfig{
				McpServer: &McpServer{
					Enable: true,
					Redis: &RedisConfig{
						Address: "redis:6379",
					},
					Servers:   []*SSEServer{},
					MatchList: []*MatchRule{},
				},
			},
			wantErr:       nil,
			wantEventPush: "push",
			wantMcp: &McpServer{
				Enable: true,
				Redis: &RedisConfig{
					Address: "redis:6379",
				},
				Servers:   []*SSEServer{},
				MatchList: []*MatchRule{},
			},
		},
		{
			name: "delete and push",
			old: &HigressConfig{
				McpServer: &McpServer{
					Enable: true,
					Redis: &RedisConfig{
						Address: "localhost:6379",
					},
					Servers:   []*SSEServer{},
					MatchList: []*MatchRule{},
				},
			},
			new: &HigressConfig{
				McpServer: nil,
			},
			wantErr:       nil,
			wantEventPush: "push",
			wantMcp:       NewDefaultMcpServer(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMcpServerController("higress-system")
			m.eventHandler = defaultHandler
			eventPush = "default"
			err := m.AddOrUpdateHigressConfig(defaultName, tt.old, tt.new)
			assert.Equal(t, tt.wantEventPush, eventPush)
			assert.Equal(t, tt.wantErr, err)
			assert.Equal(t, tt.wantMcp, m.GetMcpServer())
		})
	}
}

func TestMcpServerController_ValidHigressConfig(t *testing.T) {
	tests := []struct {
		name          string
		higressConfig *HigressConfig
		wantErr       error
	}{
		{
			name:          "nil config",
			higressConfig: nil,
			wantErr:       nil,
		},
		{
			name: "nil mcp server",
			higressConfig: &HigressConfig{
				McpServer: nil,
			},
			wantErr: nil,
		},
		{
			name: "valid config",
			higressConfig: &HigressConfig{
				McpServer: &McpServer{
					Enable: true,
					Redis: &RedisConfig{
						Address: "localhost:6379",
					},
					MatchList: []*MatchRule{},
					Servers:   []*SSEServer{},
				},
			},
			wantErr: nil,
		},
		{
			name: "invalid config - user level server without redis",
			higressConfig: &HigressConfig{
				McpServer: &McpServer{
					Enable:                true,
					EnableUserLevelServer: true,
					Redis:                 nil,
					MatchList:             []*MatchRule{},
					Servers:               []*SSEServer{},
				},
			},
			wantErr: errors.New("redis config cannot be empty when user level server is enabled"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMcpServerController("test-namespace")
			err := m.ValidHigressConfig(tt.higressConfig)
			assert.Equal(t, tt.wantErr, err)
		})
	}
}

func TestMcpServerController_ConstructEnvoyFilters(t *testing.T) {
	tests := []struct {
		name        string
		mcpServer   *McpServer
		wantConfigs int
		wantErr     error
	}{
		{
			name:        "nil mcp server",
			mcpServer:   nil,
			wantConfigs: 0,
			wantErr:     nil,
		},
		{
			name: "disabled mcp server",
			mcpServer: &McpServer{
				Enable: false,
			},
			wantConfigs: 0,
			wantErr:     nil,
		},
		{
			name: "valid mcp server with redis",
			mcpServer: &McpServer{
				Enable: true,
				Redis: &RedisConfig{
					Address: "localhost:6379",
				},
				MatchList: []*MatchRule{},
				Servers:   []*SSEServer{},
			},
			wantConfigs: 1, // Only session filter when no servers configured
			wantErr:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMcpServerController("test-namespace")
			m.mcpServer.Store(tt.mcpServer)
			configs, err := m.ConstructEnvoyFilters()
			assert.Equal(t, tt.wantErr, err)
			assert.Equal(t, tt.wantConfigs, len(configs))
		})
	}
}

func TestMcpServerController_constructMcpSessionStruct(t *testing.T) {
	tests := []struct {
		name     string
		mcp      *McpServer
		wantJSON string
	}{
		{
			name: "minimal config",
			mcp: &McpServer{
				Enable: true,
				Redis: &RedisConfig{
					Address: "localhost:6379",
				},
				MatchList: []*MatchRule{},
				Servers:   []*SSEServer{},
			},
			wantJSON: `{
				"@type": "type.googleapis.com/envoy.extensions.filters.http.golang.v3alpha.Config",
				"library_id": "mcp-session",
				"library_path": "/var/lib/istio/envoy/golang-filter.so",
				"plugin_name": "mcp-session",
				"plugin_config": {
					"@type": "type.googleapis.com/xds.type.v3.TypedStruct",
					"value": {
						"redis": {
							"address": "localhost:6379",
							"username": "",
							"password": "",
							"db": 0
						},
						"rate_limit": null,
						"sse_path_suffix": "",
						"match_list": [],
						"enable_user_level_server": false
					}
				}
			}`,
		},
		{
			name: "full config",
			mcp: &McpServer{
				Enable: true,
				Redis: &RedisConfig{
					Address:  "localhost:6379",
					Username: "user",
					Password: "pass",
					DB:       1,
				},
				SSEPathSuffix: "/sse",
				MatchList: []*MatchRule{
					{
						MatchRuleDomain: "*",
						MatchRulePath:   "/test",
						MatchRuleType:   "exact",
					},
					{
						MatchRuleDomain: "*",
						MatchRulePath:   "/sse-test-1",
						MatchRuleType:   "prefix",
						UpstreamType:    "sse",
					},
					{
						MatchRuleDomain:   "*",
						MatchRulePath:     "/sse-test-2",
						MatchRuleType:     "prefix",
						UpstreamType:      "sse",
						EnablePathRewrite: true,
						PathRewritePrefix: "/mcp",
					},
				},
				EnableUserLevelServer: true,
				Ratelimit: &MCPRatelimitConfig{
					Limit:     100,
					Window:    3600,
					WhiteList: []string{"user1", "user2"},
				},
			},
			wantJSON: `{
				"@type": "type.googleapis.com/envoy.extensions.filters.http.golang.v3alpha.Config",
				"library_id": "mcp-session",
				"library_path": "/var/lib/istio/envoy/golang-filter.so",
				"plugin_name": "mcp-session",
				"plugin_config": {
					"@type": "type.googleapis.com/xds.type.v3.TypedStruct",
					"value": {
						"redis": {
							"address": "localhost:6379",
							"username": "user",
							"password": "pass",
							"db": 1
						},
						"rate_limit": {
							"limit": 100,
							"window": 3600,
							"white_list": ["user1","user2"]
						},
						"sse_path_suffix": "/sse",
						"match_list": [{
							"match_rule_domain": "*",
							"match_rule_path": "/test",
							"match_rule_type": "exact",
							"upstream_type": "",
							"enable_path_rewrite": false,
							"path_rewrite_prefix": ""
						},{
							"match_rule_domain": "*",
							"match_rule_path": "/sse-test-1",
							"match_rule_type": "prefix",
							"upstream_type": "sse",
							"enable_path_rewrite": false,
							"path_rewrite_prefix": ""
						},{
							"match_rule_domain": "*",
							"match_rule_path": "/sse-test-2",
							"match_rule_type": "prefix",
							"upstream_type": "sse",
							"enable_path_rewrite": true,
							"path_rewrite_prefix": "/mcp"
						}],
						"enable_user_level_server": true
					}
				}
			}`,
		},
		{
			name: "config with password secret",
			mcp: &McpServer{
				Enable: true,
				Redis: &RedisConfig{
					Address:  "localhost:6379",
					Password: "ignored",
					PasswordSecret: &SecretKeyReference{
						Name: "redis-credentials",
						Key:  "password",
					},
				},
				MatchList: []*MatchRule{},
				Servers:   []*SSEServer{},
			},
			wantJSON: `{
				"@type": "type.googleapis.com/envoy.extensions.filters.http.golang.v3alpha.Config",
				"library_id": "mcp-session",
				"library_path": "/var/lib/istio/envoy/golang-filter.so",
				"plugin_name": "mcp-session",
				"plugin_config": {
					"@type": "type.googleapis.com/xds.type.v3.TypedStruct",
					"value": {
						"redis": {
							"address": "localhost:6379",
							"username": "",
							"password": "${secret.test-namespace/redis-credentials.password}",
							"db": 0
						},
						"rate_limit": null,
						"sse_path_suffix": "",
						"match_list": [],
						"enable_user_level_server": false
					}
				}
			}`,
		},
		{
			name: "config with password secret and namespace",
			mcp: &McpServer{
				Enable: true,
				Redis: &RedisConfig{
					Address: "localhost:6379",
					PasswordSecret: &SecretKeyReference{
						Namespace: "other-ns",
						Name:      "redis-credentials",
						Key:       "password",
					},
				},
				MatchList: []*MatchRule{},
				Servers:   []*SSEServer{},
			},
			wantJSON: `{
				"@type": "type.googleapis.com/envoy.extensions.filters.http.golang.v3alpha.Config",
				"library_id": "mcp-session",
				"library_path": "/var/lib/istio/envoy/golang-filter.so",
				"plugin_name": "mcp-session",
				"plugin_config": {
					"@type": "type.googleapis.com/xds.type.v3.TypedStruct",
					"value": {
						"redis": {
							"address": "localhost:6379",
							"username": "",
							"password": "${secret.other-ns/redis-credentials.password}",
							"db": 0
						},
						"rate_limit": null,
						"sse_path_suffix": "",
						"match_list": [],
						"enable_user_level_server": false
					}
				}
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMcpServerController("test-namespace")
			got := m.constructMcpSessionStruct(tt.mcp)
			// Normalize JSON strings for comparison
			var gotJSON, wantJSON interface{}
			json.Unmarshal([]byte(got), &gotJSON)
			json.Unmarshal([]byte(tt.wantJSON), &wantJSON)
			assert.Equal(t, wantJSON, gotJSON)
		})
	}
}

type staticMcpServerProvider struct {
	servers []*mcpserver.McpServer
}

func (p *staticMcpServerProvider) GetMcpServers() []*mcpserver.McpServer {
	return p.servers
}

func TestMcpServerController_constructMcpSessionStructProviderDomains(t *testing.T) {
	tests := []struct {
		name        string
		domains     []string
		wantDomains []string
	}{
		{
			name:        "empty domains preserve existing rule",
			wantDomains: []string{""},
		},
		{
			name:        "single domain",
			domains:     []string{"orders.example.com"},
			wantDomains: []string{"orders.example.com"},
		},
		{
			name:        "multiple domains",
			domains:     []string{"orders.example.com", "orders-internal.example.com"},
			wantDomains: []string{"orders.example.com", "orders-internal.example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manualRule := &MatchRule{
				MatchRuleDomain: "manual.example.com",
				MatchRulePath:   "/manual",
				MatchRuleType:   "exact",
			}
			mcp := NewDefaultMcpServer()
			mcp.MatchList = []*MatchRule{manualRule}

			controller := NewMcpServerController("test-namespace")
			controller.RegisterMcpServerProvider(&staticMcpServerProvider{
				servers: []*mcpserver.McpServer{
					{
						Name:              "orders",
						Domains:           tt.domains,
						PathMatchType:     "prefix",
						PathMatchValue:    "/mcp",
						UpstreamType:      "sse",
						EnablePathRewrite: true,
						PathRewritePrefix: "/upstream/mcp",
					},
				},
			})

			var config struct {
				PluginConfig struct {
					Value struct {
						MatchList []*MatchRule `json:"match_list"`
					} `json:"value"`
				} `json:"plugin_config"`
			}
			require.NoError(t, json.Unmarshal([]byte(controller.constructMcpSessionStruct(mcp)), &config))

			wantRules := []*MatchRule{manualRule}
			for _, domain := range tt.wantDomains {
				wantRules = append(wantRules, &MatchRule{
					MatchRuleDomain:   domain,
					MatchRulePath:     "/mcp",
					MatchRuleType:     "prefix",
					UpstreamType:      "sse",
					EnablePathRewrite: true,
					PathRewritePrefix: "/upstream/mcp",
				})
			}
			assert.Equal(t, wantRules, config.PluginConfig.Value.MatchList)
		})
	}
}

func TestMcpServerController_constructMcpServerStruct(t *testing.T) {
	tests := []struct {
		name     string
		mcp      *McpServer
		wantJSON string
	}{
		{
			name: "no servers",
			mcp: &McpServer{
				Servers: []*SSEServer{},
			},
			wantJSON: "", // Return empty string when no servers configured
		},
		{
			name: "with servers",
			mcp: &McpServer{
				Servers: []*SSEServer{
					{
						Name: "test-server",
						Path: "/test",
						Type: "test",
						Config: map[string]interface{}{
							"key": "value",
						},
						DomainList: []string{"example.com"},
					},
				},
			},
			wantJSON: `{
				"@type": "type.googleapis.com/envoy.extensions.filters.http.golang.v3alpha.Config",
				"library_id": "mcp-server",
				"library_path": "/var/lib/istio/envoy/golang-filter.so",
				"plugin_name": "mcp-server",
				"plugin_config": {
					"@type": "type.googleapis.com/xds.type.v3.TypedStruct",
					"value": {
						"servers": [{
							"name": "test-server",
							"path": "/test",
							"type": "test",
							"domain_list": ["example.com"],
							"config": {"key":"value"}
						}]
					}
				}
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMcpServerController("test-namespace")
			got := m.constructMcpServerStruct(tt.mcp)
			// Normalize JSON strings for comparison
			var gotJSON, wantJSON interface{}
			json.Unmarshal([]byte(got), &gotJSON)
			json.Unmarshal([]byte(tt.wantJSON), &wantJSON)
			assert.Equal(t, wantJSON, gotJSON)
		})
	}
}
