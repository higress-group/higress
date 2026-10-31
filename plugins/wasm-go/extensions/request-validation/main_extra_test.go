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
	"encoding/json"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

// === helpers =============================================================

func mustConfig(t *testing.T, m map[string]interface{}) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(m)
	require.NoError(t, err)
	return b
}

// invalidHeaderSchemaCfg supplies a header_schema whose JSON syntax is valid
// (so AddResource at parseConfig time succeeds) but whose schema keywords are
// not (so Compile at request time fails). Used to drive the
// "compile schema failed" log+continue branch shared by both
// onHttpRequestHeaders and onHttpRequestBody.
func invalidHeaderSchemaCfg(t *testing.T) json.RawMessage {
	return mustConfig(t, map[string]interface{}{
		"header_schema": `{"type": "invalid_type", "properties": {}}`,
		"enable_oas3":   true,
	})
}

func invalidBodySchemaCfg(t *testing.T) json.RawMessage {
	return mustConfig(t, map[string]interface{}{
		"body_schema": `{"type": "invalid_type", "properties": {}}`,
		"enable_oas3": true,
	})
}

// === Module A — parseConfig rejected_code edge defaults ==================
//
// parseConfig is 94.1% in baseline. The `else` branch at main.go:117-119
// (defaultRejectedCode = 403) is unreached because every existing fixture
// supplies an in-range rejected_code (400 / 422 / 403). The two failure
// modes — code unset (== 0) and code out of valid HTTP range — share the
// same default, but only the second is a real config error worth pinning;
// the first protects users who omit rejected_code entirely.

func TestParseConfig_RejectedCodeOmitted_DefaultsTo403(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		cfg := mustConfig(t, map[string]interface{}{
			"header_schema": `{"type":"object","required":["x-required"]}`,
			"enable_oas3":   true,
			// rejected_code intentionally omitted — defaultRejectedCode = 403
		})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		conf, err := host.GetMatchConfig()
		require.NoError(t, err)
		require.NotNil(t, conf)
		require.Equal(t, uint32(403), conf.(*Config).rejectedCode)
	})
}

func TestParseConfig_RejectedCodeOutOfRange_DefaultsTo403(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		cfg := mustConfig(t, map[string]interface{}{
			"header_schema": `{"type":"object","required":["x-required"]}`,
			"enable_oas3":   true,
			"rejected_code": 999, // > 600 → falls to default
		})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		conf, err := host.GetMatchConfig()
		require.NoError(t, err)
		require.NotNil(t, conf)
		require.Equal(t, uint32(403), conf.(*Config).rejectedCode)
	})
}

// === Module B — onHttpRequestHeaders compile-error pass-through ==========
//
// The compile-error branch at main.go:152-156 is uncovered: every existing
// fixture provides a header_schema that both AddResource and Compile accept.
// invalidSchemaConfig in main_test.go uses it only to assert parse success,
// not to drive a request through it. Behavior contract: log error and
// ActionContinue (fail-open) — distinct from validate-failure which is 4xx.

func TestOnHttpRequestHeaders_CompileFailure_PassThrough(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(invalidHeaderSchemaCfg(t))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/api/test"},
			{":method", "GET"},
			{"content-type", "application/json"},
		})
		require.Equal(t, types.ActionContinue, action)
		require.Nil(t, host.GetLocalResponse())
	})
}

// === Module C — onHttpRequestBody parse / compile failure paths ==========

func TestOnHttpRequestBody_MalformedJSON(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		cfg := mustConfig(t, map[string]interface{}{
			"body_schema":   `{"type":"object","required":["name"]}`,
			"enable_oas3":   true,
			"rejected_code": 422,
			"rejected_msg":  "Invalid request body",
		})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/api/test"},
			{":method", "POST"},
		})
		require.Equal(t, types.ActionContinue, action)

		action = host.CallOnHttpRequestBody([]byte("not a json {{{"))
		require.Equal(t, types.ActionPause, action)
		response := host.GetLocalResponse()
		require.NotNil(t, response)
		require.Equal(t, uint32(422), response.StatusCode)
		require.Equal(t, "Invalid request body", string(response.Data))
	})
}

func TestOnHttpRequestBodyTopLevelTypes(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		drafts := []struct {
			name      string
			configKey string
		}{
			{name: "draft4", configKey: "enable_swagger"},
			{name: "draft7", configKey: "enable_oas3"},
		}
		typesToTest := []struct {
			name        string
			validBody   string
			invalidBody string
		}{
			{name: "object", validBody: `{"name":"example"}`, invalidBody: `[]`},
			{name: "array", validBody: `[1,"two"]`, invalidBody: `{}`},
			{name: "string", validBody: `"hello"`, invalidBody: `1`},
			{name: "number", validBody: `42.5`, invalidBody: `"42"`},
			{name: "boolean", validBody: `true`, invalidBody: `"true"`},
			{name: "null", validBody: `null`, invalidBody: `false`},
		}

		for _, draft := range drafts {
			for _, bodyType := range typesToTest {
				t.Run(draft.name+"/"+bodyType.name, func(t *testing.T) {
					cfg := mustConfig(t, map[string]interface{}{
						"body_schema":   `{"type":"` + bodyType.name + `"}`,
						draft.configKey: true,
						"rejected_code": 422,
						"rejected_msg":  "Invalid request body",
					})
					bodies := []struct {
						name         string
						body         string
						expectAccept bool
					}{
						{name: "accept", body: bodyType.validBody, expectAccept: true},
						{name: "reject", body: bodyType.invalidBody},
					}
					for _, bodyCase := range bodies {
						t.Run(bodyCase.name, func(t *testing.T) {
							host, status := test.NewTestHost(cfg)
							defer host.Reset()
							require.Equal(t, types.OnPluginStartStatusOK, status)

							action := host.CallOnHttpRequestHeaders([][2]string{
								{":authority", "example.com"},
								{":path", "/api/test"},
								{":method", "POST"},
							})
							require.Equal(t, types.ActionContinue, action)
							action = host.CallOnHttpRequestBody([]byte(bodyCase.body))

							response := host.GetLocalResponse()
							if bodyCase.expectAccept {
								require.Equal(t, types.ActionContinue, action)
								require.Nil(t, response)
							} else {
								require.Equal(t, types.ActionPause, action)
								require.NotNil(t, response)
								require.Equal(t, uint32(422), response.StatusCode)
								require.Equal(t, "Invalid request body", string(response.Data))
							}
							host.CompleteHttp()
						})
					}
				})
			}
		}
	})
}

func TestOnHttpRequestBody_CompileFailure_PassThrough(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(invalidBodySchemaCfg(t))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/api/test"},
			{":method", "POST"},
		})
		require.Equal(t, types.ActionContinue, action)

		action = host.CallOnHttpRequestBody([]byte(`{"name":"ok"}`))
		require.Equal(t, types.ActionContinue, action)
		require.Nil(t, host.GetLocalResponse())
	})
}

// === Module D — both-schema interaction ==================================
//
// Coverage for `bothValidationConfig` only exercises parseConfig; nothing
// drives a request through both header + body schema in sequence. Pin the
// happy interaction so future refactors don't accidentally short-circuit
// body validation when header validation passes.

func TestOnHttpRequestBody_AfterHeaderValidationPass_BodyValidates(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		cfg := mustConfig(t, map[string]interface{}{
			"header_schema": `{
				"type":"object",
				"properties":{"content-type":{"type":"string"}},
				"required":["content-type"]
			}`,
			"body_schema": `{
				"type":"object",
				"properties":{"id":{"type":"integer"}},
				"required":["id"]
			}`,
			"enable_oas3":   true,
			"rejected_code": 400,
			"rejected_msg":  "validation failed",
		})
		host, status := test.NewTestHost(cfg)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		// Headers pass.
		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/api/test"},
			{":method", "POST"},
			{"content-type", "application/json"},
		})
		require.Equal(t, types.ActionContinue, action)
		require.Nil(t, host.GetLocalResponse())

		// Body fails schema (missing required `id`).
		action = host.CallOnHttpRequestBody([]byte(`{"name":"x"}`))
		require.Equal(t, types.ActionPause, action)

		resp := host.GetLocalResponse()
		require.NotNil(t, resp)
		require.Equal(t, uint32(400), resp.StatusCode)
		require.Equal(t, "validation failed", string(resp.Data))
	})
}
