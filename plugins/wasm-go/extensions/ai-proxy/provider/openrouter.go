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

// openrouterProvider is the provider for OpenRouter service.
const (
	openrouterDomain             = "openrouter.ai"
	openrouterChatCompletionPath = "/api/v1/chat/completions"
	openrouterCompletionPath     = "/api/v1/completions"
)

type openrouterProviderInitializer struct{}

func (o *openrouterProviderInitializer) ValidateConfig(config *ProviderConfig) error {
	if len(config.apiTokens) == 0 {
		return errors.New("no apiToken found in provider config")
	}
	return nil
}

func (o *openrouterProviderInitializer) DefaultCapabilities() map[string]string {
	return map[string]string{
		string(ApiNameChatCompletion): openrouterChatCompletionPath,
		string(ApiNameCompletion):     openrouterCompletionPath,
	}
}

func (o *openrouterProviderInitializer) CreateProvider(config ProviderConfig) (Provider, error) {
	config.setDefaultCapabilities(o.DefaultCapabilities())
	return &openrouterProvider{
		config:       config,
		contextCache: createContextCache(&config),
	}, nil
}

type openrouterProvider struct {
	config       ProviderConfig
	contextCache *contextCache
}

func (o *openrouterProvider) GetProviderType() string {
	return providerTypeOpenRouter
}

func (o *openrouterProvider) OnRequestHeaders(ctx wrapper.HttpContext, apiName ApiName) error {
	o.config.handleRequestHeaders(o, ctx, apiName)
	return nil
}

func (o *openrouterProvider) OnRequestBody(ctx wrapper.HttpContext, apiName ApiName, body []byte) (types.Action, error) {
	if !o.config.isSupportedAPI(apiName) {
		return types.ActionContinue, errUnsupportedApiName
	}
	return o.config.handleRequestBody(o, o.contextCache, ctx, apiName, body)
}

func (o *openrouterProvider) TransformRequestHeaders(ctx wrapper.HttpContext, apiName ApiName, headers http.Header) {
	util.OverwriteRequestPathHeaderByCapability(headers, string(apiName), o.config.capabilities)
	util.OverwriteRequestHostHeader(headers, openrouterDomain)
	util.OverwriteRequestAuthorizationHeader(headers, "Bearer "+o.config.GetApiTokenInUse(ctx))
	headers.Del("Content-Length")
}

func (o *openrouterProvider) TransformRequestBody(ctx wrapper.HttpContext, apiName ApiName, body []byte) ([]byte, error) {
	if apiName != ApiNameChatCompletion {
		return o.config.defaultTransformRequestBody(ctx, apiName, body)
	}

	// Check if ReasoningMaxTokens exists in the request body
	reasoningMaxTokens := gjson.GetBytes(body, "reasoning_max_tokens")
	if !reasoningMaxTokens.Exists() || reasoningMaxTokens.Int() == 0 {
		// Check if budget_tokens was stored in context (from Claude auto-conversion path)
		// Only use it when thinking was explicitly enabled, to avoid dirty input
		if thinkingType, _ := ctx.GetContext(ctxKeyClaudeThinkingType).(string); thinkingType == "enabled" {
			if budgetTokens, ok := ctx.GetContext(ctxKeyClaudeBudgetTokens).(int); ok && budgetTokens > 0 {
				// Use budget_tokens from Claude thinking config
				modifiedBody, err := sjson.DeleteBytes(body, "reasoning_effort")
				if err != nil {
					modifiedBody = body
				}
				modifiedBody, err = sjson.SetBytes(modifiedBody, "reasoning.max_tokens", budgetTokens)
				if err != nil {
					return nil, err
				}
				return o.config.defaultTransformRequestBody(ctx, apiName, modifiedBody)
			}
		}
		// No reasoning_max_tokens, use default transformation
		return o.config.defaultTransformRequestBody(ctx, apiName, body)
	}

	// Clear reasoning_effort field if it exists
	modifiedBody, err := sjson.DeleteBytes(body, "reasoning_effort")
	if err != nil {
		// If delete fails, continue with original body
		modifiedBody = body
	}

	// Set reasoning.max_tokens to the value of reasoning_max_tokens
	modifiedBody, err = sjson.SetBytes(modifiedBody, "reasoning.max_tokens", reasoningMaxTokens.Int())
	if err != nil {
		return nil, err
	}

	// Remove the original reasoning_max_tokens field
	modifiedBody, err = sjson.DeleteBytes(modifiedBody, "reasoning_max_tokens")
	if err != nil {
		return nil, err
	}

	// Apply default model mapping
	return o.config.defaultTransformRequestBody(ctx, apiName, modifiedBody)
}

func (o *openrouterProvider) GetApiName(path string) ApiName {
	if strings.Contains(path, openrouterChatCompletionPath) {
		return ApiNameChatCompletion
	}
	if strings.Contains(path, openrouterCompletionPath) {
		return ApiNameCompletion
	}
	return ""
}
