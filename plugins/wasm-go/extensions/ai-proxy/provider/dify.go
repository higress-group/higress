package provider

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-proxy/util"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

const (
	difyDomain         = "api.dify.ai"
	difyChatPath       = "/v1/chat-messages"
	difyCompletionPath = "/v1/completion-messages"
	difyWorkflowPath   = "/v1/workflows/run"
	BotTypeChat        = "Chat"
	BotTypeCompletion  = "Completion"
	BotTypeWorkflow    = "Workflow"
	BotTypeAgent       = "Agent"
)

type difyProviderInitializer struct{}

func (d *difyProviderInitializer) ValidateConfig(config *ProviderConfig) error {
	if config.apiTokens == nil || len(config.apiTokens) == 0 {
		return errors.New("no apiToken found in provider config")
	}
	return nil
}

func (d *difyProviderInitializer) CreateProvider(config ProviderConfig) (Provider, error) {
	return &difyProvider{
		config:       config,
		contextCache: createContextCache(&config),
	}, nil
}

type difyProvider struct {
	config       ProviderConfig
	contextCache *contextCache
}

func (d *difyProvider) GetProviderType() string {
	return providerTypeDify
}

func (d *difyProvider) OnRequestHeaders(ctx wrapper.HttpContext, apiName ApiName) error {
	d.config.handleRequestHeaders(d, ctx, apiName)
	return nil
}

func (d *difyProvider) TransformRequestHeaders(ctx wrapper.HttpContext, apiName ApiName, headers http.Header) {
	if d.config.difyApiUrl != "" {
		log.Debugf("use local host: %s", d.config.difyApiUrl)
		// Extract hostname, including Full URL or Domain
		host := d.config.difyApiUrl
		if parsedUrl, err := url.Parse(d.config.difyApiUrl); err == nil && parsedUrl.Host != "" {
			host = parsedUrl.Host
		} else {
			host = strings.TrimPrefix(strings.TrimPrefix(d.config.difyApiUrl, "http://"), "https://")
			if idx := strings.Index(host, "/"); idx != -1 {
				host = host[:idx]
			}
		}
		log.Debugf("extracted hostname: %s", host)
		util.OverwriteRequestHostHeader(headers, host)
	} else {
		util.OverwriteRequestHostHeader(headers, difyDomain)
	}
	switch d.config.botType {
	case BotTypeChat, BotTypeAgent:
		util.OverwriteRequestPathHeader(headers, difyChatPath)
	case BotTypeCompletion:
		util.OverwriteRequestPathHeader(headers, difyCompletionPath)
	case BotTypeWorkflow:
		util.OverwriteRequestPathHeader(headers, difyWorkflowPath)
	}
	util.OverwriteRequestAuthorizationHeader(headers, "Bearer "+d.config.GetApiTokenInUse(ctx))
}

func (d *difyProvider) OnRequestBody(ctx wrapper.HttpContext, apiName ApiName, body []byte) (types.Action, error) {
	if apiName != ApiNameChatCompletion {
		return types.ActionContinue, errUnsupportedApiName
	}
	return d.config.handleRequestBody(d, d.contextCache, ctx, apiName, body)
}

func (d *difyProvider) TransformRequestBodyHeaders(ctx wrapper.HttpContext, apiName ApiName, body []byte, headers http.Header) ([]byte, error) {
	if apiName != ApiNameChatCompletion {
		return d.config.defaultTransformRequestBody(ctx, apiName, body)
	}
	request := &chatCompletionRequest{}
	err := d.config.parseRequestAndMapModel(ctx, request, body)
	if err != nil {
		return nil, err
	}

	difyRequest := d.difyChatGenRequest(request)

	return json.Marshal(difyRequest)
}

func (d *difyProvider) TransformResponseBody(ctx wrapper.HttpContext, apiName ApiName, body []byte) ([]byte, error) {
	if apiName != ApiNameChatCompletion {
		return body, nil
	}
	difyResponse := &DifyChatResponse{}
	if err := json.Unmarshal(body, difyResponse); err != nil {
		return nil, fmt.Errorf("unable to unmarshal dify response: %v", err)
	}
	response := d.responseDify2OpenAI(ctx, difyResponse)
	return json.Marshal(response)
}

func (d *difyProvider) responseDify2OpenAI(ctx wrapper.HttpContext, response *DifyChatResponse) *chatCompletionResponse {
	var choice chatCompletionChoice
	var id string
	switch d.config.botType {
	case BotTypeChat, BotTypeAgent:
		choice = chatCompletionChoice{
			Index:        0,
			Message:      &chatMessage{Role: roleAssistant, Content: response.Answer},
			FinishReason: util.Ptr(finishReasonStop),
		}
		// response header中增加conversationId字段
		_ = proxywasm.ReplaceHttpResponseHeader("ConversationId", response.ConversationId)
		id = response.ConversationId
	case BotTypeCompletion:
		choice = chatCompletionChoice{
			Index:        0,
			Message:      &chatMessage{Role: roleAssistant, Content: response.Answer},
			FinishReason: util.Ptr(finishReasonStop),
		}
		id = response.MessageId
	case BotTypeWorkflow:
		choice = chatCompletionChoice{
			Index:        0,
			Message:      &chatMessage{Role: roleAssistant, Content: response.Data.Outputs[d.config.outputVariable]},
			FinishReason: util.Ptr(finishReasonStop),
		}
		id = response.Data.WorkflowId
	}
	return &chatCompletionResponse{
		Id:                id,
		Created:           response.CreatedAt,
		Model:             ctx.GetStringContext(ctxKeyFinalRequestModel, ""),
		SystemFingerprint: "",
		Object:            objectChatCompletion,
		Choices:           []chatCompletionChoice{choice},
		Usage:             &response.MetaData.Usage,
	}
}

func (d *difyProvider) OnStreamingResponseBody(ctx wrapper.HttpContext, name ApiName, chunk []byte, isLastChunk bool) ([]byte, error) {
	if name != ApiNameChatCompletion {
		return chunk, nil
	}
	if len(chunk) == 0 && !isLastChunk {
		return nil, nil
	}
	// sample event response:
	// data: {"event": "agent_thought", "id": "8dcf3648-fbad-407a-85dd-73a6f43aeb9f", "task_id": "9cf1ddd7-f94b-459b-b942-b77b26c59e9b", "message_id": "1fb10045-55fd-4040-99e6-d048d07cbad3", "position": 1, "thought": "", "observation": "", "tool": "", "tool_input": "", "created_at": 1705639511, "message_files": [], "conversation_id": "c216c595-2d89-438c-b33c-aae5ddddd142"}

	// sample end event response:
	// data: {"event": "message_end", "id": "5e52ce04-874b-4d27-9045-b3bc80def685", "conversation_id": "45701982-8118-4bc5-8e9b-64562b4555f2", "metadata": {"usage": {"prompt_tokens": 1033, "prompt_unit_price": "0.001", "prompt_price_unit": "0.001", "prompt_price": "0.0010330", "completion_tokens": 135, "completion_unit_price": "0.002", "completion_price_unit": "0.001", "completion_price": "0.0002700", "total_tokens": 1168, "total_price": "0.0013030", "currency": "USD", "latency": 1.381760165997548}, "retriever_resources": [{"position": 1, "dataset_id": "101b4c97-fc2e-463c-90b1-5261a4cdcafb", "dataset_name": "iPhone", "document_id": "8dd1ad74-0b5f-4175-b735-7d98bbbb4e00", "document_name": "iPhone List", "segment_id": "ed599c7f-2766-4294-9d1d-e5235a61270a", "score": 0.98457545, "content": "\"Model\",\"Release Date\",\"Display Size\",\"Resolution\",\"Processor\",\"RAM\",\"Storage\",\"Camera\",\"Battery\",\"Operating System\"\n\"iPhone 13 Pro Max\",\"September 24, 2021\",\"6.7 inch\",\"1284 x 2778\",\"Hexa-core (2x3.23 GHz Avalanche + 4x1.82 GHz Blizzard)\",\"6 GB\",\"128, 256, 512 GB, 1TB\",\"12 MP\",\"4352 mAh\",\"iOS 15\""}]}}
	responseBuilder := &strings.Builder{}
	for _, event := range frameSSEEvents(ctx, ctxKeyDifySSEFraming, chunk, isLastChunk) {
		for _, data := range strings.Split(event, "\n") {
			if len(data) < 6 {
				// ignore blank line or wrong format
				continue
			}
			data = data[6:]
			var difyResponse DifyChunkChatResponse
			if err := json.Unmarshal([]byte(data), &difyResponse); err != nil {
				log.Errorf("unable to unmarshal dify response: %v", err)
				continue
			}
			response := d.streamResponseDify2OpenAI(ctx, &difyResponse)
			responseBody, err := json.Marshal(response)
			if err != nil {
				log.Errorf("unable to marshal response: %v", err)
				return nil, err
			}
			d.appendResponse(responseBuilder, string(responseBody))
		}
	}
	modifiedResponseChunk := responseBuilder.String()
	log.Debugf("=== modified response chunk: %s", modifiedResponseChunk)
	return []byte(modifiedResponseChunk), nil
}

func (d *difyProvider) streamResponseDify2OpenAI(ctx wrapper.HttpContext, response *DifyChunkChatResponse) *chatCompletionResponse {
	var choice chatCompletionChoice
	var id string
	var responseUsage *usage
	switch d.config.botType {
	case BotTypeChat, BotTypeAgent:
		choice = chatCompletionChoice{
			Index: 0,
			Delta: &chatMessage{Role: roleAssistant, Content: response.Answer},
		}
		id = response.ConversationId
		_ = proxywasm.ReplaceHttpResponseHeader("ConversationId", response.ConversationId)
	case BotTypeCompletion:
		choice = chatCompletionChoice{
			Index: 0,
			Delta: &chatMessage{Role: roleAssistant, Content: response.Answer},
		}
		id = response.MessageId
	case BotTypeWorkflow:
		choice = chatCompletionChoice{
			Index: 0,
			Delta: &chatMessage{Role: roleAssistant, Content: response.Data.Outputs[d.config.outputVariable]},
		}
		id = response.Data.WorkflowId
	}
	if response.Event == "message_end" || response.Event == "workflow_finished" {
		choice.FinishReason = util.Ptr(finishReasonStop)
		if response.Event == "message_end" {
			responseUsage = &usage{
				PromptTokens:     response.MetaData.Usage.PromptTokens,
				CompletionTokens: response.MetaData.Usage.CompletionTokens,
				TotalTokens:      response.MetaData.Usage.TotalTokens,
			}
		}
	}
	return &chatCompletionResponse{
		Id:                id,
		Created:           response.CreatedAt,
		Model:             ctx.GetStringContext(ctxKeyFinalRequestModel, ""),
		SystemFingerprint: "",
		Object:            objectChatCompletionChunk,
		Choices:           []chatCompletionChoice{choice},
		Usage:             responseUsage,
	}
}

func (d *difyProvider) appendResponse(responseBuilder *strings.Builder, responseBody string) {
	responseBuilder.WriteString(fmt.Sprintf("%s %s\n\n", streamDataItemKey, responseBody))
}

func (d *difyProvider) difyChatGenRequest(request *chatCompletionRequest) *DifyChatRequest {
	content := ""
	for _, message := range request.Messages {
		if message.Role == "system" {
			content += "SYSTEM: \n" + message.StringContent() + "\n"
		} else if message.Role == "assistant" {
			content += "ASSISTANT: \n" + message.StringContent() + "\n"
		} else {
			content += "USER: \n" + message.StringContent() + "\n"
		}
	}
	mode := "blocking"
	if request.Stream {
		mode = "streaming"
	}
	user := request.User
	if user == "" {
		user = "api-user"
	}
	switch d.config.botType {
	case BotTypeChat, BotTypeAgent:
		conversationId, _ := proxywasm.GetHttpRequestHeader("ConversationId")
		return &DifyChatRequest{
			Inputs:           make(map[string]interface{}),
			Query:            content,
			ResponseMode:     mode,
			User:             user,
			AutoGenerateName: false,
			ConversationId:   conversationId,
		}
	case BotTypeCompletion:
		return &DifyChatRequest{
			Inputs: map[string]interface{}{
				"query": content,
			},
			ResponseMode: mode,
			User:         user,
		}
	case BotTypeWorkflow:
		return &DifyChatRequest{
			Inputs: map[string]interface{}{
				d.config.inputVariable: content,
			},
			ResponseMode: mode,
			User:         user,
		}
	default:
		return &DifyChatRequest{}
	}
}

type DifyChatRequest struct {
	Inputs           map[string]interface{} `json:"inputs"`
	Query            string                 `json:"query"`
	ResponseMode     string                 `json:"response_mode"`
	User             string                 `json:"user"`
	AutoGenerateName bool                   `json:"auto_generate_name"`
	ConversationId   string                 `json:"conversation_id"`
}

type DifyMetaData struct {
	Usage usage `json:"usage"`
}

type DifyData struct {
	WorkflowId string                 `json:"workflow_id"`
	Id         string                 `json:"id"`
	Outputs    map[string]interface{} `json:"outputs"`
}

type DifyChatResponse struct {
	ConversationId string       `json:"conversation_id"`
	MessageId      string       `json:"message_id"`
	Answer         string       `json:"answer"`
	CreatedAt      int64        `json:"created_at"`
	Data           DifyData     `json:"data"`
	MetaData       DifyMetaData `json:"metadata"`
}

type DifyChunkChatResponse struct {
	Event          string       `json:"event"`
	ConversationId string       `json:"conversation_id"`
	MessageId      string       `json:"message_id"`
	Answer         string       `json:"answer"`
	CreatedAt      int64        `json:"created_at"`
	Data           DifyData     `json:"data"`
	MetaData       DifyMetaData `json:"metadata"`
}
