// Copyright (c) 2023 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package config

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/go-jose/go-jose/v3"
	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/tidwall/gjson"
)

const maxJWKsFetchTimeout = int64(10 * 1000)      // milliseconds
const maxJWKsCacheDuration = int64(7 * 24 * 3600) // seconds
const minJWKsCacheDuration = RemoteJWKsMinRefreshIntervalSeconds

// ParseGlobalConfig 从wrapper提供的配置中解析并转换到插件运行时需要使用的配置。
// 此处解析的是全局配置，域名和路由级配置由 ParseRuleConfig 负责。
func ParseGlobalConfig(json gjson.Result, config *JWTAuthConfig, log log.Log) error {
	config.RuleSet = len(json.Get("_rules_").Array()) > 0
	consumers := json.Get("consumers")
	if !consumers.IsArray() {
		return fmt.Errorf("failed to parse configuration for consumers: consumers is not a array")
	}

	consumerNames := map[string]struct{}{}
	for _, v := range consumers.Array() {
		c, err := ParseConsumer(v, consumerNames)
		if err != nil {
			log.Warn(err.Error())
			continue
		}
		config.Consumers = append(config.Consumers, c)
	}
	if len(config.Consumers) == 0 {
		return fmt.Errorf("at least one consumer should be configured for a rule")
	}

	return nil
}

// ParseRuleConfig 从wrapper提供的配置中解析并转换到插件运行时需要使用的配置。
// 此处解析的是域名和路由级配置，全局配置由 ParseConfig 负责。
func ParseRuleConfig(json gjson.Result, global JWTAuthConfig, config *JWTAuthConfig, log log.Log) error {
	// override config via global
	*config = global

	allow := json.Get("allow")
	if !allow.Exists() {
		return fmt.Errorf("allow is required")
	}

	if len(allow.Array()) == 0 {
		return fmt.Errorf("allow cannot be empty")
	}

	for _, item := range allow.Array() {
		config.Allow = append(config.Allow, item.String())
	}

	config.RuleSet = true
	return nil
}

func ParseConsumer(consumer gjson.Result, names map[string]struct{}) (c *Consumer, err error) {
	c = &Consumer{}

	// 从gjson中取得原始JSON字符串，并使用标准库反序列化，以降低代码复杂度。
	err = json.Unmarshal([]byte(consumer.Raw), c)
	if err != nil {
		return nil, fmt.Errorf("failed to parse consumer: %s", err.Error())
	}

	// 检查consumer是否重复
	if _, ok := names[c.Name]; ok {
		return nil, fmt.Errorf("consumer already exists: %s", c.Name)
	}

	c.Issuer = strings.TrimSpace(c.Issuer)
	c.JWKs = strings.TrimSpace(c.JWKs)
	if c.RemoteJWKs != nil {
		normalizeRemoteJWKs(c.RemoteJWKs)
	}
	if c.JWKs == "" && c.RemoteJWKs == nil {
		return nil, fmt.Errorf("one of jwks and remote_jwks is required, consumer:%s", c.Name)
	}
	if c.JWKs != "" && c.RemoteJWKs != nil {
		return nil, fmt.Errorf("only one of jwks and remote_jwks can be configured, consumer:%s", c.Name)
	}
	if c.JWKs != "" {
		if c.JWKsCacheDuration != nil || c.JWKsFetchTimeout != nil {
			return nil, fmt.Errorf("jwks_cache_duration and jwks_fetch_timeout only apply to remote_jwks, consumer:%s", c.Name)
		}
		// Validate inline JWKS before accepting the consumer.
		jwks := &jose.JSONWebKeySet{}
		err = json.Unmarshal([]byte(c.JWKs), jwks)
		if err != nil {
			return nil, fmt.Errorf("jwks is invalid, consumer:%s, status:%s, jwks:%s", c.Name, err.Error(), c.JWKs)
		}
		if len(jwks.Keys) == 0 {
			return nil, fmt.Errorf("jwks is empty, consumer:%s", c.Name)
		}
		c.ParsedJWKs = jwks
	}
	if c.RemoteJWKs != nil {
		if c.Issuer == "" {
			return nil, fmt.Errorf("issuer is required when remote_jwks is set, consumer:%s", c.Name)
		}
		if err := validateRemoteJWKs(c.RemoteJWKs); err != nil {
			return nil, fmt.Errorf("remote_jwks is invalid, consumer:%s, reason:%s", c.Name, err.Error())
		}
	}

	// 检查是否需要使用默认jwt抽取来源
	if c.FromHeaders == nil && c.FromParams == nil && c.FromCookies == nil {
		c.FromHeaders = &DefaultFromHeader
		c.FromParams = &DefaultFromParams
		c.FromCookies = &DefaultFromCookies
	}

	// 检查ClaimsToHeaders
	if c.ClaimsToHeaders != nil {
		// header去重
		c2h := map[string]struct{}{}

		// 此处需要先把指针解引用到临时变量
		tmp := *c.ClaimsToHeaders
		for i := range tmp {
			if _, ok := c2h[tmp[i].Header]; ok {
				return nil, fmt.Errorf("claim to header already exists: %s", c2h[tmp[i].Header])
			}
			c2h[tmp[i].Header] = struct{}{}

			// 为Override填充默认值
			if tmp[i].Override == nil {
				tmp[i].Override = &DefaultClaimToHeaderOverride
			}
		}
	}

	// 为ClockSkewSeconds填充默认值
	if c.ClockSkewSeconds == nil {
		c.ClockSkewSeconds = &DefaultClockSkewSeconds
	}

	// 为KeepToken填充默认值
	if c.KeepToken == nil {
		c.KeepToken = &DefaultKeepToken
	}

	if c.RemoteJWKs != nil {
		// Fill the default remote JWKS cache duration.
		if c.JWKsCacheDuration == nil {
			v := DefaultJWKsCacheDuration
			c.JWKsCacheDuration = &v
		}
		if *c.JWKsCacheDuration <= 0 {
			return nil, fmt.Errorf("jwks_cache_duration must be positive, consumer:%s", c.Name)
		}
		if *c.JWKsCacheDuration < minJWKsCacheDuration {
			return nil, fmt.Errorf("jwks_cache_duration must be greater than or equal to %d, consumer:%s", minJWKsCacheDuration, c.Name)
		}
		if *c.JWKsCacheDuration > maxJWKsCacheDuration {
			return nil, fmt.Errorf("jwks_cache_duration must be less than or equal to %d, consumer:%s", maxJWKsCacheDuration, c.Name)
		}

		// Fill the default remote JWKS fetch timeout.
		if c.JWKsFetchTimeout == nil {
			v := DefaultJWKsFetchTimeout
			c.JWKsFetchTimeout = &v
		}
		if *c.JWKsFetchTimeout <= 0 {
			return nil, fmt.Errorf("jwks_fetch_timeout must be positive, consumer:%s", c.Name)
		}
		if *c.JWKsFetchTimeout > maxJWKsFetchTimeout {
			return nil, fmt.Errorf("jwks_fetch_timeout must be less than or equal to %d, consumer:%s", maxJWKsFetchTimeout, c.Name)
		}
	}

	// consumer合法，记录consumer名称
	names[c.Name] = struct{}{}
	return c, nil
}

func normalizeRemoteJWKs(remote *RemoteJWKs) {
	remote.ServiceName = strings.TrimSpace(remote.ServiceName)
	remote.ServiceHost = strings.TrimSpace(remote.ServiceHost)
	remote.Path = strings.TrimSpace(remote.Path)
	if remote.ServicePort == nil {
		v := int64(443)
		remote.ServicePort = &v
	}
}

func validateRemoteJWKs(remote *RemoteJWKs) error {
	if remote.ServiceName == "" {
		return fmt.Errorf("service_name is required")
	}
	if hasInvalidRemoteJWKsFieldChar(remote.ServiceName) || strings.ContainsAny(remote.ServiceName, "|/?#@:") {
		return fmt.Errorf("service_name must not contain whitespace, control characters, or URI separators")
	}
	if remote.ServiceHost == "" {
		return fmt.Errorf("service_host is required")
	}
	if hasInvalidRemoteJWKsFieldChar(remote.ServiceHost) {
		return fmt.Errorf("service_host must not contain whitespace or control characters")
	}
	if strings.ContainsAny(remote.ServiceHost, "/?#@:") || strings.Contains(remote.ServiceHost, "://") {
		return fmt.Errorf("service_host must be a host without port")
	}
	if remote.Path == "" || !strings.HasPrefix(remote.Path, "/") {
		return fmt.Errorf("path must start with /")
	}
	if hasInvalidRemoteJWKsFieldChar(remote.Path) {
		return fmt.Errorf("path must not contain whitespace or control characters")
	}
	if *remote.ServicePort <= 0 || *remote.ServicePort > 65535 {
		return fmt.Errorf("service_port is invalid")
	}
	return nil
}

func hasInvalidRemoteJWKsFieldChar(value string) bool {
	return strings.ContainsAny(value, " \t\r\n") || hasControlChar(value)
}

func hasControlChar(value string) bool {
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}
