package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCohereProviderGetApiName(t *testing.T) {
	provider := &cohereProvider{}

	tests := []struct {
		name string
		path string
		want ApiName
	}{
		{
			name: "chat",
			path: cohereChatCompletionPath,
			want: ApiNameChatCompletion,
		},
		{
			name: "rerank",
			path: cohereRerankPath,
			want: ApiNameCohereV1Rerank,
		},
		{
			name: "rerank with route prefix",
			path: "/gateway" + cohereRerankPath,
			want: ApiNameCohereV1Rerank,
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
