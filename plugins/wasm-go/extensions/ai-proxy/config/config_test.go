package config

import (
	"strings"
	"testing"

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
