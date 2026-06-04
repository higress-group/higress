package provider

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-proxy/util"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

type azureServiceUrlType int

const (
	pathAzurePrefix           = "/openai"
	pathAzureOpenAIV1         = "/openai/v1"
	pathAzureModelPlaceholder = "{model}"
	pathAzureWithModelPrefix  = "/openai/deployments/" + pathAzureModelPlaceholder
	queryAzureApiVersion      = "api-version"
)

const (
	azureServiceUrlTypeFull azureServiceUrlType = iota
	azureServiceUrlTypeWithDeployment
	azureServiceUrlTypeDomainOnly
	azureServiceUrlTypeOpenAIV1Base
)

var (
	azureModelIrrelevantApis = map[ApiName]bool{
		ApiNameModels:              true,
		ApiNameBatches:             true,
		ApiNameRetrieveBatch:       true,
		ApiNameCancelBatch:         true,
		ApiNameFiles:               true,
		ApiNameRetrieveFile:        true,
		ApiNameRetrieveFileContent: true,
		ApiNameResponses:           true,
	}
	regexAzureModelWithPath = regexp.MustCompile("/openai/deployments/(.+?)(?:/(.*)|$)")
)

// azureProvider is the provider for Azure OpenAI service.
type azureProviderInitializer struct {
}

func (m *azureProviderInitializer) DefaultCapabilities() map[string]string {
	var capabilities = map[string]string{}
	for k, v := range (&openaiProviderInitializer{}).DefaultCapabilities() {
		if !strings.HasPrefix(v, PathOpenAIPrefix) {
			log.Warnf("azureProviderInitializer: capability %s has an unexpected path %s, skipping", k, v)
			continue
		}
		path := strings.TrimPrefix(v, PathOpenAIPrefix)
		if azureModelIrrelevantApis[ApiName(k)] {
			path = pathAzurePrefix + path
		} else {
			path = pathAzureWithModelPrefix + path
		}
		capabilities[k] = path
		log.Debugf("azureProviderInitializer: capability %s -> %s", k, path)
	}
	return capabilities
}

// DefaultOpenAIV1Capabilities 生成 Azure OpenAI v1 base_url 模式下的 capability path。
// 输入约束：basePath 必须是 Azure v1 base path，通常为 /openai/v1，调用方应先完成 URL mode 判定。
// 输出语义：把 OpenAI 标准 /v1/... 能力路径平移到 Azure /openai/v1/...，不引入 deployment 和 api-version。
// 边界场景：若后续 Azure v1 base path 发生变化，只需要同步调用方传入的 basePath 与 spec 契约。
func (m *azureProviderInitializer) DefaultOpenAIV1Capabilities(basePath string) map[string]string {
	return defaultOpenAIV1Capabilities(basePath, (&openaiProviderInitializer{}).DefaultCapabilities())
}

// defaultOpenAIV1Capabilities 将 OpenAI capability map 转换为 Azure OpenAI v1 path map。
// 输入约束：openAICapabilities 中的 path 应以 /v1 开头；异常 path 会被跳过，避免生成不可用上游路径。
// 输出语义：返回的 path 统一位于 basePath 下，不包含 deployment 占位符和日期型 api-version。
// 边界场景：保留异常 capability 跳过逻辑，防止未来 OpenAI capability 定义变更时污染 Azure v1 默认映射。
func defaultOpenAIV1Capabilities(basePath string, openAICapabilities map[string]string) map[string]string {
	var capabilities = map[string]string{}
	for k, v := range openAICapabilities {
		if !strings.HasPrefix(v, PathOpenAIPrefix) {
			log.Warnf("azureProviderInitializer: capability %s has an unexpected path %s, skipping", k, v)
			continue
		}
		capabilities[k] = path.Join(basePath, strings.TrimPrefix(v, PathOpenAIPrefix))
		log.Debugf("azureProviderInitializer: v1 capability %s -> %s", k, capabilities[k])
	}
	return capabilities
}

// ValidateConfig 校验 Azure OpenAI provider 配置是否满足启动条件。
// 输入约束：azureServiceUrl 必须是合法 URL，apiTokens 至少包含一个可用 token。
// 输出语义：返回 nil 表示插件可启动；返回 error 时会阻止当前 provider 配置生效。
// 边界场景：/openai/v1 新版路径不要求日期型 api-version，legacy deployment 或 domain-only 模式仍要求非空 api-version。
func (m *azureProviderInitializer) ValidateConfig(config *ProviderConfig) error {
	if config.azureServiceUrl == "" {
		return errors.New("missing azureServiceUrl in provider config")
	}
	if azureServiceUrl, err := url.Parse(config.azureServiceUrl); err != nil {
		return fmt.Errorf("invalid azureServiceUrl: %w", err)
	} else if err := validateAzureServiceURLAPIVersion(azureServiceUrl, config.azureServiceUrl); err != nil {
		return err
	}
	if config.apiTokens == nil || len(config.apiTokens) == 0 {
		return errors.New("no apiToken found in provider config")
	}
	return nil
}

// validateAzureServiceURLAPIVersion 按 Azure OpenAI URL path mode 校验 api-version。
// 输入约束：serviceURL 应来自 azureServiceUrl 的解析结果，rawServiceURL 仅用于错误信息回显。
// 输出语义：v1 路径直接通过；legacy 路径只有携带非空 api-version 时通过。
// 边界场景：/openai/v10、/openai/deployments/... 和仅域名 URL 都不属于 v1 模式。
func validateAzureServiceURLAPIVersion(serviceURL *url.URL, rawServiceURL string) error {
	if isAzureOpenAIV1Path(serviceURL.Path) {
		// APIGO-CONTRACT: azure-service-url-api-version-optional
		// 新版 Azure OpenAI v1 API 以 /openai/v1 作为 base_url，产品机制不再要求日期型 api-version；
		// 若微软 v1 API 路径或版本机制再次变化，需要同步更新 apigo 控制面与 ai-proxy 插件判定。
		return nil
	}
	if serviceURL.Query().Get(queryAzureApiVersion) == "" {
		// APIGO-CONTRACT: azure-service-url-api-version-optional
		// legacy deployment/domain-only 代理模式仍依赖 api-version 生成旧版 Azure data-plane 请求；
		// 只有确认旧版 /openai/deployments 路径也取消版本参数要求时，才能放宽该分支。
		return fmt.Errorf("missing %s query parameter in azureServiceUrl: %s", queryAzureApiVersion, rawServiceURL)
	}
	return nil
}

// isAzureOpenAIV1Path 判断 Azure OpenAI 服务 URL 是否使用新版 /openai/v1 path mode。
// 输入约束：rawPath 可为空或带尾随斜杠，函数内部会用 path.Clean 规整。
// 输出语义：仅 /openai/v1 及其子路径返回 true，避免把 /openai/v10 误判为 v1。
// 边界场景：空路径会被规整为 "."，必须返回 false，让 domain-only legacy 模式继续要求 api-version。
func isAzureOpenAIV1Path(rawPath string) bool {
	cleanPath := path.Clean(rawPath)
	return cleanPath == pathAzureOpenAIV1 || strings.HasPrefix(cleanPath, pathAzureOpenAIV1+"/")
}

// isAzureOpenAIV1BasePath 判断 URL 是否是 Azure OpenAI v1 的 base_url，而不是具体接口完整路径。
// 输入约束：rawPath 来自 azureServiceUrl.Path，可为空或带尾随斜杠。
// 输出语义：仅精确 /openai/v1 返回 true；/openai/v1/chat/completions 仍保留 full path 语义。
// 边界场景：/openai/v10 或 /openai/v1beta 都不能被误判为 v1 base_url。
func isAzureOpenAIV1BasePath(rawPath string) bool {
	return path.Clean(rawPath) == pathAzureOpenAIV1
}

// appendAzureServiceURLRawQuery 将 azureServiceUrl 中的原始 query 拼接到目标路径。
// 输入约束：basePath 可以已经包含 query 或以 ? 结尾；rawQuery 为空时不得追加任何分隔符。
// 输出语义：返回可直接写入 :path 的完整路径，并保持已有 query 与 azureServiceUrl query 的相对顺序。
// 边界场景：Azure v1 URL 常常没有 query，此时必须避免生成尾随空问号。
func appendAzureServiceURLRawQuery(basePath, rawQuery string) string {
	if rawQuery == "" {
		return basePath
	}
	if !strings.Contains(basePath, "?") {
		return basePath + "?" + rawQuery
	}
	if strings.HasSuffix(basePath, "?") {
		return basePath + rawQuery
	}
	return basePath + "&" + rawQuery
}

// CreateProvider 基于已校验的 Azure OpenAI 配置创建运行时 provider。
// 输入约束：config.azureServiceUrl 必须可解析；ValidateConfig 会在正常启动路径提前检查 api-version 与 token。
// 输出语义：返回的 provider 会记录 URL 类型、默认模型、原始 query 和上下文缓存，用于请求阶段改写 Host/Path/Header。
// 边界场景：/openai/deployments/{deployment} 可提取默认 deployment；/openai/v1 作为 v1 base_url 映射 OpenAI path，不补 api-version。
func (m *azureProviderInitializer) CreateProvider(config ProviderConfig) (Provider, error) {
	var serviceUrl *url.URL
	if u, err := url.Parse(config.azureServiceUrl); err != nil {
		return nil, fmt.Errorf("invalid azureServiceUrl: %w", err)
	} else {
		serviceUrl = u
	}

	modelSubMatch := regexAzureModelWithPath.FindStringSubmatch(serviceUrl.Path)
	defaultModel := "placeholder"
	var serviceUrlType azureServiceUrlType
	if modelSubMatch != nil {
		defaultModel = modelSubMatch[1]
		if modelSubMatch[2] != "" {
			serviceUrlType = azureServiceUrlTypeFull
		} else {
			serviceUrlType = azureServiceUrlTypeWithDeployment
		}
		log.Debugf("azureProvider: found default model from serviceUrl: %s", defaultModel)
	} else if isAzureOpenAIV1BasePath(serviceUrl.Path) {
		// APIGO-CONTRACT: azure-service-url-api-version-optional
		// /openai/v1 是 Azure OpenAI v1 的 base_url，运行时需要继续拼接 OpenAI capability path；
		// 若将其当作 full path，会把 /v1/chat/completions 错误覆盖成 /openai/v1。
		serviceUrlType = azureServiceUrlTypeOpenAIV1Base
		log.Debugf("azureProvider: using Azure OpenAI v1 base path: %s", serviceUrl.Path)
	} else {
		// If path doesn't match the /openai/deployments pattern,
		// check if it's a custom full path or domain only
		if serviceUrl.Path != "" && serviceUrl.Path != "/" {
			serviceUrlType = azureServiceUrlTypeFull
			log.Debugf("azureProvider: using custom full path: %s", serviceUrl.Path)
		} else {
			serviceUrlType = azureServiceUrlTypeDomainOnly
			log.Debugf("azureProvider: no default model found in serviceUrl")
		}
	}
	log.Debugf("azureProvider: serviceUrlType=%d", serviceUrlType)

	if serviceUrlType == azureServiceUrlTypeOpenAIV1Base {
		config.setDefaultCapabilities(m.DefaultOpenAIV1Capabilities(pathAzureOpenAIV1))
	} else {
		config.setDefaultCapabilities(m.DefaultCapabilities())
	}
	apiVersion := serviceUrl.Query().Get(queryAzureApiVersion)
	log.Debugf("azureProvider: using %s: %s", queryAzureApiVersion, apiVersion)
	return &azureProvider{
		config:             config,
		serviceUrl:         serviceUrl,
		serviceUrlType:     serviceUrlType,
		serviceUrlFullPath: appendAzureServiceURLRawQuery(serviceUrl.Path, serviceUrl.RawQuery),
		apiVersion:         apiVersion,
		defaultModel:       defaultModel,
		contextCache:       createContextCache(&config),
	}, nil
}

type azureProvider struct {
	config ProviderConfig

	contextCache       *contextCache
	serviceUrl         *url.URL
	serviceUrlFullPath string
	serviceUrlType     azureServiceUrlType
	apiVersion         string
	defaultModel       string
}

func (m *azureProvider) GetProviderType() string {
	return providerTypeAzure
}

func (m *azureProvider) OnRequestHeaders(ctx wrapper.HttpContext, apiName ApiName) error {
	m.config.handleRequestHeaders(m, ctx, apiName)
	return nil
}

func (m *azureProvider) OnRequestBody(ctx wrapper.HttpContext, apiName ApiName, body []byte) (types.Action, error) {
	return m.config.handleRequestBody(m, m.contextCache, ctx, apiName, body)
}

func isAzureMultipartImageRequest(apiName ApiName, contentType string) bool {
	if apiName != ApiNameImageEdit && apiName != ApiNameImageVariation {
		return false
	}
	return isMultipartFormData(contentType)
}

func (m *azureProvider) TransformRequestBody(ctx wrapper.HttpContext, apiName ApiName, body []byte) (transformedBody []byte, err error) {
	transformedBody = body
	err = nil

	contentType, _ := proxywasm.GetHttpRequestHeader(util.HeaderContentType)
	isMultipartImageRequest := isAzureMultipartImageRequest(apiName, contentType)

	transformedBody, err = m.config.defaultTransformRequestBody(ctx, apiName, body)
	if isMultipartImageRequest {
		if err != nil {
			log.Debugf("[azure multipart] body transform failed: api=%s, err=%v", apiName, err)
		} else {
			log.Debugf("[azure multipart] body transformed: api=%s, originalModel=%s, mappedModel=%s, bodyBytes=%d->%d",
				apiName,
				ctx.GetStringContext(ctxKeyOriginalRequestModel, ""),
				ctx.GetStringContext(ctxKeyFinalRequestModel, ""),
				len(body),
				len(transformedBody),
			)
		}
	}
	if err != nil {
		return
	}

	// This must be called after the body is transformed, because it uses the model from the context filled by that call.
	if path := m.transformRequestPath(ctx, apiName); path != "" {
		if isMultipartImageRequest {
			log.Debugf("[azure multipart] body path overwrite: api=%s, path=%s, modelInContext=%s",
				apiName, path, ctx.GetStringContext(ctxKeyFinalRequestModel, ""))
		}
		err = util.OverwriteRequestPath(path)
		if err == nil {
			log.Debugf("azureProvider: overwrite request path to %s succeeded", path)
		} else {
			log.Errorf("azureProvider: overwrite request path to %s failed: %v", path, err)
		}
	}

	return
}

// transformRequestPath 根据插件协议和 Azure service URL 类型生成最终上游请求路径。
// 输入约束：ctx 中可能已经写入最终模型名；apiName 必须来自 ai-proxy 能识别的 API 枚举。
// 输出语义：original 协议返回空字符串表示不覆盖 path；其他协议返回包含必要 Azure path/query 的上游路径。
// 边界场景：v1 完整路径不应追加空 query，legacy domain-only 模式仍会把 api-version 追加到 capability 映射后的路径。
func (m *azureProvider) transformRequestPath(ctx wrapper.HttpContext, apiName ApiName) string {
	// When using original protocol, don't overwrite the path.
	// This ensures basePathHandling works correctly even in TransformRequestBody stage.
	if m.config.IsOriginal() {
		return ""
	}

	originalPath := util.GetOriginalRequestPath()

	if m.serviceUrlType == azureServiceUrlTypeFull {
		log.Debugf("azureProvider: use configured path %s", m.serviceUrlFullPath)
		return m.serviceUrlFullPath
	}

	log.Debugf("azureProvider: original request path: %s", originalPath)
	path := util.MapRequestPathByCapability(string(apiName), originalPath, m.config.capabilities)
	log.Debugf("azureProvider: path: %s", path)
	if strings.Contains(path, pathAzureModelPlaceholder) {
		log.Debugf("azureProvider: path contains placeholder: %s", path)
		var model string
		if m.serviceUrlType == azureServiceUrlTypeWithDeployment {
			model = m.defaultModel
		} else {
			model = ctx.GetStringContext(ctxKeyFinalRequestModel, "")
			log.Debugf("azureProvider: model from context: %s", model)
			if model == "" {
				model = m.defaultModel
				log.Debugf("azureProvider: use default model: %s", model)
			}
		}
		path = strings.ReplaceAll(path, pathAzureModelPlaceholder, model)
		log.Debugf("azureProvider: model replaced path: %s", path)
	}
	path = appendAzureServiceURLRawQuery(path, m.serviceUrl.RawQuery)
	log.Debugf("azureProvider: final path: %s", path)

	return path
}

func (m *azureProvider) TransformRequestHeaders(ctx wrapper.HttpContext, apiName ApiName, headers http.Header) {
	contentType := headers.Get(util.HeaderContentType)
	isMultipartImageRequest := isAzureMultipartImageRequest(apiName, contentType)

	// We need to overwrite the request path in the request headers stage,
	// because for some APIs, we don't read the request body and the path is model irrelevant.
	if overwrittenPath := m.transformRequestPath(ctx, apiName); overwrittenPath != "" {
		util.OverwriteRequestPathHeader(headers, overwrittenPath)
		if isMultipartImageRequest {
			log.Debugf("[azure multipart] header path overwrite: api=%s, path=%s, modelInContext=%s",
				apiName, overwrittenPath, ctx.GetStringContext(ctxKeyFinalRequestModel, ""))
		}
	}
	util.OverwriteRequestHostHeader(headers, m.serviceUrl.Host)
	headers.Set("api-key", m.config.GetApiTokenInUse(ctx))
	headers.Del("Content-Length")

	supportedAPI := m.config.isSupportedAPI(apiName)
	needProcessBody := m.config.needToProcessRequestBody(apiName)
	if isMultipartImageRequest {
		log.Debugf("[azure multipart] body processing decision: api=%s, supported=%t, needProcessBody=%t",
			apiName, supportedAPI, needProcessBody)
	}

	if !supportedAPI || !needProcessBody {
		// If the API is not supported or there is no need to process the body,
		// we should not read the request body and keep it as it is.
		ctx.DontReadRequestBody()
	}
}
