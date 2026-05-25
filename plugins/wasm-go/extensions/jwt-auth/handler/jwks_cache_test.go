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
	"net/url"
	"strconv"
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
	remoteJWKsCache[remoteJWKsCacheKeyForConsumer(consumer)] = cachedJWKs{keys: keys, fetchedAt: fetchedAt}
}

func markRemoteJWKsFetchFailedForTest(name, uri string, at time.Time) {
	consumer := remoteJWKsTestConsumer(name, uri)
	remoteJWKsFetchStates[remoteJWKsCacheKeyForConsumer(consumer)] = remoteJWKsFetchState{lastFailedAt: at}
}

func markRemoteJWKsFetchInFlightForTest(name, uri string) {
	consumer := remoteJWKsTestConsumer(name, uri)
	deadline := time.Now().Add(time.Duration(*consumer.JWKsFetchTimeout) * time.Millisecond)
	remoteJWKsFetchStates[remoteJWKsCacheKeyForConsumer(consumer)] = remoteJWKsFetchState{inFlight: true, deadline: deadline}
}

func markRemoteJWKsStaleFetchInFlightForTest(name, uri string) {
	consumer := remoteJWKsTestConsumer(name, uri)
	deadline := time.Now().Add(-time.Millisecond)
	remoteJWKsFetchStates[remoteJWKsCacheKeyForConsumer(consumer)] = remoteJWKsFetchState{inFlight: true, deadline: deadline}
}

func clearRemoteJWKsCacheForTest() {
	remoteJWKsCache = map[remoteJWKsCacheKey]cachedJWKs{}
	remoteJWKsFetchStates = map[remoteJWKsCacheKey]remoteJWKsFetchState{}
}

func remoteJWKsTestConsumer(name, uri string) *config.Consumer {
	remote := &config.RemoteJWKs{
		ServiceName: "auth.example.com.dns",
		ServiceHost: "auth.example.com",
		ServicePort: int64Ptr(443),
		Path:        "/.well-known/jwks.json",
	}
	if parsed, err := url.Parse(uri); err == nil && parsed.Hostname() != "" {
		remote.ServiceName = parsed.Hostname() + ".dns"
		remote.ServiceHost = parsed.Hostname()
		remote.Path = parsed.RequestURI()
		if parsed.Port() != "" {
			port, err := strconv.ParseInt(parsed.Port(), 10, 64)
			if err == nil {
				remote.ServicePort = &port
			}
		} else if parsed.Scheme == "http" {
			remote.ServicePort = int64Ptr(80)
		}
	}
	return &config.Consumer{
		Name:              name,
		RemoteJWKs:        remote,
		JWKsCacheDuration: &config.DefaultJWKsCacheDuration,
		JWKsFetchTimeout:  &config.DefaultJWKsFetchTimeout,
	}
}

func int64Ptr(v int64) *int64 {
	return &v
}
