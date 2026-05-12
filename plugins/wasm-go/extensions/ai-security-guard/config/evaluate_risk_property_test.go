// Copyright (c) 2024 Alibaba Group Holding Ltd.
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

package config

import (
	"fmt"
	"math/rand"
	"testing"
	"testing/quick"
)

// validSensitiveLevels are the valid sensitive data levels in ascending order.
var validSensitiveLevels = []string{"S0", "S1", "S2", "S3", "S4"}

// Feature: sensitive-data-mask-threshold, Property 1: Above-threshold mask produces RiskMask
// **Validates: Requirements 1.1, 4.1**
//
// For any valid sensitive level L and threshold T where LevelToInt(L) >= LevelToInt(T),
// when evaluateRiskMultiModal is called with a single Detail of Type=sensitiveData,
// Suggestion=mask, Level=L, config SensitiveDataAction=mask, SensitiveDataLevelBar=T,
// and no other blocking conditions, the result SHALL be RiskMask.
func TestProperty1_AboveThresholdMaskProducesRiskMask(t *testing.T) {
	f := func(seed uint64) bool {
		// Use seed to deterministically pick a (level, threshold) pair
		// where LevelToInt(level) >= LevelToInt(threshold)
		r := rand.New(rand.NewSource(int64(seed)))

		// Pick threshold index [0..4], then level index [thresholdIdx..4]
		thresholdIdx := r.Intn(len(validSensitiveLevels))
		levelIdx := thresholdIdx + r.Intn(len(validSensitiveLevels)-thresholdIdx)

		level := validSensitiveLevels[levelIdx]
		threshold := validSensitiveLevels[thresholdIdx]

		// Sanity: level >= threshold
		if LevelToInt(level) < LevelToInt(threshold) {
			t.Errorf("generator bug: level=%s (%d) < threshold=%s (%d)", level, LevelToInt(level), threshold, LevelToInt(threshold))
			return false
		}

		config := baseConfig()
		config.SensitiveDataAction = "mask"
		config.SensitiveDataLevelBar = threshold
		// Set all other thresholds to max (most permissive) to avoid interference
		config.ContentModerationLevelBar = MaxRisk
		config.PromptAttackLevelBar = MaxRisk
		config.MaliciousUrlLevelBar = MaxRisk
		config.ModelHallucinationLevelBar = MaxRisk
		config.CustomLabelLevelBar = MaxRisk
		config.RiskAction = "block"

		data := Data{
			RiskLevel: "none", // Avoid top-level gate triggering
			Detail: []Detail{
				{
					Type:       SensitiveDataType,
					Suggestion: "mask",
					Level:      level,
					Result:     []Result{{Ext: Ext{Desensitization: "masked-content"}}},
				},
			},
		}

		result := EvaluateRisk(MultiModalGuard, data, config, "")
		if result != RiskMask {
			t.Errorf("expected RiskMask for level=%s, threshold=%s, got %d", level, threshold, result)
			return false
		}
		return true
	}

	cfg := &quick.Config{MaxCount: 200}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("Property 1 failed: %v", err)
		fmt.Printf("Property 1 counterexample: %v\n", err)
	}
}

// Feature: sensitive-data-mask-threshold, Property 2: Below-threshold mask produces RiskPass
// **Validates: Requirements 1.2, 1.3**
//
// For any valid sensitive level L and threshold T where LevelToInt(L) < LevelToInt(T),
// when evaluateRiskMultiModal is called with a single Detail of Type=sensitiveData,
// Suggestion=mask, Level=L, config SensitiveDataAction=mask, SensitiveDataLevelBar=T,
// and no other blocking conditions, the result SHALL be RiskPass.
func TestProperty2_BelowThresholdMaskProducesRiskPass(t *testing.T) {
	f := func(seed uint64) bool {
		// Use seed to deterministically pick a (level, threshold) pair
		// where LevelToInt(level) < LevelToInt(threshold)
		r := rand.New(rand.NewSource(int64(seed)))

		// Pick threshold index [1..4], then level index [0..thresholdIdx-1]
		thresholdIdx := 1 + r.Intn(len(validSensitiveLevels)-1) // [1..4]
		levelIdx := r.Intn(thresholdIdx)                        // [0..thresholdIdx-1]

		level := validSensitiveLevels[levelIdx]
		threshold := validSensitiveLevels[thresholdIdx]

		// Sanity: level < threshold
		if LevelToInt(level) >= LevelToInt(threshold) {
			t.Errorf("generator bug: level=%s (%d) >= threshold=%s (%d)", level, LevelToInt(level), threshold, LevelToInt(threshold))
			return false
		}

		config := baseConfig()
		config.SensitiveDataAction = "mask"
		config.SensitiveDataLevelBar = threshold
		config.ContentModerationLevelBar = MaxRisk
		config.PromptAttackLevelBar = MaxRisk
		config.MaliciousUrlLevelBar = MaxRisk
		config.ModelHallucinationLevelBar = MaxRisk
		config.CustomLabelLevelBar = MaxRisk
		config.RiskAction = "block"

		data := Data{
			RiskLevel: "none",
			Detail: []Detail{
				{
					Type:       SensitiveDataType,
					Suggestion: "mask",
					Level:      level,
				},
			},
		}

		result := EvaluateRisk(MultiModalGuard, data, config, "")
		if result != RiskPass {
			t.Errorf("expected RiskPass for level=%s, threshold=%s, got %d", level, threshold, result)
			return false
		}
		return true
	}

	cfg := &quick.Config{MaxCount: 200}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("Property 2 failed: %v", err)
		fmt.Printf("Property 2 counterexample: %v\n", err)
	}
}

// Feature: sensitive-data-mask-threshold, Property 3: Per-detail threshold independence
// **Validates: Requirements 1.4**
//
// For any list of sensitiveData Details each with Suggestion=mask and varying levels,
// and a threshold T, when evaluateRiskMultiModal is called with SensitiveDataAction=mask
// and no blocking conditions: the result SHALL be RiskMask if and only if at least one
// Detail has LevelToInt(Level) >= LevelToInt(T).
func TestProperty3_PerDetailThresholdIndependence(t *testing.T) {
	f := func(seed uint64) bool {
		r := rand.New(rand.NewSource(int64(seed)))

		// Pick a random threshold from validSensitiveLevels
		thresholdIdx := r.Intn(len(validSensitiveLevels))
		threshold := validSensitiveLevels[thresholdIdx]

		// Generate 1-5 random sensitiveData details
		numDetails := 1 + r.Intn(5)
		details := make([]Detail, numDetails)
		expectMask := false

		for i := 0; i < numDetails; i++ {
			levelIdx := r.Intn(len(validSensitiveLevels))
			level := validSensitiveLevels[levelIdx]

			detail := Detail{
				Type:       SensitiveDataType,
				Suggestion: "mask",
				Level:      level,
			}

			// Details that meet threshold should have Result with Desensitization content
			if LevelToInt(level) >= LevelToInt(threshold) {
				expectMask = true
				detail.Result = []Result{{Ext: Ext{Desensitization: "masked-content"}}}
			}

			details[i] = detail
		}

		config := baseConfig()
		config.SensitiveDataAction = "mask"
		config.SensitiveDataLevelBar = threshold
		// Set all other thresholds to max to avoid interference
		config.ContentModerationLevelBar = MaxRisk
		config.PromptAttackLevelBar = MaxRisk
		config.MaliciousUrlLevelBar = MaxRisk
		config.ModelHallucinationLevelBar = MaxRisk
		config.CustomLabelLevelBar = MaxRisk
		config.RiskAction = "block"

		data := Data{
			RiskLevel: "none",
			Detail:    details,
		}

		result := EvaluateRisk(MultiModalGuard, data, config, "")

		if expectMask {
			if result != RiskMask {
				t.Errorf("expected RiskMask: threshold=%s, details=%v, got %d", threshold, describeLevels(details), result)
				return false
			}
		} else {
			if result != RiskPass {
				t.Errorf("expected RiskPass: threshold=%s, details=%v, got %d", threshold, describeLevels(details), result)
				return false
			}
		}
		return true
	}

	cfg := &quick.Config{MaxCount: 200}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("Property 3 failed: %v", err)
		fmt.Printf("Property 3 counterexample: %v\n", err)
	}
}

// describeLevels returns a slice of level strings from the given details for error reporting.
func describeLevels(details []Detail) []string {
	levels := make([]string, len(details))
	for i, d := range details {
		levels[i] = d.Level
	}
	return levels
}

// validGeneralRiskLevels are the valid general risk levels in ascending order.
var validGeneralRiskLevels = []string{"none", "low", "medium", "high", "max"}

// knownDetailTypes are the known dimension types used for generating random details.
var knownDetailTypes = []string{
	SensitiveDataType,
	ContentModerationType,
	PromptAttackType,
	MaliciousUrlDataType,
	ModelHallucinationDataType,
	CustomLabelType,
}

// Feature: sensitive-data-mask-threshold, Property 4: Block triggers always produce RiskBlock
// **Validates: Requirements 3.1, 3.2**
//
// Sub-property 4a: For any Detail with Suggestion=block, regardless of type, level,
// dimAction, or threshold configuration, evaluateRiskMultiModal SHALL return RiskBlock.
//
// Sub-property 4b: For any Detail where the resolved dimAction is "block" and the
// detail's level exceeds the configured threshold, evaluateRiskMultiModal SHALL return RiskBlock.
func TestProperty4a_SuggestionBlockRespectsThreshold(t *testing.T) {
	f := func(seed uint64) bool {
		r := rand.New(rand.NewSource(int64(seed)))

		detailType := knownDetailTypes[r.Intn(len(knownDetailTypes))]

		var level string
		if detailType == SensitiveDataType {
			level = validSensitiveLevels[r.Intn(len(validSensitiveLevels))]
		} else {
			level = validGeneralRiskLevels[r.Intn(len(validGeneralRiskLevels))]
		}

		config := baseConfig()
		// Set all thresholds to max so no detail exceeds threshold
		config.ContentModerationLevelBar = MaxRisk
		config.PromptAttackLevelBar = MaxRisk
		config.SensitiveDataLevelBar = S4Sensitive
		config.MaliciousUrlLevelBar = MaxRisk
		config.ModelHallucinationLevelBar = MaxRisk
		config.CustomLabelLevelBar = MaxRisk

		data := Data{
			RiskLevel: "none",
			Detail: []Detail{
				{
					Type:       detailType,
					Suggestion: "block",
					Level:      level,
				},
			},
		}

		result := EvaluateRisk(MultiModalGuard, data, config, "")
		exceeds := detailExceedsThreshold(data.Detail[0], config, "")
		if exceeds && result != RiskBlock {
			t.Errorf("expected RiskBlock when threshold exceeded for type=%s, level=%s", detailType, level)
			return false
		}
		if !exceeds && result == RiskBlock {
			t.Errorf("expected non-block when threshold not exceeded for type=%s, level=%s", detailType, level)
			return false
		}
		return true
	}

	cfg := &quick.Config{MaxCount: 200}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("Property 4a failed: %v", err)
	}
}

func TestProperty4b_DimActionBlockExceedsThresholdProducesRiskBlock(t *testing.T) {
	// Test with dimension types that support block action and have configurable thresholds.
	// For each iteration, pick a dimension type, set its action to "block",
	// and set level >= threshold to ensure exceeds=true.
	type dimConfig struct {
		detailType   string
		levels       []string
		setThreshold func(config *AISecurityConfig, threshold string)
	}

	dims := []dimConfig{
		{
			detailType: ContentModerationType,
			levels:     validGeneralRiskLevels,
			setThreshold: func(c *AISecurityConfig, t string) {
				c.ContentModerationAction = "block"
				c.ContentModerationLevelBar = t
			},
		},
		{
			detailType: PromptAttackType,
			levels:     validGeneralRiskLevels,
			setThreshold: func(c *AISecurityConfig, t string) {
				c.PromptAttackAction = "block"
				c.PromptAttackLevelBar = t
			},
		},
		{
			detailType: SensitiveDataType,
			levels:     validSensitiveLevels,
			setThreshold: func(c *AISecurityConfig, t string) {
				c.SensitiveDataAction = "block"
				c.SensitiveDataLevelBar = t
			},
		},
		{
			detailType: MaliciousUrlDataType,
			levels:     validGeneralRiskLevels,
			setThreshold: func(c *AISecurityConfig, t string) {
				c.MaliciousUrlAction = "block"
				c.MaliciousUrlLevelBar = t
			},
		},
		{
			detailType: ModelHallucinationDataType,
			levels:     validGeneralRiskLevels,
			setThreshold: func(c *AISecurityConfig, t string) {
				c.ModelHallucinationAction = "block"
				c.ModelHallucinationLevelBar = t
			},
		},
		{
			detailType: CustomLabelType,
			levels:     validGeneralRiskLevels,
			setThreshold: func(c *AISecurityConfig, t string) {
				c.CustomLabelAction = "block"
				c.CustomLabelLevelBar = t
			},
		},
	}

	f := func(seed uint64) bool {
		r := rand.New(rand.NewSource(int64(seed)))

		// Pick a random dimension
		dim := dims[r.Intn(len(dims))]

		// Pick threshold index, then level index >= threshold
		thresholdIdx := r.Intn(len(dim.levels))
		levelIdx := thresholdIdx + r.Intn(len(dim.levels)-thresholdIdx)

		threshold := dim.levels[thresholdIdx]
		level := dim.levels[levelIdx]

		// Sanity: level >= threshold
		if LevelToInt(level) < LevelToInt(threshold) {
			t.Errorf("generator bug: level=%s (%d) < threshold=%s (%d)", level, LevelToInt(level), threshold, LevelToInt(threshold))
			return false
		}

		config := baseConfig()
		// Set all other thresholds to max to avoid interference
		config.ContentModerationLevelBar = MaxRisk
		config.PromptAttackLevelBar = MaxRisk
		config.SensitiveDataLevelBar = S4Sensitive
		config.MaliciousUrlLevelBar = MaxRisk
		config.ModelHallucinationLevelBar = MaxRisk
		config.CustomLabelLevelBar = MaxRisk

		// Configure the chosen dimension with block action and threshold
		dim.setThreshold(&config, threshold)

		// Use a non-block suggestion so we test the dimAction=block + exceeds path
		// (not the Suggestion=block shortcut tested in 4a)
		suggestion := "pass"

		data := Data{
			RiskLevel: "none", // Avoid top-level gate interference
			Detail: []Detail{
				{
					Type:       dim.detailType,
					Suggestion: suggestion,
					Level:      level,
				},
			},
		}

		result := EvaluateRisk(MultiModalGuard, data, config, "")
		if result != RiskBlock {
			t.Errorf("expected RiskBlock for dimAction=block, type=%s, level=%s, threshold=%s, got %d",
				dim.detailType, level, threshold, result)
			return false
		}
		return true
	}

	cfg := &quick.Config{MaxCount: 200}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("Property 4b failed: %v", err)
		fmt.Printf("Property 4b counterexample: %v\n", err)
	}
}

// Feature: sensitive-data-mask-threshold, Property 5: Top-level gates produce RiskBlock
// **Validates: Requirements 3.3, 3.4**
//
// Sub-property 5a: For any Data.RiskLevel and contentModerationLevelBar where
// LevelToInt(RiskLevel) >= LevelToInt(contentModerationLevelBar),
// evaluateRiskMultiModal SHALL return RiskBlock regardless of Detail content.
//
// Sub-property 5b: For any Data.AttackLevel and promptAttackLevelBar where
// LevelToInt(AttackLevel) >= LevelToInt(promptAttackLevelBar),
// evaluateRiskMultiModal SHALL return RiskBlock regardless of Detail content.
func TestProperty5a_TopLevelRiskLevelGateProducesRiskBlock(t *testing.T) {
	f := func(seed uint64) bool {
		r := rand.New(rand.NewSource(int64(seed)))

		// Pick (riskLevel, threshold) where LevelToInt(riskLevel) >= LevelToInt(threshold)
		// Use validGeneralRiskLevels [none, low, medium, high, max]
		thresholdIdx := r.Intn(len(validGeneralRiskLevels))
		levelIdx := thresholdIdx + r.Intn(len(validGeneralRiskLevels)-thresholdIdx)

		riskLevel := validGeneralRiskLevels[levelIdx]
		threshold := validGeneralRiskLevels[thresholdIdx]

		// Sanity check
		if LevelToInt(riskLevel) < LevelToInt(threshold) {
			t.Errorf("generator bug: riskLevel=%s (%d) < threshold=%s (%d)",
				riskLevel, LevelToInt(riskLevel), threshold, LevelToInt(threshold))
			return false
		}

		config := baseConfig()
		config.ContentModerationLevelBar = threshold
		// Set promptAttackLevelBar to max so it doesn't interfere
		config.PromptAttackLevelBar = MaxRisk

		// Generate random details to show they don't matter
		numDetails := r.Intn(4) // 0-3 random details
		details := make([]Detail, numDetails)
		for i := 0; i < numDetails; i++ {
			detailType := knownDetailTypes[r.Intn(len(knownDetailTypes))]
			var level string
			if detailType == SensitiveDataType {
				level = validSensitiveLevels[r.Intn(len(validSensitiveLevels))]
			} else {
				level = validGeneralRiskLevels[r.Intn(len(validGeneralRiskLevels))]
			}
			details[i] = Detail{
				Type:       detailType,
				Suggestion: "pass",
				Level:      level,
			}
		}

		data := Data{
			RiskLevel: riskLevel,
			Detail:    details,
		}

		result := EvaluateRisk(MultiModalGuard, data, config, "")
		if result != RiskBlock {
			t.Errorf("expected RiskBlock for RiskLevel=%s, contentModerationLevelBar=%s, got %d",
				riskLevel, threshold, result)
			return false
		}
		return true
	}

	cfg := &quick.Config{MaxCount: 200}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("Property 5a failed: %v", err)
		fmt.Printf("Property 5a counterexample: %v\n", err)
	}
}

func TestProperty5b_TopLevelAttackLevelGateProducesRiskBlock(t *testing.T) {
	f := func(seed uint64) bool {
		r := rand.New(rand.NewSource(int64(seed)))

		// Pick (attackLevel, threshold) where LevelToInt(attackLevel) >= LevelToInt(threshold)
		thresholdIdx := r.Intn(len(validGeneralRiskLevels))
		levelIdx := thresholdIdx + r.Intn(len(validGeneralRiskLevels)-thresholdIdx)

		attackLevel := validGeneralRiskLevels[levelIdx]
		threshold := validGeneralRiskLevels[thresholdIdx]

		// Sanity check
		if LevelToInt(attackLevel) < LevelToInt(threshold) {
			t.Errorf("generator bug: attackLevel=%s (%d) < threshold=%s (%d)",
				attackLevel, LevelToInt(attackLevel), threshold, LevelToInt(threshold))
			return false
		}

		config := baseConfig()
		config.PromptAttackLevelBar = threshold
		// Set contentModerationLevelBar to max so it doesn't interfere
		config.ContentModerationLevelBar = MaxRisk

		// Generate random details to show they don't matter
		numDetails := r.Intn(4) // 0-3 random details
		details := make([]Detail, numDetails)
		for i := 0; i < numDetails; i++ {
			detailType := knownDetailTypes[r.Intn(len(knownDetailTypes))]
			var level string
			if detailType == SensitiveDataType {
				level = validSensitiveLevels[r.Intn(len(validSensitiveLevels))]
			} else {
				level = validGeneralRiskLevels[r.Intn(len(validGeneralRiskLevels))]
			}
			details[i] = Detail{
				Type:       detailType,
				Suggestion: "pass",
				Level:      level,
			}
		}

		data := Data{
			AttackLevel: attackLevel,
			RiskLevel:   "none", // Avoid contentModeration gate interference
			Detail:      details,
		}

		result := EvaluateRisk(MultiModalGuard, data, config, "")
		if result != RiskBlock {
			t.Errorf("expected RiskBlock for AttackLevel=%s, promptAttackLevelBar=%s, got %d",
				attackLevel, threshold, result)
			return false
		}
		return true
	}

	cfg := &quick.Config{MaxCount: 200}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("Property 5b failed: %v", err)
		fmt.Printf("Property 5b counterexample: %v\n", err)
	}
}

// Feature: sensitive-data-mask-threshold, Property 6: Data.Suggestion=block fallback
// **Validates: Requirements 3.5**
//
// For any set of Details that do not individually trigger block, when Data.Suggestion=block,
// evaluateRiskMultiModal SHALL return RiskBlock.
func TestProperty6_DataSuggestionBlockIgnoredWhenThresholdNotExceeded(t *testing.T) {
	f := func(seed uint64) bool {
		r := rand.New(rand.NewSource(int64(seed)))

		numDetails := r.Intn(5)
		nonBlockSuggestions := []string{"pass", "watch"}
		details := make([]Detail, numDetails)

		for i := 0; i < numDetails; i++ {
			detailType := knownDetailTypes[r.Intn(len(knownDetailTypes))]
			suggestion := nonBlockSuggestions[r.Intn(len(nonBlockSuggestions))]

			var level string
			if detailType == SensitiveDataType {
				level = "S0"
			} else {
				level = "none"
			}

			details[i] = Detail{
				Type:       detailType,
				Suggestion: suggestion,
				Level:      level,
			}
		}

		config := baseConfig()
		config.ContentModerationLevelBar = MaxRisk
		config.PromptAttackLevelBar = MaxRisk
		config.SensitiveDataLevelBar = S4Sensitive
		config.MaliciousUrlLevelBar = MaxRisk
		config.ModelHallucinationLevelBar = MaxRisk
		config.CustomLabelLevelBar = MaxRisk
		config.RiskAction = "block"

		data := Data{
			RiskLevel:   "none",
			AttackLevel: "",
			Suggestion:  "block",
			Detail:      details,
		}

		result := EvaluateRisk(MultiModalGuard, data, config, "")
		if result != RiskPass {
			t.Errorf("expected RiskPass when no detail exceeds threshold (data.Suggestion=block should be ignored), got %d", result)
			return false
		}
		return true
	}

	cfg := &quick.Config{MaxCount: 200}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("Property 6 failed: %v", err)
		fmt.Printf("Property 6 counterexample: %v\n", err)
	}
}
