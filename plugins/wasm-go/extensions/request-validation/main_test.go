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
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

// 测试配置：启用头部验证，使用Draft7
var headerValidationConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"header_schema": `{
			"type": "object",
			"properties": {
				"content-type": {"type": "string"},
				"authorization": {"type": "string"}
			},
			"required": ["content-type"]
		}`,
		"enable_oas3":   true,
		"rejected_code": 400,
		"rejected_msg":  "Invalid headers",
	})
	return data
}()

// 测试配置：启用体部验证，使用Draft4
var bodyValidationConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"body_schema": `{
			"type": "object",
			"properties": {
				"name": {"type": "string"},
				"age": {"type": "integer", "minimum": 0}
			},
			"required": ["name"]
		}`,
		"enable_swagger": true,
		"rejected_code":  422,
		"rejected_msg":   "Invalid request body",
	})
	return data
}()

// 测试配置：同时启用头部和体部验证
var bothValidationConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"header_schema": `{
			"type": "object",
			"properties": {
				"content-type": {"type": "string"}
			},
			"required": ["content-type"]
		}`,
		"body_schema": `{
			"type": "object",
			"properties": {
				"id": {"type": "integer"}
			}
		}`,
		"enable_oas3":   true,
		"rejected_code": 400,
		"rejected_msg":  "Validation failed",
	})
	return data
}()

// 测试配置：禁用所有验证
var noValidationConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"rejected_code": 403,
		"rejected_msg":  "Access denied",
	})
	return data
}()

// 测试配置：无效的JSON Schema
var invalidSchemaConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"header_schema": `{
			"type": "invalid_type",
			"properties": {}
		}`,
		"enable_oas3": true,
	})
	return data
}()

// 测试配置：同时启用swagger和oas3（应该失败）
var conflictingDraftConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"header_schema":  `{"type": "object"}`,
		"enable_swagger": true,
		"enable_oas3":    true,
	})
	return data
}()

func TestParseConfig(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// 测试头部验证配置
		t.Run("header validation config", func(t *testing.T) {
			host, status := test.NewTestHost(headerValidationConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			validationConfig := config.(*Config)
			require.True(t, validationConfig.enableHeaderSchema)
			require.False(t, validationConfig.enableBodySchema)
			require.Equal(t, uint32(400), validationConfig.rejectedCode)
			require.Equal(t, "Invalid headers", validationConfig.rejectedMsg)
			require.NotNil(t, validationConfig.compiler)
		})

		// 测试体部验证配置
		t.Run("body validation config", func(t *testing.T) {
			host, status := test.NewTestHost(bodyValidationConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			validationConfig := config.(*Config)
			require.False(t, validationConfig.enableHeaderSchema)
			require.True(t, validationConfig.enableBodySchema)
			require.Equal(t, uint32(422), validationConfig.rejectedCode)
			require.Equal(t, "Invalid request body", validationConfig.rejectedMsg)
			require.NotNil(t, validationConfig.compiler)
		})

		// 测试同时启用头部和体部验证
		t.Run("both validation config", func(t *testing.T) {
			host, status := test.NewTestHost(bothValidationConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			validationConfig := config.(*Config)
			require.True(t, validationConfig.enableHeaderSchema)
			require.True(t, validationConfig.enableBodySchema)
			require.Equal(t, uint32(400), validationConfig.rejectedCode)
			require.Equal(t, "Validation failed", validationConfig.rejectedMsg)
			require.NotNil(t, validationConfig.compiler)
		})

		// 测试禁用所有验证
		t.Run("no validation config", func(t *testing.T) {
			host, status := test.NewTestHost(noValidationConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			validationConfig := config.(*Config)
			require.False(t, validationConfig.enableHeaderSchema)
			require.False(t, validationConfig.enableBodySchema)
			require.Equal(t, uint32(403), validationConfig.rejectedCode)
			require.Equal(t, "Access denied", validationConfig.rejectedMsg)
			require.NotNil(t, validationConfig.compiler)
		})

		// 测试无效的JSON Schema
		t.Run("invalid schema config", func(t *testing.T) {
			host, status := test.NewTestHost(invalidSchemaConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			validationConfig := config.(*Config)
			require.True(t, validationConfig.enableHeaderSchema)
			require.False(t, validationConfig.enableBodySchema)
		})

		// 测试冲突的draft版本配置
		t.Run("conflicting draft config", func(t *testing.T) {
			host, status := test.NewTestHost(conflictingDraftConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})
	})
}

func TestOnHttpRequestHeaders(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试有效的请求头
		t.Run("valid headers", func(t *testing.T) {
			host, status := test.NewTestHost(headerValidationConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "test.com"},
				{":path", "/api/test"},
				{":method", "POST"},
				{"content-type", "application/json"},
				{"authorization", "Bearer token123"},
			})

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			localResponse := host.GetLocalResponse()
			require.Nil(t, localResponse, "Valid headers should not be rejected")

			host.CompleteHttp()
		})

		// 测试无效的请求头（缺少必需的content-type）
		t.Run("invalid headers - missing required", func(t *testing.T) {
			host, status := test.NewTestHost(headerValidationConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "test.com"},
				{":path", "/api/test"},
				{":method", "POST"},
				{"authorization", "Bearer token123"},
				// 缺少 content-type
			})

			require.Equal(t, types.ActionPause, action)
			require.Equal(t, types.ActionPause, host.GetHttpStreamAction())

			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			require.Equal(t, uint32(400), localResponse.StatusCode)
			require.Equal(t, "Invalid headers", string(localResponse.Data))

			host.CompleteHttp()
		})

		// 测试禁用头部验证
		t.Run("header validation disabled", func(t *testing.T) {
			host, status := test.NewTestHost(noValidationConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "test.com"},
				{":path", "/api/test"},
				{":method", "GET"},
				// 没有验证规则，应该继续
			})

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			localResponse := host.GetLocalResponse()
			require.Nil(t, localResponse)

			host.CompleteHttp()
		})
	})
}

func TestOnHttpRequestBody(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试有效的请求体
		t.Run("valid body", func(t *testing.T) {
			host, status := test.NewTestHost(bodyValidationConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先调用头部处理
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "test.com"},
				{":path", "/api/test"},
				{":method", "POST"},
			})
			require.Equal(t, types.ActionContinue, action)

			// 测试有效的请求体
			validBody := `{"name": "John Doe", "age": 30}`
			action = host.CallOnHttpRequestBody([]byte(validBody))

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			localResponse := host.GetLocalResponse()
			require.Nil(t, localResponse, "Valid body should not be rejected")

			host.CompleteHttp()
		})

		// 测试无效的请求体（缺少必需的name字段）
		t.Run("invalid body - missing required", func(t *testing.T) {
			host, status := test.NewTestHost(bodyValidationConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先调用头部处理
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "test.com"},
				{":path", "/api/test"},
				{":method", "POST"},
			})
			require.Equal(t, types.ActionContinue, action)

			// 测试无效的请求体
			invalidBody := `{"age": 30}`
			action = host.CallOnHttpRequestBody([]byte(invalidBody))

			require.Equal(t, types.ActionPause, action)
			require.Equal(t, types.ActionPause, host.GetHttpStreamAction())

			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			require.Equal(t, uint32(422), localResponse.StatusCode)
			require.Equal(t, "Invalid request body", string(localResponse.Data))

			host.CompleteHttp()
		})

		// 测试无效的请求体（age为负数）
		t.Run("invalid body - invalid value", func(t *testing.T) {
			host, status := test.NewTestHost(bodyValidationConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先调用头部处理
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "test.com"},
				{":path", "/api/test"},
				{":method", "POST"},
			})
			require.Equal(t, types.ActionContinue, action)

			// 测试无效的请求体
			invalidBody := `{"name": "John Doe", "age": -5}`
			action = host.CallOnHttpRequestBody([]byte(invalidBody))

			require.Equal(t, types.ActionPause, action)
			require.Equal(t, types.ActionPause, host.GetHttpStreamAction())

			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			require.Equal(t, uint32(422), localResponse.StatusCode)
			require.Equal(t, "Invalid request body", string(localResponse.Data))

			host.CompleteHttp()
		})

		// 测试禁用体部验证
		t.Run("body validation disabled", func(t *testing.T) {
			host, status := test.NewTestHost(noValidationConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先调用头部处理
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "test.com"},
				{":path", "/api/test"},
				{":method", "POST"},
			})
			require.Equal(t, types.ActionContinue, action)

			// 测试任意请求体
			anyBody := `not a json {{{`
			action = host.CallOnHttpRequestBody([]byte(anyBody))

			require.Equal(t, types.ActionContinue, action)
			require.Equal(t, types.ActionContinue, host.GetHttpStreamAction())

			localResponse := host.GetLocalResponse()
			require.Nil(t, localResponse)

			host.CompleteHttp()
		})
	})
}

func TestDraftVersions(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// 测试Draft4 (Swagger)
		t.Run("draft4 swagger", func(t *testing.T) {
			swaggerConfig := func() json.RawMessage {
				data, _ := json.Marshal(map[string]interface{}{
					"header_schema": `{
						"type": "object",
						"properties": {
							"x-api-key": {"type": "string"}
						}
					}`,
					"enable_swagger": true,
					"rejected_code":  401,
					"rejected_msg":   "Missing API key",
				})
				return data
			}()

			host, status := test.NewTestHost(swaggerConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			validationConfig := config.(*Config)
			require.True(t, validationConfig.enableHeaderSchema)
			require.Equal(t, uint32(401), validationConfig.rejectedCode)
		})

		// 测试Draft7 (OAS3)
		t.Run("draft7 oas3", func(t *testing.T) {
			oas3Config := func() json.RawMessage {
				data, _ := json.Marshal(map[string]interface{}{
					"body_schema": `{
						"type": "object",
						"properties": {
							"email": {"type": "string", "format": "email"}
						}
					}`,
					"enable_oas3":   true,
					"rejected_code": 400,
					"rejected_msg":  "Invalid email format",
				})
				return data
			}()

			host, status := test.NewTestHost(oas3Config)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			validationConfig := config.(*Config)
			require.True(t, validationConfig.enableBodySchema)
			require.Equal(t, uint32(400), validationConfig.rejectedCode)
		})
	})
}
