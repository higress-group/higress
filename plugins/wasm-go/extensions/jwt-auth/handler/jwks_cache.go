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
	inFlight     bool
	startedAt    time.Time
	deadline     time.Time
	lastFailedAt time.Time
}

type remoteJWKsCacheKey struct {
	serviceName string
	serviceHost string
	servicePort int64
	path        string
}

// These maps are process-local caches in the single-threaded proxy-wasm VM.
var remoteJWKsCache = map[remoteJWKsCacheKey]cachedJWKs{}
var remoteJWKsFetchStates = map[remoteJWKsCacheKey]remoteJWKsFetchState{}

var errRemoteJWKsCacheMiss = errors.New("remote jwks cache is missing or expired")
var errRemoteJWKsRefreshThrottled = errors.New("remote jwks refresh is throttled")

var dispatchRemoteJWKsHTTPCall = proxywasm.DispatchHttpCall

// Failed remote JWKS fetches are backed off per remote service reference.
// In-flight requests are not coalesced because proxy-wasm callbacks are bound
// to one HTTP stream context.
const remoteJWKsMinRefreshInterval = time.Duration(cfg.RemoteJWKsMinRefreshIntervalSeconds) * time.Second
const maxRemoteJWKsResponseSize = 64 * 1024

func remoteJWKsCacheKeyForConsumer(consumer *cfg.Consumer) remoteJWKsCacheKey {
	remote := consumer.RemoteJWKs
	if remote == nil {
		return remoteJWKsCacheKey{}
	}
	return remoteJWKsCacheKey{
		serviceName: remote.ServiceName,
		serviceHost: remote.ServiceHost,
		servicePort: remoteJWKsServicePort(remote),
		path:        remote.Path,
	}
}

func PruneRemoteJWKsCache(consumers []*cfg.Consumer) {
	active := make(map[remoteJWKsCacheKey]struct{}, len(consumers))
	for _, consumer := range consumers {
		if consumer != nil && consumer.RemoteJWKs != nil {
			active[remoteJWKsCacheKeyForConsumer(consumer)] = struct{}{}
		}
	}
	for key := range remoteJWKsCache {
		if _, ok := active[key]; !ok {
			delete(remoteJWKsCache, key)
		}
	}
	for key := range remoteJWKsFetchStates {
		if _, ok := active[key]; !ok {
			delete(remoteJWKsFetchStates, key)
		}
	}
}

func consumerJWKs(consumer *cfg.Consumer, now time.Time) (*jose.JSONWebKeySet, error) {
	raw := consumer.JWKs
	if raw == "" {
		cached, ok := remoteJWKsCache[remoteJWKsCacheKeyForConsumer(consumer)]
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

	if consumer.ParsedJWKs != nil {
		return consumer.ParsedJWKs, nil
	}
	return parseJWKs(raw)
}

// remoteJWKsFetchedAfter tells the verifier whether this same request has
// already retried with a freshly fetched JWKS.
func remoteJWKsFetchedAfter(consumer *cfg.Consumer, t time.Time) bool {
	cached, ok := remoteJWKsCache[remoteJWKsCacheKeyForConsumer(consumer)]
	return ok && cached.fetchedAt.After(t)
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
		return err
	}

	timeout := uint32(remoteJWKsFetchTimeout(consumer))
	startedAt := time.Now()
	if !recordRemoteJWKsFetchStart(consumer, startedAt) {
		return errRemoteJWKsRefreshThrottled
	}
	headers := [][2]string{{"Accept", "application/json"}, {":method", http.MethodGet}, {":path", path}, {":authority", remoteJWKsAuthority(consumer)}}
	_, err = dispatchRemoteJWKsHTTPCall(cluster.ClusterName(), headers, nil, nil, timeout, func(numHeaders, bodySize, numTrailers int) {
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
	remote := consumer.RemoteJWKs
	if remote == nil || remote.ServiceName == "" || remote.Path == "" {
		return wrapper.FQDNCluster{}, "", fmt.Errorf("remote_jwks is not configured")
	}
	return wrapper.FQDNCluster{
		FQDN: remote.ServiceName,
		Host: remote.ServiceHost,
		Port: remoteJWKsServicePort(remote),
	}, remote.Path, nil
}

func remoteJWKsServicePort(remote *cfg.RemoteJWKs) int64 {
	if remote.ServicePort == nil {
		return 443
	}
	return *remote.ServicePort
}

func remoteJWKsAuthority(consumer *cfg.Consumer) string {
	remote := consumer.RemoteJWKs
	if remote == nil {
		return ""
	}
	port := remoteJWKsServicePort(remote)
	if port == 80 || port == 443 {
		return remote.ServiceHost
	}
	return remote.ServiceHost + ":" + strconv.FormatInt(port, 10)
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

// Initial cold/expired fetches are only backed off after failures. A successful
// completed fetch must not block the next TTL-driven refresh.
func remoteJWKsFetchAllowed(consumer *cfg.Consumer, now time.Time) bool {
	state, ok := remoteJWKsFetchStates[remoteJWKsCacheKeyForConsumer(consumer)]
	if !ok {
		return true
	}
	if remoteJWKsInFlight(state, now) {
		return false
	}
	return state.lastFailedAt.IsZero() || now.Sub(state.lastFailedAt) >= remoteJWKsMinRefreshInterval
}

func remoteJWKsFetchInFlight(consumer *cfg.Consumer, now time.Time) bool {
	state, ok := remoteJWKsFetchStates[remoteJWKsCacheKeyForConsumer(consumer)]
	return ok && remoteJWKsInFlight(state, now)
}

func remoteJWKsInFlight(state remoteJWKsFetchState, now time.Time) bool {
	return state.inFlight && now.Before(state.deadline)
}

func recordRemoteJWKsFetchStart(consumer *cfg.Consumer, now time.Time) bool {
	if !remoteJWKsFetchAllowed(consumer, now) {
		return false
	}
	state := remoteJWKsFetchStates[remoteJWKsCacheKeyForConsumer(consumer)]
	state.inFlight = true
	state.startedAt = now
	state.deadline = now.Add(time.Duration(remoteJWKsFetchTimeout(consumer)) * time.Millisecond)
	remoteJWKsFetchStates[remoteJWKsCacheKeyForConsumer(consumer)] = state
	return true
}

func recordRemoteJWKsFetchFailure(consumer *cfg.Consumer, startedAt time.Time, now time.Time) {
	state := remoteJWKsFetchStates[remoteJWKsCacheKeyForConsumer(consumer)]
	if !state.startedAt.Equal(startedAt) {
		return
	}
	state.inFlight = false
	state.startedAt = time.Time{}
	state.deadline = time.Time{}
	state.lastFailedAt = now
	remoteJWKsFetchStates[remoteJWKsCacheKeyForConsumer(consumer)] = state
}

func cacheRemoteJWKs(consumer *cfg.Consumer, keys *jose.JSONWebKeySet, startedAt time.Time, now time.Time) {
	cacheKey := remoteJWKsCacheKeyForConsumer(consumer)
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
	state.lastFailedAt = time.Time{}
	remoteJWKsFetchStates[cacheKey] = state
}
