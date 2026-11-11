// Copyright (c) 2026 Alibaba Group Holding Ltd.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy at http://www.apache.org/licenses/LICENSE-2.0
package protocol

import (
	"bytes"
	"strings"
	"testing"
)

// TestLegacyResponseAckAccepts 证明收件确认保留原始 ID/报文，且不查找业务方法。
func TestLegacyResponseAckAccepts(t *testing.T) {
	deep := strings.Repeat(`{"nested":`, 80) + `{"same":1,"same":2}` + strings.Repeat(`}`, 80)
	for _, version := range append([]Version{""}, LegacyVersions()...) {
		for _, token := range []string{`"ping\\id"`, `9007199254740993`, `9223372036854775808`, `1.50`, `1E+03`, `null`} {
			for _, field := range []string{`"result":{}`, `"result":null`, `"result":[1,"x"]`, `"result":` + deep, `"result":{"large":"` + strings.Repeat("x", 1<<20) + `"}`, `"error":{"code":-32603,"message":"synthetic","data":` + deep + `}`, `"error":{"code":92233720368547758080,"message":""}`} {
				t.Run(string(version)+"/"+token+"/"+field[:min(22, len(field))], func(t *testing.T) {
					body := []byte(`{"jsonrpc":"2.0","id":` + token + `,` + field + `,"extension":1,"extension":2}`)
					original := bytes.Clone(body)
					request, err := PrepareRequest(Transport{ProtocolVersion: string(version)}, body, func(string) bool { t.Fatal("response looked up a method"); return false })
					if err != nil {
						t.Fatalf("response rejected: %+v", err)
					}
					if request.Era != EraLegacy || request.Envelope.Method != "" || request.Envelope.Notification || !request.Envelope.ID.IsPresent() {
						t.Fatalf("not a receipt: %+v", request)
					}
					if string(request.Envelope.ID.Raw()) != token || !bytes.Equal(request.Envelope.Raw, original) {
						t.Fatal("raw token/body changed")
					}
					body[0] = '['
					if !bytes.Equal(request.Envelope.Raw, original) {
						t.Fatal("envelope retained caller buffer")
					}
				})
			}
		}
	}
}

// TestLegacyResponseAckRejects 验证畸形或冲突响应不获得确认，也不触发业务分派。
func TestLegacyResponseAckRejects(t *testing.T) {
	cases := []string{
		`{"jsonrpc":"2.0","result":{}}`, `{"jsonrpc":"2.0","id":true,"result":{}}`, `{"jsonrpc":"2.0","id":{},"result":{}}`, `{"jsonrpc":"2.0","id":[],"result":{}}`,
		`{"id":1,"result":{}}`, `{"jsonrpc":2,"id":1,"result":{}}`, `{"jsonrpc":"1.0","id":1,"result":{}}`, `{"jsonrpc":"2.0","id":1}`,
		`{"jsonrpc":"2.0","id":1,"result":null,"error":null}`, `{"jsonrpc":"2.0","id":1,"method":"","result":{}}`, `{"jsonrpc":"2.0","id":1,"method":null,"result":{}}`, `{"jsonrpc":"2.0","id":1,"method":"tools/call","result":{}}`,
		`{"jsonrpc":"2.0","id":1,"error":null}`, `{"jsonrpc":"2.0","id":1,"error":[]}`, `{"jsonrpc":"2.0","id":1,"error":{}}`, `{"jsonrpc":"2.0","id":1,"error":{"code":1.0,"message":"x"}}`, `{"jsonrpc":"2.0","id":1,"error":{"code":1e3,"message":"x"}}`, `{"jsonrpc":"2.0","id":1,"error":{"code":"1","message":"x"}}`, `{"jsonrpc":"2.0","id":1,"error":{"code":1,"message":null}}`, `{"jsonrpc":"2.0","id":1,"error":{"code":1,"message":2}}`,
		`[]`, `null`, `{"jsonrpc":"2.0","id":1,"result":{}}{}`, `{"jsonrpc":"2.0","id":1,"result":`, "{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":\"\xff\"}",
	}
	for _, field := range []string{`"jsonrpc":"2.0"`, `"id":1`, `"method":null`, `"result":{}`, `"error":null`} {
		cases = append(cases, `{"jsonrpc":"2.0","id":1,"result":{},`+field+`}`)
	}
	for _, field := range []string{`"code":1`, `"message":"x"`} {
		cases = append(cases, `{"jsonrpc":"2.0","id":1,"error":{"code":1,"message":"x",`+field+`}}`)
	}
	for _, body := range cases {
		t.Run(body, func(t *testing.T) {
			_, err := PrepareRequest(Transport{}, []byte(body), func(string) bool { t.Fatal("invalid response dispatched"); return false })
			if err == nil {
				t.Fatal("invalid response acknowledged")
			}
		})
	}
}

// TestLegacyResponseAckModernBoundary 保证头部或保留元数据选择的 Modern 响应仍被拒绝。
func TestLegacyResponseAckModernBoundary(t *testing.T) {
	for _, body := range []string{`{"jsonrpc":"2.0","id":1,"result":{}}`, `{"jsonrpc":"2.0","id":1,"error":{"code":-1,"message":"x"}}`} {
		for _, transport := range []Transport{modernTransport("tools/list", ""), modernTransportFromHeaders("tools/list", "")} {
			_, err := PrepareRequest(transport, []byte(body), available)
			if err == nil || err.Message != "JSON-RPC response envelopes are not accepted" {
				t.Fatalf("modern response changed rejection: %+v", err)
			}
		}
		marked := strings.TrimSuffix(body, "}") + `,"params":{"_meta":{"` + MetaProtocolVersion + `":"2026-07-28"}}}`
		_, err := PrepareRequest(Transport{}, []byte(marked), available)
		if err == nil {
			t.Fatal("body identity downgraded")
		}
	}
}

// TestLegacyResponseAckOrdinaryRequestDuplicates 保留普通 Legacy 请求的首次出现语义。
func TestLegacyResponseAckOrdinaryRequestDuplicates(t *testing.T) {
	request, err := PrepareRequest(Transport{}, []byte(`{"jsonrpc":"2.0","id":1,"id":2,"method":"ping","method":"tools/list"}`), nil)
	if err != nil || request.Envelope.Method != "ping" || string(request.Envelope.ID.Raw()) != "1" {
		t.Fatalf("ordinary request semantics changed: %+v %+v", request, err)
	}
}
