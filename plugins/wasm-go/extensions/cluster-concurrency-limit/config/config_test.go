package config

import (
	"testing"

	"cluster-concurrency-limit/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func parse(t *testing.T, json string) (*ClusterConcurrencyLimitConfig, error) {
	t.Helper()
	cfg := &ClusterConcurrencyLimitConfig{}
	err := ParseClusterConcurrencyLimitConfig(gjson.Parse(json), cfg)
	return cfg, err
}

func TestParseClusterConcurrencyLimitConfig(t *testing.T) {
	t.Run("MissingRuleName", func(t *testing.T) {
		_, err := parse(t, `{}`)
		require.EqualError(t, err, "missing rule_name in config")
	})

	t.Run("EmptyRuleName", func(t *testing.T) {
		_, err := parse(t, `{"rule_name": ""}`)
		require.EqualError(t, err, "missing rule_name in config")
	})

	t.Run("NeitherGlobalNorRuleItems", func(t *testing.T) {
		_, err := parse(t, `{"rule_name": "test"}`)
		require.EqualError(t, err, "at least one of 'global_threshold' or 'rule_items' must be set")
	})

	t.Run("GlobalThreshold_MissingConcurrency", func(t *testing.T) {
		_, err := parse(t, `{"rule_name": "test", "global_threshold": {}}`)
		require.EqualError(t, err, "failed to parse global_threshold: 'concurrency' must be set for global_threshold")
	})

	t.Run("GlobalThreshold_InvalidConcurrency", func(t *testing.T) {
		_, err := parse(t, `{"rule_name": "test", "global_threshold": {"concurrency": -1}}`)
		require.EqualError(t, err, "failed to parse global_threshold: 'concurrency' must be a positive integer, got -1")
	})

	t.Run("GlobalThreshold_Valid", func(t *testing.T) {
		cfg, err := parse(t, `{"rule_name": "test", "global_threshold": {"concurrency": 200}}`)
		require.NoError(t, err)
		require.NotNil(t, cfg.GlobalThreshold)
		assert.Equal(t, int64(200), cfg.GlobalThreshold.Concurrency)
		assert.Equal(t, DefaultRejectedCode, cfg.RejectedCode)
		assert.Equal(t, DefaultRejectedMsg, cfg.RejectedMsg)
		assert.Equal(t, DefaultLeaseTTLSeconds, cfg.LeaseTTLSeconds)
		assert.False(t, cfg.ShowLimitQuotaHeader)
	})

	t.Run("GlobalAndRuleItemsCoexist", func(t *testing.T) {
		// 与蓝本不同:global_threshold 与 rule_items 允许并存
		cfg, err := parse(t, `{
			"rule_name": "test",
			"global_threshold": {"concurrency": 200},
			"rule_items": [
				{"limit_by_consumer": "", "limit_keys": [{"key": "alice", "concurrency": 7}]}
			]
		}`)
		require.NoError(t, err)
		require.NotNil(t, cfg.GlobalThreshold)
		require.Len(t, cfg.RuleItems, 1)
	})

	t.Run("RuleItems_Empty", func(t *testing.T) {
		_, err := parse(t, `{"rule_name": "test", "rule_items": []}`)
		require.EqualError(t, err, "config rule_items cannot be empty")
	})

	t.Run("RuleItems_ExceedMax", func(t *testing.T) {
		json := `{"rule_name": "test", "rule_items": [`
		for i := 0; i <= MaxRuleItems; i++ {
			if i > 0 {
				json += ","
			}
			json += `{"limit_by_consumer": "", "limit_keys": [{"key": "a", "concurrency": 1}]}`
		}
		json += `]}`
		_, err := parse(t, json)
		require.ErrorContains(t, err, "cannot exceed")
	})

	t.Run("RuleItem_NoLimitBy", func(t *testing.T) {
		_, err := parse(t, `{
			"rule_name": "test",
			"rule_items": [{"limit_keys": [{"key": "a", "concurrency": 1}]}]
		}`)
		require.ErrorContains(t, err, "must be set")
	})

	t.Run("RuleItem_MultipleLimitBy", func(t *testing.T) {
		_, err := parse(t, `{
			"rule_name": "test",
			"rule_items": [{
				"limit_by_header": "x-api-key",
				"limit_by_consumer": "",
				"limit_keys": [{"key": "a", "concurrency": 1}]
			}]
		}`)
		require.ErrorContains(t, err, "only one of")
	})

	t.Run("RuleItem_MissingLimitKeys", func(t *testing.T) {
		_, err := parse(t, `{
			"rule_name": "test",
			"rule_items": [{"limit_by_consumer": ""}]
		}`)
		require.ErrorContains(t, err, "missing limit_keys")
	})

	t.Run("LimitKeys_MissingConcurrency", func(t *testing.T) {
		_, err := parse(t, `{
			"rule_name": "test",
			"rule_items": [{"limit_by_consumer": "", "limit_keys": [{"key": "alice"}]}]
		}`)
		require.ErrorContains(t, err, "'concurrency' must be set for key: alice")
	})

	t.Run("LimitKeys_InvalidConcurrency", func(t *testing.T) {
		_, err := parse(t, `{
			"rule_name": "test",
			"rule_items": [{"limit_by_consumer": "", "limit_keys": [{"key": "alice", "concurrency": 0}]}]
		}`)
		require.ErrorContains(t, err, "'concurrency' must be a positive integer")
	})

	t.Run("PerType_LiteralKeyNotAllowed", func(t *testing.T) {
		_, err := parse(t, `{
			"rule_name": "test",
			"rule_items": [{"limit_by_per_header": "x-user-id", "limit_keys": [{"key": "user-1", "concurrency": 1}]}]
		}`)
		require.ErrorContains(t, err, "must start with 'regexp:' or be exactly '*'")
	})

	t.Run("PerType_AllAndRegexpAllowed", func(t *testing.T) {
		cfg, err := parse(t, `{
			"rule_name": "test",
			"rule_items": [{"limit_by_per_header": "x-user-id", "limit_keys": [
				{"key": "*", "concurrency": 3},
				{"key": "regexp:^user-\\d+$", "concurrency": 5}
			]}]
		}`)
		require.NoError(t, err)
		items := cfg.RuleItems[0].ConfigItems
		require.Len(t, items, 2)
		assert.Equal(t, AllType, items[0].ConfigType)
		assert.Equal(t, RegexpType, items[1].ConfigType)
	})

	t.Run("NonPerType_AllAndRegexpAllowed", func(t *testing.T) {
		// 非 per_ 类型同样支持 "*" / "regexp:..."(设计文档 §4 示例)
		cfg, err := parse(t, `{
			"rule_name": "test",
			"rule_items": [{"limit_by_consumer_group": "", "limit_keys": [
				{"key": "tenant-a", "concurrency": 10},
				{"key": "*", "concurrency": 5}
			]}]
		}`)
		require.NoError(t, err)
		items := cfg.RuleItems[0].ConfigItems
		require.Len(t, items, 2)
		assert.Equal(t, ExactType, items[0].ConfigType)
		assert.Equal(t, AllType, items[1].ConfigType)
	})

	t.Run("ConsumerGroup", func(t *testing.T) {
		cfg, err := parse(t, `{
			"rule_name": "test",
			"rule_items": [{"limit_by_consumer_group": "", "limit_keys": [{"key": "tenant-a", "concurrency": 10}]}]
		}`)
		require.NoError(t, err)
		assert.Equal(t, LimitByConsumerGroupType, cfg.RuleItems[0].LimitType)
		assert.Equal(t, util.ConsumerGroupHeader, cfg.RuleItems[0].Key)
	})

	t.Run("Consumer", func(t *testing.T) {
		cfg, err := parse(t, `{
			"rule_name": "test",
			"rule_items": [{"limit_by_consumer": "", "limit_keys": [{"key": "alice", "concurrency": 7}]}]
		}`)
		require.NoError(t, err)
		assert.Equal(t, LimitByConsumerType, cfg.RuleItems[0].LimitType)
		assert.Equal(t, util.ConsumerHeader, cfg.RuleItems[0].Key)
	})

	t.Run("PerIp_RemoteAddr", func(t *testing.T) {
		cfg, err := parse(t, `{
			"rule_name": "test",
			"rule_items": [{"limit_by_per_ip": "from-remote-addr", "limit_keys": [{"key": "192.168.1.0/24", "concurrency": 3}]}]
		}`)
		require.NoError(t, err)
		assert.Equal(t, LimitByPerIpType, cfg.RuleItems[0].LimitType)
		assert.Equal(t, RemoteAddrSourceType, cfg.RuleItems[0].LimitByPerIp.SourceType)
		assert.Equal(t, IpNetType, cfg.RuleItems[0].ConfigItems[0].ConfigType)
	})

	t.Run("PerIp_FromHeader", func(t *testing.T) {
		cfg, err := parse(t, `{
			"rule_name": "test",
			"rule_items": [{"limit_by_per_ip": "from-header-x-forwarded-for", "limit_keys": [{"key": "10.0.0.0/8", "concurrency": 3}]}]
		}`)
		require.NoError(t, err)
		assert.Equal(t, HeaderSourceType, cfg.RuleItems[0].LimitByPerIp.SourceType)
		assert.Equal(t, "x-forwarded-for", cfg.RuleItems[0].LimitByPerIp.HeaderName)
	})

	t.Run("PerIp_InvalidSource", func(t *testing.T) {
		_, err := parse(t, `{
			"rule_name": "test",
			"rule_items": [{"limit_by_per_ip": "from-nowhere", "limit_keys": [{"key": "10.0.0.0/8", "concurrency": 3}]}]
		}`)
		require.ErrorContains(t, err, "must start with 'from-header-' or be exactly 'from-remote-addr'")
	})

	t.Run("SkipRoutePrefixes", func(t *testing.T) {
		cfg, err := parse(t, `{
			"rule_name": "test",
			"global_threshold": {"concurrency": 10},
			"skip_route_prefixes": ["svc-", ""]
		}`)
		require.NoError(t, err)
		// 空前串被过滤,不配该字段时保持 nil(不跳过)
		assert.Equal(t, []string{"svc-"}, cfg.SkipRoutePrefixes)
	})

	t.Run("SkipRoutePrefixes_Absent", func(t *testing.T) {
		cfg, err := parse(t, `{"rule_name": "test", "global_threshold": {"concurrency": 10}}`)
		require.NoError(t, err)
		assert.Empty(t, cfg.SkipRoutePrefixes)
	})

	t.Run("CustomRejectedCodeAndMsg", func(t *testing.T) {
		cfg, err := parse(t, `{
			"rule_name": "test",
			"global_threshold": {"concurrency": 10},
			"rejected_code": 503,
			"rejected_msg": "busy",
			"show_limit_quota_header": true,
			"lease_ttl_seconds": 1200
		}`)
		require.NoError(t, err)
		assert.Equal(t, uint32(503), cfg.RejectedCode)
		assert.Equal(t, "busy", cfg.RejectedMsg)
		assert.True(t, cfg.ShowLimitQuotaHeader)
		assert.Equal(t, int64(1200), cfg.LeaseTTLSeconds)
	})

	t.Run("InvalidLeaseTTL", func(t *testing.T) {
		_, err := parse(t, `{
			"rule_name": "test",
			"global_threshold": {"concurrency": 10},
			"lease_ttl_seconds": 0
		}`)
		require.ErrorContains(t, err, "'lease_ttl_seconds' must be a positive integer")
	})
}
