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

package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
)

// JSONObject preserves an extensible JSON object without weakening the known
// ClientCapabilities fields to arbitrary JSON values.
type JSONObject map[string]json.RawMessage

type RootCapabilities struct{}

type SamplingCapabilities struct {
	Context *JSONObject `json:"context,omitempty"`
	Tools   *JSONObject `json:"tools,omitempty"`
}

type ElicitationCapabilities struct {
	Form *JSONObject `json:"form,omitempty"`
	URL  *JSONObject `json:"url,omitempty"`
}

// ClientCapabilities mirrors the 2026-07-28 schema while retaining additional
// top-level capability objects allowed by the open protocol contract.
type ClientCapabilities struct {
	Experimental map[string]JSONObject    `json:"experimental,omitempty"`
	Roots        *RootCapabilities        `json:"roots,omitempty"`
	Sampling     *SamplingCapabilities    `json:"sampling,omitempty"`
	Elicitation  *ElicitationCapabilities `json:"elicitation,omitempty"`
	Extensions   map[string]JSONObject    `json:"extensions,omitempty"`
	Additional   map[string]JSONObject    `json:"-"`
}

var extensionCapabilityName = regexp.MustCompile(`^[A-Za-z](?:[A-Za-z0-9-]*[A-Za-z0-9])?(?:\.[A-Za-z](?:[A-Za-z0-9-]*[A-Za-z0-9])?)*\/(?:[A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?)?$`)

func (c *ClientCapabilities) UnmarshalJSON(data []byte) error {
	members, err := decodeJSONObject(data)
	if err != nil {
		return errors.New("client capabilities must be an object")
	}
	decoded := ClientCapabilities{}
	for name, raw := range members {
		switch name {
		case "experimental":
			decoded.Experimental, err = decodeJSONObjectMap(raw, nil)
		case "roots":
			var roots JSONObject
			roots, err = decodeJSONObject(raw)
			if err == nil && len(roots) != 0 {
				err = errors.New("roots capability must be an empty object")
			}
			if err == nil {
				decoded.Roots = &RootCapabilities{}
			}
		case "sampling":
			decoded.Sampling, err = decodeSamplingCapabilities(raw)
		case "elicitation":
			decoded.Elicitation, err = decodeElicitationCapabilities(raw)
		case "extensions":
			decoded.Extensions, err = decodeJSONObjectMap(raw, extensionCapabilityName.MatchString)
		default:
			if name == "" {
				err = errors.New("capability name must not be empty")
				break
			}
			var additional JSONObject
			additional, err = decodeJSONObject(raw)
			if err == nil {
				if decoded.Additional == nil {
					decoded.Additional = make(map[string]JSONObject)
				}
				decoded.Additional[name] = additional
			}
		}
		if err != nil {
			return fmt.Errorf("invalid client capability %q: %w", name, err)
		}
	}
	*c = decoded
	return nil
}

func (c ClientCapabilities) MarshalJSON() ([]byte, error) {
	members := make(map[string]any, len(c.Additional)+5)
	for name, capability := range c.Additional {
		members[name] = capability
	}
	if c.Experimental != nil {
		members["experimental"] = c.Experimental
	}
	if c.Roots != nil {
		members["roots"] = c.Roots
	}
	if c.Sampling != nil {
		members["sampling"] = c.Sampling
	}
	if c.Elicitation != nil {
		members["elicitation"] = c.Elicitation
	}
	if c.Extensions != nil {
		members["extensions"] = c.Extensions
	}
	return json.Marshal(members)
}

func decodeSamplingCapabilities(raw json.RawMessage) (*SamplingCapabilities, error) {
	members, err := decodeJSONObject(raw)
	if err != nil {
		return nil, errors.New("sampling capability must be an object")
	}
	capability := &SamplingCapabilities{}
	for name, value := range members {
		object, objectErr := decodeJSONObject(value)
		if objectErr != nil {
			return nil, fmt.Errorf("sampling.%s must be an object", name)
		}
		switch name {
		case "context":
			capability.Context = &object
		case "tools":
			capability.Tools = &object
		default:
			return nil, fmt.Errorf("unknown sampling capability %q", name)
		}
	}
	return capability, nil
}

func decodeElicitationCapabilities(raw json.RawMessage) (*ElicitationCapabilities, error) {
	members, err := decodeJSONObject(raw)
	if err != nil {
		return nil, errors.New("elicitation capability must be an object")
	}
	capability := &ElicitationCapabilities{}
	for name, value := range members {
		object, objectErr := decodeJSONObject(value)
		if objectErr != nil {
			return nil, fmt.Errorf("elicitation.%s must be an object", name)
		}
		switch name {
		case "form":
			capability.Form = &object
		case "url":
			capability.URL = &object
		default:
			return nil, fmt.Errorf("unknown elicitation capability %q", name)
		}
	}
	return capability, nil
}

func decodeJSONObjectMap(raw json.RawMessage, validName func(string) bool) (map[string]JSONObject, error) {
	members, err := decodeJSONObject(raw)
	if err != nil {
		return nil, errors.New("capability collection must be an object")
	}
	result := make(map[string]JSONObject, len(members))
	for name, value := range members {
		if name == "" || (validName != nil && !validName(name)) {
			return nil, fmt.Errorf("invalid capability name %q", name)
		}
		object, objectErr := decodeJSONObject(value)
		if objectErr != nil {
			return nil, fmt.Errorf("capability %q settings must be an object", name)
		}
		result[name] = object
	}
	return result, nil
}

func decodeJSONObject(raw json.RawMessage) (JSONObject, error) {
	if !isJSONObject(raw) {
		return nil, errors.New("value must be a non-null object")
	}
	var object JSONObject
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return nil, errors.New("value must be a non-null object")
	}
	return object, nil
}

func (c ClientCapabilities) isEmpty() bool {
	return c.Roots == nil && c.Sampling == nil && c.Elicitation == nil &&
		len(c.Experimental) == 0 && len(c.Extensions) == 0 && len(c.Additional) == 0
}

func cloneClientCapabilities(capabilities ClientCapabilities) ClientCapabilities {
	raw, _ := json.Marshal(capabilities)
	var cloned ClientCapabilities
	_ = json.Unmarshal(raw, &cloned)
	return cloned
}

// missingClientCapabilities returns the structured subset of requirements not
// present in actual. Empty requirement objects express presence; deeper object
// members express nested requirements such as sampling.tools.
func missingClientCapabilities(actual, required ClientCapabilities) ClientCapabilities {
	actualRaw, _ := json.Marshal(actual)
	requiredRaw, _ := json.Marshal(required)
	actualObject, _ := decodeJSONObject(actualRaw)
	requiredObject, _ := decodeJSONObject(requiredRaw)
	missingObject := missingJSONObjectMembers(actualObject, requiredObject)
	if len(missingObject) == 0 {
		return ClientCapabilities{}
	}
	missingRaw, _ := json.Marshal(missingObject)
	var missing ClientCapabilities
	_ = json.Unmarshal(missingRaw, &missing)
	return missing
}

func missingJSONObjectMembers(actual, required JSONObject) JSONObject {
	missing := make(JSONObject)
	for name, requiredValue := range required {
		actualValue, ok := actual[name]
		if !ok {
			missing[name] = bytes.Clone(requiredValue)
			continue
		}
		requiredObject, requiredIsObject := rawJSONObject(requiredValue)
		if !requiredIsObject {
			if !jsonValuesEqual(actualValue, requiredValue) {
				missing[name] = bytes.Clone(requiredValue)
			}
			continue
		}
		actualObject, actualIsObject := rawJSONObject(actualValue)
		if !actualIsObject {
			missing[name] = bytes.Clone(requiredValue)
			continue
		}
		nested := missingJSONObjectMembers(actualObject, requiredObject)
		if len(nested) > 0 {
			missing[name], _ = json.Marshal(nested)
		}
	}
	return missing
}

func rawJSONObject(raw json.RawMessage) (JSONObject, bool) {
	object, err := decodeJSONObject(raw)
	return object, err == nil
}

func jsonValuesEqual(left, right json.RawMessage) bool {
	var leftValue any
	var rightValue any
	if json.Unmarshal(left, &leftValue) != nil || json.Unmarshal(right, &rightValue) != nil {
		return false
	}
	return reflect.DeepEqual(leftValue, rightValue)
}
