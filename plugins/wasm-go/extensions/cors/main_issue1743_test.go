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
	"net/http"
	"testing"

	"cors/config"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

func TestIssue1743InvalidActualCorsRequestContinuesUpstream(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(basicCorsConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":scheme", "http"},
			{":authority", "example.com"},
			{":path", "/api/test"},
			{":method", "GET"},
			{"origin", "http://invalid.com"},
		})

		require.Equal(t, types.ActionContinue, action)
		require.Nil(t, host.GetLocalResponse())

		action = host.CallOnHttpResponseHeaders([][2]string{
			{":status", "200"},
			{config.HeaderAccessControlAllowOrigin, "http://invalid.com"},
			{config.HeaderAccessControlAllowMethods, "GET,POST"},
			{config.HeaderAccessControlAllowHeaders, "X-Upstream"},
			{config.HeaderAccessControlExposeHeaders, "X-Upstream-Expose"},
			{config.HeaderAccessControlAllowCredentials, "true"},
			{config.HeaderAccessControlMaxAge, "600"},
		})

		require.Equal(t, types.ActionContinue, action)
		responseHeaders := host.GetResponseHeaders()
		require.False(t, test.HasHeader(responseHeaders, config.HeaderAccessControlAllowOrigin))
		require.False(t, test.HasHeader(responseHeaders, config.HeaderAccessControlAllowMethods))
		require.False(t, test.HasHeader(responseHeaders, config.HeaderAccessControlAllowHeaders))
		require.False(t, test.HasHeader(responseHeaders, config.HeaderAccessControlExposeHeaders))
		require.False(t, test.HasHeader(responseHeaders, config.HeaderAccessControlAllowCredentials))
		require.False(t, test.HasHeader(responseHeaders, config.HeaderAccessControlMaxAge))

		host.CompleteHttp()
	})
}

func TestIssue1743SameOriginOptionsWithPreflightHeadersContinuesUpstream(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(basicCorsConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":scheme", "http"},
			{":authority", "example.com"},
			{":path", "/api/test"},
			{":method", "OPTIONS"},
			{"origin", "http://example.com"},
			{"access-control-request-method", "POST"},
		})

		require.Equal(t, types.ActionContinue, action)
		require.Nil(t, host.GetLocalResponse())

		host.CompleteHttp()
	})
}

func TestIssue1743ValidPreflightReturnsNoContentWithCorsHeaders(t *testing.T) {
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
		require.Equal(t, uint32(http.StatusNoContent), localResponse.StatusCode)
		require.Equal(t, "cors.preflight", localResponse.StatusCodeDetail)
		require.Empty(t, localResponse.Data)
		require.True(t, test.HasHeaderWithValue(localResponse.Headers, config.HeaderPluginTrace, "trace"))
		require.True(t, test.HasHeaderWithValue(localResponse.Headers, config.HeaderAccessControlAllowOrigin, "http://example.com"))
		require.True(t, test.HasHeaderWithValue(localResponse.Headers, config.HeaderAccessControlAllowMethods, "GET,POST,OPTIONS"))
		require.True(t, test.HasHeaderWithValue(localResponse.Headers, config.HeaderAccessControlAllowHeaders, "Content-Type,Authorization"))
		require.True(t, test.HasHeaderWithValue(localResponse.Headers, config.HeaderAccessControlExposeHeaders, "X-Custom-Header"))
		require.True(t, test.HasHeaderWithValue(localResponse.Headers, config.HeaderAccessControlMaxAge, "3600"))

		host.CompleteHttp()
	})
}

func TestIssue1743InvalidPreflightReturnsNoContentWithoutCorsHeaders(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(basicCorsConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		action := host.CallOnHttpRequestHeaders([][2]string{
			{":scheme", "http"},
			{":authority", "api.example.com"},
			{":path", "/api/test"},
			{":method", "OPTIONS"},
			{"origin", "http://invalid.com"},
			{"access-control-request-method", "POST"},
		})

		require.Equal(t, types.ActionPause, action)
		localResponse := host.GetLocalResponse()
		require.NotNil(t, localResponse)
		require.Equal(t, uint32(http.StatusNoContent), localResponse.StatusCode)
		require.Equal(t, "cors.preflight.invalid", localResponse.StatusCodeDetail)
		require.Empty(t, localResponse.Data)
		require.True(t, test.HasHeaderWithValue(localResponse.Headers, config.HeaderPluginTrace, "trace"))
		require.False(t, test.HasHeader(localResponse.Headers, config.HeaderAccessControlAllowOrigin))
		require.False(t, test.HasHeader(localResponse.Headers, config.HeaderAccessControlAllowMethods))
		require.False(t, test.HasHeader(localResponse.Headers, config.HeaderAccessControlAllowHeaders))
		require.False(t, test.HasHeader(localResponse.Headers, config.HeaderAccessControlExposeHeaders))
		require.False(t, test.HasHeader(localResponse.Headers, config.HeaderAccessControlAllowCredentials))
		require.False(t, test.HasHeader(localResponse.Headers, config.HeaderAccessControlMaxAge))

		host.CompleteHttp()
	})
}
