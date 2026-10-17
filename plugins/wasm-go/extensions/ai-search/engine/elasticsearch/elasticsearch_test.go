package elasticsearch

import (
	"encoding/json"
	"testing"

	"github.com/tidwall/gjson"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-search/engine"
)

func TestGenerateQueryBodyEscapesQueryText(t *testing.T) {
	e := ElasticsearchSearch{
		index:             "idx",
		contentField:      "content",
		semanticTextField: "content_semantic",
	}

	for _, tc := range []struct {
		name  string
		query string
	}{
		{name: "plain", query: "higress gateway"},
		{name: "double quote", query: `He said "hello" and left`},
		{name: "backslash", query: `C:\temp\new`},
		{name: "newline", query: "line one\nline two"},
		{name: "unicode", query: "\u7f51\u5173"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := e.generateQueryBody(engine.SearchContext{EngineType: "private", Querys: []string{tc.query}})

			if !json.Valid([]byte(body)) {
				t.Fatalf("query body is not valid JSON:\n%s", body)
			}
			if got := gjson.Get(body, "retriever.rrf.retrievers.0.standard.query.match.content").String(); got != tc.query {
				t.Fatalf("match clause query = %q, want %q", got, tc.query)
			}
			if got := gjson.Get(body, "retriever.rrf.retrievers.1.standard.query.semantic.query").String(); got != tc.query {
				t.Fatalf("semantic query = %q, want %q", got, tc.query)
			}
		})
	}
}
