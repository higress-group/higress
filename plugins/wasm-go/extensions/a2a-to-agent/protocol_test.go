// Copyright 2026 Alibaba Group Holding Ltd.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0
package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

func testConfig(provider string) config {
	c := config{Provider: provider, SessionConfig: map[string]any{"environment_id": "env1"}, APIKey: "secret", AgentID: "agent-1", ConsumerHeader: "x-consumer", ContextSecret: strings.Repeat("s", 32), UpstreamCluster: "mock", UpstreamAuthority: "mock.local", AgentCard: map[string]any{"name": "Demo", "url": "https://agent.test/a2a"}}
	if err := c.validate(); err != nil {
		panic(err)
	}
	return c
}
func input(method string) []byte {
	return []byte(`{"jsonrpc":"2.0","id":"req-1","method":"` + method + `","params":{"message":{"messageId":"m1","role":"ROLE_USER","parts":[{"text":"hello"}]}}}`)
}
func testRequest() *request {
	return &request{ID: json.RawMessage(`"r1"`), Text: "hello", Consumer: "alice", Session: "session-1", Streaming: true, TaskID: "task-1"}
}
func TestNativeRequestMappings(t *testing.T) {
	for _, p := range []string{"dify", "bailian", "coze"} {
		t.Run(p, func(t *testing.T) {
			c := testConfig(p)
			r := testRequest()
			r.Session = "previous-session"
			r.Streaming = false
			path, b := nativeRequest(c, r)
			v := gjson.ParseBytes(b)
			switch p {
			case "dify":
				if path != "/v1/chat-messages" || v.Get("query").String() != "hello" || v.Get("user").String() != "alice" || v.Get("response_mode").String() != "streaming" || v.Get("conversation_id").String() != r.Session {
					t.Fatal(path, string(b))
				}
			case "bailian":
				if path != "/api/v1/apps/agent-1/completion" || v.Get("input.prompt").String() != "hello" || !v.Get("parameters.incremental_output").Bool() || v.Get("input.session_id").String() != r.Session {
					t.Fatal(path, string(b))
				}
			case "coze":
				if path != "/v3/chat?conversation_id=previous-session" || v.Get("conversation_id").Exists() || v.Get("additional_messages.0.content").String() != "hello" {
					t.Fatal(path, string(b))
				}
			}
		})
	}
}
func TestRequestValidation(t *testing.T) {
	c := testConfig("dify")
	if _, _, err := parseRequest(input("SendMessage"), c, "alice"); err != nil {
		t.Fatal(err)
	}
	cases := []string{`{`, `[]`, strings.Replace(string(input("SendMessage")), `"req-1"`, `null`, 1), strings.Replace(string(input("SendMessage")), `"req-1"`, `{}`, 1), string(input("GetTask")), strings.Replace(string(input("SendMessage")), `"ROLE_USER"`, `"ROLE_AGENT"`, 1), strings.Replace(string(input("SendMessage")), `{"text":"hello"}`, `{"file":{"uri":"http://169.254.169.254"}}`, 1), strings.Replace(string(input("SendMessage")), `{"text":"hello"}`, `{"text":"hello","data":{}}`, 1), strings.Replace(string(input("SendMessage")), `"messageId":"m1"`, `"taskId":"arbitrary","messageId":"m1"`, 1), strings.Replace(string(input("SendMessage")), `"messageId":"m1"`, `"contextId":"arbitrary","messageId":"m1"`, 1)}
	for _, s := range cases {
		if _, _, err := parseRequest([]byte(s), c, "alice"); err == nil {
			t.Errorf("accepted %s", s)
		}
	}
	if _, _, err := parseRequest(input("SendMessage"), c, ""); err == nil {
		t.Fatal("missing identity accepted")
	}
	if _, _, err := parseRequest(input("SendMessage"), testConfig("qoder"), "alice"); err == nil {
		t.Fatal("blocking cloud accepted")
	}
}
func TestContextIsolation(t *testing.T) {
	c := testConfig("dify")
	token := c.signContext("session-1", "alice")
	if s, e := c.verifyContext(token, "alice"); e != nil || s != "session-1" {
		t.Fatal(s, e)
	}
	if _, e := c.verifyContext(token, "bob"); e == nil {
		t.Fatal("cross consumer accepted")
	}
	other := c
	other.Provider = "bailian"
	if _, e := other.verifyContext(token, "alice"); e == nil {
		t.Fatal("cross provider accepted")
	}
	other = c
	other.APIPath = "/another"
	if _, e := other.verifyContext(token, "alice"); e == nil {
		t.Fatal("cross endpoint accepted")
	}
	if _, e := c.verifyContext(token+"x", "alice"); e == nil {
		t.Fatal("tampering accepted")
	}
	c.ContextTTLSeconds = -2
	token = c.signContext("s", "alice")
	if _, e := c.verifyContext(token, "alice"); e == nil {
		t.Fatal("expiry accepted")
	}
}
func TestCardV1(t *testing.T) {
	c := testConfig("dify")
	b, _ := json.Marshal(c.card())
	v := gjson.ParseBytes(b)
	if v.Get("supportedInterfaces.0.protocolVersion").String() != "1.0" || v.Get("supportedInterfaces.0.protocolBinding").String() != "JSONRPC" || v.Get("url").Exists() || !v.Get("capabilities.streaming").Bool() {
		t.Fatal(string(b))
	}
}

func TestCloudContextContinuationRejected(t *testing.T) {
	c := testConfig("qoder")
	token := c.signContext("session1", "alice")
	b := strings.Replace(string(input("SendStreamingMessage")), `"messageId":"m1"`, `"messageId":"m1","contextId":"`+token+`"`, 1)
	if _, _, err := parseRequest([]byte(b), c, "alice"); err == nil {
		t.Fatal("cloud continuation accepted")
	}
}
func TestConfigurationReturnImmediately(t *testing.T) {
	c := testConfig("dify")
	base := string(input("SendMessage"))
	for _, value := range []string{"true", "1", "null"} {
		b := strings.Replace(base, `"params":{`, `"params":{"configuration":{"returnImmediately":`+value+`},`, 1)
		if _, _, err := parseRequest([]byte(b), c, "alice"); err == nil {
			t.Fatal("accepted", value)
		}
	}
	b := strings.Replace(base, `"params":{`, `"params":{"configuration":{"returnImmediately":false},`, 1)
	if _, _, err := parseRequest([]byte(b), c, "alice"); err != nil {
		t.Fatal(err)
	}
}
