package main

import (
	"net/http"
	"strings"

	"github.com/alibaba/higress/plugins/wasm-go/pkg/wrapper"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/tidwall/gjson"
)

// 默认超时时间为1s
var defaultTimeout uint32 = 1000

const VariableReplacedStr = "$uri"

func main() {
	wrapper.SetCtx(
		"static-files",
		wrapper.ParseConfigBy(parseConfig),
		wrapper.ProcessRequestHeadersBy(onHttpRequestHeaders),
	)
}

type StaticFilesConfig struct {
	root      string
	alias     string
	aliasPath string
	index     []string
	tryPaths  []string
	timeout   uint32
	tryCodes  []int
	client    wrapper.HttpClient
}

func parseConfig(json gjson.Result, config *StaticFilesConfig, log wrapper.Log) error {
	config.root = json.Get("root").String()
	config.alias = json.Get("alias").String()
	config.aliasPath = json.Get("alias_path").String()
	for _, result := range json.Get("index").Array() {
		config.index = append(config.index, result.String())
	}

	for _, result := range json.Get("try_paths").Array() {
		config.tryPaths = append(config.tryPaths, result.String())
	}

	if json.Get("try_codes").String() == "" {
		// tryCodes默认值为["404", "403"]
		config.tryCodes = []int{http.StatusNotFound, http.StatusForbidden}
	} else {
		for _, result := range json.Get("try_codes").Array() {
			config.tryCodes = append(config.tryCodes, int(result.Int()))
		}
	}

	timeout := json.Get("timeout").Int()
	if timeout == 0 {
		// tryPaths的timeout默认值为1s
		config.timeout = defaultTimeout
	} else {
		config.timeout = uint32(timeout)
	}

	config.client = wrapper.NewClusterClient(wrapper.RouteCluster{
		Host: json.Get("host").String(),
	})
	return nil
}

func tryHttpCall(tryPaths []string, config StaticFilesConfig, index int, path string, log wrapper.Log) {
	if index >= len(tryPaths) {
		proxywasm.ResumeHttpRequest()
		return
	}

	requestPath := strings.Replace(tryPaths[index], VariableReplacedStr, path, -1)
	log.Debugf("try path request, path: %s", requestPath)
	err := config.client.Get(requestPath, nil,
		func(statusCode int, responseHeaders http.Header, responseBody []byte) {
			if !contains(config.tryCodes, statusCode) {
				proxywasm.SendHttpResponse(uint32(statusCode), convertHttpHeadersToStruct(responseHeaders), responseBody, -1)
				return
			}
			tryHttpCall(tryPaths, config, index+1, path, log)
		}, config.timeout)

	if err != nil {
		log.Errorf("try path request failed, path %s, error: %s", requestPath, err.Error())
		tryHttpCall(tryPaths, config, index+1, path, log)
	}
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config StaticFilesConfig, log wrapper.Log) types.Action {
	log.Debugf("static files request config: %+v", config)
	path := ctx.Path()
	tryPaths := make([]string, 0)
	requestPath := path
	if config.root != "" {
		requestPath = getRootRequestPath(config.root, path)
		tryPaths = append(tryPaths, requestPath)
	} else if config.aliasPath != "" {
		requestPath = getAliasRequestPath(config.alias, config.aliasPath, path)
		tryPaths = append(tryPaths, requestPath)
	}

	if len(config.index) != 0 {
		tryPaths = append(tryPaths, *getIndexRequestPath(config.index, requestPath)...)
	}

	if len(config.tryPaths) != 0 {
		tryPaths = append(tryPaths, config.tryPaths...)
	}

	if len(tryPaths) == 0 {
		return types.ActionContinue
	}

	tryHttpCall(tryPaths, config, 0, path, log)
	return types.ActionPause
}
