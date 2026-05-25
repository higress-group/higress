// Copyright (c) 2023 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0

package config

import (
	"testing"

	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Minimal log adapter used by ParseGlobalConfig (calls Warn).
type discardLog struct{}

func (discardLog) Trace(string)                       {}
func (discardLog) Tracef(string, ...interface{})      {}
func (discardLog) Debug(string)                       {}
func (discardLog) Debugf(string, ...interface{})      {}
func (discardLog) Info(string)                        {}
func (discardLog) Infof(string, ...interface{})       {}
func (discardLog) Warn(string)                        {}
func (discardLog) Warnf(string, ...interface{})       {}
func (discardLog) Error(string)                       {}
func (discardLog) Errorf(string, ...interface{})      {}
func (discardLog) Critical(string)                    {}
func (discardLog) Criticalf(string, ...interface{})   {}
func (discardLog) ResetID(string)                     {}

var _ log.Log = discardLog{}

const (
	validJWKs = `{"keys":[{"kty":"EC","kid":"p256","crv":"P-256","x":"GWym652nfByDbs4EzNpGXCkdjG03qFZHulNDHTo3YJU","y":"5uVg_n-flqRJ5Zhf_aEKS0ow9SddTDgxGduSCgpoAZQ"}]}`
)

func consumerJSON(name, issuer string) string {
	return `{"name":"` + name + `","issuer":"` + issuer + `","jwks":` + mustQuote(validJWKs) + `}`
}

func mustQuote(s string) string {
	out := `"`
	for _, r := range s {
		switch r {
		case '"':
			out += `\"`
		case '\\':
			out += `\\`
		default:
			out += string(r)
		}
	}
	out += `"`
	return out
}

func TestParseConsumer_DefaultsAreFilled(t *testing.T) {
	raw := gjson.Parse(consumerJSON("c1", "iss"))
	c, err := ParseConsumer(raw, map[string]struct{}{})
	require.NoError(t, err)

	require.NotNil(t, c.FromHeaders)
	require.Equal(t, []FromHeader{{Name: "Authorization", ValuePrefix: "Bearer "}}, *c.FromHeaders)
	require.Equal(t, []string{"access_token"}, *c.FromParams)
	require.Equal(t, []string{}, *c.FromCookies)
	require.Equal(t, int64(60), *c.ClockSkewSeconds)
	require.Equal(t, true, *c.KeepToken)
}

func TestParseConsumer_ExplicitFromSourcesPreventDefaults(t *testing.T) {
	raw := gjson.Parse(`{"name":"c","issuer":"i","jwks":` + mustQuote(validJWKs) +
		`,"from_headers":[{"name":"X-Tok","value_prefix":""}]}`)
	c, err := ParseConsumer(raw, map[string]struct{}{})
	require.NoError(t, err)
	require.Equal(t, "X-Tok", (*c.FromHeaders)[0].Name)
	// Once any source is explicit, the other defaults must remain nil rather than be filled.
	require.Nil(t, c.FromParams)
	require.Nil(t, c.FromCookies)
}

func TestParseConsumer_DuplicateNameRejected(t *testing.T) {
	names := map[string]struct{}{}
	raw := gjson.Parse(consumerJSON("dup", "iss"))
	_, err := ParseConsumer(raw, names)
	require.NoError(t, err)
	_, err = ParseConsumer(raw, names)
	require.Error(t, err)
	require.Contains(t, err.Error(), "consumer already exists")
}

func TestParseConsumer_InvalidJWKsRejected(t *testing.T) {
	raw := gjson.Parse(`{"name":"c","issuer":"i","jwks":"not-json"}`)
	_, err := ParseConsumer(raw, map[string]struct{}{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "jwks is invalid")
}

func TestParseConsumer_ClaimsToHeadersDuplicateHeaderRejected(t *testing.T) {
	raw := gjson.Parse(`{"name":"c","issuer":"i","jwks":` + mustQuote(validJWKs) + `,
"claims_to_headers":[{"claim":"a","header":"X-Dup"},{"claim":"b","header":"X-Dup"}]}`)
	_, err := ParseConsumer(raw, map[string]struct{}{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "claim to header already exists")
}

func TestParseConsumer_ClaimsToHeadersOverrideDefault(t *testing.T) {
	raw := gjson.Parse(`{"name":"c","issuer":"i","jwks":` + mustQuote(validJWKs) + `,
"claims_to_headers":[{"claim":"a","header":"X-A"}]}`)
	c, err := ParseConsumer(raw, map[string]struct{}{})
	require.NoError(t, err)
	require.Equal(t, true, *(*c.ClaimsToHeaders)[0].Override)
}

func TestParseGlobalConfig_RejectsNonArrayConsumers(t *testing.T) {
	cfg := JWTAuthConfig{}
	err := ParseGlobalConfig(gjson.Parse(`{"consumers":"not-array"}`), &cfg, discardLog{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "consumers is not a array")
}

func TestParseGlobalConfig_RejectsEmptyAfterFiltering(t *testing.T) {
	// All consumers invalid → logged + skipped → final length 0 → error.
	cfg := JWTAuthConfig{}
	err := ParseGlobalConfig(gjson.Parse(`{"consumers":[{"name":"c","issuer":"i","jwks":"bad"}]}`),
		&cfg, discardLog{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "at least one consumer")
}

func TestParseGlobalConfig_AcceptsValidConsumers(t *testing.T) {
	cfg := JWTAuthConfig{}
	err := ParseGlobalConfig(gjson.Parse(`{"consumers":[`+consumerJSON("c1", "iss")+`]}`),
		&cfg, discardLog{})
	require.NoError(t, err)
	require.Len(t, cfg.Consumers, 1)
}

func TestParseRuleConfig_AllowMissingRejected(t *testing.T) {
	cfg := JWTAuthConfig{}
	err := ParseRuleConfig(gjson.Parse(`{}`), JWTAuthConfig{}, &cfg, discardLog{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "allow is required")
}

func TestParseRuleConfig_AllowEmptyRejected(t *testing.T) {
	cfg := JWTAuthConfig{}
	err := ParseRuleConfig(gjson.Parse(`{"allow":[]}`), JWTAuthConfig{}, &cfg, discardLog{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "allow cannot be empty")
}

func TestParseRuleConfig_AllowPopulatesAndSetsRuleFlag(t *testing.T) {
	RuleSet = false
	cfg := JWTAuthConfig{}
	err := ParseRuleConfig(gjson.Parse(`{"allow":["c1","c2"]}`), JWTAuthConfig{}, &cfg, discardLog{})
	require.NoError(t, err)
	require.Equal(t, []string{"c1", "c2"}, cfg.Allow)
	require.True(t, RuleSet)
}

func TestParseRuleConfig_OverridesFromGlobal(t *testing.T) {
	// Rule config inherits global consumers via `*config = global`.
	global := JWTAuthConfig{Consumers: []*Consumer{{Name: "g"}}}
	cfg := JWTAuthConfig{}
	err := ParseRuleConfig(gjson.Parse(`{"allow":["g"]}`), global, &cfg, discardLog{})
	require.NoError(t, err)
	require.Len(t, cfg.Consumers, 1)
	require.Equal(t, "g", cfg.Consumers[0].Name)
}
