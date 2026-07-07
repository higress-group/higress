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
	"errors"
	"net/url"
	"strings"

	"regexp"
)

const (
	defaultMatchAll        = "*"
	defaultAllowMethods    = "GET, PUT, POST, DELETE, PATCH, OPTIONS"
	defaultAllAllowMethods = "GET, PUT, POST, DELETE, PATCH, OPTIONS, HEAD, TRACE, CONNECT"
	defaultAllowHeaders    = "DNT,X-CustomHeader,Keep-Alive,User-Agent,X-Requested-With," +
		"If-Modified-Since,Cache-Control,Content-Type,Authorization"
	defaultMaxAge     = 86400
	protocolHttpName  = "http"
	protocolHttpPort  = "80"
	protocolHttpsName = "https"
	protocolHttpsPort = "443"

	HeaderPluginDebug = "X-Cors-Version"
	HeaderPluginTrace = "X-Cors-Trace"
	HeaderOrigin      = "Origin"
	HttpMethodOptions = "OPTIONS"

	HeaderAccessControlAllowOrigin      = "Access-Control-Allow-Origin"
	HeaderAccessControlAllowMethods     = "Access-Control-Allow-Methods"
	HeaderAccessControlAllowHeaders     = "Access-Control-Allow-Headers"
	HeaderAccessControlAllowCredentials = "Access-Control-Allow-Credentials"
	HeaderAccessControlExposeHeaders    = "Access-Control-Expose-Headers"
	HeaderAccessControlMaxAge           = "Access-Control-Max-Age"

	HeaderControlRequestMethod  = "Access-Control-Request-Method"
	HeaderControlRequestHeaders = "Access-Control-Request-Headers"

	HttpContextKey = "CORS"
)

var portsRegex = regexp.MustCompile(`(.*):\[(\*|\d+(,\d+)*)]`)

type OriginPattern struct {
	declaredPattern string
	pattern         *regexp.Regexp
	patternValue    string
}

func newOriginPatternFromString(declaredPattern string) OriginPattern {
	declaredPattern = strings.ToLower(strings.TrimSuffix(declaredPattern, "/"))
	matches := portsRegex.FindAllStringSubmatch(declaredPattern, -1)
	portList := ""
	patternValue := declaredPattern
	if len(matches) > 0 {
		patternValue = matches[0][1]
		portList = matches[0][2]
	}

	patternValue = "\\Q" + patternValue + "\\E"
	patternValue = strings.ReplaceAll(patternValue, "*", "\\E.*\\Q")
	if len(portList) > 0 {
		if portList == defaultMatchAll {
			patternValue += "(:\\d+)?"
		} else {
			patternValue += ":(" + strings.ReplaceAll(portList, ",", "|") + ")"
		}
	}

	return OriginPattern{
		declaredPattern: declaredPattern,
		patternValue:    patternValue,
		pattern:         regexp.MustCompile(patternValue),
	}
}

type CorsConfig struct {
	// allowOrigins A list of origins for which cross-origin requests are allowed.
	// Be a specific domain, e.g. "https://example.com", or the CORS defined special value  "*"  for all origins.
	// Keep in mind however that the CORS spec does not allow "*" when allowCredentials is set to true, using allowOriginPatterns instead
	// By default, it is set to "*" when allowOriginPatterns is not set too.
	allowOrigins []string

	// allowOriginPatterns A list of origin patterns for which cross-origin requests are allowed
	// origins patterns with "*" anywhere in the host name in addition to port
	// lists  Examples:
	//	 https://*.example.com -- domains ending with example.com
	//	 https://*.example.com:[8080,9090] -- domains ending with example.com on port 8080 or port 9090
	//	 https://*.example.com:[*] -- domains ending with example.com on any port, including the default port
	// The special value "*" allows all origins
	// By default, it is not set.
	allowOriginPatterns []OriginPattern

	// allowMethods  A list of method for which cross-origin requests are allowed
	// The special value "*" allows all methods.
	// By default, it is set to "GET, PUT, POST, DELETE, PATCH, OPTIONS".
	allowMethods []string

	// allowHeaders A list of headers that a pre-flight request can list as allowed
	// The special value "*" allows actual requests to send any header
	// By default, it is set to "DNT,X-CustomHeader,Keep-Alive,User-Agent,X-Requested-With,If-Modified-Since,Cache-Control,Content-Type,Authorization"
	allowHeaders []string

	// exposeHeaders A list of response headers an actual response might have and can be exposed.
	// The special value "*" allows all headers to be exposed for non-credentialed requests.
	// By default, it is not set
	exposeHeaders []string

	// allowCredentials Whether user credentials are supported.
	// By default, it is not set (i.e. user credentials are not supported).
	allowCredentials bool

	// maxAge Configure how long, in seconds, the response from a pre-flight request can be cached by clients.
	// By default, it is set to 86400 seconds.
	maxAge int
}

type HttpCorsContext struct {
	IsValid          bool
	ValidReason      string
	IsPreFlight      bool
	IsCorsRequest    bool
	AllowOrigin      string
	AllowMethods     string
	AllowHeaders     string
	ExposeHeaders    string
	AllowCredentials bool
	MaxAge           int
}

func (c *CorsConfig) GetVersion() string {
	return "1.0.0"
}

func (c *CorsConfig) FillDefaultValues() {
	if len(c.allowOrigins) == 0 && len(c.allowOriginPatterns) == 0 && c.allowCredentials == false {
		c.allowOrigins = []string{defaultMatchAll}
	}
	if len(c.allowHeaders) == 0 {
		c.allowHeaders = []string{defaultAllowHeaders}
	}
	if len(c.allowMethods) == 0 {
		c.allowMethods = strings.Split(defaultAllowMethods, "，")
	}
	if c.maxAge == 0 {
		c.maxAge = defaultMaxAge
	}
}

func (c *CorsConfig) AddAllowOrigin(origin string) error {
	origin = strings.TrimSpace(origin)
	if len(origin) == 0 {
		return nil
	}
	if origin == defaultMatchAll {
		if c.allowCredentials == true {
			return errors.New("can't set origin to * when allowCredentials is true, use AllowOriginPatterns instead")
		}
		c.allowOrigins = []string{defaultMatchAll}
		return nil
	}
	c.allowOrigins = append(c.allowOrigins, strings.TrimSuffix(origin, "/"))
	return nil
}

func (c *CorsConfig) AddAllowHeader(header string) {
	header = strings.TrimSpace(header)
	if len(header) == 0 {
		return
	}
	if header == defaultMatchAll {
		c.allowHeaders = []string{defaultMatchAll}
		return
	}
	c.allowHeaders = append(c.allowHeaders, header)
}

func (c *CorsConfig) AddAllowMethod(method string) {
	method = strings.TrimSpace(method)
	if len(method) == 0 {
		return
	}
	if method == defaultMatchAll {
		c.allowMethods = []string{defaultMatchAll}
		return
	}
	c.allowMethods = append(c.allowMethods, strings.ToUpper(method))
}

func (c *CorsConfig) AddExposeHeader(header string) {
	header = strings.TrimSpace(header)
	if len(header) == 0 {
		return
	}
	if header == defaultMatchAll {
		c.exposeHeaders = []string{defaultMatchAll}
		return
	}
	c.exposeHeaders = append(c.exposeHeaders, header)
}

func (c *CorsConfig) AddAllowOriginPattern(pattern string) {
	pattern = strings.TrimSpace(pattern)
	if len(pattern) == 0 {
		return
	}
	originPattern := newOriginPatternFromString(pattern)
	c.allowOriginPatterns = append(c.allowOriginPatterns, originPattern)
}

func (c *CorsConfig) SetAllowCredentials(allowCredentials bool) error {
	if allowCredentials && len(c.allowOrigins) > 0 && c.allowOrigins[0] == defaultMatchAll {
		return errors.New("can't set allowCredentials to true when allowOrigin is *")
	}
	c.allowCredentials = allowCredentials
	return nil
}

func (c *CorsConfig) SetMaxAge(maxAge int) {
	if maxAge <= 0 {
		c.maxAge = defaultMaxAge
	} else {
		c.maxAge = maxAge
	}
}

func (c *CorsConfig) Process(scheme string, host string, method string, headers [][2]string) (HttpCorsContext, error) {
	scheme = strings.ToLower(strings.TrimSpace(scheme))
	host = strings.ToLower(strings.TrimSpace(host))
	method = strings.ToLower(strings.TrimSpace(method))

	// Init httpCorsContext with default values
	httpCorsContext := HttpCorsContext{IsValid: true, IsPreFlight: false, IsCorsRequest: false, AllowCredentials: false, MaxAge: 0}

	// Get request origin, controlRequestMethod, controlRequestHeaders from http headers
	origin := ""
	controlRequestMethod := ""
	controlRequestHeaders := ""
	for _, header := range headers {
		key := header[0]
		// Get origin
		if strings.ToLower(key) == strings.ToLower(HeaderOrigin) {
			origin = strings.TrimSuffix(strings.TrimSpace(header[1]), "/")
			httpCorsContext.AllowOrigin = origin
		}
		// Get control request method & headers
		if strings.ToLower(key) == strings.ToLower(HeaderControlRequestMethod) {
			controlRequestMethod = strings.TrimSpace(header[1])
			httpCorsContext.AllowMethods = controlRequestMethod
		}
		if strings.ToLower(key) == strings.ToLower(HeaderControlRequestHeaders) {
			controlRequestHeaders = strings.TrimSpace(header[1])
			httpCorsContext.AllowHeaders = controlRequestHeaders
		}
	}

	// Parse if request is CORS and pre-flight request.
	isCorsRequest := c.isCorsRequest(scheme, host, origin)
	isPreFlight := c.isPreFlight(origin, method, controlRequestMethod, controlRequestHeaders)
	httpCorsContext.IsCorsRequest = isCorsRequest
	httpCorsContext.IsPreFlight = isPreFlight

	// Skip when it is not CORS request
	if !isCorsRequest {
		httpCorsContext.IsValid = true
		return httpCorsContext, nil
	}

	if isPreFlight && c.maxAge > 0 {
		httpCorsContext.MaxAge = c.maxAge
	}
	if len(c.exposeHeaders) > 0 {
		httpCorsContext.ExposeHeaders = strings.Join(c.exposeHeaders, ",")
	}
	httpCorsContext.AllowCredentials = c.allowCredentials

	return httpCorsContext, nil
}

func (c *CorsConfig) isPreFlight(origin, method, controllerRequestMethod string, controllerRequestHeaders string) bool {
	return len(origin) > 0 && strings.ToLower(method) == strings.ToLower(HttpMethodOptions) && len(controllerRequestMethod) > 0 && len(controllerRequestHeaders) > 0
}

func (c *CorsConfig) isCorsRequest(scheme, host, origin string) bool {
	if len(origin) == 0 {
		return false
	}

	url, err := url.Parse(strings.TrimSpace(origin))
	if err != nil {
		return false
	}

	// Check scheme
	if strings.ToLower(scheme) != strings.ToLower(url.Scheme) {
		return true
	}

	// Check host and port
	port := ""
	originPort := ""
	originHost := ""
	host, port = c.getHostAndPort(scheme, host)
	originHost, originPort = c.getHostAndPort(url.Scheme, url.Host)
	if host != originHost || port != originPort {
		return true
	}

	return false
}

func (c *CorsConfig) getHostAndPort(scheme string, host string) (string, string) {
	// Get host and port
	scheme = strings.ToLower(scheme)
	host = strings.ToLower(host)
	port := ""
	hosts := strings.Split(host, ":")
	if len(hosts) > 1 {
		host = hosts[0]
		port = hosts[1]
	}
	// Get default port according scheme
	if len(port) == 0 && scheme == protocolHttpName {
		port = protocolHttpPort
	}
	if len(port) == 0 && scheme == protocolHttpsName {
		port = protocolHttpsPort
	}
	return host, port
}

func (c *CorsConfig) GetAllowOrigins(ctx HttpCorsContext) (string, bool) {
	for _, allowOriginPattern := range c.allowOriginPatterns {
		if allowOriginPattern.declaredPattern == defaultMatchAll || allowOriginPattern.pattern.MatchString(ctx.AllowOrigin) {
			return ctx.AllowOrigin, true
		}
	}
	allowOrigins := strings.Join(c.allowOrigins, ",")
	if allowOrigins == defaultMatchAll {
		return ctx.AllowOrigin, true
	}
	return "", false
}

func (c *CorsConfig) GetAllowMethods(ctx HttpCorsContext) string {
	allowMethods := strings.Join(c.allowMethods, ",")
	if allowMethods == defaultMatchAll {
		return ctx.AllowMethods
	} else {
		return allowMethods
	}
}

func (c *CorsConfig) GetAllowHeaders(ctx HttpCorsContext) string {
	allowHeaders := strings.Join(c.allowHeaders, ",")
	if allowHeaders == defaultMatchAll {
		return ctx.AllowHeaders
	} else {
		return allowHeaders
	}
}
