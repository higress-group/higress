package provider

import (
	"errors"
	"net/http"
	"strings"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-proxy/util"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

// baiduProvider is the provider for baidu service.
const (
	baiduDomain             = "qianfan.baidubce.com"
	baiduChatCompletionPath = "/v2/chat/completions"
	baiduEmbeddings         = "/v2/embeddings"
)

type baiduProviderInitializer struct{}

func (g *baiduProviderInitializer) ValidateConfig(config *ProviderConfig) error {
	if config.apiTokens == nil || len(config.apiTokens) == 0 {
		return errors.New("no apiToken found in provider config")
	}
	return nil
}

func (g *baiduProviderInitializer) DefaultCapabilities() map[string]string {
	return map[string]string{
		string(ApiNameChatCompletion): baiduChatCompletionPath,
		string(ApiNameEmbeddings):     baiduEmbeddings,
	}
}

func (g *baiduProviderInitializer) CreateProvider(config ProviderConfig) (Provider, error) {
	config.setDefaultCapabilities(g.DefaultCapabilities())
	return &baiduProvider{
		config:       config,
		contextCache: createContextCache(&config),
	}, nil
}

type baiduProvider struct {
	config       ProviderConfig
	contextCache *contextCache
}

func (g *baiduProvider) GetProviderType() string {
	return providerTypeBaidu
}

func (g *baiduProvider) OnRequestHeaders(ctx wrapper.HttpContext, apiName ApiName) error {
	g.config.handleRequestHeaders(g, ctx, apiName)
	return nil
}

func (g *baiduProvider) OnRequestBody(ctx wrapper.HttpContext, apiName ApiName, body []byte) (types.Action, error) {
	if !g.config.isSupportedAPI(apiName) {
		return types.ActionContinue, errUnsupportedApiName
	}
	return g.config.handleRequestBody(g, g.contextCache, ctx, apiName, body)
}

func (g *baiduProvider) TransformRequestHeaders(ctx wrapper.HttpContext, apiName ApiName, headers http.Header) {
	util.OverwriteRequestPathHeaderByCapability(headers, string(apiName), g.config.capabilities)
	util.OverwriteRequestHostHeader(headers, baiduDomain)
	util.OverwriteRequestAuthorizationHeader(headers, "Bearer "+g.config.GetApiTokenInUse(ctx))
	headers.Del("Content-Length")
}

func (g *baiduProvider) GetApiName(path string) ApiName {
	if strings.Contains(path, baiduChatCompletionPath) {
		return ApiNameChatCompletion
	}
	if strings.Contains(path, baiduEmbeddings) {
		return ApiNameEmbeddings
	}
	return ""
}
