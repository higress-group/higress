package prefixcache

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func BenchmarkExtractAndHash(b *testing.B) {
	for _, size := range []int{4 << 10, 32 << 10, 128 << 10, 512 << 10, 4 << 20} {
		body, err := json.Marshal(map[string]any{"model": "m", "prompt": strings.Repeat("a", size)})
		if err != nil {
			b.Fatal(err)
		}
		b.Run(fmt.Sprintf("%dKB", size>>10), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(size))
			for b.Loop() {
				if _, supported, err := Extract(body); err != nil || !supported {
					b.Fatalf("extract: supported=%v err=%v", supported, err)
				}
			}
		})
	}
}

func BenchmarkScoreFullHit2048Blocks(b *testing.B) {
	benchmarkScoreFullHit(b, 2048, 1)
}

func BenchmarkScoreFullHit128SemanticSegments(b *testing.B) {
	benchmarkScoreFullHit(b, MaxPrefixTokens/MaxSegmentTokens, MaxSegmentTokens)
}

func benchmarkScoreFullHit(b *testing.B, blockCount, tokensPerBlock int) {
	chain := make([]Block, blockCount)
	for index := range chain {
		chain[index] = Block{Hash: uint64(index + 1), EstimatedTokens: tokensPerBlock}
	}
	chains := [][]Block{chain}
	for _, endpoints := range []int{1, 8, 32, 100} {
		index := NewIndex(DefaultCapacity)
		addresses := make([]string, endpoints)
		for endpoint := range endpoints {
			addresses[endpoint] = fmt.Sprintf("endpoint-%d", endpoint)
			index.Record(addresses[endpoint], chains, DefaultBlockSize)
		}
		b.Run(fmt.Sprintf("%dEndpoints", endpoints), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				for _, address := range addresses {
					_ = index.Score(address, chains)
				}
			}
		})
	}
}
