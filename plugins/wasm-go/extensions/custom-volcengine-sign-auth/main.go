// Copyright (c) 2025 Alibaba Group Holding Ltd.
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
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

func main() {}

func init() {
	wrapper.SetCtx(
		"volcengine-sign-auth",
		wrapper.ParseConfig(parseConfig),
		wrapper.ProcessRequestHeadersBy(onHttpRequestHeaders),
		wrapper.ProcessRequestBodyBy(onHttpRequestBody),
	)
}

const (
	pluginName = "volcengine-sign-auth"

	// 签名算法常量
	HMAC_SHA256     = "HMAC-SHA256"
	CONTENT_TYPE    = "application/json; charset=utf-8"

	// 默认签名路径
	DEFAULT_SIGN_PATH = "/ai-api/volcengine/openapi/"

	// 请求头名称
	HeaderHost            = "Host"
	HeaderContentType     = "Content-Type"
	HeaderXDate           = "X-Date"
	HeaderXContentSha256  = "X-Content-Sha256"
	HeaderAuthorization   = "Authorization"
)

// VolcengineSignAuthConfig 插件配置
type VolcengineSignAuthConfig struct {
	// @Title Access Key ID
	// @Description 火山引擎 Access Key ID
	AccessKeyID string `yaml:"access_key_id" json:"access_key_id"`

	// @Title Secret Access Key
	// @Description 火山引擎 Secret Access Key
	SecretAccessKey string `yaml:"secret_access_key" json:"secret_access_key"`

	// @Title Host
	// @Description 签名时使用的 Host（重要：这是用于签名的固定值，如 openai.pixmax.ai）
	Host string `yaml:"host" json:"host"`

	// @Title 区域
	// @Description 服务区域，默认为 ap-southeast-1
	Region string `yaml:"region" json:"region"`

	// @Title 服务名
	// @Description 服务名，默认为 ark
	Service string `yaml:"service" json:"service"`

	// @Title 签名路径
	// @Description 签名时使用的路径，默认为 /ai-api/volcengine/openapi/
	SignPath string `yaml:"sign_path" json:"sign_path"`

	// @Title 是否启用签名
	// @Description 是否启用签名功能，默认为 true
	Enabled bool `yaml:"enabled" json:"enabled"`

	// @Title 是否覆盖已存在的签名头
	// @Description 如果请求中已存在签名头，是否覆盖，默认为 true
	OverrideExisting bool `yaml:"override_existing" json:"override_existing"`
}

func parseConfig(json gjson.Result, config *VolcengineSignAuthConfig) error {
	// 解析 access_key_id
	config.AccessKeyID = json.Get("access_key_id").String()
	if config.AccessKeyID == "" {
		return fmt.Errorf("access_key_id is required")
	}

	// 解析 secret_access_key
	config.SecretAccessKey = json.Get("secret_access_key").String()
	if config.SecretAccessKey == "" {
		return fmt.Errorf("secret_access_key is required")
	}

	// 解析 host（重要：必须配置，这是签名时使用的固定值）
	config.Host = json.Get("host").String()
	if config.Host == "" {
		return fmt.Errorf("host is required (e.g., openai.pixmax.ai)")
	}

	// 解析 region，默认为 ap-southeast-1
	config.Region = json.Get("region").String()
	if config.Region == "" {
		config.Region = "ap-southeast-1"
	}

	// 解析 service，默认为 ark
	config.Service = json.Get("service").String()
	if config.Service == "" {
		config.Service = "ark"
	}

	// 解析 sign_path，默认为 /ai-api/volcengine/openapi/
	config.SignPath = json.Get("sign_path").String()
	if config.SignPath == "" {
		config.SignPath = DEFAULT_SIGN_PATH
	}

	// 解析 enabled，默认为 true
	config.Enabled = json.Get("enabled").Bool()
	if !json.Get("enabled").Exists() {
		config.Enabled = true
	}

	// 解析 override_existing，默认为 true
	config.OverrideExisting = json.Get("override_existing").Bool()
	if !json.Get("override_existing").Exists() {
		config.OverrideExisting = true
	}

	log.Infof("volcengine-sign-auth plugin loaded: enabled=%v, access_key_id=%s, host=%s, region=%s, service=%s, sign_path=%s",
		config.Enabled, config.AccessKeyID, config.Host, config.Region, config.Service, config.SignPath)

	return nil
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config VolcengineSignAuthConfig, log log.Log) types.Action {
	// 如果禁用签名功能，直接放行
	if !config.Enabled {
		return types.ActionContinue
	}

	// 检查是否需要覆盖已存在的签名头
	if !config.OverrideExisting {
		// 检查是否已存在签名头
		existingAuth, err := proxywasm.GetHttpRequestHeader(HeaderAuthorization)
		if err == nil && existingAuth != "" {
			log.Debugf("volcengine-sign-auth: existing signature headers found, skipping (override_existing=false)")
			return types.ActionContinue
		}
	}

	// 生成 X-Date 并保存到上下文
	xDate := utcNow()
	ctx.SetContext("x_date", xDate)

	// 返回 ActionPause 暂停请求处理
	// 这样 onHttpRequestBody 会被调用，我们可以在那里添加请求头
	return types.ActionPause
}

func onHttpRequestBody(ctx wrapper.HttpContext, config VolcengineSignAuthConfig, body []byte, log log.Log) types.Action {
	if !config.Enabled {
		// 如果未启用，恢复请求
		proxywasm.ResumeHttpRequest()
		return types.ActionContinue
	}

	// 从上下文获取 X-Date
	xDateValue := ctx.GetContext("x_date")
	if xDateValue == nil {
		log.Errorf("volcengine-sign-auth: x_date not found in context")
		proxywasm.ResumeHttpRequest()
		return types.ActionContinue
	}

	xDate, ok := xDateValue.(string)
	if !ok {
		log.Errorf("volcengine-sign-auth: invalid x_date type in context")
		proxywasm.ResumeHttpRequest()
		return types.ActionContinue
	}

	// 获取请求路径和查询参数
	path, err := proxywasm.GetHttpRequestHeader(":path")
	if err != nil {
		log.Errorf("volcengine-sign-auth: failed to get request path: %v", err)
		proxywasm.ResumeHttpRequest()
		return types.ActionContinue
	}

	// 分离路径和查询参数
	query := ""
	if idx := strings.Index(path, "?"); idx != -1 {
		query = path[idx+1:]
	}

	// 计算请求体的 SHA256
	// 直接使用原始请求体，不做任何处理
	// 参考官方示例：body 字符串直接传入 sha256Hex()
	xContentSha256 := sha256Hex(body)


	// 生成 Authorization 头
	// 重要：使用配置中的 Host，而不是从请求中获取的 Host
	// 参考官方实现：cfg.host 是配置中的固定值
	authorization := config.generateAuthorization("POST", config.SignPath, query, xDate, xContentSha256, config.Host)

	log.Debugf("volcengine-sign-auth: generated authorization=%s", authorization)

	// 移除已存在的签名头（如果有）
	proxywasm.RemoveHttpRequestHeader(HeaderXDate)
	proxywasm.RemoveHttpRequestHeader(HeaderXContentSha256)
	proxywasm.RemoveHttpRequestHeader(HeaderAuthorization)

// 	// 添加签名请求头
// 	// 打印所有签名头值用于调试
	log.Infof("volcengine-sign-auth: [DEBUG] X-Date=%s", xDate)
	log.Infof("volcengine-sign-auth: [DEBUG] X-Content-Sha256=%s", xContentSha256)
	log.Infof("volcengine-sign-auth: [DEBUG] Authorization=%s", authorization)
	log.Infof("volcengine-sign-auth: [DEBUG] Content-Type=%s", CONTENT_TYPE)

	if err := proxywasm.AddHttpRequestHeader(HeaderXDate, xDate); err != nil {
		log.Errorf("volcengine-sign-auth: failed to add %s header: %v", HeaderXDate, err)
	}
	if err := proxywasm.AddHttpRequestHeader(HeaderXContentSha256, xContentSha256); err != nil {
		log.Errorf("volcengine-sign-auth: failed to add %s header: %v", HeaderXContentSha256, err)
	}
	if err := proxywasm.AddHttpRequestHeader(HeaderAuthorization, authorization); err != nil {
		log.Errorf("volcengine-sign-auth: failed to add %s header: %v", HeaderAuthorization, err)
	}

	// 设置 Content-Type
	// 重要：必须使用固定的 CONTENT_TYPE 进行签名，所以需要覆盖已存在的 Content-Type
	// 始终设置 Content-Type，因为签名中包含了 content-type 头
	proxywasm.RemoveHttpRequestHeader(HeaderContentType)
	if err := proxywasm.AddHttpRequestHeader(HeaderContentType, CONTENT_TYPE); err != nil {
		log.Errorf("volcengine-sign-auth: failed to set %s header: %v", HeaderContentType, err)
	}

// 	log.Infof("volcengine-sign-auth: signature headers added, resuming request")

	// 恢复请求处理
	proxywasm.ResumeHttpRequest()
	return types.ActionContinue
}

// generateAuthorization 生成火山引擎签名 v4 的 Authorization 头
func (config *VolcengineSignAuthConfig) generateAuthorization(method, path, query, xDate, xContentSha256, host string) string {
	// 打印输入参数
	log.Infof("volcengine-sign-auth: [DEBUG] generateAuthorization inputs:")
	log.Infof("volcengine-sign-auth: [DEBUG]   method=%s", method)
	log.Infof("volcengine-sign-auth: [DEBUG]   path=%s", path)
	log.Infof("volcengine-sign-auth: [DEBUG]   query=%s", query)
	log.Infof("volcengine-sign-auth: [DEBUG]   xDate=%s", xDate)
	log.Infof("volcengine-sign-auth: [DEBUG]   xContentSha256=%s", xContentSha256)
	log.Infof("volcengine-sign-auth: [DEBUG]   host=%s", host)
	log.Infof("volcengine-sign-auth: [DEBUG]   config.Region=%s", config.Region)
	log.Infof("volcengine-sign-auth: [DEBUG]   config.Service=%s", config.Service)

	signedHeaders := "content-type;host;x-content-sha256;x-date"

	// 构造规范请求
	canonicalRequest := fmt.Sprintf(
		"%s\n%s\n%s\ncontent-type:%s\nhost:%s\nx-content-sha256:%s\nx-date:%s\n\n%s\n%s",
		method,
		path,
		canonicalQuery(query),
		CONTENT_TYPE,
		host,
		xContentSha256,
		xDate,
		signedHeaders,
		xContentSha256,
	)

	// 打印规范请求用于调试
// 	log.Infof("volcengine-sign-auth: [DEBUG] CanonicalRequest:\n%s", canonicalRequest)

	// 对规范请求进行哈希
	hashedCanonicalRequest := sha256Hex([]byte(canonicalRequest))
// 	log.Infof("volcengine-sign-auth: [DEBUG] HashedCanonicalRequest=%s", hashedCanonicalRequest)

	// 构造签名范围
	shortXDate := xDate[:8]
	credentialScope := fmt.Sprintf("%s/%s/%s/request", shortXDate, config.Region, config.Service)
// 	log.Infof("volcengine-sign-auth: [DEBUG] CredentialScope=%s", credentialScope)

	// 构造待签名字符串
	stringToSign := fmt.Sprintf(
		"%s\n%s\n%s\n%s",
		HMAC_SHA256,
		xDate,
		credentialScope,
		hashedCanonicalRequest,
	)
// 	log.Infof("volcengine-sign-auth: [DEBUG] StringToSign:\n%s", stringToSign)

	// 生成签名密钥
	signingKey := genSigningSecretKeyV4(
		[]byte(config.SecretAccessKey),
		shortXDate,
		config.Region,
		config.Service,
	)

	// 计算签名
	signature := hmacSha256Hex(signingKey, stringToSign)
// 	log.Infof("volcengine-sign-auth: [DEBUG] Signature=%s", signature)

	// 构造 Authorization 头
	return fmt.Sprintf(
		"%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		HMAC_SHA256,
		config.AccessKeyID,
		credentialScope,
		signedHeaders,
		signature,
	)
}

// utcNow 获取当前 UTC 时间，格式为 YYYYMMDDTHHMMSSZ
func utcNow() string {
	return time.Now().UTC().Format("20060102T150405Z")
}

// sha256Hex 计算 SHA256 哈希值并返回十六进制字符串
func sha256Hex(content []byte) string {
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:])
}

// hmacSha256Hex 计算 HMAC-SHA256 并返回十六进制字符串
func hmacSha256Hex(key []byte, content string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(content))
	return hex.EncodeToString(mac.Sum(nil))
}

// hmacSha256Bytes 计算 HMAC-SHA256 并返回字节
func hmacSha256Bytes(key []byte, content string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(content))
	return mac.Sum(nil)
}

// genSigningSecretKeyV4 生成签名密钥
func genSigningSecretKeyV4(secretKey []byte, date, region, service string) []byte {
	kDate := hmacSha256Bytes(secretKey, date)
	kRegion := hmacSha256Bytes(kDate, region)
	kService := hmacSha256Bytes(kRegion, service)
	kRequest := hmacSha256Bytes(kService, "request")
	return kRequest
}

// canonicalQuery 构造规范查询字符串
// 参考官方实现：先解析，排序，然后进行 URL 编码
func canonicalQuery(query string) string {
	if query == "" {
		return ""
	}

	queryMap := make(map[string]string)
	for _, part := range strings.Split(query, "&") {
		keyValue := strings.SplitN(part, "=", 2)
		key := keyValue[0]
		value := ""
		if len(keyValue) == 2 {
			value = keyValue[1]
		}
		queryMap[key] = value
	}

	keys := make([]string, 0, len(queryMap))
	for key := range queryMap {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	pairs := make([]string, 0, len(keys))
	for _, key := range keys {
		pairs = append(pairs, signStringEncode(key)+"="+signStringEncode(queryMap[key]))
	}
	return strings.Join(pairs, "&")
}

// signStringEncode 对字符串进行 URL 编码（遵循签名规范）
// 参考官方实现
func signStringEncode(source string) string {
	if source == "" {
		return ""
	}

	var buffer strings.Builder
	for _, b := range []byte(source) {
		if isUrlEncodedByte(b) {
			buffer.WriteByte(b)
			continue
		}
		if b == ' ' {
			buffer.WriteString("%20")
			continue
		}
		buffer.WriteByte('%')
		buffer.WriteByte("0123456789ABCDEF"[b>>4])
		buffer.WriteByte("0123456789ABCDEF"[b&0x0F])
	}
	return buffer.String()
}

// isUrlEncodedByte 判断字节是否需要编码
func isUrlEncodedByte(c byte) bool {
	return (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') ||
		c == '-' || c == '_' || c == '.' || c == '~'
}

// maskAuth 遮蔽 Authorization 用于日志输出（只显示前20位和后20位）
func maskAuth(auth string) string {
	if len(auth) <= 40 {
		return "****"
	}
	return auth[:20] + "..." + auth[len(auth)-20:]
}
