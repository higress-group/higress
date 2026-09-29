package expr

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func createMatcher(pattern string, caseSensitive bool) Matcher {
	pathMatcher, err := newStringExactMatcher(pattern, caseSensitive)
	if err != nil {
		panic(err)
	}
	return pathMatcher
}

func TestIsAllowedByMode(t *testing.T) {
	tests := []struct {
		name     string
		config   MatchRules
		domain   string
		method   string
		path     string
		expected bool
	}{
		{
			name: "Whitelist mode, rule matches",
			config: MatchRules{
				Mode: ModeWhitelist,
				RuleList: []Rule{
					{
						Domain: "example.com",
						Method: []string{"GET"},
						Path:   createMatcher("/foo", true),
					},
				},
			},
			domain:   "example.com",
			method:   "GET",
			path:     "/foo",
			expected: true,
		},
		{
			name: "Whitelist mode, rule does not match",
			config: MatchRules{
				Mode: ModeWhitelist,
				RuleList: []Rule{
					{
						Domain: "example.com",
						Method: []string{"GET"},
						Path:   createMatcher("/foo", true),
					},
				},
			},
			domain:   "example.com",
			method:   "POST",
			path:     "/foo",
			expected: false,
		},
		{
			name: "Blacklist mode, rule matches",
			config: MatchRules{
				Mode: ModeBlacklist,
				RuleList: []Rule{
					{
						Domain: "example.com",
						Method: []string{"GET"},
						Path:   createMatcher("/foo", true),
					},
				},
			},
			domain:   "example.com",
			method:   "GET",
			path:     "/foo",
			expected: false,
		},
		{
			name: "Blacklist mode, rule does not match",
			config: MatchRules{
				Mode: ModeBlacklist,
				RuleList: []Rule{
					{
						Domain: "example.com",
						Method: []string{"GET"},
						Path:   createMatcher("/foo", true),
					},
				},
			},
			domain:   "example.com",
			method:   "POST",
			path:     "/foo",
			expected: true,
		},
		{
			name: "Domain matches, Path is empty",
			config: MatchRules{
				Mode: ModeWhitelist,
				RuleList: []Rule{
					{Domain: "example.com", Path: nil},
				},
			},
			domain:   "example.com",
			method:   "GET",
			path:     "/foo",
			expected: true,
		},
		{
			name: "Domain is empty, Path matches",
			config: MatchRules{
				Mode: ModeWhitelist,
				RuleList: []Rule{
					{
						Domain: "",
						Path:   createMatcher("/foo", true),
					},
				},
			},
			domain:   "example.com",
			method:   "GET",
			path:     "/foo",
			expected: true,
		},
		{
			name: "All fields (Domain, Method, Path) are empty",
			config: MatchRules{
				Mode: ModeWhitelist,
				RuleList: []Rule{
					{Domain: "", Method: []string{}, Path: nil},
				},
			},
			domain:   "example.com",
			method:   "GET",
			path:     "/foo",
			expected: false,
		},
		{
			name: "Invalid mode",
			config: MatchRules{
				Mode: "invalid",
				RuleList: []Rule{
					{
						Domain: "example.com",
						Method: []string{"GET"},
						Path:   createMatcher("/foo", true),
					},
				},
			},
			domain:   "example.com",
			method:   "GET",
			path:     "/foo",
			expected: false,
		},
		{
			name: "Whitelist mode, generic domain matches",
			config: MatchRules{
				Mode: ModeWhitelist,
				RuleList: []Rule{
					{
						Domain: "*.example.com",
						Method: []string{"GET"},
						Path:   createMatcher("/foo", true),
					},
				},
			},
			domain:   "sub.example.com",
			method:   "GET",
			path:     "/foo",
			expected: true,
		},
		{
			name: "Whitelist mode, generic domain does not match",
			config: MatchRules{
				Mode: ModeWhitelist,
				RuleList: []Rule{
					{
						Domain: "*.example.com",
						Method: []string{"GET"},
						Path:   createMatcher("/foo", true),
					},
				},
			},
			domain:   "example.com",
			method:   "GET",
			path:     "/foo",
			expected: false,
		},
		{
			name: "Whitelist mode, only method matches",
			config: MatchRules{
				Mode: ModeWhitelist,
				RuleList: []Rule{
					{
						Method: []string{"GET"},
						Path:   nil,
					},
				},
			},
			domain:   "example.com",
			method:   "GET",
			path:     "/foo",
			expected: true,
		},
		{
			name: "Whitelist mode, only domain matches",
			config: MatchRules{
				Mode: ModeWhitelist,
				RuleList: []Rule{
					{
						Domain: "example.com",
						Path:   nil,
					},
				},
			},
			domain:   "example.com",
			method:   "GET",
			path:     "/foo",
			expected: true,
		},
		{
			name: "Blacklist mode, generic domain matches",
			config: MatchRules{
				Mode: ModeBlacklist,
				RuleList: []Rule{
					{
						Domain: "*.example.com",
						Method: []string{"GET"},
						Path:   createMatcher("/foo", true),
					},
				},
			},
			domain:   "sub.example.com",
			method:   "GET",
			path:     "/foo",
			expected: false,
		},
		{
			name: "Blacklist mode, generic domain does not match",
			config: MatchRules{
				Mode: ModeBlacklist,
				RuleList: []Rule{
					{
						Domain: "*.example.com",
						Method: []string{"GET"},
						Path:   createMatcher("/foo", true),
					},
				},
			},
			domain:   "example.com",
			method:   "GET",
			path:     "/foo",
			expected: true,
		},
		{
			name: "Domain with special characters",
			config: MatchRules{
				Mode: ModeWhitelist,
				RuleList: []Rule{
					{
						Domain: "example-*.com",
						Method: []string{"GET"},
						Path:   createMatcher("/foo", true),
					},
				},
			},
			domain:   "example-test.com",
			method:   "GET",
			path:     "/foo",
			expected: true,
		},
		{
			name: "Path with special characters",
			config: MatchRules{
				Mode: ModeWhitelist,
				RuleList: []Rule{
					{
						Domain: "example.com",
						Method: []string{"GET"},
						Path:   createMatcher("/foo-bar", true),
					},
				},
			},
			domain:   "example.com",
			method:   "GET",
			path:     "/foo-bar",
			expected: true,
		},
		{
			name: "Multiple methods, one matches",
			config: MatchRules{
				Mode: ModeWhitelist,
				RuleList: []Rule{
					{
						Domain: "example.com",
						Method: []string{"GET", "POST"},
						Path:   createMatcher("/foo", true),
					},
				},
			},
			domain:   "example.com",
			method:   "POST",
			path:     "/foo",
			expected: true,
		},
		{
			name: "Multiple methods, none match",
			config: MatchRules{
				Mode: ModeWhitelist,
				RuleList: []Rule{
					{
						Domain: "example.com",
						Method: []string{"GET", "POST"},
						Path:   createMatcher("/foo", true),
					},
				},
			},
			domain:   "example.com",
			method:   "PUT",
			path:     "/foo",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.IsAllowedByMode(tt.domain, tt.method, tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}
