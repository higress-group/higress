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
	"encoding/json"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

var emptyConfig = json.RawMessage(`{}`)

func headers(path string) [][2]string {
	return [][2]string{
		{":authority", "example.com"},
		{":path", path},
		{":method", "POST"},
	}
}

// run feeds headers + body through the plugin and returns the (possibly
// modified) request body.
func run(t *testing.T, path, body string) []byte {
	host, status := test.NewTestHost(emptyConfig)
	defer host.Reset()
	require.Equal(t, types.OnPluginStartStatusOK, status)
	host.CallOnHttpRequestHeaders(headers(path))
	action := host.CallOnHttpRequestBody([]byte(body))
	require.Equal(t, types.ActionContinue, action)
	return host.GetRequestBody()
}

func TestFoldSystemIntoTopLevel(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("inline system moved to top-level system", func(t *testing.T) {
			out := run(t, "/v1/messages",
				`{"model":"m","max_tokens":16,"messages":[{"role":"user","content":"Hi"},{"role":"system","content":"Be terse."}]}`)
			var req map[string]interface{}
			require.NoError(t, json.Unmarshal(out, &req))
			require.Equal(t, "Be terse.", req["system"])
			msgs := req["messages"].([]interface{})
			require.Len(t, msgs, 1)
			require.Equal(t, "user", msgs[0].(map[string]interface{})["role"])
		})

		t.Run("existing top-level system preserved first", func(t *testing.T) {
			out := run(t, "/v1/messages",
				`{"model":"m","max_tokens":16,"system":"Keep this.","messages":[{"role":"user","content":"Hi"},{"role":"system","content":"Add this."}]}`)
			var req map[string]interface{}
			require.NoError(t, json.Unmarshal(out, &req))
			require.Equal(t, "Keep this.\n\nAdd this.", req["system"])
			require.Len(t, req["messages"].([]interface{}), 1)
		})

		t.Run("content blocks extracted", func(t *testing.T) {
			out := run(t, "/v1/messages",
				`{"model":"m","max_tokens":16,"messages":[{"role":"user","content":"Hi"},{"role":"system","content":[{"type":"text","text":"Block sys."}]}]}`)
			var req map[string]interface{}
			require.NoError(t, json.Unmarshal(out, &req))
			require.Equal(t, "Block sys.", req["system"])
		})

		t.Run("multiple system messages all folded", func(t *testing.T) {
			out := run(t, "/v1/messages",
				`{"model":"m","max_tokens":16,"messages":[{"role":"system","content":"First."},{"role":"user","content":"Hi"},{"role":"system","content":"Second."}]}`)
			var req map[string]interface{}
			require.NoError(t, json.Unmarshal(out, &req))
			require.Equal(t, "First.\n\nSecond.", req["system"])
			msgs := req["messages"].([]interface{})
			require.Len(t, msgs, 1)
			require.Equal(t, "user", msgs[0].(map[string]interface{})["role"])
		})

		t.Run("array top-level system gets a text block appended", func(t *testing.T) {
			out := run(t, "/v1/messages",
				`{"model":"m","max_tokens":16,"system":[{"type":"text","text":"Existing."}],"messages":[{"role":"user","content":"Hi"},{"role":"system","content":"Inline."}]}`)
			var req map[string]interface{}
			require.NoError(t, json.Unmarshal(out, &req))
			sys := req["system"].([]interface{})
			require.Len(t, sys, 2)
			require.Contains(t, string(out), "Inline.")
		})
	})
}

func TestPassThrough(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("no system message leaves body unchanged", func(t *testing.T) {
			body := `{"model":"m","max_tokens":16,"messages":[{"role":"user","content":"Hi"}]}`
			require.JSONEq(t, body, string(run(t, "/v1/messages", body)))
		})

		t.Run("chat/completions is not modified", func(t *testing.T) {
			// OpenAI endpoint legitimately allows system inside messages.
			body := `{"model":"m","max_tokens":16,"messages":[{"role":"system","content":"Sys"},{"role":"user","content":"Hi"}]}`
			require.JSONEq(t, body, string(run(t, "/v1/chat/completions", body)))
		})
	})
}
