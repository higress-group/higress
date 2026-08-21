package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBaiduProviderGetApiName(t *testing.T) {
	provider := &baiduProvider{}

	tests := []struct {
		name string
		path string
		want ApiName
	}{
		{
			name: "chat completions",
			path: baiduChatCompletionPath,
			want: ApiNameChatCompletion,
		},
		{
			name: "embeddings",
			path: baiduEmbeddings,
			want: ApiNameEmbeddings,
		},
		{
			name: "embeddings with route prefix",
			path: "/gateway" + baiduEmbeddings,
			want: ApiNameEmbeddings,
		},
		{
			name: "unknown",
			path: "/v2/unknown",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, provider.GetApiName(tt.path))
		})
	}
}
