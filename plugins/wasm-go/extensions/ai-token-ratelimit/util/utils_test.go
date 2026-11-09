package util

import (
	"fmt"
	"testing"
)

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

// TestParseIPMalformedValueDoesNotPanic is a regression test for the client
// controlled real-IP header used by limit_by_per_ip (source_type "header").
//
// The previous implementation guessed the address family from the presence of
// a dot and split on colons. That both panicked for values such as "]" (the
// wasm-go wrapper recovers the panic and continues without the limiter) and
// mis-parsed valid IPv4-mapped IPv6 such as "::ffff:192.0.2.1" as empty.
// ParseIP now parses the whole value with netip, accepts IPv4, IPv6 and
// IPv4-mapped IPv6, and returns an empty string only for values that cannot be
// resolved so the caller can fail closed.
func TestParseIPMalformedValueDoesNotPanic(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{name: "closing bracket only", source: "]", want: ""},
		{name: "closing bracket with port", source: "]:80", want: ""},
		{name: "closing bracket with trailing text", source: "]abc", want: ""},
		{name: "empty bracket pair", source: "[]", want: ""},
		{name: "nested closing brackets", source: "][", want: ""},
		{name: "stray opening bracket", source: "[", want: ""},
		{name: "opening bracket without closing", source: "[::1", want: ""},
		{name: "bracketed value with trailing text", source: "[::1]junk", want: ""},
		{name: "too many port separators", source: "192.0.2.1:80:90", want: ""},
		{name: "not an ip", source: "not-an-ip", want: ""},
		{name: "out of range ipv4", source: "999.999.999.999", want: ""},
		{name: "whitespace only", source: "   ", want: ""},
		{name: "bracketed ipv6", source: "[::1]", want: "::1"},
		{name: "bracketed ipv6 with port", source: "[fe80::14d5:8aff:fed9:2114]:123", want: "fe80::14d5:8aff:fed9:2114"},
		{name: "bracketed ipv6 with large port", source: "[::1]:99999", want: "::1"},
		{name: "bracketed ipv6 full form", source: "[2001:db8::1]", want: "2001:db8::1"},
		{name: "bare ipv6", source: "fe80::14d5:8aff:fed9:2114", want: "fe80::14d5:8aff:fed9:2114"},
		{name: "compressed loopback ipv6", source: "::1", want: "::1"},
		{name: "ipv6 with zone", source: "fe80::1%eth0", want: "fe80::1"},
		{name: "bracketed ipv6 with zone and port", source: "[fe80::1%eth0]:80", want: "fe80::1"},
		{name: "ipv4", source: "127.0.0.1", want: "127.0.0.1"},
		{name: "ipv4 with port", source: "127.0.0.1:12", want: "127.0.0.1"},
		{name: "ipv4 with surrounding whitespace", source: " 127.0.0.1 ", want: "127.0.0.1"},
		{name: "ipv4 mapped ipv6", source: "::ffff:192.0.2.1", want: "192.0.2.1"},
		{name: "ipv4 mapped ipv6 uppercase", source: "::FFFF:192.0.2.1", want: "192.0.2.1"},
		{name: "ipv4 mapped ipv6 hex form", source: "::ffff:c000:201", want: "192.0.2.1"},
		{name: "bracketed ipv4 mapped ipv6", source: "[::ffff:192.0.2.1]", want: "192.0.2.1"},
		{name: "bracketed ipv4 mapped ipv6 with port", source: "[::ffff:192.0.2.1]:443", want: "192.0.2.1"},
		{name: "empty value", source: "", want: ""},
		// getDownStreamIp trims the comma separated X-Forwarded-For list before
		// calling ParseIP, so a raw list is not part of this helper's contract
		// and must be rejected rather than partially interpreted.
		{name: "raw comma separated list is caller responsibility", source: "]:80, 127.0.0.1", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseIP(tt.source)
			if got != tt.want {
				t.Fatalf("ParseIP(%q) = %q, want %q", tt.source, got, tt.want)
			}
		})
	}
}

// TestParseIPNeverPanicsOnAdversarialInput feeds ParseIP a wider set of hostile
// values that a client can put into the real-IP header. The helper must always
// return; a panic here would be recovered by the wasm-go wrapper and silently
// turn the request into an unlimited pass.
func TestParseIPNeverPanicsOnAdversarialInput(t *testing.T) {
	inputs := []string{
		"", " ", "\t", "\n",
		"]", "]:", "]:0", "]:80", "]:80]", "]abc", "[", "[]", "[][", "][", "[ ]",
		"[::1]", "[::1]:", "[::1]:0", "[::1]:99999", "[fe80::1]extra",
		"::", "::1", "fe80::1%eth0", "2001:db8::1", "1.2.3.4]", "]:80, 127.0.0.1",
		"0.0.0.0", "255.255.255.255", "999.999.999.999", "1.2.3.4:99999", ",",
		",,", "not-an-ip", "1000", "\x00", "\xff", "a::b::c",
	}
	for i, in := range inputs {
		t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("ParseIP(%q) panicked: %v", in, r)
				}
			}()
			_ = ParseIP(in)
		})
	}
}

// TestParseIPBracketPortMatrix exercises the IPv6 bracket/port handling
// exhaustively for a few addresses and asserts that every bracket-only
// malformed value resolves to the empty string rather than panicking.
func TestParseIPBracketPortMatrix(t *testing.T) {
	addresses := []string{"::", "::1", "2001:db8::1", "fe80::14d5:8aff:fed9:2114"}
	for _, addr := range addresses {
		cases := []struct {
			in   string
			want string
		}{
			{in: addr, want: addr},
			{in: "[" + addr + "]", want: addr},
			{in: "[" + addr + "]:80", want: addr},
			{in: "[" + addr + "]:65535", want: addr},
		}
		for _, c := range cases {
			if got := ParseIP(c.in); got != c.want {
				t.Fatalf("ParseIP(%q) = %q, want %q", c.in, got, c.want)
			}
		}
	}

	malformed := []string{"]", "]:80", "]abc", "[]", "][", "]:", "[]:80"}
	for _, in := range malformed {
		if got := ParseIP(in); got != "" {
			t.Fatalf("ParseIP(%q) = %q, want empty string", in, got)
		}
	}
}
