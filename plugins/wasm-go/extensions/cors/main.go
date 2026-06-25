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

package main

import (
	"cors/config"
	"fmt"
	"net/http"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

func main() {}

func init() {
	wrapper.SetCtx(
		"cors",
		wrapper.ParseConfigBy(parseConfig),
		wrapper.ProcessRequestHeadersBy(onHttpRequestHeaders),
		wrapper.ProcessResponseHeadersBy(onHttpResponseHeaders),
	)
}

func parseConfig(json gjson.Result, corsConfig *config.CorsConfig, log log.Log) error {
	log.Debug("parseConfig()")
	allowOrigins := json.Get("allow_origins").Array()
	for _, origin := range allowOrigins {
		if err := corsConfig.AddAllowOrigin(origin.String()); err != nil {
			log.Warnf("failed to AddAllowOrigin:%s, error:%v", origin, err)
		}
	}
	allowOriginPatterns := json.Get("allow_origin_patterns").Array()
	for _, pattern := range allowOriginPatterns {
		corsConfig.AddAllowOriginPattern(pattern.String())
	}
	allowMethods := json.Get("allow_methods").Array()
	for _, method := range allowMethods {
		corsConfig.AddAllowMethod(method.String())
	}
	allowHeaders := json.Get("allow_headers").Array()
	for _, header := range allowHeaders {
		corsConfig.AddAllowHeader(header.String())
	}
	exposeHeaders := json.Get("expose_headers").Array()
	for _, header := range exposeHeaders {
		corsConfig.AddExposeHeader(header.String())
	}
	allowCredentials := json.Get("allow_credentials").Bool()
	if err := corsConfig.SetAllowCredentials(allowCredentials); err != nil {
		log.Warnf("failed to set AllowCredentials error: %v", err)
	}
	maxAge := json.Get("max_age").Int()
	corsConfig.SetMaxAge(int(maxAge))

	// Fill default values
	corsConfig.FillDefaultValues()
	log.Debugf("corsConfig:%+v", corsConfig)
	return nil
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, corsConfig config.CorsConfig, log log.Log) types.Action {
	ctx.DisableReroute()
	log.Debug("onHttpRequestHeaders()")
	requestUrl, _ := proxywasm.GetHttpRequestHeader(":path")
	method, _ := proxywasm.GetHttpRequestHeader(":method")
	host := ctx.Host()
	scheme := ctx.Scheme()
	log.Debugf("scheme:%s, host:%s, method:%s, request:%s", scheme, host, method, requestUrl)
	// Get headers
	headers, _ := proxywasm.GetHttpRequestHeaders()
	// Process request
	httpCorsContext, err := corsConfig.Process(scheme, host, method, headers)
	if err != nil {
		log.Warnf("failed to process %s : %v", requestUrl, err)
		return types.ActionContinue
	}
	log.Debugf("Process httpCorsContext:%+v", httpCorsContext)
	// Set HttpContext
	ctx.SetContext(config.HttpContextKey, httpCorsContext)

	// Response directly when it is cors preflight request
	if httpCorsContext.IsPreFlight {
		headers := make([][2]string, 0)
		headers = append(headers, [2]string{config.HeaderPluginTrace, "trace"})
		proxywasm.SendHttpResponseWithDetail(http.StatusOK, "cores.preflight", headers, nil, -1)
		return types.ActionPause
	}

	return types.ActionContinue
}

func onHttpResponseHeaders(ctx wrapper.HttpContext, corsConfig config.CorsConfig, log log.Log) types.Action {
	log.Debug("onHttpResponseHeaders()")
	// Remove trace header if existed
	proxywasm.RemoveHttpResponseHeader(config.HeaderPluginTrace)
	// Remove upstream cors response headers if existed
	proxywasm.RemoveHttpResponseHeader(config.HeaderAccessControlAllowOrigin)
	proxywasm.RemoveHttpResponseHeader(config.HeaderAccessControlAllowMethods)
	proxywasm.RemoveHttpResponseHeader(config.HeaderAccessControlExposeHeaders)
	proxywasm.RemoveHttpResponseHeader(config.HeaderAccessControlAllowCredentials)
	proxywasm.RemoveHttpResponseHeader(config.HeaderAccessControlMaxAge)
	// Add debug header
	proxywasm.AddHttpResponseHeader(config.HeaderPluginDebug, corsConfig.GetVersion())

	// Restore httpCorsContext from HttpContext
	httpCorsContext, ok := ctx.GetContext(config.HttpContextKey).(config.HttpCorsContext)
	if !ok {
		log.Debug("restore httpCorsContext from HttpContext error")
		return types.ActionContinue
	}
	log.Debugf("Restore httpCorsContext:%+v", httpCorsContext)

	// Skip which it is not valid or not cors request
	if !httpCorsContext.IsValid || !httpCorsContext.IsCorsRequest {
		return types.ActionContinue
	}

	// Add the Vary: Origin response header to prevent caching from causing CORS (Cross-Origin Resource Sharing) issues.
	proxywasm.AddHttpResponseHeader("Vary", "Origin")

	// Add Cors headers when it is cors and valid request
	allowOrigins, ok := corsConfig.GetAllowOrigins(httpCorsContext)
	if ok {
		proxywasm.AddHttpResponseHeader(config.HeaderAccessControlAllowOrigin, allowOrigins)
	}
	proxywasm.AddHttpResponseHeader(config.HeaderAccessControlAllowMethods, corsConfig.GetAllowMethods(httpCorsContext))

	proxywasm.AddHttpResponseHeader(config.HeaderAccessControlAllowHeaders, corsConfig.GetAllowHeaders(httpCorsContext))

	if len(httpCorsContext.ExposeHeaders) > 0 {
		proxywasm.AddHttpResponseHeader(config.HeaderAccessControlExposeHeaders, httpCorsContext.ExposeHeaders)
	}
	if httpCorsContext.AllowCredentials {
		proxywasm.AddHttpResponseHeader(config.HeaderAccessControlAllowCredentials, "true")
	}
	if httpCorsContext.MaxAge > 0 {
		proxywasm.AddHttpResponseHeader(config.HeaderAccessControlMaxAge, fmt.Sprintf("%d", httpCorsContext.MaxAge))
	}

	return types.ActionContinue
}
