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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Hunyuan sends usage on every frame; OpenAI wants it only on the final chunk.
func TestHunyuanProvider_ChunkUsageOnlyOnFinalChunk(t *testing.T) {
	provider := &hunyuanProvider{}
	ctx := newMockMultipartHttpContext()

	// Non-final frame (FinishReason empty): usage must be null.
	nonFinal := `{"Id":"x","Choices":[{"FinishReason":"","Delta":{"Role":"assistant","Content":"你"}}],"Usage":{"PromptTokens":9,"CompletionTokens":1,"TotalTokens":10}}`
	out, err := provider.convertChunkFromHunyuanToOpenAI(ctx, []byte(nonFinal))
	require.NoError(t, err)
	assert.Contains(t, string(out), `"usage":null`)
	assert.Contains(t, string(out), `"content":"你"`)

	// Final frame: finish chunk (usage:null) + separate choices:[] usage chunk.
	final := `{"Id":"x","Choices":[{"FinishReason":"stop","Delta":{"Role":"assistant","Content":""}}],"Usage":{"PromptTokens":9,"CompletionTokens":1,"TotalTokens":10}}`
	out, err = provider.convertChunkFromHunyuanToOpenAI(ctx, []byte(final))
	require.NoError(t, err)
	assert.Contains(t, string(out), `"finish_reason":"stop"`)
	assert.Contains(t, string(out), `"choices":[]`)      // usage-only chunk
	assert.Contains(t, string(out), `"total_tokens":10`) // usage populated on that chunk
	assert.True(t, strings.HasPrefix(string(out), ssePrefix))
}

// Hunyuan native has no [DONE]; the plugin appends it (auth set -> native path).
func TestHunyuanProvider_StreamingAppendsDoneOnLastChunk(t *testing.T) {
	provider := &hunyuanProvider{config: ProviderConfig{
		hunyuanAuthId:  "12345678-1234-1234-1234-123456789012",
		hunyuanAuthKey: "abcdefghijklmnopqrstuvwxyz012345",
	}}
	ctx := newMockMultipartHttpContext()

	// A non-final frame converts to a chunk without any terminator.
	frame := "data: {\"Id\":\"x\",\"Choices\":[{\"FinishReason\":\"\",\"Delta\":{\"Role\":\"assistant\",\"Content\":\"你\"}}],\"Usage\":{\"PromptTokens\":9,\"CompletionTokens\":1,\"TotalTokens\":10}}\n\n"
	out, err := provider.OnStreamingResponseBody(ctx, ApiNameChatCompletion, []byte(frame), false)
	require.NoError(t, err)
	assert.NotContains(t, string(out), "[DONE]")
	assert.Contains(t, string(out), `"content":"你"`)

	// The final chunk appends the OpenAI stream terminator.
	out, err = provider.OnStreamingResponseBody(ctx, ApiNameChatCompletion, []byte(""), true)
	require.NoError(t, err)
	assert.Contains(t, string(out), "data: [DONE]")
}

func TestHunyuanProvider_StreamingEdgeCases(t *testing.T) {
	ctx := newMockMultipartHttpContext()

	// OpenAI-compatible mode (no auth id/key) passes the chunk through unchanged.
	compat := &hunyuanProvider{}
	out, err := compat.OnStreamingResponseBody(ctx, ApiNameChatCompletion, []byte("raw"), false)
	require.NoError(t, err)
	assert.Equal(t, "raw", string(out))

	native := &hunyuanProvider{config: ProviderConfig{
		hunyuanAuthId:  "12345678-1234-1234-1234-123456789012",
		hunyuanAuthKey: "abcdefghijklmnopqrstuvwxyz012345",
	}}
	// An incomplete (unterminated) event on a non-final chunk is buffered, producing no output yet.
	out, err = native.OnStreamingResponseBody(ctx, ApiNameChatCompletion, []byte("data: {\"Id\":\"x\""), false)
	require.NoError(t, err)
	assert.Empty(t, string(out))

	// A malformed frame converts to an empty payload rather than erroring.
	out, err = native.convertChunkFromHunyuanToOpenAI(ctx, []byte("{not json"))
	require.NoError(t, err)
	assert.Empty(t, string(out))
}

// A choice-less frame (e.g. keep-alive) converts to empty output without panicking.
func TestHunyuanProvider_ChunkWithoutChoices(t *testing.T) {
	provider := &hunyuanProvider{}
	ctx := newMockMultipartHttpContext()
	out, err := provider.convertChunkFromHunyuanToOpenAI(ctx, []byte(`{"Id":"x","Choices":[],"Usage":{"PromptTokens":9,"CompletionTokens":1,"TotalTokens":10}}`))
	require.NoError(t, err)
	assert.Empty(t, string(out))
}
