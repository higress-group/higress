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

package main

import (
	"fmt"
	"math"
	"net"
	"net/url"
	"strconv"
	"strings"

	"cluster-key-rate-limit/config"
	"cluster-key-rate-limit/util"

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
		"cluster-key-rate-limit",
		wrapper.ParseConfig(parseConfig),
		wrapper.ProcessRequestHeaders(onHttpRequestHeaders),
		wrapper.ProcessResponseHeaders(onHttpResponseHeaders),
	)
}

const (
	// RedisKeyPrefix 集群限流插件在 Redis 中 key 的统一前缀
	RedisKeyPrefix = "higress-cluster-key-rate-limit"
	// 使用 {rule_name} hash tag 让多规则多键操作在 Redis Cluster 下落到同一 slot
	// ClusterGlobalRateLimitFormat  全局限流模式 redis key 为 RedisKeyPrefix:限流规则名称:global_threshold:时间窗口
	ClusterGlobalRateLimitFormat = RedisKeyPrefix + ":{%s}:global_threshold:%d"
	// ClusterRateLimitFormat 规则限流模式 redis key 为 RedisKeyPrefix:限流规则名称:限流类型:时间窗口:限流key名称:限流key对应的实际值
	ClusterRateLimitFormat = RedisKeyPrefix + ":{%s}:%s:%d:%s:%s"
	// MultiKeyFixedWindowScript 多规则请求阶段 check + incr 合一脚本（cluster-key-rate-limit 使用）
	// KEYS = [key1, ..., keyN]
	// ARGV = [threshold1, window1, ..., thresholdN, windowN]
	// 返回嵌套数组 {{threshold_i, current_i, ttl_i}, ...}（每个 key 独立判断是否 incr）
	MultiKeyFixedWindowScript = `
		local results = {}
		for i = 1, #KEYS do
			local threshold = tonumber(ARGV[2*i - 1])
			local window    = tonumber(ARGV[2*i])
			local current = tonumber(redis.call('get', KEYS[i]) or "0")
			local ttl = redis.call('ttl', KEYS[i])
			if ttl < 0 then ttl = window end

			if current > threshold then
				-- 已超阈值，不再 incr
				table.insert(results, {threshold, current, ttl})
			else
				-- 未超阈值，原子 incr
				current = redis.call('incr', KEYS[i])
				if current == 1 then
					redis.call('expire', KEYS[i], window)
				end
				table.insert(results, {threshold, current, redis.call('ttl', KEYS[i])})
			end
		end
		return results
	`

	LimitContextKey = "LimitContext" // 限流上下文信息

	CookieHeader = "cookie"

	RateLimitLimitHeader     = "X-RateLimit-Limit"     // 限制的总请求数
	RateLimitRemainingHeader = "X-RateLimit-Remaining" // 剩余还可以发送的请求数
	RateLimitResetHeader     = "X-RateLimit-Reset"     // 限流重置时间（触发限流时返回）
)

type LimitContext struct {
	count     int
	remaining int
	reset     int
}

// MatchedRule 表示请求阶段命中的单条限流规则（global 或 rule_item）
type MatchedRule struct {
	key    string // 完整 Redis key
	count  int64  // 时间窗口内的限额
	window int64  // 时间窗口大小（秒）
}

func parseConfig(json gjson.Result, cfg *config.ClusterKeyRateLimitConfig) error {
	err := config.InitRedisClusterClient(json, cfg)
	if err != nil {
		return err
	}
	err = config.ParseClusterKeyRateLimitConfig(json, cfg)
	if err != nil {
		return err
	}
	return nil
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, cfg config.ClusterKeyRateLimitConfig) types.Action {
	ctx.DisableReroute()

	matched := collectMatchedRules(ctx, cfg)
	if len(matched) == 0 {
		return types.ActionContinue
	}

	n := len(matched)
	keys := make([]interface{}, n)
	args := make([]interface{}, 0, n*2)
	for i, m := range matched {
		keys[i] = m.key
		args = append(args, m.count, m.window)
	}

	err := cfg.RedisClient.Eval(MultiKeyFixedWindowScript, n, keys, args, func(response resp.Value) {
		arr := response.Array()
		if len(arr) != n {
			log.Errorf("redis response length mismatch: got %d, want %d", len(arr), n)
			_ = proxywasm.ResumeHttpRequest()
			return
		}

		// 使用 math.MaxFloat64 初始化，避免在 arr[0] 校验前预先读取
		tightestIdx := 0
		tightestRatio := math.MaxFloat64

		for i, sub := range arr {
			a := sub.Array()
			if len(a) != 3 {
				log.Errorf("redis sub-array length mismatch: got %d, want 3", len(a))
				_ = proxywasm.ResumeHttpRequest()
				return
			}
			threshold, current, ttl := a[0].Integer(), a[1].Integer(), a[2].Integer()

			if current > threshold {
				rejected(cfg, LimitContext{
					count:     threshold,
					remaining: threshold - current,
					reset:     ttl,
				})
				return
			}

			if ratio := float64(threshold-current) / float64(threshold); ratio < tightestRatio {
				tightestIdx = i
				tightestRatio = ratio
			}
		}

		// 未触发：写入 tightest 规则到 LimitContext，供 onHttpResponseHeaders 读取 X-RateLimit-* 头
		tightSub := arr[tightestIdx].Array()
		tightThreshold, tightCurrent, tightTtl := tightSub[0].Integer(), tightSub[1].Integer(), tightSub[2].Integer()
		ctx.SetContext(LimitContextKey, LimitContext{
			count:     tightThreshold,
			remaining: tightThreshold - tightCurrent,
			reset:     tightTtl,
		})

		_ = proxywasm.ResumeHttpRequest()
	})

	if err != nil {
		log.Errorf("redis call failed: %v", err)
		return types.ActionContinue
	}
	return types.HeaderStopAllIterationAndWatermark
}

func onHttpResponseHeaders(ctx wrapper.HttpContext, config config.ClusterKeyRateLimitConfig) types.Action {
	limitContext, ok := ctx.GetContext(LimitContextKey).(LimitContext)
	if !ok {
		return types.ActionContinue
	}
	if config.ShowLimitQuotaHeader {
		_ = proxywasm.ReplaceHttpResponseHeader(RateLimitLimitHeader, strconv.Itoa(limitContext.count))
		_ = proxywasm.ReplaceHttpResponseHeader(RateLimitRemainingHeader, strconv.Itoa(limitContext.remaining))
	}
	return types.ActionContinue
}

// collectMatchedRules 遍历 global_threshold 和 rule_items，返回所有命中规则。
// 顺序：global_threshold（如有）→ rule_items 中所有命中项（按数组顺序追加）。
func collectMatchedRules(ctx wrapper.HttpContext, cfg config.ClusterKeyRateLimitConfig) []MatchedRule {
	var matched []MatchedRule

	if cfg.GlobalThreshold != nil {
		matched = append(matched, MatchedRule{
			key:    fmt.Sprintf(ClusterGlobalRateLimitFormat, cfg.RuleName, cfg.GlobalThreshold.TimeWindow),
			count:  cfg.GlobalThreshold.Count,
			window: cfg.GlobalThreshold.TimeWindow,
		})
	}

	for _, ruleItem := range cfg.RuleItems {
		val, hitRule, hitItem := hitRateRuleItem(ctx, ruleItem)
		if hitRule != nil && hitItem != nil {
			matched = append(matched, MatchedRule{
				key:    fmt.Sprintf(ClusterRateLimitFormat, cfg.RuleName, hitRule.LimitType, hitItem.TimeWindow, hitRule.Key, val),
				count:  hitItem.Count,
				window: hitItem.TimeWindow,
			})
		}
	}

	return matched
}

func hitRateRuleItem(ctx wrapper.HttpContext, rule config.LimitRuleItem) (string, *config.LimitRuleItem, *config.LimitConfigItem) {
	switch rule.LimitType {
	// 根据HTTP请求头限流
	case config.LimitByHeaderType, config.LimitByPerHeaderType:
		val, err := proxywasm.GetHttpRequestHeader(rule.Key)
		if err != nil {
			return logDebugAndReturnEmpty("failed to get request header %s: %v", rule.Key, err)
		}
		return val, &rule, findMatchingItem(rule.LimitType, rule.ConfigItems, val)
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
		if !ok {
			return logDebugAndReturnEmpty("request param %s is empty", rule.Key)
		}
		return val[0], &rule, findMatchingItem(rule.LimitType, rule.ConfigItems, val[0])
	// 根据consumer限流
	case config.LimitByConsumerType, config.LimitByPerConsumerType:
		val, err := proxywasm.GetHttpRequestHeader(config.ConsumerHeader)
		if err != nil {
			return logDebugAndReturnEmpty("failed to get request header %s: %v", config.ConsumerHeader, err)
		}
		return val, &rule, findMatchingItem(rule.LimitType, rule.ConfigItems, val)
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
		return val, &rule, findMatchingItem(rule.LimitType, rule.ConfigItems, val)
	// 根据客户端IP限流
	case config.LimitByPerIpType:
		realIp, err := getDownStreamIp(rule)
		if err != nil {
			log.Warnf("failed to get down stream ip: %v", err)
			return "", &rule, nil
		}
		for _, item := range rule.ConfigItems {
			if _, found, _ := item.IpNet.Get(realIp); !found {
				continue
			}
			return realIp.String(), &rule, &item
		}
	}
	return "", nil, nil
}

func logDebugAndReturnEmpty(errMsg string, args ...interface{}) (string, *config.LimitRuleItem, *config.LimitConfigItem) {
	log.Debugf(errMsg, args...)
	return "", nil, nil
}

func findMatchingItem(limitType config.LimitRuleItemType, items []config.LimitConfigItem, key string) *config.LimitConfigItem {
	for _, item := range items {
		// per类型,检查allType和regexpType
		if limitType == config.LimitByPerHeaderType ||
			limitType == config.LimitByPerParamType ||
			limitType == config.LimitByPerConsumerType ||
			limitType == config.LimitByPerCookieType {
			if item.ConfigType == config.AllType || (item.ConfigType == config.RegexpType && item.Regexp.MatchString(key)) {
				return &item
			}
		}
		// 其他类型,直接比较key
		if item.Key == key {
			return &item
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

func rejected(config config.ClusterKeyRateLimitConfig, context LimitContext) {
	headers := make(map[string][]string)
	headers[RateLimitResetHeader] = []string{strconv.Itoa(context.reset)}
	if config.ShowLimitQuotaHeader {
		headers[RateLimitLimitHeader] = []string{strconv.Itoa(context.count)}
		headers[RateLimitRemainingHeader] = []string{strconv.Itoa(0)}
	}
	_ = proxywasm.SendHttpResponseWithDetail(
		config.RejectedCode, "cluster-key-rate-limit.rejected", util.ReconvertHeaders(headers), []byte(config.RejectedMsg), -1)
}
