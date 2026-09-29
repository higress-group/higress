package provider

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/tidwall/gjson"
)

// responses_to_openai.go 实现 OpenAI Responses API → Chat Completions 的网关侧
// 协议桥接（OPE-9267，补齐 OPE-9012 codex 托管计费对 chat-only 上游的缺口）。
//
// 设计完全对齐本插件已有的 Claude→OpenAI 自动协议转换范式（main.go 的
// needClaudeResponseConversion 四段式）：
//
//   - 门控：仅当客户端请求 /v1/responses 而 provider 未声明 Responses
//     capability 时启用（今天这类组合必然 500 硬失败，行为变化无回归面）；
//   - 请求向：路径改写为 /v1/chat/completions，请求体降维成 chat 语义；
//   - 响应向：chat JSON / SSE 升维回 Responses 对象与事件流；
//   - 桥接只覆盖无状态子集：store=false 且无 previous_response_id（codex
//     disable_response_storage 下的真实形态）。有状态语义显式拒绝并给出
//     明确错误，而不是静默出错。

const (
	responsesObjectType = "response"

	responsesStatusCompleted   = "completed"
	responsesStatusInProgress  = "in_progress"
	responsesStatusIncomplete  = "incomplete"

	incompleteReasonMaxOutputTokens = "max_output_tokens"
	incompleteReasonContentFilter   = "content_filter"
)

// ConvertResponsesRequestToChat 把无状态 Responses 请求体降维成 chat
// completions 请求体。支持的映射：
//
//	instructions            → 首条 system 消息
//	input (string|array)    → messages（message/function_call/function_call_output
//	                          /reasoning 四类 item；reasoning 回填到所属 assistant
//	                          轮次的 reasoning_content 字段，见下）
//	tools                   → chat tools（去掉 openai 专属的 strict 字段）
//	tool_choice             → chat tool_choice（对象形态 name → function.name）
//	max_output_tokens       → max_tokens
//	temperature/top_p/stream/parallel_tool_calls → 原样保留
//	stream=true 时强制 stream_options.include_usage=true（计量依赖终块 usage）
//
// reasoning 回填是 thinking 模型上游的硬契约（OPE-9733）：DeepSeek 等 thinking
// 模式上游要求「thinking + tools 并存时，历史 assistant 消息必须带回
// reasoning_content」，缺失即 400 拒绝整轮请求。响应向会把上游思考内容升维成
// Responses reasoning item 交给 codex 存档，本函数负责回放时的逆向降维。
//
// 显式拒绝（返回错误）：previous_response_id、store=true、background=true、
// item_reference——这些语义无法在 chat 协议无损表达。
func ConvertResponsesRequestToChat(body []byte) ([]byte, error) {
	if previousResponseID := gjson.GetBytes(body, "previous_response_id"); previousResponseID.Exists() && previousResponseID.String() != "" {
		return nil, fmt.Errorf("responses-to-chat bridge is stateless and cannot resolve previous_response_id")
	}
	if gjson.GetBytes(body, "store").Bool() {
		return nil, fmt.Errorf("responses-to-chat bridge requires store=false")
	}
	if gjson.GetBytes(body, "background").Bool() {
		return nil, fmt.Errorf("responses-to-chat bridge does not support background mode")
	}

	messages := make([]map[string]interface{}, 0, 8)
	if instructions := gjson.GetBytes(body, "instructions"); instructions.Exists() && instructions.String() != "" {
		messages = append(messages, map[string]interface{}{
			"role":    "system",
			"content": instructions.String(),
		})
	}

	inputValue := gjson.GetBytes(body, "input")
	if inputValue.Exists() {
		switch inputValue.Type {
		case gjson.String:
			messages = append(messages, map[string]interface{}{
				"role":    "user",
				"content": inputValue.String(),
			})
		case gjson.JSON:
			// 相邻 function_call items 必须合并为一条 assistant 消息的多元素
			// tool_calls：codex 一轮并行多工具调用在 Responses 输入里是连续
			// 多个 function_call item，逐条映射会产出「assistant(tool_calls)
			// 后紧跟另一条 assistant」的非法 chat 序列，DeepSeek 等上游校验
			// 直接报 insufficient tool messages following tool_calls message。
			var pendingCalls []interface{}
			// reasoning item 与 function_call 同规则缓冲：它属于紧接着的
			// assistant 轮次（codex 会在同一轮的并行调用之间插入 reasoning
			// item，缓冲同时保证不拆散调用组）。
			var pendingReasoning []string
			attachReasoning := func(msg map[string]interface{}) {
				if text := joinReasoningText(pendingReasoning); text != "" {
					msg["reasoning_content"] = text
				}
				pendingReasoning = nil
			}
			flushPendingCalls := func() {
				if len(pendingCalls) == 0 {
					return
				}
				msg := map[string]interface{}{
					"role":       "assistant",
					"tool_calls": pendingCalls,
				}
				attachReasoning(msg)
				messages = append(messages, msg)
				pendingCalls = nil
			}
			for _, item := range inputValue.Array() {
				itemType := item.Get("type").String()
				if itemType == "function_call" {
					pendingCalls = append(pendingCalls, map[string]interface{}{
						"id":   item.Get("call_id").String(),
						"type": "function",
						"function": map[string]interface{}{
							"name":      item.Get("name").String(),
							"arguments": item.Get("arguments").String(),
						},
					})
					continue
				}
				if itemType == "reasoning" {
					if text := reasoningTextFromItem(item); text != "" {
						pendingReasoning = append(pendingReasoning, text)
					}
					continue
				}
				flushPendingCalls()
				msg, err := responsesInputItemToChatMessage(item)
				if err != nil {
					return nil, err
				}
				if msg != nil {
					// 无调用组可挂时，reasoning 归属于紧随的 assistant 文本消息；
					// 跟随 user/system/tool 轮则属无主流失，丢弃不污染。
					if msg["role"] == "assistant" {
						attachReasoning(msg)
					} else {
						pendingReasoning = nil
					}
					messages = append(messages, msg)
				}
			}
			flushPendingCalls()
		default:
			return nil, fmt.Errorf("unsupported responses input type: %v", inputValue.Type)
		}
	}
	if len(messages) == 0 {
		return nil, fmt.Errorf("responses request has no input to convert")
	}

	chatRequest := map[string]interface{}{
		"model":    gjson.GetBytes(body, "model").String(),
		"messages": messages,
	}

	if tools := gjson.GetBytes(body, "tools"); tools.Exists() && tools.IsArray() && len(tools.Array()) > 0 {
		chatTools := make([]map[string]interface{}, 0, len(tools.Array()))
		for _, t := range tools.Array() {
			if t.Get("type").String() != "function" {
				log.Debugf("[Responses→Chat] drop non-function tool type: %s", t.Get("type").String())
				continue
			}
			fn := map[string]interface{}{"name": t.Get("name").String()}
			if desc := t.Get("description"); desc.Exists() && desc.String() != "" {
				fn["description"] = desc.String()
			}
			if params := t.Get("parameters"); params.Exists() {
				fn["parameters"] = params.Value()
			}
			// strict 是 OpenAI Responses 专属开关，chat 上游（DeepSeek 等）不认识，
			// 保留可能触发严格校验失败，统一剥除。
			chatTools = append(chatTools, map[string]interface{}{
				"type":     "function",
				"function": fn,
			})
		}
		if len(chatTools) > 0 {
			chatRequest["tools"] = chatTools
			// thinking 上游（DeepSeek V4.1-Flash 线级探针实证，OPE-9733）：请求带
			// tools 且历史含 assistant 轮次时无条件校验 reasoning_content——即使
			// 上一轮模型自适应跳过思考、未产出任何推理内容，回放也必须带该字段，
			// 否则 400 拒绝整轮。对没有缓冲到思考文本的 assistant 轮次补非空占位
			//（上游实测接受任意非空文本，仅作上下文消费，不影响 codex 侧）。
			for _, m := range messages {
				if m["role"] == "assistant" {
					if _, ok := m["reasoning_content"]; !ok {
						m["reasoning_content"] = "(no reasoning content recorded)"
					}
				}
			}
			if tc := gjson.GetBytes(body, "tool_choice"); tc.Exists() {
				if tc.Type == gjson.String {
					chatRequest["tool_choice"] = tc.String()
				} else if tc.Get("type").String() == "function" {
					// Responses: {"type":"function","name":"x"} →
					// chat: {"type":"function","function":{"name":"x"}}
					chatRequest["tool_choice"] = map[string]interface{}{
						"type": "function",
						"function": map[string]interface{}{
							"name": tc.Get("name").String(),
						},
					}
				}
			}
			if ptc := gjson.GetBytes(body, "parallel_tool_calls"); ptc.Exists() {
				chatRequest["parallel_tool_calls"] = ptc.Value()
			}
		}
	}

	if maxOutput := gjson.GetBytes(body, "max_output_tokens"); maxOutput.Exists() && maxOutput.Int() > 0 {
		chatRequest["max_tokens"] = maxOutput.Int()
	}
	for _, passthrough := range []string{"temperature", "top_p"} {
		if v := gjson.GetBytes(body, passthrough); v.Exists() {
			chatRequest[passthrough] = v.Value()
		}
	}
	if gjson.GetBytes(body, "stream").Bool() {
		chatRequest["stream"] = true
		// 终块 usage 是 ai-token-report/ai-statistics 计量与 response.completed
		// 事件里 usage 字段的唯一来源，桥接链路必须带上。
		chatRequest["stream_options"] = map[string]interface{}{"include_usage": true}
	}

	converted, err := json.Marshal(chatRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal converted chat request: %v", err)
	}
	return converted, nil
}

// responsesInputItemToChatMessage 把单个 Responses input item 映射为 chat
// message。reasoning item 返回 nil（丢弃）；未知类型同样丢弃并留 debug 日志，
// 保证对 codex 未来新增 item 类型的前向兼容（宁丢不挂）。
func responsesInputItemToChatMessage(item gjson.Result) (map[string]interface{}, error) {
	switch item.Get("type").String() {
	case "message":
		role := item.Get("role").String()
		switch role {
		case "developer":
			role = "system"
		case "user", "assistant", "system":
		default:
			return nil, fmt.Errorf("unsupported responses message role: %s", role)
		}
		content := responsesContentToChat(item.Get("content"))
		if content == nil {
			return nil, fmt.Errorf("unsupported content shape in %s message", item.Get("role").String())
		}
		msg := map[string]interface{}{"role": role, "content": content}
		if name := item.Get("name"); name.Exists() && name.String() != "" {
			msg["name"] = name.String()
		}
		return msg, nil

	case "function_call":
		toolCalls := []interface{}{
			map[string]interface{}{
				"id":   item.Get("call_id").String(),
				"type": "function",
				"function": map[string]interface{}{
					"name":      item.Get("name").String(),
					"arguments": item.Get("arguments").String(),
				},
			},
		}
		return map[string]interface{}{
			"role":       "assistant",
			"tool_calls": toolCalls,
		}, nil

	case "function_call_output":
		output := item.Get("output")
		var content string
		switch {
		case output.Type == gjson.String:
			content = output.String()
		case output.IsArray():
			// {"content":[{"type":"output_text","text":"..."}]} 形态：拼接文本
			var sb strings.Builder
			for _, part := range output.Array() {
				if part.Get("type").String() == "output_text" || part.Get("text").Exists() {
					sb.WriteString(part.Get("text").String())
				}
			}
			content = sb.String()
		case output.Get("content").Exists():
			content = output.Get("content").String()
		default:
			content = ""
		}
		return map[string]interface{}{
			"role":         "tool",
			"tool_call_id": item.Get("call_id").String(),
			"content":      content,
		}, nil

	case "reasoning":
		// 请求向主循环已把 reasoning 回填到所属 assistant 轮次；此处仅作防御
		// 兜底（直调本函数的路径），保持丢弃语义。
		return nil, nil

	case "item_reference":
		return nil, fmt.Errorf("responses-to-chat bridge cannot resolve item_reference (stateful input)")

	default:
		log.Debugf("[Responses→Chat] drop unknown input item type: %s", item.Get("type").String())
		return nil, nil
	}
}

// reasoningTextFromItem 提取 Responses reasoning item 携带的思考文本。优先
// summary[]（codex 回放的标准形态，summary_text part），兼容 content[]（部分
// 客户端以 reasoning_text part 形态回传原文）。
func reasoningTextFromItem(item gjson.Result) string {
	var parts []string
	for _, group := range []gjson.Result{item.Get("summary"), item.Get("content")} {
		if !group.Exists() || !group.IsArray() {
			continue
		}
		for _, part := range group.Array() {
			if text := part.Get("text").String(); text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "\n\n")
}

// joinReasoningText 把同一 assistant 轮次缓冲到的多段思考文本合并为单个
// reasoning_content（并行调用被 reasoning item 分隔时会产生多段）。
func joinReasoningText(parts []string) string {
	return strings.Join(parts, "\n\n")
}

// responsesContentToChat 把 Responses message content（string 或 content part
// 数组）映射为 chat content（string 或多模态 part 数组）。无法识别时返回 nil。
func responsesContentToChat(content gjson.Result) interface{} {
	switch content.Type {
	case gjson.String:
		return content.String()
	case gjson.JSON:
		if !content.IsArray() {
			return nil
		}
		parts := make([]interface{}, 0, len(content.Array()))
		for _, part := range content.Array() {
			switch part.Get("type").String() {
			case "input_text", "output_text", "summary_text", "refusal":
				text := part.Get("text").String()
				if part.Get("type").String() == "refusal" {
					text = part.Get("refusal").String()
				}
				parts = append(parts, map[string]interface{}{"type": "text", "text": text})
			case "input_image":
				img := map[string]interface{}{}
				if u := part.Get("image_url"); u.Exists() {
					img["url"] = u.String()
				} else if d := part.Get("data"); d.Exists() {
					img["url"] = "data:" + part.Get("mime_type").String() + ";base64," + d.String()
				} else {
					continue
				}
				parts = append(parts, map[string]interface{}{"type": "image_url", "image_url": img})
			default:
				log.Debugf("[Responses→Chat] drop unknown content part type: %s", part.Get("type").String())
			}
		}
		if len(parts) == 0 {
			// 全部 part 被丢弃时退化成空文本，保持消息占位（tool 消息轮次依赖顺序）。
			return ""
		}
		return parts
	default:
		return nil
	}
}

// ---------------------------------------------------------------------------
// 响应向：chat → Responses
// ---------------------------------------------------------------------------

type responsesUsage struct {
	InputTokens         int                            `json:"input_tokens"`
	InputTokensDetails  *responsesInputTokensDetails   `json:"input_tokens_details,omitempty"`
	OutputTokens        int                            `json:"output_tokens"`
	OutputTokensDetails *responsesOutputTokensDetails  `json:"output_tokens_details,omitempty"`
	TotalTokens         int                            `json:"total_tokens"`
}

type responsesInputTokensDetails struct {
	CachedTokens int `json:"cached_tokens"`
}

type responsesOutputTokensDetails struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}

type responsesOutputText struct {
	Type        string        `json:"type"`
	Text        string        `json:"text"`
	Annotations []interface{} `json:"annotations"`
}

type responsesMessageItem struct {
	Id      string                `json:"id"`
	Type    string                `json:"type"`
	Role    string                `json:"role"`
	Status  string                `json:"status"`
	Content []responsesOutputText `json:"content"`
}

type responsesFunctionCallItem struct {
	Id        string `json:"id"`
	Type      string `json:"type"`
	Status    string `json:"status"`
	CallId    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// responsesReasoningItem 承载 chat 上游回吐的思考内容（reasoning_content），
// 以 OpenAI Responses 标准 reasoning item 形态交给 codex 存档；codex 会在
// 后续轮次原样回放，请求向再降维回 assistant.reasoning_content（OPE-9733）。
type responsesReasoningItem struct {
	Id      string                      `json:"id"`
	Type    string                      `json:"type"`
	Summary []responsesReasoningSummary `json:"summary"`
}

type responsesReasoningSummary struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type responsesResponseBody struct {
	Id                 string          `json:"id"`
	Object             string          `json:"object"`
	CreatedAt          int64           `json:"created_at"`
	Status             string          `json:"status"`
	Error              interface{}     `json:"error"`
	IncompleteDetails  interface{}     `json:"incomplete_details"`
	Instructions       interface{}     `json:"instructions"`
	MaxOutputTokens    interface{}     `json:"max_output_tokens"`
	Model              string          `json:"model"`
	Output             []interface{}   `json:"output"`
	ParallelToolCalls  interface{}     `json:"parallel_tool_calls"`
	PreviousResponseId interface{}     `json:"previous_response_id"`
	Reasoning          interface{}     `json:"reasoning"`
	Store              bool            `json:"store"`
	Temperature        interface{}     `json:"temperature"`
	Text               interface{}     `json:"text"`
	ToolChoice         interface{}     `json:"tool_choice"`
	Tools              interface{}     `json:"tools"`
	TopP               interface{}     `json:"top_p"`
	Truncation         string          `json:"truncation"`
	Usage              *responsesUsage `json:"usage"`
	Metadata           map[string]interface{} `json:"metadata"`
}

// ConvertChatResponseToResponses 把上游 chat completion JSON 升维为 Responses
// 对象（非流式路径）。
func ConvertChatResponseToResponses(body []byte) ([]byte, error) {
	var chat chatCompletionResponse
	if err := json.Unmarshal(body, &chat); err != nil {
		return nil, fmt.Errorf("failed to unmarshal upstream chat response: %v", err)
	}
	if chat.Id == "" && len(chat.Choices) == 0 {
		return nil, fmt.Errorf("upstream response is not a chat completion object")
	}

	resp := newResponsesBody(chat.Id, chat.Model, chat.Created, chat.Usage)

	finishReason := ""
	if len(chat.Choices) > 0 {
		finishReason = derefString(chat.Choices[0].FinishReason)
		resp.Output = chatChoiceToResponsesOutput(&chat.Choices[0])
	} else {
		resp.Output = []interface{}{}
	}
	applyFinishStatus(resp, finishReason)

	converted, err := json.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal responses object: %v", err)
	}
	return converted, nil
}

// ChatToResponsesStreamConverter 是流式路径的有状态转换器：一个请求一个实例
// （存 http context），把 chat SSE chunk 逐段翻译成 Responses 事件流。
// 输出事件序列与 OpenAI 官方流式语义对齐：
//
//	response.created → [output_item.added (message) → content_part.added →
//	output_text.delta*] / [output_item.added (function_call) →
//	function_call_arguments.delta*] → 各 item 的 done 事件 → response.completed
type ChatToResponsesStreamConverter struct {
	lineBuf string

	responseID string
	model      string
	createdAt  int64

	createdSent bool
	seq         int

	message struct {
		opened bool
		closed bool
		id     string
		text   strings.Builder
	}

	reasoning struct {
		opened bool
		closed bool
		id     string
		text   strings.Builder
	}
	reasoningSlot    int
	reasoningSlotted bool

	toolCalls   map[string]*streamToolCallState
	nextItemNum int
	// outputSlots 保持输出 item 的稳定 output_index：message 固定占用一个
	// 槽位，function_call 按 chat tool_calls 数组下标一一对应。
	messageSlot    int
	messageSlotted bool
	toolSlots      map[string]int

	finishReason   string
	usage          *usage
	completedSent  bool
}

type streamToolCallState struct {
	chatIndex  int
	fcID       string
	callID     string
	name       string
	arguments  strings.Builder
	opened     bool
	closed     bool
}

func NewChatToResponsesStreamConverter() *ChatToResponsesStreamConverter {
	return &ChatToResponsesStreamConverter{
		toolCalls: make(map[string]*streamToolCallState),
		toolSlots: make(map[string]int),
	}
}

// ProcessChunk 消费一段上游 SSE 字节，返回翻译后的 Responses SSE 字节。
// isLastChunk=true 且尚未发出 response.completed 时强制收尾（防御上游不发
// [DONE] 的场景）。
func (c *ChatToResponsesStreamConverter) ProcessChunk(chunk []byte, isLastChunk bool) ([]byte, error) {
	c.lineBuf += string(chunk)
	var out strings.Builder

	for {
		idx := strings.IndexByte(c.lineBuf, '\n')
		if idx < 0 {
			break
		}
		line := strings.TrimRight(c.lineBuf[:idx], "\r")
		c.lineBuf = c.lineBuf[idx+1:]

		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ":") || strings.HasPrefix(line, "event:") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			c.emitCompleted(&out)
			continue
		}

		var chat chatCompletionResponse
		if err := json.Unmarshal([]byte(data), &chat); err != nil {
			log.Debugf("[Chat→Responses] skip unparsable chat chunk: %v", err)
			continue
		}
		c.consumeChatChunk(&chat, &out)
	}

	if isLastChunk && !c.completedSent {
		c.emitCompleted(&out)
	}

	// 返回空切片（而非 nil）表示"本段无输出"：nil 语义在宿主框架里等价于
	// 不修改原始 chunk，会把未翻译的 chat SSE 泄漏给客户端。
	if out.Len() == 0 {
		return []byte{}, nil
	}
	return []byte(out.String()), nil
}

func (c *ChatToResponsesStreamConverter) consumeChatChunk(chat *chatCompletionResponse, out *strings.Builder) {
	if chat.Id != "" {
		if c.responseID == "" {
			c.responseID = chat.Id
		}
	}
	if chat.Model != "" && c.model == "" {
		c.model = chat.Model
	}
	if chat.Created != 0 && c.createdAt == 0 {
		c.createdAt = chat.Created
	}
	if chat.Usage != nil && chat.Usage.TotalTokens > 0 {
		c.usage = chat.Usage
	}
	if !c.createdSent {
		c.emitCreated(out)
	}

	if len(chat.Choices) == 0 {
		return
	}
	choice := &chat.Choices[0]
	if choice.FinishReason != nil && *choice.FinishReason != "" {
		c.finishReason = *choice.FinishReason
	}

	if choice.Delta != nil {
		delta := choice.Delta
		if delta.Content != nil {
			text := contentToString(delta.Content)
			if text != "" {
				c.ensureMessageOpened(out)
				c.message.text.WriteString(text)
				c.writeEvent(out, "response.output_text.delta", map[string]interface{}{
					"item_id":       c.message.id,
					"output_index":  c.messageSlot,
					"content_index": 0,
					"delta":         text,
				})
			}
		}
		if text := chatMessageReasoningText(delta); text != "" {
			// 思考增量升维成 Responses reasoning item（summary_text 形态）交给
			// codex 存档回放——thinking 上游要求多轮回传 reasoning_content，
			// 丢弃会导致下一轮必 400（OPE-9733）。
			c.ensureReasoningOpened(out)
			c.reasoning.text.WriteString(text)
			c.writeEvent(out, "response.reasoning_summary_text.delta", map[string]interface{}{
				"item_id":       c.reasoning.id,
				"output_index":  c.reasoningSlot,
				"summary_index": 0,
				"delta":         text,
			})
		}
		for i := range delta.ToolCalls {
			c.consumeToolCallDelta(&delta.ToolCalls[i], out)
		}
	}
}

func (c *ChatToResponsesStreamConverter) consumeToolCallDelta(tc *toolCall, out *strings.Builder) {
	key := fmt.Sprintf("%d", tc.Index)
	state, ok := c.toolCalls[key]
	if !ok {
		state = &streamToolCallState{
			chatIndex: tc.Index,
			fcID:      tc.Id,
			callID:    tc.Id,
			name:      tc.Function.Name,
		}
		c.toolCalls[key] = state
		c.toolSlots[key] = c.nextSlot()
	}
	if tc.Id != "" && state.callID == "" {
		state.callID = tc.Id
	}
	if tc.Function.Name != "" && state.name == "" {
		state.name = tc.Function.Name
	}

	if !state.opened {
		state.opened = true
		c.writeEvent(out, "response.output_item.added", map[string]interface{}{
			"output_index": c.toolSlots[key],
			"item": map[string]interface{}{
				"id":         state.fcID,
				"type":       "function_call",
				"status":     responsesStatusInProgress,
				"call_id":    state.callID,
				"name":       state.name,
				"arguments":  "",
			},
		})
	}
	if tc.Function.Arguments != "" {
		state.arguments.WriteString(tc.Function.Arguments)
		c.writeEvent(out, "response.function_call_arguments.delta", map[string]interface{}{
			"item_id":      state.fcID,
			"output_index": c.toolSlots[key],
			"delta":        tc.Function.Arguments,
		})
	}
}

func (c *ChatToResponsesStreamConverter) ensureReasoningOpened(out *strings.Builder) {
	if c.reasoning.opened {
		return
	}
	if !c.reasoningSlotted {
		c.reasoningSlot = c.nextSlot()
		c.reasoningSlotted = true
	}
	c.reasoning.opened = true
	c.reasoning.id = fmt.Sprintf("rs-%s", c.sanitizeID(c.responseID))
	c.writeEvent(out, "response.output_item.added", map[string]interface{}{
		"output_index": c.reasoningSlot,
		"item": map[string]interface{}{
			"id":      c.reasoning.id,
			"type":    "reasoning",
			"summary": []interface{}{},
			"content": []interface{}{},
		},
	})
	c.writeEvent(out, "response.reasoning_summary_part.added", map[string]interface{}{
		"item_id":       c.reasoning.id,
		"output_index":  c.reasoningSlot,
		"summary_index": 0,
		"part": map[string]interface{}{
			"type": "summary_text",
			"text": "",
		},
	})
}

func (c *ChatToResponsesStreamConverter) closeReasoning(out *strings.Builder) {
	c.reasoning.closed = true
	text := c.reasoning.text.String()
	c.writeEvent(out, "response.reasoning_summary_text.done", map[string]interface{}{
		"item_id":       c.reasoning.id,
		"output_index":  c.reasoningSlot,
		"summary_index": 0,
		"text":          text,
	})
	c.writeEvent(out, "response.output_item.done", map[string]interface{}{
		"output_index": c.reasoningSlot,
		"item":         newReasoningItem(c.reasoning.id, text),
	})
}

func (c *ChatToResponsesStreamConverter) ensureMessageOpened(out *strings.Builder) {
	if c.message.opened {
		return
	}
	if !c.messageSlotted {
		c.messageSlot = c.nextSlot()
		c.messageSlotted = true
	}
	c.message.opened = true
	c.message.id = fmt.Sprintf("msg-%s", c.sanitizeID(c.responseID))
	c.writeEvent(out, "response.output_item.added", map[string]interface{}{
		"output_index": c.messageSlot,
		"item": map[string]interface{}{
			"id":     c.message.id,
			"type":   "message",
			"role":   "assistant",
			"status": responsesStatusInProgress,
			"content": []interface{}{},
		},
	})
	c.writeEvent(out, "response.content_part.added", map[string]interface{}{
		"item_id":       c.message.id,
		"output_index":  c.messageSlot,
		"content_index": 0,
		"part": map[string]interface{}{
			"type":        "output_text",
			"text":        "",
			"annotations": []interface{}{},
		},
	})
}

// nextSlot 分配下一个输出 item 的 output_index：message 与 function_call 按
// 首次出现顺序递增拿号，保证与 OpenAI 官方流的序号语义一致。
func (c *ChatToResponsesStreamConverter) nextSlot() int {
	s := c.nextItemNum
	c.nextItemNum++
	return s
}

func (c *ChatToResponsesStreamConverter) emitCreated(out *strings.Builder) {
	c.createdSent = true
	c.responseID = c.sanitizeID(c.responseID)
	skeleton := newResponsesBody(c.responseID, c.model, c.createdAt, nil)
	skeleton.Status = responsesStatusInProgress
	skeleton.Output = []interface{}{}
	c.writeEvent(out, "response.created", map[string]interface{}{"response": skeleton})
	c.writeEvent(out, "response.in_progress", map[string]interface{}{"response": skeleton})
}

func (c *ChatToResponsesStreamConverter) emitCompleted(out *strings.Builder) {
	if c.completedSent {
		return
	}
	c.completedSent = true

	// 收尾所有未关闭 item（正常路径 finish_reason chunk 已收，这里兜底）。
	if c.reasoning.opened && !c.reasoning.closed {
		c.closeReasoning(out)
	}
	if c.message.opened && !c.message.closed {
		c.closeMessage(out)
	}
	for _, key := range c.sortedToolKeys() {
		state := c.toolCalls[key]
		if state.opened && !state.closed {
			c.closeToolCall(out, state)
		}
	}

	resp := newResponsesBody(c.responseID, c.model, c.createdAt, c.usage)
	resp.Output = c.buildFinalOutput()
	applyFinishStatus(resp, c.finishReason)
	c.writeEvent(out, "response.completed", map[string]interface{}{"response": resp})
}

func (c *ChatToResponsesStreamConverter) closeMessage(out *strings.Builder) {
	c.message.closed = true
	text := c.message.text.String()
	c.writeEvent(out, "response.output_text.done", map[string]interface{}{
		"item_id":       c.message.id,
		"output_index":  c.messageSlot,
		"content_index": 0,
		"text":          text,
	})
	c.writeEvent(out, "response.content_part.done", map[string]interface{}{
		"item_id":       c.message.id,
		"output_index":  c.messageSlot,
		"content_index": 0,
		"part": map[string]interface{}{
			"type":        "output_text",
			"text":        text,
			"annotations": []interface{}{},
		},
	})
	c.writeEvent(out, "response.output_item.done", map[string]interface{}{
		"output_index": c.messageSlot,
		"item": responsesMessageItem{
			Id:     c.message.id,
			Type:   "message",
			Role:   "assistant",
			Status: responsesStatusCompleted,
			Content: []responsesOutputText{{
				Type:        "output_text",
				Text:        text,
				Annotations: []interface{}{},
			}},
		},
	})
}

func (c *ChatToResponsesStreamConverter) closeToolCall(out *strings.Builder, state *streamToolCallState) {
	state.closed = true
	c.writeEvent(out, "response.function_call_arguments.done", map[string]interface{}{
		"item_id":      state.fcID,
		"output_index": c.toolSlots[fmt.Sprintf("%d", state.chatIndex)],
		"arguments":    state.arguments.String(),
	})
	c.writeEvent(out, "response.output_item.done", map[string]interface{}{
		"output_index": c.toolSlots[fmt.Sprintf("%d", state.chatIndex)],
		"item": responsesFunctionCallItem{
			Id:        state.fcID,
			Type:      "function_call",
			Status:    responsesStatusCompleted,
			CallId:    state.callID,
			Name:      state.name,
			Arguments: state.arguments.String(),
		},
	})
}

func (c *ChatToResponsesStreamConverter) buildFinalOutput() []interface{} {
	output := make([]interface{}, 0, len(c.toolCalls)+2)
	if c.reasoning.opened {
		output = append(output, newReasoningItem(c.reasoning.id, c.reasoning.text.String()))
	}
	if c.message.opened && c.message.text.String() != "" {
		output = append(output, responsesMessageItem{
			Id:     c.message.id,
			Type:   "message",
			Role:   "assistant",
			Status: responsesStatusCompleted,
			Content: []responsesOutputText{{
				Type:        "output_text",
				Text:        c.message.text.String(),
				Annotations: []interface{}{},
			}},
		})
	}
	for _, key := range c.sortedToolKeys() {
		state := c.toolCalls[key]
		output = append(output, responsesFunctionCallItem{
			Id:        state.fcID,
			Type:      "function_call",
			Status:    responsesStatusCompleted,
			CallId:    state.callID,
			Name:      state.name,
			Arguments: state.arguments.String(),
		})
	}
	return output
}

func (c *ChatToResponsesStreamConverter) sortedToolKeys() []string {
	keys := make([]string, 0, len(c.toolCalls))
	for k := range c.toolCalls {
		keys = append(keys, k)
	}
	// tool_calls 的 chat index 天然单调，按数值排序保证输出顺序稳定。
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if c.toolCalls[keys[j]].chatIndex < c.toolCalls[keys[i]].chatIndex {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}

func (c *ChatToResponsesStreamConverter) writeEvent(out *strings.Builder, eventType string, payload map[string]interface{}) {
	c.seq++
	payload["type"] = eventType
	payload["sequence_number"] = c.seq
	data, err := json.Marshal(payload)
	if err != nil {
		log.Errorf("[Chat→Responses] failed to marshal event %s: %v", eventType, err)
		return
	}
	out.WriteString("event: ")
	out.WriteString(eventType)
	out.WriteString("\ndata: ")
	out.Write(data)
	out.WriteString("\n\n")
}

func (c *ChatToResponsesStreamConverter) sanitizeID(id string) string {
	if id == "" {
		return fmt.Sprintf("resp-%d", c.seq)
	}
	return id
}

// ---------------------------------------------------------------------------
// 共享辅助
// ---------------------------------------------------------------------------

func newResponsesBody(id, model string, createdAt int64, u *usage) *responsesResponseBody {
	resp := &responsesResponseBody{
		Id:        id,
		Object:    responsesObjectType,
		CreatedAt: createdAt,
		Status:    responsesStatusCompleted,
		Model:     model,
		Tools:     []interface{}{},
		ToolChoice: "auto",
		Truncation: "disabled",
		Store:      false,
		Text:       map[string]interface{}{"format": map[string]interface{}{"type": "text"}},
		Reasoning:  map[string]interface{}{"effort": nil, "summary": nil},
		Metadata:   map[string]interface{}{},
	}
	if !strings.HasPrefix(id, "resp_") {
		resp.Id = "resp_" + strings.TrimPrefix(id, "chatcmpl-")
	}
	if u != nil {
		resp.Usage = &responsesUsage{
			InputTokens:  u.PromptTokens,
			OutputTokens: u.CompletionTokens,
			TotalTokens:  u.TotalTokens,
		}
		if u.PromptTokensDetails != nil && u.PromptTokensDetails.CachedTokens > 0 {
			resp.Usage.InputTokensDetails = &responsesInputTokensDetails{CachedTokens: u.PromptTokensDetails.CachedTokens}
		}
		if u.CompletionTokensDetails != nil && u.CompletionTokensDetails.ReasoningTokens > 0 {
			resp.Usage.OutputTokensDetails = &responsesOutputTokensDetails{ReasoningTokens: u.CompletionTokensDetails.ReasoningTokens}
		}
	}
	return resp
}

func applyFinishStatus(resp *responsesResponseBody, finishReason string) {
	switch finishReason {
	case "length":
		resp.Status = responsesStatusIncomplete
		resp.IncompleteDetails = map[string]interface{}{"reason": incompleteReasonMaxOutputTokens}
	case "content_filter":
		resp.Status = responsesStatusIncomplete
		resp.IncompleteDetails = map[string]interface{}{"reason": incompleteReasonContentFilter}
	default:
		resp.Status = responsesStatusCompleted
	}
}

func chatChoiceToResponsesOutput(choice *chatCompletionChoice) []interface{} {
	output := make([]interface{}, 0, 3)
	if choice.Message != nil {
		// 思考内容放在 output 首位：与上游生成顺序一致（先思考后输出），
		// codex 回放时请求向据此归位到同一 assistant 轮次。
		if text := chatMessageReasoningText(choice.Message); text != "" {
			output = append(output, newReasoningItem("rs-0", text))
		}
		if text := contentToString(choice.Message.Content); text != "" {
			output = append(output, responsesMessageItem{
				Id:     "msg-0",
				Type:   "message",
				Role:   "assistant",
				Status: responsesStatusCompleted,
				Content: []responsesOutputText{{
					Type:        "output_text",
					Text:        text,
					Annotations: []interface{}{},
				}},
			})
		}
		for _, tc := range choice.Message.ToolCalls {
			output = append(output, responsesFunctionCallItem{
				Id:        tc.Id,
				Type:      "function_call",
				Status:    responsesStatusCompleted,
				CallId:    tc.Id,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			})
		}
	}
	return output
}

// newReasoningItem 构造带单段 summary_text 的标准 reasoning item。id 需与
// 同一响应里已发出的 added 事件保持一致（codex 按 id 归档 item）。
func newReasoningItem(id, text string) responsesReasoningItem {
	return responsesReasoningItem{
		Id:      id,
		Type:    "reasoning",
		Summary: []responsesReasoningSummary{{Type: "summary_text", Text: text}},
	}
}

func contentToString(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []interface{}:
		var sb strings.Builder
		for _, part := range v {
			if m, ok := part.(map[string]interface{}); ok {
				if t, ok := m["text"].(string); ok {
					sb.WriteString(t)
				}
			}
		}
		return sb.String()
	default:
		return ""
	}
}

// chatMessageReasoningText 取 chat 消息/增量里的思考文本，兼容
// reasoning_content（DeepSeek/Qwen 系）与 reasoning 两种字段形态。
func chatMessageReasoningText(m *chatMessage) string {
	if m.ReasoningContent != "" {
		return m.ReasoningContent
	}
	return m.Reasoning
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
