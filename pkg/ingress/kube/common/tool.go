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

package common

import (
	"crypto/md5"
	"encoding/hex"
	"net"
	"sort"
	"strings"

	networking "istio.io/api/networking/v1alpha3"
	"istio.io/istio/pilot/pkg/model"
	"istio.io/istio/pkg/cluster"
	"istio.io/istio/pkg/config"
	"istio.io/istio/pkg/kube"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	networkingv1beta1 "k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/util/version"

	netv1 "github.com/alibaba/higress/v2/client/pkg/apis/networking/v1"
	. "github.com/alibaba/higress/v2/pkg/ingress/log"
)

func ValidateBackendResource(resource *v1.TypedLocalObjectReference) bool {
	if resource == nil || resource.APIGroup == nil ||
		*resource.APIGroup != netv1.SchemeGroupVersion.Group ||
		resource.Kind != "McpBridge" || resource.Name != "default" {
		return false
	}
	return true
}

// V1Available check if the "networking/v1" Ingress is available.
func V1Available(client kube.Client) bool {
	// check kubernetes version to use new ingress package or not
	version119, _ := version.ParseGeneric("v1.19.0")

	serverVersion, err := client.GetKubernetesVersion()
	if err != nil {
		// Consider the new ingress package is available as default
		return true
	}

	runningVersion, err := version.ParseGeneric(serverVersion.String())
	if err != nil {
		// Consider the new ingress package is available as default
		IngressLog.Errorf("unexpected error parsing running Kubernetes version: %v", err)
		return true
	}

	return runningVersion.AtLeast(version119)
}

// NetworkingIngressAvailable check if the "networking" group Ingress is available.
func NetworkingIngressAvailable(client kube.Client) bool {
	// check kubernetes version to use new ingress package or not
	version118, _ := version.ParseGeneric("v1.18.0")

	serverVersion, err := client.GetKubernetesVersion()
	if err != nil {
		return false
	}

	runningVersion, err := version.ParseGeneric(serverVersion.String())
	if err != nil {
		IngressLog.Errorf("unexpected error parsing running Kubernetes version: %v", err)
		return false
	}

	return runningVersion.AtLeast(version118)
}

// SortIngressByCreationTime sorts the list of config objects in ascending order by their creation time (if available).
// When creation timestamps are equal, sort by namespace.name to ensure base ingress resources are prioritized
// over canary variants (e.g., "default.xx-hg" comes before "default.xx-hg-canary-by-header").
func SortIngressByCreationTime(configs []config.Config) {
	sort.Slice(configs, func(i, j int) bool {
		if configs[i].CreationTimestamp == configs[j].CreationTimestamp {
			in := configs[i].Namespace + "." + configs[i].Name
			jn := configs[j].Namespace + "." + configs[j].Name
			return in < jn
		}
		return configs[i].CreationTimestamp.Before(configs[j].CreationTimestamp)
	})
}

func CreateOrUpdateAnnotations(annotations map[string]string, options Options) map[string]string {
	out := make(map[string]string, len(annotations))

	for key, value := range annotations {
		out[key] = value
	}

	out[ClusterIdAnnotation] = options.ClusterId.String()
	out[RawClusterIdAnnotation] = options.RawClusterId
	return out
}

func GetClusterId(annotations map[string]string) cluster.ID {
	if len(annotations) == 0 {
		return ""
	}

	if value, exist := annotations[ClusterIdAnnotation]; exist {
		return cluster.ID(value)
	}

	return ""
}

func GetRawClusterId(annotations map[string]string) string {
	if len(annotations) == 0 {
		return ""
	}

	if value, exist := annotations[RawClusterIdAnnotation]; exist {
		return value
	}

	return ""
}

func GetHost(annotations map[string]string) string {
	if len(annotations) == 0 {
		return ""
	}

	if value, exist := annotations[HostAnnotation]; exist {
		return value
	}

	return ""
}

// Istio requires that the name of the gateway must conform to the DNS label.
// For details, you can view: https://github.com/istio/istio/blob/2d5c40ad5e9cceebe64106005aa38381097da2ba/pkg/config/validation/validation.go#L478
func ConvertToDNSLabelValid(input string) string {
	hasher := md5.New()
	hasher.Write([]byte(input))
	hash := hasher.Sum(nil)

	return hex.EncodeToString(hash[4:12])
}

// CleanHost follow the format of mse-ops for host.
func CleanHost(host string) string {
	return ConvertToDNSLabelValid(host)
}

func CreateConvertedName(items ...string) string {
	for i := len(items) - 1; i >= 0; i-- {
		if items[i] == "" {
			items = append(items[:i], items[i+1:]...)
		}
	}
	return strings.Join(items, "-")
}

// SortHTTPRoutes sort routes base on path type and path length
func SortHTTPRoutes(routes []*WrapperHTTPRoute) {
	isDefaultBackend := func(route *WrapperHTTPRoute) bool {
		return route.IsDefaultBackend
	}

	isAllCatch := func(route *WrapperHTTPRoute) bool {
		if route.OriginPathType == Prefix && route.OriginPath == "/" {
			if route.HTTPRoute.Match == nil {
				return true
			}

			match := route.HTTPRoute.Match[0]
			if len(match.Headers) == 0 && len(match.QueryParams) == 0 && match.Method == nil {
				return true
			}
		}
		return false
	}

	// default backend,user specified root path => path type => path length =>
	// methods => header => query param
	// refer https://gateway-api.sigs.k8s.io/v1alpha2/references/spec/#gateway.networking.k8s.io/v1beta1.HTTPRouteSpec
	sort.SliceStable(routes, func(i, j int) bool {
		// Move default backend to end
		if isDefaultBackend(routes[i]) {
			return false
		}
		if isDefaultBackend(routes[j]) {
			return true
		}

		// Move user specified root path match to end
		if isAllCatch(routes[i]) {
			return false
		}
		if isAllCatch(routes[j]) {
			return true
		}

		if routes[i].OriginPathType == routes[j].OriginPathType {
			if in, jn := len(routes[i].OriginPath), len(routes[j].OriginPath); in != jn {
				return in > jn
			}

			match1, match2 := routes[i].HTTPRoute.Match[0], routes[j].HTTPRoute.Match[0]
			// methods
			if in, jn := len(match1.Method.GetRegex()), len(match2.Method.GetRegex()); in != jn {
				if in != 0 && jn != 0 {
					return in < jn
				}
				return in != 0
			}
			// headers
			if in, jn := len(match1.Headers), len(match2.Headers); in != jn {
				return in > jn
			}
			// query params
			if in, jn := len(match1.QueryParams), len(match2.QueryParams); in != jn {
				return in > jn
			}
			return false
		}

		if routes[i].OriginPathType == Exact {
			return true
		}

		if routes[i].OriginPathType != Exact &&
			routes[j].OriginPathType != Exact {
			return routes[i].OriginPathType == Prefix
		}

		return false
	})
}

func constructRouteName(route *WrapperHTTPRoute) string {
	var builder strings.Builder
	// host-pathType-path
	base := route.PathFormat()
	builder.WriteString(base)

	var mappings []string
	var headerMappings []string
	var queryMappings []string
	if len(route.HTTPRoute.Match) > 0 {
		match := route.HTTPRoute.Match[0]
		if len(match.Headers) != 0 {
			for k, v := range match.Headers {
				var mapping string
				switch v.GetMatchType().(type) {
				case *networking.StringMatch_Exact:
					mapping = CreateConvertedName("exact", k, v.GetExact())
				case *networking.StringMatch_Prefix:
					mapping = CreateConvertedName("prefix", k, v.GetPrefix())
				case *networking.StringMatch_Regex:
					mapping = CreateConvertedName("regex", k, v.GetRegex())
				}

				headerMappings = append(headerMappings, mapping)
			}

			sort.SliceStable(headerMappings, func(i, j int) bool {
				return headerMappings[i] < headerMappings[j]
			})
		}

		if len(match.QueryParams) != 0 {
			for k, v := range match.QueryParams {
				var mapping string
				switch v.GetMatchType().(type) {
				case *networking.StringMatch_Exact:
					mapping = strings.Join([]string{"exact", k, v.GetExact()}, ":")
				case *networking.StringMatch_Prefix:
					mapping = strings.Join([]string{"prefix", k, v.GetPrefix()}, ":")
				case *networking.StringMatch_Regex:
					mapping = strings.Join([]string{"regex", k, v.GetRegex()}, ":")
				}

				queryMappings = append(queryMappings, mapping)
			}

			sort.SliceStable(queryMappings, func(i, j int) bool {
				return queryMappings[i] < queryMappings[j]
			})
		}
	}

	mappings = append(mappings, headerMappings...)
	mappings = append(mappings, queryMappings...)

	if len(mappings) == 0 {
		return CreateConvertedName(base)
	}

	return CreateConvertedName(base, CreateConvertedName(mappings...))
}

func partMd5(raw string) string {
	hash := md5.Sum([]byte(raw))
	encoded := hex.EncodeToString(hash[:])
	return encoded[:4] + encoded[len(encoded)-4:]
}

func GenerateUniqueRouteName(defaultNs string, route *WrapperHTTPRoute) string {
	if route.WrapperConfig.Config.Namespace == defaultNs {
		return route.WrapperConfig.Config.Name
	}
	return route.Meta()
}

func GenerateUniqueRouteNameWithSuffix(defaultNs string, route *WrapperHTTPRoute, suffix string) string {
	return CreateConvertedName(GenerateUniqueRouteName(defaultNs, route), suffix)
}

func SplitServiceFQDN(fqdn string) (string, string, bool) {
	parts := strings.Split(fqdn, ".")
	if len(parts) > 1 {
		return parts[0], parts[1], true
	}
	return "", "", false
}

func ConvertBackendService(routeDestination *networking.HTTPRouteDestination) model.BackendService {
	parts := strings.Split(routeDestination.Destination.Host, ".")
	var namespace, name string
	if len(parts) == 2 || len(parts) > 2 && strings.HasSuffix(routeDestination.Destination.Host, "cluster.local") {
		name = parts[0]
		namespace = parts[1]
	} else {
		name = routeDestination.Destination.Host
	}
	service := model.BackendService{
		Namespace: namespace,
		Name:      name,
		Weight:    routeDestination.Weight,
	}
	if routeDestination.Destination.Port != nil {
		service.Port = routeDestination.Destination.Port.Number
	}
	return service
}

func getLoadBalancerIp(svc *v1.Service) []string {
	var out []string

	for _, ingress := range svc.Status.LoadBalancer.Ingress {
		if ingress.IP != "" {
			out = append(out, ingress.IP)
		}

		if ingress.Hostname != "" {
			hostName := strings.TrimSuffix(ingress.Hostname, SvcHostNameSuffix)
			if net.ParseIP(hostName) != nil {
				out = append(out, hostName)
			}
		}
	}

	return out
}

func getSvcIpList(svcList []*v1.Service) []string {
	var targetSvcList []*v1.Service
	for _, svc := range svcList {
		if svc.Spec.Type == v1.ServiceTypeLoadBalancer &&
			strings.HasPrefix(svc.Name, clusterPrefix) {
			targetSvcList = append(targetSvcList, svc)
		}
	}

	var out []string
	for _, svc := range targetSvcList {
		out = append(out, getLoadBalancerIp(svc)...)
	}
	return out
}

func SortLbIngressList(lbi []v1.LoadBalancerIngress) func(int, int) bool {
	return func(i int, j int) bool {
		return lbi[i].IP < lbi[j].IP
	}
}

func GetLbStatusList(svcList []*v1.Service) []v1.LoadBalancerIngress {
	svcIpList := getSvcIpList(svcList)
	lbi := make([]v1.LoadBalancerIngress, 0, len(svcIpList))
	for _, ep := range svcIpList {
		lbi = append(lbi, v1.LoadBalancerIngress{IP: ep})
	}

	sort.SliceStable(lbi, SortLbIngressList(lbi))
	return lbi
}

func SortLbIngressListV1(lbi []networkingv1.IngressLoadBalancerIngress) func(int, int) bool {
	return func(i int, j int) bool {
		return lbi[i].IP < lbi[j].IP
	}
}

func GetLbStatusListV1(svcList []*v1.Service) []networkingv1.IngressLoadBalancerIngress {
	svcIpList := getSvcIpList(svcList)
	lbi := make([]networkingv1.IngressLoadBalancerIngress, 0, len(svcIpList))
	for _, ep := range svcIpList {
		lbi = append(lbi, networkingv1.IngressLoadBalancerIngress{IP: ep})
	}

	sort.SliceStable(lbi, SortLbIngressListV1(lbi))
	return lbi
}

func SortLbIngressListV1Beta1(lbi []networkingv1beta1.IngressLoadBalancerIngress) func(int, int) bool {
	return func(i int, j int) bool {
		return lbi[i].IP < lbi[j].IP
	}
}

func GetLbStatusListV1Beta1(svcList []*v1.Service) []networkingv1beta1.IngressLoadBalancerIngress {
	svcIpList := getSvcIpList(svcList)
	lbi := make([]networkingv1beta1.IngressLoadBalancerIngress, 0, len(svcIpList))
	for _, ep := range svcIpList {
		lbi = append(lbi, networkingv1beta1.IngressLoadBalancerIngress{IP: ep})
	}

	sort.SliceStable(lbi, SortLbIngressListV1Beta1(lbi))
	return lbi
}
