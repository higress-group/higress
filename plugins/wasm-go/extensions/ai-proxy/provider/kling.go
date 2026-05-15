package provider

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-proxy/util"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	klingDefaultDomain          = "api-singapore.klingai.com"
	klingTextToVideoPath        = "/v1/videos/text2video"
	klingImageToVideoPath       = "/v1/videos/image2video"
	klingTextToVideoTaskPath    = "/v1/videos/text2video/{video_id}"
	klingImageToVideoTaskPath   = "/v1/videos/image2video/{video_id}"
	klingJWTLifetimeSeconds     = int64(1800)
	klingJWTNotBeforeSkewSecond = int64(5)
	klingDefaultRefreshAhead    = int64(60)
	klingTaskTypeTextToVideo    = "text2video"
	klingTaskTypeImageToVideo   = "image2video"
	klingTextTaskIDPrefix       = "kling-t2v-"
	klingImageTaskIDPrefix      = "kling-i2v-"
	klingTaskTypeQueryKey       = "kling_task_type"
	ctxKeyKlingVideoTaskType    = "klingVideoTaskType"
)

type klingProviderInitializer struct{}

func (k *klingProviderInitializer) ValidateConfig(config *ProviderConfig) error {
	hasAccessKey := strings.TrimSpace(config.klingAccessKey) != ""
	hasSecretKey := strings.TrimSpace(config.klingSecretKey) != ""
	if hasAccessKey || hasSecretKey {
		if !hasAccessKey || !hasSecretKey {
			return errors.New("missing klingAccessKey or klingSecretKey in provider config")
		}
		return nil
	}
	if len(config.apiTokens) > 0 {
		return nil
	}
	return errors.New("missing kling authentication parameters: either apiTokens or (klingAccessKey + klingSecretKey) is required")
}

func (k *klingProviderInitializer) DefaultCapabilities() map[string]string {
	return map[string]string{
		string(ApiNameVideos):                  klingTextToVideoPath,
		string(ApiNameKlingImageToVideo):       klingImageToVideoPath,
		string(ApiNameRetrieveVideo):           klingTextToVideoTaskPath,
		string(ApiNameKlingRetrieveImageVideo): klingImageToVideoTaskPath,
	}
}

func (k *klingProviderInitializer) CreateProvider(config ProviderConfig) (Provider, error) {
	config.setDefaultCapabilities(k.DefaultCapabilities())
	if config.klingTokenRefreshAhead == 0 {
		config.klingTokenRefreshAhead = klingDefaultRefreshAhead
	}
	provider := &klingProvider{
		config:       config,
		contextCache: createContextCache(&config),
	}
	if config.IsOriginal() {
		return provider, nil
	}
	return &klingOpenAIProvider{klingProvider: provider}, nil
}

type klingProvider struct {
	config       ProviderConfig
	contextCache *contextCache
	jwtToken     string
	jwtExpireAt  int64
}

type klingOpenAIProvider struct {
	*klingProvider
}

func (k *klingProvider) GetProviderType() string {
	return providerTypeKling
}

func (k *klingProvider) OnRequestHeaders(ctx wrapper.HttpContext, apiName ApiName) error {
	k.config.handleRequestHeaders(k, ctx, apiName)
	if k.config.IsOriginal() {
		ctx.DontReadRequestBody()
	}
	return nil
}

func (k *klingOpenAIProvider) OnRequestBody(ctx wrapper.HttpContext, apiName ApiName, body []byte) (types.Action, error) {
	if !k.config.isSupportedAPI(apiName) {
		return types.ActionContinue, errUnsupportedApiName
	}
	return k.config.handleRequestBody(k, k.contextCache, ctx, apiName, body)
}

func (k *klingProvider) TransformRequestHeaders(ctx wrapper.HttpContext, apiName ApiName, headers http.Header) {
	if !k.config.IsOriginal() {
		mappedPath := ""
		if apiName == ApiNameRetrieveVideo {
			mappedPath = k.mapRetrieveVideoPath(headers.Get(util.HeaderPath))
		} else {
			mappedPath = util.MapRequestPathByCapability(string(apiName), headers.Get(util.HeaderPath), k.config.capabilities)
		}
		if mappedPath != "" {
			util.OverwriteRequestPathHeader(headers, mappedPath)
		}
	}
	if k.config.providerDomain == "" {
		util.OverwriteRequestHostHeader(headers, klingDefaultDomain)
	}
	if token := k.getAuthorizationToken(ctx); token != "" {
		util.OverwriteRequestAuthorizationHeader(headers, "Bearer "+token)
	}
	if !k.config.IsOriginal() {
		headers.Del("Content-Length")
	}
}

func (k *klingProvider) TransformRequestBodyHeaders(ctx wrapper.HttpContext, apiName ApiName, body []byte, headers http.Header) ([]byte, error) {
	if apiName != ApiNameVideos {
		return k.config.defaultTransformRequestBody(ctx, apiName, body)
	}

	taskType := klingTaskTypeTextToVideo
	targetPath := k.textCreateVideoPath()
	if k.isImageToVideoRequest(body) {
		taskType = klingTaskTypeImageToVideo
		targetPath = k.imageCreateVideoPath()
	}
	ctx.SetContext(ctxKeyKlingVideoTaskType, taskType)
	util.OverwriteRequestPathHeader(headers, klingPathWithOriginalQuery(ctx, headers.Get(util.HeaderPath), targetPath))
	return k.transformOpenAIVideoRequest(ctx, body)
}

func (k *klingOpenAIProvider) TransformResponseBody(ctx wrapper.HttpContext, apiName ApiName, body []byte) ([]byte, error) {
	if apiName != ApiNameVideos {
		return body, nil
	}

	taskType, _ := ctx.GetContext(ctxKeyKlingVideoTaskType).(string)
	switch taskType {
	case klingTaskTypeTextToVideo:
		return prefixKlingTaskIDs(body, klingTextTaskIDPrefix)
	case klingTaskTypeImageToVideo:
		return prefixKlingTaskIDs(body, klingImageTaskIDPrefix)
	default:
		return body, nil
	}
}

func (k *klingProvider) GetApiName(path string) ApiName {
	switch {
	case isKlingNativeRetrieveVideoPath(path):
		return ApiNameRetrieveVideo
	case isKlingNativeCreateVideoPath(path):
		return ApiNameVideos
	case util.RegRetrieveVideoPath.MatchString(path):
		return ApiNameRetrieveVideo
	default:
		return ""
	}
}

func isKlingNativeCreateVideoPath(path string) bool {
	return strings.HasSuffix(path, klingTextToVideoPath) ||
		strings.HasSuffix(path, klingImageToVideoPath)
}

func isKlingNativeRetrieveVideoPath(path string) bool {
	return hasSinglePathSegmentAfter(path, klingTextToVideoPath) ||
		hasSinglePathSegmentAfter(path, klingImageToVideoPath)
}

func hasSinglePathSegmentAfter(path, prefix string) bool {
	index := strings.Index(path, prefix+"/")
	if index < 0 {
		return false
	}
	remaining := path[index+len(prefix)+1:]
	return remaining != "" && !strings.Contains(remaining, "/")
}

func (k *klingProvider) getAuthorizationToken(ctx wrapper.HttpContext) string {
	if k.isOfficialMode() {
		return k.getJWTToken()
	}
	return k.config.GetApiTokenInUse(ctx)
}

func (k *klingProvider) isOfficialMode() bool {
	return strings.TrimSpace(k.config.klingAccessKey) != "" && strings.TrimSpace(k.config.klingSecretKey) != ""
}

func (k *klingProvider) getJWTToken() string {
	now := time.Now().Unix()
	if k.jwtToken != "" && k.jwtExpireAt > now+k.config.klingTokenRefreshAhead {
		return k.jwtToken
	}

	token, expireAt, err := createKlingJWT(k.config.klingAccessKey, k.config.klingSecretKey, now)
	if err != nil {
		return ""
	}
	k.jwtToken = token
	k.jwtExpireAt = expireAt
	return k.jwtToken
}

func createKlingJWT(accessKey, secretKey string, now int64) (string, int64, error) {
	expireAt := now + klingJWTLifetimeSeconds
	header := struct {
		Alg string `json:"alg"`
		Typ string `json:"typ"`
	}{
		Alg: "HS256",
		Typ: "JWT",
	}
	payload := struct {
		Iss string `json:"iss"`
		Exp int64  `json:"exp"`
		Nbf int64  `json:"nbf"`
	}{
		Iss: strings.TrimSpace(accessKey),
		Exp: expireAt,
		Nbf: now - klingJWTNotBeforeSkewSecond,
	}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", 0, fmt.Errorf("unable to marshal kling jwt header: %v", err)
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", 0, fmt.Errorf("unable to marshal kling jwt payload: %v", err)
	}

	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)
	signingInput := headerB64 + "." + payloadB64
	mac := hmac.New(sha256.New, []byte(strings.TrimSpace(secretKey)))
	_, _ = mac.Write([]byte(signingInput))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return signingInput + "." + signature, expireAt, nil
}

func (k *klingProvider) transformOpenAIVideoRequest(ctx wrapper.HttpContext, body []byte) ([]byte, error) {
	model := gjson.GetBytes(body, "model")
	modelPath := "model"
	if !model.Exists() {
		model = gjson.GetBytes(body, "model_name")
		modelPath = "model_name"
	}
	if model.Exists() {
		rawModel := model.String()
		ctx.SetContext(ctxKeyOriginalRequestModel, rawModel)
		mappedModel := getMappedModel(rawModel, k.config.modelMapping)
		ctx.SetContext(ctxKeyFinalRequestModel, mappedModel)
		var err error
		body, err = sjson.SetBytes(body, "model_name", mappedModel)
		if err != nil {
			return nil, err
		}
		if modelPath == "model" {
			body, err = sjson.DeleteBytes(body, "model")
			if err != nil {
				return nil, err
			}
		}
	}
	return body, nil
}

func (k *klingProvider) mapRetrieveVideoPath(originPath string) string {
	pathOnly, query := splitKlingPathAndQuery(originPath)
	matches := util.RegRetrieveVideoPath.FindStringSubmatch(pathOnly)
	if matches == nil {
		return util.MapRequestPathByCapability(string(ApiNameRetrieveVideo), originPath, k.config.capabilities)
	}

	index := util.RegRetrieveVideoPath.SubexpIndex("video_id")
	if index < 0 || index >= len(matches) {
		return util.MapRequestPathByCapability(string(ApiNameRetrieveVideo), originPath, k.config.capabilities)
	}

	videoID := matches[index]
	taskType, forwardedQuery := extractKlingTaskTypeQuery(query)
	switch {
	case strings.HasPrefix(videoID, klingImageTaskIDPrefix):
		rawID := strings.TrimPrefix(videoID, klingImageTaskIDPrefix)
		return appendKlingQuery(replaceKlingVideoID(k.imageRetrieveVideoPath(), rawID), forwardedQuery)
	case strings.HasPrefix(videoID, klingTextTaskIDPrefix):
		rawID := strings.TrimPrefix(videoID, klingTextTaskIDPrefix)
		return appendKlingQuery(replaceKlingVideoID(k.textRetrieveVideoPath(), rawID), forwardedQuery)
	default:
		if taskType == klingTaskTypeImageToVideo {
			return appendKlingQuery(replaceKlingVideoID(k.imageRetrieveVideoPath(), videoID), forwardedQuery)
		}
		if taskType == klingTaskTypeTextToVideo {
			return appendKlingQuery(replaceKlingVideoID(k.textRetrieveVideoPath(), videoID), forwardedQuery)
		}
		return util.MapRequestPathByCapability(string(ApiNameRetrieveVideo), pathOnly+forwardedQuery, k.config.capabilities)
	}
}

func (k *klingProvider) textCreateVideoPath() string {
	return klingCapabilityPath(k.config.capabilities, ApiNameVideos, klingTextToVideoPath)
}

func (k *klingProvider) imageCreateVideoPath() string {
	return klingCapabilityPath(k.config.capabilities, ApiNameKlingImageToVideo, klingImageToVideoPath)
}

func (k *klingProvider) textRetrieveVideoPath() string {
	return klingCapabilityPath(k.config.capabilities, ApiNameRetrieveVideo, klingTextToVideoTaskPath)
}

func (k *klingProvider) imageRetrieveVideoPath() string {
	return klingCapabilityPath(k.config.capabilities, ApiNameKlingRetrieveImageVideo, klingImageToVideoTaskPath)
}

func klingCapabilityPath(capabilities map[string]string, apiName ApiName, fallback string) string {
	if path := capabilities[string(apiName)]; path != "" {
		return path
	}
	return fallback
}

func replaceKlingVideoID(taskPath, videoID string) string {
	return strings.Replace(taskPath, "{video_id}", videoID, 1)
}

func klingPathWithExistingQuery(currentPath, targetPath string) string {
	_, query := splitKlingPathAndQuery(currentPath)
	return appendKlingQuery(targetPath, query)
}

func klingPathWithOriginalQuery(ctx wrapper.HttpContext, currentPath, targetPath string) string {
	if originPath, ok := ctx.GetContext(CtxRequestPath).(string); ok && originPath != "" {
		_, query := splitKlingPathAndQuery(originPath)
		return appendKlingQuery(targetPath, query)
	}
	return klingPathWithExistingQuery(currentPath, targetPath)
}

func appendKlingQuery(targetPath, query string) string {
	if query == "" {
		return targetPath
	}
	query = strings.TrimPrefix(query, "?")
	if query == "" {
		return targetPath
	}
	targetPathOnly, targetQuery := splitKlingPathAndQuery(targetPath)
	targetQuery = strings.TrimPrefix(targetQuery, "?")
	if targetQuery == "" {
		return targetPathOnly + "?" + query
	}
	return targetPathOnly + "?" + mergeKlingQueryParts(targetQuery, query)
}

func mergeKlingQueryParts(baseQuery, extraQuery string) string {
	parts := make([]string, 0)
	seen := make(map[string]struct{})
	for _, part := range strings.Split(baseQuery, "&") {
		if part == "" {
			continue
		}
		parts = append(parts, part)
		seen[part] = struct{}{}
	}
	for _, part := range strings.Split(extraQuery, "&") {
		if part == "" {
			continue
		}
		if _, exists := seen[part]; exists {
			continue
		}
		parts = append(parts, part)
		seen[part] = struct{}{}
	}
	return strings.Join(parts, "&")
}

func splitKlingPathAndQuery(rawPath string) (string, string) {
	queryIndex := strings.Index(rawPath, "?")
	if queryIndex < 0 {
		return rawPath, ""
	}
	return rawPath[:queryIndex], rawPath[queryIndex:]
}

func extractKlingTaskTypeQuery(query string) (string, string) {
	if query == "" {
		return "", ""
	}

	parts := strings.Split(strings.TrimPrefix(query, "?"), "&")
	forwardedParts := make([]string, 0, len(parts))
	taskType := ""
	for _, part := range parts {
		if part == "" {
			continue
		}

		key, value, _ := strings.Cut(part, "=")
		decodedKey, err := url.QueryUnescape(key)
		if err != nil {
			decodedKey = key
		}
		if decodedKey != klingTaskTypeQueryKey {
			forwardedParts = append(forwardedParts, part)
			continue
		}

		decodedValue, err := url.QueryUnescape(value)
		if err != nil {
			decodedValue = value
		}
		// If repeated, the last task type wins; all task type hints are stripped before forwarding.
		taskType = normalizeKlingTaskType(decodedValue)
	}

	if len(forwardedParts) == 0 {
		return taskType, ""
	}
	return taskType, "?" + strings.Join(forwardedParts, "&")
}

func normalizeKlingTaskType(taskType string) string {
	switch strings.ToLower(strings.TrimSpace(taskType)) {
	case klingTaskTypeImageToVideo, "image", "i2v":
		return klingTaskTypeImageToVideo
	case klingTaskTypeTextToVideo, "text", "t2v":
		return klingTaskTypeTextToVideo
	default:
		return ""
	}
}

func prefixKlingTaskIDs(body []byte, prefix string) ([]byte, error) {
	var err error
	for _, path := range []string{"data.task_id", "task_id"} {
		value := gjson.GetBytes(body, path)
		if !value.Exists() || value.String() == "" {
			continue
		}
		taskID := value.String()
		if strings.HasPrefix(taskID, klingTextTaskIDPrefix) || strings.HasPrefix(taskID, klingImageTaskIDPrefix) {
			continue
		}
		body, err = sjson.SetBytes(body, path, prefix+taskID)
		if err != nil {
			return nil, err
		}
	}
	return body, nil
}

func (k *klingProvider) isImageToVideoRequest(body []byte) bool {
	// Keep this in sync with Kling video generation image input fields.
	imageFields := []string{
		"image",
		"image_url",
		"image_urls",
		"images",
		"image_tail",
		"image_tail_url",
		"input_image",
		"first_frame_image",
		"last_frame_image",
	}
	for _, field := range imageFields {
		if gjson.GetBytes(body, field).Exists() {
			return true
		}
	}
	return false
}
