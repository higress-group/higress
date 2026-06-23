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

import "testing"

func TestSSLPassthroughParse(t *testing.T) {
	testCases := []struct {
		name    string
		input   Annotations
		enabled bool
		exists  bool
	}{
		{
			name:  "missing",
			input: Annotations{},
		},
		{
			name: "enabled by nginx annotation",
			input: Annotations{
				buildNginxAnnotationKey(sslPassthroughAnnotation): "true",
			},
			enabled: true,
			exists:  true,
		},
		{
			name: "enabled by higress annotation",
			input: Annotations{
				buildHigressAnnotationKey(sslPassthroughAnnotation): "true",
			},
			enabled: true,
			exists:  true,
		},
		{
			name: "disabled by nginx annotation",
			input: Annotations{
				buildNginxAnnotationKey(sslPassthroughAnnotation): "false",
			},
			exists: true,
		},
		{
			name: "disabled by higress annotation",
			input: Annotations{
				buildHigressAnnotationKey(sslPassthroughAnnotation): "false",
			},
			exists: true,
		},
	}

	parser := sslPassthrough{}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := &Ingress{}
			if err := parser.Parse(tc.input, config, nil); err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if tc.exists && config.SSLPassthrough == nil {
				t.Fatal("expected ssl passthrough config")
			}
			if !tc.exists && config.SSLPassthrough != nil {
				t.Fatal("unexpected ssl passthrough config")
			}
			if tc.exists && config.SSLPassthrough.Enabled != tc.enabled {
				t.Fatalf("enabled mismatch, want %v, got %v", tc.enabled, config.SSLPassthrough.Enabled)
			}
		})
	}
}

func TestSSLPassthroughDoesNotSetUpstreamTLS(t *testing.T) {
	parser := sslPassthrough{}
	config := &Ingress{}
	err := parser.Parse(Annotations{
		buildNginxAnnotationKey(sslPassthroughAnnotation): "true",
	}, config, nil)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if config.UpstreamTLS != nil {
		t.Fatal("unexpected upstream tls config")
	}
}

func TestSSLPassthroughKeepsExplicitBackendProtocol(t *testing.T) {
	manager := NewAnnotationHandlerManager()
	config := &Ingress{}
	err := manager.Parse(Annotations{
		buildNginxAnnotationKey(sslPassthroughAnnotation): "true",
		buildNginxAnnotationKey(backendProtocol):          "HTTPS",
	}, config, nil)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if config.UpstreamTLS == nil {
		t.Fatal("expected upstream tls config")
	}
	if config.UpstreamTLS.BackendProtocol != "HTTPS" {
		t.Fatalf("backend protocol mismatch, want HTTPS, got %s", config.UpstreamTLS.BackendProtocol)
	}
}
