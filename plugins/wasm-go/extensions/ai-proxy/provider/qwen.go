package provider

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-proxy/util"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// qwenProvider is the provider for Qwen service.

const (
	qwenResultFormatMessage = "message"

	qwenDefaultDomain                     = "dashscope.aliyuncs.com"
	qwenChatCompletionPath                = "/api/v1/services/aigc/text-generation/generation"
	qwenTextEmbeddingPath                 = "/api/v1/services/embeddings/text-embedding/text-embedding"
	qwenTextRerankPath                    = "/api/v1/services/rerank/text-rerank/text-rerank"
	qwenCompatibleTextRerankPath          = "/compatible-api/v1/reranks"
	qwenCompatibleChatCompletionPath      = "/compatible-mode/v1/chat/completions"
	qwenCompatibleCompletionsPath         = "/compatible-mode/v1/completions"
	qwenCompatibleTextEmbeddingPath       = "/compatible-mode/v1/embeddings"
	qwenCompatibleConversationsPath       = "/compatible-mode/v1/conversations"
	qwenCompatibleResponsesPath           = "/compatible-mode/v1/responses"
	qwenCompatibleFilesPath               = "/compatible-mode/v1/files"
	qwenCompatibleRetrieveFilePath        = "/compatible-mode/v1/files/{file_id}"
	qwenCompatibleRetrieveFileContentPath = "/compatible-mode/v1/files/{file_id}/content"
	qwenCompatibleBatchesPath             = "/compatible-mode/v1/batches"
	qwenCompatibleRetrieveBatchPath       = "/compatible-mode/v1/batches/{batch_id}"
	qwenBailianPath                       = "/api/v1/apps"
	qwenMultimodalGenerationPath          = "/api/v1/services/aigc/multimodal-generation/generation"
	qwenAnthropicMessagesPath             = "/apps/anthropic/v1/messages"

	qwenAsyncAIGCPath = "/api/v1/services/aigc/"
	qwenAsyncTaskPath = "/api/v1/tasks/"

	qwenTopPMin = 0.000001
	qwenTopPMax = 0.999999

	qwenDummySystemMessageContent = "You are a helpful assistant."

	qwenLongModelName     = "qwen-long"
	qwenVlModelPrefixName = "qwen-vl"
)

type qwenProviderInitializer struct{}

func (m *qwenProviderInitializer) ValidateConfig(config *ProviderConfig) error {
	if len(config.qwenFileIds) != 0 && config.context != nil {
		return errors.New("qwenFileIds and context cannot be configured at the same time")
	}
	if len(config.apiTokens) == 0 {
		return errors.New("no apiToken found in provider config")
	}
	return nil
}

func (m *qwenProviderInitializer) DefaultCapabilities(qwenEnableCompatible bool) map[string]string {
	if qwenEnableCompatible {
		return map[string]string{
			string(ApiNameChatCompletion):      qwenCompatibleChatCompletionPath,
			string(ApiNameEmbeddings):          qwenCompatibleTextEmbeddingPath,
			string(ApiNameCompletion):          qwenCompatibleCompletionsPath,
			string(ApiNameResponses):           qwenCompatibleResponsesPath,
			string(ApiNameFiles):               qwenCompatibleFilesPath,
			string(ApiNameRetrieveFile):        qwenCompatibleRetrieveFilePath,
			string(ApiNameRetrieveFileContent): qwenCompatibleRetrieveFileContentPath,
			string(ApiNameBatches):             qwenCompatibleBatchesPath,
			string(ApiNameRetrieveBatch):       qwenCompatibleRetrieveBatchPath,
			string(ApiNameQwenAsyncAIGC):       qwenAsyncAIGCPath,
			string(ApiNameQwenAsyncTask):       qwenAsyncTaskPath,
			string(ApiNameQwenV1Rerank):        qwenCompatibleTextRerankPath,
			string(ApiNameQwenV1Conversations): qwenCompatibleConversationsPath,
			string(ApiNameAnthropicMessages):   qwenAnthropicMessagesPath,
		}
	} else {
		return map[string]string{
			string(ApiNameChatCompletion):    qwenChatCompletionPath,
			string(ApiNameEmbeddings):        qwenTextEmbeddingPath,
			string(ApiNameQwenAsyncAIGC):     qwenAsyncAIGCPath,
			string(ApiNameQwenAsyncTask):     qwenAsyncTaskPath,
			string(ApiNameQwenV1Rerank):      qwenTextRerankPath,
			string(ApiNameAnthropicMessages): qwenAnthropicMessagesPath,
		}
	}
}

func (m *qwenProviderInitializer) CreateProvider(config ProviderConfig) (Provider, error) {
	config.setDefaultCapabilities(m.DefaultCapabilities(config.qwenEnableCompatible))
	return &qwenProvider{
		config:       config,
		contextCache: createContextCache(&config),
	}, nil
}

type qwenProvider struct {
	config       ProviderConfig
	contextCache *contextCache
}

func (m *qwenProvider) TransformRequestHeaders(ctx wrapper.HttpContext, apiName ApiName, headers http.Header) {
	if m.config.qwenDomain != "" {
		util.OverwriteRequestHostHeader(headers, m.config.qwenDomain)
	} else {
		util.OverwriteRequestHostHeader(headers, qwenDefaultDomain)
	}
	util.OverwriteRequestAuthorizationHeader(headers, "Bearer "+m.config.GetApiTokenInUse(ctx))

	if !m.config.IsOriginal() {
		util.OverwriteRequestPathHeaderByCapability(headers, string(apiName), m.config.capabilities)
	}
}

func (m *qwenProvider) TransformRequestBodyHeaders(ctx wrapper.HttpContext, apiName ApiName, body []byte, headers http.Header) ([]byte, error) {
	if m.config.qwenEnableCompatible {
		modifiedBody := body
		if requestBodyHasMessageReasoningContent(modifiedBody) {
			// Qwen OpenAI-compatible mode requires top-level preserve_thinking=true
			// before historical reasoning_content can be reused by the model.
			var err error
			modifiedBody, err = sjson.SetBytes(modifiedBody, "preserve_thinking", true)
			if err != nil {
				return modifiedBody, err
			}
		}
		if gjson.GetBytes(modifiedBody, "model").Exists() {
			rawModel := gjson.GetBytes(modifiedBody, "model").String()
			mappedModel := getMappedModel(rawModel, m.config.modelMapping)
			newBody, err := sjson.SetBytes(modifiedBody, "model", mappedModel)
			if err != nil {
				log.Errorf("Replace model error: %v", err)
				return newBody, err
			}
			return newBody, err
		}
		return modifiedBody, nil
	}
	switch apiName {
	case ApiNameChatCompletion:
		return m.onChatCompletionRequestBody(ctx, body, headers)
	case ApiNameEmbeddings:
		return m.onEmbeddingsRequestBody(ctx, body)
	default:
		return m.config.defaultTransformRequestBody(ctx, apiName, body)
	}
}

func (m *qwenProvider) GetProviderType() string {
	return providerTypeQwen
}

func (m *qwenProvider) OnRequestHeaders(ctx wrapper.HttpContext, apiName ApiName) error {
	m.config.handleRequestHeaders(m, ctx, apiName)

	if m.config.protocol == protocolOriginal {
		ctx.DontReadRequestBody()
		return nil
	}

	return nil
}

func (m *qwenProvider) OnRequestBody(ctx wrapper.HttpContext, apiName ApiName, body []byte) (types.Action, error) {
	if !m.config.isSupportedAPI(apiName) {
		return types.ActionContinue, errUnsupportedApiName
	}
	return m.config.handleRequestBody(m, m.contextCache, ctx, apiName, body)
}

func (m *qwenProvider) onChatCompletionRequestBody(ctx wrapper.HttpContext, body []byte, headers http.Header) ([]byte, error) {
	request := &chatCompletionRequest{}
	err := m.config.parseRequestAndMapModel(ctx, request, body)
	if err != nil {
		return nil, err
	}

	// Use the qwen multimodal model generation API
	if strings.HasPrefix(request.Model, qwenVlModelPrefixName) {
		util.OverwriteRequestPathHeader(headers, qwenMultimodalGenerationPath)
	}

	streaming := request.Stream
	if streaming {
		headers.Set("Accept", "text/event-stream")
		headers.Set("X-DashScope-SSE", "enable")
	} else {
		headers.Set("Accept", "*/*")
		headers.Del("X-DashScope-SSE")
	}

	return m.buildQwenTextGenerationRequest(ctx, request, streaming)
}

func (m *qwenProvider) onEmbeddingsRequestBody(ctx wrapper.HttpContext, body []byte) ([]byte, error) {
	request := &embeddingsRequest{}
	if err := m.config.parseRequestAndMapModel(ctx, request, body); err != nil {
		return nil, err
	}

	qwenRequest, err := m.buildQwenTextEmbeddingRequest(request)
	if err != nil {
		return nil, err
	}
	return json.Marshal(qwenRequest)
}

func (m *qwenProvider) OnStreamingEvent(ctx wrapper.HttpContext, name ApiName, event StreamEvent) ([]StreamEvent, error) {
	if m.config.qwenEnableCompatible || name != ApiNameChatCompletion {
		return nil, nil
	}

	incrementalStreaming := ctx.GetBoolContext(ctxKeyIncrementalStreaming, false)

	qwenResponse := &qwenTextGenResponse{}
	if err := json.Unmarshal([]byte(event.Data), qwenResponse); err != nil {
		log.Errorf("unable to unmarshal Qwen response: %v", err)
		return nil, fmt.Errorf("unable to unmarshal Qwen response: %v", err)
	}

	var outputEvents []StreamEvent
	responses := m.buildChatCompletionStreamingResponse(ctx, qwenResponse, incrementalStreaming)
	for _, response := range responses {
		responseBody, err := json.Marshal(response)
		if err != nil {
			log.Errorf("unable to marshal response: %v", err)
			return nil, fmt.Errorf("unable to marshal response: %v", err)
		}
		modifiedEvent := event
		modifiedEvent.Data = string(responseBody)
		outputEvents = append(outputEvents, modifiedEvent)
	}
	return outputEvents, nil
}

func (m *qwenProvider) TransformResponseBody(ctx wrapper.HttpContext, apiName ApiName, body []byte) ([]byte, error) {
	if m.config.qwenEnableCompatible {
		return body, nil
	}
	if apiName == ApiNameChatCompletion {
		return m.onChatCompletionResponseBody(ctx, body)
	}
	if apiName == ApiNameEmbeddings {
		return m.onEmbeddingsResponseBody(ctx, body)
	}
	if m.config.isSupportedAPI(apiName) {
		return body, nil
	}
	return nil, errUnsupportedApiName
}

func (m *qwenProvider) onChatCompletionResponseBody(ctx wrapper.HttpContext, body []byte) ([]byte, error) {
	qwenResponse := &qwenTextGenResponse{}
	if err := json.Unmarshal(body, qwenResponse); err != nil {
		return nil, fmt.Errorf("unable to unmarshal Qwen response: %v", err)
	}
	response := m.buildChatCompletionResponse(ctx, qwenResponse)
	return json.Marshal(response)
}

func (m *qwenProvider) onEmbeddingsResponseBody(ctx wrapper.HttpContext, body []byte) ([]byte, error) {
	qwenResponse := &qwenTextEmbeddingResponse{}
	if err := json.Unmarshal(body, qwenResponse); err != nil {
		return nil, fmt.Errorf("unable to unmarshal Qwen response: %v", err)
	}
	response := m.buildEmbeddingsResponse(ctx, qwenResponse)
	return json.Marshal(response)
}

func (m *qwenProvider) buildQwenTextGenerationRequest(ctx wrapper.HttpContext, origRequest *chatCompletionRequest, streaming bool) ([]byte, error) {
	messages := make([]qwenMessage, 0, len(origRequest.Messages))
	for i := range origRequest.Messages {
		messages = append(messages, chatMessage2QwenMessage(origRequest.Messages[i]))
	}
	request := &qwenTextGenRequest{
		Model: origRequest.Model,
		Input: qwenTextGenInput{
			Messages: messages,
		},
		Parameters: qwenTextGenParameters{
			ResultFormat:      qwenResultFormatMessage,
			MaxTokens:         origRequest.MaxTokens,
			N:                 origRequest.N,
			Seed:              origRequest.Seed,
			Temperature:       origRequest.Temperature,
			TopP:              math.Max(qwenTopPMin, math.Min(origRequest.TopP, qwenTopPMax)),
			IncrementalOutput: streaming && (origRequest.Tools == nil || len(origRequest.Tools) == 0),
			EnableSearch:      m.config.qwenEnableSearch,
			PreserveThinking:  chatMessagesHaveReasoningContent(origRequest.Messages),
			Tools:             origRequest.Tools,
		},
	}

	if streaming {
		ctx.SetContext(ctxKeyIncrementalStreaming, request.Parameters.IncrementalOutput)
	}

	if len(m.config.qwenFileIds) != 0 && origRequest.Model == qwenLongModelName {
		builder := strings.Builder{}
		for _, fileId := range m.config.qwenFileIds {
			if builder.Len() != 0 {
				builder.WriteRune(',')
			}
			builder.WriteString("fileid://")
			builder.WriteString(fileId)
		}

		body, err := json.Marshal(request)
		if err != nil {
			return nil, fmt.Errorf("unable to marshal request: %v", err)
		}

		return m.insertHttpContextMessage(body, builder.String(), true)
	}
	return json.Marshal(request)
}

func (m *qwenProvider) buildChatCompletionResponse(ctx wrapper.HttpContext, qwenResponse *qwenTextGenResponse) *chatCompletionResponse {
	choices := make([]chatCompletionChoice, 0, len(qwenResponse.Output.Choices))
	for _, qwenChoice := range qwenResponse.Output.Choices {
		message := qwenMessageToChatMessage(qwenChoice.Message, m.config.reasoningContentMode)
		choices = append(choices, chatCompletionChoice{
			Message:      &message,
			FinishReason: util.Ptr(qwenChoice.FinishReason),
		})
	}
	return &chatCompletionResponse{
		Id:                qwenResponse.RequestId,
		Created:           time.Now().UnixMilli() / 1000,
		Model:             ctx.GetStringContext(ctxKeyFinalRequestModel, ""),
		SystemFingerprint: "",
		Object:            objectChatCompletion,
		Choices:           choices,
		Usage: &usage{
			PromptTokens:     qwenResponse.Usage.InputTokens,
			CompletionTokens: qwenResponse.Usage.OutputTokens,
			TotalTokens:      qwenResponse.Usage.TotalTokens,
		},
	}
}

func (m *qwenProvider) buildChatCompletionStreamingResponse(ctx wrapper.HttpContext, qwenResponse *qwenTextGenResponse, incrementalStreaming bool) []*chatCompletionResponse {
	if len(qwenResponse.Output.Choices) == 0 {
		log.Warnf("qwen response has no choices, request_id: %s", qwenResponse.RequestId)
		return nil
	}

	baseMessage := chatCompletionResponse{
		Id:                qwenResponse.RequestId,
		Created:           time.Now().UnixMilli() / 1000,
		Model:             ctx.GetStringContext(ctxKeyFinalRequestModel, ""),
		Choices:           make([]chatCompletionChoice, 0),
		SystemFingerprint: "",
		Object:            objectChatCompletionChunk,
	}

	responses := make([]*chatCompletionResponse, 0)

	qwenChoice := qwenResponse.Output.Choices[0]
	// Yes, Qwen uses a string "null" as null.
	finished := qwenChoice.FinishReason != "" && qwenChoice.FinishReason != "null"
	message := qwenChoice.Message

	reasoningContentMode := m.config.reasoningContentMode

	log.Debugf("incrementalStreaming: %v", incrementalStreaming)
	deltaContentMessage := &chatMessage{Role: message.Role, Content: message.Content, ReasoningContent: message.ReasoningContent}
	deltaToolCallsMessage := &chatMessage{Role: message.Role, ToolCalls: append([]toolCall{}, message.ToolCalls...)}
	if incrementalStreaming {
		deltaContentMessage.handleStreamingReasoningContent(ctx, reasoningContentMode)
	} else {
		for _, tc := range message.ToolCalls {
			if tc.Function.Arguments == "" && !finished {
				// We don't push any tool call until its arguments are available.
				return nil
			}
		}
		if pushedMessage, ok := ctx.GetContext(ctxKeyPushedMessage).(qwenMessage); ok {
			if message.Content == "" {
				message.Content = pushedMessage.Content
			} else if message.IsStringContent() {
				deltaContentMessage.Content = util.StripPrefix(deltaContentMessage.StringContent(), pushedMessage.StringContent())
			} else if strings.HasPrefix(baseMessage.Model, qwenVlModelPrefixName) {
				// Use the Qwen multimodal model generation API
				deltaContentList, ok := deltaContentMessage.Content.([]qwenVlMessageContent)
				if !ok {
					log.Warnf("unexpected deltaContentMessage content type: %T", deltaContentMessage.Content)
				} else {
					pushedContentList, ok := pushedMessage.Content.([]qwenVlMessageContent)
					if !ok {
						log.Warnf("unexpected pushedMessage content type: %T", pushedMessage.Content)
					} else {
						for i, content := range deltaContentList {
							if i >= len(pushedContentList) {
								break
							}
							pushedText := pushedContentList[i].Text
							content.Text = util.StripPrefix(content.Text, pushedText)
							deltaContentList[i] = content
						}
					}
				}
			}
			if message.ReasoningContent == "" {
				message.ReasoningContent = pushedMessage.ReasoningContent
			} else {
				deltaContentMessage.ReasoningContent = util.StripPrefix(deltaContentMessage.ReasoningContent, pushedMessage.ReasoningContent)
			}
			deltaContentMessage.handleStreamingReasoningContent(ctx, reasoningContentMode)

			if len(deltaToolCallsMessage.ToolCalls) > 0 && pushedMessage.ToolCalls != nil {
				for i, tc := range deltaToolCallsMessage.ToolCalls {
					if i >= len(pushedMessage.ToolCalls) {
						break
					}
					pushedFunction := pushedMessage.ToolCalls[i].Function
					tc.Function.Id = util.StripPrefix(tc.Function.Id, pushedFunction.Id)
					tc.Function.Name = util.StripPrefix(tc.Function.Name, pushedFunction.Name)
					tc.Function.Arguments = util.StripPrefix(tc.Function.Arguments, pushedFunction.Arguments)
					deltaToolCallsMessage.ToolCalls[i] = tc
				}
			}
		}
		ctx.SetContext(ctxKeyPushedMessage, message)
	}

	if !deltaContentMessage.IsEmpty() {
		response := *&baseMessage
		response.Choices = append(response.Choices, chatCompletionChoice{Delta: deltaContentMessage})
		responses = append(responses, &response)
	}
	if !deltaToolCallsMessage.IsEmpty() {
		response := *&baseMessage
		response.Choices = append(response.Choices, chatCompletionChoice{Delta: deltaToolCallsMessage})
		responses = append(responses, &response)
	}

	if finished {
		finishResponse := *&baseMessage
		finishResponse.Choices = append(finishResponse.Choices, chatCompletionChoice{Delta: &chatMessage{}, FinishReason: util.Ptr(qwenChoice.FinishReason)})

		usageResponse := *&baseMessage
		usageResponse.Choices = []chatCompletionChoice{{Delta: &chatMessage{}}}
		usageResponse.Usage = &usage{
			PromptTokens:     qwenResponse.Usage.InputTokens,
			CompletionTokens: qwenResponse.Usage.OutputTokens,
			TotalTokens:      qwenResponse.Usage.TotalTokens,
		}

		responses = append(responses, &finishResponse, &usageResponse)
	}

	return responses
}

func (m *qwenProvider) insertHttpContextMessage(body []byte, content string, onlyOneSystemBeforeFile bool) ([]byte, error) {
	request := &qwenTextGenRequest{}
	if err := json.Unmarshal(body, request); err != nil {
		return nil, fmt.Errorf("unable to unmarshal request: %v", err)
	}

	fileMessage := qwenMessage{
		Role:    roleSystem,
		Content: content,
	}
	var firstNonSystemMessageIndex int
	messages := request.Input.Messages
	if messages != nil {
		for i, message := range request.Input.Messages {
			if message.Role != roleSystem {
				firstNonSystemMessageIndex = i
				break
			}
		}
	}
	if firstNonSystemMessageIndex == 0 {
		request.Input.Messages = append([]qwenMessage{fileMessage}, request.Input.Messages...)
	} else if !onlyOneSystemBeforeFile {
		request.Input.Messages = append(request.Input.Messages[:firstNonSystemMessageIndex], append([]qwenMessage{fileMessage}, request.Input.Messages[firstNonSystemMessageIndex:]...)...)
	} else {
		builder := strings.Builder{}
		for _, message := range request.Input.Messages[:firstNonSystemMessageIndex] {
			if builder.Len() != 0 {
				builder.WriteString("\n")
			}
			builder.WriteString(message.StringContent())
		}
		request.Input.Messages = append([]qwenMessage{{Role: roleSystem, Content: builder.String()}, fileMessage}, request.Input.Messages[firstNonSystemMessageIndex:]...)
		firstNonSystemMessageIndex = 1
	}

	if firstNonSystemMessageIndex == 0 {
		// The context message cannot come first. We need to add another dummy system message before it.
		request.Input.Messages = append([]qwenMessage{{Role: roleSystem, Content: qwenDummySystemMessageContent}}, request.Input.Messages...)
	}

	return json.Marshal(request)
}

func (m *qwenProvider) appendStreamEvent(responseBuilder *strings.Builder, event *StreamEvent) {
	responseBuilder.WriteString(streamDataItemKey)
	responseBuilder.WriteString(event.Data)
	responseBuilder.WriteString("\n\n")
}

func (m *qwenProvider) buildQwenTextEmbeddingRequest(request *embeddingsRequest) (*qwenTextEmbeddingRequest, error) {
	var texts []string
	if str, isString := request.Input.(string); isString {
		texts = []string{str}
	} else if strs, isArray := request.Input.([]interface{}); isArray {
		texts = make([]string, 0, len(strs))
		for _, item := range strs {
			if str, isString := item.(string); isString {
				texts = append(texts, str)
			} else {
				return nil, errors.New("unsupported input type in array: " + reflect.TypeOf(item).String())
			}
		}
	} else {
		return nil, errors.New("unsupported input type: " + reflect.TypeOf(request.Input).String())
	}
	return &qwenTextEmbeddingRequest{
		Model: request.Model,
		Input: qwenTextEmbeddingInput{
			Texts: texts,
		},
	}, nil
}

func (m *qwenProvider) buildEmbeddingsResponse(ctx wrapper.HttpContext, qwenResponse *qwenTextEmbeddingResponse) *embeddingsResponse {
	data := make([]embedding, 0, len(qwenResponse.Output.Embeddings))
	for _, qwenEmbedding := range qwenResponse.Output.Embeddings {
		data = append(data, embedding{
			Object:    "embedding",
			Index:     qwenEmbedding.TextIndex,
			Embedding: qwenEmbedding.Embedding,
		})
	}
	return &embeddingsResponse{
		Object: "list",
		Data:   data,
		Model:  ctx.GetContext(ctxKeyFinalRequestModel).(string),
		Usage: usage{
			PromptTokens: qwenResponse.Usage.TotalTokens,
			TotalTokens:  qwenResponse.Usage.TotalTokens,
		},
	}
}

type qwenTextGenRequest struct {
	Model      string                `json:"model"`
	Input      qwenTextGenInput      `json:"input"`
	Parameters qwenTextGenParameters `json:"parameters,omitempty"`
}

type qwenTextGenInput struct {
	Messages []qwenMessage `json:"messages"`
}

type qwenTextGenParameters struct {
	ResultFormat      string  `json:"result_format,omitempty"`
	MaxTokens         int     `json:"max_tokens,omitempty"`
	RepetitionPenalty float64 `json:"repetition_penalty,omitempty"`
	N                 int     `json:"n,omitempty"`
	Seed              int     `json:"seed,omitempty"`
	Temperature       float64 `json:"temperature,omitempty"`
	TopP              float64 `json:"top_p,omitempty"`
	IncrementalOutput bool    `json:"incremental_output,omitempty"`
	EnableSearch      bool    `json:"enable_search,omitempty"`
	PreserveThinking  bool    `json:"preserve_thinking,omitempty"`
	Tools             []tool  `json:"tools,omitempty"`
}

type qwenTextGenResponse struct {
	RequestId string            `json:"request_id"`
	Output    qwenTextGenOutput `json:"output"`
	Usage     qwenUsage         `json:"usage"`
}

type qwenTextGenOutput struct {
	FinishReason string              `json:"finish_reason"`
	Choices      []qwenTextGenChoice `json:"choices"`
}

type qwenTextGenChoice struct {
	FinishReason string      `json:"finish_reason"`
	Message      qwenMessage `json:"message"`
}

type qwenUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type qwenMessage struct {
	Name             string     `json:"name,omitempty"`
	Role             string     `json:"role"`
	Content          any        `json:"content"`
	ReasoningContent string     `json:"reasoning_content,omitempty"`
	ToolCalls        []toolCall `json:"tool_calls,omitempty"`
}

type qwenVlMessageContent struct {
	Image string `json:"image,omitempty"`
	Text  string `json:"text,omitempty"`
}

type qwenTextEmbeddingRequest struct {
	Model      string                      `json:"model"`
	Input      qwenTextEmbeddingInput      `json:"input"`
	Parameters qwenTextEmbeddingParameters `json:"parameters,omitempty"`
}

type qwenTextEmbeddingInput struct {
	Texts []string `json:"texts"`
}

type qwenTextEmbeddingParameters struct {
	TextType string `json:"text_type,omitempty"`
}

type qwenTextEmbeddingResponse struct {
	RequestId string                  `json:"request_id"`
	Output    qwenTextEmbeddingOutput `json:"output"`
	Usage     qwenUsage               `json:"usage"`
}

type qwenTextEmbeddingOutput struct {
	RequestId  string               `json:"request_id"`
	Embeddings []qwenTextEmbeddings `json:"embeddings"`
}

type qwenTextEmbeddings struct {
	TextIndex int       `json:"text_index"`
	Embedding []float64 `json:"embedding"`
}

func qwenMessageToChatMessage(qwenMessage qwenMessage, reasoningContentMode string) chatMessage {
	msg := chatMessage{
		Name:             qwenMessage.Name,
		Role:             qwenMessage.Role,
		Content:          qwenMessage.Content,
		ReasoningContent: qwenMessage.ReasoningContent,
		ToolCalls:        qwenMessage.ToolCalls,
	}
	msg.handleNonStreamingReasoningContent(reasoningContentMode)
	return msg
}

func (m *qwenMessage) IsStringContent() bool {
	_, ok := m.Content.(string)
	return ok
}

func (m *qwenMessage) StringContent() string {
	content, ok := m.Content.(string)
	if ok {
		return content
	}
	contentList, ok := m.Content.([]any)
	if ok {
		var contentStr string
		for _, contentItem := range contentList {
			contentMap, ok := contentItem.(map[string]any)
			if !ok {
				continue
			}
			if text, ok := contentMap["text"].(string); ok {
				contentStr += text
			}
		}
		return contentStr
	}
	return ""
}

func chatMessagesHaveReasoningContent(messages []chatMessage) bool {
	for _, message := range messages {
		if message.ReasoningContent != "" {
			return true
		}
	}
	return false
}

func requestBodyHasMessageReasoningContent(body []byte) bool {
	for _, message := range gjson.GetBytes(body, "messages").Array() {
		if message.Get("reasoning_content").String() != "" {
			return true
		}
	}
	return false
}

func chatMessage2QwenMessage(chatMessage chatMessage) qwenMessage {
	if chatMessage.IsStringContent() {
		return qwenMessage{
			Name:             chatMessage.Name,
			Role:             chatMessage.Role,
			Content:          chatMessage.StringContent(),
			ReasoningContent: chatMessage.ReasoningContent,
			ToolCalls:        chatMessage.ToolCalls,
		}
	} else {
		var contents []qwenVlMessageContent
		openaiContent := chatMessage.ParseContent()
		for _, part := range openaiContent {
			var content qwenVlMessageContent
			if part.Type == contentTypeText {
				content.Text = part.Text
			} else if part.Type == contentTypeImageUrl {
				content.Image = part.ImageUrl.Url
			}
			contents = append(contents, content)
		}
		return qwenMessage{
			Name:             chatMessage.Name,
			Role:             chatMessage.Role,
			Content:          contents,
			ReasoningContent: chatMessage.ReasoningContent,
			ToolCalls:        chatMessage.ToolCalls,
		}
	}
}

func (m *qwenProvider) GetApiName(path string) ApiName {
	switch {
	case strings.Contains(path, qwenChatCompletionPath),
		strings.Contains(path, qwenMultimodalGenerationPath),
		strings.Contains(path, qwenBailianPath),
		strings.Contains(path, qwenCompatibleChatCompletionPath):
		return ApiNameChatCompletion
	case strings.Contains(path, qwenTextEmbeddingPath),
		strings.Contains(path, qwenCompatibleTextEmbeddingPath):
		return ApiNameEmbeddings
	case strings.Contains(path, qwenCompatibleResponsesPath):
		return ApiNameResponses
	case strings.Contains(path, qwenAsyncAIGCPath):
		return ApiNameQwenAsyncAIGC
	case strings.Contains(path, qwenAsyncTaskPath):
		return ApiNameQwenAsyncTask
	case strings.Contains(path, qwenTextRerankPath),
		strings.Contains(path, qwenCompatibleTextRerankPath):
		return ApiNameQwenV1Rerank
	case strings.Contains(path, qwenCompatibleConversationsPath):
		return ApiNameQwenV1Conversations
	default:
		return ""
	}
}
