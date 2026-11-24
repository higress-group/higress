// Copyright (c) 2024 Alibaba Group Holding Ltd.
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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"ext-auth/config"
	"ext-auth/util"

	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/tidwall/resp"
)

// cacheKeyPrefix namespaces ext-auth entries within a shared Redis database.
const cacheKeyPrefix = "ext-auth:"

// buildCacheKey derives a stable key from everything the authorization decision
// depends on: the method and path sent to the auth server plus the full set of
// forwarded request headers, which carry the caller's credentials. Headers are
// sorted so the digest never depends on map iteration order.
func buildCacheKey(method, requestPath string, headers http.Header) string {
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)

	digest := sha256.New()
	digest.Write([]byte(method))
	digest.Write([]byte{'\n'})
	digest.Write([]byte(requestPath))
	digest.Write([]byte{'\n'})
	for _, name := range names {
		digest.Write([]byte(name))
		digest.Write([]byte{':'})
		digest.Write([]byte(strings.Join(headers[name], ",")))
		digest.Write([]byte{'\n'})
	}
	return cacheKeyPrefix + hex.EncodeToString(digest.Sum(nil))
}

// keyFieldValue is one configured cache key field paired with the value read from the
// request. name is "source:key" so a header and a query parameter sharing a name never
// collide in the digest.
type keyFieldValue struct {
	name  string
	value string
}

// buildRequestFieldCacheKey derives the key for the user-specified mode. The method and
// the query-free path are always included as a floor; each configured field contributes
// the value read from the incoming request (a request header, matched case-insensitively
// against inboundHeaders, or a query parameter of fullPath), and an absent field
// contributes an empty value. Unlike the default key, the query string is not folded in
// wholesale and Authorization / forward_auth headers are not included unless explicitly
// listed. inboundHeaders is the raw inbound request header set, so this stays a pure
// function of its inputs and is testable without a wasm host.
func buildRequestFieldCacheKey(method, fullPath string, inboundHeaders [][2]string, keyFields []config.CacheKeyField) string {
	pathWithoutQuery, queryStr := splitPathAndQuery(fullPath)
	// A malformed query parameter is skipped by url.ParseQuery rather than failing the
	// whole parse; the remaining valid parameters are still read, so a bad param simply
	// contributes nothing to the key.
	query, _ := url.ParseQuery(queryStr)

	values := make([]keyFieldValue, 0, len(keyFields))
	for _, field := range keyFields {
		var value string
		if field.Source == config.KeyFieldSourceHeader {
			value = util.ExtractFromHeader(inboundHeaders, field.Key)
		} else {
			value = query.Get(field.Key)
		}
		values = append(values, keyFieldValue{name: field.Source + ":" + field.Key, value: value})
	}
	return buildCacheKeyFromFields(method, pathWithoutQuery, values)
}

// splitPathAndQuery separates the query string from a request path by splitting on the
// first '?', so the path floor stays byte-for-byte what arrived rather than being
// re-encoded by url.Parse.
func splitPathAndQuery(fullPath string) (pathWithoutQuery, query string) {
	if i := strings.IndexByte(fullPath, '?'); i >= 0 {
		return fullPath[:i], fullPath[i+1:]
	}
	return fullPath, ""
}

// buildCacheKeyFromFields hashes the method, the query-free path, and the configured
// field values in declaration order. The order comes from the parsed config, so it is
// stable across requests and needs no sorting.
func buildCacheKeyFromFields(method, pathWithoutQuery string, values []keyFieldValue) string {
	digest := sha256.New()
	digest.Write([]byte(method))
	digest.Write([]byte{'\n'})
	digest.Write([]byte(pathWithoutQuery))
	digest.Write([]byte{'\n'})
	for _, v := range values {
		digest.Write([]byte(v.name))
		digest.Write([]byte{':'})
		digest.Write([]byte(v.value))
		digest.Write([]byte{'\n'})
	}
	return cacheKeyPrefix + hex.EncodeToString(digest.Sum(nil))
}

// cachedAuthResult is the JSON envelope stored in Redis for a cached decision.
// Allow is the discriminant: only a value carrying allow:true is replayed, so a
// non-allow entry (or anything else that landed under the key) is treated as a miss.
type cachedAuthResult struct {
	Allow           bool              `json:"allow"`
	Status          int               `json:"status"`
	UpstreamHeaders map[string]string `json:"upstream_headers"`
}

// decodeCachedHeaders interprets a Redis GET response as a cached allow decision,
// returning the headers to inject upstream. It reports false for any error, miss,
// empty, corrupt, or non-allow value so the caller fails open to a real
// authorization call.
func decodeCachedHeaders(response resp.Value) (map[string]string, bool) {
	if response.Error() != nil || response.IsNull() {
		return nil, false
	}
	raw := response.String()
	if raw == "" {
		return nil, false
	}
	var cached cachedAuthResult
	if err := json.Unmarshal([]byte(raw), &cached); err != nil {
		log.Warnf("ext-auth cache value is corrupt, treating as a miss: %v", err)
		return nil, false
	}
	if !cached.Allow {
		return nil, false
	}
	return cached.UpstreamHeaders, true
}

// writeAuthCache stores an allow decision with a bounded TTL using SETNX, so on a
// concurrent double-miss the first writer wins and the entry is never overwritten.
// It is best-effort: every failure is logged and swallowed so a cache problem can
// never block a request the auth server already allowed.
func writeAuthCache(cache config.CacheConfig, cacheKey string, statusCode int, injected map[string]string) {
	value, err := json.Marshal(cachedAuthResult{Allow: true, Status: statusCode, UpstreamHeaders: injected})
	if err != nil {
		log.Warnf("ext-auth cache: failed to encode value: %v", err)
		return
	}
	setErr := cache.Client.SetNX(cacheKey, string(value), cache.TTL, func(response resp.Value) {
		if response.Error() != nil {
			log.Warnf("ext-auth cache: write failed: %v", response.Error())
		}
	})
	if setErr != nil {
		log.Warnf("ext-auth cache: write dispatch failed: %v", setErr)
	}
}
