package config

import (
	"strings"
	"testing"

	"github.com/higress-group/wasm-go/pkg/iface"
	"github.com/tidwall/gjson"
)

func TestPluginConfig_FromJsonAndValidate(t *testing.T) {
	tests := []struct {
		name      string
		json      string
		wantErr   string
		wantNilPC bool
		wantID    string
		wantType  string
	}{
		{
			name:      "legacy_single_provider_object",
			json:      `{"provider":{"type":"generic","genericHost":"http://127.0.0.1:8080","apiTokens":["t"]}}`,
			wantNilPC: false,
			wantType:  "generic",
		},
		{
			name: "providers_without_active_id_validate_ok",
			json: `{"providers":[
				{"id":"a","type":"generic","genericHost":"http://127.0.0.1:8080","apiTokens":["t"]},
				{"id":"b","type":"generic","genericHost":"http://127.0.0.1:8081","apiTokens":["u"]}
			]}`,
			wantNilPC: true,
		},
		{
			name: "providers_with_active_id",
			json: `{"providers":[
				{"id":"p1","type":"generic","genericHost":"http://127.0.0.1:8080","apiTokens":["t"]},
				{"id":"p2","type":"generic","genericHost":"http://127.0.0.1:8081","apiTokens":["u"]}
			],"activeProviderId":"p2"}`,
			wantNilPC: false,
			wantID:    "p2",
			wantType:  "generic",
		},
		{
			name: "active_id_not_found",
			json: `{"providers":[
				{"id":"p1","type":"generic","genericHost":"http://127.0.0.1:8080","apiTokens":["t"]}
			],"activeProviderId":"missing"}`,
			wantNilPC: true,
		},
		{
			name:    "invalid_protocol",
			json:    `{"providers":[{"id":"x","type":"generic","protocol":"badproto","genericHost":"http://127.0.0.1:8080","apiTokens":["t"]}],"activeProviderId":"x"}`,
			wantErr: "invalid protocol",
		},
		{
			name:    "missing_type",
			json:    `{"providers":[{"id":"x","genericHost":"http://127.0.0.1:8080","apiTokens":["t"]}],"activeProviderId":"x"}`,
			wantErr: "missing type",
		},
		{
			name:    "unknown_provider_type",
			json:    `{"providers":[{"id":"x","type":"not-a-real-provider","apiTokens":["t"]}],"activeProviderId":"x"}`,
			wantErr: "unknown provider type",
		},
		{
			name:    "initializer_validate_azure_missing_url",
			json:    `{"providers":[{"id":"x","type":"azure","apiTokens":["t"]}],"activeProviderId":"x"}`,
			wantErr: "missing azureServiceUrl",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var c PluginConfig
			c.FromJson(gjson.Parse(tt.json))
			err := c.Validate()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Validate() err = %v, want substring %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate() = %v", err)
			}
			pc := c.GetProviderConfig()
			if tt.wantNilPC {
				if pc != nil {
					t.Fatalf("GetProviderConfig() = %p, want nil", pc)
				}
			} else {
				if pc == nil {
					t.Fatal("GetProviderConfig() = nil, want non-nil")
				}
				if tt.wantID != "" && pc.GetId() != tt.wantID {
					t.Errorf("GetId() = %q, want %q", pc.GetId(), tt.wantID)
				}
				if tt.wantType != "" && pc.GetType() != tt.wantType {
					t.Errorf("GetType() = %q, want %q", pc.GetType(), tt.wantType)
				}
			}
		})
	}
}

func TestPluginConfig_OverrideMergeSimulatesParseOverride(t *testing.T) {
	globalJSON := `{"providers":[
		{"id":"p1","type":"generic","genericHost":"http://127.0.0.1:8080","apiTokens":["t"]},
		{"id":"p2","type":"generic","genericHost":"http://127.0.0.1:8081","apiTokens":["u"]}
	],"activeProviderId":"p1"}`

	t.Run("switch_active_provider_id", func(t *testing.T) {
		var global PluginConfig
		global.FromJson(gjson.Parse(globalJSON))
		if err := global.Validate(); err != nil {
			t.Fatal(err)
		}
		if global.GetProviderConfig().GetId() != "p1" {
			t.Fatalf("global active id = %q", global.GetProviderConfig().GetId())
		}

		rule := global
		rule.FromJson(gjson.Parse(`{"activeProviderId":"p2"}`))
		if err := rule.Validate(); err != nil {
			t.Fatal(err)
		}
		if got := rule.GetProviderConfig().GetId(); got != "p2" {
			t.Errorf("after override active id = %q, want p2", got)
		}
	})

	t.Run("empty_override_json_clears_active", func(t *testing.T) {
		var global PluginConfig
		global.FromJson(gjson.Parse(globalJSON))
		if err := global.Validate(); err != nil {
			t.Fatal(err)
		}

		rule := global
		rule.FromJson(gjson.Parse(`{}`))
		if err := rule.Validate(); err != nil {
			t.Fatal(err)
		}
		if rule.GetProviderConfig() != nil {
			t.Errorf("after empty override, GetProviderConfig() = %v, want nil", rule.GetProviderConfig())
		}
	})

	t.Run("clear_active_with_empty_string_id", func(t *testing.T) {
		var global PluginConfig
		global.FromJson(gjson.Parse(globalJSON))
		if err := global.Validate(); err != nil {
			t.Fatal(err)
		}

		rule := global
		rule.FromJson(gjson.Parse(`{"activeProviderId":""}`))
		if err := rule.Validate(); err != nil {
			t.Fatal(err)
		}
		if rule.GetProviderConfig() != nil {
			t.Errorf("GetProviderConfig() = %v, want nil", rule.GetProviderConfig())
		}
	})
}

func TestPluginConfig_SessionAffinity(t *testing.T) {
	t.Run("body_hash_selects_consistent_provider", func(t *testing.T) {
		var c PluginConfig
		c.FromJson(gjson.Parse(`{
			"providers": [
				{"id":"p1","type":"generic","genericHost":"http://127.0.0.1:8080","apiTokens":["t"]},
				{"id":"p2","type":"generic","genericHost":"http://127.0.0.1:8081","apiTokens":["u"]}
			],
			"sessionAffinity": {
				"enabled": true,
				"type": "hash",
				"key": {"source":"body","jsonPath":"$.callOptions.stickySessionId"}
			}
		}`))
		if err := c.Validate(); err != nil {
			t.Fatal(err)
		}
		c.providersByID = map[string]providerRuntime{
			"p1": {config: &c.providerConfigs[0]},
			"p2": {config: &c.providerConfigs[1]},
		}

		ctx := newConfigTestContext()
		body := []byte(`{"callOptions":{"stickySessionId":"session-123"}}`)
		if err := c.SelectProviderBySessionAffinity(ctx, body); err != nil {
			t.Fatal(err)
		}
		wantID, err := c.sessionAffinity.selectProviderID("session-123")
		if err != nil {
			t.Fatal(err)
		}
		if got := c.GetProviderConfig(ctx).GetId(); got != wantID {
			t.Fatalf("selected provider = %q, want %q", got, wantID)
		}
	})

	t.Run("provider_ids_limit_hash_ring", func(t *testing.T) {
		var c PluginConfig
		c.FromJson(gjson.Parse(`{
			"providers": [
				{"id":"p1","type":"generic","genericHost":"http://127.0.0.1:8080","apiTokens":["t"]},
				{"id":"p2","type":"generic","genericHost":"http://127.0.0.1:8081","apiTokens":["u"]},
				{"id":"p3","type":"generic","genericHost":"http://127.0.0.1:8082","apiTokens":["v"]}
			],
			"sessionAffinity": {
				"enabled": true,
				"providerIds": ["p2","p3"],
				"key": {"source":"header","name":"x-session"}
			}
		}`))
		if err := c.Validate(); err != nil {
			t.Fatal(err)
		}
		if strings.Join(c.sessionAffinity.providerOrder, ",") != "p2,p3" {
			t.Fatalf("providerOrder = %v, want [p2 p3]", c.sessionAffinity.providerOrder)
		}
	})

	t.Run("rejects_unknown_provider_id", func(t *testing.T) {
		var c PluginConfig
		c.FromJson(gjson.Parse(`{
			"providers": [
				{"id":"p1","type":"generic","genericHost":"http://127.0.0.1:8080","apiTokens":["t"]}
			],
			"sessionAffinity": {
				"enabled": true,
				"providerIds": ["missing"],
				"key": {"source":"header","name":"x-session"}
			}
		}`))
		err := c.Validate()
		if err == nil || !strings.Contains(err.Error(), "unknown provider") {
			t.Fatalf("Validate() err = %v, want unknown provider", err)
		}
	})

	t.Run("rejects_body_without_json_path", func(t *testing.T) {
		var c PluginConfig
		c.FromJson(gjson.Parse(`{
			"providers": [
				{"id":"p1","type":"generic","genericHost":"http://127.0.0.1:8080","apiTokens":["t"]}
			],
			"sessionAffinity": {
				"enabled": true,
				"key": {"source":"body"}
			}
		}`))
		err := c.Validate()
		if err == nil || !strings.Contains(err.Error(), "jsonPath") {
			t.Fatalf("Validate() err = %v, want jsonPath", err)
		}
	})

	t.Run("rejects_max_body_bytes_overflow", func(t *testing.T) {
		var c PluginConfig
		c.FromJson(gjson.Parse(`{
			"providers": [
				{"id":"p1","type":"generic","genericHost":"http://127.0.0.1:8080","apiTokens":["t"]}
			],
			"sessionAffinity": {
				"enabled": true,
				"max_body_bytes": 4294967296,
				"key": {"source":"body","jsonPath":"$.session"}
			}
		}`))
		err := c.Validate()
		if err == nil || !strings.Contains(err.Error(), "max_body_bytes") {
			t.Fatalf("Validate() err = %v, want max_body_bytes", err)
		}
	})

	t.Run("persistent_mode_accepts_ttl_seconds", func(t *testing.T) {
		var c PluginConfig
		c.FromJson(gjson.Parse(`{
			"providers": [
				{"id":"p1","type":"generic","genericHost":"http://127.0.0.1:8080","apiTokens":["t"]},
				{"id":"p2","type":"generic","genericHost":"http://127.0.0.1:8081","apiTokens":["u"]}
			],
			"sessionAffinity": {
				"enabled": true,
				"mode": "persistent",
				"ttlSeconds": 120,
				"key": {"source":"body","jsonPath":"$.session"}
			}
		}`))
		if err := c.Validate(); err != nil {
			t.Fatal(err)
		}
		if c.sessionAffinity.Type != sessionAffinityModePersistent {
			t.Fatalf("mode = %q, want persistent", c.sessionAffinity.Type)
		}
		if c.sessionAffinity.TTLSeconds != 120 {
			t.Fatalf("ttlSeconds = %d, want 120", c.sessionAffinity.TTLSeconds)
		}
	})

	t.Run("metadata_key_name_maps_to_metadata_property", func(t *testing.T) {
		var c PluginConfig
		c.FromJson(gjson.Parse(`{
			"providers": [
				{"id":"p1","type":"generic","genericHost":"http://127.0.0.1:8080","apiTokens":["t"]}
			],
			"sessionAffinity": {
				"enabled": true,
				"key": {"source":"metadata","name":"sticky_session_id"}
			}
		}`))
		if err := c.Validate(); err != nil {
			t.Fatal(err)
		}
		if strings.Join(c.sessionAffinity.Key.PropertyPath, ".") != "metadata.sticky_session_id" {
			t.Fatalf("property path = %v, want metadata.sticky_session_id", c.sessionAffinity.Key.PropertyPath)
		}
	})

	t.Run("scope_model_requires_request_body", func(t *testing.T) {
		var c PluginConfig
		c.FromJson(gjson.Parse(`{
			"providers": [
				{"id":"p1","type":"generic","genericHost":"http://127.0.0.1:8080","apiTokens":["t"]}
			],
			"sessionAffinity": {
				"enabled": true,
				"scope": ["model"],
				"key": {"source":"header","name":"x-session"}
			}
		}`))
		if err := c.Validate(); err != nil {
			t.Fatal(err)
		}
		if !c.NeedsSessionAffinityRequestBody() {
			t.Fatal("scope model should require request body")
		}
	})

	t.Run("on_provider_unavailable_fallback_and_update", func(t *testing.T) {
		var c PluginConfig
		c.FromJson(gjson.Parse(`{
			"providers": [
				{"id":"p1","type":"generic","genericHost":"http://127.0.0.1:8080","apiTokens":["t"]},
				{"id":"p2","type":"generic","genericHost":"http://127.0.0.1:8081","apiTokens":["u"]}
			],
			"sessionAffinity": {
				"enabled": true,
				"mode": "persistent",
				"onProviderUnavailable": "fallback_and_update",
				"unavailableStatus": ["5.*"],
				"key": {"source":"body","jsonPath":"$.session"}
			}
		}`))
		if err := c.Validate(); err != nil {
			t.Fatal(err)
		}
		if c.sessionAffinity.OnUnavailable != sessionAffinityUnavailableFallbackAndUpdate {
			t.Fatalf("onProviderUnavailable = %q, want fallbackAndUpdate", c.sessionAffinity.OnUnavailable)
		}
		if !c.NeedsSessionAffinityFallback() {
			t.Fatal("fallbackAndUpdate should require request body capture")
		}
	})

	t.Run("rejects_unknown_on_provider_unavailable", func(t *testing.T) {
		var c PluginConfig
		c.FromJson(gjson.Parse(`{
			"providers": [
				{"id":"p1","type":"generic","genericHost":"http://127.0.0.1:8080","apiTokens":["t"]}
			],
			"sessionAffinity": {
				"enabled": true,
				"onProviderUnavailable": "teleport",
				"key": {"source":"header","name":"x-session"}
			}
		}`))
		err := c.Validate()
		if err == nil || !strings.Contains(err.Error(), "onProviderUnavailable") {
			t.Fatalf("Validate() err = %v, want onProviderUnavailable", err)
		}
	})

	t.Run("rejects_fallback_timeout_overflow", func(t *testing.T) {
		var c PluginConfig
		c.FromJson(gjson.Parse(`{
			"providers": [
				{"id":"p1","type":"generic","genericHost":"http://127.0.0.1:8080","apiTokens":["t"]},
				{"id":"p2","type":"generic","genericHost":"http://127.0.0.1:8081","apiTokens":["u"]}
			],
			"sessionAffinity": {
				"enabled": true,
				"onProviderUnavailable": "fallbackWithoutUpdate",
				"fallbackTimeout": 4294967296,
				"key": {"source":"header","name":"x-session"}
			}
		}`))
		err := c.Validate()
		if err == nil || !strings.Contains(err.Error(), "fallbackTimeout") {
			t.Fatalf("Validate() err = %v, want fallbackTimeout overflow", err)
		}
	})
}

type configTestContext struct {
	contextMap map[string]interface{}
}

func newConfigTestContext() *configTestContext {
	return &configTestContext{contextMap: make(map[string]interface{})}
}

func (m *configTestContext) SetContext(key string, value interface{}) { m.contextMap[key] = value }
func (m *configTestContext) GetContext(key string) interface{}        { return m.contextMap[key] }
func (m *configTestContext) GetBoolContext(key string, def bool) bool { return def }
func (m *configTestContext) GetStringContext(key, def string) string {
	if value, ok := m.contextMap[key].(string); ok {
		return value
	}
	return def
}
func (m *configTestContext) GetByteSliceContext(key string, def []byte) []byte { return def }
func (m *configTestContext) Scheme() string                                    { return "" }
func (m *configTestContext) Host() string                                      { return "" }
func (m *configTestContext) Path() string                                      { return "" }
func (m *configTestContext) Method() string                                    { return "" }
func (m *configTestContext) GetUserAttribute(key string) interface{}           { return nil }
func (m *configTestContext) SetUserAttribute(key string, value interface{})    {}
func (m *configTestContext) SetUserAttributeMap(kvmap map[string]interface{})  {}
func (m *configTestContext) GetUserAttributeMap() map[string]interface{}       { return nil }
func (m *configTestContext) WriteUserAttributeToLog() error                    { return nil }
func (m *configTestContext) WriteUserAttributeToLogWithKey(key string) error   { return nil }
func (m *configTestContext) WriteUserAttributeToTrace() error                  { return nil }
func (m *configTestContext) DontReadRequestBody()                              {}
func (m *configTestContext) DontReadResponseBody()                             {}
func (m *configTestContext) BufferRequestBody()                                {}
func (m *configTestContext) BufferResponseBody()                               {}
func (m *configTestContext) NeedPauseStreamingResponse()                       {}
func (m *configTestContext) PushBuffer(buffer []byte)                          {}
func (m *configTestContext) PopBuffer() []byte                                 { return nil }
func (m *configTestContext) BufferQueueSize() int                              { return 0 }
func (m *configTestContext) DisableReroute()                                   {}
func (m *configTestContext) SetRequestBodyBufferLimit(byteSize uint32)         {}
func (m *configTestContext) SetResponseBodyBufferLimit(byteSize uint32)        {}
func (m *configTestContext) RouteCall(method, url string, headers [][2]string, body []byte, callback iface.RouteResponseCallback) error {
	return nil
}
func (m *configTestContext) GetExecutionPhase() iface.HTTPExecutionPhase { return 0 }
func (m *configTestContext) HasRequestBody() bool                        { return false }
func (m *configTestContext) HasResponseBody() bool                       { return false }
func (m *configTestContext) IsWebsocket() bool                           { return false }
func (m *configTestContext) IsBinaryRequestBody() bool                   { return false }
func (m *configTestContext) IsBinaryResponseBody() bool                  { return false }
