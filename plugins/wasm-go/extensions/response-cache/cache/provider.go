package cache

import (
	"errors"
	"math"
	"strings"

	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

const (
	PROVIDER_TYPE_REDIS  = "redis"
	DEFAULT_CACHE_PREFIX = "higress-resp-cache:"
)

type providerInitializer interface {
	ValidateConfig(ProviderConfig) error
	CreateProvider(ProviderConfig) (Provider, error)
}

var (
	providerInitializers = map[string]providerInitializer{
		PROVIDER_TYPE_REDIS: &redisProviderInitializer{},
	}
)

type ProviderConfig struct {
	// @Title zh-CN redis 缓存服务提供者类型
	// @Description zh-CN 缓存服务提供者类型，例如 redis
	typ string
	// @Title zh-CN redis 缓存服务名称
	// @Description zh-CN 缓存服务名称
	serviceName string
	// @Title zh-CN redis 缓存服务端口
	// @Description zh-CN 缓存服务端口，默认值为6379
	servicePort int
	// @Title zh-CN redis 缓存服务地址
	// @Description zh-CN Cache 缓存服务地址，非必填
	serviceHost string
	// @Title zh-CN 缓存服务用户名
	// @Description zh-CN 缓存服务用户名，非必填
	username string
	// @Title zh-CN 缓存服务密码
	// @Description zh-CN 缓存服务密码，非必填
	password string
	// @Title zh-CN 请求超时
	// @Description zh-CN 请求缓存服务的超时时间，单位为毫秒。默认值是10000，即10秒
	timeout int64
	// @Title zh-CN 缓存过期时间
	// @Description zh-CN 缓存过期时间，单位为秒。默认值是0，即永不过期
	cacheTTL int
	// @Title 缓存 Key 前缀
	// @Description 缓存 Key 的前缀，默认值为 "higress-resp-cache:"
	cacheKeyPrefix string
}

func (c *ProviderConfig) GetProviderType() string {
	return c.typ
}

func (c *ProviderConfig) FromJson(json gjson.Result) {
	c.typ = json.Get("type").String()
	c.serviceName = json.Get("serviceName").String()
	c.servicePort = int(json.Get("servicePort").Int())
	if !json.Get("servicePort").Exists() {
		if strings.HasSuffix(c.serviceName, ".static") {
			// use default logic port which is 80 for static service
			c.servicePort = 80
		} else {
			c.servicePort = 6379
		}
	}
	c.serviceHost = json.Get("serviceHost").String()
	c.username = json.Get("username").String()
	c.password = json.Get("password").String()
	c.timeout = json.Get("timeout").Int()
	if !json.Get("timeout").Exists() {
		c.timeout = 10000
	}
	c.cacheTTL = int(json.Get("cacheTTL").Int())
	if !json.Get("cacheTTL").Exists() {
		c.cacheTTL = 0
		// c.cacheTTL = 3600000
	}
	if json.Get("cacheKeyPrefix").Exists() {
		c.cacheKeyPrefix = json.Get("cacheKeyPrefix").String()
	} else {
		c.cacheKeyPrefix = DEFAULT_CACHE_PREFIX
	}

}

func (c *ProviderConfig) Validate() error {
	if c.typ == "" {
		return errors.New("cache service type is required")
	}
	if c.serviceName == "" {
		return errors.New("cache service name is required")
	}
	if c.servicePort <= 0 {
		return errors.New("cache service port must be greater than 0")
	}
	if c.timeout <= 0 || c.timeout > math.MaxUint32 {
		return errors.New("cache service timeout must be between 1 and 4294967295 milliseconds")
	}
	if c.cacheTTL < 0 {
		return errors.New("cache TTL must be greater than or equal to 0")
	}
	initializer, has := providerInitializers[c.typ]
	if !has {
		return errors.New("unknown cache service provider type: " + c.typ)
	}
	if err := initializer.ValidateConfig(*c); err != nil {
		return err
	}
	return nil
}

func CreateProvider(pc ProviderConfig) (Provider, error) {
	initializer, has := providerInitializers[pc.typ]
	if !has {
		return nil, errors.New("unknown provider type: " + pc.typ)
	}
	return initializer.CreateProvider(pc)
}

type Provider interface {
	GetProviderType() string
	Init(username string, password string, timeout uint32) error
	Get(key string, cb wrapper.RedisResponseCallback) error
	Set(key string, value string, cb wrapper.RedisResponseCallback) error
	GetCacheKeyPrefix() string
}
