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
)

type snapshotTestTool struct {
	description string
}

func (t *snapshotTestTool) Create(_ []byte) Tool               { return t }
func (t *snapshotTestTool) Call(_ HttpContext, _ Server) error { return nil }
func (t *snapshotTestTool) Description() string                { return t.description }
func (t *snapshotTestTool) InputSchema() map[string]any        { return map[string]any{"type": "object"} }

func TestBaseMCPServerPublishesImmutableSnapshots(t *testing.T) {
	server := NewBaseMCPServer()
	config := []byte(`{"generation":1}`)
	server.SetConfig(config)
	server.AddMCPTool("zeta", &snapshotTestTool{description: "z"})
	first := server.snapshot()

	config[len(config)-2] = '9'
	server.SetConfig([]byte(`{"generation":2}`))
	server.AddMCPTool("alpha", &snapshotTestTool{description: "a"})
	second := server.snapshot()

	if got := string(first.config); got != `{"generation":1}` {
		t.Fatalf("first snapshot config = %s", got)
	}
	if _, exists := first.tools["alpha"]; exists {
		t.Fatal("published tool mutation leaked into an in-flight snapshot")
	}
	if got := string(second.config); got != `{"generation":2}` {
		t.Fatalf("second snapshot config = %s", got)
	}
	if _, exists := second.tools["alpha"]; !exists {
		t.Fatal("subsequent snapshot did not observe published tool")
	}

	external := server.GetMCPTools()
	delete(external, "alpha")
	if _, exists := server.GetMCPTools()["alpha"]; !exists {
		t.Fatal("caller mutation changed the published snapshot")
	}
}

func TestSnapshotToolsIsDeterministicAndRequestScoped(t *testing.T) {
	server := NewBaseMCPServer()
	server.AddMCPTool("zeta", &snapshotTestTool{description: "z"})
	server.AddMCPTool("alpha", &snapshotTestTool{description: "a"})
	server.AddMCPTool("middle", &snapshotTestTool{description: "m"})

	first := snapshotTools(&server)
	repeated := snapshotTools(&server)
	server.AddMCPTool("later", &snapshotTestTool{description: "l"})
	second := snapshotTools(&server)

	names := func(snapshot []namedTool) []string {
		result := make([]string, len(snapshot))
		for i, entry := range snapshot {
			result[i] = entry.name
		}
		return result
	}
	if got, want := names(first), []string{"alpha", "middle", "zeta"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("first order = %v, want %v", got, want)
	}
	if got, want := names(repeated), names(first); !reflect.DeepEqual(got, want) {
		t.Fatalf("repeated order = %v, want %v", got, want)
	}
	if got, want := names(second), []string{"alpha", "later", "middle", "zeta"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("second order = %v, want %v", got, want)
	}
}
