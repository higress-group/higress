package expr

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHeaderPresenceConditions(t *testing.T) {
	tests := []struct {
		name      string
		condition HeaderCondition
		headers   [][2]string
		matches   bool
	}{
		{
			name:      "exists true counts mixed-case empty-value header",
			condition: HeaderCondition{Name: "x-custom-auth", Exists: true},
			headers:   [][2]string{{"X-Custom-Auth", ""}},
			matches:   true,
		},
		{name: "exists true rejects missing header", condition: HeaderCondition{Name: "x-auth", Exists: true}},
		{name: "exists false matches missing header", condition: HeaderCondition{Name: "x-auth", Exists: false}, matches: true},
		{
			name:      "exists false rejects present header",
			condition: HeaderCondition{Name: "x-auth", Exists: false},
			headers:   [][2]string{{"x-auth", "value"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := Rule{Headers: []HeaderCondition{tt.condition}}
			request := RequestAttributes{HeaderNames: NewHeaderNameSet(tt.headers)}
			require.Equal(t, tt.matches, rule.matchesAllConditions(request))
		})
	}
}

func TestHeaderConditionsComposeWithRuleAndRulesOR(t *testing.T) {
	rule := Rule{
		Domain: "api.example.com",
		Method: []string{"GET"},
		Path:   createMatcher("/public", true),
		Headers: []HeaderCondition{
			{Name: "x-custom-auth", Exists: true},
			{Name: "x-skip-auth", Exists: false},
		},
	}
	request := RequestAttributes{
		Domain:      "api.example.com",
		Method:      "GET",
		Path:        "/public",
		HeaderNames: NewHeaderNameSet([][2]string{{"x-custom-auth", ""}}),
	}

	require.True(t, rule.matchesAllConditions(request))
	request.HeaderNames["x-skip-auth"] = struct{}{}
	require.False(t, rule.matchesAllConditions(request))
	delete(request.HeaderNames, "x-skip-auth")

	request.Method = "POST"
	require.False(t, rule.matchesAllConditions(request))

	request.Method = "GET"
	rules := MatchRules{RuleList: []Rule{
		{Headers: []HeaderCondition{{Name: "x-other", Exists: true}}},
		rule,
	}}
	require.True(t, rules.matchesAnyRule(request))
}

func TestHeaderPresenceAuthorizationScope(t *testing.T) {
	rule := Rule{Headers: []HeaderCondition{{Name: "x-custom-auth", Exists: true}}}
	matchingRequest := RequestAttributes{HeaderNames: NewHeaderNameSet([][2]string{{"x-custom-auth", ""}})}
	missingRequest := RequestAttributes{HeaderNames: NewHeaderNameSet(nil)}

	tests := []struct {
		name    string
		mode    string
		request RequestAttributes
		inScope bool
	}{
		{name: "whitelist match", mode: ModeWhitelist, request: matchingRequest},
		{name: "whitelist miss", mode: ModeWhitelist, request: missingRequest, inScope: true},
		{name: "blacklist match", mode: ModeBlacklist, request: matchingRequest, inScope: true},
		{name: "blacklist miss", mode: ModeBlacklist, request: missingRequest},
		{name: "unknown mode fails closed", mode: "unknown", request: missingRequest, inScope: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules := MatchRules{Mode: tt.mode, RuleList: []Rule{rule}}
			require.Equal(t, tt.inScope, rules.Matches(tt.request))
		})
	}
}

func TestRequiresRequestHeaders(t *testing.T) {
	methodOnlyRules := MatchRules{RuleList: []Rule{{Method: []string{"GET"}}}}
	headerRules := MatchRules{RuleList: []Rule{{Headers: []HeaderCondition{{Name: "x-auth", Exists: true}}}}}

	require.False(t, methodOnlyRules.RequiresRequestHeaders())
	require.True(t, headerRules.RequiresRequestHeaders())
}
