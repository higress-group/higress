package main

import (
	"testing"

	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/tidwall/gjson"
)

// noopLog satisfies log.Log without touching the wasm host, so the rewrite
// helper can be driven directly from a unit test.
type noopLog struct{}

func (noopLog) Trace(string)                     {}
func (noopLog) Tracef(string, ...interface{})    {}
func (noopLog) Debug(string)                     {}
func (noopLog) Debugf(string, ...interface{})    {}
func (noopLog) Info(string)                      {}
func (noopLog) Infof(string, ...interface{})     {}
func (noopLog) Warn(string)                      {}
func (noopLog) Warnf(string, ...interface{})     {}
func (noopLog) Error(string)                     {}
func (noopLog) Errorf(string, ...interface{})    {}
func (noopLog) Critical(string)                  {}
func (noopLog) Criticalf(string, ...interface{}) {}
func (noopLog) ResetID(string)                   {}

func TestSearchRewriteForRequestKeepsPluginConfig(t *testing.T) {
	log.SetPluginLog(noopLog{})

	base := &SearchRewrite{
		prompt:         "search up to 3 times",
		promptTemplate: "search up to {max_count} times",
		maxCount:       3,
	}

	effective := searchRewriteForRequest(base, gjson.Parse("{\"search_context_size\":\"high\"}"))
	if effective == nil {
		t.Fatal("expected effective rewrite settings for the request")
	}
	if effective.maxCount != 5 || effective.prompt != "search up to 5 times" {
		t.Fatalf("effective settings = {maxCount:%d prompt:%q}, want {5, %q}",
			effective.maxCount, effective.prompt, "search up to 5 times")
	}

	// web_search_options is request scoped: the plugin config must survive it.
	if base.maxCount != 3 || base.prompt != "search up to 3 times" {
		t.Fatalf("plugin config was mutated by a request: maxCount=%d prompt=%q",
			base.maxCount, base.prompt)
	}
	if effective == base {
		t.Fatal("expected a per-request copy, got the shared plugin config")
	}

	// A later request without web_search_options must still see the configured value.
	next := searchRewriteForRequest(base, gjson.Result{})
	if next == nil || next.maxCount != 3 {
		t.Fatalf("later request without web_search_options got %v, want maxCount 3", next)
	}

	// An unknown value must leave the configured value alone.
	unknown := searchRewriteForRequest(base, gjson.Parse("{\"search_context_size\":\"gigantic\"}"))
	if unknown == nil || unknown.maxCount != 3 {
		t.Fatalf("unknown search_context_size changed maxCount to %v, want 3", unknown)
	}
	if base.maxCount != 3 || base.prompt != "search up to 3 times" {
		t.Fatalf("plugin config was mutated: maxCount=%d prompt=%q", base.maxCount, base.prompt)
	}
}
