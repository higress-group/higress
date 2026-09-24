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
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func supportedInputSchemaFixture() map[string]any {
	return map[string]any{
		"type":        "object",
		"description": "bounded tool input",
		"default":     map[string]any{},
		"properties": map[string]any{
			"query": map[string]any{
				"type":     "string",
				"enum":     []any{"alpha", "beta"},
				"examples": []any{"alpha"},
			},
			"count":   map[string]any{"type": "integer"},
			"score":   map[string]any{"type": "number"},
			"enabled": map[string]any{"type": "boolean"},
			"empty":   map[string]any{"type": "null"},
			"tags": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"metadata": map[string]any{
				"type":                 "object",
				"properties":           map[string]any{"owner": map[string]any{"type": "string"}},
				"required":             []any{"owner"},
				"additionalProperties": false,
			},
			"labels": map[string]any{
				"type":                 "object",
				"additionalProperties": map[string]any{"type": "string"},
			},
		},
		"required":             []any{"query", "count"},
		"additionalProperties": false,
	}
}

type reflectedInputSchemaFixture struct {
	Query string `json:"query" jsonschema_description:"query" jsonschema:"example=example query"`
	Mode  string `json:"mode,omitempty" jsonschema:"enum=fast,enum=slow,default=fast"`
}

type hiddenCycleSchemaValue struct {
	Next *hiddenCycleSchemaValue `json:"-"`
}

type opaqueSchemaValue struct {
	values []string
	calls  *int
}

func (v opaqueSchemaValue) MarshalJSON() ([]byte, error) {
	(*v.calls)++
	return []byte(`{"visible":"value"}`), nil
}

type hiddenCycleSchemaMarshaler struct {
	next  *hiddenCycleSchemaMarshaler
	calls *int
}

func (v *hiddenCycleSchemaMarshaler) MarshalJSON() ([]byte, error) {
	(*v.calls)++
	return []byte(`{"visible":"value"}`), nil
}

// compileToolInputSchema keeps the combined admissibility-and-compilation
// convenience local to tests. Production callers deliberately run the two
// stages separately so an admissible descriptor can remain publishable when
// bounded validator compilation is unavailable.
func compileToolInputSchema(schema map[string]any) (*compiledInputSchema, error) {
	_, _, validator, err := prepareToolInputSchema(schema)
	return validator, err
}

func TestCompileToolInputSchemaAcceptsCurrentReflectedSchema(t *testing.T) {
	compiled, err := compileToolInputSchema(ToInputSchema(reflectedInputSchemaFixture{}))
	require.NoError(t, err)
	require.NotNil(t, compiled)
	assert.NoError(t, compiled.validateArguments(`{"query":"example query","mode":"fast"}`))
}

func TestCompiledInputSchemaAcceptsSupportedVocabulary(t *testing.T) {
	compiled, err := compileToolInputSchema(supportedInputSchemaFixture())
	require.NoError(t, err)
	require.NotNil(t, compiled)

	for _, arguments := range []string{
		`{"query":"alpha","count":2}`,
		`{"query":"beta","count":1e3,"score":1.5,"enabled":true,"empty":null,"tags":["a","b"],"metadata":{"owner":"team"},"labels":{"region":"cn"}}`,
	} {
		assert.NoError(t, compiled.validateArguments(arguments), arguments)
	}
}

func TestCompiledInputSchemaRejectsInvalidArguments(t *testing.T) {
	compiled, err := compileToolInputSchema(supportedInputSchemaFixture())
	require.NoError(t, err)

	tests := []struct {
		name      string
		arguments string
		want      string
	}{
		{name: "missing required", arguments: `{"query":"alpha"}`, want: `required property "count" is missing`},
		{name: "wrong primitive", arguments: `{"query":"alpha","count":"2"}`, want: `expected integer`},
		{name: "fractional integer", arguments: `{"query":"alpha","count":2.5}`, want: `expected integer`},
		{name: "enum", arguments: `{"query":"gamma","count":2}`, want: `allowed enum`},
		{name: "array item", arguments: `{"query":"alpha","count":2,"tags":["ok",3]}`, want: `expected string`},
		{name: "nested required", arguments: `{"query":"alpha","count":2,"metadata":{}}`, want: `required property "owner" is missing`},
		{name: "nested additional", arguments: `{"query":"alpha","count":2,"metadata":{"owner":"team","extra":true}}`, want: `additional property "extra" is not allowed`},
		{name: "additional schema", arguments: `{"query":"alpha","count":2,"labels":{"region":3}}`, want: `expected string`},
		{name: "root additional", arguments: `{"query":"alpha","count":2,"other":true}`, want: `additional property "other" is not allowed`},
		{name: "non object", arguments: `[]`, want: `expected object`},
		{name: "malformed", arguments: `{`, want: `malformed JSON`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := compiled.validateArguments(test.arguments)
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.want)
		})
	}
}

func TestCompileToolInputSchemaRejectsUnsupportedOrMalformedSemantics(t *testing.T) {
	cyclic := map[string]any{"type": "object"}
	cyclic["properties"] = map[string]any{"self": cyclic}

	tests := []struct {
		name   string
		schema map[string]any
		want   string
	}{
		{name: "nil", schema: nil, want: "must be an object"},
		{name: "root primitive", schema: map[string]any{"type": "string"}, want: "root must declare"},
		{name: "unknown keyword", schema: map[string]any{"type": "object", "oneOf": []any{}}, want: `unsupported schema keyword "oneOf"`},
		{name: "reference", schema: map[string]any{"type": "object", "$ref": "#/$defs/input"}, want: `unsupported schema keyword "$ref"`},
		{name: "type array", schema: map[string]any{"type": []any{"object", "null"}}, want: "root must declare"},
		{name: "unknown type", schema: map[string]any{"type": "decimal"}, want: "root must declare"},
		{name: "properties array", schema: map[string]any{"type": "object", "properties": []any{}}, want: "properties: must be an object"},
		{name: "property boolean schema", schema: map[string]any{"type": "object", "properties": map[string]any{"x": true}}, want: "must be an object schema"},
		{name: "required non string", schema: map[string]any{"type": "object", "required": []any{1}}, want: "non-empty string"},
		{name: "required duplicate", schema: map[string]any{"type": "object", "required": []any{"x", "x"}}, want: "duplicate property"},
		{name: "empty enum", schema: map[string]any{"type": "object", "enum": []any{}}, want: "non-empty array"},
		{name: "numeric enum duplicate", schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"number": map[string]any{"type": "number", "enum": []any{1, 1.0}},
			},
		}, want: "duplicate value"},
		{name: "items on object", schema: map[string]any{"type": "object", "items": map[string]any{}}, want: `requires type "array"`},
		{name: "object keyword without object type", schema: map[string]any{"properties": map[string]any{}}, want: `root must declare type "object"`},
		{name: "bad additional properties", schema: map[string]any{"type": "object", "additionalProperties": "yes"}, want: "must be a boolean or object schema"},
		{name: "bad description", schema: map[string]any{"type": "object", "description": 1}, want: "must be a string"},
		{name: "bad examples", schema: map[string]any{"type": "object", "examples": "one"}, want: "must be an array"},
		{name: "cyclic schema", schema: cyclic, want: "cyclic JSON container"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := compileToolInputSchema(test.schema)
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.want)
		})
	}
}

func TestPrepareToolInputSchemaSeparatesDescriptorCaptureFromCompilation(t *testing.T) {
	reproduced := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"businessType": map[string]any{
				"type": "array",
				"enum": []any{"A", "B"},
			},
		},
	}

	descriptor, serializable, validator, err := prepareToolInputSchema(reproduced)
	require.True(t, serializable)
	require.Nil(t, validator)
	assert.Equal(t, reproduced, descriptor)

	var compilationError *schemaCompilationError
	require.ErrorAs(t, err, &compilationError)
	assert.Equal(t, schemaDiagnosticContradictoryConstraint, compilationError.reason)
}

func TestSchemaSnapshotRejectsOpaqueGraphsWithoutExecutingMarshalers(t *testing.T) {
	hiddenCycle := &hiddenCycleSchemaValue{}
	hiddenCycle.Next = hiddenCycle
	marshalCalls := 0
	opaque := opaqueSchemaValue{values: []string{"original"}, calls: &marshalCalls}
	customCycle := &hiddenCycleSchemaMarshaler{calls: &marshalCalls}
	customCycle.next = customCycle

	tests := []struct {
		name   string
		value  any
		reason schemaDiagnosticReason
	}{
		{name: "json ignored hidden cycle", value: hiddenCycle, reason: schemaDiagnosticSerializationFailure},
		{name: "custom marshaler hidden cycle", value: customCycle, reason: schemaDiagnosticSerializationFailure},
		{name: "custom marshaler with mutable hidden state", value: opaque, reason: schemaDiagnosticSerializationFailure},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var err error
			require.NotPanics(t, func() {
				_, _, _, err = prepareToolInputSchema(map[string]any{"type": "object", "opaque": test.value})
			})
			var compilationError *schemaCompilationError
			require.ErrorAs(t, err, &compilationError)
			assert.Equal(t, test.reason, compilationError.reason)
		})
	}
	assert.Zero(t, marshalCalls, "schema snapshotting must not execute custom MarshalJSON")
	opaque.values[0] = "mutated"
}

func TestSchemaSnapshotBoundsGenericContainerTraversal(t *testing.T) {
	t.Run("depth", func(t *testing.T) {
		var nested any = "leaf"
		for i := 0; i <= maxSchemaSnapshotDepth; i++ {
			nested = []any{nested}
		}
		_, _, _, err := prepareToolInputSchema(map[string]any{"type": "object", "opaque": nested})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "snapshot nesting exceeds")
	})

	t.Run("nodes", func(t *testing.T) {
		wide := make([]any, maxSchemaSnapshotItems/2)
		for index := range wide {
			wide[index] = []any{0, 1, 2, 3, 4, 5, 6, 7}
		}
		_, _, _, err := prepareToolInputSchema(map[string]any{"type": "object", "opaque": wide})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "snapshot exceeds")
	})
}

func TestPrepareToolInputSchemaRetainsBoundedValidatorPreparation(t *testing.T) {
	_, _, _, err := prepareToolInputSchema(map[string]any{"type": "string", "oneOf": []any{}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "root must declare")

	_, _, _, err = prepareToolInputSchema(map[string]any{
		"type": "object",
		"enum": append(make([]any, maxSchemaEnumSize), "overflow"),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds 256 values")

	t.Run("ordinary properties keys cannot bypass descriptor depth", func(t *testing.T) {
		adversarial := map[string]any{"leaf": true}
		for i := 0; i < maxSchemaDepth+2; i++ {
			adversarial = map[string]any{"properties": adversarial}
		}
		_, _, _, err := prepareToolInputSchema(map[string]any{
			"type":    "object",
			"unknown": adversarial,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "JSON container nesting exceeds")
	})

	t.Run("every JSON container contributes to the descriptor node bound", func(t *testing.T) {
		containers := make([]any, maxSchemaNodes)
		for i := range containers {
			containers[i] = map[string]any{}
		}
		_, _, _, err := prepareToolInputSchema(map[string]any{
			"type":    "object",
			"unknown": containers,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "JSON container nodes")
	})

	t.Run("old maximum nested object depth remains admissible and compilable", func(t *testing.T) {
		nested := map[string]any{"type": "string"}
		for i := 0; i < maxSchemaDepth; i++ {
			nested = map[string]any{
				"type":       "object",
				"properties": map[string]any{"child": nested},
			}
		}
		descriptor, serializable, _, err := prepareToolInputSchema(nested)
		require.NoError(t, err)
		require.True(t, serializable)
		require.NotNil(t, descriptor)
	})

	t.Run("one semantic schema level beyond the old depth limit is unavailable", func(t *testing.T) {
		nested := map[string]any{"type": "string"}
		for i := 0; i < maxSchemaDepth+1; i++ {
			nested = map[string]any{
				"type":       "object",
				"properties": map[string]any{"child": nested},
			}
		}
		_, _, _, err := prepareToolInputSchema(nested)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "schema nesting exceeds")
	})

	t.Run("old near-node-limit properties do not consume a second structural budget", func(t *testing.T) {
		properties := make(map[string]any, maxSchemaNodes-1)
		for i := 0; i < maxSchemaNodes-1; i++ {
			properties[fmt.Sprintf("property-%04d", i)] = map[string]any{
				"type":     "object",
				"required": []any{},
				"examples": []any{},
			}
		}
		schema := map[string]any{"type": "object", "properties": properties}
		descriptor, serializable, _, err := prepareToolInputSchema(schema)
		require.NoError(t, err)
		require.True(t, serializable)
		require.NotNil(t, descriptor)
	})
}

func TestCompileToolInputSchemaBoundsNestingAndArgumentSize(t *testing.T) {
	nested := map[string]any{"type": "string"}
	for i := 0; i < maxSchemaDepth+2; i++ {
		nested = map[string]any{"type": "array", "items": nested}
	}
	_, err := compileToolInputSchema(map[string]any{
		"type":       "object",
		"properties": map[string]any{"nested": nested},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "schema nesting exceeds")

	compiled, err := compileToolInputSchema(map[string]any{"type": "object"})
	require.NoError(t, err)
	err = compiled.validateArguments(`{"padding":"` + strings.Repeat("x", maxToolArgumentsBytes) + `"}`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "arguments exceed")
}

func TestJSONIntegerChecksStayBoundedForLargeExponents(t *testing.T) {
	for _, value := range []string{"1", "1.0", "1e3", "1.20e1", "1e999999999999999", "0e-999999999999999"} {
		assert.True(t, isJSONInteger(json.Number(value)), value)
	}
	for _, value := range []string{"1.2", "1e-1", "1e-999999999999999"} {
		assert.False(t, isJSONInteger(json.Number(value)), value)
	}
	_, comparable := comparableJSONNumber(json.Number("1e999999999999999"))
	assert.False(t, comparable)
}

func TestCompiledInputSchemaTreatsMissingArgumentsAsEmptyObject(t *testing.T) {
	optional, err := compileToolInputSchema(map[string]any{"type": "object"})
	require.NoError(t, err)
	assert.NoError(t, optional.validateArguments(""))

	required, err := compileToolInputSchema(map[string]any{
		"type":     "object",
		"required": []any{"query"},
	})
	require.NoError(t, err)
	err = required.validateArguments("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `required property "query" is missing`)
}
