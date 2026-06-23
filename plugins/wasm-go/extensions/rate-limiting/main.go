package main

import (
	"encoding/binary"
	"fmt"
	"github.com/alibaba/higress/plugins/wasm-go/pkg/wrapper"
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm"
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/tidwall/gjson"
	"regexp"
	"strings"
	"time"
)

// -----------------------------------------------------------------------------
// Utils
// -----------------------------------------------------------------------------

func max(a int64, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func getProperty(namespace string, property string) string {
	bytes, err := proxywasm.GetProperty([]string{namespace, property})
	if err != nil {
		return string(bytes)
	}
	return ""
}

// 获取ip地址
func getClientIP() string {
	// 优先从EO提供的real-ip请求头中获取IP地址
	realIp, _ := proxywasm.GetHttpRequestHeader("real-ip")
	if realIp != "" {
		return realIp
	}
	// 优先从X-Forwarded-For请求头中获取IP地址
	xForwardedFor, _ := proxywasm.GetHttpRequestHeader("X-Forwarded-For")
	if xForwardedFor != "" {
		ips := strings.Split(xForwardedFor, ",")
		// 返回最后一个IP地址，通常是客户端的IP
		if len(ips) > 0 {
			// 返回第一个IP地址，通常是客户端的IP
			return strings.TrimSpace(ips[0])
		}
	}
	return "127.0.0.1"
}

// -----------------------------------------------------------------------------
// Timestamps
// -----------------------------------------------------------------------------

type Timestamps map[string]int64

func getTimestamps(t time.Time) *Timestamps {
	ts := Timestamps{}

	ye, mo, da := t.Year(), t.Month(), t.Day()
	ho, mi, se, lo := t.Hour(), t.Minute(), t.Second(), t.Location()

	ts["now"] = t.Unix()
	ts["second"] = time.Date(ye, mo, da, ho, mi, se, 0, lo).Unix()
	ts["minute"] = time.Date(ye, mo, da, ho, mi, 0, 0, lo).Unix()
	ts["hour"] = time.Date(ye, mo, da, ho, 0, 0, 0, lo).Unix()
	ts["day"] = time.Date(ye, mo, da, 0, 0, 0, 0, lo).Unix()
	ts["month"] = time.Date(ye, mo, 1, 0, 0, 0, 0, lo).Unix()
	ts["year"] = time.Date(ye, 1, 1, 0, 0, 0, 0, lo).Unix()

	return &ts
}

var expiration map[string]int64
var xRateLimitLimit map[string]string
var xRateLimitRemaining map[string]string

func init() {
	expiration = map[string]int64{
		"second": 1,
		"minute": 60,
		"hour":   3600,
		"day":    86400,
		"month":  2592000,
		"year":   31536000,
	}

	time.LoadLocation("")

	xRateLimitLimit = make(map[string]string)
	xRateLimitRemaining = make(map[string]string)

	for k, _ := range expiration {
		t := strings.Title(k)

		xRateLimitLimit[k] = "X-RateLimit-Limit-" + t
		xRateLimitRemaining[k] = "X-RateLimit-Remaining-" + t
	}

}

type MyConfig struct {
	limits            map[string]int64
	limitBy           string // 限制方式
	HeaderName        string // 如果是header限制，这块指定定义好的header名称
	HideClientHeaders bool   // 是否将限制信息添加到头
}

func parseConfig(json gjson.Result, config *MyConfig, log wrapper.Log) error {
	config.limits = map[string]int64{
		"second": json.Get("second").Int(),
		"minute": json.Get("minute").Int(),
		"hour":   json.Get("hour").Int(),
		"day":    json.Get("day").Int(),
		"month":  json.Get("month").Int(),
		"year":   json.Get("year").Int(),
	}

	config.limitBy = json.Get("limitBy").String()
	config.HideClientHeaders = json.Get("hideClientHeaders").Bool()
	return nil
}

func getForwardedIp() string {
	return getClientIP()
}

func getLocalKey(ctx MyConfig, id Identifier, period string, date int64) string {
	localKey := fmt.Sprintf("mse_rate_limit:%v:%v:%v", id, date, period)
	return fmt.Sprintf(localKey)
}

type Identifier string

func getIdentifier(config MyConfig) Identifier {
	id := ""
	if config.limitBy == "header" { // 某个请求头
		header, err := proxywasm.GetHttpRequestHeader(config.HeaderName)
		if err != nil {
			id = header
		}
	} else if config.limitBy == "ip" { // IP
		id = getForwardedIp()
	} else {
		id = "global" // 全局
	}
	return Identifier(id)
}

type Usage struct {
	limit     int64
	remaining int64
	usage     int64
	cas       uint32
}

func localPolicyUsage(ctx MyConfig, id Identifier, period string, ts *Timestamps) (int64, uint32, error) {
	cacheKey := getLocalKey(ctx, id, period, (*ts)[period])

	value, cas, err := proxywasm.GetSharedData(cacheKey)
	if err != nil {
		if err == types.ErrorStatusNotFound {
			return 0, 0, nil
		}
		return 0, 0, err
	}

	ret := int64(binary.LittleEndian.Uint64(value))
	return ret, cas, nil
}

func localPolicyIncrement(ctx MyConfig, id Identifier, counters map[string]Usage, ts *Timestamps) {
	for period, usage := range counters {
		cacheKey := getLocalKey(ctx, id, period, (*ts)[period])

		buf := make([]byte, 8)
		value := usage.usage
		cas := usage.cas

		saved := false
		var err error
		for i := 0; i < 10; i++ { // cas冲突时会重试10次
			binary.LittleEndian.PutUint64(buf, uint64(value+1))
			err = proxywasm.SetSharedData(cacheKey, buf, cas)
			if err == nil {
				saved = true
				break
			} else if err == types.ErrorStatusCasMismatch {
				// Get updated value, updated cas and retry
				buf, cas, err = proxywasm.GetSharedData(cacheKey)
				value = int64(binary.LittleEndian.Uint64(buf))
			} else {
				break
			}
		}
		if !saved {
			proxywasm.LogErrorf("could not increment counter for period '%v': %v", period, err)
		}
	}
}

func getUsage(ctx MyConfig, id Identifier, ts *Timestamps) (map[string]Usage, string, error) {
	counters := make(map[string]Usage)
	stop := ""

	for period, limit := range ctx.limits {
		if limit == -1 {
			continue
		}

		curUsage, cas, err := localPolicyUsage(ctx, id, period, ts)
		if err != nil {
			return counters, period, err
		}

		// What is the current usage for the configured limit name?
		remaining := limit - int64(curUsage)

		// Recording usage
		counters[period] = Usage{
			limit:     limit,
			remaining: remaining,
			usage:     curUsage,
			cas:       cas,
		}

		if remaining <= 0 {
			stop = period
		}
	}

	return counters, stop, nil
}

func processUsage(ctx MyConfig, counters map[string]Usage, stop string, ts *Timestamps) types.Action {
	var headers map[string]string
	reset := int64(0)

	now := (*ts)["now"]
	if !ctx.HideClientHeaders {
		headers = make(map[string]string)
		limit := int64(0)
		window := int64(0)
		remaining := int64(0)

		for k, v := range counters {
			curLimit := v.limit
			curWindow := expiration[k]
			curRemaining := v.remaining

			if stop == "" || stop == k {
				curRemaining--
			}
			curRemaining = max(0, curRemaining)

			if (limit == 0) ||
				(curRemaining < remaining) ||
				(curRemaining == remaining && curWindow > window) {

				limit = curLimit
				window = curWindow
				remaining = curRemaining

				reset = max(1, window-(now-((*ts)[k])))
			}

			headers[xRateLimitLimit[k]] = fmt.Sprintf("%d", curLimit)
			headers[xRateLimitRemaining[k]] = fmt.Sprintf("%d", curRemaining)
		}

		headers["RateLimit-Limit"] = fmt.Sprintf("%d", limit)
		headers["RateLimit-Remaining"] = fmt.Sprintf("%d", remaining)
		headers["RateLimit-Reset"] = fmt.Sprintf("%d", reset)
	}
	if stop != "" {
		pairs := [][2]string{}

		if !ctx.HideClientHeaders {
			if headers != nil {
				for k, v := range headers {
					pairs = append(pairs, [2]string{k, v})
				}
			}
		}
		pairs = append(pairs, [2]string{"Retry-After", fmt.Sprintf("%d", reset)})

		if err := proxywasm.SendHttpResponse(429, pairs, []byte("Go informs: API rate limit exceeded!"), -1); err != nil {
			panic(err)
		}
		return types.ActionPause
	}

	if headers != nil {
		for s := range headers {
			proxywasm.AddHttpRequestHeader(s, headers[s])
		}

	}

	return types.ActionContinue
}

var staticResourceAccept = []string{"application/font", "image/", ""}
var staticResourceExt = []string{".png", ".jpg", ".jpeg", ".gif", ".js", ".css", ".ico", ".woff", ".woff2", ".json", "html", "htm"}
var regexResource = []string{".html(\\?.*)?$", ".js(\\?.*)?$", ".css(\\?.*)?$", ".htm(\\?.*)?$", ".woff(\\?.*)?$", ".woff2(\\?.*)?$", ".jpg", ".gif", ".jpeg", "png"}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config MyConfig, log wrapper.Log) types.Action {
	// 过滤静态资源
	target := ctx.Path()
	for _, item1 := range regexResource {
		r := regexp.MustCompile(item1)
		if r.MatchString(target) {
			return types.ActionContinue
		}
	}

	ts := getTimestamps(time.Now())

	// TODO Add authenticated credential id support
	id := getIdentifier(config)
	counters, stop, err := getUsage(config, id, ts)

	if err != nil {
		proxywasm.LogErrorf("err:%v", err)
		return types.ActionContinue
	}
	fmt.Println("counters=", counters)
	if counters != nil {
		action := processUsage(config, counters, stop, ts)
		if action != types.ActionContinue {
			return action
		}

		localPolicyIncrement(config, id, counters, ts)
	}

	return types.ActionContinue
}

// 全局限流
func main() {
	wrapper.SetCtx(
		// 全局限流插件
		"rate-limiting-plugin",
		wrapper.ParseConfigBy(parseConfig),
		wrapper.ProcessRequestHeadersBy(onHttpRequestHeaders),
	)
}
