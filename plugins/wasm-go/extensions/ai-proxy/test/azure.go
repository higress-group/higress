package test

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"strings"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 测试配置：基本Azure OpenAI配置
var basicAzureConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type": "azure",
			"apiTokens": []string{
				"sk-azure-test123456789",
			},
			"azureServiceUrl": "https://test-resource.openai.azure.com/openai/deployments/test-deployment/chat/completions?api-version=2024-02-15-preview",
			"modelMapping": map[string]string{
				"*": "gpt-3.5-turbo",
			},
		},
	})
	return data
}()

// 测试配置：Azure OpenAI完整路径配置
var azureFullPathConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type": "azure",
			"apiTokens": []string{
				"sk-azure-fullpath",
			},
			"azureServiceUrl": "https://fullpath-resource.openai.azure.com/openai/deployments/fullpath-deployment/chat/completions?api-version=2024-02-15-preview",
			"modelMapping": map[string]string{
				"gpt-3.5-turbo": "gpt-3.5-turbo",
				"gpt-4":         "gpt-4",
			},
		},
	})
	return data
}()

// 测试配置：Azure OpenAI仅部署配置
var azureDeploymentOnlyConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type": "azure",
			"apiTokens": []string{
				"sk-azure-deployment",
			},
			"azureServiceUrl": "https://deployment-resource.openai.azure.com/openai/deployments/deployment-only?api-version=2024-02-15-preview",
			"modelMapping": map[string]string{
				"*": "gpt-3.5-turbo",
			},
		},
	})
	return data
}()

// 测试配置：Azure OpenAI仅域名配置
var azureDomainOnlyConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type": "azure",
			"apiTokens": []string{
				"sk-azure-domain",
			},
			"azureServiceUrl": "https://domain-resource.openai.azure.com?api-version=2024-02-15-preview",
			"modelMapping": map[string]string{
				"*": "gpt-3.5-turbo",
			},
		},
	})
	return data
}()

var azureDomainOnlyImageMultipartConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type": "azure",
			"apiTokens": []string{
				"sk-azure-image-multipart",
			},
			"azureServiceUrl": "https://domain-resource.openai.azure.com?api-version=2024-02-15-preview",
			"modelMapping": map[string]string{
				"gpt-image-1.5": "gpt-image-1",
			},
		},
	})
	return data
}()

// 测试配置：Azure OpenAI多模型配置
var azureMultiModelConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type": "azure",
			"apiTokens": []string{
				"sk-azure-multi",
			},
			"azureServiceUrl": "https://multi-resource.openai.azure.com/openai/deployments/multi-deployment?api-version=2024-02-15-preview",
			"modelMapping": map[string]string{
				"gpt-3.5-turbo":          "gpt-3.5-turbo",
				"gpt-4":                  "gpt-4",
				"text-embedding-ada-002": "text-embedding-ada-002",
			},
		},
	})
	return data
}()

func getMultipartTextField(body []byte, contentType string, fieldName string) (string, bool, error) {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return "", false, err
	}
	boundary := params["boundary"]
	if boundary == "" {
		return "", false, nil
	}

	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			return "", false, nil
		}
		if err != nil {
			return "", false, err
		}

		data, err := io.ReadAll(part)
		_ = part.Close()
		if err != nil {
			return "", false, err
		}
		if part.FormName() == fieldName {
			return string(data), true, nil
		}
	}
}

func RunAzureMultipartHelperTests(t *testing.T) {
	t.Run("multipart text field returns error for invalid content type", func(t *testing.T) {
		_, _, err := getMultipartTextField([]byte("bad-body"), "multipart/form-data; boundary=\"", "model")
		require.Error(t, err)
	})

	t.Run("multipart text field returns not found for missing boundary", func(t *testing.T) {
		value, found, err := getMultipartTextField([]byte("bad-body"), "multipart/form-data", "model")
		require.NoError(t, err)
		require.False(t, found)
		require.Equal(t, "", value)
	})

	t.Run("multipart text field returns not found on eof", func(t *testing.T) {
		body, contentType := buildMultipartRequestBody(t, map[string]string{
			"model": "gpt-image-1.5",
		}, nil)

		value, found, err := getMultipartTextField(body, contentType, "prompt")
		require.NoError(t, err)
		require.False(t, found)
		require.Equal(t, "", value)
	})

	t.Run("multipart text field returns next part error on malformed body", func(t *testing.T) {
		body := []byte("--abc\r\nnot-a-header\r\n\r\nvalue\r\n--abc--\r\n")
		_, _, err := getMultipartTextField(body, "multipart/form-data; boundary=abc", "model")
		require.Error(t, err)
	})

	t.Run("multipart text field returns read error on truncated part", func(t *testing.T) {
		body := []byte("--abc\r\nContent-Disposition: form-data; name=\"model\"\r\n\r\nvalue\r\n--ab")
		_, _, err := getMultipartTextField(body, "multipart/form-data; boundary=abc", "model")
		require.Error(t, err)
	})
}

// 测试配置：Azure OpenAI无效配置（缺少azureServiceUrl）
var azureInvalidConfigMissingUrl = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type": "azure",
			"apiTokens": []string{
				"sk-azure-invalid",
			},
			"modelMapping": map[string]string{
				"*": "gpt-3.5-turbo",
			},
		},
	})
	return data
}()

// 测试配置：Azure OpenAI无效配置（缺少api-version）
var azureInvalidConfigMissingApiVersion = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type": "azure",
			"apiTokens": []string{
				"sk-azure-invalid",
			},
			"azureServiceUrl": "https://invalid-resource.openai.azure.com/openai/deployments/invalid-deployment/chat/completions",
			"modelMapping": map[string]string{
				"*": "gpt-3.5-turbo",
			},
		},
	})
	return data
}()

// 测试配置：Azure OpenAI无效配置（缺少apiToken）
var azureInvalidConfigMissingToken = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":            "azure",
			"azureServiceUrl": "https://invalid-resource.openai.azure.com/openai/deployments/invalid-deployment/chat/completions?api-version=2024-02-15-preview",
			"modelMapping": map[string]interface{}{
				"*": "gpt-3.5-turbo",
			},
		},
	})
	return data
}()

// 测试配置：Azure OpenAI Response API配置
var azureResponseAPIConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type": "azure",
			"apiTokens": []string{
				"sk-azure-multi",
			},
			"azureServiceUrl": "https://multi-resource.openai.azure.com/openai/responses?api-version=2025-04-01-preview",
		},
	})
	return data
}()

// 测试配置：Azure OpenAI basePath移除 + original协议
var azureBasePathRemovePrefixOriginalConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type": "azure",
			"apiTokens": []string{
				"sk-azure-basepath-original",
			},
			"azureServiceUrl":  "https://basepath-test.openai.azure.com/openai/deployments/gpt-4/chat/completions?api-version=2024-02-15-preview",
			"basePath":         "/azure-gpt4",
			"basePathHandling": "removePrefix",
			"protocol":         "original",
		},
	})
	return data
}()

// 测试配置：Azure OpenAI basePath移除 + openai协议
var azureBasePathRemovePrefixOpenAIConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type": "azure",
			"apiTokens": []string{
				"sk-azure-basepath-openai",
			},
			"azureServiceUrl":  "https://basepath-test.openai.azure.com/openai/deployments/gpt-4/chat/completions?api-version=2024-02-15-preview",
			"basePath":         "/azure-gpt4",
			"basePathHandling": "removePrefix",
			"modelMapping": map[string]string{
				"*": "gpt-4",
			},
		},
	})
	return data
}()

// 测试配置：Azure OpenAI basePath prepend + original协议
var azureBasePathPrependOriginalConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type": "azure",
			"apiTokens": []string{
				"sk-azure-prepend-original",
			},
			"azureServiceUrl":  "https://prepend-test.openai.azure.com/openai/deployments/gpt-4/chat/completions?api-version=2024-02-15-preview",
			"basePath":         "/api/v1",
			"basePathHandling": "prepend",
			"protocol":         "original",
		},
	})
	return data
}()

func RunAzureParseConfigTests(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// 测试基本Azure OpenAI配置解析
		t.Run("basic azure config", func(t *testing.T) {
			host, status := test.NewTestHost(basicAzureConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试Azure OpenAI完整路径配置解析
		t.Run("azure full path config", func(t *testing.T) {
			host, status := test.NewTestHost(azureFullPathConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试Azure OpenAI仅部署配置解析
		t.Run("azure deployment only config", func(t *testing.T) {
			host, status := test.NewTestHost(azureDeploymentOnlyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试Azure OpenAI仅域名配置解析
		t.Run("azure domain only config", func(t *testing.T) {
			host, status := test.NewTestHost(azureDomainOnlyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试Azure OpenAI多模型配置解析
		t.Run("azure multi model config", func(t *testing.T) {
			host, status := test.NewTestHost(azureMultiModelConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试Azure Response API 配置解析
		t.Run("azure response api config", func(t *testing.T) {
			host, status := test.NewTestHost(azureResponseAPIConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试Azure OpenAI无效配置（缺少azureServiceUrl）
		t.Run("azure invalid config missing url", func(t *testing.T) {
			host, status := test.NewTestHost(azureInvalidConfigMissingUrl)
			defer host.Reset()
			// 应该失败，因为缺少azureServiceUrl
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})

		// 测试Azure OpenAI无效配置（缺少api-version）
		t.Run("azure invalid config missing api version", func(t *testing.T) {
			host, status := test.NewTestHost(azureInvalidConfigMissingApiVersion)
			defer host.Reset()
			// 应该失败，因为缺少api-version
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})

		// 测试Azure OpenAI无效配置（缺少apiToken）
		t.Run("azure invalid config missing token", func(t *testing.T) {
			host, status := test.NewTestHost(azureInvalidConfigMissingToken)
			defer host.Reset()
			// 应该失败，因为缺少apiToken
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})
	})
}

func RunAzureOnHttpRequestHeadersTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试Azure OpenAI请求头处理（聊天完成接口）
		t.Run("azure chat completion request headers", func(t *testing.T) {
			host, status := test.NewTestHost(basicAzureConfig)
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

			// 验证Host是否被改为Azure服务域名
			hostValue, hasHost := test.GetHeaderValue(requestHeaders, ":authority")
			require.True(t, hasHost, "Host header should exist")
			require.Equal(t, "test-resource.openai.azure.com", hostValue, "Host should be changed to Azure service domain")

			// 验证api-key是否被设置
			apiKeyValue, hasApiKey := test.GetHeaderValue(requestHeaders, "api-key")
			require.True(t, hasApiKey, "api-key header should exist")
			require.Equal(t, "sk-azure-test123456789", apiKeyValue, "api-key should contain Azure API token")

			// 验证Path是否被正确处理
			pathValue, hasPath := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, hasPath, "Path header should exist")
			require.Equal(t, "/openai/deployments/test-deployment/chat/completions?api-version=2024-02-15-preview", pathValue, "Path should equal Azure deployment path")

			// 验证Content-Length是否被删除
			_, hasContentLength := test.GetHeaderValue(requestHeaders, "Content-Length")
			require.False(t, hasContentLength, "Content-Length header should be deleted")

			// 检查是否有相关的处理日志
			debugLogs := host.GetDebugLogs()
			hasAzureLogs := false
			for _, log := range debugLogs {
				if strings.Contains(log, "azureProvider") {
					hasAzureLogs = true
					break
				}
			}
			assert.True(t, hasAzureLogs, "Should have Azure provider debug logs")
		})

		// 测试Azure OpenAI请求头处理（完整路径配置）
		t.Run("azure full path request headers", func(t *testing.T) {
			host, status := test.NewTestHost(azureFullPathConfig)
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

			// 验证请求头是否被正确处理
			requestHeaders := host.GetRequestHeaders()
			require.NotNil(t, requestHeaders)

			// 验证Host是否被改为Azure服务域名
			hostValue, hasHost := test.GetHeaderValue(requestHeaders, ":authority")
			require.True(t, hasHost, "Host header should exist")
			require.Equal(t, "fullpath-resource.openai.azure.com", hostValue, "Host should be changed to Azure service domain")

			// 验证api-key是否被设置
			apiKeyValue, hasApiKey := test.GetHeaderValue(requestHeaders, "api-key")
			require.True(t, hasApiKey, "api-key header should exist")
			require.Equal(t, "sk-azure-fullpath", apiKeyValue, "api-key should contain Azure API token")
		})
	})
}

func RunAzureOnHttpRequestBodyTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试Azure OpenAI请求体处理（聊天完成接口）
		t.Run("azure chat completion request body", func(t *testing.T) {
			host, status := test.NewTestHost(basicAzureConfig)
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

			// 设置请求体
			requestBody := `{
				"model": "gpt-3.5-turbo",
				"messages": [
					{
						"role": "user",
						"content": "Hello, how are you?"
					}
				],
				"temperature": 0.7
			}`

			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			// 验证请求体是否被正确处理
			transformedBody := host.GetRequestBody()
			require.NotNil(t, transformedBody)

			// 验证模型映射是否生效
			var bodyMap map[string]interface{}
			err := json.Unmarshal(transformedBody, &bodyMap)
			require.NoError(t, err)

			model, exists := bodyMap["model"]
			require.True(t, exists, "Model should exist in request body")
			require.Equal(t, "gpt-3.5-turbo", model, "Model should be mapped correctly")

			// 验证请求路径是否被正确转换
			requestHeaders := host.GetRequestHeaders()
			pathValue, hasPath := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, hasPath, "Path header should exist")
			require.Equal(t, pathValue, "/openai/deployments/test-deployment/chat/completions?api-version=2024-02-15-preview", "Path should contain Azure deployment path")
		})

		// 测试Azure OpenAI请求体处理（不同模型）
		t.Run("azure different model request body", func(t *testing.T) {
			host, status := test.NewTestHost(azureMultiModelConfig)
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

			// 设置请求体
			requestBody := `{
				"model": "gpt-4",
				"messages": [
					{
						"role": "user",
						"content": "Explain quantum computing"
					}
				]
			}`

			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			// 验证请求体是否被正确处理
			transformedBody := host.GetRequestBody()
			require.NotNil(t, transformedBody)

			var bodyMap map[string]interface{}
			err := json.Unmarshal(transformedBody, &bodyMap)
			require.NoError(t, err)

			model, exists := bodyMap["model"]
			require.True(t, exists, "Model should exist in request body")
			require.Equal(t, "gpt-4", model, "Model should be mapped correctly")
		})

		// 测试Azure OpenAI Response API 处理
		t.Run("azure response api request body", func(t *testing.T) {
			host, status := test.NewTestHost(azureResponseAPIConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/responses/v1/responses"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			// 设置请求体
			requestBody := `{
                          "input": [
                            {
                              "role": "user",
                              "content": [
                                {
                                  "type": "input_text",
                                  "text": "Explain quantum computing"
                                }
                              ]
                            }
                          ],
                          "model": "gpt-5",
                          "reasoning": {
                            "effort": "medium"
                          }
                        }`
			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			// 验证请求体是否被正确处理
			transformedBody := host.GetRequestBody()
			require.NotNil(t, transformedBody)

			var bodyMap map[string]interface{}
			err := json.Unmarshal(transformedBody, &bodyMap)
			require.NoError(t, err)

			model, exists := bodyMap["model"]
			require.True(t, exists, "Model should exist in request body")
			require.Equal(t, "gpt-5", model, "Model should be mapped correctly")

			// 验证请求路径是否被正确转换
			requestHeaders := host.GetRequestHeaders()
			pathValue, hasPath := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, hasPath, "Path header should exist")
			require.Equal(t, pathValue, "/openai/responses?api-version=2025-04-01-preview", "Path should not equal  Azure response api path")
		})

		// 测试Azure OpenAI请求体处理（仅部署配置）
		t.Run("azure deployment only request body", func(t *testing.T) {
			host, status := test.NewTestHost(azureDeploymentOnlyConfig)
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

			// 设置请求体
			requestBody := `{
				"model": "gpt-3.5-turbo",
				"messages": [
					{
						"role": "user",
						"content": "Test message"
					}
				]
			}`

			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			// 验证请求路径是否使用默认部署
			requestHeaders := host.GetRequestHeaders()
			pathValue, hasPath := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, hasPath, "Path header should exist")
			require.Equal(t, pathValue, "/openai/deployments/deployment-only/chat/completions?api-version=2024-02-15-preview", "Path should use default deployment")
		})

		// 测试Azure OpenAI请求体处理（仅域名配置）
		t.Run("azure domain only request body", func(t *testing.T) {
			host, status := test.NewTestHost(azureDomainOnlyConfig)
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

			// 设置请求体
			requestBody := `{
				"model": "gpt-3.5-turbo",
				"messages": [
					{
						"role": "user",
						"content": "Test message"
					}
				]
			}`

			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			// 验证请求路径是否使用模型占位符
			requestHeaders := host.GetRequestHeaders()
			pathValue, hasPath := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, hasPath, "Path header should exist")
			require.Equal(t, pathValue, "/openai/deployments/gpt-3.5-turbo/chat/completions?api-version=2024-02-15-preview", "Path should use model from request body")
		})

		t.Run("azure domain only multipart image edit request body", func(t *testing.T) {
			host, status := test.NewTestHost(azureDomainOnlyImageMultipartConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			body, contentType := buildMultipartRequestBody(t, map[string]string{
				"model":  "gpt-image-1.5",
				"prompt": "把小狗换成白色",
				"size":   "1024x1024",
				"n":      "1",
			}, map[string][]byte{
				"image[]": []byte("fake-image-content"),
			})

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/images/edits"},
				{":method", "POST"},
				{"Content-Type", contentType},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			action = host.CallOnHttpRequestBody(body)
			require.Equal(t, types.ActionContinue, action)

			transformedBody := host.GetRequestBody()
			require.NotNil(t, transformedBody)

			modelValue, found, err := getMultipartTextField(transformedBody, contentType, "model")
			require.NoError(t, err)
			require.True(t, found, "Model field should exist in multipart body")
			require.Equal(t, "gpt-image-1", modelValue, "Model field should be mapped in multipart body")
			require.Contains(t, string(transformedBody), "fake-image-content", "Image file content should remain in multipart body")

			requestHeaders := host.GetRequestHeaders()
			pathValue, hasPath := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, hasPath, "Path header should exist")
			require.Equal(t, "/openai/deployments/gpt-image-1/images/edits?api-version=2024-02-15-preview", pathValue, "Path should use mapped multipart model")

			contentTypeValue, hasContentType := test.GetHeaderValue(requestHeaders, "Content-Type")
			require.True(t, hasContentType, "Content-Type header should exist")
			require.Equal(t, contentType, contentTypeValue, "Multipart Content-Type should remain unchanged")
		})

		t.Run("azure domain only multipart image variation request body", func(t *testing.T) {
			host, status := test.NewTestHost(azureDomainOnlyImageMultipartConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			body, contentType := buildMultipartRequestBody(t, map[string]string{
				"model":  "gpt-image-1.5",
				"prompt": "生成类似风格",
				"size":   "1024x1024",
				"n":      "1",
			}, map[string][]byte{
				"image": []byte("fake-image-content"),
			})

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/images/variations"},
				{":method", "POST"},
				{"Content-Type", contentType},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			action = host.CallOnHttpRequestBody(body)
			require.Equal(t, types.ActionContinue, action)

			transformedBody := host.GetRequestBody()
			require.NotNil(t, transformedBody)

			modelValue, found, err := getMultipartTextField(transformedBody, contentType, "model")
			require.NoError(t, err)
			require.True(t, found, "Model field should exist in multipart body")
			require.Equal(t, "gpt-image-1", modelValue, "Model field should be mapped in multipart body")
			require.Contains(t, string(transformedBody), "fake-image-content", "Image file content should remain in multipart body")

			requestHeaders := host.GetRequestHeaders()
			pathValue, hasPath := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, hasPath, "Path header should exist")
			require.Equal(t, "/openai/deployments/gpt-image-1/images/variations?api-version=2024-02-15-preview", pathValue, "Path should use mapped multipart model")

			contentTypeValue, hasContentType := test.GetHeaderValue(requestHeaders, "Content-Type")
			require.True(t, hasContentType, "Content-Type header should exist")
			require.Equal(t, contentType, contentTypeValue, "Multipart Content-Type should remain unchanged")
		})

		t.Run("azure domain only multipart malformed body logs transform failure", func(t *testing.T) {
			host, status := test.NewTestHost(azureDomainOnlyImageMultipartConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/images/edits"},
				{":method", "POST"},
				{"Content-Type", "multipart/form-data"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			action = host.CallOnHttpRequestBody([]byte("bad-multipart-body"))
			require.Equal(t, types.ActionContinue, action)

			debugLogs := host.GetDebugLogs()
			hasMultipartTransformFailureLog := false
			for _, debugLog := range debugLogs {
				if strings.Contains(debugLog, "[azure multipart] body transform failed") {
					hasMultipartTransformFailureLog = true
					break
				}
			}
			require.True(t, hasMultipartTransformFailureLog, "Should log azure multipart transform failure")
		})

		// 测试Azure OpenAI模型无关请求处理（仅域名配置）
		t.Run("azure domain only model independent", func(t *testing.T) {
			host, status := test.NewTestHost(azureDomainOnlyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/files?limit=10&purpose=assistants"},
				{":method", "GET"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			// 验证请求路径是否使用模型占位符
			requestHeaders := host.GetRequestHeaders()
			pathValue, hasPath := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, hasPath, "Path header should exist")
			require.Equal(t, pathValue, "/openai/files?limit=10&purpose=assistants&api-version=2024-02-15-preview", "Path should have api-version appended")

			// 设置请求头
			action = host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/files?"},
				{":method", "GET"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			// 验证请求路径是否使用模型占位符
			requestHeaders = host.GetRequestHeaders()
			pathValue, hasPath = test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, hasPath, "Path header should exist")
			require.Equal(t, pathValue, "/openai/files?api-version=2024-02-15-preview", "Path should have api-version appended")
		})
	})
}

func RunAzureOnHttpResponseHeadersTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试Azure OpenAI响应头处理
		t.Run("azure response headers", func(t *testing.T) {
			host, status := test.NewTestHost(basicAzureConfig)
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

			// 设置请求体
			requestBody := `{
				"model": "gpt-3.5-turbo",
				"messages": [
					{
						"role": "user",
						"content": "Hello"
					}
				]
			}`
			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			// 处理响应头
			action = host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.ActionContinue, action)

			// 验证响应头是否被正确处理
			responseHeaders := host.GetResponseHeaders()
			require.NotNil(t, responseHeaders)

			// 验证状态码
			statusValue, hasStatus := test.GetHeaderValue(responseHeaders, ":status")
			require.True(t, hasStatus, "Status header should exist")
			require.Equal(t, "200", statusValue, "Status should be 200")

			// 验证Content-Type
			contentTypeValue, hasContentType := test.GetHeaderValue(responseHeaders, "Content-Type")
			require.True(t, hasContentType, "Content-Type header should exist")
			require.Equal(t, "application/json", contentTypeValue, "Content-Type should be application/json")
		})
	})
}

func RunAzureOnHttpResponseBodyTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试Azure OpenAI响应体处理
		t.Run("azure response body", func(t *testing.T) {
			host, status := test.NewTestHost(basicAzureConfig)
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

			// 设置请求体
			requestBody := `{
				"model": "gpt-3.5-turbo",
				"messages": [
					{
						"role": "user",
						"content": "Hello"
					}
				]
			}`
			action = host.CallOnHttpResponseHeaders([][2]string{
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.ActionContinue, action)
			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			// 处理响应体
			responseBody := `{
				"choices": [
					{
						"message": {
							"content": "Hello! How can I help you?"
						}
					}
				]
			}`

			action = host.CallOnHttpResponseBody([]byte(responseBody))
			require.Equal(t, types.ActionContinue, action)

			// 验证响应体是否被正确处理
			transformedResponseBody := host.GetResponseBody()
			require.NotNil(t, transformedResponseBody)

			// 验证响应体内容
			var responseMap map[string]interface{}
			err := json.Unmarshal(transformedResponseBody, &responseMap)
			require.NoError(t, err)

			choices, exists := responseMap["choices"]
			require.True(t, exists, "Choices should exist in response body")
			require.NotNil(t, choices, "Choices should not be nil")
		})
	})
}

// RunAzureBasePathHandlingTests 测试 basePath 处理在不同协议下的行为
func RunAzureBasePathHandlingTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 核心用例：测试 basePath removePrefix 在 original 协议下能正常工作
		// 重要：此测试验证在 TransformRequestBody 阶段后 path 仍然保持正确
		// 之前的 bug 是 transformRequestPath 在 IsOriginal() 时返回 originalPath，
		// 导致在 Body 阶段 path 被重新覆盖为包含 basePath 的原始路径
		t.Run("azure basePath removePrefix with original protocol after body processing", func(t *testing.T) {
			host, status := test.NewTestHost(azureBasePathRemovePrefixOriginalConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 模拟带有 basePath 前缀的请求
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/azure-gpt4/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			// 在 Headers 阶段后验证 path（此时 handleRequestHeaders 已执行）
			headersAfterHeaderStage := host.GetRequestHeaders()
			pathAfterHeaders, _ := test.GetHeaderValue(headersAfterHeaderStage, ":path")
			// Headers 阶段后，basePath 应该已被移除
			require.NotContains(t, pathAfterHeaders, "/azure-gpt4",
				"After headers stage: basePath should be removed")

			// 执行 Body 阶段（此时 TransformRequestBody 会被调用）
			requestBody := `{"model": "gpt-4", "messages": [{"role": "user", "content": "Hello"}]}`
			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			// 核心验证：在 Body 阶段后验证 path
			// 这是关键测试点：确保 TransformRequestBody 中的 transformRequestPath
			// 不会将 path 重新覆盖为包含 basePath 的原始路径
			requestHeaders := host.GetRequestHeaders()
			require.NotNil(t, requestHeaders)

			pathValue, hasPath := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, hasPath, "Path header should exist")
			// basePath "/azure-gpt4" 不应该出现在最终路径中
			require.NotContains(t, pathValue, "/azure-gpt4",
				"After body stage: basePath should still be removed (not restored by TransformRequestBody)")
			// 路径应该是移除 basePath 后的结果
			require.Equal(t, "/v1/chat/completions", pathValue,
				"Path should be the original path without basePath after full request processing")

			// 验证 Host 被正确设置
			hostValue, hasHost := test.GetHeaderValue(requestHeaders, ":authority")
			require.True(t, hasHost, "Host header should exist")
			require.Equal(t, "basepath-test.openai.azure.com", hostValue)

			// 验证 api-key 被正确设置
			apiKeyValue, hasApiKey := test.GetHeaderValue(requestHeaders, "api-key")
			require.True(t, hasApiKey, "api-key header should exist")
			require.Equal(t, "sk-azure-basepath-original", apiKeyValue)
		})

		// 测试 basePath removePrefix 在 openai 协议下的行为
		// 在 openai 协议下，path 会被转换为 Azure 格式，但 basePath 仍然应该被移除
		t.Run("azure basePath removePrefix with openai protocol after body processing", func(t *testing.T) {
			host, status := test.NewTestHost(azureBasePathRemovePrefixOpenAIConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 模拟带有 basePath 前缀的请求
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/azure-gpt4/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			// 执行 Body 阶段（TransformRequestBody 会被调用）
			requestBody := `{"model": "gpt-4", "messages": [{"role": "user", "content": "Hello"}]}`
			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			// 在 Body 阶段后验证请求头
			requestHeaders := host.GetRequestHeaders()
			require.NotNil(t, requestHeaders)

			// basePath 应该被移除，路径会被转换为 Azure 路径格式
			pathValue, hasPath := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, hasPath, "Path header should exist")
			// basePath "/azure-gpt4" 不应该出现在最终路径中
			require.NotContains(t, pathValue, "/azure-gpt4",
				"After body stage: basePath should be removed from path")
			// 在 openai 协议下，路径会被转换为 Azure 的路径格式
			require.Equal(t, pathValue, "/openai/deployments/gpt-4/chat/completions?api-version=2024-02-15-preview",
				"Path should be transformed to Azure format")
		})

		// 测试 basePath prepend 在 original 协议下能正常工作
		// 验证在 Body 阶段后 prepend 的 basePath 仍然保持
		t.Run("azure basePath prepend with original protocol after body processing", func(t *testing.T) {
			host, status := test.NewTestHost(azureBasePathPrependOriginalConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 模拟不带 basePath 的请求
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			// 执行 Body 阶段
			requestBody := `{"model": "gpt-4", "messages": [{"role": "user", "content": "Hello"}]}`
			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			// 在 Body 阶段后验证请求头
			requestHeaders := host.GetRequestHeaders()
			require.NotNil(t, requestHeaders)

			// 验证 basePath 被正确添加且在 Body 阶段后保持
			pathValue, hasPath := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, hasPath, "Path header should exist")
			// basePath "/api/v1" 应该被添加到路径前面
			require.True(t, strings.HasPrefix(pathValue, "/api/v1"),
				"After body stage: Path should still start with prepended basePath")
		})

		// 测试 original 协议下请求体不被修改，同时验证 path 处理
		t.Run("azure original protocol preserves request body and path", func(t *testing.T) {
			host, status := test.NewTestHost(azureBasePathRemovePrefixOriginalConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 设置请求头
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/azure-gpt4/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			// 设置请求体（包含自定义字段）
			requestBody := `{
				"model": "custom-model-name",
				"messages": [{"role": "user", "content": "Hello"}],
				"custom_field": "custom_value"
			}`
			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			// 验证请求体被保持原样
			transformedBody := host.GetRequestBody()
			require.NotNil(t, transformedBody)

			var bodyMap map[string]interface{}
			err := json.Unmarshal(transformedBody, &bodyMap)
			require.NoError(t, err)

			// model 应该保持原样（original 协议不做模型映射）
			model, exists := bodyMap["model"]
			require.True(t, exists, "Model should exist")
			require.Equal(t, "custom-model-name", model, "Model should remain unchanged")

			// 自定义字段应该保持原样
			customField, exists := bodyMap["custom_field"]
			require.True(t, exists, "Custom field should exist")
			require.Equal(t, "custom_value", customField, "Custom field should remain unchanged")

			// 同时验证 path 在 Body 阶段后仍然正确
			requestHeaders := host.GetRequestHeaders()
			pathValue, hasPath := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, hasPath, "Path header should exist")
			require.NotContains(t, pathValue, "/azure-gpt4",
				"After body stage: basePath should be removed")
			require.Equal(t, "/v1/chat/completions", pathValue,
				"Path should be correct after body processing")
		})

		// 测试无 basePath 前缀的请求（removePrefix 配置不影响）
		// 验证在 Body 阶段后 path 仍然保持正确
		t.Run("azure request without basePath prefix after body processing", func(t *testing.T) {
			host, status := test.NewTestHost(azureBasePathRemovePrefixOriginalConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 模拟不带 basePath 前缀的请求
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, action)

			// 执行 Body 阶段
			requestBody := `{"model": "gpt-4", "messages": [{"role": "user", "content": "Hello"}]}`
			action = host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			// 在 Body 阶段后验证请求头
			requestHeaders := host.GetRequestHeaders()
			pathValue, hasPath := test.GetHeaderValue(requestHeaders, ":path")
			require.True(t, hasPath, "Path header should exist")
			// 路径应该保持原样（没有 basePath 前缀时，removePrefix 不会改变 path）
			// 同时验证 TransformRequestBody 没有覆盖 path
			require.Equal(t, "/v1/chat/completions", pathValue,
				"After body stage: Path should remain unchanged when no basePath prefix")
		})
	})
}
