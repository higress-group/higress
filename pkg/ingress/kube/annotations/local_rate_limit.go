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

	"github.com/golang/protobuf/ptypes/duration"
	networking "istio.io/api/networking/v1alpha3"
	//"istio.io/istio/pilot/pkg/networking/core/v1alpha3/mseingress"
)

const (
	limitRPM             = "route-limit-rpm"
	limitRPS             = "route-limit-rps"
	limitBurstMultiplier = "route-limit-burst-multiplier"

	defaultBurstMultiplier = 5
	defaultStatusCode      = 429
)

var (
	_ Parser       = localRateLimit{}
	_ RouteHandler = localRateLimit{}

	second = &duration.Duration{
		Seconds: 1,
	}

	minute = &duration.Duration{
		Seconds: 60,
	}
)

type localRateLimitConfig struct {
	TokensPerFill uint32
	MaxTokens     uint32
	FillInterval  *duration.Duration
}

type localRateLimit struct{}

func (l localRateLimit) Parse(annotations Annotations, config *Ingress, _ *GlobalContext) error {
	if !needLocalRateLimitConfig(annotations) {
		return nil
	}

	var local *localRateLimitConfig
	defer func() {
		config.localRateLimit = local
	}()

	multiplier := uint32(defaultBurstMultiplier)
	if annotations.HasHigress(limitBurstMultiplier) {
		m, err := annotations.ParseUint32ForHigress(limitBurstMultiplier)
		if err != nil || m == 0 {
			return fmt.Errorf("invalid %s annotation", limitBurstMultiplier)
		}
		multiplier = m
	}

	if annotations.HasHigress(limitRPM) {
		rpm, err := annotations.ParseUint32ForHigress(limitRPM)
		if err != nil {
			return fmt.Errorf("invalid %s annotation", limitRPM)
		}
		local, err = buildLocalRateLimitConfig(rpm, multiplier, minute)
		if err != nil {
			return err
		}
	} else {
		rps, err := annotations.ParseUint32ForHigress(limitRPS)
		if err != nil {
			return fmt.Errorf("invalid %s annotation", limitRPS)
		}
		local, err = buildLocalRateLimitConfig(rps, multiplier, second)
		if err != nil {
			return err
		}
	}

	return nil
}

func buildLocalRateLimitConfig(rate, multiplier uint32, interval *duration.Duration) (*localRateLimitConfig, error) {
	if rate == 0 {
		return nil, fmt.Errorf("local rate limit must be greater than zero")
	}
	if multiplier == 0 {
		return nil, fmt.Errorf("local rate limit burst multiplier must be greater than zero")
	}
	if rate > ^uint32(0)/multiplier {
		return nil, fmt.Errorf("local rate limit burst exceeds uint32")
	}
	return &localRateLimitConfig{
		MaxTokens:     rate * multiplier,
		TokensPerFill: rate,
		FillInterval:  interval,
	}, nil
}

func (l localRateLimit) ApplyRoute(route *networking.HTTPRoute, config *Ingress) {
	localRateLimitConfig := config.localRateLimit
	if localRateLimitConfig == nil {
		return
	}

	route.RouteHTTPFilters = append(route.RouteHTTPFilters, &networking.HTTPFilter{
		// TODO: hardcode
		Name: "local-rate-limit",
		Filter: &networking.HTTPFilter_LocalRateLimit{
			LocalRateLimit: &networking.LocalRateLimit{
				TokenBucket: &networking.TokenBucket{
					MaxTokens:     localRateLimitConfig.MaxTokens,
					TokensPefFill: localRateLimitConfig.TokensPerFill,
					FillInterval:  localRateLimitConfig.FillInterval,
				},
				StatusCode: defaultStatusCode,
			},
		},
	})
}

func needLocalRateLimitConfig(annotations Annotations) bool {
	return annotations.HasHigress(limitRPM) ||
		annotations.HasHigress(limitRPS)
}
