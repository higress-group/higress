package provider

import (
	"errors"
	"net/http"
	"strings"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-proxy/util"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

// groqProvider is the provider for Groq service.
const (
	groqDomain             = "api.groq.com"
	groqChatCompletionPath = "/openai/v1/chat/completions"
	groqResponsesPath      = "/openai/v1/responses"
)

type groqProviderInitializer struct{}

func (g *groqProviderInitializer) ValidateConfig(config *ProviderConfig) error {
	if config.apiTokens == nil || len(config.apiTokens) == 0 {
		return errors.New("no apiToken found in provider config")
	}
	return nil
}

func (g *groqProviderInitializer) DefaultCapabilities() map[string]string {
	return map[string]string{
		string(ApiNameChatCompletion): groqChatCompletionPath,
		string(ApiNameResponses):      groqResponsesPath,
	}
}

func (g *groqProviderInitializer) CreateProvider(config ProviderConfig) (Provider, error) {
	config.setDefaultCapabilities(g.DefaultCapabilities())
	return &groqProvider{
		config:       config,
		contextCache: createContextCache(&config),
	}, nil
}

type groqProvider struct {
	config       ProviderConfig
	contextCache *contextCache
}

func (g *groqProvider) GetProviderType() string {
	return providerTypeGroq
}

func (g *groqProvider) OnRequestHeaders(ctx wrapper.HttpContext, apiName ApiName) error {
	g.config.handleRequestHeaders(g, ctx, apiName)
	return nil
}

func (g *groqProvider) OnRequestBody(ctx wrapper.HttpContext, apiName ApiName, body []byte) (types.Action, error) {
	if !g.config.isSupportedAPI(apiName) {
		return types.ActionContinue, errUnsupportedApiName
	}
	return g.config.handleRequestBody(g, g.contextCache, ctx, apiName, body)
}

func (g *groqProvider) TransformRequestHeaders(ctx wrapper.HttpContext, apiName ApiName, headers http.Header) {
	util.OverwriteRequestPathHeaderByCapability(headers, string(apiName), g.config.capabilities)
	util.OverwriteRequestHostHeader(headers, groqDomain)
	util.OverwriteRequestAuthorizationHeader(headers, "Bearer "+g.config.GetApiTokenInUse(ctx))
	headers.Del("Content-Length")
}

func (g *groqProvider) GetApiName(path string) ApiName {
	if strings.Contains(path, groqChatCompletionPath) {
		return ApiNameChatCompletion
	}
	if strings.Contains(path, groqResponsesPath) {
		return ApiNameResponses
	}
	return ""
}
