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

// TestParseIP covers every input family ParseIP has to accept or reject.
//
// The malformedBracketValues block is the regression guard for the reported
// crash: these values used to reach `strings.Split(source, "]")[0][1:]`, which
// panics with "slice bounds out of range [1:0]" whenever the text before the
// first "]" is empty. They are asserted twice - once for the return value and
// once through assert-style no-panic checks - so a future refactor cannot
// reintroduce the panic silently.
func TestParseIP(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		// Malformed bracket values: used to panic, now pass through unchanged so
		// the caller rejects them with "invalid ip[...]".
		{name: "closing bracket only", input: "]", want: "]"},
		{name: "closing bracket with port", input: "]:8080", want: "]:8080"},
		{name: "closing bracket with junk", input: "]junk", want: "]junk"},
		{name: "empty input", input: "", want: ""},
		{name: "non-ip text", input: "garbage", want: "garbage"},

		// Socket addresses: the port is stripped.
		{name: "ipv4 with port", input: "1.2.3.4:8080", want: "1.2.3.4"},
		{name: "bracketed ipv6 with port", input: "[::1]:443", want: "::1"},
		{name: "bracketed ipv6 with port long", input: "[2001:db8::1]:443", want: "2001:db8::1"},

		// Bare and bracketed literals.
		{name: "bare ipv4", input: "1.2.3.4", want: "1.2.3.4"},
		{name: "bare ipv6", input: "2001:db8::1", want: "2001:db8::1"},
		{name: "bracketed ipv6", input: "[::1]", want: "::1"},
		{name: "bracketed ipv4", input: "[1.2.3.4]", want: "1.2.3.4"},
		{name: "ipv4 mapped ipv6", input: "::ffff:1.2.3.4", want: "::ffff:1.2.3.4"},

		// Not a valid socket address in any interpretation: passed through so the
		// caller rejects it, instead of being silently truncated to "1.2.3.4".
		{name: "too many colon segments", input: "1.2.3.4:8080:extra", want: "1.2.3.4:8080:extra"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseIP(tt.input); got != tt.want {
				t.Fatalf("ParseIP(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestParseIPDoesNotPanic states the regression guard separately from the value
// assertions, so the intent is explicit even if the expected values above are
// ever revised.
func TestParseIPDoesNotPanic(t *testing.T) {
	for _, input := range []string{"]", "]:8080", "]junk", "][", "[]", "[]:80", "]", "[", ""} {
		input := input
		t.Run(input, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("ParseIP(%q) panicked: %v", input, r)
				}
			}()
			_ = ParseIP(input)
		})
	}
}
