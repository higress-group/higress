package provider

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-proxy/util"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

const (
	cohereDomain = "api.cohere.com"
	// TODO: support more capabilities, upgrade to v2, docs: https://docs.cohere.com/v2/reference/chat
	cohereChatCompletionPath = "/v1/chat"
	cohereRerankPath         = "/v1/rerank"
)

type cohereProviderInitializer struct{}

func (m *cohereProviderInitializer) ValidateConfig(config *ProviderConfig) error {
	if config.apiTokens == nil || len(config.apiTokens) == 0 {
		return errors.New("no apiToken found in provider config")
	}
	return nil
}

func (m *cohereProviderInitializer) DefaultCapabilities() map[string]string {
	return map[string]string{
		string(ApiNameChatCompletion): cohereChatCompletionPath,
		string(ApiNameCohereV1Rerank): cohereRerankPath,
	}
}

func (m *cohereProviderInitializer) CreateProvider(config ProviderConfig) (Provider, error) {
	config.setDefaultCapabilities(m.DefaultCapabilities())
	return &cohereProvider{
		config:       config,
		contextCache: createContextCache(&config),
	}, nil
}

type cohereProvider struct {
	config       ProviderConfig
	contextCache *contextCache
}

type cohereTextGenRequest struct {
	Message          string   `json:"message,omitempty"`
	Model            string   `json:"model,omitempty"`
	Stream           bool     `json:"stream,omitempty"`
	MaxTokens        int      `json:"max_tokens,omitempty"`
	Temperature      float64  `json:"temperature,omitempty"`
	K                int      `json:"k,omitempty"`
	P                float64  `json:"p,omitempty"`
	Seed             int      `json:"seed,omitempty"`
	StopSequences    []string `json:"stop_sequences,omitempty"`
	FrequencyPenalty float64  `json:"frequency_penalty,omitempty"`
	PresencePenalty  float64  `json:"presence_penalty,omitempty"`
}

func (m *cohereProvider) GetProviderType() string {
	return providerTypeCohere
}

func (m *cohereProvider) OnRequestHeaders(ctx wrapper.HttpContext, apiName ApiName) error {
	m.config.handleRequestHeaders(m, ctx, apiName)
	return nil
}

func (m *cohereProvider) OnRequestBody(ctx wrapper.HttpContext, apiName ApiName, body []byte) (types.Action, error) {
	if !m.config.isSupportedAPI(apiName) {
		return types.ActionContinue, errUnsupportedApiName
	}
	return m.config.handleRequestBody(m, m.contextCache, ctx, apiName, body)
}

func (m *cohereProvider) buildCohereRequest(origin *chatCompletionRequest) *cohereTextGenRequest {
	if len(origin.Messages) == 0 {
		return nil
	}
	return &cohereTextGenRequest{
		Message:          origin.Messages[0].StringContent(),
		Model:            origin.Model,
		MaxTokens:        origin.MaxTokens,
		Stream:           origin.Stream,
		Temperature:      origin.Temperature,
		K:                origin.N,
		P:                origin.TopP,
		Seed:             origin.Seed,
		StopSequences:    origin.Stop,
		FrequencyPenalty: origin.FrequencyPenalty,
		PresencePenalty:  origin.PresencePenalty,
	}
}

func (m *cohereProvider) TransformRequestHeaders(ctx wrapper.HttpContext, apiName ApiName, headers http.Header) {
	util.OverwriteRequestPathHeaderByCapability(headers, string(apiName), m.config.capabilities)
	util.OverwriteRequestHostHeader(headers, cohereDomain)
	util.OverwriteRequestAuthorizationHeader(headers, "Bearer "+m.config.GetApiTokenInUse(ctx))
	headers.Del("Content-Length")
}

func (m *cohereProvider) TransformRequestBody(ctx wrapper.HttpContext, apiName ApiName, body []byte) ([]byte, error) {
	if apiName != ApiNameChatCompletion {
		return m.config.defaultTransformRequestBody(ctx, apiName, body)
	}
	request := &chatCompletionRequest{}
	if err := m.config.parseRequestAndMapModel(ctx, request, body); err != nil {
		return nil, err
	}

	cohereRequest := m.buildCohereRequest(request)
	return json.Marshal(cohereRequest)
}

func (m *cohereProvider) GetApiName(path string) ApiName {
	if strings.Contains(path, cohereChatCompletionPath) {
		return ApiNameChatCompletion
	}
	if strings.Contains(path, cohereRerankPath) {
		return ApiNameCohereV1Rerank
	}
	return ""
}
