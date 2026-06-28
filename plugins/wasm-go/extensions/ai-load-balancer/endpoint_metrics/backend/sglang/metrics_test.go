/*
Copyright 2025 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package sglang

import (
	"strings"
	"testing"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-load-balancer/endpoint_metrics/backend"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
)

// sample mirrors the gauges from a real SGLang /metrics endpoint
// (https://docs.sglang.io/references/production_metrics.html), trimmed to the
// gauges the load balancer reads plus an unrelated counter that must be ignored.
const sample = `# HELP sglang:prompt_tokens_total Number of prefill tokens processed.
# TYPE sglang:prompt_tokens_total counter
sglang:prompt_tokens_total{model_name="meta-llama/Llama-3.1-8B-Instruct"} 8128902
# HELP sglang:num_running_reqs The number of running requests
# TYPE sglang:num_running_reqs gauge
sglang:num_running_reqs{model_name="meta-llama/Llama-3.1-8B-Instruct"} 162
# HELP sglang:num_queue_reqs The number of requests in the waiting queue
# TYPE sglang:num_queue_reqs gauge
sglang:num_queue_reqs{model_name="meta-llama/Llama-3.1-8B-Instruct"} 2826
# HELP sglang:token_usage The token usage
# TYPE sglang:token_usage gauge
sglang:token_usage{model_name="meta-llama/Llama-3.1-8B-Instruct"} 0.28
`

func parse(t *testing.T, metrics string) map[string]*dto.MetricFamily {
	t.Helper()
	parser := expfmt.TextParser{}
	mfs, err := parser.TextToMetricFamilies(strings.NewReader(metrics))
	if err != nil {
		t.Fatalf("failed to parse metrics: %v", err)
	}
	return mfs
}

func newPodMetrics(targetMetric string) *backend.PodMetrics {
	return &backend.PodMetrics{
		Pod:                backend.Pod{Name: "pod-a", Address: "10.0.0.1:30000"},
		Metrics:            backend.Metrics{},
		UserSelectedMetric: backend.UserSelectedMetric{MetricName: targetMetric},
	}
}

func TestPromToPodMetrics_DefaultGauges(t *testing.T) {
	got, err := PromToPodMetrics(parse(t, sample), newPodMetrics(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.RunningQueueSize != 162 {
		t.Errorf("RunningQueueSize: got %d, want 162", got.RunningQueueSize)
	}
	if got.WaitingQueueSize != 2826 {
		t.Errorf("WaitingQueueSize: got %d, want 2826", got.WaitingQueueSize)
	}
	if got.KVCacheUsagePercent != 0.28 {
		t.Errorf("KVCacheUsagePercent: got %v, want 0.28", got.KVCacheUsagePercent)
	}
}

func TestPromToPodMetrics_UserSelectedMetric(t *testing.T) {
	got, err := PromToPodMetrics(parse(t, sample), newPodMetrics(WaitingQueueSizeMetricName))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.MetricValue != 2826 {
		t.Errorf("MetricValue: got %v, want 2826", got.MetricValue)
	}
	// When a target metric is selected, the default gauges are not populated.
	if got.RunningQueueSize != 0 {
		t.Errorf("RunningQueueSize should stay 0 when a target metric is set, got %d", got.RunningQueueSize)
	}
}

func TestPromToPodMetrics_MissingFamilies(t *testing.T) {
	// Only one of the three default gauges is present; the others must error via
	// multierr without aborting parsing of the family that is present.
	const partial = `# TYPE sglang:num_running_reqs gauge
sglang:num_running_reqs{model_name="m"} 7
`
	got, err := PromToPodMetrics(parse(t, partial), newPodMetrics(""))
	if err == nil {
		t.Fatal("expected an error for the missing metric families")
	}
	if got.RunningQueueSize != 7 {
		t.Errorf("RunningQueueSize: got %d, want 7", got.RunningQueueSize)
	}
	if got.WaitingQueueSize != 0 {
		t.Errorf("WaitingQueueSize: got %d, want 0", got.WaitingQueueSize)
	}
}

func TestPromToPodMetrics_UserSelectedMetricMissing(t *testing.T) {
	_, err := PromToPodMetrics(parse(t, sample), newPodMetrics("sglang:does_not_exist"))
	if err == nil {
		t.Fatal("expected an error when the selected metric is absent")
	}
}
