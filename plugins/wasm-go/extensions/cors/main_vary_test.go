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

	"cors/config"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

func TestIssue1743SpecificOriginResponseAddsVaryOrigin(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(basicCorsConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":scheme", "http"},
			{":authority", "api.example.com"},
			{":path", "/api/test"},
			{":method", "GET"},
			{"origin", "http://example.com"},
		})
		require.Equal(t, types.ActionContinue, action)

		action = host.CallOnHttpResponseHeaders([][2]string{
			{":status", "200"},
			{"content-type", "application/json"},
		})
		require.Equal(t, types.ActionContinue, action)

		responseHeaders := host.GetResponseHeaders()
		require.True(t, test.HasHeaderWithValue(responseHeaders, config.HeaderAccessControlAllowOrigin, "http://example.com"))
		require.True(t, test.HasHeaderWithValue(responseHeaders, headerVary, varyOrigin))

		host.CompleteHttp()
	})
}

func TestIssue1743SpecificOriginPreflightAddsVaryOrigin(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(basicCorsConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":scheme", "http"},
			{":authority", "api.example.com"},
			{":path", "/api/test"},
			{":method", "OPTIONS"},
			{"origin", "http://example.com"},
			{"access-control-request-method", "POST"},
			{"access-control-request-headers", "Content-Type"},
		})
		require.Equal(t, types.ActionPause, action)

		localResponse := host.GetLocalResponse()
		require.NotNil(t, localResponse)
		require.True(t, test.HasHeaderWithValue(localResponse.Headers, config.HeaderAccessControlAllowOrigin, "http://example.com"))
		require.True(t, test.HasHeaderWithValue(localResponse.Headers, headerVary, varyOrigin))

		host.CompleteHttp()
	})
}

func TestIssue1743AppendVaryOriginPreservesExistingVaryHeader(t *testing.T) {
	headers := appendVaryOriginHeader([][2]string{
		{headerVary, "Accept-Encoding"},
	}, "http://example.com")

	require.ElementsMatch(t, []string{"Accept-Encoding", varyOrigin}, headerValues(headers, headerVary))
}

func TestIssue1743LiteralWildcardAllowOriginDoesNotAddVaryOrigin(t *testing.T) {
	headers := appendVaryOriginHeader([][2]string{
		{config.HeaderAccessControlAllowOrigin, "*"},
	}, "*")

	require.False(t, test.HasHeader(headers, headerVary))
}

func headerValues(headers [][2]string, name string) []string {
	values := make([]string, 0)
	for _, header := range headers {
		if strings.EqualFold(header[0], name) {
			values = append(values, header[1])
		}
	}
	return values
}
