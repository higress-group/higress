package main

import (
	"encoding/binary"
	"errors"
	"github.com/alibaba/higress/plugins/wasm-go/pkg/wrapper"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/tidwall/gjson"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	secondNano                = 1000 * 1000 * 1000
	minuteNano                = 60 * secondNano
	hourNano                  = 60 * minuteNano
	dayNano                   = 24 * hourNano
	tickMilliseconds   int64  = 500
	maxGetTokenRetry   int    = 20
	initialValue       uint64 = 0
	tokenBucketPrefix  string = "mse.token_bucket"
	lastRefilledPrefix string = "mse.last_refilled"
)

type KeyRateLimitConfig struct {
	ruleId        int
	limitKeys     map[string]LimitItem
	limitByHeader string
	limitByParam  string
}

type LimitItem struct {
	ruleId                int
	key                   string
	tokensPerRefill       uint64
	refillIntervalNanosec uint64
	maxTokens             uint64
}

var ruleId int = 0

func main() {
	wrapper.SetCtx(
		"key-rate-limit",
		wrapper.ParseConfigBy(parseConfig),
		wrapper.ProcessRequestHeadersBy(onHttpRequestHeaders),
	)
}

func parseConfig(json gjson.Result, config *KeyRateLimitConfig, log wrapper.Log) error {
	//解析配置规则
	config.limitKeys = make(map[string]LimitItem)
	limitKeys := json.Get("limit_keys").Array()
	limitMapping := map[string]int64{
		"query_per_second": secondNano,
		"query_per_minute": minuteNano,
		"query_per_hour":   hourNano,
		"query_per_day":    dayNano,
	}
	for _, item := range limitKeys {
		key := item.Get("key")
		if !key.Exists() || key.String() == "" {
			return errors.New("key name is required")
		}
		matched := false
		for field, nanoValue := range limitMapping {
			qps := item.Get(field)
			if qps.Exists() && qps.String() != "" {
				config.limitKeys[key.String()] = LimitItem{
					ruleId:                ruleId,
					key:                   key.String(),
					tokensPerRefill:       qps.Uint(),
					refillIntervalNanosec: uint64(nanoValue),
					maxTokens:             qps.Uint(),
				}
				matched = true
				break
			}
		}
		if !matched {
			return errors.New("one of 'query_per_second', 'query_per_minute', 'query_per_hour' or 'query_per_day' must be set")
		}
	}
	if len(config.limitKeys) == 0 {
		return errors.New("no limit keys found in configuration")
	}
	limitByHeader := json.Get("limit_by_header")
	if limitByHeader.Exists() {
		config.limitByHeader = limitByHeader.String()
	}
	limitByParam := json.Get("limit_by_param")
	if limitByParam.Exists() {
		config.limitByParam = limitByParam.String()
	}
	emptyHeader := config.limitByHeader == ""
	emptyParam := config.limitByParam == ""
	if (emptyHeader && emptyParam) || (!emptyHeader && !emptyParam) {
		return errors.New("only one of 'limit_by_param' and 'limit_by_header' can be set")
	}
	//利用解析配置规则进行令牌桶初始化
	initializeTokenBucket(config.limitKeys, log)
	config.ruleId = ruleId
	ruleId += 1
	//定时任务填充令牌
	wrapper.RegisteTickFunc(tickMilliseconds, func() {
		refillToken(config.limitKeys, log)
	})
	return nil
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config KeyRateLimitConfig, log wrapper.Log) types.Action {
	headerKey := config.limitByHeader
	paramKey := config.limitByParam
	var key string
	if headerKey != "" {
		header, err := proxywasm.GetHttpRequestHeader(headerKey)
		if err != nil {
			return types.ActionContinue
		}
		key = header
	} else {
		requestUrl, err := proxywasm.GetHttpRequestHeader(":path")
		if err != nil {
			return types.ActionContinue
		}
		if strings.Contains(requestUrl, paramKey) {
			param, parseErr := url.Parse(requestUrl)
			if parseErr != nil {
				return types.ActionContinue
			} else {
				params := param.Query()
				value, ok := params[paramKey]
				if ok && len(value) > 0 {
					key = value[0]
				}
			}
		}
	}
	if key == "" {
		return types.ActionContinue
	}
	limitKeys := config.limitKeys
	_, exists := limitKeys[key]
	if !exists {
		return types.ActionContinue
	}
	if !getToken(config.ruleId, key) {
		ctx.DontReadRequestBody()
		return tooManyRequest()
	}
	return types.ActionContinue
}

func tooManyRequest() types.Action {
	_ = proxywasm.SendHttpResponse(429, nil,
		[]byte("Too many requests,rate_limited\n"), -1)
	return types.ActionContinue
}

func initializeTokenBucket(rules map[string]LimitItem, log wrapper.Log) bool {
	for _, rule := range rules {
		lastRefilledKey := rule.GenerateLastRefilledKey()
		tokenBucketKey := rule.GenerateTokenBucketKey()
		initialBuf := make([]byte, 8)
		binary.LittleEndian.PutUint64(initialBuf, initialValue)
		maxTokenBuf := make([]byte, 8)
		binary.LittleEndian.PutUint64(maxTokenBuf, rule.maxTokens)
		_, _, err := proxywasm.GetSharedData(lastRefilledKey)
		if errors.Is(err, types.ErrorStatusNotFound) {
			_ = proxywasm.SetSharedData(lastRefilledKey, initialBuf, 0)
			_ = proxywasm.SetSharedData(tokenBucketKey, maxTokenBuf, 0)
			log.Infof("ratelimit rule created: id:%d, lastRefilledKey:%s, tokenBucketKey:%s, max_tokens:%d",
				rule.ruleId, lastRefilledKey, tokenBucketKey, rule.maxTokens)
			continue
		}
		for {
			_, lastUpdateCas, err := proxywasm.GetSharedData(lastRefilledKey)
			if err != nil {
				log.Warnf("failed to get lastRefilled, err: %v", err)
				return false
			}
			err = proxywasm.SetSharedData(lastRefilledKey, initialBuf, lastUpdateCas)
			if errors.Is(err, types.ErrorStatusCasMismatch) {
				continue
			}
			break
		}
		for {
			_, lastUpdateCas, err := proxywasm.GetSharedData(tokenBucketKey)
			if err != nil {
				log.Warnf("failed to get tokenBucket, err: %v", err)
				return false
			}
			err = proxywasm.SetSharedData(tokenBucketKey, maxTokenBuf, lastUpdateCas)
			if errors.Is(err, types.ErrorStatusCasMismatch) {
				continue
			}
			break
		}
		log.Infof("ratelimit rule reconfigured: id:%d, lastRefilledKey:%s, tokenBucketKey:%s, max_tokens:%d",
			rule.ruleId, lastRefilledKey, tokenBucketKey, rule.maxTokens)
	}
	return true
}

func refillToken(rules map[string]LimitItem, log wrapper.Log) {
	for _, rule := range rules {
		lastRefilledKey := rule.GenerateLastRefilledKey()
		tokenBucketKey := rule.GenerateTokenBucketKey()
		lastUpdateData, lastUpdateCas, err := proxywasm.GetSharedData(lastRefilledKey)
		if err != nil {
			log.Warnf("failed to get last update time of the local rate limit: %s", err)
			continue
		}
		lastUpdate := binary.LittleEndian.Uint64(lastUpdateData)
		now := time.Now().UnixNano()
		if uint64(now)-lastUpdate < rule.refillIntervalNanosec {
			continue
		}
		log.Debugf("ratelimit rule need refilled, id:%d, lastRefilledKey:%s, now:%d, last_update:%d",
			rule.ruleId, lastRefilledKey, now, lastUpdate)
		nowBuf := make([]byte, 8)
		binary.LittleEndian.PutUint64(nowBuf, uint64(now))
		err = proxywasm.SetSharedData(lastRefilledKey, nowBuf, lastUpdateCas)
		if errors.Is(err, types.ErrorStatusCasMismatch) {
			log.Debugf("ratelimit update lastRefilledKey casmismatch, "+
				"the bucket is going to be refilled by other VMs, id:%d, lastRefilledKey:%s", rule.ruleId, lastRefilledKey)
			continue
		}
		for {
			lastUpdateData, lastUpdateCas, err = proxywasm.GetSharedData(tokenBucketKey)
			if err != nil {
				log.Warnf("failed to get current local rate limit token bucket")
				break
			}
			tokenLeft := binary.LittleEndian.Uint64(lastUpdateData)
			tokenLeft += rule.tokensPerRefill
			if tokenLeft > rule.maxTokens {
				tokenLeft = rule.maxTokens
			}
			tokenLeftBuf := make([]byte, 8)
			binary.LittleEndian.PutUint64(tokenLeftBuf, tokenLeft)
			err = proxywasm.SetSharedData(tokenBucketKey, tokenLeftBuf, lastUpdateCas)
			if errors.Is(err, types.ErrorStatusCasMismatch) {
				continue
			}
			log.Debugf("ratelimit token refilled: id:%d, tokenBucketKey:%s, token left:%d",
				rule.ruleId, tokenBucketKey, tokenLeft)
			break
		}
	}
}

func getToken(ruleId int, key string) bool {
	tokenBucketKey := strconv.Itoa(ruleId) + tokenBucketPrefix + key
	for i := 0; i < maxGetTokenRetry; i++ {
		tokenBucketData, cas, err := proxywasm.GetSharedData(tokenBucketKey)
		if err != nil {
			continue
		}
		tokenLeft := binary.LittleEndian.Uint64(tokenBucketData)
		if tokenLeft == 0 {
			return false
		}
		tokenLeft -= 1
		tokenLeftBuf := make([]byte, 8)
		binary.LittleEndian.PutUint64(tokenLeftBuf, tokenLeft)
		err = proxywasm.SetSharedData(tokenBucketKey, tokenLeftBuf, cas)
		if err != nil {
			continue
		}
		return true
	}
	return true
}

func (l *LimitItem) GenerateLastRefilledKey() string {
	return strconv.Itoa(l.ruleId) + lastRefilledPrefix + l.key
}

func (l *LimitItem) GenerateTokenBucketKey() string {
	return strconv.Itoa(l.ruleId) + tokenBucketPrefix + l.key
}
