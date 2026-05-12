// Copyright (c) 2022 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package validator

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/tidwall/gjson"
	"gopkg.in/yaml.v3"

	"github.com/higress-group/wasm-go/pkg/log"
	"github.com/alibaba/higress/plugins/wasm-go/pkg/mcp/server"
)

// validatorLogger is a simple logger implementation for validation mode
type validatorLogger struct{}

func (l *validatorLogger) Trace(msg string) { fmt.Fprintf(os.Stderr, "[TRACE] %s\n", msg) }
func (l *validatorLogger) Tracef(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[TRACE] "+format+"\n", args...)
}
func (l *validatorLogger) Debug(msg string) { fmt.Fprintf(os.Stderr, "[DEBUG] %s\n", msg) }
func (l *validatorLogger) Debugf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[DEBUG] "+format+"\n", args...)
}
func (l *validatorLogger) Info(msg string) { fmt.Fprintf(os.Stderr, "[INFO] %s\n", msg) }
func (l *validatorLogger) Infof(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[INFO] "+format+"\n", args...)
}
func (l *validatorLogger) Warn(msg string) { fmt.Fprintf(os.Stderr, "[WARN] %s\n", msg) }
func (l *validatorLogger) Warnf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[WARN] "+format+"\n", args...)
}
func (l *validatorLogger) Error(msg string) { fmt.Fprintf(os.Stderr, "[ERROR] %s\n", msg) }
func (l *validatorLogger) Errorf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[ERROR] "+format+"\n", args...)
}
func (l *validatorLogger) Critical(msg string) { fmt.Fprintf(os.Stderr, "[CRITICAL] %s\n", msg) }
func (l *validatorLogger) Criticalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[CRITICAL] "+format+"\n", args...)
}
func (l *validatorLogger) ResetID(pluginID string) {}

// init initializes the validator package
func init() {
	// Set a custom logger for validation mode to prevent panics
	log.SetPluginLog(&validatorLogger{})
}

// ValidationResult contains the result of configuration validation
type ValidationResult struct {
	IsValid    bool   `json:"isValid"`
	Error      error  `json:"error,omitempty"`
	ServerName string `json:"serverName,omitempty"`
	IsComposed bool   `json:"isComposed"`
}

// ValidateConfig validates MCP configuration
// This function focuses on validating REST tools and toolSet configurations
// It skips validation for pre-registered Go-based servers
func ValidateConfig(configJSON string) (*ValidationResult, error) {
	// Create empty dependencies for validation mode
	// We skip pre-registered servers validation by setting SkipPreRegisteredServers to true
	toolRegistry := &server.GlobalToolRegistry{}
	toolRegistry.Initialize() // Initialize the registry to prevent nil map assignment panic

	deps := &server.ConfigOptions{
		Servers:                  make(map[string]server.Server), // Empty servers map
		ToolRegistry:             toolRegistry,                   // Initialized registry
		SkipPreRegisteredServers: true,                           // Skip pre-registered servers
	}

	// Call core parsing logic for validation
	configGjson := gjson.Parse(configJSON)
	mockConfig := &server.McpServerConfig{}

	err := server.ParseConfigCore(configGjson, mockConfig, deps)

	result := &ValidationResult{
		IsValid: err == nil,
		Error:   err,
	}

	if err == nil {
		result.ServerName = mockConfig.GetServerName()
		result.IsComposed = mockConfig.GetIsComposed()
	}

	return result, nil
}

// ValidateConfigYAML validates MCP configuration from YAML format
// This function converts YAML to JSON first, then validates using the same logic
func ValidateConfigYAML(configYAML string) (*ValidationResult, error) {
	// Parse YAML into a generic interface
	var yamlData interface{}
	if err := yaml.Unmarshal([]byte(configYAML), &yamlData); err != nil {
		return &ValidationResult{
			IsValid: false,
			Error:   fmt.Errorf("failed to parse YAML: %v", err),
		}, nil
	}

	// Convert to JSON
	jsonBytes, err := json.Marshal(yamlData)
	if err != nil {
		return &ValidationResult{
			IsValid: false,
			Error:   fmt.Errorf("failed to convert YAML to JSON: %v", err),
		}, nil
	}

	// Use the existing JSON validation logic
	return ValidateConfig(string(jsonBytes))
}
