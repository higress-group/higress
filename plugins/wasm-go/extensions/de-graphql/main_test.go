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
	"encoding/json"
	"net/http"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

// 测试配置：基本配置
var basicConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"gql": `query ($owner: String!, $name: String!) {
			repository(owner: $owner, name: $name) {
				name
				forkCount
				description
			}
		}`,
		"endpoint": "/graphql",
		"timeout":  5000,
		"domain":   "api.github.com",
	})
	return data
}()

// 测试配置：带不同类型变量的配置
var multiTypeConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"gql": `query ($id: Int!, $enabled: Boolean!, $score: Float!, $title: String!) {
			item(id: $id, enabled: $enabled, score: $score, title: $title) {
				id
				name
				status
			}
		}`,
		"endpoint": "/api/graphql",
		"timeout":  3000,
		"domain":   "example.com",
	})
	return data
}()

// 测试配置：可选参数配置
var optionalParamsConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"gql": `query ($id: String, $name: String) {
			user(id: $id, name: $name) {
				id
				name
				email
			}
		}`,
		"endpoint": "/graphql",
		"timeout":  5000,
		"domain":   "api.example.com",
	})
	return data
}()

// 测试配置：默认值配置
var defaultConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"gql": `query ($owner: String!) {
			repository(owner: $owner) {
				name
			}
		}`,
	})
	return data
}()

// 测试配置：无效 GraphQL 配置
var invalidGqlConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"gql":      "",
		"endpoint": "/graphql",
		"timeout":  5000,
		"domain":   "api.github.com",
	})
	return data
}()

func TestParseConfig(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// 测试基本配置解析
		t.Run("basic config", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试多类型变量配置解析
		t.Run("multi type config", func(t *testing.T) {
			host, status := test.NewTestHost(multiTypeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试可选参数配置解析
		t.Run("optional params config", func(t *testing.T) {
			host, status := test.NewTestHost(optionalParamsConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试默认值配置解析
		t.Run("default config", func(t *testing.T) {
			host, status := test.NewTestHost(defaultConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试无效 GraphQL 配置解析
		t.Run("invalid gql config", func(t *testing.T) {
			host, status := test.NewTestHost(invalidGqlConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.Nil(t, config)
		})
	})
}

func TestOnHttpRequestHeaders(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试基本 GraphQL 查询请求头处理
		t.Run("basic graphql query", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，包含查询参数
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api?owner=alibaba&name=higress"},
				{":method", "GET"},
				{"authorization", "Bearer token123"},
			})

			// 由于需要调用外部 GraphQL 服务，应该返回 ActionPause
			require.Equal(t, types.ActionPause, action)

			// 模拟外部 GraphQL 服务的HTTP调用响应
			// 模拟成功响应（200状态码）
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(`{"data":{"repository":{"name":"higress","forkCount":149,"description":"Next-generation Cloud Native Gateway"}}}`))

			host.CompleteHttp()
		})

		// 测试多类型变量查询请求头处理
		t.Run("multi type variables query", func(t *testing.T) {
			host, status := test.NewTestHost(multiTypeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，包含不同类型的查询参数
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api?id=123&enabled=true&score=95.5&title=Test Item"},
				{":method", "GET"},
			})

			// 由于需要调用外部 GraphQL 服务，应该返回 ActionPause
			require.Equal(t, types.ActionPause, action)

			// 模拟外部 GraphQL 服务的HTTP调用响应
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(`{"data":{"item":{"id":123,"name":"Test Item","status":"active"}}}`))

			host.CompleteHttp()
		})

		// 测试可选参数查询请求头处理
		t.Run("optional parameters query", func(t *testing.T) {
			host, status := test.NewTestHost(optionalParamsConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，只包含部分查询参数
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api?name=john"},
				{":method", "GET"},
			})

			// 由于需要调用外部 GraphQL 服务，应该返回 ActionPause
			require.Equal(t, types.ActionPause, action)

			// 模拟外部 GraphQL 服务的HTTP调用响应
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(`{"data":{"user":{"id":"user123","name":"john","email":"john@example.com"}}}`))

			host.CompleteHttp()
		})

		// 测试无查询参数的请求头处理
		t.Run("no query parameters", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，不包含查询参数
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api"},
				{":method", "GET"},
			})

			// 由于需要调用外部 GraphQL 服务，应该返回 ActionPause
			require.Equal(t, types.ActionPause, action)

			// 模拟外部 GraphQL 服务的HTTP调用响应
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(`{"data":{"repository":null}}`))

			host.CompleteHttp()
		})

		// 测试 POST 请求的请求头处理
		t.Run("POST request", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头，POST 请求
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api?owner=alibaba&name=higress"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			// 由于需要调用外部 GraphQL 服务，应该返回 ActionPause
			require.Equal(t, types.ActionPause, action)

			// 模拟外部 GraphQL 服务的HTTP调用响应
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(`{"data":{"repository":{"name":"higress","forkCount":149,"description":"Next-generation Cloud Native Gateway"}}}`))

			host.CompleteHttp()
		})
	})
}

func TestOnHttpRequestBody(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试请求体处理
		t.Run("request body processing", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先处理请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api?owner=alibaba&name=higress"},
				{":method", "POST"},
			})

			// 处理请求体
			requestBody := `{"additional": "data"}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			// 请求体处理应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})
	})
}

func TestOnHttpResponseHeaders(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试响应头处理
		t.Run("response headers processing", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先处理请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api?owner=alibaba&name=higress"},
				{":method", "GET"},
			})

			// 处理响应头
			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})

			// 响应头处理应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})
	})
}

func TestOnHttpResponseBody(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试响应体处理
		t.Run("response body processing", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先处理请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api?owner=alibaba&name=higress"},
				{":method", "GET"},
			})

			// 处理响应体
			responseBody := `{"data":{"repository":{"name":"higress","forkCount":149,"description":"Next-generation Cloud Native Gateway"}}}`
			action := host.CallOnHttpResponseBody([]byte(responseBody))

			// 响应体处理应该返回 ActionContinue
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})
	})
}

// TestOnHttpRequestHeadersRejectsInvalidVariables verifies that a request whose
// variable value cannot be represented as the declared GraphQL type is rejected
// with 400 instead of being forwarded to the GraphQL endpoint.
func TestOnHttpRequestHeadersRejectsInvalidVariables(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// `?id=1,"admin":true` is what the previous implementation copied into
		// the variables object verbatim, silently adding a member to the
		// GraphQL operation. It must now be rejected before the callout.
		t.Run("int variable carrying extra JSON", func(t *testing.T) {
			host, status := test.NewTestHost(multiTypeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", `/api/graphql?id=1,"admin":true&enabled=true&score=1.0&title=ok`},
				{":method", "GET"},
			})
			require.Equal(t, types.ActionContinue, action)

			local := host.GetLocalResponse()
			require.NotNil(t, local, "the malformed variable must produce a local response")
			require.Equal(t, uint32(http.StatusBadRequest), local.StatusCode)
			require.Contains(t, string(local.Data), "Int")

			host.CompleteHttp()
		})

		t.Run("int variable that is not numeric", func(t *testing.T) {
			host, status := test.NewTestHost(multiTypeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", `/api/graphql?id=abc&enabled=true&score=1.0&title=ok`},
				{":method", "GET"},
			})
			require.Equal(t, types.ActionContinue, action)

			local := host.GetLocalResponse()
			require.NotNil(t, local)
			require.Equal(t, uint32(http.StatusBadRequest), local.StatusCode)

			host.CompleteHttp()
		})

		t.Run("valid typed variables are forwarded", func(t *testing.T) {
			host, status := test.NewTestHost(multiTypeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", `/api/graphql?id=7&enabled=false&score=2.5&title=ok`},
				{":method", "GET"},
			})
			require.Equal(t, types.ActionPause, action)
			require.Nil(t, host.GetLocalResponse())

			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(`{"data":{"item":{"id":"7"}}}`))

			host.CompleteHttp()
		})
	})
}
