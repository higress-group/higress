package main

import (
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
}

func parseConfig(json gjson.Result, config *CacheControlConfig, log log.Log) error {
	suffix := json.Get("suffix").String()
	if suffix != "" {
		parts := strings.Split(suffix, "|")
		config.suffix = parts
	}

	config.expires = json.Get("expires").String()

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
			maxAge, _ := strconv.ParseInt(config.expires, 10, 64)
			currentTime := time.Now()
			expireTime := currentTime.Add(time.Duration(maxAge) * time.Second)
			proxywasm.AddHttpResponseHeader("Expires", expireTime.UTC().Format(http.TimeFormat))
			proxywasm.AddHttpResponseHeader("Cache-Control", "max-age="+strconv.FormatInt(maxAge, 10))
		}
	}
	return types.ActionContinue
}
