package main

import (
	"encoding/json"
	"testing"

	"github.com/tidwall/gjson"
)

func TestBuildUpstreamBodyEscapesInputs(t *testing.T) {
	config := MyConfig{Model: "text-davinci-003", HumanId: "Human:", AIId: "AI:"}

	for _, tc := range []struct {
		name   string
		prompt string
	}{
		{name: "plain", prompt: "Tell me a joke"},
		{name: "double quote", prompt: `He said "hello" and left`},
		{name: "backslash", prompt: `C:\temp\new`},
		{name: "newline", prompt: "line one\nline two"},
		{name: "single quote", prompt: "I don't know"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := buildUpstreamBody(config, tc.prompt)

			if !json.Valid(body) {
				t.Fatalf("upstream request body is not valid JSON:\n%s", body)
			}
			if got := gjson.GetBytes(body, "prompt").String(); got != tc.prompt {
				t.Fatalf("prompt round-trip = %q, want %q", got, tc.prompt)
			}
			if got := gjson.GetBytes(body, "model").String(); got != config.Model {
				t.Fatalf("model = %q, want %q", got, config.Model)
			}
			if got := gjson.GetBytes(body, "stop.0").String(); got != " "+config.HumanId {
				t.Fatalf("stop[0] = %q, want %q", got, " "+config.HumanId)
			}
			if got := gjson.GetBytes(body, "stop.1").String(); got != " "+config.AIId {
				t.Fatalf("stop[1] = %q, want %q", got, " "+config.AIId)
			}
		})
	}
}
