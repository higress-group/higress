package expr

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tidwall/gjson"
)

func TestBuildRepeatedStringMatcherIgnoreCase(t *testing.T) {
	cfg := `[
		{"exact":"foo"},
		{"prefix":"pre"},
		{"regex":"^Cache"}
	]`
	matched := []string{"Foo", "foO", "foo", "PreA", "cache-control", "Cache-Control"}
	mismatched := []string{"afoo", "fo"}
	jsonArray := gjson.Parse(cfg).Array()
	built, err := BuildRepeatedStringMatcherIgnoreCase(jsonArray)
	if err != nil {
		t.Fatalf("Failed to build RepeatedStringMatcher: %v", err)
	}

	for _, s := range matched {
		assert.True(t, built.Match(s))
	}
	for _, s := range mismatched {
		assert.False(t, built.Match(s))
	}
}

func TestPassOutRegexCompileErr(t *testing.T) {
	cfg := `{"regex":"(?!)aa"}`
	_, err := BuildRepeatedStringMatcher([]gjson.Result{gjson.Parse(cfg)})
	assert.NotNil(t, err)
}
