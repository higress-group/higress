// Copyright (c) 2022 Alibaba Group Holding Ltd.
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
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

// TestOnHttpRequestBodyNonObject covers bodies that are valid JSON but not an object.
// The configured schema is an object schema, so these bodies cannot satisfy it and must
// be rejected. They used to fail the map unmarshal inside onHttpRequestBody and take the
// fail-open branch, which let a client bypass the schema by wrapping the payload in an
// array (or by sending any other non-object JSON value).
func TestOnHttpRequestBodyNonObject(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		nonObjectBodies := map[string]string{
			"array":   `[{"name": "ok"}]`,
			"string":  `"just a string"`,
			"number":  `123`,
			"boolean": `true`,
			"null":    `null`,
		}

		for name, body := range nonObjectBodies {
			t.Run("non-object body - "+name, func(t *testing.T) {
				host, status := test.NewTestHost(bodyValidationConfig)
				defer host.Reset()
				require.Equal(t, types.OnPluginStartStatusOK, status)

				action := host.CallOnHttpRequestHeaders([][2]string{
					{":authority", "test.com"},
					{":path", "/api/test"},
					{":method", "POST"},
				})
				require.Equal(t, types.ActionContinue, action)

				action = host.CallOnHttpRequestBody([]byte(body))
				require.Equal(t, types.ActionPause, action,
					"a non-object body must not bypass the configured object schema")
				require.Equal(t, types.ActionPause, host.GetHttpStreamAction())

				localResponse := host.GetLocalResponse()
				require.NotNil(t, localResponse)
				require.Equal(t, uint32(422), localResponse.StatusCode)
				require.Equal(t, "Invalid request body", string(localResponse.Data))

				host.CompleteHttp()
			})
		}

		// Bodies that are not JSON values at all (including an empty body) keep the
		// plugin's fail-open contract: the schema cannot judge them, and the existing
		// TestOnHttpRequestBody_InvalidJson_PassThrough pins this behaviour.
		for name, body := range map[string]string{
			"plain text": `not json at all`,
			"empty":      ``,
		} {
			t.Run("non-JSON body fails open - "+name, func(t *testing.T) {
				host, status := test.NewTestHost(bodyValidationConfig)
				defer host.Reset()
				require.Equal(t, types.OnPluginStartStatusOK, status)

				action := host.CallOnHttpRequestHeaders([][2]string{
					{":authority", "test.com"},
					{":path", "/api/test"},
					{":method", "POST"},
				})
				require.Equal(t, types.ActionContinue, action)

				action = host.CallOnHttpRequestBody([]byte(body))
				require.Equal(t, types.ActionContinue, action)
				require.Nil(t, host.GetLocalResponse())

				host.CompleteHttp()
			})
		}
	})
}
