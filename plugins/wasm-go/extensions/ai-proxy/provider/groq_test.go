package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGroqProviderGetApiName(t *testing.T) {
	provider := &groqProvider{}

	tests := []struct {
		name string
		path string
		want ApiName
	}{
		{
			name: "chat completions",
			path: groqChatCompletionPath,
			want: ApiNameChatCompletion,
		},
		{
			name: "responses",
			path: groqResponsesPath,
			want: ApiNameResponses,
		},
		{
			name: "responses with route prefix",
			path: "/gateway" + groqResponsesPath,
			want: ApiNameResponses,
		},
		{
			name: "unknown",
			path: "/openai/v1/unknown",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, provider.GetApiName(tt.path))
		})
	}
}
