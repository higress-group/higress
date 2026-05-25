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

package mcpserver

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	apiv1 "github.com/alibaba/higress/v2/api/networking/v1"
	common2 "github.com/alibaba/higress/v2/pkg/ingress/kube/common"
	provider "github.com/alibaba/higress/v2/registry"
	"github.com/alibaba/higress/v2/registry/memory"
	"github.com/nacos-group/nacos-sdk-go/v2/model"
	"github.com/stretchr/testify/mock"
	wrappers "google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/api/networking/v1alpha3"
	"istio.io/istio/pkg/config"
	"istio.io/istio/pkg/config/constants"
	"istio.io/istio/pkg/config/schema/gvk"
)

type mockWatcher struct {
	watcher
	mock.Mock
}

func newTestWatcher(cache memory.Cache, opts ...WatcherOption) mockWatcher {
	w := &watcher{
		watchingConfig: make(map[string]bool),
		RegistryType:   "mcpserver",
		Status:         provider.UnHealthy,
		cache:          cache,
		mutex:          &sync.Mutex{},
		stop:           make(chan struct{}),
	}

	w.NacosRefreshInterval = int64(DefaultRefreshInterval)

	for _, opt := range opts {
		opt(w)
	}

	if w.NacosNamespace == "" {
		w.NacosNamespace = w.NacosNamespaceId
	}

	return mockWatcher{watcher: *w, Mock: mock.Mock{}}
}

func testCallback(msc *McpServerConfig) memory.Cache {
	registryConfig := &apiv1.RegistryConfig{
		Type:                   string(provider.Nacos),
		Name:                   "mse-nacos-public",
		Domain:                 "",
		Port:                   8848,
		NacosAddressServer:     "",
		NacosAccessKey:         "ak",
		NacosSecretKey:         "sk",
		NacosNamespaceId:       "",
		NacosNamespace:         "public",
		NacosGroups:            []string{"dev"},
		NacosRefreshInterval:   0,
		EnableMCPServer:        wrappers.Bool(true),
		McpServerExportDomains: []string{"mcp.com"},
		McpServerBaseUrl:       "/mcp-servers/",
		EnableScopeMcpServers:  wrappers.Bool(true),
		AllowMcpServers:        []string{"mcp-server-1", "mcp-server-2"},
		Metadata: map[string]*apiv1.InnerMap{
			"routeName": {
				InnerMap: map[string]string{"mcp-server-1": "mcp-route-1", "mcp-server-2": "mcp-route-2"},
			},
		},
	}
	localCache := memory.NewCache()

	testWatcher := newTestWatcher(localCache,
		WithType(registryConfig.Type),
		WithName(registryConfig.Name),
		WithNacosAddressServer(registryConfig.NacosAddressServer),
		WithDomain(registryConfig.Domain),
		WithPort(registryConfig.Port),
		WithNacosNamespaceId(registryConfig.NacosNamespaceId),
		WithNacosNamespace(registryConfig.NacosNamespace),
		WithNacosGroups(registryConfig.NacosGroups),
		WithNacosAccessKey(registryConfig.NacosAccessKey),
		WithNacosSecretKey(registryConfig.NacosSecretKey),
		WithNacosRefreshInterval(registryConfig.NacosRefreshInterval),
		WithMcpExportDomains(registryConfig.McpServerExportDomains),
		WithMcpBaseUrl(registryConfig.McpServerBaseUrl),
		WithEnableMcpServer(registryConfig.EnableMCPServer))
	testWatcher.AppendServiceUpdateHandler(func() {
		fmt.Println("testWatcher service update success")
	})

	callback := testWatcher.mcpServerListener("mock-data-id")
	callback(msc)
	return localCache
}

func Test_Watcher(t *testing.T) {
	dataId := "mock-data-id"

	testCase := []struct {
		name       string
		msc        *McpServerConfig
		dataId     string
		wantConfig map[string]*config.Config
	}{
		{
			name:   "normal case",
			dataId: dataId,
			msc: &McpServerConfig{
				Credentials: map[string]interface{}{
					"test-server": map[string]string{"data": "value"},
				},
				ServiceInfo: &model.Service{
					Hosts: []model.Instance{
						{
							Ip:       "127.0.0.1",
							Port:     8080,
							Metadata: map[string]string{"protocol": "http"},
							Enable:   true,
							Healthy:  true,
						},
					},
				},
				ServerSpecConfig: `{
					"name": "explore",
					"protocol": "http",
					"description": "explore",
					"remoteServerConfig": {
						"serviceRef": {
							"namespaceId": "public",
							"groupName": "DEFAULT_GROUP",
							"serviceName": "explore"
						},
						"exportPath": ""
					},
					"enabled": true
				}`,
				ToolsSpecConfig: `{
					"tools": [
						{
							"name": "explore",
							"description": "find name from tag",
							"inputSchema": {
								"type": "object",
								"properties": {
									"tags": {
										"type": "string",
										"description": "tag"
									}
								}
							}
						}
					],
					"toolsMeta": {
						"explore": {
							"enabled": true,
							"templates": {
								"json-go-template": {
									"requestTemplate": {
										"method": "GET",
										"url": "/v0/explore",
										"argsToUrlParam": true
									}
								}
							}
						}
					}
				}`,
			},
			wantConfig: map[string]*config.Config{
				gvk.ServiceEntry.String(): {
					Meta: config.Meta{
						GroupVersionKind: gvk.ServiceEntry,
						Name:             fmt.Sprintf("%s-%s", provider.IstioMcpAutoGeneratedSeName, strings.TrimSuffix(dataId, ".json")),
						Namespace:        "mcp",
					},
					Spec: &v1alpha3.ServiceEntry{
						Hosts: []string{"explore.DEFAULT-GROUP.public.nacos"},
						Ports: []*v1alpha3.ServicePort{
							{
								Number:   8080,
								Name:     "HTTP",
								Protocol: "HTTP",
							},
						},
						Location:   v1alpha3.ServiceEntry_MESH_INTERNAL,
						Resolution: v1alpha3.ServiceEntry_STATIC,
						Endpoints: []*v1alpha3.WorkloadEntry{
							{
								Address: "127.0.0.1",
								Ports: map[string]uint32{
									"HTTP": 8080,
								},
								Labels: map[string]string{
									"protocol": "http",
								},
							},
						},
					},
				},
				gvk.VirtualService.String(): {
					Meta: config.Meta{
						GroupVersionKind: gvk.VirtualService,
						Name:             fmt.Sprintf("%s-%s", provider.IstioMcpAutoGeneratedVsName, strings.TrimSuffix(dataId, ".json")),
					},
					Spec: &v1alpha3.VirtualService{
						Gateways: []string{"/" + common2.CleanHost("mcp.com"), common2.CreateConvertedName(constants.IstioIngressGatewayName, common2.CleanHost("mcp.com"))},
						Hosts:    []string{"mcp.com"},
						Http: []*v1alpha3.HTTPRoute{
							{
								Name: fmt.Sprintf("%s-%s", provider.IstioMcpAutoGeneratedHttpRouteName, strings.TrimSuffix(dataId, ".json")),
								Match: []*v1alpha3.HTTPMatchRequest{
									{
										Uri: &v1alpha3.StringMatch{
											MatchType: &v1alpha3.StringMatch_Exact{
												Exact: "/mcp-servers/explore",
											},
										},
									},
									{
										Uri: &v1alpha3.StringMatch{
											MatchType: &v1alpha3.StringMatch_Prefix{
												Prefix: "/mcp-servers/explore/",
											},
										},
									},
								},
								Route: []*v1alpha3.HTTPRouteDestination{
									{
										Destination: &v1alpha3.Destination{
											Host: "explore.DEFAULT-GROUP.public.nacos",
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name:   "sse and dns endpoint case",
			dataId: dataId,
			msc: &McpServerConfig{
				Credentials: map[string]interface{}{
					"test-server": map[string]string{"data": "value"},
				},
				ServiceInfo: &model.Service{
					Hosts: []model.Instance{
						{
							Ip:       "example.com",
							Port:     8080,
							Metadata: map[string]string{"protocol": "http"},
							Enable:   true,
							Healthy:  true,
						},
					},
				},
				ServerSpecConfig: `{
					"name": "explore",
					"protocol": "mcp-sse",
					"description": "explore",
					"remoteServerConfig": {
						"serviceRef": {
							"namespaceId": "public",
							"groupName": "DEFAULT_GROUP",
							"serviceName": "explore"
						},
						"exportPath": ""
					},
					"enabled": true
				}`,
				ToolsSpecConfig: `{
					"tools": [
						{
							"name": "explore",
							"description": "find name from tag",
							"inputSchema": {
								"type": "object",
								"properties": {
									"tags": {
										"type": "string",
										"description": "tag"
									}
								}
							}
						}
					],
					"toolsMeta": {
						"explore": {
							"enabled": true,
							"templates": {
								"json-go-template": {
									"requestTemplate": {
										"method": "GET",
										"url": "/v0/explore",
										"argsToUrlParam": true
									}
								}
							}
						}
					}
				}`,
			},
			wantConfig: map[string]*config.Config{
				gvk.ServiceEntry.String(): {
					Meta: config.Meta{
						GroupVersionKind: gvk.ServiceEntry,
						Name:             fmt.Sprintf("%s-%s", provider.IstioMcpAutoGeneratedSeName, strings.TrimSuffix(dataId, ".json")),
						Namespace:        "mcp",
					},
					Spec: &v1alpha3.ServiceEntry{
						Hosts: []string{"explore.DEFAULT-GROUP.public.nacos"},
						Ports: []*v1alpha3.ServicePort{
							{
								Number:   8080,
								Name:     "HTTP",
								Protocol: "HTTP",
							},
						},
						Location:   v1alpha3.ServiceEntry_MESH_INTERNAL,
						Resolution: v1alpha3.ServiceEntry_DNS,
						Endpoints: []*v1alpha3.WorkloadEntry{
							{
								Address: "example.com",
								Ports: map[string]uint32{
									"HTTP": 8080,
								},
								Labels: map[string]string{
									"protocol": "http",
								},
							},
						},
					},
				},
				gvk.VirtualService.String(): {
					Meta: config.Meta{
						GroupVersionKind: gvk.VirtualService,
						Name:             fmt.Sprintf("%s-%s", provider.IstioMcpAutoGeneratedVsName, strings.TrimSuffix(dataId, ".json")),
					},
					Spec: &v1alpha3.VirtualService{
						Gateways: []string{"/" + common2.CleanHost("mcp.com"), common2.CreateConvertedName(constants.IstioIngressGatewayName, common2.CleanHost("mcp.com"))},
						Hosts:    []string{"mcp.com"},
						Http: []*v1alpha3.HTTPRoute{
							{
								Name: fmt.Sprintf("%s-%s", provider.IstioMcpAutoGeneratedHttpRouteName, strings.TrimSuffix(dataId, ".json")),
								Match: []*v1alpha3.HTTPMatchRequest{
									{
										Uri: &v1alpha3.StringMatch{
											MatchType: &v1alpha3.StringMatch_Exact{
												Exact: "/mcp-servers/explore",
											},
										},
									},
									{
										Uri: &v1alpha3.StringMatch{
											MatchType: &v1alpha3.StringMatch_Prefix{
												Prefix: "/mcp-servers/explore/",
											},
										},
									},
								},
								Route: []*v1alpha3.HTTPRouteDestination{
									{
										Destination: &v1alpha3.Destination{
											Host: "explore.DEFAULT-GROUP.public.nacos",
										},
									},
								},
								Rewrite: &v1alpha3.HTTPRewrite{
									Uri:       "/",
									Authority: "example.com",
								},
							},
						},
					},
				},
				gvk.DestinationRule.String(): {
					Meta: config.Meta{
						GroupVersionKind: gvk.DestinationRule,
						Name:             fmt.Sprintf("%s-%s", provider.IstioMcpAutoGeneratedDrName, strings.TrimSuffix(dataId, ".json")),
					},
					Spec: &v1alpha3.DestinationRule{
						Host: "explore.DEFAULT-GROUP.public.nacos",
						TrafficPolicy: &v1alpha3.TrafficPolicy{
							LoadBalancer: &v1alpha3.LoadBalancerSettings{
								LbPolicy: &v1alpha3.LoadBalancerSettings_ConsistentHash{
									ConsistentHash: &v1alpha3.LoadBalancerSettings_ConsistentHashLB{
										HashKey: &v1alpha3.LoadBalancerSettings_ConsistentHashLB_UseSourceIp{
											UseSourceIp: true,
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name:   "https and dns case",
			dataId: dataId,
			msc: &McpServerConfig{
				Credentials: map[string]interface{}{
					"test-server": map[string]string{"data": "value"},
				},
				ServiceInfo: &model.Service{
					Hosts: []model.Instance{
						{
							Ip:       "example.com",
							Port:     8080,
							Metadata: map[string]string{"protocol": "https"},
							Enable:   true,
							Healthy:  true,
						},
					},
				},
				ServerSpecConfig: `{
					"name": "explore",
					"protocol": "https",
					"description": "explore",
					"remoteServerConfig": {
						"serviceRef": {
							"namespaceId": "public",
							"groupName": "DEFAULT_GROUP",
							"serviceName": "explore"
						},
						"exportPath": ""
					},
					"enabled": true
				}`,
				ToolsSpecConfig: `{
					"tools": [
						{
							"name": "explore",
							"description": "find name from tag",
							"inputSchema": {
								"type": "object",
								"properties": {
									"tags": {
										"type": "string",
										"description": "tag"
									}
								}
							}
						}
					],
					"toolsMeta": {
						"explore": {
							"enabled": true,
							"templates": {
								"json-go-template": {
									"requestTemplate": {
										"method": "GET",
										"url": "/v0/explore",
										"argsToUrlParam": true
									}
								}
							}
						}
					}
				}`,
			},
			wantConfig: map[string]*config.Config{
				gvk.ServiceEntry.String(): {
					Meta: config.Meta{
						GroupVersionKind: gvk.ServiceEntry,
						Name:             fmt.Sprintf("%s-%s", provider.IstioMcpAutoGeneratedSeName, strings.TrimSuffix(dataId, ".json")),
						Namespace:        "mcp",
					},
					Spec: &v1alpha3.ServiceEntry{
						Hosts: []string{"explore.DEFAULT-GROUP.public.nacos"},
						Ports: []*v1alpha3.ServicePort{
							{
								Number:   8080,
								Name:     "HTTPS",
								Protocol: "HTTPS",
							},
						},
						Location:   v1alpha3.ServiceEntry_MESH_INTERNAL,
						Resolution: v1alpha3.ServiceEntry_DNS,
						Endpoints: []*v1alpha3.WorkloadEntry{
							{
								Address: "example.com",
								Ports: map[string]uint32{
									"HTTPS": 8080,
								},
								Labels: map[string]string{
									"protocol": "https",
								},
							},
						},
					},
				},
				gvk.VirtualService.String(): {
					Meta: config.Meta{
						GroupVersionKind: gvk.VirtualService,
						Name:             fmt.Sprintf("%s-%s", provider.IstioMcpAutoGeneratedVsName, strings.TrimSuffix(dataId, ".json")),
					},
					Spec: &v1alpha3.VirtualService{
						Gateways: []string{"/" + common2.CleanHost("mcp.com"), common2.CreateConvertedName(constants.IstioIngressGatewayName, common2.CleanHost("mcp.com"))},
						Hosts:    []string{"mcp.com"},
						Http: []*v1alpha3.HTTPRoute{
							{
								Name: fmt.Sprintf("%s-%s", provider.IstioMcpAutoGeneratedHttpRouteName, strings.TrimSuffix(dataId, ".json")),
								Match: []*v1alpha3.HTTPMatchRequest{
									{
										Uri: &v1alpha3.StringMatch{
											MatchType: &v1alpha3.StringMatch_Exact{
												Exact: "/mcp-servers/explore",
											},
										},
									},
									{
										Uri: &v1alpha3.StringMatch{
											MatchType: &v1alpha3.StringMatch_Prefix{
												Prefix: "/mcp-servers/explore/",
											},
										},
									},
								},
								Route: []*v1alpha3.HTTPRouteDestination{
									{
										Destination: &v1alpha3.Destination{
											Host: "explore.DEFAULT-GROUP.public.nacos",
										},
									},
								},
								Rewrite: &v1alpha3.HTTPRewrite{
									Authority: "example.com",
								},
							},
						},
					},
				},
				gvk.DestinationRule.String(): {
					Meta: config.Meta{
						GroupVersionKind: gvk.DestinationRule,
						Name:             fmt.Sprintf("%s-%s", provider.IstioMcpAutoGeneratedDrName, strings.TrimSuffix(dataId, ".json")),
					},
					Spec: &v1alpha3.DestinationRule{
						Host: "explore.DEFAULT-GROUP.public.nacos",
						TrafficPolicy: &v1alpha3.TrafficPolicy{
							Tls: &v1alpha3.ClientTLSSettings{
								Mode: v1alpha3.ClientTLSSettings_SIMPLE,
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			localCache := testCallback(tc.msc)
			se := localCache.GetAllConfigs(gvk.ServiceEntry)[dataId]
			wantSe := tc.wantConfig[gvk.ServiceEntry.String()]
			if !reflect.DeepEqual(se, wantSe) {
				t.Errorf("se is not equal, want %v\n, got %v", wantSe, se)
			}

			vs := localCache.GetAllConfigs(gvk.VirtualService)[dataId]
			wantVs := tc.wantConfig[gvk.VirtualService.String()]
			if !reflect.DeepEqual(vs, wantVs) {
				t.Errorf("vs is not equal, want %v\n, got %v", wantVs, vs)
			}

			dr := localCache.GetAllConfigs(gvk.DestinationRule)[dataId]
			wantDr := tc.wantConfig[gvk.DestinationRule.String()]
			if !reflect.DeepEqual(dr, wantDr) {
				t.Errorf("dr is not equal, want %v\n, got %v", wantDr, dr)
			}
		})
	}
}
