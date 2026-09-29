package mcp_server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/alibaba/higress/plugins/golang-filter/mcp-session/common"
	"github.com/envoyproxy/envoy/contrib/golang/common/go/api"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

// writeToolCall is a tools/call body for the stub server's write tool. It stands
// in for the RAG server's create-chunks-from-text call, which is the
// unauthenticated write path this gate protects.
const writeToolCall = `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"insert-doc","arguments":{}}}`

// stubWriteServer is a minimal server that owns a write tool and exposes HTTP
// Basic credentials through common.BasicAuthProvider, the same contract the rag
// and higress-ops servers implement.
type stubWriteServer struct {
	username   string
	password   string
	writeCalls *int
}

func (s *stubWriteServer) Clone() common.Server {
	clone := *s
	return &clone
}

func (s *stubWriteServer) GetBasicAuthCredentials() (string, string) {
	return s.username, s.password
}

func (s *stubWriteServer) ParseConfig(cfg map[string]any) error {
	if username, ok := cfg["username"].(string); ok {
		s.username = username
	}
	if password, ok := cfg["password"].(string); ok {
		s.password = password
	}
	return nil
}

func (s *stubWriteServer) NewServer(serverName string) (*common.MCPServer, error) {
	mcpServer := common.NewMCPServer(serverName, "1.0.0")
	mcpServer.AddTool(
		mcp.NewToolWithRawSchema("insert-doc", "write to the backing store", json.RawMessage(`{"type":"object","properties":{}}`)),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			*s.writeCalls++
			return &mcp.CallToolResult{
				Content: []mcp.Content{mcp.TextContent{Type: "text", Text: "inserted"}},
			}, nil
		},
	)
	return mcpServer, nil
}

func basicAuthHeader(user, pass string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
}

// registerStubWriteServer registers the stub under a unique type name and
// returns the shared write-call counter.
func registerStubWriteServer(t *testing.T, serverType, username, password string) *int {
	t.Helper()

	writeCalls := new(int)
	common.GlobalRegistry.RegisterServer(serverType, &stubWriteServer{
		username:   username,
		password:   password,
		writeCalls: writeCalls,
	})
	t.Cleanup(func() { common.GlobalRegistry.UnregisterServer(serverType) })
	return writeCalls
}

func parseStubServerConfig(t *testing.T, serverType, serverPath, username, password string) *config {
	t.Helper()

	serverConfig := map[string]any{
		"name": serverType + "-server",
		"type": serverType,
		"path": serverPath,
	}
	if username != "" || password != "" {
		serverConfig["config"] = map[string]any{"username": username, "password": password}
	}

	parsed, err := (&Parser{}).Parse(typedStructAny(t, map[string]any{
		"servers": []any{serverConfig},
	}), nil)
	require.NoError(t, err)

	conf := parsed.(*config)
	t.Cleanup(conf.Destroy)
	require.Len(t, conf.servers, 1, "server should have loaded")
	return conf
}

func newTestFilter(conf *config) (*filter, *testDecoderCallbacks) {
	decoder := &testDecoderCallbacks{}
	return &filter{
		callbacks: &testCallbacks{decoder: decoder},
		config:    conf,
	}, decoder
}

func postMessageHeaders(path, authorization string) testRequestHeaderMap {
	values := map[string][]string{
		":method":    {http.MethodPost},
		":scheme":    {"http"},
		":authority": {"example.com"},
		":path":      {path},
	}
	if authorization != "" {
		values["authorization"] = []string{authorization}
	}
	return testRequestHeaderMap{values: values}
}

// TestParserWiresBasicAuthCredentialsIntoServerWrapper covers the composition
// point at config.go: a server that implements common.BasicAuthProvider must
// have its credentials copied onto the wrapper, because that is the only signal
// the filter uses to decide whether to enforce auth.
func TestParserWiresBasicAuthCredentialsIntoServerWrapper(t *testing.T) {
	api.SetCommonCAPI(&mockCommonCAPI{})
	registerStubWriteServer(t, "test-auth-wiring", "admin", "s3cret")

	conf := parseStubServerConfig(t, "test-auth-wiring", "/mcp-servers/auth-wiring", "admin", "s3cret")

	require.Equal(t, "admin", conf.servers[0].AuthUsername)
	require.Equal(t, "s3cret", conf.servers[0].AuthPassword)
}

// TestDecodeHeadersRejectsUnauthenticatedWrite covers the 401 path: a request
// without valid credentials must be refused before the body is buffered or
// dispatched, so the write tool never runs.
func TestDecodeHeadersRejectsUnauthenticatedWrite(t *testing.T) {
	api.SetCommonCAPI(&mockCommonCAPI{})
	writeCalls := registerStubWriteServer(t, "test-auth-reject", "admin", "s3cret")

	conf := parseStubServerConfig(t, "test-auth-reject", "/mcp-servers/auth-reject", "admin", "s3cret")

	tests := []struct {
		name          string
		authorization string
	}{
		{"missing header", ""},
		{"wrong password", basicAuthHeader("admin", "nope")},
		{"wrong username", basicAuthHeader("root", "s3cret")},
		{"bearer token is not accepted", "Bearer s3cret"},
		{"malformed base64", "Basic !!!notbase64!!!"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			*writeCalls = 0
			f, decoder := newTestFilter(conf)

			status := f.DecodeHeaders(postMessageHeaders("/mcp-servers/auth-reject", tt.authorization), false)

			require.Equal(t, api.LocalReply, status, "request must be stopped at the filter")
			require.Equal(t, http.StatusUnauthorized, decoder.statusCode)
			require.Equal(t, "Unauthorized", decoder.body)
			require.Contains(t, decoder.headers, "WWW-Authenticate")
			require.Zero(t, *writeCalls, "write tool must not run for an unauthenticated request")
		})
	}
}

// TestDecodeDataRunsHandlerForAuthenticatedWrite covers the pass-through path:
// with valid credentials the request reaches the MCP dispatch and the write tool
// executes.
func TestDecodeDataRunsHandlerForAuthenticatedWrite(t *testing.T) {
	api.SetCommonCAPI(&mockCommonCAPI{})
	writeCalls := registerStubWriteServer(t, "test-auth-accept", "admin", "s3cret")

	conf := parseStubServerConfig(t, "test-auth-accept", "/mcp-servers/auth-accept", "admin", "s3cret")

	f, decoder := newTestFilter(conf)

	headerStatus := f.DecodeHeaders(
		postMessageHeaders("/mcp-servers/auth-accept", basicAuthHeader("admin", "s3cret")),
		false,
	)
	require.Equal(t, api.StopAndBuffer, headerStatus, "authenticated request should be buffered, not refused")
	require.Zero(t, decoder.statusCode, "no local reply should have been sent")

	dataStatus := f.DecodeData(testBuffer{data: []byte(writeToolCall)}, true)

	require.Equal(t, api.LocalReply, dataStatus)
	require.Equal(t, http.StatusOK, decoder.statusCode)
	require.Equal(t, 1, *writeCalls, "write tool should have run exactly once")
	require.Contains(t, decoder.body, "inserted")
}

// TestRAGServerFailsClosedWithoutCredentials asserts the RAG-specific half of
// the gate: the registered rag server must opt into BasicAuthProvider and must
// refuse a config with no credentials, so it is never loaded unauthenticated.
func TestRAGServerFailsClosedWithoutCredentials(t *testing.T) {
	api.SetCommonCAPI(&mockCommonCAPI{})

	tests := []struct {
		name string
		cfg  map[string]any
	}{
		{"no config", nil},
		{"empty config", map[string]any{}},
		{"missing password", map[string]any{"username": "admin"}},
		{"empty username", map[string]any{"username": "", "password": "s3cret"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := common.GlobalRegistry.NewServerConfig("rag")
			require.NotNil(t, server, "rag server should be registered")

			provider, ok := server.(common.BasicAuthProvider)
			require.True(t, ok, "rag must implement common.BasicAuthProvider to be gated by the filter")

			err := server.ParseConfig(tt.cfg)
			require.Error(t, err, "rag must fail closed without basic auth credentials")

			username, password := provider.GetBasicAuthCredentials()
			require.Empty(t, username, "a rejected rag config must not publish a username")
			require.Empty(t, password, "a rejected rag config must not publish a password")
		})
	}
}

// TestRAGServerPublishesCredentialsForAuthGate asserts that a fully configured
// rag server hands its credentials to the filter gate.
func TestRAGServerPublishesCredentialsForAuthGate(t *testing.T) {
	api.SetCommonCAPI(&mockCommonCAPI{})

	server := common.GlobalRegistry.NewServerConfig("rag")
	require.NotNil(t, server)

	err := server.ParseConfig(map[string]any{
		"username": "admin",
		"password": "s3cret",
	})
	require.NoError(t, err)

	provider, ok := server.(common.BasicAuthProvider)
	require.True(t, ok)

	username, password := provider.GetBasicAuthCredentials()
	require.Equal(t, "admin", username)
	require.Equal(t, "s3cret", password)

	require.True(t, common.CheckBasicAuth(basicAuthHeader("admin", "s3cret"), username, password))
	require.False(t, common.CheckBasicAuth(basicAuthHeader("admin", "nope"), username, password))
}

// TestRAGServerClonePreservesCredentials guards against a silent fail-open: the
// rag Clone implementation round-trips only its inner config through JSON, so
// the credentials must be carried across explicitly. A clone reporting an empty
// username would make the filter treat auth as disabled.
func TestRAGServerClonePreservesCredentials(t *testing.T) {
	api.SetCommonCAPI(&mockCommonCAPI{})

	server := common.GlobalRegistry.NewServerConfig("rag")
	require.NotNil(t, server)
	require.NoError(t, server.ParseConfig(map[string]any{
		"username": "admin",
		"password": "s3cret",
	}))

	cloner, ok := server.(common.ServerCloner)
	require.True(t, ok, "rag should implement common.ServerCloner")

	provider, ok := cloner.Clone().(common.BasicAuthProvider)
	require.True(t, ok, "cloned rag config must still implement common.BasicAuthProvider")

	username, password := provider.GetBasicAuthCredentials()
	require.Equal(t, "admin", username)
	require.Equal(t, "s3cret", password)
}
