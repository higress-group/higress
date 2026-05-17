// Copyright (c) 2022 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

type LogExporter struct {
	level string
}

type PrometheusExporter struct {
	namespace string
	subsystem string
	model     string
}

// CacheExporter handles cache hit/miss statistics export
type CacheExporter struct {
	namespace string
	subsystem string
}

// metricCounter defines the minimal interface we need from metrics.
type metricCounter interface {
	Increment(uint64)
}

// Global cache for defined counters (WASM is single-threaded, no lock needed)
var metricCache = make(map[string]metricCounter)

// getOrDefineCounter returns an existing counter if cached, otherwise
// defines it via proxywasm.DefineCounterMetric.
func getOrDefineCounter(name string) metricCounter {
	if c, ok := metricCache[name]; ok {
		return c
	}

	// Attempt to define counter, protect from hostcall panics
	defer func() {
		if r := recover(); r != nil {
			proxywasm.LogWarnf("[token-statistics] DefineCounterMetric panic for %s: %v", name, r)
		}
	}()

	c := proxywasm.DefineCounterMetric(name)
	metricCache[name] = c
	return c
}

func NewPrometheusExporter(namespace, subsystem, model string) *PrometheusExporter {
	return &PrometheusExporter{
		namespace: namespace,
		subsystem: subsystem,
		model:     model,
	}
}

func NewCacheExporter(namespace, subsystem string) *CacheExporter {
	return &CacheExporter{
		namespace: namespace,
		subsystem: subsystem,
	}
}

func (p *PrometheusExporter) Export(usage *TokenUsage) {
	// 创建指标名称（使用字符串拼接，避免 fmt 依赖）
	inputMetricName := p.namespace + "_" + p.subsystem + "_" + p.model + "_input_tokens_total"
	outputMetricName := p.namespace + "_" + p.subsystem + "_" + p.model + "_output_tokens_total"
	totalMetricName := p.namespace + "_" + p.subsystem + "_" + p.model + "_total_tokens_total"

	// 定义（若尚未定义）并更新指标（懒初始化）
	inputCounter := getOrDefineCounter(inputMetricName)
	outputCounter := getOrDefineCounter(outputMetricName)
	totalCounter := getOrDefineCounter(totalMetricName)

	inputCounter.Increment(uint64(usage.InputTokens))
	outputCounter.Increment(uint64(usage.OutputTokens))
	totalCounter.Increment(uint64(usage.TotalTokens))
}

// ExportCacheStatus exports cache hit/miss statistics
func (c *CacheExporter) ExportCacheStatus(status string) {
	if status == "" {
		return
	}

	// Increment total request counter
	totalMetricName := c.namespace + "_" + c.subsystem + "_requests_total"
	totalCounter := getOrDefineCounter(totalMetricName)
	totalCounter.Increment(1)

	// Increment specific cache status counter
	switch status {
	case "hit":
		proxywasm.LogDebugf("[token-statistics] cache status: hit")
		hitMetricName := c.namespace + "_" + c.subsystem + "_hits_total"
		hitCounter := getOrDefineCounter(hitMetricName)
		hitCounter.Increment(1)
	case "miss":
		proxywasm.LogDebugf("[token-statistics] cache status: miss")
		missMetricName := c.namespace + "_" + c.subsystem + "_misses_total"
		missCounter := getOrDefineCounter(missMetricName)
		missCounter.Increment(1)
	default:
		proxywasm.LogWarnf("[token-statistics] unknown cache status: %s", status)
	}
}

// 日志输出

func (l *LogExporter) Export(ctx wrapper.HttpContext, model string, usage *TokenUsage) {
	proxywasm.LogInfof("[token-statistics] model=%s, input_tokens=%d, output_tokens=%d, total_tokens=%d",
		model, usage.InputTokens, usage.OutputTokens, usage.TotalTokens)
}
