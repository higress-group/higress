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

	"github.com/tidwall/gjson"
)

func event(name, data string) string {
	out := ""
	if name != "" {
		out = "event: " + name + "\n"
	}
	return out + "data: " + data + "\n\n"
}
func TestProviderStreams(t *testing.T) {
	cases := []struct{ provider, stream, want string }{
		{"dify", event("", `{"event":"agent_message","conversation_id":"conv1","answer":"hello "}`) + event("", `{"event":"agent_message","answer":"世界"}`) + event("", `{"event":"message_end"}`), "hello 世界"},
		{"bailian", event("", `{"output":{"text":"hello ","session_id":"s1","finish_reason":"null"}}`) + event("", `{"output":{"text":"world","finish_reason":"stop"}}`), "hello world"},
		{"coze", event("conversation.chat.created", `{"conversation_id":"c1"}`) + event("conversation.message.delta", `{"type":"answer","content":"hello"}`) + event("conversation.message.completed", `{"type":"answer","content":"hello"}`) + event("conversation.chat.completed", `{}`), "hello"},
	}
	for _, tc := range cases {
		t.Run(tc.provider, func(t *testing.T) {
			for _, step := range []int{1, 2, 7, len(tc.stream)} {
				r := testRequest()
				r.Session = ""
				m := newMapper(testConfig(tc.provider), r)
				var out []byte
				for i := 0; i < len(tc.stream); i += step {
					end := i + step
					if end > len(tc.stream) {
						end = len(tc.stream)
					}
					out = append(out, m.feed([]byte(tc.stream[i:end]), end == len(tc.stream))...)
				}
				if m.text != tc.want || m.state != "TASK_STATE_COMPLETED" || m.r.ContextID == "" {
					t.Fatalf("step %d: %q %s %s", step, m.text, m.state, out)
				}
				decoder := sseDecoder{limit: 1 << 20}
				events, err := decoder.feed(out, true)
				if err != nil {
					t.Fatal(err)
				}
				for _, ev := range events {
					if !json.Valid([]byte(ev.Data)) {
						t.Fatal(ev)
					}
				}
				if bytes.Count(out, []byte(`TASK_STATE_COMPLETED`)) != 1 {
					t.Fatal("terminal repeated", string(out))
				}
			}
		})
	}
}
func TestStreamFailures(t *testing.T) {
	cases := []struct{ p, s string }{
		{"dify", event("", `{"event":"message","answer":"partial"}`)},
		{"dify", event("", `{"event":"error","message":"bad"}`)},
		{"bailian", event("", `{"code":"InvalidApiKey","message":"secret details"}`)},
		{"bailian", event("", `{"output":{"text":"partial","finish_reason":"length"}}`)},
		{"coze", event("conversation.chat.failed", `{"last_error":{"code":1,"msg":"bad"}}`)},
		{"dify", "data: {broken}\n\n"}, {"dify", "data: {\"event\":\"message_end\"}"},
	}
	for _, tc := range cases {
		m := newMapper(testConfig(tc.p), testRequest())
		m.feed([]byte(tc.s), true)
		if m.state != "TASK_STATE_FAILED" {
			t.Fatalf("%s: %s", tc.s, m.state)
		}
	}
}
func TestLimitsAndReplacement(t *testing.T) {
	c := testConfig("dify")
	c.MaxBodyBytes = 64
	m := newMapper(c, testRequest())
	m.feed([]byte("data: "+strings.Repeat("x", 65)), false)
	if !m.done || m.state != "TASK_STATE_FAILED" {
		t.Fatal("unbounded frame")
	}
	m = newMapper(testConfig("dify"), testRequest())
	m.feed([]byte(event("", `{"event":"message","answer":"old"}`)+event("", `{"event":"message_replace","answer":"new"}`)+event("", `{"event":"message_end"}`)), true)
	if m.text != "new" {
		t.Fatal(m.text)
	}
}
func TestSSECRLFCommentsMultiline(t *testing.T) {
	s := ": ping\r\nevent: message\r\ndata: {\r\ndata: \"event\":\"message_end\"}\r\n\r\n"
	d := sseDecoder{limit: 1000}
	var events []sseEvent
	for _, b := range []byte(s) {
		ev, e := d.feed([]byte{b}, false)
		if e != nil {
			t.Fatal(e)
		}
		events = append(events, ev...)
	}
	if len(events) != 1 || events[0].Name != "message" || !gjson.Valid(events[0].Data) {
		t.Fatal(events)
	}
}
func TestCloudEventSemantics(t *testing.T) {
	for _, p := range []string{"qoder", "claude-managed"} {
		t.Run(p, func(t *testing.T) {
			r := testRequest()
			r.Session = "session-1"
			m := newMapper(testConfig(p), r)
			frames := []string{`{"type":"session.status_idle","stop_reason":{"type":"end_turn"}}`, `{"type":"session.status_running","session_id":"session-1"}`, `{"type":"session.thread_status_idle","stop_reason":{"type":"end_turn"}}`, `{"type":"session.status_idle","session_id":"another-session","stop_reason":{"type":"end_turn"}}`, `{"type":"event_start","event":{"type":"agent.message","id":"e1"}}`, `{"type":"event_delta","event_id":"wrong","delta":{"type":"content_delta","content":{"type":"text","text":"WRONG"}}}`, `{"type":"event_delta","event_id":"e1","delta":{"type":"content_delta","content":{"type":"text","text":"hello"}}}`, `{"type":"agent.message","id":"e1","content":[{"type":"text","text":"hello"}]}`, `{"type":"session.error","error":{"retry_status":"retrying"}}`}
			for _, f := range frames {
				m.feed([]byte(event("", f)), false)
				if m.done {
					t.Fatalf("early completion %s", f)
				}
			}
			m.feed([]byte(event("", `{"type":"session.status_idle","stop_reason":{"type":"end_turn"}}`)), false)
			if m.text != "hello" || m.state != "TASK_STATE_COMPLETED" {
				t.Fatal(m.text, m.state)
			}
		})
	}
}
func TestCloudRequiresAction(t *testing.T) {
	m := newMapper(testConfig("qoder"), testRequest())
	m.running = true
	m.feed([]byte(event("", `{"type":"session.status_idle","stop_reason":{"type":"requires_action"}}`)), false)
	if m.state != "TASK_STATE_INPUT_REQUIRED" {
		t.Fatal(m.state)
	}
}
func TestBlockingAggregatesAgentSSE(t *testing.T) {
	m := newMapper(testConfig("dify"), testRequest())
	body := event("", `{"event":"agent_message","answer":"hello"}`) + event("", `{"event":"message_end"}`)
	v := gjson.ParseBytes(m.blocking([]byte(body), true))
	if v.Get("result.task.status.state").String() != "TASK_STATE_COMPLETED" || v.Get("result.task.artifacts.0.parts.0.text").String() != "hello" {
		t.Fatal(v.Raw)
	}
}

func TestCloudPartialPreviewReconciliation(t *testing.T) {
	m := newMapper(testConfig("qoder"), testRequest())
	m.running = true
	m.messageTextUpdate("event1", "Hello", true)
	m.messageTextUpdate("event1", "Hello world", false)
	m.messageTextUpdate("event1", "Hello world", false)
	if m.text != "Hello world" {
		t.Fatal(m.text)
	}
	m.messageTextUpdate("event2", "!", false)
	m.messageTextUpdate("event1", "Changed", false)
	if m.text != "Changed!" {
		t.Fatal(m.text)
	}
}

func TestNewSessionContextIsStableAndRequired(t *testing.T) {
	r := testRequest()
	r.Session = ""
	m := newMapper(testConfig("dify"), r)
	if out := m.feed([]byte(event("", `{"event":"message","answer":"hello"}`)), false); len(out) != 0 {
		t.Fatal("emitted missing context", string(out))
	}
	out := m.feed([]byte(event("", `{"event":"message_end","conversation_id":"native-id"}`)), true)
	if m.text != "hello" || m.r.ContextID == "" || !bytes.Contains(out, []byte(`"contextId"`)) {
		t.Fatal(string(out))
	}
	r = testRequest()
	r.Session = ""
	m = newMapper(testConfig("dify"), r)
	out = m.feed([]byte(event("", `{"event":"error"}`)), true)
	if !bytes.Contains(out, []byte(`"error"`)) || bytes.Contains(out, []byte(`"statusUpdate"`)) {
		t.Fatal(string(out))
	}
}

func TestTerminalPrecedesTruncatedTrailingFrame(t *testing.T) {
	m := newMapper(testConfig("dify"), testRequest())
	m.feed([]byte(event("", `{"event":"message_end"}`)+": trailing comment"), true)
	if m.state != "TASK_STATE_COMPLETED" {
		t.Fatal(m.state)
	}
}

func TestCozePartialSnapshotReconciliation(t *testing.T) {
	m := newMapper(testConfig("coze"), testRequest())
	body := event("conversation.message.delta", `{"id":"m1","type":"answer","content":"Hello"}`) + event("conversation.message.completed", `{"id":"m1","type":"answer","content":"Hello world"}`) + event("conversation.chat.completed", `{}`)
	m.feed([]byte(body), true)
	if m.text != "Hello world" || m.state != "TASK_STATE_COMPLETED" {
		t.Fatal(m.text, m.state)
	}
}
