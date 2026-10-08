// Copyright (c) 2022 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"

	mcpplugin "github.com/alibaba/higress/plugins/wasm-go/pkg/mcp"
	wasmtest "github.com/higress-group/wasm-go/pkg/test"
	"github.com/tidwall/gjson"
)

func TestMain(m *testing.M) {
	mcpplugin.LoadMCPServer(mcpplugin.AddMCPServer("interop-fixture", mcpplugin.NewMCPServer()))
	mcpplugin.InitMCPServer()
	os.Exit(m.Run())
}

// Deliberately make the error fixture advertise success. The unchanged plugin
// then emits a real tools/call hostcall, which the fixture's error-path oracle
// must reject. No synthetic entry is inserted into its observed callout ledger.
type unexpectedBusinessHost struct {
	wasmtest.TestHost
	businessCalls int
}

func (h *unexpectedBusinessHost) CallOnHttpCallResponse(id uint32, headers, trailers [][2]string, body []byte) {
	for _, callout := range h.GetHttpCalloutAttributes() {
		if callout.CalloutID != id {
			continue
		}
		switch gjson.GetBytes(callout.Body, "method").String() {
		case "server/discover":
			headers = [][2]string{{":status", "200"}, {"content-type", "application/json"}}
			body = []byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"result":{"resultType":"complete","supportedVersions":["2026-07-28"],"capabilities":{"tools":{}},"ttlMs":0,"cacheScope":"private","_meta":{"io.modelcontextprotocol/serverInfo":{"name":"negative-oracle","version":"1"}}}}`, gjson.GetBytes(callout.Body, "id").Raw))
		case "tools/call":
			h.businessCalls++
		}
	}
	h.TestHost.CallOnHttpCallResponse(id, headers, trailers, body)
}

func runProbeChecker(t *testing.T, root string, wantExit int, wantMessage string) {
	t.Helper()
	output, err := exec.Command("python3", "../check_probe.py", root).CombinedOutput()
	exit := 0
	if err != nil {
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) {
			t.Fatal(err)
		}
		exit = exitError.ExitCode()
	}
	if exit != wantExit || !strings.Contains(string(output), wantMessage) {
		t.Fatalf("probe checker exit %d, want %d; output: %s", exit, wantExit, output)
	}
}

func TestProbeCheckerAcceptsExpectedError(t *testing.T) {
	server := httptest.NewServer(&pluginHandler{})
	defer server.Close()
	runProbeChecker(t, server.URL, 0, "no fixture failures")
}

func TestProbeCheckerRejectsEarlierUnexpectedBusinessCallout(t *testing.T) {
	var injected *unexpectedBusinessHost
	handler := &pluginHandler{completeCallouts: func(host wasmtest.TestHost, path, profile, business string) error {
		injected = &unexpectedBusinessHost{TestHost: host}
		return completeFixtureCallouts(injected, path, profile, business)
	}}
	server := httptest.NewServer(handler)
	defer server.Close()
	body := `{"jsonrpc":"2.0","id":"negative-oracle","method":"tools/call","params":{"name":"get_weather","arguments":{"location":"New York"},"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientInfo":{"name":"negative-oracle","version":"1"},"io.modelcontextprotocol/clientCapabilities":{}}}}`
	request, err := http.NewRequest(http.MethodPost, server.URL+"/proxy-auto-error", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json,text/event-stream")
	request.Header.Set("MCP-Protocol-Version", modernVersion)
	request.Header.Set("Mcp-Method", "tools/call")
	request.Header.Set("Mcp-Name", "get_weather")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	responseBody, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 500 || !strings.Contains(string(responseBody), "[server/discover tools/call]") {
		t.Fatalf("extra business was not rejected: status=%d body=%s", response.StatusCode, responseBody)
	}
	handler.mu.Lock()
	actualBusinessCalls := injected.businessCalls
	handler.completeCallouts = nil
	handler.mu.Unlock()
	if actualBusinessCalls != 1 {
		t.Fatalf("negative test did not exercise a real business callout: %d", actualBusinessCalls)
	}
	// The next probe is valid. The independent, sticky verdict must still fail
	// the same checker run.sh uses, even if an SDK swallowed the earlier 500.
	runProbeChecker(t, server.URL, 42, "interop fixture recorded a failure")
}
