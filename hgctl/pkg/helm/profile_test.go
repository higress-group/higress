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

package helm

import (
	"reflect"
	"testing"

	"sigs.k8s.io/yaml"
)

func TestProfileValidatePersistsResources(t *testing.T) {
	defaultProfile := validK8sProfile()
	spacedProfile := validK8sProfile()
	spacedResources := Resource{
		Requests: Requests{CPU: " 2 5 0 m ", Memory: " 5 1 2 M i "},
		Limits:   Limits{CPU: " 2 0 0 0 m ", Memory: " 2 0 4 8 M i "},
	}
	spacedProfile.Console.Resources = spacedResources
	spacedProfile.Gateway.Resources = spacedResources
	spacedProfile.Controller.Resources = spacedResources
	normalizedResources := Resource{
		Requests: Requests{CPU: "250m", Memory: "512Mi"},
		Limits:   Limits{CPU: "2000m", Memory: "2048Mi"},
	}

	tests := []struct {
		name    string
		profile Profile
		want    profileResources
	}{
		{
			name:    "defaults",
			profile: defaultProfile,
			want: profileResources{
				Console: normalizedResources,
				Gateway: Resource{
					Requests: Requests{CPU: "2000m", Memory: "2048Mi"},
					Limits:   Limits{CPU: "2000m", Memory: "2048Mi"},
				},
				Controller: Resource{
					Requests: Requests{CPU: "500m", Memory: "2048Mi"},
					Limits:   Limits{CPU: "1000m", Memory: "2048Mi"},
				},
			},
		},
		{
			name:    "whitespace normalization",
			profile: spacedProfile,
			want: profileResources{
				Console:    normalizedResources,
				Gateway:    normalizedResources,
				Controller: normalizedResources,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.profile.Validate(); err != nil {
				t.Fatalf("Validate() returned an unexpected error: %v", err)
			}

			got := resourcesFromProfile(&tt.profile)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("resources after Validate() = %#v, want %#v", got, tt.want)
			}

			valuesYAML, err := tt.profile.ValuesYaml()
			if err != nil {
				t.Fatalf("ValuesYaml() returned an unexpected error: %v", err)
			}
			got, err = resourcesFromValuesYAML(valuesYAML)
			if err != nil {
				t.Fatalf("failed to parse ValuesYaml() output: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("resources in ValuesYaml() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

type profileResources struct {
	Console    Resource
	Gateway    Resource
	Controller Resource
}

func validK8sProfile() Profile {
	return Profile{
		Global: ProfileGlobal{
			Install:      InstallK8s,
			IngressClass: "higress",
			Namespace:    "higress-system",
		},
		Console:    ProfileConsole{Replicas: 1},
		Gateway:    ProfileGateway{Replicas: 1},
		Controller: ProfileController{Replicas: 1},
	}
}

func resourcesFromProfile(profile *Profile) profileResources {
	return profileResources{
		Console:    profile.Console.Resources,
		Gateway:    profile.Gateway.Resources,
		Controller: profile.Controller.Resources,
	}
}

func resourcesFromValuesYAML(valuesYAML string) (profileResources, error) {
	var values struct {
		HigressCore struct {
			Controller struct {
				Resources Resource `json:"resources"`
			} `json:"controller"`
			Gateway struct {
				Resources Resource `json:"resources"`
			} `json:"gateway"`
		} `json:"higress-core"`
		HigressConsole struct {
			Resources Resource `json:"resources"`
		} `json:"higress-console"`
	}
	if err := yaml.Unmarshal([]byte(valuesYAML), &values); err != nil {
		return profileResources{}, err
	}
	return profileResources{
		Console:    values.HigressConsole.Resources,
		Gateway:    values.HigressCore.Gateway.Resources,
		Controller: values.HigressCore.Controller.Resources,
	}, nil
}
