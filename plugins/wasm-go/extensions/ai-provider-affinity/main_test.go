package main

import (
	"fmt"
	"testing"

	"github.com/tidwall/gjson"
)

func TestParseConfig_Valid(t *testing.T) {
	json := gjson.Parse(`{
		"providers": [
			{"cluster": "outbound|443||llm-a.internal.dns", "weight": 70},
			{"cluster": "outbound|443||llm-a.internal.dns", "weight": 30}
		]
	}`)
	var cfg ProviderAffinityConfig
	if err := parseConfig(json, &cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ConsumerHeader != "x-mse-consumer" {
		t.Errorf("expected default consumer_header, got %q", cfg.ConsumerHeader)
	}
	if cfg.ClusterHeader != "x-higress-target-cluster" {
		t.Errorf("expected default cluster_header, got %q", cfg.ClusterHeader)
	}
	if len(cfg.slots) != 100 {
		t.Errorf("expected 100 slots, got %d", len(cfg.slots))
	}
}

func TestParseConfig_CustomHeaders(t *testing.T) {
	json := gjson.Parse(`{
		"consumer_header": "x-custom-consumer",
		"cluster_header": "x-custom-target",
		"providers": [
			{"cluster": "outbound|443||llm-a.internal.dns", "weight": 100}
		]
	}`)
	var cfg ProviderAffinityConfig
	if err := parseConfig(json, &cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ConsumerHeader != "x-custom-consumer" {
		t.Errorf("got %q", cfg.ConsumerHeader)
	}
	if cfg.ClusterHeader != "x-custom-target" {
		t.Errorf("got %q", cfg.ClusterHeader)
	}
}

func TestParseConfig_WeightNotSum100(t *testing.T) {
	json := gjson.Parse(`{
		"providers": [
			{"cluster": "outbound|443||llm-a.internal.dns", "weight": 60},
			{"cluster": "outbound|443||llm-a.internal.dns", "weight": 30}
		]
	}`)
	var cfg ProviderAffinityConfig
	if err := parseConfig(json, &cfg); err == nil {
		t.Fatal("expected error for weights not summing to 100")
	}
}

func TestParseConfig_EmptyProviders(t *testing.T) {
	json := gjson.Parse(`{"providers": []}`)
	var cfg ProviderAffinityConfig
	if err := parseConfig(json, &cfg); err == nil {
		t.Fatal("expected error for empty providers")
	}
}

func TestParseConfig_MissingCluster(t *testing.T) {
	json := gjson.Parse(`{
		"providers": [
			{"weight": 100}
		]
	}`)
	var cfg ProviderAffinityConfig
	if err := parseConfig(json, &cfg); err == nil {
		t.Fatal("expected error for missing cluster")
	}
}

func TestParseConfig_ZeroWeight(t *testing.T) {
	json := gjson.Parse(`{
		"providers": [
			{"cluster": "outbound|443||llm-a.internal.dns", "weight": 0},
			{"cluster": "outbound|443||llm-a.internal.dns", "weight": 100}
		]
	}`)
	var cfg ProviderAffinityConfig
	if err := parseConfig(json, &cfg); err == nil {
		t.Fatal("expected error for zero weight")
	}
}

func TestSelectCluster_Consistency(t *testing.T) {
	slots := buildSlots([]provider{
		{Cluster: "outbound|443||llm-a.internal.dns", Weight: 50},
		{Cluster: "outbound|443||llm-a.internal.dns", Weight: 50},
	})

	consumer := "alice"
	first := selectCluster(slots, consumer)
	for i := 0; i < 10; i++ {
		if got := selectCluster(slots, consumer); got != first {
			t.Errorf("inconsistent result for same consumer: got %q, want %q", got, first)
		}
	}
}

func TestSelectCluster_Distribution(t *testing.T) {
	clusterA := "outbound|443||llm-a.internal.dns"
	clusterB := "outbound|443||llm-b.internal.dns"
	slots := buildSlots([]provider{
		{Cluster: clusterA, Weight: 70},
		{Cluster: clusterB, Weight: 30},
	})

	hasA, hasB := false, false
	for _, c := range slots {
		switch c {
		case clusterA:
			hasA = true
		case clusterB:
			hasB = true
		}
	}
	if !hasA || !hasB {
		t.Fatalf("weight-expanded slots must include both clusters, hasA=%v hasB=%v", hasA, hasB)
	}

	seen := map[string]struct{}{}
	for i := 0; i < 256 && len(seen) < 2; i++ {
		seen[selectCluster(slots, fmt.Sprintf("consumer-%d", i))] = struct{}{}
	}
	if len(seen) < 2 {
		t.Errorf("expected hash routing to reach at least 2 clusters, got %v", seen)
	}
}

func TestSelectCluster_WeightedDistribution(t *testing.T) {
	slots := buildSlots([]provider{
		{Cluster: "outbound|443||llm-a.internal.dns", Weight: 100},
	})
	for _, c := range []string{"alice", "bob", "carol"} {
		if got := selectCluster(slots, c); got != "outbound|443||llm-a.internal.dns" {
			t.Errorf("single provider: expected llm-a, got %q for consumer %q", got, c)
		}
	}
}

func buildSlots(providers []provider) []string {
	slots := make([]string, 0, 100)
	for _, p := range providers {
		for i := 0; i < p.Weight; i++ {
			slots = append(slots, p.Cluster)
		}
	}
	return slots
}
