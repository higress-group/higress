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

package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetMappedModel_ExactMatch(t *testing.T) {
	mapping := map[string]string{
		"gpt-4": "exact-gpt-4",
		"gpt-*": "prefix-gpt",
		"*":     "fallback",
	}

	assert.Equal(t, "exact-gpt-4", getMappedModel("gpt-4", mapping))
	assert.Equal(t, "prefix-gpt", getMappedModel("gpt-3.5", mapping))
	assert.Equal(t, "fallback", getMappedModel("unknown", mapping))
}

func TestGetMappedModel_OverlappingWildcardPrefixDeterminism(t *testing.T) {
	mapping := map[string]string{
		"gpt-*":   "model-A",
		"gpt-4-*": "model-B",
	}

	// Repeatedly verify determinism across many iterations to ensure Go's randomized map iteration does not affect outcome
	for i := 0; i < 10000; i++ {
		res := doGetMappedModel("gpt-4-turbo", mapping)
		if res != "model-B" {
			t.Fatalf("iteration %d: expected model-B, got %s", i, res)
		}
	}

	assert.Equal(t, "model-A", doGetMappedModel("gpt-3.5-turbo", mapping))
	assert.Equal(t, "model-A", doGetMappedModel("gpt-4", mapping))
}

func TestGetMappedModel_MultiLevelWildcards(t *testing.T) {
	mapping := map[string]string{
		"gpt-*":             "model-A",
		"gpt-4-*":           "model-B",
		"gpt-4-turbo-*":     "model-C",
		"gpt-4-turbo-2024*": "model-D",
	}

	assert.Equal(t, "model-D", getMappedModel("gpt-4-turbo-2024-04-09", mapping))
	assert.Equal(t, "model-C", getMappedModel("gpt-4-turbo-preview", mapping))
	assert.Equal(t, "model-B", getMappedModel("gpt-4-0613", mapping))
	assert.Equal(t, "model-A", getMappedModel("gpt-3.5-turbo", mapping))
	assert.Equal(t, "unmatched-model", getMappedModel("unmatched-model", mapping))
}

func TestGetMappedModel_RegexMatch(t *testing.T) {
	mapping := map[string]string{
		"~^claude-(.*)":   "claude-broad-$1",
		"~^claude-3-(.*)": "claude-narrow-$1",
		"~gpt(.*)":        "openai/gpt$1",
	}

	// Longer regex pattern wins for specificity
	assert.Equal(t, "claude-narrow-opus", getMappedModel("claude-3-opus", mapping))
	assert.Equal(t, "claude-broad-2.1", getMappedModel("claude-2.1", mapping))
	assert.Equal(t, "openai/gpt-4o", getMappedModel("gpt-4o", mapping))
}

func TestGetMappedModel_InvalidRegexSafe(t *testing.T) {
	mapping := map[string]string{
		"~[invalid-regex": "should-not-match",
		"valid-*":         "matched-valid",
	}

	assert.Equal(t, "matched-valid", getMappedModel("valid-123", mapping))
	assert.Equal(t, "other", getMappedModel("other", mapping))
}

func TestGetMappedModel_EmptyTargetPreservesModel(t *testing.T) {
	mapping := map[string]string{
		"gpt-4-*": "",
		"*":       "fallback",
	}

	assert.Equal(t, "gpt-4-turbo", getMappedModel("gpt-4-turbo", mapping))
	assert.Equal(t, "fallback", getMappedModel("claude-3", mapping))
}

func TestGetMappedModel_EmptyMapping(t *testing.T) {
	assert.Equal(t, "original", getMappedModel("original", nil))
	assert.Equal(t, "original", getMappedModel("original", map[string]string{}))
}
