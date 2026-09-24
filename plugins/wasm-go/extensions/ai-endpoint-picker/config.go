package main

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-endpoint-picker/prefixcache"
	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-endpoint-picker/scheduling"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/tidwall/gjson"
)

const (
	defaultProfile                 = "default"
	balancedProfile                = "balanced"
	maxScorePicker                 = "max-score"
	defaultMaxRequestBodyBytes     = 4 << 20
	maxRequestBodyBytesLimit       = 100 << 20
	defaultVMRebuildThresholdBytes = 200 << 20
	maxVMRebuildThresholdBytes     = 4 << 30
	maxCacheBlocksPerEndpointLimit = 1 << 20
)

type Config struct {
	profile                   string
	weights                   scheduling.Weights
	ewmaAlpha                 float64
	sampleRate                float64
	toolMode                  prefixcache.ToolMode
	maxBlocks                 int
	blockSizeTokens           int
	maxCacheBlocksPerEndpoint int
	maxRequestBodyBytes       uint32
	vmRebuildThresholdBytes   uint64
	store                     *scheduling.FeedbackStore
	pipeline                  *scheduling.Pipeline
	prefix                    *prefixcache.Index
	metrics                   *pluginMetrics
	hostCache                 *endpointSnapshotCache
	random                    *rand.Rand
}

func parseConfig(json gjson.Result, config *Config) error {
	profile := json.Get("profile").String()
	if profile == "" {
		profile = defaultProfile
	}
	if profile != defaultProfile && profile != balancedProfile {
		return fmt.Errorf("unsupported profile %q", profile)
	}

	weights := scheduling.Weights{
		scheduling.SignalQueue:        2,
		scheduling.SignalKVCache:      2,
		scheduling.SignalPrefixCache:  3,
		scheduling.SignalLoRAAffinity: 0,
		scheduling.SignalInflight:     1,
		scheduling.SignalFailure:      0,
	}
	weightsJSON := json.Get("weights")
	if weightsJSON.Exists() && !weightsJSON.IsObject() {
		return fmt.Errorf("weights must be an object")
	}
	weightFields := []struct {
		field  string
		signal scheduling.SignalName
	}{
		{"queue", scheduling.SignalQueue},
		{"kvCache", scheduling.SignalKVCache},
		{"prefixCache", scheduling.SignalPrefixCache},
		{"loraAffinity", scheduling.SignalLoRAAffinity},
		{"inflight", scheduling.SignalInflight},
		{"failure", scheduling.SignalFailure},
	}
	positive := false
	for _, item := range weightFields {
		value := weightsJSON.Get(item.field)
		if value.Exists() {
			if value.Type != gjson.Number || value.Float() < 0 || !finite(value.Float()) {
				return fmt.Errorf("weight %s must be a non-negative finite number", item.field)
			}
			weights[item.signal] = value.Float()
		}
		positive = positive || weights[item.signal] > 0
	}
	if !positive {
		return fmt.Errorf("at least one weight must be greater than zero")
	}

	feedbackJSON := json.Get("feedback")
	if feedbackJSON.Exists() && !feedbackJSON.IsObject() {
		return fmt.Errorf("feedback must be an object")
	}
	ewmaAlpha := 0.2
	if value := json.Get("feedback.ewmaAlpha"); value.Exists() {
		if value.Type != gjson.Number || !finite(value.Float()) || value.Float() <= 0 || value.Float() > 1 {
			return fmt.Errorf("feedback.ewmaAlpha must be in (0,1]")
		}
		ewmaAlpha = value.Float()
	}
	pickerJSON := json.Get("picker")
	if pickerJSON.Exists() && !pickerJSON.IsObject() {
		return fmt.Errorf("picker must be an object")
	}
	picker := json.Get("picker.mode").String()
	if picker == "" {
		picker = maxScorePicker
	}
	if picker != maxScorePicker {
		return fmt.Errorf("unsupported picker mode %q", picker)
	}
	debugJSON := json.Get("debug")
	if debugJSON.Exists() && !debugJSON.IsObject() {
		return fmt.Errorf("debug must be an object")
	}
	sampleRate := 0.0
	if value := json.Get("debug.sampleRate"); value.Exists() {
		if value.Type != gjson.Number || !finite(value.Float()) || value.Float() < 0 || value.Float() > 1 {
			return fmt.Errorf("debug.sampleRate must be in [0,1]")
		}
		sampleRate = value.Float()
	}
	prefixJSON := json.Get("prefix")
	if prefixJSON.Exists() && !prefixJSON.IsObject() {
		return fmt.Errorf("prefix must be an object")
	}
	toolMode := prefixcache.DefaultToolMode
	if value := json.Get("prefix.toolMode"); value.Exists() {
		if value.Type != gjson.String {
			return fmt.Errorf("prefix.toolMode must be one of none, identity, full")
		}
		toolMode = prefixcache.ToolMode(value.String())
	}
	if toolMode != prefixcache.ToolModeNone && toolMode != prefixcache.ToolModeIdentity && toolMode != prefixcache.ToolModeFull {
		return fmt.Errorf("unsupported prefix.toolMode %q", toolMode)
	}
	maxBlocks := prefixcache.DefaultMaxBlocks
	if value := json.Get("prefix.maxBlocks"); value.Exists() {
		if value.Type != gjson.Number || value.Float() != math.Trunc(value.Float()) || value.Int() < 1 || value.Int() > prefixcache.MaxBlocksLimit {
			return fmt.Errorf("prefix.maxBlocks must be an integer in [1,%d]", prefixcache.MaxBlocksLimit)
		}
		maxBlocks = int(value.Int())
	}
	blockSizeTokens := prefixcache.DefaultBlockSizeTokens
	if value := json.Get("prefix.blockSizeTokens"); value.Exists() {
		if value.Type != gjson.Number || value.Float() != math.Trunc(value.Float()) || value.Int() < 1 || value.Int() > prefixcache.MaxSegmentTokens {
			return fmt.Errorf("prefix.blockSizeTokens must be an integer in [1,%d]", prefixcache.MaxSegmentTokens)
		}
		blockSizeTokens = int(value.Int())
	}
	maxCacheBlocksPerEndpoint := prefixcache.DefaultCapacity
	if value := json.Get("prefix.maxCacheBlocksPerEndpoint"); value.Exists() {
		if value.Type != gjson.Number || value.Float() != math.Trunc(value.Float()) || value.Int() < 1 || value.Int() > maxCacheBlocksPerEndpointLimit {
			return fmt.Errorf("prefix.maxCacheBlocksPerEndpoint must be an integer in [1,%d]", maxCacheBlocksPerEndpointLimit)
		}
		maxCacheBlocksPerEndpoint = int(value.Int())
	}
	limitsJSON := json.Get("limits")
	if limitsJSON.Exists() && !limitsJSON.IsObject() {
		return fmt.Errorf("limits must be an object")
	}
	maxRequestBodyBytes := uint32(defaultMaxRequestBodyBytes)
	if value := json.Get("limits.maxRequestBodyBytes"); value.Exists() {
		if value.Type != gjson.Number || value.Float() != math.Trunc(value.Float()) || value.Int() < 1 || value.Int() > maxRequestBodyBytesLimit {
			return fmt.Errorf("limits.maxRequestBodyBytes must be an integer in [1,%d]", maxRequestBodyBytesLimit)
		}
		maxRequestBodyBytes = uint32(value.Int())
	}
	vmRebuildThresholdBytes := uint64(defaultVMRebuildThresholdBytes)
	if value := json.Get("limits.vmRebuildThresholdBytes"); value.Exists() {
		if value.Type != gjson.Number || value.Float() != math.Trunc(value.Float()) || value.Int() < 0 || value.Uint() > maxVMRebuildThresholdBytes {
			return fmt.Errorf("limits.vmRebuildThresholdBytes must be zero or an integer in [1,%d]", uint64(maxVMRebuildThresholdBytes))
		}
		vmRebuildThresholdBytes = value.Uint()
	}

	random := rand.New(rand.NewSource(time.Now().UnixNano()))
	config.profile = profile
	config.weights = weights
	config.ewmaAlpha = ewmaAlpha
	config.sampleRate = sampleRate
	config.toolMode = toolMode
	config.maxBlocks = maxBlocks
	config.blockSizeTokens = blockSizeTokens
	config.maxCacheBlocksPerEndpoint = maxCacheBlocksPerEndpoint
	config.maxRequestBodyBytes = maxRequestBodyBytes
	config.vmRebuildThresholdBytes = vmRebuildThresholdBytes
	config.store = scheduling.NewFeedbackStore(ewmaAlpha)
	config.pipeline = scheduling.NewPipeline(weights, random)
	config.prefix = prefixcache.NewIndex(maxCacheBlocksPerEndpoint)
	config.metrics = &pluginMetrics{}
	config.hostCache = newEndpointSnapshotCache(proxywasm.GetUpstreamHosts, scheduling.ParseCompactVLLMMetrics, time.Now)
	config.random = random
	return nil
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
