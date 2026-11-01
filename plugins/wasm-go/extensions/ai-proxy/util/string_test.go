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
