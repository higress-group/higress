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
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"reflect"
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
	maxSchemaSnapshotDepth  = 256
	maxSchemaSnapshotNodes  = 65536
	maxSchemaSnapshotItems  = 4 * maxSchemaCollectionSize
	maxSchemaSnapshotText   = 4 * maxToolInputSchemaBytes
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
	root *inputSchemaNode
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

type descriptorTraversalRole uint8

const (
	descriptorRoleSchemaNode descriptorTraversalRole = iota
	descriptorRolePropertiesContainer
	descriptorRoleKeywordCollection
	descriptorRoleGenericContainer
)

// descriptorResourceState is intentionally separate from schemaCompileState.
// Schema nodes retain the compiler's historical capacity, while unknown and
// annotation JSON containers have their own independent resource budget.
type descriptorResourceState struct {
	schemaNodes  int
	genericNodes int
}

type schemaDiagnosticReason uint8

const (
	_ schemaDiagnosticReason = iota
	schemaDiagnosticUnsupportedKeyword
	schemaDiagnosticUnsupportedForm
	schemaDiagnosticContradictoryConstraint
	schemaDiagnosticResourceLimit
	schemaDiagnosticSerializationFailure
)

func (r schemaDiagnosticReason) String() string {
	switch r {
	case schemaDiagnosticUnsupportedKeyword:
		return "unsupported_keyword"
	case schemaDiagnosticUnsupportedForm:
		return "unsupported_form"
	case schemaDiagnosticContradictoryConstraint:
		return "contradictory_constraint"
	case schemaDiagnosticResourceLimit:
		return "resource_limit"
	case schemaDiagnosticSerializationFailure:
		return "serialization_failure"
	default:
		return "none"
	}
}

type schemaCompilationError struct {
	reason schemaDiagnosticReason
	err    error
}

func (e *schemaCompilationError) Error() string { return e.err.Error() }
func (e *schemaCompilationError) Unwrap() error { return e.err }

func schemaCompileError(reason schemaDiagnosticReason, format string, args ...any) error {
	return &schemaCompilationError{reason: reason, err: fmt.Errorf(format, args...)}
}

type schemaSnapshotVisit struct {
	kind    reflect.Kind
	pointer uintptr
}

type schemaSnapshotState struct {
	active map[schemaSnapshotVisit]struct{}
	nodes  int
	text   int
}

var (
	jsonMarshalerType = reflect.TypeOf((*json.Marshaler)(nil)).Elem()
	textMarshalerType = reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem()
)

// cloneSchemaSnapshot traverses only JSON container and primitive kinds. It
// deliberately does not invoke arbitrary marshalers or follow pointers and
// structs: either could hide cycles or mutable state outside the owned schema
// snapshot. The independent limits preserve the semantic compiler's historical
// depth/node capacity while bounding generic container work.
func cloneSchemaSnapshot(schema map[string]any) (map[string]any, error) {
	cloned, err := cloneSchemaSnapshotValue(reflect.ValueOf(schema), 0, &schemaSnapshotState{
		active: make(map[schemaSnapshotVisit]struct{}),
	})
	if err != nil {
		return nil, err
	}
	if !cloned.IsValid() || cloned.IsNil() {
		return nil, nil
	}
	return cloned.Interface().(map[string]any), nil
}

func cloneSchemaSnapshotValue(value reflect.Value, depth int, state *schemaSnapshotState) (reflect.Value, error) {
	if !value.IsValid() {
		return value, nil
	}
	if value.Kind() == reflect.Interface {
		if value.IsNil() {
			return reflect.Zero(value.Type()), nil
		}
		cloned, err := cloneSchemaSnapshotValue(value.Elem(), depth, state)
		if err != nil {
			return reflect.Value{}, err
		}
		result := reflect.New(value.Type()).Elem()
		result.Set(cloned)
		return result, nil
	}
	state.nodes++
	if state.nodes > maxSchemaSnapshotNodes {
		return reflect.Value{}, schemaCompileError(schemaDiagnosticResourceLimit,
			"input schema snapshot exceeds %d JSON values", maxSchemaSnapshotNodes)
	}
	if hasCustomSchemaMarshaler(value.Type()) {
		return reflect.Value{}, schemaCompileError(schemaDiagnosticSerializationFailure,
			"input schema contains unsupported custom JSON representation %s", value.Type())
	}
	switch value.Kind() {
	case reflect.Map:
		if value.IsNil() {
			return reflect.Zero(value.Type()), nil
		}
		if value.Type().Key().Kind() != reflect.String {
			return reflect.Value{}, schemaCompileError(schemaDiagnosticSerializationFailure,
				"input schema contains non-string map keys")
		}
		if hasCustomSchemaMarshaler(value.Type().Key()) {
			return reflect.Value{}, schemaCompileError(schemaDiagnosticSerializationFailure,
				"input schema contains unsupported custom JSON map keys %s", value.Type().Key())
		}
		if value.Len() > maxSchemaSnapshotItems {
			return reflect.Value{}, schemaCompileError(schemaDiagnosticResourceLimit,
				"input schema snapshot collection exceeds %d entries", maxSchemaSnapshotItems)
		}
		if depth >= maxSchemaSnapshotDepth {
			return reflect.Value{}, schemaCompileError(schemaDiagnosticResourceLimit,
				"input schema snapshot nesting exceeds %d", maxSchemaSnapshotDepth)
		}
		leave, err := enterSchemaSnapshotContainer(value, state)
		if err != nil {
			return reflect.Value{}, err
		}
		defer leave()
		result := reflect.MakeMapWithSize(value.Type(), value.Len())
		iterator := value.MapRange()
		for iterator.Next() {
			key := iterator.Key()
			if err := addSchemaSnapshotText(key.String(), state); err != nil {
				return reflect.Value{}, err
			}
			cloned, err := cloneSchemaSnapshotValue(iterator.Value(), depth+1, state)
			if err != nil {
				return reflect.Value{}, err
			}
			result.SetMapIndex(key, cloned)
		}
		return result, nil
	case reflect.Slice:
		if value.IsNil() {
			return reflect.Zero(value.Type()), nil
		}
		if value.Len() > maxSchemaSnapshotItems {
			return reflect.Value{}, schemaCompileError(schemaDiagnosticResourceLimit,
				"input schema snapshot collection exceeds %d entries", maxSchemaSnapshotItems)
		}
		if depth >= maxSchemaSnapshotDepth {
			return reflect.Value{}, schemaCompileError(schemaDiagnosticResourceLimit,
				"input schema snapshot nesting exceeds %d", maxSchemaSnapshotDepth)
		}
		leave, err := enterSchemaSnapshotContainer(value, state)
		if err != nil {
			return reflect.Value{}, err
		}
		defer leave()
		result := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		for index := 0; index < value.Len(); index++ {
			cloned, err := cloneSchemaSnapshotValue(value.Index(index), depth+1, state)
			if err != nil {
				return reflect.Value{}, err
			}
			result.Index(index).Set(cloned)
		}
		return result, nil
	case reflect.Array:
		if value.Len() > maxSchemaSnapshotItems {
			return reflect.Value{}, schemaCompileError(schemaDiagnosticResourceLimit,
				"input schema snapshot collection exceeds %d entries", maxSchemaSnapshotItems)
		}
		if depth >= maxSchemaSnapshotDepth {
			return reflect.Value{}, schemaCompileError(schemaDiagnosticResourceLimit,
				"input schema snapshot nesting exceeds %d", maxSchemaSnapshotDepth)
		}
		result := reflect.New(value.Type()).Elem()
		for index := 0; index < value.Len(); index++ {
			cloned, err := cloneSchemaSnapshotValue(value.Index(index), depth+1, state)
			if err != nil {
				return reflect.Value{}, err
			}
			result.Index(index).Set(cloned)
		}
		return result, nil
	case reflect.String:
		if err := addSchemaSnapshotText(value.String(), state); err != nil {
			return reflect.Value{}, err
		}
		return value, nil
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return value, nil
	case reflect.Float32, reflect.Float64:
		if number := value.Float(); math.IsNaN(number) || math.IsInf(number, 0) {
			return reflect.Value{}, schemaCompileError(schemaDiagnosticSerializationFailure,
				"input schema contains a non-finite number")
		}
		return value, nil
	default:
		return reflect.Value{}, schemaCompileError(schemaDiagnosticSerializationFailure,
			"input schema contains unsupported Go value %s", value.Type())
	}
}

func hasCustomSchemaMarshaler(valueType reflect.Type) bool {
	if valueType == reflect.TypeOf(json.Number("")) {
		return false
	}
	if valueType.Implements(jsonMarshalerType) || valueType.Implements(textMarshalerType) {
		return true
	}
	return valueType.Kind() != reflect.Pointer &&
		(reflect.PointerTo(valueType).Implements(jsonMarshalerType) ||
			reflect.PointerTo(valueType).Implements(textMarshalerType))
}

func enterSchemaSnapshotContainer(value reflect.Value, state *schemaSnapshotState) (func(), error) {
	visit := schemaSnapshotVisit{kind: value.Kind(), pointer: value.Pointer()}
	if _, found := state.active[visit]; found {
		return nil, schemaCompileError(schemaDiagnosticSerializationFailure, "input schema contains a cyclic JSON container")
	}
	state.active[visit] = struct{}{}
	return func() { delete(state.active, visit) }, nil
}

func addSchemaSnapshotText(value string, state *schemaSnapshotState) error {
	state.text += len(value)
	if state.text > maxSchemaSnapshotText {
		return schemaCompileError(schemaDiagnosticResourceLimit,
			"input schema snapshot text exceeds %d bytes", maxSchemaSnapshotText)
	}
	return nil
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

// cloneToolInputSchema captures a generation-owned descriptor when it is a
// bounded tree of explicit JSON containers and primitives. Validator
// preparation is a separate concern and must not decide whether configuration
// is accepted.
func cloneToolInputSchema(schema map[string]any) (map[string]any, []byte, error) {
	owned, err := cloneSchemaSnapshot(schema)
	if err != nil {
		return nil, nil, err
	}
	raw, err := json.Marshal(owned)
	if err != nil {
		return nil, nil, schemaCompileError(schemaDiagnosticSerializationFailure, "input schema is not JSON-compatible: %v", err)
	}
	value, err := decodeJSONValue(raw)
	if err != nil {
		return nil, nil, schemaCompileError(schemaDiagnosticSerializationFailure, "input schema is malformed: %v", err)
	}
	if value == nil {
		return nil, raw, nil
	}
	normalized, ok := value.(map[string]any)
	if !ok {
		return nil, raw, schemaCompileError(schemaDiagnosticUnsupportedForm, "input schema must be an object")
	}
	return normalized, raw, nil
}

func validateToolInputSchemaPreparation(normalized map[string]any, raw []byte) error {
	if normalized == nil {
		return schemaCompileError(schemaDiagnosticUnsupportedForm, "input schema must be an object")
	}
	if len(raw) > maxToolInputSchemaBytes {
		return schemaCompileError(schemaDiagnosticResourceLimit, "input schema exceeds %d bytes", maxToolInputSchemaBytes)
	}
	if normalized["type"] != "object" {
		return schemaCompileError(schemaDiagnosticUnsupportedForm, "input schema root must declare type \"object\"")
	}
	if err := validateJSONDescriptorResourceBounds(normalized, "$", descriptorRoleSchemaNode, 0, 0, false, &descriptorResourceState{}); err != nil {
		return schemaCompileError(schemaDiagnosticResourceLimit, "%v", err)
	}
	return nil
}

func prepareToolInputSchema(schema map[string]any) (map[string]any, bool, *compiledInputSchema, error) {
	normalized, raw, err := cloneToolInputSchema(schema)
	if err != nil {
		return nil, false, nil, err
	}
	if err := validateToolInputSchemaPreparation(normalized, raw); err != nil {
		return normalized, true, nil, err
	}
	validator, err := compileNormalizedToolInputSchema(normalized)
	return normalized, true, validator, err
}

func schemaPreparationDiagnosticReason(err error) schemaDiagnosticReason {
	var typed *schemaCompilationError
	if errors.As(err, &typed) {
		return typed.reason
	}
	// All remaining compiler failures are bounded resource checks. Keeping this
	// fallback typed ensures an unclassified compiler error cannot become a
	// configuration-fatal path or an unbounded diagnostic label.
	return schemaDiagnosticResourceLimit
}

func validateJSONDescriptorResourceBounds(
	value any,
	path string,
	role descriptorTraversalRole,
	schemaDepth int,
	genericDepth int,
	enumValues bool,
	state *descriptorResourceState,
) error {
	switch typed := value.(type) {
	case map[string]any:
		return validateJSONDescriptorMapBounds(typed, path, role, schemaDepth, genericDepth, state)
	case []any:
		if role != descriptorRoleKeywordCollection {
			if genericDepth > maxSchemaDepth {
				return fmt.Errorf("%s: JSON container nesting exceeds %d", path, maxSchemaDepth)
			}
			state.genericNodes++
			if state.genericNodes > maxSchemaNodes {
				return fmt.Errorf("input schema exceeds %d generic JSON container nodes", maxSchemaNodes)
			}
		}
		limit := maxSchemaCollectionSize
		if enumValues {
			limit = maxSchemaEnumSize
		}
		if len(typed) > limit {
			return fmt.Errorf("%s: exceeds %d values", path, limit)
		}
		for i, item := range typed {
			if number, ok := item.(json.Number); ok && enumValues {
				if _, comparable := comparableJSONNumber(number); !comparable {
					return fmt.Errorf("%s[%d]: numeric value exceeds comparison bounds", path, i)
				}
			}
			childGenericDepth := genericDepth + 1
			if role == descriptorRoleKeywordCollection {
				childGenericDepth = 0
			}
			if err := validateJSONDescriptorResourceBounds(item, fmt.Sprintf("%s[%d]", path, i), descriptorRoleGenericContainer, schemaDepth, childGenericDepth, false, state); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateJSONDescriptorMapBounds(
	value map[string]any,
	path string,
	role descriptorTraversalRole,
	schemaDepth int,
	genericDepth int,
	state *descriptorResourceState,
) error {
	if len(value) > maxSchemaCollectionSize {
		return fmt.Errorf("%s: exceeds %d entries", path, maxSchemaCollectionSize)
	}
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	switch role {
	case descriptorRoleSchemaNode:
		if schemaDepth > maxSchemaDepth {
			return fmt.Errorf("%s: schema nesting exceeds %d", path, maxSchemaDepth)
		}
		state.schemaNodes++
		if state.schemaNodes > maxSchemaNodes {
			return fmt.Errorf("input schema exceeds %d schema nodes", maxSchemaNodes)
		}
		for _, key := range keys {
			child := value[key]
			childRole := descriptorRoleGenericContainer
			childSchemaDepth := schemaDepth
			if _, isMap := child.(map[string]any); isMap {
				switch key {
				case "properties":
					childRole = descriptorRolePropertiesContainer
				case "items", "additionalProperties":
					childRole = descriptorRoleSchemaNode
					childSchemaDepth++
				}
			}
			if _, isArray := child.([]any); isArray {
				switch key {
				case "enum", "required", "examples":
					childRole = descriptorRoleKeywordCollection
				}
			}
			if err := validateJSONDescriptorResourceBounds(child, path+"."+key, childRole, childSchemaDepth, 0, key == "enum", state); err != nil {
				return err
			}
		}
	case descriptorRolePropertiesContainer:
		// This role is reachable only from a schema node's map-valued
		// "properties" member. The container is structural; each map-valued
		// property is the next semantic schema node.
		for _, key := range keys {
			child := value[key]
			childRole := descriptorRoleGenericContainer
			childSchemaDepth := schemaDepth
			if _, isMap := child.(map[string]any); isMap {
				childRole = descriptorRoleSchemaNode
				childSchemaDepth++
			}
			if err := validateJSONDescriptorResourceBounds(child, path+"."+key, childRole, childSchemaDepth, 0, false, state); err != nil {
				return err
			}
		}
	case descriptorRoleGenericContainer, descriptorRoleKeywordCollection:
		if genericDepth > maxSchemaDepth {
			return fmt.Errorf("%s: JSON container nesting exceeds %d", path, maxSchemaDepth)
		}
		state.genericNodes++
		if state.genericNodes > maxSchemaNodes {
			return fmt.Errorf("input schema exceeds %d generic JSON container nodes", maxSchemaNodes)
		}
		for _, key := range keys {
			if err := validateJSONDescriptorResourceBounds(value[key], path+"."+key, descriptorRoleGenericContainer, schemaDepth, genericDepth+1, false, state); err != nil {
				return err
			}
		}
	}
	return nil
}

func compileNormalizedToolInputSchema(normalized map[string]any) (*compiledInputSchema, error) {
	root, err := compileInputSchemaNode(normalized, "$", 0, &schemaCompileState{})
	if err != nil {
		return nil, err
	}
	if root.kind != schemaObject {
		return nil, errors.New("input schema root must declare type \"object\"")
	}
	return &compiledInputSchema{root: root}, nil
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
			return nil, schemaCompileError(schemaDiagnosticUnsupportedKeyword, "%s: unsupported schema keyword %q", path, keyword)
		}
	}

	node := &inputSchemaNode{kind: schemaAny, allowAdditional: true}
	if rawType, present := schema["type"]; present {
		typeName, ok := rawType.(string)
		if !ok {
			return nil, schemaCompileError(schemaDiagnosticUnsupportedForm, "%s.type: must be a string", path)
		}
		kind, ok := parseSchemaValueKind(typeName)
		if !ok {
			return nil, schemaCompileError(schemaDiagnosticUnsupportedForm, "%s.type: unsupported type %q", path, typeName)
		}
		node.kind = kind
	}

	if description, present := schema["description"]; present {
		if _, ok := description.(string); !ok {
			return nil, schemaCompileError(schemaDiagnosticUnsupportedForm, "%s.description: must be a string", path)
		}
	}
	if examples, present := schema["examples"]; present {
		values, ok := examples.([]any)
		if !ok {
			return nil, schemaCompileError(schemaDiagnosticUnsupportedForm, "%s.examples: must be an array", path)
		}
		if len(values) > maxSchemaCollectionSize {
			return nil, fmt.Errorf("%s.examples: exceeds %d values", path, maxSchemaCollectionSize)
		}
	}

	if rawEnum, present := schema["enum"]; present {
		values, ok := rawEnum.([]any)
		if !ok || len(values) == 0 {
			return nil, schemaCompileError(schemaDiagnosticUnsupportedForm, "%s.enum: must be a non-empty array", path)
		}
		if len(values) > maxSchemaEnumSize {
			return nil, fmt.Errorf("%s.enum: exceeds %d values", path, maxSchemaEnumSize)
		}
		if node.kind == schemaAny || node.kind == schemaObject || node.kind == schemaArray {
			return nil, schemaCompileError(schemaDiagnosticContradictoryConstraint, "%s.enum: requires a primitive type", path)
		}
		for i, value := range values {
			if !valueMatchesSchemaKind(value, node.kind) {
				return nil, schemaCompileError(schemaDiagnosticContradictoryConstraint, "%s.enum[%d]: does not match declared type", path, i)
			}
			if number, ok := value.(json.Number); ok {
				if _, comparable := comparableJSONNumber(number); !comparable {
					return nil, schemaCompileError(schemaDiagnosticUnsupportedForm, "%s.enum[%d]: numeric value exceeds comparison bounds", path, i)
				}
			}
			for j := 0; j < i; j++ {
				if equalJSONValue(value, values[j]) {
					return nil, schemaCompileError(schemaDiagnosticContradictoryConstraint, "%s.enum: duplicate value at index %d", path, i)
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
			return nil, schemaCompileError(schemaDiagnosticUnsupportedForm, "%s.properties: must be an object", path)
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
				return nil, schemaCompileError(schemaDiagnosticUnsupportedForm, "%s.properties[%q]: must be an object schema", path, name)
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
			return nil, schemaCompileError(schemaDiagnosticUnsupportedForm, "%s.required: must be an array of strings", path)
		}
		if len(required) > maxSchemaCollectionSize {
			return nil, fmt.Errorf("%s.required: exceeds %d entries", path, maxSchemaCollectionSize)
		}
		seen := make(map[string]struct{}, len(required))
		for i, rawName := range required {
			name, ok := rawName.(string)
			if !ok || name == "" {
				return nil, schemaCompileError(schemaDiagnosticUnsupportedForm, "%s.required[%d]: must be a non-empty string", path, i)
			}
			if _, duplicate := seen[name]; duplicate {
				return nil, schemaCompileError(schemaDiagnosticContradictoryConstraint, "%s.required: duplicate property %q", path, name)
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
			return nil, schemaCompileError(schemaDiagnosticUnsupportedForm, "%s.additionalProperties: must be a boolean or object schema", path)
		}
	}
	if objectKeyword && node.kind != schemaObject {
		return nil, schemaCompileError(schemaDiagnosticContradictoryConstraint, "%s: object keywords require type \"object\"", path)
	}

	if rawItems, present := schema["items"]; present {
		if node.kind != schemaArray {
			return nil, schemaCompileError(schemaDiagnosticContradictoryConstraint, "%s.items: requires type \"array\"", path)
		}
		items, ok := rawItems.(map[string]any)
		if !ok {
			return nil, schemaCompileError(schemaDiagnosticUnsupportedForm, "%s.items: must be an object schema", path)
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
