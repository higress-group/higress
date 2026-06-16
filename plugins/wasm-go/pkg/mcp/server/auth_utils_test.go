// Copyright (c) 2022 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package server

import (
	"encoding/base64"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubProvider implements SecuritySchemeProvider for ApplySecurity tests.
type stubProvider struct {
	schemes map[string]SecurityScheme
}

func (p *stubProvider) GetSecurityScheme(id string) (SecurityScheme, bool) {
	s, ok := p.schemes[id]
	return s, ok
}

func newProvider(schemes ...SecurityScheme) *stubProvider {
	m := make(map[string]SecurityScheme, len(schemes))
	for _, s := range schemes {
		m[s.ID] = s
	}
	return &stubProvider{schemes: m}
}

// mustParseURL helps build the ParsedURL field of AuthRequestContext.
func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)
	return u
}

func findHeader(headers [][2]string, key string) (string, bool) {
	for _, kv := range headers {
		if strings.EqualFold(kv[0], key) {
			return kv[1], true
		}
	}
	return "", false
}

func countHeader(headers [][2]string, key string) int {
	c := 0
	for _, kv := range headers {
		if strings.EqualFold(kv[0], key) {
			c++
		}
	}
	return c
}

// -----------------------------------------------------------------------------
// setOrReplaceHeader
// -----------------------------------------------------------------------------

func TestSetOrReplaceHeader_AppendsWhenAbsent(t *testing.T) {
	headers := [][2]string{{"X-Other", "1"}}
	setOrReplaceHeader(&headers, "X-New", "v")
	require.Len(t, headers, 2)
	v, ok := findHeader(headers, "X-New")
	require.True(t, ok)
	assert.Equal(t, "v", v)
}

func TestSetOrReplaceHeader_ReplacesCaseInsensitively(t *testing.T) {
	headers := [][2]string{
		{"Content-Type", "text/plain"},
		{"AUTHORIZATION", "old"},
	}
	setOrReplaceHeader(&headers, "authorization", "new")
	v, ok := findHeader(headers, "Authorization")
	require.True(t, ok)
	assert.Equal(t, "new", v)
	// Replacement is in-place, no duplicate header inserted.
	assert.Equal(t, 1, countHeader(headers, "Authorization"))
	assert.Len(t, headers, 2)
}

func TestSetOrReplaceHeader_PreservesOriginalKeyOnReplace(t *testing.T) {
	headers := [][2]string{{"X-Token", "old"}}
	setOrReplaceHeader(&headers, "x-token", "new")
	// Replacement updates value but keeps the original key casing.
	assert.Equal(t, [][2]string{{"X-Token", "new"}}, headers)
}

func TestSetOrReplaceHeader_FirstMatchWins(t *testing.T) {
	headers := [][2]string{
		{"X-Dup", "first"},
		{"x-dup", "second"},
	}
	setOrReplaceHeader(&headers, "X-Dup", "new")
	// Only the first occurrence is replaced; the second is left alone.
	assert.Equal(t, [][2]string{{"X-Dup", "new"}, {"x-dup", "second"}}, headers)
}

func TestSetOrReplaceHeader_IdempotentOnSecondCall(t *testing.T) {
	headers := [][2]string{}
	setOrReplaceHeader(&headers, "X-K", "v")
	setOrReplaceHeader(&headers, "X-K", "v")
	require.Len(t, headers, 1)
	assert.Equal(t, "v", headers[0][1])
}

// -----------------------------------------------------------------------------
// ApplySecurity — early returns / preconditions
// -----------------------------------------------------------------------------

func TestApplySecurity_EmptyIDIsNoOp(t *testing.T) {
	reqCtx := &AuthRequestContext{
		Headers:   [][2]string{{"X-Other", "x"}},
		ParsedURL: mustParseURL(t, "/p?a=1"),
	}
	err := ApplySecurity(SecurityRequirement{}, newProvider(), reqCtx)
	require.NoError(t, err)
	assert.Equal(t, [][2]string{{"X-Other", "x"}}, reqCtx.Headers)
	assert.Equal(t, "a=1", reqCtx.ParsedURL.RawQuery)
}

func TestApplySecurity_NilParsedURLReturnsError(t *testing.T) {
	reqCtx := &AuthRequestContext{}
	err := ApplySecurity(
		SecurityRequirement{ID: "x"},
		newProvider(SecurityScheme{ID: "x", Type: "apiKey", In: "header", Name: "X"}),
		reqCtx,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ParsedURL")
}

func TestApplySecurity_SchemeIDNotFound(t *testing.T) {
	reqCtx := &AuthRequestContext{ParsedURL: mustParseURL(t, "/p")}
	err := ApplySecurity(SecurityRequirement{ID: "missing"}, newProvider(), reqCtx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// -----------------------------------------------------------------------------
// ApplySecurity — apiKey × {header, query}
// -----------------------------------------------------------------------------

func TestApplySecurity_ApiKey_Header_DefaultCredential(t *testing.T) {
	reqCtx := &AuthRequestContext{
		Headers:   [][2]string{{"X-Other", "x"}},
		ParsedURL: mustParseURL(t, "/p"),
	}
	err := ApplySecurity(
		SecurityRequirement{ID: "K"},
		newProvider(SecurityScheme{
			ID: "K", Type: "apiKey", In: "header", Name: "X-Api-Key",
			DefaultCredential: "def",
		}),
		reqCtx,
	)
	require.NoError(t, err)
	v, ok := findHeader(reqCtx.Headers, "X-Api-Key")
	require.True(t, ok)
	assert.Equal(t, "def", v)
}

func TestApplySecurity_ApiKey_Header_ExplicitOverridesDefault(t *testing.T) {
	reqCtx := &AuthRequestContext{ParsedURL: mustParseURL(t, "/p")}
	err := ApplySecurity(
		SecurityRequirement{ID: "K", Credential: "override"},
		newProvider(SecurityScheme{
			ID: "K", Type: "apiKey", In: "header", Name: "X-Api-Key",
			DefaultCredential: "def",
		}),
		reqCtx,
	)
	require.NoError(t, err)
	v, _ := findHeader(reqCtx.Headers, "X-Api-Key")
	assert.Equal(t, "override", v)
}

func TestApplySecurity_ApiKey_Header_PassthroughBeatsExplicitAndDefault(t *testing.T) {
	reqCtx := &AuthRequestContext{
		ParsedURL:             mustParseURL(t, "/p"),
		PassthroughCredential: "from-client",
	}
	err := ApplySecurity(
		SecurityRequirement{ID: "K", Credential: "configured"},
		newProvider(SecurityScheme{
			ID: "K", Type: "apiKey", In: "header", Name: "X-Api-Key",
			DefaultCredential: "def",
		}),
		reqCtx,
	)
	require.NoError(t, err)
	v, _ := findHeader(reqCtx.Headers, "X-Api-Key")
	assert.Equal(t, "from-client", v, "passthrough wins over configured + default")
}

func TestApplySecurity_ApiKey_Header_ReplacesExisting(t *testing.T) {
	reqCtx := &AuthRequestContext{
		Headers:   [][2]string{{"x-api-key", "stale"}},
		ParsedURL: mustParseURL(t, "/p"),
	}
	err := ApplySecurity(
		SecurityRequirement{ID: "K", Credential: "fresh"},
		newProvider(SecurityScheme{
			ID: "K", Type: "apiKey", In: "header", Name: "X-Api-Key",
		}),
		reqCtx,
	)
	require.NoError(t, err)
	// Case-insensitive replace, no duplicate header.
	assert.Equal(t, 1, countHeader(reqCtx.Headers, "X-Api-Key"))
	v, _ := findHeader(reqCtx.Headers, "X-Api-Key")
	assert.Equal(t, "fresh", v)
}

func TestApplySecurity_ApiKey_NoCredentialAvailable(t *testing.T) {
	reqCtx := &AuthRequestContext{ParsedURL: mustParseURL(t, "/p")}
	err := ApplySecurity(
		SecurityRequirement{ID: "K"},
		newProvider(SecurityScheme{ID: "K", Type: "apiKey", In: "header", Name: "X"}),
		reqCtx,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no credential")
}

func TestApplySecurity_ApiKey_Header_MissingName(t *testing.T) {
	reqCtx := &AuthRequestContext{ParsedURL: mustParseURL(t, "/p")}
	err := ApplySecurity(
		SecurityRequirement{ID: "K", Credential: "v"},
		newProvider(SecurityScheme{ID: "K", Type: "apiKey", In: "header", Name: ""}),
		reqCtx,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name")
}

func TestApplySecurity_ApiKey_Query_AppendsToExistingQuery(t *testing.T) {
	reqCtx := &AuthRequestContext{ParsedURL: mustParseURL(t, "/p?existing=1")}
	err := ApplySecurity(
		SecurityRequirement{ID: "K", Credential: "secret"},
		newProvider(SecurityScheme{ID: "K", Type: "apiKey", In: "query", Name: "api_key"}),
		reqCtx,
	)
	require.NoError(t, err)
	q := reqCtx.ParsedURL.Query()
	assert.Equal(t, "secret", q.Get("api_key"))
	assert.Equal(t, "1", q.Get("existing"), "existing query params must be preserved")
}

func TestApplySecurity_ApiKey_Query_NoExistingQuery(t *testing.T) {
	reqCtx := &AuthRequestContext{ParsedURL: mustParseURL(t, "/p")}
	err := ApplySecurity(
		SecurityRequirement{ID: "K", Credential: "secret"},
		newProvider(SecurityScheme{ID: "K", Type: "apiKey", In: "query", Name: "api_key"}),
		reqCtx,
	)
	require.NoError(t, err)
	assert.Equal(t, "api_key=secret", reqCtx.ParsedURL.RawQuery)
}

func TestApplySecurity_ApiKey_Query_OverwritesExistingValue(t *testing.T) {
	reqCtx := &AuthRequestContext{ParsedURL: mustParseURL(t, "/p?api_key=stale&keep=me")}
	err := ApplySecurity(
		SecurityRequirement{ID: "K", Credential: "fresh"},
		newProvider(SecurityScheme{ID: "K", Type: "apiKey", In: "query", Name: "api_key"}),
		reqCtx,
	)
	require.NoError(t, err)
	q := reqCtx.ParsedURL.Query()
	assert.Equal(t, "fresh", q.Get("api_key"))
	assert.Equal(t, "me", q.Get("keep"))
	// Sanity: no duplicate api_key entries.
	assert.Len(t, q["api_key"], 1)
}

func TestApplySecurity_ApiKey_Query_MissingName(t *testing.T) {
	reqCtx := &AuthRequestContext{ParsedURL: mustParseURL(t, "/p")}
	err := ApplySecurity(
		SecurityRequirement{ID: "K", Credential: "v"},
		newProvider(SecurityScheme{ID: "K", Type: "apiKey", In: "query", Name: ""}),
		reqCtx,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name")
}

func TestApplySecurity_ApiKey_UnsupportedIn(t *testing.T) {
	reqCtx := &AuthRequestContext{ParsedURL: mustParseURL(t, "/p")}
	err := ApplySecurity(
		SecurityRequirement{ID: "K", Credential: "v"},
		newProvider(SecurityScheme{ID: "K", Type: "apiKey", In: "cookie", Name: "X"}),
		reqCtx,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported apiKey")
}

// -----------------------------------------------------------------------------
// ApplySecurity — http × {bearer, basic}
// -----------------------------------------------------------------------------

func TestApplySecurity_HttpBearer_AddsPrefix(t *testing.T) {
	reqCtx := &AuthRequestContext{ParsedURL: mustParseURL(t, "/p")}
	err := ApplySecurity(
		SecurityRequirement{ID: "B", Credential: "raw-token"},
		newProvider(SecurityScheme{ID: "B", Type: "http", Scheme: "bearer"}),
		reqCtx,
	)
	require.NoError(t, err)
	v, _ := findHeader(reqCtx.Headers, "Authorization")
	assert.Equal(t, "Bearer raw-token", v)
}

func TestApplySecurity_HttpBearer_RespectsExistingPrefix(t *testing.T) {
	reqCtx := &AuthRequestContext{ParsedURL: mustParseURL(t, "/p")}
	err := ApplySecurity(
		SecurityRequirement{ID: "B", Credential: "Bearer already-prefixed"},
		newProvider(SecurityScheme{ID: "B", Type: "http", Scheme: "bearer"}),
		reqCtx,
	)
	require.NoError(t, err)
	v, _ := findHeader(reqCtx.Headers, "Authorization")
	assert.Equal(t, "Bearer already-prefixed", v, "must not double-prefix")
}

func TestApplySecurity_HttpBasic_UserPassEncoded(t *testing.T) {
	reqCtx := &AuthRequestContext{ParsedURL: mustParseURL(t, "/p")}
	err := ApplySecurity(
		SecurityRequirement{ID: "B", Credential: "alice:s3cret"},
		newProvider(SecurityScheme{ID: "B", Type: "http", Scheme: "basic"}),
		reqCtx,
	)
	require.NoError(t, err)
	v, _ := findHeader(reqCtx.Headers, "Authorization")
	expected := "Basic " + base64.StdEncoding.EncodeToString([]byte("alice:s3cret"))
	assert.Equal(t, expected, v)
}

func TestApplySecurity_HttpBasic_PreEncodedToken(t *testing.T) {
	// No colon → treated as already-base64 token.
	reqCtx := &AuthRequestContext{ParsedURL: mustParseURL(t, "/p")}
	err := ApplySecurity(
		SecurityRequirement{ID: "B", Credential: "QWxpY2U6czNjcmV0"},
		newProvider(SecurityScheme{ID: "B", Type: "http", Scheme: "basic"}),
		reqCtx,
	)
	require.NoError(t, err)
	v, _ := findHeader(reqCtx.Headers, "Authorization")
	assert.Equal(t, "Basic QWxpY2U6czNjcmV0", v)
}

func TestApplySecurity_HttpBasic_RespectsExistingPrefix(t *testing.T) {
	reqCtx := &AuthRequestContext{ParsedURL: mustParseURL(t, "/p")}
	err := ApplySecurity(
		SecurityRequirement{ID: "B", Credential: "Basic ZXhpc3Rpbmc="},
		newProvider(SecurityScheme{ID: "B", Type: "http", Scheme: "basic"}),
		reqCtx,
	)
	require.NoError(t, err)
	v, _ := findHeader(reqCtx.Headers, "Authorization")
	assert.Equal(t, "Basic ZXhpc3Rpbmc=", v, "must not re-encode already-prefixed value")
}

func TestApplySecurity_HttpBasic_PassthroughTreatedAsTokenPart(t *testing.T) {
	reqCtx := &AuthRequestContext{
		ParsedURL:             mustParseURL(t, "/p"),
		PassthroughCredential: "QWxpY2U6czNjcmV0", // base64-encoded "alice:s3cret"
	}
	err := ApplySecurity(
		SecurityRequirement{ID: "B"},
		newProvider(SecurityScheme{ID: "B", Type: "http", Scheme: "basic"}),
		reqCtx,
	)
	require.NoError(t, err)
	v, _ := findHeader(reqCtx.Headers, "Authorization")
	// Passthrough path must NOT re-base64-encode; only adds the prefix.
	assert.Equal(t, "Basic QWxpY2U6czNjcmV0", v)
}

func TestApplySecurity_HttpBearer_Passthrough(t *testing.T) {
	reqCtx := &AuthRequestContext{
		ParsedURL:             mustParseURL(t, "/p"),
		PassthroughCredential: "client-token",
	}
	err := ApplySecurity(
		SecurityRequirement{ID: "B"},
		newProvider(SecurityScheme{ID: "B", Type: "http", Scheme: "bearer"}),
		reqCtx,
	)
	require.NoError(t, err)
	v, _ := findHeader(reqCtx.Headers, "Authorization")
	assert.Equal(t, "Bearer client-token", v)
}

func TestApplySecurity_HttpUnsupportedScheme(t *testing.T) {
	reqCtx := &AuthRequestContext{ParsedURL: mustParseURL(t, "/p")}
	err := ApplySecurity(
		SecurityRequirement{ID: "B", Credential: "x"},
		newProvider(SecurityScheme{ID: "B", Type: "http", Scheme: "digest"}),
		reqCtx,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported http scheme")
}

func TestApplySecurity_UnsupportedSchemeType(t *testing.T) {
	reqCtx := &AuthRequestContext{ParsedURL: mustParseURL(t, "/p")}
	err := ApplySecurity(
		SecurityRequirement{ID: "B", Credential: "x"},
		newProvider(SecurityScheme{ID: "B", Type: "oauth2"}),
		reqCtx,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported security scheme type")
}
