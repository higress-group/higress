// Copyright (c) 2022 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCohereProvider_TransformResponseBody(t *testing.T) {
	provider := &cohereProvider{}
	ctx := newMockMultipartHttpContext()

	cohereBody := `{"response_id":"resp-1","text":"你好","generation_id":"g-1","finish_reason":"COMPLETE","meta":{"tokens":{"input_tokens":9,"output_tokens":1}}}`
	out, err := provider.TransformResponseBody(ctx, ApiNameChatCompletion, []byte(cohereBody))
	require.NoError(t, err)
	s := string(out)
	assert.Contains(t, s, `"id":"resp-1"`)
	assert.Contains(t, s, `"object":"chat.completion"`)
	assert.Contains(t, s, `"content":"你好"`)
	assert.Contains(t, s, `"finish_reason":"stop"`)
	assert.Contains(t, s, `"prompt_tokens":9`)
	assert.Contains(t, s, `"completion_tokens":1`)
	assert.Contains(t, s, `"total_tokens":10`)
}

// JSONL -> OpenAI SSE: content deltas, finish chunk, usage chunk, [DONE].
func TestCohereProvider_OnStreamingResponseBody(t *testing.T) {
	provider := &cohereProvider{}
	ctx := newMockMultipartHttpContext()

	stream := `{"event_type":"stream-start","generation_id":"g-1"}` + "\n" +
		`{"event_type":"text-generation","text":"你"}` + "\n" +
		`{"event_type":"stream-end","response":{"finish_reason":"COMPLETE","meta":{"tokens":{"input_tokens":9,"output_tokens":1}}}}` + "\n"
	out, err := provider.OnStreamingResponseBody(ctx, ApiNameChatCompletion, []byte(stream), true)
	require.NoError(t, err)
	s := string(out)
	assert.Contains(t, s, `"delta":{"role":"assistant"}`) // stream-start -> role-only first delta
	assert.Contains(t, s, `"content":"你"`)                // text-generation -> content delta
	assert.Contains(t, s, `"usage":null`)                 // the content delta chunk
	assert.Contains(t, s, `"finish_reason":"stop"`)       // the stream-end chunk
	assert.Contains(t, s, `"total_tokens":10`)            // usage populated on the final chunk
	assert.Contains(t, s, ssePrefix+streamEndDataValue)   // terminates with data: [DONE]
}

// A last-chunk line without a trailing newline must still be processed.
func TestCohereProvider_OnStreamingResponseBody_LastFrameNoNewline(t *testing.T) {
	provider := &cohereProvider{}
	ctx := newMockMultipartHttpContext()

	stream := `{"event_type":"text-generation","text":"你"}` + "\n" +
		`{"event_type":"stream-end","response":{"finish_reason":"COMPLETE","meta":{"tokens":{"input_tokens":9,"output_tokens":1}}}}` // no trailing newline
	out, err := provider.OnStreamingResponseBody(ctx, ApiNameChatCompletion, []byte(stream), true)
	require.NoError(t, err)
	s := string(out)
	assert.Contains(t, s, `"finish_reason":"stop"`) // stream-end must NOT be dropped
	assert.Contains(t, s, `"total_tokens":10`)
	assert.Contains(t, s, ssePrefix+streamEndDataValue)
}

func TestCohereProvider_OriginalProtocolPassthrough(t *testing.T) {
	provider := &cohereProvider{config: ProviderConfig{protocol: protocolOriginal}}
	ctx := newMockMultipartHttpContext()

	out, err := provider.TransformResponseBody(ctx, ApiNameChatCompletion, []byte("raw-cohere-body"))
	require.NoError(t, err)
	assert.Equal(t, "raw-cohere-body", string(out))

	out, err = provider.OnStreamingResponseBody(ctx, ApiNameChatCompletion, []byte("raw-chunk"), false)
	require.NoError(t, err)
	assert.Equal(t, "raw-chunk", string(out))
}

func TestCohereProvider_OnStreamingResponseBody_BuffersPartialLine(t *testing.T) {
	provider := &cohereProvider{}
	ctx := newMockMultipartHttpContext()

	// First chunk ends mid-line; nothing complete yet, so no content is emitted.
	out, err := provider.OnStreamingResponseBody(ctx, ApiNameChatCompletion, []byte(`{"event_type":"text-gene`), false)
	require.NoError(t, err)
	assert.NotContains(t, string(out), `"content"`)

	// The rest of the line arrives; the completed event is now converted.
	out, err = provider.OnStreamingResponseBody(ctx, ApiNameChatCompletion, []byte(`ration","text":"好"}`+"\n"), false)
	require.NoError(t, err)
	assert.Contains(t, string(out), `"content":"好"`)
}

func TestCohereFinishReason2OpenAI(t *testing.T) {
	assert.Equal(t, finishReasonStop, cohereFinishReason2OpenAI("COMPLETE"))
	assert.Equal(t, finishReasonLength, cohereFinishReason2OpenAI("MAX_TOKENS"))
	assert.Equal(t, finishReasonStop, cohereFinishReason2OpenAI("SOMETHING_ELSE"))
}

func TestCohereProvider_ResponseEdgeCases(t *testing.T) {
	provider := &cohereProvider{}
	ctx := newMockMultipartHttpContext()

	// Non-chat-completion responses (e.g. rerank) pass through untouched.
	out, err := provider.TransformResponseBody(ctx, ApiNameCohereV1Rerank, []byte("raw"))
	require.NoError(t, err)
	assert.Equal(t, "raw", string(out))

	// A malformed non-stream body is an error.
	_, err = provider.TransformResponseBody(ctx, ApiNameChatCompletion, []byte("{not json"))
	require.Error(t, err)

	// Non-chat streaming passes through; its final chunk returns nil.
	out, err = provider.OnStreamingResponseBody(ctx, ApiNameCohereV1Rerank, []byte("raw"), false)
	require.NoError(t, err)
	assert.Equal(t, "raw", string(out))
	out, err = provider.OnStreamingResponseBody(ctx, ApiNameCohereV1Rerank, []byte("raw"), true)
	require.NoError(t, err)
	assert.Nil(t, out)

	// A malformed stream line is skipped; the last chunk still terminates with [DONE].
	out, err = provider.OnStreamingResponseBody(ctx, ApiNameChatCompletion, []byte("{not json\n"), true)
	require.NoError(t, err)
	assert.Contains(t, string(out), ssePrefix+streamEndDataValue)
}

// SSE frames (event:/data:) must convert like JSONL, and stream-start's generation_id becomes the chunk id.
func TestCohereProvider_OnStreamingResponseBody_SSEModeAndId(t *testing.T) {
	provider := &cohereProvider{}
	ctx := newMockMultipartHttpContext()

	sse := "event: stream-start\n" +
		`data: {"event_type":"stream-start","generation_id":"gen-1"}` + "\n\n" +
		"event: text-generation\n" +
		`data: {"event_type":"text-generation","text":"hi"}` + "\n\n"
	out, err := provider.OnStreamingResponseBody(ctx, ApiNameChatCompletion, []byte(sse), false)
	require.NoError(t, err)
	s := string(out)
	assert.Contains(t, s, `"content":"hi"`)     // data: frame parsed despite the event: lines
	assert.Contains(t, s, `"id":"gen-1"`)       // generation_id reused as the chunk id
	assert.Contains(t, s, `"role":"assistant"`) // stream-start -> role delta
}
