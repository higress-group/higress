package handler

import "testing"

func TestFindCookie(t *testing.T) {
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
			if got := findCookie(tt.cookie, tt.key); got != tt.want {
				t.Fatalf("findCookie() = %q, want %q", got, tt.want)
			}
		})
	}
}
