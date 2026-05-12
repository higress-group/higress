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

package filter

import (
	"github.com/tidwall/gjson"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"

	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/utils"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

const (
	defaultMaxBodyBytes uint32 = 100 * 1024 * 1024
)

type HTTPFilterF func(context wrapper.HttpContext, config any, headers [][2]string, body []byte) types.Action

type ToolCallRequestFilterF func(context wrapper.HttpContext, config any, toolName string, toolArgs gjson.Result, rawBody []byte) types.Action

type ToolCallResponseFilterF func(context wrapper.HttpContext, config any, isError bool, content gjson.Result, rawBody []byte) types.Action

type ToolListResponseFilterF func(context wrapper.HttpContext, config any, tools gjson.Result, rawBody []byte) types.Action

type JsonRpcRequestFilterF func(context wrapper.HttpContext, config any, id utils.JsonRpcID, method string, params gjson.Result, rawBody []byte) types.Action

type JsonRpcResponseFilterF func(context wrapper.HttpContext, config any, id utils.JsonRpcID, result, error gjson.Result, rawBody []byte) types.Action

type Context struct {
	filterName                    string
	httpRequestFilter             HTTPFilterF
	httpResponseFilter            HTTPFilterF
	jsonRpcRequestFilter          JsonRpcRequestFilterF
	jsonRpcResponseFilter         JsonRpcResponseFilterF
	toolCallRequestFilter         ToolCallRequestFilterF
	toolCallResponseFilter        ToolCallResponseFilterF
	toolListResponseFilter        ToolListResponseFilterF
	parseFilterConfig             ParseFilterConfigF
	parseFilterRuleOverrideConfig ParseFilterRuleOverrideConfigF
}

type CtxOption interface {
	Apply(*Context)
}

var globalContext Context

type ParseFilterConfigF func(configBytes []byte, filterConfig *any) error

type ParseFilterRuleOverrideConfigF func(configBytes []byte, filterGlobalConfig any, filterConfig *any) error

type setConfigParserOption struct {
	f ParseFilterConfigF
	g ParseFilterRuleOverrideConfigF
}

func SetConfigParser(f ParseFilterConfigF) CtxOption {
	return &setConfigParserOption{
		f: f,
	}
}

func SetConfigOverrideParser(f ParseFilterConfigF, g ParseFilterRuleOverrideConfigF) CtxOption {
	return &setConfigParserOption{
		f: f,
		g: g,
	}
}

func (o *setConfigParserOption) Apply(ctx *Context) {
	ctx.parseFilterConfig = o.f
	ctx.parseFilterRuleOverrideConfig = o.g
}

type filterNameOption struct {
	name string
}

func FilterName(name string) CtxOption {
	return &filterNameOption{name}
}

func (o *filterNameOption) Apply(ctx *Context) {
	ctx.filterName = o.name
}

type setJsonRpcRequestFilterOption struct {
	f JsonRpcRequestFilterF
}

func SetJsonRpcRequestFilter(f JsonRpcRequestFilterF) CtxOption {
	return &setJsonRpcRequestFilterOption{f}
}

func (o *setJsonRpcRequestFilterOption) Apply(ctx *Context) {
	ctx.jsonRpcRequestFilter = o.f
}

type setJsonRpcResponseFilterOption struct {
	f JsonRpcResponseFilterF
}

func SetJsonRpcResponseFilter(f JsonRpcResponseFilterF) CtxOption {
	return &setJsonRpcResponseFilterOption{f}
}

func (o *setJsonRpcResponseFilterOption) Apply(ctx *Context) {
	ctx.jsonRpcResponseFilter = o.f
}

type setFallbackHTTPRequestFilterOption struct {
	f HTTPFilterF
}

func SetFallbackHTTPRequestFilter(f HTTPFilterF) CtxOption {
	return &setFallbackHTTPRequestFilterOption{f}
}

func (o *setFallbackHTTPRequestFilterOption) Apply(ctx *Context) {
	ctx.httpRequestFilter = o.f
}

type setFallbackHTTPResponseFilterOption struct {
	f HTTPFilterF
}

func SetFallbackHTTPResponseFilter(f HTTPFilterF) CtxOption {
	return &setFallbackHTTPResponseFilterOption{f}
}

func (o *setFallbackHTTPResponseFilterOption) Apply(ctx *Context) {
	ctx.httpResponseFilter = o.f
}

type toolCallRequestFilterOption struct {
	f ToolCallRequestFilterF
}

func SetToolCallRequestFilter(f ToolCallRequestFilterF) CtxOption {
	return &toolCallRequestFilterOption{f: f}
}

func (o *toolCallRequestFilterOption) Apply(ctx *Context) {
	ctx.toolCallRequestFilter = o.f
}

type toolCallResponseFilterOption struct {
	f ToolCallResponseFilterF
}

func SetToolCallResponseFilter(f ToolCallResponseFilterF) CtxOption {
	return &toolCallResponseFilterOption{f: f}
}

func (o *toolCallResponseFilterOption) Apply(ctx *Context) {
	ctx.toolCallResponseFilter = o.f
}

type toolListResponseFilterOption struct {
	f ToolListResponseFilterF
}

func SetToolListResponseFilter(f ToolListResponseFilterF) CtxOption {
	return &toolListResponseFilterOption{f: f}
}

func (o *toolListResponseFilterOption) Apply(ctx *Context) {
	ctx.toolListResponseFilter = o.f
}

func Load(options ...CtxOption) {
	for _, opt := range options {
		opt.Apply(&globalContext)
	}
}

func Initialize() {
	if globalContext.filterName == "" {
		panic("FilterName not set")
	}
	if globalContext.parseFilterConfig == nil {
		panic("SetConfigParser not set")
	}
	var configOption wrapper.CtxOption[mcpFilterConfig]
	if globalContext.parseFilterRuleOverrideConfig == nil {
		configOption = wrapper.ParseRawConfig(parseRawConfig)
	} else {
		configOption = wrapper.ParseOverrideRawConfig(parseGlobalConfig, parseOverrideConfig)
	}
	wrapper.SetCtx(
		globalContext.filterName,
		configOption,
		wrapper.ProcessRequestHeaders(onHttpRequestHeaders),
		wrapper.ProcessResponseHeaders(onHttpResponseHeaders),
		wrapper.ProcessRequestBody(onHttpRequestBody),
		wrapper.ProcessResponseBody(onHttpResponseBody),
	)

}

type mcpFilterConfig struct {
	config                 any
	httpRequestHandler     HTTPFilterF
	httpResponseHandler    HTTPFilterF
	jsonRpcRequestHandler  utils.JsonRpcRequestHandler
	jsonRpcResponseHandler utils.JsonRpcResponseHandler
}

func installHandler(config *mcpFilterConfig) {
	config.httpRequestHandler = globalContext.httpRequestFilter
	config.httpResponseHandler = globalContext.httpResponseFilter
	bizConfig := config.config
	if globalContext.jsonRpcRequestFilter != nil || globalContext.toolCallRequestFilter != nil {
		config.jsonRpcRequestHandler = func(context wrapper.HttpContext, id utils.JsonRpcID, method string, params gjson.Result, rawBody []byte) types.Action {
			if globalContext.jsonRpcRequestFilter != nil {
				ret := globalContext.jsonRpcRequestFilter(context, bizConfig, id, method, params, rawBody)
				if ret != types.ActionContinue {
					return ret
				}
			}
			context.SetContext("JSONRPC_METHOD", method)
			if method == "tools/call" && globalContext.toolCallRequestFilter != nil {
				toolName := params.Get("name").String()
				toolArgs := params.Get("arguments")
				return globalContext.toolCallRequestFilter(context, bizConfig, toolName, toolArgs, rawBody)
			}
			return types.ActionContinue
		}
	}
	if globalContext.jsonRpcResponseFilter != nil || globalContext.toolListResponseFilter != nil || globalContext.toolCallResponseFilter != nil {
		config.jsonRpcResponseHandler = func(context wrapper.HttpContext, id utils.JsonRpcID, result, error gjson.Result, rawBody []byte) types.Action {
			if globalContext.jsonRpcResponseFilter != nil {
				ret := globalContext.jsonRpcResponseFilter(context, bizConfig, id, result, error, rawBody)
				if ret != types.ActionContinue {
					return ret
				}
			}
			method := context.GetStringContext("JSONRPC_METHOD", "")
			if method == "tools/list" && globalContext.toolListResponseFilter != nil {
				return globalContext.toolListResponseFilter(context, bizConfig, result.Get("tools"), rawBody)
			}
			if method == "tools/call" && globalContext.toolCallResponseFilter != nil {
				return globalContext.toolCallResponseFilter(context, bizConfig, result.Get("isError").Bool(), result.Get("content"), rawBody)
			}
			return types.ActionContinue
		}
	}
	log.Debugf("installHandler called, config is: %#v", config)
}

func parseRawConfig(configBytes []byte, config *mcpFilterConfig) error {
	err := globalContext.parseFilterConfig(configBytes, &config.config)
	if err != nil {
		return err
	}
	installHandler(config)
	return nil
}

func parseGlobalConfig(configBytes []byte, config *mcpFilterConfig) error {
	err := globalContext.parseFilterConfig(configBytes, &config.config)
	if err != nil {
		return err
	}
	return nil
}

func parseOverrideConfig(configBytes []byte, global mcpFilterConfig, config *mcpFilterConfig) error {
	err := globalContext.parseFilterRuleOverrideConfig(configBytes, global.config, &config.config)
	if err != nil {
		return err
	}
	installHandler(config)
	return nil
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config mcpFilterConfig) types.Action {
	log.Debugf("onHttpRequestHeaders called")
	if !ctx.HasRequestBody() || (config.httpRequestHandler == nil && config.jsonRpcRequestHandler == nil) {
		log.Debugf("no request body or no handler, skip reading body")
		ctx.DontReadRequestBody()
		return types.ActionContinue
	}
	log.Debugf("has request body and handler, read body")
	ctx.SetRequestBodyBufferLimit(defaultMaxBodyBytes)
	return types.HeaderStopIteration
}

func onHttpRequestBody(ctx wrapper.HttpContext, config mcpFilterConfig, body []byte) types.Action {
	log.Debugf("onHttpRequestBody called, body size: %d", len(body))
	if !gjson.GetBytes(body, "jsonrpc").Exists() {
		if config.httpRequestHandler != nil {
			log.Debugf("body is not jsonrpc, using httpRequestHandler")
			headers, err := proxywasm.GetHttpRequestHeaders()
			if err != nil {
				log.Errorf("get request headers failed, err:%v", err)
				return types.ActionContinue
			}
			return config.httpRequestHandler(ctx, config.config, headers, body)
		}
		log.Debugf("body is not jsonrpc, but no httpRequestHandler, skip")
		return types.ActionContinue
	}
	log.Debugf("body is jsonrpc, using HandleJsonRpcRequest")
	return utils.HandleJsonRpcRequest(ctx, body, config.jsonRpcRequestHandler)
}

func onHttpResponseHeaders(ctx wrapper.HttpContext, config mcpFilterConfig) types.Action {
	log.Debugf("onHttpResponseHeaders called")
	// IsApplicationJson checks if the content type is application/json, so we can skip reading the body if it's application/octet-stream
	if !ctx.HasResponseBody() || !wrapper.IsApplicationJson() || (config.httpResponseHandler == nil && config.jsonRpcResponseHandler == nil) {
		log.Debugf("no response body or no handler, skip reading body")
		ctx.DontReadResponseBody()
		return types.ActionContinue
	}
	log.Debugf("has response body and handler, read body")
	ctx.SetResponseBodyBufferLimit(defaultMaxBodyBytes)
	return types.HeaderStopIteration
}

func onHttpResponseBody(ctx wrapper.HttpContext, config mcpFilterConfig, body []byte) types.Action {
	log.Debugf("onHttpResponseBody called, body size: %d", len(body))
	if !gjson.GetBytes(body, "jsonrpc").Exists() {
		if config.httpResponseHandler != nil {
			log.Debugf("body is not jsonrpc, using httpResponseHandler")
			headers, err := proxywasm.GetHttpResponseHeaders()
			if err != nil {
				log.Errorf("get response headers failed, err:%v", err)
				return types.ActionContinue
			}
			return config.httpResponseHandler(ctx, config.config, headers, body)
		}
		log.Debugf("body is not jsonrpc, but no httpResponseHandler, skip")
		return types.ActionContinue
	}
	log.Debugf("body is jsonrpc, using HandleJsonRpcResponse")
	return utils.HandleJsonRpcResponse(ctx, body, config.jsonRpcResponseHandler)
}
