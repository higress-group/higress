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

var duplicateMessagesHistoryConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"redis": map[string]interface{}{
			"serviceName": "redis.static",
			"servicePort": 6379,
			"timeout":     1000,
		},
	})
	return data
}()

func callHistory(t *testing.T, body string) (types.Action, int, string, bool) {
	t.Helper()
	host, status := test.NewTestHost(duplicateMessagesHistoryConfig)
	defer host.Reset()
	require.Equal(t, types.OnPluginStartStatusOK, status)

	_ = host.CallOnHttpRequestHeaders([][2]string{
		{":authority", "example.com"},
		{":path", "/v1/chat/completions"},
		{":method", "POST"},
		{"content-type", "application/json"},
		{"authorization", "test-token"},
	})
	action := host.CallOnHttpRequestBody([]byte(body))
	local := host.GetLocalResponse()
	if local == nil {
		return action, 0, "", false
	}
	return action, int(local.StatusCode), string(local.Data), true
}

// TestDuplicateMessagesMemberIsRejected pins the ambiguity rule for the history
// path: the question and the conversation merged into the body are read from
// the first "messages" occurrence (gjson), while the upstream parser keeps the
// last one. A duplicated member would be recorded with one conversation and
// served another, so the request must be rejected before the Redis lookup.
func TestDuplicateMessagesMemberIsRejected(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		body := `{"model":"gpt-4","messages":[{"role":"user","content":"first"}],"messages":[{"role":"user","content":"second"}]}`
		action, code, message, hasLocal := callHistory(t, body)
		require.Equal(t, types.ActionPause, action)
		require.True(t, hasLocal, "the ambiguous body must be answered locally")
		require.Equal(t, 400, code)
		require.Contains(t, message, `duplicate "messages" member in request body`)
	})
}

// TestSingleMessagesMemberIsAccepted is the control case: a normal request
// goes through the regular question extraction and Redis lookup instead of
// being rejected locally.
func TestSingleMessagesMemberIsAccepted(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		body := `{"model":"gpt-4","messages":[{"role":"user","content":"hello"}]}`
		_, code, _, hasLocal := callHistory(t, body)
		require.False(t, hasLocal, "a single-member body must not be answered locally")
		require.Zero(t, code)
	})
}
