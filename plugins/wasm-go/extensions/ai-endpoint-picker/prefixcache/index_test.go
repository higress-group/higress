package prefixcache

import (
	"encoding/json"
	"math"
	"strings"
	"sync"
	"testing"
)

func expectedScore(matched, total int) float64 {
	ratio := float64(matched) / float64(total)
	length := math.Min(float64(matched)/8192, 1)
	return 0.75*ratio + 0.25*length*length
}

func TestIndexColdRepeatAndMiddleMessageDivergence(t *testing.T) {
	base := extractForTest(t, `{"model":"m","messages":[{"role":"system","content":"rules"},{"role":"user","content":"question"},{"role":"assistant","content":"answer"}]}`)
	changed := extractForTest(t, `{"model":"m","messages":[{"role":"system","content":"rules"},{"role":"user","content":"different"},{"role":"assistant","content":"answer"}]}`)
	index := NewIndex(DefaultCapacity)
	if got := index.Score("a", base.Chains); got != 0 {
		t.Fatalf("cold score=%v want 0", got)
	}
	index.Record("a", base.Chains, DefaultBlockSize)
	if got := index.Score("a", base.Chains); math.Abs(got-expectedScore(totalTokens(base.Chains), totalTokens(base.Chains))) > 1e-12 {
		t.Fatalf("repeat score=%v", got)
	}
	matched := changed.Chains[0][0].EstimatedTokens
	if got, want := index.Score("a", changed.Chains), expectedScore(matched, totalTokens(changed.Chains)); math.Abs(got-want) > 1e-12 {
		t.Fatalf("middle-message divergence score=%v want %v", got, want)
	}
}

func TestScoreAggregatesMultiplePromptsByTokenLength(t *testing.T) {
	locality := extractForTest(t, `{"model":"m","prompt":["short","a much longer second prompt"]}`)
	index := NewIndex(DefaultCapacity)
	index.Record("a", locality.Chains[:1], DefaultBlockSize)
	matched := totalTokens(locality.Chains[:1])
	if got, want := index.Score("a", locality.Chains), expectedScore(matched, totalTokens(locality.Chains)); math.Abs(got-want) > 1e-12 {
		t.Fatalf("multi-prompt score=%v want %v", got, want)
	}
}

func TestShortFullMatchBeatsLongPartialMatch(t *testing.T) {
	shared := `{"role":"user","content":"shared"}`
	short := extractForTest(t, `{"model":"m","messages":[`+shared+`]}`)
	long := extractForTest(t, `{"model":"m","messages":[`+shared+`,{"role":"assistant","content":"`+strings.Repeat("x", 4000)+`"}]}`)
	index := NewIndex(DefaultCapacity)
	index.Record("a", short.Chains, DefaultBlockSize)
	if shortScore, longScore := index.Score("a", short.Chains), index.Score("a", long.Chains); shortScore <= longScore {
		t.Fatalf("short full score=%v must exceed long partial score=%v", shortScore, longScore)
	}
}

func TestWeightedLRUInsertUpdateShrinkAndOversize(t *testing.T) {
	index := NewIndex(10)
	chain := [][]Block{{{Hash: 1, EstimatedTokens: 32}, {Hash: 2, EstimatedTokens: 16}}}
	index.Record("a", chain, 16)
	if index.Len("a") != 2 || index.UsedCost("a") != 3 {
		t.Fatalf("insert state len=%d cost=%d", index.Len("a"), index.UsedCost("a"))
	}
	index.Record("a", [][]Block{{{Hash: 1, EstimatedTokens: 32}}}, 8)
	if index.Len("a") != 2 || index.UsedCost("a") != 5 {
		t.Fatalf("duplicate cost update len=%d cost=%d", index.Len("a"), index.UsedCost("a"))
	}
	index.SetCapacity("a", 4)
	if index.Len("a") != 1 || index.UsedCost("a") != 4 {
		t.Fatalf("shrink state len=%d cost=%d", index.Len("a"), index.UsedCost("a"))
	}
	index.SetCapacity("a", 2)
	if index.Len("a") != 0 || index.UsedCost("a") != 0 {
		t.Fatalf("oversized entry was not self-evicted: len=%d cost=%d", index.Len("a"), index.UsedCost("a"))
	}
}

func TestScoreDoesNotRefreshLRU(t *testing.T) {
	index := NewIndex(2)
	index.Record("a", [][]Block{{{Hash: 1, EstimatedTokens: 1}}, {{Hash: 2, EstimatedTokens: 1}}}, 16)
	_ = index.Score("a", [][]Block{{{Hash: 1, EstimatedTokens: 1}}})
	index.Record("a", [][]Block{{{Hash: 3, EstimatedTokens: 1}}}, 16)
	if got := index.Score("a", [][]Block{{{Hash: 1, EstimatedTokens: 1}}}); got != 0 {
		t.Fatalf("score refreshed oldest entry, score=%v", got)
	}
}

func TestRecordRefreshesDuplicateAndOversizeInsertSelfEvicts(t *testing.T) {
	index := NewIndex(2)
	index.Record("a", [][]Block{{{Hash: 1, EstimatedTokens: 1}, {Hash: 2, EstimatedTokens: 1}}}, 16)
	index.Record("a", [][]Block{{{Hash: 1, EstimatedTokens: 1}}}, 16)
	index.Record("a", [][]Block{{{Hash: 3, EstimatedTokens: 1}}}, 16)
	if index.Score("a", [][]Block{{{Hash: 1, EstimatedTokens: 1}}}) == 0 || index.Score("a", [][]Block{{{Hash: 2, EstimatedTokens: 1}}}) != 0 {
		t.Fatal("record did not refresh duplicate recency")
	}

	index.SetCapacity("oversize", 1)
	index.Record("oversize", [][]Block{{{Hash: 4, EstimatedTokens: 33}}}, 16)
	if index.Len("oversize") != 0 || index.UsedCost("oversize") != 0 {
		t.Fatal("oversized insert did not self-evict")
	}
}

func TestRecordEvictsSuffixBeforePrefix(t *testing.T) {
	index := NewIndex(2)
	chain := [][]Block{{
		{Hash: 1, EstimatedTokens: 1},
		{Hash: 2, EstimatedTokens: 1},
		{Hash: 3, EstimatedTokens: 1},
	}}
	index.Record("a", chain, 16)
	if index.Len("a") != 2 {
		t.Fatalf("retained entries=%d want 2", index.Len("a"))
	}
	prefix := [][]Block{{chain[0][0], chain[0][1]}}
	if score := index.Score("a", prefix); score == 0 {
		t.Fatal("capacity pressure evicted the chain head")
	}
	if score := index.Score("a", [][]Block{{chain[0][2]}}); score != 0 {
		t.Fatalf("tail entry survived before prefix, score=%v", score)
	}
}

func TestDeleteRemovesEndpointState(t *testing.T) {
	index := NewIndex(2)
	index.Record("unhealthy", [][]Block{{{Hash: 1, EstimatedTokens: 1}}}, 16)
	index.Delete("unhealthy")
	if index.Len("unhealthy") != 0 || index.EndpointCount() != 0 {
		t.Fatalf("deleted endpoint state remains: len=%d count=%d", index.Len("unhealthy"), index.EndpointCount())
	}
}

func TestDifferentEndpointBlockSizesUseDifferentCosts(t *testing.T) {
	index := NewIndex(DefaultCapacity)
	chain := [][]Block{{{Hash: 1, EstimatedTokens: 33}}}
	index.Record("small-block", chain, 16)
	index.Record("large-block", chain, 32)
	index.Record("fallback", chain, 0)
	if index.UsedCost("small-block") != 3 || index.UsedCost("large-block") != 2 || index.UsedCost("fallback") != 3 {
		t.Fatalf("costs small=%d large=%d fallback=%d", index.UsedCost("small-block"), index.UsedCost("large-block"), index.UsedCost("fallback"))
	}
}

func TestIndexCleanupAndConcurrentAccess(t *testing.T) {
	index := NewIndex(128)
	locality := extractForTest(t, completionBody(t, strings.Repeat("a", 8192)))
	index.SetCapacity("stale", 32)
	var wait sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wait.Add(1)
		go func(id int) {
			defer wait.Done()
			endpoint := string(rune('a' + id%2))
			for iteration := 0; iteration < 100; iteration++ {
				index.SetCapacity(endpoint, 128)
				index.Record(endpoint, locality.Chains, 16+id)
				_ = index.Score(endpoint, locality.Chains)
			}
		}(worker)
	}
	wait.Wait()
	index.Cleanup(map[string]struct{}{"a": {}, "b": {}})
	if index.EndpointCount() != 2 || index.Len("stale") != 0 {
		t.Fatalf("cleanup state count=%d stale=%d", index.EndpointCount(), index.Len("stale"))
	}
}

func completionBody(t *testing.T, prompt string) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{"model": "m", "prompt": prompt})
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func totalTokens(chains [][]Block) int {
	total := 0
	for _, chain := range chains {
		for _, block := range chain {
			total += block.EstimatedTokens
		}
	}
	return total
}
