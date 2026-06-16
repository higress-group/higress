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

package server

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseSSEMessage tests SSE message parsing
func TestParseSSEMessage(t *testing.T) {
	tests := []struct {
		name        string
		input       []byte
		wantEvent   string
		wantData    string
		wantID      string
		shouldParse bool
	}{
		{
			name: "endpoint message",
			input: []byte(`event: endpoint
data: /messages/?session_id=test123

`),
			wantEvent:   "endpoint",
			wantData:    "/messages/?session_id=test123",
			shouldParse: true,
		},
		{
			name: "message with JSON data",
			input: []byte(`event: message
data: {"jsonrpc":"2.0","id":1,"result":{"test":"value"}}

`),
			wantEvent:   "message",
			wantData:    `{"jsonrpc":"2.0","id":1,"result":{"test":"value"}}`,
			shouldParse: true,
		},
		{
			name: "incomplete message",
			input: []byte(`event: message
data: {"jsonrpc":"2.0"`),
			shouldParse: false,
		},
		{
			name: "message with id",
			input: []byte(`id: 123
event: message
data: test data

`),
			wantEvent:   "message",
			wantData:    "test data",
			wantID:      "123",
			shouldParse: true,
		},
		{
			name: "comment line ignored",
			input: []byte(`: this is a comment
event: message
data: test data

`),
			wantEvent:   "message",
			wantData:    "test data",
			shouldParse: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, remaining, err := ParseSSEMessage(tt.input)

			if err != nil {
				t.Fatalf("parseSSEMessage() error = %v", err)
			}

			if tt.shouldParse {
				if msg == nil {
					t.Errorf("parseSSEMessage() expected message but got nil")
					return
				}
				if msg.Event != tt.wantEvent {
					t.Errorf("parseSSEMessage() Event = %v, want %v", msg.Event, tt.wantEvent)
				}
				if msg.Data != tt.wantData {
					t.Errorf("parseSSEMessage() Data = %v, want %v", msg.Data, tt.wantData)
				}
				if msg.ID != tt.wantID {
					t.Errorf("parseSSEMessage() ID = %v, want %v", msg.ID, tt.wantID)
				}
				if len(remaining) != 0 {
					t.Errorf("parseSSEMessage() expected no remaining bytes, got %d bytes", len(remaining))
				}
			} else {
				if msg != nil {
					t.Errorf("parseSSEMessage() expected no message but got %v", msg)
				}
				if len(remaining) != len(tt.input) {
					t.Errorf("parseSSEMessage() expected all data as remaining, got %d bytes instead of %d", len(remaining), len(tt.input))
				}
			}
		})
	}
}

// TestExtractEndpointURL tests endpoint URL extraction
func TestExtractEndpointURL(t *testing.T) {
	tests := []struct {
		name         string
		endpointData string
		baseURL      string
		want         string
		wantErr      bool
	}{
		{
			name:         "full URL",
			endpointData: "http://example.com/messages?session=123",
			baseURL:      "http://backend.com/mcp",
			want:         "http://example.com/messages?session=123",
			wantErr:      false,
		},
		{
			name:         "path only",
			endpointData: "/messages/?session_id=abc",
			baseURL:      "http://backend.com/mcp",
			want:         "http://backend.com/messages/?session_id=abc",
			wantErr:      false,
		},
		{
			name:         "https base URL",
			endpointData: "/sse/endpoint",
			baseURL:      "https://secure.backend.com:8443/api",
			want:         "https://secure.backend.com:8443/sse/endpoint",
			wantErr:      false,
		},
		{
			name:         "path-only base URL",
			endpointData: "/messages",
			baseURL:      "/api/v1",
			want:         "/messages",
			wantErr:      false,
		},
		{
			name:         "path without leading slash",
			endpointData: "api/v1/messages",
			baseURL:      "http://backend.com",
			want:         "http://backend.com/api/v1/messages",
			wantErr:      false,
		},
		{
			name:         "path without leading slash with port",
			endpointData: "sse/endpoint",
			baseURL:      "https://secure.backend.com:8443",
			want:         "https://secure.backend.com:8443/sse/endpoint",
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractEndpointURL(tt.endpointData, tt.baseURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractEndpointURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("extractEndpointURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestTransportProtocolValidation tests transport protocol validation
func TestTransportProtocolValidation(t *testing.T) {
	tests := []struct {
		name      string
		transport string
		wantValid bool
	}{
		{
			name:      "valid http transport",
			transport: "http",
			wantValid: true,
		},
		{
			name:      "valid sse transport",
			transport: "sse",
			wantValid: true,
		},
		{
			name:      "invalid transport",
			transport: "websocket",
			wantValid: false,
		},
		{
			name:      "empty transport",
			transport: "",
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := TransportProtocol(tt.transport)
			isValid := transport == TransportHTTP || transport == TransportSSE
			if isValid != tt.wantValid {
				t.Errorf("TransportProtocol validation = %v, want %v for %s", isValid, tt.wantValid, tt.transport)
			}
		})
	}
}

// TestMcpProxyServerTransport tests transport getter/setter
func TestMcpProxyServerTransport(t *testing.T) {
	server := NewMcpProxyServer("test-server")

	// Test default transport
	if server.GetTransport() != "" {
		t.Errorf("Expected empty default transport, got %v", server.GetTransport())
	}

	// Test setting HTTP transport
	server.SetTransport(TransportHTTP)
	if server.GetTransport() != TransportHTTP {
		t.Errorf("Expected HTTP transport, got %v", server.GetTransport())
	}

	// Test setting SSE transport
	server.SetTransport(TransportSSE)
	if server.GetTransport() != TransportSSE {
		t.Errorf("Expected SSE transport, got %v", server.GetTransport())
	}
}

// TestSSEMessageParsing_MultipleMessages tests parsing multiple SSE messages
func TestSSEMessageParsing_MultipleMessages(t *testing.T) {
	data := []byte(`event: endpoint
data: /messages/123

event: message
data: {"id":1}

: comment line
event: message
data: {"id":2}

`)

	// First message
	msg1, remaining, err := ParseSSEMessage(data)
	if err != nil {
		t.Fatalf("Failed to parse first message: %v", err)
	}
	if msg1 == nil || msg1.Event != "endpoint" || msg1.Data != "/messages/123" {
		t.Errorf("First message incorrect: %+v", msg1)
	}

	// Second message
	msg2, remaining, err := ParseSSEMessage(remaining)
	if err != nil {
		t.Fatalf("Failed to parse second message: %v", err)
	}
	if msg2 == nil || msg2.Event != "message" || msg2.Data != `{"id":1}` {
		t.Errorf("Second message incorrect: %+v", msg2)
	}

	// Third message
	msg3, remaining, err := ParseSSEMessage(remaining)
	if err != nil {
		t.Fatalf("Failed to parse third message: %v", err)
	}
	if msg3 == nil || msg3.Event != "message" || msg3.Data != `{"id":2}` {
		t.Errorf("Third message incorrect: %+v", msg3)
	}

	// Should be no more complete messages
	msg4, _, err := ParseSSEMessage(remaining)
	if err != nil {
		t.Fatalf("Error parsing remaining data: %v", err)
	}
	if msg4 != nil {
		t.Errorf("Expected no more messages, got: %+v", msg4)
	}
}

// -----------------------------------------------------------------------------
// ParseSSEMessage — additional edge cases (multi-line data, retry, empty)
// -----------------------------------------------------------------------------

func TestParseSSEMessage_EmptyInput(t *testing.T) {
	msg, remaining, err := ParseSSEMessage([]byte(""))
	require.NoError(t, err)
	assert.Nil(t, msg)
	assert.Len(t, remaining, 0)
}

func TestParseSSEMessage_RetryFieldIgnored(t *testing.T) {
	// `retry:` is part of the SSE spec but not implemented — must not break parsing.
	input := []byte("retry: 5000\nevent: message\ndata: hi\n\n")
	msg, _, err := ParseSSEMessage(input)
	require.NoError(t, err)
	require.NotNil(t, msg)
	assert.Equal(t, "message", msg.Event)
	assert.Equal(t, "hi", msg.Data)
}

func TestParseSSEMessage_MultiLineDataConcatenated(t *testing.T) {
	// Per SSE spec, multiple `data:` lines in one message join with `\n`.
	input := []byte("data: line-one\ndata: line-two\ndata: line-three\n\n")
	msg, _, err := ParseSSEMessage(input)
	require.NoError(t, err)
	require.NotNil(t, msg)
	assert.Equal(t, "line-one\nline-two\nline-three", msg.Data)
}

func TestParseSSEMessage_NoFinalBlankLine_NoMessageReturned(t *testing.T) {
	// Message without the terminating blank line is treated as incomplete.
	input := []byte("event: message\ndata: payload\n")
	msg, remaining, err := ParseSSEMessage(input)
	require.NoError(t, err)
	assert.Nil(t, msg, "incomplete message must not be returned")
	assert.Equal(t, input, remaining, "remaining is the entire input")
}

func TestParseSSEMessage_LineWithoutColonSkipped(t *testing.T) {
	// SplitN with len<2 → field/value pair can't be formed → skipped, not an error.
	input := []byte("a-line-without-colon\nevent: msg\ndata: x\n\n")
	msg, _, err := ParseSSEMessage(input)
	require.NoError(t, err)
	require.NotNil(t, msg)
	assert.Equal(t, "msg", msg.Event)
	assert.Equal(t, "x", msg.Data)
}

func TestParseSSEMessage_UnknownFieldIgnored(t *testing.T) {
	// `random-field:` is parsed but the switch case ignores it.
	input := []byte("random-field: stuff\nevent: msg\ndata: x\n\n")
	msg, _, err := ParseSSEMessage(input)
	require.NoError(t, err)
	require.NotNil(t, msg)
	assert.Equal(t, "msg", msg.Event)
}

// -----------------------------------------------------------------------------
// ExtractEndpointURL — edge cases not in the table
// -----------------------------------------------------------------------------

func TestExtractEndpointURL_HttpsPassthrough(t *testing.T) {
	got, err := ExtractEndpointURL("https://other.example/x", "http://b.example")
	require.NoError(t, err)
	assert.Equal(t, "https://other.example/x", got, "full https URL must pass through unchanged")
}

func TestExtractEndpointURL_EmptyEndpointData_PathOnlyBase(t *testing.T) {
	got, err := ExtractEndpointURL("", "/some/path")
	require.NoError(t, err)
	assert.Equal(t, "", got, "empty endpointData with path-only base → empty result")
}

func TestExtractEndpointURL_RelativeEndpointWithSchemeBase(t *testing.T) {
	got, err := ExtractEndpointURL("messages", "http://b.example/mcp")
	require.NoError(t, err)
	assert.Equal(t, "http://b.example/messages", got, "leading slash auto-inserted")
}

// -----------------------------------------------------------------------------
// applyProxyAuthenticationForSSE — pure URL+header munging (no proxywasm)
// -----------------------------------------------------------------------------

func TestApplyProxyAuthenticationForSSE_ApiKeyHeader(t *testing.T) {
	server := NewMcpProxyServer("p")
	server.AddSecurityScheme(SecurityScheme{
		ID: "K", Type: "apiKey", In: "header", Name: "X-Api-Key",
		DefaultCredential: "abc",
	})

	headers := [][2]string{{"X-Other", "v"}}
	got, err := applyProxyAuthenticationForSSE(server, "K", "", &headers, "http://backend/x")
	require.NoError(t, err)
	assert.Equal(t, "http://backend/x", got, "no query → URL preserved")

	found := false
	for _, kv := range headers {
		if strings.EqualFold(kv[0], "X-Api-Key") {
			assert.Equal(t, "abc", kv[1])
			found = true
		}
	}
	assert.True(t, found, "API key header must be injected")
}

func TestApplyProxyAuthenticationForSSE_ApiKeyQuery_PreservesExisting(t *testing.T) {
	server := NewMcpProxyServer("p")
	server.AddSecurityScheme(SecurityScheme{
		ID: "K", Type: "apiKey", In: "query", Name: "api_key",
		DefaultCredential: "secret",
	})

	headers := [][2]string{}
	got, err := applyProxyAuthenticationForSSE(server, "K", "", &headers, "http://backend/x?existing=1")
	require.NoError(t, err)
	// Query is rebuilt via url.Values.Encode — both pairs must be present.
	assert.Contains(t, got, "api_key=secret")
	assert.Contains(t, got, "existing=1")
}

func TestApplyProxyAuthenticationForSSE_PathOnlyURL_PreservesShape(t *testing.T) {
	server := NewMcpProxyServer("p")
	server.AddSecurityScheme(SecurityScheme{
		ID: "K", Type: "apiKey", In: "header", Name: "X-Api-Key",
		DefaultCredential: "abc",
	})

	headers := [][2]string{}
	got, err := applyProxyAuthenticationForSSE(server, "K", "", &headers, "/relative/path")
	require.NoError(t, err)
	assert.Equal(t, "/relative/path", got, "path-only URL must come back as path-only")
}

func TestApplyProxyAuthenticationForSSE_HttpBearerPassthrough(t *testing.T) {
	server := NewMcpProxyServer("p")
	server.AddSecurityScheme(SecurityScheme{ID: "B", Type: "http", Scheme: "bearer"})

	headers := [][2]string{}
	got, err := applyProxyAuthenticationForSSE(server, "B", "passthrough-token", &headers, "http://backend/x")
	require.NoError(t, err)
	assert.Equal(t, "http://backend/x", got)

	var authValue string
	for _, kv := range headers {
		if strings.EqualFold(kv[0], "Authorization") {
			authValue = kv[1]
		}
	}
	assert.Equal(t, "Bearer passthrough-token", authValue)
}

func TestApplyProxyAuthenticationForSSE_MissingScheme_ReturnsError(t *testing.T) {
	server := NewMcpProxyServer("p")
	headers := [][2]string{}
	_, err := applyProxyAuthenticationForSSE(server, "missing", "", &headers, "http://backend/x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestApplyProxyAuthenticationForSSE_PreservesFragment(t *testing.T) {
	server := NewMcpProxyServer("p")
	server.AddSecurityScheme(SecurityScheme{
		ID: "K", Type: "apiKey", In: "header", Name: "X-Api-Key",
		DefaultCredential: "abc",
	})

	headers := [][2]string{}
	got, err := applyProxyAuthenticationForSSE(server, "K", "", &headers, "http://backend/path#section-2")
	require.NoError(t, err)
	assert.Contains(t, got, "#section-2", "fragment must round-trip")
}
