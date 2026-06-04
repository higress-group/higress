// Copyright (c) 2024 Alibaba Group Holding Ltd.
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
	"net/http"
	"path"

	"ext-auth/config"
	"ext-auth/util"

	"github.com/higress-group/wasm-go/pkg/log"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

func main() {}

func init() {
	wrapper.SetCtx(
		"ext-auth",
		wrapper.ParseConfig(config.ParseConfig),
		wrapper.ProcessRequestHeaders(onHttpRequestHeaders),
		wrapper.ProcessRequestBody(onHttpRequestBody),
	)
}

const (
	HeaderAuthorization    = "authorization"
	HeaderFailureModeAllow = "x-envoy-auth-failure-mode-allowed"
)

// Currently, x-forwarded-xxx headers only apply for forward_auth.
const (
	HeaderOriginalMethod   = "x-original-method"
	HeaderOriginalUri      = "x-original-uri"
	HeaderXForwardedProto  = "x-forwarded-proto"
	HeaderXForwardedMethod = "x-forwarded-method"
	HeaderXForwardedUri    = "x-forwarded-uri"
	HeaderXForwardedHost   = "x-forwarded-host"
)

func onHttpRequestHeaders(ctx wrapper.HttpContext, config config.ExtAuthConfig) types.Action {
	// If the request's domain and path match the MatchRules, skip authentication
	if config.MatchRules.IsAllowedByMode(ctx.Host(), ctx.Method(), wrapper.GetRequestPathWithoutQuery()) {
		ctx.DontReadRequestBody()
		return types.ActionContinue
	}

	// Disable the route re-calculation since the plugin may modify some headers related to the chosen route.
	ctx.DisableReroute()

	// If withRequestBody is true AND the HTTP request contains a request body,
	// it will be handled in the onHttpRequestBody phase.
	if wrapper.HasRequestBody() && config.HttpService.AuthorizationRequest.WithRequestBody {
		ctx.SetRequestBodyBufferLimit(config.HttpService.AuthorizationRequest.MaxRequestBodyBytes)
		// The request has a body and requires delaying the header transmission until a cache miss occurs,
		// at which point the header should be sent.
		return types.HeaderStopIteration
	}

	ctx.DontReadRequestBody()
	return checkExtAuth(ctx, config, nil, types.HeaderStopAllIterationAndWatermark)
}

func onHttpRequestBody(ctx wrapper.HttpContext, config config.ExtAuthConfig, body []byte) types.Action {
	if config.HttpService.AuthorizationRequest.WithRequestBody {
		return checkExtAuth(ctx, config, body, types.DataStopIterationAndBuffer)
	}
	return types.ActionContinue
}

func checkExtAuth(ctx wrapper.HttpContext, cfg config.ExtAuthConfig, body []byte, pauseAction types.Action) types.Action {
	httpServiceConfig := cfg.HttpService

	extAuthReqHeaders := buildExtAuthRequestHeaders(ctx, cfg)

	// Set the requestMethod and requestPath based on the endpoint_mode
	requestMethod := httpServiceConfig.RequestMethod
	requestPath := httpServiceConfig.Path
	if httpServiceConfig.EndpointMode == config.EndpointModeEnvoy {
		requestMethod = ctx.Method()
		requestPath = path.Join(httpServiceConfig.PathPrefix, ctx.Path())
	}

	// Call ext auth server
	err := httpServiceConfig.Client.Call(requestMethod, requestPath, util.ReconvertHeaders(extAuthReqHeaders), body,
		func(statusCode int, responseHeaders http.Header, responseBody []byte) {
			if statusCode != http.StatusOK {
				log.Errorf("failed to call ext auth server, status: %d", statusCode)
				callExtAuthServerErrorHandler(cfg, statusCode, responseHeaders, responseBody)
				return
			}

			if httpServiceConfig.AuthorizationResponse.AllowedUpstreamHeaders != nil {
				for headK, headV := range responseHeaders {
					if httpServiceConfig.AuthorizationResponse.AllowedUpstreamHeaders.Match(headK) {
						_ = proxywasm.ReplaceHttpRequestHeader(headK, headV[0])
					}
				}
			}
			proxywasm.ResumeHttpRequest()

		}, httpServiceConfig.Timeout)

	if err != nil {
		log.Errorf("failed to call ext auth server: %v", err)
		// Since the handling logic for call errors and HTTP status code 500 is the same, we directly use 500 here.
		callExtAuthServerErrorHandler(cfg, http.StatusInternalServerError, nil, nil)
		return types.ActionContinue
	}
	return pauseAction
}

// buildExtAuthRequestHeaders builds the request headers to be sent to the ext auth server.
func buildExtAuthRequestHeaders(ctx wrapper.HttpContext, cfg config.ExtAuthConfig) http.Header {
	extAuthReqHeaders := http.Header{}

	httpServiceConfig := cfg.HttpService
	requestConfig := httpServiceConfig.AuthorizationRequest
	reqHeaders, _ := proxywasm.GetHttpRequestHeaders()
	if requestConfig.AllowedHeaders != nil {
		for _, header := range reqHeaders {
			headK := header[0]
			if requestConfig.AllowedHeaders.Match(headK) {
				extAuthReqHeaders.Set(headK, header[1])
			}
		}
	}

	for key, value := range requestConfig.HeadersToAdd {
		extAuthReqHeaders.Set(key, value)
	}

	// Add the Authorization header if present
	authorization := util.ExtractFromHeader(reqHeaders, HeaderAuthorization)
	if authorization != "" {
		extAuthReqHeaders.Set(HeaderAuthorization, authorization)
	}

	// Add filter properties forwarding
	if requestConfig.AllowedProperties != nil {
		for _, prop := range requestConfig.AllowedProperties {
			if raw, err := proxywasm.GetProperty(prop.Path); err == nil {
				extAuthReqHeaders.Set(prop.Header, string(raw))
			}
		}
	}

	// Add additional headers when endpoint_mode is forward_auth
	if httpServiceConfig.EndpointMode == config.EndpointModeForwardAuth {
		// Compatible with older versions
		extAuthReqHeaders.Set(HeaderOriginalMethod, ctx.Method())
		extAuthReqHeaders.Set(HeaderOriginalUri, ctx.Path())
		// Add x-forwarded-xxx headers
		extAuthReqHeaders.Set(HeaderXForwardedProto, ctx.Scheme())
		extAuthReqHeaders.Set(HeaderXForwardedMethod, ctx.Method())
		extAuthReqHeaders.Set(HeaderXForwardedUri, ctx.Path())
		extAuthReqHeaders.Set(HeaderXForwardedHost, ctx.Host())
	}
	return extAuthReqHeaders
}

func callExtAuthServerErrorHandler(config config.ExtAuthConfig, statusCode int, extAuthRespHeaders http.Header, responseBody []byte) {
	if statusCode >= http.StatusInternalServerError && config.FailureModeAllow {
		if config.FailureModeAllowHeaderAdd {
			_ = proxywasm.ReplaceHttpRequestHeader(HeaderFailureModeAllow, "true")
		}
		proxywasm.ResumeHttpRequest()
		return
	}

	var respHeaders = extAuthRespHeaders
	if config.HttpService.AuthorizationResponse.AllowedClientHeaders != nil {
		respHeaders = http.Header{}
		for headK, headV := range extAuthRespHeaders {
			if config.HttpService.AuthorizationResponse.AllowedClientHeaders.Match(headK) {
				respHeaders.Set(headK, headV[0])
			}
		}
	}

	// Rejects client requests with StatusOnError if extAuth is unavailable or returns a 5xx status.
	// Otherwise, uses the status code returned by extAuth to reject requests.
	statusToUse := statusCode
	if statusCode >= http.StatusInternalServerError {
		statusToUse = int(config.StatusOnError)
	}
	_ = util.SendResponse(uint32(statusToUse), "ext-auth.unauthorized", respHeaders, responseBody)
}
