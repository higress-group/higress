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

package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/consts"
	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/protocol"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	wasmtest "github.com/higress-group/wasm-go/pkg/test"
	"github.com/tidwall/gjson"
)

const routerTestConfig = `{
	"servers":[
		{"name":"source","domain":"source.example.test","path":"/source"},
		{"name":"source","path":"/shadow"},
		{"name":"empty","path":""},
		{"name":"empty","path":"/shadow"}
	],
	"_rules_":[
		{"_match_domain_":["virtual.example.test"],"enable":true},
		{"_match_domain_":["disabled.example.test"],"enable":false}
	]
}`

func routerHost(t *testing.T, domain, config string) wasmtest.TestHost {
	t.Helper()
	host, status := wasmtest.NewTestHost(json.RawMessage(config))
	t.Cleanup(host.Reset)
	if status != types.OnPluginStartStatusOK {
		t.Fatalf("plugin start status = %v", status)
	}
	host.InitHttp()
	t.Cleanup(host.CompleteHttp)
	if err := host.SetDomainName(domain); err != nil {
		t.Fatal(err)
	}
	return host
}

func routerRequestHeaders(domain string) [][2]string {
	return [][2]string{
		{":method", "POST"}, {":authority", domain}, {":path", "/virtual"},
		{"content-type", "application/json"}, {"accept", "application/json, text/event-stream"},
	}
}

func TestRouterToolDispatchSignalFollowsEnabledRule(t *testing.T) {
	const enabled = "virtual.example.test"
	const emptyRoutes = `{"servers":[],"_rules_":[{"_match_domain_":["virtual.example.test"],"enable":true}]}`
	tests := []struct {
		name, domain, config, method string
		want                         bool
	}{
		{name: "enabled", domain: enabled, config: routerTestConfig, method: "server/discover", want: true},
		{name: "enabled with no sources", domain: enabled, config: emptyRoutes, method: "server/discover", want: true},
		{name: "disabled", domain: "disabled.example.test", config: routerTestConfig, method: "server/discover"},
		{name: "other method", domain: enabled, config: routerTestConfig, method: "tools/list"},
	}
	wasmtest.RunTest(t, func(t *testing.T) {
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				host := routerHost(t, tc.domain, tc.config)
				warnings := len(host.GetWarnLogs())
				host.CallOnHttpRequestHeaders(routerRequestHeaders(tc.domain))
				host.CallOnHttpRequestBody([]byte(`{"jsonrpc":"2.0","id":1,"method":` + strconv.Quote(tc.method) + `,"params":{}}`))
				if got := host.GetWarnLogs()[warnings:]; len(got) != 0 {
					t.Fatalf("unexpected warnings: %q", got)
				}
				raw, err := host.GetProperty([]string{consts.MCPRouterToolDispatchEnabled})
				if tc.want {
					if err != nil || string(raw) != "true" {
						t.Fatalf("dispatch state = %q, error = %v", raw, err)
					}
				} else if len(raw) != 0 {
					t.Fatalf("unexpected dispatch state %q", raw)
				}
			})
		}
	})
}

func TestRouterWarnsWhenToolDispatchFlagWriteFails(t *testing.T) {
	const domain = "virtual.example.test"
	host := routerHost(t, domain, routerTestConfig)
	warnings := len(host.GetWarnLogs())
	writes := 0
	rejectProperty := func(path []string, value []byte) error {
		writes++
		if len(path) != 1 || path[0] != consts.MCPRouterToolDispatchEnabled || string(value) != "true" {
			t.Fatalf("unexpected property write: %q = %q", path, value)
		}
		return types.ErrorInternalFailure
	}
	action := markToolDispatchEnabled(McpRouterConfig{enable: true}, "server/discover", rejectProperty)
	if action != types.ActionContinue || writes != 1 {
		t.Fatalf("want one attempt and ActionContinue, got %d attempts and action %v", writes, action)
	}
	raw, err := host.GetProperty([]string{consts.MCPRouterToolDispatchEnabled})
	if len(raw) != 0 || !errors.Is(err, types.ErrorStatusNotFound) {
		t.Fatalf("failed write left property %q, error = %v", raw, err)
	}
	got := host.GetWarnLogs()[warnings:]
	if len(got) != 1 || !strings.Contains(got[0], "Failed to set mcp-router tool dispatch flag") ||
		!strings.Contains(got[0], types.ErrorInternalFailure.Error()) {
		t.Fatalf("want one dispatch write warning with the host error, got %q", got)
	}
	t.Logf("ActionContinue, no dispatch flag, warning: %s", got[0])
}

func TestRouterStopsRewritingOnHeaderFailure(t *testing.T) {
	target := &ServerConfig{Domain: "source.example.test", Path: "/source"}
	headers := []string{":authority", "x-envoy-internal-route", ":path", protocol.HeaderName}
	rejected := errors.New("test host rejected the header mutation")
	for failAt, header := range headers {
		t.Run(header, func(t *testing.T) {
			written := 0
			err := forwardToolCall(target, "echo", nil, func(key, _ string) error {
				if key != headers[written] {
					t.Fatalf("header %d = %q, want %q", written, key, headers[written])
				}
				written++
				if key == header {
					return rejected
				}
				return nil
			})
			if !errors.Is(err, rejected) || written != failAt+1 {
				t.Fatalf("header mutation progressed after failure: written = %d, error = %v", written, err)
			}
		})
	}
}

func TestRouterRewritesOnlyValidatedModernToolIdentity(t *testing.T) {
	const meta = `"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{},"example.test/context":{"key":"preserved"}},`
	const bodyStart = `{"jsonrpc":"2.0","id":9007199254740993,"method":"tools/call","params":{`
	const bodyEnd = `,"arguments":{"value":9007199254740993,"nested":[true,null]}}}`
	encoded := func(name string) string {
		return "=?base64?" + base64.StdEncoding.EncodeToString([]byte(name)) + "?="
	}
	tests := []struct {
		label, name, header, wantHeader string
		meta, extra                     string
		wantError                       int64
		duplicateHeader                 bool
		unsupportedVersion              bool
		omitVersion, omitMethod         bool
		wantNullID                      bool
	}{
		{label: "modern", name: "source___echo", header: "source___echo", wantHeader: "echo", meta: meta},
		{label: "unicode", name: "source___回声", header: encoded("source___回声"), wantHeader: encoded("回声"), meta: meta},
		{label: "whitespace", name: "source___ echo", header: "source___ echo", wantHeader: encoded(" echo"), meta: meta},
		{label: "legacy", name: "source___echo"},
		{label: "mismatch before rewrite", name: "source___echo", header: "echo", meta: meta, wantError: protocol.CodeHeaderMismatch},
		{label: "duplicate header", name: "source___echo", header: "source___echo", meta: meta, wantError: protocol.CodeHeaderMismatch, duplicateHeader: true, wantNullID: true},
		{label: "unsupported version", name: "source___echo", header: "source___echo", meta: meta, wantError: protocol.CodeUnsupportedVersion, unsupportedVersion: true, wantNullID: true},
		{label: "missing version", name: "source___echo", header: "source___echo", meta: meta, wantError: protocol.CodeHeaderMismatch, omitVersion: true, wantNullID: true},
		{label: "missing method", name: "source___echo", header: "source___echo", meta: meta, wantError: protocol.CodeHeaderMismatch, omitMethod: true, wantNullID: true},
		{label: "duplicate body name", name: "source___echo", header: "source___echo", meta: meta, extra: `"name":"second",`, wantError: protocol.CodeParseError},
		{label: "metadata without headers", name: "source___echo", meta: meta, wantError: protocol.CodeHeaderMismatch},
		{label: "empty target tool", name: "source___", header: "source___", meta: meta, wantError: protocol.CodeInvalidParams},
	}
	wasmtest.RunTest(t, func(t *testing.T) {
		for _, tc := range tests {
			t.Run(tc.label, func(t *testing.T) {
				const domain = "virtual.example.test"
				host := routerHost(t, domain, routerTestConfig)
				headers := routerRequestHeaders(domain)
				if tc.header != "" {
					version := string(protocol.Version20260728)
					if tc.unsupportedVersion {
						version = "1900-01-01"
					}
					if !tc.omitVersion {
						headers = append(headers, [2]string{protocol.HeaderProtocolVersion, version})
					}
					if !tc.omitMethod {
						headers = append(headers, [2]string{protocol.HeaderMethod, "tools/call"})
					}
					headers = append(headers, [2]string{protocol.HeaderName, tc.header})
					if tc.duplicateHeader {
						headers = append(headers, [2]string{"mcp-name", tc.header})
					}
				}
				body := bodyStart + tc.meta + `"name":` + strconv.Quote(tc.name) + "," + tc.extra + strings.TrimPrefix(bodyEnd, ",")
				host.CallOnHttpRequestHeaders(headers)
				host.CallOnHttpRequestBody([]byte(body))
				var authority, path, nameHeader, protocolVersion, methodHeader string
				for _, h := range host.GetRequestHeaders() {
					switch strings.ToLower(h[0]) {
					case ":authority":
						authority = h[1]
					case ":path":
						path = h[1]
					case "mcp-name":
						nameHeader = h[1]
					case "mcp-protocol-version":
						protocolVersion = h[1]
					case "mcp-method":
						methodHeader = h[1]
					}
				}
				response := host.GetLocalResponse()
				if tc.wantError != 0 {
					if response == nil || response.StatusCode != 400 || gjson.GetBytes(response.Data, "error.code").Int() != tc.wantError {
						t.Fatalf("want HTTP 400/RPC %d, got %+v", tc.wantError, response)
					}
					wantID := "9007199254740993"
					if tc.wantNullID {
						wantID = "null"
					}
					if got := gjson.GetBytes(response.Data, "id").Raw; got != wantID {
						t.Fatalf("error ID = %s, want %s", got, wantID)
					}
					if authority != domain || path != "/virtual" || string(host.GetRequestBody()) != body {
						t.Fatal("invalid client identity changed the route or body")
					}
					return
				}
				if response != nil || authority != "source.example.test" || path != "/source" || nameHeader != tc.wantHeader {
					t.Fatalf("route = %s%s, name header = %q, response = %+v", authority, path, nameHeader, response)
				}
				if tc.header != "" && (protocolVersion != string(protocol.Version20260728) || methodHeader != "tools/call") ||
					tc.header == "" && (protocolVersion != "" || methodHeader != "") {
					t.Fatalf("protocol identity changed: version = %q, method = %q", protocolVersion, methodHeader)
				}
				_, sourceName, _ := strings.Cut(tc.name, consts.ToolSetNameSplitter)
				wantBody := bodyStart + tc.meta + `"name":` + strconv.Quote(sourceName) + bodyEnd
				if got := string(host.GetRequestBody()); got != wantBody {
					t.Fatalf("body = %s, want %s", got, wantBody)
				}
			})
		}
	})
}
