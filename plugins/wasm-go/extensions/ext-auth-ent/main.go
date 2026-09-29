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
	"strings"

	"ext-auth/config"
	"ext-auth/util"

	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/tidwall/gjson"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

func main() {}

func init() {
	wrapper.SetCtx(
		"ext-auth-ent",
		wrapper.ParseConfig(parseEntConfig),
		wrapper.ProcessRequestHeaders(onHttpRequestHeaders),
		wrapper.ProcessRequestBody(onHttpRequestBody),
	)
}

// OPE-8474 R2 企业短票专用变体（方式二）：
// 线上官方 ext-auth（20260811181059）的 match_list 语义与源码相反、matchRules 路由级配置
// 不下发（ecds configuration 为空），两机制都不可用。本变体删掉全部 match 逻辑，
// 改为「路径前缀门」：仅当请求路径命中 entPathPrefixes 时执行 forward_auth 认证，
// 其余路径一律直接放行（不读 body、零开销）。前缀默认硬编码 /v1-ent（test 企业路由），
// 可用配置项 ent_path_prefixes 覆盖（逗号分隔，后续多企业路由时配置化）。
// forward_auth 全部逻辑（回源、X-Original-Api-Key 回写、failure_mode_allow）保持官方实现。
var entPathPrefixes = []string{"/v1-ent"}

const entPathPrefixesConfigKey = "ent_path_prefixes"

// parseEntConfig 在官方 http_service/forward_auth 配置之外，额外解析 ent_path_prefixes。
// 注意：此处不调 SDK log（proxy-wasm host 未初始化时单测会 panic），日志延后到首个请求阶段。
func parseEntConfig(json gjson.Result, cfg *config.ExtAuthConfig) error {
	if err := config.ParseConfig(json, cfg); err != nil {
		return err
	}
	if raw := json.Get(entPathPrefixesConfigKey).String(); raw != "" {
		prefixes := make([]string, 0)
		for _, p := range strings.Split(raw, ",") {
			if p = strings.TrimSpace(p); p != "" {
				prefixes = append(prefixes, p)
			}
		}
		if len(prefixes) > 0 {
			entPathPrefixes = prefixes
		}
	}
	return nil
}

// shouldAuthByPath 前缀门：命中才认证。挂载可放心全局（defaultConfig），
// 未命中前缀的请求零行为变化——这是本变体存在的全部理由。
func shouldAuthByPath(requestPath string) bool {
	for _, prefix := range entPathPrefixes {
		if strings.HasPrefix(requestPath, prefix) {
			return true
		}
	}
	return false
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
	// 路径前缀门：未命中企业路由前缀的请求直接放行（替代官方 MatchRules，语义与源码一致、可预测）
	if !shouldAuthByPath(wrapper.GetRequestPathWithoutQuery()) {
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
