package main

import "testing"

func TestIsNoSearchReply(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
		want    bool
	}{
		{name: "exact none", content: "none", want: true},
		{name: "upper case none", content: "NONE", want: true},
		{name: "none with surrounding whitespace", content: "  none\n", want: true},
		{name: "search plan containing nonetheless", content: "internet: nonetheless meaning", want: false},
		{name: "search plan containing nonempty", content: "internet: nonempty string check", want: false},
		{name: "question about the word none", content: "internet: the English word none", want: false},
		{name: "ordinary search plan", content: "internet: higress gateway", want: false},
		{name: "sentence containing none", content: "none of the above", want: false},
		{name: "empty reply", content: "", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := isNoSearchReply(tc.content); got != tc.want {
				t.Fatalf("isNoSearchReply(%q) = %v, want %v", tc.content, got, tc.want)
			}
		})
	}
}
