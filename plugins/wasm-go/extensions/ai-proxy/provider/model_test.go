package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestChatMessageParseContent covers the array-shaped message content that
// OpenAI-compatible clients send. ParseContent is fed with raw `any` values
// decoded straight from the request JSON, so it must never trust the type or
// presence of the nested fields: a part like {"type":"image_url","image_url":{}}
// used to panic the whole request because of the unchecked `subObj["url"].(string)`
// assertions.
func TestChatMessageParseContent(t *testing.T) {
	tests := []struct {
		name    string
		content any
		want    []chatMessageContent
	}{
		{
			name:    "nil content yields nil",
			content: nil,
			want:    nil,
		},
		{
			name:    "plain string becomes a single text part",
			content: "hello",
			want: []chatMessageContent{
				{Type: contentTypeText, Text: "hello"},
			},
		},
		{
			name: "text part is preserved",
			content: []any{
				map[string]any{"type": contentTypeText, "text": "hi"},
			},
			want: []chatMessageContent{
				{Type: contentTypeText, Text: "hi"},
			},
		},
		{
			name: "text part with non-string text is dropped",
			content: []any{
				map[string]any{"type": contentTypeText, "text": 1},
			},
			want: nil,
		},
		{
			name: "valid image_url with detail",
			content: []any{
				map[string]any{
					"type": contentTypeImageUrl,
					"image_url": map[string]any{
						"url":    "https://example.com/a.png",
						"detail": "high",
					},
				},
			},
			want: []chatMessageContent{
				{
					Type:     contentTypeImageUrl,
					ImageUrl: &chatMessageContentImageUrl{Url: "https://example.com/a.png", Detail: "high"},
				},
			},
		},
		{
			name: "valid image_url without detail",
			content: []any{
				map[string]any{
					"type":      contentTypeImageUrl,
					"image_url": map[string]any{"url": "https://example.com/a.png"},
				},
			},
			want: []chatMessageContent{
				{
					Type:     contentTypeImageUrl,
					ImageUrl: &chatMessageContentImageUrl{Url: "https://example.com/a.png"},
				},
			},
		},
		{
			name: "image_url with empty object is dropped without panicking",
			content: []any{
				map[string]any{"type": contentTypeImageUrl, "image_url": map[string]any{}},
			},
			want: nil,
		},
		{
			name: "image_url with numeric url is dropped",
			content: []any{
				map[string]any{"type": contentTypeImageUrl, "image_url": map[string]any{"url": 42}},
			},
			want: nil,
		},
		{
			name: "image_url with empty string url is dropped",
			content: []any{
				map[string]any{"type": contentTypeImageUrl, "image_url": map[string]any{"url": ""}},
			},
			want: nil,
		},
		{
			name: "image_url field that is not an object is dropped",
			content: []any{
				map[string]any{"type": contentTypeImageUrl, "image_url": "https://example.com/a.png"},
			},
			want: nil,
		},
		{
			name: "valid input_audio",
			content: []any{
				map[string]any{
					"type":        contentTypeInputAudio,
					"input_audio": map[string]any{"data": "AAAA", "format": "wav"},
				},
			},
			want: []chatMessageContent{
				{
					Type:       contentTypeInputAudio,
					InputAudio: &chatMessageContentAudio{Data: "AAAA", Format: "wav"},
				},
			},
		},
		{
			name: "input_audio missing data is dropped",
			content: []any{
				map[string]any{"type": contentTypeInputAudio, "input_audio": map[string]any{"format": "wav"}},
			},
			want: nil,
		},
		{
			name: "input_audio with numeric format is dropped",
			content: []any{
				map[string]any{"type": contentTypeInputAudio, "input_audio": map[string]any{"data": "AAAA", "format": 2}},
			},
			want: nil,
		},
		{
			name: "input_audio with empty object is dropped",
			content: []any{
				map[string]any{"type": contentTypeInputAudio, "input_audio": map[string]any{}},
			},
			want: nil,
		},
		{
			name: "input_audio field that is not an object is dropped",
			content: []any{
				map[string]any{"type": contentTypeInputAudio, "input_audio": "AAAA"},
			},
			want: nil,
		},
		{
			name: "valid file part",
			content: []any{
				map[string]any{
					"type": contentTypeFile,
					"file": map[string]any{"file_id": "file-123"},
				},
			},
			want: []chatMessageContent{
				{
					Type: contentTypeFile,
					File: &chatMessageContentFile{FileId: "file-123"},
				},
			},
		},
		{
			name: "file part with empty object is dropped",
			content: []any{
				map[string]any{"type": contentTypeFile, "file": map[string]any{}},
			},
			want: nil,
		},
		{
			name: "file part with numeric file_id is dropped",
			content: []any{
				map[string]any{"type": contentTypeFile, "file": map[string]any{"file_id": 7}},
			},
			want: nil,
		},
		{
			name: "file field that is not an object is dropped",
			content: []any{
				map[string]any{"type": contentTypeFile, "file": []any{"file-123"}},
			},
			want: nil,
		},
		{
			name: "non-map content item is skipped",
			content: []any{
				"not-a-map",
				map[string]any{"type": contentTypeText, "text": "kept"},
			},
			want: []chatMessageContent{
				{Type: contentTypeText, Text: "kept"},
			},
		},
		{
			name: "unknown content type is ignored",
			content: []any{
				map[string]any{"type": "video", "video": map[string]any{"url": "https://example.com/a.mp4"}},
			},
			want: nil,
		},
		{
			name: "mixed valid and malformed parts keep only the valid ones",
			content: []any{
				map[string]any{"type": contentTypeText, "text": "describe this"},
				map[string]any{"type": contentTypeImageUrl, "image_url": map[string]any{}},
				map[string]any{
					"type":      contentTypeImageUrl,
					"image_url": map[string]any{"url": "https://example.com/ok.png"},
				},
				map[string]any{"type": contentTypeInputAudio, "input_audio": map[string]any{"data": 1, "format": 2}},
				map[string]any{"type": contentTypeFile, "file": map[string]any{}},
			},
			want: []chatMessageContent{
				{Type: contentTypeText, Text: "describe this"},
				{
					Type:     contentTypeImageUrl,
					ImageUrl: &chatMessageContentImageUrl{Url: "https://example.com/ok.png"},
				},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			msg := &chatMessage{Role: roleUser, Content: tt.content}
			// The assertion is that this call never panics; assert.Equal then
			// pins down exactly which parts survived.
			require.NotPanics(t, func() {
				assert.Equal(t, tt.want, msg.ParseContent())
			})
		})
	}
}

// TestChatMessageParseContentDoesNotPanicOnHostileInput feeds ParseContent a
// batch of adversarial content shapes that previously triggered the unchecked
// assertions. Each message must be handled without panicking.
func TestChatMessageParseContentDoesNotPanicOnHostileInput(t *testing.T) {
	hostile := []any{
		map[string]any{"type": contentTypeImageUrl, "image_url": map[string]any{}},
		map[string]any{"type": contentTypeImageUrl, "image_url": map[string]any{"url": 3.14}},
		map[string]any{"type": contentTypeImageUrl, "image_url": []any{"x"}},
		map[string]any{"type": contentTypeImageUrl},
		map[string]any{"type": contentTypeInputAudio, "input_audio": map[string]any{"data": true}},
		map[string]any{"type": contentTypeInputAudio, "input_audio": []any{}},
		map[string]any{"type": contentTypeFile, "file": map[string]any{}},
		map[string]any{"type": contentTypeFile, "file": map[string]any{"file_id": map[string]any{}}},
		map[string]any{"type": nil},
		map[string]any{},
	}

	for i, part := range hostile {
		msg := &chatMessage{Role: roleUser, Content: []any{part}}
		require.NotPanicsf(t, func() {
			msg.ParseContent()
		}, "hostile part %d panicked: %#v", i, part)
	}

	// The whole message list must survive both the parse step and the
	// consecutive-message merge that also calls ParseContent internally.
	require.NotPanics(t, func() {
		mergeMessageContent(hostile, []any{
			map[string]any{"type": contentTypeImageUrl, "image_url": map[string]any{}},
		})
	})
}

// TestMergeMessageContentWithMalformedParts guards the request_helper.go path
// (mergeMessageContent -> ParseContent) against the same malformed content.
func TestMergeMessageContentWithMalformedParts(t *testing.T) {
	merged := mergeMessageContent(
		[]any{map[string]any{"type": contentTypeImageUrl, "image_url": map[string]any{}}},
		"follow up",
	)

	require.NotNil(t, merged)
	parts, ok := merged.([]chatMessageContent)
	require.True(t, ok)
	require.Len(t, parts, 1)
	assert.Equal(t, contentTypeText, parts[0].Type)
	assert.Equal(t, "follow up", parts[0].Text)
}

// TestBuildClaudeTextGenRequestWithMalformedContent reproduces the reported
// crash end to end: a claude provider request whose user message carries a
// malformed image_url part must be converted without panicking, and the valid
// parts must still make it into the upstream request.
func TestBuildClaudeTextGenRequestWithMalformedContent(t *testing.T) {
	provider := &claudeProvider{
		config: ProviderConfig{claudeCodeMode: false},
	}

	request := &chatCompletionRequest{
		Model:     "claude-sonnet-4-5-20250929",
		MaxTokens: 1024,
		Messages: []chatMessage{
			{
				Role: roleUser,
				Content: []any{
					map[string]any{"type": contentTypeText, "text": "what is in this image?"},
					map[string]any{"type": contentTypeImageUrl, "image_url": map[string]any{}},
					map[string]any{"type": contentTypeImageUrl, "image_url": map[string]any{"url": 123}},
					map[string]any{"type": contentTypeFile, "file": map[string]any{}},
					map[string]any{
						"type":      contentTypeImageUrl,
						"image_url": map[string]any{"url": "https://example.com/ok.png"},
					},
				},
			},
		},
	}

	var claudeReq *claudeTextGenRequest
	require.NotPanics(t, func() {
		claudeReq = provider.buildClaudeTextGenRequest(request)
	})

	require.NotNil(t, claudeReq)
	require.Len(t, claudeReq.Messages, 1)
	blocks := claudeReq.Messages[0].Content.GetArrayValue()
	require.Len(t, blocks, 2)
	assert.Equal(t, contentTypeText, blocks[0].Type)
	assert.Equal(t, "what is in this image?", blocks[0].Text)
	assert.Equal(t, "image", blocks[1].Type)
	require.NotNil(t, blocks[1].Source)
	assert.Equal(t, "https://example.com/ok.png", blocks[1].Source.Url)
}
