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
	"github.com/golang/protobuf/ptypes/wrappers"
	networking "istio.io/api/networking/v1alpha3"
)

func TestSplitStringWithSpaceTrim(t *testing.T) {
	testCases := []struct {
		input  string
		expect []string
	}{
		{
			input:  "*",
			expect: []string{"*"},
		},
		{
			input:  "a, b, c",
			expect: []string{"a", "b", "c"},
		},
		{
			input:  "a, *, c",
			expect: []string{"*"},
		},
	}

	for _, testCase := range testCases {
		t.Run("", func(t *testing.T) {
			result := splitStringWithSpaceTrim(testCase.input)
			if !reflect.DeepEqual(testCase.expect, result) {
				t.Fatalf("Must be equal, but got %s", result)
			}
		})
	}
}

func TestCorsParse(t *testing.T) {
	cors := cors{}
	testCases := []struct {
		input  Annotations
		expect *CorsConfig
	}{
		{
			input:  Annotations{},
			expect: nil,
		},
		{
			input: Annotations{
				buildNginxAnnotationKey(enableCors): "false",
			},
			expect: nil,
		},
		{
			input: Annotations{
				buildNginxAnnotationKey(enableCors): "true",
			},
			expect: &CorsConfig{
				Enabled:          true,
				AllowOrigin:      []string{defaultAllowOrigin},
				AllowMethods:     splitStringWithSpaceTrim(defaultAllowMethods),
				AllowHeaders:     splitStringWithSpaceTrim(defaultAllowHeaders),
				AllowCredentials: defaultAllowCredentials,
				MaxAge:           defaultMaxAge,
			},
		},
		{
			input: Annotations{
				buildNginxAnnotationKey(enableCors):  "true",
				buildNginxAnnotationKey(allowOrigin): "https://origin-site.com:4443, http://origin-site.com, https://example.org:1199",
			},
			expect: &CorsConfig{
				Enabled:          true,
				AllowOrigin:      []string{"https://origin-site.com:4443", "http://origin-site.com", "https://example.org:1199"},
				AllowMethods:     splitStringWithSpaceTrim(defaultAllowMethods),
				AllowHeaders:     splitStringWithSpaceTrim(defaultAllowHeaders),
				AllowCredentials: defaultAllowCredentials,
				MaxAge:           defaultMaxAge,
			},
		},
		{
			input: Annotations{
				buildNginxAnnotationKey(enableCors):   "true",
				buildNginxAnnotationKey(allowOrigin):  "https://origin-site.com:4443, http://origin-site.com, https://example.org:1199",
				buildNginxAnnotationKey(allowMethods): "GET, PUT",
				buildNginxAnnotationKey(allowHeaders): "foo,bar",
			},
			expect: &CorsConfig{
				Enabled:          true,
				AllowOrigin:      []string{"https://origin-site.com:4443", "http://origin-site.com", "https://example.org:1199"},
				AllowMethods:     []string{"GET", "PUT"},
				AllowHeaders:     []string{"foo", "bar"},
				AllowCredentials: defaultAllowCredentials,
				MaxAge:           defaultMaxAge,
			},
		},
		{
			input: Annotations{
				buildNginxAnnotationKey(enableCors):       "true",
				buildNginxAnnotationKey(allowOrigin):      "https://origin-site.com:4443, http://origin-site.com, https://example.org:1199",
				buildNginxAnnotationKey(allowMethods):     "GET, PUT",
				buildNginxAnnotationKey(allowHeaders):     "foo,bar",
				buildNginxAnnotationKey(allowCredentials): "false",
				buildNginxAnnotationKey(maxAge):           "100",
			},
			expect: &CorsConfig{
				Enabled:          true,
				AllowOrigin:      []string{"https://origin-site.com:4443", "http://origin-site.com", "https://example.org:1199"},
				AllowMethods:     []string{"GET", "PUT"},
				AllowHeaders:     []string{"foo", "bar"},
				AllowCredentials: false,
				MaxAge:           100,
			},
		},
		{
			input: Annotations{
				buildHigressAnnotationKey(enableCors):     "true",
				buildNginxAnnotationKey(allowOrigin):      "https://origin-site.com:4443, http://origin-site.com, https://example.org:1199",
				buildHigressAnnotationKey(allowMethods):   "GET, PUT",
				buildNginxAnnotationKey(allowHeaders):     "foo,bar",
				buildNginxAnnotationKey(allowCredentials): "false",
				buildNginxAnnotationKey(maxAge):           "100",
			},
			expect: &CorsConfig{
				Enabled:          true,
				AllowOrigin:      []string{"https://origin-site.com:4443", "http://origin-site.com", "https://example.org:1199"},
				AllowMethods:     []string{"GET", "PUT"},
				AllowHeaders:     []string{"foo", "bar"},
				AllowCredentials: false,
				MaxAge:           100,
			},
		},
	}

	for _, testCase := range testCases {
		t.Run("", func(t *testing.T) {
			config := &Ingress{}
			_ = cors.Parse(testCase.input, config, nil)
			if !reflect.DeepEqual(config.Cors, testCase.expect) {
				t.Fatalf("Must be equal.")
			}
		})
	}
}

func TestCorsApplyRoute(t *testing.T) {
	cors := cors{}
	testCases := []struct {
		route  *networking.HTTPRoute
		config *Ingress
		expect *networking.HTTPRoute
	}{
		{
			route:  &networking.HTTPRoute{},
			config: &Ingress{},
			expect: &networking.HTTPRoute{},
		},
		{
			route: &networking.HTTPRoute{},
			config: &Ingress{
				Cors: &CorsConfig{
					Enabled: false,
				},
			},
			expect: &networking.HTTPRoute{},
		},
		{
			route: &networking.HTTPRoute{},
			config: &Ingress{
				Cors: &CorsConfig{
					Enabled:          true,
					AllowOrigin:      []string{"https://origin-site.com:4443", "http://origin-site.com", "https://example.org:1199"},
					AllowMethods:     []string{"GET", "POST"},
					AllowHeaders:     []string{"test", "unique"},
					ExposeHeaders:    []string{"hello", "bye"},
					AllowCredentials: defaultAllowCredentials,
					MaxAge:           defaultMaxAge,
				},
			},
			expect: &networking.HTTPRoute{
				CorsPolicy: &networking.CorsPolicy{
					AllowOrigins: []*networking.StringMatch{
						{
							MatchType: &networking.StringMatch_Exact{
								Exact: "https://origin-site.com:4443",
							},
						},
						{
							MatchType: &networking.StringMatch_Exact{
								Exact: "http://origin-site.com",
							},
						},
						{
							MatchType: &networking.StringMatch_Exact{
								Exact: "https://example.org:1199",
							},
						},
					},
					AllowMethods:  []string{"GET", "POST"},
					AllowHeaders:  []string{"test", "unique"},
					ExposeHeaders: []string{"hello", "bye"},
					AllowCredentials: &wrappers.BoolValue{
						Value: true,
					},
					MaxAge: &duration.Duration{
						Seconds: defaultMaxAge,
					},
				},
			},
		},
		{
			route: &networking.HTTPRoute{},
			config: &Ingress{
				Cors: &CorsConfig{
					Enabled:          true,
					AllowOrigin:      []string{"https://*.origin-site.com:4443", "http://*.origin-site.com", "https://example.org:1199"},
					AllowMethods:     []string{"GET", "POST"},
					AllowHeaders:     []string{"test", "unique"},
					ExposeHeaders:    []string{"hello", "bye"},
					AllowCredentials: defaultAllowCredentials,
					MaxAge:           defaultMaxAge,
				},
			},
			expect: &networking.HTTPRoute{
				CorsPolicy: &networking.CorsPolicy{
					AllowOrigins: []*networking.StringMatch{
						{
							MatchType: &networking.StringMatch_Regex{
								Regex: ".*\\.origin-site\\.com:4443",
							},
						},
						{
							MatchType: &networking.StringMatch_Regex{
								Regex: ".*\\.origin-site\\.com",
							},
						},
						{
							MatchType: &networking.StringMatch_Exact{
								Exact: "https://example.org:1199",
							},
						},
					},
					AllowMethods:  []string{"GET", "POST"},
					AllowHeaders:  []string{"test", "unique"},
					ExposeHeaders: []string{"hello", "bye"},
					AllowCredentials: &wrappers.BoolValue{
						Value: true,
					},
					MaxAge: &duration.Duration{
						Seconds: defaultMaxAge,
					},
				},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run("", func(t *testing.T) {
			cors.ApplyRoute(testCase.route, testCase.config)
			if !reflect.DeepEqual(testCase.route, testCase.expect) {
				t.Fatal("Must be equal.")
			}
		})
	}
}

func TestCorsParseNegativeMaxAge(t *testing.T) {
	config := &Ingress{}
	annotations := Annotations{
		buildNginxAnnotationKey(enableCors): "true",
		buildNginxAnnotationKey(maxAge):     "-100",
	}
	if err := (cors{}).Parse(annotations, config, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config.Cors == nil {
		t.Fatal("expected cors config")
	}
	if config.Cors.MaxAge != defaultMaxAge {
		t.Fatalf("a negative max age must be ignored, got %d", config.Cors.MaxAge)
	}
}
