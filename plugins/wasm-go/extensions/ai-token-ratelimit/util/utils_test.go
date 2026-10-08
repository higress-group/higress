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

func TestParseIP(t *testing.T) {
	cases := []struct {
		name   string
		source string
		want   string
	}{
		{"empty string", "", ""},
		{"ipv4 bare", "192.168.1.1", "192.168.1.1"},
		{"ipv4 with port", "192.168.1.1:8080", "192.168.1.1"},
		{"ipv6 bare", "2001:db8::1", "2001:db8::1"},
		{"ipv6 bracketed no port", "[2001:db8::1]", "2001:db8::1"},
		{"ipv6 bracketed with port", "[2001:db8::1]:8080", "2001:db8::1"},
		{"closing bracket only", "]", "]"},
		{"starts with closing bracket", "]-evil", "]-evil"},
		{"closing bracket with port", "]:8080", "]:8080"},
		{"closing bracket without opening", "::1]", "::1]"},
		{"unclosed bracket", "[::1", "[::1"},
		{"empty brackets", "[]", "[]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ParseIP(c.source)
			if got != c.want {
				t.Errorf("ParseIP(%q) = %q, want %q", c.source, got, c.want)
			}
		})
	}
}
