// Copyright (c) 2023 Alibaba Group Holding Ltd.
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

// streamingCapConfig enables request body logging with a 10-byte cap so that a body
// delivered in several chunks crosses the cap more than once.
var streamingCapConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"request": map[string]interface{}{
			"headers": map[string]interface{}{
				"enabled": false,
			},
			"body": map[string]interface{}{
				"enabled":      true,
				"maxSize":      10,
				"contentTypes": []string{"text/plain"},
			},
		},
		"response": map[string]interface{}{
			"headers": map[string]interface{}{
				"enabled": false,
			},
			"body": map[string]interface{}{
				"enabled": false,
			},
		},
	})
	return data
}()

// TestStreamingBodyOverCapPublishesHeadOnce checks that a streamed body larger than
// maxSize leaves the first maxSize bytes in the filter state. The buffer used to keep
// accumulating after the cap was reached and published again, which replaced the head
// with a later slice of the body.
func TestStreamingBodyOverCapPublishesHeadOnce(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(streamingCapConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":method", "POST"},
			{":path", "/test"},
			{":scheme", "https"},
			{"content-type", "text/plain"},
		})

		// Four six-byte chunks: the 10-byte cap is crossed inside the second and the
		// fourth chunk.
		chunks := []string{"AAAAAA", "BBBBBB", "CCCCCC", "DDDDDD"}
		for i, chunk := range chunks {
			action := host.CallOnHttpStreamingRequestBody([]byte(chunk), i == len(chunks)-1)
			require.Equal(t, types.ActionContinue, action)
		}

		raw, err := host.GetProperty([]string{logKeyRequestBody})
		require.NoError(t, err)
		require.Equal(t, "AAAAAABBBB", string(raw),
			"the filter state must hold the first maxSize bytes of the stream")
	})
}

// TestStreamingBodyUnderCapStillLogged checks the unchanged path: a body that never
// reaches the cap is published once, at the end of the stream.
func TestStreamingBodyUnderCapStillLogged(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(streamingCapConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":method", "POST"},
			{":path", "/test"},
			{":scheme", "https"},
			{"content-type", "text/plain"},
		})

		action := host.CallOnHttpStreamingRequestBody([]byte("ABCDEFGH"), true)
		require.Equal(t, types.ActionContinue, action)

		raw, err := host.GetProperty([]string{logKeyRequestBody})
		require.NoError(t, err)
		require.Equal(t, "ABCDEFGH", string(raw))
	})
}
