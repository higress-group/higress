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

package ingressv1

import (
	"strings"
	"testing"

	"github.com/alibaba/higress/v2/pkg/cert"
	"github.com/alibaba/higress/v2/pkg/ingress/kube/annotations"
	"github.com/alibaba/higress/v2/pkg/ingress/kube/common"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	networking "istio.io/api/networking/v1alpha3"
	"istio.io/istio/pkg/config"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestShouldProcessIngressUpdate(t *testing.T) {
	c := controller{
		options: common.Options{
			IngressClass: "mse",
		},
		ingresses: make(map[string]*v1.Ingress),
	}

	ingressClass := "mse"

	ingress1 := &v1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-1",
		},
		Spec: v1.IngressSpec{
			IngressClassName: &ingressClass,
			Rules: []v1.IngressRule{
				{
					Host: "test.com",
					IngressRuleValue: v1.IngressRuleValue{
						HTTP: &v1.HTTPIngressRuleValue{
							Paths: []v1.HTTPIngressPath{
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

func TestGenerateHttpMatches(t *testing.T) {
	c := controller{}

	tt := []struct {
		pathType common.PathType
		path     string
		expect   []*networking.HTTPMatchRequest
	}{
		{
			pathType: common.Prefix,
			path:     "/foo",
			expect: []*networking.HTTPMatchRequest{
				{
					Uri: &networking.StringMatch{
						MatchType: &networking.StringMatch_Exact{Exact: "/foo"},
					},
				}, {
					Uri: &networking.StringMatch{
						MatchType: &networking.StringMatch_Prefix{Prefix: "/foo/"},
					},
				},
			},
		},
	}

	unexportedIgnoredTypes := []interface{}{
		networking.HTTPMatchRequest{},
		networking.StringMatch{},
	}

	for _, testcase := range tt {
		httpMatches := c.generateHttpMatches(testcase.pathType, testcase.path, nil)
		for idx, httpMatch := range httpMatches {
			if diff := cmp.Diff(httpMatch, testcase.expect[idx], cmpopts.IgnoreUnexported(unexportedIgnoredTypes...)); diff != "" {
				t.Errorf("generateHttpMatches() mismatch (-want +got):\n%s", diff)
			}
		}
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
			Spec: ingressSpecWithSSLPassthroughBackend("example.com", "app", 443),
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
	if tlsServer.Tls.GetMode() != networking.ServerTLSSettings_PASSTHROUGH {
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

func TestSSLPassthroughConvertGatewayRejectsNilInputs(t *testing.T) {
	c := controller{}
	wrapper := &common.WrapperConfig{
		Config:            &config.Config{},
		AnnotationsConfig: &annotations.Ingress{},
	}

	if err := c.ConvertGateway(nil, wrapper, nil); err == nil {
		t.Fatal("ConvertGateway() with nil convertOptions returned nil error")
	}
	if err := c.ConvertGateway(&common.ConvertOptions{}, nil, nil); err == nil {
		t.Fatal("ConvertGateway() with nil wrapper returned nil error")
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
			Spec: ingressSpecWithSSLPassthroughBackend("example.com", "app", 443),
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
				Name:      "tls-passthrough-canary",
			},
			Spec: ingressSpecWithSSLPassthroughBackend("example.com", "app-canary", 443),
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
			Spec: ingressSpecWithSSLPassthroughBackend("example.com", "primary", 443),
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
			Spec: ingressSpecWithSSLPassthroughBackend("example.com", "duplicate", 443),
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

func TestSSLPassthroughDuplicateTLSHostRecordsInvalidDomain(t *testing.T) {
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
			Spec: ingressSpecWithSSLPassthroughBackend("example.com", "primary", 443),
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
			Spec: ingressSpecWithSSLPassthroughBackend("example.com", "duplicate", 443),
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

	if len(options.IngressDomainCache.Invalid) != 1 {
		t.Fatalf("invalid domain count mismatch, want 1, got %d", len(options.IngressDomainCache.Invalid))
	}
	invalid := options.IngressDomainCache.Invalid[0]
	if invalid.Error == "" {
		t.Fatal("duplicated tls invalid domain error is empty")
	}
	if !strings.Contains(invalid.Error, "tls-passthrough-primary") {
		t.Fatalf("invalid domain error does not reference previous ingress: %s", invalid.Error)
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
			Spec: v1.IngressSpec{
				TLS: []v1.IngressTLS{
					{Hosts: []string{"example.com"}},
				},
				Rules: []v1.IngressRule{
					ingressRule("example.com", ingressPath("/", "primary", 443)),
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
			Spec: ingressSpecWithSSLPassthroughBackend("example.com", "duplicate", 443),
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
			Spec: v1.IngressSpec{
				Rules: []v1.IngressRule{
					ingressRule("example.com", ingressPath("/", "root", 443)),
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
			Spec: ingressSpecWithSSLPassthroughPaths("example.com", []v1.HTTPIngressPath{
				ingressPath("/passthrough", "passthrough", 443),
			}),
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
	if gateway.Servers[1].Tls.GetMode() != networking.ServerTLSSettings_PASSTHROUGH {
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
			Spec: ingressSpecWithSSLPassthroughPaths("example.com", []v1.HTTPIngressPath{
				ingressPath("/api", "api", 8443),
			}),
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
			Spec: ingressSpecWithSSLPassthroughBackend("example.com", "root", 443),
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
	if options.Gateways["example.com"].Gateway.Servers[1].Tls.GetMode() != networking.ServerTLSSettings_PASSTHROUGH {
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
			Spec: v1.IngressSpec{
				Rules: []v1.IngressRule{
					{
						Host: "example.com",
						IngressRuleValue: v1.IngressRuleValue{
							HTTP: &v1.HTTPIngressRuleValue{
								Paths: []v1.HTTPIngressPath{
									ingressPath("/health", "health", 8443),
								},
							},
						},
					},
					{
						Host: "example.com",
						IngressRuleValue: v1.IngressRuleValue{
							HTTP: &v1.HTTPIngressRuleValue{
								Paths: []v1.HTTPIngressPath{
									ingressPath("/", "root", 443),
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
			Spec: v1.IngressSpec{
				Rules: []v1.IngressRule{
					{
						Host: "example.com",
						IngressRuleValue: v1.IngressRuleValue{
							HTTP: &v1.HTTPIngressRuleValue{
								Paths: []v1.HTTPIngressPath{
									ingressPath("/", "first", 443),
								},
							},
						},
					},
					{
						Host: "example.com",
						IngressRuleValue: v1.IngressRuleValue{
							HTTP: &v1.HTTPIngressRuleValue{
								Paths: []v1.HTTPIngressPath{
									ingressPath("/", "second", 443),
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
		rules      []v1.IngressRule
		wantHosts  []string
		wantRoutes map[string]string
	}{
		{
			name: "root path first",
			rules: []v1.IngressRule{
				ingressRule("first.example.com", ingressPath("/", "first", 443)),
				ingressRule("middle.example.com", ingressPath("/health", "middle", 8443)),
				ingressRule("last.example.com", ingressPath("/health", "last", 8443)),
			},
			wantHosts: []string{"first.example.com"},
			wantRoutes: map[string]string{
				"first.example.com": "first.default.svc.cluster.local",
			},
		},
		{
			name: "root path middle",
			rules: []v1.IngressRule{
				ingressRule("first.example.com", ingressPath("/health", "first", 8443)),
				ingressRule("middle.example.com", ingressPath("/", "middle", 443)),
				ingressRule("last.example.com", ingressPath("/health", "last", 8443)),
			},
			wantHosts: []string{"middle.example.com"},
			wantRoutes: map[string]string{
				"middle.example.com": "middle.default.svc.cluster.local",
			},
		},
		{
			name: "root path last",
			rules: []v1.IngressRule{
				ingressRule("first.example.com", ingressPath("/health", "first", 8443)),
				ingressRule("middle.example.com", ingressPath("/health", "middle", 8443)),
				ingressRule("last.example.com", ingressPath("/", "last", 443)),
			},
			wantHosts: []string{"last.example.com"},
			wantRoutes: map[string]string{
				"last.example.com": "last.default.svc.cluster.local",
			},
		},
		{
			name: "multiple root hosts",
			rules: []v1.IngressRule{
				ingressRule("first.example.com", ingressPath("/", "first", 443)),
				ingressRule("middle.example.com", ingressPath("/", "middle", 443)),
				ingressRule("last.example.com", ingressPath("/", "last", 443)),
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
					Spec: v1.IngressSpec{
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

func TestSSLPassthroughUsesRootPathBackend(t *testing.T) {
	c := controller{}
	wrapper := &common.WrapperConfig{
		Config: &config.Config{
			Meta: config.Meta{
				Namespace: "default",
				Name:      "tls-passthrough-root",
			},
			Spec: ingressSpecWithSSLPassthroughPaths("example.com", []v1.HTTPIngressPath{
				ingressPath("/api", "api", 8443),
				ingressPath("/", "root", 443),
			}),
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
			Spec: ingressSpecWithSSLPassthroughBackend("", "root", 443),
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
			Spec: ingressSpecWithSSLPassthroughPaths("example.com", []v1.HTTPIngressPath{
				ingressPath("/api", "api", 8443),
			}),
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

func TestSSLPassthroughKeepsMCPResourceBackend(t *testing.T) {
	c := controller{}
	apiGroup := "networking.higress.io"
	wrapper := &common.WrapperConfig{
		Config: &config.Config{
			Meta: config.Meta{
				Namespace: "default",
				Name:      "tls-passthrough-mcp",
			},
			Spec: ingressSpecWithSSLPassthroughPaths("example.com", []v1.HTTPIngressPath{
				{
					Path:     "/",
					PathType: pathTypePtr(v1.PathTypePrefix),
					Backend: v1.IngressBackend{
						Resource: &corev1.TypedLocalObjectReference{
							APIGroup: &apiGroup,
							Kind:     "McpBridge",
							Name:     "default",
						},
					},
				},
			}),
		},
		AnnotationsConfig: &annotations.Ingress{
			Destination: &annotations.DestinationConfig{
				McpDestination: []*networking.HTTPRouteDestination{
					{
						Destination: &networking.Destination{
							Host: "mcp.example.internal",
							Port: &networking.PortSelector{Number: 443},
						},
						Weight: 100,
					},
				},
				WeightSum: 100,
			},
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
	if got := routes[0].Route[0].Destination.Host; got != "mcp.example.internal" {
		t.Fatalf("destination host mismatch, got %s", got)
	}
}

func TestBackendToTLSRouteDestinationRejectsEmptyMCPDestination(t *testing.T) {
	c := controller{}
	apiGroup := "networking.higress.io"
	backend := &v1.IngressBackend{
		Resource: &corev1.TypedLocalObjectReference{
			APIGroup: &apiGroup,
			Kind:     "McpBridge",
			Name:     "default",
		},
	}
	config := &annotations.DestinationConfig{}

	destinations, event := c.backendToTLSRouteDestination(backend, "default", config)
	if event != common.InvalidBackendService {
		t.Fatalf("event mismatch, want InvalidBackendService, got %s", event)
	}
	if len(destinations) != 0 {
		t.Fatalf("destination count mismatch, want 0, got %d", len(destinations))
	}
}

func ingressSpecWithSSLPassthroughBackend(host, service string, port int32) v1.IngressSpec {
	return ingressSpecWithSSLPassthroughPaths(host, []v1.HTTPIngressPath{
		ingressPath("/", service, port),
	})
}

func ingressSpecWithSSLPassthroughPaths(host string, paths []v1.HTTPIngressPath) v1.IngressSpec {
	return v1.IngressSpec{
		Rules: []v1.IngressRule{
			{
				Host: host,
				IngressRuleValue: v1.IngressRuleValue{
					HTTP: &v1.HTTPIngressRuleValue{
						Paths: paths,
					},
				},
			},
		},
	}
}

func ingressPath(path, service string, port int32) v1.HTTPIngressPath {
	return v1.HTTPIngressPath{
		Path:     path,
		PathType: pathTypePtr(v1.PathTypePrefix),
		Backend: v1.IngressBackend{
			Service: &v1.IngressServiceBackend{
				Name: service,
				Port: v1.ServiceBackendPort{Number: port},
			},
		},
	}
}

func pathTypePtr(pathType v1.PathType) *v1.PathType {
	return &pathType
}

func ingressRule(host string, paths ...v1.HTTPIngressPath) v1.IngressRule {
	return v1.IngressRule{
		Host: host,
		IngressRuleValue: v1.IngressRuleValue{
			HTTP: &v1.HTTPIngressRuleValue{
				Paths: paths,
			},
		},
	}
}
