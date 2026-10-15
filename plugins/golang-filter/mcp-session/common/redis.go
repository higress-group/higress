package common

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/envoyproxy/envoy/contrib/golang/common/go/api"
	"github.com/go-redis/redis/v8"
)

type RedisConfig struct {
	address  string
	username string
	password string
	db       int
	secret   string // Encryption key
}

// ParseRedisConfig parses Redis configuration from a map
func ParseRedisConfig(config map[string]interface{}) (*RedisConfig, error) {
	c := &RedisConfig{}

	// address is required
	if addr, ok := config["address"].(string); ok && addr != "" {
		c.address = addr
	} else {
		return nil, fmt.Errorf("address is required and must be a non-empty string")
	}

	// username is optional
	if username, ok := config["username"].(string); ok {
		c.username = username
	}

	// password is optional
	if password, ok := config["password"].(string); ok {
		c.password = password
	}

	// db is optional, default to 0
	if db, ok := config["db"].(int); ok {
		c.db = db
	}

	// secret is optional
	if secret, ok := config["secret"].(string); ok {
		c.secret = secret
	}

	return c, nil
}

// RedisClient is a struct to handle Redis connections and operations
type RedisClient struct {
	clientMu sync.RWMutex
	client   *redis.Client
	ctx      context.Context
	cancel context.CancelFunc
	config *RedisConfig
	crypto *Crypto
}

// NewRedisClient creates a new RedisClient instance and establishes a connection to the Redis server
func NewRedisClient(config *RedisConfig) (*RedisClient, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     config.address,
		Username: config.username,
		Password: config.password,
		DB:       config.db,
	})

	// Ping the Redis server to check the connection
	pong, err := client.Ping(context.Background()).Result()
	if err != nil {
		api.LogErrorf("Failed to connect to Redis: %v", err)
	} else {
		api.LogDebugf("Connected to Redis: %s", pong)
	}

	ctx, cancel := context.WithCancel(context.Background())

	var crypto *Crypto
	if config.secret != "" {
		crypto, err = NewCrypto(config.secret)
		if err != nil {
			cancel()
			api.LogWarnf("Failed to initialize redis crypto: %v", err)
		}
	}

	redisClient := &RedisClient{
		client: client,
		ctx:    ctx,
		cancel: cancel,
		config: config,
		crypto: crypto,
	}

	// Start keep-alive check
	go redisClient.keepAlive()

	return redisClient, nil
}

// keepAlive periodically checks Redis connection and attempts to reconnect if needed
func (r *RedisClient) keepAlive() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
			if err := r.checkConnection(); err != nil {
				api.LogErrorf("Redis connection check failed: %v", err)
				if err := r.reconnect(); err != nil {
					api.LogErrorf("Failed to reconnect to Redis: %v", err)
				}
			}
		}
	}
}

// checkConnection verifies if the Redis connection is still alive
func (r *RedisClient) checkConnection() error {
	r.clientMu.RLock()
	client := r.client
	r.clientMu.RUnlock()
	_, err := client.Ping(r.ctx).Result()
	return err
}

// reconnect attempts to establish a new connection to Redis
func (r *RedisClient) reconnect() error {
	r.clientMu.Lock()
	defer r.clientMu.Unlock()

	// Close the old client
	if err := r.client.Close(); err != nil {
		api.LogErrorf("Error closing old Redis connection: %v", err)
	}

	// Create new client
	r.client = redis.NewClient(&redis.Options{
		Addr:     r.config.address,
		Username: r.config.username,
		Password: r.config.password,
		DB:       r.config.db,
	})

	// Test the new connection (inline to avoid recursive RLock)
	_, err := r.client.Ping(r.ctx).Result()
	if err != nil {
		return fmt.Errorf("failed to reconnect to Redis: %w", err)
	}

	api.LogDebugf("Successfully reconnected to Redis")
	return nil
}

// Publish publishes a message to a Redis channel
func (r *RedisClient) Publish(channel string, message string) error {
	r.clientMu.RLock()
	client := r.client
	r.clientMu.RUnlock()
	err := client.Publish(r.ctx, channel, message).Err()
	if err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}
	return nil
}

// Subscribe subscribes to a Redis channel and processes messages
func (r *RedisClient) Subscribe(channel string, stopChan chan struct{}, callback func(message string)) error {
	r.clientMu.RLock()
	client := r.client
	r.clientMu.RUnlock()
	pubsub := client.Subscribe(r.ctx, channel)
	_, err := pubsub.Receive(r.ctx)
	if err != nil {
		return fmt.Errorf("failed to subscribe to channel: %w", err)
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				api.LogErrorf("Redis Subscribe recovered from panic: %v", r)
			}
		}()

		defer func() {
			pubsub.Close()
			api.LogDebugf("Closed subscription to channel %s", channel)
		}()

		ch := pubsub.Channel()
		for {
			select {
			case <-stopChan:
				api.LogDebugf("Stopping subscription to channel %s", channel)
				return
			case msg, ok := <-ch:
				if !ok {
					api.LogDebugf("Redis subscription channel closed for %s", channel)
					return
				}

				func() {
					defer func() {
						if r := recover(); r != nil {
							api.LogErrorf("Recovered from panic in callback: %v", r)
						}
					}()
					callback(msg.Payload)
				}()
			}
		}
	}()

	return nil
}

// Set sets the value of a key in Redis
func (r *RedisClient) Set(key string, value string, expiration time.Duration) error {
	var finalValue string
	if r.crypto != nil {
		// Encrypt the data
		encryptedValue, err := r.crypto.Encrypt([]byte(value))
		if err != nil {
			return fmt.Errorf("failed to encrypt value: %w", err)
		}
		finalValue = encryptedValue
	} else {
		finalValue = value
	}

	r.clientMu.RLock()
	client := r.client
	r.clientMu.RUnlock()
	err := client.Set(r.ctx, key, finalValue, expiration).Err()
	if err != nil {
		return fmt.Errorf("failed to set key: %w", err)
	}
	return nil
}

// Get retrieves the value of a key from Redis
func (r *RedisClient) Get(key string) (string, error) {
	r.clientMu.RLock()
	client := r.client
	r.clientMu.RUnlock()
	value, err := client.Get(r.ctx, key).Result()
	if err == redis.Nil {
		return "", fmt.Errorf("key does not exist")
	} else if err != nil {
		return "", fmt.Errorf("failed to get key: %w", err)
	}

	if r.crypto != nil {
		// Decrypt the data
		decryptedValue, err := r.crypto.Decrypt(value)
		if err != nil {
			return "", fmt.Errorf("failed to decrypt value: %w", err)
		}
		return string(decryptedValue), nil
	}

	return value, nil
}

// Expire sets the expiration time for a key
func (r *RedisClient) Expire(key string, expiration time.Duration) error {
	r.clientMu.RLock()
	client := r.client
	r.clientMu.RUnlock()
	ok, err := client.Expire(r.ctx, key, expiration).Result()
	if err != nil {
		return fmt.Errorf("failed to set expiration for key: %w", err)
	}
	if !ok {
		return fmt.Errorf("key does not exist")
	}
	return nil
}

// Close closes the Redis client and stops the keepalive goroutine
func (r *RedisClient) Close() error {
	r.cancel()
	r.clientMu.Lock()
	defer r.clientMu.Unlock()
	return r.client.Close()
}

// Eval executes a Lua script
func (r *RedisClient) Eval(script string, numKeys int, keys []string, args []interface{}) (interface{}, error) {
	r.clientMu.RLock()
	client := r.client
	r.clientMu.RUnlock()
	result, err := client.Eval(r.ctx, script, keys, args...).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to execute Lua script: %w", err)
	}

	return result, nil
}
