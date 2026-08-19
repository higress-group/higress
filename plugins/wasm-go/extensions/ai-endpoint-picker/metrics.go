package main

import "github.com/higress-group/proxy-wasm-go-sdk/proxywasm"

type pluginMetrics struct {
	initialized   bool
	decisions     proxywasm.MetricCounter
	fallbacks     proxywasm.MetricCounter
	missingSignal proxywasm.MetricCounter
	feedback      proxywasm.MetricCounter
	inflight      proxywasm.MetricGauge
}

func (m *pluginMetrics) ensure() {
	if m.initialized {
		return
	}
	m.decisions = proxywasm.DefineCounterMetric("ai_endpoint_picker_decisions_total")
	m.fallbacks = proxywasm.DefineCounterMetric("ai_endpoint_picker_fallback_total")
	m.missingSignal = proxywasm.DefineCounterMetric("ai_endpoint_picker_missing_signal_total")
	m.feedback = proxywasm.DefineCounterMetric("ai_endpoint_picker_feedback_total")
	m.inflight = proxywasm.DefineGaugeMetric("ai_endpoint_picker_inflight")
	m.initialized = true
}

func (m *pluginMetrics) decision() {
	m.ensure()
	m.decisions.Increment(1)
}

func (m *pluginMetrics) fallback() {
	m.ensure()
	m.fallbacks.Increment(1)
}

func (m *pluginMetrics) missing(count uint64) {
	if count == 0 {
		return
	}
	m.ensure()
	m.missingSignal.Increment(count)
}

func (m *pluginMetrics) beginFeedback() {
	m.ensure()
	m.inflight.Add(1)
}

func (m *pluginMetrics) completeFeedback() {
	m.ensure()
	m.feedback.Increment(1)
	m.inflight.Add(-1)
}
