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

	"github.com/golang/protobuf/ptypes/duration"
	networking "istio.io/api/networking/v1alpha3"
)

func TestLoadBalanceParse(t *testing.T) {
	loadBalance := loadBalance{}
	inputCases := []struct {
		input  map[string]string
		expect *LoadBalanceConfig
	}{
		{},
		{
			input: map[string]string{
				buildNginxAnnotationKey(affinity):     "cookie",
				buildNginxAnnotationKey(affinityMode): "balanced",
			},
			expect: &LoadBalanceConfig{
				simple: networking.LoadBalancerSettings_ROUND_ROBIN,
				cookie: &consistentHashByCookie{
					name: defaultAffinityCookieName,
					path: defaultAffinityCookiePath,
					age:  &duration.Duration{},
				},
			},
		},
		{
			input: map[string]string{
				buildNginxAnnotationKey(affinity):            "cookie",
				buildNginxAnnotationKey(affinityMode):        "balanced",
				buildNginxAnnotationKey(sessionCookieName):   "test",
				buildNginxAnnotationKey(sessionCookiePath):   "/test",
				buildNginxAnnotationKey(sessionCookieMaxAge): "100",
			},
			expect: &LoadBalanceConfig{
				simple: networking.LoadBalancerSettings_ROUND_ROBIN,
				cookie: &consistentHashByCookie{
					name: "test",
					path: "/test",
					age: &duration.Duration{
						Seconds: 100,
					},
				},
			},
		},
		{
			input: map[string]string{
				buildNginxAnnotationKey(affinity):             "cookie",
				buildNginxAnnotationKey(affinityMode):         "balanced",
				buildNginxAnnotationKey(sessionCookieName):    "test",
				buildNginxAnnotationKey(sessionCookieExpires): "10",
			},
			expect: &LoadBalanceConfig{
				simple: networking.LoadBalancerSettings_ROUND_ROBIN,
				cookie: &consistentHashByCookie{
					name: "test",
					path: defaultAffinityCookiePath,
					age: &duration.Duration{
						Seconds: 10,
					},
				},
			},
		},
		{
			input: map[string]string{
				buildNginxAnnotationKey(upstreamHashBy): "$request_uri",
			},
			expect: &LoadBalanceConfig{
				simple: networking.LoadBalancerSettings_ROUND_ROBIN,
				other: &consistentHashByOther{
					header: ":path",
				},
			},
		},
		{
			input: map[string]string{
				buildNginxAnnotationKey(upstreamHashBy): "$host",
			},
			expect: &LoadBalanceConfig{
				simple: networking.LoadBalancerSettings_ROUND_ROBIN,
				other: &consistentHashByOther{
					header: ":authority",
				},
			},
		},
		{
			input: map[string]string{
				buildNginxAnnotationKey(upstreamHashBy): "$remote_addr",
			},
			expect: &LoadBalanceConfig{
				simple: networking.LoadBalancerSettings_ROUND_ROBIN,
				other: &consistentHashByOther{
					useSourceIp: true,
				},
			},
		},
		{
			input: map[string]string{
				buildNginxAnnotationKey(upstreamHashBy): "$http_test",
			},
			expect: &LoadBalanceConfig{
				simple: networking.LoadBalancerSettings_ROUND_ROBIN,
				other: &consistentHashByOther{
					header: "test",
				},
			},
		},
		{
			input: map[string]string{
				buildNginxAnnotationKey(upstreamHashBy): "$arg_query",
			},
			expect: &LoadBalanceConfig{
				simple: networking.LoadBalancerSettings_ROUND_ROBIN,
				other: &consistentHashByOther{
					queryParam: "query",
				},
			},
		},
	}

	for _, inputCase := range inputCases {
		t.Run("", func(t *testing.T) {
			config := &Ingress{}
			_ = loadBalance.Parse(inputCase.input, config, nil)
			if !reflect.DeepEqual(inputCase.expect, config.LoadBalance) {
				t.Fatal("Should be equal")
			}
		})
	}
}

func TestLoadBalanceApplyTrafficPolicy(t *testing.T) {
	loadBalance := loadBalance{}
	inputCases := []struct {
		config *Ingress
		input  *networking.TrafficPolicy_PortTrafficPolicy
		expect *networking.TrafficPolicy_PortTrafficPolicy
	}{
		{
			config: &Ingress{},
			input:  &networking.TrafficPolicy_PortTrafficPolicy{},
			expect: &networking.TrafficPolicy_PortTrafficPolicy{},
		},
		{
			config: &Ingress{
				LoadBalance: &LoadBalanceConfig{
					cookie: &consistentHashByCookie{
						name: "test",
						path: "/",
						age: &duration.Duration{
							Seconds: 100,
						},
					},
				},
			},
			input: &networking.TrafficPolicy_PortTrafficPolicy{},
			expect: &networking.TrafficPolicy_PortTrafficPolicy{
				LoadBalancer: &networking.LoadBalancerSettings{
					LbPolicy: &networking.LoadBalancerSettings_ConsistentHash{
						ConsistentHash: &networking.LoadBalancerSettings_ConsistentHashLB{
							HashKey: &networking.LoadBalancerSettings_ConsistentHashLB_HttpCookie{
								HttpCookie: &networking.LoadBalancerSettings_ConsistentHashLB_HTTPCookie{
									Name: "test",
									Path: "/",
									Ttl: &duration.Duration{
										Seconds: 100,
									},
								},
							},
						},
					},
				},
			},
		},
		{
			config: &Ingress{
				LoadBalance: &LoadBalanceConfig{
					other: &consistentHashByOther{
						header: ":authority",
					},
				},
			},
			input: &networking.TrafficPolicy_PortTrafficPolicy{},
			expect: &networking.TrafficPolicy_PortTrafficPolicy{
				LoadBalancer: &networking.LoadBalancerSettings{
					LbPolicy: &networking.LoadBalancerSettings_ConsistentHash{
						ConsistentHash: &networking.LoadBalancerSettings_ConsistentHashLB{
							HashKey: &networking.LoadBalancerSettings_ConsistentHashLB_HttpHeaderName{
								HttpHeaderName: ":authority",
							},
						},
					},
				},
			},
		},
		{
			config: &Ingress{
				LoadBalance: &LoadBalanceConfig{
					other: &consistentHashByOther{
						queryParam: "query",
					},
				},
			},
			input: &networking.TrafficPolicy_PortTrafficPolicy{},
			expect: &networking.TrafficPolicy_PortTrafficPolicy{
				LoadBalancer: &networking.LoadBalancerSettings{
					LbPolicy: &networking.LoadBalancerSettings_ConsistentHash{
						ConsistentHash: &networking.LoadBalancerSettings_ConsistentHashLB{
							HashKey: &networking.LoadBalancerSettings_ConsistentHashLB_HttpQueryParameterName{
								HttpQueryParameterName: "query",
							},
						},
					},
				},
			},
		},
		{
			config: &Ingress{
				LoadBalance: &LoadBalanceConfig{
					other: &consistentHashByOther{
						useSourceIp: true,
					},
				},
			},
			input: &networking.TrafficPolicy_PortTrafficPolicy{},
			expect: &networking.TrafficPolicy_PortTrafficPolicy{
				LoadBalancer: &networking.LoadBalancerSettings{
					LbPolicy: &networking.LoadBalancerSettings_ConsistentHash{
						ConsistentHash: &networking.LoadBalancerSettings_ConsistentHashLB{
							HashKey: &networking.LoadBalancerSettings_ConsistentHashLB_UseSourceIp{
								UseSourceIp: true,
							},
						},
					},
				},
			},
		},
	}

	for _, inputCase := range inputCases {
		t.Run("", func(t *testing.T) {
			loadBalance.ApplyTrafficPolicy(nil, inputCase.input, inputCase.config)
			if !reflect.DeepEqual(inputCase.input, inputCase.expect) {
				t.Fatal("Should be equal")
			}
		})
	}
}

func TestLoadBalanceParseNegativeCookieAge(t *testing.T) {
	config := &Ingress{}
	annotations := Annotations{
		buildNginxAnnotationKey(affinity):            "cookie",
		buildNginxAnnotationKey(sessionCookieMaxAge): "-100",
	}
	if err := (loadBalance{}).Parse(annotations, config, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config.LoadBalance == nil || config.LoadBalance.cookie == nil {
		t.Fatal("expected cookie affinity config")
	}
	if got := config.LoadBalance.cookie.age.GetSeconds(); got != 0 {
		t.Fatalf("a negative cookie age must be ignored, got %d", got)
	}
}
