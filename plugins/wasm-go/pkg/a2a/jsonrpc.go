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

// Package a2a contains bounded framing helpers for the A2A JSON-RPC and SSE
// bindings. It deliberately does not own Agent task state.
package a2a

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
)

const (
	DefaultMaxRequestBytes  = 4 << 20
	HardMaxRequestBytes     = 16 << 20
	DefaultMaxSSEEventBytes = 256 << 10
	MaxMetadataValueBytes   = 256
)

var (
	ErrOversized      = errors.New("a2a payload exceeds configured limit")
	ErrInvalidJSONRPC = errors.New("invalid A2A JSON-RPC envelope")
	ErrUnknownMethod  = errors.New("unknown A2A JSON-RPC method")
)

var canonicalMethods = map[string]string{
	"SendMessage":                      "SendMessage",
	"SendStreamingMessage":             "SendStreamingMessage",
	"GetTask":                          "GetTask",
	"ListTasks":                        "ListTasks",
	"CancelTask":                       "CancelTask",
	"SubscribeToTask":                  "SubscribeToTask",
	"CreateTaskPushNotificationConfig": "CreateTaskPushNotificationConfig",
	"GetTaskPushNotificationConfig":    "GetTaskPushNotificationConfig",
	"ListTaskPushNotificationConfigs":  "ListTaskPushNotificationConfigs",
	"DeleteTaskPushNotificationConfig": "DeleteTaskPushNotificationConfig",
	"GetExtendedAgentCard":             "GetExtendedAgentCard",
}

var legacyMethods = map[string]string{
	"message/send":                        "SendMessage",
	"message/stream":                      "SendStreamingMessage",
	"tasks/get":                           "GetTask",
	"tasks/list":                          "ListTasks",
	"tasks/cancel":                        "CancelTask",
	"tasks/resubscribe":                   "SubscribeToTask",
	"tasks/pushNotificationConfig/set":    "CreateTaskPushNotificationConfig",
	"tasks/pushNotificationConfig/get":    "GetTaskPushNotificationConfig",
	"tasks/pushNotificationConfig/list":   "ListTaskPushNotificationConfigs",
	"tasks/pushNotificationConfig/delete": "DeleteTaskPushNotificationConfig",
	"agent/getAuthenticatedExtendedCard":  "GetExtendedAgentCard",
}

// Metadata is the bounded protocol state that may be exposed to later filters.
// It intentionally excludes Parts, artifacts, credentials, and callback URLs.
type Metadata struct {
	Version         string
	Binding         string
	Method          string
	RequestID       string
	TaskID          string
	ContextID       string
	MessageID       string
	Tenant          string
	TaskState       string
	StreamEventType string
	ErrorCode       string
	ParseStatus     string
}

type envelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

type rpcError struct {
	Code json.RawMessage `json:"code"`
}

type identifiers struct {
	Kind           string       `json:"kind"`
	ID             string       `json:"id"`
	TaskID         string       `json:"taskId"`
	ContextID      string       `json:"contextId"`
	MessageID      string       `json:"messageId"`
	Tenant         string       `json:"tenant"`
	State          string       `json:"state"`
	Message        *identifiers `json:"message"`
	Task           *identifiers `json:"task"`
	Status         *identifiers `json:"status"`
	StatusUpdate   *identifiers `json:"statusUpdate"`
	ArtifactUpdate *identifiers `json:"artifactUpdate"`
}

// CanonicalMethod validates a method and maps an enabled 0.3 alias to its 1.0
// operation name. It never translates payload shapes.
func CanonicalMethod(method string, allowLegacy bool) (canonical string, legacy bool, err error) {
	if canonical, ok := canonicalMethods[method]; ok {
		return canonical, false, nil
	}
	if allowLegacy {
		if canonical, ok := legacyMethods[method]; ok {
			return canonical, true, nil
		}
	}
	return "", false, fmt.Errorf("%w: %s", ErrUnknownMethod, method)
}

// ParseRequest parses one bounded A2A JSON-RPC request.
func ParseRequest(body []byte, maxBytes int, version string, allowLegacy bool) (Metadata, error) {
	meta := Metadata{Version: version, Binding: "jsonrpc", ParseStatus: "invalid"}
	env, err := parseEnvelope(body, maxBytes)
	if err != nil {
		if errors.Is(err, ErrOversized) {
			meta.ParseStatus = "oversized"
		}
		return meta, err
	}
	if env.Method == "" {
		return meta, fmt.Errorf("%w: method is required", ErrInvalidJSONRPC)
	}
	canonical, legacy, err := CanonicalMethod(env.Method, allowLegacy)
	if err != nil {
		return meta, err
	}
	meta.Method = canonical
	meta.RequestID = bounded(rawScalar(env.ID))
	meta.ParseStatus = "parsed"
	if legacy {
		meta.ParseStatus = "legacy"
	}
	mergeIdentifiers(&meta, env.Params)
	return meta, nil
}

// ParseResponse parses one bounded unary JSON-RPC response. method is trusted
// request context, rather than a value accepted from the upstream payload.
func ParseResponse(body []byte, maxBytes int, version, method string) (Metadata, error) {
	meta := Metadata{Version: version, Binding: "jsonrpc", Method: bounded(method), ParseStatus: "invalid"}
	env, err := parseEnvelope(body, maxBytes)
	if err != nil {
		if errors.Is(err, ErrOversized) {
			meta.ParseStatus = "oversized"
		}
		return meta, err
	}
	if len(env.ID) == 0 || (len(env.Result) == 0 && env.Error == nil) {
		return meta, fmt.Errorf("%w: response requires id and result or error", ErrInvalidJSONRPC)
	}
	meta.RequestID = bounded(rawScalar(env.ID))
	meta.ParseStatus = "parsed"
	mergeIdentifiers(&meta, env.Result)
	if env.Error != nil {
		meta.StreamEventType = "error"
		meta.ErrorCode = bounded(rawScalar(env.Error.Code))
	}
	return meta, nil
}

func parseEnvelope(body []byte, maxBytes int) (envelope, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxRequestBytes
	}
	if maxBytes > HardMaxRequestBytes {
		maxBytes = HardMaxRequestBytes
	}
	if len(body) > maxBytes {
		return envelope{}, ErrOversized
	}
	var env envelope
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(&env); err != nil {
		return envelope{}, fmt.Errorf("%w: %v", ErrInvalidJSONRPC, err)
	}
	if err := decoder.Decode(&struct{}{}); errors.Is(err, io.EOF) && env.JSONRPC == "2.0" {
		return env, nil
	}
	return envelope{}, fmt.Errorf("%w: jsonrpc must be 2.0 and trailing data is forbidden", ErrInvalidJSONRPC)
}

func mergeIdentifiers(meta *Metadata, raw json.RawMessage) {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return
	}
	var ids identifiers
	if json.Unmarshal(raw, &ids) != nil {
		return
	}
	mergeOne(meta, &ids)
	mergeOne(meta, ids.Message)
	mergeOne(meta, ids.Task)
	if ids.Task != nil {
		mergeOne(meta, ids.Task.Status)
	}
	mergeOne(meta, ids.Status)
	if ids.StatusUpdate != nil {
		mergeOne(meta, ids.StatusUpdate)
		mergeOne(meta, ids.StatusUpdate.Status)
		meta.StreamEventType = "status"
	}
	if ids.ArtifactUpdate != nil {
		mergeOne(meta, ids.ArtifactUpdate)
		meta.StreamEventType = "artifact"
	}
	if meta.TaskID == "" {
		meta.TaskID = bounded(ids.ID)
	}
}

func mergeOne(meta *Metadata, ids *identifiers) {
	if ids == nil {
		return
	}
	if meta.TaskID == "" {
		meta.TaskID = bounded(ids.TaskID)
	}
	if meta.TaskID == "" {
		meta.TaskID = bounded(ids.ID)
	}
	if meta.ContextID == "" {
		meta.ContextID = bounded(ids.ContextID)
	}
	if meta.MessageID == "" {
		meta.MessageID = bounded(ids.MessageID)
	}
	if meta.Tenant == "" {
		meta.Tenant = bounded(ids.Tenant)
	}
	if meta.TaskState == "" {
		meta.TaskState = bounded(ids.State)
	}
	if meta.StreamEventType == "" {
		meta.StreamEventType = canonicalEventType(ids.Kind)
	}
}

func canonicalEventType(kind string) string {
	switch kind {
	case "task":
		return "task"
	case "message":
		return "message"
	case "status-update", "task-status-update":
		return "status"
	case "artifact", "artifact-update", "task-artifact-update":
		return "artifact"
	default:
		return ""
	}
}

func rawScalar(raw json.RawMessage) string {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var number json.Number
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&number) == nil {
		if _, err := strconv.ParseFloat(number.String(), 64); err == nil {
			return number.String()
		}
	}
	return ""
}

func bounded(value string) string {
	if len(value) > MaxMetadataValueBytes {
		return value[:MaxMetadataValueBytes]
	}
	return value
}
