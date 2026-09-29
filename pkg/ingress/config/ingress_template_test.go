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

	"github.com/alibaba/higress/v2/pkg/ingress/kube/util"
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
			expected: &extensions.WasmPlugin{
                PluginName: "test-plugin",
                PluginConfig: makeStructValue(t, map[string]interface{}{
                    "api_key": "${secret.default/non-existent.api_key}",
                }),
            },
			expectError: false,
		},
        {
            name: "config with multiple fields and non-existent secret",
            wasmPlugin: &extensions.WasmPlugin{
                PluginName: "test-plugin",
                PluginConfig: makeStructValue(t, map[string]interface{}{
                    "api_key_non_existent": "${secret.default/non-existent.api_key}",
                    "api_key":              "${secret.default/test-secret.api_key}",
                }),
            },
            expected: &extensions.WasmPlugin{
                PluginName: "test-plugin",
                PluginConfig: makeStructValue(t, map[string]interface{}{
                    "api_key_non_existent": "${secret.default/non-existent.api_key}",
                    "api_key":              "test-api-key",
                }),
            },
            expectError: false,
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

// newRecordingSecretGetter returns a getValue implementation backed by values, whose keys
// are of the form type.namespace/name.key, together with the list of references it was
// asked to resolve. The list lets a test prove that a refused reference never reached the
// secret store at all.
func newRecordingSecretGetter(values map[string]string) (func(valueType, namespace, name, key string) (string, error), *[]string) {
	var calls []string
	getValue := func(valueType, namespace, name, key string) (string, error) {
		fullKey := fmt.Sprintf("%s.%s/%s.%s", valueType, namespace, name, key)
		calls = append(calls, fullKey)
		if value, exists := values[fullKey]; exists {
			return value, nil
		}
		return "", fmt.Errorf("value not found for %s", fullKey)
	}
	return getValue, &calls
}

// tenantSecrets holds a value for every namespace used below. The foreign ones must never
// be handed out to a config owned by tenant-a.
var tenantSecrets = map[string]string{
	"secret.tenant-a/app.api_key":        "tenant-a-key",
	"secret.tenant-b/app.api_key":        "tenant-b-key",
	"secret.higress-system/shared.token": "system-token",
}

func pluginWithConfig(t *testing.T, namespace string, fields map[string]interface{}) *config.Config {
	return &config.Config{
		Meta: config.Meta{
			GroupVersionKind: gvk.WasmPlugin,
			Name:             "test-plugin",
			Namespace:        namespace,
		},
		Spec: &extensions.WasmPlugin{
			PluginName:   "test-plugin",
			PluginConfig: makeStructValue(t, fields),
		},
	}
}

func assertConfigValue(t *testing.T, cfg *config.Config, key, expected string) {
	t.Helper()
	plugin := cfg.Spec.(*extensions.WasmPlugin)
	assert.Equal(t, expected, plugin.PluginConfig.Fields[key].GetStringValue())
}

func TestTemplateProcessor_RestrictTemplatesToNamespace(t *testing.T) {
	processor := NewTemplateProcessor(nil, "higress-system", nil)

	t.Run("annotations without references are returned untouched", func(t *testing.T) {
		annotations := map[string]string{
			"higress.io/destination":                     "foo.default.svc.cluster.local",
			"nginx.ingress.kubernetes.io/rewrite-target": "/",
		}
		sanitized, refused := processor.RestrictTemplatesToNamespace(annotations, "tenant-a")
		assert.Empty(t, refused)
		assert.Equal(t, annotations, sanitized)

		// Nothing was defused, so no copy is made: every Ingress pays for this call.
		sanitized["probe"] = "x"
		assert.Equal(t, "x", annotations["probe"])
		delete(annotations, "probe")
	})

	t.Run("same namespace reference is preserved byte identically", func(t *testing.T) {
		annotations := map[string]string{
			"higress.io/request-header": "Bearer ${secret.tenant-a/app.api_key}",
		}
		sanitized, refused := processor.RestrictTemplatesToNamespace(annotations, "tenant-a")
		assert.Empty(t, refused)
		assert.Equal(t, "Bearer ${secret.tenant-a/app.api_key}", sanitized["higress.io/request-header"])
	})

	t.Run("cross namespace reference is replaced with an opaque placeholder", func(t *testing.T) {
		annotations := map[string]string{
			"higress.io/request-header": "Bearer ${secret.tenant-b/app.api_key}",
			"higress.io/destination":    "foo.default.svc.cluster.local",
		}
		sanitized, refused := processor.RestrictTemplatesToNamespace(annotations, "tenant-a")
		assert.Equal(t, []string{"higress.io/request-header=${secret.tenant-b/app.api_key}"}, refused)
		assert.Equal(t, "Bearer "+util.RefusedReferencePlaceholder, sanitized["higress.io/request-header"])
		assert.Equal(t, "foo.default.svc.cluster.local", sanitized["higress.io/destination"])
		// The caller's map is shared with the informer cache and must not be modified.
		assert.Equal(t, "Bearer ${secret.tenant-b/app.api_key}", annotations["higress.io/request-header"])
	})

	t.Run("short form reference is expanded to the owner namespace", func(t *testing.T) {
		// The ${secret.name.key} form means the namespace of the object that supplied it, so
		// it is allowed rather than refused. It has to be expanded here: the annotation ends
		// up in a config stamped with the Higress system namespace, which is where an
		// unexpanded short form would be resolved instead.
		annotations := map[string]string{"higress.io/request-header": "Bearer ${secret.shared.token}"}
		sanitized, refused := processor.RestrictTemplatesToNamespace(annotations, "tenant-a")
		assert.Empty(t, refused)
		assert.Equal(t, "Bearer ${secret.tenant-a/shared.token}", sanitized["higress.io/request-header"])
	})

	t.Run("an unknown owner namespace refuses every reference", func(t *testing.T) {
		// Without an owner there is nothing to compare a reference against, so failing open
		// would hand every namespace's secrets to whoever wrote the value.
		for _, tt := range []struct{ value, ref string }{
			{"${secret.tenant-a/app.api_key}", "secret.tenant-a/app.api_key"},
			{"${secret.shared.token}", "secret.shared.token"},
		} {
			annotations := map[string]string{"higress.io/request-header": tt.value}
			sanitized, refused := processor.RestrictTemplatesToNamespace(annotations, "")
			assert.Equal(t, []string{"higress.io/request-header=${" + tt.ref + "}"}, refused)
			assert.Equal(t, util.RefusedReferencePlaceholder, sanitized["higress.io/request-header"])
		}
	})

	t.Run("defused references can never be resolved", func(t *testing.T) {
		// Values a tenant could surround a reference with in an attempt to have the
		// placeholder reassembled into a resolvable reference.
		for _, value := range []string{
			"${secret.tenant-b/app.api_key}",
			"$$${secret.tenant-b/app.api_key}",
			"prefix-${secret.tenant-b/app.api_key}-${secret.higress-system/shared.token}-suffix",
			// The name and key parts of a reference may themselves contain '$' and '{'.
			"${secret.tenant-b/a${secret.higress-system/shared.token}",
			"${secret.${x}/app.api_key}",
		} {
			annotations := map[string]string{"higress.io/request-header": value}
			sanitized, _ := processor.RestrictTemplatesToNamespace(annotations, "tenant-a")
			defused := sanitized["higress.io/request-header"]

			// Run the defused value through the unrestricted processor: if anything is
			// still shaped like a reference it would be substituted here.
			getValue, calls := newRecordingSecretGetter(tenantSecrets)
			unrestricted := NewTemplateProcessor(getValue, "higress-system", nil)
			cfg := pluginWithConfig(t, "higress-system", map[string]interface{}{"v": defused})
			assert.NoError(t, unrestricted.ProcessConfig(cfg))
			assert.Empty(t, *calls, "defused value %q must not reach the secret store", value)
			assertConfigValue(t, cfg, "v", defused)
			assert.NotContains(t, defused, "tenant-b-key")
			assert.NotContains(t, defused, "system-token")
			// The placeholder is fixed-form, so it never carries the refused reference.
			assert.NotContains(t, defused, "tenant-b")
			assert.Empty(t, util.TemplateRegex.FindAllString(defused, -1),
				"defused value %q still matches as a reference: %q", value, defused)
		}
	})
}

// TestTemplateProcessor_TenantAnnotationEndToEnd proves the two halves of the tenant path
// agree: once RestrictTemplatesToNamespace has rewritten an Ingress annotation, the
// unrestricted ProcessConfig that later runs over the generated config can only read what
// the Ingress's own namespace owns.
func TestTemplateProcessor_TenantAnnotationEndToEnd(t *testing.T) {
	getValue, calls := newRecordingSecretGetter(map[string]string{
		"secret.tenant-a/shared.token":       "tenant-a-token",
		"secret.higress-system/shared.token": "system-token",
		"secret.tenant-b/app.api_key":        "tenant-b-key",
	})
	restricting := NewTemplateProcessor(nil, "higress-system", nil)
	unrestricted := NewTemplateProcessor(getValue, "higress-system", nil)

	annotations := map[string]string{
		"higress.io/short-form": "${secret.shared.token}",
		"higress.io/explicit":   "${secret.tenant-a/shared.token}",
		"higress.io/foreign":    "${secret.tenant-b/app.api_key}",
	}
	sanitized, refused := restricting.RestrictTemplatesToNamespace(annotations, "tenant-a")
	assert.Equal(t, []string{"higress.io/foreign=${secret.tenant-b/app.api_key}"}, refused)

	// The generated config carries the Higress system namespace, which is exactly why the
	// annotation had to be rewritten while the Ingress's namespace was still known.
	cfg := pluginWithConfig(t, "higress-system", map[string]interface{}{
		"short-form": sanitized["higress.io/short-form"],
		"explicit":   sanitized["higress.io/explicit"],
		"foreign":    sanitized["higress.io/foreign"],
	})
	assert.NoError(t, unrestricted.ProcessConfig(cfg))

	// Both tenant-a references resolve, the short form included.
	assertConfigValue(t, cfg, "short-form", "tenant-a-token")
	assertConfigValue(t, cfg, "explicit", "tenant-a-token")
	// The foreign one is gone rather than resolved.
	assertConfigValue(t, cfg, "foreign", util.RefusedReferencePlaceholder)
	assert.NotContains(t, *calls, "secret.tenant-b/app.api_key")
	// The short form must not be resolved in the Higress system namespace, which is what it
	// meant before ownership was enforced.
	assert.NotContains(t, *calls, "secret.higress-system/shared.token")
	assert.ElementsMatch(t,
		[]string{"secret.tenant-a/shared.token", "secret.tenant-a/shared.token"}, *calls)
}

// TestTemplateProcessor_ProcessConfigAllowsCrossNamespace pins the operator-owned path:
// ProcessConfig stays unrestricted so that WasmPlugin CRs and the higress-config ConfigMap
// can keep referencing secrets in other namespaces, as the basic-auth template e2e does.
func TestTemplateProcessor_ProcessConfigAllowsCrossNamespace(t *testing.T) {
	getValue, calls := newRecordingSecretGetter(tenantSecrets)
	processor := NewTemplateProcessor(getValue, "higress-system", nil)

	cfg := pluginWithConfig(t, "higress-system", map[string]interface{}{
		"api_key": "${secret.tenant-b/app.api_key}",
		"token":   "${secret.shared.token}",
	})
	assert.NoError(t, processor.ProcessConfig(cfg))
	assertConfigValue(t, cfg, "api_key", "tenant-b-key")
	assertConfigValue(t, cfg, "token", "system-token")
	assert.ElementsMatch(t, []string{"secret.tenant-b/app.api_key", "secret.higress-system/shared.token"}, *calls)
}

// Helper function to create structpb.Struct from map
func makeStructValue(t *testing.T, m map[string]interface{}) *structpb.Struct {
	s, err := structpb.NewStruct(m)
	assert.NoError(t, err, "Failed to create struct value")
	return s
}
