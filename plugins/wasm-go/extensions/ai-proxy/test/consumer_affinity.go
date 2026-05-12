package test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

// 测试配置：多 API Token 配置（用于测试 consumer affinity）
var multiTokenOpenAIConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":      "openai",
			"apiTokens": []string{"sk-token-1", "sk-token-2", "sk-token-3"},
			"modelMapping": map[string]string{
				"*": "gpt-4",
			},
		},
	})
	return data
}()

// 测试配置：单 API Token 配置
var singleTokenOpenAIConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":      "openai",
			"apiTokens": []string{"sk-single-token"},
			"modelMapping": map[string]string{
				"*": "gpt-4",
			},
		},
	})
	return data
}()

func RunConsumerAffinityParseConfigTests(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		t.Run("multi token config", func(t *testing.T) {
			host, status := test.NewTestHost(multiTokenOpenAIConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})
	})
}

func RunConsumerAffinityOnHttpRequestHeadersTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试 stateful API（responses）使用 consumer affinity
		t.Run("stateful api responses with consumer header", func(t *testing.T) {
			host, status := test.NewTestHost(multiTokenOpenAIConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 使用 x-mse-consumer header
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/responses"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
				{"x-mse-consumer", "consumer-alice"},
			})

			require.Equal(t, types.HeaderStopIteration, action)

			requestHeaders := host.GetRequestHeaders()
			require.NotNil(t, requestHeaders)

			// 验证 Authorization header 使用了其中一个 token
			authValue, hasAuth := test.GetHeaderValue(requestHeaders, "Authorization")
			require.True(t, hasAuth, "Authorization header should exist")
			require.True(t, strings.Contains(authValue, "sk-token-"), "Authorization should contain one of the tokens")
		})

		// 测试 stateful API（files）使用 consumer affinity
		t.Run("stateful api files with consumer header", func(t *testing.T) {
			host, status := test.NewTestHost(multiTokenOpenAIConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/files"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
				{"x-mse-consumer", "consumer-files"},
			})

			require.Equal(t, types.HeaderStopIteration, action)

			requestHeaders := host.GetRequestHeaders()
			require.NotNil(t, requestHeaders)

			authValue, hasAuth := test.GetHeaderValue(requestHeaders, "Authorization")
			require.True(t, hasAuth, "Authorization header should exist")
			require.True(t, strings.Contains(authValue, "sk-token-"), "Authorization should contain one of the tokens")
		})

		// 测试 stateful API（batches）使用 consumer affinity
		t.Run("stateful api batches with consumer header", func(t *testing.T) {
			host, status := test.NewTestHost(multiTokenOpenAIConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/batches"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
				{"x-mse-consumer", "consumer-batches"},
			})

			require.Equal(t, types.HeaderStopIteration, action)

			requestHeaders := host.GetRequestHeaders()
			require.NotNil(t, requestHeaders)

			authValue, hasAuth := test.GetHeaderValue(requestHeaders, "Authorization")
			require.True(t, hasAuth, "Authorization header should exist")
			require.True(t, strings.Contains(authValue, "sk-token-"), "Authorization should contain one of the tokens")
		})

		// 测试 stateful API（fine_tuning）使用 consumer affinity
		t.Run("stateful api fine_tuning with consumer header", func(t *testing.T) {
			host, status := test.NewTestHost(multiTokenOpenAIConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/fine_tuning/jobs"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
				{"x-mse-consumer", "consumer-finetuning"},
			})

			require.Equal(t, types.HeaderStopIteration, action)

			requestHeaders := host.GetRequestHeaders()
			require.NotNil(t, requestHeaders)

			authValue, hasAuth := test.GetHeaderValue(requestHeaders, "Authorization")
			require.True(t, hasAuth, "Authorization header should exist")
			require.True(t, strings.Contains(authValue, "sk-token-"), "Authorization should contain one of the tokens")
		})

		// 测试非 stateful API 正常工作
		t.Run("non stateful api chat completions works normally", func(t *testing.T) {
			host, status := test.NewTestHost(multiTokenOpenAIConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
				{"x-mse-consumer", "consumer-chat"},
			})

			require.Equal(t, types.HeaderStopIteration, action)

			requestHeaders := host.GetRequestHeaders()
			require.NotNil(t, requestHeaders)

			authValue, hasAuth := test.GetHeaderValue(requestHeaders, "Authorization")
			require.True(t, hasAuth, "Authorization header should exist")
			require.True(t, strings.Contains(authValue, "sk-token-"), "Authorization should contain one of the tokens")
		})

		// 测试无 x-mse-consumer header 时正常工作
		t.Run("stateful api without consumer header works normally", func(t *testing.T) {
			host, status := test.NewTestHost(multiTokenOpenAIConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/responses"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			require.Equal(t, types.HeaderStopIteration, action)

			requestHeaders := host.GetRequestHeaders()
			require.NotNil(t, requestHeaders)

			authValue, hasAuth := test.GetHeaderValue(requestHeaders, "Authorization")
			require.True(t, hasAuth, "Authorization header should exist")
			require.True(t, strings.Contains(authValue, "sk-token-"), "Authorization should contain one of the tokens")
		})

		// 测试单个 token 时始终使用该 token
		t.Run("single token always used", func(t *testing.T) {
			host, status := test.NewTestHost(singleTokenOpenAIConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/responses"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
				{"x-mse-consumer", "consumer-test"},
			})

			require.Equal(t, types.HeaderStopIteration, action)

			requestHeaders := host.GetRequestHeaders()
			authValue, _ := test.GetHeaderValue(requestHeaders, "Authorization")
			require.Contains(t, authValue, "sk-single-token", "Single token should always be used")
		})

		// 测试同一 consumer 多次请求获得相同 token（consumer affinity 一致性）
		t.Run("same consumer gets consistent token across requests", func(t *testing.T) {
			consumer := "consumer-consistency-test"
			var firstToken string

			// 运行 5 次请求，验证同一个 consumer 始终获得相同的 token
			for i := 0; i < 5; i++ {
				host, status := test.NewTestHost(multiTokenOpenAIConfig)
				require.Equal(t, types.OnPluginStartStatusOK, status)

				action := host.CallOnHttpRequestHeaders([][2]string{
					{":authority", "example.com"},
					{":path", "/v1/responses"},
					{":method", "POST"},
					{"Content-Type", "application/json"},
					{"x-mse-consumer", consumer},
				})

				require.Equal(t, types.HeaderStopIteration, action)

				requestHeaders := host.GetRequestHeaders()
				authValue, hasAuth := test.GetHeaderValue(requestHeaders, "Authorization")
				require.True(t, hasAuth, "Authorization header should exist")
				require.True(t, strings.Contains(authValue, "sk-token-"), "Should use one of the configured tokens")

				if i == 0 {
					firstToken = authValue
				} else {
					require.Equal(t, firstToken, authValue, "Same consumer should get same token consistently (consumer affinity)")
				}

				host.Reset()
			}
		})

		// 测试不同 consumer 可能获得不同 token
		t.Run("different consumers get tokens based on hash", func(t *testing.T) {
			tokens := make(map[string]string)

			consumers := []string{"consumer-alpha", "consumer-beta", "consumer-gamma", "consumer-delta", "consumer-epsilon"}
			for _, consumer := range consumers {
				host, status := test.NewTestHost(multiTokenOpenAIConfig)
				require.Equal(t, types.OnPluginStartStatusOK, status)

				action := host.CallOnHttpRequestHeaders([][2]string{
					{":authority", "example.com"},
					{":path", "/v1/responses"},
					{":method", "POST"},
					{"Content-Type", "application/json"},
					{"x-mse-consumer", consumer},
				})

				require.Equal(t, types.HeaderStopIteration, action)

				requestHeaders := host.GetRequestHeaders()
				authValue, _ := test.GetHeaderValue(requestHeaders, "Authorization")
				tokens[consumer] = authValue

				host.Reset()
			}

			// 验证至少使用了多个不同的 token（hash 分布）
			uniqueTokens := make(map[string]bool)
			for _, token := range tokens {
				uniqueTokens[token] = true
			}
			require.GreaterOrEqual(t, len(uniqueTokens), 2, "Different consumers should use at least 2 different tokens")
		})
	})
}
