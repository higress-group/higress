package main

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
	"time"

	"github.com/alibaba/higress/plugins/wasm-go/pkg/wrapper"
	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm"
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/tidwall/gjson"
)

func main() {
	wrapper.SetCtx(
		"oauth",
		wrapper.ParseOverrideConfigBy(parseGlobalConfig, parseOverrideRuleConfig),
		wrapper.ProcessRequestHeadersBy(onHttpRequestHeaders),
		wrapper.ProcessRequestBodyBy(onHttpRequestBody),
	)
}

// @Name oauth
// @Category auth
// @Phase AUTHN
// @Priority 320
// @Title zh-CN OAuth
// @Description zh-CN 本插件实现了JWT(JSON Web Tokens)进行OAuth2 Access Token签发、认证鉴权的能力
// @Description en-US This plugin implements functions based on JWT(JSON Web Tokens) to issue and authenticate OAuth2 Access tokens
// @IconUrl https://img.alicdn.com/imgextra/i4/O1CN01BPFGlT1pGZ2VDLgaH_!!6000000005333-2-tps-42-42.png
// @Version 1.0.0

// @Contact.name Higress Team
// @Contact.url http://higress.io/
// @Contact.email admin@higress.io

// @Example
// metadata:
//   name: oauth
//   namespace: higress-system
// spec:
//   defaultConfig:
//     consumers:
//       - name: consumer1
//         client_id: 9515b564-0b1d-11ee-9c4c-00163e1250b5
//         client_secret: 9e55de56-0b1d-11ee-b8ec-00163e1250b5
//       - name: consumer2
//         client_id: 8521b564-0b1d-11ee-9c4c-00163e1250b5
//         client_secret: 8520b564-0b1d-11ee-9c4c-00163e1250b5
//       - name: consumer3
//         client_id: 4987b564-0b1d-11ee-9c4c-00163e1250b5
//         client_secret: 498766s4-0b1d-11ee-9c4c-00163e1250b5
//       - name: consumer4
//         client_id: 5559qv64-0b1d-11ee-9c4c-00163e1250b5
//         client_secret: 58as2a84-0b1d-11ee-9c4c-00163e1250b5
//     issuer: Higress-Gateway
//     auth_path: /oauth2/token
//     global_credentials: true
//     auth_header_name: Authorization
//     token_ttl: 7200
//     clock_skew_seconds: 3153600000
//     keep_token: true
// 	   global_auth: true
//   matchRules:
//       # 规则一：按路由名称匹配生效
//       - ingress:
//         - "higress-conformance-infra/wasmplugin-oauth"
//         - "asd"
//         config:
//           allow:
//           - consumer1

//       # 规则二：按域名匹配生效
//       - domain:
//         - "*.example.com"
//         - foo.com
//         config:
//           allow:
//           - consumer3
// @End

type OAuthConfig struct {
	// @Title 调用方列表
	// @Title en-US Consumer List
	// @Description 服务调用方列表，用于对请求进行认证。
	// @Description en-US List of service consumers which will be used in request authentication.
	// @Scope GLOBAL
	consumers map[string]Consumer `yaml:"consumers"`

	// @Title 签发者
	// @Title en-US
	// @Description JWT服务签发者，用于填充JWT中的issuer
	// @Description en-US Issuer of JWT service.
	// @Scope GLOBAL
	issuer string `yaml:"issuer"`

	// @Title 签发路径
	// @Title en-US Authentication Path
	// @Description 签发token时使用的特定路由后缀，当有路由级配置，需确保路由与该签发路径匹配
	// @Description en-US Specified route suffix for issuing tokens. If route level is configured, ensure the route matches the authPath
	// @Scope GLOBAL
	authPath string `yaml:"auth_path"`

	// @Title 是否开启全局凭证
	// @Title en-US enable credentials globally or not
	// @Description 是否允许路由A下的auth_path签发的Token可以用于访问路由B
	// @Description en-US for example. whether to allow the token issued by auth_path in route A to be used to access route B
	// @Scope GLOBAL
	globalCredentials bool `yaml:"global_credentials"`

	// @Title 签发请求头的名称
	// @Title en-US name of the issuing request header
	// @Description 用于指定从哪个请求头获取JWT
	// @Description en-US It is used to specify which request header to get the JWT from
	// @Scope GLOBAL
	authHeaderName string `yaml:"auth_header_name"`

	// @Title token的有效时长，单位为秒
	// @Title en-US Time to live for a token, in seconds
	// @Scope GLOBAL
	tokenTtl uint64 `yaml:"token_ttl"`

	// @Title 时钟偏移量，单位为秒
	// @Title en-US Clock offset, in seconds
	// @Description 校验JWT的exp和iat字段时允许的时钟偏移量
	// @Description en-US The clock offset allowed when verifying the exp and iat fields of JWT
	// @Scope GLOBAL
	clockSkewSeconds uint64 `yaml:"clock_skew_seconds"`

	// @Description 转发给后端时是否保留JWT
	// @Description en-US Whether to retain JWT when forwarding to back-end
	// @Scope GLOBAL
	keepToken bool `yaml:"keep_token"`

	// @Title 授权访问的调用方列表
	// @Title en-US Allowed Consumers
	// @Description 对于匹配上述条件的请求，允许访问的调用方列表，列表包含调用者名称。依附特定路由/域名规则而存在
	// @Description en-US Consumers to be allowed for matched requests. Consisting of client_name. It exists based on specific routing/domain rules.
	// @Scope RULELOCAL
	allow []string `yaml:"allow"`

	// @Title 是否开启全局认证
	// @Title en-US Enable Global Auth
	// @Description 若配置为true，则全局生效认证机制; 若配置为false，则只对做了配置的域名和路由生效认证机制; 若不配置则仅当没有域名和路由配置时全局生效（兼容机制）
	// @Description en-US en-US If set to false, only consumer info will be accepted from the global config. Auth feature shall only be enabled if the corresponding domain or route is configured.
	// @Scope GLOBAL
	globalAuth *bool `yaml:"global_auth"`
}

type Consumer struct {
	name     string `yaml:"name"`
	clientId string `yaml:"client_id"`

	// @Title 调用方密钥
	// @Title en-US Secret key
	// @Description 签发JWT时，使用该密钥进行加密生成签名
	// @Description en-US When a JWT is issued, the secret is used to encrypt and generate a signature.
	clientSecret string `yaml:"client_secret"`
}

type Res struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

// 直接转json需要保证struct成员首字母大写
type TokenResponse struct {
	TokenType   string `json:"token_type"`
	AccessToken string `json:"access_token"`
	ExpireTime  uint   `json:"expires_in"`
}

var (
	BearerPrefix           = "Bearer "
	ClientCredentialsGrant = "client_credentials"
	checkBodyParams        = false
	routeName              = ""
	DefaultAudience        = "default"
	TypeHeader             = "application/at+jwt"
	ruleSet                = false // oauth认证是否至少在一个 domain 或 route 上生效
)

// parseGlobalConfig 读取json中的数据到global中，除Consumer的数据检查外，OAuthConfig中的其他数据在json中不存在时赋默认值
//
//	@param json
//	@param global 初始为空
//	@param log
//	@return error
func parseGlobalConfig(json gjson.Result, global *OAuthConfig, log wrapper.Log) error {
	global.issuer = "Higress-Gateway"
	global.authPath = "/oauth2/token"
	global.globalCredentials = true
	global.authHeaderName = "Authorization"
	global.tokenTtl = 7200
	global.clockSkewSeconds = 60
	global.keepToken = true

	nameSet := make(map[string]int)
	global.consumers = make(map[string]Consumer)
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

		if nameSet[name.String()] >= 1 {
			return errors.Errorf("duplicate name: %s", name.String())
		}

		nameSet[name.String()]++

		clientId := item.Get("client_id")
		if !clientId.Exists() || clientId.String() == "" {
			return errors.New("consumer client_id is required")
		}
		if _, ok := global.consumers[clientId.String()]; ok {
			return errors.Errorf("duplicate consumer client_id: %s", clientId.String())
		}

		clientSecret := item.Get("client_secret")
		if !clientSecret.Exists() || clientSecret.String() == "" {
			return errors.New("consumer client_secret is required")
		}

		consumer := Consumer{
			name:         name.String(),
			clientId:     clientId.String(),
			clientSecret: clientSecret.String(),
		}
		global.consumers[clientId.String()] = consumer
	}

	issuer := json.Get("issuer")
	if issuer.Exists() {
		global.issuer = issuer.String()
	}

	authPath := json.Get("auth_path")
	if authPath.Exists() {
		global.authPath = authPath.String()
	}

	globalCredentials := json.Get("global_credentials")
	if globalCredentials.Exists() {
		global.globalCredentials = globalCredentials.Bool()
	}

	authHeaderName := json.Get("auth_header_name")
	if authHeaderName.Exists() {
		global.authHeaderName = authHeaderName.String()
	}

	tokenTtl := json.Get("token_ttl")
	if tokenTtl.Exists() {
		global.tokenTtl = tokenTtl.Uint()
	}

	keepToken := json.Get("keep_token")
	if keepToken.Exists() {
		global.keepToken = keepToken.Bool()

	}

	clockSkewSeconds := json.Get("clock_skew_seconds")
	if clockSkewSeconds.Exists() {
		global.clockSkewSeconds = clockSkewSeconds.Uint()
	}

	globalAuth := json.Get("global_auth")
	if globalAuth.Exists() {
		ga := globalAuth.Bool()
		global.globalAuth = &ga
	}
	return nil
}

func parseOverrideRuleConfig(json gjson.Result, global OAuthConfig, config *OAuthConfig, log wrapper.Log) error {
	// override config via global
	*config = global

	allowJson := json.Get("allow")

	allow := make([]string, 0)

	if !allowJson.Exists() || len(allowJson.Array()) == 0 {
		log.Debug("allow is empty originally or not set")
	} else {
		for _, item := range allowJson.Array() {
			allow = append(allow, item.String())
		}
		ruleSet = true
	}
	config.allow = allow
	return nil
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config OAuthConfig, log wrapper.Log) types.Action {
	var res Res
	token := ""
	errMsg := ""

	routeName, _ := proxywasm.GetProperty([]string{"route_name"})
	path, _ := proxywasm.GetHttpRequestHeader(":path")

	var uriEnd int
	paramsPos := strings.Index(path, "?")
	method, _ := proxywasm.GetHttpRequestHeader(":method")
	if paramsPos == -1 {
		uriEnd = len(path)
	} else {
		uriEnd = paramsPos
	}
	if endsWith(path[:uriEnd], config.authPath) {
		if method == "GET" {
			generateToken(config, string(routeName), path, &token, &errMsg)
			goto done
		}

		if method == "POST" {
			contentType, _ := proxywasm.GetHttpRequestHeader("content-type")
			if find := strings.Contains(strings.ToLower(contentType), "application/x-www-form-urlencoded"); !find {
				errMsg = "Invalid or unsupported content-type"
				goto done
			}

			checkBodyParams = true
		}

	done:
		if errMsg != "" {
			res.Code = 400
			res.Msg = errMsg
			data, _ := json.Marshal(res)
			// TODO: SendHttpResponse和cpp版本的sendLocalResponse参数列表略有不同，暂不确定如何转成go的形式
			proxywasm.SendHttpResponse(400, nil, data, -1)
			return types.ActionPause
		}
		if token != "" {
			tR := TokenResponse{"bearer", token, uint(config.tokenTtl)}
			tokenResponse, _ := json.Marshal(tR)
			proxywasm.SendHttpResponse(200, nil, tokenResponse, -1)

		}
		return types.ActionContinue
	}
	if valid := parseTokenValid(config, string(routeName), &errMsg, log); valid {
		return types.ActionContinue
	} else {
		return types.ActionPause
	}
}

func onHttpRequestBody(ctx wrapper.HttpContext, config OAuthConfig, body []byte, log wrapper.Log) types.Action {

	var res Res
	token := ""
	errMsg := ""
	if !checkBodyParams {
		return types.ActionContinue
	}
	if len(body) == 0 {
		errMsg = "Authorize parameters are missing"
		return types.ActionContinue
	}

	// 目前只支持content-type=application/x-www-form-urlencoded，因此直接将body当url处理来得到参数
	if tokenSuccess := generateToken(config, routeName, "?"+string(body), &token, &errMsg); tokenSuccess {
		tR := TokenResponse{"bearer", token, uint(config.tokenTtl)}
		tokenResponse, _ := json.Marshal(tR)
		proxywasm.SendHttpResponse(200, nil, tokenResponse, -1)
		return types.ActionContinue
	}

	res.Code = 400
	res.Msg = errMsg
	data, _ := json.Marshal(res)
	proxywasm.SendHttpResponse(400, nil, data, -1)
	return types.ActionContinue
}

func generateToken(config OAuthConfig, routeName string, raw_params string, token *string, errMsg *string) bool {
	var consumer Consumer
	u, err := url.Parse(raw_params)
	if err != nil {
		*errMsg = err.Error()
		return false
	}
	params := u.Query()

	consumer.clientId = params.Get("client_id")
	consumer.clientSecret = params.Get("client_secret")
	grantType := params.Get("grant_type")

	if len(grantType) == 0 {
		*errMsg = "grant_type is missing"
		return false
	}
	if grantType != ClientCredentialsGrant {
		*errMsg = "grant type " + grantType + " is not supported."
		return false
	}

	if len(consumer.clientId) == 0 {
		*errMsg = "client_id is missing"
		return false
	}

	consumerInConfig, exist := config.consumers[consumer.clientId]
	if !exist {
		*errMsg = "invalid client_id or client_secret"
		return false
	}

	if len(consumerInConfig.clientSecret) == 0 {
		*errMsg = "client_secret is missing"
		return false
	}

	if consumer.clientSecret != consumerInConfig.clientSecret {
		*errMsg = "invalid client_id or client_secret"
		return false
	}

	var audience string
	if config.globalCredentials {
		audience = DefaultAudience
	} else {
		audience = routeName
	}
	nowTime := time.Now()
	claims := jwt.MapClaims{
		"client_id": consumer.clientId,
		"sub":       consumerInConfig.name,
		"exp":       jwt.NewNumericDate(nowTime.Add(time.Duration(config.tokenTtl) * time.Second)),
		"jti":       uuid.New().String(),
		"iat":       jwt.NewNumericDate(time.Now()),
		"iss":       config.issuer,
		"aud":       audience,
	}
	tokenObj := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenObj.Header["typ"] = TypeHeader

	*token, err = tokenObj.SignedString([]byte(consumer.clientSecret))
	if err != nil {
		*errMsg = "jwt sign failed: " + err.Error()
	}

	return true
}

// 基础认证：token解码->判断consumer合法性->token解密验证，失败返回401
// 签发路由匹配：当globalCredentials为false时，需保证token签发路由与当前路由匹配，失败返回403，做签发路由匹配之前必须做基础认证token解码
// 路由规则匹配：在 allow 列表中查找，如果找到则认证通过，否则认证失败，返回403

// - global_auth == true 开启全局生效：
//   - 若当前 domain/route 未配置 allow 列表，即未配置该插件，则基础认证->签发路由匹配 (1*)
//   - 若当前 domain/route 配置了该插件：则基础认证->签发路由匹配->路由规则匹配
//
// - global_auth == false 非全局生效：(2*)
//   - 若当前 domain/route 未配置该插件：则直接放行
//   - 若当前 domain/route 配置了该插件：则基础认证->签发路由匹配->路由规则匹配
//
// - global_auth 未设置：
//   - 若没有一个 domain/route 配置该插件，默认全局生效：则基础认证->签发路由匹配 (1*)
//   - 若有至少一个 domain/route 配置该插件，默认非全局生效：遵循 (2*)

// TODO：函数命名不够准确，不仅包含了检验token的逻辑，还包含了不验token直接放行的逻辑
func parseTokenValid(config OAuthConfig, routeName string, errMsg *string, log wrapper.Log) bool {
	var (
		noAllow         = len(config.allow) == 0 // 未配置 allow 列表，表示插件在该 domain/route 未生效
		globalAuthNoSet = config.globalAuth == nil
		// globalAuthSetTrue  = !globalAuthNoSet && *config.globalAuth
		globalAuthSetFalse = !globalAuthNoSet && !*config.globalAuth
		verified           = false
	)

	// 不做基础认证，签发路由匹配、和路由规则匹配而直接放行：
	// - global_auth == false 且 当前 domain/route 未配置该插件
	// - global_auth 未设置 且 有至少一个 domain/route 配置该插件（视为非全局生效，只对做了配置的域名和路由生效认证机制），且当前domain/route未配置该插件
	if noAllow && (globalAuthSetFalse || (globalAuthNoSet && ruleSet)) {
		log.Debug("authorization is not required")
		return true
	}

	{
		// 基础认证
		auth, err := proxywasm.GetHttpRequestHeader(config.authHeaderName)
		if err != nil {
			log.Debug("auth header is empty")
			goto failed
		}
		tokenIndexStart := strings.Index(auth, BearerPrefix)
		if tokenIndexStart < 0 {
			log.Debug("auth header is not a bearer token")
			goto failed
		}
		tokenIndexStart += len(BearerPrefix)
		tokenString := auth[tokenIndexStart:]

		// 按照jwt三段式进行解码
		payloadIndex, signatureIndex := strings.Index(tokenString, ".")+1, strings.LastIndex(tokenString, ".")+1
		if len(tokenString) == 0 || payloadIndex <= 0 || signatureIndex == payloadIndex {
			log.Debug("token not in jwt's format")
			goto failed
		}

		rawPayload, err := base64.RawStdEncoding.DecodeString(tokenString[payloadIndex : signatureIndex-1])
		if err != nil {
			log.Debugf("token decode fail: %s", err.Error())
			goto failed
		}
		var decodedPayload map[string]interface{}

		err = json.Unmarshal(rawPayload, &decodedPayload)

		// 从jwt解码结果中取出client_id
		rawClientIdInToken, exist := decodedPayload["client_id"]
		if !exist {
			log.Debug("client_id not found in token")
			goto failed
		}

		clientId, ok := rawClientIdInToken.(string)
		if !ok {
			log.Debugf("invalid client_id, token: %s", tokenString)
			goto failed
		}

		consumer, exist := config.consumers[clientId]
		if !exist {
			log.Debugf("client_id not found: %s", clientId)
			goto failed
		}

		// 判断该client合法性后，再用其对应的client_secret将jwt解密
		_, err = jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, errors.Errorf("Unexpected signing method: %v", token.Header["alg"])
			}

			return []byte(consumer.clientSecret), nil
		}, jwt.WithLeeway(time.Duration(config.clockSkewSeconds)*(time.Second)))

		if err != nil {
			log.Debugf("token verify failed, token: %s, reason: %s", tokenString, err.Error())
			goto failed
		}

		// 以上基础认证不通过时，返回401，以上条件都通过时，进行签发路由匹配和路由规则匹配，若不符合规则返回403
		verified = true

		// 签发路由匹配
		if !config.globalCredentials {
			rawAudienceInToken, exist := decodedPayload["aud"]
			if !exist {
				log.Debug("audience not found in token")
				goto failed
			}

			audience, ok := rawAudienceInToken.(string)
			if !ok {
				log.Debugf("invalid audience, token: %s", tokenString)
				goto failed
			}

			if audience != routeName {
				log.Debugf("audience: %s not match this route: %s", audience, routeName)
				goto failed
			}
		}

		// 满足某些条件时需进行路由规则匹配
		// 当前domain/route已配置该插件时，不论global_auth的值，都进行路由规则匹配
		if !noAllow {
			if !contains(config.allow, consumer.name) {
				routeName, _ := proxywasm.GetProperty([]string{"route_name"})
				log.Debugf("consumer: %s is not in route's: %s allow_set", consumer.name, routeName)
				goto failed
			}
		}

		// 其余情况不做路由规则匹配，验证过程结束
		// - global_auth == true 且 当前 domain/route 未配置该插件
		// - global_auth 未设置 且 没有任何一个 domain/route 配置该插件

		if !config.keepToken {
			err = proxywasm.RemoveHttpRequestHeader(config.authHeaderName)
			if err != nil {
				log.Debug("failed to remove jwt in request header")
			}
		}
		err = proxywasm.AddHttpRequestHeader("X-Mse-Consumer", consumer.name)
		if err != nil {
			log.Debug("failed to set request header")
		}
		log.Debugf("consumer %q authenticated", consumer.name)
		return true
	}
failed:
	var res Res
	if !verified {
		res.Code = 401
		res.Msg = "Invalid Jwt token: " + *errMsg
		data, _ := json.Marshal(res)
		proxywasm.SendHttpResponse(401, nil, data, -1)
	} else {
		res.Code = 403
		res.Msg = "Access Denied: " + *errMsg
		data, _ := json.Marshal(res)
		proxywasm.SendHttpResponse(403, nil, data, -1)
	}
	return false
}

func contains(arr []string, item string) bool {
	for _, i := range arr {
		if i == item {
			return true
		}
	}
	return false
}

func endsWith(str, suffix string) bool {
	if len(str) < len(suffix) {
		return false
	}
	return str[len(str)-len(suffix):] == suffix
}
