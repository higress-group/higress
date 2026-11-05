// Copyright (c) 2025 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"encoding/json"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"

	"hmac-auth-apisix/config"

	"github.com/tidwall/gjson"
)

// This file pins down the whole attachment/authorization matrix:
// global_auth (unset/true/false) x override rule (matched/not matched/absent) x
// allow list (present/empty/absent).
//
// The security-relevant cells are the ones where an override rule attaches the
// plugin to a domain/route but carries no allow list. Those used to be treated
// as "plugin not configured here" and were let through without any signature
// verification; they must now be rejected with 401.

const (
	matrixRoute      = "r1"
	matrixOtherRoute = "some-other-route"
	matrixDomain     = "api.example.com"
)

func matrixConsumers() []map[string]interface{} {
	return []map[string]interface{}{
		{"name": "c1", "access_key": "ak1", "secret_key": "sk1"},
		{"name": "c2", "access_key": "ak2", "secret_key": "sk2"},
	}
}

// ruleWithoutAllow attaches the plugin to a route without listing any consumer.
func ruleWithoutAllow(route string) []map[string]interface{} {
	return []map[string]interface{}{{"_match_route_": []string{route}}}
}

func ruleWithAllow(route string, allow ...string) []map[string]interface{} {
	return []map[string]interface{}{{"_match_route_": []string{route}, "allow": allow}}
}

// matrixAuthHeader signs (GET /p) for the given consumer key pair and returns
// the Authorization and Date headers to send.
func matrixAuthHeader(ak, sk string) (string, string) {
	date := gmt()
	return authHeaderRequestTargetDate(ak, sk, "hmac-sha256", "GET", "/p", date), date
}

func TestAllowListMatrix(t *testing.T) {
	tests := []struct {
		name string
		// consumers is nil for configurations that only carry rules.
		consumers  []map[string]interface{}
		extra      map[string]interface{}
		routeName  string
		authority  string
		credential string // "c1", "c2", "bogus" or "" for no Authorization header
		// expectStatus of 0 means the request must not be answered locally.
		expectStatus   uint32
		expectBody     string
		expectConsumer string
	}{
		// === global_auth unset ============================================
		{
			name:           "global_auth unset, no rules, valid credentials -> authorized",
			consumers:      matrixConsumers(),
			extra:          map[string]interface{}{},
			authority:      "e.com",
			credential:     "c1",
			expectConsumer: "c1",
		},
		{
			name:         "global_auth unset, no rules, no credentials -> 401",
			consumers:    matrixConsumers(),
			extra:        map[string]interface{}{},
			authority:    "e.com",
			expectStatus: 401,
			expectBody:   "missing Authorization header",
		},
		{
			name:      "global_auth unset, matched rule with allow, listed consumer -> authorized",
			consumers: matrixConsumers(),
			extra: map[string]interface{}{
				"_rules_": ruleWithAllow(matrixRoute, "c1"),
			},
			routeName:      matrixRoute,
			authority:      "e.com",
			credential:     "c1",
			expectConsumer: "c1",
		},
		{
			name:      "global_auth unset, matched rule with allow, unlisted consumer -> 401",
			consumers: matrixConsumers(),
			extra: map[string]interface{}{
				"_rules_": ruleWithAllow(matrixRoute, "c1"),
			},
			routeName:    matrixRoute,
			authority:    "e.com",
			credential:   "c2",
			expectStatus: 401,
			expectBody:   "consumer 'c2' is not allowed",
		},
		{
			// FIXED: used to bypass authentication entirely.
			name:      "global_auth unset, matched rule without allow, valid credentials -> 401",
			consumers: matrixConsumers(),
			extra: map[string]interface{}{
				"_rules_": ruleWithoutAllow(matrixRoute),
			},
			routeName:    matrixRoute,
			authority:    "e.com",
			credential:   "c1",
			expectStatus: 401,
			expectBody:   "no consumer is allowed",
		},
		{
			// FIXED: an explicitly empty allow list denies every consumer too.
			name:      "global_auth unset, matched rule with empty allow, valid credentials -> 401",
			consumers: matrixConsumers(),
			extra: map[string]interface{}{
				"_rules_": ruleWithAllow(matrixRoute),
			},
			routeName:    matrixRoute,
			authority:    "e.com",
			credential:   "c1",
			expectStatus: 401,
			expectBody:   "no consumer is allowed",
		},
		{
			// FIXED: the rejection happens before credential parsing, so an
			// unauthenticated request is denied as well.
			name:      "global_auth unset, matched rule without allow, no credentials -> 401",
			consumers: matrixConsumers(),
			extra: map[string]interface{}{
				"_rules_": ruleWithoutAllow(matrixRoute),
			},
			routeName:    matrixRoute,
			authority:    "e.com",
			expectStatus: 401,
			expectBody:   "no consumer is allowed",
		},
		{
			// FIXED: anonymous_consumer must not bypass an attached rule.
			name:      "global_auth unset, matched rule without allow, anonymous consumer -> 401",
			consumers: matrixConsumers(),
			extra: map[string]interface{}{
				"anonymous_consumer": "guest",
				"_rules_":            ruleWithoutAllow(matrixRoute),
			},
			routeName:    matrixRoute,
			authority:    "e.com",
			expectStatus: 401,
			expectBody:   "no consumer is allowed",
		},
		{
			// FIXED: same for a rule matched by domain instead of route.
			name:      "global_auth unset, matched domain rule without allow -> 401",
			consumers: matrixConsumers(),
			extra: map[string]interface{}{
				"_rules_": []map[string]interface{}{{"_match_domain_": []string{matrixDomain}}},
			},
			authority:    matrixDomain,
			credential:   "c1",
			expectStatus: 401,
			expectBody:   "no consumer is allowed",
		},
		{
			// Legacy compatibility: with global_auth unset, a request that matches
			// no rule falls back to the instance-level config and is authenticated.
			name:      "global_auth unset, unmatched rule, no credentials -> 401",
			consumers: matrixConsumers(),
			extra: map[string]interface{}{
				"_rules_": ruleWithAllow(matrixRoute, "c1"),
			},
			routeName:    matrixOtherRoute,
			authority:    "e.com",
			expectStatus: 401,
			expectBody:   "missing Authorization header",
		},

		// === global_auth: false ===========================================
		{
			name:      "global_auth false, no rule matched, no credentials -> passed through",
			consumers: matrixConsumers(),
			extra: map[string]interface{}{
				"global_auth": false,
				"_rules_":     ruleWithAllow(matrixRoute, "c1"),
			},
			routeName: matrixOtherRoute,
			authority: "e.com",
		},
		{
			name:      "global_auth false, no rule matched, valid credentials -> passed through",
			consumers: matrixConsumers(),
			extra: map[string]interface{}{
				"global_auth": false,
				"_rules_":     ruleWithAllow(matrixRoute, "c1"),
			},
			routeName: matrixOtherRoute,
			authority: "e.com",
			// The plugin is not attached to this route, so it must not even
			// identify the caller.
			credential: "c1",
		},
		{
			name:      "global_auth false, matched rule with allow, listed consumer -> authorized",
			consumers: matrixConsumers(),
			extra: map[string]interface{}{
				"global_auth": false,
				"_rules_":     ruleWithAllow(matrixRoute, "c1"),
			},
			routeName:      matrixRoute,
			authority:      "e.com",
			credential:     "c1",
			expectConsumer: "c1",
		},
		{
			name:      "global_auth false, matched rule with allow, unlisted consumer -> 401",
			consumers: matrixConsumers(),
			extra: map[string]interface{}{
				"global_auth": false,
				"_rules_":     ruleWithAllow(matrixRoute, "c1"),
			},
			routeName:    matrixRoute,
			authority:    "e.com",
			credential:   "c2",
			expectStatus: 401,
			expectBody:   "consumer 'c2' is not allowed",
		},
		{
			// FIXED: attaching the plugin to a route without listing any consumer
			// used to disable authentication on that route.
			name:      "global_auth false, matched rule without allow, valid credentials -> 401",
			consumers: matrixConsumers(),
			extra: map[string]interface{}{
				"global_auth": false,
				"_rules_":     ruleWithoutAllow(matrixRoute),
			},
			routeName:    matrixRoute,
			authority:    "e.com",
			credential:   "c1",
			expectStatus: 401,
			expectBody:   "no consumer is allowed",
		},
		{
			name:      "global_auth false, no rules at all, no credentials -> passed through",
			consumers: matrixConsumers(),
			extra: map[string]interface{}{
				"global_auth": false,
			},
			authority: "e.com",
		},

		// === global_auth: true ============================================
		{
			name:      "global_auth true, no rules, valid credentials -> authorized",
			consumers: matrixConsumers(),
			extra: map[string]interface{}{
				"global_auth": true,
			},
			authority:      "e.com",
			credential:     "c2",
			expectConsumer: "c2",
		},
		{
			// Authentication stays global, so a matched rule without allow only
			// means "no extra restriction": the signature is still verified.
			name:      "global_auth true, matched rule without allow, valid credentials -> authorized",
			consumers: matrixConsumers(),
			extra: map[string]interface{}{
				"global_auth": true,
				"_rules_":     ruleWithoutAllow(matrixRoute),
			},
			routeName:      matrixRoute,
			authority:      "e.com",
			credential:     "c1",
			expectConsumer: "c1",
		},
		{
			name:      "global_auth true, matched rule without allow, no credentials -> 401",
			consumers: matrixConsumers(),
			extra: map[string]interface{}{
				"global_auth": true,
				"_rules_":     ruleWithoutAllow(matrixRoute),
			},
			routeName:    matrixRoute,
			authority:    "e.com",
			expectStatus: 401,
			expectBody:   "missing Authorization header",
		},
		{
			name:      "global_auth true, matched rule without allow, forged credentials -> 401",
			consumers: matrixConsumers(),
			extra: map[string]interface{}{
				"global_auth": true,
				"_rules_":     ruleWithoutAllow(matrixRoute),
			},
			routeName:    matrixRoute,
			authority:    "e.com",
			credential:   "bogus",
			expectStatus: 401,
			expectBody:   "Invalid signature",
		},
		{
			name:      "global_auth true, matched rule with allow, unlisted consumer -> 401",
			consumers: matrixConsumers(),
			extra: map[string]interface{}{
				"global_auth": true,
				"_rules_":     ruleWithAllow(matrixRoute, "c2"),
			},
			routeName:    matrixRoute,
			authority:    "e.com",
			credential:   "c1",
			expectStatus: 401,
			expectBody:   "consumer 'c1' is not allowed",
		},

		// === rules only, no instance-level configuration ==================
		{
			// FIXED: the plugin is attached through the rule only.
			name: "rules only, matched rule without allow -> 401",
			extra: map[string]interface{}{
				"_rules_": ruleWithoutAllow(matrixRoute),
			},
			routeName:    matrixRoute,
			authority:    "e.com",
			credential:   "c1",
			expectStatus: 401,
			expectBody:   "no consumer is allowed",
		},
		{
			// Nothing is configured for this request: the plugin stays inert.
			name: "rules only, no rule matched -> passed through",
			extra: map[string]interface{}{
				"_rules_": ruleWithoutAllow(matrixRoute),
			},
			routeName: matrixOtherRoute,
			authority: "e.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			consumers := tt.consumers
			if consumers == nil {
				consumers = []map[string]interface{}{}
			}
			var config json.RawMessage = createConfig(consumers, tt.extra)

			test.RunTest(t, func(t *testing.T) {
				host, status := test.NewTestHost(config)
				defer host.Reset()
				require.Equal(t, types.OnPluginStartStatusOK, status)

				if tt.routeName != "" {
					require.NoError(t, host.SetRouteName(tt.routeName))
				}

				headers := [][2]string{
					{":authority", tt.authority},
					{":path", "/p"},
					{":method", "GET"},
				}
				switch tt.credential {
				case "c1":
					auth, date := matrixAuthHeader("ak1", "sk1")
					headers = append(headers, [2]string{"authorization", auth}, [2]string{"date", date})
				case "c2":
					auth, date := matrixAuthHeader("ak2", "sk2")
					headers = append(headers, [2]string{"authorization", auth}, [2]string{"date", date})
				case "bogus":
					headers = append(headers,
						[2]string{"authorization", `Signature keyId="ak1",algorithm="hmac-sha256",signature="Zm9yZ2Vk",headers="@request-target date"`},
						[2]string{"date", gmt()})
				}

				action := host.CallOnHttpRequestHeaders(headers)
				require.Equal(t, types.ActionContinue, action)

				localResponse := host.GetLocalResponse()
				if tt.expectStatus == 0 {
					require.Nil(t, localResponse, "request should not be rejected")
				} else {
					require.NotNil(t, localResponse)
					require.Equal(t, tt.expectStatus, localResponse.StatusCode)
					require.Contains(t, string(localResponse.Data), tt.expectBody)
				}

				consumer, hasConsumer := findHeader(host.GetRequestHeaders(), consumerHeader)
				if tt.expectConsumer == "" {
					require.False(t, hasConsumer, "no consumer should be attached to the request")
				} else {
					require.True(t, hasConsumer)
					require.Equal(t, tt.expectConsumer, consumer)
				}

				host.CompleteHttp()
			})
		})
	}
}

// TestParseGlobalConfigIgnoresAllow pins the invariant that ParseGlobalConfig
// never parses a global "allow" key. The fail-closed matrix in this file relies
// on it: Allow is non-empty only when a rule matched (RuleSet=true), so
// "attached rule with empty allow" is the only path where noAllow && ruleSet
// both hold. If global allow parsing is ever added, that coupling silently
// changes behavior — this test exists to make the change loud.
func TestParseGlobalConfigIgnoresAllow(t *testing.T) {
	var cfg config.HmacAuthConfig
	if err := config.ParseGlobalConfig(gjson.Parse(`{"allow":["someone"],"consumers":[{"name":"c1","access_key":"k1","secret_key":"s1"}]}`), &cfg); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(cfg.Allow) != 0 {
		t.Fatalf("global parser must ignore allow, got %v", cfg.Allow)
	}
	if cfg.RuleSet {
		t.Fatal("global config must not set RuleSet")
	}
}
