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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	cfg "github.com/alibaba/higress/plugins/wasm-go/extensions/jwt-auth/config"
	"github.com/go-jose/go-jose/v3"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
)

type cachedJWKs struct {
	keys      *jose.JSONWebKeySet
	fetchedAt time.Time
}

type remoteJWKsFetchState struct {
	inFlight        bool
	startedAt       time.Time
	deadline        time.Time
	lastCompletedAt time.Time
	lastFailedAt    time.Time
}

// These maps are process-local caches in the single-threaded proxy-wasm VM.
var remoteJWKsCache = map[string]cachedJWKs{}
var remoteJWKsFetchStates = map[string]remoteJWKsFetchState{}

var errRemoteJWKsCacheMiss = errors.New("remote jwks cache is missing or expired")
var errRemoteJWKsRefreshThrottled = errors.New("remote jwks refresh is throttled")

// Failed remote JWKS fetches are backed off per URI. In-flight requests are not
// coalesced because proxy-wasm callbacks are bound to one HTTP stream context.
const remoteJWKsMinRefreshInterval = 30 * time.Second
const maxRemoteJWKsResponseSize = 64 * 1024

func remoteJWKsCacheKey(consumer *cfg.Consumer) string {
	return consumer.JWKsURI
}

func consumerJWKs(consumer *cfg.Consumer, now time.Time) (*jose.JSONWebKeySet, error) {
	raw := consumer.JWKs
	if raw == "" {
		cached, ok := remoteJWKsCache[remoteJWKsCacheKey(consumer)]
		cacheDuration := remoteJWKsCacheDuration(consumer)
		if ok && now.Before(cached.fetchedAt.Add(time.Duration(cacheDuration)*time.Second)) {
			return cached.keys, nil
		}
		if ok && remoteJWKsFetchInFlight(consumer, now) {
			return cached.keys, nil
		}
		if !remoteJWKsFetchAllowed(consumer, now) {
			return nil, errRemoteJWKsRefreshThrottled
		}
		return nil, errRemoteJWKsCacheMiss
	}

	return parseJWKs(raw)
}

func isRemoteJWKsCacheMiss(err error) bool {
	return errors.Is(err, errRemoteJWKsCacheMiss)
}

func isRemoteJWKsRefreshThrottled(err error) bool {
	return errors.Is(err, errRemoteJWKsRefreshThrottled)
}

func fetchRemoteJWKs(consumer *cfg.Consumer, log log.Log, callback func()) error {
	cluster, path, err := remoteJWKsFetchCluster(consumer)
	if err != nil {
		recordRemoteJWKsFetchFailure(consumer, time.Time{}, time.Now())
		return err
	}

	timeout := uint32(remoteJWKsFetchTimeout(consumer))
	startedAt := time.Now()
	recordRemoteJWKsFetchStart(consumer, startedAt)
	headers := [][2]string{{"Accept", "application/json"}, {":method", http.MethodGet}, {":path", path}, {":authority", cluster.HostName()}}
	_, err = proxywasm.DispatchHttpCall(cluster.ClusterName(), headers, nil, nil, timeout, func(numHeaders, bodySize, numTrailers int) {
		statusCode, err := remoteJWKsResponseStatus()
		if err != nil {
			recordRemoteJWKsFetchFailure(consumer, startedAt, time.Now())
			log.Warnf("failed to read remote jwks response status, consumer:%s, reason:%s", consumer.Name, err.Error())
			callback()
			return
		}
		if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
			recordRemoteJWKsFetchFailure(consumer, startedAt, time.Now())
			log.Warnf("failed to fetch remote jwks, consumer:%s, status:%d", consumer.Name, statusCode)
			callback()
			return
		}
		if bodySize > maxRemoteJWKsResponseSize {
			recordRemoteJWKsFetchFailure(consumer, startedAt, time.Now())
			log.Warnf("remote jwks is invalid, consumer:%s, status:%d, reason:jwks response exceeds %d bytes", consumer.Name, statusCode, maxRemoteJWKsResponseSize)
			callback()
			return
		}
		body, err := proxywasm.GetHttpCallResponseBody(0, bodySize)
		if err != nil {
			recordRemoteJWKsFetchFailure(consumer, startedAt, time.Now())
			log.Warnf("failed to read remote jwks response body, consumer:%s, status:%d, reason:%s", consumer.Name, statusCode, err.Error())
			callback()
			return
		}
		keys, err := parseRemoteJWKsResponse(string(body))
		if err != nil {
			recordRemoteJWKsFetchFailure(consumer, startedAt, time.Now())
			log.Warnf("remote jwks is invalid, consumer:%s, status:%d, reason:%s", consumer.Name, statusCode, err.Error())
			callback()
			return
		}
		cacheRemoteJWKs(consumer, keys, startedAt, time.Now())
		callback()
	})
	if err != nil {
		recordRemoteJWKsFetchFailure(consumer, startedAt, time.Now())
		return err
	}
	return nil
}

func remoteJWKsResponseStatus() (int, error) {
	headers, err := proxywasm.GetHttpCallResponseHeaders()
	if err != nil {
		return 0, err
	}
	for _, header := range headers {
		if header[0] == ":status" {
			return strconv.Atoi(header[1])
		}
	}
	return 0, fmt.Errorf("missing :status")
}

func remoteJWKsFetchCluster(consumer *cfg.Consumer) (wrapper.FQDNCluster, string, error) {
	parsed, err := url.Parse(consumer.JWKsURI)
	if err != nil {
		return wrapper.FQDNCluster{}, "", err
	}
	if parsed.Scheme != "https" {
		return wrapper.FQDNCluster{}, "", fmt.Errorf("jwks_uri scheme must be https")
	}

	port := int64(443)
	if parsed.Port() != "" {
		parsedPort, err := strconv.ParseInt(parsed.Port(), 10, 64)
		if err != nil {
			return wrapper.FQDNCluster{}, "", err
		}
		port = parsedPort
	}
	if port <= 0 || port > 65535 {
		return wrapper.FQDNCluster{}, "", fmt.Errorf("jwks_uri port is invalid: %d", port)
	}
	return wrapper.FQDNCluster{
		FQDN: parsed.Hostname(),
		Host: parsed.Host,
		Port: port,
	}, parsed.RequestURI(), nil
}

func remoteJWKsCacheDuration(consumer *cfg.Consumer) int64 {
	if consumer.JWKsCacheDuration == nil {
		return cfg.DefaultJWKsCacheDuration
	}
	return *consumer.JWKsCacheDuration
}

func remoteJWKsFetchTimeout(consumer *cfg.Consumer) int64 {
	if consumer.JWKsFetchTimeout == nil {
		return cfg.DefaultJWKsFetchTimeout
	}
	return *consumer.JWKsFetchTimeout
}

func parseRemoteJWKsResponse(raw string) (*jose.JSONWebKeySet, error) {
	if len(raw) > maxRemoteJWKsResponseSize {
		return nil, fmt.Errorf("jwks response exceeds %d bytes", maxRemoteJWKsResponseSize)
	}
	return parseJWKs(raw)
}

func parseJWKs(raw string) (*jose.JSONWebKeySet, error) {
	jwks := &jose.JSONWebKeySet{}
	if err := json.Unmarshal([]byte(raw), jwks); err != nil {
		return nil, err
	}
	if len(jwks.Keys) == 0 {
		return nil, fmt.Errorf("jwks has no keys")
	}
	return jwks, nil
}

func remoteJWKsFetchAllowed(consumer *cfg.Consumer, now time.Time) bool {
	state, ok := remoteJWKsFetchStates[remoteJWKsCacheKey(consumer)]
	if !ok {
		return true
	}
	if remoteJWKsInFlight(state, now) {
		return false
	}
	return state.lastFailedAt.IsZero() || now.Sub(state.lastFailedAt) >= remoteJWKsMinRefreshInterval
}

func remoteJWKsKeyRefreshAllowed(consumer *cfg.Consumer, now time.Time) bool {
	state, ok := remoteJWKsFetchStates[remoteJWKsCacheKey(consumer)]
	if !ok {
		return true
	}
	if remoteJWKsInFlight(state, now) {
		return false
	}
	lastAttemptAt := state.lastFailedAt
	if state.lastCompletedAt.After(lastAttemptAt) {
		lastAttemptAt = state.lastCompletedAt
	}
	return lastAttemptAt.IsZero() || now.Sub(lastAttemptAt) >= remoteJWKsMinRefreshInterval
}

func remoteJWKsFetchInFlight(consumer *cfg.Consumer, now time.Time) bool {
	state, ok := remoteJWKsFetchStates[remoteJWKsCacheKey(consumer)]
	return ok && remoteJWKsInFlight(state, now)
}

func remoteJWKsInFlight(state remoteJWKsFetchState, now time.Time) bool {
	return state.inFlight && now.Before(state.deadline)
}

func recordRemoteJWKsFetchStart(consumer *cfg.Consumer, now time.Time) {
	// Callers must check remoteJWKsFetchAllowed before starting a new fetch.
	state := remoteJWKsFetchStates[remoteJWKsCacheKey(consumer)]
	state.inFlight = true
	state.startedAt = now
	state.deadline = now.Add(time.Duration(remoteJWKsFetchTimeout(consumer)) * time.Millisecond)
	remoteJWKsFetchStates[remoteJWKsCacheKey(consumer)] = state
}

func recordRemoteJWKsFetchFailure(consumer *cfg.Consumer, startedAt time.Time, now time.Time) {
	state := remoteJWKsFetchStates[remoteJWKsCacheKey(consumer)]
	if !state.startedAt.Equal(startedAt) {
		return
	}
	state.inFlight = false
	state.startedAt = time.Time{}
	state.deadline = time.Time{}
	state.lastFailedAt = now
	remoteJWKsFetchStates[remoteJWKsCacheKey(consumer)] = state
}

func cacheRemoteJWKs(consumer *cfg.Consumer, keys *jose.JSONWebKeySet, startedAt time.Time, now time.Time) {
	cacheKey := remoteJWKsCacheKey(consumer)
	state := remoteJWKsFetchStates[cacheKey]
	if !state.startedAt.Equal(startedAt) {
		return
	}
	remoteJWKsCache[cacheKey] = cachedJWKs{
		keys:      keys,
		fetchedAt: now,
	}
	state.inFlight = false
	state.startedAt = time.Time{}
	state.deadline = time.Time{}
	state.lastCompletedAt = now
	state.lastFailedAt = time.Time{}
	remoteJWKsFetchStates[cacheKey] = state
}
