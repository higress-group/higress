package config

import (
	"errors"
	"fmt"
	re "regexp"
	"strings"

	"cluster-concurrency-limit/util"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
	"github.com/zmap/go-iptree/iptree"
)

// LimitRuleItemType 限流规则项类型
type LimitRuleItemType string

// LimitConfigItemType 限流配置项key类型
type LimitConfigItemType string

const (
	LimitByHeaderType        LimitRuleItemType = "limit_by_header"
	LimitByParamType         LimitRuleItemType = "limit_by_param"
	LimitByConsumerType      LimitRuleItemType = "limit_by_consumer"
	LimitByConsumerGroupType LimitRuleItemType = "limit_by_consumer_group"
	LimitByCookieType        LimitRuleItemType = "limit_by_cookie"
	LimitByPerHeaderType     LimitRuleItemType = "limit_by_per_header"
	LimitByPerParamType      LimitRuleItemType = "limit_by_per_param"
	LimitByPerConsumerType   LimitRuleItemType = "limit_by_per_consumer"
	LimitByPerCookieType     LimitRuleItemType = "limit_by_per_cookie"
	LimitByPerIpType         LimitRuleItemType = "limit_by_per_ip"

	ExactType  LimitConfigItemType = "exact"  // 精确匹配
	RegexpType LimitConfigItemType = "regexp" // 正则表达式
	AllType    LimitConfigItemType = "*"      // 匹配所有情况
	IpNetType  LimitConfigItemType = "ipNet"  // ip段

	RemoteAddrSourceType = "remote-addr"
	HeaderSourceType     = "header"

	// MaxRuleItems rule_items 上限
	MaxRuleItems = 10

	DefaultRejectedCode uint32 = 429
	DefaultRejectedMsg  string = "concurrency limit exceeded"

	// DefaultLeaseTTLSeconds member 租约时长(秒)默认值,仅用于防御进程崩溃导致的槽位泄漏
	DefaultLeaseTTLSeconds int64 = 600
)

type ClusterConcurrencyLimitConfig struct {
	RuleName             string           // 限流规则名称,参与 Redis key 拼装(hash tag)
	GlobalThreshold      *GlobalThreshold // 规则组总并发兜底,可与 RuleItems 并存
	RuleItems            []LimitRuleItem  // 维度规则项,all-match OR 叠加
	RejectedCode         uint32           // 当请求超过阈值被拒绝时,返回的HTTP状态码
	RejectedMsg          string           // 当请求超过阈值被拒绝时,返回的响应体
	ShowLimitQuotaHeader bool             // 是否返回 X-Concurrency-Limit / X-Concurrency-Remaining 响应头
	LeaseTTLSeconds      int64            // member 租约时长(秒)
	// SkipRoutePrefixes 命中的路由名前缀列表:当前路由名以任一前缀开头时本实例直接放行
	// (不计数、不拒绝、不发起 Redis 调用)。用途:AI 服务两跳回环架构(逻辑两层、物理一套)
	// 下,全局 defaultConfig 实例会在第二跳 svc- 内部路由上再执行一次,导致外层维度每请求
	// 双占槽(有效并发=limit/2,limit=1 完全自阻塞)。外层全局配置配 ["svc-"] 跳过第二跳;
	// 服务级路由实例(仅 global_threshold)不配此字段,第二跳服务级并发照常生效。
	SkipRoutePrefixes []string
	RedisClient       wrapper.RedisClient
	CounterMetrics    map[string]proxywasm.MetricCounter // Metrics
}

type GlobalThreshold struct {
	Concurrency int64 // 规则组总并发上限
}

type LimitRuleItem struct {
	LimitType    LimitRuleItemType // 限流类型
	Key          string            // 根据该key值进行限流,limit_by_consumer为ConsumerHeader,limit_by_consumer_group为ConsumerGroupHeader,其他类型为对应的key值
	LimitByPerIp LimitByPerIp      // 对端ip地址或ip段
	ConfigItems  []LimitConfigItem // 限流配置项
}

type LimitByPerIp struct {
	SourceType string // ip来源类型
	HeaderName string // 根据该请求头获取客户端ip
}

type LimitConfigItem struct {
	ConfigType  LimitConfigItemType // 限流配置项key类型
	Key         string              // 限流key
	IpNet       *iptree.IPTree      // 限流key转换的ip地址或者ip段,仅用于itemType为ipNetType
	Regexp      *re.Regexp          // 正则表达式,仅用于itemType为regexpType
	Concurrency int64               // 并发上限(Gauge)
}

func (cfg *ClusterConcurrencyLimitConfig) IncrementCounter(metricName string, inc uint64) {
	if inc == 0 {
		return
	}
	counter, ok := cfg.CounterMetrics[metricName]
	if !ok {
		counter = proxywasm.DefineCounterMetric(metricName)
		cfg.CounterMetrics[metricName] = counter
	}
	counter.Increment(inc)
}

func InitRedisClusterClient(json gjson.Result, config *ClusterConcurrencyLimitConfig) error {
	redisConfig := json.Get("redis")
	if !redisConfig.Exists() {
		return errors.New("missing redis in config")
	}

	serviceName := redisConfig.Get("service_name").String()
	if serviceName == "" {
		return errors.New("redis service name must not be empty")
	}

	servicePort := int(redisConfig.Get("service_port").Int())
	if servicePort == 0 {
		if strings.HasSuffix(serviceName, ".static") {
			// use default logic port which is 80 for static service
			servicePort = 80
		} else {
			servicePort = 6379
		}
	}

	username := redisConfig.Get("username").String()
	password := redisConfig.Get("password").String()
	timeout := int(redisConfig.Get("timeout").Int())
	if timeout == 0 {
		timeout = 1000
	}

	config.RedisClient = wrapper.NewRedisClusterClient(wrapper.FQDNCluster{
		FQDN: serviceName,
		Port: int64(servicePort),
	})
	database := int(redisConfig.Get("database").Int())
	err := config.RedisClient.Init(username, password, int64(timeout), wrapper.WithDataBase(database))
	if config.RedisClient.Ready() {
		log.Info("redis init successfully")
	} else {
		log.Error("redis init failed, will try later")
	}
	return err
}

func ParseClusterConcurrencyLimitConfig(json gjson.Result, config *ClusterConcurrencyLimitConfig) error {
	ruleName := json.Get("rule_name")
	if !ruleName.Exists() || ruleName.String() == "" {
		return errors.New("missing rule_name in config")
	}
	config.RuleName = ruleName.String()

	// 初始化限流规则
	err := initLimitRule(json, config)
	if err != nil {
		return err
	}

	rejectedCode := json.Get("rejected_code")
	if rejectedCode.Exists() {
		config.RejectedCode = uint32(rejectedCode.Uint())
	} else {
		config.RejectedCode = DefaultRejectedCode
	}
	rejectedMsg := json.Get("rejected_msg")
	if rejectedMsg.Exists() {
		config.RejectedMsg = rejectedMsg.String()
	} else {
		config.RejectedMsg = DefaultRejectedMsg
	}

	config.ShowLimitQuotaHeader = json.Get("show_limit_quota_header").Bool()

	// skip_route_prefixes:命中的路由名前缀,本实例对匹配路由直接放行(见结构体注释)
	for _, prefix := range json.Get("skip_route_prefixes").Array() {
		p := prefix.String()
		if p != "" {
			config.SkipRoutePrefixes = append(config.SkipRoutePrefixes, p)
		}
	}

	leaseTTL := json.Get("lease_ttl_seconds")
	if leaseTTL.Exists() {
		ttl := leaseTTL.Int()
		if ttl <= 0 {
			return fmt.Errorf("'lease_ttl_seconds' must be a positive integer, got %d", ttl)
		}
		config.LeaseTTLSeconds = ttl
	} else {
		config.LeaseTTLSeconds = DefaultLeaseTTLSeconds
	}
	return nil
}

func initLimitRule(json gjson.Result, config *ClusterConcurrencyLimitConfig) error {
	globalThresholdResult := json.Get("global_threshold")
	ruleItemsResult := json.Get("rule_items")

	hasGlobal := globalThresholdResult.Exists()
	hasRule := ruleItemsResult.Exists()
	if !hasGlobal && !hasRule {
		return errors.New("at least one of 'global_threshold' or 'rule_items' must be set")
	}
	// 与蓝本不同:global_threshold 与 rule_items 允许并存,global 作兜底维度参与 all-match

	// 处理全局限流配置
	if hasGlobal {
		threshold, err := parseGlobalThreshold(globalThresholdResult)
		if err != nil {
			return fmt.Errorf("failed to parse global_threshold: %w", err)
		}
		config.GlobalThreshold = threshold
	}

	// 处理条件限流规则
	if hasRule {
		items := ruleItemsResult.Array()
		if len(items) == 0 {
			return errors.New("config rule_items cannot be empty")
		}
		if len(items) > MaxRuleItems {
			return fmt.Errorf("config rule_items cannot exceed %d items, got %d", MaxRuleItems, len(items))
		}

		var ruleItems []LimitRuleItem
		// 用于记录已出现的LimitType和Key的组合
		seenLimitRules := make(map[string]bool)

		for _, item := range items {
			ruleItem, err := parseLimitRuleItem(item)
			if err != nil {
				return fmt.Errorf("failed to parse rule_item in rule_items: %w", err)
			}

			// 构造LimitType和Key的唯一标识
			ruleKey := string(ruleItem.LimitType) + ":" + ruleItem.Key

			// 检查是否有重复的LimitType和Key组合
			if seenLimitRules[ruleKey] {
				log.Warnf("duplicate rule found: %s='%s' in rule_items", ruleItem.LimitType, ruleItem.Key)
			} else {
				seenLimitRules[ruleKey] = true
			}

			ruleItems = append(ruleItems, *ruleItem)
		}
		config.RuleItems = ruleItems
	}
	return nil
}

func parseGlobalThreshold(item gjson.Result) (*GlobalThreshold, error) {
	q := item.Get("concurrency")
	if !q.Exists() {
		return nil, errors.New("'concurrency' must be set for global_threshold")
	}
	concurrency := q.Int()
	if concurrency <= 0 {
		return nil, fmt.Errorf("'concurrency' must be a positive integer, got %d", concurrency)
	}
	return &GlobalThreshold{Concurrency: concurrency}, nil
}

func parseLimitRuleItem(item gjson.Result) (*LimitRuleItem, error) {
	var ruleItem LimitRuleItem

	// 根据配置区分限流类型,一个规则项只能设置一种 limit_by
	var limitType LimitRuleItemType
	setCount := 0
	setLimitByKeyIfExists := func(field gjson.Result, limitTypeStr LimitRuleItemType) {
		if field.Exists() && field.String() != "" {
			ruleItem.Key = field.String()
			limitType = limitTypeStr
			setCount++
		}
	}
	setLimitByKeyIfExists(item.Get("limit_by_header"), LimitByHeaderType)
	setLimitByKeyIfExists(item.Get("limit_by_param"), LimitByParamType)
	setLimitByKeyIfExists(item.Get("limit_by_cookie"), LimitByCookieType)
	setLimitByKeyIfExists(item.Get("limit_by_per_header"), LimitByPerHeaderType)
	setLimitByKeyIfExists(item.Get("limit_by_per_param"), LimitByPerParamType)
	setLimitByKeyIfExists(item.Get("limit_by_per_cookie"), LimitByPerCookieType)

	limitByConsumer := item.Get("limit_by_consumer")
	if limitByConsumer.Exists() {
		ruleItem.Key = util.ConsumerHeader
		limitType = LimitByConsumerType
		setCount++
	}
	limitByConsumerGroup := item.Get("limit_by_consumer_group")
	if limitByConsumerGroup.Exists() {
		ruleItem.Key = util.ConsumerGroupHeader
		limitType = LimitByConsumerGroupType
		setCount++
	}
	limitByPerConsumer := item.Get("limit_by_per_consumer")
	if limitByPerConsumer.Exists() {
		ruleItem.Key = util.ConsumerHeader
		limitType = LimitByPerConsumerType
		setCount++
	}

	limitByPerIpResult := item.Get("limit_by_per_ip")
	if limitByPerIpResult.Exists() && limitByPerIpResult.String() != "" {
		limitByPerIp := limitByPerIpResult.String()
		ruleItem.Key = limitByPerIp
		if strings.HasPrefix(limitByPerIp, "from-header-") {
			headerName := limitByPerIp[len("from-header-"):]
			if headerName == "" {
				return nil, errors.New("limit_by_per_ip parse error: empty after 'from-header-'")
			}
			ruleItem.LimitByPerIp = LimitByPerIp{
				SourceType: HeaderSourceType,
				HeaderName: headerName,
			}
		} else if limitByPerIp == "from-remote-addr" {
			ruleItem.LimitByPerIp = LimitByPerIp{
				SourceType: RemoteAddrSourceType,
				HeaderName: "",
			}
		} else {
			return nil, errors.New("the 'limit_by_per_ip' restriction must start with 'from-header-' or be exactly 'from-remote-addr'")
		}
		limitType = LimitByPerIpType
		setCount++
	}

	if limitType == "" {
		return nil, errors.New("one of 'limit_by_header', 'limit_by_param', 'limit_by_consumer', 'limit_by_consumer_group', 'limit_by_cookie', 'limit_by_per_header', 'limit_by_per_param', 'limit_by_per_consumer', 'limit_by_per_cookie' or 'limit_by_per_ip' must be set")
	}
	if setCount > 1 {
		return nil, errors.New("only one of 'limit_by_header', 'limit_by_param', 'limit_by_consumer', 'limit_by_consumer_group', 'limit_by_cookie', 'limit_by_per_header', 'limit_by_per_param', 'limit_by_per_consumer', 'limit_by_per_cookie' or 'limit_by_per_ip' can be set in one rule item")
	}
	ruleItem.LimitType = limitType

	// 初始化configItems
	err := initConfigItems(item, &ruleItem)
	if err != nil {
		return nil, err
	}

	return &ruleItem, nil
}

func initConfigItems(json gjson.Result, rule *LimitRuleItem) error {
	limitKeys := json.Get("limit_keys")
	if !limitKeys.Exists() {
		return errors.New("missing limit_keys in config")
	}
	if len(limitKeys.Array()) == 0 {
		return errors.New("config limit_keys cannot be empty")
	}
	var configItems []LimitConfigItem
	for _, item := range limitKeys.Array() {
		key := item.Get("key")
		if !key.Exists() || key.String() == "" {
			return errors.New("limit_keys key is required")
		}

		var (
			itemKey  = key.String()
			itemType LimitConfigItemType
			ipNet    *iptree.IPTree
			regexp   *re.Regexp
		)
		isPerType := rule.LimitType == LimitByPerHeaderType ||
			rule.LimitType == LimitByPerParamType ||
			rule.LimitType == LimitByPerConsumerType ||
			rule.LimitType == LimitByPerCookieType
		if rule.LimitType == LimitByPerIpType {
			var err error
			ipNet, err = util.ParseIPNet(itemKey)
			if err != nil {
				return fmt.Errorf("failed to parse IPNet for key '%s': %w", itemKey, err)
			}
			itemType = IpNetType
		} else if itemKey == "*" {
			itemType = AllType
		} else if strings.HasPrefix(itemKey, "regexp:") {
			regexpStr := itemKey[len("regexp:"):]
			var err error
			regexp, err = re.Compile(regexpStr)
			if err != nil {
				return fmt.Errorf("failed to compile regex for key '%s': %w", itemKey, err)
			}
			itemType = RegexpType
		} else if isPerType {
			// per_* 类型的 limit_keys 仅接受 "*" / "regexp:..."
			return fmt.Errorf("the '%s' restriction must start with 'regexp:' or be exactly '*'", rule.LimitType)
		} else {
			itemType = ExactType
		}

		concurrency := item.Get("concurrency")
		if !concurrency.Exists() {
			return errors.New("'concurrency' must be set for key: " + itemKey)
		}
		count := concurrency.Int()
		if count <= 0 {
			return fmt.Errorf("'concurrency' must be a positive integer for key '%s', got %d", itemKey, count)
		}
		configItems = append(configItems, LimitConfigItem{
			ConfigType:  itemType,
			Key:         itemKey,
			IpNet:       ipNet,
			Regexp:      regexp,
			Concurrency: count,
		})
	}
	rule.ConfigItems = configItems
	return nil
}
