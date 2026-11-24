package tools

import "testing"

func TestParseIP(t *testing.T) {
	cases := []struct {
		name       string
		source     string
		fromHeader bool
		want       string
	}{
		{"ipv4 only", "10.0.0.1", false, "10.0.0.1"},
		{"ipv4 with port", "10.0.0.1:8080", false, "10.0.0.1"},
		{"ipv4 X-Forwarded-For first hop", "10.0.0.1, 10.0.0.2, 10.0.0.3", true, "10.0.0.1"},
		{"ipv6 bracketed with port", "[2001:db8::1]:443", false, "2001:db8::1"},
		{"ipv6 bracketed no port", "[2001:db8::1]", false, "2001:db8::1"},
		{"empty string", "", false, ""},
		{"xff closing bracket only", "]", true, ""},
		{"xff closing bracket prefix", "]-evil", true, ""},
		{"ipv6 mapped with port", "[::ffff:198.51.100.9]:8080", true, "::ffff:198.51.100.9"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseIP(c.source, c.fromHeader)
			if got != c.want {
				t.Fatalf("parseIP(%q, %v) = %q, want %q", c.source, c.fromHeader, got, c.want)
			}
		})
	}
}
