package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

// 自定义插件配置
func main() {}

func init() {
	wrapper.SetCtx(
		"simple-jwt-auth", // 配置插件名称
		wrapper.ParseConfigBy(parseConfig),
		wrapper.ProcessRequestHeadersBy(onHttpRequestHeaders),
	)
}

type Config struct {
	TokenSecretKey string // 解析Token SecretKey
	TokenHeaders   string // 定义获取Token请求头名称
}

type Res struct {
	Code int    `json:"code"` // 返回状态码
	Msg  string `json:"msg"`  // 返回信息
}

// bearerScheme 是请求头中可选的认证方案前缀（RFC 6750 Bearer Token）。
// 调用方既可能发送 "Bearer <token>"，也可能只发送裸 token，
// 这里在解析前统一归一化，避免同一份配置对不同调用方表现不一致。
const bearerScheme = "Bearer "

// allowedJWTMethods 是插件显式接受的签名算法白名单。
// 显式声明白名单后，Parser 会在验签之前拒绝声明了其它算法（例如 "none"）
// 的 token，而不是把 token_secret_key 无条件交给这些算法处理。
var allowedJWTMethods = []string{"HS256", "HS384", "HS512"}

func parseConfig(json gjson.Result, config *Config, log log.Log) error {
	// 解析出配置，更新到config中
	config.TokenSecretKey = json.Get("token_secret_key").String()
	config.TokenHeaders = json.Get("token_headers").String()
	return nil
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config Config, log log.Log) types.Action {
	var res Res
	if config.TokenHeaders == "" || config.TokenSecretKey == "" {
		res.Code = http.StatusBadRequest
		res.Msg = "token or secret 不允许为空"
		data, _ := json.Marshal(res)
		_ = proxywasm.SendHttpResponseWithDetail(http.StatusUnauthorized, "simple-jwt-auth.bad_config", nil, data, -1)
		return types.ActionContinue
	}

	token, err := proxywasm.GetHttpRequestHeader(config.TokenHeaders)
	if err != nil {
		res.Code = http.StatusUnauthorized
		res.Msg = "认证失败"
		data, _ := json.Marshal(res)
		_ = proxywasm.SendHttpResponseWithDetail(http.StatusUnauthorized, "simple-jwt-auth.auth_failed", nil, data, -1)
		return types.ActionContinue
	}
	// 畸形 token（值为空、分段数不是三段等）只会让 ParseTokenValid 返回 false，
	// 不会再 panic；解析失败原因写入日志便于排查，响应体保持固定文案，
	// 不向未认证的调用方暴露解析细节。
	valid, parseErr := ParseTokenValid(token, config.TokenSecretKey)
	if parseErr != nil {
		log.Errorf("simple-jwt-auth: failed to parse token from header %q: %v", config.TokenHeaders, parseErr)
	}
	if valid {
		return types.ActionContinue
	}
	res.Code = http.StatusUnauthorized
	res.Msg = "认证失败"
	data, _ := json.Marshal(res)
	_ = proxywasm.SendHttpResponseWithDetail(http.StatusUnauthorized, "simple-jwt-auth.auth_failed", nil, data, -1)
	return types.ActionContinue
}

// ParseTokenValid 校验 tokenString 是否是使用 TokenSecretKey 签发的合法 JWT，
// 第二个返回值描述校验失败的原因，供调用方记录日志。
//
// github.com/dgrijalva/jwt-go 在输入无法解析时（例如分段数不是三段，
// 即 "Bearer"、"abc"、"a.b"、"a.b.c.d" 或空字符串）会返回 (nil, err)，
// 因此在读取 token.Valid 之前必须先判断 err 与 token 是否为空，
// 否则会触发 nil pointer panic —— 在 WASM 数据面中会直接 trap 掉插件实例，
// 一条未认证请求就足以让整条路由不可用。
func ParseTokenValid(tokenString, TokenSecretKey string) (bool, error) {
	token, err := parseToken(tokenString, TokenSecretKey)
	if err != nil {
		return false, err
	}
	if token == nil {
		// 防御性判断：即便解析器将来返回 (nil, nil)，也不能在这里解引用。
		return false, errors.New("jwt parser returned a nil token without an error")
	}
	return token.Valid, nil
}

// parseToken 使用算法白名单解析 token，并把 SecretKey 作为 HMAC 密钥交给解析器。
func parseToken(tokenString, tokenSecretKey string) (*jwt.Token, error) {
	parser := &jwt.Parser{ValidMethods: append([]string(nil), allowedJWTMethods...)}
	return parser.Parse(normalizeTokenString(tokenString), func(_ *jwt.Token) (interface{}, error) {
		return []byte(tokenSecretKey), nil
	})
}

// normalizeTokenString 去掉首尾空白以及可选的 "Bearer " 前缀（大小写不敏感），
// 让插件同时兼容 "Bearer <token>" 与裸 token 两种请求头写法。
func normalizeTokenString(tokenString string) string {
	tokenString = strings.TrimSpace(tokenString)
	if len(tokenString) > len(bearerScheme) && strings.EqualFold(tokenString[:len(bearerScheme)], bearerScheme) {
		return strings.TrimSpace(tokenString[len(bearerScheme):])
	}
	return tokenString
}
