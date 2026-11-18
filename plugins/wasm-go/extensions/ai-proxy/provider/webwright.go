package provider

import (
	"errors"
	"net/http"
	"strings"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-proxy/util"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

// webwrightProvider is the provider for the WebWright service.

const (
	webwrightDomain = "webwright.ai"
)

type webwrightProviderInitializer struct{}

func (m *webwrightProviderInitializer) ValidateConfig(config *ProviderConfig) error {
	if config.apiTokens == nil || len(config.apiTokens) == 0 {
		return errors.New("no apiToken found in provider config")
	}
	return nil
}

func (m *webwrightProviderInitializer) DefaultCapabilities() map[string]string {
	return map[string]string{
		string(ApiNameChatCompletion): PathOpenAIChatCompletions,
		string(ApiNameModels):         PathOpenAIModels,
	}
}

func (m *webwrightProviderInitializer) CreateProvider(config ProviderConfig) (Provider, error) {
	config.setDefaultCapabilities(m.DefaultCapabilities())
	return &webwrightProvider{
		config:       config,
		contextCache: createContextCache(&config),
	}, nil
}

type webwrightProvider struct {
	config       ProviderConfig
	contextCache *contextCache
}

func (w *webwrightProvider) GetProviderType() string {
	return providerTypeWebWright
}

func (w *webwrightProvider) OnRequestHeaders(ctx wrapper.HttpContext, apiName ApiName) error {
	w.config.handleRequestHeaders(w, ctx, apiName)
	return nil
}

func (w *webwrightProvider) OnRequestBody(ctx wrapper.HttpContext, apiName ApiName, body []byte) (types.Action, error) {
	if !w.config.isSupportedAPI(apiName) {
		return types.ActionContinue, errUnsupportedApiName
	}
	return w.config.handleRequestBody(w, w.contextCache, ctx, apiName, body)
}

func (w *webwrightProvider) OnStreamingResponseBody(ctx wrapper.HttpContext, apiName ApiName, chunk []byte, isLastChunk bool) ([]byte, error) {
	if len(chunk) == 0 {
		return nil, nil
	}
	// WebWright chat completion streams use the OpenAI-compatible SSE format,
	// so no provider-specific conversion is required here.
	return chunk, nil
}

func (w *webwrightProvider) TransformRequestHeaders(ctx wrapper.HttpContext, apiName ApiName, headers http.Header) {
	util.OverwriteRequestPathHeaderByCapability(headers, string(apiName), w.config.capabilities)
	util.OverwriteRequestHostHeader(headers, webwrightDomain)
	util.OverwriteRequestAuthorizationHeader(headers, "Bearer "+w.config.GetApiTokenInUse(ctx))
	headers.Del("Content-Length")
}

func (w *webwrightProvider) GetApiName(path string) ApiName {
	if strings.Contains(path, PathOpenAIChatCompletions) {
		return ApiNameChatCompletion
	}
	if strings.Contains(path, PathOpenAIModels) {
		return ApiNameModels
	}
	return ""
}
