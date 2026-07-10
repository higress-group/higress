package provider

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-proxy/util"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

// deeplProvider is the provider for DeepL service.
const (
	deeplHostPro            = "api.deepl.com"
	deeplHostFree           = "api-free.deepl.com"
	deeplChatCompletionPath = "/v2/translate"
)

type deeplProviderInitializer struct {
}

type deeplProvider struct {
	config       ProviderConfig
	contextCache *contextCache
}

// spec reference: https://developers.deepl.com/docs/v/zh/api-reference/translate/openapi-spec-for-text-translation
type deeplRequest struct {
	// "Model" parameter is used to distinguish which service to use
	Model              string   `json:"model,omitempty"`
	Text               []string `json:"text"`
	SourceLang         string   `json:"source_lang,omitempty"`
	TargetLang         string   `json:"target_lang"`
	Context            string   `json:"context,omitempty"`
	SplitSentences     string   `json:"split_sentences,omitempty"`
	PreserveFormatting bool     `json:"preserve_formatting,omitempty"`
	Formality          string   `json:"formality,omitempty"`
	GlossaryId         string   `json:"glossary_id,omitempty"`
	TagHandling        string   `json:"tag_handling,omitempty"`
	OutlineDetection   bool     `json:"outline_detection,omitempty"`
	NonSplittingTags   []string `json:"non_splitting_tags,omitempty"`
	SplittingTags      []string `json:"splitting_tags,omitempty"`
	IgnoreTags         []string `json:"ignore_tags,omitempty"`
}

type deeplResponse struct {
	Translations []deeplResponseTranslation `json:"translations,omitempty"`
	Message      string                     `json:"message,omitempty"`
}

type deeplResponseTranslation struct {
	DetectedSourceLanguage string `json:"detected_source_language"`
	Text                   string `json:"text"`
}

func (d *deeplProviderInitializer) ValidateConfig(config *ProviderConfig) error {
	if config.targetLang == "" {
		return errors.New("missing targetLang in deepl provider config")
	}
	return nil
}

func (d *deeplProviderInitializer) DefaultCapabilities() map[string]string {
	return map[string]string{
		string(ApiNameChatCompletion): deeplChatCompletionPath,
	}
}

func (d *deeplProviderInitializer) CreateProvider(config ProviderConfig) (Provider, error) {
	config.setDefaultCapabilities(d.DefaultCapabilities())
	return &deeplProvider{
		config:       config,
		contextCache: createContextCache(&config),
	}, nil
}

func (d *deeplProvider) GetProviderType() string {
	return providerTypeDeepl
}

func (d *deeplProvider) OnRequestHeaders(ctx wrapper.HttpContext, apiName ApiName) error {
	d.config.handleRequestHeaders(d, ctx, apiName)
	return nil
}

func (d *deeplProvider) TransformRequestHeaders(ctx wrapper.HttpContext, apiName ApiName, headers http.Header) {
	if apiName != "" {
		util.OverwriteRequestPathHeader(headers, deeplChatCompletionPath)
	}
	// TODO: Support default host through configuration
	util.OverwriteRequestHostHeader(headers, deeplHostFree)
	util.OverwriteRequestAuthorizationHeader(headers, "DeepL-Auth-Key "+d.config.GetApiTokenInUse(ctx))
}

func (d *deeplProvider) OnRequestBody(ctx wrapper.HttpContext, apiName ApiName, body []byte) (types.Action, error) {
	if !d.config.isSupportedAPI(apiName) {
		return types.ActionContinue, errUnsupportedApiName
	}
	return d.config.handleRequestBody(d, d.contextCache, ctx, apiName, body)
}

func (d *deeplProvider) TransformRequestBodyHeaders(ctx wrapper.HttpContext, apiName ApiName, body []byte, headers http.Header) ([]byte, error) {
	request := &chatCompletionRequest{}
	if err := decodeChatCompletionRequest(body, request); err != nil {
		return nil, err
	}
	ctx.SetContext(ctxKeyFinalRequestModel, request.Model)

	err := d.overwriteRequestHost(headers, request.Model)
	if err != nil {
		return nil, err
	}

	baiduRequest := d.deeplTextGenRequest(request)
	return json.Marshal(baiduRequest)
}

func (d *deeplProvider) TransformResponseBody(ctx wrapper.HttpContext, apiName ApiName, body []byte) ([]byte, error) {
	// Original-protocol responses pass through untouched.
	if d.config.IsOriginal() || apiName != ApiNameChatCompletion {
		return body, nil
	}
	deeplResponse := &deeplResponse{}
	if err := json.Unmarshal(body, deeplResponse); err != nil {
		return nil, fmt.Errorf("unable to unmarshal deepl response: %v", err)
	}
	response := d.responseDeepl2OpenAI(ctx, deeplResponse)
	return json.Marshal(response)
}

func (d *deeplProvider) responseDeepl2OpenAI(ctx wrapper.HttpContext, deeplResponse *deeplResponse) *chatCompletionResponse {
	var choices []chatCompletionChoice
	// Fail
	if deeplResponse.Message != "" {
		choices = make([]chatCompletionChoice, 1)
		choices[0] = chatCompletionChoice{
			Message: &chatMessage{Role: roleAssistant, Content: deeplResponse.Message},
			Index:   0,
		}
	} else {
		// Success
		choices = make([]chatCompletionChoice, len(deeplResponse.Translations))
		for idx, t := range deeplResponse.Translations {
			choices[idx] = chatCompletionChoice{
				Index:   idx,
				Message: &chatMessage{Role: roleAssistant, Content: t.Text, Name: t.DetectedSourceLanguage},
			}
		}
	}
	return &chatCompletionResponse{
		Created: time.Now().UnixMilli() / 1000,
		Object:  objectChatCompletion,
		Choices: choices,
		Model:   ctx.GetStringContext(ctxKeyFinalRequestModel, ""),
	}
}

func (d *deeplProvider) overwriteRequestHost(headers http.Header, model string) error {
	if model == "Pro" {
		util.OverwriteRequestHostHeader(headers, deeplHostPro)
	} else if model == "Free" {
		util.OverwriteRequestHostHeader(headers, deeplHostFree)
	} else {
		return errors.New(`deepl model should be "Free" or "Pro"`)
	}
	return nil
}

func (d *deeplProvider) deeplTextGenRequest(request *chatCompletionRequest) *deeplRequest {
	deeplRequest := &deeplRequest{
		Text:       make([]string, 0),
		TargetLang: d.config.targetLang,
	}
	for _, msg := range request.Messages {
		if msg.Role == roleSystem {
			deeplRequest.Context = msg.StringContent()
		} else {
			deeplRequest.Text = append(deeplRequest.Text, msg.StringContent())
		}
	}
	return deeplRequest
}

func (d *deeplProvider) GetApiName(path string) ApiName {
	if strings.Contains(path, deeplChatCompletionPath) {
		return ApiNameChatCompletion
	}
	return ""
}
