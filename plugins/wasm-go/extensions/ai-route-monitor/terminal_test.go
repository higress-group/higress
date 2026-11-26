package main

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	wasmtest "github.com/higress-group/wasm-go/pkg/test"
	"github.com/tidwall/gjson"
)

func TestConfig(t *testing.T) {
	for _, raw := range []string{`{}`, `{"terminalMetrics":true,"source":{"headerName":"X-Product","allowedValues":["desktop"],"fallbackValue":"other"},"models":{"allowedValues":["gpt-4"]}}`} {
		var c RouteMonitorConfig
		if err := parseConfig(gjson.Parse(raw), &c, nil); err != nil {
			t.Fatal(err)
		}
		if !c.legacyMetrics {
			t.Fatal("legacy metrics must default on")
		}
	}
	for _, raw := range []string{
		`[]`, `1`,
		`{"terminalMetrics":"true"}`, `{"legacyMetrics":1}`, `{"source":[]}`,
		`{"source":{"headerName":":path"}}`, `{"source":{"headerName":"x\nsource"}}`,
		`{"source":{"allowedValues":"wujieai"}}`, `{"source":{"allowedValues":[1]}}`,
		`{"models":{"allowedValues":[false]}}`, `{"models":{"fallbackValue":""}}`,
		`{"asyncSubmissionPaths":["/tasks?x=1"]}`, `{"asyncSubmissionPaths":[false]}`,
	} {
		var c RouteMonitorConfig
		if err := parseConfig(gjson.Parse(raw), &c, nil); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
	var c RouteMonitorConfig
	if err := parseConfig(gjson.Parse(`{"source":{"allowedValues":["web"]}}`), &c, nil); err != nil {
		t.Fatal(err)
	}
	if c.source.value("web") != "web" || c.source.value("unbounded-user-value") != "unknown" || c.source.value("") != "unknown" {
		t.Fatal("unbounded source")
	}
	if err := parseConfig(gjson.Parse(`{"models":{"allowedValues":["claude-sonnet-4.5"]}}`), &c, nil); err != nil {
		t.Fatal(err)
	}
	if c.models.value("claude-sonnet-4.5") != "claude-sonnet-4_5" {
		t.Fatal("configured model not normalized")
	}
}

func newHost(t *testing.T, config string) wasmtest.TestHost {
	t.Helper()
	host, status := wasmtest.NewTestHost(json.RawMessage(config))
	if status != types.OnPluginStartStatusOK {
		host.Reset()
		t.Fatal("plugin start failed")
	}
	t.Cleanup(host.Reset)
	return host
}

func metricValue(t *testing.T, host wasmtest.TestHost, name string) uint64 {
	t.Helper()
	value, _ := host.GetCounterMetric(name) // undefined metric has value zero for these assertions
	return value
}

func requestHeaders(host wasmtest.TestHost, headers [][2]string) {
	host.CallOnHttpRequestHeaders(append(headers, [2]string{":authority", "models.test"}))
}

func TestLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name, status, contentType, body, source, outcome string
		local, headersOnly, bodyEnd                      bool
		flags                                            uint64
	}{
		{name: "json success", status: "200", contentType: "application/json", body: `{"choices":[{"message":{"content":"ok"}}]}`, source: "wujieai", outcome: "success", bodyEnd: true},
		{name: "429", status: "429", contentType: "application/json", body: `{"error":"quota"}`, source: "wujieai", outcome: "http_error", bodyEnd: true},
		{name: "500", status: "500", headersOnly: true, source: "wujieai", outcome: "http_error"},
		{name: "local reject", status: "403", headersOnly: true, local: true, source: "wujieai", outcome: "http_error"},
		{name: "sse done", status: "200", contentType: "text/event-stream", body: "data: {\"choices\":[]}\n\ndata: [DONE]\n\n", source: "wujieai", outcome: "success", bodyEnd: true},
		{name: "sse error 200", status: "200", contentType: "text/event-stream", body: "event: error\ndata: {\"error\":{\"message\":\"failed\"}}\n\ndata: [DONE]\n\n", source: "wujieai", outcome: "stream_error", bodyEnd: true},
		{name: "sse missing terminator", status: "200", contentType: "text/event-stream", body: "data: {\"choices\":[]}\n\n", source: "wujieai", outcome: "unknown", bodyEnd: true},
		{name: "timeout", status: "200", contentType: "text/event-stream", flags: 1 << 16, source: "wujieai", outcome: "timeout"},
		{name: "downstream reset", status: "200", flags: 1 << 14, source: "wujieai", outcome: "interrupted"},
		{name: "downstream remote reset", status: "200", flags: 1 << 28, source: "wujieai", outcome: "interrupted"},
		{name: "unknown no eof", status: "200", source: "wujieai", outcome: "unknown"},
		{name: "missing source", status: "204", headersOnly: true, outcome: "success"},
		{name: "unknown source", status: "200", headersOnly: true, source: "arbitrary", outcome: "success"},
		{name: "redirect", status: "302", headersOnly: true, source: "wujieai", outcome: "unknown"},
		{name: "http 200 json error", status: "200", contentType: "application/json", body: `{"error":{"message":"failed"}}`, source: "wujieai", outcome: "stream_error", bodyEnd: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			host := newHost(t, `{"terminalMetrics":true,"source":{"allowedValues":["wujieai"]},"models":{"allowedValues":["gpt-4"]}}`)
			requestHeaders(host, [][2]string{{":method", "POST"}, {":path", "/v1/chat/completions"}, {"x-wujie-source", tc.source}})
			// Property becomes available only after monitor's request headers.
			host.SetProperty([]string{"wasm.requestModel"}, []byte("gpt-4"))
			if tc.local {
				host.SetProperty([]string{"response", "code_details"}, []byte("ext_authz_denied"))
			} else {
				host.SetProperty([]string{"response", "code_details"}, []byte("via_upstream"))
			}
			host.CallOnHttpResponseHeaders([][2]string{{":status", tc.status}, {"content-type", tc.contentType}}, wasmtest.WithEndOfStream(tc.headersOnly))
			source := tc.source
			if source != "wujieai" {
				source = "unknown"
			}
			name := terminalMetricName(false, source, "gpt-4", tc.outcome)
			if metricValue(t, host, name) != 0 {
				t.Fatal("counted before completion")
			}
			if tc.body != "" {
				// Every byte boundary is a possible transport chunk boundary.
				for i := range []byte(tc.body) {
					host.CallOnHttpStreamingResponseBody([]byte(tc.body[i:i+1]), tc.bodyEnd && i == len(tc.body)-1)
				}
			}
			flags := make([]byte, 8)
			binary.LittleEndian.PutUint64(flags, tc.flags)
			host.SetProperty([]string{"response", "flags"}, flags)
			host.CompleteHttp()
			if got := metricValue(t, host, name); got != 1 {
				t.Fatalf("counter %s=%d, want 1", name, got)
			}
		})
	}
}

func TestSSEBoundaries(t *testing.T) {
	for _, tc := range []struct{ name, body, outcome string }{
		{"openai", "data: {\"choices\":[]}\r\n\r\ndata: [DONE]\r\n\r\n", "success"},
		{"anthropic", "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n", "success"},
		{"responses", "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n", "success"},
		{"responses incomplete", "data: {\"type\":\"response.incomplete\"}\n\n", "stream_error"},
		{"multiline", "data: {\n" + "data: \"type\":\"message_stop\"}\n\n", "success"},
		{"truncated event", "data: [DONE]\n", "unknown"},
		{"malformed", "data: broken\n\ndata: [DONE]\n\n", "unknown"},
		{"null error", "data: {\"error\":null}\n\ndata: [DONE]\n\n", "success"},
		{"content filter", "data: {\"choices\":[{\"finish_reason\":\"content_filter\"}]}\n\ndata: [DONE]\n\n", "stream_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for split := 0; split <= len(tc.body); split++ {
				s := requestTerminal{status: 200, stream: true, inspectable: true}
				s.observeBody([]byte(tc.body[:split]), false)
				s.observeBody([]byte(tc.body[split:]), true)
				if got := s.outcome(0); got != tc.outcome {
					t.Fatalf("split %d: got %s", split, got)
				}
			}
		})
	}
	s := requestTerminal{status: 200, stream: true, inspectable: true}
	for i := 0; i < 100; i++ {
		s.observeBody([]byte(strings.Repeat("x", 4096)), false)
	}
	s.observeBody([]byte("\n\ndata: [DONE]\n\n"), true)
	if len(s.line) > inspectionLimit || len(s.data) > inspectionLimit || s.outcome(0) != "unknown" {
		t.Fatal("inspection bounds or unknown classification violated")
	}
}

func TestDefaultsAndRules(t *testing.T) {
	host := newHost(t, `{"_rules_":[{"_match_route_":["desktop-route"],"terminalMetrics":true,"legacyMetrics":false,"source":{"headerName":"x-product","allowedValues":["desktop"],"fallbackValue":"other"},"models":{"allowedValues":["gpt-4"]}}]}`)
	host.SetRouteName("desktop-route")
	requestHeaders(host, [][2]string{{":method", "POST"}, {":path", "/sync"}, {"x-product", "desktop"}})
	host.SetProperty([]string{"wasm.requestModel"}, []byte("unbounded-model"))
	host.CallOnHttpResponseHeaders([][2]string{{":status", "204"}}, wasmtest.WithEndOfStream(true))
	host.CompleteHttp()
	if metricValue(t, host, terminalMetricName(false, "desktop", "unknown", "success")) != 1 {
		t.Fatal("matched config not used")
	}
}

func TestLegacyAndIndependentSwitches(t *testing.T) {
	for _, tc := range []struct {
		config           string
		legacy, terminal uint64
	}{
		{`{}`, 1, 0},
		{`{"terminalMetrics":true}`, 1, 1},
		{`{"terminalMetrics":true,"legacyMetrics":false}`, 0, 1},
	} {
		t.Run(tc.config, func(t *testing.T) {
			host := newHost(t, tc.config)
			requestHeaders(host, [][2]string{{":method", "POST"}, {":path", "/sync"}})
			host.SetProperty([]string{"response", "code_details"}, []byte("via_upstream"))
			host.CallOnHttpResponseHeaders([][2]string{{":status", "204"}}, wasmtest.WithEndOfStream(true))
			legacy := generateMetricName("test-route-default", "unknown", "unknown", "none", MetricRequestSuccess, "/sync")
			if got := metricValue(t, host, legacy); got != tc.legacy {
				t.Fatalf("legacy %d want %d", got, tc.legacy)
			}
			host.CompleteHttp()
			if got := metricValue(t, host, terminalMetricName(false, "unknown", "unknown", "success")); got != tc.terminal {
				t.Fatalf("terminal %d want %d", got, tc.terminal)
			}
		})
	}
}

func TestAsyncSubmissionSeparate(t *testing.T) {
	host := newHost(t, `{"terminalMetrics":true,"legacyMetrics":false,"asyncSubmissionPaths":["/v1/tasks"]}`)
	requestHeaders(host, [][2]string{{":method", "POST"}, {":path", "/v1/tasks?trace=1"}})
	host.CallOnHttpResponseHeaders([][2]string{{":status", "202"}}, wasmtest.WithEndOfStream(true))
	host.CompleteHttp()
	if metricValue(t, host, terminalMetricName(true, "unknown", "unknown", "success")) != 1 || metricValue(t, host, terminalMetricName(false, "unknown", "unknown", "success")) != 0 {
		t.Fatal("submission polluted synchronous success")
	}
}

func TestOnceAtLogAndLocalCodeWithoutEncodeHeaders(t *testing.T) {
	host := newHost(t, `{"terminalMetrics":true,"legacyMetrics":false}`)
	id := host.InitializeHttpContext()
	host.CallOnRequestHeaders(id, [][2]string{{":authority", "models.test"}, {":path", "/sync"}}, true)
	code := make([]byte, 8)
	binary.LittleEndian.PutUint64(code, 429)
	host.SetProperty([]string{"response", "code"}, code)
	ctx := proxywasm.GetHttpContext(id)
	ctx.OnHttpStreamDone()
	ctx.OnHttpStreamDone()
	host.CompleteHttpContext(id)
	if got := metricValue(t, host, terminalMetricName(false, "unknown", "unknown", "http_error")); got != 1 {
		t.Fatalf("completion counter=%d, want 1 after repeated log callbacks", got)
	}
}

func TestResponseFlagClassification(t *testing.T) {
	s := requestTerminal{status: 200, ended: true, inspectable: true}
	for _, tc := range []struct {
		flag uint64
		want string
	}{
		{1 << 2, "timeout"}, {1 << 16, "timeout"}, {1 << 19, "timeout"}, {1 << 22, "timeout"},
		{1 << 14, "interrupted"}, {1 << 28, "interrupted"}, {1 << 13, "unknown"},
		{1 << 9, "success"}, {1 << 20, "success"},
	} {
		if got := s.outcome(tc.flag); got != tc.want {
			t.Errorf("flag %d got %s want %s", tc.flag, got, tc.want)
		}
	}
}

func TestLegacyErrorBody(t *testing.T) {
	host := newHost(t, `{}`)
	requestHeaders(host, [][2]string{{":method", "POST"}, {":path", "/sync"}})
	host.SetProperty([]string{"response", "code_details"}, []byte("via_upstream"))
	host.CallOnHttpResponseHeaders([][2]string{{":status", "429"}, {"content-type", "application/json"}})
	host.CallOnHttpResponseBody([]byte(`{"error":{"message":"quota exceeded"}}`))
	host.CompleteHttp()
	for _, suffix := range []string{MetricErrorCount, "error_code_count_429", "error_type_count_insufficient_quota"} {
		name := generateMetricName("test-route-default", "unknown", "unknown", "none", suffix, "/sync")
		if got := metricValue(t, host, name); got != 1 {
			t.Fatalf("legacy error counter %s=%d", name, got)
		}
	}
}

func BenchmarkSSE(b *testing.B) {
	chunk := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		s := requestTerminal{status: 200, stream: true, inspectable: true}
		for j := 0; j < 100; j++ {
			s.observeBody(chunk, false)
		}
		s.observeBody([]byte("data: [DONE]\n\n"), true)
	}
}

func TestCompiledWasm(t *testing.T) {
	path := os.Getenv("AI_ROUTE_MONITOR_WASM")
	if path == "" {
		t.Skip("set AI_ROUTE_MONITOR_WASM to exercise compiled Wasm in SDK host emulator")
	}
	wasmtest.RunWasmTestWithPath(t, path, func(t *testing.T) {
		t.Run("lifecycle", TestLifecycle)
		t.Run("rules", TestDefaultsAndRules)
		t.Run("legacy", TestLegacyAndIndependentSwitches)
		t.Run("submission", TestAsyncSubmissionSeparate)
	})
}
