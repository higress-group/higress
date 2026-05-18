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
	"strings"

	apiExtensionsV1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiExtensionsClient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

// CRDVersionInfo contains expected CRD version information
type CRDVersionInfo struct {
	Name            string
	ExpectedVersion string
	RequiredFields  []string
	Description     string
}

// RequiredCRDs defines the CRDs required by Higress with their expected versions
//
// NOTE: This list should be kept in sync with:
//   - helm/core/crds/customresourcedefinitions.gen.yaml (CRD definitions)
//   - api/extensions/v1alpha1/*.pb.go (API definitions)
//   - api/networking/v1/*.pb.go (API definitions)
//
// When adding a new CRD:
//  1. Add the CRD definition to helm/core/crds/customresourcedefinitions.gen.yaml
//  2. Add the API definition to api/extensions/ or api/networking/
//  3. Add an entry here with the expected version and required fields
//  4. Update tests to verify the CRD
//
// CRD Information Sources:
//   - Name: From CRD metadata.name in helm/core/crds/customresourcedefinitions.gen.yaml
//   - ExpectedVersion: From CRD spec.versions[].name (the storage version)
//   - RequiredFields: From CRD spec.versions[].schema.openAPIV3Schema.properties
//   - Description: From API protobuf comments and CRD usage in code
var RequiredCRDs = []CRDVersionInfo{
	{
		Name:            "wasmplugins.extensions.higress.io",
		ExpectedVersion: "v1alpha1",
		RequiredFields:  []string{"spec.pluginName", "spec.url", "spec.matchRules"},
		Description:     "WasmPlugin for extending Higress functionality",
		// Source: api/extensions/v1alpha1/wasmplugin.pb.go
		// CRD: helm/core/crds/customresourcedefinitions.gen.yaml (line 7)
	},
	{
		Name:            "http2rpcs.networking.higress.io",
		ExpectedVersion: "v1",
		RequiredFields:  []string{"spec.dubbo", "spec.grpc"},
		Description:     "Http2Rpc for HTTP to RPC protocol conversion",
		// Source: api/networking/v1/http_2_rpc.pb.go
		// CRD: helm/core/crds/customresourcedefinitions.gen.yaml (line 150)
	},
	{
		Name:            "mcpbridges.networking.higress.io",
		ExpectedVersion: "v1",
		RequiredFields:  []string{"spec.registries", "spec.proxies"},
		Description:     "McpBridge for service registry integration (including Nacos 3 MCP Server)",
		// Source: api/networking/v1/mcp_bridge.pb.go
		// CRD: helm/core/crds/customresourcedefinitions.gen.yaml (line 237)
	},
}

// CheckCRDVersions checks if all required CRDs exist with correct versions
// Returns a list of warning messages if any issues are found
func CheckCRDVersions(config *rest.Config) []string {
	warnings := []string{}

	apiExtClientset, err := apiExtensionsClient.NewForConfig(config)
	if err != nil {
		return []string{fmt.Sprintf("Failed to create API extension client: %v", err)}
	}

	crdList, err := apiExtClientset.CustomResourceDefinitions().List(context.TODO(), metaV1.ListOptions{})
	if err != nil {
		return []string{fmt.Sprintf("Failed to list CRDs: %v", err)}
	}

	crdMap := make(map[string]*apiExtensionsV1.CustomResourceDefinition)
	for i := range crdList.Items {
		crdMap[crdList.Items[i].Name] = &crdList.Items[i]
	}

	for _, required := range RequiredCRDs {
		crd, exists := crdMap[required.Name]
		if !exists {
			warnings = append(warnings, fmt.Sprintf(
				"Required CRD '%s' not found. %s. Please apply the latest CRDs.",
				required.Name, required.Description,
			))
			continue
		}

		// Check if expected version exists
		versionFound := false
		for _, version := range crd.Spec.Versions {
			if version.Name == required.ExpectedVersion {
				versionFound = true

				// Check for required fields in schema
				if version.Schema != nil && version.Schema.OpenAPIV3Schema != nil {
					missingFields := checkRequiredFields(version.Schema.OpenAPIV3Schema, required.RequiredFields)
					if len(missingFields) > 0 {
						warnings = append(warnings, fmt.Sprintf(
							"CRD '%s' version '%s' is missing required fields: %v. "+
								"Please update CRDs to the latest version.",
							required.Name, required.ExpectedVersion, missingFields,
						))
					}
				} else if len(required.RequiredFields) > 0 {
					// Schema is nil but we have required fields to check
					warnings = append(warnings, fmt.Sprintf(
						"CRD '%s' version '%s' has no schema configured; cannot verify required fields: %v. "+
							"Please update CRDs to enable schema validation.",
						required.Name, required.ExpectedVersion, required.RequiredFields,
					))
				}
				break
			}
		}

		if !versionFound {
			warnings = append(warnings, fmt.Sprintf(
				"CRD '%s' does not have expected version '%s'. "+
					"Current versions: %v. Please update CRDs to the latest version.",
				required.Name, required.ExpectedVersion, getCRDVersions(crd),
			))
		}
	}

	return warnings
}

// checkRequiredFields checks if required fields exist in the schema
func checkRequiredFields(schema *apiExtensionsV1.JSONSchemaProps, requiredFields []string) []string {
	missing := []string{}

	for _, field := range requiredFields {
		if !fieldExistsInSchema(schema, field) {
			missing = append(missing, field)
		}
	}

	return missing
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
