package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildPromptTokensDetailsPreservesCacheUsage(t *testing.T) {
	tests := []struct {
		name       string
		cacheRead  int
		cacheWrite int
		wantNil    bool
	}{
		{
			name:    "no cache usage",
			wantNil: true,
		},
		{
			name:      "cache read only",
			cacheRead: 7,
		},
		{
			name:       "cache write only",
			cacheWrite: 3,
		},
		{
			name:       "cache read and write",
			cacheRead:  7,
			cacheWrite: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			details := buildPromptTokensDetails(tt.cacheRead, tt.cacheWrite)
			if tt.wantNil {
				assert.Nil(t, details)
				return
			}
			require.NotNil(t, details)
			assert.Equal(t, tt.cacheRead, details.CachedTokens)
			assert.Equal(t, tt.cacheWrite, details.CacheWriteTokens)
		})
	}
}

func TestTokenUsageCacheTokensSupportsBedrockFieldAliases(t *testing.T) {
	tests := []struct {
		name      string
		usage     tokenUsage
		wantRead  int
		wantWrite int
	}{
		{
			name: "current fields",
			usage: tokenUsage{
				CacheReadInputTokens:  7,
				CacheWriteInputTokens: 3,
			},
			wantRead:  7,
			wantWrite: 3,
		},
		{
			name: "legacy count fields",
			usage: tokenUsage{
				CacheReadInputTokenCount:  11,
				CacheWriteInputTokenCount: 5,
			},
			wantRead:  11,
			wantWrite: 5,
		},
		{
			name: "current fields take precedence",
			usage: tokenUsage{
				CacheReadInputTokens:      7,
				CacheReadInputTokenCount:  70,
				CacheWriteInputTokens:     3,
				CacheWriteInputTokenCount: 30,
			},
			wantRead:  7,
			wantWrite: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantRead, tt.usage.cacheReadTokens())
			assert.Equal(t, tt.wantWrite, tt.usage.cacheWriteTokens())
		})
	}
}
