package main

import (
	"testing"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-endpoint-picker/scheduling"
	"github.com/tidwall/gjson"
)

func TestParseConfigDefaults(t *testing.T) {
	var config Config
	if err := parseConfig(gjson.Parse(`{}`), &config); err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if config.profile != defaultProfile || config.ewmaAlpha != 0.2 || config.sampleRate != 0 {
		t.Fatalf("unexpected defaults: %+v", config)
	}
	want := scheduling.Weights{
		scheduling.SignalQueue: 2, scheduling.SignalKVCache: 2,
		scheduling.SignalPrefixCache: 3, scheduling.SignalLoRAAffinity: 0,
		scheduling.SignalInflight: 0, scheduling.SignalFailure: 0,
	}
	for signal, weight := range want {
		if config.weights[signal] != weight {
			t.Errorf("weight %s = %v, want %v", signal, config.weights[signal], weight)
		}
	}
}

func TestParseConfigBalancedAliasesDefault(t *testing.T) {
	var defaultConfig, balancedConfig Config
	if err := parseConfig(gjson.Parse(`{}`), &defaultConfig); err != nil {
		t.Fatal(err)
	}
	if err := parseConfig(gjson.Parse(`{"profile":"balanced"}`), &balancedConfig); err != nil {
		t.Fatal(err)
	}
	for signal, weight := range defaultConfig.weights {
		if balancedConfig.weights[signal] != weight {
			t.Fatalf("balanced weight %s=%v want default %v", signal, balancedConfig.weights[signal], weight)
		}
	}
}

func TestParseConfigOverrides(t *testing.T) {
	var config Config
	err := parseConfig(gjson.Parse(`{
        "profile":"balanced",
        "weights":{"queue":0,"kvCache":3,"prefixCache":0,"loraAffinity":0,"inflight":2,"failure":0},
        "feedback":{"ewmaAlpha":0.5},
        "picker":{"mode":"max-score"},
        "debug":{"sampleRate":1}
    }`), &config)
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if config.weights[scheduling.SignalKVCache] != 3 || config.weights[scheduling.SignalInflight] != 2 {
		t.Fatalf("weights not applied: %+v", config.weights)
	}
	if config.ewmaAlpha != 0.5 || config.sampleRate != 1 {
		t.Fatalf("scalar overrides not applied: %+v", config)
	}
}

func TestParseConfigRejectsInvalidValues(t *testing.T) {
	tests := []string{
		`{"profile":"latency"}`,
		`{"weights":"queue"}`,
		`{"weights":{"queue":-1}}`,
		`{"weights":{"queue":0,"kvCache":0,"prefixCache":0,"loraAffinity":0,"inflight":0,"failure":0}}`,
		`{"feedback":"invalid"}`,
		`{"feedback":{"ewmaAlpha":0}}`,
		`{"feedback":{"ewmaAlpha":1.1}}`,
		`{"picker":"invalid"}`,
		`{"picker":{"mode":"random"}}`,
		`{"debug":"invalid"}`,
		`{"debug":{"sampleRate":-0.1}}`,
		`{"debug":{"sampleRate":1.1}}`,
	}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			if err := parseConfig(gjson.Parse(input), &Config{}); err == nil {
				t.Fatal("parseConfig() succeeded, want error")
			}
		})
	}
}
