// Copyright (c) 2026 Alibaba Group Holding Ltd.
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

package provider

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetMappedModel(t *testing.T) {
	t.Run("explicit mapping", func(t *testing.T) {
		mapping := map[string]string{"gpt-4": "qwen-max"}
		require.Equal(t, "qwen-max", getMappedModel("gpt-4", mapping))
	})

	// A key ending with "*" matches by prefix, and the configured value is
	// used as is.
	t.Run("prefix mapping", func(t *testing.T) {
		mapping := map[string]string{"gpt-*": "qwen-default"}
		require.Equal(t, "qwen-default", getMappedModel("gpt-4", mapping))
	})

	t.Run("regex mapping", func(t *testing.T) {
		mapping := map[string]string{"~gpt-(.*)": "qwen-$1"}
		require.Equal(t, "qwen-4", getMappedModel("gpt-4", mapping))
	})

	t.Run("wildcard mapping", func(t *testing.T) {
		mapping := map[string]string{"*": "default-model"}
		require.Equal(t, "default-model", getMappedModel("any-model", mapping))
	})

	t.Run("no mapping keeps the original model", func(t *testing.T) {
		mapping := map[string]string{"other": "x"}
		require.Equal(t, "gpt-4", getMappedModel("gpt-4", mapping))
	})

	t.Run("empty mapping keeps the original model", func(t *testing.T) {
		require.Equal(t, "gpt-4", getMappedModel("gpt-4", nil))
	})

	// The mapping key comes from the plugin configuration, so an invalid
	// regular expression must not panic on every request.
	t.Run("invalid regex is skipped instead of panicking", func(t *testing.T) {
		mapping := map[string]string{"~gpt-(": "qwen-$1"}
		require.NotPanics(t, func() {
			require.Equal(t, "gpt-4", getMappedModel("gpt-4", mapping))
		})
	})

	// An invalid regex must not prevent the remaining mapping entries from
	// being applied. The wildcard entry is looked up after the loop, so the
	// result does not depend on the map iteration order.
	t.Run("invalid regex does not break the other entries", func(t *testing.T) {
		mapping := map[string]string{
			"~gpt-(": "qwen-$1",
			"*":      "fallback-model",
		}
		require.NotPanics(t, func() {
			require.Equal(t, "fallback-model", getMappedModel("gpt-4", mapping))
		})
	})
}
