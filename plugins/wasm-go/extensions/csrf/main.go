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
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alibaba/higress/plugins/wasm-go/pkg/wrapper"
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm"
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/tidwall/gjson"
)

var (
	ruleSet         bool            // 插件是否至少在一个 domain 或 route 上生效
	protectionSpace = "MSE Gateway" // 认证失败时，返回响应头 WWW-Authenticate: Key realm=MSE Gateway
	SAFE_METHODS    = []string{http.MethodGet, http.MethodHead, http.MethodOptions}
)

const (
	DEFAULTEXPIRES  = int64(7200)
	DEFAULTCSRFNAME = "higress-csrf-token"
)

func main() {
	wrapper.SetCtx(
		"csrf", // middleware name
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

	// @Title 密钥
	// @Title en-US token
	// @Description 该调用方密钥.
	// @Description en-US The Token use to generate csrf token.
	Token string `yaml:"token"`

	// @Title 过期时间
	// @Title en-US Expires
	// @Description 过期时间. 默认值 7200s.
	// @Description en-US expires time(s) for csrf token. default 7200s.
	Expires int64 `yaml:"expires,omitempty"`
}

// @Name csrf
// @Category auth
// @Phase AUTHN
// @Priority 325
// @Title zh-CN CSRF
// @Description zh-CN 本插件基于 Double Submit Cookie 的方式，保护您的 API 免于 CSRF 攻击.
// @Description en-US This plugin plugin based on the Double Submit Cookie way, protect your API from CSRF attacks.
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
//   - name: higress-csrf-token
//     key: token1
//     expires: 7200
//   - name: custom-csrf-token
//     key: token2
//     expires: 3600
// @End
type CSRFConfig struct {
	// @Title 是否开启全局认证
	// @Title en-US Enable Global Auth
	// @Description 若不开启全局认证，则全局配置只提供凭证信息。只有在域名或路由上进行了配置才会启用认证。
	// @Description en-US If set to false, only consumer info will be accepted from the global config. Auth feature shall only be enabled if the corresponding domain or route is configured.
	// @Scope GLOBAL
	globalAuth *bool `yaml:"global_auth,omitempty"` //是否开启全局认证. 若不开启全局认证，则全局配置只提供凭证信息。只有在域名或路由上进行了配置才会启用认证。

	// @Title 调用方列表
	// @Title en-US Consumer List
	// @Description 服务调用方列表，用于对请求进行认证。
	// @Description en-US List of service consumers which will be used in request authentication.
	// @Scope GLOBAL
	consumers []Consumer `yaml:"consumers"`

	// @Title csrf名称
	// @Title en-US Name
	// @Description csrf名称. 默认值 higress-csrf-token
	// @Description en-US The csrf token name. default higress-csrf-token.
	// @Scope GLOBAL
	Key string `yaml:"key,omitempty"`

	// @Title 授权访问的调用方列表
	// @Title en-US Allowed Consumers
	// @Description 对于匹配上述条件的请求，允许访问的调用方列表。
	// @Description en-US Consumers to be allowed for matched requests.
	allow []string `yaml:"allow"`

	name2consumer map[string]Consumer `yaml:"-"`
}

type Token struct {
	Random  string `json:"random,omitempty"`
	Expires int64  `json:"expires,omitempty"`
	Sign    string `json:"sign"`
}

func parseGlobalConfig(json gjson.Result, global *CSRFConfig, log wrapper.Log) error {
	log.Debug("global config")

	// init
	ruleSet = false
	global.name2consumer = make(map[string]Consumer)

	// global_auth
	globalAuth := json.Get("global_auth")
	if globalAuth.Exists() {
		ga := globalAuth.Bool()
		global.globalAuth = &ga
	}

	// key
	key := json.Get("key")
	keyValue := DEFAULTCSRFNAME
	if key.Exists() && key.String() != "" {
		keyValue = key.String()
	}
	global.Key = keyValue

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

		token := item.Get("token")
		if !token.Exists() || token.String() == "" {
			return errors.New("consumer token is required")
		}

		if _, ok := global.name2consumer[name.String()]; ok {
			return errors.New("duplicate consumer name: " + name.String())
		}

		expires := item.Get("expires")
		expiresVaule := DEFAULTEXPIRES
		if expires.Exists() && expires.Int() > 0 {
			expiresVaule = expires.Int()
		}

		consumer := Consumer{
			Name:    name.String(),
			Token:   token.String(),
			Expires: expiresVaule,
		}
		global.consumers = append(global.consumers, consumer)
		global.name2consumer[name.String()] = consumer

	}
	return nil
}

func parseOverrideRuleConfig(json gjson.Result, global CSRFConfig, config *CSRFConfig, log wrapper.Log) error {
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

// CSRF 插件认证逻辑：
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
func onHttpRequestHeaders(ctx wrapper.HttpContext, config CSRFConfig, log wrapper.Log) types.Action {
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
	// 需要认证
	method, _ := proxywasm.GetHttpRequestHeader(":method")
	if contains(SAFE_METHODS, method) {
		log.Info("method is in SAFE_METHODS, authorization is not required")
		return types.ActionContinue
	}

	headerToken, err := proxywasm.GetHttpRequestHeader(config.Key)
	if err != nil || headerToken == "" {
		log.Warnf("No CSRF token in headers")
		return deniedNoCSRFToken()
	}

	headers, _ := proxywasm.GetHttpRequestHeaders()
	h := http.Header{}
	for _, header := range headers {
		key := header[0]
		if strings.ToLower(key) == strings.ToLower("Cookie") {
			h.Add("Cookie", header[1])
		}
	}
	req := http.Request{Header: h}
	flag := false
	name := ""
	for _, cookie := range req.Cookies() {
		if cookie.Value == headerToken {
			flag = true
			name = cookie.Name
		}
	}

	// headerToken != cookieToken
	if !flag {
		log.Warnf("No CSRF token in headers")
		return deniedNoCSRFMismatch()
	}

	consumer, ok := config.name2consumer[name]
	if !ok {
		log.Warnf("No CSRF cookie")
		return deniedNoCSRFCookie()
	}

	// check cookie
	if !checkCSRFToken(consumer, log) {
		return deniedNoCSRFVerifySignature()
	}

	// 全局生效：
	// - global_auth == true 且 当前 domain/route 未配置该插件
	// - global_auth 未设置 且 没有任何一个 domain/route 配置该插件
	if (globalAuthSetTrue && noAllow) || (globalAuthNoSet && !ruleSet) {
		log.Infof("consumer %q authenticated", name)
		return authenticated(consumer, name)
	}

	// 全局生效，但当前 domain/route 配置了 allow 列表
	if globalAuthSetTrue && !noAllow {
		if !contains(config.allow, name) {
			log.Warnf("consumer %q is not allowed", name)
			return deniedNoCSRFVerifySignature()
		}
		log.Infof("consumer %q authenticated", name)
		return authenticated(consumer, name)
	}

	// 非全局生效
	if globalAuthSetFalse || (globalAuthNoSet && ruleSet) {
		if !noAllow { // 配置了 allow 列表
			if !contains(config.allow, name) {
				log.Warnf("consumer %q is not allowed", name)
				return deniedNoCSRFVerifySignature()
			}
			log.Infof("consumer %q authenticated", name)
			return authenticated(consumer, name)
		}
	}

	return authenticated(consumer, name)
}

func genSign(random string, expires int64, token string) string {
	sha := sha256.New()
	sha.Write([]byte("{expires:" + strconv.FormatInt(expires, 10) + ",random:" + random + ",key:" + token + "}"))
	return string(sha.Sum(nil))
}

func checkCSRFToken(consumer Consumer, log wrapper.Log) bool {
	tokenStr, err := base64.RawStdEncoding.DecodeString(consumer.Token)
	if err != nil || len(tokenStr) <= 0 {
		log.Errorf("failed to csrf token base64 decode: %v", err)
		return false
	}
	var token Token
	err = json.Unmarshal(tokenStr, &token)
	if err != nil {
		log.Errorf("failed to csrf token base64 decode: %v", err)
		return false
	}

	if token.Expires <= 0 || token.Random == "" {
		log.Errorf("no expires/random in token")
		return false
	}

	if token.Sign != genSign(token.Random, token.Expires, consumer.Token) {
		log.Errorf("Invalid signatures")
		return false
	}
	return true
}

func genCSRFToken(expires int64, token string) string {
	random := rand.Float64()
	randomStr := strconv.FormatFloat(random, 'f', -1, 64)
	sign := genSign(randomStr, expires, token)
	tk := &Token{
		Random:  randomStr,
		Expires: expires,
		Sign:    sign,
	}
	tokenByte, _ := json.Marshal(tk)
	cookie := base64.RawStdEncoding.EncodeToString(tokenByte)
	return cookie
}

func deniedNoCSRFToken() types.Action {
	_ = proxywasm.SendHttpResponse(401, WWWAuthenticateHeader(protectionSpace),
		[]byte("Request denied by CSRF check. No CSRF token in headers."), -1)
	return types.ActionContinue
}

func deniedNoCSRFCookie() types.Action {
	_ = proxywasm.SendHttpResponse(401, WWWAuthenticateHeader(protectionSpace),
		[]byte("Request denied by CSRF check. No CSRF cookie."), -1)
	return types.ActionContinue
}

func deniedNoCSRFMismatch() types.Action {
	_ = proxywasm.SendHttpResponse(401, WWWAuthenticateHeader(protectionSpace),
		[]byte("Request denied by CSRF check. CSRF token mismatch."), -1)
	return types.ActionContinue
}

func deniedNoCSRFVerifySignature() types.Action {
	_ = proxywasm.SendHttpResponse(401, WWWAuthenticateHeader(protectionSpace),
		[]byte("Request denied by CSRF check. Failed to verify the CSRF token signature."), -1)
	return types.ActionContinue
}

func authenticated(consumer Consumer, key string) types.Action {
	csrfToken := genCSRFToken(consumer.Expires, consumer.Token)
	cookie := http.Cookie{
		Name:     key,
		Value:    csrfToken,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
		MaxAge:   int(consumer.Expires),
		Expires:  time.Now().Add(time.Duration(consumer.Expires) * time.Second).UTC(),
	}
	_ = proxywasm.AddHttpRequestHeader("X-Mse-Consumer", consumer.Name)
	_ = proxywasm.AddHttpResponseHeader("Set-Cookie", cookie.String())
	return types.ActionContinue
}

func WWWAuthenticateHeader(realm string) [][2]string {
	return [][2]string{
		{"WWW-Authenticate", fmt.Sprintf("Key realm=%s", realm)},
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
