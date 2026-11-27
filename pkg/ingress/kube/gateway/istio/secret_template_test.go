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

package istio

import (
	"encoding/json"
	"strings"
	"testing"

	istio "istio.io/api/networking/v1alpha3"
	"istio.io/istio/pkg/config"
	"istio.io/istio/pkg/config/schema/gvk"
	"istio.io/istio/pkg/config/schema/kind"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	k8s "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/alibaba/higress/v2/pkg/ingress/kube/util"
)

// convertTenantHTTPRoute runs the real Gateway API conversion for an HTTPRoute in namespace
// tenantNamespace whose RequestHeaderModifier values carry the given secret references, and
// then applies the ownership restriction computeRoute applies to every converted route.
func convertTenantHTTPRoute(t *testing.T, tenantNamespace string, set ...k8s.HTTPHeader) *istio.HTTPRoute {
	t.Helper()
	// No backend refs: a real destination is resolved through krt, which a zero RouteContext
	// cannot serve. The header filter is the tenant-writable string under test.
	rule := k8s.HTTPRouteRule{
		Filters: []k8s.HTTPRouteFilter{{
			Type:                  k8s.HTTPRouteFilterRequestHeaderModifier,
			RequestHeaderModifier: &k8s.HTTPHeaderFilter{Set: set},
		}},
	}
	obj := &k8s.HTTPRoute{ObjectMeta: metav1.ObjectMeta{
		Name:      "tenant-route",
		Namespace: tenantNamespace,
	}}

	route, _, _ := convertHTTPRoute(RouteContext{}, rule, obj, 0, false)
	if route == nil {
		t.Fatal("convertHTTPRoute() route is nil")
	}
	if route.Headers == nil || route.Headers.Request == nil {
		t.Fatalf("convertHTTPRoute() produced no request headers: %v", route.Headers)
	}
	if err := defuseRouteSecretTemplates(obj, route); err != nil {
		t.Fatalf("defuseRouteSecretTemplates() error = %v", err)
	}
	return route
}

// headerValue reads a generated header. The conversion lowercases Gateway API header names,
// so the tests use the normalized form.
func headerValue(t *testing.T, route *istio.HTTPRoute, name string) string {
	t.Helper()
	value, ok := route.Headers.Request.Set[name]
	if !ok {
		t.Fatalf("generated route has no header %q, has %v", name, route.Headers.Request.Set)
	}
	return value
}

// specJSON serializes a generated spec the way ProcessConfig does, so that tests assert
// against the text template resolution actually scans.
func specJSON(t *testing.T, spec any) string {
	t.Helper()
	jsonBytes, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("failed to marshal generated spec: %v", err)
	}
	return string(jsonBytes)
}

// TestDefuseRouteSecretTemplates pins the Gateway API half of the secret template ownership
// fix. A tenant writes references into an HTTPRoute, and because generated VirtualServices
// are merged across namespaces while configs derived from routes are re-stamped with the
// Higress system namespace, the restriction has to be applied during conversion, while the
// route's own namespace is still known.
func TestDefuseRouteSecretTemplates(t *testing.T) {
	t.Run("references are restricted to the route namespace", func(t *testing.T) {
		route := convertTenantHTTPRoute(t, "tenant-a",
			k8s.HTTPHeader{Name: "x-own", Value: "${secret.tenant-a/app.api_key}"},
			k8s.HTTPHeader{Name: "x-short", Value: "${secret.app.api_key}"},
			k8s.HTTPHeader{Name: "x-foreign", Value: "${secret.higress-system/db.password}"},
			k8s.HTTPHeader{Name: "x-other-tenant", Value: "${secret.tenant-b/app.api_key}"},
			k8s.HTTPHeader{Name: "x-plain", Value: "static-value"},
		)

		// An explicit same-namespace reference and plain text survive byte identically.
		if got := headerValue(t, route, "x-own"); got != "${secret.tenant-a/app.api_key}" {
			t.Errorf("x-own = %q, want the reference untouched", got)
		}
		if got := headerValue(t, route, "x-plain"); got != "static-value" {
			t.Errorf("x-plain = %q, want %q", got, "static-value")
		}
		// The short form means the route's own namespace, so it is expanded rather than
		// refused: left alone it would resolve against the Higress system namespace.
		if got, want := headerValue(t, route, "x-short"), "${secret.tenant-a/app.api_key}"; got != want {
			t.Errorf("x-short = %q, want %q", got, want)
		}
		// Everything pointing elsewhere is replaced with the opaque placeholder.
		for _, header := range []string{"x-foreign", "x-other-tenant"} {
			if got := headerValue(t, route, header); got != util.RefusedReferencePlaceholder {
				t.Errorf("%s = %q, want %q", header, got, util.RefusedReferencePlaceholder)
			}
		}
	})

	t.Run("no foreign namespace survives in the generated route", func(t *testing.T) {
		route := convertTenantHTTPRoute(t, "tenant-a",
			k8s.HTTPHeader{Name: "x-short", Value: "${secret.db.password}"},
			k8s.HTTPHeader{Name: "x-foreign", Value: "${secret.higress-system/db.password}"},
		)

		// The generated route is what the config store serializes and what template
		// resolution scans, so check all of it rather than the headers alone.
		generated := specJSON(t, route)
		for _, namespace := range []string{"higress-system", "tenant-b"} {
			if ref := "${secret." + namespace + "/"; strings.Contains(generated, ref) {
				t.Errorf("generated route still references namespace %s: %s", namespace, generated)
			}
		}
		// The one reference left is the tenant's own, and it names the namespace explicitly
		// so that merging it under another namespace later cannot redirect it.
		if !strings.Contains(generated, "${secret.tenant-a/db.password}") {
			t.Errorf("generated route lost the tenant's own reference: %s", generated)
		}
	})

	t.Run("the placeholder is opaque even for a hostile reference", func(t *testing.T) {
		// Parts of a reference may themselves contain '$' and '{'. The placeholder must not
		// interpolate them, or the defused value could match as a reference again.
		for _, value := range []string{
			"${secret.tenant-b/a${secret.higress-system/db.password}",
			"$${secret.tenant-b/app.api_key}",
			"${secret.tenant-b/app.api_key}${secret.higress-system/db.password}",
		} {
			route := convertTenantHTTPRoute(t, "tenant-a", k8s.HTTPHeader{Name: "x-v", Value: value})
			got := headerValue(t, route, "x-v")
			if matches := util.TemplateRegex.FindAllString(got, -1); len(matches) > 0 {
				t.Errorf("defused value %q still contains references %v", value, matches)
			}
			if strings.Contains(got, "tenant-b") || strings.Contains(got, "higress-system") {
				t.Errorf("defused value %q = %q, leaked the refused reference", value, got)
			}
			// The property that matters for reassembly: nothing left in the value can start a
			// reference again, whatever the surrounding text contributed.
			if strings.Contains(got, "${") {
				t.Errorf("defused value %q = %q, still contains a reference opener", value, got)
			}
			if !strings.Contains(got, util.RefusedReferencePlaceholder) {
				t.Errorf("defused value %q = %q, expected the opaque placeholder", value, got)
			}
		}
	})
}

// backendPolicyTarget is the Service a BackendTLSPolicy or BackendTrafficPolicy in
// tenantNamespace attaches to. Local policies can only target their own namespace, which is
// why the merged DestinationRule is stamped with the target's namespace.
func backendPolicyTarget(tenantNamespace string) TypedNamespacedName {
	return TypedNamespacedName{
		NamespacedName: k8stypes.NamespacedName{Namespace: tenantNamespace, Name: "reviews"},
		Kind:           kind.Service,
	}
}

// TestDefuseBackendPolicySecretTemplates pins the DestinationRule half of the fix. The
// backend policy merge takes TLS and load balancer settings straight out of tenant-written
// BackendTLSPolicy and BackendTrafficPolicy objects and stamps the result with the target's
// namespace, so without the restriction a tenant could put a reference in a value that is
// served back to clients, or in one that changes what the gateway connects to.
//
// The specs here are shaped like the merge output rather than driven through
// DestinationRuleCollection, which needs a full krt harness. What is under test is
// defuseBackendPolicySecretTemplates, the function the merge calls on the spec it assembled
// before stamping target.Namespace onto it.
func TestDefuseBackendPolicySecretTemplates(t *testing.T) {
	t.Run("a consistent hash cookie name cannot echo a foreign secret", func(t *testing.T) {
		// HttpCookie.Name is turned into a Set-Cookie header by Envoy, so a resolved
		// reference there is handed to every client that reaches the route.
		spec := &istio.DestinationRule{
			Host: "reviews.tenant-a.svc.cluster.local",
			TrafficPolicy: &istio.TrafficPolicy{
				LoadBalancer: &istio.LoadBalancerSettings{
					LbPolicy: &istio.LoadBalancerSettings_ConsistentHash{
						ConsistentHash: &istio.LoadBalancerSettings_ConsistentHashLB{
							HashKey: &istio.LoadBalancerSettings_ConsistentHashLB_HttpCookie{
								HttpCookie: &istio.LoadBalancerSettings_ConsistentHashLB_HTTPCookie{
									Name: "${secret.higress-system/creds.token}",
									Path: "/session",
								},
							},
						},
					},
				},
			},
		}

		if err := defuseBackendPolicySecretTemplates(spec, backendPolicyTarget("tenant-a")); err != nil {
			t.Fatalf("defuseBackendPolicySecretTemplates() error = %v", err)
		}
		cookie := spec.GetTrafficPolicy().GetLoadBalancer().GetConsistentHash().GetHttpCookie()
		if cookie.GetName() != util.RefusedReferencePlaceholder {
			t.Errorf("HttpCookie.Name = %q, want %q", cookie.GetName(), util.RefusedReferencePlaceholder)
		}
		if cookie.GetPath() != "/session" {
			t.Errorf("HttpCookie.Path = %q, want it left alone", cookie.GetPath())
		}
	})

	t.Run("backend TLS settings cannot redirect the upstream handshake", func(t *testing.T) {
		// SNI and the subject alternative names come from BackendTLSPolicy validation fields.
		// A resolved reference in either would let a tenant name the peer identity the gateway
		// accepts, and the value is not otherwise visible to them.
		spec := &istio.DestinationRule{
			Host: "reviews.tenant-a.svc.cluster.local",
			TrafficPolicy: &istio.TrafficPolicy{
				Tls: &istio.ClientTLSSettings{
					Mode: istio.ClientTLSSettings_MUTUAL,
					Sni:  "${secret.tenant-a/app.sni}",
					SubjectAltNames: []string{
						"spiffe://${secret.higress-system/creds.token}",
						"spiffe://cluster.local/ns/tenant-a/sa/reviews",
					},
				},
			},
		}

		if err := defuseBackendPolicySecretTemplates(spec, backendPolicyTarget("tenant-a")); err != nil {
			t.Fatalf("defuseBackendPolicySecretTemplates() error = %v", err)
		}
		tls := spec.GetTrafficPolicy().GetTls()
		if got, want := tls.GetSni(), "${secret.tenant-a/app.sni}"; got != want {
			t.Errorf("Tls.Sni = %q, want the tenant's own reference %q", got, want)
		}
		if got, want := tls.GetSubjectAltNames()[0], "spiffe://"+util.RefusedReferencePlaceholder; got != want {
			t.Errorf("Tls.SubjectAltNames[0] = %q, want %q", got, want)
		}
		if got := tls.GetSubjectAltNames()[1]; got != "spiffe://cluster.local/ns/tenant-a/sa/reviews" {
			t.Errorf("Tls.SubjectAltNames[1] = %q, want it left alone", got)
		}
		if tls.GetMode() != istio.ClientTLSSettings_MUTUAL {
			t.Errorf("Tls.Mode = %v, want it preserved", tls.GetMode())
		}
	})

	t.Run("a short form reference is bound to the policy namespace", func(t *testing.T) {
		spec := &istio.DestinationRule{
			Host: "reviews.tenant-a.svc.cluster.local",
			TrafficPolicy: &istio.TrafficPolicy{
				Tls: &istio.ClientTLSSettings{Sni: "${secret.app.sni}"},
			},
		}

		if err := defuseBackendPolicySecretTemplates(spec, backendPolicyTarget("tenant-a")); err != nil {
			t.Fatalf("defuseBackendPolicySecretTemplates() error = %v", err)
		}
		// Left short, the reference would be resolved against the namespace of the generated
		// rule by ProcessConfig, not against the namespace of the policy that wrote it.
		if got, want := spec.GetTrafficPolicy().GetTls().GetSni(), "${secret.tenant-a/app.sni}"; got != want {
			t.Errorf("Tls.Sni = %q, want %q", got, want)
		}
	})

	t.Run("no foreign namespace survives in the generated rule", func(t *testing.T) {
		spec := &istio.DestinationRule{
			Host: "reviews.tenant-a.svc.cluster.local",
			TrafficPolicy: &istio.TrafficPolicy{
				Tls: &istio.ClientTLSSettings{
					Sni:             "${secret.tenant-b/app.sni}",
					CredentialName:  "${secret.higress-system/creds.token}",
					SubjectAltNames: []string{"${secret.kube-system/default.token}"},
				},
			},
		}

		if err := defuseBackendPolicySecretTemplates(spec, backendPolicyTarget("tenant-a")); err != nil {
			t.Fatalf("defuseBackendPolicySecretTemplates() error = %v", err)
		}
		generated := specJSON(t, spec)
		for _, namespace := range []string{"tenant-b", "higress-system", "kube-system"} {
			if strings.Contains(generated, "${secret."+namespace+"/") {
				t.Errorf("generated rule still references namespace %s: %s", namespace, generated)
			}
		}
		if strings.Contains(generated, "${") {
			t.Errorf("generated rule still contains a reference: %s", generated)
		}
	})
}

// TestDefuseGatewaySecretTemplates covers the listener half for both object kinds that produce
// an Istio Gateway config. ListenerSetCollection builds the same config from a different
// tenant-owned object than GatewayCollection does, so both have to restrict it.
func TestDefuseGatewaySecretTemplates(t *testing.T) {
	for _, owner := range []config.GroupVersionKind{gvk.KubernetesGateway, gvk.ListenerSet} {
		t.Run(owner.Kind, func(t *testing.T) {
			spec := &istio.Gateway{
				Servers: []*istio.Server{{
					Hosts: []string{"${secret.higress-system/creds.token}.example.com"},
					Port:  &istio.Port{Number: 443, Protocol: "HTTPS", Name: "https"},
					Tls: &istio.ServerTLSSettings{
						Mode:            istio.ServerTLSSettings_SIMPLE,
						CredentialName:  "${secret.higress-system/creds.token}",
						SubjectAltNames: []string{"${secret.tenant-a/app.san}"},
					},
				}},
				Selector: map[string]string{"istio": "higress-gateway"},
			}

			if err := defuseGatewaySecretTemplates(owner, "tenant-a", "tenant-gateway", spec); err != nil {
				t.Fatalf("defuseGatewaySecretTemplates() error = %v", err)
			}
			server := spec.GetServers()[0]
			if got := server.GetTls().GetCredentialName(); got != util.RefusedReferencePlaceholder {
				t.Errorf("Tls.CredentialName = %q, want %q", got, util.RefusedReferencePlaceholder)
			}
			if got := server.GetHosts()[0]; got != util.RefusedReferencePlaceholder+".example.com" {
				t.Errorf("Hosts[0] = %q, want the refused reference to stay inert inside the host", got)
			}
			if got, want := server.GetTls().GetSubjectAltNames()[0], "${secret.tenant-a/app.san}"; got != want {
				t.Errorf("Tls.SubjectAltNames[0] = %q, want the tenant's own reference %q", got, want)
			}
			// Structural fields the listener depends on must survive the rewrite.
			if server.GetPort().GetNumber() != 443 || server.GetPort().GetProtocol() != "HTTPS" {
				t.Errorf("Port = %v, want it preserved", server.GetPort())
			}
			if got := spec.GetSelector()["istio"]; got != "higress-gateway" {
				t.Errorf("Selector = %v, want it preserved", spec.GetSelector())
			}
		})
	}
}
