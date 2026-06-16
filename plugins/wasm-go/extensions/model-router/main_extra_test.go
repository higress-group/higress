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

package main

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

// === helpers =============================================================

func mustConfigBytes(t *testing.T, m map[string]interface{}) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(m)
	require.NoError(t, err)
	return b
}

// === Module A — onHttpRequestHeaders edges ==============================
//
// onHttpRequestHeaders is 87.5% in baseline. Existing main_test.go drives
// the bare-suffix match and miss paths but never the query-string strip
// (main.go:126-128) or the explicit `*` wildcard short-circuit
// (main.go:132). Both are part of the documented suffix-matching contract.

// Path with `?...` query string must be stripped before suffix matching.
// Without the strip, `/v1/chat/completions?stream=true` would be compared
// against `/v1/chat/completions` and miss — pin the strip.
func TestOnHttpRequestHeaders_PathWithQueryStripped(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(basicConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/v1/chat/completions?stream=true&debug=1"},
			{":method", "POST"},
			{"content-type", "application/json"},
		})
		require.Equal(t, types.HeaderStopIteration, action)
	})
}

// `*` is the explicit catch-all suffix — must enable for any path. Pins the
// `suffix == "*"` short-circuit so a future refactor that switched to pure
// HasSuffix matching (where `"*"` would only match a literal `*` ending)
// would fail this test.
func TestOnHttpRequestHeaders_WildcardSuffixEnablesAnyPath(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		cfg := mustConfigBytes(t, map[string]interface{}{
			"modelKey":           "model",
			"addProviderHeader":  "x-provider",
			"modelToHeader":      "x-model",
			"enableOnPathSuffix": []string{"*"},
		})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/anything/at/all"},
			{":method", "POST"},
			{"content-type", "application/json"},
		})
		require.Equal(t, types.HeaderStopIteration, action)
	})
}

// === Module B — onHttpRequestBody dispatch fall-through =================
//
// onHttpRequestBody is 75.0% in baseline. The `else` fall-through at
// main.go:161-163 — content-type matched the path-suffix gate but is
// neither application/json nor multipart/form-data — is uncovered. The
// existing "do not process for unsupported content-type" test only checks
// the headers phase; nothing exercises the body-side neutral exit.
func TestOnHttpRequestBody_UnsupportedContentType_PassThrough(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(basicConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/v1/chat/completions"},
			{":method", "POST"},
			{"content-type", "text/plain"},
		})
		action := host.CallOnHttpRequestBody([]byte("hello"))
		require.Equal(t, types.ActionContinue, action)
		// Neither header was injected.
		_, found := getHeader(host.GetRequestHeaders(), "x-model")
		require.False(t, found)
		_, found = getHeader(host.GetRequestHeaders(), "x-provider")
		require.False(t, found)
	})
}

// === Module C — handleJsonBody edges ====================================
//
// handleJsonBody is 83.3%. Two reachable uncovered branches:
//   - main.go:204-207 — invalid JSON → log + ActionContinue (fail-open)
//   - main.go:264-266 — modelValue contains no `/` while addProviderHeader
//     is configured → SplitN returns 1 element, provider rewrite skipped

// Malformed JSON body must NOT block the request. The plugin's contract is
// that a bad payload is the upstream's problem, not this filter's.
func TestHandleJsonBody_InvalidJson_PassThrough(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(basicConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/v1/chat/completions"},
			{":method", "POST"},
			{"content-type", "application/json"},
		})
		action := host.CallOnHttpRequestBody([]byte("{not json"))
		require.Equal(t, types.ActionContinue, action)

		// Crucially: no header injection on bad body — provider rewrite
		// must never fire on a body that wasn't validated.
		_, found := getHeader(host.GetRequestHeaders(), "x-provider")
		require.False(t, found)
		_, found = getHeader(host.GetRequestHeaders(), "x-model")
		require.False(t, found)
	})
}

// `model: "plain-model"` (no `/` separator) with addProviderHeader set ⇒
// modelToHeader still fires (separate concern), but the provider-split
// block enters and exits via the `else` log branch without rewriting body
// or setting addProviderHeader. Pins the asymmetry between the two header
// configs in the no-slash case.
func TestHandleJsonBody_ModelWithoutSlash_AddProviderConfigured(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(basicConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/v1/chat/completions"},
			{":method", "POST"},
			{"content-type", "application/json"},
		})
		action := host.CallOnHttpRequestBody([]byte(`{"model":"plain-model","messages":[]}`))
		require.Equal(t, types.ActionContinue, action)

		// modelToHeader fires unconditionally (line 246-248).
		hv, found := getHeader(host.GetRequestHeaders(), "x-model")
		require.True(t, found)
		require.Equal(t, "plain-model", hv)
		// addProviderHeader path skipped — no x-provider.
		_, found = getHeader(host.GetRequestHeaders(), "x-provider")
		require.False(t, found)
	})
}

// === Module D — handleMultipartBody edges ===============================
//
// handleMultipartBody is 73.8% — the largest single gap in main.go. Four
// reachable uncovered branches addressed below; the writer.CreatePart and
// io.ReadAll error paths require host-injected i/o failures and are not
// covered.

// content-type is structurally invalid (`boundary` param with no `=value`)
// ⇒ mime.ParseMediaType returns "invalid media parameter" at main.go:273-277
// → log + ActionContinue. Distinct from the NoBoundary case below: this
// fails parsing entirely; that one parses but the param is absent.
func TestHandleMultipartBody_BadContentType(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(basicConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/v1/chat/completions"},
			{":method", "POST"},
			{"content-type", "multipart/form-data; boundary"}, // missing `=value`
		})
		action := host.CallOnHttpRequestBody([]byte("ignored"))
		require.Equal(t, types.ActionContinue, action)
		_, found := getHeader(host.GetRequestHeaders(), "x-model")
		require.False(t, found)
	})
}

// Body advances past the boundary delimiter into a malformed MIME header
// (no colon) ⇒ NextPart returns a non-EOF error at main.go:296-299 →
// log + ActionContinue. Existing test only sees clean parts followed by
// EOF; the inner-loop error path was unreached.
func TestHandleMultipartBody_NextPartError(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(basicConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/v1/chat/completions"},
			{":method", "POST"},
			{"content-type", "multipart/form-data; boundary=xxx"},
		})
		// Boundary delimiter is correct, but the part header that follows
		// has no colon — NextPart fails with "malformed MIME header".
		body := []byte("--xxx\r\nbroken header here\r\n\r\nbody\r\n--xxx--\r\n")
		action := host.CallOnHttpRequestBody(body)
		require.Equal(t, types.ActionContinue, action)
	})
}

// content-type contains the literal `multipart/form-data` (so the dispatch
// at main.go:159 picks the multipart handler) but no `boundary` parameter
// ⇒ params["boundary"] miss at main.go:278-282 → log + ActionContinue.
func TestHandleMultipartBody_NoBoundary(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(basicConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/v1/chat/completions"},
			{":method", "POST"},
			{"content-type", "multipart/form-data"}, // no `boundary=` param
		})
		action := host.CallOnHttpRequestBody([]byte("ignored"))
		require.Equal(t, types.ActionContinue, action)
		// No header rewrites — handler exited before the parts loop.
		_, found := getHeader(host.GetRequestHeaders(), "x-model")
		require.False(t, found)
	})
}

// model field present but value has no `/` ⇒ provider-split block enters
// the `else` log branch (main.go:343) and falls through to the bottom
// original-write path; the model field is round-tripped unchanged. modified
// stays false so the body is not replaced. modelToHeader still fires
// (mirrors the JSON-side asymmetry above).
func TestHandleMultipartBody_ModelWithoutSlash(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(basicConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)
		modelW, err := writer.CreateFormField("model")
		require.NoError(t, err)
		_, err = modelW.Write([]byte("plain-model"))
		require.NoError(t, err)
		promptW, err := writer.CreateFormField("prompt")
		require.NoError(t, err)
		_, err = promptW.Write([]byte("hi"))
		require.NoError(t, err)
		require.NoError(t, writer.Close())

		host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/v1/chat/completions"},
			{":method", "POST"},
			{"content-type", "multipart/form-data; boundary=" + writer.Boundary()},
		})
		action := host.CallOnHttpRequestBody(buf.Bytes())
		require.Equal(t, types.ActionContinue, action)

		// modelToHeader fires before the split logic.
		hv, found := getHeader(host.GetRequestHeaders(), "x-model")
		require.True(t, found)
		require.Equal(t, "plain-model", hv)
		// No provider — split path skipped via the else branch.
		_, found = getHeader(host.GetRequestHeaders(), "x-provider")
		require.False(t, found)
	})
}

// addProviderHeader empty (only modelToHeader configured) ⇒ the entire
// provider-split block at main.go:316-345 is skipped; the model field is
// written through the bottom "original part" branch unchanged even with
// `provider/model` form. Pins the `addProviderHeader == ""` short-circuit.
func TestHandleMultipartBody_NoAddProviderHeader(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		cfg := mustConfigBytes(t, map[string]interface{}{
			"modelKey":      "model",
			"modelToHeader": "x-model",
			"enableOnPathSuffix": []string{
				"/v1/chat/completions",
			},
		})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)
		modelW, err := writer.CreateFormField("model")
		require.NoError(t, err)
		_, err = modelW.Write([]byte("openai/gpt-4o"))
		require.NoError(t, err)
		require.NoError(t, writer.Close())

		host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/v1/chat/completions"},
			{":method", "POST"},
			{"content-type", "multipart/form-data; boundary=" + writer.Boundary()},
		})
		action := host.CallOnHttpRequestBody(buf.Bytes())
		require.Equal(t, types.ActionContinue, action)

		// Header carries the FULL value — no split happened.
		hv, found := getHeader(host.GetRequestHeaders(), "x-model")
		require.True(t, found)
		require.Equal(t, "openai/gpt-4o", hv)
		// No provider header was ever requested.
		_, found = getHeader(host.GetRequestHeaders(), "x-provider")
		require.False(t, found)
	})
}
