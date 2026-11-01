package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-proxy/config"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	wasmtest "github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestParseGlobalAndOverrideConfig(t *testing.T) {
	wasmtest.RunGoTest(t, func(t *testing.T) {
		bootstrap, st := wasmtest.NewTestHost(json.RawMessage(`{"provider":{"type":"generic","genericHost":"http://127.0.0.1:1","apiTokens":["bootstrap"]}}`))
		require.Equal(t, types.OnPluginStartStatusOK, st)
		defer bootstrap.Reset()

		t.Run("parse_global_empty_ok", func(t *testing.T) {
			var c config.PluginConfig
			err := ParseGlobalConfigForTest(gjson.Parse(`{}`), &c)
			require.NoError(t, err)
			require.Nil(t, c.GetProviderConfig())
		})

		t.Run("parse_global_invalid_provider", func(t *testing.T) {
			var c config.PluginConfig
			err := ParseGlobalConfigForTest(gjson.Parse(`{"provider":{"type":"not-a-real-provider","apiTokens":["x"]}}`), &c)
			require.Error(t, err)
		})

		t.Run("parse_override_switches_active_provider", func(t *testing.T) {
			globalJSON := `{"providers":[
				{"id":"p1","type":"generic","genericHost":"http://127.0.0.1:8080","apiTokens":["t"]},
				{"id":"p2","type":"generic","genericHost":"http://127.0.0.1:8081","apiTokens":["u"]}
			],"activeProviderId":"p1"}`

			var global config.PluginConfig
			require.NoError(t, ParseGlobalConfigForTest(gjson.Parse(globalJSON), &global))
			require.Equal(t, "p1", global.GetProviderConfig().GetId())

			var rule config.PluginConfig
			err := ParseOverrideRuleConfigForTest(gjson.Parse(`{"activeProviderId":"p2"}`), global, &rule)
			require.NoError(t, err)
			require.Equal(t, "p2", rule.GetProviderConfig().GetId())
		})

		t.Run("parse_override_invalid_disables_rule", func(t *testing.T) {
			var global config.PluginConfig
			require.NoError(t, ParseGlobalConfigForTest(gjson.Parse(`{"provider":{"type":"generic","genericHost":"http://127.0.0.1:1","apiTokens":["a"]}}`), &global))

			var rule config.PluginConfig
			err := ParseOverrideRuleConfigForTest(gjson.Parse(`{"provider":{"type":"azure","apiTokens":["t"]}}`), global, &rule)
			require.NoError(t, err)
			require.Nil(t, rule.GetProviderConfig())
			require.Nil(t, rule.GetProvider())
			require.True(t, rule.IsDisabled())
		})

		t.Run("invalid_reload_does_not_retain_previous_provider", func(t *testing.T) {
			var global config.PluginConfig
			require.NoError(t, ParseGlobalConfigForTest(gjson.Parse(`{}`), &global))

			var rule config.PluginConfig
			require.NoError(t, ParseOverrideRuleConfigForTest(gjson.Parse(
				`{"provider":{"type":"generic","genericHost":"old.example.com","apiTokens":["old-secret"]}}`,
			), global, &rule))
			require.NotNil(t, rule.GetProviderConfig())

			err := ParseOverrideRuleConfigForTest(gjson.Parse(
				`{"provider":{"type":"azure","azureServiceUrl":"http://1111","apiTokens":["new-secret"]}}`,
			), global, &rule)
			require.NoError(t, err)
			require.Nil(t, rule.GetProviderConfig())
			require.Nil(t, rule.GetProvider())
			require.True(t, rule.IsDisabled())
		})
	})
}

func TestInvalidProviderRuleDoesNotBlockValidProvider(t *testing.T) {
	wasmtest.RunGoTest(t, func(t *testing.T) {
		pluginJSON := json.RawMessage(`{
			"providers": [
				{
					"id": "valid",
					"type": "generic",
					"genericHost": "valid.example.com",
					"apiTokens": ["valid-token"]
				},
				{
					"id": "invalid",
					"type": "azure",
					"azureServiceUrl": "http://1111",
					"apiTokens": ["invalid-token"]
				}
			],
			"_rules_": [
				{
					"_match_route_": ["invalid-route"],
					"activeProviderId": "invalid"
				},
				{
					"_match_route_": ["valid-route"],
					"activeProviderId": "valid"
				}
			]
		}`)

		host, status := wasmtest.NewTestHost(pluginJSON)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		require.NoError(t, host.SetRouteName("valid-route"))
		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "gateway.example.com"},
			{":path", "/v1/chat/completions"},
			{":method", "POST"},
			{"Content-Type", "application/json"},
		})
		require.Equal(t, types.HeaderStopIteration, action)
		authority, ok := wasmtest.GetHeaderValue(host.GetRequestHeaders(), ":authority")
		require.True(t, ok)
		require.Equal(t, "valid.example.com", authority)

		host.CompleteHttp()
		require.NoError(t, host.SetRouteName("invalid-route"))
		action = host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "gateway.example.com"},
			{":path", "/v1/chat/completions"},
			{":method", "POST"},
		})
		require.Equal(t, types.ActionPause, action)
		localResponse := host.GetLocalResponse()
		require.NotNil(t, localResponse)
		require.Equal(t, uint32(500), localResponse.StatusCode)
		require.Equal(t, "ai-proxy.invalid_provider_config", localResponse.StatusCodeDetail)
		requireHostLogsDoNotContain(t, host, "invalid-token")
	})
}

func TestInvalidRuleLocalProviderDoesNotBlockValidProvider(t *testing.T) {
	wasmtest.RunGoTest(t, func(t *testing.T) {
		pluginJSON := json.RawMessage(`{
			"_rules_": [
				{
					"_match_route_": ["invalid-route"],
					"provider": {
						"type": "azure",
						"azureServiceUrl": "http://1111",
						"apiTokens": ["must-not-be-logged"]
					}
				},
				{
					"_match_route_": ["valid-route"],
					"provider": {
						"type": "generic",
						"genericHost": "valid-local.example.com",
						"apiTokens": ["valid-local-token"]
					}
				}
			]
		}`)

		host, status := wasmtest.NewTestHost(pluginJSON)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		require.NoError(t, host.SetRouteName("valid-route"))
		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "gateway.example.com"},
			{":path", "/v1/chat/completions"},
			{":method", "POST"},
			{"Content-Type", "application/json"},
		})
		require.Equal(t, types.HeaderStopIteration, action)
		authority, ok := wasmtest.GetHeaderValue(host.GetRequestHeaders(), ":authority")
		require.True(t, ok)
		require.Equal(t, "valid-local.example.com", authority)

		host.CompleteHttp()
		require.NoError(t, host.SetRouteName("invalid-route"))
		action = host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "gateway.example.com"},
			{":path", "/v1/chat/completions"},
			{":method", "POST"},
		})
		require.Equal(t, types.ActionPause, action)
		localResponse := host.GetLocalResponse()
		require.NotNil(t, localResponse)
		require.Equal(t, uint32(500), localResponse.StatusCode)
		require.Equal(t, "ai-proxy.invalid_provider_config", localResponse.StatusCodeDetail)
		requireHostLogsDoNotContain(t, host, "must-not-be-logged")
	})
}

func requireHostLogsDoNotContain(t *testing.T, host wasmtest.TestHost, secret string) {
	t.Helper()
	logs := append([]string{}, host.GetDebugLogs()...)
	logs = append(logs, host.GetInfoLogs()...)
	logs = append(logs, host.GetWarnLogs()...)
	logs = append(logs, host.GetErrorLogs()...)
	require.NotContains(t, strings.Join(logs, "\n"), secret)
}
