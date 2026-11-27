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
	"encoding/base64"
	"reflect"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/proxytest"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/stretchr/testify/require"
)

type snapshotTestTool struct {
	description string
}

type baseServerTestPluginContext struct {
	types.DefaultPluginContext
}

func (*baseServerTestPluginContext) NewHttpContext(uint32) types.HttpContext {
	return &types.DefaultHttpContext{}
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

func TestBaseMCPServerGetConfigUsesStoredConfigAndStripsRequestHeaders(t *testing.T) {
	tests := []struct {
		name    string
		headers [][2]string
	}{
		{
			name: "public header",
			headers: [][2]string{
				{"x-higress-mcpserver-config", base64.StdEncoding.EncodeToString([]byte(`{"source":"public"}`))},
				{"x-forwarded-header", "preserve"},
			},
		},
		{
			name: "internal header",
			headers: [][2]string{
				{"x-envoy-internal-mcpserver-config", base64.StdEncoding.EncodeToString([]byte(`{"source":"internal"}`))},
				{"x-forwarded-header", "preserve"},
			},
		},
		{
			name: "both headers",
			headers: [][2]string{
				{"x-higress-mcpserver-config", base64.StdEncoding.EncodeToString([]byte(`{"source":"public"}`))},
				{"x-envoy-internal-mcpserver-config", base64.StdEncoding.EncodeToString([]byte(`{"source":"internal"}`))},
				{"x-forwarded-header", "preserve"},
			},
		},
		{
			name: "no config headers",
			headers: [][2]string{
				{"x-forwarded-header", "preserve"},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			opt := proxytest.NewEmulatorOption().WithVMContext(&types.DefaultVMContext{}).WithPluginContext(
				func(uint32) types.PluginContext { return &baseServerTestPluginContext{} },
			)
			host, reset := proxytest.NewHostEmulator(opt)
			defer reset()
			require.Equal(t, types.OnPluginStartStatusOK, host.StartPlugin())
			contextID := host.InitializeHttpContext()
			host.CallOnRequestHeaders(contextID, test.headers, true)

			server := NewBaseMCPServer()
			server.SetConfig([]byte(`{"source":"stored"}`))
			var config struct {
				Source string `json:"source"`
			}
			server.GetConfig(&config)

			require.Equal(t, "stored", config.Source)
			forwardedHeaders := host.GetCurrentRequestHeaders(contextID)
			require.Contains(t, forwardedHeaders, [2]string{"x-forwarded-header", "preserve"})
			for _, header := range forwardedHeaders {
				require.NotEqual(t, "x-higress-mcpserver-config", header[0])
				require.NotEqual(t, "x-envoy-internal-mcpserver-config", header[0])
			}
		})
	}
}
