package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"mime/multipart"
	"strings"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	pluginName = "ai-adapter"

	// 上下文键
	ctxKeyModel        = "model"
	ctxKeyProvider     = "provider"
	ctxKeyMappingIndex = "mapping_index"
	ctxKeyOriginalPath = "original_path"
	ctxKeyOriginalBody = "original_body"
	// 与 ai-proxy/provider/provider.go 中的 CtxKeyApiName 保持一致
	ctxKeyApiName = "apiName"

	// 最大 body 大小限制
	defaultMaxBodyBytes uint32 = 100 * 1024 * 1024
)

type PluginConfig struct {
	// 按 URI 和 JSON 字段匹配的请求体规则，先于原有 body 转换执行。
	RequestBodyRules []RequestBodyRule `json:"requestBodyRules"`

	// 模型到渠道的映射配置
	ModelProviderMappings []ModelProviderMapping `json:"modelProviderMappings"`

	// 是否启用请求转换
	EnableRequestTransform bool `json:"enableRequestTransform"`

	// 是否启用响应转换
	EnableResponseTransform bool `json:"enableResponseTransform"`

	// 图片下载服务名称
	ImageDownloadServiceName string `json:"imageDownloadServiceName"`
}

type ModelProviderMapping struct {
	// 模型名称（支持通配符 *）
	Model string `json:"model"`

	// Provider 名称
	Provider string `json:"provider"`

	// 请求转换配置
	RequestTransform *TransformConfig `json:"requestTransform,omitempty"`

	// 响应转换配置
	ResponseTransform *TransformConfig `json:"responseTransform,omitempty"`
}

type TransformConfig struct {
	// 转换类型
	Type string `json:"type"` // "url_rewrite", "param_transform", "format_transform", "header_transform", "body_transform", "none"

	// URL 重写配置
	URLRewrite *URLRewriteConfig `json:"urlRewrite,omitempty"`

	// 参数转换配置
	ParamTransform *ParamTransformConfig `json:"paramTransform,omitempty"`

	// 格式转换配置
	FormatTransform *FormatTransformConfig `json:"formatTransform,omitempty"`

	// 头转换配置
	HeaderTransform *HeaderTransformConfig `json:"headerTransform,omitempty"`

	// Body 转换配置
	BodyTransform *BodyTransformConfig `json:"bodyTransform,omitempty"`
}

type URLRewriteConfig struct {
	// 新的路径
	Path string `json:"path,omitempty"`

	// 路径重写规则（使用正则表达式）
	FromPattern string `json:"fromPattern,omitempty"`
	ToPattern   string `json:"toPattern,omitempty"`
}

type ParamTransformConfig struct {
	// 参数重命名映射
	RenameMap map[string]string `json:"renameMap,omitempty"`

	// 要删除的参数列表
	RemoveParams []string `json:"removeParams,omitempty"`

	// 要添加的参数
	AddParams map[string]interface{} `json:"addParams,omitempty"`
}

type FormatTransformConfig struct {
	// 目标格式: "multipart", "json", "form"
	TargetFormat string `json:"targetFormat"`

	// multipart 配置
	MultipartConfig *MultipartTransformConfig `json:"multipartConfig,omitempty"`
}

type MultipartTransformConfig struct {
	// 字段名映射：指定哪些 JSON 字段需要转换为 multipart 文件流
	// 格式：JSON字段名: multipart字段名
	FieldMapping map[string]string `json:"fieldMapping,omitempty"`

	// 要添加的额外表单字段
	AddFields map[string]string `json:"addFields,omitempty"`
}

type HeaderTransformConfig struct {
	// 要添加的头
	AddHeaders map[string]string `json:"addHeaders,omitempty"`

	// 要移除的头
	RemoveHeaders []string `json:"removeHeaders,omitempty"`

	// 要重命名的头
	RenameHeaders map[string]string `json:"renameHeaders,omitempty"`
}

type BodyTransformConfig struct {
	// JSON 路径和值的映射
	SetValues map[string]interface{} `json:"setValues,omitempty"`

	// 要删除的 JSON 路径
	RemovePaths []string `json:"removePaths,omitempty"`
}

func main() {}

func init() {
	wrapper.SetCtx(
		pluginName,
		wrapper.ParseConfigBy(parseConfig),
		wrapper.ProcessRequestHeadersBy(onHttpRequestHeaders),
		wrapper.ProcessRequestBodyBy(onHttpRequestBody),
		// 		wrapper.ProcessResponseHeadersBy(onHttpResponseHeaders),
		// 		wrapper.ProcessResponseBodyBy(onHttpResponseBody),
		wrapper.WithRebuildAfterRequests[PluginConfig](1000),
		wrapper.WithRebuildMaxMemBytes[PluginConfig](200*1024*1024),
	)
}

// parseConfig 解析插件配置
func parseConfig(json gjson.Result, config *PluginConfig, log log.Log) error {
	rules, err := parseRequestBodyRules(json.Get("requestBodyRules"))
	if err != nil {
		return err
	}
	config.RequestBodyRules = rules

	if log != nil {
		log.Infof("parsing config: %s", json.String())
	}

	// 解析模型到渠道的映射
	mappingsJson := json.Get("modelProviderMappings")
	if mappingsJson.Exists() && mappingsJson.IsArray() {
		config.ModelProviderMappings = make([]ModelProviderMapping, 0, mappingsJson.Get("#").Int())
		for _, mappingJson := range mappingsJson.Array() {
			mapping := ModelProviderMapping{
				Model:    mappingJson.Get("model").String(),
				Provider: mappingJson.Get("provider").String(),
			}

			// 解析请求转换配置
			if requestTransformJson := mappingJson.Get("requestTransform"); requestTransformJson.Exists() {
				mapping.RequestTransform = parseTransformConfig(requestTransformJson)
			}

			// 解析响应转换配置
			if responseTransformJson := mappingJson.Get("responseTransform"); responseTransformJson.Exists() {
				mapping.ResponseTransform = parseTransformConfig(responseTransformJson)
			}

			config.ModelProviderMappings = append(config.ModelProviderMappings, mapping)
		}
	}

	// 解析启用标志
	config.EnableRequestTransform = json.Get("enableRequestTransform").Bool()
	config.EnableResponseTransform = json.Get("enableResponseTransform").Bool()

	// 解析图片下载服务名称
	config.ImageDownloadServiceName = json.Get("imageDownloadServiceName").String()
	if config.ImageDownloadServiceName == "" {
		config.ImageDownloadServiceName = "wujieai-cdn" // 默认值
	}

	if log != nil {
		log.Infof("config loaded: %d mappings, %d request body rules, image download service: %s", len(config.ModelProviderMappings), len(config.RequestBodyRules), config.ImageDownloadServiceName)
	}

	return nil
}

// parseTransformConfig 解析转换配置
func parseTransformConfig(json gjson.Result) *TransformConfig {
	transform := &TransformConfig{
		Type: json.Get("type").String(),
	}

	// 解析 URL 重写配置
	if urlRewriteJson := json.Get("urlRewrite"); urlRewriteJson.Exists() {
		transform.URLRewrite = &URLRewriteConfig{
			Path:        urlRewriteJson.Get("path").String(),
			FromPattern: urlRewriteJson.Get("fromPattern").String(),
			ToPattern:   urlRewriteJson.Get("toPattern").String(),
		}
	}

	// 解析参数转换配置
	if paramTransformJson := json.Get("paramTransform"); paramTransformJson.Exists() {
		renameMap := make(map[string]string)
		paramTransformJson.Get("renameMap").ForEach(func(key, value gjson.Result) bool {
			renameMap[key.String()] = value.String()
			return true
		})

		addParams := make(map[string]interface{})
		paramTransformJson.Get("addParams").ForEach(func(key, value gjson.Result) bool {
			addParams[key.String()] = value.Value()
			return true
		})

		transform.ParamTransform = &ParamTransformConfig{
			RenameMap:    renameMap,
			RemoveParams: extractStrings(paramTransformJson.Get("removeParams")),
			AddParams:    addParams,
		}
	}

	// 解析格式转换配置
	if formatTransformJson := json.Get("formatTransform"); formatTransformJson.Exists() {
		transform.FormatTransform = &FormatTransformConfig{
			TargetFormat: formatTransformJson.Get("targetFormat").String(),
		}

		if multipartJson := formatTransformJson.Get("multipartConfig"); multipartJson.Exists() {
			fieldMapping := make(map[string]string)
			multipartJson.Get("fieldMapping").ForEach(func(key, value gjson.Result) bool {
				fieldMapping[key.String()] = value.String()
				return true
			})

			addFields := make(map[string]string)
			multipartJson.Get("addFields").ForEach(func(key, value gjson.Result) bool {
				addFields[key.String()] = value.String()
				return true
			})

			transform.FormatTransform.MultipartConfig = &MultipartTransformConfig{
				FieldMapping: fieldMapping,
				AddFields:    addFields,
			}
		}
	}

	// 解析头转换配置
	if headerTransformJson := json.Get("headerTransform"); headerTransformJson.Exists() {
		addHeaders := make(map[string]string)
		headerTransformJson.Get("addHeaders").ForEach(func(key, value gjson.Result) bool {
			addHeaders[key.String()] = value.String()
			return true
		})

		renameHeaders := make(map[string]string)
		headerTransformJson.Get("renameHeaders").ForEach(func(key, value gjson.Result) bool {
			renameHeaders[key.String()] = value.String()
			return true
		})

		transform.HeaderTransform = &HeaderTransformConfig{
			AddHeaders:    addHeaders,
			RemoveHeaders: extractStrings(headerTransformJson.Get("removeHeaders")),
			RenameHeaders: renameHeaders,
		}
	}

	// 解析 Body 转换配置
	if bodyTransformJson := json.Get("bodyTransform"); bodyTransformJson.Exists() {
		setValues := make(map[string]interface{})
		bodyTransformJson.Get("setValues").ForEach(func(key, value gjson.Result) bool {
			setValues[key.String()] = value.Value()
			return true
		})

		transform.BodyTransform = &BodyTransformConfig{
			SetValues:   setValues,
			RemovePaths: extractStrings(bodyTransformJson.Get("removePaths")),
		}
	}

	return transform
}

// extractStrings 从 gjson 结果中提取字符串数组
func extractStrings(json gjson.Result) []string {
	if !json.Exists() || !json.IsArray() {
		return nil
	}
	result := make([]string, 0, json.Get("#").Int())
	json.ForEach(func(_, value gjson.Result) bool {
		result = append(result, value.String())
		return true
	})
	return result
}

// getClusterName 获取当前路由的服务提供者名称（cluster_name）
// cluster_name 格式: outbound|443||llm-RightCodes.internal.dns
// 提取其中的服务名称部分，例如: RightCodes
func getClusterName(ctx wrapper.HttpContext) string {
	if raw, err := proxywasm.GetProperty([]string{"cluster_name"}); err == nil {
		clusterName := string(raw)
		if clusterName != "" {
			log.Debugf("got cluster name from cluster_name: %s", clusterName)

			// cluster_name 格式: outbound|443||llm-RightCodes.internal.dns
			// 提取 llm- 后面的服务名称部分
			// 例如: llm-RightCodes.internal.dns -> RightCodes
			parts := strings.Split(clusterName, "||")
			if len(parts) >= 2 {
				serviceName := parts[1]
				log.Debugf("service name part: %s", serviceName)

				// 移除 llm- 前缀（如果存在）
				serviceName = strings.TrimPrefix(serviceName, "llm-")

				// 提取第一个点之前的部分（例如: RightCodes.internal.dns -> RightCodes）
				if idx := strings.Index(serviceName, "."); idx > 0 {
					serviceName = serviceName[:idx]
				}

				log.Debugf("extracted provider name: %s", serviceName)
				return serviceName
			}

			// 如果格式不匹配，返回原始值
			return clusterName
		}
	}
	return ""
}

// findTransformMappingByProvider 根据 provider 名称查找转换配置
// 返回匹配的映射配置索引
func findTransformMappingByProvider(config PluginConfig, provider string) int {
	// 先精确匹配
	for i, mapping := range config.ModelProviderMappings {
		if mapping.Provider == provider {
			return i
		}
	}

	// 通配符匹配
	for i, mapping := range config.ModelProviderMappings {
		if mapping.Provider == "*" {
			return i
		}
		if strings.HasSuffix(mapping.Provider, "*") {
			prefix := strings.TrimSuffix(mapping.Provider, "*")
			if prefix != "" && strings.HasPrefix(provider, prefix) {
				return i
			}
		}
	}

	return -1
}

// matchModel 检查模型名称是否匹配配置中的模型模式（支持通配符）
func matchModel(model, pattern string) bool {
	// 精确匹配
	if model == pattern {
		return true
	}

	// 通配符匹配
	if pattern == "*" {
		return true
	}

	// 前缀通配符匹配（如 "gpt-*"）
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		if prefix != "" && strings.HasPrefix(model, prefix) {
			return true
		}
	}

	return false
}

// findTransformMappingByModelAndProvider 根据模型名称和 provider 名称查找转换配置
// 需要同时匹配模型和 provider
// 返回匹配的映射配置索引
func findTransformMappingByModelAndProvider(config PluginConfig, model, provider string) int {
	log.Debugf("findTransformMappingByModelAndProvider called: model=%s, provider=%s", model, provider)
	log.Debugf("Available mappings count: %d", len(config.ModelProviderMappings))

	// 先精确匹配 provider，再匹配模型
	for i, mapping := range config.ModelProviderMappings {
		log.Debugf("Checking mapping %d: mapping.Provider=%s, mapping.Model=%s", i, mapping.Provider, mapping.Model)
		if mapping.Provider == provider {
			log.Debugf("Provider matched! Checking model: mapping.Model != %s = %v", "", mapping.Model != "")
			if mapping.Model != "" {
				matched := matchModel(model, mapping.Model)
				log.Debugf("Model matching result: model=%s, pattern=%s, matched=%v", model, mapping.Model, matched)
				if matched {
					log.Debugf("Found matching mapping at index %d", i)
					return i
				}
			} else {
				log.Debugf("mapping.Model is empty, skipping model match")
			}
		}
	}

	log.Debugf("No exact provider match found, trying wildcard matching")

	// provider 通配符匹配
	for i, mapping := range config.ModelProviderMappings {
		if mapping.Provider == "*" {
			if mapping.Model != "" && matchModel(model, mapping.Model) {
				return i
			}
		}
		if strings.HasSuffix(mapping.Provider, "*") {
			prefix := strings.TrimSuffix(mapping.Provider, "*")
			if prefix != "" && strings.HasPrefix(provider, prefix) {
				if mapping.Model != "" && matchModel(model, mapping.Model) {
					return i
				}
			}
		}
	}

	log.Debugf("No matching mapping found, returning -1")
	return -1
}

// onLegacyRequestHeaders 处理原有模型/渠道转换的请求头
func onLegacyRequestHeaders(ctx wrapper.HttpContext, config PluginConfig, log log.Log) types.Action {
	ctx.DisableReroute()

	// 获取模型名称（用于判断是否需要处理此请求）
	model := getModelFromRequest(ctx)
	if model == "" {
		log.Debugf("no model found in request, skip processing")
		ctx.DontReadRequestBody()
		return types.ActionContinue
	}

	log.Infof("got model name from request: %s", model)

	// 保存上下文
	ctx.SetContext(ctxKeyModel, model)

	// 检查是否有配置适用于此模型
	// 一个模型可以对应多个 provider，我们需要找到至少一个配置匹配此模型
	hasModelConfig := false
	for _, mapping := range config.ModelProviderMappings {
		if mapping.Model != "" && matchModel(model, mapping.Model) {
			hasModelConfig = true
			break
		}
	}

	if !hasModelConfig {
		log.Debugf("no configuration found for model: %s, skip processing", model)
		ctx.DontReadRequestBody()
		return types.ActionContinue
	}

	// 获取当前路由的服务提供者名称（cluster_name）
	// 这是由 Higress 负载均衡后实际路由到的服务
	provider := getClusterName(ctx)
	if provider == "" {
		log.Debugf("no cluster name found in request, skip processing")
		ctx.DontReadRequestBody()
		return types.ActionContinue
	}

	log.Infof("matched cluster (provider): %s for model: %s", provider, model)

	// 保存上下文
	ctx.SetContext(ctxKeyProvider, provider)

	// 保存原始路径
	originalPath, _ := proxywasm.GetHttpRequestHeader(":path")
	ctx.SetContext(ctxKeyOriginalPath, originalPath)

	// 如果没有启用请求转换，直接继续
	if !config.EnableRequestTransform {
		log.Debugf("request transform not enabled")
		ctx.DontReadRequestBody()
		return types.ActionContinue
	}

	// 根据模型和 provider 查找匹配的转换配置
	// 需要同时匹配模型和 provider
	mappingIndex := findTransformMappingByModelAndProvider(config, model, provider)
	if mappingIndex < 0 {
		log.Debugf("no transform configured for model: %s and provider: %s", model, provider)
		ctx.DontReadRequestBody()
		return types.ActionContinue
	}

	// 保存映射索引
	ctx.SetContext(ctxKeyMappingIndex, mappingIndex)

	mapping := &config.ModelProviderMappings[mappingIndex]
	if mapping.RequestTransform == nil {
		log.Debugf("no request transform configured for provider: %s", provider)
		ctx.DontReadRequestBody()
		return types.ActionContinue
	}

	transform := mapping.RequestTransform

	// 检查转换类型是否有效
	if transform.Type == "" {
		log.Debugf("transform type is empty for provider: %s", provider)
		ctx.DontReadRequestBody()
		return types.ActionContinue
	}

	// 处理 URL 重写（可以在请求头阶段完成）
	if transform.Type == "url_rewrite" && transform.URLRewrite != nil {
		newPath, err := applyURLRewrite(ctx, transform.URLRewrite, originalPath)
		if err != nil {
			log.Errorf("failed to apply URL rewrite: %v", err)
		} else {
			log.Infof("URL rewritten from %s to %s for provider: %s", originalPath, newPath, provider)
			// 如果只有 URL 重写，不需要处理 body
			ctx.DontReadRequestBody()
			return types.ActionContinue
		}
	}

	// 处理头转换（可以在请求头阶段完成）
	if transform.Type == "header_transform" && transform.HeaderTransform != nil {
		if err := applyHeaderTransform(transform.HeaderTransform, true); err != nil {
			log.Errorf("failed to apply header transform: %v", err)
		}
		// 头转换完成后，不需要处理 body
		ctx.DontReadRequestBody()
		return types.ActionContinue
	}

	// 对于需要处理 body 的转换，停止迭代，等待 body
	if transform.Type == "format_transform" || transform.Type == "param_transform" || transform.Type == "body_transform" {
		ctx.SetRequestBodyBufferLimit(defaultMaxBodyBytes)
		return types.HeaderStopIteration
	}

	ctx.DontReadRequestBody()
	return types.ActionContinue
}

// onHttpRequestBody 处理请求体
func onHttpRequestBody(ctx wrapper.HttpContext, config PluginConfig, body []byte, log log.Log) types.Action {
	if rules, ok := ctx.GetContext(ctxKeyBodyRules).([]RequestBodyRule); ok {
		updated, changed, err := applyRequestBodyRules(body, rules)
		if err != nil {
			log.Warnf("request body rule skipped: %v", err)
		} else if changed {
			if err := proxywasm.ReplaceHttpRequestBody(updated); err != nil {
				log.Errorf("failed to replace request body for rule: %v", err)
			} else {
				body = updated
				log.Infof("request body rule applied")
			}
		}
	}

	// 从上下文中获取映射索引（避免重复查找）
	mappingIndex, ok := ctx.GetContext(ctxKeyMappingIndex).(int)
	if !ok || mappingIndex < 0 || mappingIndex >= len(config.ModelProviderMappings) {
		log.Debugf("no valid mapping index in context, skip processing")
		return types.ActionContinue
	}

	mapping := &config.ModelProviderMappings[mappingIndex]
	provider := mapping.Provider

	if mapping.RequestTransform == nil {
		return types.ActionContinue
	}

	transform := mapping.RequestTransform
	var transformedBody []byte = body
	var err error

	// 应用转换
	switch transform.Type {
	case "format_transform":
		if transform.FormatTransform != nil {
			transformedBody, err = applyFormatTransform(transform.FormatTransform, body, log)
		}
	case "param_transform":
		if transform.ParamTransform != nil {
			transformedBody, err = applyParamTransform(transform.ParamTransform, body)
		}
	case "body_transform":
		if transform.BodyTransform != nil {
			transformedBody, err = applyBodyTransform(transform.BodyTransform, body)
		}
	default:
		return types.ActionContinue
	}

	if err != nil {
		log.Errorf("failed to apply transform type %s: %v", transform.Type, err)
		return types.ActionContinue
	}

	// 如果 body 被转换，替换请求 body
	if transformedBody != nil && !bytes.Equal(body, transformedBody) {
		if err := proxywasm.ReplaceHttpRequestBody(transformedBody); err != nil {
			log.Errorf("failed to replace request body: %v", err)
		} else {
			log.Infof("request body transformed for provider: %s", provider)
		}
	}

	return types.ActionContinue
}

// onHttpResponseHeaders 处理响应头
func onHttpResponseHeaders(ctx wrapper.HttpContext, config PluginConfig, log log.Log) types.Action {
	if !wrapper.IsResponseFromUpstream() {
		ctx.DontReadResponseBody()
		return types.ActionContinue
	}

	// 从上下文中获取映射索引（避免重复查找）
	mappingIndex, ok := ctx.GetContext(ctxKeyMappingIndex).(int)
	if !ok || mappingIndex < 0 || mappingIndex >= len(config.ModelProviderMappings) {
		log.Debugf("no valid mapping index in context, skip processing")
		ctx.DontReadResponseBody()
		return types.ActionContinue
	}

	// 如果没有启用响应转换，直接继续
	if !config.EnableResponseTransform {
		ctx.DontReadResponseBody()
		return types.ActionContinue
	}

	mapping := &config.ModelProviderMappings[mappingIndex]
	if mapping.ResponseTransform == nil {
		ctx.DontReadResponseBody()
		return types.ActionContinue
	}

	transform := mapping.ResponseTransform

	// 检查转换类型是否有效
	if transform.Type == "" {
		ctx.DontReadResponseBody()
		return types.ActionContinue
	}

	// 处理头转换（可以在响应头阶段完成）
	if transform.Type == "header_transform" && transform.HeaderTransform != nil {
		if err := applyHeaderTransform(transform.HeaderTransform, false); err != nil {
			log.Errorf("failed to apply response header transform: %v", err)
		}
		ctx.DontReadResponseBody()
		return types.ActionContinue
	}

	// 对于需要处理 body 的转换，读取响应 body
	if transform.Type == "format_transform" || transform.Type == "body_transform" || transform.Type == "param_transform" {
		ctx.SetResponseBodyBufferLimit(defaultMaxBodyBytes)
		return types.ActionContinue
	}

	ctx.DontReadResponseBody()
	return types.ActionContinue
}

// onHttpResponseBody 处理响应体
func onHttpResponseBody(ctx wrapper.HttpContext, config PluginConfig, body []byte, log log.Log) types.Action {
	// 从上下文中获取映射索引（避免重复查找）
	mappingIndex, ok := ctx.GetContext(ctxKeyMappingIndex).(int)
	if !ok || mappingIndex < 0 || mappingIndex >= len(config.ModelProviderMappings) {
		return types.ActionContinue
	}

	mapping := &config.ModelProviderMappings[mappingIndex]
	if mapping.ResponseTransform == nil {
		return types.ActionContinue
	}

	transform := mapping.ResponseTransform
	var transformedBody []byte = body
	var err error

	// 应用转换
	switch transform.Type {
	case "format_transform":
		if transform.FormatTransform != nil {
			transformedBody, err = applyFormatTransform(transform.FormatTransform, body, log)
		}
	case "body_transform":
		if transform.BodyTransform != nil {
			transformedBody, err = applyBodyTransform(transform.BodyTransform, body)
		}
	case "param_transform":
		if transform.ParamTransform != nil {
			transformedBody, err = applyParamTransform(transform.ParamTransform, body)
		}
	default:
		return types.ActionContinue
	}

	if err != nil {
		log.Errorf("failed to apply response transform type %s: %v", transform.Type, err)
		return types.ActionContinue
	}

	// 如果 body 被转换，替换响应 body
	if transformedBody != nil && !bytes.Equal(body, transformedBody) {
		if err := proxywasm.ReplaceHttpResponseBody(transformedBody); err != nil {
			log.Errorf("failed to replace response body: %v", err)
		} else {
			log.Infof("response body transformed")
		}
	}

	return types.ActionContinue
}

// getModelFromRequest 从请求中获取模型名称
func getModelFromRequest(ctx wrapper.HttpContext) string {
	// 优先从 Envoy 属性中获取（由之前的插件设置，如 model-router）
	if requestModel, err := proxywasm.GetProperty([]string{"wasm.requestModel"}); err == nil {
		model := string(requestModel)
		if model != "" {
			log.Debugf("got model from wasm.requestModel: %s", model)
			return model
		}
	}

	// 如果无法获取，返回空字符串
	return ""
}

// applyURLRewrite 应用 URL 重写
func applyURLRewrite(ctx wrapper.HttpContext, config *URLRewriteConfig, originalPath string) (string, error) {
	newPath := originalPath

	if config.Path != "" {
		newPath = config.Path
	} else if config.FromPattern != "" && config.ToPattern != "" {
		// 简单的模式替换（可以用正则扩展）
		newPath = strings.ReplaceAll(originalPath, config.FromPattern, config.ToPattern)
	}

	// 保留查询参数
	if idx := strings.Index(originalPath, "?"); idx >= 0 {
		if idx2 := strings.Index(newPath, "?"); idx2 < 0 {
			newPath += originalPath[idx:]
		}
	}

	// 替换请求路径
	if err := proxywasm.ReplaceHttpRequestHeader(":path", newPath); err != nil {
		return newPath, fmt.Errorf("failed to replace request path: %w", err)
	}

	// 更新 ctxKeyApiName 上下文，让下游插件（如 ai-proxy）获取重写后的 API 名称
	// 将路径转换为 ApiName 格式（格式：{vendor}/{version}/{apitype}）
	apiName := convertPathToApiName(newPath)
	ctx.SetContext(ctxKeyApiName, apiName)

	return newPath, nil
}

// convertPathToApiName 将 URL 路径转换为 ApiName 格式
// 例如：/v1/images/generations -> openai/v1/imagegeneration
// 例如：/v1/images/edits -> openai/v1/imageedit
func convertPathToApiName(path string) string {
	// 移除查询参数
	if idx := strings.Index(path, "?"); idx >= 0 {
		path = path[:idx]
	}

	// 移除开头的 /
	if strings.HasPrefix(path, "/") {
		path = path[1:]
	}

	// 转换为小写
	path = strings.ToLower(path)

	// 映射常见路径到标准 ApiName
	// 参考ai-proxy中的标准命名规范
	pathMappings := map[string]string{
		"v1/images/generations": "openai/v1/imagegeneration",
		"v1/images/edits":       "openai/v1/imageedit",
		"v1/images/variations":  "openai/v1/imagevariation",
		"v1/completions":        "openai/v1/completions",
		"v1/chat/completions":   "openai/v1/chatcompletions",
		"v1/embeddings":         "openai/v1/embeddings",
		"v1/audio/speech":       "openai/v1/audiospeech",
		"v1/files":              "openai/v1/files",
		"v1/models":             "openai/v1/models",
	}

	// 检查是否有映射
	if apiName, ok := pathMappings[path]; ok {
		return apiName
	}

	// 如果没有映射，使用通用格式
	// 将 / 替换为空字符串，然后格式化
	normalizedPath := strings.ReplaceAll(path, "/", "")
	return "openai/" + normalizedPath
}

// applyHeaderTransform 应用头转换
func applyHeaderTransform(config *HeaderTransformConfig, isRequest bool) error {
	// 添加头
	for key, value := range config.AddHeaders {
		var err error
		if isRequest {
			err = proxywasm.AddHttpRequestHeader(key, value)
		} else {
			err = proxywasm.AddHttpResponseHeader(key, value)
		}
		if err != nil {
			return fmt.Errorf("failed to add header %s: %w", key, err)
		}
	}

	// 移除头
	for _, key := range config.RemoveHeaders {
		var err error
		if isRequest {
			err = proxywasm.RemoveHttpRequestHeader(key)
		} else {
			err = proxywasm.RemoveHttpResponseHeader(key)
		}
		if err != nil {
			return fmt.Errorf("failed to remove header %s: %w", key, err)
		}
	}

	// 重命名头
	for oldKey, newKey := range config.RenameHeaders {
		var value string
		var err error
		if isRequest {
			value, err = proxywasm.GetHttpRequestHeader(oldKey)
		} else {
			value, err = proxywasm.GetHttpResponseHeader(oldKey)
		}
		if err == nil {
			if isRequest {
				_ = proxywasm.RemoveHttpRequestHeader(oldKey)
				err = proxywasm.AddHttpRequestHeader(newKey, value)
			} else {
				_ = proxywasm.RemoveHttpResponseHeader(oldKey)
				err = proxywasm.AddHttpResponseHeader(newKey, value)
			}
			if err != nil {
				return fmt.Errorf("failed to rename header %s to %s: %w", oldKey, newKey, err)
			}
		}
	}

	return nil
}

// applyParamTransform 应用参数转换
func applyParamTransform(config *ParamTransformConfig, body []byte) ([]byte, error) {
	if len(body) == 0 || config == nil {
		return body, nil
	}

	result := string(body)
	var err error

	// 重命名参数
	for oldKey, newKey := range config.RenameMap {
		result, err = sjson.Set(result, newKey, gjson.Get(result, oldKey).Value())
		if err != nil {
			return body, fmt.Errorf("failed to rename param %s to %s: %w", oldKey, newKey, err)
		}
		result, err = sjson.Delete(result, oldKey)
		if err != nil {
			return body, fmt.Errorf("failed to delete old param %s: %w", oldKey, err)
		}
	}

	// 添加参数
	for key, value := range config.AddParams {
		result, err = sjson.Set(result, key, value)
		if err != nil {
			return body, fmt.Errorf("failed to add param %s: %w", key, err)
		}
	}

	// 删除参数
	for _, key := range config.RemoveParams {
		result, err = sjson.Delete(result, key)
		if err != nil {
			return body, fmt.Errorf("failed to delete param %s: %w", key, err)
		}
	}

	return []byte(result), nil
}

// applyBodyTransform 应用 body 转换
func applyBodyTransform(config *BodyTransformConfig, body []byte) ([]byte, error) {
	if len(body) == 0 || config == nil {
		return body, nil
	}

	result := string(body)
	var err error

	// 设置值
	for key, value := range config.SetValues {
		result, err = sjson.Set(result, key, value)
		if err != nil {
			return body, fmt.Errorf("failed to set value for %s: %w", key, err)
		}
	}

	// 删除路径
	for _, path := range config.RemovePaths {
		result, err = sjson.Delete(result, path)
		if err != nil {
			return body, fmt.Errorf("failed to delete path %s: %w", path, err)
		}
	}

	return []byte(result), nil
}

// applyFormatTransform 应用格式转换（支持 multipart）
func applyFormatTransform(config *FormatTransformConfig, body []byte, log log.Log) ([]byte, error) {
	if config == nil || config.TargetFormat == "" {
		return body, nil
	}

	switch config.TargetFormat {
	case "multipart":
		if config.MultipartConfig == nil {
			return body, nil
		}
		return convertToMultipart(body, config.MultipartConfig, log)
	case "json":
		return convertFromMultipart(body, log)
	default:
		return body, nil
	}
}

// convertToMultipart 将 JSON body 转换为 multipart/form-data 格式
// 支持 image 数组字段处理，Base64 和 URL 类型都会转换为文件流
func convertToMultipart(body []byte, config *MultipartTransformConfig, log log.Log) ([]byte, error) {
	if len(body) == 0 {
		return body, nil
	}

	// 创建 multipart buffer
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	defer func() {
		if writer != nil {
			_ = writer.Close()
		}
	}()

	// 解析 JSON
	jsonData := gjson.ParseBytes(body)

	// 遍历所有字段
	jsonData.ForEach(func(key, value gjson.Result) bool {
		originalFieldName := key.String()

		// 检查是否在字段映射中（需要转换为文件流的字段）
		if mappedName, ok := config.FieldMapping[originalFieldName]; ok {
			// 这是一个需要转换为文件流的字段
			fieldName := mappedName

			switch value.Type {
			case gjson.JSON:
				// JSON 类型：如果是数组类型且是 image 字段，处理数组中的每个图片
				if value.IsArray() {
					if originalFieldName == "image" || strings.HasPrefix(originalFieldName, "image") {
						// 处理 image 数组，每个元素转换为独立的文件流
						if err := handleImageArray(writer, fieldName, value, log); err != nil {
							log.Errorf("failed to handle image array for field %s: %v", fieldName, err)
						}
					} else {
						// 普通数组，转换为 JSON 字符串
						_ = writer.WriteField(fieldName, value.String())
					}
				} else {
					// 普通对象，转换为 JSON 字符串
					_ = writer.WriteField(fieldName, value.String())
				}

			default:
				// 其他类型（字符串、数字、布尔值等）转换为字符串
				_ = writer.WriteField(fieldName, value.String())
			}

			// 字段已转换并添加到 multipart，不需要再作为 form 字段添加
			return true
		}

		// 不是 fieldMapping 中的字段，作为普通 form 字段保留
		switch value.Type {
		case gjson.String:
			_ = writer.WriteField(originalFieldName, value.String())
		case gjson.JSON:
			_ = writer.WriteField(originalFieldName, value.String())
		default:
			_ = writer.WriteField(originalFieldName, value.String())
		}

		return true
	})

	// 添加额外的字段
	for key, value := range config.AddFields {
		_ = writer.WriteField(key, value)
	}

	// 在关闭前获取 content-type（必须在 Close 之前调用）
	contentType := writer.FormDataContentType()

	// 关闭 writer
	_ = writer.Close()
	writer = nil // 避免重复关闭

	// 返回 multipart body
	multipartBody := buf.Bytes()

	// 替换 Content-Type 头
	_ = proxywasm.ReplaceHttpRequestHeader("Content-Type", contentType)

	log.Debugf("converted to multipart, content-type: %s", contentType)
	return multipartBody, nil
}

// handleImageArray 处理图片数组
// 数组中的每个元素可以是 Base64 字符串或 URL
func handleImageArray(writer *multipart.Writer, fieldName string, images gjson.Result, log log.Log) error {
	// 检查是否为数组
	if !images.Exists() {
		return fmt.Errorf("images field does not exist")
	}

	// 获取数组元素
	var array []gjson.Result
	if images.Type == gjson.JSON {
		array = images.Array()
	} else if images.IsArray() {
		array = images.Array()
	} else {
		return fmt.Errorf("images is not an array")
	}

	if len(array) == 0 {
		return fmt.Errorf("images array is empty")
	}

	// 遍历数组中的每个图片
	for i, image := range array {
		if image.Type != gjson.String {
			log.Warnf("image array element %d is not a string, skipping", i)
			continue
		}

		imageStr := image.String()
		arrayFieldName := fmt.Sprintf("%s[%d]", fieldName, i)

		// 处理每个图片元素
		if isBase64Image(imageStr) {
			// Base64 编码的图片，解码并创建文件流
			imageData, err := base64.StdEncoding.DecodeString(extractBase64Data(imageStr))
			if err != nil {
				log.Errorf("failed to decode base64 image for %s: %v", arrayFieldName, err)
				// 失败时写入原始字符串
				_ = writer.WriteField(arrayFieldName, imageStr)
				continue
			}

			// 从 data URI 中提取文件扩展名
			ext := ".png"
			if idx := strings.Index(imageStr, "data:image/"); idx >= 0 {
				imagePrefix := imageStr[idx+11:]
				if semicolonIdx := strings.Index(imagePrefix, ";"); semicolonIdx > 0 && semicolonIdx < len(imagePrefix) {
					ext = "." + imagePrefix[:semicolonIdx]
				}
			}

			// 创建 form 文件字段
			part, err := writer.CreateFormFile(arrayFieldName, arrayFieldName+ext)
			if err != nil {
				log.Errorf("failed to create form file for %s: %v", arrayFieldName, err)
				continue
			}

			// 写入图片数据
			if _, err = part.Write(imageData); err != nil {
				log.Errorf("failed to write image data for %s: %v", arrayFieldName, err)
			}

			log.Debugf("processed base64 image for field %s, size: %d bytes", arrayFieldName, len(imageData))
		} else if isURL(imageStr) {
			// URL 类型的图片，下载并转换为文件流
			imageData, ext, err := downloadImageFromURL(imageStr, log)
			if err != nil {
				log.Errorf("failed to download image from URL for %s: %v", arrayFieldName, err)
				// 失败时写入原始 URL
				_ = writer.WriteField(arrayFieldName, imageStr)
				continue
			}

			// 创建 form 文件字段
			part, err := writer.CreateFormFile(arrayFieldName, arrayFieldName+ext)
			if err != nil {
				log.Errorf("failed to create form file for %s: %v", arrayFieldName, err)
				continue
			}

			// 写入图片数据
			if _, err = part.Write(imageData); err != nil {
				log.Errorf("failed to write image data for %s: %v", arrayFieldName, err)
			}

			log.Debugf("processed URL image for field %s, size: %d bytes", arrayFieldName, len(imageData))
		} else {
			// 既不是 Base64 也不是 URL，作为普通字符串处理
			_ = writer.WriteField(arrayFieldName, imageStr)
		}
	}

	log.Debugf("processed image array for field %s, count: %d", fieldName, len(array))
	return nil
}

// convertFromMultipart 将 multipart/form-data 转换为 JSON
func convertFromMultipart(body []byte, log log.Log) ([]byte, error) {
	// 这里需要解析 multipart 格式并转换为 JSON
	// 由于解析 multipart 比较复杂，这里提供一个简化实现
	// 实际生产中需要更完整的 multipart 解析逻辑

	log.Warnf("multipart to JSON conversion is simplified, full implementation needed")
	return body, nil
}

// isBase64Image 检查字符串是否是 Base64 编码的图片
func isBase64Image(s string) bool {
	return strings.HasPrefix(s, "data:image/") && strings.Contains(s, "base64,")
}

// isURL 检查字符串是否是有效的 URL
func isURL(s string) bool {
	// 简单的 URL 检查：必须包含 http:// 或 https://
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

// extractBase64Data 从 data URI 中提取 Base64 数据
func extractBase64Data(s string) string {
	if idx := strings.Index(s, "base64,"); idx >= 0 {
		return s[idx+7:]
	}
	return s
}

// downloadImageFromURL 从 URL 下载图片并返回图片数据和文件扩展名
// 注意：在 WASM 环境中，同步 HTTP 请求会导致死锁，因此此函数降级为错误处理
// 如需使用 URL 图片，请在客户端转换为 Base64 格式
func downloadImageFromURL(imageURL string, log log.Log) ([]byte, string, error) {
	log.Warnf("URL image download is not supported in WASM environment to avoid deadlock. URL: %s. Please use Base64 format instead.", imageURL)
	return nil, "", fmt.Errorf("URL download not supported in WASM environment")
}

// detectImageExtension 根据 Content-Type 检测图片扩展名
func detectImageExtension(contentType string) string {
	// 默认扩展名
	defaultExt := ".jpg"

	// 根据 Content-Type 映射扩展名
	extMap := map[string]string{
		"image/jpeg":      ".jpg",
		"image/jpg":       ".jpg",
		"image/png":       ".png",
		"image/gif":       ".gif",
		"image/webp":      ".webp",
		"image/bmp":       ".bmp",
		"image/tiff":      ".tiff",
		"image/svg+xml":   ".svg",
		"image/x-icon":    ".ico",
		"application/pdf": ".pdf",
	}

	// 移除可能的字符集参数（如 image/jpeg; charset=utf-8）
	if idx := strings.Index(contentType, ";"); idx > 0 {
		contentType = strings.TrimSpace(contentType[:idx])
	}

	// 查找对应的扩展名
	if ext, ok := extMap[contentType]; ok {
		return ext
	}

	// 尝试通过 Content-Type 的最后一部分判断
	if parts := strings.Split(contentType, "/"); len(parts) == 2 {
		return "." + parts[1]
	}

	return defaultExt
}
