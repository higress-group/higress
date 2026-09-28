// Copyright (c) 2024 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"ext-auth/config"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/resp"
)

// 测试配置：开启认证结果缓存（envoy 模式 + allowed_upstream_headers）
var cacheConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"http_service": map[string]interface{}{
			"endpoint_mode": "envoy",
			"endpoint": map[string]interface{}{
				"service_name": "ext-auth.backend.svc.cluster.local",
				"service_port": 8090,
				"path_prefix":  "/auth",
			},
			"timeout": 1000,
			"authorization_response": map[string]interface{}{
				"allowed_upstream_headers": []map[string]interface{}{
					{"exact": "x-user-id"},
				},
			},
		},
		"cache": map[string]interface{}{
			"enabled": true,
			"ttl":     60,
			"redis": map[string]interface{}{
				"service_name": "redis.static",
				"service_port": 6379,
			},
		},
	})
	return data
}()

// 测试配置：缓存 TTL 超过上限，应被 clamp 到 MaxCacheTTL
var cacheClampConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"http_service": map[string]interface{}{
			"endpoint_mode": "envoy",
			"endpoint": map[string]interface{}{
				"service_name": "ext-auth.backend.svc.cluster.local",
				"service_port": 8090,
				"path_prefix":  "/auth",
			},
			"timeout": 1000,
		},
		"cache": map[string]interface{}{
			"enabled": true,
			"ttl":     9999,
			"redis": map[string]interface{}{
				"service_name": "redis.static",
				"service_port": 6379,
			},
		},
	})
	return data
}()

// 测试配置：开启缓存但同时转发请求体，缓存应被跳过
var cacheWithBodyConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"http_service": map[string]interface{}{
			"endpoint_mode": "envoy",
			"endpoint": map[string]interface{}{
				"service_name": "ext-auth.backend.svc.cluster.local",
				"service_port": 8090,
				"path_prefix":  "/auth",
			},
			"timeout": 1000,
			"authorization_request": map[string]interface{}{
				"with_request_body": true,
			},
		},
		"cache": map[string]interface{}{
			"enabled": true,
			"ttl":     60,
			"redis": map[string]interface{}{
				"service_name": "redis.static",
				"service_port": 6379,
			},
		},
	})
	return data
}()

// TestBuildCacheKey 验证缓存 key 的确定性、命名空间前缀，以及对方法/路径/请求头变化的敏感性。
// 该用例不触发日志，可在没有 host 的情况下直接运行。
func TestBuildCacheKey(t *testing.T) {
	headers := http.Header{}
	headers.Set("authorization", "Bearer token123")
	headers.Set("x-custom", "v")

	key := buildCacheKey("POST", "/auth/users", headers)
	require.True(t, strings.HasPrefix(key, cacheKeyPrefix))
	// 确定性：相同输入产生相同 key
	require.Equal(t, key, buildCacheKey("POST", "/auth/users", headers))
	// 方法敏感
	require.NotEqual(t, key, buildCacheKey("GET", "/auth/users", headers))
	// 路径敏感
	require.NotEqual(t, key, buildCacheKey("POST", "/auth/orders", headers))
	// 请求头取值敏感（凭据变化必须换 key）
	changed := headers.Clone()
	changed.Set("authorization", "Bearer other")
	require.NotEqual(t, key, buildCacheKey("POST", "/auth/users", changed))
	// 新增请求头敏感
	extra := headers.Clone()
	extra.Set("x-extra", "1")
	require.NotEqual(t, key, buildCacheKey("POST", "/auth/users", extra))
	// 与请求头插入顺序无关
	reordered := http.Header{}
	reordered.Set("x-custom", "v")
	reordered.Set("authorization", "Bearer token123")
	require.Equal(t, key, buildCacheKey("POST", "/auth/users", reordered))
}

// TestDecodeCachedHeaders 只覆盖不触发日志的分支（命中/缺 allow 判定/allow:false/null/error/空串）。
// 损坏 JSON 分支会写日志，放到带 host 的运行时用例里覆盖，避免 nil logger panic。
func TestDecodeCachedHeaders(t *testing.T) {
	// 命中：allow:true 时返回注入头
	got, ok := decodeCachedHeaders(resp.StringValue(`{"allow":true,"status":200,"upstream_headers":{"x-user-id":"u1"}}`))
	require.True(t, ok)
	require.Equal(t, map[string]string{"x-user-id": "u1"}, got)

	// 命中但无注入头：allow:true + 空 upstream_headers 仍是合法命中
	got, ok = decodeCachedHeaders(resp.StringValue(`{"allow":true,"status":200,"upstream_headers":{}}`))
	require.True(t, ok)
	require.Empty(t, got)

	// 缺 allow 判定：即使带有 upstream_headers 也当 miss（只回放明确的放行）
	_, ok = decodeCachedHeaders(resp.StringValue(`{"status":200,"upstream_headers":{"x-user-id":"u1"}}`))
	require.False(t, ok)

	// 空对象：无 allow 判定，miss
	_, ok = decodeCachedHeaders(resp.StringValue(`{}`))
	require.False(t, ok)

	// 显式 allow:false：miss
	_, ok = decodeCachedHeaders(resp.StringValue(`{"allow":false,"status":403,"upstream_headers":{}}`))
	require.False(t, ok)

	// miss（null）
	_, ok = decodeCachedHeaders(resp.NullValue())
	require.False(t, ok)

	// redis 错误
	_, ok = decodeCachedHeaders(resp.ErrorValue(errors.New("redis down")))
	require.False(t, ok)

	// 空串
	_, ok = decodeCachedHeaders(resp.StringValue(""))
	require.False(t, ok)
}

// TestCacheConfigParsing 通过 host 解析配置，验证 cache 块的启用、TTL clamp 与默认关闭。
func TestCacheConfigParsing(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		t.Run("cache enabled", func(t *testing.T) {
			host, status := test.NewTestHost(cacheConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			cfgAny, err := host.GetMatchConfig()
			require.NoError(t, err)
			cfg := cfgAny.(*config.ExtAuthConfig)
			require.True(t, cfg.Cache.Enabled)
			require.Equal(t, 60, cfg.Cache.TTL)
			require.NotNil(t, cfg.Cache.Client)
		})

		t.Run("cache ttl clamped to max", func(t *testing.T) {
			host, status := test.NewTestHost(cacheClampConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			cfgAny, err := host.GetMatchConfig()
			require.NoError(t, err)
			cfg := cfgAny.(*config.ExtAuthConfig)
			require.True(t, cfg.Cache.Enabled)
			require.Equal(t, config.MaxCacheTTL, cfg.Cache.TTL)
		})

		t.Run("cache disabled by default", func(t *testing.T) {
			host, status := test.NewTestHost(basicEnvoyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			cfgAny, err := host.GetMatchConfig()
			require.NoError(t, err)
			cfg := cfgAny.(*config.ExtAuthConfig)
			require.False(t, cfg.Cache.Enabled)
			require.Nil(t, cfg.Cache.Client)
		})
	})
}

// TestAuthCacheFlow 在 host 上端到端验证缓存命中/未命中/失败放行/写回/跳过等运行时行为。
func TestAuthCacheFlow(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// (a) 命中：直接复用上次放行结果，不调用认证服务
		t.Run("cache hit skips auth server", func(t *testing.T) {
			host, status := test.NewTestHost(cacheConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/users"},
				{":method", "POST"},
				{"authorization", "Bearer token123"},
			})
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			// 应先发起一次 redis GET（在消费前快照，框架会移除已消费的 callout）
			redisCalls := host.GetRedisCalloutAttributes()
			require.Len(t, redisCalls, 1)
			query := string(redisCalls[0].Query)
			require.Contains(t, query, "get")
			require.Contains(t, query, cacheKeyPrefix)

			// 返回缓存的放行结果（{allow,status,upstream_headers} 信封）
			host.CallOnRedisCall(0, test.CreateRedisRespString(`{"allow":true,"status":200,"upstream_headers":{"x-user-id":"u1"}}`))

			// 请求被恢复，且从未调用认证服务
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())
			require.Empty(t, host.GetHttpCalloutAttributes())

			// 缓存中的头被注入到上游请求
			injected := false
			for _, h := range host.GetRequestHeaders() {
				if strings.EqualFold(h[0], "x-user-id") && h[1] == "u1" {
					injected = true
				}
			}
			require.True(t, injected, "x-user-id should be replayed from cache")

			host.CompleteHttp()
		})

		// (b) 未命中：回源认证服务，放行后写回缓存
		t.Run("cache miss falls through to auth server and writes back", func(t *testing.T) {
			host, status := test.NewTestHost(cacheConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/users"},
				{":method", "POST"},
				{"authorization", "Bearer token123"},
			})
			require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)

			// GET 返回 miss
			host.CallOnRedisCall(0, test.CreateRedisRespNull())

			// 此时应调用认证服务
			require.Len(t, host.GetHttpCalloutAttributes(), 1)

			// 认证服务放行并返回 x-user-id
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"x-user-id", "u1"},
				{"content-type", "application/json"},
			}, []byte(`{"authorized": true}`))
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			// 放行结果应以 SETNX 写回 redis（SET ... NX EX，并发双 miss 时先到者胜）
			redisCalls := host.GetRedisCalloutAttributes()
			require.Len(t, redisCalls, 1)
			writeQuery := string(redisCalls[0].Query)
			require.Contains(t, writeQuery, "set")
			require.Contains(t, writeQuery, "nx")
			require.Contains(t, writeQuery, "u1")

			// 确认写回
			host.CallOnRedisCall(0, test.CreateRedisRespString("OK"))
			host.CompleteHttp()
		})

		// (c) redis 错误：失败放行，回源认证服务
		t.Run("redis error fails open to auth server", func(t *testing.T) {
			host, status := test.NewTestHost(cacheConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/users"},
				{":method", "POST"},
				{"authorization", "Bearer token123"},
			})

			host.CallOnRedisCall(0, test.CreateRedisRespError("redis unavailable"))

			require.Len(t, host.GetHttpCalloutAttributes(), 1)
			host.CompleteHttp()
		})

		// (d) 缓存值损坏：失败放行，回源认证服务（覆盖 decodeCachedHeaders 的损坏分支）
		t.Run("corrupt cache value fails open to auth server", func(t *testing.T) {
			host, status := test.NewTestHost(cacheConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/users"},
				{":method", "POST"},
				{"authorization", "Bearer token123"},
			})

			host.CallOnRedisCall(0, test.CreateRedisRespString("not-json"))

			require.Len(t, host.GetHttpCalloutAttributes(), 1)
			host.CompleteHttp()
		})

		// (e) 写回失败：不阻塞已被放行的请求
		t.Run("cache write failure does not block the request", func(t *testing.T) {
			host, status := test.NewTestHost(cacheConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/users"},
				{":method", "POST"},
				{"authorization", "Bearer token123"},
			})

			// miss
			host.CallOnRedisCall(0, test.CreateRedisRespNull())
			require.Len(t, host.GetHttpCalloutAttributes(), 1)

			// 放行
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"x-user-id", "u1"},
			}, []byte(`{"authorized": true}`))
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			// 写回 SET 失败，请求仍应放行
			require.Len(t, host.GetRedisCalloutAttributes(), 1)
			host.CallOnRedisCall(0, test.CreateRedisRespError("write failed"))
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			host.CompleteHttp()
		})

		// (f) 转发请求体时跳过缓存：直接回源认证服务，不访问 redis
		t.Run("with_request_body skips the cache", func(t *testing.T) {
			host, status := test.NewTestHost(cacheWithBodyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/users"},
				{":method", "POST"},
				{"authorization", "Bearer token123"},
				{"content-type", "application/json"},
			})

			action := host.CallOnHttpRequestBody([]byte(`{"username":"test"}`))
			require.Equal(t, types.DataStopIterationAndBuffer, action)

			require.Empty(t, host.GetRedisCalloutAttributes())
			require.Len(t, host.GetHttpCalloutAttributes(), 1)

			host.CompleteHttp()
		})
	})
}

// 测试配置：key_fields 只选一个 query 参数，用于验证用户指定 key 模式
var cacheKeyFieldsQueryConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"http_service": map[string]interface{}{
			"endpoint_mode": "envoy",
			"endpoint": map[string]interface{}{
				"service_name": "ext-auth.backend.svc.cluster.local",
				"service_port": 8090,
				"path_prefix":  "/auth",
			},
			"timeout": 1000,
		},
		"cache": map[string]interface{}{
			"enabled": true,
			"ttl":     60,
			"key_fields": []map[string]interface{}{
				{"source": "query", "key": "userId"},
			},
			"redis": map[string]interface{}{
				"service_name": "redis.static",
				"service_port": 6379,
			},
		},
	})
	return data
}()

// 测试配置：key_fields 只选一个 header，用于验证 header 读取与 Authorization 不被自动纳入
var cacheKeyFieldsHeaderConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"http_service": map[string]interface{}{
			"endpoint_mode": "envoy",
			"endpoint": map[string]interface{}{
				"service_name": "ext-auth.backend.svc.cluster.local",
				"service_port": 8090,
				"path_prefix":  "/auth",
			},
			"timeout": 1000,
		},
		"cache": map[string]interface{}{
			"enabled": true,
			"ttl":     60,
			"key_fields": []map[string]interface{}{
				{"source": "header", "key": "x-app-key"},
			},
			"redis": map[string]interface{}{
				"service_name": "redis.static",
				"service_port": 6379,
			},
		},
	})
	return data
}()

// TestSplitPathAndQuery 验证按第一个 '?' 切分，路径部分保持原样不被重新编码。
func TestSplitPathAndQuery(t *testing.T) {
	path, query := splitPathAndQuery("/users?userId=1&trace=x")
	require.Equal(t, "/users", path)
	require.Equal(t, "userId=1&trace=x", query)

	// 无 query
	path, query = splitPathAndQuery("/users")
	require.Equal(t, "/users", path)
	require.Equal(t, "", query)

	// 空 query（只有 '?'）
	path, query = splitPathAndQuery("/users?")
	require.Equal(t, "/users", path)
	require.Equal(t, "", query)

	// 路径里含编码字符也不动它，只认第一个 '?'
	path, query = splitPathAndQuery("/a%20b/c?x=1?y=2")
	require.Equal(t, "/a%20b/c", path)
	require.Equal(t, "x=1?y=2", query)
}

// TestBuildCacheKeyFromFields 验证纯哈希助手：确定性、方法/路径敏感、字段值与顺序敏感。
func TestBuildCacheKeyFromFields(t *testing.T) {
	fields := []keyFieldValue{
		{name: "query:userId", value: "1"},
		{name: "header:x-app-key", value: "k"},
	}
	key := buildCacheKeyFromFields("GET", "/users", fields)
	require.True(t, strings.HasPrefix(key, cacheKeyPrefix))
	// 确定性
	require.Equal(t, key, buildCacheKeyFromFields("GET", "/users", fields))
	// 方法敏感
	require.NotEqual(t, key, buildCacheKeyFromFields("POST", "/users", fields))
	// 路径敏感
	require.NotEqual(t, key, buildCacheKeyFromFields("GET", "/orders", fields))
	// 字段值敏感
	changed := []keyFieldValue{{name: "query:userId", value: "2"}, {name: "header:x-app-key", value: "k"}}
	require.NotEqual(t, key, buildCacheKeyFromFields("GET", "/users", changed))
	// 顺序敏感（声明顺序进入摘要）
	reordered := []keyFieldValue{{name: "header:x-app-key", value: "k"}, {name: "query:userId", value: "1"}}
	require.NotEqual(t, key, buildCacheKeyFromFields("GET", "/users", reordered))
	// name 前缀避免 header/query 同名冲突
	collideA := []keyFieldValue{{name: "header:id", value: "v"}}
	collideB := []keyFieldValue{{name: "query:id", value: "v"}}
	require.NotEqual(t,
		buildCacheKeyFromFields("GET", "/users", collideA),
		buildCacheKeyFromFields("GET", "/users", collideB))
}

// TestBuildRequestFieldCacheKey 直接验证纯函数（不依赖 host）：query 模式只认列出的
// query 参数、header 模式大小写不敏感、Authorization 不被自动纳入、路径以去 query 的
// 形式兜底、缺失字段贡献空值。
func TestBuildRequestFieldCacheKey(t *testing.T) {
	queryFields := []config.CacheKeyField{{Source: config.KeyFieldSourceQuery, Key: "userId"}}
	headerFields := []config.CacheKeyField{{Source: config.KeyFieldSourceHeader, Key: "x-app-key"}}

	t.Run("query field mode", func(t *testing.T) {
		authA := [][2]string{{"authorization", "Bearer A"}}
		base := buildRequestFieldCacheKey("GET", "/users?userId=1", authA, queryFields)
		require.True(t, strings.HasPrefix(base, cacheKeyPrefix))

		// Authorization 未列入 key_fields：换一个 bearer，key 不变
		require.Equal(t, base, buildRequestFieldCacheKey("GET", "/users?userId=1",
			[][2]string{{"authorization", "Bearer B"}}, queryFields))
		// 未列入的 query 参数被忽略；路径以去 query 的形式兜底
		require.Equal(t, base, buildRequestFieldCacheKey("GET", "/users?userId=1&trace=xyz", authA, queryFields))
		// 列入的 query 参数变化 -> key 变化
		require.NotEqual(t, base, buildRequestFieldCacheKey("GET", "/users?userId=2", authA, queryFields))
		// 方法是兜底的一部分
		require.NotEqual(t, base, buildRequestFieldCacheKey("POST", "/users?userId=1", authA, queryFields))
		// 缺失的 query 参数贡献空值，与显式空值等价
		require.Equal(t,
			buildRequestFieldCacheKey("GET", "/users", nil, queryFields),
			buildRequestFieldCacheKey("GET", "/users?userId=", nil, queryFields))
	})

	t.Run("header field mode is case-insensitive", func(t *testing.T) {
		base := buildRequestFieldCacheKey("GET", "/users",
			[][2]string{{"x-app-key", "k1"}, {"authorization", "Bearer A"}}, headerFields)

		// 请求头名大小写不影响取值（配置侧 key 已在解析期归一小写）
		require.Equal(t, base, buildRequestFieldCacheKey("GET", "/users",
			[][2]string{{"X-App-Key", "k1"}, {"authorization", "Bearer A"}}, headerFields))
		require.Equal(t, base, buildRequestFieldCacheKey("GET", "/users",
			[][2]string{{"X-APP-KEY", "k1"}}, headerFields))
		// 列入的 header 值变化 -> key 变化
		require.NotEqual(t, base, buildRequestFieldCacheKey("GET", "/users",
			[][2]string{{"x-app-key", "k2"}}, headerFields))
		// Authorization 不被自动纳入：换 bearer，key 不变
		require.Equal(t, base, buildRequestFieldCacheKey("GET", "/users",
			[][2]string{{"x-app-key", "k1"}, {"authorization", "Bearer B"}}, headerFields))
		// query 不影响 header 模式的 key（未列入 query，路径去 query 兜底）
		require.Equal(t, base, buildRequestFieldCacheKey("GET", "/users?anything=1",
			[][2]string{{"x-app-key", "k1"}}, headerFields))
		// 缺失的 header 贡献空值，与存在的 header 不同
		require.NotEqual(t, base, buildRequestFieldCacheKey("GET", "/users",
			[][2]string{{"authorization", "Bearer A"}}, headerFields))
	})
}

// TestKeyFieldsCacheKeyWiring 在 host 上验证 main.go 的接线：key_fields 模式确实读取
// 原始入站请求（方法、含 query 的路径、原始请求头），而非过滤后的转发头集合。
func TestKeyFieldsCacheKeyWiring(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		queryKey := func(cfg json.RawMessage, method, path string, extra [][2]string) string {
			host, status := test.NewTestHost(cfg)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			headers := [][2]string{
				{":authority", "example.com"},
				{":path", path},
				{":method", method},
			}
			headers = append(headers, extra...)
			host.CallOnHttpRequestHeaders(headers)

			calls := host.GetRedisCalloutAttributes()
			require.Len(t, calls, 1)
			return extractCacheKey(t, string(calls[0].Query))
		}

		// header 接线：读的是原始入站头（大小写混合也能命中），不是过滤后的转发头
		lower := queryKey(cacheKeyFieldsHeaderConfig, "GET", "/users",
			[][2]string{{"x-app-key", "k1"}, {"authorization", "Bearer A"}})
		require.Equal(t, lower, queryKey(cacheKeyFieldsHeaderConfig, "GET", "/users",
			[][2]string{{"X-App-Key", "k1"}, {"authorization", "Bearer A"}}))
		// 值变化 -> key 变化，证明 main.go 真的读了入站 x-app-key
		require.NotEqual(t, lower, queryKey(cacheKeyFieldsHeaderConfig, "GET", "/users",
			[][2]string{{"x-app-key", "k2"}, {"authorization", "Bearer A"}}))
		// Authorization 不进 key
		require.Equal(t, lower, queryKey(cacheKeyFieldsHeaderConfig, "GET", "/users",
			[][2]string{{"x-app-key", "k1"}, {"authorization", "Bearer B"}}))

		// query 接线：main.go 传入含 query 的原始路径，列入的 query 参数值变化改变 key
		require.NotEqual(t,
			queryKey(cacheKeyFieldsQueryConfig, "GET", "/users?userId=1", nil),
			queryKey(cacheKeyFieldsQueryConfig, "GET", "/users?userId=2", nil))
	})
}

// extractCacheKey 从 redis 命令里取出以 cacheKeyPrefix 开头的 key（到下一个空白/控制符为止）。
func extractCacheKey(t *testing.T, query string) string {
	t.Helper()
	i := strings.Index(query, cacheKeyPrefix)
	require.GreaterOrEqual(t, i, 0, "cache key not found in redis query: %q", query)
	rest := query[i:]
	if j := strings.IndexAny(rest, "\r\n \t"); j >= 0 {
		return rest[:j]
	}
	return rest
}
