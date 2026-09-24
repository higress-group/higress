package scheduling

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/cespare/xxhash/v2"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
)

const (
	queueMetric             = "vllm:num_requests_waiting"
	currentKVMetric         = "vllm:kv_cache_usage_perc"
	legacyKVMetric          = "vllm:gpu_cache_usage_perc"
	loraMetric              = "vllm:lora_requests_info"
	cacheConfigMetric       = "vllm:cache_config_info"
	loraAdaptersLabel       = "running_lora_adapters"
	MaxRelevantMetricsBytes = 64 << 10
	initialRelevantCapacity = 256
)

var ErrRelevantMetricsTooLarge = errors.New("relevant metrics subset exceeds 64 KiB")

type CacheConfig struct {
	BlockSize    int
	NumGPUBlocks int
}

type VLLMMetrics struct {
	BaseSignals   map[SignalName]SignalValue
	CacheConfig   CacheConfig
	LoRAAdapters  []string
	LoRAAvailable bool
}

// ParseVLLMSignals parses one endpoint's Prometheus snapshot. A missing metric
// family is a valid partial snapshot; malformed exposition is not.
func ParseVLLMSignals(metrics, model string) (map[SignalName]SignalValue, error) {
	parsed, err := ParseVLLMMetrics(metrics)
	if err != nil {
		return nil, err
	}
	return parsed.SignalsForModel(model), nil
}

func ParseVLLMMetrics(metrics string) (VLLMMetrics, error) {
	if strings.TrimSpace(metrics) == "" {
		return emptyVLLMMetrics(), nil
	}
	relevant, _, err := CompactVLLMMetrics(metrics)
	if err != nil {
		return VLLMMetrics{}, err
	}
	return ParseCompactVLLMMetrics(relevant)
}

// CompactVLLMMetrics retains only metric families consumed by the picker and
// fingerprints that compact subset. Unrelated metric churn therefore does not
// force another Prometheus parse.
func CompactVLLMMetrics(metrics string) ([]byte, uint64, error) {
	relevant, err := relevantMetricsSubset(metrics)
	if err != nil {
		return nil, 0, err
	}
	return relevant, xxhash.Sum64(relevant), nil
}

// ParseCompactVLLMMetrics parses output returned by CompactVLLMMetrics.
func ParseCompactVLLMMetrics(relevant []byte) (VLLMMetrics, error) {
	result := emptyVLLMMetrics()
	if len(relevant) == 0 {
		return result, nil
	}

	var parser expfmt.TextParser
	families, err := parser.TextToMetricFamilies(bytes.NewReader(relevant))
	if err != nil {
		return VLLMMetrics{}, fmt.Errorf("parse prometheus metrics: %w", err)
	}

	if value, ok := latestFiniteValue(families[queueMetric]); ok && value >= 0 {
		result.BaseSignals[SignalQueue] = available(value, queueMetric)
	}
	if value, ok := latestFiniteValue(families[currentKVMetric]); ok {
		result.BaseSignals[SignalKVCache] = available(value, currentKVMetric)
	} else if value, ok := latestFiniteValue(families[legacyKVMetric]); ok {
		result.BaseSignals[SignalKVCache] = available(value, legacyKVMetric)
	}
	if family := families[loraMetric]; family != nil {
		result.LoRAAdapters, result.LoRAAvailable = runningLoRAAdapters(family)
	}
	result.CacheConfig = cacheConfig(families[cacheConfigMetric])
	return result, nil
}

func emptyVLLMMetrics() VLLMMetrics {
	return VLLMMetrics{BaseSignals: map[SignalName]SignalValue{}}
}

func (metrics VLLMMetrics) SignalsForModel(model string) map[SignalName]SignalValue {
	signals := make(map[SignalName]SignalValue, len(metrics.BaseSignals)+1)
	for name, value := range metrics.BaseSignals {
		signals[name] = value
	}
	if metrics.LoRAAvailable {
		affinity := 0.0
		for _, adapter := range metrics.LoRAAdapters {
			if adapter == model {
				affinity = 1
				break
			}
		}
		signals[SignalLoRAAffinity] = available(affinity, loraMetric)
	}
	return signals
}

func relevantMetricsSubset(metrics string) ([]byte, error) {
	capacity := len(metrics)
	if capacity > initialRelevantCapacity {
		capacity = initialRelevantCapacity
	}
	result := make([]byte, 0, capacity)
	for start := 0; start < len(metrics); {
		end := strings.IndexByte(metrics[start:], '\n')
		if end < 0 {
			end = len(metrics)
		} else {
			end += start
		}
		line := metrics[start:end]
		if isRelevantMetricLine(line) {
			if len(result)+len(line)+1 > MaxRelevantMetricsBytes {
				return nil, ErrRelevantMetricsTooLarge
			}
			result = append(result, line...)
			result = append(result, '\n')
		}
		if end == len(metrics) {
			break
		}
		start = end + 1
	}
	return result, nil
}

func isRelevantMetricLine(line string) bool {
	line = strings.TrimLeft(line, " \t\r")
	if line == "" {
		return false
	}
	if line[0] == '#' {
		for _, directive := range []string{"# HELP ", "# TYPE ", "# UNIT "} {
			if strings.HasPrefix(line, directive) {
				return isRelevantMetricName(metricToken(line[len(directive):]))
			}
		}
		return false
	}
	return isRelevantMetricName(metricToken(line))
}

func metricToken(line string) string {
	end := 0
	for end < len(line) {
		switch line[end] {
		case '{', ' ', '\t', '\r':
			return line[:end]
		default:
			end++
		}
	}
	return line
}

func isRelevantMetricName(name string) bool {
	switch name {
	case queueMetric, currentKVMetric, legacyKVMetric, loraMetric, cacheConfigMetric:
		return true
	default:
		return false
	}
}

func cacheConfig(family *dto.MetricFamily) CacheConfig {
	metric := latestMetric(family)
	if metric == nil {
		return CacheConfig{}
	}
	var config CacheConfig
	for _, label := range metric.Label {
		value, err := strconv.Atoi(label.GetValue())
		if err != nil || value <= 0 {
			continue
		}
		switch label.GetName() {
		case "block_size":
			config.BlockSize = value
		case "num_gpu_blocks":
			config.NumGPUBlocks = value
		}
	}
	return config
}

func available(value float64, source string) SignalValue {
	return SignalValue{Value: value, Available: true, Confidence: 1, Source: source}
}

func latestFiniteValue(family *dto.MetricFamily) (float64, bool) {
	selected := latestMetric(family)
	value, ok := metricValue(selected)
	return value, ok && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func latestMetric(family *dto.MetricFamily) *dto.Metric {
	if family == nil || len(family.Metric) == 0 {
		return nil
	}
	var selected *dto.Metric
	for _, metric := range family.Metric {
		if selected == nil || metric.GetTimestampMs() >= selected.GetTimestampMs() {
			selected = metric
		}
	}
	return selected
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

func runningLoRAAdapters(family *dto.MetricFamily) ([]string, bool) {
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
		return nil, false
	}
	for _, label := range selected.Label {
		if label.GetName() != loraAdaptersLabel {
			continue
		}
		adapters := make([]string, 0)
		for _, adapter := range strings.Split(label.GetValue(), ",") {
			adapter = strings.TrimSpace(adapter)
			if adapter != "" {
				adapters = append(adapters, adapter)
			}
		}
		return adapters, true
	}
	return nil, true
}
