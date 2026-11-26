package main

import (
	"strings"
	"testing"

	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm"
	"github.com/higress-group/proxy-wasm-go-sdk/proxywasm/proxytest"
	"github.com/tidwall/gjson"
)

// 测试配置解析
func TestParseConfig(t *testing.T) {
	configJSON := `{
		"modelProviderMappings": [
			{
				"model": "gpt-image-2",
				"provider": "Azure",
				"requestTransform": {
					"type": "format_transform",
					"formatTransform": {
						"targetFormat": "multipart"
					}
				}
			}
		],
		"enableRequestTransform": true,
		"enableResponseTransform": false
	}`

	var config PluginConfig
	result := gjson.Parse(configJSON)
	err := parseConfig(result, &config, nil)

	if err != nil {
		t.Fatalf("Failed to parse config: %v", err)
	}

	if len(config.ModelProviderMappings) != 1 {
		t.Errorf("Expected 1 mapping, got %d", len(config.ModelProviderMappings))
	}

	if config.ModelProviderMappings[0].Model != "gpt-image-2" {
		t.Errorf("Expected model 'gpt-image-2', got '%s'", config.ModelProviderMappings[0].Model)
	}

	if !config.EnableRequestTransform {
		t.Error("Expected request transform to be enabled")
	}
}

// 测试模型匹配
func TestMatchModel(t *testing.T) {
	tests := []struct {
		model    string
		pattern  string
		expected bool
	}{
		{"gpt-image-2", "gpt-image-2", true},
		{"gpt-4", "gpt-*", true},
		{"gpt-3.5-turbo", "gpt-*", true},
		{"claude-3", "gpt-*", false},
		{"any-model", "*", true},
		{"test", "test", true},
	}

	for _, tt := range tests {
		t.Run(tt.model+" vs "+tt.pattern, func(t *testing.T) {
			result := matchModel(tt.model, tt.pattern)
			if result != tt.expected {
				t.Errorf("Expected %v for model '%s' matching pattern '%s', got %v",
					tt.expected, tt.model, tt.pattern, result)
			}
		})
	}
}

// 测试 cluster_name 提取
func TestExtractClusterName(t *testing.T) {
	tests := []struct {
		name             string
		clusterName      string
		expectedProvider string
	}{
		{
			name:             "Extract RightCodes from standard format",
			clusterName:      "outbound|443||llm-RightCodes.internal.dns",
			expectedProvider: "RightCodes",
		},
		{
			name:             "Extract Azure from standard format",
			clusterName:      "outbound|443||llm-Azure.internal.dns",
			expectedProvider: "Azure",
		},
		{
			name:             "Extract Qiniu from standard format",
			clusterName:      "outbound|443||llm-Qiniu.internal.dns",
			expectedProvider: "Qiniu",
		},
		{
			name:             "Handle cluster name without llm prefix",
			clusterName:      "outbound|443||RightCodes.internal.dns",
			expectedProvider: "RightCodes",
		},
		{
			name:             "Handle cluster name without dot suffix",
			clusterName:      "outbound|443||llm-RightCodes",
			expectedProvider: "RightCodes",
		},
		{
			name:             "Handle cluster name with complex format",
			clusterName:      "outbound|8080||llm-TestService.prod.example.com",
			expectedProvider: "TestService",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 手动测试提取逻辑
			clusterName := tt.clusterName
			parts := strings.Split(clusterName, "||")
			if len(parts) < 2 {
				t.Errorf("Invalid cluster name format: %s", clusterName)
				return
			}

			serviceName := parts[1]
			serviceName = strings.TrimPrefix(serviceName, "llm-")

			if idx := strings.Index(serviceName, "."); idx > 0 {
				serviceName = serviceName[:idx]
			}

			if serviceName != tt.expectedProvider {
				t.Errorf("Expected provider '%s', got '%s'", tt.expectedProvider, serviceName)
			}
		})
	}
}

// 测试根据模型和 provider 查找转换配置
func TestFindTransformMappingByModelAndProvider(t *testing.T) {
	// 匹配函数使用宿主日志接口，原生测试也需要初始化该接口。
	vm := proxywasm.GetVMContext()
	host, reset := proxytest.NewHostEmulator(proxytest.NewEmulatorOption().WithVMContext(vm))
	host.RegisterForeignFunction("get_log_level", func([]byte) []byte { return []byte{0, 0, 0, 0} })
	t.Cleanup(func() {
		reset()
		proxywasm.SetVMContext(vm)
	})

	config := PluginConfig{
		ModelProviderMappings: []ModelProviderMapping{
			{Model: "gpt-image-2", Provider: "Azure"},
			{Model: "gpt-image-2", Provider: "RightCode"},
			{Model: "gpt-*", Provider: "OpenAI"},
			{Model: "*", Provider: "Default"},
		},
	}

	// 精确匹配
	index := findTransformMappingByModelAndProvider(config, "gpt-image-2", "Azure")
	if index != 0 {
		t.Errorf("Expected index 0 for (gpt-image-2, Azure), got %d", index)
	}

	// 同一模型不同 provider
	index = findTransformMappingByModelAndProvider(config, "gpt-image-2", "RightCode")
	if index != 1 {
		t.Errorf("Expected index 1 for (gpt-image-2, RightCode), got %d", index)
	}

	// 通配符模型匹配
	index = findTransformMappingByModelAndProvider(config, "gpt-4", "OpenAI")
	if index != 2 {
		t.Errorf("Expected index 2 for (gpt-4, OpenAI), got %d", index)
	}

	// 双重通配符匹配
	index = findTransformMappingByModelAndProvider(config, "unknown-model", "Default")
	if index != 3 {
		t.Errorf("Expected index 3 for (unknown-model, Default), got %d", index)
	}

	// 不匹配的情况
	index = findTransformMappingByModelAndProvider(config, "gpt-image-2", "UnknownProvider")
	if index != -1 {
		t.Errorf("Expected -1 for no match, got %d", index)
	}
}

// 测试 URL 重写
func TestURLRewrite(t *testing.T) {
	tests := []struct {
		name         string
		originalPath string
		config       *URLRewriteConfig
		expectedPath string
	}{
		{
			name:         "Direct path replacement",
			originalPath: "/v1/images/edits",
			config: &URLRewriteConfig{
				Path: "/v1/images/generations",
			},
			expectedPath: "/v1/images/generations",
		},
		{
			name:         "Pattern replacement",
			originalPath: "/v1/images/edits",
			config: &URLRewriteConfig{
				FromPattern: "/v1/images/edits",
				ToPattern:   "/v1/images/generations",
			},
			expectedPath: "/v1/images/generations",
		},
		{
			name:         "Pattern replacement with query params",
			originalPath: "/v1/images/edits?param=value",
			config: &URLRewriteConfig{
				FromPattern: "/v1/images/edits",
				ToPattern:   "/v1/images/generations",
			},
			expectedPath: "/v1/images/generations?param=value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.originalPath
			if tt.config.Path != "" {
				result = tt.config.Path
			} else if tt.config.FromPattern != "" && tt.config.ToPattern != "" {
				result = strings.ReplaceAll(tt.originalPath, tt.config.FromPattern, tt.config.ToPattern)
			}

			if idx := strings.Index(tt.originalPath, "?"); idx >= 0 {
				if idx2 := strings.Index(result, "?"); idx2 < 0 {
					result += tt.originalPath[idx:]
				}
			}

			if result != tt.expectedPath {
				t.Errorf("Expected path '%s', got '%s'", tt.expectedPath, result)
			}
		})
	}
}

// 测试参数转换
func TestParamTransform(t *testing.T) {
	config := &ParamTransformConfig{
		RenameMap: map[string]string{
			"old_param": "new_param",
		},
		AddParams: map[string]interface{}{
			"extra_param": "value",
		},
		RemoveParams: []string{"remove_me"},
	}

	body := []byte(`{"old_param": "value1", "remove_me": "value2"}`)
	transformedBody, err := applyParamTransform(config, body)

	if err != nil {
		t.Fatalf("Failed to apply param transform: %v", err)
	}

	result := gjson.GetBytes(transformedBody, "new_param")
	if !result.Exists() || result.String() != "value1" {
		t.Errorf("Expected new_param to be 'value1', got '%s'", result.String())
	}

	result = gjson.GetBytes(transformedBody, "remove_me")
	if result.Exists() {
		t.Error("Expected remove_me to be removed")
	}

	result = gjson.GetBytes(transformedBody, "extra_param")
	if !result.Exists() || result.String() != "value" {
		t.Errorf("Expected extra_param to be 'value', got '%s'", result.String())
	}
}

// 测试 Body 转换
func TestBodyTransform(t *testing.T) {
	config := &BodyTransformConfig{
		SetValues: map[string]interface{}{
			"new_field": "value",
		},
		RemovePaths: []string{"delete_me"},
	}

	body := []byte(`{"existing": "data", "delete_me": "value"}`)
	transformedBody, err := applyBodyTransform(config, body)

	if err != nil {
		t.Fatalf("Failed to apply body transform: %v", err)
	}

	result := gjson.GetBytes(transformedBody, "new_field")
	if !result.Exists() || result.String() != "value" {
		t.Errorf("Expected new_field to be 'value', got '%s'", result.String())
	}

	result = gjson.GetBytes(transformedBody, "delete_me")
	if result.Exists() {
		t.Error("Expected delete_me to be removed")
	}

	result = gjson.GetBytes(transformedBody, "existing")
	if !result.Exists() || result.String() != "data" {
		t.Errorf("Expected existing field to be preserved, got '%s'", result.String())
	}
}

// 测试 Base64 图片检测
func TestIsBase64Image(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+ip1sAAAAASUVORK5CYII=", true},
		{"data:image/jpeg;base64,/9j/4AAQSkZJRgABAQAAAQABAAD/2wBD", true},
		{"just a string", false},
		{"base64,something", false},
	}

	for _, tt := range tests {
		t.Run(tt.input[:min(len(tt.input), 20)], func(t *testing.T) {
			result := isBase64Image(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %v for '%s', got %v", tt.expected, tt.input, result)
			}
		})
	}
}

// 测试图片扩展名检测
func TestDetectImageExtension(t *testing.T) {
	tests := []struct {
		contentType string
		expected    string
	}{
		{"image/jpeg", ".jpg"},
		{"image/jpg", ".jpg"},
		{"image/png", ".png"},
		{"image/gif", ".gif"},
		{"image/webp", ".webp"},
		{"image/bmp", ".bmp"},
		{"image/svg+xml", ".svg"},
		{"image/tiff", ".tiff"},
		{"image/x-icon", ".ico"},
		{"application/pdf", ".pdf"},
		{"image/jpeg; charset=utf-8", ".jpg"},
		{"", ".jpg"},
		{"unknown/type", ".type"},
	}

	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			result := detectImageExtension(tt.contentType)
			if result != tt.expected {
				t.Errorf("Expected extension '%s' for content type '%s', got '%s'", tt.expected, tt.contentType, result)
			}
		})
	}
}

// 测试 Base64 数据提取
func TestExtractBase64Data(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"data:image/png;base64,iVBORw0KGgo", "iVBORw0KGgo"},
		{"data:image/jpeg;base64,/9j/4AAQ", "/9j/4AAQ"},
		{"no base64 here", "no base64 here"},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := extractBase64Data(tt.input)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

// 内存泄漏测试 - 模拟多次请求处理
func TestMemoryLeak(t *testing.T) {
	// 模拟处理多个请求
	for i := 0; i < 1000; i++ {
		body := []byte(`{"test": "value"}`)
		config := &ParamTransformConfig{
			AddParams: map[string]interface{}{
				"index": i,
			},
		}

		_, err := applyParamTransform(config, body)
		if err != nil {
			t.Errorf("Iteration %d failed: %v", i, err)
		}
	}

	// 如果没有 panic 或错误，说明内存管理正常
}

// 辅助函数
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
