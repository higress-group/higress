// Copyright (c) 2026 Alibaba Group Holding Ltd.
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

package services

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPublishA2ACreatesServiceRoutePluginAndAttachment(t *testing.T) {
	type request struct {
		method string
		path   string
		body   map[string]interface{}
	}
	var requests []request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		requests = append(requests, request{method: r.Method, path: r.URL.Path, body: body})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{}}`))
	}))
	defer server.Close()

	if err := PublishA2A(NewHigressClient(server.URL, "admin", "secret"), "weather", "https://agent.example.com:8443/a2a", "https://gateway.example.com/a2a", "oci://registry.example.com/plugins/a2a-protocol:1.0.0"); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 6 {
		t.Fatalf("expected 6 requests, got %d", len(requests))
	}
	want := []struct{ method, path string }{
		{http.MethodPost, "/v1/service-sources"},
		{http.MethodPost, "/v1/routes"},
		{http.MethodPost, "/v1/routes"},
		{http.MethodPost, "/v1/wasm-plugins"},
		{http.MethodPut, "/v1/routes/weather-route/plugin-instances/a2a-protocol"},
		{http.MethodPut, "/v1/routes/weather-discovery-route/plugin-instances/a2a-protocol"},
	}
	for i := range want {
		if requests[i].method != want[i].method || requests[i].path != want[i].path {
			t.Fatalf("request %d: got %s %s", i, requests[i].method, requests[i].path)
		}
	}
	if requests[0].body["domain"] != "agent.example.com" || requests[0].body["port"] != "8443" {
		t.Fatalf("unexpected service: %#v", requests[0].body)
	}
	pathConfig := requests[1].body["path"].(map[string]interface{})
	if pathConfig["matchValue"] != "/a2a" {
		t.Fatalf("unexpected route: %#v", requests[1].body)
	}
	discoveryPath := requests[2].body["path"].(map[string]interface{})
	if discoveryPath["matchType"] != "EQUAL" || discoveryPath["matchValue"] != "/.well-known/agent-card.json" {
		t.Fatalf("unexpected discovery route: %#v", requests[2].body)
	}
	if requests[3].body["phase"] != "AUTHN" || requests[3].body["priority"] != float64(300) {
		t.Fatalf("unexpected plugin: %#v", requests[3].body)
	}
	if requests[3].body["imageRepository"] != "oci://registry.example.com/plugins/a2a-protocol" || requests[3].body["imageVersion"] != "1.0.0" {
		t.Fatalf("unexpected plugin image: %#v", requests[3].body)
	}
	config := requests[4].body["configurations"].(map[string]interface{})
	agent := config["agent"].(map[string]interface{})
	if agent["id"] != "weather" || agent["externalBaseURL"] != "https://gateway.example.com/a2a" {
		t.Fatalf("unexpected attachment: %#v", requests[4].body)
	}
}

func TestPublishA2AContinuesWhenPluginAlreadyExists(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/wasm-plugins" {
			http.Error(w, "exists", http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	if err := PublishA2A(NewHigressClient(server.URL, "", ""), "weather", "http://127.0.0.1:8080/", "https://gateway.example.com/", "oci://registry.example.com/a2a-protocol:1.0.0"); err != nil {
		t.Fatal(err)
	}
}

func TestBuildA2APublicationRejectsInvalidURL(t *testing.T) {
	if _, err := buildA2APublication("weather", "file:///tmp/agent", "https://gateway.example.com/a2a", "oci://registry.example.com/a2a-protocol:1.0.0"); err == nil {
		t.Fatal("expected invalid URL error")
	}
}

func TestBuildA2APublicationUsesConsoleStaticServiceContract(t *testing.T) {
	publication, err := buildA2APublication("weather", "http://127.0.0.1:8080/a2a", "https://gateway.example.com/a2a", "oci://registry.example.com/a2a-protocol:1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if publication.service["type"] != "static" || publication.service["domain"] != "127.0.0.1:8080" || publication.service["port"] != 80 {
		t.Fatalf("unexpected static service: %#v", publication.service)
	}
	if len(publication.routes) != 2 || publication.routes[1].route["name"] != "weather-discovery-route" {
		t.Fatalf("unexpected routes: %#v", publication.routes)
	}
}

func TestBuildA2APublicationRequiresTrustedExternalBaseURL(t *testing.T) {
	for _, externalBaseURL := range []string{
		"",
		"http://gateway.example.com/a2a",
		"https://user@gateway.example.com/a2a",
		"https://gateway.example.com/a2a#fragment",
		"https://gateway.example.com:0/a2a",
		"https://gateway.example.com:65536/a2a",
		"https://gateway.example.com:notaport/a2a",
		"https://127.0.0.1/a2a",
		"https://127.1/a2a",
		"https://gateway.localhost/a2a",
		"https://0.0.0.0/a2a",
		"https://10.0.0.1/a2a",
		"https://169.254.1.2/a2a",
		"https://224.0.0.1/a2a",
		"https://[::]/a2a",
		"https://[::1]/a2a",
		"https://[fd00::1]/a2a",
		"https://[ff02::1]/a2a",
	} {
		if _, err := buildA2APublication("weather", "http://127.0.0.1:8080/a2a", externalBaseURL, "oci://registry.example.com/a2a-protocol:1.0.0"); err == nil {
			t.Fatalf("expected external URL %q to be rejected", externalBaseURL)
		}
	}
}

func TestBuildA2APublicationRequiresExternalPathToMatchRoute(t *testing.T) {
	if _, err := buildA2APublication("weather", "http://agent.example.com/internal/a2a", "https://gateway.example.com/public/a2a", "oci://registry.example.com/a2a-protocol:1.0.0"); err == nil {
		t.Fatal("expected mismatched external and route paths to be rejected")
	}
	if _, err := buildA2APublication("weather", "http://agent.example.com/a2a/", "https://gateway.example.com/a2a", "oci://registry.example.com/a2a-protocol:1.0.0"); err != nil {
		t.Fatalf("expected equivalent route paths to be accepted: %v", err)
	}
}

func TestBuildA2APublicationRequiresExplicitOperatorPluginURL(t *testing.T) {
	for _, pluginURL := range []string{"", "https://registry.example.com/a2a:1.0.0", "oci://registry.example.com/a2a", "oci://registry.example.com/a2a@sha256:abc", "oci://user@registry.example.com/a2a:1.0.0"} {
		if _, err := buildA2APublication("weather", "http://agent.example.com/a2a", "https://gateway.example.com/a2a", pluginURL); err == nil {
			t.Fatalf("expected plugin URL %q to be rejected", pluginURL)
		}
	}
}
