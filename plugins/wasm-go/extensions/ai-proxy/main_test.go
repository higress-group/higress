package main

import (
	"strings"
	"testing"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-proxy/provider"
	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-proxy/test"
	"github.com/tidwall/gjson"
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
		{"openai chat completions with path_prefix", "/gateway/proxy/v1/chat/completions", provider.ApiNameChatCompletion},
		{"openai chat completions_extra_path_not_suffix_match", "/v1/chat/completions/extra", ""},
		{"openai realtime_with_query_not_matched_as_suffix", "/v1/realtime?stream=1", ""},
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
		// Qwen
		{"qwen reranks", "/v1/reranks", provider.ApiNameQwenV1Rerank},
		{"qwen conversations", "/v1/conversations", provider.ApiNameQwenV1Conversations},
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
		{
			name:        "json_with_charset",
			apiName:     provider.ApiNameChatCompletion,
			contentType: "application/json; charset=utf-8",
			want:        true,
		},
		{
			name:        "multipart_uppercase_image_edit",
			apiName:     provider.ApiNameImageEdit,
			contentType: "MULTIPART/FORM-DATA; boundary=abc",
			want:        true,
		},
		{
			name:        "multipart_image_generation_not_allowed",
			apiName:     provider.ApiNameImageGeneration,
			contentType: "multipart/form-data; boundary=----boundary",
			want:        false,
		},
		{
			name:        "multipart_embeddings_not_allowed",
			apiName:     provider.ApiNameEmbeddings,
			contentType: "multipart/form-data; boundary=----boundary",
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

func Test_normalizeOpenAiRequestBody(t *testing.T) {
	t.Run("stream_adds_include_usage", func(t *testing.T) {
		in := []byte(`{"model":"x","stream":true}`)
		got := normalizeOpenAiRequestBody(in)
		if !gjson.GetBytes(got, "stream_options.include_usage").Bool() {
			t.Fatalf("want stream_options.include_usage true, got %s", string(got))
		}
	})
	t.Run("stream_false_no_stream_options", func(t *testing.T) {
		in := []byte(`{"model":"x","stream":false}`)
		got := normalizeOpenAiRequestBody(in)
		if gjson.GetBytes(got, "stream_options").Exists() {
			t.Fatalf("did not expect stream_options, got %s", string(got))
		}
	})
	t.Run("respect_explicit_include_usage_false", func(t *testing.T) {
		in := []byte(`{"model":"x","stream":true,"stream_options":{"include_usage":false}}`)
		got := normalizeOpenAiRequestBody(in)
		if gjson.GetBytes(got, "stream_options.include_usage").Bool() {
			t.Fatalf("want include_usage false, got %s", string(got))
		}
	})
	t.Run("stream_missing_no_stream_options", func(t *testing.T) {
		in := []byte(`{"model":"x"}`)
		got := normalizeOpenAiRequestBody(in)
		if gjson.GetBytes(got, "stream_options").Exists() {
			t.Fatalf("unexpected stream_options: %s", string(got))
		}
	})
	t.Run("stream_non_bool_treated_as_false", func(t *testing.T) {
		in := []byte(`{"model":"x","stream":"yes"}`)
		got := normalizeOpenAiRequestBody(in)
		if gjson.GetBytes(got, "stream_options").Exists() {
			t.Fatalf("unexpected stream_options for non-bool stream: %s", string(got))
		}
	})
}

func Test_convertResponseBodyToClaude_glue(t *testing.T) {
	ctx := test.NewMockHttpContext()
	openaiBody := []byte(`{"id":"id1","object":"chat.completion","created":1,"model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"hello"}}]}`)

	out, err := convertResponseBodyToClaude(ctx, openaiBody)
	if err != nil || string(out) != string(openaiBody) {
		t.Fatalf("without flag: err=%v out=%s", err, string(out))
	}
	// Full OpenAI→Claude conversion runs log.Debugf inside the provider and requires a Wasm host
	// when this package's init() has registered the plugin (see provider/claude_to_openai_test.go).
}

func Test_convertStreamingResponseToClaude_glue(t *testing.T) {
	chunk := []byte("data: {\"x\":1}\n\n")
	ctx := test.NewMockHttpContext()
	out, err := convertStreamingResponseToClaude(ctx, chunk)
	if err != nil || string(out) != string(chunk) {
		t.Fatalf("without conversion flag: err=%v out=%q", err, string(out))
	}
}

func Test_needsClaudeResponseConversion(t *testing.T) {
	ctx := test.NewMockHttpContext()
	if NeedsClaudeResponseConversionForTest(ctx) {
		t.Fatal("expected false without context flag")
	}
	ctx.SetContext("needClaudeResponseConversion", true)
	if !NeedsClaudeResponseConversionForTest(ctx) {
		t.Fatal("expected true when flag set")
	}
}

func Test_promoteThinkingInStreamingChunk(t *testing.T) {
	ctx := test.NewMockHttpContext()
	reasoningJSON := `{"choices":[{"index":0,"delta":{"reasoning_content":"only-thinking"}}]}`
	sse := "data: " + reasoningJSON + "\n"
	out := promoteThinkingInStreamingChunk(ctx, []byte(sse), true)
	if len(out) == 0 {
		t.Fatal("expected non-empty output")
	}
	// Last chunk should prepend flush SSE when no content delta was seen
	if !strings.HasPrefix(string(out), "data: ") {
		t.Fatalf("expected flush data line prepended, got prefix %q", string(out))
	}
	// Original line should still be present (possibly stripped reasoning)
	if !strings.Contains(string(out), "data:") {
		t.Fatalf("expected SSE data lines: %s", string(out))
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

func TestMainEdgeCases(t *testing.T) {
	test.RunMainEdgeCaseTests(t)
}

func TestApiPathRegression(t *testing.T) {
	test.RunApiPathRegressionTests(t)
}

func TestGeneric(t *testing.T) {
	test.RunGenericParseConfigTests(t)
	test.RunGenericOnHttpRequestHeadersTests(t)
	test.RunGenericOnHttpRequestBodyTests(t)
}

func TestKling(t *testing.T) {
	test.RunKlingParseConfigTests(t)
	test.RunKlingOnHttpRequestHeadersTests(t)
	test.RunKlingOnHttpRequestBodyTests(t)
	test.RunKlingOnHttpResponseBodyTests(t)
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

func TestDeepSeek(t *testing.T) {
	test.RunDeepSeekParseConfigTests(t)
	test.RunDeepSeekOnHttpRequestHeadersTests(t)
}

func TestDoubao(t *testing.T) {
	test.RunDoubaoParseConfigTests(t)
	test.RunDoubaoOnHttpRequestHeadersTests(t)
}

func TestGroq(t *testing.T) {
	test.RunGroqParseConfigTests(t)
	test.RunGroqOnHttpRequestHeadersTests(t)
}

func TestMistral(t *testing.T) {
	test.RunMistralParseConfigTests(t)
	test.RunMistralOnHttpRequestHeadersTests(t)
}

func TestMoonshot(t *testing.T) {
	test.RunMoonshotParseConfigTests(t)
	test.RunMoonshotOnHttpRequestHeadersTests(t)
}

func TestSpark(t *testing.T) {
	test.RunSparkParseConfigTests(t)
	test.RunSparkOnHttpRequestHeadersTests(t)
}

func TestTogetherAI(t *testing.T) {
	test.RunTogetherAIParseConfigTests(t)
	test.RunTogetherAIOnHttpRequestHeadersTests(t)
}

func TestGithub(t *testing.T) {
	test.RunGithubParseConfigTests(t)
	test.RunGithubOnHttpRequestHeadersTests(t)
}

func TestGrok(t *testing.T) {
	test.RunGrokParseConfigTests(t)
	test.RunGrokOnHttpRequestHeadersTests(t)
}

func TestProviderWasmSmoke(t *testing.T) {
	test.RunBaichuanWasmSmokeTests(t)
	test.RunYiWasmSmokeTests(t)
	test.RunOllamaWasmSmokeTests(t)
	test.RunBaiduWasmSmokeTests(t)
	test.RunHunyuanWasmSmokeTests(t)
	test.RunStepfunWasmSmokeTests(t)
	test.RunCloudflareWasmSmokeTests(t)
	test.RunDeeplWasmSmokeTests(t)
	test.RunCohereWasmSmokeTests(t)
	test.RunCozeWasmSmokeTests(t)
	test.RunDifyWasmSmokeTests(t)
	test.RunTritonWasmSmokeTests(t)
	test.RunVllmWasmSmokeTests(t)
}
