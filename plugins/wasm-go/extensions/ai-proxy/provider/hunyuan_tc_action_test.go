package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHunyuanTCActionForApiName(t *testing.T) {
	tests := []struct {
		name    string
		apiName ApiName
		want    string
	}{
		{"chat completion", ApiNameChatCompletion, hunyuanChatCompletionTCAction},
		{"embeddings", ApiNameEmbeddings, hunyuanEmbeddingsTCAction},
		{"other", ApiNameModels, hunyuanChatCompletionTCAction},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, hunyuanTCActionForApiName(tt.apiName))
		})
	}
}
