package provider

import (
	"errors"
	"net/http"
	"strings"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-proxy/util"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

// galadrielProvider is the provider for Galadriel service.

const (
	galadrielDomain = "api.galadriel.com"
)

type galadrielProviderInitializer struct{}

func (m *galadrielProviderInitializer) ValidateConfig(config *ProviderConfig) error {
	if config.apiTokens == nil || len(config.apiTokens) == 0 {
		return errors.New("no apiToken found in provider config")
	}
	return nil
}

func (m *galadrielProviderInitializer) DefaultCapabilities() map[string]string {
	return map[string]string{
		string(ApiNameChatCompletion): PathOpenAIChatCompletions,
		string(ApiNameModels):         PathOpenAIModels,
	}
}

func (m *galadrielProviderInitializer) CreateProvider(config ProviderConfig) (Provider, error) {
	config.setDefaultCapabilities(m.DefaultCapabilities())
	return &galadrielProvider{
		config:       config,
		contextCache: createContextCache(&config),
	}, nil
}

type galadrielProvider struct {
	config       ProviderConfig
	contextCache *contextCache
}

func (g *galadrielProvider) GetProviderType() string {
	return providerTypeGaladriel
}

func (g *galadrielProvider) OnRequestHeaders(ctx wrapper.HttpContext, apiName ApiName) error {
	g.config.handleRequestHeaders(g, ctx, apiName)
	return nil
}

func (g *galadrielProvider) OnRequestBody(ctx wrapper.HttpContext, apiName ApiName, body []byte) (types.Action, error) {
	if !g.config.isSupportedAPI(apiName) {
		return types.ActionContinue, errUnsupportedApiName
	}
	return g.config.handleRequestBody(g, g.contextCache, ctx, apiName, body)
}

func (g *galadrielProvider) TransformRequestHeaders(ctx wrapper.HttpContext, apiName ApiName, headers http.Header) {
	util.OverwriteRequestPathHeaderByCapability(headers, string(apiName), g.config.capabilities)
	util.OverwriteRequestHostHeader(headers, galadrielDomain)
	util.OverwriteRequestAuthorizationHeader(headers, "Bearer "+g.config.GetApiTokenInUse(ctx))
	headers.Del("Content-Length")
}

func (g *galadrielProvider) GetApiName(path string) ApiName {
	if strings.Contains(path, PathOpenAIChatCompletions) {
		return ApiNameChatCompletion
	}
	if strings.Contains(path, PathOpenAIModels) {
		return ApiNameModels
	}
	return ""
}
