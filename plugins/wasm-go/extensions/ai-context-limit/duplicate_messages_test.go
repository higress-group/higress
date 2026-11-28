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

var duplicateMessagesConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"max_context_tokens": 100000,
		"error_status_code":  413,
		"buffer_ratio":       1.0,
	})
	return data
}()

func callContextLimit(t *testing.T, body string) (types.Action, uint32, string) {
	t.Helper()
	host, status := test.NewTestHost(duplicateMessagesConfig)
	defer host.Reset()
	require.Equal(t, types.OnPluginStartStatusOK, status)

	_ = host.CallOnHttpRequestHeaders([][2]string{
		{":authority", "example.com"},
		{":path", "/v1/chat/completions"},
		{":method", "POST"},
		{"content-type", "application/json"},
	})
	action := host.CallOnHttpRequestBody([]byte(body))
	if local := host.GetLocalResponse(); local != nil {
		return action, local.StatusCode, string(local.Data)
	}
	return action, 0, ""
}

// TestDuplicateMessagesMemberIsRejected pins the ambiguity rule: the limit is
// computed from the first occurrence of "messages" (gjson) while the upstream
// parser keeps the last one, so a duplicated member would let a small first
// value carry an oversized conversation past the limit.
func TestDuplicateMessagesMemberIsRejected(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		body := `{"model":"gpt-4","messages":[{"role":"user","content":"hello"}],"messages":[{"role":"user","content":"hello"}]}`
		action, code, message := callContextLimit(t, body)
		require.Equal(t, types.ActionPause, action)
		require.Equal(t, uint32(400), code)
		require.Contains(t, message, `duplicate "messages" member in request body`)
	})
}

// TestSingleMessagesMemberIsAccepted is the control case: a request with one
// well within the limit is still forwarded unchanged.
func TestSingleMessagesMemberIsAccepted(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		body := `{"model":"gpt-4","messages":[{"role":"user","content":"hello"}]}`
		action, code, _ := callContextLimit(t, body)
		require.Equal(t, types.ActionContinue, action)
		require.Zero(t, code)
	})
}
