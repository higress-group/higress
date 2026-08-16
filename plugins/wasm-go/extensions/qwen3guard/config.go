package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

const (
	pluginName = "qwen3guard"

	defaultAPIKey                           = "EMPTY"
	defaultModel                            = "Qwen/Qwen3Guard-Gen-4B"
	defaultTimeoutMS                        = 2000
	defaultRequestPath                      = "/v1/chat/completions"
	defaultRequestContentJSONPath           = "messages.@reverse.0.content"
	defaultResponseContentJSONPath          = "choices.0.message.content"
	defaultStreamingResponseContentJSONPath = "choices.0.delta.content"
	defaultStreamBufferChars                = 1000
	defaultMaxBodyBytes                     = 10 * 1024 * 1024
	defaultRiskLevelBar                     = riskLevelUnsafe
	defaultDenyCode                         = 200
	defaultDenyMessage                      = "很抱歉，我无法回答您的问题"
)

const (
	riskLevelSafe          = "Safe"
	riskLevelControversial = "Controversial"
	riskLevelUnsafe        = "Unsafe"
)

type pluginConfig struct {
	client                           wrapper.HttpClient
	serviceSource                    string
	serviceName                      string
	servicePort                      int64
	namespace                        string
	domain                           string
	requestPath                      string
	apiKey                           string
	model                            string
	timeoutMS                        uint32
	checkRequest                     bool
	checkResponse                    bool
	requestContentJSONPath           string
	responseContentJSONPath          string
	streamingResponseContentJSONPath string
	streamBufferChars                int
	riskLevelBar                     string
	denyCode                         int
	denyMessage                      string
	maxBodyBytes                     uint32
}

func parseConfig(json gjson.Result, c *pluginConfig, log log.Log) error {
	*c = *defaultConfig()

	c.serviceSource = strings.TrimSpace(json.Get("service_source").String())
	if c.serviceSource == "" {
		c.serviceSource = strings.TrimSpace(json.Get("serviceSource").String())
	}
	c.serviceName = strings.TrimSpace(json.Get("service_name").String())
	if c.serviceName == "" {
		c.serviceName = strings.TrimSpace(json.Get("serviceName").String())
	}
	c.servicePort = json.Get("service_port").Int()
	if c.servicePort == 0 {
		c.servicePort = json.Get("servicePort").Int()
	}
	c.namespace = strings.TrimSpace(json.Get("namespace").String())
	c.domain = strings.TrimSpace(json.Get("domain").String())
	setString(json, "request_path", "requestPath", &c.requestPath)
	setString(json, "api_key", "apiKey", &c.apiKey)
	setString(json, "model", "model", &c.model)
	setBool(json, "check_request", "checkRequest", &c.checkRequest)
	setBool(json, "check_response", "checkResponse", &c.checkResponse)
	setString(json, "request_content_json_path", "requestContentJsonPath", &c.requestContentJSONPath)
	setString(json, "response_content_json_path", "responseContentJsonPath", &c.responseContentJSONPath)
	setString(json, "streaming_response_content_json_path", "streamingResponseContentJsonPath", &c.streamingResponseContentJSONPath)
	if err := setPositiveInt(json, "stream_buffer_chars", "streamBufferChars", &c.streamBufferChars); err != nil {
		return err
	}
	if err := setPositiveInt(json, "deny_code", "denyCode", &c.denyCode); err != nil {
		return err
	}
	setString(json, "deny_message", "denyMessage", &c.denyMessage)
	timeout, timeoutSet := getPositiveInt(json, "timeout_ms", "timeoutMs")
	if timeoutSet {
		if timeout <= 0 {
			return errors.New("timeout_ms must be greater than 0")
		}
		c.timeoutMS = uint32(timeout)
	}
	maxBodyBytes, maxBodyBytesSet := getPositiveInt(json, "max_body_bytes", "maxBodyBytes")
	if maxBodyBytesSet {
		if maxBodyBytes <= 0 {
			return errors.New("max_body_bytes must be greater than 0")
		}
		c.maxBodyBytes = uint32(maxBodyBytes)
	}
	if riskLevelBar := strings.TrimSpace(json.Get("risk_level_bar").String()); riskLevelBar != "" {
		normalized, ok := normalizeRiskLevelBar(riskLevelBar)
		if !ok {
			return fmt.Errorf("risk_level_bar must be %s or %s", riskLevelControversial, riskLevelUnsafe)
		}
		c.riskLevelBar = normalized
	} else if riskLevelBar = strings.TrimSpace(json.Get("riskLevelBar").String()); riskLevelBar != "" {
		normalized, ok := normalizeRiskLevelBar(riskLevelBar)
		if !ok {
			return fmt.Errorf("risk_level_bar must be %s or %s", riskLevelControversial, riskLevelUnsafe)
		}
		c.riskLevelBar = normalized
	}
	if c.serviceSource == "k8s" && c.namespace == "" {
		c.namespace = "default"
	}

	if err := c.validate(); err != nil {
		return err
	}

	client, err := newGuardClient(*c)
	if err != nil {
		return err
	}
	c.client = client
	return nil
}

func defaultConfig() *pluginConfig {
	return &pluginConfig{
		requestPath:                      defaultRequestPath,
		apiKey:                           defaultAPIKey,
		model:                            defaultModel,
		timeoutMS:                        defaultTimeoutMS,
		checkRequest:                     true,
		checkResponse:                    true,
		requestContentJSONPath:           defaultRequestContentJSONPath,
		responseContentJSONPath:          defaultResponseContentJSONPath,
		streamingResponseContentJSONPath: defaultStreamingResponseContentJSONPath,
		streamBufferChars:                defaultStreamBufferChars,
		riskLevelBar:                     defaultRiskLevelBar,
		denyCode:                         defaultDenyCode,
		denyMessage:                      defaultDenyMessage,
		maxBodyBytes:                     defaultMaxBodyBytes,
	}
}

func (c *pluginConfig) validate() error {
	if c.serviceSource == "" {
		return errors.New("service_source is required")
	}
	if c.serviceName == "" {
		return errors.New("service_name is required")
	}
	if c.servicePort <= 0 {
		return errors.New("service_port must be greater than 0")
	}
	if c.serviceSource == "dns" && c.domain == "" {
		return errors.New("domain is required when service_source is dns")
	}
	if c.requestPath == "" {
		return errors.New("request_path must not be empty")
	}
	if c.model == "" {
		return errors.New("model must not be empty")
	}
	if c.timeoutMS == 0 {
		return errors.New("timeout_ms must be greater than 0")
	}
	if c.streamBufferChars <= 0 {
		return errors.New("stream_buffer_chars must be greater than 0")
	}
	if c.denyCode <= 0 {
		return errors.New("deny_code must be greater than 0")
	}
	if c.maxBodyBytes == 0 {
		return errors.New("max_body_bytes must be greater than 0")
	}
	return nil
}

func newGuardClient(c pluginConfig) (wrapper.HttpClient, error) {
	switch c.serviceSource {
	case "k8s":
		return wrapper.NewClusterClient(wrapper.K8sCluster{
			ServiceName: c.serviceName,
			Namespace:   c.namespace,
			Port:        c.servicePort,
		}), nil
	case "nacos":
		return wrapper.NewClusterClient(wrapper.NacosCluster{
			ServiceName: c.serviceName,
			NamespaceID: c.namespace,
			Port:        c.servicePort,
		}), nil
	case "ip":
		return wrapper.NewClusterClient(wrapper.StaticIpCluster{
			ServiceName: c.serviceName,
			Port:        c.servicePort,
		}), nil
	case "dns":
		return wrapper.NewClusterClient(wrapper.DnsCluster{
			ServiceName: c.serviceName,
			Domain:      c.domain,
			Port:        c.servicePort,
		}), nil
	default:
		return nil, fmt.Errorf("unknown service_source: %s", c.serviceSource)
	}
}

func setString(json gjson.Result, snakeKey string, camelKey string, target *string) {
	if value := strings.TrimSpace(json.Get(snakeKey).String()); value != "" {
		*target = value
		return
	}
	if value := strings.TrimSpace(json.Get(camelKey).String()); value != "" {
		*target = value
	}
}

func setBool(json gjson.Result, snakeKey string, camelKey string, target *bool) {
	if value := json.Get(snakeKey); value.Exists() {
		*target = value.Bool()
		return
	}
	if value := json.Get(camelKey); value.Exists() {
		*target = value.Bool()
	}
}

func setPositiveInt(json gjson.Result, snakeKey string, camelKey string, target *int) error {
	value, exists := getPositiveInt(json, snakeKey, camelKey)
	if !exists {
		return nil
	}
	if value <= 0 {
		return fmt.Errorf("%s must be greater than 0", snakeKey)
	}
	*target = value
	return nil
}

func getPositiveInt(json gjson.Result, snakeKey string, camelKey string) (int, bool) {
	if value := json.Get(snakeKey); value.Exists() {
		return int(value.Int()), true
	}
	if value := json.Get(camelKey); value.Exists() {
		return int(value.Int()), true
	}
	return 0, false
}

func normalizeRiskLevelBar(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case strings.ToLower(riskLevelControversial):
		return riskLevelControversial, true
	case strings.ToLower(riskLevelUnsafe):
		return riskLevelUnsafe, true
	default:
		return "", false
	}
}

func shouldBlockRisk(safety string, riskLevelBar string) bool {
	return riskLevelValue(safety) >= riskLevelValue(riskLevelBar)
}

func riskLevelValue(safety string) int {
	switch strings.ToLower(strings.TrimSpace(safety)) {
	case strings.ToLower(riskLevelSafe):
		return 0
	case strings.ToLower(riskLevelControversial):
		return 1
	case strings.ToLower(riskLevelUnsafe):
		return 2
	default:
		return -1
	}
}
