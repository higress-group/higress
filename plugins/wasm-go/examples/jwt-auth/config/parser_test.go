// Copyright (c) 2023 Alibaba Group Holding Ltd.
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

package config

import (
	"strconv"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

const testJWKs = "{\"keys\":[{\"kty\":\"EC\",\"kid\":\"p256\",\"crv\":\"P-256\",\"x\":\"GWym652nfByDbs4EzNpGXCkdjG03qFZHulNDHTo3YJU\",\"y\":\"5uVg_n-flqRJ5Zhf_aEKS0ow9SddTDgxGduSCgpoAZQ\"}]}"

func TestParseGlobalConfigRecordsRulesExist(t *testing.T) {
	cfg := &JWTAuthConfig{}
	err := ParseGlobalConfig(gjson.Parse(`{
		"consumers": [{
			"name": "inline-consumer",
			"issuer": "higress-test",
			"jwks": `+quoteJSON(testJWKs)+`
		}],
		"_rules_": [{
			"_match_domain_": ["private.example.com"],
			"allow": ["inline-consumer"]
		}]
	}`), cfg, nil)

	if err != nil {
		t.Fatalf("ParseGlobalConfig returned error: %v", err)
	}
	if !cfg.RuleSet {
		t.Fatalf("expected global config to record that route/domain rules exist")
	}
}

func TestParseConsumerCachesInlineJWKs(t *testing.T) {
	consumer, err := ParseConsumer(gjson.Parse(`{
		"name": "inline-consumer",
		"issuer": "higress-test",
		"jwks": `+quoteJSON(testJWKs)+`
	}`), map[string]struct{}{})

	if err != nil {
		t.Fatalf("ParseConsumer returned error: %v", err)
	}
	if consumer.ParsedJWKs == nil || len(consumer.ParsedJWKs.Keys) != 1 {
		t.Fatalf("expected parsed inline jwks to be cached, got: %#v", consumer.ParsedJWKs)
	}
}

func TestParseConsumerTrimsIssuer(t *testing.T) {
	consumer, err := ParseConsumer(gjson.Parse(`{
		"name": "inline-consumer",
		"issuer": " higress-test ",
		"jwks": `+quoteJSON(testJWKs)+`
	}`), map[string]struct{}{})

	if err != nil {
		t.Fatalf("ParseConsumer returned error: %v", err)
	}
	if consumer.Issuer != "higress-test" {
		t.Fatalf("expected issuer to be trimmed, got: %q", consumer.Issuer)
	}
}

func TestParseConsumerAcceptsRemoteJWKsService(t *testing.T) {
	consumer, err := ParseConsumer(gjson.Parse(`{
		"name": "remote-consumer",
		"issuer": "higress-test",
		"remote_jwks": {
			"service_name": "auth.example.com.dns",
			"service_host": "auth.example.com",
			"service_port": 443,
			"path": "/.well-known/jwks.json"
		}
	}`), map[string]struct{}{})

	if err != nil {
		t.Fatalf("ParseConsumer returned error: %v", err)
	}
	if consumer.RemoteJWKs == nil {
		t.Fatalf("expected remote_jwks to be parsed")
	}
	if consumer.RemoteJWKs.ServiceName != "auth.example.com.dns" {
		t.Fatalf("unexpected service_name: %q", consumer.RemoteJWKs.ServiceName)
	}
	if consumer.RemoteJWKs.ServiceHost != "auth.example.com" {
		t.Fatalf("unexpected service_host: %q", consumer.RemoteJWKs.ServiceHost)
	}
	if consumer.RemoteJWKs.ServicePort == nil || *consumer.RemoteJWKs.ServicePort != 443 {
		t.Fatalf("unexpected service_port: %v", consumer.RemoteJWKs.ServicePort)
	}
	if consumer.RemoteJWKs.Path != "/.well-known/jwks.json" {
		t.Fatalf("unexpected path: %q", consumer.RemoteJWKs.Path)
	}
	if got := *consumer.JWKsCacheDuration; got != 600 {
		t.Fatalf("unexpected jwks_cache_duration: %d", got)
	}
	if got := *consumer.JWKsFetchTimeout; got != 1500 {
		t.Fatalf("unexpected jwks_fetch_timeout: %d", got)
	}
}

func TestParseConsumerTrimsRemoteJWKsServiceFields(t *testing.T) {
	consumer, err := ParseConsumer(gjson.Parse(`{
		"name": "remote-consumer",
		"issuer": "higress-test",
		"remote_jwks": {
			"service_name": " auth.example.com.dns ",
			"service_host": " auth.example.com ",
			"path": " /.well-known/jwks.json "
		}
	}`), map[string]struct{}{})

	if err != nil {
		t.Fatalf("ParseConsumer returned error: %v", err)
	}
	if consumer.RemoteJWKs.ServiceName != "auth.example.com.dns" {
		t.Fatalf("unexpected service_name: %q", consumer.RemoteJWKs.ServiceName)
	}
	if consumer.RemoteJWKs.ServiceHost != "auth.example.com" {
		t.Fatalf("unexpected service_host: %q", consumer.RemoteJWKs.ServiceHost)
	}
	if consumer.RemoteJWKs.Path != "/.well-known/jwks.json" {
		t.Fatalf("unexpected path: %q", consumer.RemoteJWKs.Path)
	}
	if consumer.RemoteJWKs.ServicePort == nil || *consumer.RemoteJWKs.ServicePort != 443 {
		t.Fatalf("expected default service_port 443, got: %v", consumer.RemoteJWKs.ServicePort)
	}
}

func TestParseConsumerRejectsBothInlineAndRemoteJWKs(t *testing.T) {
	_, err := ParseConsumer(gjson.Parse(`{
		"name": "remote-consumer",
		"issuer": "higress-test",
		"jwks": `+quoteJSON(testJWKs)+`,
		"remote_jwks": {"service_name": "auth.example.com.dns", "path": "/.well-known/jwks.json"}
	}`), map[string]struct{}{})

	if err == nil || !containsError(err, "only one of jwks and remote_jwks can be configured") {
		t.Fatalf("expected mutually exclusive jwks error, got: %v", err)
	}
}

func TestParseConsumerRejectsMissingJWKs(t *testing.T) {
	_, err := ParseConsumer(gjson.Parse(`{
		"name": "remote-consumer",
		"issuer": "higress-test"
	}`), map[string]struct{}{})

	if err == nil || !containsError(err, "one of jwks and remote_jwks is required") {
		t.Fatalf("expected missing jwks error, got: %v", err)
	}
}

func TestParseConsumerRejectsRemoteJWKsWithoutIssuer(t *testing.T) {
	_, err := ParseConsumer(gjson.Parse(`{
		"name": "remote-consumer",
		"remote_jwks": {"service_name": "auth.example.com.dns", "path": "/.well-known/jwks.json"}
	}`), map[string]struct{}{})

	if err == nil || !containsError(err, "issuer is required when remote_jwks is set") {
		t.Fatalf("expected missing issuer error for remote jwks, got: %v", err)
	}
}

func TestParseConsumerRejectsInvalidRemoteJWKsService(t *testing.T) {
	tests := []struct {
		name       string
		remoteJWKs string
	}{
		{name: "missing service_name", remoteJWKs: `"path": "/.well-known/jwks.json"`},
		{name: "blank service_name", remoteJWKs: `"service_name": " ", "path": "/.well-known/jwks.json"`},
		{name: "service_name whitespace", remoteJWKs: `"service_name": "auth example", "path": "/.well-known/jwks.json"`},
		{name: "service_name cluster separator", remoteJWKs: `"service_name": "auth|example", "path": "/.well-known/jwks.json"`},
		{name: "service_name path", remoteJWKs: `"service_name": "auth.example.com/jwks", "path": "/.well-known/jwks.json"`},
		{name: "missing service_host", remoteJWKs: `"service_name": "auth.example.com.dns", "path": "/.well-known/jwks.json"`},
		{name: "service_host whitespace", remoteJWKs: `"service_name": "auth.example.com.dns", "service_host": "auth example", "path": "/.well-known/jwks.json"`},
		{name: "service_host scheme", remoteJWKs: `"service_name": "auth.example.com.dns", "service_host": "https://auth.example.com", "path": "/.well-known/jwks.json"`},
		{name: "service_host port", remoteJWKs: `"service_name": "auth.example.com.dns", "service_host": "auth.example.com:8443", "path": "/.well-known/jwks.json"`},
		{name: "service_host path", remoteJWKs: `"service_name": "auth.example.com.dns", "service_host": "auth.example.com/jwks", "path": "/.well-known/jwks.json"`},
		{name: "service_host userinfo", remoteJWKs: `"service_name": "auth.example.com.dns", "service_host": "user@auth.example.com", "path": "/.well-known/jwks.json"`},
		{name: "path control char", remoteJWKs: `"service_name": "auth.example.com.dns", "service_host": "auth.example.com", "path": "/jwks\n.json"`},
		{name: "path whitespace", remoteJWKs: `"service_name": "auth.example.com.dns", "service_host": "auth.example.com", "path": "/jwks file.json"`},
		{name: "missing path", remoteJWKs: `"service_name": "auth.example.com.dns", "service_host": "auth.example.com"`},
		{name: "relative path", remoteJWKs: `"service_name": "auth.example.com.dns", "service_host": "auth.example.com", "path": "jwks.json"`},
		{name: "invalid port", remoteJWKs: `"service_name": "auth.example.com.dns", "service_host": "auth.example.com", "service_port": 99999, "path": "/.well-known/jwks.json"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseConsumer(gjson.Parse(`{
				"name": "remote-consumer",
				"issuer": "higress-test",
				"remote_jwks": {`+tt.remoteJWKs+`}
			}`), map[string]struct{}{})

			if err == nil || !containsError(err, "remote_jwks is invalid") {
				t.Fatalf("expected invalid remote_jwks error, got: %v", err)
			}
		})
	}
}

func TestParseConsumerRejectsTooLargeRemoteJWKsFetchTimeout(t *testing.T) {
	_, err := ParseConsumer(gjson.Parse(`{
		"name": "remote-consumer",
		"issuer": "higress-test",
		"remote_jwks": {"service_name": "auth.example.com.dns", "service_host": "auth.example.com", "path": "/.well-known/jwks.json"},
		"jwks_fetch_timeout": 10001
	}`), map[string]struct{}{})

	if err == nil || !containsError(err, "jwks_fetch_timeout must be less than or equal to") {
		t.Fatalf("expected invalid jwks_fetch_timeout error, got: %v", err)
	}
}

func TestParseConsumerRejectsTooLargeRemoteJWKsCacheDuration(t *testing.T) {
	_, err := ParseConsumer(gjson.Parse(`{
		"name": "remote-consumer",
		"issuer": "higress-test",
		"remote_jwks": {"service_name": "auth.example.com.dns", "service_host": "auth.example.com", "path": "/.well-known/jwks.json"},
		"jwks_cache_duration": 604801
	}`), map[string]struct{}{})

	if err == nil || !containsError(err, "jwks_cache_duration must be less than or equal to") {
		t.Fatalf("expected invalid jwks_cache_duration error, got: %v", err)
	}
}

func TestParseConsumerRejectsTooSmallRemoteJWKsCacheDuration(t *testing.T) {
	_, err := ParseConsumer(gjson.Parse(`{
		"name": "remote-consumer",
		"issuer": "higress-test",
		"remote_jwks": {"service_name": "auth.example.com.dns", "service_host": "auth.example.com", "path": "/.well-known/jwks.json"},
		"jwks_cache_duration": 29
	}`), map[string]struct{}{})

	if err == nil || !containsError(err, "jwks_cache_duration must be greater than or equal to 30") {
		t.Fatalf("expected invalid jwks_cache_duration error, got: %v", err)
	}
}

func TestParseConsumerRejectsRemoteJWKsOptionsForInlineJWKs(t *testing.T) {
	_, err := ParseConsumer(gjson.Parse(`{
		"name": "inline-consumer",
		"issuer": "higress-test",
		"jwks": `+quoteJSON(testJWKs)+`,
		"jwks_cache_duration": 600
	}`), map[string]struct{}{})

	if err == nil || !containsError(err, "jwks_cache_duration and jwks_fetch_timeout only apply to remote_jwks") {
		t.Fatalf("expected inline jwks remote option error, got: %v", err)
	}
}

func TestParseConsumerRejectsEmptyInlineJWKs(t *testing.T) {
	_, err := ParseConsumer(gjson.Parse(`{
		"name": "remote-consumer",
		"issuer": "higress-test",
		"jwks": "{\"keys\":[]}"
	}`), map[string]struct{}{})

	if err == nil || !containsError(err, "jwks is empty") {
		t.Fatalf("expected empty jwks error, got: %v", err)
	}
}

func quoteJSON(value string) string {
	return strconv.Quote(value)
}

func containsError(err error, want string) bool {
	return err != nil && strings.Contains(err.Error(), want)
}
