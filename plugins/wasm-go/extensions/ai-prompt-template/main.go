package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

func main() {}

func init() {
	wrapper.SetCtx(
		"ai-prompt-template",
		wrapper.ParseConfigBy(parseConfig),
		wrapper.ProcessRequestHeadersBy(onHttpRequestHeaders),
		wrapper.ProcessRequestBodyBy(onHttpRequestBody),
	)
}

type AIPromptTemplateConfig struct {
	templates map[string]string
}

func parseConfig(json gjson.Result, config *AIPromptTemplateConfig, log log.Log) error {
	config.templates = make(map[string]string)
	for _, v := range json.Get("templates").Array() {
		config.templates[v.Get("name").String()] = v.Get("template").Raw
		log.Info(v.Get("template").Raw)
	}
	return nil
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config AIPromptTemplateConfig, log log.Log) types.Action {
	ctx.DisableReroute()
	templateEnable, _ := proxywasm.GetHttpRequestHeader("template-enable")
	if templateEnable == "false" {
		ctx.DontReadRequestBody()
		return types.ActionContinue
	}
	proxywasm.RemoveHttpRequestHeader("content-length")
	return types.ActionContinue
}

func onHttpRequestBody(ctx wrapper.HttpContext, config AIPromptTemplateConfig, body []byte, log log.Log) types.Action {
	if gjson.GetBytes(body, "template").Exists() && gjson.GetBytes(body, "properties").Exists() {
		name := gjson.GetBytes(body, "template").String()
		template := config.templates[name]
		for key, value := range gjson.GetBytes(body, "properties").Map() {
			// The template is serialized JSON, so replacement text must retain JSON escaping.
			escaped, _ := json.Marshal(value.String())
			template = strings.ReplaceAll(template, fmt.Sprintf("{{%s}}", key), string(escaped[1:len(escaped)-1]))
		}
		proxywasm.ReplaceHttpRequestBody([]byte(template))
	}
	return types.ActionContinue
}
