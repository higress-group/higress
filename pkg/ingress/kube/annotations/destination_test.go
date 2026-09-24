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

func TestDestinationParse(t *testing.T) {
	parser := destination{}

	testCases := []struct {
		input  Annotations
		expect *DestinationConfig
	}{
		{
			input:  Annotations{},
			expect: nil,
		},
		{
			input: Annotations{
				buildHigressAnnotationKey(destinationKey): "",
			},
			expect: nil,
		},
		{
			input: Annotations{
				buildHigressAnnotationKey(destinationKey): "100% my-svc.DEFAULT-GROUP.xxxx.nacos:8080 v1",
			},
			expect: &DestinationConfig{
				McpDestination: []*networking.HTTPRouteDestination{
					{
						Destination: &networking.Destination{
							Host:   "my-svc.DEFAULT-GROUP.xxxx.nacos",
							Subset: "v1",
							Port:   &networking.PortSelector{Number: 8080},
						},
						Weight: 100,
					},
				},
				WeightSum: 100,
			},
		},
		{
			input: Annotations{
				buildHigressAnnotationKey(destinationKey): "50% my-svc.DEFAULT-GROUP.xxxx.nacos:8080 v1\n\n" +
					"50% my-svc.DEFAULT-GROUP.xxxx.nacos:8080 v2",
			},
			expect: &DestinationConfig{
				McpDestination: []*networking.HTTPRouteDestination{
					{
						Destination: &networking.Destination{
							Host:   "my-svc.DEFAULT-GROUP.xxxx.nacos",
							Subset: "v1",
							Port:   &networking.PortSelector{Number: 8080},
						},
						Weight: 50,
					},
					{
						Destination: &networking.Destination{
							Host:   "my-svc.DEFAULT-GROUP.xxxx.nacos",
							Subset: "v2",
							Port:   &networking.PortSelector{Number: 8080},
						},
						Weight: 50,
					},
				},
				WeightSum: 100,
			},
		},
		{
			input: Annotations{
				buildHigressAnnotationKey(destinationKey): "providers:com.alibaba.nacos.example.dubbo.service.DemoService:1.0.0:.DEFAULT-GROUP.public.nacos",
			},
			expect: &DestinationConfig{
				McpDestination: []*networking.HTTPRouteDestination{
					{
						Destination: &networking.Destination{
							Host: "providers:com.alibaba.nacos.example.dubbo.service.DemoService:1.0.0:.DEFAULT-GROUP.public.nacos",
						},
						Weight: 100,
					},
				},
				WeightSum: 100,
			},
		},
		{
			input: Annotations{
				buildHigressAnnotationKey(destinationKey): "providers:com.alibaba.nacos.example.dubbo.service.DemoService:1.0.0:.DEFAULT-GROUP.public.nacos:8080",
			},
			expect: &DestinationConfig{
				McpDestination: []*networking.HTTPRouteDestination{
					{
						Destination: &networking.Destination{
							Host: "providers:com.alibaba.nacos.example.dubbo.service.DemoService:1.0.0:.DEFAULT-GROUP.public.nacos",
							Port: &networking.PortSelector{Number: 8080},
						},
						Weight: 100,
					},
				},
				WeightSum: 100,
			},
		},
		{
			input: Annotations{
				buildHigressAnnotationKey(destinationKey): "50% http://plain.example.com:80\n50% https://secure.example.com:443",
			},
			expect: &DestinationConfig{
				McpDestination: []*networking.HTTPRouteDestination{
					{
						Destination: &networking.Destination{
							Host: "plain.example.com",
							Port: &networking.PortSelector{Number: 80},
						},
						Weight: 50,
					},
					{
						Destination: &networking.Destination{
							Host: "secure.example.com",
							Port: &networking.PortSelector{Number: 443},
						},
						Weight: 50,
					},
				},
				WeightSum: 100,
				Protocols: map[string]string{
					"plain.example.com:80":   "HTTP",
					"secure.example.com:443": "HTTPS",
				},
			},
		},
	}

	unexportedIgnoredTypes := []interface{}{
		networking.HTTPRouteDestination{},
		networking.Destination{},
		networking.PortSelector{},
	}

	for _, testCase := range testCases {
		t.Run("", func(t *testing.T) {
			config := &Ingress{}
			_ = parser.Parse(testCase.input, config, nil)
			if diff := cmp.Diff(config.Destination, testCase.expect, cmpopts.IgnoreUnexported(unexportedIgnoredTypes...)); diff != "" {
				t.Fatalf("TestDestinationParse() mismatch: (-want +got)\n%s", diff)
			}
		})
	}
}
