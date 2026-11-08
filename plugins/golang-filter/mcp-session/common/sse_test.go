package common

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
