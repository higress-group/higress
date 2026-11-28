package main

import (
	"encoding/json"
	"reflect"
	"testing"

	oidc "github.com/higress-group/oauth2-proxy"
	"github.com/higress-group/oauth2-proxy/pkg/util"
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

func TestSessionCookiesPrefersPerRequestRecorder(t *testing.T) {
	rw := util.NewRecorder()
	rw.Header().Add(oidc.SetCookieHeader, "_oauth2_proxy_session=abc")
	rw.Header().Add(oidc.SetCookieHeader, "other=1")

	// A stale value left by a concurrent request on the shared handler
	// context must be ignored when the per-request recorder is available.
	got := sessionCookies(rw, []string{"_oauth2_proxy_session=stale-from-another-request"})
	want := []string{"_oauth2_proxy_session=abc", "other=1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sessionCookies() = %v, want %v", got, want)
	}

	// A recorder without cookies wins over a stale legacy value: the request
	// genuinely has no session cookies to relay.
	emptyRw := util.NewRecorder()
	if got := sessionCookies(emptyRw, []string{"stale=1"}); got != nil {
		t.Fatalf("sessionCookies() = %v, want nil when the recorder has no cookies", got)
	}
}

func TestSessionCookiesFallsBackToLegacyContext(t *testing.T) {
	if got := sessionCookies(nil, []string{"legacy=1"}); !reflect.DeepEqual(got, []string{"legacy=1"}) {
		t.Fatalf("sessionCookies() = %v, want the legacy context value", got)
	}
	if got := sessionCookies(nil, nil); got != nil {
		t.Fatalf("sessionCookies() = %v, want nil when nothing is recorded", got)
	}
}

func TestOnHttpResponseHeadersAfterAsyncWaitDoesNotPanic(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(oidcTestConfig(nil))
		defer host.Reset()
		if status != types.OnPluginStartStatusOK {
			t.Fatalf("plugin start status = %v, want %v", status, types.OnPluginStartStatusOK)
		}

		if action := host.CallOnHttpRequestHeaders([][2]string{
			{":scheme", "http"},
			{":authority", "foo.bar.com"},
			{":path", "/protected"},
			{":method", "POST"},
		}); action != types.ActionPause {
			t.Fatalf("request action = %v, want %v", action, types.ActionPause)
		}

		// The response phase must relay cookies from the per-request recorder
		// without panicking, even when no cookies were recorded.
		if action := host.CallOnHttpResponseHeaders([][2]string{{":status", "503"}}); action != types.ActionContinue {
			t.Fatalf("response action = %v, want %v", action, types.ActionContinue)
		}
	})
}
