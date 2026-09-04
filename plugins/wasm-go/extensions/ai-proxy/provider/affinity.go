package provider

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-proxy/util"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
	"github.com/tidwall/resp"
)

const (
	defaultAffinityBindingTTLDays = 7
	defaultAffinityMaxRetries     = 3
	maxAffinityRetries            = 10
	defaultAffinityRetryTimeout   = int64(30_000)
	defaultAffinityRedisTimeout   = int64(1_000)
	affinityRedisKeyPrefix        = "higress:ai-proxy:api-key-affinity:v1:"
	ctxAffinityBinding            = "ai-proxy-affinity-binding"
	ctxAffinityConsumer           = "ai-proxy-affinity-consumer"
	ctxAffinityExcludedTokens     = "ai-proxy-affinity-excluded-token-fingerprints"
	ctxAffinityRetryPrepared      = "ai-proxy-affinity-retry-prepared"
	ctxAffinityRetryReentry       = "ai-proxy-affinity-retry-reentry"
)

// affinityRedisConfig describes the Redis instance used for affinity bindings.
type affinityRedisConfig struct {
	serviceName string
	servicePort int
	username    string
	password    string
	database    int
	timeout     int64
}

// apiKeyAffinity stores the affinity policy for one provider; the Redis client is runtime-only.
type apiKeyAffinity struct {
	enabled        bool
	redis          affinityRedisConfig
	bindingTTLDays int
	maxRetries     int
	retryTimeout   int64
	redisClient    wrapper.RedisClient
}

// affinityBinding stores only a token fingerprint so API keys are never written to Redis.
type affinityBinding struct {
	TokenFingerprint string `json:"tokenFingerprint"`
	RenewAfter       int64  `json:"renewAfter"`
}

// fromJSON applies affinity defaults while the provider configuration is decoded.
func (a *apiKeyAffinity) fromJSON(jsonValue gjson.Result) {
	a.enabled = jsonValue.Get("enabled").Bool()
	a.redis.serviceName = jsonValue.Get("redis.serviceName").String()
	a.redis.servicePort = int(jsonValue.Get("redis.servicePort").Int())
	if a.redis.servicePort == 0 {
		if strings.HasSuffix(a.redis.serviceName, ".static") {
			a.redis.servicePort = 80
		} else {
			a.redis.servicePort = 6379
		}
	}
	a.redis.username = jsonValue.Get("redis.username").String()
	a.redis.password = jsonValue.Get("redis.password").String()
	a.redis.database = int(jsonValue.Get("redis.database").Int())
	a.redis.timeout = jsonValue.Get("redis.timeout").Int()
	if !jsonValue.Get("redis.timeout").Exists() {
		a.redis.timeout = defaultAffinityRedisTimeout
	}

	a.bindingTTLDays = int(jsonValue.Get("bindingTTLDays").Int())
	if !jsonValue.Get("bindingTTLDays").Exists() {
		a.bindingTTLDays = defaultAffinityBindingTTLDays
	}
	a.maxRetries = int(jsonValue.Get("maxRetries").Int())
	if !jsonValue.Get("maxRetries").Exists() {
		a.maxRetries = defaultAffinityMaxRetries
	}
	a.retryTimeout = jsonValue.Get("retryTimeout").Int()
	if !jsonValue.Get("retryTimeout").Exists() {
		a.retryTimeout = defaultAffinityRetryTimeout
	}
}

// validate checks affinity dependencies only when the feature is explicitly enabled.
func (a *apiKeyAffinity) validate() error {
	if !a.enabled {
		return nil
	}
	if a.redis.serviceName == "" {
		return errors.New("apiKeyAffinity.redis.serviceName must not be empty when apiKeyAffinity is enabled")
	}
	if a.redis.servicePort <= 0 {
		return errors.New("apiKeyAffinity.redis.servicePort must be greater than 0")
	}
	if a.redis.timeout <= 0 {
		return errors.New("apiKeyAffinity.redis.timeout must be greater than 0")
	}
	if a.bindingTTLDays <= 0 {
		return errors.New("apiKeyAffinity.bindingTTLDays must be greater than 0")
	}
	if a.maxRetries < 0 || a.maxRetries > maxAffinityRetries {
		return fmt.Errorf("apiKeyAffinity.maxRetries must be between 0 and %d", maxAffinityRetries)
	}
	if a.retryTimeout <= 0 {
		return errors.New("apiKeyAffinity.retryTimeout must be greater than 0")
	}
	return nil
}

// affinityRenewAfterDays rounds the 80 percent renewal threshold to a whole day.
func affinityRenewAfterDays(bindingTTLDays int) int {
	return int(math.Floor(float64(bindingTTLDays)*0.8 + 0.5))
}

// tokenFingerprint creates an irreversible identifier for bindings and internal retries.
func tokenFingerprint(token string) string {
	return sha256Hex([]byte(token))
}

// marshal produces the versioned Redis representation of a binding.
func (b affinityBinding) marshal() (string, error) {
	data, err := json.Marshal(b)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// affinityProviderScope hashes the provider and token pool to isolate bindings between upstreams.
func (c *ProviderConfig) affinityProviderScope() string {
	tokenFingerprints := make([]string, 0, len(c.apiTokens))
	for _, token := range c.apiTokens {
		tokenFingerprints = append(tokenFingerprints, tokenFingerprint(token))
	}
	sort.Strings(tokenFingerprints)

	scope := struct {
		ID               string   `json:"id"`
		Type             string   `json:"type"`
		ProviderDomain   string   `json:"providerDomain"`
		OpenAICustomURL  string   `json:"openaiCustomUrl"`
		AzureServiceURL  string   `json:"azureServiceUrl"`
		GenericHost      string   `json:"genericHost"`
		VLLMCustomURL    string   `json:"vllmCustomUrl"`
		VLLMServerHost   string   `json:"vllmServerHost"`
		ProviderBasePath string   `json:"providerBasePath"`
		Tokens           []string `json:"tokens"`
	}{
		ID:               c.id,
		Type:             c.typ,
		ProviderDomain:   c.providerDomain,
		OpenAICustomURL:  c.openaiCustomUrl,
		AzureServiceURL:  c.azureServiceUrl,
		GenericHost:      c.genericHost,
		VLLMCustomURL:    c.vllmCustomUrl,
		VLLMServerHost:   c.vllmServerHost,
		ProviderBasePath: c.providerBasePath,
		Tokens:           tokenFingerprints,
	}
	encoded, _ := json.Marshal(scope)
	return sha256Hex(encoded)
}

// affinityBindingKey isolates providers and consumers without exposing either value.
func (c *ProviderConfig) affinityBindingKey(consumer string) string {
	return affinityRedisKeyPrefix + c.affinityProviderScope() + ":" + sha256Hex([]byte(consumer))
}

// ApiKeyAffinityEnabled reports whether persistent API key affinity is enabled.
func (c *ProviderConfig) ApiKeyAffinityEnabled() bool {
	return c.apiKeyAffinity != nil && c.apiKeyAffinity.enabled
}

// IsApiKeyAffinityRequest reports whether this request established affinity from a consumer identity.
func (c *ProviderConfig) IsApiKeyAffinityRequest(ctx wrapper.HttpContext) bool {
	return c.ApiKeyAffinityEnabled() && ctx.GetStringContext(ctxAffinityConsumer, "") != ""
}

// IsApiKeyAffinityRetryRequest distinguishes internal key retry re-entry from the initial request.
func (c *ProviderConfig) IsApiKeyAffinityRetryRequest(ctx wrapper.HttpContext) bool {
	reentry, _ := ctx.GetContext(ctxAffinityRetryReentry).(bool)
	return reentry
}

// GetApiKeyAffinityConsumer returns the consumer name injected by gateway authentication.
func (c *ProviderConfig) GetApiKeyAffinityConsumer() string {
	consumer, err := proxywasm.GetHttpRequestHeader("x-mse-consumer")
	if err != nil {
		return ""
	}
	return consumer
}

// GetApiKeyAffinityToken prefers the Redis binding and otherwise uses the existing selection algorithm.
func (c *ProviderConfig) GetApiKeyAffinityToken(ctx wrapper.HttpContext) string {
	if !c.ApiKeyAffinityEnabled() {
		return ""
	}
	available := c.GetAvailableApiToken(ctx)
	binding, _ := ctx.GetContext(ctxAffinityBinding).(affinityBinding)
	excluded, _ := ctx.GetContext(ctxAffinityExcludedTokens).(map[string]struct{})
	consumer := ctx.GetStringContext(ctxAffinityConsumer, "")
	if len(available) == 0 {
		if c.IsApiKeyAffinityRetryRequest(ctx) {
			return ""
		}
		// Preserve failover's existing fallback to an unavailable key for the initial request.
		if c.isFailoverEnabled() {
			return c.GetGlobalRandomToken()
		}
		available = c.apiTokens
	}
	if consumer == "" {
		return c.GetRandomToken()
	}
	if binding.TokenFingerprint == "" && len(excluded) == 0 {
		if c.isFailoverEnabled() {
			return c.GetGlobalRandomToken()
		}
		return c.GetOrSetTokenWithContext(ctx)
	}
	return selectAvailableAffinityToken(available, consumer, binding.TokenFingerprint, excluded)
}

// selectAvailableAffinityToken filters failed keys before validating a binding.
func selectAvailableAffinityToken(tokens []string, consumer, bindingFingerprint string, excluded map[string]struct{}) string {
	available := make([]string, 0, len(tokens))
	for _, token := range tokens {
		fingerprint := tokenFingerprint(token)
		if _, failed := excluded[fingerprint]; failed {
			continue
		}
		if bindingFingerprint != "" && fingerprint == bindingFingerprint {
			return token
		}
		available = append(available, token)
	}
	return selectTokenByConsumer(available, consumer)
}

// InitializeApiKeyAffinityRequest accepts control headers only from this plugin's custom_response re-entry.
func (c *ProviderConfig) InitializeApiKeyAffinityRequest(ctx wrapper.HttpContext) {
	if !c.ApiKeyAffinityEnabled() {
		return
	}
	fallbackFrom, _ := proxywasm.GetHttpRequestHeader(util.HeaderHigressFallbackFrom)
	if fallbackFrom != util.ApiKeyAffinityFallbackSource {
		ctx.SetContext(ctxAffinityRetryReentry, false)
		for _, header := range []string{
			util.HeaderApiKeyAffinityRetryCount,
			util.HeaderApiKeyAffinityFailedFingerprints,
			util.HeaderApiKeyAffinityRetryStartedAt,
		} {
			_ = proxywasm.RemoveHttpRequestHeader(header)
		}
		ctx.SetContext(ctxAffinityExcludedTokens, map[string]struct{}{})
		return
	}
	ctx.SetContext(ctxAffinityRetryReentry, true)
	c.limitApiKeyAffinityRetryTimeout()
	_, excluded := readAffinityFailedFingerprints()
	ctx.SetContext(ctxAffinityExcludedTokens, excluded)
}

// limitApiKeyAffinityRetryTimeout applies the remaining timeout budget measured from the first failure.
func (c *ProviderConfig) limitApiKeyAffinityRetryTimeout() {
	startedAt := readPositiveInt64Header(util.HeaderApiKeyAffinityRetryStartedAt)
	if startedAt == 0 {
		return
	}
	remaining := c.apiKeyAffinity.retryTimeout - (time.Now().UnixMilli() - startedAt)
	if remaining < 1 {
		remaining = 1
	} else if remaining > c.apiKeyAffinity.retryTimeout {
		remaining = c.apiKeyAffinity.retryTimeout
	}
	existing := readPositiveInt64Header(util.HeaderEnvoyUpstreamRequestTimeout)
	if existing > 0 && existing <= remaining {
		return
	}
	_ = proxywasm.ReplaceHttpRequestHeader(util.HeaderEnvoyUpstreamRequestTimeout, strconv.FormatInt(remaining, 10))
}

// PrepareApiKeyAffinityRetry converts a failed response into an internal custom_response re-entry signal.
func (c *ProviderConfig) PrepareApiKeyAffinityRetry(ctx wrapper.HttpContext, status string) bool {
	if !c.ApiKeyAffinityEnabled() || !isAffinityRetryStatus(status) {
		return false
	}
	count := readPositiveIntHeader(util.HeaderApiKeyAffinityRetryCount)
	if count >= c.apiKeyAffinity.maxRetries {
		return false
	}
	now := time.Now().UnixMilli()
	startedAt := readPositiveInt64Header(util.HeaderApiKeyAffinityRetryStartedAt)
	if startedAt > 0 && now-startedAt >= c.apiKeyAffinity.retryTimeout {
		return false
	}
	if startedAt == 0 {
		startedAt = now
	}

	orderedFailed, excluded := readAffinityFailedFingerprints()
	currentToken := c.GetApiTokenInUse(ctx)
	if currentToken == "" {
		return false
	}
	currentFingerprint := tokenFingerprint(currentToken)
	if _, exists := excluded[currentFingerprint]; !exists {
		excluded[currentFingerprint] = struct{}{}
		orderedFailed = append(orderedFailed, currentFingerprint)
	}

	available := c.GetAvailableApiToken(ctx)
	if selectAvailableAffinityToken(available, ctx.GetStringContext(ctxAffinityConsumer, ""), "", excluded) == "" {
		return false
	}

	_ = proxywasm.ReplaceHttpRequestHeader(util.HeaderApiKeyAffinityRetryCount, strconv.Itoa(count+1))
	_ = proxywasm.ReplaceHttpRequestHeader(util.HeaderApiKeyAffinityFailedFingerprints, strings.Join(orderedFailed, ","))
	_ = proxywasm.ReplaceHttpRequestHeader(util.HeaderApiKeyAffinityRetryStartedAt, strconv.FormatInt(startedAt, 10))
	_ = proxywasm.ReplaceHttpResponseHeader(":status", "299")
	_ = proxywasm.ReplaceHttpResponseHeader(util.HeaderApiKeyAffinityRetry, "1")
	ctx.SetContext(ctxAffinityRetryPrepared, true)
	ctx.SetUserAttribute("api_key_affinity_status", "retrying")
	_ = ctx.WriteUserAttributeToLogWithKey(wrapper.AILogKey)
	return true
}

// ApiKeyAffinityRetryPrepared reports whether custom_response should handle the marked response immediately.
func (c *ProviderConfig) ApiKeyAffinityRetryPrepared(ctx wrapper.HttpContext) bool {
	prepared, _ := ctx.GetContext(ctxAffinityRetryPrepared).(bool)
	return prepared
}

// isAffinityRetryStatus recognizes explicit authentication, billing, and rate-limit key failures.
func isAffinityRetryStatus(status string) bool {
	switch status {
	case "401", "402", "403", "429":
		return true
	default:
		return false
	}
}

// readAffinityFailedFingerprints ignores invalid and duplicate values while bounding header state.
func readAffinityFailedFingerprints() ([]string, map[string]struct{}) {
	raw, _ := proxywasm.GetHttpRequestHeader(util.HeaderApiKeyAffinityFailedFingerprints)
	ordered := make([]string, 0, maxAffinityRetries)
	excluded := make(map[string]struct{}, maxAffinityRetries)
	for _, item := range strings.Split(raw, ",") {
		fingerprint := strings.TrimSpace(item)
		if len(fingerprint) != sha256.Size*2 || len(ordered) >= maxAffinityRetries {
			continue
		}
		if _, exists := excluded[fingerprint]; exists {
			continue
		}
		excluded[fingerprint] = struct{}{}
		ordered = append(ordered, fingerprint)
	}
	return ordered, excluded
}

// readPositiveIntHeader treats invalid control headers as unset.
func readPositiveIntHeader(header string) int {
	value, _ := proxywasm.GetHttpRequestHeader(header)
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return 0
	}
	return parsed
}

// readPositiveInt64Header treats invalid timestamps as unset so the first real failure starts the budget.
func readPositiveInt64Header(header string) int64 {
	value, _ := proxywasm.GetHttpRequestHeader(header)
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 {
		return 0
	}
	return parsed
}

// selectTokenByConsumer uses stable hashing for deterministic initial selection.
func selectTokenByConsumer(tokens []string, consumer string) string {
	if len(tokens) == 0 {
		return ""
	}
	hashValue := fnv.New32a()
	_, _ = hashValue.Write([]byte(consumer))
	return tokens[int(hashValue.Sum32())%len(tokens)]
}

// InitApiKeyAffinity initializes Redis after configuration; runtime failures use RedisClient recovery.
func (c *ProviderConfig) InitApiKeyAffinity() error {
	if !c.ApiKeyAffinityEnabled() {
		return nil
	}
	redisConfig := c.apiKeyAffinity.redis
	client := wrapper.NewRedisClusterClient(wrapper.FQDNCluster{
		FQDN: redisConfig.serviceName,
		Port: int64(redisConfig.servicePort),
	})
	c.apiKeyAffinity.redisClient = client
	return client.Init(
		redisConfig.username,
		redisConfig.password,
		redisConfig.timeout,
		wrapper.WithDataBase(redisConfig.database),
	)
}

// LoadApiKeyAffinity loads a consumer binding asynchronously and degrades without blocking on Redis errors.
func (c *ProviderConfig) LoadApiKeyAffinity(ctx wrapper.HttpContext, consumer string, done func()) error {
	if !c.ApiKeyAffinityEnabled() || consumer == "" {
		return nil
	}
	ctx.SetContext(ctxAffinityConsumer, consumer)
	if c.apiKeyAffinity.redisClient == nil {
		markAffinityRedisDegraded(ctx)
		return errors.New("api key affinity redis client is not initialized")
	}
	key := c.affinityBindingKey(consumer)
	err := c.apiKeyAffinity.redisClient.Get(key, func(response resp.Value) {
		finish := func() {
			c.limitApiKeyAffinityRetryTimeout()
			if done != nil {
				done()
			}
		}
		if response.Error() != nil {
			markAffinityRedisDegraded(ctx)
			finish()
			return
		}
		if !response.IsNull() {
			var binding affinityBinding
			if err := json.Unmarshal([]byte(response.String()), &binding); err != nil || binding.TokenFingerprint == "" {
				markAffinityRedisDegraded(ctx)
			} else {
				ctx.SetContext(ctxAffinityBinding, binding)
			}
		}
		finish()
	})
	if err != nil {
		markAffinityRedisDegraded(ctx)
	}
	return err
}

// PersistApiKeyAffinity writes a new binding or refreshes an existing one within its renewal window.
func (c *ProviderConfig) PersistApiKeyAffinity(ctx wrapper.HttpContext, token string) {
	if !c.ApiKeyAffinityEnabled() || token == "" || c.apiKeyAffinity.redisClient == nil {
		return
	}
	consumer := ctx.GetStringContext(ctxAffinityConsumer, "")
	if consumer == "" {
		return
	}
	fingerprint := tokenFingerprint(token)
	previous, _ := ctx.GetContext(ctxAffinityBinding).(affinityBinding)
	now := time.Now().Unix()
	if previous.TokenFingerprint == fingerprint && previous.RenewAfter > now {
		return
	}
	renewAfter := now + int64(affinityRenewAfterDays(c.apiKeyAffinity.bindingTTLDays))*86400
	binding := affinityBinding{TokenFingerprint: fingerprint, RenewAfter: renewAfter}
	encoded, err := binding.marshal()
	if err != nil {
		markAffinityRedisDegraded(ctx)
		return
	}
	ttl := c.apiKeyAffinity.bindingTTLDays * 86400
	if err := c.apiKeyAffinity.redisClient.SetEx(c.affinityBindingKey(consumer), encoded, ttl, func(response resp.Value) {
		if response.Error() != nil {
			markAffinityRedisDegraded(ctx)
		}
	}); err != nil {
		markAffinityRedisDegraded(ctx)
	}
}

// markAffinityRedisDegraded records the failure in ai_log without blocking the request.
func markAffinityRedisDegraded(ctx wrapper.HttpContext) {
	ctx.SetUserAttribute("api_key_affinity_status", "redis_degraded")
	_ = ctx.WriteUserAttributeToLogWithKey(wrapper.AILogKey)
}
