package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tidwall/gjson"
)

func TestParseCacheConfigInactive(t *testing.T) {
	tests := []struct {
		name string
		json string
	}{
		{name: "Absent", json: `{}`},
		{name: "Explicitly Disabled", json: `{"cache":{"enabled":false,"ttl":60,"redis":{"service_name":"redis.static"}}}`},
		{name: "Enabled Field Missing", json: `{"cache":{"ttl":60,"redis":{"service_name":"redis.static"}}}`},
		// A non-positive ttl does not satisfy the cache gate: degrade silently to the
		// cache-free flow instead of failing the config.
		{name: "Enabled Without TTL", json: `{"cache":{"enabled":true,"redis":{"service_name":"redis.static"}}}`},
		{name: "Enabled With Non Positive TTL", json: `{"cache":{"enabled":true,"ttl":-1,"redis":{"service_name":"redis.static"}}}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg ExtAuthConfig
			err := parseCacheConfig(gjson.Parse(tt.json), &cfg)
			assert.NoError(t, err)
			assert.False(t, cfg.Cache.Enabled)
			assert.Zero(t, cfg.Cache.TTL)
			assert.Nil(t, cfg.Cache.Client)
		})
	}
}

func TestParseCacheConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		json        string
		expectedErr string
	}{
		{
			name:        "Enabled Without Redis",
			json:        `{"cache":{"enabled":true,"ttl":60}}`,
			expectedErr: "cache.redis.service_name must not be empty when cache is enabled",
		},
		{
			name:        "Enabled With Empty Redis Service Name",
			json:        `{"cache":{"enabled":true,"ttl":60,"redis":{"service_name":""}}}`,
			expectedErr: "cache.redis.service_name must not be empty when cache is enabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg ExtAuthConfig
			err := parseCacheConfig(gjson.Parse(tt.json), &cfg)
			assert.EqualError(t, err, tt.expectedErr)
			assert.False(t, cfg.Cache.Enabled)
		})
	}
}

func TestParseCacheKeyFields(t *testing.T) {
	t.Run("Absent yields nil", func(t *testing.T) {
		fields, err := parseCacheKeyFields(gjson.Parse(`{"cache":{"enabled":true}}`).Get("cache.key_fields"))
		assert.NoError(t, err)
		assert.Nil(t, fields)
	})

	t.Run("Valid header and query mix keeps order", func(t *testing.T) {
		fields, err := parseCacheKeyFields(gjson.Parse(
			`[{"source":"header","key":"x-app-key"},{"source":"query","key":"userId"}]`))
		assert.NoError(t, err)
		assert.Equal(t, []CacheKeyField{
			{Source: KeyFieldSourceHeader, Key: "x-app-key"},
			{Source: KeyFieldSourceQuery, Key: "userId"},
		}, fields)
	})

	t.Run("Header key is lowercased, query key is preserved", func(t *testing.T) {
		fields, err := parseCacheKeyFields(gjson.Parse(
			`[{"source":"header","key":"X-App-Key"},{"source":"query","key":"UserId"}]`))
		assert.NoError(t, err)
		assert.Equal(t, []CacheKeyField{
			{Source: KeyFieldSourceHeader, Key: "x-app-key"},
			{Source: KeyFieldSourceQuery, Key: "UserId"},
		}, fields)
	})

	invalid := []struct {
		name string
		json string
	}{
		{name: "Not An Array", json: `{"source":"header","key":"x"}`},
		{name: "Item Not An Object", json: `["x-app-key"]`},
		{name: "Bad Source", json: `[{"source":"cookie","key":"x"}]`},
		{name: "Missing Source", json: `[{"key":"x"}]`},
		{name: "Empty Key", json: `[{"source":"header","key":""}]`},
		{name: "Missing Key", json: `[{"source":"query"}]`},
	}
	for _, tt := range invalid {
		t.Run("Invalid "+tt.name, func(t *testing.T) {
			fields, err := parseCacheKeyFields(gjson.Parse(tt.json))
			assert.Error(t, err)
			assert.Nil(t, fields)
		})
	}
}

func TestParseCacheConfigRejectsInvalidKeyFields(t *testing.T) {
	// key_fields is parsed before the redis block, so an invalid list rejects the
	// whole config without ever initializing a Redis client.
	var cfg ExtAuthConfig
	err := parseCacheConfig(gjson.Parse(`{"cache":{"enabled":true,"ttl":60,"key_fields":{"source":"header"}}}`), &cfg)
	assert.EqualError(t, err, "cache.key_fields must be an array")
	assert.False(t, cfg.Cache.Enabled)
	assert.Nil(t, cfg.Cache.Client)
}
