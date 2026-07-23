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
	"testing"

	"github.com/alibaba/higress/v2/pkg/ingress/kube/common"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"
	extensions "istio.io/api/extensions/v1alpha1"
	"istio.io/api/networking/v1alpha3"
)

func TestClusterHeaderForPluginConfig(t *testing.T) {
	route := wrapperRoute("route-a", "api.example.com", "llm-a.internal.dns", 80)
	pluginConfig := mustStruct(t, map[string]interface{}{
		"lb_type":   "endpoint",
		"lb_policy": "endpoint_metrics",
		"_rules_": []interface{}{
			map[string]interface{}{
				"_match_route_": []interface{}{"route-a"},
				"lb_type":       "cluster",
				"lb_policy":     "cluster_hash",
				"lb_config": map[string]interface{}{
					"cluster_header": "x-custom-target-cluster",
				},
			},
		},
	})

	require.Equal(t, "x-custom-target-cluster", clusterHeaderForPluginConfig(pluginConfig, route))
}

func TestClusterHeaderForPluginConfigFallsBackToGlobal(t *testing.T) {
	route := wrapperRoute("route-b", "api.example.com", "llm-b.internal.dns", 80)
	pluginConfig := mustStruct(t, map[string]interface{}{
		"lb_type":   "cluster",
		"lb_policy": "cluster_metrics",
		"lb_config": map[string]interface{}{
			"mode": "LeastBusy",
		},
		"_rules_": []interface{}{
			map[string]interface{}{
				"_match_route_": []interface{}{"route-a"},
				"lb_type":       "endpoint",
				"lb_policy":     "endpoint_metrics",
			},
		},
	})

	require.Equal(t, defaultAILoadBalancerClusterHeader, clusterHeaderForPluginConfig(pluginConfig, route))
}

func TestClusterHeaderForPluginConfigDoesNotFallbackAfterMatchedEndpointRule(t *testing.T) {
	route := wrapperRoute("route-a", "api.example.com", "llm-a.internal.dns", 80)
	pluginConfig := mustStruct(t, map[string]interface{}{
		"lb_type":   "cluster",
		"lb_policy": "cluster_metrics",
		"_rules_": []interface{}{
			map[string]interface{}{
				"_match_route_": []interface{}{"route-a"},
				"lb_type":       "endpoint",
				"lb_policy":     "endpoint_metrics",
			},
		},
	})

	require.Empty(t, clusterHeaderForPluginConfig(pluginConfig, route))
}

func TestClusterHeaderForPluginConfigMatchesDomainAndService(t *testing.T) {
	route := wrapperRoute("route-c", "api.example.com:443", "llm-c.internal.dns", 8080)
	domainRule := mustStruct(t, map[string]interface{}{
		"_match_domain_": []interface{}{"*.example.com"},
		"lb_type":        "cluster",
		"lb_policy":      "cluster_metrics",
		"lb_config": map[string]interface{}{
			"cluster_header": "x-domain-target",
		},
	})
	serviceRule := mustStruct(t, map[string]interface{}{
		"_match_service_": []interface{}{"llm-c.internal.dns:8080"},
		"lb_type":         "cluster",
		"lb_policy":       "cluster_metrics",
		"lb_config": map[string]interface{}{
			"cluster_header": "x-service-target",
		},
	})

	require.True(t, routeMatchesWasmRule(route, domainRule))
	require.True(t, routeMatchesWasmRule(route, serviceRule))
}

func TestRouteMatchesWasmRuleRequiresRouteAndService(t *testing.T) {
	route := wrapperRoute("route-c", "api.example.com", "llm-c.internal.dns", 8080)
	rule := mustStruct(t, map[string]interface{}{
		"_match_route_":   []interface{}{"route-c"},
		"_match_service_": []interface{}{"llm-other.internal.dns:8080"},
		"lb_type":         "cluster",
	})

	require.False(t, routeMatchesWasmRule(route, rule))

	rule = mustStruct(t, map[string]interface{}{
		"_match_route_":   []interface{}{"route-c"},
		"_match_service_": []interface{}{"llm-c.internal.dns:8080"},
		"lb_type":         "cluster",
	})

	require.True(t, routeMatchesWasmRule(route, rule))
}

func TestRouteMatchesWasmRuleUsesRoutePrefixBeforeServiceWithoutRoute(t *testing.T) {
	route := wrapperRoute("route-c", "api.example.com", "llm-c.internal.dns", 8080)
	rule := mustStruct(t, map[string]interface{}{
		"_match_route_prefix_": []interface{}{"route-"},
		"_match_service_":      []interface{}{"llm-other.internal.dns:8080"},
		"lb_type":              "cluster",
	})

	require.True(t, routeMatchesWasmRule(route, rule))
}

func TestConstructAILoadBalancerClusterHeaderEnvoyFilter(t *testing.T) {
	envoyFilter := constructAILoadBalancerClusterHeaderEnvoyFilter(map[string]string{
		"route-b": "x-b",
		"route-a": "x-a",
	}, "higress-system")

	require.NotNil(t, envoyFilter)
	require.Equal(t, "ai-load-balancer-cluster-header", envoyFilter.Name)

	spec := envoyFilter.Spec.(*v1alpha3.EnvoyFilter)
	require.Len(t, spec.ConfigPatches, 2)
	require.Equal(t, "route-a", spec.ConfigPatches[0].GetMatch().GetRouteConfiguration().GetVhost().GetRoute().GetName())
	require.Equal(t, "x-a", spec.ConfigPatches[0].GetPatch().GetValue().GetFields()["route"].GetStructValue().GetFields()["cluster_header"].GetStringValue())
	require.Equal(t, "route-b", spec.ConfigPatches[1].GetMatch().GetRouteConfiguration().GetVhost().GetRoute().GetName())
	require.Equal(t, "x-b", spec.ConfigPatches[1].GetPatch().GetValue().GetFields()["route"].GetStructValue().GetFields()["cluster_header"].GetStringValue())
}

func TestSelectAILoadBalancerClusterHeaderIgnoresEndpointPlugin(t *testing.T) {
	route := wrapperRoute("route-a", "api.example.com", "llm-a.internal.dns", 80)
	plugin := &extensions.WasmPlugin{
		PluginName: aiLoadBalancerPluginName,
		PluginConfig: mustStruct(t, map[string]interface{}{
			"lb_type":   "endpoint",
			"lb_policy": "endpoint_metrics",
		}),
	}

	require.Empty(t, selectAILoadBalancerClusterHeader([]*extensions.WasmPlugin{plugin}, route))
}

func wrapperRoute(routeName, host, destinationHost string, port uint32) *common.WrapperHTTPRoute {
	return &common.WrapperHTTPRoute{
		HTTPRoute: &v1alpha3.HTTPRoute{
			Name: routeName,
			Route: []*v1alpha3.HTTPRouteDestination{
				{
					Destination: &v1alpha3.Destination{
						Host: destinationHost,
						Port: &v1alpha3.PortSelector{Number: port},
					},
					Weight: 100,
				},
			},
		},
		Host: host,
	}
}

func mustStruct(t *testing.T, value map[string]interface{}) *structpb.Struct {
	out, err := structpb.NewStruct(value)
	require.NoError(t, err)
	return out
}
