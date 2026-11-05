package provider

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeProvider_InterleavedStreams(t *testing.T) {
	p := &claudeProvider{}
	a, b := newMapCtx(), newMapCtx()

	convertClaudeTestStream(t, p, a, `data: {"type":"message_start","message":{"id":"msg_a","usage":{"input_tokens":10,"output_tokens":0,"service_tier":"standard"}}}`+"\n\n", false)
	convertClaudeTestStream(t, p, b, `data: {"type":"message_start","message":{"id":"msg_b","usage":{"input_tokens":200,"output_tokens":0,"service_tier":"priority"}}}`+"\n\n", false)

	text := convertClaudeTestStream(t, p, a, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"answer a"}}`+"\n\n", false)
	require.Len(t, text, 1)
	assert.Equal(t, "msg_a", text[0].Id)
	assert.Equal(t, "standard", text[0].ServiceTier)
	assert.Equal(t, "answer a", text[0].Choices[0].Delta.Content)

	convertClaudeTestStream(t, p, a, `data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":3}}`+"\n\n", false)
	endA := convertClaudeTestStream(t, p, a, `data: {"type":"message_stop"}`+"\n\n", true)
	require.Len(t, endA, 1)
	assert.Equal(t, "msg_a", endA[0].Id)
	assert.Equal(t, "standard", endA[0].ServiceTier)
	require.NotNil(t, endA[0].Usage)
	assert.Equal(t, 10, endA[0].Usage.PromptTokens)
	assert.Equal(t, 3, endA[0].Usage.CompletionTokens)
	assert.Equal(t, 13, endA[0].Usage.TotalTokens)

	convertClaudeTestStream(t, p, b, `data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}`+"\n\n", false)
	endB := convertClaudeTestStream(t, p, b, `data: {"type":"message_stop"}`+"\n\n", true)
	require.Len(t, endB, 1)
	assert.Equal(t, "msg_b", endB[0].Id)
	assert.Equal(t, "priority", endB[0].ServiceTier)
	require.NotNil(t, endB[0].Usage)
	assert.Equal(t, 200, endB[0].Usage.PromptTokens)
	assert.Equal(t, 5, endB[0].Usage.CompletionTokens)
	assert.Equal(t, 205, endB[0].Usage.TotalTokens)
}

func TestClaudeProvider_InterleavedFragmentedStreams(t *testing.T) {
	streamA := `data: {"type":"message_start","message":{"id":"msg_a","usage":{"input_tokens":10,"output_tokens":1,"service_tier":"standard"}}}` + "\n\n" +
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"answer a"}}` + "\n\n" +
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":3}}` + "\n\n" +
		`data: {"type":"message_stop"}` + "\n\n"
	streamB := `data: {"type":"message_start","message":{"id":"msg_b","usage":{"input_tokens":200,"output_tokens":0,"service_tier":"priority"}}}` + "\n\n" +
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"call_b","name":"weather","input":{}}}` + "\n\n" +
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"city\":\"Beijing\"}"}}` + "\n\n" +
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":5}}` + "\n\n" +
		`data: {"type":"message_stop"}` + "\n\n"

	// Finish B while A is split at every possible transport boundary.
	for split := 0; split <= len(streamA); split++ {
		p := &claudeProvider{}
		a, b := newMapCtx(), newMapCtx()
		responsesA := convertClaudeTestStream(t, p, a, streamA[:split], false)
		responsesB := convertClaudeTestStream(t, p, b, streamB, true)
		responsesA = append(responsesA, convertClaudeTestStream(t, p, a, streamA[split:], true)...)
		require.Len(t, responsesA, 4, "split %d", split)
		require.Len(t, responsesB, 5, "split %d", split)
		for _, response := range responsesA {
			require.Equal(t, "msg_a", response.Id, "split %d", split)
			require.Equal(t, "standard", response.ServiceTier, "split %d", split)
		}
		for _, response := range responsesB {
			require.Equal(t, "msg_b", response.Id, "split %d", split)
			require.Equal(t, "priority", response.ServiceTier, "split %d", split)
		}
		assert.Equal(t, "answer a", responsesA[1].Choices[0].Delta.Content)
		assert.Equal(t, &usage{PromptTokens: 10, CompletionTokens: 4, TotalTokens: 14}, responsesA[3].Usage)
		assert.Equal(t, "call_b", responsesB[1].Choices[0].Delta.ToolCalls[0].Id)
		assert.Equal(t, "weather", responsesB[1].Choices[0].Delta.ToolCalls[0].Function.Name)
		assert.Equal(t, `{"city":"Beijing"}`, responsesB[2].Choices[0].Delta.ToolCalls[0].Function.Arguments)
		assert.Equal(t, &usage{PromptTokens: 200, CompletionTokens: 5, TotalTokens: 205}, responsesB[4].Usage)
	}
}

func TestClaudeProvider_StreamStateInitialization(t *testing.T) {
	p := &claudeProvider{}
	a, b := newMapCtx(), newMapCtx()
	start := `data: {"type":"message_start","message":{"id":"msg_a","usage":{"input_tokens":10,"output_tokens":1,"service_tier":"standard"}}}` + "\n\n"
	convertClaudeTestStream(t, p, a, start, false)

	// A stream without message_start must not inherit another request's state.
	end := convertClaudeTestStream(t, p, b, `data: {"type":"message_stop"}`+"\n\n", true)
	require.Len(t, end, 1)
	assert.Empty(t, end[0].Id)
	assert.Empty(t, end[0].ServiceTier)
	assert.Equal(t, &usage{}, end[0].Usage)

	// Reusing a context for a new message also resets any accumulated usage.
	convertClaudeTestStream(t, p, a, `data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":3}}`+"\n\n", false)
	convertClaudeTestStream(t, p, a, start, false)
	end = convertClaudeTestStream(t, p, a, `data: {"type":"message_stop"}`+"\n\n", false)
	require.Len(t, end, 1)
	assert.Equal(t, "msg_a", end[0].Id)
	assert.Equal(t, &usage{PromptTokens: 10, CompletionTokens: 1}, end[0].Usage)

	convertClaudeTestStream(t, p, a, `data: {"type":"message_start"}`+"\n\n", false)
	end = convertClaudeTestStream(t, p, a, `data: {"type":"message_stop"}`+"\n\n", true)
	require.Len(t, end, 1)
	assert.Empty(t, end[0].Id)
	assert.Empty(t, end[0].ServiceTier)
	assert.Equal(t, &usage{}, end[0].Usage)
}

func convertClaudeTestStream(t *testing.T, p *claudeProvider, ctx wrapper.HttpContext, chunk string, last bool) []chatCompletionResponse {
	t.Helper()
	out, err := p.OnStreamingResponseBody(ctx, ApiNameChatCompletion, []byte(chunk), last)
	require.NoError(t, err)
	var responses []chatCompletionResponse
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var response chatCompletionResponse
		require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &response))
		responses = append(responses, response)
	}
	return responses
}
