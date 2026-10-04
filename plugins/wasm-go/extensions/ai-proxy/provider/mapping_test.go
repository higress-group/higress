package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestGetMappedModelDeterministicWithOverlappingWildcards guards #4883:
// with overlapping wildcard prefixes the most specific (longest) prefix
// must win, and the result must not depend on map iteration order.
func TestGetMappedModelDeterministicWithOverlappingWildcards(t *testing.T) {
	mapping := map[string]string{
		"gpt-*":   "model-A",
		"gpt-4-*": "model-B",
	}
	for i := 0; i < 1000; i++ {
		assert.Equal(t, "model-B", getMappedModel("gpt-4-turbo", mapping),
			"iteration %d: the longest matching prefix must win deterministically", i)
	}
}

func TestGetMappedModelPrefixSemantics(t *testing.T) {
	mapping := map[string]string{
		"gpt-3":    "qwen-turbo",
		"gpt-4-*":  "qwen-max",
		"gpt-*":    "fallback",
		"~^gpt-5(-[a-z]+)?$": "regex-mapped",
		"*":        "star",
	}

	assert.Equal(t, "qwen-turbo", getMappedModel("gpt-3", mapping))      // exact
	assert.Equal(t, "qwen-max", getMappedModel("gpt-4-turbo", mapping))  // longest prefix
	assert.Equal(t, "fallback", getMappedModel("gpt-4", mapping))        // no trailing dash: matches gpt-* only
	assert.Equal(t, "regex-mapped", getMappedModel("gpt-5", mapping))    // regex key
	assert.Equal(t, "star", getMappedModel("claude-4", mapping))         // wildcard
}
