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
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	. "github.com/alibaba/higress/v2/pkg/ingress/log"
	"google.golang.org/protobuf/proto"
	"istio.io/istio/pkg/config"
)

// TemplateProcessor handles template substitution in configs
type TemplateProcessor struct {
	// getValue is a function that retrieves values by type, namespace, name and key
	getValue        func(valueType, namespace, name, key string) (string, error)
	namespace       string
	secretConfigMgr *SecretConfigMgr
}

// NewTemplateProcessor creates a new TemplateProcessor with the given value getter function
func NewTemplateProcessor(getValue func(valueType, namespace, name, key string) (string, error), namespace string, secretConfigMgr *SecretConfigMgr) *TemplateProcessor {
	return &TemplateProcessor{
		getValue:        getValue,
		namespace:       namespace,
		secretConfigMgr: secretConfigMgr,
	}
}

// ProcessConfig processes a config and substitutes any template variables
func (p *TemplateProcessor) ProcessConfig(cfg *config.Config) error {
	// Convert spec to JSON string to process substitutions
	jsonBytes, err := json.Marshal(cfg.Spec)
	if err != nil {
		return fmt.Errorf("failed to marshal config spec: %v", err)
	}

	configStr := string(jsonBytes)
	// Find all value references in format:
	// ${type.name.key} or ${type.namespace/name.key}
	valueRegex := regexp.MustCompile(`\$\{([^.}/]+)\.(?:([^/}]+)/)?([^.}/]+)\.([^}]+)\}`)
	matches := valueRegex.FindAllStringSubmatch(configStr, -1)
	// If there are no value references, return immediately
	if len(matches) == 0 {
		if p.secretConfigMgr != nil {
			if err := p.secretConfigMgr.DeleteConfig(cfg); err != nil {
				IngressLog.Errorf("failed to delete secret dependency: %v", err)
			}
		}
		return nil
	}

	foundSecretSource := false
	IngressLog.Infof("start to apply config %s/%s with %d variables", cfg.Namespace, cfg.Name, len(matches))
	for _, match := range matches {
		valueType := match[1]
		var namespace, name, key string
		if match[2] != "" {
			// Format: ${type.namespace/name.key}
			namespace = match[2]
		} else {
			// Format: ${type.name.key} - use default namespace
			namespace = p.namespace
		}
		name = match[3]
		key = match[4]

		// Get value using the provided getter function
		value, err := p.getValue(valueType, namespace, name, key)
		if err != nil {
			return fmt.Errorf("failed to get %s value for %s/%s.%s: %v", valueType, namespace, name, key, err)
		}

		// Add secret dependency if this is a secret reference
		if valueType == "secret" && p.secretConfigMgr != nil {
			foundSecretSource = true
			secretKey := fmt.Sprintf("%s/%s", namespace, name)
			if err := p.secretConfigMgr.AddConfig(secretKey, cfg); err != nil {
				IngressLog.Errorf("failed to add secret dependency: %v", err)
			}
		}
		// Replace placeholder with actual value
		configStr = strings.Replace(configStr, match[0], value, 1)
	}

	// Create a new instance of the same type as cfg.Spec
	newSpec := proto.Clone(cfg.Spec.(proto.Message))
	if err := json.Unmarshal([]byte(configStr), newSpec); err != nil {
		return fmt.Errorf("failed to unmarshal substituted config: %v", err)
	}
	cfg.Spec = newSpec

	// Delete secret dependency if no secret reference is found
	if !foundSecretSource {
		if p.secretConfigMgr != nil {
			if err := p.secretConfigMgr.DeleteConfig(cfg); err != nil {
				IngressLog.Errorf("failed to delete secret dependency: %v", err)
			}
		}
	}

	IngressLog.Infof("end to process config %s/%s", cfg.Namespace, cfg.Name)
	return nil
}
