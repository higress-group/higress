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
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

func TestOnHttpRequestHeaders(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试 HTTP/1.1 协议（应该直接通过）
		t.Run("HTTP/1.1 protocol", func(t *testing.T) {
			host, status := test.NewTestHost(nil)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 模拟 HTTP/1.1 请求
			host.SetProperty([]string{"request", "protocol"}, []byte("HTTP/1.1"))

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":scheme", "http"},
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "GET"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			localResponse := host.GetLocalResponse()
			require.Nil(t, localResponse, "HTTP/1.1 request should pass through")

			host.CompleteHttp()
		})

		// 测试 HTTP 协议（非 HTTPS，应该直接通过）
		t.Run("HTTP scheme", func(t *testing.T) {
			host, status := test.NewTestHost(nil)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 模拟 HTTP 请求
			host.SetProperty([]string{"request", "protocol"}, []byte("HTTP/2"))

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "GET"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			localResponse := host.GetLocalResponse()
			require.Nil(t, localResponse, "HTTP request should pass through")

			host.CompleteHttp()
		})

		// 测试 gRPC 请求（应该直接通过）
		t.Run("gRPC request", func(t *testing.T) {
			host, status := test.NewTestHost(nil)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 模拟 gRPC 请求
			host.SetProperty([]string{"request", "protocol"}, []byte("HTTP/2"))

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":scheme", "https"},
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "POST"},
				{"content-type", "application/grpc"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			localResponse := host.GetLocalResponse()
			require.Nil(t, localResponse, "gRPC request should pass through")

			host.CompleteHttp()
		})

		// 测试 SNI 和 Host 匹配的情况（应该通过）
		t.Run("SNI matches Host", func(t *testing.T) {
			host, status := test.NewTestHost(nil)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 模拟 HTTPS 请求，SNI 和 Host 匹配
			host.SetProperty([]string{"request", "protocol"}, []byte("HTTP/2"))
			host.SetProperty([]string{"connection", "requested_server_name"}, []byte("example.com"))

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":scheme", "https"},
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "GET"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			localResponse := host.GetLocalResponse()
			require.Nil(t, localResponse, "Matching SNI and Host should pass through")

			host.CompleteHttp()
		})

		// 测试 SNI 和 Host 不匹配的情况（非通配符，应该被阻止）
		t.Run("SNI mismatches Host non-wildcard", func(t *testing.T) {
			host, status := test.NewTestHost(nil)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 模拟 HTTPS 请求，SNI 和 Host 不匹配
			host.SetProperty([]string{"request", "protocol"}, []byte("HTTP/2"))
			host.SetProperty([]string{"connection", "requested_server_name"}, []byte("evil.com"))

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":scheme", "https"},
				{":authority", "example.com"},
				{":path", "/test"},
				{":method", "GET"},
				{"content-type", "text/plain"},
			})

			require.Equal(t, types.ActionPause, action)
			require.Equal(t, types.ActionPause, host.GetHttpStreamAction())

			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			require.Equal(t, uint32(421), localResponse.StatusCode) // 421 Misdirected Request
			require.Equal(t, "Misdirected Request", string(localResponse.Data))

			host.CompleteHttp()
		})

		// 测试通配符 SNI 匹配的情况（应该通过）
		t.Run("Wildcard SNI matches Host", func(t *testing.T) {
			host, status := test.NewTestHost(nil)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 模拟 HTTPS 请求，通配符 SNI 匹配 Host
			host.SetProperty([]string{"request", "protocol"}, []byte("HTTP/2"))
			host.SetProperty([]string{"connection", "requested_server_name"}, []byte("*.example.com"))

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":scheme", "https"},
				{":authority", "sub.example.com"},
				{":path", "/test"},
				{":method", "GET"},
				{"content-type", "text/plain"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			localResponse := host.GetLocalResponse()
			require.Nil(t, localResponse, "Wildcard SNI matching Host should pass through")

			host.CompleteHttp()
		})

		// 测试通配符 SNI 不匹配的情况（应该被阻止）
		t.Run("Wildcard SNI mismatches Host", func(t *testing.T) {
			host, status := test.NewTestHost(nil)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 模拟 HTTPS 请求，通配符 SNI 不匹配 Host
			host.SetProperty([]string{"request", "protocol"}, []byte("HTTP/2"))
			host.SetProperty([]string{"connection", "requested_server_name"}, []byte("*.example.com"))

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":scheme", "https"},
				{":authority", "other.com"},
				{":path", "/test"},
				{":method", "GET"},
				{"content-type", "text/plain"},
			})

			require.Equal(t, types.ActionPause, action)
			require.Equal(t, types.ActionPause, host.GetHttpStreamAction())

			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			require.Equal(t, uint32(421), localResponse.StatusCode) // 421 Misdirected Request
			require.Equal(t, "Misdirected Request", string(localResponse.Data))

			host.CompleteHttp()
		})

		// 测试带端口的 Host（应该正确处理）
		t.Run("Host with port", func(t *testing.T) {
			host, status := test.NewTestHost(nil)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 模拟 HTTPS 请求，Host 带端口
			host.SetProperty([]string{"request", "protocol"}, []byte("HTTP/2"))
			host.SetProperty([]string{"connection", "requested_server_name"}, []byte("example.com"))

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":scheme", "https"},
				{":authority", "example.com:443"},
				{":path", "/test"},
				{":method", "GET"},
				{"content-type", "text/plain"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			localResponse := host.GetLocalResponse()
			require.Nil(t, localResponse, "Host with port should be handled correctly")

			host.CompleteHttp()
		})
	})
}

func TestStripPortFromHost(t *testing.T) {
	// 测试 stripPortFromHost 函数
	t.Run("host without port", func(t *testing.T) {
		result := stripPortFromHost("example.com")
		require.Equal(t, "example.com", result)
	})

	t.Run("host with port", func(t *testing.T) {
		result := stripPortFromHost("example.com:8080")
		require.Equal(t, "example.com", result)
	})

	t.Run("host with multiple colons", func(t *testing.T) {
		result := stripPortFromHost("example.com:8080:9090")
		require.Equal(t, "example.com:8080", result)
	})

	t.Run("IPv6 host without port", func(t *testing.T) {
		result := stripPortFromHost("[2001:db8::1]")
		require.Equal(t, "[2001:db8::1]", result)
	})

	t.Run("IPv6 host with port", func(t *testing.T) {
		result := stripPortFromHost("[2001:db8::1]:443")
		require.Equal(t, "[2001:db8::1]", result)
	})

	t.Run("IPv6 host with port and multiple colons", func(t *testing.T) {
		result := stripPortFromHost("[2001:db8::1]:443:8080")
		require.Equal(t, "[2001:db8::1]:443", result)
	})

	t.Run("empty host", func(t *testing.T) {
		result := stripPortFromHost("")
		require.Equal(t, "", result)
	})

	t.Run("host with colon at end", func(t *testing.T) {
		result := stripPortFromHost("example.com:")
		require.Equal(t, "example.com", result)
	})

	t.Run("IPv6 host with colon at end", func(t *testing.T) {
		result := stripPortFromHost("[2001:db8::1]:")
		require.Equal(t, "[2001:db8::1]", result)
	})
}
