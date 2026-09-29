package provider

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

// ---------------------------------------------------------------------------
// 请求向：Responses → Chat
// ---------------------------------------------------------------------------

func TestConvertResponsesRequestToChat_BasicStringInput(t *testing.T) {
	body := `{
		"model": "deepseek-v4-flash",
		"instructions": "You are a coding agent.",
		"input": "hi",
		"max_output_tokens": 1024,
		"store": false,
		"stream": true,
		"temperature": 0.2
	}`
	out, err := ConvertResponsesRequestToChat([]byte(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	g := gjson.ParseBytes(out)
	if g.Get("model").String() != "deepseek-v4-flash" {
		t.Errorf("model = %v", g.Get("model").String())
	}
	if int(g.Get("max_tokens").Int()) != 1024 {
		t.Errorf("max_tokens = %v, want 1024", g.Get("max_tokens"))
	}
	if !g.Get("stream").Bool() {
		t.Error("stream should be true")
	}
	if !g.Get("stream_options.include_usage").Bool() {
		t.Error("stream_options.include_usage must be forced true for billing")
	}
	msgs := g.Get("messages").Array()
	if len(msgs) != 2 {
		t.Fatalf("messages len = %d, want 2 (instructions + user)", len(msgs))
	}
	if msgs[0].Get("role").String() != "system" || msgs[0].Get("content").String() != "You are a coding agent." {
		t.Errorf("system message wrong: %v", msgs[0])
	}
	if msgs[1].Get("role").String() != "user" || msgs[1].Get("content").String() != "hi" {
		t.Errorf("user message wrong: %v", msgs[1])
	}
	if g.Get("temperature").Float() != 0.2 {
		t.Errorf("temperature = %v", g.Get("temperature"))
	}
	// store 等 responses 专属字段不得出现在 chat 请求里
	for _, forbidden := range []string{"store", "instructions", "input", "max_output_tokens", "previous_response_id"} {
		if g.Get(forbidden).Exists() {
			t.Errorf("responses-only field %q leaked into chat request", forbidden)
		}
	}
}

func TestConvertResponsesRequestToChat_CodexStatelessShape(t *testing.T) {
	// codex disable_response_storage 下的真实形态：input 是 item 数组，含
	// 历史 function_call / function_call_output 与 content parts。
	body := `{
		"model": "deepseek-v4-flash",
		"instructions": "sys prompt",
		"input": [
			{"type":"message","role":"user","content":[{"type":"input_text","text":"list files"}]},
			{"type":"reasoning","summary":[]},
			{"type":"function_call","name":"shell","arguments":"{\"cmd\":\"ls\"}","call_id":"call_1"},
			{"type":"function_call_output","call_id":"call_1","output":"file_a\nfile_b"},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"thanks"}]}
		],
		"tools": [
			{"type":"function","name":"shell","description":"run a shell","parameters":{"type":"object"},"strict":true}
		],
		"tool_choice": {"type":"function","name":"shell"},
		"parallel_tool_calls": false,
		"stream": true,
		"store": false
	}`
	out, err := ConvertResponsesRequestToChat([]byte(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	g := gjson.ParseBytes(out)

	msgs := g.Get("messages").Array()
	// system + user(list files) + assistant(function_call) + tool(output) + user(thanks)
	if len(msgs) != 5 {
		t.Fatalf("messages len = %d, want 5: %s", len(msgs), out)
	}
	if msgs[0].Get("role").String() != "system" {
		t.Errorf("msg0 role = %v", msgs[0].Get("role"))
	}
	if msgs[1].Get("content.0.type").String() != "text" || msgs[1].Get("content.0.text").String() != "list files" {
		t.Errorf("user content parts wrong: %v", msgs[1].Get("content"))
	}
	if msgs[2].Get("role").String() != "assistant" {
		t.Errorf("msg2 role = %v", msgs[2].Get("role"))
	}
	tc := msgs[2].Get("tool_calls.0")
	if tc.Get("id").String() != "call_1" || tc.Get("function.name").String() != "shell" {
		t.Errorf("assistant tool_calls wrong: %v", tc)
	}
	if tc.Get("function.arguments").String() != `{"cmd":"ls"}` {
		t.Errorf("function arguments wrong: %v", tc.Get("function.arguments"))
	}
	if msgs[3].Get("role").String() != "tool" || msgs[3].Get("tool_call_id").String() != "call_1" {
		t.Errorf("tool message wrong: %v", msgs[3])
	}
	if msgs[3].Get("content").String() != "file_a\nfile_b" {
		t.Errorf("tool content = %q", msgs[3].Get("content").String())
	}

	tools := g.Get("tools").Array()
	if len(tools) != 1 || tools[0].Get("function.name").String() != "shell" {
		t.Fatalf("tools wrong: %v", g.Get("tools"))
	}
	if tools[0].Get("function.strict").Exists() || tools[0].Get("strict").Exists() {
		t.Error("strict field must be stripped (chat upstreams reject unknown openai-only fields)")
	}
	if g.Get("tool_choice.type").String() != "function" || g.Get("tool_choice.function.name").String() != "shell" {
		t.Errorf("tool_choice wrong: %v", g.Get("tool_choice"))
	}
	if g.Get("parallel_tool_calls").Bool() {
		t.Error("parallel_tool_calls should be false")
	}
}

func TestConvertResponsesRequestToChat_RejectsStateful(t *testing.T) {
	cases := map[string]string{
		"previous_response_id": `{"model":"m","input":"hi","previous_response_id":"resp_123"}`,
		"store":                `{"model":"m","input":"hi","store":true}`,
		"background":           `{"model":"m","input":"hi","background":true}`,
		"item_reference":       `{"model":"m","input":[{"type":"item_reference","id":"msg_1"}]}`,
	}
	for name, body := range cases {
		if _, err := ConvertResponsesRequestToChat([]byte(body)); err == nil {
			t.Errorf("%s: expected error, got nil", name)
		}
	}
}

func TestConvertResponsesRequestToChat_DeveloperRoleAndUnknownItem(t *testing.T) {
	body := `{"model":"m","input":[
		{"type":"message","role":"developer","content":"dev rules"},
		{"type":"web_search_call","action":{}},
		{"type":"message","role":"user","content":"q"}
	]}`
	out, err := ConvertResponsesRequestToChat([]byte(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	msgs := gjson.GetBytes(out, "messages").Array()
	if len(msgs) != 2 {
		t.Fatalf("messages len = %d, want 2 (developer→system, unknown dropped, user)", len(msgs))
	}
	if msgs[0].Get("role").String() != "system" {
		t.Errorf("developer should map to system, got %v", msgs[0].Get("role"))
	}
}

// ---------------------------------------------------------------------------
// 响应向（非流式）：Chat → Responses
// ---------------------------------------------------------------------------

func TestConvertChatResponseToResponses_Text(t *testing.T) {
	body := `{
		"id": "chatcmpl-abc123",
		"object": "chat.completion",
		"created": 1789441920,
		"model": "deepseek-v4-flash",
		"choices": [{"index":0,"message":{"role":"assistant","content":"hello world"},"finish_reason":"stop"}],
		"usage": {"prompt_tokens": 11, "completion_tokens": 7, "total_tokens": 18, "completion_tokens_details": {"reasoning_tokens": 3}}
	}`
	out, err := ConvertChatResponseToResponses([]byte(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	g := gjson.ParseBytes(out)
	if g.Get("object").String() != "response" {
		t.Errorf("object = %v", g.Get("object"))
	}
	if !strings.HasPrefix(g.Get("id").String(), "resp_") {
		t.Errorf("id = %v, want resp_ prefix", g.Get("id"))
	}
	if g.Get("status").String() != "completed" {
		t.Errorf("status = %v", g.Get("status"))
	}
	msg := g.Get("output.0")
	if msg.Get("type").String() != "message" || msg.Get("role").String() != "assistant" {
		t.Errorf("output item wrong: %v", msg)
	}
	if msg.Get("content.0.type").String() != "output_text" || msg.Get("content.0.text").String() != "hello world" {
		t.Errorf("content wrong: %v", msg.Get("content"))
	}
	u := g.Get("usage")
	if u.Get("input_tokens").Int() != 11 || u.Get("output_tokens").Int() != 7 || u.Get("total_tokens").Int() != 18 {
		t.Errorf("usage mapping wrong: %v", u)
	}
	if u.Get("output_tokens_details.reasoning_tokens").Int() != 3 {
		t.Errorf("reasoning tokens lost: %v", u)
	}
}

func TestConvertChatResponseToResponses_ToolCallsAndLength(t *testing.T) {
	body := `{
		"id": "chatcmpl-x",
		"created": 1,
		"model": "m",
		"choices": [{"index":0,"message":{"role":"assistant","content":null,"tool_calls":[
			{"id":"call_1","type":"function","function":{"name":"shell","arguments":"{\"cmd\":\"ls\"}"}}
		]},"finish_reason":"tool_calls"}],
		"usage": {"prompt_tokens": 5, "completion_tokens": 2, "total_tokens": 7}
	}`
	out, err := ConvertChatResponseToResponses([]byte(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	g := gjson.ParseBytes(out)
	fc := g.Get("output.0")
	if fc.Get("type").String() != "function_call" {
		t.Fatalf("output.0 type = %v, want function_call", fc.Get("type"))
	}
	if fc.Get("call_id").String() != "call_1" || fc.Get("name").String() != "shell" {
		t.Errorf("function_call fields wrong: %v", fc)
	}
	if g.Get("status").String() != "completed" {
		t.Errorf("tool_calls finish should be completed, got %v", g.Get("status"))
	}

	lengthBody := strings.Replace(string(body), `"finish_reason":"tool_calls"`, `"finish_reason":"length"`, 1)
	out2, _ := ConvertChatResponseToResponses([]byte(lengthBody))
	if gjson.GetBytes(out2, "status").String() != "incomplete" {
		t.Errorf("length finish should be incomplete")
	}
	if gjson.GetBytes(out2, "incomplete_details.reason").String() != "max_output_tokens" {
		t.Errorf("incomplete reason wrong: %v", gjson.GetBytes(out2, "incomplete_details.reason"))
	}
}

func TestConvertChatResponseToResponses_RejectsGarbage(t *testing.T) {
	if _, err := ConvertChatResponseToResponses([]byte(`{"error":{"message":"boom"}}`)); err == nil {
		t.Error("expected error for non-chat body")
	}
}

// ---------------------------------------------------------------------------
// 响应向（流式）：Chat SSE → Responses 事件流
// ---------------------------------------------------------------------------

// parseSSE 把转换器输出按事件拆开，返回 (event, dataJSON) 序列。
func parseSSE(t *testing.T, raw []byte) []gjson.Result {
	t.Helper()
	var events []gjson.Result
	for _, block := range strings.Split(strings.TrimSpace(string(raw)), "\n\n") {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		var dataLine string
		for _, line := range strings.Split(block, "\n") {
			if strings.HasPrefix(line, "data: ") {
				dataLine = strings.TrimPrefix(line, "data: ")
			}
		}
		if dataLine == "" {
			t.Fatalf("SSE block without data line: %q", block)
		}
		events = append(events, gjson.Parse(dataLine))
	}
	return events
}

func eventTypes(events []gjson.Result) []string {
	types := make([]string, len(events))
	for i, e := range events {
		types[i] = e.Get("type").String()
	}
	return types
}

func feedChunks(t *testing.T, chunks []string) []gjson.Result {
	t.Helper()
	conv := NewChatToResponsesStreamConverter()
	var all strings.Builder
	for i, c := range chunks {
		out, err := conv.ProcessChunk([]byte(c), i == len(chunks)-1)
		if err != nil {
			t.Fatalf("chunk %d: unexpected error: %v", i, err)
		}
		if out == nil {
			t.Fatalf("chunk %d: nil output would pass original chunk through", i)
		}
		all.Write(out)
	}
	return parseSSE(t, []byte(all.String()))
}

func TestStreamConversion_TextHappyPath(t *testing.T) {
	chunks := []string{
		"data: {\"id\":\"chatcmpl-1\",\"created\":1789441920,\"model\":\"deepseek-v4-flash\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"\"},\"finish_reason\":null}]}\n\n",
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hel\"},\"finish_reason\":null}]}\n\n",
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"lo\"},\"finish_reason\":null}]}\n\n",
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n",
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[],\"usage\":{\"prompt_tokens\":9,\"completion_tokens\":4,\"total_tokens\":13}}\n\n",
		"data: [DONE]\n\n",
	}
	events := feedChunks(t, chunks)
	got := eventTypes(events)

	wantPrefix := []string{
		"response.created",
		"response.in_progress",
		"response.output_item.added",
		"response.content_part.added",
		"response.output_text.delta",
		"response.output_text.delta",
		"response.output_text.done",
		"response.content_part.done",
		"response.output_item.done",
		"response.completed",
	}
	if len(got) != len(wantPrefix) {
		t.Fatalf("event sequence mismatch:\ngot:  %v\nwant: %v", got, wantPrefix)
	}
	for i := range wantPrefix {
		if got[i] != wantPrefix[i] {
			t.Fatalf("event[%d] = %s, want %s\nfull: %v", i, got[i], wantPrefix[i], got)
		}
	}

	// delta 累积与顺序号
	var deltas []string
	for _, e := range events {
		if e.Get("type").String() == "response.output_text.delta" {
			deltas = append(deltas, e.Get("delta").String())
		}
	}
	if strings.Join(deltas, "") != "Hello" {
		t.Errorf("deltas = %v, want Hello", deltas)
	}

	completed := events[len(events)-1]
	if completed.Get("response.usage.input_tokens").Int() != 9 ||
		completed.Get("response.usage.output_tokens").Int() != 4 ||
		completed.Get("response.usage.total_tokens").Int() != 13 {
		t.Errorf("completed usage wrong: %v", completed.Get("response.usage"))
	}
	item := completed.Get("response.output.0")
	if item.Get("type").String() != "message" || item.Get("content.0.text").String() != "Hello" {
		t.Errorf("completed output wrong: %v", item)
	}
	if completed.Get("response.status").String() != "completed" {
		t.Errorf("completed status = %v", completed.Get("response.status"))
	}

	// sequence_number 单调
	prev := 0
	for _, e := range events {
		seq := int(e.Get("sequence_number").Int())
		if seq <= prev {
			t.Errorf("sequence_number not increasing: %d after %d", seq, prev)
		}
		prev = seq
	}
}

func TestStreamConversion_ToolCalls(t *testing.T) {
	chunks := []string{
		"data: {\"id\":\"c1\",\"created\":1,\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"shell\",\"arguments\":\"\"}}]},\"finish_reason\":null}]}\n\n",
		"data: {\"id\":\"c1\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"{\\\"cmd\\\":\\\"\"}}]},\"finish_reason\":null}]}\n\n",
		"data: {\"id\":\"c1\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"ls\\\"}\"}}]},\"finish_reason\":null}]}\n\n",
		"data: {\"id\":\"c1\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n",
		"data: {\"id\":\"c1\",\"choices\":[],\"usage\":{\"prompt_tokens\":4,\"completion_tokens\":6,\"total_tokens\":10}}\n\n",
		"data: [DONE]\n\n",
	}
	events := feedChunks(t, chunks)
	got := eventTypes(events)

	want := []string{
		"response.created",
		"response.in_progress",
		"response.output_item.added",
		"response.function_call_arguments.delta",
		"response.function_call_arguments.delta",
		"response.function_call_arguments.done",
		"response.output_item.done",
		"response.completed",
	}
	if len(got) != len(want) {
		t.Fatalf("event sequence mismatch:\ngot:  %v\nwant: %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("event[%d] = %s, want %s\nfull: %v", i, got[i], want[i], got)
		}
	}
	fcItem := events[len(events)-1].Get("response.output.0")
	if fcItem.Get("type").String() != "function_call" {
		t.Fatalf("final output.0 = %v", fcItem)
	}
	if fcItem.Get("call_id").String() != "call_1" || fcItem.Get("name").String() != "shell" {
		t.Errorf("function_call fields wrong: %v", fcItem)
	}
	if fcItem.Get("arguments").String() != `{"cmd":"ls"}` {
		t.Errorf("assembled arguments = %v, want {\"cmd\":\"ls\"}", fcItem.Get("arguments"))
	}
}

func TestStreamConversion_MixedTextAndTools(t *testing.T) {
	chunks := []string{
		"data: {\"id\":\"c2\",\"created\":1,\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"Let me check.\"},\"finish_reason\":null}]}\n\n",
		"data: {\"id\":\"c2\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_a\",\"type\":\"function\",\"function\":{\"name\":\"read\",\"arguments\":\"{\\\"path\\\":\\\"x\\\"}\"}},{\"index\":1,\"id\":\"call_b\",\"type\":\"function\",\"function\":{\"name\":\"write\",\"arguments\":\"{}\"}}]},\"finish_reason\":null}]}\n\n",
		"data: {\"id\":\"c2\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n",
		"data: [DONE]\n\n",
	}
	events := feedChunks(t, chunks)
	completed := events[len(events)-1]
	if completed.Get("type").String() != "response.completed" {
		t.Fatalf("last event = %v", completed.Get("type"))
	}
	output := completed.Get("response.output").Array()
	if len(output) != 3 {
		t.Fatalf("final output items = %d, want 3 (message + 2 function_calls): %v", len(output), completed.Get("response.output"))
	}
	if output[0].Get("type").String() != "message" || output[0].Get("content.0.text").String() != "Let me check." {
		t.Errorf("output[0] wrong: %v", output[0])
	}
	if output[1].Get("call_id").String() != "call_a" || output[2].Get("call_id").String() != "call_b" {
		t.Errorf("tool outputs wrong: %v / %v", output[1], output[2])
	}
	// output_index：message=0，两个 tool 依次 1、2
	if output[1].Get("id").String() == "" {
		t.Errorf("function_call id missing: %v", output[1])
	}
}

func TestStreamConversion_ChunkSplitAtLineBoundary(t *testing.T) {
	// TCP 分段把一行 data 撕成三段，转换器必须拼回完整 JSON 而不是泄漏原文。
	full := "data: {\"id\":\"c3\",\"created\":1,\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"torn line\"},\"finish_reason\":null}]}\n\ndata: [DONE]\n\n"
	parts := []string{full[:20], full[20:45], full[45:70], full[70:]}
	events := feedChunks(t, parts)

	var sawDelta bool
	for _, e := range events {
		if e.Get("type").String() == "response.output_text.delta" {
			sawDelta = true
			if e.Get("delta").String() != "torn line" {
				t.Errorf("delta = %q, want %q", e.Get("delta").String(), "torn line")
			}
		}
	}
	if !sawDelta {
		t.Fatal("torn-line delta never assembled")
	}
	if events[len(events)-1].Get("type").String() != "response.completed" {
		t.Error("completed missing after torn stream")
	}
}

func TestStreamConversion_MissingDoneFinalizesOnLastChunk(t *testing.T) {
	chunks := []string{
		"data: {\"id\":\"c4\",\"created\":1,\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hi\"},\"finish_reason\":\"stop\"}]}\n\n",
		// 上游异常断流：没有 [DONE]，isLastChunk 兜底收尾
	}
	conv := NewChatToResponsesStreamConverter()
	var all strings.Builder
	for i, c := range chunks {
		out, _ := conv.ProcessChunk([]byte(c), i == len(chunks)-1)
		all.Write(out)
	}
	events := parseSSE(t, []byte(all.String()))
	if len(events) == 0 {
		t.Fatal("no events emitted")
	}
	last := events[len(events)-1]
	if last.Get("type").String() != "response.completed" {
		t.Fatalf("force-finalize failed, last = %v", last.Get("type"))
	}
	if last.Get("response.output.0.content.0.text").String() != "hi" {
		t.Errorf("finalized output wrong: %v", last.Get("response.output"))
	}
}

func TestStreamConversion_EmptyOutputChunkIsNonNil(t *testing.T) {
	conv := NewChatToResponsesStreamConverter()
	out, err := conv.ProcessChunk([]byte("data: {\"partial\": "), false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil {
		t.Fatal("nil output would let the host pass the original chat chunk through")
	}
	if len(out) != 0 {
		t.Errorf("partial line should produce no output, got %q", out)
	}
}

// 编译期保证 wire 结构的 JSON 形态稳定（codex SDK 反序列化依赖字段名）。
func TestResponsesWireShapeStability(t *testing.T) {
	b, err := json.Marshal(responsesMessageItem{
		Id: "m", Type: "message", Role: "assistant", Status: "completed",
		Content: []responsesOutputText{{Type: "output_text", Text: "t", Annotations: []interface{}{}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	g := gjson.ParseBytes(b)
	for _, key := range []string{"id", "type", "role", "status", "content"} {
		if !g.Get(key).Exists() {
			t.Errorf("responsesMessageItem missing wire key %q: %s", key, b)
		}
	}
	fc, _ := json.Marshal(responsesFunctionCallItem{Id: "f", Type: "function_call", Status: "completed", CallId: "c", Name: "n", Arguments: "{}"})
	gf := gjson.ParseBytes(fc)
	for _, key := range []string{"id", "type", "status", "call_id", "name", "arguments"} {
		if !gf.Get(key).Exists() {
			t.Errorf("responsesFunctionCallItem missing wire key %q: %s", key, fc)
		}
	}
}

func TestConvertResponsesRequestToChat_ParallelFunctionCallsMergedIntoOneAssistant(t *testing.T) {
	// codex 一轮并行多工具调用：Responses 输入是连续多个 function_call item
	// （可被 reasoning item 隔开），后跟各自的 function_call_output。桥必须
	// 把相邻 function_call 合并为一条 assistant 消息的多元素 tool_calls——
	// 逐条映射会产出 assistant(tool_calls) 后紧跟另一条 assistant 的非法
	// chat 序列，DeepSeek 校验报 insufficient tool messages。
	body := `{
		"model": "deepseek-flash",
		"input": [
			{"type":"message","role":"user","content":[{"type":"input_text","text":"run both"}]},
			{"type":"reasoning","summary":[]},
			{"type":"function_call","name":"shell","arguments":"{\"cmd\":\"a\"}","call_id":"call_1"},
			{"type":"reasoning","summary":[]},
			{"type":"function_call","name":"shell","arguments":"{\"cmd\":\"b\"}","call_id":"call_2"},
			{"type":"function_call_output","call_id":"call_1","output":"out_a"},
			{"type":"function_call_output","call_id":"call_2","output":"out_b"},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"done"}]}
		],
		"tools": [{"type":"function","name":"shell","parameters":{"type":"object"}}],
		"stream": false,
		"store": false
	}`
	out, err := ConvertResponsesRequestToChat([]byte(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	g := gjson.ParseBytes(out)
	msgs := g.Get("messages").Array()
	// system? 无 instructions → user + assistant(双 tool_calls) + tool + tool + user
	if len(msgs) != 5 {
		t.Fatalf("messages len = %d, want 5: %s", len(msgs), out)
	}
	if msgs[0].Get("role").String() != "user" {
		t.Errorf("msg0 role = %v, want user", msgs[0].Get("role"))
	}
	assistant := msgs[1]
	if assistant.Get("role").String() != "assistant" {
		t.Fatalf("msg1 role = %v, want assistant (merged parallel calls)", assistant.Get("role"))
	}
	calls := assistant.Get("tool_calls").Array()
	if len(calls) != 2 {
		t.Fatalf("merged tool_calls len = %d, want 2: %s", len(calls), out)
	}
	if calls[0].Get("id").String() != "call_1" || calls[0].Get("function.arguments").String() != `{"cmd":"a"}` {
		t.Errorf("tool_call[0] wrong: %v", calls[0])
	}
	if calls[1].Get("id").String() != "call_2" || calls[1].Get("function.arguments").String() != `{"cmd":"b"}` {
		t.Errorf("tool_call[1] wrong: %v", calls[1])
	}
	if msgs[2].Get("role").String() != "tool" || msgs[2].Get("tool_call_id").String() != "call_1" {
		t.Errorf("msg2 wrong: %v", msgs[2])
	}
	if msgs[3].Get("role").String() != "tool" || msgs[3].Get("tool_call_id").String() != "call_2" {
		t.Errorf("msg3 wrong: %v", msgs[3])
	}
	if msgs[4].Get("role").String() != "user" || msgs[4].Get("content.0.text").String() != "done" {
		t.Errorf("msg4 wrong: %v", msgs[4])
	}
}

func TestConvertResponsesRequestToChat_InterleavedCallOutputPairsStaysValid(t *testing.T) {
	// call/output 交错形态（call_1, out_1, call_2, out_2）：合并遇 output
	// 即落盘，产出 assistant(call_1)/tool(call_1)/assistant(call_2)/tool(call_2)，
	// 每段各自合法。
	body := `{
		"model": "deepseek-flash",
		"input": [
			{"type":"function_call","name":"shell","arguments":"{}","call_id":"c1"},
			{"type":"function_call_output","call_id":"c1","output":"o1"},
			{"type":"function_call","name":"shell","arguments":"{}","call_id":"c2"},
			{"type":"function_call_output","call_id":"c2","output":"o2"}
		],
		"store": false
	}`
	out, err := ConvertResponsesRequestToChat([]byte(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	g := gjson.ParseBytes(out)
	msgs := g.Get("messages").Array()
	if len(msgs) != 4 {
		t.Fatalf("messages len = %d, want 4: %s", len(msgs), out)
	}
	roles := []string{}
	for _, m := range msgs {
		roles = append(roles, m.Get("role").String())
	}
	want := []string{"assistant", "tool", "assistant", "tool"}
	for i := range want {
		if roles[i] != want[i] {
			t.Fatalf("roles = %v, want %v", roles, want)
		}
	}
	if msgs[0].Get("tool_calls.0.id").String() != "c1" || msgs[2].Get("tool_calls.0.id").String() != "c2" {
		t.Errorf("pair grouping wrong: %s", out)
	}
}

// ---------------------------------------------------------------------------
// OPE-9733：reasoning 双向回传（请求向回填 + 响应向升维）
// ---------------------------------------------------------------------------

func TestConvertResponsesRequestToChat_ReasoningRoundTrip(t *testing.T) {
	// 真实 codex 回放形态：reasoning item 紧跟在所属 assistant 轮次之前，
	// 且可能插在一轮并行调用的两个 function_call 之间。
	body := `{
		"model": "deepseek-v4-flash",
		"input": [
			{"type":"message","role":"user","content":[{"type":"input_text","text":"list files"}]},
			{"type":"reasoning","summary":[{"type":"summary_text","text":"先列目录再读文件"}]},
			{"type":"function_call","name":"shell","arguments":"{\"cmd\":\"ls\"}","call_id":"call_1"},
			{"type":"function_call_output","call_id":"call_1","output":"file_a"},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"再看看内容"}]},
			{"type":"function_call","name":"shell","arguments":"{\"cmd\":\"cat a\"}","call_id":"call_2"},
			{"type":"reasoning","summary":[{"type":"summary_text","text":"并行读两个文件"}]},
			{"type":"function_call","name":"shell","arguments":"{\"cmd\":\"cat b\"}","call_id":"call_3"},
			{"type":"function_call_output","call_id":"call_2","output":"A"},
			{"type":"function_call_output","call_id":"call_3","output":"B"},
			{"type":"reasoning","content":[{"type":"reasoning_text","text":"总结一下"}]},
			{"type":"message","role":"assistant","content":[{"type":"output_text","text":"done"}]}
		],
		"store": false
	}`
	out, err := ConvertResponsesRequestToChat([]byte(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	msgs := gjson.ParseBytes(out).Get("messages").Array()
	// user / assistant(tc1+reasoning) / tool / user / assistant(tc2+tc3+reasoning) / tool / tool / assistant(text+reasoning)
	if len(msgs) != 8 {
		t.Fatalf("messages len = %d, want 8: %s", len(msgs), out)
	}
	if msgs[1].Get("role").String() != "assistant" ||
		msgs[1].Get("reasoning_content").String() != "先列目录再读文件" {
		t.Errorf("msg1 (tool_calls turn) reasoning wrong: %v", msgs[1])
	}
	if msgs[1].Get("tool_calls.0.id").String() != "call_1" {
		t.Errorf("msg1 tool_calls wrong: %v", msgs[1].Get("tool_calls"))
	}
	// 被 reasoning 隔断的并行调用必须仍合并为一条 assistant 多 tool_calls
	if n := len(msgs[4].Get("tool_calls").Array()); n != 2 {
		t.Fatalf("msg4 tool_calls len = %d, want 2 (parallel merge must survive reasoning)", n)
	}
	if msgs[4].Get("reasoning_content").String() != "并行读两个文件" {
		t.Errorf("msg4 reasoning wrong: %v", msgs[4].Get("reasoning_content"))
	}
	// content[] 形态（reasoning_text part）也要能取到
	if msgs[7].Get("reasoning_content").String() != "总结一下" {
		t.Errorf("msg7 (assistant text) reasoning wrong: %v", msgs[7].Get("reasoning_content"))
	}
	if msgs[7].Get("role").String() != "assistant" {
		t.Errorf("msg7 role = %v", msgs[7].Get("role"))
	}
	// tool / user 消息不得被污染
	for _, i := range []int{2, 3, 5, 6} {
		if msgs[i].Get("reasoning_content").Exists() {
			t.Errorf("msg%d must not carry reasoning_content", i)
		}
	}
}

func TestConvertResponsesRequestToChat_ReasoningOrphanDropped(t *testing.T) {
	// reasoning 后面跟的是 user 轮（无 assistant 可归属）→ 静默丢弃。
	body := `{"model":"m","input":[
		{"type":"message","role":"user","content":"q1"},
		{"type":"reasoning","summary":[{"type":"summary_text","text":"孤儿思考"}]},
		{"type":"message","role":"user","content":"q2"}
	]}`
	out, err := ConvertResponsesRequestToChat([]byte(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	msgs := gjson.ParseBytes(out).Get("messages").Array()
	if len(msgs) != 2 {
		t.Fatalf("messages len = %d, want 2", len(msgs))
	}
	for i, m := range msgs {
		if m.Get("reasoning_content").Exists() {
			t.Errorf("msg%d must not carry reasoning_content: %v", i, m)
		}
	}
}

func TestConvertResponsesRequestToChat_ReasoningEmptySummaryNoField(t *testing.T) {
	// 既有形态回归：空 summary 的 reasoning item 不应给 assistant 消息凭空
	// 加 reasoning_content 键（保持消息形状与历史行为一致）。
	body := `{"model":"m","input":[
		{"type":"message","role":"user","content":"q"},
		{"type":"reasoning","summary":[]},
		{"type":"function_call","name":"shell","arguments":"{}","call_id":"c1"},
		{"type":"function_call_output","call_id":"c1","output":"ok"}
	]}`
	out, err := ConvertResponsesRequestToChat([]byte(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	msgs := gjson.ParseBytes(out).Get("messages").Array()
	if len(msgs) != 3 {
		t.Fatalf("messages len = %d, want 3", len(msgs))
	}
	if msgs[1].Get("reasoning_content").Exists() {
		t.Errorf("empty summary must not produce reasoning_content: %v", msgs[1])
	}
}

func TestConvertChatResponseToResponses_ReasoningItem(t *testing.T) {
	body := `{
		"id": "chatcmpl-r",
		"created": 1,
		"model": "deepseek-v4-flash",
		"choices": [{"index":0,"message":{
			"role":"assistant",
			"reasoning_content":"需要先列目录",
			"content":"已完成",
			"tool_calls":[{"id":"call_1","type":"function","function":{"name":"shell","arguments":"{}"}}]
		},"finish_reason":"stop"}],
		"usage": {"prompt_tokens": 3, "completion_tokens": 5, "total_tokens": 8}
	}`
	out, err := ConvertChatResponseToResponses([]byte(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	g := gjson.ParseBytes(out)
	items := g.Get("output").Array()
	if len(items) != 3 {
		t.Fatalf("output len = %d, want 3 (reasoning + message + function_call): %s", len(items), out)
	}
	r := items[0]
	if r.Get("type").String() != "reasoning" {
		t.Fatalf("output.0 type = %v, want reasoning", r.Get("type"))
	}
	if r.Get("summary.0.type").String() != "summary_text" || r.Get("summary.0.text").String() != "需要先列目录" {
		t.Errorf("reasoning summary wrong: %v", r.Get("summary"))
	}
	if items[1].Get("type").String() != "message" || items[1].Get("content.0.text").String() != "已完成" {
		t.Errorf("output.1 message wrong: %v", items[1])
	}
	if items[2].Get("type").String() != "function_call" {
		t.Errorf("output.2 type = %v", items[2].Get("type"))
	}
}

func TestStreamConversion_ReasoningEvents(t *testing.T) {
	chunks := []string{
		"data: {\"id\":\"chatcmpl-1\",\"created\":1789441920,\"model\":\"deepseek-v4-flash\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"reasoning_content\":\"先\"},\"finish_reason\":null}]}\n\n",
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"reasoning_content\":\"思考\"},\"finish_reason\":null}]}\n\n",
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"答\"},\"finish_reason\":null}]}\n\n",
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n",
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[],\"usage\":{\"prompt_tokens\":6,\"completion_tokens\":3,\"total_tokens\":9}}\n\n",
		"data: [DONE]\n\n",
	}
	events := feedChunks(t, chunks)

	// 思考增量必须以 summary_text delta 形式透传给 codex
	var reasoningDeltas []string
	for _, e := range events {
		if e.Get("type").String() == "response.reasoning_summary_text.delta" {
			reasoningDeltas = append(reasoningDeltas, e.Get("delta").String())
		}
	}
	if strings.Join(reasoningDeltas, "") != "先思考" {
		t.Fatalf("reasoning deltas = %v, want 先思考", reasoningDeltas)
	}

	// 事件序：reasoning item 先于 message item 打开
	openItems := map[string]string{} // item type -> item id
	for _, e := range events {
		if e.Get("type").String() == "response.output_item.added" {
			openItems[e.Get("item.type").String()] = e.Get("item.id").String()
		}
	}
	if _, ok := openItems["reasoning"]; !ok {
		t.Fatalf("missing reasoning output_item.added: %v", eventTypes(events))
	}

	// 收尾：summary done + output_item.done 全量文本
	var doneText string
	var doneItem gjson.Result
	for _, e := range events {
		if e.Get("type").String() == "response.reasoning_summary_text.done" {
			doneText = e.Get("text").String()
		}
		if e.Get("type").String() == "response.output_item.done" && e.Get("item.type").String() == "reasoning" {
			doneItem = e.Get("item")
		}
	}
	if doneText != "先思考" {
		t.Errorf("reasoning summary done text = %q", doneText)
	}
	if doneItem.Get("id").String() == "" || doneItem.Get("summary.0.text").String() != "先思考" {
		t.Errorf("reasoning output_item.done wrong: %v", doneItem)
	}

	// completed 的 output 首位是 reasoning item，随后才是 message
	completed := events[len(events)-1]
	out0 := completed.Get("response.output.0")
	if out0.Get("type").String() != "reasoning" || out0.Get("summary.0.text").String() != "先思考" {
		t.Errorf("completed output.0 wrong: %v", out0)
	}
	if completed.Get("response.output.1.type").String() != "message" {
		t.Errorf("completed output.1 should be message: %v", completed.Get("response.output"))
	}
	// added 与 done 的 item id 一致（codex 按 id 归档）
	if openItems["reasoning"] != doneItem.Get("id").String() {
		t.Errorf("reasoning id mismatch: added=%s done=%s", openItems["reasoning"], doneItem.Get("id"))
	}
}

func TestConvertResponsesRequestToChat_ReasoningPlaceholderWhenTools(t *testing.T) {
	// OPE-9733 线级实证：带 tools 时上游对历史 assistant 轮次无条件要求
	// reasoning_content（即使模型上一轮自适应跳过思考）。桥必须兜底占位。
	body := `{"model":"deepseek-flash","input":[
		{"type":"message","role":"user","content":"q"},
		{"type":"function_call","name":"exec_command","arguments":"{}","call_id":"c1"},
		{"type":"function_call_output","call_id":"c1","output":"ok"},
		{"type":"message","role":"assistant","content":[{"type":"output_text","text":"done"}]}
	],"tools":[{"type":"function","name":"exec_command","parameters":{"type":"object"}}]}`
	out, err := ConvertResponsesRequestToChat([]byte(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	msgs := gjson.ParseBytes(out).Get("messages").Array()
	// user / assistant(tool_calls) / tool / assistant(text)
	if len(msgs) != 4 {
		t.Fatalf("messages len = %d, want 4", len(msgs))
	}
	if msgs[1].Get("reasoning_content").String() != "(no reasoning content recorded)" {
		t.Errorf("tool_calls assistant must get placeholder, got %q", msgs[1].Get("reasoning_content").String())
	}
	if msgs[3].Get("reasoning_content").String() != "(no reasoning content recorded)" {
		t.Errorf("text assistant must get placeholder, got %q", msgs[3].Get("reasoning_content").String())
	}
	for _, i := range []int{0, 2} {
		if msgs[i].Get("reasoning_content").Exists() {
			t.Errorf("msg%d (role=%s) must not carry reasoning_content", i, msgs[i].Get("role").String())
		}
	}

	// 无 tools 的请求不注入占位
	body2 := `{"model":"m","input":[
		{"type":"message","role":"user","content":"q"},
		{"type":"function_call","name":"exec_command","arguments":"{}","call_id":"c1"},
		{"type":"function_call_output","call_id":"c1","output":"ok"}
	]}`
	out2, err := ConvertResponsesRequestToChat([]byte(body2))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	msgs2 := gjson.ParseBytes(out2).Get("messages").Array()
	if msgs2[1].Get("reasoning_content").Exists() {
		t.Errorf("no-tools request must not inject placeholder")
	}

	// 已有真实思考文本时不被占位覆盖
	body3 := `{"model":"m","input":[
		{"type":"message","role":"user","content":"q"},
		{"type":"reasoning","summary":[{"type":"summary_text","text":"真实思考"}]},
		{"type":"function_call","name":"exec_command","arguments":"{}","call_id":"c1"},
		{"type":"function_call_output","call_id":"c1","output":"ok"}
	],"tools":[{"type":"function","name":"exec_command","parameters":{"type":"object"}}]}`
	out3, err := ConvertResponsesRequestToChat([]byte(body3))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	msgs3 := gjson.ParseBytes(out3).Get("messages").Array()
	if msgs3[1].Get("reasoning_content").String() != "真实思考" {
		t.Errorf("real reasoning must be preserved, got %q", msgs3[1].Get("reasoning_content").String())
	}
}
