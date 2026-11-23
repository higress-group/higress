package main

import (
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-endpoint-picker/scheduling"
)

func TestEndpointSnapshotCacheTTLAndFingerprintReuse(t *testing.T) {
	now := time.Unix(1, 0)
	metrics := "# TYPE vllm:num_requests_waiting gauge\nvllm:num_requests_waiting 1\n"
	hosts := [][2]string{{"host", healthyMetadata(metrics)}}
	getCalls, parseCalls := 0, 0
	cache := newEndpointSnapshotCache(func() ([][2]string, error) {
		getCalls++
		return hosts, nil
	}, func(metrics []byte) (scheduling.VLLMMetrics, error) {
		parseCalls++
		return scheduling.ParseCompactVLLMMetrics(metrics)
	}, func() time.Time { return now })

	first, err := cache.get()
	if err != nil || len(first.hosts) != 1 || getCalls != 1 || parseCalls != 1 {
		t.Fatalf("first snapshot=%+v get=%d parse=%d err=%v", first, getCalls, parseCalls, err)
	}
	now = now.Add(endpointSnapshotTTL - time.Millisecond)
	if _, err := cache.get(); err != nil || getCalls != 1 || parseCalls != 1 {
		t.Fatalf("TTL hit get=%d parse=%d err=%v", getCalls, parseCalls, err)
	}
	now = now.Add(time.Millisecond)
	if _, err := cache.get(); err != nil || getCalls != 2 || parseCalls != 1 {
		t.Fatalf("same fingerprint refresh get=%d parse=%d err=%v", getCalls, parseCalls, err)
	}

	metrics += "unrelated_metric 1\n"
	hosts = [][2]string{{"host", healthyMetadata(metrics)}}
	now = now.Add(endpointSnapshotTTL)
	unchanged, err := cache.get()
	if err != nil || getCalls != 3 || parseCalls != 1 {
		t.Fatalf("unrelated metric churn reparsed snapshot: get=%d parse=%d err=%v", getCalls, parseCalls, err)
	}
	if got := unchanged.hosts[0].candidate("model").endpoint.Signals[scheduling.SignalQueue].Value; got != 1 {
		t.Fatalf("unchanged queue=%v want 1", got)
	}

	metrics = "# TYPE vllm:num_requests_waiting gauge\nvllm:num_requests_waiting 2\n"
	hosts = [][2]string{{"host", healthyMetadata(metrics)}}
	now = now.Add(endpointSnapshotTTL)
	changed, err := cache.get()
	if err != nil || getCalls != 4 || parseCalls != 2 {
		t.Fatalf("changed refresh get=%d parse=%d err=%v", getCalls, parseCalls, err)
	}
	if got := changed.hosts[0].candidate("model").endpoint.Signals[scheduling.SignalQueue].Value; got != 2 {
		t.Fatalf("refreshed queue=%v want 2", got)
	}
}

func TestEndpointSnapshotCacheDerivesLoRAAffinityPerRequest(t *testing.T) {
	now := time.Unix(1, 0)
	metrics := "# TYPE vllm:lora_requests_info gauge\n" +
		`vllm:lora_requests_info{running_lora_adapters="adapter-a"} 1` + "\n"
	cache := newEndpointSnapshotCache(func() ([][2]string, error) {
		return [][2]string{{"host", healthyMetadata(metrics)}}, nil
	}, scheduling.ParseCompactVLLMMetrics, func() time.Time { return now })
	result, err := cache.get()
	if err != nil {
		t.Fatal(err)
	}
	snapshot := result.hosts[0]
	if _, exists := snapshot.metrics.BaseSignals[scheduling.SignalLoRAAffinity]; exists {
		t.Fatal("cached snapshot contains model-specific affinity")
	}
	if got := snapshot.candidate("adapter-a").endpoint.Signals[scheduling.SignalLoRAAffinity].Value; got != 1 {
		t.Fatalf("loaded adapter affinity=%v want 1", got)
	}
	if got := snapshot.candidate("adapter-b").endpoint.Signals[scheduling.SignalLoRAAffinity].Value; got != 0 {
		t.Fatalf("other adapter affinity=%v want 0", got)
	}
}

func TestEndpointSnapshotCacheDoesNotServeExpiredOnRefreshFailure(t *testing.T) {
	now := time.Unix(1, 0)
	fail := false
	getCalls := 0
	cache := newEndpointSnapshotCache(func() ([][2]string, error) {
		getCalls++
		if fail {
			return nil, errors.New("discovery failed")
		}
		return [][2]string{{"host", healthyMetadata("")}}, nil
	}, scheduling.ParseCompactVLLMMetrics, func() time.Time { return now })
	if result, err := cache.get(); err != nil || len(result.hosts) != 1 {
		t.Fatalf("initial result=%+v err=%v", result, err)
	}
	fail = true
	now = now.Add(endpointSnapshotTTL)
	if result, err := cache.get(); !errors.Is(err, errUpstreamHostsUnavailable) || len(result.hosts) != 0 {
		t.Fatalf("expired failure served stale result=%+v err=%v", result, err)
	}
	if _, err := cache.get(); !errors.Is(err, errUpstreamHostsUnavailable) || getCalls != 3 {
		t.Fatalf("failure unexpectedly cached: get=%d err=%v", getCalls, err)
	}
}

func TestEndpointSnapshotCacheDropsRemovedHostsAndIsolatesBadMetrics(t *testing.T) {
	now := time.Unix(1, 0)
	hosts := [][2]string{
		{"removed", healthyMetadata("")},
		{"kept", healthyMetadata("# TYPE vllm:num_requests_waiting gauge\nvllm:num_requests_waiting 1\n")},
	}
	cache := newEndpointSnapshotCache(func() ([][2]string, error) { return hosts, nil }, scheduling.ParseCompactVLLMMetrics, func() time.Time { return now })
	if result, err := cache.get(); err != nil || len(result.hosts) != 2 {
		t.Fatalf("initial result=%+v err=%v", result, err)
	}
	hosts = [][2]string{
		{"bad", healthyMetadata("vllm:num_requests_waiting nope\n")},
		{"kept", healthyMetadata("# TYPE vllm:num_requests_waiting gauge\nvllm:num_requests_waiting 1\n")},
	}
	now = now.Add(endpointSnapshotTTL)
	result, err := cache.get()
	if err != nil || len(result.hosts) != 1 || result.hosts[0].address != "kept" || result.skipped != 1 || result.skipMask&candidateSkipMetrics == 0 {
		t.Fatalf("refreshed result=%+v err=%v", result, err)
	}
	if _, exists := func() (compactHostSnapshot, bool) {
		for _, host := range cache.result.hosts {
			if host.address == "removed" {
				return host, true
			}
		}
		return compactHostSnapshot{}, false
	}(); exists {
		t.Fatal("removed host remained in compact cache")
	}
}

func TestEndpointSnapshotCacheRetainsCompactCacheConfig(t *testing.T) {
	now := time.Unix(1, 0)
	metrics := "# TYPE vllm:cache_config_info gauge\n" +
		`vllm:cache_config_info{block_size="128",num_gpu_blocks="4096"} 1` + "\n"
	cache := newEndpointSnapshotCache(func() ([][2]string, error) {
		return [][2]string{{"host", healthyMetadata(metrics)}}, nil
	}, scheduling.ParseCompactVLLMMetrics, func() time.Time { return now })
	result, err := cache.get()
	if err != nil {
		t.Fatal(err)
	}
	snapshot := result.hosts[0]
	if snapshot.cacheConfig.BlockSize != 128 || snapshot.cacheConfig.NumGPUBlocks != 4096 {
		t.Fatalf("cache config=%+v", snapshot.cacheConfig)
	}
	if snapshot.metrics.CacheConfig != (scheduling.CacheConfig{}) {
		t.Fatalf("cache config duplicated in metrics snapshot: %+v", snapshot.metrics.CacheConfig)
	}
}

func healthyMetadata(metrics string) string {
	return `{"health_status":"Healthy","metrics":` + strconv.Quote(metrics) + `}`
}
