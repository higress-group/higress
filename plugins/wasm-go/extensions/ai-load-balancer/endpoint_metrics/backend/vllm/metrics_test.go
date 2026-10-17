package vllm

import (
	"testing"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-load-balancer/endpoint_metrics/backend"
	dto "github.com/prometheus/client_model/go"
)

func TestPromToPodMetrics_UserSelectedMetric(t *testing.T) {
	existing := &backend.PodMetrics{
		Pod: backend.Pod{Name: "pod-a"},
		UserSelectedMetric: backend.UserSelectedMetric{
			MetricName: "custom_metric",
		},
	}
	updated, err := PromToPodMetrics(map[string]*dto.MetricFamily{
		"custom_metric": gaugeFamily("custom_metric", 42),
	}, existing)
	if err != nil {
		t.Fatal(err)
	}
	if updated.MetricValue != 42 {
		t.Fatalf("MetricValue = %v, want 42", updated.MetricValue)
	}
}

func TestPromToPodMetrics_DefaultMetricsAndLora(t *testing.T) {
	existing := &backend.PodMetrics{
		Pod: backend.Pod{Name: "pod-a"},
		Metrics: backend.Metrics{
			ActiveModels: map[string]int{},
		},
	}
	updated, err := PromToPodMetrics(map[string]*dto.MetricFamily{
		RunningQueueSizeMetricName:    gaugeFamily(RunningQueueSizeMetricName, 3),
		WaitingQueueSizeMetricName:    gaugeFamily(WaitingQueueSizeMetricName, 5),
		KVCacheUsagePercentMetricName: gaugeFamily(KVCacheUsagePercentMetricName, 0.75),
		LoraRequestInfoMetricName: loraFamily([]*dto.Metric{
			loraMetric(1, "old", "1"),
			loraMetric(2, "adapter-a,adapter-b", "4"),
		}),
	}, existing)
	if err != nil {
		t.Fatal(err)
	}
	if updated.RunningQueueSize != 3 || updated.WaitingQueueSize != 5 || updated.KVCacheUsagePercent != 0.75 {
		t.Fatalf("metrics = running:%d waiting:%d kv:%v", updated.RunningQueueSize, updated.WaitingQueueSize, updated.KVCacheUsagePercent)
	}
	if updated.MaxActiveModels != 4 || len(updated.ActiveModels) != 2 {
		t.Fatalf("lora metrics = max:%d active:%v", updated.MaxActiveModels, updated.ActiveModels)
	}
}

func TestGetLatestMetricErrors(t *testing.T) {
	if _, err := getLatestMetric(map[string]*dto.MetricFamily{}, "missing"); err == nil {
		t.Fatal("expected missing metric error")
	}
	if _, err := getLatestMetric(map[string]*dto.MetricFamily{
		"empty": {Name: strPtr("empty")},
	}, "empty"); err == nil {
		t.Fatal("expected empty metric error")
	}
	if _, _, err := getLatestLoraMetric(map[string]*dto.MetricFamily{}); err == nil {
		t.Fatal("expected missing lora metric error")
	}
}

func gaugeFamily(name string, value float64) *dto.MetricFamily {
	return &dto.MetricFamily{
		Name: strPtr(name),
		Type: dto.MetricType_GAUGE.Enum(),
		Metric: []*dto.Metric{
			{
				Gauge:       &dto.Gauge{Value: floatPtr(value)},
				TimestampMs: int64Ptr(1),
			},
		},
	}
}

func loraFamily(metrics []*dto.Metric) *dto.MetricFamily {
	return &dto.MetricFamily{
		Name:   strPtr(LoraRequestInfoMetricName),
		Type:   dto.MetricType_GAUGE.Enum(),
		Metric: metrics,
	}
}

func loraMetric(value float64, runningAdapters, maxAdapters string) *dto.Metric {
	return &dto.Metric{
		Gauge: &dto.Gauge{Value: floatPtr(value)},
		Label: []*dto.LabelPair{
			{Name: strPtr(LoraRequestInfoRunningAdaptersMetricName), Value: strPtr(runningAdapters)},
			{Name: strPtr(LoraRequestInfoMaxAdaptersMetricName), Value: strPtr(maxAdapters)},
		},
	}
}

func strPtr(value string) *string     { return &value }
func floatPtr(value float64) *float64 { return &value }
func int64Ptr(value int64) *int64     { return &value }
