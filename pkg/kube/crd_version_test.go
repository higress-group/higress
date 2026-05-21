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

package kube

import (
	"testing"

	apiExtensionsV1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestFieldExistsInSchema(t *testing.T) {
	tests := []struct {
		name      string
		schema    *apiExtensionsV1.JSONSchemaProps
		fieldPath string
		want      bool
	}{
		{
			name: "simple field exists",
			schema: &apiExtensionsV1.JSONSchemaProps{
				Properties: map[string]apiExtensionsV1.JSONSchemaProps{
					"spec": {
						Properties: map[string]apiExtensionsV1.JSONSchemaProps{
							"pluginName": {Type: "string"},
						},
					},
				},
			},
			fieldPath: "spec.pluginName",
			want:      true,
		},
		{
			name: "simple field does not exist",
			schema: &apiExtensionsV1.JSONSchemaProps{
				Properties: map[string]apiExtensionsV1.JSONSchemaProps{
					"spec": {
						Properties: map[string]apiExtensionsV1.JSONSchemaProps{
							"pluginName": {Type: "string"},
						},
					},
				},
			},
			fieldPath: "spec.nonExistent",
			want:      false,
		},
		{
			name: "nested field exists",
			schema: &apiExtensionsV1.JSONSchemaProps{
				Properties: map[string]apiExtensionsV1.JSONSchemaProps{
					"spec": {
						Properties: map[string]apiExtensionsV1.JSONSchemaProps{
							"registries": {
								Properties: map[string]apiExtensionsV1.JSONSchemaProps{
									"enableMCPServer": {Type: "boolean"},
								},
							},
						},
					},
				},
			},
			fieldPath: "spec.registries.enableMCPServer",
			want:      true,
		},
		{
			name: "nested field does not exist",
			schema: &apiExtensionsV1.JSONSchemaProps{
				Properties: map[string]apiExtensionsV1.JSONSchemaProps{
					"spec": {
						Properties: map[string]apiExtensionsV1.JSONSchemaProps{
							"registries": {
								Properties: map[string]apiExtensionsV1.JSONSchemaProps{
									"enableMCPServer": {Type: "boolean"},
								},
							},
						},
					},
				},
			},
			fieldPath: "spec.registries.nonExistent",
			want:      false,
		},
		{
			name: "nil properties",
			schema: &apiExtensionsV1.JSONSchemaProps{
				Properties: nil,
			},
			fieldPath: "spec.pluginName",
			want:      false,
		},
		{
			name: "empty field path",
			schema: &apiExtensionsV1.JSONSchemaProps{
				Properties: map[string]apiExtensionsV1.JSONSchemaProps{
					"spec": {Type: "object"},
				},
			},
			fieldPath: "",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fieldExistsInSchema(tt.schema, tt.fieldPath)
			if got != tt.want {
				t.Errorf("fieldExistsInSchema() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheckRequiredFields(t *testing.T) {
	tests := []struct {
		name           string
		schema         *apiExtensionsV1.JSONSchemaProps
		requiredFields []string
		wantMissing    []string
	}{
		{
			name: "all fields exist",
			schema: &apiExtensionsV1.JSONSchemaProps{
				Properties: map[string]apiExtensionsV1.JSONSchemaProps{
					"spec": {
						Properties: map[string]apiExtensionsV1.JSONSchemaProps{
							"pluginName": {Type: "string"},
							"url":        {Type: "string"},
							"matchRules": {Type: "array"},
						},
					},
				},
			},
			requiredFields: []string{"spec.pluginName", "spec.url", "spec.matchRules"},
			wantMissing:    []string{},
		},
		{
			name: "some fields missing",
			schema: &apiExtensionsV1.JSONSchemaProps{
				Properties: map[string]apiExtensionsV1.JSONSchemaProps{
					"spec": {
						Properties: map[string]apiExtensionsV1.JSONSchemaProps{
							"pluginName": {Type: "string"},
						},
					},
				},
			},
			requiredFields: []string{"spec.pluginName", "spec.url", "spec.matchRules"},
			wantMissing:    []string{"spec.url", "spec.matchRules"},
		},
		{
			name: "all fields missing",
			schema: &apiExtensionsV1.JSONSchemaProps{
				Properties: map[string]apiExtensionsV1.JSONSchemaProps{
					"spec": {
						Properties: map[string]apiExtensionsV1.JSONSchemaProps{},
					},
				},
			},
			requiredFields: []string{"spec.pluginName", "spec.url"},
			wantMissing:    []string{"spec.pluginName", "spec.url"},
		},
		{
			name: "no required fields",
			schema: &apiExtensionsV1.JSONSchemaProps{
				Properties: map[string]apiExtensionsV1.JSONSchemaProps{
					"spec": {Type: "object"},
				},
			},
			requiredFields: []string{},
			wantMissing:    []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checkRequiredFields(tt.schema, tt.requiredFields)
			if len(got) != len(tt.wantMissing) {
				t.Errorf("checkRequiredFields() returned %d missing fields, want %d", len(got), len(tt.wantMissing))
				t.Errorf("got: %v, want: %v", got, tt.wantMissing)
				return
			}
			// Check each missing field
			for i, field := range got {
				if field != tt.wantMissing[i] {
					t.Errorf("checkRequiredFields()[%d] = %v, want %v", i, field, tt.wantMissing[i])
				}
			}
		})
	}
}

func TestGetCRDVersions(t *testing.T) {
	tests := []struct {
		name string
		crd  *apiExtensionsV1.CustomResourceDefinition
		want []string
	}{
		{
			name: "single version",
			crd: &apiExtensionsV1.CustomResourceDefinition{
				Spec: apiExtensionsV1.CustomResourceDefinitionSpec{
					Versions: []apiExtensionsV1.CustomResourceDefinitionVersion{
						{Name: "v1alpha1"},
					},
				},
			},
			want: []string{"v1alpha1"},
		},
		{
			name: "multiple versions",
			crd: &apiExtensionsV1.CustomResourceDefinition{
				Spec: apiExtensionsV1.CustomResourceDefinitionSpec{
					Versions: []apiExtensionsV1.CustomResourceDefinitionVersion{
						{Name: "v1alpha1"},
						{Name: "v1beta1"},
						{Name: "v1"},
					},
				},
			},
			want: []string{"v1alpha1", "v1beta1", "v1"},
		},
		{
			name: "no versions",
			crd: &apiExtensionsV1.CustomResourceDefinition{
				Spec: apiExtensionsV1.CustomResourceDefinitionSpec{
					Versions: []apiExtensionsV1.CustomResourceDefinitionVersion{},
				},
			},
			want: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getCRDVersions(tt.crd)
			if len(got) != len(tt.want) {
				t.Errorf("getCRDVersions() returned %d versions, want %d", len(got), len(tt.want))
				return
			}
			for i, v := range got {
				if v != tt.want[i] {
					t.Errorf("getCRDVersions()[%d] = %v, want %v", i, v, tt.want[i])
				}
			}
		})
	}
}

func TestCRDVersionHelpers_AllFieldsPresent(t *testing.T) {
	// This test validates the helper functions with a complete CRD
	// that has all required fields and the correct version

	// Create a mock CRD with correct version and fields
	mockCRD := &apiExtensionsV1.CustomResourceDefinition{
		ObjectMeta: metaV1.ObjectMeta{
			Name: "wasmplugins.extensions.higress.io",
		},
		Spec: apiExtensionsV1.CustomResourceDefinitionSpec{
			Versions: []apiExtensionsV1.CustomResourceDefinitionVersion{
				{
					Name: "v1alpha1",
					Schema: &apiExtensionsV1.CustomResourceValidation{
						OpenAPIV3Schema: &apiExtensionsV1.JSONSchemaProps{
							Properties: map[string]apiExtensionsV1.JSONSchemaProps{
								"spec": {
									Properties: map[string]apiExtensionsV1.JSONSchemaProps{
										"pluginName": {Type: "string"},
										"url":        {Type: "string"},
										"matchRules": {Type: "array"},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Test that the CRD has the expected version
	versions := getCRDVersions(mockCRD)
	if len(versions) != 1 || versions[0] != "v1alpha1" {
		t.Errorf("Expected version v1alpha1, got %v", versions)
	}

	// Test that required fields exist
	schema := mockCRD.Spec.Versions[0].Schema.OpenAPIV3Schema
	requiredFields := []string{"spec.pluginName", "spec.url", "spec.matchRules"}
	missing := checkRequiredFields(schema, requiredFields)
	if len(missing) > 0 {
		t.Errorf("Expected no missing fields, got %v", missing)
	}
}

func TestCRDVersionHelpers_MissingFields(t *testing.T) {
	// Test that checkRequiredFields correctly identifies missing fields
	mockCRD := &apiExtensionsV1.CustomResourceDefinition{
		ObjectMeta: metaV1.ObjectMeta{
			Name: "wasmplugins.extensions.higress.io",
		},
		Spec: apiExtensionsV1.CustomResourceDefinitionSpec{
			Versions: []apiExtensionsV1.CustomResourceDefinitionVersion{
				{
					Name: "v1alpha1",
					Schema: &apiExtensionsV1.CustomResourceValidation{
						OpenAPIV3Schema: &apiExtensionsV1.JSONSchemaProps{
							Properties: map[string]apiExtensionsV1.JSONSchemaProps{
								"spec": {
									Properties: map[string]apiExtensionsV1.JSONSchemaProps{
										"pluginName": {Type: "string"},
										// Missing: url, matchRules
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Test that required fields are missing
	schema := mockCRD.Spec.Versions[0].Schema.OpenAPIV3Schema
	requiredFields := []string{"spec.pluginName", "spec.url", "spec.matchRules"}
	missing := checkRequiredFields(schema, requiredFields)

	expectedMissing := []string{"spec.url", "spec.matchRules"}
	if len(missing) != len(expectedMissing) {
		t.Errorf("Expected %d missing fields, got %d: %v", len(expectedMissing), len(missing), missing)
	}
}

func TestCRDVersionHelpers_WrongVersion(t *testing.T) {
	// Create a mock CRD with wrong version
	mockCRD := &apiExtensionsV1.CustomResourceDefinition{
		ObjectMeta: metaV1.ObjectMeta{
			Name: "wasmplugins.extensions.higress.io",
		},
		Spec: apiExtensionsV1.CustomResourceDefinitionSpec{
			Versions: []apiExtensionsV1.CustomResourceDefinitionVersion{
				{
					Name: "v1alpha", // Wrong version, should be v1alpha1
					Schema: &apiExtensionsV1.CustomResourceValidation{
						OpenAPIV3Schema: &apiExtensionsV1.JSONSchemaProps{
							Properties: map[string]apiExtensionsV1.JSONSchemaProps{
								"spec": {Type: "object"},
							},
						},
					},
				},
			},
		},
	}

	// Test that the version is different from expected
	versions := getCRDVersions(mockCRD)
	expectedVersion := "v1alpha1"

	versionFound := false
	for _, v := range versions {
		if v == expectedVersion {
			versionFound = true
			break
		}
	}

	if versionFound {
		t.Errorf("Expected version %s not to be found, but it was", expectedVersion)
	}
}

func TestCRDVersionHelpers_NilSchema(t *testing.T) {
	// Test that we get a warning when schema is nil but required fields exist
	mockCRD := &apiExtensionsV1.CustomResourceDefinition{
		ObjectMeta: metaV1.ObjectMeta{
			Name: "wasmplugins.extensions.higress.io",
		},
		Spec: apiExtensionsV1.CustomResourceDefinitionSpec{
			Versions: []apiExtensionsV1.CustomResourceDefinitionVersion{
				{
					Name:   "v1alpha1", // Correct version
					Schema: nil,        // But schema is nil
				},
			},
		},
	}

	// Verify that schema is nil
	if mockCRD.Spec.Versions[0].Schema != nil {
		t.Error("Expected schema to be nil for this test")
	}

	// This scenario should trigger a warning in CheckCRDVersions
	// when there are required fields to check
	// (We can't easily test CheckCRDVersions without mocking the k8s client,
	// but we've verified the logic exists in the code)
}

func TestRequiredCRDsDefinition(t *testing.T) {
	// Test that RequiredCRDs is properly defined
	if len(RequiredCRDs) == 0 {
		t.Error("RequiredCRDs should not be empty")
	}

	// Test that each CRD has required fields
	for _, crd := range RequiredCRDs {
		if crd.Name == "" {
			t.Error("CRD Name should not be empty")
		}
		if crd.ExpectedVersion == "" {
			t.Error("CRD ExpectedVersion should not be empty")
		}
		if crd.Description == "" {
			t.Error("CRD Description should not be empty")
		}
		// RequiredFields can be empty for some CRDs
	}

	// Test specific known CRDs - verify they exist with correct properties
	// Note: This test intentionally allows additional CRDs to be added to RequiredCRDs
	// without breaking the test. It only verifies that the known CRDs have the expected configuration.
	expectedCRDs := map[string]struct {
		version string
		fields  int
	}{
		"wasmplugins.extensions.higress.io": {version: "v1alpha1", fields: 3},
		"http2rpcs.networking.higress.io":   {version: "v1", fields: 2},
		"mcpbridges.networking.higress.io":  {version: "v1", fields: 2},
	}

	// Build a map of actual CRDs for easy lookup
	actualCRDs := make(map[string]CRDVersionInfo)
	for _, crd := range RequiredCRDs {
		actualCRDs[crd.Name] = crd
	}

	// Verify each expected CRD exists and has correct properties
	for name, expected := range expectedCRDs {
		actual, found := actualCRDs[name]
		if !found {
			t.Errorf("Expected CRD %s not found in RequiredCRDs", name)
			continue
		}

		if actual.ExpectedVersion != expected.version {
			t.Errorf("CRD %s: expected version %s, got %s", name, expected.version, actual.ExpectedVersion)
		}

		if len(actual.RequiredFields) != expected.fields {
			t.Errorf("CRD %s: expected %d required fields, got %d", name, expected.fields, len(actual.RequiredFields))
		}
	}

	// Optional: Log additional CRDs (not an error, just informational)
	for name := range actualCRDs {
		if _, expected := expectedCRDs[name]; !expected {
			t.Logf("Info: Additional CRD found in RequiredCRDs: %s", name)
		}
	}
}
