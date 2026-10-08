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
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/santhosh-tekuri/jsonschema"
	"github.com/tidwall/gjson"

	"github.com/higress-group/wasm-go/pkg/wrapper"
)

const (
	DEFAULT_SCHEMA                    = "defaultSchema"
	HTTP_STATUS_OK                    = uint32(200)
	HTTP_STATUS_INTERNAL_SERVER_ERROR = uint32(500)
	FROM_THIS_PLUGIN_KEY              = "fromThisPlugin"
	EXTEND_HEADER_KEY                 = "X-HIGRESS-AI-JSON-RESP"

	JSON_SCHEMA_INVALID_CODE          = 1001
	JSON_SCHEMA_COMPILE_FAILED_CODE   = 1002
	CANNOT_FIND_JSON_IN_RESPONSE_CODE = 1003
	CONTENT_IS_EMPTY_CODE             = 1004
	JSON_MISMATCH_SCHEMA_CODE         = 1005
	REACH_MAX_RETRY_COUNT_CODE        = 1006
	SERVICE_UNAVAILABLE_CODE          = 1007
	SERVICE_CONFIG_INVALID_CODE       = 1008
)

type RejectStruct struct {
	RejectCode uint32 `json:"Code"`
	RejectMsg  string `json:"Msg"`
}

func (r RejectStruct) GetBytes() []byte {
	jsonData, _ := json.Marshal(r)
	return jsonData
}

func (r RejectStruct) GetShortMsg() string {
	return "ai-json-resp." + strings.Split(r.RejectMsg, ":")[0]
}

type PluginConfig struct {
	// @Title zh-CN 服务名称
	// @Description zh-CN 用以请求服务的名称(网关或其他AI服务)
	serviceName string `required:"true" json:"serviceName" yaml:"serviceName"`
	// @Title zh-CN 服务域名
	// @Description zh-CN 用以请求服务的域名
	serviceDomain string `required:"false" json:"serviceDomain" yaml:"serviceDomain"`
	// @Title zh-CN 服务端口
	// @Description zh-CN 用以请求服务的端口
	servicePort int `required:"false" json:"servicePort" yaml:"servicePort"`
	// @Title zh-CN 服务URL
	// @Description zh-CN 用以请求服务的URL，若提供则会覆盖serviceDomain和servicePort
	serviceUrl string `required:"false" json:"serviceUrl" yaml:"serviceUrl"`
	// @Title zh-CN API Key
	// @Description zh-CN 若使用AI服务，需要填写请求服务的API Key
	apiKey string `required:"false" json: "apiKey" yaml:"apiKey"`
	// @Title zh-CN 请求端点
	// @Description zh-CN 用以请求服务的端点, 默认为"/v1/chat/completions"
	servicePath string `required:"false" json: "servicePath" yaml:"servicePath"`
	// @Title zh-CN 服务超时时间
	// @Description zh-CN 用以请求服务的超时时间
	serviceTimeout int `required:"false" json:"serviceTimeout" yaml:"serviceTimeout"`
	// @Title zh-CN 最大重试次数
	// @Description zh-CN 用以请求服务的最大重试次数
	maxRetry int `required:"false" json:"maxRetry" yaml:"maxRetry"`
	// @Title zh-CN 内容路径
	// @Description zh-CN 从AI服务返回的响应中提取json的gpath路径
	contentPath string `required:"false" json:"contentPath" yaml:"contentPath"`
	// @Title zh-CN Json Schema
	// @Description zh-CN 用以验证响应json的Json Schema, 为空则只验证返回的响应是否为合法json
	jsonSchema map[string]interface{} `required:"false" json:"jsonSchema" yaml:"jsonSchema"`
	// @Title zh-CN 是否启用swagger
	// @Description zh-CN 是否启用swagger进行Json Schema验证
	enableSwagger bool `required:"false" json:"enableSwagger" yaml:"enableSwagger"`
	// @Title zh-CN 是否启用oas3
	// @Description zh-CN 是否启用oas3进行Json Schema验证
	enableOas3 bool `required:"false" json:"enableOas3" yaml:"enableOas3"`
	// @Title zh-CN 是否启用Content-Disposition
	// @Description zh-CN 是否启用Content-Disposition, 若启用则会在响应头中添加Content-Disposition: attachment; filename="response.json"
	enableContentDisposition bool `required:"false" json:"enableContentDisposition" yaml:"enableContentDisposition"`

	serviceClient              wrapper.HttpClient
	draft                      *jsonschema.Draft
	compiler                   *jsonschema.Compiler
	compile                    *jsonschema.Schema
	rejectStruct               RejectStruct
	jsonSchemaMaxDepth         int
	enableJsonSchemaValidation bool
}

func main() {}

func init() {
	wrapper.SetCtx(
		"ai-json-resp",
		wrapper.ParseConfigBy(parseConfig),
		wrapper.ProcessRequestHeadersBy(onHttpRequestHeaders),
		wrapper.ProcessRequestBodyBy(onHttpRequestBody),
	)
}

type RequestContext struct {
	Path            string
	ReqHeaders      [][2]string
	ReqBody         []byte
	RespHeader      [][2]string
	RespBody        []byte
	HistoryMessages []chatMessage
}

func parseUrl(url string) (string, string) {
	if url == "" {
		return "", ""
	}
	url = strings.TrimPrefix(url, "http://")
	url = strings.TrimPrefix(url, "https://")
	index := strings.Index(url, "/")
	if index == -1 {
		return url, ""
	}
	return url[:index], url[index:]
}

func parseConfig(result gjson.Result, config *PluginConfig, log log.Log) error {
	config.serviceName = result.Get("serviceName").String()
	config.serviceUrl = result.Get("serviceUrl").String()
	config.serviceDomain = result.Get("serviceDomain").String()
	config.servicePath = result.Get("servicePath").String()
	config.servicePort = int(result.Get("servicePort").Int())
	if config.serviceUrl != "" {
		domain, url := parseUrl(config.serviceUrl)
		log.Debugf("serviceUrl: %s, the parsed domain: %s, the parsed url: %s", config.serviceUrl, domain, url)
		if config.serviceDomain == "" {
			config.serviceDomain = domain
		}
		if config.servicePath == "" {
			config.servicePath = url
		}
	}
	if config.servicePort == 0 {
		config.servicePort = 443
	}
	config.serviceTimeout = int(result.Get("serviceTimeout").Int())
	config.apiKey = result.Get("apiKey").String()
	config.rejectStruct = RejectStruct{HTTP_STATUS_OK, ""}
	if config.serviceTimeout == 0 {
		config.serviceTimeout = 50000
	}
	config.maxRetry = int(result.Get("maxRetry").Int())
	if config.maxRetry == 0 {
		config.maxRetry = 3
	}
	config.contentPath = result.Get("contentPath").String()
	if config.contentPath == "" {
		config.contentPath = "choices.0.message.content"
	}

	if jsonSchemaValue := result.Get("jsonSchema"); jsonSchemaValue.Exists() {
		if schemaValue, ok := jsonSchemaValue.Value().(map[string]interface{}); ok {
			config.jsonSchema = schemaValue

		} else {
			config.rejectStruct = RejectStruct{JSON_SCHEMA_INVALID_CODE, "Json Schema is not valid"}
		}
	} else {
		config.jsonSchema = nil
	}

	if config.serviceDomain == "" {
		config.rejectStruct = RejectStruct{JSON_SCHEMA_INVALID_CODE, "service domain is empty"}
	}

	config.serviceClient = wrapper.NewClusterClient(wrapper.DnsCluster{
		ServiceName: config.serviceName,
		Port:        int64(config.servicePort),
		Domain:      config.serviceDomain,
	})

	enableSwagger := result.Get("enableSwagger").Bool()
	enableOas3 := result.Get("enableOas3").Bool()

	// set draft version
	if enableSwagger {
		config.draft = jsonschema.Draft4
	}
	if enableOas3 {
		config.draft = jsonschema.Draft7
	}
	if !enableSwagger && !enableOas3 {
		config.draft = jsonschema.Draft7
	}

	// create compiler
	compiler := jsonschema.NewCompiler()
	compiler.Draft = config.draft
	config.compiler = compiler

	// set max depth of json schema
	config.jsonSchemaMaxDepth = 6

	enableContentDispositionValue := result.Get("enableContentDisposition")
	if !enableContentDispositionValue.Exists() {
		config.enableContentDisposition = true
	} else {
		config.enableContentDisposition = enableContentDispositionValue.Bool()
	}

	config.enableJsonSchemaValidation = true

	jsonSchemaBytes, err := json.Marshal(config.jsonSchema)
	if err != nil {
		config.rejectStruct = RejectStruct{JSON_SCHEMA_INVALID_CODE, "Json Schema marshal failed"}
		return err
	}

	maxDepth := GetMaxDepth(config.jsonSchema)
	log.Debugf("max depth of json schema: %d", maxDepth)
	if maxDepth > config.jsonSchemaMaxDepth {
		config.enableJsonSchemaValidation = false
		log.Infof("Json Schema depth exceeded: %d from %d , Json Schema validation will not be used.", maxDepth, config.jsonSchemaMaxDepth)
	}

	if config.enableJsonSchemaValidation {
		jsonSchemaStr := string(jsonSchemaBytes)
		config.compiler.AddResource(DEFAULT_SCHEMA, strings.NewReader(jsonSchemaStr))
		// Test if the Json Schema is valid
		compile, err := config.compiler.Compile(DEFAULT_SCHEMA)
		if err != nil {
			log.Infof("Json Schema compile failed: %v", err)
			config.rejectStruct = RejectStruct{JSON_SCHEMA_COMPILE_FAILED_CODE, "Json Schema compile failed: " + err.Error()}
			config.compile = nil
		} else {
			config.compile = compile
		}
	}

	return nil
}

func (r *RequestContext) assembleReqBody(config PluginConfig) []byte {
	var reqBodystrut chatCompletionRequest
	json.Unmarshal(r.ReqBody, &reqBodystrut)
	content := gjson.ParseBytes(r.RespBody).Get(config.contentPath).String()
	jsonSchemaBytes, _ := json.Marshal(config.jsonSchema)
	jsonSchemaStr := string(jsonSchemaBytes)

	askQuestion := "Given the Json Schema: " + jsonSchemaStr + ", please help me convert the following content to a pure json: " + content
	askQuestion += "\n Do not respond other content except the pure json!!!!"

	reqBodystrut.Messages = append(r.HistoryMessages, []chatMessage{
		{
			Role:    "user",
			Content: askQuestion,
		},
	}...)

	reqBody, _ := json.Marshal(reqBodystrut)
	return reqBody
}

func (r *RequestContext) SaveBodyToHistMsg(log log.Log, reqBody []byte, respBody []byte) {
	r.RespBody = respBody
	lastUserMessage := ""
	lastSystemMessage := ""

	var reqBodystrut chatCompletionRequest
	err := json.Unmarshal(reqBody, &reqBodystrut)
	if err != nil {
		log.Debugf("unmarshal reqBody failed: %v", err)
	} else {
		if len(reqBodystrut.Messages) != 0 {
			lastUserMessage = reqBodystrut.Messages[len(reqBodystrut.Messages)-1].Content
		}
	}

	var respBodystrut chatCompletionResponse
	err = json.Unmarshal(respBody, &respBodystrut)
	if err != nil {
		log.Debugf("unmarshal respBody failed: %v", err)
	} else {
		if len(respBodystrut.Choices) != 0 {
			lastChoice := respBodystrut.Choices[len(respBodystrut.Choices)-1]
			if lastChoice.Message != nil {
				lastSystemMessage = lastChoice.Message.Content
			}
		}
	}

	if lastUserMessage != "" {
		r.HistoryMessages = append(r.HistoryMessages, chatMessage{
			Role:    "user",
			Content: lastUserMessage,
		})
	}

	if lastSystemMessage != "" {
		r.HistoryMessages = append(r.HistoryMessages, chatMessage{
			Role:    "system",
			Content: lastSystemMessage,
		})
	}
}

func (r *RequestContext) SaveStrToHistMsg(log log.Log, errMsg string) {
	r.HistoryMessages = append(r.HistoryMessages, chatMessage{
		Role:    "system",
		Content: errMsg,
	})
}

func (c *PluginConfig) ValidateBody(body []byte) error {
	var respJsonStrct chatCompletionResponse
	err := json.Unmarshal(body, &respJsonStrct)
	if err != nil {
		c.rejectStruct = RejectStruct{SERVICE_UNAVAILABLE_CODE, "service unavailable: " + string(body)}
		return errors.New(c.rejectStruct.RejectMsg)
	}
	content := gjson.ParseBytes(body).Get(c.contentPath)
	if !content.Exists() {
		c.rejectStruct = RejectStruct{SERVICE_UNAVAILABLE_CODE, "response body does not contain the content: " + string(body)}
		return errors.New(c.rejectStruct.RejectMsg)
	}
	return nil
}

func (c *PluginConfig) ValidateJson(body []byte, log log.Log) (string, error) {
	content := gjson.ParseBytes(body).Get(c.contentPath).String()
	// first extract json from response body
	if content == "" {
		log.Infof("response body does not contain the content")
		c.rejectStruct = RejectStruct{CONTENT_IS_EMPTY_CODE, "response body does not contain the content"}
		return "", errors.New(c.rejectStruct.RejectMsg)
	}
	jsonStr, err := c.ExtractJson(content)

	if err != nil {
		log.Infof("response body does not contain the valid json: %v", err.Error())
		c.rejectStruct = RejectStruct{CANNOT_FIND_JSON_IN_RESPONSE_CODE, "response body does not contain the valid json: " + err.Error()}
		return "", errors.New(c.rejectStruct.RejectMsg)
	}

	if c.jsonSchema != nil && c.enableJsonSchemaValidation {
		compile, err := c.compiler.Compile(DEFAULT_SCHEMA)
		if err != nil {
			log.Infof("Json Schema compile failed: %v", err)
			c.rejectStruct = RejectStruct{JSON_SCHEMA_COMPILE_FAILED_CODE, "Json Schema compile failed: " + err.Error()}
			c.compile = nil
		} else {
			c.compile = compile
		}

		// validate the json
		err = c.compile.Validate(strings.NewReader(jsonStr))
		if err != nil {
			log.Infof("response body does not match the Json Schema: %v", err)
			c.rejectStruct = RejectStruct{JSON_MISMATCH_SCHEMA_CODE, "response body does not match the Json Schema: " + err.Error()}
			return "", errors.New(c.rejectStruct.RejectMsg)
		}
	}
	c.rejectStruct = RejectStruct{HTTP_STATUS_OK, ""}
	return jsonStr, nil
}

func (c *PluginConfig) ExtractJson(bodyStr string) (string, error) {
	// simply extract json from response body string
	startIndex := strings.Index(bodyStr, "{")
	endIndex := strings.LastIndex(bodyStr, "}") + 1

	// if not found
	if startIndex == -1 || endIndex == -1 || startIndex >= endIndex {
		return "", errors.New("cannot find json in the response body")
	}

	jsonStr := bodyStr[startIndex:endIndex]

	// attempt to parse the JSON
	var result map[string]interface{}
	err := json.Unmarshal([]byte(jsonStr), &result)
	if err != nil {
		return "", err
	}
	return jsonStr, nil
}

func sendResponse(ctx wrapper.HttpContext, config PluginConfig, log log.Log, body []byte) {
	log.Infof("Final send: Code %d, Message %s, Body: %s", config.rejectStruct.RejectCode, config.rejectStruct.RejectMsg, string(body))
	header := [][2]string{
		{"Content-Type", "application/json"},
	}
	if body != nil && config.enableContentDisposition {
		header = append(header, [2]string{"Content-Disposition", "attachment; filename=\"response.json\""})
	}
	if config.rejectStruct.RejectCode != HTTP_STATUS_OK {
		proxywasm.SendHttpResponseWithDetail(HTTP_STATUS_INTERNAL_SERVER_ERROR, config.rejectStruct.GetShortMsg(), nil, config.rejectStruct.GetBytes(), -1)
	} else {
		proxywasm.SendHttpResponse(HTTP_STATUS_OK, header, body, -1)
	}
}

func recursiveRefineJson(ctx wrapper.HttpContext, config PluginConfig, log log.Log, retryCount int, requestContext *RequestContext) {
	// if retry count exceeds max retry count, return the response
	if retryCount >= config.maxRetry {
		log.Debugf("retry count exceeds max retry count")
		// report more useful error by appending the last of previous error message
		config.rejectStruct = RejectStruct{REACH_MAX_RETRY_COUNT_CODE, "retry count exceeds max retry count: " + config.rejectStruct.RejectMsg}
		sendResponse(ctx, config, log, nil)
		return
	}

	// recursively refine json
	config.serviceClient.Post(requestContext.Path, requestContext.ReqHeaders, requestContext.assembleReqBody(config),
		func(statusCode int, responseHeaders http.Header, responseBody []byte) {
			err := config.ValidateBody(responseBody)
			if err != nil {
				sendResponse(ctx, config, log, nil)
				return
			}
			retryCount++
			requestContext.SaveBodyToHistMsg(log, requestContext.assembleReqBody(config), responseBody)
			log.Debugf("[retry request %d/%d] resp code: %d", retryCount, config.maxRetry, statusCode)
			validateJson, err := config.ValidateJson(responseBody, log)
			if err == nil {
				sendResponse(ctx, config, log, []byte(validateJson))
			} else {
				requestContext.SaveStrToHistMsg(log, err.Error())
				recursiveRefineJson(ctx, config, log, retryCount, requestContext)
			}
		}, uint32(config.serviceTimeout))
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config PluginConfig, log log.Log) types.Action {
	ctx.DisableReroute()
	if config.rejectStruct.RejectCode != HTTP_STATUS_OK {
		sendResponse(ctx, config, log, nil)
		return types.ActionPause
	}

	// verify if the request is from this plugin
	extendHeaderValue, err := proxywasm.GetHttpRequestHeader(EXTEND_HEADER_KEY)
	if err == nil {
		fromThisPlugin, convErr := strconv.ParseBool(extendHeaderValue)
		if convErr != nil {
			log.Debugf("failed to parse header value as bool: %v", convErr)
			ctx.SetContext(FROM_THIS_PLUGIN_KEY, false)
		}
		if fromThisPlugin {
			ctx.SetContext(FROM_THIS_PLUGIN_KEY, true)
			return types.ActionContinue
		}
	} else {
		ctx.SetContext(FROM_THIS_PLUGIN_KEY, false)
	}

	path, err := proxywasm.GetHttpRequestHeader(":path")
	if err != nil {
		log.Infof("get request path failed: %v", err)
		path = ""
	} else {
		ctx.SetContext("path", path)
	}

	headers, err := proxywasm.GetHttpRequestHeaders()
	if err != nil {
		log.Infof("get request header failed: %v", err)
	}

	apiKey, err := proxywasm.GetHttpRequestHeader("Authorization")
	if err != nil {
		log.Infof("get request header failed: %v", err)
		apiKey = ""
	}
	if apiKey != "" {
		// remove the Authorization header
		proxywasm.RemoveHttpRequestHeader("Authorization")
		// remove the Authorization header from the headers
		for i, header := range headers {
			if header[0] == "Authorization" {
				headers = append(headers[:i], headers[i+1:]...)
				break
			}
		}
	}
	if config.apiKey != "" {
		log.Debugf("add Authorization header %s", "Bearer "+config.apiKey)
		headers = append(headers, [2]string{"Authorization", "Bearer " + config.apiKey})
	}
	ctx.SetContext("headers", headers)

	return types.ActionContinue
}

func onHttpRequestBody(ctx wrapper.HttpContext, config PluginConfig, body []byte, log log.Log) types.Action {
	// if the request is from this plugin, continue the request
	fromThisPlugin, ok := ctx.GetContext(FROM_THIS_PLUGIN_KEY).(bool)
	if ok && fromThisPlugin {
		log.Debugf("detected buffer_request, sending request to AI service")
		return types.ActionContinue
	}

	var headers [][2]string
	if h, ok := ctx.GetContext("headers").([][2]string); ok {
		headers = append(h, [2]string{EXTEND_HEADER_KEY, "true"})
	} else {
		log.Debugf("cannot get headers from context, use default headers")
		headers = [][2]string{
			{"Content-Type", "application/json"},
			{EXTEND_HEADER_KEY, "true"},
		}
	}

	// if there is any error in the config, return the response directly
	if config.rejectStruct.RejectCode != HTTP_STATUS_OK {
		sendResponse(ctx, config, log, nil)
		return types.ActionContinue
	}

	var path string
	if path, ok := ctx.GetContext("path").(string); ok {
		log.Debugf("use path: %s", path)
	} else {
		log.Debugf("cannot get path from context, use default path")
		path = "/v1/chat/completions"
	}

	if config.servicePath != "" {
		log.Debugf("use base path: %s", config.servicePath)
		path = config.servicePath
	}

	requestContext := &RequestContext{
		Path:       path,
		ReqHeaders: headers,
		ReqBody:    body,
	}

	config.serviceClient.Post(requestContext.Path, requestContext.ReqHeaders, requestContext.ReqBody,
		func(statusCode int, responseHeaders http.Header, responseBody []byte) {
			err := config.ValidateBody(responseBody)
			if err != nil {
				sendResponse(ctx, config, log, nil)
				return
			}
			requestContext.SaveBodyToHistMsg(log, body, responseBody)
			log.Debugf("[first request] resp code: %d", statusCode)
			validateJson, err := config.ValidateJson(responseBody, log)
			if err == nil {
				sendResponse(ctx, config, log, []byte(validateJson))
				return
			} else {
				retryCount := 0
				requestContext.SaveStrToHistMsg(log, err.Error())
				recursiveRefineJson(ctx, config, log, retryCount, requestContext)
			}
		}, uint32(config.serviceTimeout))

	return types.ActionPause
}
