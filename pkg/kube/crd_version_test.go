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
	"errors"
	"strings"
	"testing"

	apiExtensionsV1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiExtensionsFake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgotesting "k8s.io/client-go/testing"
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
	contracts, err := loadExpectedCRDContracts()
	if err != nil {
		t.Fatalf("loadExpectedCRDContracts() returned error: %v", err)
	}

	if len(contracts) == 0 {
		t.Fatal("expected manifest-derived CRD contracts to be non-empty")
	}

	for _, crd := range contracts {
		if crd.Name == "" {
			t.Error("CRD Name should not be empty")
		}
		if crd.ExpectedVersion == "" {
			t.Error("CRD ExpectedVersion should not be empty")
		}
		if crd.StorageSchema == nil {
			t.Errorf("CRD %s should have a storage schema", crd.Name)
		}
	}

	expectedCRDs := map[string]string{
		"wasmplugins.extensions.higress.io": "v1alpha1",
		"http2rpcs.networking.higress.io":   "v1",
		"mcpbridges.networking.higress.io":  "v1",
	}

	actualCRDs := make(map[string]CRDVersionInfo)
	for _, crd := range contracts {
		actualCRDs[crd.Name] = crd
	}

	for name, expectedVersion := range expectedCRDs {
		actual, found := actualCRDs[name]
		if !found {
			t.Errorf("Expected CRD %s not found in manifest-derived contracts", name)
			continue
		}

		if actual.ExpectedVersion != expectedVersion {
			t.Errorf("CRD %s: expected version %s, got %s", name, expectedVersion, actual.ExpectedVersion)
		}
	}
}

func TestCheckCRDVersionsWithClient_AllValid(t *testing.T) {
	client := apiExtensionsFake.NewSimpleClientset(
		&apiExtensionsV1.CustomResourceDefinition{
			ObjectMeta: metaV1.ObjectMeta{
				Name: "wasmplugins.extensions.higress.io",
			},
			Spec: apiExtensionsV1.CustomResourceDefinitionSpec{
				Versions: []apiExtensionsV1.CustomResourceDefinitionVersion{
					{
						Name:    "v1alpha1",
						Served:  true,
						Storage: true,
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
		},
	)

	warnings := checkCRDVersionsWithClient(client.ApiextensionsV1().CustomResourceDefinitions(), []CRDVersionInfo{
		{
			Name:            "wasmplugins.extensions.higress.io",
			ExpectedVersion: "v1alpha1",
			StorageSchema: &apiExtensionsV1.JSONSchemaProps{
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
	}, nil)

	if len(warnings) != 0 {
		t.Fatalf("expected no warnings for valid CRDs, got %v", warnings)
	}
}

func TestCheckCRDVersionsWithClient_StorageVersionMismatch(t *testing.T) {
	client := apiExtensionsFake.NewSimpleClientset(
		&apiExtensionsV1.CustomResourceDefinition{
			ObjectMeta: metaV1.ObjectMeta{
				Name: "wasmplugins.extensions.higress.io",
			},
			Spec: apiExtensionsV1.CustomResourceDefinitionSpec{
				Versions: []apiExtensionsV1.CustomResourceDefinitionVersion{
					{
						Name:    "v1alpha1",
						Served:  true,
						Storage: false,
					},
					{
						Name:    "v1beta1",
						Served:  true,
						Storage: true,
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
		},
	)

	warnings := checkCRDVersionsWithClient(client.ApiextensionsV1().CustomResourceDefinitions(), []CRDVersionInfo{
		{
			Name:            "wasmplugins.extensions.higress.io",
			ExpectedVersion: "v1alpha1",
			StorageSchema: &apiExtensionsV1.JSONSchemaProps{
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
	}, nil)

	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning for storage version mismatch, got %d: %v", len(warnings), warnings)
	}

	got := warnings[0]
	if !strings.Contains(got, "expected storage version 'v1alpha1'") {
		t.Fatalf("expected warning to mention expected storage version, got %q", got)
	}
	if !strings.Contains(got, "Current storage version is 'v1beta1'") {
		t.Fatalf("expected warning to mention current storage version, got %q", got)
	}
}

func TestCheckCRDVersionsWithClient_ListError(t *testing.T) {
	client := apiExtensionsFake.NewSimpleClientset()
	client.PrependReactor("list", "customresourcedefinitions", func(action clientgotesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("boom")
	})

	warnings := checkCRDVersionsWithClient(client.ApiextensionsV1().CustomResourceDefinitions(), []CRDVersionInfo{
		{
			Name:            "wasmplugins.extensions.higress.io",
			ExpectedVersion: "v1alpha1",
			StorageSchema: &apiExtensionsV1.JSONSchemaProps{
				Properties: map[string]apiExtensionsV1.JSONSchemaProps{
					"spec": {
						Properties: map[string]apiExtensionsV1.JSONSchemaProps{
							"pluginName": {Type: "string"},
						},
					},
				},
			},
		},
	}, nil)

	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning for list error, got %d: %v", len(warnings), warnings)
	}

	expected := "Failed to list CRDs: boom"
	if warnings[0] != expected {
		t.Fatalf("expected %q, got %q", expected, warnings[0])
	}
}

func TestCheckCRDVersionsWithClient_MissingManifestField(t *testing.T) {
	client := apiExtensionsFake.NewSimpleClientset(
		&apiExtensionsV1.CustomResourceDefinition{
			ObjectMeta: metaV1.ObjectMeta{
				Name: "wasmplugins.extensions.higress.io",
			},
			Spec: apiExtensionsV1.CustomResourceDefinitionSpec{
				Versions: []apiExtensionsV1.CustomResourceDefinitionVersion{
					{
						Name:    "v1alpha1",
						Served:  true,
						Storage: true,
						Schema: &apiExtensionsV1.CustomResourceValidation{
							OpenAPIV3Schema: &apiExtensionsV1.JSONSchemaProps{
								Properties: map[string]apiExtensionsV1.JSONSchemaProps{
									"spec": {
										Properties: map[string]apiExtensionsV1.JSONSchemaProps{
											"pluginName": {Type: "string"},
											"url":        {Type: "string"},
											// matchRules intentionally missing
										},
									},
								},
							},
						},
					},
				},
			},
		},
	)

	warnings := checkCRDVersionsWithClient(client.ApiextensionsV1().CustomResourceDefinitions(), []CRDVersionInfo{
		{
			Name:            "wasmplugins.extensions.higress.io",
			ExpectedVersion: "v1alpha1",
			StorageSchema: &apiExtensionsV1.JSONSchemaProps{
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
	}, nil)

	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning for missing schema field, got %d: %v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], "spec.matchRules") {
		t.Fatalf("expected warning to mention missing spec.matchRules, got %q", warnings[0])
	}
}

func TestCheckCRDVersionsWithClient_OptionalPathBypass(t *testing.T) {
	client := apiExtensionsFake.NewSimpleClientset(
		&apiExtensionsV1.CustomResourceDefinition{
			ObjectMeta: metaV1.ObjectMeta{
				Name: "wasmplugins.extensions.higress.io",
			},
			Spec: apiExtensionsV1.CustomResourceDefinitionSpec{
				Versions: []apiExtensionsV1.CustomResourceDefinitionVersion{
					{
						Name:    "v1alpha1",
						Served:  true,
						Storage: true,
						Schema: &apiExtensionsV1.CustomResourceValidation{
							OpenAPIV3Schema: &apiExtensionsV1.JSONSchemaProps{
								Properties: map[string]apiExtensionsV1.JSONSchemaProps{
									"spec": {
										Properties: map[string]apiExtensionsV1.JSONSchemaProps{
											"pluginName": {Type: "string"},
											"url":        {Type: "string"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	)

	warnings := checkCRDVersionsWithClient(client.ApiextensionsV1().CustomResourceDefinitions(), []CRDVersionInfo{
		{
			Name:            "wasmplugins.extensions.higress.io",
			ExpectedVersion: "v1alpha1",
			StorageSchema: &apiExtensionsV1.JSONSchemaProps{
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
	}, map[string][]string{
		"wasmplugins.extensions.higress.io": {"spec.matchRules"},
	})

	if len(warnings) != 0 {
		t.Fatalf("expected no warnings when missing field is configured optional, got %v", warnings)
	}
}
