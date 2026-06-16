// Copyright (c) 2025 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package expr

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// === Module A — MatchRulesDefaults ======================================
//
// MatchRulesDefaults is at 0% in the baseline. It is consumed by
// config.ParseConfig as the zero-value MatchRules when the user supplies
// no match_list, so a regression here would silently change route-skip
// semantics from "whitelist with empty rule list" (block all by default)
// to whatever a future zero-value happens to mean.

func TestMatchRulesDefaults_WhitelistMode(t *testing.T) {
	d := MatchRulesDefaults()
	require.Equal(t, ModeWhitelist, d.Mode)
}

func TestMatchRulesDefaults_EmptyButNonNilRuleList(t *testing.T) {
	d := MatchRulesDefaults()
	require.NotNil(t, d.RuleList)
	require.Len(t, d.RuleList, 0)
}

// In whitelist mode with an empty rule list, every (domain, method, path)
// triple must be DENIED by the rule check (i.e. the auth server gets to see
// the request). The dual contract — blacklist + empty rule list = ALLOW —
// is already covered by match_rules_test.go via populated rule sets, but
// the empty-list defaults case is an important degenerate edge.
func TestMatchRulesDefaults_EmptyWhitelistDenies(t *testing.T) {
	d := MatchRulesDefaults()
	require.False(t, d.IsAllowedByMode("example.com", "GET", "/x"))
}

// === Module B — IsAllowedByMode default branch ==========================
//
// `default: return false` at match_rules.go:51 is unreachable through
// MatchRulesDefaults because Mode is whitelist there. A misconfigured /
// hand-built MatchRules with an unknown mode must safely fall back to
// "not allowed" so the request still goes through the auth server rather
// than silently bypassing it.

func TestIsAllowedByMode_UnknownModeFallsToFalse(t *testing.T) {
	mr := MatchRules{Mode: "not-a-mode", RuleList: []Rule{}}
	require.False(t, mr.IsAllowedByMode("example.com", "GET", "/x"))
}

// === Module C — BuildStringMatcher edges ================================
//
// BuildStringMatcher is at 75%; the unknown-type error branch and the
// invalid-regex branch are both unreached. Both must produce errors rather
// than nil-matchers so config.parseMatchRules can surface a proper config
// validation error.

func TestBuildStringMatcher_UnknownType(t *testing.T) {
	m, err := BuildStringMatcher("not-a-pattern", "x", false)
	require.Error(t, err)
	require.Nil(t, m)
	require.Contains(t, err.Error(), "unknown string matcher type")
}

func TestBuildStringMatcher_InvalidRegex(t *testing.T) {
	// Unbalanced "[" is a regexp.Compile error.
	m, err := BuildStringMatcher(MatchPatternRegex, "[unbalanced", false)
	require.Error(t, err)
	require.Nil(t, m)
}

// IgnoreCase + already-prefixed `(?i)` regex must NOT double-prefix —
// pins matcher.go:119-121's idempotency check.
func TestBuildStringMatcher_RegexIgnoreCaseAlreadyPrefixed(t *testing.T) {
	m, err := BuildStringMatcher(MatchPatternRegex, "(?i)foo", true)
	require.NoError(t, err)
	require.NotNil(t, m)
	require.True(t, m.Match("FOO"))
}
