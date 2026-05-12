package provider

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-proxy/util"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	vertexAuthDomain = "oauth2.googleapis.com"
	vertexDomain     = "aiplatform.googleapis.com"
	// /v1/projects/{PROJECT_ID}/locations/{REGION}/publishers/google/models/{MODEL_ID}:{ACTION}
	vertexPathTemplate          = "/v1/projects/%s/locations/%s/publishers/google/models/%s:%s"
	vertexPathAnthropicTemplate = "/v1/projects/%s/locations/%s/publishers/anthropic/models/%s:%s"
	// Express Mode 路径模板 (不含 project/location)
	vertexExpressPathTemplate          = "/v1/publishers/google/models/%s:%s"
	vertexExpressPathAnthropicTemplate = "/v1/publishers/anthropic/models/%s:%s"
	// OpenAI-compatible endpoint 路径模板
	// /v1beta1/projects/{PROJECT_ID}/locations/{LOCATION}/endpoints/openapi/chat/completions
	vertexOpenAICompatiblePathTemplate = "/v1beta1/projects/%s/locations/%s/endpoints/openapi/chat/completions"
	vertexChatCompletionAction         = "generateContent"
	vertexChatCompletionStreamAction   = "streamGenerateContent?alt=sse"
	vertexAnthropicMessageAction       = "rawPredict"
	vertexAnthropicMessageStreamAction = "streamRawPredict"
	vertexEmbeddingAction              = "predict"
	vertexGlobalRegion                 = "global"
	contextClaudeMarker                = "isClaudeRequest"
	contextOpenAICompatibleMarker      = "isOpenAICompatibleRequest"
	contextVertexRawMarker             = "isVertexRawRequest"
	contextVertexStreamDoneMarker      = "vertexStreamDoneSent"
	vertexAnthropicVersion             = "vertex-2023-10-16"
	vertexImageVariationDefaultPrompt  = "Create variations of the provided image."
)

// vertexRawPathRegex 匹配原生 Vertex AI REST API 路径
// 格式: [任意前缀]/{api-version}/projects/{project}/locations/{location}/publishers/{publisher}/models/{model}:{action}
// 允许任意 basePath 前缀，兼容 basePathHandling 配置
var vertexRawPathRegex = regexp.MustCompile(`^.*/([^/]+)/projects/([^/]+)/locations/([^/]+)/publishers/([^/]+)/models/([^/:]+):([^/?]+)`)

type vertexProviderInitializer struct{}

func (v *vertexProviderInitializer) ValidateConfig(config *ProviderConfig) error {
	// Express Mode: 如果配置了 apiTokens，则使用 API Key 认证
	if len(config.apiTokens) > 0 {
		// Express Mode 与 OpenAI 兼容模式互斥
		if config.vertexOpenAICompatible {
			return errors.New("vertexOpenAICompatible is not compatible with Express Mode (apiTokens)")
		}
		// Express Mode 不需要其他配置
		return nil
	}

	// OpenAI 兼容模式: 需要 OAuth 认证配置
	if config.vertexOpenAICompatible {
		if config.vertexAuthKey == "" {
			return errors.New("missing vertexAuthKey in vertex provider config for OpenAI compatible mode")
		}
		if config.vertexRegion == "" || config.vertexProjectId == "" {
			return errors.New("missing vertexRegion or vertexProjectId in vertex provider config for OpenAI compatible mode")
		}
		if config.vertexAuthServiceName == "" {
			return errors.New("missing vertexAuthServiceName in vertex provider config for OpenAI compatible mode")
		}
		return nil
	}

	// 标准模式: 保持原有验证逻辑
	if config.vertexAuthKey == "" {
		return errors.New("missing vertexAuthKey in vertex provider config")
	}
	if config.vertexRegion == "" || config.vertexProjectId == "" {
		return errors.New("missing vertexRegion or vertexProjectId in vertex provider config")
	}
	if config.vertexAuthServiceName == "" {
		return errors.New("missing vertexAuthServiceName in vertex provider config")
	}
	return nil
}

func (v *vertexProviderInitializer) DefaultCapabilities() map[string]string {
	return map[string]string{
		string(ApiNameChatCompletion):  vertexPathTemplate,
		string(ApiNameEmbeddings):      vertexPathTemplate,
		string(ApiNameImageGeneration): vertexPathTemplate,
		string(ApiNameImageEdit):       vertexPathTemplate,
		string(ApiNameImageVariation):  vertexPathTemplate,
		string(ApiNameVertexRaw):       "", // 空字符串表示保持原路径，不做路径转换
	}
}

func (v *vertexProviderInitializer) CreateProvider(config ProviderConfig) (Provider, error) {
	config.setDefaultCapabilities(v.DefaultCapabilities())

	provider := &vertexProvider{
		config:       config,
		contextCache: createContextCache(&config),
		claude: &claudeProvider{
			config:       config,
			contextCache: createContextCache(&config),
		},
	}

	// 仅标准模式需要 OAuth 客户端（Express Mode 通过 apiTokens 配置）
	if !provider.isExpressMode() {
		provider.client = wrapper.NewClusterClient(wrapper.DnsCluster{
			Domain:      vertexAuthDomain,
			ServiceName: config.vertexAuthServiceName,
			Port:        443,
		})
	}

	return provider, nil
}

// isExpressMode 检测是否启用 Express Mode
// 如果配置了 apiTokens，则使用 Express Mode（API Key 认证）
func (v *vertexProvider) isExpressMode() bool {
	return len(v.config.apiTokens) > 0
}

// isOpenAICompatibleMode 检测是否启用 OpenAI 兼容模式
// 使用 Vertex AI 的 OpenAI-compatible Chat Completions API
func (v *vertexProvider) isOpenAICompatibleMode() bool {
	return v.config.vertexOpenAICompatible
}

type vertexProvider struct {
	client       wrapper.HttpClient
	config       ProviderConfig
	contextCache *contextCache
	claude       *claudeProvider
}

func (v *vertexProvider) GetProviderType() string {
	return providerTypeVertex
}

func (v *vertexProvider) GetApiName(path string) ApiName {
	// 优先匹配原生 Vertex AI REST API 路径，支持任意 basePath 前缀
	// 格式: [任意前缀]/{api-version}/projects/{project}/locations/{location}/publishers/{publisher}/models/{model}:{action}
	// 必须在其他 action 检查之前，因为 :predict、:generateContent 等 action 会被其他规则匹配
	if vertexRawPathRegex.MatchString(path) {
		return ApiNameVertexRaw
	}
	if strings.HasSuffix(path, vertexChatCompletionAction) || strings.HasSuffix(path, vertexChatCompletionStreamAction) {
		return ApiNameChatCompletion
	}
	if strings.HasSuffix(path, vertexEmbeddingAction) {
		return ApiNameEmbeddings
	}
	return ""
}

func (v *vertexProvider) OnRequestHeaders(ctx wrapper.HttpContext, apiName ApiName) error {
	v.config.handleRequestHeaders(v, ctx, apiName)
	return nil
}

func (v *vertexProvider) TransformRequestHeaders(ctx wrapper.HttpContext, apiName ApiName, headers http.Header) {
	var finalVertexDomain string

	if v.isExpressMode() {
		// Express Mode: 固定域名，不带 region 前缀
		finalVertexDomain = vertexDomain
	} else {
		// 标准模式: 带 region 前缀
		if v.config.vertexRegion != vertexGlobalRegion {
			finalVertexDomain = fmt.Sprintf("%s-%s", v.config.vertexRegion, vertexDomain)
		} else {
			finalVertexDomain = vertexDomain
		}
	}

	util.OverwriteRequestHostHeader(headers, finalVertexDomain)
}

func (v *vertexProvider) getToken() (cached bool, err error) {
	cacheKeyName := v.buildTokenKey()
	cachedAccessToken, err := v.getCachedAccessToken(cacheKeyName)
	if err == nil && cachedAccessToken != "" {
		_ = proxywasm.ReplaceHttpRequestHeader("Authorization", "Bearer "+cachedAccessToken)
		return true, nil
	}

	var key ServiceAccountKey
	if err := json.Unmarshal([]byte(v.config.vertexAuthKey), &key); err != nil {
		return false, fmt.Errorf("[vertex]: unable to unmarshal auth key json: %v", err)
	}

	if key.ClientEmail == "" || key.PrivateKey == "" || key.TokenURI == "" {
		return false, fmt.Errorf("[vertex]: missing auth params")
	}

	jwtToken, err := createJWT(&key)
	if err != nil {
		log.Errorf("[vertex]: unable to create JWT token: %v", err)
		return false, err
	}

	err = v.getAccessToken(jwtToken)
	if err != nil {
		log.Errorf("[vertex]: unable to get access token: %v", err)
		return false, err
	}

	return false, err
}

func appendOrReplaceAPIKey(path, apiKey string) string {
	if apiKey == "" {
		return path
	}

	parsedPath, err := url.ParseRequestURI(path)
	if err != nil {
		// Fallback to simple append when path is not parseable.
		if strings.Contains(path, "?") {
			return path + "&key=" + apiKey
		}
		return path + "?key=" + apiKey
	}

	query := parsedPath.Query()
	query.Set("key", apiKey)
	parsedPath.RawQuery = query.Encode()
	return parsedPath.RequestURI()
}

func (v *vertexProvider) getExpressAPIKey(ctx wrapper.HttpContext) string {
	apiKey := v.config.GetApiTokenInUse(ctx)
	if apiKey == "" {
		apiKey = v.config.GetRandomToken()
	}
	return apiKey
}

func (v *vertexProvider) OnRequestBody(ctx wrapper.HttpContext, apiName ApiName, body []byte) (types.Action, error) {
	if !v.config.isSupportedAPI(apiName) {
		return types.ActionContinue, errUnsupportedApiName
	}

	// Vertex Raw 模式: 透传请求体，只做 OAuth 认证
	// 用于直接访问 Vertex AI REST API，不做协议转换
	// 注意：此检查必须在 IsOriginal() 之前，因为 Vertex Raw 模式通常与 original 协议一起使用
	if apiName == ApiNameVertexRaw {
		ctx.SetContext(contextVertexRawMarker, true)
		// Express Mode: 将 API Key 追加到 URL query 参数中
		if v.isExpressMode() {
			headers := util.GetRequestHeaders()
			path := headers.Get(":path")
			path = appendOrReplaceAPIKey(path, v.getExpressAPIKey(ctx))
			util.OverwriteRequestPathHeader(headers, path)
			headers.Del("Authorization")
			util.ReplaceRequestHeaders(headers)
			return types.ActionContinue, nil
		}
		// 标准模式需要获取 OAuth token
		cached, err := v.getToken()
		if cached {
			return types.ActionContinue, nil
		}
		if err == nil {
			return types.ActionPause, nil
		}
		return types.ActionContinue, err
	}

	if v.config.IsOriginal() {
		return types.ActionContinue, nil
	}

	headers := util.GetRequestHeaders()

	// OpenAI 兼容模式: 不转换请求体，只设置路径和进行模型映射
	if v.isOpenAICompatibleMode() {
		ctx.SetContext(contextOpenAICompatibleMarker, true)
		body, err := v.onOpenAICompatibleRequestBody(ctx, apiName, body, headers)
		if err != nil {
			return types.ActionContinue, err
		}
		headers.Set("Content-Length", fmt.Sprint(len(body)))
		util.ReplaceRequestHeaders(headers)
		_ = proxywasm.ReplaceHttpRequestBody(body)
		// OpenAI 兼容模式需要 OAuth token
		cached, err := v.getToken()
		if cached {
			return types.ActionContinue, nil
		}
		if err == nil {
			return types.ActionPause, nil
		}
		return types.ActionContinue, err
	}

	body, err := v.TransformRequestBodyHeaders(ctx, apiName, body, headers)
	if err != nil {
		return types.ActionContinue, err
	}
	headers.Set("Content-Length", fmt.Sprint(len(body)))

	if v.isExpressMode() {
		// Express Mode: 不需要 Authorization header，API Key 已在 URL 中
		headers.Del("Authorization")
		util.ReplaceRequestHeaders(headers)
		_ = proxywasm.ReplaceHttpRequestBody(body)
		return types.ActionContinue, nil
	}

	// 标准模式: 需要获取 OAuth token
	util.ReplaceRequestHeaders(headers)
	_ = proxywasm.ReplaceHttpRequestBody(body)
	cached, err := v.getToken()
	if cached {
		return types.ActionContinue, nil
	}
	if err == nil {
		return types.ActionPause, nil
	}
	return types.ActionContinue, err
}

func (v *vertexProvider) TransformRequestBodyHeaders(ctx wrapper.HttpContext, apiName ApiName, body []byte, headers http.Header) ([]byte, error) {
	switch apiName {
	case ApiNameChatCompletion:
		return v.onChatCompletionRequestBody(ctx, body, headers)
	case ApiNameEmbeddings:
		return v.onEmbeddingsRequestBody(ctx, body, headers)
	case ApiNameImageGeneration:
		return v.onImageGenerationRequestBody(ctx, body, headers)
	case ApiNameImageEdit:
		return v.onImageEditRequestBody(ctx, body, headers)
	case ApiNameImageVariation:
		return v.onImageVariationRequestBody(ctx, body, headers)
	default:
		return body, nil
	}
}

// onOpenAICompatibleRequestBody 处理 OpenAI 兼容模式的请求
// 不转换请求体格式，只进行模型映射和路径设置
func (v *vertexProvider) onOpenAICompatibleRequestBody(ctx wrapper.HttpContext, apiName ApiName, body []byte, headers http.Header) ([]byte, error) {
	if apiName != ApiNameChatCompletion {
		return nil, fmt.Errorf("OpenAI compatible mode only supports chat completions API")
	}

	// 解析请求进行模型映射
	request := &chatCompletionRequest{}
	if err := v.config.parseRequestAndMapModel(ctx, request, body); err != nil {
		return nil, err
	}

	// 设置 OpenAI 兼容端点路径
	path := v.getOpenAICompatibleRequestPath()
	util.OverwriteRequestPathHeader(headers, path)

	// 如果模型被映射，需要更新请求体中的模型字段
	if request.Model != "" {
		body, _ = sjson.SetBytes(body, "model", request.Model)
	}

	// 保持 OpenAI 格式，直接返回（可能更新了模型字段）
	return body, nil
}

func (v *vertexProvider) onChatCompletionRequestBody(ctx wrapper.HttpContext, body []byte, headers http.Header) ([]byte, error) {
	request := &chatCompletionRequest{}
	err := v.config.parseRequestAndMapModel(ctx, request, body)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(request.Model, "claude") {
		ctx.SetContext(contextClaudeMarker, true)
		path := v.getAhthropicRequestPath(ctx, ApiNameChatCompletion, request.Model, request.Stream)
		util.OverwriteRequestPathHeader(headers, path)

		claudeRequest := v.claude.buildClaudeTextGenRequest(request)
		claudeRequest.Model = ""
		claudeRequest.AnthropicVersion = vertexAnthropicVersion
		claudeBody, err := json.Marshal(claudeRequest)
		if err != nil {
			return nil, err
		}
		return claudeBody, nil
	} else {
		path := v.getRequestPath(ctx, ApiNameChatCompletion, request.Model, request.Stream)
		util.OverwriteRequestPathHeader(headers, path)

		vertexRequest, err := v.buildVertexChatRequest(request)
		if err != nil {
			return nil, err
		}
		return json.Marshal(vertexRequest)
	}
}

func (v *vertexProvider) onEmbeddingsRequestBody(ctx wrapper.HttpContext, body []byte, headers http.Header) ([]byte, error) {
	request := &embeddingsRequest{}
	if err := v.config.parseRequestAndMapModel(ctx, request, body); err != nil {
		return nil, err
	}
	path := v.getRequestPath(ctx, ApiNameEmbeddings, request.Model, false)
	util.OverwriteRequestPathHeader(headers, path)

	vertexRequest := v.buildEmbeddingRequest(request)
	return json.Marshal(vertexRequest)
}

func (v *vertexProvider) onImageGenerationRequestBody(ctx wrapper.HttpContext, body []byte, headers http.Header) ([]byte, error) {
	request := &imageGenerationRequest{}
	if err := v.config.parseRequestAndMapModel(ctx, request, body); err != nil {
		return nil, err
	}
	// 图片生成不使用流式端点，需要完整响应
	path := v.getRequestPath(ctx, ApiNameImageGeneration, request.Model, false)
	util.OverwriteRequestPathHeader(headers, path)

	vertexRequest, err := v.buildVertexImageGenerationRequest(request)
	if err != nil {
		return nil, err
	}
	return json.Marshal(vertexRequest)
}

func (v *vertexProvider) onImageEditRequestBody(ctx wrapper.HttpContext, body []byte, headers http.Header) ([]byte, error) {
	request := &imageEditRequest{}
	imageURLs := make([]string, 0)
	contentType := headers.Get("Content-Type")
	if isMultipartFormData(contentType) {
		parsedRequest, err := parseMultipartImageRequest(body, contentType)
		if err != nil {
			return nil, err
		}
		request.Model = parsedRequest.Model
		request.Prompt = parsedRequest.Prompt
		request.Size = parsedRequest.Size
		request.OutputFormat = parsedRequest.OutputFormat
		request.N = parsedRequest.N
		imageURLs = parsedRequest.ImageURLs
		if err := v.config.mapModel(ctx, &request.Model); err != nil {
			return nil, err
		}
		if parsedRequest.HasMask {
			return nil, fmt.Errorf("mask is not supported for vertex image edits yet")
		}
	} else {
		if err := v.config.parseRequestAndMapModel(ctx, request, body); err != nil {
			return nil, err
		}
		if request.HasMask() {
			return nil, fmt.Errorf("mask is not supported for vertex image edits yet")
		}
		imageURLs = request.GetImageURLs()
	}
	if len(imageURLs) == 0 {
		return nil, fmt.Errorf("missing image_url in request")
	}
	if request.Prompt == "" {
		return nil, fmt.Errorf("missing prompt in request")
	}

	path := v.getRequestPath(ctx, ApiNameImageEdit, request.Model, false)
	util.OverwriteRequestPathHeader(headers, path)
	headers.Set("Content-Type", util.MimeTypeApplicationJson)
	vertexRequest, err := v.buildVertexImageRequest(request.Prompt, request.Size, request.OutputFormat, imageURLs)
	if err != nil {
		return nil, err
	}
	return json.Marshal(vertexRequest)
}

func (v *vertexProvider) onImageVariationRequestBody(ctx wrapper.HttpContext, body []byte, headers http.Header) ([]byte, error) {
	request := &imageVariationRequest{}
	imageURLs := make([]string, 0)
	contentType := headers.Get("Content-Type")
	if isMultipartFormData(contentType) {
		parsedRequest, err := parseMultipartImageRequest(body, contentType)
		if err != nil {
			return nil, err
		}
		request.Model = parsedRequest.Model
		request.Prompt = parsedRequest.Prompt
		request.Size = parsedRequest.Size
		request.OutputFormat = parsedRequest.OutputFormat
		request.N = parsedRequest.N
		imageURLs = parsedRequest.ImageURLs
		if err := v.config.mapModel(ctx, &request.Model); err != nil {
			return nil, err
		}
	} else {
		if err := v.config.parseRequestAndMapModel(ctx, request, body); err != nil {
			return nil, err
		}
		imageURLs = request.GetImageURLs()
	}
	if len(imageURLs) == 0 {
		return nil, fmt.Errorf("missing image_url in request")
	}

	prompt := request.Prompt
	if prompt == "" {
		prompt = vertexImageVariationDefaultPrompt
	}

	path := v.getRequestPath(ctx, ApiNameImageVariation, request.Model, false)
	util.OverwriteRequestPathHeader(headers, path)
	headers.Set("Content-Type", util.MimeTypeApplicationJson)
	vertexRequest, err := v.buildVertexImageRequest(prompt, request.Size, request.OutputFormat, imageURLs)
	if err != nil {
		return nil, err
	}
	return json.Marshal(vertexRequest)
}

func (v *vertexProvider) buildVertexImageGenerationRequest(request *imageGenerationRequest) (*vertexChatRequest, error) {
	return v.buildVertexImageRequest(request.Prompt, request.Size, request.OutputFormat, nil)
}

func (v *vertexProvider) buildVertexImageRequest(prompt string, size string, outputFormat string, imageURLs []string) (*vertexChatRequest, error) {
	// 构建安全设置
	safetySettings := make([]vertexChatSafetySetting, 0)
	for category, threshold := range v.config.geminiSafetySetting {
		safetySettings = append(safetySettings, vertexChatSafetySetting{
			Category:  category,
			Threshold: threshold,
		})
	}

	// 解析尺寸参数
	aspectRatio, imageSize := v.parseImageSize(size)

	// 确定输出 MIME 类型
	mimeType := "image/png"
	if outputFormat != "" {
		switch outputFormat {
		case "jpeg", "jpg":
			mimeType = "image/jpeg"
		case "webp":
			mimeType = "image/webp"
		default:
			mimeType = "image/png"
		}
	}

	parts := make([]vertexPart, 0, len(imageURLs)+1)
	for _, imageURL := range imageURLs {
		part, err := convertMediaContent(imageURL)
		if err != nil {
			return nil, err
		}
		parts = append(parts, part)
	}
	if prompt != "" {
		parts = append(parts, vertexPart{
			Text: prompt,
		})
	}
	if len(parts) == 0 {
		return nil, fmt.Errorf("missing prompt and image_url in request")
	}

	vertexRequest := &vertexChatRequest{
		Contents: []vertexChatContent{{
			Role:  roleUser,
			Parts: parts,
		}},
		SafetySettings: safetySettings,
		GenerationConfig: vertexChatGenerationConfig{
			Temperature:        1.0,
			MaxOutputTokens:    32768,
			ResponseModalities: []string{"TEXT", "IMAGE"},
			ImageConfig: &vertexImageConfig{
				AspectRatio: aspectRatio,
				ImageSize:   imageSize,
				ImageOutputOptions: &vertexImageOutputOptions{
					MimeType: mimeType,
				},
				PersonGeneration: "ALLOW_ALL",
			},
		},
	}

	return vertexRequest, nil
}

// parseImageSize 解析 OpenAI 格式的尺寸字符串（如 "1024x1024"）为 Vertex AI 的 aspectRatio 和 imageSize
// Vertex AI 支持的 aspectRatio: 1:1, 3:2, 2:3, 3:4, 4:3, 4:5, 5:4, 9:16, 16:9, 21:9
// Vertex AI 支持的 imageSize: 1k, 2k, 4k
func (v *vertexProvider) parseImageSize(size string) (aspectRatio, imageSize string) {
	// 默认值
	aspectRatio = "1:1"
	imageSize = "1k"

	if size == "" {
		return
	}

	// 预定义的尺寸映射（OpenAI 标准尺寸）
	sizeMapping := map[string]struct {
		aspectRatio string
		imageSize   string
	}{
		// OpenAI DALL-E 标准尺寸
		"256x256":   {"1:1", "1k"},
		"512x512":   {"1:1", "1k"},
		"1024x1024": {"1:1", "1k"},
		"1792x1024": {"16:9", "2k"},
		"1024x1792": {"9:16", "2k"},
		// 扩展尺寸支持
		"2048x2048": {"1:1", "2k"},
		"4096x4096": {"1:1", "4k"},
		// 3:2 和 2:3 比例
		"1536x1024": {"3:2", "2k"},
		"1024x1536": {"2:3", "2k"},
		// 4:3 和 3:4 比例
		"1024x768":  {"4:3", "1k"},
		"768x1024":  {"3:4", "1k"},
		"1365x1024": {"4:3", "1k"},
		"1024x1365": {"3:4", "1k"},
		// 5:4 和 4:5 比例
		"1280x1024": {"5:4", "1k"},
		"1024x1280": {"4:5", "1k"},
		// 21:9 超宽比例
		"2560x1080": {"21:9", "2k"},
	}

	if mapping, ok := sizeMapping[size]; ok {
		return mapping.aspectRatio, mapping.imageSize
	}

	return
}

func (v *vertexProvider) OnStreamingResponseBody(ctx wrapper.HttpContext, name ApiName, chunk []byte, isLastChunk bool) ([]byte, error) {
	// OpenAI 兼容模式: 透传响应，但需要解码 Unicode 转义序列
	// Vertex AI OpenAI-compatible API 返回 ASCII-safe JSON，将非 ASCII 字符编码为 \uXXXX
	if ctx.GetContext(contextOpenAICompatibleMarker) != nil && ctx.GetContext(contextOpenAICompatibleMarker).(bool) {
		return util.DecodeUnicodeEscapesInSSE(chunk), nil
	}

	if ctx.GetContext(contextClaudeMarker) != nil && ctx.GetContext(contextClaudeMarker).(bool) {
		return v.claude.OnStreamingResponseBody(ctx, name, chunk, isLastChunk)
	}
	log.Infof("[vertexProvider] receive chunk body: %s", string(chunk))
	if len(chunk) == 0 && !isLastChunk {
		return nil, nil
	}
	if name != ApiNameChatCompletion {
		if isLastChunk {
			return []byte(ssePrefix + "[DONE]\n\n"), nil
		}
		return chunk, nil
	}

	responseBuilder := &strings.Builder{}
	// Flush a trailing event when upstream closes stream without a final blank line.
	chunkForParsing := chunk
	if isLastChunk {
		trailingNewLineCount := 0
		for i := len(chunkForParsing) - 1; i >= 0 && chunkForParsing[i] == '\n'; i-- {
			trailingNewLineCount++
		}
		if trailingNewLineCount < 2 {
			chunkForParsing = append([]byte(nil), chunk...)
			for i := 0; i < 2-trailingNewLineCount; i++ {
				chunkForParsing = append(chunkForParsing, '\n')
			}
		}
	}
	streamEvents := ExtractStreamingEvents(ctx, chunkForParsing)
	doneSent, _ := ctx.GetContext(contextVertexStreamDoneMarker).(bool)
	appendDone := isLastChunk && !doneSent
	for _, event := range streamEvents {
		data := event.Data
		if data == "" {
			continue
		}
		if data == streamEndDataValue {
			if !doneSent {
				appendDone = true
				doneSent = true
			}
			continue
		}
		var vertexResp vertexChatResponse
		if err := json.Unmarshal([]byte(data), &vertexResp); err != nil {
			log.Errorf("unable to unmarshal vertex response: %v", err)
			continue
		}
		response := v.buildChatCompletionStreamResponse(ctx, &vertexResp)
		responseBody, err := json.Marshal(response)
		if err != nil {
			log.Errorf("unable to marshal response: %v", err)
			return nil, err
		}
		v.appendResponse(responseBuilder, string(responseBody))
	}
	if appendDone {
		responseBuilder.WriteString(ssePrefix + "[DONE]\n\n")
		doneSent = true
	}
	ctx.SetContext(contextVertexStreamDoneMarker, doneSent)
	modifiedResponseChunk := responseBuilder.String()
	if modifiedResponseChunk == "" {
		// Returning an empty payload prevents main.go from falling back to
		// forwarding the original raw chunk, which may contain partial JSON.
		return []byte(""), nil
	}
	log.Debugf("=== modified response chunk: %s", modifiedResponseChunk)
	return []byte(modifiedResponseChunk), nil
}

func (v *vertexProvider) TransformResponseBody(ctx wrapper.HttpContext, apiName ApiName, body []byte) ([]byte, error) {
	// OpenAI 兼容模式: 透传响应，但需要解码 Unicode 转义序列
	// Vertex AI OpenAI-compatible API 返回 ASCII-safe JSON，将非 ASCII 字符编码为 \uXXXX
	if ctx.GetContext(contextOpenAICompatibleMarker) != nil && ctx.GetContext(contextOpenAICompatibleMarker).(bool) {
		return util.DecodeUnicodeEscapes(body), nil
	}

	if ctx.GetContext(contextClaudeMarker) != nil && ctx.GetContext(contextClaudeMarker).(bool) {
		return v.claude.TransformResponseBody(ctx, apiName, body)
	}

	switch apiName {
	case ApiNameChatCompletion:
		return v.onChatCompletionResponseBody(ctx, body)
	case ApiNameEmbeddings:
		return v.onEmbeddingsResponseBody(ctx, body)
	case ApiNameImageGeneration, ApiNameImageEdit, ApiNameImageVariation:
		return v.onImageGenerationResponseBody(ctx, body)
	default:
		return body, nil
	}
}

func (v *vertexProvider) onChatCompletionResponseBody(ctx wrapper.HttpContext, body []byte) ([]byte, error) {
	vertexResponse := &vertexChatResponse{}
	if err := json.Unmarshal(body, vertexResponse); err != nil {
		return nil, fmt.Errorf("unable to unmarshal vertex chat response: %v", err)
	}
	response := v.buildChatCompletionResponse(ctx, vertexResponse)
	return json.Marshal(response)
}

func (v *vertexProvider) buildChatCompletionResponse(ctx wrapper.HttpContext, response *vertexChatResponse) *chatCompletionResponse {
	fullTextResponse := chatCompletionResponse{
		Id:      response.ResponseId,
		Object:  objectChatCompletion,
		Created: time.Now().UnixMilli() / 1000,
		Model:   ctx.GetStringContext(ctxKeyFinalRequestModel, ""),
		Choices: make([]chatCompletionChoice, 0, len(response.Candidates)),
		Usage: &usage{
			PromptTokens:     response.UsageMetadata.PromptTokenCount,
			CompletionTokens: response.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      response.UsageMetadata.TotalTokenCount,
			CompletionTokensDetails: &completionTokensDetails{
				ReasoningTokens: response.UsageMetadata.ThoughtsTokenCount,
			},
		},
	}
	for _, candidate := range response.Candidates {
		choice := chatCompletionChoice{
			Index: candidate.Index,
			Message: &chatMessage{
				Role: roleAssistant,
			},
			FinishReason: util.Ptr(candidate.FinishReason),
		}
		if len(candidate.Content.Parts) > 0 {
			part := candidate.Content.Parts[0]
			if part.FunctionCall != nil {
				args, _ := json.Marshal(part.FunctionCall.Args)
				choice.Message.ToolCalls = []toolCall{
					{
						Type: "function",
						Function: functionCall{
							Name:      part.FunctionCall.Name,
							Arguments: string(args),
						},
					},
				}
			} else if part.Thounght != nil && len(candidate.Content.Parts) > 1 {
				choice.Message.Content = reasoningStartTag + part.Text + reasoningEndTag + candidate.Content.Parts[1].Text
			} else if part.Text != "" {
				choice.Message.Content = part.Text
			}
		} else {
			choice.Message.Content = ""
		}
		fullTextResponse.Choices = append(fullTextResponse.Choices, choice)
	}
	return &fullTextResponse
}

func (v *vertexProvider) onEmbeddingsResponseBody(ctx wrapper.HttpContext, body []byte) ([]byte, error) {
	vertexResponse := &vertexEmbeddingResponse{}
	if err := json.Unmarshal(body, vertexResponse); err != nil {
		return nil, fmt.Errorf("unable to unmarshal vertex embeddings response: %v", err)
	}
	response := v.buildEmbeddingsResponse(ctx, vertexResponse)
	return json.Marshal(response)
}

func (v *vertexProvider) buildEmbeddingsResponse(ctx wrapper.HttpContext, vertexResp *vertexEmbeddingResponse) *embeddingsResponse {
	response := embeddingsResponse{
		Object: "list",
		Data:   make([]embedding, 0, len(vertexResp.Predictions)),
		Model:  ctx.GetContext(ctxKeyFinalRequestModel).(string),
	}
	totalTokens := 0
	for _, item := range vertexResp.Predictions {
		response.Data = append(response.Data, embedding{
			Object:    `embedding`,
			Index:     0,
			Embedding: item.Embeddings.Values,
		})
		if item.Embeddings.Statistics != nil {
			totalTokens += item.Embeddings.Statistics.TokenCount
		}
	}
	response.Usage.TotalTokens = totalTokens
	return &response
}

func (v *vertexProvider) onImageGenerationResponseBody(ctx wrapper.HttpContext, body []byte) ([]byte, error) {
	// 使用 gjson 直接提取字段，避免完整反序列化大型 base64 数据
	// 这样可以显著减少内存分配和复制次数
	response := v.buildImageGenerationResponseFromJSON(body)
	return json.Marshal(response)
}

// buildImageGenerationResponseFromJSON 使用 gjson 从原始 JSON 中提取图片生成响应
// 相比 json.Unmarshal 完整反序列化，这种方式内存效率更高
func (v *vertexProvider) buildImageGenerationResponseFromJSON(body []byte) *imageGenerationResponse {
	result := gjson.ParseBytes(body)
	data := make([]imageGenerationData, 0)

	// 遍历所有 candidates，提取图片数据
	candidates := result.Get("candidates")
	candidates.ForEach(func(_, candidate gjson.Result) bool {
		parts := candidate.Get("content.parts")
		parts.ForEach(func(_, part gjson.Result) bool {
			// 跳过思考过程 (thought: true)
			if part.Get("thought").Bool() {
				return true
			}
			// 提取图片数据
			inlineData := part.Get("inlineData.data")
			if inlineData.Exists() && inlineData.String() != "" {
				data = append(data, imageGenerationData{
					B64: inlineData.String(),
				})
			}
			return true
		})
		return true
	})

	// 提取 usage 信息
	usage := result.Get("usageMetadata")

	return &imageGenerationResponse{
		Created: time.Now().UnixMilli() / 1000,
		Data:    data,
		Usage: &imageGenerationUsage{
			TotalTokens:  int(usage.Get("totalTokenCount").Int()),
			InputTokens:  int(usage.Get("promptTokenCount").Int()),
			OutputTokens: int(usage.Get("candidatesTokenCount").Int()),
		},
	}
}

func (v *vertexProvider) buildChatCompletionStreamResponse(ctx wrapper.HttpContext, vertexResp *vertexChatResponse) *chatCompletionResponse {
	var choice chatCompletionChoice
	choice.Delta = &chatMessage{}
	if len(vertexResp.Candidates) > 0 && len(vertexResp.Candidates[0].Content.Parts) > 0 {
		part := vertexResp.Candidates[0].Content.Parts[0]
		if part.FunctionCall != nil {
			args, _ := json.Marshal(part.FunctionCall.Args)
			choice.Delta = &chatMessage{
				ToolCalls: []toolCall{
					{
						Type: "function",
						Function: functionCall{
							Name:      part.FunctionCall.Name,
							Arguments: string(args),
						},
					},
				},
			}
		} else if part.Thounght != nil {
			if ctx.GetContext("thinking_start") == nil {
				choice.Delta = &chatMessage{Content: reasoningStartTag + part.Text}
				ctx.SetContext("thinking_start", true)
			} else {
				choice.Delta = &chatMessage{Content: part.Text}
			}
		} else if part.Text != "" {
			if ctx.GetContext("thinking_start") != nil && ctx.GetContext("thinking_end") == nil {
				choice.Delta = &chatMessage{Content: reasoningEndTag + part.Text}
				ctx.SetContext("thinking_end", true)
			} else {
				choice.Delta = &chatMessage{Content: part.Text}
			}
		}
	}
	streamResponse := chatCompletionResponse{
		Id:      vertexResp.ResponseId,
		Object:  objectChatCompletionChunk,
		Created: time.Now().UnixMilli() / 1000,
		Model:   ctx.GetStringContext(ctxKeyFinalRequestModel, ""),
		Choices: []chatCompletionChoice{choice},
		Usage: &usage{
			PromptTokens:     vertexResp.UsageMetadata.PromptTokenCount,
			CompletionTokens: vertexResp.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      vertexResp.UsageMetadata.TotalTokenCount,
			CompletionTokensDetails: &completionTokensDetails{
				ReasoningTokens: vertexResp.UsageMetadata.ThoughtsTokenCount,
			},
		},
	}
	return &streamResponse
}

func (v *vertexProvider) appendResponse(responseBuilder *strings.Builder, responseBody string) {
	responseBuilder.WriteString(fmt.Sprintf("%s %s\n\n", streamDataItemKey, responseBody))
}

func (v *vertexProvider) getAhthropicRequestPath(ctx wrapper.HttpContext, apiName ApiName, modelId string, stream bool) string {
	action := ""
	if stream {
		action = vertexAnthropicMessageStreamAction
	} else {
		action = vertexAnthropicMessageAction
	}

	if v.isExpressMode() {
		// Express Mode: 简化路径 + API Key 参数
		basePath := fmt.Sprintf(vertexExpressPathAnthropicTemplate, modelId, action)
		apiKey := v.getExpressAPIKey(ctx)
		return appendOrReplaceAPIKey(basePath, apiKey)
	}

	path := fmt.Sprintf(vertexPathAnthropicTemplate, v.config.vertexProjectId, v.config.vertexRegion, modelId, action)
	return path
}

func (v *vertexProvider) getRequestPath(ctx wrapper.HttpContext, apiName ApiName, modelId string, stream bool) string {
	action := ""
	switch apiName {
	case ApiNameEmbeddings:
		action = vertexEmbeddingAction
	case ApiNameImageGeneration, ApiNameImageEdit, ApiNameImageVariation:
		// 图片生成使用非流式端点，需要完整响应
		action = vertexChatCompletionAction
	default:
		if stream {
			action = vertexChatCompletionStreamAction
		} else {
			action = vertexChatCompletionAction
		}
	}

	if v.isExpressMode() {
		// Express Mode: 简化路径 + API Key 参数
		basePath := fmt.Sprintf(vertexExpressPathTemplate, modelId, action)
		apiKey := v.getExpressAPIKey(ctx)
		return appendOrReplaceAPIKey(basePath, apiKey)
	}

	path := fmt.Sprintf(vertexPathTemplate, v.config.vertexProjectId, v.config.vertexRegion, modelId, action)
	return path
}

// getOpenAICompatibleRequestPath 获取 OpenAI 兼容模式的请求路径
func (v *vertexProvider) getOpenAICompatibleRequestPath() string {
	return fmt.Sprintf(vertexOpenAICompatiblePathTemplate, v.config.vertexProjectId, v.config.vertexRegion)
}

func (v *vertexProvider) buildVertexChatRequest(request *chatCompletionRequest) (*vertexChatRequest, error) {
	safetySettings := make([]vertexChatSafetySetting, 0)
	for category, threshold := range v.config.geminiSafetySetting {
		safetySettings = append(safetySettings, vertexChatSafetySetting{
			Category:  category,
			Threshold: threshold,
		})
	}
	vertexRequest := vertexChatRequest{
		Contents:       make([]vertexChatContent, 0),
		SafetySettings: safetySettings,
		GenerationConfig: vertexChatGenerationConfig{
			Temperature:     request.Temperature,
			TopP:            request.TopP,
			MaxOutputTokens: request.MaxTokens,
		},
	}
	if request.ReasoningEffort != "" {
		thinkingConfig := vertexThinkingConfig{
			IncludeThoughts: true,
			ThinkingBudget:  1024,
		}
		switch request.ReasoningEffort {
		case "none":
			thinkingConfig.IncludeThoughts = false
			thinkingConfig.ThinkingBudget = 0
		case "low":
			thinkingConfig.ThinkingBudget = 1024
		case "medium":
			thinkingConfig.ThinkingBudget = 4096
		case "high":
			thinkingConfig.ThinkingBudget = 16384
		}
		vertexRequest.GenerationConfig.ThinkingConfig = thinkingConfig
	}
	if err := v.applyResponseFormatToGenerationConfig(request.ResponseFormat, &vertexRequest.GenerationConfig, request.Model); err != nil {
		return nil, err
	}
	if request.Tools != nil {
		functions := make([]function, 0, len(request.Tools))
		for _, tool := range request.Tools {
			functions = append(functions, tool.Function)
		}
		vertexRequest.Tools = []vertexTool{
			{
				FunctionDeclarations: functions,
			},
		}
	}
	shouldAddDummyModelMessage := false
	var lastFunctionName string
	for _, message := range request.Messages {
		content := vertexChatContent{
			Role:  message.Role,
			Parts: []vertexPart{},
		}
		if len(message.ToolCalls) > 0 {
			lastFunctionName = message.ToolCalls[0].Function.Name
			args := make(map[string]interface{})
			if err := json.Unmarshal([]byte(message.ToolCalls[0].Function.Arguments), &args); err != nil {
				log.Errorf("unable to unmarshal function arguments: %v", err)
			}
			content.Parts = append(content.Parts, vertexPart{
				FunctionCall: &vertexFunctionCall{
					Name: lastFunctionName,
					Args: args,
				},
			})
		} else {
			for _, part := range message.ParseContent() {
				switch part.Type {
				case contentTypeText:
					if message.Role == roleTool {
						content.Parts = append(content.Parts, vertexPart{
							FunctionResponse: &vertexFunctionResponse{
								Name: lastFunctionName,
								Response: vertexFunctionResponseDetail{
									Output: part.Text,
								},
							},
						})
					} else {
						content.Parts = append(content.Parts, vertexPart{
							Text: part.Text,
						})
					}
				case contentTypeImageUrl:
					vpart, err := convertMediaContent(part.ImageUrl.Url)
					if err != nil {
						log.Errorf("unable to convert image content: %v", err)
					} else {
						content.Parts = append(content.Parts, vpart)
					}
				}
			}
		}

		// there's no assistant role in vertex and API shall vomit if role is not user or model
		switch content.Role {
		case roleAssistant:
			content.Role = "model"
		case roleTool:
			content.Role = roleUser
		case roleSystem: // converting system prompt to prompt from user for the same reason
			content.Role = roleUser
			shouldAddDummyModelMessage = true
		}
		vertexRequest.Contents = append(vertexRequest.Contents, content)

		// if a system message is the last message, we need to add a dummy model message to make vertex happy
		if shouldAddDummyModelMessage {
			vertexRequest.Contents = append(vertexRequest.Contents, vertexChatContent{
				Role: "model",
				Parts: []vertexPart{
					{
						Text: "Okay",
					},
				},
			})
			shouldAddDummyModelMessage = false
		}
	}

	return &vertexRequest, nil
}

// applyResponseFormatToGenerationConfig maps OpenAI response_format into Vertex generationConfig.
// The mapping is strict for type=json_schema to avoid silently breaking structured-output contracts.
func (v *vertexProvider) applyResponseFormatToGenerationConfig(responseFormat map[string]interface{}, generationConfig *vertexChatGenerationConfig, model string) error {
	if generationConfig == nil || len(responseFormat) == 0 {
		return nil
	}

	// NOTE: Gemini 2.0 structured output requires propertyOrdering.
	// Because gemini-2.0-* is legacy and rarely used, we intentionally do not implement
	// propertyOrdering synthesis here; instead we ignore response_format and keep request
	// as non-structured output for stability and minimal conversion behavior.
	if requiresPropertyOrderingForModel(model) {
		return nil
	}

	responseFormatType, _ := responseFormat["type"].(string)
	responseFormatType = strings.ToLower(responseFormatType)

	switch responseFormatType {
	case "":
		// Be tolerant for non-standard clients that pass schema directly in response_format.
		if isJSONSchemaMap(responseFormat) {
			generationConfig.ResponseMimeType = util.MimeTypeApplicationJson
			generationConfig.ResponseSchema = responseFormat
		}
	case "json_object":
		generationConfig.ResponseMimeType = util.MimeTypeApplicationJson
	case "json_schema":
		schema := extractOpenAIJSONSchema(responseFormat)
		if len(schema) == 0 {
			return fmt.Errorf("invalid response_format.json_schema: missing schema object")
		}
		generationConfig.ResponseMimeType = util.MimeTypeApplicationJson
		generationConfig.ResponseSchema = schema
	case "text":
		// Vertex defaults to text output when no response mime/schema is provided.
	default:
		// Be tolerant for non-standard usage where response_format itself is a JSON schema.
		if isJSONSchemaType(responseFormatType) && isJSONSchemaMap(responseFormat) {
			generationConfig.ResponseMimeType = util.MimeTypeApplicationJson
			generationConfig.ResponseSchema = responseFormat
		}
	}
	return nil
}

func extractOpenAIJSONSchema(responseFormat map[string]interface{}) map[string]interface{} {
	jsonSchemaValue, ok := responseFormat["json_schema"]
	if !ok {
		return nil
	}

	jsonSchemaMap, ok := jsonSchemaValue.(map[string]interface{})
	if !ok {
		return nil
	}

	// OpenAI canonical format:
	// {
	//   "type":"json_schema",
	//   "json_schema":{"name":"...","strict":true,"schema":{...}}
	// }
	if nestedSchemaValue, ok := jsonSchemaMap["schema"]; ok {
		if nestedSchema, ok := nestedSchemaValue.(map[string]interface{}); ok {
			return nestedSchema
		}
	}

	// Tolerate non-standard format where json_schema itself is the schema.
	if isJSONSchemaMap(jsonSchemaMap) {
		return jsonSchemaMap
	}
	return nil
}

func isJSONSchemaType(value string) bool {
	switch strings.ToLower(value) {
	case "object", "array", "string", "number", "integer", "boolean", "null":
		return true
	default:
		return false
	}
}

func isJSONSchemaMap(schema map[string]interface{}) bool {
	if len(schema) == 0 {
		return false
	}

	if typeValue, ok := schema["type"].(string); ok && isJSONSchemaType(typeValue) {
		return true
	}

	// Schema might omit "type" and still be valid for specific cases.
	schemaKeys := []string{
		"anyOf",
		"enum",
		"format",
		"items",
		"maximum",
		"maxItems",
		"minimum",
		"minItems",
		"nullable",
		"properties",
		"description",
		"propertyOrdering",
		"required",
	}
	for _, key := range schemaKeys {
		if _, ok := schema[key]; ok {
			return true
		}
	}

	return false
}

func requiresPropertyOrderingForModel(model string) bool {
	model = strings.ToLower(model)
	return strings.HasPrefix(model, "gemini-2.0-")
}

func (v *vertexProvider) buildEmbeddingRequest(request *embeddingsRequest) *vertexEmbeddingRequest {
	inputs := request.ParseInput()
	instances := make([]vertexEmbeddingInstance, len(inputs))
	for i, input := range inputs {
		instances[i] = vertexEmbeddingInstance{
			Content: input,
		}
	}
	return &vertexEmbeddingRequest{Instances: instances}
}

type vertexChatRequest struct {
	CachedContent     string                     `json:"cachedContent,omitempty"`
	Contents          []vertexChatContent        `json:"contents"`
	SystemInstruction *vertexSystemInstruction   `json:"systemInstruction,omitempty"`
	Tools             []vertexTool               `json:"tools,omitempty"`
	SafetySettings    []vertexChatSafetySetting  `json:"safetySettings,omitempty"`
	GenerationConfig  vertexChatGenerationConfig `json:"generationConfig,omitempty"`
	Labels            map[string]string          `json:"labels,omitempty"`
}

type vertexChatContent struct {
	// The producer of the content. Must be either 'user' or 'model'.
	Role  string       `json:"role,omitempty"`
	Parts []vertexPart `json:"parts"`
}

type vertexPart struct {
	Text             string                  `json:"text,omitempty"`
	InlineData       *blob                   `json:"inlineData,omitempty"`
	FileData         *fileData               `json:"fileData,omitempty"`
	FunctionCall     *vertexFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *vertexFunctionResponse `json:"functionResponse,omitempty"`
	Thounght         *bool                   `json:"thought,omitempty"`
}

type blob struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type fileData struct {
	MimeType string `json:"mimeType"`
	FileUri  string `json:"fileUri"`
}

type vertexFunctionCall struct {
	Name string                 `json:"name"`
	Args map[string]interface{} `json:"args,omitempty"`
}

type vertexFunctionResponse struct {
	Name     string                       `json:"name"`
	Response vertexFunctionResponseDetail `json:"response"`
}

type vertexFunctionResponseDetail struct {
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

type vertexSystemInstruction struct {
	Role  string       `json:"role"`
	Parts []vertexPart `json:"parts"`
}

type vertexTool struct {
	FunctionDeclarations any `json:"functionDeclarations"`
}

type vertexChatSafetySetting struct {
	Category  string `json:"category"`
	Threshold string `json:"threshold"`
}

type vertexChatGenerationConfig struct {
	Temperature        float64                `json:"temperature,omitempty"`
	TopP               float64                `json:"topP,omitempty"`
	TopK               int                    `json:"topK,omitempty"`
	CandidateCount     int                    `json:"candidateCount,omitempty"`
	MaxOutputTokens    int                    `json:"maxOutputTokens,omitempty"`
	ThinkingConfig     vertexThinkingConfig   `json:"thinkingConfig,omitempty"`
	ResponseMimeType   string                 `json:"responseMimeType,omitempty"`
	ResponseSchema     map[string]interface{} `json:"responseSchema,omitempty"`
	ResponseModalities []string               `json:"responseModalities,omitempty"`
	ImageConfig        *vertexImageConfig     `json:"imageConfig,omitempty"`
}

type vertexImageConfig struct {
	AspectRatio        string                    `json:"aspectRatio,omitempty"`
	ImageSize          string                    `json:"imageSize,omitempty"`
	ImageOutputOptions *vertexImageOutputOptions `json:"imageOutputOptions,omitempty"`
	PersonGeneration   string                    `json:"personGeneration,omitempty"`
}

type vertexImageOutputOptions struct {
	MimeType string `json:"mimeType,omitempty"`
}

type vertexThinkingConfig struct {
	IncludeThoughts bool `json:"includeThoughts,omitempty"`
	ThinkingBudget  int  `json:"thinkingBudget,omitempty"`
}

type vertexEmbeddingRequest struct {
	Instances  []vertexEmbeddingInstance `json:"instances"`
	Parameters *vertexEmbeddingParams    `json:"parameters,omitempty"`
}

type vertexEmbeddingInstance struct {
	TaskType string `json:"task_type"`
	Title    string `json:"title,omitempty"`
	Content  string `json:"content"`
}

type vertexEmbeddingParams struct {
	AutoTruncate bool `json:"autoTruncate,omitempty"`
}

type vertexChatResponse struct {
	Candidates     []vertexChatCandidate    `json:"candidates"`
	ResponseId     string                   `json:"responseId,omitempty"`
	PromptFeedback vertexChatPromptFeedback `json:"promptFeedback"`
	UsageMetadata  vertexUsageMetadata      `json:"usageMetadata"`
}

type vertexChatCandidate struct {
	Content       vertexChatContent        `json:"content"`
	FinishReason  string                   `json:"finishReason"`
	Index         int                      `json:"index"`
	SafetyRatings []vertexChatSafetyRating `json:"safetyRatings"`
}

type vertexChatSafetyRating struct {
	Category    string `json:"category"`
	Probability string `json:"probability"`
}

type vertexChatPromptFeedback struct {
	SafetyRatings []vertexChatSafetyRating `json:"safetyRatings"`
}

type vertexUsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount,omitempty"`
	CandidatesTokenCount int `json:"candidatesTokenCount,omitempty"`
	TotalTokenCount      int `json:"totalTokenCount,omitempty"`
	ThoughtsTokenCount   int `json:"thoughtsTokenCount,omitempty"`
}

type vertexEmbeddingResponse struct {
	Predictions []vertexPredictions `json:"predictions"`
}

type vertexPredictions struct {
	Embeddings struct {
		Values     []float64         `json:"values"`
		Statistics *vertexStatistics `json:"statistics,omitempty"`
	} `json:"embeddings"`
}

type vertexStatistics struct {
	TokenCount int  `json:"token_count"`
	Truncated  bool `json:"truncated"`
}

type ServiceAccountKey struct {
	ClientEmail  string `json:"client_email"`
	PrivateKeyID string `json:"private_key_id"`
	PrivateKey   string `json:"private_key"`
	TokenURI     string `json:"token_uri"`
}

func createJWT(key *ServiceAccountKey) (string, error) {
	// 解析 PEM 格式的 RSA 私钥
	block, _ := pem.Decode([]byte(key.PrivateKey))
	if block == nil {
		return "", fmt.Errorf("invalid PEM block")
	}
	parsedKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return "", err
	}
	rsaKey := parsedKey.(*rsa.PrivateKey)

	// 构造 JWT Header
	jwtHeader := map[string]string{
		"alg": "RS256",
		"typ": "JWT",
		"kid": key.PrivateKeyID,
	}
	headerJSON, _ := json.Marshal(jwtHeader)
	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)

	// 构造 JWT Claims
	now := time.Now().Unix()
	claims := map[string]interface{}{
		"iss":   key.ClientEmail,
		"scope": "https://www.googleapis.com/auth/cloud-platform",
		"aud":   key.TokenURI,
		"iat":   now,
		"exp":   now + 3600, // 1 小时有效期
	}
	claimsJSON, _ := json.Marshal(claims)
	claimsB64 := base64.RawURLEncoding.EncodeToString(claimsJSON)

	signingInput := fmt.Sprintf("%s.%s", headerB64, claimsB64)
	hashed := sha256.Sum256([]byte(signingInput))
	signature, err := rsaKey.Sign(nil, hashed[:], crypto.SHA256)
	if err != nil {
		return "", err
	}
	sigB64 := base64.RawURLEncoding.EncodeToString(signature)

	return fmt.Sprintf("%s.%s.%s", headerB64, claimsB64, sigB64), nil
}

func (v *vertexProvider) getAccessToken(jwtToken string) error {
	headers := [][2]string{
		{"Content-Type", "application/x-www-form-urlencoded"},
	}
	reqBody := "grant_type=urn:ietf:params:oauth:grant-type:jwt-bearer&assertion=" + jwtToken
	err := v.client.Post("/token", headers, []byte(reqBody), func(statusCode int, responseHeaders http.Header, responseBody []byte) {
		responseString := string(responseBody)
		defer func() {
			_ = proxywasm.ResumeHttpRequest()
		}()
		if statusCode != http.StatusOK {
			log.Errorf("failed to create vertex access key, status: %d body: %s", statusCode, responseString)
			_ = util.ErrorHandler("ai-proxy.vertex.load_ak_failed", fmt.Errorf("failed to load vertex ak"))
			return
		}
		responseJson := gjson.Parse(responseString)
		accessToken := responseJson.Get("access_token").String()
		_ = proxywasm.ReplaceHttpRequestHeader("Authorization", "Bearer "+accessToken)

		expiresIn := int64(3600)
		if expiresInVal := responseJson.Get("expires_in"); expiresInVal.Exists() {
			expiresIn = expiresInVal.Int()
		}
		expireTime := time.Now().Add(time.Duration(expiresIn) * time.Second).Unix()
		keyName := v.buildTokenKey()
		err := setCachedAccessToken(keyName, accessToken, expireTime)
		if err != nil {
			log.Errorf("[vertex]: unable to cache access token: %v", err)
		}
	}, v.config.timeout)
	return err
}

func (v *vertexProvider) buildTokenKey() string {
	region := v.config.vertexRegion
	projectID := v.config.vertexProjectId

	return fmt.Sprintf("vertex-%s-%s-access-token", region, projectID)
}

type cachedAccessToken struct {
	Token    string `json:"token"`
	ExpireAt int64  `json:"expireAt"`
}

func (v *vertexProvider) getCachedAccessToken(key string) (string, error) {
	data, _, err := proxywasm.GetSharedData(key)
	if err != nil {
		if errors.Is(err, types.ErrorStatusNotFound) {
			return "", nil
		}
		return "", err
	}
	if data == nil {
		return "", nil
	}

	var tokenInfo cachedAccessToken
	if err = json.Unmarshal(data, &tokenInfo); err != nil {
		return "", err
	}

	now := time.Now().Unix()
	refreshAhead := v.config.vertexTokenRefreshAhead

	if tokenInfo.ExpireAt > now+refreshAhead {
		return tokenInfo.Token, nil
	}

	return "", nil
}

func setCachedAccessToken(key string, accessToken string, expireTime int64) error {
	tokenInfo := cachedAccessToken{
		Token:    accessToken,
		ExpireAt: expireTime,
	}

	_, cas, err := proxywasm.GetSharedData(key)
	if err != nil && !errors.Is(err, types.ErrorStatusNotFound) {
		return err
	}

	data, err := json.Marshal(tokenInfo)
	if err != nil {
		return err
	}

	return proxywasm.SetSharedData(key, data, cas)
}

// convertMediaContent 将 OpenAI 格式的媒体 URL 转换为 Vertex AI 格式
// 支持图片、视频、音频等多种媒体类型
func convertMediaContent(mediaUrl string) (vertexPart, error) {
	part := vertexPart{}
	if strings.HasPrefix(mediaUrl, "http") {
		mimeType := detectMimeTypeFromURL(mediaUrl)
		part.FileData = &fileData{
			MimeType: mimeType,
			FileUri:  mediaUrl,
		}
		return part, nil
	} else {
		// Base64 data URL 格式: data:<mimeType>;base64,<data>
		re := regexp.MustCompile(`^data:([^;]+);base64,`)
		matches := re.FindStringSubmatch(mediaUrl)
		if len(matches) < 2 {
			return part, fmt.Errorf("invalid base64 format, expected data:<mimeType>;base64,<data>")
		}

		mimeType := matches[1] // e.g. image/png, video/mp4, audio/mp3
		parts := strings.Split(mimeType, "/")
		if len(parts) < 2 {
			return part, fmt.Errorf("invalid mimeType: %s", mimeType)
		}
		part.InlineData = &blob{
			MimeType: mimeType,
			Data:     strings.TrimPrefix(mediaUrl, matches[0]),
		}
		return part, nil
	}
}

// detectMimeTypeFromURL 根据 URL 的文件扩展名检测 MIME 类型
// 支持图片、视频、音频和文档类型
func detectMimeTypeFromURL(url string) string {
	// 移除查询参数和片段标识符
	if idx := strings.Index(url, "?"); idx != -1 {
		url = url[:idx]
	}
	if idx := strings.Index(url, "#"); idx != -1 {
		url = url[:idx]
	}

	// 获取最后一个路径段
	lastSlash := strings.LastIndex(url, "/")
	if lastSlash != -1 {
		url = url[lastSlash+1:]
	}

	// 获取扩展名
	lastDot := strings.LastIndex(url, ".")
	if lastDot == -1 || lastDot == len(url)-1 {
		return "application/octet-stream"
	}
	ext := strings.ToLower(url[lastDot+1:])

	// 扩展名到 MIME 类型的映射
	mimeTypes := map[string]string{
		// 图片格式
		"jpg":  "image/jpeg",
		"jpeg": "image/jpeg",
		"png":  "image/png",
		"gif":  "image/gif",
		"webp": "image/webp",
		"bmp":  "image/bmp",
		"svg":  "image/svg+xml",
		"ico":  "image/x-icon",
		"heic": "image/heic",
		"heif": "image/heif",
		"tiff": "image/tiff",
		"tif":  "image/tiff",
		// 视频格式
		"mp4":  "video/mp4",
		"mpeg": "video/mpeg",
		"mpg":  "video/mpeg",
		"mov":  "video/quicktime",
		"avi":  "video/x-msvideo",
		"wmv":  "video/x-ms-wmv",
		"webm": "video/webm",
		"mkv":  "video/x-matroska",
		"flv":  "video/x-flv",
		"3gp":  "video/3gpp",
		"3g2":  "video/3gpp2",
		"m4v":  "video/x-m4v",
		// 音频格式
		"mp3":  "audio/mpeg",
		"wav":  "audio/wav",
		"ogg":  "audio/ogg",
		"flac": "audio/flac",
		"aac":  "audio/aac",
		"m4a":  "audio/mp4",
		"wma":  "audio/x-ms-wma",
		"opus": "audio/opus",
		// 文档格式
		"pdf": "application/pdf",
	}

	if mimeType, ok := mimeTypes[ext]; ok {
		return mimeType
	}

	return "application/octet-stream"
}
