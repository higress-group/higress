package common

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/envoyproxy/envoy/contrib/golang/common/go/api"
	"github.com/stretchr/testify/require"
)

type registryTestClient struct {
	values      map[string]string
	expirations map[string]time.Duration
	expiresAt   map[string]time.Time
	setCalls    int
	now         func() time.Time
	setErr      error
	getErr      error
}

func (c *registryTestClient) currentTime() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

func (c *registryTestClient) Set(key string, value string, expiration time.Duration) error {
	if c.setErr != nil {
		return c.setErr
	}
	if c.values == nil {
		c.values = make(map[string]string)
	}
	if c.expirations == nil {
		c.expirations = make(map[string]time.Duration)
	}
	if c.expiresAt == nil {
		c.expiresAt = make(map[string]time.Time)
	}
	c.values[key] = value
	c.expirations[key] = expiration
	c.expiresAt[key] = c.currentTime().Add(expiration)
	c.setCalls++
	return nil
}

func (c *registryTestClient) Get(key string) (string, error) {
	if c.getErr != nil {
		return "", c.getErr
	}
	value, ok := c.values[key]
	if !ok {
		return "", ErrKeyNotExist
	}
	if expiry, ok := c.expiresAt[key]; ok && !c.currentTime().Before(expiry) {
		delete(c.values, key)
		delete(c.expirations, key)
		delete(c.expiresAt, key)
		return "", ErrKeyNotExist
	}
	return value, nil
}

func TestRedisSessionRegistryValidatesServerBinding(t *testing.T) {
	client := &registryTestClient{
		values:      make(map[string]string),
		expirations: make(map[string]time.Duration),
	}
	registry := &RedisSessionRegistry{client: client}

	require.NoError(t, registry.RegisterSession("server-A", "session-1"))
	require.Equal(t, sessionTimeout, client.expirations[getSessionRegistryKey("server-A", "session-1")])

	valid, err := registry.ValidateSession("server-A", "session-1")
	require.NoError(t, err)
	require.True(t, valid)

	valid, err = registry.ValidateSession("server-B", "session-1")
	require.NoError(t, err)
	require.False(t, valid)

	valid, err = registry.ValidateSession("server-A", "unknown")
	require.NoError(t, err)
	require.False(t, valid)
}

func TestRedisSessionRegistryReturnsStoreErrors(t *testing.T) {
	storeErr := errors.New("redis unavailable")
	registry := &RedisSessionRegistry{client: &registryTestClient{getErr: storeErr}}

	valid, err := registry.ValidateSession("server-A", "session-1")
	require.False(t, valid)
	require.ErrorIs(t, err, storeErr)

	registry = &RedisSessionRegistry{client: &registryTestClient{setErr: storeErr}}
	require.ErrorIs(t, registry.RegisterSession("server-A", "session-1"), storeErr)
}

func TestRedisSessionRegistryRefreshesOnSuccessfulValidation(t *testing.T) {
	now := time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)
	client := &registryTestClient{now: func() time.Time { return now }}
	registry := &RedisSessionRegistry{client: client}
	key := getSessionRegistryKey("server-A", "session-1")

	require.NoError(t, registry.RegisterSession("server-A", "session-1"))
	require.Equal(t, 1, client.setCalls)
	require.Equal(t, sessionTimeout, client.expirations[key])
	firstExpiry := client.expiresAt[key]

	// A validation 23 hours after registration must re-SET the full TTL.
	now = now.Add(23 * time.Hour)
	valid, err := registry.ValidateSession("server-A", "session-1")
	require.NoError(t, err)
	require.True(t, valid)
	require.Equal(t, 2, client.setCalls)
	require.Equal(t, sessionTimeout, client.expirations[key])
	require.Equal(t, now.Add(sessionTimeout), client.expiresAt[key])
	require.True(t, client.expiresAt[key].After(firstExpiry))

	// 25 hours after registration the original TTL would have expired, but the
	// refreshed session is still valid.
	now = now.Add(2 * time.Hour)
	valid, err = registry.ValidateSession("server-A", "session-1")
	require.NoError(t, err)
	require.True(t, valid)
	require.Equal(t, 3, client.setCalls)
}

func TestRedisSessionRegistryDoesNotRefreshRejectedSession(t *testing.T) {
	client := &registryTestClient{}
	registry := &RedisSessionRegistry{client: client}

	require.NoError(t, registry.RegisterSession("server-A", "session-1"))
	require.Equal(t, 1, client.setCalls)

	valid, err := registry.ValidateSession("server-B", "session-1")
	require.NoError(t, err)
	require.False(t, valid)
	require.Equal(t, 1, client.setCalls)
}

func TestRedisSessionRegistryFailsWhenRefreshFails(t *testing.T) {
	storeErr := errors.New("redis unavailable")
	key := getSessionRegistryKey("server-A", "session-1")
	client := &registryTestClient{
		values: map[string]string{key: "server-A"},
		setErr: storeErr,
	}
	registry := &RedisSessionRegistry{client: client}

	valid, err := registry.ValidateSession("server-A", "session-1")
	require.False(t, valid)
	require.ErrorIs(t, err, storeErr)
}

func TestSessionRegistryKeyUsesComponentSafeEncoding(t *testing.T) {
	left := getSessionRegistryKey("a", "b:c")
	right := getSessionRegistryKey("a:b", "c")
	require.NotEqual(t, left, right)
	require.Equal(t, "mcp-server-session:v1:1:a:3:b:c", left)
}

type failingSessionRegistry struct {
	serverName string
	sessionID  string
}

func (r *failingSessionRegistry) RegisterSession(serverName, sessionID string) error {
	r.serverName = serverName
	r.sessionID = sessionID
	return errors.New("registry unavailable")
}

func (r *failingSessionRegistry) ValidateSession(string, string) (bool, error) {
	return false, nil
}

type registrySSECallbacks struct {
	api.FilterCallbackHandler
	encoder *registrySSEEncoderCallbacks
}

func (c *registrySSECallbacks) EncoderFilterCallbacks() api.EncoderFilterCallbacks {
	return c.encoder
}

type registrySSEEncoderCallbacks struct {
	api.EncoderFilterCallbacks
	statusCode int
}

func (c *registrySSEEncoderCallbacks) SendLocalReply(statusCode int, _ string, _ map[string][]string, _ int64, _ string) {
	c.statusCode = statusCode
}

func TestHandleSSERegistersSessionBeforeServing(t *testing.T) {
	api.SetCommonCAPI(&mockCommonCAPI{})
	registry := &failingSessionRegistry{}
	encoder := &registrySSEEncoderCallbacks{}
	callbacks := &registrySSECallbacks{encoder: encoder}
	server := NewSSEServer(NewMCPServer("server-A", "1.0.0"), WithSessionRegistry(registry))

	server.HandleSSE(callbacks, make(chan struct{}))

	require.Equal(t, http.StatusServiceUnavailable, encoder.statusCode)
	require.Equal(t, "server-A", registry.serverName)
	require.NotEmpty(t, registry.sessionID)
}
