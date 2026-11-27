// Copyright (c) 2024-2025
// Beijing Baolande Software Corporation
// All Rights Reserved

package main

import (
	"errors"
	"strings"

	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/higress-group/wasm-go/pkg/wrapper"
	"github.com/tidwall/gjson"
)

const (
	defaultRedisPort      = 6379
	defaultStaticRedisPort = 80
	defaultRedisTimeoutMs = 5000
)

// enable_path_suffixes 缺省值
var defaultEnablePathSuffixes = []string{"/v1/chat/completions", "/v1/responses", "/v1/messages"}

// skip_route_prefixes 缺省值,覆盖内部二跳路由
var defaultSkipRoutePrefixes = []string{"svc-", "infer-", "svc"}

type UserQuotaConfig struct {
	// EnablePathSuffixes 命中才计量,access 检查不受其限制
	EnablePathSuffixes []string
	// SkipRoutePrefixes 命中的路由直接放行,避免回环二跳重复计量
	SkipRoutePrefixes []string
	RedisClient       wrapper.RedisClient
}

func parseConfig(json gjson.Result, cfg *UserQuotaConfig) error {
	suffixes, err := parseEnablePathSuffixes(json)
	if err != nil {
		return err
	}
	cfg.EnablePathSuffixes = suffixes

	cfg.SkipRoutePrefixes, err = parseSkipRoutePrefixes(json)
	if err != nil {
		return err
	}
	return initRedisClient(json, cfg)
}

// parseEnablePathSuffixes 计量后缀,缺失取默认值,不允许为空
func parseEnablePathSuffixes(json gjson.Result) ([]string, error) {
	suffixes, err := parseStringArray(json.Get("enable_path_suffixes"), "enable_path_suffixes", defaultEnablePathSuffixes)
	if err != nil {
		return nil, err
	}
	if len(suffixes) == 0 {
		return nil, errors.New("enable_path_suffixes must not be empty")
	}
	return suffixes, nil
}

// parseSkipRoutePrefixes 跳过的路由前缀,缺失取默认值
func parseSkipRoutePrefixes(json gjson.Result) ([]string, error) {
	return parseStringArray(json.Get("skip_route_prefixes"), "skip_route_prefixes", defaultSkipRoutePrefixes)
}

// parseStringArray 解析字符串数组,缺失按默认值,类型非法报错
func parseStringArray(result gjson.Result, fieldName string, defaults []string) ([]string, error) {
	if !result.Exists() {
		return defaults, nil
	}
	if !result.IsArray() {
		return nil, errors.New(fieldName + " must be an array")
	}
	values := make([]string, 0, len(result.Array()))
	for _, item := range result.Array() {
		value := strings.TrimSpace(item.String())
		if value == "" {
			continue
		}
		values = append(values, value)
	}
	return values, nil
}

func initRedisClient(json gjson.Result, cfg *UserQuotaConfig) error {
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
			servicePort = defaultStaticRedisPort
		} else {
			servicePort = defaultRedisPort
		}
	}
	username := redisConfig.Get("username").String()
	password := redisConfig.Get("password").String()
	timeout := int(redisConfig.Get("timeout").Int())
	if timeout == 0 {
		timeout = defaultRedisTimeoutMs
	}
	database := int(redisConfig.Get("database").Int())

	cfg.RedisClient = wrapper.NewRedisClusterClient(wrapper.FQDNCluster{
		FQDN: serviceName,
		Port: int64(servicePort),
	})
	if err := cfg.RedisClient.Init(username, password, int64(timeout), wrapper.WithDataBase(database)); err != nil {
		log.Errorf("redis init failed: %v", err)
		return err
	}
	if cfg.RedisClient.Ready() {
		log.Infof("redis init successfully")
	} else {
		log.Error("redis init failed, will try later")
	}
	return nil
}