package provider

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-proxy/util"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

// hunyuanProvider is the provider for hunyuan AI service.

const (
	hunyuanDomain                 = "hunyuan.tencentcloudapi.com"
	hunyuanRequestPath            = "/"
	hunyuanChatCompletionTCAction = "ChatCompletions"

	// headers necessary for TC hunyuan api call:
	// ref: https://cloud.tencent.com/document/api/1729/105701, https://cloud.tencent.com/document/api/1729/101842
	actionKey        = "X-TC-Action"
	timestampKey     = "X-TC-Timestamp"
	authorizationKey = "Authorization"
	versionKey       = "X-TC-Version"
	versionValue     = "2023-09-01"
	hostKey          = "Host"

	ssePrefix            = "data: " // Server-Sent Events (SSE) 类型的流式响应的开始标记
	hunyuanStreamEndMark = "stop"   // 混元的流式的finishReason为stop时，表示结束

	hunyuanAuthKeyLen = 32
	hunyuanAuthIdLen  = 36

	// docs: https://cloud.tencent.com/document/product/1729/111007
	hunyuanOpenAiDomain = "api.hunyuan.cloud.tencent.com"
)

type hunyuanProviderInitializer struct{}

// ref: https://console.cloud.tencent.com/api/explorer?Product=hunyuan&Version=2023-09-01&Action=ChatCompletions
type hunyuanTextGenRequest struct {
	Model             string               `json:"Model"`
	Messages          []hunyuanChatMessage `json:"Messages"`
	Stream            bool                 `json:"Stream,omitempty"`
	StreamModeration  bool                 `json:"StreamModeration,omitempty"`
	TopP              float32              `json:"TopP,omitempty"`
	Temperature       float32              `json:"Temperature,omitempty"`
	EnableEnhancement bool                 `json:"EnableEnhancement,omitempty"`
}

type hunyuanTextGenResponseNonStreaming struct {
	Response hunyuanTextGenDetailedResponseNonStreaming `json:"Response"`
}

type hunyuanTextGenDetailedResponseNonStreaming struct {
	RequestId string                 `json:"RequestId,omitempty"`
	Note      string                 `json:"Note"`
	Choices   []hunyuanTextGenChoice `json:"Choices"`
	Created   int64                  `json:"Created"`
	Id        string                 `json:"Id"`
	Usage     hunyuanTextGenUsage    `json:"Usage"`
}

type hunyuanTextGenChoice struct {
	FinishReason string             `json:"FinishReason"`
	Message      hunyuanChatMessage `json:"Message,omitempty"` // 当非流式返回时存储大模型生成文字
	Delta        hunyuanChatMessage `json:"Delta,omitempty"`   // 流式返回时存储大模型生成文字
}

type hunyuanTextGenUsage struct {
	PromptTokens     int `json:"PromptTokens"`
	CompletionTokens int `json:"CompletionTokens"`
	TotalTokens      int `json:"TotalTokens"`
}

type hunyuanChatMessage struct {
	Role    string `json:"Role,omitempty"`
	Content string `json:"Content,omitempty"`
}

func (m *hunyuanProviderInitializer) ValidateConfig(config *ProviderConfig) error {
	// 允许 hunyuanauthid 和 hunyuanauthkey 为空, 当他们都为空的时候，认为是使用openai的 兼容接口
	if len(config.hunyuanAuthId) == 0 && len(config.hunyuanAuthKey) == 0 {
		return nil
	}
	// 校验hunyuan id 和 key的合法性
	if len(config.hunyuanAuthId) != hunyuanAuthIdLen || len(config.hunyuanAuthKey) != hunyuanAuthKeyLen {
		return errors.New("hunyuanAuthId / hunyuanAuthKey is illegal in config file")
	}
	return nil
}

func (m *hunyuanProviderInitializer) DefaultCapabilities() map[string]string {
	return map[string]string{
		string(ApiNameChatCompletion): PathOpenAIChatCompletions,
		string(ApiNameEmbeddings):     PathOpenAIEmbeddings,
	}
}

func (m *hunyuanProviderInitializer) CreateProvider(config ProviderConfig) (Provider, error) {
	config.setDefaultCapabilities(m.DefaultCapabilities())
	return &hunyuanProvider{
		config: config,
		client: wrapper.NewClusterClient(wrapper.RouteCluster{
			Host: hunyuanDomain,
		}),
		contextCache: createContextCache(&config),
	}, nil
}

type hunyuanProvider struct {
	config ProviderConfig

	client       wrapper.HttpClient
	contextCache *contextCache
}

func (m *hunyuanProvider) GetProviderType() string {
	return providerTypeHunyuan
}

func (m *hunyuanProvider) useOpenAICompatibleAPI() bool {
	return len(m.config.hunyuanAuthId) == 0 && len(m.config.hunyuanAuthKey) == 0
}

func (m *hunyuanProvider) OnRequestHeaders(ctx wrapper.HttpContext, apiName ApiName) error {
	m.config.handleRequestHeaders(m, ctx, apiName)
	// Delay the header processing to allow changing streaming mode in OnRequestBody
	return nil
}

func (m *hunyuanProvider) TransformRequestHeaders(ctx wrapper.HttpContext, apiName ApiName, headers http.Header) {
	if m.useOpenAICompatibleAPI() {
		util.OverwriteRequestHostHeader(headers, hunyuanOpenAiDomain)
		util.OverwriteRequestPathHeaderByCapability(headers, string(apiName), m.config.capabilities)
		util.OverwriteRequestAuthorizationHeader(headers, "Bearer "+m.config.GetApiTokenInUse(ctx))
	} else {
		util.OverwriteRequestHostHeader(headers, hunyuanDomain)
		util.OverwriteRequestPathHeader(headers, hunyuanRequestPath)
		// 添加 hunyuan 需要的自定义字段
		headers.Set(actionKey, hunyuanChatCompletionTCAction)
		headers.Set(versionKey, versionValue)
	}
}

// hunyuan 的 OnRequestBody 逻辑中包含了对 headers 签名的逻辑，并且插入 context 以后还要重新计算签名，因此无法复用 handleRequestBody 方法
func (m *hunyuanProvider) OnRequestBody(ctx wrapper.HttpContext, apiName ApiName, body []byte) (types.Action, error) {
	if !m.config.isSupportedAPI(apiName) {
		return types.ActionContinue, errUnsupportedApiName
	}
	if m.useOpenAICompatibleAPI() {
		return types.ActionContinue, nil
	}

	// 为header添加时间戳字段 （因为需要根据body进行签名时依赖时间戳，故于body处理部分创建时间戳）
	var timestamp int64 = time.Now().Unix()
	_ = proxywasm.ReplaceHttpRequestHeader(timestampKey, fmt.Sprintf("%d", timestamp))

	// 使用混元本身接口的协议
	if m.config.protocol == protocolOriginal {
		request := &hunyuanTextGenRequest{}

		if err := json.Unmarshal(body, request); err != nil {
			return types.ActionContinue, fmt.Errorf("unable to unmarshal request: %v", err)
		}

		// 根据确定好的payload进行签名
		hunyuanBody, _ := json.Marshal(request)
		authorizedValueNew := GetTC3Authorizationcode(m.config.hunyuanAuthId, m.config.hunyuanAuthKey, timestamp, hunyuanDomain, hunyuanChatCompletionTCAction, string(hunyuanBody))
		_ = util.OverwriteRequestAuthorization(authorizedValueNew)
		_ = proxywasm.ReplaceHttpRequestHeader("Accept", "*/*")
		// log.Debugf("#debug nash5# OnRequestBody call hunyuan api using original api! signature computation done!")

		// 若无配置文件，直接返回
		if m.config.context == nil {
			return types.ActionContinue, replaceJsonRequestBody(request)
		}
		err := m.contextCache.GetContent(func(content string, err error) {
			log.Debugf("#debug nash5# ctx file loaded! callback start, content is: %s", content)
			defer func() {
				_ = proxywasm.ResumeHttpRequest()
			}()

			if err != nil {
				log.Errorf("failed to load context file: %v", err)
				util.ErrorHandler("ai-proxy.hunyuan.load_ctx_failed", fmt.Errorf("failed to load context file: %v", err))
			}
			m.insertContextMessageIntoHunyuanRequest(request, content)

			// 因为手动插入了context内容，这里需要重新计算签名
			hunyuanBody, _ := json.Marshal(request)
			authorizedValueNew := GetTC3Authorizationcode(m.config.hunyuanAuthId, m.config.hunyuanAuthKey, timestamp, hunyuanDomain, hunyuanChatCompletionTCAction, string(hunyuanBody))
			_ = util.OverwriteRequestAuthorization(authorizedValueNew)

			if err := replaceJsonRequestBody(request); err != nil {
				util.ErrorHandler("ai-proxy.hunyuan.insert_ctx_failed", fmt.Errorf("failed to replace request body: %v", err))
			}
		})
		if err == nil {
			log.Debugf("#debug nash5# ctx file load success!")
			return types.ActionPause, nil
		}

		log.Debugf("#debug nash5# ctx file load failed!")
		return types.ActionContinue, replaceJsonRequestBody(request)
	}

	// 使用open ai接口协议
	request := &chatCompletionRequest{}
	if err := decodeChatCompletionRequest(body, request); err != nil {
		return types.ActionContinue, err
	}

	model := request.Model
	if model == "" {
		return types.ActionContinue, errors.New("missing model in chat completion request")
	}
	ctx.SetContext(ctxKeyOriginalRequestModel, model) // 设置原始请求的model，以便返回值使用
	mappedModel := getMappedModel(model, m.config.modelMapping)
	if mappedModel == "" {
		return types.ActionContinue, errors.New("model becomes empty after applying the configured mapping")
	}
	request.Model = mappedModel
	ctx.SetContext(ctxKeyFinalRequestModel, request.Model) // 设置真实请求的模型，以便返回值使用

	// 看请求中的stream的设置，相应的我们更该http头
	streaming := request.Stream
	if streaming {
		_ = proxywasm.ReplaceHttpRequestHeader("Accept", "text/event-stream")
	} else {
		_ = proxywasm.ReplaceHttpRequestHeader("Accept", "*/*")
	}

	// 若没有配置上下文，直接开始请求
	if m.config.context == nil {
		hunyuanRequest := m.buildHunyuanTextGenerationRequest(request)

		// 根据确定好的payload进行签名：
		body, _ := json.Marshal(hunyuanRequest)
		authorizedValueNew := GetTC3Authorizationcode(
			m.config.hunyuanAuthId,
			m.config.hunyuanAuthKey,
			timestamp,
			hunyuanDomain,
			hunyuanChatCompletionTCAction,
			string(body),
		)
		_ = util.OverwriteRequestAuthorization(authorizedValueNew)
		return types.ActionContinue, replaceJsonRequestBody(hunyuanRequest)
	}

	err := m.contextCache.GetContent(func(content string, err error) {
		defer func() {
			_ = proxywasm.ResumeHttpRequest()
		}()
		if err != nil {
			log.Errorf("failed to load context file: %v", err)
			util.ErrorHandler("ai-proxy.hunyuan.load_ctx_failed", fmt.Errorf("failed to load context file: %v", err))
			return
		}
		insertContextMessage(request, content)
		hunyuanRequest := m.buildHunyuanTextGenerationRequest(request)

		// 因为手动插入了context内容，这里需要重新计算签名
		hunyuanBody, _ := json.Marshal(hunyuanRequest)
		authorizedValueNew := GetTC3Authorizationcode(m.config.hunyuanAuthId, m.config.hunyuanAuthKey, timestamp, hunyuanDomain, hunyuanChatCompletionTCAction, string(hunyuanBody))
		_ = util.OverwriteRequestAuthorization(authorizedValueNew)

		if err := replaceJsonRequestBody(hunyuanRequest); err != nil {
			util.ErrorHandler("ai-proxy.hunyuan.insert_ctx_failed", fmt.Errorf("failed to replace request body: %v", err))
		}
	})
	if err == nil {
		return types.ActionPause, nil
	}
	return types.ActionContinue, err
}

// hunyuan 的 TransformRequestBodyHeaders 方法只在 failover 健康检查的时候会调用
func (m *hunyuanProvider) TransformRequestBodyHeaders(ctx wrapper.HttpContext, apiName ApiName, body []byte, headers http.Header) ([]byte, error) {
	if m.useOpenAICompatibleAPI() {
		return m.config.defaultTransformRequestBody(ctx, apiName, body)
	}
	request := &chatCompletionRequest{}
	err := m.config.parseRequestAndMapModel(ctx, request, body)
	if err != nil {
		return nil, err
	}

	hunyuanRequest := m.buildHunyuanTextGenerationRequest(request)

	var timestamp int64 = time.Now().Unix()
	_ = proxywasm.ReplaceHttpRequestHeader(timestampKey, fmt.Sprintf("%d", timestamp))
	// 根据确定好的payload进行签名：
	body, _ = json.Marshal(hunyuanRequest)
	authorizedValueNew := GetTC3Authorizationcode(
		m.config.hunyuanAuthId,
		m.config.hunyuanAuthKey,
		timestamp,
		hunyuanDomain,
		hunyuanChatCompletionTCAction,
		string(body),
	)
	util.OverwriteRequestAuthorizationHeader(headers, authorizedValueNew)
	return json.Marshal(hunyuanRequest)
}

func (m *hunyuanProvider) OnStreamingResponseBody(ctx wrapper.HttpContext, name ApiName, chunk []byte, isLastChunk bool) ([]byte, error) {
	if m.config.IsOriginal() || m.useOpenAICompatibleAPI() || name != ApiNameChatCompletion {
		return chunk, nil
	}

	// hunyuan的流式返回:
	// data: {"Note":"以上内容为AI生成，不代表开发者立场，请勿删除或修改本标记","Choices":[{"Delta":{"Role":"assistant","Content":"有助于"},"FinishReason":""}],"Created":1716359713,"Id":"086b6b19-8b2c-4def-a65c-db6a7bc86acd","Usage":{"PromptTokens":7,"CompletionTokens":145,"TotalTokens":152}}

	// openai的流式返回
	// data: {"id": "chatcmpl-7QyqpwdfhqwajicIEznoc6Q47XAyW", "object": "chat.completion.chunk", "created": 1677664795, "model": "gpt-3.5-turbo-0613", "choices": [{"delta": {"content": "The "}, "index": 0, "finish_reason": null}]}

	// log.Debugf("#debug nash5# [OnStreamingResponseBody] chunk is: %s", string(chunk))

	// 从上下文获取现有缓冲区数据
	newBufferedBody := chunk
	if bufferedBody, has := ctx.GetContext(ctxKeyStreamingBody).([]byte); has {
		newBufferedBody = append(bufferedBody, chunk...)
	}

	// 初始化处理下标，以及将要返回的处理过的chunks
	newEventPivot := -1
	var outputBuffer []byte

	// 从buffer区取出若干完整的chunk，将其转为openAI格式后返回
	// 处理可能包含多个事件的缓冲区
	for {
		eventStartIndex := bytes.Index(newBufferedBody, []byte(ssePrefix))
		if eventStartIndex == -1 {
			break // 没有找到新事件，跳出循环
		}

		// 移除缓冲区前面非事件部分
		newBufferedBody = newBufferedBody[eventStartIndex+len(ssePrefix):]

		// 查找事件结束的位置（即下一个事件的开始）
		newEventPivot = bytes.Index(newBufferedBody, []byte("\n\n"))
		var eventData []byte
		if newEventPivot == -1 {
			if !isLastChunk {
				// 未找到事件结束标识，跳出循环等待更多数据，若是最后一个chunk，不一定有2个换行符
				break
			}
			eventData = bytes.TrimSpace(newBufferedBody)
			newBufferedBody = nil
		} else {
			// 提取并处理一个完整的事件
			eventData = newBufferedBody[:newEventPivot]
			// log.Debugf("@@@ <<< ori chun is: %s", string(newBufferedBody[:newEventPivot]))
			newBufferedBody = newBufferedBody[newEventPivot+2:] // 跳过结束标识
		}
		if len(eventData) == 0 {
			continue
		}

		// 转换并追加到输出缓冲区
		convertedData, _ := m.convertChunkFromHunyuanToOpenAI(ctx, eventData)
		// log.Debugf("@@@ >>> converted one chunk: %s", string(convertedData))
		outputBuffer = append(outputBuffer, convertedData...)
	}

	// 刷新剩余的不完整事件回到上下文缓冲区以便下次继续处理
	ctx.SetContext(ctxKeyStreamingBody, newBufferedBody)

	log.Debugf("=== modified response chunk: %s", string(outputBuffer))
	return outputBuffer, nil
}

func (m *hunyuanProvider) convertChunkFromHunyuanToOpenAI(ctx wrapper.HttpContext, hunyuanChunk []byte) ([]byte, error) {
	// 将hunyuan的chunk转为openai的chunk
	hunyuanFormattedChunk := &hunyuanTextGenDetailedResponseNonStreaming{}
	if err := json.Unmarshal(hunyuanChunk, hunyuanFormattedChunk); err != nil {
		return []byte(""), nil
	}
	if len(hunyuanFormattedChunk.Choices) == 0 {
		return []byte(""), nil
	}

	openAIFormattedChunk := &chatCompletionResponse{
		Id:                hunyuanFormattedChunk.Id,
		Created:           time.Now().UnixMilli() / 1000,
		Model:             ctx.GetStringContext(ctxKeyFinalRequestModel, ""),
		SystemFingerprint: "",
		Object:            objectChatCompletionChunk,
		Usage: &usage{
			PromptTokens:     hunyuanFormattedChunk.Usage.PromptTokens,
			CompletionTokens: hunyuanFormattedChunk.Usage.CompletionTokens,
			TotalTokens:      hunyuanFormattedChunk.Usage.TotalTokens,
		},
	}
	// tmpStr3, _ := json.Marshal(hunyuanFormattedChunk)
	// log.Debugf("@@@ --- 源数据是：: %s", tmpStr3)

	// 是否为最后一个chunk？
	if hunyuanFormattedChunk.Choices[0].FinishReason == hunyuanStreamEndMark {
		// log.Debugf("@@@ --- 最后chunk: ")
		openAIFormattedChunk.Choices = append(openAIFormattedChunk.Choices, chatCompletionChoice{
			FinishReason: util.Ptr(hunyuanFormattedChunk.Choices[0].FinishReason),
		})
	} else {
		deltaMsg := chatMessage{
			Name:      "",
			Role:      hunyuanFormattedChunk.Choices[0].Delta.Role,
			Content:   hunyuanFormattedChunk.Choices[0].Delta.Content,
			ToolCalls: []toolCall{},
		}

		// tmpStr2, _ := json.Marshal(deltaMsg)
		// log.Debugf("@@@ --- 中间chunk: choices.chatMsg 是: %s", tmpStr2)

		openAIFormattedChunk.Choices = append(
			openAIFormattedChunk.Choices,
			chatCompletionChoice{Delta: &deltaMsg},
		)
		// tmpStr, _ := json.Marshal(openAIFormattedChunk.Choices)
		// log.Debugf("@@@ --- 中间chunk: choices 是: %s", tmpStr)
	}

	// 返回的格式
	openAIFormattedChunkBytes, _ := json.Marshal(openAIFormattedChunk)
	var openAIChunk strings.Builder
	openAIChunk.WriteString(ssePrefix)
	openAIChunk.WriteString(string(openAIFormattedChunkBytes))
	openAIChunk.WriteString("\n\n")

	return []byte(openAIChunk.String()), nil
}

func (m *hunyuanProvider) TransformResponseBody(ctx wrapper.HttpContext, apiName ApiName, body []byte) ([]byte, error) {
	if m.config.IsOriginal() || m.useOpenAICompatibleAPI() {
		return body, nil
	}
	if apiName != ApiNameChatCompletion {
		return body, nil
	}
	log.Debugf("#debug nash5# onRespBody's resp is: %s", string(body))
	hunyuanResponse := &hunyuanTextGenResponseNonStreaming{}
	if err := json.Unmarshal(body, hunyuanResponse); err != nil {
		return nil, fmt.Errorf("unable to unmarshal hunyuan response: %v", err)
	}
	response := m.buildChatCompletionResponse(ctx, hunyuanResponse)
	return json.Marshal(response)
}

func (m *hunyuanProvider) insertContextMessageIntoHunyuanRequest(request *hunyuanTextGenRequest, content string) {
	fileMessage := hunyuanChatMessage{
		Role:    roleSystem,
		Content: content,
	}
	messages := request.Messages
	request.Messages = append([]hunyuanChatMessage{},
		append([]hunyuanChatMessage{fileMessage}, messages...)...,
	)
}

func (m *hunyuanProvider) buildHunyuanTextGenerationRequest(request *chatCompletionRequest) *hunyuanTextGenRequest {
	hunyuanRequest := &hunyuanTextGenRequest{
		Model:             request.Model,
		Messages:          convertMessagesFromOpenAIToHunyuan(request.Messages),
		Stream:            request.Stream,
		StreamModeration:  false,
		TopP:              float32(request.TopP),
		Temperature:       float32(request.Temperature),
		EnableEnhancement: false,
	}

	return hunyuanRequest
}

func convertMessagesFromOpenAIToHunyuan(openAIMessages []chatMessage) []hunyuanChatMessage {
	// 将chatgpt的messages转换为hunyuan的messages
	hunyuanChatMessages := make([]hunyuanChatMessage, 0, len(openAIMessages))
	for _, msg := range openAIMessages {
		hunyuanChatMessages = append(hunyuanChatMessages, hunyuanChatMessage{
			Role:    msg.Role,
			Content: msg.StringContent(),
		})
	}

	return hunyuanChatMessages
}

func (m *hunyuanProvider) buildChatCompletionResponse(ctx wrapper.HttpContext, hunyuanResponse *hunyuanTextGenResponseNonStreaming) *chatCompletionResponse {
	choices := make([]chatCompletionChoice, 0, len(hunyuanResponse.Response.Choices))
	for _, choice := range hunyuanResponse.Response.Choices {
		choices = append(choices, chatCompletionChoice{
			Message: &chatMessage{
				Name:      "",
				Role:      choice.Message.Role,
				Content:   choice.Message.Content,
				ToolCalls: nil,
			},
			FinishReason: util.Ptr(choice.FinishReason),
		})
	}
	return &chatCompletionResponse{
		Id:                hunyuanResponse.Response.Id,
		Created:           time.Now().UnixMilli() / 1000,
		Model:             ctx.GetStringContext(ctxKeyFinalRequestModel, ""),
		SystemFingerprint: "",
		Object:            objectChatCompletion,
		Choices:           choices,
		Usage: &usage{
			PromptTokens:     hunyuanResponse.Response.Usage.PromptTokens,
			CompletionTokens: hunyuanResponse.Response.Usage.CompletionTokens,
			TotalTokens:      hunyuanResponse.Response.Usage.TotalTokens,
		},
	}
}

func Sha256hex(s string) string {
	b := sha256.Sum256([]byte(s))
	return hex.EncodeToString(b[:])
}

func Hmacsha256(s, key string) string {
	hashed := hmac.New(sha256.New, []byte(key))
	hashed.Write([]byte(s))
	return string(hashed.Sum(nil))
}

/**
 * @param secretId 秘钥id
 * @param secretKey 秘钥
 * @param timestamp 时间戳
 * @param host 目标域名
 * @param action 请求动作
 * @param payload 请求体
 * @return 签名
 */
func GetTC3Authorizationcode(secretId string, secretKey string, timestamp int64, host string, action string, payload string) string {
	algorithm := "TC3-HMAC-SHA256"
	service := "hunyuan" // 注意，必须和域名中的产品名保持一致

	// step 1: build canonical request string
	httpRequestMethod := "POST"
	canonicalURI := "/"
	canonicalQueryString := ""
	canonicalHeaders := fmt.Sprintf("content-type:%s\nhost:%s\nx-tc-action:%s\n",
		"application/json", host, strings.ToLower(action))
	signedHeaders := "content-type;host;x-tc-action"

	// fmt.Println("payload is: %s", payload)
	hashedRequestPayload := Sha256hex(payload)
	canonicalRequest := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s",
		httpRequestMethod,
		canonicalURI,
		canonicalQueryString,
		canonicalHeaders,
		signedHeaders,
		hashedRequestPayload)
	// fmt.Println(canonicalRequest)

	// step 2: build string to sign
	date := time.Unix(timestamp, 0).UTC().Format("2006-01-02")
	credentialScope := fmt.Sprintf("%s/%s/tc3_request", date, service)
	hashedCanonicalRequest := Sha256hex(canonicalRequest)
	string2sign := fmt.Sprintf("%s\n%d\n%s\n%s",
		algorithm,
		timestamp,
		credentialScope,
		hashedCanonicalRequest)
	// fmt.Println(string2sign)

	// step 3: sign string
	secretDate := Hmacsha256(date, "TC3"+secretKey)
	secretService := Hmacsha256(service, secretDate)
	secretSigning := Hmacsha256("tc3_request", secretService)
	signature := hex.EncodeToString([]byte(Hmacsha256(string2sign, secretSigning)))
	// fmt.Println(signature)

	// step 4: build authorization
	authorization := fmt.Sprintf("%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		algorithm,
		secretId,
		credentialScope,
		signedHeaders,
		signature)

	// curl := fmt.Sprintf(`curl -X POST https://%s \
	// 	-H "Authorization: %s" \
	// 	-H "Content-Type: application/json" \
	// 	-H "Host: %s" -H "X-TC-Action: %s" \
	// 	-H "X-TC-Timestamp: %d" \
	// 	-H "X-TC-Version: 2023-09-01" \
	// 	-d '%s'`, host, authorization, host, action, timestamp, payload)
	// fmt.Println(curl)
	return authorization
}

func (m *hunyuanProvider) GetApiName(path string) ApiName {
	return ApiNameChatCompletion
}
