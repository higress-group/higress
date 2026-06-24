package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/tidwall/gjson"
)

// testLogger satisfies log.Log so log.Warnf calls during tests write to
// stderr instead of nil-panicking. The config parser emits a warn when
// duplicate limit_by+key combinations appear in rule_items.
type testLogger struct{}

func (l *testLogger) Trace(msg string) { fmt.Fprintf(os.Stderr, "[TRACE] %s\n", msg) }
func (l *testLogger) Tracef(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[TRACE] "+format+"\n", args...)
}
func (l *testLogger) Debug(msg string) { fmt.Fprintf(os.Stderr, "[DEBUG] %s\n", msg) }
func (l *testLogger) Debugf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[DEBUG] "+format+"\n", args...)
}
func (l *testLogger) Info(msg string) { fmt.Fprintf(os.Stderr, "[INFO] %s\n", msg) }
func (l *testLogger) Infof(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[INFO] "+format+"\n", args...)
}
func (l *testLogger) Warn(msg string) { fmt.Fprintf(os.Stderr, "[WARN] %s\n", msg) }
func (l *testLogger) Warnf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[WARN] "+format+"\n", args...)
}
func (l *testLogger) Error(msg string) { fmt.Fprintf(os.Stderr, "[ERROR] %s\n", msg) }
func (l *testLogger) Errorf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[ERROR] "+format+"\n", args...)
}
func (l *testLogger) Critical(msg string) { fmt.Fprintf(os.Stderr, "[CRITICAL] %s\n", msg) }
func (l *testLogger) Criticalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[CRITICAL] "+format+"\n", args...)
}
func (l *testLogger) ResetID(pluginID string) {}

func init() {
	log.SetPluginLog(&testLogger{})
}

func TestParseClusterKeyRateLimitConfig(t *testing.T) {
	tests := []struct {
		name        string
		json        string
		expected    ClusterKeyRateLimitConfig
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
					"query_per_minute": -100
				}
			}`,
			expectedErr: errors.New("failed to parse global_threshold: 'query_per_minute' must be a positive integer, got -100"),
		},
		{
			name: "GlobalThreshold_QueryPerSecond",
			json: `{
				"rule_name": "global-route-limit",
				"global_threshold": {
					"query_per_second": 100
				}
			}`,
			expected: ClusterKeyRateLimitConfig{
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
					"query_per_minute": 1000
				}
			}`,
			expected: ClusterKeyRateLimitConfig{
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
							{"key": "key1", "query_per_minute": -100}
						]
					}
				]
			}`,
			expectedErr: errors.New("failed to parse rule_item in rule_items: 'query_per_minute' must be a positive integer for key 'key1', got -100"),
		},
		{
			name: "RuleItems_SingleRule",
			json: `{
				"rule_name": "rule-based-limit",
				"rule_items": [
					{
						"limit_by_header": "x-test",
						"limit_keys": [
							{"key": "key1", "query_per_second": 10}
						]
					}
				]
			}`,
			expected: ClusterKeyRateLimitConfig{
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
							{"key": "123", "query_per_hour": 50}
						]
					},
					{
						"limit_by_per_cookie": "session_id",
						"limit_keys": [
							{"key": "*", "query_per_day": 100}
						]
					}
				]
			}`,
			expected: ClusterKeyRateLimitConfig{
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
				"global_threshold": {"query_per_second": 100},
				"rule_items": [
					{
						"limit_by_header": "x-test",
						"limit_keys": [{"key": "k1", "query_per_minute": 10}]
					}
				]
			}`,
			expected: ClusterKeyRateLimitConfig{
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
				ShowLimitQuotaHeader: false,
				RejectedCode:         DefaultRejectedCode,
				RejectedMsg:          DefaultRejectedMsg,
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
				"global_threshold": {"query_per_second": 100}
			}`,
			expected: ClusterKeyRateLimitConfig{
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
			name: "ShowLimitQuotaHeader_Enabled",
			json: `{
				"rule_name": "show-header",
				"show_limit_quota_header": true,
				"global_threshold": {"query_per_second": 100}
			}`,
			expected: ClusterKeyRateLimitConfig{
				RuleName: "show-header",
				GlobalThreshold: &GlobalThreshold{
					Count:      100,
					TimeWindow: Second,
				},
				ShowLimitQuotaHeader: true,
				RejectedCode:         DefaultRejectedCode,
				RejectedMsg:          DefaultRejectedMsg,
			},
		},
		{
			// Multi-window with the same limit_by + key is not supported: a warn
			// is logged at parse time, but the parser still accepts the config so
			// business logic remains preserved.
			name: "DuplicateRules_DiffWindow",
			json: `{
				"rule_name": "dup-diff-window",
				"rule_items": [
					{
						"limit_by_header": "x-api-key",
						"limit_keys": [{"key": "k1", "query_per_minute": 100}]
					},
					{
						"limit_by_header": "x-api-key",
						"limit_keys": [{"key": "k1", "query_per_hour": 1000}]
					}
				]
			}`,
			expected: ClusterKeyRateLimitConfig{
				RuleName: "dup-diff-window",
				RuleItems: []LimitRuleItem{
					{
						LimitType: LimitByHeaderType,
						Key:       "x-api-key",
						ConfigItems: []LimitConfigItem{
							{ConfigType: ExactType, Key: "k1", Count: 100, TimeWindow: SecondsPerMinute},
						},
					},
					{
						LimitType: LimitByHeaderType,
						Key:       "x-api-key",
						ConfigItems: []LimitConfigItem{
							{ConfigType: ExactType, Key: "k1", Count: 1000, TimeWindow: SecondsPerHour},
						},
					},
				},
				ShowLimitQuotaHeader: false,
				RejectedCode:         DefaultRejectedCode,
				RejectedMsg:          DefaultRejectedMsg,
			},
		},
		{
			// Exact duplicate of (limit_by + key + window) is not allowed: a warn
			// is logged at parse time, but the parser still accepts the config so
			// business logic remains preserved.
			name: "DuplicateRules_SameWindow",
			json: `{
				"rule_name": "dup-same-window",
				"rule_items": [
					{
						"limit_by_header": "x-api-key",
						"limit_keys": [{"key": "k1", "query_per_minute": 100}]
					},
					{
						"limit_by_header": "x-api-key",
						"limit_keys": [{"key": "k1", "query_per_minute": 100}]
					}
				]
			}`,
			expected: ClusterKeyRateLimitConfig{
				RuleName: "dup-same-window",
				RuleItems: []LimitRuleItem{
					{
						LimitType: LimitByHeaderType,
						Key:       "x-api-key",
						ConfigItems: []LimitConfigItem{
							{ConfigType: ExactType, Key: "k1", Count: 100, TimeWindow: SecondsPerMinute},
						},
					},
					{
						LimitType: LimitByHeaderType,
						Key:       "x-api-key",
						ConfigItems: []LimitConfigItem{
							{ConfigType: ExactType, Key: "k1", Count: 100, TimeWindow: SecondsPerMinute},
						},
					},
				},
				ShowLimitQuotaHeader: false,
				RejectedCode:         DefaultRejectedCode,
				RejectedMsg:          DefaultRejectedMsg,
			},
		},
		{
			name: "LimitByType_AtLeastOne_Error",
			json: `{
				"rule_name": "no-limit-by",
				"rule_items": [
					{"limit_keys": [{"key": "k", "query_per_second": 1}]}
				]
			}`,
			expectedErr: errors.New("failed to parse rule_item in rule_items: at least one of 'limit_by_header', 'limit_by_param', 'limit_by_consumer', 'limit_by_cookie', 'limit_by_per_header', 'limit_by_per_param', 'limit_by_per_consumer', 'limit_by_per_cookie', 'limit_by_per_ip' must be set"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var config ClusterKeyRateLimitConfig
			result := gjson.Parse(tt.json)
			err := ParseClusterKeyRateLimitConfig(result, &config)

			if tt.expectedErr != nil {
				assert.EqualError(t, err, tt.expectedErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, config)
			}
		})
	}
}

func TestParseClusterKeyRateLimitConfig_RuleItemsLimit(t *testing.T) {
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
					fmt.Sprintf(`{"limit_by_header":"h%d","limit_keys":[{"key":"k","query_per_second":1}]}`, i+1))
			}
			jsonStr := fmt.Sprintf(`{"rule_name":"t","rule_items":[%s]}`, strings.Join(items, ","))
			var config ClusterKeyRateLimitConfig
			err := ParseClusterKeyRateLimitConfig(gjson.Parse(jsonStr), &config)
			if tt.expectErr == "" {
				assert.NoError(t, err)
				assert.Len(t, config.RuleItems, tt.itemCnt)
			} else {
				assert.EqualError(t, err, tt.expectErr)
			}
		})
	}
}
