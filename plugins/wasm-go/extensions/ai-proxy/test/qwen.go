package test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

// 测试配置：基本qwen配置
var basicQwenConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":      "qwen",
			"apiTokens": []string{"sk-qwen-test123456789"},
			"modelMapping": map[string]string{
				"*": "qwen-turbo",
			},
			"qwenEnableCompatible": false,
		},
	})
	return data
}()

// 测试配置：qwen多模型配置
var qwenMultiModelConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":      "qwen",
			"apiTokens": []string{"sk-qwen-multi-model"},
			"modelMapping": map[string]string{
				"gpt-3.5-turbo":          "qwen-turbo",
				"gpt-4":                  "qwen-plus",
				"text-embedding-ada-002": "text-embedding-v1",
				"qwen-long":              "qwen-long",
				"qwen-vl-plus":           "qwen-vl-plus",
			},
		},
	})
	return data
}()

// 测试配置：无效qwen配置（缺少apiToken）
var invalidQwenConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type": "qwen",
			// 缺少apiTokens
		},
	})
	return data
}()

// 测试配置：qwen自定义域名配置
var qwenCustomDomainConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":      "qwen",
			"apiTokens": []string{"sk-qwen-custom-domain"},
			"modelMapping": map[string]string{
				"*": "qwen-turbo",
			},
			"qwenDomain":           "custom.qwen.com",
			"qwenEnableCompatible": false,
		},
	})
	return data
}()

// 测试配置：qwen启用搜索功能配置
var qwenEnableSearchConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":      "qwen",
			"apiTokens": []string{"sk-qwen-search"},
			"modelMapping": map[string]string{
				"*": "qwen-turbo",
			},
			"qwenEnableSearch": true,
		},
	})
	return data
}()

// 测试配置：qwen启用兼容模式配置
var qwenEnableCompatibleConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":      "qwen",
			"apiTokens": []string{"sk-qwen-compatible"},
			"modelMapping": map[string]string{
				"*": "qwen-turbo",
			},
			"qwenEnableCompatible": true,
		},
	})
	return data
}()

// 测试配置：qwen original + 兼容模式（用于覆盖 provider.GetApiName 分支）
var qwenOriginalCompatibleConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":      "qwen",
			"apiTokens": []string{"sk-qwen-original-compatible"},
			"modelMapping": map[string]string{
				"*": "qwen-turbo",
			},
			"qwenEnableCompatible": true,
			"protocol":             "original",
		},
	})
	return data
}()

// 测试配置：qwen文件ID配置
var qwenFileIdsConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":      "qwen",
			"apiTokens": []string{"sk-qwen-files"},
			"modelMapping": map[string]string{
				"*": "qwen-long",
			},
			"qwenFileIds": []string{"file-123", "file-456"},
		},
	})
	return data
}()

// 测试配置：qwen完整配置（包含所有特殊字段）
var completeQwenConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":      "qwen",
			"apiTokens": []string{"sk-qwen-complete"},
			"modelMapping": map[string]string{
				"*": "qwen-turbo",
			},
			"qwenDomain":           "custom.qwen.com",
			"qwenEnableSearch":     true,
			"qwenEnableCompatible": false,
			"reasoningContentMode": "passthrough",
			"failover": map[string]interface{}{
				"enabled": false,
			},
			"retryOnFailure": map[string]interface{}{
				"enabled": false,
			},
		},
	})
	return data
}()

// 测试配置：qwen配置冲突（同时配置qwenFileIds和context）
var qwenConflictConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":      "qwen",
			"apiTokens": []string{"sk-qwen-conflict"},
			"modelMapping": map[string]string{
				"*": "qwen-turbo",
			},
			"qwenFileIds": []string{"file-123"},
			"context": map[string]interface{}{
				"fileUrl":     "http://example.com/context.txt",
				"serviceName": "context-service",
				"servicePort": 8080,
			},
		},
	})
	return data
}()

func hasUnsupportedAPINameError(errorLogs []string) bool {
	for _, log := range errorLogs {
		if strings.Contains(log, "unsupported API name") {
			return true
		}
	}
	return false
}

func RunQwenParseConfigTests(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// 测试基本qwen配置解析
		t.Run("basic qwen config", func(t *testing.T) {
			host, status := test.NewTestHost(basicQwenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试qwen多模型配置解析
		t.Run("qwen multi model config", func(t *testing.T) {
			host, status := test.NewTestHost(qwenMultiModelConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试无效qwen配置（缺少apiToken）
		t.Run("invalid qwen config - missing api token", func(t *testing.T) {
			host, status := test.NewTestHost(invalidQwenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})

		// 测试qwen自定义域名配置解析
		t.Run("qwen custom domain config", func(t *testing.T) {
			host, status := test.NewTestHost(qwenCustomDomainConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试qwen启用搜索功能配置解析
		t.Run("qwen enable search config", func(t *testing.T) {
			host, status := test.NewTestHost(qwenEnableSearchConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试qwen启用兼容模式配置解析
		t.Run("qwen enable compatible config", func(t *testing.T) {
			host, status := test.NewTestHost(qwenEnableCompatibleConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试qwen文件ID配置解析
		t.Run("qwen file ids config", func(t *testing.T) {
			host, status := test.NewTestHost(qwenFileIdsConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试qwen完整配置解析
		t.Run("qwen complete config", func(t *testing.T) {
			host, status := test.NewTestHost(completeQwenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试qwen配置冲突（同时配置qwenFileIds和context）
		t.Run("qwen conflict config - qwenFileIds and context", func(t *testing.T) {
			host, status := test.NewTestHost(qwenConflictConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})
	})
}

func RunQwenOnHttpRequestHeadersTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试qwen请求头处理（聊天完成接口）
		t.Run("qwen chat completion request headers", func(t *testing.T) {
			host, status := test.NewTestHost(basicQwenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			// 应该返回HeaderStopIteration，因为需要处理请求体
			require.Equal(t, types.HeaderStopIteration, action)

			// 验证请求头是否被正确处理
			requestHeaders := host.GetRequestHeaders()
			require.NotNil(t, requestHeaders)

			// 验证Host是否被改为qwen默认域名
			hostValue, hasHost := test.GetHeaderValue(requestHeaders, ":authority")
			require.True(t, hasHost, "Host header should exist")
			require.Equal(t, "dashscope.aliyuncs.com", hostValue, "Host should be changed to qwen default domain")

			// 验证Authorization是否被设置
			authValue, hasAuth := test.GetHeaderValue(requestHeaders, "Authorization")
			require.True(t, hasAuth, "Authorization header should exist")
			require.Contains(t, authValue, "sk-qwen-test123456789", "Authorization should contain qwen API token")

			// 验证Path是否被正确转换为qwen API路径
			pathValue, hasPath := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, hasPath, "Path header should exist")
			// qwen会将OpenAI路径转换为自己的API路径
			require.Contains(t, pathValue, "/api/v1/services/aigc/text-generation/generation", "Path should be converted to qwen API path")

			// 检查是否有相关的处理日志
			debugLogs := host.GetDebugLogs()
			hasQwenLogs := false
			for _, log := range debugLogs {
				if strings.Contains(log, "qwen") {
					hasQwenLogs = true
					break
				}
			}
			require.True(t, hasQwenLogs, "Should have qwen processing logs")
		})

		// 测试qwen请求头处理（嵌入接口）
		t.Run("qwen embeddings request headers", func(t *testing.T) {
			host, status := test.NewTestHost(basicQwenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/embeddings"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			require.Equal(t, types.HeaderStopIteration, action)

			// 验证嵌入接口的请求头处理
			requestHeaders := host.GetRequestHeaders()
			require.NotNil(t, requestHeaders)

			// 验证Host转换
			hostValue, hasHost := test.GetHeaderValue(requestHeaders, ":authority")
			require.True(t, hasHost)
			require.Equal(t, "dashscope.aliyuncs.com", hostValue)

			// 验证Path转换（qwen会将OpenAI路径转换为自己的API路径）
			pathValue, hasPath := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, hasPath)
			require.Contains(t, pathValue, "/api/v1/services/embeddings/text-embedding/text-embedding", "Path should be converted to qwen embeddings API path")

			// 验证Authorization设置
			authValue, hasAuth := test.GetHeaderValue(requestHeaders, "Authorization")
			require.True(t, hasAuth, "Authorization header should exist for embeddings")
			require.Contains(t, authValue, "sk-qwen-test123456789", "Authorization should contain qwen API token")
		})

		// 测试qwen请求头处理（reranks接口）
		t.Run("qwen reranks request headers", func(t *testing.T) {
			host, status := test.NewTestHost(basicQwenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/reranks"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			require.Equal(t, types.HeaderStopIteration, action)

			requestHeaders := host.GetRequestHeaders()
			require.NotNil(t, requestHeaders)

			pathValue, hasPath := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, hasPath)
			require.Contains(t, pathValue, "/api/v1/services/rerank/text-rerank/text-rerank", "Path should be converted to qwen rerank path")
		})

		// 测试qwen自定义域名请求头处理
		t.Run("qwen custom domain request headers", func(t *testing.T) {
			host, status := test.NewTestHost(qwenCustomDomainConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			require.Equal(t, types.HeaderStopIteration, action)

			// 验证自定义域名的请求头处理
			requestHeaders := host.GetRequestHeaders()
			require.NotNil(t, requestHeaders)

			// 验证Host是否被改为自定义域名
			hostValue, hasHost := test.GetHeaderValue(requestHeaders, ":authority")
			require.True(t, hasHost)
			require.Equal(t, "custom.qwen.com", hostValue, "Host should be changed to custom domain")

			// 验证Path是否被正确转换为qwen API路径
			pathValue, hasPath := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, hasPath)
			// 即使使用自定义域名，路径仍然会被转换为qwen API路径
			require.Contains(t, pathValue, "/api/v1/services/aigc/text-generation/generation", "Path should be converted to qwen API path")
		})

		// 测试qwen兼容模式请求头处理
		t.Run("qwen compatible mode request headers", func(t *testing.T) {
			host, status := test.NewTestHost(qwenEnableCompatibleConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			require.Equal(t, types.HeaderStopIteration, action)

			// 验证兼容模式的请求头处理
			requestHeaders := host.GetRequestHeaders()
			require.NotNil(t, requestHeaders)

			// 验证Host转换
			hostValue, hasHost := test.GetHeaderValue(requestHeaders, ":authority")
			require.True(t, hasHost)
			require.Equal(t, "dashscope.aliyuncs.com", hostValue)

			// 验证Path转换（兼容模式应该使用兼容路径）
			pathValue, hasPath := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, hasPath)
			require.Contains(t, pathValue, "/compatible-mode/v1/chat/completions", "Path should use compatible mode path")
		})

		// 测试qwen兼容模式请求头处理（responses接口）
		t.Run("qwen compatible mode responses request headers", func(t *testing.T) {
			host, status := test.NewTestHost(qwenEnableCompatibleConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/responses"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			require.Equal(t, types.HeaderStopIteration, action)

			requestHeaders := host.GetRequestHeaders()
			require.NotNil(t, requestHeaders)

			pathValue, hasPath := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, hasPath)
			require.Contains(t, pathValue, "/compatible-mode/v1/responses", "Path should use compatible mode responses path")
		})

		// 测试qwen兼容模式请求头处理（reranks接口）
		t.Run("qwen compatible mode reranks request headers", func(t *testing.T) {
			host, status := test.NewTestHost(qwenEnableCompatibleConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/reranks"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			require.Equal(t, types.HeaderStopIteration, action)

			requestHeaders := host.GetRequestHeaders()
			require.NotNil(t, requestHeaders)

			pathValue, hasPath := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, hasPath)
			require.Contains(t, pathValue, "/compatible-api/v1/reranks", "Path should use compatible API reranks path")
		})

		// 测试qwen兼容模式请求头处理（conversations接口）
		t.Run("qwen compatible mode conversations request headers", func(t *testing.T) {
			host, status := test.NewTestHost(qwenEnableCompatibleConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/conversations"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			require.Equal(t, types.HeaderStopIteration, action)

			requestHeaders := host.GetRequestHeaders()
			require.NotNil(t, requestHeaders)

			pathValue, hasPath := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, hasPath)
			require.Contains(t, pathValue, "/compatible-mode/v1/conversations", "Path should use compatible mode conversations path")
		})
	})
}

func RunQwenOnHttpRequestBodyTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试qwen请求体处理（聊天完成接口）
		t.Run("qwen chat completion request body", func(t *testing.T) {
			host, status := test.NewTestHost(basicQwenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			// 设置请求体
			requestBody := `{"model":"qwen-turbo","messages":[{"role":"user","content":"test"}]}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			require.Equal(t, types.ActionContinue, action)

			// 验证请求体是否被正确处理
			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)

			// 验证模型名称是否被正确映射
			require.Contains(t, string(processedBody), "qwen-turbo", "Original model name should be preserved or mapped")

			// 检查是否有相关的处理日志
			debugLogs := host.GetDebugLogs()
			infoLogs := host.GetInfoLogs()

			// 验证是否有qwen相关的处理日志
			hasQwenLogs := false
			for _, log := range debugLogs {
				if strings.Contains(log, "qwen") {
					hasQwenLogs = true
					break
				}
			}
			for _, log := range infoLogs {
				if strings.Contains(log, "qwen") {
					hasQwenLogs = true
					break
				}
			}
			require.True(t, hasQwenLogs, "Should have qwen processing logs")
		})

		// 测试qwen请求体处理（嵌入接口）
		t.Run("qwen embeddings request body", func(t *testing.T) {
			host, status := test.NewTestHost(basicQwenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/embeddings"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			// 设置请求体
			requestBody := `{"model":"text-embedding-v1","input":"test text"}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			require.Equal(t, types.ActionContinue, action)

			// 验证嵌入接口的请求体处理
			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)

			// 验证模型名称映射
			// 由于使用了通配符映射 "*": "qwen-turbo"，text-embedding-v1 会被映射为 qwen-turbo
			require.Contains(t, string(processedBody), "qwen-turbo", "Model name should be mapped via wildcard")

			// 检查处理日志
			debugLogs := host.GetDebugLogs()
			hasEmbeddingLogs := false
			for _, log := range debugLogs {
				if strings.Contains(log, "embeddings") || strings.Contains(log, "qwen") {
					hasEmbeddingLogs = true
					break
				}
			}
			require.True(t, hasEmbeddingLogs, "Should have embedding processing logs")
		})

		// 测试qwen请求体处理（qwen-long模型，带文件ID）
		t.Run("qwen qwen-long model with file ids request body", func(t *testing.T) {
			host, status := test.NewTestHost(qwenFileIdsConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			// 设置请求体
			requestBody := `{"model":"qwen-long","messages":[{"role":"user","content":"test"}]}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			require.Equal(t, types.ActionContinue, action)

			// 验证qwen-long模型的请求体处理
			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)

			// 验证模型名称映射
			require.Contains(t, string(processedBody), "qwen-long", "qwen-long model name should be preserved")

			// 检查处理日志
			debugLogs := host.GetDebugLogs()
			hasFileLogs := false
			for _, log := range debugLogs {
				if strings.Contains(log, "file") || strings.Contains(log, "qwen") {
					hasFileLogs = true
					break
				}
			}
			require.True(t, hasFileLogs, "Should have file processing logs")
		})

		// 测试qwen请求体处理（reranks接口）
		t.Run("qwen reranks request body", func(t *testing.T) {
			host, status := test.NewTestHost(qwenMultiModelConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/reranks"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			requestBody := `{
				"model":"qwen3-rerank",
				"documents":["doc1","doc2"],
				"query":"test query",
				"top_n":1
			}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			require.Equal(t, types.ActionContinue, action)

			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)
			require.Contains(t, string(processedBody), "qwen3-rerank", "Reranks request model should be preserved")
			require.Contains(t, string(processedBody), "documents", "Reranks request documents should be preserved")
		})

		// 测试qwen请求体处理（qwen-vl模型，多模态）
		t.Run("qwen qwen-vl model multimodal request body", func(t *testing.T) {
			host, status := test.NewTestHost(qwenMultiModelConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			// 设置请求体
			requestBody := `{"model":"qwen-vl-plus","messages":[{"role":"user","content":"test"}]}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			require.Equal(t, types.ActionContinue, action)

			// 验证qwen-vl模型的请求体处理
			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)

			// 验证模型名称映射
			require.Contains(t, string(processedBody), "qwen-vl-plus", "qwen-vl model name should be preserved")

			// 检查处理日志
			debugLogs := host.GetDebugLogs()
			hasVlLogs := false
			for _, log := range debugLogs {
				if strings.Contains(log, "vl") || strings.Contains(log, "qwen") {
					hasVlLogs = true
					break
				}
			}
			require.True(t, hasVlLogs, "Should have qwen-vl processing logs")
		})

		// 测试qwen请求体处理（启用搜索功能）
		t.Run("qwen enable search request body", func(t *testing.T) {
			host, status := test.NewTestHost(qwenEnableSearchConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			// 设置请求体
			requestBody := `{"model":"qwen-turbo","messages":[{"role":"user","content":"test"}]}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			require.Equal(t, types.ActionContinue, action)

			// 验证启用搜索功能的请求体处理
			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)

			// 验证模型名称映射
			require.Contains(t, string(processedBody), "qwen-turbo", "Model name should be preserved")

			// 检查处理日志
			debugLogs := host.GetDebugLogs()
			hasSearchLogs := false
			for _, log := range debugLogs {
				if strings.Contains(log, "search") || strings.Contains(log, "qwen") {
					hasSearchLogs = true
					break
				}
			}
			require.True(t, hasSearchLogs, "Should have search processing logs")
		})

		// 测试qwen请求体处理（兼容模式）
		t.Run("qwen compatible mode request body", func(t *testing.T) {
			host, status := test.NewTestHost(qwenEnableCompatibleConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			// 设置请求体
			requestBody := `{"model":"qwen-turbo","messages":[{"role":"user","content":"test"}]}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			require.Equal(t, types.ActionContinue, action)

			// 验证兼容模式的请求体处理
			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)

			// 验证模型名称映射
			require.Contains(t, string(processedBody), "qwen-turbo", "Model name should be preserved")

			// 检查处理日志
			debugLogs := host.GetDebugLogs()
			hasCompatibleLogs := false
			for _, log := range debugLogs {
				if strings.Contains(log, "compatible") || strings.Contains(log, "qwen") {
					hasCompatibleLogs = true
					break
				}
			}
			require.True(t, hasCompatibleLogs, "Should have compatible mode processing logs")
		})

		// 测试qwen请求体处理（兼容模式 responses接口）
		t.Run("qwen compatible mode responses request body", func(t *testing.T) {
			host, status := test.NewTestHost(qwenEnableCompatibleConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/responses"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			requestBody := `{"model":"qwen-turbo","input":"test"}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			require.Equal(t, types.ActionContinue, action)

			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)
			require.Contains(t, string(processedBody), "qwen-turbo", "Model name should be preserved in responses request")
		})

		// 测试qwen请求体处理（兼容模式 reranks接口）
		t.Run("qwen compatible mode reranks request body", func(t *testing.T) {
			host, status := test.NewTestHost(qwenEnableCompatibleConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/reranks"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			requestBody := `{
				"model":"qwen3-rerank",
				"documents":["doc1","doc2"],
				"query":"test query",
				"top_n":1
			}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			require.Equal(t, types.ActionContinue, action)

			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)
			require.Contains(t, string(processedBody), "qwen-turbo", "Reranks request model should be mapped by wildcard")
			require.Contains(t, string(processedBody), "documents", "Reranks request documents should be preserved")
		})

		// 测试qwen请求体处理（兼容模式 conversations接口）
		t.Run("qwen compatible mode conversations request body", func(t *testing.T) {
			host, status := test.NewTestHost(qwenEnableCompatibleConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/conversations"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			requestBody := `{
				"metadata":{"topic":"demo"},
				"items":[{"type":"message","role":"system","content":"test content"}]
			}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			require.Equal(t, types.ActionContinue, action)

			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)
			require.Contains(t, string(processedBody), "\"metadata\"", "Conversations metadata should be preserved")
			require.Contains(t, string(processedBody), "\"items\"", "Conversations items should be preserved")
			require.NotContains(t, string(processedBody), "\"model\":", "Conversations request should not inject model field")
		})

		// 测试qwen请求体处理（非兼容模式 responses接口应报不支持）
		t.Run("qwen non-compatible mode responses request body unsupported", func(t *testing.T) {
			host, status := test.NewTestHost(basicQwenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/responses"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			requestHeaders := host.GetRequestHeaders()
			require.NotNil(t, requestHeaders)

			pathValue, hasPath := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, hasPath)
			require.Contains(t, pathValue, "/v1/responses", "Path should remain unchanged when responses is unsupported")

			requestBody := `{"model":"qwen-turbo","input":"test"}`
			bodyAction := host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, bodyAction)

			hasUnsupportedErr := hasUnsupportedAPINameError(host.GetErrorLogs())
			require.True(t, hasUnsupportedErr, "Should log unsupported API name for non-compatible responses")
		})

		// 覆盖 qwen.GetApiName 中以下分支：
		// - qwenCompatibleTextEmbeddingPath => ApiNameEmbeddings
		// - qwenCompatibleResponsesPath => ApiNameResponses
		// - qwenCompatibleTextRerankPath => ApiNameQwenV1Rerank
		// - qwenCompatibleConversationsPath => ApiNameQwenV1Conversations
		// - qwenAsyncAIGCPath => ApiNameQwenAsyncAIGC
		// - qwenAsyncTaskPath => ApiNameQwenAsyncTask
		t.Run("qwen original protocol get api name coverage for compatible paths and async paths", func(t *testing.T) {
			cases := []struct {
				name string
				path string
			}{
				{
					name: "compatible embeddings path",
					path: "/compatible-mode/v1/embeddings",
				},
				{
					name: "compatible responses path",
					path: "/compatible-mode/v1/responses",
				},
				{
					name: "compatible reranks path",
					path: "/compatible-api/v1/reranks",
				},
				{
					name: "compatible conversations path",
					path: "/compatible-mode/v1/conversations",
				},
				{
					name: "async aigc path",
					path: "/api/v1/services/aigc/custom-async-endpoint",
				},
				{
					name: "async task path",
					path: "/api/v1/tasks/task-123",
				},
			}

			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					host, status := test.NewTestHost(qwenOriginalCompatibleConfig)
					defer host.Reset()
					require.Equal(t, types.OnPluginStartStatusOK, status)

					action := host.CallOnHttpRequestHeaders([][2]string{
						{":authority", "example.com"},
						{":path", tc.path},
						{":method", "POST"},
						{"Content-Type", "application/json"},
					})
					// 测试框架中 action 可能表现为 Continue 或 HeaderStopIteration，
					// 这里关注的是后续 body 阶段不出现 unsupported API name。
					require.True(t, action == types.ActionContinue || action == types.HeaderStopIteration)

					requestBody := `{"model":"qwen-turbo","input":"test"}`
					bodyAction := host.CallOnHttpRequestBody([]byte(requestBody))
					require.Equal(t, types.ActionContinue, bodyAction)

					hasUnsupportedErr := hasUnsupportedAPINameError(host.GetErrorLogs())
					require.False(t, hasUnsupportedErr, "Path should be recognized by qwen.GetApiName in original protocol")
				})
			}
		})
	})
}

func RunQwenOnHttpResponseHeadersTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试qwen响应头处理（聊天完成接口）
		t.Run("qwen chat completion response headers", func(t *testing.T) {
			host, status := test.NewTestHost(basicQwenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			// 设置请求体
			requestBody := `{"model":"qwen-turbo","messages":[{"role":"user","content":"test"}]}`
			host.CallOnHttpRequestBody([]byte(requestBody))

			// 设置响应头
			responseHeaders := [][2]string{
				{":status", "200"},
				{"Content-Type", "application/json"},
				{"X-Request-Id", "req-123"},
			}
			action := host.CallOnHttpResponseHeaders(responseHeaders)

			require.Equal(t, types.ActionContinue, action)

			// 验证响应头是否被正确处理
			processedResponseHeaders := host.GetResponseHeaders()
			require.NotNil(t, processedResponseHeaders)

			// 验证状态码
			statusValue, hasStatus := test.GetHeaderValue(processedResponseHeaders, ":status")
			require.True(t, hasStatus, "Status header should exist")
			require.Equal(t, "200", statusValue, "Status should be 200")

			// 验证Content-Type
			contentTypeValue, hasContentType := test.GetHeaderValue(processedResponseHeaders, "Content-Type")
			require.True(t, hasContentType, "Content-Type header should exist")
			require.Equal(t, "application/json", contentTypeValue, "Content-Type should be application/json")

			// 检查是否有相关的处理日志
			debugLogs := host.GetDebugLogs()
			hasResponseLogs := false
			for _, log := range debugLogs {
				if strings.Contains(log, "response") || strings.Contains(log, "qwen") {
					hasResponseLogs = true
					break
				}
			}
			require.True(t, hasResponseLogs, "Should have response processing logs")
		})

		// 测试qwen响应头处理（嵌入接口）
		t.Run("qwen embeddings response headers", func(t *testing.T) {
			host, status := test.NewTestHost(basicQwenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/embeddings"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			// 设置请求体
			requestBody := `{"model":"text-embedding-v1","input":"test text"}`
			host.CallOnHttpRequestBody([]byte(requestBody))

			// 设置响应头
			responseHeaders := [][2]string{
				{":status", "200"},
				{"Content-Type", "application/json"},
				{"X-Embedding-Model", "text-embedding-v1"},
			}
			action := host.CallOnHttpResponseHeaders(responseHeaders)

			require.Equal(t, types.ActionContinue, action)

			// 验证响应头处理
			processedResponseHeaders := host.GetResponseHeaders()
			require.NotNil(t, processedResponseHeaders)

			// 验证嵌入模型信息
			modelValue, hasModel := test.GetHeaderValue(processedResponseHeaders, "X-Embedding-Model")
			require.True(t, hasModel, "Embedding model header should exist")
			require.Equal(t, "text-embedding-v1", modelValue, "Embedding model should match configuration")
		})

		// 测试qwen响应头处理（错误响应）
		t.Run("qwen error response headers", func(t *testing.T) {
			host, status := test.NewTestHost(basicQwenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			// 设置请求体
			requestBody := `{"model":"qwen-turbo","messages":[{"role":"user","content":"test"}]}`
			host.CallOnHttpRequestBody([]byte(requestBody))

			// 设置错误响应头
			errorResponseHeaders := [][2]string{
				{":status", "429"},
				{"Content-Type", "application/json"},
				{"Retry-After", "60"},
			}
			action := host.CallOnHttpResponseHeaders(errorResponseHeaders)

			require.Equal(t, types.ActionContinue, action)

			// 验证错误响应头处理
			processedResponseHeaders := host.GetResponseHeaders()
			require.NotNil(t, processedResponseHeaders)

			// 验证错误状态码
			statusValue, hasStatus := test.GetHeaderValue(processedResponseHeaders, ":status")
			require.True(t, hasStatus, "Status header should exist")
			require.Equal(t, "429", statusValue, "Status should be 429 (Too Many Requests)")

			// 验证重试信息
			retryValue, hasRetry := test.GetHeaderValue(processedResponseHeaders, "Retry-After")
			require.True(t, hasRetry, "Retry-After header should exist")
			require.Equal(t, "60", retryValue, "Retry-After should be 60 seconds")
		})
	})
}

func RunQwenOnHttpResponseBodyTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试qwen响应体处理（聊天完成接口）
		t.Run("qwen chat completion response body", func(t *testing.T) {
			host, status := test.NewTestHost(basicQwenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			// 设置请求体
			requestBody := `{"model":"qwen-turbo","messages":[{"role":"user","content":"test"}]}`
			host.CallOnHttpRequestBody([]byte(requestBody))

			// 设置响应头
			responseHeaders := [][2]string{
				{":status", "200"},
				{"Content-Type", "application/json"},
			}
			host.CallOnHttpResponseHeaders(responseHeaders)

			// 设置响应体
			responseBody := `{
				"request_id": "req-123",
				"output": {
					"choices": [{
						"message": {
							"role": "assistant",
							"content": "Hello! How can I help you today?"
						},
						"finish_reason": "stop"
					}]
				},
				"usage": {
					"input_tokens": 9,
					"output_tokens": 12,
					"total_tokens": 21
				}
			}`
			action := host.CallOnHttpResponseBody([]byte(responseBody))

			require.Equal(t, types.ActionContinue, action)

			// 验证响应体是否被正确处理
			processedResponseBody := host.GetResponseBody()
			require.NotNil(t, processedResponseBody)

			// 验证响应体内容（qwen格式）
			responseStr := string(processedResponseBody)
			require.Contains(t, responseStr, "request_id", "Response should contain request_id")
			require.Contains(t, responseStr, "output", "Response should contain output object")
			require.Contains(t, responseStr, "assistant", "Response should contain assistant role")
			require.Contains(t, responseStr, "usage", "Response should contain usage information")

			// 检查是否有相关的处理日志
			debugLogs := host.GetDebugLogs()
			hasResponseBodyLogs := false
			for _, log := range debugLogs {
				if strings.Contains(log, "response") || strings.Contains(log, "body") || strings.Contains(log, "qwen") {
					hasResponseBodyLogs = true
					break
				}
			}
			require.True(t, hasResponseBodyLogs, "Should have response body processing logs")
		})

		// 测试qwen响应体处理（嵌入接口）
		t.Run("qwen embeddings response body", func(t *testing.T) {
			host, status := test.NewTestHost(basicQwenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/embeddings"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			// 设置请求体
			requestBody := `{"model":"text-embedding-v1","input":"test text"}`
			host.CallOnHttpRequestBody([]byte(requestBody))

			// 设置响应头
			responseHeaders := [][2]string{
				{":status", "200"},
				{"Content-Type", "application/json"},
			}
			host.CallOnHttpResponseHeaders(responseHeaders)

			// 设置响应体
			responseBody := `{
				"output": {
					"embeddings": [{
						"embedding": [0.1, 0.2, 0.3, 0.4, 0.5],
						"text_index": 0
					}]
				},
				"usage": {
					"total_tokens": 5
				}
			}`
			action := host.CallOnHttpResponseBody([]byte(responseBody))

			require.Equal(t, types.ActionContinue, action)

			// 验证响应体处理
			processedResponseBody := host.GetResponseBody()
			require.NotNil(t, processedResponseBody)

			// 验证嵌入响应内容（qwen格式）
			responseStr := string(processedResponseBody)
			require.Contains(t, responseStr, "embedding", "Response should contain embedding object")
			require.Contains(t, responseStr, "0.1", "Response should contain embedding vector")
			require.Contains(t, responseStr, "output", "Response should contain output object")

			// 检查处理日志
			debugLogs := host.GetDebugLogs()
			hasEmbeddingLogs := false
			for _, log := range debugLogs {
				if strings.Contains(log, "embeddings") || strings.Contains(log, "qwen") {
					hasEmbeddingLogs = true
					break
				}
			}
			require.True(t, hasEmbeddingLogs, "Should have embedding processing logs")
		})

		// 测试qwen响应体处理（兼容模式）
		t.Run("qwen compatible mode response body", func(t *testing.T) {
			host, status := test.NewTestHost(qwenEnableCompatibleConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			// 设置请求体
			requestBody := `{"model":"qwen-turbo","messages":[{"role":"user","content":"test"}]}`
			host.CallOnHttpRequestBody([]byte(requestBody))

			// 设置响应头
			responseHeaders := [][2]string{
				{":status", "200"},
				{"Content-Type", "application/json"},
			}
			host.CallOnHttpResponseHeaders(responseHeaders)

			// 设置响应体（兼容模式应该直接返回）
			responseBody := `{
				"id": "chatcmpl-123",
				"object": "chat.completion",
				"created": 1677652288,
				"model": "qwen-turbo",
				"choices": [{
					"index": 0,
					"message": {
						"role": "assistant",
						"content": "Hello! How can I help you today?"
					},
					"finish_reason": "stop"
				}],
				"usage": {
					"prompt_tokens": 9,
					"completion_tokens": 12,
					"total_tokens": 21
				}
			}`
			action := host.CallOnHttpResponseBody([]byte(responseBody))

			require.Equal(t, types.ActionContinue, action)

			// 验证兼容模式的响应体处理
			processedResponseBody := host.GetResponseBody()
			require.NotNil(t, processedResponseBody)

			// 兼容模式应该直接返回原始响应
			responseStr := string(processedResponseBody)
			require.Contains(t, responseStr, "chat.completion", "Response should contain chat completion object")
			require.Contains(t, responseStr, "qwen-turbo", "Response should contain model name")
		})

		// 测试qwen响应体处理（兼容模式 responses 接口透传）
		t.Run("qwen compatible mode responses response body", func(t *testing.T) {
			host, status := test.NewTestHost(qwenEnableCompatibleConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/responses"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			requestBody := `{"model":"qwen-turbo","input":"test"}`
			host.CallOnHttpRequestBody([]byte(requestBody))

			responseHeaders := [][2]string{
				{":status", "200"},
				{"Content-Type", "application/json"},
			}
			host.CallOnHttpResponseHeaders(responseHeaders)

			responseBody := `{
				"id": "resp-123",
				"object": "response",
				"status": "completed",
				"output": [{
					"type": "message",
					"role": "assistant",
					"content": [{
						"type": "output_text",
						"text": "hello"
					}]
				}]
			}`
			action := host.CallOnHttpResponseBody([]byte(responseBody))
			require.Equal(t, types.ActionContinue, action)

			processedResponseBody := host.GetResponseBody()
			require.NotNil(t, processedResponseBody)
			responseStr := string(processedResponseBody)
			require.Contains(t, responseStr, "\"object\": \"response\"", "Responses API payload should be passthrough in compatible mode")
			require.Contains(t, responseStr, "\"text\": \"hello\"", "Assistant content should be preserved")
		})
	})
}

func RunQwenOnStreamingResponseBodyTests(t *testing.T) {
	// 测试qwen响应体处理（流式响应）
	test.RunTest(t, func(t *testing.T) {
		t.Run("qwen streaming response body", func(t *testing.T) {
			host, status := test.NewTestHost(basicQwenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			// 设置流式请求体
			requestBody := `{"model":"qwen-turbo","messages":[{"role":"user","content":"test"}],"stream":true}`
			host.CallOnHttpRequestBody([]byte(requestBody))

			// 设置流式响应头
			responseHeaders := [][2]string{
				{":status", "200"},
				{"Content-Type", "text/event-stream"},
			}
			host.CallOnHttpResponseHeaders(responseHeaders)

			// 模拟流式响应体
			chunk1 := `{"request_id":"req-123","output":{"choices":[{"message":{"role":"assistant","content":""},"finish_reason":""}]},"usage":{"input_tokens":9,"output_tokens":0,"total_tokens":9}}`
			chunk2 := `{"request_id":"req-123","output":{"choices":[{"message":{"role":"assistant","content":"Hello"},"finish_reason":""}]},"usage":{"input_tokens":9,"output_tokens":5,"total_tokens":14}}`
			chunk3 := `{"request_id":"req-123","output":{"choices":[{"message":{"role":"assistant","content":"Hello! How can I help you today?"},"finish_reason":"stop"}]},"usage":{"input_tokens":9,"output_tokens":12,"total_tokens":21}}`

			// 处理流式响应体
			action1 := host.CallOnHttpStreamingResponseBody([]byte(chunk1), false)
			require.Equal(t, types.ActionContinue, action1)

			action2 := host.CallOnHttpStreamingResponseBody([]byte(chunk2), false)
			require.Equal(t, types.ActionContinue, action2)

			action3 := host.CallOnHttpStreamingResponseBody([]byte(chunk3), true)
			require.Equal(t, types.ActionContinue, action3)

			// 验证流式响应处理
			// 注意：流式响应可能不会在GetResponseBody中累积，需要检查日志或其他方式验证
			debugLogs := host.GetDebugLogs()
			hasStreamingLogs := false
			for _, log := range debugLogs {
				if strings.Contains(log, "streaming") || strings.Contains(log, "chunk") || strings.Contains(log, "qwen") {
					hasStreamingLogs = true
					break
				}
			}
			require.True(t, hasStreamingLogs, "Should have streaming response processing logs")
		})

		// 测试qwen增量流式响应处理
		t.Run("qwen incremental streaming response body", func(t *testing.T) {
			host, status := test.NewTestHost(basicQwenConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			// 设置增量流式请求体
			requestBody := `{"model":"qwen-turbo","messages":[{"role":"user","content":"test"}],"stream":true}`
			host.CallOnHttpRequestBody([]byte(requestBody))

			// 设置流式响应头
			responseHeaders := [][2]string{
				{":status", "200"},
				{"Content-Type", "text/event-stream"},
			}
			host.CallOnHttpResponseHeaders(responseHeaders)

			// 模拟增量流式响应体
			chunk1 := `{"request_id":"req-123","output":{"choices":[{"message":{"role":"assistant","content":"H"},"finish_reason":""}]},"usage":{"input_tokens":9,"output_tokens":1,"total_tokens":10}}`
			chunk2 := `{"request_id":"req-123","output":{"choices":[{"message":{"role":"assistant","content":"He"},"finish_reason":""}]},"usage":{"input_tokens":9,"output_tokens":2,"total_tokens":11}}`
			chunk3 := `{"request_id":"req-123","output":{"choices":[{"message":{"role":"assistant","content":"Hello"},"finish_reason":""}]},"usage":{"input_tokens":9,"output_tokens":5,"total_tokens":14}}`
			chunk4 := `{"request_id":"req-123","output":{"choices":[{"message":{"role":"assistant","content":"Hello! How can I help you today?"},"finish_reason":"stop"}]},"usage":{"input_tokens":9,"output_tokens":12,"total_tokens":21}}`

			// 处理增量流式响应体
			action1 := host.CallOnHttpStreamingResponseBody([]byte(chunk1), false)
			require.Equal(t, types.ActionContinue, action1)

			action2 := host.CallOnHttpStreamingResponseBody([]byte(chunk2), false)
			require.Equal(t, types.ActionContinue, action2)

			action3 := host.CallOnHttpStreamingResponseBody([]byte(chunk3), false)
			require.Equal(t, types.ActionContinue, action3)

			action4 := host.CallOnHttpStreamingResponseBody([]byte(chunk4), true)
			require.Equal(t, types.ActionContinue, action4)

			// 验证增量流式响应处理
			debugLogs := host.GetDebugLogs()
			hasIncrementalLogs := false
			for _, log := range debugLogs {
				if strings.Contains(log, "incremental") || strings.Contains(log, "streaming") || strings.Contains(log, "qwen") {
					hasIncrementalLogs = true
					break
				}
			}
			require.True(t, hasIncrementalLogs, "Should have incremental streaming response processing logs")
		})

		// 测试qwen兼容模式流式响应处理
		t.Run("qwen compatible mode streaming response body", func(t *testing.T) {
			host, status := test.NewTestHost(qwenEnableCompatibleConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			// 设置流式请求体
			requestBody := `{"model":"qwen-turbo","messages":[{"role":"user","content":"test"}],"stream":true}`
			host.CallOnHttpRequestBody([]byte(requestBody))

			// 设置流式响应头
			responseHeaders := [][2]string{
				{":status", "200"},
				{"Content-Type", "text/event-stream"},
			}
			host.CallOnHttpResponseHeaders(responseHeaders)

			// 模拟兼容模式流式响应体
			chunk1 := `data: {"id":"chatcmpl-123","object":"chat.completion.chunk","choices":[{"delta":{"role":"assistant"},"index":0}]}

`
			chunk2 := `data: {"id":"chatcmpl-123","object":"chat.completion.chunk","choices":[{"delta":{"content":"Hello"},"index":0}]}

`
			chunk3 := `data: {"id":"chatcmpl-123","object":"chat.completion.chunk","choices":[{"delta":{"content":"!"},"index":0}]}

`
			chunk4 := `data: [DONE]

`

			// 处理兼容模式流式响应体
			action1 := host.CallOnHttpStreamingResponseBody([]byte(chunk1), false)
			require.Equal(t, types.ActionContinue, action1)

			action2 := host.CallOnHttpStreamingResponseBody([]byte(chunk2), false)
			require.Equal(t, types.ActionContinue, action2)

			action3 := host.CallOnHttpStreamingResponseBody([]byte(chunk3), false)
			require.Equal(t, types.ActionContinue, action3)

			action4 := host.CallOnHttpStreamingResponseBody([]byte(chunk4), true)
			require.Equal(t, types.ActionContinue, action4)

			// 验证兼容模式流式响应处理
			debugLogs := host.GetDebugLogs()
			hasCompatibleStreamingLogs := false
			for _, log := range debugLogs {
				if strings.Contains(log, "compatible") || strings.Contains(log, "streaming") || strings.Contains(log, "qwen") {
					hasCompatibleStreamingLogs = true
					break
				}
			}
			require.True(t, hasCompatibleStreamingLogs, "Should have compatible mode streaming response processing logs")
		})

		// 测试qwen多模态模型流式响应处理
		t.Run("qwen multimodal streaming response body", func(t *testing.T) {
			host, status := test.NewTestHost(qwenMultiModelConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			// 设置多模态流式请求体
			requestBody := `{"model":"qwen-vl-plus","messages":[{"role":"user","content":"test"}],"stream":true}`
			host.CallOnHttpRequestBody([]byte(requestBody))

			// 设置流式响应头
			responseHeaders := [][2]string{
				{":status", "200"},
				{"Content-Type", "text/event-stream"},
			}
			host.CallOnHttpResponseHeaders(responseHeaders)

			// 模拟多模态流式响应体
			chunk1 := `{"request_id":"req-123","output":{"choices":[{"message":{"role":"assistant","content":[{"text":"Hello","type":"text"}]},"finish_reason":""}]},"usage":{"input_tokens":9,"output_tokens":5,"total_tokens":14}}`
			chunk2 := `{"request_id":"req-123","output":{"choices":[{"message":{"role":"assistant","content":[{"text":"Hello! How can I help you today?","type":"text"}]},"finish_reason":"stop"}]},"usage":{"input_tokens":9,"output_tokens":12,"total_tokens":21}}`

			// 处理多模态流式响应体
			action1 := host.CallOnHttpStreamingResponseBody([]byte(chunk1), false)
			require.Equal(t, types.ActionContinue, action1)

			action2 := host.CallOnHttpStreamingResponseBody([]byte(chunk2), true)
			require.Equal(t, types.ActionContinue, action2)

			// 验证多模态流式响应处理
			debugLogs := host.GetDebugLogs()
			hasMultimodalLogs := false
			for _, log := range debugLogs {
				if strings.Contains(log, "vl") || strings.Contains(log, "multimodal") || strings.Contains(log, "qwen") {
					hasMultimodalLogs = true
					break
				}
			}
			require.True(t, hasMultimodalLogs, "Should have multimodal streaming response processing logs")
		})
	})
}
