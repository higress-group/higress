package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestResultJSONBoundaries(t *testing.T) {
	for _, tc := range []struct {
		body         string
		search, want bool
	}{
		{`{"d\u0061ta":[null,{"url":""},{"b64_\u006ason":"x"}]}`, false, true},
		{`{"data":[{"b64_json":"a\u002fb"}],"meta":[-0,12.30e+1,false,null,true]}`, false, true},
		{`{"data":[{"url":"image","error":"nested"}],"err\u006fr":null}`, false, false},
		{`{"meta":{"data":[{"url":"nested"}]},"data":[[{"url":"wrong depth"}],{"url":false}]}`, false, false},
		{`{"results":[{"score":0.01}]}`, true, true},
		{`{"results":[],"error":null}`, true, false},
		{`{"data":[{"url":"x"}],"n":1e}`, false, false},
		{`{"data":[{"url":"x"}],"n":01}`, false, false},
		{`{"data":[{"url":"x"}],"x":"\u999g"}`, false, false},
		{`{"data":[{"url":"x"}],}`, false, false},
		{`{"data":[{"url":"x"}]}{}`, false, false},
	} {
		for split := 0; split <= len(tc.body); split++ {
			p := resultJSON{search: tc.search}
			p.feed([]byte(tc.body[:split]))
			p.feed([]byte(tc.body[split:]))
			if p.billable() != tc.want {
				t.Fatalf("split %d, body %q, want %v", split, tc.body, tc.want)
			}
		}
	}
	p := resultJSON{}
	p.feed([]byte(`{"` + strings.Repeat("k", 100000) + `":true,"data":[{"b64_json":"`))
	chunk := []byte(strings.Repeat("A", 64<<10))
	for i := 0; i < 64; i++ {
		p.feed(chunk)
	}
	if p.billable() {
		t.Fatal("unfinished document")
	}
	p.feed([]byte(`"}]}`))
	if !p.billable() {
		t.Fatal("complete large image")
	}
}

// For arbitrary metadata, a fixed otherwise successful response must be
// accepted exactly when the standard library accepts the full JSON document.
func FuzzResultJSONValue(f *testing.F) {
	for _, seed := range []string{"null", "false", "-10.2e+52", `"a\u002fb"`, `{"nested":[true,{},[]]}`, "1.", "[1,]", `"\u01"`, ""} {
		f.Add(seed, uint8(3))
	}
	f.Fuzz(func(t *testing.T, value string, stride uint8) {
		body := []byte(`{"data":[{"url":"image"}],"meta":` + value + `}`)
		want := json.Valid(body)
		var p resultJSON
		for len(body) > 0 {
			n := min(len(body), int(stride)+1)
			p.feed(body[:n])
			body = body[n:]
		}
		if complete := p.started && !p.invalid && len(p.stack) == 0; complete != want {
			t.Fatalf("metadata %q: want valid document %v", value, want)
		}
		if json.Valid([]byte(value)) && !p.billable() {
			t.Fatalf("valid metadata suppressed a successful result: %q", value)
		}
	})
}
