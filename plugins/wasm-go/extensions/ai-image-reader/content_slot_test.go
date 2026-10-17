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
	"strings"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

// TestOnHttpRequestBodyMissingOCRResult covers the position/label mapping when one image
// has no OCR result: the label of every image must carry that image's own content, and an
// image whose OCR failed must not shift the remaining labels.
//
// The same root cause as out-of-order completion: the plugin used to append results in
// completion order while labelling them by dispatch position, so a missing result moved
// the surviving content to the wrong image number.
func TestOnHttpRequestBodyMissingOCRResult(t *testing.T) {
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

		requestBody := `{
			"messages": [
				{
					"role": "user",
					"content": [
						{"type": "text", "text": "what do these images show?"},
						{"type": "image_url", "image_url": {"url": "https://example.com/image1.jpg"}},
						{"type": "image_url", "image_url": {"url": "https://example.com/image2.jpg"}}
					]
				}
			]
		}`

		action := host.CallOnHttpRequestBody([]byte(requestBody))
		require.Equal(t, types.ActionPause, action)

		// Callout 0 belongs to image 2 (the content parts are walked in reverse) and is
		// answered with an error; callout 1 belongs to image 1 and succeeds.
		host.CallOnHttpCall([][2]string{
			{"content-type", "application/json"},
			{":status", "500"},
		}, []byte(`{"error":"ocr failed"}`))
		host.CallOnHttpCall([][2]string{
			{"content-type", "application/json"},
			{":status", "200"},
		}, []byte(`{"choices":[{"message":{"content":"CONTENT_OF_IMAGE_1"}}]}`))

		modifiedBody := string(host.GetRequestBody())
		require.NotEmpty(t, modifiedBody)
		require.Contains(t, modifiedBody, "第1张图片内容为 CONTENT_OF_IMAGE_1",
			"image 1 must keep its own OCR text even when image 2 has no result")
		require.NotContains(t, modifiedBody, "第2张图片内容为 CONTENT_OF_IMAGE_1",
			"image 1's content must not be reported as image 2's")

		host.CompleteHttp()
	})
}

// TestOnHttpRequestBodyInOrderOCR is the control case: when every image produces a result,
// each label carries its own image's content.
func TestOnHttpRequestBodyInOrderOCR(t *testing.T) {
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

		requestBody := `{
			"messages": [
				{
					"role": "user",
					"content": [
						{"type": "text", "text": "what do these images show?"},
						{"type": "image_url", "image_url": {"url": "https://example.com/image1.jpg"}},
						{"type": "image_url", "image_url": {"url": "https://example.com/image2.jpg"}}
					]
				}
			]
		}`

		action := host.CallOnHttpRequestBody([]byte(requestBody))
		require.Equal(t, types.ActionPause, action)

		host.CallOnHttpCall([][2]string{{"content-type", "application/json"}, {":status", "200"}},
			[]byte(`{"choices":[{"message":{"content":"CONTENT_OF_IMAGE_2"}}]}`))
		host.CallOnHttpCall([][2]string{{"content-type", "application/json"}, {":status", "200"}},
			[]byte(`{"choices":[{"message":{"content":"CONTENT_OF_IMAGE_1"}}]}`))

		modifiedBody := string(host.GetRequestBody())
		require.True(t, strings.Contains(modifiedBody, "第1张图片内容为 CONTENT_OF_IMAGE_1"), modifiedBody)
		require.True(t, strings.Contains(modifiedBody, "第2张图片内容为 CONTENT_OF_IMAGE_2"), modifiedBody)

		host.CompleteHttp()
	})
}
