package provider

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEncodeSigV4Path(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "raw model id keeps colon",
			path: "/model/global.amazon.nova-2-lite-v1:0/converse-stream",
			want: "/model/global.amazon.nova-2-lite-v1:0/converse-stream",
		},
		{
			name: "pre-encoded model id escapes percent to avoid mismatch",
			path: "/model/global.amazon.nova-2-lite-v1%3A0/converse-stream",
			want: "/model/global.amazon.nova-2-lite-v1%253A0/converse-stream",
		},
		{
			name: "raw inference profile arn keeps colon and slash delimiters",
			path: "/model/arn:aws:bedrock:us-east-1:123456789012:inference-profile/global.anthropic.claude-sonnet-4-20250514-v1:0/converse",
			want: "/model/arn:aws:bedrock:us-east-1:123456789012:inference-profile/global.anthropic.claude-sonnet-4-20250514-v1:0/converse",
		},
		{
			name: "encoded inference profile arn preserves escaped slash as double-escaped percent",
			path: "/model/arn%3Aaws%3Abedrock%3Aus-east-1%3A123456789012%3Ainference-profile%2Fglobal.anthropic.claude-sonnet-4-20250514-v1%3A0/converse",
			want: "/model/arn%253Aaws%253Abedrock%253Aus-east-1%253A123456789012%253Ainference-profile%252Fglobal.anthropic.claude-sonnet-4-20250514-v1%253A0/converse",
		},
		{
			name: "query string is stripped before canonical encoding",
			path: "/model/global.amazon.nova-2-lite-v1%3A0/converse-stream?trace=1&foo=bar",
			want: "/model/global.amazon.nova-2-lite-v1%253A0/converse-stream",
		},
		{
			name: "invalid percent sequence falls back to escaped percent",
			path: "/model/abc%ZZxyz/converse",
			want: "/model/abc%25ZZxyz/converse",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, encodeSigV4Path(tt.path))
		})
	}
}

func TestOverwriteRequestPathHeaderPreservesSingleEncodedRequestPath(t *testing.T) {
	p := &bedrockProvider{}
	plainModel := "arn:aws:bedrock:us-east-1:123456789012:inference-profile/global.amazon.nova-2-lite-v1:0"
	preEncodedModel := url.QueryEscape(plainModel)

	t.Run("plain model is encoded once", func(t *testing.T) {
		headers := http.Header{}
		p.overwriteRequestPathHeader(headers, bedrockChatCompletionPath, plainModel)
		assert.Equal(t, "/model/arn%3Aaws%3Abedrock%3Aus-east-1%3A123456789012%3Ainference-profile%2Fglobal.amazon.nova-2-lite-v1%3A0/converse", headers.Get(":path"))
	})

	t.Run("pre-encoded model is not double encoded", func(t *testing.T) {
		headers := http.Header{}
		p.overwriteRequestPathHeader(headers, bedrockChatCompletionPath, preEncodedModel)
		assert.Equal(t, "/model/arn%3Aaws%3Abedrock%3Aus-east-1%3A123456789012%3Ainference-profile%2Fglobal.amazon.nova-2-lite-v1%3A0/converse", headers.Get(":path"))
	})
}

func TestGenerateSignatureIgnoresQueryStringInCanonicalURI(t *testing.T) {
	p := &bedrockProvider{
		config: ProviderConfig{
			awsRegion:    "ap-northeast-3",
			awsSecretKey: "test-secret",
		},
	}
	body := []byte(`{"messages":[{"role":"user","content":[{"text":"hello"}]}]}`)
	pathWithoutQuery := "/model/global.amazon.nova-2-lite-v1%3A0/converse-stream"
	pathWithQuery := pathWithoutQuery + "?trace=1&foo=bar"

	sigWithoutQuery := p.generateSignature(pathWithoutQuery, "20260312T142942Z", "20260312", body)
	sigWithQuery := p.generateSignature(pathWithQuery, "20260312T142942Z", "20260312", body)
	assert.Equal(t, sigWithoutQuery, sigWithQuery)
}

func TestGenerateSignatureDiffersForRawAndPreEncodedModelPath(t *testing.T) {
	p := &bedrockProvider{
		config: ProviderConfig{
			awsRegion:    "ap-northeast-3",
			awsSecretKey: "test-secret",
		},
	}
	body := []byte(`{"messages":[{"role":"user","content":[{"text":"hello"}]}]}`)
	rawPath := "/model/global.amazon.nova-2-lite-v1:0/converse-stream"
	preEncodedPath := "/model/global.amazon.nova-2-lite-v1%3A0/converse-stream"

	rawSignature := p.generateSignature(rawPath, "20260312T142942Z", "20260312", body)
	preEncodedSignature := p.generateSignature(preEncodedPath, "20260312T142942Z", "20260312", body)
	assert.NotEqual(t, rawSignature, preEncodedSignature)
}

func TestNormalizePromptCacheRetention(t *testing.T) {
	tests := []struct {
		name      string
		retention string
		want      string
	}{
		{
			name:      "inmemory alias maps to in_memory",
			retention: "inmemory",
			want:      "in_memory",
		},
		{
			name:      "dash style maps to in_memory",
			retention: "in-memory",
			want:      "in_memory",
		},
		{
			name:      "space style with trim maps to in_memory",
			retention: " in memory ",
			want:      "in_memory",
		},
		{
			name:      "already normalized remains unchanged",
			retention: "in_memory",
			want:      "in_memory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, normalizePromptCacheRetention(tt.retention))
		})
	}
}

func TestAppendCachePointToBedrockMessageInvalidIndexNoop(t *testing.T) {
	request := &bedrockTextGenRequest{
		Messages: []bedrockMessage{
			{
				Role: roleUser,
				Content: []bedrockMessageContent{
					{Text: "hello"},
				},
			},
		},
	}

	appendCachePointToBedrockMessage(request, -1, bedrockCacheTTL5m)
	appendCachePointToBedrockMessage(request, len(request.Messages), bedrockCacheTTL5m)

	assert.Len(t, request.Messages[0].Content, 1)

	appendCachePointToBedrockMessage(request, 0, bedrockCacheTTL5m)
	assert.Len(t, request.Messages[0].Content, 2)
	assert.NotNil(t, request.Messages[0].Content[1].CachePoint)
}

func TestIsPromptCacheSupportedModel(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  bool
	}{
		{
			name:  "anthropic claude model is supported",
			model: "anthropic.claude-3-5-haiku-20241022-v1:0",
			want:  true,
		},
		{
			name:  "amazon nova inference profile is supported",
			model: "arn:aws:bedrock:us-east-1:123456789012:inference-profile/global.amazon.nova-2-lite-v1:0",
			want:  true,
		},
		{
			name:  "other model is not supported",
			model: "meta.llama3-70b-instruct-v1:0",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isPromptCacheSupportedModel(tt.model))
		})
	}
}
