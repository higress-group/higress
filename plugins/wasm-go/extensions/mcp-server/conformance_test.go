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
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/protocol"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	wasmtest "github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const officialFixtureDir = "testdata/conformance/official"

var conformanceServerConfig = json.RawMessage(`{
	"server":{"name":"fixture-server","type":"rest"},
	"tools":[{
		"name":"get_weather",
		"description":"Return deterministic fixture weather",
		"args":[{"name":"location","description":"City name","type":"string","required":true}],
		"responseTemplate":{"body":"weather for {{.args.location}}"}
	}]
}`)

type provenanceManifest struct {
	Commit  string                       `json:"commit"`
	Files   map[string]string            `json:"files"`
	SHA256  map[string]string            `json:"sha256"`
	Derived map[string]derivedProvenance `json:"derivedCases"`
}

type derivedProvenance struct {
	Base       string `json:"base"`
	BasePath   string `json:"basePath"`
	Derivation string `json:"derivation"`
	SHA256     string `json:"sha256"`
}

type errorExpectation struct {
	HTTPStatus uint32
	Code       int64
	Message    string
	ID         string
	Data       string
	Reference  string
}

type derivedCase struct {
	Name       string
	Base       string
	BasePath   string
	Derivation string
	Headers    [][2]string
	Body       []byte
	WantStatus uint32
	WantID     string
	WantError  *errorExpectation
}

func TestOfficial20260728FixturesArePinned(t *testing.T) {
	var provenance provenanceManifest
	bytes := readOfficialFixture(t, "PROVENANCE.json")
	require.NoError(t, json.Unmarshal(bytes, &provenance))
	require.Equal(t, "f817239f4d6b1efff2c4dfc2f7af85c985d73076", provenance.Commit)
	require.Equal(t, len(provenance.Files), len(provenance.SHA256))
	for name, sourcePath := range provenance.Files {
		require.NotEmpty(t, sourcePath, name)
		want, ok := provenance.SHA256[name]
		require.True(t, ok, "missing checksum for %s", name)
		sum := sha256.Sum256(readOfficialFixture(t, name))
		require.Equal(t, want, hex.EncodeToString(sum[:]), name)
	}

	cases := derivedConformanceCases(t)
	if len(provenance.Derived) != len(cases) {
		t.Errorf("derived provenance entries = %d, want %d", len(provenance.Derived), len(cases))
	}
	known := make(map[string]struct{}, len(cases))
	for _, tc := range cases {
		known[tc.Name] = struct{}{}
		gotSHA := derivedCaseSHA256(tc)
		entry, ok := provenance.Derived[tc.Name]
		if !ok {
			t.Errorf("missing derived provenance %q: base=%q basePath=%q derivation=%q sha256=%q", tc.Name, tc.Base, tc.BasePath, tc.Derivation, gotSHA)
			continue
		}
		require.Equal(t, tc.Base, entry.Base, tc.Name)
		require.Equal(t, tc.BasePath, entry.BasePath, tc.Name)
		require.Equal(t, tc.Derivation, entry.Derivation, tc.Name)
		require.Equal(t, gotSHA, entry.SHA256, tc.Name)
	}
	for name := range provenance.Derived {
		_, ok := known[name]
		require.True(t, ok, "unknown derived provenance entry %q", name)
	}
}

func TestOfficial20260728PositiveRequestsAndCapabilityHonesty(t *testing.T) {
	wasmtest.RunTest(t, func(t *testing.T) {
		references := map[string]string{
			"discover-request.json":   "discover-result-response.json",
			"list-tools-request.json": "list-tools-result-response.json",
			"call-tool-request.json":  "call-tool-result-response.json",
		}
		for requestName, responseName := range references {
			t.Run(requestName, func(t *testing.T) {
				body := readOfficialFixture(t, requestName)
				response := conformanceExchange(t, body, modernHeaders(body))
				wantID := gjson.GetBytes(body, "id").Raw
				assertSuccessEnvelope(t, response, 200, wantID)
				reference := readOfficialFixture(t, responseName)
				require.Equal(t, gjson.GetBytes(reference, "result.resultType").String(), gjson.GetBytes(response.Data, "result.resultType").String())
				require.Equal(t, "fixture-server", gjson.GetBytes(response.Data, "result._meta.io\\.modelcontextprotocol/serverInfo.name").String())
				require.Equal(t, "1.0.0", gjson.GetBytes(response.Data, "result._meta.io\\.modelcontextprotocol/serverInfo.version").String())

				switch gjson.GetBytes(body, "method").String() {
				case "server/discover":
					minimumTools := readOfficialFixture(t, "tools-minimum-capability.json")
					require.JSONEq(t, string(minimumTools), gjson.GetBytes(response.Data, "result.capabilities").Raw)
					require.JSONEq(t, `["2024-11-05","2025-03-26","2025-06-18","2026-07-28"]`, gjson.GetBytes(response.Data, "result.supportedVersions").Raw)
					assertCacheWireContract(t, response)
					for _, path := range []string{
						"result.capabilities.tools.listChanged",
						"result.capabilities.mrtr",
						"result.capabilities.subscriptions",
						"result.capabilities.resources",
						"result.capabilities.prompts",
						"result.capabilities.completion",
						"result.capabilities.tasks",
						"result.capabilities.apps",
					} {
						require.False(t, gjson.GetBytes(response.Data, path).Exists(), "%s leaked in %s", path, response.Data)
					}
					require.False(t, gjson.GetBytes(response.Data, `result.supportedVersions.#(=="2025-11-25")`).Exists())
				case "tools/list":
					require.Equal(t, "get_weather", gjson.GetBytes(response.Data, "result.tools.0.name").String())
					assertCacheWireContract(t, response)
					for _, path := range []string{"result.nextCursor", "result.tools.0.outputSchema", "result.tools.0.icons", "result.tools.0.annotations"} {
						require.False(t, gjson.GetBytes(response.Data, path).Exists(), "%s leaked in %s", path, response.Data)
					}
				case "tools/call":
					require.Contains(t, string(response.Data), "weather for New York")
					require.False(t, gjson.GetBytes(response.Data, "result.ttlMs").Exists())
					require.False(t, gjson.GetBytes(response.Data, "result.cacheScope").Exists())
					require.False(t, gjson.GetBytes(response.Data, "result.requestState").Exists())
				}
			})
		}

		for _, tc := range derivedConformanceCases(t) {
			if tc.WantError != nil {
				continue
			}
			t.Run(tc.Name, func(t *testing.T) {
				response := conformanceExchange(t, tc.Body, tc.Headers)
				if tc.WantStatus == 202 {
					require.Equal(t, uint32(202), response.StatusCode)
					require.Empty(t, response.Data)
					return
				}
				assertSuccessEnvelope(t, response, tc.WantStatus, tc.WantID)
			})
		}
	})
}

func TestOfficial20260728MissingRequiredClientCapabilityPolicy(t *testing.T) {
	body := readOfficialFixture(t, "list-tools-request.json")
	transport := protocol.NewTransport("POST", "mcp.example.com", modernHeaders(body))
	request, protocolError := protocol.PrepareRequestWithPolicy(transport, body, func(method string) protocol.MethodPolicy {
		require.Equal(t, "tools/list", method)
		return protocol.MethodPolicy{
			Available: true,
			RequiredClientCapabilities: protocol.ClientCapabilities{
				Elicitation: &protocol.ElicitationCapabilities{},
			},
		}
	})
	require.NotNil(t, request)
	require.NotNil(t, protocolError)

	reference := readOfficialFixture(t, "missing-required-client-capability-error.json")
	require.Equal(t, uint32(400), protocolError.HTTPStatus)
	require.Equal(t, int(gjson.GetBytes(reference, "error.code").Int()), protocolError.Code)
	require.Equal(t, "missing required client capability", protocolError.Message)
	require.NotNil(t, protocolError.Data)
	require.NotNil(t, protocolError.Data.RequiredCapabilities)
	requiredCapabilities, err := json.Marshal(protocolError.Data.RequiredCapabilities)
	require.NoError(t, err)
	require.JSONEq(t, gjson.GetBytes(reference, "error.data.requiredCapabilities").Raw, string(requiredCapabilities))

	response := protocol.MarshalErrorResponse(request.Envelope.ID, protocolError)
	var envelope map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(response, &envelope))
	require.ElementsMatch(t, []string{"jsonrpc", "id", "error"}, mapKeys(envelope))
	require.JSONEq(t, `"2.0"`, string(envelope["jsonrpc"]))
	require.JSONEq(t, gjson.GetBytes(body, "id").Raw, string(envelope["id"]))
	var responseError map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(envelope["error"], &responseError))
	require.ElementsMatch(t, []string{"code", "message", "data"}, mapKeys(responseError))
	require.Equal(t, gjson.GetBytes(reference, "error.code").Int(), gjson.GetBytes(envelope["error"], "code").Int())
	require.Equal(t, "missing required client capability", gjson.GetBytes(envelope["error"], "message").String())
	require.JSONEq(t, gjson.GetBytes(reference, "error.data.requiredCapabilities").Raw, gjson.GetBytes(envelope["error"], "data.requiredCapabilities").Raw)
}

func TestOfficial20260728NegativeContracts(t *testing.T) {
	wasmtest.RunTest(t, func(t *testing.T) {
		for _, tc := range derivedConformanceCases(t) {
			if tc.WantError == nil {
				continue
			}
			t.Run(tc.Name, func(t *testing.T) {
				response := conformanceExchange(t, tc.Body, tc.Headers)
				assertProtocolError(t, response, *tc.WantError)
			})
		}
	})
}

func TestOfficial20260728RequestAndResponseBufferBounds(t *testing.T) {
	wasmtest.RunTest(t, func(t *testing.T) {
		body := readOfficialFixture(t, "list-tools-request.json")
		host, status := wasmtest.NewTestHost(conformanceServerConfig)
		require.Equal(t, types.OnPluginStartStatusOK, status)
		defer host.Reset()
		host.InitHttp()
		defer host.CompleteHttp()
		require.Equal(t, types.HeaderStopIteration, host.CallOnHttpRequestHeaders(modernHeaders(body)))

		requestLimit, err := host.GetProperty([]string{"set_decoder_buffer_limit"})
		require.NoError(t, err)
		require.Equal(t, "1048576", string(requestLimit), "modern request body limit must remain 1 MiB")
		responseLimit, err := host.GetProperty([]string{"set_encoder_buffer_limit"})
		require.NoError(t, err)
		require.Equal(t, "104857600", string(responseLimit), "response buffer limit must remain the explicit 100 MiB compatibility bound")

		host.CallOnHttpRequestBody(body)
		require.NotNil(t, host.GetLocalResponse())
	})
}

func derivedConformanceCases(t *testing.T) []derivedCase {
	t.Helper()
	headerMismatchAtHeaders := errorExpectation{HTTPStatus: 400, Code: -32020, Message: "MCP header does not match request body", ID: "null", Reference: "header-mismatch-error.json"}
	headerMismatchWithListID := headerMismatchAtHeaders
	headerMismatchWithListID.ID = `"list-tools-example"`
	headerMismatchWithCallID := headerMismatchAtHeaders
	headerMismatchWithCallID.ID = `"call-tool-example"`

	invalidRequestAtHeaders := func(status uint32, message string) *errorExpectation {
		return &errorExpectation{HTTPStatus: status, Code: -32600, Message: message, ID: "null"}
	}
	invalidRequestWithNullID := func(message string) *errorExpectation {
		return &errorExpectation{HTTPStatus: 400, Code: -32600, Message: message, ID: "null"}
	}

	newCase := func(name, base, derivation string) derivedCase {
		body := readOfficialFixture(t, base)
		return derivedCase{
			Name:       name,
			Base:       base,
			BasePath:   officialSourcePath(base),
			Derivation: derivation,
			Headers:    modernHeaders(body),
			Body:       body,
			WantStatus: 200,
			WantID:     gjson.GetBytes(body, "id").Raw,
		}
	}

	contentType := newCase("unsupported-content-type", "list-tools-request.json", "replace Content-Type application/json with text/plain")
	contentType.Headers = replaceHeader(t, contentType.Headers, "content-type", "text/plain")
	contentType.WantError = invalidRequestAtHeaders(415, "unsupported Content-Type")

	accept := newCase("unacceptable-accept", "list-tools-request.json", "replace Accept with application/json so text/event-stream is absent")
	accept.Headers = replaceHeader(t, accept.Headers, "accept", "application/json")
	accept.WantError = invalidRequestAtHeaders(406, "unacceptable response media type")

	hostileOrigin := newCase("hostile-origin", "list-tools-request.json", "append Origin https://evil.example while authority remains mcp.example.com")
	hostileOrigin.Headers = append(hostileOrigin.Headers, [2]string{"origin", "https://evil.example"})
	hostileOrigin.WantError = invalidRequestAtHeaders(403, "untrusted Origin")

	trustedOrigin := newCase("same-origin", "list-tools-request.json", "append same-origin Origin https://mcp.example.com")
	trustedOrigin.Headers = append(trustedOrigin.Headers, [2]string{"origin", "https://mcp.example.com"})

	batch := newCase("batch-envelope", "list-tools-request.json", "wrap the complete official request in a one-element JSON array")
	batch.Body = append(append([]byte("["), batch.Body...), ']')
	batch.WantError = invalidRequestWithNullID("JSON-RPC batch and non-object envelopes are not supported")

	responseEnvelope := newCase("response-envelope", "list-tools-result-response.json", "send the official ListToolsResultResponse example as a request using ListToolsRequest transport headers")
	responseEnvelope.Headers = modernHeaders(readOfficialFixture(t, "list-tools-request.json"))
	responseEnvelope.WantError = invalidRequestWithNullID("JSON-RPC response envelopes are not accepted")

	trailingJSON := newCase("trailing-json", "list-tools-request.json", "append a second empty JSON object after the official request")
	trailingJSON.Body = append(trailingJSON.Body, []byte("\n{}")...)
	trailingJSON.WantError = &errorExpectation{HTTPStatus: 400, Code: -32700, Message: "parse error", ID: "null", Reference: "parse-error.json"}

	malformedJSON := newCase("malformed-json", "list-tools-request.json", "truncate the official request immediately after the params key")
	malformedJSON.Body = []byte(`{"jsonrpc":"2.0","id":"list-tools-example","method":"tools/list","params":`)
	malformedJSON.WantError = &errorExpectation{HTTPStatus: 400, Code: -32700, Message: "parse error", ID: "null", Reference: "parse-error.json"}

	duplicateVersion := newCase("duplicate-protocol-version-header", "list-tools-request.json", "append a second identical MCP-Protocol-Version singleton header")
	duplicateVersion.Headers = duplicateHeader(t, duplicateVersion.Headers, "MCP-Protocol-Version")
	duplicateVersion.WantError = cloneExpectation(headerMismatchAtHeaders)

	duplicateMethod := newCase("duplicate-method-header", "list-tools-request.json", "append a second identical Mcp-Method singleton header")
	duplicateMethod.Headers = duplicateHeader(t, duplicateMethod.Headers, "Mcp-Method")
	duplicateMethod.WantError = cloneExpectation(headerMismatchAtHeaders)

	duplicateName := newCase("duplicate-name-header", "call-tool-request.json", "append a second identical Mcp-Name singleton header")
	duplicateName.Headers = duplicateHeader(t, duplicateName.Headers, "Mcp-Name")
	duplicateName.WantError = cloneExpectation(headerMismatchAtHeaders)

	missingVersion := newCase("missing-protocol-version-header", "list-tools-request.json", "remove MCP-Protocol-Version while retaining the modern method header")
	missingVersion.Headers = removeHeader(missingVersion.Headers, "MCP-Protocol-Version")
	missingVersion.WantError = cloneExpectation(headerMismatchAtHeaders)

	missingMethod := newCase("missing-method-header", "list-tools-request.json", "remove Mcp-Method while retaining the 2026 protocol header")
	missingMethod.Headers = removeHeader(missingMethod.Headers, "Mcp-Method")
	missingMethod.WantError = cloneExpectation(headerMismatchAtHeaders)

	missingName := newCase("missing-name-header", "call-tool-request.json", "remove the required Mcp-Name header from the official tools/call request")
	missingName.Headers = removeHeader(missingName.Headers, "Mcp-Name")
	missingName.WantError = cloneExpectation(headerMismatchWithCallID)

	mismatchedMethod := newCase("mismatched-method-header", "list-tools-request.json", "replace Mcp-Method tools/list with tools/call without changing the body")
	mismatchedMethod.Headers = replaceHeader(t, mismatchedMethod.Headers, "Mcp-Method", "tools/call")
	mismatchedMethod.WantError = cloneExpectation(headerMismatchWithListID)

	mismatchedName := newCase("mismatched-name-header", "call-tool-request.json", "replace Mcp-Name get_weather with different_tool without changing the body")
	mismatchedName.Headers = replaceHeader(t, mismatchedName.Headers, "Mcp-Name", "different_tool")
	mismatchedName.WantError = cloneExpectation(headerMismatchWithCallID)

	mismatchedBodyVersion := newCase("mismatched-body-version", "list-tools-request.json", "replace body protocolVersion 2026-07-28 with 1900-01-01 while retaining the 2026 transport header")
	mismatchedBodyVersion.Body = mutateJSONObject(t, mismatchedBodyVersion.Body, func(object map[string]any) {
		requestMeta(t, object)[protocol.MetaProtocolVersion] = "1900-01-01"
	})
	mismatchedBodyVersion.WantError = &errorExpectation{
		HTTPStatus: 400,
		Code:       -32022,
		Message:    "unsupported MCP protocol version",
		ID:         `"list-tools-example"`,
		Data:       `{"supported":["2024-11-05","2025-03-26","2025-06-18","2026-07-28"],"requested":"1900-01-01"}`,
		Reference:  "unsupported-version-error.json",
	}

	unsupportedHeaderVersion := newCase("unsupported-header-version", "list-tools-request.json", "replace MCP-Protocol-Version 2026-07-28 with 1900-01-01 before body processing")
	unsupportedHeaderVersion.Headers = replaceHeader(t, unsupportedHeaderVersion.Headers, "MCP-Protocol-Version", "1900-01-01")
	unsupportedHeaderVersion.WantError = &errorExpectation{
		HTTPStatus: 400,
		Code:       -32022,
		Message:    "unsupported MCP protocol version",
		ID:         "null",
		Data:       `{"supported":["2024-11-05","2025-03-26","2025-06-18","2026-07-28"],"requested":"1900-01-01"}`,
		Reference:  "unsupported-version-error.json",
	}

	nullID := newCase("null-jsonrpc-id", "list-tools-request.json", "replace the official string id with JSON null")
	nullID.Body = mutateJSONObject(t, nullID.Body, func(object map[string]any) { object["id"] = nil })
	nullID.WantError = invalidRequestWithNullID("JSON-RPC id must be a string or integer")

	fractionalID := newCase("fractional-jsonrpc-id", "list-tools-request.json", "replace the official string id with fractional number 1.5")
	fractionalID.Body = mutateJSONObject(t, fractionalID.Body, func(object map[string]any) { object["id"] = 1.5 })
	fractionalID.WantError = invalidRequestWithNullID("JSON-RPC id must be a string or integer")

	booleanID := newCase("boolean-jsonrpc-id", "list-tools-request.json", "replace the official string id with boolean true")
	booleanID.Body = mutateJSONObject(t, booleanID.Body, func(object map[string]any) { object["id"] = true })
	booleanID.WantError = invalidRequestWithNullID("JSON-RPC id must be a string or integer")

	integerID := newCase("integer-jsonrpc-id", "list-tools-request.json", "replace the official string id with integer 42")
	integerID.Body = mutateJSONObject(t, integerID.Body, func(object map[string]any) { object["id"] = int64(42) })
	integerID.WantID = "42"

	notification := newCase("notification-without-id", "list-tools-request.json", "remove id to derive a valid modern tools/list notification")
	notification.Body = mutateJSONObject(t, notification.Body, func(object map[string]any) { delete(object, "id") })
	notification.WantStatus = 202
	notification.WantID = ""

	missingMeta := newCase("missing-meta", "list-tools-request.json", "remove params._meta while retaining complete modern transport headers")
	missingMeta.Body = mutateJSONObject(t, missingMeta.Body, func(object map[string]any) { delete(requestParams(t, object), "_meta") })
	missingMeta.WantError = cloneExpectation(headerMismatchWithListID)

	missingProtocolMeta := newCase("missing-protocol-version-meta", "list-tools-request.json", "remove io.modelcontextprotocol/protocolVersion from request metadata")
	missingProtocolMeta.Body = mutateJSONObject(t, missingProtocolMeta.Body, func(object map[string]any) { delete(requestMeta(t, object), protocol.MetaProtocolVersion) })
	missingProtocolMeta.WantError = cloneExpectation(headerMismatchWithListID)

	missingCapabilities := newCase("missing-client-capabilities-meta", "list-tools-request.json", "remove required io.modelcontextprotocol/clientCapabilities metadata")
	missingCapabilities.Body = mutateJSONObject(t, missingCapabilities.Body, func(object map[string]any) { delete(requestMeta(t, object), protocol.MetaClientCapabilities) })
	missingCapabilities.WantError = &errorExpectation{HTTPStatus: 400, Code: -32602, Message: "modern MCP clientCapabilities metadata is required", ID: `"list-tools-example"`, Reference: "invalid-tool-arguments-error.json"}

	optionalClientInfo := newCase("optional-client-info-omitted", "list-tools-request.json", "remove optional io.modelcontextprotocol/clientInfo metadata")
	optionalClientInfo.Body = mutateJSONObject(t, optionalClientInfo.Body, func(object map[string]any) { delete(requestMeta(t, object), protocol.MetaClientInfo) })

	cancelled := newCase("modern-cancelled-notification", "cancelled-notification.json", "add required modern metadata to the official cancellation notification and send it with modern identity headers")
	cancelled.Body = mutateJSONObject(t, cancelled.Body, func(object map[string]any) {
		requestParams(t, object)["_meta"] = map[string]any{
			protocol.MetaProtocolVersion:    string(protocol.Version20260728),
			protocol.MetaClientCapabilities: map[string]any{},
		}
	})
	cancelled.Headers = modernHeaders(cancelled.Body)
	cancelled.WantError = &errorExpectation{HTTPStatus: 404, Code: -32601, Message: "method not found", ID: "null", Reference: "method-not-found-error.json"}

	oversizedBody := newCase("oversized-request-body", "list-tools-request.json", "append ASCII whitespace until the request exceeds the 1 MiB modern body limit")
	oversizedBody.Body = append(oversizedBody.Body, bytes.Repeat([]byte(" "), int(protocol.ModernMaxBodyBytes)+1)...)
	oversizedBody.WantError = invalidRequestAtHeaders(413, "request body exceeds the modern MCP limit")

	oversizedToolName := newCase("oversized-tool-name", "call-tool-request.json", "replace params.name and Mcp-Name with the same 4097 ASCII bytes, one byte beyond the decoded tool-name limit")
	longToolName := strings.Repeat("a", 4097)
	oversizedToolName.Body = mutateJSONObject(t, oversizedToolName.Body, func(object map[string]any) { requestParams(t, object)["name"] = longToolName })
	oversizedToolName.Headers = replaceHeader(t, oversizedToolName.Headers, "Mcp-Name", longToolName)
	oversizedToolName.WantError = &errorExpectation{HTTPStatus: 400, Code: -32602, Message: "tools/call params.name is required", ID: `"call-tool-example"`, Reference: "invalid-tool-arguments-error.json"}

	invalidToolName := newCase("invalid-tool-name-type", "call-tool-request.json", "replace params.name string with JSON number 7 while retaining the original Mcp-Name header")
	invalidToolName.Body = mutateJSONObject(t, invalidToolName.Body, func(object map[string]any) { requestParams(t, object)["name"] = 7 })
	invalidToolName.WantError = &errorExpectation{HTTPStatus: 400, Code: -32602, Message: "tools/call params.name is required", ID: `"call-tool-example"`, Reference: "invalid-tool-arguments-error.json"}

	methodNotFound := newCase("method-not-found", "list-tools-request.json", "replace method tools/list with prompts/list in both body and Mcp-Method header")
	methodNotFound.Body = mutateJSONObject(t, methodNotFound.Body, func(object map[string]any) { object["method"] = "prompts/list" })
	methodNotFound.Headers = replaceHeader(t, methodNotFound.Headers, "Mcp-Method", "prompts/list")
	methodNotFound.WantError = &errorExpectation{HTTPStatus: 404, Code: -32601, Message: "method not found", ID: `"list-tools-example"`, Reference: "method-not-found-error.json"}

	return []derivedCase{
		contentType, accept, hostileOrigin, trustedOrigin,
		batch, responseEnvelope, trailingJSON, malformedJSON,
		duplicateVersion, duplicateMethod, duplicateName,
		missingVersion, missingMethod, missingName,
		mismatchedMethod, mismatchedName, mismatchedBodyVersion, unsupportedHeaderVersion,
		nullID, fractionalID, booleanID, integerID, notification,
		missingMeta, missingProtocolMeta, missingCapabilities, optionalClientInfo,
		cancelled, oversizedBody, oversizedToolName, invalidToolName, methodNotFound,
	}
}

func cloneExpectation(expectation errorExpectation) *errorExpectation {
	copy := expectation
	return &copy
}

func assertCacheWireContract(t *testing.T, response *wasmtestLocalResponse) {
	t.Helper()
	ttl := gjson.GetBytes(response.Data, "result.ttlMs")
	require.True(t, ttl.Exists(), "result.ttlMs missing from %s", response.Data)
	require.Equal(t, gjson.Number, ttl.Type)
	require.Equal(t, int64(0), ttl.Int())
	cacheScope := gjson.GetBytes(response.Data, "result.cacheScope")
	require.True(t, cacheScope.Exists(), "result.cacheScope missing from %s", response.Data)
	require.Equal(t, gjson.String, cacheScope.Type)
	require.Equal(t, "private", cacheScope.String())
}

func assertSuccessEnvelope(t *testing.T, response *wasmtestLocalResponse, status uint32, wantID string) {
	t.Helper()
	require.Equal(t, status, response.StatusCode)
	require.True(t, wasmtest.HasHeaderWithValue(response.Headers, "Content-Type", "application/json; charset=utf-8"), "%#v", response.Headers)
	var envelope map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(response.Data, &envelope), string(response.Data))
	require.ElementsMatch(t, []string{"jsonrpc", "id", "result"}, mapKeys(envelope))
	require.JSONEq(t, `"2.0"`, string(envelope["jsonrpc"]))
	require.JSONEq(t, wantID, string(envelope["id"]))
	require.NotEmpty(t, envelope["result"])
}

func assertProtocolError(t *testing.T, response *wasmtestLocalResponse, want errorExpectation) {
	t.Helper()
	require.Equal(t, want.HTTPStatus, response.StatusCode)
	require.True(t, wasmtest.HasHeaderWithValue(response.Headers, "Content-Type", "application/json; charset=utf-8"), "%#v", response.Headers)
	var envelope map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(response.Data, &envelope), string(response.Data))
	require.ElementsMatch(t, []string{"jsonrpc", "id", "error"}, mapKeys(envelope))
	require.JSONEq(t, `"2.0"`, string(envelope["jsonrpc"]))
	require.JSONEq(t, want.ID, string(envelope["id"]))
	require.NotContains(t, envelope, "result")

	var protocolError map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(envelope["error"], &protocolError))
	wantErrorFields := []string{"code", "message"}
	if want.Data != "" {
		wantErrorFields = append(wantErrorFields, "data")
	}
	require.ElementsMatch(t, wantErrorFields, mapKeys(protocolError))
	require.Equal(t, want.Code, gjson.GetBytes(envelope["error"], "code").Int())
	require.Equal(t, want.Message, gjson.GetBytes(envelope["error"], "message").String())
	if want.Data == "" {
		require.NotContains(t, protocolError, "data")
	} else {
		require.JSONEq(t, want.Data, string(protocolError["data"]))
	}
	if want.Reference != "" {
		require.Equal(t, officialErrorCode(t, want.Reference), want.Code, want.Reference)
	}
}

func conformanceExchange(t *testing.T, body []byte, headers [][2]string) *wasmtestLocalResponse {
	t.Helper()
	host, status := wasmtest.NewTestHost(conformanceServerConfig)
	require.Equal(t, types.OnPluginStartStatusOK, status)
	defer host.Reset()
	host.InitHttp()
	defer host.CompleteHttp()
	host.CallOnHttpRequestHeaders(headers)
	if host.GetLocalResponse() == nil {
		host.CallOnHttpRequestBody(body)
	}
	response := host.GetLocalResponse()
	require.NotNil(t, response)
	return &wasmtestLocalResponse{
		StatusCode: response.StatusCode,
		Data:       append([]byte(nil), response.Data...),
		Headers:    append([][2]string(nil), response.Headers...),
	}
}

func modernHeaders(body []byte) [][2]string {
	headers := [][2]string{
		{":authority", "mcp.example.com"}, {":method", "POST"}, {":path", "/mcp"},
		{"content-type", "application/json"}, {"accept", "application/json, text/event-stream"},
	}
	version := gjson.GetBytes(body, "params._meta.io\\.modelcontextprotocol/protocolVersion").String()
	method := gjson.GetBytes(body, "method").String()
	if version != "" {
		headers = append(headers, [2]string{"MCP-Protocol-Version", version})
	}
	if method != "" {
		headers = append(headers, [2]string{"Mcp-Method", method})
	}
	if name := gjson.GetBytes(body, "params.name").String(); name != "" {
		headers = append(headers, [2]string{"Mcp-Name", name})
	}
	return headers
}

func replaceHeader(t *testing.T, headers [][2]string, name, value string) [][2]string {
	t.Helper()
	copy := append([][2]string(nil), headers...)
	for i := range copy {
		if strings.EqualFold(copy[i][0], name) {
			copy[i][1] = value
			return copy
		}
	}
	t.Fatalf("header %q not found in %#v", name, headers)
	return nil
}

func removeHeader(headers [][2]string, name string) [][2]string {
	filtered := make([][2]string, 0, len(headers))
	for _, header := range headers {
		if !strings.EqualFold(header[0], name) {
			filtered = append(filtered, header)
		}
	}
	return filtered
}

func duplicateHeader(t *testing.T, headers [][2]string, name string) [][2]string {
	t.Helper()
	for _, header := range headers {
		if strings.EqualFold(header[0], name) {
			return append(append([][2]string(nil), headers...), header)
		}
	}
	t.Fatalf("header %q not found in %#v", name, headers)
	return nil
}

func mutateJSONObject(t *testing.T, body []byte, mutate func(map[string]any)) []byte {
	t.Helper()
	var object map[string]any
	require.NoError(t, json.Unmarshal(body, &object))
	mutate(object)
	derived, err := json.Marshal(object)
	require.NoError(t, err)
	return derived
}

func requestParams(t *testing.T, object map[string]any) map[string]any {
	t.Helper()
	params, ok := object["params"].(map[string]any)
	require.True(t, ok, "params missing from %#v", object)
	return params
}

func requestMeta(t *testing.T, object map[string]any) map[string]any {
	t.Helper()
	meta, ok := requestParams(t, object)["_meta"].(map[string]any)
	require.True(t, ok, "_meta missing from %#v", object)
	return meta
}

func derivedCaseSHA256(tc derivedCase) string {
	canonical, _ := json.Marshal(struct {
		Headers [][2]string `json:"headers"`
		Body    string      `json:"body"`
	}{Headers: tc.Headers, Body: string(tc.Body)})
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

func officialSourcePath(name string) string {
	paths := map[string]string{
		"discover-request.json":                         "schema/draft/examples/DiscoverRequest/server-discover-request.json",
		"discover-result-response.json":                 "schema/draft/examples/DiscoverResultResponse/discover-result-response.json",
		"list-tools-request.json":                       "schema/draft/examples/ListToolsRequest/list-tools-request.json",
		"list-tools-result-response.json":               "schema/draft/examples/ListToolsResultResponse/list-tools-result-response.json",
		"call-tool-request.json":                        "schema/draft/examples/CallToolRequest/call-tool-request.json",
		"call-tool-result-response.json":                "schema/draft/examples/CallToolResultResponse/call-tool-result-response.json",
		"cancelled-notification.json":                   "schema/draft/examples/CancelledNotification/user-requested-cancellation.json",
		"header-mismatch-error.json":                    "schema/draft/examples/HeaderMismatchError/header-mismatch.json",
		"unsupported-version-error.json":                "schema/draft/examples/UnsupportedProtocolVersionError/unsupported-version.json",
		"invalid-tool-arguments-error.json":             "schema/draft/examples/InvalidParamsError/invalid-tool-arguments.json",
		"method-not-found-error.json":                   "schema/draft/examples/MethodNotFoundError/prompts-not-supported.json",
		"parse-error.json":                              "schema/draft/examples/ParseError/invalid-json.json",
		"tools-minimum-capability.json":                 "schema/draft/examples/ServerCapabilities/tools-minimum-baseline-support.json",
		"missing-required-client-capability-error.json": "schema/draft/examples/MissingRequiredClientCapabilityError/missing-elicitation-capability.json",
	}
	return paths[name]
}

func mapKeys[T any](items map[string]T) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	return keys
}

// wasmtestLocalResponse copies the fields used by assertions before TestHost
// cleanup releases the process-global proxy-wasm emulator.
type wasmtestLocalResponse struct {
	StatusCode uint32
	Data       []byte
	Headers    [][2]string
}

func readOfficialFixture(t *testing.T, name string) []byte {
	t.Helper()
	bytes, err := os.ReadFile(filepath.Join(officialFixtureDir, name))
	require.NoError(t, err)
	return bytes
}

func officialErrorCode(t *testing.T, name string) int64 {
	t.Helper()
	fixture := readOfficialFixture(t, name)
	if code := gjson.GetBytes(fixture, "error.code"); code.Exists() {
		return code.Int()
	}
	return gjson.GetBytes(fixture, "code").Int()
}
