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

// The 'Basic' HTTP Authentication Scheme: https://datatracker.ietf.org/doc/html/rfc7617

package main

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/pkg/errors"
	"github.com/tidwall/gjson"
)

func main() {}

func init() {
	wrapper.SetCtx(
		"basic-auth",
		wrapper.ParseOverrideConfigBy(parseGlobalConfig, parseOverrideRuleConfig),
		wrapper.ProcessRequestHeadersBy(onHttpRequestHeaders),
	)
}

// @Name basic-auth
// @Category auth
// @Phase AUTHN
// @Priority 320
// @Title zh-CN Basic Auth
// @Description zh-CN 本插件实现了基于 HTTP Basic Auth 标准进行认证鉴权的功能。
// @Description en-US This plugin implements an authentication function based on HTTP Basic Auth standard.
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
//     credential: admin:123456
//   - name: consumer2
//     credential: guest:abc
//
// @End
type BasicAuthConfig struct {
	// @Title 是否开启全局认证
	// @Title en-US Enable Global Auth
	// @Description 若不开启全局认证，则全局配置只提供凭证信息。只有在域名或路由上进行了配置才会启用认证。
	// @Description en-US If set to false, only consumer info will be accepted from the global config. Auth feature shall only be enabled if the corresponding domain or route is configured.
	// @Scope GLOBAL
	globalAuth *bool `yaml:"global_auth"`

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
	username2Passwd map[string]string `yaml:"-"`
}

type Consumer struct {
	// @Title 名称
	// @Title en-US Name
	// @Description 该调用方的名称。
	// @Description en-US The name of the consumer.
	name string `yaml:"name"`

	// @Title 访问凭证
	// @Title en-US Credential
	// @Description 该调用方的访问凭证。
	// @Description en-US The credential of the consumer.
	// @Scope GLOBAL
	credential string `yaml:"credential"`
}

var (
	ruleSet         bool            // 插件是否至少在一个 domain 或 route 上生效
	protectionSpace = "MSE Gateway" // 认证失败时，返回响应头 WWW-Authenticate: Basic realm=MSE Gateway
)

func parseGlobalConfig(json gjson.Result, global *BasicAuthConfig, log log.Log) error {
	// log.Debug("global config")
	ruleSet = false
	global.credential2Name = make(map[string]string)
	global.username2Passwd = make(map[string]string)

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
		if !credential.Exists() || credential.String() == "" {
			return errors.New("consumer credential is required")
		}
		if _, ok := global.credential2Name[credential.String()]; ok {
			return errors.Errorf("duplicate consumer credential: %s", credential.String())
		}
		userAndPasswd := strings.Split(credential.String(), ":")
		if len(userAndPasswd) != 2 {
			return errors.Errorf("invalid credential format: %s", credential.String())
		}

		consumer := Consumer{
			name:       name.String(),
			credential: credential.String(),
		}
		global.consumers = append(global.consumers, consumer)
		global.credential2Name[consumer.credential] = consumer.name
		global.username2Passwd[userAndPasswd[0]] = userAndPasswd[1]
	}

	globalAuth := json.Get("global_auth")
	if globalAuth.Exists() {
		ga := globalAuth.Bool()
		global.globalAuth = &ga
	}

	return nil
}

func parseOverrideRuleConfig(json gjson.Result, global BasicAuthConfig, config *BasicAuthConfig, log log.Log) error {
	log.Debug("domain/route config")
	// override config via global
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

// basic-auth 插件认证逻辑：
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
func onHttpRequestHeaders(ctx wrapper.HttpContext, config BasicAuthConfig, log log.Log) types.Action {
	var (
		noAllow            = len(config.allow) == 0 // 未配置 allow 列表，表示插件在该 domain/route 未生效
		globalAuthNoSet    = config.globalAuth == nil
		globalAuthSetTrue  = !globalAuthNoSet && *config.globalAuth
		globalAuthSetFalse = !globalAuthNoSet && !*config.globalAuth
	)
	// log.Debugf("global auth set: %t", !globalAuthNoSet)
	// log.Debugf("rule set: %t", ruleSet)
	// log.Debugf("config: %+v", config)

	// 不需要认证而直接放行的情况：
	// - global_auth == false 且 当前 domain/route 未配置该插件
	// - global_auth 未设置 且 有至少一个 domain/route 配置该插件 且 当前 domain/route 未配置该插件
	if globalAuthSetFalse || (globalAuthNoSet && ruleSet) {
		if noAllow {
			log.Info("authorization is not required")
			return types.ActionContinue
		}
	}

	// 以下为需要认证的情况：
	auth, err := proxywasm.GetHttpRequestHeader("Authorization")
	if err != nil {
		log.Warnf("failed to get authorization: %v", err)
		return deniedNoBasicAuthData()
	}
	if auth == "" {
		log.Warnf("authorization is empty")
		return deniedNoBasicAuthData()
	}
	if !strings.HasPrefix(auth, "Basic ") {
		log.Warnf("authorization has no prefix 'Basic '")
		return deniedNoBasicAuthData()
	}

	encodedCredential := strings.TrimPrefix(auth, "Basic ")
	credentialByte, err := base64.StdEncoding.DecodeString(encodedCredential)
	if err != nil {
		log.Warnf("failed to decode authorization %q: %v", string(credentialByte), err)
		return deniedInvalidCredentials()
	}

	credential := string(credentialByte)
	userAndPasswd := strings.Split(credential, ":")
	if len(userAndPasswd) != 2 {
		log.Warnf("invalid credential format: %s", credential)
		return deniedInvalidCredentials()
	}

	user, passwd := userAndPasswd[0], userAndPasswd[1]
	if correctPasswd, ok := config.username2Passwd[user]; !ok {
		log.Warnf("credential username %q is not configured", user)
		return deniedInvalidCredentials()
	} else {
		if passwd != correctPasswd {
			log.Warnf("credential password is not correct for username %q", user)
			return deniedInvalidCredentials()
		}
	}

	// 以下为 username 和 password 正确的情况：
	name, ok := config.credential2Name[credential]
	if !ok { // 理论上该分支永远不可达，因为 username 和 password 都是从 credential 中获取的
		log.Warnf("credential %q is not configured", credential)
		return deniedUnauthorizedConsumer()
	}

	// 全局生效：
	// - global_auth == true 且 当前 domain/route 未配置该插件
	// - global_auth 未设置 且 没有任何一个 domain/route 配置该插件
	if (globalAuthSetTrue && noAllow) || (globalAuthNoSet && !ruleSet) {
		// log.Debug("authenticated case 1")
		log.Infof("consumer %q authenticated", name)
		return authenticated(name)
	}

	// 全局生效，但当前 domain/route 配置了 allow 列表
	if globalAuthSetTrue && !noAllow {
		if !contains(config.allow, name) {
			log.Warnf("consumer %q is not allowed", name)
			return deniedUnauthorizedConsumer()
		}
		// log.Debug("authenticated case 2")
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
			// log.Debug("authenticated case 3")
			log.Infof("consumer %q authenticated", name)
			return authenticated(name)
		}
	}

	return types.ActionContinue
}

func deniedNoBasicAuthData() types.Action {
	_ = proxywasm.SendHttpResponseWithDetail(http.StatusUnauthorized, "basic-auth.no_auth_data", WWWAuthenticateHeader(protectionSpace),
		[]byte("Request denied by Basic Auth check. No Basic Authentication information found."), -1)
	return types.ActionContinue
}

func deniedInvalidCredentials() types.Action {
	_ = proxywasm.SendHttpResponseWithDetail(http.StatusUnauthorized, "basic-auth.bad_credential", WWWAuthenticateHeader(protectionSpace),
		[]byte("Request denied by Basic Auth check. Invalid username and/or password."), -1)
	return types.ActionContinue
}

func deniedUnauthorizedConsumer() types.Action {
	_ = proxywasm.SendHttpResponseWithDetail(http.StatusForbidden, "basic-auth.unauthorized", WWWAuthenticateHeader(protectionSpace),
		[]byte("Request denied by Basic Auth check. Unauthorized consumer."), -1)
	return types.ActionContinue
}

func authenticated(name string) types.Action {
	_ = proxywasm.AddHttpRequestHeader("X-Mse-Consumer", name)
	return types.ActionContinue
}

func WWWAuthenticateHeader(realm string) [][2]string {
	return [][2]string{
		{"WWW-Authenticate", fmt.Sprintf("Basic realm=%s", realm)},
	}
}

func contains(arr []string, item string) bool {
	for _, i := range arr {
		if i == item {
			return true
		}
	}
	return false
}
