package common

import (
	"errors"
	"fmt"
	"strconv"
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
