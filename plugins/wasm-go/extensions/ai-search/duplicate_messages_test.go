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

var duplicateMessagesSearchConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"searchFrom": []map[string]interface{}{
			{
				"type":        "bing",
				"apiKey":      "test-key",
				"serviceName": "bing.static",
				"servicePort": 443,
			},
		},
	})
	return data
}()

func callSearch(t *testing.T, body string) (types.Action, int, string, bool) {
	t.Helper()
	host, status := test.NewTestHost(duplicateMessagesSearchConfig)
	defer host.Reset()
	require.Equal(t, types.OnPluginStartStatusOK, status)

	_ = host.CallOnHttpRequestHeaders([][2]string{
		{":authority", "example.com"},
		{":path", "/v1/chat/completions"},
		{":method", "POST"},
		{"content-type", "application/json"},
	})
	action := host.CallOnHttpRequestBody([]byte(body))
	local := host.GetLocalResponse()
	if local == nil {
		return action, 0, "", false
	}
	return action, int(local.StatusCode), string(local.Data), true
}

// TestDuplicateMessagesMemberIsRejected pins the ambiguity rule for the search
// rewrite path: the query index is taken from the first "messages" occurrence
// (gjson) and the result is written back to that occurrence with sjson, while
// the upstream parser keeps the last one. A duplicated member would therefore
// have the search results written into the occurrence the model never sees.
func TestDuplicateMessagesMemberIsRejected(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		body := `{"model":"gpt-4","messages":[{"role":"user","content":"first"}],"messages":[{"role":"user","content":"second"}]}`
		action, code, message, hasLocal := callSearch(t, body)
		require.Equal(t, types.ActionPause, action)
		require.True(t, hasLocal, "the ambiguous body must be answered locally")
		require.Equal(t, 400, code)
		require.Contains(t, message, `duplicate "messages" member in request body`)
	})
}

// TestSingleMessagesMemberIsAccepted is the control case: a normal request goes
// through the regular search path instead of being rejected locally.
func TestSingleMessagesMemberIsAccepted(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		body := `{"model":"gpt-4","messages":[{"role":"user","content":"hello"}]}`
		action, code, _, hasLocal := callSearch(t, body)
		require.Equal(t, types.ActionPause, action)
		require.False(t, hasLocal, "a single-member body must not be answered locally")
		require.Zero(t, code)
	})
}
