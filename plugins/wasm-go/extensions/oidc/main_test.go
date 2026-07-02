package main

import (
	"encoding/json"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
)

func oidcTestConfig(matchList []map[string]interface{}) json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"redirect_url":      "http://foo.bar.com/oauth2/callback",
		"oidc_issuer_url":   "http://127.0.0.1:65535/realms/poc",
		"client_id":         "poc",
		"client_secret":     "poc",
		"cookie_secret":     "nqavJrGvRmQxWwGNptLdyUVKcBNZ2b18Guc1n_8DCfY=",
		"service_name":      "keycloak.static",
		"service_port":      80,
		"service_host":      "127.0.0.1:65535",
		"match_type":        "whitelist",
		"match_list":        matchList,
		"verifier_interval": "2s",
	})
	return data
}

func TestOnHttpRequestHeadersVerifierUnavailable(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(oidcTestConfig(nil))
		defer host.Reset()
		if status != types.OnPluginStartStatusOK {
			t.Fatalf("plugin start status = %v, want %v", status, types.OnPluginStartStatusOK)
		}

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":scheme", "http"},
			{":authority", "foo.bar.com"},
			{":path", "/protected"},
			{":method", "GET"},
		})

		if action != types.ActionPause {
			t.Fatalf("request action = %v, want %v", action, types.ActionPause)
		}
		if streamAction := host.GetHttpStreamAction(); streamAction != types.ActionPause {
			t.Fatalf("stream action = %v, want %v", streamAction, types.ActionPause)
		}
		localResponse := host.GetLocalResponse()
		if localResponse == nil {
			t.Fatal("local response is nil")
		}
		if localResponse.StatusCode != 503 {
			t.Fatalf("local response status = %d, want 503", localResponse.StatusCode)
		}
		if body := string(localResponse.Data); body != "OIDC verifier is unavailable" {
			t.Fatalf("local response body = %q, want %q", body, "OIDC verifier is unavailable")
		}
	})
}

func TestOnHttpRequestHeadersAllowlistBypassesVerifierCheck(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(oidcTestConfig([]map[string]interface{}{
			{
				"match_rule_domain": "foo.bar.com",
				"match_rule_path":   "/public",
				"match_rule_type":   "prefix",
			},
		}))
		defer host.Reset()
		if status != types.OnPluginStartStatusOK {
			t.Fatalf("plugin start status = %v, want %v", status, types.OnPluginStartStatusOK)
		}

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":scheme", "http"},
			{":authority", "foo.bar.com"},
			{":path", "/public/info"},
			{":method", "GET"},
		})

		if action != types.ActionContinue {
			t.Fatalf("request action = %v, want %v", action, types.ActionContinue)
		}
		if streamAction := host.GetHttpStreamAction(); streamAction != types.ActionContinue {
			t.Fatalf("stream action = %v, want %v", streamAction, types.ActionContinue)
		}
		if localResponse := host.GetLocalResponse(); localResponse != nil {
			t.Fatalf("local response = %+v, want nil", localResponse)
		}
	})
}
