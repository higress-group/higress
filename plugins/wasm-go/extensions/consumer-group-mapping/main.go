// Copyright (c) 2024 Alibaba Group Holding Ltd.
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
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

func main() {}

func init() {
	wrapper.SetCtx(
		"consumer-group-mapping",
		wrapper.ParseConfigBy(parseConfig),
		wrapper.ProcessRequestHeadersBy(onHttpRequestHeaders),
		//wrapper.ProcessResponseHeadersBy(onHttpResponseHeaders),
	)
}

const (
	// OriginalApiKeyHeader 保存原始API Key的header名称
	OriginalApiKeyHeader = "X-Original-Api-Key"
	// DefaultAuthHeader 默认的认证header名称
	DefaultAuthHeader = "Authorization"
	// DefaultTimeout 默认超时时间(毫秒)
	DefaultTimeout = 5000
)

// ConsumerGroupMappingConfig 插件配置
type ConsumerGroupMappingConfig struct {
	// HTTP 客户端
	client wrapper.HttpClient

	// @Title 认证Header名称
	// @Description 从哪个请求头获取API Key，默认为 Authorization
	// @Scope GLOBAL
	AuthHeader string `yaml:"authHeader"`

	// @Title API Key参数名称
	// @Description 调用分组信息接口时，传递API Key的参数名，默认为 key
	// @Scope GLOBAL
	ApiKeyParamName string `yaml:"apiKeyParamName"`

	// @Title 服务名称
	// @Description Higress Console 中的服务名称，用于 DNS cluster
	// @Scope GLOBAL
	ServiceName string `yaml:"serviceName"`

	// @Title API 完整 URL
	// @Description 分组信息接口的完整 URL，例如：http://api.example.com/apiKey/groupInfos
	// @Scope GLOBAL
	ApiUrl string `yaml:"apiUrl"`

	// @Title 超时时间(毫秒)
	// @Description HTTP 调用超时时间，默认为 5000ms
	// @Scope GLOBAL
	Timeout int64 `yaml:"timeout"`
}

func parseConfig(json gjson.Result, config *ConsumerGroupMappingConfig, log log.Log) error {
	// 解析 authHeader，默认为 Authorization
	config.AuthHeader = json.Get("authHeader").String()
	if config.AuthHeader == "" {
		config.AuthHeader = DefaultAuthHeader
	}

	// 解析 apiKeyParamName，默认为 key
	config.ApiKeyParamName = json.Get("apiKeyParamName").String()
	if config.ApiKeyParamName == "" {
		config.ApiKeyParamName = "key"
	}

	// 解析 serviceName（Higress Console 中的服务名称）
	config.ServiceName = json.Get("serviceName").String()
	if config.ServiceName == "" {
		log.Errorf("serviceName is required")
		return errors.New("serviceName is required")
	}

	// 解析 API URL（必填）
	config.ApiUrl = json.Get("apiUrl").String()
	if config.ApiUrl == "" {
		log.Errorf("apiUrl is required")
		return errors.New("apiUrl is required")
	}

	// 解析超时配置，默认为 5000ms
	config.Timeout = json.Get("timeout").Int()
	if config.Timeout == 0 {
		config.Timeout = DefaultTimeout
	}

	// 解析 URL，提取实际域名和端口
	parsedUrl, err := url.Parse(config.ApiUrl)
	if err != nil {
		log.Errorf("failed to parse apiUrl: %v", err)
		return errors.New("invalid apiUrl: " + err.Error())
	}

	// 提取实际域名（用于 Host header 和 TLS SNI）
	actualHost := parsedUrl.Hostname()
	if actualHost == "" {
		log.Errorf("failed to extract hostname from apiUrl: %s", config.ApiUrl)
		return errors.New("invalid apiUrl: missing hostname")
	}

	// 提取端口，默认 80（HTTP）或 443（HTTPS）
	port := int64(80) // 默认 HTTP
	if parsedUrl.Scheme == "https" {
		port = 443 // 默认 HTTPS
	}

	// 如果 URL 中显式指定了端口，使用指定的端口
	if actualPort := parsedUrl.Port(); actualPort != "" {
		if p, err := strconv.Atoi(actualPort); err == nil {
			port = int64(p)
		}
	}

	// 创建 HTTP 客户端 - 使用 DnsCluster
	// ServiceName: 服务名称（Higress Console 中配置的服务名）
	// Domain: 实际域名（从 URL 提取，用于 HTTP Host header 和 TLS SNI）
	// Port: 服务端口（从 URL 自动提取，HTTP=80, HTTPS=443）
	config.client = wrapper.NewClusterClient(wrapper.DnsCluster{
		ServiceName: config.ServiceName, // Higress Console 中的服务名称
		Domain:      actualHost,         // 实际访问的域名
		Port:        port,               // 从 URL 自动提取的端口
	})

	log.Infof("consumer-group-mapping plugin loaded:")
	log.Infof("  authHeader: %s", config.AuthHeader)
	log.Infof("  apiKeyParamName: %s", config.ApiKeyParamName)
	log.Infof("  serviceName: %s", config.ServiceName)
	log.Infof("  apiUrl: %s", config.ApiUrl)
	log.Infof("  timeout: %dms", config.Timeout)
	log.Infof("  DnsCluster.ServiceName (服务名): %s", config.ServiceName)
	log.Infof("  DnsCluster.Domain (实际域名): %s", actualHost)
	log.Infof("  DnsCluster.Port (服务端口): %d", 80)

	return nil
}

// onHttpRequestHeaders 在鉴权前执行
func onHttpRequestHeaders(ctx wrapper.HttpContext, config ConsumerGroupMappingConfig, log log.Log) types.Action {
	log.Debugf("=== Consumer Group Mapping Plugin: onHttpRequestHeaders called ===")

	// 检查是否已经进行过 groupKey 替换（防止 ResumeHttpRequest 后重复执行）
	// 当 need_restore 已在上下文中时，说明本插件已完成映射，不应再处理
	if ctx.GetContext("need_restore") != nil {
		log.Debugf("=== Consumer Group Mapping Plugin: request already mapped, skipping ===")
		return types.ActionContinue
	}

	// 1. 获取 Authorization header 的值
	authValue, err := proxywasm.GetHttpRequestHeader(config.AuthHeader)
	if err != nil || authValue == "" {
		log.Debugf("=== Consumer Group Mapping Plugin: no %s header, skipping ===", config.AuthHeader)
		return types.ActionContinue
	}

	// 2. 提取 API Key
	apiKey := extractApiKey(authValue)
	if apiKey == "" {
		log.Debugf("=== Consumer Group Mapping Plugin: failed to extract API key, skipping ===")
		return types.ActionContinue
	}


	// 3. 保存原始 API Key 到新的 header
	// 如果 X-Original-Api-Key 已经存在（例如 failback/retry 时请求头被重新处理），
	// 说明之前已经保存过原始 key，不应覆写为当前的 Authorization 值（此时可能已被 ai-proxy 替换为渠道 apiToken）
	existingKey, _ := proxywasm.GetHttpRequestHeader(OriginalApiKeyHeader)
	if existingKey != "" {
		log.Debugf("=== Consumer Group Mapping Plugin: %s already set, skipping overwrite ===", OriginalApiKeyHeader)
		return types.ActionContinue
	}
	if err := proxywasm.ReplaceHttpRequestHeader(OriginalApiKeyHeader, apiKey); err != nil {
		log.Errorf("=== Consumer Group Mapping Plugin: failed to set %s header: %v ===", OriginalApiKeyHeader, err)
		return types.ActionContinue
	}

	// 4. 保存原始值到上下文
	ctx.SetContext("original_api_key", apiKey)
	ctx.SetContext("original_auth_value", authValue)

	// 5. 构建请求路径（相对路径 + 查询参数）
	parsedUrl, _ := url.Parse(config.ApiUrl)
	requestPath := parsedUrl.Path
	if requestPath == "" {
		requestPath = "/"
	}

	// 添加查询参数
	if strings.Contains(requestPath, "?") {
		requestPath += "&" + config.ApiKeyParamName + "=" + url.QueryEscape(apiKey)
	} else {
		requestPath += "?" + config.ApiKeyParamName + "=" + url.QueryEscape(apiKey)
	}

	log.Debugf("=== Consumer Group Mapping Plugin: calling API, path: %s ===", requestPath)

	// 6. 发起异步 HTTP 调用
	// DnsCluster 会使用配置的 Domain 作为 Host header
	// 使用相对路径，http_wrapper.go 会自动添加正确的 :authority header
	err = config.client.Get(
		requestPath,
		nil,
		func(statusCode int, responseHeaders http.Header, responseBody []byte) {
			log.Debugf("=== Consumer Group Mapping Plugin: HTTP callback triggered, status=%d ===", statusCode)

			defer func() {
				if err := recover(); err != nil {
					log.Errorf("=== Consumer Group Mapping Plugin: panic recovered in callback: %v ===", err)
				}
				proxywasm.ResumeHttpRequest()
			}()

			if statusCode != http.StatusOK {
				log.Errorf("=== Consumer Group Mapping Plugin: API error status=%d, response=%s ===",
					statusCode, string(responseBody))
				return
			}

			// 解析响应
			responseStr := string(responseBody)
			if responseStr == "" {
				log.Errorf("=== Consumer Group Mapping Plugin: received empty response body ===")
				return
			}

			result := gjson.Parse(responseStr)
			if !result.Exists() || result.Type == gjson.Null {
				log.Errorf("=== Consumer Group Mapping Plugin: received null or invalid response: %s ===", responseStr)
				return
			}

			var groupKey string
			if result.IsArray() {
				if result.Array() == nil || len(result.Array()) == 0 {
					log.Errorf("=== Consumer Group Mapping Plugin: received empty array response: %s ===", responseStr)
					return
				}
				groupKey = result.Get("0.group_key").String()
			} else if result.IsObject() {
				data := result.Get("data")
				if data.Exists() && data.IsArray() {
					if data.Array() == nil || len(data.Array()) == 0 {
						log.Errorf("=== Consumer Group Mapping Plugin: data array is empty in response: %s ===", responseStr)
						return
					}
					groupKey = data.Get("0.group_key").String()
				} else {
					groupKey = result.Get("group_key").String()
				}
			}

			if groupKey == "" {
				log.Errorf("=== Consumer Group Mapping Plugin: groupKey not found or empty in response: %s ===", responseStr)
				return
			}

			log.Debugf("=== Consumer Group Mapping Plugin: fetched groupKey: %s ===", maskApiKey(groupKey))

			// 7. 替换 Authorization header
			originalAuthValue, _ := proxywasm.GetHttpRequestHeader(config.AuthHeader)
			newAuthValue := formatAuthValue(originalAuthValue, groupKey)
			log.Debugf("=== Consumer Group Mapping Plugin: before replace, authHeader=%s ===",
				maskApiKey(extractApiKey(originalAuthValue)))

			if err := proxywasm.ReplaceHttpRequestHeader(config.AuthHeader, newAuthValue); err != nil {
				log.Errorf("=== Consumer Group Mapping Plugin: failed to replace auth header: %v ===", err)
				return
			}

			// 验证替换结果
			verifyAuth, _ := proxywasm.GetHttpRequestHeader(config.AuthHeader)
			log.Debugf("=== Consumer Group Mapping Plugin: replace auth header success, new authHeader=%s ===",
				maskApiKey(extractApiKey(verifyAuth)))

			ctx.SetContext("group_api_key", groupKey)
			ctx.SetContext("need_restore", true)

		},
		uint32(config.Timeout),
	)

    log.Debugf("=== Consumer Group Mapping Plugin: async HTTP call dispatched ===")

	if err != nil {
		log.Errorf("=== Consumer Group Mapping Plugin: HTTP call failed: %v, resuming request ===", err)
		proxywasm.ResumeHttpRequest()
		return types.ActionContinue
	}

	log.Debugf("=== Consumer Group Mapping Plugin: initiated async HTTP call, pausing ===")
	ctx.DontReadRequestBody()
	return types.HeaderStopAllIterationAndWatermark
}

// onHttpResponseHeaders 在响应阶段执行
func onHttpResponseHeaders(ctx wrapper.HttpContext, config ConsumerGroupMappingConfig, log log.Log) types.Action {
	needRestore := ctx.GetContext("need_restore")
	if needRestore != true {
		return types.ActionContinue
	}

	originalApiKey := ctx.GetContext("original_api_key")
	if originalApiKey == nil {
		return types.ActionContinue
	}

	apiKey, ok := originalApiKey.(string)
	if !ok || apiKey == "" {
		return types.ActionContinue
	}

	currentAuthValue, _ := proxywasm.GetHttpRequestHeader(config.AuthHeader)
	groupApiKey := ctx.GetContext("group_api_key")
	if groupApiKey != nil {
		groupKeyStr, ok := groupApiKey.(string)
		if ok {
			currentKey := extractApiKey(currentAuthValue)
			if currentKey == groupKeyStr {
				newAuthValue := formatAuthValue(currentAuthValue, apiKey)
				if err := proxywasm.ReplaceHttpRequestHeader(config.AuthHeader, newAuthValue); err != nil {
					log.Errorf("failed to restore %s header in response phase: %v", config.AuthHeader, err)
					return types.ActionContinue
				}
			}
		}
	}

	// 清理上下文值，防止内存泄漏
	ctx.SetContext("need_restore", nil)
	ctx.SetContext("original_api_key", nil)
	ctx.SetContext("original_auth_value", nil)
	ctx.SetContext("group_api_key", nil)
	return types.ActionContinue
}

// extractApiKey 从 Authorization header 中提取 API Key
func extractApiKey(authValue string) string {
	authValue = strings.TrimSpace(authValue)

	if strings.HasPrefix(strings.ToLower(authValue), "bearer ") {
		parts := strings.SplitN(authValue, " ", 2)
		if len(parts) == 2 {
			return strings.TrimSpace(parts[1])
		}
	}

	return authValue
}

// formatAuthValue 根据原始格式格式化新的认证值
func formatAuthValue(originalAuthValue, newApiKey string) string {
	originalAuthValue = strings.TrimSpace(originalAuthValue)

	if strings.HasPrefix(strings.ToLower(originalAuthValue), "bearer ") {
		return "Bearer " + newApiKey
	}

	return newApiKey
}

// maskApiKey 遮蔽 API Key 用于日志输出
func maskApiKey(apiKey string) string {
	if len(apiKey) <= 8 {
		return "****"
	}
	return apiKey[:4] + "..." + apiKey[len(apiKey)-4:]
}
