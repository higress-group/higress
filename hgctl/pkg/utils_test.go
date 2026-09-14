// Copyright (c) 2026 Alibaba Group Holding Ltd.
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

package hgctl

import (
	"strings"
	"testing"
)

func TestGetXDSResource(t *testing.T) {
	tests := []struct {
		name         string
		resourceType envoyConfigType
		configDump   string
		wantType     string
		wantErr      string
	}{
		{
			name:         "invalid JSON",
			resourceType: ClusterEnvoyConfigType,
			configDump:   `{`,
			wantErr:      "decode config dump",
		},
		{
			name:         "missing configs",
			resourceType: ClusterEnvoyConfigType,
			configDump:   `{}`,
			wantErr:      "config dump is missing configs",
		},
		{
			name:         "null configs",
			resourceType: ClusterEnvoyConfigType,
			configDump:   `{"configs":null}`,
			wantErr:      "config dump configs must be an array",
		},
		{
			name:         "non-array configs",
			resourceType: ClusterEnvoyConfigType,
			configDump:   `{"configs":{}}`,
			wantErr:      "config dump configs must be an array",
		},
		{
			name:         "non-object config",
			resourceType: ClusterEnvoyConfigType,
			configDump:   `{"configs":["invalid"]}`,
			wantErr:      "config dump configs[0] must be an object",
		},
		{
			name:         "null config",
			resourceType: ClusterEnvoyConfigType,
			configDump:   `{"configs":[null]}`,
			wantErr:      "config dump configs[0] must be an object",
		},
		{
			name:         "missing requested resource",
			resourceType: ClusterEnvoyConfigType,
			configDump:   `{"configs":[]}`,
			wantErr:      "config dump is missing cluster resource",
		},
		{
			name:         "unknown resource type",
			resourceType: envoyConfigType("secret"),
			configDump:   `{"configs":[]}`,
			wantErr:      `unknown resource type "secret"`,
		},
		{
			name:         "valid bootstrap resource",
			resourceType: BootstrapEnvoyConfigType,
			configDump:   `{"configs":[{"@type":"type.googleapis.com/envoy.admin.v3.BootstrapConfigDump","value":"preserved"}]}`,
			wantType:     "type.googleapis.com/envoy.admin.v3.BootstrapConfigDump",
		},
		{
			name:         "valid endpoint resource",
			resourceType: EndpointEnvoyConfigType,
			configDump:   `{"configs":[{"@type":"type.googleapis.com/envoy.admin.v3.EndpointsConfigDump","value":"preserved"}]}`,
			wantType:     "type.googleapis.com/envoy.admin.v3.EndpointsConfigDump",
		},
		{
			name:         "valid cluster resource",
			resourceType: ClusterEnvoyConfigType,
			configDump:   `{"configs":[{"@type":"type.googleapis.com/envoy.admin.v3.ClustersConfigDump","value":"preserved"}]}`,
			wantType:     "type.googleapis.com/envoy.admin.v3.ClustersConfigDump",
		},
		{
			name:         "valid listener resource",
			resourceType: ListenerEnvoyConfigType,
			configDump:   `{"configs":[{"@type":"type.googleapis.com/envoy.admin.v3.ListenersConfigDump","value":"preserved"}]}`,
			wantType:     "type.googleapis.com/envoy.admin.v3.ListenersConfigDump",
		},
		{
			name:         "valid route resource",
			resourceType: RouteEnvoyConfigType,
			configDump:   `{"configs":[{"@type":"type.googleapis.com/envoy.admin.v3.RoutesConfigDump","value":"preserved"}]}`,
			wantType:     "type.googleapis.com/envoy.admin.v3.RoutesConfigDump",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource, err := GetXDSResource(tt.resourceType, []byte(tt.configDump))
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("GetXDSResource() error = nil, want error containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("GetXDSResource() error = %q, want error containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("GetXDSResource() unexpected error: %v", err)
			}
			config, ok := resource.(map[string]any)
			if !ok {
				t.Fatalf("GetXDSResource() result type = %T, want map[string]any", resource)
			}
			if got := config["@type"]; got != tt.wantType {
				t.Errorf("GetXDSResource() resource @type = %v, want %q", got, tt.wantType)
			}
			if got := config["value"]; got != "preserved" {
				t.Errorf("GetXDSResource() resource value = %v, want preserved", got)
			}
		})
	}
}

func TestGetXDSResourceAll(t *testing.T) {
	resource, err := GetXDSResource(AllEnvoyConfigType, []byte(`{"metadata":"preserved"}`))
	if err != nil {
		t.Fatalf("GetXDSResource() unexpected error: %v", err)
	}
	configDump, ok := resource.(map[string]any)
	if !ok {
		t.Fatalf("GetXDSResource() result type = %T, want map[string]any", resource)
	}
	if got := configDump["metadata"]; got != "preserved" {
		t.Errorf("GetXDSResource() config dump metadata = %v, want preserved", got)
	}
}
