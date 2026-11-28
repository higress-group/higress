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

var duplicateMessagesGuardConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"serviceName":               "security-service",
		"servicePort":               8080,
		"serviceHost":               "security.example.com",
		"accessKey":                 "test-ak",
		"secretKey":                 "test-sk",
		"checkRequest":              true,
		"checkResponse":             false,
		"contentModerationLevelBar": "high",
		"promptAttackLevelBar":      "high",
		"sensitiveDataLevelBar":     "S3",
		"timeout":                   1000,
		"bufferLimit":               500,
	})
	return data
}()

func callGuard(t *testing.T, body string) (types.Action, int, string, bool) {
	t.Helper()
	host, status := test.NewTestHost(duplicateMessagesGuardConfig)
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

// TestDuplicateMessagesMemberIsRejected pins the guardrail rule: the text sent
// to the moderation service is extracted from the first "messages" occurrence
// (gjson resolves the default path messages.@reverse.0.content against it),
// while the upstream parser keeps the last one. A duplicated member would have
// the first conversation moderated and the last one served, bypassing the check.
func TestDuplicateMessagesMemberIsRejected(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		body := `{"model":"qwen-turbo","messages":[{"role":"user","content":"hello"}],"messages":[{"role":"user","content":"unsafe content"}]}`
		action, code, message, hasLocal := callGuard(t, body)
		require.Equal(t, types.ActionPause, action)
		require.True(t, hasLocal, "the ambiguous body must be answered locally")
		require.Equal(t, 400, code)
		require.Contains(t, message, `duplicate "messages" member in request body`)
	})
}

// TestSingleMessagesMemberIsChecked is the control case: a normal request is
// sent to the moderation service instead of being rejected locally.
func TestSingleMessagesMemberIsChecked(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		body := `{"model":"qwen-turbo","messages":[{"role":"user","content":"hello, how are you?"}]}`
		action, code, _, hasLocal := callGuard(t, body)
		require.Equal(t, types.ActionPause, action)
		require.False(t, hasLocal, "a single-member body must not be answered locally")
		require.Zero(t, code)
	})
}
