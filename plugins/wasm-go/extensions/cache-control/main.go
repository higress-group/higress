package main

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

func main() {}

// maxAgeUpperBound is the largest expires value in seconds that can be
// converted to a time.Duration without overflow.
const maxAgeUpperBound int64 = math.MaxInt64 / int64(time.Second)

var errInvalidExpires = fmt.Errorf("expires must be max, epoch, or an integer between 0 and %d", maxAgeUpperBound)

func init() {
	wrapper.SetCtx(
		"cache-control",
		wrapper.ParseConfigBy(parseConfig),
		wrapper.ProcessRequestHeadersBy(onHttpRequestHeaders),
		wrapper.ProcessResponseHeadersBy(onHttpResponseHeaders),
	)
}

type CacheControlConfig struct {
	suffix []string
	// expires keeps the special-value markers "max" and "epoch";
	// it stays empty for numeric configurations.
	expires string
	// maxAgeSeconds holds the validated numeric expiration in seconds.
	maxAgeSeconds int64
}

// parseExpiresSeconds accepts only a plain non-negative decimal integer
// within [0, maxAgeUpperBound]. It works on the raw text and never goes
// through float conversions, so fractional or out-of-range inputs are
// rejected instead of being silently truncated.
func parseExpiresSeconds(raw string) (int64, error) {
	if raw == "" {
		return 0, errInvalidExpires
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] < '0' || raw[i] > '9' {
			return 0, errInvalidExpires
		}
	}
	seconds, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || seconds > uint64(maxAgeUpperBound) {
		return 0, errInvalidExpires
	}
	return int64(seconds), nil
}

func parseConfig(json gjson.Result, config *CacheControlConfig, log log.Log) error {
	suffix := json.Get("suffix").String()
	if suffix != "" {
		parts := strings.Split(suffix, "|")
		config.suffix = parts
	}

	expires := json.Get("expires")
	switch expires.Type {
	case gjson.String:
		switch expires.Str {
		case "max", "epoch":
			config.expires = expires.Str
		default:
			seconds, err := parseExpiresSeconds(expires.Str)
			if err != nil {
				return err
			}
			config.maxAgeSeconds = seconds
		}
	case gjson.Number:
		// Check the raw decimal text: a JSON number may carry a fraction
		// or exponent that must be rejected instead of truncated.
		seconds, err := parseExpiresSeconds(expires.Raw)
		if err != nil {
			return err
		}
		config.maxAgeSeconds = seconds
	default:
		return errInvalidExpires
	}

	if config.expires != "" {
		log.Infof("suffix: %q, expires: %s", config.suffix, config.expires)
	} else {
		log.Infof("suffix: %q, expires: %d seconds", config.suffix, config.maxAgeSeconds)
	}
	return nil
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config CacheControlConfig, log log.Log) types.Action {
	path := ctx.Path()
	if strings.Contains(path, "?") {
		path = strings.Split(path, "?")[0]
	}
	ctx.SetContext("path", path)
	log.Debugf("path: %s", path)

	return types.ActionContinue
}

func onHttpResponseHeaders(ctx wrapper.HttpContext, config CacheControlConfig, log log.Log) types.Action {
	hit := false
	if len(config.suffix) == 0 {
		hit = true
	} else {
		path, ok := ctx.GetContext("path").(string)
		if !ok {
			log.Error("failed to get request path")
			return types.ActionContinue
		}

		for _, part := range config.suffix {
			if strings.HasSuffix(path, "."+part) {
				hit = true
				break
			}
		}
	}
	if hit {
		if config.expires == "max" {
			proxywasm.AddHttpResponseHeader("Expires", "Thu, 31 Dec 2037 23:55:55 GMT")
			proxywasm.AddHttpResponseHeader("Cache-Control", "max-age=315360000")
		} else if config.expires == "epoch" {
			proxywasm.AddHttpResponseHeader("Expires", "Thu, 01 Jan 1970 00:00:01 GMT")
			proxywasm.AddHttpResponseHeader("Cache-Control", "no-cache")
		} else {
			// maxAgeSeconds was validated at configuration time; invalid
			// values never reach the response path.
			expireTime := time.Now().Add(time.Duration(config.maxAgeSeconds) * time.Second)
			proxywasm.AddHttpResponseHeader("Expires", expireTime.UTC().Format(http.TimeFormat))
			proxywasm.AddHttpResponseHeader("Cache-Control", "max-age="+strconv.FormatInt(config.maxAgeSeconds, 10))
		}
	}
	return types.ActionContinue
}
