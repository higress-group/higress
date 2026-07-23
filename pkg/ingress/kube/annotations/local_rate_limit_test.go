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

	networking "istio.io/api/networking/v1alpha3"
)

func TestLocalRateLimitParse(t *testing.T) {
	localRateLimit := localRateLimit{}
	inputCases := []struct {
		input   map[string]string
		expect  *localRateLimitConfig
		wantErr bool
	}{
		{},
		{
			input: map[string]string{
				buildHigressAnnotationKey(limitRPM): "2",
			},
			expect: &localRateLimitConfig{
				MaxTokens:     10,
				TokensPerFill: 2,
				FillInterval:  minute,
			},
		},
		{
			input: map[string]string{
				buildHigressAnnotationKey(limitRPM):             "2",
				buildHigressAnnotationKey(limitRPS):             "3",
				buildHigressAnnotationKey(limitBurstMultiplier): "10",
			},
			expect: &localRateLimitConfig{
				MaxTokens:     20,
				TokensPerFill: 2,
				FillInterval:  minute,
			},
		},
		{
			input: map[string]string{
				buildHigressAnnotationKey(limitRPS):             "3",
				buildHigressAnnotationKey(limitBurstMultiplier): "10",
			},
			expect: &localRateLimitConfig{
				MaxTokens:     30,
				TokensPerFill: 3,
				FillInterval:  second,
			},
		},
		{
			input: map[string]string{
				buildHigressAnnotationKey(limitRPM): "-1",
			},
			wantErr: true,
		},
		{
			input: map[string]string{
				buildHigressAnnotationKey(limitRPM): "4294967296",
			},
			wantErr: true,
		},
		{
			input: map[string]string{
				buildHigressAnnotationKey(limitRPM):             "4294967295",
				buildHigressAnnotationKey(limitBurstMultiplier): "2",
			},
			wantErr: true,
		},
		{
			input: map[string]string{
				buildHigressAnnotationKey(limitRPS):             "1",
				buildHigressAnnotationKey(limitBurstMultiplier): "0",
			},
			wantErr: true,
		},
	}

	for _, inputCase := range inputCases {
		t.Run("", func(t *testing.T) {
			config := &Ingress{}
			err := localRateLimit.Parse(inputCase.input, config, nil)
			if (err != nil) != inputCase.wantErr {
				t.Fatalf("Parse() error = %v, wantErr %v", err, inputCase.wantErr)
			}
			if !reflect.DeepEqual(inputCase.expect, config.localRateLimit) {
				t.Fatal("Should be equal")
			}
		})
	}
}

func TestLocalRateLimitApplyRoute(t *testing.T) {
	localRateLimit := localRateLimit{}
	inputCases := []struct {
		config *Ingress
		input  *networking.HTTPRoute
		expect *networking.HTTPRoute
	}{
		{
			config: &Ingress{},
			input:  &networking.HTTPRoute{},
			expect: &networking.HTTPRoute{},
		},
		{
			config: &Ingress{
				localRateLimit: &localRateLimitConfig{
					MaxTokens:     60,
					TokensPerFill: 20,
					FillInterval:  second,
				},
			},
			input: &networking.HTTPRoute{},
			expect: &networking.HTTPRoute{
				RouteHTTPFilters: []*networking.HTTPFilter{
					{
						// TODO: hardcode
						Name: "local-rate-limit",
						Filter: &networking.HTTPFilter_LocalRateLimit{
							LocalRateLimit: &networking.LocalRateLimit{
								TokenBucket: &networking.TokenBucket{
									MaxTokens:     60,
									TokensPefFill: 20,
									FillInterval:  second,
								},
								StatusCode: defaultStatusCode,
							},
						},
					},
				},
			},
		},
	}

	for _, inputCase := range inputCases {
		t.Run("", func(t *testing.T) {
			localRateLimit.ApplyRoute(inputCase.input, inputCase.config)
			if !reflect.DeepEqual(inputCase.input, inputCase.expect) {
				t.Fatal("Should be equal")
			}
		})
	}
}
