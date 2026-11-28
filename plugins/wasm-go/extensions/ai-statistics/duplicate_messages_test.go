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

var duplicateMessagesStatisticsConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"use_default_attributes": true,
	})
	return data
}()

func callStatistics(t *testing.T, body string) (types.Action, int, string, bool) {
	t.Helper()
	host, status := test.NewTestHost(duplicateMessagesStatisticsConfig)
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

// TestDuplicateMessagesMemberIsRejected pins the audit-log rule: the "messages"
// attribute written to the AI log (and the chat-round metric) is extracted from
// the first "messages" occurrence with gjson, while the upstream parser keeps
// the last one. A duplicated member would be logged with one conversation and
// served another, so the request must be rejected before anything is recorded.
func TestDuplicateMessagesMemberIsRejected(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		body := `{"model":"qwen-turbo","messages":[{"role":"user","content":"hello"}],"messages":[{"role":"user","content":"unsafe content"}]}`
		action, code, message, hasLocal := callStatistics(t, body)
		require.Equal(t, types.ActionPause, action)
		require.True(t, hasLocal, "the ambiguous body must be answered locally")
		require.Equal(t, 400, code)
		require.Contains(t, message, `duplicate "messages" member in request body`)
	})
}

// TestSingleMessagesMemberIsRecorded is the control case: a normal request is
// recorded and forwarded instead of being rejected locally.
func TestSingleMessagesMemberIsRecorded(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		body := `{"model":"qwen-turbo","messages":[{"role":"user","content":"hello"}]}`
		action, code, _, hasLocal := callStatistics(t, body)
		require.Equal(t, types.ActionContinue, action)
		require.False(t, hasLocal, "a single-member body must not be answered locally")
		require.Zero(t, code)
	})
}
