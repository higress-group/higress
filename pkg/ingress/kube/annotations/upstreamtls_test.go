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
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	networking "istio.io/api/networking/v1alpha3"
)

func TestUpstreamTLSParse(t *testing.T) {
	parser := upstreamTLS{}

	testCases := []struct {
		input  Annotations
		expect *UpstreamTLSConfig
	}{
		{
			input:  Annotations{},
			expect: nil,
		},
		{
			input: Annotations{
				buildNginxAnnotationKey(backendProtocol): "HTTP",
			},
			expect: nil,
		},
		{
			input: Annotations{
				buildNginxAnnotationKey(proxySSLSecret):     "",
				buildNginxAnnotationKey(backendProtocol):    "HTTP2",
				buildNginxAnnotationKey(proxySSLSecret):     "namespace/SSLSecret",
				buildNginxAnnotationKey(proxySSLVerify):     "on",
				buildNginxAnnotationKey(proxySSLName):       "SSLName",
				buildNginxAnnotationKey(proxySSLServerName): "on",
			},
			expect: &UpstreamTLSConfig{
				BackendProtocol: "HTTP2",
				SSLVerify:       true,
				SNI:             "SSLName",
				SecretName:      "namespace/SSLSecret",
				EnableSNI:       true,
			},
		},
		{
			input: Annotations{
				buildNginxAnnotationKey(proxySSLSecret):     "",
				buildNginxAnnotationKey(backendProtocol):    "HTTP2",
				buildNginxAnnotationKey(proxySSLSecret):     "", // if there is no ssl secret, it will be return directly
				buildNginxAnnotationKey(proxySSLVerify):     "on",
				buildNginxAnnotationKey(proxySSLName):       "SSLName",
				buildNginxAnnotationKey(proxySSLServerName): "on",
			},
			expect: &UpstreamTLSConfig{
				BackendProtocol: "HTTP2",
				SSLVerify:       true,
				SNI:             "SSLName",
				SecretName:      "",
				EnableSNI:       true,
			},
		},
	}

	for _, testCase := range testCases {
		t.Run("", func(t *testing.T) {
			config := &Ingress{}
			_ = parser.Parse(testCase.input, config, nil)
			if diff := cmp.Diff(testCase.expect, config.UpstreamTLS); diff != "" {
				t.Fatalf("TestUpstreamTLSParse() mismatch: \n%s", diff)
			}
		})
	}
}

func TestApplyTrafficPolicy(t *testing.T) {
	parser := upstreamTLS{}

	testCases := []struct {
		input  *networking.TrafficPolicy_PortTrafficPolicy
		config *Ingress
		expect *networking.TrafficPolicy_PortTrafficPolicy
	}{
		{
			input: &networking.TrafficPolicy_PortTrafficPolicy{},
			config: &Ingress{
				UpstreamTLS: &UpstreamTLSConfig{},
			},
			expect: &networking.TrafficPolicy_PortTrafficPolicy{},
		},
		{
			input: &networking.TrafficPolicy_PortTrafficPolicy{},
			config: &Ingress{
				UpstreamTLS: &UpstreamTLSConfig{
					BackendProtocol: "HTTP2",
				},
			},
			expect: &networking.TrafficPolicy_PortTrafficPolicy{
				ConnectionPool: &networking.ConnectionPoolSettings{
					Http: &networking.ConnectionPoolSettings_HTTPSettings{
						H2UpgradePolicy: networking.ConnectionPoolSettings_HTTPSettings_UPGRADE,
					},
				},
			},
		},
		{
			input: &networking.TrafficPolicy_PortTrafficPolicy{},
			config: &Ingress{
				UpstreamTLS: &UpstreamTLSConfig{
					BackendProtocol: "HTTPS",
					EnableSNI:       true,
					SNI:             "SNI",
				},
			},
			expect: &networking.TrafficPolicy_PortTrafficPolicy{
				Tls: &networking.ClientTLSSettings{
					Mode: networking.ClientTLSSettings_SIMPLE,
					Sni:  "SNI",
				},
			},
		},
		{
			input: &networking.TrafficPolicy_PortTrafficPolicy{},
			config: &Ingress{
				UpstreamTLS: &UpstreamTLSConfig{
					SecretName: "namespace/secretName",
					SSLVerify:  true,
				},
			},
			expect: &networking.TrafficPolicy_PortTrafficPolicy{
				Tls: &networking.ClientTLSSettings{
					Mode:           networking.ClientTLSSettings_MUTUAL,
					CredentialName: "kubernetes-ingress://Kubernetes/namespace/secretName",
				},
			},
		},
	}
	unexportedIgnoredTypes := []interface{}{
		networking.TrafficPolicy_PortTrafficPolicy{},
		networking.ClientTLSSettings{},
		networking.ConnectionPoolSettings{},
		networking.ConnectionPoolSettings_HTTPSettings{},
	}

	for _, testCase := range testCases {
		t.Run("", func(t *testing.T) {
			parser.ApplyTrafficPolicy(nil, testCase.input, testCase.config)
			if diff := cmp.Diff(testCase.expect, testCase.input, cmpopts.IgnoreUnexported(unexportedIgnoredTypes...)); diff != "" {
				t.Fatalf("TestApplyTrafficPolicy() mismatch (-want +got): \n%s", diff)
			}
		})
	}
}
