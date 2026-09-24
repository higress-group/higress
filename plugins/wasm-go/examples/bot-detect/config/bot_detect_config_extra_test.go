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

package config

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

// === Module A — FillDefaultValue =========================================
//
// FillDefaultValue is 0% in baseline. The existing ProcessTest never calls
// it; every fixture pre-fills BlockedCode and BlockedMessage. The function
// is the contract for "missing config → safe defaults" and a regression
// here would silently change either the deny status code (403) or the
// reject message ("Invalid User-Agent"), both of which are user-visible.

// Zero-value config gets BOTH defaults populated.
func TestFillDefaultValue_ZeroConfigSetsBoth(t *testing.T) {
	c := &BotDetectConfig{}
	c.FillDefaultValue()
	require.Equal(t, uint32(403), c.BlockedCode)
	require.Equal(t, "Invalid User-Agent", c.BlockedMessage)
}

// User-supplied non-zero BlockedCode must NOT be overwritten — the default
// only fills the unset slot. Distinct from the message-only case below.
func TestFillDefaultValue_PreservesCustomCode(t *testing.T) {
	c := &BotDetectConfig{BlockedCode: 429}
	c.FillDefaultValue()
	require.Equal(t, uint32(429), c.BlockedCode)
	// Message was empty → still gets default.
	require.Equal(t, "Invalid User-Agent", c.BlockedMessage)
}

// Mirror of the code case for the message field.
func TestFillDefaultValue_PreservesCustomMessage(t *testing.T) {
	c := &BotDetectConfig{BlockedMessage: "go away"}
	c.FillDefaultValue()
	require.Equal(t, "go away", c.BlockedMessage)
	require.Equal(t, uint32(403), c.BlockedCode)
}

// Both fields set → no-op (every default-fill branch's `if` evaluates
// false). Pins idempotency: calling FillDefaultValue twice on a fully
// configured struct must not mutate it.
func TestFillDefaultValue_FullyConfiguredIsNoop(t *testing.T) {
	c := &BotDetectConfig{BlockedCode: 418, BlockedMessage: "teapot"}
	c.FillDefaultValue()
	c.FillDefaultValue() // twice on purpose
	require.Equal(t, uint32(418), c.BlockedCode)
	require.Equal(t, "teapot", c.BlockedMessage)
}

// === Module B — Process fall-through =====================================
//
// Process is 91.7%. The existing ProcessTest hits every branch EXCEPT the
// final `return true, ""` at bot_detect_config.go:67 — the "non-empty UA,
// no allow match, no deny match, no default-bot match" case. This is the
// happy path for real browser traffic; without coverage, a refactor that
// flipped the default verdict would slip through.
func TestProcess_RealBrowserUA_AllowedByDefault(t *testing.T) {
	c := &BotDetectConfig{}
	// A typical Chrome desktop UA — no "bot/spider/crawler" substrings,
	// no version suffix that the default regex matrix targets.
	ua := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) " +
		"AppleWebKit/537.36 (KHTML, like Gecko) " +
		"Chrome/120.0.0.0 Safari/537.36"
	ok, reason := c.Process(ua)
	require.True(t, ok)
	require.Empty(t, reason)
}

// Allow list configured but the rule does NOT match the UA → loop
// completes, falls through to the deny + default checks. Mirrors the
// existing "allow + match" case but pins the no-match branch through the
// allow loop (statement at line 52.38). Existing test always supplies an
// allow rule that matches — never one that doesn't.
func TestProcess_AllowListNoMatch_FallsThrough(t *testing.T) {
	c := &BotDetectConfig{
		Allow: []*regexp.Regexp{regexp.MustCompile(`^MyAllowedAgent$`)},
	}
	// UA matches NEITHER the allow rule NOR any default bot regex.
	ok, reason := c.Process("Mozilla/5.0 (compatible)")
	require.True(t, ok)
	require.Empty(t, reason)
}

// Deny rule fires before any default-bot rule could match. Existing
// "test deny bot detect" passes "Chrome" + deny=["Chrome"], but Chrome
// also doesn't match any default regex — so the test doesn't actually
// prove the user-deny path is checked BEFORE the default list. This UA
// would match a default regex (`indexer/1.2`) AND a user-deny regex; if
// the order were swapped, the returned reason string would change from
// the user-supplied rule to the default rule.
func TestProcess_UserDenyTakesPrecedenceOverDefault(t *testing.T) {
	customRule := `^indexer/`
	c := &BotDetectConfig{
		Deny: []*regexp.Regexp{regexp.MustCompile(customRule)},
	}
	ok, reason := c.Process("indexer/1.2")
	require.False(t, ok)
	// The reason must be the USER-CONFIGURED rule, not whichever default
	// regex would also have matched. Pins the "user deny first, default
	// list second" iteration order.
	require.Equal(t, customRule, reason)
}
