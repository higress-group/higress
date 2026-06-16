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

package server

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -----------------------------------------------------------------------------
// NewMcpProtocolHandler
// -----------------------------------------------------------------------------

func TestNewMcpProtocolHandler(t *testing.T) {
	h := NewMcpProtocolHandler("http://backend.example/mcp", 5000)
	require.NotNil(t, h)
	assert.Equal(t, "http://backend.example/mcp", h.backendURL)
	assert.Equal(t, 5000, h.timeout)
	assert.Empty(t, h.sessionID, "fresh handler has no session id until Initialize runs")
}

// -----------------------------------------------------------------------------
// parseSSEResponse — fill the remaining branches
// -----------------------------------------------------------------------------

func TestParseSSEResponse_OnlyCommentsAndBlanks(t *testing.T) {
	// All non-data lines → must surface "no data field found".
	_, err := parseSSEResponse([]byte(": only a comment\n\n: another\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no data field")
}

func TestParseSSEResponse_TooLongLine(t *testing.T) {
	// Single data line larger than the scanner's 32MB max-token cap.
	big := strings.Repeat("x", 33*1024*1024)
	_, err := parseSSEResponse([]byte("data: " + big + "\n\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "32MB", "must surface the max-token overflow as a clear error")
}

func TestParseSSEResponse_MultipleDataLinesReturnsFirst(t *testing.T) {
	body := "data: first\n\ndata: second\n\n"
	out, err := parseSSEResponse([]byte(body))
	require.NoError(t, err)
	assert.Equal(t, "first", string(out), "the function returns the first data line and stops")
}

// -----------------------------------------------------------------------------
// createInitializeRequest / createToolsListRequest / createToolsCallRequest
// -----------------------------------------------------------------------------

func TestCreateInitializeRequest_StableShape(t *testing.T) {
	h := NewMcpProtocolHandler("http://backend.example/mcp", 5000)
	req := h.createInitializeRequest()

	assert.Equal(t, "2.0", req["jsonrpc"])
	assert.Equal(t, 1, req["id"])
	assert.Equal(t, "initialize", req["method"])

	params, ok := req["params"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "2025-03-26", params["protocolVersion"])

	clientInfo, ok := params["clientInfo"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "Higress-mcp-proxy", clientInfo["name"])
	assert.Equal(t, "1.0.0", clientInfo["version"])

	_, hasCaps := params["capabilities"]
	assert.True(t, hasCaps)
}

func TestCreateToolsListRequest_NoCursor(t *testing.T) {
	h := NewMcpProtocolHandler("http://backend.example/mcp", 5000)
	req := h.createToolsListRequest(nil)

	assert.Equal(t, "2.0", req["jsonrpc"])
	assert.Equal(t, 2, req["id"])
	assert.Equal(t, "tools/list", req["method"])

	params, ok := req["params"].(map[string]interface{})
	require.True(t, ok)
	_, hasCursor := params["cursor"]
	assert.False(t, hasCursor, "nil cursor must produce no cursor field")
}

func TestCreateToolsListRequest_EmptyStringCursor(t *testing.T) {
	h := NewMcpProtocolHandler("http://backend.example/mcp", 5000)
	empty := ""
	req := h.createToolsListRequest(&empty)

	params := req["params"].(map[string]interface{})
	_, hasCursor := params["cursor"]
	assert.False(t, hasCursor, "empty-string cursor is treated as absent")
}

func TestCreateToolsListRequest_WithCursor(t *testing.T) {
	h := NewMcpProtocolHandler("http://backend.example/mcp", 5000)
	c := "next-page"
	req := h.createToolsListRequest(&c)

	params := req["params"].(map[string]interface{})
	assert.Equal(t, "next-page", params["cursor"])
}

func TestCreateToolsCallRequest_StableShape(t *testing.T) {
	h := NewMcpProtocolHandler("http://backend.example/mcp", 5000)
	args := map[string]interface{}{"q": "hello", "limit": 5}
	req := h.createToolsCallRequest("search", args)

	assert.Equal(t, "2.0", req["jsonrpc"])
	assert.Equal(t, 3, req["id"])
	assert.Equal(t, "tools/call", req["method"])

	params, ok := req["params"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "search", params["name"])
	gotArgs, ok := params["arguments"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "hello", gotArgs["q"])
	assert.Equal(t, 5, gotArgs["limit"])
}

func TestCreateToolsCallRequest_NilArguments(t *testing.T) {
	h := NewMcpProtocolHandler("http://backend.example/mcp", 5000)
	req := h.createToolsCallRequest("noop", nil)
	params := req["params"].(map[string]interface{})
	assert.Equal(t, "noop", params["name"])
	args, ok := params["arguments"]
	require.True(t, ok)
	assert.Nil(t, args)
}

// -----------------------------------------------------------------------------
// ParseBackendResponse / IsBackendError — extra branches
// -----------------------------------------------------------------------------

func TestParseBackendResponse_StringErrorField(t *testing.T) {
	// JSON-RPC error field doesn't have to be an object — anything truthy works.
	body := []byte(`{"jsonrpc":"2.0","id":1,"error":"some-text"}`)
	resp, isErr, etype := ParseBackendResponse(body)
	require.NotNil(t, resp)
	assert.True(t, isErr)
	assert.Equal(t, "jsonrpc_error", etype)
}

func TestParseBackendResponse_NoResultNoError(t *testing.T) {
	// Valid JSON without result/error → not an error, but still parsed.
	body := []byte(`{"jsonrpc":"2.0","id":2}`)
	resp, isErr, etype := ParseBackendResponse(body)
	require.NotNil(t, resp)
	assert.False(t, isErr)
	assert.Empty(t, etype)
}

func TestParseBackendResponse_ResultIsErrorFalseNotAnError(t *testing.T) {
	body := []byte(`{"jsonrpc":"2.0","id":3,"result":{"isError":false}}`)
	_, isErr, etype := ParseBackendResponse(body)
	assert.False(t, isErr)
	assert.Empty(t, etype)
}

func TestParseBackendResponse_ResultNotAnObject(t *testing.T) {
	// result is a scalar — the isError-extraction branch is skipped.
	body := []byte(`{"jsonrpc":"2.0","id":3,"result":"ok"}`)
	_, isErr, etype := ParseBackendResponse(body)
	assert.False(t, isErr)
	assert.Empty(t, etype)
}

func TestIsBackendError_DelegatesToParse(t *testing.T) {
	cases := []struct {
		body    string
		isError bool
		etype   string
	}{
		{`{"error":{"code":-1}}`, true, "jsonrpc_error"},
		{`{"result":{"isError":true}}`, true, "result_isError"},
		{`{"result":{"isError":false}}`, false, ""},
		{`not json`, false, ""},
	}
	for _, c := range cases {
		isErr, etype := IsBackendError([]byte(c.body))
		assert.Equal(t, c.isError, isErr, "body=%s", c.body)
		assert.Equal(t, c.etype, etype, "body=%s", c.body)
	}
}

// -----------------------------------------------------------------------------
// McpSessionManagerImpl
// -----------------------------------------------------------------------------

func TestNewMcpSessionManagerImpl(t *testing.T) {
	m := NewMcpSessionManagerImpl()
	require.NotNil(t, m)
	require.NotNil(t, m.sessions)
	assert.Empty(t, m.sessions)
}

func TestSessionManager_CreateAndGet(t *testing.T) {
	m := NewMcpSessionManagerImpl()
	id, err := m.CreateSession("http://backend.example/mcp")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(id, "mcp-session-"))

	session, ok := m.GetSession(id)
	require.True(t, ok)
	assert.Equal(t, id, session.ID)
	assert.Equal(t, "http://backend.example/mcp", session.BackendURL)
	assert.False(t, session.CreatedAt.IsZero())
}

func TestSessionManager_GetSessionUpdatesLastUsed(t *testing.T) {
	m := NewMcpSessionManagerImpl()
	id, err := m.CreateSession("http://b")
	require.NoError(t, err)

	// Force a measurable gap so LastUsed changes monotonically.
	original := m.sessions[id].LastUsed
	time.Sleep(2 * time.Millisecond)

	s, ok := m.GetSession(id)
	require.True(t, ok)
	assert.True(t, s.LastUsed.After(original), "GetSession should refresh LastUsed")
}

func TestSessionManager_GetUnknownSession(t *testing.T) {
	m := NewMcpSessionManagerImpl()
	s, ok := m.GetSession("missing")
	assert.False(t, ok)
	assert.Nil(t, s)
}

func TestSessionManager_CleanupSession_Existing(t *testing.T) {
	m := NewMcpSessionManagerImpl()
	id, _ := m.CreateSession("http://b")
	m.CleanupSession(id)
	_, ok := m.GetSession(id)
	assert.False(t, ok)
}

func TestSessionManager_CleanupSession_NonExistent(t *testing.T) {
	m := NewMcpSessionManagerImpl()
	// Must not panic on unknown id.
	m.CleanupSession("never-existed")
	assert.Empty(t, m.sessions)
}

func TestSessionManager_CleanupExpiredSessions(t *testing.T) {
	m := NewMcpSessionManagerImpl()
	fresh, _ := m.CreateSession("fresh")
	stale, _ := m.CreateSession("stale")

	// Backdate the stale session.
	m.sessions[stale].LastUsed = time.Now().Add(-10 * time.Minute)

	m.CleanupExpiredSessions(1 * time.Minute)

	_, freshOk := m.sessions[fresh]
	_, staleOk := m.sessions[stale]
	assert.True(t, freshOk, "fresh session must remain")
	assert.False(t, staleOk, "stale session must be removed")
}

func TestSessionManager_CleanupExpiredSessions_EmptyMap(t *testing.T) {
	m := NewMcpSessionManagerImpl()
	// Must not panic on empty manager.
	m.CleanupExpiredSessions(1 * time.Second)
	assert.Empty(t, m.sessions)
}

func TestSessionManager_CreateSessionsAreUnique(t *testing.T) {
	m := NewMcpSessionManagerImpl()
	id1, _ := m.CreateSession("http://b")
	// Guarantee a different nanosecond timestamp.
	time.Sleep(1 * time.Millisecond)
	id2, _ := m.CreateSession("http://b")
	assert.NotEqual(t, id1, id2, "session IDs should be unique")
}

// -----------------------------------------------------------------------------
// ensureHeader
// -----------------------------------------------------------------------------

func TestEnsureHeader_AddsWhenMissing(t *testing.T) {
	headers := [][2]string{{"X-Other", "v"}}
	ensureHeader(&headers, "X-New", "value")
	require.Len(t, headers, 2)
	assert.Equal(t, [2]string{"X-New", "value"}, headers[1])
}

func TestEnsureHeader_ReplacesCaseInsensitively(t *testing.T) {
	headers := [][2]string{{"content-type", "text/plain"}}
	ensureHeader(&headers, "Content-Type", "application/json")
	require.Len(t, headers, 1)
	// Replace path rewrites the original casing too.
	assert.Equal(t, "Content-Type", headers[0][0])
	assert.Equal(t, "application/json", headers[0][1])
}

func TestEnsureHeader_NoDuplicateOnRepeatedCalls(t *testing.T) {
	headers := [][2]string{}
	ensureHeader(&headers, "X-K", "1")
	ensureHeader(&headers, "X-K", "2")
	require.Len(t, headers, 1)
	assert.Equal(t, "2", headers[0][1])
}
