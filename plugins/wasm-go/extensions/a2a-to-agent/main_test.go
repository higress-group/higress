// Copyright 2026 Alibaba Group Holding Ltd.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0
package main

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/proxytest"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

func testHost(t *testing.T, p string) (proxytest.HostEmulator, uint32) {
	t.Helper()
	c := testConfig(p)
	b, _ := json.Marshal(c)
	vm := wrapper.NewCommonVmCtx("a2a-to-agent", wrapper.ParseConfig(parseConfig), wrapper.ProcessRequestHeaders(onRequestHeaders), wrapper.ProcessRequestBody(onRequestBody), wrapper.ProcessResponseHeaders(onResponseHeaders), wrapper.ProcessStreamingResponseBody(onStream), wrapper.ProcessResponseBody(onResponseBody))
	h, reset := proxytest.NewHostEmulator(proxytest.NewEmulatorOption().WithVMContext(vm).WithPluginConfiguration(b))
	t.Cleanup(reset)
	h.RegisterForeignFunction("get_log_level", func([]byte) []byte { return []byte{2, 0, 0, 0} })
	if h.StartVM() != types.OnVMStartStatusOK || h.StartPlugin() != types.OnPluginStartStatusOK {
		t.Fatal("plugin start failed")
	}
	return h, h.InitializeHttpContext()
}
func headers(body []byte) [][2]string {
	return [][2]string{{":method", "POST"}, {":path", "/a2a"}, {":authority", "gateway.example"}, {"content-type", "application/json"}, {"content-length", strconv.Itoa(len(body))}, {"x-consumer", "alice"}, {"authorization", "Bearer client-key"}, {"accept-encoding", "gzip"}}
}
func header(h [][2]string, name string) string {
	for _, v := range h {
		if v[0] == name {
			return v[1]
		}
	}
	return ""
}
func TestHostMapsBlockingAgentRequest(t *testing.T) {
	h, id := testHost(t, "dify")
	b := input("SendMessage")
	h.CallOnRequestHeaders(id, headers(b), false)
	h.CallOnRequestBody(id, b, true)
	if got := gjson.GetBytes(h.GetCurrentRequestBody(id), "response_mode").String(); got != "streaming" {
		t.Fatal(got)
	}
	req := h.GetCurrentRequestHeaders(id)
	if header(req, "authorization") != "Bearer secret" || header(req, "accept-encoding") != "" || header(req, "x-consumer") != "" {
		t.Fatal(req)
	}
	if a := h.CallOnResponseHeaders(id, [][2]string{{":status", "200"}, {"content-type", "text/event-stream"}}, false); a != types.HeaderStopIteration {
		t.Fatal(a)
	}
	body := event("", `{"event":"agent_message","conversation_id":"native-1","answer":"hello"}`) + event("", `{"event":"message_end"}`)
	h.CallOnResponseBody(id, []byte(body), true)
	if got := gjson.GetBytes(h.GetCurrentResponseBody(id), "result.task.status.state").String(); got != "TASK_STATE_COMPLETED" {
		t.Fatal(got, string(h.GetCurrentResponseBody(id)))
	}
}
func TestHostCloudOpensStreamBeforeSubmittingMessage(t *testing.T) {
	h, id := testHost(t, "qoder")
	b := input("SendStreamingMessage")
	h.CallOnRequestHeaders(id, headers(b), false)
	h.CallOnRequestBody(id, b, true)
	calls := h.GetCalloutAttributesFromContext(id)
	if len(calls) != 1 || header(calls[0].Headers, ":path") != "/api/v1/cloud/sessions" {
		t.Fatal(calls)
	}
	h.CallOnHttpCallResponse(calls[0].CalloutID, [][2]string{{":status", "201"}}, nil, []byte(`{"id":"session-new"}`))
	if len(h.GetCalloutAttributesFromContext(id)) != 0 {
		t.Fatal("message sent before SSE opened")
	}
	if req := h.GetCurrentRequestHeaders(id); header(req, ":method") != "GET" || header(req, "content-length") != "0" || len(h.GetCurrentRequestBody(id)) != 0 {
		t.Fatal(h.GetCurrentRequestHeaders(id))
	}
	a := h.CallOnResponseHeaders(id, [][2]string{{":status", "200"}, {"content-type", "text/event-stream"}}, false)
	if a != types.HeaderStopAllIterationAndWatermark {
		t.Fatal(a)
	}
	calls = h.GetCalloutAttributesFromContext(id)
	if len(calls) != 1 || gjson.GetBytes(calls[0].Body, "events.0.type").String() != "user.message" {
		t.Fatal(calls)
	}
	// Submission errors are local JSON-RPC responses while response headers are held.
	h.CallOnHttpCallResponse(calls[0].CalloutID, [][2]string{{":status", "409"}}, nil, []byte(`{"error":"conflict"}`))
	response := h.GetSentLocalResponse(id)
	if response == nil || response.StatusCode != 502 || !strings.Contains(string(response.Data), "submission failed") {
		t.Fatal(response)
	}
}
func TestHostRejectsNonSSEBlockingResponse(t *testing.T) {
	h, id := testHost(t, "dify")
	b := input("SendMessage")
	h.CallOnRequestHeaders(id, headers(b), false)
	h.CallOnRequestBody(id, b, true)
	h.CallOnResponseHeaders(id, [][2]string{{":status", "200"}, {"content-type", "application/octet-stream"}}, false)
	r := h.GetSentLocalResponse(id)
	if r == nil || r.StatusCode != 502 {
		t.Fatal(r)
	}
}

func TestHostRejectsNonJSONBeforeBody(t *testing.T) {
	h, id := testHost(t, "dify")
	h.CallOnRequestHeaders(id, [][2]string{{":method", "POST"}, {":path", "/a2a"}, {":authority", "gateway.example"}, {"x-consumer", "alice"}, {"content-type", "application/octet-stream"}, {"content-length", "20"}}, false)
	r := h.GetSentLocalResponse(id)
	if r == nil || r.StatusCode != 415 {
		t.Fatal(r)
	}
}
