package main

import (
	"errors"
	"net/http"
	"strings"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func main() {}

func init() {
	wrapper.SetCtx(
		"ai-transformer",
		wrapper.ParseConfigBy(parseConfig),
		wrapper.ProcessRequestHeadersBy(onHttpRequestHeaders),
		wrapper.ProcessRequestBodyBy(onHttpRequestBody),
		wrapper.ProcessResponseHeadersBy(onHttpResponseHeaders),
		wrapper.ProcessResponseBodyBy(onHttpResponseBody),
	)
}

type AITransformerConfig struct {
	client                  wrapper.HttpClient
	requestTransformEnable  bool
	requestTransformPrompt  string
	responseTransformEnable bool
	responseTransformPrompt string
	providerAPIKey          string
}

const llmRequestTemplate = `{
	"model": "qwen-max",
	"input":{
		"messages":[  
			{
				"role": "system",
				"content": "假设你是一个http 1.1协议专家，你的回答应该只包含http报文，除此之外不要有任何其他内容。"
			},
            {
                "role": "system",
                "content": ""
            },
			{
				"role": "user",
				"content": ""
			}
		]
	}
}`

func parseConfig(json gjson.Result, config *AITransformerConfig, log log.Log) error {
	config.requestTransformEnable = json.Get("request.enable").Bool()
	config.requestTransformPrompt = json.Get("request.prompt").String()
	config.responseTransformEnable = json.Get("response.enable").Bool()
	config.responseTransformPrompt = json.Get("response.prompt").String()
	config.providerAPIKey = json.Get("provider.apiKey").String()
	config.client = wrapper.NewClusterClient(wrapper.DnsCluster{
		ServiceName: json.Get("provider.serviceName").String(),
		Port:        443,
		Domain:      json.Get("provider.domain").String(),
	})
	return nil
}

func getSplitPos(header string) int {
	for i, ch := range header {
		if ch == ':' && i != 0 {
			return i
		}
	}
	return -1
}

func extraceHttpFrame(frame string) ([][2]string, []byte, error) {
	pos := strings.Index(frame, "\n\n")
	separatorLen := len("\n\n")
	if pos == -1 {
		pos = strings.Index(frame, "\r\n\r\n")
		separatorLen = len("\r\n\r\n")
	}
	if pos == -1 {
		return nil, nil, errors.New("missing http frame separator")
	}
	headers := [][2]string{}
	for _, header := range strings.Split(frame[:pos], "\n") {
		header = strings.TrimRight(header, "\r")
		splitPos := getSplitPos(header)
		if splitPos == -1 {
			return nil, nil, errors.New("invalid http frame.")
		}
		headers = append(headers, [2]string{header[:splitPos], header[splitPos+1:]})
	}
	body := []byte(frame[pos+separatorLen:])
	return headers, body, nil
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config AITransformerConfig, log log.Log) types.Action {
	ctx.DisableReroute()
	log.Info("onHttpRequestHeaders")
	if !config.requestTransformEnable || config.requestTransformPrompt == "" {
		ctx.DontReadRequestBody()
		return types.ActionContinue
	} else {
		return types.HeaderStopIteration
	}
}

func onHttpRequestBody(ctx wrapper.HttpContext, config AITransformerConfig, body []byte, log log.Log) types.Action {
	log.Info("onHttpRequestBody")
	headers, err := proxywasm.GetHttpRequestHeaders()
	if err != nil {
		log.Error("Failed to get http response headers.")
		return types.ActionContinue
	}
	headerStr := ""
	for _, hd := range headers {
		headerStr += hd[0] + ":" + hd[1] + "\n"
	}
	var llmRequestBody string
	llmRequestBody, _ = sjson.Set(llmRequestTemplate, "input.messages.1.content", config.requestTransformPrompt)
	llmRequestBody, _ = sjson.Set(llmRequestBody, "input.messages.2.content", headerStr+"\n"+string(body))
	hds := [][2]string{{"Authorization", "Bearer " + config.providerAPIKey}, {"Content-Type", "application/json"}}
	log.Info(headerStr + "\n" + string(body))
	config.client.Post(
		"/api/v1/services/aigc/text-generation/generation",
		hds,
		[]byte(llmRequestBody),
		func(statusCode int, responseHeaders http.Header, responseBody []byte) {
			newHeaders, newBody, err := extraceHttpFrame(gjson.GetBytes(responseBody, "output.text").String())
			if err == nil {
				proxywasm.ReplaceHttpRequestHeaders(newHeaders)
				proxywasm.ReplaceHttpRequestBody(newBody)
			}
			proxywasm.ResumeHttpRequest()
		},
		50000,
	)

	return types.ActionPause
}

func onHttpResponseHeaders(ctx wrapper.HttpContext, config AITransformerConfig, log log.Log) types.Action {
	if !config.responseTransformEnable || config.responseTransformPrompt == "" {
		ctx.DontReadResponseBody()
		return types.ActionContinue
	} else {
		return types.HeaderStopIteration
	}
}

func onHttpResponseBody(ctx wrapper.HttpContext, config AITransformerConfig, body []byte, log log.Log) types.Action {
	headers, err := proxywasm.GetHttpResponseHeaders()
	if err != nil {
		log.Error("Failed to get http response headers.")
		return types.ActionContinue
	}
	headerStr := ""
	for _, hd := range headers {
		headerStr += hd[0] + ":" + hd[1] + "\n"
	}
	var llmRequestBody string
	llmRequestBody, _ = sjson.Set(llmRequestTemplate, "input.messages.1.content", config.responseTransformPrompt)
	llmRequestBody, _ = sjson.Set(llmRequestBody, "input.messages.2.content", headerStr+"\n"+string(body))
	hds := [][2]string{{"Authorization", "Bearer " + config.providerAPIKey}, {"Content-Type", "application/json"}}
	log.Info(headerStr + "\n" + string(body))
	config.client.Post(
		"/api/v1/services/aigc/text-generation/generation",
		hds,
		[]byte(llmRequestBody),
		func(statusCode int, responseHeaders http.Header, responseBody []byte) {
			newHeaders, newBody, err := extraceHttpFrame(gjson.GetBytes(responseBody, "output.text").String())
			if err == nil {
				proxywasm.ReplaceHttpResponseHeaders(newHeaders)
				proxywasm.ReplaceHttpResponseBody(newBody)
			}
			proxywasm.ResumeHttpResponse()
		},
		50000,
	)

	return types.ActionPause
}
