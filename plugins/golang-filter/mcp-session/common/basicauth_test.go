package common

import (
	"encoding/base64"
	"testing"
)

func basicHeader(user, pass string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
}

func TestCheckBasicAuth(t *testing.T) {
	tests := []struct {
		name       string
		authHeader string
		wantUser   string
		wantPass   string
		want       bool
	}{
		{"valid", basicHeader("admin", "s3cret"), "admin", "s3cret", true},
		{"wrong password", basicHeader("admin", "nope"), "admin", "s3cret", false},
		{"wrong username", basicHeader("root", "s3cret"), "admin", "s3cret", false},
		{"empty expected username disables auth", basicHeader("admin", "s3cret"), "", "s3cret", false},
		{"missing header", "", "admin", "s3cret", false},
		{"non-basic scheme", "Bearer sometoken", "admin", "s3cret", false},
		{"malformed base64", "Basic !!!notbase64!!!", "admin", "s3cret", false},
		{"missing colon", "Basic " + base64.StdEncoding.EncodeToString([]byte("adminonly")), "admin", "s3cret", false},
		{"case-insensitive scheme", "basic " + base64.StdEncoding.EncodeToString([]byte("admin:s3cret")), "admin", "s3cret", true},
		{"password containing colon", basicHeader("admin", "a:b:c"), "admin", "a:b:c", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CheckBasicAuth(tt.authHeader, tt.wantUser, tt.wantPass); got != tt.want {
				t.Fatalf("CheckBasicAuth(%q, %q, %q) = %v, want %v", tt.authHeader, tt.wantUser, tt.wantPass, got, tt.want)
			}
		})
	}
}
