// Copyright (c) 2025 Alibaba Group Holding Ltd.
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

package tests

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/roundtripper"
	"github.com/alibaba/higress/v2/test/e2e/conformance/utils/suite"
)

func init() {
	Register(WasmPluginsExtAuthCache)
}

const (
	// extAuthCacheDefaultHost routes to the plugin config using the default cache key.
	extAuthCacheDefaultHost = "ext-auth-cache-default.example.com"
	// extAuthCacheKeyFieldsHost routes to the plugin config using cache.key_fields.
	extAuthCacheKeyFieldsHost = "ext-auth-cache-keyfields.example.com"
	// extAuthUUIDHeader carries the authorization server's per-call uuid, mapped by
	// ext-auth into the upstream request. The echo-server backend reflects it back, so
	// it is the client-visible signal of whether the auth server was really called.
	extAuthUUIDHeader = "x-auth-uuid"
)

// extAuthCacheCall is one client request to the gateway. query is the raw query
// string (without "?"); headers are sent verbatim.
type extAuthCacheCall struct {
	host    string
	path    string
	query   string
	headers map[string]string
}

var WasmPluginsExtAuthCache = suite.ConformanceTest{
	ShortName:   "WasmPluginsExtAuthCache",
	Description: "ext-auth caches the allow decision; cache.key_fields narrows the key so an unlisted per-request credential still hits while a listed field misses.",
	Manifests:   []string{"tests/go-wasm-ext-auth-cache.yaml"},
	Features:    []suite.SupportedFeature{suite.WASMGoConformanceFeature},
	Test: func(t *testing.T, suite *suite.ConformanceTestSuite) {
		timeout := suite.TimeoutConfig.MaxTimeToConsistency

		// callOnce issues a single request and returns the reflected x-auth-uuid, the
		// HTTP status, and any transport error. It does not retry.
		callOnce := func(c extAuthCacheCall) (string, int, error) {
			headers := make(map[string][]string, len(c.headers))
			for name, value := range c.headers {
				headers[name] = []string{value}
			}
			req := roundtripper.Request{
				Method:  "GET",
				Host:    c.host,
				URL:     url.URL{Scheme: "http", Host: suite.GatewayAddress, Path: c.path, RawQuery: c.query},
				Headers: headers,
			}
			cReq, cRes, err := suite.RoundTripper.CaptureRoundTrip(req)
			if err != nil {
				return "", 0, err
			}
			return extAuthCacheHeaderValue(cReq.Headers, extAuthUUIDHeader), cRes.StatusCode, nil
		}

		// settle drives one cache key until the chain is fully ready and the cached
		// value has stabilized: it returns the uuid once two consecutive calls agree.
		// Two equal uuids can only happen when the second call was a cache hit replaying
		// the first, which also means Redis and the auth server are both live. Waiting
		// for stability (rather than trusting the first uuid) absorbs the race where the
		// async cache write has not landed yet and an immediate second call would miss.
		settle := func(c extAuthCacheCall) string {
			deadline := time.Now().Add(timeout)
			var prev string
			for {
				uuid, status, err := callOnce(c)
				if err == nil && status == 200 && uuid != "" {
					if uuid == prev {
						return uuid
					}
					prev = uuid
				} else {
					prev = ""
				}
				if time.Now().After(deadline) {
					t.Fatalf("cache never settled for %+v: last status=%d uuid=%q err=%v", c, status, uuid, err)
				}
				time.Sleep(300 * time.Millisecond)
			}
		}

		// expectSame asserts a cache hit: the request replays the settled uuid, proving
		// the authorization server was not called again.
		expectSame := func(baseline string, c extAuthCacheCall, desc string) {
			deadline := time.Now().Add(timeout)
			for {
				uuid, status, err := callOnce(c)
				if err == nil && status == 200 && uuid == baseline {
					return
				}
				if time.Now().After(deadline) {
					t.Fatalf("%s: expected a cache hit replaying uuid %q, got status=%d uuid=%q err=%v", desc, baseline, status, uuid, err)
				}
				time.Sleep(300 * time.Millisecond)
			}
		}

		// expectDifferent asserts a cache miss: the request falls through to the auth
		// server and gets a fresh uuid, proving it landed on a different cache entry.
		expectDifferent := func(baseline string, c extAuthCacheCall, desc string) {
			deadline := time.Now().Add(timeout)
			for {
				uuid, status, err := callOnce(c)
				if err == nil && status == 200 && uuid != "" && uuid != baseline {
					return
				}
				if time.Now().After(deadline) {
					t.Fatalf("%s: expected a cache miss with a fresh uuid != %q, got status=%d uuid=%q err=%v", desc, baseline, status, uuid, err)
				}
				time.Sleep(300 * time.Millisecond)
			}
		}

		// --- Default cache key: method + path + every forwarded header (credentials
		// included). Seeding also warms the route, plugin, auth server and Redis. ---
		defaultSeed := extAuthCacheCall{
			host:    extAuthCacheDefaultHost,
			path:    "/default",
			headers: map[string]string{"Authorization": "Bearer aaa"},
		}
		base := settle(defaultSeed)

		// An identical request hits the cache and replays the same uuid.
		expectSame(base, defaultSeed, "default key: identical request")

		// A different credential is part of the default key, so it misses.
		expectDifferent(base, extAuthCacheCall{
			host:    extAuthCacheDefaultHost,
			path:    "/default",
			headers: map[string]string{"Authorization": "Bearer zzz"},
		}, "default key: changed credential")

		// A different path is part of the default key, so it misses.
		expectDifferent(base, extAuthCacheCall{
			host:    extAuthCacheDefaultHost,
			path:    "/default-other",
			headers: map[string]string{"Authorization": "Bearer aaa"},
		}, "default key: changed path")

		// --- cache.key_fields: [query userId, header x-app-key]. Authorization is NOT
		// listed, so it no longer affects the key. The seed also confirms a mixed-case
		// X-App-Key matches the lower-cased configured field name. ---
		keyFieldsSeed := extAuthCacheCall{
			host:  extAuthCacheKeyFieldsHost,
			path:  "/api",
			query: "userId=1",
			headers: map[string]string{
				"Authorization": "Bearer aaa",
				"X-App-Key":     "k1",
			},
		}
		kfBase := settle(keyFieldsSeed)

		// Core value of key_fields: changing an unlisted, per-request credential still
		// hits the same entry instead of defeating the cache.
		expectSame(kfBase, extAuthCacheCall{
			host:  extAuthCacheKeyFieldsHost,
			path:  "/api",
			query: "userId=1",
			headers: map[string]string{
				"Authorization": "Bearer bbb",
				"X-App-Key":     "k1",
			},
		}, "key_fields: changed unlisted credential")

		// The query string is not folded into the key wholesale; an unlisted query
		// parameter is ignored, so this still hits.
		expectSame(kfBase, extAuthCacheCall{
			host:  extAuthCacheKeyFieldsHost,
			path:  "/api",
			query: "userId=1&foo=bar",
			headers: map[string]string{
				"Authorization": "Bearer aaa",
				"X-App-Key":     "k1",
			},
		}, "key_fields: added unlisted query parameter")

		// Changing the listed query field userId selects a different entry, so it misses.
		expectDifferent(kfBase, extAuthCacheCall{
			host:  extAuthCacheKeyFieldsHost,
			path:  "/api",
			query: "userId=2",
			headers: map[string]string{
				"Authorization": "Bearer aaa",
				"X-App-Key":     "k1",
			},
		}, "key_fields: changed listed query field")

		// Changing the listed header field x-app-key selects a different entry, so it
		// misses.
		expectDifferent(kfBase, extAuthCacheCall{
			host:  extAuthCacheKeyFieldsHost,
			path:  "/api",
			query: "userId=1",
			headers: map[string]string{
				"Authorization": "Bearer aaa",
				"X-App-Key":     "k2",
			},
		}, "key_fields: changed listed header field")
	},
}

// extAuthCacheHeaderValue looks up a header case-insensitively in the request the
// echo-server backend captured, returning the first value or "" if absent.
func extAuthCacheHeaderValue(headers map[string][]string, name string) string {
	name = strings.ToLower(name)
	for k, values := range headers {
		if strings.ToLower(k) == name && len(values) > 0 {
			return values[0]
		}
	}
	return ""
}
