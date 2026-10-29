// Copyright (c) 2026 Alibaba Group Holding Ltd.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

func testConfig(t *testing.T, legacy bool) []byte {
	t.Helper()
	data, err := json.Marshal(map[string]interface{}{
		"protocolVersion": "1.0",
		"mode":            "enforce",
		"legacy03":        map[string]interface{}{"enabled": legacy},
		"agent": map[string]interface{}{
			"id":              "weather-agent",
			"externalBaseURL": "https://agents.example.com/a2a",
		},
		"jsonrpc": map[string]interface{}{
			"maxRequestBytes": 4096,
		},
	})
	require.NoError(t, err)
	return data
}

func testConfigWithExternalURL(t *testing.T, externalURL string) []byte {
	t.Helper()
	var config map[string]interface{}
	require.NoError(t, json.Unmarshal(testConfig(t, false), &config))
	agent := config["agent"].(map[string]interface{})
	agent["externalBaseURL"] = externalURL
	data, err := json.Marshal(config)
	require.NoError(t, err)
	return data
}

func testConfigWithCardRewrite(t *testing.T, legacy, rewrite bool) []byte {
	t.Helper()
	var config map[string]interface{}
	require.NoError(t, json.Unmarshal(testConfig(t, legacy), &config))
	config["agentCard"] = map[string]interface{}{"rewrite": rewrite}
	data, err := json.Marshal(config)
	require.NoError(t, err)
	return data
}

func testConfigWithoutExternalURL(t *testing.T) []byte {
	t.Helper()
	var config map[string]interface{}
	require.NoError(t, json.Unmarshal(testConfig(t, false), &config))
	delete(config["agent"].(map[string]interface{}), "externalBaseURL")
	data, err := json.Marshal(config)
	require.NoError(t, err)
	return data
}

func TestTrustedHeadersReplaceSpoofedInput(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(testConfig(t, false))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		host.InitHttp()
		require.Equal(t, types.HeaderStopIteration, host.CallOnHttpRequestHeaders([][2]string{
			{":method", "POST"}, {":path", "/a2a"}, {":authority", "agent.example.com"},
			{"content-type", "application/json"}, {"a2a-version", "1.0"},
			{"x-higress-a2a-method", "DeleteTask"}, {"x-higress-a2a-task-id", "spoofed"},
		}))
		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody([]byte(`{"jsonrpc":"2.0","id":"r1","method":"SendMessage","params":{"tenant":"tenant-a","message":{"messageId":"message-1"}}}`)))
		headers := map[string]string{}
		for _, pair := range host.GetRequestHeaders() {
			headers[pair[0]] = pair[1]
		}
		require.Equal(t, "SendMessage", headers["x-higress-a2a-method"])
		require.Equal(t, "message-1", headers["x-higress-a2a-message-id"])
		require.Equal(t, "weather-agent", headers["x-higress-a2a-agent-id"])
		require.Equal(t, "tenant-a", headers["x-higress-a2a-tenant"])
		method, err := host.GetProperty([]string{"a2a", "method"})
		require.NoError(t, err)
		require.Equal(t, "SendMessage", string(method))
		host.CompleteHttp()
	})
}

func TestLegacyAliasAndStrictUnknownMethod(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(testConfig(t, true))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		host.InitHttp()
		host.CallOnHttpRequestHeaders([][2]string{{":method", "POST"}, {":path", "/a2a"}, {":authority", "agent.example.com"}, {"content-type", "application/json"}, {"a2a-version", "0.3"}})
		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody([]byte(`{"jsonrpc":"2.0","id":1,"method":"tasks/cancel","params":{"id":"t1"}}`)))
		headers := map[string]string{}
		for _, pair := range host.GetRequestHeaders() {
			headers[pair[0]] = pair[1]
		}
		require.Equal(t, "CancelTask", headers["x-higress-a2a-method"])
		require.Equal(t, "legacy", headers["x-higress-a2a-parse-status"])
		host.CompleteHttp()
	})

	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(testConfig(t, false))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		host.InitHttp()
		host.CallOnHttpRequestHeaders([][2]string{{":method", "POST"}, {":path", "/a2a"}, {":authority", "agent.example.com"}, {"content-type", "application/json"}, {"a2a-version", "1.0"}})
		require.Equal(t, types.ActionPause, host.CallOnHttpRequestBody([]byte(`{"jsonrpc":"2.0","id":1,"method":"unknown"}`)))
		host.CompleteHttp()
	})
}

func TestRequestVersionControlsLegacyMethodSemantics(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(testConfig(t, true))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		host.InitHttp()
		host.CallOnHttpRequestHeaders([][2]string{{":method", "POST"}, {":path", "/a2a"}, {":authority", "agent.example.com"}, {"content-type", "application/json"}})
		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody([]byte(`{"jsonrpc":"2.0","id":1,"method":"tasks/get","params":{"id":"t1"}}`)))
		require.Equal(t, "0.3", headerMap(host.GetRequestHeaders())["x-higress-a2a-version"])
		host.CompleteHttp()
	})

	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(testConfig(t, true))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		host.InitHttp()
		host.CallOnHttpRequestHeaders([][2]string{{":method", "POST"}, {":path", "/a2a"}, {":authority", "agent.example.com"}, {"content-type", "application/json"}, {"a2a-version", "1.0"}})
		require.Equal(t, types.ActionPause, host.CallOnHttpRequestBody([]byte(`{"jsonrpc":"2.0","id":1,"method":"tasks/get","params":{"id":"t1"}}`)))
		require.Contains(t, string(host.GetLocalResponse().Data), `"code":-32601`)
		host.CompleteHttp()
	})

	for _, headers := range [][][2]string{
		{{":method", "POST"}, {":path", "/a2a"}, {":authority", "agent.example.com"}, {"content-type", "application/json"}},
		{{":method", "POST"}, {":path", "/a2a"}, {":authority", "agent.example.com"}, {"content-type", "application/json"}, {"a2a-version", "0.3"}},
	} {
		test.RunGoTest(t, func(t *testing.T) {
			host, status := test.NewTestHost(testConfig(t, false))
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)
			host.InitHttp()
			host.CallOnHttpRequestHeaders(headers)
			require.Equal(t, types.ActionPause, host.CallOnHttpRequestBody([]byte(`{"jsonrpc":"2.0","id":1,"method":"tasks/get"}`)))
			require.Contains(t, string(host.GetLocalResponse().Data), `"code":-32009`)
			host.CompleteHttp()
		})
	}
}

func TestEncodedA2ARequestFailsClosedBeforeBodyCallback(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(testConfig(t, false))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		host.InitHttp()
		action := host.CallOnHttpRequestHeaders([][2]string{
			{":method", "POST"}, {":path", "/a2a"}, {":authority", "agent.example.com"},
			{"content-type", "application/json"}, {"content-encoding", "gzip"}, {"a2a-version", "1.0"},
		})
		require.Equal(t, types.ActionPause, action)
		require.Contains(t, string(host.GetLocalResponse().Data), `"code":-32600`)
		host.CompleteHttp()
	})
}

func TestEmptyA2ARequestFailsClosedWithoutWaitingForBody(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(testConfig(t, false))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		host.InitHttp()
		action := host.CallOnHttpRequestHeaders([][2]string{
			{":method", "POST"}, {":path", "/a2a"}, {":authority", "agent.example.com"},
			{"content-type", "application/json"}, {"a2a-version", "1.0"},
		}, test.WithEndOfStream(true))
		require.Equal(t, types.ActionPause, action)
		require.Contains(t, string(host.GetLocalResponse().Data), `"code":-32600`)
		host.CompleteHttp()
	})
}

func TestNonA2ARequestPassesWithoutTrustedHeaders(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(testConfig(t, false))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		host.InitHttp()
		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{{":method", "GET"}, {":path", "/health"}, {":authority", "agent.example.com"}, {"x-higress-a2a-method", "spoofed"}}))
		for _, pair := range host.GetRequestHeaders() {
			require.NotEqual(t, "x-higress-a2a-method", pair[0])
		}
		host.CompleteHttp()
	})
}

func TestHTTPJSONMediaTypeIsNotParsedAsJSONRPC(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(testConfig(t, false))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		host.InitHttp()
		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
			{":method", "POST"}, {":path", "/a2a"}, {":authority", "agent.example.com"},
			{"content-type", "application/a2a+json"}, {"a2a-version", "1.0"},
		}))
		require.Nil(t, host.GetLocalResponse())
		require.Empty(t, headerMap(host.GetRequestHeaders())["x-higress-a2a-method"])
		host.CompleteHttp()
	})
}

func TestResponseTrustedHeadersReplaceSpoofedUpstream(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(testConfig(t, false))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		host.InitHttp()
		require.Equal(t, types.HeaderStopIteration, host.CallOnHttpRequestHeaders([][2]string{{":method", "POST"}, {":path", "/a2a"}, {":authority", "agent.example.com"}, {"content-type", "application/json"}, {"a2a-version", "1.0"}}))
		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody([]byte(`{"jsonrpc":"2.0","id":"r1","method":"SendMessage","params":{"message":{"messageId":"message-1"}}}`)))
		require.Equal(t, types.HeaderStopIteration, host.CallOnHttpResponseHeaders([][2]string{
			{":status", "200"}, {"content-type", "application/json"},
			{"x-higress-a2a-task-id", "spoofed"}, {"x-higress-a2a-agent-id", "spoofed-agent"}, {"x-higress-a2a-error-code", "spoofed-error"},
		}))
		headers := headerMap(host.GetResponseHeaders())
		require.Empty(t, headers["x-higress-a2a-task-id"])
		require.Empty(t, headers["x-higress-a2a-agent-id"])
		require.Empty(t, headers["x-higress-a2a-error-code"])
		require.Equal(t, types.ActionContinue, host.CallOnHttpResponseBody([]byte(`{"jsonrpc":"2.0","id":"r1","result":{"id":"task-unary-1","contextId":"ctx-unary-1","status":{"state":"completed"}}}`)))
		headers = headerMap(host.GetResponseHeaders())
		require.Equal(t, "task-unary-1", headers["x-higress-a2a-task-id"])
		require.Equal(t, "ctx-unary-1", headers["x-higress-a2a-context-id"])
		require.Equal(t, "completed", headers["x-higress-a2a-task-state"])
		require.Equal(t, "weather-agent", headers["x-higress-a2a-agent-id"])
		host.CompleteHttp()
	})
}

func TestSSEResponseHeadersContinueWithoutFullStreamBuffering(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(testConfig(t, false))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		host.InitHttp()
		require.Equal(t, types.HeaderStopIteration, host.CallOnHttpRequestHeaders([][2]string{{":method", "POST"}, {":path", "/a2a"}, {":authority", "agent.example.com"}, {"content-type", "application/json"}, {"a2a-version", "1.0"}}))
		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestBody([]byte(`{"jsonrpc":"2.0","id":"r1","method":"SendStreamingMessage","params":{"message":{"messageId":"message-1"}}}`)))
		require.Equal(t, types.ActionContinue, host.CallOnHttpResponseHeaders([][2]string{{":status", "200"}, {"content-type", "text/event-stream"}}))
		require.Equal(t, types.ActionContinue, host.CallOnHttpStreamingResponseBody([]byte("data: {}\n\n"), false))
		host.CompleteHttp()
	})
}

func TestCanonicalAgentCardRewritesPublicInterfacesAndPreservesUnknownFields(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(testConfig(t, false))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		host.InitHttp()
		require.Equal(t, types.ActionContinue, host.CallOnHttpRequestHeaders([][2]string{
			{":method", "GET"}, {":scheme", "https"}, {":path", "/agents/weather/.well-known/agent-card.json"},
			{":authority", "agents.example.com"}, {"x-forwarded-proto", "http"}, {"x-forwarded-host", "attacker.example.com"},
			{"x-mse-consumer", "consumer-a"},
		}))
		require.Equal(t, types.HeaderStopIteration, host.CallOnHttpResponseHeaders([][2]string{{":status", "200"}, {"content-type", "application/json"}, {"etag", `"upstream"`}, {"x-higress-a2a-card-cache-key", "untrusted-shared-key"}}))
		body := []byte(`{
			"name":"Weather",
			"supportedInterfaces":[{"url":"http://internal-agent:8080/a2a","protocolBinding":"JSONRPC","protocolVersion":"1.0","x-interface":{"keep":true}}],
			"capabilities":{"streaming":true,"extendedAgentCard":true,"stateTransitionHistory":true},
			"x-vendor":{"nested":[1,"two",{"three":3}]}
		}`)
		require.Equal(t, types.ActionContinue, host.CallOnHttpResponseBody(body))
		var card map[string]interface{}
		require.NoError(t, json.Unmarshal(host.GetResponseBody(), &card))
		interfaces := card["supportedInterfaces"].([]interface{})
		iface := interfaces[0].(map[string]interface{})
		require.Equal(t, "https://agents.example.com/a2a", iface["url"])
		require.Equal(t, map[string]interface{}{"keep": true}, iface["x-interface"])
		require.Equal(t, map[string]interface{}{"nested": []interface{}{float64(1), "two", map[string]interface{}{"three": float64(3)}}}, card["x-vendor"])
		capabilities := card["capabilities"].(map[string]interface{})
		require.Equal(t, true, capabilities["extendedAgentCard"])
		require.NotContains(t, capabilities, "stateTransitionHistory")
		responseHeaders := headerMap(host.GetResponseHeaders())
		require.NotEqual(t, `"upstream"`, responseHeaders["etag"])
		require.NotEmpty(t, responseHeaders["etag"])
		require.Empty(t, responseHeaders["x-higress-a2a-card-cache-key"], "authenticated cards must not opt into an unpartitioned shared cache")
		host.CompleteHttp()
	})
}

func TestLegacyAgentCardPathRewritesTopLevelURL(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(testConfig(t, true))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		host.InitHttp()
		host.CallOnHttpRequestHeaders([][2]string{
			{":method", "GET"}, {":scheme", "https"}, {":path", "/a2a/.well-known/agent.json"},
			{":authority", "agents.example.com"},
		})
		host.CallOnHttpResponseHeaders([][2]string{{":status", "200"}, {"content-type", "application/json; charset=utf-8"}})
		host.CallOnHttpResponseBody([]byte(`{"name":"Weather","url":"http://internal-agent/a2a","preferredTransport":"JSON-RPC","additionalInterfaces":[{"url":"http://169.254.1.2/a2a","transport":"HTTP+JSON"}],"x-safe":"kept"}`))
		var card map[string]interface{}
		require.NoError(t, json.Unmarshal(host.GetResponseBody(), &card))
		require.Equal(t, "https://agents.example.com/a2a", card["url"])
		require.Empty(t, card["additionalInterfaces"])
		require.Equal(t, "kept", card["x-safe"])
		host.CompleteHttp()
	})
}

func TestLegacyAgentCardAtCanonicalPathWhenCompatibilityEnabled(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(testConfig(t, true))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		host.InitHttp()
		host.CallOnHttpRequestHeaders([][2]string{{":method", "GET"}, {":scheme", "https"}, {":path", canonicalAgentCardPath}, {":authority", "agents.example.com"}})
		host.CallOnHttpResponseHeaders([][2]string{{":status", "200"}, {"content-type", "application/json"}})
		host.CallOnHttpResponseBody([]byte(`{"url":"http://internal-agent/a2a","preferredTransport":"JSONRPC"}`))
		var card map[string]interface{}
		require.NoError(t, json.Unmarshal(host.GetResponseBody(), &card))
		require.Equal(t, "https://agents.example.com/a2a", card["url"])
		host.CompleteHttp()
	})
}

func TestV1AgentCardAdvertisesOnlyJSONRPC10(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(testConfigWithExternalURL(t, "https://public.example.com/a2a"))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		host.InitHttp()
		host.CallOnHttpRequestHeaders([][2]string{{":method", "GET"}, {":scheme", "https"}, {":path", canonicalAgentCardPath}, {":authority", "agents.example.com"}})
		host.CallOnHttpResponseHeaders([][2]string{{":status", "200"}, {"content-type", "application/json"}})
		host.CallOnHttpResponseBody([]byte(`{"supportedInterfaces":[
			{"url":"http://internal-agent/jsonrpc","protocolBinding":"JSONRPC","protocolVersion":"1.0","tenant":"tenant-a"},
			{"url":"internal-agent:8443","protocolBinding":"GRPC","protocolVersion":"1.0"},
			{"url":"http://internal-agent/http-json","protocolBinding":"HTTP+JSON","protocolVersion":"1.0"},
			{"url":"http://internal-agent/legacy","protocolBinding":"JSONRPC","protocolVersion":"0.3"}
		]}`))
		var card map[string]interface{}
		require.NoError(t, json.Unmarshal(host.GetResponseBody(), &card))
		interfaces := card["supportedInterfaces"].([]interface{})
		require.Len(t, interfaces, 1)
		iface := interfaces[0].(map[string]interface{})
		require.Equal(t, "JSONRPC", iface["protocolBinding"])
		require.Equal(t, "1.0", iface["protocolVersion"])
		require.Equal(t, "https://public.example.com/a2a", iface["url"])
		require.Equal(t, "tenant-a", iface["tenant"])
		host.CompleteHttp()
	})
}

func TestV1AgentCardRejectsInvalidTenant(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(testConfig(t, false))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		host.InitHttp()
		host.CallOnHttpRequestHeaders([][2]string{{":method", "GET"}, {":path", canonicalAgentCardPath}, {":authority", "agents.example.com"}})
		host.CallOnHttpResponseHeaders([][2]string{{":status", "200"}, {"content-type", "application/json"}})
		host.CallOnHttpResponseBody([]byte(`{"supportedInterfaces":[{"url":"http://internal-agent/a2a","protocolBinding":"JSONRPC","protocolVersion":"1.0","tenant":42}]}`))
		require.Equal(t, "502", headerMap(host.GetResponseHeaders())[":status"])
		host.CompleteHttp()
	})
}

func TestV1AgentCardRejectsUnsupportedOnlyCard(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(testConfig(t, false))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		host.InitHttp()
		host.CallOnHttpRequestHeaders([][2]string{{":method", "GET"}, {":scheme", "https"}, {":path", canonicalAgentCardPath}, {":authority", "agents.example.com"}})
		host.CallOnHttpResponseHeaders([][2]string{{":status", "200"}, {"content-type", "application/json"}})
		host.CallOnHttpResponseBody([]byte(`{"supportedInterfaces":[{"url":"http://internal-agent/http-json","protocolBinding":"HTTP+JSON","protocolVersion":"1.0"}]}`))
		require.Equal(t, "502", headerMap(host.GetResponseHeaders())[":status"])
		host.CompleteHttp()
	})
}

func TestConfiguredExternalURLTakesPrecedence(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(testConfigWithExternalURL(t, "https://public.example.com/weather"))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		host.InitHttp()
		host.CallOnHttpRequestHeaders([][2]string{
			{":method", "GET"}, {":scheme", "https"}, {":path", canonicalAgentCardPath}, {":authority", "ignored.example.com"},
			{"x-forwarded-host", "also-ignored.example.com"},
		})
		host.CallOnHttpResponseHeaders([][2]string{{":status", "200"}, {"content-type", "application/json"}})
		host.CallOnHttpResponseBody([]byte(`{"supportedInterfaces":[{"url":"http://internal-agent:8080/a2a","protocolBinding":"JSONRPC","protocolVersion":"1.0"}]}`))
		var card map[string]interface{}
		require.NoError(t, json.Unmarshal(host.GetResponseBody(), &card))
		iface := card["supportedInterfaces"].([]interface{})[0].(map[string]interface{})
		require.Equal(t, "https://public.example.com/weather", iface["url"])
		host.CompleteHttp()
	})
}

func TestMissingExternalBaseURLFailsAgentCardClosed(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(testConfigWithoutExternalURL(t))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		host.InitHttp()
		host.CallOnHttpRequestHeaders([][2]string{{":method", "GET"}, {":scheme", "https"}, {":path", canonicalAgentCardPath}, {":authority", "agents.example.com"}})
		host.CallOnHttpResponseHeaders([][2]string{{":status", "200"}, {"content-type", "application/json"}})
		host.CallOnHttpResponseBody([]byte(`{"supportedInterfaces":[{"url":"http://internal-agent:8080/a2a","protocolBinding":"JSONRPC","protocolVersion":"1.0"}]}`))
		require.Equal(t, "502", headerMap(host.GetResponseHeaders())[":status"])
		host.CompleteHttp()
	})
}

func TestAgentCardRejectsUnsafeEndpointAndUnsupportedTransport(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"loopback endpoint", `{"supportedInterfaces":[{"url":"https://127.0.0.1/a2a","protocolBinding":"JSONRPC","protocolVersion":"1.0"}]}`},
		{"non-canonical loopback endpoint", `{"supportedInterfaces":[{"url":"https://127.1/a2a","protocolBinding":"JSONRPC","protocolVersion":"1.0"}]}`},
		{"link-local endpoint", `{"supportedInterfaces":[{"url":"https://169.254.1.2/a2a","protocolBinding":"JSONRPC","protocolVersion":"1.0"}]}`},
		{"unspecified endpoint", `{"supportedInterfaces":[{"url":"https://0.0.0.0/a2a","protocolBinding":"JSONRPC","protocolVersion":"1.0"}]}`},
		{"private endpoint", `{"supportedInterfaces":[{"url":"https://10.0.0.1/a2a","protocolBinding":"JSONRPC","protocolVersion":"1.0"}]}`},
		{"ULA endpoint", `{"supportedInterfaces":[{"url":"https://[fd00::1]/a2a","protocolBinding":"JSONRPC","protocolVersion":"1.0"}]}`},
		{"multicast endpoint", `{"supportedInterfaces":[{"url":"https://224.0.0.1/a2a","protocolBinding":"JSONRPC","protocolVersion":"1.0"}]}`},
		{"unsupported binding", `{"supportedInterfaces":[{"url":"https://upstream.example.com/a2a","protocolBinding":"websocket","protocolVersion":"1.0"}]}`},
		{"missing protocol version", `{"supportedInterfaces":[{"url":"https://upstream.example.com/a2a","protocolBinding":"JSONRPC"}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			test.RunGoTest(t, func(t *testing.T) {
				host, status := test.NewTestHost(testConfigWithCardRewrite(t, false, false))
				defer host.Reset()
				require.Equal(t, types.OnPluginStartStatusOK, status)
				host.InitHttp()
				host.CallOnHttpRequestHeaders([][2]string{{":method", "GET"}, {":scheme", "https"}, {":path", canonicalAgentCardPath}, {":authority", "agents.example.com"}})
				host.CallOnHttpResponseHeaders([][2]string{{":status", "200"}, {"content-type", "application/json"}})
				host.CallOnHttpResponseBody([]byte(tt.body))
				require.Equal(t, "502", headerMap(host.GetResponseHeaders())[":status"])
				require.JSONEq(t, `{"error":"invalid A2A Agent Card"}`, string(host.GetResponseBody()))
				host.CompleteHttp()
			})
		})
	}
}

func TestLegacyAgentCardRejectsUnsafeAdditionalInterfaceWithoutRewrite(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(testConfigWithCardRewrite(t, true, false))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		host.InitHttp()
		host.CallOnHttpRequestHeaders([][2]string{{":method", "GET"}, {":scheme", "https"}, {":path", legacyAgentCardPath}, {":authority", "agents.example.com"}})
		host.CallOnHttpResponseHeaders([][2]string{{":status", "200"}, {"content-type", "application/json"}})
		host.CallOnHttpResponseBody([]byte(`{"url":"https://agent.example.com/a2a","preferredTransport":"JSONRPC","additionalInterfaces":[{"url":"https://10.0.0.1/a2a","transport":"HTTP+JSON"}]}`))
		require.Equal(t, "502", headerMap(host.GetResponseHeaders())[":status"])
		host.CompleteHttp()
	})
}

func TestAgentCardRejectsInvalidContentTypeAndDeclaredOversize(t *testing.T) {
	tests := []struct {
		name    string
		headers [][2]string
	}{
		{"content type", [][2]string{{":status", "200"}, {"content-type", "text/html"}}},
		{"content length", [][2]string{{":status", "200"}, {"content-type", "application/json"}, {"content-length", "262145"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			test.RunGoTest(t, func(t *testing.T) {
				host, status := test.NewTestHost(testConfig(t, false))
				defer host.Reset()
				require.Equal(t, types.OnPluginStartStatusOK, status)
				host.InitHttp()
				host.CallOnHttpRequestHeaders([][2]string{{":method", "GET"}, {":scheme", "https"}, {":path", canonicalAgentCardPath}, {":authority", "agents.example.com"}})
				require.Equal(t, types.ActionContinue, host.CallOnHttpResponseHeaders(tt.headers))
				host.CallOnHttpStreamingResponseBody([]byte("untrusted upstream body"), true)
				require.Equal(t, "502", headerMap(host.GetResponseHeaders())[":status"])
				require.JSONEq(t, `{"error":"invalid A2A Agent Card"}`, string(host.GetResponseBody()))
				host.CompleteHttp()
			})
		})
	}
}

func TestEncodedAgentCardResponseFailsClosedAtHeaders(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(testConfig(t, false))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		host.InitHttp()
		host.CallOnHttpRequestHeaders([][2]string{{":method", "GET"}, {":scheme", "https"}, {":path", canonicalAgentCardPath}, {":authority", "agents.example.com"}})
		require.Equal(t, types.ActionContinue, host.CallOnHttpResponseHeaders([][2]string{{":status", "200"}, {"content-type", "application/json"}, {"content-encoding", "gzip"}}))
		headers := headerMap(host.GetResponseHeaders())
		require.Equal(t, "502", headers[":status"])
		require.Equal(t, "no-store", headers["cache-control"])
		host.CompleteHttp()
	})
}

func TestSignedAgentCardPreserveModeDoesNotRewriteBytes(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(testConfig(t, false))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		host.InitHttp()
		host.CallOnHttpRequestHeaders([][2]string{{":method", "GET"}, {":scheme", "https"}, {":path", canonicalAgentCardPath}, {":authority", "agents.example.com"}})
		host.CallOnHttpResponseHeaders([][2]string{{":status", "200"}, {"content-type", "application/json"}})
		body := []byte(`{ "supportedInterfaces": [{"url":"https://agents.example.com/a2a","protocolBinding":"JSONRPC","protocolVersion":"1.0"}], "signatures": [{"protected":"abc","signature":"def"}] }`)
		host.CallOnHttpResponseBody(body)
		require.True(t, bytes.Equal(body, host.GetResponseBody()))
		host.CompleteHttp()
	})
}

func TestAgentCardWithoutRewriteRequiresConfiguredEndpoint(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(testConfigWithCardRewrite(t, false, false))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		host.InitHttp()
		host.CallOnHttpRequestHeaders([][2]string{{":method", "GET"}, {":scheme", "https"}, {":path", canonicalAgentCardPath}, {":authority", "agents.example.com"}})
		host.CallOnHttpResponseHeaders([][2]string{{":status", "200"}, {"content-type", "application/json"}})
		body := []byte(`{"supportedInterfaces":[{"url":"https://agents.example.com/a2a","protocolBinding":"JSONRPC","protocolVersion":"1.0"}]}`)
		host.CallOnHttpResponseBody(body)
		require.Equal(t, "200", headerMap(host.GetResponseHeaders())[":status"])
		require.True(t, bytes.Equal(body, host.GetResponseBody()))
		host.CompleteHttp()
	})
}

func TestSignedAgentCardPreserveRejectsUntrustedEndpoint(t *testing.T) {
	for _, endpoint := range []string{"https://10.0.0.1/a2a", "https://other-public.example.com/a2a"} {
		test.RunGoTest(t, func(t *testing.T) {
			host, status := test.NewTestHost(testConfig(t, false))
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)
			host.InitHttp()
			host.CallOnHttpRequestHeaders([][2]string{{":method", "GET"}, {":scheme", "https"}, {":path", canonicalAgentCardPath}, {":authority", "agents.example.com"}})
			host.CallOnHttpResponseHeaders([][2]string{{":status", "200"}, {"content-type", "application/json"}})
			body := fmt.Sprintf(`{"supportedInterfaces":[{"url":%q,"protocolBinding":"JSONRPC","protocolVersion":"1.0"}],"signatures":[{"protected":"abc","signature":"def"}]}`, endpoint)
			host.CallOnHttpResponseBody([]byte(body))
			require.Equal(t, "502", headerMap(host.GetResponseHeaders())[":status"])
			host.CompleteHttp()
		})
	}
}

func TestMalformedAgentCardSignaturesFailClosed(t *testing.T) {
	for _, signatures := range []string{`null`, `{}`, `true`, `[{"protected":"abc"}]`, `[{"signature":"def"}]`} {
		test.RunGoTest(t, func(t *testing.T) {
			host, status := test.NewTestHost(testConfig(t, false))
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)
			host.InitHttp()
			host.CallOnHttpRequestHeaders([][2]string{{":method", "GET"}, {":scheme", "https"}, {":path", canonicalAgentCardPath}, {":authority", "agents.example.com"}})
			host.CallOnHttpResponseHeaders([][2]string{{":status", "200"}, {"content-type", "application/json"}})
			body := `{"supportedInterfaces":[{"url":"http://internal-agent/a2a","protocolBinding":"JSONRPC","protocolVersion":"1.0"}],"signatures":` + signatures + `}`
			host.CallOnHttpResponseBody([]byte(body))
			require.Equal(t, "502", headerMap(host.GetResponseHeaders())[":status"])
			host.CompleteHttp()
		})
	}
}

func TestNonAgentCardResponseBodyIsUnchanged(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(testConfig(t, false))
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)
		host.InitHttp()
		host.CallOnHttpRequestHeaders([][2]string{{":method", "GET"}, {":scheme", "https"}, {":path", "/health"}, {":authority", "agents.example.com"}})
		host.CallOnHttpResponseHeaders([][2]string{{":status", "200"}, {"content-type", "application/json"}})
		body := []byte(`{"url":"https://127.0.0.1/not-a-card","unknown":true}`)
		host.CallOnHttpResponseBody(body)
		require.True(t, bytes.Equal(body, host.GetResponseBody()))
		host.CompleteHttp()
	})
}

func headerMap(headers [][2]string) map[string]string {
	result := make(map[string]string, len(headers))
	for _, header := range headers {
		result[strings.ToLower(header[0])] = header[1]
	}
	return result
}
