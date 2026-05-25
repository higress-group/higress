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
	"errors"
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

func TestRemoteJWKsFetchClusterUsesConfiguredServiceReference(t *testing.T) {
	port := int64(8443)
	cluster, path, err := remoteJWKsFetchCluster(&config.Consumer{
		Name: "consumer-remote",
		RemoteJWKs: &config.RemoteJWKs{
			ServiceName: "issuer-jwks.dns",
			ServiceHost: "auth.example.com",
			ServicePort: &port,
			Path:        "/.well-known/jwks.json",
		},
	})
	if err != nil {
		t.Fatalf("remoteJWKsFetchCluster returned error: %v", err)
	}
	if cluster.FQDN != "issuer-jwks.dns" {
		t.Fatalf("unexpected cluster FQDN: %q", cluster.FQDN)
	}
	if cluster.Host != "auth.example.com" {
		t.Fatalf("expected bare host for authority, got: %q", cluster.Host)
	}
	if cluster.Port != 8443 {
		t.Fatalf("unexpected cluster port: %d", cluster.Port)
	}
	if path != "/.well-known/jwks.json" {
		t.Fatalf("unexpected request path: %q", path)
	}
}

func TestFetchRemoteJWKsDispatchesConfiguredRequest(t *testing.T) {
	oldDispatch := dispatchRemoteJWKsHTTPCall
	var gotCluster string
	var gotHeaders [][2]string
	var gotTimeout uint32
	dispatchRemoteJWKsHTTPCall = func(cluster string, headers [][2]string, body []byte, trailers [][2]string, timeout uint32, callback func(int, int, int)) (uint32, error) {
		gotCluster = cluster
		gotHeaders = headers
		gotTimeout = timeout
		return 1, nil
	}
	defer func() {
		dispatchRemoteJWKsHTTPCall = oldDispatch
		clearRemoteJWKsCacheForTest()
	}()

	timeout := int64(2500)
	port := int64(8443)
	consumer := &config.Consumer{
		Name:             "consumer-remote",
		JWKsFetchTimeout: &timeout,
		RemoteJWKs: &config.RemoteJWKs{
			ServiceName: "issuer-jwks.dns",
			ServiceHost: "auth.example.com",
			ServicePort: &port,
			Path:        "/.well-known/jwks.json",
		},
	}
	if err := fetchRemoteJWKs(consumer, &testLogger{T: t}, func() {
		t.Fatalf("callback should not run without an HTTP response")
	}); err != nil {
		t.Fatalf("fetchRemoteJWKs returned error: %v", err)
	}

	if gotCluster != "outbound|8443||issuer-jwks.dns" {
		t.Fatalf("unexpected cluster: %q", gotCluster)
	}
	if gotTimeout != uint32(timeout) {
		t.Fatalf("unexpected timeout: %d", gotTimeout)
	}
	wantHeaders := map[string]string{
		"Accept":     "application/json",
		":method":    "GET",
		":path":      "/.well-known/jwks.json",
		":authority": "auth.example.com:8443",
	}
	for _, header := range gotHeaders {
		if want, ok := wantHeaders[header[0]]; ok {
			if header[1] != want {
				t.Fatalf("unexpected %s header: %q", header[0], header[1])
			}
			delete(wantHeaders, header[0])
		}
	}
	if len(wantHeaders) != 0 {
		t.Fatalf("missing dispatch headers: %v", wantHeaders)
	}
}

func TestRemoteJWKsFetchClusterRejectsMissingPath(t *testing.T) {
	_, _, err := remoteJWKsFetchCluster(&config.Consumer{Name: "consumer-remote", RemoteJWKs: &config.RemoteJWKs{ServiceName: "issuer-jwks.dns"}})
	if err == nil {
		t.Fatalf("expected remoteJWKsFetchCluster to reject remote_jwks without path")
	}
}

func TestRemoteJWKsDefaultServicePortIsHTTPS(t *testing.T) {
	if got := remoteJWKsServicePort(&config.RemoteJWKs{}); got != 443 {
		t.Fatalf("expected default remote jwks service port 443, got: %d", got)
	}
}

func TestPruneRemoteJWKsCacheRemovesInactiveEntries(t *testing.T) {
	active := remoteJWKsTestConsumer("active", "https://active.example.com/.well-known/jwks.json")
	inactive := remoteJWKsTestConsumer("inactive", "https://inactive.example.com/.well-known/jwks.json")
	now := time.Now()
	cacheRemoteJWKsFetchedAtForTest(active.Name, "https://active.example.com/.well-known/jwks.json", JWKs, now)
	cacheRemoteJWKsFetchedAtForTest(inactive.Name, "https://inactive.example.com/.well-known/jwks.json", JWKs, now)
	markRemoteJWKsFetchFailedForTest(inactive.Name, "https://inactive.example.com/.well-known/jwks.json", now)
	defer clearRemoteJWKsCacheForTest()

	PruneRemoteJWKsCache([]*config.Consumer{active})

	if _, ok := remoteJWKsCache[remoteJWKsCacheKeyForConsumer(active)]; !ok {
		t.Fatalf("expected active remote jwks cache entry to remain")
	}
	if _, ok := remoteJWKsCache[remoteJWKsCacheKeyForConsumer(inactive)]; ok {
		t.Fatalf("expected inactive remote jwks cache entry to be pruned")
	}
	if _, ok := remoteJWKsFetchStates[remoteJWKsCacheKeyForConsumer(inactive)]; ok {
		t.Fatalf("expected inactive remote jwks fetch state to be pruned")
	}
}

func TestRemoteJWKsCacheExpiryReturnsCacheMiss(t *testing.T) {
	uri := "https://auth.example.com/.well-known/jwks.json"
	cacheRemoteJWKsForTest("consumer-remote", uri, JWKs, time.Now().Add(-time.Minute))
	defer clearRemoteJWKsCacheForTest()

	consumer := remoteJWKsTestConsumer("consumer-remote", uri)
	if _, err := consumerJWKs(consumer, time.Now()); !isRemoteJWKsCacheMiss(err) {
		t.Fatalf("expected expired remote jwks to return cache miss, got: %v", err)
	}
}

func TestRemoteJWKsCacheExpiryAfterSuccessDoesNotThrottleRefresh(t *testing.T) {
	uri := "https://auth.example.com/.well-known/jwks.json"
	now := time.Now()
	cacheRemoteJWKsForTest("consumer-remote", uri, JWKs, now.Add(-time.Second))
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

	consumer := remoteJWKsTestConsumer("consumer-remote", uri)
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

func TestRemoteJWKsPreDispatchErrorDoesNotThrottleURI(t *testing.T) {
	consumer := &config.Consumer{Name: "consumer-remote"}
	err := fetchRemoteJWKs(consumer, &testLogger{T: t}, func() {
		t.Fatalf("callback should not run on pre-dispatch validation failure")
	})
	defer clearRemoteJWKsCacheForTest()

	if err == nil {
		t.Fatalf("expected missing remote_jwks to fail before dispatch")
	}
	state := remoteJWKsFetchStates[remoteJWKsCacheKeyForConsumer(consumer)]
	if !state.lastFailedAt.IsZero() {
		t.Fatalf("pre-dispatch error should not update remote jwks failure throttle")
	}
}

func TestRemoteJWKsDispatchErrorFailsClosedAndThrottles(t *testing.T) {
	consumer := remoteJWKsTestConsumer("consumer-remote", "https://auth.example.com/.well-known/jwks.json")
	oldDispatch := dispatchRemoteJWKsHTTPCall
	dispatchRemoteJWKsHTTPCall = func(string, [][2]string, []byte, [][2]string, uint32, func(int, int, int)) (uint32, error) {
		return 0, errors.New("cluster not found")
	}
	defer func() {
		dispatchRemoteJWKsHTTPCall = oldDispatch
		clearRemoteJWKsCacheForTest()
	}()

	err := fetchRemoteJWKs(consumer, &testLogger{T: t}, func() {
		t.Fatalf("callback should not run when dispatch fails inline")
	})
	if err == nil {
		t.Fatalf("expected inline dispatch error")
	}
	if _, err := consumerJWKs(consumer, time.Now()); !isRemoteJWKsRefreshThrottled(err) {
		t.Fatalf("expected inline dispatch failure to throttle later fetches, got: %v", err)
	}
	if state := remoteJWKsFetchStates[remoteJWKsCacheKeyForConsumer(consumer)]; state.inFlight {
		t.Fatalf("inline dispatch failure should not leave an in-flight fetch")
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

func TestRemoteJWKsFetchStartDoesNotOverwriteActiveInFlightFetch(t *testing.T) {
	uri := "https://auth.example.com/.well-known/jwks.json"
	consumer := remoteJWKsTestConsumer("consumer-remote", uri)
	now := time.Now()
	defer clearRemoteJWKsCacheForTest()

	if !recordRemoteJWKsFetchStart(consumer, now) {
		t.Fatalf("expected first remote jwks fetch start to be recorded")
	}
	if recordRemoteJWKsFetchStart(consumer, now.Add(time.Millisecond)) {
		t.Fatalf("expected active in-flight fetch to reject a second start")
	}

	state := remoteJWKsFetchStates[remoteJWKsCacheKeyForConsumer(consumer)]
	if !state.startedAt.Equal(now) {
		t.Fatalf("expected original fetch start time to be preserved")
	}
}

func TestStaleRemoteJWKsFailureDoesNotClearNewInFlightFetch(t *testing.T) {
	uri := "https://auth.example.com/.well-known/jwks.json"
	consumer := remoteJWKsTestConsumer("consumer-remote", uri)
	now := time.Now()
	firstStartedAt := now.Add(-2 * time.Second)
	secondStartedAt := now
	defer clearRemoteJWKsCacheForTest()

	recordRemoteJWKsFetchStart(consumer, firstStartedAt)
	recordRemoteJWKsFetchStart(consumer, secondStartedAt)
	recordRemoteJWKsFetchFailure(consumer, firstStartedAt, now.Add(time.Millisecond))

	state := remoteJWKsFetchStates[remoteJWKsCacheKeyForConsumer(consumer)]
	if !state.inFlight || !state.startedAt.Equal(secondStartedAt) {
		t.Fatalf("expected stale failure not to clear newer in-flight fetch")
	}
	if !state.lastFailedAt.IsZero() {
		t.Fatalf("expected stale failure not to reintroduce failure throttle")
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

	cached := remoteJWKsCache[remoteJWKsCacheKeyForConsumer(consumer)]
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

	state := remoteJWKsFetchStates[remoteJWKsCacheKeyForConsumer(consumer)]
	if !state.lastFailedAt.IsZero() {
		t.Fatalf("expected stale failure not to reintroduce throttle after newer success")
	}
	if _, ok := remoteJWKsCache[remoteJWKsCacheKeyForConsumer(consumer)]; !ok {
		t.Fatalf("expected newer success cache entry to be preserved")
	}
}
