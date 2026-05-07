package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMatchStatus(t *testing.T) {
	defaultRetryPatterns := []string{"4.*", "5.*"}
	tests := []struct {
		name     string
		status   string
		patterns []string
		want     bool
	}{
		{"200_no_match", "200", defaultRetryPatterns, false},
		{"201_no_match", "201", defaultRetryPatterns, false},
		{"429_matches_4xx", "429", defaultRetryPatterns, true},
		{"400_matches_4xx", "400", defaultRetryPatterns, true},
		{"503_matches_5xx", "503", defaultRetryPatterns, true},
		{"500_matches_5xx", "500", defaultRetryPatterns, true},
		{"exact_503_pattern", "503", []string{"503"}, true},
		{"exact_503_miss", "502", []string{"503"}, false},
		{"empty_patterns", "500", []string{}, false},
		{"empty_status", "", defaultRetryPatterns, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchStatus(tt.status, tt.patterns)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDecodeUnicodeEscapes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Chinese characters",
			input:    `\u4e2d\u6587\u6d4b\u8bd5`,
			expected: `中文测试`,
		},
		{
			name:     "Mixed content",
			input:    `Hello \u4e16\u754c World`,
			expected: `Hello 世界 World`,
		},
		{
			name:     "No escape sequences",
			input:    `Hello World`,
			expected: `Hello World`,
		},
		{
			name:     "JSON with Unicode escapes",
			input:    `{"content":"\u76c8\u5229\u80fd\u529b"}`,
			expected: `{"content":"盈利能力"}`,
		},
		{
			name:     "Full width parentheses",
			input:    `\uff08\u76c8\u5229\uff09`,
			expected: `（盈利）`,
		},
		{
			name:     "Empty string",
			input:    ``,
			expected: ``,
		},
		{
			name:     "Invalid escape sequence (not modified)",
			input:    `\u00GG`,
			expected: `\u00GG`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DecodeUnicodeEscapes([]byte(tt.input))
			assert.Equal(t, tt.expected, string(result))
		})
	}
}

func TestDecodeUnicodeEscapesInSSE(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "SSE data with Unicode escapes",
			input: `data: {"choices":[{"delta":{"content":"\u4e2d\u6587"}}]}

`,
			expected: `data: {"choices":[{"delta":{"content":"中文"}}]}

`,
		},
		{
			name: "Multiple SSE data lines",
			input: `data: {"content":"\u4e2d\u6587"}
data: {"content":"\u82f1\u6587"}
data: [DONE]
`,
			expected: `data: {"content":"中文"}
data: {"content":"英文"}
data: [DONE]
`,
		},
		{
			name:     "Non-data lines unchanged",
			input:    ": comment\nevent: message\ndata: test\n",
			expected: ": comment\nevent: message\ndata: test\n",
		},
		{
			name: "Real Vertex AI response format",
			input: `data: {"choices":[{"delta":{"content":"\uff08\u76c8\u5229\u80fd\u529b\uff09","role":"assistant"},"index":0}],"created":1768307454,"id":"test","model":"gemini","object":"chat.completion.chunk"}

`,
			expected: `data: {"choices":[{"delta":{"content":"（盈利能力）","role":"assistant"},"index":0}],"created":1768307454,"id":"test","model":"gemini","object":"chat.completion.chunk"}

`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DecodeUnicodeEscapesInSSE([]byte(tt.input))
			assert.Equal(t, tt.expected, string(result))
		})
	}
}
