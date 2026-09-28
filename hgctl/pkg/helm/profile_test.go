// Copyright (c) 2022 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helm

import (
	"testing"

	"sigs.k8s.io/yaml"
)

func TestProfileValidatePersistsResources(t *testing.T) {
	spaced := Resource{
		Requests: Requests{CPU: " 2 5 0 m ", Memory: " 5 1 2 M i "},
		Limits:   Limits{CPU: " 2 0 0 0 m ", Memory: " 2 0 4 8 M i "},
	}
	normalized := Resource{
		Requests: Requests{CPU: "250m", Memory: "512Mi"},
		Limits:   Limits{CPU: "2000m", Memory: "2048Mi"},
	}

	tests := []struct {
		name           string
		input          Resource
		wantConsole    Resource
		wantGateway    Resource
		wantController Resource
	}{
		{
			name:  "defaults",
			input: Resource{},
			wantConsole: Resource{
				Requests: Requests{CPU: "250m", Memory: "512Mi"},
				Limits:   Limits{CPU: "2000m", Memory: "2048Mi"},
			},
			wantGateway: Resource{
				Requests: Requests{CPU: "2000m", Memory: "2048Mi"},
				Limits:   Limits{CPU: "2000m", Memory: "2048Mi"},
			},
			wantController: Resource{
				Requests: Requests{CPU: "500m", Memory: "2048Mi"},
				Limits:   Limits{CPU: "1000m", Memory: "2048Mi"},
			},
		},
		{
			name:           "whitespace is normalized",
			input:          spaced,
			wantConsole:    normalized,
			wantGateway:    normalized,
			wantController: normalized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := &Profile{
				Global: ProfileGlobal{
					Install:      InstallK8s,
					IngressClass: "higress",
					Namespace:    "higress-system",
				},
				Console:    ProfileConsole{Replicas: 1, Resources: tt.input},
				Gateway:    ProfileGateway{Replicas: 1, Resources: tt.input},
				Controller: ProfileController{Replicas: 1, Resources: tt.input},
			}

			if err := profile.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			assertResource(t, "console", profile.Console.Resources, tt.wantConsole)
			assertResource(t, "gateway", profile.Gateway.Resources, tt.wantGateway)
			assertResource(t, "controller", profile.Controller.Resources, tt.wantController)

			valuesYaml, err := profile.ValuesYaml()
			if err != nil {
				t.Fatalf("ValuesYaml() error = %v", err)
			}
			values := make(map[string]any)
			if err := yaml.Unmarshal([]byte(valuesYaml), &values); err != nil {
				t.Fatalf("unmarshal values: %v", err)
			}
			assertValuesResource(t, values, tt.wantConsole, "higress-console", "resources")
			assertValuesResource(t, values, tt.wantGateway, "higress-core", "gateway", "resources")
			assertValuesResource(t, values, tt.wantController, "higress-core", "controller", "resources")
		})
	}
}

func assertResource(t *testing.T, component string, got, want Resource) {
	t.Helper()
	if got != want {
		t.Errorf("%s resources = %+v, want %+v", component, got, want)
	}
}

func assertValuesResource(t *testing.T, values map[string]any, want Resource, path ...string) {
	t.Helper()
	got := Resource{
		Requests: Requests{
			CPU:    lookupString(values, append(path, "requests", "cpu")...),
			Memory: lookupString(values, append(path, "requests", "memory")...),
		},
		Limits: Limits{
			CPU:    lookupString(values, append(path, "limits", "cpu")...),
			Memory: lookupString(values, append(path, "limits", "memory")...),
		},
	}
	if got != want {
		t.Errorf("values %v = %+v, want %+v", path, got, want)
	}
}

func lookupString(values map[string]any, path ...string) string {
	var current any = values
	for _, key := range path {
		m, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = m[key]
	}
	s, _ := current.(string)
	return s
}
