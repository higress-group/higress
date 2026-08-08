package scheduling

import (
	"fmt"
	"math"
	"strings"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
)

const (
	queueMetric       = "vllm:num_requests_waiting"
	currentKVMetric   = "vllm:kv_cache_usage_perc"
	legacyKVMetric    = "vllm:gpu_cache_usage_perc"
	loraMetric        = "vllm:lora_requests_info"
	loraAdaptersLabel = "running_lora_adapters"
)

// ParseVLLMSignals parses one endpoint's Prometheus snapshot. A missing metric
// family is a valid partial snapshot; malformed exposition is not.
func ParseVLLMSignals(metrics, model string) (map[SignalName]SignalValue, error) {
	result := map[SignalName]SignalValue{}
	if strings.TrimSpace(metrics) == "" {
		return result, nil
	}

	var parser expfmt.TextParser
	families, err := parser.TextToMetricFamilies(strings.NewReader(metrics))
	if err != nil {
		return nil, fmt.Errorf("parse prometheus metrics: %w", err)
	}

	if value, ok := latestFiniteValue(families[queueMetric]); ok && value >= 0 {
		result[SignalQueue] = available(value, queueMetric)
	}
	if value, ok := latestFiniteValue(families[currentKVMetric]); ok {
		result[SignalKVCache] = available(value, currentKVMetric)
	} else if value, ok := latestFiniteValue(families[legacyKVMetric]); ok {
		result[SignalKVCache] = available(value, legacyKVMetric)
	}
	if family := families[loraMetric]; family != nil {
		if value, ok := loraAffinity(family, model); ok {
			result[SignalLoRAAffinity] = available(value, loraMetric)
		}
	}
	return result, nil
}

func available(value float64, source string) SignalValue {
	return SignalValue{Value: value, Available: true, Confidence: 1, Source: source}
}

func latestFiniteValue(family *dto.MetricFamily) (float64, bool) {
	if family == nil || len(family.Metric) == 0 {
		return 0, false
	}
	var selected *dto.Metric
	for _, metric := range family.Metric {
		if selected == nil || metric.GetTimestampMs() >= selected.GetTimestampMs() {
			selected = metric
		}
	}
	value, ok := metricValue(selected)
	return value, ok && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func metricValue(metric *dto.Metric) (float64, bool) {
	if metric == nil {
		return 0, false
	}
	switch {
	case metric.Gauge != nil:
		return metric.GetGauge().GetValue(), true
	case metric.Counter != nil:
		return metric.GetCounter().GetValue(), true
	case metric.Untyped != nil:
		return metric.GetUntyped().GetValue(), true
	default:
		return 0, false
	}
}

func loraAffinity(family *dto.MetricFamily, model string) (float64, bool) {
	var selected *dto.Metric
	var selectedValue float64
	for _, metric := range family.Metric {
		value, ok := metricValue(metric)
		if !ok || math.IsNaN(value) || math.IsInf(value, 0) {
			continue
		}
		if selected == nil || value > selectedValue {
			selected, selectedValue = metric, value
		}
	}
	if selected == nil {
		return 0, false
	}
	for _, label := range selected.Label {
		if label.GetName() != loraAdaptersLabel {
			continue
		}
		for _, adapter := range strings.Split(label.GetValue(), ",") {
			if strings.TrimSpace(adapter) == model {
				return 1, true
			}
		}
		return 0, true
	}
	return 0, true
}
