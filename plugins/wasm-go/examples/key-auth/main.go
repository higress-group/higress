// Copyright (c) 2023 Alibaba Group Holding Ltd.
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
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

var (
	ruleSet         bool            // 插件是否至少在一个 domain 或 route 上生效
	protectionSpace = "MSE Gateway" // 认证失败时，返回响应头 WWW-Authenticate: Key realm=MSE Gateway
)

func main() {}

func init() {
	wrapper.SetCtx(
		"key-auth", // middleware name
		wrapper.ParseOverrideConfigBy(parseGlobalConfig, parseOverrideRuleConfig),
		wrapper.ProcessRequestHeadersBy(onHttpRequestHeaders),
	)
}

type Consumer struct {
	// @Title 名称
	// @Title en-US Name
	// @Description 该调用方的名称。
	// @Description en-US The name of the consumer.
	Name string `yaml:"name"`

	// @Title 访问凭证
	// @Title en-US Credential
	// @Description 该调用方的访问凭证。
	// @Description en-US The credential of the consumer.
	// @Scope GLOBAL
	Credential string `yaml:"credential"`

	// @Title 访问凭证列表
	// @Title en-US Credentials
	// @Description 该调用方的访问凭证列表，不能与 credential 同时配置。
	// @Description en-US The credentials of the consumer. Cannot be configured together with credential.
	// @Scope GLOBAL
	Credentials []string `yaml:"credentials"`
}

// @Name key-auth
// @Category auth
// @Phase AUTHN
// @Priority 321
// @Title zh-CN Key Auth
// @Description zh-CN 本插件实现了实现了基于 API Key 进行认证鉴权的功能.
// @Description en-US This plugin implements an authentication function based on API Key Auth standard.
// @IconUrl https://img.alicdn.com/imgextra/i4/O1CN01BPFGlT1pGZ2VDLgaH_!!6000000005333-2-tps-42-42.png
// @Version 1.0.0
//
// @Contact.name Higress Team
// @Contact.url http://higress.io/
// @Contact.email admin@higress.io
//
// @Example
// global_auth: false
// consumers:
//   - name: consumer1
//     credential: token1
//   - name: consumer2
//     credential: token2
//
// keys:
//   - x-api-key
//   - token
//
// in_query: true
// @End
type KeyAuthConfig struct {
	// @Title 是否开启全局认证
	// @Title en-US Enable Global Auth
	// @Description 若不开启全局认证，则全局配置只提供凭证信息。只有在域名或路由上进行了配置才会启用认证。
	// @Description en-US If set to false, only consumer info will be accepted from the global config. Auth feature shall only be enabled if the corresponding domain or route is configured.
	// @Scope GLOBAL
	globalAuth *bool `yaml:"global_auth,omitempty"` //是否开启全局认证. 若不开启全局认证，则全局配置只提供凭证信息。只有在域名或路由上进行了配置才会启用认证。

	// @Title API Key 的来源字段名称列表
	// @Title en-US The name of the source field of the API Key
	// @Description API Key 的来源字段名称，可以是 URL 参数或者 HTTP 请求头名称.
	// @Description en-US The name of the source field of the API Key, which can be a URL parameter or an HTTP request header name.
	// @Scope GLOBAL
	Keys []string `yaml:"keys"` // key auth names

	// @Title key是否来源于URL参数
	// @Title en-US the API Key from the URL parameters.
	// @Description 如果配置 true 时，网关会尝试从 URL 参数中解析 API Key
	// @Description en-US When configured true, the gateway will try to parse the API Key from the URL parameters.
	// @Scope GLOBAL
	InQuery bool `yaml:"in_query,omitempty"`

	// @Title key是否来源于Header
	// @Title en-US the API Key from the HTTP request header name.
	// @Description 配置 true 时，网关会尝试从 URL header头中解析 API Key
	// @Description en-US When configured true, the gateway will try to parse the API Key from the HTTP request header name.
	// @Scope GLOBAL
	InHeader bool `yaml:"in_header,omitempty"`

	// @Title 调用方列表
	// @Title en-US Consumer List
	// @Description 服务调用方列表，用于对请求进行认证。
	// @Description en-US List of service consumers which will be used in request authentication.
	// @Scope GLOBAL
	consumers []Consumer `yaml:"consumers"`

	// @Title 授权访问的调用方列表
	// @Title en-US Allowed Consumers
	// @Description 对于匹配上述条件的请求，允许访问的调用方列表。
	// @Description en-US Consumers to be allowed for matched requests.
	allow []string `yaml:"allow"`

	credential2Name map[string]string `yaml:"-"`
}

func parseGlobalConfig(json gjson.Result, global *KeyAuthConfig, log log.Log) error {
	log.Debug("global config")

	// init
	ruleSet = false
	global.credential2Name = make(map[string]string)

	// global_auth
	globalAuth := json.Get("global_auth")
	if globalAuth.Exists() {
		ga := globalAuth.Bool()
		global.globalAuth = &ga
	}

	// keys
	names := json.Get("keys")
	if !names.Exists() {
		return errors.New("keys is required")
	}
	if len(names.Array()) == 0 {
		return errors.New("keys cannot be empty")
	}

	for _, name := range names.Array() {
		global.Keys = append(global.Keys, name.String())
	}

	// in_query and in_header
	in_query := json.Get("in_query")
	in_header := json.Get("in_header")
	if !in_query.Exists() && !in_header.Exists() {
		return errors.New("must one of in_query/in_header required")
	}

	if in_query.Exists() {
		global.InQuery = in_query.Bool()
	}
	if in_header.Exists() {
		global.InHeader = in_header.Bool()
	}

	// consumers
	consumers := json.Get("consumers")
	if !consumers.Exists() {
		return errors.New("consumers is required")
	}
	if len(consumers.Array()) == 0 {
		return errors.New("consumers cannot be empty")
	}

	for _, item := range consumers.Array() {
		name := item.Get("name")
		if !name.Exists() || name.String() == "" {
			return errors.New("consumer name is required")
		}
		credential := item.Get("credential")
		credentials := item.Get("credentials")
		if credential.Exists() && credentials.Exists() {
			return errors.New("'credential' and 'credentials' can't appear at the same time")
		}
		if !credential.Exists() && !credentials.Exists() {
			return errors.New("consumer credential is required")
		}

		consumerCredentials := make([]string, 0, 1)
		if credential.Exists() {
			if credential.String() == "" {
				return errors.New("consumer credential is required")
			}
			consumerCredentials = append(consumerCredentials, credential.String())
		} else {
			if !credentials.IsArray() || len(credentials.Array()) == 0 {
				return errors.New("consumer credentials cannot be empty")
			}
			for _, credential := range credentials.Array() {
				if credential.String() == "" {
					return errors.New("consumer credential is required")
				}
				consumerCredentials = append(consumerCredentials, credential.String())
			}
		}

		for _, credential := range consumerCredentials {
			if _, ok := global.credential2Name[credential]; ok {
				return errors.New("duplicate consumer credential: " + credential)
			}
		}

		consumer := Consumer{
			Name: name.String(),
		}
		if credential.Exists() {
			consumer.Credential = credential.String()
		} else {
			consumer.Credentials = consumerCredentials
		}

		global.consumers = append(global.consumers, consumer)
		for _, credential := range consumerCredentials {
			global.credential2Name[credential] = name.String()
		}
	}
	return nil
}

func parseOverrideRuleConfig(json gjson.Result, global KeyAuthConfig, config *KeyAuthConfig, log log.Log) error {
	log.Debug("domain/route config")

	*config = global

	allow := json.Get("allow")
	if !allow.Exists() {
		return errors.New("allow is required")
	}
	if len(allow.Array()) == 0 {
		return errors.New("allow cannot be empty")
	}

	for _, item := range allow.Array() {
		config.allow = append(config.allow, item.String())
	}
	ruleSet = true

	return nil
}

// key-auth 插件认证逻辑：
// - global_auth == true 开启全局生效：
//   - 若当前 domain/route 未配置 allow 列表，即未配置该插件：则在所有 consumers 中查找，如果找到则认证通过，否则认证失败 (1*)
//   - 若当前 domain/route 配置了该插件：则在 allow 列表中查找，如果找到则认证通过，否则认证失败
//
// - global_auth == false 非全局生效：(2*)
//   - 若当前 domain/route 未配置该插件：则直接放行
//   - 若当前 domain/route 配置了该插件：则在 allow 列表中查找，如果找到则认证通过，否则认证失败
//
// - global_auth 未设置：
//   - 若没有一个 domain/route 配置该插件：则遵循 (1*)
//   - 若有至少一个 domain/route 配置该插件：则遵循 (2*)
func onHttpRequestHeaders(ctx wrapper.HttpContext, config KeyAuthConfig, log log.Log) types.Action {
	var (
		noAllow            = len(config.allow) == 0 // 未配置 allow 列表，表示插件在该 domain/route 未生效
		globalAuthNoSet    = config.globalAuth == nil
		globalAuthSetTrue  = !globalAuthNoSet && *config.globalAuth
		globalAuthSetFalse = !globalAuthNoSet && !*config.globalAuth
	)
	// 不需要认证而直接放行的情况：
	// - global_auth == false 且 当前 domain/route 未配置该插件
	// - global_auth 未设置 且 有至少一个 domain/route 配置该插件 且 当前 domain/route 未配置该插件
	if globalAuthSetFalse || (globalAuthNoSet && ruleSet) {
		if noAllow {
			log.Info("authorization is not required")
			return types.ActionContinue
		}
	}

	// 以下需要认证：
	// - 从 header 中获取 tokens 信息
	// - 从 query 中获取 tokens 信息
	var tokens []string
	if config.InHeader {
		// 匹配keys中的 keyname
		for _, key := range config.Keys {
			value, err := proxywasm.GetHttpRequestHeader(key)
			if err == nil && value != "" {
				tokens = append(tokens, value)
			}
		}
	} else if config.InQuery {
		requestUrl, _ := proxywasm.GetHttpRequestHeader(":path")
		url, _ := url.Parse(requestUrl)
		queryValues := url.Query()
		for _, key := range config.Keys {
			values, ok := queryValues[key]
			if ok && len(values) > 0 {
				tokens = append(tokens, values...)
			}
		}
	}

	// header/query
	if len(tokens) > 1 {
		return deniedMultiKeyAuthData()
	} else if len(tokens) <= 0 {
		return deniedNoKeyAuthData()
	}

	// 验证token
	name, ok := config.credential2Name[tokens[0]]
	if !ok {
		log.Warnf("credential %q is not configured", tokens[0])
		return deniedUnauthorizedConsumer()
	}

	proxywasm.AddHttpRequestHeader("X-Mse-Consumer", name)

	// 全局生效：
	// - global_auth == true 且 当前 domain/route 未配置该插件
	// - global_auth 未设置 且 没有任何一个 domain/route 配置该插件
	if (globalAuthSetTrue && noAllow) || (globalAuthNoSet && !ruleSet) {
		log.Infof("consumer %q authenticated", name)
		return authenticated(name)
	}

	// 全局生效，但当前 domain/route 配置了 allow 列表
	if globalAuthSetTrue && !noAllow {
		if !contains(config.allow, name) {
			log.Warnf("consumer %q is not allowed", name)
			return deniedUnauthorizedConsumer()
		}
		log.Infof("consumer %q authenticated", name)
		return authenticated(name)
	}

	// 非全局生效
	if globalAuthSetFalse || (globalAuthNoSet && ruleSet) {
		if !noAllow { // 配置了 allow 列表
			if !contains(config.allow, name) {
				log.Warnf("consumer %q is not allowed", name)
				return deniedUnauthorizedConsumer()
			}
			log.Infof("consumer %q authenticated", name)
			return authenticated(name)
		}
	}

	return types.ActionContinue
}

func deniedMultiKeyAuthData() types.Action {
	_ = proxywasm.SendHttpResponseWithDetail(http.StatusUnauthorized, "key-auth.multi_key", WWWAuthenticateHeader(protectionSpace),
		[]byte("Request denied by Key Auth check. Multi Key Authentication information found."), -1)
	return types.ActionContinue
}

func deniedNoKeyAuthData() types.Action {
	_ = proxywasm.SendHttpResponseWithDetail(http.StatusUnauthorized, "key-auth.no_key", WWWAuthenticateHeader(protectionSpace),
		[]byte("Request denied by Key Auth check. No Key Authentication information found."), -1)
	return types.ActionContinue
}

func deniedUnauthorizedConsumer() types.Action {
	_ = proxywasm.SendHttpResponseWithDetail(http.StatusForbidden, "key-auth.unauthorized", WWWAuthenticateHeader(protectionSpace),
		[]byte("Request denied by Key Auth check. Unauthorized consumer."), -1)
	return types.ActionContinue
}

func authenticated(name string) types.Action {
	return types.ActionContinue
}

func contains(arr []string, item string) bool {
	for _, i := range arr {
		if i == item {
			return true
		}
	}
	return false
}

func WWWAuthenticateHeader(realm string) [][2]string {
	return [][2]string{
		{"WWW-Authenticate", fmt.Sprintf("Key realm=%s", realm)},
	}
}
