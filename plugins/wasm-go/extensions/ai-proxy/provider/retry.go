package provider

import (
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"strings"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-proxy/util"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

const (
	ctxRetryCount       = "retryCount"
	ctxRetryIsStreaming = "retryIsStreaming"
)

type retryOnFailure struct {
	// @Title zh-CN 是否启用请求重试
	enabled bool `required:"false" yaml:"enabled" json:"enabled"`
	// @Title zh-CN 重试次数
	maxRetries int64 `required:"false" yaml:"maxRetries" json:"maxRetries"`
	// @Title zh-CN 重试超时时间
	retryTimeout int64 `required:"false" yaml:"retryTimeout" json:"retryTimeout"`
	// @Title zh-CN 需要进行重试的原始请求的状态码，支持正则表达式匹配
	retryOnStatus []string `required:"false" yaml:"retryOnStatus" json:"retryOnStatus"`
}

func (r *retryOnFailure) FromJson(json gjson.Result) {
	r.enabled = json.Get("enabled").Bool()
	r.maxRetries = json.Get("maxRetries").Int()
	if r.maxRetries == 0 {
		r.maxRetries = 1
	}
	r.retryTimeout = json.Get("retryTimeout").Int()
	if r.retryTimeout == 0 {
		r.retryTimeout = 60 * 1000
	}
	for _, status := range json.Get("retryOnStatus").Array() {
		r.retryOnStatus = append(r.retryOnStatus, status.String())
	}
	// If retryOnStatus is empty, default to retry on 4xx and 5xx
	if len(r.retryOnStatus) == 0 {
		r.retryOnStatus = []string{"4.*", "5.*"}
	}
}

func (c *ProviderConfig) IsRetryOnFailureEnabled() bool {
	return c.retryOnFailure.enabled
}

func (c *ProviderConfig) retryFailedRequest(activeProvider Provider, ctx wrapper.HttpContext, apiTokenInUse string, apiTokens []string) error {
	log.Infof("Retry failed request: provider=%s", activeProvider.GetProviderType())
	retryClient := createRetryClient()
	apiName, _ := ctx.GetContext(CtxKeyApiName).(ApiName)
	ctx.SetContext(ctxRetryCount, 1)
	return c.sendRetryRequest(ctx, apiName, activeProvider, retryClient, apiTokenInUse, apiTokens)
}

func (c *ProviderConfig) transformResponseHeadersAndBody(ctx wrapper.HttpContext, activeProvider Provider, apiName ApiName, headers http.Header, body []byte) ([][2]string, []byte) {
	if handler, ok := activeProvider.(TransformResponseHeadersHandler); ok {
		handler.TransformResponseHeaders(ctx, apiName, headers)
	} else {
		c.DefaultTransformResponseHeaders(ctx, headers)
	}

	// When protocol is "original", pass through the body without transformation
	if !c.IsOriginal() {
		if handler, ok := activeProvider.(TransformResponseBodyHandler); ok {
			var err error
			body, err = handler.TransformResponseBody(ctx, apiName, body)
			if err != nil {
				log.Errorf("Failed to transform response body: %v", err)
			}
		}
	}

	return util.HeaderToSlice(headers), body
}

func (c *ProviderConfig) transformStreamingRetryResponse(
	ctx wrapper.HttpContext, activeProvider Provider,
	apiName ApiName, headers http.Header, body []byte) ([][2]string, []byte) {

	if handler, ok := activeProvider.(TransformResponseHeadersHandler); ok {
		handler.TransformResponseHeaders(ctx, apiName, headers)
	} else {
		c.DefaultTransformResponseHeaders(ctx, headers)
	}
	headers.Set("Content-Type", "text/event-stream")

	var resultBuilder strings.Builder
	// When protocol is "original", pass through the body without any transformation,
	// mirroring the normal streaming path that also skips body processing.
	if c.IsOriginal() {
		resultBuilder.Write(body)
	} else if handler, ok := activeProvider.(StreamingResponseBodyHandler); ok {
		events := ExtractStreamingEvents(ctx, body)
		for i, event := range events {
			isLast := i == len(events)-1
			chunk, err := handler.OnStreamingResponseBody(ctx, apiName, []byte(event.RawEvent), isLast)
			if err == nil && chunk != nil {
				resultBuilder.Write(chunk)
			} else {
				resultBuilder.WriteString(event.RawEvent)
			}
		}
	} else if handler, ok := activeProvider.(StreamingEventHandler); ok {
		events := ExtractStreamingEvents(ctx, body)
		for _, event := range events {
			outputEvents, err := handler.OnStreamingEvent(ctx, apiName, event)
			if err != nil || len(outputEvents) == 0 {
				resultBuilder.WriteString(event.RawEvent)
			} else {
				for _, out := range outputEvents {
					resultBuilder.WriteString(out.ToHttpString())
				}
			}
		}
	} else {
		resultBuilder.Write(body)
	}

	result := []byte(resultBuilder.String())

	// Apply Claude response conversion if needed (same as normal streaming path)
	needClaudeConversion, _ := ctx.GetContext("needClaudeResponseConversion").(bool)
	if needClaudeConversion {
		const claudeConverterKey = "claudeConverter"
		var converter *ClaudeToOpenAIConverter
		if converterData := ctx.GetContext(claudeConverterKey); converterData != nil {
			if c, ok := converterData.(*ClaudeToOpenAIConverter); ok {
				converter = c
			}
		}
		if converter == nil {
			converter = &ClaudeToOpenAIConverter{}
			ctx.SetContext(claudeConverterKey, converter)
		}
		claudeResult, err := converter.ConvertOpenAIStreamResponseToClaude(ctx, result)
		if err != nil {
			log.Errorf("Failed to convert streaming retry response to Claude format: %v", err)
		} else {
			result = claudeResult
		}
	}

	return util.HeaderToSlice(headers), result
}

func (c *ProviderConfig) retryCall(
	ctx wrapper.HttpContext, activeProvider Provider,
	apiName ApiName, statusCode int, responseHeaders http.Header, responseBody []byte,
	retryClient *wrapper.ClusterClient[wrapper.RouteCluster],
	apiTokenInUse string, apiTokens []string) {

	retryCount := ctx.GetContext(ctxRetryCount).(int)
	log.Infof("Sent retry request: %d/%d", retryCount, c.retryOnFailure.maxRetries)

	if statusCode == 200 {
		log.Infof("Retry request succeeded")
		isStreaming, _ := ctx.GetContext(ctxRetryIsStreaming).(bool)
		if isStreaming {
			headers, body := c.transformStreamingRetryResponse(ctx, activeProvider, apiName, responseHeaders, responseBody)
			proxywasm.SendHttpResponse(200, headers, body, -1)
		} else {
			headers, body := c.transformResponseHeadersAndBody(ctx, activeProvider, apiName, responseHeaders, responseBody)
			proxywasm.SendHttpResponse(200, headers, body, -1)
		}
		return
	} else {
		log.Infof("The retry request still failed, status: %d, responseHeaders: %v, responseBody: %s", statusCode, responseHeaders, string(responseBody))
	}

	retryCount++
	if retryCount <= int(c.retryOnFailure.maxRetries) {
		ctx.SetContext(ctxRetryCount, retryCount)
		err := c.sendRetryRequest(ctx, apiName, activeProvider, retryClient, apiTokenInUse, apiTokens)
		if err != nil {
			log.Errorf("sendRetryRequest failed, err:%v", err)
			proxywasm.ResumeHttpResponse()
			return
		}
	} else {
		log.Infof("Reached the maximum retry count: %d", c.retryOnFailure.maxRetries)
		proxywasm.ResumeHttpResponse()
		return
	}
}

func (c *ProviderConfig) sendRetryRequest(
	ctx wrapper.HttpContext, apiName ApiName, activeProvider Provider,
	retryClient *wrapper.ClusterClient[wrapper.RouteCluster],
	apiTokenInUse string, apiTokens []string) error {

	// Remove last failed token from retry apiTokens list
	apiTokens = removeApiTokenFromRetryList(apiTokens, apiTokenInUse)
	if len(apiTokens) == 0 {
		return errors.New("No more apiTokens to retry")
	}
	// Set apiTokenInUse for the retry request
	apiTokenInUse = GetRandomToken(apiTokens)
	log.Debugf("Retry request with apiToken: %s", apiTokenInUse)
	ctx.SetContext(c.failover.ctxApiTokenInUse, apiTokenInUse)
	requestBody := ctx.GetByteSliceContext(CtxRequestBody, []byte(""))
	log.Debugf("get original requestBody:%s", requestBody)
	modifiedHeaders, modifiedBody, err := c.transformRequestHeadersAndBody(ctx, activeProvider, [][2]string{
		{"content-type", "application/json"},
		{":authority", ctx.GetStringContext(CtxRequestHost, "")},
		{":path", ctx.GetStringContext(CtxRequestPath, "")},
	}, requestBody)
	if err != nil {
		return fmt.Errorf("sendRetryRequest failed to transform request headers and body: %v", err)
	}

	isStreaming, _ := ctx.GetContext(ctxKeyIsStreaming).(bool)
	ctx.SetContext(ctxRetryIsStreaming, isStreaming)

	err = retryClient.Post(generateUrl(modifiedHeaders), util.HeaderToSlice(modifiedHeaders), modifiedBody,
		func(statusCode int, responseHeaders http.Header, responseBody []byte) {
			c.retryCall(ctx, activeProvider, apiName, statusCode, responseHeaders, responseBody, retryClient, apiTokenInUse, apiTokens)
		}, uint32(c.retryOnFailure.retryTimeout))
	if err != nil {
		return fmt.Errorf("Failed to send retry request: %v", err)
	}
	return nil
}

func createRetryClient() *wrapper.ClusterClient[wrapper.RouteCluster] {
	retryClient := wrapper.NewClusterClient(wrapper.RouteCluster{})
	return retryClient
}

func removeApiTokenFromRetryList(apiTokens []string, removedApiToken string) []string {
	var availableApiTokens []string
	for _, s := range apiTokens {
		if s != removedApiToken {
			availableApiTokens = append(availableApiTokens, s)
		}
	}
	log.Debugf("Remove apiToken %s from retry apiTokens list", removedApiToken)
	log.Debugf("Available retry apiTokens: %v", availableApiTokens)
	return availableApiTokens
}

func GetRandomToken(apiTokens []string) string {
	count := len(apiTokens)
	switch count {
	case 0:
		return ""
	case 1:
		return apiTokens[0]
	default:
		return apiTokens[rand.Intn(count)]
	}
}
