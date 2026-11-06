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

var duplicateMessagesDecoratorConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"prepend": []map[string]interface{}{
			{"role": "system", "content": "decorated"},
		},
	})
	return data
}()

func callDecorator(t *testing.T, body string) (types.Action, uint32, string, []byte) {
	t.Helper()
	host, status := test.NewTestHost(duplicateMessagesDecoratorConfig)
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
		return action, local.StatusCode, string(local.Data), nil
	}
	return action, 0, "", host.GetRequestBody()
}

// TestDuplicateMessagesMemberIsRejected pins the ambiguity rule for the rewrite
// path: gjson reads and sjson writes the first "messages" member while the
// upstream parser keeps the last one, so a duplicated member would carry the
// caller's original messages past the decorator.
func TestDuplicateMessagesMemberIsRejected(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		body := `{"model":"gpt-4","messages":[{"role":"user","content":"first"}],"messages":[{"role":"user","content":"second"}]}`
		action, code, message, _ := callDecorator(t, body)
		require.Equal(t, types.ActionPause, action)
		require.Equal(t, uint32(400), code)
		require.Contains(t, message, `duplicate "messages" member in request body`)
	})
}

// TestSingleMessagesMemberIsDecorated is the control case: a normal request is
// still rewritten with the prepended message.
func TestSingleMessagesMemberIsDecorated(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		body := `{"model":"gpt-4","messages":[{"role":"user","content":"hi"}]}`
		action, code, _, rewritten := callDecorator(t, body)
		require.Equal(t, types.ActionContinue, action)
		require.Zero(t, code)
		require.Contains(t, string(rewritten), "decorated")
		require.Contains(t, string(rewritten), `"hi"`)
	})
}
