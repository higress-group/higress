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
		// deleteCookie 用 HasPrefix(pair, key) 删除，不区分段是否含 "="：
		// 任何以 key 开头的段都会被删除（包括无 "=" 的段）。
		{
			name:   "segment without equals sign is deleted when key matches prefix",
			cookie: "user; other=value",
			key:    "user",
			want:   "other=value",
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
