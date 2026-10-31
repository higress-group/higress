package config

import (
	"fmt"
	"strings"

	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

const (
	FailureModeFailOpen   = "fail_open"
	FailureModeFailClosed = "fail_closed"
)

type ReplayProtectionConfig struct {
	ForceNonce     bool // Whether to enforce nonce verification
	NonceTTL       int  // Expiration time of the nonce (in seconds)
	Redis          RedisConfig
	NonceMinLen    int    // Minimum length of the nonce
	NonceMaxLen    int    // Maximum length of the nonce
	NonceHeader    string // Name of the nonce header
	ValidateBase64 bool   // Whether to validate base64 encoding format
	RejectCode     uint32 // Response code
	RejectMsg      string // Response body
	FailureMode    string
}

type RedisConfig struct {
	Client    wrapper.RedisClient
	KeyPrefix string
}

func ParseConfig(json gjson.Result, config *ReplayProtectionConfig) error {
	// Parse Redis configuration
	redisConfig := json.Get("redis")
	if !redisConfig.Exists() {
		return fmt.Errorf("missing redis config")
	}

	serviceName := redisConfig.Get("service_name").String()
	if serviceName == "" {
		return fmt.Errorf("redis service name is required")
	}

	servicePort := redisConfig.Get("service_port").Int()
	if servicePort == 0 {
		if strings.HasSuffix(serviceName, ".static") {
			servicePort = 80 // default logic port for static service
		} else {
			servicePort = 6379
		}
	}

	username := redisConfig.Get("username").String()
	password := redisConfig.Get("password").String()
	timeout := redisConfig.Get("timeout").Int()
	if timeout == 0 {
		timeout = 1000
	}

	// Initialize Redis client
	config.Redis.Client = wrapper.NewRedisClusterClient(wrapper.FQDNCluster{
		FQDN: serviceName,
		Port: servicePort,
	})
	database := int(redisConfig.Get("database").Int())
	if err := config.Redis.Client.Init(username, password, timeout, wrapper.WithDataBase(database)); err != nil {
		return err
	}

	keyPrefix := redisConfig.Get("key_prefix").String()
	if keyPrefix == "" {
		keyPrefix = "replay-protection"
	}
	config.Redis.KeyPrefix = keyPrefix

	config.NonceHeader = json.Get("nonce_header").String()
	if config.NonceHeader == "" {
		config.NonceHeader = "X-Higress-Nonce"
	}

	config.ValidateBase64 = json.Get("validate_base64").Bool()

	config.RejectCode = uint32(json.Get("reject_code").Int())
	if config.RejectCode == 0 {
		config.RejectCode = 429
	}

	config.RejectMsg = json.Get("reject_msg").String()
	if config.RejectMsg == "" {
		config.RejectMsg = "Replay Attack Detected"
	}

	config.ForceNonce = json.Get("force_nonce").Bool()

	config.NonceTTL = int(json.Get("nonce_ttl").Int())
	if config.NonceTTL == 0 {
		config.NonceTTL = 900
	}

	config.NonceMinLen = int(json.Get("nonce_min_length").Int())
	if config.NonceMinLen == 0 {
		config.NonceMinLen = 8
	}

	config.NonceMaxLen = int(json.Get("nonce_max_length").Int())
	if config.NonceMaxLen == 0 {
		config.NonceMaxLen = 128
	}

	config.FailureMode = FailureModeFailClosed
	if failureMode := json.Get("failure_mode"); failureMode.Exists() {
		config.FailureMode = failureMode.String()
	}
	if config.FailureMode != FailureModeFailOpen && config.FailureMode != FailureModeFailClosed {
		return fmt.Errorf("invalid failure_mode %q: must be %q or %q", config.FailureMode, FailureModeFailOpen, FailureModeFailClosed)
	}

	return nil
}
