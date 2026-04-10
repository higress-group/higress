package main

import (
	"testing"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-proxy/provider"
	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-proxy/test"
)

func Test_getApiName(t *testing.T) {
	tests := []struct {
		name string
		path string
		want provider.ApiName
	}{
		// OpenAI style
		{"openai chat completions", "/v1/chat/completions", provider.ApiNameChatCompletion},
		{"openai completions", "/v1/completions", provider.ApiNameCompletion},
		{"openai embeddings", "/v1/embeddings", provider.ApiNameEmbeddings},
		{"openai audio speech", "/v1/audio/speech", provider.ApiNameAudioSpeech},
		{"openai audio transcriptions", "/v1/audio/transcriptions", provider.ApiNameAudioTranscription},
		{"openai audio transcriptions with prefix", "/proxy/v1/audio/transcriptions", provider.ApiNameAudioTranscription},
		{"openai audio translations", "/v1/audio/translations", provider.ApiNameAudioTranslation},
		{"openai realtime", "/v1/realtime", provider.ApiNameRealtime},
		{"openai realtime with prefix", "/proxy/v1/realtime", provider.ApiNameRealtime},
		{"openai realtime with trailing slash", "/v1/realtime/", ""},
		{"openai image generation", "/v1/images/generations", provider.ApiNameImageGeneration},
		{"openai image variation", "/v1/images/variations", provider.ApiNameImageVariation},
		{"openai image edit", "/v1/images/edits", provider.ApiNameImageEdit},
		{"openai batches", "/v1/batches", provider.ApiNameBatches},
		{"openai retrieve batch", "/v1/batches/batchid", provider.ApiNameRetrieveBatch},
		{"openai cancel batch", "/v1/batches/batchid/cancel", provider.ApiNameCancelBatch},
		{"openai files", "/v1/files", provider.ApiNameFiles},
		{"openai retrieve file", "/v1/files/fileid", provider.ApiNameRetrieveFile},
		{"openai retrieve file content", "/v1/files/fileid/content", provider.ApiNameRetrieveFileContent},
		{"openai videos", "/v1/videos", provider.ApiNameVideos},
		{"openai retrieve video", "/v1/videos/videoid", provider.ApiNameRetrieveVideo},
		{"openai retrieve video content", "/v1/videos/videoid/content", provider.ApiNameRetrieveVideoContent},
		{"openai video remix", "/v1/videos/videoid/remix", provider.ApiNameVideoRemix},
		{"openai models", "/v1/models", provider.ApiNameModels},
		{"openai fine tuning jobs", "/v1/fine_tuning/jobs", provider.ApiNameFineTuningJobs},
		{"openai retrieve fine tuning job", "/v1/fine_tuning/jobs/jobid", provider.ApiNameRetrieveFineTuningJob},
		{"openai fine tuning job events", "/v1/fine_tuning/jobs/jobid/events", provider.ApiNameFineTuningJobEvents},
		{"openai fine tuning job checkpoints", "/v1/fine_tuning/jobs/jobid/checkpoints", provider.ApiNameFineTuningJobCheckpoints},
		{"openai cancel fine tuning job", "/v1/fine_tuning/jobs/jobid/cancel", provider.ApiNameCancelFineTuningJob},
		{"openai resume fine tuning job", "/v1/fine_tuning/jobs/jobid/resume", provider.ApiNameResumeFineTuningJob},
		{"openai pause fine tuning job", "/v1/fine_tuning/jobs/jobid/pause", provider.ApiNamePauseFineTuningJob},
		{"openai fine tuning checkpoint permissions", "/v1/fine_tuning/checkpoints/checkpointid/permissions", provider.ApiNameFineTuningCheckpointPermissions},
		{"openai delete fine tuning checkpoint permission", "/v1/fine_tuning/checkpoints/checkpointid/permissions/permissionid", provider.ApiNameDeleteFineTuningCheckpointPermission},
		{"openai responses", "/v1/responses", provider.ApiNameResponses},
		// Anthropic
		{"anthropic messages", "/v1/messages", provider.ApiNameAnthropicMessages},
		{"anthropic complete", "/v1/complete", provider.ApiNameAnthropicComplete},
		// Gemini
		{"gemini generate content", "/v1beta/models/gemini-1.0-pro:generateContent", provider.ApiNameGeminiGenerateContent},
		{"gemini stream generate content", "/v1beta/models/gemini-1.0-pro:streamGenerateContent", provider.ApiNameGeminiStreamGenerateContent},
		// Cohere
		{"cohere rerank", "/v1/rerank", provider.ApiNameCohereV1Rerank},
		// Unknown
		{"unknown", "/v1/unknown", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getApiName(tt.path)
			if got != tt.want {
				t.Errorf("getApiName(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func Test_isSupportedRequestContentType(t *testing.T) {
	tests := []struct {
		name        string
		apiName     provider.ApiName
		contentType string
		want        bool
	}{
		{
			name:        "json chat completion",
			apiName:     provider.ApiNameChatCompletion,
			contentType: "application/json",
			want:        true,
		},
		{
			name:        "multipart image edit",
			apiName:     provider.ApiNameImageEdit,
			contentType: "multipart/form-data; boundary=----boundary",
			want:        true,
		},
		{
			name:        "multipart image variation",
			apiName:     provider.ApiNameImageVariation,
			contentType: "multipart/form-data; boundary=----boundary",
			want:        true,
		},
		{
			name:        "multipart chat completion",
			apiName:     provider.ApiNameChatCompletion,
			contentType: "multipart/form-data; boundary=----boundary",
			want:        false,
		},
		{
			name:        "text plain image edit",
			apiName:     provider.ApiNameImageEdit,
			contentType: "text/plain",
			want:        false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSupportedRequestContentType(tt.apiName, tt.contentType)
			if got != tt.want {
				t.Errorf("isSupportedRequestContentType(%v, %q) = %v, want %v", tt.apiName, tt.contentType, got, tt.want)
			}
		})
	}
}

func TestAi360(t *testing.T) {
	test.RunAi360ParseConfigTests(t)
	test.RunAi360OnHttpRequestHeadersTests(t)
	test.RunAi360OnHttpRequestBodyTests(t)
	test.RunAi360OnHttpResponseHeadersTests(t)
	test.RunAi360OnHttpResponseBodyTests(t)
	test.RunAi360OnStreamingResponseBodyTests(t)
}

func TestOpenAI(t *testing.T) {
	test.RunOpenAIParseConfigTests(t)
	test.RunOpenAIOnHttpRequestHeadersTests(t)
	test.RunOpenAIOnHttpRequestBodyTests(t)
	test.RunOpenAIOnHttpResponseHeadersTests(t)
	test.RunOpenAIOnHttpResponseBodyTests(t)
	test.RunOpenAIOnStreamingResponseBodyTests(t)
	test.RunOpenAIPromoteThinkingOnEmptyTests(t)
	test.RunOpenAIPromoteThinkingOnEmptyStreamingTests(t)
}

func TestQwen(t *testing.T) {
	test.RunQwenParseConfigTests(t)
	test.RunQwenOnHttpRequestHeadersTests(t)
	test.RunQwenOnHttpRequestBodyTests(t)
	test.RunQwenOnHttpResponseHeadersTests(t)
	test.RunQwenOnHttpResponseBodyTests(t)
	test.RunQwenOnStreamingResponseBodyTests(t)
}

func TestGemini(t *testing.T) {
	test.RunGeminiParseConfigTests(t)
	test.RunGeminiOnHttpRequestHeadersTests(t)
	test.RunGeminiOnHttpRequestBodyTests(t)
	test.RunGeminiOnHttpResponseHeadersTests(t)
	test.RunGeminiOnHttpResponseBodyTests(t)
	test.RunGeminiOnStreamingResponseBodyTests(t)
	test.RunGeminiGetImageURLTests(t)
}

func TestAzure(t *testing.T) {
	test.RunAzureParseConfigTests(t)
	test.RunAzureMultipartHelperTests(t)
	test.RunAzureOnHttpRequestHeadersTests(t)
	test.RunAzureOnHttpRequestBodyTests(t)
	test.RunAzureOnHttpResponseHeadersTests(t)
	test.RunAzureOnHttpResponseBodyTests(t)
	test.RunAzureBasePathHandlingTests(t)
}

func TestFireworks(t *testing.T) {
	test.RunFireworksParseConfigTests(t)
	test.RunFireworksOnHttpRequestHeadersTests(t)
	test.RunFireworksOnHttpRequestBodyTests(t)
}

func TestMinimax(t *testing.T) {
	test.RunMinimaxBasePathHandlingTests(t)
}

func TestUtil(t *testing.T) {
	test.RunMapRequestPathByCapabilityTests(t)
}

func TestApiPathRegression(t *testing.T) {
	test.RunApiPathRegressionTests(t)
}

func TestGeneric(t *testing.T) {
	test.RunGenericParseConfigTests(t)
	test.RunGenericOnHttpRequestHeadersTests(t)
	test.RunGenericOnHttpRequestBodyTests(t)
}

func TestVertex(t *testing.T) {
	test.RunVertexParseConfigTests(t)
	test.RunVertexExpressModeOnHttpRequestHeadersTests(t)
	test.RunVertexExpressModeOnHttpRequestBodyTests(t)
	test.RunVertexExpressModeOnHttpResponseBodyTests(t)
	test.RunVertexExpressModeOnStreamingResponseBodyTests(t)
	test.RunVertexOpenAICompatibleModeOnHttpRequestHeadersTests(t)
	test.RunVertexOpenAICompatibleModeOnHttpRequestBodyTests(t)
	test.RunVertexExpressModeImageGenerationRequestBodyTests(t)
	test.RunVertexExpressModeImageGenerationResponseBodyTests(t)
	test.RunVertexExpressModeImageEditVariationRequestBodyTests(t)
	test.RunVertexExpressModeImageEditVariationResponseBodyTests(t)
	// Vertex Raw 模式测试
	test.RunVertexRawModeOnHttpRequestHeadersTests(t)
	test.RunVertexRawModeOnHttpRequestBodyTests(t)
	test.RunVertexRawModeOnHttpResponseBodyTests(t)
}

func TestBedrock(t *testing.T) {
	test.RunBedrockParseConfigTests(t)
	test.RunBedrockOnHttpRequestHeadersTests(t)
	test.RunBedrockOnHttpRequestBodyTests(t)
	test.RunBedrockOnHttpResponseHeadersTests(t)
	test.RunBedrockOnHttpResponseBodyTests(t)
	test.RunBedrockOnStreamingResponseBodyTests(t)
	test.RunBedrockToolCallTests(t)
}

func TestClaude(t *testing.T) {
	test.RunClaudeParseConfigTests(t)
	test.RunClaudeOnHttpRequestHeadersTests(t)
	test.RunClaudeOnHttpRequestBodyTests(t)
}

func TestConsumerAffinity(t *testing.T) {
	test.RunConsumerAffinityParseConfigTests(t)
	test.RunConsumerAffinityOnHttpRequestHeadersTests(t)
}

func TestOpenRouter(t *testing.T) {
	test.RunOpenRouterClaudeAutoConversionTests(t)
}

func TestZhipuAI(t *testing.T) {
	test.RunZhipuAIClaudeAutoConversionTests(t)
}

func TestCooldown(t *testing.T) {
	test.RunCooldownParseConfigTests(t)
	test.RunCooldownOnHttpResponseHeadersTests(t)
	test.RunCooldownRecoveryTests(t)
}
