package expr

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStringMatcher(t *testing.T) {
	tests := []struct {
		name       string
		matchType  string
		target     string
		ignoreCase bool
		matched    []string
		mismatched []string
	}{
		{
			name:       "exact",
			matchType:  MatchPatternExact,
			target:     "foo",
			matched:    []string{"foo"},
			mismatched: []string{"fo", "fooo"},
		},
		{
			name:       "exact, ignore_case",
			matchType:  MatchPatternExact,
			target:     "foo",
			ignoreCase: true,
			matched:    []string{"Foo", "foo"},
		},
		{
			name:       "prefix",
			matchType:  MatchPatternPrefix,
			target:     "/p",
			matched:    []string{"/p", "/pa"},
			mismatched: []string{"/P"},
		},
		{
			name:       "prefix, ignore_case",
			matchType:  MatchPatternPrefix,
			target:     "/p",
			ignoreCase: true,
			matched:    []string{"/P", "/p", "/pa", "/Pa"},
			mismatched: []string{"/"},
		},
		{
			name:       "suffix",
			matchType:  MatchPatternSuffix,
			target:     "foo",
			matched:    []string{"foo", "0foo"},
			mismatched: []string{"fo", "fooo", "aFoo"},
		},
		{
			name:       "suffix, ignore_case",
			matchType:  MatchPatternSuffix,
			target:     "foo",
			ignoreCase: true,
			matched:    []string{"aFoo", "foo"},
			mismatched: []string{"fo", "fooo"},
		},
		{
			name:       "contains",
			matchType:  MatchPatternContains,
			target:     "foo",
			matched:    []string{"foo", "0foo", "fooo"},
			mismatched: []string{"fo", "aFoo"},
		},
		{
			name:       "contains, ignore_case",
			matchType:  MatchPatternContains,
			target:     "foo",
			ignoreCase: true,
			matched:    []string{"aFoo", "foo", "FoO"},
			mismatched: []string{"fo"},
		},
		{
			name:       "regex",
			matchType:  MatchPatternRegex,
			target:     "fo{2}",
			matched:    []string{"foo", "0foo", "fooo"},
			mismatched: []string{"aFoo", "fo"},
		},
		{
			name:       "regex, ignore_case",
			matchType:  MatchPatternRegex,
			target:     "fo{2}",
			ignoreCase: true,
			matched:    []string{"foo", "0foo", "fooo", "aFoo"},
			mismatched: []string{"fo"},
		},
		{
			name:       "regex, ignore_case & case insensitive specified in regex",
			matchType:  MatchPatternRegex,
			target:     "(?i)fo{2}",
			ignoreCase: true,
			matched:    []string{"foo", "0foo", "fooo", "aFoo"},
			mismatched: []string{"fo"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			built, _ := BuildStringMatcher(tt.matchType, tt.target, tt.ignoreCase)
			for _, s := range tt.matched {
				assert.True(t, built.Match(s))
			}
			for _, s := range tt.mismatched {
				assert.False(t, built.Match(s))
			}
		})
	}
}
