package main

import (
	"fmt"
	"strings"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/frontend-gray/config"
	"github.com/alibaba/higress/plugins/wasm-go/extensions/frontend-gray/util"
	"github.com/higress-group/wasm-go/pkg/log"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

func main() {}

func init() {
	wrapper.SetCtx(
		"frontend-gray",
		wrapper.ParseConfig(parseConfig),
		wrapper.ProcessRequestHeaders(onHttpRequestHeaders),
		wrapper.ProcessResponseHeaders(onHttpResponseHeader),
		wrapper.ProcessResponseBody(onHttpResponseBody),
	)
}

func parseConfig(json gjson.Result, grayConfig *config.GrayConfig) error {
	// 解析json 为GrayConfig
	if err := config.JsonToGrayConfig(json, grayConfig); err != nil {
		log.Errorf("failed to parse config: %v", err)
		return err
	}

	return nil
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, grayConfig config.GrayConfig) types.Action {
	requestPath := util.GetRequestPath()
	enabledGray := util.IsGrayEnabled(requestPath, &grayConfig)
	ctx.SetContext(config.EnabledGray, enabledGray)
	route, _ := util.GetRouteName()

	if !enabledGray {
		log.Infof("route: %s, gray not enabled, requestPath: %v", route, requestPath)
		ctx.DontReadRequestBody()
		return types.ActionContinue
	}

	cookie, _ := proxywasm.GetHttpRequestHeader("cookie")
	isHtmlRequest := util.CheckIsHtmlRequest(requestPath)
	ctx.SetContext(config.IsHtmlRequest, isHtmlRequest)
	isIndexRequest := util.IsIndexRequest(requestPath, grayConfig.IndexPaths)
	ctx.SetContext(config.IsIndexRequest, isIndexRequest)
	hasRewrite := len(grayConfig.Rewrite.File) > 0 || len(grayConfig.Rewrite.Index) > 0
	grayKeyValueByCookie := util.GetCookieValue(cookie, grayConfig.GrayKey)
	grayKeyValueByHeader, _ := proxywasm.GetHttpRequestHeader(grayConfig.GrayKey)
	// 优先从cookie中获取，否则从header中获取
	grayKeyValue := util.GetGrayKey(grayKeyValueByCookie, grayKeyValueByHeader, grayConfig.GraySubKey)
	// 如果有重写的配置，则进行重写
	if hasRewrite {
		// 禁止重新路由，要在更改Header之前操作，否则会失效
		ctx.DisableReroute()
	}
	frontendVersion := util.GetCookieValue(cookie, config.XHigressTag)

	if grayConfig.UniqueGrayTagConfigured || grayConfig.GrayWeight > 0 {
		ctx.SetContext(grayConfig.UniqueGrayTag, util.GetGrayWeightUniqueId(cookie, grayConfig.UniqueGrayTag))
	}

	// 删除Accept-Encoding，避免压缩， 如果是压缩的内容，后续插件就没法处理了
	_ = proxywasm.RemoveHttpRequestHeader("Accept-Encoding")
	_ = proxywasm.RemoveHttpRequestHeader("Content-Length")
	deployment := &config.Deployment{}

	globalConfig := grayConfig.Injection.GlobalConfig
	if globalConfig.Enabled {
		conditionRule := util.GetConditionRules(grayConfig.Rules, grayKeyValue, cookie)
		trimmedValue := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(globalConfig.Value), "{"), "}")
		ctx.SetContext(globalConfig.Key, fmt.Sprintf("<script>var %s = {\n%s:%s,\n %s \n}\n</script>", globalConfig.Key, globalConfig.FeatureKey, conditionRule, trimmedValue))
	}

	if isHtmlRequest {
		// index首页请求每次都会进度灰度规则判断
		deployment = util.FilterGrayRule(&grayConfig, grayKeyValue, cookie)
		log.Infof("route: %s, index html request: %v, backend: %v, xPreHigressVersion: %s", route, requestPath, deployment.BackendVersion, frontendVersion)
		ctx.SetContext(config.PreHigressVersion, deployment.Version)
		ctx.SetContext(grayConfig.BackendGrayTag, deployment.BackendVersion)
	} else {
		if util.IsSupportMultiVersion(grayConfig) {
			deployment = util.FilterMultiVersionGrayRule(&grayConfig, grayKeyValue, cookie, requestPath)
			log.Infof("route: %s, multi version %v", route, deployment)
		} else {
			grayDeployment := util.FilterGrayRule(&grayConfig, grayKeyValue, cookie)
			if isIndexRequest {
				deployment = grayDeployment
			} else {
				deployment = util.GetVersion(grayConfig, grayDeployment, frontendVersion)
			}
		}
	}
	proxywasm.AddHttpRequestHeader(config.XHigressTag, deployment.Version)

	rewrite := grayConfig.Rewrite
	if rewrite.Host != "" {
		err := proxywasm.ReplaceHttpRequestHeader(":authority", rewrite.Host)
		if err != nil {
			log.Errorf("route: %s, host rewrite failed: %v", route, err)
		}
	}

	if hasRewrite {
		rewritePath := requestPath
		if isHtmlRequest {
			rewritePath = util.IndexRewrite(requestPath, deployment.Version, grayConfig.Rewrite.Index)
		} else {
			rewritePath = util.PrefixFileRewrite(requestPath, deployment.Version, grayConfig.Rewrite.File)
		}
		if requestPath != rewritePath {
			log.Infof("route: %s, rewrite path:%s, rewritePath:%s, Version:%v", route, requestPath, rewritePath, deployment.Version)
			proxywasm.ReplaceHttpRequestHeader(":path", rewritePath)
		}
	}
	return types.ActionContinue
}

func onHttpResponseHeader(ctx wrapper.HttpContext, grayConfig config.GrayConfig) types.Action {
	enabledGray, _ := ctx.GetContext(config.EnabledGray).(bool)
	if !enabledGray {
		ctx.DontReadResponseBody()
		return types.ActionContinue
	}
	if !grayConfig.UseManifestAsEntry {
		isIndexRequest, indexOk := ctx.GetContext(config.IsIndexRequest).(bool)
		if indexOk && isIndexRequest {
			// 首页请求强制不缓存
			proxywasm.ReplaceHttpResponseHeader("cache-control", "no-cache, no-store, max-age=0, must-revalidate")
			ctx.DontReadResponseBody()
			return types.ActionContinue
		}

		isHtmlRequest, htmlOk := ctx.GetContext(config.IsHtmlRequest).(bool)
		// response 不处理非首页的请求
		if !htmlOk || !isHtmlRequest {
			ctx.DontReadResponseBody()
			return types.ActionContinue
		} else {
			// 不会进去Streaming 的Body处理
			ctx.BufferResponseBody()
		}
	}

	// 处理HTML的首页
	status, err := proxywasm.GetHttpResponseHeader(":status")
	if grayConfig.Rewrite != nil && grayConfig.Rewrite.Host != "" {
		// 删除Content-Disposition，避免自动下载文件
		proxywasm.RemoveHttpResponseHeader("Content-Disposition")
	}

	// 删除content-length，可能要修改Response返回值
	proxywasm.RemoveHttpResponseHeader("Content-Length")

	// 处理code为 200的情况
	if err != nil || status != "200" {
		// 如果找不到HTML，但配置了HTML页面
		if status == "404" && grayConfig.Html != "" {
			responseHeaders, _ := proxywasm.GetHttpResponseHeaders()
			headersMap := util.ConvertHeaders(responseHeaders)
			delete(headersMap, "content-length")
			headersMap[":status"] = []string{"200"}
			headersMap["content-type"] = []string{"text/html"}
			ctx.BufferResponseBody()
			proxywasm.ReplaceHttpResponseHeaders(util.ReconvertHeaders(headersMap))
		} else {
			route, _ := util.GetRouteName()
			log.Errorf("route: %s, request error code: %s, message: %v", route, status, err)
			ctx.DontReadResponseBody()
			return types.ActionContinue
		}
	}
	proxywasm.ReplaceHttpResponseHeader("cache-control", "no-cache, no-store, max-age=0, must-revalidate")

	// 前端版本
	frontendVersion, isFrontendVersionOk := ctx.GetContext(config.PreHigressVersion).(string)
	if isFrontendVersionOk {
		proxywasm.AddHttpResponseHeader("Set-Cookie", fmt.Sprintf("%s=%s; Max-Age=%d; Path=/; HttpOnly; Secure", config.XHigressTag, frontendVersion, grayConfig.StoreMaxAge))
	}
	// 设置GrayWeight 唯一值
	if grayConfig.UniqueGrayTagConfigured || grayConfig.GrayWeight > 0 {
		uniqueId, isUniqueIdOk := ctx.GetContext(grayConfig.UniqueGrayTag).(string)
		if isUniqueIdOk {
			proxywasm.AddHttpResponseHeader("Set-Cookie", fmt.Sprintf("%s=%s; Max-Age=%d; Path=/; HttpOnly; Secure", grayConfig.UniqueGrayTag, uniqueId, grayConfig.StoreMaxAge))
		}
	}
	// 设置后端的版本
	if util.IsBackendGrayEnabled(grayConfig) {
		backendVersion, isBackVersionOk := ctx.GetContext(grayConfig.BackendGrayTag).(string)
		if isBackVersionOk {
			if backendVersion == "" {
				// 删除后端灰度版本
				proxywasm.AddHttpResponseHeader("Set-Cookie", fmt.Sprintf("%s=%s; Expires=Thu, 01 Jan 1970 00:00:00 GMT; Path=/; HttpOnly; Secure", grayConfig.BackendGrayTag, backendVersion))
			} else {
				proxywasm.AddHttpResponseHeader("Set-Cookie", fmt.Sprintf("%s=%s; Max-Age=%d; Path=/; HttpOnly; Secure", grayConfig.BackendGrayTag, backendVersion, grayConfig.StoreMaxAge))
			}
		}
	}
	return types.ActionContinue
}

func onHttpResponseBody(ctx wrapper.HttpContext, grayConfig config.GrayConfig, body []byte) types.Action {
	enabledGray, _ := ctx.GetContext(config.EnabledGray).(bool)
	if !enabledGray {
		return types.ActionContinue
	}
	isHtmlRequest, isHtmlRequestOk := ctx.GetContext(config.IsHtmlRequest).(bool)
	frontendVersion, isFeVersionOk := ctx.GetContext(config.PreHigressVersion).(string)
	// 只处理首页相关请求
	if !isFeVersionOk || !isHtmlRequestOk || !isHtmlRequest {
		return types.ActionContinue
	}
	globalConfig := grayConfig.Injection.GlobalConfig
	globalConfigValue, isGobalConfigOk := ctx.GetContext(globalConfig.Key).(string)
	if !isGobalConfigOk {
		globalConfigValue = ""
	}

	newHtml := string(body)
	if grayConfig.Html != "" {
		newHtml = grayConfig.Html
	}
	newHtml = util.InjectContent(newHtml, grayConfig.Injection, globalConfigValue)
	// 替换当前html加载的动态文件版本
	newHtml = strings.ReplaceAll(newHtml, "{version}", frontendVersion)
	newHtml = util.FixLocalStorageKey(newHtml, grayConfig.LocalStorageGrayKey)

	// 最终替换响应体
	if err := proxywasm.ReplaceHttpResponseBody([]byte(newHtml)); err != nil {
		route, _ := util.GetRouteName()
		log.Errorf("route: %s, Failed to replace response body: %v", route, err)
		return types.ActionContinue
	}
	return types.ActionContinue
}
