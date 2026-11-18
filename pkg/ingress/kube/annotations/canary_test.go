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
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	networking "istio.io/api/networking/v1alpha3"
)

func TestCanaryParse(t *testing.T) {
	parser := canary{}

	testCases := []struct {
		name   string
		input  Annotations
		expect *CanaryConfig
	}{
		{
			name:   "Don't contain the 'enableCanary' key",
			input:  Annotations{},
			expect: nil,
		},
		{
			name: "the 'enableCanary' is false",
			input: Annotations{
				buildNginxAnnotationKey(enableCanary): "false",
			},
			expect: &CanaryConfig{
				Enabled:     false,
				WeightTotal: defaultCanaryWeightTotal,
			},
		},
		{
			name: "By header",
			input: Annotations{
				buildNginxAnnotationKey(enableCanary):   "true",
				buildNginxAnnotationKey(canaryByHeader): "header",
			},
			expect: &CanaryConfig{
				Enabled:     true,
				Header:      "header",
				WeightTotal: defaultCanaryWeightTotal,
			},
		},
		{
			name: "By headerValue",
			input: Annotations{
				buildNginxAnnotationKey(enableCanary):        "true",
				buildNginxAnnotationKey(canaryByHeader):      "header",
				buildNginxAnnotationKey(canaryByHeaderValue): "headerValue",
			},
			expect: &CanaryConfig{
				Enabled:     true,
				Header:      "header",
				HeaderValue: "headerValue",
				WeightTotal: defaultCanaryWeightTotal,
			},
		},
		{
			name: "By headerPattern",
			input: Annotations{
				buildNginxAnnotationKey(enableCanary):          "true",
				buildNginxAnnotationKey(canaryByHeader):        "header",
				buildNginxAnnotationKey(canaryByHeaderPattern): "headerPattern",
			},
			expect: &CanaryConfig{
				Enabled:       true,
				Header:        "header",
				HeaderPattern: "headerPattern",
				WeightTotal:   defaultCanaryWeightTotal,
			},
		},
		{
			name: "By cookie",
			input: Annotations{
				buildNginxAnnotationKey(enableCanary):   "true",
				buildNginxAnnotationKey(canaryByCookie): "cookie",
			},
			expect: &CanaryConfig{
				Enabled:     true,
				Cookie:      "cookie",
				WeightTotal: defaultCanaryWeightTotal,
			},
		},
		{
			name: "By weight",
			input: Annotations{
				buildNginxAnnotationKey(enableCanary):      "true",
				buildNginxAnnotationKey(canaryWeight):      "50",
				buildNginxAnnotationKey(canaryWeightTotal): "100",
			},
			expect: &CanaryConfig{
				Enabled:     true,
				Weight:      50,
				WeightTotal: 100,
			},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			config := &Ingress{}
			_ = parser.Parse(tt.input, config, nil)
			if diff := cmp.Diff(tt.expect, config.Canary); diff != "" {
				t.Fatalf("TestCanaryParse() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestApplyWeight(t *testing.T) {
	testCases := []struct {
		normal *networking.HTTPRoute
		canary []*networking.HTTPRoute
		config []*Ingress
		expect *networking.HTTPRoute
	}{
		{
			normal: &networking.HTTPRoute{
				Headers: &networking.Headers{
					Request: &networking.Headers_HeaderOperations{
						Add: map[string]string{
							"normal": "true",
						},
					},
				},
				Route: []*networking.HTTPRouteDestination{
					{
						Destination: &networking.Destination{
							Host: "normal",
							Port: &networking.PortSelector{
								Number: 80,
							},
						},
					},
				},
			},
			canary: []*networking.HTTPRoute{
				{
					Headers: &networking.Headers{
						Request: &networking.Headers_HeaderOperations{
							Add: map[string]string{
								"canary1": "true",
							},
						},
					},
					Route: []*networking.HTTPRouteDestination{
						{
							Destination: &networking.Destination{
								Host: "canary1",
								Port: &networking.PortSelector{
									Number: 80,
								},
							},
						},
					},
				},
				{
					Headers: &networking.Headers{
						Request: &networking.Headers_HeaderOperations{
							Add: map[string]string{
								"canary2": "true",
							},
						},
					},
					Route: []*networking.HTTPRouteDestination{
						{
							Destination: &networking.Destination{
								Host: "canary2",
								Port: &networking.PortSelector{
									Number: 80,
								},
							},
						},
					},
				},
			},
			config: []*Ingress{
				{
					Canary: &CanaryConfig{
						Weight: 30,
					},
				},
				{
					Canary: &CanaryConfig{
						Weight: 20,
					},
				},
			},
			expect: &networking.HTTPRoute{
				Route: []*networking.HTTPRouteDestination{
					{
						Destination: &networking.Destination{
							Host: "normal",
							Port: &networking.PortSelector{
								Number: 80,
							},
						},
						Headers: &networking.Headers{
							Request: &networking.Headers_HeaderOperations{
								Add: map[string]string{
									"normal": "true",
								},
							},
						},
					},
					{
						Destination: &networking.Destination{
							Host: "canary1",
							Port: &networking.PortSelector{
								Number: 80,
							},
						},
						Headers: &networking.Headers{
							Request: &networking.Headers_HeaderOperations{
								Add: map[string]string{
									"canary1": "true",
								},
							},
						},
						Weight: 30,
						FallbackClusters: []*networking.Destination{
							{
								Host: "normal",
								Port: &networking.PortSelector{
									Number: 80,
								},
							},
						},
					},
					{
						Destination: &networking.Destination{
							Host: "canary2",
							Port: &networking.PortSelector{
								Number: 80,
							},
						},
						Headers: &networking.Headers{
							Request: &networking.Headers_HeaderOperations{
								Add: map[string]string{
									"canary2": "true",
								},
							},
						},
						Weight: 20,
						FallbackClusters: []*networking.Destination{
							{
								Host: "normal",
								Port: &networking.PortSelector{
									Number: 80,
								},
							},
						},
					},
				},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run("", func(t *testing.T) {
			for i := range testCase.canary {
				canary := testCase.canary[i]
				config := testCase.config[i]
				ApplyByWeight(canary, testCase.normal, config)
			}
			for index, route := range testCase.normal.Route {
				fmt.Printf("actual route %d: %+v\n", index, route)

			}
			for index, route := range testCase.expect.Route {
				fmt.Printf("expect route %d: %+v\n", index, route)

			}
			assert.Equal(t, testCase.expect, testCase.normal)
		})
	}
}

func TestCanaryParseNegativeWeight(t *testing.T) {
	config := &Ingress{}
	annotations := Annotations{
		buildNginxAnnotationKey(enableCanary): "true",
		buildNginxAnnotationKey(canaryWeight): "-5",
	}
	if err := (canary{}).Parse(annotations, config, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config.Canary == nil {
		t.Fatal("expected canary config")
	}
	if config.Canary.Weight != 0 {
		t.Fatalf("a negative weight must be ignored, got %d", config.Canary.Weight)
	}
}
