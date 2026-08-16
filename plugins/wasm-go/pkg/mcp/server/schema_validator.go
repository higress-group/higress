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
package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"sort"
	"strconv"
	"strings"
)

const (
	maxToolInputSchemaBytes = 1 << 20
	maxToolArgumentsBytes   = 1 << 20
	maxSchemaDepth          = 64
	maxSchemaNodes          = 4096
	maxSchemaCollectionSize = 4096
	maxSchemaEnumSize       = 256
	maxArgumentDepth        = 64
	maxArgumentNodes        = 65536
	maxComparableNumberSize = 1024
	maxComparableExponent   = 4096
)

type schemaValueKind uint8

const (
	schemaAny schemaValueKind = iota
	schemaObject
	schemaArray
	schemaString
	schemaNumber
	schemaInteger
	schemaBoolean
	schemaNull
)

type compiledInputSchema struct {
	descriptor map[string]any
	root       *inputSchemaNode
}

type inputSchemaNode struct {
	kind               schemaValueKind
	enum               []any
	properties         map[string]*inputSchemaNode
	propertyNames      []string
	required           []string
	items              *inputSchemaNode
	allowAdditional    bool
	additionalProperty *inputSchemaNode
}

type schemaCompileState struct {
	nodes int
}

type argumentValidationState struct {
	nodes int
}

var supportedInputSchemaKeywords = map[string]struct{}{
	"type":                 {},
	"properties":           {},
	"required":             {},
	"enum":                 {},
	"items":                {},
	"additionalProperties": {},
	// Current REST and reflected registered-tool schemas use these annotations.
	"description": {},
	"default":     {},
	"examples":    {},
}

// compileToolInputSchema accepts only the bounded schema vocabulary that the
// direct-tool runtime validates. Normalization both copies the public
// descriptor and removes Go-specific numeric/container representations.
func compileToolInputSchema(schema map[string]any) (*compiledInputSchema, error) {
	if schema == nil {
		return nil, errors.New("input schema must be an object")
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		return nil, fmt.Errorf("input schema is not JSON-compatible: %w", err)
	}
	if len(raw) > maxToolInputSchemaBytes {
		return nil, fmt.Errorf("input schema exceeds %d bytes", maxToolInputSchemaBytes)
	}
	normalized, err := decodeJSONObject(raw)
	if err != nil {
		return nil, fmt.Errorf("input schema is malformed: %w", err)
	}
	root, err := compileInputSchemaNode(normalized, "$", 0, &schemaCompileState{})
	if err != nil {
		return nil, err
	}
	if root.kind != schemaObject {
		return nil, errors.New("input schema root must declare type \"object\"")
	}
	return &compiledInputSchema{descriptor: normalized, root: root}, nil
}

func compileInputSchemaNode(schema map[string]any, path string, depth int, state *schemaCompileState) (*inputSchemaNode, error) {
	if depth > maxSchemaDepth {
		return nil, fmt.Errorf("%s: schema nesting exceeds %d", path, maxSchemaDepth)
	}
	state.nodes++
	if state.nodes > maxSchemaNodes {
		return nil, fmt.Errorf("input schema exceeds %d schema nodes", maxSchemaNodes)
	}

	keys := make([]string, 0, len(schema))
	for keyword := range schema {
		keys = append(keys, keyword)
	}
	sort.Strings(keys)
	for _, keyword := range keys {
		if _, supported := supportedInputSchemaKeywords[keyword]; !supported {
			return nil, fmt.Errorf("%s: unsupported schema keyword %q", path, keyword)
		}
	}

	node := &inputSchemaNode{kind: schemaAny, allowAdditional: true}
	if rawType, present := schema["type"]; present {
		typeName, ok := rawType.(string)
		if !ok {
			return nil, fmt.Errorf("%s.type: must be a string", path)
		}
		kind, ok := parseSchemaValueKind(typeName)
		if !ok {
			return nil, fmt.Errorf("%s.type: unsupported type %q", path, typeName)
		}
		node.kind = kind
	}

	if description, present := schema["description"]; present {
		if _, ok := description.(string); !ok {
			return nil, fmt.Errorf("%s.description: must be a string", path)
		}
	}
	if examples, present := schema["examples"]; present {
		values, ok := examples.([]any)
		if !ok {
			return nil, fmt.Errorf("%s.examples: must be an array", path)
		}
		if len(values) > maxSchemaCollectionSize {
			return nil, fmt.Errorf("%s.examples: exceeds %d values", path, maxSchemaCollectionSize)
		}
	}

	if rawEnum, present := schema["enum"]; present {
		values, ok := rawEnum.([]any)
		if !ok || len(values) == 0 {
			return nil, fmt.Errorf("%s.enum: must be a non-empty array", path)
		}
		if len(values) > maxSchemaEnumSize {
			return nil, fmt.Errorf("%s.enum: exceeds %d values", path, maxSchemaEnumSize)
		}
		if node.kind == schemaAny || node.kind == schemaObject || node.kind == schemaArray {
			return nil, fmt.Errorf("%s.enum: requires a primitive type", path)
		}
		for i, value := range values {
			if !valueMatchesSchemaKind(value, node.kind) {
				return nil, fmt.Errorf("%s.enum[%d]: does not match declared type", path, i)
			}
			if number, ok := value.(json.Number); ok {
				if _, comparable := comparableJSONNumber(number); !comparable {
					return nil, fmt.Errorf("%s.enum[%d]: numeric value exceeds comparison bounds", path, i)
				}
			}
			for j := 0; j < i; j++ {
				if equalJSONValue(value, values[j]) {
					return nil, fmt.Errorf("%s.enum: duplicate value at index %d", path, i)
				}
			}
		}
		node.enum = values
	}

	objectKeyword := false
	if rawProperties, present := schema["properties"]; present {
		objectKeyword = true
		properties, ok := rawProperties.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s.properties: must be an object", path)
		}
		if len(properties) > maxSchemaCollectionSize {
			return nil, fmt.Errorf("%s.properties: exceeds %d entries", path, maxSchemaCollectionSize)
		}
		node.properties = make(map[string]*inputSchemaNode, len(properties))
		node.propertyNames = make([]string, 0, len(properties))
		for name := range properties {
			node.propertyNames = append(node.propertyNames, name)
		}
		sort.Strings(node.propertyNames)
		for _, name := range node.propertyNames {
			propertySchema, ok := properties[name].(map[string]any)
			if !ok {
				return nil, fmt.Errorf("%s.properties[%q]: must be an object schema", path, name)
			}
			compiled, err := compileInputSchemaNode(propertySchema, schemaPropertyPath(path, name), depth+1, state)
			if err != nil {
				return nil, err
			}
			node.properties[name] = compiled
		}
	}

	if rawRequired, present := schema["required"]; present {
		objectKeyword = true
		required, ok := rawRequired.([]any)
		if !ok {
			return nil, fmt.Errorf("%s.required: must be an array of strings", path)
		}
		if len(required) > maxSchemaCollectionSize {
			return nil, fmt.Errorf("%s.required: exceeds %d entries", path, maxSchemaCollectionSize)
		}
		seen := make(map[string]struct{}, len(required))
		for i, rawName := range required {
			name, ok := rawName.(string)
			if !ok || name == "" {
				return nil, fmt.Errorf("%s.required[%d]: must be a non-empty string", path, i)
			}
			if _, duplicate := seen[name]; duplicate {
				return nil, fmt.Errorf("%s.required: duplicate property %q", path, name)
			}
			seen[name] = struct{}{}
			node.required = append(node.required, name)
		}
		sort.Strings(node.required)
	}

	if rawAdditional, present := schema["additionalProperties"]; present {
		objectKeyword = true
		switch additional := rawAdditional.(type) {
		case bool:
			node.allowAdditional = additional
		case map[string]any:
			compiled, err := compileInputSchemaNode(additional, path+".additionalProperties", depth+1, state)
			if err != nil {
				return nil, err
			}
			node.allowAdditional = true
			node.additionalProperty = compiled
		default:
			return nil, fmt.Errorf("%s.additionalProperties: must be a boolean or object schema", path)
		}
	}
	if objectKeyword && node.kind != schemaObject {
		return nil, fmt.Errorf("%s: object keywords require type \"object\"", path)
	}

	if rawItems, present := schema["items"]; present {
		if node.kind != schemaArray {
			return nil, fmt.Errorf("%s.items: requires type \"array\"", path)
		}
		items, ok := rawItems.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s.items: must be an object schema", path)
		}
		compiled, err := compileInputSchemaNode(items, path+".items", depth+1, state)
		if err != nil {
			return nil, err
		}
		node.items = compiled
	}

	return node, nil
}

func parseSchemaValueKind(value string) (schemaValueKind, bool) {
	switch value {
	case "object":
		return schemaObject, true
	case "array":
		return schemaArray, true
	case "string":
		return schemaString, true
	case "number":
		return schemaNumber, true
	case "integer":
		return schemaInteger, true
	case "boolean":
		return schemaBoolean, true
	case "null":
		return schemaNull, true
	default:
		return schemaAny, false
	}
}

func (schema *compiledInputSchema) validateArguments(raw string) error {
	if raw == "" {
		raw = "{}"
	}
	if len(raw) > maxToolArgumentsBytes {
		return fmt.Errorf("arguments exceed %d bytes", maxToolArgumentsBytes)
	}
	value, err := decodeJSONValue([]byte(raw))
	if err != nil {
		return fmt.Errorf("arguments are malformed JSON: %w", err)
	}
	return validateInputValue(schema.root, value, "$", 0, &argumentValidationState{})
}

func validateInputValue(schema *inputSchemaNode, value any, path string, depth int, state *argumentValidationState) error {
	if depth > maxArgumentDepth {
		return fmt.Errorf("%s: argument nesting exceeds %d", path, maxArgumentDepth)
	}
	state.nodes++
	if state.nodes > maxArgumentNodes {
		return fmt.Errorf("arguments exceed %d values", maxArgumentNodes)
	}
	if !valueMatchesSchemaKind(value, schema.kind) {
		return fmt.Errorf("%s: expected %s", path, schemaKindName(schema.kind))
	}
	if len(schema.enum) > 0 {
		matched := false
		for _, candidate := range schema.enum {
			if equalJSONValue(value, candidate) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("%s: value is not in the allowed enum", path)
		}
	}

	switch typed := value.(type) {
	case map[string]any:
		for _, name := range schema.required {
			if _, present := typed[name]; !present {
				return fmt.Errorf("%s: required property %q is missing", path, name)
			}
		}
		for _, name := range schema.propertyNames {
			propertyValue, present := typed[name]
			if !present {
				continue
			}
			if err := validateInputValue(schema.properties[name], propertyValue, argumentPropertyPath(path, name), depth+1, state); err != nil {
				return err
			}
		}
		additionalNames := make([]string, 0)
		for name := range typed {
			if _, declared := schema.properties[name]; !declared {
				additionalNames = append(additionalNames, name)
			}
		}
		sort.Strings(additionalNames)
		for _, name := range additionalNames {
			if !schema.allowAdditional {
				return fmt.Errorf("%s: additional property %q is not allowed", path, name)
			}
			if schema.additionalProperty != nil {
				if err := validateInputValue(schema.additionalProperty, typed[name], argumentPropertyPath(path, name), depth+1, state); err != nil {
					return err
				}
			}
		}
	case []any:
		if schema.items != nil {
			for i, item := range typed {
				if err := validateInputValue(schema.items, item, fmt.Sprintf("%s[%d]", path, i), depth+1, state); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func valueMatchesSchemaKind(value any, kind schemaValueKind) bool {
	switch kind {
	case schemaAny:
		return true
	case schemaObject:
		_, ok := value.(map[string]any)
		return ok
	case schemaArray:
		_, ok := value.([]any)
		return ok
	case schemaString:
		_, ok := value.(string)
		return ok
	case schemaNumber:
		_, ok := value.(json.Number)
		return ok
	case schemaInteger:
		number, ok := value.(json.Number)
		return ok && isJSONInteger(number)
	case schemaBoolean:
		_, ok := value.(bool)
		return ok
	case schemaNull:
		return value == nil
	default:
		return false
	}
}

func schemaKindName(kind schemaValueKind) string {
	switch kind {
	case schemaObject:
		return "object"
	case schemaArray:
		return "array"
	case schemaString:
		return "string"
	case schemaNumber:
		return "number"
	case schemaInteger:
		return "integer"
	case schemaBoolean:
		return "boolean"
	case schemaNull:
		return "null"
	default:
		return "value"
	}
}

func isJSONInteger(number json.Number) bool {
	value := number.String()
	if value == "" {
		return false
	}
	if value[0] == '-' {
		value = value[1:]
	}
	exponent := 0
	if index := strings.IndexAny(value, "eE"); index >= 0 {
		rawExponent := value[index+1:]
		value = value[:index]
		negativeExponent := strings.HasPrefix(rawExponent, "-")
		digits := strings.TrimLeft(strings.TrimPrefix(strings.TrimPrefix(rawExponent, "+"), "-"), "0")
		if len(digits) > 9 {
			if negativeExponent {
				return numberMantissaIsZero(value)
			}
			return true
		}
		if digits != "" {
			parsed, err := strconv.Atoi(digits)
			if err != nil {
				return false
			}
			exponent = parsed
			if negativeExponent {
				exponent = -exponent
			}
		}
	}
	decimalIndex := strings.IndexByte(value, '.')
	if decimalIndex < 0 {
		if exponent >= 0 {
			return true
		}
		fractionalDigits := -exponent
		if numberMantissaIsZero(value) {
			return true
		}
		if fractionalDigits >= len(value) {
			return false
		}
		return allZeroes(value[len(value)-fractionalDigits:])
	}
	digits := value[:decimalIndex] + value[decimalIndex+1:]
	fractionalDigits := len(value) - decimalIndex - 1 - exponent
	if fractionalDigits <= 0 || numberMantissaIsZero(digits) {
		return true
	}
	if fractionalDigits >= len(digits) {
		return false
	}
	return allZeroes(digits[len(digits)-fractionalDigits:])
}

func numberMantissaIsZero(value string) bool {
	return allZeroes(strings.ReplaceAll(value, ".", ""))
}

func allZeroes(value string) bool {
	for _, char := range value {
		if char != '0' {
			return false
		}
	}
	return true
}

func comparableJSONNumber(number json.Number) (*big.Rat, bool) {
	value := number.String()
	if len(value) > maxComparableNumberSize {
		return nil, false
	}
	if index := strings.IndexAny(value, "eE"); index >= 0 {
		exponent, err := strconv.ParseInt(value[index+1:], 10, 32)
		if err != nil || exponent < -maxComparableExponent || exponent > maxComparableExponent {
			return nil, false
		}
	}
	parsed, ok := new(big.Rat).SetString(value)
	return parsed, ok
}

func equalJSONValue(left, right any) bool {
	leftNumber, leftIsNumber := left.(json.Number)
	rightNumber, rightIsNumber := right.(json.Number)
	if leftIsNumber || rightIsNumber {
		if !leftIsNumber || !rightIsNumber {
			return false
		}
		leftValue, leftOK := comparableJSONNumber(leftNumber)
		rightValue, rightOK := comparableJSONNumber(rightNumber)
		return leftOK && rightOK && leftValue.Cmp(rightValue) == 0
	}
	switch leftValue := left.(type) {
	case nil:
		return right == nil
	case string:
		rightValue, ok := right.(string)
		return ok && leftValue == rightValue
	case bool:
		rightValue, ok := right.(bool)
		return ok && leftValue == rightValue
	case []any:
		rightValue, ok := right.([]any)
		if !ok || len(leftValue) != len(rightValue) {
			return false
		}
		for i := range leftValue {
			if !equalJSONValue(leftValue[i], rightValue[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		rightValue, ok := right.(map[string]any)
		if !ok || len(leftValue) != len(rightValue) {
			return false
		}
		for key, value := range leftValue {
			other, present := rightValue[key]
			if !present || !equalJSONValue(value, other) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func decodeJSONObject(raw []byte) (map[string]any, error) {
	value, err := decodeJSONValue(raw)
	if err != nil {
		return nil, err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("must be a JSON object")
	}
	return object, nil
}

func decodeJSONValue(raw []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("contains trailing JSON")
		}
		return nil, err
	}
	return value, nil
}

func schemaPropertyPath(path, name string) string {
	return path + ".properties[\"" + escapePathName(name) + "\"]"
}

func argumentPropertyPath(path, name string) string {
	return path + "[\"" + escapePathName(name) + "\"]"
}

func escapePathName(name string) string {
	return strings.NewReplacer("\\", "\\\\", "\"", "\\\"").Replace(name)
}
