// Copyright (c) 2026 Alibaba Group Holding Ltd.
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

package main

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"cluster-concurrency-limit/config"
	"cluster-concurrency-limit/util"
	"github.com/google/uuid"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
	"github.com/tidwall/resp"
)

func main() {}

func init() {
	wrapper.SetCtx(
		"cluster-concurrency-limit",
		wrapper.ParseConfig(parseConfig),
		wrapper.ProcessRequestHeaders(onHttpRequestHeaders),
		wrapper.ProcessResponseHeaders(onHttpResponseHeaders),
		wrapper.ProcessStreamDone(onHttpStreamDone),
	)
}

const (
	// ConcurrencyLimitRedisKeyFormat 维度计数的 redis key 为 {限流规则名称}:concurrency:维度:实际值
	// {rule_name} 为 hash tag,保证一次请求涉及的所有维度 key 落在 Redis Cluster 同一 slot,单条 Lua 才能原子操作
	ConcurrencyLimitRedisKeyFormat = "{%s}:concurrency:%s:%s"
	// ConcurrencyLimitGlobalRedisKeyFormat 全局兜底维度的 redis key 为 {限流规则名称}:concurrency:global
	ConcurrencyLimitGlobalRedisKeyFormat = "{%s}:concurrency:global"

	// AcquireScript 先检后占,原子处理所有命中维度,无需回滚:
	// 第一轮惰性清理过期 member 并检查每个维度是否超限,任一超限直接返回 {reject=i, cur_i};
	// 全部通过后第二轮才 ZADD 占槽并 PEXPIRE 整 key 垃圾回收,返回 {0, cur_1..cur_N}(cur 为占槽后的计数)
	AcquireScript = `
		local now = tonumber(ARGV[1])
		local lease = tonumber(ARGV[2])
		local member = ARGV[3]
		local curs = {}
		for i = 1, #KEYS do
			local limit = tonumber(ARGV[3 + i])
			redis.call('zremrangebyscore', KEYS[i], 0, now)
			local cur = redis.call('zcard', KEYS[i])
			if cur >= limit then
				return {i, cur}
			end
			curs[i] = cur
		end
		for i = 1, #KEYS do
			redis.call('zadd', KEYS[i], now + lease, member)
			redis.call('pexpire', KEYS[i], lease)
			curs[i] = curs[i] + 1
		end
		local res = {0}
		for i = 1, #KEYS do
			res[i + 1] = curs[i]
		end
		return res
	`
	// ReleaseScript 释放是删自己的 member,天然幂等:重复删/删不存在均 no-op
	ReleaseScript = `
		for i = 1, #KEYS do
			redis.call('zrem', KEYS[i], ARGV[1])
		end
		return 1
	`

	OccupiedContextKey = "ConcurrencyLimitOccupiedContext" // 已占槽信息,stream done 时释放
	QuotaContextKey    = "ConcurrencyLimitQuotaContext"    // 配额响应头信息

	CookieHeader = "cookie"

	ConcurrencyLimitHeader     = "X-Concurrency-Limit"     // 命中维度中最紧桶的并发上限
	ConcurrencyRemainingHeader = "X-Concurrency-Remaining" // 所有命中维度的最小剩余并发数

	ConcurrencyLimitRejectedMetric   = "concurrency_limit_rejected"    // 拒绝数
	ConcurrencyLimitRedisErrorMetric = "concurrency_limit_redis_error" // fail-open 计数
)

// limitDimension 一个命中的限流维度
type limitDimension struct {
	redisKey string // redis key
	limit    int64  // 并发上限
	desc     string // 维度描述,用于日志
}

// occupiedContext 已占槽信息
type occupiedContext struct {
	keys   []string
	member string
}

// quotaContext 配额响应头信息
type quotaContext struct {
	limit     int64
	remaining int64
}

func parseConfig(json gjson.Result, cfg *config.ClusterConcurrencyLimitConfig) error {
	err := config.InitRedisClusterClient(json, cfg)
	if err != nil {
		return err
	}
	err = config.ParseClusterConcurrencyLimitConfig(json, cfg)
	if err != nil {
		return err
	}
	// Metric settings
	cfg.CounterMetrics = make(map[string]proxywasm.MetricCounter)
	return nil
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, cfg config.ClusterConcurrencyLimitConfig) types.Action {
	ctx.DisableReroute()

	// 内部跳路由跳过:AI 服务两跳回环架构下,全局实例会在第二跳 svc- 路由上重复执行,
	// 外层维度被双占槽。命中 skip_route_prefixes 的路由由本实例直接放行(不计数、不调 Redis),
	// 外层维度只在上层路由(第一跳)计一次;服务级路由实例不配该字段,不受影响
	if len(cfg.SkipRoutePrefixes) > 0 {
		if routeName, err := util.GetRouteName(); err == nil {
			// Ingress 派生路由的 envoy 路由名带 "{namespace}/" 前缀
			// (如 baolande/infer-8563635d…),先剥离再按前缀匹配,
			// 否则 infer-/svc{id} 形态的第二跳永不命中 skip → 双占槽
			matchName := routeName
			if idx := strings.LastIndex(matchName, "/"); idx >= 0 {
				matchName = matchName[idx+1:]
			}
			for _, prefix := range cfg.SkipRoutePrefixes {
				if strings.HasPrefix(matchName, prefix) {
					log.Debugf("skip concurrency limit on internal route: %s", routeName)
					return types.ActionContinue
				}
			}
		}
	}

	// 规则收集:global + rule_items 全量收集,all-match OR 叠加
	// 每级独立判定:哪一级配了限哪一级;未配置的身份在该级跳过(不限、不占槽),
	// 已配置的祖先级(如租户)照常生效——访问准入由认证插件负责,本插件只管配额
	dimensions := collectDimensions(ctx, cfg)
	if len(dimensions) == 0 {
		// 没有命中任何维度(含无身份头流量),放行不计数,不发起 Redis 调用
		return types.ActionContinue
	}

	// member:优先 x-request-id,缺失则生成 UUID
	member, err := proxywasm.GetHttpRequestHeader(util.RequestIDHeader)
	if err != nil || member == "" {
		member = uuid.NewString()
	}

	nowMs := time.Now().UnixMilli()
	leaseMs := cfg.LeaseTTLSeconds * 1000

	keys := make([]interface{}, 0, len(dimensions))
	args := make([]interface{}, 0, len(dimensions)+3)
	args = append(args, nowMs, leaseMs, member)
	for _, dim := range dimensions {
		keys = append(keys, dim.redisKey)
		args = append(args, dim.limit)
	}

	err = cfg.RedisClient.Eval(AcquireScript, len(keys), keys, args, func(response resp.Value) {
		if response.Error() != nil {
			// Redis 故障 fail-open 放行
			log.Errorf("redis eval failed: %v", response.Error())
			incrementMetric(cfg, ConcurrencyLimitRedisErrorMetric)
			proxywasm.ResumeHttpRequest()
			return
		}
		resultArray := response.Array()
		if len(resultArray) < 2 {
			log.Errorf("redis response parse error, response: %v", response)
			incrementMetric(cfg, ConcurrencyLimitRedisErrorMetric)
			proxywasm.ResumeHttpRequest()
			return
		}

		rejectIndex := resultArray[0].Integer()
		if rejectIndex > 0 {
			// 触发限流,按 global → rule_items 顺序的第一个超限维度
			if rejectIndex > len(dimensions) {
				log.Errorf("redis response reject index %d out of range, dimensions: %d", rejectIndex, len(dimensions))
				incrementMetric(cfg, ConcurrencyLimitRedisErrorMetric)
				proxywasm.ResumeHttpRequest()
				return
			}
			dim := dimensions[rejectIndex-1]
			log.Infof("concurrency limit exceeded, dimension: %s, current: %d, limit: %d", dim.desc, resultArray[1].Integer(), dim.limit)
			ctx.SetUserAttribute("concurrency_limit_status", "limited")
			ctx.WriteUserAttributeToLogWithKey(wrapper.AILogKey)
			rejected(cfg, dim)
			return
		}

		// 放行:记录已占槽信息(stream done 释放)与配额响应头
		if len(resultArray) != len(dimensions)+1 {
			log.Errorf("redis response parse error, expect %d elements, got: %v", len(dimensions)+1, response)
			incrementMetric(cfg, ConcurrencyLimitRedisErrorMetric)
			proxywasm.ResumeHttpRequest()
			return
		}
		occupiedKeys := make([]string, 0, len(dimensions))
		for _, dim := range dimensions {
			occupiedKeys = append(occupiedKeys, dim.redisKey)
		}
		ctx.SetContext(OccupiedContextKey, occupiedContext{keys: occupiedKeys, member: member})

		if cfg.ShowLimitQuotaHeader {
			// 取所有命中维度的最小剩余("报最紧的桶")
			var minRemaining, limitOfMin int64
			for i, dim := range dimensions {
				remaining := dim.limit - int64(resultArray[i+1].Integer())
				if i == 0 || remaining < minRemaining {
					minRemaining = remaining
					limitOfMin = dim.limit
				}
			}
			ctx.SetContext(QuotaContextKey, quotaContext{limit: limitOfMin, remaining: minRemaining})
		}
		proxywasm.ResumeHttpRequest()
	})
	if err != nil {
		// Redis 调用发起失败,fail-open 放行
		log.Errorf("redis call failed: %v", err)
		incrementMetric(cfg, ConcurrencyLimitRedisErrorMetric)
		return types.ActionContinue
	}
	return types.HeaderStopAllIterationAndWatermark
}

func onHttpResponseHeaders(ctx wrapper.HttpContext, cfg config.ClusterConcurrencyLimitConfig) types.Action {
	if !cfg.ShowLimitQuotaHeader {
		return types.ActionContinue
	}
	quota, ok := ctx.GetContext(QuotaContextKey).(quotaContext)
	if !ok {
		return types.ActionContinue
	}
	_ = proxywasm.ReplaceHttpResponseHeader(ConcurrencyLimitHeader, strconv.FormatInt(quota.limit, 10))
	_ = proxywasm.ReplaceHttpResponseHeader(ConcurrencyRemainingHeader, strconv.FormatInt(quota.remaining, 10))
	return types.ActionContinue
}

// onHttpStreamDone stream 关闭(正常结束/上游 reset/客户端断开全覆盖)时释放占槽,唯一释放入口
func onHttpStreamDone(ctx wrapper.HttpContext, cfg config.ClusterConcurrencyLimitConfig) {
	occupied, ok := ctx.GetContext(OccupiedContextKey).(occupiedContext)
	if !ok || len(occupied.keys) == 0 {
		return
	}
	keys := make([]interface{}, 0, len(occupied.keys))
	for _, key := range occupied.keys {
		keys = append(keys, key)
	}
	err := cfg.RedisClient.Eval(ReleaseScript, len(keys), keys, []interface{}{occupied.member}, nil)
	if err != nil {
		// 释放失败不阻塞,member 会随租约到期被惰性清理
		log.Errorf("failed to release concurrency slots: %v", err)
		incrementMetric(cfg, ConcurrencyLimitRedisErrorMetric)
	}
}

// collectDimensions 收集所有命中的限流维度,global 兜底维度在前,之后按 rule_items 数组顺序。
// 未命中 limit_keys 的维度跳过(不计数、不拒绝),各级独立生效
func collectDimensions(ctx wrapper.HttpContext, cfg config.ClusterConcurrencyLimitConfig) []limitDimension {
	var dimensions []limitDimension
	if cfg.GlobalThreshold != nil {
		dimensions = append(dimensions, limitDimension{
			redisKey: fmt.Sprintf(ConcurrencyLimitGlobalRedisKeyFormat, cfg.RuleName),
			limit:    cfg.GlobalThreshold.Concurrency,
			desc:     "global",
		})
	}
	for _, rule := range cfg.RuleItems {
		val, configItem := hitRuleItem(ctx, rule)
		if configItem == nil {
			continue
		}
		dim := dimName(rule)
		dimensions = append(dimensions, limitDimension{
			redisKey: fmt.Sprintf(ConcurrencyLimitRedisKeyFormat, cfg.RuleName, dim, val),
			limit:    configItem.Concurrency,
			desc:     dim + "=" + val,
		})
	}
	return dimensions
}

// dimName 维度名,参与 redis key 拼装
func dimName(rule config.LimitRuleItem) string {
	switch rule.LimitType {
	case config.LimitByConsumerType:
		return "consumer"
	case config.LimitByConsumerGroupType:
		return "consumer_group"
	case config.LimitByPerConsumerType:
		return "per_consumer"
	case config.LimitByPerIpType:
		return "per_ip"
	case config.LimitByHeaderType:
		return "header:" + rule.Key
	case config.LimitByParamType:
		return "param:" + rule.Key
	case config.LimitByCookieType:
		return "cookie:" + rule.Key
	case config.LimitByPerHeaderType:
		return "per_header:" + rule.Key
	case config.LimitByPerParamType:
		return "per_param:" + rule.Key
	case config.LimitByPerCookieType:
		return "per_cookie:" + rule.Key
	}
	return string(rule.LimitType)
}

// hitRuleItem 判断请求是否命中规则项,返回实际值与命中的配置项(未命中返回 nil,该维度跳过)
func hitRuleItem(ctx wrapper.HttpContext, rule config.LimitRuleItem) (string, *config.LimitConfigItem) {
	switch rule.LimitType {
	// 根据HTTP请求头限流
	case config.LimitByHeaderType, config.LimitByPerHeaderType:
		val, err := proxywasm.GetHttpRequestHeader(rule.Key)
		if err != nil || val == "" {
			return logDebugAndReturnEmpty("failed to get request header %s: %v", rule.Key, err)
		}
		return val, findMatchingItem(rule.ConfigItems, val)
	// 根据HTTP请求参数限流
	case config.LimitByParamType, config.LimitByPerParamType:
		parse, err := url.Parse(ctx.Path())
		if err != nil {
			return logDebugAndReturnEmpty("failed to parse request path: %v", err)
		}
		query, err := url.ParseQuery(parse.RawQuery)
		if err != nil {
			return logDebugAndReturnEmpty("failed to parse query params: %v", err)
		}
		val, ok := query[rule.Key]
		if !ok || len(val) == 0 || val[0] == "" {
			return logDebugAndReturnEmpty("request param %s is empty", rule.Key)
		}
		return val[0], findMatchingItem(rule.ConfigItems, val[0])
	// 根据consumer / 租户(consumer group)限流,身份头由认证插件/前置插件注入,读不到则该维度不计数、放行
	case config.LimitByConsumerType, config.LimitByPerConsumerType, config.LimitByConsumerGroupType:
		val, err := proxywasm.GetHttpRequestHeader(rule.Key)
		if err != nil || val == "" {
			return logDebugAndReturnEmpty("failed to get request header %s: %v", rule.Key, err)
		}
		return val, findMatchingItem(rule.ConfigItems, val)
	// 根据cookie中key值限流
	case config.LimitByCookieType, config.LimitByPerCookieType:
		cookie, err := proxywasm.GetHttpRequestHeader(CookieHeader)
		if err != nil {
			return logDebugAndReturnEmpty("failed to get request cookie : %v", err)
		}
		val := util.ExtractCookieValueByKey(cookie, rule.Key)
		if val == "" {
			return logDebugAndReturnEmpty("cookie key '%s' extracted from cookie '%s' is empty.", rule.Key, cookie)
		}
		return val, findMatchingItem(rule.ConfigItems, val)
	// 根据客户端IP限流
	case config.LimitByPerIpType:
		realIp, err := getDownStreamIp(rule)
		if err != nil {
			log.Warnf("failed to get down stream ip: %v", err)
			return "", nil
		}
		for _, item := range rule.ConfigItems {
			if _, found, _ := item.IpNet.Get(realIp); !found {
				continue
			}
			return realIp.String(), &item
		}
	}
	return "", nil
}

func logDebugAndReturnEmpty(errMsg string, args ...interface{}) (string, *config.LimitConfigItem) {
	log.Debugf(errMsg, args...)
	return "", nil
}

// findMatchingItem 按配置顺序匹配:精确字面值 / "*" / "regexp:..." 均支持
func findMatchingItem(items []config.LimitConfigItem, key string) *config.LimitConfigItem {
	for i, item := range items {
		switch item.ConfigType {
		case config.AllType:
			return &items[i]
		case config.RegexpType:
			if item.Regexp.MatchString(key) {
				return &items[i]
			}
		default:
			if item.Key == key {
				return &items[i]
			}
		}
	}
	return nil
}

func getDownStreamIp(rule config.LimitRuleItem) (net.IP, error) {
	var (
		realIpStr string
		err       error
	)
	if rule.LimitByPerIp.SourceType == config.HeaderSourceType {
		realIpStr, err = proxywasm.GetHttpRequestHeader(rule.LimitByPerIp.HeaderName)
		if err == nil {
			realIpStr = strings.Split(strings.Trim(realIpStr, " "), ",")[0]
		}
	} else {
		var bs []byte
		bs, err = proxywasm.GetProperty([]string{"source", "address"})
		realIpStr = string(bs)
	}
	if err != nil {
		return nil, err
	}
	ip := util.ParseIP(realIpStr)
	realIP := net.ParseIP(ip)
	if realIP == nil {
		return nil, fmt.Errorf("invalid ip[%s]", ip)
	}
	return realIP, nil
}

func incrementMetric(cfg config.ClusterConcurrencyLimitConfig, metricName string) {
	route, _ := util.GetRouteName()
	cluster, _ := util.GetClusterName()
	consumer, _ := util.GetConsumer()
	cfg.IncrementCounter(fmt.Sprintf("route.%s.upstream.%s.consumer.%s.metric.%s", route, cluster, consumer, metricName), 1)
}

func rejected(cfg config.ClusterConcurrencyLimitConfig, dim limitDimension) {
	headers := make(map[string][]string)
	if cfg.ShowLimitQuotaHeader {
		headers[ConcurrencyLimitHeader] = []string{strconv.FormatInt(dim.limit, 10)}
		headers[ConcurrencyRemainingHeader] = []string{"0"}
	}
	_ = proxywasm.SendHttpResponseWithDetail(
		cfg.RejectedCode, "cluster-concurrency-limit.rejected", util.ReconvertHeaders(headers), []byte(cfg.RejectedMsg), -1)

	incrementMetric(cfg, ConcurrencyLimitRejectedMetric)
}
