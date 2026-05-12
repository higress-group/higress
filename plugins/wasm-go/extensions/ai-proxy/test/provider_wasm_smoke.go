// Package test contains Wasm smoke tests for additional AI providers (legacy top-level
// "provider" JSON, plugin start, request headers, and minimal body where useful). This file
// complements per-vendor files such as openai.go and deepseek.go.
package test

import (
	"encoding/json"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	wasmhost "github.com/higress-group/wasm-go/pkg/test"
	"github.com/stretchr/testify/require"
)

func providerSmokeLegacyJSON(m map[string]interface{}) json.RawMessage {
	data, _ := json.Marshal(map[string]interface{}{"provider": m})
	return data
}

func RunBaichuanWasmSmokeTests(t *testing.T) {
	wasmhost.RunTest(t, func(t *testing.T) {
		cfg := providerSmokeLegacyJSON(map[string]interface{}{
			"type": "baichuan", "apiTokens": []string{"sk-bc"}, "modelMapping": map[string]string{"*": "bc-model"},
		})
		t.Run("parse", func(t *testing.T) {
			h, st := wasmhost.NewTestHost(cfg)
			defer h.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, st)
		})
		t.Run("headers", func(t *testing.T) {
			h, st := wasmhost.NewTestHost(cfg)
			defer h.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, st)
			act := h.CallOnHttpRequestHeaders([][2]string{
				{":authority", "ex.com"}, {":path", "/v1/chat/completions"}, {":method", "POST"}, {"Content-Type", "application/json"},
			})
			require.Equal(t, types.HeaderStopIteration, act)
			auth, _ := wasmhost.GetHeaderValue(h.GetRequestHeaders(), "Authorization")
			require.Contains(t, auth, "Bearer")
		})
	})
}

func RunYiWasmSmokeTests(t *testing.T) {
	wasmhost.RunTest(t, func(t *testing.T) {
		cfg := providerSmokeLegacyJSON(map[string]interface{}{"type": "yi", "apiTokens": []string{"sk-yi"}})
		t.Run("headers", func(t *testing.T) {
			h, st := wasmhost.NewTestHost(cfg)
			defer h.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, st)
			_ = h.CallOnHttpRequestHeaders([][2]string{
				{":authority", "ex.com"}, {":path", "/v1/chat/completions"}, {":method", "POST"}, {"Content-Type", "application/json"},
			})
			host, _ := wasmhost.GetHeaderValue(h.GetRequestHeaders(), ":authority")
			require.Contains(t, host, "lingyi")
		})
	})
}

func RunOllamaWasmSmokeTests(t *testing.T) {
	wasmhost.RunTest(t, func(t *testing.T) {
		cfg := providerSmokeLegacyJSON(map[string]interface{}{
			"type": "ollama", "ollamaServerHost": "127.0.0.1", "ollamaServerPort": 11434, "apiTokens": []string{"x"},
		})
		t.Run("headers", func(t *testing.T) {
			h, st := wasmhost.NewTestHost(cfg)
			defer h.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, st)
			_ = h.CallOnHttpRequestHeaders([][2]string{
				{":authority", "ex.com"}, {":path", "/v1/chat/completions"}, {":method", "POST"}, {"Content-Type", "application/json"},
			})
			host, _ := wasmhost.GetHeaderValue(h.GetRequestHeaders(), ":authority")
			require.Contains(t, host, "127.0.0.1")
		})
	})
}

func RunBaiduWasmSmokeTests(t *testing.T) {
	wasmhost.RunTest(t, func(t *testing.T) {
		cfg := providerSmokeLegacyJSON(map[string]interface{}{"type": "baidu", "apiTokens": []string{"sk-bd"}})
		t.Run("headers", func(t *testing.T) {
			h, st := wasmhost.NewTestHost(cfg)
			defer h.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, st)
			_ = h.CallOnHttpRequestHeaders([][2]string{
				{":authority", "ex.com"}, {":path", "/v1/chat/completions"}, {":method", "POST"}, {"Content-Type", "application/json"},
			})
			path, _ := wasmhost.GetHeaderValue(h.GetRequestHeaders(), ":path")
			require.Contains(t, path, "/v2/")
		})
	})
}

func RunHunyuanWasmSmokeTests(t *testing.T) {
	wasmhost.RunTest(t, func(t *testing.T) {
		cfg := providerSmokeLegacyJSON(map[string]interface{}{"type": "hunyuan", "apiTokens": []string{"tok"}})
		t.Run("parse", func(t *testing.T) {
			h, st := wasmhost.NewTestHost(cfg)
			defer h.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, st)
		})
	})
}

func RunStepfunWasmSmokeTests(t *testing.T) {
	wasmhost.RunTest(t, func(t *testing.T) {
		cfg := providerSmokeLegacyJSON(map[string]interface{}{"type": "stepfun", "apiTokens": []string{"sk-sf"}})
		t.Run("headers", func(t *testing.T) {
			h, st := wasmhost.NewTestHost(cfg)
			defer h.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, st)
			_ = h.CallOnHttpRequestHeaders([][2]string{
				{":authority", "ex.com"}, {":path", "/v1/chat/completions"}, {":method", "POST"}, {"Content-Type", "application/json"},
			})
			host, _ := wasmhost.GetHeaderValue(h.GetRequestHeaders(), ":authority")
			require.Contains(t, host, "stepfun")
		})
	})
}

func RunCloudflareWasmSmokeTests(t *testing.T) {
	wasmhost.RunTest(t, func(t *testing.T) {
		cfg := providerSmokeLegacyJSON(map[string]interface{}{
			"type": "cloudflare", "apiTokens": []string{"cf"}, "cloudflareAccountId": "acc1",
		})
		t.Run("headers", func(t *testing.T) {
			h, st := wasmhost.NewTestHost(cfg)
			defer h.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, st)
			_ = h.CallOnHttpRequestHeaders([][2]string{
				{":authority", "ex.com"}, {":path", "/v1/chat/completions"}, {":method", "POST"}, {"Content-Type", "application/json"},
			})
			path, _ := wasmhost.GetHeaderValue(h.GetRequestHeaders(), ":path")
			require.Contains(t, path, "acc1")
		})
	})
}

func RunDeeplWasmSmokeTests(t *testing.T) {
	wasmhost.RunTest(t, func(t *testing.T) {
		cfg := providerSmokeLegacyJSON(map[string]interface{}{
			"type": "deepl", "apiTokens": []string{"k"}, "targetLang": "EN",
		})
		t.Run("parse", func(t *testing.T) {
			h, st := wasmhost.NewTestHost(cfg)
			defer h.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, st)
		})
	})
}

func RunCohereWasmSmokeTests(t *testing.T) {
	wasmhost.RunTest(t, func(t *testing.T) {
		cfg := providerSmokeLegacyJSON(map[string]interface{}{"type": "cohere", "apiTokens": []string{"ck"}})
		t.Run("headers", func(t *testing.T) {
			h, st := wasmhost.NewTestHost(cfg)
			defer h.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, st)
			_ = h.CallOnHttpRequestHeaders([][2]string{
				{":authority", "ex.com"}, {":path", "/v1/chat/completions"}, {":method", "POST"}, {"Content-Type", "application/json"},
			})
			host, _ := wasmhost.GetHeaderValue(h.GetRequestHeaders(), ":authority")
			require.Contains(t, host, "cohere")
		})
	})
}

func RunCozeWasmSmokeTests(t *testing.T) {
	wasmhost.RunTest(t, func(t *testing.T) {
		cfg := providerSmokeLegacyJSON(map[string]interface{}{"type": "coze", "apiTokens": []string{"cz"}})
		t.Run("headers", func(t *testing.T) {
			h, st := wasmhost.NewTestHost(cfg)
			defer h.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, st)
			_ = h.CallOnHttpRequestHeaders([][2]string{
				{":authority", "ex.com"}, {":path", "/v1/chat/completions"}, {":method", "POST"}, {"Content-Type", "application/json"},
			})
			host, _ := wasmhost.GetHeaderValue(h.GetRequestHeaders(), ":authority")
			require.Contains(t, host, "coze")
		})
	})
}

func RunDifyWasmSmokeTests(t *testing.T) {
	wasmhost.RunTest(t, func(t *testing.T) {
		cfg := providerSmokeLegacyJSON(map[string]interface{}{
			"type": "dify", "apiTokens": []string{"d"}, "botType": "Chat",
		})
		t.Run("headers", func(t *testing.T) {
			h, st := wasmhost.NewTestHost(cfg)
			defer h.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, st)
			_ = h.CallOnHttpRequestHeaders([][2]string{
				{":authority", "ex.com"}, {":path", "/v1/chat/completions"}, {":method", "POST"}, {"Content-Type", "application/json"},
			})
			path, _ := wasmhost.GetHeaderValue(h.GetRequestHeaders(), ":path")
			require.Contains(t, path, "chat-messages")
		})
	})
}

func RunTritonWasmSmokeTests(t *testing.T) {
	wasmhost.RunTest(t, func(t *testing.T) {
		cfg := providerSmokeLegacyJSON(map[string]interface{}{
			"type": "triton", "apiTokens": []string{"t"}, "tritonDomain": "triton.example.com",
		})
		t.Run("headers_and_body", func(t *testing.T) {
			h, st := wasmhost.NewTestHost(cfg)
			defer h.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, st)
			_ = h.CallOnHttpRequestHeaders([][2]string{
				{":authority", "ex.com"}, {":path", "/v1/chat/completions"}, {":method", "POST"}, {"Content-Type", "application/json"},
			})
			_ = h.CallOnHttpRequestBody([]byte(`{"model":"m1","messages":[{"role":"user","content":"hi"}]}`))
			path, _ := wasmhost.GetHeaderValue(h.GetRequestHeaders(), ":path")
			require.Contains(t, path, "m1")
		})
	})
}

func RunVllmWasmSmokeTests(t *testing.T) {
	wasmhost.RunTest(t, func(t *testing.T) {
		cfg := providerSmokeLegacyJSON(map[string]interface{}{"type": "vllm"})
		t.Run("parse", func(t *testing.T) {
			h, st := wasmhost.NewTestHost(cfg)
			defer h.Reset()
			require.Equal(t, types.OnPluginStartStatusOK, st)
		})
	})
}
