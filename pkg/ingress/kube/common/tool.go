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

// SortIngressByCreationTime sorts the list of config objects in ascending
// order by their creation time (if available). When two objects share the
// same creation timestamp, ties are broken by namespace first and then by
// name, both in lexicographic order.
//
// Note: this sorter does NOT actively identify canary ingresses. A base
// ingress sorting before its "<name>-canary-*" variants is merely a
// lexicographic consequence of the namespace/name ordering, and is only
// guaranteed when the canary names share the base name as a prefix. Canary
// ingresses named without that prefix may sort in any order relative to the
// base, since canary status is determined by annotations rather than by name.
func SortIngressByCreationTime(configs []config.Config) {
	sort.Slice(configs, func(i, j int) bool {
		if configs[i].CreationTimestamp == configs[j].CreationTimestamp {
			if configs[i].Namespace != configs[j].Namespace {
				return configs[i].Namespace < configs[j].Namespace
			}
			return configs[i].Name < configs[j].Name
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
			if len(route.HTTPRoute.Match) == 0 {
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
	less := func(a, b *WrapperHTTPRoute) bool {
		// Move default backend to end
		if isDefaultBackend(a) {
			return false
		}
		if isDefaultBackend(b) {
			return true
		}

		// Move user specified root path match to end
		if isAllCatch(a) {
			return false
		}
		if isAllCatch(b) {
			return true
		}

		if a.OriginPathType == b.OriginPathType {
			if in, jn := len(a.OriginPath), len(b.OriginPath); in != jn {
				return in > jn
			}

			lenI, lenJ := len(a.HTTPRoute.Match), len(b.HTTPRoute.Match)
			if lenI == 0 && lenJ == 0 {
				return false
			}
			if lenI == 0 {
				return false
			}
			if lenJ == 0 {
				return true
			}
			match1, match2 := a.HTTPRoute.Match[0], b.HTTPRoute.Match[0]
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

		if a.OriginPathType == Exact {
			return true
		}

		if a.OriginPathType != Exact &&
			b.OriginPathType != Exact {
			return a.OriginPathType == Prefix
		}

		return false
	}

	// Group (canary, base) pairs into sort units so that a canary route always
	// stays adjacent to its base route, as required by the nginx-ingress
	// semantic model. Units are compared with each other through their base
	// route, and within a unit the canary routes come first, followed by the
	// base. Routes without a canary relationship each form their own unit, so
	// configurations without canary routes sort exactly as before.
	// See https://github.com/higress-group/higress/issues/4849
	units := groupCanaryRoutes(routes)
	sort.SliceStable(units, func(i, j int) bool {
		return less(units[i].rep, units[j].rep)
	})

	n := 0
	for _, unit := range units {
		for _, canary := range unit.canaries {
			routes[n] = canary
			n++
		}
		routes[n] = unit.rep
		n++
	}
}

// routeSortUnit is a logical unit used by SortHTTPRoutes. rep is the
// representative route whose attributes are used for comparisons against
// other units. canaries are the canary routes bound to rep, emitted right
// before it in their original relative order.
type routeSortUnit struct {
	rep      *WrapperHTTPRoute
	canaries []*WrapperHTTPRoute
}

// groupCanaryRoutes binds canary routes to their base route. A route is
// treated as the base of a canary when both share the same non-empty RuleKey
// and the base itself is not a canary, mirroring the association applied by
// the ingress controller. Every route starts as its own unit; canary routes
// with a resolvable base are moved into the base's unit.
func groupCanaryRoutes(routes []*WrapperHTTPRoute) []*routeSortUnit {
	units := make([]*routeSortUnit, 0, len(routes))
	baseUnitByRuleKey := make(map[string]*routeSortUnit, len(routes))
	for _, route := range routes {
		unit := &routeSortUnit{rep: route}
		units = append(units, unit)
		if route.RuleKey == "" || isCanaryHTTPRoute(route) {
			continue
		}
		// Only the first non-canary route of a rule key is a pairing candidate,
		// so duplicate keys never merge unrelated routes into one unit.
		if _, ok := baseUnitByRuleKey[route.RuleKey]; !ok {
			baseUnitByRuleKey[route.RuleKey] = unit
		}
	}

	for i, route := range routes {
		if route.RuleKey == "" || !isCanaryHTTPRoute(route) {
			continue
		}
		baseUnit, ok := baseUnitByRuleKey[route.RuleKey]
		if !ok || baseUnit.rep == route {
			// A canary without its base route keeps sorting on its own
			// attributes.
			continue
		}
		// Detach the canary from its own unit and bind it to the base unit.
		units[i] = nil
		baseUnit.canaries = append(baseUnit.canaries, route)
	}

	return compactSortUnits(units)
}

// isCanaryHTTPRoute reports whether the route originates from a canary
// ingress. Routes without annotation information are never canaries.
func isCanaryHTTPRoute(route *WrapperHTTPRoute) bool {
	if route == nil || route.WrapperConfig == nil || route.WrapperConfig.AnnotationsConfig == nil {
		return false
	}
	return route.WrapperConfig.AnnotationsConfig.IsCanary()
}

// compactSortUnits drops units detached by groupCanaryRoutes while preserving
// the order of the remaining units.
func compactSortUnits(units []*routeSortUnit) []*routeSortUnit {
	compact := units[:0]
	for _, unit := range units {
		if unit != nil {
			compact = append(compact, unit)
		}
	}
	return compact
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
			} else {
				out = append(out, ingress.Hostname)
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
		return loadBalancerIngressAddress(lbi[i].IP, lbi[i].Hostname) <
			loadBalancerIngressAddress(lbi[j].IP, lbi[j].Hostname)
	}
}

func GetLbStatusList(svcList []*v1.Service) []v1.LoadBalancerIngress {
	svcIpList := getSvcIpList(svcList)
	lbi := make([]v1.LoadBalancerIngress, 0, len(svcIpList))
	for _, ep := range svcIpList {
		if net.ParseIP(ep) != nil {
			lbi = append(lbi, v1.LoadBalancerIngress{IP: ep})
		} else {
			lbi = append(lbi, v1.LoadBalancerIngress{Hostname: ep})
		}
	}

	sort.SliceStable(lbi, SortLbIngressList(lbi))
	return lbi
}

func SortLbIngressListV1(lbi []networkingv1.IngressLoadBalancerIngress) func(int, int) bool {
	return func(i int, j int) bool {
		return loadBalancerIngressAddress(lbi[i].IP, lbi[i].Hostname) <
			loadBalancerIngressAddress(lbi[j].IP, lbi[j].Hostname)
	}
}

func GetLbStatusListV1(svcList []*v1.Service) []networkingv1.IngressLoadBalancerIngress {
	svcIpList := getSvcIpList(svcList)
	lbi := make([]networkingv1.IngressLoadBalancerIngress, 0, len(svcIpList))
	for _, ep := range svcIpList {
		if net.ParseIP(ep) != nil {
			lbi = append(lbi, networkingv1.IngressLoadBalancerIngress{IP: ep})
		} else {
			lbi = append(lbi, networkingv1.IngressLoadBalancerIngress{Hostname: ep})
		}
	}

	sort.SliceStable(lbi, SortLbIngressListV1(lbi))
	return lbi
}

func SortLbIngressListV1Beta1(lbi []networkingv1beta1.IngressLoadBalancerIngress) func(int, int) bool {
	return func(i int, j int) bool {
		return loadBalancerIngressAddress(lbi[i].IP, lbi[i].Hostname) <
			loadBalancerIngressAddress(lbi[j].IP, lbi[j].Hostname)
	}
}

func GetLbStatusListV1Beta1(svcList []*v1.Service) []networkingv1beta1.IngressLoadBalancerIngress {
	svcIpList := getSvcIpList(svcList)
	lbi := make([]networkingv1beta1.IngressLoadBalancerIngress, 0, len(svcIpList))
	for _, ep := range svcIpList {
		if net.ParseIP(ep) != nil {
			lbi = append(lbi, networkingv1beta1.IngressLoadBalancerIngress{IP: ep})
		} else {
			lbi = append(lbi, networkingv1beta1.IngressLoadBalancerIngress{Hostname: ep})
		}
	}

	sort.SliceStable(lbi, SortLbIngressListV1Beta1(lbi))
	return lbi
}

func loadBalancerIngressAddress(ip, hostname string) string {
	if ip != "" {
		return ip
	}
	return hostname
}
