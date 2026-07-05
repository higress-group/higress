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

package ingress

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/higress/v2/pkg/cert"
	"github.com/google/go-cmp/cmp"
	"istio.io/api/networking/v1alpha3"
	"istio.io/istio/pilot/pkg/model"
	istiomodel "istio.io/istio/pilot/pkg/model"
	"istio.io/istio/pkg/config"
	"istio.io/istio/pkg/config/schema/gvk"
	"istio.io/istio/pkg/config/schema/gvr"
	schemakubeclient "istio.io/istio/pkg/config/schema/kubeclient"
	"istio.io/istio/pkg/kube/controllers"
	ktypes "istio.io/istio/pkg/kube/kubetypes"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/networking/v1beta1"
	ingress "k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/watch"
	listerv1 "k8s.io/client-go/listers/core/v1"
	networkinglister "k8s.io/client-go/listers/networking/v1beta1"

	"github.com/alibaba/higress/v2/pkg/ingress/kube/annotations"
	"github.com/alibaba/higress/v2/pkg/ingress/kube/common"
	"github.com/alibaba/higress/v2/pkg/ingress/kube/secret"
	"github.com/alibaba/higress/v2/pkg/ingress/kube/util"
	"github.com/alibaba/higress/v2/pkg/kube"
	"github.com/stretchr/testify/require"
)

func TestIngressControllerApplies(t *testing.T) {
	fakeClient := kube.NewFakeClient()
	localKubeClient, client := fakeClient, fakeClient

	options := common.Options{IngressClass: "mse", ClusterId: ""}

	secretController := secret.NewController(localKubeClient, options)
	ingressController := NewController(localKubeClient, client, options, secretController)

	testcases := map[string]func(*testing.T, common.IngressController){
		"test apply canary ingress":  testApplyCanaryIngress,
		"test apply default backend": testApplyDefaultBackend,
	}
	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			tc(t, ingressController)
		})
	}
}

func TestSSLPassthroughConvertGatewayAndTLSRoute(t *testing.T) {
	c := controller{
		options: common.Options{
			GatewayHttpPort:  80,
			GatewayHttpsPort: 443,
		},
	}
	wrapper := &common.WrapperConfig{
		Config: &config.Config{
			Meta: config.Meta{
				Namespace: "default",
				Name:      "tls-passthrough",
			},
			Spec: ingress.IngressSpec{
				Rules: []ingress.IngressRule{
					{
						Host: "example.com",
						IngressRuleValue: ingress.IngressRuleValue{
							HTTP: &ingress.HTTPIngressRuleValue{
								Paths: []ingress.HTTPIngressPath{
									{
										Path: "/",
										Backend: ingress.IngressBackend{
											ServiceName: "app",
											ServicePort: intstr.FromInt(443),
										},
									},
								},
							},
						},
					},
				},
			},
		},
		AnnotationsConfig: &annotations.Ingress{
			SSLPassthrough: &annotations.SSLPassthroughConfig{Enabled: true},
		},
	}

	gatewayOptions := &common.ConvertOptions{
		Gateways:           map[string]*common.WrapperGateway{},
		IngressDomainCache: common.NewIngressDomainCache(),
	}
	if err := c.ConvertGateway(gatewayOptions, wrapper, nil); err != nil {
		t.Fatalf("ConvertGateway() error = %v", err)
	}
	gateway := gatewayOptions.Gateways["example.com"].Gateway
	if len(gateway.Servers) != 2 {
		t.Fatalf("server count mismatch, want 2, got %d", len(gateway.Servers))
	}
	tlsServer := gateway.Servers[1]
	if tlsServer.Port.Protocol != "TLS" {
		t.Fatalf("protocol mismatch, want TLS, got %s", tlsServer.Port.Protocol)
	}
	if tlsServer.Port.Number != 443 {
		t.Fatalf("port mismatch, want 443, got %d", tlsServer.Port.Number)
	}
	if tlsServer.Tls.GetMode() != v1alpha3.ServerTLSSettings_PASSTHROUGH {
		t.Fatalf("tls mode mismatch, want PASSTHROUGH, got %s", tlsServer.Tls.GetMode())
	}

	routeOptions := &common.ConvertOptions{}
	if err := c.ConvertHTTPRoute(routeOptions, wrapper); err != nil {
		t.Fatalf("ConvertHTTPRoute() error = %v", err)
	}
	httpRoutes := routeOptions.HTTPRoutes["example.com"]
	if len(httpRoutes) != 1 {
		t.Fatalf("http route count mismatch, want 1, got %d", len(httpRoutes))
	}
	if got := httpRoutes[0].HTTPRoute.Route[0].Destination.Host; got != "app.default.svc.cluster.local" {
		t.Fatalf("http destination host mismatch, got %s", got)
	}
	routes := routeOptions.VirtualServices["example.com"].VirtualService.Tls
	if len(routes) != 1 {
		t.Fatalf("tls route count mismatch, want 1, got %d", len(routes))
	}
	route := routes[0]
	if got := route.Match[0].SniHosts[0]; got != "example.com" {
		t.Fatalf("sni host mismatch, want example.com, got %s", got)
	}
	if got := route.Route[0].Destination.Host; got != "app.default.svc.cluster.local" {
		t.Fatalf("destination host mismatch, got %s", got)
	}
	if got := route.Route[0].Destination.Port.Number; got != 443 {
		t.Fatalf("destination port mismatch, got %d", got)
	}
}

func TestSSLPassthroughConvertTLSRouteRejectsNilInputs(t *testing.T) {
	c := controller{}
	wrapper := &common.WrapperConfig{
		Config:            &config.Config{},
		AnnotationsConfig: &annotations.Ingress{},
	}

	if err := c.ConvertTLSRoute(nil, wrapper); err == nil {
		t.Fatal("ConvertTLSRoute() with nil convertOptions returned nil error")
	}
	if err := c.ConvertTLSRoute(&common.ConvertOptions{}, nil); err == nil {
		t.Fatal("ConvertTLSRoute() with nil wrapper returned nil error")
	}
}

func TestSSLPassthroughUsesConfiguredHTTPSPort(t *testing.T) {
	c := controller{
		options: common.Options{
			GatewayHttpPort:  80,
			GatewayHttpsPort: 8443,
		},
	}
	wrapper := &common.WrapperConfig{
		Config: &config.Config{
			Meta: config.Meta{
				Namespace: "default",
				Name:      "tls-passthrough",
			},
			Spec: ingress.IngressSpec{
				Rules: []ingress.IngressRule{
					{
						Host: "example.com",
						IngressRuleValue: ingress.IngressRuleValue{
							HTTP: &ingress.HTTPIngressRuleValue{
								Paths: []ingress.HTTPIngressPath{
									{
										Path: "/",
										Backend: ingress.IngressBackend{
											ServiceName: "app",
											ServicePort: intstr.FromInt(443),
										},
									},
								},
							},
						},
					},
				},
			},
		},
		AnnotationsConfig: &annotations.Ingress{
			SSLPassthrough: &annotations.SSLPassthroughConfig{Enabled: true},
		},
	}

	gatewayOptions := &common.ConvertOptions{
		Gateways:           map[string]*common.WrapperGateway{},
		IngressDomainCache: common.NewIngressDomainCache(),
	}
	if err := c.ConvertGateway(gatewayOptions, wrapper, nil); err != nil {
		t.Fatalf("ConvertGateway() error = %v", err)
	}
	tlsServer := gatewayOptions.Gateways["example.com"].Gateway.Servers[1]
	if tlsServer.Port.Number != 8443 {
		t.Fatalf("port mismatch, want 8443, got %d", tlsServer.Port.Number)
	}
}

func TestSSLPassthroughCanaryIngressKeepsCanaryHandling(t *testing.T) {
	c := controller{}
	wrapper := &common.WrapperConfig{
		Config: &config.Config{
			Meta: config.Meta{
				Namespace: "default",
				Name:      "tls-passthrough-canary",
			},
			Spec: ingress.IngressSpec{
				Rules: []ingress.IngressRule{
					{
						Host: "example.com",
						IngressRuleValue: ingress.IngressRuleValue{
							HTTP: &ingress.HTTPIngressRuleValue{
								Paths: []ingress.HTTPIngressPath{
									{
										Path: "/",
										Backend: ingress.IngressBackend{
											ServiceName: "app-canary",
											ServicePort: intstr.FromInt(443),
										},
									},
								},
							},
						},
					},
				},
			},
		},
		AnnotationsConfig: &annotations.Ingress{
			Canary:         &annotations.CanaryConfig{Enabled: true},
			SSLPassthrough: &annotations.SSLPassthroughConfig{Enabled: true},
		},
	}

	routeOptions := &common.ConvertOptions{}
	if err := c.ConvertHTTPRoute(routeOptions, wrapper); err != nil {
		t.Fatalf("ConvertHTTPRoute() error = %v", err)
	}
	if len(routeOptions.CanaryIngresses) != 1 {
		t.Fatalf("canary ingress count mismatch, want 1, got %d", len(routeOptions.CanaryIngresses))
	}
	if len(routeOptions.VirtualServices) != 0 {
		t.Fatalf("unexpected virtual services: %+v", routeOptions.VirtualServices)
	}
}

func TestSSLPassthroughSkipsDuplicatedTLSHost(t *testing.T) {
	c := controller{
		options: common.Options{
			GatewayHttpPort:  80,
			GatewayHttpsPort: 443,
		},
	}
	primary := &common.WrapperConfig{
		Config: &config.Config{
			Meta: config.Meta{
				Namespace: "default",
				Name:      "tls-passthrough-primary",
			},
			Spec: ingress.IngressSpec{
				Rules: []ingress.IngressRule{
					{
						Host: "example.com",
						IngressRuleValue: ingress.IngressRuleValue{
							HTTP: &ingress.HTTPIngressRuleValue{
								Paths: []ingress.HTTPIngressPath{
									{
										Path: "/",
										Backend: ingress.IngressBackend{
											ServiceName: "primary",
											ServicePort: intstr.FromInt(443),
										},
									},
								},
							},
						},
					},
				},
			},
		},
		AnnotationsConfig: &annotations.Ingress{
			SSLPassthrough: &annotations.SSLPassthroughConfig{Enabled: true},
		},
	}
	duplicate := &common.WrapperConfig{
		Config: &config.Config{
			Meta: config.Meta{
				Namespace: "default",
				Name:      "tls-passthrough-duplicate",
			},
			Spec: ingress.IngressSpec{
				Rules: []ingress.IngressRule{
					{
						Host: "example.com",
						IngressRuleValue: ingress.IngressRuleValue{
							HTTP: &ingress.HTTPIngressRuleValue{
								Paths: []ingress.HTTPIngressPath{
									{
										Path: "/",
										Backend: ingress.IngressBackend{
											ServiceName: "duplicate",
											ServicePort: intstr.FromInt(443),
										},
									},
								},
							},
						},
					},
				},
			},
		},
		AnnotationsConfig: &annotations.Ingress{
			SSLPassthrough: &annotations.SSLPassthroughConfig{Enabled: true},
		},
	}

	options := &common.ConvertOptions{
		Gateways:                 map[string]*common.WrapperGateway{},
		IngressDomainCache:       common.NewIngressDomainCache(),
		PassthroughTLSHostOwners: map[string]*config.Config{"example.com": primary.Config},
	}
	if err := c.ConvertGateway(options, primary, nil); err != nil {
		t.Fatalf("ConvertGateway(primary) error = %v", err)
	}
	if err := c.ConvertGateway(options, duplicate, nil); err != nil {
		t.Fatalf("ConvertGateway(duplicate) error = %v", err)
	}
	options.VirtualServices = map[string]*common.WrapperVirtualService{}
	if err := c.ConvertTLSRoute(options, duplicate); err != nil {
		t.Fatalf("ConvertTLSRoute() error = %v", err)
	}
	if len(options.VirtualServices) != 0 {
		t.Fatalf("unexpected virtual services: %+v", options.VirtualServices)
	}
}

func TestSSLPassthroughDuplicateTLSHostUsesExistingGatewayOwner(t *testing.T) {
	c := controller{
		options: common.Options{
			GatewayHttpPort:  80,
			GatewayHttpsPort: 443,
		},
	}
	primary := &common.WrapperConfig{
		Config: &config.Config{
			Meta: config.Meta{
				Namespace: "default",
				Name:      "tls-primary",
			},
			Spec: ingress.IngressSpec{
				TLS: []ingress.IngressTLS{
					{Hosts: []string{"example.com"}},
				},
				Rules: []ingress.IngressRule{
					{
						Host: "example.com",
						IngressRuleValue: ingress.IngressRuleValue{
							HTTP: &ingress.HTTPIngressRuleValue{
								Paths: []ingress.HTTPIngressPath{
									{
										Path: "/",
										Backend: ingress.IngressBackend{
											ServiceName: "primary",
											ServicePort: intstr.FromInt(443),
										},
									},
								},
							},
						},
					},
				},
			},
		},
		AnnotationsConfig: &annotations.Ingress{},
	}
	duplicate := &common.WrapperConfig{
		Config: &config.Config{
			Meta: config.Meta{
				Namespace: "default",
				Name:      "tls-passthrough-duplicate",
			},
			Spec: ingress.IngressSpec{
				Rules: []ingress.IngressRule{
					{
						Host: "example.com",
						IngressRuleValue: ingress.IngressRuleValue{
							HTTP: &ingress.HTTPIngressRuleValue{
								Paths: []ingress.HTTPIngressPath{
									{
										Path: "/",
										Backend: ingress.IngressBackend{
											ServiceName: "duplicate",
											ServicePort: intstr.FromInt(443),
										},
									},
								},
							},
						},
					},
				},
			},
		},
		AnnotationsConfig: &annotations.Ingress{
			SSLPassthrough: &annotations.SSLPassthroughConfig{Enabled: true},
		},
	}
	httpsCredentialConfig := &cert.Config{
		CredentialConfig: []cert.CredentialEntry{
			{
				Domains:   []string{"example.com"},
				TLSSecret: "default/example-tls",
			},
		},
	}

	options := &common.ConvertOptions{
		Gateways:           map[string]*common.WrapperGateway{},
		IngressDomainCache: common.NewIngressDomainCache(),
	}
	if err := c.ConvertGateway(options, primary, httpsCredentialConfig); err != nil {
		t.Fatalf("ConvertGateway(primary) error = %v", err)
	}
	if err := c.ConvertGateway(options, duplicate, httpsCredentialConfig); err != nil {
		t.Fatalf("ConvertGateway(duplicate) error = %v", err)
	}

	if len(options.IngressDomainCache.Invalid) != 1 {
		t.Fatalf("invalid domain count mismatch, want 1, got %d", len(options.IngressDomainCache.Invalid))
	}
	invalid := options.IngressDomainCache.Invalid[0]
	if !strings.Contains(invalid.Error, "tls-primary") {
		t.Fatalf("invalid domain error does not reference existing gateway owner: %s", invalid.Error)
	}
}

func TestBackendToTLSRouteDestinationRejectsEmptyMCPDestination(t *testing.T) {
	c := controller{}
	backend := &ingress.IngressBackend{}
	config := &annotations.DestinationConfig{}

	destinations, event := c.backendToTLSRouteDestination(backend, "default", config)
	if event != common.InvalidBackendService {
		t.Fatalf("event mismatch, want InvalidBackendService, got %s", event)
	}
	if len(destinations) != 0 {
		t.Fatalf("destination count mismatch, want 0, got %d", len(destinations))
	}
}

func TestSSLPassthroughUsesFirstRootOwnerWhenLaterIngressEnablesPassthrough(t *testing.T) {
	c := controller{
		options: common.Options{
			GatewayHttpPort:  80,
			GatewayHttpsPort: 443,
		},
	}
	root := &common.WrapperConfig{
		Config: &config.Config{
			Meta: config.Meta{
				Namespace: "default",
				Name:      "root",
			},
			Spec: ingress.IngressSpec{
				Rules: []ingress.IngressRule{
					ingressV1Beta1Rule("example.com", ingressV1Beta1Path("/", "root", 443)),
				},
			},
		},
		AnnotationsConfig: &annotations.Ingress{},
	}
	passthrough := &common.WrapperConfig{
		Config: &config.Config{
			Meta: config.Meta{
				Namespace: "default",
				Name:      "passthrough",
			},
			Spec: ingress.IngressSpec{
				Rules: []ingress.IngressRule{
					ingressV1Beta1Rule("example.com", ingressV1Beta1Path("/passthrough", "passthrough", 443)),
				},
			},
		},
		AnnotationsConfig: &annotations.Ingress{
			SSLPassthrough: &annotations.SSLPassthroughConfig{Enabled: true},
		},
	}

	options := &common.ConvertOptions{
		Gateways:                 map[string]*common.WrapperGateway{},
		IngressDomainCache:       common.NewIngressDomainCache(),
		PassthroughTLSHostOwners: map[string]*config.Config{"example.com": root.Config},
	}
	if err := c.ConvertGateway(options, root, nil); err != nil {
		t.Fatalf("ConvertGateway(root) error = %v", err)
	}
	if err := c.ConvertGateway(options, passthrough, nil); err != nil {
		t.Fatalf("ConvertGateway(passthrough) error = %v", err)
	}
	gateway := options.Gateways["example.com"].Gateway
	if len(gateway.Servers) != 2 {
		t.Fatalf("server count mismatch, want 2, got %d", len(gateway.Servers))
	}
	if gateway.Servers[1].Tls.GetMode() != v1alpha3.ServerTLSSettings_PASSTHROUGH {
		t.Fatalf("tls mode mismatch, want PASSTHROUGH, got %s", gateway.Servers[1].Tls.GetMode())
	}

	routeOptions := &common.ConvertOptions{
		PassthroughTLSHostOwners: map[string]*config.Config{"example.com": root.Config},
	}
	if err := c.ConvertHTTPRoute(routeOptions, root); err != nil {
		t.Fatalf("ConvertHTTPRoute(root) error = %v", err)
	}
	if err := c.ConvertHTTPRoute(routeOptions, passthrough); err != nil {
		t.Fatalf("ConvertHTTPRoute(passthrough) error = %v", err)
	}
	routes := routeOptions.VirtualServices["example.com"].VirtualService.Tls
	if len(routes) != 1 {
		t.Fatalf("tls route count mismatch, want 1, got %d", len(routes))
	}
	if got := routes[0].Route[0].Destination.Host; got != "root.default.svc.cluster.local" {
		t.Fatalf("destination host mismatch, want root.default.svc.cluster.local, got %s", got)
	}
}

func TestSSLPassthroughNonRootIngressDoesNotBlockLaterRootIngress(t *testing.T) {
	c := controller{
		options: common.Options{
			GatewayHttpPort:  80,
			GatewayHttpsPort: 443,
		},
	}
	nonRoot := &common.WrapperConfig{
		Config: &config.Config{
			Meta: config.Meta{
				Namespace: "default",
				Name:      "tls-passthrough-non-root",
			},
			Spec: ingress.IngressSpec{
				Rules: []ingress.IngressRule{
					ingressV1Beta1Rule("example.com", ingressV1Beta1Path("/api", "api", 8443)),
				},
			},
		},
		AnnotationsConfig: &annotations.Ingress{
			SSLPassthrough: &annotations.SSLPassthroughConfig{Enabled: true},
		},
	}
	root := &common.WrapperConfig{
		Config: &config.Config{
			Meta: config.Meta{
				Namespace: "default",
				Name:      "tls-passthrough-root",
			},
			Spec: ingress.IngressSpec{
				Rules: []ingress.IngressRule{
					ingressV1Beta1Rule("example.com", ingressV1Beta1Path("/", "root", 443)),
				},
			},
		},
		AnnotationsConfig: &annotations.Ingress{
			SSLPassthrough: &annotations.SSLPassthroughConfig{Enabled: true},
		},
	}

	options := &common.ConvertOptions{
		Gateways:           map[string]*common.WrapperGateway{},
		IngressDomainCache: common.NewIngressDomainCache(),
	}
	if err := c.ConvertGateway(options, nonRoot, nil); err != nil {
		t.Fatalf("ConvertGateway(nonRoot) error = %v", err)
	}
	if len(options.Gateways["example.com"].Gateway.Servers) != 1 {
		t.Fatalf("non-root ingress server count mismatch, want 1, got %d", len(options.Gateways["example.com"].Gateway.Servers))
	}
	if err := c.ConvertGateway(options, root, nil); err != nil {
		t.Fatalf("ConvertGateway(root) error = %v", err)
	}
	if options.Gateways["example.com"].Gateway.Servers[1].Tls.GetMode() != v1alpha3.ServerTLSSettings_PASSTHROUGH {
		t.Fatal("root ingress did not create a TLS passthrough server")
	}

	options.VirtualServices = map[string]*common.WrapperVirtualService{}
	if err := c.ConvertTLSRoute(options, root); err != nil {
		t.Fatalf("ConvertTLSRoute(root) error = %v", err)
	}
	routes := options.VirtualServices["example.com"].VirtualService.Tls
	if len(routes) != 1 {
		t.Fatalf("tls route count mismatch, want 1, got %d", len(routes))
	}
	if got := routes[0].Route[0].Destination.Host; got != "root.default.svc.cluster.local" {
		t.Fatalf("destination host mismatch, got %s", got)
	}
}

func TestSSLPassthroughPreservesRepeatedHostInSameIngress(t *testing.T) {
	c := controller{
		options: common.Options{
			GatewayHttpPort:  80,
			GatewayHttpsPort: 443,
		},
	}
	wrapper := &common.WrapperConfig{
		Config: &config.Config{
			Meta: config.Meta{
				Namespace: "default",
				Name:      "tls-passthrough-repeated-host",
			},
			Spec: ingress.IngressSpec{
				Rules: []ingress.IngressRule{
					{
						Host: "example.com",
						IngressRuleValue: ingress.IngressRuleValue{
							HTTP: &ingress.HTTPIngressRuleValue{
								Paths: []ingress.HTTPIngressPath{
									{
										Path: "/health",
										Backend: ingress.IngressBackend{
											ServiceName: "health",
											ServicePort: intstr.FromInt(8443),
										},
									},
								},
							},
						},
					},
					{
						Host: "example.com",
						IngressRuleValue: ingress.IngressRuleValue{
							HTTP: &ingress.HTTPIngressRuleValue{
								Paths: []ingress.HTTPIngressPath{
									{
										Path: "/",
										Backend: ingress.IngressBackend{
											ServiceName: "root",
											ServicePort: intstr.FromInt(443),
										},
									},
								},
							},
						},
					},
				},
			},
		},
		AnnotationsConfig: &annotations.Ingress{
			SSLPassthrough: &annotations.SSLPassthroughConfig{Enabled: true},
		},
	}

	options := &common.ConvertOptions{
		Gateways:           map[string]*common.WrapperGateway{},
		IngressDomainCache: common.NewIngressDomainCache(),
	}
	if err := c.ConvertGateway(options, wrapper, nil); err != nil {
		t.Fatalf("ConvertGateway() error = %v", err)
	}
	options.VirtualServices = map[string]*common.WrapperVirtualService{}
	if err := c.ConvertTLSRoute(options, wrapper); err != nil {
		t.Fatalf("ConvertTLSRoute() error = %v", err)
	}
	routes := options.VirtualServices["example.com"].VirtualService.Tls
	if len(routes) != 1 {
		t.Fatalf("tls route count mismatch, want 1, got %d", len(routes))
	}
	if got := routes[0].Route[0].Destination.Host; got != "root.default.svc.cluster.local" {
		t.Fatalf("destination host mismatch, got %s", got)
	}
}

func TestSSLPassthroughUsesFirstRootBackendForRepeatedHost(t *testing.T) {
	c := controller{}
	wrapper := &common.WrapperConfig{
		Config: &config.Config{
			Meta: config.Meta{
				Namespace: "default",
				Name:      "tls-passthrough-repeated-root",
			},
			Spec: ingress.IngressSpec{
				Rules: []ingress.IngressRule{
					{
						Host: "example.com",
						IngressRuleValue: ingress.IngressRuleValue{
							HTTP: &ingress.HTTPIngressRuleValue{
								Paths: []ingress.HTTPIngressPath{
									{
										Path: "/",
										Backend: ingress.IngressBackend{
											ServiceName: "first",
											ServicePort: intstr.FromInt(443),
										},
									},
								},
							},
						},
					},
					{
						Host: "example.com",
						IngressRuleValue: ingress.IngressRuleValue{
							HTTP: &ingress.HTTPIngressRuleValue{
								Paths: []ingress.HTTPIngressPath{
									{
										Path: "/",
										Backend: ingress.IngressBackend{
											ServiceName: "second",
											ServicePort: intstr.FromInt(443),
										},
									},
								},
							},
						},
					},
				},
			},
		},
		AnnotationsConfig: &annotations.Ingress{
			SSLPassthrough: &annotations.SSLPassthroughConfig{Enabled: true},
		},
	}

	routeOptions := &common.ConvertOptions{}
	if err := c.ConvertHTTPRoute(routeOptions, wrapper); err != nil {
		t.Fatalf("ConvertHTTPRoute() error = %v", err)
	}
	routes := routeOptions.VirtualServices["example.com"].VirtualService.Tls
	if len(routes) != 1 {
		t.Fatalf("tls route count mismatch, want 1, got %d", len(routes))
	}
	if got := routes[0].Route[0].Destination.Host; got != "first.default.svc.cluster.local" {
		t.Fatalf("destination host mismatch, got %s", got)
	}
}

func TestSSLPassthroughHandlesMultipleHosts(t *testing.T) {
	c := controller{}
	testcases := []struct {
		name       string
		rules      []ingress.IngressRule
		wantHosts  []string
		wantRoutes map[string]string
	}{
		{
			name: "root path first",
			rules: []ingress.IngressRule{
				ingressV1Beta1Rule("first.example.com", ingressV1Beta1Path("/", "first", 443)),
				ingressV1Beta1Rule("middle.example.com", ingressV1Beta1Path("/health", "middle", 8443)),
				ingressV1Beta1Rule("last.example.com", ingressV1Beta1Path("/health", "last", 8443)),
			},
			wantHosts: []string{"first.example.com"},
			wantRoutes: map[string]string{
				"first.example.com": "first.default.svc.cluster.local",
			},
		},
		{
			name: "root path middle",
			rules: []ingress.IngressRule{
				ingressV1Beta1Rule("first.example.com", ingressV1Beta1Path("/health", "first", 8443)),
				ingressV1Beta1Rule("middle.example.com", ingressV1Beta1Path("/", "middle", 443)),
				ingressV1Beta1Rule("last.example.com", ingressV1Beta1Path("/health", "last", 8443)),
			},
			wantHosts: []string{"middle.example.com"},
			wantRoutes: map[string]string{
				"middle.example.com": "middle.default.svc.cluster.local",
			},
		},
		{
			name: "root path last",
			rules: []ingress.IngressRule{
				ingressV1Beta1Rule("first.example.com", ingressV1Beta1Path("/health", "first", 8443)),
				ingressV1Beta1Rule("middle.example.com", ingressV1Beta1Path("/health", "middle", 8443)),
				ingressV1Beta1Rule("last.example.com", ingressV1Beta1Path("/", "last", 443)),
			},
			wantHosts: []string{"last.example.com"},
			wantRoutes: map[string]string{
				"last.example.com": "last.default.svc.cluster.local",
			},
		},
		{
			name: "multiple root hosts",
			rules: []ingress.IngressRule{
				ingressV1Beta1Rule("first.example.com", ingressV1Beta1Path("/", "first", 443)),
				ingressV1Beta1Rule("middle.example.com", ingressV1Beta1Path("/", "middle", 443)),
				ingressV1Beta1Rule("last.example.com", ingressV1Beta1Path("/", "last", 443)),
			},
			wantHosts: []string{"first.example.com", "middle.example.com", "last.example.com"},
			wantRoutes: map[string]string{
				"first.example.com":  "first.default.svc.cluster.local",
				"middle.example.com": "middle.default.svc.cluster.local",
				"last.example.com":   "last.default.svc.cluster.local",
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			wrapper := &common.WrapperConfig{
				Config: &config.Config{
					Meta: config.Meta{
						Namespace: "default",
						Name:      "tls-passthrough-multi-host",
					},
					Spec: ingress.IngressSpec{
						Rules: tc.rules,
					},
				},
				AnnotationsConfig: &annotations.Ingress{
					SSLPassthrough: &annotations.SSLPassthroughConfig{Enabled: true},
				},
			}

			routeOptions := &common.ConvertOptions{}
			if err := c.ConvertHTTPRoute(routeOptions, wrapper); err != nil {
				t.Fatalf("ConvertHTTPRoute() error = %v", err)
			}
			for _, host := range tc.wantHosts {
				routes := routeOptions.VirtualServices[host].VirtualService.Tls
				if len(routes) != 1 {
					t.Fatalf("tls route count mismatch for host %s, want 1, got %d", host, len(routes))
				}
				if got := routes[0].Route[0].Destination.Host; got != tc.wantRoutes[host] {
					t.Fatalf("destination host mismatch for host %s, want %s, got %s", host, tc.wantRoutes[host], got)
				}
			}
		})
	}
}

func ingressV1Beta1Path(path, service string, port int32) ingress.HTTPIngressPath {
	return ingress.HTTPIngressPath{
		Path: path,
		Backend: ingress.IngressBackend{
			ServiceName: service,
			ServicePort: intstr.FromInt(int(port)),
		},
	}
}

func ingressV1Beta1Rule(host string, paths ...ingress.HTTPIngressPath) ingress.IngressRule {
	return ingress.IngressRule{
		Host: host,
		IngressRuleValue: ingress.IngressRuleValue{
			HTTP: &ingress.HTTPIngressRuleValue{
				Paths: paths,
			},
		},
	}
}

func TestSSLPassthroughUsesRootPathBackend(t *testing.T) {
	c := controller{}
	wrapper := &common.WrapperConfig{
		Config: &config.Config{
			Meta: config.Meta{
				Namespace: "default",
				Name:      "tls-passthrough-root",
			},
			Spec: ingress.IngressSpec{
				Rules: []ingress.IngressRule{
					{
						Host: "example.com",
						IngressRuleValue: ingress.IngressRuleValue{
							HTTP: &ingress.HTTPIngressRuleValue{
								Paths: []ingress.HTTPIngressPath{
									{
										Path: "/api",
										Backend: ingress.IngressBackend{
											ServiceName: "api",
											ServicePort: intstr.FromInt(8443),
										},
									},
									{
										Path: "/",
										Backend: ingress.IngressBackend{
											ServiceName: "root",
											ServicePort: intstr.FromInt(443),
										},
									},
								},
							},
						},
					},
				},
			},
		},
		AnnotationsConfig: &annotations.Ingress{
			SSLPassthrough: &annotations.SSLPassthroughConfig{Enabled: true},
		},
	}

	routeOptions := &common.ConvertOptions{}
	if err := c.ConvertHTTPRoute(routeOptions, wrapper); err != nil {
		t.Fatalf("ConvertHTTPRoute() error = %v", err)
	}
	routes := routeOptions.VirtualServices["example.com"].VirtualService.Tls
	if len(routes) != 1 {
		t.Fatalf("tls route count mismatch, want 1, got %d", len(routes))
	}
	if got := routes[0].Route[0].Destination.Host; got != "root.default.svc.cluster.local" {
		t.Fatalf("destination host mismatch, got %s", got)
	}
}

func TestSSLPassthroughWildcardHostKeepsVirtualServiceConsistent(t *testing.T) {
	c := controller{}
	wrapper := &common.WrapperConfig{
		Config: &config.Config{
			Meta: config.Meta{
				Namespace: "default",
				Name:      "tls-passthrough-wildcard",
			},
			Spec: ingress.IngressSpec{
				Rules: []ingress.IngressRule{
					{
						IngressRuleValue: ingress.IngressRuleValue{
							HTTP: &ingress.HTTPIngressRuleValue{
								Paths: []ingress.HTTPIngressPath{
									{
										Path: "/",
										Backend: ingress.IngressBackend{
											ServiceName: "root",
											ServicePort: intstr.FromInt(443),
										},
									},
								},
							},
						},
					},
				},
			},
		},
		AnnotationsConfig: &annotations.Ingress{
			SSLPassthrough: &annotations.SSLPassthroughConfig{Enabled: true},
		},
	}

	routeOptions := &common.ConvertOptions{}
	if err := c.ConvertHTTPRoute(routeOptions, wrapper); err != nil {
		t.Fatalf("ConvertHTTPRoute() error = %v", err)
	}
	if err := c.ConvertTLSRoute(routeOptions, wrapper); err != nil {
		t.Fatalf("ConvertTLSRoute() error = %v", err)
	}

	vs := routeOptions.VirtualServices[""].VirtualService
	if got := vs.Hosts; len(got) != 1 || got[0] != "*" {
		t.Fatalf("virtual service hosts mismatch, got %+v", got)
	}
	if len(vs.Tls) != 1 {
		t.Fatalf("tls route count mismatch, want 1, got %d", len(vs.Tls))
	}
	if got := vs.Tls[0].Match[0].SniHosts; len(got) != 1 || got[0] != "*" {
		t.Fatalf("sni hosts mismatch, got %+v", got)
	}
}

func TestSSLPassthroughIgnoresNonRootPath(t *testing.T) {
	c := controller{}
	wrapper := &common.WrapperConfig{
		Config: &config.Config{
			Meta: config.Meta{
				Namespace: "default",
				Name:      "tls-passthrough-non-root",
			},
			Spec: ingress.IngressSpec{
				Rules: []ingress.IngressRule{
					{
						Host: "example.com",
						IngressRuleValue: ingress.IngressRuleValue{
							HTTP: &ingress.HTTPIngressRuleValue{
								Paths: []ingress.HTTPIngressPath{
									{
										Path: "/api",
										Backend: ingress.IngressBackend{
											ServiceName: "api",
											ServicePort: intstr.FromInt(8443),
										},
									},
								},
							},
						},
					},
				},
			},
		},
		AnnotationsConfig: &annotations.Ingress{
			SSLPassthrough: &annotations.SSLPassthroughConfig{Enabled: true},
		},
	}

	routeOptions := &common.ConvertOptions{}
	if err := c.ConvertHTTPRoute(routeOptions, wrapper); err != nil {
		t.Fatalf("ConvertHTTPRoute() error = %v", err)
	}
	if len(routeOptions.HTTPRoutes["example.com"]) != 1 {
		t.Fatalf("http route count mismatch, want 1, got %d", len(routeOptions.HTTPRoutes["example.com"]))
	}
	if routes := routeOptions.VirtualServices["example.com"].VirtualService.Tls; len(routes) != 0 {
		t.Fatalf("unexpected tls routes: %+v", routes)
	}
}

func testApplyCanaryIngress(t *testing.T, c common.IngressController) {
	testcases := []struct {
		description string
		input       struct {
			options       *common.ConvertOptions
			wrapperConfig *common.WrapperConfig
		}
		expectNoError bool
	}{
		{
			description: "convertOptions is nil",
			input: struct {
				options       *common.ConvertOptions
				wrapperConfig *common.WrapperConfig
			}{
				options:       nil,
				wrapperConfig: nil,
			},
			expectNoError: false,
		},
		{
			description: "convertOptions is not nil but empty",
			input: struct {
				options       *common.ConvertOptions
				wrapperConfig *common.WrapperConfig
			}{
				options: &common.ConvertOptions{},
				wrapperConfig: &common.WrapperConfig{
					Config:            &config.Config{},
					AnnotationsConfig: &annotations.Ingress{},
				},
			},
			expectNoError: false,
		},
		{
			description: "valid canary ingress",
			input: struct {
				options       *common.ConvertOptions
				wrapperConfig *common.WrapperConfig
			}{
				options: &common.ConvertOptions{
					IngressDomainCache: &common.IngressDomainCache{
						Valid:   make(map[string]*common.IngressDomainBuilder),
						Invalid: make([]model.IngressDomain, 0),
					},
					Route2Ingress:     map[string]*common.WrapperConfigWithRuleKey{},
					VirtualServices:   make(map[string]*common.WrapperVirtualService),
					Gateways:          make(map[string]*common.WrapperGateway),
					IngressRouteCache: &common.IngressRouteCache{},
					HTTPRoutes: map[string][]*common.WrapperHTTPRoute{
						"test1": make([]*common.WrapperHTTPRoute, 0),
					},
				},
				wrapperConfig: &common.WrapperConfig{Config: &config.Config{
					Spec: ingress.IngressSpec{
						Rules: []ingress.IngressRule{
							{
								Host: "test1",
								IngressRuleValue: ingress.IngressRuleValue{
									HTTP: &ingress.HTTPIngressRuleValue{
										Paths: []ingress.HTTPIngressPath{
											{
												Path:     "/test",
												PathType: &defaultPathType,
											},
										},
									},
								},
							},
						},
						Backend: &ingress.IngressBackend{},
						TLS: []ingress.IngressTLS{
							{
								Hosts:      []string{"test1", "test2"},
								SecretName: "test",
							},
						},
					},
				}, AnnotationsConfig: &annotations.Ingress{}},
			},
			expectNoError: true,
		},
	}

	for _, testcase := range testcases {
		err := c.ApplyCanaryIngress(testcase.input.options, testcase.input.wrapperConfig)
		if err != nil {
			require.Equal(t, testcase.expectNoError, false)
		} else {
			require.Equal(t, testcase.expectNoError, true)
		}
	}
}

func testApplyDefaultBackend(t *testing.T, c common.IngressController) {
	testcases := []struct {
		description string
		input       struct {
			options       *common.ConvertOptions
			wrapperConfig *common.WrapperConfig
		}
		expectNoError bool
	}{
		{
			description: "convertOptions is nil",
			input: struct {
				options       *common.ConvertOptions
				wrapperConfig *common.WrapperConfig
			}{
				options:       nil,
				wrapperConfig: nil,
			},
			expectNoError: false,
		}, {
			description: "convertOptions is not nil but empty",
			input: struct {
				options       *common.ConvertOptions
				wrapperConfig *common.WrapperConfig
			}{
				options: &common.ConvertOptions{},
				wrapperConfig: &common.WrapperConfig{
					Config:            &config.Config{},
					AnnotationsConfig: &annotations.Ingress{},
				},
			},
			expectNoError: false,
		}, {
			description: "valid default backend",
			input: struct {
				options       *common.ConvertOptions
				wrapperConfig *common.WrapperConfig
			}{
				options: &common.ConvertOptions{
					IngressDomainCache: &common.IngressDomainCache{
						Valid:   make(map[string]*common.IngressDomainBuilder),
						Invalid: make([]model.IngressDomain, 0),
					},
					Route2Ingress:     map[string]*common.WrapperConfigWithRuleKey{},
					VirtualServices:   make(map[string]*common.WrapperVirtualService),
					Gateways:          make(map[string]*common.WrapperGateway),
					IngressRouteCache: &common.IngressRouteCache{},
					HTTPRoutes:        make(map[string][]*common.WrapperHTTPRoute),
				},
				wrapperConfig: &common.WrapperConfig{Config: &config.Config{
					Spec: ingress.IngressSpec{
						Rules: []ingress.IngressRule{
							{
								Host: "test1",
								IngressRuleValue: ingress.IngressRuleValue{
									HTTP: &ingress.HTTPIngressRuleValue{
										Paths: []ingress.HTTPIngressPath{
											{
												Path:     "/test",
												PathType: &defaultPathType,
											},
										},
									},
								},
							},
						},
						Backend: &ingress.IngressBackend{},
						TLS: []ingress.IngressTLS{
							{
								Hosts:      []string{"test1", "test2"},
								SecretName: "test",
							},
						},
					},
				}, AnnotationsConfig: &annotations.Ingress{}},
			},
			expectNoError: true,
		},
	}

	for _, testcase := range testcases {
		err := c.ApplyDefaultBackend(testcase.input.options, testcase.input.wrapperConfig)
		if err != nil {
			require.Equal(t, testcase.expectNoError, false)
		} else {
			require.Equal(t, testcase.expectNoError, true)
		}
	}
}

func TestIngressControllerConventions(t *testing.T) {
	fakeClient := kube.NewFakeClient()
	localKubeClient, client := fakeClient, fakeClient

	options := common.Options{IngressClass: "mse", ClusterId: "", EnableStatus: true}

	secretController := secret.NewController(localKubeClient, options)
	ingressController := NewController(localKubeClient, client, options, secretController)

	testcases := map[string]func(*testing.T, common.IngressController){
		"test convert Gateway":       testConvertGateway,
		"test convert HTTPRoute":     testConvertHTTPRoute,
		"test convert TrafficPolicy": testConvertTrafficPolicy,
	}
	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			tc(t, ingressController)
		})
	}
}

func testConvertGateway(t *testing.T, c common.IngressController) {
	testcases := []struct {
		description string
		input       struct {
			options       *common.ConvertOptions
			wrapperConfig *common.WrapperConfig
		}
		expectNoError bool
	}{
		{
			description: "convertOptions is nil",
			input: struct {
				options       *common.ConvertOptions
				wrapperConfig *common.WrapperConfig
			}{
				options:       nil,
				wrapperConfig: nil,
			},
			expectNoError: false,
		}, {
			description: "convertOptions is not nil but empty",
			input: struct {
				options       *common.ConvertOptions
				wrapperConfig *common.WrapperConfig
			}{
				options: &common.ConvertOptions{},
				wrapperConfig: &common.WrapperConfig{
					Config:            &config.Config{},
					AnnotationsConfig: &annotations.Ingress{},
				},
			},
			expectNoError: false,
		}, {
			description: "valid gateway convention",
			input: struct {
				options       *common.ConvertOptions
				wrapperConfig *common.WrapperConfig
			}{
				options: &common.ConvertOptions{
					IngressDomainCache: &common.IngressDomainCache{
						Valid:   make(map[string]*common.IngressDomainBuilder),
						Invalid: make([]model.IngressDomain, 0),
					},
					Gateways: make(map[string]*common.WrapperGateway),
				},
				wrapperConfig: &common.WrapperConfig{Config: &config.Config{
					Spec: ingress.IngressSpec{
						Rules: []ingress.IngressRule{
							{
								Host: "test1",
								IngressRuleValue: ingress.IngressRuleValue{
									HTTP: &ingress.HTTPIngressRuleValue{
										Paths: []ingress.HTTPIngressPath{
											{
												Path: "/test",
											},
										},
									},
								},
							},
						},
						Backend: &ingress.IngressBackend{},
						TLS: []ingress.IngressTLS{
							{
								Hosts:      []string{"test1", "test2"},
								SecretName: "test",
							},
						},
					},
				}, AnnotationsConfig: &annotations.Ingress{}},
			},
			expectNoError: true,
		},
	}

	for _, testcase := range testcases {
		err := c.ConvertGateway(testcase.input.options, testcase.input.wrapperConfig, nil)
		if err != nil {
			require.Equal(t, testcase.expectNoError, false)
		} else {
			require.Equal(t, testcase.expectNoError, true)
		}
	}
}

func testConvertHTTPRoute(t *testing.T, c common.IngressController) {
	testcases := []struct {
		description string
		input       struct {
			options       *common.ConvertOptions
			wrapperConfig *common.WrapperConfig
		}
		expectNoError bool
	}{
		{
			description: "convertOptions is nil",
			input: struct {
				options       *common.ConvertOptions
				wrapperConfig *common.WrapperConfig
			}{
				options:       nil,
				wrapperConfig: nil,
			},
			expectNoError: false,
		}, {
			description: "convertOptions is not nil but empty",
			input: struct {
				options       *common.ConvertOptions
				wrapperConfig *common.WrapperConfig
			}{
				options: &common.ConvertOptions{},
				wrapperConfig: &common.WrapperConfig{
					Config:            &config.Config{},
					AnnotationsConfig: &annotations.Ingress{},
				},
			},
			expectNoError: false,
		}, {
			description: "valid httpRoute convention",
			input: struct {
				options       *common.ConvertOptions
				wrapperConfig *common.WrapperConfig
			}{
				options: &common.ConvertOptions{
					IngressDomainCache: &common.IngressDomainCache{
						Valid:   make(map[string]*common.IngressDomainBuilder),
						Invalid: make([]model.IngressDomain, 0),
					},
					Route2Ingress:     map[string]*common.WrapperConfigWithRuleKey{},
					VirtualServices:   make(map[string]*common.WrapperVirtualService),
					Gateways:          make(map[string]*common.WrapperGateway),
					IngressRouteCache: &common.IngressRouteCache{},
					HTTPRoutes:        make(map[string][]*common.WrapperHTTPRoute),
				},
				wrapperConfig: &common.WrapperConfig{
					Config: &config.Config{
						Spec: ingress.IngressSpec{
							Rules: []ingress.IngressRule{
								{
									Host: "test1",
									IngressRuleValue: ingress.IngressRuleValue{
										HTTP: &ingress.HTTPIngressRuleValue{
											Paths: []ingress.HTTPIngressPath{
												{
													Path:     "/test",
													PathType: &defaultPathType,
												},
											},
										},
									},
								},
							},
							Backend: &ingress.IngressBackend{},
							TLS: []ingress.IngressTLS{
								{
									Hosts:      []string{"test1", "test2"},
									SecretName: "test",
								},
							},
						},
					}, AnnotationsConfig: &annotations.Ingress{},
				},
			},
			expectNoError: true,
		},
	}

	for _, testcase := range testcases {
		err := c.ConvertHTTPRoute(testcase.input.options, testcase.input.wrapperConfig)
		if err != nil {
			require.Equal(t, testcase.expectNoError, false)
		} else {
			require.Equal(t, testcase.expectNoError, true)
		}
	}
}

func testConvertTrafficPolicy(t *testing.T, c common.IngressController) {
	testcases := []struct {
		description string
		input       struct {
			options       *common.ConvertOptions
			wrapperConfig *common.WrapperConfig
		}
		expectNoError bool
	}{
		{
			description: "convertOptions is nil",
			input: struct {
				options       *common.ConvertOptions
				wrapperConfig *common.WrapperConfig
			}{
				options:       nil,
				wrapperConfig: nil,
			},
			expectNoError: false,
		}, {
			description: "convertOptions is not nil but empty",
			input: struct {
				options       *common.ConvertOptions
				wrapperConfig *common.WrapperConfig
			}{
				options: &common.ConvertOptions{},
				wrapperConfig: &common.WrapperConfig{
					Config:            &config.Config{},
					AnnotationsConfig: &annotations.Ingress{},
				},
			},
			expectNoError: true,
		}, {
			description: "valid trafficPolicy convention",
			input: struct {
				options       *common.ConvertOptions
				wrapperConfig *common.WrapperConfig
			}{
				options: &common.ConvertOptions{
					IngressDomainCache: &common.IngressDomainCache{
						Valid:   make(map[string]*common.IngressDomainBuilder),
						Invalid: make([]model.IngressDomain, 0),
					},
					Route2Ingress:         map[string]*common.WrapperConfigWithRuleKey{},
					VirtualServices:       make(map[string]*common.WrapperVirtualService),
					Gateways:              make(map[string]*common.WrapperGateway),
					IngressRouteCache:     &common.IngressRouteCache{},
					Service2TrafficPolicy: make(map[common.ServiceKey]*common.WrapperTrafficPolicy),
					HTTPRoutes:            make(map[string][]*common.WrapperHTTPRoute),
				},
				wrapperConfig: &common.WrapperConfig{Config: &config.Config{
					Spec: ingress.IngressSpec{
						Rules: []ingress.IngressRule{
							{
								Host: "test1",
								IngressRuleValue: ingress.IngressRuleValue{
									HTTP: &ingress.HTTPIngressRuleValue{
										Paths: []ingress.HTTPIngressPath{
											{
												Path:     "/test",
												PathType: &defaultPathType,
												Backend: ingress.IngressBackend{
													ServiceName: "test",
													ServicePort: intstr.FromInt(8080),
												},
											},
										},
									},
								},
							},
						},
						Backend: &ingress.IngressBackend{
							ServiceName: "test",
						},
						TLS: []ingress.IngressTLS{
							{
								Hosts:      []string{"test1", "test2"},
								SecretName: "test",
							},
						},
					},
				}, AnnotationsConfig: &annotations.Ingress{
					LoadBalance: &annotations.LoadBalanceConfig{},
				}},
			},
			expectNoError: true,
		},
	}

	for _, testcase := range testcases {
		err := c.ConvertTrafficPolicy(testcase.input.options, testcase.input.wrapperConfig)
		if err != nil {
			require.Equal(t, testcase.expectNoError, false)
		} else {
			require.Equal(t, testcase.expectNoError, true)
		}
	}
}

func TestIngressControllerGenerations(t *testing.T) {
	c := &controller{
		options: common.Options{
			IngressClass:    "mse",
			SystemNamespace: "higress-system",
		},
		ingresses: make(map[string]*v1beta1.Ingress),
	}

	testcases := map[string]func(*testing.T, *controller){
		"test create DefaultRoute":         testcreateDefaultRoute,
		"test create ServiceKey":           testcreateServiceKey,
		"test backend to RouteDestination": testbackendToRouteDestination,
	}
	for name, tc := range testcases {
		t.Run(name, func(t *testing.T) {
			tc(t, c)
		})
	}
}

func testcreateDefaultRoute(t *testing.T, c *controller) {
	testcases := []struct {
		input struct {
			wrapper *common.WrapperConfig
			backend *ingress.IngressBackend
			host    string
		}
		description string
		expect      *common.WrapperHTTPRoute
	}{
		{
			input: struct {
				wrapper *common.WrapperConfig
				backend *ingress.IngressBackend
				host    string
			}{
				wrapper: nil,
				backend: nil,
				host:    "",
			},
			description: "wrapperConfig is nil",
			expect:      nil,
		},
		{
			input: struct {
				wrapper *common.WrapperConfig
				backend *ingress.IngressBackend
				host    string
			}{
				wrapper: &common.WrapperConfig{},
				backend: &ingress.IngressBackend{},
				host:    "test",
			},
			description: "wrapperConfig is not nil but empty",
			expect:      nil,
		},
		{
			input: struct {
				wrapper *common.WrapperConfig
				backend *ingress.IngressBackend
				host    string
			}{
				wrapper: &common.WrapperConfig{
					Config: &config.Config{
						Meta: config.Meta{
							Namespace: "higress-system",
							Name:      "test",
						},
					},
					AnnotationsConfig: &annotations.Ingress{},
				},
				backend: &ingress.IngressBackend{
					ServiceName: "test",
					ServicePort: intstr.FromInt(8088),
				},
				host: "test",
			},
			description: "create expected httpRoute",
			expect: &common.WrapperHTTPRoute{
				WrapperConfig: &common.WrapperConfig{
					Config: &config.Config{
						Meta: config.Meta{
							Name:      "test",
							Namespace: "higress-system",
						},
					},
					AnnotationsConfig: &annotations.Ingress{},
				},
				RawClusterId:     "",
				ClusterId:        "",
				ClusterName:      "",
				Host:             "test",
				OriginPath:       "/",
				OriginPathType:   "prefix",
				WeightTotal:      0,
				IsDefaultBackend: true,
				HTTPRoute: &v1alpha3.HTTPRoute{
					Name: "test-default",
					Route: []*v1alpha3.HTTPRouteDestination{
						{
							Weight: 100,
							Destination: &v1alpha3.Destination{
								Port: &v1alpha3.PortSelector{
									Number: 8088,
								},
								Host: "test.higress-system.svc.cluster.local",
							},
						},
					},
				},
			},
		},
	}

	for _, testcase := range testcases {
		httpRoute := c.createDefaultRoute(testcase.input.wrapper, testcase.input.backend, testcase.input.host)
		require.Equal(t, testcase.expect, httpRoute)
	}
}

func testcreateServiceKey(t *testing.T, c *controller) {
	testcases := []struct {
		input struct {
			backend   *ingress.IngressBackend
			namespace string
		}
		expectNoError bool
		description   string
	}{
		{
			description:   "nil",
			expectNoError: false,
			input: struct {
				backend   *ingress.IngressBackend
				namespace string
			}{
				backend:   nil,
				namespace: "",
			},
		},
		{
			description:   "nil",
			expectNoError: false,
			input: struct {
				backend   *ingress.IngressBackend
				namespace string
			}{
				backend:   &ingress.IngressBackend{},
				namespace: "",
			},
		},
		{
			description:   "create success",
			expectNoError: true,
			input: struct {
				backend   *ingress.IngressBackend
				namespace string
			}{
				backend: &ingress.IngressBackend{
					ServiceName: "test",
					ServicePort: intstr.FromInt(8080),
				},
				namespace: "default",
			},
		},
	}

	for _, testcase := range testcases {
		_, err := c.createServiceKey(testcase.input.backend, testcase.input.namespace)
		if err != nil {
			require.Equal(t, testcase.expectNoError, false)
		} else {
			require.Equal(t, testcase.expectNoError, true)
		}
	}
}

func testbackendToRouteDestination(t *testing.T, c *controller) {
	testcases := []struct {
		input struct {
			backend   *ingress.IngressBackend
			namespace string
			builder   *common.IngressRouteBuilder
			config    *annotations.DestinationConfig
		}
		expectNoError bool
		description   string
	}{
		{
			description:   "nil",
			expectNoError: false,
			input: struct {
				backend   *ingress.IngressBackend
				namespace string
				builder   *common.IngressRouteBuilder
				config    *annotations.DestinationConfig
			}{
				backend:   nil,
				namespace: "",
				builder:   nil,
				config:    nil,
			},
		},
		{
			description:   "nil",
			expectNoError: false,
			input: struct {
				backend   *ingress.IngressBackend
				namespace string
				builder   *common.IngressRouteBuilder
				config    *annotations.DestinationConfig
			}{
				backend:   &ingress.IngressBackend{ServiceName: ""},
				namespace: "",
				builder:   nil,
				config:    nil,
			},
		},
		{
			description:   "create success",
			expectNoError: true,
			input: struct {
				backend   *ingress.IngressBackend
				namespace string
				builder   *common.IngressRouteBuilder
				config    *annotations.DestinationConfig
			}{
				backend: &ingress.IngressBackend{
					ServiceName: "test",
					ServicePort: intstr.FromInt(8080),
				},
				namespace: "default",
				builder:   &common.IngressRouteBuilder{},
				config:    nil,
			},
		},
	}

	for _, testcase := range testcases {
		_, err := c.backendToRouteDestination(
			testcase.input.backend,
			testcase.input.namespace,
			testcase.input.builder,
			testcase.input.config,
		)

		if err == common.InvalidBackendService {
			require.Equal(t, testcase.expectNoError, false)
		} else {
			require.Equal(t, testcase.expectNoError, true)
		}
	}
}

func TestIsCanaryRoute(t *testing.T) {
	testcases := []struct {
		input struct {
			canary *common.WrapperHTTPRoute
			route  *common.WrapperHTTPRoute
		}
		expect      bool
		description string
	}{
		{
			input: struct {
				canary *common.WrapperHTTPRoute
				route  *common.WrapperHTTPRoute
			}{
				canary: nil,
				route:  nil,
			},
			expect:      false,
			description: "both are nil",
		}, {
			input: struct {
				canary *common.WrapperHTTPRoute
				route  *common.WrapperHTTPRoute
			}{
				canary: &common.WrapperHTTPRoute{
					OriginPathType: common.Exact,
					OriginPath:     "/test",
				},
				route: &common.WrapperHTTPRoute{
					WrapperConfig: &common.WrapperConfig{
						AnnotationsConfig: &annotations.Ingress{
							Canary: nil,
						},
					},
					OriginPathType: common.Exact,
					OriginPath:     "/test",
				},
			},
			expect:      true,
			description: "canary is nil",
		}, {
			input: struct {
				canary *common.WrapperHTTPRoute
				route  *common.WrapperHTTPRoute
			}{
				canary: &common.WrapperHTTPRoute{
					OriginPathType: common.Exact,
					OriginPath:     "/test",
				},
				route: &common.WrapperHTTPRoute{
					WrapperConfig: &common.WrapperConfig{
						AnnotationsConfig: &annotations.Ingress{
							Canary: &annotations.CanaryConfig{
								Enabled: true,
							},
						},
					},
					OriginPathType: common.Exact,
					OriginPath:     "/test",
				},
			},
			expect:      false,
			description: "canary is not nil",
		},
	}
	for _, testcase := range testcases {
		actual := isCanaryRoute(testcase.input.canary, testcase.input.route)
		require.Equal(t, testcase.expect, actual)
	}
}

func TestExtractTLSSecretName(t *testing.T) {
	testcases := []struct {
		input struct {
			host string
			tls  []ingress.IngressTLS
		}
		expect      string
		description string
	}{
		{
			input: struct {
				host string
				tls  []ingress.IngressTLS
			}{
				host: "",
				tls:  nil,
			},
			expect:      "",
			description: "both are nil",
		},
		{
			input: struct {
				host string
				tls  []ingress.IngressTLS
			}{
				host: "test",
				tls: []ingress.IngressTLS{
					{
						Hosts:      []string{"test"},
						SecretName: "test-secret",
					},
					{
						Hosts:      []string{"test1"},
						SecretName: "test1-secret",
					},
				},
			},
			expect:      "test-secret",
			description: "found secret name",
		},
	}

	for _, testcase := range testcases {
		actual := extractTLSSecretName(testcase.input.host, testcase.input.tls)
		require.Equal(t, testcase.expect, actual)
	}
}

func TestSetDefaultMSEIngressOptionalField(t *testing.T) {
	pathType := ingress.PathTypeImplementationSpecific
	testcases := []struct {
		input struct {
			ing *ingress.Ingress
		}
		expect      *ingress.Ingress
		description string
	}{
		{
			input: struct{ ing *ingress.Ingress }{
				ing: nil,
			},
			expect:      nil,
			description: "nil",
		},
		{
			input: struct{ ing *ingress.Ingress }{
				ing: &ingress.Ingress{},
			},
			expect:      &ingress.Ingress{},
			description: "nil",
		},
		{
			input: struct{ ing *ingress.Ingress }{
				ing: &ingress.Ingress{
					Spec: ingress.IngressSpec{
						TLS: []ingress.IngressTLS{
							{
								SecretName: "test",
							},
						},
					},
				},
			},
			expect: &ingress.Ingress{
				Spec: ingress.IngressSpec{
					TLS: []ingress.IngressTLS{
						{
							SecretName: "test",
							Hosts:      []string{"*"},
						},
					},
				},
			},
			description: "tls host is empty",
		},
		{
			input: struct{ ing *ingress.Ingress }{
				ing: &ingress.Ingress{
					Spec: ingress.IngressSpec{
						TLS: []ingress.IngressTLS{
							{
								SecretName: "test",
								Hosts:      []string{"www.example.com"},
							},
						},
					},
				},
			},
			expect: &ingress.Ingress{
				Spec: ingress.IngressSpec{
					TLS: []ingress.IngressTLS{
						{
							SecretName: "test",
							Hosts:      []string{"www.example.com"},
						},
					},
				},
			},
			description: "tls host is not empty",
		},
		{
			input: struct{ ing *ingress.Ingress }{
				ing: &ingress.Ingress{
					Spec: ingress.IngressSpec{
						Rules: []ingress.IngressRule{
							{
								IngressRuleValue: ingress.IngressRuleValue{
									HTTP: nil,
								},
							},
						},
						TLS: []ingress.IngressTLS{
							{
								SecretName: "test",
								Hosts:      []string{"www.example.com"},
							},
						},
					},
				},
			},
			expect: &ingress.Ingress{
				Spec: ingress.IngressSpec{
					Rules: []ingress.IngressRule{
						{
							IngressRuleValue: ingress.IngressRuleValue{
								HTTP: nil,
							},
						},
					},
					TLS: []ingress.IngressTLS{
						{
							SecretName: "test",
							Hosts:      []string{"www.example.com"},
						},
					},
				},
			},
			description: "http is nil",
		},
		{
			input: struct{ ing *ingress.Ingress }{
				ing: &ingress.Ingress{
					Spec: ingress.IngressSpec{
						Rules: []ingress.IngressRule{
							{
								IngressRuleValue: ingress.IngressRuleValue{
									HTTP: &ingress.HTTPIngressRuleValue{
										Paths: []ingress.HTTPIngressPath{
											{
												Path:     "/test",
												PathType: &defaultPathType,
												Backend:  ingress.IngressBackend{},
											},
										},
									},
								},
							},
						},
						TLS: []ingress.IngressTLS{
							{
								SecretName: "test",
								Hosts:      []string{"www.example.com"},
							},
						},
					},
				},
			},
			expect: &ingress.Ingress{
				Spec: ingress.IngressSpec{
					Rules: []ingress.IngressRule{
						{
							Host: "*",
							IngressRuleValue: ingress.IngressRuleValue{
								HTTP: &ingress.HTTPIngressRuleValue{
									Paths: []ingress.HTTPIngressPath{
										{
											Path:     "/test",
											PathType: &defaultPathType,
											Backend:  ingress.IngressBackend{},
										},
									},
								},
							},
						},
					},
					TLS: []ingress.IngressTLS{
						{
							SecretName: "test",
							Hosts:      []string{"www.example.com"},
						},
					},
				},
			},
			description: "http is not nil but host is empty",
		},
		{
			input: struct{ ing *ingress.Ingress }{
				ing: &ingress.Ingress{
					Spec: ingress.IngressSpec{
						Rules: []ingress.IngressRule{
							{
								IngressRuleValue: ingress.IngressRuleValue{
									HTTP: &ingress.HTTPIngressRuleValue{
										Paths: []ingress.HTTPIngressPath{
											{
												Path:     "/test",
												PathType: &pathType,
												Backend:  ingress.IngressBackend{},
											},
										},
									},
								},
							},
						},
						TLS: []ingress.IngressTLS{
							{
								SecretName: "test",
								Hosts:      []string{"www.example.com"},
							},
						},
					},
				},
			},
			expect: &ingress.Ingress{
				Spec: ingress.IngressSpec{
					Rules: []ingress.IngressRule{
						{
							Host: "*",
							IngressRuleValue: ingress.IngressRuleValue{
								HTTP: &ingress.HTTPIngressRuleValue{
									Paths: []ingress.HTTPIngressPath{
										{
											Path:     "/test",
											PathType: &defaultPathType,
											Backend:  ingress.IngressBackend{},
										},
									},
								},
							},
						},
					},
					TLS: []ingress.IngressTLS{
						{
							SecretName: "test",
							Hosts:      []string{"www.example.com"},
						},
					},
				},
			},
			description: "http path type is ImplementationSpecific",
		},
	}

	for _, testcase := range testcases {
		setDefaultMSEIngressOptionalField(testcase.input.ing)
		require.Equal(t, testcase.expect, testcase.input.ing)
	}
}

func TestIngressControllerProcessing(t *testing.T) {
	fakeClient := kube.NewFakeClient()
	localKubeClient, _ := fakeClient, fakeClient

	options := common.Options{IngressClass: "mse", ClusterId: "", EnableStatus: true}

	secretController := secret.NewController(localKubeClient, options)

	opts := ktypes.InformerOptions{}
	ingressInformer := util.GetInformerFiltered(fakeClient, opts, gvrIngressV1Beta1, &ingress.Ingress{},
		func(options metav1.ListOptions) (runtime.Object, error) {
			return fakeClient.Kube().NetworkingV1beta1().Ingresses(opts.Namespace).List(context.Background(), options)
		},
		func(options metav1.ListOptions) (watch.Interface, error) {
			return fakeClient.Kube().NetworkingV1beta1().Ingresses(opts.Namespace).Watch(context.Background(), options)
		})
	ingressLister := networkinglister.NewIngressLister(ingressInformer.Informer.GetIndexer())
	serviceInformer := schemakubeclient.GetInformerFilteredFromGVR(fakeClient, opts, gvr.Service)
	serviceLister := listerv1.NewServiceLister(serviceInformer.Informer.GetIndexer())

	ingressController := &controller{
		options:          options,
		ingresses:        make(map[string]*ingress.Ingress),
		ingressInformer:  ingressInformer,
		ingressLister:    ingressLister,
		serviceInformer:  serviceInformer,
		serviceLister:    serviceLister,
		secretController: secretController,
	}

	ingressController.queue = controllers.NewQueue("ingress-test",
		controllers.WithReconciler(ingressController.onEvent),
		controllers.WithMaxAttempts(5))
	_, _ = ingressController.ingressInformer.Informer.AddEventHandler(controllers.ObjectHandler(ingressController.queue.AddObject))

	stopChan := make(chan struct{})
	t.Cleanup(func() {
		time.Sleep(3 * time.Second)
		close(stopChan)
	})

	go ingressController.ingressInformer.Start(stopChan)
	go ingressController.serviceInformer.Start(stopChan)
	go ingressController.secretController.Informer().Run(stopChan)

	go ingressController.Run(stopChan)

	ingressController.RegisterEventHandler(gvk.VirtualService, func(c1, c2 config.Config, e istiomodel.Event) {})
	ingressController.RegisterEventHandler(gvk.DestinationRule, func(c1, c2 config.Config, e istiomodel.Event) {})
	ingressController.RegisterEventHandler(gvk.EnvoyFilter, func(c1, c2 config.Config, e istiomodel.Event) {})
	ingressController.RegisterEventHandler(gvk.Gateway, func(c1, c2 config.Config, e istiomodel.Event) {})

	svcObj, err := fakeClient.Kube().CoreV1().Services("default").Create(context.Background(), &v1.Service{ObjectMeta: metav1.ObjectMeta{Name: "test"}}, metav1.CreateOptions{})
	require.NoError(t, err)
	err = serviceInformer.Informer.GetStore().Add(svcObj)
	require.NoError(t, err)
	services, err := serviceLister.List(labels.Everything())
	require.NoError(t, err)
	require.Equal(t, 1, len(services))

	ingress1 := &ingress.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-1",
		},
		Spec: v1beta1.IngressSpec{
			IngressClassName: &options.IngressClass,
			Rules: []v1beta1.IngressRule{
				{
					Host: "test.com",
					IngressRuleValue: v1beta1.IngressRuleValue{
						HTTP: &v1beta1.HTTPIngressRuleValue{
							Paths: []v1beta1.HTTPIngressPath{
								{
									Path: "/test",
								},
							},
						},
					},
				},
			},
		},
	}
	ingressObj, err := fakeClient.Kube().NetworkingV1beta1().Ingresses("default").Create(context.Background(), ingress1, metav1.CreateOptions{})
	require.NoError(t, err)
	err = ingressController.ingressInformer.Informer.GetStore().Add(ingressObj)
	require.NoError(t, err)
	ingresses := ingressController.List()
	require.Equal(t, 1, len(ingresses))

	ingress2 := &ingress.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-2",
			Namespace: "test-2",
		},
		Spec: v1beta1.IngressSpec{
			IngressClassName: &options.IngressClass,
			Rules: []v1beta1.IngressRule{
				{
					Host: "test.com",
					IngressRuleValue: v1beta1.IngressRuleValue{
						HTTP: &v1beta1.HTTPIngressRuleValue{
							Paths: []v1beta1.HTTPIngressPath{
								{
									Path: "/test",
								},
							},
						},
					},
				},
			},
		},
	}
	err = ingressController.ingressInformer.Informer.GetStore().Add(ingress2)
	require.NoError(t, err)
	ingresses = ingressController.List()
	require.Equal(t, 2, len(ingresses))
}

func TestShouldProcessIngressUpdate(t *testing.T) {
	c := controller{
		options: common.Options{
			IngressClass: "mse",
		},
		ingresses: make(map[string]*v1beta1.Ingress),
	}

	ingressClass := "mse"

	ingress1 := &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-1",
		},
		Spec: v1beta1.IngressSpec{
			IngressClassName: &ingressClass,
			Rules: []v1beta1.IngressRule{
				{
					Host: "test.com",
					IngressRuleValue: v1beta1.IngressRuleValue{
						HTTP: &v1beta1.HTTPIngressRuleValue{
							Paths: []v1beta1.HTTPIngressPath{
								{
									Path: "/test",
								},
							},
						},
					},
				},
			},
		},
	}

	should, _ := c.shouldProcessIngressUpdate(ingress1)
	if !should {
		t.Fatal("should be true")
	}

	ingress2 := *ingress1
	should, _ = c.shouldProcessIngressUpdate(&ingress2)
	if should {
		t.Fatal("should be false")
	}

	ingress3 := *ingress1
	ingress3.Annotations = map[string]string{
		"test": "true",
	}
	should, _ = c.shouldProcessIngressUpdate(&ingress3)
	if !should {
		t.Fatal("should be true")
	}
}

func TestCreateRuleKey(t *testing.T) {
	sep := "\n\n"
	wrapperHttpRoute := &common.WrapperHTTPRoute{
		Host:           "higress.com",
		OriginPathType: common.Prefix,
		OriginPath:     "/foo",
	}

	annots := annotations.Annotations{
		buildHigressAnnotationKey(annotations.MatchMethod):                                 "GET PUT",
		buildHigressAnnotationKey("exact-" + annotations.MatchHeader + "-abc"):             "123",
		buildHigressAnnotationKey("prefix-" + annotations.MatchHeader + "-def"):            "456",
		buildHigressAnnotationKey("exact-" + annotations.MatchPseudoHeader + "-authority"): "foo.bar.com",
		buildHigressAnnotationKey("prefix-" + annotations.MatchPseudoHeader + "-scheme"):   "htt",
		buildHigressAnnotationKey("exact-" + annotations.MatchQuery + "-region"):           "beijing",
		buildHigressAnnotationKey("prefix-" + annotations.MatchQuery + "-user-id"):         "user-",
	}
	expect := "higress.com-prefix-/foo" + sep + // host-pathType-path
		"GET PUT" + sep + // method
		"exact-:authority\tfoo.bar.com" + "\n" + "exact-abc\t123" + "\n" +
		"prefix-:scheme\thtt" + "\n" + "prefix-def\t456" + sep + // header
		"exact-region\tbeijing" + "\n" + "prefix-user-id\tuser-" + sep // params

	key := createRuleKey(annots, wrapperHttpRoute.PathFormat())
	if diff := cmp.Diff(expect, key); diff != "" {
		t.Errorf("CreateRuleKey() mismatch (-want +got):\n%s", diff)
	}
}

func buildHigressAnnotationKey(key string) string {
	return annotations.HigressAnnotationsPrefix + "/" + key
}
