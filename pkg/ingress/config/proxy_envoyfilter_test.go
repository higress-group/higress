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

	"github.com/stretchr/testify/assert"
	networking "istio.io/api/networking/v1alpha3"

	higresscommon "github.com/alibaba/higress/v2/pkg/common"
	ingress "github.com/alibaba/higress/v2/pkg/ingress/kube/common"
)

// buildProxyCluster returns the cluster patch generated for a DNS service source bound to a proxy
// server, with the ECDH curves declared on the registry.
func buildProxyCluster(t *testing.T, ecdhCurves []string) map[string]interface{} {
	t.Helper()

	proxyWrappers := map[string]*ingress.ProxyWrapper{
		"provider-1-proxy": {
			ProxyName:    "provider-1-proxy",
			ListenerPort: 50001,
			EnvoyFilter:  &networking.EnvoyFilter{},
		},
	}
	serviceWrappers := map[string]*ingress.ServiceWrapper{
		"provider-1.dns": {
			ServiceName:  "provider-1",
			RegistryType: "dns",
			ServiceEntry: &networking.ServiceEntry{
				Hosts: []string{"provider-1.dns"},
				Ports: []*networking.ServicePort{
					{Number: 443, Name: "https", Protocol: "https"},
				},
			},
			ProxyConfig: &ingress.ServiceProxyConfig{
				ProxyName:          "provider-1-proxy",
				UpstreamProtocol:   higresscommon.HTTPS,
				UpstreamSni:        "sub2api.ccgate.top",
				UpstreamEcdhCurves: ecdhCurves,
			},
		},
	}

	filters := constructProxyEnvoyFilters(proxyWrappers, serviceWrappers, "higress-system")
	// One EnvoyFilter per proxy server plus one holding the patches for the services using it.
	if !assert.Len(t, filters, 2) {
		return nil
	}

	var cluster map[string]interface{}
	for _, filter := range filters {
		for _, patch := range filter.Spec.(*networking.EnvoyFilter).ConfigPatches {
			if patch.ApplyTo == networking.EnvoyFilter_CLUSTER &&
				patch.Patch.Operation == networking.EnvoyFilter_Patch_ADD {
				cluster = patch.Patch.Value.AsMap()
			}
		}
	}
	if !assert.NotNil(t, cluster) {
		return nil
	}
	return cluster
}

// assertProxyClusterBasics asserts what must hold for a proxied cluster regardless of the curves.
func assertProxyClusterBasics(t *testing.T, cluster map[string]interface{}) {
	t.Helper()

	assert.Equal(t, "outbound|443||provider-1.dns", cluster["name"])

	// The cluster must still point at the local proxy listener.
	lbEndpoints := cluster["load_assignment"].(map[string]interface{})["endpoints"].([]interface{})
	endpoint := lbEndpoints[0].(map[string]interface{})["lb_endpoints"].([]interface{})[0].(map[string]interface{})["endpoint"].(map[string]interface{})
	socketAddress := endpoint["address"].(map[string]interface{})["socket_address"].(map[string]interface{})
	assert.Equal(t, "127.0.0.1", socketAddress["address"])
	assert.Equal(t, float64(50001), socketAddress["port_value"])

	// The upstream SNI is what the local proxy listener uses as the CONNECT target.
	typedConfig := cluster["transport_socket"].(map[string]interface{})["typed_config"].(map[string]interface{})
	assert.Equal(t, "sub2api.ccgate.top", typedConfig["sni"])
}

// TestConstructProxyEnvoyFiltersTlsParams verifies that the cluster rebuilt for a proxied HTTPS
// service is left untouched when the registry does not declare any ECDH curve.
func TestConstructProxyEnvoyFiltersTlsParams(t *testing.T) {
	cluster := buildProxyCluster(t, nil)
	if cluster == nil {
		return
	}
	assertProxyClusterBasics(t, cluster)

	// Not configured: Envoy keeps using the defaults of its linked TLS library.
	typedConfig := cluster["transport_socket"].(map[string]interface{})["typed_config"].(map[string]interface{})
	assert.NotContains(t, typedConfig, "common_tls_context")
}

// TestConstructProxyEnvoyFiltersConfiguredCurves verifies that the curves declared on the registry
// are emitted in the TLS context, with unsupported names dropped.
func TestConstructProxyEnvoyFiltersConfiguredCurves(t *testing.T) {
	cluster := buildProxyCluster(t, []string{"X25519", "P-256", "P-384", "X25519Kyber768Draft00", "bogus"})
	if cluster == nil {
		return
	}
	assertProxyClusterBasics(t, cluster)

	typedConfig := cluster["transport_socket"].(map[string]interface{})["typed_config"].(map[string]interface{})
	tlsParams := typedConfig["common_tls_context"].(map[string]interface{})["tls_params"].(map[string]interface{})
	assert.Equal(t, []interface{}{"X25519", "P-256", "P-384"}, tlsParams["ecdh_curves"])
}

func TestResolveUpstreamEcdhCurves(t *testing.T) {
	// Not configured, or nothing usable: no curve is emitted at all.
	assert.Nil(t, resolveUpstreamEcdhCurves(nil))
	assert.Nil(t, resolveUpstreamEcdhCurves([]string{}))
	assert.Nil(t, resolveUpstreamEcdhCurves([]string{"", "P-1024"}))

	// Unsupported names are dropped instead of breaking the whole configuration. Names that are
	// accepted by istio's ValidECDHCurves but not supported by every TLS library are dropped too:
	// letting them through would make the transport socket fail to initialize, which rejects the
	// cluster configuration and breaks every service behind the cluster.
	assert.Equal(t, []string{"P-256", "P-384"}, resolveUpstreamEcdhCurves([]string{"P-256", "bogus", "P-384"}))
	assert.Nil(t, resolveUpstreamEcdhCurves([]string{"X25519Kyber768Draft00"}))
	assert.Nil(t, resolveUpstreamEcdhCurves([]string{"X25519MLKEM768"}))
	assert.Equal(t, []string{"P-521", "X25519"}, resolveUpstreamEcdhCurves([]string{"P-521", "X25519"}))
}
