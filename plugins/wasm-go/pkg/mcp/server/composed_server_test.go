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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubTool is a minimal Tool implementation for registry population in tests.
type stubTool struct {
	desc   string
	input  map[string]any
	output map[string]any
}

func (s *stubTool) Create(_ []byte) Tool                       { return s }
func (s *stubTool) Call(_ HttpContext, _ Server) error         { return nil }
func (s *stubTool) Description() string                        { return s.desc }
func (s *stubTool) InputSchema() map[string]any                { return s.input }
func (s *stubTool) OutputSchema() map[string]any               { return s.output }

func newPopulatedRegistry(t *testing.T) *GlobalToolRegistry {
	t.Helper()
	r := &GlobalToolRegistry{}
	r.Initialize()
	r.RegisterTool("alpha", "search", &stubTool{
		desc:   "alpha search",
		input:  map[string]any{"type": "object", "props": "a"},
		output: map[string]any{"type": "string"},
	})
	r.RegisterTool("alpha", "fetch", &stubTool{
		desc:   "alpha fetch",
		input:  map[string]any{"type": "object", "props": "f"},
		output: nil,
	})
	r.RegisterTool("beta", "search", &stubTool{
		desc:   "beta search",
		input:  map[string]any{"type": "object", "props": "bs"},
		output: map[string]any{"type": "array"},
	})
	return r
}

func TestComposedMCPServer_NewAndGetName(t *testing.T) {
	r := newPopulatedRegistry(t)
	cs := NewComposedMCPServer("myset", []ServerToolConfig{
		{ServerName: "alpha", Tools: []string{"search"}},
	}, r)
	require.NotNil(t, cs)
	assert.Equal(t, "myset", cs.GetName())
}

func TestComposedMCPServer_AddMCPTool_IsNoOp(t *testing.T) {
	r := newPopulatedRegistry(t)
	cs := NewComposedMCPServer("set", []ServerToolConfig{
		{ServerName: "alpha", Tools: []string{"search"}},
	}, r)

	// AddMCPTool should not panic and should be a no-op (tool not added).
	ret := cs.AddMCPTool("ignored", &stubTool{desc: "x"})
	assert.Same(t, cs, ret, "AddMCPTool should return the server itself")

	tools := cs.GetMCPTools()
	_, exists := tools["ignored"]
	assert.False(t, exists, "no-op AddMCPTool must not register the tool")
	// Only the one from registry should remain.
	_, found := tools["alpha___search"]
	assert.True(t, found, "registered tool should be present")
	assert.Len(t, tools, 1)
}

func TestComposedMCPServer_GetMCPTools_AggregatesWithPrefix(t *testing.T) {
	r := newPopulatedRegistry(t)
	cs := NewComposedMCPServer("compound", []ServerToolConfig{
		{ServerName: "alpha", Tools: []string{"search", "fetch"}},
		{ServerName: "beta", Tools: []string{"search"}},
	}, r)

	tools := cs.GetMCPTools()
	require.Len(t, tools, 3)

	// All keys must be prefixed with the original server name and the splitter.
	want := []string{"alpha___search", "alpha___fetch", "beta___search"}
	for _, k := range want {
		_, ok := tools[k]
		assert.True(t, ok, "expected composed tool key %q", k)
	}

	// Descriptions / input schemas are forwarded from the registry's ToolInfo.
	dt, ok := tools["alpha___search"].(*DescriptiveTool)
	require.True(t, ok)
	assert.Equal(t, "alpha search", dt.Description())
	assert.Equal(t, "a", dt.InputSchema()["props"])
	assert.Equal(t, "string", dt.OutputSchema()["type"])

	// Tool without OutputSchema in registry produces a DescriptiveTool with nil output.
	dt2, ok := tools["alpha___fetch"].(*DescriptiveTool)
	require.True(t, ok)
	assert.Nil(t, dt2.OutputSchema())
}

func TestComposedMCPServer_GetMCPTools_MissingToolIsSkipped(t *testing.T) {
	r := newPopulatedRegistry(t)
	cs := NewComposedMCPServer("set", []ServerToolConfig{
		{ServerName: "alpha", Tools: []string{"search", "nonexistent"}},
		{ServerName: "ghost", Tools: []string{"any"}}, // entire server missing
	}, r)

	tools := cs.GetMCPTools()
	// Only "alpha___search" survives; missing ones are logged and skipped.
	assert.Len(t, tools, 1)
	_, ok := tools["alpha___search"]
	assert.True(t, ok)
}

func TestComposedMCPServer_GetMCPTools_SameSimpleNameDifferentServersDoNotCollide(t *testing.T) {
	r := newPopulatedRegistry(t)
	cs := NewComposedMCPServer("set", []ServerToolConfig{
		{ServerName: "alpha", Tools: []string{"search"}},
		{ServerName: "beta", Tools: []string{"search"}},
	}, r)

	tools := cs.GetMCPTools()
	require.Len(t, tools, 2)
	assert.Contains(t, tools, "alpha___search")
	assert.Contains(t, tools, "beta___search")
}

func TestComposedMCPServer_GetMCPTools_EmptyConfig(t *testing.T) {
	r := newPopulatedRegistry(t)
	cs := NewComposedMCPServer("empty", nil, r)
	tools := cs.GetMCPTools()
	assert.NotNil(t, tools, "should return a non-nil empty map")
	assert.Empty(t, tools)
}

func TestComposedMCPServer_SetGetConfig_BytePointer(t *testing.T) {
	r := newPopulatedRegistry(t)
	cs := NewComposedMCPServer("set", nil, r)

	// Empty config: GetConfig must not modify the destination.
	var dst []byte
	dst = []byte("untouched")
	cs.GetConfig(&dst)
	assert.Equal(t, []byte("untouched"), dst, "GetConfig on empty config must be a no-op")

	// After SetConfig, byte-pointer destinations receive the stored bytes.
	cs.SetConfig([]byte(`{"k":"v"}`))
	var out []byte
	cs.GetConfig(&out)
	assert.Equal(t, []byte(`{"k":"v"}`), out)
}

func TestComposedMCPServer_GetConfig_UnhandledDestinationType(t *testing.T) {
	r := newPopulatedRegistry(t)
	cs := NewComposedMCPServer("set", nil, r)
	cs.SetConfig([]byte(`{"k":"v"}`))

	// Non-byte-pointer destinations are logged and left untouched (no panic).
	var s string = "untouched"
	cs.GetConfig(&s)
	assert.Equal(t, "untouched", s)

	type holder struct{ K string }
	h := holder{K: "untouched"}
	cs.GetConfig(&h)
	assert.Equal(t, "untouched", h.K)
}

func TestComposedMCPServer_Clone_IndependentConfig(t *testing.T) {
	r := newPopulatedRegistry(t)
	cs := NewComposedMCPServer("orig", []ServerToolConfig{
		{ServerName: "alpha", Tools: []string{"search"}},
	}, r)
	cs.SetConfig([]byte(`{"a":1}`))

	clonedI := cs.Clone()
	require.NotNil(t, clonedI)
	cloned, ok := clonedI.(*ComposedMCPServer)
	require.True(t, ok)
	assert.NotSame(t, cs, cloned, "Clone must return a new struct pointer")
	assert.Equal(t, cs.GetName(), cloned.GetName())

	// Confirm both see the same config initially.
	var origBytes, clonedBytes []byte
	cs.GetConfig(&origBytes)
	cloned.GetConfig(&clonedBytes)
	assert.Equal(t, origBytes, clonedBytes)

	// Mutating clone's config must not propagate to original.
	cloned.SetConfig([]byte(`{"a":2}`))
	cs.GetConfig(&origBytes)
	assert.Equal(t, []byte(`{"a":1}`), origBytes, "original config must remain unchanged after cloning")

	// Cloned still resolves tools through the shared registry.
	assert.Contains(t, cloned.GetMCPTools(), "alpha___search")
}

func TestDescriptiveTool_Create_ReturnsNewInstanceWithSameFields(t *testing.T) {
	dt := &DescriptiveTool{
		description:  "d",
		inputSchema:  map[string]any{"k": "v"},
		outputSchema: map[string]any{"o": "w"},
	}
	created := dt.Create([]byte(`{"ignored":true}`))
	require.NotNil(t, created)
	cdt, ok := created.(*DescriptiveTool)
	require.True(t, ok)
	assert.NotSame(t, dt, cdt, "Create must return a new instance")
	assert.Equal(t, dt.Description(), cdt.Description())
	assert.Equal(t, dt.InputSchema(), cdt.InputSchema())
	assert.Equal(t, dt.OutputSchema(), cdt.OutputSchema())
}

func TestDescriptiveTool_Call_ReturnsError(t *testing.T) {
	dt := &DescriptiveTool{description: "d"}
	err := dt.Call(nil, nil)
	require.Error(t, err, "DescriptiveTool.Call is a guard rail — must return an error")
}

func TestDescriptiveTool_Accessors(t *testing.T) {
	dt := &DescriptiveTool{
		description:  "desc",
		inputSchema:  map[string]any{"in": 1},
		outputSchema: map[string]any{"out": 2},
	}
	assert.Equal(t, "desc", dt.Description())
	assert.Equal(t, map[string]any{"in": 1}, dt.InputSchema())
	assert.Equal(t, map[string]any{"out": 2}, dt.OutputSchema())

	// Nil schemas must round-trip as nil.
	empty := &DescriptiveTool{}
	assert.Equal(t, "", empty.Description())
	assert.Nil(t, empty.InputSchema())
	assert.Nil(t, empty.OutputSchema())
}
