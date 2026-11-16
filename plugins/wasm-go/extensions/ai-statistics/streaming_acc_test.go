/*
 * 流式属性逐事件累积(OOM 根治改造)的单元测试。
 * 对应《ai-statistics流式缓冲限长改造方案.md》§7.1 新增用例。
 */

package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/stretchr/testify/require"
)

// fakeHttpContext 仅实现累积逻辑用到的 SetContext/GetContext,
// 其余方法经嵌入的 nil 接口兜底(调用即 panic,测试中不应触达)。
type fakeHttpContext struct {
	wrapper.HttpContext
	m map[string]interface{}
}

func newFakeHttpContext() *fakeHttpContext {
	return &fakeHttpContext{m: map[string]interface{}{}}
}

func (f *fakeHttpContext) SetContext(key string, value interface{}) { f.m[key] = value }
func (f *fakeHttpContext) GetContext(key string) interface{}        { return f.m[key] }

// testAccConfig 构造只含流式属性的配置
func testAccConfig(limit int, attrs ...Attribute) AIStatisticsConfig {
	return AIStatisticsConfig{
		attributes:       attrs,
		valueLengthLimit: limit,
	}
}

// feedChunks 逐 chunk 驱动 accumulateStreamingAttributes,最后一个 chunk 带 endOfStream=true
func feedChunks(ctx wrapper.HttpContext, config AIStatisticsConfig, chunks ...[]byte) {
	for i, c := range chunks {
		accumulateStreamingAttributes(ctx, config, c, i == len(chunks)-1)
	}
}

// TestStreamingAccEquivalenceAnchor 等价性锚点:多 chunk SSE 流,逐事件累积的 answer
// 与改造前 extractStreamingBodyByJsonPath 对整流 buffer 的解析结果逐字节一致
func TestStreamingAccEquivalenceAnchor(t *testing.T) {
	chunks := [][]byte{
		[]byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n"),
		[]byte("data: {\"choices\":[{\"delta\":{\"content\":\" there\"}}]}\n\n"),
		[]byte("data: {\"choices\":[{\"delta\":{\"content\":\"!\"}}]}\n\ndata: [DONE]\n\n"),
	}
	full := []byte(string(chunks[0]) + string(chunks[1]) + string(chunks[2]))

	attr := Attribute{Key: BuiltinAnswerKey, Rule: RuleAppend}
	config := testAccConfig(4096, attr)

	ctx := newFakeHttpContext()
	feedChunks(ctx, config, chunks...)

	old := extractStreamingBodyByJsonPath(full, AnswerPathOpenAIStreaming, RuleAppend)
	require.Equal(t, old, getStreamingAccValue(ctx, BuiltinAnswerKey))
	require.Equal(t, "Hello there!", getStreamingAccValue(ctx, BuiltinAnswerKey))
}

// TestStreamingAccCrossBoundary 事件跨 chunk 任意位置拆分(含 \r\n 被劈开)
func TestStreamingAccCrossBoundary(t *testing.T) {
	stream := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"AB\"}}]}\r\n\r\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"CD\"}}]}\n\n")
	attr := Attribute{Key: BuiltinAnswerKey, Rule: RuleAppend}
	config := testAccConfig(4096, attr)

	// 每个字节一个 chunk,最坏拆分
	ctx := newFakeHttpContext()
	for i := 0; i < len(stream); i++ {
		accumulateStreamingAttributes(ctx, config, stream[i:i+1], i == len(stream)-1)
	}
	require.Equal(t, "ABCD", getStreamingAccValue(ctx, BuiltinAnswerKey))
}

// TestStreamingAccAppendLimit answer 超 value_length_limit:累积停在限长,后续事件跳过
func TestStreamingAccAppendLimit(t *testing.T) {
	attr := Attribute{Key: BuiltinAnswerKey, Rule: RuleAppend}
	config := testAccConfig(8, attr)

	ctx := newFakeHttpContext()
	feedChunks(ctx, config,
		[]byte("data: {\"choices\":[{\"delta\":{\"content\":\"12345\"}}]}\n\n"),
		[]byte("data: {\"choices\":[{\"delta\":{\"content\":\"67890\"}}]}\n\n"), // 触发到限
		[]byte("data: {\"choices\":[{\"delta\":{\"content\":\"abcde\"}}]}\n\n"), // 应被跳过
	)
	require.Equal(t, "12345678", getStreamingAccValue(ctx, BuiltinAnswerKey))

	// 累积器应已 finished
	acc, ok := ctx.GetContext(ctxKeyStreamingAttrAccPrefix + BuiltinAnswerKey).(*streamingAttrAcc)
	require.True(t, ok)
	require.True(t, acc.finished)
}

// TestStreamingAccFirstAndReplace first / replace 语义
func TestStreamingAccFirstAndReplace(t *testing.T) {
	mk := func(rule string) (wrapper.HttpContext, AIStatisticsConfig) {
		attr := Attribute{Key: BuiltinAnswerKey, Rule: rule}
		return newFakeHttpContext(), testAccConfig(4096, attr)
	}
	chunks := [][]byte{
		[]byte("data: {\"choices\":[{\"delta\":{\"content\":\"first\"}}]}\n\n"),
		[]byte("data: {\"choices\":[{\"delta\":{\"content\":\"second\"}}]}\n\n"),
		[]byte("data: {\"choices\":[{\"delta\":{\"content\":\"third\"}}]}\n\n"),
	}

	ctxFirst, cfgFirst := mk(RuleFirst)
	feedChunks(ctxFirst, cfgFirst, chunks...)
	require.Equal(t, "first", getStreamingAccValue(ctxFirst, BuiltinAnswerKey))

	ctxReplace, cfgReplace := mk(RuleReplace)
	feedChunks(ctxReplace, cfgReplace, chunks...)
	require.Equal(t, "third", getStreamingAccValue(ctxReplace, BuiltinAnswerKey))
}

// TestStreamingAccFirstEmptyFinished first 规则取到 Exists 但值为空串的事件后即停止,
// 与 extractStreamingBodyByJsonPath 的 break 语义一致(不继续等后续非空事件)
func TestStreamingAccFirstEmptyFinished(t *testing.T) {
	attr := Attribute{Key: BuiltinAnswerKey, Rule: RuleFirst}
	config := testAccConfig(4096, attr)

	ctx := newFakeHttpContext()
	feedChunks(ctx, config,
		[]byte("data: {\"choices\":[{\"delta\":{\"content\":\"\"}}]}\n\n"),      // Exists 但空串
		[]byte("data: {\"choices\":[{\"delta\":{\"content\":\"later\"}}]}\n\n"), // 应被忽略
	)
	require.Equal(t, "", getStreamingAccValue(ctx, BuiltinAnswerKey))

	acc, _ := ctx.GetContext(ctxKeyStreamingAttrAccPrefix + BuiltinAnswerKey).(*streamingAttrAcc)
	require.NotNil(t, acc)
	require.True(t, acc.finished)

	// 对照旧语义:整流解析同样返回 ""
	full := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"later\"}}]}\n\n")
	require.Equal(t, "", extractStreamingBodyByJsonPath(full, AnswerPathOpenAIStreaming, RuleFirst))
}

// TestStreamingAccReplaceLimit replace 规则写入时按限长截断
func TestStreamingAccReplaceLimit(t *testing.T) {
	attr := Attribute{Key: "custom", ValueSource: ResponseStreamingBody, Value: "choices.0.delta.content", Rule: RuleReplace}
	config := testAccConfig(4, attr)

	ctx := newFakeHttpContext()
	feedChunks(ctx, config,
		[]byte("data: {\"choices\":[{\"delta\":{\"content\":\"123456789\"}}]}\n\n"),
	)
	require.Equal(t, "1234", getStreamingAccValue(ctx, "custom"))
}

// TestStreamingAccClaudeAnswer Claude 协议 answer(delta.text)
func TestStreamingAccClaudeAnswer(t *testing.T) {
	attr := Attribute{Key: BuiltinAnswerKey, Rule: RuleAppend}
	config := testAccConfig(4096, attr)

	ctx := newFakeHttpContext()
	feedChunks(ctx, config,
		[]byte("data: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"Hel\"}}\n\n"),
		[]byte("data: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"lo\"}}\n\n"),
	)
	require.Equal(t, "Hello", getStreamingAccValue(ctx, BuiltinAnswerKey))
}

// TestStreamingAccReasoning 内置 reasoning 固定 append
func TestStreamingAccReasoning(t *testing.T) {
	attr := Attribute{Key: BuiltinReasoningKey} // 不配置 rule,内置 reasoning 固定 append
	config := testAccConfig(4096, attr)

	ctx := newFakeHttpContext()
	feedChunks(ctx, config,
		[]byte("data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"think\"}}]}\n\n"),
		[]byte("data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"ing\"}}]}\n\n"),
	)
	require.Equal(t, "thinking", getStreamingAccValue(ctx, BuiltinReasoningKey))
}

// TestStreamingAccToolCallsOpenAI tool_calls 逐事件喂入与整buffer一次喂入结果一致
func TestStreamingAccToolCallsOpenAI(t *testing.T) {
	chunks := [][]byte{
		[]byte("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"get_weather\",\"arguments\":\"\"}}]}}]}\n\n"),
		[]byte("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"{\\\"locat\"}}]}}]}\n\n"),
		[]byte("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"ion\\\": \\\"Beijing\\\"}\"}}]}}]}\n\n"),
	}
	attr := Attribute{Key: BuiltinToolCallsKey}
	config := testAccConfig(4096, attr)

	ctx := newFakeHttpContext()
	feedChunks(ctx, config, chunks...)

	got, _ := ctx.GetContext(CtxStreamingToolCallsBuffer).(*StreamingToolCallsBuffer)

	// 旧行为:整 buffer 一次喂入
	full := []byte(strings.Join([]string{string(chunks[0]), string(chunks[1]), string(chunks[2])}, ""))
	want := extractStreamingToolCalls(full, nil, 4096)

	require.Equal(t, getToolCallsFromBuffer(want), getToolCallsFromBuffer(got))
	require.Len(t, got.ToolCalls, 1)
	require.Equal(t, `{"location": "Beijing"}`, got.ToolCalls[0].Function.Arguments)
}

// TestStreamingAccToolCallsClaude Claude tool_calls 逐事件喂入
func TestStreamingAccToolCallsClaude(t *testing.T) {
	chunks := [][]byte{
		[]byte("data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_1\",\"name\":\"get_weather\"}}\n\n"),
		[]byte("data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"partial_json\":\"{\\\"city\\\"\"}}\n\n"),
		[]byte("data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"partial_json\":\": \\\"BJ\\\"}\"}}\n\n"),
		[]byte("data: {\"type\":\"content_block_stop\",\"index\":0}\n\n"),
	}
	attr := Attribute{Key: BuiltinToolCallsKey}
	config := testAccConfig(4096, attr)

	ctx := newFakeHttpContext()
	feedChunks(ctx, config, chunks...)

	got, _ := ctx.GetContext(CtxStreamingToolCallsBuffer).(*StreamingToolCallsBuffer)
	toolCalls := getToolCallsFromBuffer(got)
	require.Len(t, toolCalls, 1)
	require.Equal(t, "toolu_1", toolCalls[0].ID)
	require.Equal(t, `{"city": "BJ"}`, toolCalls[0].Function.Arguments)
}

// TestStreamingAccToolCallsArgsLimit tool_calls arguments 到限即停(OpenAI 与 Claude 两条累积路径)
func TestStreamingAccToolCallsArgsLimit(t *testing.T) {
	openAIChunk := []byte("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"c1\",\"function\":{\"name\":\"f\",\"arguments\":\"1234567890\"}}]}}]}\n\n")
	attr := Attribute{Key: BuiltinToolCallsKey}
	config := testAccConfig(4, attr)

	ctx := newFakeHttpContext()
	feedChunks(ctx, config, openAIChunk)
	got, _ := ctx.GetContext(CtxStreamingToolCallsBuffer).(*StreamingToolCallsBuffer)
	require.Equal(t, "1234", got.ToolCalls[0].Function.Arguments)

	// Claude: 限长必须落在 ArgumentsBuffer(累积点),不是 stop 时
	claudeChunks := [][]byte{
		[]byte("data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"t1\",\"name\":\"f\"}}\n\n"),
		[]byte("data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"partial_json\":\"1234567890\"}}\n\n"),
		[]byte("data: {\"type\":\"content_block_stop\",\"index\":0}\n\n"),
	}
	ctx2 := newFakeHttpContext()
	feedChunks(ctx2, config, claudeChunks...)
	got2, _ := ctx2.GetContext(CtxStreamingToolCallsBuffer).(*StreamingToolCallsBuffer)
	require.Equal(t, "1234", got2.ArgumentsBuffer[0])
	require.Equal(t, "1234", got2.ToolCalls[0].Function.Arguments)
}

// TestStreamingAccToolCallsMapCap ToolCalls map 条目数上限 128(异常 index 流防御)
func TestStreamingAccToolCallsMapCap(t *testing.T) {
	attr := Attribute{Key: BuiltinToolCallsKey}
	config := testAccConfig(4096, attr)

	ctx := newFakeHttpContext()
	for i := 0; i < maxStreamingToolCalls+10; i++ {
		chunk := []byte(fmt.Sprintf("data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":%d,\"id\":\"c%d\",\"function\":{\"name\":\"f\"}}]}}]}\n\n", i, i))
		accumulateStreamingAttributes(ctx, config, chunk, i == maxStreamingToolCalls+9)
	}
	got, _ := ctx.GetContext(CtxStreamingToolCallsBuffer).(*StreamingToolCallsBuffer)
	require.Len(t, got.ToolCalls, maxStreamingToolCalls)
}

// TestStreamingAccFlushTail 流末尾无 \n\n 终止符的事件经 flush 尽力解析(与改造前一致)
func TestStreamingAccFlushTail(t *testing.T) {
	attr := Attribute{Key: BuiltinAnswerKey, Rule: RuleAppend}
	config := testAccConfig(4096, attr)

	ctx := newFakeHttpContext()
	feedChunks(ctx, config,
		[]byte("data: {\"choices\":[{\"delta\":{\"content\":\"closed\"}}]}\n\n"),
		[]byte("data: {\"choices\":[{\"delta\":{\"content\":\"tail\"}}]}"), // 无终止符
	)
	require.Equal(t, "closedtail", getStreamingAccValue(ctx, BuiltinAnswerKey))
}

// TestStreamingAccFlushTailCRLF 含 \r\n 的尾事件经 UnifySSEChunk 归一化后提取一致
func TestStreamingAccFlushTailCRLF(t *testing.T) {
	attr := Attribute{Key: BuiltinAnswerKey, Rule: RuleAppend}
	config := testAccConfig(4096, attr)

	ctx := newFakeHttpContext()
	accumulateStreamingAttributes(ctx, config,
		[]byte("data: {\"choices\":[{\"delta\":{\"content\":\"crlf\"}}]}\r\n"), true)
	require.Equal(t, "crlf", getStreamingAccValue(ctx, BuiltinAnswerKey))
}

// TestStreamingAccFlushInResync RESYNC 状态下 flush 返回空(不解析超大事件残片)
func TestStreamingAccFlushInResync(t *testing.T) {
	framer := &sseFramer{}
	// 构造超过 1MiB 的未闭合事件 → RESYNC
	big := make([]byte, maxIncompleteSSEEventBytes+10)
	for i := range big {
		big[i] = 'a'
	}
	events := framer.frameCallback(big)
	require.Empty(t, events)
	require.True(t, framer.resyncing)
	require.Nil(t, framer.flush())
}

// TestStreamingAccEmptyAndDone 空流与 [DONE] 哨兵
func TestStreamingAccEmptyAndDone(t *testing.T) {
	attr := Attribute{Key: BuiltinAnswerKey, Rule: RuleAppend}
	config := testAccConfig(4096, attr)

	// 空流:仅 EOS
	ctx := newFakeHttpContext()
	accumulateStreamingAttributes(ctx, config, nil, true)
	require.Nil(t, getStreamingAccValue(ctx, BuiltinAnswerKey))

	// 仅 [DONE]
	ctx2 := newFakeHttpContext()
	feedChunks(ctx2, config, []byte("data: [DONE]\n\n"))
	require.Equal(t, "", getStreamingAccValue(ctx2, BuiltinAnswerKey))
}

// TestStreamingAccRuleEmpty rule 为空:值恒空、流程不崩(与旧行为一致,日志每请求只记一次)。
// log.Errorf 依赖宿主 foreign function,必须走 proxytest 宿主。
func TestStreamingAccRuleEmpty(t *testing.T) {
	configJSON, _ := json.Marshal(map[string]interface{}{
		"attributes": []map[string]interface{}{
			{
				"key":          "answer",
				"value_source": "response_streaming_body",
				"value":        "choices.0.delta.content",
				// 故意不配置 rule
				"apply_to_log": true,
			},
		},
	})
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(configJSON)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/v1/chat/completions"},
			{":method", "POST"},
		})
		host.CallOnHttpRequestBody([]byte(`{"model":"gpt-4","stream":true,"messages":[{"role":"user","content":"Hi"}]}`))
		host.CallOnHttpResponseHeaders([][2]string{
			{":status", "200"},
			{"content-type", "text/event-stream"},
		})
		host.CallOnHttpStreamingResponseBody([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"a\"}}]}\n\n"), false)
		host.CallOnHttpStreamingResponseBody([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"b\"}}]}\n\n"), true)
		host.CompleteHttp()
	})
}
