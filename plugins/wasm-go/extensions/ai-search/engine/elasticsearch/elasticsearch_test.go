// Copyright (c) 2022 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package elasticsearch

import (
	"encoding/json"
	"testing"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-search/engine"
	"github.com/tidwall/gjson"
)

func newTestEngine() ElasticsearchSearch {
	return ElasticsearchSearch{
		index:             "test-index",
		contentField:      "content",
		semanticTextField: "semantic_text_field",
	}
}

func TestGenerateQueryBodyIsAlwaysValidJSON(t *testing.T) {
	es := newTestEngine()

	for _, tc := range []struct {
		name   string
		querys []string
	}{
		{name: "plain", querys: []string{"Tell me a joke"}},
		{name: "double quote", querys: []string{`He said "hello" and left`}},
		{name: "backslash", querys: []string{`C:\temp\new`}},
		{name: "newline", querys: []string{"line one\nline two"}},
		{name: "quote and backslash", querys: []string{`"a"\` + "\n" + `"b"`}},
		{name: "empty query", querys: []string{""}},
		{name: "multiple queries joined", querys: []string{"alpha", `beta "quoted"`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := es.generateQueryBody(engine.SearchContext{Querys: tc.querys})

			if !json.Valid([]byte(body)) {
				t.Fatalf("query body is not valid JSON:\n%s", body)
			}
			joined := ""
			if len(tc.querys) > 0 {
				for i, q := range tc.querys {
					if i > 0 {
						joined += " "
					}
					joined += q
				}
			}
			if got := gjson.Get(body, "retriever.rrf.retrievers.0.standard.query.match.content").String(); got != joined {
				t.Errorf("match query = %q, want %q", got, joined)
			}
			if got := gjson.Get(body, "retriever.rrf.retrievers.1.standard.query.semantic.query").String(); got != joined {
				t.Errorf("semantic query = %q, want %q", got, joined)
			}
			if got := gjson.Get(body, "retriever.rrf.retrievers.1.standard.query.semantic.field").String(); got != "semantic_text_field" {
				t.Errorf("semantic field = %q, want %q", got, "semantic_text_field")
			}
			if got := gjson.Get(body, "_source.excludes").String(); got != "content" {
				t.Errorf("_source.excludes = %q, want %q", got, "content")
			}
		})
	}
}

// A crafted query must not be able to inject extra JSON-RPC/ES query DSL
// clauses: the whole document must contain exactly one retriever with the two
// expected sub-retrievers and nothing else.
func TestGenerateQueryBodyCannotInjectTopLevelClauses(t *testing.T) {
	es := newTestEngine()
	malicious := []string{`" , "bool" : {"should" : [{}]}}`}
	body := es.generateQueryBody(engine.SearchContext{Querys: malicious})

	if !json.Valid([]byte(body)) {
		t.Fatalf("query body is not valid JSON:\n%s", body)
	}
	var doc map[string]interface{}
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatalf("query body does not unmarshal to a single object: %v", err)
	}
	if len(doc) != 2 {
		t.Fatalf("top-level keys = %v, want only [_source retriever]", doc)
	}
	if _, ok := doc["bool"]; ok {
		t.Fatalf("injected top-level clause survived: %s", body)
	}
	if retrievers := gjson.Get(body, "retriever.rrf.retrievers").Array(); len(retrievers) != 2 {
		t.Fatalf("retriever count = %d, want 2", len(retrievers))
	}
}
