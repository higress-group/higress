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

package main

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
)

const (
	remoteES256Allow = "eyJhbGciOiJFUzI1NiIsImtpZCI6InAyNTYiLCJ0eXAiOiJKV1QifQ.eyJhdWQiOlsiZm9vIiwiYmFyIl0sImV4cCI6MjAxOTY4NjQwMCwiaXNzIjoiaGlncmVzcy10ZXN0IiwibmJmIjoxNzA0MDY3MjAwLCJzdWIiOiJoaWdyZXNzLXRlc3QifQ.hm71YWfjALshUAgyOu-r9W2WBG_zfqIZZacAbc7oIH1r7dbB0sGQn3wKMWMmOzmxX0UyaVZ0KMk-HFTA1hDnBQ"
	remoteJWKs       = "{\"keys\":[{\"kty\":\"EC\",\"kid\":\"p256\",\"crv\":\"P-256\",\"x\":\"GWym652nfByDbs4EzNpGXCkdjG03qFZHulNDHTo3YJU\",\"y\":\"5uVg_n-flqRJ5Zhf_aEKS0ow9SddTDgxGduSCgpoAZQ\"}]}"
)

func TestRemoteJWKsFetchAuthenticatesRequest(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(remoteJWKsConfig("https://auth.example.com/.well-known/jwks.json"))
		defer host.Reset()
		if status != types.OnPluginStartStatusOK {
			t.Fatalf("unexpected plugin start status: %v", status)
		}

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/"},
			{":method", "GET"},
			{"authorization", "Bearer " + remoteES256Allow},
		})
		if action != types.HeaderStopAllIterationAndWatermark {
			t.Fatalf("expected header stop for remote JWKS fetch, got: %v", action)
		}

		host.CallOnHttpCall([][2]string{{":status", "200"}, {"content-type", "application/json"}}, []byte(remoteJWKs))
		if got := host.GetHttpStreamAction(); got != types.ActionContinue {
			t.Fatalf("expected request to resume, got: %v", got)
		}

		foundConsumer := false
		for _, header := range host.GetRequestHeaders() {
			if strings.EqualFold(header[0], "X-Mse-Consumer") && header[1] == "remote-consumer" {
				foundConsumer = true
			}
		}
		if !foundConsumer {
			t.Fatalf("expected X-Mse-Consumer header after remote JWKS authentication")
		}
		host.CompleteHttp()
	})
}

func TestRemoteJWKsFetchAuthenticatesRequestWithKeepTokenFalse(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(remoteJWKsConfigWithKeepToken("https://auth.example.com/.well-known/keep-token-false.json", false))
		defer host.Reset()
		if status != types.OnPluginStartStatusOK {
			t.Fatalf("unexpected plugin start status: %v", status)
		}

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/"},
			{":method", "GET"},
			{"authorization", "Bearer " + remoteES256Allow},
		})
		if action != types.HeaderStopAllIterationAndWatermark {
			t.Fatalf("expected header stop for remote JWKS fetch, got: %v", action)
		}

		host.CallOnHttpCall([][2]string{{":status", "200"}, {"content-type", "application/json"}}, []byte(remoteJWKs))
		if got := host.GetHttpStreamAction(); got != types.ActionContinue {
			t.Fatalf("expected request to resume, got: %v", got)
		}
		for _, header := range host.GetRequestHeaders() {
			if strings.EqualFold(header[0], "authorization") {
				t.Fatalf("expected authorization header to be removed after authentication")
			}
		}
		host.CompleteHttp()
	})
}

func TestKeepTokenFalseDoesNotRemoveClaimHeaderWithSameName(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(remoteJWKsConfigWithAuthorizationClaimHeader("https://auth.example.com/.well-known/claim-header.json"))
		defer host.Reset()
		if status != types.OnPluginStartStatusOK {
			t.Fatalf("unexpected plugin start status: %v", status)
		}

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/"},
			{":method", "GET"},
			{"authorization", "Bearer " + remoteES256Allow},
		})
		if action != types.HeaderStopAllIterationAndWatermark {
			t.Fatalf("expected header stop for remote JWKS fetch, got: %v", action)
		}

		host.CallOnHttpCall([][2]string{{":status", "200"}, {"content-type", "application/json"}}, []byte(remoteJWKs))
		if got := host.GetHttpStreamAction(); got != types.ActionContinue {
			t.Fatalf("expected request to resume, got: %v", got)
		}
		foundClaimHeader := false
		for _, header := range host.GetRequestHeaders() {
			if strings.EqualFold(header[0], "authorization") && header[1] == "higress-test" {
				foundClaimHeader = true
			}
		}
		if !foundClaimHeader {
			t.Fatalf("expected claims_to_headers to replace authorization after token removal")
		}
		host.CompleteHttp()
	})
}

func TestRemoteJWKsMissDoesNotBlockLaterInlineConsumer(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(remoteMissThenInlineConfig())
		defer host.Reset()
		if status != types.OnPluginStartStatusOK {
			t.Fatalf("unexpected plugin start status: %v", status)
		}

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/"},
			{":method", "GET"},
			{"authorization", "Bearer " + remoteES256Allow},
		})
		if action != types.ActionContinue {
			t.Fatalf("expected later inline consumer to continue request, got: %v", action)
		}

		foundConsumer := false
		for _, header := range host.GetRequestHeaders() {
			if strings.EqualFold(header[0], "X-Mse-Consumer") && header[1] == "inline-consumer" {
				foundConsumer = true
			}
		}
		if !foundConsumer {
			t.Fatalf("expected X-Mse-Consumer header for inline consumer")
		}
		host.CompleteHttp()
	})
}

func TestUnmatchedRouteBypassesWhenRulesExistAndGlobalAuthUnset(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(routeScopedInlineConfig())
		defer host.Reset()
		if status != types.OnPluginStartStatusOK {
			t.Fatalf("unexpected plugin start status: %v", status)
		}

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "public.example.com"},
			{":path", "/health"},
			{":method", "GET"},
		})
		if action != types.ActionContinue {
			t.Fatalf("expected unmatched route to bypass auth when global_auth is unset, got: %v", action)
		}
		host.CompleteHttp()
	})
}

func TestRemoteJWKsFetchFailureThrottlesNextRequest(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(remoteJWKsConfig("https://auth.example.com/.well-known/failing-jwks.json"))
		defer host.Reset()
		if status != types.OnPluginStartStatusOK {
			t.Fatalf("unexpected plugin start status: %v", status)
		}

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/"},
			{":method", "GET"},
			{"authorization", "Bearer " + remoteES256Allow},
		})
		if action != types.HeaderStopAllIterationAndWatermark {
			t.Fatalf("expected header stop for remote JWKS fetch, got: %v", action)
		}

		host.CallOnHttpCall([][2]string{{":status", "500"}}, nil)
		host.CompleteHttp()

		action = host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/"},
			{":method", "GET"},
			{"authorization", "Bearer " + remoteES256Allow},
		})
		if action == types.HeaderStopAllIterationAndWatermark {
			t.Fatalf("expected recent remote JWKS failure to throttle instead of fetching again")
		}
		if response := host.GetLocalResponse(); response == nil {
			t.Fatalf("expected throttled request to fail closed")
		}
		host.CompleteHttp()
	})
}

func TestRemoteJWKsOnlyFetchesAllowedConsumers(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(disallowedRemoteThenAllowedRemoteConfig())
		defer host.Reset()
		if status != types.OnPluginStartStatusOK {
			t.Fatalf("unexpected plugin start status: %v", status)
		}

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/"},
			{":method", "GET"},
			{"authorization", "Bearer " + remoteES256Allow},
		})
		if action != types.HeaderStopAllIterationAndWatermark {
			t.Fatalf("expected header stop for allowed remote JWKS fetch, got: %v", action)
		}

		host.CallOnHttpCall([][2]string{{":status", "200"}, {"content-type", "application/json"}}, []byte(remoteJWKs))
		if got := host.GetHttpStreamAction(); got != types.ActionContinue {
			t.Fatalf("expected request to resume, got: %v", got)
		}

		foundConsumer := false
		for _, header := range host.GetRequestHeaders() {
			if strings.EqualFold(header[0], "X-Mse-Consumer") && header[1] == "allowed-remote-consumer" {
				foundConsumer = true
			}
		}
		if !foundConsumer {
			t.Fatalf("expected X-Mse-Consumer header for allowed remote consumer")
		}
		host.CompleteHttp()
	})
}

func TestRemoteJWKsFetchFailureTriesNextAllowedRemoteConsumer(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(twoAllowedRemoteConsumersConfig())
		defer host.Reset()
		if status != types.OnPluginStartStatusOK {
			t.Fatalf("unexpected plugin start status: %v", status)
		}

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/"},
			{":method", "GET"},
			{"authorization", "Bearer " + remoteES256Allow},
		})
		if action != types.HeaderStopAllIterationAndWatermark {
			t.Fatalf("expected header stop for first remote JWKS fetch, got: %v", action)
		}

		host.CallOnHttpCall([][2]string{{":status", "500"}}, nil)
		if response := host.GetLocalResponse(); response != nil {
			t.Fatalf("expected first remote JWKS fetch failure to try another allowed consumer")
		}

		host.CallOnHttpCall([][2]string{{":status", "200"}, {"content-type", "application/json"}}, []byte(remoteJWKs))
		if got := host.GetHttpStreamAction(); got != types.ActionContinue {
			t.Fatalf("expected request to resume after second remote JWKS authentication, got: %v", got)
		}
		host.CompleteHttp()
	})
}

func TestRemoteJWKsFetchChainExhaustsEligibleConsumers(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(manyAllowedRemoteConsumersConfig(4))
		defer host.Reset()
		if status != types.OnPluginStartStatusOK {
			t.Fatalf("unexpected plugin start status: %v", status)
		}

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/"},
			{":method", "GET"},
			{"authorization", "Bearer " + remoteES256Allow},
		})
		if action != types.HeaderStopAllIterationAndWatermark {
			t.Fatalf("expected header stop for first remote JWKS fetch, got: %v", action)
		}

		host.CallOnHttpCall([][2]string{{":status", "500"}}, nil)
		if response := host.GetLocalResponse(); response != nil {
			t.Fatalf("expected first failed fetch to try the next remote consumer")
		}
		host.CallOnHttpCall([][2]string{{":status", "500"}}, nil)
		if response := host.GetLocalResponse(); response != nil {
			t.Fatalf("expected second failed fetch to try the next remote consumer")
		}
		host.CallOnHttpCall([][2]string{{":status", "500"}}, nil)
		if response := host.GetLocalResponse(); response != nil {
			t.Fatalf("expected third failed fetch to try the next remote consumer")
		}
		host.CallOnHttpCall([][2]string{{":status", "500"}}, nil)
		if response := host.GetLocalResponse(); response == nil {
			t.Fatalf("expected chained remote JWKS fetches to stop after eligible consumers are exhausted")
		}
		host.CompleteHttp()
	})
}

func TestAllowedRemoteMissTakesPriorityOverUnauthorizedInlineConsumer(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(allowedRemoteThenUnauthorizedInlineConfig())
		defer host.Reset()
		if status != types.OnPluginStartStatusOK {
			t.Fatalf("unexpected plugin start status: %v", status)
		}

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/"},
			{":method", "GET"},
			{"authorization", "Bearer " + remoteES256Allow},
		})
		if action != types.HeaderStopAllIterationAndWatermark {
			t.Fatalf("expected allowed remote consumer to fetch before unauthorized denial, got: %v", action)
		}
		host.CompleteHttp()
	})
}

func TestUnauthorizedConsumerDoesNotMutateRequestBeforeAllowedRemoteFetch(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(allowedRemoteThenMutatingUnauthorizedInlineConfig())
		defer host.Reset()
		if status != types.OnPluginStartStatusOK {
			t.Fatalf("unexpected plugin start status: %v", status)
		}

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/"},
			{":method", "GET"},
			{"authorization", "Bearer " + remoteES256Allow},
		})
		if action != types.HeaderStopAllIterationAndWatermark {
			t.Fatalf("expected allowed remote consumer to fetch before unauthorized denial, got: %v", action)
		}

		host.CallOnHttpCall([][2]string{{":status", "200"}, {"content-type", "application/json"}}, []byte(remoteJWKs))
		if got := host.GetHttpStreamAction(); got != types.ActionContinue {
			t.Fatalf("expected request to resume, got: %v", got)
		}
		for _, header := range host.GetRequestHeaders() {
			if strings.EqualFold(header[0], "x-unauthorized-subject") {
				t.Fatalf("unexpected header mutation from unauthorized consumer")
			}
			if strings.EqualFold(header[0], "authorization") {
				t.Fatalf("expected authorization header to be removed only after allowed remote authentication")
			}
		}
		host.CompleteHttp()
	})
}

func remoteJWKsConfig(endpoint string) json.RawMessage {
	return remoteJWKsConfigWithKeepToken(endpoint, true)
}

func remoteJWKsConfigWithKeepToken(endpoint string, keepToken bool) json.RawMessage {
	data, _ := json.Marshal(map[string]any{
		"global_auth": true,
		"consumers": []map[string]any{{
			"name":               "remote-consumer",
			"issuer":             "higress-test",
			"remote_jwks":        remoteJWKsService(endpoint),
			"clock_skew_seconds": 2000000000,
			"keep_token":         keepToken,
		}},
	})
	return data
}

func remoteJWKsConfigWithAuthorizationClaimHeader(endpoint string) json.RawMessage {
	data, _ := json.Marshal(map[string]any{
		"global_auth": true,
		"consumers": []map[string]any{{
			"name":               "remote-consumer",
			"issuer":             "higress-test",
			"remote_jwks":        remoteJWKsService(endpoint),
			"clock_skew_seconds": 2000000000,
			"keep_token":         false,
			"claims_to_headers": []map[string]any{{
				"claim":  "iss",
				"header": "authorization",
			}},
		}},
	})
	return data
}

func disallowedRemoteThenAllowedRemoteConfig() json.RawMessage {
	data, _ := json.Marshal(map[string]any{
		"global_auth": true,
		"consumers": []map[string]any{
			{
				"name":               "disallowed-remote-consumer",
				"issuer":             "higress-test",
				"remote_jwks":        remoteJWKsService("https://auth.example.com/.well-known/disallowed.json"),
				"clock_skew_seconds": 2000000000,
			},
			{
				"name":               "allowed-remote-consumer",
				"issuer":             "higress-test",
				"remote_jwks":        remoteJWKsService("https://auth.example.com/.well-known/allowed.json"),
				"clock_skew_seconds": 2000000000,
			},
		},
		"_rules_": []map[string]any{{
			"_match_route_": []string{"test-route-default"},
			"allow":         []string{"allowed-remote-consumer"},
		}},
	})
	return data
}

func twoAllowedRemoteConsumersConfig() json.RawMessage {
	data, _ := json.Marshal(map[string]any{
		"global_auth": true,
		"consumers": []map[string]any{
			{
				"name":               "first-remote-consumer",
				"issuer":             "higress-test",
				"remote_jwks":        remoteJWKsService("https://auth.example.com/.well-known/first.json"),
				"clock_skew_seconds": 2000000000,
			},
			{
				"name":               "second-remote-consumer",
				"issuer":             "higress-test",
				"remote_jwks":        remoteJWKsService("https://auth.example.com/.well-known/second.json"),
				"clock_skew_seconds": 2000000000,
			},
		},
		"_rules_": []map[string]any{{
			"_match_route_": []string{"test-route-default"},
			"allow":         []string{"first-remote-consumer", "second-remote-consumer"},
		}},
	})
	return data
}

func manyAllowedRemoteConsumersConfig(count int) json.RawMessage {
	consumers := make([]map[string]any, 0, count)
	allow := make([]string, 0, count)
	for i := 0; i < count; i++ {
		name := "remote-consumer-" + string(rune('a'+i))
		consumers = append(consumers, map[string]any{
			"name":               name,
			"issuer":             "higress-test",
			"remote_jwks":        remoteJWKsService("https://auth.example.com/.well-known/" + name + ".json"),
			"clock_skew_seconds": 2000000000,
		})
		allow = append(allow, name)
	}
	data, _ := json.Marshal(map[string]any{
		"global_auth": true,
		"consumers":   consumers,
		"_rules_": []map[string]any{{
			"_match_route_": []string{"test-route-default"},
			"allow":         allow,
		}},
	})
	return data
}

func allowedRemoteThenUnauthorizedInlineConfig() json.RawMessage {
	data, _ := json.Marshal(map[string]any{
		"global_auth": true,
		"consumers": []map[string]any{
			{
				"name":               "allowed-remote-consumer",
				"issuer":             "higress-test",
				"remote_jwks":        remoteJWKsService("https://auth.example.com/.well-known/allowed-priority.json"),
				"clock_skew_seconds": 2000000000,
			},
			{
				"name":               "unauthorized-inline-consumer",
				"issuer":             "higress-test",
				"jwks":               remoteJWKs,
				"clock_skew_seconds": 2000000000,
			},
		},
		"_rules_": []map[string]any{{
			"_match_route_": []string{"test-route-default"},
			"allow":         []string{"allowed-remote-consumer"},
		}},
	})
	return data
}

func allowedRemoteThenMutatingUnauthorizedInlineConfig() json.RawMessage {
	data, _ := json.Marshal(map[string]any{
		"global_auth": true,
		"consumers": []map[string]any{
			{
				"name":               "allowed-remote-consumer",
				"issuer":             "higress-test",
				"remote_jwks":        remoteJWKsService("https://auth.example.com/.well-known/allowed-mutation.json"),
				"clock_skew_seconds": 2000000000,
				"keep_token":         false,
			},
			{
				"name":               "unauthorized-inline-consumer",
				"issuer":             "higress-test",
				"jwks":               remoteJWKs,
				"clock_skew_seconds": 2000000000,
				"keep_token":         false,
				"claims_to_headers": []map[string]any{{
					"claim":  "sub",
					"header": "x-unauthorized-subject",
				}},
			},
		},
		"_rules_": []map[string]any{{
			"_match_route_": []string{"test-route-default"},
			"allow":         []string{"allowed-remote-consumer"},
		}},
	})
	return data
}

func remoteMissThenInlineConfig() json.RawMessage {
	data, _ := json.Marshal(map[string]any{
		"global_auth": true,
		"consumers": []map[string]any{
			{
				"name":               "remote-consumer",
				"issuer":             "higress-test",
				"remote_jwks":        remoteJWKsService("https://auth.example.com/.well-known/miss-before-inline.json"),
				"clock_skew_seconds": 2000000000,
			},
			{
				"name":               "inline-consumer",
				"issuer":             "higress-test",
				"jwks":               remoteJWKs,
				"clock_skew_seconds": 2000000000,
			},
		},
	})
	return data
}

func routeScopedInlineConfig() json.RawMessage {
	data, _ := json.Marshal(map[string]any{
		"consumers": []map[string]any{{
			"name":               "inline-consumer",
			"issuer":             "higress-test",
			"jwks":               remoteJWKs,
			"clock_skew_seconds": 2000000000,
		}},
		"_rules_": []map[string]any{{
			"_match_domain_": []string{"private.example.com"},
			"allow":          []string{"inline-consumer"},
		}},
	})
	return data
}

func remoteJWKsService(endpoint string) map[string]any {
	parsed, _ := url.Parse(endpoint)
	port := 443
	if parsed.Scheme == "http" {
		port = 80
	}
	return map[string]any{
		"service_name": parsed.Hostname() + ".dns",
		"service_host": parsed.Hostname(),
		"service_port": port,
		"path":         parsed.RequestURI(),
	}
}
