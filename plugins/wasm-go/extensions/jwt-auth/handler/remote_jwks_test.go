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

package handler

import (
	"strings"
	"testing"
	"time"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/jwt-auth/config"
)

func TestParseRemoteJWKsRejectsInvalidKeySets(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "invalid json", raw: `{"keys":[`},
		{name: "empty key set", raw: `{"keys":[]}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parseJWKs(tt.raw); err == nil {
				t.Fatalf("expected %s to be rejected", tt.name)
			}
		})
	}
}

func TestParseRemoteJWKsRejectsOversizedResponses(t *testing.T) {
	raw := strings.Repeat(" ", maxRemoteJWKsResponseSize+1) + JWKs
	if _, err := parseRemoteJWKsResponse(raw); err == nil {
		t.Fatalf("expected oversized remote jwks response to be rejected")
	}
}

func TestRemoteJWKsFetchClusterUsesHostnameForExplicitPort(t *testing.T) {
	cluster, path, err := remoteJWKsFetchCluster(remoteJWKsTestConsumer("consumer-remote", "https://auth.example.com:8443/.well-known/jwks.json"))
	if err != nil {
		t.Fatalf("remoteJWKsFetchCluster returned error: %v", err)
	}
	if cluster.FQDN != "auth.example.com" {
		t.Fatalf("unexpected cluster FQDN: %q", cluster.FQDN)
	}
	if cluster.Host != "auth.example.com:8443" {
		t.Fatalf("expected host with port for authority, got: %q", cluster.Host)
	}
	if cluster.Port != 8443 {
		t.Fatalf("unexpected cluster port: %d", cluster.Port)
	}
	if path != "/.well-known/jwks.json" {
		t.Fatalf("unexpected request path: %q", path)
	}
}

func TestRemoteJWKsCacheExpiryReturnsCacheMiss(t *testing.T) {
	uri := "https://auth.example.com/.well-known/jwks.json"
	cacheRemoteJWKsForTest("consumer-remote", uri, JWKs, time.Now().Add(-time.Minute))
	defer clearRemoteJWKsCacheForTest()

	consumer := &config.Consumer{
		Name:              "consumer-remote",
		JWKsURI:           uri,
		JWKsCacheDuration: &config.DefaultJWKsCacheDuration,
		JWKsFetchTimeout:  &config.DefaultJWKsFetchTimeout,
	}
	if _, err := consumerJWKs(consumer, time.Now()); !isRemoteJWKsCacheMiss(err) {
		t.Fatalf("expected expired remote jwks to return cache miss, got: %v", err)
	}
}

func TestRemoteJWKsCacheExpiryAfterSuccessDoesNotThrottleRefresh(t *testing.T) {
	uri := "https://auth.example.com/.well-known/jwks.json"
	now := time.Now()
	cacheRemoteJWKsForTest("consumer-remote", uri, JWKs, now.Add(-time.Second))
	markRemoteJWKsFetchCompletedForTest("consumer-remote", uri, now.Add(-2*time.Second))
	defer clearRemoteJWKsCacheForTest()

	consumer := remoteJWKsTestConsumer("consumer-remote", uri)
	if _, err := consumerJWKs(consumer, now); !isRemoteJWKsCacheMiss(err) {
		t.Fatalf("expected expired remote jwks to request refresh, got: %v", err)
	}
}

func TestRemoteJWKsSuccessClearsRecentFailureThrottle(t *testing.T) {
	uri := "https://auth.example.com/.well-known/jwks.json"
	now := time.Now()
	markRemoteJWKsFetchFailedForTest("consumer-remote", uri, now.Add(-3*time.Second))
	keys, err := parseJWKs(JWKs)
	if err != nil {
		t.Fatalf("parseJWKs returned error: %v", err)
	}
	cacheRemoteJWKs(remoteJWKsTestConsumer("consumer-remote", uri), keys, time.Time{}, now.Add(-2*time.Second))
	defer clearRemoteJWKsCacheForTest()

	cacheDuration := int64(1)
	consumer := remoteJWKsTestConsumer("consumer-remote", uri)
	consumer.JWKsCacheDuration = &cacheDuration
	if _, err := consumerJWKs(consumer, now); !isRemoteJWKsCacheMiss(err) {
		t.Fatalf("expected expired remote jwks to request refresh after a later success, got: %v", err)
	}
}

func TestRemoteJWKsExpiredCacheServedWhileRefreshInFlight(t *testing.T) {
	uri := "https://auth.example.com/.well-known/jwks.json"
	now := time.Now()
	cacheRemoteJWKsFetchedAtForTest("consumer-remote", uri, JWKs, now.Add(-time.Minute))
	markRemoteJWKsFetchInFlightForTest("consumer-remote", uri)
	defer clearRemoteJWKsCacheForTest()

	cacheDuration := int64(1)
	consumer := remoteJWKsTestConsumer("consumer-remote", uri)
	consumer.JWKsCacheDuration = &cacheDuration
	keys, err := consumerJWKs(consumer, now)
	if err != nil {
		t.Fatalf("expected expired remote jwks to be served while refresh is in flight, got: %v", err)
	}
	if len(keys.Keys) == 0 {
		t.Fatalf("expected cached keys")
	}
}

func TestRemoteJWKsRecentFailureThrottlesCacheMiss(t *testing.T) {
	uri := "https://auth.example.com/.well-known/jwks.json"
	now := time.Now()
	markRemoteJWKsFetchFailedForTest("consumer-remote", uri, now)
	defer clearRemoteJWKsCacheForTest()

	consumer := &config.Consumer{
		Name:              "consumer-remote",
		JWKsURI:           uri,
		JWKsCacheDuration: &config.DefaultJWKsCacheDuration,
		JWKsFetchTimeout:  &config.DefaultJWKsFetchTimeout,
	}
	if _, err := consumerJWKs(consumer, now.Add(time.Second)); !isRemoteJWKsRefreshThrottled(err) {
		t.Fatalf("expected recent remote jwks fetch to be throttled, got: %v", err)
	}
}

func TestRemoteJWKsInFlightFetchThrottlesCacheMiss(t *testing.T) {
	uri := "https://auth.example.com/.well-known/jwks.json"
	markRemoteJWKsFetchInFlightForTest("consumer-remote", uri)
	defer clearRemoteJWKsCacheForTest()

	consumer := remoteJWKsTestConsumer("consumer-remote", uri)
	if _, err := consumerJWKs(consumer, time.Now()); !isRemoteJWKsRefreshThrottled(err) {
		t.Fatalf("expected in-flight remote jwks fetch to be throttled, got: %v", err)
	}
}

func TestRemoteJWKsStaleInFlightFetchAllowsCacheMiss(t *testing.T) {
	uri := "https://auth.example.com/.well-known/jwks.json"
	markRemoteJWKsStaleFetchInFlightForTest("consumer-remote", uri)
	defer clearRemoteJWKsCacheForTest()

	consumer := remoteJWKsTestConsumer("consumer-remote", uri)
	if _, err := consumerJWKs(consumer, time.Now()); !isRemoteJWKsCacheMiss(err) {
		t.Fatalf("expected stale in-flight remote jwks fetch to allow retry, got: %v", err)
	}
}

func TestRemoteJWKsFailureThrottleIsSharedByURI(t *testing.T) {
	uri := "https://auth.example.com/.well-known/jwks.json"
	now := time.Now()
	markRemoteJWKsFetchFailedForTest("consumer-short", uri, now)
	defer clearRemoteJWKsCacheForTest()

	cacheDuration := int64(120)
	consumer := remoteJWKsTestConsumer("consumer-long", uri)
	consumer.JWKsCacheDuration = &cacheDuration
	if _, err := consumerJWKs(consumer, now.Add(time.Second)); !isRemoteJWKsRefreshThrottled(err) {
		t.Fatalf("expected recent remote jwks failure to throttle all consumers for same URI, got: %v", err)
	}
}

func TestRemoteJWKsCacheIsSharedByURIWithConsumerSpecificExpiry(t *testing.T) {
	uri := "https://auth.example.com/.well-known/jwks.json"
	now := time.Now()
	cacheRemoteJWKsFetchedAtForTest("consumer-short", uri, JWKs, now.Add(-time.Minute))
	markRemoteJWKsFetchCompletedForTest("consumer-short", uri, now.Add(-time.Second))
	defer clearRemoteJWKsCacheForTest()

	cacheDuration := int64(120)
	consumer := remoteJWKsTestConsumer("consumer-long", uri)
	consumer.JWKsCacheDuration = &cacheDuration
	if _, err := consumerJWKs(consumer, now); err != nil {
		t.Fatalf("expected consumer with longer cache duration to reuse shared remote jwks, got: %v", err)
	}

	cacheDuration = int64(30)
	consumer.JWKsCacheDuration = &cacheDuration
	if _, err := consumerJWKs(consumer, now); !isRemoteJWKsCacheMiss(err) {
		t.Fatalf("expected consumer with shorter cache duration to expire shared remote jwks, got: %v", err)
	}

	cacheDuration = int64(120)
	consumer.JWKsCacheDuration = &cacheDuration
	if _, err := consumerJWKs(consumer, now); err != nil {
		t.Fatalf("expected shorter cache duration miss not to evict shared remote jwks for longer duration, got: %v", err)
	}
}

func TestRemoteJWKsInFlightDeadlineUsesOriginatorTimeout(t *testing.T) {
	uri := "https://auth.example.com/.well-known/jwks.json"
	longTimeout := int64(5000)
	originator := remoteJWKsTestConsumer("consumer-long-timeout", uri)
	originator.JWKsFetchTimeout = &longTimeout
	recordRemoteJWKsFetchStart(originator, time.Now().Add(-2*time.Second))
	defer clearRemoteJWKsCacheForTest()

	shortTimeout := int64(1000)
	consumer := remoteJWKsTestConsumer("consumer-short-timeout", uri)
	consumer.JWKsFetchTimeout = &shortTimeout
	if _, err := consumerJWKs(consumer, time.Now()); !isRemoteJWKsRefreshThrottled(err) {
		t.Fatalf("expected in-flight fetch to use originator timeout, got: %v", err)
	}
}

func TestStaleRemoteJWKsFetchFailureDoesNotClearNewInFlightFetch(t *testing.T) {
	uri := "https://auth.example.com/.well-known/jwks.json"
	consumer := remoteJWKsTestConsumer("consumer-remote", uri)
	now := time.Now()
	firstStartedAt := now.Add(-2 * time.Second)
	secondStartedAt := now

	recordRemoteJWKsFetchStart(consumer, firstStartedAt)
	recordRemoteJWKsFetchStart(consumer, secondStartedAt)
	recordRemoteJWKsFetchFailure(consumer, firstStartedAt, now.Add(time.Millisecond))
	defer clearRemoteJWKsCacheForTest()

	if !remoteJWKsFetchInFlight(consumer, now.Add(2*time.Millisecond)) {
		t.Fatalf("expected stale callback not to clear newer in-flight fetch")
	}
}

func TestStaleRemoteJWKsFetchSuccessDoesNotOverwriteNewerCache(t *testing.T) {
	uri := "https://auth.example.com/.well-known/jwks.json"
	consumer := remoteJWKsTestConsumer("consumer-remote", uri)
	now := time.Now()
	firstStartedAt := now.Add(-2 * time.Second)
	secondStartedAt := now
	staleKeys, err := parseJWKs(JWKs)
	if err != nil {
		t.Fatalf("parseJWKs returned error: %v", err)
	}
	freshKeys, err := parseJWKs(`{"keys":[{"kty":"RSA","kid":"fresh","n":"pFKAKJ0V3vFwGTvBSHbPwrNdvPyr-zMTh7Y9IELFIMNUQfG9_d2D1wZcrX5CPvtEISHin3GdPyfqEX6NjPyqvCLFTuNh80-r5Mvld-A5CHwITZXz5krBdqY5Z0wu64smMbzst3HNxHbzLQvHUY-KS6hceOB84d9B4rhkIJEEAWxxIA7yPJYjYyIC_STpPddtJkkweVvoa0m0-_FQkDFsbRS0yGgMNG4-uc7qLIU4kSwMQWcw1Rwy39LUDP4zNzuZABbWsDDBsMlVUaszRdKIlk5AQ-Fkah3E247dYGUQjSQ0N3dFLlMDv_e62BT3IBXGLg7wvGosWFNT_LpIenIW6Q","e":"AQAB"}]}`)
	if err != nil {
		t.Fatalf("parseJWKs returned error: %v", err)
	}

	recordRemoteJWKsFetchStart(consumer, firstStartedAt)
	recordRemoteJWKsFetchStart(consumer, secondStartedAt)
	cacheRemoteJWKs(consumer, freshKeys, secondStartedAt, now.Add(time.Millisecond))
	cacheRemoteJWKs(consumer, staleKeys, firstStartedAt, now.Add(2*time.Millisecond))
	defer clearRemoteJWKsCacheForTest()

	cached := remoteJWKsCache[remoteJWKsCacheKey(consumer)]
	if got := cached.keys.Keys[0].KeyID; got != "fresh" {
		t.Fatalf("expected stale success not to overwrite newer cache, got key id: %q", got)
	}
}

func TestStaleRemoteJWKsFetchFailureDoesNotThrottleAfterNewerSuccess(t *testing.T) {
	uri := "https://auth.example.com/.well-known/jwks.json"
	consumer := remoteJWKsTestConsumer("consumer-remote", uri)
	now := time.Now()
	firstStartedAt := now.Add(-2 * time.Second)
	secondStartedAt := now
	keys, err := parseJWKs(JWKs)
	if err != nil {
		t.Fatalf("parseJWKs returned error: %v", err)
	}

	recordRemoteJWKsFetchStart(consumer, firstStartedAt)
	recordRemoteJWKsFetchStart(consumer, secondStartedAt)
	cacheRemoteJWKs(consumer, keys, secondStartedAt, now.Add(time.Millisecond))
	recordRemoteJWKsFetchFailure(consumer, firstStartedAt, now.Add(2*time.Millisecond))
	defer clearRemoteJWKsCacheForTest()

	state := remoteJWKsFetchStates[remoteJWKsCacheKey(consumer)]
	if !state.lastFailedAt.IsZero() {
		t.Fatalf("expected stale failure not to reintroduce throttle after newer success")
	}
	if state.lastCompletedAt.IsZero() {
		t.Fatalf("expected newer success timestamp to be preserved")
	}
}

func TestRemoteJWKsUsesDefaultsWhenRemoteConsumerDefaultsAreNil(t *testing.T) {
	uri := "https://auth.example.com/.well-known/jwks.json"
	now := time.Now()
	cacheRemoteJWKsFetchedAtForTest("consumer-remote", uri, JWKs, now)
	defer clearRemoteJWKsCacheForTest()

	consumer := &config.Consumer{Name: "consumer-remote", JWKsURI: uri}
	keys, err := consumerJWKs(consumer, now)
	if err != nil {
		t.Fatalf("expected nil remote defaults to use package defaults, got: %v", err)
	}
	if len(keys.Keys) == 0 {
		t.Fatalf("expected cached keys")
	}
	recordRemoteJWKsFetchStart(consumer, now)
	if !remoteJWKsFetchInFlight(consumer, now) {
		t.Fatalf("expected nil fetch timeout to use package default")
	}
}
