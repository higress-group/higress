package util

import "testing"

func TestExtractCookieValueByKey(t *testing.T) {
	tests := []struct {
		name   string
		cookie string
		key    string
		want   string
	}{
		{
			name:   "extracts matching cookie value",
			cookie: "user=alice; other=value",
			key:    "user",
			want:   "alice",
		},
		{
			name:   "skips segment without equals sign",
			cookie: "user; other=value",
			key:    "user",
			want:   "",
		},
		{
			name:   "keeps equals signs in cookie value",
			cookie: "user=alice=admin; other=value",
			key:    "user",
			want:   "alice=admin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExtractCookieValueByKey(tt.cookie, tt.key); got != tt.want {
				t.Fatalf("ExtractCookieValueByKey() = %q, want %q", got, tt.want)
			}
		})
	}
}
