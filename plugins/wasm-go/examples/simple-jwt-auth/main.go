package main

import (
	"encoding/json"
	"net/http"

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
	valid := ParseTokenValid(token, config.TokenSecretKey)
	if valid {
		return types.ActionContinue
	}
	res.Code = http.StatusUnauthorized
	res.Msg = "认证失败"
	data, _ := json.Marshal(res)
	_ = proxywasm.SendHttpResponseWithDetail(http.StatusUnauthorized, "simple-jwt-auth.auth_failed", nil, data, -1)
	return types.ActionContinue
}

func ParseTokenValid(tokenString, TokenSecretKey string) bool {
	token, _ := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// 在这里提供用于验证签名的密钥
		return []byte(TokenSecretKey), nil
	})
	return token.Valid
}
