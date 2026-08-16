// Copyright (c) 2022 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package server

import (
	"reflect"
	"testing"

	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/protocol"
)

func modernResultRequest(method string) *protocol.RequestContext {
	return &protocol.RequestContext{
		Era:     protocol.EraModern,
		Version: protocol.Version20260728,
		Envelope: protocol.Envelope{
			Method: method,
		},
	}
}

func TestShapeResultAddsModernContractWithoutMutatingSemanticValue(t *testing.T) {
	value := map[string]any{
		"tools": []any{},
		"_meta": map[string]any{"vendor.example/trace": "opaque"},
	}
	semantic := protocol.SemanticResult{
		Value:      value,
		ResultType: resultTypeComplete,
		Meta:       map[string]any{"vendor.example/feature": true},
	}
	shaped := ShapeResult(modernResultRequest("tools/list"), "catalog", semantic)

	if shaped["resultType"] != resultTypeComplete || shaped["ttlMs"] != 0 || shaped["cacheScope"] != cacheScopePrivate {
		t.Fatalf("modern wire fields = %+v", shaped)
	}
	metadata, ok := shaped["_meta"].(map[string]any)
	if !ok {
		t.Fatalf("_meta = %#v", shaped["_meta"])
	}
	serverInfo, ok := metadata[serverInfoMetaKey].(map[string]any)
	if !ok || serverInfo["name"] != "catalog" || serverInfo["version"] != serverImplementationVersion {
		t.Fatalf("serverInfo = %#v", metadata[serverInfoMetaKey])
	}
	if metadata["vendor.example/trace"] != "opaque" || metadata["vendor.example/feature"] != true {
		t.Fatalf("extension metadata not preserved: %#v", metadata)
	}
	if _, exists := value["resultType"]; exists {
		t.Fatal("semantic value was mutated")
	}
	if len(value["_meta"].(map[string]any)) != 1 {
		t.Fatal("semantic metadata was mutated")
	}
}

func TestShapeResultKeepsCacheWireFieldsMethodScoped(t *testing.T) {
	shaped := ShapeResult(modernResultRequest("tools/call"), "catalog", protocol.SemanticResult{
		Value: map[string]any{"content": []any{}},
	})
	if shaped["resultType"] != resultTypeComplete {
		t.Fatalf("default resultType = %#v", shaped["resultType"])
	}
	if _, exists := shaped["ttlMs"]; exists {
		t.Fatal("tools/call unexpectedly received ttlMs")
	}
	if _, exists := shaped["cacheScope"]; exists {
		t.Fatal("tools/call unexpectedly received cacheScope")
	}
}

func TestShapeResultLeavesLegacyResultUnchanged(t *testing.T) {
	legacy := map[string]any{
		"tools": []any{map[string]any{"name": "echo"}},
	}
	want := map[string]any{
		"tools": []any{map[string]any{"name": "echo"}},
	}
	got := ShapeResult(&protocol.RequestContext{Era: protocol.EraLegacy}, "catalog", protocol.SemanticResult{
		Value:      legacy,
		ResultType: resultTypeComplete,
	})
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("legacy result = %#v, want %#v", got, want)
	}
	for _, forbidden := range []string{"resultType", "_meta", "ttlMs", "cacheScope"} {
		if _, exists := got[forbidden]; exists {
			t.Fatalf("legacy result gained %q", forbidden)
		}
	}
}
