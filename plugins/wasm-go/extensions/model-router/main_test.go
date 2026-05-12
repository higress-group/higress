package main

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"regexp"
	"strings"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Basic configs for wasm test host
var (
	basicConfig = func() json.RawMessage {
		data, _ := json.Marshal(map[string]interface{}{
			"modelKey":          "model",
			"addProviderHeader": "x-provider",
			"modelToHeader":     "x-model",
			"enableOnPathSuffix": []string{
				"/v1/chat/completions",
			},
		})
		return data
	}()

	defaultSuffixConfig = func() json.RawMessage {
		data, _ := json.Marshal(map[string]interface{}{
			"modelKey":          "model",
			"addProviderHeader": "x-provider",
			"modelToHeader":     "x-model",
		})
		return data
	}()
)

func getHeader(headers [][2]string, key string) (string, bool) {
	for _, h := range headers {
		if strings.EqualFold(h[0], key) {
			return h[1], true
		}
	}
	return "", false
}

func TestParseConfig(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		t.Run("basic config with defaults", func(t *testing.T) {
			var cfg ModelRouterConfig
			err := parseConfig(gjson.ParseBytes(defaultSuffixConfig), &cfg)
			require.NoError(t, err)

			// default modelKey
			require.Equal(t, "model", cfg.modelKey)
			// headers
			require.Equal(t, "x-provider", cfg.addProviderHeader)
			require.Equal(t, "x-model", cfg.modelToHeader)
			// default enabled path suffixes should contain common openai paths
			require.Contains(t, cfg.enableOnPathSuffix, "/completions")
			require.Contains(t, cfg.enableOnPathSuffix, "/embeddings")
		})

		t.Run("custom enableOnPathSuffix", func(t *testing.T) {
			jsonData := []byte(`{
				"modelKey": "my_model",
				"addProviderHeader": "x-prov",
				"modelToHeader": "x-mod",
				"enableOnPathSuffix": ["/foo", "/bar"]
			}`)
			var cfg ModelRouterConfig
			err := parseConfig(gjson.ParseBytes(jsonData), &cfg)
			require.NoError(t, err)

			require.Equal(t, "my_model", cfg.modelKey)
			require.Equal(t, "x-prov", cfg.addProviderHeader)
			require.Equal(t, "x-mod", cfg.modelToHeader)
			require.Equal(t, []string{"/foo", "/bar"}, cfg.enableOnPathSuffix)
		})
	})
}

func TestOnHttpRequestHeaders(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("skip when path not matched", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			originalHeaders := [][2]string{
				{":authority", "example.com"},
				{":path", "/v1/other"},
				{":method", "POST"},
				{"content-type", "application/json"},
				{"content-length", "123"},
			}
			action := host.CallOnHttpRequestHeaders(originalHeaders)
			require.Equal(t, types.ActionContinue, action)

			newHeaders := host.GetRequestHeaders()
			_, found := getHeader(newHeaders, "content-length")
			require.True(t, found, "content-length should be kept when path not enabled")
		})

		t.Run("process when path and content-type match", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			originalHeaders := [][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"content-type", "application/json"},
				{"content-length", "123"},
			}
			action := host.CallOnHttpRequestHeaders(originalHeaders)
			require.Equal(t, types.HeaderStopIteration, action)

			newHeaders := host.GetRequestHeaders()
			_, found := getHeader(newHeaders, "content-length")
			require.False(t, found, "content-length should be removed when buffering body")
		})

		t.Run("do not process for unsupported content-type", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			originalHeaders := [][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"content-type", "text/plain"},
				{"content-length", "123"},
			}
			action := host.CallOnHttpRequestHeaders(originalHeaders)
			require.Equal(t, types.HeaderStopIteration, action)

			newHeaders := host.GetRequestHeaders()
			_, found := getHeader(newHeaders, "content-length")
			require.False(t, found, "content-length should not be removed for unsupported content-type")
		})
	})
}

func TestOnHttpRequestBody_JSON(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("set headers and rewrite model when provider/model format", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			origBody := []byte(`{
				"model": "openai/gpt-4o",
				"messages": [{"role": "user", "content": "hello"}]
			}`)
			action := host.CallOnHttpRequestBody(origBody)
			require.Equal(t, types.ActionContinue, action)

			processed := host.GetRequestBody()
			require.NotNil(t, processed)
			// model should be rewritten to only the model part
			require.Equal(t, "gpt-4o", gjson.GetBytes(processed, "model").String())

			headers := host.GetRequestHeaders()
			hv, found := getHeader(headers, "x-model")
			require.True(t, found)
			require.Equal(t, "openai/gpt-4o", hv)
			pv, found := getHeader(headers, "x-provider")
			require.True(t, found)
			require.Equal(t, "openai", pv)
		})

		t.Run("no change when model not provided", func(t *testing.T) {
			host, status := test.NewTestHost(basicConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			origBody := []byte(`{
				"messages": [{"role": "user", "content": "hello"}]
			}`)
			action := host.CallOnHttpRequestBody(origBody)
			require.Equal(t, types.ActionContinue, action)

			processed := host.GetRequestBody()
			// body should remain nil or unchanged as plugin does nothing
			if processed != nil {
				require.JSONEq(t, string(origBody), string(processed))
			}
			_, found := getHeader(host.GetRequestHeaders(), "x-provider")
			require.False(t, found)
		})
	})
}

func TestOnHttpRequestBody_Multipart(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		host, status := test.NewTestHost(basicConfig)
		defer host.Reset()
		require.Equal(t, types.OnPluginStartStatusOK, status)

		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)

		// model field
		modelWriter, err := writer.CreateFormField("model")
		require.NoError(t, err)
		_, err = modelWriter.Write([]byte("openai/gpt-4o"))
		require.NoError(t, err)

		// another field to ensure others are preserved
		fileWriter, err := writer.CreateFormField("prompt")
		require.NoError(t, err)
		_, err = fileWriter.Write([]byte("hello"))
		require.NoError(t, err)

		err = writer.Close()
		require.NoError(t, err)

		contentType := "multipart/form-data; boundary=" + writer.Boundary()

		host.CallOnHttpRequestHeaders([][2]string{
			{":authority", "example.com"},
			{":path", "/v1/chat/completions"},
			{":method", "POST"},
			{"content-type", contentType},
		})

		action := host.CallOnHttpRequestBody(buf.Bytes())
		require.Equal(t, types.ActionContinue, action)

		processed := host.GetRequestBody()
		require.NotNil(t, processed)

		// Parse multipart body again to verify fields
		reader := multipart.NewReader(bytes.NewReader(processed), writer.Boundary())

		foundModel := false
		foundPrompt := false
		for {
			part, err := reader.NextPart()
			if err != nil {
				break
			}
			name := part.FormName()
			data, err := io.ReadAll(part)
			require.NoError(t, err)

			switch name {
			case "model":
				foundModel = true
				require.Equal(t, "gpt-4o", string(data))
			case "prompt":
				foundPrompt = true
				require.Equal(t, "hello", string(data))
			}
		}

		require.True(t, foundModel)
		require.True(t, foundPrompt)

		headers := host.GetRequestHeaders()
		hv, found := getHeader(headers, "x-model")
		require.True(t, found)
		require.Equal(t, "openai/gpt-4o", hv)
		pv, found := getHeader(headers, "x-provider")
		require.True(t, found)
		require.Equal(t, "openai", pv)
	})
}

// Auto routing config for tests
var autoRoutingConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"modelKey":      "model",
		"modelToHeader": "x-model",
		"enableOnPathSuffix": []string{
			"/v1/chat/completions",
		},
		"autoRouting": map[string]interface{}{
			"enable":       true,
			"defaultModel": "qwen-turbo",
			"rules": []map[string]string{
				{"pattern": "(?i)(画|绘|生成图|图片|image|draw|paint)", "model": "qwen-vl-max"},
				{"pattern": "(?i)(代码|编程|code|program|function|debug)", "model": "qwen-coder"},
				{"pattern": "(?i)(翻译|translate|translation)", "model": "qwen-turbo"},
				{"pattern": "(?i)(数学|计算|math|calculate)", "model": "qwen-math"},
			},
		},
	})
	return data
}()

var autoRoutingNoDefaultConfig = func() json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{
		"modelKey":      "model",
		"modelToHeader": "x-model",
		"enableOnPathSuffix": []string{
			"/v1/chat/completions",
		},
		"autoRouting": map[string]interface{}{
			"enable": true,
			"rules": []map[string]string{
				{"pattern": "(?i)(画|绘)", "model": "qwen-vl-max"},
			},
		},
	})
	return data
}()

func TestParseConfigAutoRouting(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		t.Run("parse auto routing config", func(t *testing.T) {
			var cfg ModelRouterConfig
			err := parseConfig(gjson.ParseBytes(autoRoutingConfig), &cfg)
			require.NoError(t, err)

			require.True(t, cfg.enableAutoRouting)
			require.Equal(t, "qwen-turbo", cfg.defaultModel)
			require.Len(t, cfg.autoRoutingRules, 4)

			// Verify first rule
			require.Equal(t, "qwen-vl-max", cfg.autoRoutingRules[0].Model)
			require.NotNil(t, cfg.autoRoutingRules[0].Pattern)
		})

		t.Run("skip invalid regex patterns", func(t *testing.T) {
			jsonData := []byte(`{
				"autoRouting": {
					"enable": true,
					"rules": [
						{"pattern": "[invalid", "model": "model1"},
						{"pattern": "valid", "model": "model2"}
					]
				}
			}`)
			var cfg ModelRouterConfig
			err := parseConfig(gjson.ParseBytes(jsonData), &cfg)
			require.NoError(t, err)

			// Only valid rule should be parsed
			require.Len(t, cfg.autoRoutingRules, 1)
			require.Equal(t, "model2", cfg.autoRoutingRules[0].Model)
		})

		t.Run("skip rules with empty pattern or model", func(t *testing.T) {
			jsonData := []byte(`{
				"autoRouting": {
					"enable": true,
					"rules": [
						{"pattern": "", "model": "model1"},
						{"pattern": "test", "model": ""},
						{"pattern": "valid", "model": "model2"}
					]
				}
			}`)
			var cfg ModelRouterConfig
			err := parseConfig(gjson.ParseBytes(jsonData), &cfg)
			require.NoError(t, err)

			require.Len(t, cfg.autoRoutingRules, 1)
			require.Equal(t, "model2", cfg.autoRoutingRules[0].Model)
		})
	})
}

func TestExtractLastUserMessage(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		t.Run("extract from simple string content", func(t *testing.T) {
			body := []byte(`{
				"model": "higress/auto",
				"messages": [
					{"role": "system", "content": "You are a helpful assistant"},
					{"role": "user", "content": "Hello, how are you?"},
					{"role": "assistant", "content": "I am fine"},
					{"role": "user", "content": "Please draw a cat"}
				]
			}`)
			result := extractLastUserMessage(body)
			require.Equal(t, "Please draw a cat", result)
		})

		t.Run("extract from array content (multimodal)", func(t *testing.T) {
			body := []byte(`{
				"model": "higress/auto",
				"messages": [
					{"role": "user", "content": [
						{"type": "text", "text": "What is in this image?"},
						{"type": "image_url", "image_url": {"url": "https://example.com/image.jpg"}}
					]}
				]
			}`)
			result := extractLastUserMessage(body)
			require.Equal(t, "What is in this image?", result)
		})

		t.Run("extract last text from array with multiple text items", func(t *testing.T) {
			body := []byte(`{
				"model": "higress/auto",
				"messages": [
					{"role": "user", "content": [
						{"type": "text", "text": "First text"},
						{"type": "image_url", "image_url": {"url": "https://example.com/image.jpg"}},
						{"type": "text", "text": "Second text about drawing"}
					]}
				]
			}`)
			result := extractLastUserMessage(body)
			require.Equal(t, "Second text about drawing", result)
		})

		t.Run("return empty when no messages", func(t *testing.T) {
			body := []byte(`{"model": "higress/auto"}`)
			result := extractLastUserMessage(body)
			require.Equal(t, "", result)
		})

		t.Run("return empty when no user messages", func(t *testing.T) {
			body := []byte(`{
				"model": "higress/auto",
				"messages": [
					{"role": "system", "content": "You are a helpful assistant"},
					{"role": "assistant", "content": "Hello!"}
				]
			}`)
			result := extractLastUserMessage(body)
			require.Equal(t, "", result)
		})

		t.Run("handle multiple user messages", func(t *testing.T) {
			body := []byte(`{
				"model": "higress/auto",
				"messages": [
					{"role": "user", "content": "First question"},
					{"role": "assistant", "content": "First answer"},
					{"role": "user", "content": "帮我写一段代码"}
				]
			}`)
			result := extractLastUserMessage(body)
			require.Equal(t, "帮我写一段代码", result)
		})
	})
}

func TestMatchAutoRoutingRule(t *testing.T) {
	test.RunGoTest(t, func(t *testing.T) {
		config := ModelRouterConfig{
			autoRoutingRules: []AutoRoutingRule{
				{Pattern: regexp.MustCompile(`(?i)(画|绘|图片)`), Model: "qwen-vl-max"},
				{Pattern: regexp.MustCompile(`(?i)(代码|编程|code)`), Model: "qwen-coder"},
				{Pattern: regexp.MustCompile(`(?i)(数学|计算)`), Model: "qwen-math"},
			},
		}

		t.Run("match drawing keywords", func(t *testing.T) {
			model, found := matchAutoRoutingRule(config, "请帮我画一只猫")
			require.True(t, found)
			require.Equal(t, "qwen-vl-max", model)
		})

		t.Run("match code keywords", func(t *testing.T) {
			model, found := matchAutoRoutingRule(config, "Write a Python code to sort a list")
			require.True(t, found)
			require.Equal(t, "qwen-coder", model)
		})

		t.Run("match Chinese code keywords", func(t *testing.T) {
			model, found := matchAutoRoutingRule(config, "帮我写一段编程代码")
			require.True(t, found)
			// First matching rule wins (代码 matches first rule with 代码)
			require.Equal(t, "qwen-coder", model)
		})

		t.Run("match math keywords", func(t *testing.T) {
			model, found := matchAutoRoutingRule(config, "计算123+456等于多少")
			require.True(t, found)
			require.Equal(t, "qwen-math", model)
		})

		t.Run("no match returns false", func(t *testing.T) {
			model, found := matchAutoRoutingRule(config, "今天天气怎么样？")
			require.False(t, found)
			require.Equal(t, "", model)
		})

		t.Run("case insensitive matching", func(t *testing.T) {
			model, found := matchAutoRoutingRule(config, "Write some CODE for me")
			require.True(t, found)
			require.Equal(t, "qwen-coder", model)
		})

		t.Run("first matching rule wins", func(t *testing.T) {
			// Message contains both "图片" and "代码"
			model, found := matchAutoRoutingRule(config, "生成一张图片的代码")
			require.True(t, found)
			// "图片" rule comes first
			require.Equal(t, "qwen-vl-max", model)
		})
	})
}

func TestAutoRoutingIntegration(t *testing.T) {
	test.RunTest(t, func(t *testing.T) {
		t.Run("auto routing with matching rule", func(t *testing.T) {
			host, status := test.NewTestHost(autoRoutingConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			body := []byte(`{
				"model": "higress/auto",
				"messages": [
					{"role": "system", "content": "You are a helpful assistant"},
					{"role": "user", "content": "请帮我画一只可爱的小猫"}
				]
			}`)
			action := host.CallOnHttpRequestBody(body)
			require.Equal(t, types.ActionContinue, action)

			headers := host.GetRequestHeaders()
			modelHeader, found := getHeader(headers, "x-higress-llm-model")
			require.True(t, found, "x-higress-llm-model header should be set")
			require.Equal(t, "qwen-vl-max", modelHeader)
		})

		t.Run("auto routing with code keywords", func(t *testing.T) {
			host, status := test.NewTestHost(autoRoutingConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			body := []byte(`{
				"model": "higress/auto",
				"messages": [
					{"role": "user", "content": "Write a function to calculate fibonacci numbers"}
				]
			}`)
			action := host.CallOnHttpRequestBody(body)
			require.Equal(t, types.ActionContinue, action)

			headers := host.GetRequestHeaders()
			modelHeader, found := getHeader(headers, "x-higress-llm-model")
			require.True(t, found)
			require.Equal(t, "qwen-coder", modelHeader)
		})

		t.Run("auto routing falls back to default model", func(t *testing.T) {
			host, status := test.NewTestHost(autoRoutingConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			body := []byte(`{
				"model": "higress/auto",
				"messages": [
					{"role": "user", "content": "今天天气怎么样？"}
				]
			}`)
			action := host.CallOnHttpRequestBody(body)
			require.Equal(t, types.ActionContinue, action)

			headers := host.GetRequestHeaders()
			modelHeader, found := getHeader(headers, "x-higress-llm-model")
			require.True(t, found)
			require.Equal(t, "qwen-turbo", modelHeader)
		})

		t.Run("auto routing no default model configured", func(t *testing.T) {
			host, status := test.NewTestHost(autoRoutingNoDefaultConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			body := []byte(`{
				"model": "higress/auto",
				"messages": [
					{"role": "user", "content": "今天天气怎么样？"}
				]
			}`)
			action := host.CallOnHttpRequestBody(body)
			require.Equal(t, types.ActionContinue, action)

			headers := host.GetRequestHeaders()
			_, found := getHeader(headers, "x-higress-llm-model")
			require.False(t, found, "x-higress-llm-model should not be set when no rule matches and no default")
		})

		t.Run("normal routing when model is not higress/auto", func(t *testing.T) {
			host, status := test.NewTestHost(autoRoutingConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			body := []byte(`{
				"model": "qwen-long",
				"messages": [
					{"role": "user", "content": "请帮我画一只猫"}
				]
			}`)
			action := host.CallOnHttpRequestBody(body)
			require.Equal(t, types.ActionContinue, action)

			headers := host.GetRequestHeaders()
			modelHeader, found := getHeader(headers, "x-model")
			require.True(t, found)
			require.Equal(t, "qwen-long", modelHeader)

			// x-higress-llm-model should NOT be set (auto routing not triggered)
			_, found = getHeader(headers, "x-higress-llm-model")
			require.False(t, found)
		})

		t.Run("auto routing with multimodal content", func(t *testing.T) {
			host, status := test.NewTestHost(autoRoutingConfig)
			defer host.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, status)

			host.CallOnHttpRequestHeaders([][2]string{
				{":authority", "example.com"},
				{":path", "/v1/chat/completions"},
				{":method", "POST"},
				{"content-type", "application/json"},
			})

			body := []byte(`{
				"model": "higress/auto",
				"messages": [
					{"role": "user", "content": [
						{"type": "text", "text": "帮我翻译这段话"},
						{"type": "image_url", "image_url": {"url": "https://example.com/image.jpg"}}
					]}
				]
			}`)
			action := host.CallOnHttpRequestBody(body)
			require.Equal(t, types.ActionContinue, action)

			headers := host.GetRequestHeaders()
			modelHeader, found := getHeader(headers, "x-higress-llm-model")
			require.True(t, found)
			require.Equal(t, "qwen-turbo", modelHeader) // matches 翻译 rule
		})
	})
}
