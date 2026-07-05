package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ==================== chatToolMessage2BedrockToolResultContent Tests ====================

// TestBedrockToolResultWithDocumentContent tests that tool results with document content blocks
// are correctly converted to Bedrock format
func TestBedrockToolResultWithDocumentContent(t *testing.T) {
	chatMsg := chatMessage{
		Role:       roleTool,
		ToolCallId: "call_doc_001",
		Content: []any{
			map[string]any{
				"type":   "document",
				"format": "pdf",
				"name":   "report.pdf",
				"source": map[string]any{
					"bytes": "base64encodedcontent",
				},
			},
		},
	}
	result := chatToolMessage2BedrockToolResultContent(chatMsg)

	require.NotNil(t, result.ToolResult)
	assert.Equal(t, "call_doc_001", result.ToolResult.ToolUseId)
	require.Len(t, result.ToolResult.Content, 1)
	require.NotNil(t, result.ToolResult.Content[0].Document)
	assert.Equal(t, "pdf", result.ToolResult.Content[0].Document.Format)
	assert.Equal(t, "report.pdf", result.ToolResult.Content[0].Document.Name)
	assert.Equal(t, "base64encodedcontent", result.ToolResult.Content[0].Document.Source.Bytes)
}

// TestBedrockToolResultWithImageContent tests that tool results with image content
// are correctly converted to Bedrock format
func TestBedrockToolResultWithImageContent(t *testing.T) {
	chatMsg := chatMessage{
		Role:       roleTool,
		ToolCallId: "call_img_001",
		Content: []any{
			map[string]any{
				"type": "image_url",
				"image_url": map[string]any{
					"url": "data:image/png;base64,iVBORw0KGgo=",
				},
			},
		},
	}
	result := chatToolMessage2BedrockToolResultContent(chatMsg)

	require.NotNil(t, result.ToolResult)
	assert.Equal(t, "call_img_001", result.ToolResult.ToolUseId)
	require.Len(t, result.ToolResult.Content, 1)
	require.NotNil(t, result.ToolResult.Content[0].Image)
	assert.Equal(t, "png", result.ToolResult.Content[0].Image.Format)
	assert.Equal(t, "iVBORw0KGgo=", result.ToolResult.Content[0].Image.Source.Bytes)
}

// TestBedrockToolResultWithErrorStatus tests that tool results with error status
// are correctly converted to Bedrock format with status="error"
func TestBedrockToolResultWithErrorStatus(t *testing.T) {
	chatMsg := chatMessage{
		Role:              roleTool,
		ToolCallId:        "call_err_001",
		IsToolResultError: true,
		Content:           "Error: function execution failed",
	}
	result := chatToolMessage2BedrockToolResultContent(chatMsg)

	require.NotNil(t, result.ToolResult)
	assert.Equal(t, "call_err_001", result.ToolResult.ToolUseId)
	assert.Equal(t, "error", result.ToolResult.Status)
	require.Len(t, result.ToolResult.Content, 1)
	require.NotNil(t, result.ToolResult.Content[0].Text)
	assert.Equal(t, "Error: function execution failed", *result.ToolResult.Content[0].Text)
}

// TestBedrockToolResultWithMixedContent tests tool results with mixed content types
func TestBedrockToolResultWithMixedContent(t *testing.T) {
	textContent := "Analysis result"
	chatMsg := chatMessage{
		Role:       roleTool,
		ToolCallId: "call_mixed_001",
		Content: []any{
			map[string]any{
				"type": "text",
				"text": textContent,
			},
			map[string]any{
				"type":   "document",
				"format": "csv",
				"name":   "data.csv",
				"source": map[string]any{
					"bytes": "csvbase64content",
				},
			},
		},
	}
	result := chatToolMessage2BedrockToolResultContent(chatMsg)

	require.NotNil(t, result.ToolResult)
	assert.Equal(t, "call_mixed_001", result.ToolResult.ToolUseId)
	require.Len(t, result.ToolResult.Content, 2)
	// First content: text
	require.NotNil(t, result.ToolResult.Content[0].Text)
	assert.Equal(t, textContent, *result.ToolResult.Content[0].Text)
	// Second content: document
	require.NotNil(t, result.ToolResult.Content[1].Document)
	assert.Equal(t, "csv", result.ToolResult.Content[1].Document.Format)
	assert.Equal(t, "data.csv", result.ToolResult.Content[1].Document.Name)
}

// TestBedrockToolResultWithoutErrorStatus tests that normal tool results don't have error status
func TestBedrockToolResultWithoutErrorStatus(t *testing.T) {
	chatMsg := chatMessage{
		Role:       roleTool,
		ToolCallId: "call_ok_001",
		Content:    "Success: function executed correctly",
	}
	result := chatToolMessage2BedrockToolResultContent(chatMsg)

	require.NotNil(t, result.ToolResult)
	assert.Equal(t, "call_ok_001", result.ToolResult.ToolUseId)
	// Status should be empty for non-error results
	assert.Equal(t, "", result.ToolResult.Status)
}

// TestBedrockToolResultWithTextOnly tests tool results with plain text content
func TestBedrockToolResultWithTextOnly(t *testing.T) {
	chatMsg := chatMessage{
		Role:       roleTool,
		ToolCallId: "call_text_001",
		Content:    "Plain text result",
	}
	result := chatToolMessage2BedrockToolResultContent(chatMsg)

	require.NotNil(t, result.ToolResult)
	assert.Equal(t, "call_text_001", result.ToolResult.ToolUseId)
	require.Len(t, result.ToolResult.Content, 1)
	require.NotNil(t, result.ToolResult.Content[0].Text)
	assert.Equal(t, "Plain text result", *result.ToolResult.Content[0].Text)
}

// TestBedrockToolResultWithTextContentArray tests tool results with text content in array format
func TestBedrockToolResultWithTextContentArray(t *testing.T) {
	chatMsg := chatMessage{
		Role:       roleTool,
		ToolCallId: "call_textarr_001",
		Content: []any{
			map[string]any{
				"type": "text",
				"text": "First part",
			},
			map[string]any{
				"type": "text",
				"text": "Second part",
			},
		},
	}
	result := chatToolMessage2BedrockToolResultContent(chatMsg)

	require.NotNil(t, result.ToolResult)
	assert.Equal(t, "call_textarr_001", result.ToolResult.ToolUseId)
	require.Len(t, result.ToolResult.Content, 2)
	require.NotNil(t, result.ToolResult.Content[0].Text)
	assert.Equal(t, "First part", *result.ToolResult.Content[0].Text)
	require.NotNil(t, result.ToolResult.Content[1].Text)
	assert.Equal(t, "Second part", *result.ToolResult.Content[1].Text)
}

// ==================== buildDocumentBlockFromContent Tests ====================

// TestBuildDocumentBlockFromContent tests the document block builder
func TestBuildDocumentBlockFromContent(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		wantNil  bool
		format   string
		docName  string
		bytesVal string
	}{
		{
			name: "valid document content",
			input: map[string]any{
				"format": "pdf",
				"name":   "test.pdf",
				"source": map[string]any{
					"bytes": "base64data",
				},
			},
			wantNil:  false,
			format:   "pdf",
			docName:  "test.pdf",
			bytesVal: "base64data",
		},
		{
			name: "valid csv document",
			input: map[string]any{
				"format": "csv",
				"name":   "data.csv",
				"source": map[string]any{
					"bytes": "csvdata",
				},
			},
			wantNil:  false,
			format:   "csv",
			docName:  "data.csv",
			bytesVal: "csvdata",
		},
		{
			name: "missing format",
			input: map[string]any{
				"name":   "test.pdf",
				"source": map[string]any{"bytes": "data"},
			},
			wantNil: true,
		},
		{
			name: "missing name",
			input: map[string]any{
				"format": "pdf",
				"source": map[string]any{"bytes": "data"},
			},
			wantNil: true,
		},
		{
			name: "missing source",
			input: map[string]any{
				"format": "pdf",
				"name":   "test.pdf",
			},
			wantNil: true,
		},
		{
			name: "missing bytes in source",
			input: map[string]any{
				"format": "pdf",
				"name":   "test.pdf",
				"source": map[string]any{},
			},
			wantNil:  false, // source exists, bytes defaults to ""
			format:   "pdf",
			docName:  "test.pdf",
			bytesVal: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildDocumentBlockFromContent(tt.input)
			if tt.wantNil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, tt.format, result.Format)
				assert.Equal(t, tt.docName, result.Name)
				assert.Equal(t, tt.bytesVal, result.Source.Bytes)
			}
		})
	}
}

// ==================== extractCanonicalQueryString Tests ====================

// TestExtractCanonicalQueryString tests the canonical query string extraction for SigV4
func TestExtractCanonicalQueryString(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "path without query string",
			path:     "/model/claude-3/converse",
			expected: "",
		},
		{
			name:     "path with empty query string",
			path:     "/model/claude-3/converse?",
			expected: "",
		},
		{
			name:     "path with single query parameter",
			path:     "/model/claude-3/converse?trace=1",
			expected: "trace=1",
		},
		{
			name:     "path with multiple query parameters (sorted)",
			path:     "/model/claude-3/converse?foo=bar&abc=123",
			expected: "abc=123&foo=bar",
		},
		{
			name:     "path with already sorted query parameters",
			path:     "/model/claude-3/converse?a=1&b=2&c=3",
			expected: "a=1&b=2&c=3",
		},
		{
			name:     "path with reverse sorted query parameters",
			path:     "/model/claude-3/converse?z=1&m=2&a=3",
			expected: "a=3&m=2&z=1",
		},
		{
			name:     "root path without query",
			path:     "/",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractCanonicalQueryString(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ==================== extractImageType Tests ====================

// TestExtractImageType tests the image type extraction from base64 data URL
func TestExtractImageType(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantErr   bool
		prefix    string
		imageType string
	}{
		{
			name:      "valid PNG data URL",
			input:     "data:image/png;base64,iVBORw0KGgo=",
			wantErr:   false,
			prefix:    "data:image/png;base64,",
			imageType: "png",
		},
		{
			name:      "valid JPEG data URL",
			input:     "data:image/jpeg;base64,/9j/4AAQ",
			wantErr:   false,
			prefix:    "data:image/jpeg;base64,",
			imageType: "jpeg",
		},
		{
			name:      "valid GIF data URL",
			input:     "data:image/gif;base64,R0lGODlh",
			wantErr:   false,
			prefix:    "data:image/gif;base64,",
			imageType: "gif",
		},
		{
			name:      "valid WEBP data URL",
			input:     "data:image/webp;base64,UklGR",
			wantErr:   false,
			prefix:    "data:image/webp;base64,",
			imageType: "webp",
		},
		{
			name:    "invalid format - no data prefix",
			input:   "iVBORw0KGgo=",
			wantErr: true,
		},
		{
			name:    "invalid format - no base64 marker",
			input:   "data:image/png,iVBORw0KGgo=",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prefix, imageType, err := extractImageType(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.prefix, prefix)
				assert.Equal(t, tt.imageType, imageType)
			}
		})
	}
}

// Note: TestEncodeSigV4Path is already defined in bedrock_sigv4_path_test.go
// with more comprehensive test cases including ARN paths and pre-encoded paths.

// ==================== SigV4 Signing Tests ====================

// TestGetSignatureKey tests the AWS SigV4 signature key derivation
func TestGetSignatureKey(t *testing.T) {
	key := getSignatureKey("secret", "20240101", "us-east-1", "bedrock")
	assert.NotEmpty(t, key)
	assert.Len(t, key, 32) // SHA-256 HMAC output is 32 bytes
}

// TestHmacSha256 tests the HMAC-SHA256 function
func TestHmacSha256(t *testing.T) {
	key := []byte("test-key")
	data := "test-data"
	result := hmacSha256(key, data)
	assert.Len(t, result, 32) // SHA-256 output is 32 bytes
}

// TestSha256Hex tests the SHA-256 hex encoding function
func TestSha256Hex(t *testing.T) {
	data := []byte("test-data")
	result := sha256Hex(data)
	assert.NotEmpty(t, result)
	// SHA-256 hex output should be 64 characters (32 bytes * 2)
	assert.Len(t, result, 64)
}

// TestHmacHex tests the HMAC hex encoding function
func TestHmacHex(t *testing.T) {
	key := []byte("test-key")
	data := "test-data"
	result := hmacHex(key, data)
	assert.NotEmpty(t, result)
	// SHA-256 hex output should be 64 characters
	assert.Len(t, result, 64)
}

// ==================== bedrockAWSService Tests ====================

// TestBedrockAWSService tests the AWS service name determination
func TestBedrockAWSService(t *testing.T) {
	assert.Equal(t, awsServiceBedrock, bedrockAWSService(ApiNameChatCompletion))
	assert.Equal(t, awsServiceBedrockMantle, bedrockAWSService(ApiNameAnthropicMessages))
}

// ==================== bedrockAWSEndpoint Tests ====================

// TestBedrockAWSEndpoint tests the AWS endpoint URL generation
func TestBedrockAWSEndpoint(t *testing.T) {
	bedrockEndpoint := bedrockAWSEndpoint(awsServiceBedrock, "us-east-1")
	assert.Equal(t, "bedrock-runtime.us-east-1.amazonaws.com", bedrockEndpoint)

	mantleEndpoint := bedrockAWSEndpoint(awsServiceBedrockMantle, "eu-west-1")
	assert.Equal(t, "bedrock-mantle.eu-west-1.api.aws", mantleEndpoint)
}

// ==================== normalizeBedrockCachePointPosition Tests ====================

// TestNormalizeBedrockCachePointPosition tests the cache point position normalization
func TestNormalizeBedrockCachePointPosition(t *testing.T) {
	assert.Equal(t, bedrockCachePointPositionSystemPrompt, normalizeBedrockCachePointPosition("systemPrompt"))
	assert.Equal(t, bedrockCachePointPositionLastUserMessage, normalizeBedrockCachePointPosition("lastUserMessage"))
	assert.Equal(t, bedrockCachePointPositionLastMessage, normalizeBedrockCachePointPosition("lastMessage"))
	assert.Equal(t, bedrockCachePointPositionSystemPrompt, normalizeBedrockCachePointPosition("SYSTEM_PROMPT"))
	assert.Equal(t, bedrockCachePointPositionLastUserMessage, normalizeBedrockCachePointPosition("LASTUSERMESSAGE"))
}

// Note: TestIsPromptCacheSupportedModel and TestNormalizePromptCacheRetention
// are already defined in bedrock_sigv4_path_test.go with comprehensive test cases.