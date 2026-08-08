package main

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-endpoint-picker/prefixcache"
	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-endpoint-picker/scheduling"
	"github.com/tidwall/gjson"
)

const (
	defaultProfile  = "default"
	balancedProfile = "balanced"
	maxScorePicker  = "max-score"
)

type Config struct {
	profile    string
	weights    scheduling.Weights
	ewmaAlpha  float64
	sampleRate float64
	store      *scheduling.FeedbackStore
	pipeline   *scheduling.Pipeline
	prefix     *prefixcache.Index
	metrics    *pluginMetrics
	random     *rand.Rand
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
		scheduling.SignalInflight:     0,
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

	random := rand.New(rand.NewSource(time.Now().UnixNano()))
	config.profile = profile
	config.weights = weights
	config.ewmaAlpha = ewmaAlpha
	config.sampleRate = sampleRate
	config.store = scheduling.NewFeedbackStore(ewmaAlpha)
	config.pipeline = scheduling.NewPipeline(weights, random)
	config.prefix = prefixcache.NewIndex(prefixcache.DefaultCapacity)
	config.metrics = &pluginMetrics{}
	config.random = random
	return nil
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
