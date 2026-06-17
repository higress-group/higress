package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVllmProviderInitializer_DefaultCapabilities(t *testing.T) {
	initializer := &vllmProviderInitializer{}

	capabilities := initializer.DefaultCapabilities()
	expected := map[string]string{
		string(ApiNameChatCompletion):       PathOpenAIChatCompletions,
		string(ApiNameCompletion):           PathOpenAICompletions,
		string(ApiNameModels):               PathOpenAIModels,
		string(ApiNameEmbeddings):           PathOpenAIEmbeddings,
		string(ApiNameCohereV1Rerank):       PathCohereV1Rerank,
		string(ApiNameAnthropicMessages):    PathAnthropicMessages,
		string(ApiNameAnthropicCountTokens): PathAnthropicMessagesCountTokens,
		string(ApiNameResponses):            PathOpenAIResponses,
		string(ApiNameAudioTranscription):   PathOpenAIAudioTranscriptions,
		string(ApiNameAudioTranslation):     PathOpenAIAudioTranslations,
	}

	assert.Equal(t, expected, capabilities)
}

func TestVllmProvider_GetApiName(t *testing.T) {
	provider := &vllmProvider{}

	cases := []struct {
		path     string
		expected ApiName
	}{
		// existing (regression guard)
		{PathOpenAIChatCompletions, ApiNameChatCompletion},
		{PathOpenAICompletions, ApiNameCompletion},
		{PathOpenAIModels, ApiNameModels},
		{PathOpenAIEmbeddings, ApiNameEmbeddings},
		{PathCohereV1Rerank, ApiNameCohereV1Rerank},
		// new passthrough endpoints
		// count_tokens must be checked before /v1/messages (substring) — guards the ordering
		{PathAnthropicMessagesCountTokens, ApiNameAnthropicCountTokens},
		{PathAnthropicMessages, ApiNameAnthropicMessages},
		{PathOpenAIResponses, ApiNameResponses},
		{PathOpenAIAudioTranscriptions, ApiNameAudioTranscription},
		{PathOpenAIAudioTranslations, ApiNameAudioTranslation},
		// unknown path
		{"/v1/unknown", ApiName("")},
	}

	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			assert.Equal(t, c.expected, provider.GetApiName(c.path))
		})
	}
}
