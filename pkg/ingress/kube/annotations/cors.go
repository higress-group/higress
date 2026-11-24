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
	"net/url"
	"strings"

	"github.com/golang/protobuf/ptypes/duration"
	"github.com/golang/protobuf/ptypes/wrappers"
	networking "istio.io/api/networking/v1alpha3"
)

const (
	// annotation key
	enableCors       = "enable-cors"
	allowOrigin      = "cors-allow-origin"
	allowMethods     = "cors-allow-methods"
	allowHeaders     = "cors-allow-headers"
	exposeHeaders    = "cors-expose-headers"
	allowCredentials = "cors-allow-credentials"
	maxAge           = "cors-max-age"

	// default annotation value
	defaultAllowOrigin  = "*"
	defaultAllowMethods = "GET, PUT, POST, DELETE, PATCH, OPTIONS"
	defaultAllowHeaders = "DNT,X-CustomHeader,Keep-Alive,User-Agent,X-Requested-With," +
		"If-Modified-Since,Cache-Control,Content-Type,Authorization"
	defaultAllowCredentials = true
	defaultMaxAge           = 1728000
)

var (
	_ Parser       = &cors{}
	_ RouteHandler = &cors{}
)

type CorsConfig struct {
	Enabled          bool
	AllowOrigin      []string
	AllowMethods     []string
	AllowHeaders     []string
	ExposeHeaders    []string
	AllowCredentials bool
	MaxAge           int
}

type cors struct{}

func (c cors) Parse(annotations Annotations, config *Ingress, _ *GlobalContext) error {
	if !needCorsConfig(annotations) {
		return nil
	}

	// cors enable
	enable, _ := annotations.ParseBoolASAP(enableCors)
	if !enable {
		return nil
	}

	corsConfig := &CorsConfig{
		Enabled:          enable,
		AllowOrigin:      []string{defaultAllowOrigin},
		AllowMethods:     splitStringWithSpaceTrim(defaultAllowMethods),
		AllowHeaders:     splitStringWithSpaceTrim(defaultAllowHeaders),
		AllowCredentials: defaultAllowCredentials,
		MaxAge:           defaultMaxAge,
	}

	defer func() {
		config.Cors = corsConfig
	}()

	// allow origin
	if origin, err := annotations.ParseStringASAP(allowOrigin); err == nil {
		corsConfig.AllowOrigin = splitStringWithSpaceTrim(origin)
	}

	// allow methods
	if methods, err := annotations.ParseStringASAP(allowMethods); err == nil {
		corsConfig.AllowMethods = splitStringWithSpaceTrim(methods)
	}

	// allow headers
	if headers, err := annotations.ParseStringASAP(allowHeaders); err == nil {
		corsConfig.AllowHeaders = splitStringWithSpaceTrim(headers)
	}

	// expose headers
	if exposeHeaders, err := annotations.ParseStringASAP(exposeHeaders); err == nil {
		corsConfig.ExposeHeaders = splitStringWithSpaceTrim(exposeHeaders)
	}

	// allow credentials
	if allowCredentials, err := annotations.ParseBoolASAP(allowCredentials); err == nil {
		corsConfig.AllowCredentials = allowCredentials
	}

	// max age
	// A negative max age is not a usable duration; ignore it like any other
	// unparsable annotation instead of forwarding it to Envoy.
	if age, err := annotations.ParseIntASAP(maxAge); err == nil && age >= 0 {
		corsConfig.MaxAge = age
	}

	return nil
}

func (c cors) ApplyRoute(route *networking.HTTPRoute, config *Ingress) {
	corsConfig := config.Cors
	if corsConfig == nil || !corsConfig.Enabled {
		return
	}

	corsPolicy := &networking.CorsPolicy{
		AllowMethods:  corsConfig.AllowMethods,
		AllowHeaders:  corsConfig.AllowHeaders,
		ExposeHeaders: corsConfig.ExposeHeaders,
		AllowCredentials: &wrappers.BoolValue{
			Value: corsConfig.AllowCredentials,
		},
		MaxAge: &duration.Duration{
			Seconds: int64(corsConfig.MaxAge),
		},
	}

	var allowOrigins []*networking.StringMatch
	for _, origin := range corsConfig.AllowOrigin {
		if origin == "*" {
			allowOrigins = append(allowOrigins, &networking.StringMatch{
				MatchType: &networking.StringMatch_Regex{
					Regex: ".*",
				},
			})
			break
		}
		if strings.Contains(origin, "*") {
			parsedURL, err := url.Parse(origin)
			if err != nil {
				continue
			}
			if strings.HasPrefix(parsedURL.Host, "*") {
				var sb strings.Builder
				sb.WriteString(".*")
				for idx, char := range parsedURL.Host {
					if idx == 0 {
						continue
					}

					if char == '.' {
						sb.WriteString("\\")
					}

					sb.WriteString(string(char))
				}

				allowOrigins = append(allowOrigins, &networking.StringMatch{
					MatchType: &networking.StringMatch_Regex{
						Regex: sb.String(),
					},
				})
			}
			continue
		}

		allowOrigins = append(allowOrigins, &networking.StringMatch{
			MatchType: &networking.StringMatch_Exact{
				Exact: origin,
			},
		})
	}
	corsPolicy.AllowOrigins = allowOrigins

	route.CorsPolicy = corsPolicy
}

func needCorsConfig(annotations Annotations) bool {
	return annotations.HasASAP(enableCors)
}

func splitStringWithSpaceTrim(input string) []string {
	out := strings.Split(input, ",")
	for i, item := range out {
		converted := strings.TrimSpace(item)
		if converted == "*" {
			return []string{"*"}
		}
		out[i] = converted
	}
	return out
}
