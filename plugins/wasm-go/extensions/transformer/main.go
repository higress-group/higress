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
	"strings"

	"regexp"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/pkg/errors"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"github.com/higress-group/wasm-go/pkg/wrapper"
)

func main() {}

func init() {
	wrapper.SetCtx(
		"transformer",
		wrapper.ParseConfigBy(parseConfig),
		wrapper.ProcessRequestHeadersBy(onHttpRequestHeaders),
		wrapper.ProcessRequestBodyBy(onHttpRequestBody),
		wrapper.ProcessResponseHeadersBy(onHttpResponseHeaders),
		wrapper.ProcessResponseBodyBy(onHttpResponseBody),
		wrapper.WithRebuildAfterRequests[TransformerConfig](1000),
	)
}

// @Name transformer
// @Category custom
// @Phase UNSPECIFIED_PHASE
// @Priority 100
// @Title zh-CN 请求/响应转换器
// @Title en-US Request/Response Transformer
// @Description zh-CN transformer 插件可以对请求/响应头、请求查询参数、请求/响应体参数进行转换，支持的转换操作类型包括删除、重命名、更新、添加、追加、映射、去重。
// @Description en-US The transformer plugin can transform request/response headers, request query parameters, and request/response body parameters. Supported transform operations include remove, rename, replace, add, append, map, and dedupe.
// @IconUrl https://img.alicdn.com/imgextra/i1/O1CN018iKKih1iVx287RltL_!!6000000004419-2-tps-42-42.png
// @Version 1.0.0
//
// @Contact.name Higress Team
// @Contact.url http://higress.io/
// @Contact.email admin@higress.io
//
// @Example
// reqRules:
//   - operate: remove
//     headers:
//   - key: X-remove
//     querys:
//   - key: k1
//     body:
//   - key: a1
//   - operate: rename
//     headers:
//   - oldKey: X-not-renamed
//     newKey: X-renamed
//   - operate: replace
//     headers:
//   - key: X-replace
//     newValue: replaced
//   - operate: add
//     headers:
//   - key: X-add-append
//     value: host-$1
//     host_pattern: ^(.*)\.com$
//   - operate: append
//     headers:
//   - key: X-add-append
//     appendValue: path-$1
//     path_pattern: ^.*?\/(\w+)[\?]{0,1}.*$
//     body:
//   - key: a1-new
//     appendValue: t1-$1-append
//     value_type: string
//     host_pattern: ^(.*)\.com$
//   - operate: map
//     headers:
//   - fromKey: X-add-append
//     toKey: X-map
//   - operate: dedupe
//     headers:
//   - key: X-dedupe-first
//     stratergy: RETAIN_FIRST
//
// @End
type TransformerConfig struct {
	// @Title 是否重新路由
	// @Description 是否在请求转换过程中对路由目标进行重新选择，默认为 true
	reroute bool `yaml:"reroute"`

	// @Title 转换规则
	// @Description 指定转换操作类型以及请求/响应头、请求查询参数、请求/响应体参数的转换规则
	reqRules  []TransformRule `yaml:"reqRules"`
	respRules []TransformRule `yaml:"respRules"`

	// this field is not exposed to the user and is used to store the request and response transformer instance
	reqTrans  Transformer `yaml:"-"`
	respTrans Transformer `yaml:"-"`
}

type TransformRule struct {
	// @Title 转换操作类型
	// @Description 指定转换操作类型，可选值为 remove, rename, replace, add, append, map, dedupe
	operate string `yaml:"operate"`

	// @Title 映射来源类型
	// @Description map操作可使用该字段进行跨类型映射，可选值为headers, query, body，若yaml中未出现该字段，则默认map操作不做跨类型映射，代码内设定为"self"，若yaml中无map操作要求，代码内设定为空字符串
	mapSource string `yaml:"mapSource"`

	// @Title 请求/响应头转换规则
	// @Description 指定请求/响应头转换规则
	headers []Param `yaml:"headers"`

	// @Title 请求查询参数转换规则
	// @Description 指定请求查询参数转换规则
	querys []Param `yaml:"querys"`

	// @Title 请求/响应体参数转换规则
	// @Description 指定请求/响应体参数转换规则，请求体转换允许 content-type 为 application/json, application/x-www-form-urlencoded, multipart/form-data；响应体转换仅允许 content-type 为 application/json
	body []Param `yaml:"body"`
}
type RemoveParam struct {
	// @Title 目标key
	// @Description
	key string `yaml:"key"`
}

type RenameParam struct {
	// @Title 目标
	// @Description
	oldKey string `yaml:"oldKey"`

	// @Title 新的key名称
	// @Description
	newKey string `yaml:"newKey"`
}

type ReplaceParam struct {
	// @Title 目标key
	// @Description
	key string `yaml:"key"`

	// @Title 新的value值
	// @Description
	newValue string `yaml:"newValue"`
}

type AddParam struct {
	// @Title 添加的key
	// @Description
	key string `yaml:"key"`

	// @Title 添加的value
	// @Description
	value string `yaml:"value"`
}

type AppendParam struct {
	// @Title 目标key
	// @Description
	key string `yaml:"key"`

	// @Title 追加的value值
	// @Description
	appendValue string `yaml:"appendValue"`
}

type MapParam struct {
	// @Title 映射来源key
	// @Description
	fromKey string `yaml:"fromKey"`

	// @Title 映射目标
	// @Description
	toKey string `yaml:"toKey"`
}

type DedupeParam struct {
	// @Title 目标key
	// @Description
	key string `yaml:"key"`

	// @Title 指定去重策略
	// @Description
	strategy string `yaml:"strategy"`
}

type Param struct {
	removeParam  RemoveParam
	renameParam  RenameParam
	replaceParam ReplaceParam
	addParam     AddParam
	appendParam  AppendParam
	mapParam     MapParam
	dedupeParam  DedupeParam
	// @Title 值类型
	// @Description 当 content-type=application/json 时，为请求/响应体参数指定值类型，可选值为 object, boolean, number, string(default)
	valueType string `yaml:"value_type"`

	// @Title 请求主机名匹配规则
	// @Description 指定主机名匹配规则，当转换操作类型为 replace, add, append 时有效
	hostPattern string `yaml:"host_pattern"`

	// @Title 请求路径匹配规则
	// @Description 指定路径匹配规则，当转换操作类型为 replace, add, append 时有效
	pathPattern string `yaml:"path_pattern"`
}

func parseConfig(json gjson.Result, config *TransformerConfig, log log.Log) (err error) {
	reroute := json.Get("reroute")
	if !reroute.Exists() {
		config.reroute = true
	} else {
		config.reroute = reroute.Bool()
	}

	reqRulesInJson := json.Get("reqRules")
	respRulesInJson := json.Get("respRules")

	if !reqRulesInJson.Exists() && !respRulesInJson.Exists() {
		return errors.New("transformer rule not exist in yaml")
	}

	if reqRulesInJson.Exists() {
		config.reqRules, err = newTransformRule(reqRulesInJson.Array())
	}
	if respRulesInJson.Exists() {
		config.respRules, err = newTransformRule(respRulesInJson.Array())
	}

	if err != nil {
		return errors.Wrapf(err, "failed to new transform rule")
	}

	if config.reqRules != nil {
		config.reqTrans, err = newRequestTransformer(config)
	}
	if config.respRules != nil {
		config.respTrans, err = newResponseTransformer(config)
	}
	if err != nil {
		return errors.Wrapf(err, "failed to new transformer")
	}

	log.Infof("transform config is: reqRules:%+v, respRules:%+v", config.reqRules, config.respRules)

	return nil
}

// TODO: 增加检查某些字段比如oldKey&newKey未同时存在时的提示信息
func constructParam(item gjson.Result, op, valueType string) Param {
	p := Param{
		valueType: valueType,
	}

	switch op {
	case "remove":
		p.removeParam.key = item.Get("key").String()
	case "rename":
		p.renameParam.oldKey = item.Get("oldKey").String()
		p.renameParam.newKey = item.Get("newKey").String()
	case "replace":
		p.replaceParam.key = item.Get("key").String()
		p.replaceParam.newValue = item.Get("newValue").String()
	case "add":
		p.addParam.key = item.Get("key").String()
		p.addParam.value = item.Get("value").String()
	case "append":
		p.appendParam.key = item.Get("key").String()
		p.appendParam.appendValue = item.Get("appendValue").String()
	case "map":
		p.mapParam.fromKey = item.Get("fromKey").String()
		p.mapParam.toKey = item.Get("toKey").String()
	case "dedupe":
		p.dedupeParam.key = item.Get("key").String()
		p.dedupeParam.strategy = item.Get("strategy").String()
	}

	if op == "replace" || op == "add" || op == "append" {
		p.hostPattern = item.Get("host_pattern").String()
		p.pathPattern = item.Get("path_pattern").String()
	}
	return p
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config TransformerConfig, log log.Log) types.Action {
	if !config.reroute {
		log.Debug("disable reroute")
		ctx.DisableReroute()
	}

	// because it may be a response transformer, so the setting of host and path have to advance
	host, path := ctx.Host(), ctx.Path()
	ctx.SetContext("host", host)
	ctx.SetContext("path", path)

	if config.reqTrans == nil {
		ctx.DontReadRequestBody()
		return types.ActionContinue
	}

	log.Debug("on http request headers ...")

	headers, err := proxywasm.GetHttpRequestHeaders()
	if err != nil {
		log.Warn("failed to get request headers")
		ctx.DontReadRequestBody()
		return types.ActionContinue
	}
	hs := convertHeaders(headers)
	if hs[":authority"] == nil {
		log.Warn(errGetRequestHost.Error())
		ctx.DontReadRequestBody()
		return types.ActionContinue
	}
	if hs[":path"] == nil {
		log.Warn(errGetRequestPath.Error())
		ctx.DontReadRequestBody()
		return types.ActionContinue
	}
	contentType := ""
	if hs["content-type"] != nil {
		contentType = hs["content-type"][0]
	}
	ctx.SetContext("content-type", contentType)

	isValidRequestContent := isValidRequestContentType(contentType)
	isBodyChange := config.reqTrans.IsBodyChange()
	needBodyMapSource := config.reqTrans.NeedBodyMapSource()

	log.Debugf("contentType:%s, isValidRequestContent:%v, isBodyChange:%v, needBodyMapSource:%v",
		contentType, isValidRequestContent, isBodyChange, needBodyMapSource)

	if isBodyChange && isValidRequestContent {
		delete(hs, "content-length")
	}

	qs, err := parseQueryByPath(path)
	if err != nil {
		log.Warnf("failed to parse query params by path: %v", err)
		ctx.DontReadRequestBody()
		return types.ActionContinue
	}

	ctx.SetContext("headers", hs)
	ctx.SetContext("querys", qs)

	if !isValidRequestContent || (!isBodyChange && !needBodyMapSource) {
		ctx.DontReadRequestBody()
	} else if needBodyMapSource {
		// we need do transform during body phase
		ctx.SetContext("need_head_trans", struct{}{})
		log.Debug("delay header's transform to body phase")
		return types.HeaderStopIteration
	}

	mapSourceData := make(map[string]MapSourceData)
	mapSourceData["headers"] = MapSourceData{
		mapSourceType: "headers",
		kvs:           hs,
	}
	mapSourceData["querys"] = MapSourceData{
		mapSourceType: "querys",
		kvs:           qs,
	}

	if config.reqTrans.IsHeaderChange() {
		if err = config.reqTrans.TransformHeaders(host, path, hs, mapSourceData); err != nil {
			log.Warnf("failed to transform request headers: %v", err)
			ctx.DontReadRequestBody()
			return types.ActionContinue
		}
	}

	if config.reqTrans.IsQueryChange() {
		if err = config.reqTrans.TransformQuerys(host, path, qs, mapSourceData); err != nil {
			log.Warnf("failed to transform request query params: %v", err)
			ctx.DontReadRequestBody()
			return types.ActionContinue
		}
		path, err = constructPath(path, qs)
		if err != nil {
			log.Warnf("failed to construct path: %v", err)
			ctx.DontReadRequestBody()
			return types.ActionContinue
		}
		hs[":path"] = []string{path}
	}

	headers = reconvertHeaders(hs)
	if err = proxywasm.ReplaceHttpRequestHeaders(headers); err != nil {
		log.Warnf("failed to replace request headers: %v", err)
		ctx.DontReadRequestBody()
		return types.ActionContinue
	}

	return types.ActionContinue
}

func onHttpRequestBody(ctx wrapper.HttpContext, config TransformerConfig, body []byte, log log.Log) types.Action {
	if config.reqTrans == nil {
		return types.ActionContinue
	}

	log.Debug("on http request body ...")

	host, path, err := getHostAndPathFromHttpCtx(ctx)
	if err != nil {
		log.Warn(err.Error())
		return types.ActionContinue
	}
	contentType, ok := ctx.GetContext("content-type").(string)
	if !ok {
		log.Warn(errGetContentType.Error())
		return types.ActionContinue
	}
	structuredBody, err := parseBody(contentType, body)
	if err != nil {
		if !errors.Is(err, errEmptyBody) {
			log.Warnf("failed to parse request body: %v", err)
		}
		log.Debug("request body is empty")
		return types.ActionContinue
	}

	mapSourceData := make(map[string]MapSourceData)
	var hs map[string][]string
	var qs map[string][]string

	hs = ctx.GetContext("headers").(map[string][]string)
	if hs == nil {
		log.Warn("failed to get request headers")
		return types.ActionContinue
	}
	if hs[":authority"] == nil {
		log.Warn(errGetRequestHost.Error())
		return types.ActionContinue
	}
	if hs[":path"] == nil {
		log.Warn(errGetRequestPath.Error())
		return types.ActionContinue
	}
	mapSourceData["headers"] = MapSourceData{
		mapSourceType: "headers",
		kvs:           hs,
	}

	qs = ctx.GetContext("querys").(map[string][]string)
	if qs == nil {
		log.Warn("failed to get request querys")
		return types.ActionContinue
	}
	mapSourceData["querys"] = MapSourceData{
		mapSourceType: "querys",
		kvs:           qs,
	}

	switch structuredBody.(type) {
	case map[string]interface{}:
		mapSourceData["body"] = MapSourceData{
			mapSourceType: "bodyJson",
			json:          structuredBody.(map[string]interface{})["body"].([]byte),
		}
	case map[string][]string:
		mapSourceData["body"] = MapSourceData{
			mapSourceType: "bodyKv",
			kvs:           structuredBody.(map[string][]string),
		}
	}

	if ctx.GetContext("need_head_trans") != nil {
		if config.reqTrans.IsHeaderChange() {
			if err = config.reqTrans.TransformHeaders(host, path, hs, mapSourceData); err != nil {
				log.Warnf("failed to transform request headers: %v", err)
				return types.ActionContinue
			}
		}

		if config.reqTrans.IsQueryChange() {
			if err = config.reqTrans.TransformQuerys(host, path, qs, mapSourceData); err != nil {
				log.Warnf("failed to transform request query params: %v", err)
				return types.ActionContinue
			}
			path, err = constructPath(path, qs)
			if err != nil {
				log.Warnf("failed to construct path: %v", err)
				return types.ActionContinue
			}
			hs[":path"] = []string{path}
		}

		headers := reconvertHeaders(hs)
		if err = proxywasm.ReplaceHttpRequestHeaders(headers); err != nil {
			log.Warnf("failed to replace request headers: %v", err)
			return types.ActionContinue
		}
	}

	if !config.reqTrans.IsBodyChange() {
		return types.ActionContinue
	}

	if err = config.reqTrans.TransformBody(host, path, structuredBody, mapSourceData); err != nil {
		log.Warnf("failed to transform request body: %v", err)
		return types.ActionContinue
	}

	body, err = constructBody(contentType, structuredBody)
	if err != nil {
		log.Warnf("failed to construct request body: %v", err)
		return types.ActionContinue
	}
	if err = proxywasm.ReplaceHttpRequestBody(body); err != nil {
		log.Warnf("failed to replace request body: %v", err)
		return types.ActionContinue
	}

	return types.ActionContinue
}

func onHttpResponseHeaders(ctx wrapper.HttpContext, config TransformerConfig, log log.Log) types.Action {
	if config.respTrans == nil {
		ctx.DontReadResponseBody()
		return types.ActionContinue
	}

	log.Debug("on http response headers ...")

	host, path, err := getHostAndPathFromHttpCtx(ctx)
	if err != nil {
		log.Warn(err.Error())
		ctx.DontReadResponseBody()
		return types.ActionContinue
	}
	headers, err := proxywasm.GetHttpResponseHeaders()
	if err != nil {
		log.Warnf("failed to get response headers: %v", err)
		ctx.DontReadResponseBody()
		return types.ActionContinue
	}
	hs := convertHeaders(headers)
	ctx.SetContext("headers", hs)
	contentType := ""
	if hs["content-type"] != nil {
		contentType = hs["content-type"][0]
	}
	ctx.SetContext("content-type", contentType)

	isValidResponseContent := isValidResponseContentType(contentType)
	isBodyChange := config.respTrans.IsBodyChange()
	needBodyMapSource := config.respTrans.NeedBodyMapSource()

	if isBodyChange && isValidResponseContent {
		delete(hs, "content-length")
	}

	if !isValidResponseContent || (!isBodyChange && !needBodyMapSource) {
		ctx.DontReadResponseBody()
	} else if needBodyMapSource {
		// we need do transform during body phase
		ctx.SetContext("need_head_trans", struct{}{})
		return types.HeaderStopIteration
	}

	mapSourceData := make(map[string]MapSourceData)
	mapSourceData["headers"] = MapSourceData{
		mapSourceType: "headers",
		kvs:           hs,
	}

	if config.respTrans.IsHeaderChange() {
		if err = config.respTrans.TransformHeaders(host, path, hs, mapSourceData); err != nil {
			log.Warnf("failed to transform response headers: %v", err)
			ctx.DontReadResponseBody()
			return types.ActionContinue
		}
	}

	headers = reconvertHeaders(hs)
	if err = proxywasm.ReplaceHttpResponseHeaders(headers); err != nil {
		log.Warnf("failed to replace response headers: %v", err)
		ctx.DontReadResponseBody()
		return types.ActionContinue
	}

	return types.ActionContinue
}

func onHttpResponseBody(ctx wrapper.HttpContext, config TransformerConfig, body []byte, log log.Log) types.Action {
	if config.respTrans == nil {
		return types.ActionContinue
	}

	log.Debug("on http response body ...")

	host, path, err := getHostAndPathFromHttpCtx(ctx)
	if err != nil {
		log.Warn(err.Error())
		return types.ActionContinue
	}
	contentType, ok := ctx.GetContext("content-type").(string)
	if !ok {
		log.Warn(errGetContentType.Error())
		return types.ActionContinue
	}
	structuredBody, err := parseBody(contentType, body)
	if err != nil {
		if !errors.Is(err, errEmptyBody) {
			log.Warnf("failed to parse response body: %v", err)
		}
		log.Debug("response body is empty")
		return types.ActionContinue
	}

	mapSourceData := make(map[string]MapSourceData)
	var hs map[string][]string

	hs = ctx.GetContext("headers").(map[string][]string)
	if hs == nil {
		log.Warn("failed to get response headers")
		return types.ActionContinue
	}
	mapSourceData["headers"] = MapSourceData{
		mapSourceType: "headers",
		kvs:           hs,
	}

	switch structuredBody.(type) {
	case map[string]interface{}:
		mapSourceData["body"] = MapSourceData{
			mapSourceType: "bodyJson",
			json:          structuredBody.(map[string]interface{})["body"].([]byte),
		}
	case map[string][]string:
		mapSourceData["body"] = MapSourceData{
			mapSourceType: "bodyKv",
			kvs:           structuredBody.(map[string][]string),
		}
	}

	if ctx.GetContext("need_head_trans") != nil {
		if config.respTrans.IsHeaderChange() {
			if err = config.respTrans.TransformHeaders(host, path, hs, mapSourceData); err != nil {
				log.Warnf("failed to transform response headers: %v", err)
				return types.ActionContinue
			}
		}

		headers := reconvertHeaders(hs)
		if err = proxywasm.ReplaceHttpResponseHeaders(headers); err != nil {
			log.Warnf("failed to replace response headers: %v", err)
			return types.ActionContinue
		}
	}

	if !config.respTrans.IsBodyChange() {
		return types.ActionContinue
	}

	if err = config.respTrans.TransformBody(host, path, structuredBody, mapSourceData); err != nil {
		log.Warnf("failed to transform response body: %v", err)
		return types.ActionContinue
	}

	body, err = constructBody(contentType, structuredBody)
	if err != nil {
		log.Warnf("failed to construct response body: %v", err)
		return types.ActionContinue
	}
	if err = proxywasm.ReplaceHttpResponseBody(body); err != nil {
		log.Warnf("failed to replace response body: %v", err)
		return types.ActionContinue
	}

	return types.ActionContinue
}

func getHostAndPathFromHttpCtx(ctx wrapper.HttpContext) (host, path string, err error) {
	host, ok := ctx.GetContext("host").(string)
	if !ok {
		return "", "", errGetRequestHost
	}
	path, ok = ctx.GetContext("path").(string)
	if !ok {
		return "", "", errGetRequestPath
	}
	return host, path, nil
}

func newTransformRule(rules []gjson.Result) (res []TransformRule, err error) {

	for _, r := range rules {
		var tRule TransformRule
		tRule.operate = strings.ToLower(r.Get("operate").String())
		if !isValidOperation(tRule.operate) {
			errors.Wrapf(err, "invalid operate type %q", tRule.operate)
			return
		}

		if tRule.operate == "map" {
			mapSourceInJson := r.Get("mapSource")
			if !mapSourceInJson.Exists() {
				tRule.mapSource = "self"
			} else {
				tRule.mapSource = mapSourceInJson.String()
				if !isValidMapSource(tRule.mapSource) {
					errors.Wrapf(err, "invalid map source %q", tRule.mapSource)
					return
				}
			}
		}

		for _, h := range r.Get("headers").Array() {
			tRule.headers = append(tRule.headers, constructParam(h, tRule.operate, ""))
		}
		for _, q := range r.Get("querys").Array() {
			tRule.querys = append(tRule.querys, constructParam(q, tRule.operate, ""))
		}
		for _, b := range r.Get("body").Array() {
			valueType := strings.ToLower(b.Get("value_type").String())
			if valueType == "" { // default
				valueType = "string"
			}
			if !isValidJsonType(valueType) {
				errors.Wrapf(err, "invalid body params type %q", valueType)
				return
			}
			tRule.body = append(tRule.body, constructParam(b, tRule.operate, valueType))
		}
		res = append(res, tRule)
	}
	return
}

type Transformer interface {
	TransformHeaders(host, path string, hs map[string][]string, mapSourceData map[string]MapSourceData) error
	TransformQuerys(host, path string, qs map[string][]string, mapSourceData map[string]MapSourceData) error
	TransformBody(host, path string, body interface{}, mapSourceData map[string]MapSourceData) error
	IsHeaderChange() bool
	IsQueryChange() bool
	IsBodyChange() bool
	NeedBodyMapSource() bool
}

var _ Transformer = (*requestTransformer)(nil)
var _ Transformer = (*responseTransformer)(nil)

type requestTransformer struct {
	headerHandler     *kvHandler
	queryHandler      *kvHandler
	bodyHandler       *requestBodyHandler
	isHeaderChange    bool
	isQueryChange     bool
	isBodyChange      bool
	needBodyMapSource bool
}

func newRequestTransformer(config *TransformerConfig) (Transformer, error) {
	headerKvtGroup, isHeaderChange, _, err := newKvtGroup(config.reqRules, "headers")
	if err != nil {
		return nil, errors.Wrap(err, "failed to new kvt group for headers")
	}
	queryKvtGroup, isQueryChange, _, err := newKvtGroup(config.reqRules, "querys")
	if err != nil {
		return nil, errors.Wrap(err, "failed to new kvt group for querys")
	}
	bodyKvtGroup, isBodyChange, _, err := newKvtGroup(config.reqRules, "body")
	if err != nil {
		return nil, errors.Wrap(err, "failed to new kvt group for body")
	}

	bodyMapSource := bodyMapSourceInRule(config.reqRules)

	return &requestTransformer{
		headerHandler: &kvHandler{headerKvtGroup},
		queryHandler:  &kvHandler{queryKvtGroup},
		bodyHandler: &requestBodyHandler{
			formDataHandler: &kvHandler{bodyKvtGroup},
			jsonHandler:     &jsonHandler{bodyKvtGroup},
		},
		isHeaderChange:    isHeaderChange,
		isQueryChange:     isQueryChange,
		isBodyChange:      isBodyChange,
		needBodyMapSource: bodyMapSource,
	}, nil
}

func (t requestTransformer) TransformHeaders(host, path string, hs map[string][]string, mapSourceData map[string]MapSourceData) error {
	return t.headerHandler.handle(host, path, hs, mapSourceData)
}

func (t requestTransformer) TransformQuerys(host, path string, qs map[string][]string, mapSourceData map[string]MapSourceData) error {
	return t.queryHandler.handle(host, path, qs, mapSourceData)
}

func (t requestTransformer) TransformBody(host, path string, body interface{}, mapSourceData map[string]MapSourceData) error {
	switch body.(type) {
	case map[string][]string:
		return t.bodyHandler.formDataHandler.handle(host, path, body.(map[string][]string), mapSourceData)

	case map[string]interface{}:
		m := body.(map[string]interface{})
		newBody, err := t.bodyHandler.handle(host, path, m["body"].([]byte), mapSourceData)
		if err != nil {
			return err
		}
		m["body"] = newBody

	default:
		return errBodyType
	}

	return nil
}

func (t requestTransformer) IsHeaderChange() bool    { return t.isHeaderChange }
func (t requestTransformer) IsQueryChange() bool     { return t.isQueryChange }
func (t requestTransformer) IsBodyChange() bool      { return t.isBodyChange }
func (t requestTransformer) NeedBodyMapSource() bool { return t.needBodyMapSource }

type responseTransformer struct {
	headerHandler     *kvHandler
	bodyHandler       *responseBodyHandler
	isHeaderChange    bool
	isBodyChange      bool
	needBodyMapSource bool
}

func newResponseTransformer(config *TransformerConfig) (Transformer, error) {

	headerKvtGroup, isHeaderChange, _, err := newKvtGroup(config.respRules, "headers")
	if err != nil {
		return nil, errors.Wrap(err, "failed to new kvt group for headers")
	}
	bodyKvtGroup, isBodyChange, _, err := newKvtGroup(config.respRules, "body")
	if err != nil {
		return nil, errors.Wrap(err, "failed to new kvt group for body")
	}
	bodyMapSource := bodyMapSourceInRule(config.respRules)

	return &responseTransformer{
		headerHandler:     &kvHandler{headerKvtGroup},
		bodyHandler:       &responseBodyHandler{&jsonHandler{bodyKvtGroup}},
		isHeaderChange:    isHeaderChange,
		isBodyChange:      isBodyChange,
		needBodyMapSource: bodyMapSource,
	}, nil
}

func (t responseTransformer) TransformHeaders(host, path string, hs map[string][]string, mapSourceData map[string]MapSourceData) error {
	return t.headerHandler.handle(host, path, hs, mapSourceData)
}

func (t responseTransformer) TransformQuerys(host, path string, qs map[string][]string, mapSourceData map[string]MapSourceData) error {
	// the response does not need to transform the query params, always returns nil
	return nil
}

func (t responseTransformer) TransformBody(host, path string, body interface{}, mapSourceData map[string]MapSourceData) error {
	switch body.(type) {
	case map[string]interface{}:
		m := body.(map[string]interface{})
		newBody, err := t.bodyHandler.handle(host, path, m["body"].([]byte), mapSourceData)
		if err != nil {
			return err
		}
		m["body"] = newBody

	default:
		return errBodyType
	}

	return nil
}

func (t responseTransformer) IsHeaderChange() bool    { return t.isHeaderChange }
func (t responseTransformer) IsQueryChange() bool     { return false } // the response does not need to transform the query params, always returns false
func (t responseTransformer) IsBodyChange() bool      { return t.isBodyChange }
func (t responseTransformer) NeedBodyMapSource() bool { return t.needBodyMapSource }

type requestBodyHandler struct {
	formDataHandler *kvHandler
	*jsonHandler
}

type responseBodyHandler struct {
	*jsonHandler
}

type kvHandler struct {
	kvtOps []kvtOperation
}

type jsonHandler struct {
	kvtOps []kvtOperation
}

func (h kvHandler) handle(host, path string, kvs map[string][]string, mapSourceData map[string]MapSourceData) error {
	// arbitrary order. for example: remove → rename → replace → add → append → map → dedupe

	for _, kvtOp := range h.kvtOps {
		switch kvtOp.kvtOpType {
		case RemoveK:
			// remove
			for _, remove := range kvtOp.removeKvtGroup {
				delete(kvs, remove.key)
			}
		case RenameK:
			// rename: 若指定 oldKey 不存在则无操作；否则将 oldKey 的值追加给 newKey，并删除 oldKey:value
			for _, rename := range kvtOp.renameKvtGroup {
				oldKey, newKey := rename.oldKey, rename.newKey
				if ovs, ok := kvs[oldKey]; ok {
					kvs[newKey] = append(kvs[newKey], ovs...)
					delete(kvs, oldKey)
				}
			}
		case ReplaceK:
			// replace: 若指定 key 不存在，相当于添加操作；否则替换 value 为 newValue
			for _, replace := range kvtOp.replaceKvtGroup {
				key, newValue := replace.key, replace.newValue
				if replace.reg != nil {
					newValue = replace.reg.matchAndReplace(newValue, host, path)
				}
				kvs[key] = []string{newValue}
			}
		case AddK:
			// add: 若指定 key 存在则无操作；否则添加 key:value
			for _, add := range kvtOp.addKvtGroup {
				key, value := add.key, add.value
				if _, ok := kvs[key]; ok {
					continue
				}
				if add.reg != nil {
					value = add.reg.matchAndReplace(value, host, path)
				}
				kvs[key] = []string{value}
			}

		case AppendK:
			// append: 若指定 key 存在，则追加同名 kv；否则相当于添加操作
			for _, append_ := range kvtOp.appendKvtGroup {
				key, appendValue := append_.key, append_.appendValue
				if append_.reg != nil {
					appendValue = append_.reg.matchAndReplace(appendValue, host, path)
				}
				kvs[key] = append(kvs[key], appendValue)
			}
		case MapK:
			// map: 若指定 fromKey 不存在则无操作；否则将 fromKey 的值映射给 toKey 的值
			for _, map_ := range kvtOp.mapKvtGroup {
				fromKey, toKey := map_.fromKey, map_.toKey
				if kvtOp.mapSource == "headers" {
					fromKey = strings.ToLower(fromKey)
				}
				source, exist := mapSourceData[kvtOp.mapSource]
				if !exist {
					proxywasm.LogWarnf("map key failed, source:%s not exists", kvtOp.mapSource)
					continue
				}
				proxywasm.LogDebugf("search key:%s in source:%s", fromKey, kvtOp.mapSource)
				if fromValue, ok := source.search(fromKey); ok {
					switch source.mapSourceType {
					case "headers", "querys", "bodyKv":
						kvs[toKey] = fromValue.([]string)
						proxywasm.LogDebugf("map key:%s to key:%s success, value is: %v", fromKey, toKey, fromValue)

					case "bodyJson":
						if valueJson, ok := fromValue.(gjson.Result); ok {
							valueStr := valueJson.String()
							if valueStr != "" {
								kvs[toKey] = []string{valueStr}
								proxywasm.LogDebugf("map key:%s to key:%s success, values is:%s", fromKey, toKey, valueStr)
							}
						}
					}
				}
			}

		case DedupeK:
			// dedupe: 根据 strategy 去重：RETAIN_UNIQUE 保留所有唯一值，RETAIN_LAST 保留最后一个值，RETAIN_FIRST 保留第一个值 (default)
			for _, dedupe := range kvtOp.dedupeKvtGroup {
				key, strategy := dedupe.key, dedupe.strategy
				switch strings.ToUpper(strategy) {
				case "RETAIN_UNIQUE":
					uniSet, uniques := make(map[string]struct{}), make([]string, 0)
					for _, v := range kvs[key] {
						if _, ok := uniSet[v]; !ok {
							uniSet[v] = struct{}{}
							uniques = append(uniques, v)
						}
					}
					kvs[key] = uniques

				case "RETAIN_LAST":
					if vs, ok := kvs[key]; ok && len(vs) >= 1 {
						kvs[key] = vs[len(vs)-1:]
					}
				case "SPLIT_AND_RETAIN_FIRST":
					if vs, ok := kvs[key]; ok && len(vs) >= 1 {
						kvs[key] = strings.Split(vs[0], ",")[:1]
					}
				case "SPLIT_AND_RETAIN_LAST":
					if vs, ok := kvs[key]; ok && len(vs) >= 1 {
						split := strings.Split(vs[0], ",")
						kvs[key] = split[len(split)-1:]
					}
				case "RETAIN_FIRST":
					fallthrough
				default:
					if vs, ok := kvs[key]; ok && len(vs) >= 1 {
						kvs[key] = vs[:1]
					}
				}
			}

		}
	}

	return nil
}

// only for body
func (h jsonHandler) handle(host, path string, oriData []byte, mapSourceData map[string]MapSourceData) (data []byte, err error) {
	// arbitrary order. for example: remove → rename → replace → add → append → map → dedupe
	if !gjson.ValidBytes(oriData) {
		return nil, errors.New("invalid json body")
	}
	data = oriData

	for _, kvtOp := range h.kvtOps {
		switch kvtOp.kvtOpType {
		case RemoveK:
			// remove
			for _, remove := range kvtOp.removeKvtGroup {
				if data, err = sjson.DeleteBytes(data, remove.key); err != nil {
					return nil, errors.Wrap(err, errRemove.Error())
				}
			}
		case RenameK:
			// rename: 若指定 oldKey 不存在则无操作；否则将 oldKey 的值追加给 newKey，并删除 oldKey:value
			for _, rename := range kvtOp.renameKvtGroup {
				oldKey, newKey := rename.oldKey, rename.newKey
				value := gjson.GetBytes(data, oldKey)
				if !value.Exists() {
					continue
				}
				if data, err = sjson.SetBytes(data, newKey, value.Value()); err != nil {
					return nil, errors.Wrap(err, errRename.Error())
				}
				if data, err = sjson.DeleteBytes(data, oldKey); err != nil {
					return nil, errors.Wrap(err, errRename.Error())
				}
			}
		case ReplaceK:
			// replace: 若指定 key 不存在，则相当于添加操作；否则替换 value 为 newValue
			for _, replace := range kvtOp.replaceKvtGroup {
				key, newValue, valueType := replace.key, replace.newValue, replace.typ
				if valueType == "string" && replace.reg != nil {
					newValue = replace.reg.matchAndReplace(newValue, host, path)
				}
				convertedNewValue, err := convertByJsonType(valueType, newValue)
				if err != nil {
					return nil, errors.Wrap(err, errReplace.Error())
				}
				if data, err = sjson.SetBytes(data, key, convertedNewValue); err != nil {
					return nil, errors.Wrap(err, errReplace.Error())
				}
			}
		case AddK:
			// add: 若指定 key 存在则无操作；否则添加 key:value
			for _, add := range kvtOp.addKvtGroup {
				key, value, valueType := add.key, add.value, add.typ
				if gjson.GetBytes(data, key).Exists() {
					continue
				}
				if valueType == "string" && add.reg != nil {
					value = add.reg.matchAndReplace(value, host, path)
				}
				convertedValue, err := convertByJsonType(valueType, value)
				if err != nil {
					return nil, errors.Wrap(err, errAdd.Error())
				}
				if data, err = sjson.SetBytes(data, key, convertedValue); err != nil {
					return nil, errors.Wrap(err, errAdd.Error())
				}
			}
		case AppendK:
			// append: 若指定 key 存在，则追加同名 kv；否则相当于添加操作
			// 当原本的 value 为数组时，追加；当原本的 value 不为数组时，将原本的 value 和 appendValue 组成数组
			for _, append_ := range kvtOp.appendKvtGroup {
				key, appendValue, valueType := append_.key, append_.appendValue, append_.typ
				if valueType == "string" && append_.reg != nil {
					appendValue = append_.reg.matchAndReplace(appendValue, host, path)
				}
				convertedAppendValue, err := convertByJsonType(valueType, appendValue)
				if err != nil {
					return nil, errors.Wrapf(err, errAppend.Error())
				}
				oldValue := gjson.GetBytes(data, key)
				if !oldValue.Exists() {
					if data, err = sjson.SetBytes(data, key, convertedAppendValue); err != nil { // key: appendValue
						return nil, errors.Wrap(err, errAppend.Error())
					}
					continue
				}

				// oldValue exists
				if oldValue.IsArray() {
					if len(oldValue.Array()) == 0 {
						if data, err = sjson.SetBytes(data, key, []interface{}{convertedAppendValue}); err != nil { // key: [appendValue]
							return nil, errors.Wrap(err, errAppend.Error())
						}
						continue
					}

					// len(oldValue.Array()) != 0
					oldValues := make([]interface{}, 0, len(oldValue.Array())+1)
					for _, val := range oldValue.Array() {
						oldValues = append(oldValues, val.Value())
					}
					if data, err = sjson.SetBytes(data, key, append(oldValues, convertedAppendValue)); err != nil { // key: [oldValue..., appendValue]
						return nil, errors.Wrap(err, errAppend.Error())
					}
					continue
				}

				// oldValue is not array
				if data, err = sjson.SetBytes(data, key, []interface{}{oldValue.Value(), convertedAppendValue}); err != nil { // key: [oldValue, appendValue]
					return nil, errors.Wrap(err, errAppend.Error())
				}
			}
		case MapK:
			// map: 若指定 fromKey 不存在则无操作；否则将 fromKey 的值映射给 toKey 的值
			for _, map_ := range kvtOp.mapKvtGroup {
				fromKey, toKey := map_.fromKey, map_.toKey
				if kvtOp.mapSource == "headers" {
					fromKey = strings.ToLower(fromKey)
				}
				source, exist := mapSourceData[kvtOp.mapSource]
				if !exist {
					proxywasm.LogWarnf("map key failed, source:%s not exists", kvtOp.mapSource)
					continue
				}

				proxywasm.LogDebugf("search key:%s in source:%s", fromKey, kvtOp.mapSource)
				if fromValue, ok := source.search(fromKey); ok {
					switch source.mapSourceType {
					case "headers", "querys", "bodyKv":
						if data, err = sjson.SetBytes(data, toKey, fromValue); err != nil {
							return nil, errors.Wrap(err, errMap.Error())
						}
						proxywasm.LogDebugf("map key:%s to key:%s success, value is: %v", fromKey, toKey, fromValue)
					case "bodyJson":
						if valueJson, ok := fromValue.(gjson.Result); ok {
							if data, err = sjson.SetBytes(data, toKey, valueJson.Value()); err != nil {
								return nil, errors.Wrap(err, errMap.Error())
							}
							proxywasm.LogDebugf("map key:%s to key:%s success, value is: %v", fromKey, toKey, fromValue)
						}
					}
				}
			}
		case DedupeK:
			// dedupe: 根据 strategy 去重：RETAIN_UNIQUE 保留所有唯一值，RETAIN_LAST 保留最后一个值，RETAIN_FIRST 保留第一个值 (default)
			for _, dedupe := range kvtOp.dedupeKvtGroup {
				key, strategy := dedupe.key, dedupe.strategy
				value := gjson.GetBytes(data, key)
				if !value.Exists() || !value.IsArray() {
					continue
				}

				// value is array
				values := value.Array()
				if len(values) == 0 {
					continue
				}

				var dedupedVal interface{}
				switch strings.ToUpper(strategy) {
				case "RETAIN_UNIQUE":
					uniSet, uniques := make(map[string]struct{}), make([]interface{}, 0)
					for _, v := range values {
						vstr := v.String()
						if _, ok := uniSet[vstr]; !ok {
							uniSet[vstr] = struct{}{}
							uniques = append(uniques, v.Value())
						}
					}
					if len(uniques) == 1 {
						dedupedVal = uniques[0] // key: uniques[0]
					} else if len(uniques) > 1 {
						dedupedVal = uniques // key: [uniques...]
					}

				case "RETAIN_LAST":
					dedupedVal = values[len(values)-1].Value() // key: last
				case "SPLIT_AND_RETAIN_FIRST":
					if len(values) > 0 {
						split := strings.Split(values[0].String(), ",")
						if len(split) > 0 {
							dedupedVal = split[0]
						}
					}
				case "SPLIT_AND_RETAIN_LAST":
					if len(values) > 0 {
						split := strings.Split(values[0].String(), ",")
						if len(split) > 0 {
							dedupedVal = split[len(split)-1]
						}
					}
				case "RETAIN_FIRST":
					fallthrough
				default:
					dedupedVal = values[0].Value() // key: first
				}

				if dedupedVal == nil {
					continue
				}
				if data, err = sjson.SetBytes(data, key, dedupedVal); err != nil {
					return nil, errors.Wrap(err, errDedupe.Error())
				}
			}
		}
	}

	return data, nil
}

type removeKvt struct {
	key string
}
type renameKvt struct {
	oldKey string
	newKey string
	typ    string
}
type replaceKvt struct {
	key      string
	newValue string
	typ      string
	*reg
}
type addKvt struct {
	key   string
	value string
	typ   string
	*reg
}
type appendKvt struct {
	key         string
	appendValue string
	typ         string
	*reg
}

type mapKvt struct {
	fromKey string
	toKey   string
}
type dedupeKvt struct {
	key      string
	strategy string
}
type KvtOpType int

const (
	RemoveK KvtOpType = iota
	RenameK
	ReplaceK
	AddK
	AppendK
	MapK
	DedupeK
)

type kvtOperation struct {
	kvtOpType       KvtOpType
	removeKvtGroup  []removeKvt
	renameKvtGroup  []renameKvt
	replaceKvtGroup []replaceKvt
	addKvtGroup     []addKvt
	appendKvtGroup  []appendKvt
	mapKvtGroup     []mapKvt
	dedupeKvtGroup  []dedupeKvt
	mapSource       string
}

func newKvtGroup(rules []TransformRule, typ string) (g []kvtOperation, isChange bool, withMapKvt bool, err error) {
	g = []kvtOperation{}
	for _, r := range rules {
		var prams []Param
		switch typ {
		case "headers":
			prams = r.headers
		case "querys":
			prams = r.querys
		case "body":
			prams = r.body
		}

		var kvtOp kvtOperation
		switch r.operate {
		case "remove":
			kvtOp.kvtOpType = RemoveK
		case "rename":
			kvtOp.kvtOpType = RenameK
		case "map":
			kvtOp.kvtOpType = MapK
		case "replace":
			kvtOp.kvtOpType = ReplaceK
		case "dedupe":
			kvtOp.kvtOpType = DedupeK
		case "add":
			kvtOp.kvtOpType = AddK
		case "append":
			kvtOp.kvtOpType = AppendK
		default:
			return nil, false, false, errors.Wrap(err, "invalid operation type")
		}
		for _, p := range prams {
			switch r.operate {
			case "remove":
				key := p.removeParam.key
				if typ == "headers" {
					key = strings.ToLower(key)
				}
				kvtOp.removeKvtGroup = append(kvtOp.removeKvtGroup, removeKvt{key})
			case "rename":
				if typ == "headers" {
					p.renameParam.oldKey = strings.ToLower(p.renameParam.oldKey)
					p.renameParam.newKey = strings.ToLower(p.renameParam.newKey)
				}
				kvtOp.renameKvtGroup = append(kvtOp.renameKvtGroup, renameKvt{p.renameParam.oldKey, p.renameParam.newKey, p.valueType})
			case "map":
				if typ == "headers" {
					p.mapParam.toKey = strings.ToLower(p.mapParam.toKey)
				}
				kvtOp.mapSource = r.mapSource
				if kvtOp.mapSource == "self" {
					kvtOp.mapSource = typ
					r.mapSource = typ
				}
				if kvtOp.mapSource == "headers" {
					p.mapParam.fromKey = strings.ToLower(p.mapParam.fromKey)
				}

				kvtOp.mapKvtGroup = append(kvtOp.mapKvtGroup, mapKvt{p.mapParam.fromKey, p.mapParam.toKey})
			case "dedupe":
				if typ == "headers" {
					p.dedupeParam.key = strings.ToLower(p.dedupeParam.key)
				}
				kvtOp.dedupeKvtGroup = append(kvtOp.dedupeKvtGroup, dedupeKvt{p.dedupeParam.key, p.dedupeParam.strategy})
			case "replace":
				if typ == "headers" {
					p.replaceParam.key = strings.ToLower(p.replaceParam.key)
				}
				var rg *reg
				if p.hostPattern != "" || p.pathPattern != "" {
					rg, err = newReg(p.hostPattern, p.pathPattern)
					if err != nil {
						return nil, false, false, errors.Wrap(err, "failed to new reg")
					}
				}
				kvtOp.replaceKvtGroup = append(kvtOp.replaceKvtGroup, replaceKvt{p.replaceParam.key, p.replaceParam.newValue, p.valueType, rg})
			case "add":
				if typ == "headers" {
					p.addParam.key = strings.ToLower(p.addParam.key)
				}
				var rg *reg
				if p.hostPattern != "" || p.pathPattern != "" {
					rg, err = newReg(p.hostPattern, p.pathPattern)
					if err != nil {
						return nil, false, false, errors.Wrap(err, "failed to new reg")
					}
				}
				kvtOp.addKvtGroup = append(kvtOp.addKvtGroup, addKvt{p.addParam.key, p.addParam.value, p.valueType, rg})
			case "append":
				if typ == "headers" {
					p.appendParam.key = strings.ToLower(p.appendParam.key)
				}
				var rg *reg
				if p.hostPattern != "" || p.pathPattern != "" {
					rg, err = newReg(p.hostPattern, p.pathPattern)
					if err != nil {
						return nil, false, false, errors.Wrap(err, "failed to new reg")
					}
				}
				kvtOp.appendKvtGroup = append(kvtOp.appendKvtGroup, appendKvt{p.appendParam.key, p.appendParam.appendValue, p.valueType, rg})
			}
		}
		isChange = isChange || len(kvtOp.removeKvtGroup) != 0 ||
			len(kvtOp.renameKvtGroup) != 0 || len(kvtOp.replaceKvtGroup) != 0 ||
			len(kvtOp.addKvtGroup) != 0 || len(kvtOp.appendKvtGroup) != 0 ||
			len(kvtOp.mapKvtGroup) != 0 || len(kvtOp.dedupeKvtGroup) != 0
		withMapKvt = withMapKvt || len(kvtOp.mapKvtGroup) != 0
		g = append(g, kvtOp)
	}

	return g, isChange, withMapKvt, nil
}

type MapSourceData struct {
	mapSourceType string
	kvs           map[string][]string // headers or querys or body in kvs
	json          []byte              // body in json
}

func (msdata MapSourceData) search(fromKey string) (interface{}, bool) {
	switch msdata.mapSourceType {
	case "headers", "querys", "bodyKv":
		fromValue, ok := msdata.kvs[fromKey]
		return fromValue, ok
	case "bodyJson":
		fromValue := gjson.GetBytes(msdata.json, fromKey)
		if !fromValue.Exists() {
			return nil, false
		}
		return fromValue, true
	default:
		return "", false
	}
}

func bodyMapSourceInRule(rules []TransformRule) bool {
	for _, r := range rules {
		if r.operate == "map" && r.mapSource == "body" {
			return true
		}
	}
	return false
}

type kvtReg struct {
	kvt
	*reg
}

type kvt struct {
	key   string
	value string
	typ   string
}

type reg struct {
	hostReg *regexp.Regexp
	pathReg *regexp.Regexp
}

// you can only choose one between host and path
func newReg(hostPatten, pathPatten string) (r *reg, err error) {
	r = &reg{}
	if hostPatten != "" {
		r.hostReg, err = regexp.Compile(hostPatten)
		return
	}
	if pathPatten != "" {
		r.pathReg, err = regexp.Compile(pathPatten)
		return
	}
	return
}

func (r reg) matchAndReplace(value, host, path string) string {
	if r.hostReg != nil && r.hostReg.MatchString(host) {
		return r.hostReg.ReplaceAllString(host, value)
	}
	if r.pathReg != nil && r.pathReg.MatchString(path) {
		return r.pathReg.ReplaceAllString(path, value)
	}
	return value
}
