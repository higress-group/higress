package main

import (
	"fmt"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/nginx-rewrite-compatible/pkg"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

func main() {}

func init() {
	wrapper.SetCtx(
		"nginx-rewrite-compatible",
		wrapper.ParseConfigBy(parseConfig),
		wrapper.ProcessRequestHeadersBy(onHttpRequestHeaders),
	)
}

// @Name nginx-rewrite-compatible
// @Category custom
// @Phase UNSPECIFIED_PHASE
// @Priority 100
// @Title zh-CN Nginx Rewrite 兼容迁移
// @Title en-US Nginx Rewrite Compatibility Migration
// @Description zh-CN 提供与 nginx rewrite + set 指令组合等价的路径重写、查询串重写，以及通过 arg_/http_/cookie_ 前缀修改请求参数、请求头和 Cookie 的能力，用于安全迁移到 Higress。
// @Description en-US Provides path rewrite, query rewrite, and nginx-style arg_/http_/cookie_ variable handling for request parameters, headers, and cookies, enabling safe migration to Higress.
// @Version 1.0.0
func parseConfig(json gjson.Result, config *pkg.PluginConfig, logger log.Log) error {
	if err := config.FromJson(json); err != nil {
		return fmt.Errorf("failed to parse plugin config: %w", err)
	}
	return nil
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config pkg.PluginConfig, logger log.Log) types.Action {
	changed, err := config.Apply(ctx, logger)
	if err != nil {
		logger.Errorf("failed to apply rewrite rules: %v", err)
		return types.ActionContinue
	}
	if changed {
		logger.Debugf("rewrite rules applied successfully")
	}
	return types.ActionContinue
}
