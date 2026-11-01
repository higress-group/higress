package provider

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestBuildOpenAIStreamChunkFromHunyuanWithoutChoices guards #4893: a native
// streaming frame without Choices (usage-only / error frames) must convert to
// a well-formed OpenAI chunk with an empty choices list instead of panicking
// on Choices[0].
func TestBuildOpenAIStreamChunkFromHunyuanWithoutChoices(t *testing.T) {
	chunk := &hunyuanTextGenDetailedResponseNonStreaming{
		Id: "chunk-id",
		Usage: hunyuanTextGenUsage{
			PromptTokens:     9,
			CompletionTokens: 1,
			TotalTokens:      10,
		},
	}

	out := buildOpenAIStreamChunkFromHunyuan("gpt-3", chunk)
	s := string(out)

	assert.True(t, strings.HasPrefix(s, "data: "))
	assert.Contains(t, s, `"choices":[]`)
	assert.Contains(t, s, `"model":"gpt-3"`)
	assert.Contains(t, s, `"total_tokens":10`)
}

func TestBuildOpenAIStreamChunkFromHunyuanWithDelta(t *testing.T) {
	chunk := &hunyuanTextGenDetailedResponseNonStreaming{
		Id: "chunk-id",
		Choices: []hunyuanTextGenChoice{{
			Delta: hunyuanChatMessage{Role: "assistant", Content: "h"},
		}},
	}

	out := buildOpenAIStreamChunkFromHunyuan("gpt-3", chunk)
	s := string(out)

	assert.Contains(t, s, `"delta":{"role":"assistant","content":"h"}`)
}

func TestBuildOpenAIStreamChunkFromHunyuanWithFinish(t *testing.T) {
	chunk := &hunyuanTextGenDetailedResponseNonStreaming{
		Id: "chunk-id",
		Choices: []hunyuanTextGenChoice{{
			FinishReason: "stop",
		}},
	}

	out := buildOpenAIStreamChunkFromHunyuan("gpt-3", chunk)
	s := string(out)

	assert.Contains(t, s, `"finish_reason":"stop"`)
}
