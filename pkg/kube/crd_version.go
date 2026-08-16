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
	RequiredFields  []string
}

// criticalCRDFieldPaths lists the spec field paths whose presence is
// feature-critical for this build, per CRD. These are the only fields that
// startup validation actively verifies against the live storage schema.
//
// optionalCRDFieldPaths lists the remaining shipped spec field paths that are
// deliberately excluded from validation.
//
// Maintenance contract (enforced by TestCriticalAndOptionalCRDFieldPaths_ClassifyAllShippedSpecFields):
// the union of criticalCRDFieldPaths and optionalCRDFieldPaths for each CRD
// must contain every spec.* field path present in the shipped storage schema
// (customresourcedefinitions.gen.yaml), and nothing more. Adding a new field
// to the generated manifest therefore forces a classification decision here —
// either promote it to criticalCRDFieldPaths or explicitly acknowledge it in
// optionalCRDFieldPaths — so the critical set cannot silently drift out of
// sync with the shipped schema.
var criticalCRDFieldPaths = map[string][]string{
	"wasmplugins.extensions.higress.io": {"spec.pluginName", "spec.url", "spec.matchRules"},
	"http2rpcs.networking.higress.io":   {"spec.dubbo", "spec.grpc"},
	"mcpbridges.networking.higress.io":  {"spec.registries", "spec.proxies"},
}

var optionalCRDFieldPaths = map[string][]string{
	"wasmplugins.extensions.higress.io": {
		"spec.defaultConfig", "spec.defaultConfigDisable", "spec.failStrategy",
		"spec.imagePullPolicy", "spec.imagePullSecret",
		"spec.matchRules.config", "spec.matchRules.configDisable",
		"spec.matchRules.domain", "spec.matchRules.ingress",
		"spec.matchRules.routeType", "spec.matchRules.service",
		"spec.phase", "spec.pluginConfig", "spec.priority", "spec.sha256",
		"spec.verificationKey",
		"spec.vmConfig", "spec.vmConfig.env",
		"spec.vmConfig.env.name", "spec.vmConfig.env.value", "spec.vmConfig.env.valueFrom",
	},
	"http2rpcs.networking.higress.io": {
		"spec.dubbo.group", "spec.dubbo.methods",
		"spec.dubbo.methods.headersAttach", "spec.dubbo.methods.httpMethods",
		"spec.dubbo.methods.httpPath", "spec.dubbo.methods.paramFromEntireBody",
		"spec.dubbo.methods.paramFromEntireBody.paramType",
		"spec.dubbo.methods.params",
		"spec.dubbo.methods.params.paramKey", "spec.dubbo.methods.params.paramSource",
		"spec.dubbo.methods.params.paramType",
		"spec.dubbo.methods.serviceMethod",
		"spec.dubbo.service", "spec.dubbo.version",
	},
	"mcpbridges.networking.higress.io": {
		"spec.proxies.connectTimeout", "spec.proxies.listenerPort",
		"spec.proxies.name", "spec.proxies.serverAddress",
		"spec.proxies.serverPort", "spec.proxies.type",
		"spec.registries.allowMcpServers", "spec.registries.authSecretName",
		"spec.registries.consulDatacenter", "spec.registries.consulNamespace",
		"spec.registries.consulRefreshInterval", "spec.registries.consulServiceTag",
		"spec.registries.domain", "spec.registries.enableMCPServer",
		"spec.registries.enableScopeMcpServers", "spec.registries.mcpServerBaseUrl",
		"spec.registries.mcpServerExportDomains", "spec.registries.metadata",
		"spec.registries.nacosAccessKey", "spec.registries.nacosAddressServer",
		"spec.registries.nacosGroups", "spec.registries.nacosNamespace",
		"spec.registries.nacosNamespaceId", "spec.registries.nacosRefreshInterval",
		"spec.registries.nacosSecretKey", "spec.registries.nacosTimeout",
		"spec.registries.name",
		"spec.registries.port", "spec.registries.protocol",
		"spec.registries.proxyName", "spec.registries.sni",
		"spec.registries.type", "spec.registries.vport",
		"spec.registries.vport.default", "spec.registries.vport.services",
		"spec.registries.vport.services.name", "spec.registries.vport.services.value",
		"spec.registries.zkServicesPath",
	},
}

// CheckCRDVersions checks if all required CRDs exist with correct versions
// Returns a list of warning messages if any issues are found.
//
// The ctx bounds the CRD list request: a stalled API server (e.g. during etcd
// or kube-apiserver degradation) cannot block server startup indefinitely, and
// the caller is expected to cancel it when the server is asked to stop. Since
// this is a diagnostic warning check, a cancelled/expired context yields a
// single "skipped" warning rather than failing startup.
func CheckCRDVersions(ctx context.Context, config *rest.Config) []string {
	requiredCRDs, err := loadExpectedCRDContracts()
	if err != nil {
		return []string{fmt.Sprintf("Failed to load generated CRD contracts: %v", err)}
	}

	apiExtensionsClient, err := apiExtensionsV1Client.NewForConfig(config)
	if err != nil {
		return []string{fmt.Sprintf("Failed to create API extension client: %v", err)}
	}

	return checkCRDVersionsWithClient(ctx, apiExtensionsClient.CustomResourceDefinitions(), requiredCRDs, optionalCRDFieldPaths)
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
			RequiredFields:  criticalCRDFieldPaths[contract.Name],
		})
	}

	return requiredCRDs, nil
}

func checkCRDVersionsWithClient(ctx context.Context, client apiExtensionsV1Client.CustomResourceDefinitionInterface, requiredCRDs []CRDVersionInfo, optionalFieldPaths map[string][]string) []string {
	warnings := []string{}

	// Fail fast when the caller has already cancelled or expired the context,
	// so a degraded API server cannot stall startup before the list request is
	// even issued.
	if err := ctx.Err(); err != nil {
		return []string{fmt.Sprintf("CRD version check skipped: %v", err)}
	}

	crdList, err := client.List(ctx, metaV1.ListOptions{})
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

		if len(required.RequiredFields) == 0 {
			continue
		}

		missingFields := findMissingRequiredFields(required.RequiredFields, storageVersion.Schema.OpenAPIV3Schema, optionalFieldPaths[required.Name])
		if len(missingFields) > 0 {
			warnings = append(warnings, fmt.Sprintf(
				"CRD '%s' version '%s' is missing required fields: %v. "+
					"Please update CRDs to the latest version.",
				required.Name, required.ExpectedVersion, missingFields,
			))
		}
	}

	return warnings
}

func findMissingRequiredFields(requiredFields []string, liveSchema *apiExtensionsV1.JSONSchemaProps, ignoredPaths []string) []string {
	livePathSet := collectComparableSchemaPathSet(liveSchema)
	missing := make([]string, 0, len(requiredFields))

	for _, field := range requiredFields {
		if isIgnoredPath(field, ignoredPaths) {
			continue
		}
		if _, exists := livePathSet[field]; !exists {
			missing = append(missing, field)
		}
	}

	sort.Strings(missing)
	return missing
}

func collectComparableSchemaPathSet(schema *apiExtensionsV1.JSONSchemaProps) map[string]struct{} {
	if schema == nil {
		return map[string]struct{}{}
	}

	specSchema, exists := schema.Properties["spec"]
	if !exists {
		return map[string]struct{}{}
	}

	paths := map[string]struct{}{}
	collectSchemaPathsRecursive(&specSchema, "spec", paths)
	return paths
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

// getCRDVersions returns a list of version names for a CRD
func getCRDVersions(crd *apiExtensionsV1.CustomResourceDefinition) []string {
	versions := []string{}
	for _, v := range crd.Spec.Versions {
		versions = append(versions, v.Name)
	}
	return versions
}
