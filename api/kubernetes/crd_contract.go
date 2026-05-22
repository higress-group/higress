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
	StorageSchema   *apiExtensionsV1.JSONSchemaProps
}

var (
	loadContractsOnce sync.Once
	cachedContracts   []CRDContract
	cachedErr         error
)

func LoadCRDContracts() ([]CRDContract, error) {
	loadContractsOnce.Do(func() {
		cachedContracts, cachedErr = loadCRDContracts()
	})
	return cachedContracts, cachedErr
}

func loadCRDContracts() ([]CRDContract, error) {
	data, err := customResourceDefinitionsYAML.ReadFile("customresourcedefinitions.gen.yaml")
	if err != nil {
		return nil, fmt.Errorf("read generated CRD manifest: %w", err)
	}

	decoder := utilyaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)
	contracts := make([]CRDContract, 0, 4)

	for {
		var crd apiExtensionsV1.CustomResourceDefinition
		if err := decoder.Decode(&crd); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("decode generated CRD manifest: %w", err)
		}
		if crd.Name == "" {
			continue
		}

		storageVersion, found := storageVersionFromDefinition(&crd)
		if !found {
			return nil, fmt.Errorf("crd %s has no storage version in generated manifest", crd.Name)
		}

		contracts = append(contracts, CRDContract{
			Name:            crd.Name,
			ExpectedVersion: storageVersion.Name,
			StorageSchema:   schemaOrNil(storageVersion),
		})
	}

	return contracts, nil
}

func storageVersionFromDefinition(crd *apiExtensionsV1.CustomResourceDefinition) (*apiExtensionsV1.CustomResourceDefinitionVersion, bool) {
	for i := range crd.Spec.Versions {
		if crd.Spec.Versions[i].Storage {
			return &crd.Spec.Versions[i], true
		}
	}
	return nil, false
}

func schemaOrNil(version *apiExtensionsV1.CustomResourceDefinitionVersion) *apiExtensionsV1.JSONSchemaProps {
	if version.Schema == nil {
		return nil
	}
	return version.Schema.OpenAPIV3Schema
}
