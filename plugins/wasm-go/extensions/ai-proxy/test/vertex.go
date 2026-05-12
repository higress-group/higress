package test

import (
	"bytes"
	"encoding/json"
	"math/rand"
	"mime/multipart"
	"net/url"
	"strings"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

// 测试配置：Vertex 标准模式配置
var basicVertexConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":                  "vertex",
			"vertexAuthKey":         `{"type":"service_account","client_email":"test@test.iam.gserviceaccount.com","private_key":"-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQC7k1v5C7y8L4SN\n-----END PRIVATE KEY-----\n","token_uri":"https://oauth2.googleapis.com/token"}`,
			"vertexRegion":          "us-central1",
			"vertexProjectId":       "test-project-id",
			"vertexAuthServiceName": "test-auth-service",
		},
	})
	return data
}()

// 测试配置：Vertex Express Mode 配置（使用 apiTokens）
var vertexExpressModeConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":      "vertex",
			"apiTokens": []string{"test-api-key-123456789"},
		},
	})
	return data
}()

// 测试配置：Vertex Express Mode 配置（多 API Token）
var vertexExpressModeMultiTokensConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":      "vertex",
			"apiTokens": []string{"test-api-key-express-a", "test-api-key-express-b"},
		},
	})
	return data
}()

// 测试配置：Vertex Express Mode 配置（含模型映射）
var vertexExpressModeWithModelMappingConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":      "vertex",
			"apiTokens": []string{"test-api-key-123456789"},
			"modelMapping": map[string]string{
				"gpt-4":                  "gemini-2.5-flash",
				"gpt-3.5-turbo":          "gemini-2.5-flash-lite",
				"text-embedding-ada-002": "text-embedding-001",
			},
		},
	})
	return data
}()

// 测试配置：Vertex Express Mode 配置（含安全设置）
var vertexExpressModeWithSafetyConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":      "vertex",
			"apiTokens": []string{"test-api-key-123456789"},
			"geminiSafetySetting": map[string]string{
				"HARM_CATEGORY_HARASSMENT":        "BLOCK_MEDIUM_AND_ABOVE",
				"HARM_CATEGORY_HATE_SPEECH":       "BLOCK_LOW_AND_ABOVE",
				"HARM_CATEGORY_SEXUALLY_EXPLICIT": "BLOCK_NONE",
			},
		},
	})
	return data
}()

// 测试配置：无效 Vertex 标准模式配置（缺少 vertexAuthKey）
var invalidVertexStandardModeConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type": "vertex",
			// 缺少必需的标准模式配置
		},
	})
	return data
}()

// 测试配置：Vertex OpenAI 兼容模式配置
var vertexOpenAICompatibleModeConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":                   "vertex",
			"vertexOpenAICompatible": true,
			"vertexAuthKey":          `{"type":"service_account","client_email":"test@test.iam.gserviceaccount.com","private_key":"-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQC7k1v5C7y8L4SN\n-----END PRIVATE KEY-----\n","token_uri":"https://oauth2.googleapis.com/token"}`,
			"vertexRegion":           "us-central1",
			"vertexProjectId":        "test-project-id",
			"vertexAuthServiceName":  "test-auth-service",
		},
	})
	return data
}()

// 测试配置：Vertex OpenAI 兼容模式配置（含模型映射）
var vertexOpenAICompatibleModeWithModelMappingConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":                   "vertex",
			"vertexOpenAICompatible": true,
			"vertexAuthKey":          `{"type":"service_account","client_email":"test@test.iam.gserviceaccount.com","private_key":"-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQC7k1v5C7y8L4SN\n-----END PRIVATE KEY-----\n","token_uri":"https://oauth2.googleapis.com/token"}`,
			"vertexRegion":           "us-central1",
			"vertexProjectId":        "test-project-id",
			"vertexAuthServiceName":  "test-auth-service",
			"modelMapping": map[string]string{
				"gpt-4":         "gemini-2.0-flash",
				"gpt-3.5-turbo": "gemini-1.5-flash",
			},
		},
	})
	return data
}()

// 测试配置：无效配置 - Express Mode 与 OpenAI 兼容模式互斥
var invalidVertexExpressAndOpenAICompatibleConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":                   "vertex",
			"apiTokens":              []string{"test-api-key"},
			"vertexOpenAICompatible": true,
		},
	})
	return data
}()

// 测试配置：Vertex Raw 模式配置（Express Mode + 原生 Vertex API 路径）
var vertexRawModeExpressConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":      "vertex",
			"apiTokens": []string{"test-api-key-for-raw-mode"},
			"protocol":  "original",
		},
	})
	return data
}()

// 测试配置：Vertex Raw 模式配置（标准模式 + 原生 Vertex API 路径）
var vertexRawModeStandardConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":                  "vertex",
			"vertexAuthKey":         `{"type":"service_account","client_email":"test@test.iam.gserviceaccount.com","private_key":"-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQC7k1v5C7y8L4SN\n-----END PRIVATE KEY-----\n","token_uri":"https://oauth2.googleapis.com/token"}`,
			"vertexRegion":          "us-central1",
			"vertexProjectId":       "test-project-id",
			"vertexAuthServiceName": "test-auth-service",
			"protocol":              "original",
		},
	})
	return data
}()

// 测试配置：Vertex Raw 模式配置（Express Mode + basePath removePrefix）
var vertexRawModeWithBasePathConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":             "vertex",
			"apiTokens":        []string{"test-api-key-for-raw-mode"},
			"protocol":         "original",
			"basePath":         "/vertex-proxy",
			"basePathHandling": "removePrefix",
		},
	})
	return data
}()

// 测试配置：Vertex Raw 模式配置（Express Mode + 多 API Token）
var vertexRawModeExpressMultiTokensConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"provider": map[string]interface{}{
			"type":      "vertex",
			"apiTokens": []string{"test-api-key-raw-a", "test-api-key-raw-b"},
			"protocol":  "original",
		},
	})
	return data
}()

func RunVertexParseConfigTests(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		// 测试 Vertex 标准模式配置解析
		t.Run("vertex standard mode config", func(t *testing.T) {
			host, status := test.NewTestHost(basicVertexConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试 Vertex Express Mode 配置解析
		t.Run("vertex express mode config", func(t *testing.T) {
			host, status := test.NewTestHost(vertexExpressModeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试 Vertex Express Mode 配置（含模型映射）
		t.Run("vertex express mode with model mapping config", func(t *testing.T) {
			host, status := test.NewTestHost(vertexExpressModeWithModelMappingConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试无效 Vertex 标准模式配置（缺少 vertexAuthKey）
		t.Run("invalid vertex standard mode config - missing auth key", func(t *testing.T) {
			host, status := test.NewTestHost(invalidVertexStandardModeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})

		// 测试 Vertex Express Mode 配置（含安全设置）
		t.Run("vertex express mode with safety setting config", func(t *testing.T) {
			host, status := test.NewTestHost(vertexExpressModeWithSafetyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试 Vertex OpenAI 兼容模式配置解析
		t.Run("vertex openai compatible mode config", func(t *testing.T) {
			host, status := test.NewTestHost(vertexOpenAICompatibleModeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试 Vertex OpenAI 兼容模式配置（含模型映射）
		t.Run("vertex openai compatible mode with model mapping config", func(t *testing.T) {
			host, status := test.NewTestHost(vertexOpenAICompatibleModeWithModelMappingConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			config, err := host.GetMatchConfig()
			require.NoError(t, err)
			require.NotNil(t, config)
		})

		// 测试无效配置 - Express Mode 与 OpenAI 兼容模式互斥
		t.Run("invalid config - express mode and openai compatible mode conflict", func(t *testing.T) {
			host, status := test.NewTestHost(invalidVertexExpressAndOpenAICompatibleConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusFailed, status)
		})
	})
}

func RunVertexExpressModeOnHttpRequestHeadersTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试 Vertex Express Mode 请求头处理（聊天完成接口）
		t.Run("vertex express mode chat completion request headers", func(t *testing.T) {
			host, status := test.NewTestHost(vertexExpressModeConfig)
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

			// 验证Host是否被改为 vertex 域名（Express Mode 使用不带 region 前缀的域名）
			require.True(t, test.HasHeaderWithValue(requestHeaders, ":authority", "aiplatform.googleapis.com"), "Host header should be changed to vertex domain without region prefix")

			// 检查是否有相关的处理日志
			debugLogs := host.GetDebugLogs()
			hasVertexLogs := false
			for _, log := range debugLogs {
				if strings.Contains(log, "vertex") {
					hasVertexLogs = true
					break
				}
			}
			require.True(t, hasVertexLogs, "Should have vertex processing logs")
		})

		// 测试 Vertex Express Mode 请求头处理（嵌入接口）
		t.Run("vertex express mode embeddings request headers", func(t *testing.T) {
			host, status := test.NewTestHost(vertexExpressModeConfig)
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
			require.True(t, test.HasHeaderWithValue(requestHeaders, ":authority", "aiplatform.googleapis.com"), "Host header should be changed to vertex domain")
		})
	})
}

func RunVertexExpressModeOnHttpRequestBodyTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试 Vertex Express Mode 请求体处理（聊天完成接口）
		t.Run("vertex express mode chat completion request body", func(t *testing.T) {
			host, status := test.NewTestHost(vertexExpressModeConfig)
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
			requestBody := `{"model":"gemini-2.5-flash","messages":[{"role":"user","content":"test"}]}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			// Express Mode 不需要暂停等待 OAuth token
			require.Equal(t, types.ActionContinue, action)

			// 验证请求体是否被正确处理
			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)

			// 验证请求体被转换为 Vertex 格式
			require.Contains(t, string(processedBody), "contents", "Request should be converted to vertex format")
			require.Contains(t, string(processedBody), "generationConfig", "Request should contain vertex generation config")

			// 验证路径包含 API Key
			requestHeaders := host.GetRequestHeaders()
			pathHeader := ""
			for _, header := range requestHeaders {
				if header[0] == ":path" {
					pathHeader = header[1]
					break
				}
			}
			require.Contains(t, pathHeader, "key=test-api-key-123456789", "Path should contain API key as query parameter")
			require.Contains(t, pathHeader, "/v1/publishers/google/models/", "Path should use Express Mode format without project/location")

			// 验证没有 Authorization header（Express Mode 使用 URL 参数）
			hasAuthHeader := false
			for _, header := range requestHeaders {
				if header[0] == "Authorization" && header[1] != "" {
					hasAuthHeader = true
					break
				}
			}
			require.False(t, hasAuthHeader, "Authorization header should be removed in Express Mode")

			// 检查是否有相关的处理日志
			debugLogs := host.GetDebugLogs()
			hasVertexLogs := false
			for _, log := range debugLogs {
				if strings.Contains(log, "vertex") {
					hasVertexLogs = true
					break
				}
			}
			require.True(t, hasVertexLogs, "Should have vertex processing logs")
		})

		// 测试 Vertex Express Mode 请求体处理（多 token - Google 路径使用请求上下文中的 apiTokenInUse）
		t.Run("vertex express mode chat completion should reuse api token in context", func(t *testing.T) {
			host, status := test.NewTestHost(vertexExpressModeMultiTokensConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			tokens := []string{"test-api-key-express-a", "test-api-key-express-b"}

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			// 从 debug log 中提取请求头阶段固定的 apiTokenInUse
			var apiTokenInUse string
			for _, debugLog := range host.GetDebugLogs() {
				const prefix = "Use apiToken "
				const suffix = " to send request"
				start := strings.Index(debugLog, prefix)
				if start == -1 {
					continue
				}
				start += len(prefix)
				end := strings.Index(debugLog[start:], suffix)
				if end == -1 {
					continue
				}
				apiTokenInUse = debugLog[start : start+end]
				break
			}
			require.Contains(t, tokens, apiTokenInUse, "apiTokenInUse should be selected from configured tokens")

			// 强制设置随机种子，让旧实现（OnRequestBody 再次随机）必然选到不同 token
			targetIndex := 0
			if apiTokenInUse == tokens[0] {
				targetIndex = 1
			}
			seed := int64(1)
			for {
				if rand.New(rand.NewSource(seed)).Intn(len(tokens)) == targetIndex {
					break
				}
				seed++
			}
			rand.Seed(seed)

			requestBody := `{"model":"gemini-2.5-flash","messages":[{"role":"user","content":"token consistency test"}]}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			requestHeaders := host.GetRequestHeaders()
			pathHeader := ""
			for _, header := range requestHeaders {
				if header[0] == ":path" {
					pathHeader = header[1]
					break
				}
			}
			require.NotEmpty(t, pathHeader, "Path header should not be empty")
			require.Contains(t, pathHeader, "/v1/publishers/google/models/", "Path should use Google publisher endpoint")

			parsedPath, err := url.ParseRequestURI(pathHeader)
			require.NoError(t, err)
			query := parsedPath.Query()
			require.Len(t, query["key"], 1, "Path should contain exactly one key query parameter")
			require.Equal(t, apiTokenInUse, query.Get("key"),
				"Path key should use apiTokenInUse selected in request headers phase")
		})

		// 测试 Vertex Express Mode 请求体处理（多 token - Anthropic 路径使用请求上下文中的 apiTokenInUse）
		t.Run("vertex express mode anthropic request should reuse api token in context", func(t *testing.T) {
			host, status := test.NewTestHost(vertexExpressModeMultiTokensConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			tokens := []string{"test-api-key-express-a", "test-api-key-express-b"}

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			// 从 debug log 中提取请求头阶段固定的 apiTokenInUse
			var apiTokenInUse string
			for _, debugLog := range host.GetDebugLogs() {
				const prefix = "Use apiToken "
				const suffix = " to send request"
				start := strings.Index(debugLog, prefix)
				if start == -1 {
					continue
				}
				start += len(prefix)
				end := strings.Index(debugLog[start:], suffix)
				if end == -1 {
					continue
				}
				apiTokenInUse = debugLog[start : start+end]
				break
			}
			require.Contains(t, tokens, apiTokenInUse, "apiTokenInUse should be selected from configured tokens")

			// 强制设置随机种子，让旧实现（OnRequestBody 再次随机）必然选到不同 token
			targetIndex := 0
			if apiTokenInUse == tokens[0] {
				targetIndex = 1
			}
			seed := int64(1)
			for {
				if rand.New(rand.NewSource(seed)).Intn(len(tokens)) == targetIndex {
					break
				}
				seed++
			}
			rand.Seed(seed)

			requestBody := `{"model":"claude-sonnet-4@20250514","messages":[{"role":"user","content":"hello anthropic"}]}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			requestHeaders := host.GetRequestHeaders()
			pathHeader := ""
			for _, header := range requestHeaders {
				if header[0] == ":path" {
					pathHeader = header[1]
					break
				}
			}
			require.NotEmpty(t, pathHeader, "Path header should not be empty")
			require.Contains(t, pathHeader, "/v1/publishers/anthropic/models/claude-sonnet-4@20250514:rawPredict",
				"Path should use Anthropic publisher endpoint")

			parsedPath, err := url.ParseRequestURI(pathHeader)
			require.NoError(t, err)
			query := parsedPath.Query()
			require.Len(t, query["key"], 1, "Path should contain exactly one key query parameter")
			require.Equal(t, apiTokenInUse, query.Get("key"),
				"Path key should use apiTokenInUse selected in request headers phase")
		})

		// 测试 Vertex Express Mode structured outputs: json_schema 映射
		t.Run("vertex express mode structured outputs json_schema request body mapping", func(t *testing.T) {
			host, status := test.NewTestHost(vertexExpressModeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			requestBody := `{
				"model":"gemini-2.5-flash",
				"messages":[{"role":"user","content":"return structured output"}],
				"response_format":{
					"type":"json_schema",
					"json_schema":{
						"name":"demo_schema",
						"strict":true,
						"schema":{
							"type":"object",
							"properties":{
								"answer":{"type":"string"}
							},
							"required":["answer"]
						}
					}
				}
			}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)

			var transformed map[string]interface{}
			require.NoError(t, json.Unmarshal(processedBody, &transformed))

			generationConfig, ok := transformed["generationConfig"].(map[string]interface{})
			require.True(t, ok, "generationConfig should exist")
			require.Equal(t, "application/json", generationConfig["responseMimeType"], "responseMimeType should be mapped for json_schema")

			responseSchema, ok := generationConfig["responseSchema"].(map[string]interface{})
			require.True(t, ok, "responseSchema should be mapped from response_format.json_schema.schema")
			require.Equal(t, "object", responseSchema["type"])

			properties, ok := responseSchema["properties"].(map[string]interface{})
			require.True(t, ok, "responseSchema.properties should exist")
			_, hasAnswer := properties["answer"]
			require.True(t, hasAnswer, "responseSchema.properties.answer should exist")
		})

		// 测试 Gemini 2.0 structured outputs: 忽略 response_format，按非结构化输出处理
		t.Run("vertex express mode structured outputs gemini 2.0 ignore response format", func(t *testing.T) {
			host, status := test.NewTestHost(vertexExpressModeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			requestBody := `{
				"model":"gemini-2.0-flash",
				"messages":[{"role":"user","content":"return structured output"}],
				"response_format":{
					"type":"json_schema",
					"json_schema":{
						"name":"demo_schema",
						"strict":true,
						"schema":{
							"type":"object",
							"properties":{
								"beta":{"type":"string"},
								"alpha":{
									"type":"object",
									"properties":{
										"z":{"type":"string"},
										"a":{"type":"string"}
									}
								}
							}
						}
					}
				}
			}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)

			var transformed map[string]interface{}
			require.NoError(t, json.Unmarshal(processedBody, &transformed))

			generationConfig, ok := transformed["generationConfig"].(map[string]interface{})
			require.True(t, ok, "generationConfig should exist")
			_, hasMimeType := generationConfig["responseMimeType"]
			_, hasSchema := generationConfig["responseSchema"]
			require.False(t, hasMimeType, "gemini-2.0 should ignore response_format and not set responseMimeType")
			require.False(t, hasSchema, "gemini-2.0 should ignore response_format and not set responseSchema")
		})

		// 测试 Vertex Express Mode structured outputs: json_object 映射
		t.Run("vertex express mode structured outputs json_object request body mapping", func(t *testing.T) {
			host, status := test.NewTestHost(vertexExpressModeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			requestBody := `{
				"model":"gemini-2.5-flash",
				"messages":[{"role":"user","content":"return json"}],
				"response_format":{"type":"json_object"}
			}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)

			var transformed map[string]interface{}
			require.NoError(t, json.Unmarshal(processedBody, &transformed))

			generationConfig, ok := transformed["generationConfig"].(map[string]interface{})
			require.True(t, ok, "generationConfig should exist")
			require.Equal(t, "application/json", generationConfig["responseMimeType"], "responseMimeType should be mapped for json_object")

			_, hasSchema := generationConfig["responseSchema"]
			require.False(t, hasSchema, "json_object should not inject responseSchema")
		})

		// 测试 Vertex Express Mode structured outputs: 兼容 direct schema
		t.Run("vertex express mode structured outputs direct schema response_format mapping", func(t *testing.T) {
			host, status := test.NewTestHost(vertexExpressModeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			requestBody := `{
				"model":"gemini-2.5-flash",
				"messages":[{"role":"user","content":"return structured output"}],
				"response_format":{
					"type":"object",
					"properties":{"city":{"type":"string"}}
				}
			}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)

			var transformed map[string]interface{}
			require.NoError(t, json.Unmarshal(processedBody, &transformed))

			generationConfig, ok := transformed["generationConfig"].(map[string]interface{})
			require.True(t, ok, "generationConfig should exist")
			require.Equal(t, "application/json", generationConfig["responseMimeType"], "direct schema should be mapped to JSON mime type")

			responseSchema, ok := generationConfig["responseSchema"].(map[string]interface{})
			require.True(t, ok, "direct schema should be mapped to responseSchema")
			require.Equal(t, "object", responseSchema["type"])
		})

		// 测试 Vertex Express Mode structured outputs: 异常 json_schema 应返回错误（不能静默降级）
		t.Run("vertex express mode structured outputs malformed json_schema mapping", func(t *testing.T) {
			host, status := test.NewTestHost(vertexExpressModeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			requestBody := `{
				"model":"gemini-2.5-flash",
				"messages":[{"role":"user","content":"return structured output"}],
				"response_format":{
					"type":"json_schema",
					"json_schema":"invalid"
				}
			}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			errorLogs := host.GetErrorLogs()
			hasInvalidSchemaError := false
			for _, log := range errorLogs {
				if strings.Contains(log, "invalid response_format.json_schema") {
					hasInvalidSchemaError = true
					break
				}
			}
			require.True(t, hasInvalidSchemaError, "malformed json_schema should produce explicit validation error")

			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)
			require.Contains(t, string(processedBody), `"response_format"`, "failed request should keep original body")
			require.NotContains(t, string(processedBody), `"generationConfig"`, "failed request should not be rewritten into Vertex format")

			requestHeaders := host.GetRequestHeaders()
			pathHeader := ""
			for _, header := range requestHeaders {
				if header[0] == ":path" {
					pathHeader = header[1]
					break
				}
			}
			require.Equal(t, "/v1/chat/completions", pathHeader, "failed validation should not rewrite upstream path")
		})

		// 测试 Vertex Express Mode structured outputs: 未知类型不映射
		t.Run("vertex express mode structured outputs unknown response format type", func(t *testing.T) {
			host, status := test.NewTestHost(vertexExpressModeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			requestBody := `{
				"model":"gemini-2.5-flash",
				"messages":[{"role":"user","content":"return xml"}],
				"response_format":{"type":"xml"}
			}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)

			var transformed map[string]interface{}
			require.NoError(t, json.Unmarshal(processedBody, &transformed))

			generationConfig, ok := transformed["generationConfig"].(map[string]interface{})
			require.True(t, ok, "generationConfig should exist")
			_, hasMime := generationConfig["responseMimeType"]
			_, hasSchema := generationConfig["responseSchema"]
			require.False(t, hasMime, "unknown response_format type should not inject responseMimeType")
			require.False(t, hasSchema, "unknown response_format type should not inject responseSchema")
		})

		// 测试 Vertex Express Mode 请求体处理（嵌入接口）
		t.Run("vertex express mode embeddings request body", func(t *testing.T) {
			host, status := test.NewTestHost(vertexExpressModeConfig)
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
			requestBody := `{"model":"text-embedding-001","input":"test text"}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			require.Equal(t, types.ActionContinue, action)

			// 验证嵌入接口的请求体处理
			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)

			// 验证请求体被转换为 Vertex 格式
			require.Contains(t, string(processedBody), "instances", "Request should be converted to vertex format")

			// 验证路径包含 API Key
			requestHeaders := host.GetRequestHeaders()
			pathHeader := ""
			for _, header := range requestHeaders {
				if header[0] == ":path" {
					pathHeader = header[1]
					break
				}
			}
			require.Contains(t, pathHeader, "key=test-api-key-123456789", "Path should contain API key as query parameter")
		})

		// 测试 Vertex Express Mode 请求体处理（流式请求）
		t.Run("vertex express mode streaming request body", func(t *testing.T) {
			host, status := test.NewTestHost(vertexExpressModeConfig)
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
			requestBody := `{"model":"gemini-2.5-flash","messages":[{"role":"user","content":"test"}],"stream":true}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			require.Equal(t, types.ActionContinue, action)

			// 验证路径包含流式 action
			requestHeaders := host.GetRequestHeaders()
			pathHeader := ""
			for _, header := range requestHeaders {
				if header[0] == ":path" {
					pathHeader = header[1]
					break
				}
			}
			require.Contains(t, pathHeader, "streamGenerateContent", "Path should contain streaming action")
			require.Contains(t, pathHeader, "key=test-api-key-123456789", "Path should contain API key")
		})

		// 测试 Vertex Express Mode 请求体处理（含模型映射）
		t.Run("vertex express mode with model mapping request body", func(t *testing.T) {
			host, status := test.NewTestHost(vertexExpressModeWithModelMappingConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			// 设置请求体（使用 OpenAI 模型名）
			requestBody := `{"model":"gpt-4","messages":[{"role":"user","content":"test"}]}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			require.Equal(t, types.ActionContinue, action)

			// 验证路径包含映射后的模型名
			requestHeaders := host.GetRequestHeaders()
			pathHeader := ""
			for _, header := range requestHeaders {
				if header[0] == ":path" {
					pathHeader = header[1]
					break
				}
			}
			require.Contains(t, pathHeader, "gemini-2.5-flash", "Path should contain mapped model name")
		})
	})
}

func RunVertexExpressModeOnHttpResponseBodyTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试 Vertex Express Mode 响应体处理（聊天完成接口）
		t.Run("vertex express mode chat completion response body", func(t *testing.T) {
			host, status := test.NewTestHost(vertexExpressModeConfig)
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
			requestBody := `{"model":"gemini-2.5-flash","messages":[{"role":"user","content":"test"}]}`
			host.CallOnHttpRequestBody([]byte(requestBody))

			// 设置响应属性，确保IsResponseFromUpstream()返回true
			host.SetProperty([]string{"response", "code_details"}, []byte("via_upstream"))

			// 设置响应头
			responseHeaders := [][2]string{
				{":status", "200"},
				{"Content-Type", "application/json"},
			}
			host.CallOnHttpResponseHeaders(responseHeaders)

			// 设置响应体（Vertex 格式）
			responseBody := `{
				"candidates": [{
					"content": {
						"parts": [{
							"text": "Hello! How can I help you today?"
						}]
					},
					"finishReason": "STOP",
					"index": 0
				}],
				"usageMetadata": {
					"promptTokenCount": 9,
					"candidatesTokenCount": 12,
					"totalTokenCount": 21
				}
			}`
			action := host.CallOnHttpResponseBody([]byte(responseBody))

			require.Equal(t, types.ActionContinue, action)

			// 验证响应体是否被正确处理
			processedResponseBody := host.GetResponseBody()
			require.NotNil(t, processedResponseBody)

			// 验证响应体内容（转换为OpenAI格式）
			responseStr := string(processedResponseBody)

			// 检查响应体是否被转换
			if strings.Contains(responseStr, "chat.completion") {
				require.Contains(t, responseStr, "assistant", "Response should contain assistant role")
				require.Contains(t, responseStr, "usage", "Response should contain usage information")
			}

			// 检查是否有相关的处理日志
			debugLogs := host.GetDebugLogs()
			hasResponseBodyLogs := false
			for _, log := range debugLogs {
				if strings.Contains(log, "response") || strings.Contains(log, "body") || strings.Contains(log, "vertex") {
					hasResponseBodyLogs = true
					break
				}
			}
			require.True(t, hasResponseBodyLogs, "Should have response body processing logs")
		})
	})
}

func RunVertexOpenAICompatibleModeOnHttpRequestHeadersTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试 Vertex OpenAI 兼容模式请求头处理
		t.Run("vertex openai compatible mode request headers", func(t *testing.T) {
			host, status := test.NewTestHost(vertexOpenAICompatibleModeConfig)
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

			// 验证Host是否被改为 vertex 域名（带 region 前缀）
			require.True(t, test.HasHeaderWithValue(requestHeaders, ":authority", "us-central1-aiplatform.googleapis.com"), "Host header should be changed to vertex domain with region prefix")
		})
	})
}

func RunVertexOpenAICompatibleModeOnHttpRequestBodyTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试 Vertex OpenAI 兼容模式请求体处理（不转换格式，保持 OpenAI 格式）
		t.Run("vertex openai compatible mode request body - no format conversion", func(t *testing.T) {
			host, status := test.NewTestHost(vertexOpenAICompatibleModeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			// 设置请求体（OpenAI 格式）
			requestBody := `{"model":"gemini-2.0-flash","messages":[{"role":"user","content":"test"}]}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			// 测试环境使用伪造密钥，OAuth 获取会失败，期望 ActionContinue 并记录错误
			require.Equal(t, types.ActionContinue, action)

			// 验证请求体保持 OpenAI 格式（不转换为 Vertex 原生格式）
			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)

			// OpenAI 兼容模式应该保持 messages 字段，而不是转换为 contents
			require.Contains(t, string(processedBody), "messages", "Request should keep OpenAI format with messages field")
			require.NotContains(t, string(processedBody), "contents", "Request should NOT be converted to vertex native format")

			// 验证路径为 OpenAI 兼容端点
			requestHeaders := host.GetRequestHeaders()
			pathHeader := ""
			for _, header := range requestHeaders {
				if header[0] == ":path" {
					pathHeader = header[1]
					break
				}
			}
			require.Contains(t, pathHeader, "/v1beta1/projects/", "Path should use OpenAI compatible endpoint format")
			require.Contains(t, pathHeader, "/endpoints/openapi/chat/completions", "Path should contain openapi chat completions endpoint")
		})

		// 测试 Vertex OpenAI 兼容模式 structured outputs 请求体透传
		t.Run("vertex openai compatible mode structured outputs passthrough", func(t *testing.T) {
			host, status := test.NewTestHost(vertexOpenAICompatibleModeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			requestBody := `{
				"model":"gemini-2.0-flash",
				"messages":[{"role":"user","content":"test"}],
				"response_format":{
					"type":"json_schema",
					"json_schema":{
						"name":"demo_schema",
						"strict":true,
						"schema":{
							"type":"object",
							"properties":{"answer":{"type":"string"}},
							"required":["answer"]
						}
					}
				}
			}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)
			bodyStr := string(processedBody)

			require.Contains(t, bodyStr, `"response_format"`, "OpenAI compatible mode should preserve response_format")
			require.Contains(t, bodyStr, `"json_schema"`, "OpenAI compatible mode should preserve json_schema")
			require.NotContains(t, bodyStr, `"generationConfig"`, "OpenAI compatible mode should not convert to Vertex native generationConfig")
		})

		// 测试 Vertex OpenAI 兼容模式请求体处理（含模型映射）
		t.Run("vertex openai compatible mode with model mapping", func(t *testing.T) {
			host, status := test.NewTestHost(vertexOpenAICompatibleModeWithModelMappingConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			// 设置请求体（使用 OpenAI 模型名）
			requestBody := `{"model":"gpt-4","messages":[{"role":"user","content":"test"}]}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			require.Equal(t, types.ActionContinue, action)

			// 验证请求体中的模型名被映射
			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)

			// 模型名应该被映射为 gemini-2.0-flash
			require.Contains(t, string(processedBody), "gemini-2.0-flash", "Model name should be mapped to gemini-2.0-flash")
		})

		// 测试 Vertex OpenAI 兼容模式不支持 Embeddings API
		t.Run("vertex openai compatible mode - embeddings not supported", func(t *testing.T) {
			host, status := test.NewTestHost(vertexOpenAICompatibleModeConfig)
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
			requestBody := `{"model":"text-embedding-001","input":"test text"}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			// OpenAI 兼容模式只支持 chat completions，embeddings 应该返回错误
			require.Equal(t, types.ActionContinue, action)
		})
	})
}

func RunVertexExpressModeOnStreamingResponseBodyTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试 Vertex Express Mode 流式响应处理：最后一个 chunk 不应丢失
		t.Run("vertex express mode streaming response body", func(t *testing.T) {
			host, status := test.NewTestHost(vertexExpressModeConfig)
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
			requestBody := `{"model":"gemini-2.5-flash","messages":[{"role":"user","content":"test"}],"stream":true}`
			host.CallOnHttpRequestBody([]byte(requestBody))

			// 设置响应属性，确保IsResponseFromUpstream()返回true
			host.SetProperty([]string{"response", "code_details"}, []byte("via_upstream"))

			// 设置流式响应头
			responseHeaders := [][2]string{
				{":status", "200"},
				{"Content-Type", "text/event-stream"},
			}
			host.CallOnHttpResponseHeaders(responseHeaders)

			// 模拟流式响应体
			chunk1 := "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"Hello\"}],\"role\":\"model\"},\"finishReason\":\"\",\"index\":0}],\"usageMetadata\":{\"promptTokenCount\":9,\"candidatesTokenCount\":5,\"totalTokenCount\":14}}\n\n"
			chunk2 := "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"Hello! How can I help you today?\"}],\"role\":\"model\"},\"finishReason\":\"STOP\",\"index\":0}],\"usageMetadata\":{\"promptTokenCount\":9,\"candidatesTokenCount\":12,\"totalTokenCount\":21}}\n\n"

			// 处理流式响应体
			action1 := host.CallOnHttpStreamingResponseBody([]byte(chunk1), false)
			require.Equal(t, types.ActionContinue, action1)

			action2 := host.CallOnHttpStreamingResponseBody([]byte(chunk2), true)
			require.Equal(t, types.ActionContinue, action2)

			// 验证最后一个 chunk 的内容不会被 [DONE] 覆盖
			transformedResponseBody := host.GetResponseBody()
			require.NotNil(t, transformedResponseBody)
			responseStr := string(transformedResponseBody)
			require.Contains(t, responseStr, "Hello! How can I help you today?", "last chunk content should be preserved")
			require.Contains(t, responseStr, "data: [DONE]", "stream should end with [DONE]")
		})

		// 测试 Vertex Express Mode 流式响应处理：单个 SSE 事件被拆包时可正确重组
		t.Run("vertex express mode streaming response body with split sse event", func(t *testing.T) {
			host, status := test.NewTestHost(vertexExpressModeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			requestBody := `{"model":"gemini-2.5-flash","messages":[{"role":"user","content":"test"}],"stream":true}`
			host.CallOnHttpRequestBody([]byte(requestBody))

			// 设置响应属性，确保IsResponseFromUpstream()返回true
			host.SetProperty([]string{"response", "code_details"}, []byte("via_upstream"))

			responseHeaders := [][2]string{
				{":status", "200"},
				{"Content-Type", "text/event-stream"},
			}
			host.CallOnHttpResponseHeaders(responseHeaders)

			fullEvent := "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"split chunk\"}],\"role\":\"model\"},\"finishReason\":\"STOP\",\"index\":0}],\"usageMetadata\":{\"promptTokenCount\":1,\"candidatesTokenCount\":2,\"totalTokenCount\":3}}\n\n"
			splitIdx := strings.Index(fullEvent, "chunk")
			require.Greater(t, splitIdx, 0, "split marker should exist in test payload")
			chunkPart1 := fullEvent[:splitIdx]
			chunkPart2 := fullEvent[splitIdx:]

			action1 := host.CallOnHttpStreamingResponseBody([]byte(chunkPart1), false)
			require.Equal(t, types.ActionContinue, action1)
			action2 := host.CallOnHttpStreamingResponseBody([]byte(chunkPart2), true)
			require.Equal(t, types.ActionContinue, action2)

			transformedResponseBody := host.GetResponseBody()
			require.NotNil(t, transformedResponseBody)
			responseStr := string(transformedResponseBody)
			require.Contains(t, responseStr, "split chunk", "split SSE event should be reassembled and parsed")
			require.Contains(t, responseStr, "data: [DONE]", "stream should end with [DONE]")
		})

		// 测试：thoughtSignature 很大时，单个 SSE 事件被拆成多段也能重组并成功解析
		t.Run("vertex express mode streaming response body with huge thought signature split across chunks", func(t *testing.T) {
			host, status := test.NewTestHost(vertexExpressModeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			requestBody := `{"model":"gemini-2.5-flash","messages":[{"role":"user","content":"test"}],"stream":true}`
			host.CallOnHttpRequestBody([]byte(requestBody))
			host.SetProperty([]string{"response", "code_details"}, []byte("via_upstream"))
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"Content-Type", "text/event-stream"},
			})

			hugeThoughtSignature := strings.Repeat("CmMBjz1rX4j+TQjtDy2rZxSdYOE1jUqDbRhWetraLlQNrkyaRNQZ/", 180)
			fullEvent := "data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"thought-signature-merge-ok\",\"thoughtSignature\":\"" +
				hugeThoughtSignature +
				"\"}]},\"finishReason\":\"STOP\",\"index\":0}],\"usageMetadata\":{\"promptTokenCount\":28,\"candidatesTokenCount\":3589,\"totalTokenCount\":5240,\"thoughtsTokenCount\":1623}}\n\n"

			signatureStart := strings.Index(fullEvent, "\"thoughtSignature\":\"")
			require.Greater(t, signatureStart, 0, "thoughtSignature field should exist in test payload")
			splitAt1 := signatureStart + len("\"thoughtSignature\":\"") + 700
			splitAt2 := splitAt1 + 1600
			require.Less(t, splitAt2, len(fullEvent)-1, "split indexes should keep payload in three chunks")

			chunkPart1 := fullEvent[:splitAt1]
			chunkPart2 := fullEvent[splitAt1:splitAt2]
			chunkPart3 := fullEvent[splitAt2:]

			action1 := host.CallOnHttpStreamingResponseBody([]byte(chunkPart1), false)
			require.Equal(t, types.ActionContinue, action1)
			firstBody := host.GetResponseBody()
			require.Equal(t, 0, len(firstBody), "partial chunk should not be forwarded to client")

			action2 := host.CallOnHttpStreamingResponseBody([]byte(chunkPart2), false)
			require.Equal(t, types.ActionContinue, action2)
			secondBody := host.GetResponseBody()
			require.Equal(t, 0, len(secondBody), "partial chunk should not be forwarded to client")

			action3 := host.CallOnHttpStreamingResponseBody([]byte(chunkPart3), true)
			require.Equal(t, types.ActionContinue, action3)

			transformedResponseBody := host.GetResponseBody()
			require.NotNil(t, transformedResponseBody)
			responseStr := string(transformedResponseBody)
			require.Contains(t, responseStr, "thought-signature-merge-ok", "split huge thoughtSignature event should be reassembled and parsed")
			require.Contains(t, responseStr, "data: [DONE]", "stream should end with [DONE]")

			errorLogs := host.GetErrorLogs()
			hasUnmarshalError := false
			for _, log := range errorLogs {
				if strings.Contains(log, "unable to unmarshal vertex response") {
					hasUnmarshalError = true
					break
				}
			}
			require.False(t, hasUnmarshalError, "should not have vertex unmarshal errors for split huge thoughtSignature event")
		})

		// 测试：上游已发送 [DONE]，框架再触发空的最后回调时不应重复输出 [DONE]
		t.Run("vertex express mode streaming response body with upstream done and empty final callback", func(t *testing.T) {
			host, status := test.NewTestHost(vertexExpressModeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			requestBody := `{"model":"gemini-2.5-flash","messages":[{"role":"user","content":"test"}],"stream":true}`
			host.CallOnHttpRequestBody([]byte(requestBody))
			host.SetProperty([]string{"response", "code_details"}, []byte("via_upstream"))
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"Content-Type", "text/event-stream"},
			})

			doneChunk := "data: [DONE]\n\n"
			action1 := host.CallOnHttpStreamingResponseBody([]byte(doneChunk), false)
			require.Equal(t, types.ActionContinue, action1)
			firstBody := host.GetResponseBody()
			require.NotNil(t, firstBody)
			require.Contains(t, string(firstBody), "data: [DONE]", "first callback should output [DONE]")

			action2 := host.CallOnHttpStreamingResponseBody([]byte{}, true)
			require.Equal(t, types.ActionContinue, action2)

			debugLogs := host.GetDebugLogs()
			doneChunkLogCount := 0
			for _, log := range debugLogs {
				if strings.Contains(log, "=== modified response chunk: data: [DONE]") {
					doneChunkLogCount++
				}
			}
			require.Equal(t, 1, doneChunkLogCount, "[DONE] should only be emitted once when upstream already sent it")
		})

		// 测试：最后一个 chunk 缺少 SSE 结束空行时，isLastChunk=true 也应正确解析并输出
		t.Run("vertex express mode streaming response body last chunk without terminator", func(t *testing.T) {
			host, status := test.NewTestHost(vertexExpressModeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			requestBody := `{"model":"gemini-2.5-flash","messages":[{"role":"user","content":"test"}],"stream":true}`
			host.CallOnHttpRequestBody([]byte(requestBody))
			host.SetProperty([]string{"response", "code_details"}, []byte("via_upstream"))
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"Content-Type", "text/event-stream"},
			})

			lastChunkWithoutTerminator := "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"no terminator\"}],\"role\":\"model\"},\"finishReason\":\"STOP\",\"index\":0}],\"usageMetadata\":{\"promptTokenCount\":2,\"candidatesTokenCount\":3,\"totalTokenCount\":5}}"
			action := host.CallOnHttpStreamingResponseBody([]byte(lastChunkWithoutTerminator), true)
			require.Equal(t, types.ActionContinue, action)

			transformedResponseBody := host.GetResponseBody()
			require.NotNil(t, transformedResponseBody)
			responseStr := string(transformedResponseBody)
			require.Contains(t, responseStr, "no terminator", "last chunk without terminator should still be parsed")
			require.Contains(t, responseStr, "data: [DONE]", "stream should end with [DONE]")
		})
	})
}

func RunVertexOpenAICompatibleModeOnHttpResponseBodyTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试 Vertex OpenAI 兼容模式响应体处理（直接透传，不转换格式）
		t.Run("vertex openai compatible mode response body - passthrough", func(t *testing.T) {
			host, status := test.NewTestHost(vertexOpenAICompatibleModeConfig)
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
			requestBody := `{"model":"gemini-2.0-flash","messages":[{"role":"user","content":"test"}]}`
			host.CallOnHttpRequestBody([]byte(requestBody))

			// 设置响应属性，确保IsResponseFromUpstream()返回true
			host.SetProperty([]string{"response", "code_details"}, []byte("via_upstream"))

			// 设置响应头
			responseHeaders := [][2]string{
				{":status", "200"},
				{"Content-Type", "application/json"},
			}
			host.CallOnHttpResponseHeaders(responseHeaders)

			// 设置响应体（OpenAI 格式 - 因为 Vertex AI OpenAI-compatible API 返回的就是 OpenAI 格式）
			responseBody := `{
				"id": "chatcmpl-abc123",
				"choices": [{
					"index": 0,
					"message": {
						"role": "assistant",
						"content": "Hello! How can I help you today?"
					},
					"finish_reason": "stop"
				}],
				"created": 1729986750,
				"model": "gemini-2.0-flash",
				"object": "chat.completion",
				"usage": {
					"prompt_tokens": 9,
					"completion_tokens": 12,
					"total_tokens": 21
				}
			}`
			action := host.CallOnHttpResponseBody([]byte(responseBody))

			require.Equal(t, types.ActionContinue, action)

			// 验证响应体被直接透传（不进行格式转换）
			processedResponseBody := host.GetResponseBody()
			require.NotNil(t, processedResponseBody)

			// 响应应该保持原样
			responseStr := string(processedResponseBody)
			require.Contains(t, responseStr, "chatcmpl-abc123", "Response should be passed through unchanged")
			require.Contains(t, responseStr, "chat.completion", "Response should contain original object type")
		})

		// 测试 Vertex OpenAI 兼容模式流式响应处理（直接透传）
		t.Run("vertex openai compatible mode streaming response - passthrough", func(t *testing.T) {
			host, status := test.NewTestHost(vertexOpenAICompatibleModeConfig)
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
			requestBody := `{"model":"gemini-2.0-flash","messages":[{"role":"user","content":"test"}],"stream":true}`
			host.CallOnHttpRequestBody([]byte(requestBody))

			// 设置流式响应头
			responseHeaders := [][2]string{
				{":status", "200"},
				{"Content-Type", "text/event-stream"},
			}
			host.CallOnHttpResponseHeaders(responseHeaders)

			// 模拟 OpenAI 格式的流式响应（Vertex AI OpenAI-compatible API 返回）
			chunk1 := `data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","created":1729986750,"model":"gemini-2.0-flash","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"},"finish_reason":null}]}`
			chunk2 := `data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","created":1729986750,"model":"gemini-2.0-flash","choices":[{"index":0,"delta":{"content":"!"},"finish_reason":"stop"}]}`

			// 处理流式响应体 - 应该直接透传
			action1 := host.CallOnHttpStreamingResponseBody([]byte(chunk1), false)
			require.Equal(t, types.ActionContinue, action1)

			action2 := host.CallOnHttpStreamingResponseBody([]byte(chunk2), true)
			require.Equal(t, types.ActionContinue, action2)
		})

		// 测试 Vertex OpenAI 兼容模式流式响应处理（Unicode 转义解码）
		t.Run("vertex openai compatible mode streaming response - unicode escape decoding", func(t *testing.T) {
			host, status := test.NewTestHost(vertexOpenAICompatibleModeConfig)
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
			requestBody := `{"model":"gemini-2.0-flash","messages":[{"role":"user","content":"test"}],"stream":true}`
			host.CallOnHttpRequestBody([]byte(requestBody))

			// 设置流式响应头
			responseHeaders := [][2]string{
				{":status", "200"},
				{"Content-Type", "text/event-stream"},
			}
			host.CallOnHttpResponseHeaders(responseHeaders)

			// 模拟带有 Unicode 转义的流式响应（Vertex AI OpenAI-compatible API 可能返回的格式）
			// \u4e2d\u6587 = 中文
			chunkWithUnicode := `data: {"id":"chatcmpl-abc123","object":"chat.completion.chunk","created":1729986750,"model":"gemini-2.0-flash","choices":[{"index":0,"delta":{"role":"assistant","content":"\u4e2d\u6587\u6d4b\u8bd5"},"finish_reason":null}]}`

			// 处理流式响应体 - 应该解码 Unicode 转义
			action := host.CallOnHttpStreamingResponseBody([]byte(chunkWithUnicode), false)
			require.Equal(t, types.ActionContinue, action)

			// 验证响应体中的 Unicode 转义已被解码
			responseBody := host.GetResponseBody()
			require.NotNil(t, responseBody)

			responseStr := string(responseBody)
			// 应该包含解码后的中文字符，而不是 \uXXXX 转义序列
			require.Contains(t, responseStr, "中文测试", "Unicode escapes should be decoded to Chinese characters")
			require.NotContains(t, responseStr, `\u4e2d`, "Should not contain Unicode escape sequences")
		})

		// 测试 Vertex OpenAI 兼容模式非流式响应处理（Unicode 转义解码）
		t.Run("vertex openai compatible mode response body - unicode escape decoding", func(t *testing.T) {
			host, status := test.NewTestHost(vertexOpenAICompatibleModeConfig)
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
			requestBody := `{"model":"gemini-2.0-flash","messages":[{"role":"user","content":"test"}]}`
			host.CallOnHttpRequestBody([]byte(requestBody))

			// 设置响应头
			responseHeaders := [][2]string{
				{":status", "200"},
				{"Content-Type", "application/json"},
			}
			host.CallOnHttpResponseHeaders(responseHeaders)

			// 模拟带有 Unicode 转义的响应体
			// \u76c8\u5229\u80fd\u529b = 盈利能力
			responseBodyWithUnicode := `{"id":"chatcmpl-abc123","object":"chat.completion","created":1729986750,"model":"gemini-2.0-flash","choices":[{"index":0,"message":{"role":"assistant","content":"\u76c8\u5229\u80fd\u529b\u5206\u6790"},"finish_reason":"stop"}]}`

			// 处理响应体 - 应该解码 Unicode 转义
			action := host.CallOnHttpResponseBody([]byte(responseBodyWithUnicode))
			require.Equal(t, types.ActionContinue, action)

			// 验证响应体中的 Unicode 转义已被解码
			processedResponseBody := host.GetResponseBody()
			require.NotNil(t, processedResponseBody)

			responseStr := string(processedResponseBody)
			// 应该包含解码后的中文字符
			require.Contains(t, responseStr, "盈利能力分析", "Unicode escapes should be decoded to Chinese characters")
			require.NotContains(t, responseStr, `\u76c8`, "Should not contain Unicode escape sequences")
		})
	})
}

// ==================== 图片生成测试 ====================

func RunVertexExpressModeImageGenerationRequestBodyTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试 Vertex Express Mode 图片生成请求体处理
		t.Run("vertex express mode image generation request body", func(t *testing.T) {
			host, status := test.NewTestHost(vertexExpressModeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/images/generations"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			// 设置请求体（OpenAI 图片生成格式）
			requestBody := `{"model":"gemini-2.0-flash-exp","prompt":"A cute orange cat napping in the sunshine","size":"1024x1024"}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			// Express Mode 不需要暂停等待 OAuth token
			require.Equal(t, types.ActionContinue, action)

			// 验证请求体是否被正确处理
			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)

			// 验证请求体被转换为 Vertex 格式
			bodyStr := string(processedBody)
			require.Contains(t, bodyStr, "contents", "Request should be converted to vertex format with contents")
			require.Contains(t, bodyStr, "generationConfig", "Request should contain generationConfig")
			require.Contains(t, bodyStr, "responseModalities", "Request should contain responseModalities for image generation")
			require.Contains(t, bodyStr, "IMAGE", "Request should specify IMAGE in responseModalities")
			require.Contains(t, bodyStr, "imageConfig", "Request should contain imageConfig")

			// 验证路径包含 API Key 和正确的模型
			requestHeaders := host.GetRequestHeaders()
			pathHeader := ""
			for _, header := range requestHeaders {
				if header[0] == ":path" {
					pathHeader = header[1]
					break
				}
			}
			require.Contains(t, pathHeader, "key=test-api-key-123456789", "Path should contain API key as query parameter")
			require.Contains(t, pathHeader, "/v1/publishers/google/models/", "Path should use Express Mode format")
			require.Contains(t, pathHeader, "generateContent", "Path should use generateContent action for image generation")
			require.NotContains(t, pathHeader, "streamGenerateContent", "Path should NOT use streaming for image generation")
		})

		// 测试 Vertex Express Mode 图片生成请求体处理（自定义尺寸）
		t.Run("vertex express mode image generation with custom size", func(t *testing.T) {
			host, status := test.NewTestHost(vertexExpressModeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/images/generations"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			// 设置请求体（宽屏尺寸）
			requestBody := `{"model":"gemini-2.0-flash-exp","prompt":"A beautiful sunset over the ocean","size":"1792x1024"}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			require.Equal(t, types.ActionContinue, action)

			// 验证请求体是否正确处理尺寸映射
			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)

			bodyStr := string(processedBody)
			// 1792x1024 应该映射为 16:9 宽高比
			require.Contains(t, bodyStr, "aspectRatio", "Request should contain aspectRatio in imageConfig")
			require.Contains(t, bodyStr, "16:9", "Request should map 1792x1024 to 16:9 aspect ratio")
		})

		// 测试 Vertex Express Mode 图片生成请求体处理（含安全设置）
		t.Run("vertex express mode image generation with safety settings", func(t *testing.T) {
			host, status := test.NewTestHost(vertexExpressModeWithSafetyConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/images/generations"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			// 设置请求体
			requestBody := `{"model":"gemini-2.0-flash-exp","prompt":"A mountain landscape"}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			require.Equal(t, types.ActionContinue, action)

			// 验证请求体包含安全设置
			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)

			bodyStr := string(processedBody)
			require.Contains(t, bodyStr, "safetySettings", "Request should contain safetySettings")
		})

		// 测试 Vertex Express Mode 图片生成请求体处理（含模型映射）
		t.Run("vertex express mode image generation with model mapping", func(t *testing.T) {
			host, status := test.NewTestHost(vertexExpressModeWithModelMappingConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/images/generations"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			// 设置请求体（使用映射前的模型名称）
			requestBody := `{"model":"gpt-4","prompt":"A futuristic city"}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			require.Equal(t, types.ActionContinue, action)

			// 验证路径中使用了映射后的模型名称
			requestHeaders := host.GetRequestHeaders()
			pathHeader := ""
			for _, header := range requestHeaders {
				if header[0] == ":path" {
					pathHeader = header[1]
					break
				}
			}
			require.Contains(t, pathHeader, "gemini-2.5-flash", "Path should contain mapped model name")
		})
	})
}

func RunVertexExpressModeImageGenerationResponseBodyTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试 Vertex Express Mode 图片生成响应体处理
		t.Run("vertex express mode image generation response body", func(t *testing.T) {
			host, status := test.NewTestHost(vertexExpressModeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/images/generations"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			// 设置请求体
			requestBody := `{"model":"gemini-2.0-flash-exp","prompt":"A cute cat"}`
			host.CallOnHttpRequestBody([]byte(requestBody))

			// 设置响应属性，确保IsResponseFromUpstream()返回true
			host.SetProperty([]string{"response", "code_details"}, []byte("via_upstream"))

			// 设置响应头
			responseHeaders := [][2]string{
				{":status", "200"},
				{"Content-Type", "application/json"},
			}
			host.CallOnHttpResponseHeaders(responseHeaders)

			// 设置响应体（Vertex 图片生成格式）
			responseBody := `{
				"candidates": [{
					"content": {
						"role": "model",
						"parts": [{
							"inlineData": {
								"mimeType": "image/png",
								"data": "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
							}
						}]
					},
					"finishReason": "STOP"
				}],
				"usageMetadata": {
					"promptTokenCount": 10,
					"candidatesTokenCount": 1024,
					"totalTokenCount": 1034
				}
			}`
			action := host.CallOnHttpResponseBody([]byte(responseBody))

			require.Equal(t, types.ActionContinue, action)

			// 验证响应体是否被正确处理
			processedResponseBody := host.GetResponseBody()
			require.NotNil(t, processedResponseBody)

			responseStr := string(processedResponseBody)

			// 验证响应体被转换为 OpenAI 图片生成格式
			require.Contains(t, responseStr, "created", "Response should contain created field")
			require.Contains(t, responseStr, "data", "Response should contain data array")
			require.Contains(t, responseStr, "b64_json", "Response should contain b64_json field with base64 image data")
			require.Contains(t, responseStr, "usage", "Response should contain usage information")
			require.Contains(t, responseStr, "total_tokens", "Response should contain total_tokens in usage")
		})

		// 测试 Vertex Express Mode 图片生成响应体处理（跳过思考过程）
		t.Run("vertex express mode image generation response body - skip thinking", func(t *testing.T) {
			host, status := test.NewTestHost(vertexExpressModeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/images/generations"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			// 设置请求体
			requestBody := `{"model":"gemini-3-pro-image-preview","prompt":"An Eiffel tower"}`
			host.CallOnHttpRequestBody([]byte(requestBody))

			// 设置响应属性
			host.SetProperty([]string{"response", "code_details"}, []byte("via_upstream"))

			// 设置响应头
			responseHeaders := [][2]string{
				{":status", "200"},
				{"Content-Type", "application/json"},
			}
			host.CallOnHttpResponseHeaders(responseHeaders)

			// 设置响应体（包含思考过程和图片）
			responseBody := `{
				"candidates": [{
					"content": {
						"role": "model",
						"parts": [
							{
								"text": "Considering visual elements...",
								"thought": true
							},
							{
								"inlineData": {
									"mimeType": "image/png",
									"data": "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
								}
							}
						]
					},
					"finishReason": "STOP"
				}],
				"usageMetadata": {
					"promptTokenCount": 13,
					"candidatesTokenCount": 1120,
					"totalTokenCount": 1356,
					"thoughtsTokenCount": 223
				}
			}`
			action := host.CallOnHttpResponseBody([]byte(responseBody))

			require.Equal(t, types.ActionContinue, action)

			// 验证响应体是否被正确处理
			processedResponseBody := host.GetResponseBody()
			require.NotNil(t, processedResponseBody)

			responseStr := string(processedResponseBody)

			// 验证响应体只包含图片数据，不包含思考过程文本
			require.Contains(t, responseStr, "b64_json", "Response should contain b64_json field")
			require.NotContains(t, responseStr, "Considering visual elements", "Response should NOT contain thinking text")
			require.NotContains(t, responseStr, "thought", "Response should NOT contain thought field")
		})

		// 测试 Vertex Express Mode 图片生成响应体处理（空图片数据）
		t.Run("vertex express mode image generation response body - no image", func(t *testing.T) {
			host, status := test.NewTestHost(vertexExpressModeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/images/generations"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			// 设置请求体
			requestBody := `{"model":"gemini-2.0-flash-exp","prompt":"test"}`
			host.CallOnHttpRequestBody([]byte(requestBody))

			// 设置响应属性
			host.SetProperty([]string{"response", "code_details"}, []byte("via_upstream"))

			// 设置响应头
			responseHeaders := [][2]string{
				{":status", "200"},
				{"Content-Type", "application/json"},
			}
			host.CallOnHttpResponseHeaders(responseHeaders)

			// 设置响应体（只有文本，没有图片）
			responseBody := `{
				"candidates": [{
					"content": {
						"role": "model",
						"parts": [{
							"text": "I cannot generate that image."
						}]
					},
					"finishReason": "SAFETY"
				}],
				"usageMetadata": {
					"promptTokenCount": 5,
					"candidatesTokenCount": 10,
					"totalTokenCount": 15
				}
			}`
			action := host.CallOnHttpResponseBody([]byte(responseBody))

			require.Equal(t, types.ActionContinue, action)

			// 验证响应体是否被正确处理（即使没有图片）
			processedResponseBody := host.GetResponseBody()
			require.NotNil(t, processedResponseBody)

			responseStr := string(processedResponseBody)

			// 验证响应体结构正确，data 数组为空
			require.Contains(t, responseStr, "created", "Response should contain created field")
			require.Contains(t, responseStr, "data", "Response should contain data array")
		})
	})
}

func buildMultipartRequestBody(t *testing.T, fields map[string]string, files map[string][]byte) ([]byte, string) {
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)

	for key, value := range fields {
		require.NoError(t, writer.WriteField(key, value))
	}

	for fieldName, data := range files {
		part, err := writer.CreateFormFile(fieldName, "upload-image.png")
		require.NoError(t, err)
		_, err = part.Write(data)
		require.NoError(t, err)
	}

	require.NoError(t, writer.Close())
	return buffer.Bytes(), writer.FormDataContentType()
}

func RunVertexExpressModeImageEditVariationRequestBodyTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		const testDataURL = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

		t.Run("vertex express mode image edit request body with image_url", func(t *testing.T) {
			host, status := test.NewTestHost(vertexExpressModeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/images/edits"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			requestBody := `{"model":"gemini-2.0-flash-exp","prompt":"Add sunglasses to the cat","image":{"image_url":{"url":"` + testDataURL + `"}},"size":"1024x1024"}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)

			bodyStr := string(processedBody)
			require.Contains(t, bodyStr, "inlineData", "Request should contain inlineData converted from image_url")
			require.Contains(t, bodyStr, "Add sunglasses to the cat", "Prompt text should be preserved")
			require.NotContains(t, bodyStr, "image_url", "OpenAI image_url field should be converted to Vertex format")

			requestHeaders := host.GetRequestHeaders()
			pathHeader := ""
			for _, header := range requestHeaders {
				if header[0] == ":path" {
					pathHeader = header[1]
					break
				}
			}
			require.Contains(t, pathHeader, "generateContent", "Image edit should use generateContent action")
			require.Contains(t, pathHeader, "key=test-api-key-123456789", "Path should contain API key")
		})

		t.Run("vertex express mode image edit request body with image string", func(t *testing.T) {
			host, status := test.NewTestHost(vertexExpressModeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/images/edits"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			requestBody := `{"model":"gemini-2.0-flash-exp","prompt":"Add sunglasses to the cat","image":"` + testDataURL + `","size":"1024x1024"}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)

			bodyStr := string(processedBody)
			require.Contains(t, bodyStr, "inlineData", "Request should contain inlineData converted from image string")
			require.Contains(t, bodyStr, "Add sunglasses to the cat", "Prompt text should be preserved")
		})

		t.Run("vertex express mode image edit multipart request body", func(t *testing.T) {
			host, status := test.NewTestHost(vertexExpressModeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			body, contentType := buildMultipartRequestBody(t, map[string]string{
				"model":  "gemini-2.0-flash-exp",
				"prompt": "Add sunglasses to the cat",
				"size":   "1024x1024",
			}, map[string][]byte{
				"image": []byte("fake-image-content"),
			})

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/images/edits"},
				{":method", "POST"},
				{"Content-Type", contentType},
			})

			action := host.CallOnHttpRequestBody(body)
			require.Equal(t, types.ActionContinue, action)

			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)
			bodyStr := string(processedBody)
			require.Contains(t, bodyStr, "inlineData", "Multipart image should be converted to inlineData")
			require.Contains(t, bodyStr, "Add sunglasses to the cat", "Prompt text should be preserved")

			requestHeaders := host.GetRequestHeaders()
			require.True(t, test.HasHeaderWithValue(requestHeaders, "Content-Type", "application/json"), "Content-Type should be rewritten to application/json")
		})

		t.Run("vertex express mode image variation multipart request body", func(t *testing.T) {
			host, status := test.NewTestHost(vertexExpressModeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			body, contentType := buildMultipartRequestBody(t, map[string]string{
				"model": "gemini-2.0-flash-exp",
				"size":  "1024x1024",
			}, map[string][]byte{
				"image": []byte("fake-image-content"),
			})

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/images/variations"},
				{":method", "POST"},
				{"Content-Type", contentType},
			})

			action := host.CallOnHttpRequestBody(body)
			require.Equal(t, types.ActionContinue, action)

			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)
			bodyStr := string(processedBody)
			require.Contains(t, bodyStr, "inlineData", "Multipart image should be converted to inlineData")
			require.Contains(t, bodyStr, "Create variations of the provided image.", "Variation request should inject a default prompt")

			requestHeaders := host.GetRequestHeaders()
			require.True(t, test.HasHeaderWithValue(requestHeaders, "Content-Type", "application/json"), "Content-Type should be rewritten to application/json")
		})

		t.Run("vertex express mode image edit with model mapping", func(t *testing.T) {
			host, status := test.NewTestHost(vertexExpressModeWithModelMappingConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/images/edits"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			requestBody := `{"model":"gpt-4","prompt":"Turn it into watercolor","image_url":{"url":"` + testDataURL + `"}}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			requestHeaders := host.GetRequestHeaders()
			pathHeader := ""
			for _, header := range requestHeaders {
				if header[0] == ":path" {
					pathHeader = header[1]
					break
				}
			}
			require.Contains(t, pathHeader, "gemini-2.5-flash", "Path should contain mapped model name")
		})

		t.Run("vertex express mode image variation request body with image_url", func(t *testing.T) {
			host, status := test.NewTestHost(vertexExpressModeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/images/variations"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			requestBody := `{"model":"gemini-2.0-flash-exp","image_url":{"url":"` + testDataURL + `"}}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)

			bodyStr := string(processedBody)
			require.Contains(t, bodyStr, "inlineData", "Request should contain inlineData converted from image_url")
			require.Contains(t, bodyStr, "Create variations of the provided image.", "Variation request should inject a default prompt")

			requestHeaders := host.GetRequestHeaders()
			pathHeader := ""
			for _, header := range requestHeaders {
				if header[0] == ":path" {
					pathHeader = header[1]
					break
				}
			}
			require.Contains(t, pathHeader, "generateContent", "Image variation should use generateContent action")
			require.Contains(t, pathHeader, "key=test-api-key-123456789", "Path should contain API key")
		})
	})
}

func RunVertexExpressModeImageEditVariationResponseBodyTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		const testDataURL = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

		t.Run("vertex express mode image edit response body", func(t *testing.T) {
			host, status := test.NewTestHost(vertexExpressModeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/images/edits"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			requestBody := `{"model":"gemini-2.0-flash-exp","prompt":"Add glasses","image_url":{"url":"` + testDataURL + `"}}`
			host.CallOnHttpRequestBody([]byte(requestBody))

			host.SetProperty([]string{"response", "code_details"}, []byte("via_upstream"))
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"Content-Type", "application/json"},
			})

			responseBody := `{
				"candidates": [{
					"content": {
						"role": "model",
						"parts": [{
							"inlineData": {
								"mimeType": "image/png",
								"data": "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
							}
						}]
					}
				}],
				"usageMetadata": {
					"promptTokenCount": 12,
					"candidatesTokenCount": 1024,
					"totalTokenCount": 1036
				}
			}`
			action := host.CallOnHttpResponseBody([]byte(responseBody))
			require.Equal(t, types.ActionContinue, action)

			processedResponseBody := host.GetResponseBody()
			require.NotNil(t, processedResponseBody)

			responseStr := string(processedResponseBody)
			require.Contains(t, responseStr, "b64_json", "Response should contain b64_json field")
			require.Contains(t, responseStr, "usage", "Response should contain usage field")
		})

		t.Run("vertex express mode image variation response body", func(t *testing.T) {
			host, status := test.NewTestHost(vertexExpressModeConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/images/variations"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			requestBody := `{"model":"gemini-2.0-flash-exp","image_url":{"url":"` + testDataURL + `"}}`
			host.CallOnHttpRequestBody([]byte(requestBody))

			host.SetProperty([]string{"response", "code_details"}, []byte("via_upstream"))
			host.CallOnHttpResponseHeaders([][2]string{
				{":status", "200"},
				{"Content-Type", "application/json"},
			})

			responseBody := `{
				"candidates": [{
					"content": {
						"role": "model",
						"parts": [{
							"inlineData": {
								"mimeType": "image/png",
								"data": "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
							}
						}]
					}
				}],
				"usageMetadata": {
					"promptTokenCount": 8,
					"candidatesTokenCount": 768,
					"totalTokenCount": 776
				}
			}`
			action := host.CallOnHttpResponseBody([]byte(responseBody))
			require.Equal(t, types.ActionContinue, action)

			processedResponseBody := host.GetResponseBody()
			require.NotNil(t, processedResponseBody)

			responseStr := string(processedResponseBody)
			require.Contains(t, responseStr, "b64_json", "Response should contain b64_json field")
			require.Contains(t, responseStr, "usage", "Response should contain usage field")
		})
	})
}

// ==================== Vertex Raw 模式测试 ====================

func RunVertexRawModeOnHttpRequestHeadersTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试 Vertex Raw 模式请求头处理（Express Mode + 原生 Vertex API 路径）
		t.Run("vertex raw mode express - request headers with native vertex path", func(t *testing.T) {
			host, status := test.NewTestHost(vertexRawModeExpressConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 使用原生 Vertex AI REST API 路径
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/projects/test-project/locations/us-central1/publishers/google/models/gemini-2.0-flash:generateContent"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			// 应该返回 HeaderStopIteration，因为需要处理请求体
			require.Equal(t, types.HeaderStopIteration, action)

			// 验证请求头是否被正确处理
			requestHeaders := host.GetRequestHeaders()
			require.NotNil(t, requestHeaders)

			// 验证 Host 是否被改为 vertex 域名（Express Mode 使用不带 region 前缀的域名）
			require.True(t, test.HasHeaderWithValue(requestHeaders, ":authority", "aiplatform.googleapis.com"),
				"Host header should be changed to vertex domain without region prefix")
		})

		// 测试 Vertex Raw 模式请求头处理（标准模式 + 原生 Vertex API 路径）
		t.Run("vertex raw mode standard - request headers with native vertex path", func(t *testing.T) {
			host, status := test.NewTestHost(vertexRawModeStandardConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 使用原生 Vertex AI REST API 路径
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/projects/test-project/locations/us-central1/publishers/google/models/gemini-2.0-flash:generateContent"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			require.Equal(t, types.HeaderStopIteration, action)

			// 验证请求头是否被正确处理
			requestHeaders := host.GetRequestHeaders()
			require.NotNil(t, requestHeaders)

			// 验证 Host 是否被改为 vertex 域名（标准模式使用带 region 前缀的域名）
			require.True(t, test.HasHeaderWithValue(requestHeaders, ":authority", "us-central1-aiplatform.googleapis.com"),
				"Host header should be changed to vertex domain with region prefix")
		})

		// 测试 Vertex Raw 模式请求头处理（带 basePath 前缀）
		t.Run("vertex raw mode with basePath - request headers", func(t *testing.T) {
			host, status := test.NewTestHost(vertexRawModeWithBasePathConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 使用带 basePath 前缀的原生 Vertex AI REST API 路径
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/vertex-proxy/v1/projects/test-project/locations/us-central1/publishers/google/models/imagen-4.0-generate-preview-06-06:predict"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			require.Equal(t, types.HeaderStopIteration, action)

			// 验证请求头是否被正确处理
			requestHeaders := host.GetRequestHeaders()
			require.NotNil(t, requestHeaders)

			// 验证 Host 是否被改为 vertex 域名
			require.True(t, test.HasHeaderWithValue(requestHeaders, ":authority", "aiplatform.googleapis.com"),
				"Host header should be changed to vertex domain")

			// 验证路径是否移除了 basePath 前缀
			pathHeader := ""
			for _, header := range requestHeaders {
				if header[0] == ":path" {
					pathHeader = header[1]
					break
				}
			}
			require.NotContains(t, pathHeader, "/vertex-proxy", "Path should have basePath prefix removed")
			require.Contains(t, pathHeader, "/v1/projects/", "Path should contain original vertex path after basePath removal")
		})

		// 测试 Vertex Raw 模式请求头处理（Anthropic 模型路径）
		t.Run("vertex raw mode express - request headers with anthropic model path", func(t *testing.T) {
			host, status := test.NewTestHost(vertexRawModeExpressConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 使用 Anthropic 模型的原生 Vertex AI REST API 路径
			action := host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/projects/test-project/locations/us-east5/publishers/anthropic/models/claude-sonnet-4@20250514:rawPredict"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			require.Equal(t, types.HeaderStopIteration, action)

			// 验证请求头是否被正确处理
			requestHeaders := host.GetRequestHeaders()
			require.NotNil(t, requestHeaders)

			// 验证 Host 是否被改为 vertex 域名
			require.True(t, test.HasHeaderWithValue(requestHeaders, ":authority", "aiplatform.googleapis.com"),
				"Host header should be changed to vertex domain")
		})
	})
}

func RunVertexRawModeOnHttpRequestBodyTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试 Vertex Raw 模式请求体处理（Express Mode - 透传请求体 + API Key 认证）
		t.Run("vertex raw mode express - request body passthrough", func(t *testing.T) {
			host, status := test.NewTestHost(vertexRawModeExpressConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/projects/test-project/locations/us-central1/publishers/google/models/gemini-2.0-flash:generateContent"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
				{"Authorization", "Bearer some-token"},
			})

			// 设置原生 Vertex 格式的请求体
			requestBody := `{"contents":[{"role":"user","parts":[{"text":"Hello, world!"}]}],"generationConfig":{"temperature":0.7}}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			// Express Mode 不需要暂停等待 OAuth token
			require.Equal(t, types.ActionContinue, action)

			// 验证请求体被透传（不做格式转换）
			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)

			// 请求体应该保持原样
			require.Equal(t, requestBody, string(processedBody), "Request body should be passed through unchanged")

			// 验证 API Key 被追加到 URL path 中
			requestHeaders := host.GetRequestHeaders()
			var pathHeader string
			for _, header := range requestHeaders {
				if header[0] == ":path" {
					pathHeader = header[1]
					break
				}
			}
			require.Contains(t, pathHeader, "?key=test-api-key-for-raw-mode",
				"API key should be appended to path as query parameter")

			// 验证 Authorization header 被删除
			require.False(t, test.HasHeaderWithValue(requestHeaders, "Authorization", "Bearer some-token"),
				"Authorization header should be removed in Express Mode")
		})

		// 测试 Vertex Raw 模式请求体处理（标准模式 - 需要 OAuth token）
		// 注意：使用 countTokens action，因为 generateContent/predict 等会被识别为其他 API 类型
		// 注意：在单元测试环境中，由于测试配置使用的是无效的私钥，JWT 创建会失败，
		// 因此 getToken() 会返回错误，导致 ActionContinue 而不是 ActionPause。
		// 这个测试主要验证代码正确进入了 Vertex Raw 模式的处理分支，请求体被透传。
		t.Run("vertex raw mode standard - request body with oauth", func(t *testing.T) {
			host, status := test.NewTestHost(vertexRawModeStandardConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头 - 使用 countTokens action，这是一个不会被其他 API 类型匹配的原生 Vertex API
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/projects/test-project/locations/us-central1/publishers/google/models/gemini-2.0-flash:countTokens"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			// 设置原生 Vertex 格式的请求体
			requestBody := `{"contents":[{"role":"user","parts":[{"text":"Hello, world!"}]}]}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			// 注意：在单元测试环境中，由于私钥无效，JWT 创建失败会返回 ActionContinue
			// 在真实环境中，如果 JWT 创建成功，会返回 ActionPause 等待 OAuth token
			// 这里我们只验证代码正确进入了 Vertex Raw 模式的处理分支
			require.Equal(t, types.ActionContinue, action)

			// 验证请求体被透传（不做格式转换）
			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)

			// 请求体应该保持原样（这是 Vertex Raw 模式的核心功能）
			require.Equal(t, requestBody, string(processedBody), "Request body should be passed through unchanged")
		})

		// 测试 Vertex Raw 模式请求体处理（带 basePath 前缀 - 路径正确处理）
		t.Run("vertex raw mode with basePath - request body passthrough", func(t *testing.T) {
			host, status := test.NewTestHost(vertexRawModeWithBasePathConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头（带 basePath 前缀）
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/vertex-proxy/v1/projects/test-project/locations/us-central1/publishers/google/models/imagen-4.0-generate-preview-06-06:predict"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			// 设置原生 Vertex 格式的请求体（图片生成）
			requestBody := `{"instances":[{"prompt":"A beautiful sunset"}],"parameters":{"sampleCount":1}}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			// Express Mode 不需要暂停等待 OAuth token
			require.Equal(t, types.ActionContinue, action)

			// 验证请求体被透传
			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)
			require.Equal(t, requestBody, string(processedBody), "Request body should be passed through unchanged")

			// 验证路径已正确处理（移除 basePath）
			requestHeaders := host.GetRequestHeaders()
			pathHeader := ""
			for _, header := range requestHeaders {
				if header[0] == ":path" {
					pathHeader = header[1]
					break
				}
			}
			require.NotContains(t, pathHeader, "/vertex-proxy", "Path should have basePath prefix removed")
		})

		// 测试 Vertex Raw 模式请求体处理（流式请求 - path 已含 ? 时用 & 拼接 API Key）
		t.Run("vertex raw mode express - streaming request body passthrough", func(t *testing.T) {
			host, status := test.NewTestHost(vertexRawModeExpressConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头（流式端点，path 已含 ?alt=sse）
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/projects/test-project/locations/us-central1/publishers/google/models/gemini-2.0-flash:streamGenerateContent?alt=sse"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			// 设置原生 Vertex 格式的请求体
			requestBody := `{"contents":[{"role":"user","parts":[{"text":"Tell me a story"}]}]}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			require.Equal(t, types.ActionContinue, action)

			// 验证请求体被透传
			processedBody := host.GetRequestBody()
			require.NotNil(t, processedBody)
			require.Equal(t, requestBody, string(processedBody), "Request body should be passed through unchanged")

			// 验证 API Key 使用 & 拼接（因为 path 已含 ?alt=sse）
			requestHeaders := host.GetRequestHeaders()
			var pathHeader string
			for _, header := range requestHeaders {
				if header[0] == ":path" {
					pathHeader = header[1]
					break
				}
			}
			require.Contains(t, pathHeader, "?alt=sse&key=test-api-key-for-raw-mode",
				"API key should be appended with & when path already contains ?")
		})

		// 测试 Vertex Raw 模式请求体处理（Express Mode + Anthropic 模型路径）
		t.Run("vertex raw mode express - anthropic model request body with api key", func(t *testing.T) {
			host, status := test.NewTestHost(vertexRawModeExpressConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 使用 Anthropic 模型的原生 Vertex AI REST API 路径
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/projects/test-project/locations/us-east5/publishers/anthropic/models/claude-sonnet-4@20250514:rawPredict"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			requestBody := `{"anthropic_version":"vertex-2023-10-16","messages":[{"role":"user","content":"Hello"}],"max_tokens":1024}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			require.Equal(t, types.ActionContinue, action)

			// 验证请求体被透传
			processedBody := host.GetRequestBody()
			require.Equal(t, requestBody, string(processedBody), "Request body should be passed through unchanged")

			// 验证 API Key 被追加到 path
			requestHeaders := host.GetRequestHeaders()
			var pathHeader string
			for _, header := range requestHeaders {
				if header[0] == ":path" {
					pathHeader = header[1]
					break
				}
			}
			require.Contains(t, pathHeader, "?key=test-api-key-for-raw-mode",
				"API key should be appended to anthropic model path")
		})

		// 测试 Vertex Raw 模式请求体处理（Express Mode + basePath - API Key 正确追加）
		t.Run("vertex raw mode with basePath express - request body with api key", func(t *testing.T) {
			host, status := test.NewTestHost(vertexRawModeWithBasePathConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 带 basePath 前缀的请求
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/vertex-proxy/v1/projects/test-project/locations/us-central1/publishers/google/models/gemini-2.0-flash:generateContent"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			requestBody := `{"contents":[{"role":"user","parts":[{"text":"Hello"}]}]}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))

			require.Equal(t, types.ActionContinue, action)

			// 验证路径：basePath 被移除 + API Key 被追加
			requestHeaders := host.GetRequestHeaders()
			var pathHeader string
			for _, header := range requestHeaders {
				if header[0] == ":path" {
					pathHeader = header[1]
					break
				}
			}
			require.NotContains(t, pathHeader, "/vertex-proxy",
				"Path should have basePath prefix removed")
			require.Contains(t, pathHeader, "?key=test-api-key-for-raw-mode",
				"API key should be appended after basePath removal")
		})

		// 测试 Vertex Raw 模式请求体处理（Express Mode + 多 token，使用请求上下文中的 apiTokenInUse）
		t.Run("vertex raw mode express - should reuse api token in context for query key", func(t *testing.T) {
			host, status := test.NewTestHost(vertexRawModeExpressMultiTokensConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 选择一个保证前两次 Intn(2) 结果不同的种子：
			// 第一次用于 SetApiTokenInUse，第二次仅在旧实现中用于 OnRequestBody.GetRandomToken。
			seed := int64(1)
			for {
				r := rand.New(rand.NewSource(seed))
				if r.Intn(2) != r.Intn(2) {
					break
				}
				seed++
			}
			rand.Seed(seed)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/projects/test-project/locations/us-central1/publishers/google/models/gemini-2.0-flash:generateContent"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			requestBody := `{"contents":[{"role":"user","parts":[{"text":"Hello"}]}]}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			requestHeaders := host.GetRequestHeaders()
			var pathHeader string
			for _, header := range requestHeaders {
				if header[0] == ":path" {
					pathHeader = header[1]
					break
				}
			}
			require.NotEmpty(t, pathHeader, "Path header should not be empty")

			parsedPath, err := url.ParseRequestURI(pathHeader)
			require.NoError(t, err)
			query := parsedPath.Query()
			require.Len(t, query["key"], 1, "Path should contain exactly one key query parameter")
			keyInPath := query.Get("key")
			require.NotEmpty(t, keyInPath, "Path should contain key query parameter")

			// 从 debug log 中提取本次请求固定的 apiTokenInUse
			var apiTokenInUse string
			for _, debugLog := range host.GetDebugLogs() {
				const prefix = "Use apiToken "
				const suffix = " to send request"
				start := strings.Index(debugLog, prefix)
				if start == -1 {
					continue
				}
				start += len(prefix)
				end := strings.Index(debugLog[start:], suffix)
				if end == -1 {
					continue
				}
				apiTokenInUse = debugLog[start : start+end]
				break
			}
			require.NotEmpty(t, apiTokenInUse, "apiTokenInUse should be logged")
			require.Equal(t, apiTokenInUse, keyInPath,
				"Query key must use apiTokenInUse from request context")
		})

		// 测试 Vertex Raw 模式请求体处理（Express Mode + 已有 key 参数时应覆盖而不是追加重复）
		t.Run("vertex raw mode express - should replace existing key query parameter", func(t *testing.T) {
			host, status := test.NewTestHost(vertexRawModeExpressConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/projects/test-project/locations/us-central1/publishers/google/models/gemini-2.0-flash:streamGenerateContent?alt=sse&key=client-key&trace=1"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			requestBody := `{"contents":[{"role":"user","parts":[{"text":"Hello"}]}]}`
			action := host.CallOnHttpRequestBody([]byte(requestBody))
			require.Equal(t, types.ActionContinue, action)

			requestHeaders := host.GetRequestHeaders()
			var pathHeader string
			for _, header := range requestHeaders {
				if header[0] == ":path" {
					pathHeader = header[1]
					break
				}
			}
			require.NotEmpty(t, pathHeader, "Path header should not be empty")

			parsedPath, err := url.ParseRequestURI(pathHeader)
			require.NoError(t, err)
			query := parsedPath.Query()

			require.Len(t, query["key"], 1, "Path should contain exactly one key query parameter")
			require.Equal(t, "test-api-key-for-raw-mode", query.Get("key"),
				"Existing key query parameter should be replaced by configured API key")
			require.Equal(t, "sse", query.Get("alt"), "Existing query parameter alt should be preserved")
			require.Equal(t, "1", query.Get("trace"), "Existing query parameter trace should be preserved")
		})
	})
}

func RunVertexRawModeOnHttpResponseBodyTests(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		// 测试 Vertex Raw 模式响应体处理（透传响应）
		t.Run("vertex raw mode express - response body passthrough", func(t *testing.T) {
			host, status := test.NewTestHost(vertexRawModeExpressConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			// 先设置请求头
			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/projects/test-project/locations/us-central1/publishers/google/models/gemini-2.0-flash:generateContent"},
				{":method", "POST"},
				{"Content-Type", "application/json"},
			})

			// 设置请求体
			requestBody := `{"contents":[{"role":"user","parts":[{"text":"Hello"}]}]}`
			host.CallOnHttpRequestBody([]byte(requestBody))

			// 设置响应属性
			host.SetProperty([]string{"response", "code_details"}, []byte("via_upstream"))

			// 设置响应头
			responseHeaders := [][2]string{
				{":status", "200"},
				{"Content-Type", "application/json"},
			}
			host.CallOnHttpResponseHeaders(responseHeaders)

			// 设置原生 Vertex 格式的响应体
			responseBody := `{
				"candidates": [{
					"content": {
						"role": "model",
						"parts": [{"text": "Hello! How can I help you?"}]
					},
					"finishReason": "STOP"
				}],
				"usageMetadata": {
					"promptTokenCount": 5,
					"candidatesTokenCount": 10,
					"totalTokenCount": 15
				}
			}`
			action := host.CallOnHttpResponseBody([]byte(responseBody))

			require.Equal(t, types.ActionContinue, action)

			// 验证响应体被透传（不做格式转换）
			processedResponseBody := host.GetResponseBody()
			require.NotNil(t, processedResponseBody)

			responseStr := string(processedResponseBody)
			// 响应应该保持原生 Vertex 格式
			require.Contains(t, responseStr, "candidates", "Response should keep native vertex format with candidates")
			require.Contains(t, responseStr, "usageMetadata", "Response should keep native vertex format with usageMetadata")
		})
	})
}
