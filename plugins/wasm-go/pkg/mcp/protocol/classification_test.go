// Copyright (c) 2022 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package protocol

import (
	"bytes"
	"encoding/json"
	"testing"
)

func headerlessModernMediaTransport() Transport {
	return Transport{
		Method:      "POST",
		Authority:   "mcp.example.com",
		ContentType: "application/json",
		Accept:      "application/json, text/event-stream",
	}
}

func TestStructuredModernMetadataLexerPathsAndEscapes(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bodyClassification
	}{
		{
			name: "escaped relevant keys",
			body: `{"p\u0061rams":{"_m\u0065ta":{"io.modelcontextprotocol\/protocol\u0056ersion":"2026-07-28"}}}`,
			want: bodyClassificationModern,
		},
		{
			name: "modern marker followed by trailing value",
			body: modernListRequest + `{}`,
			want: bodyClassificationModern,
		},
		{
			name: "complete marker key before unterminated value",
			body: `{"params":{"_meta":{"io.modelcontextprotocol/protocolVersion":`,
			want: bodyClassificationModern,
		},
		{
			name: "marker in string literal",
			body: `{"params":{"note":"io.modelcontextprotocol/protocolVersion"}}`,
			want: bodyClassificationLegacy,
		},
		{
			name: "nested meta is not direct",
			body: `{"params":{"nested":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}}}}`,
			want: bodyClassificationLegacy,
		},
		{
			name: "meta inside params array is not direct",
			body: `{"params":[{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}}]}`,
			want: bodyClassificationLegacy,
		},
		{
			name: "marker in a trailing root value is not direct",
			body: `{}` + `{"params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}}}`,
			want: bodyClassificationInvalid,
		},
		{
			name: "unterminated literal before marker never recognizes identity",
			body: `{"params":{"note":"io.modelcontextprotocol/protocolVersion`,
			want: bodyClassificationInvalid,
		},
		{
			name: "valid legacy array",
			body: `[{"jsonrpc":"2.0","method":"ping"}]`,
			want: bodyClassificationLegacy,
		},
		{
			name: "valid legacy scalar",
			body: `1.25e2`,
			want: bodyClassificationLegacy,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := classifyRequestBody([]byte(test.body)); got != test.want {
				t.Fatalf("classifyRequestBody() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestHeaderlessModernIdentityFailsClosed(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"trailing JSON value", modernListRequest + `{}`},
		{"escaped relevant keys", `{"jsonrpc":"2.0","id":1,"method":"tools/list","p\u0061rams":{"_m\u0065ta":{"io.modelcontextprotocol\/protocol\u0056ersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}`},
		{"unterminated after marker", `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request, protocolError := PrepareRequest(headerlessModernMediaTransport(), []byte(test.body), func(string) bool {
				t.Fatal("headerless modern identity must not reach any business method lookup")
				return true
			})
			if request != nil || protocolError == nil || protocolError.Code != CodeHeaderMismatch {
				t.Fatalf("request=%+v error=%+v, want modern header mismatch", request, protocolError)
			}
		})
	}
}

func TestInvalidHeaderlessJSONIsRejectedBeforeDispatch(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "invalid literal before modern marker",
			body: `{"jsonrpc":"2.0","id":1,"method":"tools/list","junk":truX,"params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}}}`,
		},
		{
			name: "invalid string before modern marker notification",
			body: `{"jsonrpc":"2.0","method":"tools/list","junk":"\x","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}}}`,
		},
		{
			name: "invalid container before modern marker",
			body: `{"jsonrpc":"2.0","id":1,"method":"tools/list","junk":[},"params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}}}`,
		},
		{
			name: "invalid number before modern marker",
			body: `{"jsonrpc":"2.0","id":1,"method":"tools/list","junk":01,"params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}}}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request, protocolError := PrepareRequest(headerlessModernMediaTransport(), []byte(test.body), func(string) bool {
				t.Fatal("invalid headerless JSON must not reach any business method lookup")
				return true
			})
			if request != nil || protocolError == nil || protocolError.HTTPStatus != 400 || protocolError.Code != CodeParseError {
				t.Fatalf("request=%+v error=%+v, want HTTP 400 parse error", request, protocolError)
			}
			response := MarshalErrorResponse(ID{}, protocolError)
			var envelope struct {
				ID json.RawMessage `json:"id"`
			}
			if err := json.Unmarshal(response, &envelope); err != nil {
				t.Fatalf("invalid protocol response: %v", err)
			}
			if string(envelope.ID) != "null" {
				t.Fatalf("invalid body response id = %s, want null", envelope.ID)
			}
		})
	}
}

func TestModernMarkerBeforeLexerErrorRemainsModern(t *testing.T) {
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}},"junk":truX}`)
	if got := classifyRequestBody(body); got != bodyClassificationModern {
		t.Fatalf("classifyRequestBody() = %d, want modern identity before later lexer error", got)
	}
	request, protocolError := PrepareRequest(headerlessModernMediaTransport(), body, func(string) bool {
		t.Fatal("headerless modern identity must fail before business method lookup")
		return true
	})
	if request != nil || protocolError == nil || protocolError.Code != CodeHeaderMismatch {
		t.Fatalf("request=%+v error=%+v, want modern header mismatch", request, protocolError)
	}
}

func TestDeepLateModernIdentityHasNoDecoderDepthDowngrade(t *testing.T) {
	const depth = 10001
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"irrelevant":`)
	body = append(body, bytes.Repeat([]byte{'['}, depth)...)
	body = append(body, '0')
	body = append(body, bytes.Repeat([]byte{']'}, depth)...)
	body = append(body, []byte(`,"padding":"`)...)
	body = append(body, bytes.Repeat([]byte{'x'}, int(ModernMaxBodyBytes))...)
	body = append(body, []byte(`","_meta":{"`+MetaProtocolVersion+`":"2026-07-28"}}}`)...)

	if got := classifyRequestBody(body); got != bodyClassificationModern {
		t.Fatal("late direct modern identity was lost after deeply nested irrelevant value")
	}
	request, protocolError := PrepareRequest(headerlessModernMediaTransport(), body, func(string) bool {
		t.Fatal("oversized deep modern request must fail before business method lookup")
		return true
	})
	if request != nil || protocolError == nil || protocolError.HTTPStatus != 413 || protocolError.Code != CodeInvalidRequest {
		t.Fatalf("request=%+v error=%+v, want HTTP 413 modern failure", request, protocolError)
	}
}

func TestDeepLegacyBodyRemainsLegacy(t *testing.T) {
	const depth = 10001
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"irrelevant":`)
	body = append(body, bytes.Repeat([]byte{'['}, depth)...)
	body = append(body, '0')
	body = append(body, bytes.Repeat([]byte{']'}, depth)...)
	body = append(body, []byte(`}}`)...)

	if got := classifyRequestBody(body); got != bodyClassificationLegacy {
		t.Fatalf("classifyRequestBody() = %d, want deeply nested legal legacy", got)
	}
	request, protocolError := PrepareRequest(headerlessModernMediaTransport(), body, func(string) bool {
		t.Fatal("deep legacy body must not enter modern business method lookup")
		return true
	})
	if protocolError != nil || request == nil || request.Era != EraLegacy {
		t.Fatalf("request=%+v error=%+v, want legacy passthrough", request, protocolError)
	}
}

func TestLargeLegacyStringScanHasBoundedAllocations(t *testing.T) {
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"note":"` + MetaProtocolVersion + `"},"padding":"`)
	body = append(body, bytes.Repeat([]byte{'x'}, 4*1024*1024)...)
	body = append(body, []byte(`"}`)...)

	if got := classifyRequestBody(body); got != bodyClassificationLegacy {
		t.Fatal("metadata marker string was classified as modern identity")
	}
	allocations := testing.AllocsPerRun(3, func() {
		if got := classifyRequestBody(body); got != bodyClassificationLegacy {
			t.Fatal("metadata marker string was classified as modern identity")
		}
	})
	if allocations > 2 {
		t.Fatalf("raw legacy scan allocations = %.0f, want at most 2 independent of value size", allocations)
	}
}
