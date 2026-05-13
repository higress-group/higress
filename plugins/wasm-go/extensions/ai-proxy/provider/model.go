package provider

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

const (
	streamEventIdItemKey        = "id:"
	streamEventNameItemKey      = "event:"
	streamBuiltInItemKey        = ":"
	streamHttpStatusValuePrefix = "HTTP_STATUS/"
	streamDataItemKey           = "data:"
	streamEndDataValue          = "[DONE]"

	eventResult = "result"

	httpStatus200 = "200"

	contentTypeText       = "text"
	contentTypeImageUrl   = "image_url"
	contentTypeInputAudio = "input_audio"
	contentTypeFile       = "file"

	reasoningStartTag = "<think>"
	reasoningEndTag   = "</think>"
)

type NonOpenAIStyleOptions struct {
	ReasoningMaxTokens int            `json:"reasoning_max_tokens,omitempty"`
	Thinking           *thinkingParam `json:"thinking,omitempty"`
}

type thinkingParam struct {
	Type        string `json:"type,omitempty"`
	BudgetToken int    `json:"budget_token,omitempty"`
}

type chatCompletionRequest struct {
	NonOpenAIStyleOptions
	Messages             []chatMessage          `json:"messages"`
	Model                string                 `json:"model"`
	Store                bool                   `json:"store,omitempty"`
	ReasoningEffort      string                 `json:"reasoning_effort,omitempty"`
	ClaudeThinking       *claudeThinkingConfig  `json:"claude_thinking,omitempty"`
	ClaudeOutputConfig   *claudeOutputConfig    `json:"claude_output_config,omitempty"`
	ClaudeAnthropicBeta  []string               `json:"claude_anthropic_beta,omitempty"`
	Metadata             map[string]string      `json:"metadata,omitempty"`
	FrequencyPenalty     float64                `json:"frequency_penalty,omitempty"`
	LogitBias            map[string]int         `json:"logit_bias,omitempty"`
	Logprobs             bool                   `json:"logprobs,omitempty"`
	TopLogprobs          int                    `json:"top_logprobs,omitempty"`
	MaxTokens            int                    `json:"max_tokens,omitempty"`
	MaxCompletionTokens  int                    `json:"max_completion_tokens,omitempty"`
	N                    int                    `json:"n,omitempty"`
	Modalities           []string               `json:"modalities,omitempty"`
	Prediction           map[string]interface{} `json:"prediction,omitempty"`
	Audio                map[string]interface{} `json:"audio,omitempty"`
	PresencePenalty      float64                `json:"presence_penalty,omitempty"`
	ResponseFormat       map[string]interface{} `json:"response_format,omitempty"`
	Seed                 int                    `json:"seed,omitempty"`
	ServiceTier          string                 `json:"service_tier,omitempty"`
	Stop                 []string               `json:"stop,omitempty"`
	Stream               bool                   `json:"stream,omitempty"`
	StreamOptions        *streamOptions         `json:"stream_options,omitempty"`
	PromptCacheRetention string                 `json:"prompt_cache_retention,omitempty"`
	PromptCacheKey       string                 `json:"prompt_cache_key,omitempty"`
	Temperature          float64                `json:"temperature,omitempty"`
	TopP                 float64                `json:"top_p,omitempty"`
	Tools                []tool                 `json:"tools,omitempty"`
	ToolChoice           interface{}            `json:"tool_choice,omitempty"`
	ParallelToolCalls    *bool                  `json:"parallel_tool_calls,omitempty"`
	User                 string                 `json:"user,omitempty"`
}

func (c *chatCompletionRequest) getMaxTokens() int {
	if c.MaxCompletionTokens > 0 {
		return c.MaxCompletionTokens
	}
	return c.MaxTokens
}

func (c *chatCompletionRequest) getToolChoiceString() string {
	if c.ToolChoice == nil {
		return ""
	}

	if tc, ok := c.ToolChoice.(string); ok {
		return tc
	}
	return ""
}

func (c *chatCompletionRequest) getToolChoiceType() string {
	if tc := c.getToolChoiceString(); tc != "" {
		return tc
	}
	if tc := c.getToolChoiceObject(); tc != nil {
		return tc.Type
	}
	return ""
}

func (c *chatCompletionRequest) getToolChoiceObject() *toolChoice {
	switch tc := c.ToolChoice.(type) {
	case nil, string:
		return nil
	case *toolChoice:
		return tc
	}

	body, err := json.Marshal(c.ToolChoice)
	if err != nil {
		return nil
	}
	var parsed toolChoice
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil
	}
	return &parsed
}

type CompletionRequest struct {
	Model            string         `json:"model"`
	Prompt           string         `json:"prompt"`
	BestOf           int            `json:"best_of,omitempty"`
	Echo             bool           `json:"echo,omitempty"`
	FrequencyPenalty float64        `json:"frequency_penalty,omitempty"`
	LogitBias        map[string]int `json:"logit_bias,omitempty"`
	Logprobs         int            `json:"logprobs,omitempty"`
	MaxTokens        int            `json:"max_tokens,omitempty"`
	N                int            `json:"n,omitempty"`
	PresencePenalty  float64        `json:"presence_penalty,omitempty"`
	Seed             int            `json:"seed,omitempty"`
	Stop             []string       `json:"stop,omitempty"`
	Stream           bool           `json:"stream,omitempty"`
	StreamOptions    *streamOptions `json:"stream_options,omitempty"`
	Suffix           string         `json:"suffix,omitempty"`
	Temperature      float64        `json:"temperature,omitempty"`
	TopP             float64        `json:"top_p,omitempty"`
	User             string         `json:"user,omitempty"`
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage,omitempty"`
}

type tool struct {
	Type     string   `json:"type"`
	Function function `json:"function"`
}

type function struct {
	Description string                 `json:"description,omitempty"`
	Name        string                 `json:"name"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

type toolChoice struct {
	Type     string   `json:"type"`
	Function function `json:"function"`
}

type chatCompletionResponse struct {
	Id                string                 `json:"id,omitempty"`
	Choices           []chatCompletionChoice `json:"choices"`
	Created           int64                  `json:"created,omitempty"`
	Model             string                 `json:"model,omitempty"`
	ServiceTier       string                 `json:"service_tier,omitempty"`
	SystemFingerprint string                 `json:"system_fingerprint,omitempty"`
	Object            string                 `json:"object,omitempty"`
	Usage             *usage                 `json:"usage"`
}

type chatCompletionChoice struct {
	Index        int                    `json:"index"`
	Message      *chatMessage           `json:"message,omitempty"`
	Delta        *chatMessage           `json:"delta,omitempty"`
	FinishReason *string                `json:"finish_reason"`
	Logprobs     map[string]interface{} `json:"logprobs"`
}

type usage struct {
	PromptTokens            int                      `json:"prompt_tokens,omitempty"`
	CompletionTokens        int                      `json:"completion_tokens,omitempty"`
	TotalTokens             int                      `json:"total_tokens,omitempty"`
	CompletionTokensDetails *completionTokensDetails `json:"completion_tokens_details,omitempty"`
	PromptTokensDetails     *promptTokensDetails     `json:"prompt_tokens_details,omitempty"`
}

type promptTokensDetails struct {
	AudioTokens  int `json:"audio_tokens,omitempty"`
	CachedTokens int `json:"cached_tokens,omitempty"`
}

type completionTokensDetails struct {
	ReasoningTokens          int `json:"reasoning_tokens,omitempty"`
	AudioTokens              int `json:"audio_tokens,omitempty"`
	AcceptedPredictionTokens int `json:"accepted_prediction_tokens,omitempty"`
	RejectedPredictionTokens int `json:"rejected_prediction_tokens,omitempty"`
}

type chatMessage struct {
	Id                       string                     `json:"id,omitempty"`
	Audio                    map[string]interface{}     `json:"audio,omitempty"`
	Name                     string                     `json:"name,omitempty"`
	Role                     string                     `json:"role,omitempty"`
	Content                  any                        `json:"content,omitempty"`
	ReasoningContent         string                     `json:"reasoning_content,omitempty"`
	Reasoning                string                     `json:"reasoning,omitempty"` // For streaming responses
	ReasoningSignature       string                     `json:"reasoning_signature,omitempty"`
	ReasoningRedactedContent string                     `json:"reasoning_redacted_content,omitempty"`
	ClaudeContentBlocks      []claudeChatMessageContent `json:"claude_content_blocks,omitempty"`
	ClaudeContentBlockIndex  *int                       `json:"claude_content_block_index,omitempty"`
	ClaudeContentBlockStop   *int                       `json:"claude_content_block_stop,omitempty"`
	ToolCalls                []toolCall                 `json:"tool_calls,omitempty"`
	FunctionCall             *functionCall              `json:"function_call,omitempty"` // For legacy OpenAI format
	Refusal                  string                     `json:"refusal,omitempty"`
	ToolCallId               string                     `json:"tool_call_id,omitempty"`
}

func (m *chatMessage) handleNonStreamingReasoningContent(reasoningContentMode string) {
	if m.ReasoningContent == "" {
		return
	}
	switch reasoningContentMode {
	case reasoningBehaviorIgnore:
		m.ReasoningContent = ""
		break
	case reasoningBehaviorConcat:
		m.Content = fmt.Sprintf("%s%v%s\n%v", reasoningStartTag, m.ReasoningContent, reasoningEndTag, m.Content)
		m.ReasoningContent = ""
		break
	case reasoningBehaviorPassThrough:
	default:
		break
	}
}

func (m *chatMessage) handleStreamingReasoningContent(ctx wrapper.HttpContext, reasoningContentMode string) {
	switch reasoningContentMode {
	case reasoningBehaviorIgnore:
		m.ReasoningContent = ""
		break
	case reasoningBehaviorConcat:
		contentPushed, _ := ctx.GetContext(ctxKeyContentPushed).(bool)
		reasoningContentPushed, _ := ctx.GetContext(ctxKeyReasoningContentPushed).(bool)

		if contentPushed {
			if m.ReasoningContent != "" {
				// This shouldn't happen, but if it does, we can add a log here.
				log.Warnf("[ai-proxy] Content already pushed, but reasoning content is not empty: %v", m)
			}
			return
		}

		if m.ReasoningContent != "" && !reasoningContentPushed {
			m.ReasoningContent = reasoningStartTag + m.ReasoningContent
			reasoningContentPushed = true
		}
		if m.Content != "" {
			if reasoningContentPushed && !contentPushed /* Keep the second part just to make it easy to understand*/ {
				m.ReasoningContent += reasoningEndTag
			}
			contentPushed = true
		}

		m.Content = fmt.Sprintf("%s\n%v", m.ReasoningContent, m.Content)
		m.ReasoningContent = ""

		ctx.SetContext(ctxKeyContentPushed, contentPushed)
		ctx.SetContext(ctxKeyReasoningContentPushed, reasoningContentPushed)
		break
	case reasoningBehaviorPassThrough:
	default:
		break
	}
}

// promoteThinkingOnEmpty promotes reasoning_content to content when content is empty.
// This handles models that put user-facing replies into thinking blocks instead of text blocks.
func (r *chatCompletionResponse) promoteThinkingOnEmpty() {
	for i := range r.Choices {
		msg := r.Choices[i].Message
		if msg == nil {
			continue
		}
		if !isContentEmpty(msg.Content) {
			continue
		}
		if msg.ReasoningContent != "" {
			msg.Content = msg.ReasoningContent
			msg.ReasoningContent = ""
		}
	}
}

// promoteStreamingThinkingOnEmpty accumulates reasoning content during streaming.
// It strips reasoning from chunks and buffers it. When content is seen, it marks
// the stream as having content so no promotion will happen.
// Call PromoteStreamingThinkingFlush at the end of the stream to emit buffered
// reasoning as content if no content was ever seen.
// Returns true if the chunk was modified (reasoning stripped).
func promoteStreamingThinkingOnEmpty(ctx wrapper.HttpContext, msg *chatMessage) bool {
	if msg == nil {
		return false
	}
	hasContentDelta, _ := ctx.GetContext(ctxKeyHasContentDelta).(bool)
	if hasContentDelta {
		return false
	}

	if !isContentEmpty(msg.Content) {
		ctx.SetContext(ctxKeyHasContentDelta, true)
		return false
	}

	// Buffer reasoning content and strip it from the chunk
	reasoning := msg.ReasoningContent
	if reasoning == "" {
		reasoning = msg.Reasoning
	}
	if reasoning != "" {
		buffered, _ := ctx.GetContext(ctxKeyBufferedReasoning).(string)
		ctx.SetContext(ctxKeyBufferedReasoning, buffered+reasoning)
		msg.ReasoningContent = ""
		msg.Reasoning = ""
		return true
	}
	return false
}

func isContentEmpty(content any) bool {
	switch v := content.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(v) == ""
	default:
		return false
	}
}

type chatMessageContent struct {
	CacheControl map[string]interface{}      `json:"cache_control,omitempty"`
	Type         string                      `json:"type,omitempty"`
	Text         string                      `json:"text"`
	ImageUrl     *chatMessageContentImageUrl `json:"image_url,omitempty"`
	File         *chatMessageContentFile     `json:"file,omitempty"`
	InputAudio   *chatMessageContentAudio    `json:"input_audio,omitempty"`
}

type chatMessageContentAudio struct {
	Data   string `json:"data"`
	Format string `json:"format"`
}

type chatMessageContentFile struct {
	FileData string `json:"file_data,omitempty"`
	FileId   string `json:"file_id,omitempty"`
	FileName string `json:"file_name,omitempty"`
}

type chatMessageContentImageUrl struct {
	Url    string `json:"url,omitempty"`
	Detail string `json:"detail,omitempty"`
}

func (m *chatMessage) IsEmpty() bool {
	if m.ReasoningContent != "" {
		return false
	}
	if m.IsStringContent() && m.Content != "" {
		return false
	}
	anyList, ok := m.Content.([]any)
	if ok && len(anyList) > 0 {
		return false
	}
	if len(m.ToolCalls) != 0 {
		nonEmpty := false
		for _, toolCall := range m.ToolCalls {
			if !toolCall.Function.IsEmpty() {
				nonEmpty = true
				break
			}
		}
		if nonEmpty {
			return false
		}
	}
	return true
}

func (m *chatMessage) IsStringContent() bool {
	_, ok := m.Content.(string)
	return ok
}

func (m *chatMessage) StringContent() string {
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
			if contentMap["type"] == contentTypeText {
				if subStr, ok := contentMap[contentTypeText].(string); ok {
					contentStr += subStr + "\n"
				}
			}
		}
		return contentStr
	}
	return ""
}

func (m *chatMessage) ParseContent() []chatMessageContent {
	var contentList []chatMessageContent
	content, ok := m.Content.(string)
	if ok {
		contentList = append(contentList, chatMessageContent{
			Type: contentTypeText,
			Text: content,
		})
		return contentList
	}
	anyList, ok := m.Content.([]any)
	if ok {
		for _, contentItem := range anyList {
			contentMap, ok := contentItem.(map[string]any)
			if !ok {
				continue
			}
			switch contentMap["type"] {
			case contentTypeText:
				if subStr, ok := contentMap[contentTypeText].(string); ok {
					contentList = append(contentList, chatMessageContent{
						Type: contentTypeText,
						Text: subStr,
					})
				}
			case contentTypeImageUrl:
				if subObj, ok := contentMap[contentTypeImageUrl].(map[string]any); ok {
					msg := chatMessageContent{
						Type: contentTypeImageUrl,
						ImageUrl: &chatMessageContentImageUrl{
							Url: subObj["url"].(string),
						},
					}
					if detail, ok := subObj["detail"].(string); ok {
						msg.ImageUrl.Detail = detail
					}
					contentList = append(contentList, msg)
				}
			case contentTypeInputAudio:
				if subObj, ok := contentMap[contentTypeInputAudio].(map[string]any); ok {
					contentList = append(contentList, chatMessageContent{
						Type: contentTypeInputAudio,
						InputAudio: &chatMessageContentAudio{
							Data:   subObj["data"].(string),
							Format: subObj["format"].(string),
						},
					})
				}
			case contentTypeFile:
				if subObj, ok := contentMap[contentTypeFile].(map[string]any); ok {
					contentList = append(contentList, chatMessageContent{
						Type: contentTypeFile,
						File: &chatMessageContentFile{
							FileId: subObj["file_id"].(string),
							// FileName: subObj["file_name"].(string),
							// FileData: subObj["file_data"].(string),
						},
					})
				}
			}
		}
		return contentList
	}
	return nil
}

type toolCall struct {
	Index    int          `json:"index"`
	Id       string       `json:"id,omitempty"`
	Type     string       `json:"type"`
	Function functionCall `json:"function"`
}

type functionCall struct {
	Id        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments"`
}

func (m *functionCall) IsEmpty() bool {
	return m.Name == "" && m.Arguments == ""
}

type StreamEvent struct {
	RawEvent   string `json:"-"`
	Id         string `json:"id"`
	Event      string `json:"event"`
	Data       string `json:"data"`
	HttpStatus string `json:"http_status"`
}

func (e *StreamEvent) IsEndData() bool {
	return e.Data == streamEndDataValue
}

func (e *StreamEvent) SetValue(key, value string) {
	switch key {
	case streamEventIdItemKey:
		e.Id = value
	case streamEventNameItemKey:
		e.Event = value
	case streamDataItemKey:
		e.Data = value
	case streamBuiltInItemKey:
		if strings.HasPrefix(value, streamHttpStatusValuePrefix) {
			e.HttpStatus = value[len(streamHttpStatusValuePrefix):]
		}
	}
}

func (e *StreamEvent) ToHttpString() string {
	return fmt.Sprintf("%s %s\n\n", streamDataItemKey, e.Data)
}

// https://platform.openai.com/docs/guides/images
type imageGenerationRequest struct {
	Model             string `json:"model"`
	Prompt            string `json:"prompt"`
	Background        string `json:"background,omitempty"`
	Moderation        string `json:"moderation,omitempty"`
	OutputCompression int    `json:"output_compression,omitempty"`
	OutputFormat      string `json:"output_format,omitempty"`
	Quality           string `json:"quality,omitempty"`
	ResponseFormat    string `json:"response_format,omitempty"`
	Style             string `json:"style,omitempty"`
	N                 int    `json:"n,omitempty"`
	Size              string `json:"size,omitempty"`
}

type imageInputURL struct {
	URL      string                      `json:"url,omitempty"`
	ImageURL *chatMessageContentImageUrl `json:"image_url,omitempty"`
}

func (i *imageInputURL) UnmarshalJSON(data []byte) error {
	// Support a plain string payload, e.g. "data:image/png;base64,..."
	var rawURL string
	if err := json.Unmarshal(data, &rawURL); err == nil {
		i.URL = rawURL
		i.ImageURL = nil
		return nil
	}

	type alias imageInputURL
	var value alias
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*i = imageInputURL(value)
	return nil
}

func (i *imageInputURL) GetURL() string {
	if i == nil {
		return ""
	}
	if i.ImageURL != nil && i.ImageURL.Url != "" {
		return i.ImageURL.Url
	}
	return i.URL
}

type imageEditRequest struct {
	Model             string          `json:"model"`
	Prompt            string          `json:"prompt"`
	Image             *imageInputURL  `json:"image,omitempty"`
	Images            []imageInputURL `json:"images,omitempty"`
	ImageURL          *imageInputURL  `json:"image_url,omitempty"`
	Mask              *imageInputURL  `json:"mask,omitempty"`
	MaskURL           *imageInputURL  `json:"mask_url,omitempty"`
	Background        string          `json:"background,omitempty"`
	Moderation        string          `json:"moderation,omitempty"`
	OutputCompression int             `json:"output_compression,omitempty"`
	OutputFormat      string          `json:"output_format,omitempty"`
	Quality           string          `json:"quality,omitempty"`
	ResponseFormat    string          `json:"response_format,omitempty"`
	Style             string          `json:"style,omitempty"`
	N                 int             `json:"n,omitempty"`
	Size              string          `json:"size,omitempty"`
}

func (r *imageEditRequest) GetImageURLs() []string {
	urls := make([]string, 0, len(r.Images)+2)
	for _, image := range r.Images {
		if url := image.GetURL(); url != "" {
			urls = append(urls, url)
		}
	}
	if r.Image != nil {
		if url := r.Image.GetURL(); url != "" {
			urls = append(urls, url)
		}
	}
	if r.ImageURL != nil {
		if url := r.ImageURL.GetURL(); url != "" {
			urls = append(urls, url)
		}
	}
	return urls
}

func (r *imageEditRequest) HasMask() bool {
	if r.Mask != nil && r.Mask.GetURL() != "" {
		return true
	}
	return r.MaskURL != nil && r.MaskURL.GetURL() != ""
}

type imageVariationRequest struct {
	Model             string          `json:"model"`
	Prompt            string          `json:"prompt,omitempty"`
	Image             *imageInputURL  `json:"image,omitempty"`
	Images            []imageInputURL `json:"images,omitempty"`
	ImageURL          *imageInputURL  `json:"image_url,omitempty"`
	Background        string          `json:"background,omitempty"`
	Moderation        string          `json:"moderation,omitempty"`
	OutputCompression int             `json:"output_compression,omitempty"`
	OutputFormat      string          `json:"output_format,omitempty"`
	Quality           string          `json:"quality,omitempty"`
	ResponseFormat    string          `json:"response_format,omitempty"`
	Style             string          `json:"style,omitempty"`
	N                 int             `json:"n,omitempty"`
	Size              string          `json:"size,omitempty"`
}

func (r *imageVariationRequest) GetImageURLs() []string {
	urls := make([]string, 0, len(r.Images)+2)
	for _, image := range r.Images {
		if url := image.GetURL(); url != "" {
			urls = append(urls, url)
		}
	}
	if r.Image != nil {
		if url := r.Image.GetURL(); url != "" {
			urls = append(urls, url)
		}
	}
	if r.ImageURL != nil {
		if url := r.ImageURL.GetURL(); url != "" {
			urls = append(urls, url)
		}
	}
	return urls
}

type imageGenerationData struct {
	URL           string `json:"url,omitempty"`
	B64           string `json:"b64_json,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

type imageGenerationUsage struct {
	TotalTokens        int `json:"total_tokens"`
	InputTokens        int `json:"input_tokens"`
	OutputTokens       int `json:"output_tokens"`
	InputTokensDetails struct {
		TextTokens  int `json:"text_tokens"`
		ImageTokens int `json:"image_tokens"`
	} `json:"input_tokens_details"`
}

type imageGenerationResponse struct {
	Created int64                 `json:"created"`
	Data    []imageGenerationData `json:"data"`
	Usage   *imageGenerationUsage `json:"usage,omitempty"`
}

// https://platform.openai.com/docs/guides/speech-to-text
type audioSpeechRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
	Voice string `json:"voice"`
}

type embeddingsRequest struct {
	Input          interface{} `json:"input"`
	Model          string      `json:"model"`
	EncodingFormat string      `json:"encoding_format,omitempty"`
	Dimensions     int         `json:"dimensions,omitempty"`
	User           string      `json:"user,omitempty"`
}

type embeddingsResponse struct {
	Object string      `json:"object"`
	Data   []embedding `json:"data"`
	Model  string      `json:"model"`
	Usage  usage       `json:"usage"`
}

type embedding struct {
	Object    string    `json:"object"`
	Index     int       `json:"index"`
	Embedding []float64 `json:"embedding"`
}

func (r embeddingsRequest) ParseInput() []string {
	if r.Input == nil {
		return nil
	}
	var input []string
	switch r.Input.(type) {
	case string:
		input = []string{r.Input.(string)}
	case []any:
		input = make([]string, 0, len(r.Input.([]any)))
		for _, item := range r.Input.([]any) {
			if str, ok := item.(string); ok {
				input = append(input, str)
			}
		}
	}
	return input
}

// PromoteThinkingOnEmptyResponse promotes reasoning_content to content in a non-streaming
// response body when content is empty. Returns the original body if no promotion is needed.
func PromoteThinkingOnEmptyResponse(body []byte) ([]byte, error) {
	var resp chatCompletionResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return body, fmt.Errorf("unable to unmarshal response for thinking promotion: %v", err)
	}
	promoted := false
	for i := range resp.Choices {
		msg := resp.Choices[i].Message
		if msg == nil {
			continue
		}
		if !isContentEmpty(msg.Content) {
			continue
		}
		if msg.ReasoningContent != "" {
			msg.Content = msg.ReasoningContent
			msg.ReasoningContent = ""
			promoted = true
		}
	}
	if !promoted {
		return body, nil
	}
	return json.Marshal(resp)
}

// PromoteStreamingThinkingOnEmptyChunk buffers reasoning deltas and strips them from
// the chunk during streaming. Call PromoteStreamingThinkingFlush on the last chunk
// to emit buffered reasoning as content if no real content was ever seen.
func PromoteStreamingThinkingOnEmptyChunk(ctx wrapper.HttpContext, data []byte) ([]byte, error) {
	var resp chatCompletionResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return data, nil // not a valid chat completion chunk, skip
	}
	modified := false
	for i := range resp.Choices {
		msg := resp.Choices[i].Delta
		if msg == nil {
			continue
		}
		if promoteStreamingThinkingOnEmpty(ctx, msg) {
			modified = true
		}
	}
	if !modified {
		return data, nil
	}
	return json.Marshal(resp)
}

// PromoteStreamingThinkingFlush checks if the stream had no content and returns
// an SSE chunk that emits the buffered reasoning as content. Returns nil if
// content was already seen or no reasoning was buffered.
func PromoteStreamingThinkingFlush(ctx wrapper.HttpContext) []byte {
	hasContentDelta, _ := ctx.GetContext(ctxKeyHasContentDelta).(bool)
	if hasContentDelta {
		return nil
	}
	buffered, _ := ctx.GetContext(ctxKeyBufferedReasoning).(string)
	if buffered == "" {
		return nil
	}
	// Build a minimal chat.completion.chunk with the buffered reasoning as content
	resp := chatCompletionResponse{
		Object: objectChatCompletionChunk,
		Choices: []chatCompletionChoice{
			{
				Index: 0,
				Delta: &chatMessage{
					Content: buffered,
				},
			},
		},
	}
	data, err := json.Marshal(resp)
	if err != nil {
		return nil
	}
	// Format as SSE
	return []byte("data: " + string(data) + "\n\n")
}
