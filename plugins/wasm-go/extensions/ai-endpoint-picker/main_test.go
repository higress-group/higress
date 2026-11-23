package main

import (
	"encoding/binary"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-endpoint-picker/prefixcache"
	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-endpoint-picker/scheduling"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
)

func TestVMMemoryThreshold(t *testing.T) {
	memory := make([]byte, 8)
	binary.LittleEndian.PutUint64(memory, 200)
	if !vmMemoryAtOrAboveThreshold(memory, 200) || !vmMemoryAtOrAboveThreshold(memory, 199) {
		t.Fatal("memory at or above threshold was not detected")
	}
	if vmMemoryAtOrAboveThreshold(memory, 201) || vmMemoryAtOrAboveThreshold(memory[:7], 1) || vmMemoryAtOrAboveThreshold(memory, 0) {
		t.Fatal("memory below threshold or malformed memory was accepted")
	}
}

type requestBodyControlStub struct {
	dontRead    bool
	bufferLimit uint32
}

func TestOverrideAndRecordOnlyLearnsAfterSuccess(t *testing.T) {
	chains := [][]prefixcache.Block{{{Hash: 1, EstimatedTokens: 32}}}
	index := prefixcache.NewIndex(10)
	failure := errors.New("override failed")
	if err := overrideAndRecord(func([]byte) error { return failure }, index, "a", chains, 16); !errors.Is(err, failure) {
		t.Fatalf("override error=%v want %v", err, failure)
	}
	if index.Len("a") != 0 {
		t.Fatal("failed override recorded prefix")
	}
	var overridden string
	if err := overrideAndRecord(func(address []byte) error {
		overridden = string(address)
		return nil
	}, index, "a", chains, 16); err != nil {
		t.Fatal(err)
	}
	if overridden != "a" || index.Len("a") != 1 || index.UsedCost("a") != 2 {
		t.Fatalf("success boundary endpoint=%q len=%d cost=%d", overridden, index.Len("a"), index.UsedCost("a"))
	}
}

func TestParseHostCandidateIsolatesMalformedHosts(t *testing.T) {
	validMetrics := "# TYPE vllm:num_requests_waiting gauge\nvllm:num_requests_waiting 1\n"
	validMetadata := `{"health_status":"Healthy","metrics":` + strconv.Quote(validMetrics) + `}`
	tests := []struct {
		name       string
		host       [2]string
		wantReason candidateSkipReason
	}{
		{name: "empty address", host: [2]string{"", validMetadata}, wantReason: candidateSkipAddress},
		{name: "invalid metadata", host: [2]string{"bad", `{"health_status":`}, wantReason: candidateSkipMetadata},
		{name: "non-object metadata", host: [2]string{"bad", `[]`}, wantReason: candidateSkipMetadata},
		{name: "missing health", host: [2]string{"bad", `{}`}, wantReason: candidateSkipHealth},
		{name: "invalid metrics type", host: [2]string{"bad", `{"health_status":"Healthy","metrics":3}`}, wantReason: candidateSkipMetrics},
		{name: "malformed prometheus", host: [2]string{"bad", `{"health_status":"Healthy","metrics":"vllm:num_requests_waiting nope\\n"}`}, wantReason: candidateSkipMetrics},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cache := newEndpointSnapshotCache(nil, scheduling.ParseCompactVLLMMetrics, nil)
			if _, got := cache.parseHost(test.host, compactHostSnapshot{}); got != test.wantReason {
				t.Fatalf("skip reason=%d want %d", got, test.wantReason)
			}
		})
	}

	cache := newEndpointSnapshotCache(nil, scheduling.ParseCompactVLLMMetrics, nil)
	goodSnapshot, reason := cache.parseHost([2]string{"good", validMetadata}, compactHostSnapshot{})
	good := goodSnapshot.candidate("m")
	if reason != 0 || good.endpoint.Address != "good" || !good.endpoint.Healthy || !good.endpoint.Signals[scheduling.SignalQueue].Available {
		t.Fatalf("valid candidate=%+v reason=%d", good, reason)
	}
	missingCapacitySnapshot, reason := cache.parseHost([2]string{"default-capacity", `{"health_status":"Healthy","metrics":""}`}, compactHostSnapshot{})
	missingCapacity := missingCapacitySnapshot.candidate("m")
	if reason != 0 || missingCapacity.cacheConfig.NumGPUBlocks != 0 {
		t.Fatalf("missing capacity candidate=%+v reason=%d", missingCapacity, reason)
	}
	unhealthySnapshot, reason := cache.parseHost([2]string{"unhealthy", `{"health_status":"Unhealthy","metrics":"malformed ignored"}`}, compactHostSnapshot{})
	unhealthy := unhealthySnapshot.candidate("m")
	if reason != 0 || unhealthy.endpoint.Healthy {
		t.Fatalf("unhealthy candidate=%+v reason=%d", unhealthy, reason)
	}
}

func TestMalformedHostDoesNotPreventHealthyCandidateScheduling(t *testing.T) {
	validMetrics := "# TYPE vllm:num_requests_waiting gauge\nvllm:num_requests_waiting 1\n"
	validMetadata := `{"health_status":"Healthy","metrics":` + strconv.Quote(validMetrics) + `}`
	hosts := [][]string{
		{"bad", `{"health_status":"Healthy","metrics":"vllm:num_requests_waiting nope\\n"}`},
		{"good", validMetadata},
	}
	endpoints := make([]scheduling.EndpointSnapshot, 0, len(hosts))
	cache := newEndpointSnapshotCache(nil, scheduling.ParseCompactVLLMMetrics, nil)
	for _, host := range hosts {
		snapshot, reason := cache.parseHost([2]string{host[0], host[1]}, compactHostSnapshot{})
		if reason == 0 {
			endpoints = append(endpoints, snapshot.candidate("m").endpoint)
		}
	}
	decision := scheduling.NewPipeline(scheduling.Weights{scheduling.SignalQueue: 1}, nil).Schedule(endpoints)
	if decision.Address != "good" || decision.FallbackReason != "" {
		t.Fatalf("isolated scheduling decision=%+v", decision)
	}
}

func TestBatchedTokenPromptLeavesQueueSchedulingAvailable(t *testing.T) {
	locality, prefixAvailable, err := prefixcache.Extract([]byte(`{"model":"m","prompt":[[1,2],[3,4]]}`))
	if err != nil || prefixAvailable || locality != nil {
		t.Fatalf("batched prompt prefix result locality=%+v available=%v err=%v", locality, prefixAvailable, err)
	}
	decision := scheduling.NewPipeline(scheduling.Weights{scheduling.SignalQueue: 1}, nil).Schedule([]scheduling.EndpointSnapshot{
		{
			Address: "queue-observed",
			Healthy: true,
			Signals: map[scheduling.SignalName]scheduling.SignalValue{
				scheduling.SignalQueue: {Value: 1, Available: true, Confidence: 1},
			},
		},
	})
	if decision.Address != "queue-observed" || decision.FallbackReason != "" {
		t.Fatalf("queue scheduling failed after prefix-unsupported prompt: %+v", decision)
	}
}

func TestDeepRequestPreflightRunsBeforeRecursiveJSONAccess(t *testing.T) {
	depth65 := deepChatMetadataRequest(65)
	depth20000 := deepChatMetadataRequest(20_000)
	for _, body := range [][]byte{depth65, depth20000} {
		model, locality, prefixAvailable, err := inspectRequestBody(body, prefixcache.DefaultToolMode, prefixcache.DefaultMaxBlocks, prefixcache.DefaultBlockSizeTokens)
		if err != nil || model != "m" || prefixAvailable || locality != nil {
			t.Fatalf("deep request inspection model=%q locality=%+v available=%v err=%v", model, locality, prefixAvailable, err)
		}
		decision := scheduling.NewPipeline(scheduling.Weights{scheduling.SignalQueue: 1}, nil).Schedule([]scheduling.EndpointSnapshot{
			{
				Address: "queue-observed",
				Healthy: true,
				Signals: map[scheduling.SignalName]scheduling.SignalValue{
					scheduling.SignalQueue: {Value: 1, Available: true, Confidence: 1},
				},
			},
		})
		if decision.Address != "queue-observed" || decision.FallbackReason != "" {
			t.Fatalf("queue scheduling failed for deep prefix-unsupported request: %+v", decision)
		}
	}
	baselineAllocs := testing.AllocsPerRun(10, func() {
		_, _, _, _ = inspectRequestBody(depth65, prefixcache.DefaultToolMode, prefixcache.DefaultMaxBlocks, prefixcache.DefaultBlockSizeTokens)
	})
	deepAllocs := testing.AllocsPerRun(10, func() {
		_, _, _, _ = inspectRequestBody(depth20000, prefixcache.DefaultToolMode, prefixcache.DefaultMaxBlocks, prefixcache.DefaultBlockSizeTokens)
	})
	if deepAllocs > baselineAllocs+4 {
		t.Fatalf("main request inspection allocations grew with depth: depth65=%v depth20000=%v", baselineAllocs, deepAllocs)
	}
}

func TestRequestInspectionRejectsInvalidJSONAndModel(t *testing.T) {
	for _, body := range []string{
		`{"model":"m",}`,
		`{"messages":[]}`,
		`{"model":3,"messages":[]}`,
	} {
		if _, _, _, err := inspectRequestBody([]byte(body), prefixcache.DefaultToolMode, prefixcache.DefaultMaxBlocks, prefixcache.DefaultBlockSizeTokens); err == nil {
			t.Fatalf("invalid request succeeded: %s", body)
		}
	}
}

func deepChatMetadataRequest(depth int) []byte {
	var body strings.Builder
	body.Grow(depth*2 + 100)
	body.WriteString(`{"model":"m","messages":[{"role":"user","content":"hello","metadata":`)
	body.WriteString(strings.Repeat("[", depth))
	body.WriteByte('0')
	body.WriteString(strings.Repeat("]", depth))
	body.WriteString(`}]}`)
	return []byte(body.String())
}

func TestSyncPrefixCapacityDeletesUnhealthyAndKeepsHealthyDefault(t *testing.T) {
	index := prefixcache.NewIndex(2)
	chain := [][]prefixcache.Block{{
		{Hash: 1, EstimatedTokens: 1},
		{Hash: 2, EstimatedTokens: 1},
	}}
	index.Record("host", chain, 16)
	syncPrefixCapacity(index, "host", false, 0, 2)
	if index.Len("host") != 0 || index.EndpointCount() != 0 {
		t.Fatalf("unhealthy prefix state remains: len=%d endpoints=%d", index.Len("host"), index.EndpointCount())
	}

	syncPrefixCapacity(index, "host", true, 0, 2)
	index.Record("host", chain, 16)
	if index.Len("host") != 2 {
		t.Fatalf("healthy host without num_gpu_blocks did not use default capacity: len=%d", index.Len("host"))
	}
}

func TestSyncPrefixCapacityClampsReportedCapacity(t *testing.T) {
	index := prefixcache.NewIndex(2)
	syncPrefixCapacity(index, "host", true, 1000, 2)
	index.Record("host", [][]prefixcache.Block{{
		{Hash: 1, EstimatedTokens: 1},
		{Hash: 2, EstimatedTokens: 1},
		{Hash: 3, EstimatedTokens: 1},
	}}, 16)
	if index.Len("host") != 2 {
		t.Fatalf("reported capacity bypassed configured cap: len=%d", index.Len("host"))
	}
}

func (s *requestBodyControlStub) DontReadRequestBody() {
	s.dontRead = true
}

func (s *requestBodyControlStub) SetRequestBodyBufferLimit(limit uint32) {
	s.bufferLimit = limit
}

func TestRequestHeadersAction(t *testing.T) {
	tests := []struct {
		name            string
		framing         requestBodyFraming
		want            types.Action
		wantBufferLimit uint32
	}{
		{
			name:    "header-only POST with no framing",
			framing: requestBodyFraming{contentType: "application/json"},
			want:    types.HeaderContinue,
		},
		{
			name:    "explicit zero content length",
			framing: requestBodyFraming{contentLength: "0", contentType: "application/json"},
			want:    types.HeaderContinue,
		},
		{
			name: "compressed JSON",
			framing: requestBodyFraming{
				contentLength: "128", contentType: "application/json", contentEncoding: "gzip",
			},
			want: types.HeaderContinue,
		},
		{
			name: "gRPC",
			framing: requestBodyFraming{
				contentLength: "128", contentType: "application/grpc+proto",
			},
			want: types.HeaderContinue,
		},
		{
			name: "octet stream",
			framing: requestBodyFraming{
				contentLength: "128", contentType: "application/octet-stream",
			},
			want: types.HeaderContinue,
		},
		{
			name: "websocket upgrade",
			framing: requestBodyFraming{
				contentLength: "128", contentType: "application/json", connection: "keep-alive, Upgrade", upgrade: "websocket",
			},
			want: types.HeaderContinue,
		},
		{
			name: "positive content length JSON",
			framing: requestBodyFraming{
				contentLength: "128", contentType: "application/json",
			},
			want:            types.HeaderStopIteration,
			wantBufferLimit: 1024,
		},
		{
			name: "oversized content length fails open",
			framing: requestBodyFraming{
				contentLength: "1025", contentType: "application/json",
			},
			want: types.HeaderContinue,
		},
		{
			name: "chunked JSON fails open without buffering",
			framing: requestBodyFraming{
				transferEncoding: "gzip, chunked", contentType: "application/json",
			},
			want: types.HeaderContinue,
		},
		{
			name: "content length with chunked transfer encoding fails open",
			framing: requestBodyFraming{
				contentLength: "128", transferEncoding: "chunked", contentType: "application/json",
			},
			want: types.HeaderContinue,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			control := &requestBodyControlStub{}
			if got := requestHeadersAction(control, test.framing, 1024); got != test.want {
				t.Fatalf("requestHeadersAction() = %v, want %v", got, test.want)
			}
			wantDontRead := test.want == types.HeaderContinue
			if control.dontRead != wantDontRead {
				t.Fatalf("DontReadRequestBody called = %v, want %v", control.dontRead, wantDontRead)
			}
			if control.bufferLimit != test.wantBufferLimit {
				t.Fatalf("buffer limit = %d, want %d", control.bufferLimit, test.wantBufferLimit)
			}
		})
	}
}
