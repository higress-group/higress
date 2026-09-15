package cache

import (
	"errors"

	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

type redisProviderInitializer struct {
}

func (r *redisProviderInitializer) ValidateConfig(cf ProviderConfig) error {
	if len(cf.serviceName) == 0 {
		return errors.New("cache service name is required")
	}
	return nil
}

func (r *redisProviderInitializer) CreateProvider(cf ProviderConfig) (Provider, error) {
	rp := redisProvider{
		config: cf,
		client: wrapper.NewRedisClusterClient(wrapper.FQDNCluster{
			FQDN: cf.serviceName,
			Host: cf.serviceHost,
			Port: int64(cf.servicePort)}),
	}
	err := rp.Init(cf.username, cf.password, uint32(cf.timeout))
	return &rp, err
}

type redisProvider struct {
	config ProviderConfig
	client wrapper.RedisClient
}

func (rp *redisProvider) GetProviderType() string {
	return PROVIDER_TYPE_REDIS
}

func (rp *redisProvider) Init(username string, password string, timeout uint32) error {
	err := rp.client.Init(rp.config.username, rp.config.password, int64(rp.config.timeout))
	if rp.client.Ready() {
		log.Info("redis init successfully")
	} else {
		log.Error("redis init failed, will try later")
	}
	return err
}

func (rp *redisProvider) Get(key string, cb wrapper.RedisResponseCallback) error {
	return rp.client.Get(key, cb)
}

func (rp *redisProvider) Set(key string, value string, cb wrapper.RedisResponseCallback) error {
	if rp.config.cacheTTL == 0 {
		return rp.client.Set(key, value, cb)
	} else {
		return rp.client.SetEx(key, value, rp.config.cacheTTL, cb)
	}
}

func (rp *redisProvider) GetCacheKeyPrefix() string {
	return rp.config.cacheKeyPrefix
}
