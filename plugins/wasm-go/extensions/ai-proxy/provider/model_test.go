package provider

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChatMessageParseContentSkipsMalformedParts(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []chatMessageContent
	}{
		{
			name:    "image_url without url",
			content: `[{"type":"image_url","image_url":{}}]`,
		},
		{
			name:    "image_url with non-string url",
			content: `[{"type":"image_url","image_url":{"url":123}}]`,
		},
		{
			name:    "input_audio without format",
			content: `[{"type":"input_audio","input_audio":{"data":"UklGRg=="}}]`,
		},
		{
			name:    "input_audio with non-string data",
			content: `[{"type":"input_audio","input_audio":{"data":1,"format":"wav"}}]`,
		},
		{
			name:    "file without file_id",
			content: `[{"type":"file","file":{}}]`,
		},
		{
			name:    "file with non-string file_id",
			content: `[{"type":"file","file":{"file_id":42}}]`,
		},
		{
			name: "valid parts are preserved in order",
			content: `[
				{"type":"text","text":"before"},
				{"type":"image_url","image_url":{"url":"https://example.com/cat.png","detail":"low"}},
				{"type":"image_url","image_url":{}},
				{"type":"input_audio","input_audio":{"data":"UklGRg==","format":"wav"}},
				{"type":"file","file":{"file_id":42}},
				{"type":"file","file":{"file_id":"file-abc"}},
				{"type":"text","text":"after"}
			]`,
			want: []chatMessageContent{
				{Type: contentTypeText, Text: "before"},
				{Type: contentTypeImageUrl, ImageUrl: &chatMessageContentImageUrl{Url: "https://example.com/cat.png", Detail: "low"}},
				{Type: contentTypeInputAudio, InputAudio: &chatMessageContentAudio{Data: "UklGRg==", Format: "wav"}},
				{Type: contentTypeFile, File: &chatMessageContentFile{FileId: "file-abc"}},
				{Type: contentTypeText, Text: "after"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var content any
			require.NoError(t, json.Unmarshal([]byte(tt.content), &content))
			message := &chatMessage{Content: content}

			var got []chatMessageContent
			require.NotPanics(t, func() {
				got = message.ParseContent()
			})
			require.Equal(t, tt.want, got)
		})
	}
}
