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
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/tidwall/gjson"
)

const maxJSONDepth = 64

const (
	MetaProtocolVersion    = "io.modelcontextprotocol/protocolVersion"
	MetaClientCapabilities = "io.modelcontextprotocol/clientCapabilities"
	MetaClientInfo         = "io.modelcontextprotocol/clientInfo"
)

// ID is a validated JSON-RPC request ID. Notifications have no ID.
type ID struct {
	raw     json.RawMessage
	present bool
}

func (id ID) IsPresent() bool {
	return id.present
}

func (id ID) Raw() json.RawMessage {
	return bytes.Clone(id.raw)
}

// Envelope is a single JSON-RPC request or notification.
type Envelope struct {
	ID           ID
	Method       string
	Params       json.RawMessage
	Notification bool
	Raw          json.RawMessage
}

// decodeEnvelope checks message framing independently of the control decoder.
// Ordinary fields use the same first occurrence as the legacy gjson handlers.
func decodeEnvelope(body []byte, modern bool) (Envelope, *Error) {
	if !utf8.Valid(body) || !gjson.ValidBytes(body) {
		return Envelope{}, ParseError()
	}
	if !isJSONObject(body) {
		return Envelope{}, InvalidRequest("JSON-RPC batch and non-object envelopes are not supported")
	}
	owned := bytes.Clone(body)
	// Parameter slices share the request-owned body instead of retaining
	// another copy of a potentially 100 MiB arguments object.
	members := businessObjectMembers(owned)
	id, protocolError := ParseID(members["id"])
	envelope := Envelope{ID: id, Params: members["params"], Raw: owned}
	if protocolError != nil {
		return envelope, protocolError
	}
	if modern && ambiguousModernIdentity(owned) {
		return envelope, ParseError()
	}
	if len(members["result"]) != 0 || len(members["error"]) != 0 {
		return envelope, InvalidRequest("JSON-RPC response envelopes are not accepted")
	}
	if modern {
		var jsonrpc string
		if err := json.Unmarshal(members["jsonrpc"], &jsonrpc); err != nil || jsonrpc != "2.0" {
			return envelope, InvalidRequest("jsonrpc must be 2.0")
		}
	}
	method := gjson.ParseBytes(members["method"])
	if method.Type != gjson.String || strings.TrimSpace(method.String()) == "" {
		return envelope, InvalidRequest("method must be a non-empty string")
	}
	envelope.Method = method.String()
	envelope.Notification = !id.IsPresent()
	return envelope, nil
}

// ParseID retains an independently owned client token, including explicit null.
// The legacy handler's int64 view is deliberately separate from this identity.
func ParseID(raw json.RawMessage) (ID, *Error) {
	if len(raw) == 0 {
		return ID{}, nil
	}
	trimmed := bytes.TrimSpace(raw)
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil || !json.Valid(trimmed) {
		return ID{}, InvalidRequest("JSON-RPC id must be a string, number or null")
	}
	switch value.(type) {
	case nil, string, json.Number:
		return ID{raw: bytes.Clone(trimmed), present: true}, nil
	default:
		return ID{}, InvalidRequest("JSON-RPC id must be a string, number or null")
	}
}

// CorrelationID returns an ID accepted by strict upstreams. Callers generate
// internalID within their existing exchange and retain this ID for the client.
func (id ID) CorrelationID(internalID string) json.RawMessage {
	if id.present && len(id.raw) > 0 {
		if id.raw[0] == '"' {
			return id.Raw()
		}
		if !bytes.Equal(id.raw, []byte("-0")) && !bytes.ContainsAny(id.raw, ".eE") {
			if _, err := strconv.ParseInt(string(id.raw), 10, 64); err == nil {
				return id.Raw()
			}
		}
	}
	raw, _ := json.Marshal(internalID)
	return raw
}

// DecodeBusinessJSONObject validates one UTF-8 JSON object without imposing
// control-message duplicate/depth rules. Values borrow body and retain exact
// JSON number tokens; callers must keep the input alive and unchanged.
func DecodeBusinessJSONObject(body []byte) (JSONObject, error) {
	if !utf8.Valid(body) || !gjson.ValidBytes(body) {
		return nil, errors.New("invalid JSON")
	}
	if !isJSONObject(body) {
		return nil, errors.New("JSON-RPC batch and non-object envelopes are not supported")
	}
	return businessObjectMembers(body), nil
}

func businessObjectMembers(body []byte) JSONObject {
	members := make(JSONObject)
	gjson.ParseBytes(body).ForEach(func(key, value gjson.Result) bool {
		if _, exists := members[key.String()]; !exists {
			members[key.String()] = body[value.Index : value.Index+len(value.Raw)]
		}
		return true
	})
	return members
}

func ambiguousModernIdentity(body []byte) bool {
	methods, versions, names := 0, 0, 0
	gjson.ParseBytes(body).ForEach(func(key, value gjson.Result) bool {
		switch key.String() {
		case "method":
			methods++
		case "params":
			if !value.IsObject() {
				return true
			}
			value.ForEach(func(key, value gjson.Result) bool {
				switch key.String() {
				case "name":
					names++
				case "_meta":
					if value.IsObject() {
						value.ForEach(func(key, _ gjson.Result) bool {
							if key.String() == MetaProtocolVersion {
								versions++
							}
							return true
						})
					}
				}
				return true
			})
		}
		return true
	})
	method := gjson.GetBytes(body, "method").String()
	return methods > 1 || versions > 1 || (method == "tools/call" && names > 1)
}

func isJSONObject(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && trimmed[0] == '{'
}

func validateUniqueJSON(body []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := validateJSONValue(decoder, 0); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("trailing JSON value")
		}
		return err
	}
	return nil
}

func validateJSONValue(decoder *json.Decoder, depth int) error {
	if depth > maxJSONDepth {
		return errors.New("JSON nesting limit exceeded")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		keys := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("invalid object key")
			}
			if _, duplicate := keys[key]; duplicate {
				return errors.New("duplicate object key")
			}
			keys[key] = struct{}{}
			if err := validateJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim('}') {
			return errors.New("unterminated object")
		}
	case '[':
		for decoder.More() {
			if err := validateJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim(']') {
			return errors.New("unterminated array")
		}
	default:
		return errors.New("unexpected JSON delimiter")
	}
	return nil
}

// DecodeSingleJSONObject validates response framing without converting opaque
// values to float64. Its strict duplicate/depth rules belong to control flows.
func DecodeSingleJSONObject(body []byte) (JSONObject, error) {
	if !utf8.Valid(body) {
		return nil, errors.New("invalid JSON UTF-8")
	}
	if err := validateUniqueJSON(body); err != nil {
		return nil, err
	}
	return decodeJSONObject(body)
}
