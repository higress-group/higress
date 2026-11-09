package provider

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/proxytest"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeProvider_InterleavedStreams(t *testing.T) {
	initClaudeStreamTestHost(t)
	for _, providerType := range []string{"claude", "vertex"} {
		t.Run(providerType, func(t *testing.T) {
			for _, first := range []string{"a", "b"} {
				t.Run(first+"_finishes_first", func(t *testing.T) {
					p, newContext := newClaudeStreamTestProvider(t, providerType)
					contexts := map[string]wrapper.HttpContext{"a": newContext(), "b": newContext()}
					responses := map[string][]chatCompletionResponse{}
					for _, role := range []string{"a", "b"} {
						responses[role] = convertClaudeTestStream(t, p, contexts[role], claudeStreamTestFrames(role)[0], false)
					}
					order := []string{"a", "b"}
					if first == "b" {
						order = []string{"b", "a"}
					}
					for _, role := range order {
						frames := claudeStreamTestFrames(role)
						for i, frame := range frames[1:] {
							last := i == len(frames)-2
							if last {
								frame = strings.TrimRight(frame, "\n")
							}
							responses[role] = append(responses[role], convertClaudeTestStream(t, p, contexts[role], frame, last)...)
						}
					}
					for _, role := range []string{"a", "b"} {
						assertClaudeTestStream(t, role, responses[role])
					}
				})
			}
		})
	}
}

func TestClaudeProvider_InterleavedFragmentedStreams(t *testing.T) {
	initClaudeStreamTestHost(t)
	for _, providerType := range []string{"claude", "vertex"} {
		t.Run(providerType, func(t *testing.T) {
			for _, role := range []string{"a", "b"} {
				t.Run("split_"+role, func(t *testing.T) {
					other := "b"
					if role == "b" {
						other = "a"
					}
					stream := strings.TrimRight(strings.Join(claudeStreamTestFrames(role), ""), "\n")
					otherStream := strings.TrimRight(strings.Join(claudeStreamTestFrames(other), ""), "\n")
					// Complete the other stream between every possible byte boundary.
					for split := 0; split <= len(stream); split++ {
						p, newContext := newClaudeStreamTestProvider(t, providerType)
						a, b := newContext(), newContext()
						responses := convertClaudeTestStream(t, p, a, stream[:split], false)
						otherResponses := convertClaudeTestStream(t, p, b, otherStream, true)
						responses = append(responses, convertClaudeTestStream(t, p, a, stream[split:], true)...)
						assertClaudeTestStream(t, role, responses)
						assertClaudeTestStream(t, other, otherResponses)
					}
				})
			}
		})
	}
}

func TestClaudeProvider_StreamStateInitialization(t *testing.T) {
	initClaudeStreamTestHost(t)
	for _, providerType := range []string{"claude", "vertex"} {
		t.Run(providerType, func(t *testing.T) {
			p, newContext := newClaudeStreamTestProvider(t, providerType)
			a, b := newContext(), newContext()
			start := `data: {"type":"message_start","message":{"id":"msg_a","usage":{"input_tokens":10,"output_tokens":1,"service_tier":"standard"}}}` + "\n\n"
			convertClaudeTestStream(t, p, a, start, false)

			// A stream without message_start must not inherit another request's state.
			end := convertClaudeTestStream(t, p, b, `data: {"type":"message_stop"}`+"\n\n", true)
			require.Len(t, end, 1)
			assert.Empty(t, end[0].Id)
			assert.Empty(t, end[0].ServiceTier)
			assert.Equal(t, &usage{}, end[0].Usage)

			// A new message on the same context resets accumulated usage.
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

			// Reuse the provider for single and consecutive requests.
			for _, role := range []string{"a", "b", "a"} {
				responses := convertClaudeTestStream(t, p, newContext(), strings.Join(claudeStreamTestFrames(role), ""), true)
				assertClaudeTestStream(t, role, responses)
			}
		})
	}
}

func initClaudeStreamTestHost(t *testing.T) {
	t.Helper()
	host, reset := proxytest.NewHostEmulator(proxytest.NewEmulatorOption().WithHttpContext(func(uint32) types.HttpContext {
		return &types.DefaultHttpContext{}
	}))
	t.Cleanup(reset)
	id := host.InitializeHttpContext()
	host.CallOnRequestHeaders(id, nil, false)
}

func newClaudeStreamTestProvider(t *testing.T, providerType string) (StreamingResponseBodyHandler, func() wrapper.HttpContext) {
	t.Helper()
	config := ProviderConfig{
		apiTokens: []string{"fixture-placeholder"},
		failover:  &failover{ctxApiTokenInUse: "claude-test-token"},
	}
	var p StreamingResponseBodyHandler = &claudeProvider{config: config}
	if providerType == "vertex" {
		provider, err := (&vertexProviderInitializer{}).CreateProvider(config)
		require.NoError(t, err)
		p = provider.(StreamingResponseBodyHandler)
	}
	return p, func() wrapper.HttpContext {
		ctx := newMapCtx()
		ctx.SetContext("needClaudeResponseConversion", true)
		request := []byte(`{"model":"claude-fixture","messages":[{"role":"user","content":"hello"}],"stream":true}`)
		if v, ok := p.(*vertexProvider); ok {
			headers := http.Header{}
			body, err := v.TransformRequestBodyHeaders(ctx, ApiNameChatCompletion, request, headers)
			require.NoError(t, err)
			assert.Equal(t, true, ctx.GetContext(contextClaudeMarker))
			assert.Contains(t, headers.Get(":path"), "/publishers/anthropic/models/claude-fixture:streamRawPredict")
			assert.Contains(t, string(body), vertexAnthropicVersion)
		} else {
			_, err := p.(*claudeProvider).TransformRequestBody(ctx, ApiNameChatCompletion, request)
			require.NoError(t, err)
		}
		return ctx
	}
}

func claudeStreamTestFrames(role string) []string {
	if role == "a" {
		return []string{
			`data: {"type":"message_start","message":{"id":"msg_a","usage":{"input_tokens":955,"output_tokens":0,"service_tier":"standard"}}}` + "\n\n",
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"thinking a"}}` + "\n\n",
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"signature a"}}` + "\n\n",
			`data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"answer a"}}` + "\n\n",
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":256}}` + "\n\n",
			`data: {"type":"message_stop"}` + "\n\n",
		}
	}
	return []string{
		`data: {"type":"message_start","message":{"id":"msg_b","usage":{"input_tokens":45,"output_tokens":0,"service_tier":"priority"}}}` + "\n\n",
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"call_b","name":"weather","input":{}}}` + "\n\n",
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"city\":"}}` + "\n\n",
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\"Beijing\"}"}}` + "\n\n",
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":510}}` + "\n\n",
		`data: {"type":"message_stop"}` + "\n\n",
	}
}

func assertClaudeTestStream(t *testing.T, role string, responses []chatCompletionResponse) {
	t.Helper()
	require.Len(t, responses, 6)
	tier := "standard"
	expected := &usage{PromptTokens: 955, CompletionTokens: 256, TotalTokens: 1211}
	if role == "b" {
		tier = "priority"
		expected = &usage{PromptTokens: 45, CompletionTokens: 510, TotalTokens: 555}
	}
	for i, response := range responses {
		require.Equal(t, "msg_"+role, response.Id, "chunk %d", i)
		require.Equal(t, tier, response.ServiceTier, "chunk %d", i)
		if i < len(responses)-1 {
			require.Nil(t, response.Usage, "chunk %d", i)
		}
	}
	assert.Equal(t, expected, responses[5].Usage)
	if role == "a" {
		assert.Equal(t, "thinking a", responses[1].Choices[0].Delta.Reasoning)
		assert.Equal(t, "signature a", responses[2].Choices[0].Delta.ReasoningSignature)
		assert.Equal(t, "answer a", responses[3].Choices[0].Delta.Content)
		assert.Equal(t, finishReasonStop, *responses[4].Choices[0].FinishReason)
	} else {
		call := responses[1].Choices[0].Delta.ToolCalls[0]
		assert.Equal(t, "call_b", call.Id)
		assert.Equal(t, "weather", call.Function.Name)
		arguments := responses[2].Choices[0].Delta.ToolCalls[0].Function.Arguments + responses[3].Choices[0].Delta.ToolCalls[0].Function.Arguments
		assert.Equal(t, `{"city":"Beijing"}`, arguments)
		assert.Equal(t, finishReasonToolCall, *responses[4].Choices[0].FinishReason)
	}
}

func convertClaudeTestStream(t *testing.T, p StreamingResponseBodyHandler, ctx wrapper.HttpContext, chunk string, last bool) []chatCompletionResponse {
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
