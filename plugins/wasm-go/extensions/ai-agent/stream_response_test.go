package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/stretchr/testify/require"
)

// nopLog discards every log line. The parser assertions below only need a
// log.Log to satisfy the function signature.
type nopLog struct{}

func (nopLog) Trace(string)                     {}
func (nopLog) Tracef(string, ...interface{})    {}
func (nopLog) Debug(string)                     {}
func (nopLog) Debugf(string, ...interface{})    {}
func (nopLog) Info(string)                      {}
func (nopLog) Infof(string, ...interface{})     {}
func (nopLog) Warn(string)                      {}
func (nopLog) Warnf(string, ...interface{})     {}
func (nopLog) Error(string)                     {}
func (nopLog) Errorf(string, ...interface{})    {}
func (nopLog) Critical(string)                  {}
func (nopLog) Criticalf(string, ...interface{}) {}
func (nopLog) ResetID(string)                   {}

var _ log.Log = nopLog{}

// dataFrames returns the payload of every "data:" frame in an SSE body, in
// order. It fails the test when a block is not a data frame, which is exactly
// what a raw newline injected from the model output would produce.
func dataFrames(t *testing.T, body string) []string {
	t.Helper()
	var frames []string
	for _, block := range strings.Split(body, "\n\n") {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		require.True(t, strings.HasPrefix(block, "data:"), "SSE block is not a data frame: %q", block)
		frames = append(frames, strings.TrimPrefix(block, "data:"))
	}
	return frames
}

// TestBuildStreamResponseBodyEscapesModelOutput is the regression test for the
// hand-written SSE template: model output that contains double quotes,
// backslashes, newlines or whole JSON objects used to be pasted verbatim into
// the frame, producing invalid JSON (and, for blank lines, extra data frames).
func TestBuildStreamResponseBodyEscapesModelOutput(t *testing.T) {
	rawResponse := Response{
		ID:    `chatcmpl-"quoted"\id`,
		Model: `qwen"turbo`,
		Usage: Usage{PromptTokens: 11, CompletionTokens: 22, TotalTokens: 33},
	}

	for _, tc := range []struct {
		name  string
		value string
	}{
		{name: "plain text", value: "The weather is sunny."},
		{name: "double quotes", value: `He said "hi" and left.`},
		{name: "backslashes", value: `open C:\temp\new folder`},
		{name: "quote after backslash", value: `say "C:\temp\"`},
		{name: "newline", value: "line one\nline two"},
		{name: "blank line cannot inject a frame", value: "before\n\ndata:{\"injected\":true}\n\nafter"},
		{name: "json object", value: `{"action":"getWeather","action_input":{"city":"Beijing"}}`},
		{name: "unicode", value: "\u591a\u4e91 25\u00b0C"},
		{name: "empty", value: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := string(buildStreamResponseBody(tc.value, rawResponse))
			require.NotEmpty(t, body)

			frames := dataFrames(t, body)
			require.Len(t, frames, 2, "the answer must produce exactly one chunk frame plus [DONE]")
			require.Equal(t, "[DONE]", frames[1])

			// The chunk must be valid JSON and confined to one SSE line so that
			// newlines in the model output cannot splice in extra frames.
			require.True(t, json.Valid([]byte(frames[0])), "chunk payload is not valid JSON: %s", frames[0])
			require.NotContains(t, frames[0], "\n")

			var chunk streamResponseChunk
			require.NoError(t, json.Unmarshal([]byte(frames[0]), &chunk))
			require.Equal(t, rawResponse.ID, chunk.ID)
			require.Equal(t, rawResponse.Model, chunk.Model)
			require.Equal(t, "chat.completion", chunk.Object)
			require.Equal(t, rawResponse.Usage, chunk.Usage)
			require.Len(t, chunk.Choices, 1)
			require.Equal(t, 0, chunk.Choices[0].Index)
			require.Equal(t, "assistant", chunk.Choices[0].Delta.Role)
			require.Equal(t, "stop", chunk.Choices[0].FinishReason)
			// The content has to survive the round trip byte for byte.
			require.Equal(t, tc.value, chunk.Choices[0].Delta.Content)
		})
	}
}

// TestBuildStreamResponseBodyFrameLayout pins the exact framing the SSE clients
// rely on: a single "data:" chunk followed by the "data:[DONE]" terminator.
func TestBuildStreamResponseBodyFrameLayout(t *testing.T) {
	body := string(buildStreamResponseBody("ok", Response{ID: "id", Model: "m"}))

	require.True(t, strings.HasPrefix(body, "data:{"))
	require.True(t, strings.HasSuffix(body, "\n\ndata:[DONE]\n\n"))
	require.Equal(t, 2, strings.Count(body, "data:"))
}

// TestOutputParserFallbackDecodesQuotedActionInput covers the regex fallback:
// a captured JSON string literal must be decoded to its raw value, while a
// captured JSON object is kept as JSON text.
func TestOutputParserFallbackDecodesQuotedActionInput(t *testing.T) {
	logger := nopLog{}
	for _, tc := range []struct {
		name       string
		response   string
		wantAction string
		wantInput  string
	}{
		{
			name:       "quoted string",
			response:   `{"action": "search", "action_input": "hello world"} trailing`,
			wantAction: "search",
			wantInput:  "hello world",
		},
		{
			name:       "quoted string with escapes",
			response:   "{\"action\": \"search\", \"action_input\": \"a\\tb\"} trailing",
			wantAction: "search",
			wantInput:  "a\tb",
		},
		{
			name:       "json object",
			response:   `{"action": "getWeather", "action_input": {"city": "Beijing"}} trailing`,
			wantAction: "getWeather",
			wantInput:  `{"city": "Beijing"}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			action, input := outputParser(tc.response, logger)
			require.Equal(t, tc.wantAction, action)
			require.Equal(t, tc.wantInput, input)
		})
	}
}
