// Copyright 2026 Alibaba Group Holding Ltd.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0
package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/tidwall/gjson"
)

func managedStatus(status, reason string) string {
	b, _ := json.Marshal(map[string]any{"type": "session_status", "content": []any{map[string]any{"type": "data", "data": map[string]any{"session_status": status, "stop_reason": map[string]any{"type": reason}}}}})
	return string(b)
}
func managedFrame(m *streamMapper, data string) []byte {
	return m.feed([]byte(event("message", data)), false)
}
func TestBailianManagedConfigAndRequest(t *testing.T) {
	c := testConfig("bailian-managed")
	if c.APIBasePath != "/api/v1/agentstudio" || !c.cloud() || header(authHeaders(c), "authorization") != "Bearer secret" {
		t.Fatal(c)
	}
	for _, mutate := range []func(*config){func(c *config) { c.AgentID = "" }, func(c *config) { c.SessionConfig = nil }, func(c *config) { c.UpstreamCluster = "" }, func(c *config) { c.UpstreamAuthority = "" }} {
		invalid := c
		mutate(&invalid)
		if invalid.validate() == nil {
			t.Fatal("accepted incomplete cloud configuration")
		}
	}
	if _, _, err := parseRequest(input("SendMessage"), c, "alice"); err == nil {
		t.Fatal("accepted blocking")
	}
	b := strings.Replace(string(input("SendStreamingMessage")), `"messageId":"m1"`, `"messageId":"m1","contextId":"`+c.signContext("s", "alice")+`"`, 1)
	if _, _, err := parseRequest([]byte(b), c, "alice"); err == nil {
		t.Fatal("accepted cloud continuation")
	}
	if _, _, err := parseRequest(input("SendStreamingMessage"), c, "alice"); err != nil {
		t.Fatal(err)
	}
	v := gjson.ParseBytes(cloudMessage(c, "hello"))
	if v.Get("input.0.type").String() != "message" || v.Get("input.0.role").String() != "user" || v.Get("input.0.content.0.text").String() != "hello" || v.Get("events").Exists() {
		t.Fatal(v.Raw)
	}
}
func TestHostBailianManagedSubscriptionAndSubmission(t *testing.T) {
	h, id := testHost(t, "bailian-managed")
	b := input("SendStreamingMessage")
	requestHeaders := headers(b)
	for i, h := range requestHeaders {
		if h[0] == "content-length" {
			requestHeaders[i] = [2]string{"transfer-encoding", "chunked"}
		}
	}
	h.CallOnRequestHeaders(id, requestHeaders, false)
	h.CallOnRequestBody(id, b, true)
	calls := h.GetCalloutAttributesFromContext(id)
	if len(calls) != 1 || header(calls[0].Headers, ":path") != "/api/v1/agentstudio/sessions" || header(calls[0].Headers, "authorization") != "Bearer secret" || gjson.GetBytes(calls[0].Body, "agent").String() != "agent-1" || gjson.GetBytes(calls[0].Body, "environment_id").String() != "env1" {
		t.Fatal(calls)
	}
	h.CallOnHttpCallResponse(calls[0].CalloutID, [][2]string{{":status", "201"}}, nil, []byte(`{"id":"session-new"}`))
	req := h.GetCurrentRequestHeaders(id)
	if len(h.GetCalloutAttributesFromContext(id)) != 0 || header(req, ":method") != "GET" || header(req, ":path") != "/api/v1/agentstudio/sessions/session-new/events/stream?event_deltas%5B%5D=message" || header(req, "authorization") != "Bearer secret" || header(req, "x-dashscope-sse") != "" || header(req, "x-consumer") != "" {
		t.Fatal(req)
	}
	if header(req, "content-length") != "0" || header(req, "transfer-encoding") != "" || len(h.GetCurrentRequestBody(id)) != 0 {
		t.Fatal("SSE subscription must have an explicitly empty body", req)
	}
	if a := h.CallOnResponseHeaders(id, [][2]string{{":status", "200"}, {"content-type", "text/event-stream"}}, false); a != types.HeaderStopAllIterationAndWatermark {
		t.Fatal(a)
	}
	calls = h.GetCalloutAttributesFromContext(id)
	if len(calls) != 1 || header(calls[0].Headers, ":path") != "/api/v1/agentstudio/sessions/session-new/events" || !bytes.Equal(calls[0].Body, cloudMessage(testConfig("bailian-managed"), "hello")) {
		t.Fatal(calls)
	}
	h.CallOnHttpCallResponse(calls[0].CalloutID, [][2]string{{":status", "202"}}, nil, []byte(`{}`))
	if h.GetSentLocalResponse(id) != nil {
		t.Fatal("successful submission rejected")
	}
	body := event("message", managedStatus("running", "")) + event("message", `{"type":"message","role":"assistant","id":"answer","content":[{"type":"text","text":"hello"}]}`) + event("message", managedStatus("idle", "end_turn"))
	h.CallOnResponseBody(id, []byte(body), true)
	if !bytes.Contains(h.GetCurrentResponseBody(id), []byte("TASK_STATE_COMPLETED")) {
		t.Fatal(string(h.GetCurrentResponseBody(id)))
	}
}
func TestBailianManagedRealEventShapes(t *testing.T) {
	frames := []string{
		managedStatus("idle", "end_turn"), managedStatus("running", ""),
		`{"type":"message","role":"user","thread_id":"root","content":[{"type":"text","text":"do not echo"}]}`,
		`{"type":"model_request_start","thread_id":"root","content":[{"type":"data","data":{"model":"test-model"}}]}`,
		`{"type":"tool_call","role":"assistant","thread_id":"root","content":[{"type":"data","data":{"name":"bash","arguments":"echo 6","call_id":"call1"}}]}`,
		`{"type":"tool_call_output","role":"tool","thread_id":"root","is_error":false,"content":[{"type":"data","data":{"name":"bash","output":"6","call_id":"call1"}}]}`,
		`{"type":"event_start","event":{"id":"answer","type":"message"}}`,
		`{"type":"event_delta","event_id":"answer","delta":{"type":"content_delta","index":0,"content":{"type":"text","text":"hel"}}}`,
		`{"type":"model_request_end","thread_id":"root","content":[{"type":"data","data":{"output_tokens":2}}]}`,
		`{"type":"message","role":"assistant","thread_id":"root","id":"answer","content":[{"type":"text","text":"hello"}]}`,
		managedStatus("idle", "end_turn"),
	}
	stream := ""
	for _, f := range frames {
		stream += event("message", f)
	}
	for _, size := range []int{1, 7, len(stream)} {
		m := newMapper(testConfig("bailian-managed"), testRequest())
		var out []byte
		for i := 0; i < len(stream); i += size {
			end := i + size
			if end > len(stream) {
				end = len(stream)
			}
			out = append(out, m.feed([]byte(stream[i:end]), false)...)
		}
		if m.text != "hello" || m.state != "TASK_STATE_COMPLETED" || bytes.Count(out, []byte("TASK_STATE_COMPLETED")) != 1 {
			t.Fatal(m.text, m.state, string(out))
		}
		for _, name := range []string{"tool_call", "tool_call_output", "model_request_start", "model_request_end"} {
			if !bytes.Contains(out, []byte(`"event":"`+name+`"`)) {
				t.Fatal(name, string(out))
			}
		}
		if bytes.Contains(out, []byte("do not echo")) {
			t.Fatal("echoed user input")
		}
	}
}
func TestBailianManagedThreadAndSessionIsolation(t *testing.T) {
	m := newMapper(testConfig("bailian-managed"), testRequest())
	managedFrame(m, managedStatus("running", ""))
	managedFrame(m, `{"type":"message","role":"user","thread_id":"root"}`)
	for _, f := range []string{
		`{"type":"session_status","session_id":"other","content":[{"type":"data","data":{"session_status":"idle","stop_reason":{"type":"end_turn"}}}]}`,
		`{"type":"session_status","thread_id":"child","content":[{"type":"data","data":{"session_status":"idle","stop_reason":{"type":"end_turn"}}}]}`,
		`{"type":"session_status","metadata":{"thread_id":"child"},"content":[{"type":"data","data":{"session_status":"terminated"}}]}`,
		`{"type":"event_start","event":{"id":"child","type":"message","thread_id":"child"}}`,
		`{"type":"event_start","event":{"id":"other","type":"message","session_id":"other"}}`,
		`{"type":"event_delta","event_id":"child","delta":{"type":"content_delta","index":0,"content":{"type":"text","text":"wrong"}}}`,
		`{"type":"message","id":"child","role":"assistant","thread_id":"child","content":[{"type":"text","text":"wrong"}]}`,
		`{"type":"message","id":"tool","role":"tool","content":[{"type":"text","text":"wrong"}]}`,
	} {
		if out := managedFrame(m, f); len(out) != 0 || m.done {
			t.Fatal(f, string(out))
		}
	}
	managedFrame(m, `{"type":"message","id":"right","role":"assistant","metadata":{"thread_id":"root"},"content":[{"type":"text","text":"right"}]}`)
	managedFrame(m, managedStatus("idle", "end_turn"))
	if m.text != "right" || m.state != "TASK_STATE_COMPLETED" {
		t.Fatal(m.text, m.state)
	}
}
func TestBailianManagedPreviewBlocksAndSnapshots(t *testing.T) {
	m := newMapper(testConfig("bailian-managed"), testRequest())
	m.running = true
	for _, f := range []string{
		`{"type":"event_start","event":{"id":"a","type":"message"}}`,
		`{"type":"event_delta","event_id":"a","delta":{"type":"content_delta","index":1,"content":{"type":"text","text":"world"}}}`,
		`{"type":"event_delta","event_id":"a","delta":{"type":"content_delta","index":0,"content":{"type":"text","text":"hello "}}}`,
	} {
		managedFrame(m, f)
	}
	if m.text != "hello world" {
		t.Fatal(m.text)
	}
	snapshot := `{"type":"message","role":"assistant","id":"a","content":[{"type":"text","text":"corrected"}]}`
	out := managedFrame(m, snapshot)
	if !bytes.Contains(out, []byte(`"append":false`)) || m.text != "corrected" {
		t.Fatal(string(out))
	}
	if out = managedFrame(m, snapshot); len(out) != 0 {
		t.Fatal("duplicate snapshot", string(out))
	}
	managedFrame(m, `{"type":"event_delta","event_id":"a","delta":{"type":"content_delta","index":0,"content":{"type":"text","text":"stale"}}}`)
	if m.text != "corrected" {
		t.Fatal("late preview changed authoritative text", m.text)
	}
}
func TestBailianManagedTerminalAndMalformed(t *testing.T) {
	cases := []struct{ frame, state string }{
		{managedStatus("idle", "requires_action"), "TASK_STATE_INPUT_REQUIRED"},
		{managedStatus("idle", "retries_exhausted"), "TASK_STATE_FAILED"},
		{managedStatus("terminated", ""), "TASK_STATE_FAILED"},
		{managedStatus("idle", ""), "TASK_STATE_FAILED"},
		{`{"type":"session_status"}`, "TASK_STATE_FAILED"},
		{`{"type":"message","role":"assistant","content":[{"type":"text","text":"x"}]}`, "TASK_STATE_FAILED"},
		{`{"type":"message","role":"assistant","id":"a","content":[{"type":"text","text":3}]}`, "TASK_STATE_FAILED"},
		{`{"type":"event_start","event":{"type":"message"}}`, "TASK_STATE_FAILED"},
		{`{"type":"event_delta","event_id":"a","delta":{"type":"content_delta","index":-1,"content":{"type":"text","text":"x"}}}`, "TASK_STATE_FAILED"},
		{`{"type":"event_delta","event_id":"a","delta":{"type":"content_delta","content":{"type":"text","text":"x"}}}`, "TASK_STATE_FAILED"},
		{`{broken}`, "TASK_STATE_FAILED"},
	}
	for _, tc := range cases {
		m := newMapper(testConfig("bailian-managed"), testRequest())
		m.running = true
		managedFrame(m, `{"type":"event_start","event":{"type":"message","id":"a"}}`)
		managedFrame(m, tc.frame)
		if !m.done || m.state != tc.state {
			t.Fatal(tc.frame, m.state)
		}
	}
	m := newMapper(testConfig("bailian-managed"), testRequest())
	managedFrame(m, managedStatus("idle", "end_turn"))
	if m.done {
		t.Fatal("initial idle completed turn")
	}
	m.feed(nil, true)
	if m.state != "TASK_STATE_FAILED" {
		t.Fatal("missing terminal accepted")
	}
	m = newMapper(testConfig("bailian-managed"), testRequest())
	managedFrame(m, managedStatus("terminated", ""))
	if m.state != "TASK_STATE_FAILED" {
		t.Fatal("termination before running ignored")
	}
}
func TestBailianManagedLimits(t *testing.T) {
	c := testConfig("bailian-managed")
	c.MaxBodyBytes = 1024
	m := newMapper(c, testRequest())
	m.running = true
	for _, id := range []string{"a", "b"} {
		b, _ := json.Marshal(map[string]any{"type": "message", "role": "assistant", "id": id, "content": []any{map[string]any{"type": "text", "text": strings.Repeat("x", 600)}}})
		managedFrame(m, string(b))
	}
	if m.state != "TASK_STATE_FAILED" {
		t.Fatal("aggregate limit ignored")
	}
	m = newMapper(c, testRequest())
	m.feed([]byte("data: "+strings.Repeat("x", 1025)), false)
	if m.state != "TASK_STATE_FAILED" {
		t.Fatal("frame limit ignored")
	}
	m = newMapper(c, testRequest())
	m.feed([]byte(strings.Repeat(": keepalive\n\n", 2000)), false)
	if m.state != "TASK_STATE_FAILED" {
		t.Fatal("stream limit ignored")
	}
}

func TestBailianManagedChildSnapshotRetractsUnidentifiedPreview(t *testing.T) {
	m := newMapper(testConfig("bailian-managed"), testRequest())
	m.running = true
	m.managedRootThread = "root"
	managedFrame(m, `{"type":"event_start","event":{"id":"child-answer","type":"message"}}`)
	managedFrame(m, `{"type":"event_delta","event_id":"child-answer","delta":{"type":"content_delta","index":0,"content":{"type":"text","text":"provisional"}}}`)
	out := managedFrame(m, `{"type":"message","id":"child-answer","role":"assistant","thread_id":"child","content":[{"type":"text","text":"child result"}]}`)
	if m.text != "" || !bytes.Contains(out, []byte(`"append":false`)) {
		t.Fatal(m.text, string(out))
	}
	managedFrame(m, `{"type":"message","id":"root-answer","role":"assistant","thread_id":"root","content":[{"type":"text","text":"root result"}]}`)
	managedFrame(m, managedStatus("idle", "end_turn"))
	if m.text != "root result" || m.state != "TASK_STATE_COMPLETED" {
		t.Fatal(m.text, m.state)
	}
}
