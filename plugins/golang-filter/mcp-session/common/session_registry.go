package common

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"
)

const sessionTimeout = 24 * time.Hour

type SessionRegistry interface {
	RegisterSession(serverName, sessionID string) error
	ValidateSession(serverName, sessionID string) (bool, error)
}

type sessionRegistryClient interface {
	Set(key string, value string, expiration time.Duration) error
	Get(key string) (string, error)
}

type RedisSessionRegistry struct {
	client sessionRegistryClient
}

func NewRedisSessionRegistry(redisClient *RedisClient) SessionRegistry {
	return &RedisSessionRegistry{client: redisClient}
}

func encodeKeyComponent(value string) string {
	return strconv.Itoa(len(value)) + ":" + value
}

func getSessionRegistryKey(serverName, sessionID string) string {
	return "mcp-server-session:v1:" + encodeKeyComponent(serverName) + ":" + encodeKeyComponent(sessionID)
}

func (r *RedisSessionRegistry) RegisterSession(serverName, sessionID string) error {
	if err := r.client.Set(getSessionRegistryKey(serverName, sessionID), serverName, sessionTimeout); err != nil {
		return fmt.Errorf("failed to register MCP session: %w", err)
	}
	return nil
}

func (r *RedisSessionRegistry) ValidateSession(serverName, sessionID string) (bool, error) {
	key := getSessionRegistryKey(serverName, sessionID)
	value, err := r.client.Get(key)
	if err != nil {
		if errors.Is(err, ErrKeyNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("failed to validate MCP session: %w", err)
	}
	if value != serverName {
		return false, nil
	}
	// Sliding validity: an actively used session is re-registered with the
	// full TTL so long-lived SSE connections do not expire mid-use, while
	// abandoned sessions still age out after the idle timeout.
	if err := r.client.Set(key, value, sessionTimeout); err != nil {
		return false, fmt.Errorf("failed to refresh MCP session: %w", err)
	}
	return true, nil
}

// SessionBackendRegistry remembers which Redis backend owns each live MCP SSE
// session.
//
// An MCP SSE session is carried by a Redis pub/sub channel: the SSE handler
// subscribes to `mcp-server-sse:<sessionId>` when the connection is opened and
// every message endpoint (POST) publication for that session has to land on the
// very same backend, otherwise the response never reaches the SSE client.
//
// The Redis endpoint lives in the `mcpServer.redis` configuration, which can be
// hot-updated at runtime. Such an update builds a new Redis client while SSE
// connections created under the previous generation are still alive. Without
// pinning, a POST that publishes through the *currently configured* client
// would write to the new backend while the SSE subscription keeps listening on
// the old one, silently splitting one logical session across two Redis
// instances:
//
//	existing SSE side  -> configuration at connection time -> Redis A
//	same-session POST  -> current configuration             -> Redis B
//
// The registry keeps every live session bound to the client that created its
// subscription, so the session stays coherent on one backend for its whole
// lifetime no matter how often the configuration is updated. New sessions
// naturally pick up the newly configured client. The pin is released together
// with the SSE connection.
type SessionBackendRegistry struct {
	mu       sync.RWMutex
	sessions map[string]*RedisClient
}

// GlobalSessionRegistry is the registry shared by the process. It is a package
// level value rather than a field of the filter configuration because the whole
// point of the registry is to survive configuration regeneration.
var GlobalSessionRegistry = NewSessionRegistry()

// NewSessionRegistry creates an empty registry.
func NewSessionRegistry() *SessionBackendRegistry {
	return &SessionBackendRegistry{
		sessions: make(map[string]*RedisClient),
	}
}

// Register pins sessionID to the Redis client that backs its SSE subscription.
// Empty session IDs and nil clients are ignored so callers do not have to guard
// the "Redis is not configured" case themselves.
func (r *SessionBackendRegistry) Register(sessionID string, client *RedisClient) {
	if r == nil || sessionID == "" || client == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[sessionID] = client
}

// Unregister drops the pin once the SSE connection has been torn down. Calling
// it for an unknown session is a no-op, which keeps the cleanup path safe when
// it runs more than once.
func (r *SessionBackendRegistry) Unregister(sessionID string) {
	if r == nil || sessionID == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sessions, sessionID)
}

// PublisherForSession returns the Redis client that must publish messages for
// sessionID. A live SSE session is pinned to the client that created its
// subscription; sessions without a live subscription fall back to the client
// currently present in the configuration. It returns nil when neither exists,
// which lets callers keep the "Redis is not enabled" path silent, and for an
// empty session ID because there is no channel to publish to in that case.
func (r *SessionBackendRegistry) PublisherForSession(sessionID string, fallback *RedisClient) *RedisClient {
	if r == nil || sessionID == "" {
		return nil
	}

	r.mu.RLock()
	client := r.sessions[sessionID]
	r.mu.RUnlock()
	if client != nil {
		return client
	}
	return fallback
}

// Len reports how many sessions are currently pinned. It is meant for tests and
// diagnostics.
func (r *SessionBackendRegistry) Len() int {
	if r == nil {
		return 0
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.sessions)
}
