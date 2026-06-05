package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"net/http"
	"strings"
	"time"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-proxy/provider"
	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-proxy/util"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

// @Name ai-proxy
// @Category custom
// @Phase UNSPECIFIED_PHASE
// @Priority 0
// @Title zh-CN AI代理
// @Description zh-CN 通过AI助手提供智能对话服务
// @IconUrl https://img.alicdn.com/imgextra/i1/O1CN018iKKih1iVx287RltL_!!6000000004419-2-tps-42-42.png
// @Version 0.1.0
//
// @Contact.name CH3CHO
// @Contact.url https://github.com/CH3CHO
// @Contact.email ch3cho@qq.com
//
// @Example
// { "provider": { "type": "qwen", "apiToken": "YOUR_DASHSCOPE_API_TOKEN", "modelMapping": { "*": "qwen-turbo" } } }
// @End
type PluginConfig struct {
	// @Title zh-CN AI服务提供商配置
	// @Description zh-CN AI服务提供商配置，包含API接口、模型和知识库文件等信息
	providerConfigs []provider.ProviderConfig `required:"true" yaml:"providers"`

	activeProviderConfig *provider.ProviderConfig `yaml:"-"`
	activeProvider       provider.Provider        `yaml:"-"`
	providersByID        map[string]providerRuntime
	sessionAffinity      sessionAffinityConfig
}

type providerRuntime struct {
	config   *provider.ProviderConfig
	provider provider.Provider
}

type sessionAffinityConfig struct {
	Enabled           bool
	Type              string
	ProviderIDs       []string
	Scope             []string
	Key               sessionAffinityKeyConfig
	OnUnavailable     string
	UnavailableStatus []string
	FallbackTimeout   uint64
	MaxBodyBytes      uint64
	TTLSeconds        int64
	providerOrder     []string
	sharedDataKey     string
}

type sessionAffinityKeyConfig struct {
	Source       string
	Name         string
	JSONPath     string
	PropertyPath []string
}

type sessionAffinityRecord struct {
	ProviderID string `json:"providerId"`
	ExpiresAt  int64  `json:"expiresAt"`
}

const (
	sessionAffinityModeHash       = "hash"
	sessionAffinityModePersistent = "persistent"

	sessionAffinityUnavailableFailFast              = "failFast"
	sessionAffinityUnavailableFallbackAndUpdate     = "fallbackAndUpdate"
	sessionAffinityUnavailableFallbackWithoutUpdate = "fallbackWithoutUpdate"

	sessionAffinityKeySourceHeader   = "header"
	sessionAffinityKeySourceCookie   = "cookie"
	sessionAffinityKeySourceBody     = "body"
	sessionAffinityKeySourceMetadata = "metadata"

	defaultSessionAffinityHeader          = "x-mse-consumer"
	defaultSessionAffinityMaxBodyBytes    = 100 * 1024 * 1024
	defaultSessionAffinityTTLSeconds      = 3600
	defaultSessionAffinityFallbackTimeout = 60 * 1000
	maxUint32Value                        = uint64(^uint32(0))

	ctxSelectedProviderID         = "aiProxySelectedProviderID"
	ctxAffinityKey                = "aiProxySessionAffinityKey"
	userAttributeSelectedProvider = "ai_proxy_selected_provider"
)

func (c *PluginConfig) FromJson(json gjson.Result) {
	if providersJson := json.Get("providers"); providersJson.Exists() && providersJson.IsArray() {
		c.providerConfigs = make([]provider.ProviderConfig, 0)
		for _, providerJson := range providersJson.Array() {
			providerConfig := provider.ProviderConfig{}
			providerConfig.FromJson(providerJson)
			c.providerConfigs = append(c.providerConfigs, providerConfig)
		}
	}

	if providerJson := json.Get("provider"); providerJson.Exists() && providerJson.IsObject() {
		// TODO: For legacy config support. To be removed later.
		providerConfig := provider.ProviderConfig{}
		providerConfig.FromJson(providerJson)
		c.providerConfigs = []provider.ProviderConfig{providerConfig}
		c.activeProviderConfig = &providerConfig
		c.sessionAffinity = parseSessionAffinityConfig(json.Get("sessionAffinity"))
		// Legacy configuration is used and the active provider is determined.
		// We don't need to continue with the new configuration style.
		return
	}

	c.activeProviderConfig = nil
	c.sessionAffinity = parseSessionAffinityConfig(json.Get("sessionAffinity"))

	activeProviderId := json.Get("activeProviderId").String()
	if activeProviderId != "" {
		for _, providerConfig := range c.providerConfigs {
			if providerConfig.GetId() == activeProviderId {
				c.activeProviderConfig = &providerConfig
				break
			}
		}
	}
}

func (c *PluginConfig) Validate() error {
	if err := c.sessionAffinity.Validate(c.providerConfigs); err != nil {
		return err
	}
	if c.sessionAffinity.Enabled {
		for i := range c.providerConfigs {
			if err := c.providerConfigs[i].Validate(); err != nil {
				return err
			}
		}
		return nil
	}
	if c.activeProviderConfig == nil {
		return nil
	}
	if err := c.activeProviderConfig.Validate(); err != nil {
		return err
	}
	return nil
}

func (c *PluginConfig) Complete() error {
	c.providersByID = nil
	if c.sessionAffinity.Enabled {
		c.providersByID = make(map[string]providerRuntime, len(c.providerConfigs))
		for i := range c.providerConfigs {
			providerConfig := &c.providerConfigs[i]
			activeProvider, err := provider.CreateProvider(*providerConfig)
			if err != nil {
				return err
			}
			if err := providerConfig.SetApiTokensFailover(activeProvider); err != nil {
				return err
			}
			c.providersByID[providerConfig.GetId()] = providerRuntime{
				config:   providerConfig,
				provider: activeProvider,
			}
		}
		if c.activeProviderConfig != nil {
			selected := c.providersByID[c.activeProviderConfig.GetId()]
			c.activeProviderConfig = selected.config
			c.activeProvider = selected.provider
		} else if len(c.sessionAffinity.providerOrder) > 0 {
			selected := c.providersByID[c.sessionAffinity.providerOrder[0]]
			c.activeProviderConfig = selected.config
			c.activeProvider = selected.provider
		}
		return nil
	}

	if c.activeProviderConfig == nil {
		c.activeProvider = nil
		return nil
	}

	var err error

	c.activeProvider, err = provider.CreateProvider(*c.activeProviderConfig)
	if err != nil {
		return err
	}

	providerConfig := c.GetProviderConfig()
	return providerConfig.SetApiTokensFailover(c.activeProvider)
}

func (c *PluginConfig) GetProvider(ctx ...wrapper.HttpContext) provider.Provider {
	if selected := c.getSelectedProvider(ctx...); selected != nil {
		return selected.provider
	}
	return c.activeProvider
}

func (c *PluginConfig) GetProviderConfig(ctx ...wrapper.HttpContext) *provider.ProviderConfig {
	if selected := c.getSelectedProvider(ctx...); selected != nil {
		return selected.config
	}
	return c.activeProviderConfig
}

// SetActiveProviderForTest replaces the runtime Provider after Complete(); intended for unit tests in package main only.
func (c *PluginConfig) SetActiveProviderForTest(p provider.Provider) {
	c.activeProvider = p
}

func (c *PluginConfig) IsSessionAffinityEnabled() bool {
	return c.sessionAffinity.Enabled
}

func (c *PluginConfig) NeedsSessionAffinityRequestBody() bool {
	return c.sessionAffinity.Enabled && c.sessionAffinity.needsRequestBody()
}

func (c *PluginConfig) SessionAffinityMaxBodyBytes() uint32 {
	if c.sessionAffinity.MaxBodyBytes == 0 {
		return defaultSessionAffinityMaxBodyBytes
	}
	return uint32(c.sessionAffinity.MaxBodyBytes)
}

func (c *PluginConfig) NeedsSessionAffinityFallback() bool {
	return c.sessionAffinity.Enabled && c.sessionAffinity.isFallbackEnabled()
}

func (c *PluginConfig) SelectProviderBySessionAffinity(ctx wrapper.HttpContext, body []byte) error {
	if !c.sessionAffinity.Enabled {
		return nil
	}
	key, err := c.sessionAffinity.buildAffinityKey(ctx, body)
	if err != nil {
		return err
	}
	providerID, err := c.sessionAffinity.selectProviderID(key)
	if err != nil {
		return err
	}
	if _, ok := c.providersByID[providerID]; !ok {
		return fmt.Errorf("sessionAffinity selected unknown provider %q", providerID)
	}
	ctx.SetContext(ctxAffinityKey, key)
	ctx.SetContext(ctxSelectedProviderID, providerID)
	ctx.SetUserAttribute(userAttributeSelectedProvider, providerID)
	setPropertySafely([]string{userAttributeSelectedProvider}, []byte(providerID))
	return nil
}

func (c *PluginConfig) HandleProviderUnavailable(ctx wrapper.HttpContext, activeProvider provider.Provider, status string) (types.Action, bool) {
	if !c.sessionAffinity.Enabled || !c.sessionAffinity.shouldHandleUnavailable(status) {
		return types.ActionContinue, false
	}
	switch c.sessionAffinity.OnUnavailable {
	case sessionAffinityUnavailableFailFast:
		_ = proxywasm.SendHttpResponse(503, nil, []byte("selected provider unavailable"), -1)
		return types.HeaderStopAllIterationAndWatermark, true
	case sessionAffinityUnavailableFallbackAndUpdate, sessionAffinityUnavailableFallbackWithoutUpdate:
		selected, fallback, ok := c.nextFallbackProvider(ctx)
		if !ok {
			return types.ActionContinue, false
		}
		err := fallback.config.SendProviderFallbackRequest(ctx, fallback.provider, uint32(c.sessionAffinity.FallbackTimeout), func(statusCode int, responseHeaders http.Header, responseBody []byte) {
			if statusCode >= 200 && statusCode < 300 {
				if c.sessionAffinity.OnUnavailable == sessionAffinityUnavailableFallbackAndUpdate {
					c.sessionAffinity.updatePersistentRecord(ctx.GetStringContext(ctxAffinityKey, ""), fallback.config.GetId())
				}
				headers, body := fallback.config.TransformFallbackResponse(ctx, fallback.provider, responseHeaders, responseBody)
				_ = proxywasm.SendHttpResponse(uint32(statusCode), headers, body, -1)
				return
			}
			_ = selected
			proxywasm.ResumeHttpResponse()
		})
		if err != nil {
			return types.ActionContinue, false
		}
		_ = activeProvider
		return types.HeaderStopAllIterationAndWatermark, true
	default:
		return types.ActionContinue, false
	}
}

func (c *PluginConfig) nextFallbackProvider(ctx wrapper.HttpContext) (providerRuntime, providerRuntime, bool) {
	selectedID := ctx.GetStringContext(ctxSelectedProviderID, "")
	if selectedID == "" || len(c.sessionAffinity.providerOrder) < 2 {
		return providerRuntime{}, providerRuntime{}, false
	}
	selected, ok := c.providersByID[selectedID]
	if !ok {
		return providerRuntime{}, providerRuntime{}, false
	}
	for i, id := range c.sessionAffinity.providerOrder {
		if id != selectedID {
			continue
		}
		for offset := 1; offset < len(c.sessionAffinity.providerOrder); offset++ {
			nextID := c.sessionAffinity.providerOrder[(i+offset)%len(c.sessionAffinity.providerOrder)]
			if fallback, ok := c.providersByID[nextID]; ok {
				return selected, fallback, true
			}
		}
	}
	return providerRuntime{}, providerRuntime{}, false
}

func (c *PluginConfig) getSelectedProvider(ctx ...wrapper.HttpContext) *providerRuntime {
	if len(ctx) == 0 || ctx[0] == nil {
		return nil
	}
	providerID := ctx[0].GetStringContext(ctxSelectedProviderID, "")
	if providerID == "" {
		return nil
	}
	selected, ok := c.providersByID[providerID]
	if !ok {
		return nil
	}
	return &selected
}

func parseSessionAffinityConfig(json gjson.Result) sessionAffinityConfig {
	cfg := sessionAffinityConfig{
		Type:              sessionAffinityModeHash,
		MaxBodyBytes:      defaultSessionAffinityMaxBodyBytes,
		TTLSeconds:        defaultSessionAffinityTTLSeconds,
		OnUnavailable:     sessionAffinityUnavailableFailFast,
		FallbackTimeout:   defaultSessionAffinityFallbackTimeout,
		UnavailableStatus: []string{"5.*"},
		Key: sessionAffinityKeyConfig{
			Source: sessionAffinityKeySourceHeader,
			Name:   defaultSessionAffinityHeader,
		},
	}
	if !json.Exists() {
		return cfg
	}
	cfg.Enabled = json.Get("enabled").Bool()
	if typ := strings.TrimSpace(json.Get("mode").String()); typ != "" {
		cfg.Type = typ
	} else if typ := strings.TrimSpace(json.Get("type").String()); typ != "" {
		cfg.Type = typ
	}
	for _, id := range json.Get("providerIds").Array() {
		if value := strings.TrimSpace(id.String()); value != "" {
			cfg.ProviderIDs = append(cfg.ProviderIDs, value)
		}
	}
	for _, scope := range json.Get("scope").Array() {
		if value := strings.TrimSpace(scope.String()); value != "" {
			cfg.Scope = append(cfg.Scope, value)
		}
	}
	if onUnavailable := strings.TrimSpace(json.Get("onProviderUnavailable").String()); onUnavailable != "" {
		cfg.OnUnavailable = normalizeOnProviderUnavailable(onUnavailable)
	}
	if timeout := json.Get("fallbackTimeout").Uint(); timeout > 0 {
		cfg.FallbackTimeout = timeout
	} else if timeout := json.Get("fallback_timeout").Uint(); timeout > 0 {
		cfg.FallbackTimeout = timeout
	}
	if statuses := json.Get("unavailableStatus"); statuses.Exists() {
		cfg.UnavailableStatus = nil
		for _, status := range statuses.Array() {
			if value := strings.TrimSpace(status.String()); value != "" {
				cfg.UnavailableStatus = append(cfg.UnavailableStatus, value)
			}
		}
	}
	if maxBodyBytes := json.Get("max_body_bytes").Uint(); maxBodyBytes > 0 {
		cfg.MaxBodyBytes = maxBodyBytes
	}
	if ttlSeconds := json.Get("ttlSeconds").Int(); ttlSeconds > 0 {
		cfg.TTLSeconds = ttlSeconds
	} else if ttlSeconds := json.Get("ttl_seconds").Int(); ttlSeconds > 0 {
		cfg.TTLSeconds = ttlSeconds
	}
	if keyJson := json.Get("key"); keyJson.Exists() {
		if source := strings.TrimSpace(keyJson.Get("source").String()); source != "" {
			cfg.Key.Source = source
		}
		if name := strings.TrimSpace(keyJson.Get("name").String()); name != "" {
			cfg.Key.Name = name
		}
		if jsonPath := normalizeBodyPath(keyJson.Get("jsonPath").String()); jsonPath != "" {
			cfg.Key.JSONPath = jsonPath
		} else if jsonPath := normalizeBodyPath(keyJson.Get("json_path").String()); jsonPath != "" {
			cfg.Key.JSONPath = jsonPath
		}
		for _, part := range keyJson.Get("propertyPath").Array() {
			if value := strings.TrimSpace(part.String()); value != "" {
				cfg.Key.PropertyPath = append(cfg.Key.PropertyPath, value)
			}
		}
		if len(cfg.Key.PropertyPath) == 0 {
			for _, part := range keyJson.Get("property_path").Array() {
				if value := strings.TrimSpace(part.String()); value != "" {
					cfg.Key.PropertyPath = append(cfg.Key.PropertyPath, value)
				}
			}
		}
	}
	return cfg
}

func (c *sessionAffinityConfig) Validate(providerConfigs []provider.ProviderConfig) error {
	if !c.Enabled {
		return nil
	}
	c.providerOrder = nil
	if c.Type != sessionAffinityModeHash && c.Type != sessionAffinityModePersistent {
		return fmt.Errorf("unsupported sessionAffinity.mode %q", c.Type)
	}
	if len(providerConfigs) == 0 {
		return fmt.Errorf("sessionAffinity requires providers")
	}
	providerIDs := map[string]struct{}{}
	for _, providerConfig := range providerConfigs {
		id := providerConfig.GetId()
		if id == "" {
			return fmt.Errorf("sessionAffinity requires each provider to have id")
		}
		if _, exists := providerIDs[id]; exists {
			return fmt.Errorf("duplicate provider id %q", id)
		}
		providerIDs[id] = struct{}{}
	}
	if len(c.ProviderIDs) == 0 {
		for _, providerConfig := range providerConfigs {
			c.providerOrder = append(c.providerOrder, providerConfig.GetId())
		}
	} else {
		for _, id := range c.ProviderIDs {
			if _, ok := providerIDs[id]; !ok {
				return fmt.Errorf("sessionAffinity.providerIds contains unknown provider %q", id)
			}
			c.providerOrder = append(c.providerOrder, id)
		}
	}
	if len(c.providerOrder) == 0 {
		return fmt.Errorf("sessionAffinity requires at least one selectable provider")
	}
	if c.MaxBodyBytes > maxUint32Value {
		return fmt.Errorf("sessionAffinity.max_body_bytes exceeds uint32 limit")
	}
	if c.FallbackTimeout > maxUint32Value {
		return fmt.Errorf("sessionAffinity.fallbackTimeout exceeds uint32 limit")
	}
	if c.Type == sessionAffinityModePersistent && c.TTLSeconds <= 0 {
		return fmt.Errorf("sessionAffinity.ttlSeconds must be positive when mode is persistent")
	}
	switch c.OnUnavailable {
	case sessionAffinityUnavailableFailFast, sessionAffinityUnavailableFallbackAndUpdate, sessionAffinityUnavailableFallbackWithoutUpdate:
	default:
		return fmt.Errorf("unsupported sessionAffinity.onProviderUnavailable %q", c.OnUnavailable)
	}
	switch c.Key.Source {
	case sessionAffinityKeySourceHeader:
		if c.Key.Name == "" {
			c.Key.Name = defaultSessionAffinityHeader
		}
	case sessionAffinityKeySourceCookie:
		if c.Key.Name == "" {
			return fmt.Errorf("sessionAffinity.key.name is required when source is cookie")
		}
	case sessionAffinityKeySourceBody:
		if c.Key.JSONPath == "" {
			return fmt.Errorf("sessionAffinity.key.jsonPath is required when source is body")
		}
	case sessionAffinityKeySourceMetadata:
		if len(c.Key.PropertyPath) == 0 {
			if c.Key.Name == "" {
				return fmt.Errorf("sessionAffinity.key.name or key.propertyPath is required when source is metadata")
			}
			c.Key.PropertyPath = []string{"metadata", c.Key.Name}
		}
	default:
		return fmt.Errorf("unsupported sessionAffinity.key.source %q", c.Key.Source)
	}
	c.sharedDataKey = "ai-proxy-session-affinity:" + stableHash(strings.Join(c.providerOrder, ","))
	return nil
}

func (c sessionAffinityConfig) buildAffinityKey(ctx wrapper.HttpContext, body []byte) (string, error) {
	key, err := c.extractKey(body)
	if err != nil {
		return "", err
	}
	parts := []string{"key=" + key}
	for _, scope := range c.Scope {
		value := c.extractScopeValue(ctx, body, scope)
		if value == "" {
			continue
		}
		parts = append(parts, scope+"="+value)
	}
	return strings.Join(parts, "|"), nil
}

func (c sessionAffinityConfig) extractKey(body []byte) (string, error) {
	var key string
	switch c.Key.Source {
	case sessionAffinityKeySourceHeader:
		value, err := proxywasm.GetHttpRequestHeader(c.Key.Name)
		if err != nil {
			return "", fmt.Errorf("missing session affinity header %q", c.Key.Name)
		}
		key = value
	case sessionAffinityKeySourceCookie:
		cookieHeader, err := proxywasm.GetHttpRequestHeader("cookie")
		if err != nil {
			return "", fmt.Errorf("missing cookie header")
		}
		request := http.Request{Header: http.Header{"Cookie": []string{cookieHeader}}}
		cookie, err := request.Cookie(c.Key.Name)
		if err != nil {
			return "", fmt.Errorf("missing session affinity cookie %q", c.Key.Name)
		}
		key = cookie.Value
	case sessionAffinityKeySourceBody:
		key = gjson.GetBytes(body, c.Key.JSONPath).String()
	case sessionAffinityKeySourceMetadata:
		value, err := proxywasm.GetProperty(c.Key.PropertyPath)
		if err != nil {
			return "", fmt.Errorf("missing session affinity metadata %q", strings.Join(c.Key.PropertyPath, "."))
		}
		key = string(value)
	default:
		return "", fmt.Errorf("unsupported sessionAffinity.key.source %q", c.Key.Source)
	}
	if key == "" {
		return "", fmt.Errorf("empty session affinity key")
	}
	return key, nil
}

func (c sessionAffinityConfig) extractScopeValue(ctx wrapper.HttpContext, body []byte, scope string) string {
	switch scope {
	case "consumer":
		value, _ := proxywasm.GetHttpRequestHeader(defaultSessionAffinityHeader)
		return value
	case "route":
		return ctx.Path()
	case "model":
		return gjson.GetBytes(body, "model").String()
	default:
		if strings.HasPrefix(scope, "metadata.") {
			path := strings.Split(scope, ".")
			value, err := proxywasm.GetProperty(path)
			if err != nil {
				return ""
			}
			return string(value)
		}
		return ""
	}
}

func (c sessionAffinityConfig) needsRequestBody() bool {
	if c.Key.Source == sessionAffinityKeySourceBody {
		return true
	}
	for _, scope := range c.Scope {
		if scope == "model" {
			return true
		}
	}
	return false
}

func (c sessionAffinityConfig) selectProviderID(key string) (string, error) {
	if c.Type == sessionAffinityModePersistent {
		return c.selectPersistentProviderID(key)
	}
	return c.selectHashProviderID(key)
}

func (c sessionAffinityConfig) selectHashProviderID(key string) (string, error) {
	count := len(c.providerOrder)
	if count == 0 {
		return "", fmt.Errorf("sessionAffinity has no selectable provider")
	}
	if count == 1 {
		return c.providerOrder[0], nil
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return c.providerOrder[int(h.Sum32())%count], nil
}

func (c sessionAffinityConfig) selectPersistentProviderID(key string) (string, error) {
	now := time.Now().Unix()
	records, cas, err := c.loadPersistentRecords()
	if err != nil {
		return c.selectHashProviderID(key)
	}
	pruneExpiredPersistentRecords(records, now)
	if record, ok := records[key]; ok && record.ExpiresAt > now && c.hasProvider(record.ProviderID) {
		return record.ProviderID, nil
	}

	providerID, err := c.selectHashProviderID(key)
	if err != nil {
		return "", err
	}
	records[key] = sessionAffinityRecord{
		ProviderID: providerID,
		ExpiresAt:  now + c.TTLSeconds,
	}
	if err := c.storePersistentRecords(records, cas); err != nil {
		return "", err
	}
	return providerID, nil
}

func (c sessionAffinityConfig) updatePersistentRecord(key, providerID string) {
	if key == "" || providerID == "" {
		return
	}
	now := time.Now().Unix()
	records, cas, err := c.loadPersistentRecords()
	if err != nil {
		return
	}
	pruneExpiredPersistentRecords(records, now)
	records[key] = sessionAffinityRecord{
		ProviderID: providerID,
		ExpiresAt:  now + c.TTLSeconds,
	}
	_ = c.storePersistentRecords(records, cas)
}

func pruneExpiredPersistentRecords(records map[string]sessionAffinityRecord, now int64) {
	for key, record := range records {
		if record.ExpiresAt <= now {
			delete(records, key)
		}
	}
}

func (c sessionAffinityConfig) loadPersistentRecords() (map[string]sessionAffinityRecord, uint32, error) {
	data, cas, err := proxywasm.GetSharedData(c.sharedDataKey)
	if errors.Is(err, types.ErrorStatusNotFound) || len(data) == 0 {
		return map[string]sessionAffinityRecord{}, cas, nil
	}
	if err != nil {
		return map[string]sessionAffinityRecord{}, cas, err
	}
	records := map[string]sessionAffinityRecord{}
	if err := json.Unmarshal(data, &records); err != nil {
		return map[string]sessionAffinityRecord{}, cas, err
	}
	return records, cas, nil
}

func (c sessionAffinityConfig) storePersistentRecords(records map[string]sessionAffinityRecord, cas uint32) error {
	data, err := json.Marshal(records)
	if err != nil {
		return err
	}
	for attempt := 0; attempt < 3; attempt++ {
		err = proxywasm.SetSharedData(c.sharedDataKey, data, cas)
		if err == nil {
			return nil
		}
		if !errors.Is(err, types.ErrorStatusCasMismatch) {
			return err
		}
		latest, latestCas, loadErr := c.loadPersistentRecords()
		if loadErr != nil {
			return loadErr
		}
		for key, record := range records {
			latest[key] = record
		}
		records = latest
		cas = latestCas
		data, err = json.Marshal(records)
		if err != nil {
			return err
		}
	}
	return err
}

func (c sessionAffinityConfig) hasProvider(providerID string) bool {
	for _, id := range c.providerOrder {
		if id == providerID {
			return true
		}
	}
	return false
}

func (c sessionAffinityConfig) isFallbackEnabled() bool {
	return c.OnUnavailable == sessionAffinityUnavailableFallbackAndUpdate ||
		c.OnUnavailable == sessionAffinityUnavailableFallbackWithoutUpdate
}

func (c sessionAffinityConfig) shouldHandleUnavailable(status string) bool {
	if status == "" {
		return false
	}
	return util.MatchStatus(status, c.UnavailableStatus)
}

func normalizeOnProviderUnavailable(value string) string {
	switch strings.ToLower(strings.ReplaceAll(value, "_", "")) {
	case "failfast":
		return sessionAffinityUnavailableFailFast
	case "fallbackandupdate":
		return sessionAffinityUnavailableFallbackAndUpdate
	case "fallbackwithoutupdate":
		return sessionAffinityUnavailableFallbackWithoutUpdate
	default:
		return value
	}
}

func stableHash(value string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(value))
	return fmt.Sprintf("%x", h.Sum32())
}

func setPropertySafely(path []string, value []byte) {
	defer func() {
		_ = recover()
	}()
	_ = proxywasm.SetProperty(path, value)
}

func normalizeBodyPath(path string) string {
	path = strings.TrimSpace(path)
	path = strings.TrimPrefix(path, "$.")
	if path == "$" {
		return ""
	}
	return path
}
