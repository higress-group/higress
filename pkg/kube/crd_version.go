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
	"context"
	"fmt"
	"sort"
	"strings"

	crdmanifest "github.com/alibaba/higress/v2/api/kubernetes"
	apiExtensionsV1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiExtensionsV1Client "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

// CRDVersionInfo contains the expected CRD contract derived from the shipped manifest.
type CRDVersionInfo struct {
	Name            string
	ExpectedVersion string
	StorageSchema   *apiExtensionsV1.JSONSchemaProps
}

var optionalCRDFieldPaths = map[string][]string{}

// CheckCRDVersions checks if all required CRDs exist with correct versions
// Returns a list of warning messages if any issues are found
func CheckCRDVersions(config *rest.Config) []string {
	requiredCRDs, err := loadExpectedCRDContracts()
	if err != nil {
		return []string{fmt.Sprintf("Failed to load generated CRD contracts: %v", err)}
	}

	apiExtensionsClient, err := apiExtensionsV1Client.NewForConfig(config)
	if err != nil {
		return []string{fmt.Sprintf("Failed to create API extension client: %v", err)}
	}

	return checkCRDVersionsWithClient(apiExtensionsClient.CustomResourceDefinitions(), requiredCRDs, optionalCRDFieldPaths)
}

func loadExpectedCRDContracts() ([]CRDVersionInfo, error) {
	contracts, err := crdmanifest.LoadCRDContracts()
	if err != nil {
		return nil, err
	}

	requiredCRDs := make([]CRDVersionInfo, 0, len(contracts))
	for _, contract := range contracts {
		requiredCRDs = append(requiredCRDs, CRDVersionInfo{
			Name:            contract.Name,
			ExpectedVersion: contract.ExpectedVersion,
			StorageSchema:   contract.StorageSchema,
		})
	}

	return requiredCRDs, nil
}

func checkCRDVersionsWithClient(client apiExtensionsV1Client.CustomResourceDefinitionInterface, requiredCRDs []CRDVersionInfo, optionalFieldPaths map[string][]string) []string {
	warnings := []string{}

	crdList, err := client.List(context.TODO(), metaV1.ListOptions{})
	if err != nil {
		return []string{fmt.Sprintf("Failed to list CRDs: %v", err)}
	}

	crdMap := make(map[string]*apiExtensionsV1.CustomResourceDefinition)
	for i := range crdList.Items {
		crdMap[crdList.Items[i].Name] = &crdList.Items[i]
	}

	for _, required := range requiredCRDs {
		crd, exists := crdMap[required.Name]
		if !exists {
			warnings = append(warnings, fmt.Sprintf(
				"Required CRD '%s' not found. Please apply the Higress CRDs that match this build.",
				required.Name,
			))
			continue
		}

		storageVersion, found := getStorageVersion(crd)
		if !found {
			warnings = append(warnings, fmt.Sprintf(
				"CRD '%s' has no storage version configured. Current versions: %v. "+
					"Please update CRDs to the latest version.",
				required.Name, getCRDVersions(crd),
			))
			continue
		}

		if storageVersion.Name != required.ExpectedVersion {
			warnings = append(warnings, fmt.Sprintf(
				"CRD '%s' does not have expected storage version '%s'. "+
					"Current storage version is '%s'; available versions: %v. "+
					"Please update CRDs to the latest version.",
				required.Name, required.ExpectedVersion, storageVersion.Name, getCRDVersions(crd),
			))
			continue
		}

		if storageVersion.Schema == nil || storageVersion.Schema.OpenAPIV3Schema == nil {
			warnings = append(warnings, fmt.Sprintf(
				"CRD '%s' version '%s' has no schema configured; cannot verify the shipped Higress CRD contract. "+
					"Please update CRDs to enable schema validation.",
				required.Name, required.ExpectedVersion,
			))
			continue
		}

		if required.StorageSchema == nil {
			warnings = append(warnings, fmt.Sprintf(
				"The shipped CRD contract for '%s' version '%s' has no storage schema. "+
					"Please regenerate Higress CRD manifests for this build.",
				required.Name, required.ExpectedVersion,
			))
			continue
		}

		missingFields := findMissingSchemaPaths(required.StorageSchema, storageVersion.Schema.OpenAPIV3Schema, optionalFieldPaths[required.Name])
		if len(missingFields) > 0 {
			warnings = append(warnings, fmt.Sprintf(
				"CRD '%s' version '%s' is missing fields from the shipped Higress CRD schema: %v. "+
					"Please update CRDs to the latest version.",
				required.Name, required.ExpectedVersion, missingFields,
			))
		}
	}

	return warnings
}

func findMissingSchemaPaths(expectedSchema, liveSchema *apiExtensionsV1.JSONSchemaProps, ignoredPaths []string) []string {
	expectedPaths := collectComparableSchemaPaths(expectedSchema)
	missing := make([]string, 0, len(expectedPaths))

	for _, field := range expectedPaths {
		if isIgnoredPath(field, ignoredPaths) {
			continue
		}
		if !fieldExistsInSchema(liveSchema, field) {
			missing = append(missing, field)
		}
	}

	return missing
}
func collectComparableSchemaPaths(schema *apiExtensionsV1.JSONSchemaProps) []string {
	if schema == nil {
		return nil
	}

	specSchema, exists := schema.Properties["spec"]
	if !exists {
		return nil
	}

	paths := map[string]struct{}{}
	collectSchemaPathsRecursive(&specSchema, "spec", paths)

	collected := make([]string, 0, len(paths))
	for path := range paths {
		collected = append(collected, path)
	}
	sort.Strings(collected)
	return collected
}

func collectSchemaPathsRecursive(schema *apiExtensionsV1.JSONSchemaProps, path string, paths map[string]struct{}) {
	if schema == nil {
		return
	}

	if schema.XPreserveUnknownFields != nil && *schema.XPreserveUnknownFields {
		paths[path] = struct{}{}
		return
	}

	for name, prop := range schema.Properties {
		childPath := path + "." + name
		paths[childPath] = struct{}{}

		propCopy := prop
		if propCopy.Items != nil && propCopy.Items.Schema != nil {
			collectSchemaPathsRecursive(propCopy.Items.Schema, childPath, paths)
		}
		collectSchemaPathsRecursive(&propCopy, childPath, paths)
	}
}

func getStorageVersion(crd *apiExtensionsV1.CustomResourceDefinition) (*apiExtensionsV1.CustomResourceDefinitionVersion, bool) {
	for i := range crd.Spec.Versions {
		if crd.Spec.Versions[i].Storage {
			return &crd.Spec.Versions[i], true
		}
	}
	return nil, false
}

func isIgnoredPath(path string, ignoredPaths []string) bool {
	for _, ignored := range ignoredPaths {
		if path == ignored || strings.HasPrefix(path, ignored+".") {
			return true
		}
	}
	return false
}

// fieldExistsInSchema checks if a field path exists in the schema
// Field path format: "spec.fieldName" or "spec.nested.fieldName"
func fieldExistsInSchema(schema *apiExtensionsV1.JSONSchemaProps, fieldPath string) bool {
	// Check for empty field path first
	if fieldPath == "" {
		return false
	}

	if schema.Properties == nil {
		return false
	}

	// Parse field path (e.g., "spec.pluginName" -> ["spec", "pluginName"])
	parts := strings.Split(fieldPath, ".")
	current := schema

	for _, part := range parts {
		if current.Properties == nil {
			return false
		}

		prop, exists := current.Properties[part]
		if !exists {
			return false
		}
		current = &prop
	}

	return true
}

// getCRDVersions returns a list of version names for a CRD
func getCRDVersions(crd *apiExtensionsV1.CustomResourceDefinition) []string {
	versions := []string{}
	for _, v := range crd.Spec.Versions {
		versions = append(versions, v.Name)
	}
	return versions
}
