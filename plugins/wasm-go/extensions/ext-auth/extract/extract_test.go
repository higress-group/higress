package extract

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValue(t *testing.T) {
	headers := http.Header{}
	headers.Set("X-User-Token", "abc")
	headers.Set("X-Empty", "")

	body := []byte(`{"data":{"code":"OK","uid":"1024"},"friends":[{"first":"Alice"}],"count":3,"ok":true,"nil":null}`)

	tests := []struct {
		name       string
		source     string
		key        string
		statusCode int
		headers    http.Header
		body       []byte
		wantValue  string
		wantOK     bool
	}{
		{"status_code always extracts", SourceStatusCode, "", 200, nil, nil, "200", true},
		{"status_code non-200", SourceStatusCode, "", 403, nil, nil, "403", true},
		{"header hit is case-insensitive on key", SourceHeader, "x-user-token", 200, headers, nil, "abc", true},
		{"header hit canonical key", SourceHeader, "X-User-Token", 200, headers, nil, "abc", true},
		{"header missing", SourceHeader, "x-absent", 200, headers, nil, "", false},
		{"header present but empty", SourceHeader, "x-empty", 200, headers, nil, "", false},
		{"header nil map", SourceHeader, "x-user-token", 200, nil, nil, "", false},
		{"body_json nested path", SourceBodyJson, "data.uid", 200, nil, body, "1024", true},
		{"body_json string field", SourceBodyJson, "data.code", 200, nil, body, "OK", true},
		{"body_json array index", SourceBodyJson, "friends.0.first", 200, nil, body, "Alice", true},
		{"body_json number field", SourceBodyJson, "count", 200, nil, body, "3", true},
		{"body_json bool field", SourceBodyJson, "ok", 200, nil, body, "true", true},
		{"body_json path missing", SourceBodyJson, "data.absent", 200, nil, body, "", false},
		{"body_json empty body", SourceBodyJson, "data.uid", 200, nil, []byte{}, "", false},
		{"body_json nil body", SourceBodyJson, "data.uid", 200, nil, nil, "", false},
		{"body_json explicit null exists", SourceBodyJson, "nil", 200, nil, body, "", true},
		{"unknown source", "unknown", "data.uid", 200, headers, body, "", false},
		{"empty source", "", "data.uid", 200, headers, body, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotValue, gotOK := Value(tt.source, tt.key, tt.statusCode, tt.headers, tt.body)
			assert.Equal(t, tt.wantOK, gotOK)
			assert.Equal(t, tt.wantValue, gotValue)
		})
	}
}
