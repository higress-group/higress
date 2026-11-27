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
	"math"

	"github.com/tidwall/gjson"
)

type extractTokenUsage interface {
	ExtractTokenUsage(json gjson.Result, body []byte) *TokenUsage
}

type OpenAI struct{}
type AzureOpenAI struct{}
type Anthropic struct{}
type GoogleGemini struct{}

// 从OpenAI格式响应中提取Token使用量
func (openai *OpenAI) ExtractTokenUsage(json gjson.Result, body []byte) *TokenUsage {
	usage := &TokenUsage{}

	if usageData := gjson.GetBytes(body, "usage"); usageData.Exists() {
		usage.InputTokens = usageData.Get("prompt_tokens").Int()
		usage.OutputTokens = usageData.Get("completion_tokens").Int()
		usage.TotalTokens = usageData.Get("total_tokens").Int()
		usage.Model = gjson.GetBytes(body, "model").String()
		return usage
	}

	return nil
}

func (azureOpenAI *AzureOpenAI) ExtractTokenUsage(json gjson.Result, body []byte) *TokenUsage {
	usage := &TokenUsage{}

	if usageData := gjson.GetBytes(body, "usage"); usageData.Exists() {
		usage.InputTokens = usageData.Get("prompt_tokens").Int()
		usage.OutputTokens = usageData.Get("completion_tokens").Int()
		usage.TotalTokens = usageData.Get("total_tokens").Int()
		usage.Model = gjson.GetBytes(body, "model").String()
		return usage
	}

	return nil
}

// 从Anthropic格式响应中提取Token使用量
func (anthropic *Anthropic) ExtractTokenUsage(json gjson.Result, body []byte) *TokenUsage {
	usage := &TokenUsage{}

	if usageData := gjson.GetBytes(body, "usage"); usageData.Exists() {
		usage.InputTokens = usageData.Get("input_tokens").Int()
		usage.OutputTokens = usageData.Get("output_tokens").Int()
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
		usage.Model = gjson.GetBytes(body, "model").String()
		return usage
	}

	return nil
}

// 从Google Gemini格式响应中提取Token使用量
func (gemini *GoogleGemini) ExtractTokenUsage(json gjson.Result, body []byte) *TokenUsage {
	usage := &TokenUsage{}

	if usageData := gjson.GetBytes(body, "usageMetadata"); usageData.Exists() {
		usage.InputTokens = usageData.Get("promptTokenCount").Int()
		usage.OutputTokens = usageData.Get("candidatesTokenCount").Int()
		usage.TotalTokens = usageData.Get("totalTokenCount").Int()
		// Gemini没有直接返回模型名称，需要从请求中获取
		return usage
	}

	return nil
}

// -------------------------- 国内主流厂商 --------------------------
// 通义千问（Qwen）
type Qwen struct{}

func (q *Qwen) ExtractTokenUsage(json gjson.Result, body []byte) *TokenUsage {
	usage := &TokenUsage{}

	// 通义千问响应格式：{"usage":{"input_tokens":xxx,"output_tokens":xxx},"model":"qwen-turbo"}
	if usageData := gjson.GetBytes(body, "usage"); usageData.Exists() {
		usage.InputTokens = usageData.Get("input_tokens").Int()
		usage.OutputTokens = usageData.Get("output_tokens").Int()
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens // 部分版本不返回total，手动计算
		usage.Model = gjson.GetBytes(body, "model").String()
		return usage
	}

	// 兼容流式响应/老版本格式
	if choices := gjson.GetBytes(body, "choices"); choices.Exists() {
		usage.Model = gjson.GetBytes(body, "model").String()
		// 流式响应需从header或后续聚合，此处返回基础模型名
		return usage
	}

	return nil
}

// 月之暗面（Moonshot）
type Moonshot struct{}

func (m *Moonshot) ExtractTokenUsage(json gjson.Result, body []byte) *TokenUsage {
	usage := &TokenUsage{}

	// Moonshot响应格式：{"usage":{"prompt_tokens":xxx,"completion_tokens":xxx,"total_tokens":xxx},"model":"moonshot-v1-8k"}
	if usageData := gjson.GetBytes(body, "usage"); usageData.Exists() {
		usage.InputTokens = usageData.Get("prompt_tokens").Int()
		usage.OutputTokens = usageData.Get("completion_tokens").Int()
		usage.TotalTokens = usageData.Get("total_tokens").Int()
		usage.Model = gjson.GetBytes(body, "model").String()
		return usage
	}

	return nil
}

// 智谱AI（ZhipuAI）
type ZhipuAI struct{}

func (z *ZhipuAI) ExtractTokenUsage(json gjson.Result, body []byte) *TokenUsage {
	usage := &TokenUsage{}

	// 智谱响应格式：{"usage":{"prompt_tokens":xxx,"completion_tokens":xxx,"total_tokens":xxx},"model":"glm-4"}
	if usageData := gjson.GetBytes(body, "usage"); usageData.Exists() {
		usage.InputTokens = usageData.Get("prompt_tokens").Int()
		usage.OutputTokens = usageData.Get("completion_tokens").Int()
		usage.TotalTokens = usageData.Get("total_tokens").Int()
		usage.Model = gjson.GetBytes(body, "model").String()
		return usage
	}

	// 兼容智谱老版本格式
	if usageData := gjson.GetBytes(body, "data.usage"); usageData.Exists() {
		usage.InputTokens = usageData.Get("prompt_tokens").Int()
		usage.OutputTokens = usageData.Get("completion_tokens").Int()
		usage.TotalTokens = usageData.Get("total_tokens").Int()
		usage.Model = gjson.GetBytes(body, "data.model").String()
		return usage
	}

	return nil
}

// 百川智能（Baichuan）
type Baichuan struct{}

func (b *Baichuan) ExtractTokenUsage(json gjson.Result, body []byte) *TokenUsage {
	usage := &TokenUsage{}

	// 百川响应格式：{"usage":{"input_tokens":xxx,"output_tokens":xxx},"model":"baichuan2-13b-chat"}
	if usageData := gjson.GetBytes(body, "usage"); usageData.Exists() {
		usage.InputTokens = usageData.Get("input_tokens").Int()
		usage.OutputTokens = usageData.Get("output_tokens").Int()
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
		usage.Model = gjson.GetBytes(body, "model").String()
		return usage
	}

	return nil
}

// 零一万物（Yi）
type Yi struct{}

func (y *Yi) ExtractTokenUsage(json gjson.Result, body []byte) *TokenUsage {
	usage := &TokenUsage{}

	// 零一万物响应格式（兼容OpenAI）：{"usage":{"prompt_tokens":xxx,"completion_tokens":xxx,"total_tokens":xxx},"model":"yi-large"}
	if usageData := gjson.GetBytes(body, "usage"); usageData.Exists() {
		usage.InputTokens = usageData.Get("prompt_tokens").Int()
		usage.OutputTokens = usageData.Get("completion_tokens").Int()
		usage.TotalTokens = usageData.Get("total_tokens").Int()
		usage.Model = gjson.GetBytes(body, "model").String()
		return usage
	}

	return nil
}

// 百度文心一言（Baidu）
type Baidu struct{}

func (bd *Baidu) ExtractTokenUsage(json gjson.Result, body []byte) *TokenUsage {
	usage := &TokenUsage{}

	// 文心一言响应格式：{"usage":{"input_tokens":xxx,"output_tokens":xxx},"result":"...","id":"..."}
	if usageData := gjson.GetBytes(body, "usage"); usageData.Exists() {
		usage.InputTokens = usageData.Get("input_tokens").Int()
		usage.OutputTokens = usageData.Get("output_tokens").Int()
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
		// 模型名需从请求参数或响应header获取，响应体无直接字段时兼容填充
		usage.Model = gjson.GetBytes(body, "model").String()
		if usage.Model == "" {
			usage.Model = "ernie-bot" // 默认值
		}
		return usage
	}

	return nil
}

// 讯飞星火（Spark）
type Spark struct{}

func (s *Spark) ExtractTokenUsage(json gjson.Result, body []byte) *TokenUsage {
	usage := &TokenUsage{}

	// 讯飞星火响应格式：{"usage":{"prompt_tokens":xxx,"completion_tokens":xxx},"model":"spark-3.5"}
	if usageData := gjson.GetBytes(body, "usage"); usageData.Exists() {
		usage.InputTokens = usageData.Get("prompt_tokens").Int()
		usage.OutputTokens = usageData.Get("completion_tokens").Int()
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
		usage.Model = gjson.GetBytes(body, "model").String()
		return usage
	}

	// 兼容星火老版本格式
	if usageData := gjson.GetBytes(body, "data.usage"); usageData.Exists() {
		usage.InputTokens = usageData.Get("prompt_tokens").Int()
		usage.OutputTokens = usageData.Get("completion_tokens").Int()
		usage.TotalTokens = usageData.Get("total_tokens").Int()
		usage.Model = "spark-3.0" // 老版本默认
		return usage
	}

	return nil
}

// 腾讯混元（Hunyuan）
type Hunyuan struct{}

func (h *Hunyuan) ExtractTokenUsage(json gjson.Result, body []byte) *TokenUsage {
	usage := &TokenUsage{}

	// 混元响应格式（兼容OpenAI）：{"usage":{"prompt_tokens":xxx,"completion_tokens":xxx,"total_tokens":xxx},"model":"hunyuan-pro"}
	if usageData := gjson.GetBytes(body, "usage"); usageData.Exists() {
		usage.InputTokens = usageData.Get("prompt_tokens").Int()
		usage.OutputTokens = usageData.Get("completion_tokens").Int()
		usage.TotalTokens = usageData.Get("total_tokens").Int()
		usage.Model = gjson.GetBytes(body, "model").String()
		return usage
	}

	return nil
}

// MiniMax
type MiniMax struct{}

func (m *MiniMax) ExtractTokenUsage(json gjson.Result, body []byte) *TokenUsage {
	usage := &TokenUsage{}

	// MiniMax响应格式：{"usage":{"input_tokens":xxx,"output_tokens":xxx},"model":"abab5.5-chat"}
	if usageData := gjson.GetBytes(body, "usage"); usageData.Exists() {
		usage.InputTokens = usageData.Get("input_tokens").Int()
		usage.OutputTokens = usageData.Get("output_tokens").Int()
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
		usage.Model = gjson.GetBytes(body, "model").String()
		return usage
	}

	return nil
}

// 360智脑
type AI360 struct{}

func (a *AI360) ExtractTokenUsage(json gjson.Result, body []byte) *TokenUsage {
	usage := &TokenUsage{}

	// 360智脑响应格式：{"usage":{"prompt_tokens":xxx,"completion_tokens":xxx},"model":"360zhinao-pro"}
	if usageData := gjson.GetBytes(body, "usage"); usageData.Exists() {
		usage.InputTokens = usageData.Get("prompt_tokens").Int()
		usage.OutputTokens = usageData.Get("completion_tokens").Int()
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
		usage.Model = gjson.GetBytes(body, "model").String()
		return usage
	}

	return nil
}

// 阶跃星辰（Stepfun）
type Stepfun struct{}

func (s *Stepfun) ExtractTokenUsage(json gjson.Result, body []byte) *TokenUsage {
	usage := &TokenUsage{}

	// 阶跃星辰响应格式（兼容OpenAI）：{"usage":{"prompt_tokens":xxx,"completion_tokens":xxx,"total_tokens":xxx},"model":"stepfun-7b"}
	if usageData := gjson.GetBytes(body, "usage"); usageData.Exists() {
		usage.InputTokens = usageData.Get("prompt_tokens").Int()
		usage.OutputTokens = usageData.Get("completion_tokens").Int()
		usage.TotalTokens = usageData.Get("total_tokens").Int()
		usage.Model = gjson.GetBytes(body, "model").String()
		return usage
	}

	return nil
}

// Anthropic Claude
type Claude struct{}

func (c *Claude) ExtractTokenUsage(json gjson.Result, body []byte) *TokenUsage {
	usage := &TokenUsage{}

	// Claude响应格式：{"usage":{"input_tokens":xxx,"output_tokens":xxx,"total_tokens":xxx},"model":"claude-3-opus-20240229"}
	if usageData := gjson.GetBytes(body, "usage"); usageData.Exists() {
		usage.InputTokens = usageData.Get("input_tokens").Int()
		usage.OutputTokens = usageData.Get("output_tokens").Int()
		usage.TotalTokens = usageData.Get("total_tokens").Int()
		usage.Model = gjson.GetBytes(body, "model").String()
		return usage
	}

	// 兼容Claude老版本格式
	if usageData := gjson.GetBytes(body, "usage_metadata"); usageData.Exists() {
		usage.InputTokens = usageData.Get("input_tokens").Int()
		usage.OutputTokens = usageData.Get("output_tokens").Int()
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
		usage.Model = gjson.GetBytes(body, "model").String()
		return usage
	}

	return nil
}

// Groq
type Groq struct{}

func (g *Groq) ExtractTokenUsage(json gjson.Result, body []byte) *TokenUsage {
	usage := &TokenUsage{}

	// Groq响应格式（兼容OpenAI）：{"usage":{"prompt_tokens":xxx,"completion_tokens":xxx,"total_tokens":xxx},"model":"llama3-70b-8192"}
	if usageData := gjson.GetBytes(body, "usage"); usageData.Exists() {
		usage.InputTokens = usageData.Get("prompt_tokens").Int()
		usage.OutputTokens = usageData.Get("completion_tokens").Int()
		usage.TotalTokens = usageData.Get("total_tokens").Int()
		usage.Model = gjson.GetBytes(body, "model").String()
		return usage
	}

	return nil
}

// Mistral
type Mistral struct{}

func (m *Mistral) ExtractTokenUsage(json gjson.Result, body []byte) *TokenUsage {
	usage := &TokenUsage{}

	// Mistral响应格式（兼容OpenAI）：{"usage":{"prompt_tokens":xxx,"completion_tokens":xxx,"total_tokens":xxx},"model":"mistral-large-latest"}
	if usageData := gjson.GetBytes(body, "usage"); usageData.Exists() {
		usage.InputTokens = usageData.Get("prompt_tokens").Int()
		usage.OutputTokens = usageData.Get("completion_tokens").Int()
		usage.TotalTokens = usageData.Get("total_tokens").Int()
		usage.Model = gjson.GetBytes(body, "model").String()
		return usage
	}

	return nil
}

// Google Gemini
type Gemini struct{}

func (g *Gemini) ExtractTokenUsage(json gjson.Result, body []byte) *TokenUsage {
	usage := &TokenUsage{}

	// Gemini响应格式：{"usageMetadata":{"promptTokenCount":xxx,"candidatesTokenCount":xxx,"totalTokenCount":xxx},"model":"gemini-pro"}
	if usageData := gjson.GetBytes(body, "usageMetadata"); usageData.Exists() {
		usage.InputTokens = usageData.Get("promptTokenCount").Int()
		usage.OutputTokens = usageData.Get("candidatesTokenCount").Int()
		usage.TotalTokens = usageData.Get("totalTokenCount").Int()
		usage.Model = gjson.GetBytes(body, "model").String()
		return usage
	}

	return nil
}

// Ollama（本地开源模型）
type Ollama struct{}

func (o *Ollama) ExtractTokenUsage(json gjson.Result, body []byte) *TokenUsage {
	usage := &TokenUsage{}

	// Ollama响应格式：{"usage":{"prompt_tokens":xxx,"completion_tokens":xxx,"total_tokens":xxx},"model":"llama3:8b"}
	if usageData := gjson.GetBytes(body, "usage"); usageData.Exists() {
		usage.InputTokens = usageData.Get("prompt_tokens").Int()
		usage.OutputTokens = usageData.Get("completion_tokens").Int()
		usage.TotalTokens = usageData.Get("total_tokens").Int()
		usage.Model = gjson.GetBytes(body, "model").String()
		return usage
	}

	return nil
}

// DeepL（翻译模型）
type DeepL struct{}

func (d *DeepL) ExtractTokenUsage(json gjson.Result, body []byte) *TokenUsage {
	usage := &TokenUsage{}

	// DeepL响应格式：{"character_count":xxx,"word_count":xxx,"sentence_count":xxx}
	// DeepL按字符计费，需转换为Token（粗略：1 Token ≈ 1.3字符）
	if charCount := gjson.GetBytes(body, "character_count").Int(); charCount > 0 {
		// 第一步：先做浮点数运算
		inputFloat := float64(charCount) / 1.3 // 转为float64计算，避免精度丢失
		outputFloat := float64(gjson.GetBytes(body, "character_count_target").Int()) / 1.3

		// 第二步：显式转换为int64（明确处理小数，推荐四舍五入，也可根据需求取整）
		usage.InputTokens = int64(math.Round(inputFloat)) // 四舍五入（更合理的计费方式）
		usage.OutputTokens = int64(math.Round(outputFloat))
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens

		usage.Model = gjson.GetBytes(body, "model").String()
		if usage.Model == "" {
			usage.Model = "deepl-pro"
		}
		return usage
	}

	return nil
}

// Cohere
type Cohere struct{}

func (c *Cohere) ExtractTokenUsage(json gjson.Result, body []byte) *TokenUsage {
	usage := &TokenUsage{}

	// Cohere响应格式：{"meta":{"tokens":{"input_tokens":xxx,"output_tokens":xxx}},"model":"command-r-plus"}
	if metaData := gjson.GetBytes(body, "meta"); metaData.Exists() {
		tokenData := metaData.Get("tokens")
		usage.InputTokens = tokenData.Get("input_tokens").Int()
		usage.OutputTokens = tokenData.Get("output_tokens").Int()
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
		usage.Model = gjson.GetBytes(body, "model").String()
		return usage
	}

	return nil
}

// Cloudflare Workers AI
type Cloudflare struct{}

func (cf *Cloudflare) ExtractTokenUsage(json gjson.Result, body []byte) *TokenUsage {
	usage := &TokenUsage{}

	// Cloudflare响应格式（简化）：{"result":"...","usage":{"input_tokens":xxx,"output_tokens":xxx}}
	if usageData := gjson.GetBytes(body, "usage"); usageData.Exists() {
		usage.InputTokens = usageData.Get("input_tokens").Int()
		usage.OutputTokens = usageData.Get("output_tokens").Int()
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
		// 模型名需从请求参数获取，响应体无则填充默认
		usage.Model = gjson.GetBytes(body, "model").String()
		if usage.Model == "" {
			usage.Model = "cloudflare/llama-3-7b-instruct"
		}
		return usage
	}

	return nil
}

// TogetherAI
type TogetherAI struct{}

func (t *TogetherAI) ExtractTokenUsage(json gjson.Result, body []byte) *TokenUsage {
	usage := &TokenUsage{}

	// TogetherAI响应格式（兼容OpenAI）：{"usage":{"prompt_tokens":xxx,"completion_tokens":xxx,"total_tokens":xxx},"model":"meta-llama/Llama-3-70b-chat-hf"}
	if usageData := gjson.GetBytes(body, "usage"); usageData.Exists() {
		usage.InputTokens = usageData.Get("prompt_tokens").Int()
		usage.OutputTokens = usageData.Get("completion_tokens").Int()
		usage.TotalTokens = usageData.Get("total_tokens").Int()
		usage.Model = gjson.GetBytes(body, "model").String()
		return usage
	}

	return nil
}

// DeepSeek
type DeepSeek struct{}

func (d *DeepSeek) ExtractTokenUsage(json gjson.Result, body []byte) *TokenUsage {
	usage := &TokenUsage{}

	// DeepSeek响应格式（兼容OpenAI）：{"usage":{"prompt_tokens":xxx,"completion_tokens":xxx,"total_tokens":xxx},"model":"deepseek-chat"}
	if usageData := gjson.GetBytes(body, "usage"); usageData.Exists() {
		usage.InputTokens = usageData.Get("prompt_tokens").Int()
		usage.OutputTokens = usageData.Get("completion_tokens").Int()
		usage.TotalTokens = usageData.Get("total_tokens").Int()
		usage.Model = gjson.GetBytes(body, "model").String()
		return usage
	}

	return nil
}
