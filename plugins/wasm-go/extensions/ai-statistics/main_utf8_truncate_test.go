// Copyright (c) 2024 Alibaba Group Holding Ltd.
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
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/stretchr/testify/require"
)

// utf8TruncateLightModeLimit mirrors the lightweight-mode default of
// value_length_limit (see parseConfig: 4000 when use_default_response_attributes
// is set and no explicit value_length_limit is configured). setAttributeBySource
// cuts limit/2 bytes for the head and the tail.
const utf8TruncateLightModeLimit = 4000

// utf8TruncateDefaultLimit mirrors the default value_length_limit used with
// custom attributes (see parseConfig: 32000).
const utf8TruncateDefaultLimit = 32000

// longCjkAnswer returns an answer of 2000 CJK characters ("答案" x1000,
// 3 bytes per char => 6000 UTF-8 bytes), 1.5x the light-mode limit, so the
// historical byte cut at limit/2 = 2000 bytes landed mid-character at both cut
// points (2000 % 3 = 2, 4000 % 3 = 1) and emitted invalid UTF-8 into proxy
// properties and user attributes.
func longCjkAnswer() string {
	return strings.Repeat("答案", 1000)
}

// TestTruncateUtf8Safe_CutPointsStayOnRuneBoundaries is the table-driven
// regression test for the UTF-8 aware truncation used by setAttributeBySource
// (mirrors the jsonrpc-converter paradigm, commit 72176941). Before the fix the
// cut points were pure byte offsets: for the CJK light-mode case the recorded
// value was invalid UTF-8 (utf8.ValidString == false, 4 invalid bytes, head cut
// ending mid-rune e7 ad / tail cut starting mid-rune a1 88), and the corrupted
// value was persisted into proxy properties and user attributes.
func TestTruncateUtf8Safe_CutPointsStayOnRuneBoundaries(t *testing.T) {
	tests := []struct {
		name  string
		value string
		limit int
	}{
		{"ascii long at light-mode limit", strings.Repeat("a", 6000), utf8TruncateLightModeLimit},
		{"ascii long at default limit", strings.Repeat("a", 50000), utf8TruncateDefaultLimit},
		{"cjk long at light-mode limit", longCjkAnswer(), utf8TruncateLightModeLimit},
		{"cjk long at default limit", strings.Repeat("答案", 20000), utf8TruncateDefaultLimit},
		{"cjk long at odd light-mode limit", longCjkAnswer(), utf8TruncateLightModeLimit + 1},
		{"cjk long at odd default limit", strings.Repeat("答案", 20000), utf8TruncateDefaultLimit + 1},
		{"mixed cjk and ascii at tiny limit", "答案ab答案cd答案ef答案", 7},
		{"emoji at tiny limit", "🙂🙂🙂🙂🙂🙂🙂🙂", 9},
		{"short value under limit", "short answer", utf8TruncateLightModeLimit},
		{"value exactly at limit", strings.Repeat("x", utf8TruncateLightModeLimit), utf8TruncateLightModeLimit},
		{"cjk exactly at limit", strings.Repeat("答案", 1333), utf8TruncateLightModeLimit},
		{"empty value", "", utf8TruncateLightModeLimit},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.True(t, utf8.ValidString(tc.value), "input fixture must be valid UTF-8")
			got := truncateUtf8Safe(tc.value, tc.limit)
			require.True(t, utf8.ValidString(got), "recorded value must stay valid UTF-8")

			if len(tc.value) <= tc.limit {
				require.Equal(t, tc.value, got, "value at/under the limit must be returned unchanged")
				require.NotContains(t, got, " [truncated] ")
				return
			}

			require.Contains(t, got, " [truncated] ")
			head, tail, found := strings.Cut(got, " [truncated] ")
			require.True(t, found)
			// Cut points must move onto rune boundaries without changing the
			// limit semantics: head + tail stay within the byte limit, and
			// both pieces are genuine prefix/suffix of the original value.
			require.LessOrEqual(t, len(head)+len(tail), tc.limit)
			require.LessOrEqual(t, len(head), tc.limit/2)
			require.True(t, strings.HasPrefix(tc.value, head))
			require.True(t, strings.HasSuffix(tc.value, tail))
		})
	}

	// Pure-ASCII cut behavior must be byte-identical to the pre-fix output.
	t.Run("ascii cut points unchanged", func(t *testing.T) {
		value := strings.Repeat("a", 6000)
		limit := utf8TruncateLightModeLimit
		expected := value[:limit/2] + " [truncated] " + value[len(value)-limit/2:]
		require.Equal(t, expected, truncateUtf8Safe(value, limit))
	})
}

// TestUtf8TruncationEndToEnd_LightModeLimit drives the REAL response path
// (onHttpRequestHeaders -> onHttpResponseHeaders -> onHttpResponseBody ->
// parseResponse -> setAttributeBySource) through the plugin test host with
// value_length_limit=4000 (the light-mode tier) and a 6000-byte CJK answer,
// then reads back what the host recorded:
//   - property "answer" (as_separate_log_field: proxywasm.SetProperty of
//     MarshalStr(formattedValue) into filter state)
//   - property "ai_log" (user attributes flushed by
//     WriteUserAttributeToLogWithKey)
//
// Regression guard: the recorded value must stay valid UTF-8 (no \ufffd
// escapes) while keeping the truncation marker. Pre-fix this test failed with
// 4 literal \ufffd escapes (one per destroyed byte) in property "answer".
func TestUtf8TruncationEndToEnd_LightModeLimit(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		answer := longCjkAnswer()
		require.Equal(t, 6000, len(answer))
		require.True(t, utf8.ValidString(answer))

		config := fmt.Sprintf(`{
			"value_length_limit": %d,
			"attributes": [
				{"key": "answer", "apply_to_log": true, "as_separate_log_field": true},
				{"key": "answer", "apply_to_log": true}
			]
		}`, utf8TruncateLightModeLimit)

		host, status := test.NewTestHost([]byte(config))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/v1/chat/completions"},
			{":method", "POST"},
		})
		host.CallOnHttpRequestBody([]byte(`{"model":"gpt-4","messages":[{"role":"user","content":"hi"}]}`))
		host.CallOnHttpResponseHeaders([][2]string{
			{":status", "200"},
			{"content-type", "application/json"},
		})

		contentJSON, err := json.Marshal(answer)
		require.NoError(t, err)
		body := fmt.Sprintf(`{"choices":[{"message":{"content":%s}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2},"model":"gpt-4"}`, contentJSON)
		action := host.CallOnHttpResponseBody([]byte(body))
		require.Equal(t, types.ActionContinue, action)
		host.CompleteHttp()

		// 1. Separate log field: what landed in the filter state (proxy
		// properties). Head and tail keep 666 full CJK chars each; no byte of
		// a rune is destroyed, so no \ufffd escape may appear.
		raw, propErr := host.GetProperty([]string{"answer"})
		require.NoError(t, propErr)
		recorded := string(raw)
		require.Contains(t, recorded, " [truncated] ", "marker must survive into the recorded value")
		require.Equal(t, 0, strings.Count(recorded, `\ufffd`), "no destroyed bytes may reach the filter state")
		require.True(t, utf8.ValidString(recorded))
		// Head and tail keep 1998 bytes each = 666 CJK chars = 333 "答案"
		// pairs per cut point.
		require.Equal(t, 666, strings.Count(recorded, "答案"), "333 full pairs at each cut point")
		require.Equal(t, 4009, len(recorded), "1998 head bytes + 13 marker bytes + 1998 tail bytes")

		// 2. Combined user attributes flushed to the ai_log property.
		rawLog, logErr := host.GetProperty([]string{wrapper.AILogKey})
		require.NoError(t, logErr)
		aiLog := string(rawLog)
		require.Contains(t, aiLog, "answer")
		require.Contains(t, aiLog, " [truncated] ")
		require.Equal(t, 0, strings.Count(aiLog, `\ufffd`), "no destroyed bytes may reach the user attributes")
		require.True(t, utf8.ValidString(aiLog))
	})
}
