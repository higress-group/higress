package common

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
)

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
	var registry *SessionRegistry
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
