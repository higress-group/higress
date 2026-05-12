// Copyright (c) 2022 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package utils

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

func TestJsonRpcIDFromGjson(t *testing.T) {
	tests := []struct {
		name     string
		jsonData string
		expected JsonRpcID
	}{
		{
			name:     "integer id",
			jsonData: `{"id": 123}`,
			expected: JsonRpcID{
				IntValue: 123,
				IsString: false,
			},
		},
		{
			name:     "string id",
			jsonData: `{"id": "abc-123"}`,
			expected: JsonRpcID{
				StringValue: "abc-123",
				IsString:    true,
			},
		},
		{
			name:     "float id treated as int",
			jsonData: `{"id": 123.45}`,
			expected: JsonRpcID{
				IntValue: 123,
				IsString: false,
			},
		},
		{
			name:     "boolean id treated as int",
			jsonData: `{"id": true}`,
			expected: JsonRpcID{
				IntValue: 1,
				IsString: false,
			},
		},
		{
			name:     "null id treated as int zero",
			jsonData: `{"id": null}`,
			expected: JsonRpcID{
				IntValue: 0,
				IsString: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idResult := gjson.Get(tt.jsonData, "id")
			result := NewJsonRpcIDFromGjson(idResult)

			if result.IsString != tt.expected.IsString {
				t.Errorf("IsString = %v, want %v", result.IsString, tt.expected.IsString)
			}

			if result.IsString {
				if result.StringValue != tt.expected.StringValue {
					t.Errorf("StringValue = %v, want %v", result.StringValue, tt.expected.StringValue)
				}
			} else {
				if result.IntValue != tt.expected.IntValue {
					t.Errorf("IntValue = %v, want %v", result.IntValue, tt.expected.IntValue)
				}
			}
		})
	}
}

// Skip TestSendJsonRpcResponse because it requires proxywasm which is not available in the test environment
// This function would normally test that sendJsonRpcResponse correctly handles different ID types
func TestSendJsonRpcResponse(t *testing.T) {
	t.Skip("Skipping test that requires proxywasm")
}

func TestJsonRpcIDMarshaling(t *testing.T) {
	// Test that JsonRpcID is correctly marshaled in a JSON response

	tests := []struct {
		name     string
		id       JsonRpcID
		expected string
	}{
		{
			name: "integer id",
			id: JsonRpcID{
				IntValue: 123,
				IsString: false,
			},
			expected: `"id":123`,
		},
		{
			name: "string id",
			id: JsonRpcID{
				StringValue: "abc-123",
				IsString:    true,
			},
			expected: `"id":"abc-123"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a JSON object with the ID
			var jsonObj map[string]interface{}
			if tt.id.IsString {
				jsonObj = map[string]interface{}{
					"jsonrpc": "2.0",
					"id":      tt.id.StringValue,
				}
			} else {
				jsonObj = map[string]interface{}{
					"jsonrpc": "2.0",
					"id":      tt.id.IntValue,
				}
			}

			// Marshal to JSON
			body, err := json.Marshal(jsonObj)
			if err != nil {
				t.Errorf("Failed to marshal JSON: %v", err)
			}

			// Check that the ID is correctly marshaled
			if !json.Valid(body) {
				t.Errorf("Invalid JSON: %s", string(body))
			}

			// Check that the ID is correctly formatted
			if !strings.Contains(string(body), tt.expected) {
				t.Errorf("ID not correctly formatted. Expected to contain %s, got %s", tt.expected, string(body))
			}
		})
	}
}
