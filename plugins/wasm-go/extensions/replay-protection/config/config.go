package config

import (
	"fmt"
	"strings"

	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
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

	servicePortConfig := redisConfig.Get("service_port")
	servicePort := servicePortConfig.Int()
	if servicePortConfig.Exists() && servicePort <= 0 {
		return fmt.Errorf("redis service port must be greater than 0")
	}
	if !servicePortConfig.Exists() {
		if strings.HasSuffix(serviceName, ".static") {
			servicePort = 80 // default logic port for static service
		} else {
			servicePort = 6379
		}
	}

	username := redisConfig.Get("username").String()
	password := redisConfig.Get("password").String()
	timeoutConfig := redisConfig.Get("timeout")
	timeout := timeoutConfig.Int()
	if timeoutConfig.Exists() && timeout <= 0 {
		return fmt.Errorf("redis timeout must be greater than 0")
	}
	if !timeoutConfig.Exists() {
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

	rejectCode := json.Get("reject_code")
	if rejectCode.Exists() {
		code := rejectCode.Int()
		if code < 100 || code > 599 {
			return fmt.Errorf("reject code must be between 100 and 599")
		}
		config.RejectCode = uint32(code)
	} else {
		config.RejectCode = 429
	}

	config.RejectMsg = json.Get("reject_msg").String()
	if config.RejectMsg == "" {
		config.RejectMsg = "Replay Attack Detected"
	}

	config.ForceNonce = json.Get("force_nonce").Bool()

	nonceTTL := json.Get("nonce_ttl")
	if nonceTTL.Exists() {
		config.NonceTTL = int(nonceTTL.Int())
		if config.NonceTTL <= 0 {
			return fmt.Errorf("nonce ttl must be greater than 0")
		}
	} else {
		config.NonceTTL = 900
	}

	nonceMinLength := json.Get("nonce_min_length")
	if nonceMinLength.Exists() {
		config.NonceMinLen = int(nonceMinLength.Int())
		if config.NonceMinLen <= 0 {
			return fmt.Errorf("nonce min length must be greater than 0")
		}
	} else {
		config.NonceMinLen = 8
	}

	nonceMaxLength := json.Get("nonce_max_length")
	if nonceMaxLength.Exists() {
		config.NonceMaxLen = int(nonceMaxLength.Int())
		if config.NonceMaxLen <= 0 {
			return fmt.Errorf("nonce max length must be greater than 0")
		}
	} else {
		config.NonceMaxLen = 128
	}
	if config.NonceMinLen > config.NonceMaxLen {
		return fmt.Errorf("nonce min length must not exceed nonce max length")
	}

	return nil
}
