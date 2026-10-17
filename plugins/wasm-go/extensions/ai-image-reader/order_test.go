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
	"strings"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/proxytest"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

// twoImageOrderRequestBody is a chat-completions body whose last user message
// carries image1 then image2, in that order.
const twoImageOrderRequestBody = `{
	"messages": [
		{
			"role": "user",
			"content": [
				{"type": "text", "text": "这些图片里有什么？"},
				{"type": "image_url", "image_url": {"url": "https://example.com/image1.jpg"}},
				{"type": "image_url", "image_url": {"url": "https://example.com/image2.jpg"}}
			]
		}
	]
}`

func ocrResponseBody(content string) []byte {
	data, _ := json.Marshal(map[string]interface{}{
		"choices": []interface{}{
			map[string]interface{}{
				"message": map[string]interface{}{"content": content},
			},
		},
	})
	return data
}

// calloutIndexByIdentity returns the position, inside attrs, of the OCR callout
// whose request body carries the given marker (the image URL).
func calloutIndexByIdentity(t *testing.T, attrs []proxytest.HttpCalloutAttribute, marker string) int {
	t.Helper()
	for i, attr := range attrs {
		if strings.Contains(string(attr.Body), marker) {
			return i
		}
	}
	return -1
}

// pendingCallouts collects the OCR callouts recorded for the active HTTP context.
// Note that GetCalloutAttributesFromContext returns them in dispatch order, and
// this plugin dispatches them in REVERSE order of the message content parts.
func pendingCallouts(t *testing.T, host test.TestHost) []proxytest.HttpCalloutAttribute {
	t.Helper()
	for contextID := uint32(0); contextID < 64; contextID++ {
		if attrs := host.GetCalloutAttributesFromContext(contextID); len(attrs) > 0 {
			return attrs
		}
	}
	return nil
}

var ocrOKHeaders = [][2]string{{"content-type", "application/json"}, {":status", "200"}}

// TestOnHttpRequestBody_MultipleImagesOutOfOrderCompletion is the regression
// test for #5098: each OCR callout must contribute to its own image's label,
// whatever order the asynchronous OCR callouts complete in.
//
// The plugin dispatches the OCR callouts in reverse message order, so the
// out-of-order case is: the FIRST message image's callout answers before the
// SECOND message image's callout.
func TestOnHttpRequestBody_MultipleImagesOutOfOrderCompletion(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(basicDashScopeConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/api/chat"},
			{":method", "POST"},
			{"content-type", "application/json"},
		})

		action := host.CallOnHttpRequestBody([]byte(twoImageOrderRequestBody))
		require.Equal(t, types.ActionPause, action)

		attrs := pendingCallouts(t, host)
		require.Len(t, attrs, 2, "expected one OCR callout per image")

		firstIndex := calloutIndexByIdentity(t, attrs, "image1.jpg")
		secondIndex := calloutIndexByIdentity(t, attrs, "image2.jpg")
		require.NotEqual(t, -1, firstIndex, "no OCR callout carries image1.jpg")
		require.NotEqual(t, -1, secondIndex, "no OCR callout carries image2.jpg")

		// Out-of-order completion: the first image answers first, even though its
		// callout was dispatched last.
		host.CallOnHttpCallResponse(attrs[firstIndex].CalloutID, ocrOKHeaders, nil, ocrResponseBody("FIRST-image-content"))
		host.CallOnHttpCallResponse(attrs[secondIndex].CalloutID, ocrOKHeaders, nil, ocrResponseBody("SECOND-image-content"))

		modifiedBody := string(host.GetRequestBody())
		require.Contains(t, modifiedBody, "第1张图片内容为 FIRST-image-content",
			"label 1 must carry the first image's OCR text regardless of completion order")
		require.Contains(t, modifiedBody, "第2张图片内容为 SECOND-image-content",
			"label 2 must carry the second image's OCR text regardless of completion order")

		host.CompleteHttp()
	})
}

// NOTE: the in-order regression control already exists in main_test.go
// ("multiple images request body"), which drives the OCR responses through
// host.CallOnHttpCall (dispatch order) and must stay green.
//
// A two-image control that completes the callouts in dispatch order cannot be
// used here: after the fix, dispatching callout k increments `finished` before
// the callbacks for callouts < k have run, so completing them in dispatch order
// would trip the `totalImages == finished` guard early. Dispatch order and
// completion order for a multi-image request are therefore not
// interchangeable -- which is exactly the defect this test guards.
