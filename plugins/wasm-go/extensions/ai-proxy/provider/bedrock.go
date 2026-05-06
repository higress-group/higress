package provider

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-proxy/util"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

const (
	httpPostMethod = "POST"
	awsService     = "bedrock"
	// bedrock-runtime.{awsRegion}.amazonaws.com
	bedrockDefaultDomain = "bedrock-runtime.%s.amazonaws.com"
	// converse路径 /model/{modelId}/converse
	bedrockChatCompletionPath = "/model/%s/converse"
	// converseStream路径 /model/{modelId}/converse-stream
	bedrockStreamChatCompletionPath = "/model/%s/converse-stream"
	// invoke_model 路径 /model/{modelId}/invoke
	bedrockInvokeModelPath   = "/model/%s/invoke"
	bedrockSignedHeaders     = "host;x-amz-date"
	requestIdHeader          = "X-Amzn-Requestid"
	bedrockCacheTypeDefault  = "default"
	bedrockCacheTTL5m        = "5m"
	bedrockCacheTTL1h        = "1h"
	bedrockPromptCacheNova   = "amazon.nova"
	bedrockPromptCacheClaude = "anthropic.claude"

	bedrockCachePointPositionSystemPrompt    = "systemPrompt"
	bedrockCachePointPositionLastUserMessage = "lastUserMessage"
	bedrockCachePointPositionLastMessage     = "lastMessage"

	ctxKeyBedrockToolCallState = "bedrock_tool_call_state"
)

var (
	bedrockConversePathPattern = regexp.MustCompile(`/model/[^/]+/converse(-stream)?$`)
	bedrockInvokePathPattern   = regexp.MustCompile(`/model/[^/]+/invoke(-with-response-stream)?$`)
)

type bedrockProviderInitializer struct{}

func (b *bedrockProviderInitializer) ValidateConfig(config *ProviderConfig) error {
	hasAkSk := len(config.awsAccessKey) > 0 && len(config.awsSecretKey) > 0
	hasApiToken := len(config.apiTokens) > 0

	if !hasAkSk && !hasApiToken {
		return errors.New("missing bedrock access authentication parameters: either apiTokens or (awsAccessKey + awsSecretKey) is required")
	}
	if len(config.awsRegion) == 0 {
		return errors.New("missing bedrock region parameters")
	}
	return nil
}

func (b *bedrockProviderInitializer) DefaultCapabilities() map[string]string {
	return map[string]string{
		string(ApiNameChatCompletion):  bedrockChatCompletionPath,
		string(ApiNameImageGeneration): bedrockInvokeModelPath,
	}
}

func (b *bedrockProviderInitializer) CreateProvider(config ProviderConfig) (Provider, error) {
	config.setDefaultCapabilities(b.DefaultCapabilities())
	return &bedrockProvider{
		config:       config,
		contextCache: createContextCache(&config),
	}, nil
}

type bedrockProvider struct {
	config       ProviderConfig
	contextCache *contextCache
}

func (b *bedrockProvider) OnStreamingResponseBody(ctx wrapper.HttpContext, name ApiName, chunk []byte, isLastChunk bool) ([]byte, error) {
	events := extractAmazonEventStreamEvents(ctx, chunk)
	if len(events) == 0 {
		if isLastChunk {
			return []byte(ssePrefix + "[DONE]\n\n"), nil
		}
		return chunk, fmt.Errorf("No events are extracted ")
	}
	var responseBuilder strings.Builder
	for _, event := range events {
		outputEvent, err := b.convertEventFromBedrockToOpenAI(ctx, event)
		if err != nil {
			log.Errorf("[onStreamingResponseBody] failed to process streaming event: %v\n%s", err, chunk)
			return chunk, err
		}
		responseBuilder.WriteString(string(outputEvent))
	}
	if isLastChunk {
		responseBuilder.WriteString(ssePrefix + "[DONE]\n\n")
	}
	return []byte(responseBuilder.String()), nil
}

func (b *bedrockProvider) convertEventFromBedrockToOpenAI(ctx wrapper.HttpContext, bedrockEvent ConverseStreamEvent) ([]byte, error) {
	choices := make([]chatCompletionChoice, 0)
	chatChoice := chatCompletionChoice{
		Delta: &chatMessage{},
	}
	if bedrockEvent.Role != nil {
		chatChoice.Delta.Role = *bedrockEvent.Role
	}
	if bedrockEvent.Start != nil {
		toolCallIndex := getBedrockOpenAIToolCallIndex(ctx, bedrockEvent.ContentBlockIndex)
		chatChoice.Delta.Content = nil
		chatChoice.Delta.ToolCalls = []toolCall{
			{
				Index: toolCallIndex,
				Id:    bedrockEvent.Start.ToolUse.ToolUseID,
				Type:  "function",
				Function: functionCall{
					Name:      bedrockEvent.Start.ToolUse.Name,
					Arguments: "",
				},
			},
		}
	}
	if bedrockEvent.Delta != nil {
		if bedrockEvent.Delta.ReasoningContent != nil {
			var content string
			if ctx.GetContext("thinking_start") == nil {
				content += reasoningStartTag
				ctx.SetContext("thinking_start", true)
			}
			content += bedrockEvent.Delta.ReasoningContent.Text
			chatChoice.Delta = &chatMessage{Content: &content}
		} else if bedrockEvent.Delta.Text != nil {
			var content string
			if ctx.GetContext("thinking_start") != nil && ctx.GetContext("thinking_end") == nil {
				content += reasoningEndTag
				ctx.SetContext("thinking_end", true)
			}
			content += *bedrockEvent.Delta.Text
			chatChoice.Delta = &chatMessage{Content: &content}
		}
		if bedrockEvent.Delta.ToolUse != nil {
			toolCallIndex := getBedrockOpenAIToolCallIndex(ctx, bedrockEvent.ContentBlockIndex)
			chatChoice.Delta.ToolCalls = []toolCall{
				{
					Index: toolCallIndex,
					Type:  "function",
					Function: functionCall{
						Arguments: bedrockEvent.Delta.ToolUse.Input,
					},
				},
			}
		}
	}
	if bedrockEvent.StopReason != nil {
		chatChoice.FinishReason = util.Ptr(stopReasonBedrock2OpenAI(*bedrockEvent.StopReason))
	}
	choices = append(choices, chatChoice)
	requestId := ctx.GetStringContext(requestIdHeader, "")
	openAIFormattedChunk := &chatCompletionResponse{
		Id:                requestId,
		Created:           time.Now().UnixMilli() / 1000,
		Model:             ctx.GetStringContext(ctxKeyFinalRequestModel, ""),
		SystemFingerprint: "",
		Object:            objectChatCompletion,
		Choices:           choices,
	}
	if bedrockEvent.Usage != nil {
		openAIFormattedChunk.Choices = choices[:0]
		openAIFormattedChunk.Usage = &usage{
			CompletionTokens:    bedrockEvent.Usage.OutputTokens,
			PromptTokens:        bedrockEvent.Usage.InputTokens,
			TotalTokens:         bedrockEvent.Usage.TotalTokens,
			PromptTokensDetails: buildPromptTokensDetails(bedrockEvent.Usage.CacheReadInputTokens, bedrockEvent.Usage.CacheWriteInputTokens),
		}
	}
	openAIFormattedChunkBytes, _ := json.Marshal(openAIFormattedChunk)
	var openAIChunk strings.Builder
	openAIChunk.WriteString(ssePrefix)
	openAIChunk.WriteString(string(openAIFormattedChunkBytes))
	openAIChunk.WriteString("\n\n")
	return []byte(openAIChunk.String()), nil
}

type bedrockToolCallState struct {
	indexes map[int]int
	next    int
}

func getBedrockOpenAIToolCallIndex(ctx wrapper.HttpContext, contentBlockIndex int) int {
	state, _ := ctx.GetContext(ctxKeyBedrockToolCallState).(*bedrockToolCallState)
	if state == nil {
		state = &bedrockToolCallState{indexes: make(map[int]int)}
		ctx.SetContext(ctxKeyBedrockToolCallState, state)
	}

	if toolCallIndex, ok := state.indexes[contentBlockIndex]; ok {
		return toolCallIndex
	}

	toolCallIndex := state.next
	state.indexes[contentBlockIndex] = toolCallIndex
	state.next++
	return toolCallIndex
}

type ConverseStreamEvent struct {
	ContentBlockIndex int                                   `json:"contentBlockIndex,omitempty"`
	Delta             *converseStreamEventContentBlockDelta `json:"delta,omitempty"`
	Role              *string                               `json:"role,omitempty"`
	StopReason        *string                               `json:"stopReason,omitempty"`
	Usage             *tokenUsage                           `json:"usage,omitempty"`
	Start             *contentBlockStart                    `json:"start,omitempty"`
}

type converseStreamEventContentBlockDelta struct {
	Text             *string                `json:"text,omitempty"`
	ToolUse          *toolUseBlockDelta     `json:"toolUse,omitempty"`
	ReasoningContent *reasoningContentDelta `json:"reasoningContent,omitempty"`
}

type toolUseBlockStart struct {
	Name      string `json:"name"`
	ToolUseID string `json:"toolUseId"`
}

type contentBlockStart struct {
	ToolUse *toolUseBlockStart `json:"toolUse,omitempty"`
}

type toolUseBlockDelta struct {
	Input string `json:"input"`
}

type reasoningContentDelta struct {
	Text      string `json:"text,omitempty"`
	Signature string `json:"signature,omitempty"`
}

type bedrockImageGenerationResponse struct {
	Images []string `json:"images"`
	Error  string   `json:"error"`
}

type bedrockImageGenerationTextToImageParams struct {
	Text            string  `json:"text"`
	NegativeText    string  `json:"negativeText,omitempty"`
	ConditionImage  string  `json:"conditionImage,omitempty"`
	ControlMode     string  `json:"controlMode,omitempty"`
	ControlStrength float32 `json:"controlLength,omitempty"`
}

type bedrockImageGenerationConfig struct {
	Width          int     `json:"width"`
	Height         int     `json:"height"`
	Quality        string  `json:"quality,omitempty"`
	CfgScale       float32 `json:"cfgScale,omitempty"`
	Seed           int     `json:"seed,omitempty"`
	NumberOfImages int     `json:"numberOfImages"`
}

type bedrockImageGenerationColorGuidedGenerationParams struct {
	Colors         []string `json:"colors"`
	ReferenceImage string   `json:"referenceImage"`
	Text           string   `json:"text"`
	NegativeText   string   `json:"negativeText,omitempty"`
}

type bedrockImageGenerationImageVariationParams struct {
	Images             []string `json:"images"`
	SimilarityStrength float32  `json:"similarityStrength"`
	Text               string   `json:"text"`
	NegativeText       string   `json:"negativeText,omitempty"`
}

type bedrockImageGenerationInPaintingParams struct {
	Image        string `json:"image"`
	MaskPrompt   string `json:"maskPrompt"`
	MaskImage    string `json:"maskImage"`
	Text         string `json:"text"`
	NegativeText string `json:"negativeText,omitempty"`
}

type bedrockImageGenerationOutPaintingParams struct {
	Image           string `json:"image"`
	MaskPrompt      string `json:"maskPrompt"`
	MaskImage       string `json:"maskImage"`
	OutPaintingMode string `json:"outPaintingMode"`
	Text            string `json:"text"`
	NegativeText    string `json:"negativeText,omitempty"`
}

type bedrockImageGenerationBackgroundRemovalParams struct {
	Image string `json:"image"`
}

type bedrockImageGenerationRequest struct {
	TaskType                    string                                             `json:"taskType"`
	ImageGenerationConfig       *bedrockImageGenerationConfig                      `json:"imageGenerationConfig"`
	TextToImageParams           *bedrockImageGenerationTextToImageParams           `json:"textToImageParams,omitempty"`
	ColorGuidedGenerationParams *bedrockImageGenerationColorGuidedGenerationParams `json:"colorGuidedGenerationParams,omitempty"`
	ImageVariationParams        *bedrockImageGenerationImageVariationParams        `json:"imageVariationParams,omitempty"`
	InPaintingParams            *bedrockImageGenerationInPaintingParams            `json:"inPaintingParams,omitempty"`
	OutPaintingParams           *bedrockImageGenerationOutPaintingParams           `json:"outPaintingParams,omitempty"`
	BackgroundRemovalParams     *bedrockImageGenerationBackgroundRemovalParams     `json:"backgroundRemovalParams,omitempty"`
}

func extractAmazonEventStreamEvents(ctx wrapper.HttpContext, chunk []byte) []ConverseStreamEvent {
	body := chunk
	if bufferedStreamingBody, has := ctx.GetContext(ctxKeyStreamingBody).([]byte); has {
		body = append(bufferedStreamingBody, chunk...)
	}

	r := bytes.NewReader(body)
	var events []ConverseStreamEvent
	var lastRead int64 = 0
	messageBuffer := make([]byte, 1024)
	defer func() {
		log.Infof("extractAmazonEventStreamEvents: lastRead=%d, r.Size=%d", lastRead, r.Size())
	}()

	for {
		msg, err := decodeMessage(r, messageBuffer)
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Errorf("failed to decode message: %v", err)
			break
		}
		var event ConverseStreamEvent
		if err = json.Unmarshal(msg.Payload, &event); err == nil {
			events = append(events, event)
		}
		lastRead = r.Size() - int64(r.Len())
	}
	if lastRead < int64(len(body)) {
		ctx.SetContext(ctxKeyStreamingBody, body[lastRead:])
	} else {
		ctx.SetContext(ctxKeyStreamingBody, nil)
	}
	return events
}

type bedrockStreamMessage struct {
	Headers headers
	Payload []byte
}

type EventFrame struct {
	TotalLength   uint32
	HeadersLength uint32
	PreludeCRC    uint32
	Headers       map[string]interface{}
	Payload       []byte
	PayloadCRC    uint32
}

type headers []header

type header struct {
	Name  string
	Value Value
}

func (hs *headers) Set(name string, value Value) {
	var i int
	for ; i < len(*hs); i++ {
		if (*hs)[i].Name == name {
			(*hs)[i].Value = value
			return
		}
	}

	*hs = append(*hs, header{
		Name: name, Value: value,
	})
}

func decodeMessage(reader io.Reader, payloadBuf []byte) (m bedrockStreamMessage, err error) {
	crc := crc32.New(crc32.MakeTable(crc32.IEEE))
	hashReader := io.TeeReader(reader, crc)

	prelude, err := decodePrelude(hashReader, crc)
	if err != nil {
		return bedrockStreamMessage{}, err
	}

	if prelude.HeadersLen > 0 {
		lr := io.LimitReader(hashReader, int64(prelude.HeadersLen))
		m.Headers, err = decodeHeaders(lr)
		if err != nil {
			return bedrockStreamMessage{}, err
		}
	}

	if payloadLen := prelude.PayloadLen(); payloadLen > 0 {
		buf, err := decodePayload(payloadBuf, io.LimitReader(hashReader, int64(payloadLen)))
		if err != nil {
			return bedrockStreamMessage{}, err
		}
		m.Payload = buf
	}

	msgCRC := crc.Sum32()
	if err := validateCRC(reader, msgCRC); err != nil {
		return bedrockStreamMessage{}, err
	}

	return m, nil
}

func decodeHeaders(r io.Reader) (headers, error) {
	hs := headers{}

	for {
		name, err := decodeHeaderName(r)
		if err != nil {
			if err == io.EOF {
				// EOF while getting header name means no more headers
				break
			}
			return nil, err
		}

		value, err := decodeHeaderValue(r)
		if err != nil {
			return nil, err
		}

		hs.Set(name, value)
	}

	return hs, nil
}

func decodeHeaderValue(r io.Reader) (Value, error) {
	var raw rawValue

	typ, err := decodeUint8(r)
	if err != nil {
		return nil, err
	}
	raw.Type = valueType(typ)

	var v Value

	switch raw.Type {
	case stringValueType:
		var tv StringValue
		err = tv.decode(r)
		v = tv
	default:
		log.Errorf("unknown value type %d", raw.Type)
	}

	// Error could be EOF, let caller deal with it
	return v, err
}

type Value interface {
	Get() interface{}
}

type StringValue string

func (v StringValue) Get() interface{} {
	return string(v)
}

func (v *StringValue) decode(r io.Reader) error {
	s, err := decodeStringValue(r)
	if err != nil {
		return err
	}

	*v = StringValue(s)
	return nil
}

func decodeBytesValue(r io.Reader) ([]byte, error) {
	var raw rawValue
	var err error
	raw.Len, err = decodeUint16(r)
	if err != nil {
		return nil, err
	}

	buf := make([]byte, raw.Len)
	_, err = io.ReadFull(r, buf)
	if err != nil {
		return nil, err
	}

	return buf, nil
}

func decodeUint16(r io.Reader) (uint16, error) {
	var b [2]byte
	bs := b[:]
	_, err := io.ReadFull(r, bs)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(bs), nil
}

func decodeStringValue(r io.Reader) (string, error) {
	v, err := decodeBytesValue(r)
	return string(v), err
}

type rawValue struct {
	Type  valueType
	Len   uint16 // Only set for variable length slices
	Value []byte // byte representation of value, BigEndian encoding.
}

type valueType uint8

const (
	trueValueType valueType = iota
	falseValueType
	int8ValueType  // Byte
	int16ValueType // Short
	int32ValueType // Integer
	int64ValueType // Long
	bytesValueType
	stringValueType
	timestampValueType
	uuidValueType
)

func decodeHeaderName(r io.Reader) (string, error) {
	var n headerName

	var err error
	n.Len, err = decodeUint8(r)
	if err != nil {
		return "", err
	}

	name := n.Name[:n.Len]
	if _, err := io.ReadFull(r, name); err != nil {
		return "", err
	}

	return string(name), nil
}

func decodeUint8(r io.Reader) (uint8, error) {
	type byteReader interface {
		ReadByte() (byte, error)
	}

	if br, ok := r.(byteReader); ok {
		v, err := br.ReadByte()
		return v, err
	}

	var b [1]byte
	_, err := io.ReadFull(r, b[:])
	return b[0], err
}

const maxHeaderNameLen = 255

type headerName struct {
	Len  uint8
	Name [maxHeaderNameLen]byte
}

func decodePayload(buf []byte, r io.Reader) ([]byte, error) {
	w := bytes.NewBuffer(buf[0:0])

	_, err := io.Copy(w, r)
	return w.Bytes(), err
}

type messagePrelude struct {
	Length     uint32
	HeadersLen uint32
	PreludeCRC uint32
}

func (p messagePrelude) ValidateLens() error {
	if p.Length == 0 {
		return fmt.Errorf("message prelude want: 16, have: %v", int(p.Length))
	}
	return nil
}

func (p messagePrelude) PayloadLen() uint32 {
	return p.Length - p.HeadersLen - 16
}

func decodePrelude(r io.Reader, crc hash.Hash32) (messagePrelude, error) {
	var p messagePrelude

	var err error
	p.Length, err = decodeUint32(r)
	if err != nil {
		return messagePrelude{}, err
	}

	p.HeadersLen, err = decodeUint32(r)
	if err != nil {
		return messagePrelude{}, err
	}

	if err := p.ValidateLens(); err != nil {
		return messagePrelude{}, err
	}

	preludeCRC := crc.Sum32()
	if err := validateCRC(r, preludeCRC); err != nil {
		return messagePrelude{}, err
	}

	p.PreludeCRC = preludeCRC

	return p, nil
}

func decodeUint32(r io.Reader) (uint32, error) {
	var b [4]byte
	bs := b[:]
	_, err := io.ReadFull(r, bs)
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(bs), nil
}

func validateCRC(r io.Reader, expect uint32) error {
	msgCRC, err := decodeUint32(r)
	if err != nil {
		return err
	}

	if msgCRC != expect {
		return fmt.Errorf("message checksum mismatch")
	}

	return nil
}

func (b *bedrockProvider) TransformResponseHeaders(ctx wrapper.HttpContext, apiName ApiName, headers http.Header) {
	ctx.SetContext(requestIdHeader, headers.Get(requestIdHeader))
	if headers.Get("Content-Type") == "application/vnd.amazon.eventstream" {
		headers.Set("Content-Type", "text/event-stream; charset=utf-8")
	}
	headers.Del("Content-Length")
}

func (b *bedrockProvider) GetProviderType() string {
	return providerTypeBedrock
}

func (b *bedrockProvider) GetApiName(path string) ApiName {
	switch {
	case bedrockConversePathPattern.MatchString(path):
		return ApiNameChatCompletion
	case bedrockInvokePathPattern.MatchString(path):
		return ApiNameImageGeneration
	default:
		return ""
	}
}

func (b *bedrockProvider) OnRequestHeaders(ctx wrapper.HttpContext, apiName ApiName) error {
	b.config.handleRequestHeaders(b, ctx, apiName)
	return nil
}

func (b *bedrockProvider) TransformRequestHeaders(ctx wrapper.HttpContext, apiName ApiName, headers http.Header) {
	util.OverwriteRequestHostHeader(headers, fmt.Sprintf(bedrockDefaultDomain, strings.TrimSpace(b.config.awsRegion)))

	// If apiTokens is configured, set Bearer token authentication here
	// This follows the same pattern as other providers (qwen, zhipuai, etc.)
	// AWS SigV4 authentication is handled in setAuthHeaders because it requires the request body
	if len(b.config.apiTokens) > 0 {
		util.OverwriteRequestAuthorizationHeader(headers, "Bearer "+b.config.GetApiTokenInUse(ctx))
	}
}

func (b *bedrockProvider) OnRequestBody(ctx wrapper.HttpContext, apiName ApiName, body []byte) (types.Action, error) {
	// In original protocol mode (e.g. /model/{modelId}/converse-stream), keep the body/path untouched
	// and only apply auth headers.
	if b.config.IsOriginal() {
		headers := util.GetRequestHeaders()
		b.setAuthHeaders(body, headers)
		util.ReplaceRequestHeaders(headers)
		return types.ActionContinue, replaceRequestBody(body)
	}

	if !b.config.isSupportedAPI(apiName) {
		return types.ActionContinue, errUnsupportedApiName
	}
	return b.config.handleRequestBody(b, b.contextCache, ctx, apiName, body)
}

func (b *bedrockProvider) TransformRequestBodyHeaders(ctx wrapper.HttpContext, apiName ApiName, body []byte, headers http.Header) ([]byte, error) {
	var transformedBody []byte
	var err error
	switch apiName {
	case ApiNameChatCompletion:
		transformedBody, err = b.onChatCompletionRequestBody(ctx, body, headers)
	case ApiNameImageGeneration:
		transformedBody, err = b.onImageGenerationRequestBody(ctx, body, headers)
	default:
		transformedBody, err = b.config.defaultTransformRequestBody(ctx, apiName, body)
	}

	if err != nil {
		return nil, err
	}

	// Always apply auth after request body/path are finalized.
	// For Bearer token mode this is a no-op; for AK/SK mode this generates SigV4 headers.
	b.setAuthHeaders(transformedBody, headers)
	return transformedBody, nil
}

func (b *bedrockProvider) TransformResponseBody(ctx wrapper.HttpContext, apiName ApiName, body []byte) ([]byte, error) {
	switch apiName {
	case ApiNameChatCompletion:
		return b.onChatCompletionResponseBody(ctx, body)
	case ApiNameImageGeneration:
		return b.onImageGenerationResponseBody(body)
	}
	return nil, errUnsupportedApiName
}

func (b *bedrockProvider) onImageGenerationResponseBody(body []byte) ([]byte, error) {
	bedrockResponse := &bedrockImageGenerationResponse{}
	if err := json.Unmarshal(body, bedrockResponse); err != nil {
		log.Errorf("unable to unmarshal bedrock image gerneration response: %v", err)
		return nil, fmt.Errorf("unable to unmarshal bedrock image generation response: %v", err)
	}
	response := b.buildBedrockImageGenerationResponse(bedrockResponse)
	return json.Marshal(response)
}

func (b *bedrockProvider) onImageGenerationRequestBody(ctx wrapper.HttpContext, body []byte, headers http.Header) ([]byte, error) {
	request := &imageGenerationRequest{}
	err := b.config.parseRequestAndMapModel(ctx, request, body)
	if err != nil {
		return nil, err
	}
	headers.Set("Accept", "*/*")
	b.overwriteRequestPathHeader(headers, bedrockInvokeModelPath, request.Model)
	return b.buildBedrockImageGenerationRequest(request, headers)
}

func (b *bedrockProvider) buildBedrockImageGenerationRequest(origRequest *imageGenerationRequest, headers http.Header) ([]byte, error) {
	width, height := 1024, 1024
	pairs := strings.Split(origRequest.Size, "x")
	if len(pairs) == 2 {
		width, _ = strconv.Atoi(pairs[0])
		height, _ = strconv.Atoi(pairs[1])
	}

	request := &bedrockImageGenerationRequest{
		TaskType: "TEXT_IMAGE",
		TextToImageParams: &bedrockImageGenerationTextToImageParams{
			Text: origRequest.Prompt,
		},
		ImageGenerationConfig: &bedrockImageGenerationConfig{
			NumberOfImages: origRequest.N,
			Width:          width,
			Height:         height,
			Quality:        origRequest.Quality,
		},
	}
	return json.Marshal(request)
}

func (b *bedrockProvider) buildBedrockImageGenerationResponse(bedrockResponse *bedrockImageGenerationResponse) *imageGenerationResponse {
	data := make([]imageGenerationData, len(bedrockResponse.Images))
	for i, image := range bedrockResponse.Images {
		data[i] = imageGenerationData{
			B64: image,
		}
	}
	return &imageGenerationResponse{
		Created: time.Now().UnixMilli() / 1000,
		Data:    data,
	}
}

func (b *bedrockProvider) onChatCompletionResponseBody(ctx wrapper.HttpContext, body []byte) ([]byte, error) {
	bedrockResponse := &bedrockConverseResponse{}
	if err := json.Unmarshal(body, bedrockResponse); err != nil {
		log.Errorf("unable to unmarshal bedrock response: %v", err)
		return nil, fmt.Errorf("unable to unmarshal bedrock response: %v", err)
	}
	response := b.buildChatCompletionResponse(ctx, bedrockResponse)
	return json.Marshal(response)
}

func (b *bedrockProvider) onChatCompletionRequestBody(ctx wrapper.HttpContext, body []byte, headers http.Header) ([]byte, error) {
	request := &chatCompletionRequest{}
	err := b.config.parseRequestAndMapModel(ctx, request, body)
	if err != nil {
		return nil, err
	}

	streaming := request.Stream
	headers.Set("Accept", "*/*")
	if streaming {
		b.overwriteRequestPathHeader(headers, bedrockStreamChatCompletionPath, request.Model)
	} else {
		b.overwriteRequestPathHeader(headers, bedrockChatCompletionPath, request.Model)
	}
	return b.buildBedrockTextGenerationRequest(request, headers)
}

func (b *bedrockProvider) buildBedrockTextGenerationRequest(origRequest *chatCompletionRequest, headers http.Header) ([]byte, error) {
	messages := make([]bedrockMessage, 0, len(origRequest.Messages))
	systemMessages := make([]systemContentBlock, 0)

	for _, msg := range origRequest.Messages {
		switch msg.Role {
		case roleSystem:
			systemMessages = append(systemMessages, systemContentBlock{Text: msg.StringContent()})
		case roleTool:
			toolResultContent := chatToolMessage2BedrockToolResultContent(msg)
			if len(messages) > 0 && messages[len(messages)-1].Role == roleUser && messages[len(messages)-1].Content[0].ToolResult != nil {
				messages[len(messages)-1].Content = append(messages[len(messages)-1].Content, toolResultContent)
			} else {
				messages = append(messages, bedrockMessage{
					Role:    roleUser,
					Content: []bedrockMessageContent{toolResultContent},
				})
			}
		default:
			messages = append(messages, chatMessage2BedrockMessage(msg))
		}
	}

	request := &bedrockTextGenRequest{
		System:   systemMessages,
		Messages: messages,
		InferenceConfig: bedrockInferenceConfig{
			MaxTokens:   origRequest.getMaxTokens(),
			Temperature: origRequest.Temperature,
			TopP:        origRequest.TopP,
		},
		AdditionalModelRequestFields: make(map[string]interface{}),
		PerformanceConfig: PerformanceConfiguration{
			Latency: "standard",
		},
	}

	effectivePromptCacheRetention := b.resolvePromptCacheRetention(origRequest.PromptCacheRetention)

	if origRequest.PromptCacheKey != "" {
		log.Warnf("bedrock provider ignores prompt_cache_key because Converse API has no equivalent field")
	}
	if isPromptCacheSupportedModel(origRequest.Model) {
		if cacheTTL, ok := mapPromptCacheRetentionToBedrockTTL(effectivePromptCacheRetention); ok {
			addPromptCachePointsToBedrockRequest(request, cacheTTL, b.getPromptCachePointPositions())
		}
	} else if effectivePromptCacheRetention != "" {
		log.Warnf("skip prompt cache injection for unsupported model: %s", origRequest.Model)
	}

	if origRequest.ReasoningEffort != "" {
		thinkingBudget := 1024 // default
		switch origRequest.ReasoningEffort {
		case "low":
			thinkingBudget = 1024
		case "medium":
			thinkingBudget = 4096
		case "high":
			thinkingBudget = 16384
		}
		request.AdditionalModelRequestFields["thinking"] = map[string]interface{}{
			"type":          "enabled",
			"budget_tokens": thinkingBudget,
		}
	}

	if origRequest.Tools != nil && origRequest.getToolChoiceType() != "none" {
		request.ToolConfig = &bedrockToolConfig{}
		request.ToolConfig.ToolChoice.Auto = &struct{}{}
		if choice_type := origRequest.getToolChoiceType(); choice_type != "" {
			switch choice_type {
			// "any" is accepted for direct Anthropic-compatible callers; OpenAI
			// uses "required" for the same "must call at least one tool" behavior.
			case "required", "any":
				request.ToolConfig.ToolChoice.Auto = nil
				request.ToolConfig.ToolChoice.Any = &struct{}{}
			case "auto":
				request.ToolConfig.ToolChoice.Auto = &struct{}{}
			case "function":
				if choice := origRequest.getToolChoiceObject(); choice != nil && choice.Function.Name != "" {
					request.ToolConfig.ToolChoice.Auto = nil
					request.ToolConfig.ToolChoice.Tool = &bedrockSpecificToolChoice{
						Name: choice.Function.Name,
					}
				}
			}
		}
		request.ToolConfig.Tools = []bedrockTool{}
		for _, tool := range origRequest.Tools {
			request.ToolConfig.Tools = append(request.ToolConfig.Tools, bedrockTool{
				ToolSpec: bedrockToolSpecification{
					InputSchema: bedrockToolInputSchemaJson{Json: tool.Function.Parameters},
					Name:        tool.Function.Name,
					Description: tool.Function.Description,
				},
			})
		}
	}

	for key, value := range b.config.bedrockAdditionalFields {
		request.AdditionalModelRequestFields[key] = value
	}

	return json.Marshal(request)
}

func (b *bedrockProvider) buildChatCompletionResponse(ctx wrapper.HttpContext, bedrockResponse *bedrockConverseResponse) *chatCompletionResponse {
	var outputContent, reasoningContent, normalContent string
	for _, content := range bedrockResponse.Output.Message.Content {
		if content.ReasoningContent != nil {
			reasoningContent = content.ReasoningContent.ReasoningText.Text
		}
		if content.Text != "" {
			normalContent = content.Text
		}
	}
	if reasoningContent != "" {
		outputContent = reasoningStartTag + reasoningContent + reasoningEndTag + normalContent
	} else {
		outputContent = normalContent
	}
	choice := chatCompletionChoice{
		Index: 0,
		Message: &chatMessage{
			Role:    bedrockResponse.Output.Message.Role,
			Content: outputContent,
		},
		FinishReason: util.Ptr(stopReasonBedrock2OpenAI(bedrockResponse.StopReason)),
	}
	choice.Message.ToolCalls = []toolCall{}
	for _, content := range bedrockResponse.Output.Message.Content {
		if content.ToolUse != nil {
			args, _ := json.Marshal(content.ToolUse.Input)
			choice.Message.ToolCalls = append(choice.Message.ToolCalls, toolCall{
				Id:   content.ToolUse.ToolUseId,
				Type: "function",
				Function: functionCall{
					Name:      content.ToolUse.Name,
					Arguments: string(args),
				},
			})
		}
	}
	choices := []chatCompletionChoice{choice}
	requestId := ctx.GetStringContext(requestIdHeader, "")
	modelId, _ := url.QueryUnescape(ctx.GetStringContext(ctxKeyFinalRequestModel, ""))
	return &chatCompletionResponse{
		Id:                requestId,
		Created:           time.Now().UnixMilli() / 1000,
		Model:             modelId,
		SystemFingerprint: "",
		Object:            objectChatCompletion,
		Choices:           choices,
		Usage: &usage{
			PromptTokens:        bedrockResponse.Usage.InputTokens,
			CompletionTokens:    bedrockResponse.Usage.OutputTokens,
			TotalTokens:         bedrockResponse.Usage.TotalTokens,
			PromptTokensDetails: buildPromptTokensDetails(bedrockResponse.Usage.CacheReadInputTokens, bedrockResponse.Usage.CacheWriteInputTokens),
		},
	}
}

func (b *bedrockProvider) overwriteRequestPathHeader(headers http.Header, format, model string) {
	modelInPath := model
	// Just in case the model name has already been URL-escaped, we shouldn't escape it again.
	if !strings.ContainsRune(model, '%') {
		modelInPath = url.QueryEscape(model)
	}
	path := fmt.Sprintf(format, modelInPath)
	log.Debugf("overwriting bedrock request path: %s", path)
	util.OverwriteRequestPathHeader(headers, path)
}

func stopReasonBedrock2OpenAI(reason string) string {
	switch reason {
	case "end_turn":
		return finishReasonStop
	case "stop_sequence":
		return finishReasonStop
	case "max_tokens":
		return finishReasonLength
	case "tool_use":
		return finishReasonToolCall
	default:
		return reason
	}
}

func mapPromptCacheRetentionToBedrockTTL(retention string) (string, bool) {
	normalizedRetention := normalizePromptCacheRetention(retention)
	switch normalizedRetention {
	case "":
		return "", false
	case "in_memory":
		// For the default 5-minute cache, omit ttl and let Bedrock apply its default.
		// This is more robust for models that are strict about explicit ttl fields.
		return "", true
	case "24h":
		return bedrockCacheTTL1h, true
	default:
		log.Warnf("unsupported prompt_cache_retention for bedrock mapping: %s", retention)
		return "", false
	}
}

func normalizePromptCacheRetention(retention string) string {
	normalized := strings.ToLower(strings.TrimSpace(retention))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	normalized = strings.ReplaceAll(normalized, " ", "_")
	if normalized == "inmemory" {
		return "in_memory"
	}
	return normalized
}

func isPromptCacheSupportedModel(model string) bool {
	normalizedModel := strings.ToLower(strings.TrimSpace(model))
	return strings.Contains(normalizedModel, bedrockPromptCacheNova) ||
		strings.Contains(normalizedModel, bedrockPromptCacheClaude)
}

func (b *bedrockProvider) resolvePromptCacheRetention(requestPromptCacheRetention string) string {
	if requestPromptCacheRetention != "" {
		return requestPromptCacheRetention
	}
	if b.config.promptCacheRetention != "" {
		return b.config.promptCacheRetention
	}
	return ""
}

func (b *bedrockProvider) getPromptCachePointPositions() map[string]bool {
	if b.config.bedrockPromptCachePointPositions == nil {
		return map[string]bool{
			bedrockCachePointPositionSystemPrompt: true,
			bedrockCachePointPositionLastMessage:  false,
		}
	}
	positions := map[string]bool{
		bedrockCachePointPositionSystemPrompt:    false,
		bedrockCachePointPositionLastUserMessage: false,
		bedrockCachePointPositionLastMessage:     false,
	}
	for rawKey, enabled := range b.config.bedrockPromptCachePointPositions {
		key := normalizeBedrockCachePointPosition(rawKey)
		switch key {
		case bedrockCachePointPositionSystemPrompt, bedrockCachePointPositionLastUserMessage, bedrockCachePointPositionLastMessage:
			positions[key] = enabled
		default:
			log.Warnf("unsupported bedrockPromptCachePointPositions key: %s", rawKey)
		}
	}
	return positions
}

func normalizeBedrockCachePointPosition(raw string) string {
	key := strings.ToLower(raw)
	key = strings.ReplaceAll(key, "_", "")
	key = strings.ReplaceAll(key, "-", "")
	switch key {
	case "systemprompt":
		return bedrockCachePointPositionSystemPrompt
	case "lastusermessage":
		return bedrockCachePointPositionLastUserMessage
	case "lastmessage":
		return bedrockCachePointPositionLastMessage
	default:
		return raw
	}
}

func addPromptCachePointsToBedrockRequest(request *bedrockTextGenRequest, cacheTTL string, positions map[string]bool) {
	if positions[bedrockCachePointPositionSystemPrompt] && len(request.System) > 0 {
		request.System = append(request.System, systemContentBlock{
			CachePoint: &bedrockCachePoint{
				Type: bedrockCacheTypeDefault,
				TTL:  cacheTTL,
			},
		})
	}

	lastUserMessageIndex := -1
	if positions[bedrockCachePointPositionLastUserMessage] {
		lastUserMessageIndex = findLastMessageIndexByRole(request.Messages, roleUser)
		if lastUserMessageIndex >= 0 {
			appendCachePointToBedrockMessage(request, lastUserMessageIndex, cacheTTL)
		}
	}
	if positions[bedrockCachePointPositionLastMessage] && len(request.Messages) > 0 {
		lastMessageIndex := len(request.Messages) - 1
		if lastMessageIndex != lastUserMessageIndex {
			appendCachePointToBedrockMessage(request, lastMessageIndex, cacheTTL)
		}
	}
}

func findLastMessageIndexByRole(messages []bedrockMessage, role string) int {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == role {
			return i
		}
	}
	return -1
}

func appendCachePointToBedrockMessage(request *bedrockTextGenRequest, messageIndex int, cacheTTL string) {
	if messageIndex < 0 || messageIndex >= len(request.Messages) {
		return
	}
	request.Messages[messageIndex].Content = append(request.Messages[messageIndex].Content, bedrockMessageContent{
		CachePoint: &bedrockCachePoint{
			Type: bedrockCacheTypeDefault,
			TTL:  cacheTTL,
		},
	})
}

func buildPromptTokensDetails(cacheReadInputTokens int, cacheWriteInputTokens int) *promptTokensDetails {
	totalCachedTokens := cacheReadInputTokens + cacheWriteInputTokens
	if totalCachedTokens <= 0 {
		return nil
	}
	return &promptTokensDetails{
		CachedTokens: totalCachedTokens,
	}
}

type bedrockTextGenRequest struct {
	Messages                     []bedrockMessage         `json:"messages"`
	System                       []systemContentBlock     `json:"system,omitempty"`
	InferenceConfig              bedrockInferenceConfig   `json:"inferenceConfig,omitempty"`
	AdditionalModelRequestFields map[string]interface{}   `json:"additionalModelRequestFields,omitempty"`
	PerformanceConfig            PerformanceConfiguration `json:"performanceConfig,omitempty"`
	ToolConfig                   *bedrockToolConfig       `json:"toolConfig,omitempty"`
}

type bedrockToolConfig struct {
	Tools      []bedrockTool     `json:"tools,omitempty"`
	ToolChoice bedrockToolChoice `json:"toolChoice,omitempty"`
}

type PerformanceConfiguration struct {
	Latency string `json:"latency,omitempty"`
}

type bedrockTool struct {
	ToolSpec bedrockToolSpecification `json:"toolSpec,omitempty"`
}

type bedrockToolChoice struct {
	Any  *struct{}                  `json:"any,omitempty"`
	Auto *struct{}                  `json:"auto,omitempty"`
	Tool *bedrockSpecificToolChoice `json:"tool,omitempty"`
}

type bedrockSpecificToolChoice struct {
	Name string `json:"name"`
}

type bedrockToolSpecification struct {
	InputSchema bedrockToolInputSchemaJson `json:"inputSchema,omitempty"`
	Name        string                     `json:"name"`
	Description string                     `json:"description,omitempty"`
}

type bedrockToolInputSchemaJson struct {
	Json map[string]interface{} `json:"json,omitempty"`
}

type bedrockMessage struct {
	Role    string                  `json:"role"`
	Content []bedrockMessageContent `json:"content"`
}

type bedrockMessageContent struct {
	Text       string             `json:"text,omitempty"`
	Image      *imageBlock        `json:"image,omitempty"`
	ToolResult *toolResultBlock   `json:"toolResult,omitempty"`
	ToolUse    *toolUseBlock      `json:"toolUse,omitempty"`
	CachePoint *bedrockCachePoint `json:"cachePoint,omitempty"`
}

type systemContentBlock struct {
	Text       string             `json:"text,omitempty"`
	CachePoint *bedrockCachePoint `json:"cachePoint,omitempty"`
}

type bedrockCachePoint struct {
	Type string `json:"type"`
	TTL  string `json:"ttl,omitempty"`
}

type imageBlock struct {
	Format string      `json:"format,omitempty"`
	Source imageSource `json:"source,omitempty"`
}

type imageSource struct {
	Bytes string `json:"bytes,omitempty"`
}

type toolResultBlock struct {
	ToolUseId string                   `json:"toolUseId"`
	Content   []toolResultContentBlock `json:"content"`
	Status    string                   `json:"status,omitempty"`
}

type toolResultContentBlock struct {
	Text string `json:"text"`
}

type toolUseBlock struct {
	Input     map[string]interface{} `json:"input"`
	Name      string                 `json:"name"`
	ToolUseId string                 `json:"toolUseId"`
}

type bedrockInferenceConfig struct {
	StopSequences []string `json:"stopSequences,omitempty"`
	MaxTokens     int      `json:"maxTokens,omitempty"`
	Temperature   float64  `json:"temperature,omitempty"`
	TopP          float64  `json:"topP,omitempty"`
}

type bedrockConverseResponse struct {
	Metrics    converseMetrics             `json:"metrics"`
	Output     converseOutputMemberMessage `json:"output"`
	StopReason string                      `json:"stopReason"`
	Usage      tokenUsage                  `json:"usage"`
}

type converseMetrics struct {
	LatencyMs int `json:"latencyMs"`
}

type converseOutputMemberMessage struct {
	Message message `json:"message"`
}

type message struct {
	Content []contentBlock `json:"content"`
	Role    string         `json:"role"`
}

type contentBlock struct {
	Text             string            `json:"text,omitempty"`
	ToolUse          *bedrockToolUse   `json:"toolUse,omitempty"`
	ReasoningContent *reasoningContent `json:"reasoningContent,omitempty"`
}

type reasoningContent struct {
	ReasoningText reasoningText `json:"reasoningText"`
}

type reasoningText struct {
	Text      string `json:"text,omitempty"`
	Signature string `json:"signature,omitempty"`
}

type bedrockToolUse struct {
	Name      string                 `json:"name"`
	ToolUseId string                 `json:"toolUseId"`
	Input     map[string]interface{} `json:"input"`
}

type tokenUsage struct {
	InputTokens int `json:"inputTokens,omitempty"`

	OutputTokens int `json:"outputTokens,omitempty"`

	TotalTokens int `json:"totalTokens"`

	CacheReadInputTokens int `json:"cacheReadInputTokens,omitempty"`

	CacheWriteInputTokens int `json:"cacheWriteInputTokens,omitempty"`
}

func chatToolMessage2BedrockToolResultContent(chatMessage chatMessage) bedrockMessageContent {
	toolResultContent := &toolResultBlock{}
	toolResultContent.ToolUseId = chatMessage.ToolCallId
	if text, ok := chatMessage.Content.(string); ok {
		toolResultContent.Content = []toolResultContentBlock{
			{
				Text: text,
			},
		}
	} else if contentList, ok := chatMessage.Content.([]any); ok {
		for _, contentItem := range contentList {
			contentMap, ok := contentItem.(map[string]any)
			if ok && contentMap["type"] == contentTypeText {
				if text, ok := contentMap[contentTypeText].(string); ok {
					toolResultContent.Content = append(toolResultContent.Content, toolResultContentBlock{
						Text: text,
					})
				}
			}
		}
	} else {
		log.Warnf("the content type is not supported, current content is %v", chatMessage.Content)
	}
	return bedrockMessageContent{
		ToolResult: toolResultContent,
	}
}

func chatMessage2BedrockMessage(chatMessage chatMessage) bedrockMessage {
	var result bedrockMessage
	if len(chatMessage.ToolCalls) > 0 {
		contents := make([]bedrockMessageContent, 0, len(chatMessage.ToolCalls))
		for _, toolCall := range chatMessage.ToolCalls {
			params := map[string]interface{}{}
			json.Unmarshal([]byte(toolCall.Function.Arguments), &params)
			contents = append(contents, bedrockMessageContent{
				ToolUse: &toolUseBlock{
					Input:     params,
					Name:      toolCall.Function.Name,
					ToolUseId: toolCall.Id,
				},
			})
		}
		result = bedrockMessage{
			Role:    chatMessage.Role,
			Content: contents,
		}
	} else if chatMessage.IsStringContent() {
		result = bedrockMessage{
			Role:    chatMessage.Role,
			Content: []bedrockMessageContent{{Text: chatMessage.StringContent()}},
		}
	} else {
		var contents []bedrockMessageContent
		openaiContent := chatMessage.ParseContent()
		for _, part := range openaiContent {
			var content bedrockMessageContent
			if part.Type == contentTypeText {
				content.Text = part.Text
			} else if part.Type == contentTypeImageUrl {
				base64Str := part.ImageUrl.Url
				prefix, imageType, err := extractImageType(base64Str)
				if err != nil {
					log.Warn("image url is not supported")
					continue
				}
				base64WoPrefix, _ := strings.CutPrefix(base64Str, prefix)
				content.Image = &imageBlock{
					Format: imageType,
					Source: imageSource{
						Bytes: base64WoPrefix,
					},
				}
			} else {
				log.Warnf("type is not supported: %s", part.Type)
				continue
			}
			contents = append(contents, content)
		}
		result = bedrockMessage{
			Role:    chatMessage.Role,
			Content: contents,
		}
	}
	return result
}

func (b *bedrockProvider) setAuthHeaders(body []byte, headers http.Header) {
	// Bearer token authentication is already set in TransformRequestHeaders
	// This function only handles AWS SigV4 authentication which requires the request body
	if len(b.config.apiTokens) > 0 {
		return
	}

	// Use AWS Signature V4 authentication
	accessKey := strings.TrimSpace(b.config.awsAccessKey)
	region := strings.TrimSpace(b.config.awsRegion)
	t := time.Now().UTC()
	amzDate := t.Format("20060102T150405Z")
	dateStamp := t.Format("20060102")
	path := headers.Get(":path")
	signature := b.generateSignature(path, amzDate, dateStamp, body)
	headers.Set("X-Amz-Date", amzDate)
	util.OverwriteRequestAuthorizationHeader(headers, fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s/%s/%s/aws4_request, SignedHeaders=%s, Signature=%s", accessKey, dateStamp, region, awsService, bedrockSignedHeaders, signature))
}

func (b *bedrockProvider) generateSignature(path, amzDate, dateStamp string, body []byte) string {
	canonicalURI := encodeSigV4Path(path)
	hashedPayload := sha256Hex(body)
	region := strings.TrimSpace(b.config.awsRegion)
	secretKey := strings.TrimSpace(b.config.awsSecretKey)

	endpoint := fmt.Sprintf(bedrockDefaultDomain, region)
	canonicalHeaders := fmt.Sprintf("host:%s\nx-amz-date:%s\n", endpoint, amzDate)
	canonicalRequest := fmt.Sprintf("%s\n%s\n\n%s\n%s\n%s",
		httpPostMethod, canonicalURI, canonicalHeaders, bedrockSignedHeaders, hashedPayload)

	credentialScope := fmt.Sprintf("%s/%s/%s/aws4_request", dateStamp, region, awsService)
	hashedCanonReq := sha256Hex([]byte(canonicalRequest))
	stringToSign := fmt.Sprintf("AWS4-HMAC-SHA256\n%s\n%s\n%s",
		amzDate, credentialScope, hashedCanonReq)

	signingKey := getSignatureKey(secretKey, dateStamp, region, awsService)
	signature := hmacHex(signingKey, stringToSign)
	return signature
}

func encodeSigV4Path(path string) string {
	// Keep only the URI path for canonical URI. Query string is handled separately in SigV4,
	// and this implementation uses an empty canonical query string.
	if queryIndex := strings.Index(path, "?"); queryIndex >= 0 {
		path = path[:queryIndex]
	}

	segments := strings.Split(path, "/")
	for i, seg := range segments {
		if seg == "" {
			continue
		}
		segments[i] = url.PathEscape(seg)
	}
	return strings.Join(segments, "/")
}

func getSignatureKey(key, dateStamp, region, service string) []byte {
	kDate := hmacSha256([]byte("AWS4"+key), dateStamp)
	kRegion := hmacSha256(kDate, region)
	kService := hmacSha256(kRegion, service)
	kSigning := hmacSha256(kService, "aws4_request")
	return kSigning
}

func hmacSha256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

func sha256Hex(data []byte) string {
	h := sha256.New()
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

func hmacHex(key []byte, data string) string {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

func extractImageType(base64Str string) (string, string, error) {
	re := regexp.MustCompile(`^data:([^;]+);base64,`)
	matches := re.FindStringSubmatch(base64Str)
	if len(matches) < 2 {
		return "", "", fmt.Errorf("invalid base64 format")
	}

	mimeType := matches[1] // e.g. image/png
	parts := strings.Split(mimeType, "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid mimeType")
	}
	return matches[0], parts[1], nil
}
