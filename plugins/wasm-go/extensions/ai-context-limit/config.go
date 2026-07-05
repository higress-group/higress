// Copyright (c) 2026 Alibaba Group Holding Ltd.
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

package main

import (
	"encoding/json"
	"fmt"

	"github.com/tidwall/gjson"
)

// Config 上下文限制插件配置
type Config struct {
	// MaxContextTokens 必填，输入侧 token 上限（用户阈值）
	MaxContextTokens int `json:"max_context_tokens"`
	// ErrorStatusCode 超限响应码，默认 400
	ErrorStatusCode int `json:"error_status_code"`
	// BufferRatio token 预估值放大系数，默认 1.10
	BufferRatio float64 `json:"buffer_ratio"`
}

const (
	defaultErrorStatusCode = 400
	defaultBufferRatio     = 1.10
	// MaxRequestBodyBytes 强制调大的 envoy 请求体 buffer 上限
	// 上下文限制仅需要读取请求体中的文本输入，8MB 可覆盖常见长上下文请求。
	MaxRequestBodyBytes uint32 = 8 * 1024 * 1024
)

// parseConfig 解析 WasmPlugin defaultConfig 字段
func parseConfig(jsonConfig gjson.Result, cfg *Config) error {
	if err := json.Unmarshal([]byte(jsonConfig.Raw), cfg); err != nil {
		return fmt.Errorf("parse config failed: %w", err)
	}
	if cfg.MaxContextTokens < 0 {
		return fmt.Errorf("max_context_tokens must be non-negative, got %d", cfg.MaxContextTokens)
	}
	if cfg.MaxContextTokens == 0 {
		// 阈值为 0 视为未启用，不拦截请求（防止误配置导致全量 5xx）
		return nil
	}
	if cfg.ErrorStatusCode == 0 {
		cfg.ErrorStatusCode = defaultErrorStatusCode
	} else if cfg.ErrorStatusCode < 400 || cfg.ErrorStatusCode > 599 {
		return fmt.Errorf("error_status_code must be between 400 and 599, got %d", cfg.ErrorStatusCode)
	}
	if cfg.BufferRatio < 0 {
		return fmt.Errorf("buffer_ratio must be non-negative, got %f", cfg.BufferRatio)
	}
	if cfg.BufferRatio == 0 {
		cfg.BufferRatio = defaultBufferRatio
	} else if cfg.BufferRatio > 10 {
		return fmt.Errorf("buffer_ratio must not exceed 10, got %f", cfg.BufferRatio)
	}
	return nil
}

// IsEnabled 判断当前配置是否需要执行拦截
func (c *Config) IsEnabled() bool {
	return c.MaxContextTokens > 0
}
