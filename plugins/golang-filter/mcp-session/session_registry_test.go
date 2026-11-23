package mcp_session

import (
	"errors"
	"net/http"
	"net/url"
	"testing"

	"github.com/alibaba/higress/plugins/golang-filter/mcp-session/common"
	"github.com/envoyproxy/envoy/contrib/golang/common/go/api"
)

type filterSessionRegistry struct {
	valid      bool
	err        error
	calls      int
	serverName string
	sessionID  string
}

func (r *filterSessionRegistry) RegisterSession(string, string) error {
	return nil
}

func (r *filterSessionRegistry) ValidateSession(serverName, sessionID string) (bool, error) {
	r.calls++
	r.serverName = serverName
	r.sessionID = sessionID
	return r.valid, r.err
}

type registryFilterCallbacks struct {
	api.FilterCallbackHandler
	decoder *registryDecoderCallbacks
}

func (c *registryFilterCallbacks) DecoderFilterCallbacks() api.DecoderFilterCallbacks {
	return c.decoder
}

type registryDecoderCallbacks struct {
	api.DecoderFilterCallbacks
	statusCode int
}

func (c *registryDecoderCallbacks) SendLocalReply(statusCode int, _ string, _ map[string][]string, _ int64, _ string) {
	c.statusCode = statusCode
}

type routeScopedSessionRegistry struct {
	sessions map[string]map[string]struct{}
}

func (r *routeScopedSessionRegistry) RegisterSession(serverName, sessionID string) error {
	if r.sessions == nil {
		r.sessions = make(map[string]map[string]struct{})
	}
	if r.sessions[serverName] == nil {
		r.sessions[serverName] = make(map[string]struct{})
	}
	r.sessions[serverName][sessionID] = struct{}{}
	return nil
}

func (r *routeScopedSessionRegistry) ValidateSession(serverName, sessionID string) (bool, error) {
	_, ok := r.sessions[serverName][sessionID]
	return ok, nil
}

func runMessagePost(t *testing.T, registry common.SessionRegistry, rule common.MatchRule) (api.StatusType, int) {
	t.Helper()
	parsedURL, err := url.Parse("http://example.com/mcp?sessionId=session-1")
	if err != nil {
		t.Fatal(err)
	}
	decoder := &registryDecoderCallbacks{}
	filter := &filter{
		callbacks:   &registryFilterCallbacks{decoder: decoder},
		config:      &config{sessionRegistry: registry},
		matchedRule: rule,
		needProcess: true,
		req:         &http.Request{Method: http.MethodPost, URL: parsedURL},
	}
	status := filter.processMcpRequestHeadersForRestUpstream(testHeaderMap{values: map[string]string{}}, false)
	return status, decoder.statusCode
}

func TestMessagePostValidatesSessionRegistry(t *testing.T) {
	api.SetCommonCAPI(&mockCommonCAPI{})
	storeErr := errors.New("registry unavailable")
	matchedRule := common.MatchRule{UpstreamType: common.RestUpstream, ServerName: "server-a"}
	tests := []struct {
		name           string
		registry       *filterSessionRegistry
		expectedStatus api.StatusType
		expectedCode   int
		expectedCalls  int
	}{
		{name: "valid session accepted", registry: &filterSessionRegistry{valid: true}, expectedStatus: api.Continue, expectedCalls: 1},
		{name: "unknown session rejected", registry: &filterSessionRegistry{}, expectedStatus: api.LocalReply, expectedCode: http.StatusForbidden, expectedCalls: 1},
		{name: "store error fails closed", registry: &filterSessionRegistry{err: storeErr}, expectedStatus: api.LocalReply, expectedCode: http.StatusServiceUnavailable, expectedCalls: 1},
		{name: "no Redis accepts session", expectedStatus: api.Continue},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsedURL, err := url.Parse("http://example.com/mcp?sessionId=session-1")
			if err != nil {
				t.Fatal(err)
			}
			decoder := &registryDecoderCallbacks{}
			var sessionRegistry common.SessionRegistry
			if tt.registry != nil {
				sessionRegistry = tt.registry
			}
			filter := &filter{
				callbacks:   &registryFilterCallbacks{decoder: decoder},
				config:      &config{sessionRegistry: sessionRegistry},
				matchedRule: matchedRule,
				needProcess: true,
				req:         &http.Request{Method: http.MethodPost, URL: parsedURL},
			}

			status := filter.processMcpRequestHeadersForRestUpstream(testHeaderMap{values: map[string]string{}}, false)

			if status != tt.expectedStatus {
				t.Fatalf("expected status %v, got %v", tt.expectedStatus, status)
			}
			if decoder.statusCode != tt.expectedCode {
				t.Fatalf("expected local reply %d, got %d", tt.expectedCode, decoder.statusCode)
			}
			calls := 0
			if tt.registry != nil {
				calls = tt.registry.calls
				if tt.registry.serverName != matchedRule.SessionServerName() || tt.registry.sessionID != "session-1" {
					t.Fatalf("unexpected validation binding %q/%q", tt.registry.serverName, tt.registry.sessionID)
				}
			}
			if calls != tt.expectedCalls {
				t.Fatalf("expected %d validation calls, got %d", tt.expectedCalls, calls)
			}
		})
	}
}

func TestSessionRegistryRejectsCrossRouteReplay(t *testing.T) {
	api.SetCommonCAPI(&mockCommonCAPI{})
	ruleA := common.MatchRule{
		ServerName:      "server-a",
		MatchRuleDomain: "a.example.com",
		MatchRulePath:   "/server-a",
		MatchRuleType:   common.PrefixMatch,
		UpstreamType:    common.RestUpstream,
	}
	ruleB := common.MatchRule{
		ServerName:      "server-b",
		MatchRuleDomain: "b.example.com",
		MatchRulePath:   "/server-b",
		MatchRuleType:   common.PrefixMatch,
		UpstreamType:    common.RestUpstream,
	}
	if ruleA.SessionServerName() == ruleB.SessionServerName() {
		t.Fatal("distinct rules must not share a session identity")
	}

	registry := &routeScopedSessionRegistry{}
	if err := registry.RegisterSession(ruleA.SessionServerName(), "session-1"); err != nil {
		t.Fatal(err)
	}

	status, code := runMessagePost(t, registry, ruleA)
	if status != api.Continue || code != 0 {
		t.Fatalf("expected session accepted on its own route, got status %v code %d", status, code)
	}

	status, code = runMessagePost(t, registry, ruleB)
	if status != api.LocalReply || code != http.StatusForbidden {
		t.Fatalf("expected cross-route replay rejected with 403, got status %v code %d", status, code)
	}
}

func TestSSERequestUsesMatchedRuleServerName(t *testing.T) {
	api.SetCommonCAPI(&mockCommonCAPI{})
	rule := common.MatchRule{
		ServerName:      "server-a",
		MatchRuleDomain: "a.example.com",
		MatchRulePath:   "/server-a",
		MatchRuleType:   common.PrefixMatch,
		UpstreamType:    common.RestUpstream,
	}
	parsedURL, err := url.Parse("http://a.example.com/server-a/sse")
	if err != nil {
		t.Fatal(err)
	}
	decoder := &registryDecoderCallbacks{}
	filter := &filter{
		callbacks:   &registryFilterCallbacks{decoder: decoder},
		config:      &config{sharedMCPServer: common.NewMCPServer("shared", "1.0.0")},
		matchedRule: rule,
		needProcess: true,
		req:         &http.Request{Method: http.MethodGet, URL: parsedURL},
	}

	status := filter.processMcpRequestHeadersForRestUpstream(testHeaderMap{values: map[string]string{}}, false)

	if status != api.LocalReply || decoder.statusCode != http.StatusOK {
		t.Fatalf("expected SSE connection accepted, got status %v code %d", status, decoder.statusCode)
	}
	if filter.sseServer == nil || filter.sseServer.GetServerName() != rule.SessionServerName() {
		t.Fatalf("expected SSE server bound to %q", rule.SessionServerName())
	}
}
