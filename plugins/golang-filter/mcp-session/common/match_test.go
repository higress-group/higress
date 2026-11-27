package common

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseMatchListCarriesServerName(t *testing.T) {
	rules := ParseMatchList([]interface{}{
		map[string]interface{}{
			"server_name":       "orders",
			"match_rule_domain": "orders.example.com",
			"match_rule_path":   "/mcp",
			"match_rule_type":   "prefix",
			"upstream_type":     "rest",
		},
	})

	require.Len(t, rules, 1)
	require.Equal(t, "orders", rules[0].ServerName)
	require.Equal(t, "orders", rules[0].SessionServerName())
}

func TestSessionServerNameUsesServerName(t *testing.T) {
	rule := MatchRule{ServerName: "server-A"}
	require.Equal(t, "server-A", rule.SessionServerName())
}

func TestSessionServerNameFallsBackToRuleKey(t *testing.T) {
	ruleA := MatchRule{
		MatchRuleDomain: "a.example.com",
		MatchRulePath:   "/server-a",
		MatchRuleType:   PrefixMatch,
		UpstreamType:    RestUpstream,
	}
	ruleB := MatchRule{
		MatchRuleDomain: "b.example.com",
		MatchRulePath:   "/server-b",
		MatchRuleType:   PrefixMatch,
		UpstreamType:    RestUpstream,
	}

	nameA := ruleA.SessionServerName()
	require.True(t, strings.HasPrefix(nameA, "rule:"))
	require.NotEqual(t, nameA, ruleB.SessionServerName())

	// The key is stable for the same rule and changes with its match fields.
	require.Equal(t, nameA, ruleA.SessionServerName())
	ruleA.MatchRulePath = "/server-c"
	require.NotEqual(t, nameA, ruleA.SessionServerName())
}
