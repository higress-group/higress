package main

import (
	"encoding/json"
	"testing"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/response-cache/config"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

// 测试配置：使用header提取key
var configWithHeaderKey = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"cache": map[string]interface{}{
			"type":        "redis",
			"serviceName": "redis.static",
			"servicePort": 6379,
			"timeout":     10000,
		},
		"cacheKeyFromHeader":     "x-user-id",
		"cacheValueFromBody":     "data",
		"cacheValueFromBodyType": "application/json",
		"cacheResponseCode":      []int{200},
	})
	return data
}()

// 测试配置：使用body提取key
var configWithBodyKey = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"cache": map[string]interface{}{
			"type":        "redis",
			"serviceName": "redis.static",
			"servicePort": 6379,
			"timeout":     10000,
		},
		"cacheKeyFromBody":       "user_id",
		"cacheValueFromBody":     "message.content",
		"cacheValueFromBodyType": "application/json",
		"cacheResponseCode":      []int{200},
	})
	return data
}()

// 测试配置：使用整个body作为key
var configWithBodyAsKey = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"cache": map[string]interface{}{
			"type":        "redis",
			"serviceName": "redis.static",
			"servicePort": 6379,
			"timeout":     10000,
		},
		"cacheKeyFromBody":       "",
		"cacheValueFromBody":     "",
		"cacheValueFromBodyType": "application/json",
		"cacheResponseCode":      []int{200},
	})
	return data
}()

// 测试配置：配置冲突（同时设置header和body key）
var configConflict = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"cache": map[string]interface{}{
			"type":        "redis",
			"serviceName": "redis.static",
			"servicePort": 6379,
		},
		"cacheKeyFromHeader": "x-user-id",
		"cacheKeyFromBody":   "user_id",
	})
	return data
}()

// 测试配置：最小配置
var minimalConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"cache": map[string]interface{}{
			"type":        "redis",
			"serviceName": "redis.static",
			"servicePort": 6379,
		},
	})
	return data
}()

// 测试配置：缺少cache provider
var configMissingCache = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"cacheKeyFromHeader": "x-user-id",
	})
	return data
}()

func TestParseConfig(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// 测试header key配置
		t.Run("config with header key", func(t *testing.T) {
			host, status := test.NewTestHost(configWithHeaderKey)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			configRaw, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, configRaw)

			cfg, ok := configRaw.(*config.PluginConfig)
			require.True(t, ok)
			require.Equal(t, "x-user-id", cfg.CacheKeyFromHeader)
			require.Equal(t, "", cfg.CacheKeyFromBody)
			require.Equal(t, "data", cfg.CacheValueFromBody)
			require.Equal(t, []int32{200}, cfg.CacheResponseCode)
		})

		// 测试body key配置
		t.Run("config with body key", func(t *testing.T) {
			host, status := test.NewTestHost(configWithBodyKey)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			configRaw, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, configRaw)

			cfg, ok := configRaw.(*config.PluginConfig)
			require.True(t, ok)
			require.Equal(t, "", cfg.CacheKeyFromHeader)
			require.Equal(t, "user_id", cfg.CacheKeyFromBody)
			require.Equal(t, "message.content", cfg.CacheValueFromBody)
		})

		// 测试整个body作为key
		t.Run("config with body as key", func(t *testing.T) {
			host, status := test.NewTestHost(configWithBodyAsKey)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			configRaw, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, configRaw)

			cfg, ok := configRaw.(*config.PluginConfig)
			require.True(t, ok)
			require.Equal(t, "", cfg.CacheKeyFromHeader)
			require.Equal(t, "", cfg.CacheKeyFromBody)
		})

		// 测试配置冲突
		t.Run("conflict config", func(t *testing.T) {
			host, status := test.NewTestHost(configConflict)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})

		// 测试缺少cache provider
		t.Run("missing cache provider", func(t *testing.T) {
			host, status := test.NewTestHost(configMissingCache)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})

		// 测试最小配置
		t.Run("minimal config", func(t *testing.T) {
			host, status := test.NewTestHost(minimalConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			configRaw, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, configRaw)

			cfg, ok := configRaw.(*config.PluginConfig)
			require.True(t, ok)
			require.Equal(t, []int32{200}, cfg.CacheResponseCode)
		})
	})
}

func TestOnHttpRequestHeaders(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试使用header key的请求头处理
		t.Run("request headers with header key", func(t *testing.T) {
			host, status := test.NewTestHost(configWithHeaderKey)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，包含cache key
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/data"},
				{":method", "GET"},
				{"x-user-id", "user123"},
			})

			// 应该返回ActionContinue，因为从header提取key后继续处理
			require.Equal(t, types.ActionContinue, action)
		})

		// 测试header key为空
		t.Run("request headers with empty header key", func(t *testing.T) {
			host, status := test.NewTestHost(configWithHeaderKey)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，不包含x-user-id
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/data"},
				{":method", "GET"},
			})

			// 应该返回ActionContinue，跳过缓存
			require.Equal(t, types.ActionContinue, action)
		})

		// 测试skip cache header
		t.Run("skip cache header", func(t *testing.T) {
			host, status := test.NewTestHost(configWithHeaderKey)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置跳过缓存的请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/data"},
				{":method", "GET"},
				{"x-user-id", "user123"},
				{"x-higress-skip-response-cache", "on"},
			})

			// 应该返回ActionContinue
			require.Equal(t, types.ActionContinue, action)
		})

		// 测试使用body key的content-type检查
		t.Run("request headers for body key with content type", func(t *testing.T) {
			host, status := test.NewTestHost(configWithBodyKey)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，包含application/json content-type
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/data"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			// 应该返回HeaderStopIteration，等待读取body
			require.Equal(t, types.HeaderStopIteration, action)
		})

		// 测试content-type不匹配
		t.Run("request headers with non-json content type", func(t *testing.T) {
			host, status := test.NewTestHost(configWithBodyKey)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，content-type不是json
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/data"},
				{":method", "POST"},
				{"content-type", "text/plain"},
			})

			// 应该返回ActionContinue，跳过缓存
			require.Equal(t, types.ActionContinue, action)
		})

		// 测试无content-type
		t.Run("request headers without content type", func(t *testing.T) {
			host, status := test.NewTestHost(configWithBodyKey)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，无content-type
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/data"},
				{":method", "POST"},
			})

			// 应该返回ActionContinue，跳过缓存
			require.Equal(t, types.ActionContinue, action)
		})
	})
}

func TestOnHttpRequestBody(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试从body提取key
		t.Run("request body with key extraction", func(t *testing.T) {
			host, status := test.NewTestHost(configWithBodyKey)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/data"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			// 构造请求体
			requestBody := `{"user_id": "user123", "data": "test"}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			// 应该返回ActionPause，等待缓存检查结果
			require.Equal(t, types.ActionPause, action)
		})

		// 测试从body提取key失败（key为空）
		t.Run("request body with empty key", func(t *testing.T) {
			host, status := test.NewTestHost(configWithBodyKey)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/data"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			// 构造请求体，不包含user_id字段
			requestBody := `{"data": "test"}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			// 应该返回ActionContinue，跳过缓存
			require.Equal(t, types.ActionContinue, action)
		})

		// 测试整个body作为key
		t.Run("request body as key", func(t *testing.T) {
			host, status := test.NewTestHost(configWithBodyAsKey)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/data"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			// 构造请求体
			requestBody := `{"data": "test"}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			// 应该返回ActionPause
			require.Equal(t, types.ActionPause, action)
		})
	})
}

func TestOnHttpResponseHeaders(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试响应头处理 - 状态码200
		t.Run("response headers with 200 status", func(t *testing.T) {
			host, status := test.NewTestHost(configWithHeaderKey)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/data"},
				{":method", "GET"},
				{"x-user-id", "user123"},
			})

			// 设置响应头
			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})

			// 应该返回ActionContinue
			require.Equal(t, types.ActionContinue, action)
		})

		// 测试响应头处理 - 状态码500（不支持缓存）
		t.Run("response headers with 500 status", func(t *testing.T) {
			host, status := test.NewTestHost(configWithHeaderKey)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/data"},
				{":method", "GET"},
				{"x-user-id", "user123"},
			})

			// 设置响应头，状态码500
			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "500"},
				{"content-type", "application/json"},
			})

			// 应该返回ActionContinue，但跳过缓存
			require.Equal(t, types.ActionContinue, action)
		})

		// 测试skip cache header的处理
		t.Run("response headers with skip cache", func(t *testing.T) {
			host, status := test.NewTestHost(configWithHeaderKey)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，包含skip cache标志
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/data"},
				{":method", "GET"},
				{"x-user-id", "user123"},
				{"x-higress-skip-response-cache", "on"},
			})

			// 设置响应头
			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})

			// 应该返回ActionContinue
			require.Equal(t, types.ActionContinue, action)
		})
	})
}

func TestIsCacheableResponseStatus(t *testing.T) {
	allowed := []int32{200, 204}
	for _, tc := range []struct {
		name   string
		status string
		want   bool
	}{
		{name: "allowed", status: "200", want: true},
		{name: "valid but not allowed", status: "500", want: false},
		{name: "negative", status: "-1", want: false},
		{name: "below HTTP range", status: "99", want: false},
		{name: "above HTTP range", status: "600", want: false},
		{name: "int32 overflow", status: "2147483648", want: false},
		{name: "not a number", status: "invalid", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, isCacheableResponseStatus(tc.status, allowed))
		})
	}
}

func TestOnHttpResponseBody(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试响应体处理 - 提取特定字段
		t.Run("response body with value extraction", func(t *testing.T) {
			host, status := test.NewTestHost(configWithHeaderKey)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/data"},
				{":method", "GET"},
				{"x-user-id", "user123"},
			})

			// 设置响应头
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})

			// 构造响应体
			responseBody := `{"data": "cached value", "other": "ignored"}`
			action := host.CallOnHttpResponseBody([]byte(responseBody))

			// 应该返回ActionContinue
			require.Equal(t, types.ActionContinue, action)
		})

		// 测试响应体处理 - 整个body作为value
		t.Run("response body as value", func(t *testing.T) {
			host, status := test.NewTestHost(configWithBodyAsKey)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/data"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			// 设置请求体
			host.CallOnHttpRequestBody([]byte(`{"test": "data"}`))

			// 设置响应头
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})

			// 构造响应体
			responseBody := `{"data": "full response"}`
			action := host.CallOnHttpResponseBody([]byte(responseBody))

			// 应该返回ActionContinue
			require.Equal(t, types.ActionContinue, action)
		})

		// 测试无key的响应体处理
		t.Run("response body without key", func(t *testing.T) {
			host, status := test.NewTestHost(configWithHeaderKey)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置响应头，不经过请求处理
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})

			// 构造响应体
			responseBody := `{"data": "test"}`
			host.CallOnHttpResponseBody([]byte(responseBody))
		})
	})
}

// 测试缓存命中流程
func TestCacheHitFlow(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试完整的缓存命中流程
		t.Run("complete cache hit flow with header key", func(t *testing.T) {
			host, status := test.NewTestHost(configWithHeaderKey)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/data"},
				{":method", "GET"},
				{"x-user-id", "user123"},
			})

			// 模拟Redis缓存命中 - 返回之前缓存的data字段值
			cacheHitResp := test.CreateRedisRespString("cached value")
			host.CallOnRedisCall(0, cacheHitResp)

			// 完成HTTP请求
			host.CompleteHttp()

			// 验证缓存命中的响应
			localResp := host.GetLocalResponse()
			require.Equal(t, uint32(200), localResp.StatusCode)
			require.Equal(t, "cached value", string(localResp.Data))
			require.True(t, test.HasHeaderWithValue(localResp.Headers, "content-type", "application/json"))
			require.True(t, test.HasHeaderWithValue(localResp.Headers, "x-cache-status", "hit"))
		})

		// 测试缓存未命中然后存储的流程
		t.Run("cache miss and store flow", func(t *testing.T) {
			host, status := test.NewTestHost(configWithHeaderKey)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/data"},
				{":method", "GET"},
				{"x-user-id", "user123"},
			})

			// 模拟Redis缓存未命中（返回null）
			cacheMissResp := test.CreateRedisRespNull()
			host.CallOnRedisCall(0, cacheMissResp)

			// 设置响应头
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})

			// 设置响应体
			responseBody := `{"data": "new data", "other": "ignored"}`
			action := host.CallOnHttpResponseBody([]byte(responseBody))
			require.Equal(t, types.ActionContinue, action)

			// 模拟Redis存储操作（SET操作返回OK）
			storeResp := test.CreateRedisRespArray([]interface{}{"OK"})
			host.CallOnRedisCall(0, storeResp)

			// 完成HTTP请求
			host.CompleteHttp()
		})

		// 测试两次请求：第一次miss，第二次hit
		t.Run("first request miss then second request hit", func(t *testing.T) {
			host, status := test.NewTestHost(configWithHeaderKey)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// ========== 第一次请求：缓存未命中 ==========
			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/data"},
				{":method", "GET"},
				{"x-user-id", "user123"},
			})

			// 模拟Redis缓存未命中（第一次查询返回null）
			cacheMissResp := test.CreateRedisRespNull()
			host.CallOnRedisCall(0, cacheMissResp)

			// 设置响应头
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})

			// 设置响应体
			responseBody := `{"data": "first response"}`
			action := host.CallOnHttpResponseBody([]byte(responseBody))
			require.Equal(t, types.ActionContinue, action)

			// 模拟Redis SET操作（第一次请求后将数据存入缓存）
			storeResp := test.CreateRedisRespArray([]interface{}{"OK"})
			host.CallOnRedisCall(0, storeResp)
			host.CompleteHttp()

			// 设置请求头（相同的x-user-id）
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/data"},
				{":method", "GET"},
				{"x-user-id", "user123"}, // 相同的user ID
			})

			// 模拟Redis缓存命中（第二次查询返回缓存的数据）
			cacheHitResp := test.CreateRedisRespString("first response")
			host.CallOnRedisCall(0, cacheHitResp)

			// 完成HTTP请求
			host.CompleteHttp()

			// 验证第二次请求返回的是缓存的数据
			localResp := host.GetLocalResponse()
			require.Equal(t, uint32(200), localResp.StatusCode)
			require.Equal(t, "first response", string(localResp.Data))
			require.True(t, test.HasHeaderWithValue(localResp.Headers, "x-cache-status", "hit"))
			require.True(t, test.HasHeaderWithValue(localResp.Headers, "content-type", "application/json"))
		})

		// 测试body key的两次请求流程
		t.Run("body key first miss then second hit", func(t *testing.T) {
			host, status := test.NewTestHost(configWithBodyKey)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 第一次请求
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/data"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			requestBody := `{"user_id": "user123"}`
			host.CallOnHttpRequestBody([]byte(requestBody))

			// Redis缓存未命中
			cacheMissResp := test.CreateRedisRespNull()
			host.CallOnRedisCall(0, cacheMissResp)

			// 响应
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})

			responseBody := `{"message": {"content": "hello world"}}`
			host.CallOnHttpResponseBody([]byte(responseBody))

			// 存储到Redis
			storeResp := test.CreateRedisRespArray([]interface{}{"OK"})
			host.CallOnRedisCall(0, storeResp)
			host.CompleteHttp()

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/data"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})
			host.CallOnHttpRequestBody([]byte(`{"user_id": "user123"}`))

			// 缓存命中
			cacheHitResp := test.CreateRedisRespString("hello world")
			host.CallOnRedisCall(0, cacheHitResp)
			host.CompleteHttp()

			// 验证第二次请求返回的是缓存的数据
			localResp := host.GetLocalResponse()
			require.Equal(t, uint32(200), localResp.StatusCode)
			require.Equal(t, "hello world", string(localResp.Data))
			require.True(t, test.HasHeaderWithValue(localResp.Headers, "x-cache-status", "hit"))
			require.True(t, test.HasHeaderWithValue(localResp.Headers, "content-type", "application/json"))
		})
	})
}
