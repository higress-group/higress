package scheduling

import "testing"

func TestGetScheduler_RejectsEmptyMetrics(t *testing.T) {
	if _, err := GetScheduler(nil, MetricPolicyDefault, ""); err == nil {
		t.Fatal("expected empty metrics error")
	}
}
