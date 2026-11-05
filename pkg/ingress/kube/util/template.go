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

package util

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"google.golang.org/protobuf/proto"

	. "github.com/alibaba/higress/v2/pkg/ingress/log"
)

// TemplateRegex matches value references of the form ${type.name.key} and
// ${type.namespace/name.key}. Everything that produces or resolves a reference uses it, so
// that the two can never disagree about what counts as a reference.
var TemplateRegex = regexp.MustCompile(`\$\{([^.}/]+)\.(?:([^/}]+)/)?([^.}/]+)\.([^}]+)\}`)

// RefusedReferencePlaceholder replaces a reference that the object which supplied it is not
// allowed to resolve. It is fixed-form on purpose: it contains neither '$' nor '{' nor any
// part of the refused reference, so it cannot be turned back into a reference by itself or
// by the text surrounding it. What was refused is reported separately for logging.
const RefusedReferencePlaceholder = "secret-reference-refused"

// DefuseTemplates rewrites every reference in values that the object in ownerNamespace is
// not allowed to resolve, so that substitution can no longer pick it up. It exists because
// generated configs are merged across namespaces and re-stamped with a namespace of their
// own, after which the namespace of the object that supplied a value can no longer be
// recovered: raw annotation values are the last place where a reference and its owner are
// both known.
//
// The returned map is the input map unless something was rewritten, in which case it is a
// copy; the input map is never modified because it is shared with the informer cache. The
// second return value describes the refused references for logging.
func DefuseTemplates(values map[string]string, ownerNamespace string) (map[string]string, []string) {
	var refused []string
	sanitized := values
	copied := false
	for field, value := range values {
		replaced, fieldRefused := defuseString(value, ownerNamespace, field)
		refused = append(refused, fieldRefused...)
		if replaced == value {
			continue
		}
		if !copied {
			sanitized = make(map[string]string, len(values))
			for k, v := range values {
				sanitized[k] = v
			}
			copied = true
		}
		sanitized[field] = replaced
	}
	return sanitized, refused
}

// DefuseSpecTemplates is DefuseTemplates for a generated protobuf spec: it rewrites the
// references in the JSON serialization of spec and writes the result back into spec. owner
// identifies the object that produced spec in log messages and errors.
//
// Call it where the spec is built, while its namespace is still the namespace of the object
// it was generated from.
//
// A spec that cannot be round-tripped cannot be inspected, and an uninspected spec would
// resolve references against whatever namespace the generated config happens to carry, so
// the round-trip failure is returned instead of logged: the caller must abort generation of
// the config that carried it and discard spec, which may be partly rewritten. Failing open
// here would defeat the whole check for exactly the specs it is least able to read.
//
// Rewriting deserializes into spec, which replaces the sub-messages it reaches rather than
// mutating them, so a spec assembled from values that are already in a cache can be defused
// in place without touching the cache. The backend policy merge relies on that and
// TestDefuseSpecTemplatesLeavesCachedValuesAlone pins it.
func DefuseSpecTemplates(spec any, ownerNamespace, owner string) ([]string, error) {
	message, ok := spec.(proto.Message)
	if !ok || message == nil {
		err := fmt.Errorf("%s: spec of type %T cannot be checked for secret references", owner, spec)
		IngressLog.Errorf("%v, dropping the config it generated", err)
		return nil, err
	}
	jsonBytes, err := json.Marshal(message)
	if err != nil {
		err = fmt.Errorf("%s: failed to marshal spec for the secret reference ownership check: %w", owner, err)
		IngressLog.Errorf("%v, dropping the config it generated", err)
		return nil, err
	}
	defused, refused := defuseString(string(jsonBytes), ownerNamespace, owner)
	if len(refused) > 0 {
		IngressLog.Errorf("%s references secrets outside its own namespace, replacing them with %q: %v",
			owner, RefusedReferencePlaceholder, refused)
	}
	if defused == string(jsonBytes) {
		return refused, nil
	}
	if err := json.Unmarshal([]byte(defused), message); err != nil {
		err = fmt.Errorf("%s: failed to unmarshal spec after the secret reference ownership check: %w", owner, err)
		IngressLog.Errorf("%v, dropping the config it generated", err)
		return refused, err
	}
	return refused, nil
}

func defuseString(value, ownerNamespace, field string) (string, []string) {
	if !strings.Contains(value, "${") {
		return value, nil
	}
	var refused []string
	defused := TemplateRegex.ReplaceAllStringFunc(value, func(match string) string {
		replacement, ref, allowed := rewriteReference(match, ownerNamespace)
		if !allowed {
			refused = append(refused, fmt.Sprintf("%s=${%s}", field, ref))
		}
		return replacement
	})
	return defused, refused
}

// rewriteReference returns what a single reference has to be replaced with, a human readable
// form of it for logging, and whether the object in ownerNamespace may resolve it.
//
// A reference without a namespace means "the namespace of the object that supplied it", so
// it is expanded to that namespace. Expanding rather than leaving it alone is what makes the
// short form safe: the value is merged into configs stamped with another namespace, and an
// unexpanded short form would be resolved against the Higress system namespace instead.
func rewriteReference(match, ownerNamespace string) (replacement, ref string, allowed bool) {
	sub := TemplateRegex.FindStringSubmatch(match)
	valueType, namespace, name, key := sub[1], sub[2], sub[3], sub[4]
	if namespace == "" {
		ref = fmt.Sprintf("%s.%s.%s", valueType, name, key)
	} else {
		ref = fmt.Sprintf("%s.%s/%s.%s", valueType, namespace, name, key)
	}
	// An empty owner namespace means the caller cannot say who supplied the value, so nothing
	// may be resolved: failing open here would hand every namespace's secrets to whoever
	// wrote the value.
	if ownerNamespace == "" {
		return RefusedReferencePlaceholder, ref, false
	}
	switch {
	case namespace == "":
		return fmt.Sprintf("${%s.%s/%s.%s}", valueType, ownerNamespace, name, key), ref, true
	case namespace != ownerNamespace:
		return RefusedReferencePlaceholder, ref, false
	default:
		return match, ref, true
	}
}
