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

	"ai-token-ratelimit/config"
	"ai-token-ratelimit/util"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/tokenusage"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
	"github.com/tidwall/resp"
)

func main() {}

func init() {
	wrapper.SetCtx(
		"ai-token-ratelimit",
		wrapper.ParseConfig(parseConfig),
		wrapper.ProcessRequestHeaders(onHttpRequestHeaders),
		wrapper.ProcessStreamingResponseBody(onHttpStreamingBody),
	)
}

const (
	RedisKeyPrefix string = "higress-token-ratelimit"
	// 使用 {rule_name} hash tag 让多规则多键操作在 Redis Cluster 下落到同一 slot
	// AiTokenGlobalRateLimitFormat  全局限流模式 redis key 为 RedisKeyPrefix:{限流规则名称}:global_threshold:时间窗口
	AiTokenGlobalRateLimitFormat = RedisKeyPrefix + ":{%s}:global_threshold:%d"
	// AiTokenRateLimitFormat 规则限流模式 redis key 为 RedisKeyPrefix:{限流规则名称}:限流类型:时间窗口:限流key名称:限流key对应的实际值
	AiTokenRateLimitFormat = RedisKeyPrefix + ":{%s}:%s:%d:%s:%s"
	// MultiKeyRequestPhaseScript 多规则请求阶段只读检查脚本
	// KEYS = [key1, ..., keyN]
	// ARGV = [threshold1, window1, threshold2, window2, ..., thresholdN, windowN]
	// 返回嵌套数组 {{threshold_i, current_i, ttl_i}, ...}
	MultiKeyRequestPhaseScript = `
		local results = {}
		for i = 1, #KEYS do
			local threshold = tonumber(ARGV[2*i - 1])
			local window    = tonumber(ARGV[2*i])
			local current = redis.call('get', KEYS[i])
			local ttl = redis.call('ttl', KEYS[i])

			-- 键不存在时，返回初始状态（计数0，窗口时间为过期时间）
			if not current then
				table.insert(results, {threshold, 0, window})
			else
				-- 修复异常过期时间（确保窗口有效）
				if ttl < 0 then
					ttl = window
				end
				-- 返回窗口状态：阈值、当前计数、剩余时间
				table.insert(results, {threshold, tonumber(current), ttl})
			end
		end
		return results
	`
	// MultiKeyResponsePhaseScript 多规则响应阶段累加脚本（仅 ai-token-ratelimit 使用）
	// KEYS = [key1, ..., keyN]
	// ARGV = [threshold1, window1, count1, ..., thresholdN, windowN, countN]
	// 每条规则独立判断 current <= threshold 才累加；返回 KEYS 数量
	MultiKeyResponsePhaseScript = `
		for i = 1, #KEYS do
			local threshold = tonumber(ARGV[3*i - 2])
			local window    = tonumber(ARGV[3*i - 1])
			local added     = tonumber(ARGV[3*i])
			local current = tonumber(redis.call('get', KEYS[i]) or "0")
			if current <= threshold then
				current = redis.call('incrby', KEYS[i], added)
				if current == added then
					redis.call('expire', KEYS[i], window)
				else
					local ttl = redis.call('ttl', KEYS[i])
					if ttl < 0 then redis.call('expire', KEYS[i], window) end
				end
			end
		end
		return #KEYS
	`

	LimitRedisContextKey = "LimitRedisContext"

	CookieHeader = "cookie"

	RateLimitResetHeader = "X-TokenRateLimit-Reset" // 限流重置时间（触发限流时返回）

	TokenRateLimitCount = "token_ratelimit_count" // metric name
)

type LimitContext struct {
	count     int
	remaining int
	reset     int
}

// MatchedRule 表示请求阶段命中的单条限流规则（global 或 rule_item）
type MatchedRule struct {
	key    string // 完整 Redis key
	count  int64  // 时间窗口内的限额（与 LimitConfigItem.Count / GlobalThreshold.Count 同义）
	window int64  // 时间窗口大小（秒）
}

// LimitRedisContext 暂存请求阶段命中的全部规则，供响应阶段多键 INCRBY 使用
type LimitRedisContext struct {
	rules []MatchedRule
}

func parseConfig(json gjson.Result, cfg *config.AiTokenRateLimitConfig) error {
	err := config.InitRedisClusterClient(json, cfg)
	if err != nil {
		return err
	}
	err = config.ParseAiTokenRateLimitConfig(json, cfg)
	if err != nil {
		return err
	}
	// Metric settings
	cfg.CounterMetrics = make(map[string]proxywasm.MetricCounter)
	return nil
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, cfg config.AiTokenRateLimitConfig) types.Action {
	ctx.DisableReroute()

	matched := collectMatchedRules(ctx, cfg)
	if len(matched) == 0 {
		// 无任何规则命中：直接放行，不发起 Redis 调用
		return types.ActionContinue
	}

	n := len(matched)
	keys := make([]interface{}, n)
	args := make([]interface{}, 0, n*2)
	for i, m := range matched {
		keys[i] = m.key
		args = append(args, m.count, m.window)
	}

	// 暂存命中规则，供响应阶段多键 INCRBY 使用
	ctx.SetContext(LimitRedisContextKey, LimitRedisContext{rules: matched})

	err := cfg.RedisClient.Eval(MultiKeyRequestPhaseScript, n, keys, args, func(response resp.Value) {
		arr := response.Array()
		if len(arr) != n {
			log.Errorf("redis response length mismatch: got %d, want %d", len(arr), n)
			_ = proxywasm.ResumeHttpRequest()
			return
		}

		// 单次遍历：触发即 return；未触发则放行。
		// ai-token-ratelimit 不对外暴露 LimitContext（与 cluster-key-ratelimit 不同，
		// 后者通过 X-RateLimit-* 头可观测 tightest 选择），因此此处不再写入 Context。
		// tightestRatio 用 math.MaxFloat64 初始化，避免依赖 arr[0] 形状（防 0/0 NaN）。
		tightestRatio := math.MaxFloat64

		for _, ruleResult := range arr {
			ruleState := ruleResult.Array()
			if len(ruleState) != 3 {
				log.Errorf("redis sub-array length mismatch: got %d, want 3", len(ruleState))
				_ = proxywasm.ResumeHttpRequest()
				return
			}
			threshold, current, ttl := ruleState[0].Integer(), ruleState[1].Integer(), ruleState[2].Integer()

			if current > threshold {
				// 命中触发的第一条规则（按 collectMatchedRules 顺序，global 优先）
				ctx.SetUserAttribute("token_ratelimit_status", "limited")
				_ = ctx.WriteUserAttributeToLogWithKey(wrapper.AILogKey)
				rejected(cfg, LimitContext{
					count:     threshold,
					remaining: threshold - current,
					reset:     ttl,
				})
				return
			}

			if ratio := float64(threshold-current) / float64(threshold); ratio < tightestRatio {
				tightestRatio = ratio
			}
		}

		_ = proxywasm.ResumeHttpRequest()
	})

	if err != nil {
		log.Errorf("redis call failed: %v", err)
		return types.ActionContinue
	}
	return types.HeaderStopAllIterationAndWatermark
}

func onHttpStreamingBody(ctx wrapper.HttpContext, cfg config.AiTokenRateLimitConfig, data []byte, endOfStream bool) []byte {
	if usage := tokenusage.GetTokenUsage(ctx, data); usage.TotalToken > 0 {
		ctx.SetContext(tokenusage.CtxKeyInputToken, usage.InputToken)
		ctx.SetContext(tokenusage.CtxKeyOutputToken, usage.OutputToken)
	}
	if !endOfStream {
		return data
	}

	inputTokenRaw := ctx.GetContext(tokenusage.CtxKeyInputToken)
	outputTokenRaw := ctx.GetContext(tokenusage.CtxKeyOutputToken)
	if inputTokenRaw == nil || outputTokenRaw == nil {
		return data
	}
	inputToken, ok1 := inputTokenRaw.(int64)
	outputToken, ok2 := outputTokenRaw.(int64)
	if !ok1 || !ok2 {
		return data
	}

	limitRedisContextRaw := ctx.GetContext(LimitRedisContextKey)
	if limitRedisContextRaw == nil {
		return data
	}
	limitRedisContext, ok := limitRedisContextRaw.(LimitRedisContext)
	if !ok || len(limitRedisContext.rules) == 0 {
		return data
	}

	// 多键 INCRBY：每条规则一组 (threshold, window, added)
	n := len(limitRedisContext.rules)
	keys := make([]interface{}, n)
	args := make([]interface{}, 0, n*3)
	added := inputToken + outputToken
	for i, r := range limitRedisContext.rules {
		keys[i] = r.key
		args = append(args, r.count, r.window, added)
	}

	err := cfg.RedisClient.Eval(MultiKeyResponsePhaseScript, n, keys, args, nil)
	if err != nil {
		log.Errorf("redis call failed: %v", err)
	}
	return data
}

// collectMatchedRules 遍历 global_threshold 和 rule_items，返回所有命中规则。
// 顺序：global_threshold（如有）→ rule_items 中所有命中项（按数组顺序追加）。
// 该顺序决定了"触发时优先报告哪条规则"以及"未触发时 tightest 的选择范围"。
func collectMatchedRules(ctx wrapper.HttpContext, cfg config.AiTokenRateLimitConfig) []MatchedRule {
	var matched []MatchedRule

	if cfg.GlobalThreshold != nil {
		matched = append(matched, MatchedRule{
			key:    fmt.Sprintf(AiTokenGlobalRateLimitFormat, cfg.RuleName, cfg.GlobalThreshold.TimeWindow),
			count:  cfg.GlobalThreshold.Count,
			window: cfg.GlobalThreshold.TimeWindow,
		})
	}

	for _, ruleItem := range cfg.RuleItems {
		val, hitRule, hitItem := hitRateRuleItem(ctx, ruleItem)
		if hitRule != nil && hitItem != nil {
			matched = append(matched, MatchedRule{
				key:    fmt.Sprintf(AiTokenRateLimitFormat, cfg.RuleName, hitRule.LimitType, hitItem.TimeWindow, hitRule.Key, val),
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
		val, err := proxywasm.GetHttpRequestHeader(util.ConsumerHeader)
		if err != nil {
			return logDebugAndReturnEmpty("failed to get request header %s: %v", util.ConsumerHeader, err)
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

func generateMetricName(route, cluster, model, consumer, metricName string) string {
	return fmt.Sprintf("route.%s.upstream.%s.model.%s.consumer.%s.metric.%s", route, cluster, model, consumer, metricName)
}

func rejected(cfg config.AiTokenRateLimitConfig, context LimitContext) {
	headers := make(map[string][]string)
	headers[RateLimitResetHeader] = []string{strconv.Itoa(context.reset)}
	_ = proxywasm.SendHttpResponseWithDetail(
		cfg.RejectedCode, "ai-token-ratelimit.rejected", util.ReconvertHeaders(headers), []byte(cfg.RejectedMsg), -1)

	route, _ := util.GetRouteName()
	cluster, _ := util.GetClusterName()
	consumer, _ := util.GetConsumer()
	cfg.IncrementCounter(generateMetricName(route, cluster, "none", consumer, TokenRateLimitCount), 1)
}
