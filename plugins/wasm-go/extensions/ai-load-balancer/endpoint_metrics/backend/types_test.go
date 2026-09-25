package backend

import "testing"

func TestPodMetricsCloneDeepCopiesActiveModels(t *testing.T) {
	original := &PodMetrics{
		Pod: Pod{Name: "pod-a", Address: "10.0.0.1"},
		Metrics: Metrics{
			ActiveModels:            map[string]int{"adapter-a": 1},
			MaxActiveModels:         4,
			RunningQueueSize:        2,
			WaitingQueueSize:        3,
			KVCacheUsagePercent:     0.5,
			KvCacheMaxTokenCapacity: 100,
		},
		UserSelectedMetric: UserSelectedMetric{
			MetricName:  "custom_metric",
			MetricValue: 7,
		},
	}

	clone := original.Clone()
	clone.ActiveModels["adapter-a"] = 9
	clone.ActiveModels["adapter-b"] = 1

	if original.ActiveModels["adapter-a"] != 1 {
		t.Fatalf("clone mutated original active model value: %v", original.ActiveModels)
	}
	if _, ok := original.ActiveModels["adapter-b"]; ok {
		t.Fatalf("clone added model to original: %v", original.ActiveModels)
	}
	if original.String() == "" || original.Pod.String() != "pod-a:10.0.0.1" {
		t.Fatalf("unexpected string output: pod=%q metrics=%q", original.Pod.String(), original.String())
	}
}
