package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChatMessageParseContentSkipsMalformedStructuredParts(t *testing.T) {
	validText := map[string]any{
		"type": contentTypeText,
		"text": "valid text",
	}
	wantText := []chatMessageContent{
		{
			Type: contentTypeText,
			Text: "valid text",
		},
		{
			Type: contentTypeText,
			Text: "valid text",
		},
	}
	tests := []struct {
		name      string
		malformed map[string]any
	}{
		{
			name: "image URL missing URL",
			malformed: map[string]any{
				"type":      contentTypeImageUrl,
				"image_url": map[string]any{},
			},
		},
		{
			name: "image URL has non-string URL",
			malformed: map[string]any{
				"type": contentTypeImageUrl,
				"image_url": map[string]any{
					"url": 1,
				},
			},
		},
		{
			name: "input audio missing data",
			malformed: map[string]any{
				"type": contentTypeInputAudio,
				"input_audio": map[string]any{
					"format": "wav",
				},
			},
		},
		{
			name: "input audio has non-string data",
			malformed: map[string]any{
				"type": contentTypeInputAudio,
				"input_audio": map[string]any{
					"data":   false,
					"format": "wav",
				},
			},
		},
		{
			name: "input audio missing format",
			malformed: map[string]any{
				"type": contentTypeInputAudio,
				"input_audio": map[string]any{
					"data": "audio data",
				},
			},
		},
		{
			name: "input audio has non-string format",
			malformed: map[string]any{
				"type": contentTypeInputAudio,
				"input_audio": map[string]any{
					"data":   "audio data",
					"format": true,
				},
			},
		},
		{
			name: "file missing file ID",
			malformed: map[string]any{
				"type": contentTypeFile,
				"file": map[string]any{},
			},
		},
		{
			name: "file has non-string file ID",
			malformed: map[string]any{
				"type": contentTypeFile,
				"file": map[string]any{
					"file_id": true,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message := chatMessage{Content: []any{validText, tt.malformed, validText}}

			assert.Equal(t, wantText, message.ParseContent())
		})
	}
}

func TestChatMessageParseContentReturnsEmptyForAllMalformedStructuredParts(t *testing.T) {
	message := chatMessage{Content: []any{
		map[string]any{
			"type":      contentTypeImageUrl,
			"image_url": map[string]any{},
		},
		map[string]any{
			"type": contentTypeInputAudio,
			"input_audio": map[string]any{
				"data": "audio data",
			},
		},
		map[string]any{
			"type": contentTypeFile,
			"file": map[string]any{
				"file_id": false,
			},
		},
	}}

	assert.Empty(t, message.ParseContent())
}

func TestChatMessageParseContentPreservesValidStructuredParts(t *testing.T) {
	message := chatMessage{Content: []any{
		map[string]any{
			"type": contentTypeImageUrl,
			"image_url": map[string]any{
				"url":    "https://example.com/image.png",
				"detail": "high",
			},
		},
		map[string]any{
			"type": contentTypeInputAudio,
			"input_audio": map[string]any{
				"data":   "audio data",
				"format": "wav",
			},
		},
		map[string]any{
			"type": contentTypeFile,
			"file": map[string]any{
				"file_id": "file-id",
			},
		},
	}}
	want := []chatMessageContent{
		{
			Type: contentTypeImageUrl,
			ImageUrl: &chatMessageContentImageUrl{
				Url:    "https://example.com/image.png",
				Detail: "high",
			},
		},
		{
			Type: contentTypeInputAudio,
			InputAudio: &chatMessageContentAudio{
				Data:   "audio data",
				Format: "wav",
			},
		},
		{
			Type: contentTypeFile,
			File: &chatMessageContentFile{
				FileId: "file-id",
			},
		},
	}

	assert.Equal(t, want, message.ParseContent())
}
