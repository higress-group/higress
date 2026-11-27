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
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

func TestExtAuthRawBodyDelivery(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		tests := []struct {
			name    string
			headers [][2]string
		}{
			{name: "binary content type", headers: [][2]string{{"content-type", "application/octet-stream"}}},
			{name: "identity encoding", headers: [][2]string{{"content-encoding", "identity"}}},
			{name: "gzip encoding", headers: [][2]string{{"content-encoding", "gzip"}}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				host, status := test.NewTestHost(withRequestBodyConfig)
				defer host.Reset()
				require.Equal(t, types.OnPluginStartStatusOK, status)

				headers := [][2]string{
					{":authority", "example.com"}, {":path", "/users"}, {":method", "POST"},
					{"authorization", "Bearer token123"},
				}
				headers = append(headers, tt.headers...)
				require.Equal(t, types.HeaderStopIteration, host.CallOnHttpRequestHeaders(headers))

				payload := []byte{0, 1, 2, 0xff, 'a', 'b', 'c'}
				require.Equal(t, types.ActionPause, host.CallOnHttpStreamingRequestBody(payload[:3], false))
				require.Equal(t, types.DataStopIterationAndBuffer, host.CallOnHttpStreamingRequestBody(payload[3:], true))

				callouts := host.GetHttpCalloutAttributes()
				require.Len(t, callouts, 1)
				require.Equal(t, payload, callouts[0].Body)
			})
		}
	})
}

func TestExtAuthEmptyBodyDelivery(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(withRequestBodyConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"}, {":path", "/users"}, {":method", "POST"},
		})
		require.Equal(t, types.HeaderStopIteration, action)
		require.Equal(t, types.ActionPause, host.CallOnHttpRequestBody(nil))
		callouts := host.GetHttpCalloutAttributes()
		require.Len(t, callouts, 1)
		require.Empty(t, callouts[0].Body)
		require.Nil(t, host.GetLocalResponse())
	})
}

func TestExtAuthBodyAcquisitionFailure(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(withRequestBodyConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"}, {":path", "/users"}, {":method", "POST"},
		})
		require.Equal(t, types.HeaderStopIteration, action)
		require.Equal(t, types.ActionPause, host.CallOnHttpRequestBody(make([]byte, 1025)))
		require.Empty(t, host.GetHttpCalloutAttributes())
		response := host.GetLocalResponse()
		require.NotNil(t, response)
		require.Equal(t, uint32(503), response.StatusCode)
	})
}

func TestExtAuthBodyOptionDisabledDoesNotBuffer(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(basicEnvoyConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"}, {":path", "/users"}, {":method", "POST"},
			{"content-type", "application/octet-stream"},
		})
		require.Equal(t, types.HeaderStopAllIterationAndWatermark, action)
		callouts := host.GetHttpCalloutAttributes()
		require.Len(t, callouts, 1)
		require.Empty(t, callouts[0].Body)

		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody([]byte{0, 1, 2}))
		require.Len(t, host.GetHttpCalloutAttributes(), 1)
	})
}
