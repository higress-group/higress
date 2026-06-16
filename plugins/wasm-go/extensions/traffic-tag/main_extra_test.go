// Copyright (c) 2025 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0

package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

// === helpers ============================================================

// mustConfig marshals m to JSON and fails the test on error.
func mustConfig(t *testing.T, m map[string]interface{}) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(m)
	require.NoError(t, err)
	return b
}

// findHeader returns the value of the first matching header (case-insensitive).
func findHeader(headers [][2]string, key string) (string, bool) {
	k := strings.ToLower(key)
	for _, h := range headers {
		if strings.ToLower(h[0]) == k {
			return h[1], true
		}
	}
	return "", false
}

// hasHeaderWithValue checks for a (case-insensitive) name + exact value match.
func hasHeaderWithValue(headers [][2]string, key, value string) bool {
	v, ok := findHeader(headers, key)
	return ok && v == value
}

// findPercentageKey searches a small input space for a string whose
// sha256(value)[:8] (big-endian uint64) % 100 satisfies want — mirroring
// content.go matchCondition's Op_Percent hashing so tests can assert both
// sides of a threshold deterministically.
func findPercentageKey(t *testing.T, want func(bucket uint64) bool) string {
	t.Helper()
	for i := 0; i < 100000; i++ {
		k := fmt.Sprintf("user-%d", i)
		h := sha256.Sum256([]byte(k))
		bucket := binary.BigEndian.Uint64(h[:8]) % 100
		if want(bucket) {
			return k
		}
	}
	t.Fatalf("no candidate key found within search budget")
	return ""
}

// noopLog satisfies log.Log without touching the wasm host — used when
// driving internal helpers directly (module D weight CDF).
type noopLog struct{}

func (noopLog) Trace(string)                     {}
func (noopLog) Tracef(string, ...interface{})    {}
func (noopLog) Debug(string)                     {}
func (noopLog) Debugf(string, ...interface{})    {}
func (noopLog) Info(string)                      {}
func (noopLog) Infof(string, ...interface{})     {}
func (noopLog) Warn(string)                      {}
func (noopLog) Warnf(string, ...interface{})     {}
func (noopLog) Error(string)                     {}
func (noopLog) Errorf(string, ...interface{})    {}
func (noopLog) Critical(string)                  {}
func (noopLog) Criticalf(string, ...interface{}) {}
func (noopLog) ResetID(string)                   {}

// singleConditionConfig builds a config with one condition group containing
// one rule — cuts boilerplate for module C operator-matrix tests.
func singleConditionConfig(t *testing.T, logic, condType, key, op string, value []string) json.RawMessage {
	return mustConfig(t, map[string]interface{}{
		"conditionGroups": []map[string]interface{}{
			{
				"headerName":  "X-Traffic-Tag",
				"headerValue": "match",
				"logic":       logic,
				"conditions": []map[string]interface{}{
					{
						"conditionType": condType,
						"key":           key,
						"operator":      op,
						"value":         value,
					},
				},
			},
		},
	})
}

// === Module A — config-reject paths ====================================

// Each test below asserts the plugin refuses to start when the config violates
// a documented validation rule. Goal: cover (ConditionRule).validate and
// parseWeightConfig reject branches that are line-of-defence against
// route-coloring misconfiguration in production.

func TestParseConfig_Reject_EmptyConditionType(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		cfg := mustConfig(t, map[string]interface{}{
			"conditionGroups": []map[string]interface{}{{
				"headerName": "X-Traffic-Tag", "headerValue": "x", "logic": "and",
				"conditions": []map[string]interface{}{{
					"conditionType": "", "key": "k", "operator": "equal", "value": []string{"v"},
				}},
			}},
		})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusFailed, status)
	})
}

func TestParseConfig_Reject_EmptyKey(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		cfg := mustConfig(t, map[string]interface{}{
			"conditionGroups": []map[string]interface{}{{
				"headerName": "X-Traffic-Tag", "headerValue": "x", "logic": "and",
				"conditions": []map[string]interface{}{{
					"conditionType": "header", "key": "", "operator": "equal", "value": []string{"v"},
				}},
			}},
		})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusFailed, status)
	})
}

func TestParseConfig_Reject_EmptyOperator(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		cfg := mustConfig(t, map[string]interface{}{
			"conditionGroups": []map[string]interface{}{{
				"headerName": "X-Traffic-Tag", "headerValue": "x", "logic": "and",
				"conditions": []map[string]interface{}{{
					"conditionType": "header", "key": "k", "operator": "", "value": []string{"v"},
				}},
			}},
		})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusFailed, status)
	})
}

func TestParseConfig_Reject_InvalidConditionType(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		cfg := singleConditionConfig(t, "and", "body", "k", "equal", []string{"v"})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusFailed, status)
	})
}

func TestParseConfig_Reject_InvalidOperator(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		cfg := singleConditionConfig(t, "and", "header", "k", "contains", []string{"v"})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusFailed, status)
	})
}

func TestParseConfig_Reject_InvalidLogic(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		cfg := singleConditionConfig(t, "xor", "header", "k", "equal", []string{"v"})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusFailed, status)
	})
}

func TestParseConfig_Reject_InEmptyValue(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		cfg := singleConditionConfig(t, "and", "header", "k", "in", []string{})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusFailed, status)
	})
}

func TestParseConfig_Reject_EqualMultipleValues(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// non-in/not_in/percentage operators require exactly one value
		cfg := singleConditionConfig(t, "and", "header", "k", "equal", []string{"a", "b"})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusFailed, status)
	})
}

func TestParseConfig_Reject_PercentageMultipleValues(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		cfg := singleConditionConfig(t, "and", "header", "k", "percentage", []string{"10", "20"})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusFailed, status)
	})
}

func TestParseConfig_Reject_PercentageNotInteger(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		cfg := singleConditionConfig(t, "and", "header", "k", "percentage", []string{"thirty"})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusFailed, status)
	})
}

func TestParseConfig_Reject_PercentageNegative(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		cfg := singleConditionConfig(t, "and", "header", "k", "percentage", []string{"-1"})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusFailed, status)
	})
}

func TestParseConfig_Reject_PercentageOver100(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		cfg := singleConditionConfig(t, "and", "header", "k", "percentage", []string{"101"})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusFailed, status)
	})
}

func TestParseConfig_Reject_InvalidRegex(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// "[" is an unterminated character class — regexp.Compile fails.
		cfg := singleConditionConfig(t, "and", "header", "k", "regex", []string{"["})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusFailed, status)
	})
}

func TestParseConfig_Reject_WeightNegative(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		cfg := mustConfig(t, map[string]interface{}{
			"weightGroups": []map[string]interface{}{
				{"headerName": "X-T", "headerValue": "v", "weight": -1},
			},
		})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusFailed, status)
	})
}

func TestParseConfig_Reject_WeightOver100(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		cfg := mustConfig(t, map[string]interface{}{
			"weightGroups": []map[string]interface{}{
				{"headerName": "X-T", "headerValue": "v", "weight": 101},
			},
		})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusFailed, status)
	})
}

func TestParseConfig_Reject_WeightSumExceeds(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		cfg := mustConfig(t, map[string]interface{}{
			"weightGroups": []map[string]interface{}{
				{"headerName": "X-T", "headerValue": "a", "weight": 60},
				{"headerName": "X-T", "headerValue": "b", "weight": 60},
			},
		})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusFailed, status)
	})
}

func TestParseConfig_Reject_MissingHeaderName(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		cfg := mustConfig(t, map[string]interface{}{
			"conditionGroups": []map[string]interface{}{{
				// HeaderName omitted
				"headerValue": "x", "logic": "and",
				"conditions": []map[string]interface{}{{
					"conditionType": "header", "key": "k", "operator": "equal", "value": []string{"v"},
				}},
			}},
		})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusFailed, status)
	})
}

// === Module B — config-field assertions (cast-back) =====================

// Locks down the CDF invariant: WeightGroup.Accumulate is the running sum,
// not the per-group weight. Bug here = wrong traffic split in production.
func TestParseConfig_FieldsAndCDF(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		cfg := mustConfig(t, map[string]interface{}{
			"conditionGroups": []map[string]interface{}{{
				"headerName": "X-Traffic-Tag", "headerValue": "match",
				"logic": "AND", // mixed case → must be lowercased
				"conditions": []map[string]interface{}{{
					"conditionType": "Header", // mixed case → must be lowercased
					"key":           "User-Agent",
					"operator":      "Equal", // mixed case
					"value":         []string{"Mozilla"},
				}},
			}},
			"weightGroups": []map[string]interface{}{
				{"headerName": "X-Traffic-Tag", "headerValue": "v30", "weight": 30},
				{"headerName": "X-Traffic-Tag", "headerValue": "v50", "weight": 50},
			},
			"defaultTagKey": "X-Default-Tag",
			"defaultTagVal": "fallback",
		})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		raw, err := host.GetMatchConfig()
		require.NoError(t, err)
		require.NotNil(t, raw)
		c := raw.(*TrafficTagConfig)

		require.Len(t, c.ConditionGroups, 1)
		g := c.ConditionGroups[0]
		require.Equal(t, "and", g.Logic, "logic must be lowercased")
		require.Len(t, g.Conditions, 1)
		rule := g.Conditions[0]
		require.Equal(t, "header", rule.ConditionType, "conditionType must be lowercased")
		require.Equal(t, "equal", rule.Operator, "operator must be lowercased")

		require.Len(t, c.WeightGroups, 2)
		require.EqualValues(t, 30, c.WeightGroups[0].Accumulate)
		require.EqualValues(t, 80, c.WeightGroups[1].Accumulate, "Accumulate must be running CDF (30+50), not per-group weight")

		require.Equal(t, "X-Default-Tag", c.DefaultTagKey)
		require.Equal(t, "fallback", c.DefaultTagVal)
	})
}

// === Module C — operator × logic × conditionType matrix ================

func TestCondition_Equal_NotMatch(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		cfg := singleConditionConfig(t, "and", "header", "X-Env", "equal", []string{"prod"})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		_ = host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "e.com"}, {":path", "/"}, {":method", "GET"},
			{"X-Env", "staging"},
		})
		_, ok := findHeader(host.GetRequestHeaders(), "x-traffic-tag")
		require.False(t, ok, "value differs from required → no tag")
		host.CompleteHttp()
	})
}

func TestCondition_NotEqual_Match(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		cfg := singleConditionConfig(t, "or", "header", "X-Env", "not_equal", []string{"prod"})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		_ = host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "e.com"}, {":path", "/"}, {":method", "GET"},
			{"X-Env", "staging"},
		})
		require.True(t, hasHeaderWithValue(host.GetRequestHeaders(), "x-traffic-tag", "match"))
		host.CompleteHttp()
	})
}

func TestCondition_NotEqual_NotMatch(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		cfg := singleConditionConfig(t, "and", "header", "X-Env", "not_equal", []string{"prod"})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		_ = host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "e.com"}, {":path", "/"}, {":method", "GET"},
			{"X-Env", "prod"},
		})
		_, ok := findHeader(host.GetRequestHeaders(), "x-traffic-tag")
		require.False(t, ok)
		host.CompleteHttp()
	})
}

func TestCondition_Prefix_NotMatch(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		cfg := singleConditionConfig(t, "and", "header", "User-Agent", "prefix", []string{"Mozilla"})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		_ = host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "e.com"}, {":path", "/"}, {":method", "GET"},
			{"User-Agent", "curl/7.0"},
		})
		_, ok := findHeader(host.GetRequestHeaders(), "x-traffic-tag")
		require.False(t, ok)
		host.CompleteHttp()
	})
}

func TestCondition_Regex_NotMatch(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		cfg := singleConditionConfig(t, "and", "header", "User-Agent", "regex", []string{`.*Mobile.*`})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		_ = host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "e.com"}, {":path", "/"}, {":method", "GET"},
			{"User-Agent", "Mozilla/5.0 Desktop"},
		})
		_, ok := findHeader(host.GetRequestHeaders(), "x-traffic-tag")
		require.False(t, ok)
		host.CompleteHttp()
	})
}

func TestCondition_In_Match(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		cfg := singleConditionConfig(t, "or", "header", "X-Tier", "in", []string{"gold", "platinum"})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		_ = host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "e.com"}, {":path", "/"}, {":method", "GET"},
			{"X-Tier", "gold"},
		})
		require.True(t, hasHeaderWithValue(host.GetRequestHeaders(), "x-traffic-tag", "match"))
		host.CompleteHttp()
	})
}

func TestCondition_In_NotMatch(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		cfg := singleConditionConfig(t, "and", "header", "X-Tier", "in", []string{"gold", "platinum"})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		_ = host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "e.com"}, {":path", "/"}, {":method", "GET"},
			{"X-Tier", "bronze"},
		})
		_, ok := findHeader(host.GetRequestHeaders(), "x-traffic-tag")
		require.False(t, ok)
		host.CompleteHttp()
	})
}

func TestCondition_NotIn_Match(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		cfg := singleConditionConfig(t, "or", "header", "X-Tier", "not_in", []string{"gold", "platinum"})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		_ = host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "e.com"}, {":path", "/"}, {":method", "GET"},
			{"X-Tier", "bronze"},
		})
		require.True(t, hasHeaderWithValue(host.GetRequestHeaders(), "x-traffic-tag", "match"))
		host.CompleteHttp()
	})
}

func TestCondition_NotIn_NotMatch(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		cfg := singleConditionConfig(t, "and", "header", "X-Tier", "not_in", []string{"gold", "platinum"})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		_ = host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "e.com"}, {":path", "/"}, {":method", "GET"},
			{"X-Tier", "gold"},
		})
		_, ok := findHeader(host.GetRequestHeaders(), "x-traffic-tag")
		require.False(t, ok)
		host.CompleteHttp()
	})
}

// Op_Percent is hashed sha256[:8] BE % 100; both sides of the threshold must
// be tested deterministically. findPercentageKey searches for inputs that
// land in each half so the test is not random-flaky.
func TestCondition_Percentage_Below(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		threshold := 30
		// Bucket < threshold satisfies hashInt64 < int64(percentThresholdInt).
		key := findPercentageKey(t, func(b uint64) bool { return b < uint64(threshold) })

		cfg := singleConditionConfig(t, "or", "header", "X-User-Id", "percentage", []string{fmt.Sprintf("%d", threshold)})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		_ = host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "e.com"}, {":path", "/"}, {":method", "GET"},
			{"X-User-Id", key},
		})
		require.True(t, hasHeaderWithValue(host.GetRequestHeaders(), "x-traffic-tag", "match"))
		host.CompleteHttp()
	})
}

func TestCondition_Percentage_AtOrAbove(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		threshold := 30
		key := findPercentageKey(t, func(b uint64) bool { return b >= uint64(threshold) })

		cfg := singleConditionConfig(t, "and", "header", "X-User-Id", "percentage", []string{fmt.Sprintf("%d", threshold)})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		_ = host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "e.com"}, {":path", "/"}, {":method", "GET"},
			{"X-User-Id", key},
		})
		_, ok := findHeader(host.GetRequestHeaders(), "x-traffic-tag")
		require.False(t, ok)
		host.CompleteHttp()
	})
}

// --- logic-state machine ----------------------------------------------

func TestCondition_AndLogic_PartialFail_NoTag(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// Two conditions joined by AND: first matches, second doesn't → no tag.
		cfg := mustConfig(t, map[string]interface{}{
			"conditionGroups": []map[string]interface{}{{
				"headerName": "X-Traffic-Tag", "headerValue": "match", "logic": "and",
				"conditions": []map[string]interface{}{
					{"conditionType": "header", "key": "X-A", "operator": "equal", "value": []string{"a"}},
					{"conditionType": "header", "key": "X-B", "operator": "equal", "value": []string{"b"}},
				},
			}},
		})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		_ = host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "e.com"}, {":path", "/"}, {":method", "GET"},
			{"X-A", "a"}, {"X-B", "wrong"},
		})
		_, ok := findHeader(host.GetRequestHeaders(), "x-traffic-tag")
		require.False(t, ok)
		host.CompleteHttp()
	})
}

func TestCondition_AndLogic_AllMatch_Tag(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		cfg := mustConfig(t, map[string]interface{}{
			"conditionGroups": []map[string]interface{}{{
				"headerName": "X-Traffic-Tag", "headerValue": "match", "logic": "and",
				"conditions": []map[string]interface{}{
					{"conditionType": "header", "key": "X-A", "operator": "equal", "value": []string{"a"}},
					{"conditionType": "header", "key": "X-B", "operator": "equal", "value": []string{"b"}},
				},
			}},
		})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		_ = host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "e.com"}, {":path", "/"}, {":method", "GET"},
			{"X-A", "a"}, {"X-B", "b"},
		})
		require.True(t, hasHeaderWithValue(host.GetRequestHeaders(), "x-traffic-tag", "match"))
		host.CompleteHttp()
	})
}

func TestCondition_OrLogic_AllFail_NoTag(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		cfg := mustConfig(t, map[string]interface{}{
			"conditionGroups": []map[string]interface{}{{
				"headerName": "X-Traffic-Tag", "headerValue": "match", "logic": "or",
				"conditions": []map[string]interface{}{
					{"conditionType": "header", "key": "X-A", "operator": "equal", "value": []string{"a"}},
					{"conditionType": "header", "key": "X-B", "operator": "equal", "value": []string{"b"}},
				},
			}},
		})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		_ = host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "e.com"}, {":path", "/"}, {":method", "GET"},
			{"X-A", "x"}, {"X-B", "y"},
		})
		_, ok := findHeader(host.GetRequestHeaders(), "x-traffic-tag")
		require.False(t, ok)
		host.CompleteHttp()
	})
}

func TestCondition_AndLogic_MissingHeader_NoTag(t *testing.T) {
	// AND + getConditionValue returns error (header absent) → matchCondition
	// short-circuits to false. Covers the "if conditionGroup.Logic == and:
	// return false" branch on the error path.
	test.RunTest(t, func(t *testing.T) {
		cfg := singleConditionConfig(t, "and", "header", "X-Required", "equal", []string{"v"})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		_ = host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "e.com"}, {":path", "/"}, {":method", "GET"},
		})
		_, ok := findHeader(host.GetRequestHeaders(), "x-traffic-tag")
		require.False(t, ok)
		host.CompleteHttp()
	})
}

// --- conditionType=cookie ---------------------------------------------

func TestCondition_Cookie_Missing_NoMatch(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		cfg := singleConditionConfig(t, "and", "cookie", "sid", "equal", []string{"abc"})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		// No Cookie header — parseCookie returns "", false → getConditionValue
		// returns error → AND short-circuits.
		_ = host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "e.com"}, {":path", "/"}, {":method", "GET"},
		})
		_, ok := findHeader(host.GetRequestHeaders(), "x-traffic-tag")
		require.False(t, ok)
		host.CompleteHttp()
	})
}

func TestCondition_Cookie_NotPresentInList_NoMatch(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		cfg := singleConditionConfig(t, "and", "cookie", "sid", "equal", []string{"abc"})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		// Cookie header present but key "sid" missing.
		_ = host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "e.com"}, {":path", "/"}, {":method", "GET"},
			{"Cookie", "other=value; another=v2"},
		})
		_, ok := findHeader(host.GetRequestHeaders(), "x-traffic-tag")
		require.False(t, ok)
		host.CompleteHttp()
	})
}

func TestCondition_Cookie_ValueWithEquals_Match(t *testing.T) {
	// parseCookie's SplitN("=",2) must keep the inner "=" in the value.
	test.RunTest(t, func(t *testing.T) {
		cfg := singleConditionConfig(t, "or", "cookie", "token", "equal", []string{"a=b=c"})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		_ = host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "e.com"}, {":path", "/"}, {":method", "GET"},
			{"Cookie", "other=v1; token=a=b=c; trailing=v2"},
		})
		require.True(t, hasHeaderWithValue(host.GetRequestHeaders(), "x-traffic-tag", "match"))
		host.CompleteHttp()
	})
}

// --- conditionType=parameter (utils.getFullRequestURL + getQueryParameter)

func TestCondition_Parameter_Match(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		cfg := singleConditionConfig(t, "or", "parameter", "user", "equal", []string{"alice"})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		_ = host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "e.com"}, {":path", "/api/data?foo=bar&user=alice"}, {":method", "GET"},
		})
		require.True(t, hasHeaderWithValue(host.GetRequestHeaders(), "x-traffic-tag", "match"))
		host.CompleteHttp()
	})
}

func TestCondition_Parameter_Missing_NoMatch(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		cfg := singleConditionConfig(t, "and", "parameter", "user", "equal", []string{"alice"})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		// "user" param is absent from the query string.
		_ = host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "e.com"}, {":path", "/api/data?foo=bar"}, {":method", "GET"},
		})
		_, ok := findHeader(host.GetRequestHeaders(), "x-traffic-tag")
		require.False(t, ok)
		host.CompleteHttp()
	})
}

func TestCondition_Parameter_NoQueryString_NoMatch(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		cfg := singleConditionConfig(t, "and", "parameter", "user", "equal", []string{"alice"})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		_ = host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "e.com"}, {":path", "/api/data"}, {":method", "GET"},
		})
		_, ok := findHeader(host.GetRequestHeaders(), "x-traffic-tag")
		require.False(t, ok)
		host.CompleteHttp()
	})
}

// --- multiple condition groups -----------------------------------------

func TestCondition_MultipleGroups_SecondMatches_TaggedWithSecond(t *testing.T) {
	// onContentRequestHeaders iterates groups in order; first match wins. When
	// group[0] does not match, it must fall through to group[1].
	test.RunTest(t, func(t *testing.T) {
		cfg := mustConfig(t, map[string]interface{}{
			"conditionGroups": []map[string]interface{}{
				{
					"headerName": "X-Traffic-Tag", "headerValue": "first", "logic": "and",
					"conditions": []map[string]interface{}{
						{"conditionType": "header", "key": "X-Env", "operator": "equal", "value": []string{"prod"}},
					},
				},
				{
					"headerName": "X-Traffic-Tag", "headerValue": "second", "logic": "and",
					"conditions": []map[string]interface{}{
						{"conditionType": "header", "key": "X-Env", "operator": "equal", "value": []string{"staging"}},
					},
				},
			},
		})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		_ = host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "e.com"}, {":path", "/"}, {":method", "GET"},
			{"X-Env", "staging"},
		})
		require.True(t, hasHeaderWithValue(host.GetRequestHeaders(), "x-traffic-tag", "second"))
		host.CompleteHttp()
	})
}

// === Module D — weight CDF deterministic boundaries ====================

// onWeightRequestHeaders is exported (within the package) and takes randomNum
// directly, so we drive the CDF state machine without random flakiness. We
// must run inside a NewTestHost session because addTagHeader calls proxywasm.*
// — but only the bool return value is asserted.
func TestOnWeightRequestHeaders_DeterministicCDF(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(json.RawMessage(`{}`))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		// Initialise an http context so proxywasm.* calls inside addTagHeader
		// have somewhere to land. The plugin body is a no-op on empty config.
		_ = host.CallOnHttpRequestHeaders([][2]string{{":authority", "e.com"}})

		// CDF for [{w:30, accum:30}, {w:50, accum:80}] — total 80, gap is [80,99].
		groups := []WeightGroup{
			{HeaderName: "X-T", HeaderValue: "v30", Weight: 30, Accumulate: 30},
			{HeaderName: "X-T", HeaderValue: "v50", Weight: 50, Accumulate: 80},
		}
		cases := []struct {
			name      string
			randomNum uint64
			wantMatch bool
		}{
			{"randomNum=0 → first bucket", 0, true},
			{"randomNum=29 → still first bucket (boundary low)", 29, true},
			{"randomNum=30 → second bucket (boundary)", 30, true},
			{"randomNum=79 → still second bucket (boundary low)", 79, true},
			{"randomNum=80 → falls into gap, no match", 80, false},
			{"randomNum=99 → still in gap, no match", 99, false},
			{"randomNum=100 → mod wraps to 0, first bucket", 100, true},
			{"randomNum=130 → mod wraps to 30, second bucket", 130, true},
			{"randomNum=180 → mod wraps to 80, gap, no match", 180, false},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				got := onWeightRequestHeaders(groups, c.randomNum, noopLog{})
				require.Equal(t, c.wantMatch, got)
			})
		}
	})
}

// 100% coverage CDF — when weights sum to 100 there is no gap; every randomNum
// hits some bucket. Pairs with the gap test above.
func TestOnWeightRequestHeaders_FullCoverageCDF(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(json.RawMessage(`{}`))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		_ = host.CallOnHttpRequestHeaders([][2]string{{":authority", "e.com"}})

		groups := []WeightGroup{
			{HeaderName: "X-T", HeaderValue: "v40", Weight: 40, Accumulate: 40},
			{HeaderName: "X-T", HeaderValue: "v60", Weight: 60, Accumulate: 100},
		}
		// every input lands in some bucket
		for _, r := range []uint64{0, 39, 40, 99} {
			t.Run(fmt.Sprintf("r=%d", r), func(t *testing.T) {
				require.True(t, onWeightRequestHeaders(groups, r, noopLog{}))
			})
		}
	})
}

// === Module E — default tag fallback ===================================

func TestDefault_FallbackApplied(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		cfg := mustConfig(t, map[string]interface{}{
			"conditionGroups": []map[string]interface{}{{
				"headerName": "X-Traffic-Tag", "headerValue": "match", "logic": "and",
				"conditions": []map[string]interface{}{
					{"conditionType": "header", "key": "X-Required", "operator": "equal", "value": []string{"yes"}},
				},
			}},
			// no weight groups; condition won't match → default fires
			"defaultTagKey": "X-Default-Tag",
			"defaultTagVal": "fallback-value",
		})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		_ = host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "e.com"}, {":path", "/"}, {":method", "GET"},
			// X-Required absent → condition fails → fallback expected
		})
		require.True(t, hasHeaderWithValue(host.GetRequestHeaders(), "x-default-tag", "fallback-value"))
		host.CompleteHttp()
	})
}

func TestDefault_EmptyKey_NotApplied(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		cfg := mustConfig(t, map[string]interface{}{
			// only the val set; key absent. parser only populates DefaultTagKey/Val
			// when both keys Exist(), so this should leave both empty and the
			// setDefaultTag early-return fires.
			"defaultTagVal": "ignored",
		})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		_ = host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "e.com"}, {":path", "/"}, {":method", "GET"},
		})
		// no tag header expected
		for _, h := range host.GetRequestHeaders() {
			require.NotContains(t, strings.ToLower(h[0]), "default", "no default tag should be added")
		}
		host.CompleteHttp()
	})
}

// Drive setDefaultTag's k=="" || v=="" branch directly — the parseConfig
// path never produces a half-populated default (both must Exist() together),
// so we exercise the early-return through the helper itself.
func TestSetDefaultTag_EmptyKeyOrVal_NoOp(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(json.RawMessage(`{}`))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		_ = host.CallOnHttpRequestHeaders([][2]string{{":authority", "e.com"}})

		before := host.GetRequestHeaders()
		setDefaultTag("", "v", noopLog{})
		setDefaultTag("k", "", noopLog{})
		setDefaultTag("", "", noopLog{})
		after := host.GetRequestHeaders()
		require.Equal(t, before, after, "setDefaultTag with empty key or value must not mutate headers")
	})
}

// === Module F — addTagHeader idempotency =================================

// When a request already carries the target header (e.g. user supplied it
// directly), the plugin must NOT overwrite — preserves caller intent and
// avoids double-stamping when multiple traffic-tag instances chain.
func TestAddTag_PreexistingHeader_NotOverwritten(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		cfg := singleConditionConfig(t, "or", "header", "User-Agent", "prefix", []string{"Mozilla"})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		_ = host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "e.com"}, {":path", "/"}, {":method", "GET"},
			{"User-Agent", "Mozilla/5.0"},
			{"X-Traffic-Tag", "preset-by-client"}, // already present
		})

		// First (and only) value should still be the caller's; "match" must not
		// have been added on top.
		v, ok := findHeader(host.GetRequestHeaders(), "x-traffic-tag")
		require.True(t, ok)
		require.Equal(t, "preset-by-client", v, "existing tag must be preserved")

		// also assert there is exactly one occurrence (no duplicate stamp)
		count := 0
		for _, h := range host.GetRequestHeaders() {
			if strings.EqualFold(h[0], "x-traffic-tag") {
				count++
			}
		}
		require.Equal(t, 1, count, "tag header must not be duplicated")
		host.CompleteHttp()
	})
}
