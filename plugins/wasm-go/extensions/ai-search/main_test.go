// Copyright (c) 2026 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

type streamTestContext struct {
	wrapper.HttpContext
	values map[string]interface{}
}

func (ctx *streamTestContext) GetContext(key string) interface{}        { return ctx.values[key] }
func (ctx *streamTestContext) SetContext(key string, value interface{}) { ctx.values[key] = value }
func (ctx *streamTestContext) GetStringContext(key, fallback string) string {
	if value, ok := ctx.values[key].(string); ok {
		return value
	}
	return fallback
}
func (ctx *streamTestContext) GetBoolContext(key string, fallback bool) bool {
	if value, ok := ctx.values[key].(bool); ok {
		return value
	}
	return fallback
}

type streamTestLog struct{ log.Log }

func (streamTestLog) Debugf(string, ...interface{}) {}
func (streamTestLog) Errorf(string, ...interface{}) {}

func newStreamTestContext() *streamTestContext {
	log.SetPluginLog(streamTestLog{})
	return &streamTestContext{values: map[string]interface{}{"References": "[1] source"}}
}

var streamTestConfig = Config{referenceFormat: "References: %s", referenceLocation: "head"}

const streamTestReferences = "References: [1] source"

func contentEvent(content string) string {
	encoded, _ := json.Marshal(content)
	return fmt.Sprintf("data: {\"id\":\"chat-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":%s},\"finish_reason\":null}]}\n\n", encoded)
}

func terminalEvent(reason string) string {
	return fmt.Sprintf("data: {\"id\":\"chat-1\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":%q}],\"usage\":{\"completion_tokens\":3}}\n\n", reason)
}

func streamContent(t *testing.T, output string) string {
	t.Helper()
	var content strings.Builder
	for _, line := range strings.Split(output, "\n") {
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		body := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if body == "[DONE]" {
			continue
		}
		if !gjson.Valid(body) {
			t.Fatalf("invalid JSON event: %s", line)
		}
		content.WriteString(gjson.Get(body, "choices.0.delta.content").String())
	}
	return content.String()
}

func assertStreamBuffersEmpty(t *testing.T, ctx *streamTestContext) {
	t.Helper()
	if pending := ctx.GetStringContext(BUFFER_CONTENT_CONTEXT_KEY, ""); pending != "" {
		t.Errorf("unflushed content: %q", pending)
	}
	if pending := ctx.GetContext(PARTIAL_MESSAGE_CONTEXT_KEY); pending != nil {
		t.Errorf("unflushed SSE message: %q", pending)
	}
}

func TestStreamingShortAnswerFlushesAtTermination(t *testing.T) {
	for _, ending := range []string{"stop", "length", "content_filter", "tool_calls", "done", "eof", "unterminated"} {
		t.Run(ending, func(t *testing.T) {
			ctx := newStreamTestContext()
			answer := "你好, world!"
			first := contentEvent(answer)
			if ending == "unterminated" {
				first = strings.TrimSuffix(first, "\n\n")
			}
			output := string(onStreamingResponseBody(ctx, streamTestConfig, []byte(first), false))
			var last string
			switch ending {
			case "done":
				last = "data: [DONE]\n\n"
			case "eof", "unterminated":
			default:
				last = terminalEvent(ending)
			}
			output += string(onStreamingResponseBody(ctx, streamTestConfig, []byte(last), true))
			if got, want := streamContent(t, output), streamTestReferences+"\n\n"+answer; got != want {
				t.Fatalf("content = %q, want %q", got, want)
			}
			if ending == "done" && !strings.HasSuffix(output, last) {
				t.Fatalf("buffer must flush before [DONE]: %s", output)
			}
			if last != "" && ending != "done" {
				if !strings.Contains(output, `"finish_reason":"`+ending+`"`) || !strings.Contains(output, `"completion_tokens":3`) {
					t.Fatalf("terminal metadata lost: %s", output)
				}
			}
			assertStreamBuffersEmpty(t, ctx)
		})
	}
}

func TestStreamingReasoningContentIsLossless(t *testing.T) {
	for split := 0; split <= len("</think>"); split++ {
		t.Run(fmt.Sprintf("closing_tag_split_%d", split), func(t *testing.T) {
			ctx := newStreamTestContext()
			parts := []string{"<thi", "nk>", strings.Repeat("reason ", 6), "more ", "</think>"[:split], "</think>"[split:] + "answer"}
			var output strings.Builder
			for _, part := range parts {
				output.Write(onStreamingResponseBody(ctx, streamTestConfig, []byte(contentEvent(part)), false))
			}
			output.Write(onStreamingResponseBody(ctx, streamTestConfig, []byte(terminalEvent("stop")+"data: [DONE]\n\n"), true))
			want := "<think>" + strings.Repeat("reason ", 6) + "more </think>\n" + streamTestReferences + "\n\nanswer"
			if got := streamContent(t, output.String()); got != want {
				t.Fatalf("content = %q, want %q", got, want)
			}
			assertStreamBuffersEmpty(t, ctx)
		})
	}
}

func TestStreamingUnfinishedReasoningFlushesVerbatim(t *testing.T) {
	for _, answer := range []string{"<think>short", "<think>" + strings.Repeat("reason ", 6) + "</thi"} {
		for _, last := range []string{"", "data: [DONE]\n\n", terminalEvent("length")} {
			t.Run(fmt.Sprintf("%s/%s", answer, last), func(t *testing.T) {
				ctx := newStreamTestContext()
				output := string(onStreamingResponseBody(ctx, streamTestConfig, []byte(contentEvent(answer)), false))
				output += string(onStreamingResponseBody(ctx, streamTestConfig, []byte(last), true))
				if got := streamContent(t, output); got != answer {
					t.Fatalf("content = %q, want %q", got, answer)
				}
				assertStreamBuffersEmpty(t, ctx)
			})
		}
	}
}

func TestStreamingProcessesReferencesOnlyOncePerChunk(t *testing.T) {
	ctx := newStreamTestContext()
	answer := strings.Repeat("x", BUFFER_SIZE)
	input := contentEvent(answer) + contentEvent("next") + contentEvent("last") + terminalEvent("stop") + "data: [DONE]\n\n"
	output := string(onStreamingResponseBody(ctx, streamTestConfig, []byte(input), true))
	if got, want := streamContent(t, output), streamTestReferences+"\n\n"+answer+"nextlast"; got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
	if !strings.Contains(output, contentEvent("next")+contentEvent("last")+terminalEvent("stop")) {
		t.Fatalf("events after references must pass through unchanged: %s", output)
	}
	assertStreamBuffersEmpty(t, ctx)
}

func TestStreamingPreservesPartialMessageAfterReferences(t *testing.T) {
	ctx := newStreamTestContext()
	answer := strings.Repeat("x", BUFFER_SIZE)
	next := contentEvent("remaining")
	first := contentEvent(answer) + next[:len(next)/2]
	output := string(onStreamingResponseBody(ctx, streamTestConfig, []byte(first), false))
	output += string(onStreamingResponseBody(ctx, streamTestConfig, []byte(next[len(next)/2:]+"data: [DONE]\n\n"), true))
	if got, want := streamContent(t, output), streamTestReferences+"\n\n"+answer+"remaining"; got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
	assertStreamBuffersEmpty(t, ctx)
}

func TestStreamingAllChunkBoundaries(t *testing.T) {
	input := contentEvent("<think>brief</thi") + contentEvent("nk>answer") + terminalEvent("stop") + "data: [DONE]\n\n"
	want := "<think>brief</think>\n" + streamTestReferences + "\n\nanswer"
	for split := 0; split <= len(input); split++ {
		ctx := newStreamTestContext()
		output := string(onStreamingResponseBody(ctx, streamTestConfig, []byte(input[:split]), false))
		output += string(onStreamingResponseBody(ctx, streamTestConfig, []byte(input[split:]), true))
		if got := streamContent(t, output); got != want {
			t.Fatalf("split %d: content = %q, want %q", split, got, want)
		}
		assertStreamBuffersEmpty(t, ctx)
	}
	ctx := newStreamTestContext()
	var output strings.Builder
	for i := range input {
		output.Write(onStreamingResponseBody(ctx, streamTestConfig, []byte(input[i:i+1]), i == len(input)-1))
	}
	if got := streamContent(t, output.String()); got != want {
		t.Fatalf("single-byte chunks: content = %q, want %q", got, want)
	}
	assertStreamBuffersEmpty(t, ctx)
}

func TestStreamingPreservesSSEFields(t *testing.T) {
	ctx := newStreamTestContext()
	input := "id: event-1\nevent: message\n" + contentEvent(strings.Repeat("x", BUFFER_SIZE))
	output := string(onStreamingResponseBody(ctx, streamTestConfig, []byte(input), true))
	if !strings.HasPrefix(output, "id: event-1\nevent: message\n") {
		t.Fatalf("SSE fields lost: %s", output)
	}
}

func TestStreamingWithoutReferencesPassesThrough(t *testing.T) {
	ctx := newStreamTestContext()
	ctx.SetContext("References", "")
	input := "data: partial"
	if got := string(onStreamingResponseBody(ctx, streamTestConfig, []byte(input), true)); got != input {
		t.Fatalf("output = %q, want %q", got, input)
	}
}

func TestStreamingTailReferencesRemainAtStop(t *testing.T) {
	ctx := newStreamTestContext()
	config := streamTestConfig
	config.referenceLocation = "tail"
	input := contentEvent("answer") + terminalEvent("stop") + "data: [DONE]\n\n"
	output := string(onStreamingResponseBody(ctx, config, []byte(input), true))
	if got, want := streamContent(t, output), "answer\n\n"+streamTestReferences; got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
	assertStreamBuffersEmpty(t, ctx)
}

func TestStreamingPreservesNonContentEvents(t *testing.T) {
	ctx := newStreamTestContext()
	role := "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":null,\"reasoning_content\":null},\"finish_reason\":null}]}\n\n"
	reasoning := "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"reasoning\"},\"finish_reason\":null}]}\n\n"
	input := role + reasoning + contentEvent("answer") + terminalEvent("stop")
	output := string(onStreamingResponseBody(ctx, streamTestConfig, []byte(input), true))
	if !strings.HasPrefix(output, role+reasoning) {
		t.Fatalf("role or reasoning event changed: %s", output)
	}
	if got, want := streamContent(t, output), streamTestReferences+"\n\nanswer"; got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
	assertStreamBuffersEmpty(t, ctx)
}

func TestStreamingFinishEventContentIsIncluded(t *testing.T) {
	ctx := newStreamTestContext()
	last := strings.Replace(contentEvent("last"), `"finish_reason":null`, `"finish_reason":"stop"`, 1)
	input := contentEvent("first ") + last + "data: [DONE]\n\n"
	output := string(onStreamingResponseBody(ctx, streamTestConfig, []byte(input), true))
	if got, want := streamContent(t, output), streamTestReferences+"\n\nfirst last"; got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
	assertStreamBuffersEmpty(t, ctx)
}

func TestStreamingThinkTagInAnswerIsNotReasoning(t *testing.T) {
	ctx := newStreamTestContext()
	answer := "The <think> tag marks reasoning."
	output := string(onStreamingResponseBody(ctx, streamTestConfig, []byte(contentEvent(answer)), true))
	if got, want := streamContent(t, output), streamTestReferences+"\n\n"+answer; got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
	assertStreamBuffersEmpty(t, ctx)
}
