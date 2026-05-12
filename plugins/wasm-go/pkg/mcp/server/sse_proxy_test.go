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
	"testing"
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
