package main

import (
	"fmt"
	"testing"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-endpoint-picker/prefixcache"
	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-endpoint-picker/scheduling"
	"github.com/tidwall/gjson"
)

func TestParseConfigDefaults(t *testing.T) {
	var config Config
	if err := parseConfig(gjson.Parse(`{}`), &config); err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if config.profile != defaultProfile || config.ewmaAlpha != 0.2 || config.sampleRate != 0 || config.toolMode != prefixcache.ToolModeIdentity ||
		config.maxBlocks != prefixcache.DefaultMaxBlocks || config.blockSizeTokens != prefixcache.DefaultBlockSizeTokens || config.maxCacheBlocksPerEndpoint != prefixcache.DefaultCapacity ||
		config.maxRequestBodyBytes != defaultMaxRequestBodyBytes || config.vmRebuildThresholdBytes != defaultVMRebuildThresholdBytes {
		t.Fatalf("unexpected defaults: %+v", config)
	}
	want := scheduling.Weights{
		scheduling.SignalQueue: 2, scheduling.SignalKVCache: 2,
		scheduling.SignalPrefixCache: 3, scheduling.SignalLoRAAffinity: 0,
		scheduling.SignalInflight: 1, scheduling.SignalFailure: 0,
	}
	for signal, weight := range want {
		if config.weights[signal] != weight {
			t.Errorf("weight %s = %v, want %v", signal, config.weights[signal], weight)
		}
	}
}

func TestParseConfigBlockSizeTokens(t *testing.T) {
	for _, blockSizeTokens := range []int{64, 128, prefixcache.DefaultBlockSizeTokens} {
		t.Run(fmt.Sprint(blockSizeTokens), func(t *testing.T) {
			var config Config
			if err := parseConfig(gjson.Parse(`{"prefix":{"blockSizeTokens":`+fmt.Sprint(blockSizeTokens)+`}}`), &config); err != nil {
				t.Fatalf("parseConfig() error = %v", err)
			}
			if config.blockSizeTokens != blockSizeTokens {
				t.Fatalf("blockSizeTokens=%d want %d", config.blockSizeTokens, blockSizeTokens)
			}
		})
	}
}

func TestDefaultInflightWeightSelectsLessBusyEndpoint(t *testing.T) {
	var config Config
	if err := parseConfig(gjson.Parse(`{}`), &config); err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	decision := config.pipeline.Schedule([]scheduling.EndpointSnapshot{
		{
			Address: "busy",
			Healthy: true,
			Signals: map[scheduling.SignalName]scheduling.SignalValue{
				scheduling.SignalInflight: {Value: 2, Available: true, Confidence: 1},
			},
		},
		{
			Address: "idle",
			Healthy: true,
			Signals: map[scheduling.SignalName]scheduling.SignalValue{
				scheduling.SignalInflight: {Value: 0, Available: true, Confidence: 1},
			},
		},
	})
	if decision.Address != "idle" {
		t.Fatalf("selected %q, want idle with the default inflight weight", decision.Address)
	}
}

func TestParseConfigMaxBlocks(t *testing.T) {
	for _, maxBlocks := range []int{1, 64, prefixcache.MaxBlocksLimit} {
		t.Run(fmt.Sprint(maxBlocks), func(t *testing.T) {
			var config Config
			if err := parseConfig(gjson.Parse(`{"prefix":{"maxBlocks":`+fmt.Sprint(maxBlocks)+`}}`), &config); err != nil {
				t.Fatalf("parseConfig() error = %v", err)
			}
			if config.maxBlocks != maxBlocks {
				t.Fatalf("maxBlocks=%d want %d", config.maxBlocks, maxBlocks)
			}
		})
	}
}

func TestParseConfigToolModes(t *testing.T) {
	for _, mode := range []prefixcache.ToolMode{
		prefixcache.ToolModeNone,
		prefixcache.ToolModeIdentity,
		prefixcache.ToolModeFull,
	} {
		t.Run(string(mode), func(t *testing.T) {
			var config Config
			if err := parseConfig(gjson.Parse(`{"prefix":{"toolMode":"`+string(mode)+`"}}`), &config); err != nil {
				t.Fatalf("parseConfig() error = %v", err)
			}
			if config.toolMode != mode {
				t.Fatalf("toolMode=%q want %q", config.toolMode, mode)
			}
		})
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
		"prefix":{"maxCacheBlocksPerEndpoint":4096},
		"limits":{"maxRequestBodyBytes":1048576,"vmRebuildThresholdBytes":0},
        "debug":{"sampleRate":1}
    }`), &config)
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if config.weights[scheduling.SignalKVCache] != 3 || config.weights[scheduling.SignalInflight] != 2 {
		t.Fatalf("weights not applied: %+v", config.weights)
	}
	if config.ewmaAlpha != 0.5 || config.sampleRate != 1 || config.maxCacheBlocksPerEndpoint != 4096 ||
		config.maxRequestBodyBytes != 1048576 || config.vmRebuildThresholdBytes != 0 {
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
		`{"prefix":"invalid"}`,
		`{"prefix":{"toolMode":"approximate"}}`,
		`{"prefix":{"toolMode":1}}`,
		`{"prefix":{"toolMode":null}}`,
		`{"prefix":{"maxBlocks":0}}`,
		`{"prefix":{"maxBlocks":129}}`,
		`{"prefix":{"maxBlocks":1.5}}`,
		`{"prefix":{"maxBlocks":"32"}}`,
		`{"prefix":{"blockSizeTokens":0}}`,
		`{"prefix":{"blockSizeTokens":1025}}`,
		`{"prefix":{"blockSizeTokens":1.5}}`,
		`{"prefix":{"blockSizeTokens":"128"}}`,
		`{"prefix":{"maxCacheBlocksPerEndpoint":0}}`,
		`{"prefix":{"maxCacheBlocksPerEndpoint":1048577}}`,
		`{"limits":"invalid"}`,
		`{"limits":{"maxRequestBodyBytes":0}}`,
		`{"limits":{"maxRequestBodyBytes":104857601}}`,
		`{"limits":{"vmRebuildThresholdBytes":-1}}`,
		`{"limits":{"vmRebuildThresholdBytes":4294967297}}`,
	}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			if err := parseConfig(gjson.Parse(input), &Config{}); err == nil {
				t.Fatal("parseConfig() succeeded, want error")
			}
		})
	}
}
