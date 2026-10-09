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
		{
			name:   "empty cookie returns empty",
			cookie: "",
			key:    "user",
			want:   "",
		},
		{
			name:   "key not present returns empty",
			cookie: "user=alice; other=value",
			key:    "missing",
			want:   "",
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

func TestParseIPMalformedValueDoesNotPanic(t *testing.T) {
	// The value comes from the configured real-IP header when
	// limit_by_per_ip.source_type is "header", so it is client controlled.
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{name: "closing bracket only", source: "]", want: ""},
		{name: "closing bracket with port", source: "]:80", want: ""},
		{name: "bracketed ipv6", source: "[::1]", want: "::1"},
		{name: "bracketed ipv6 with port", source: "[fe80::14d5:8aff:fed9:2114]:123", want: "fe80::14d5:8aff:fed9:2114"},
		{name: "ipv4", source: "127.0.0.1", want: "127.0.0.1"},
		{name: "ipv4 with port", source: "127.0.0.1:12", want: "127.0.0.1"},
		{name: "bare ipv6", source: "fe80::14d5:8aff:fed9:2114", want: "fe80::14d5:8aff:fed9:2114"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseIP(tt.source); got != tt.want {
				t.Fatalf("ParseIP(%q) = %q, want %q", tt.source, got, tt.want)
			}
		})
	}
}
