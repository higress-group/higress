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

func TestMatchesPreservesBypassSemantics(t *testing.T) {
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
			bypassed := !tt.config.Matches(RequestAttributes{
				Domain: tt.domain,
				Method: tt.method,
				Path:   tt.path,
			})
			assert.Equal(t, tt.expected, bypassed)
		})
	}
}

func TestMatchDomainASCIICase(t *testing.T) {
	tests := []struct {
		name    string
		domain  string
		pattern string
		want    bool
	}{
		{
			name:    "exact uppercase pattern",
			domain:  "api.example.com",
			pattern: "API.EXAMPLE.COM",
			want:    true,
		},
		{
			name:    "exact uppercase domain",
			domain:  "API.EXAMPLE.COM",
			pattern: "api.example.com",
			want:    true,
		},
		{
			name:    "exact mixed case",
			domain:  "ApI.ExAmPlE.CoM",
			pattern: "aPi.eXaMpLe.cOm",
			want:    true,
		},
		{
			name:    "wildcard mixed case",
			domain:  "V1.API.Example.COM",
			pattern: "*.api.EXAMPLE.com",
			want:    true,
		},
		{
			name:    "different domain",
			domain:  "api.example.net",
			pattern: "API.EXAMPLE.COM",
			want:    false,
		},
		{
			name:    "non ASCII exact",
			domain:  "bücher.example",
			pattern: "bücher.example",
			want:    true,
		},
		{
			name:    "non ASCII case variant",
			domain:  "bücher.example",
			pattern: "BÜCHER.EXAMPLE",
			want:    false,
		},
		{
			name:    "wildcard does not match apex",
			domain:  "example.com",
			pattern: "*.EXAMPLE.COM",
			want:    false,
		},
		{
			name:    "question mark remains literal",
			domain:  "api1.example.com",
			pattern: "api?.EXAMPLE.COM",
			want:    false,
		},
		{
			name:    "character class remains literal",
			domain:  "a.example.com",
			pattern: "[ab].EXAMPLE.COM",
			want:    false,
		},
		{
			name:    "dot remains literal",
			domain:  "apiXexample.com",
			pattern: "API.EXAMPLE.COM",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, matchDomain(tt.domain, tt.pattern))
		})
	}
}

func TestMatchRulesDomainASCIICase(t *testing.T) {
	tests := []struct {
		name           string
		mode           string
		requestDomain  string
		configuredRule string
		want           bool
	}{
		{
			name:           "blacklist uppercase pattern",
			mode:           ModeBlacklist,
			requestDomain:  "api.example.com",
			configuredRule: "API.EXAMPLE.COM",
			want:           true,
		},
		{
			name:           "blacklist uppercase domain",
			mode:           ModeBlacklist,
			requestDomain:  "API.EXAMPLE.COM",
			configuredRule: "api.example.com",
			want:           true,
		},
		{
			name:           "blacklist mixed case wildcard",
			mode:           ModeBlacklist,
			requestDomain:  "V1.API.Example.COM",
			configuredRule: "*.api.EXAMPLE.com",
			want:           true,
		},
		{
			name:           "whitelist uppercase pattern",
			mode:           ModeWhitelist,
			requestDomain:  "api.example.com",
			configuredRule: "API.EXAMPLE.COM",
			want:           false,
		},
		{
			name:           "whitelist uppercase domain",
			mode:           ModeWhitelist,
			requestDomain:  "API.EXAMPLE.COM",
			configuredRule: "api.example.com",
			want:           false,
		},
		{
			name:           "whitelist mixed case wildcard",
			mode:           ModeWhitelist,
			requestDomain:  "V1.API.Example.COM",
			configuredRule: "*.api.EXAMPLE.com",
			want:           false,
		},
		{
			name:           "blacklist different domain",
			mode:           ModeBlacklist,
			requestDomain:  "api.example.net",
			configuredRule: "API.EXAMPLE.COM",
			want:           false,
		},
		{
			name:           "whitelist different domain",
			mode:           ModeWhitelist,
			requestDomain:  "api.example.net",
			configuredRule: "API.EXAMPLE.COM",
			want:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := MatchRules{
				Mode:     tt.mode,
				RuleList: []Rule{{Domain: tt.configuredRule}},
			}
			assert.Equal(t, tt.want, config.Matches(RequestAttributes{Domain: tt.requestDomain}))
		})
	}
}
