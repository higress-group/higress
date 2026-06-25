package provider

import (
	"errors"
	"net/http"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-proxy/util"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

// tokenplanProvider is the provider for tokenplan Ai service.

const (
	defaultTokenPlanDomain                = "api.coding.net"
	defaultTokenPlanAnthropicMessagesPath = "/anthropic/v1/messages"
)

type tokenplanProviderInitializer struct{}

func (m *tokenplanProviderInitializer) ValidateConfig(config *ProviderConfig) error {
	if len(config.apiTokens) == 0 {
		return errors.New("no apiToken found in provider config")
	}
	return nil
}

func (m *tokenplanProviderInitializer) DefaultCapabilities() map[string]string {
	return map[string]string{
		string(ApiNameChatCompletion):    PathOpenAIChatCompletions,
		string(ApiNameModels):            PathOpenAIModels,
		string(ApiNameAnthropicMessages): defaultTokenPlanAnthropicMessagesPath,
	}
}

func (m *tokenplanProviderInitializer) CreateProvider(config ProviderConfig) (Provider, error) {
	config.setDefaultCapabilities(m.DefaultCapabilities())
	return &tokenplanProvider{
		config:       config,
		contextCache: createContextCache(&config),
	}, nil
}

type tokenplanProvider struct {
	config       ProviderConfig
	contextCache *contextCache
}

func (m *tokenplanProvider) GetProviderType() string {
	return providerTypeTokenPlan
}

func (m *tokenplanProvider) OnRequestHeaders(ctx wrapper.HttpContext, apiName ApiName) error {
	m.config.handleRequestHeaders(m, ctx, apiName)
	return nil
}

func (m *tokenplanProvider) OnRequestBody(ctx wrapper.HttpContext, apiName ApiName, body []byte) (types.Action, error) {
	if !m.config.isSupportedAPI(apiName) {
		return types.ActionContinue, errUnsupportedApiName
	}
	return m.config.handleRequestBody(m, m.contextCache, ctx, apiName, body)
}

func (m *tokenplanProvider) TransformRequestHeaders(ctx wrapper.HttpContext, apiName ApiName, headers http.Header) {
	util.OverwriteRequestPathHeaderByCapability(headers, string(apiName), m.config.capabilities)
	util.OverwriteRequestHostHeader(headers, defaultTokenPlanDomain)
	util.OverwriteRequestAuthorizationHeader(headers, "Bearer "+m.config.GetApiTokenInUse(ctx))
	headers.Del("Content-Length")
}
