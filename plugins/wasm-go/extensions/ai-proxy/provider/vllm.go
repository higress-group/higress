package provider

import (
	"net/http"
	"path"
	"strings"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-proxy/util"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

const (
	defaultVllmDomain = "vllm-service.cluster.local"
)

// isVllmDirectPath checks if the path is a known standard vLLM interface path,
// i.e. the configured vllmCustomUrl already points at a concrete endpoint rather
// than a base path. Such paths are forwarded as-is; base paths get the per-API
// suffix appended. Must cover every endpoint in DefaultCapabilities that a user
// might configure directly, otherwise the path is mistakenly treated as a base
// and double-appended (e.g. /v1/responses -> /v1/responses/responses).
func isVllmDirectPath(path string) bool {
	return strings.HasSuffix(path, "/completions") ||
		strings.HasSuffix(path, "/rerank") ||
		strings.HasSuffix(path, "/responses") ||
		strings.HasSuffix(path, "/messages") ||
		strings.HasSuffix(path, "/count_tokens") ||
		strings.HasSuffix(path, "/transcriptions") ||
		strings.HasSuffix(path, "/translations")
}

type vllmProviderInitializer struct{}

func (m *vllmProviderInitializer) ValidateConfig(config *ProviderConfig) error {
	// vLLM supports both authenticated and unauthenticated access
	// If API tokens are configured, they will be used for authentication
	// If no tokens are configured, the service will be accessed without authentication
	return nil
}

func (m *vllmProviderInitializer) DefaultCapabilities() map[string]string {
	return map[string]string{
		string(ApiNameChatCompletion): PathOpenAIChatCompletions,
		string(ApiNameCompletion):     PathOpenAICompletions,
		string(ApiNameModels):         PathOpenAIModels,
		string(ApiNameEmbeddings):     PathOpenAIEmbeddings,
		string(ApiNameCohereV1Rerank): PathCohereV1Rerank,
		// vLLM also natively serves the Anthropic Messages API and newer OpenAI
		// endpoints; expose them as passthrough (no protocol translation).
		string(ApiNameAnthropicMessages):    PathAnthropicMessages,
		string(ApiNameAnthropicCountTokens): PathAnthropicMessagesCountTokens,
		string(ApiNameResponses):            PathOpenAIResponses,
		string(ApiNameAudioTranscription):   PathOpenAIAudioTranscriptions,
		string(ApiNameAudioTranslation):     PathOpenAIAudioTranslations,
	}
}

func (m *vllmProviderInitializer) CreateProvider(config ProviderConfig) (Provider, error) {
	if config.GetVllmCustomUrl() == "" {
		config.setDefaultCapabilities(m.DefaultCapabilities())
		return &vllmProvider{
			config:       config,
			contextCache: createContextCache(&config),
		}, nil
	}

	// Parse custom URL to extract domain and path
	customUrl := strings.TrimPrefix(strings.TrimPrefix(config.GetVllmCustomUrl(), "http://"), "https://")
	pairs := strings.SplitN(customUrl, "/", 2)
	customPath := "/"
	if len(pairs) == 2 {
		customPath += pairs[1]
	}

	// Check if the custom path is a direct path
	isDirectCustomPath := isVllmDirectPath(customPath)
	capabilities := m.DefaultCapabilities()
	if !isDirectCustomPath {
		for key, mapPath := range capabilities {
			capabilities[key] = path.Join(customPath, strings.TrimPrefix(mapPath, "/v1"))
		}
	}
	config.setDefaultCapabilities(capabilities)

	return &vllmProvider{
		config:             config,
		customDomain:       pairs[0],
		customPath:         customPath,
		isDirectCustomPath: isDirectCustomPath,
		contextCache:       createContextCache(&config),
	}, nil
}

type vllmProvider struct {
	config             ProviderConfig
	customDomain       string
	customPath         string
	isDirectCustomPath bool
	contextCache       *contextCache
}

func (m *vllmProvider) GetProviderType() string {
	return providerTypeVllm
}

func (m *vllmProvider) OnRequestHeaders(ctx wrapper.HttpContext, apiName ApiName) error {
	m.config.handleRequestHeaders(m, ctx, apiName)
	return nil
}

func (m *vllmProvider) OnRequestBody(ctx wrapper.HttpContext, apiName ApiName, body []byte) (types.Action, error) {
	if !m.config.isSupportedAPI(apiName) {
		return types.ActionContinue, errUnsupportedApiName
	}
	return m.config.handleRequestBody(m, m.contextCache, ctx, apiName, body)
}

func (m *vllmProvider) TransformRequestHeaders(ctx wrapper.HttpContext, apiName ApiName, headers http.Header) {
	if m.isDirectCustomPath {
		util.OverwriteRequestPathHeader(headers, m.customPath)
	} else if apiName != "" {
		util.OverwriteRequestPathHeaderByCapability(headers, string(apiName), m.config.capabilities)
	}

	// Set vLLM server host
	if m.customDomain != "" {
		util.OverwriteRequestHostHeader(headers, m.customDomain)
	} else {
		// Fallback to legacy vllmServerHost configuration
		serverHost := m.config.GetVllmServerHost()
		if serverHost == "" {
			serverHost = defaultVllmDomain
		} else {
			// Extract domain from host:port format if present
			if strings.Contains(serverHost, ":") {
				parts := strings.SplitN(serverHost, ":", 2)
				serverHost = parts[0]
			}
		}
		util.OverwriteRequestHostHeader(headers, serverHost)
	}

	// Add Bearer Token authentication if API tokens are configured
	if len(m.config.apiTokens) > 0 {
		util.OverwriteRequestAuthorizationHeader(headers, "Bearer "+m.config.GetApiTokenInUse(ctx))
	}

	// Remove Content-Length header to allow body modification
	headers.Del("Content-Length")
}

func (m *vllmProvider) TransformRequestBody(ctx wrapper.HttpContext, apiName ApiName, body []byte) ([]byte, error) {
	// For vLLM, we can use the default transformation which handles model mapping
	return m.config.defaultTransformRequestBody(ctx, apiName, body)
}

func (m *vllmProvider) GetApiName(path string) ApiName {
	if strings.Contains(path, PathOpenAIChatCompletions) {
		return ApiNameChatCompletion
	}
	if strings.Contains(path, PathOpenAICompletions) {
		return ApiNameCompletion
	}
	if strings.Contains(path, PathOpenAIModels) {
		return ApiNameModels
	}
	if strings.Contains(path, PathOpenAIEmbeddings) {
		return ApiNameEmbeddings
	}
	if strings.Contains(path, PathCohereV1Rerank) {
		return ApiNameCohereV1Rerank
	}
	// count_tokens must be matched before /v1/messages: the former contains the latter.
	if strings.Contains(path, PathAnthropicMessagesCountTokens) {
		return ApiNameAnthropicCountTokens
	}
	if strings.Contains(path, PathAnthropicMessages) {
		return ApiNameAnthropicMessages
	}
	if strings.Contains(path, PathOpenAIResponses) {
		return ApiNameResponses
	}
	if strings.Contains(path, PathOpenAIAudioTranscriptions) {
		return ApiNameAudioTranscription
	}
	if strings.Contains(path, PathOpenAIAudioTranslations) {
		return ApiNameAudioTranslation
	}
	return ""
}

// TransformResponseHeaders handles response header transformation for vLLM
func (m *vllmProvider) TransformResponseHeaders(ctx wrapper.HttpContext, apiName ApiName, headers http.Header) {
	// Remove Content-Length header to allow response body modification
	headers.Del("Content-Length")
}

// TransformResponseBody handles response body transformation for vLLM
func (m *vllmProvider) TransformResponseBody(ctx wrapper.HttpContext, apiName ApiName, body []byte) ([]byte, error) {
	// For now, just return the body as-is
	// This can be extended to handle vLLM-specific response transformations
	return body, nil
}

// OnStreamingResponseBody handles streaming response body for vLLM
func (m *vllmProvider) OnStreamingResponseBody(ctx wrapper.HttpContext, name ApiName, chunk []byte, isLastChunk bool) ([]byte, error) {
	// For now, just return the chunk as-is
	// This can be extended to handle vLLM-specific streaming transformations
	return chunk, nil
}
