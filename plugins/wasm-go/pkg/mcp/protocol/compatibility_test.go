// Copyright (c) 2026 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package protocol

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestSharedCompleteBodyLimit(t *testing.T) {
	if ModernMaxBodyBytes != LegacyMaxBodyBytes || MaxBodyBytes != 100*1024*1024 {
		t.Fatal("both profiles must use the same 100 MiB full-body limit")
	}
	for _, modern := range []bool{false, true} {
		transport := Transport{Method: "PUT"}
		requestBody := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`
		if modern {
			transport = modernTransport("tools/list", "")
			requestBody = modernListRequest
		}
		// Whitespace is part of the complete body, outside params/arguments.
		body := bytes.Repeat([]byte{' '}, int(MaxBodyBytes)+1)
		copy(body, requestBody)
		request, protocolError := PrepareRequest(transport, body[:MaxBodyBytes], available)
		if protocolError != nil || request == nil {
			t.Fatalf("modern=%t: exact full-body limit rejected: %v", modern, protocolError)
		}
		lookups := 0
		_, protocolError = PrepareRequest(transport, body, func(string) bool { lookups++; return true })
		if protocolError == nil || protocolError.HTTPStatus != 413 || lookups != 0 {
			t.Fatalf("modern=%t: oversized body error=%v lookups=%d", modern, protocolError, lookups)
		}
	}
}

func TestBusinessFieldsKeepHistoricalFirstDuplicate(t *testing.T) {
	for _, modern := range []bool{false, true} {
		transport := Transport{Method: "POST"}
		meta := `"vendor":{"first":1},"vendor":{"second":2}`
		if modern {
			transport = modernTransport("tools/call", "echo")
			meta += `,"` + MetaProtocolVersion + `":"2026-07-28"`
		}
		body := []byte(`{"jsonrpc":"2.0","id":1.50,"id":99,"method":"tools/call","params":{"name":"echo","arguments":{"n":9007199254740993,"n":2,"nested":{"k":1,"k":2}},"arguments":{"other":true},"_meta":{` + meta + `}},"params":{"ordinary":"ignored"}}`)
		request, protocolError := PrepareRequest(transport, body, available)
		if protocolError != nil {
			t.Fatalf("modern=%t: ordinary duplicate rejected: %v", modern, protocolError)
		}
		if got := string(request.Envelope.ID.Raw()); got != "1.50" {
			t.Fatalf("modern=%t: first client ID=%s", modern, got)
		}
		params, err := DecodeBusinessJSONObject(request.Envelope.Params)
		if err != nil || string(params["arguments"]) != `{"n":9007199254740993,"n":2,"nested":{"k":1,"k":2}}` {
			t.Fatalf("modern=%t: original arguments changed", modern)
		}
		args, err := DecodeBusinessJSONObject(params["arguments"])
		if err != nil || string(args["n"]) != "9007199254740993" {
			t.Fatalf("modern=%t: first argument or exact number changed", modern)
		}
	}
}

func TestModernIdentityCannotHideInDuplicateCarriers(t *testing.T) {
	cases := []string{
		strings.Replace(modernListRequest, `"method":"tools/list"`, `"method":"tools/list","method":"tools/call"`, 1),
		strings.Replace(modernListRequest, `"2026-07-28"`, `"2026-07-28","`+MetaProtocolVersion+`":"2025-06-18"`, 1),
		strings.TrimSuffix(strings.TrimSpace(modernListRequest), "}") + `,"params":{"_meta":{"` + MetaProtocolVersion + `":"2025-06-18"}}}`,
		strings.Replace(modernListRequest, `"_meta":{`, `"_meta":{"`+MetaProtocolVersion+`":"2025-06-18"},"_meta":{`, 1),
	}
	for i, body := range cases {
		lookups := 0
		request, protocolError := PrepareRequest(modernTransport("tools/list", ""), []byte(body), func(string) bool { lookups++; return true })
		if protocolError == nil || lookups != 0 || request == nil || string(request.Envelope.ID.Raw()) != `"request-1"` {
			t.Fatalf("case %d: identity ambiguity error=%v lookups=%d", i, protocolError, lookups)
		}
	}
	body := strings.Replace(modernCallRequest, `"name":"echo"`, `"name":"echo","name":"other"`, 1)
	if _, err := PrepareRequest(modernTransport("tools/call", "echo"), []byte(body), available); err == nil {
		t.Fatal("duplicate mirrored tool name must be rejected")
	}
}

func TestBusinessArgumentsAndNamesHaveNoPrivateBudgets(t *testing.T) {
	arguments := []string{
		`{"padding":"` + strings.Repeat("x", 1024*1024+1) + `"}`,
		`{"nested":` + strings.Repeat("[", 70) + `0` + strings.Repeat("]", 70) + `}`,
		`{"values":[` + strings.Repeat("0,", 65536) + `0]}`,
	}
	longName := strings.Repeat("工具", 2048)
	for _, arguments := range arguments {
		body := modernCallRequestWithName(longName)
		body = strings.Replace(body, `{"text":"hello"}`, arguments, 1)
		header := nameHeaderPrefix + base64.StdEncoding.EncodeToString([]byte(longName)) + nameHeaderSuffix
		request, protocolError := PrepareRequest(modernTransport("tools/call", header), []byte(body), available)
		if protocolError != nil || request == nil {
			t.Fatalf("historically accepted name/arguments rejected: %v", protocolError)
		}
	}
}

func TestClientIDTokensAndStrictUpstreamCorrelation(t *testing.T) {
	for _, token := range []string{`"request\\id"`, `0`, `9007199254740993`, `9223372036854775808`, `1.50`, `1E+03`, `-0.00e-2`, `null`, ``} {
		id, protocolError := ParseID(json.RawMessage(token))
		if protocolError != nil || id.IsPresent() != (token != "") || string(id.Raw()) != token {
			t.Fatalf("ID %q was not preserved: %v", token, protocolError)
		}
		correlation := id.CorrelationID("internal-correlation")
		// Strict peers accept string/int64 IDs, never null or fractional tokens.
		var text string
		var integer int64
		if len(correlation) == 0 || bytes.Equal(correlation, []byte("null")) ||
			(json.Unmarshal(correlation, &text) != nil && json.Unmarshal(correlation, &integer) != nil) {
			t.Fatalf("ID %q produced an invalid strict-upstream correlation", token)
		}
		if string(id.Raw()) != token || id.IsPresent() != (token != "") {
			t.Fatal("internal correlation changed client ID or notification semantics")
		}
	}
}

func TestStrictControlDecoderRemainsIndependent(t *testing.T) {
	for _, body := range []string{
		`{"result":{"n":1,"n":2}}`,
		`{"result":` + strings.Repeat("[", 70) + `0` + strings.Repeat("]", 70) + `}`,
	} {
		if _, err := DecodeBusinessJSONObject([]byte(body)); err != nil {
			t.Fatalf("business body rejected: %v", err)
		}
		if _, err := DecodeSingleJSONObject([]byte(body)); err == nil {
			t.Fatal("strict control decoder accepted a duplicate/deep response")
		}
	}
}

func TestRequestOwnsRawBodyAndID(t *testing.T) {
	body := []byte(modernCallRequest)
	request, protocolError := PrepareRequest(modernTransport("tools/call", "echo"), body, available)
	if protocolError != nil {
		t.Fatal(protocolError)
	}
	clear(body)
	id := request.Envelope.ID.Raw()
	clear(id)
	if !bytes.Equal(request.Envelope.Raw, []byte(modernCallRequest)) || string(request.Envelope.ID.Raw()) != "7" ||
		!bytes.Contains(request.Envelope.Params, []byte(`"arguments":{"text":"hello"}`)) {
		t.Fatal("caller mutation escaped into the request's owned input")
	}
}
