package provider

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
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
	pathAzureModelPlaceholder = "{model}"
	pathAzureWithModelPrefix  = "/openai/deployments/" + pathAzureModelPlaceholder
	queryAzureApiVersion      = "api-version"
)

const (
	azureServiceUrlTypeFull azureServiceUrlType = iota
	azureServiceUrlTypeWithDeployment
	azureServiceUrlTypeDomainOnly
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

func (m *azureProviderInitializer) ValidateConfig(config *ProviderConfig) error {
	if config.azureServiceUrl == "" {
		return errors.New("missing azureServiceUrl in provider config")
	}
	if azureServiceUrl, err := url.Parse(config.azureServiceUrl); err != nil {
		return fmt.Errorf("invalid azureServiceUrl: %w", err)
	} else if !azureServiceUrl.Query().Has(queryAzureApiVersion) {
		return fmt.Errorf("missing %s query parameter in azureServiceUrl: %s", queryAzureApiVersion, config.azureServiceUrl)
	}
	if config.apiTokens == nil || len(config.apiTokens) == 0 {
		return errors.New("no apiToken found in provider config")
	}
	return nil
}

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

	config.setDefaultCapabilities(m.DefaultCapabilities())
	apiVersion := serviceUrl.Query().Get(queryAzureApiVersion)
	log.Debugf("azureProvider: using %s: %s", queryAzureApiVersion, apiVersion)
	return &azureProvider{
		config:             config,
		serviceUrl:         serviceUrl,
		serviceUrlType:     serviceUrlType,
		serviceUrlFullPath: serviceUrl.Path + "?" + serviceUrl.RawQuery,
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
	if !strings.Contains(path, "?") {
		// No query string yet
		path = path + "?" + m.serviceUrl.RawQuery
	} else if strings.HasSuffix(path, "?") {
		// Ends with "?" and has no query parameter
		path = path + m.serviceUrl.RawQuery
	} else {
		// Has other query parameters
		path = path + "&" + m.serviceUrl.RawQuery
	}
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
