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

	httppb "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"
	networking "istio.io/api/networking/v1alpha3"
	"istio.io/istio/pkg/cluster"
	"istio.io/istio/pkg/config"
	"istio.io/istio/pkg/config/constants"
	"istio.io/istio/pkg/config/schema/gvk"
	"istio.io/istio/pkg/config/xds"
	corev1 "k8s.io/api/core/v1"
	ingress "k8s.io/api/networking/v1"
	ingressv1beta1 "k8s.io/api/networking/v1beta1"

	"github.com/alibaba/higress/v2/pkg/ingress/kube/annotations"
	"github.com/alibaba/higress/v2/pkg/ingress/kube/common"
	controllerv1beta1 "github.com/alibaba/higress/v2/pkg/ingress/kube/ingress"
	controllerv1 "github.com/alibaba/higress/v2/pkg/ingress/kube/ingressv1"
	"github.com/alibaba/higress/v2/pkg/kube"
)

func TestNormalizeWeightedCluster(t *testing.T) {
	validate := func(route *common.WrapperHTTPRoute) int32 {
		var total int32
		for _, routeDestination := range route.HTTPRoute.Route {
			total += routeDestination.Weight
		}

		return total
	}

	var testCases []*common.WrapperHTTPRoute
	testCases = append(testCases, &common.WrapperHTTPRoute{
		HTTPRoute: &networking.HTTPRoute{
			Route: []*networking.HTTPRouteDestination{
				{
					Weight: 100,
				},
			},
		},
	})
	testCases = append(testCases, &common.WrapperHTTPRoute{
		HTTPRoute: &networking.HTTPRoute{
			Route: []*networking.HTTPRouteDestination{
				{
					Weight: 98,
				},
			},
		},
	})

	testCases = append(testCases, &common.WrapperHTTPRoute{
		HTTPRoute: &networking.HTTPRoute{
			Route: []*networking.HTTPRouteDestination{
				{
					Weight: 0,
				},
				{
					Weight: 48,
				},
				{
					Weight: 48,
				},
			},
		},
		WeightTotal: 100,
	})

	testCases = append(testCases, &common.WrapperHTTPRoute{
		HTTPRoute: &networking.HTTPRoute{
			Route: []*networking.HTTPRouteDestination{
				{
					Weight: 0,
				},
				{
					Weight: 48,
				},
				{
					Weight: 48,
				},
			},
		},
		WeightTotal: 80,
	})

	for _, route := range testCases {
		t.Run("", func(t *testing.T) {
			normalizeWeightedCluster(nil, route)
			if validate(route) != 100 {
				t.Fatalf("Weight sum should be 100, but actual is %d", validate(route))
			}
		})
	}
}

func TestVirtualServiceNameAndClusterID(t *testing.T) {
	cleanHost := common.CleanHost("example.com")
	wrapperVS := &common.WrapperVirtualService{
		WrapperConfig: &common.WrapperConfig{
			Config: &config.Config{
				Meta: config.Meta{
					Namespace: "tls-ns",
					Name:      "tls-ingress",
					Annotations: map[string]string{
						common.ClusterIdAnnotation: "tls-cluster",
					},
				},
			},
		},
	}
	routes := []*common.WrapperHTTPRoute{
		{
			WrapperConfig: &common.WrapperConfig{
				Config: &config.Config{
					Meta: config.Meta{
						Namespace: "http-ns",
						Name:      "http-ingress",
					},
				},
			},
			ClusterId: "http-cluster",
		},
	}

	name, clusterID := virtualServiceNameAndClusterID(cleanHost, wrapperVS, routes)
	if name != common.CreateConvertedName(constants.IstioIngressGatewayName, "http-ns", "http-ingress", cleanHost) {
		t.Fatalf("http-backed virtual service name mismatch: %s", name)
	}
	if clusterID != "http-cluster" {
		t.Fatalf("http-backed cluster id mismatch: %s", clusterID)
	}

	name, clusterID = virtualServiceNameAndClusterID(cleanHost, wrapperVS, nil)
	if name != common.CreateConvertedName(constants.IstioIngressGatewayName, "tls-ns", "tls-ingress", cleanHost) {
		t.Fatalf("tls-only virtual service name mismatch: %s", name)
	}
	if clusterID != "tls-cluster" {
		t.Fatalf("tls-only cluster id mismatch: %s", clusterID)
	}
}

func TestPreparePassthroughTLSHostOwnersRequiresPassthroughHost(t *testing.T) {
	m := &IngressConfig{}
	configs := []common.WrapperConfig{
		{
			Config: &config.Config{
				Meta: config.Meta{
					Namespace: "default",
					Name:      "plain-root",
				},
				Spec: ingress.IngressSpec{
					Rules: []ingress.IngressRule{
						{
							Host: "example.com",
							IngressRuleValue: ingress.IngressRuleValue{
								HTTP: &ingress.HTTPIngressRuleValue{
									Paths: []ingress.HTTPIngressPath{
										{Path: "/"},
									},
								},
							},
						},
					},
				},
			},
			AnnotationsConfig: &annotations.Ingress{},
		},
		{
			Config: &config.Config{
				Meta: config.Meta{
					Namespace: "default",
					Name:      "plain-root-duplicate",
				},
				Spec: ingress.IngressSpec{
					Rules: []ingress.IngressRule{
						{
							Host: "example.com",
							IngressRuleValue: ingress.IngressRuleValue{
								HTTP: &ingress.HTTPIngressRuleValue{
									Paths: []ingress.HTTPIngressPath{
										{Path: "/"},
									},
								},
							},
						},
					},
				},
			},
			AnnotationsConfig: &annotations.Ingress{},
		},
	}

	options := &common.ConvertOptions{}
	m.preparePassthroughTLSHostOwners(options, configs)

	if len(options.PassthroughTLSHostOwners) != 0 {
		t.Fatalf("unexpected ssl passthrough owners: %+v", options.PassthroughTLSHostOwners)
	}
}

func TestPreparePassthroughTLSHostOwnersUsesFirstRootPathOwner(t *testing.T) {
	m := &IngressConfig{}
	configs := []common.WrapperConfig{
		{
			Config: &config.Config{
				Meta: config.Meta{
					Namespace: "default",
					Name:      "plain-root",
				},
				Spec: ingress.IngressSpec{
					Rules: []ingress.IngressRule{
						{
							Host: "example.com",
							IngressRuleValue: ingress.IngressRuleValue{
								HTTP: &ingress.HTTPIngressRuleValue{
									Paths: []ingress.HTTPIngressPath{
										{Path: "/"},
									},
								},
							},
						},
					},
				},
			},
			AnnotationsConfig: &annotations.Ingress{},
		},
		{
			Config: &config.Config{
				Meta: config.Meta{
					Namespace: "default",
					Name:      "passthrough-non-root",
				},
				Spec: ingress.IngressSpec{
					Rules: []ingress.IngressRule{
						{
							Host: "example.com",
							IngressRuleValue: ingress.IngressRuleValue{
								HTTP: &ingress.HTTPIngressRuleValue{
									Paths: []ingress.HTTPIngressPath{
										{Path: "/api"},
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
		},
	}

	options := &common.ConvertOptions{}
	m.preparePassthroughTLSHostOwners(options, configs)

	if !common.IsPassthroughTLSHostOwner(options, configs[0].Config, "example.com") {
		t.Fatal("first root ingress was not recorded as passthrough owner")
	}
	if !common.HasPassthroughTLSHostOwner(options, configs[0].Config) {
		t.Fatal("first root ingress was not found as passthrough owner")
	}
}

func TestPreparePassthroughTLSHostOwnersIgnoresHTTPOnlyIngressForHTTPSFallback(t *testing.T) {
	m := &IngressConfig{}
	configs := []common.WrapperConfig{
		{
			Config: &config.Config{
				Meta: config.Meta{
					Namespace: "default",
					Name:      "http-only",
				},
				Spec: ingress.IngressSpec{
					Rules: []ingress.IngressRule{
						{
							Host: "example.com",
							IngressRuleValue: ingress.IngressRuleValue{
								HTTP: &ingress.HTTPIngressRuleValue{
									Paths: []ingress.HTTPIngressPath{
										{Path: "/api"},
									},
								},
							},
						},
					},
				},
			},
			AnnotationsConfig: &annotations.Ingress{},
		},
		{
			Config: &config.Config{
				Meta: config.Meta{
					Namespace: "default",
					Name:      "tls-ingress",
				},
				Spec: ingress.IngressSpec{
					TLS: []ingress.IngressTLS{
						{
							Hosts:      []string{"example.com"},
							SecretName: "example-com",
						},
					},
					Rules: []ingress.IngressRule{
						{
							Host: "example.com",
							IngressRuleValue: ingress.IngressRuleValue{
								HTTP: &ingress.HTTPIngressRuleValue{
									Paths: []ingress.HTTPIngressPath{
										{Path: "/app"},
									},
								},
							},
						},
					},
				},
			},
			AnnotationsConfig: &annotations.Ingress{},
		},
	}

	options := &common.ConvertOptions{}
	m.preparePassthroughTLSHostOwners(options, configs)

	if len(options.PassthroughTLSHostOwners) != 0 {
		t.Fatalf("unexpected ssl passthrough owners: %+v", options.PassthroughTLSHostOwners)
	}
}

func TestConvertGatewaysHonorsFirstRootPathSSLPassthroughOwner(t *testing.T) {
	fake := kube.NewFakeClient()
	options := common.Options{
		Enable:           true,
		ClusterId:        "ingress-v1",
		RawClusterId:     "ingress-v1__",
		GatewayHttpPort:  80,
		GatewayHttpsPort: 443,
	}
	ingressController := controllerv1.NewController(fake, fake, options, nil)
	m := NewIngressConfig(fake, nil, "wakanda", options)
	m.remoteIngressControllers = map[cluster.ID]common.IngressController{
		"ingress-v1": ingressController,
	}

	configs := []common.WrapperConfig{
		{
			Config: &config.Config{
				Meta: config.Meta{
					Namespace: "default",
					Name:      "tls-non-root",
					Annotations: map[string]string{
						common.ClusterIdAnnotation: "ingress-v1",
					},
				},
				Spec: ingress.IngressSpec{
					TLS: []ingress.IngressTLS{
						{
							Hosts:      []string{"example.com"},
							SecretName: "example-com",
						},
					},
					Rules: []ingress.IngressRule{
						{
							Host: "example.com",
							IngressRuleValue: ingress.IngressRuleValue{
								HTTP: &ingress.HTTPIngressRuleValue{
									Paths: []ingress.HTTPIngressPath{
										{Path: "/api"},
									},
								},
							},
						},
					},
				},
			},
			AnnotationsConfig: &annotations.Ingress{},
		},
		{
			Config: &config.Config{
				Meta: config.Meta{
					Namespace: "default",
					Name:      "passthrough-root",
					Annotations: map[string]string{
						common.ClusterIdAnnotation: "ingress-v1",
					},
				},
				Spec: ingress.IngressSpec{
					Rules: []ingress.IngressRule{
						{
							Host: "example.com",
							IngressRuleValue: ingress.IngressRuleValue{
								HTTP: &ingress.HTTPIngressRuleValue{
									Paths: []ingress.HTTPIngressPath{
										{Path: "/"},
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
		},
	}

	result := m.convertGateways(configs)
	if len(result) != 1 {
		t.Fatalf("gateway count mismatch, want 1, got %d", len(result))
	}
	gateway := result[0].Spec.(*networking.Gateway)
	if len(gateway.Servers) != 2 {
		t.Fatalf("server count mismatch, want 2, got %d", len(gateway.Servers))
	}
	tlsServer := gateway.Servers[1]
	if tlsServer.Port.Protocol != "TLS" {
		t.Fatalf("tls server protocol mismatch, want TLS, got %s", tlsServer.Port.Protocol)
	}
	if tlsServer.Tls.GetMode() != networking.ServerTLSSettings_PASSTHROUGH {
		t.Fatalf("tls mode mismatch, want PASSTHROUGH, got %s", tlsServer.Tls.GetMode())
	}
}

func TestConvertGatewaysUsesFirstRootOwnerWhenLaterIngressEnablesSSLPassthrough(t *testing.T) {
	fake := kube.NewFakeClient()
	options := common.Options{
		Enable:           true,
		ClusterId:        "ingress-v1",
		RawClusterId:     "ingress-v1__",
		GatewayHttpPort:  80,
		GatewayHttpsPort: 443,
	}
	ingressController := controllerv1.NewController(fake, fake, options, nil)
	m := NewIngressConfig(fake, nil, "wakanda", options)
	m.remoteIngressControllers = map[cluster.ID]common.IngressController{
		"ingress-v1": ingressController,
	}

	configs := []common.WrapperConfig{
		ingressV1Wrapper("root", "example.com", "/", false),
		ingressV1Wrapper("passthrough", "example.com", "/passthrough", true),
	}

	result := m.convertGateways(configs)
	if len(result) != 1 {
		t.Fatalf("gateway count mismatch, want 1, got %d", len(result))
	}
	gateway := result[0].Spec.(*networking.Gateway)
	if len(gateway.Servers) != 2 {
		t.Fatalf("server count mismatch, want 2, got %d", len(gateway.Servers))
	}
	tlsServer := gateway.Servers[1]
	if tlsServer.Port.Protocol != "TLS" {
		t.Fatalf("tls server protocol mismatch, want TLS, got %s", tlsServer.Port.Protocol)
	}
	if tlsServer.Tls.GetMode() != networking.ServerTLSSettings_PASSTHROUGH {
		t.Fatalf("tls mode mismatch, want PASSTHROUGH, got %s", tlsServer.Tls.GetMode())
	}
}

func TestConvertVirtualServiceUsesFirstRootOwnerWhenLaterIngressEnablesSSLPassthrough(t *testing.T) {
	fake := kube.NewFakeClient()
	options := common.Options{
		Enable:           true,
		ClusterId:        "ingress-v1",
		RawClusterId:     "ingress-v1__",
		GatewayHttpPort:  80,
		GatewayHttpsPort: 443,
	}
	ingressController := controllerv1.NewController(fake, fake, options, nil)
	m := NewIngressConfig(fake, nil, "wakanda", options)
	m.remoteIngressControllers = map[cluster.ID]common.IngressController{
		"ingress-v1": ingressController,
	}

	configs := []common.WrapperConfig{
		ingressV1Wrapper("root", "example.com", "/", false),
		ingressV1Wrapper("passthrough", "example.com", "/passthrough", true),
	}

	result := m.convertVirtualService(configs)
	if len(result) != 1 {
		t.Fatalf("virtual service count mismatch, want 1, got %d", len(result))
	}
	vs := result[0].Spec.(*networking.VirtualService)
	if len(vs.Tls) != 1 {
		t.Fatalf("tls route count mismatch, want 1, got %d", len(vs.Tls))
	}
	if got := vs.Tls[0].Route[0].Destination.Host; got != "root.default.svc.cluster.local" {
		t.Fatalf("destination host mismatch, want root.default.svc.cluster.local, got %s", got)
	}
}

func TestConvertGatewaysForIngress(t *testing.T) {
	fake := kube.NewFakeClient()
	v1Beta1Options := common.Options{
		Enable:           true,
		ClusterId:        "ingress-v1beta1",
		RawClusterId:     "ingress-v1beta1__",
		GatewayHttpPort:  80,
		GatewayHttpsPort: 443,
	}
	v1Options := common.Options{
		Enable:           true,
		ClusterId:        "ingress-v1",
		RawClusterId:     "ingress-v1__",
		GatewayHttpPort:  80,
		GatewayHttpsPort: 443,
	}
	ingressV1Beta1Controller := controllerv1beta1.NewController(fake, fake, v1Beta1Options, nil)
	ingressV1Controller := controllerv1.NewController(fake, fake, v1Options, nil)
	options := common.Options{
		Enable:           true,
		ClusterId:        "gw-123-istio",
		RawClusterId:     "gw-123-istio__",
		GatewayHttpPort:  80,
		GatewayHttpsPort: 443,
	}
	m := NewIngressConfig(fake, nil, "wakanda", options)
	m.remoteIngressControllers = map[cluster.ID]common.IngressController{
		"ingress-v1beta1": ingressV1Beta1Controller,
		"ingress-v1":      ingressV1Controller,
	}

	testCases := []struct {
		name        string
		inputConfig []common.WrapperConfig
		expect      map[string]config.Config
	}{
		{
			name: "ingress v1beta1",
			inputConfig: []common.WrapperConfig{
				{
					Config: &config.Config{
						Meta: config.Meta{
							Name:      "test-1",
							Namespace: "wakanda",
							Annotations: map[string]string{
								common.ClusterIdAnnotation: "ingress-v1beta1",
							},
						},
						Spec: ingressv1beta1.IngressSpec{
							TLS: []ingressv1beta1.IngressTLS{
								{
									Hosts:      []string{"test.com"},
									SecretName: "test-com",
								},
							},
							Rules: []ingressv1beta1.IngressRule{
								{
									Host: "foo.com",
									IngressRuleValue: ingressv1beta1.IngressRuleValue{
										HTTP: &ingressv1beta1.HTTPIngressRuleValue{
											Paths: []ingressv1beta1.HTTPIngressPath{
												{
													Path: "/test",
												},
											},
										},
									},
								},
								{
									Host: "test.com",
									IngressRuleValue: ingressv1beta1.IngressRuleValue{
										HTTP: &ingressv1beta1.HTTPIngressRuleValue{
											Paths: []ingressv1beta1.HTTPIngressPath{
												{
													Path: "/test",
												},
											},
										},
									},
								},
							},
						},
					},
					AnnotationsConfig: &annotations.Ingress{
						DownstreamTLS: &annotations.DownstreamTLSConfig{
							CipherSuites: []string{"ECDHE-RSA-AES128-GCM-SHA256", "AES256-SHA"},
						},
					},
				},
				{
					Config: &config.Config{
						Meta: config.Meta{
							Name:      "test-2",
							Namespace: "wakanda",
							Annotations: map[string]string{
								common.ClusterIdAnnotation: "ingress-v1beta1",
							},
						},
						Spec: ingressv1beta1.IngressSpec{
							TLS: []ingressv1beta1.IngressTLS{
								{
									Hosts:      []string{"foo.com"},
									SecretName: "foo-com",
								},
								{
									Hosts:      []string{"test.com"},
									SecretName: "test-com-2",
								},
							},
							Rules: []ingressv1beta1.IngressRule{
								{
									Host: "foo.com",
									IngressRuleValue: ingressv1beta1.IngressRuleValue{
										HTTP: &ingressv1beta1.HTTPIngressRuleValue{
											Paths: []ingressv1beta1.HTTPIngressPath{
												{
													Path: "/test",
												},
											},
										},
									},
								},
								{
									Host: "bar.com",
									IngressRuleValue: ingressv1beta1.IngressRuleValue{
										HTTP: &ingressv1beta1.HTTPIngressRuleValue{
											Paths: []ingressv1beta1.HTTPIngressPath{
												{
													Path: "/test",
												},
											},
										},
									},
								},
								{
									Host: "test.com",
									IngressRuleValue: ingressv1beta1.IngressRuleValue{
										HTTP: &ingressv1beta1.HTTPIngressRuleValue{
											Paths: []ingressv1beta1.HTTPIngressPath{
												{
													Path: "/test",
												},
											},
										},
									},
								},
							},
						},
					},
					AnnotationsConfig: &annotations.Ingress{
						DownstreamTLS: &annotations.DownstreamTLSConfig{
							CipherSuites: []string{"ECDHE-RSA-AES128-GCM-SHA256"},
						},
					},
				},
			},
			expect: map[string]config.Config{
				"foo.com": {
					Meta: config.Meta{
						GroupVersionKind: gvk.Gateway,
						Name:             "istio-autogenerated-k8s-ingress-" + common.CleanHost("foo.com"),
						Namespace:        "wakanda",
						Annotations: map[string]string{
							common.ClusterIdAnnotation: "ingress-v1beta1",
							common.HostAnnotation:      "foo.com",
						},
					},
					Spec: &networking.Gateway{
						Servers: []*networking.Server{
							{
								Port: &networking.Port{
									Number:   80,
									Protocol: "HTTP",
									Name:     "http-80-ingress-ingress-v1beta1",
								},
								Hosts: []string{"foo.com"},
							},
							{
								Port: &networking.Port{
									Number:   443,
									Protocol: "HTTPS",
									Name:     "https-443-ingress-ingress-v1beta1",
								},
								Hosts: []string{"foo.com"},
								Tls: &networking.ServerTLSSettings{
									Mode:           networking.ServerTLSSettings_SIMPLE,
									CredentialName: "kubernetes-ingress://ingress-v1beta1__/wakanda/foo-com",
									CipherSuites:   []string{"ECDHE-RSA-AES128-GCM-SHA256", "AES256-SHA"},
								},
							},
						},
					},
				},
				"test.com": {
					Meta: config.Meta{
						GroupVersionKind: gvk.Gateway,
						Name:             "istio-autogenerated-k8s-ingress-" + common.CleanHost("test.com"),
						Namespace:        "wakanda",
						Annotations: map[string]string{
							common.ClusterIdAnnotation: "ingress-v1beta1",
							common.HostAnnotation:      "test.com",
						},
					},
					Spec: &networking.Gateway{
						Servers: []*networking.Server{
							{
								Port: &networking.Port{
									Number:   80,
									Protocol: "HTTP",
									Name:     "http-80-ingress-ingress-v1beta1",
								},
								Hosts: []string{"test.com"},
							},
							{
								Port: &networking.Port{
									Number:   443,
									Protocol: "HTTPS",
									Name:     "https-443-ingress-ingress-v1beta1",
								},
								Hosts: []string{"test.com"},
								Tls: &networking.ServerTLSSettings{
									Mode:           networking.ServerTLSSettings_SIMPLE,
									CredentialName: "kubernetes-ingress://ingress-v1beta1__/wakanda/test-com",
									CipherSuites:   []string{"ECDHE-RSA-AES128-GCM-SHA256", "AES256-SHA"},
								},
							},
						},
					},
				},
				"bar.com": {
					Meta: config.Meta{
						GroupVersionKind: gvk.Gateway,
						Name:             "istio-autogenerated-k8s-ingress-" + common.CleanHost("bar.com"),
						Namespace:        "wakanda",
						Annotations: map[string]string{
							common.ClusterIdAnnotation: "ingress-v1beta1",
							common.HostAnnotation:      "bar.com",
						},
					},
					Spec: &networking.Gateway{
						Servers: []*networking.Server{
							{
								Port: &networking.Port{
									Number:   80,
									Protocol: "HTTP",
									Name:     "http-80-ingress-ingress-v1beta1",
								},
								Hosts: []string{"bar.com"},
							},
						},
					},
				},
			},
		},
		{
			name: "ingress v1",
			inputConfig: []common.WrapperConfig{
				{
					Config: &config.Config{
						Meta: config.Meta{
							Name:      "test-1",
							Namespace: "wakanda",
							Annotations: map[string]string{
								common.ClusterIdAnnotation: "ingress-v1",
							},
						},
						Spec: ingress.IngressSpec{
							TLS: []ingress.IngressTLS{
								{
									Hosts:      []string{"test.com"},
									SecretName: "test-com",
								},
							},
							Rules: []ingress.IngressRule{
								{
									Host: "foo.com",
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
								{
									Host: "test.com",
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
						},
					},
					AnnotationsConfig: &annotations.Ingress{},
				},
				{
					Config: &config.Config{
						Meta: config.Meta{
							Name:      "test-2",
							Namespace: "wakanda",
							Annotations: map[string]string{
								common.ClusterIdAnnotation: "ingress-v1",
							},
						},
						Spec: ingress.IngressSpec{
							TLS: []ingress.IngressTLS{
								{
									Hosts:      []string{"foo.com"},
									SecretName: "foo-com",
								},
								{
									Hosts:      []string{"test.com"},
									SecretName: "test-com-2",
								},
							},
							Rules: []ingress.IngressRule{
								{
									Host: "foo.com",
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
								{
									Host: "bar.com",
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
								{
									Host: "test.com",
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
						},
					},
					AnnotationsConfig: &annotations.Ingress{
						DownstreamTLS: &annotations.DownstreamTLSConfig{
							CipherSuites: []string{"ECDHE-RSA-AES128-GCM-SHA256"},
						},
					},
				},
			},
			expect: map[string]config.Config{
				"foo.com": {
					Meta: config.Meta{
						GroupVersionKind: gvk.Gateway,
						Name:             "istio-autogenerated-k8s-ingress-" + common.CleanHost("foo.com"),
						Namespace:        "wakanda",
						Annotations: map[string]string{
							common.ClusterIdAnnotation: "ingress-v1",
							common.HostAnnotation:      "foo.com",
						},
					},
					Spec: &networking.Gateway{
						Servers: []*networking.Server{
							{
								Port: &networking.Port{
									Number:   80,
									Protocol: "HTTP",
									Name:     "http-80-ingress-ingress-v1",
								},
								Hosts: []string{"foo.com"},
							},
							{
								Port: &networking.Port{
									Number:   443,
									Protocol: "HTTPS",
									Name:     "https-443-ingress-ingress-v1",
								},
								Hosts: []string{"foo.com"},
								Tls: &networking.ServerTLSSettings{
									Mode:           networking.ServerTLSSettings_SIMPLE,
									CredentialName: "kubernetes-ingress://ingress-v1__/wakanda/foo-com",
									CipherSuites:   []string{"ECDHE-RSA-AES128-GCM-SHA256"},
								},
							},
						},
					},
				},
				"test.com": {
					Meta: config.Meta{
						GroupVersionKind: gvk.Gateway,
						Name:             "istio-autogenerated-k8s-ingress-" + common.CleanHost("test.com"),
						Namespace:        "wakanda",
						Annotations: map[string]string{
							common.ClusterIdAnnotation: "ingress-v1",
							common.HostAnnotation:      "test.com",
						},
					},
					Spec: &networking.Gateway{
						Servers: []*networking.Server{
							{
								Port: &networking.Port{
									Number:   80,
									Protocol: "HTTP",
									Name:     "http-80-ingress-ingress-v1",
								},
								Hosts: []string{"test.com"},
							},
							{
								Port: &networking.Port{
									Number:   443,
									Protocol: "HTTPS",
									Name:     "https-443-ingress-ingress-v1",
								},
								Hosts: []string{"test.com"},
								Tls: &networking.ServerTLSSettings{
									Mode:           networking.ServerTLSSettings_SIMPLE,
									CredentialName: "kubernetes-ingress://ingress-v1__/wakanda/test-com",
									CipherSuites:   []string{"ECDHE-RSA-AES128-GCM-SHA256"},
								},
							},
						},
					},
				},
				"bar.com": {
					Meta: config.Meta{
						GroupVersionKind: gvk.Gateway,
						Name:             "istio-autogenerated-k8s-ingress-" + common.CleanHost("bar.com"),
						Namespace:        "wakanda",
						Annotations: map[string]string{
							common.ClusterIdAnnotation: "ingress-v1",
							common.HostAnnotation:      "bar.com",
						},
					},
					Spec: &networking.Gateway{
						Servers: []*networking.Server{
							{
								Port: &networking.Port{
									Number:   80,
									Protocol: "HTTP",
									Name:     "http-80-ingress-ingress-v1",
								},
								Hosts: []string{"bar.com"},
							},
						},
					},
				},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			result := m.convertGateways(testCase.inputConfig)
			target := map[string]config.Config{}
			for _, item := range result {
				host := common.GetHost(item.Annotations)
				target[host] = item
			}
			assert.Equal(t, testCase.expect, target)
		})
	}
}

func TestConvertDestinationRuleForMcpBridgeMixedDestinationProtocols(t *testing.T) {
	fake := kube.NewFakeClient()
	baseOptions := common.Options{
		Enable:           true,
		ClusterId:        "gw-123-istio",
		RawClusterId:     "gw-123-istio__",
		GatewayHttpPort:  80,
		GatewayHttpsPort: 443,
	}

	testCases := []struct {
		name       string
		clusterID  cluster.ID
		controller common.IngressController
		wrapper    func(cluster.ID, bool) common.WrapperConfig
	}{
		{
			name:      "ingress v1beta1",
			clusterID: "ingress-v1beta1",
			controller: controllerv1beta1.NewController(fake, fake, common.Options{
				Enable:           true,
				ClusterId:        "ingress-v1beta1",
				RawClusterId:     "ingress-v1beta1__",
				GatewayHttpPort:  80,
				GatewayHttpsPort: 443,
			}, nil),
			wrapper: mcpBridgeIngressV1beta1Wrapper,
		},
		{
			name:      "ingress v1",
			clusterID: "ingress-v1",
			controller: controllerv1.NewController(fake, fake, common.Options{
				Enable:           true,
				ClusterId:        "ingress-v1",
				RawClusterId:     "ingress-v1__",
				GatewayHttpPort:  80,
				GatewayHttpsPort: 443,
			}, nil),
			wrapper: mcpBridgeIngressV1Wrapper,
		},
	}

	for _, testCase := range testCases {
		for _, destinationMode := range []struct {
			name      string
			withPorts bool
			ports     [3]uint32
		}{
			{name: "port level", withPorts: true, ports: [3]uint32{80, 443, 8443}},
			{name: "service level"},
		} {
			t.Run(testCase.name+"/"+destinationMode.name, func(t *testing.T) {
				m := NewIngressConfig(fake, nil, "wakanda", baseOptions)
				m.remoteIngressControllers = map[cluster.ID]common.IngressController{
					testCase.clusterID: testCase.controller,
				}

				wrapper := testCase.wrapper(testCase.clusterID, destinationMode.withPorts)
				result := m.convertDestinationRule([]common.WrapperConfig{wrapper})
				target := map[string]*networking.DestinationRule{}
				for _, item := range result {
					target[item.Spec.(*networking.DestinationRule).Host] = item.Spec.(*networking.DestinationRule)
				}
				if len(target) != 3 {
					t.Fatalf("destination rule count mismatch, want 3, got %d", len(target))
				}

				assertDestinationRuleTLSMode(t, target, "plain.example.internal", destinationMode.ports[0], nil)
				httpsMode := networking.ClientTLSSettings_SIMPLE
				assertDestinationRuleTLSMode(t, target, "secure.example.internal", destinationMode.ports[1], &httpsMode)
				assertDestinationRuleTLSMode(t, target, "noscheme.example.internal", destinationMode.ports[2], &httpsMode)
			})
		}
	}
}

func TestConstructBasicAuthEnvoyFilter(t *testing.T) {
	rules := &common.BasicAuthRules{
		Rules: []*common.Rule{
			{
				Realm:       "test",
				MatchRoute:  []string{"route"},
				Credentials: []string{"user:password"},
				Encrypted:   true,
			},
		},
	}

	config, err := constructBasicAuthEnvoyFilter(rules, "")
	if err != nil {
		t.Fatalf("construct error %v", err)
	}
	envoyFilter := config.Spec.(*networking.EnvoyFilter)
	pb, err := xds.BuildXDSObjectFromStruct(networking.EnvoyFilter_HTTP_FILTER, envoyFilter.ConfigPatches[0].Patch.Value, false)
	if err != nil {
		t.Fatalf("build object error %v", err)
	}
	target := proto.Clone(pb).(*httppb.HttpFilter)
	t.Log(target)
}

func ingressV1Wrapper(name, host, path string, sslPassthrough bool) common.WrapperConfig {
	wrapper := common.WrapperConfig{
		Config: &config.Config{
			Meta: config.Meta{
				Namespace: "default",
				Name:      name,
				Annotations: map[string]string{
					common.ClusterIdAnnotation: "ingress-v1",
				},
			},
			Spec: ingress.IngressSpec{
				Rules: []ingress.IngressRule{
					{
						Host: host,
						IngressRuleValue: ingress.IngressRuleValue{
							HTTP: &ingress.HTTPIngressRuleValue{
								Paths: []ingress.HTTPIngressPath{
									{
										Path: path,
										Backend: ingress.IngressBackend{
											Service: &ingress.IngressServiceBackend{
												Name: name,
												Port: ingress.ServiceBackendPort{Number: 443},
											},
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
			Match: &annotations.MatchConfig{},
		},
	}
	if sslPassthrough {
		wrapper.AnnotationsConfig.SSLPassthrough = &annotations.SSLPassthroughConfig{Enabled: true}
	}
	return wrapper
}

func mcpBridgeIngressV1Wrapper(clusterID cluster.ID, withPorts bool) common.WrapperConfig {
	apiGroup := "networking.higress.io"
	return common.WrapperConfig{
		Config: &config.Config{
			Meta: config.Meta{
				Namespace: "wakanda",
				Name:      "mcp-bridge-v1",
				Annotations: map[string]string{
					common.ClusterIdAnnotation: clusterID.String(),
				},
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
											Resource: &corev1.TypedLocalObjectReference{
												APIGroup: &apiGroup,
												Kind:     "McpBridge",
												Name:     "default",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		AnnotationsConfig: mcpBridgeIngressAnnotations(withPorts),
	}
}

func mcpBridgeIngressV1beta1Wrapper(clusterID cluster.ID, withPorts bool) common.WrapperConfig {
	apiGroup := "networking.higress.io"
	return common.WrapperConfig{
		Config: &config.Config{
			Meta: config.Meta{
				Namespace: "wakanda",
				Name:      "mcp-bridge-v1beta1",
				Annotations: map[string]string{
					common.ClusterIdAnnotation: clusterID.String(),
				},
			},
			Spec: ingressv1beta1.IngressSpec{
				Rules: []ingressv1beta1.IngressRule{
					{
						Host: "example.com",
						IngressRuleValue: ingressv1beta1.IngressRuleValue{
							HTTP: &ingressv1beta1.HTTPIngressRuleValue{
								Paths: []ingressv1beta1.HTTPIngressPath{
									{
										Path: "/",
										Backend: ingressv1beta1.IngressBackend{
											Resource: &corev1.TypedLocalObjectReference{
												APIGroup: &apiGroup,
												Kind:     "McpBridge",
												Name:     "default",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		AnnotationsConfig: mcpBridgeIngressAnnotations(withPorts),
	}
}

func mcpBridgeIngressAnnotations(withPorts bool) *annotations.Ingress {
	ports := [3]uint32{}
	protocols := map[string]string{
		"plain.example.internal:0":  "HTTP",
		"secure.example.internal:0": "HTTPS",
	}
	if withPorts {
		ports = [3]uint32{80, 443, 8443}
		protocols = map[string]string{
			"plain.example.internal:80":   "HTTP",
			"secure.example.internal:443": "HTTPS",
		}
	}

	return &annotations.Ingress{
		Destination: &annotations.DestinationConfig{
			McpDestination: []*networking.HTTPRouteDestination{
				{
					Destination: mcpDestination("plain.example.internal", ports[0]),
					Weight:      34,
				},
				{
					Destination: mcpDestination("secure.example.internal", ports[1]),
					Weight:      33,
				},
				{
					Destination: mcpDestination("noscheme.example.internal", ports[2]),
					Weight:      33,
				},
			},
			WeightSum: 100,
			Protocols: protocols,
		},
		UpstreamTLS: &annotations.UpstreamTLSConfig{BackendProtocol: "HTTPS"},
	}
}

func mcpDestination(host string, port uint32) *networking.Destination {
	destination := &networking.Destination{Host: host}
	if port != 0 {
		destination.Port = &networking.PortSelector{Number: port}
	}
	return destination
}

func assertDestinationRuleTLSMode(t *testing.T, rules map[string]*networking.DestinationRule, host string, port uint32, want *networking.ClientTLSSettings_TLSmode) {
	t.Helper()

	rule := rules[host]
	if rule == nil {
		t.Fatalf("missing destination rule for %s", host)
	}
	var tls *networking.ClientTLSSettings
	if port == 0 {
		if len(rule.TrafficPolicy.GetPortLevelSettings()) != 0 {
			t.Fatalf("%s unexpectedly generated port-level settings", host)
		}
		tls = rule.TrafficPolicy.GetTls()
	} else {
		if len(rule.TrafficPolicy.GetPortLevelSettings()) != 1 {
			t.Fatalf("%s port settings count mismatch, want 1, got %d", host, len(rule.TrafficPolicy.GetPortLevelSettings()))
		}

		portPolicy := rule.TrafficPolicy.GetPortLevelSettings()[0]
		if got := portPolicy.Port.GetNumber(); got != port {
			t.Fatalf("%s port mismatch, want %d, got %d", host, port, got)
		}
		tls = portPolicy.GetTls()
	}

	if want == nil {
		if tls != nil {
			t.Fatalf("%s unexpectedly enabled tls: %+v", host, tls)
		}
		return
	}

	if tls == nil {
		t.Fatalf("%s missing tls settings", host)
	}
	if got := tls.GetMode(); got != *want {
		t.Fatalf("%s tls mode mismatch, want %s, got %s", host, want.String(), got)
	}
}
