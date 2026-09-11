// Copyright (c) 2026 Alibaba Group Holding Ltd.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software distributed
// under the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
// CONDITIONS OF ANY KIND, either express or implied.

package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"mime"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/protocol"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

const autoProbeBodyLimit = 1 << 20

var errAutoResponseTooLarge = errors.New("probe_response_too_large")

type autoHTTPResponse struct {
	status  int
	headers [][2]string
	body    []byte
	err     error
}

type autoEnvelope struct {
	raw      json.RawMessage
	members  protocol.JSONObject
	result   protocol.JSONObject
	rpcError protocol.JSONObject
	code     int
}

func rawString(raw json.RawMessage) (string, bool) {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.TrimSpace(raw)[0] != '"' {
		return "", false
	}
	var s string
	err := json.Unmarshal(raw, &s)
	return s, err == nil
}

func sameRPCID(a, b json.RawMessage) bool {
	if as, ok := rawString(a); ok {
		bs, ok := rawString(b)
		return ok && as == bs
	}
	// Request IDs are validated integers; compare their exact numeric spelling,
	// never float64. JSON strings and numeric IDs are different identities.
	return len(a) > 0 && bytes.Equal(bytes.TrimSpace(a), bytes.TrimSpace(b))
}

func decodeAutoEnvelope(body, id []byte, allowNoID, allowNotification bool) (*autoEnvelope, error) {
	members, err := protocol.DecodeSingleJSONObject(body)
	if err != nil {
		return nil, errors.New("invalid_json")
	}
	version, _ := rawString(members["jsonrpc"])
	if version != "2.0" {
		return nil, errors.New("invalid_envelope")
	}
	result, hasResult := members["result"]
	rpcError, hasError := members["error"]
	if method, hasMethod := members["method"]; hasMethod {
		name, valid := rawString(method)
		_, hasID := members["id"]
		if !allowNotification || hasID || hasResult || hasError || !valid || !strings.HasPrefix(name, "notifications/") {
			return nil, errors.New("unexpected_server_request")
		}
		if params, ok := members["params"]; ok {
			if _, err := protocol.DecodeSingleJSONObject(params); err != nil {
				return nil, errors.New("invalid_notification")
			}
		}
		return nil, nil
	}
	if hasResult == hasError {
		return nil, errors.New("invalid_envelope")
	}
	envelope := &autoEnvelope{raw: bytes.Clone(body), members: members}
	if hasResult {
		envelope.result, err = protocol.DecodeSingleJSONObject(result)
	} else {
		envelope.rpcError, err = protocol.DecodeSingleJSONObject(rpcError)
		if err == nil {
			var code int
			if json.Unmarshal(envelope.rpcError["code"], &code) != nil {
				return nil, errors.New("invalid_error")
			}
			if _, ok := rawString(envelope.rpcError["message"]); !ok {
				return nil, errors.New("invalid_error")
			}
			envelope.code = code
		}
	}
	if err != nil {
		return nil, errors.New("invalid_envelope")
	}
	responseID, hasID := members["id"]
	noID := !hasID || bytes.Equal(bytes.TrimSpace(responseID), []byte("null"))
	if !(hasError && noID && allowNoID) && !sameRPCID(responseID, id) {
		return nil, errors.New("response_id_mismatch")
	}
	return envelope, nil
}

// decodeAutoResponse consumes complete SSE events, not the first data line.
// Notifications can precede a single final response; they are not forwarded.
func decodeAutoResponse(r autoHTTPResponse, id []byte) (*autoEnvelope, error) {
	if r.err != nil {
		return nil, r.err
	}
	contentType, _ := autoHeader(r.headers, "content-type")
	media, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, errors.New("invalid_content_type")
	}
	if media == "application/json" {
		return decodeAutoEnvelope(r.body, id, r.status >= 400 || len(id) == 0, false)
	}
	if media != "text/event-stream" || !utf8.Valid(r.body) {
		return nil, errors.New("invalid_content_type")
	}
	var final *autoEnvelope
	var data []string
	lines := strings.Split(strings.ReplaceAll(string(r.body), "\r\n", "\n"), "\n")
	for i, line := range lines {
		// Split's final empty element is not another received newline/event boundary.
		if i == len(lines)-1 && line == "" {
			break
		}
		if line == "" {
			if len(data) == 0 {
				continue
			}
			envelope, err := decodeAutoEnvelope([]byte(strings.Join(data, "\n")), id, r.status >= 400 || len(id) == 0, true)
			if err != nil {
				return nil, err
			}
			data = nil
			if envelope != nil {
				if final != nil {
					return nil, errors.New("multiple_final_responses")
				}
				final = envelope
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, _ := strings.Cut(line, ":")
		if strings.HasPrefix(value, " ") {
			value = value[1:]
		}
		if field == "data" {
			data = append(data, value)
		}
	}
	if len(data) > 0 {
		return nil, errors.New("unterminated_sse_event")
	}
	if final == nil {
		return nil, errors.New("missing_final_response")
	}
	return final, nil
}

type autoFailure struct {
	status  int
	code    int
	reason  string
	data    protocol.JSONObject
	headers [][2]string
}

func gatewayAutoFailure(reason string) *autoFailure {
	return &autoFailure{status: 502, code: protocol.CodeInternalError, reason: reason}
}

func operationalAutoFailure(r autoHTTPResponse) *autoFailure {
	if r.err != nil {
		return gatewayAutoFailure(r.err.Error())
	}
	if r.status == 401 || r.status == 403 || r.status == 429 {
		return &autoFailure{status: r.status, code: protocol.CodeInternalError, reason: "upstream_rejected", headers: safeAutoResponseHeaders(r.headers)}
	}
	if r.status >= 300 && r.status < 400 {
		return gatewayAutoFailure("unexpected_probe_status")
	}
	if r.status == 504 {
		return &autoFailure{status: 504, code: protocol.CodeInternalError, reason: "upstream_timeout"}
	}
	if r.status < 100 || r.status >= 500 {
		return gatewayAutoFailure("upstream_unavailable")
	}
	return nil
}

func safeAutoResponseHeaders(headers [][2]string) [][2]string {
	safe := [][2]string{{"Content-Type", "application/json"}}
	for _, h := range headers {
		if (strings.EqualFold(h[0], "WWW-Authenticate") || strings.EqualFold(h[0], "Retry-After")) && !strings.ContainsAny(h[1], "\r\n") {
			safe = append(safe, h)
		}
	}
	return safe
}

func protocolAutoFailure(r autoHTTPResponse, e *autoEnvelope) *autoFailure {
	status := r.status
	if status < 200 || status >= 300 && status < 400 {
		status = 502
	}
	failure := &autoFailure{status: status, code: e.code, reason: "upstream_protocol_error", headers: safeAutoResponseHeaders(r.headers)}
	// Control messages must not reflect arbitrary upstream payloads or messages.
	// Retain only the structured version/capability fields after shape checks.
	if data, err := protocol.DecodeSingleJSONObject(e.rpcError["data"]); err == nil {
		safe := protocol.JSONObject{}
		if requested, ok := rawString(data["requested"]); ok && validVersionLabel(requested) {
			safe["requested"] = data["requested"]
		}
		if _, ok := decodeVersionList(data["supported"]); ok {
			safe["supported"] = data["supported"]
		}
		if raw, ok := data["requiredCapabilities"]; ok {
			var caps protocol.ClientCapabilities
			if json.Unmarshal(raw, &caps) == nil && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				safe["requiredCapabilities"] = raw
			}
		}
		if len(safe) > 0 {
			failure.data = safe
		}
	}
	return failure
}

type autoProbeDecision struct {
	profile   ProtocolStrategy
	version   protocol.Version
	supported []string
	reason    string
	failure   *autoFailure
}

func validVersionLabel(s string) bool {
	if len(s) != 10 || s[4] != '-' || s[7] != '-' {
		return false
	}
	for i, c := range s {
		if i != 4 && i != 7 && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

func decodeVersionList(raw []byte) ([]string, bool) {
	var versions []string
	if json.Unmarshal(raw, &versions) != nil || len(versions) == 0 {
		return nil, false
	}
	seen := map[string]bool{}
	for _, v := range versions {
		if !validVersionLabel(v) || seen[v] {
			return nil, false
		}
		seen[v] = true
	}
	return versions, true
}

func validServerInfo(raw []byte) bool {
	info, err := protocol.DecodeSingleJSONObject(raw)
	if err != nil {
		return false
	}
	name, nok := rawString(info["name"])
	version, vok := rawString(info["version"])
	return nok && vok && name != "" && version != ""
}

func validateDiscoverResult(result protocol.JSONObject) string {
	resultType, _ := rawString(result["resultType"])
	if resultType != resultTypeComplete {
		return "invalid_discover_result"
	}
	versions, ok := decodeVersionList(result["supportedVersions"])
	if !ok || !slices.Contains(versions, string(protocol.Version20260728)) {
		return "unsupported_discover_version"
	}
	meta, err := protocol.DecodeSingleJSONObject(result["_meta"])
	if err != nil || !validServerInfo(meta[serverInfoMetaKey]) {
		return "invalid_server_info"
	}
	var ttl json.Number
	rawTTL := bytes.TrimSpace(result["ttlMs"])
	if len(rawTTL) == 0 || rawTTL[0] < '0' || rawTTL[0] > '9' || json.Unmarshal(rawTTL, &ttl) != nil {
		return "invalid_discover_ttl"
	}
	ttlValue, err := strconv.ParseInt(string(ttl), 10, 64)
	if err != nil || ttlValue < 0 {
		return "invalid_discover_ttl"
	}
	scope, _ := rawString(result["cacheScope"])
	if scope != "public" && scope != "private" {
		return "invalid_discover_scope"
	}
	capabilities, err := protocol.DecodeSingleJSONObject(result["capabilities"])
	if err != nil {
		return "invalid_capabilities"
	}
	if _, ok := capabilities["tools"]; !ok {
		return "tools_unavailable"
	}
	if _, err := protocol.DecodeSingleJSONObject(capabilities["tools"]); err != nil {
		return "invalid_tools_capability"
	}
	return ""
}

// Check error hints even when a reserved error has a malformed ID/envelope.
// SSE frames are inspected as complete data fields, including multiline data.
func hasReservedAutoError(body []byte) bool { return hasAutoErrorHint(body, true) }

func hasAutoErrorHint(body []byte, reservedOnly bool) bool {
	reserved := func(raw []byte) bool {
		if !reservedOnly {
			return gjson.GetBytes(raw, "error").Exists()
		}
		code := gjson.GetBytes(raw, "error.code").Int()
		return code == protocol.CodeHeaderMismatch || code == protocol.CodeMissingRequiredClientCapability || code == protocol.CodeUnsupportedVersion
	}
	if reserved(body) {
		return true
	}
	var data []string
	for _, line := range strings.Split(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n") {
		if line == "" {
			if reserved([]byte(strings.Join(data, "\n"))) {
				return true
			}
			data = nil
			continue
		}
		field, value, _ := strings.Cut(line, ":")
		if field == "data" {
			data = append(data, strings.TrimPrefix(value, " "))
		}
	}
	return reserved([]byte(strings.Join(data, "\n")))
}

func autoModernEvidence(r autoHTTPResponse, e *autoEnvelope) bool {
	version, _ := autoHeader(r.headers, protocol.HeaderProtocolVersion)
	if version == string(protocol.Version20260728) {
		return true
	}
	if e == nil {
		return false
	}
	if _, present := e.members["_meta"]; present {
		return true
	}
	if _, present := e.rpcError["_meta"]; present {
		return true
	}
	data, _ := protocol.DecodeSingleJSONObject(e.rpcError["data"])
	requested, _ := rawString(data["requested"])
	_, capabilities := data["requiredCapabilities"]
	return requested == string(protocol.Version20260728) || capabilities
}

func classifyAutoProbe(r autoHTTPResponse, id []byte) autoProbeDecision {
	failed := func(reason string) autoProbeDecision { return autoProbeDecision{failure: gatewayAutoFailure(reason)} }
	if failure := operationalAutoFailure(r); failure != nil {
		return autoProbeDecision{failure: failure}
	}
	envelope, err := decodeAutoResponse(r, id)
	// A malformed reserved modern error cannot be mistaken for legacy just
	// because its outer HTTP status is 400/404/405.
	if err != nil && hasReservedAutoError(r.body) {
		return failed("invalid_modern_error")
	}
	if err == nil && envelope.rpcError != nil {
		switch envelope.code {
		case protocol.CodeHeaderMismatch:
			return autoProbeDecision{failure: protocolAutoFailure(r, envelope)}
		case protocol.CodeMissingRequiredClientCapability:
			data, dataErr := protocol.DecodeSingleJSONObject(envelope.rpcError["data"])
			var capabilities protocol.ClientCapabilities
			if dataErr != nil || json.Unmarshal(data["requiredCapabilities"], &capabilities) != nil {
				return failed("invalid_modern_error")
			}
			return autoProbeDecision{failure: protocolAutoFailure(r, envelope)}
		case protocol.CodeUnsupportedVersion:
			data, dataErr := protocol.DecodeSingleJSONObject(envelope.rpcError["data"])
			versions, ok := decodeVersionList(data["supported"])
			requested, requestedOK := rawString(data["requested"])
			if dataErr != nil || !ok || !requestedOK || requested != string(protocol.Version20260728) || slices.Contains(versions, requested) {
				return failed("invalid_version_error")
			}
			for _, version := range []protocol.Version{protocol.Version20250618, protocol.Version20250326} {
				if slices.Contains(versions, string(version)) {
					return autoProbeDecision{profile: ProtocolStrategyLegacy, version: version, supported: versions, reason: "advertised_legacy_version"}
				}
			}
			return autoProbeDecision{failure: protocolAutoFailure(r, envelope)}
		case protocol.CodeMethodNotFound:
			if r.status == 404 {
				return autoProbeDecision{failure: protocolAutoFailure(r, envelope)}
			}
			if r.status == 200 {
				if !autoModernEvidence(r, envelope) {
					return autoProbeDecision{profile: ProtocolStrategyLegacy, version: protocol.Version20250326, reason: "legacy_method_missing"}
				}
			}
		}
	}
	if r.status == 200 {
		if err != nil {
			return failed("invalid_probe_response")
		}
		if envelope.rpcError != nil {
			return autoProbeDecision{failure: protocolAutoFailure(r, envelope)}
		}
		if reason := validateDiscoverResult(envelope.result); reason != "" {
			return failed(reason)
		}
		return autoProbeDecision{profile: ProtocolStrategyModern, version: protocol.Version20260728, reason: "modern_discovered"}
	}
	if r.status == 400 || r.status == 404 || r.status == 405 {
		if autoModernEvidence(r, envelope) {
			if envelope != nil && envelope.rpcError != nil {
				return autoProbeDecision{failure: protocolAutoFailure(r, envelope)}
			}
			return failed("invalid_probe_response")
		}
		// Valid JSON with a wrong response ID or conflicting envelope is not a
		// legacy transport signal. Plain HTML/empty responses remain candidates.
		contentType, _ := autoHeader(r.headers, "Content-Type")
		if err != nil && (strings.Contains(strings.ToLower(contentType), "text/event-stream") ||
			(err.Error() != "invalid_json" && err.Error() != "invalid_content_type") ||
			(gjson.GetBytes(r.body, "jsonrpc").Exists() || bytes.HasPrefix(bytes.TrimSpace(r.body), []byte("[")))) {
			return failed("invalid_probe_response")
		}
		return autoProbeDecision{profile: ProtocolStrategyLegacy, version: protocol.Version20250326, reason: "legacy_http_candidate"}
	}
	return failed("unexpected_probe_status")
}

func validateAutoInitialize(e *autoEnvelope, supported []string) (protocol.Version, error) {
	version, ok := rawString(e.result["protocolVersion"])
	if !ok || (version != string(protocol.Version20250326) && version != string(protocol.Version20250618)) || (len(supported) > 0 && !slices.Contains(supported, version)) {
		return "", errors.New("incompatible_initialize_version")
	}
	if !validServerInfo(e.result["serverInfo"]) {
		return "", errors.New("invalid_initialize_server_info")
	}
	capabilities, err := protocol.DecodeSingleJSONObject(e.result["capabilities"])
	if err != nil {
		return "", errors.New("invalid_initialize_capabilities")
	}
	if _, err := protocol.DecodeSingleJSONObject(capabilities["tools"]); err != nil {
		return "", errors.New("tools_unavailable")
	}
	return protocol.Version(version), nil
}

func initializedAutoFailure(r autoHTTPResponse) *autoFailure {
	if failure := operationalAutoFailure(r); failure != nil {
		return failure
	}
	if r.status < 200 || r.status >= 300 {
		return gatewayAutoFailure("initialized_rejected")
	}
	if len(bytes.TrimSpace(r.body)) != 0 {
		// Notifications have no result envelope. A recognizable JSON-RPC error,
		// even inside a 2xx response, never authorizes business dispatch.
		if e, err := decodeAutoResponse(r, nil); err == nil && e.rpcError != nil {
			return protocolAutoFailure(r, e)
		}
		if hasAutoErrorHint(r.body, false) {
			return gatewayAutoFailure("initialized_error")
		}
		return nil
	}
	return nil
}

func sendAutoFailure(ctx wrapper.HttpContext, id []byte, phase string, failure *autoFailure) {
	// Defined alongside the response codec to keep every gateway-generated
	// message independent of arbitrary upstream text, URLs and credentials.
	errorObject := map[string]any{"code": failure.code, "message": "MCP upstream " + phase + ": " + failure.reason}
	if failure.data != nil {
		errorObject["data"] = failure.data
	}
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(id), "error": errorObject})
	headers := failure.headers
	if headers == nil {
		headers = safeAutoResponseHeaders(nil)
	}
	proxywasm.SendHttpResponseWithDetail(uint32(failure.status), "mcp-proxy:auto:"+phase+":"+failure.reason, headers, body, -1)
}

func autoHeader(headers [][2]string, name string) (string, bool) {
	for _, header := range headers {
		if strings.EqualFold(header[0], name) {
			return header[1], true
		}
	}
	return "", false
}
