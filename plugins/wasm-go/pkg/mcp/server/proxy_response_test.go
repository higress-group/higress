// Copyright (c) 2026 Alibaba Group Holding Ltd.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software distributed
// under the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
// CONDITIONS OF ANY KIND, either express or implied.

package server

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validAutoDiscoverResult = `{"resultType":"complete","supportedVersions":["2026-07-28"],"capabilities":{"tools":{}},"_meta":{"io.modelcontextprotocol/serverInfo":{"name":"backend","version":"1"}},"ttlMs":0,"cacheScope":"private"}`
const validAutoInitResult = `{"protocolVersion":"2025-03-26","capabilities":{"tools":{}},"serverInfo":{"name":"backend","version":"1"}}`

func autoResult(id, result string) []byte {
	return []byte(`{"jsonrpc":"2.0","id":` + id + `,"result":` + result + `}`)
}
func autoError(id string, code int, data string) []byte {
	extra := ""
	if data != "" {
		extra = `,"data":` + data
	}
	return []byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"error":{"code":%d,"message":"backend message"%s}}`, id, code, extra))
}
func jsonAutoResponse(status int, body []byte) autoHTTPResponse {
	return autoHTTPResponse{status: status, headers: [][2]string{{"Content-Type", "application/json"}}, body: body}
}

func TestAutoProbeClassifier(t *testing.T) {
	id := `"probe"`
	for _, tc := range []struct {
		name    string
		status  int
		body    []byte
		profile ProtocolStrategy
		version protocol.Version
		reason  string
	}{
		{"modern", 200, autoResult(id, validAutoDiscoverResult), ProtocolStrategyModern, protocol.Version20260728, ""},
		{"no tools", 200, autoResult(id, strings.Replace(validAutoDiscoverResult, `"tools":{}`, `"resources":{}`, 1)), "", "", "tools_unavailable"},
		{"no version", 200, autoResult(id, strings.Replace(validAutoDiscoverResult, "2026-07-28", "2025-11-25", 1)), "", "", "unsupported_discover_version"},
		{"invalid result type", 200, autoResult(id, strings.Replace(validAutoDiscoverResult, "complete", "input_required", 1)), "", "", "invalid_discover_result"},
		{"missing server info", 200, autoResult(id, strings.Replace(validAutoDiscoverResult, `"name":"backend"`, `"title":"backend"`, 1)), "", "", "invalid_server_info"},
		{"invalid ttl", 200, autoResult(id, strings.Replace(validAutoDiscoverResult, `"ttlMs":0`, `"ttlMs":-1`, 1)), "", "", "invalid_discover_ttl"},
		{"header mismatch", 400, autoError(id, -32020, ""), "", "", "upstream_protocol_error"},
		{"missing capability", 400, autoError("null", -32021, `{"requiredCapabilities":{"sampling":{}}}`), "", "", "upstream_protocol_error"},
		{"modern method missing", 404, autoError(id, -32601, ""), "", "", "upstream_protocol_error"},
		{"legacy method missing", 200, autoError(id, -32601, ""), ProtocolStrategyLegacy, protocol.Version20250326, ""},
		{"modern evidence", 200, []byte(`{"jsonrpc":"2.0","id":"probe","_meta":{},"error":{"code":-32601,"message":"missing"}}`), "", "", "upstream_protocol_error"},
		{"version preference", 400, autoError("null", -32022, `{"requested":"2026-07-28","supported":["2025-03-26","2025-06-18"]}`), ProtocolStrategyLegacy, protocol.Version20250618, ""},
		{"old HTTP version", 400, autoError(id, -32022, `{"requested":"2026-07-28","supported":["2025-03-26"]}`), ProtocolStrategyLegacy, protocol.Version20250326, ""},
		{"SSE only version", 400, autoError(id, -32022, `{"requested":"2026-07-28","supported":["2024-11-05"]}`), "", "", "upstream_protocol_error"},
		{"unimplemented version", 400, autoError(id, -32022, `{"requested":"2026-07-28","supported":["2025-11-25"]}`), "", "", "upstream_protocol_error"},
		{"contradictory version", 400, autoError(id, -32022, `{"requested":"2026-07-28","supported":["2026-07-28","2025-03-26"]}`), "", "", "invalid_version_error"},
		{"missing requested", 400, autoError(id, -32022, `{"supported":["2025-03-26"]}`), "", "", "invalid_version_error"},
		{"wrong requested", 400, autoError(id, -32022, `{"requested":"2025-03-26","supported":["2025-03-26"]}`), "", "", "invalid_version_error"},
		{"malformed reserved", 400, autoError(`"wrong"`, -32022, `{"requested":"2026-07-28","supported":["2025-03-26"]}`), "", "", "invalid_modern_error"},
		{"malformed success", 200, []byte(`bad json`), "", "", "invalid_probe_response"},
		{"wrong id success", 200, autoResult(`"wrong"`, validAutoDiscoverResult), "", "", "invalid_probe_response"},
		{"wrong id error", 404, autoError(`"wrong"`, -32601, ""), "", "", "invalid_probe_response"},
		{"batch", 200, []byte(`[` + string(autoResult(id, validAutoDiscoverResult)) + `]`), "", "", "invalid_probe_response"},
		{"legacy 400", 400, []byte(`old request rejected`), ProtocolStrategyLegacy, protocol.Version20250326, ""},
		{"legacy 404", 404, nil, ProtocolStrategyLegacy, protocol.Version20250326, ""},
		{"legacy 405", 405, []byte(`<html>method not allowed</html>`), ProtocolStrategyLegacy, protocol.Version20250326, ""},
		{"authentication", 401, autoError(id, -32022, `{"requested":"2026-07-28","supported":["2025-03-26"]}`), "", "", "upstream_rejected"},
		{"forbidden", 403, nil, "", "", "upstream_rejected"},
		{"throttled", 429, nil, "", "", "upstream_rejected"},
		{"upstream 500", 500, nil, "", "", "upstream_unavailable"},
		{"upstream timeout status", 504, nil, "", "", "upstream_timeout"},
		{"no response", 0, nil, "", "", "upstream_unavailable"},
		{"redirect", 302, nil, "", "", "unexpected_probe_status"},
		{"accepted without body", 202, nil, "", "", "unexpected_probe_status"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := classifyAutoProbe(jsonAutoResponse(tc.status, tc.body), []byte(id))
			if tc.reason != "" {
				require.NotNil(t, result.failure)
				assert.Equal(t, tc.reason, result.failure.reason)
				return
			}
			require.Nil(t, result.failure)
			assert.Equal(t, tc.profile, result.profile)
			assert.Equal(t, tc.version, result.version)
		})
	}
}

func TestAutoResponseFraming(t *testing.T) {
	result := string(autoResult(`9007199254740993`, `{"opaque":900719925474099312345}`))
	notification := `{"jsonrpc":"2.0","method":"notifications/progress","params":{"progressToken":"p","progress":1}}`
	for _, tc := range []struct {
		name, body, media string
		valid             bool
	}{
		{"json", result, "application/json", true},
		{"json charset", result, "application/json; charset=utf-8", true},
		{"sse comments notifications multiline", ": keepalive\r\n\r\ndata:" + notification + "\r\n\r\ndata: {\r\ndata: " + result[1:] + "\r\n\r\n", "text/event-stream", true},
		{"sse no space", "data:" + result + "\n\n", "text/event-stream", true},
		{"duplicate final", "data:" + result + "\n\ndata:" + result + "\n\n", "text/event-stream", false},
		{"missing final", "data:" + notification + "\n\n", "text/event-stream", false},
		{"incomplete event", "data:" + result + "\n", "text/event-stream", false},
		{"server request", `data:{"jsonrpc":"2.0","id":1,"method":"sampling/createMessage"}` + "\n\n", "text/event-stream", false},
		{"wrong id", strings.Replace(result, "9007199254740993", "9007199254740992", 1), "application/json", false},
		{"string id", strings.Replace(result, "9007199254740993", `"9007199254740993"`, 1), "application/json", false},
		{"trailing json", result + ` {}`, "application/json", false},
		{"result and error", strings.Replace(result, `"result":`, `"error":{"code":-1,"message":"error"},"result":`, 1), "application/json", false},
		{"duplicate keys", strings.Replace(result, `"jsonrpc":`, `"jsonrpc":"2.0","jsonrpc":`, 1), "application/json", false},
		{"non utf8", strings.Replace(result, "opaque", string([]byte{255}), 1), "application/json", false},
		{"text", result, "text/plain", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := autoHTTPResponse{status: 200, body: []byte(tc.body), headers: [][2]string{{"Content-Type", tc.media}}}
			envelope, err := decodeAutoResponse(r, []byte(`9007199254740993`))
			if !tc.valid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, `900719925474099312345`, string(envelope.result["opaque"]))
		})
	}
}

func TestAutoResponseNoIDAndStrictInitialization(t *testing.T) {
	for _, status := range []int{200, 400} {
		_, err := decodeAutoResponse(jsonAutoResponse(status, autoError("null", -32020, "")), []byte(`"probe"`))
		assert.Equal(t, status == 200, err != nil)
	}
	for _, tc := range []struct {
		version   string
		supported []string
		valid     bool
	}{
		{"2025-03-26", nil, true}, {"2025-06-18", nil, true}, {"2024-11-05", nil, false}, {"2025-11-25", nil, false},
		{"2025-03-26", []string{"2025-06-18"}, false},
	} {
		envelope, err := decodeAutoResponse(jsonAutoResponse(200, autoResult(`"init"`, strings.Replace(validAutoInitResult, "2025-03-26", tc.version, 1))), []byte(`"init"`))
		require.NoError(t, err)
		_, err = validateAutoInitialize(envelope, tc.supported)
		assert.Equal(t, tc.valid, err == nil)
	}
	for _, status := range []int{200, 202, 204, 400, 401, 429, 500} {
		failure := initializedAutoFailure(jsonAutoResponse(status, nil))
		assert.Equal(t, status >= 300, failure != nil)
	}
	require.NotNil(t, initializedAutoFailure(jsonAutoResponse(200, autoError("null", -32603, ""))))
}

func TestAutoControlErrorPreservesOnlySafeData(t *testing.T) {
	raw := autoError(`"probe"`, -32022, `{"requested":"2026-07-28","supported":["2025-11-25"],"secret":"credential"}`)
	r := jsonAutoResponse(400, raw)
	envelope, err := decodeAutoResponse(r, []byte(`"probe"`))
	require.NoError(t, err)
	failure := protocolAutoFailure(r, envelope)
	encoded, err := json.Marshal(failure.data)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "credential")
	assert.Contains(t, string(encoded), "2025-11-25")
}

func TestAutoMalformedSSEErrorsNeverFallBack(t *testing.T) {
	for _, body := range []string{
		"data:" + string(autoError(`\"wrong\"`, -32022, `{"requested":"2026-07-28","supported":["2025-03-26"]}`)) + "\n\n",
		"data:" + string(autoError(`"wrong"`, -32601, "")) + "\n\n",
		"data:" + string(autoError(`"probe"`, -32601, "")) + "\n\ndata:" + string(autoError(`"probe"`, -32601, "")) + "\n\n",
	} {
		response := autoHTTPResponse{status: 400, body: []byte(body), headers: [][2]string{{"Content-Type", "text/event-stream"}}}
		require.NotNil(t, classifyAutoProbe(response, []byte(`"probe"`)).failure)
	}
	// A redirect must remain a terminal transport result even with a valid
	// advertised-version error body; the plugin never follows another URL.
	response := jsonAutoResponse(307, autoError(`"probe"`, -32022, `{"requested":"2026-07-28","supported":["2025-03-26"]}`))
	require.NotNil(t, classifyAutoProbe(response, []byte(`"probe"`)).failure)
}

func TestAutoModernEvidenceAndNotificationAcknowledgement(t *testing.T) {
	response := jsonAutoResponse(200, autoError(`"probe"`, -32601, ""))
	response.headers = append(response.headers, [2]string{protocol.HeaderProtocolVersion, "2026-07-28"})
	require.NotNil(t, classifyAutoProbe(response, []byte(`"probe"`)).failure)
	require.Nil(t, initializedAutoFailure(jsonAutoResponse(200, []byte("OK"))))
	response = jsonAutoResponse(202, autoError("null", -32603, ""))
	failure := initializedAutoFailure(response)
	require.NotNil(t, failure)
	assert.Equal(t, protocol.CodeInternalError, failure.code)
}
