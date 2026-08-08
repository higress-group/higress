package prefixcache

import (
	"strings"
	"testing"
)

func chainsFor(t *testing.T, text string) [][]uint64 {
	t.Helper()
	locality, supported, err := Extract([]byte(`{"model":"m","prompt":"` + text + `"}`))
	if err != nil || !supported {
		t.Fatalf("extract: supported=%v err=%v", supported, err)
	}
	return locality.BlockChains(MinBlockSizeTokens)
}

func TestIndexColdStartRepeatAndSuffixDivergence(t *testing.T) {
	index := NewIndex(DefaultCapacity)
	base := chainsFor(t, strings.Repeat("a", 256)+strings.Repeat("b", 256))
	if got := index.Score("a", base); got != 0 {
		t.Fatalf("cold score=%v want 0", got)
	}
	index.Record("a", base)
	if got := index.Score("a", base); got != 1 {
		t.Fatalf("repeated score=%v want 1", got)
	}
	diverged := chainsFor(t, strings.Repeat("a", 256)+strings.Repeat("c", 256))
	if got := index.Score("a", diverged); got != 0.5 {
		t.Fatalf("diverged score=%v want 0.5", got)
	}
}

func TestIndexLRUCapacityAndCleanup(t *testing.T) {
	index := NewIndex(1)
	first := chainsFor(t, strings.Repeat("a", 256))
	second := chainsFor(t, strings.Repeat("b", 256))
	index.Record("a", first)
	index.Record("a", second)
	if got := index.Score("a", first); got != 0 {
		t.Fatalf("evicted prefix score=%v want 0", got)
	}
	if got := index.Score("a", second); got != 1 {
		t.Fatalf("recent prefix score=%v want 1", got)
	}
	index.SetCapacity("b", 4)
	index.Cleanup(map[string]struct{}{"b": {}})
	if index.EndpointCount() != 1 || index.Len("a") != 0 {
		t.Fatalf("cleanup retained stale endpoint: count=%d stale_len=%d", index.EndpointCount(), index.Len("a"))
	}
}

func TestIndexPerEndpointCapacityOverride(t *testing.T) {
	index := NewIndex(1)
	index.SetCapacity("a", 2)
	index.Record("a", chainsFor(t, strings.Repeat("a", 512)))
	if got := index.Len("a"); got != 2 {
		t.Fatalf("endpoint capacity override retained %d entries, want 2", got)
	}
}
