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

// Package sglang provides sglang specific pod metrics implementation.
package sglang

import (
	"fmt"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-load-balancer/endpoint_metrics/backend"

	dto "github.com/prometheus/client_model/go"
	"go.uber.org/multierr"
)

const (
	// SGLang exposes its scheduler state as standard Prometheus gauges when the
	// server is started with `--enable-metrics`.
	// See https://docs.sglang.io/references/production_metrics.html
	RunningQueueSizeMetricName    = "sglang:num_running_reqs"
	WaitingQueueSizeMetricName    = "sglang:num_queue_reqs"
	KVCacheUsagePercentMetricName = "sglang:token_usage"
)

// PromToPodMetrics updates internal pod metrics with scraped prometheus metrics.
// A combined error is returned if errors occur in one or more metric processing.
// It returns a new PodMetrics pointer which can be used to atomically update the
// pod metrics map.
//
// SGLang has no LoRA-adapter metric analogous to vLLM's `vllm:lora_requests_info`,
// so ActiveModels is left unpopulated; the default metric policy works on queue
// sizes and KV cache usage.
func PromToPodMetrics(
	metricFamilies map[string]*dto.MetricFamily,
	existing *backend.PodMetrics,
) (*backend.PodMetrics, error) {
	var errs error
	updated := existing.Clone()
	// User selected metric
	if updated.MetricName != "" {
		metricValue, err := getLatestMetric(metricFamilies, updated.MetricName)
		errs = multierr.Append(errs, err)
		if err == nil {
			updated.MetricValue = metricValue.GetGauge().GetValue()
		}
		return updated, errs
	}
	// Default metric
	runningQueueSize, err := getLatestMetric(metricFamilies, RunningQueueSizeMetricName)
	errs = multierr.Append(errs, err)
	if err == nil {
		updated.RunningQueueSize = int(runningQueueSize.GetGauge().GetValue())
	}
	waitingQueueSize, err := getLatestMetric(metricFamilies, WaitingQueueSizeMetricName)
	errs = multierr.Append(errs, err)
	if err == nil {
		updated.WaitingQueueSize = int(waitingQueueSize.GetGauge().GetValue())
	}
	cachePercent, err := getLatestMetric(metricFamilies, KVCacheUsagePercentMetricName)
	errs = multierr.Append(errs, err)
	if err == nil {
		updated.KVCacheUsagePercent = cachePercent.GetGauge().GetValue()
	}

	return updated, errs
}

// getLatestMetric gets the latest metric of a family. This should be used to get the latest Gauge metric.
// Since sglang doesn't set the timestamp in metric, this metric essentially gets the first metric.
func getLatestMetric(metricFamilies map[string]*dto.MetricFamily, metricName string) (*dto.Metric, error) {
	mf, ok := metricFamilies[metricName]
	if !ok {
		return nil, fmt.Errorf("metric family %q not found", metricName)
	}
	if len(mf.GetMetric()) == 0 {
		return nil, fmt.Errorf("no metrics available for %q", metricName)
	}
	var latestTs int64
	var latest *dto.Metric
	for _, m := range mf.GetMetric() {
		if m.GetTimestampMs() >= latestTs {
			latestTs = m.GetTimestampMs()
			latest = m
		}
	}
	return latest, nil
}
