package provider

import (
	"errors"
	"net/http"
	"path"
	"strings"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-proxy/util"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

// cerebrasProvider is the provider for Cerebras service.

const (
	defaultCerebrasDomain = "api.cerebras.ai"
)

type cerebrasProviderInitializer struct{}

func (c *cerebrasProviderInitializer) ValidateConfig(config *ProviderConfig) error {
	if config.apiTokens == nil || len(config.apiTokens) == 0 {
		return errors.New("no apiToken found in provider config")
	}
	return nil
}

func (c *cerebrasProviderInitializer) DefaultCapabilities() map[string]string {
	return map[string]string{
		string(ApiNameChatCompletion): PathOpenAIChatCompletions,
		string(ApiNameModels):         PathOpenAIModels,
	}
}

func (c *cerebrasProviderInitializer) CreateProvider(config ProviderConfig) (Provider, error) {
	if config.openaiCustomUrl != "" {
		// Handle custom URL like OpenAI
		customUrl := strings.TrimPrefix(strings.TrimPrefix(config.openaiCustomUrl, "http://"), "https://")
		pairs := strings.SplitN(customUrl, "/", 2)
		customPath := "/"
		if len(pairs) == 2 {
			customPath += pairs[1]
		}
		capabilities := c.DefaultCapabilities()
		for key, mapPath := range capabilities {
			capabilities[key] = path.Join(customPath, strings.TrimPrefix(mapPath, "/v1"))
		}
		config.setDefaultCapabilities(capabilities)
		log.Debugf("ai-proxy: cerebras provider customDomain:%s, customPath:%s, capabilities:%v",
			pairs[0], customPath, capabilities)
		return &cerebrasProvider{
			config:       config,
			contextCache: createContextCache(&config),
			customDomain: pairs[0],
			customPath:   customPath,
		}, nil
	}

	// Set default capabilities
	config.setDefaultCapabilities(c.DefaultCapabilities())

	return &cerebrasProvider{
		config:       config,
		contextCache: createContextCache(&config),
	}, nil
}

type cerebrasProvider struct {
	config       ProviderConfig
	contextCache *contextCache
	customDomain string
	customPath   string
}

func (p *cerebrasProvider) GetProviderType() string {
	return providerTypeCerebras
}

func (p *cerebrasProvider) OnRequestHeaders(ctx wrapper.HttpContext, apiName ApiName) error {
	p.config.handleRequestHeaders(p, ctx, apiName)
	return nil
}

func (p *cerebrasProvider) TransformRequestHeaders(ctx wrapper.HttpContext, apiName ApiName, headers http.Header) {
	if p.customPath != "" {
		util.OverwriteRequestPathHeader(headers, p.customPath)
	} else if apiName != "" {
		util.OverwriteRequestPathHeaderByCapability(headers, string(apiName), p.config.capabilities)
	}

	if p.customDomain != "" {
		util.OverwriteRequestHostHeader(headers, p.customDomain)
	} else {
		util.OverwriteRequestHostHeader(headers, defaultCerebrasDomain)
	}
	if len(p.config.apiTokens) > 0 {
		util.OverwriteRequestAuthorizationHeader(headers, "Bearer "+p.config.GetApiTokenInUse(ctx))
	}
	headers.Del("Content-Length")
}

func (p *cerebrasProvider) OnRequestBody(ctx wrapper.HttpContext, apiName ApiName, body []byte) (types.Action, error) {
	if !p.config.isSupportedAPI(apiName) {
		return types.ActionContinue, errUnsupportedApiName
	}
	return p.config.handleRequestBody(p, p.contextCache, ctx, apiName, body)
}

func (p *cerebrasProvider) GetApiName(path string) ApiName {
	if strings.Contains(path, PathOpenAIChatCompletions) {
		return ApiNameChatCompletion
	}
	if strings.Contains(path, PathOpenAIModels) {
		return ApiNameModels
	}
	return ""
}
