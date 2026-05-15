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

func TestParseConsumerAcceptsRemoteJWKsURI(t *testing.T) {
	consumer, err := ParseConsumer(gjson.Parse(`{
		"name": "remote-consumer",
		"issuer": "higress-test",
		"jwks_uri": "https://auth.example.com/.well-known/jwks.json"
	}`), map[string]struct{}{})

	if err != nil {
		t.Fatalf("ParseConsumer returned error: %v", err)
	}
	if consumer.JWKsURI != "https://auth.example.com/.well-known/jwks.json" {
		t.Fatalf("unexpected jwks_uri: %q", consumer.JWKsURI)
	}
	if got := *consumer.JWKsCacheDuration; got != 600 {
		t.Fatalf("unexpected jwks_cache_duration: %d", got)
	}
	if got := *consumer.JWKsFetchTimeout; got != 1500 {
		t.Fatalf("unexpected jwks_fetch_timeout: %d", got)
	}
}

func TestParseConsumerNormalizesRemoteJWKsURIHost(t *testing.T) {
	consumer, err := ParseConsumer(gjson.Parse(`{
		"name": "remote-consumer",
		"issuer": "higress-test",
		"jwks_uri": "https://Auth.Example.com:8443/.well-known/jwks.json"
	}`), map[string]struct{}{})

	if err != nil {
		t.Fatalf("ParseConsumer returned error: %v", err)
	}
	if consumer.JWKsURI != "https://auth.example.com:8443/.well-known/jwks.json" {
		t.Fatalf("unexpected normalized jwks_uri: %q", consumer.JWKsURI)
	}
}

func TestParseConsumerRejectsBothInlineAndRemoteJWKs(t *testing.T) {
	_, err := ParseConsumer(gjson.Parse(`{
		"name": "remote-consumer",
		"issuer": "higress-test",
		"jwks": `+quoteJSON(testJWKs)+`,
		"jwks_uri": "https://auth.example.com/.well-known/jwks.json"
	}`), map[string]struct{}{})

	if err == nil || !containsError(err, "only one of jwks and jwks_uri can be configured") {
		t.Fatalf("expected mutually exclusive jwks error, got: %v", err)
	}
}

func TestParseConsumerRejectsMissingJWKs(t *testing.T) {
	_, err := ParseConsumer(gjson.Parse(`{
		"name": "remote-consumer",
		"issuer": "higress-test"
	}`), map[string]struct{}{})

	if err == nil || !containsError(err, "one of jwks and jwks_uri is required") {
		t.Fatalf("expected missing jwks error, got: %v", err)
	}
}

func TestParseConsumerRejectsRemoteJWKsWithoutIssuer(t *testing.T) {
	_, err := ParseConsumer(gjson.Parse(`{
		"name": "remote-consumer",
		"jwks_uri": "https://auth.example.com/.well-known/jwks.json"
	}`), map[string]struct{}{})

	if err == nil || !containsError(err, "issuer is required when jwks_uri is set") {
		t.Fatalf("expected missing issuer error for remote jwks, got: %v", err)
	}
}

func TestParseConsumerRejectsNonHTTPRemoteJWKsURI(t *testing.T) {
	tests := []struct {
		name string
		uri  string
	}{
		{name: "non http scheme", uri: "ftp://auth.example.com/jwks.json"},
		{name: "cleartext http", uri: "http://auth.example.com/jwks.json"},
		{name: "userinfo", uri: "https://user:pass@auth.example.com/jwks.json"},
		{name: "empty userinfo", uri: "https://@auth.example.com/jwks.json"},
		{name: "fragment", uri: "https://auth.example.com/jwks.json#keys"},
		{name: "empty host with port", uri: "https://:443/jwks.json"},
		{name: "empty host with empty port", uri: "https://:/jwks.json"},
		{name: "empty port", uri: "https://auth.example.com:/jwks.json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseConsumer(gjson.Parse(`{
				"name": "remote-consumer",
				"issuer": "higress-test",
				"jwks_uri": "`+tt.uri+`"
			}`), map[string]struct{}{})

			if err == nil || !containsError(err, "jwks_uri is invalid") {
				t.Fatalf("expected invalid jwks_uri error, got: %v", err)
			}
		})
	}
}

func TestParseConsumerRejectsTooLargeRemoteJWKsFetchTimeout(t *testing.T) {
	_, err := ParseConsumer(gjson.Parse(`{
		"name": "remote-consumer",
		"issuer": "higress-test",
		"jwks_uri": "https://auth.example.com/.well-known/jwks.json",
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
		"jwks_uri": "https://auth.example.com/.well-known/jwks.json",
		"jwks_cache_duration": 604801
	}`), map[string]struct{}{})

	if err == nil || !containsError(err, "jwks_cache_duration must be less than or equal to") {
		t.Fatalf("expected invalid jwks_cache_duration error, got: %v", err)
	}
}

func TestParseConsumerRejectsInvalidRemoteJWKsURIPort(t *testing.T) {
	_, err := ParseConsumer(gjson.Parse(`{
		"name": "remote-consumer",
		"issuer": "higress-test",
		"jwks_uri": "https://auth.example.com:99999/.well-known/jwks.json"
	}`), map[string]struct{}{})

	if err == nil || !containsError(err, "jwks_uri is invalid") {
		t.Fatalf("expected invalid jwks_uri port error, got: %v", err)
	}
}

func TestParseConsumerIgnoresRemoteJWKsOptionsForInlineJWKs(t *testing.T) {
	consumer, err := ParseConsumer(gjson.Parse(`{
		"name": "inline-consumer",
		"issuer": "higress-test",
		"jwks": `+quoteJSON(testJWKs)+`,
		"jwks_cache_duration": -1,
		"jwks_fetch_timeout": -1
	}`), map[string]struct{}{})

	if err != nil {
		t.Fatalf("inline jwks consumer should ignore remote jwks options, got: %v", err)
	}
	if consumer.JWKsCacheDuration != nil || consumer.JWKsFetchTimeout != nil {
		t.Fatalf("inline jwks consumer should not keep remote jwks options")
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
