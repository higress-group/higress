package config

import (
	"fmt"
	"testing"

	"ext-auth/expr"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func parseMatchRuleConfig(ruleJSON string) (ExtAuthConfig, error) {
	var cfg ExtAuthConfig
	configJSON := fmt.Sprintf(`{
		"match_type": "blacklist",
		"match_list": [{%s}]
	}`, ruleJSON)
	err := parseMatchRules(gjson.Parse(configJSON), &cfg)
	return cfg, err
}

func TestParseMatchRuleHeaders(t *testing.T) {
	cfg, err := parseMatchRuleConfig(`"match_rule_headers": [
		{"name": "X-Custom-Auth", "exists": true},
		{"name": "x-skip-auth", "exists": false}
	]`)

	require.NoError(t, err)
	require.Equal(t, []expr.HeaderCondition{
		{Name: "x-custom-auth", Exists: true},
		{Name: "x-skip-auth", Exists: false},
	}, cfg.MatchRules.RuleList[0].Headers)
}

func TestParseMatchRuleHeadersOmitted(t *testing.T) {
	cfg, err := parseMatchRuleConfig(`"match_rule_method": ["GET"]`)

	require.NoError(t, err)
	require.Nil(t, cfg.MatchRules.RuleList[0].Headers)
}

func TestParseMatchRuleHeadersRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name      string
		ruleJSON  string
		errorText string
	}{
		{name: "empty list", ruleJSON: `"match_rule_headers": []`, errorText: "must be a non-empty array"},
		{name: "not a list", ruleJSON: `"match_rule_headers": {}`, errorText: "must be a non-empty array"},
		{name: "missing name", ruleJSON: `"match_rule_headers": [{"exists": true}]`, errorText: "missing required field 'name'"},
		{name: "missing exists", ruleJSON: `"match_rule_headers": [{"name": "x-auth"}]`, errorText: "missing required field 'exists'"},
		{name: "non-string name", ruleJSON: `"match_rule_headers": [{"name": 1, "exists": true}]`, errorText: "valid non-pseudo HTTP header name"},
		{name: "empty name", ruleJSON: `"match_rule_headers": [{"name": "", "exists": true}]`, errorText: "valid non-pseudo HTTP header name"},
		{name: "invalid name", ruleJSON: `"match_rule_headers": [{"name": "bad header", "exists": true}]`, errorText: "valid non-pseudo HTTP header name"},
		{name: "pseudo header", ruleJSON: `"match_rule_headers": [{"name": ":authority", "exists": true}]`, errorText: "valid non-pseudo HTTP header name"},
		{name: "non-boolean exists", ruleJSON: `"match_rule_headers": [{"name": "x-auth", "exists": "true"}]`, errorText: "exists must be a boolean"},
		{
			name:      "duplicate name ignoring case",
			ruleJSON:  `"match_rule_headers": [{"name": "X-Auth", "exists": true}, {"name": "x-auth", "exists": false}]`,
			errorText: `duplicate header name "x-auth"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseMatchRuleConfig(tt.ruleJSON)
			require.ErrorContains(t, err, tt.errorText)
		})
	}
}
