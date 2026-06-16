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

// Package util's first test file. SendResponse calls into proxywasm and
// requires a host emulator, so it is exercised end-to-end through main
// package tests; the three deterministic helpers (ReconvertHeaders,
// ExtractFromHeader, ContainsString) are unit-tested directly here so that
// future refactors to the helpers themselves don't depend on dragging in
// the wasm host harness.

package util

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// === Module A — ReconvertHeaders ========================================

// nil http.Header must produce a non-panicking nil/empty slice; downstream
// proxywasm calls accept either.
func TestReconvertHeaders_Nil(t *testing.T) {
	require.Empty(t, ReconvertHeaders(nil))
}

func TestReconvertHeaders_Empty(t *testing.T) {
	require.Empty(t, ReconvertHeaders(http.Header{}))
}

// Multi-key + multi-value: each (key, value) pair becomes a separate
// [2]string entry, and the result is sorted stably by key — required so
// proxywasm sees a deterministic order regardless of map iteration.
func TestReconvertHeaders_MultiValueSorted(t *testing.T) {
	h := http.Header{}
	h.Add("X-A", "1")
	h.Add("X-A", "2")
	h.Set("X-B", "b")
	h.Set("X-C", "c")

	got := ReconvertHeaders(h)
	// Two values for X-A → two entries; one each for X-B / X-C.
	require.Len(t, got, 4)
	// Sorted by key, ascending.
	require.Equal(t, "X-A", got[0][0])
	require.Equal(t, "X-A", got[1][0])
	require.Equal(t, "X-B", got[2][0])
	require.Equal(t, "X-C", got[3][0])
	// Values for the same key preserve their insertion order.
	require.Equal(t, "1", got[0][1])
	require.Equal(t, "2", got[1][1])
	require.Equal(t, "b", got[2][1])
}

// === Module B — ExtractFromHeader =======================================

// Hit on the literal-case key the caller asked for. The lookup compares the
// header key to its lower-case form, so callers must pass already-lowercased
// keys; `ExtractFromHeader(headers, "x-foo")` matches both "X-Foo" and
// "x-foo" but `(headers, "X-Foo")` matches neither.
func TestExtractFromHeader_LowercaseKeyHit(t *testing.T) {
	headers := [][2]string{
		{"Authorization", "Bearer token"},
		{"X-Foo", "bar"},
	}
	require.Equal(t, "Bearer token", ExtractFromHeader(headers, "authorization"))
}

// Mixed-case stored key still matches because the comparison lowercases the
// stored key, not the search key — pins the asymmetry above.
func TestExtractFromHeader_StoredMixedCase(t *testing.T) {
	headers := [][2]string{{"X-Foo", "bar"}}
	require.Equal(t, "bar", ExtractFromHeader(headers, "x-foo"))
}

// Leading and trailing whitespace in the stored value is trimmed so the
// caller doesn't have to defensively re-trim.
func TestExtractFromHeader_TrimsWhitespace(t *testing.T) {
	headers := [][2]string{{"X-Token", "   trimmed-value  "}}
	require.Equal(t, "trimmed-value", ExtractFromHeader(headers, "x-token"))
}

// Miss → empty string, not error: callers branch on `value != ""`.
func TestExtractFromHeader_Miss(t *testing.T) {
	headers := [][2]string{{"X-Foo", "bar"}}
	require.Equal(t, "", ExtractFromHeader(headers, "x-missing"))
}

func TestExtractFromHeader_EmptySlice(t *testing.T) {
	require.Equal(t, "", ExtractFromHeader(nil, "x-foo"))
	require.Equal(t, "", ExtractFromHeader([][2]string{}, "x-foo"))
}

// === Module C — ContainsString ==========================================

// Hit semantics: case-insensitive equality, NOT substring.
func TestContainsString_Hit(t *testing.T) {
	require.True(t, ContainsString([]string{"GET", "POST"}, "POST"))
}

func TestContainsString_HitCaseInsensitive(t *testing.T) {
	require.True(t, ContainsString([]string{"GET", "POST"}, "post"))
	require.True(t, ContainsString([]string{"GeT"}, "get"))
}

// "PO" is not a member, only a prefix — must miss. Pins that the helper
// is equality-based, not strings.Contains-based, in case of refactor drift.
func TestContainsString_PrefixIsNotMember(t *testing.T) {
	require.False(t, ContainsString([]string{"POST"}, "PO"))
}

func TestContainsString_Miss(t *testing.T) {
	require.False(t, ContainsString([]string{"GET", "POST"}, "PUT"))
}

func TestContainsString_EmptySlice(t *testing.T) {
	require.False(t, ContainsString(nil, "x"))
	require.False(t, ContainsString([]string{}, "x"))
}
