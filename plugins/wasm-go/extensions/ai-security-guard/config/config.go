package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/alibaba/higress/plugins/wasm-go/extensions/ai-security-guard/utils"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

type OpenAIDenyResponseFormat string

const (
	MaxRisk    = "max"
	HighRisk   = "high"
	MediumRisk = "medium"
	LowRisk    = "low"
	NoRisk     = "none"

	S4Sensitive = "s4"
	S3Sensitive = "s3"
	S2Sensitive = "s2"
	S1Sensitive = "s1"
	NoSensitive = "s0"

	ContentModerationType      = "contentModeration"
	PromptAttackType           = "promptAttack"
	SensitiveDataType          = "sensitiveData"
	MaliciousUrlDataType       = "maliciousUrl"
	ModelHallucinationDataType = "modelHallucination"
	CustomLabelType            = "customLabel"
	MaliciousFileType          = "maliciousFile"
	WaterMarkType              = "waterMark"

	// Default configurations
	// Template parameter order:
	//   OpenAIResponseFormatLegacy:          id, created (unix sec), content
	//   OpenAIResponseFormatStructured:      id, created (unix sec), content, x_higress_guardrail JSON
	//   OpenAIStreamResponseChunk:           id, created, content
	//   OpenAIStreamResponseEndLegacy:       id, created
	//   OpenAIStreamResponseEndStructured:   id, created, x_higress_guardrail JSON
	//   OpenAIStreamResponseFormatLegacy:    id, created, content, id, created
	//   OpenAIStreamResponseFormatStructured: id, created, content, id, created, x_higress_guardrail JSON
	// `created` is required by openai-python (ChatCompletion.created is non-Optional).
	// `finish_reason: "stop"` preserves wire-level compatibility with downstream
	// consumers (LangChain / LiteLLM / SDKs / BI dashboards) that treat `stop` as
	// "valid completion"; the moderation-event signal lives in the nested
	// `choices[0].x_higress_guardrail` block (denyCode / blockedDetails) instead.
	OpenAIResponseFormatLegacy           = `{"id":"%s","object":"chat.completion","created":%d,"model":"from-security-guard","choices":[{"index":0,"message":{"role":"assistant","content":"%s"},"logprobs":null,"finish_reason":"stop"}],"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}}`
	OpenAIResponseFormatStructured       = `{"id":"%s","object":"chat.completion","created":%d,"model":"from-security-guard","choices":[{"index":0,"message":{"role":"assistant","content":"%s"},"logprobs":null,"finish_reason":"stop","x_higress_guardrail":%s}],"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}}`
	OpenAIStreamResponseChunk            = `data:{"id":"%s","object":"chat.completion.chunk","created":%d,"model":"from-security-guard","choices":[{"index":0,"delta":{"role":"assistant","content":"%s"},"logprobs":null,"finish_reason":null}]}`
	OpenAIStreamResponseEndLegacy        = `data:{"id":"%s","object":"chat.completion.chunk","created":%d,"model":"from-security-guard","choices":[{"index":0,"delta":{},"logprobs":null,"finish_reason":"stop"}],"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}}`
	OpenAIStreamResponseEndStructured    = `data:{"id":"%s","object":"chat.completion.chunk","created":%d,"model":"from-security-guard","choices":[{"index":0,"delta":{},"logprobs":null,"finish_reason":"stop","x_higress_guardrail":%s}],"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}}`
	OpenAIStreamResponseFormatLegacy     = OpenAIStreamResponseChunk + "\n\n" + OpenAIStreamResponseEndLegacy + "\n\n" + `data: [DONE]`
	OpenAIStreamResponseFormatStructured = OpenAIStreamResponseChunk + "\n\n" + OpenAIStreamResponseEndStructured + "\n\n" + `data: [DONE]`

	OpenAIDenyResponseFormatLegacy     OpenAIDenyResponseFormat = "legacy"
	OpenAIDenyResponseFormatStructured OpenAIDenyResponseFormat = "structured"

	OpenAIDenyResponseFormatConsumerScopeError = "openAIDenyResponseFormat must be configured at plugin global scope, not under consumerRiskLevel"

	DefaultDenyCode    = 200
	DefaultDenyMessage = "很抱歉，我无法回答您的问题"
	DefaultTimeout     = 2000

	AliyunUserAgent = "CIPFrom/AIGateway"
	LengthLimit     = 1800

	DefaultRequestCheckService       = "llm_query_moderation"
	DefaultResponseCheckService      = "llm_response_moderation"
	DefaultRequestJsonPath           = "messages.@reverse.0.content"
	DefaultResponseJsonPath          = "choices.0.message.content"
	DefaultStreamingResponseJsonPath = "choices.0.delta.content"

	// Actions
	MultiModalGuard          = "MultiModalGuard"
	MultiModalGuardForBase64 = "MultiModalGuardForBase64"
	TextModerationPlus       = "TextModerationPlus"

	// Services
	DefaultMultiModalGuardTextInputCheckService  = "query_security_check"
	DefaultMultiModalGuardTextOutputCheckService = "response_security_check"
	DefaultMultiModalGuardImageInputCheckService = "img_query_security_check"

	DefaultTextModerationPlusTextInputCheckService  = "llm_query_moderation"
	DefaultTextModerationPlusTextOutputCheckService = "llm_response_moderation"
)

var (
	// Keep these defaults aligned with previous hardcoded fallback extraction behavior.
	defaultResponseFallbackJsonPaths = []string{
		"choices.0.message.content",
		`content.#(type=="text")#.text`,
	}
	defaultStreamingResponseFallbackJsonPaths = []string{
		"choices.0.delta.content",
		"delta.text",
	}
)

func DefaultResponseFallbackJsonPaths() []string {
	return append([]string(nil), defaultResponseFallbackJsonPaths...)
}

func DefaultStreamingResponseFallbackJsonPaths() []string {
	return append([]string(nil), defaultStreamingResponseFallbackJsonPaths...)
}

// api types

const (
	ApiTextGeneration  = "text_generation"
	ApiImageGeneration = "image_generation"
	ApiMCP             = "mcp"
	ApiEmbedding       = "embedding"
)

// provider types
const (
	ProviderOpenAI  = "openai"
	ProviderQwen    = "qwen"
	ProviderComfyUI = "comfyui"
)

type Response struct {
	Code      int    `json:"Code"`
	Message   string `json:"Message"`
	RequestId string `json:"RequestId"`
	Data      Data   `json:"Data"`
}

type Data struct {
	RiskLevel   string   `json:"RiskLevel,omitempty"`
	AttackLevel string   `json:"AttackLevel,omitempty"`
	Suggestion  string   `json:"Suggestion,omitempty"`
	Result      []Result `json:"Result,omitempty"`
	Advice      []Advice `json:"Advice,omitempty"`
	Detail      []Detail `json:"Detail,omitempty"`
}

type Ext struct {
	Desensitization string   `json:"Desensitization,omitempty"`
	SensitiveData   []string `json:"SensitiveData,omitempty"`
}

type Result struct {
	RiskWords   string  `json:"RiskWords,omitempty"`
	Description string  `json:"Description,omitempty"`
	Confidence  float64 `json:"Confidence,omitempty"`
	Label       string  `json:"Label,omitempty"`
	Ext         Ext     `json:"Ext,omitempty"`
}

type Advice struct {
	Answer     string `json:"Answer,omitempty"`
	HitLabel   string `json:"HitLabel,omitempty"`
	HitLibName string `json:"HitLibName,omitempty"`
}

type Detail struct {
	Suggestion string   `json:"Suggestion,omitempty"`
	Type       string   `json:"Type,omitempty"`
	Level      string   `json:"Level,omitempty"`
	Result     []Result `json:"Result,omitempty"`
}

type Matcher struct {
	Exact  string
	Prefix string
	Re     *regexp.Regexp
}

func (m *Matcher) match(consumer string) bool {
	if m.Exact != "" {
		return consumer == m.Exact
	} else if m.Prefix != "" {
		return strings.HasPrefix(consumer, m.Prefix)
	} else if m.Re != nil {
		return m.Re.MatchString(consumer)
	} else {
		return false
	}
}

type AISecurityConfig struct {
	Client                                 wrapper.HttpClient
	Host                                   string
	AK                                     string
	SK                                     string
	Token                                  string
	Action                                 string
	CheckRequest                           bool
	CheckRequestImage                      bool
	RequestCheckService                    string
	RequestImageCheckService               string
	RequestContentJsonPath                 string
	CheckResponse                          bool
	ResponseCheckService                   string
	ResponseImageCheckService              string
	ResponseContentJsonPath                string
	ResponseStreamContentJsonPath          string
	ResponseContentFallbackJsonPaths       []string
	ResponseStreamContentFallbackJsonPaths []string
	ResponseErrorContentJsonPath           string
	DenyCode                               int64
	DenyMessage                            string
	ProtocolOriginal                       bool
	OpenAIDenyResponseFormat               OpenAIDenyResponseFormat
	RiskLevelBar                           string
	ContentModerationLevelBar              string
	PromptAttackLevelBar                   string
	SensitiveDataLevelBar                  string
	MaliciousUrlLevelBar                   string
	ModelHallucinationLevelBar             string
	CustomLabelLevelBar                    string
	Timeout                                uint32
	BufferLimit                            int
	Metrics                                map[string]proxywasm.MetricCounter
	ConsumerRequestCheckService            []map[string]interface{}
	ConsumerResponseCheckService           []map[string]interface{}
	ConsumerRiskLevel                      []map[string]interface{}
	// text_generation, image_generation, embedding, etc.
	ApiType string
	// openai, qwen, comfyui, etc.
	ProviderType string
	// "block" or "mask", default "block"
	RiskAction string
	// Dimension-level action fields (optional, empty string means not configured)
	ContentModerationAction  string
	PromptAttackAction       string
	SensitiveDataAction      string
	MaliciousUrlAction       string
	ModelHallucinationAction string
	CustomLabelAction        string
}

func (config *AISecurityConfig) Parse(json gjson.Result) error {
	serviceName := json.Get("serviceName").String()
	servicePort := json.Get("servicePort").Int()
	serviceHost := json.Get("serviceHost").String()
	config.Host = serviceHost
	if serviceName == "" || servicePort == 0 || serviceHost == "" {
		return errors.New("invalid service config")
	}
	config.AK = json.Get("accessKey").String()
	config.SK = json.Get("secretKey").String()
	if config.AK == "" || config.SK == "" {
		return errors.New("invalid AK/SK config")
	}
	config.Token = json.Get("securityToken").String()
	// set action
	if obj := json.Get("action"); obj.Exists() {
		config.Action = json.Get("action").String()
	} else {
		config.Action = TextModerationPlus
	}
	// set default values
	config.SetDefaultValues()
	// set riskAction
	if obj := json.Get("riskAction"); obj.Exists() {
		config.RiskAction = obj.String()
		if config.RiskAction != "block" && config.RiskAction != "mask" {
			return errors.New("invalid riskAction, value must be one of [block, mask]")
		}
	}
	// parse global dimension action fields
	isMultiModalGuard := config.Action == MultiModalGuard || config.Action == MultiModalGuardForBase64
	dimensionActionFields := []struct {
		fieldName string
		target    *string
	}{
		{"contentModerationAction", &config.ContentModerationAction},
		{"promptAttackAction", &config.PromptAttackAction},
		{"sensitiveDataAction", &config.SensitiveDataAction},
		{"maliciousUrlAction", &config.MaliciousUrlAction},
		{"modelHallucinationAction", &config.ModelHallucinationAction},
		{"customLabelAction", &config.CustomLabelAction},
	}
	hasDimensionAction := false
	for _, field := range dimensionActionFields {
		if isMultiModalGuard {
			val, err := parseDimensionAction(json, field.fieldName)
			if err != nil {
				return err
			}
			*field.target = val
			if val != "" {
				hasDimensionAction = true
			}
		} else {
			// Non-MultiModalGuard: read value without validation, field will be ignored at runtime
			if obj := json.Get(field.fieldName); obj.Exists() {
				*field.target = obj.String()
				hasDimensionAction = true
			}
		}
	}
	if hasDimensionAction && !isMultiModalGuard {
		proxywasm.LogWarnf("dimension action fields are configured but will be ignored because action is %s (not MultiModalGuard/MultiModalGuardForBase64)", config.Action)
	}
	// set values
	if obj := json.Get("riskLevelBar"); obj.Exists() {
		config.RiskLevelBar = obj.String()
	}
	if obj := json.Get("requestCheckService"); obj.Exists() {
		config.RequestCheckService = obj.String()
	}
	if obj := json.Get("requestImageCheckService"); obj.Exists() {
		config.RequestImageCheckService = obj.String()
	}
	if obj := json.Get("responseCheckService"); obj.Exists() {
		config.ResponseCheckService = obj.String()
	}
	if obj := json.Get("responseImageCheckService"); obj.Exists() {
		config.ResponseImageCheckService = obj.String()
	}
	config.CheckRequest = json.Get("checkRequest").Bool()
	config.CheckRequestImage = json.Get("checkRequestImage").Bool()
	config.CheckResponse = json.Get("checkResponse").Bool()
	config.ProtocolOriginal = json.Get("protocol").String() == "original"
	if obj := json.Get("openAIDenyResponseFormat"); obj.Exists() {
		switch OpenAIDenyResponseFormat(obj.String()) {
		case OpenAIDenyResponseFormatLegacy:
			config.OpenAIDenyResponseFormat = OpenAIDenyResponseFormatLegacy
		case OpenAIDenyResponseFormatStructured:
			config.OpenAIDenyResponseFormat = OpenAIDenyResponseFormatStructured
		default:
			return errors.New("invalid openAIDenyResponseFormat, value must be one of [legacy, structured]")
		}
	}
	config.DenyMessage = json.Get("denyMessage").String()
	if obj := json.Get("denyCode"); obj.Exists() {
		config.DenyCode = obj.Int()
	}
	if obj := json.Get("requestContentJsonPath"); obj.Exists() {
		config.RequestContentJsonPath = obj.String()
	}
	if obj := json.Get("responseContentJsonPath"); obj.Exists() {
		config.ResponseContentJsonPath = obj.String()
	}
	if obj := json.Get("responseStreamContentJsonPath"); obj.Exists() {
		config.ResponseStreamContentJsonPath = obj.String()
	}
	if paths, exists, err := parseOptionalStringArrayConfig(json, "responseContentFallbackJsonPaths"); err != nil {
		return err
	} else if exists {
		config.ResponseContentFallbackJsonPaths = paths
	}
	if paths, exists, err := parseOptionalStringArrayConfig(json, "responseStreamContentFallbackJsonPaths"); err != nil {
		return err
	} else if exists {
		config.ResponseStreamContentFallbackJsonPaths = paths
	}
	if obj := json.Get("responseErrorContentJsonPath"); obj.Exists() {
		config.ResponseErrorContentJsonPath = obj.String()
	}
	if obj := json.Get("contentModerationLevelBar"); obj.Exists() {
		config.ContentModerationLevelBar = obj.String()
		if LevelToInt(config.ContentModerationLevelBar) <= 0 {
			return errors.New("invalid contentModerationLevelBar, value must be one of [max, high, medium, low]")
		}
	}
	if obj := json.Get("promptAttackLevelBar"); obj.Exists() {
		config.PromptAttackLevelBar = obj.String()
		if LevelToInt(config.PromptAttackLevelBar) <= 0 {
			return errors.New("invalid promptAttackLevelBar, value must be one of [max, high, medium, low]")
		}
	}
	if obj := json.Get("sensitiveDataLevelBar"); obj.Exists() {
		config.SensitiveDataLevelBar = obj.String()
		if LevelToInt(config.SensitiveDataLevelBar) <= 0 {
			return errors.New("invalid sensitiveDataLevelBar, value must be one of [S4, S3, S2, S1]")
		}
	}
	if obj := json.Get("modelHallucinationLevelBar"); obj.Exists() {
		config.ModelHallucinationLevelBar = obj.String()
		if LevelToInt(config.ModelHallucinationLevelBar) <= 0 {
			return errors.New("invalid modelHallucinationLevelBar, value must be one of [max, high, medium, low]")
		}
	}
	if obj := json.Get("maliciousUrlLevelBar"); obj.Exists() {
		config.MaliciousUrlLevelBar = obj.String()
		if LevelToInt(config.MaliciousUrlLevelBar) <= 0 {
			return errors.New("invalid maliciousUrlLevelBar, value must be one of [max, high, medium, low]")
		}
	}
	if obj := json.Get("customLabelLevelBar"); obj.Exists() {
		config.CustomLabelLevelBar = obj.String()
		if LevelToInt(config.CustomLabelLevelBar) <= 0 {
			return errors.New("invalid customLabelLevelBar, value must be one of [max, high, medium, low]")
		}
	}
	if obj := json.Get("timeout"); obj.Exists() {
		config.Timeout = uint32(obj.Int())
	}
	if obj := json.Get("bufferLimit"); obj.Exists() {
		config.BufferLimit = int(obj.Int())
	}
	if obj := json.Get("consumerRequestCheckService"); obj.Exists() {
		for _, item := range json.Get("consumerRequestCheckService").Array() {
			m := make(map[string]interface{})
			for k, v := range item.Map() {
				m[k] = v.Value()
			}
			consumerName, ok1 := m["name"]
			matchType, ok2 := m["matchType"]
			if !ok1 || !ok2 {
				continue
			}
			switch fmt.Sprint(matchType) {
			case "exact":
				m["matcher"] = Matcher{Exact: fmt.Sprint(consumerName)}
			case "prefix":
				m["matcher"] = Matcher{Prefix: fmt.Sprint(consumerName)}
			case "regexp":
				m["matcher"] = Matcher{Re: regexp.MustCompile(fmt.Sprint(consumerName))}
			}
			config.ConsumerRequestCheckService = append(config.ConsumerRequestCheckService, m)
		}
	}
	if obj := json.Get("consumerResponseCheckService"); obj.Exists() {
		for _, item := range json.Get("consumerResponseCheckService").Array() {
			m := make(map[string]interface{})
			for k, v := range item.Map() {
				m[k] = v.Value()
			}
			consumerName, ok1 := m["name"]
			matchType, ok2 := m["matchType"]
			if !ok1 || !ok2 {
				continue
			}
			switch fmt.Sprint(matchType) {
			case "exact":
				m["matcher"] = Matcher{Exact: fmt.Sprint(consumerName)}
			case "prefix":
				m["matcher"] = Matcher{Prefix: fmt.Sprint(consumerName)}
			case "regexp":
				m["matcher"] = Matcher{Re: regexp.MustCompile(fmt.Sprint(consumerName))}
			}
			config.ConsumerResponseCheckService = append(config.ConsumerResponseCheckService, m)
		}
	}
	if obj := json.Get("consumerRiskLevel"); obj.Exists() {
		for _, item := range json.Get("consumerRiskLevel").Array() {
			m := make(map[string]interface{})
			for k, v := range item.Map() {
				m[k] = v.Value()
			}
			if _, ok := m["openAIDenyResponseFormat"]; ok {
				return errors.New(OpenAIDenyResponseFormatConsumerScopeError)
			}
			consumerName, ok1 := m["name"]
			matchType, ok2 := m["matchType"]
			if !ok1 || !ok2 {
				continue
			}
			switch fmt.Sprint(matchType) {
			case "exact":
				m["matcher"] = Matcher{Exact: fmt.Sprint(consumerName)}
			case "prefix":
				m["matcher"] = Matcher{Prefix: fmt.Sprint(consumerName)}
			case "regexp":
				m["matcher"] = Matcher{Re: regexp.MustCompile(fmt.Sprint(consumerName))}
			}
			if ra, ok := m["riskAction"]; ok {
				raStr := fmt.Sprint(ra)
				if raStr != "block" && raStr != "mask" {
					return errors.New("invalid riskAction in consumerRiskLevel, value must be one of [block, mask]")
				}
			}
			// Validate dimension action fields in consumer risk level
			if isMultiModalGuard {
				consumerDimensionActionFields := []string{
					"contentModerationAction",
					"promptAttackAction",
					"sensitiveDataAction",
					"maliciousUrlAction",
					"modelHallucinationAction",
					"customLabelAction",
				}
				for _, fieldName := range consumerDimensionActionFields {
					if v, ok := m[fieldName]; ok {
						vStr := fmt.Sprint(v)
						if vStr != "block" && vStr != "mask" {
							return fmt.Errorf("invalid %s in consumerRiskLevel, value must be one of [block, mask]", fieldName)
						}
					}
				}
			}
			config.ConsumerRiskLevel = append(config.ConsumerRiskLevel, m)
		}
	}
	if obj := json.Get("apiType"); obj.Exists() {
		config.ApiType = obj.String()
	}
	if obj := json.Get("providerType"); obj.Exists() {
		config.ProviderType = obj.String()
	}
	config.Client = wrapper.NewClusterClient(wrapper.FQDNCluster{
		FQDN: serviceName,
		Port: servicePort,
		Host: serviceHost,
	})
	config.Metrics = make(map[string]proxywasm.MetricCounter)
	return nil
}

// parseDimensionAction parses a dimension action field from JSON config.
// Returns the value if valid (block/mask), empty string if not present, or error if invalid.
func parseDimensionAction(json gjson.Result, fieldName string) (string, error) {
	if obj := json.Get(fieldName); obj.Exists() {
		val := obj.String()
		if val != "block" && val != "mask" {
			return "", fmt.Errorf("invalid %s, value must be one of [block, mask]", fieldName)
		}
		return val, nil
	}
	return "", nil
}

func parseOptionalStringArrayConfig(json gjson.Result, fieldName string) ([]string, bool, error) {
	obj := json.Get(fieldName)
	if !obj.Exists() {
		return nil, false, nil
	}
	if !obj.IsArray() {
		return nil, true, fmt.Errorf("invalid %s, value must be an array of non-empty strings", fieldName)
	}
	items := obj.Array()
	paths := make([]string, 0, len(items))
	for _, item := range items {
		if item.Type != gjson.String {
			return nil, true, fmt.Errorf("invalid %s, value must be an array of non-empty strings", fieldName)
		}
		path := strings.TrimSpace(item.String())
		if path == "" {
			return nil, true, fmt.Errorf("invalid %s, value must be an array of non-empty strings", fieldName)
		}
		paths = append(paths, path)
	}
	return paths, true, nil
}

func (config *AISecurityConfig) SetDefaultValues() {
	switch config.Action {
	case TextModerationPlus:
		config.RequestCheckService = DefaultTextModerationPlusTextInputCheckService
		config.ResponseCheckService = DefaultTextModerationPlusTextOutputCheckService
	case MultiModalGuard:
		config.RequestCheckService = DefaultMultiModalGuardTextInputCheckService
		config.RequestImageCheckService = DefaultMultiModalGuardImageInputCheckService
		config.ResponseCheckService = DefaultMultiModalGuardTextOutputCheckService
	}
	config.RiskLevelBar = HighRisk
	config.DenyCode = DefaultDenyCode
	config.RequestContentJsonPath = DefaultRequestJsonPath
	config.ResponseContentJsonPath = DefaultResponseJsonPath
	config.ResponseStreamContentJsonPath = DefaultStreamingResponseJsonPath
	config.ResponseContentFallbackJsonPaths = DefaultResponseFallbackJsonPaths()
	config.ResponseStreamContentFallbackJsonPaths = DefaultStreamingResponseFallbackJsonPaths()
	config.ContentModerationLevelBar = MaxRisk
	config.PromptAttackLevelBar = MaxRisk
	config.SensitiveDataLevelBar = S4Sensitive
	config.ModelHallucinationLevelBar = MaxRisk
	config.MaliciousUrlLevelBar = MaxRisk
	config.CustomLabelLevelBar = MaxRisk
	config.Timeout = DefaultTimeout
	config.BufferLimit = 1000
	config.ApiType = ApiTextGeneration
	config.ProviderType = ProviderOpenAI
	config.RiskAction = "block"
	config.OpenAIDenyResponseFormat = OpenAIDenyResponseFormatLegacy
}

func (config *AISecurityConfig) IncrementCounter(metricName string, inc uint64) {
	counter, ok := config.Metrics[metricName]
	if !ok {
		counter = proxywasm.DefineCounterMetric(metricName)
		config.Metrics[metricName] = counter
	}
	counter.Increment(inc)
}

func (config *AISecurityConfig) GetRequestCheckService(consumer string) string {
	result := config.RequestCheckService
	for _, obj := range config.ConsumerRequestCheckService {
		if matcher, ok := obj["matcher"].(Matcher); ok {
			if matcher.match(consumer) {
				if requestCheckService, ok := obj["requestCheckService"]; ok {
					result, _ = requestCheckService.(string)
				}
				break
			}
		}
	}
	return result
}

func (config *AISecurityConfig) GetRequestImageCheckService(consumer string) string {
	result := config.RequestImageCheckService
	for _, obj := range config.ConsumerRequestCheckService {
		if matcher, ok := obj["matcher"].(Matcher); ok {
			if matcher.match(consumer) {
				if requestCheckService, ok := obj["requestImageCheckService"]; ok {
					result, _ = requestCheckService.(string)
				}
				break
			}
		}
	}
	return result
}

func (config *AISecurityConfig) GetResponseCheckService(consumer string) string {
	result := config.ResponseCheckService
	for _, obj := range config.ConsumerResponseCheckService {
		if matcher, ok := obj["matcher"].(Matcher); ok {
			if matcher.match(consumer) {
				if responseCheckService, ok := obj["responseCheckService"]; ok {
					result, _ = responseCheckService.(string)
				}
				break
			}
		}
	}
	return result
}

func (config *AISecurityConfig) GetResponseImageCheckService(consumer string) string {
	result := config.ResponseImageCheckService
	for _, obj := range config.ConsumerResponseCheckService {
		if matcher, ok := obj["matcher"].(Matcher); ok {
			if matcher.match(consumer) {
				if responseCheckService, ok := obj["responseImageCheckService"]; ok {
					result, _ = responseCheckService.(string)
				}
				break
			}
		}
	}
	return result
}

// getMatchedConsumerRiskRule returns the first matched consumer rule using first-match semantics.
// It iterates ConsumerRiskLevel in order and returns the first rule whose matcher matches the consumer.
// Returns nil, false if no rule matches.
func (config *AISecurityConfig) getMatchedConsumerRiskRule(consumer string) (map[string]interface{}, bool) {
	for _, obj := range config.ConsumerRiskLevel {
		if matcher, ok := obj["matcher"].(Matcher); ok {
			if matcher.match(consumer) {
				return obj, true
			}
		}
	}
	return nil, false
}

func (config *AISecurityConfig) GetRiskLevelBar(consumer string) string {
	result := config.RiskLevelBar
	if rule, ok := config.getMatchedConsumerRiskRule(consumer); ok {
		if riskLevelBar, ok := rule["riskLevelBar"]; ok {
			result, _ = riskLevelBar.(string)
		}
	}
	return result
}

func (config *AISecurityConfig) GetContentModerationLevelBar(consumer string) string {
	result := config.ContentModerationLevelBar
	if rule, ok := config.getMatchedConsumerRiskRule(consumer); ok {
		if contentModerationLevelBar, ok := rule["contentModerationLevelBar"]; ok {
			result, _ = contentModerationLevelBar.(string)
		}
	}
	return result
}

func (config *AISecurityConfig) GetPromptAttackLevelBar(consumer string) string {
	result := config.PromptAttackLevelBar
	if rule, ok := config.getMatchedConsumerRiskRule(consumer); ok {
		if promptAttackLevelBar, ok := rule["promptAttackLevelBar"]; ok {
			result, _ = promptAttackLevelBar.(string)
		}
	}
	return result
}

func (config *AISecurityConfig) GetSensitiveDataLevelBar(consumer string) string {
	result := config.SensitiveDataLevelBar
	if rule, ok := config.getMatchedConsumerRiskRule(consumer); ok {
		if sensitiveDataLevelBar, ok := rule["sensitiveDataLevelBar"]; ok {
			result, _ = sensitiveDataLevelBar.(string)
		}
	}
	return result
}

func (config *AISecurityConfig) GetMaliciousUrlLevelBar(consumer string) string {
	result := config.MaliciousUrlLevelBar
	if rule, ok := config.getMatchedConsumerRiskRule(consumer); ok {
		if maliciousUrlLevelBar, ok := rule["maliciousUrlLevelBar"]; ok {
			result, _ = maliciousUrlLevelBar.(string)
		}
	}
	return result
}

func (config *AISecurityConfig) GetModelHallucinationLevelBar(consumer string) string {
	result := config.ModelHallucinationLevelBar
	if rule, ok := config.getMatchedConsumerRiskRule(consumer); ok {
		if modelHallucinationLevelBar, ok := rule["modelHallucinationLevelBar"]; ok {
			result, _ = modelHallucinationLevelBar.(string)
		}
	}
	return result
}

func (config *AISecurityConfig) GetCustomLabelLevelBar(consumer string) string {
	result := config.CustomLabelLevelBar
	if rule, ok := config.getMatchedConsumerRiskRule(consumer); ok {
		if customLabelLevelBar, ok := rule["customLabelLevelBar"]; ok {
			result, _ = customLabelLevelBar.(string)
		}
	}
	return result
}

func (config *AISecurityConfig) GetRiskAction(consumer string) string {
	result := config.RiskAction
	if rule, ok := config.getMatchedConsumerRiskRule(consumer); ok {
		if riskAction, ok := rule["riskAction"]; ok {
			result, _ = riskAction.(string)
		}
	}
	return result
}

// dimensionActionKey maps a detailType to the corresponding key used in consumerRiskLevel map.
// For example, SensitiveDataType -> "sensitiveDataAction".
func dimensionActionKey(detailType string) string {
	switch detailType {
	case ContentModerationType:
		return "contentModerationAction"
	case PromptAttackType:
		return "promptAttackAction"
	case SensitiveDataType:
		return "sensitiveDataAction"
	case MaliciousUrlDataType:
		return "maliciousUrlAction"
	case ModelHallucinationDataType:
		return "modelHallucinationAction"
	case CustomLabelType:
		return "customLabelAction"
	default:
		return ""
	}
}

// getGlobalDimensionAction returns the global dimension action field value for the given detailType.
func (config *AISecurityConfig) getGlobalDimensionAction(detailType string) string {
	switch detailType {
	case ContentModerationType:
		return config.ContentModerationAction
	case PromptAttackType:
		return config.PromptAttackAction
	case SensitiveDataType:
		return config.SensitiveDataAction
	case MaliciousUrlDataType:
		return config.MaliciousUrlAction
	case ModelHallucinationDataType:
		return config.ModelHallucinationAction
	case CustomLabelType:
		return config.CustomLabelAction
	default:
		return ""
	}
}

// enforceMaskBoundary downgrades mask to block for non-sensitiveData dimensions,
// since only sensitiveData supports actual mask/desensitization.
func enforceMaskBoundary(action, detailType, source string) (string, string) {
	if action == "mask" && detailType != SensitiveDataType {
		proxywasm.LogWarnf("mask action not supported for dimension %s, downgrading to block", detailType)
		return "block", source
	}
	return action, source
}

// ResolveRiskActionByType resolves the final action for a given dimension type
// using 5-level priority: consumer_dimension > consumer_global > global_dimension > global_global > default(block).
// Returns (action, source) where source indicates which priority level the action came from.
func (config *AISecurityConfig) ResolveRiskActionByType(consumer string, detailType string) (string, string) {
	dimKey := dimensionActionKey(detailType)

	// 1. Check matched consumer rule
	if rule, ok := config.getMatchedConsumerRiskRule(consumer); ok {
		// 1a. consumer dimension action
		if dimKey != "" {
			if v, exists := rule[dimKey]; exists {
				if s, ok := v.(string); ok && s != "" {
					return enforceMaskBoundary(s, detailType, "consumer_dimension")
				}
			}
		}
		// 1b. consumer global riskAction
		if v, exists := rule["riskAction"]; exists {
			if s, ok := v.(string); ok && s != "" {
				return enforceMaskBoundary(s, detailType, "consumer_global")
			}
		}
	}

	// 2. Global dimension action
	globalDimAction := config.getGlobalDimensionAction(detailType)
	if globalDimAction != "" {
		return enforceMaskBoundary(globalDimAction, detailType, "global_dimension")
	}

	// 3. Global riskAction
	if config.RiskAction != "" {
		return enforceMaskBoundary(config.RiskAction, detailType, "global_global")
	}

	// 4. Default block
	return "block", "default"
}

func LevelToInt(riskLevel string) int {
	// First check against our defined constants
	switch strings.ToLower(riskLevel) {
	case MaxRisk, S4Sensitive:
		return 4
	case HighRisk, S3Sensitive:
		return 3
	case MediumRisk, S2Sensitive:
		return 2
	case LowRisk, S1Sensitive:
		return 1
	case NoRisk, NoSensitive:
		return 0
	default:
		return -1
	}
}

type RiskResult int

const (
	RiskPass  RiskResult = iota // 放行
	RiskMask                    // 需要脱敏
	RiskBlock                   // 需要拦截
)

// EvaluateRisk evaluates the risk of the given data and returns a RiskResult.
// For MultiModalGuard/MultiModalGuardForBase64, it uses the unified per-dimension
// action resolution flow (evaluateRiskMultiModal).
// For other actions (e.g. TextModerationPlus), it only checks RiskLevelBar.
func EvaluateRisk(action string, data Data, config AISecurityConfig, consumer string) RiskResult {
	if action == MultiModalGuard || action == MultiModalGuardForBase64 {
		return evaluateRiskMultiModal(data, config, consumer)
	}
	// TextModerationPlus and other non-MultiModalGuard actions: dimension actions not used
	if LevelToInt(data.RiskLevel) < LevelToInt(config.GetRiskLevelBar(consumer)) {
		return RiskPass
	}
	return RiskBlock
}

// evaluateRiskMultiModal implements the unified per-dimension risk evaluation for MultiModalGuard.
// It follows the design doc section 11.1-7 pseudocode:
// 1. Top-level compatibility gate (RiskLevel / AttackLevel)
// 2. Per-Detail dimension action resolution and threshold check
// 3. Data.Suggestion=block fallback
func evaluateRiskMultiModal(data Data, config AISecurityConfig, consumer string) RiskResult {
	// 1. Top-level compatibility gate
	if LevelToInt(data.RiskLevel) >= LevelToInt(config.GetContentModerationLevelBar(consumer)) {
		return RiskBlock
	}
	if LevelToInt(data.AttackLevel) >= LevelToInt(config.GetPromptAttackLevelBar(consumer)) {
		return RiskBlock
	}

	// 2. Detail per-dimension evaluation
	hasMask := false
	for _, detail := range data.Detail {
		dimAction, actionSource := config.ResolveRiskActionByType(consumer, detail.Type)
		exceeds := detailExceedsThreshold(detail, config, consumer)

		proxywasm.LogInfof("safecheck_risk_type=%s, safecheck_resolved_action=%s, safecheck_action_source=%s",
			detail.Type, dimAction, actionSource)

		if detailTriggersBlock(detail, dimAction, exceeds) {
			return RiskBlock
		}
		// dimAction == "mask" (only sensitiveData effective; others already downgraded by enforceMaskBoundary)
		if dimAction == "mask" && detail.Suggestion == "mask" {
			if exceeds {
				hasMask = true
			} else {
				proxywasm.LogInfof("safecheck_mask_skipped: type=%s, suggestion=%s, level=%s, threshold=%s",
					detail.Type, detail.Suggestion, detail.Level, config.GetSensitiveDataLevelBar(consumer))
			}
		}
	}

	if hasMask {
		return RiskMask
	}
	return RiskPass
}

// detailTriggersBlock returns whether this single detail should trigger blocking,
// given the resolved dimension action and threshold evaluation result.
func detailTriggersBlock(detail Detail, dimAction string, exceeds bool) bool {
	if dimAction == "block" {
		return exceeds
	}
	// dimAction == "mask": explicit mask suggestion is allowed to pass for desensitization.
	if detail.Suggestion == "mask" {
		return false
	}
	return exceeds
}

// detailExceedsThreshold checks if a single Detail's level exceeds the configured threshold
// for its Type.
func detailExceedsThreshold(detail Detail, config AISecurityConfig, consumer string) bool {
	switch detail.Type {
	case ContentModerationType:
		return LevelToInt(detail.Level) >= LevelToInt(config.GetContentModerationLevelBar(consumer))
	case PromptAttackType:
		return LevelToInt(detail.Level) >= LevelToInt(config.GetPromptAttackLevelBar(consumer))
	case SensitiveDataType:
		return LevelToInt(detail.Level) >= LevelToInt(config.GetSensitiveDataLevelBar(consumer))
	case MaliciousUrlDataType:
		return LevelToInt(detail.Level) >= LevelToInt(config.GetMaliciousUrlLevelBar(consumer))
	case ModelHallucinationDataType:
		return LevelToInt(detail.Level) >= LevelToInt(config.GetModelHallucinationLevelBar(consumer))
	case CustomLabelType:
		return LevelToInt(detail.Level) >= LevelToInt(config.GetCustomLabelLevelBar(consumer))
	default:
		return false
	}
}

func IsRiskLevelAcceptable(action string, data Data, config AISecurityConfig, consumer string) bool {
	return EvaluateRisk(action, data, config, consumer) != RiskBlock
}

// ExtractDesensitization extracts the desensitization content from the first Detail
// with Type=sensitiveData and Suggestion=mask. Returns empty string if no such
// Detail exists, if the Detail has no Result entries, or if the desensitization
// content is empty.
func ExtractDesensitization(data Data) string {
	for _, detail := range data.Detail {
		if detail.Type == SensitiveDataType && detail.Suggestion == "mask" {
			if len(detail.Result) > 0 && detail.Result[0].Ext.Desensitization != "" {
				return detail.Result[0].Ext.Desensitization
			}
		}
	}
	return ""
}

type BlockedDetail struct {
	Type  string `json:"type"`
	Level string `json:"level"`
}

type DenyResponseBody struct {
	Code           int             `json:"code"`
	DenyMessage    string          `json:"denyMessage,omitempty"`
	BlockedDetails []BlockedDetail `json:"blockedDetails"`
}

func BuildDenyResponseBody(response Response, config AISecurityConfig, consumer string) ([]byte, error) {
	details := GetUnacceptableDetail(response.Data, config, consumer)
	blocked := make([]BlockedDetail, 0, len(details))
	for _, d := range details {
		blocked = append(blocked, BlockedDetail{
			Type:  d.Type,
			Level: d.Level,
		})
	}
	body := DenyResponseBody{
		Code:           response.Code,
		DenyMessage:    config.DenyMessage,
		BlockedDetails: blocked,
	}
	return json.Marshal(body)
}

// ResolveDenyMessage returns the human-readable deny text used both for
// non-original OpenAI wrappers (message.content / delta.content) and for the
// x_higress_guardrail.denyMessage field, ensuring the two stay aligned.
func ResolveDenyMessage(config AISecurityConfig) string {
	if config.DenyMessage != "" {
		return config.DenyMessage
	}
	return DefaultDenyMessage
}

// BuildOpenAIDenyResponseBody builds the guardrail JSON object embedded by the
// outer OpenAI structured template as choices[0].x_higress_guardrail. Its shape
// mirrors DenyResponseBody, but DenyMessage is filled via ResolveDenyMessage so
// the field is always present and consistent with the rendered content.
// Code is sourced from config.DenyCode so that x_higress_guardrail.code
// consistently represents "the HTTP status this gateway returns to the client",
// aligned with BuildOpenAIFallbackDenyResponseBody.
func BuildOpenAIDenyResponseBody(response Response, config AISecurityConfig, consumer string) ([]byte, error) {
	details := GetUnacceptableDetail(response.Data, config, consumer)
	blocked := make([]BlockedDetail, 0, len(details))
	for _, d := range details {
		blocked = append(blocked, BlockedDetail{
			Type:  d.Type,
			Level: d.Level,
		})
	}
	body := DenyResponseBody{
		Code:           int(config.DenyCode),
		DenyMessage:    ResolveDenyMessage(config),
		BlockedDetails: blocked,
	}
	return json.Marshal(body)
}

// BuildOpenAIFallbackDenyResponseBody builds the guardrail JSON object embedded
// by the outer OpenAI structured template as choices[0].x_higress_guardrail for
// mask→block fallback paths. The fallback is triggered by
// ReplaceJsonFieldTextContent failure or empty desensitization, so there is no
// upstream Response object to derive blockedDetails from.
func BuildOpenAIFallbackDenyResponseBody(config AISecurityConfig) ([]byte, error) {
	body := DenyResponseBody{
		Code:           int(config.DenyCode),
		DenyMessage:    ResolveDenyMessage(config),
		BlockedDetails: []BlockedDetail{},
	}
	return json.Marshal(body)
}

func openAIDenyContentType(isStream bool) [][2]string {
	if isStream {
		return [][2]string{{"content-type", "text/event-stream;charset=UTF-8"}}
	}
	return [][2]string{{"content-type", "application/json"}}
}

// BuildOpenAIDenyData builds the complete OpenAI-formatted deny response bytes
// (structured or legacy). Callers that need raw bytes (e.g. streaming response
// handlers using InjectEncodedDataToFilterChain) use this directly; callers that
// want a full SendHttpResponse dispatch should use SendDenyResponse instead.
func BuildOpenAIDenyData(config AISecurityConfig, response Response, consumer string, isStream bool) ([]byte, error) {
	if config.OpenAIDenyResponseFormat == OpenAIDenyResponseFormatStructured {
		guardrailBody, err := BuildOpenAIDenyResponseBody(response, config, consumer)
		if err != nil {
			return nil, err
		}
		marshalledDenyMessage := wrapper.MarshalStr(ResolveDenyMessage(config))
		randomID := utils.GenerateRandomChatID()
		createdTs := time.Now().Unix()
		if isStream {
			return []byte(fmt.Sprintf(OpenAIStreamResponseFormatStructured, randomID, createdTs, marshalledDenyMessage, randomID, createdTs, string(guardrailBody))), nil
		}
		return []byte(fmt.Sprintf(OpenAIResponseFormatStructured, randomID, createdTs, marshalledDenyMessage, string(guardrailBody))), nil
	}
	denyBody, err := BuildDenyResponseBody(response, config, consumer)
	if err != nil {
		return nil, err
	}
	marshalledDenyBody := wrapper.MarshalStr(string(denyBody))
	randomID := utils.GenerateRandomChatID()
	createdTs := time.Now().Unix()
	if isStream {
		return []byte(fmt.Sprintf(OpenAIStreamResponseFormatLegacy, randomID, createdTs, marshalledDenyBody, randomID, createdTs)), nil
	}
	return []byte(fmt.Sprintf(OpenAIResponseFormatLegacy, randomID, createdTs, marshalledDenyBody)), nil
}

// SendDenyResponse dispatches a deny HTTP response in the appropriate format
// (ProtocolOriginal, Structured, or Legacy). It returns an error only if
// building the response body fails; the caller should handle the error
// (e.g. log, mark guardrail error, resume request/response).
func SendDenyResponse(config AISecurityConfig, response Response, consumer string, isStream bool) error {
	if config.ProtocolOriginal {
		denyBody, err := BuildDenyResponseBody(response, config, consumer)
		if err != nil {
			return err
		}
		proxywasm.SendHttpResponse(uint32(config.DenyCode), [][2]string{{"content-type", "application/json"}}, denyBody, -1)
		return nil
	}
	data, err := BuildOpenAIDenyData(config, response, consumer, isStream)
	if err != nil {
		return err
	}
	proxywasm.SendHttpResponse(uint32(config.DenyCode), openAIDenyContentType(isStream), data, -1)
	return nil
}

// SendFallbackDenyResponse dispatches a fallback deny HTTP response when no
// upstream Response object is available (e.g. mask-to-block on replace error
// or empty desensitization). For ProtocolOriginal it sends the plain deny
// message; for OpenAI formats it wraps it in the appropriate template with
// an empty-blockedDetails guardrail object (structured) or the deny message
// as content (legacy).
func SendFallbackDenyResponse(config AISecurityConfig, isStream bool) error {
	marshalledDenyMessage := wrapper.MarshalStr(ResolveDenyMessage(config))
	if config.ProtocolOriginal {
		proxywasm.SendHttpResponse(uint32(config.DenyCode), [][2]string{{"content-type", "application/json"}}, []byte(marshalledDenyMessage), -1)
		return nil
	}
	randomID := utils.GenerateRandomChatID()
	createdTs := time.Now().Unix()
	if config.OpenAIDenyResponseFormat == OpenAIDenyResponseFormatStructured {
		guardrailBody, err := BuildOpenAIFallbackDenyResponseBody(config)
		if err != nil {
			return err
		}
		var data []byte
		if isStream {
			data = []byte(fmt.Sprintf(OpenAIStreamResponseFormatStructured, randomID, createdTs, marshalledDenyMessage, randomID, createdTs, string(guardrailBody)))
		} else {
			data = []byte(fmt.Sprintf(OpenAIResponseFormatStructured, randomID, createdTs, marshalledDenyMessage, string(guardrailBody)))
		}
		proxywasm.SendHttpResponse(uint32(config.DenyCode), openAIDenyContentType(isStream), data, -1)
		return nil
	}
	var data []byte
	if isStream {
		data = []byte(fmt.Sprintf(OpenAIStreamResponseFormatLegacy, randomID, createdTs, marshalledDenyMessage, randomID, createdTs))
	} else {
		data = []byte(fmt.Sprintf(OpenAIResponseFormatLegacy, randomID, createdTs, marshalledDenyMessage))
	}
	proxywasm.SendHttpResponse(uint32(config.DenyCode), openAIDenyContentType(isStream), data, -1)
	return nil
}

func GetUnacceptableDetail(data Data, config AISecurityConfig, consumer string) []Detail {
	result := []Detail{}
	for _, detail := range data.Detail {
		dimAction, _ := config.ResolveRiskActionByType(consumer, detail.Type)
		exceeds := detailExceedsThreshold(detail, config, consumer)
		if detailTriggersBlock(detail, dimAction, exceeds) {
			result = append(result, detail)
		}
	}
	// Fallback: when the security service returns a top-level risk signal but no Detail entries,
	// synthesise detail items from RiskLevel/AttackLevel so blockedDetails is never empty on a
	// real block event.
	if len(result) == 0 {
		if LevelToInt(data.RiskLevel) >= LevelToInt(config.GetContentModerationLevelBar(consumer)) {
			result = append(result, Detail{
				Type:       ContentModerationType,
				Level:      data.RiskLevel,
				Suggestion: "block",
			})
		}
		if LevelToInt(data.AttackLevel) >= LevelToInt(config.GetPromptAttackLevelBar(consumer)) {
			result = append(result, Detail{
				Type:       PromptAttackType,
				Level:      data.AttackLevel,
				Suggestion: "block",
			})
		}
	}
	return result
}
