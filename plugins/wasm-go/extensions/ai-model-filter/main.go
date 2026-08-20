package main

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

func main() {}

func init() {
	wrapper.SetCtx(
		"ai-model-filter",
		wrapper.ParseConfig(parseConfig),
		wrapper.ProcessRequestHeaders(onHttpRequestHeaders),
		wrapper.ProcessRequestBody(onHttpRequestBody),
	)
}

const (
	defaultMaxBodyBytes uint32 = 100 * 1024 * 1024
	RequestPath                = "request_path"
)

// AIModelFilterConfig 配置结构
type AIModelFilterConfig struct {
	// 允许的模型列表
	allowedModels []string
	// 是否启用严格模式（默认为true，拒绝不在列表中的模型）
	strictMode bool
	// 自定义拒绝响应消息
	rejectMessage string
	// 自定义拒绝响应状态码
	rejectStatusCode int
}

// parseConfig 解析配置
func parseConfig(configJson gjson.Result, config *AIModelFilterConfig) error {
	// 解析允许的模型列表
	allowedModelsArray := configJson.Get("allowed_models").Array()
	config.allowedModels = make([]string, len(allowedModelsArray))
	for i, model := range allowedModelsArray {
		config.allowedModels[i] = model.String()
	}

	// 解析严格模式设置，默认为true
	config.strictMode = configJson.Get("strict_mode").Bool()
	if !configJson.Get("strict_mode").Exists() {
		config.strictMode = true
	}

	// 解析自定义拒绝消息，默认消息
	config.rejectMessage = configJson.Get("reject_message").String()
	if config.rejectMessage == "" {
		config.rejectMessage = "Model not allowed"
	}

	// 解析自定义拒绝状态码，默认403
	config.rejectStatusCode = int(configJson.Get("reject_status_code").Int())
	if config.rejectStatusCode == 0 {
		config.rejectStatusCode = 403
	}

	log.Infof("AI Model Filter Config: allowed_models=%v, strict_mode=%v, reject_message=%s, reject_status_code=%d",
		config.allowedModels, config.strictMode, config.rejectMessage, config.rejectStatusCode)

	return nil
}

// onHttpRequestHeaders 处理请求头
func onHttpRequestHeaders(ctx wrapper.HttpContext, config AIModelFilterConfig) types.Action {
	ctx.DisableReroute()

	// 获取请求路径并保存到上下文
	if requestPath, _ := proxywasm.GetHttpRequestHeader(":path"); requestPath != "" {
		ctx.SetContext(RequestPath, requestPath)
	}

	// 设置请求体缓冲区限制
	ctx.SetRequestBodyBufferLimit(defaultMaxBodyBytes)

	return types.ActionContinue
}

// onHttpRequestBody 处理请求体
func onHttpRequestBody(ctx wrapper.HttpContext, config AIModelFilterConfig, body []byte) types.Action {
	// 如果没有配置允许的模型列表，直接通过
	if len(config.allowedModels) == 0 {

		log.Info("No allowed models configured, allowing all requests")

		return types.ActionContinue
	}

	// 提取模型名称
	requestModel := extractModelName(ctx, body)
	if requestModel == "" {
		log.Warn("Could not extract model name from request")
		// 如果无法提取模型名称且启用严格模式，拒绝请求
		if config.strictMode {
			return rejectRequest(ctx, config, "Could not determine model name")
		}
		return types.ActionContinue
	}

	log.Infof("Extracted model name: %s", requestModel)

	// 检查模型是否在允许列表中
	if !isModelAllowed(requestModel, config.allowedModels) {
		log.Warnf("Model '%s' is not in the allowed list: %v", requestModel, config.allowedModels)
		return rejectRequest(ctx, config, "Model not allowed: "+requestModel)
	}

	log.Infof("Model '%s' is allowed, continuing request", requestModel)

	return types.ActionContinue
}

// extractModelName 从请求中提取模型名称
func extractModelName(ctx wrapper.HttpContext, body []byte) string {
	// 首先尝试从请求体的model字段获取
	if model := gjson.GetBytes(body, "model"); model.Exists() {
		return model.String()
	}

	// 如果请求体中没有model字段，尝试从URL路径中提取（适用于Google Gemini等API）
	requestPath := ctx.GetStringContext(RequestPath, "")
	if requestPath != "" {
		// Google Gemini GenerateContent API模式
		if strings.Contains(requestPath, "generateContent") || strings.Contains(requestPath, "streamGenerateContent") {
			reg := regexp.MustCompile(`^.*/(?P<api_version>[^/]+)/models/(?P<model>[^:]+):\w+Content$`)
			matches := reg.FindStringSubmatch(requestPath)
			if len(matches) == 3 {
				return matches[2]
			}
		}

		// OpenAI API模式 (例如: /v1/chat/completions)
		// 对于OpenAI API，模型名称通常在请求体中，这里已经在上面处理了

		// Anthropic API模式 (例如: /v1/messages)
		// 对于Anthropic API，模型名称也通常在请求体中

		// 可以根据需要添加更多API提供商的URL模式匹配
	}

	return ""
}

// isModelAllowed 检查模型是否在允许列表中
func isModelAllowed(modelName string, allowedModels []string) bool {
	for _, allowedModel := range allowedModels {
		// 支持精确匹配
		if modelName == allowedModel {
			return true
		}
		// 支持通配符匹配（简单的前缀匹配）
		if strings.HasSuffix(allowedModel, "*") {
			prefix := strings.TrimSuffix(allowedModel, "*")
			if strings.HasPrefix(modelName, prefix) {
				return true
			}
		}
	}
	return false
}

// rejectRequest 拒绝请求
func rejectRequest(ctx wrapper.HttpContext, config AIModelFilterConfig, reason string) types.Action {
	log.Warnf("Rejecting request: %s", reason)

	// 构造错误响应体
	errorResponse := map[string]interface{}{
		"error": map[string]interface{}{
			"message": config.rejectMessage,
			"type":    "model_not_allowed",
			"code":    "model_filter_rejected",
			"details": reason,
		},
	}

	errorBody, err := json.Marshal(errorResponse)
	if err != nil {
		log.Errorf("Failed to marshal error response: %v", err)
		errorBody = []byte(`{"error":{"message":"Model not allowed","type":"model_not_allowed"}}`)
	}

	// 发送错误响应
	proxywasm.SendHttpResponse(uint32(config.rejectStatusCode), [][2]string{
		{"content-type", "application/json"},
		{"cache-control", "no-cache"},
	}, errorBody, -1)

	return types.ActionPause
}
