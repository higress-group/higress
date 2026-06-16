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

package main

import (
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

// === Module A — parseConfig validation & wildcard short-circuits =========
//
// parseConfig is 88.7% in baseline. Four reachable uncovered branches
// pinned below; together they exercise the full validation contract for
// user-supplied attributes lists and the `*` wildcards on the two enable
// gates. Without these tests:
//   - a malformed attribute object would silently survive ParseConfig
//   - an unknown rule string ("bogus") would propagate downstream and only
//     fail at attribute application time
//   - the `*` wildcard would behave as if it were a literal path suffix
//
// All four are driven through ParseConfig+NewTestHost rather than calling
// parseConfig directly so the wasm logger is initialized.

// `attributes: [42]` ⇒ gjson Array yields one element whose .Raw is "42";
// json.Unmarshal of that into Attribute struct returns
// "json: cannot unmarshal number into Go value of type main.Attribute" →
// parseConfig returns the error → host start fails. Pins main.go:581-584.
func TestParseConfig_AttributeNotObject_StartFails(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost([]byte(`{
			"attributes": [42]
		}`))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusFailed, status)
	})
}

// `rule` value not in the allowed enum ⇒ parseConfig returns
// "value of rule must be one of [nil, first, replace, append]" →
// host start fails. Pins main.go:585-587. The existing main_test.go
// fixtures only ever use the four legal rule values.
func TestParseConfig_InvalidRule_StartFails(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost([]byte(`{
			"attributes": [
				{
					"key": "x",
					"value_source": "fixed_value",
					"value": "y",
					"rule": "bogus"
				}
			]
		}`))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusFailed, status)
	})
}

// `enable_path_suffixes: ["*"]` ⇒ wildcard short-circuit clears the list
// and breaks out of the loop, leaving an empty enabledSuffixes list which
// isPathEnabled treats as "all paths enabled". Distinct from the existing
// "default path suffixes" tests where the suffixes are a literal list.
// Pins main.go:635-638.
func TestParseConfig_PathSuffixWildcard_EnablesAllPaths(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost([]byte(`{
			"enable_path_suffixes": ["*"]
		}`))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		conf, err := host.GetMatchConfig()
		require.NoError(t, err)
		c := conf.(*AIStatisticsConfig)
		// Wildcard must collapse to empty slice — isPathEnabled then
		// returns true for any path (per main.go:512-514).
		require.Len(t, c.enablePathSuffixes, 0)
		require.True(t, isPathEnabled("/anything", c.enablePathSuffixes))
	})
}

// Same wildcard contract on the content-type gate. Pins main.go:650-653.
func TestParseConfig_ContentTypeWildcard_EnablesAllContentTypes(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost([]byte(`{
			"enable_content_types": ["*"]
		}`))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		conf, err := host.GetMatchConfig()
		require.NoError(t, err)
		c := conf.(*AIStatisticsConfig)
		require.Len(t, c.enableContentTypes, 0)
		require.True(t, isContentTypeEnabled("text/anything", c.enableContentTypes))
	})
}

// === Module B — convertToUInt unsupported types =========================
//
// convertToUInt is 100% per existing tests, BUT the existing
// TestConvertToUInt only exercises the documented numeric types and one
// `"10"` string for the default branch. Pin two more default-branch
// shapes that are realistic in production (nil from a missing user
// attribute, slice from a malformed type assertion) so a future "support
// strings via Atoi" change can't sneak past unnoticed.
func TestConvertToUInt_NilAndSlice_FallToDefault(t *testing.T) {
	v, ok := convertToUInt(nil)
	require.False(t, ok)
	require.Equal(t, uint64(0), v)

	v, ok = convertToUInt([]int{1, 2, 3})
	require.False(t, ok)
	require.Equal(t, uint64(0), v)
}
