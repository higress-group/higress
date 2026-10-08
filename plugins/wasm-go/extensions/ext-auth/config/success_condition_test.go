package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tidwall/gjson"
)

func TestParseSuccessCondition(t *testing.T) {
	tests := []struct {
		name        string
		json        string
		expected    []Condition
		expectedErr string
	}{
		{name: "Not Exists", json: `{"timeout": 1000}`, expected: nil},
		{name: "Not An Array", json: `{"success_condition": {"source": "status_code"}}`, expectedErr: "success_condition must be an array"},
		{name: "Empty Array", json: `{"success_condition": []}`, expected: []Condition{}},
		{
			name:     "Body Json Eq Scalar Value",
			json:     `{"success_condition": [{"source": "body_json", "key": "data.code", "op": "eq", "value": "0"}]}`,
			expected: []Condition{{Source: "body_json", Key: "data.code", Op: "eq", Value: []string{"0"}}},
		},
		{
			name:     "In Op Array Value",
			json:     `{"success_condition": [{"source": "header", "key": "x-role", "op": "in", "value": ["admin", "owner"]}]}`,
			expected: []Condition{{Source: "header", Key: "x-role", Op: "in", Value: []string{"admin", "owner"}}},
		},
		{
			name:     "Status Code Without Key",
			json:     `{"success_condition": [{"source": "status_code", "op": "eq", "value": "200"}]}`,
			expected: []Condition{{Source: "status_code", Key: "", Op: "eq", Value: []string{"200"}}},
		},
		{
			name:     "Exists Without Value",
			json:     `{"success_condition": [{"source": "body_json", "key": "data.uid", "op": "exists"}]}`,
			expected: []Condition{{Source: "body_json", Key: "data.uid", Op: "exists", Value: nil}},
		},
		{
			name:     "Not Exists Without Value",
			json:     `{"success_condition": [{"source": "body_json", "key": "error", "op": "not_exists"}]}`,
			expected: []Condition{{Source: "body_json", Key: "error", Op: "not_exists", Value: nil}},
		},
		{
			name:     "Numeric Value Coerced To String",
			json:     `{"success_condition": [{"source": "body_json", "key": "data.count", "op": "gt", "value": 10}]}`,
			expected: []Condition{{Source: "body_json", Key: "data.count", Op: "gt", Value: []string{"10"}}},
		},
		{
			name: "Multiple Conditions",
			json: `{"success_condition": [{"source": "status_code", "op": "eq", "value": "200"}, {"source": "body_json", "key": "data.code", "op": "eq", "value": "0"}, {"source": "header", "key": "x-allowed", "op": "not_exists"}]}`,
			expected: []Condition{
				{Source: "status_code", Key: "", Op: "eq", Value: []string{"200"}},
				{Source: "body_json", Key: "data.code", Op: "eq", Value: []string{"0"}},
				{Source: "header", Key: "x-allowed", Op: "not_exists", Value: nil},
			},
		},
		{
			name:        "Invalid Source",
			json:        `{"success_condition": [{"source": "cookie", "key": "sid", "op": "exists"}]}`,
			expectedErr: `success_condition[0]: invalid source "cookie", must be one of status_code, header, body_json`,
		},
		{
			name:        "Invalid Op",
			json:        `{"success_condition": [{"source": "body_json", "key": "data.code", "op": "matches", "value": "0"}]}`,
			expectedErr: `success_condition[0]: invalid op "matches"`,
		},
		{
			name:        "Header Missing Key",
			json:        `{"success_condition": [{"source": "header", "op": "exists"}]}`,
			expectedErr: `success_condition[0]: source "header" requires a key`,
		},
		{
			name:        "Body Json Missing Key",
			json:        `{"success_condition": [{"source": "body_json", "op": "exists"}]}`,
			expectedErr: `success_condition[0]: source "body_json" requires a key`,
		},
		{
			name:        "Value Op Missing Value",
			json:        `{"success_condition": [{"source": "body_json", "key": "data.code", "op": "eq"}]}`,
			expectedErr: `success_condition[0]: op "eq" requires a value`,
		},
		{
			name:        "In Op Missing Value",
			json:        `{"success_condition": [{"source": "header", "key": "x-role", "op": "in"}]}`,
			expectedErr: `success_condition[0]: op "in" requires a value`,
		},
		{
			name:        "Error Index For Second Item",
			json:        `{"success_condition": [{"source": "status_code", "op": "eq", "value": "200"}, {"source": "body_json", "key": "x", "op": "bad"}]}`,
			expectedErr: `success_condition[1]: invalid op "bad"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSuccessCondition(gjson.Parse(tt.json).Get("success_condition"))
			if tt.expectedErr != "" {
				assert.EqualError(t, err, tt.expectedErr)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, got)
			}
		})
	}
}
