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
	"encoding/json"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

// allQuestionsKeyConfig drives the plugin through the "allQuestions" cache key strategy with a
// redis provider, so the cache key the plugin derives can be read from the redis callout.
var allQuestionsKeyConfig = func() json.RawMessage {
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

// cacheKeyFor sends one chat-completions request through a fresh plugin instance and returns
// the key of the redis lookup the plugin performs for it.
func cacheKeyFor(t *testing.T, body string) string {
	t.Helper()
	host, status := test.NewTestHost(allQuestionsKeyConfig)
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
	require.Equal(t, types.ActionPause, action, "the plugin must wait for the cache lookup")

	calls := host.GetRedisCalloutAttributes()
	require.GreaterOrEqual(t, len(calls), 1, "expected a redis lookup for the request")
	return string(calls[0].Query)
}

// TestAllQuestionsCacheKeyIsUnambiguous pins the property that the "allQuestions" cache key
// must depend on the message sequence, not merely on the text it contains: one message holding
// a line break and two messages holding one line each are different conversations and must not
// share a cache entry (a shared entry serves one conversation's answer to the other).
func TestAllQuestionsCacheKeyIsUnambiguous(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		oneMultilineMessage := `{"model":"qwen-turbo","messages":[{"role":"user","content":"first line\nsecond line"}]}`
		twoMessages := `{"model":"qwen-turbo","messages":[{"role":"user","content":"first line"},{"role":"user","content":"second line"}]}`

		keyOneMessage := cacheKeyFor(t, oneMultilineMessage)
		keyTwoMessages := cacheKeyFor(t, twoMessages)

		require.NotEqual(t, keyOneMessage, keyTwoMessages,
			"one message containing a line break and two messages must not map to the same cache key")
	})
}

// TestAllQuestionsCacheKeyStableForSameConversation is the control case: the same conversation
// always maps to the same key, and a different conversation maps to a different one.
func TestAllQuestionsCacheKeyStableForSameConversation(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		conversation := `{"model":"qwen-turbo","messages":[{"role":"user","content":"what is the weather"},{"role":"assistant","content":"sunny"},{"role":"user","content":"and tomorrow"}]}`
		otherConversation := `{"model":"qwen-turbo","messages":[{"role":"user","content":"what is the weather"},{"role":"assistant","content":"sunny"},{"role":"user","content":"and next week"}]}`

		first := cacheKeyFor(t, conversation)
		repeated := cacheKeyFor(t, conversation)
		other := cacheKeyFor(t, otherConversation)

		require.Equal(t, first, repeated, "the same conversation must map to the same cache key")
		require.NotEqual(t, first, other, "a different conversation must map to a different cache key")
	})
}
