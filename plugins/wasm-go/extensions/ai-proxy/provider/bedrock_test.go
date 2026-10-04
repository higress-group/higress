package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBedrockProviderGetApiName(t *testing.T) {
	provider := &bedrockProvider{}

	tests := []struct {
		name string
		path string
		want ApiName
	}{
		{
			name: "converse",
			path: "/model/anthropic.claude-3-5-sonnet-20240620-v1:0/converse",
			want: ApiNameChatCompletion,
		},
		{
			name: "converse stream",
			path: "/model/anthropic.claude-3-5-sonnet-20240620-v1:0/converse-stream",
			want: ApiNameChatCompletion,
		},
		{
			name: "invoke",
			path: "/model/amazon.titan-embed-image-v1/invoke",
			want: ApiNameImageGeneration,
		},
		{
			name: "anthropic messages",
			path: bedrockMantleMessagesPath,
			want: ApiNameAnthropicMessages,
		},
		{
			name: "anthropic messages with route prefix",
			path: "/gateway" + bedrockMantleMessagesPath,
			want: ApiNameAnthropicMessages,
		},
		{
			name: "unknown",
			path: "/v1/unknown",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, provider.GetApiName(tt.path))
		})
	}
}
