// Copyright (c) 2026 Alibaba Group Holding Ltd.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRequestTrailersFinalizeEnforcement(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, _ := test.NewTestHost(testConfig(t, false))
		defer host.Reset()
		host.InitHttp()
		host.CallOnHttpRequestHeaders([][2]string{{":authority", "agent.example.com"}, {":method", "POST"}, {":path", "/"}, {"content-type", "application/json"}, {"a2a-version", "1.0"}})
		require.Equal(t, types.ActionPause, host.CallOnHttpStreamingRequestBody([]byte(`{"jsonrpc":"2.0","id":1,"method":"unknown"}`), false))
		require.Equal(t, types.ActionPause, host.CallOnHttpRequestTrailers([][2]string{{"x-test", "done"}}))
		host.CompleteHttp()
	})
}

func TestResponseTrailersRewriteAgentCard(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, _ := test.NewTestHost(testConfig(t, true))
		defer host.Reset()
		host.InitHttp()
		host.CallOnHttpRequestHeaders([][2]string{{":authority", "agent.example.com"}, {":method", "GET"}, {":path", "/.well-known/agent-card.json"}})
		host.CallOnHttpResponseHeaders([][2]string{{":status", "200"}, {"content-type", "application/json"}})
		host.CallOnHttpStreamingResponseBody([]byte(`{"url":"http://internal-agent:8080/"}`), false)
		host.CallOnHttpResponseTrailers([][2]string{{"x-test", "done"}})
		require.JSONEq(t, `{"url":"https://agents.example.com/a2a"}`, string(host.GetResponseBody()))
		host.CompleteHttp()
	})
}
