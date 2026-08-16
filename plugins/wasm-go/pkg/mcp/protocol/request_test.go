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

package protocol

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func modernTransport(method, name string) Transport {
	return Transport{
		Method:          "POST",
		Authority:       "mcp.example.com",
		ContentType:     "application/json; charset=utf-8",
		Accept:          "application/json, text/event-stream",
		ProtocolVersion: string(Version20260728),
		MCPMethod:       method,
		MCPName:         name,
	}
}

func modernTransportFromHeaders(method, name string) Transport {
	return NewTransport("POST", "mcp.example.com", [][2]string{
		{"Content-Type", "application/json; charset=utf-8"},
		{"Accept", "application/json, text/event-stream"},
		{HeaderProtocolVersion, string(Version20260728)},
		{HeaderMethod, method},
		{HeaderName, name},
	})
}

func modernCallRequestWithName(name string) string {
	encodedName, err := json.Marshal(name)
	if err != nil {
		panic(err)
	}
	return strings.Replace(modernCallRequest, `"name":"echo"`, `"name":`+string(encodedName), 1)
}

func available(method string) bool {
	switch method {
	case "server/discover", "tools/list", "tools/call":
		return true
	default:
		return false
	}
}

const modernListRequest = `{
  "jsonrpc":"2.0",
  "id":"request-1",
  "method":"tools/list",
  "params":{"_meta":{
    "io.modelcontextprotocol/protocolVersion":"2026-07-28",
    "io.modelcontextprotocol/clientCapabilities":{}
  }}
}`

const modernCallRequest = `{
  "jsonrpc":"2.0",
  "id":7,
  "method":"tools/call",
  "params":{
    "name":"echo",
    "arguments":{"text":"hello"},
    "_meta":{
      "io.modelcontextprotocol/protocolVersion":"2026-07-28",
      "io.modelcontextprotocol/clientCapabilities":{"tools":{}},
      "io.modelcontextprotocol/clientInfo":{"name":"tier-one-sdk","version":"1.2.3"}
    }
  }
}`

const modernListNotification = `{
  "jsonrpc":"2.0",
  "method":"tools/list",
  "params":{"_meta":{
    "io.modelcontextprotocol/protocolVersion":"2026-07-28",
    "io.modelcontextprotocol/clientCapabilities":{}
  }}
}`

func TestPrepareRequestAcceptsModernRequestAndNotification(t *testing.T) {
	tests := []struct {
		name           string
		transport      Transport
		body           string
		notification   bool
		wantClientInfo bool
		wantMethod     string
		wantID         string
	}{
		{
			name:       "stateless request without optional clientInfo",
			transport:  modernTransport("tools/list", ""),
			body:       modernListRequest,
			wantMethod: "tools/list",
			wantID:     `"request-1"`,
		},
		{
			name:           "tool request with clientInfo",
			transport:      modernTransport("tools/call", "echo"),
			body:           modernCallRequest,
			wantClientInfo: true,
			wantMethod:     "tools/call",
			wantID:         "7",
		},
		{
			name:         "valid notification",
			transport:    modernTransport("tools/list", ""),
			body:         modernListNotification,
			notification: true,
			wantMethod:   "tools/list",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lookups := 0
			request, protocolError := PrepareRequest(test.transport, []byte(test.body), func(method string) bool {
				lookups++
				return available(method)
			})
			if protocolError != nil {
				t.Fatalf("PrepareRequest() error = %+v", protocolError)
			}
			if request.Era != EraModern || request.Version != Version20260728 {
				t.Fatalf("unexpected profile: %+v", request)
			}
			if request.Envelope.Method != test.wantMethod || request.Envelope.Notification != test.notification {
				t.Fatalf("unexpected envelope: %+v", request.Envelope)
			}
			if got := string(request.Envelope.ID.Raw()); got != test.wantID {
				t.Fatalf("ID = %q, want %q", got, test.wantID)
			}
			if (request.Metadata.ClientInfo != nil) != test.wantClientInfo {
				t.Fatalf("ClientInfo = %+v", request.Metadata.ClientInfo)
			}
			if request.Cancellation == nil {
				t.Fatal("modern request must have request-scoped cancellation")
			}
			if lookups != 1 {
				t.Fatalf("method availability lookups = %d, want 1", lookups)
			}
		})
	}
}

func TestPrepareRequestRejectsModernBoundaryViolationsBeforeMethodLookup(t *testing.T) {
	oversized := append([]byte(modernListRequest), bytes.Repeat([]byte{' '}, int(ModernMaxBodyBytes))...)
	tests := []struct {
		name      string
		transport Transport
		body      string
		bodyBytes []byte
		wantHTTP  uint32
		wantCode  int
	}{
		{"batch", modernTransport("tools/list", ""), `[` + modernListRequest + `]`, nil, 400, CodeInvalidRequest},
		{"response envelope", modernTransport("tools/list", ""), `{"jsonrpc":"2.0","id":1,"result":{}}`, nil, 400, CodeInvalidRequest},
		{"trailing JSON", modernTransport("tools/list", ""), modernListRequest + `{}`, nil, 400, CodeParseError},
		{"malformed JSON", modernTransport("tools/list", ""), `{"jsonrpc":`, nil, 400, CodeParseError},
		{"null id", modernTransport("tools/list", ""), strings.Replace(modernListRequest, `"id":"request-1"`, `"id":null`, 1), nil, 400, CodeInvalidRequest},
		{"fractional id", modernTransport("tools/list", ""), strings.Replace(modernListRequest, `"id":"request-1"`, `"id":1.5`, 1), nil, 400, CodeInvalidRequest},
		{"boolean id", modernTransport("tools/list", ""), strings.Replace(modernListRequest, `"id":"request-1"`, `"id":true`, 1), nil, 400, CodeInvalidRequest},
		{"duplicate identity", modernTransport("tools/list", ""), strings.Replace(modernListRequest, `"method":"tools/list"`, `"method":"tools/list","method":"tools/list"`, 1), nil, 400, CodeParseError},
		{"oversized body", modernTransport("tools/list", ""), "", oversized, 413, CodeInvalidRequest},
		{"missing protocol header", func() Transport { v := modernTransport("tools/list", ""); v.ProtocolVersion = ""; return v }(), modernListRequest, nil, 400, CodeHeaderMismatch},
		{"legacy protocol header cannot downgrade modern body", func() Transport {
			v := modernTransport("tools/list", "")
			v.ProtocolVersion = string(Version20250618)
			return v
		}(), modernListRequest, nil, 400, CodeHeaderMismatch},
		{"mismatched method header", func() Transport { v := modernTransport("ping", ""); return v }(), modernListRequest, nil, 400, CodeHeaderMismatch},
		{"missing name header", modernTransport("tools/call", ""), modernCallRequest, nil, 400, CodeHeaderMismatch},
		{"mismatched name header", modernTransport("tools/call", "other"), modernCallRequest, nil, 400, CodeHeaderMismatch},
		{"unexpected name header", modernTransport("tools/list", "echo"), modernListRequest, nil, 400, CodeHeaderMismatch},
		{"missing metadata", modernTransport("tools/list", ""), `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`, nil, 400, CodeHeaderMismatch},
		{"missing capabilities", modernTransport("tools/list", ""), `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}}}`, nil, 400, CodeInvalidParams},
		{"invalid clientInfo", modernTransport("tools/list", ""), strings.Replace(modernListRequest, `"io.modelcontextprotocol/clientCapabilities":{}`, `"io.modelcontextprotocol/clientCapabilities":{},"io.modelcontextprotocol/clientInfo":{"name":"sdk"}`, 1), nil, 400, CodeInvalidParams},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := []byte(test.body)
			if test.bodyBytes != nil {
				body = test.bodyBytes
			}
			lookups := 0
			_, protocolError := PrepareRequest(test.transport, body, func(string) bool {
				lookups++
				return true
			})
			if protocolError == nil {
				t.Fatal("PrepareRequest() unexpectedly succeeded")
			}
			if protocolError.HTTPStatus != test.wantHTTP || protocolError.Code != test.wantCode {
				t.Fatalf("error = %+v, want HTTP %d/code %d", protocolError, test.wantHTTP, test.wantCode)
			}
			if lookups != 0 {
				t.Fatalf("business method lookup ran %d times before validation completed", lookups)
			}
		})
	}
}

func TestModernOnlyHeadersCannotDowngradeToLegacy(t *testing.T) {
	transport := modernTransport("tools/list", "")
	transport.ProtocolVersion = ""
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
	request, protocolError := PrepareRequest(transport, body, func(string) bool {
		t.Fatal("method lookup must not run for incomplete modern identity")
		return true
	})
	if request != nil || protocolError == nil || protocolError.HTTPStatus != 400 || protocolError.Code != CodeHeaderMismatch {
		t.Fatalf("request=%+v error=%+v, want HTTP 400/code %d", request, protocolError, CodeHeaderMismatch)
	}

	transport.Origin = "https://evil.example"
	_, protocolError = PrepareRequest(transport, []byte(`{"deeply":"malformed"`), available)
	if protocolError == nil || protocolError.HTTPStatus != 403 {
		t.Fatalf("cross-origin modern candidate error = %+v, want HTTP 403 before body parsing", protocolError)
	}
}

func TestOriginPrecedesVersionAndBodyClassification(t *testing.T) {
	transport := modernTransport("tools/list", "")
	transport.ProtocolVersion = "2025-11-25"
	transport.Origin = "https://evil.example"
	_, protocolError := PrepareRequest(transport, []byte(`{"malformed":`), func(string) bool {
		t.Fatal("method lookup must not run for an untrusted Origin")
		return true
	})
	if protocolError == nil || protocolError.HTTPStatus != 403 || protocolError.Code != CodeInvalidRequest {
		t.Fatalf("origin-first error = %+v, want HTTP 403/code %d", protocolError, CodeInvalidRequest)
	}
}

func TestHeaderlessLargeLegacyMarkerAndLateModernMetadata(t *testing.T) {
	transport := Transport{
		Method:      "POST",
		Authority:   "mcp.example.com",
		ContentType: "application/json",
		Accept:      "application/json, text/event-stream",
	}
	padding := strings.Repeat("x", int(ModernMaxBodyBytes))
	legacyBody := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"note":"` +
		MetaProtocolVersion + padding + `"}}`)
	request, protocolError := PrepareRequest(transport, legacyBody, func(string) bool {
		t.Fatal("large headerless legacy request must not enter modern method lookup")
		return true
	})
	if protocolError != nil || request == nil || request.Era != EraLegacy {
		t.Fatalf("large legacy marker string misclassified: request=%+v error=%+v", request, protocolError)
	}

	modernBody := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"padding":"` +
		padding + `","_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28",` +
		`"io.modelcontextprotocol/clientCapabilities":{}}}}`)
	request, protocolError = PrepareRequest(transport, modernBody, func(string) bool {
		t.Fatal("oversized body-only modern request must fail before business dispatch")
		return true
	})
	if request != nil || protocolError == nil || protocolError.HTTPStatus != 413 || protocolError.Code != CodeInvalidRequest {
		t.Fatalf("late body-only modern error = %+v, want HTTP 413/code %d", protocolError, CodeInvalidRequest)
	}
}

func TestMCPNameBase64Sentinel(t *testing.T) {
	validNames := []string{
		"Hello, 世界",
		"\t leading and trailing \n",
		"=?base64?literal?=",
	}
	for _, name := range validNames {
		t.Run(strconv.Quote(name), func(t *testing.T) {
			body := strings.Replace(modernCallRequest, `"name":"echo"`, `"name":`+strconv.Quote(name), 1)
			header := nameHeaderPrefix + base64.StdEncoding.EncodeToString([]byte(name)) + nameHeaderSuffix
			request, protocolError := PrepareRequest(modernTransport("tools/call", header), []byte(body), available)
			if protocolError != nil || request == nil || request.Era != EraModern {
				t.Fatalf("valid sentinel rejected: request=%+v error=%+v", request, protocolError)
			}
		})
	}

	invalidHeaders := []string{
		"=?base64??=",
		"=?base64?not-base64?=",
		"=?base64?ZWNoby\n?=",
		"=?base64?/w==?=",
		"=?base64?SGVsbG8=",
		"Hello, 世界",
		" leading",
	}
	for _, header := range invalidHeaders {
		t.Run(header, func(t *testing.T) {
			_, protocolError := PrepareRequest(modernTransport("tools/call", header), []byte(modernCallRequest), available)
			if protocolError == nil || protocolError.Code != CodeHeaderMismatch {
				t.Fatalf("malformed/plain-unsafe header error = %+v, want code %d", protocolError, CodeHeaderMismatch)
			}
		})
	}
}

func TestMCPNamePlainHeaderAllowsInternalWhitespace(t *testing.T) {
	for _, name := range []string{"hello world", "hello\tworld"} {
		t.Run(strconv.Quote(name), func(t *testing.T) {
			body := modernCallRequestWithName(name)
			request, protocolError := PrepareRequest(modernTransportFromHeaders("tools/call", name), []byte(body), available)
			if protocolError != nil || request == nil || request.Era != EraModern {
				t.Fatalf("plain name rejected: request=%+v error=%+v", request, protocolError)
			}
		})
	}
}

func TestMCPNamePlainHeaderRejectsBoundaryWhitespaceAndUnsafeBytes(t *testing.T) {
	invalidNames := []string{
		" leading",
		"trailing ",
		"\tleading",
		"trailing\t",
		"hello\rworld",
		"hello\nworld",
		"hello\x00world",
		"hello\x1fworld",
		"hello\x7fworld",
		"héllo",
	}
	for _, name := range invalidNames {
		t.Run(strconv.Quote(name), func(t *testing.T) {
			body := modernCallRequestWithName(name)
			_, protocolError := PrepareRequest(modernTransportFromHeaders("tools/call", name), []byte(body), available)
			if protocolError == nil || protocolError.Code != CodeHeaderMismatch {
				t.Fatalf("unsafe plain name error = %+v, want code %d", protocolError, CodeHeaderMismatch)
			}
		})
	}
}

func TestMCPNameBase64HeaderAllowsBoundaryWhitespace(t *testing.T) {
	for _, name := range []string{" leading", "trailing ", "\tleading", "trailing\t"} {
		t.Run(strconv.Quote(name), func(t *testing.T) {
			body := modernCallRequestWithName(name)
			header := nameHeaderPrefix + base64.StdEncoding.EncodeToString([]byte(name)) + nameHeaderSuffix
			request, protocolError := PrepareRequest(modernTransportFromHeaders("tools/call", header), []byte(body), available)
			if protocolError != nil || request == nil || request.Era != EraModern {
				t.Fatalf("Base64 boundary whitespace rejected: request=%+v error=%+v", request, protocolError)
			}
		})
	}
}

func TestModernMetadataSchemaAndExtensions(t *testing.T) {
	completeInfo := `{
		"name":"sdk","title":"SDK","version":"1.0","description":"client",
		"websiteUrl":"https://example.com/sdk",
		"icons":[{"src":"https://example.com/icon.svg","mimeType":"image/svg+xml","sizes":["any"],"theme":"dark"}]
	}`
	body := strings.Replace(modernListRequest,
		`"io.modelcontextprotocol/clientCapabilities":{}`,
		`"io.modelcontextprotocol/clientCapabilities":{
			"roots":{},
			"sampling":{"context":{},"tools":{}},
			"elicitation":{"form":{},"url":{}},
			"experimental":{"vendor.feature":{"level":1}},
			"extensions":{"com.example/ui":{"mimeTypes":["text/html"]}},
			"vendorCapability":{"nested":true}
		},"io.modelcontextprotocol/clientInfo":`+completeInfo+`,"vendor.example/trace":{"enabled":true}`,
		1,
	)
	request, protocolError := PrepareRequest(modernTransport("tools/list", ""), []byte(body), available)
	if protocolError != nil {
		t.Fatalf("complete metadata rejected: %+v", protocolError)
	}
	if request.Metadata.ClientInfo == nil || request.Metadata.ClientInfo.Title != "SDK" || len(request.Metadata.ClientInfo.Icons) != 1 {
		t.Fatalf("clientInfo not preserved: %+v", request.Metadata.ClientInfo)
	}
	if _, ok := request.Metadata.Extensions["vendor.example/trace"]; !ok {
		t.Fatal("extensible _meta member was not preserved")
	}
	if request.Metadata.ClientCapabilities.Sampling == nil || request.Metadata.ClientCapabilities.Sampling.Tools == nil ||
		request.Metadata.ClientCapabilities.Elicitation == nil || request.Metadata.ClientCapabilities.Extensions["com.example/ui"] == nil {
		t.Fatalf("structured client capabilities not preserved: %+v", request.Metadata.ClientCapabilities)
	}

	invalidMembers := []string{
		`"io.modelcontextprotocol/clientCapabilities":{"roots":true}`,
		`"io.modelcontextprotocol/clientCapabilities":{"roots":{"listChanged":true}}`,
		`"io.modelcontextprotocol/clientCapabilities":{"sampling":{"tools":true}}`,
		`"io.modelcontextprotocol/clientCapabilities":{"sampling":{"tools":null}}`,
		`"io.modelcontextprotocol/clientCapabilities":{"sampling":{"unknown":{}}}`,
		`"io.modelcontextprotocol/clientCapabilities":{"elicitation":{"form":null}}`,
		`"io.modelcontextprotocol/clientCapabilities":{"experimental":{"vendor":null}}`,
		`"io.modelcontextprotocol/clientCapabilities":{"extensions":{"not-prefixed":{}}}`,
		`"io.modelcontextprotocol/clientCapabilities":{"extensions":{"com.example/ui":null}}`,
		`"io.modelcontextprotocol/clientCapabilities":{"":{}}`,
		`"io.modelcontextprotocol/clientCapabilities":{},"io.modelcontextprotocol/clientInfo":null`,
		`"io.modelcontextprotocol/clientCapabilities":{},"io.modelcontextprotocol/clientInfo":{"name":"sdk","version":"1","title":null}`,
		`"io.modelcontextprotocol/clientCapabilities":{},"io.modelcontextprotocol/clientInfo":{"name":"sdk","version":"1","icons":null}`,
		`"io.modelcontextprotocol/clientCapabilities":{},"io.modelcontextprotocol/clientInfo":{"name":"sdk","version":"1","icons":"not-an-array"}`,
		`"io.modelcontextprotocol/clientCapabilities":{},"io.modelcontextprotocol/clientInfo":{"name":"sdk","version":"1","websiteUrl":7}`,
		`"io.modelcontextprotocol/clientCapabilities":{},"io.modelcontextprotocol/clientInfo":{"name":"sdk","version":"1","future":true}`,
		`"io.modelcontextprotocol/clientCapabilities":{},"io.modelcontextprotocol/clientInfo":{"name":"sdk","version":"1","icons":[{"src":"https://example.com/i","mimeType":null}]}`,
		`"io.modelcontextprotocol/clientCapabilities":{},"io.modelcontextprotocol/clientInfo":{"name":"sdk","version":"1","icons":[{"src":"https://example.com/i","sizes":null}]}`,
		`"io.modelcontextprotocol/clientCapabilities":{},"io.modelcontextprotocol/clientInfo":{"name":"sdk","version":"1","icons":[{"src":"https://example.com/i","sizes":[null]}]}`,
		`"io.modelcontextprotocol/clientCapabilities":{},"io.modelcontextprotocol/clientInfo":{"name":"sdk","version":"1","icons":[{"src":"https://example.com/i","theme":"system"}]}`,
	}
	for _, member := range invalidMembers {
		t.Run(member, func(t *testing.T) {
			invalidBody := strings.Replace(modernListRequest, `"io.modelcontextprotocol/clientCapabilities":{}`, member, 1)
			_, protocolError := PrepareRequest(modernTransport("tools/list", ""), []byte(invalidBody), available)
			if protocolError == nil || protocolError.Code != CodeInvalidParams {
				t.Fatalf("invalid metadata error = %+v, want code %d", protocolError, CodeInvalidParams)
			}
		})
	}
}

func TestRequiredClientCapabilityHook(t *testing.T) {
	bodyWithRoots := strings.Replace(modernListRequest,
		`"io.modelcontextprotocol/clientCapabilities":{}`,
		`"io.modelcontextprotocol/clientCapabilities":{"roots":{},"sampling":{"context":{}}}`,
		1,
	)
	requiredTools := JSONObject{}
	requiredCapabilities := ClientCapabilities{
		Roots:    &RootCapabilities{},
		Sampling: &SamplingCapabilities{Tools: &requiredTools},
	}
	request, protocolError := PrepareRequestWithPolicy(modernTransport("tools/list", ""), []byte(bodyWithRoots), func(string) MethodPolicy {
		return MethodPolicy{Available: true, RequiredClientCapabilities: requiredCapabilities}
	})
	if request == nil || protocolError == nil || protocolError.Code != CodeMissingRequiredClientCapability {
		t.Fatalf("request=%+v error=%+v, want missing capability", request, protocolError)
	}
	wantMissingTools := ClientCapabilities{Sampling: &SamplingCapabilities{Tools: &JSONObject{}}}
	if protocolError.Data == nil || protocolError.Data.RequiredCapabilities == nil ||
		!reflect.DeepEqual(*protocolError.Data.RequiredCapabilities, wantMissingTools) {
		t.Fatalf("required capability data = %+v", protocolError.Data)
	}

	request, protocolError = PrepareRequestWithPolicy(modernTransport("tools/list", ""), []byte(bodyWithRoots), func(string) MethodPolicy {
		return MethodPolicy{Available: true, RequiredClientCapabilities: ClientCapabilities{Roots: &RootCapabilities{}}}
	})
	if protocolError != nil || request == nil {
		t.Fatalf("declared required capability rejected: request=%+v error=%+v", request, protocolError)
	}
}

func TestPrepareRequestExactVersionAndMethodErrors(t *testing.T) {
	tests := []struct {
		name      string
		transport Transport
		body      string
		wantCode  int
	}{
		{
			name: "unsupported version",
			transport: func() Transport {
				value := modernTransport("tools/list", "")
				value.ProtocolVersion = "2025-11-25"
				return value
			}(),
			body:     strings.ReplaceAll(modernListRequest, "2026-07-28", "2025-11-25"),
			wantCode: CodeUnsupportedVersion,
		},
		{
			name:      "removed lifecycle method",
			transport: modernTransport("initialize", ""),
			body:      strings.Replace(modernListRequest, "tools/list", "initialize", 1),
			wantCode:  CodeMethodNotFound,
		},
		{
			name:      "unsupported subscription",
			transport: modernTransport("subscriptions/listen", ""),
			body:      strings.Replace(modernListRequest, "tools/list", "subscriptions/listen", 1),
			wantCode:  CodeMethodNotFound,
		},
		{
			name:      "known surface not registered",
			transport: modernTransport("server/discover", ""),
			body:      strings.Replace(modernListRequest, "tools/list", "server/discover", 1),
			wantCode:  CodeMethodNotFound,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, protocolError := PrepareRequest(test.transport, []byte(test.body), func(method string) bool {
				return method != "server/discover"
			})
			if protocolError == nil || protocolError.HTTPStatus != 400 && protocolError.HTTPStatus != 404 || protocolError.Code != test.wantCode {
				t.Fatalf("error = %+v, want code %d", protocolError, test.wantCode)
			}
			if test.wantCode == CodeMethodNotFound && protocolError.HTTPStatus != 404 {
				t.Fatalf("method error HTTP status = %d, want 404", protocolError.HTTPStatus)
			}
		})
	}
}

func TestPrepareRequestPreservesAllLegacyProfiles(t *testing.T) {
	legacyBody := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"opaque":"trailing-legacy-data"}}`)
	for _, version := range LegacyVersions() {
		t.Run(string(version), func(t *testing.T) {
			transport := Transport{Method: "PUT", ProtocolVersion: string(version)}
			request, protocolError := PrepareRequest(transport, legacyBody, func(string) bool {
				t.Fatal("modern method lookup must not run for legacy")
				return false
			})
			if protocolError != nil {
				t.Fatalf("legacy request rejected: %+v", protocolError)
			}
			if request.Era != EraLegacy || request.Version != version {
				t.Fatalf("unexpected legacy profile: %+v", request)
			}
			if !bytes.Equal(request.Envelope.Raw, legacyBody) {
				t.Fatal("legacy bytes changed at compatibility boundary")
			}
		})
	}

	initialize := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`)
	request, protocolError := PrepareRequest(Transport{Method: "POST"}, initialize, available)
	if protocolError != nil || request.Era != EraLegacy {
		t.Fatalf("headerless legacy initialize was not preserved: request=%+v error=%+v", request, protocolError)
	}
}

func TestModernCancellationIsRequestScopedAndIdempotent(t *testing.T) {
	request, protocolError := PrepareRequest(modernTransport("tools/list", ""), []byte(modernListRequest), available)
	if protocolError != nil {
		t.Fatalf("PrepareRequest() error = %+v", protocolError)
	}
	cleanupCalls := 0
	request.OnCancel(func() { cleanupCalls++ })
	unregister := request.OnCancel(func() { cleanupCalls += 100 })
	unregister()
	unregister()
	request.Cancel()
	request.Cancel()
	if cleanupCalls != 1 {
		t.Fatalf("cleanup calls = %d, want 1", cleanupCalls)
	}
	request.OnCancel(func() { cleanupCalls++ })
	if cleanupCalls != 2 {
		t.Fatalf("late cleanup calls = %d, want 2", cleanupCalls)
	}
	select {
	case <-request.Cancellation.Done():
	default:
		t.Fatal("request cancellation was not released")
	}
	requestType := reflect.TypeOf(*request)
	for i := 0; i < requestType.NumField(); i++ {
		if strings.Contains(strings.ToLower(requestType.Field(i).Name), "session") {
			t.Fatalf("modern request context contains protocol session field %q", requestType.Field(i).Name)
		}
	}
}

func TestMarshalErrorResponsePreservesValidID(t *testing.T) {
	request, protocolError := PrepareRequest(
		func() Transport { value := modernTransport("ping", ""); return value }(),
		[]byte(strings.Replace(modernListRequest, "tools/list", "initialize", 1)),
		available,
	)
	if protocolError == nil {
		t.Fatal("expected method-not-found error")
	}
	body := MarshalErrorResponse(request.Envelope.ID, protocolError)
	var decoded struct {
		ID    json.RawMessage `json:"id"`
		Error struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("invalid response JSON: %v", err)
	}
	if string(decoded.ID) != `"request-1"` || decoded.Error.Code != CodeHeaderMismatch {
		t.Fatalf("unexpected error response: %s", body)
	}
}
