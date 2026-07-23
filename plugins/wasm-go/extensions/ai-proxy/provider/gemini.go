package provider

import (
	"errors"
	"net/http"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-proxy/util"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

// geminiProvider is the provider for google gemini/gemini flash service.
// 支持两种模式：
// 1. OpenAI格式（默认）：当protocol为openai时，使用gemini提供的openai兼容接口，对header进行必要的修改，然后透传OpenAI格式的请求/响应body
// 2. 原生模式：当protocol设置为original时，直接透传gemini格式的请求/响应

const (
	geminiApiKeyHeader                  = "x-goog-api-key"
	geminiDefaultApiVersion             = "v1beta" // 可选: v1, v1beta
	geminiDomain                        = "generativelanguage.googleapis.com"
	geminiCompatibleChatCompletionPath  = "/v1beta/openai/chat/completions"
	geminiCompatibleEmbeddingPath       = "/v1beta/openai/embeddings"
	geminiCompatibleImageGenerationPath = "/v1beta/openai/images/generations"
	geminiCompatibleModelsPath          = "/v1beta/openai/models"
)

type geminiProviderInitializer struct{}

func (g *geminiProviderInitializer) ValidateConfig(config *ProviderConfig) error {
	if len(config.apiTokens) == 0 {
		return errors.New("no apiToken found in provider config")
	}
	return nil
}

func (g *geminiProviderInitializer) DefaultCapabilities() map[string]string {
	return map[string]string{
		string(ApiNameChatCompletion):  "",
		string(ApiNameEmbeddings):      "",
		string(ApiNameModels):          "",
		string(ApiNameImageGeneration): "",
	}
}

func (g *geminiProviderInitializer) CreateProvider(config ProviderConfig) (Provider, error) {
	config.setDefaultCapabilities(g.DefaultCapabilities())
	return &geminiProvider{
		config:       config,
		contextCache: createContextCache(&config),
		client: wrapper.NewClusterClient(wrapper.RouteCluster{
			Host: geminiDomain,
		}),
	}, nil
}

type geminiProvider struct {
	config       ProviderConfig
	contextCache *contextCache

	client wrapper.HttpClient
}

func (g *geminiProvider) GetProviderType() string {
	return providerTypeGemini
}

func (g *geminiProvider) OnRequestHeaders(ctx wrapper.HttpContext, apiName ApiName) error {
	g.config.handleRequestHeaders(g, ctx, apiName)
	// Delay the header processing to allow changing streaming mode in OnRequestBody
	return nil
}

func (g *geminiProvider) TransformRequestHeaders(ctx wrapper.HttpContext, apiName ApiName, headers http.Header) {
	util.OverwriteRequestHostHeader(headers, geminiDomain)
	if g.config.IsOriginal() {
		// gemini original protocol
		headers.Set(geminiApiKeyHeader, g.config.GetApiTokenInUse(ctx))
		util.OverwriteRequestAuthorizationHeader(headers, "")
	} else {
		// gemini openai compatible protocol
		util.OverwriteRequestAuthorizationHeader(headers, "Bearer "+g.config.GetApiTokenInUse(ctx))
		path := g.getRequestPath(apiName)
		util.OverwriteRequestPathHeader(headers, path)
	}
}

func (g *geminiProvider) getRequestPath(apiName ApiName) string {
	if g.config.apiVersion == "" {
		g.config.apiVersion = geminiDefaultApiVersion
	}
	switch apiName {
	case ApiNameModels:
		return geminiCompatibleModelsPath
	case ApiNameEmbeddings:
		return geminiCompatibleEmbeddingPath
	case ApiNameChatCompletion:
		return geminiCompatibleChatCompletionPath
	case ApiNameImageGeneration:
		return geminiCompatibleImageGenerationPath
	}
	return ""
}
