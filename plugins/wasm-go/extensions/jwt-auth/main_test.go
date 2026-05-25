// Copyright (c) 2023 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0

package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"testing"
	"time"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/jwt-auth/config"
	jose "github.com/go-jose/go-jose/v3"
	"github.com/go-jose/go-jose/v3/jwt"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

// The mock host lowercases all header keys written through
// AddHttpRequestHeader, so test assertions must compare against the
// lowercase form. Local-response headers (SendHttpResponseWithDetail)
// are stored verbatim however.

// signES256 returns (jwks json, valid signed token) for the given (kid, issuer).
func signES256(t *testing.T, kid, issuer string) (string, string) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	sig, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.ES256, Key: priv},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader(jose.HeaderKey("kid"), kid),
	)
	require.NoError(t, err)

	tok, err := jwt.Signed(sig).Claims(jwt.Claims{
		Issuer:    issuer,
		Subject:   "sub",
		Audience:  jwt.Audience{"foo"},
		Expiry:    jwt.NewNumericDate(time.Now().Add(time.Hour)),
		NotBefore: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
	}).CompactSerialize()
	require.NoError(t, err)

	jwks := jose.JSONWebKeySet{Keys: []jose.JSONWebKey{
		{Key: &priv.PublicKey, KeyID: kid, Algorithm: string(jose.ES256)},
	}}
	raw, err := json.Marshal(jwks)
	require.NoError(t, err)
	return string(raw), tok
}

func signExpiredES256(t *testing.T, kid, issuer string) (string, string) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	sig, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.ES256, Key: priv},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader(jose.HeaderKey("kid"), kid),
	)
	require.NoError(t, err)
	tok, err := jwt.Signed(sig).Claims(jwt.Claims{
		Issuer:   issuer,
		Subject:  "sub",
		Audience: jwt.Audience{"foo"},
		Expiry:   jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
	}).CompactSerialize()
	require.NoError(t, err)
	jwks := jose.JSONWebKeySet{Keys: []jose.JSONWebKey{
		{Key: &priv.PublicKey, KeyID: kid, Algorithm: string(jose.ES256)},
	}}
	raw, err := json.Marshal(jwks)
	require.NoError(t, err)
	return string(raw), tok
}

func jsonConfig(t *testing.T, consumers []map[string]any, opts map[string]any) []byte {
	t.Helper()
	full := map[string]any{"consumers": consumers}
	for k, v := range opts {
		full[k] = v
	}
	b, err := json.Marshal(full)
	require.NoError(t, err)
	return b
}

func consumerEntry(name, jwks, issuer string) map[string]any {
	return map[string]any{
		"name":   name,
		"issuer": issuer,
		"jwks":   jwks,
	}
}

func headerValue(headers [][2]string, key string) string {
	for _, h := range headers {
		if h[0] == key {
			return h[1]
		}
	}
	return ""
}

func TestPlugin_ValidToken_GlobalAuthTrue(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		jwks, tok := signES256(t, "k1", "iss")
		cfg := jsonConfig(t,
			[]map[string]any{consumerEntry("c1", jwks, "iss")},
			map[string]any{"global_auth": true},
		)
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"},
			{":path", "/p"},
			{"authorization", "Bearer " + tok},
		})
		require.Equal(t, types.ActionContinue, action)
		require.Equal(t, "c1", headerValue(host.GetRequestHeaders(), "x-mse-consumer"))
	})
}

func TestPlugin_MissingToken_AllowSingleConsumer_Returns401(t *testing.T) {
	// With a rule-level allow of length 1 the handler reports the per-consumer
	// error (here: missing token → 401 token_missing) rather than the 403 fallback.
	test.RunGoTest(t, func(t *testing.T) {
		jwks, _ := signES256(t, "k1", "iss")
		cfg := jsonConfig(t,
			[]map[string]any{consumerEntry("c1", jwks, "iss")},
			map[string]any{
				"global_auth": true,
				"_rules_": []map[string]any{{
					"_match_route_": []string{"r1"},
					"allow":         []string{"c1"},
				}},
			},
		)
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		require.NoError(t, host.SetRouteName("r1"))

		_ = host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"}, {":path", "/p"},
		})
		resp := host.GetLocalResponse()
		require.NotNil(t, resp)
		require.Equal(t, uint32(401), resp.StatusCode)
		require.Equal(t, "jwt-auth.token_missing", resp.StatusCodeDetail)
		require.Equal(t, "JWT realm=MSE Gateway", headerValue(resp.Headers, "WWW-Authenticate"))
	})
}

func TestPlugin_MissingToken_NoAllow_Returns403Fallback(t *testing.T) {
	// global_auth=true with no allow list and no consumer succeeds →
	// fallback to deniedNotAllow (403).
	test.RunGoTest(t, func(t *testing.T) {
		jwks, _ := signES256(t, "k1", "iss")
		cfg := jsonConfig(t,
			[]map[string]any{consumerEntry("c1", jwks, "iss")},
			map[string]any{"global_auth": true},
		)
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		_ = host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"}, {":path", "/p"},
		})
		resp := host.GetLocalResponse()
		require.NotNil(t, resp)
		require.Equal(t, uint32(403), resp.StatusCode)
		require.Equal(t, "jwt-auth.not_allowed_by_default", resp.StatusCodeDetail)
	})
}

func TestPlugin_ExpiredToken_AllowSingleConsumer_Returns401Expired(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		jwks, tok := signExpiredES256(t, "k1", "iss")
		cfg := jsonConfig(t,
			[]map[string]any{consumerEntry("c1", jwks, "iss")},
			map[string]any{
				"global_auth": true,
				"_rules_": []map[string]any{{
					"_match_route_": []string{"r1"},
					"allow":         []string{"c1"},
				}},
			},
		)
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		require.NoError(t, host.SetRouteName("r1"))

		_ = host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"},
			{":path", "/p"},
			{"authorization", "Bearer " + tok},
		})
		resp := host.GetLocalResponse()
		require.NotNil(t, resp)
		require.Equal(t, uint32(401), resp.StatusCode)
		require.Equal(t, "jwt-auth.token_expired", resp.StatusCodeDetail)
	})
}

func TestPlugin_MultiConsumer_GlobalAuth_PicksMatchingKey(t *testing.T) {
	// Two consumers with the same issuer but different signing keys.
	// Token is signed by k2 → only c2 verifies. The handler returns the
	// X-Mse-Consumer of the matching consumer regardless of iteration order.
	test.RunGoTest(t, func(t *testing.T) {
		jwks1, _ := signES256(t, "k1", "iss")
		jwks2, tok2 := signES256(t, "k2", "iss")
		cfg := jsonConfig(t,
			[]map[string]any{
				consumerEntry("c1", jwks1, "iss"),
				consumerEntry("c2", jwks2, "iss"),
			},
			map[string]any{"global_auth": true},
		)
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"},
			{":path", "/p"},
			{"authorization", "Bearer " + tok2},
		})
		require.Equal(t, types.ActionContinue, action)
		require.Equal(t, "c2", headerValue(host.GetRequestHeaders(), "x-mse-consumer"))
	})
}

func TestPlugin_TokenFromCustomHeader(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		jwks, tok := signES256(t, "k1", "iss")
		consumer := consumerEntry("c1", jwks, "iss")
		consumer["from_headers"] = []map[string]any{{"name": "X-JWT", "value_prefix": ""}}
		cfg := jsonConfig(t,
			[]map[string]any{consumer},
			map[string]any{"global_auth": true},
		)
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"},
			{":path", "/p"},
			{"x-jwt", tok},
		})
		require.Equal(t, types.ActionContinue, action)
		require.Equal(t, "c1", headerValue(host.GetRequestHeaders(), "x-mse-consumer"))
	})
}

func TestPlugin_TokenFromQueryParam(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		jwks, tok := signES256(t, "k1", "iss")
		consumer := consumerEntry("c1", jwks, "iss")
		consumer["from_params"] = []string{"jwt"}
		cfg := jsonConfig(t,
			[]map[string]any{consumer},
			map[string]any{"global_auth": true},
		)
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"},
			{":path", "/p?jwt=" + tok},
		}))
		require.Equal(t, "c1", headerValue(host.GetRequestHeaders(), "x-mse-consumer"))
	})
}

func TestPlugin_ClaimsToHeaders(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		jwks, tok := signES256(t, "k1", "iss")
		consumer := consumerEntry("c1", jwks, "iss")
		consumer["claims_to_headers"] = []map[string]any{
			{"claim": "sub", "header": "X-User"},
		}
		cfg := jsonConfig(t,
			[]map[string]any{consumer},
			map[string]any{"global_auth": true},
		)
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"},
			{":path", "/p"},
			{"authorization", "Bearer " + tok},
		}))
		require.Equal(t, "sub", headerValue(host.GetRequestHeaders(), "x-user"))
	})
}

func TestPlugin_ConfigError_NoConsumers(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost([]byte(`{}`))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusFailed, status)
	})
}

func TestPlugin_InvalidSignature_AllowSingleConsumer_Returns401(t *testing.T) {
	// Token signed by a key not in the consumer's JWKs → verification fails →
	// per-consumer action surfaced via the single-allow path = 401.
	test.RunGoTest(t, func(t *testing.T) {
		jwks1, _ := signES256(t, "k1", "iss")
		_, tokOther := signES256(t, "k1", "iss") // fresh key, same kid
		cfg := jsonConfig(t,
			[]map[string]any{consumerEntry("c1", jwks1, "iss")},
			map[string]any{
				"global_auth": true,
				"_rules_": []map[string]any{{
					"_match_route_": []string{"r1"},
					"allow":         []string{"c1"},
				}},
			},
		)
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		require.NoError(t, host.SetRouteName("r1"))

		_ = host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"},
			{":path", "/p"},
			{"authorization", "Bearer " + tokOther},
		})
		resp := host.GetLocalResponse()
		require.NotNil(t, resp)
		require.Equal(t, uint32(401), resp.StatusCode)
		require.Equal(t, "jwt-auth.verification_failed", resp.StatusCodeDetail)
	})
}

func TestPlugin_GlobalAuthTrue_AllowExcludesConsumer_Returns403(t *testing.T) {
	// Token verifies as c1 but allow=[c2] → unauthorized consumer (403).
	test.RunGoTest(t, func(t *testing.T) {
		jwks1, tok1 := signES256(t, "k1", "iss")
		jwks2, _ := signES256(t, "k2", "iss")
		cfg := jsonConfig(t,
			[]map[string]any{
				consumerEntry("c1", jwks1, "iss"),
				consumerEntry("c2", jwks2, "iss"),
			},
			map[string]any{
				"global_auth": true,
				"_rules_": []map[string]any{{
					"_match_route_": []string{"r1"},
					"allow":         []string{"c2"},
				}},
			},
		)
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		require.NoError(t, host.SetRouteName("r1"))

		_ = host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"},
			{":path", "/p"},
			{"authorization", "Bearer " + tok1},
		})
		resp := host.GetLocalResponse()
		require.NotNil(t, resp)
		require.Equal(t, uint32(403), resp.StatusCode)
		require.Equal(t, "jwt-auth.unauthorized_customer", resp.StatusCodeDetail)
	})
}

func TestPlugin_GlobalAuthFalse_RuleAllowMatches_Authenticates(t *testing.T) {
	// global_auth=false but the matched route has allow=[c1] → that consumer
	// must verify and authenticate. Exercises the globalAuthSetFalse + !noAllow branch.
	test.RunGoTest(t, func(t *testing.T) {
		jwks, tok := signES256(t, "k1", "iss")
		cfg := jsonConfig(t,
			[]map[string]any{consumerEntry("c1", jwks, "iss")},
			map[string]any{
				"global_auth": false,
				"_rules_": []map[string]any{{
					"_match_route_": []string{"r1"},
					"allow":         []string{"c1"},
				}},
			},
		)
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		require.NoError(t, host.SetRouteName("r1"))

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"},
			{":path", "/p"},
			{"authorization", "Bearer " + tok},
		})
		require.Equal(t, types.ActionContinue, action)
		require.Equal(t, "c1", headerValue(host.GetRequestHeaders(), "x-mse-consumer"))
	})
}

func TestPlugin_GlobalAuthFalse_NoAllow_Passthrough(t *testing.T) {
	// global_auth=false + no rule-level allow → plugin should bypass auth.
	test.RunGoTest(t, func(t *testing.T) {
		// Reset the package-level RuleSet flag — other tests may have flipped it.
		config.RuleSet = false
		jwks, _ := signES256(t, "k1", "iss")
		cfg := jsonConfig(t,
			[]map[string]any{consumerEntry("c1", jwks, "iss")},
			map[string]any{"global_auth": false},
		)
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "test.com"}, {":path", "/p"},
		})
		require.Equal(t, types.ActionContinue, action)
		// No X-Mse-Consumer is injected because verification was skipped.
		require.Empty(t, headerValue(host.GetRequestHeaders(), "x-mse-consumer"))
	})
}
