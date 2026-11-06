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

func TestDeleteCookie(t *testing.T) {
	tests := []struct {
		name   string
		cookie string
		key    string
		want   string
	}{
		{
			name:   "removes matching key",
			cookie: "user=alice; other=value",
			key:    "user",
			want:   "other=value",
		},
		{
			name:   "key not present keeps all segments",
			cookie: "user=alice; other=value",
			key:    "missing",
			want:   "user=alice;other=value",
		},
		// deleteCookie 用 strings.Cut(pair, "=") 取名并精确匹配（与 findCookie 一致）：
		// 无 "=" 的段不会匹配任何 key，会被保留。
		{
			name:   "segment without equals sign is kept",
			cookie: "user; other=value",
			key:    "user",
			want:   "user;other=value",
		},
		{
			name:   "keeps cookies whose name shares the token cookie prefix",
			cookie: "user_session=<jwt>; user_session_v2=keepme; theme=dark",
			key:    "user_session",
			want:   "user_session_v2=keepme;theme=dark",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deleteCookie(tt.cookie, tt.key); got != tt.want {
				t.Fatalf("deleteCookie() = %q, want %q", got, tt.want)
			}
		})
	}
}
