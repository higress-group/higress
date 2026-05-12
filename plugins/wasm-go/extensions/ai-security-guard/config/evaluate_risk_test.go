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
	"testing"

	"github.com/stretchr/testify/require"
)

// baseConfig returns a config with all thresholds set to max (most permissive)
// so that tests can focus on specific dimension action behavior.
func baseConfig() AISecurityConfig {
	return AISecurityConfig{
		RiskAction:                 "block",
		ContentModerationLevelBar:  MaxRisk,
		PromptAttackLevelBar:       MaxRisk,
		SensitiveDataLevelBar:      S4Sensitive,
		MaliciousUrlLevelBar:       MaxRisk,
		ModelHallucinationLevelBar: MaxRisk,
		CustomLabelLevelBar:        MaxRisk,
	}
}

// =============================================================================
// TC-EVAL: 风险判定核心测试（EvaluateRisk）
// =============================================================================

// TestTC_EVAL_001 MultiModalGuard，sensitiveDataAction=mask，Suggestion=mask，无 block => RiskMask
func TestTC_EVAL_001(t *testing.T) {
	config := baseConfig()
	config.SensitiveDataAction = "mask"
	config.SensitiveDataLevelBar = "S2" // Lower threshold to match detail Level=S2

	data := Data{
		RiskLevel: "none",
		Detail: []Detail{
			{
				Suggestion: "mask",
				Type:       SensitiveDataType,
				Level:      "S2",
				Result:     []Result{{Ext: Ext{Desensitization: "masked-text"}}},
			},
		},
	}

	result := EvaluateRisk(MultiModalGuard, data, config, "")
	require.Equal(t, RiskMask, result)
}

// TestTC_EVAL_002 Suggestion=block but level below threshold => RiskPass
func TestTC_EVAL_002(t *testing.T) {
	config := baseConfig()
	config.SensitiveDataAction = "mask"

	data := Data{
		RiskLevel: "none",
		Detail: []Detail{
			{
				Suggestion: "block",
				Type:       SensitiveDataType,
				Level:      "S2",
			},
		},
	}

	result := EvaluateRisk(MultiModalGuard, data, config, "")
	require.Equal(t, RiskPass, result)
}

// TestTC_EVAL_003 promptAttackAction=block 且该维度超阈值 => RiskBlock
func TestTC_EVAL_003(t *testing.T) {
	config := baseConfig()
	config.PromptAttackAction = "block"
	config.PromptAttackLevelBar = "high" // threshold = high

	data := Data{
		RiskLevel: "none",
		Detail: []Detail{
			{
				Suggestion: "pass",
				Type:       PromptAttackType,
				Level:      "high", // level >= threshold
			},
		},
	}

	result := EvaluateRisk(MultiModalGuard, data, config, "")
	require.Equal(t, RiskBlock, result)
}

// TestTC_EVAL_004 同时存在 sensitiveData(mask) 与 promptAttack(block) 命中 => RiskBlock
func TestTC_EVAL_004(t *testing.T) {
	config := baseConfig()
	config.SensitiveDataAction = "mask"
	config.PromptAttackAction = "block"
	config.PromptAttackLevelBar = "high"

	data := Data{
		RiskLevel: "none",
		Detail: []Detail{
			{
				Suggestion: "mask",
				Type:       SensitiveDataType,
				Level:      "S2",
				Result:     []Result{{Ext: Ext{Desensitization: "masked"}}},
			},
			{
				Suggestion: "pass",
				Type:       PromptAttackType,
				Level:      "high", // exceeds threshold
			},
		},
	}

	result := EvaluateRisk(MultiModalGuard, data, config, "")
	require.Equal(t, RiskBlock, result)
}

// TestTC_EVAL_005 仅有 mask 候选且无 block => RiskMask
func TestTC_EVAL_005(t *testing.T) {
	config := baseConfig()
	config.SensitiveDataAction = "mask"
	config.SensitiveDataLevelBar = "S1" // Lower threshold to match detail Level=S1

	data := Data{
		RiskLevel: "none",
		Detail: []Detail{
			{
				Suggestion: "mask",
				Type:       SensitiveDataType,
				Level:      "S1",
				Result:     []Result{{Ext: Ext{Desensitization: "masked"}}},
			},
			{
				Suggestion: "pass",
				Type:       ContentModerationType,
				Level:      "low",
			},
		},
	}

	result := EvaluateRisk(MultiModalGuard, data, config, "")
	require.Equal(t, RiskMask, result)
}

// TestTC_EVAL_006 各维度均不超阈值且无建议 => RiskPass
func TestTC_EVAL_006(t *testing.T) {
	config := baseConfig()

	data := Data{
		RiskLevel: "none",
		Detail: []Detail{
			{
				Suggestion: "pass",
				Type:       ContentModerationType,
				Level:      "low",
			},
			{
				Suggestion: "pass",
				Type:       PromptAttackType,
				Level:      "low",
			},
		},
	}

	result := EvaluateRisk(MultiModalGuard, data, config, "")
	require.Equal(t, RiskPass, result)
}

// TestTC_EVAL_007 顶层 RiskLevel 超 contentModerationLevelBar => RiskBlock
func TestTC_EVAL_007(t *testing.T) {
	config := baseConfig()
	config.ContentModerationLevelBar = "high"

	data := Data{
		RiskLevel: "high", // >= threshold
	}

	result := EvaluateRisk(MultiModalGuard, data, config, "")
	require.Equal(t, RiskBlock, result)
}

// TestTC_EVAL_008 顶层 AttackLevel 超 promptAttackLevelBar => RiskBlock
func TestTC_EVAL_008(t *testing.T) {
	config := baseConfig()
	config.PromptAttackLevelBar = "high"

	data := Data{
		RiskLevel:   "none",
		AttackLevel: "high", // >= threshold
	}

	result := EvaluateRisk(MultiModalGuard, data, config, "")
	require.Equal(t, RiskBlock, result)
}

// TestTC_EVAL_009 未知 detail.Type 且 Suggestion=pass/watch => 不触发 block/mask
func TestTC_EVAL_009(t *testing.T) {
	config := baseConfig()

	data := Data{
		RiskLevel: "none",
		Detail: []Detail{
			{
				Suggestion: "pass",
				Type:       "unknownType",
				Level:      "high",
			},
			{
				Suggestion: "watch",
				Type:       "anotherUnknown",
				Level:      "medium",
			},
		},
	}

	result := EvaluateRisk(MultiModalGuard, data, config, "")
	require.Equal(t, RiskPass, result)
}

// TestTC_EVAL_010 TextModerationPlus 下配置动作字段 => 仅按 RiskLevelBar 决策
func TestTC_EVAL_010(t *testing.T) {
	config := baseConfig()
	config.RiskLevelBar = "high"
	config.SensitiveDataAction = "mask"

	// RiskLevel=low < threshold=high => RiskPass
	data := Data{
		RiskLevel: "low",
		Detail: []Detail{
			{
				Suggestion: "mask",
				Type:       SensitiveDataType,
				Level:      "S2",
				Result:     []Result{{Ext: Ext{Desensitization: "masked"}}},
			},
		},
	}

	result := EvaluateRisk(TextModerationPlus, data, config, "")
	require.Equal(t, RiskPass, result)

	// RiskLevel=high >= threshold=high => RiskBlock
	data2 := Data{
		RiskLevel: "high",
		Detail: []Detail{
			{
				Suggestion: "mask",
				Type:       SensitiveDataType,
				Level:      "S2",
			},
		},
	}

	result2 := EvaluateRisk(TextModerationPlus, data2, config, "")
	require.Equal(t, RiskBlock, result2)
}

// TestTC_EVAL_011 contentModerationAction=mask，但顶层 RiskLevel 超阈值 => RiskBlock
func TestTC_EVAL_011(t *testing.T) {
	config := baseConfig()
	config.ContentModerationAction = "mask"
	config.ContentModerationLevelBar = "high"

	data := Data{
		RiskLevel: "high", // >= threshold => 顶层门控触发
	}

	result := EvaluateRisk(MultiModalGuard, data, config, "")
	require.Equal(t, RiskBlock, result)
}

// TestTC_EVAL_012 promptAttackAction=mask，但顶层 AttackLevel 超阈值 => RiskBlock
func TestTC_EVAL_012(t *testing.T) {
	config := baseConfig()
	config.PromptAttackAction = "mask"
	config.PromptAttackLevelBar = "high"

	data := Data{
		RiskLevel:   "none",
		AttackLevel: "high", // >= threshold => 顶层门控触发
	}

	result := EvaluateRisk(MultiModalGuard, data, config, "")
	require.Equal(t, RiskBlock, result)
}

// TestTC_EVAL_013 顶层未超阈值，Detail(sensitiveData) Suggestion=mask 且 action=mask => RiskMask
func TestTC_EVAL_013(t *testing.T) {
	config := baseConfig()
	config.SensitiveDataAction = "mask"
	config.SensitiveDataLevelBar = "S1" // Lower threshold to match detail Level=S1
	config.ContentModerationLevelBar = "high"
	config.PromptAttackLevelBar = "high"

	data := Data{
		RiskLevel:   "low",  // < threshold
		AttackLevel: "none", // < threshold
		Detail: []Detail{
			{
				Suggestion: "mask",
				Type:       SensitiveDataType,
				Level:      "S1",
				Result:     []Result{{Ext: Ext{Desensitization: "masked-content"}}},
			},
		},
	}

	result := EvaluateRisk(MultiModalGuard, data, config, "")
	require.Equal(t, RiskMask, result)
}

// TestTC_EVAL_014 未知维度 Detail.Type=maliciousFile 无阈值配置 => RiskPass
func TestTC_EVAL_014(t *testing.T) {
	config := baseConfig()

	data := Data{
		RiskLevel: "none",
		Detail: []Detail{
			{
				Suggestion: "block",
				Type:       MaliciousFileType,
				Level:      "high",
			},
		},
	}

	result := EvaluateRisk(MultiModalGuard, data, config, "")
	require.Equal(t, RiskPass, result)
}

// TestTC_EVAL_015 Detail level below threshold, data.Suggestion=block ignored => RiskPass
func TestTC_EVAL_015(t *testing.T) {
	config := baseConfig()

	data := Data{
		RiskLevel:  "none",
		Suggestion: "block",
		Detail: []Detail{
			{
				Suggestion: "pass",
				Type:       ContentModerationType,
				Level:      "low",
			},
		},
	}

	result := EvaluateRisk(MultiModalGuard, data, config, "")
	require.Equal(t, RiskPass, result)
}

// TestTC_EVAL_016 Data.Suggestion=mask 但无 sensitiveData 脱敏明细 => 不返回 RiskMask
func TestTC_EVAL_016(t *testing.T) {
	config := baseConfig()

	data := Data{
		RiskLevel:  "none",
		Suggestion: "mask", // Data 级别的 mask，但无 sensitiveData 明细
		Detail: []Detail{
			{
				Suggestion: "pass",
				Type:       ContentModerationType,
				Level:      "low",
			},
		},
	}

	result := EvaluateRisk(MultiModalGuard, data, config, "")
	require.Equal(t, RiskPass, result)
}

// TestTC_EVAL_017 contentModerationAction=mask 且 Detail(contentModeration).Suggestion=mask
// => 不返回 RiskMask（降级为 block 语义）
func TestTC_EVAL_017(t *testing.T) {
	config := baseConfig()
	config.ContentModerationAction = "mask"
	config.ContentModerationLevelBar = "high"

	// contentModeration 维度配置 mask，但 enforceMaskBoundary 会降级为 block
	// Detail level=low < threshold=high => 不超阈值 => 不触发 block
	// Suggestion=mask 对非 sensitiveData 维度不产生 RiskMask
	data := Data{
		RiskLevel: "none",
		Detail: []Detail{
			{
				Suggestion: "mask",
				Type:       ContentModerationType,
				Level:      "low",
			},
		},
	}

	result := EvaluateRisk(MultiModalGuard, data, config, "")
	// contentModeration 的 mask 被降级为 block，level 未超阈值 => 不触发 block
	// Suggestion=mask 但 dimAction 已降级为 block => 不进入 mask 分支
	// 最终 RiskPass
	require.Equal(t, RiskPass, result)
}

// =============================================================================
// TC-DESENS: 脱敏提取测试（ExtractDesensitization）
// =============================================================================

// TestTC_DESENS_001 sensitiveData + Suggestion=mask + Ext.Desensitization => 返回脱敏文本
func TestTC_DESENS_001(t *testing.T) {
	data := Data{
		Detail: []Detail{
			{
				Suggestion: "mask",
				Type:       SensitiveDataType,
				Level:      "S2",
				Result: []Result{
					{Ext: Ext{Desensitization: "我的电话是1**********"}},
				},
			},
		},
	}

	result := ExtractDesensitization(data)
	require.Equal(t, "我的电话是1**********", result)
}

// TestTC_DESENS_002 非 sensitiveData 且 Suggestion=mask => 忽略
func TestTC_DESENS_002(t *testing.T) {
	data := Data{
		Detail: []Detail{
			{
				Suggestion: "mask",
				Type:       ContentModerationType,
				Level:      "high",
				Result: []Result{
					{Ext: Ext{Desensitization: "some-content"}},
				},
			},
		},
	}

	result := ExtractDesensitization(data)
	require.Equal(t, "", result)
}

// TestTC_DESENS_003 多条 sensitiveData 明细，首条无脱敏、次条有脱敏 => 返回次条
func TestTC_DESENS_003(t *testing.T) {
	data := Data{
		Detail: []Detail{
			{
				Suggestion: "mask",
				Type:       SensitiveDataType,
				Level:      "S2",
				Result: []Result{
					{Ext: Ext{Desensitization: ""}}, // 首条无脱敏内容
				},
			},
			{
				Suggestion: "mask",
				Type:       SensitiveDataType,
				Level:      "S3",
				Result: []Result{
					{Ext: Ext{Desensitization: "脱敏后的内容"}},
				},
			},
		},
	}

	result := ExtractDesensitization(data)
	require.Equal(t, "脱敏后的内容", result)
}

// TestTC_DESENS_004 无任何可用脱敏文本 => 返回空字符串
func TestTC_DESENS_004(t *testing.T) {
	data := Data{
		Detail: []Detail{
			{
				Suggestion: "mask",
				Type:       SensitiveDataType,
				Level:      "S2",
				Result: []Result{
					{Ext: Ext{Desensitization: ""}},
				},
			},
			{
				Suggestion: "pass",
				Type:       SensitiveDataType,
				Level:      "S1",
				Result: []Result{
					{Ext: Ext{Desensitization: "some-text"}},
				},
			},
		},
	}

	result := ExtractDesensitization(data)
	require.Equal(t, "", result)
}

// =============================================================================
// 补充边界测试
// =============================================================================

// TestTC_EVAL_018 MultiModalGuardForBase64 路径走统一判定流程
func TestTC_EVAL_018(t *testing.T) {
	config := baseConfig()
	config.SensitiveDataAction = "mask"
	config.SensitiveDataLevelBar = "S2" // Lower threshold to match detail Level=S2

	data := Data{
		RiskLevel: "none",
		Detail: []Detail{
			{
				Suggestion: "mask",
				Type:       SensitiveDataType,
				Level:      "S2",
				Result:     []Result{{Ext: Ext{Desensitization: "masked-text"}}},
			},
		},
	}

	result := EvaluateRisk(MultiModalGuardForBase64, data, config, "")
	require.Equal(t, RiskMask, result)

	// block scenario: level=high but threshold=max => RiskPass
	data2 := Data{
		RiskLevel: "none",
		Detail: []Detail{
			{
				Suggestion: "block",
				Type:       ContentModerationType,
				Level:      "high",
			},
		},
	}
	result2 := EvaluateRisk(MultiModalGuardForBase64, data2, config, "")
	require.Equal(t, RiskPass, result2)
}

// TestTC_EVAL_019 空 Detail 列表 + Data.Suggestion=block => RiskPass (threshold not exceeded)
func TestTC_EVAL_019(t *testing.T) {
	config := baseConfig()

	data := Data{
		RiskLevel:  "none",
		Suggestion: "block",
		Detail:     []Detail{},
	}

	result := EvaluateRisk(MultiModalGuard, data, config, "")
	require.Equal(t, RiskPass, result)
}

// TestTC_EVAL_020 空 Detail 列表 + 无 Data.Suggestion => RiskPass
func TestTC_EVAL_020(t *testing.T) {
	config := baseConfig()

	data := Data{
		RiskLevel: "none",
		Detail:    []Detail{},
	}

	result := EvaluateRisk(MultiModalGuard, data, config, "")
	require.Equal(t, RiskPass, result)
}

// TestTC_EVAL_021 多维度混合：sensitiveData(mask) + contentModeration(pass) + promptAttack(block 超阈值)
// => RiskBlock（promptAttack 超阈值触发拦截）
func TestTC_EVAL_021(t *testing.T) {
	config := baseConfig()
	config.SensitiveDataAction = "mask"
	config.PromptAttackAction = "block"
	config.PromptAttackLevelBar = "high"
	config.ContentModerationLevelBar = "high"

	data := Data{
		RiskLevel: "none",
		Detail: []Detail{
			{
				Suggestion: "mask",
				Type:       SensitiveDataType,
				Level:      "S2",
				Result:     []Result{{Ext: Ext{Desensitization: "masked"}}},
			},
			{
				Suggestion: "pass",
				Type:       ContentModerationType,
				Level:      "low",
			},
			{
				Suggestion: "pass",
				Type:       PromptAttackType,
				Level:      "high", // 超阈值
			},
		},
	}

	result := EvaluateRisk(MultiModalGuard, data, config, "")
	require.Equal(t, RiskBlock, result)
}

// TestTC_EVAL_022 多维度混合：sensitiveData(mask) + contentModeration(block 未超阈值) + promptAttack(block 未超阈值)
// => RiskMask（无 block 触发，有 mask 候选）
func TestTC_EVAL_022(t *testing.T) {
	config := baseConfig()
	config.SensitiveDataAction = "mask"
	config.SensitiveDataLevelBar = "S2" // Lower threshold to match detail Level=S2
	config.ContentModerationAction = "block"
	config.ContentModerationLevelBar = "high"
	config.PromptAttackAction = "block"
	config.PromptAttackLevelBar = "high"

	data := Data{
		RiskLevel: "none",
		Detail: []Detail{
			{
				Suggestion: "mask",
				Type:       SensitiveDataType,
				Level:      "S2",
				Result:     []Result{{Ext: Ext{Desensitization: "masked"}}},
			},
			{
				Suggestion: "pass",
				Type:       ContentModerationType,
				Level:      "low", // 未超阈值
			},
			{
				Suggestion: "pass",
				Type:       PromptAttackType,
				Level:      "low", // 未超阈值
			},
		},
	}

	result := EvaluateRisk(MultiModalGuard, data, config, "")
	require.Equal(t, RiskMask, result)
}

// TestTC_EVAL_023 未知维度 Type + Suggestion=pass + 高 level => RiskPass
// （detailExceedsThreshold 对未知 Type 返回 false）
func TestTC_EVAL_023(t *testing.T) {
	config := baseConfig()

	data := Data{
		RiskLevel: "none",
		Detail: []Detail{
			{
				Suggestion: "pass",
				Type:       MaliciousFileType, // 未知维度（不在 dimensionActionKey 映射中）
				Level:      "max",             // 即使 level 很高
			},
			{
				Suggestion: "pass",
				Type:       WaterMarkType, // 另一个未知维度
				Level:      "max",
			},
		},
	}

	result := EvaluateRisk(MultiModalGuard, data, config, "")
	require.Equal(t, RiskPass, result)
}

// TestTC_EVAL_024 sensitiveDataAction=mask 但 Suggestion=pass 且 level 超阈值 => RiskBlock
func TestTC_EVAL_024(t *testing.T) {
	config := baseConfig()
	config.SensitiveDataAction = "mask"
	config.SensitiveDataLevelBar = "S2"

	data := Data{
		RiskLevel: "none",
		Detail: []Detail{
			{
				Suggestion: "pass",
				Type:       SensitiveDataType,
				Level:      "S3", // 超阈值
			},
		},
	}

	result := EvaluateRisk(MultiModalGuard, data, config, "")
	require.Equal(t, RiskBlock, result)
}

// TestTC_EVAL_025 sensitiveDataAction=mask 但 Suggestion=pass 且 level 未超阈值 => RiskPass
func TestTC_EVAL_025(t *testing.T) {
	config := baseConfig()
	config.SensitiveDataAction = "mask"
	config.SensitiveDataLevelBar = "S4"

	data := Data{
		RiskLevel: "none",
		Detail: []Detail{
			{
				Suggestion: "pass",
				Type:       SensitiveDataType,
				Level:      "S1", // 未超阈值
			},
		},
	}

	result := EvaluateRisk(MultiModalGuard, data, config, "")
	require.Equal(t, RiskPass, result)
}

// TestTC_EVAL_026 Data.RiskLevel 为空字符串 => 不触发顶层门控
func TestTC_EVAL_026(t *testing.T) {
	config := baseConfig()
	config.ContentModerationLevelBar = "high"

	data := Data{
		RiskLevel: "", // 空字符串
		Detail: []Detail{
			{
				Suggestion: "pass",
				Type:       ContentModerationType,
				Level:      "low",
			},
		},
	}

	result := EvaluateRisk(MultiModalGuard, data, config, "")
	require.Equal(t, RiskPass, result)
}

// TestTC_EVAL_027 consumer 维度动作集成：consumer sensitiveDataAction=mask + riskAction=block
// => sensitiveData 走 mask，promptAttack 走 block
func TestTC_EVAL_027(t *testing.T) {
	config := baseConfig()
	config.PromptAttackLevelBar = "high"
	config.ConsumerRiskLevel = []map[string]interface{}{
		{
			"matcher":               Matcher{Exact: "user-a"},
			"riskAction":            "block",
			"sensitiveDataAction":   "mask",
			"sensitiveDataLevelBar": "S2", // Lower threshold to match detail Level=S2
		},
	}

	// sensitiveData mask + promptAttack 未超阈值 => RiskMask
	data := Data{
		RiskLevel: "none",
		Detail: []Detail{
			{
				Suggestion: "mask",
				Type:       SensitiveDataType,
				Level:      "S2",
				Result:     []Result{{Ext: Ext{Desensitization: "masked"}}},
			},
			{
				Suggestion: "pass",
				Type:       PromptAttackType,
				Level:      "low", // 未超阈值
			},
		},
	}
	result := EvaluateRisk(MultiModalGuard, data, config, "user-a")
	require.Equal(t, RiskMask, result)

	// promptAttack 超阈值 => RiskBlock（即使有 mask 候选）
	data2 := Data{
		RiskLevel: "none",
		Detail: []Detail{
			{
				Suggestion: "mask",
				Type:       SensitiveDataType,
				Level:      "S2",
				Result:     []Result{{Ext: Ext{Desensitization: "masked"}}},
			},
			{
				Suggestion: "pass",
				Type:       PromptAttackType,
				Level:      "high", // 超阈值
			},
		},
	}
	result2 := EvaluateRisk(MultiModalGuard, data2, config, "user-a")
	require.Equal(t, RiskBlock, result2)
}

// TestTC_EVAL_028 Data.Suggestion=block does NOT override threshold checks.
// Detail level S1 < threshold S4, so the request should pass even though
// the top-level Suggestion is "block".
func TestTC_EVAL_028(t *testing.T) {
	config := baseConfig()
	config.SensitiveDataAction = "mask"

	data := Data{
		RiskLevel:  "none",
		Suggestion: "block",
		Detail: []Detail{
			{
				Suggestion: "mask",
				Type:       SensitiveDataType,
				Level:      "S1",
				Result:     []Result{{Ext: Ext{Desensitization: "masked"}}},
			},
		},
	}

	result := EvaluateRisk(MultiModalGuard, data, config, "")
	require.Equal(t, RiskPass, result)
}

// TestTC_DESENS_005 Detail.Result 为空数组 => 返回空字符串
func TestTC_DESENS_005(t *testing.T) {
	data := Data{
		Detail: []Detail{
			{
				Suggestion: "mask",
				Type:       SensitiveDataType,
				Level:      "S2",
				Result:     []Result{}, // 空数组
			},
		},
	}

	result := ExtractDesensitization(data)
	require.Equal(t, "", result)
}

// TestTC_EVAL_029 未命中 consumer 规则时回退全局维度动作
func TestTC_EVAL_029(t *testing.T) {
	config := baseConfig()
	config.SensitiveDataAction = "mask"
	config.SensitiveDataLevelBar = "S1" // Lower threshold to match detail Level=S1
	config.ConsumerRiskLevel = []map[string]interface{}{
		{
			"matcher":    Matcher{Exact: "vip-user"},
			"riskAction": "block",
		},
	}

	// "other-user" 不匹配任何规则，回退到 global_dimension(mask)
	data := Data{
		RiskLevel: "none",
		Detail: []Detail{
			{
				Suggestion: "mask",
				Type:       SensitiveDataType,
				Level:      "S1",
				Result:     []Result{{Ext: Ext{Desensitization: "masked"}}},
			},
		},
	}

	result := EvaluateRisk(MultiModalGuard, data, config, "other-user")
	require.Equal(t, RiskMask, result)
}

// =============================================================================
// TC-EVAL: detailExceedsThreshold 各维度覆盖
// =============================================================================

// TestTC_EVAL_030 MaliciousUrlDataType 超阈值 => RiskBlock
func TestTC_EVAL_030(t *testing.T) {
	config := baseConfig()
	config.MaliciousUrlLevelBar = "medium"

	data := Data{
		RiskLevel: "none",
		Detail: []Detail{
			{
				Suggestion: "pass",
				Type:       MaliciousUrlDataType,
				Level:      "high", // exceeds "medium"
			},
		},
	}

	result := EvaluateRisk(MultiModalGuard, data, config, "")
	require.Equal(t, RiskBlock, result)
}

// TestTC_EVAL_031 ModelHallucinationDataType 超阈值 => RiskBlock
func TestTC_EVAL_031(t *testing.T) {
	config := baseConfig()
	config.ModelHallucinationLevelBar = "medium"

	data := Data{
		RiskLevel: "none",
		Detail: []Detail{
			{
				Suggestion: "none",
				Type:       ModelHallucinationDataType,
				Level:      "high", // exceeds "medium"
			},
		},
	}

	result := EvaluateRisk(MultiModalGuard, data, config, "")
	require.Equal(t, RiskBlock, result)
}

// TestTC_EVAL_032 CustomLabelType 超阈值 => RiskBlock
func TestTC_EVAL_032(t *testing.T) {
	config := baseConfig()
	config.CustomLabelLevelBar = "low"

	data := Data{
		RiskLevel: "none",
		Detail: []Detail{
			{
				Suggestion: "none",
				Type:       CustomLabelType,
				Level:      "medium", // exceeds "low"
			},
		},
	}

	result := EvaluateRisk(MultiModalGuard, data, config, "")
	require.Equal(t, RiskBlock, result)
}

// TestTC_EVAL_033 MaliciousUrlDataType 未超阈值 => RiskPass
func TestTC_EVAL_033(t *testing.T) {
	config := baseConfig()
	config.MaliciousUrlLevelBar = "high"

	data := Data{
		RiskLevel: "none",
		Detail: []Detail{
			{
				Suggestion: "pass",
				Type:       MaliciousUrlDataType,
				Level:      "low", // below "high"
			},
		},
	}

	result := EvaluateRisk(MultiModalGuard, data, config, "")
	require.Equal(t, RiskPass, result)
}

// TestTC_EVAL_034 ModelHallucinationDataType 未超阈值 => RiskPass
func TestTC_EVAL_034(t *testing.T) {
	config := baseConfig()
	config.ModelHallucinationLevelBar = "high"

	data := Data{
		RiskLevel: "none",
		Detail: []Detail{
			{
				Suggestion: "none",
				Type:       ModelHallucinationDataType,
				Level:      "low", // below "high"
			},
		},
	}

	result := EvaluateRisk(MultiModalGuard, data, config, "")
	require.Equal(t, RiskPass, result)
}

// TestTC_EVAL_035 CustomLabelType 未超阈值 + 有 mask 候选 => RiskMask
func TestTC_EVAL_035(t *testing.T) {
	config := baseConfig()
	config.CustomLabelLevelBar = "high"
	config.SensitiveDataAction = "mask"
	config.SensitiveDataLevelBar = "S1" // Lower threshold to match detail Level=S1

	data := Data{
		RiskLevel: "none",
		Detail: []Detail{
			{
				Suggestion: "none",
				Type:       CustomLabelType,
				Level:      "low", // below "high"
			},
			{
				Suggestion: "mask",
				Type:       SensitiveDataType,
				Level:      "S1",
				Result:     []Result{{Ext: Ext{Desensitization: "masked"}}},
			},
		},
	}

	result := EvaluateRisk(MultiModalGuard, data, config, "")
	require.Equal(t, RiskMask, result)
}

// =============================================================================
// TC-EVAL: 阈值边界测试（Threshold Boundary Tests）
// =============================================================================

// TestTC_EVAL_036 低于阈值的 mask 建议 => RiskPass
// Config: sensitiveDataAction=mask, sensitiveDataLevelBar=S3
// Detail: Type=sensitiveData, Suggestion=mask, Level=S1 (低于 S3)
// Expected: RiskPass（Level 未达阈值，跳过脱敏）
// Validates: Requirements 5.2
func TestTC_EVAL_036(t *testing.T) {
	config := baseConfig()
	config.SensitiveDataAction = "mask"
	config.SensitiveDataLevelBar = "S3"

	data := Data{
		RiskLevel: "none",
		Detail: []Detail{
			{
				Suggestion: "mask",
				Type:       SensitiveDataType,
				Level:      "S1", // S1 < S3 => 低于阈值
				Result:     []Result{{Ext: Ext{Desensitization: "masked"}}},
			},
		},
	}

	result := EvaluateRisk(MultiModalGuard, data, config, "")
	require.Equal(t, RiskPass, result)
}

// TestTC_EVAL_037 恰好达到阈值的 mask 建议 => RiskMask
// Config: sensitiveDataAction=mask, sensitiveDataLevelBar=S2
// Detail: Type=sensitiveData, Suggestion=mask, Level=S2 (等于 S2)
// Expected: RiskMask（Level 达到阈值，触发脱敏）
// Validates: Requirements 5.3
func TestTC_EVAL_037(t *testing.T) {
	config := baseConfig()
	config.SensitiveDataAction = "mask"
	config.SensitiveDataLevelBar = "S2"

	data := Data{
		RiskLevel: "none",
		Detail: []Detail{
			{
				Suggestion: "mask",
				Type:       SensitiveDataType,
				Level:      "S2", // S2 >= S2 => 达到阈值
				Result:     []Result{{Ext: Ext{Desensitization: "masked-text"}}},
			},
		},
	}

	result := EvaluateRisk(MultiModalGuard, data, config, "")
	require.Equal(t, RiskMask, result)
}

// TestTC_EVAL_038 混合高低阈值明细 => RiskMask
// Config: sensitiveDataAction=mask, sensitiveDataLevelBar=S3
// Details: 一条 Level=S1（低于阈值），一条 Level=S3（达到阈值）
// Expected: RiskMask（达到阈值的明细触发脱敏）
// Validates: Requirements 5.4
func TestTC_EVAL_038(t *testing.T) {
	config := baseConfig()
	config.SensitiveDataAction = "mask"
	config.SensitiveDataLevelBar = "S3"

	data := Data{
		RiskLevel: "none",
		Detail: []Detail{
			{
				Suggestion: "mask",
				Type:       SensitiveDataType,
				Level:      "S1", // S1 < S3 => 低于阈值，不贡献 mask
				Result:     []Result{{Ext: Ext{Desensitization: "masked-low"}}},
			},
			{
				Suggestion: "mask",
				Type:       SensitiveDataType,
				Level:      "S3", // S3 >= S3 => 达到阈值，贡献 mask
				Result:     []Result{{Ext: Ext{Desensitization: "masked-high"}}},
			},
		},
	}

	result := EvaluateRisk(MultiModalGuard, data, config, "")
	require.Equal(t, RiskMask, result)
}

// TestTC_EVAL_039 所有明细均低于阈值 => RiskPass
// Config: sensitiveDataAction=mask, sensitiveDataLevelBar=S4
// Details: 两条 sensitiveData，Level=S1 和 Level=S2（均低于 S4）
// Expected: RiskPass（无明细达到阈值，全部跳过脱敏）
// Validates: Requirements 5.2, 5.4
func TestTC_EVAL_039(t *testing.T) {
	config := baseConfig()
	config.SensitiveDataAction = "mask"
	config.SensitiveDataLevelBar = "S4"

	data := Data{
		RiskLevel: "none",
		Detail: []Detail{
			{
				Suggestion: "mask",
				Type:       SensitiveDataType,
				Level:      "S1", // S1 < S4 => 低于阈值
				Result:     []Result{{Ext: Ext{Desensitization: "masked-1"}}},
			},
			{
				Suggestion: "mask",
				Type:       SensitiveDataType,
				Level:      "S2", // S2 < S4 => 低于阈值
				Result:     []Result{{Ext: Ext{Desensitization: "masked-2"}}},
			},
		},
	}

	result := EvaluateRisk(MultiModalGuard, data, config, "")
	require.Equal(t, RiskPass, result)
}
