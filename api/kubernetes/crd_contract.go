package kubernetes

import (
	"bytes"
	"embed"
	"fmt"
	"io"
	"sync"

	apiExtensionsV1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
)

//go:embed customresourcedefinitions.gen.yaml
var customResourceDefinitionsYAML embed.FS

type CRDContract struct {
	Name            string
	ExpectedVersion string
}

var (
	loadContractsOnce sync.Once
	cachedContracts   []CRDContract
	cachedSchemas     map[string]*apiExtensionsV1.JSONSchemaProps
	cachedErr         error
)

func LoadCRDContracts() ([]CRDContract, error) {
	loadContractsOnce.Do(loadCRDContracts)
	if cachedErr != nil {
		return nil, cachedErr
	}
	return cachedContracts, nil
}

// LoadCRDStorageSchemas returns the storage-version OpenAPI v3 schema for each
// shipped CRD, keyed by CRD name. It is cached after the first call together
// with the CRD contracts. A CRD whose storage version has no schema is present
// in the map with a nil value.
func LoadCRDStorageSchemas() (map[string]*apiExtensionsV1.JSONSchemaProps, error) {
	loadContractsOnce.Do(loadCRDContracts)
	if cachedErr != nil {
		return nil, cachedErr
	}
	return cachedSchemas, nil
}

func loadCRDContracts() {
	data, err := customResourceDefinitionsYAML.ReadFile("customresourcedefinitions.gen.yaml")
	if err != nil {
		cachedErr = fmt.Errorf("read generated CRD manifest: %w", err)
		return
	}

	decoder := utilyaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)
	contracts := make([]CRDContract, 0, 4)
	schemas := make(map[string]*apiExtensionsV1.JSONSchemaProps, 4)

	for {
		var crd apiExtensionsV1.CustomResourceDefinition
		if err := decoder.Decode(&crd); err != nil {
			if err == io.EOF {
				break
			}
			cachedErr = fmt.Errorf("decode generated CRD manifest: %w", err)
			return
		}
		if crd.Name == "" {
			continue
		}

		storageVersion, found := storageVersionFromDefinition(&crd)
		if !found {
			cachedErr = fmt.Errorf("crd %s has no storage version in generated manifest", crd.Name)
			return
		}

		var storageSchema *apiExtensionsV1.JSONSchemaProps
		if storageVersion.Schema != nil {
			storageSchema = storageVersion.Schema.OpenAPIV3Schema
		}
		schemas[crd.Name] = storageSchema

		contracts = append(contracts, CRDContract{
			Name:            crd.Name,
			ExpectedVersion: storageVersion.Name,
		})
	}

	cachedContracts = contracts
	cachedSchemas = schemas
}

func storageVersionFromDefinition(crd *apiExtensionsV1.CustomResourceDefinition) (*apiExtensionsV1.CustomResourceDefinitionVersion, bool) {
	for i := range crd.Spec.Versions {
		if crd.Spec.Versions[i].Storage {
			return &crd.Spec.Versions[i], true
		}
	}
	return nil, false
}
