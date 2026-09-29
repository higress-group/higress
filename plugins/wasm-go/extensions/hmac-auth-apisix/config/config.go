package config

import (
	"encoding/json"
	"errors"

	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/tidwall/gjson"
)

// validAlgorithms allowed_algorithms 配置中允许的算法
var validAlgorithms = map[string]bool{
	"hmac-sha1":   true,
	"hmac-sha256": true,
	"hmac-sha512": true,
}

type HmacAuthConfig struct {
	Consumers           []Consumer `json:"consumers" yaml:"consumers"`
	GlobalAuth          *bool      `json:"global_auth,omitempty" yaml:"global_auth,omitempty"`
	AllowedAlgorithms   []string   `json:"allowed_algorithms,omitempty" yaml:"allowed_algorithms,omitempty"`
	ClockSkew           int        `json:"clock_skew,omitempty" yaml:"clock_skew,omitempty"`
	SignedHeaders       []string   `json:"signed_headers,omitempty" yaml:"signed_headers,omitempty"`
	ValidateRequestBody bool       `json:"validate_request_body,omitempty" yaml:"validate_request_body,omitempty"`
	HideCredentials     bool       `json:"hide_credentials,omitempty" yaml:"hide_credentials,omitempty"`
	AnonymousConsumer   string     `json:"anonymous_consumer,omitempty" yaml:"anonymous_consumer,omitempty"`
	// Allow lists the consumers authorized at the rule scope. It is only parsed from
	// domain/route override rules (ParseOverrideRuleConfig); the global config parser
	// deliberately ignores a global "allow" key, so an empty Allow at global scope means
	// "no extra fine-grained restriction", never "deny all". The fail-closed semantics in
	// onHttpRequestHeaders depend on this invariant together with RuleSet.
	Allow               []string   `json:"allow,omitempty" yaml:"allow,omitempty"`
	// RuleSet 当前请求是否命中了 domain/route 级规则，即插件是否在该 domain/route 上被显式启用
	RuleSet bool `json:"-" yaml:"-"`
}

type Consumer struct {
	Name      string `json:"name,omitempty" yaml:"name,omitempty"`
	AccessKey string `json:"access_key" yaml:"access_key"`
	SecretKey string `json:"secret_key" yaml:"secret_key"`
}

func ParseGlobalConfig(jsonData gjson.Result, global *HmacAuthConfig) error {
	log.Debug("global config")
	global.RuleSet = false

	// 处理 consumers 配置
	consumers := jsonData.Get("consumers")
	if !consumers.Exists() {
		return errors.New("consumers is required")
	}
	if len(consumers.Array()) == 0 {
		return errors.New("consumers cannot be empty")
	}

	accessKeyMap := make(map[string]string)
	for _, item := range consumers.Array() {
		ak := item.Get("access_key")
		if !ak.Exists() || ak.String() == "" {
			return errors.New("consumer access_key is required")
		}
		sk := item.Get("secret_key")
		if !sk.Exists() || sk.String() == "" {
			return errors.New("consumer secret_key is required")
		}
		if _, ok := accessKeyMap[ak.String()]; ok {
			return errors.New("duplicate consumer access_key: " + ak.String())
		}

		consumer := Consumer{
			AccessKey: ak.String(),
			SecretKey: sk.String(),
		}

		name := item.Get("name")
		if name.Exists() && name.String() != "" {
			consumer.Name = name.String()
		} else {
			// 如果没有提供 name，则使用 access_key 作为 name
			consumer.Name = ak.String()
		}

		global.Consumers = append(global.Consumers, consumer)
		accessKeyMap[ak.String()] = ak.String()
	}

	// 处理 global_auth 配置
	globalAuth := jsonData.Get("global_auth")
	if globalAuth.Exists() {
		ga := globalAuth.Bool()
		global.GlobalAuth = &ga
	}

	// 处理 allowed_algorithms 配置
	allowedAlgorithms := jsonData.Get("allowed_algorithms")
	if allowedAlgorithms.Exists() && len(allowedAlgorithms.Array()) > 0 {
		global.AllowedAlgorithms = []string{}
		for _, item := range allowedAlgorithms.Array() {
			algorithm := item.String()
			if !validAlgorithms[algorithm] {
				return errors.New("invalid allowed_algorithm: " + algorithm + ". Must be one of: hmac-sha1, hmac-sha256, hmac-sha512")
			}
			global.AllowedAlgorithms = append(global.AllowedAlgorithms, algorithm)
		}
	} else {
		// 如果未设置，则使用默认值
		global.AllowedAlgorithms = []string{"hmac-sha1", "hmac-sha256", "hmac-sha512"}
	}

	// 处理 clock_skew 配置
	clockSkew := jsonData.Get("clock_skew")
	if !clockSkew.Exists() {
		// 如果未设置，则使用默认值300
		global.ClockSkew = 300
	} else if clockSkew.Int() >= 1 {
		global.ClockSkew = int(clockSkew.Int())
	}

	// 处理 signed_headers 配置
	signedHeaders := jsonData.Get("signed_headers")
	if signedHeaders.Exists() {
		global.SignedHeaders = []string{}
		for _, item := range signedHeaders.Array() {
			global.SignedHeaders = append(global.SignedHeaders, item.String())
		}
	}

	// 处理 validate_request_body 配置
	validateRequestBody := jsonData.Get("validate_request_body")
	if validateRequestBody.Exists() {
		global.ValidateRequestBody = validateRequestBody.Bool()
	}

	// 处理 hide_credentials 配置
	hideCredentials := jsonData.Get("hide_credentials")
	if hideCredentials.Exists() {
		global.HideCredentials = hideCredentials.Bool()
	}

	// 处理 anonymous_consumer 配置
	anonymousConsumer := jsonData.Get("anonymous_consumer")
	if anonymousConsumer.Exists() {
		global.AnonymousConsumer = anonymousConsumer.String()
	}

	if globalBytes, err := json.Marshal(global); err == nil {
		log.Debugf("global: %s", string(globalBytes))
	}
	return nil
}

func ParseOverrideRuleConfig(jsonData gjson.Result, global HmacAuthConfig, config *HmacAuthConfig) error {
	log.Debug("domain/route config")
	*config = global

	// 处理 allow 配置
	allow := jsonData.Get("allow")
	if allow.Exists() {
		config.Allow = []string{}
		consumerNames := make(map[string]bool)
		for _, consumer := range config.Consumers {
			consumerNames[consumer.Name] = true
		}

		for _, item := range allow.Array() {
			allowedName := item.String()
			config.Allow = append(config.Allow, allowedName)

			// 检查允许的名称是否在消费者列表中
			if !consumerNames[allowedName] {
				log.Warnf("allowed consumer name '%s' is not in the consumers list", allowedName)
			}
		}
	}

	config.RuleSet = true
	if configBytes, err := json.Marshal(config); err == nil {
		log.Debugf("config: %s", string(configBytes))
	}
	return nil
}
