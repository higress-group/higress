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
	"encoding/json"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

// 测试配置：基本工作流配置
var basicWorkflowConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"env": map[string]interface{}{
			"timeout":   5000,
			"max_depth": 100,
		},
		"workflow": map[string]interface{}{
			"edges": []map[string]interface{}{
				{
					"source": "start",
					"target": "A",
				},
				{
					"source": "A",
					"target": "end",
				},
			},
			"nodes": []map[string]interface{}{
				{
					"name":           "A",
					"service_name":   "test-service.static",
					"service_port":   80,
					"service_path":   "/api/test",
					"service_method": "POST",
					"service_body_tmpl": map[string]interface{}{
						"message": "hello",
						"data":    "",
					},
					"service_body_replace_keys": []map[string]interface{}{
						{
							"from": "start||message",
							"to":   "data",
						},
					},
					"service_headers": []map[string]interface{}{
						{
							"key":   "Content-Type",
							"value": "application/json",
						},
					},
				},
			},
		},
	})
	return data
}()

// 测试配置：条件分支工作流配置
var conditionalWorkflowConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"env": map[string]interface{}{
			"timeout":   3000,
			"max_depth": 50,
		},
		"workflow": map[string]interface{}{
			"edges": []map[string]interface{}{
				{
					"source": "start",
					"target": "A",
				},
				{
					"source":      "A",
					"target":      "end",
					"conditional": "gt {{A||score}} 0.5",
				},
				{
					"source":      "A",
					"target":      "B",
					"conditional": "lt {{A||score}} 0.5",
				},
				{
					"source": "B",
					"target": "end",
				},
			},
			"nodes": []map[string]interface{}{
				{
					"name":           "A",
					"service_name":   "service-a.static",
					"service_port":   80,
					"service_path":   "/api/score",
					"service_method": "GET",
				},
				{
					"name":           "B",
					"service_name":   "service-b.static",
					"service_port":   80,
					"service_path":   "/api/fallback",
					"service_method": "POST",
					"service_body_tmpl": map[string]interface{}{
						"fallback": "default",
					},
				},
			},
		},
	})
	return data
}()

// 测试配置：并行执行工作流配置
var parallelWorkflowConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"env": map[string]interface{}{
			"timeout":   5000,
			"max_depth": 100,
		},
		"workflow": map[string]interface{}{
			"edges": []map[string]interface{}{
				{
					"source": "start",
					"target": "A",
				},
				{
					"source": "start",
					"target": "B",
				},
				{
					"source": "start",
					"target": "C",
				},
				{
					"source": "A",
					"target": "D",
				},
				{
					"source": "B",
					"target": "D",
				},
				{
					"source": "C",
					"target": "D",
				},
				{
					"source": "D",
					"target": "end",
				},
			},
			"nodes": []map[string]interface{}{
				{
					"name":           "A",
					"service_name":   "service-a.static",
					"service_port":   80,
					"service_path":   "/api/a",
					"service_method": "GET",
				},
				{
					"name":           "B",
					"service_name":   "service-b.static",
					"service_port":   80,
					"service_path":   "/api/b",
					"service_method": "GET",
				},
				{
					"name":           "C",
					"service_name":   "service-c.static",
					"service_port":   80,
					"service_path":   "/api/c",
					"service_method": "GET",
				},
				{
					"name":           "D",
					"service_name":   "service-d.static",
					"service_port":   80,
					"service_path":   "/api/d",
					"service_method": "POST",
					"service_body_tmpl": map[string]interface{}{
						"a_result": "",
						"b_result": "",
						"c_result": "",
					},
					"service_body_replace_keys": []map[string]interface{}{
						{
							"from": "A||result",
							"to":   "a_result",
						},
						{
							"from": "B||result",
							"to":   "b_result",
						},
						{
							"from": "C||result",
							"to":   "c_result",
						},
					},
				},
			},
		},
	})
	return data
}()

// 测试配置：continue 工作流配置
var continueWorkflowConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"env": map[string]interface{}{
			"timeout":   5000,
			"max_depth": 100,
		},
		"workflow": map[string]interface{}{
			"edges": []map[string]interface{}{
				{
					"source": "start",
					"target": "A",
				},
				{
					"source": "A",
					"target": "continue",
				},
			},
			"nodes": []map[string]interface{}{
				{
					"name":           "A",
					"service_name":   "service-a.static",
					"service_port":   80,
					"service_path":   "/api/process",
					"service_method": "POST",
					"service_body_tmpl": map[string]interface{}{
						"processed": true,
					},
				},
			},
		},
	})
	return data
}()

func TestParseConfig(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// 测试基本工作流配置解析
		t.Run("basic workflow config", func(t *testing.T) {
			host, status := test.NewTestHost(basicWorkflowConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试条件分支工作流配置解析
		t.Run("conditional workflow config", func(t *testing.T) {
			host, status := test.NewTestHost(conditionalWorkflowConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试并行执行工作流配置解析
		t.Run("parallel workflow config", func(t *testing.T) {
			host, status := test.NewTestHost(parallelWorkflowConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试 continue 工作流配置解析
		t.Run("continue workflow config", func(t *testing.T) {
			host, status := test.NewTestHost(continueWorkflowConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})
	})
}

func TestOnHttpRequestBody(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试基本工作流执行
		t.Run("basic workflow execution", func(t *testing.T) {
			host, status := test.NewTestHost(basicWorkflowConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求体
			requestBody := []byte(`{"message": "test message"}`)
			action := host.CallOnHttpRequestBody(requestBody)

			// 应该返回 ActionPause，因为需要等待外部 HTTP 调用完成
			require.Equal(t, types.ActionPause, action)

			// 模拟外部服务的 HTTP 调用响应
			// 模拟成功响应
			host.CallOnHttpCall([][2]string{
				{"Content-Type", "application/json"},
				{":status", "200"},
			}, []byte(`{"result": "success", "data": "processed"}`))

			// 检查插件的响应状态
			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			// 如果插件发送了响应，验证响应内容
			require.Equal(t, uint32(200), localResponse.StatusCode)
			require.Contains(t, string(localResponse.Data), "success")
			require.NotContains(t, localResponse.Headers, [2]string{}, "回调响应头中不应出现空键值对")

			host.CompleteHttp()
		})

		// 测试条件分支工作流执行
		t.Run("conditional workflow execution", func(t *testing.T) {
			host, status := test.NewTestHost(conditionalWorkflowConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求体
			requestBody := []byte(`{"input": "test"}`)
			action := host.CallOnHttpRequestBody(requestBody)

			// 应该返回 ActionPause，因为需要等待外部 HTTP 调用完成
			require.Equal(t, types.ActionPause, action)

			// 模拟外部服务的 HTTP 调用响应
			// 模拟成功响应
			host.CallOnHttpCall([][2]string{
				{"Content-Type", "application/json"},
				{":status", "200"},
			}, []byte(`{"score": 0.8}`))

			// 检查插件的响应状态
			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			// 如果插件发送了响应，验证响应内容
			require.Equal(t, uint32(200), localResponse.StatusCode)

			host.CompleteHttp()
		})

		// 测试并行执行工作流执行
		t.Run("parallel workflow execution", func(t *testing.T) {
			host, status := test.NewTestHost(parallelWorkflowConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求体
			requestBody := []byte(`{"data": "test data"}`)
			action := host.CallOnHttpRequestBody(requestBody)

			// 应该返回 ActionPause，因为需要等待外部 HTTP 调用完成
			require.Equal(t, types.ActionPause, action)

			// 模拟外部服务的 HTTP 调用响应
			// 模拟 A 服务的响应
			host.CallOnHttpCall([][2]string{
				{"Content-Type", "application/json"},
				{":status", "200"},
			}, []byte(`{"result": "a_result"}`))

			// 模拟 B 服务的响应
			host.CallOnHttpCall([][2]string{
				{"Content-Type", "application/json"},
				{":status", "200"},
			}, []byte(`{"result": "b_result"}`))

			// 模拟 C 服务的响应
			host.CallOnHttpCall([][2]string{
				{"Content-Type", "application/json"},
				{":status", "200"},
			}, []byte(`{"result": "c_result"}`))

			// 模拟 D 服务的响应（这是汇聚节点）
			host.CallOnHttpCall([][2]string{
				{"Content-Type", "application/json"},
				{":status", "200"},
			}, []byte(`{"final_result": "success"}`))

			// 检查插件的响应状态
			localResponse := host.GetLocalResponse()
			require.NotNil(t, localResponse)
			// 如果插件发送了响应，验证响应内容
			require.Equal(t, uint32(200), localResponse.StatusCode)

			host.CompleteHttp()
		})

		// 测试 continue 工作流执行
		t.Run("continue workflow execution", func(t *testing.T) {
			host, status := test.NewTestHost(continueWorkflowConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求体
			requestBody := []byte(`{"process": true}`)
			action := host.CallOnHttpRequestBody(requestBody)

			// 应该返回 ActionPause，因为需要等待外部 HTTP 调用完成
			require.Equal(t, types.ActionPause, action)

			// 模拟外部服务的 HTTP 调用响应
			// 模拟成功响应
			host.CallOnHttpCall([][2]string{
				{"Content-Type", "application/json"},
				{":status", "200"},
			}, []byte(`{"processed": true, "status": "success"}`))

			// 检查插件的响应状态
			action = host.GetHttpStreamAction()
			require.Equal(t, types.ActionContinue, action)
			host.CompleteHttp()
		})
	})
}
