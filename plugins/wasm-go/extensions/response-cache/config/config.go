package config

import (
	"fmt"
	"strconv"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/response-cache/cache"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/tidwall/gjson"
)

type PluginConfig struct {
	cacheProvider       cache.Provider
	cacheProviderConfig cache.ProviderConfig

	CacheKeyFromHeader string
	CacheKeyFromBody   string

	CacheValueFromBodyType string
	CacheValueFromBody     string

	CacheResponseCode []int32
}

func (c *PluginConfig) FromJson(json gjson.Result) {
	c.cacheProviderConfig.FromJson(json.Get("cache"))
	c.CacheKeyFromHeader = json.Get("cacheKeyFromHeader").String()
	c.CacheKeyFromBody = json.Get("cacheKeyFromBody").String()

	c.CacheValueFromBodyType = json.Get("cacheValueFromBodyType").String()
	if c.CacheValueFromBodyType == "" {
		c.CacheValueFromBodyType = "application/json"
	}

	c.CacheValueFromBody = json.Get("cacheValueFromBody").String()

	cacheResponseCode := json.Get("cacheResponseCode").Array()
	c.CacheResponseCode = make([]int32, 0, len(cacheResponseCode))
	for _, v := range cacheResponseCode {
		responseCode, err := strconv.Atoi(v.String())
		if err != nil || responseCode < 100 || responseCode > 999 {
			log.Errorf("Skip invalid response_code value: %s", v.String())
			continue
		}
		c.CacheResponseCode = append(c.CacheResponseCode, int32(responseCode))
	}

	if len(c.CacheResponseCode) == 0 {
		c.CacheResponseCode = []int32{200}
	}
}

func (c *PluginConfig) Validate() error {
	// cache cannot be empty
	if c.cacheProviderConfig.GetProviderType() == "" {
		return fmt.Errorf("cache provider cannot be empty")
	}

	// if cache provider is configured, validate it
	if c.cacheProviderConfig.GetProviderType() != "" {
		if err := c.cacheProviderConfig.Validate(); err != nil {
			return err
		}
	}

	// cache key cannot be all set
	if c.CacheKeyFromHeader != "" && c.CacheKeyFromBody != "" {
		return fmt.Errorf("cacheKeyFromHeader and cacheKeyFromBody cannot be all set")
	}
	return nil
}
func (c *PluginConfig) Complete() error {
	var err error
	if c.cacheProviderConfig.GetProviderType() != "" {
		log.Debugf("cache provider is set to %s", c.cacheProviderConfig.GetProviderType())
		c.cacheProvider, err = cache.CreateProvider(c.cacheProviderConfig)
		if err != nil {
			return err
		}
	} else {
		log.Info("cache provider is not configured")
		c.cacheProvider = nil
	}
	return nil
}

func (c *PluginConfig) GetCacheProvider() cache.Provider {
	return c.cacheProvider
}
