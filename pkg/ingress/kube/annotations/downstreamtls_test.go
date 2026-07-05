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
	"reflect"
	"testing"

	networking "istio.io/api/networking/v1alpha3"
	"k8s.io/apimachinery/pkg/types"
)

var parser = downstreamTLS{}

func TestParse(t *testing.T) {
	testCases := []struct {
		name   string
		input  map[string]string
		expect *DownstreamTLSConfig
	}{
		{
			name: "empty config",
		},
		{
			name: "ssl cipher only",
			input: map[string]string{
				buildNginxAnnotationKey(sslCipher): "ECDHE-RSA-AES256-GCM-SHA384:AES128-SHA",
			},
			expect: &DownstreamTLSConfig{
				Mode:         networking.ServerTLSSettings_SIMPLE,
				CipherSuites: []string{"ECDHE-RSA-AES256-GCM-SHA384", "AES128-SHA"},
			},
		},
		{
			name: "with TLS version config",
			input: map[string]string{
				buildNginxAnnotationKey(annotationMinTLSVersion): "TLSv1.2",
				buildNginxAnnotationKey(annotationMaxTLSVersion): "TLSv1.3",
			},
			expect: &DownstreamTLSConfig{
				Mode:       networking.ServerTLSSettings_SIMPLE,
				MinVersion: "TLSv1.2",
				MaxVersion: "TLSv1.3",
			},
		},
		{
			name: "complete config",
			input: map[string]string{
				buildNginxAnnotationKey(authTLSSecret):           "test",
				buildNginxAnnotationKey(sslCipher):               "ECDHE-RSA-AES256-GCM-SHA384:AES128-SHA",
				buildNginxAnnotationKey(annotationMinTLSVersion): "TLSv1.2",
				buildNginxAnnotationKey(annotationMaxTLSVersion): "TLSv1.3",
			},
			expect: &DownstreamTLSConfig{
				CASecretName: types.NamespacedName{
					Namespace: "foo",
					Name:      "test",
				},
				Mode:         networking.ServerTLSSettings_MUTUAL,
				CipherSuites: []string{"ECDHE-RSA-AES256-GCM-SHA384", "AES128-SHA"},
				MinVersion:   "TLSv1.2",
				MaxVersion:   "TLSv1.3",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := &Ingress{
				Meta: Meta{
					Namespace: "foo",
				},
			}
			err := parser.Parse(tc.input, config, nil)
			if err != nil {
				t.Fatalf("Parse failed: %v", err)
			}
			if !reflect.DeepEqual(tc.expect, config.DownstreamTLS) {
				t.Fatalf("Parse result mismatch:\nExpect: %+v\nGot: %+v", tc.expect, config.DownstreamTLS)
			}
		})
	}
}

func TestConvertTLSVersion(t *testing.T) {
	testCases := []struct {
		name    string
		version string
		expect  networking.ServerTLSSettings_TLSProtocol
		wantErr bool
	}{
		{
			name:    "TLS 1.0",
			version: "TLSv1.0",
			expect:  networking.ServerTLSSettings_TLSV1_0,
		},
		{
			name:    "TLS 1.1",
			version: "TLSv1.1",
			expect:  networking.ServerTLSSettings_TLSV1_1,
		},
		{
			name:    "TLS 1.2",
			version: "TLSv1.2",
			expect:  networking.ServerTLSSettings_TLSV1_2,
		},
		{
			name:    "TLS 1.3",
			version: "TLSv1.3",
			expect:  networking.ServerTLSSettings_TLSV1_3,
		},
		{
			name:    "invalid version",
			version: "invalid",
			expect:  networking.ServerTLSSettings_TLS_AUTO,
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := convertTLSVersion(tc.version)
			if tc.wantErr {
				if err == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if result != tc.expect {
					t.Errorf("Expected %v but got %v", tc.expect, result)
				}
			}
		})
	}
}

func TestApplyGateway(t *testing.T) {
	testCases := []struct {
		name   string
		input  *networking.Gateway
		config *Ingress
		expect *networking.Gateway
	}{
		{
			name: "apply TLS version",
			input: &networking.Gateway{
				Servers: []*networking.Server{
					{
						Port: &networking.Port{
							Protocol: "HTTPS",
						},
						Tls: &networking.ServerTLSSettings{
							Mode: networking.ServerTLSSettings_SIMPLE,
						},
					},
				},
			},
			config: &Ingress{
				DownstreamTLS: &DownstreamTLSConfig{
					MinVersion: "TLSv1.2",
					MaxVersion: "TLSv1.3",
				},
			},
			expect: &networking.Gateway{
				Servers: []*networking.Server{
					{
						Port: &networking.Port{
							Protocol: "HTTPS",
						},
						Tls: &networking.ServerTLSSettings{
							Mode:               networking.ServerTLSSettings_SIMPLE,
							MinProtocolVersion: networking.ServerTLSSettings_TLSV1_2,
							MaxProtocolVersion: networking.ServerTLSSettings_TLSV1_3,
						},
					},
				},
			},
		},
		{
			name: "complete config",
			input: &networking.Gateway{
				Servers: []*networking.Server{
					{
						Port: &networking.Port{
							Protocol: "HTTPS",
						},
						Tls: &networking.ServerTLSSettings{
							Mode:           networking.ServerTLSSettings_SIMPLE,
							CredentialName: "kubernetes-ingress://cluster/foo/bar",
						},
					},
				},
			},
			config: &Ingress{
				DownstreamTLS: &DownstreamTLSConfig{
					CASecretName: types.NamespacedName{
						Namespace: "foo",
						Name:      "bar",
					},
					Mode:         networking.ServerTLSSettings_MUTUAL,
					CipherSuites: []string{"ECDHE-RSA-AES256-GCM-SHA384"},
					MinVersion:   "TLSv1.2",
					MaxVersion:   "TLSv1.3",
				},
			},
			expect: &networking.Gateway{
				Servers: []*networking.Server{
					{Port: &networking.Port{
						Protocol: "HTTPS",
					},
						Tls: &networking.ServerTLSSettings{
							CredentialName:     "kubernetes-ingress://cluster/foo/bar",
							Mode:               networking.ServerTLSSettings_MUTUAL,
							CipherSuites:       []string{"ECDHE-RSA-AES256-GCM-SHA384"},
							MinProtocolVersion: networking.ServerTLSSettings_TLSV1_2,
							MaxProtocolVersion: networking.ServerTLSSettings_TLSV1_3,
						},
					},
				},
			},
		},
		{
			name: "invalid TLS version",
			input: &networking.Gateway{
				Servers: []*networking.Server{
					{
						Port: &networking.Port{
							Protocol: "HTTPS",
						},
						Tls: &networking.ServerTLSSettings{
							Mode: networking.ServerTLSSettings_SIMPLE,
						},
					},
				},
			},
			config: &Ingress{
				DownstreamTLS: &DownstreamTLSConfig{
					MinVersion: "invalid",
					MaxVersion: "invalid",
				},
			},
			expect: &networking.Gateway{
				Servers: []*networking.Server{
					{
						Port: &networking.Port{
							Protocol: "HTTPS",
						},
						Tls: &networking.ServerTLSSettings{
							Mode: networking.ServerTLSSettings_SIMPLE,
							// Invalid versions should default to TLS_AUTO
							MinProtocolVersion: networking.ServerTLSSettings_TLS_AUTO,
							MaxProtocolVersion: networking.ServerTLSSettings_TLS_AUTO,
						},
					},
				},
			},
		},
		{
			name: "skip passthrough server",
			input: &networking.Gateway{
				Servers: []*networking.Server{
					{
						Port: &networking.Port{
							Protocol: "TLS",
						},
						Tls: &networking.ServerTLSSettings{
							Mode: networking.ServerTLSSettings_PASSTHROUGH,
						},
					},
				},
			},
			config: &Ingress{
				DownstreamTLS: &DownstreamTLSConfig{
					CipherSuites: []string{"ECDHE-RSA-AES256-GCM-SHA384"},
					MinVersion:   "TLSv1.2",
					MaxVersion:   "TLSv1.3",
				},
			},
			expect: &networking.Gateway{
				Servers: []*networking.Server{
					{
						Port: &networking.Port{
							Protocol: "TLS",
						},
						Tls: &networking.ServerTLSSettings{
							Mode: networking.ServerTLSSettings_PASSTHROUGH,
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			parser.ApplyGateway(tc.input, tc.config)
			if !reflect.DeepEqual(tc.input, tc.expect) {
				t.Fatalf("ApplyGateway result mismatch for %s:\nExpect: %+v\nGot: %+v",
					tc.name, tc.expect, tc.input)
			}
		})
	}
}

func TestNeedDownstreamTLS(t *testing.T) {
	testCases := []struct {
		name        string
		annotations map[string]string
		expect      bool
	}{
		{
			name:        "empty annotations",
			annotations: map[string]string{},
			expect:      false,
		},
		{
			name: "with ssl cipher",
			annotations: map[string]string{
				buildNginxAnnotationKey(sslCipher): "ECDHE-RSA-AES256-GCM-SHA384",
			},
			expect: true,
		},
		{
			name: "with TLS version",
			annotations: map[string]string{
				buildNginxAnnotationKey(annotationMinTLSVersion): "TLSv1.2",
			},
			expect: true,
		},
		{
			name: "with multiple TLS configs",
			annotations: map[string]string{
				buildNginxAnnotationKey(sslCipher):               "ECDHE-RSA-AES256-GCM-SHA384",
				buildNginxAnnotationKey(annotationMinTLSVersion): "TLSv1.2",
				buildNginxAnnotationKey(annotationMaxTLSVersion): "TLSv1.3",
			},
			expect: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := needDownstreamTLS(tc.annotations)
			if result != tc.expect {
				t.Errorf("needDownstreamTLS() for %s = %v, want %v",
					tc.name, result, tc.expect)
			}
		})
	}
}
