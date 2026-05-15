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
	"time"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/jwt-auth/config"
)

func cacheRemoteJWKsForTest(name, uri, raw string, expiresAt time.Time) {
	consumer := remoteJWKsTestConsumer(name, uri)
	fetchedAt := expiresAt.Add(-time.Duration(*consumer.JWKsCacheDuration) * time.Second)
	cacheRemoteJWKsFetchedAtForTest(name, uri, raw, fetchedAt)
}

func cacheRemoteJWKsFetchedAtForTest(name, uri, raw string, fetchedAt time.Time) {
	consumer := remoteJWKsTestConsumer(name, uri)
	keys, err := parseJWKs(raw)
	if err != nil {
		panic(err)
	}
	remoteJWKsCache[remoteJWKsCacheKey(consumer)] = cachedJWKs{keys: keys, fetchedAt: fetchedAt}
}

func markRemoteJWKsFetchFailedForTest(name, uri string, at time.Time) {
	consumer := remoteJWKsTestConsumer(name, uri)
	remoteJWKsFetchStates[remoteJWKsCacheKey(consumer)] = remoteJWKsFetchState{lastFailedAt: at}
}

func markRemoteJWKsFetchCompletedForTest(name, uri string, at time.Time) {
	consumer := remoteJWKsTestConsumer(name, uri)
	remoteJWKsFetchStates[remoteJWKsCacheKey(consumer)] = remoteJWKsFetchState{lastCompletedAt: at}
}

func markRemoteJWKsFetchInFlightForTest(name, uri string) {
	consumer := remoteJWKsTestConsumer(name, uri)
	deadline := time.Now().Add(time.Duration(*consumer.JWKsFetchTimeout) * time.Millisecond)
	remoteJWKsFetchStates[remoteJWKsCacheKey(consumer)] = remoteJWKsFetchState{inFlight: true, deadline: deadline}
}

func markRemoteJWKsStaleFetchInFlightForTest(name, uri string) {
	consumer := remoteJWKsTestConsumer(name, uri)
	deadline := time.Now().Add(-time.Millisecond)
	remoteJWKsFetchStates[remoteJWKsCacheKey(consumer)] = remoteJWKsFetchState{inFlight: true, deadline: deadline}
}

func clearRemoteJWKsCacheForTest() {
	remoteJWKsCache = map[string]cachedJWKs{}
	remoteJWKsFetchStates = map[string]remoteJWKsFetchState{}
}

func remoteJWKsTestConsumer(name, uri string) *config.Consumer {
	return &config.Consumer{
		Name:              name,
		JWKsURI:           uri,
		JWKsCacheDuration: &config.DefaultJWKsCacheDuration,
		JWKsFetchTimeout:  &config.DefaultJWKsFetchTimeout,
	}
}
