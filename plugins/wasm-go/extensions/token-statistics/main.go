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
	"regexp"
	"strings"
	"time"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

const (
	pluginName     = "token-statistics"
	RequestPath    = "request_path"
	SkipProcessing = "skip_processing"
)

func main() {
}

func init() {
	wrapper.SetCtx(
		pluginName,
		wrapper.ParseConfig(parseConfig),
		wrapper.ProcessRequestHeaders(onHttpRequestHeaders),
		wrapper.ProcessRequestBody(onHttpRequestBody),
		wrapper.ProcessResponseHeaders(onHttpResponseHeaders),
		wrapper.ProcessStreamingResponseBody(onHttpStreamingBody),
		wrapper.ProcessResponseBody(onHttpResponseBody),
	)
}

type TokenUsage struct {
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	TotalTokens  int64  `json:"total_tokens"`
	Model        string `json:"model"`
}
type Dimension struct {
	// 维度名称
	Name string `json:"name"`
	// 维度值来源，支持 "header", "query", "path", "constant"
	ValueSource string `json:"value_source"`
	// 维度值
	Value string `json:"value"`
	// 默认值
	DefaultValue string `json:"default_value"`
	// 规则，例如正则表达式
	Rule string `json:"rule"`
	// 是否应用到日志
	ApplyToLog bool `json:"apply_to_log"`
	// 是否应用到指标
	ApplyToMetric bool `json:"apply_to_metric"`
	// 是否作为单独的日志字段输出
	AsSeparateLogField bool `json:"as_separate_log_field"`
}

type ExporterConfig struct {
	// 导出器类型，例如 "log", "metric"
	Type string `json:"type"`
	// 其他配置项
	Config map[string]interface{} `json:"config"`
}

type PluginConfig struct {
	// 统计维度配置
	Dimensions []Dimension `json:"dimensions"`
	// 输出配置
	Exporters []ExporterConfig `json:"exporters"`
	// 路径过滤配置
	EnablePathSuffixes []string `json:"enable_path_suffixes"`
	// 内容类型过滤配置
	EnableContentTypes []string `json:"enable_content_types"`
	// 是否禁用OpenAI使用量统计（用于非标准协议）
	DisableOpenAIUsage bool `json:"disable_openai_usage"`
	// 值长度限制
	ValueLengthLimit int `json:"value_length_limit"`
}

func parseConfig(json gjson.Result, config *PluginConfig) error {
	// 解析dimensions配置
	dimensionConfigs := json.Get("dimensions").Array()
	config.Dimensions = make([]Dimension, len(dimensionConfigs))
	for i, dimConfig := range dimensionConfigs {
		dim := Dimension{}
		dim.Name = dimConfig.Get("name").String()
		dim.ValueSource = dimConfig.Get("value_source").String()
		dim.Value = dimConfig.Get("value").String()
		dim.DefaultValue = dimConfig.Get("default_value").String()
		dim.Rule = dimConfig.Get("rule").String()
		dim.ApplyToLog = dimConfig.Get("apply_to_log").Bool()
		dim.ApplyToMetric = dimConfig.Get("apply_to_metric").Bool()
		dim.AsSeparateLogField = dimConfig.Get("as_separate_log_field").Bool()
		config.Dimensions[i] = dim
	}

	// 解析exporters配置
	exporterConfigs := json.Get("exporters").Array()
	config.Exporters = make([]ExporterConfig, len(exporterConfigs))
	for i, expConfig := range exporterConfigs {
		exp := ExporterConfig{}
		exp.Type = expConfig.Get("type").String()
		exp.Config = make(map[string]interface{})
		expConfig.Get("config").ForEach(func(key, value gjson.Result) bool {
			exp.Config[key.String()] = value.Value()
			return true
		})
		config.Exporters[i] = exp
	}

	// 解析其他配置项
	enablePathSuffixes := json.Get("enable_path_suffixes").Array()
	for _, suffix := range enablePathSuffixes {
		config.EnablePathSuffixes = append(config.EnablePathSuffixes, suffix.String())
	}

	enableContentTypes := json.Get("enable_content_types").Array()
	for _, ctype := range enableContentTypes {
		config.EnableContentTypes = append(config.EnableContentTypes, ctype.String())
	}

	config.DisableOpenAIUsage = json.Get("disable_openai_usage").Bool()
	config.ValueLengthLimit = int(json.Get("value_length_limit").Int())
	return nil
}

// isPathEnabled checks if the request path matches any of the enabled path suffixes
func isPathEnabled(requestPath string, enabledSuffixes []string) bool {
	if len(enabledSuffixes) == 0 {
		return true // If no path suffixes configured, enable for all
	}

	// Remove query parameters from path
	pathWithoutQuery := requestPath
	if queryPos := strings.Index(requestPath, "?"); queryPos != -1 {
		pathWithoutQuery = requestPath[:queryPos]
	}

	// Check if path ends with any enabled suffix
	for _, suffix := range enabledSuffixes {
		if strings.HasSuffix(pathWithoutQuery, suffix) {
			return true
		}
	}
	return false
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config PluginConfig) types.Action {
	// 记录请求开始时间
	ctx.SetContext("request_start_time", time.Now())

	// 提取请求路径
	path, _ := proxywasm.GetHttpRequestHeader(":path")
	ctx.SetContext("request_path", path)

	// 检查路径过滤
	if !isPathEnabled(path, config.EnablePathSuffixes) {
		ctx.SetContext("skip_processing", true)
		ctx.DontReadRequestBody()
		ctx.DontReadResponseBody()
		return types.ActionContinue
	}

	return types.ActionContinue
}

func onHttpRequestBody(ctx wrapper.HttpContext, config PluginConfig, body []byte) types.Action {
	// 检查是否跳过处理
	if ctx.GetBoolContext("skip_processing", false) {
		return types.ActionContinue
	}

	// 提取模型信息
	requestModel := "UNKNOWN"
	if model := gjson.GetBytes(body, "model"); model.Exists() {
		requestModel = model.String()
	} else {
		requestPath := ctx.GetStringContext(RequestPath, "")
		if strings.Contains(requestPath, "generateContent") || strings.Contains(requestPath, "streamGenerateContent") { // Google Gemini GenerateContent
			reg := regexp.MustCompile(`^.*/(?P<api_version>[^/]+)/models/(?P<model>[^:]+):\w+Content$`)
			matches := reg.FindStringSubmatch(requestPath)
			if len(matches) == 3 {
				requestModel = matches[2]
			}
		}
	}
	ctx.SetContext("request_model", requestModel)
	return types.ActionContinue
}

// func onHttpResponseHeaders(ctx wrapper.HttpContext, c *PluginConfig, log wrapper.Log) types.Action {
func onHttpResponseHeaders(ctx wrapper.HttpContext, config PluginConfig) types.Action {
	contentType, _ := proxywasm.GetHttpResponseHeader("content-type")
	if !strings.Contains(contentType, "text/event-stream") {
		ctx.BufferResponseBody()
	}

	// Record cache hit/miss statistics if cache_status attribute is available
	if cacheStatus := ctx.GetUserAttribute("cache_status"); cacheStatus != "" {
		if status, ok := cacheStatus.(string); ok {
			cacheExporter := NewCacheExporter("higress", "ai_cache")
			cacheExporter.ExportCacheStatus(status)
		}
	}

	return types.ActionContinue
}

func onHttpResponseBody(ctx wrapper.HttpContext, config PluginConfig, body []byte) types.Action {
	// Check if processing should be skipped
	if ctx.GetBoolContext(SkipProcessing, false) {
		return types.ActionContinue
	}

	return types.ActionContinue
}

func onHttpStreamingBody(ctx wrapper.HttpContext, config PluginConfig, data []byte, endOfStream bool) []byte {
	// 检查是否跳过处理
	if ctx.GetBoolContext("skip_processing", false) {
		return data
	}

	// 累积流式数据
	if !endOfStream {
		// 保存流式数据片段供后续处理
		accumulatedData, _ := ctx.GetContext("accumulated_stream_data").([]byte)
		accumulatedData = append(accumulatedData, data...)
		ctx.SetContext("accumulated_stream_data", accumulatedData)
		return data
	}

	// 处理最后一个数据块
	accumulatedData, _ := ctx.GetContext("accumulated_stream_data").([]byte)
	accumulatedData = append(accumulatedData, data...)

	// 从累积的数据中提取Token使用量
	model := ctx.GetStringContext("request_model", "UNKNOWN")
	usage := extractStreamingTokenUsage(model, accumulatedData)
	if usage != nil {
		// 记录统计信息
		recordTokenUsage(ctx, model, usage)
	}

	return data
}

// 从流式响应中提取Token使用量
func extractStreamingTokenUsage(model string, data []byte) *TokenUsage {
	// 统一转为小写，兼容大小写输入（如"OpenAI"="openai"）
	normalizedType := strings.TrimSpace(strings.ToLower(model))

	// 处理别名（兼容不同命名习惯）
	switch normalizedType {
	case "azureopenai", "azure_openai":
		normalizedType = "azure"
	case "zhipu", "chatglm":
		normalizedType = "zhipuai"
	case "baidu", "ernie":
		normalizedType = "baidu"
	case "sparkai", "xunfei":
		normalizedType = "spark"
	case "hunyuanai", "tencent":
		normalizedType = "hunyuan"
	case "360", "360zhinao":
		normalizedType = "ai360"
	case "stepfunai", "jieyue":
		normalizedType = "stepfun"
	case "anthropic", "claudeai":
		normalizedType = "claude"
	case "together", "together-ai":
		normalizedType = "togetherai"
	case "cloudflareai", "cfai":
		normalizedType = "cloudflare"
	}

	// 核心类型映射（覆盖所有供应商）
	switch normalizedType {
	// 基础类型
	case "openai":
		openAIExporter := &OpenAI{}
		return openAIExporter.ExtractTokenUsage(gjson.ParseBytes(data), data)
	case "azure":
		azureaiExporter := &AzureOpenAI{}
		return azureaiExporter.ExtractTokenUsage(gjson.ParseBytes(data), data)

	// 国内厂商
	case "qwen":
		qwenExporter := &Qwen{}
		return qwenExporter.ExtractTokenUsage(gjson.ParseBytes(data), data)
	case "moonshot":
		moonshotExporter := &Moonshot{}
		return moonshotExporter.ExtractTokenUsage(gjson.ParseBytes(data), data)
	case "zhipuai":
		zhipuExporter := &ZhipuAI{}
		return zhipuExporter.ExtractTokenUsage(gjson.ParseBytes(data), data)
	case "baichuan":
		baichuanExporter := &Baichuan{}
		return baichuanExporter.ExtractTokenUsage(gjson.ParseBytes(data), data)
	case "yi":
		yiExporter := &Yi{}
		return yiExporter.ExtractTokenUsage(gjson.ParseBytes(data), data)
	case "baidu":
		baiduExporter := &Baidu{}
		return baiduExporter.ExtractTokenUsage(gjson.ParseBytes(data), data)
	case "spark":
		sparkExporter := &Spark{}
		return sparkExporter.ExtractTokenUsage(gjson.ParseBytes(data), data)
	case "hunyuan":
		hunyuanExporter := &Hunyuan{}
		return hunyuanExporter.ExtractTokenUsage(gjson.ParseBytes(data), data)
	case "minimax":
		minimaxExporter := &MiniMax{}
		return minimaxExporter.ExtractTokenUsage(gjson.ParseBytes(data), data)
	case "ai360":
		ai360Exporter := &AI360{}
		return ai360Exporter.ExtractTokenUsage(gjson.ParseBytes(data), data)
	case "stepfun":
		stepfunExporter := &Stepfun{}
		return stepfunExporter.ExtractTokenUsage(gjson.ParseBytes(data), data)
	case "deepseek":
		deepseekExporter := &DeepSeek{}
		return deepseekExporter.ExtractTokenUsage(gjson.ParseBytes(data), data)

	// 国外厂商
	case "claude":
		claudeExporter := &Claude{}
		return claudeExporter.ExtractTokenUsage(gjson.ParseBytes(data), data)
	case "groq":
		groqExporter := &Groq{}
		return groqExporter.ExtractTokenUsage(gjson.ParseBytes(data), data)
	case "mistral":
		mistralExporter := &Mistral{}
		return mistralExporter.ExtractTokenUsage(gjson.ParseBytes(data), data)
	case "gemini":
		geminiExporter := &Gemini{}
		return geminiExporter.ExtractTokenUsage(gjson.ParseBytes(data), data)
	case "ollama":
		ollamaExporter := &Ollama{}
		return ollamaExporter.ExtractTokenUsage(gjson.ParseBytes(data), data)
	case "deepl":
		deeplExporter := &DeepL{}
		return deeplExporter.ExtractTokenUsage(gjson.ParseBytes(data), data)
	case "cohere":
		cohereExporter := &Cohere{}
		return cohereExporter.ExtractTokenUsage(gjson.ParseBytes(data), data)
	case "cloudflare":
		cloudflareExporter := &Cloudflare{}
		return cloudflareExporter.ExtractTokenUsage(gjson.ParseBytes(data), data)
	case "togetherai":
		togetheraiExporter := &TogetherAI{}
		return togetheraiExporter.ExtractTokenUsage(gjson.ParseBytes(data), data)
	default:
		return nil
	}
}

func recordTokenUsage(ctx wrapper.HttpContext, model string, usage *TokenUsage) {
	// 记录日志
	logExporter := &LogExporter{level: "info"}
	logExporter.Export(ctx, model, usage)

	// 记录指标
	promExporter := &PrometheusExporter{namespace: "higress", subsystem: "token_statistics", model: model}
	promExporter.Export(usage)
}
