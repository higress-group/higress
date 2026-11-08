// Copyright (c) 2025 Alibaba Group Holding Ltd.
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
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

func hmacBodyTestConfig(validate bool) []byte {
	return createConfig(
		[]map[string]interface{}{{"name": "consumer1", "access_key": "ak1", "secret_key": "sk1"}},
		map[string]interface{}{"global_auth": true, "validate_request_body": validate},
	)
}

func hmacBodyRequestHeaders(digestBody []byte, extra [][2]string) [][2]string {
	date := gmt()
	auth := authHeaderRequestTargetDate("ak1", "sk1", "hmac-sha256", "POST", "/p", date)
	headers := [][2]string{
		{":authority", "e.com"}, {":path", "/p"}, {":method", "POST"},
		{"authorization", auth}, {"date", date}, {"digest", calculateBodyDigest(digestBody)},
	}
	return append(headers, extra...)
}

func TestHmacRawBodyDigest(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		tests := []struct {
			name            string
			headers         [][2]string
			incorrectDigest bool
		}{
			{name: "binary content type", headers: [][2]string{{"content-type", "application/octet-stream"}}},
			{name: "binary content type with incorrect digest", headers: [][2]string{{"content-type", "application/octet-stream"}}, incorrectDigest: true},
			{name: "identity encoding", headers: [][2]string{{"content-encoding", "identity"}}},
			{name: "gzip encoding", headers: [][2]string{{"content-encoding", "gzip"}}},
			{name: "gzip encoding with incorrect digest", headers: [][2]string{{"content-encoding", "gzip"}}, incorrectDigest: true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				host, status := test.NewTestHost(hmacBodyTestConfig(true))
				defer host.Reset()
				require.Equal(t, types.OnPluginStartStatusOK, status)

				payload := []byte{0, 1, 2, 0xff, 'a', 'b', 'c'}
				digestBody := payload
				if tt.incorrectDigest {
					digestBody = []byte("different")
				}
				headers := hmacBodyRequestHeaders(digestBody, tt.headers)
				require.Equal(t, types.HeaderStopIteration, host.CallOnHttpRequestHeaders(headers))
				require.Equal(t, types.ActionPause, host.CallOnHttpStreamingRequestBody(payload[:3], false))
				require.Equal(t, types.ActionContinue, host.CallOnHttpStreamingRequestBody(payload[3:], true))

				response := host.GetLocalResponse()
				if tt.incorrectDigest {
					require.NotNil(t, response)
					require.Equal(t, uint32(401), response.StatusCode)
					require.Contains(t, string(response.Data), "Invalid digest")
				} else {
					require.Nil(t, response)
				}
			})
		}
	})
}

func TestHmacBodyOptionDisabledDoesNotBuffer(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(hmacBodyTestConfig(false))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		headers := hmacBodyRequestHeaders(nil, [][2]string{{"content-encoding", "gzip"}})
		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders(headers))
		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody([]byte{0, 1, 2}))
		require.Nil(t, host.GetLocalResponse())
	})
}
