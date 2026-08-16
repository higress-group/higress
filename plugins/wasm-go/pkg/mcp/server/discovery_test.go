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
	"encoding/json"
	"reflect"
	"testing"

	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/protocol"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	wasmtest "github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestDiscoveryResultAdvertisesOnlyEffectiveProfilesAndCapabilities(t *testing.T) {
	semantic := DiscoveryResult(true)
	value := semantic.Value.(map[string]any)
	wantVersions := []string{"2024-11-05", "2025-03-26", "2025-06-18", "2026-07-28"}
	if got := value["supportedVersions"]; !reflect.DeepEqual(got, wantVersions) {
		t.Fatalf("supportedVersions = %#v, want %#v", got, wantVersions)
	}
	capabilities := value["capabilities"].(map[string]any)
	if len(capabilities) != 1 {
		t.Fatalf("capabilities = %#v", capabilities)
	}
	tools, ok := capabilities["tools"].(map[string]any)
	if !ok || len(tools) != 0 {
		t.Fatalf("tools capability = %#v", capabilities["tools"])
	}
	if semantic.ResultType != resultTypeComplete {
		t.Fatalf("resultType = %q", semantic.ResultType)
	}

	withoutTools := DiscoveryResult(false).Value.(map[string]any)["capabilities"].(map[string]any)
	if len(withoutTools) != 0 {
		t.Fatalf("unavailable tools advertised: %#v", withoutTools)
	}
}

func TestProductionDiscoveryAndToolsListResultContracts(t *testing.T) {
	savedGlobalContext := globalContext
	globalContext = Context{servers: make(map[string]Server)}
	Initialize()
	t.Cleanup(func() { globalContext = savedGlobalContext })

	config := json.RawMessage(`{
		"server":{"name":"catalog"},
		"tools":[
			{"name":"zeta","description":"z","requestTemplate":{"url":"/z","method":"GET"}},
			{
				"name":"alpha",
				"description":"a",
				"requestTemplate":{"url":"/a","method":"GET"},
				"outputSchema":{"type":"object","properties":{"answer":{"type":"string"}}}
			}
		]
	}`)
	newHost := func(t *testing.T) wasmtest.TestHost {
		t.Helper()
		host, status := wasmtest.NewTestHost(config)
		require.Equal(t, types.OnPluginStartStatusOK, status)
		t.Cleanup(host.Reset)
		return host
	}
	commonHeaders := func() [][2]string {
		return [][2]string{
			{":authority", "mcp.example.com"},
			{":method", "POST"},
			{":path", "/mcp"},
			{"content-type", "application/json"},
			{"accept", "application/json, text/event-stream"},
		}
	}
	modernExchange := func(t *testing.T, method, body string) map[string]any {
		t.Helper()
		host := newHost(t)
		headers := append(commonHeaders(),
			[2]string{protocol.HeaderProtocolVersion, string(protocol.Version20260728)},
			[2]string{protocol.HeaderMethod, method},
		)
		require.Equal(t, types.ActionPause, host.CallOnHttpRequestHeaders(headers))
		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody([]byte(body)))
		response := host.GetLocalResponse()
		require.NotNil(t, response)
		require.Equal(t, uint32(200), response.StatusCode)
		var envelope struct {
			Result map[string]any `json:"result"`
		}
		require.NoError(t, json.Unmarshal(response.Data, &envelope))
		return envelope.Result
	}
	modernBody := func(method string) string {
		return `{"jsonrpc":"2.0","id":1,"method":"` + method + `","params":{"_meta":{"` +
			protocol.MetaProtocolVersion + `":"2026-07-28","` + protocol.MetaClientCapabilities + `":{}}}}`
	}

	t.Run("modern discovery", func(t *testing.T) {
		result := modernExchange(t, "server/discover", modernBody("server/discover"))
		assertModernCompleteResult(t, result, "catalog", true)
		if got, want := stringSlice(result["supportedVersions"]), []string{"2024-11-05", "2025-03-26", "2025-06-18", "2026-07-28"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("supportedVersions = %v, want %v", got, want)
		}
		capabilities := result["capabilities"].(map[string]any)
		if len(capabilities) != 1 {
			t.Fatalf("capabilities = %#v", capabilities)
		}
		tools := capabilities["tools"].(map[string]any)
		if len(tools) != 0 {
			t.Fatalf("tools capability leaked deferred fields: %#v", tools)
		}
	})

	t.Run("modern deterministic tools list", func(t *testing.T) {
		result := modernExchange(t, "tools/list", modernBody("tools/list"))
		assertModernCompleteResult(t, result, "catalog", true)
		tools := result["tools"].([]any)
		if got, want := []string{tools[0].(map[string]any)["name"].(string), tools[1].(map[string]any)["name"].(string)}, []string{"alpha", "zeta"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("tool order = %v, want %v", got, want)
		}
		for _, tool := range tools {
			if _, exists := tool.(map[string]any)["outputSchema"]; exists {
				t.Fatalf("modern descriptor exposed unvalidated outputSchema: %#v", tool)
			}
		}
	})

	t.Run("proxy discovery advertises implemented tools", func(t *testing.T) {
		host, status := wasmtest.NewTestHost(json.RawMessage(`{
			"server":{
				"name":"proxy-only",
				"type":"mcp-proxy",
				"transport":"http",
				"mcpServerURL":"http://backend.example/mcp"
			}
		}`))
		require.Equal(t, types.OnPluginStartStatusOK, status)
		t.Cleanup(host.Reset)
		headers := append(commonHeaders(),
			[2]string{protocol.HeaderProtocolVersion, string(protocol.Version20260728)},
			[2]string{protocol.HeaderMethod, "server/discover"},
		)
		require.Equal(t, types.ActionPause, host.CallOnHttpRequestHeaders(headers))
		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody([]byte(modernBody("server/discover"))))
		response := host.GetLocalResponse()
		require.NotNil(t, response)
		var envelope struct {
			Result map[string]any `json:"result"`
		}
		require.NoError(t, json.Unmarshal(response.Data, &envelope))
		assertModernCompleteResult(t, envelope.Result, "proxy-only", true)
		capabilities := envelope.Result["capabilities"].(map[string]any)
		tools, ok := capabilities["tools"].(map[string]any)
		if !ok || len(tools) != 0 {
			t.Fatalf("proxy omitted implemented tools capability: %#v", capabilities)
		}
	})

	t.Run("legacy shapes unchanged", func(t *testing.T) {
		t.Run("tools list", func(t *testing.T) {
			host := newHost(t)
			require.Equal(t, types.ActionPause, host.CallOnHttpRequestHeaders(commonHeaders()))
			body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
			require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody(body))
			response := host.GetLocalResponse()
			require.NotNil(t, response)
			var envelope struct {
				Result struct {
					Tools []map[string]any `json:"tools"`
				} `json:"result"`
			}
			require.NoError(t, json.Unmarshal(response.Data, &envelope))
			var alpha map[string]any
			for _, tool := range envelope.Result.Tools {
				if tool["name"] == "alpha" {
					alpha = tool
					break
				}
			}
			require.NotNil(t, alpha)
			outputSchema, ok := alpha["outputSchema"].(map[string]any)
			require.True(t, ok, "legacy descriptor must retain outputSchema: %#v", alpha)
			require.Equal(t, "object", outputSchema["type"])
			for _, path := range []string{"result.resultType", "result._meta", "result.ttlMs", "result.cacheScope"} {
				if gjson.GetBytes(response.Data, path).Exists() {
					t.Fatalf("legacy tools/list gained %s: %s", path, response.Data)
				}
			}
		})

		t.Run("initialize", func(t *testing.T) {
			host := newHost(t)
			require.Equal(t, types.ActionPause, host.CallOnHttpRequestHeaders(commonHeaders()))
			body := []byte(`{"jsonrpc":"2.0","id":2,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`)
			require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody(body))
			response := host.GetLocalResponse()
			require.NotNil(t, response)
			require.Equal(t, "2025-06-18", gjson.GetBytes(response.Data, "result.protocolVersion").String())
			for _, path := range []string{"result.resultType", "result._meta", "result.ttlMs", "result.cacheScope"} {
				if gjson.GetBytes(response.Data, path).Exists() {
					t.Fatalf("legacy initialize gained %s: %s", path, response.Data)
				}
			}
		})

		t.Run("discovery unavailable", func(t *testing.T) {
			host := newHost(t)
			require.Equal(t, types.ActionPause, host.CallOnHttpRequestHeaders(commonHeaders()))
			body := []byte(`{"jsonrpc":"2.0","id":3,"method":"server/discover","params":{}}`)
			require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody(body))
			response := host.GetLocalResponse()
			require.NotNil(t, response)
			require.Equal(t, int64(protocol.CodeMethodNotFound), gjson.GetBytes(response.Data, "error.code").Int())
			if gjson.GetBytes(response.Data, "result").Exists() {
				t.Fatalf("legacy server/discover unexpectedly succeeded: %s", response.Data)
			}
		})
	})
}

func assertModernCompleteResult(t *testing.T, result map[string]any, serverName string, cacheFields bool) {
	t.Helper()
	if result["resultType"] != resultTypeComplete {
		t.Fatalf("resultType = %#v", result["resultType"])
	}
	metadata, ok := result["_meta"].(map[string]any)
	if !ok {
		t.Fatalf("_meta = %#v", result["_meta"])
	}
	serverInfo, ok := metadata[serverInfoMetaKey].(map[string]any)
	if !ok || serverInfo["name"] != serverName || serverInfo["version"] != serverImplementationVersion {
		t.Fatalf("serverInfo = %#v", metadata[serverInfoMetaKey])
	}
	if cacheFields {
		if result["ttlMs"] != float64(0) || result["cacheScope"] != cacheScopePrivate {
			t.Fatalf("cache wire fields = %#v", result)
		}
	}
}

func stringSlice(value any) []string {
	values := value.([]any)
	result := make([]string, len(values))
	for i, item := range values {
		result[i] = item.(string)
	}
	return result
}
