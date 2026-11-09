package common

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/envoyproxy/envoy/contrib/golang/common/go/api"
	"github.com/go-redis/redis/v8"
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

// fakeRedisBackend is a minimal RESP server used to observe which Redis backend
// receives a publication. It answers every command with ":0" and records the
// commands it received, so a test can tell which client a message was published
// with without running a real Redis server.
type fakeRedisBackend struct {
	listener net.Listener
	mu       sync.Mutex
	commands []string
}

func newFakeRedisBackend(t *testing.T) *fakeRedisBackend {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	backend := &fakeRedisBackend{listener: listener}
	go backend.serve()
	t.Cleanup(func() { _ = listener.Close() })
	return backend
}

func (b *fakeRedisBackend) Addr() string {
	return b.listener.Addr().String()
}

func (b *fakeRedisBackend) Commands() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.commands...)
}

func (b *fakeRedisBackend) record(args []string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	// Redis command names are case insensitive and go-redis writes them in
	// lower case, so normalize the command word. Without this the recorded
	// command depends on the client's casing instead of the protocol.
	if len(args) > 0 {
		args[0] = strings.ToUpper(args[0])
	}
	b.commands = append(b.commands, strings.Join(args, " "))
}

func (b *fakeRedisBackend) serve() {
	for {
		conn, err := b.listener.Accept()
		if err != nil {
			return
		}
		go b.handle(conn)
	}
}

func (b *fakeRedisBackend) handle(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	for {
		args, err := readRESPCommand(reader)
		if err != nil {
			return
		}
		b.record(args)
		if _, err := conn.Write([]byte(":0\r\n")); err != nil {
			return
		}
	}
}

// readRESPCommand reads one client command from the RESP stream. It is only
// meant to decode the arrays that go-redis writes for plain commands.
func readRESPCommand(reader *bufio.Reader) ([]string, error) {
	header, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	header = strings.TrimRight(header, "\r\n")
	if header == "" || header[0] != '*' {
		return nil, fmt.Errorf("unexpected RESP array header %q", header)
	}
	count, err := strconv.Atoi(header[1:])
	if err != nil {
		return nil, err
	}
	args := make([]string, 0, count)
	for i := 0; i < count; i++ {
		bulkHeader, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		bulkHeader = strings.TrimRight(bulkHeader, "\r\n")
		if bulkHeader == "" || bulkHeader[0] != '$' {
			return nil, fmt.Errorf("unexpected RESP bulk header %q", bulkHeader)
		}
		length, err := strconv.Atoi(bulkHeader[1:])
		if err != nil {
			return nil, err
		}
		buf := make([]byte, length+2)
		if _, err := io.ReadFull(reader, buf); err != nil {
			return nil, err
		}
		args = append(args, string(buf[:length]))
	}
	return args, nil
}

// newTestRedisClient builds a RedisClient without pinging the server, so tests
// can point it at a fake backend.
func newTestRedisClient(t *testing.T, addr string) *RedisClient {
	t.Helper()
	client := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = client.Close() })
	return &RedisClient{
		client: client,
		ctx:    context.Background(),
		cancel: func() {},
		config: &RedisConfig{address: addr},
	}
}

// TestSessionRegistryKeepsSessionOnTheBackendThatOwnsIt is the regression test
// for a live SSE session being split across two Redis backends after
// mcpServer.redis is hot-updated. The session is created while Redis A is
// configured, the configuration is then switched to Redis B, and the response
// for the same session must still be published on A because that is where the
// SSE subscription listens.
func TestSessionRegistryKeepsSessionOnTheBackendThatOwnsIt(t *testing.T) {
	registry := NewSessionRegistry()
	backendA := newFakeRedisBackend(t)
	backendB := newFakeRedisBackend(t)
	clientA := newTestRedisClient(t, backendA.Addr())
	clientB := newTestRedisClient(t, backendB.Addr())

	const sessionID = "session-1"
	registry.Register(sessionID, clientA)

	// clientB is what the configuration points at after the update.
	publisher := registry.PublisherForSession(sessionID, clientB)
	require.Same(t, clientA, publisher)
	require.NoError(t, publisher.Publish(GetSSEChannelName(sessionID), "event: message\ndata: ok\n\n"))

	commands := backendA.Commands()
	require.Len(t, commands, 1)
	require.True(t, strings.EqualFold(commands[0], "PUBLISH mcp-server-sse:session-1 event: message\ndata: ok\n\n"),
		"unexpected command published to the backend that owns the session: %q", commands[0])
	require.Empty(t, backendB.Commands())
}

func TestSessionRegistryFallsBackToConfiguredClientForUnknownSession(t *testing.T) {
	registry := NewSessionRegistry()
	backend := newFakeRedisBackend(t)
	client := newTestRedisClient(t, backend.Addr())

	publisher := registry.PublisherForSession("never-seen", client)
	require.Same(t, client, publisher)
	require.NoError(t, publisher.Publish(GetSSEChannelName("never-seen"), "hello"))

	commands := backend.Commands()
	require.Len(t, commands, 1)
	require.True(t, strings.EqualFold(commands[0], "PUBLISH mcp-server-sse:never-seen hello"),
		"unexpected command published to the fallback backend: %q", commands[0])
}

func TestSessionRegistryReleasesPinOnUnregister(t *testing.T) {
	registry := NewSessionRegistry()
	client := newTestRedisClient(t, "127.0.0.1:1")

	registry.Register("session-1", client)
	require.Equal(t, 1, registry.Len())
	require.Same(t, client, registry.PublisherForSession("session-1", nil))

	registry.Unregister("session-1")
	require.Zero(t, registry.Len())
	require.Nil(t, registry.PublisherForSession("session-1", nil))

	// Releasing the same session twice must stay a no-op.
	registry.Unregister("session-1")
	require.Zero(t, registry.Len())
}

func TestSessionRegistryIgnoresIncompleteRegistrations(t *testing.T) {
	registry := NewSessionRegistry()
	client := newTestRedisClient(t, "127.0.0.1:1")

	registry.Register("", client)
	registry.Register("session-1", nil)
	require.Zero(t, registry.Len())

	// An empty session id has no channel to publish to, so it never resolves.
	require.Nil(t, registry.PublisherForSession("", client))
	require.Same(t, client, registry.PublisherForSession("session-1", client))
}

func TestSessionRegistryToleratesNilReceiver(t *testing.T) {
	var registry *SessionBackendRegistry
	client := newTestRedisClient(t, "127.0.0.1:1")

	registry.Register("session-1", client)
	registry.Unregister("session-1")
	require.Zero(t, registry.Len())
	require.Nil(t, registry.PublisherForSession("session-1", client))
}

func TestSessionRegistryIsSafeForConcurrentUse(t *testing.T) {
	registry := NewSessionRegistry()
	clientA := newTestRedisClient(t, "127.0.0.1:1")
	clientB := newTestRedisClient(t, "127.0.0.1:2")

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			sessionID := fmt.Sprintf("session-%d", index)
			for j := 0; j < 100; j++ {
				registry.Register(sessionID, clientA)
				if got := registry.PublisherForSession(sessionID, clientB); got != clientA {
					t.Errorf("session %s resolved to %p, want %p", sessionID, got, clientA)
				}
				registry.Unregister(sessionID)
			}
		}(i)
	}
	wg.Wait()
	require.Zero(t, registry.Len())
}
