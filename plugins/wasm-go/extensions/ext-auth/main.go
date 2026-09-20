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
	"slices"
	"strconv"
	"strings"

	"ext-auth/config"
	"ext-auth/expr"
	"ext-auth/extract"
	"ext-auth/util"

	"github.com/higress-group/wasm-go/pkg/log"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/resp"
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
	matchRequest, err := buildMatchRequest(ctx, config.MatchRules)
	if err != nil {
		log.Errorf("failed to read request headers for match rules: %v; continuing external authorization", err)
	} else if !config.MatchRules.Matches(matchRequest) {
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

func buildMatchRequest(ctx wrapper.HttpContext, matchRules expr.MatchRules) (expr.RequestAttributes, error) {
	request := expr.RequestAttributes{
		Domain: ctx.Host(),
		Method: ctx.Method(),
		Path:   wrapper.GetRequestPathWithoutQuery(),
	}
	if !matchRules.RequiresRequestHeaders() {
		return request, nil
	}

	requestHeaders, err := proxywasm.GetHttpRequestHeaders()
	if err != nil {
		return expr.RequestAttributes{}, err
	}
	request.HeaderNames = expr.NewHeaderNameSet(requestHeaders)
	return request, nil
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

	// The auth-result cache is opt-in. It is skipped when the request body is
	// forwarded to the auth server, since the body influences the decision but is
	// not part of the cache key. A hit replays a previous allow; any miss or cache
	// error fails open to a real authorization call.
	if cfg.Cache.Enabled && !httpServiceConfig.AuthorizationRequest.WithRequestBody {
		// Two key modes: the default hashes the full forwarded request; a configured
		// cache.key_fields narrows the key to the method, the query-free path, and the
		// listed request fields, so a per-request-unique credential no longer defeats it.
		// key_fields reads the raw inbound method/path and headers, not the
		// auth-server-bound requestMethod/requestPath or the filtered forwarded set: the
		// listed fields describe the client request, so this keeps the key
		// endpoint_mode-independent and lets a field the auth server never sees still
		// distinguish cache entries.
		var cacheKey string
		if len(cfg.Cache.KeyFields) > 0 {
			inboundHeaders, _ := proxywasm.GetHttpRequestHeaders()
			cacheKey = buildRequestFieldCacheKey(ctx.Method(), ctx.Path(), inboundHeaders, cfg.Cache.KeyFields)
		} else {
			cacheKey = buildCacheKey(requestMethod, requestPath, extAuthReqHeaders)
		}
		err := cfg.Cache.Client.Get(cacheKey, func(response resp.Value) {
			if injected, ok := decodeCachedHeaders(response); ok {
				for name, value := range injected {
					_ = proxywasm.ReplaceHttpRequestHeader(name, value)
				}
				proxywasm.ResumeHttpRequest()
				return
			}
			callExtAuthServer(cfg, body, requestMethod, requestPath, extAuthReqHeaders, cacheKey, pauseAction)
		})
		if err == nil {
			return pauseAction
		}
		// The cache lookup could not even be dispatched; fall back to a direct,
		// uncached authorization call.
		log.Errorf("failed to look up ext-auth cache, falling back to the auth server: %v", err)
	}

	return callExtAuthServer(cfg, body, requestMethod, requestPath, extAuthReqHeaders, "", pauseAction)
}

// callExtAuthServer invokes the external authorization server and applies its
// decision. cacheKey is non-empty only when a successful allow should be written
// back to the cache.
func callExtAuthServer(cfg config.ExtAuthConfig, body []byte, requestMethod, requestPath string, extAuthReqHeaders http.Header, cacheKey string, pauseAction types.Action) types.Action {
	httpServiceConfig := cfg.HttpService

	// Call ext auth server
	err := httpServiceConfig.Client.Call(requestMethod, requestPath, util.ReconvertHeaders(extAuthReqHeaders), body,
		func(statusCode int, responseHeaders http.Header, responseBody []byte) {
			if statusCode != http.StatusOK {
				log.Errorf("failed to call ext auth server, status: %d", statusCode)
				callExtAuthServerErrorHandler(cfg, statusCode, responseHeaders, responseBody)
				return
			}

			// When success_condition is configured, a 200 authorization response must
			// also satisfy it; otherwise the client request is rejected.
			if len(cfg.SuccessCondition) > 0 && !evaluateSuccessCondition(cfg.SuccessCondition, statusCode, responseHeaders, responseBody) {
				_ = util.SendResponse(cfg.StatusOnError, "ext-auth.denied-by-condition", filterAllowedClientHeaders(cfg, responseHeaders), nil)
				return
			}

			// injected collects every header applied to the upstream request so the
			// allow decision can be replayed from cache later.
			injected := make(map[string]string)
			if httpServiceConfig.AuthorizationResponse.AllowedUpstreamHeaders != nil {
				for headK, headV := range responseHeaders {
					if httpServiceConfig.AuthorizationResponse.AllowedUpstreamHeaders.Match(headK) {
						_ = proxywasm.ReplaceHttpRequestHeader(headK, headV[0])
						injected[headK] = headV[0]
					}
				}
			}

			// mapped_upstream_headers extracts a value from the authorization response
			// (status code, header, or body JSON) and forwards it upstream under a
			// possibly different name. A value that cannot be extracted, or is empty, is
			// not injected.
			for _, m := range httpServiceConfig.AuthorizationResponse.MappedUpstreamHeaders {
				if v, ok := extract.Value(m.Source, m.Key, statusCode, responseHeaders, responseBody); ok && v != "" {
					_ = proxywasm.ReplaceHttpRequestHeader(m.ToHeader, v)
					injected[m.ToHeader] = v
				}
			}

			if cacheKey != "" {
				writeAuthCache(cfg.Cache, cacheKey, statusCode, injected)
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

	respHeaders := filterAllowedClientHeaders(config, extAuthRespHeaders)

	// Rejects client requests with StatusOnError if extAuth is unavailable or returns a 5xx status.
	// Otherwise, uses the status code returned by extAuth to reject requests.
	statusToUse := statusCode
	if statusCode >= http.StatusInternalServerError {
		statusToUse = int(config.StatusOnError)
	}
	_ = util.SendResponse(uint32(statusToUse), "ext-auth.unauthorized", respHeaders, responseBody)
}

// filterAllowedClientHeaders returns the authorization response headers filtered by
// allowed_client_headers. When allowed_client_headers is not configured, the headers
// are returned unchanged.
func filterAllowedClientHeaders(cfg config.ExtAuthConfig, extAuthRespHeaders http.Header) http.Header {
	if cfg.HttpService.AuthorizationResponse.AllowedClientHeaders == nil {
		return extAuthRespHeaders
	}
	respHeaders := http.Header{}
	for headK, headV := range extAuthRespHeaders {
		if cfg.HttpService.AuthorizationResponse.AllowedClientHeaders.Match(headK) {
			respHeaders.Set(headK, headV[0])
		}
	}
	return respHeaders
}

// evaluateSuccessCondition reports whether the authorization response satisfies every
// condition in the list. Conditions are combined with an implicit AND, so an empty list
// is trivially satisfied.
func evaluateSuccessCondition(conditions []config.Condition, statusCode int, headers http.Header, body []byte) bool {
	for _, c := range conditions {
		if !matchCondition(c, statusCode, headers, body) {
			return false
		}
	}
	return true
}

// matchCondition evaluates a single condition against the authorization response.
func matchCondition(c config.Condition, statusCode int, headers http.Header, body []byte) bool {
	value, ok := extract.Value(c.Source, c.Key, statusCode, headers, body)

	switch c.Op {
	case config.OpExists:
		return ok
	case config.OpNotExists:
		return !ok
	}

	// Value operators require the value to be present.
	if !ok {
		return false
	}

	switch c.Op {
	case config.OpEq:
		return value == c.Value[0]
	case config.OpNe:
		return value != c.Value[0]
	case config.OpIn:
		return slices.Contains(c.Value, value)
	case config.OpNotIn:
		return !slices.Contains(c.Value, value)
	case config.OpGt:
		return compareNumeric(value, c.Value[0]) > 0
	case config.OpLt:
		return compareNumeric(value, c.Value[0]) < 0
	default:
		return false
	}
}

// compareNumeric parses both operands as floats and returns -1, 0, or 1. It returns 0
// when either operand fails to parse, so gt and lt both evaluate to false for
// non-numeric input.
func compareNumeric(left, right string) int {
	l, err := strconv.ParseFloat(strings.TrimSpace(left), 64)
	if err != nil {
		return 0
	}
	r, err := strconv.ParseFloat(strings.TrimSpace(right), 64)
	if err != nil {
		return 0
	}
	switch {
	case l < r:
		return -1
	case l > r:
		return 1
	default:
		return 0
	}
}
