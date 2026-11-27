package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHunyuanProviderGetApiName(t *testing.T) {
	provider := &hunyuanProvider{}

	tests := []struct {
		name string
		path string
		want ApiName
	}{
		{
			name: "chat completions",
			path: PathOpenAIChatCompletions,
			want: ApiNameChatCompletion,
		},
		{
			name: "embeddings",
			path: PathOpenAIEmbeddings,
			want: ApiNameEmbeddings,
		},
		{
			name: "embeddings with route prefix",
			path: "/gateway" + PathOpenAIEmbeddings,
			want: ApiNameEmbeddings,
		},
		{
			name: "unknown path defaults to chat completion",
			path: "/v1/unknown",
			want: ApiNameChatCompletion,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, provider.GetApiName(tt.path))
		})
	}
}
