package main

import (
	"strconv"
	"strings"
	"time"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-endpoint-picker/scheduling"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

const feedbackContextKey = "ai_endpoint_picker_feedback"

type requestFeedback struct {
	lease     *scheduling.FeedbackLease
	status    int
	firstByte time.Time
}

func main() {}

func init() {
	wrapper.SetCtx(
		"ai-endpoint-picker",
		wrapper.ParseConfig(parseConfig),
		wrapper.ProcessRequestHeaders(onHttpRequestHeaders),
		wrapper.ProcessRequestBody(onHttpRequestBody),
		wrapper.ProcessResponseHeaders(onHttpResponseHeaders),
		wrapper.ProcessStreamingResponseBody(onHttpStreamingResponseBody),
		wrapper.ProcessResponseBody(onHttpResponseBody),
		wrapper.ProcessStreamDone(onHttpStreamDone),
	)
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config Config) types.Action {
	method := strings.ToUpper(ctx.Method())
	contentLength, contentLengthErr := proxywasm.GetHttpRequestHeader("content-length")
	if method == "GET" || method == "HEAD" || (contentLengthErr == nil && strings.TrimSpace(contentLength) == "0") {
		config.metrics.fallback()
		ctx.DontReadRequestBody()
		return types.HeaderContinue
	}
	return types.HeaderStopIteration
}

func onHttpRequestBody(ctx wrapper.HttpContext, config Config, body []byte) types.Action {
	model := gjson.GetBytes(body, "model")
	if !gjson.ValidBytes(body) || !model.Exists() || model.String() == "" {
		config.metrics.fallback()
		return types.ActionContinue
	}
	hosts, err := proxywasm.GetUpstreamHosts()
	if err != nil || len(hosts) == 0 {
		config.metrics.fallback()
		return types.ActionContinue
	}

	active := make(map[string]struct{}, len(hosts))
	endpoints := make([]scheduling.EndpointSnapshot, 0, len(hosts))
	missingSignals := uint64(0)
	for _, host := range hosts {
		address, metadata := host[0], host[1]
		if address == "" || !gjson.Valid(metadata) {
			config.metrics.fallback()
			return types.ActionContinue
		}
		active[address] = struct{}{}
		health := gjson.Get(metadata, "health_status")
		if !health.Exists() {
			config.metrics.fallback()
			return types.ActionContinue
		}
		endpoint := scheduling.EndpointSnapshot{
			Address: address,
			Healthy: health.String() == "Healthy",
			Signals: map[scheduling.SignalName]scheduling.SignalValue{},
		}
		if endpoint.Healthy {
			signals, parseErr := scheduling.ParseVLLMSignals(gjson.Get(metadata, "metrics").String(), model.String())
			if parseErr != nil {
				config.metrics.fallback()
				return types.ActionContinue
			}
			for name, value := range signals {
				endpoint.Signals[name] = value
			}
			for name, value := range config.store.Signals(address) {
				endpoint.Signals[name] = value
			}
			for _, name := range scheduling.SignalNames {
				if value, ok := endpoint.Signals[name]; !ok || !value.Available {
					missingSignals++
				}
			}
		}
		endpoints = append(endpoints, endpoint)
	}
	config.store.Cleanup(active)
	decision := config.pipeline.Schedule(endpoints)
	if decision.FallbackReason != "" || decision.Address == "" {
		config.metrics.fallback()
		return types.ActionContinue
	}
	if err := proxywasm.SetUpstreamOverrideHost([]byte(decision.Address)); err != nil {
		config.metrics.fallback()
		return types.ActionContinue
	}

	now := time.Now()
	feedback := &requestFeedback{lease: config.store.Begin(decision.Address, now)}
	ctx.SetContext(feedbackContextKey, feedback)
	config.metrics.missing(missingSignals)
	config.metrics.decision()
	config.metrics.beginFeedback()
	if config.sampleRate > 0 && config.random.Float64() < config.sampleRate {
		log.Debugf("ai-endpoint-picker candidates=%d score=%.4f missing_signals=%d", decision.CandidateCount, decision.Score, missingSignals)
	}
	return types.ActionContinue
}

func onHttpResponseHeaders(ctx wrapper.HttpContext, _ Config) types.Action {
	feedback := getRequestFeedback(ctx)
	if feedback == nil {
		return types.ActionContinue
	}
	status, err := proxywasm.GetHttpResponseHeader(":status")
	if err == nil {
		feedback.status, _ = strconv.Atoi(status)
	}
	return types.ActionContinue
}

func onHttpStreamingResponseBody(ctx wrapper.HttpContext, _ Config, data []byte, _ bool) []byte {
	recordFirstByte(ctx, data)
	return data
}

func onHttpResponseBody(ctx wrapper.HttpContext, _ Config, body []byte) types.Action {
	recordFirstByte(ctx, body)
	return types.ActionContinue
}

func onHttpStreamDone(ctx wrapper.HttpContext, config Config) {
	feedback := getRequestFeedback(ctx)
	if feedback == nil {
		return
	}
	failed := feedback.status == 0 || feedback.status >= 500
	if feedback.lease.Complete(time.Now(), feedback.firstByte, failed) {
		config.metrics.completeFeedback()
	}
}

func getRequestFeedback(ctx wrapper.HttpContext) *requestFeedback {
	feedback, _ := ctx.GetContext(feedbackContextKey).(*requestFeedback)
	return feedback
}

func recordFirstByte(ctx wrapper.HttpContext, data []byte) {
	if len(data) == 0 {
		return
	}
	feedback := getRequestFeedback(ctx)
	if feedback != nil && feedback.firstByte.IsZero() {
		feedback.firstByte = time.Now()
	}
}
