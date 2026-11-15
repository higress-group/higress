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
	"strings"

	"github.com/alibaba/higress/v2/pkg/ingress/kube/util"
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

// ProcessConfig processes a config and substitutes every template variable in it, allowing
// references to secrets in any namespace.
//
// Only call this for config sources whose references have already been restricted to the
// namespace of the object that supplied them, or that already require write access to the
// Higress control plane namespace, such as WasmPlugin CRs and the higress-config ConfigMap:
// those are operator-owned, and cross-namespace references are their intended use. Anything
// a tenant can write must be passed through RestrictTemplatesToNamespace or
// util.DefuseSpecTemplates first, otherwise a tenant could read secrets from arbitrary
// namespaces.
func (p *TemplateProcessor) ProcessConfig(cfg *config.Config) error {
	// Convert spec to JSON string to process substitutions
	jsonBytes, err := json.Marshal(cfg.Spec)
	if err != nil {
		return fmt.Errorf("failed to marshal config spec: %v", err)
	}

	configStr := string(jsonBytes)
	matches := util.TemplateRegex.FindAllStringSubmatch(configStr, -1)
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

// RestrictTemplatesToNamespace defuses every reference in values that resolves to a namespace
// other than ownerNamespace, and expands every reference that has no namespace at all to
// ownerNamespace, so that ProcessConfig can only substitute what the owning object is allowed
// to read. A refused reference is logged and replaced with util.RefusedReferencePlaceholder,
// an opaque fixed-form string that cannot be reassembled into a reference.
//
// It exists because configs generated from Ingresses are merged across namespaces (one
// VirtualService per host, one EnvoyFilter per plugin) and always carry the Higress system
// namespace, so the owning namespace cannot be recovered from the generated config. Raw
// Ingress annotations are the last place where a template and the namespace of the object
// that supplied it are both known. The Gateway API path faces the same problem and defuses at
// the equivalent point via util.DefuseSpecTemplates.
//
// The returned map is the input map unless something was defused, in which case it is a
// copy; the input map is never modified because it is shared with the informer cache. The
// second return value describes the defused references for logging.
func (p *TemplateProcessor) RestrictTemplatesToNamespace(values map[string]string, ownerNamespace string) (map[string]string, []string) {
	return util.DefuseTemplates(values, ownerNamespace)
}
