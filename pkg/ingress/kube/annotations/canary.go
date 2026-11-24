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
	networking "istio.io/api/networking/v1alpha3"
)

const (
	enableCanary          = "canary"
	canaryByHeader        = "canary-by-header"
	canaryByHeaderValue   = "canary-by-header-value"
	canaryByHeaderPattern = "canary-by-header-pattern"
	canaryByCookie        = "canary-by-cookie"
	canaryWeight          = "canary-weight"
	canaryWeightTotal     = "canary-weight-total"

	defaultCanaryWeightTotal = 100
)

var _ Parser = &canary{}

type CanaryConfig struct {
	Enabled       bool
	Header        string
	HeaderValue   string
	HeaderPattern string
	Cookie        string
	Weight        int
	WeightTotal   int
}

type canary struct{}

func (c canary) Parse(annotations Annotations, config *Ingress, _ *GlobalContext) error {
	if !needCanaryConfig(annotations) {
		return nil
	}

	canaryConfig := &CanaryConfig{
		WeightTotal: defaultCanaryWeightTotal,
	}

	defer func() {
		config.Canary = canaryConfig
	}()

	canaryConfig.Enabled, _ = annotations.ParseBoolASAP(enableCanary)
	if !canaryConfig.Enabled {
		return nil
	}

	if header, err := annotations.ParseStringASAP(canaryByHeader); err == nil {
		canaryConfig.Header = header
	}

	if headerValue, err := annotations.ParseStringASAP(canaryByHeaderValue); err == nil &&
		headerValue != "" {
		canaryConfig.HeaderValue = headerValue
		return nil
	}

	if headerPattern, err := annotations.ParseStringASAP(canaryByHeaderPattern); err == nil &&
		headerPattern != "" {
		canaryConfig.HeaderPattern = headerPattern
		return nil
	}

	if cookie, err := annotations.ParseStringASAP(canaryByCookie); err == nil &&
		cookie != "" {
		canaryConfig.Cookie = cookie
		return nil
	}

	if weight, err := annotations.ParseIntASAP(canaryWeight); err == nil && weight >= 0 {
		canaryConfig.Weight = weight
	}
	if weightTotal, err := annotations.ParseIntASAP(canaryWeightTotal); err == nil && weightTotal > 0 {
		canaryConfig.WeightTotal = weightTotal
	}

	return nil
}

func ApplyByWeight(canary, route *networking.HTTPRoute, canaryIngress *Ingress) {
	if len(route.Route) == 1 {
		// Move route level to destination level
		route.Route[0].Headers = route.Headers
		route.Headers = nil
	}

	// Modify canary weighted cluster
	canary.Route[0].Weight = int32(canaryIngress.Canary.Weight)

	// Append canary weight upstream service.
	// We will process total weight in the end.
	route.Route = append(route.Route, canary.Route[0])

	// canary route use the header control applied on itself.
	headerControl{}.ApplyRoute(canary, canaryIngress)
	// reset
	canary.Route[0].FallbackClusters = nil
	// Move route level to destination level
	canary.Route[0].Headers = canary.Headers

	// First add normal route cluster
	canary.Route[0].FallbackClusters = append(canary.Route[0].FallbackClusters,
		route.Route[0].Destination.DeepCopy())
	// Second add fallback cluster of normal route cluster
	canary.Route[0].FallbackClusters = append(canary.Route[0].FallbackClusters,
		route.Route[0].FallbackClusters...)
}

func ApplyByHeader(canary, route *networking.HTTPRoute, canaryIngress *Ingress) {
	canaryConfig := canaryIngress.Canary

	// Copy canary http route
	temp := canary.DeepCopy()

	// Inherit configuration from non-canary rule
	route.DeepCopyInto(canary)
	// Assign temp copied canary route destination
	canary.Route = temp.Route

	// Modified match base on by header
	if canaryConfig.Header != "" {
		for _, match := range canary.Match {
			match.Headers = map[string]*networking.StringMatch{
				canaryConfig.Header: {
					MatchType: &networking.StringMatch_Exact{
						Exact: "always",
					},
				},
			}
			if canaryConfig.HeaderValue != "" {
				match.Headers = map[string]*networking.StringMatch{
					canaryConfig.Header: {
						MatchType: &networking.StringMatch_Regex{
							Regex: "always|" + canaryConfig.HeaderValue,
						},
					},
				}
			} else if canaryConfig.HeaderPattern != "" {
				match.Headers = map[string]*networking.StringMatch{
					canaryConfig.Header: {
						MatchType: &networking.StringMatch_Regex{
							Regex: ".*" + canaryConfig.HeaderPattern + ".*",
						},
					},
				}
			}
		}
	} else if canaryConfig.Cookie != "" {
		for _, match := range canary.Match {
			match.Headers = map[string]*networking.StringMatch{
				"cookie": {
					MatchType: &networking.StringMatch_Regex{
						Regex: "^(.*?;\\s*)?(" + canaryConfig.Cookie + "=always)(;.*)?$",
					},
				},
			}
		}
	}

	canary.Headers = nil
	// canary route use the header control applied on itself.
	headerControl{}.ApplyRoute(canary, canaryIngress)

	// First add normal route cluster
	canary.Route[0].FallbackClusters = append(canary.Route[0].FallbackClusters,
		route.Route[0].Destination.DeepCopy())
	// Second add fallback cluster of normal route cluster
	canary.Route[0].FallbackClusters = append(canary.Route[0].FallbackClusters,
		route.Route[0].FallbackClusters...)
}

func needCanaryConfig(annotations Annotations) bool {
	return annotations.HasASAP(enableCanary)
}
