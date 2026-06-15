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
	"errors"
	"fmt"
	"time"

	"istio.io/istio/pilot/pkg/model"
	"istio.io/istio/pkg/cluster"
	"istio.io/istio/pkg/config"
	"istio.io/istio/pkg/config/schema/collection"
	"istio.io/istio/pkg/config/schema/collections"
	"k8s.io/apimachinery/pkg/labels"

	. "github.com/alibaba/higress/v2/pkg/ingress/log"
)

type PathType string

const (
	prefixAnnotation = "internal.higress.io/"

	ClusterIdAnnotation = prefixAnnotation + "cluster-id"

	RawClusterIdAnnotation = prefixAnnotation + "raw-cluster-id"

	HostAnnotation = prefixAnnotation + "host"

	// PrefixMatchRegex optionally matches "/..." at the end of a path.
	// regex taken from https://github.com/projectcontour/contour/blob/2b3376449bedfea7b8cea5fbade99fb64009c0f6/internal/envoy/v3/route.go#L59
	PrefixMatchRegex = `((\/).*)?`

	DefaultHost = "*"

	DefaultPath = "/"

	// DefaultIngressClass defines the default class used in the nginx ingress controller.
	// For compatible ingress nginx case, istio controller will watch ingresses whose ingressClass is
	// nginx, empty string or unset.
	DefaultIngressClass = "nginx"

	Exact PathType = "exact"

	Prefix PathType = "prefix"

	// PrefixRegex :if PathType is PrefixRegex, then the /foo/bar/[A-Z0-9]{3} is actually ^/foo/bar/[A-Z0-9]{3}.*
	PrefixRegex PathType = "prefixRegex"

	// FullPathRegex :if PathType is FullPathRegex, then the /foo/bar/[A-Z0-9]{3} is actually ^/foo/bar/[A-Z0-9]{3}$
	FullPathRegex PathType = "fullPathRegex"

	DefaultStatusUpdateInterval = 10 * time.Second

	AppKey            = "app"
	AppValue          = "higress-gateway"
	SvcHostNameSuffix = ".multiplenic"
)

var (
	ErrUnsupportedOp = errors.New("unsupported operation: the ingress config store is a read-only view")

	ErrNotFound = errors.New("item not found")

	Schemas = collection.SchemasFor(
		collections.VirtualService,
		collections.Gateway,
		collections.DestinationRule,
		collections.EnvoyFilter,
	)

	clusterPrefix    string
	SvcLabelSelector labels.Selector
)

func init() {
	set := labels.Set{
		AppKey: AppValue,
	}
	SvcLabelSelector = labels.SelectorFromSet(set)
}

type Options struct {
	Enable               bool
	ClusterId            cluster.ID
	IngressClass         string
	GatewayClass         string
	WatchNamespace       string
	RawClusterId         string
	EnableStatus         bool
	SystemNamespace      string
	GatewaySelectorKey   string
	GatewaySelectorValue string
	GatewayHttpPort      uint32
	GatewayHttpsPort     uint32
}

type BasicAuthRules struct {
	Rules []*Rule `json:"_rules_"`
}

type Rule struct {
	Realm       string   `json:"realm"`
	MatchRoute  []string `json:"_match_route_"`
	Credentials []string `json:"credentials"`
	Encrypted   bool     `json:"encrypted"`
}

type IngressDomainCache struct {
	// host as key
	Valid map[string]*IngressDomainBuilder

	Invalid []model.IngressDomain
}

func NewIngressDomainCache() *IngressDomainCache {
	return &IngressDomainCache{
		Valid: map[string]*IngressDomainBuilder{},
	}
}

func (i *IngressDomainCache) Extract() model.IngressDomainCollection {
	var valid []model.IngressDomain

	for _, builder := range i.Valid {
		valid = append(valid, builder.Build())
	}

	return model.IngressDomainCollection{
		Valid:   valid,
		Invalid: i.Invalid,
	}
}

func SameConfig(left *config.Config, right *config.Config) bool {
	if left == nil || right == nil {
		return left == right
	}
	return GetClusterId(left.Annotations) == GetClusterId(right.Annotations) &&
		left.Namespace == right.Namespace &&
		left.Name == right.Name
}

func IsPassthroughTLSHostOwner(convertOptions *ConvertOptions, cfg *config.Config, host string) bool {
	if convertOptions == nil || convertOptions.PassthroughTLSHostOwners == nil {
		return true
	}
	return SameConfig(convertOptions.PassthroughTLSHostOwners[host], cfg)
}

func PassthroughTLSHostOwner(convertOptions *ConvertOptions, host string) *config.Config {
	if convertOptions == nil || len(convertOptions.PassthroughTLSHostOwners) == 0 {
		return nil
	}
	return convertOptions.PassthroughTLSHostOwners[host]
}

func HasPassthroughTLSHostOwner(convertOptions *ConvertOptions, cfg *config.Config) bool {
	if convertOptions == nil || len(convertOptions.PassthroughTLSHostOwners) == 0 {
		return false
	}
	for _, owner := range convertOptions.PassthroughTLSHostOwners {
		if SameConfig(owner, cfg) {
			return true
		}
	}
	return false
}

type ConvertOptions struct {
	HostWithRule2Ingress map[string]*config.Config

	HostWithTls2Ingress map[string]*config.Config

	Gateways map[string]*WrapperGateway

	IngressDomainCache *IngressDomainCache

	// the host, path, headers, params of rule => ingress
	Route2Ingress map[string]*WrapperConfigWithRuleKey

	// Record valid/invalid routes from ingress
	IngressRouteCache *IngressRouteCache

	VirtualServices map[string]*WrapperVirtualService

	// host -> routes
	HTTPRoutes map[string][]*WrapperHTTPRoute

	CanaryIngresses []*WrapperConfig

	// Host to the first root-path ingress owner for hosts that have TLS passthrough enabled.
	PassthroughTLSHostOwners map[string]*config.Config

	Service2TrafficPolicy map[ServiceKey]*WrapperTrafficPolicy

	ServiceWrappers map[string]*ServiceWrapper

	ProxyWrappers map[string]*ProxyWrapper

	HasDefaultBackend bool
}

type IngressRouteCache struct {
	routes  map[string]*IngressRouteBuilder
	invalid []model.IngressRoute
}

func NewIngressRouteCache() *IngressRouteCache {
	return &IngressRouteCache{
		routes: map[string]*IngressRouteBuilder{},
	}
}

func (i *IngressRouteCache) New(route *WrapperHTTPRoute) *IngressRouteBuilder {
	return &IngressRouteBuilder{
		ClusterId: route.ClusterId,
		RouteName: route.HTTPRoute.Name,
		Path:      route.OriginPath,
		PathType:  string(route.OriginPathType),
		Host:      route.Host,
		Event:     Normal,
		Ingress:   route.WrapperConfig.Config,
	}
}

func (i *IngressRouteCache) NewAndAdd(route *WrapperHTTPRoute) {
	routeBuilder := &IngressRouteBuilder{
		ClusterId: route.ClusterId,
		RouteName: route.HTTPRoute.Name,
		Path:      route.OriginPath,
		PathType:  string(route.OriginPathType),
		Host:      route.Host,
		Event:     Normal,
		Ingress:   route.WrapperConfig.Config,
	}

	// Only care about the first destination
	destination := route.HTTPRoute.Route[0].Destination
	svcName, namespace, _ := SplitServiceFQDN(destination.Host)
	routeBuilder.ServiceList = []model.BackendService{
		{
			Name:      svcName,
			Namespace: namespace,
			Port:      destination.Port.Number,
			Weight:    route.HTTPRoute.Route[0].Weight,
		},
	}

	i.routes[route.HTTPRoute.Name] = routeBuilder
}

func (i *IngressRouteCache) Add(builder *IngressRouteBuilder) {
	if builder.Event != Normal {
		builder.RouteName = "invalid-route"
		i.invalid = append(i.invalid, builder.Build())
		return
	}

	i.routes[builder.RouteName] = builder
}

func (i *IngressRouteCache) Update(route *WrapperHTTPRoute) {
	oldBuilder, exist := i.routes[route.HTTPRoute.Name]
	if !exist {
		// Never happen
		IngressLog.Errorf("ingress route builder %s not found.", route.HTTPRoute.Name)
		return
	}

	var serviceList []model.BackendService
	for _, routeDestination := range route.HTTPRoute.Route {
		serviceList = append(serviceList, ConvertBackendService(routeDestination))
	}

	oldBuilder.ServiceList = serviceList
}

func (i *IngressRouteCache) Delete(route *WrapperHTTPRoute) {
	delete(i.routes, route.HTTPRoute.Name)
}

func (i *IngressRouteCache) Extract() model.IngressRouteCollection {
	var valid []model.IngressRoute

	for _, builder := range i.routes {
		valid = append(valid, builder.Build())
	}

	return model.IngressRouteCollection{
		Valid:   valid,
		Invalid: i.invalid,
	}
}

type IngressRouteBuilder struct {
	ClusterId   cluster.ID
	RouteName   string
	Host        string
	PathType    string
	Path        string
	ServiceList []model.BackendService
	PortName    string
	Event       Event
	Ingress     *config.Config
	PreIngress  *config.Config
}

func (i *IngressRouteBuilder) Build() model.IngressRoute {
	errorMsg := ""
	switch i.Event {
	case DuplicatedRoute:
		preClusterId := GetClusterId(i.PreIngress.Annotations)
		errorMsg = fmt.Sprintf("host %s and path %s in ingress %s/%s within cluster %s is already defined in ingress %s/%s within cluster %s",
			i.Host,
			i.Path,
			i.Ingress.Namespace,
			i.Ingress.Name,
			i.ClusterId,
			i.PreIngress.Namespace,
			i.PreIngress.Name,
			preClusterId)
	case InvalidBackendService:
		errorMsg = fmt.Sprintf("backend service of host %s and path %s is invalid defined in ingress %s/%s within cluster %s",
			i.Host,
			i.Path,
			i.Ingress.Namespace,
			i.Ingress.Name,
			i.ClusterId,
		)
	case PortNameResolveError:
		errorMsg = fmt.Sprintf("service port name %s of host %s and path %s resolves error defined in ingress %s/%s within cluster %s",
			i.PortName,
			i.Host,
			i.Path,
			i.Ingress.Namespace,
			i.Ingress.Name,
			i.ClusterId,
		)
	}

	ingressRoute := model.IngressRoute{
		Name:            i.RouteName,
		Host:            i.Host,
		Path:            i.Path,
		PathType:        i.PathType,
		DestinationType: model.Single,
		ServiceList:     i.ServiceList,
		Error:           errorMsg,
	}

	// backward compatibility
	if len(ingressRoute.ServiceList) > 0 {
		ingressRoute.ServiceName = i.ServiceList[0].Name
	}

	if len(ingressRoute.ServiceList) > 1 {
		ingressRoute.DestinationType = model.Multiple
	}

	return ingressRoute
}

type Protocol string

const (
	HTTP  Protocol = "HTTP"
	HTTPS Protocol = "HTTPS"
)

type IngressDomainBuilder struct {
	ClusterId cluster.ID
	Host      string
	Protocol  Protocol
	Event     Event
	// format is cluster id/namespace/name
	SecretName string
	Ingress    *config.Config
	PreIngress *config.Config
}

func (i *IngressDomainBuilder) Build() model.IngressDomain {
	errorMsg := ""
	switch i.Event {
	case MissingSecret:
		errorMsg = fmt.Sprintf("tls field of host %s defined in ingress %s/%s within cluster %s misses secret",
			i.Host,
			i.Ingress.Namespace,
			i.Ingress.Name,
			i.ClusterId,
		)
	case DuplicatedTls:
		preClusterId := GetClusterId(i.PreIngress.Annotations)
		errorMsg = fmt.Sprintf("tls field of host %s defined in ingress %s/%s within cluster %s "+
			"is conflicted with ingress %s/%s within cluster %s",
			i.Host,
			i.Ingress.Namespace,
			i.Ingress.Name,
			i.ClusterId,
			i.PreIngress.Namespace,
			i.PreIngress.Name,
			preClusterId,
		)
	}

	return model.IngressDomain{
		Host:         i.Host,
		Protocol:     string(i.Protocol),
		SecretName:   i.SecretName,
		CreationTime: i.Ingress.CreationTimestamp,
		Error:        errorMsg,
	}
}
