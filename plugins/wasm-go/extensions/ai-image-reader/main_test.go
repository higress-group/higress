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

// 测试配置：基本DashScope OCR配置
// modelPatterns 白名单：与 main.go 的模型过滤逻辑配套，
// 测试请求体中的 model 字段必须匹配白名单才会触发 OCR/剥离逻辑
var basicDashScopeConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"type":         "dashscope",
		"apiKey":       "test-api-key-123",
		"serviceName":  "ocr-service",
		"serviceHost":  "dashscope.aliyuncs.com",
		"servicePort":  443,
		"timeout":      10000,
		"model":        "qwen-vl-ocr",
		"modelPatterns": []string{"^deepseek-ai/DeepSeek-V4-Flash$"},
	})
	return data
}()

// 测试配置：最小DashScope配置（使用默认值）
var minimalDashScopeConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"type":        "dashscope",
		"apiKey":      "minimal-api-key",
		"serviceName": "ocr-service",
	})
	return data
}()

// 测试配置：自定义端口和超时配置
var customPortTimeoutConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"type":        "dashscope",
		"apiKey":      "custom-api-key",
		"serviceName": "ocr-service",
		"serviceHost": "custom.dashscope.com",
		"servicePort": 8443,
		"timeout":     30000,
		"model":       "qwen-vl-ocr",
	})
	return data
}()

// 测试配置：自定义模型配置
var customModelConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"type":        "dashscope",
		"apiKey":      "model-api-key",
		"serviceName": "ocr-service",
		"serviceHost": "dashscope.aliyuncs.com",
		"servicePort": 443,
		"timeout":     15000,
		"model":       "custom-ocr-model",
	})
	return data
}()

func TestParseConfig(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// 测试基本DashScope配置解析
		t.Run("basic dashscope config", func(t *testing.T) {
			host, status := test.NewTestHost(basicDashScopeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试最小DashScope配置解析（使用默认值）
		t.Run("minimal dashscope config", func(t *testing.T) {
			host, status := test.NewTestHost(minimalDashScopeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试自定义端口和超时配置解析
		t.Run("custom port timeout config", func(t *testing.T) {
			host, status := test.NewTestHost(customPortTimeoutConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试自定义模型配置解析
		t.Run("custom model config", func(t *testing.T) {
			host, status := test.NewTestHost(customModelConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})
	})
}

func TestOnHttpRequestHeaders(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试JSON内容类型的请求头处理
		t.Run("JSON content type headers", func(t *testing.T) {
			host, status := test.NewTestHost(basicDashScopeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置JSON内容类型的请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			// 应该返回ActionContinue，因为禁用了重路由但允许继续处理
			require.Equal(t, types.ActionContinue, action)
		})

		// 测试非JSON内容类型的请求头处理
		t.Run("non-JSON content type headers", func(t *testing.T) {
			host, status := test.NewTestHost(basicDashScopeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置非JSON内容类型的请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"content-type", "text/plain"},
			})

			// 应该返回ActionContinue，但不会读取请求体
			require.Equal(t, types.ActionContinue, action)
		})

		// 测试缺少content-type的请求头处理
		t.Run("missing content type headers", func(t *testing.T) {
			host, status := test.NewTestHost(basicDashScopeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置缺少content-type的请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
			})

			// 应该返回ActionContinue
			require.Equal(t, types.ActionContinue, action)
		})
	})
}

func TestOnHttpRequestBody(t *testing.T) {
	// 注意：使用 RunGoTest 而非 RunTest——目录下的 plugin.wasm 是旧的本地构建产物
	//（不含 modelPatterns 白名单与历史 image_url 剥离逻辑），wasm 模式会假通过；
	// wasm 模式需在重新 make build 后再启用验证
	test.RunGoTest(t, func(t *testing.T) {
		// 测试包含单张图片的请求体处理
		t.Run("single image request body", func(t *testing.T) {
			host, status := test.NewTestHost(basicDashScopeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			// 构造包含单张图片的请求体
			requestBody := `{
				"model": "deepseek-ai/DeepSeek-V4-Flash",
				"messages": [
					{
						"role": "user",
						"content": [
							{
								"type": "text",
								"text": "这张图片里有什么？"
							},
							{
								"type": "image_url",
								"image_url": {
									"url": "https://example.com/image1.jpg"
								}
							}
						]
					}
				]
			}`

			// 调用请求体处理
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			// 应该返回ActionPause，因为需要等待OCR响应
			require.Equal(t, types.ActionPause, action)

			// 模拟OCR服务响应
			ocrResponse := `{
				"choices": [
					{
						"message": {
							"content": "图片中包含一些文字内容"
						}
					}
				]
			}`

			// 模拟HTTP调用响应
			host.CallOnHttpCall([][2]string{
				{"content-type", "application/json"},
				{":status", "200"},
			}, []byte(ocrResponse))

			modifiedBody := host.GetRequestBody()
			require.NotNil(t, modifiedBody)
			require.Contains(t, string(modifiedBody), "图片中包含一些文字内容")

			// 完成HTTP请求
			host.CompleteHttp()
		})

		// 测试包含多张图片的请求体处理
		t.Run("multiple images request body", func(t *testing.T) {
			host, status := test.NewTestHost(basicDashScopeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			// 构造包含多张图片的请求体
			requestBody := `{
				"model": "deepseek-ai/DeepSeek-V4-Flash",
				"messages": [
					{
						"role": "user",
						"content": [
							{
								"type": "text",
								"text": "这些图片里有什么？"
							},
							{
								"type": "image_url",
								"image_url": {
									"url": "https://example.com/image1.jpg"
								}
							},
							{
								"type": "image_url",
								"image_url": {
									"url": "https://example.com/image2.jpg"
								}
							}
						]
					}
				]
			}`

			// 调用请求体处理
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			// 应该返回ActionPause，因为需要等待OCR响应
			require.Equal(t, types.ActionPause, action)

			// 模拟第一张图片的OCR响应
			ocrResponse1 := `{
				"choices": [
					{
						"message": {
							"content": "第一张图片包含文字A"
						}
					}
				]
			}`

			// 模拟第二张图片的OCR响应
			ocrResponse2 := `{
				"choices": [
					{
						"message": {
							"content": "第二张图片包含文字B"
						}
					}
				]
			}`

			// 模拟第一个HTTP调用响应
			host.CallOnHttpCall([][2]string{
				{"content-type", "application/json"},
				{":status", "200"},
			}, []byte(ocrResponse1))

			// 模拟第二个HTTP调用响应
			host.CallOnHttpCall([][2]string{
				{"content-type", "application/json"},
				{":status", "200"},
			}, []byte(ocrResponse2))

			modifiedBody := host.GetRequestBody()
			require.NotNil(t, modifiedBody)
			require.Contains(t, string(modifiedBody), "第一张图片包含文字A")
			require.Contains(t, string(modifiedBody), "第二张图片包含文字B")

			// 完成HTTP请求
			host.CompleteHttp()
		})

		// 测试不包含图片的请求体处理
		t.Run("no image request body", func(t *testing.T) {
			host, status := test.NewTestHost(basicDashScopeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			// 构造不包含图片的请求体
			requestBody := `{
				"model": "deepseek-ai/DeepSeek-V4-Flash",
				"messages": [
					{
						"role": "user",
						"content": [
							{
								"type": "text",
								"text": "你好，请介绍一下自己"
							}
						]
					}
				]
			}`

			// 调用请求体处理
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			// 应该返回ActionContinue，因为没有图片需要处理
			require.Equal(t, types.ActionContinue, action)
		})

		// 测试一次多图 + 部分失败：结果按索引写入，不依赖回调顺序
		t.Run("multiple images with partial OCR failure", func(t *testing.T) {
			host, status := test.NewTestHost(basicDashScopeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			requestBody := `{
				"model": "deepseek-ai/DeepSeek-V4-Flash",
				"messages": [
					{
						"role": "user",
						"content": [
							{
								"type": "text",
								"text": "帮我看看这些图"
							},
							{
								"type": "image_url",
								"image_url": {
									"url": "https://example.com/image1.jpg"
								}
							},
							{
								"type": "image_url",
								"image_url": {
									"url": "https://example.com/image2.jpg"
								}
							},
							{
								"type": "image_url",
								"image_url": {
									"url": "https://example.com/image3.jpg"
								}
							}
						]
					}
				]
			}`

			action := host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionPause, action)

			// image1: 成功
			ocrResponse1 := `{"choices":[{"message":{"content":"图片1是代码"}}]}`
			// image2: 失败（503）
			// image3: 成功
			ocrResponse3 := `{"choices":[{"message":{"content":"图片3是图表"}}]}`

			// 模拟回调顺序不确定（先成功、再失败、最后成功）
			host.CallOnHttpCall([][2]string{
				{"content-type", "application/json"},
				{":status", "200"},
			}, []byte(ocrResponse1))
			host.CallOnHttpCall([][2]string{
				{"content-type", "application/json"},
				{":status", "503"},
			}, []byte(`{"error":"OCR service down"}`))
			host.CallOnHttpCall([][2]string{
				{"content-type", "application/json"},
				{":status", "200"},
			}, []byte(ocrResponse3))

			modifiedBody := host.GetRequestBody()
			require.NotNil(t, modifiedBody)
			// 成功的图片内容有正序标识
			require.Contains(t, string(modifiedBody), "第1张图片内容为 图片1是代码")
			require.Contains(t, string(modifiedBody), "第3张图片内容为 图片3是图表")
			// 失败的图片有占位标识
			require.Contains(t, string(modifiedBody), "第2张图片识别失败")
			// 原始提问文本保留
			require.Contains(t, string(modifiedBody), "帮我看看这些图")
			// 无 image_url
			require.NotContains(t, string(modifiedBody), "image_url")

			host.CompleteHttp()
		})
	})
}

// 测试多轮对话场景：历史消息残留 image_url 的剥离
// 场景来源：客户端本地保存的是原始多模态消息，后续每轮请求都会携带历史 image_url，
// 而非多模态后端（DeepSeek-V4-Flash）会因历史 image_url 报"不是多模态大模型"错误
func TestMultiTurnImageHandling(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// 场景一：第一轮发图，第二轮发纯文本
		// 最后一条 user 消息没有图片，但历史消息里残留 image_url → 应直接剥离放行（无需 OCR）
		t.Run("text-only turn with history image", func(t *testing.T) {
			host, status := test.NewTestHost(basicDashScopeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			// 第一轮：user 发图（历史残留）；第二轮：user 发纯文本
			requestBody := `{
				"model": "deepseek-ai/DeepSeek-V4-Flash",
				"messages": [
					{
						"role": "user",
						"content": [
							{
								"type": "text",
								"text": "这张图片里有什么？"
							},
							{
								"type": "image_url",
								"image_url": {
									"url": "https://example.com/old-image.jpg"
								}
							}
						]
					},
					{
						"role": "assistant",
						"content": "图片里是一段文字介绍。"
					},
					{
						"role": "user",
						"content": "帮我总结一下"
					}
				]
			}`

			action := host.CallOnHttpRequestBody([]byte(requestBody))

			// 纯文本轮次：无需 OCR，直接放行
			require.Equal(t, types.ActionContinue, action)

			// 但历史 image_url 必须已被剥离
			modifiedBody := host.GetRequestBody()
			require.NotNil(t, modifiedBody)
			require.NotContains(t, string(modifiedBody), "image_url")
			// 历史消息的文字部分保留
			require.Contains(t, string(modifiedBody), "这张图片里有什么？")
			// 当前轮次原文保留
			require.Contains(t, string(modifiedBody), "帮我总结一下")
			// assistant 回复保留
			require.Contains(t, string(modifiedBody), "图片里是一段文字介绍。")

			host.CompleteHttp()
		})

		// 场景二：历史消息是纯图片（无文字），剥离后不能留下空 content 数组
		t.Run("image-only history message gets placeholder", func(t *testing.T) {
			host, status := test.NewTestHost(basicDashScopeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			requestBody := `{
				"model": "deepseek-ai/DeepSeek-V4-Flash",
				"messages": [
					{
						"role": "user",
						"content": [
							{
								"type": "image_url",
								"image_url": {
									"url": "https://example.com/only-image.jpg"
								}
							}
						]
					},
					{
						"role": "assistant",
						"content": "这是一张截图。"
					},
					{
						"role": "user",
						"content": "继续"
					}
				]
			}`

			action := host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			modifiedBody := host.GetRequestBody()
			require.NotNil(t, modifiedBody)
			require.NotContains(t, string(modifiedBody), "image_url")
			// 纯图片消息被替换为占位文本，而不是空数组
			require.Contains(t, string(modifiedBody), "[图片]")
			// 不能出现空 content 数组
			require.NotContains(t, string(modifiedBody), `"content": []`)

			host.CompleteHttp()
		})

		// 场景三：第一轮发图，第二轮又发新图
		// 当前轮触发 OCR，历史 image_url 也必须一并清除
		t.Run("new image turn with history image", func(t *testing.T) {
			host, status := test.NewTestHost(basicDashScopeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			requestBody := `{
				"model": "deepseek-ai/DeepSeek-V4-Flash",
				"messages": [
					{
						"role": "user",
						"content": [
							{
								"type": "text",
								"text": "第一张图有什么？"
							},
							{
								"type": "image_url",
								"image_url": {
									"url": "https://example.com/old-image.jpg"
								}
							}
						]
					},
					{
						"role": "assistant",
						"content": "第一张图是一个表格。"
					},
					{
						"role": "user",
						"content": [
							{
								"type": "text",
								"text": "第二张图呢？"
							},
							{
								"type": "image_url",
								"image_url": {
									"url": "https://example.com/new-image.jpg"
								}
							}
						]
					}
				]
			}`

			action := host.CallOnHttpRequestBody([]byte(requestBody))

			// 当前轮有图，需要等待 OCR
			require.Equal(t, types.ActionPause, action)

			// 模拟 OCR 响应（当前轮的 new-image.jpg）
			ocrResponse := `{
				"choices": [
					{
						"message": {
							"content": "第二张图是一段代码"
						}
					}
				]
			}`

			host.CallOnHttpCall([][2]string{
				{"content-type", "application/json"},
				{":status", "200"},
			}, []byte(ocrResponse))

			// 当前轮 content 已被替换为 OCR 注入的纯文本 prompt
			modifiedBody := host.GetRequestBody()
			require.NotNil(t, modifiedBody)
			require.Contains(t, string(modifiedBody), "第二张图是一段代码")
			// 历史消息的文字保留
			require.Contains(t, string(modifiedBody), "第一张图有什么？")
			// 整个请求中不允许再出现任何 image_url（含历史）
			require.NotContains(t, string(modifiedBody), "image_url")

			host.CompleteHttp()
		})

		// 场景四：非白名单模型（多模态模型）带历史 image_url → 完全放行，不做任何修改
		t.Run("non-whitelisted model passes through", func(t *testing.T) {
			host, status := test.NewTestHost(basicDashScopeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			requestBody := `{
				"model": "Qwen/Qwen3-VL-8B-Instruct",
				"messages": [
					{
						"role": "user",
						"content": [
							{
								"type": "text",
								"text": "这张图片里有什么？"
							},
							{
								"type": "image_url",
								"image_url": {
									"url": "https://example.com/image.jpg"
								}
							}
						]
					}
				]
			}`

			action := host.CallOnHttpRequestBody([]byte(requestBody))

			// 多模态模型不匹配白名单：不触发 OCR，也不剥离 image_url
			require.Equal(t, types.ActionContinue, action)

			host.CompleteHttp()
		})

		// 场景五：连续多轮各发一张图（历史逐轮累积，stripAllImageUrls 必须全清）
		// 模拟客户端发了 3 轮图后，第 4 轮纯文字追问
		t.Run("multi-turn each sending one image then text query", func(t *testing.T) {
			host, status := test.NewTestHost(basicDashScopeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			// 第1轮：user[A图] → assistant[回答A]
			// 第2轮：user[B图] → assistant[回答B]
			// 第3轮：user[C图] → assistant[回答C]
			// 第4轮：user 纯文字追问
			// 历史中 image_url 累积在第1/2/3条 user 消息
			requestBody := `{
				"model": "deepseek-ai/DeepSeek-V4-Flash",
				"messages": [
					{
						"role": "user",
						"content": [
							{"type": "image_url", "image_url": {"url": "https://example.com/A.jpg"}},
							{"type": "text", "text": "图A内容？"}
						]
					},
					{"role": "assistant", "content": "图A是一个表格"},
					{
						"role": "user",
						"content": [
							{"type": "image_url", "image_url": {"url": "https://example.com/B.jpg"}},
							{"type": "text", "text": "图B内容？"}
						]
					},
					{"role": "assistant", "content": "图B是一张截图"},
					{
						"role": "user",
						"content": [
							{"type": "image_url", "image_url": {"url": "https://example.com/C.jpg"}},
							{"type": "text", "text": "图C内容？"}
						]
					},
					{"role": "assistant", "content": "图C是一段代码"},
					{
						"role": "user",
						"content": "三个图有什么关联？"
					}
				]
			}`

			action := host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			modifiedBody := host.GetRequestBody()
			require.NotNil(t, modifiedBody)
			// 所有历史 image_url 必须被剥离
			require.NotContains(t, string(modifiedBody), "image_url")
			require.NotContains(t, string(modifiedBody), "A.jpg")
			require.NotContains(t, string(modifiedBody), "B.jpg")
			require.NotContains(t, string(modifiedBody), "C.jpg")
			// 历史文字部分保留
			require.Contains(t, string(modifiedBody), "图A内容？")
			require.Contains(t, string(modifiedBody), "图B内容？")
			require.Contains(t, string(modifiedBody), "图C内容？")
			// assistant 历史保留
			require.Contains(t, string(modifiedBody), "图A是一个表格")
			require.Contains(t, string(modifiedBody), "图B是一张截图")
			require.Contains(t, string(modifiedBody), "图C是一段代码")
			// 当前轮提问保留
			require.Contains(t, string(modifiedBody), "三个图有什么关联？")

			host.CompleteHttp()
		})
	})
}

// 测试配置验证
func TestConfigValidation(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// 测试缺少type配置
		t.Run("missing type", func(t *testing.T) {
			invalidConfig := func() json.RawMessage {
				data, _ := json.Marshal(map[string]interface{}{
					"apiKey":      "test-api-key",
					"serviceName": "ocr-service",
					"serviceHost": "dashscope.aliyuncs.com",
					"servicePort": 443,
				})
				return data
			}()

			host, status := test.NewTestHost(invalidConfig)
			defer host.Reset()
			// 应该返回错误状态，因为缺少必需的type
			require.NotEqual(t, types.OnPluginStartStatusOK, status)
		})

		// 测试缺少apiKey配置
		t.Run("missing apiKey", func(t *testing.T) {
			invalidConfig := func() json.RawMessage {
				data, _ := json.Marshal(map[string]interface{}{
					"type":        "dashscope",
					"serviceName": "ocr-service",
					"serviceHost": "dashscope.aliyuncs.com",
					"servicePort": 443,
					"timeout":     10000,
					"model":       "qwen-vl-ocr",
				})
				return data
			}()

			host, status := test.NewTestHost(invalidConfig)
			defer host.Reset()
			// 应该返回错误状态，因为缺少必需的apiKey
			require.NotEqual(t, types.OnPluginStartStatusOK, status)
		})

		// 测试缺少serviceName配置
		t.Run("missing serviceName", func(t *testing.T) {
			invalidConfig := func() json.RawMessage {
				data, _ := json.Marshal(map[string]interface{}{
					"type":        "dashscope",
					"apiKey":      "test-api-key",
					"serviceHost": "dashscope.aliyuncs.com",
					"servicePort": 443,
					"timeout":     10000,
					"model":       "qwen-vl-ocr",
				})
				return data
			}()

			host, status := test.NewTestHost(invalidConfig)
			defer host.Reset()
			// 应该返回错误状态，因为缺少必需的serviceName
			require.NotEqual(t, types.OnPluginStartStatusOK, status)
		})

		// 测试未知的provider类型
		t.Run("unknown provider type", func(t *testing.T) {
			invalidConfig := func() json.RawMessage {
				data, _ := json.Marshal(map[string]interface{}{
					"type":        "unknown-provider",
					"apiKey":      "test-api-key",
					"serviceName": "ocr-service",
					"serviceHost": "example.com",
					"servicePort": 443,
				})
				return data
			}()

			host, status := test.NewTestHost(invalidConfig)
			defer host.Reset()
			// 应该返回错误状态，因为provider类型未知
			require.NotEqual(t, types.OnPluginStartStatusOK, status)
		})
	})
}

// 测试边界情况
func TestEdgeCases(t *testing.T) {
	// 注意：同 TestOnHttpRequestBody，plugin.wasm 为旧构建产物，仅跑 go 模式
	test.RunGoTest(t, func(t *testing.T) {
		// 测试空请求体
		t.Run("empty request body", func(t *testing.T) {
			host, status := test.NewTestHost(basicDashScopeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			// 调用请求体处理 - 空请求体
			action := host.CallOnHttpRequestBody([]byte{})

			// 应该返回ActionContinue，因为没有图片需要处理
			require.Equal(t, types.ActionContinue, action)
		})

		// 测试无效JSON请求体
		t.Run("invalid JSON request body", func(t *testing.T) {
			host, status := test.NewTestHost(basicDashScopeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			// 调用请求体处理 - 无效JSON
			invalidJSON := []byte(`{"messages": [{"role": "user", "content": "test"}`)
			action := host.CallOnHttpRequestBody(invalidJSON)

			// 应该返回ActionContinue，因为JSON解析失败
			require.Equal(t, types.ActionContinue, action)
		})

		// 测试OCR服务错误响应
		t.Run("OCR service error response", func(t *testing.T) {
			host, status := test.NewTestHost(basicDashScopeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			// 构造包含图片的请求体
			requestBody := `{
				"model": "deepseek-ai/DeepSeek-V4-Flash",
				"messages": [
					{
						"role": "user",
						"content": [
							{
								"type": "text",
								"text": "这张图片里有什么？"
							},
							{
								"type": "image_url",
								"image_url": {
									"url": "https://example.com/image1.jpg"
								}
							}
						]
					}
				]
			}`

			// 调用请求体处理
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			// 应该返回ActionPause
			require.Equal(t, types.ActionPause, action)

			// 模拟OCR服务错误响应
			errorResponse := `{
				"error": "Service unavailable",
				"message": "OCR service is down"
			}`

			host.CallOnHttpCall([][2]string{
				{"content-type", "application/json"},
				{":status", "503"},
			}, []byte(errorResponse))

			host.CompleteHttp()
		})

		// 测试OCR服务返回空结果
		t.Run("OCR service empty response", func(t *testing.T) {
			host, status := test.NewTestHost(basicDashScopeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/api/chat"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			// 构造包含图片的请求体
			requestBody := `{
				"model": "deepseek-ai/DeepSeek-V4-Flash",
				"messages": [
					{
						"role": "user",
						"content": [
							{
								"type": "text",
								"text": "这张图片里有什么？"
							},
							{
								"type": "image_url",
								"image_url": {
									"url": "https://example.com/image1.jpg"
								}
							}
						]
					}
				]
			}`

			// 调用请求体处理
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			// 应该返回ActionPause
			require.Equal(t, types.ActionPause, action)

			// 模拟OCR服务返回空结果
			emptyResponse := `{
				"choices": []
			}`

			host.CallOnHttpCall([][2]string{
				{"content-type", "application/json"},
				{":status", "200"},
			}, []byte(emptyResponse))

			host.CompleteHttp()
		})
	})
}
