package scheduling

import (
	"testing"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-load-balancer/endpoint_metrics/backend"
)

func TestPredicatesAndMetricFilters(t *testing.T) {
	req := &LLMRequest{Model: "adapter-a"}
	pods := []*backend.PodMetrics{
		podMetrics("pod-a", 1, 0.2, map[string]int{"adapter-a": 1}, 2),
		podMetrics("pod-b", 4, 0.7, map[string]int{}, 2),
		podMetrics("pod-c", 9, 0.9, map[string]int{"other": 1, "x": 1}, 2),
	}

	if !lowLoRACostPredicate(req, pods[0]) || !lowLoRACostPredicate(req, pods[1]) {
		t.Fatal("active or capacity-available pod should be low LoRA cost")
	}
	if lowLoRACostPredicate(req, pods[2]) {
		t.Fatal("full pod without adapter should not be low LoRA cost")
	}
	if !canAcceptNewLoraPredicate(req, pods[1]) || canAcceptNewLoraPredicate(req, pods[2]) {
		t.Fatal("canAcceptNewLoraPredicate mismatch")
	}
	if !lowQueueingPodPredicate(req, pods[0]) {
		t.Fatal("low queue pod should pass")
	}
	if !noQueueAndLessThanKVCacheThresholdPredicate(1, 0.3)(req, pods[0]) {
		t.Fatal("pod-a should satisfy queue and kv threshold")
	}
	if defaultFilter.Name() != "critical request" || (*filter)(nil).Name() != "nil" {
		t.Fatal("filter name mismatch")
	}

	leastQueue, err := leastQueuingFilterFunc(req, pods)
	if err != nil || len(leastQueue) != 1 || leastQueue[0].Name != "pod-a" {
		t.Fatalf("leastQueuingFilterFunc = %v, %v; want pod-a", leastQueue, err)
	}
	leastKV, err := leastKVCacheFilterFunc(req, pods)
	if err != nil || len(leastKV) != 1 || leastKV[0].Name != "pod-a" {
		t.Fatalf("leastKVCacheFilterFunc = %v, %v; want pod-a", leastKV, err)
	}
	adapterPods, err := toFilterFunc(loRAAffinityPredicate)(req, pods)
	if err != nil || len(adapterPods) != 1 || adapterPods[0].Name != "pod-a" {
		t.Fatalf("loRAAffinityPredicate filter = %v, %v; want pod-a", adapterPods, err)
	}
}

func podMetrics(name string, queue int, kv float64, active map[string]int, maxActive int) *backend.PodMetrics {
	return &backend.PodMetrics{
		Pod: backend.Pod{Name: name, Address: name + ".svc"},
		Metrics: backend.Metrics{
			ActiveModels:        active,
			MaxActiveModels:     maxActive,
			WaitingQueueSize:    queue,
			KVCacheUsagePercent: kv,
		},
	}
}
