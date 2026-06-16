package cluster_hash

import (
	"fmt"
	"testing"

	"github.com/tidwall/gjson"
)

func TestParseConfig_Valid(t *testing.T) {
	json := gjson.Parse(`{
		"clusters": [
			{"cluster": "outbound|443||llm-a.internal.dns", "weight": 70},
			{"cluster": "outbound|443||llm-b.internal.dns", "weight": 30}
		]
	}`)
	lb, err := NewClusterHashLoadBalancer(json)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lb.HashHeader != DefaultHashHeader {
		t.Errorf("expected default hash_header %q, got %q", DefaultHashHeader, lb.HashHeader)
	}
	if lb.ClusterHeader != DefaultClusterHeader {
		t.Errorf("expected default cluster_header %q, got %q", DefaultClusterHeader, lb.ClusterHeader)
	}
	if len(lb.slots) != 100 {
		t.Errorf("expected 100 slots, got %d", len(lb.slots))
	}
}

func TestParseConfig_CustomHeaders(t *testing.T) {
	json := gjson.Parse(`{
		"hash_header": "x-custom-key",
		"cluster_header": "x-custom-target",
		"clusters": [
			{"cluster": "outbound|443||llm-a.internal.dns", "weight": 100}
		]
	}`)
	lb, err := NewClusterHashLoadBalancer(json)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lb.HashHeader != "x-custom-key" {
		t.Errorf("got hash_header %q", lb.HashHeader)
	}
	if lb.ClusterHeader != "x-custom-target" {
		t.Errorf("got cluster_header %q", lb.ClusterHeader)
	}
}

func TestParseConfig_WeightNotSum100(t *testing.T) {
	json := gjson.Parse(`{
		"clusters": [
			{"cluster": "outbound|443||llm-a.internal.dns", "weight": 60},
			{"cluster": "outbound|443||llm-b.internal.dns", "weight": 30}
		]
	}`)
	if _, err := NewClusterHashLoadBalancer(json); err == nil {
		t.Fatal("expected error for weights not summing to 100")
	}
}

func TestParseConfig_EmptyClusters(t *testing.T) {
	json := gjson.Parse(`{"clusters": []}`)
	if _, err := NewClusterHashLoadBalancer(json); err == nil {
		t.Fatal("expected error for empty clusters")
	}
}

func TestParseConfig_MissingClusters(t *testing.T) {
	json := gjson.Parse(`{}`)
	if _, err := NewClusterHashLoadBalancer(json); err == nil {
		t.Fatal("expected error for missing clusters field")
	}
}

func TestParseConfig_MissingClusterField(t *testing.T) {
	json := gjson.Parse(`{
		"clusters": [
			{"weight": 100}
		]
	}`)
	if _, err := NewClusterHashLoadBalancer(json); err == nil {
		t.Fatal("expected error for missing cluster field")
	}
}

func TestParseConfig_ZeroWeight(t *testing.T) {
	json := gjson.Parse(`{
		"clusters": [
			{"cluster": "outbound|443||llm-a.internal.dns", "weight": 0},
			{"cluster": "outbound|443||llm-b.internal.dns", "weight": 100}
		]
	}`)
	if _, err := NewClusterHashLoadBalancer(json); err == nil {
		t.Fatal("expected error for zero weight")
	}
}

func TestSelectCluster_Consistency(t *testing.T) {
	lb := buildLB(t, []clusterEntry{
		{Cluster: "outbound|443||llm-a.internal.dns", Weight: 50},
		{Cluster: "outbound|443||llm-b.internal.dns", Weight: 50},
	})

	key := "alice"
	first := lb.selectCluster(key)
	for range 10 {
		if got := lb.selectCluster(key); got != first {
			t.Errorf("inconsistent result for same key: got %q, want %q", got, first)
		}
	}
}

func TestSelectCluster_Distribution(t *testing.T) {
	clusterA := "outbound|443||llm-a.internal.dns"
	clusterB := "outbound|443||llm-b.internal.dns"
	lb := buildLB(t, []clusterEntry{
		{Cluster: clusterA, Weight: 70},
		{Cluster: clusterB, Weight: 30},
	})

	hasA, hasB := false, false
	for _, c := range lb.slots {
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
		seen[lb.selectCluster(fmt.Sprintf("key-%d", i))] = struct{}{}
	}
	if len(seen) < 2 {
		t.Errorf("expected hash routing to reach at least 2 clusters, got %v", seen)
	}
}

func TestSelectCluster_SingleCluster(t *testing.T) {
	target := "outbound|443||llm-a.internal.dns"
	lb := buildLB(t, []clusterEntry{
		{Cluster: target, Weight: 100},
	})
	for _, key := range []string{"alice", "bob", "carol"} {
		if got := lb.selectCluster(key); got != target {
			t.Errorf("single cluster: expected %q, got %q for key %q", target, got, key)
		}
	}
}

func buildLB(t *testing.T, entries []clusterEntry) ClusterHashLoadBalancer {
	t.Helper()
	slots := make([]string, 0, 100)
	for _, e := range entries {
		for i := 0; i < e.Weight; i++ {
			slots = append(slots, e.Cluster)
		}
	}
	return ClusterHashLoadBalancer{
		HashHeader:    DefaultHashHeader,
		ClusterHeader: DefaultClusterHeader,
		slots:         slots,
	}
}
