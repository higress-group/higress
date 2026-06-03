package provider

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/sjson"
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

func TestBedrockAnthropicMessagesEndpointDetection(t *testing.T) {
	tests := []struct {
		name           string
		endpoint       string
		providerDomain string
		capabilities   map[string]string
		want           string
	}{
		{
			name:     "explicit runtime endpoint selects invoke",
			endpoint: "runtime",
			capabilities: map[string]string{
				string(ApiNameAnthropicMessages): bedrockMantleMessagesPath,
			},
			want: bedrockAnthropicMessagesEndpointInvoke,
		},
		{
			name:     "explicit invoke endpoint alias selects invoke",
			endpoint: "invoke",
			capabilities: map[string]string{
				string(ApiNameAnthropicMessages): bedrockMantleMessagesPath,
			},
			want: bedrockAnthropicMessagesEndpointInvoke,
		},
		{
			name:     "explicit mantle endpoint selects mantle",
			endpoint: "mantle",
			capabilities: map[string]string{
				string(ApiNameAnthropicMessages): bedrockInvokeModelPath,
			},
			want: bedrockAnthropicMessagesEndpointMantle,
		},
		{
			name:           "provider domain mantle wins over default invoke capability",
			providerDomain: "bedrock-mantle.us-east-1.api.aws",
			capabilities: map[string]string{
				string(ApiNameAnthropicMessages): bedrockInvokeModelPath,
			},
			want: bedrockAnthropicMessagesEndpointMantle,
		},
		{
			name:           "provider domain runtime selects invoke",
			providerDomain: "bedrock-runtime.us-east-1.amazonaws.com",
			capabilities: map[string]string{
				string(ApiNameAnthropicMessages): bedrockMantleMessagesPath,
			},
			want: bedrockAnthropicMessagesEndpointInvoke,
		},
		{
			name:           "provider domain runtime with scheme port and path selects invoke",
			providerDomain: "https://bedrock-runtime.us-east-1.amazonaws.com:443/proxy",
			capabilities: map[string]string{
				string(ApiNameAnthropicMessages): bedrockMantleMessagesPath,
			},
			want: bedrockAnthropicMessagesEndpointInvoke,
		},
		{
			name:           "proxy domain containing runtime falls back to capability",
			providerDomain: "bedrock-runtime-proxy.internal.example.com",
			capabilities: map[string]string{
				string(ApiNameAnthropicMessages): bedrockMantleMessagesPath,
			},
			want: bedrockAnthropicMessagesEndpointMantle,
		},
		{
			name:           "proxy domain containing mantle falls back to invoke capability",
			providerDomain: "bedrock-mantle-proxy.internal.example.com",
			capabilities: map[string]string{
				string(ApiNameAnthropicMessages): bedrockInvokeModelPath,
			},
			want: bedrockAnthropicMessagesEndpointInvoke,
		},
		{
			name: "mantle capability selects mantle",
			capabilities: map[string]string{
				string(ApiNameAnthropicMessages): bedrockMantleMessagesPath,
			},
			want: bedrockAnthropicMessagesEndpointMantle,
		},
		{
			name: "default mantle capability selects mantle",
			capabilities: map[string]string{
				string(ApiNameAnthropicMessages): bedrockMantleMessagesPath,
			},
			want: bedrockAnthropicMessagesEndpointMantle,
		},
		{
			name:         "empty capability selects mantle",
			capabilities: map[string]string{},
			want:         bedrockAnthropicMessagesEndpointMantle,
		},
		{
			name: "invoke capability selects invoke",
			capabilities: map[string]string{
				string(ApiNameAnthropicMessages): bedrockInvokeModelPath,
			},
			want: bedrockAnthropicMessagesEndpointInvoke,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &bedrockProvider{
				config: ProviderConfig{
					bedrockAnthropicMessagesEndpoint: tt.endpoint,
					providerDomain:                   tt.providerDomain,
					capabilities:                     tt.capabilities,
				},
			}
			assert.Equal(t, tt.want, p.anthropicMessagesEndpoint())
		})
	}
}

func TestBedrockAnthropicMessagesInvokeProviderBasePathIsSigned(t *testing.T) {
	p := &bedrockProvider{
		config: ProviderConfig{
			awsAccessKey:     "test-ak-for-unit-test",
			awsSecretKey:     "test-sk-for-unit-test",
			awsRegion:        "us-east-1",
			providerBasePath: "/bedrock-proxy",
			capabilities: map[string]string{
				string(ApiNameAnthropicMessages): bedrockInvokeModelPath,
			},
			modelMapping: map[string]string{
				"*": "anthropic.claude-3-5-haiku-20241022-v1:0",
			},
		},
	}
	ctx := newMockMultipartHttpContext()
	headers := http.Header{}
	headers.Set(":path", "/v1/messages")
	headers.Set(":authority", "example.com")

	p.TransformRequestHeaders(ctx, ApiNameAnthropicMessages, headers)
	transformedBody, err := p.TransformRequestBodyHeaders(ctx, ApiNameAnthropicMessages, []byte(`{
		"model": "claude-request-model",
		"max_tokens": 100,
		"messages": [{"role": "user", "content": "hello"}]
	}`), headers)
	require.NoError(t, err)

	path := headers.Get(":path")
	assert.Equal(t, "/bedrock-proxy/model/anthropic.claude-3-5-haiku-20241022-v1%3A0/invoke", path)
	amzDate := headers.Get("X-Amz-Date")
	require.Len(t, amzDate, len("20060102T150405Z"))
	expectedSignature := p.generateSignatureWithService(path, amzDate, amzDate[:8], transformedBody, awsServiceBedrock)
	assert.Contains(t, headers.Get("Authorization"), "Signature="+expectedSignature)
}

func TestBedrockAnthropicMessagesInvalidExplicitEndpoint(t *testing.T) {
	err := (&bedrockProviderInitializer{}).ValidateConfig(&ProviderConfig{
		apiTokens:                        []string{"test-token-for-unit-test"},
		awsRegion:                        "us-east-1",
		bedrockAnthropicMessagesEndpoint: "bad-endpoint",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid bedrockAnthropicMessagesEndpoint")
}

func TestBedrockAnthropicMessagesInvokeProviderDomainHostIsSigned(t *testing.T) {
	providerDomain := normalizeProviderDomainHost("https://bedrock-runtime.us-west-2.amazonaws.com:443/proxy")
	p := &bedrockProvider{
		config: ProviderConfig{
			awsAccessKey:   "test-ak-for-unit-test",
			awsSecretKey:   "test-sk-for-unit-test",
			awsRegion:      "us-west-2",
			providerDomain: providerDomain,
			capabilities: map[string]string{
				string(ApiNameAnthropicMessages): bedrockInvokeModelPath,
			},
			modelMapping: map[string]string{
				"*": "anthropic.claude-3-5-haiku-20241022-v1:0",
			},
		},
	}
	ctx := newMockMultipartHttpContext()
	headers := http.Header{}
	headers.Set(":path", "/v1/messages")
	headers.Set(":authority", "example.com")

	p.TransformRequestHeaders(ctx, ApiNameAnthropicMessages, headers)
	headers.Set(":authority", providerDomain)
	transformedBody, err := p.TransformRequestBodyHeaders(ctx, ApiNameAnthropicMessages, []byte(`{
		"model": "claude-request-model",
		"max_tokens": 100,
		"messages": [{"role": "user", "content": "hello"}]
	}`), headers)
	require.NoError(t, err)

	assert.Equal(t, "bedrock-runtime.us-west-2.amazonaws.com:443", headers.Get(":authority"))
	amzDate := headers.Get("X-Amz-Date")
	require.Len(t, amzDate, len("20060102T150405Z"))
	expectedSignature := p.generateSignatureWithServiceAndHost(headers.Get(":path"), amzDate, amzDate[:8], transformedBody, awsServiceBedrock, providerDomain)
	assert.Contains(t, headers.Get("Authorization"), "Signature="+expectedSignature)
}

func TestBedrockAnthropicMessagesAutoBetas(t *testing.T) {
	body := []byte(`{
		"messages": [{
			"role": "user",
			"content": [{
				"type": "document",
				"source": {"type": "file", "file_id": "file_123"}
			}]
		}],
		"tools": [
			{"type": "computer_20241022"},
			{"type": "tool_search_tool_regex_20251119"}
		],
		"mcp_servers": [{"type": "url", "url": "https://mcp.example.com", "name": "mcp"}]
	}`)

	betas := bedrockAnthropicMessagesAutoBetas(body, "anthropic.claude-opus-4-20250514-v1:0")
	assert.ElementsMatch(t, []string{
		bedrockComputerUseBeta20241022,
		bedrockToolSearchBeta20251019,
		bedrockFilesAPIBeta,
		bedrockCodeExecutionBeta,
		bedrockMCPClientBeta,
	}, betas)
	assert.Contains(t, betas, "tool-search-tool-2025-10-19")

	assert.Empty(t, bedrockAnthropicMessagesAutoBetas(body, "amazon.nova-pro-v1:0"))
}

func TestBedrockAnthropicMessagesAutoBetasToolSearchClaudeModels(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		toolType string
		wantBeta bool
	}{
		{
			name:     "bedrock opus 4 model",
			model:    "anthropic.claude-opus-4-20250514-v1:0",
			toolType: "tool_search_tool_regex",
			wantBeta: true,
		},
		{
			name:     "bedrock sonnet model",
			model:    "anthropic.claude-sonnet-4-5-20250929-v1:0",
			toolType: "tool_search_tool_regex_20251119",
			wantBeta: true,
		},
		{
			name:     "bedrock haiku inference profile",
			model:    "global.anthropic.claude-haiku-4-5-20251001-v1:0",
			toolType: "tool_search_tool_bm25_20251119",
			wantBeta: true,
		},
		{
			name:     "future claude opus 40 model",
			model:    "anthropic.claude-opus-40-20270101-v1:0",
			toolType: "tool_search_tool_regex",
			wantBeta: true,
		},
		{
			name:     "future claude opus 4o model",
			model:    "anthropic.claude-opus-4o-20270101-v1:0",
			toolType: "tool_search_tool_regex",
			wantBeta: true,
		},
		{
			name:     "non claude model",
			model:    "amazon.nova-pro-v1:0",
			toolType: "tool_search_tool_regex",
			wantBeta: false,
		},
		{
			name:     "claude model without tool search tool",
			model:    "anthropic.claude-sonnet-4-5-20250929-v1:0",
			toolType: "custom_tool",
			wantBeta: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := sjson.SetBytes([]byte(`{"tools":[{}]}`), "tools.0.type", tt.toolType)
			require.NoError(t, err)
			betas := bedrockAnthropicMessagesAutoBetas(body, tt.model)
			if tt.wantBeta {
				assert.Contains(t, betas, bedrockToolSearchBeta20251019)
				return
			}
			assert.NotContains(t, betas, bedrockToolSearchBeta20251019)
		})
	}
}
