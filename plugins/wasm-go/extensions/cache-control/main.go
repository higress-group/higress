package main

import (
	"fmt"
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

func init() {
	wrapper.SetCtx(
		"cache-control",
		wrapper.ParseConfigBy(parseConfig),
		wrapper.ProcessRequestHeadersBy(onHttpRequestHeaders),
		wrapper.ProcessResponseHeadersBy(onHttpResponseHeaders),
	)
}

type CacheControlConfig struct {
	suffix  []string
	expires string
	maxAge  int64
}

const maxCacheAgeSeconds = int64(1<<63-1) / int64(time.Second)

func parseConfig(json gjson.Result, config *CacheControlConfig, log log.Log) error {
	suffix := json.Get("suffix").String()
	if suffix != "" {
		parts := strings.Split(suffix, "|")
		config.suffix = parts
	}

	config.expires = json.Get("expires").String()
	switch config.expires {
	case "":
		return fmt.Errorf("expires is required")
	case "max", "epoch":
	default:
		maxAge, err := strconv.ParseInt(config.expires, 10, 64)
		if err != nil {
			return fmt.Errorf("expires %q is not a valid integer: %w", config.expires, err)
		}
		if maxAge < 0 {
			return fmt.Errorf("expires must not be negative: %d", maxAge)
		}
		if maxAge > maxCacheAgeSeconds {
			return fmt.Errorf("expires exceeds the maximum supported value of %d seconds: %d", maxCacheAgeSeconds, maxAge)
		}
		config.maxAge = maxAge
	}

	log.Infof("suffix: %q, expires: %s", config.suffix, config.expires)
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
			currentTime := time.Now()
			expireTime := currentTime.Add(time.Duration(config.maxAge) * time.Second)
			proxywasm.AddHttpResponseHeader("Expires", expireTime.UTC().Format(http.TimeFormat))
			proxywasm.AddHttpResponseHeader("Cache-Control", "max-age="+strconv.FormatInt(config.maxAge, 10))
		}
	}
	return types.ActionContinue
}
