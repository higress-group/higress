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
	"os"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/proxytest"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/stretchr/testify/require"
)

// testVMContext is a minimal VMContext for setting up the proxy-wasm mock host.
type testVMContext struct {
	types.DefaultVMContext
}

// TestMain sets up the proxy-wasm mock host for all tests in the config package.
// This is required because functions like enforceMaskBoundary call proxywasm.LogWarnf.
func TestMain(m *testing.M) {
	opt := proxytest.NewEmulatorOption().WithVMContext(&testVMContext{})
	_, reset := proxytest.NewHostEmulator(opt)
	defer reset()
	os.Exit(m.Run())
}

// =============================================================================
// TC-RESOLVE: 动作解析优先级测试（ResolveRiskActionByType）
// =============================================================================

// TestTC_RESOLVE_001 仅全局 riskAction=mask，无维度动作
// => sensitiveData 返回 mask，非 sensitiveData 维度降级为 block
func TestTC_RESOLVE_001(t *testing.T) {
	config := AISecurityConfig{
		RiskAction: "mask",
	}

	// sensitiveData 返回 mask（source=global_global）
	action, source := config.ResolveRiskActionByType("", SensitiveDataType)
	require.Equal(t, "mask", action)
	require.Equal(t, "global_global", source)

	// promptAttack 降级为 block（source=global_global）
	action, source = config.ResolveRiskActionByType("", PromptAttackType)
	require.Equal(t, "block", action)
	require.Equal(t, "global_global", source)

	// contentModeration 降级为 block
	action, source = config.ResolveRiskActionByType("", ContentModerationType)
	require.Equal(t, "block", action)
	require.Equal(t, "global_global", source)

	// maliciousUrl 降级为 block
	action, source = config.ResolveRiskActionByType("", MaliciousUrlDataType)
	require.Equal(t, "block", action)
	require.Equal(t, "global_global", source)
}

// TestTC_RESOLVE_002 全局 riskAction=mask + 全局 promptAttackAction=block
// => promptAttack 返回 block，sensitiveData 返回 mask
func TestTC_RESOLVE_002(t *testing.T) {
	config := AISecurityConfig{
		RiskAction:         "mask",
		PromptAttackAction: "block",
	}

	// promptAttack 返回 block（source=global_dimension）
	action, source := config.ResolveRiskActionByType("", PromptAttackType)
	require.Equal(t, "block", action)
	require.Equal(t, "global_dimension", source)

	// sensitiveData 返回 mask（source=global_global）
	action, source = config.ResolveRiskActionByType("", SensitiveDataType)
	require.Equal(t, "mask", action)
	require.Equal(t, "global_global", source)
}

// TestTC_RESOLVE_003 consumer 规则含 riskAction=block，全局 sensitiveDataAction=mask
// => sensitiveData 返回 block（consumer_global 优先于 global_dimension）
func TestTC_RESOLVE_003(t *testing.T) {
	config := AISecurityConfig{
		RiskAction:          "mask",
		SensitiveDataAction: "mask",
		ConsumerRiskLevel: []map[string]interface{}{
			{
				"matcher":    Matcher{Exact: "user-a"},
				"riskAction": "block",
			},
		},
	}

	// consumer_global(block) 优先于 global_dimension(mask)
	action, source := config.ResolveRiskActionByType("user-a", SensitiveDataType)
	require.Equal(t, "block", action)
	require.Equal(t, "consumer_global", source)

	// 未命中 consumer 规则时，回退到 global_dimension
	action, source = config.ResolveRiskActionByType("user-b", SensitiveDataType)
	require.Equal(t, "mask", action)
	require.Equal(t, "global_dimension", source)
}

// TestTC_RESOLVE_004 consumer 规则含 sensitiveDataAction=mask 且 riskAction=block
// => sensitiveData 返回 mask（consumer_dimension 优先）
func TestTC_RESOLVE_004(t *testing.T) {
	config := AISecurityConfig{
		RiskAction: "block",
		ConsumerRiskLevel: []map[string]interface{}{
			{
				"matcher":             Matcher{Exact: "user-a"},
				"riskAction":          "block",
				"sensitiveDataAction": "mask",
			},
		},
	}

	// consumer_dimension(mask) 优先于 consumer_global(block)
	action, source := config.ResolveRiskActionByType("user-a", SensitiveDataType)
	require.Equal(t, "mask", action)
	require.Equal(t, "consumer_dimension", source)

	// promptAttack 无 consumer_dimension，回退到 consumer_global(block)
	action, source = config.ResolveRiskActionByType("user-a", PromptAttackType)
	require.Equal(t, "block", action)
	require.Equal(t, "consumer_global", source)
}

// TestTC_RESOLVE_005 都未配置 => 返回 block（source=default）
func TestTC_RESOLVE_005(t *testing.T) {
	config := AISecurityConfig{}

	action, source := config.ResolveRiskActionByType("", SensitiveDataType)
	require.Equal(t, "block", action)
	require.Equal(t, "default", source)

	action, source = config.ResolveRiskActionByType("", PromptAttackType)
	require.Equal(t, "block", action)
	require.Equal(t, "default", source)
}

// =============================================================================
// TC-MATCH: first-match 语义测试（getMatchedConsumerRiskRule）
// =============================================================================

// TestTC_MATCH_001 两条规则都可命中（prefix + exact），prefix 在前 => 命中 prefix
func TestTC_MATCH_001(t *testing.T) {
	config := AISecurityConfig{
		RiskAction: "block",
		ConsumerRiskLevel: []map[string]interface{}{
			{
				"matcher":             Matcher{Prefix: "user-"},
				"sensitiveDataAction": "mask",
			},
			{
				"matcher":             Matcher{Exact: "user-a"},
				"sensitiveDataAction": "block",
			},
		},
	}

	// "user-a" 同时匹配 prefix("user-") 和 exact("user-a")，但 prefix 在前
	action, source := config.ResolveRiskActionByType("user-a", SensitiveDataType)
	require.Equal(t, "mask", action)
	require.Equal(t, "consumer_dimension", source)
}

// TestTC_MATCH_002 首条命中但未配置某维度动作，第二条配置了 => 不读取第二条，回退全局
func TestTC_MATCH_002(t *testing.T) {
	config := AISecurityConfig{
		RiskAction:         "mask",
		PromptAttackAction: "block",
		ConsumerRiskLevel: []map[string]interface{}{
			{
				"matcher":    Matcher{Prefix: "user-"},
				"riskAction": "mask",
				// 未配置 promptAttackAction
			},
			{
				"matcher":            Matcher{Exact: "user-a"},
				"promptAttackAction": "block",
			},
		},
	}

	// "user-a" 命中首条 prefix 规则，promptAttackAction 未配置
	// 回退到 consumer_global(mask)，然后 enforceMaskBoundary 降级为 block
	action, source := config.ResolveRiskActionByType("user-a", PromptAttackType)
	require.Equal(t, "block", action)
	require.Equal(t, "consumer_global", source)
}

// TestTC_MATCH_003 无规则命中 => 回退全局
func TestTC_MATCH_003(t *testing.T) {
	config := AISecurityConfig{
		RiskAction:          "mask",
		SensitiveDataAction: "mask",
		ConsumerRiskLevel: []map[string]interface{}{
			{
				"matcher":             Matcher{Exact: "vip-user"},
				"sensitiveDataAction": "block",
			},
		},
	}

	// "other-user" 不匹配任何规则，回退到 global_dimension
	action, source := config.ResolveRiskActionByType("other-user", SensitiveDataType)
	require.Equal(t, "mask", action)
	require.Equal(t, "global_dimension", source)

	// promptAttack 无 global_dimension，回退到 global_global(mask)，降级为 block
	action, source = config.ResolveRiskActionByType("other-user", PromptAttackType)
	require.Equal(t, "block", action)
	require.Equal(t, "global_global", source)
}

// =============================================================================
// 补充边界测试
// =============================================================================

// TestTC_RESOLVE_006 consumer 规则中 promptAttackAction=mask => 降级为 block
func TestTC_RESOLVE_006(t *testing.T) {
	config := AISecurityConfig{
		RiskAction: "block",
		ConsumerRiskLevel: []map[string]interface{}{
			{
				"matcher":            Matcher{Exact: "user-a"},
				"promptAttackAction": "mask", // 非 sensitiveData 维度配置 mask
			},
		},
	}

	// consumer_dimension(mask) 降级为 block
	action, source := config.ResolveRiskActionByType("user-a", PromptAttackType)
	require.Equal(t, "block", action)
	require.Equal(t, "consumer_dimension", source)
}

// TestTC_RESOLVE_007 consumer 规则中 contentModerationAction=mask => 降级为 block
func TestTC_RESOLVE_007(t *testing.T) {
	config := AISecurityConfig{
		RiskAction: "block",
		ConsumerRiskLevel: []map[string]interface{}{
			{
				"matcher":                 Matcher{Exact: "user-a"},
				"contentModerationAction": "mask",
			},
		},
	}

	action, source := config.ResolveRiskActionByType("user-a", ContentModerationType)
	require.Equal(t, "block", action)
	require.Equal(t, "consumer_dimension", source)
}

// TestTC_RESOLVE_008 consumer 规则中 maliciousUrlAction=mask => 降级为 block
func TestTC_RESOLVE_008(t *testing.T) {
	config := AISecurityConfig{
		RiskAction: "block",
		ConsumerRiskLevel: []map[string]interface{}{
			{
				"matcher":            Matcher{Exact: "user-a"},
				"maliciousUrlAction": "mask",
			},
		},
	}

	action, source := config.ResolveRiskActionByType("user-a", MaliciousUrlDataType)
	require.Equal(t, "block", action)
	require.Equal(t, "consumer_dimension", source)
}

// TestTC_RESOLVE_009 未知 detailType => dimensionActionKey 返回空，跳过 consumer_dimension
func TestTC_RESOLVE_009(t *testing.T) {
	config := AISecurityConfig{
		RiskAction: "block",
		ConsumerRiskLevel: []map[string]interface{}{
			{
				"matcher":    Matcher{Exact: "user-a"},
				"riskAction": "mask",
			},
		},
	}

	// 未知 Type，dimKey 为空，跳过 consumer_dimension，回退到 consumer_global(mask)
	// 非 sensitiveData 维度的 mask 降级为 block
	action, source := config.ResolveRiskActionByType("user-a", "unknownType")
	require.Equal(t, "block", action)
	require.Equal(t, "consumer_global", source)
}

// TestTC_RESOLVE_010 未知 detailType + 无 consumer 匹配 => 回退到 global_global
func TestTC_RESOLVE_010(t *testing.T) {
	config := AISecurityConfig{
		RiskAction: "mask",
	}

	// 未知 Type，无 consumer 匹配，回退到 global_global(mask)
	// 非 sensitiveData 维度的 mask 降级为 block
	action, source := config.ResolveRiskActionByType("", "unknownType")
	require.Equal(t, "block", action)
	require.Equal(t, "global_global", source)
}

// TestTC_RESOLVE_011 所有 6 个维度的 global dimension action 正确映射
func TestTC_RESOLVE_011(t *testing.T) {
	config := AISecurityConfig{
		ContentModerationAction:  "block",
		PromptAttackAction:       "block",
		SensitiveDataAction:      "mask",
		MaliciousUrlAction:       "block",
		ModelHallucinationAction: "block",
		CustomLabelAction:        "block",
	}

	tests := []struct {
		detailType     string
		expectedAction string
		expectedSource string
	}{
		{ContentModerationType, "block", "global_dimension"},
		{PromptAttackType, "block", "global_dimension"},
		{SensitiveDataType, "mask", "global_dimension"},
		{MaliciousUrlDataType, "block", "global_dimension"},
		{ModelHallucinationDataType, "block", "global_dimension"},
		{CustomLabelType, "block", "global_dimension"},
	}

	for _, tt := range tests {
		action, source := config.ResolveRiskActionByType("", tt.detailType)
		require.Equal(t, tt.expectedAction, action, "detailType=%s", tt.detailType)
		require.Equal(t, tt.expectedSource, source, "detailType=%s", tt.detailType)
	}
}

// TestTC_MATCH_004 空 consumer 不匹配 exact/prefix 规则 => 回退全局
func TestTC_MATCH_004(t *testing.T) {
	config := AISecurityConfig{
		RiskAction:          "mask",
		SensitiveDataAction: "block",
		ConsumerRiskLevel: []map[string]interface{}{
			{
				"matcher":             Matcher{Exact: "vip"},
				"sensitiveDataAction": "mask",
			},
		},
	}

	// 空 consumer 不匹配 exact("vip")，回退到 global_dimension(block)
	action, source := config.ResolveRiskActionByType("", SensitiveDataType)
	require.Equal(t, "block", action)
	require.Equal(t, "global_dimension", source)
}
