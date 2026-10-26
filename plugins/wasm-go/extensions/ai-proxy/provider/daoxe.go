package provider

import (
	"errors"
	"net/http"
	"strings"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-proxy/util"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

// daoxeProvider is the provider for the DaoXE service (https://daoxe.com),
// an OpenAI-compatible multi-model LLM gateway.

const (
	daoxeDomain = "api.daoxe.com"
)

type daoxeProviderInitializer struct{}

func (m *daoxeProviderInitializer) ValidateConfig(config *ProviderConfig) error {
	if config.apiTokens == nil || len(config.apiTokens) == 0 {
		return errors.New("no apiToken found in provider config")
	}
	return nil
}

func (m *daoxeProviderInitializer) DefaultCapabilities() map[string]string {
	return map[string]string{
		string(ApiNameChatCompletion): PathOpenAIChatCompletions,
		string(ApiNameCompletion):     PathOpenAICompletions,
		string(ApiNameModels):         PathOpenAIModels,
	}
}

func (m *daoxeProviderInitializer) CreateProvider(config ProviderConfig) (Provider, error) {
	config.setDefaultCapabilities(m.DefaultCapabilities())
	return &daoxeProvider{
		config:       config,
		contextCache: createContextCache(&config),
	}, nil
}

type daoxeProvider struct {
	config       ProviderConfig
	contextCache *contextCache
}

func (d *daoxeProvider) GetProviderType() string {
	return providerTypeDaoxe
}

func (d *daoxeProvider) OnRequestHeaders(ctx wrapper.HttpContext, apiName ApiName) error {
	d.config.handleRequestHeaders(d, ctx, apiName)
	return nil
}

func (d *daoxeProvider) OnRequestBody(ctx wrapper.HttpContext, apiName ApiName, body []byte) (types.Action, error) {
	if !d.config.isSupportedAPI(apiName) {
		return types.ActionContinue, errUnsupportedApiName
	}
	return d.config.handleRequestBody(d, d.contextCache, ctx, apiName, body)
}

func (d *daoxeProvider) OnStreamingResponseBody(ctx wrapper.HttpContext, apiName ApiName, chunk []byte, isLastChunk bool) ([]byte, error) {
	if len(chunk) == 0 {
		return nil, nil
	}
	// DaoXE chat completion streams use the OpenAI-compatible SSE format,
	// so no provider-specific conversion is required here.
	return chunk, nil
}

func (d *daoxeProvider) TransformRequestHeaders(ctx wrapper.HttpContext, apiName ApiName, headers http.Header) {
	util.OverwriteRequestPathHeaderByCapability(headers, string(apiName), d.config.capabilities)
	util.OverwriteRequestHostHeader(headers, daoxeDomain)
	util.OverwriteRequestAuthorizationHeader(headers, "Bearer "+d.config.GetApiTokenInUse(ctx))
	headers.Del("Content-Length")
}

func (d *daoxeProvider) GetApiName(path string) ApiName {
	if strings.Contains(path, PathOpenAIChatCompletions) {
		return ApiNameChatCompletion
	}
	if strings.Contains(path, PathOpenAICompletions) {
		return ApiNameCompletion
	}
	if strings.Contains(path, PathOpenAIModels) {
		return ApiNameModels
	}
	return ""
}
