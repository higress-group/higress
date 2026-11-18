// Copyright (c) 2022 Alibaba Group Holding Ltd.
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

package main

import (
	"encoding/json"
	"regexp"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

var hex16 = regexp.MustCompile(`^[0-9a-f]{16}$`)

func mustConfig(t *testing.T, v map[string]interface{}) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return data
}

// claudeRule is a fully-compiled rule used by the pure-function tests.
func claudeRule(t *testing.T) *ClientRule {
	t.Helper()
	return &ClientRule{
		Name:           "claude-code",
		MatchHeader:    "user-agent",
		MatchPattern:   `(?i)claude`,
		SessionHeaders: []string{"authorization", "x-api-key", "user-agent"},
		compiled:       regexp.MustCompile(`(?i)claude`),
	}
}

// ----------------------------------------------------------------------------
// Reproducibility: the core contract.
// ----------------------------------------------------------------------------

func TestDeriveSessionID_Reproducible(t *testing.T) {
	rule := claudeRule(t)
	headers := [][2]string{
		{"authorization", "Bearer sk-abc123"},
		{"x-api-key", "key-xyz"},
		{"user-agent", "claude-cli/1.0.42"},
	}

	id1, present1 := deriveSessionID(rule, headers, hashAlgoFNV)
	id2, present2 := deriveSessionID(rule, headers, hashAlgoFNV)

	require.Equal(t, id1, id2, "same input headers must yield the same id")
	require.True(t, present1)
	require.True(t, present2)
	require.Regexp(t, `^claude-code-[0-9a-f]{16}$`, id1)
	require.True(t, hex16.MatchString(id1[len("claude-code-"):]))
}

func TestDeriveSessionID_OrderMatters(t *testing.T) {
	base := claudeRule(t)
	reordered := claudeRule(t)
	reordered.SessionHeaders = []string{"user-agent", "x-api-key", "authorization"}

	headers := [][2]string{
		{"authorization", "Bearer sk-abc123"},
		{"x-api-key", "key-xyz"},
		{"user-agent", "claude-cli/1.0.42"},
	}

	idBase, _ := deriveSessionID(base, headers, hashAlgoFNV)
	idReordered, _ := deriveSessionID(reordered, headers, hashAlgoFNV)
	require.NotEqual(t, idBase, idReordered, "header order is part of the contract")
}

func TestDeriveSessionID_AlgorithmsDiffer(t *testing.T) {
	rule := claudeRule(t)
	headers := [][2]string{{"authorization", "Bearer x"}, {"x-api-key", "k"}, {"user-agent", "claude"}}

	idFNV, _ := deriveSessionID(rule, headers, hashAlgoFNV)
	idSHA, _ := deriveSessionID(rule, headers, hashAlgoSHA256)
	require.NotEqual(t, idFNV, idSHA)
	require.Regexp(t, `^claude-code-[0-9a-f]{16}$`, idSHA)
}

func TestDeriveSessionID_MissingHeader(t *testing.T) {
	rule := claudeRule(t)
	// x-api-key absent -> allPresent false, but still deterministic.
	headers := [][2]string{{"authorization", "Bearer x"}, {"user-agent", "claude"}}

	id1, present := deriveSessionID(rule, headers, hashAlgoFNV)
	id2, _ := deriveSessionID(rule, headers, hashAlgoFNV)
	require.False(t, present)
	require.Equal(t, id1, id2)
	require.Regexp(t, `^claude-code-[0-9a-f]{16}$`, id1)
}

func TestGetHeader_CaseInsensitive(t *testing.T) {
	headers := [][2]string{{"User-Agent", "Cursor/0.1"}}
	require.Equal(t, "Cursor/0.1", getHeader(headers, "user-agent"))
	require.Equal(t, "", getHeader(headers, "x-missing"))
}

// ----------------------------------------------------------------------------
// header 方案：逐级提取 + 取匹配之后的内容。
// ----------------------------------------------------------------------------

func TestExtractByHeaderRules_AfterMatch(t *testing.T) {
	rules := []HeaderExtractRule{
		{Header: "authorization", Pattern: `(?i)^bearer\s+`, compiled: regexp.MustCompile(`(?i)^bearer\s+`)},
	}
	headers := [][2]string{{"authorization", "Bearer sk-abc123"}}
	label, value, ok := extractByHeaderRules(rules, headers)
	require.True(t, ok)
	require.Equal(t, "authorization", label)
	require.Equal(t, "sk-abc123", value, "应取匹配之后的内容")
}

func TestExtractByHeaderRules_NoPatternTakesWholeValue(t *testing.T) {
	rules := []HeaderExtractRule{{Header: "x-session"}}
	headers := [][2]string{{"x-session", "  s-001  "}}
	label, value, ok := extractByHeaderRules(rules, headers)
	require.True(t, ok)
	require.Equal(t, "x-session", label)
	require.Equal(t, "s-001", value)
}

func TestExtractByHeaderRules_LevelByLevel(t *testing.T) {
	// 第一条规则的 header 缺失 -> 逐级落到第二条。
	rules := []HeaderExtractRule{
		{Header: "x-first"},
		{Header: "x-second"},
	}
	headers := [][2]string{{"x-second", "v2"}}
	label, value, ok := extractByHeaderRules(rules, headers)
	require.True(t, ok)
	require.Equal(t, "x-second", label)
	require.Equal(t, "v2", value)
}

func TestExtractByHeaderRules_NoMatch(t *testing.T) {
	rules := []HeaderExtractRule{
		{Header: "authorization", Pattern: `(?i)^bearer\s+`, compiled: regexp.MustCompile(`(?i)^bearer\s+`)},
	}
	headers := [][2]string{{"authorization", "Basic xxx"}}
	_, _, ok := extractByHeaderRules(rules, headers)
	require.False(t, ok)
}

func TestParseConfig_HeaderMode(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(mustConfig(t, map[string]interface{}{
			"match_mode": "header",
			"header_rules": []map[string]interface{}{
				{"header": "Authorization", "pattern": "(?i)^bearer\\s+"},
				{"header": "X-Session"},
			},
		}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		raw, err := host.GetMatchConfig()
		require.NoError(t, err)
		cfg := raw.(*AIHeaderSessionConfig)
		require.Equal(t, matchModeHeader, cfg.MatchMode)
		require.Len(t, cfg.HeaderRules, 2)
		require.Equal(t, "authorization", cfg.HeaderRules[0].Header)
		require.NotNil(t, cfg.HeaderRules[0].compiled)
		require.Nil(t, cfg.HeaderRules[1].compiled)
	})
}

func TestParseConfig_HeaderModeEmptyRules(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		_, status := test.NewTestHost(mustConfig(t, map[string]interface{}{
			"match_mode": "header",
		}))
		require.Equal(t, types.OnPluginStartStatusFailed, status)
	})
}

func TestParseConfig_InvalidMode(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		_, status := test.NewTestHost(mustConfig(t, map[string]interface{}{
			"match_mode": "magic",
		}))
		require.Equal(t, types.OnPluginStartStatusFailed, status)
	})
}

func TestOnHttpRequestHeaders_HeaderModeSetsSession(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(mustConfig(t, map[string]interface{}{
			"match_mode": "header",
			"header_rules": []map[string]interface{}{
				{"header": "authorization", "pattern": "(?i)^bearer\\s+"},
			},
		}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":path", "/v1/messages"},
			{"authorization", "Bearer sk-abc123"},
			{"user-agent", "curl/8.0"},
		})
		require.Equal(t, types.ActionContinue, action)
		got := sessionValue(host.GetRequestHeaders(), defaultSessionHeader)
		require.Regexp(t, `^authorization-[0-9a-f]{16}$`, got)
		host.CompleteHttp()
	})
}

// ----------------------------------------------------------------------------
// Config parsing.
// ----------------------------------------------------------------------------

func TestParseConfig_Defaults(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(mustConfig(t, map[string]interface{}{}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		raw, err := host.GetMatchConfig()
		require.NoError(t, err)
		cfg, ok := raw.(*AIHeaderSessionConfig)
		require.True(t, ok)
		require.Equal(t, defaultSessionHeader, cfg.SessionHeader)
		require.Equal(t, hashAlgoFNV, cfg.HashAlgorithm)
		require.True(t, cfg.Log.DumpUnmatched)
		require.Len(t, cfg.Clients, len(defaultClients()))
		for i := range cfg.Clients {
			require.NotNil(t, cfg.Clients[i].compiled, "client %d regex must be compiled", i)
		}
	})
}

func TestParseConfig_InvalidHash(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		_, status := test.NewTestHost(mustConfig(t, map[string]interface{}{
			"hash_algorithm": "md5",
		}))
		require.Equal(t, types.OnPluginStartStatusFailed, status)
	})
}

func TestParseConfig_InvalidRegex(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		_, status := test.NewTestHost(mustConfig(t, map[string]interface{}{
			"clients": []map[string]interface{}{
				{"name": "bad", "match_pattern": "([", "session_headers": []string{"user-agent"}},
			},
		}))
		require.Equal(t, types.OnPluginStartStatusFailed, status)
	})
}

func TestParseConfig_CustomClient(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(mustConfig(t, map[string]interface{}{
			"session_header": "X-My-Session",
			"clients": []map[string]interface{}{
				{"name": "myclient", "match_pattern": "(?i)mytool", "session_headers": []string{"X-Token"}},
			},
		}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		raw, err := host.GetMatchConfig()
		require.NoError(t, err)
		cfg := raw.(*AIHeaderSessionConfig)
		require.Equal(t, "X-My-Session", cfg.SessionHeader)
		require.Len(t, cfg.Clients, 1)
		require.Equal(t, "myclient", cfg.Clients[0].Name)
		require.Equal(t, defaultMatchHeader, cfg.Clients[0].MatchHeader)
		require.Equal(t, []string{"x-token"}, cfg.Clients[0].SessionHeaders)
	})
}

// ----------------------------------------------------------------------------
// Request-header processing flow.
// ----------------------------------------------------------------------------

func sessionValue(headers [][2]string, name string) string {
	return getHeader(headers, name)
}

func TestOnHttpRequestHeaders_SetsSession(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(mustConfig(t, map[string]interface{}{}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/v1/messages"},
			{":method", "POST"},
			{"authorization", "Bearer sk-abc"},
			{"x-api-key", "key-1"},
			{"user-agent", "claude-cli/1.0.42"},
		})
		require.Equal(t, types.ActionContinue, action)

		got := sessionValue(host.GetRequestHeaders(), defaultSessionHeader)
		require.Regexp(t, `^claude-code-[0-9a-f]{16}$`, got)
		host.CompleteHttp()
	})
}

func TestOnHttpRequestHeaders_SkipExisting(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(mustConfig(t, map[string]interface{}{}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":path", "/v1/messages"},
			{defaultSessionHeader, "preset-value"},
			{"user-agent", "claude-cli/1.0.42"},
		})
		require.Equal(t, types.ActionContinue, action)
		require.Equal(t, "preset-value", sessionValue(host.GetRequestHeaders(), defaultSessionHeader))
		host.CompleteHttp()
	})
}

func TestOnHttpRequestHeaders_UnmatchedPassThrough(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(mustConfig(t, map[string]interface{}{}))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":path", "/v1/chat/completions"},
			{"user-agent", "curl/8.0"},
		})
		require.Equal(t, types.ActionContinue, action)
		require.Equal(t, "", sessionValue(host.GetRequestHeaders(), defaultSessionHeader))
		host.CompleteHttp()
	})
}
