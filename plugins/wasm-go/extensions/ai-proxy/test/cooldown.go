package test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

// setUpstreamResponse marks the response as coming from upstream so onHttpResponseHeaders processes it
func setUpstreamResponse(host test.TestHost) {
	_ = host.SetProperty([]string{"response", "code_details"}, []byte("via_upstream"))
}

// 测试配置：cooldown only（无 healthCheckModel）
var cooldownOnlyConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":      "openai",
			"apiTokens": []string{"sk-token-a", "sk-token-b"},
			"modelMapping": map[string]string{
				"*": "gpt-3.5-turbo",
			},
			"failover": map[string]interface{}{
				"enabled":          true,
				"failureThreshold": 1,
				"cooldownDuration": 100,
				"failoverOnStatus": []string{"429"},
			},
		},
	})
	return data
}()

// 测试配置：cooldown + healthCheck 同时配置
var cooldownWithHealthCheckConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":      "openai",
			"apiTokens": []string{"sk-token-a", "sk-token-b"},
			"modelMapping": map[string]string{
				"*": "gpt-3.5-turbo",
			},
			"failover": map[string]interface{}{
				"enabled":            true,
				"failureThreshold":   1,
				"cooldownDuration":   100,
				"healthCheckModel":   "gpt-3.5-turbo",
				"healthCheckTimeout": 5000,
				"failoverOnStatus":   []string{"429"},
			},
		},
	})
	return data
}()

// 测试配置：failover 启用但既没有 cooldown 也没有 healthCheckModel
var failoverNoRecoveryConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":      "openai",
			"apiTokens": []string{"sk-token-a"},
			"modelMapping": map[string]string{
				"*": "gpt-3.5-turbo",
			},
			"failover": map[string]interface{}{
				"enabled":          true,
				"failureThreshold": 1,
				"failoverOnStatus": []string{"429"},
			},
		},
	})
	return data
}()

// 测试配置：cooldown 较长，用于测试冷却未到期的场景
var cooldownLongDurationConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":      "openai",
			"apiTokens": []string{"sk-token-a", "sk-token-b"},
			"modelMapping": map[string]string{
				"*": "gpt-3.5-turbo",
			},
			"failover": map[string]interface{}{
				"enabled":          true,
				"failureThreshold": 1,
				"cooldownDuration": 600000,
				"failoverOnStatus": []string{"429"},
			},
		},
	})
	return data
}()

// 测试配置：三个 token，failureThreshold=2
var cooldownThreeTokensConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":      "openai",
			"apiTokens": []string{"sk-token-a", "sk-token-b", "sk-token-c"},
			"modelMapping": map[string]string{
				"*": "gpt-3.5-turbo",
			},
			"failover": map[string]interface{}{
				"enabled":          true,
				"failureThreshold": 2,
				"cooldownDuration": 100,
				"failoverOnStatus": []string{"429"},
			},
		},
	})
	return data
}()

// 测试配置：单个 token + cooldown
var cooldownSingleTokenConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":      "openai",
			"apiTokens": []string{"sk-only-token"},
			"modelMapping": map[string]string{
				"*": "gpt-3.5-turbo",
			},
			"failover": map[string]interface{}{
				"enabled":          true,
				"failureThreshold": 1,
				"cooldownDuration": 100,
				"failoverOnStatus": []string{"429"},
			},
		},
	})
	return data
}()

// 测试配置：cooldown + 多种 failoverOnStatus
var cooldownMultiStatusConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":      "openai",
			"apiTokens": []string{"sk-token-a", "sk-token-b"},
			"modelMapping": map[string]string{
				"*": "gpt-3.5-turbo",
			},
			"failover": map[string]interface{}{
				"enabled":          true,
				"failureThreshold": 1,
				"cooldownDuration": 100,
				"failoverOnStatus": []string{"429", "5.*"},
			},
		},
	})
	return data
}()

// ============ Parse Config Tests ============

func RunCooldownParseConfigTests(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// cooldown only 配置应正常启动
		t.Run("cooldown only config starts ok", func(t *testing.T) {
			host, status := test.NewTestHost(cooldownOnlyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// cooldown + healthCheck 同时配置应正常启动
		t.Run("cooldown with healthCheck config starts ok", func(t *testing.T) {
			host, status := test.NewTestHost(cooldownWithHealthCheckConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// failover 启用但既没有 cooldown 也没有 healthCheckModel 应启动失败
		t.Run("failover without recovery config fails", func(t *testing.T) {
			host, status := test.NewTestHost(failoverNoRecoveryConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})

		// 单 token + cooldown 配置应正常启动
		t.Run("single token with cooldown config starts ok", func(t *testing.T) {
			host, status := test.NewTestHost(cooldownSingleTokenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})
	})
}

// ============ Failover on 429 Tests ============

func RunCooldownOnHttpResponseHeadersTests(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// 429 响应应触发 failover 日志
		t.Run("429 triggers failover", func(t *testing.T) {
			host, status := test.NewTestHost(cooldownOnlyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			host.CallOnHttpRequestBody([]byte(`{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"hi"}]}`))

			setUpstreamResponse(host)
			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "429"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.ActionContinue, action)

			// 验证 failover 日志
			warnLogs := host.GetWarnLogs()
			hasFailoverLog := false
			for _, log := range warnLogs {
				if strings.Contains(log, "need failover") && strings.Contains(log, "429") {
					hasFailoverLog = true
					break
				}
			}
			require.True(t, hasFailoverLog, "Should have failover warning log on 429")
		})

		// 200 响应不应触发 failover
		t.Run("200 does not trigger failover", func(t *testing.T) {
			host, status := test.NewTestHost(cooldownOnlyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			host.CallOnHttpRequestBody([]byte(`{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"hi"}]}`))

			setUpstreamResponse(host)
			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.ActionContinue, action)

			warnLogs := host.GetWarnLogs()
			for _, log := range warnLogs {
				require.False(t, strings.Contains(log, "need failover"), "Should not have failover log on 200")
			}
		})

		// 非 failoverOnStatus 的错误码不应触发 failover
		t.Run("500 does not trigger failover when only 429 configured", func(t *testing.T) {
			host, status := test.NewTestHost(cooldownOnlyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			host.CallOnHttpRequestBody([]byte(`{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"hi"}]}`))

			setUpstreamResponse(host)
			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "500"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.ActionContinue, action)

			warnLogs := host.GetWarnLogs()
			for _, log := range warnLogs {
				require.False(t, strings.Contains(log, "need failover"), "Should not have failover log on 500 when only 429 configured")
			}
		})

		// 多种 failoverOnStatus 匹配测试：500 应触发 failover
		t.Run("500 triggers failover when 5xx configured", func(t *testing.T) {
			host, status := test.NewTestHost(cooldownMultiStatusConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			host.CallOnHttpRequestBody([]byte(`{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"hi"}]}`))

			setUpstreamResponse(host)
			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "500"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.ActionContinue, action)

			warnLogs := host.GetWarnLogs()
			hasFailoverLog := false
			for _, log := range warnLogs {
				if strings.Contains(log, "need failover") {
					hasFailoverLog = true
					break
				}
			}
			require.True(t, hasFailoverLog, "Should have failover log on 500 when 5.* configured")
		})
	})
}

// ============ Cooldown Recovery Tests ============

func RunCooldownRecoveryTests(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// token 被摘除后，冷却到期后 tick 应恢复
		t.Run("token recovered after cooldown expires", func(t *testing.T) {
			host, status := test.NewTestHost(cooldownOnlyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 触发一次 429 使 token 被摘除（failureThreshold=1）
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			host.CallOnHttpRequestBody([]byte(`{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"hi"}]}`))
			setUpstreamResponse(host)
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "429"},
				{"Content-Type", "application/json"},
			})

			// 验证 token 被标记为不可用
			infoLogs := host.GetInfoLogs()
			hasUnavailableLog := false
			for _, log := range infoLogs {
				if strings.Contains(log, "is unavailable now") {
					hasUnavailableLog = true
					break
				}
			}
			require.True(t, hasUnavailableLog, "Token should be marked as unavailable after 429")

			// 等待冷却到期（cooldownDuration=100ms）
			time.Sleep(150 * time.Millisecond)

			// 触发 tick 执行冷却恢复
			host.Tick()

			// 验证 token 被恢复
			infoLogs = host.GetInfoLogs()
			hasRecoveryLog := false
			for _, log := range infoLogs {
				if strings.Contains(log, "cooldown recovery") && strings.Contains(log, "restoring to available list") {
					hasRecoveryLog = true
					break
				}
			}
			require.True(t, hasRecoveryLog, "Token should be recovered after cooldown expires and tick fires")
		})

		// 冷却未到期时 tick 不应恢复 token
		t.Run("token not recovered before cooldown expires", func(t *testing.T) {
			host, status := test.NewTestHost(cooldownLongDurationConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 触发 429 使 token 被摘除
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			host.CallOnHttpRequestBody([]byte(`{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"hi"}]}`))
			setUpstreamResponse(host)
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "429"},
				{"Content-Type", "application/json"},
			})

			// 立即 tick，冷却未到期（cooldownDuration=600000ms）
			host.Tick()

			// 验证 token 未被恢复
			infoLogs := host.GetInfoLogs()
			for _, log := range infoLogs {
				require.False(t, strings.Contains(log, "cooldown recovery"),
					"Token should NOT be recovered before cooldown expires")
			}
		})

		// failureThreshold > 1 时，单次失败不应摘除 token
		t.Run("single failure does not remove token when threshold is 2", func(t *testing.T) {
			host, status := test.NewTestHost(cooldownThreeTokensConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 只触发一次 429（failureThreshold=2）
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			host.CallOnHttpRequestBody([]byte(`{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"hi"}]}`))
			setUpstreamResponse(host)
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "429"},
				{"Content-Type", "application/json"},
			})

			// 验证 token 未被摘除（需要连续 2 次失败）
			infoLogs := host.GetInfoLogs()
			for _, log := range infoLogs {
				require.False(t, strings.Contains(log, "is unavailable now"),
					"Token should NOT be removed after single failure when threshold is 2")
			}

			// 验证有 debug 日志记录失败次数
			debugLogs := host.GetDebugLogs()
			hasThresholdLog := false
			for _, log := range debugLogs {
				if strings.Contains(log, "has not reached the failure threshold") {
					hasThresholdLog = true
					break
				}
			}
			require.True(t, hasThresholdLog, "Should log that failure threshold not reached")
		})

		// 单个 token 被摘除后，应使用不可用 token 兜底
		t.Run("single token fallback to unavailable when all removed", func(t *testing.T) {
			host, status := test.NewTestHost(cooldownSingleTokenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 触发 429 使唯一的 token 被摘除
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			host.CallOnHttpRequestBody([]byte(`{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"hi"}]}`))
			setUpstreamResponse(host)
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "429"},
				{"Content-Type", "application/json"},
			})
			host.CompleteHttp()

			// 发起新请求，应使用不可用 token 兜底
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			requestHeaders := host.GetRequestHeaders()
			authValue, hasAuth := test.GetHeaderValue(requestHeaders, "Authorization")
			require.True(t, hasAuth, "Authorization header should exist even when all tokens unavailable")
			require.Contains(t, authValue, "sk-only-token", "Should fallback to the unavailable token")

			// 验证有 warn 日志
			warnLogs := host.GetWarnLogs()
			hasAllUnavailableLog := false
			for _, log := range warnLogs {
				if strings.Contains(log, "all tokens are unavailable") {
					hasAllUnavailableLog = true
					break
				}
			}
			require.True(t, hasAllUnavailableLog, "Should warn that all tokens are unavailable")
		})

		// 单个 token 被摘除后，冷却到期后恢复，新请求应正常使用
		t.Run("single token recovered after cooldown and used in new request", func(t *testing.T) {
			host, status := test.NewTestHost(cooldownSingleTokenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 触发 429 使 token 被摘除
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			host.CallOnHttpRequestBody([]byte(`{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"hi"}]}`))
			setUpstreamResponse(host)
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "429"},
				{"Content-Type", "application/json"},
			})
			host.CompleteHttp()

			// 等待冷却到期
			time.Sleep(150 * time.Millisecond)
			host.Tick()

			// 验证恢复日志
			infoLogs := host.GetInfoLogs()
			hasRecoveryLog := false
			for _, log := range infoLogs {
				if strings.Contains(log, "cooldown recovery") {
					hasRecoveryLog = true
					break
				}
			}
			require.True(t, hasRecoveryLog, "Token should be recovered after cooldown")

			// 发起新请求，应正常使用恢复的 token（不再有 all tokens unavailable 警告）
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			requestHeaders := host.GetRequestHeaders()
			authValue, hasAuth := test.GetHeaderValue(requestHeaders, "Authorization")
			require.True(t, hasAuth, "Authorization header should exist")
			require.Contains(t, authValue, "sk-only-token", "Should use the recovered token")
		})

		// 两个 token，一个被摘除后另一个继续使用
		t.Run("second token used after first is removed", func(t *testing.T) {
			host, status := test.NewTestHost(cooldownOnlyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 触发 429 使一个 token 被摘除
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			host.CallOnHttpRequestBody([]byte(`{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"hi"}]}`))
			setUpstreamResponse(host)
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "429"},
				{"Content-Type", "application/json"},
			})
			host.CompleteHttp()

			// 验证 token 被摘除
			infoLogs := host.GetInfoLogs()
			hasUnavailableLog := false
			for _, log := range infoLogs {
				if strings.Contains(log, "is unavailable now") {
					hasUnavailableLog = true
					break
				}
			}
			require.True(t, hasUnavailableLog, "One token should be marked unavailable")

			// 发起新请求，应使用剩余的可用 token
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			requestHeaders := host.GetRequestHeaders()
			authValue, hasAuth := test.GetHeaderValue(requestHeaders, "Authorization")
			require.True(t, hasAuth, "Authorization header should exist")
			require.True(t, strings.Contains(authValue, "sk-token-"), "Should use one of the configured tokens")
		})

		// 成功请求应重置失败计数
		t.Run("successful request resets failure count", func(t *testing.T) {
			host, status := test.NewTestHost(cooldownThreeTokensConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 第一次请求 429（failureThreshold=2，不会摘除）
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			host.CallOnHttpRequestBody([]byte(`{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"hi"}]}`))
			setUpstreamResponse(host)
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "429"},
				{"Content-Type", "application/json"},
			})
			host.CompleteHttp()

			// 第二次请求成功（200），应重置失败计数
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			host.CallOnHttpRequestBody([]byte(`{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"hi"}]}`))
			setUpstreamResponse(host)
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"Content-Type", "application/json"},
			})
			host.CompleteHttp()

			// 第三次请求 429（因为计数已重置，不应摘除）
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			host.CallOnHttpRequestBody([]byte(`{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"hi"}]}`))
			setUpstreamResponse(host)
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "429"},
				{"Content-Type", "application/json"},
			})

			// 验证 token 未被摘除
			infoLogs := host.GetInfoLogs()
			for _, log := range infoLogs {
				require.False(t, strings.Contains(log, "is unavailable now"),
					"Token should NOT be removed because failure count was reset by successful request")
			}
		})
	})
}
