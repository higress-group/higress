package common

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/envoyproxy/envoy/contrib/golang/common/go/api"
	"github.com/stretchr/testify/require"
)

type mockCommonCAPI struct{}

func (m *mockCommonCAPI) Log(api.LogType, string) {}

func (m *mockCommonCAPI) LogLevel() api.LogType {
	return api.Debug
}

type recordingSessionRegistry struct {
	serverName string
	sessionID  string
}

func (r *recordingSessionRegistry) RegisterSession(serverName, sessionID string) error {
	r.serverName = serverName
	r.sessionID = sessionID
	return nil
}

func (r *recordingSessionRegistry) ValidateSession(string, string) (bool, error) {
	return true, nil
}

func TestSSEServerBindsSessionsToConfiguredServerName(t *testing.T) {
	registry := &recordingSessionRegistry{}
	server := NewSSEServer(
		NewMCPServer("shared", "1.0.0"),
		WithServerName("server-A"),
		WithSessionRegistry(registry),
	)

	require.Equal(t, "server-A", server.GetServerName())
	require.NoError(t, server.registerSession("session-1"))
	require.Equal(t, "server-A", registry.serverName)
	require.Equal(t, "session-1", registry.sessionID)
}

func TestHandleMessageSetsContentTypeBeforeWritingResponse(t *testing.T) {
	api.SetCommonCAPI(&mockCommonCAPI{})

	server := NewSSEServer(NewMCPServer("test", "1.0.0"))
	request := httptest.NewRequest(http.MethodPost, "/message", strings.NewReader(`not-json-at-all`))
	recorder := httptest.NewRecorder()

	status := server.HandleMessage(recorder, request, []byte(`not-json-at-all`))

	require.Equal(t, http.StatusOK, status)
	require.Equal(t, "application/json", recorder.Result().Header.Get("Content-Type"))
	require.Contains(t, recorder.Body.String(), "Failed to parse message")
}

// TestRegisterSessionForRedisBackendPinsUntilStopChanClosed verifies that the
// Redis client which owns an SSE connection stays pinned for the whole session
// and is released only when the connection is torn down.
func TestRegisterSessionForRedisBackendPinsUntilStopChanClosed(t *testing.T) {
	registry := NewSessionRegistry()
	client := newTestRedisClient(t, "127.0.0.1:1")
	server := NewSSEServer(NewMCPServer("test", "1.0.0"),
		WithRedisClient(client),
		WithSessionRegistry(registry))

	stopChan := make(chan struct{})
	server.registerSessionForRedisBackend("session-1", stopChan)
	require.Equal(t, 1, registry.Len())
	require.Same(t, client, registry.PublisherForSession("session-1", nil))

	// filter.OnDestroy closes the channel exactly once; the pin must go away with
	// the connection and stay released afterwards.
	close(stopChan)
	require.Eventually(t, func() bool {
		return registry.PublisherForSession("session-1", nil) == nil
	}, time.Second, 10*time.Millisecond)
	require.Zero(t, registry.Len())
}

// TestRegisterSessionForRedisBackendSkipsServerWithoutRedis makes sure the SSE
// server used by the mcp-server plugin, which runs without Redis, does not pin
// anything.
func TestRegisterSessionForRedisBackendSkipsServerWithoutRedis(t *testing.T) {
	registry := NewSessionRegistry()
	server := NewSSEServer(NewMCPServer("test", "1.0.0"), WithSessionRegistry(registry))

	server.registerSessionForRedisBackend("session-1", make(chan struct{}))
	require.Zero(t, registry.Len())
}

// TestRegisterSessionForRedisBackendKeepsPinWithoutStopChan documents the nil
// stop channel behavior: without a teardown signal the pin is never released.
func TestRegisterSessionForRedisBackendKeepsPinWithoutStopChan(t *testing.T) {
	registry := NewSessionRegistry()
	client := newTestRedisClient(t, "127.0.0.1:1")
	server := NewSSEServer(NewMCPServer("test", "1.0.0"),
		WithRedisClient(client),
		WithSessionRegistry(registry))

	server.registerSessionForRedisBackend("session-1", nil)
	require.Same(t, client, registry.PublisherForSession("session-1", nil))
}
