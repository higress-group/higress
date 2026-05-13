package provider

import (
	"errors"
	"net/http"
	"strings"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-proxy/util"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	zhipuAiDefaultDomain         = "open.bigmodel.cn"
	zhipuAiInternationalDomain   = "api.z.ai"
	zhipuAiChatCompletionPath    = "/api/paas/v4/chat/completions"
	zhipuAiCodePlanPath          = "/api/coding/paas/v4/chat/completions"
	zhipuAiEmbeddingsPath        = "/api/paas/v4/embeddings"
	zhipuAiAnthropicMessagesPath = "/api/anthropic/v1/messages"
)

type zhipuAiProviderInitializer struct{}

func (m *zhipuAiProviderInitializer) ValidateConfig(config *ProviderConfig) error {
	if config.apiTokens == nil || len(config.apiTokens) == 0 {
		return errors.New("no apiToken found in provider config")
	}
	return nil
}

func (m *zhipuAiProviderInitializer) DefaultCapabilities(codePlanMode bool) map[string]string {
	chatPath := zhipuAiChatCompletionPath
	if codePlanMode {
		chatPath = zhipuAiCodePlanPath
	}
	return map[string]string{
		string(ApiNameChatCompletion): chatPath,
		string(ApiNameEmbeddings):     zhipuAiEmbeddingsPath,
		// string(ApiNameAnthropicMessages): zhipuAiAnthropicMessagesPath,
	}
}

func (m *zhipuAiProviderInitializer) CreateProvider(config ProviderConfig) (Provider, error) {
	config.setDefaultCapabilities(m.DefaultCapabilities(config.zhipuCodePlanMode))
	return &zhipuAiProvider{
		config:       config,
		contextCache: createContextCache(&config),
	}, nil
}

type zhipuAiProvider struct {
	config       ProviderConfig
	contextCache *contextCache
}

func (m *zhipuAiProvider) GetProviderType() string {
	return providerTypeZhipuAi
}

func (m *zhipuAiProvider) OnRequestHeaders(ctx wrapper.HttpContext, apiName ApiName) error {
	m.config.handleRequestHeaders(m, ctx, apiName)
	return nil
}

func (m *zhipuAiProvider) OnRequestBody(ctx wrapper.HttpContext, apiName ApiName, body []byte) (types.Action, error) {
	if !m.config.isSupportedAPI(apiName) {
		return types.ActionContinue, errUnsupportedApiName
	}
	return m.config.handleRequestBody(m, m.contextCache, ctx, apiName, body)
}

func (m *zhipuAiProvider) TransformRequestHeaders(ctx wrapper.HttpContext, apiName ApiName, headers http.Header) {
	util.OverwriteRequestPathHeaderByCapability(headers, string(apiName), m.config.capabilities)
	// Use configured domain or default to China domain
	domain := m.config.zhipuDomain
	if domain == "" {
		domain = zhipuAiDefaultDomain
	}
	util.OverwriteRequestHostHeader(headers, domain)
	util.OverwriteRequestAuthorizationHeader(headers, "Bearer "+m.config.GetApiTokenInUse(ctx))
	headers.Del("Content-Length")
}

func (m *zhipuAiProvider) TransformRequestBody(ctx wrapper.HttpContext, apiName ApiName, body []byte) ([]byte, error) {
	if apiName != ApiNameChatCompletion {
		return m.config.defaultTransformRequestBody(ctx, apiName, body)
	}

	// Check if reasoning_effort is set
	reasoningEffort := gjson.GetBytes(body, "reasoning_effort").String()
	if reasoningEffort != "" {
		// Add thinking config for ZhipuAI
		body, _ = sjson.SetBytes(body, "thinking", map[string]string{"type": "enabled"})
		// Remove reasoning_effort field as ZhipuAI doesn't recognize it
		body, _ = sjson.DeleteBytes(body, "reasoning_effort")
	} else if thinkingType, ok := ctx.GetContext(ctxKeyClaudeThinkingType).(string); ok && thinkingType != "enabled" {
		// Request came from Claude auto-conversion with thinking explicitly disabled or absent.
		// Explicitly set thinking=disabled to prevent ZhipuAI from enabling it by default.
		body, _ = sjson.SetBytes(body, "thinking", map[string]string{"type": "disabled"})
	}

	return m.config.defaultTransformRequestBody(ctx, apiName, body)
}

func (m *zhipuAiProvider) GetApiName(path string) ApiName {
	if strings.Contains(path, zhipuAiChatCompletionPath) || strings.Contains(path, zhipuAiCodePlanPath) {
		return ApiNameChatCompletion
	}
	if strings.Contains(path, zhipuAiEmbeddingsPath) {
		return ApiNameEmbeddings
	}
	if strings.Contains(path, zhipuAiAnthropicMessagesPath) {
		return ApiNameAnthropicMessages
	}
	return ""
}
