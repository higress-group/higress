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

	"github.com/tidwall/gjson"
)

// Clean, minimal tests using only the standard library.
// These tests cover isPathEnabled and extractStreamingTokenUsage for several vendors and edge cases.

// Test configurations
var (
	basicConfig = func() json.RawMessage {
		data, _ := json.Marshal(map[string]interface{}{
			"enable_path_suffixes": []string{"/v1/chat/completions", "/v1/completions"},
			"enable_content_types": []string{"application/json", "text/event-stream"},
			"exporters": []map[string]interface{}{
				{
					"type": "log",
					"config": map[string]interface{}{
						"level": "info",
					},
				},
				{
					"type": "prometheus",
					"config": map[string]interface{}{
						"namespace": "higress",
						"subsystem": "token_statistics",
					},
				},
			},
		})
		return data
	}()

	emptyConfig = func() json.RawMessage {
		data, _ := json.Marshal(map[string]interface{}{})
		return data
	}()

	pathFilterConfig = func() json.RawMessage {
		data, _ := json.Marshal(map[string]interface{}{
			"enable_path_suffixes": []string{"/chat/completions"},
		})
		return data
	}()
)

func TestIsPathEnabled_Basic(t *testing.T) {
	cases := []struct {
		name     string
		path     string
		suffixes []string
		want     bool
	}{
		{"match", "/v1/chat/completions?model=x", []string{"/chat/completions"}, true},
		{"no match", "/v1/embeddings", []string{"/chat/completions"}, false},
		{"empty suffixes", "/any/path", []string{}, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := isPathEnabled(c.path, c.suffixes)
			if got != c.want {
				t.Fatalf("isPathEnabled(%q, %v) = %v, want %v", c.path, c.suffixes, got, c.want)
			}
		})
	}
}

func TestExtractStreamingTokenUsage_OpenAI(t *testing.T) {
	body := []byte(`{"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15},"model":"gpt-4"}`)
	u := extractStreamingTokenUsage("openai", body)
	if u == nil {
		t.Fatal("expected non-nil result for OpenAI")
	}
	if u.InputTokens != 10 || u.OutputTokens != 5 || u.TotalTokens != 15 || u.Model != "gpt-4" {
		t.Fatalf("unexpected OpenAI result: %+v", u)
	}
}

func TestExtractStreamingTokenUsage_Anthropic(t *testing.T) {
	body := []byte(`{"usage":{"input_tokens":4,"output_tokens":6,"total_tokens":10},"model":"claude-1"}`)
	u := extractStreamingTokenUsage("anthropic", body)
	if u == nil {
		t.Fatal("expected non-nil for Anthropic")
	}
	if u.InputTokens != 4 || u.OutputTokens != 6 || u.TotalTokens != 10 || u.Model != "claude-1" {
		t.Fatalf("unexpected Anthropic result: %+v", u)
	}
}

func TestExtractStreamingTokenUsage_DeepL(t *testing.T) {
	body := []byte(`{"character_count":130,"character_count_target":65}`)
	u := extractStreamingTokenUsage("deepl", body)
	if u == nil {
		t.Fatal("expected non-nil for DeepL")
	}
	if u.InputTokens != 100 || u.OutputTokens != 50 || u.TotalTokens != 150 {
		t.Fatalf("unexpected DeepL result: %+v", u)
	}
}

func TestExtractStreamingTokenUsage_Gemini(t *testing.T) {
	body := []byte(`{"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":3,"totalTokenCount":5},"model":"gemini-pro"}`)
	u := extractStreamingTokenUsage("gemini", body)
	if u == nil {
		t.Fatal("expected non-nil for Gemini")
	}
	if u.InputTokens != 2 || u.OutputTokens != 3 || u.TotalTokens != 5 || u.Model != "gemini-pro" {
		t.Fatalf("unexpected Gemini result: %+v", u)
	}
}

func TestExtractStreamingTokenUsage_MultipleVendors(t *testing.T) {
	cases := []struct {
		vendor       string
		body         []byte
		in, out, tot int64
		model        string
	}{
		{"qwen", []byte(`{"usage":{"input_tokens":7,"output_tokens":2},"model":"qwen-turbo"}`), 7, 2, 9, "qwen-turbo"},
		{"baichuan", []byte(`{"usage":{"input_tokens":3,"output_tokens":4},"model":"baichuan2-13b-chat"}`), 3, 4, 7, "baichuan2-13b-chat"},
		{"cohere", []byte(`{"meta":{"tokens":{"input_tokens":11,"output_tokens":1}},"model":"command-r-plus"}`), 11, 1, 12, "command-r-plus"},
	}

	for _, c := range cases {
		t.Run(c.vendor, func(t *testing.T) {
			u := extractStreamingTokenUsage(c.vendor, c.body)
			if u == nil {
				t.Fatalf("%s: expected non-nil", c.vendor)
			}
			if u.InputTokens != c.in || u.OutputTokens != c.out || u.TotalTokens != c.tot || u.Model != c.model {
				t.Fatalf("%s: unexpected %+v", c.vendor, u)
			}
		})
	}
}

func TestExtractStreamingTokenUsage_AliasAndMalformed(t *testing.T) {
	// alias azure_openai -> azure
	body := []byte(`{"usage":{"prompt_tokens":2,"completion_tokens":2,"total_tokens":4},"model":"azure-model"}`)
	u := extractStreamingTokenUsage("azure_openai", body)
	if u == nil {
		t.Fatal("expected non-nil for alias")
	}
	if u.InputTokens != 2 || u.OutputTokens != 2 || u.TotalTokens != 4 || u.Model != "azure-model" {
		t.Fatalf("unexpected alias result: %+v", u)
	}

	// malformed body should return nil
	bad := []byte(`not-a-json`)
	u2 := extractStreamingTokenUsage("openai", bad)
	if u2 != nil {
		t.Fatalf("expected nil for malformed, got %+v", u2)
	}
}

// Test ExtractTokenUsage for various AI providers
func TestAzureOpenAIExtractTokenUsage(t *testing.T) {
	p := &AzureOpenAI{}
	body := []byte(`{"usage":{"prompt_tokens":20,"completion_tokens":10,"total_tokens":30},"model":"gpt-4"}`)
	usage := p.ExtractTokenUsage(gjson.Parse(string(body)), body)
	if usage == nil || usage.InputTokens != 20 || usage.OutputTokens != 10 || usage.TotalTokens != 30 {
		t.Fatalf("AzureOpenAI extract failed: %+v", usage)
	}
}

func TestMoonshotExtractTokenUsage(t *testing.T) {
	p := &Moonshot{}
	body := []byte(`{"usage":{"prompt_tokens":15,"completion_tokens":8,"total_tokens":23},"model":"moonshot-v1-8k"}`)
	usage := p.ExtractTokenUsage(gjson.Parse(string(body)), body)
	if usage == nil || usage.InputTokens != 15 || usage.OutputTokens != 8 || usage.Model != "moonshot-v1-8k" {
		t.Fatalf("Moonshot extract failed: %+v", usage)
	}
}

func TestZhipuAIExtractTokenUsage(t *testing.T) {
	p := &ZhipuAI{}
	// Test new format
	body := []byte(`{"usage":{"prompt_tokens":12,"completion_tokens":6,"total_tokens":18},"model":"glm-4"}`)
	usage := p.ExtractTokenUsage(gjson.Parse(string(body)), body)
	if usage == nil || usage.InputTokens != 12 || usage.Model != "glm-4" {
		t.Fatalf("ZhipuAI new format failed: %+v", usage)
	}

	// Test old format
	body2 := []byte(`{"data":{"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15},"model":"glm-3-turbo"}}`)
	usage2 := p.ExtractTokenUsage(gjson.Parse(string(body2)), body2)
	if usage2 == nil || usage2.InputTokens != 10 || usage2.Model != "glm-3-turbo" {
		t.Fatalf("ZhipuAI old format failed: %+v", usage2)
	}
}

func TestYiExtractTokenUsage(t *testing.T) {
	p := &Yi{}
	body := []byte(`{"usage":{"prompt_tokens":9,"completion_tokens":4,"total_tokens":13},"model":"yi-large"}`)
	usage := p.ExtractTokenUsage(gjson.Parse(string(body)), body)
	if usage == nil || usage.OutputTokens != 4 || usage.Model != "yi-large" {
		t.Fatalf("Yi extract failed: %+v", usage)
	}
}

func TestBaiduExtractTokenUsage(t *testing.T) {
	p := &Baidu{}
	body := []byte(`{"usage":{"input_tokens":11,"output_tokens":7},"result":"test"}`)
	usage := p.ExtractTokenUsage(gjson.Parse(string(body)), body)
	if usage == nil || usage.InputTokens != 11 || usage.OutputTokens != 7 || usage.Model != "ernie-bot" {
		t.Fatalf("Baidu extract failed: %+v", usage)
	}
}

func TestSparkExtractTokenUsage(t *testing.T) {
	p := &Spark{}
	// Test new format
	body := []byte(`{"usage":{"prompt_tokens":14,"completion_tokens":9},"model":"spark-3.5"}`)
	usage := p.ExtractTokenUsage(gjson.Parse(string(body)), body)
	if usage == nil || usage.InputTokens != 14 || usage.OutputTokens != 9 {
		t.Fatalf("Spark new format failed: %+v", usage)
	}

	// Test old format
	body2 := []byte(`{"data":{"usage":{"prompt_tokens":8,"completion_tokens":5,"total_tokens":13}}}`)
	usage2 := p.ExtractTokenUsage(gjson.Parse(string(body2)), body2)
	if usage2 == nil || usage2.TotalTokens != 13 || usage2.Model != "spark-3.0" {
		t.Fatalf("Spark old format failed: %+v", usage2)
	}
}

func TestMiniMaxExtractTokenUsage(t *testing.T) {
	p := &MiniMax{}
	body := []byte(`{"usage":{"input_tokens":15,"output_tokens":10},"model":"minimax-pro"}`)
	usage := p.ExtractTokenUsage(gjson.Parse(string(body)), body)
	if usage == nil || usage.InputTokens != 15 || usage.OutputTokens != 10 || usage.TotalTokens != 25 {
		t.Fatalf("MiniMax extract failed: %+v", usage)
	}
}

func TestDeepSeekExtractTokenUsage(t *testing.T) {
	p := &DeepSeek{}
	body := []byte(`{"usage":{"prompt_tokens":13,"completion_tokens":7,"total_tokens":20},"model":"deepseek-chat"}`)
	usage := p.ExtractTokenUsage(gjson.Parse(string(body)), body)
	if usage == nil || usage.OutputTokens != 7 || usage.Model != "deepseek-chat" {
		t.Fatalf("DeepSeek extract failed: %+v", usage)
	}
}

func TestCohereExtractTokenUsage(t *testing.T) {
	p := &Cohere{}
	body := []byte(`{"meta":{"tokens":{"input_tokens":18,"output_tokens":9}},"model":"command-r-plus"}`)
	usage := p.ExtractTokenUsage(gjson.Parse(string(body)), body)
	if usage == nil || usage.InputTokens != 18 || usage.OutputTokens != 9 {
		t.Fatalf("Cohere extract failed: %+v", usage)
	}
}

func TestMistralExtractTokenUsage(t *testing.T) {
	p := &Mistral{}
	body := []byte(`{"usage":{"prompt_tokens":19,"completion_tokens":11,"total_tokens":30},"model":"mistral-large"}`)
	usage := p.ExtractTokenUsage(gjson.Parse(string(body)), body)
	if usage == nil || usage.TotalTokens != 30 || usage.Model != "mistral-large" {
		t.Fatalf("Mistral extract failed: %+v", usage)
	}
}

func TestGroqExtractTokenUsage(t *testing.T) {
	p := &Groq{}
	body := []byte(`{"usage":{"prompt_tokens":17,"completion_tokens":8,"total_tokens":25},"model":"llama3-70b"}`)
	usage := p.ExtractTokenUsage(gjson.Parse(string(body)), body)
	if usage == nil || usage.InputTokens != 17 || usage.Model != "llama3-70b" {
		t.Fatalf("Groq extract failed: %+v", usage)
	}
}

func TestDeepLExtractTokenUsage(t *testing.T) {
	p := &DeepL{}
	body := []byte(`{"character_count":260,"character_count_target":130}`)
	usage := p.ExtractTokenUsage(gjson.Parse(string(body)), body)
	// DeepL: 260/1.3 = 200, 130/1.3 = 100
	if usage == nil || usage.InputTokens != 200 || usage.OutputTokens != 100 {
		t.Fatalf("DeepL extract failed: %+v", usage)
	}
}

func TestAnthropicExtractTokenUsage(t *testing.T) {
	p := &Anthropic{}
	body := []byte(`{"usage":{"input_tokens":25,"output_tokens":15},"model":"claude-3"}`)
	usage := p.ExtractTokenUsage(gjson.Parse(string(body)), body)
	if usage == nil || usage.InputTokens != 25 || usage.OutputTokens != 15 {
		t.Fatalf("Anthropic extract failed: %+v", usage)
	}
}

func TestGeminiExtractTokenUsage(t *testing.T) {
	p := &Gemini{}
	body := []byte(`{"usageMetadata":{"promptTokenCount":12,"candidatesTokenCount":8,"totalTokenCount":20},"model":"gemini-pro"}`)
	usage := p.ExtractTokenUsage(gjson.Parse(string(body)), body)
	if usage == nil || usage.InputTokens != 12 || usage.TotalTokens != 20 {
		t.Fatalf("Gemini extract failed: %+v", usage)
	}
}

func TestHunyuanExtractTokenUsage(t *testing.T) {
	p := &Hunyuan{}
	body := []byte(`{"usage":{"prompt_tokens":18,"completion_tokens":12,"total_tokens":30},"model":"hunyuan-pro"}`)
	usage := p.ExtractTokenUsage(gjson.Parse(string(body)), body)
	if usage == nil || usage.InputTokens != 18 || usage.Model != "hunyuan-pro" {
		t.Fatalf("Hunyuan extract failed: %+v", usage)
	}
}

func TestAI360ExtractTokenUsage(t *testing.T) {
	p := &AI360{}
	body := []byte(`{"usage":{"prompt_tokens":14,"completion_tokens":9},"model":"360zhinao-pro"}`)
	usage := p.ExtractTokenUsage(gjson.Parse(string(body)), body)
	if usage == nil || usage.InputTokens != 14 || usage.OutputTokens != 9 {
		t.Fatalf("AI360 extract failed: %+v", usage)
	}
}

func TestStepfunExtractTokenUsage(t *testing.T) {
	p := &Stepfun{}
	body := []byte(`{"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18},"model":"stepfun-7b"}`)
	usage := p.ExtractTokenUsage(gjson.Parse(string(body)), body)
	if usage == nil || usage.TotalTokens != 18 || usage.Model != "stepfun-7b" {
		t.Fatalf("Stepfun extract failed: %+v", usage)
	}
}

func TestClaudeExtractTokenUsage(t *testing.T) {
	p := &Claude{}
	// Test new format
	body := []byte(`{"usage":{"input_tokens":20,"output_tokens":15,"total_tokens":35},"model":"claude-3-opus"}`)
	usage := p.ExtractTokenUsage(gjson.Parse(string(body)), body)
	if usage == nil || usage.InputTokens != 20 || usage.TotalTokens != 35 {
		t.Fatalf("Claude new format failed: %+v", usage)
	}

	// Test old format with usage_metadata
	body2 := []byte(`{"usage_metadata":{"input_tokens":16,"output_tokens":10},"model":"claude-2"}`)
	usage2 := p.ExtractTokenUsage(gjson.Parse(string(body2)), body2)
	if usage2 == nil || usage2.InputTokens != 16 || usage2.OutputTokens != 10 {
		t.Fatalf("Claude old format failed: %+v", usage2)
	}
}

func TestOllamaExtractTokenUsage(t *testing.T) {
	p := &Ollama{}
	body := []byte(`{"usage":{"prompt_tokens":22,"completion_tokens":14,"total_tokens":36},"model":"llama3:8b"}`)
	usage := p.ExtractTokenUsage(gjson.Parse(string(body)), body)
	if usage == nil || usage.InputTokens != 22 || usage.Model != "llama3:8b" {
		t.Fatalf("Ollama extract failed: %+v", usage)
	}
}

func TestCloudflareExtractTokenUsage(t *testing.T) {
	p := &Cloudflare{}
	body := []byte(`{"usage":{"input_tokens":19,"output_tokens":11},"model":"@cf/meta/llama-2"}`)
	usage := p.ExtractTokenUsage(gjson.Parse(string(body)), body)
	if usage == nil || usage.InputTokens != 19 || usage.OutputTokens != 11 || usage.TotalTokens != 30 {
		t.Fatalf("Cloudflare extract failed: %+v", usage)
	}
}

func TestTogetherAIExtractTokenUsage(t *testing.T) {
	p := &TogetherAI{}
	body := []byte(`{"usage":{"prompt_tokens":17,"completion_tokens":13,"total_tokens":30},"model":"mistralai/Mistral-7B"}`)
	usage := p.ExtractTokenUsage(gjson.Parse(string(body)), body)
	if usage == nil || usage.InputTokens != 17 || usage.OutputTokens != 13 {
		t.Fatalf("TogetherAI extract failed: %+v", usage)
	}
}
