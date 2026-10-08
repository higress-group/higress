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
	"ai-rag/dashscope"
	"ai-rag/dashvector"
	"encoding/json"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

// 测试配置：基础RAG配置
var basicConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"dashscope": map[string]interface{}{
			"apiKey":      "test-dashscope-key",
			"serviceFQDN": "dashscope-service",
			"servicePort": 8080,
			"serviceHost": "dashscope.example.com",
		},
		"dashvector": map[string]interface{}{
			"apiKey":      "test-dashvector-key",
			"collection":  "test-collection",
			"serviceFQDN": "dashvector-service",
			"servicePort": 8081,
			"serviceHost": "dashvector.example.com",
			"topk":        5,
			"threshold":   0.8,
			"field":       "content",
		},
	})
	return data
}()

// 测试配置：缺少必需字段
var missingRequiredConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"dashscope": map[string]interface{}{
			"apiKey": "test-dashscope-key",
		},
		"dashvector": map[string]interface{}{
			"apiKey": "test-dashvector-key",
		},
	})
	return data
}()

func TestParseConfig(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// 测试基础配置解析
		t.Run("basic config", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)
			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)

			ragConfig := config.(*AIRagConfig)
			require.Equal(t, "test-dashscope-key", ragConfig.DashScopeAPIKey)
			require.Equal(t, "test-dashvector-key", ragConfig.DashVectorAPIKey)
			require.Equal(t, "test-collection", ragConfig.DashVectorCollection)
			require.Equal(t, int32(5), ragConfig.DashVectorTopK)
			require.Equal(t, 0.8, ragConfig.DashVectorThreshold)
			require.Equal(t, "content", ragConfig.DashVectorField)
		})

		// 测试缺少必需字段的配置
		t.Run("missing required config", func(t *testing.T) {
			host, status := test.NewTestHost(missingRequiredConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})
	})
}

func TestOnHttpRequestHeaders(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试请求头处理
		t.Run("request headers processing", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"content-length", "100"},
			})

			// 应该返回ActionContinue
			require.Equal(t, types.ActionContinue, action)
		})
	})
}

func TestOnHttpRequestBody(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试空消息的请求体
		t.Run("empty messages", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			// 设置空消息的请求体
			body := `{"model": "gpt-3.5-turbo", "messages": []}`
			action := host.CallOnHttpRequestBody([]byte(body))

			// 空消息应该直接通过
			require.Equal(t, types.ActionContinue, action)
		})

		// 测试正常RAG流程
		t.Run("normal rag flow", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			// 设置包含消息的请求体
			body := `{"model": "gpt-3.5-turbo", "messages": [{"role": "user", "content": "What is AI?"}]}`
			action := host.CallOnHttpRequestBody([]byte(body))

			// 应该返回ActionPause，等待RAG流程完成
			require.Equal(t, types.ActionPause, action)

			// 模拟DashScope嵌入服务响应
			embeddingResponse := `{
				"output": {
					"embeddings": [{
						"embedding": [0.1, 0.2, 0.3, 0.4, 0.5],
						"text_index": 0
					}]
				},
				"usage": {"total_tokens": 10},
				"request_id": "req-123"
			}`
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(embeddingResponse))

			// 模拟DashVector向量搜索响应
			vectorResponse := `{
				"code": 200,
				"request_id": "req-456",
				"message": "success",
				"output": [{
					"id": "doc1",
					"fields": {"raw": "AI is artificial intelligence"},
					"score": 0.75
				}]
			}`
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(vectorResponse))

			// 获取修改后的请求体
			requestBody := host.GetRequestBody()
			require.NotEmpty(t, requestBody)

			// 解析修改后的请求体，验证RAG增强
			var modifiedRequest Request
			err := json.Unmarshal(requestBody, &modifiedRequest)
			require.NoError(t, err)
			require.Equal(t, "gpt-3.5-turbo", modifiedRequest.Model)

			// 验证消息数量：检索文档(1) + 问题提示(1) = 2
			// 注意：原始消息被清空了，因为 messageLength-1 = 0
			require.Len(t, modifiedRequest.Messages, 2)

			// 验证第一个消息（检索到的文档）
			require.Equal(t, "user", modifiedRequest.Messages[0].Role)
			require.Equal(t, "AI is artificial intelligence", modifiedRequest.Messages[0].Content)

			// 验证第二个消息（问题提示）
			require.Equal(t, "user", modifiedRequest.Messages[1].Role)
			require.Equal(t, "现在，请回答以下问题：\nWhat is AI?", modifiedRequest.Messages[1].Content)

			host.CompleteHttp()
		})

		// 测试DashScope返回错误或者空嵌入向量时不会Panic且能Resume请求
		t.Run("dashscope error does not panic", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			body := `{"model": "gpt-3.5-turbo", "messages": [{"role": "user", "content": "What is AI?"}]}`
			action := host.CallOnHttpRequestBody([]byte(body))
			require.Equal(t, types.ActionPause, action)

			// 模拟DashScope返回401错误
			errorResponse := `{"code": "InvalidApiKey", "message": "Requests with invalid API Key"}`
			require.NotPanics(t, func() {
				host.CallOnHttpCall([][2]string{
					{":status", "401"},
					{"content-type", "application/json"},
				}, []byte(errorResponse))
			})

			host.CompleteHttp()
		})

		t.Run("dashscope empty embeddings does not panic", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			body := `{"model": "gpt-3.5-turbo", "messages": [{"role": "user", "content": "What is AI?"}]}`
			action := host.CallOnHttpRequestBody([]byte(body))
			require.Equal(t, types.ActionPause, action)

			emptyResponse := `{"output": {"embeddings": []}}`
			require.NotPanics(t, func() {
				host.CallOnHttpCall([][2]string{
					{":status", "200"},
					{"content-type", "application/json"},
				}, []byte(emptyResponse))
			})

			host.CompleteHttp()
		})
	})
}

func TestOnHttpResponseHeaders(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试RAG召回标记
		t.Run("rag recall header", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
			})

			// 设置请求体
			body := `{"model": "gpt-3.5-turbo", "messages": [{"role": "user", "content": "What is AI?"}]}`
			host.CallOnHttpRequestBody([]byte(body))

			// 模拟DashScope嵌入服务响应
			embeddingResponse := `{
				"output": {
					"embeddings": [{
						"embedding": [0.1, 0.2, 0.3, 0.4, 0.5],
						"text_index": 0
					}]
				},
				"usage": {"total_tokens": 10},
				"request_id": "req-123"
			}`
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(embeddingResponse))

			// 模拟DashVector向量搜索响应
			vectorResponse := `{
				"code": 200,
				"request_id": "req-456",
				"message": "success",
				"output": [{
					"id": "doc1",
					"fields": {"raw": "AI is artificial intelligence"},
					"score": 0.75
				}]
			}`
			host.CallOnHttpCall([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			}, []byte(vectorResponse))

			// 设置响应头
			action := host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"content-type", "application/json"},
			})

			// 应该返回ActionContinue
			require.Equal(t, types.ActionContinue, action)

			// 验证响应头包含RAG召回标记
			require.True(t, test.HasHeaderWithValue(host.GetResponseHeaders(), "x-envoy-rag-recall", "true"))

			host.CompleteHttp()
		})
	})
}

func TestStructs(t *testing.T) {
	// 测试Request结构体
	t.Run("Request struct", func(t *testing.T) {
		request := Request{
			Model:            "gpt-3.5-turbo",
			Messages:         []Message{{Role: "user", Content: "Hello"}},
			FrequencyPenalty: 0.0,
			PresencePenalty:  0.0,
			Stream:           false,
			Temperature:      0.7,
			Topp:             1,
		}
		require.Equal(t, "gpt-3.5-turbo", request.Model)
		require.Len(t, request.Messages, 1)
		require.Equal(t, "user", request.Messages[0].Role)
		require.Equal(t, "Hello", request.Messages[0].Content)
		require.Equal(t, 0.7, request.Temperature)
	})

	// 测试Message结构体
	t.Run("Message struct", func(t *testing.T) {
		message := Message{
			Role:    "assistant",
			Content: "Hello! How can I help you?",
		}
		require.Equal(t, "assistant", message.Role)
		require.Equal(t, "Hello! How can I help you?", message.Content)
	})
}

func TestDashScopeTypes(t *testing.T) {
	// 测试DashScope Request结构体
	t.Run("DashScope Request", func(t *testing.T) {
		request := dashscope.Request{
			Model: "text-embedding-v2",
			Input: dashscope.Input{
				Texts: []string{"Hello, world"},
			},
			Parameter: dashscope.Parameter{
				TextType: "query",
			},
		}
		require.Equal(t, "text-embedding-v2", request.Model)
		require.Len(t, request.Input.Texts, 1)
		require.Equal(t, "Hello, world", request.Input.Texts[0])
		require.Equal(t, "query", request.Parameter.TextType)
	})

	// 测试DashScope Response结构体
	t.Run("DashScope Response", func(t *testing.T) {
		response := dashscope.Response{
			Output: dashscope.Output{
				Embeddings: []dashscope.Embedding{
					{
						Embedding: []float32{0.1, 0.2, 0.3},
						TextIndex: 0,
					},
				},
			},
			Usage: dashscope.Usage{
				TotalTokens: 10,
			},
			RequestID: "req-123",
		}
		require.Equal(t, "req-123", response.RequestID)
		require.Equal(t, int32(10), response.Usage.TotalTokens)
		require.Len(t, response.Output.Embeddings, 1)
		require.Len(t, response.Output.Embeddings[0].Embedding, 3)
	})
}

func TestDashVectorTypes(t *testing.T) {
	// 测试DashVector Request结构体
	t.Run("DashVector Request", func(t *testing.T) {
		request := dashvector.Request{
			TopK:         5,
			OutputFileds: []string{"content", "title"},
			Vector:       []float32{0.1, 0.2, 0.3, 0.4, 0.5},
		}
		require.Equal(t, int32(5), request.TopK)
		require.Len(t, request.OutputFileds, 2)
		require.Len(t, request.Vector, 5)
	})

	// 测试DashVector Response结构体
	t.Run("DashVector Response", func(t *testing.T) {
		response := dashvector.Response{
			Code:      200,
			RequestID: "req-456",
			Message:   "success",
			Output: []dashvector.OutputObject{
				{
					ID: "doc1",
					Fields: dashvector.FieldObject{
						Raw: "AI is artificial intelligence",
					},
					Score: 0.75,
				},
			},
		}
		require.Equal(t, int32(200), response.Code)
		require.Equal(t, "req-456", response.RequestID)
		require.Equal(t, "success", response.Message)
		require.Len(t, response.Output, 1)
		require.Equal(t, "doc1", response.Output[0].ID)
		require.Equal(t, "AI is artificial intelligence", response.Output[0].Fields.Raw)
		require.Equal(t, float32(0.75), response.Output[0].Score)
	})
}
