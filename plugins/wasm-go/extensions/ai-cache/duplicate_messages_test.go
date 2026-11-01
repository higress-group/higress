// Copyright (c) 2026 Alibaba Group Holding Ltd.
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

var duplicateMessagesCacheConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"cache": map[string]interface{}{
			"type":        "redis",
			"serviceName": "redis.static",
			"servicePort": 6379,
			"timeout":     10000,
		},
		"cacheKeyStrategy":    "allQuestions",
		"cacheKeyFrom":        "messages.@reverse.0.content",
		"cacheValueFrom":      "choices.0.message.content",
		"enableSemanticCache": false,
	})
	return data
}()

// callCache sends one chat-completions request through a fresh plugin instance
// and reports the local response (if any) together with the number of redis
// lookups the plugin dispatched.
func callCache(t *testing.T, body string) (types.Action, int, string, bool, int) {
	t.Helper()
	host, status := test.NewTestHost(duplicateMessagesCacheConfig)
	defer host.Reset()
	require.Equal(t, types.OnPluginStartStatusOK, status)

	action := host.CallOnHttpRequestHeaders([][2]string{
		{":authority", "example.com"},
		{":path", "/api/chat"},
		{":method", "POST"},
		{"content-type", "application/json"},
	})
	require.Equal(t, types.HeaderStopIteration, action)

	action = host.CallOnHttpRequestBody([]byte(body))
	callouts := len(host.GetRedisCalloutAttributes())
	local := host.GetLocalResponse()
	if local == nil {
		return action, 0, "", false, callouts
	}
	return action, int(local.StatusCode), string(local.Data), true, callouts
}

// TestDuplicateMessagesMemberIsRejected pins the ambiguity rule for the cache
// path: the key is derived from the first "messages" occurrence (gjson) while
// the body is forwarded unchanged and the upstream parser keeps the last one.
// A body that declares the member twice would store one conversation's answer
// under another conversation's key and serve it on a later hit.
func TestDuplicateMessagesMemberIsRejected(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		body := `{"model":"qwen-turbo","messages":[{"role":"user","content":"first"}],"messages":[{"role":"user","content":"second"}]}`
		action, code, message, hasLocal, callouts := callCache(t, body)
		require.Equal(t, types.ActionPause, action)
		require.True(t, hasLocal, "the ambiguous body must be answered locally")
		require.Equal(t, 400, code)
		require.Contains(t, message, `duplicate "messages" member in request body`)
		require.Zero(t, callouts, "the ambiguous body must not reach the cache lookup")
	})
}

// TestSingleMessagesMemberIsAccepted is the control case: a normal request goes
// through the regular key derivation and cache lookup instead of being rejected
// locally.
func TestSingleMessagesMemberIsAccepted(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		body := `{"model":"qwen-turbo","messages":[{"role":"user","content":"what is the weather"}]}`
		action, code, _, hasLocal, callouts := callCache(t, body)
		require.Equal(t, types.ActionPause, action)
		require.False(t, hasLocal, "a single-member body must not be answered locally")
		require.Zero(t, code)
		require.GreaterOrEqual(t, callouts, 1, "the plugin must reach the cache lookup")
	})
}
