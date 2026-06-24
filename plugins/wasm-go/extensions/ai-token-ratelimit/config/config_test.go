package config

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tidwall/gjson"
)

func TestParseAiTokenRateLimitConfig(t *testing.T) {
	tests := []struct {
		name        string
		json        string
		expected    AiTokenRateLimitConfig
		expectedErr error
	}{
		{
			name:        "MissingRuleName",
			json:        `{}`,
			expectedErr: errors.New("missing rule_name in config"),
		},
		{
			name: "GlobalThreshold_InvalidThreshold",
			json: `{
				"rule_name": "invalid-threshold",
				"global_threshold": {
					"token_per_minute": -100
				}
			}`,
			expectedErr: errors.New("failed to parse global_threshold: 'token_per_minute' must be a positive integer, got -100"),
		},
		{
			name: "GlobalThreshold_QueryPerSecond",
			json: `{
				"rule_name": "global-route-limit",
				"global_threshold": {
					"token_per_second": 100
				}
			}`,
			expected: AiTokenRateLimitConfig{
				RuleName: "global-route-limit",
				GlobalThreshold: &GlobalThreshold{
					Count:      100,
					TimeWindow: Second,
				},
				RejectedCode: DefaultRejectedCode,
				RejectedMsg:  DefaultRejectedMsg,
			},
		},
		{
			name: "GlobalThreshold_QueryPerMinute",
			json: `{
				"rule_name": "global-route-limit",
				"global_threshold": {
					"token_per_minute": 1000
				}
			}`,
			expected: AiTokenRateLimitConfig{
				RuleName: "global-route-limit",
				GlobalThreshold: &GlobalThreshold{
					Count:      1000,
					TimeWindow: SecondsPerMinute,
				},
				RejectedCode: DefaultRejectedCode,
				RejectedMsg:  DefaultRejectedMsg,
			},
		},
		{
			name: "RuleItems_InvalidThreshold",
			json: `{
				"rule_name": "invalid-threshold",
				"rule_items": [
					{
						"limit_by_header": "x-test",
						"limit_keys": [
							{"key": "key1", "token_per_minute": -100}
						]
					}
				]
			}`,
			expectedErr: errors.New("failed to parse rule_item in rule_items: 'token_per_minute' must be a positive integer for key 'key1', got -100"),
		},
		{
			name: "RuleItems_SingleRule",
			json: `{
				"rule_name": "rule-based-limit",
				"rule_items": [
					{
						"limit_by_header": "x-test",
						"limit_keys": [
							{"key": "key1", "token_per_second": 10}
						]
					}
				]
			}`,
			expected: AiTokenRateLimitConfig{
				RuleName: "rule-based-limit",
				RuleItems: []LimitRuleItem{
					{
						LimitType: LimitByHeaderType,
						Key:       "x-test",
						ConfigItems: []LimitConfigItem{
							{
								ConfigType: ExactType,
								Key:        "key1",
								Count:      10,
								TimeWindow: Second,
							},
						},
					},
				},
				RejectedCode: DefaultRejectedCode,
				RejectedMsg:  DefaultRejectedMsg,
			},
		},
		{
			name: "RuleItems_MultipleRules",
			json: `{
				"rule_name": "multi-rule-limit",
				"rule_items": [
					{
						"limit_by_param": "user_id",
						"limit_keys": [
							{"key": "123", "token_per_hour": 50}
						]
					},
					{
						"limit_by_per_cookie": "session_id",
						"limit_keys": [
							{"key": "*", "token_per_day": 100}
						]
					}
				]
			}`,
			expected: AiTokenRateLimitConfig{
				RuleName: "multi-rule-limit",
				RuleItems: []LimitRuleItem{
					{
						LimitType: LimitByParamType,
						Key:       "user_id",
						ConfigItems: []LimitConfigItem{
							{
								ConfigType: ExactType,
								Key:        "123",
								Count:      50,
								TimeWindow: SecondsPerHour,
							},
						},
					},
					{
						LimitType: LimitByPerCookieType,
						Key:       "session_id",
						ConfigItems: []LimitConfigItem{
							{
								ConfigType: AllType,
								Key:        "*",
								Count:      100,
								TimeWindow: SecondsPerDay,
							},
						},
					},
				},
				RejectedCode: DefaultRejectedCode,
				RejectedMsg:  DefaultRejectedMsg,
			},
		},
		{
			name: "Both_ShouldSucceed",
			json: `{
				"rule_name": "test-both",
				"global_threshold": {"token_per_second": 100},
				"rule_items": [
					{
						"limit_by_header": "x-test",
						"limit_keys": [{"key": "k1", "token_per_minute": 10}]
					}
				]
			}`,
			expected: AiTokenRateLimitConfig{
				RuleName: "test-both",
				GlobalThreshold: &GlobalThreshold{
					Count:      100,
					TimeWindow: Second,
				},
				RuleItems: []LimitRuleItem{
					{
						LimitType: LimitByHeaderType,
						Key:       "x-test",
						ConfigItems: []LimitConfigItem{
							{
								ConfigType: ExactType,
								Key:        "k1",
								Count:      10,
								TimeWindow: SecondsPerMinute,
							},
						},
					},
				},
				RejectedCode: DefaultRejectedCode,
				RejectedMsg:  DefaultRejectedMsg,
			},
		},
		{
			name: "Missing_GlobalThresholdAndRuleItems",
			json: `{
				"rule_name": "test-missing"
			}`,
			expectedErr: errors.New("at least one of 'global_threshold' or 'rule_items' must be set"),
		},
		{
			name: "Custom_RejectedCodeAndMessage",
			json: `{
				"rule_name": "custom-reject",
				"rejected_code": 403,
				"rejected_msg": "Forbidden",
				"global_threshold": {"token_per_second": 100}
			}`,
			expected: AiTokenRateLimitConfig{
				RuleName: "custom-reject",
				GlobalThreshold: &GlobalThreshold{
					Count:      100,
					TimeWindow: Second,
				},
				RejectedCode: 403,
				RejectedMsg:  "Forbidden",
			},
		},
		{
			name: "LimitByType_AtLeastOne_Error",
			json: `{
				"rule_name": "no-limit-by",
				"rule_items": [
					{"limit_keys": [{"key": "k", "token_per_second": 1}]}
				]
			}`,
			expectedErr: errors.New("failed to parse rule_item in rule_items: at least one of 'limit_by_header', 'limit_by_param', 'limit_by_consumer', 'limit_by_cookie', 'limit_by_per_header', 'limit_by_per_param', 'limit_by_per_consumer', 'limit_by_per_cookie', 'limit_by_per_ip' must be set"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var config AiTokenRateLimitConfig
			result := gjson.Parse(tt.json)
			err := ParseAiTokenRateLimitConfig(result, &config)

			if tt.expectedErr != nil {
				assert.EqualError(t, err, tt.expectedErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, config)
			}
		})
	}
}

func TestParseAiTokenRateLimitConfig_RuleItemsLimit(t *testing.T) {
	tests := []struct {
		name      string
		itemCnt   int
		expectErr string
	}{
		{"Exactly10", 10, ""},
		{"Over10", 11, "rule_items length 11 exceeds maximum 10"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := make([]string, 0, tt.itemCnt)
			for i := 0; i < tt.itemCnt; i++ {
				items = append(items,
					fmt.Sprintf(`{"limit_by_header":"h%d","limit_keys":[{"key":"k","token_per_second":1}]}`, i+1))
			}
			jsonStr := fmt.Sprintf(`{"rule_name":"t","rule_items":[%s]}`, strings.Join(items, ","))
			var config AiTokenRateLimitConfig
			err := ParseAiTokenRateLimitConfig(gjson.Parse(jsonStr), &config)
			if tt.expectErr == "" {
				assert.NoError(t, err)
				assert.Len(t, config.RuleItems, tt.itemCnt)
			} else {
				assert.EqualError(t, err, tt.expectErr)
			}
		})
	}
}
