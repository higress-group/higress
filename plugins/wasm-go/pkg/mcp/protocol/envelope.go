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

// Envelope is a strict, single JSON-RPC request or notification.
type Envelope struct {
	ID           ID
	Method       string
	Params       json.RawMessage
	Notification bool
	Raw          json.RawMessage
}

type rawEnvelope struct {
	JSONRPC json.RawMessage `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  json.RawMessage `json:"method"`
	Params  json.RawMessage `json:"params"`
	Result  json.RawMessage `json:"result"`
	Error   json.RawMessage `json:"error"`
}

func decodeEnvelope(body []byte) (Envelope, *Error) {
	if len(body) == 0 {
		return Envelope{}, ParseError()
	}
	if len(body) > int(ModernMaxBodyBytes) {
		return Envelope{}, RequestTooLarge()
	}
	if err := validateUniqueJSON(body); err != nil {
		return Envelope{}, ParseError()
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return Envelope{}, ParseError()
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return Envelope{}, InvalidRequest("exactly one JSON-RPC message is required")
		}
		return Envelope{}, ParseError()
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return Envelope{}, InvalidRequest("JSON-RPC batch and non-object envelopes are not supported")
	}
	var decoded rawEnvelope
	if err := json.Unmarshal(trimmed, &decoded); err != nil {
		return Envelope{}, ParseError()
	}
	if len(decoded.Result) != 0 || len(decoded.Error) != 0 {
		return Envelope{}, InvalidRequest("JSON-RPC response envelopes are not accepted")
	}
	var jsonrpc string
	if err := json.Unmarshal(decoded.JSONRPC, &jsonrpc); err != nil || jsonrpc != "2.0" {
		return Envelope{}, InvalidRequest("jsonrpc must be 2.0")
	}
	var method string
	if err := json.Unmarshal(decoded.Method, &method); err != nil || strings.TrimSpace(method) == "" {
		return Envelope{}, InvalidRequest("method must be a non-empty string")
	}
	if len(decoded.Params) != 0 && !isJSONObject(decoded.Params) {
		return Envelope{}, InvalidRequest("params must be an object")
	}
	id, protocolError := decodeID(decoded.ID)
	if protocolError != nil {
		return Envelope{}, protocolError
	}
	return Envelope{
		ID:           id,
		Method:       method,
		Params:       bytes.Clone(decoded.Params),
		Notification: !id.IsPresent(),
		Raw:          bytes.Clone(trimmed),
	}, nil
}

func decodeID(raw json.RawMessage) (ID, *Error) {
	if len(raw) == 0 {
		return ID{}, nil
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return ID{}, InvalidRequest("JSON-RPC id must be a string or integer")
	}
	if trimmed[0] == '"' {
		var value string
		if err := json.Unmarshal(trimmed, &value); err != nil {
			return ID{}, InvalidRequest("JSON-RPC id must be a string or integer")
		}
		return ID{raw: bytes.Clone(trimmed), present: true}, nil
	}
	if strings.ContainsAny(string(trimmed), ".eE") {
		return ID{}, InvalidRequest("JSON-RPC id must be a string or integer")
	}
	if _, err := strconv.ParseInt(string(trimmed), 10, 64); err != nil {
		return ID{}, InvalidRequest("JSON-RPC id must be a string or integer")
	}
	return ID{raw: bytes.Clone(trimmed), present: true}, nil
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
