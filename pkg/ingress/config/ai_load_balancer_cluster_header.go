package config

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alibaba/higress/v2/pkg/ingress/kube/common"
	"google.golang.org/protobuf/types/known/structpb"
	extensions "istio.io/api/extensions/v1alpha1"
	"istio.io/api/networking/v1alpha3"
	istioconfig "istio.io/istio/pkg/config"
	"istio.io/istio/pkg/config/schema/gvk"
)

const (
	aiLoadBalancerPluginName           = "ai-load-balancer"
	aiLoadBalancerClusterType          = "cluster"
	defaultAILoadBalancerClusterHeader = "x-higress-target-cluster"

	wasmRulesKey            = "_rules_"
	wasmMatchRouteKey       = "_match_route_"
	wasmMatchDomainKey      = "_match_domain_"
	wasmMatchServiceKey     = "_match_service_"
	wasmMatchRoutePrefixKey = "_match_route_prefix_"
)

func (m *IngressConfig) constructAILoadBalancerClusterHeaderEnvoyFilter(convertOptions *common.ConvertOptions) *istioconfig.Config {
	if convertOptions == nil {
		return nil
	}

	m.mutex.RLock()
	plugins := make([]*extensions.WasmPlugin, 0, len(m.wasmPlugins))
	for _, plugin := range m.wasmPlugins {
		if plugin.GetPluginName() == aiLoadBalancerPluginName {
			plugins = append(plugins, plugin.DeepCopy())
		}
	}
	m.mutex.RUnlock()

	sort.Slice(plugins, func(i, j int) bool {
		if plugins[i].GetPriority().GetValue() == plugins[j].GetPriority().GetValue() {
			return plugins[i].String() < plugins[j].String()
		}
		return plugins[i].GetPriority().GetValue() < plugins[j].GetPriority().GetValue()
	})

	routeHeaders := map[string]string{}
	for _, routes := range convertOptions.HTTPRoutes {
		for _, route := range routes {
			if route == nil || route.HTTPRoute == nil || strings.HasSuffix(route.HTTPRoute.Name, "app-root") {
				continue
			}
			if header := selectAILoadBalancerClusterHeader(plugins, route); header != "" {
				routeHeaders[route.HTTPRoute.Name] = header
			}
		}
	}
	if len(routeHeaders) == 0 {
		return nil
	}
	return constructAILoadBalancerClusterHeaderEnvoyFilter(routeHeaders, m.namespace)
}

func selectAILoadBalancerClusterHeader(plugins []*extensions.WasmPlugin, route *common.WrapperHTTPRoute) string {
	for _, plugin := range plugins {
		header := clusterHeaderForPluginConfig(plugin.GetPluginConfig(), route)
		if header != "" {
			return header
		}
	}
	return ""
}

func clusterHeaderForPluginConfig(pluginConfig *structpb.Struct, route *common.WrapperHTTPRoute) string {
	if pluginConfig == nil {
		return ""
	}
	for _, rule := range structList(pluginConfig.Fields[wasmRulesKey]) {
		if !routeMatchesWasmRule(route, rule) {
			continue
		}
		return clusterHeaderFromConfig(rule)
	}
	return clusterHeaderFromConfig(pluginConfig)
}

func clusterHeaderFromConfig(pluginConfig *structpb.Struct) string {
	if pluginConfig == nil || stringValue(pluginConfig.Fields["lb_type"]) != aiLoadBalancerClusterType {
		return ""
	}
	lbConfig := pluginConfig.Fields["lb_config"].GetStructValue()
	if lbConfig == nil {
		return defaultAILoadBalancerClusterHeader
	}
	if header := stringValue(lbConfig.Fields["cluster_header"]); header != "" {
		return header
	}
	return defaultAILoadBalancerClusterHeader
}

func routeMatchesWasmRule(route *common.WrapperHTTPRoute, rule *structpb.Struct) bool {
	if route == nil || route.HTTPRoute == nil || rule == nil {
		return false
	}
	switch {
	case len(stringList(rule.Fields[wasmMatchRouteKey])) > 0:
		return stringListContains(stringList(rule.Fields[wasmMatchRouteKey]), route.HTTPRoute.Name)
	case len(stringList(rule.Fields[wasmMatchDomainKey])) > 0:
		return domainListMatches(stringList(rule.Fields[wasmMatchDomainKey]), route.Host)
	case len(stringList(rule.Fields[wasmMatchServiceKey])) > 0:
		return serviceListMatches(stringList(rule.Fields[wasmMatchServiceKey]), route.HTTPRoute.Route)
	case len(stringList(rule.Fields[wasmMatchRoutePrefixKey])) > 0:
		for _, prefix := range stringList(rule.Fields[wasmMatchRoutePrefixKey]) {
			if strings.HasPrefix(route.HTTPRoute.Name, prefix) {
				return true
			}
		}
	}
	return false
}

func constructAILoadBalancerClusterHeaderEnvoyFilter(routeHeaders map[string]string, namespace string) *istioconfig.Config {
	routeNames := make([]string, 0, len(routeHeaders))
	for routeName := range routeHeaders {
		routeNames = append(routeNames, routeName)
	}
	sort.Strings(routeNames)

	configPatches := make([]*v1alpha3.EnvoyFilter_EnvoyConfigObjectPatch, 0, len(routeNames))
	for _, routeName := range routeNames {
		configPatches = append(configPatches, &v1alpha3.EnvoyFilter_EnvoyConfigObjectPatch{
			ApplyTo: v1alpha3.EnvoyFilter_HTTP_ROUTE,
			Match: &v1alpha3.EnvoyFilter_EnvoyConfigObjectMatch{
				Context: v1alpha3.EnvoyFilter_GATEWAY,
				ObjectTypes: &v1alpha3.EnvoyFilter_EnvoyConfigObjectMatch_RouteConfiguration{
					RouteConfiguration: &v1alpha3.EnvoyFilter_RouteConfigurationMatch{
						Vhost: &v1alpha3.EnvoyFilter_RouteConfigurationMatch_VirtualHostMatch{
							Route: &v1alpha3.EnvoyFilter_RouteConfigurationMatch_RouteMatch{
								Action: v1alpha3.EnvoyFilter_RouteConfigurationMatch_RouteMatch_ROUTE,
								Name:   routeName,
							},
						},
					},
				},
			},
			Patch: &v1alpha3.EnvoyFilter_Patch{
				Operation: v1alpha3.EnvoyFilter_Patch_MERGE,
				Value: buildPatchStruct(fmt.Sprintf(`{
					"route": {
						"cluster_header": %q
					}
				}`, routeHeaders[routeName])),
			},
		})
	}

	return &istioconfig.Config{
		Meta: istioconfig.Meta{
			GroupVersionKind: gvk.EnvoyFilter,
			Name:             "ai-load-balancer-cluster-header",
			Namespace:        namespace,
		},
		Spec: &v1alpha3.EnvoyFilter{
			ConfigPatches: configPatches,
		},
	}
}

func structList(value *structpb.Value) []*structpb.Struct {
	list := value.GetListValue()
	if list == nil {
		return nil
	}
	out := make([]*structpb.Struct, 0, len(list.Values))
	for _, item := range list.Values {
		if item.GetStructValue() != nil {
			out = append(out, item.GetStructValue())
		}
	}
	return out
}

func stringList(value *structpb.Value) []string {
	list := value.GetListValue()
	if list == nil {
		return nil
	}
	out := make([]string, 0, len(list.Values))
	for _, item := range list.Values {
		if item.GetStringValue() != "" {
			out = append(out, item.GetStringValue())
		}
	}
	return out
}

func stringValue(value *structpb.Value) string {
	if value == nil {
		return ""
	}
	return value.GetStringValue()
}

func stringListContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func domainListMatches(patterns []string, routeHost string) bool {
	routeHost = stripPortFromHost(routeHost)
	for _, pattern := range patterns {
		switch {
		case strings.HasPrefix(pattern, "*"):
			if strings.HasSuffix(routeHost, pattern[1:]) {
				return true
			}
		case strings.HasSuffix(pattern, "*"):
			if strings.HasPrefix(routeHost, pattern[:len(pattern)-1]) {
				return true
			}
		case routeHost == pattern:
			return true
		}
	}
	return false
}

func stripPortFromHost(reqHost string) string {
	portStart := strings.LastIndexByte(reqHost, ':')
	if portStart != -1 {
		v6EndIndex := strings.LastIndexByte(reqHost, ']')
		if v6EndIndex == -1 || v6EndIndex < portStart {
			return reqHost[:portStart]
		}
	}
	return reqHost
}

func serviceListMatches(services []string, destinations []*v1alpha3.HTTPRouteDestination) bool {
	for _, destination := range destinations {
		serviceName := clusterNameForDestination(destination)
		if serviceName == "" {
			continue
		}
		for _, service := range services {
			colonIndex := strings.LastIndexByte(service, ':')
			if colonIndex != -1 {
				if serviceName == fmt.Sprintf("outbound|%s||%s", service[colonIndex+1:], service[:colonIndex]) {
					return true
				}
				continue
			}
			if strings.HasSuffix(serviceName, "||"+service) {
				return true
			}
		}
	}
	return false
}

func clusterNameForDestination(destination *v1alpha3.HTTPRouteDestination) string {
	if destination == nil || destination.GetDestination() == nil || destination.GetDestination().GetHost() == "" {
		return ""
	}
	port := destination.GetDestination().GetPort().GetNumber()
	if port == 0 {
		return ""
	}
	return fmt.Sprintf("outbound|%d||%s", port, destination.GetDestination().GetHost())
}
