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

// Command host exposes the production mcp-server plugin through the wasm-go
// TestHost. It is intentionally test-only: official SDK clients use it as a
// real HTTP peer while every request still runs through the plugin callbacks.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	mcpplugin "github.com/alibaba/higress/plugins/wasm-go/pkg/mcp"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	wasmtest "github.com/higress-group/wasm-go/pkg/test"
	"github.com/tidwall/gjson"
)

const (
	modernVersion = "2026-07-28"
	legacyVersion = "2025-03-26"
)

var configs = map[string]json.RawMessage{
	"/direct": json.RawMessage(`{
		"server":{"name":"interop-direct","type":"rest"},
		"tools":[{
			"name":"get_weather",
			"description":"Return deterministic fixture weather",
			"args":[{"name":"location","description":"City name","type":"string","required":true}],
			"responseTemplate":{"body":"weather for {{.args.location}}"}
		}]
	}`),
	"/proxy-modern": json.RawMessage(`{
		"server":{"name":"interop-proxy-modern","type":"mcp-proxy","transport":"http","protocolStrategy":"modern","mcpServerURL":"http://fixture.invalid/mcp"}
	}`),
	"/proxy-legacy": json.RawMessage(`{
		"server":{"name":"interop-proxy-legacy","type":"mcp-proxy","transport":"http","protocolStrategy":"legacy","mcpServerURL":"http://fixture.invalid/mcp"}
	}`),
}

type pluginHandler struct {
	mu sync.Mutex
}

func main() {
	addr := flag.String("addr", "127.0.0.1:0", "listen address")
	readyFile := flag.String("ready-file", "", "write the endpoint root here after listening")
	flag.Parse()

	mcpplugin.LoadMCPServer(mcpplugin.AddMCPServer("interop-fixture", mcpplugin.NewMCPServer()))
	mcpplugin.InitMCPServer()
	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	root := "http://" + listener.Addr().String()
	if *readyFile != "" {
		if err := os.WriteFile(*readyFile, []byte(root), 0o600); err != nil {
			log.Fatal(err)
		}
	}
	log.Printf("MCP interoperability host listening on %s", root)
	log.Fatal(http.Serve(listener, &pluginHandler{}))
}

func (h *pluginHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	config, ok := configs[r.URL.Path]
	if !ok {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// The proxy-wasm test host owns process-global emulator state. Serialize
	// requests exactly as Envoy serializes callbacks on one HTTP stream.
	h.mu.Lock()
	defer h.mu.Unlock()

	host, status := wasmtest.NewTestHost(config)
	if status != types.OnPluginStartStatusOK {
		http.Error(w, "mcp-server test host failed to start", http.StatusInternalServerError)
		return
	}
	defer host.Reset()
	host.InitHttp()
	defer host.CompleteHttp()

	headers := [][2]string{
		{":authority", r.Host},
		{":method", r.Method},
		{":path", r.URL.RequestURI()},
	}
	for name, values := range r.Header {
		for _, value := range values {
			headers = append(headers, [2]string{name, value})
		}
	}
	host.CallOnHttpRequestHeaders(headers)
	host.CallOnHttpRequestBody(body)
	log.Printf("%s %s method=%s protocol=%s", r.Method, r.URL.Path, gjson.GetBytes(body, "method").String(), r.Header.Get("Mcp-Protocol-Version"))

	if err := completeFixtureCallouts(host); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writePluginResponse(w, host)
}

func completeFixtureCallouts(host wasmtest.TestHost) error {
	for step := 0; step < 4; step++ {
		callouts := host.GetHttpCalloutAttributes()
		if len(callouts) == 0 {
			return nil
		}
		callout := callouts[0]
		method := gjson.GetBytes(callout.Body, "method").String()
		id := gjson.GetBytes(callout.Body, "id").Raw
		if id == "" {
			id = "null"
		}

		var status string
		var response []byte
		switch method {
		case "initialize":
			status = "200"
			response = []byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":%q,"capabilities":{"tools":{}},"serverInfo":{"name":"fixture-legacy","version":"1.0.0"}}}`, id, legacyVersion))
		case "notifications/initialized":
			status = "202"
		case "tools/list":
			status = "200"
			result := `{"tools":[{"name":"get_weather","description":"Return deterministic fixture weather","inputSchema":{"type":"object","properties":{"location":{"type":"string"}},"required":["location"]}}]}`
			if hasHeader(callout.Headers, "Mcp-Protocol-Version", modernVersion) {
				result = `{"resultType":"complete","tools":[{"name":"get_weather","description":"Return deterministic fixture weather","inputSchema":{"type":"object","properties":{"location":{"type":"string"}},"required":["location"]}}],"ttlMs":0,"cacheScope":"private"}`
			}
			response = []byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"result":%s}`, id, result))
		case "tools/call":
			status = "200"
			location := gjson.GetBytes(callout.Body, "params.arguments.location").String()
			result := fmt.Sprintf(`{"content":[{"type":"text","text":%s}]}`, strconv.Quote("weather for "+location))
			if hasHeader(callout.Headers, "Mcp-Protocol-Version", modernVersion) {
				result = strings.TrimSuffix(result, "}") + `,"resultType":"complete"}`
			}
			response = []byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"result":%s}`, id, result))
		default:
			return fmt.Errorf("unexpected fixture callout method %q", method)
		}
		headers := [][2]string{{":status", status}}
		if len(response) > 0 {
			headers = append(headers, [2]string{"content-type", "application/json"})
		}
		host.CallOnHttpCallResponse(callout.CalloutID, headers, nil, response)
	}
	if len(host.GetHttpCalloutAttributes()) != 0 {
		return fmt.Errorf("fixture callout sequence exceeded four steps")
	}
	return nil
}

func hasHeader(headers [][2]string, name, value string) bool {
	for _, header := range headers {
		if strings.EqualFold(header[0], name) && header[1] == value {
			return true
		}
	}
	return false
}

func writePluginResponse(w http.ResponseWriter, host wasmtest.TestHost) {
	if response := host.GetLocalResponse(); response != nil {
		log.Printf("plugin response status=%d body=%s", response.StatusCode, response.Data)
		for _, header := range response.Headers {
			if !strings.HasPrefix(header[0], ":") {
				w.Header().Add(header[0], header[1])
			}
		}
		w.WriteHeader(int(response.StatusCode))
		_, _ = w.Write(response.Data)
		return
	}
	log.Printf("plugin streaming response body=%s", host.GetResponseBody())
	for _, header := range host.GetResponseHeaders() {
		if !strings.HasPrefix(header[0], ":") {
			w.Header().Add(header[0], header[1])
		}
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(host.GetResponseBody())
}
