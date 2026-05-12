package text

import (
	"os"
	"testing"

	cfg "github.com/alibaba/higress/plugins/wasm-go/extensions/ai-security-guard/config"
	wasmlog "github.com/higress-group/wasm-go/pkg/log"
	"github.com/tidwall/gjson"
)

type noopPluginLog struct{}

func (noopPluginLog) Trace(string)                     {}
func (noopPluginLog) Tracef(string, ...interface{})    {}
func (noopPluginLog) Debug(string)                     {}
func (noopPluginLog) Debugf(string, ...interface{})    {}
func (noopPluginLog) Info(string)                      {}
func (noopPluginLog) Infof(string, ...interface{})     {}
func (noopPluginLog) Warn(string)                      {}
func (noopPluginLog) Warnf(string, ...interface{})     {}
func (noopPluginLog) Error(string)                     {}
func (noopPluginLog) Errorf(string, ...interface{})    {}
func (noopPluginLog) Critical(string)                  {}
func (noopPluginLog) Criticalf(string, ...interface{}) {}
func (noopPluginLog) ResetID(string)                   {}

func TestMain(m *testing.M) {
	wasmlog.SetPluginLog(noopPluginLog{})
	os.Exit(m.Run())
}

type fallbackPathMockContext struct {
	values map[string]interface{}
}

func (m *fallbackPathMockContext) GetContext(key string) interface{} {
	return m.values[key]
}

func (m *fallbackPathMockContext) SetContext(key string, value interface{}) {
	if m.values == nil {
		m.values = make(map[string]interface{})
	}
	m.values[key] = value
}

func TestAutoExtractResponseContent(t *testing.T) {
	tests := []struct {
		name          string
		body          string
		fallbackPaths []string
		want          string
	}{
		{
			name: "OpenAI format",
			body: `{"choices":[{"message":{"content":"hello world"}}]}`,
			want: "hello world",
		},
		{
			name: "Claude format simple",
			body: `{"content":[{"type":"text","text":"hello claude"}]}`,
			want: "hello claude",
		},
		{
			name: "Claude format with thinking block first",
			body: `{"content":[{"type":"thinking","thinking":"let me think..."},{"type":"text","text":"hello after thinking"}]}`,
			want: "hello after thinking",
		},
		{
			name: "Claude format multiple text blocks concatenated",
			body: `{"content":[{"type":"thinking","thinking":"..."},{"type":"text","text":"first"},{"type":"text","text":" second"}]}`,
			want: "first second",
		},
		{
			name: "Claude format first text block empty, second non-empty",
			body: `{"content":[{"type":"text","text":""},{"type":"text","text":"actual content"}]}`,
			want: "actual content",
		},
		{
			name: "empty body",
			body: `{}`,
			want: "",
		},
		{
			name: "no matching format",
			body: `{"result":"some other format"}`,
			want: "",
		},
		{
			name:          "custom fallback path",
			body:          `{"output":{"text":"custom fallback text"}}`,
			fallbackPaths: []string{"output.text"},
			want:          "custom fallback text",
		},
		{
			name:          "fallback path list with empty item",
			body:          `{"output":{"text":"custom fallback text"}}`,
			fallbackPaths: []string{" ", "output.text"},
			want:          "custom fallback text",
		},
		{
			name:          "fallback disabled explicitly",
			body:          `{"choices":[{"message":{"content":"hello world"}}]}`,
			fallbackPaths: []string{},
			want:          "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fallbackPaths := tt.fallbackPaths
			if fallbackPaths == nil {
				fallbackPaths = cfg.DefaultResponseFallbackJsonPaths()
			}
			got := autoExtractResponseContent([]byte(tt.body), fallbackPaths)
			if got != tt.want {
				t.Errorf("autoExtractResponseContent() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAutoExtractStreamingResponseContent(t *testing.T) {
	tests := []struct {
		name          string
		chunk         string
		fallbackPaths []string
		want          string
	}{
		{
			name:  "OpenAI streaming format",
			chunk: `{"choices":[{"delta":{"content":"hello"}}]}`,
			want:  "hello",
		},
		{
			name:  "Claude streaming format",
			chunk: `{"type":"content_block_delta","delta":{"type":"text_delta","text":"hello claude"}}`,
			want:  "hello claude",
		},
		{
			name:  "Claude thinking delta - no text extracted",
			chunk: `{"type":"content_block_delta","delta":{"type":"thinking_delta","thinking":"let me think"}}`,
			want:  "",
		},
		{
			name:  "empty chunk",
			chunk: `{}`,
			want:  "",
		},
		{
			name:  "OpenAI with data: prefix",
			chunk: "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}",
			want:  "hello",
		},
		{
			name:  "Claude with event: and data: prefix",
			chunk: "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"hello\"}}",
			want:  "hello",
		},
		{
			name: "OpenAI with multi-line data fields",
			chunk: `event: message
data: {
data: "choices": [{"delta": {"content": "hello multiline"}}]
data: }`,
			want: "hello multiline",
		},
		{
			name:  "data: [DONE] returns empty",
			chunk: "data: [DONE]",
			want:  "",
		},
		{
			name:          "custom streaming fallback path",
			chunk:         `{"payload":{"delta":"custom stream"}}`,
			fallbackPaths: []string{"payload.delta"},
			want:          "custom stream",
		},
		{
			name:          "streaming fallback disabled explicitly",
			chunk:         `{"choices":[{"delta":{"content":"hello"}}]}`,
			fallbackPaths: []string{},
			want:          "",
		},
		{
			name:  "empty chunk payload",
			chunk: "",
			want:  "",
		},
		{
			name:  "invalid json payload after data extraction",
			chunk: "data: invalid-json",
			want:  "",
		},
		{
			name:  "streaming payload with empty data line",
			chunk: "event: message\ndata:\ndata: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}",
			want:  "hello",
		},
		{
			name:  "streaming payload without data lines",
			chunk: "event: ping",
			want:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fallbackPaths := tt.fallbackPaths
			if fallbackPaths == nil {
				fallbackPaths = cfg.DefaultStreamingResponseFallbackJsonPaths()
			}
			got := autoExtractStreamingResponseContent([]byte(tt.chunk), fallbackPaths)
			if got != tt.want {
				t.Errorf("autoExtractStreamingResponseContent() = %q, want %q", got, tt.want)
			}
		})
	}
}

// Test that configured path takes priority over fallback.
func TestConfiguredPathPriority(t *testing.T) {
	// Body has both OpenAI and a custom field
	body := `{"choices":[{"message":{"content":"openai content"}}],"custom":"custom content"}`

	// Custom path extracts successfully - should NOT fall back
	content := extractWithFallback([]byte(body), "custom", cfg.DefaultResponseFallbackJsonPaths())
	if content != "custom content" {
		t.Errorf("expected custom path to take priority, got %q", content)
	}

	// Custom path misses - should fall back to OpenAI
	content = extractWithFallback([]byte(body), "nonexistent.path", cfg.DefaultResponseFallbackJsonPaths())
	if content != "openai content" {
		t.Errorf("expected fallback to OpenAI, got %q", content)
	}

	// Fallback disabled - should stay empty when configured path misses.
	content = extractWithFallback([]byte(body), "nonexistent.path", []string{})
	if content != "" {
		t.Errorf("expected empty result when fallback disabled, got %q", content)
	}
}

// extractWithFallback mirrors the real extraction logic in HandleTextGenerationResponseBody.
func extractWithFallback(body []byte, jsonPath string, fallbackPaths []string) string {
	content := gjsonGetString(body, jsonPath)
	if len(content) == 0 {
		content = autoExtractResponseContent(body, fallbackPaths)
	}
	return content
}

func gjsonGetString(body []byte, path string) string {
	return gjson.GetBytes(body, path).String()
}

// Test SSE body fallback for buffered streaming branch.
func TestAutoExtractStreamingResponseFromSSE(t *testing.T) {
	tests := []struct {
		name          string
		body          string
		fallbackPaths []string
		want          string
	}{
		{
			name: "OpenAI SSE body",
			body: "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\" world\"}}]}\n\ndata: [DONE]\n\n",
			want: "hello world",
		},
		{
			name: "Claude SSE body with thinking and text deltas",
			body: "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"hmm\"}}\n\n" +
				"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"hello\"}}\n\n" +
				"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\" claude\"}}\n\n" +
				"data: [DONE]\n\n",
			want: "hello claude",
		},
		{
			name: "empty SSE body",
			body: "data: [DONE]\n\n",
			want: "",
		},
		{
			name: "OpenAI multi-line data events in full SSE body",
			body: `event: message
data: {
data: "choices": [{"delta": {"content": "hello"}}]
data: }

event: message
data: {
data: "choices": [{"delta": {"content": " world"}}]
data: }

data: [DONE]

`,
			want: "hello world",
		},
		{
			name: "custom fallback paths in full SSE body",
			body: "data: {\"payload\":{\"delta\":\"hello\"}}\n\ndata: {\"payload\":{\"delta\":\" world\"}}\n\n",
			fallbackPaths: []string{
				"payload.delta",
			},
			want: "hello world",
		},
		{
			name:          "streaming fallback disabled for full SSE body",
			body:          "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n",
			fallbackPaths: []string{},
			want:          "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fallbackPaths := tt.fallbackPaths
			if fallbackPaths == nil {
				fallbackPaths = cfg.DefaultStreamingResponseFallbackJsonPaths()
			}
			got := autoExtractStreamingResponseFromSSE([]byte(tt.body), fallbackPaths)
			if got != tt.want {
				t.Errorf("autoExtractStreamingResponseFromSSE() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildEffectiveFallbackPaths(t *testing.T) {
	if paths := buildEffectiveFallbackPaths("choices.0.message.content", nil); len(paths) != 0 {
		t.Fatalf("expected empty paths when fallback list is nil, got %#v", paths)
	}

	emptyByFilter := buildEffectiveFallbackPaths("choices.0.message.content", []string{
		"choices.0.message.content",
		" ",
		"",
	})
	if len(emptyByFilter) != 0 {
		t.Fatalf("expected empty paths after filtering duplicates/empty values, got %#v", emptyByFilter)
	}

	paths := buildEffectiveFallbackPaths("choices.0.message.content", []string{
		"choices.0.message.content",
		"delta.text",
		"delta.text",
		"",
		"  ",
		"output.text",
	})
	if len(paths) != 2 {
		t.Fatalf("expected 2 paths after filtering, got %d", len(paths))
	}
	if paths[0] != "delta.text" || paths[1] != "output.text" {
		t.Fatalf("unexpected filtered fallback paths: %#v", paths)
	}
}

func TestGetEffectiveFallbackPathsFromContext(t *testing.T) {
	ctx := &fallbackPathMockContext{values: make(map[string]interface{})}
	got := getEffectiveFallbackPathsFromContext(ctx, "fallback_key", "choices.0.message.content", []string{
		"choices.0.message.content",
		"output.text",
	})
	if len(got) != 1 || got[0] != "output.text" {
		t.Fatalf("unexpected effective paths from uncached context: %#v", got)
	}
	if cached, ok := ctx.values["fallback_key"].([]string); !ok || len(cached) != 1 || cached[0] != "output.text" {
		t.Fatalf("expected effective paths to be cached in context, got %#v", ctx.values["fallback_key"])
	}

	ctx.values["fallback_key"] = []string{"cached.path"}
	got = getEffectiveFallbackPathsFromContext(ctx, "fallback_key", "nonexistent", []string{"another.path"})
	if len(got) != 1 || got[0] != "cached.path" {
		t.Fatalf("expected cached paths to take precedence, got %#v", got)
	}
}
