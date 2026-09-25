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

package config

import (
	"encoding/json"
	"testing"

	networking "istio.io/api/networking/v1alpha3"
	"istio.io/istio/pkg/config"

	higresscommon "github.com/alibaba/higress/v2/pkg/common"
	"github.com/alibaba/higress/v2/pkg/ingress/kube/common"
	"github.com/alibaba/higress/v2/pkg/ingress/kube/configmap"
)

// makeTestProxyWrapper returns a ProxyWrapper for tests.
func makeTestProxyWrapper(proxyName string, listenerPort uint32, connectTimeout uint32) *common.ProxyWrapper {
	return &common.ProxyWrapper{
		ProxyName:      proxyName,
		ListenerPort:   listenerPort,
		ConnectTimeout: connectTimeout,
		EnvoyFilter:    &networking.EnvoyFilter{},
	}
}

// makeTestServiceWrapper returns a ServiceWrapper with a ProxyConfig for tests.
func makeTestServiceWrapper(host string, port uint32, proxyName string, upstreamProtocol higresscommon.Protocol, sni string) *common.ServiceWrapper {
	return &common.ServiceWrapper{
		ServiceName: host,
		ServiceEntry: &networking.ServiceEntry{
			Hosts: []string{host},
			Ports: []*networking.ServicePort{
				{
					Number:   port,
					Protocol: string(upstreamProtocol),
					Name:     "https",
				},
			},
			Resolution: networking.ServiceEntry_STATIC,
		},
		RegistryType: "dns",
		ProxyConfig: &common.ServiceProxyConfig{
			ProxyName:        proxyName,
			UpstreamProtocol: upstreamProtocol,
			UpstreamSni:      sni,
		},
	}
}

// extractAddPatch finds the ADD cluster patch value from the service-proxy EnvoyFilter
// and returns its fields as a map[string]interface{} via JSON round-trip.
func extractAddPatchFields(configs []*config.Config, clusterName string) map[string]interface{} {
	for _, cfg := range configs {
		ef, ok := cfg.Spec.(*networking.EnvoyFilter)
		if !ok {
			continue
		}
		for _, patch := range ef.ConfigPatches {
			if patch.Patch == nil || patch.Patch.Operation != networking.EnvoyFilter_Patch_ADD {
				continue
			}
			if patch.Patch.Value == nil {
				continue
			}
			// Marshal the structpb to JSON and unmarshal to map.
			raw, err := json.Marshal(patch.Patch.Value.AsMap())
			if err != nil {
				continue
			}
			var fields map[string]interface{}
			if err := json.Unmarshal(raw, &fields); err != nil {
				continue
			}
			// Check the name matches the cluster we want.
			if name, ok := fields["name"].(string); ok && name == clusterName {
				return fields
			}
		}
	}
	return nil
}

func TestConstructProxyEnvoyFilters(t *testing.T) {
	const (
		proxyName    = "p1"
		host         = "svc.dns"
		port         = uint32(443)
		listenerPort = uint32(50001)
		namespace    = "higress-system"
	)
	clusterName := "outbound|443||svc.dns"

	tests := []struct {
		name               string
		upstream           *configmap.Upstream
		connectTimeout     uint32
		wantIdleTimeout    string
		wantBufferLimit    float64
		wantHasIdleTimeout bool
		wantHasBufferLimit bool
		wantConnectTimeout string
	}{
		{
			name: "with upstream",
			upstream: &configmap.Upstream{
				IdleTimeout:            30,
				ConnectionBufferLimits: 2048,
			},
			connectTimeout:     0,
			wantIdleTimeout:    "30s",
			wantBufferLimit:    2048,
			wantHasIdleTimeout: true,
			wantHasBufferLimit: true,
			wantConnectTimeout: "1200ms",
		},
		{
			name:               "without upstream nil",
			upstream:           nil,
			connectTimeout:     0,
			wantHasIdleTimeout: false,
			wantHasBufferLimit: false,
			wantConnectTimeout: "1200ms",
		},
		{
			name: "ConnectTimeout plumbing",
			upstream: &configmap.Upstream{
				IdleTimeout:            10,
				ConnectionBufferLimits: 0,
			},
			connectTimeout:     2500,
			wantIdleTimeout:    "10s",
			wantHasIdleTimeout: true,
			wantHasBufferLimit: false,
			wantConnectTimeout: "2500ms",
		},
		{
			// idleTimeout==0 mirrors global-option's always-render: it must inject
			// "0s" (envoy disable), NOT be skipped, so proxy clusters stay consistent
			// with non-proxy clusters that get "0s" via the global-option MERGE.
			// ConnectionBufferLimits>0 still injects per_connection_buffer_limit_bytes.
			name: "idle zero renders 0s with buffer nonzero",
			upstream: &configmap.Upstream{
				IdleTimeout:            0,
				ConnectionBufferLimits: 4096,
			},
			connectTimeout:     0,
			wantIdleTimeout:    "0s",
			wantHasIdleTimeout: true,
			wantHasBufferLimit: true,
			wantBufferLimit:    4096,
			wantConnectTimeout: "1200ms",
		},
		{
			// Both fields zero/unset: idle still injects "0s" (always-render),
			// buffer limit stays guarded (>0) so it is absent.
			name: "idle zero buffer zero",
			upstream: &configmap.Upstream{
				IdleTimeout:            0,
				ConnectionBufferLimits: 0,
			},
			connectTimeout:     0,
			wantIdleTimeout:    "0s",
			wantHasIdleTimeout: true,
			wantHasBufferLimit: false,
			wantConnectTimeout: "1200ms",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			proxyWrappers := map[string]*common.ProxyWrapper{
				proxyName: makeTestProxyWrapper(proxyName, listenerPort, tc.connectTimeout),
			}
			serviceWrappers := map[string]*common.ServiceWrapper{
				host: makeTestServiceWrapper(host, port, proxyName, higresscommon.HTTPS, "sni"),
			}

			result := constructProxyEnvoyFilters(proxyWrappers, serviceWrappers, namespace, tc.upstream)

			fields := extractAddPatchFields(result, clusterName)
			if fields == nil {
				t.Fatalf("ADD patch for cluster %q not found in result", clusterName)
			}

			// Assert connect_timeout.
			if ct, ok := fields["connect_timeout"].(string); !ok || ct != tc.wantConnectTimeout {
				t.Errorf("connect_timeout: got %v, want %s", fields["connect_timeout"], tc.wantConnectTimeout)
			}

			// Assert endpoint address and port.
			loadAssignment, ok := fields["load_assignment"].(map[string]interface{})
			if !ok {
				t.Fatal("load_assignment not found or wrong type")
			}
			endpoints, ok := loadAssignment["endpoints"].([]interface{})
			if !ok || len(endpoints) == 0 {
				t.Fatal("endpoints not found")
			}
			lbEndpoints, ok := endpoints[0].(map[string]interface{})["lb_endpoints"].([]interface{})
			if !ok || len(lbEndpoints) == 0 {
				t.Fatal("lb_endpoints not found")
			}
			endpoint := lbEndpoints[0].(map[string]interface{})["endpoint"].(map[string]interface{})
			address := endpoint["address"].(map[string]interface{})
			socketAddress := address["socket_address"].(map[string]interface{})
			if addr, ok := socketAddress["address"].(string); !ok || addr != "127.0.0.1" {
				t.Errorf("endpoint address: got %v, want 127.0.0.1", socketAddress["address"])
			}
			portVal, ok := socketAddress["port_value"].(float64)
			if !ok || uint32(portVal) != listenerPort {
				t.Errorf("endpoint port: got %v, want %d", socketAddress["port_value"], listenerPort)
			}

			// Assert common_http_protocol_options / idle_timeout.
			httpOpts, hasHttpOpts := fields["common_http_protocol_options"]
			if tc.wantHasIdleTimeout {
				if !hasHttpOpts {
					t.Errorf("expected common_http_protocol_options, not found")
				} else {
					httpOptsMap, ok := httpOpts.(map[string]interface{})
					if !ok {
						t.Errorf("common_http_protocol_options wrong type: %T", httpOpts)
					} else if idleTimeout, ok := httpOptsMap["idleTimeout"].(string); !ok || idleTimeout != tc.wantIdleTimeout {
						t.Errorf("idleTimeout: got %v, want %s", httpOptsMap["idleTimeout"], tc.wantIdleTimeout)
					}
				}
			} else {
				if hasHttpOpts {
					t.Errorf("unexpected common_http_protocol_options present")
				}
			}

			// Assert per_connection_buffer_limit_bytes.
			bufLimit, hasBufferLimit := fields["per_connection_buffer_limit_bytes"]
			if tc.wantHasBufferLimit {
				if !hasBufferLimit {
					t.Errorf("expected per_connection_buffer_limit_bytes, not found")
				} else {
					if bv, ok := bufLimit.(float64); !ok || bv != tc.wantBufferLimit {
						t.Errorf("per_connection_buffer_limit_bytes: got %v, want %v", bufLimit, tc.wantBufferLimit)
					}
				}
			} else {
				if hasBufferLimit {
					t.Errorf("unexpected per_connection_buffer_limit_bytes present")
				}
			}
		})
	}
}
