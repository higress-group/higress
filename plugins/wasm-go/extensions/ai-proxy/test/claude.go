package test

import (
	"encoding/json"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

// Claude standard mode config
var claudeStandardConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":      "claude",
			"apiTokens": []string{"sk-ant-api-key-123"},
		},
	})
	return data
}()

// Claude Code mode config
var claudeCodeModeConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":           "claude",
			"apiTokens":      []string{"sk-ant-oat01-oauth-token-456"},
			"claudeCodeMode": true,
		},
	})
	return data
}()

// Claude Code mode config with custom apiVersion
var claudeCodeModeWithVersionConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":           "claude",
			"apiTokens":      []string{"sk-ant-oat01-oauth-token-789"},
			"claudeCodeMode": true,
			"claudeVersion":  "2024-01-01",
		},
	})
	return data
}()

// Claude config without token (should fail validation)
var claudeNoTokenConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type": "claude",
		},
	})
	return data
}()

func RunClaudeParseConfigTests(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		t.Run("claude standard config", func(t *testing.T) {
			host, status := test.NewTestHost(claudeStandardConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		t.Run("claude code mode config", func(t *testing.T) {
			host, status := test.NewTestHost(claudeCodeModeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		t.Run("claude config without token fails", func(t *testing.T) {
			host, status := test.NewTestHost(claudeNoTokenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})
	})
}

func RunClaudeOnHttpRequestHeadersTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("claude standard mode uses x-api-key", func(t *testing.T) {
			host, status := test.NewTestHost(claudeStandardConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "api.anthropic.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			requestHeaders := host.GetRequestHeaders()
			require.True(t, test.HasHeaderWithValue(requestHeaders, "x-api-key", "sk-ant-api-key-123"))
			require.True(t, test.HasHeaderWithValue(requestHeaders, "anthropic-version", "2023-06-01"))

			// Should NOT have Claude Code specific headers
			_, hasAuth := test.GetHeaderValue(requestHeaders, "authorization")
			require.False(t, hasAuth, "standard mode should not have authorization header")

			_, hasXApp := test.GetHeaderValue(requestHeaders, "x-app")
			require.False(t, hasXApp, "standard mode should not have x-app header")
		})

		t.Run("claude code mode uses bearer authorization", func(t *testing.T) {
			host, status := test.NewTestHost(claudeCodeModeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "api.anthropic.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			requestHeaders := host.GetRequestHeaders()

			// Claude Code mode should use Bearer authorization
			require.True(t, test.HasHeaderWithValue(requestHeaders, "authorization", "Bearer sk-ant-oat01-oauth-token-456"))

			// Should NOT have x-api-key in Claude Code mode
			_, hasXApiKey := test.GetHeaderValue(requestHeaders, "x-api-key")
			require.False(t, hasXApiKey, "claude code mode should not have x-api-key header")

			// Should have Claude Code specific headers
			require.True(t, test.HasHeaderWithValue(requestHeaders, "x-app", "cli"))
			require.True(t, test.HasHeaderWithValue(requestHeaders, "user-agent", "claude-cli/2.1.2 (external, cli)"))
			require.True(t, test.HasHeaderWithValue(requestHeaders, "anthropic-beta", "oauth-2025-04-20,interleaved-thinking-2025-05-14,claude-code-20250219"))
			require.True(t, test.HasHeaderWithValue(requestHeaders, "anthropic-version", "2023-06-01"))
		})

		t.Run("claude code mode adds beta query param", func(t *testing.T) {
			host, status := test.NewTestHost(claudeCodeModeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "api.anthropic.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			requestHeaders := host.GetRequestHeaders()
			path, found := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, found)
			require.Contains(t, path, "beta=true", "claude code mode should add beta=true query param")
		})

		t.Run("claude code mode with custom version", func(t *testing.T) {
			host, status := test.NewTestHost(claudeCodeModeWithVersionConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "api.anthropic.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			requestHeaders := host.GetRequestHeaders()
			require.True(t, test.HasHeaderWithValue(requestHeaders, "anthropic-version", "2024-01-01"))
		})
	})
}

func RunClaudeOnHttpRequestBodyTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("claude standard mode does not inject defaults", func(t *testing.T) {
			host, status := test.NewTestHost(claudeStandardConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "api.anthropic.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			body := `{
				"model": "claude-sonnet-4-5-20250929",
				"max_tokens": 8192,
				"stream": true,
				"messages": [
					{"role": "user", "content": "Hello"}
				]
			}`
			action := host.CallOnHttpRequestBody([]byte(body))
			require.Equal(t, types.ActionContinue, action)

			processedBody := host.GetRequestBody()
			var request map[string]interface{}
			err := json.Unmarshal(processedBody, &request)
			require.NoError(t, err)

			// Standard mode should NOT inject system prompt or tools
			_, hasSystem := request["system"]
			require.False(t, hasSystem, "standard mode should not inject system prompt")

			tools, hasTools := request["tools"]
			if hasTools {
				toolsArr, ok := tools.([]interface{})
				require.True(t, ok)
				require.Empty(t, toolsArr, "standard mode should not inject tools")
			}
		})

		t.Run("claude code mode injects default system prompt", func(t *testing.T) {
			host, status := test.NewTestHost(claudeCodeModeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "api.anthropic.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			body := `{
				"model": "claude-sonnet-4-5-20250929",
				"max_tokens": 8192,
				"stream": true,
				"messages": [
					{"role": "user", "content": "List files"}
				]
			}`
			action := host.CallOnHttpRequestBody([]byte(body))
			require.Equal(t, types.ActionContinue, action)

			processedBody := host.GetRequestBody()
			var request map[string]interface{}
			err := json.Unmarshal(processedBody, &request)
			require.NoError(t, err)

			// Claude Code mode should inject system prompt
			system, hasSystem := request["system"]
			require.True(t, hasSystem, "claude code mode should inject system prompt")

			systemArr, ok := system.([]interface{})
			require.True(t, ok, "system should be an array in claude code mode")
			require.Len(t, systemArr, 1)

			systemBlock, ok := systemArr[0].(map[string]interface{})
			require.True(t, ok)
			require.Equal(t, "text", systemBlock["type"])
			require.Equal(t, "You are Claude Code, Anthropic's official CLI for Claude.", systemBlock["text"])

			// Should have cache_control
			cacheControl, hasCacheControl := systemBlock["cache_control"]
			require.True(t, hasCacheControl, "system prompt should have cache_control")
			cacheControlMap, ok := cacheControl.(map[string]interface{})
			require.True(t, ok)
			require.Equal(t, "ephemeral", cacheControlMap["type"])
		})

		t.Run("claude code mode preserves existing system prompt", func(t *testing.T) {
			host, status := test.NewTestHost(claudeCodeModeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "api.anthropic.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			body := `{
				"model": "claude-sonnet-4-5-20250929",
				"max_tokens": 8192,
				"messages": [
					{"role": "system", "content": "You are a custom assistant."},
					{"role": "user", "content": "Hello"}
				]
			}`
			action := host.CallOnHttpRequestBody([]byte(body))
			require.Equal(t, types.ActionContinue, action)

			processedBody := host.GetRequestBody()
			var request map[string]interface{}
			err := json.Unmarshal(processedBody, &request)
			require.NoError(t, err)

			// Should preserve custom system prompt (not default)
			system, hasSystem := request["system"]
			require.True(t, hasSystem)

			systemArr, ok := system.([]interface{})
			require.True(t, ok)
			require.Len(t, systemArr, 1)

			systemBlock, ok := systemArr[0].(map[string]interface{})
			require.True(t, ok)
			require.Equal(t, "You are a custom assistant.", systemBlock["text"])
		})
	})
}

// Note: Response headers tests are skipped as they require complex mocking
// The response header transformation is covered by integration tests
