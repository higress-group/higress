package provider

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChatMessageParseContentSkipsMalformedMultimodalParts(t *testing.T) {
	message := chatMessage{Content: []any{
		map[string]any{"type": contentTypeImageUrl, contentTypeImageUrl: map[string]any{}},
		map[string]any{"type": contentTypeInputAudio, contentTypeInputAudio: map[string]any{"data": "audio", "format": 1}},
		map[string]any{"type": contentTypeFile, contentTypeFile: map[string]any{"file_id": nil}},
		map[string]any{"type": contentTypeText, contentTypeText: "kept"},
	}}

	content := message.ParseContent()
	require.Len(t, content, 1)
	require.Equal(t, contentTypeText, content[0].Type)
	require.Equal(t, "kept", content[0].Text)
}

func TestChatMessageParseContentPreservesValidMultimodalParts(t *testing.T) {
	message := chatMessage{Content: []any{
		map[string]any{"type": contentTypeImageUrl, contentTypeImageUrl: map[string]any{"url": "https://example.com/image.png", "detail": "low"}},
		map[string]any{"type": contentTypeInputAudio, contentTypeInputAudio: map[string]any{"data": "audio", "format": "wav"}},
		map[string]any{"type": contentTypeFile, contentTypeFile: map[string]any{"file_id": "file-1"}},
	}}

	content := message.ParseContent()
	require.Len(t, content, 3)
	require.Equal(t, "https://example.com/image.png", content[0].ImageUrl.Url)
	require.Equal(t, "low", content[0].ImageUrl.Detail)
	require.Equal(t, "audio", content[1].InputAudio.Data)
	require.Equal(t, "wav", content[1].InputAudio.Format)
	require.Equal(t, "file-1", content[2].File.FileId)
}
