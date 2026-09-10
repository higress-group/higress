package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	aiclientcontext "model-router/ai-client-context"
	"net/http"
	"net/textproto"
	"regexp"
	"strings"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
	"github.com/tidwall/resp"
	"github.com/tidwall/sjson"
)

const (
	DefaultMaxBodyBytes = 100 * 1024 * 1024 // 100MB
	AutoModelPrefix     = "higress/auto"
)

func main() {}

func init() {
	wrapper.SetCtx(
		"model-router",
		wrapper.ParseConfig(parseConfig),
		wrapper.ProcessRequestHeaders(onHttpRequestHeaders),
		wrapper.ProcessRequestBody(onHttpRequestBody),
		wrapper.WithRebuildMaxMemBytes[ModelRouterConfig](200*1024*1024),
	)
}

// AutoRoutingRule defines a regex-based routing rule for auto model selection
type AutoRoutingRule struct {
	Pattern *regexp.Regexp
	Model   string
}

type ModelRouterConfig struct {
	redisInfo             RedisInfo `yaml:"redis"`
	RedisKeyPrefix        string    `yaml:"redis_key_prefix"`
	modelKey              string
	addProviderHeader     string
	modelToHeader         string
	enableOnPathSuffix    []string
	keepOriginalModelName bool
	// Auto routing configuration
	enableAutoRouting          bool
	enableAutoRoutingAgentMode bool
	autoRoutingRules           []AutoRoutingRule
	defaultModel               string
	redisClient                wrapper.RedisClient
}

type RedisInfo struct {
	ServiceName string `required:"true" yaml:"service_name" json:"service_name"`
	ServicePort int    `required:"false" yaml:"service_port" json:"service_port"`
	Username    string `required:"false" yaml:"username" json:"username"`
	Password    string `required:"false" yaml:"password" json:"password"`
	Timeout     int    `required:"false" yaml:"timeout" json:"timeout"`
	Database    int    `required:"false" yaml:"database" json:"database"`
}

func parseConfig(json gjson.Result, config *ModelRouterConfig) error {
	config.modelKey = json.Get("modelKey").String()
	if config.modelKey == "" {
		config.modelKey = "model"
	}
	config.addProviderHeader = json.Get("addProviderHeader").String()
	config.modelToHeader = json.Get("modelToHeader").String()
	config.keepOriginalModelName = json.Get("keepOriginalModelName").Bool()

	enableOnPathSuffix := json.Get("enableOnPathSuffix")
	if enableOnPathSuffix.Exists() && enableOnPathSuffix.IsArray() {
		for _, item := range enableOnPathSuffix.Array() {
			config.enableOnPathSuffix = append(config.enableOnPathSuffix, item.String())
		}
	} else {
		// Default suffixes if not provided
		config.enableOnPathSuffix = []string{
			"/completions",
			"/embeddings",
			"/images/generations",
			"/audio/speech",
			"/fine_tuning/jobs",
			"/moderations",
			"/image-synthesis",
			"/video-synthesis",
			"/rerank",
			"/messages",
			"/responses",
		}
	}

	// Parse auto routing configuration
	autoRouting := json.Get("autoRouting")
	if autoRouting.Exists() {
		config.enableAutoRouting = autoRouting.Get("enable").Bool()
		config.defaultModel = autoRouting.Get("defaultModel").String()
		config.enableAutoRoutingAgentMode = autoRouting.Get("agentMode").Bool()

		rules := autoRouting.Get("rules")
		if rules.Exists() && rules.IsArray() {
			for _, rule := range rules.Array() {
				patternStr := rule.Get("pattern").String()
				model := rule.Get("model").String()
				if patternStr == "" || model == "" {
					log.Warnf("skipping invalid auto routing rule: pattern=%s, model=%s", patternStr, model)
					continue
				}
				compiled, err := regexp.Compile(patternStr)
				if err != nil {
					log.Warnf("failed to compile regex pattern '%s': %v", patternStr, err)
					continue
				}
				config.autoRoutingRules = append(config.autoRoutingRules, AutoRoutingRule{
					Pattern: compiled,
					Model:   model,
				})
				log.Debugf("loaded auto routing rule: pattern=%s, model=%s", patternStr, model)
			}
		}

		if config.enableAutoRoutingAgentMode {
			// Redis
			config.RedisKeyPrefix = json.Get("redis_key_prefix").String()
			if config.RedisKeyPrefix == "" {
				config.RedisKeyPrefix = "chat_quota:"
			}
			redisConfig := json.Get("redis")
			if !redisConfig.Exists() {
				return errors.New("missing redis in config")
			}
			serviceName := redisConfig.Get("service_name").String()
			if serviceName == "" {
				return errors.New("redis service name must not be empty")
			}
			servicePort := int(redisConfig.Get("service_port").Int())
			if servicePort == 0 {
				if strings.HasSuffix(serviceName, ".static") {
					// use default logic port which is 80 for static service
					servicePort = 80
				} else {
					servicePort = 6379
				}
			}
			username := redisConfig.Get("username").String()
			password := redisConfig.Get("password").String()
			timeout := int(redisConfig.Get("timeout").Int())
			if timeout == 0 {
				timeout = 1000
			}
			database := int(redisConfig.Get("database").Int())
			config.redisInfo.ServiceName = serviceName
			config.redisInfo.ServicePort = servicePort
			config.redisInfo.Username = username
			config.redisInfo.Password = password
			config.redisInfo.Timeout = timeout
			config.redisInfo.Database = database
			config.redisClient = wrapper.NewRedisClusterClient(wrapper.FQDNCluster{
				FQDN: serviceName,
				Port: int64(servicePort),
			})

			return config.redisClient.Init(username, password, int64(timeout), wrapper.WithDataBase(database))
		}
	}

	return nil
}

func onHttpRequestHeaders(ctx wrapper.HttpContext, config ModelRouterConfig) types.Action {
	path, err := proxywasm.GetHttpRequestHeader(":path")
	if err != nil {
		return types.ActionContinue
	}

	// Remove query parameters for suffix check
	if idx := strings.Index(path, "?"); idx != -1 {
		path = path[:idx]
	}

	enable := false
	for _, suffix := range config.enableOnPathSuffix {
		if suffix == "*" || strings.HasSuffix(path, suffix) {
			enable = true
			break
		}
	}

	if !enable || !ctx.HasRequestBody() {
		ctx.DontReadRequestBody()
		return types.ActionContinue
	}

	// Prepare for body processing
	proxywasm.RemoveHttpRequestHeader("content-length")
	// 100MB buffer limit
	ctx.SetRequestBodyBufferLimit(DefaultMaxBodyBytes)

	return types.HeaderStopIteration
}

func onHttpRequestBody(ctx wrapper.HttpContext, config ModelRouterConfig, body []byte) types.Action {
	contentType, err := proxywasm.GetHttpRequestHeader("content-type")
	if err != nil {
		return types.ActionContinue
	}

	if strings.Contains(contentType, "application/json") {
		return handleJsonBody(ctx, config, body)
	} else if strings.Contains(contentType, "multipart/form-data") {
		return handleMultipartBody(ctx, config, body, contentType)
	}

	return types.ActionContinue
}

// extractLastUserMessage extracts the content of the last message with role "user" from the messages array
func extractLastUserMessage(body []byte) string {
	// Chat Completions API
	messages := gjson.GetBytes(body, "messages")
	if messages.Exists() && messages.IsArray() {
		return extractLastUserFromMessages(messages)
	}

	// Responses API
	input := gjson.GetBytes(body, "input")
	if input.Exists() {
		return extractLastUserFromInput(input)
	}

	return ""
}

func extractLastUserFromMessages(messages gjson.Result) string {
	var lastUserContent string

	for _, msg := range messages.Array() {
		if msg.Get("role").String() != "user" {
			continue
		}

		content := msg.Get("content")

		if content.IsArray() {
			for _, item := range content.Array() {
				if item.Get("type").String() == "text" {
					lastUserContent = item.Get("text").String()
				}
			}
		} else if content.Type == gjson.String {
			lastUserContent = content.String()
		}
	}

	return lastUserContent
}

func extractLastUserFromInput(input gjson.Result) string {
	// input: "hello"
	if input.Type == gjson.String {
		return input.String()
	}

	if !input.IsArray() {
		return ""
	}

	var lastUserContent string

	for _, item := range input.Array() {
		if item.Get("role").String() != "user" {
			continue
		}

		content := item.Get("content")

		// content: "hello"
		if content.Type == gjson.String {
			lastUserContent = content.String()
			continue
		}

		// content: [{type:"input_text", text:"hello"}]
		if content.IsArray() {
			for _, part := range content.Array() {
				switch part.Get("type").String() {
				case "input_text", "text":
					if text := part.Get("text").String(); text != "" {
						lastUserContent = text
					}
				}
			}
		}
	}

	return lastUserContent
}

// matchAutoRoutingRule matches the user message against auto routing rules and returns the matched model
func matchAutoRoutingRule(config ModelRouterConfig, userMessage string) (string, bool) {
	for _, rule := range config.autoRoutingRules {
		if rule.Pattern.MatchString(userMessage) {
			log.Debugf("auto routing rule matched: pattern=%s, model=%s", rule.Pattern.String(), rule.Model)
			return rule.Model, true
		}
	}
	return "", false
}

func handleJsonBody(ctx wrapper.HttpContext, config ModelRouterConfig, body []byte) types.Action {
	if !json.Valid(body) {
		log.Error("invalid json body")
		return types.ActionContinue
	}
	modelValue := gjson.GetBytes(body, config.modelKey).String()
	if modelValue == "" {
		return types.ActionContinue
	}

	// Check if auto routing should be triggered
	if config.enableAutoRouting && modelValue == AutoModelPrefix {
		userMessage := extractLastUserMessage(body)
		var targetModel string
		if userMessage != "" {
			if config.enableAutoRoutingAgentMode {
				var agentLoopID string
				for _, client := range aiclientcontext.NewClientList() {
					if !client.Match() {
						continue
					}
					agentMetadata, ok := client.ExtractContext(body)
					if !ok {
						break
					}
					agentLoopID = agentMetadata.LoopID
				}
				err := getAgentLoopIdFromRedis(config.redisClient, agentLoopID, func(value string, err error) {
					if err != nil || value == "" {
						log.Errorf("get agent loop id  failed: %v", err)
						if matchedModel, found := matchAutoRoutingRule(config, userMessage); found {
							targetModel = matchedModel
							log.Infof("auto routing: user message matched, routing to model: %s", matchedModel)
						}
						err = insertAgentLoopIdInRedis(config.redisClient, agentLoopID, targetModel, func(err error) {
							if err != nil {
								log.Errorf("insert redis failed: %v", err)
								return
							}

							log.Infof("insert redis success")
						})

						if err != nil {
							log.Errorf("dispatch redis call failed: %v", err)
						}

					} else {
						log.Infof("get agent loop id success, value is %s", value)
						targetModel = value
					}

					proxywasm.ResumeHttpRequest()

				})
				if err != nil {
					log.Errorf("get agent loop id function failed: %v", err)
					return types.ActionContinue
				}

				return types.ActionPause
			} else {
				if matchedModel, found := matchAutoRoutingRule(config, userMessage); found {
					targetModel = matchedModel
					log.Infof("auto routing: user message matched, routing to model: %s", matchedModel)
				}
			}
		}
		// No rule matched, use default model if configured
		if targetModel == "" && config.defaultModel != "" {
			targetModel = config.defaultModel
			log.Infof("auto routing: no rule matched, using default model: %s", config.defaultModel)
		}

		if targetModel != "" {
			// Set the matched model to the header for routing
			_ = proxywasm.ReplaceHttpRequestHeader("x-higress-llm-model", targetModel)
			// Update the model field in the request body
			newBody, err := sjson.SetBytes(body, config.modelKey, targetModel)
			if err != nil {
				log.Errorf("failed to update model in auto routing json body: %v", err)
				return types.ActionContinue
			}
			_ = proxywasm.ReplaceHttpRequestBody(newBody)
			log.Debugf("auto routing: updated body model field to: %s", targetModel)
		} else {
			log.Warnf("auto routing: no rule matched and no default model configured")
		}
		return types.ActionContinue
	}

	if config.modelToHeader != "" {
		_ = proxywasm.ReplaceHttpRequestHeader(config.modelToHeader, modelValue)
	}

	if config.addProviderHeader != "" {
		parts := strings.SplitN(modelValue, "/", 2)
		if len(parts) == 2 {
			provider := parts[0]
			model := parts[1]
			_ = proxywasm.ReplaceHttpRequestHeader(config.addProviderHeader, provider)

			if !config.keepOriginalModelName {
				newBody, err := sjson.SetBytes(body, config.modelKey, model)
				if err != nil {
					log.Errorf("failed to update model in json body: %v", err)
					return types.ActionContinue
				}
				_ = proxywasm.ReplaceHttpRequestBody(newBody)
			}
			log.Debugf("model route to provider: %s, model: %s", provider, model)
		} else {
			log.Debugf("model route to provider not work, model: %s", modelValue)
		}
	}

	return types.ActionContinue
}

func insertAgentLoopIdInRedis(
	client wrapper.RedisClient,
	turnId string,
	modelValue string,
	onDone func(error),
) error {

	key := fmt.Sprintf(
		"%s%s",
		aiclientcontext.PrefixKey,
		turnId,
	)

	err := client.Command(
		[]interface{}{
			"SET",
			key,
			modelValue,
		},
		func(response resp.Value) {
			if response.Error() != nil {
				onDone(response.Error())
				return
			}

			onDone(nil)
		},
	)

	return err
}

func getAgentLoopIdFromRedis(
	client wrapper.RedisClient,
	turnId string,
	onDone func(string, error),
) error {

	key := fmt.Sprintf(
		"%s%s",
		aiclientcontext.PrefixKey,
		turnId,
	)

	err := client.Command(
		[]interface{}{
			"GET",
			key,
		},
		func(response resp.Value) {
			if response.Error() != nil {
				onDone("", response.Error())
				return
			}

			// Redis key 不存在
			if response.Type == nil {
				onDone("", nil)
				return
			}

			value := response.String()

			onDone(value, nil)
		},
	)

	return err
}

func handleMultipartBody(ctx wrapper.HttpContext, config ModelRouterConfig, body []byte, contentType string) types.Action {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		log.Errorf("failed to parse content type: %v", err)
		return types.ActionContinue
	}
	boundary, ok := params["boundary"]
	if !ok {
		log.Errorf("no boundary in content type")
		return types.ActionContinue
	}

	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	var newBody bytes.Buffer
	writer := multipart.NewWriter(&newBody)
	writer.SetBoundary(boundary)

	modified := false

	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Errorf("failed to read multipart part: %v", err)
			return types.ActionContinue
		}

		// Read part content
		partContent, err := io.ReadAll(part)
		if err != nil {
			log.Errorf("failed to read part content: %v", err)
			return types.ActionContinue
		}

		formName := part.FormName()
		if formName == config.modelKey {
			modelValue := string(partContent)

			if config.modelToHeader != "" {
				_ = proxywasm.ReplaceHttpRequestHeader(config.modelToHeader, modelValue)
			}

			if config.addProviderHeader != "" {
				parts := strings.SplitN(modelValue, "/", 2)
				if len(parts) == 2 {
					provider := parts[0]
					model := parts[1]
					_ = proxywasm.ReplaceHttpRequestHeader(config.addProviderHeader, provider)

					if !config.keepOriginalModelName {
						// Write modified part
						h := make(http.Header)
						for k, v := range part.Header {
							h[k] = v
						}

						pw, err := writer.CreatePart(textproto.MIMEHeader(h))
						if err != nil {
							log.Errorf("failed to create part: %v", err)
							return types.ActionContinue
						}
						_, err = pw.Write([]byte(model))
						if err != nil {
							log.Errorf("failed to write part content: %v", err)
							return types.ActionContinue
						}
						modified = true
						log.Debugf("model route to provider: %s, model: %s", provider, model)
						continue
					}
					log.Debugf("model route to provider: %s, model kept: %s", provider, modelValue)
				} else {
					log.Debugf("model route to provider not work, model: %s", modelValue)
				}
			}
		}

		// Write original part
		h := make(http.Header)
		for k, v := range part.Header {
			h[k] = v
		}
		pw, err := writer.CreatePart(textproto.MIMEHeader(h))
		if err != nil {
			log.Errorf("failed to create part: %v", err)
			return types.ActionContinue
		}
		_, err = pw.Write(partContent)
		if err != nil {
			log.Errorf("failed to write part content: %v", err)
			return types.ActionContinue
		}
	}

	writer.Close()

	if modified {
		_ = proxywasm.ReplaceHttpRequestBody(newBody.Bytes())
	}

	return types.ActionContinue
}
