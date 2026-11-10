package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tidwall/gjson"
)

func TestParseMappedUpstreamHeaders(t *testing.T) {
	tests := []struct {
		name        string
		json        string
		expected    []HeaderMapping
		expectedErr string
	}{
		{name: "Not An Array", json: `{"mapped_upstream_headers": {"source": "status_code"}}`, expectedErr: "mapped_upstream_headers must be an array"},
		{name: "Empty Array", json: `{"mapped_upstream_headers": []}`, expected: []HeaderMapping{}},
		{
			name:     "Body Json Mapping",
			json:     `{"mapped_upstream_headers": [{"source": "body_json", "key": "data.uid", "to_header": "x-auth-user-id"}]}`,
			expected: []HeaderMapping{{Source: "body_json", Key: "data.uid", ToHeader: "x-auth-user-id"}},
		},
		{
			name:     "Header Mapping",
			json:     `{"mapped_upstream_headers": [{"source": "header", "key": "x-user-token", "to_header": "x-auth-token"}]}`,
			expected: []HeaderMapping{{Source: "header", Key: "x-user-token", ToHeader: "x-auth-token"}},
		},
		{
			name:     "Status Code Mapping Without Key",
			json:     `{"mapped_upstream_headers": [{"source": "status_code", "to_header": "x-auth-status"}]}`,
			expected: []HeaderMapping{{Source: "status_code", Key: "", ToHeader: "x-auth-status"}},
		},
		{
			name: "Multiple Mappings",
			json: `{"mapped_upstream_headers": [{"source": "body_json", "key": "data.uid", "to_header": "x-auth-user-id"}, {"source": "header", "key": "x-user-token", "to_header": "x-auth-token"}]}`,
			expected: []HeaderMapping{
				{Source: "body_json", Key: "data.uid", ToHeader: "x-auth-user-id"},
				{Source: "header", Key: "x-user-token", ToHeader: "x-auth-token"},
			},
		},
		{
			name:        "Invalid Source",
			json:        `{"mapped_upstream_headers": [{"source": "cookie", "key": "sid", "to_header": "x-sid"}]}`,
			expectedErr: `mapped_upstream_headers[0]: invalid source "cookie", must be one of status_code, header, body_json`,
		},
		{
			name:        "Header Missing Key",
			json:        `{"mapped_upstream_headers": [{"source": "header", "to_header": "x-auth-token"}]}`,
			expectedErr: `mapped_upstream_headers[0]: source "header" requires a key`,
		},
		{
			name:        "Body Json Missing Key",
			json:        `{"mapped_upstream_headers": [{"source": "body_json", "to_header": "x-auth-user-id"}]}`,
			expectedErr: `mapped_upstream_headers[0]: source "body_json" requires a key`,
		},
		{
			name:        "Missing To Header",
			json:        `{"mapped_upstream_headers": [{"source": "body_json", "key": "data.uid"}]}`,
			expectedErr: `mapped_upstream_headers[0]: missing required field 'to_header'`,
		},
		{
			name:        "Error Index For Second Item",
			json:        `{"mapped_upstream_headers": [{"source": "status_code", "to_header": "x-auth-status"}, {"source": "body_json", "key": "data.uid"}]}`,
			expectedErr: `mapped_upstream_headers[1]: missing required field 'to_header'`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseMappedUpstreamHeaders(gjson.Parse(tt.json).Get("mapped_upstream_headers"))
			if tt.expectedErr != "" {
				assert.EqualError(t, err, tt.expectedErr)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, got)
			}
		})
	}
}
