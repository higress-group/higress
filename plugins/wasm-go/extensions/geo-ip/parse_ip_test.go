package main

import "testing"

func TestParseIPMalformedHeaderDoesNotPanic(t *testing.T) {
	// X-Forwarded-For is attacker controlled when ip_source_type is "header".
	for _, tc := range []struct {
		source string
		want   string
	}{
		{source: "]", want: ""},
		{source: "]:80", want: ""},
		{source: "[::1]", want: "::1"},
		{source: "[fe80::14d5:8aff:fed9:2114]:123", want: "fe80::14d5:8aff:fed9:2114"},
		{source: "127.0.0.1", want: "127.0.0.1"},
	} {
		t.Run(tc.source, func(t *testing.T) {
			if got := parseIP(tc.source); got != tc.want {
				t.Errorf("parseIP(%q) = %q, want %q", tc.source, got, tc.want)
			}
		})
	}
}
