package provider

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-proxy/util"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

const (
	cohereDomain = "api.cohere.com"
	// TODO: support more capabilities, upgrade to v2, docs: https://docs.cohere.com/v2/reference/chat
	cohereChatCompletionPath = "/v1/chat"
	cohereRerankPath         = "/v1/rerank"
	// ctxKeyCohereStreamId carries the Cohere generation id across chunks for the OpenAI chunk id.
	ctxKeyCohereStreamId = "cohereStreamId"
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
	return ""
}

// Cohere v1 /chat non-streaming response, only the fields ai-proxy maps (tools are never
// forwarded to Cohere, so tool_calls responses can't occur).
type cohereChatResponse struct {
	ResponseId   string `json:"response_id"`
	Text         string `json:"text"`
	FinishReason string `json:"finish_reason"`
	Meta         struct {
		Tokens struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"tokens"`
	} `json:"meta"`
}

// One Cohere v1 /chat streaming event (JSONL or SSE frame).
type cohereStreamEvent struct {
	EventType    string `json:"event_type"`
	GenerationId string `json:"generation_id"` // from stream-start; reused as the chunk id
	Text         string `json:"text"`
	Response     *struct {
		ResponseId   string `json:"response_id"`
		FinishReason string `json:"finish_reason"`
		Meta         struct {
			Tokens struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"tokens"`
		} `json:"meta"`
	} `json:"response"`
}

func cohereFinishReason2OpenAI(reason string) string {
	if reason == "MAX_TOKENS" {
		return finishReasonLength
	}
	return finishReasonStop
}

func (m *cohereProvider) TransformResponseHeaders(ctx wrapper.HttpContext, apiName ApiName, headers http.Header) {
	// Original-protocol and non-chat (e.g. rerank) responses pass through untouched.
	if m.config.IsOriginal() || apiName != ApiNameChatCompletion {
		ctx.DontReadResponseBody()
		return
	}
	headers.Del("Content-Length")
	// Cohere streams newline-delimited JSON; the converted output is OpenAI SSE.
	if strings.Contains(headers.Get("Content-Type"), "stream") {
		headers.Set("Content-Type", "text/event-stream; charset=utf-8")
	}
}

func (m *cohereProvider) TransformResponseBody(ctx wrapper.HttpContext, apiName ApiName, body []byte) ([]byte, error) {
	if m.config.IsOriginal() || apiName != ApiNameChatCompletion {
		return body, nil
	}
	cohereResponse := &cohereChatResponse{}
	if err := json.Unmarshal(body, cohereResponse); err != nil {
		return nil, fmt.Errorf("unable to unmarshal cohere response: %v", err)
	}
	openAIResponse := &chatCompletionResponse{
		Id:      cohereResponse.ResponseId,
		Created: time.Now().Unix(),
		Model:   ctx.GetStringContext(ctxKeyFinalRequestModel, ""),
		Object:  objectChatCompletion,
		Choices: []chatCompletionChoice{{
			Index:        0,
			Message:      &chatMessage{Role: roleAssistant, Content: cohereResponse.Text},
			FinishReason: util.Ptr(cohereFinishReason2OpenAI(cohereResponse.FinishReason)),
		}},
		Usage: &usage{
			PromptTokens:     cohereResponse.Meta.Tokens.InputTokens,
			CompletionTokens: cohereResponse.Meta.Tokens.OutputTokens,
			TotalTokens:      cohereResponse.Meta.Tokens.InputTokens + cohereResponse.Meta.Tokens.OutputTokens,
		},
	}
	return json.Marshal(openAIResponse)
}

func (m *cohereProvider) OnStreamingResponseBody(ctx wrapper.HttpContext, name ApiName, chunk []byte, isLastChunk bool) ([]byte, error) {
	if m.config.IsOriginal() || name != ApiNameChatCompletion {
		if isLastChunk {
			return nil, nil
		}
		return chunk, nil
	}

	body := chunk
	if buffered, has := ctx.GetContext(ctxKeyStreamingBody).([]byte); has {
		body = append(buffered, chunk...)
	}
	lines := bytes.Split(body, []byte("\n"))
	if isLastChunk {
		// Last chunk: the un-terminated trailing line is complete too.
		ctx.SetContext(ctxKeyStreamingBody, []byte(nil))
	} else {
		// Re-buffer the trailing partial line.
		ctx.SetContext(ctxKeyStreamingBody, lines[len(lines)-1])
		lines = lines[:len(lines)-1]
	}

	responseBuilder := &strings.Builder{}
	model := ctx.GetStringContext(ctxKeyFinalRequestModel, "")
	for _, line := range lines {
		line = bytes.TrimSpace(line)
		// Accept both bare JSONL and SSE frames: skip comment/event lines, strip an optional "data:".
		if len(line) == 0 || bytes.HasPrefix(line, []byte(":")) || bytes.HasPrefix(line, []byte("event:")) {
			continue
		}
		line = bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if len(line) == 0 {
			continue
		}
		var event cohereStreamEvent
		if err := json.Unmarshal(line, &event); err != nil {
			log.Errorf("unable to unmarshal cohere stream event: %v", err)
			continue
		}
		openAIChunk := &chatCompletionResponse{
			Id:      ctx.GetStringContext(ctxKeyCohereStreamId, ""),
			Created: time.Now().Unix(),
			Model:   model,
			Object:  objectChatCompletionChunk,
		}
		switch event.EventType {
		case "stream-start":
			ctx.SetContext(ctxKeyCohereStreamId, event.GenerationId)
			openAIChunk.Id = event.GenerationId
			// Role belongs in the first delta only (OpenAI convention).
			openAIChunk.Choices = []chatCompletionChoice{{Index: 0, Delta: &chatMessage{Role: roleAssistant}}}
		case "text-generation":
			openAIChunk.Choices = []chatCompletionChoice{{Index: 0, Delta: &chatMessage{Content: event.Text}}}
		case "stream-end":
			finishReason := finishReasonStop
			if event.Response != nil {
				finishReason = cohereFinishReason2OpenAI(event.Response.FinishReason)
			}
			// Finish chunk (usage:null), then a separate choices:[] usage chunk (OpenAI convention).
			openAIChunk.Choices = []chatCompletionChoice{{Index: 0, Delta: &chatMessage{}, FinishReason: util.Ptr(finishReason)}}
			finishBytes, err := json.Marshal(openAIChunk)
			if err != nil {
				return nil, err
			}
			responseBuilder.WriteString(ssePrefix + string(finishBytes) + "\n\n")
			if event.Response != nil {
				usageChunk := &chatCompletionResponse{
					Id:      ctx.GetStringContext(ctxKeyCohereStreamId, ""),
					Created: time.Now().Unix(),
					Model:   model,
					Object:  objectChatCompletionChunk,
					Choices: []chatCompletionChoice{},
					Usage: &usage{
						PromptTokens:     event.Response.Meta.Tokens.InputTokens,
						CompletionTokens: event.Response.Meta.Tokens.OutputTokens,
						TotalTokens:      event.Response.Meta.Tokens.InputTokens + event.Response.Meta.Tokens.OutputTokens,
					},
				}
				usageBytes, err := json.Marshal(usageChunk)
				if err != nil {
					return nil, err
				}
				responseBuilder.WriteString(ssePrefix + string(usageBytes) + "\n\n")
			}
			continue
		default:
			continue // other events carry no OpenAI-visible content
		}
		chunkBytes, err := json.Marshal(openAIChunk)
		if err != nil {
			return nil, err
		}
		responseBuilder.WriteString(ssePrefix + string(chunkBytes) + "\n\n")
	}
	// Emit the OpenAI `data: [DONE]` terminator on the final chunk (Cohere native has none).
	if isLastChunk {
		responseBuilder.WriteString(ssePrefix + streamEndDataValue + "\n\n")
	}
	return []byte(responseBuilder.String()), nil
}
