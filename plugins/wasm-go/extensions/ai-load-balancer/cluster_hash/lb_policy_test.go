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
	if lb.Key.Source != hashKeySourceHeader {
		t.Errorf("expected default key.source %q, got %q", hashKeySourceHeader, lb.Key.Source)
	}
	if lb.Key.Name != DefaultHashHeader {
		t.Errorf("expected default key.name %q, got %q", DefaultHashHeader, lb.Key.Name)
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
	if lb.Key.Name != "x-custom-key" {
		t.Errorf("got key.name %q", lb.Key.Name)
	}
	if lb.ClusterHeader != "x-custom-target" {
		t.Errorf("got cluster_header %q", lb.ClusterHeader)
	}
}

func TestParseConfig_KeyHeader(t *testing.T) {
	json := gjson.Parse(`{
		"key": {"source": "header", "name": "x-session-id"},
		"clusters": [
			{"cluster": "outbound|443||llm-a.internal.dns", "weight": 100}
		]
	}`)
	lb, err := NewClusterHashLoadBalancer(json)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lb.Key.Source != hashKeySourceHeader {
		t.Errorf("got key.source %q", lb.Key.Source)
	}
	if lb.Key.Name != "x-session-id" {
		t.Errorf("got key.name %q", lb.Key.Name)
	}
}

func TestParseConfig_KeyCookie(t *testing.T) {
	json := gjson.Parse(`{
		"key": {"source": "cookie", "name": "llm_session"},
		"clusters": [
			{"cluster": "outbound|443||llm-a.internal.dns", "weight": 100}
		]
	}`)
	lb, err := NewClusterHashLoadBalancer(json)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lb.Key.Source != hashKeySourceCookie {
		t.Errorf("got key.source %q", lb.Key.Source)
	}
	if lb.Key.Name != "llm_session" {
		t.Errorf("got key.name %q", lb.Key.Name)
	}
}

func TestParseConfig_KeyCookieMissingName(t *testing.T) {
	json := gjson.Parse(`{
		"key": {"source": "cookie"},
		"clusters": [
			{"cluster": "outbound|443||llm-a.internal.dns", "weight": 100}
		]
	}`)
	if _, err := NewClusterHashLoadBalancer(json); err == nil {
		t.Fatal("expected error for cookie key without name")
	}
}

func TestParseConfig_KeyBody(t *testing.T) {
	json := gjson.Parse(`{
		"key": {"source": "body", "jsonPath": "$.callOptions.stickySessionId", "max_body_bytes": 1024},
		"clusters": [
			{"cluster": "outbound|443||llm-a.internal.dns", "weight": 100}
		]
	}`)
	lb, err := NewClusterHashLoadBalancer(json)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lb.Key.Source != hashKeySourceBody {
		t.Errorf("got key.source %q", lb.Key.Source)
	}
	if lb.Key.JSONPath != "callOptions.stickySessionId" {
		t.Errorf("got json path %q", lb.Key.JSONPath)
	}
	if lb.Key.MaxBodyBytes != 1024 {
		t.Errorf("got max body bytes %d", lb.Key.MaxBodyBytes)
	}
}

func TestParseConfig_KeyBodyMissingJSONPath(t *testing.T) {
	json := gjson.Parse(`{
		"key": {"source": "body"},
		"clusters": [
			{"cluster": "outbound|443||llm-a.internal.dns", "weight": 100}
		]
	}`)
	if _, err := NewClusterHashLoadBalancer(json); err == nil {
		t.Fatal("expected error for body key without jsonPath")
	}
}

func TestParseConfig_KeyBodyMaxBytesOverflow(t *testing.T) {
	json := gjson.Parse(`{
		"key": {"source": "body", "jsonPath": "$.callOptions.stickySessionId", "max_body_bytes": 4294967296},
		"clusters": [
			{"cluster": "outbound|443||llm-a.internal.dns", "weight": 100}
		]
	}`)
	if _, err := NewClusterHashLoadBalancer(json); err == nil {
		t.Fatal("expected error for max_body_bytes overflow")
	}
}

func TestParseConfig_KeyMetadataName(t *testing.T) {
	json := gjson.Parse(`{
		"key": {"source": "metadata", "name": "selected_session_key"},
		"clusters": [
			{"cluster": "outbound|443||llm-a.internal.dns", "weight": 100}
		]
	}`)
	lb, err := NewClusterHashLoadBalancer(json)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fmt.Sprint(lb.Key.PropertyPath) != "[metadata selected_session_key]" {
		t.Errorf("got metadata path %v", lb.Key.PropertyPath)
	}
}

func TestParseConfig_KeyMetadataPropertyPath(t *testing.T) {
	json := gjson.Parse(`{
		"key": {"source": "metadata", "propertyPath": ["plugin", "session"]},
		"clusters": [
			{"cluster": "outbound|443||llm-a.internal.dns", "weight": 100}
		]
	}`)
	lb, err := NewClusterHashLoadBalancer(json)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fmt.Sprint(lb.Key.PropertyPath) != "[plugin session]" {
		t.Errorf("got metadata path %v", lb.Key.PropertyPath)
	}
}

func TestParseConfig_UnsupportedKeySource(t *testing.T) {
	json := gjson.Parse(`{
		"key": {"source": "query", "name": "session"},
		"clusters": [
			{"cluster": "outbound|443||llm-a.internal.dns", "weight": 100}
		]
	}`)
	if _, err := NewClusterHashLoadBalancer(json); err == nil {
		t.Fatal("expected error for unsupported key source")
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

func TestExtractCookieHashKey(t *testing.T) {
	got := extractCookieHashKey("theme=dark; llm_session=session-123; other=1", "llm_session")
	if got != "session-123" {
		t.Errorf("expected cookie value %q, got %q", "session-123", got)
	}
	if got := extractCookieHashKey("theme=dark", "llm_session"); got != "" {
		t.Errorf("expected empty missing cookie, got %q", got)
	}
}

func TestExtractBodyHashKey(t *testing.T) {
	body := []byte(`{"model":"gpt-4o","callOptions":{"stickySessionId":"session-123"}}`)
	got := extractBodyHashKey(body, "callOptions.stickySessionId")
	if got != "session-123" {
		t.Errorf("expected body key %q, got %q", "session-123", got)
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
		Key: hashKeyConfig{
			Source:       hashKeySourceHeader,
			Name:         DefaultHashHeader,
			MaxBodyBytes: DefaultMaxBodyBytes,
		},
		slots: slots,
	}
}
