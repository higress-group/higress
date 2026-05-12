// Copyright (c) 2022 Alibaba Group Holding Ltd.
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
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/structpb"
	extensions "istio.io/api/extensions/v1alpha1"
	"istio.io/istio/pkg/config"
	"istio.io/istio/pkg/config/schema/gvk"
)

func TestTemplateProcessor_ProcessConfig(t *testing.T) {
	// Create test values map
	values := map[string]string{
		"secret.default/test-secret.api_key":                        "test-api-key",
		"secret.default/test-secret.plugin_conf.timeout":            "5000",
		"secret.default/test-secret.plugin_conf.max_retries":        "3",
		"secret.higress-system/auth-secret.auth_config.type":        "basic",
		"secret.higress-system/auth-secret.auth_config.credentials": "base64-encoded",
	}

	// Mock value getter function
	getValue := func(valueType, namespace, name, key string) (string, error) {
		fullKey := fmt.Sprintf("%s.%s/%s.%s", valueType, namespace, name, key)
		fmt.Printf("Getting value for %s", fullKey)
		if value, exists := values[fullKey]; exists {
			return value, nil
		}
		return "", fmt.Errorf("value not found for %s", fullKey)
	}

	// Create template processor
	processor := NewTemplateProcessor(getValue, "higress-system", nil)

	tests := []struct {
		name        string
		wasmPlugin  *extensions.WasmPlugin
		expected    *extensions.WasmPlugin
		expectError bool
	}{
		{
			name: "simple api key reference",
			wasmPlugin: &extensions.WasmPlugin{
				PluginName: "test-plugin",
				PluginConfig: makeStructValue(t, map[string]interface{}{
					"api_key": "${secret.default/test-secret.api_key}",
				}),
			},
			expected: &extensions.WasmPlugin{
				PluginName: "test-plugin",
				PluginConfig: makeStructValue(t, map[string]interface{}{
					"api_key": "test-api-key",
				}),
			},
			expectError: false,
		},
		{
			name: "config with multiple fields",
			wasmPlugin: &extensions.WasmPlugin{
				PluginName: "test-plugin",
				PluginConfig: makeStructValue(t, map[string]interface{}{
					"config": map[string]interface{}{
						"timeout":     "${secret.default/test-secret.plugin_conf.timeout}",
						"max_retries": "${secret.default/test-secret.plugin_conf.max_retries}",
					},
				}),
			},
			expected: &extensions.WasmPlugin{
				PluginName: "test-plugin",
				PluginConfig: makeStructValue(t, map[string]interface{}{
					"config": map[string]interface{}{
						"timeout":     "5000",
						"max_retries": "3",
					},
				}),
			},
			expectError: false,
		},
		{
			name: "auth config with default namespace",
			wasmPlugin: &extensions.WasmPlugin{
				PluginName: "test-plugin",
				PluginConfig: makeStructValue(t, map[string]interface{}{
					"auth": map[string]interface{}{
						"type":        "${secret.auth-secret.auth_config.type}",
						"credentials": "${secret.auth-secret.auth_config.credentials}",
					},
				}),
			},
			expected: &extensions.WasmPlugin{
				PluginName: "test-plugin",
				PluginConfig: makeStructValue(t, map[string]interface{}{
					"auth": map[string]interface{}{
						"type":        "basic",
						"credentials": "base64-encoded",
					},
				}),
			},
			expectError: false,
		},
		{
			name: "config with default and non-default namespaces (default first)",
			wasmPlugin: &extensions.WasmPlugin{
				PluginName: "test-plugin",
				PluginConfig: makeStructValue(t, map[string]interface{}{
					"a1": map[string]interface{}{
						"type":        "${secret.auth-secret.auth_config.type}",
						"credentials": "${secret.auth-secret.auth_config.credentials}",
					},
					"a2": map[string]interface{}{
						"timeout":     "${secret.default/test-secret.plugin_conf.timeout}",
						"max_retries": "${secret.default/test-secret.plugin_conf.max_retries}",
					},
				}),
			},
			expected: &extensions.WasmPlugin{
				PluginName: "test-plugin",
				PluginConfig: makeStructValue(t, map[string]interface{}{
					"a1": map[string]interface{}{
						"type":        "basic",
						"credentials": "base64-encoded",
					},
					"a2": map[string]interface{}{
						"timeout":     "5000",
						"max_retries": "3",
					},
				}),
			},
			expectError: false,
		},
		{
			name: "config with default and non-default namespaces (non-default first)",
			wasmPlugin: &extensions.WasmPlugin{
				PluginName: "test-plugin",
				PluginConfig: makeStructValue(t, map[string]interface{}{
					"a1": map[string]interface{}{
						"timeout":     "${secret.default/test-secret.plugin_conf.timeout}",
						"max_retries": "${secret.default/test-secret.plugin_conf.max_retries}",
					},
					"a2": map[string]interface{}{
						"type":        "${secret.auth-secret.auth_config.type}",
						"credentials": "${secret.auth-secret.auth_config.credentials}",
					},
				}),
			},
			expected: &extensions.WasmPlugin{
				PluginName: "test-plugin",
				PluginConfig: makeStructValue(t, map[string]interface{}{
					"a1": map[string]interface{}{
						"timeout":     "5000",
						"max_retries": "3",
					},
					"a2": map[string]interface{}{
						"type":        "basic",
						"credentials": "base64-encoded",
					},
				}),
			},
			expectError: false,
		},
		{
			name: "non-existent secret",
			wasmPlugin: &extensions.WasmPlugin{
				PluginName: "test-plugin",
				PluginConfig: makeStructValue(t, map[string]interface{}{
					"api_key": "${secret.default/non-existent.api_key}",
				}),
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Meta: config.Meta{
					GroupVersionKind: gvk.WasmPlugin,
					Name:             "test-plugin",
					Namespace:        "default",
				},
				Spec: tt.wasmPlugin,
			}

			err := processor.ProcessConfig(cfg)
			if tt.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			processedPlugin := cfg.Spec.(*extensions.WasmPlugin)

			// Compare plugin name
			assert.Equal(t, tt.expected.PluginName, processedPlugin.PluginName)

			// Compare plugin configs
			if tt.expected.PluginConfig != nil {
				assert.NotNil(t, processedPlugin.PluginConfig)
				assert.Equal(t, tt.expected.PluginConfig.AsMap(), processedPlugin.PluginConfig.AsMap())
			}
		})
	}
}

// Helper function to create structpb.Struct from map
func makeStructValue(t *testing.T, m map[string]interface{}) *structpb.Struct {
	s, err := structpb.NewStruct(m)
	assert.NoError(t, err, "Failed to create struct value")
	return s
}
