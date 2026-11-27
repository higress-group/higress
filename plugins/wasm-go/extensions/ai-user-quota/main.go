// Copyright (c) 2024-2025
// Beijing Baolande Software Corporation
// All Rights Reserved

package main

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"ai-user-quota/util"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/tokenusage"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/resp"
)

const (
	pluginName     = "ai-user-quota"
	consumerHeader = "x-mse-consumer"

	rejectReason = "user monthly token quota exceeded"

	// 请求上下文缓存键
	ctxConsumer   = "uq_consumer"
	ctxMeter      = "uq_meter"
	ctxLimit      = "uq_limit"
	ctxEpoch      = "uq_epoch"
	ctxCycleStart = "uq_cycle_start"
	ctxTTL        = "uq_ttl"
	ctxRespStatus = "uq_resp_status"

	// 读配置:单 key,不在脚本内拼 key
	readCfgScript = `return redis.call('HMGET', KEYS[1], 'limit', 'anchor', 'epoch')`
	// 读用量
	readUsedScript = `return redis.call('GET', KEYS[1])`
	// 累加用量并补 TTL
	incrUsedScript = `local v = redis.call('INCRBY', KEYS[1], ARGV[1])
if redis.call('TTL', KEYS[1]) < 0 then
  redis.call('EXPIRE', KEYS[1], ARGV[2])
end
return v`
)

func main() {}

func init() {
	wrapper.SetCtx(
		pluginName,
		wrapper.ParseConfig(parseConfig),
		wrapper.ProcessRequestHeaders(onRequestHeaders),
		wrapper.ProcessResponseHeaders(onResponseHeaders),
		wrapper.ProcessStreamingResponseBody(onStreamingResponseBody),
	)
}

func onRequestHeaders(ctx wrapper.HttpContext, cfg UserQuotaConfig) types.Action {
	ctx.DisableReroute()
	ctx.DontReadRequestBody()

	// 内部二跳路由直接放行,避免同一请求重复计量
	if skipInternalRoute(cfg.SkipRoutePrefixes) {
		return types.ActionContinue
	}
	consumer, err := proxywasm.GetHttpRequestHeader(consumerHeader)
	if err != nil || consumer == "" {
		return types.ActionContinue
	}
	meter := matchPathSuffix(ctx.Path(), cfg.EnablePathSuffixes)

	// 第一次调用:只读配额配置
	err = cfg.RedisClient.Eval(readCfgScript, 1, []interface{}{cfgKey(consumer)}, nil, func(response resp.Value) {
		if err := response.Error(); err != nil {
			log.Errorf("read user quota cfg failed, consumer: %s, error: %v", consumer, err)
			proxywasm.ResumeHttpRequest()
			return
		}
		fields := response.Array()
		if len(fields) != 3 {
			log.Errorf("unexpected quota cfg response, consumer: %s", consumer)
			proxywasm.ResumeHttpRequest()
			return
		}
		limit := int64(fields[0].Integer())
		anchor, ok := parseAnchor(fields[1].String())
		if !ok || limit <= 0 {
			// 未配置或已关闭
			proxywasm.ResumeHttpRequest()
			return
		}
		epoch := int64(fields[2].Integer())
		now := nowUTC8()
		cycleStart, nextCycleStart := calcCycle(anchor, now)
		ttl := cycleTTLSeconds(anchor, now)

		// 第二次调用:只读当前周期用量
		err := cfg.RedisClient.Eval(readUsedScript, 1, []interface{}{usedKey(consumer, epoch, cycleStart)}, nil, func(usedResp resp.Value) {
			if err := usedResp.Error(); err != nil {
				log.Errorf("read user quota used failed, consumer: %s, error: %v", consumer, err)
				proxywasm.ResumeHttpRequest()
				return
			}
			used := int64(usedResp.Integer())
			if used >= limit {
				rejectQuota(limit, used, nextCycleStart)
				return
			}
			ctx.SetContext(ctxConsumer, consumer)
			ctx.SetContext(ctxMeter, meter)
			ctx.SetContext(ctxLimit, limit)
			ctx.SetContext(ctxEpoch, epoch)
			ctx.SetContext(ctxCycleStart, cycleStart)
			ctx.SetContext(ctxTTL, ttl)
			proxywasm.ResumeHttpRequest()
		})
		if err != nil {
			log.Errorf("read user quota used dispatch failed, consumer: %s, error: %v", consumer, err)
			proxywasm.ResumeHttpRequest()
		}
	})
	if err != nil {
		log.Errorf("read user quota cfg dispatch failed, consumer: %s, error: %v", consumer, err)
		return types.ActionContinue
	}
	return types.HeaderStopAllIterationAndWatermark
}

func onResponseHeaders(ctx wrapper.HttpContext, cfg UserQuotaConfig) types.Action {
	if status, err := proxywasm.GetHttpResponseHeader(":status"); err == nil {
		ctx.SetContext(ctxRespStatus, status)
	}
	return types.ActionContinue
}

func onStreamingResponseBody(ctx wrapper.HttpContext, cfg UserQuotaConfig, data []byte, endOfStream bool) []byte {
	if usage := tokenusage.GetTokenUsage(ctx, data); usage.TotalToken > 0 {
		ctx.SetContext(tokenusage.CtxKeyInputToken, usage.InputToken)
		ctx.SetContext(tokenusage.CtxKeyOutputToken, usage.OutputToken)
	}
	if !endOfStream {
		return data
	}
	status, ok := ctx.GetContext(ctxRespStatus).(string)
	if !ok || status != "200" {
		return data
	}
	meter, ok := ctx.GetContext(ctxMeter).(bool)
	if !ok || !meter {
		return data
	}
	consumer, ok := ctx.GetContext(ctxConsumer).(string)
	if !ok {
		return data
	}
	limit, ok := ctx.GetContext(ctxLimit).(int64)
	if !ok || limit <= 0 {
		return data
	}
	inputToken := int64(0)
	if input, ok := ctx.GetContext(tokenusage.CtxKeyInputToken).(int64); ok {
		inputToken = input
	}
	outputToken := int64(0)
	if output, ok := ctx.GetContext(tokenusage.CtxKeyOutputToken).(int64); ok {
		outputToken = output
	}
	total := inputToken + outputToken
	if total <= 0 {
		return data
	}
	epoch, ok := ctx.GetContext(ctxEpoch).(int64)
	if !ok {
		return data
	}
	cycleStart, ok := ctx.GetContext(ctxCycleStart).(time.Time)
	if !ok {
		return data
	}
	ttl, ok := ctx.GetContext(ctxTTL).(int64)
	if !ok {
		return data
	}

	// 先累加用量,再写待同步标记;两键可能跨 slot,分开调用
	err := cfg.RedisClient.Eval(
		incrUsedScript, 1,
		[]interface{}{usedKey(consumer, epoch, cycleStart)},
		[]interface{}{total, ttl},
		nil,
	)
	if err != nil {
		log.Errorf("increase user quota used dispatch failed, consumer: %s, error: %v", consumer, err)
		return data
	}

	member := dirtyMember(consumer, epoch, cycleStart)
	if err := cfg.RedisClient.SAdd(
		dirtyCurrentKey(), []interface{}{member}, nil,
	); err != nil {
		log.Errorf("mark user quota dirty dispatch failed, consumer: %s, error: %v", consumer, err)
	}
	return data
}

func rejectQuota(limit, used int64, nextCycleStart time.Time) {
	body := fmt.Sprintf(
		`{"message":"User monthly token quota exceeded: limit %d tokens, used %d tokens, resets at %s","reason":"%s"}`,
		limit, used, nextCycleStart.Format("2006-01-02 15:04:05"), rejectReason)
	if err := util.SendResponse(http.StatusTooManyRequests, "ai-user-quota.rejected", util.MimeTypeApplicationJson, body); err != nil {
		log.Errorf("send user quota rejection failed: %v", err)
	}
}

// skipInternalRoute 路由名命中前缀列表时跳过
func skipInternalRoute(prefixes []string) bool {
	if len(prefixes) == 0 {
		return false
	}
	routeName, err := util.GetRouteName()
	if err != nil {
		return false
	}
	if matchRoutePrefix(routeName, prefixes) {
		log.Debugf("skip user quota on internal route: %s", routeName)
		return true
	}
	return false
}

// matchRoutePrefix 剥离命名空间前缀后按前缀匹配路由名
func matchRoutePrefix(routeName string, prefixes []string) bool {
	matchName := util.TrimNamespacePrefix(routeName)
	for _, prefix := range prefixes {
		if prefix != "" && strings.HasPrefix(matchName, prefix) {
			return true
		}
	}
	return false
}

// matchPathSuffix 路径命中计量后缀
func matchPathSuffix(rawPath string, suffixes []string) bool {
	path := rawPath
	if parsed, err := url.Parse(rawPath); err == nil && parsed.Path != "" {
		path = parsed.Path
	}
	for _, suffix := range suffixes {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}
	return false
}

func parseAnchor(value string) (time.Time, bool) {
	anchor, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, false
	}
	return anchor, true
}
