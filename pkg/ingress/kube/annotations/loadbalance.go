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
	"strings"

	"github.com/golang/protobuf/ptypes/duration"
	networking "istio.io/api/networking/v1alpha3"
)

const (
	loadBalanceAnnotation = "load-balance"
	upstreamHashBy        = "upstream-hash-by"
	// affinity in nginx/mse ingress always be cookie
	affinity = "affinity"
	// affinityMode in mse ingress always be balanced
	affinityMode = "affinity-mode"
	// affinityCanaryBehavior in mse ingress always be legacy
	affinityCanaryBehavior = "affinity-canary-behavior"
	sessionCookieName      = "session-cookie-name"
	sessionCookiePath      = "session-cookie-path"
	sessionCookieMaxAge    = "session-cookie-max-age"
	sessionCookieExpires   = "session-cookie-expires"

	varIndicator        = "$"
	headerIndicator     = "$http_"
	queryParamIndicator = "$arg_"

	defaultAffinityCookieName = "INGRESSCOOKIE"
	defaultAffinityCookiePath = "/"

	mcpSseStatefulKey        = "mcp-sse-stateful-param-name"
	defaultMcpSseStatefulKey = "sessionId"
)

var (
	_ Parser               = loadBalance{}
	_ TrafficPolicyHandler = loadBalance{}

	headersMapping = map[string]string{
		"$request_uri": ":path",
		"$host":        ":authority",
	}
)

type consistentHashByOther struct {
	header      string
	queryParam  string
	useSourceIp bool
}

type consistentHashByCookie struct {
	name string
	path string
	age  *duration.Duration
}

type LoadBalanceConfig struct {
	simple            networking.LoadBalancerSettings_SimpleLB
	other             *consistentHashByOther
	cookie            *consistentHashByCookie
	McpSseStateful    bool
	McpSseStatefulKey string
}

type loadBalance struct{}

func (l loadBalance) Parse(annotations Annotations, config *Ingress, _ *GlobalContext) error {
	if !needLoadBalanceConfig(annotations) {
		return nil
	}

	loadBalanceConfig := &LoadBalanceConfig{
		simple: networking.LoadBalancerSettings_ROUND_ROBIN,
	}
	defer func() {
		config.LoadBalance = loadBalanceConfig
	}()

	if isCookieAffinity(annotations) {
		loadBalanceConfig.cookie = &consistentHashByCookie{
			name: defaultAffinityCookieName,
			path: defaultAffinityCookiePath,
			age:  &duration.Duration{},
		}
		if name, err := annotations.ParseStringASAP(sessionCookieName); err == nil {
			loadBalanceConfig.cookie.name = name
		}
		if path, err := annotations.ParseStringASAP(sessionCookiePath); err == nil {
			loadBalanceConfig.cookie.path = path
		}
		if age, err := annotations.ParseIntASAP(sessionCookieMaxAge); err == nil && age >= 0 {
			loadBalanceConfig.cookie.age = &duration.Duration{
				Seconds: int64(age),
			}
		} else if age, err = annotations.ParseIntASAP(sessionCookieExpires); err == nil && age >= 0 {
			loadBalanceConfig.cookie.age = &duration.Duration{
				Seconds: int64(age),
			}
		}
	} else if isOtherAffinity(annotations) {
		if key, err := annotations.ParseStringASAP(upstreamHashBy); err == nil &&
			strings.HasPrefix(key, varIndicator) {
			// Special case for $remote_addr: use useSourceIp instead of header mapping
			if key == "$remote_addr" {
				loadBalanceConfig.other = &consistentHashByOther{
					useSourceIp: true,
				}
			} else {
				value, exist := headersMapping[key]
				if exist {
					loadBalanceConfig.other = &consistentHashByOther{
						header: value,
					}
				} else {
					if strings.HasPrefix(key, headerIndicator) {
						loadBalanceConfig.other = &consistentHashByOther{
							header: strings.TrimPrefix(key, headerIndicator),
						}
					} else if strings.HasPrefix(key, queryParamIndicator) {
						loadBalanceConfig.other = &consistentHashByOther{
							queryParam: strings.TrimPrefix(key, queryParamIndicator),
						}
					}
				}
			}
		}
	} else {
		if lb, err := annotations.ParseStringASAP(loadBalanceAnnotation); err == nil {
			lb = strings.ToUpper(lb)
			if lb == "MCP-SSE" {
				loadBalanceConfig.McpSseStateful = true
				if key, err := annotations.ParseStringASAP(mcpSseStatefulKey); err == nil {
					loadBalanceConfig.McpSseStatefulKey = key
				} else {
					loadBalanceConfig.McpSseStatefulKey = defaultMcpSseStatefulKey
				}
			} else {
				loadBalanceConfig.simple = networking.LoadBalancerSettings_SimpleLB(networking.LoadBalancerSettings_SimpleLB_value[lb])
			}
		}
	}

	return nil
}

func (l loadBalance) ApplyTrafficPolicy(trafficPolicy *networking.TrafficPolicy, portTrafficPolicy *networking.TrafficPolicy_PortTrafficPolicy, config *Ingress) {
	loadBalanceConfig := config.LoadBalance
	if loadBalanceConfig == nil {
		return
	}

	var loadBalancer *networking.LoadBalancerSettings

	if loadBalanceConfig.cookie != nil {
		loadBalancer = &networking.LoadBalancerSettings{
			LbPolicy: &networking.LoadBalancerSettings_ConsistentHash{
				ConsistentHash: &networking.LoadBalancerSettings_ConsistentHashLB{
					HashKey: &networking.LoadBalancerSettings_ConsistentHashLB_HttpCookie{
						HttpCookie: &networking.LoadBalancerSettings_ConsistentHashLB_HTTPCookie{
							Name: loadBalanceConfig.cookie.name,
							Path: loadBalanceConfig.cookie.path,
							Ttl:  loadBalanceConfig.cookie.age,
						},
					},
				},
			},
		}
	} else if loadBalanceConfig.other != nil {
		var consistentHash *networking.LoadBalancerSettings_ConsistentHashLB
		if loadBalanceConfig.other.useSourceIp {
			consistentHash = &networking.LoadBalancerSettings_ConsistentHashLB{
				HashKey: &networking.LoadBalancerSettings_ConsistentHashLB_UseSourceIp{
					UseSourceIp: true,
				},
			}
		} else if loadBalanceConfig.other.header != "" {
			consistentHash = &networking.LoadBalancerSettings_ConsistentHashLB{
				HashKey: &networking.LoadBalancerSettings_ConsistentHashLB_HttpHeaderName{
					HttpHeaderName: loadBalanceConfig.other.header,
				},
			}
		} else {
			consistentHash = &networking.LoadBalancerSettings_ConsistentHashLB{
				HashKey: &networking.LoadBalancerSettings_ConsistentHashLB_HttpQueryParameterName{
					HttpQueryParameterName: loadBalanceConfig.other.queryParam,
				},
			}
		}
		loadBalancer = &networking.LoadBalancerSettings{
			LbPolicy: &networking.LoadBalancerSettings_ConsistentHash{
				ConsistentHash: consistentHash,
			},
		}
	} else {
		loadBalancer = &networking.LoadBalancerSettings{
			LbPolicy: &networking.LoadBalancerSettings_Simple{
				Simple: loadBalanceConfig.simple,
			},
		}
	}

	if trafficPolicy != nil {
		trafficPolicy.LoadBalancer = loadBalancer
	}
	if portTrafficPolicy != nil {
		portTrafficPolicy.LoadBalancer = loadBalancer
	}
}

func isCookieAffinity(annotations Annotations) bool {
	return annotations.HasASAP(affinity) ||
		annotations.HasASAP(sessionCookieName) ||
		annotations.HasASAP(sessionCookiePath)
}

func isOtherAffinity(annotations Annotations) bool {
	return annotations.HasASAP(upstreamHashBy)
}

func needLoadBalanceConfig(annotations Annotations) bool {
	return annotations.HasASAP(loadBalanceAnnotation) ||
		isCookieAffinity(annotations) ||
		isOtherAffinity(annotations)
}
