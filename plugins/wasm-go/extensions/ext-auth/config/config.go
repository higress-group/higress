package config

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"ext-auth/expr"
	"ext-auth/extract"

	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

const (
	DefaultStatusOnError = http.StatusForbidden

	DefaultHttpServiceTimeout = 1000

	DefaultMaxRequestBodyBytes = 10 * 1024 * 1024

	EndpointModeEnvoy       = "envoy"
	EndpointModeForwardAuth = "forward_auth"
)

const (
	// MaxCacheTTL bounds how long an allow decision may be cached, so a stale
	// allow can never outlive it.
	MaxCacheTTL = 600

	// DefaultCacheRedisTimeout is the per-call Redis timeout in milliseconds. It is
	// kept short because the cache sits on the authorization critical path and must
	// fail open rather than stall the request.
	DefaultCacheRedisTimeout = 1000

	// Sources for a cache.key_fields entry. These name where on the incoming request
	// to read a key field, and are distinct from the response-side extract.Source*
	// values used by success_condition / mapped_upstream_headers.
	KeyFieldSourceHeader = "header"
	KeyFieldSourceQuery  = "query"
)

// Operators supported by a success_condition entry.
const (
	OpEq        = "eq"
	OpNe        = "ne"
	OpIn        = "in"
	OpNotIn     = "not_in"
	OpExists    = "exists"
	OpNotExists = "not_exists"
	OpGt        = "gt"
	OpLt        = "lt"
)

type ExtAuthConfig struct {
	HttpService               HttpService
	MatchRules                expr.MatchRules
	FailureModeAllow          bool
	FailureModeAllowHeaderAdd bool
	StatusOnError             uint32
	// SuccessCondition is a flat list of conditions evaluated as an implicit AND.
	SuccessCondition []Condition
	// Cache holds the opt-in auth-result cache. It is active only when Cache.Enabled.
	Cache CacheConfig
}

type HttpService struct {
	EndpointMode string
	Client       wrapper.HttpClient
	// PathPrefix is only used when endpoint_mode is envoy
	PathPrefix string
	// RequestMethod is only used when endpoint_mode is forward_auth
	RequestMethod string
	// Path is only used when endpoint_mode is forward_auth
	Path                  string
	Timeout               uint32
	AuthorizationRequest  AuthorizationRequest
	AuthorizationResponse AuthorizationResponse
}

type AuthorizationRequest struct {
	AllowedHeaders      expr.Matcher
	HeadersToAdd        map[string]string
	WithRequestBody     bool
	MaxRequestBodyBytes uint32
	AllowedProperties   []AllowedProperty
}

type AllowedProperty struct {
	Path   []string
	Header string
}

type AuthorizationResponse struct {
	AllowedUpstreamHeaders expr.Matcher
	AllowedClientHeaders   expr.Matcher
	MappedUpstreamHeaders  []HeaderMapping
}

// HeaderMapping extracts a value from the authorization response by Source and Key,
// then forwards it to the upstream request under the ToHeader name.
type HeaderMapping struct {
	Source   string
	Key      string
	ToHeader string
}

// Condition is a single success_condition entry: extract a value from the
// authorization response by Source and Key, then compare it against Value using Op.
type Condition struct {
	Source string
	Key    string
	Op     string
	Value  []string
}

// CacheConfig holds the auth-result cache settings. Caching is opt-in and
// best-effort: when Enabled is false (the default) no Redis client is created and
// every request goes straight to the authorization server.
type CacheConfig struct {
	Enabled bool
	// TTL is the cache entry lifetime in seconds, clamped to [1, MaxCacheTTL].
	TTL int
	// Client reads and writes cache entries. It is nil when caching is inactive.
	Client wrapper.RedisClient
	// KeyFields, when non-empty, narrows the cache key to these request fields plus
	// the method and query-free path, instead of hashing the full forwarded request.
	// When empty the default full-request key is used.
	KeyFields []CacheKeyField
}

// CacheKeyField names one request field that composes the cache key when
// cache.key_fields is configured. Source is KeyFieldSourceHeader or
// KeyFieldSourceQuery and Key is the header name or query parameter name.
type CacheKeyField struct {
	Source string
	Key    string
}

func ParseConfig(json gjson.Result, config *ExtAuthConfig) error {
	httpServiceConfig := json.Get("http_service")
	if !httpServiceConfig.Exists() {
		return errors.New("missing http_service in config")
	}
	if err := parseHttpServiceConfig(httpServiceConfig, config); err != nil {
		return err
	}

	if err := parseMatchRules(json, config); err != nil {
		return err
	}

	failureModeAllow := json.Get("failure_mode_allow")
	if failureModeAllow.Exists() {
		config.FailureModeAllow = failureModeAllow.Bool()
	}

	failureModeAllowHeaderAdd := json.Get("failure_mode_allow_header_add")
	if failureModeAllowHeaderAdd.Exists() {
		config.FailureModeAllowHeaderAdd = failureModeAllowHeaderAdd.Bool()
	}

	statusOnError := uint32(json.Get("status_on_error").Uint())
	if statusOnError == 0 {
		statusOnError = DefaultStatusOnError
	}
	config.StatusOnError = statusOnError

	if err := parseCacheConfig(json, config); err != nil {
		return err
	}

	return nil
}

// parseCacheConfig reads the optional top-level cache block. Caching stays off
// unless cache.enabled is true with a positive ttl and a Redis service_name. A
// non-positive ttl means the cache gate is not satisfied: caching is silently left
// off rather than failing the whole config, so the request keeps flowing through the
// original cache-free path.
func parseCacheConfig(json gjson.Result, config *ExtAuthConfig) error {
	cacheConfig := json.Get("cache")
	if !cacheConfig.Exists() || !cacheConfig.Get("enabled").Bool() {
		return nil
	}

	ttl := int(cacheConfig.Get("ttl").Int())
	if ttl <= 0 {
		return nil
	}
	if ttl > MaxCacheTTL {
		log.Warnf("cache.ttl %d exceeds the maximum %d and was clamped", ttl, MaxCacheTTL)
		ttl = MaxCacheTTL
	}

	keyFields, err := parseCacheKeyFields(cacheConfig.Get("key_fields"))
	if err != nil {
		return err
	}

	redisConfig := cacheConfig.Get("redis")
	serviceName := redisConfig.Get("service_name").String()
	if serviceName == "" {
		return errors.New("cache.redis.service_name must not be empty when cache is enabled")
	}
	servicePort := redisConfig.Get("service_port").Int()
	if servicePort == 0 {
		if strings.HasSuffix(serviceName, ".static") {
			servicePort = 80
		} else {
			servicePort = 6379
		}
	}
	timeout := redisConfig.Get("timeout").Int()
	if timeout == 0 {
		timeout = DefaultCacheRedisTimeout
	}

	client := wrapper.NewRedisClusterClient(wrapper.FQDNCluster{
		FQDN: serviceName,
		Port: servicePort,
	})
	database := int(redisConfig.Get("database").Int())
	if err := client.Init(redisConfig.Get("username").String(), redisConfig.Get("password").String(), timeout, wrapper.WithDataBase(database)); err != nil {
		return err
	}

	config.Cache = CacheConfig{Enabled: true, TTL: ttl, Client: client, KeyFields: keyFields}
	return nil
}

// parseCacheKeyFields reads the optional cache.key_fields list. An absent list
// yields nil, which selects the default full-request cache key. Any invalid shape
// is a hard error so the whole plugin config is rejected rather than silently
// degrading to a key the operator did not intend.
func parseCacheKeyFields(result gjson.Result) ([]CacheKeyField, error) {
	if !result.Exists() {
		return nil, nil
	}
	if !result.IsArray() {
		return nil, errors.New("cache.key_fields must be an array")
	}

	items := result.Array()
	fields := make([]CacheKeyField, 0, len(items))
	for i, item := range items {
		if !item.IsObject() {
			return nil, fmt.Errorf("cache.key_fields[%d] must be an object with 'source' and 'key'", i)
		}

		source := item.Get("source").String()
		if source != KeyFieldSourceHeader && source != KeyFieldSourceQuery {
			return nil, fmt.Errorf("cache.key_fields[%d]: invalid source %q, must be %q or %q",
				i, source, KeyFieldSourceHeader, KeyFieldSourceQuery)
		}

		key := item.Get("key").String()
		if key == "" {
			return nil, fmt.Errorf("cache.key_fields[%d]: 'key' must not be empty", i)
		}
		// Header names are case-insensitive, so normalize to lower case for stable
		// lookup and a stable digest regardless of how the operator cased the config.
		// Query parameter names are case-sensitive and are kept verbatim.
		if source == KeyFieldSourceHeader {
			key = strings.ToLower(key)
		}

		fields = append(fields, CacheKeyField{Source: source, Key: key})
	}
	return fields, nil
}

func parseHttpServiceConfig(json gjson.Result, config *ExtAuthConfig) error {
	var httpService HttpService

	if err := parseEndpointConfig(json, &httpService); err != nil {
		return err
	}

	timeout := uint32(json.Get("timeout").Uint())
	if timeout == 0 {
		timeout = DefaultHttpServiceTimeout
	}
	httpService.Timeout = timeout

	if err := parseAuthorizationRequestConfig(json, &httpService); err != nil {
		return err
	}

	if err := parseAuthorizationResponseConfig(json, &httpService); err != nil {
		return err
	}

	successCondition, err := parseSuccessCondition(json.Get("success_condition"))
	if err != nil {
		return err
	}
	config.SuccessCondition = successCondition

	config.HttpService = httpService

	return nil
}

func parseEndpointConfig(json gjson.Result, httpService *HttpService) error {
	endpointMode := json.Get("endpoint_mode").String()
	if endpointMode == "" {
		endpointMode = EndpointModeEnvoy
	} else if endpointMode != EndpointModeEnvoy && endpointMode != EndpointModeForwardAuth {
		return errors.New(fmt.Sprintf("endpoint_mode %s is not supported", endpointMode))
	}
	httpService.EndpointMode = endpointMode

	endpointConfig := json.Get("endpoint")
	if !endpointConfig.Exists() {
		return errors.New("missing endpoint in config")
	}

	serviceName := endpointConfig.Get("service_name").String()
	if serviceName == "" {
		return errors.New("endpoint service name must not be empty")
	}
	servicePort := endpointConfig.Get("service_port").Int()
	if servicePort == 0 {
		servicePort = 80
	}
	serviceHost := endpointConfig.Get("service_host").String()

	httpService.Client = wrapper.NewClusterClient(wrapper.FQDNCluster{
		FQDN: serviceName,
		Port: servicePort,
		Host: serviceHost,
	})

	switch endpointMode {
	case EndpointModeEnvoy:
		pathPrefixConfig := endpointConfig.Get("path_prefix")
		if !pathPrefixConfig.Exists() {
			return errors.New("when endpoint_mode is envoy, endpoint path_prefix must not be empty")
		}
		httpService.PathPrefix = pathPrefixConfig.String()

		if endpointConfig.Get("request_method").Exists() || endpointConfig.Get("path").Exists() {
			log.Warn("when endpoint_mode is envoy, endpoint request_method and path will be ignored")
		}
	case EndpointModeForwardAuth:
		requestMethodConfig := endpointConfig.Get("request_method")
		if !requestMethodConfig.Exists() {
			httpService.RequestMethod = http.MethodGet
		} else {
			httpService.RequestMethod = strings.ToUpper(requestMethodConfig.String())
		}

		pathConfig := endpointConfig.Get("path")
		if !pathConfig.Exists() {
			return errors.New("when endpoint_mode is forward_auth, endpoint path must not be empty")
		}
		httpService.Path = pathConfig.String()

		if endpointConfig.Get("path_prefix").Exists() {
			log.Warn("when endpoint_mode is forward_auth, endpoint path_prefix will be ignored")
		}
	}
	return nil
}

func parseAuthorizationRequestConfig(json gjson.Result, httpService *HttpService) error {
	authorizationRequestConfig := json.Get("authorization_request")
	if authorizationRequestConfig.Exists() {
		var authorizationRequest AuthorizationRequest

		allowedHeaders := authorizationRequestConfig.Get("allowed_headers")
		if allowedHeaders.Exists() {
			result, err := expr.BuildRepeatedStringMatcherIgnoreCase(allowedHeaders.Array())
			if err != nil {
				return err
			}
			authorizationRequest.AllowedHeaders = result
		}

		authorizationRequest.HeadersToAdd = convertToStringMap(authorizationRequestConfig.Get("headers_to_add"))

		withRequestBody := authorizationRequestConfig.Get("with_request_body")
		if withRequestBody.Exists() {
			// withRequestBody is true and the request method is GET, OPTIONS or HEAD
			if withRequestBody.Bool() &&
				(httpService.RequestMethod == http.MethodGet || httpService.RequestMethod == http.MethodOptions || httpService.RequestMethod == http.MethodHead) {
				return errors.New(fmt.Sprintf("requestMethod %s does not support with_request_body set to true", httpService.RequestMethod))
			}
			authorizationRequest.WithRequestBody = withRequestBody.Bool()
		}

		maxRequestBodyBytes := uint32(authorizationRequestConfig.Get("max_request_body_bytes").Uint())
		if maxRequestBodyBytes == 0 {
			maxRequestBodyBytes = DefaultMaxRequestBodyBytes
		}
		authorizationRequest.MaxRequestBodyBytes = maxRequestBodyBytes

		allowedProperties := authorizationRequestConfig.Get("allowed_properties").Array()
		var err error
		authorizationRequest.AllowedProperties, err = parseAllowedProperties(allowedProperties)
		if err != nil {
			return err
		}

		httpService.AuthorizationRequest = authorizationRequest
	}
	return nil
}

func parseAuthorizationResponseConfig(json gjson.Result, httpService *HttpService) error {
	authorizationResponseConfig := json.Get("authorization_response")
	if authorizationResponseConfig.Exists() {
		var authorizationResponse AuthorizationResponse

		allowedUpstreamHeaders := authorizationResponseConfig.Get("allowed_upstream_headers")
		if allowedUpstreamHeaders.Exists() {
			result, err := expr.BuildRepeatedStringMatcherIgnoreCase(allowedUpstreamHeaders.Array())
			if err != nil {
				return err
			}
			authorizationResponse.AllowedUpstreamHeaders = result
		}

		allowedClientHeaders := authorizationResponseConfig.Get("allowed_client_headers")
		if allowedClientHeaders.Exists() {
			result, err := expr.BuildRepeatedStringMatcherIgnoreCase(allowedClientHeaders.Array())
			if err != nil {
				return err
			}
			authorizationResponse.AllowedClientHeaders = result
		}

		mappedUpstreamHeaders := authorizationResponseConfig.Get("mapped_upstream_headers")
		if mappedUpstreamHeaders.Exists() {
			mappings, err := parseMappedUpstreamHeaders(mappedUpstreamHeaders)
			if err != nil {
				return err
			}
			authorizationResponse.MappedUpstreamHeaders = mappings
		}

		httpService.AuthorizationResponse = authorizationResponse
	}
	return nil
}

func parseMappedUpstreamHeaders(result gjson.Result) ([]HeaderMapping, error) {
	if !result.IsArray() {
		return nil, errors.New("mapped_upstream_headers must be an array")
	}
	items := result.Array()
	mappings := make([]HeaderMapping, 0, len(items))
	for i, item := range items {
		source := item.Get("source").String()
		if !isValidSource(source) {
			return nil, fmt.Errorf("mapped_upstream_headers[%d]: invalid source %q, must be one of %s, %s, %s",
				i, source, extract.SourceStatusCode, extract.SourceHeader, extract.SourceBodyJson)
		}

		key := item.Get("key").String()
		if source != extract.SourceStatusCode && key == "" {
			return nil, fmt.Errorf("mapped_upstream_headers[%d]: source %q requires a key", i, source)
		}

		toHeader := item.Get("to_header").String()
		if toHeader == "" {
			return nil, fmt.Errorf("mapped_upstream_headers[%d]: missing required field 'to_header'", i)
		}

		mappings = append(mappings, HeaderMapping{
			Source:   source,
			Key:      key,
			ToHeader: toHeader,
		})
	}
	return mappings, nil
}

func parseSuccessCondition(result gjson.Result) ([]Condition, error) {
	if !result.Exists() {
		return nil, nil
	}
	if !result.IsArray() {
		return nil, errors.New("success_condition must be an array")
	}

	items := result.Array()
	conditions := make([]Condition, 0, len(items))
	for i, item := range items {
		source := item.Get("source").String()
		if !isValidSource(source) {
			return nil, fmt.Errorf("success_condition[%d]: invalid source %q, must be one of %s, %s, %s",
				i, source, extract.SourceStatusCode, extract.SourceHeader, extract.SourceBodyJson)
		}

		op := item.Get("op").String()
		if !isValidOp(op) {
			return nil, fmt.Errorf("success_condition[%d]: invalid op %q", i, op)
		}

		key := item.Get("key").String()
		if source != extract.SourceStatusCode && key == "" {
			return nil, fmt.Errorf("success_condition[%d]: source %q requires a key", i, source)
		}

		var values []string
		if valueResult := item.Get("value"); valueResult.IsArray() {
			values = convertToStringList(valueResult.Array())
		} else if valueResult.Exists() {
			values = []string{valueResult.String()}
		}
		if op != OpExists && op != OpNotExists && len(values) == 0 {
			return nil, fmt.Errorf("success_condition[%d]: op %q requires a value", i, op)
		}

		conditions = append(conditions, Condition{
			Source: source,
			Key:    key,
			Op:     op,
			Value:  values,
		})
	}
	return conditions, nil
}

func isValidSource(source string) bool {
	switch source {
	case extract.SourceStatusCode, extract.SourceHeader, extract.SourceBodyJson:
		return true
	default:
		return false
	}
}

func isValidOp(op string) bool {
	switch op {
	case OpEq, OpNe, OpIn, OpNotIn, OpExists, OpNotExists, OpGt, OpLt:
		return true
	default:
		return false
	}
}

func parseMatchRules(json gjson.Result, config *ExtAuthConfig) error {
	matchListConfig := json.Get("match_list")
	if !matchListConfig.Exists() {
		config.MatchRules = expr.MatchRulesDefaults()
		return nil
	}

	matchType := json.Get("match_type")
	if !matchType.Exists() {
		return errors.New("missing match_type in config")
	}
	if matchType.Str != expr.ModeWhitelist && matchType.Str != expr.ModeBlacklist {
		return errors.New("invalid match_type in config, must be 'whitelist' or 'blacklist'")
	}

	ruleList := make([]expr.Rule, 0)
	var err error

	matchListConfig.ForEach(func(key, value gjson.Result) bool {
		domain := value.Get("match_rule_domain").Str
		methodArray := value.Get("match_rule_method").Array()
		matchRuleType := value.Get("match_rule_type").Str
		matchRulePath := value.Get("match_rule_path").Str

		var pathMatcher expr.Matcher
		var buildErr error

		if matchRuleType == "" && matchRulePath == "" {
			pathMatcher = nil
		} else {
			pathMatcher, buildErr = expr.BuildStringMatcher(matchRuleType, matchRulePath, false)
			if buildErr != nil {
				err = fmt.Errorf("failed to build string matcher for rule with domain %q, method %v, path %q, type %q: %w",
					domain, methodArray, matchRulePath, matchRuleType, buildErr)
				return false // stop iterating
			}
		}

		headerConditions, parseErr := parseHeaderConditions(value.Get("match_rule_headers"), int(key.Int()))
		if parseErr != nil {
			err = parseErr
			return false // stop iterating
		}

		ruleList = append(ruleList, expr.Rule{
			Domain:  domain,
			Method:  convertToStringList(methodArray),
			Path:    pathMatcher,
			Headers: headerConditions,
		})
		return true // keep iterating
	})

	if err != nil {
		return err
	}

	config.MatchRules = expr.MatchRules{
		Mode:     matchType.Str,
		RuleList: ruleList,
	}
	return nil
}

func parseHeaderConditions(result gjson.Result, ruleIndex int) ([]expr.HeaderCondition, error) {
	if !result.Exists() {
		return nil, nil
	}

	fieldPath := fmt.Sprintf("match_list[%d].match_rule_headers", ruleIndex)
	if !result.IsArray() {
		return nil, fmt.Errorf("%s must be a non-empty array", fieldPath)
	}
	items := result.Array()
	if len(items) == 0 {
		return nil, fmt.Errorf("%s must be a non-empty array", fieldPath)
	}

	conditions := make([]expr.HeaderCondition, 0, len(items))
	seenNames := make(map[string]struct{}, len(items))
	for index, item := range items {
		itemPath := fmt.Sprintf("%s[%d]", fieldPath, index)
		nameResult := item.Get("name")
		if !nameResult.Exists() {
			return nil, fmt.Errorf("%s: missing required field 'name'", itemPath)
		}
		name := nameResult.String()
		if nameResult.Type != gjson.String || !isValidHTTPHeaderName(name) {
			return nil, fmt.Errorf("%s.name must be a valid non-pseudo HTTP header name", itemPath)
		}
		name = strings.ToLower(name)
		if _, duplicate := seenNames[name]; duplicate {
			return nil, fmt.Errorf("%s contains duplicate header name %q", fieldPath, name)
		}

		existsResult := item.Get("exists")
		if !existsResult.Exists() {
			return nil, fmt.Errorf("%s: missing required field 'exists'", itemPath)
		}
		if existsResult.Type != gjson.True && existsResult.Type != gjson.False {
			return nil, fmt.Errorf("%s.exists must be a boolean", itemPath)
		}

		seenNames[name] = struct{}{}
		conditions = append(conditions, expr.HeaderCondition{Name: name, Exists: existsResult.Bool()})
	}
	return conditions, nil
}

func isValidHTTPHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for _, char := range []byte(name) {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' {
			continue
		}
		switch char {
		case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
			continue
		}
		return false
	}
	return true
}

func convertToStringMap(result gjson.Result) map[string]string {
	m := make(map[string]string)
	result.ForEach(func(key, value gjson.Result) bool {
		m[key.String()] = value.String()
		return true // keep iterating
	})
	return m
}

func convertToStringList(results []gjson.Result) []string {
	interfaces := make([]string, len(results))
	for i, result := range results {
		interfaces[i] = result.String()
	}
	return interfaces
}

func parseAllowedProperties(results []gjson.Result) ([]AllowedProperty, error) {
	props := make([]AllowedProperty, 0, len(results))
	for i, result := range results {
		pathVal := result.Get("path")
		headerVal := result.Get("header")
		if !pathVal.Exists() {
			return nil, fmt.Errorf("allowed_properties[%d]: missing required field 'path'", i)
		}
		if !headerVal.Exists() {
			return nil, fmt.Errorf("allowed_properties[%d]: missing required field 'header'", i)
		}
		// path can be array format: [route_name] or [metadata, test]
		// or single value format: route_name
		var path []string
		if pathVal.IsArray() {
			pathVal.ForEach(func(key, value gjson.Result) bool {
				path = append(path, value.String())
				return true
			})
		} else {
			path = []string{pathVal.String()}
		}
		props = append(props, AllowedProperty{
			Path:   path,
			Header: headerVal.String(),
		})
	}
	return props, nil
}
