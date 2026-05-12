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

		t.Run("parse_override_invalid_fails", func(t *testing.T) {
			var global config.PluginConfig
			require.NoError(t, ParseGlobalConfigForTest(gjson.Parse(`{"provider":{"type":"generic","genericHost":"http://127.0.0.1:1","apiTokens":["a"]}}`), &global))

			var rule config.PluginConfig
			err := ParseOverrideRuleConfigForTest(gjson.Parse(`{"provider":{"type":"azure","apiTokens":["t"]}}`), global, &rule)
			require.Error(t, err)
			require.Contains(t, strings.ToLower(err.Error()), "azure")
		})
	})
}
