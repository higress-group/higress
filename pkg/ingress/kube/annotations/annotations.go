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

package annotations

import (
	"strings"

	networking "istio.io/api/networking/v1alpha3"
	"istio.io/istio/pkg/cluster"
	"istio.io/istio/pkg/util/sets"
	listersv1 "k8s.io/client-go/listers/core/v1"

	"github.com/alibaba/higress/v2/pkg/ingress/kube/mcpserver"
)

type GlobalContext struct {
	// secret key is cluster/namespace/name
	WatchedSecrets sets.Set[string]

	ClusterSecretLister map[cluster.ID]listersv1.SecretLister

	ClusterServiceList map[cluster.ID]listersv1.ServiceLister

	McpServers []*mcpserver.McpServer
}

type Meta struct {
	Namespace    string
	Name         string
	RawClusterId string
	ClusterId    cluster.ID
}

// Ingress defines the valid annotations present in one NGINX Ingress.
type Ingress struct {
	Meta

	Cors *CorsConfig

	Rewrite *RewriteConfig

	Redirect *RedirectConfig

	UpstreamTLS *UpstreamTLSConfig

	DownstreamTLS *DownstreamTLSConfig

	SSLPassthrough *SSLPassthroughConfig

	Canary *CanaryConfig

	IPAccessControl *IPAccessControlConfig

	Timeout *TimeoutConfig

	Retry *RetryConfig

	LoadBalance *LoadBalanceConfig

	localRateLimit *localRateLimitConfig

	Fallback *FallbackConfig

	Auth *AuthConfig

	Mirror *MirrorConfig

	Destination *DestinationConfig

	IgnoreCase *IgnoreCaseConfig

	Match *MatchConfig

	HeaderControl *HeaderControlConfig

	Http2Rpc *Http2RpcConfig
}

func (i *Ingress) NeedRegexMatch(path string) bool {
	if i.Rewrite == nil {
		return false
	}
	if i.Rewrite.RewriteTarget != "" && strings.ContainsAny(path, `\.+*?()|[]{}^$`) {
		return true
	}
	if strings.ContainsAny(i.Rewrite.RewriteTarget, `$\`) {
		return true
	}
	return i.IsPrefixRegexMatch() || i.IsFullPathRegexMatch()
}

func (i *Ingress) IsPrefixRegexMatch() bool {
	return i.Rewrite.UseRegex
}

func (i *Ingress) IsFullPathRegexMatch() bool {
	return i.Rewrite.FullPathRegex
}

func (i *Ingress) IsCanary() bool {
	if i.Canary == nil {
		return false
	}

	return i.Canary.Enabled
}

func (i *Ingress) IsSSLPassthrough() bool {
	return i.SSLPassthrough != nil && i.SSLPassthrough.Enabled
}

// CanaryKind return byHeader, byWeight
func (i *Ingress) CanaryKind() (bool, bool) {
	if !i.IsCanary() {
		return false, false
	}

	// first header, cookie
	if i.Canary.Header != "" || i.Canary.Cookie != "" {
		return true, false
	}

	// then weight
	return false, true
}

func (i *Ingress) NeedTrafficPolicy() bool {
	return i.UpstreamTLS != nil ||
		i.LoadBalance != nil
}

type AnnotationHandler interface {
	Parser
	GatewayHandler
	VirtualServiceHandler
	RouteHandler
	TrafficPolicyHandler
}

type AnnotationHandlerManager struct {
	parsers                []Parser
	gatewayHandlers        []GatewayHandler
	virtualServiceHandlers []VirtualServiceHandler
	routeHandlers          []RouteHandler
	trafficPolicyHandlers  []TrafficPolicyHandler
}

func NewAnnotationHandlerManager() AnnotationHandler {
	return &AnnotationHandlerManager{
		parsers: []Parser{
			canary{},
			cors{},
			downstreamTLS{},
			sslPassthrough{},
			redirect{},
			rewrite{},
			upstreamTLS{},
			ipAccessControl{},
			timeout{},
			retry{},
			loadBalance{},
			localRateLimit{},
			fallback{},
			auth{},
			mirror{},
			destination{},
			ignoreCaseMatching{},
			match{},
			headerControl{},
			http2rpc{},
			mcpServer{},
		},
		gatewayHandlers: []GatewayHandler{
			downstreamTLS{},
		},
		virtualServiceHandlers: []VirtualServiceHandler{
			ipAccessControl{},
		},
		routeHandlers: []RouteHandler{
			cors{},
			redirect{},
			rewrite{},
			ipAccessControl{},
			timeout{},
			retry{},
			localRateLimit{},
			fallback{},
			mirror{},
			ignoreCaseMatching{},
			match{},
			headerControl{},
		},
		trafficPolicyHandlers: []TrafficPolicyHandler{
			upstreamTLS{},
			loadBalance{},
		},
	}
}

func (h *AnnotationHandlerManager) Parse(annotations Annotations, config *Ingress, globalContext *GlobalContext) error {
	for _, parser := range h.parsers {
		_ = parser.Parse(annotations, config, globalContext)
	}

	return nil
}

func (h *AnnotationHandlerManager) ApplyGateway(gateway *networking.Gateway, config *Ingress) {
	for _, handler := range h.gatewayHandlers {
		handler.ApplyGateway(gateway, config)
	}
}

func (h *AnnotationHandlerManager) ApplyVirtualServiceHandler(virtualService *networking.VirtualService, config *Ingress) {
	for _, handler := range h.virtualServiceHandlers {
		handler.ApplyVirtualServiceHandler(virtualService, config)
	}
}

func (h *AnnotationHandlerManager) ApplyRoute(route *networking.HTTPRoute, config *Ingress) {
	for _, handler := range h.routeHandlers {
		handler.ApplyRoute(route, config)
	}
}

func (h *AnnotationHandlerManager) ApplyTrafficPolicy(trafficPolicy *networking.TrafficPolicy, portTrafficPolicy *networking.TrafficPolicy_PortTrafficPolicy, config *Ingress) {
	for _, handler := range h.trafficPolicyHandlers {
		handler.ApplyTrafficPolicy(trafficPolicy, portTrafficPolicy, config)
	}
}
