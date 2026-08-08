package scheduling

import "testing"

func TestParseVLLMSignals(t *testing.T) {
	metrics := `
# TYPE vllm:num_requests_waiting gauge
vllm:num_requests_waiting 7
# TYPE vllm:kv_cache_usage_perc gauge
vllm:kv_cache_usage_perc 0.25
# TYPE vllm:gpu_cache_usage_perc gauge
vllm:gpu_cache_usage_perc 0.9
# TYPE vllm:lora_requests_info gauge
vllm:lora_requests_info{running_lora_adapters="base, adapter-a",max_lora="4"} 123
`
	signals, err := ParseVLLMSignals(metrics, "adapter-a")
	if err != nil {
		t.Fatalf("ParseVLLMSignals() error = %v", err)
	}
	if signals[SignalQueue].Value != 7 {
		t.Errorf("queue = %v, want 7", signals[SignalQueue].Value)
	}
	if got := signals[SignalKVCache]; got.Value != 0.25 || got.Source != currentKVMetric {
		t.Errorf("kv = %+v, want current metric value 0.25", got)
	}
	if signals[SignalLoRAAffinity].Value != 1 {
		t.Errorf("lora affinity = %v, want 1", signals[SignalLoRAAffinity].Value)
	}
}

func TestParseVLLMSignalsLegacyKVAndOptionalLoRA(t *testing.T) {
	signals, err := ParseVLLMSignals("# TYPE vllm:gpu_cache_usage_perc gauge\nvllm:gpu_cache_usage_perc 0.4\n", "model")
	if err != nil {
		t.Fatalf("ParseVLLMSignals() error = %v", err)
	}
	if got := signals[SignalKVCache]; got.Value != 0.4 || got.Source != legacyKVMetric {
		t.Errorf("kv = %+v, want legacy metric", got)
	}
	if _, ok := signals[SignalLoRAAffinity]; ok {
		t.Fatal("missing LoRA family must remain unavailable")
	}
}

func TestParseVLLMSignalsPartialAndMalformed(t *testing.T) {
	signals, err := ParseVLLMSignals("# TYPE vllm:num_requests_waiting gauge\nvllm:num_requests_waiting 2\n", "model")
	if err != nil || len(signals) != 1 || !signals[SignalQueue].Available {
		t.Fatalf("partial snapshot = %+v, %v", signals, err)
	}
	if _, err := ParseVLLMSignals("vllm:num_requests_waiting not-a-number\n", "model"); err == nil {
		t.Fatal("malformed exposition succeeded, want error")
	}
}

func TestParseVLLMSignalsStaleMetricDoesNotHideFreshSignal(t *testing.T) {
	metrics := "# TYPE vllm:num_requests_waiting gauge\nvllm:num_requests_waiting NaN\n" +
		"# TYPE vllm:kv_cache_usage_perc gauge\nvllm:kv_cache_usage_perc 0.3\n"
	signals, err := ParseVLLMSignals(metrics, "model")
	if err != nil {
		t.Fatalf("ParseVLLMSignals() error = %v", err)
	}
	if _, ok := signals[SignalQueue]; ok {
		t.Fatal("stale queue marker must make only queue unavailable")
	}
	if got := signals[SignalKVCache]; !got.Available || got.Value != 0.3 {
		t.Fatalf("fresh KV signal = %+v, want available 0.3", got)
	}
}
