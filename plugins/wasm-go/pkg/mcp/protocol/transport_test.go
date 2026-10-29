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

import "testing"

func TestValidateModernTransportMappings(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*Transport)
		wantHTTP uint32
		wantCode int
	}{
		{"GET", func(v *Transport) { v.Method = "GET" }, 405, CodeInvalidRequest},
		{"DELETE", func(v *Transport) { v.Method = "DELETE" }, 405, CodeInvalidRequest},
		{"ambiguous identity header", func(v *Transport) { v.AmbiguousModernHeader = true }, 400, CodeHeaderMismatch},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := modernTransport("tools/list", "")
			test.mutate(&transport)
			protocolError := ValidateModernTransport(transport)
			if protocolError == nil || protocolError.HTTPStatus != test.wantHTTP || protocolError.Code != test.wantCode {
				t.Fatalf("error = %+v, want HTTP %d/code %d", protocolError, test.wantHTTP, test.wantCode)
			}
		})
	}
}

func TestModernTransportUsesLegacyMethodAndMediaPolicy(t *testing.T) {
	for _, method := range []string{"POST", "PUT", "PATCH", "OPTIONS"} {
		for _, media := range []string{"", "text/plain", "application/json"} {
			transport := modernTransport("tools/list", "")
			transport.Method, transport.ContentType, transport.Accept = method, media, "application/json;q=0"
			transport.AmbiguousHeader = true // Ordinary duplicate headers are not identity.
			if err := ValidateModernTransport(transport); err != nil {
				t.Fatalf("method %s media %q rejected: %v", method, media, err)
			}
		}
	}
}

func TestValidateModernTransportAcceptsHistoricalOriginForms(t *testing.T) {
	for _, origin := range []string{"", "https://mcp.example.com", "https://mcp.example.com:443", "https://other.example", "https://mcp.example.com:8443", "null", "invalid origin"} {
		transport := modernTransport("tools/list", "")
		transport.Origin = origin
		if protocolError := ValidateModernTransport(transport); protocolError != nil {
			t.Fatalf("Origin %q rejected: %+v", origin, protocolError)
		}
	}
}

func TestNewTransportRejectsDuplicateSensitiveHeaders(t *testing.T) {
	transport := NewTransport("POST", "mcp.example.com", [][2]string{
		{"Content-Type", "application/json"},
		{"Accept", "application/json, text/event-stream"},
		{"Mcp-Method", "tools/list"},
		{"mcp-method", "tools/list"},
	})
	if !transport.AmbiguousHeader {
		t.Fatal("duplicate case-insensitive identity header was not marked ambiguous")
	}
}

func TestNewTransportCombinesAcceptFieldLines(t *testing.T) {
	transport := NewTransport("POST", "mcp.example.com", [][2]string{
		{"Content-Type", "application/json"},
		{"Accept", "application/json"},
		{"accept", "text/event-stream"},
		{HeaderProtocolVersion, string(Version20260728)},
		{HeaderMethod, "tools/list"},
	})
	if transport.AmbiguousHeader {
		t.Fatal("repeated Accept field lines must not be marked ambiguous")
	}
	if transport.Accept != "application/json, text/event-stream" {
		t.Fatalf("combined Accept = %q", transport.Accept)
	}
	if protocolError := ValidateModernTransport(transport); protocolError != nil {
		t.Fatalf("combined JSON/SSE Accept rejected: %+v", protocolError)
	}

	disabledJSON := NewTransport("POST", "mcp.example.com", [][2]string{
		{"Content-Type", "application/json"},
		{"Accept", "application/json;q=0"},
		{"Accept", "text/event-stream"},
	})
	protocolError := ValidateModernTransport(disabledJSON)
	if protocolError != nil {
		t.Fatalf("ordinary Accept header rejected: %v", protocolError)
	}

	reenabledJSON := NewTransport("POST", "mcp.example.com", [][2]string{
		{"Content-Type", "application/json"},
		{"Accept", "application/json;q=0"},
		{"Accept", "text/event-stream"},
		{"Accept", "application/json;q=0.5"},
	})
	if protocolError := ValidateModernTransport(reenabledJSON); protocolError != nil {
		t.Fatalf("later positive q field line did not re-enable JSON: %+v", protocolError)
	}
}

func TestNewTransportRecordsDuplicateHeadersSeparatelyFromIdentity(t *testing.T) {
	tests := []struct {
		name   string
		header string
		modern bool
	}{
		{"content type", "Content-Type", false},
		{"origin", "Origin", false},
		{"protocol version", HeaderProtocolVersion, true},
		{"method", HeaderMethod, true},
		{"name", HeaderName, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := NewTransport("POST", "mcp.example.com", [][2]string{
				{test.header, "first"},
				{test.header, "second"},
			})
			if !transport.AmbiguousHeader {
				t.Fatal("duplicate singleton header was not marked ambiguous")
			}
			if transport.AmbiguousModernHeader != test.modern {
				t.Fatalf("AmbiguousModernHeader = %t, want %t", transport.AmbiguousModernHeader, test.modern)
			}
		})
	}
}

func TestHasModernIdentityHeaders(t *testing.T) {
	tests := []struct {
		name    string
		headers [][2]string
		want    bool
	}{
		{
			name:    "modern method without protocol header",
			headers: [][2]string{{HeaderMethod, "tools/list"}},
			want:    true,
		},
		{
			name:    "modern name without protocol header",
			headers: [][2]string{{HeaderName, "weather"}},
			want:    true,
		},
		{
			name:    "present but empty modern method",
			headers: [][2]string{{HeaderMethod, " "}},
			want:    true,
		},
		{
			name:    "empty protocol identity",
			headers: [][2]string{{HeaderProtocolVersion, " "}},
			want:    true,
		},
		{
			name: "duplicate modern identity",
			headers: [][2]string{
				{HeaderMethod, "tools/list"},
				{HeaderMethod, "tools/list"},
			},
			want: true,
		},
		{
			name:    "legacy protocol only",
			headers: [][2]string{{HeaderProtocolVersion, string(Version20250618)}},
			want:    false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := NewTransport("POST", "mcp.example.com", test.headers)
			if got := HasModernIdentityHeaders(transport); got != test.want {
				t.Fatalf("HasModernIdentityHeaders() = %v, want %v; transport = %+v", got, test.want, transport)
			}
		})
	}
}
