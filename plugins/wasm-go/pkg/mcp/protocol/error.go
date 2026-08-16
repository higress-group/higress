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
	"encoding/json"
	"slices"
)

const (
	CodeParseError                      = -32700
	CodeInvalidRequest                  = -32600
	CodeMethodNotFound                  = -32601
	CodeInvalidParams                   = -32602
	CodeInternalError                   = -32603
	CodeHeaderMismatch                  = -32020
	CodeMissingRequiredClientCapability = -32021
	CodeUnsupportedVersion              = -32022
)

// ErrorData is the typed data contract for modern protocol errors.
type ErrorData struct {
	Supported            []Version           `json:"supported,omitempty"`
	Requested            Version             `json:"requested,omitempty"`
	RequiredCapabilities *ClientCapabilities `json:"requiredCapabilities,omitempty"`
}

// Error is a transport-aware JSON-RPC error. Messages are deliberately fixed
// and must not contain request parameters, credentials, or other hostile input.
type Error struct {
	HTTPStatus uint32
	Code       int
	Message    string
	Data       *ErrorData
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func newError(httpStatus uint32, code int, message string) *Error {
	return &Error{HTTPStatus: httpStatus, Code: code, Message: message}
}

func ParseError() *Error {
	return newError(400, CodeParseError, "parse error")
}

func InvalidRequest(message string) *Error {
	return newError(400, CodeInvalidRequest, message)
}

func InvalidParams(message string) *Error {
	return newError(400, CodeInvalidParams, message)
}

func HeaderMismatch() *Error {
	return newError(400, CodeHeaderMismatch, "MCP header does not match request body")
}

func MissingRequiredClientCapability(requiredCapabilities ClientCapabilities) *Error {
	required := cloneClientCapabilities(requiredCapabilities)
	protocolError := newError(400, CodeMissingRequiredClientCapability, "missing required client capability")
	protocolError.Data = &ErrorData{RequiredCapabilities: &required}
	return protocolError
}

func UnsupportedVersion(requested Version, supported []Version) *Error {
	protocolError := newError(400, CodeUnsupportedVersion, "unsupported MCP protocol version")
	protocolError.Data = &ErrorData{
		Supported: slices.Clone(supported),
		Requested: requested,
	}
	return protocolError
}

func MethodNotFound() *Error {
	return newError(404, CodeMethodNotFound, "method not found")
}

func MethodNotAllowed() *Error {
	return newError(405, CodeInvalidRequest, "modern MCP requires HTTP POST")
}

func UnsupportedMediaType() *Error {
	return newError(415, CodeInvalidRequest, "unsupported Content-Type")
}

func NotAcceptable() *Error {
	return newError(406, CodeInvalidRequest, "unacceptable response media type")
}

func UntrustedOrigin() *Error {
	return newError(403, CodeInvalidRequest, "untrusted Origin")
}

func RequestTooLarge() *Error {
	return newError(413, CodeInvalidRequest, "request body exceeds the modern MCP limit")
}

type errorObject struct {
	Code    int        `json:"code"`
	Message string     `json:"message"`
	Data    *ErrorData `json:"data,omitempty"`
}

type errorEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Error   errorObject     `json:"error"`
}

// MarshalErrorResponse creates the exact JSON-RPC error envelope for a
// rejected request. Invalid or unavailable IDs are represented as null.
func MarshalErrorResponse(id ID, protocolError *Error) []byte {
	rawID := json.RawMessage("null")
	if id.IsPresent() {
		rawID = id.Raw()
	}
	body, _ := json.Marshal(errorEnvelope{
		JSONRPC: "2.0",
		ID:      rawID,
		Error: errorObject{
			Code:    protocolError.Code,
			Message: protocolError.Message,
			Data:    protocolError.Data,
		},
	})
	return body
}
