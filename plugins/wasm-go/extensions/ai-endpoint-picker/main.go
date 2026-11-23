package main

import (
	"encoding/binary"
	"strconv"
	"strings"
	"time"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-endpoint-picker/prefixcache"
	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-endpoint-picker/scheduling"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

const feedbackContextKey = "ai_endpoint_picker_feedback"

type candidateSkipReason uint8

const (
	candidateSkipAddress candidateSkipReason = 1 << iota
	candidateSkipMetadata
	candidateSkipHealth
	candidateSkipMetrics
)

type parsedHostCandidate struct {
	endpoint    scheduling.EndpointSnapshot
	cacheConfig scheduling.CacheConfig
}

type requestFeedback struct {
	lease     *scheduling.FeedbackLease
	status    int
	firstByte time.Time
}

type requestBodyFraming struct {
	contentLength    string
	transferEncoding string
	contentType      string
	contentEncoding  string
	connection       string
	upgrade          string
}

type requestBodyControl interface {
	DontReadRequestBody()
	SetRequestBodyBufferLimit(uint32)
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
	maybeRequestVMRebuild(config.vmRebuildThresholdBytes)
	framing := requestBodyFraming{}
	framing.contentLength, _ = proxywasm.GetHttpRequestHeader("content-length")
	framing.transferEncoding, _ = proxywasm.GetHttpRequestHeader("transfer-encoding")
	framing.contentType, _ = proxywasm.GetHttpRequestHeader("content-type")
	framing.contentEncoding, _ = proxywasm.GetHttpRequestHeader("content-encoding")
	framing.connection, _ = proxywasm.GetHttpRequestHeader("connection")
	framing.upgrade, _ = proxywasm.GetHttpRequestHeader("upgrade")
	action := requestHeadersAction(ctx, framing, config.maxRequestBodyBytes)
	if action == types.HeaderContinue {
		config.metrics.fallback()
	}
	return action
}

func requestHeadersAction(ctx requestBodyControl, framing requestBodyFraming, maxRequestBodyBytes uint32) types.Action {
	if !hasReadableRequestBody(framing) {
		ctx.DontReadRequestBody()
		return types.HeaderContinue
	}
	contentLength, err := strconv.ParseUint(strings.TrimSpace(framing.contentLength), 10, 64)
	if err != nil || contentLength == 0 || contentLength > uint64(maxRequestBodyBytes) || hasHeaderToken(framing.transferEncoding, "chunked") {
		ctx.DontReadRequestBody()
		return types.HeaderContinue
	}
	ctx.SetRequestBodyBufferLimit(maxRequestBodyBytes)
	return types.HeaderStopIteration
}

func hasReadableRequestBody(framing requestBodyFraming) bool {
	contentType := strings.ToLower(framing.contentType)
	if strings.Contains(contentType, "octet-stream") || strings.Contains(contentType, "grpc") {
		return false
	}
	if strings.TrimSpace(framing.contentEncoding) != "" {
		return false
	}
	if hasHeaderToken(framing.connection, "upgrade") && strings.EqualFold(strings.TrimSpace(framing.upgrade), "websocket") {
		return false
	}
	if contentLength, err := strconv.ParseInt(strings.TrimSpace(framing.contentLength), 10, 64); err == nil && contentLength > 0 {
		return true
	}
	return hasHeaderToken(framing.transferEncoding, "chunked")
}

func hasHeaderToken(value, token string) bool {
	for _, part := range strings.Split(value, ",") {
		if strings.EqualFold(strings.TrimSpace(part), token) {
			return true
		}
	}
	return false
}

func onHttpRequestBody(ctx wrapper.HttpContext, config Config, body []byte) types.Action {
	if uint64(len(body)) > uint64(config.maxRequestBodyBytes) {
		log.Debugf("ai-endpoint-picker fail-open: reason=request_body_over_limit")
		config.metrics.fallback()
		return types.ActionContinue
	}
	model, locality, prefixAvailable, err := inspectRequestBody(body, config.toolMode, config.maxBlocks, config.blockSizeTokens)
	if err != nil {
		log.Debugf("ai-endpoint-picker fail-open: reason=invalid_request")
		config.metrics.fallback()
		return types.ActionContinue
	}
	snapshots, err := config.hostCache.get()
	if err != nil {
		log.Debugf("ai-endpoint-picker fail-open: reason=upstream_hosts_unavailable")
		config.metrics.fallback()
		return types.ActionContinue
	}

	active := make(map[string]struct{}, len(snapshots.hosts))
	endpoints := make([]scheduling.EndpointSnapshot, 0, len(snapshots.hosts))
	actualBlockSizes := make(map[string]int, len(snapshots.hosts))
	for _, snapshot := range snapshots.hosts {
		candidate := snapshot.candidate(model)
		address := candidate.endpoint.Address
		active[address] = struct{}{}
		syncPrefixCapacity(config.prefix, address, candidate.endpoint.Healthy, candidate.cacheConfig.NumGPUBlocks, config.maxCacheBlocksPerEndpoint)
		if candidate.endpoint.Healthy {
			actualBlockSizes[address] = candidate.cacheConfig.BlockSize
			for name, value := range config.store.Signals(address) {
				candidate.endpoint.Signals[name] = value
			}
		}
		endpoints = append(endpoints, candidate.endpoint)
	}
	config.store.Cleanup(active)
	config.prefix.Cleanup(active)
	var prefixChains [][]prefixcache.Block
	if prefixAvailable {
		prefixChains = locality.Chains
		if blockCount(prefixChains) > 0 {
			for index := range endpoints {
				if !endpoints[index].Healthy {
					continue
				}
				endpoints[index].Signals[scheduling.SignalPrefixCache] = scheduling.SignalValue{
					Value:      config.prefix.Score(endpoints[index].Address, prefixChains),
					Available:  true,
					Confidence: 1,
					Source:     "gateway:approx_prefix_cache",
				}
			}
		}
	}
	missingSignals := uint64(0)
	for _, endpoint := range endpoints {
		if !endpoint.Healthy {
			continue
		}
		for _, name := range scheduling.SignalNames {
			if value, ok := endpoint.Signals[name]; !ok || !value.Available {
				missingSignals++
			}
		}
	}
	decision := config.pipeline.Schedule(endpoints)
	if decision.FallbackReason != "" || decision.Address == "" {
		log.Debugf("ai-endpoint-picker fail-open: reason=%s candidates=%d signal_mask=0x%x skip_mask=0x%x skipped=%d", decision.FallbackReason, decision.CandidateCount, decision.SignalAvailability, snapshots.skipMask, snapshots.skipped)
		config.metrics.fallback()
		return types.ActionContinue
	}
	if err := overrideAndRecord(proxywasm.SetUpstreamOverrideHost, config.prefix, decision.Address, prefixChains, actualBlockSizes[decision.Address]); err != nil {
		log.Debugf("ai-endpoint-picker fail-open: reason=override_failed")
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
		log.Debugf("ai-endpoint-picker reason=%s candidates=%d score=%.4f signal_mask=0x%x missing_signals=%d skip_mask=0x%x skipped=%d", decision.Reason, decision.CandidateCount, decision.Score, decision.SignalAvailability, missingSignals, snapshots.skipMask, snapshots.skipped)
	}
	return types.ActionContinue
}

func inspectRequestBody(body []byte, toolMode prefixcache.ToolMode, maxBlocks, blockSizeTokens int) (string, *prefixcache.Locality, bool, error) {
	return prefixcache.InspectRequestWithOptions(body, prefixcache.Options{
		ToolMode: toolMode, MaxBlocks: maxBlocks, BlockSizeTokens: blockSizeTokens,
	})
}

func syncPrefixCapacity(index *prefixcache.Index, address string, healthy bool, capacity, maxCapacity int) {
	if !healthy {
		index.Delete(address)
		return
	}
	if capacity <= 0 || capacity > maxCapacity {
		capacity = maxCapacity
	}
	index.SetCapacity(address, capacity)
}

func maybeRequestVMRebuild(threshold uint64) {
	if threshold == 0 {
		return
	}
	memory, err := proxywasm.GetProperty([]string{"plugin_vm_memory"})
	if err != nil || !vmMemoryAtOrAboveThreshold(memory, threshold) {
		return
	}
	_ = proxywasm.SetProperty([]string{"wasm_need_rebuild"}, []byte("true"))
}

func vmMemoryAtOrAboveThreshold(memory []byte, threshold uint64) bool {
	return threshold > 0 && len(memory) == 8 && binary.LittleEndian.Uint64(memory) >= threshold
}

func blockCount(chains [][]prefixcache.Block) int {
	total := 0
	for _, chain := range chains {
		total += len(chain)
	}
	return total
}

func overrideAndRecord(override func([]byte) error, index *prefixcache.Index, endpoint string, chains [][]prefixcache.Block, actualBlockSize int) error {
	if err := override([]byte(endpoint)); err != nil {
		return err
	}
	if blockCount(chains) > 0 {
		index.Record(endpoint, chains, actualBlockSize)
	}
	return nil
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
