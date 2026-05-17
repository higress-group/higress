// Copyright (c) 2025 Alibaba Group Holding Ltd.
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

package main

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tidwall/gjson"
)

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		expected RequestIDConfig
	}{
		{
			name: "default config",
			json: `{}`,
			expected: RequestIDConfig{
				Enable:           true,
				RequestHeader:    "X-Request-Id",
				ResponseHeader:   "",
				OverrideExisting: false,
			},
		},
		{
			name: "custom request header",
			json: `{"request_header": "X-Trace-Id"}`,
			expected: RequestIDConfig{
				Enable:           true,
				RequestHeader:    "X-Trace-Id",
				ResponseHeader:   "",
				OverrideExisting: false,
			},
		},
		{
			name: "with response header",
			json: `{"response_header": "X-Request-Id"}`,
			expected: RequestIDConfig{
				Enable:           true,
				RequestHeader:    "X-Request-Id",
				ResponseHeader:   "X-Request-Id",
				OverrideExisting: false,
			},
		},
		{
			name: "override existing enabled",
			json: `{"override_existing": true}`,
			expected: RequestIDConfig{
				Enable:           true,
				RequestHeader:    "X-Request-Id",
				ResponseHeader:   "",
				OverrideExisting: true,
			},
		},
		{
			name: "plugin disabled",
			json: `{"enable": false}`,
			expected: RequestIDConfig{
				Enable:           false,
				RequestHeader:    "X-Request-Id",
				ResponseHeader:   "",
				OverrideExisting: false,
			},
		},
		{
			name: "full config",
			json: `{
				"enable": true,
				"request_header": "X-Custom-Request-Id",
				"response_header": "X-Custom-Response-Id",
				"override_existing": true
			}`,
			expected: RequestIDConfig{
				Enable:           true,
				RequestHeader:    "X-Custom-Request-Id",
				ResponseHeader:   "X-Custom-Response-Id",
				OverrideExisting: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &RequestIDConfig{}
			// Create a mock logger (nil is acceptable for tests)
			err := parseConfig(gjson.Parse(tt.json), config, nil)
			
			assert.NoError(t, err)
			assert.Equal(t, tt.expected.Enable, config.Enable)
			assert.Equal(t, tt.expected.RequestHeader, config.RequestHeader)
			assert.Equal(t, tt.expected.ResponseHeader, config.ResponseHeader)
			assert.Equal(t, tt.expected.OverrideExisting, config.OverrideExisting)
		})
	}
}

func TestGenerateUUID(t *testing.T) {
	// UUID v4 format: xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
	// where y is one of 8, 9, a, or b
	uuidPattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

	t.Run("generates valid UUID v4", func(t *testing.T) {
		uuid, err := generateUUID()
		assert.NoError(t, err)
		assert.Regexp(t, uuidPattern, uuid)
	})

	t.Run("generates unique UUIDs", func(t *testing.T) {
		uuids := make(map[string]bool)
		
		// Generate 100 UUIDs and ensure they're all unique
		for i := 0; i < 100; i++ {
			uuid, err := generateUUID()
			assert.NoError(t, err)
			assert.False(t, uuids[uuid], "UUID collision detected: %s", uuid)
			uuids[uuid] = true
		}
		
		assert.Equal(t, 100, len(uuids))
	})

	t.Run("UUID format validation", func(t *testing.T) {
		uuid, err := generateUUID()
		assert.NoError(t, err)
		
		// Check length (36 characters including hyphens)
		assert.Len(t, uuid, 36)
		
		// Check version bit (character at position 14 should be '4')
		assert.Equal(t, byte('4'), uuid[14], "UUID version should be 4")
		
		// Check variant bits (character at position 19 should be 8, 9, a, or b)
		variantChar := uuid[19]
		assert.Contains(t, "89ab", string(variantChar), "UUID variant should be 8, 9, a, or b")
	})
}

