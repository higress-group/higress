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

package config

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestCorsConfig_getHostAndPort(t *testing.T) {

	tests := []struct {
		name     string
		scheme   string
		host     string
		wantHost string
		wantPort string
	}{
		{
			name:     "http without port",
			scheme:   "http",
			host:     "http.example.com",
			wantHost: "http.example.com",
			wantPort: "80",
		},
		{
			name:     "https without port",
			scheme:   "https",
			host:     "http.example.com",
			wantHost: "http.example.com",
			wantPort: "443",
		},

		{
			name:     "http with port and case insensitive",
			scheme:   "hTTp",
			host:     "hTTp.Example.com:8080",
			wantHost: "http.example.com",
			wantPort: "8080",
		},

		{
			name:     "https with port and case insensitive",
			scheme:   "hTTps",
			host:     "hTTp.Example.com:8080",
			wantHost: "http.example.com",
			wantPort: "8080",
		},

		{
			name:     "protocol is not http",
			scheme:   "wss",
			host:     "hTTp.Example.com",
			wantHost: "http.example.com",
			wantPort: "",
		},

		{
			name:     "protocol is not http",
			scheme:   "wss",
			host:     "hTTp.Example.com:8080",
			wantHost: "http.example.com",
			wantPort: "8080",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &CorsConfig{}
			host, port := c.getHostAndPort(tt.scheme, tt.host)
			assert.Equal(t, tt.wantHost, host)
			assert.Equal(t, tt.wantPort, port)
		})
	}
}

func TestCorsConfig_isCorsRequest(t *testing.T) {
	tests := []struct {
		name   string
		scheme string
		host   string
		origin string
		want   bool
	}{
		{
			name:   "blank origin",
			scheme: "http",
			host:   "httpbin.example.com",
			origin: "",
			want:   false,
		},
		{
			name:   "normal equal case with space and case ",
			scheme: "http",
			host:   "httpbin.example.com",
			origin: "http://hTTPbin.Example.com",
			want:   false,
		},

		{
			name:   "cors request with port diff",
			scheme: "http",
			host:   "httpbin.example.com",
			origin: " http://httpbin.example.com:8080 ",
			want:   true,
		},
		{
			name:   "cors request with scheme diff",
			scheme: "http",
			host:   "httpbin.example.com",
			origin: " https://HTTPpbin.Example.com ",
			want:   true,
		},
		{
			name:   "cors request with host diff",
			scheme: "http",
			host:   "httpbin.example.com",
			origin: " http://HTTPpbin.Example.org ",
			want:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &CorsConfig{}
			assert.Equalf(t, tt.want, c.isCorsRequest(tt.scheme, tt.host, tt.origin), "isCorsRequest(%v, %v, %v)", tt.scheme, tt.host, tt.origin)
		})
	}
}

func TestCorsConfig_isPreFlight(t *testing.T) {
	tests := []struct {
		name                    string
		origin                  string
		method                  string
		controllerRequestMethod string
		want                    bool
	}{
		{
			name:                    "blank case",
			origin:                  "",
			method:                  "",
			controllerRequestMethod: "",
			want:                    false,
		},
		{
			name:                    "normal case",
			origin:                  "http://httpbin.example.com",
			method:                  "Options",
			controllerRequestMethod: "PUT",
			want:                    true,
		},
		{
			name:                    "bad case with diff method",
			origin:                  "http://httpbin.example.com",
			method:                  "GET",
			controllerRequestMethod: "PUT",
			want:                    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &CorsConfig{}
			assert.Equalf(t, tt.want, c.isPreFlight(tt.origin, tt.method, tt.controllerRequestMethod), "isPreFlight(%v, %v, %v)", tt.origin, tt.method, tt.controllerRequestMethod)
		})
	}
}

func TestCorsConfig_checkMethods(t *testing.T) {
	tests := []struct {
		name          string
		allowMethods  []string
		requestMethod string
		wantMethods   string
		wantOk        bool
	}{
		{
			name:          "default *",
			allowMethods:  []string{"*"},
			requestMethod: "GET",
			wantMethods:   defaultAllAllowMethods,
			wantOk:        true,
		},
		{
			name:          "normal allow case",
			allowMethods:  []string{"GET", "PUT", "HEAD"},
			requestMethod: "get",
			wantMethods:   "GET,PUT,HEAD",
			wantOk:        true,
		},
		{
			name:          "forbidden case",
			allowMethods:  []string{"GET", "PUT", "HEAD"},
			requestMethod: "POST",
			wantMethods:   "",
			wantOk:        false,
		},

		{
			name:          "blank method",
			allowMethods:  []string{"GET", "PUT", "HEAD"},
			requestMethod: "",
			wantMethods:   "",
			wantOk:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &CorsConfig{
				allowMethods: tt.allowMethods,
			}
			allowMethods, allowOk := c.checkMethods(tt.requestMethod)
			assert.Equalf(t, tt.wantMethods, allowMethods, "checkMethods(%v)", tt.requestMethod)
			assert.Equalf(t, tt.wantOk, allowOk, "checkMethods(%v)", tt.requestMethod)
		})
	}
}

func TestCorsConfig_checkHeaders(t *testing.T) {
	tests := []struct {
		name           string
		allowHeaders   []string
		requestHeaders string
		wantHeaders    string
		wantOk         bool
	}{
		{
			name:           "not pre-flight",
			allowHeaders:   []string{"Content-Type", "Authorization"},
			requestHeaders: "",
			wantHeaders:    "Content-Type,Authorization",
			wantOk:         true,
		},
		{
			name:           "blank allowheaders case 1",
			allowHeaders:   []string{},
			requestHeaders: "",
			wantHeaders:    "",
			wantOk:         false,
		},
		{
			name:           "blank allowheaders case 2",
			requestHeaders: "Authorization",
			wantHeaders:    "",
			wantOk:         false,
		},

		{
			name:           "allowheaders *",
			allowHeaders:   []string{"*"},
			requestHeaders: "Content-Type,Authorization",
			wantHeaders:    "Content-Type,Authorization",
			wantOk:         true,
		},

		{
			name:           "allowheader values 1",
			allowHeaders:   []string{"Content-Type", "Authorization"},
			requestHeaders: "Content-Type,Authorization",
			wantHeaders:    "Content-Type,Authorization",
			wantOk:         true,
		},

		{
			name:           "allowheader values 2",
			allowHeaders:   []string{"Content-Type", "Authorization"},
			requestHeaders: "",
			wantHeaders:    "Content-Type,Authorization",
			wantOk:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &CorsConfig{
				allowHeaders: tt.allowHeaders,
			}
			allowHeaders, allowOk := c.checkHeaders(tt.requestHeaders)
			assert.Equalf(t, tt.wantHeaders, allowHeaders, "checkHeaders(%v)", tt.requestHeaders)
			assert.Equalf(t, tt.wantOk, allowOk, "checkHeaders(%v)", tt.requestHeaders)
		})
	}
}

func TestCorsConfig_checkOrigin(t *testing.T) {

	tests := []struct {
		name                string
		allowOrigins        []string
		allowOriginPatterns []OriginPattern
		origin              string
		wantOrigin          string
		wantOk              bool
	}{
		{
			name:                "allowOrigins *",
			allowOrigins:        []string{defaultMatchAll},
			allowOriginPatterns: []OriginPattern{},
			origin:              "http://Httpbin.Example.COM",
			wantOrigin:          "http://Httpbin.Example.COM",
			wantOk:              true,
		},

		{
			name:                "allowOrigins exact match case 1",
			allowOrigins:        []string{"http://httpbin.example.com"},
			allowOriginPatterns: []OriginPattern{},
			origin:              "http://HTTPBin.EXample.COM",
			wantOrigin:          "http://HTTPBin.EXample.COM",
			wantOk:              true,
		},
		{
			name:                "allowOrigins exact match case 2",
			allowOrigins:        []string{"https://httpbin.example.com"},
			allowOriginPatterns: []OriginPattern{},
			origin:              "http://HTTPBin.EXample.COM",
			wantOrigin:          "",
			wantOk:              false,
		},

		{
			name:         "OriginPattern pattern match with *",
			allowOrigins: []string{},
			allowOriginPatterns: []OriginPattern{
				newOriginPatternFromString("*"),
			},
			origin:     "http://HTTPBin.EXample.COM",
			wantOrigin: "http://HTTPBin.EXample.COM",
			wantOk:     true,
		},

		{
			name:         "OriginPattern pattern match case with any port",
			allowOrigins: []string{},
			allowOriginPatterns: []OriginPattern{
				newOriginPatternFromString("http://*.example.com:[*]"),
			},
			origin:     "http://HTTPBin.EXample.COM",
			wantOrigin: "http://HTTPBin.EXample.COM",
			wantOk:     true,
		},
		{
			name:         "OriginPattern pattern match case with any port",
			allowOrigins: []string{},
			allowOriginPatterns: []OriginPattern{
				newOriginPatternFromString("http://*.example.com:[*]"),
			},
			origin:     "http://HTTPBin.EXample.COM:10000",
			wantOrigin: "http://HTTPBin.EXample.COM:10000",
			wantOk:     true,
		},

		{
			name:         "OriginPattern pattern match case with special port 1",
			allowOrigins: []string{},
			allowOriginPatterns: []OriginPattern{
				newOriginPatternFromString("http://*.example.com:[8080,9090]"),
			},
			origin:     "http://HTTPBin.EXample.COM:10000",
			wantOrigin: "",
			wantOk:     false,
		},

		{
			name:         "OriginPattern pattern match case with special port 2",
			allowOrigins: []string{},
			allowOriginPatterns: []OriginPattern{
				newOriginPatternFromString("http://*.example.com:[8080,9090]"),
			},
			origin:     "http://HTTPBin.EXample.COM:9090",
			wantOrigin: "http://HTTPBin.EXample.COM:9090",
			wantOk:     true,
		},

		{
			name:         "OriginPattern pattern match case with special port 3",
			allowOrigins: []string{},
			allowOriginPatterns: []OriginPattern{
				newOriginPatternFromString("http://*.example.com:[8080,9090]"),
			},
			origin:     "http://HTTPBin.EXample.org:9090",
			wantOrigin: "",
			wantOk:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &CorsConfig{
				allowOrigins:        tt.allowOrigins,
				allowOriginPatterns: tt.allowOriginPatterns,
			}
			allowOrigin, allowOk := c.checkOrigin(tt.origin)
			assert.Equalf(t, tt.wantOrigin, allowOrigin, "checkOrigin(%v)", tt.origin)
			assert.Equalf(t, tt.wantOk, allowOk, "checkOrigin(%v)", tt.origin)
		})
	}
}
