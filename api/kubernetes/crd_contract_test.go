package kubernetes

import "testing"

func TestLoadCRDContracts(t *testing.T) {
	contracts, err := LoadCRDContracts()
	if err != nil {
		t.Fatalf("LoadCRDContracts() returned error: %v", err)
	}

	if len(contracts) != 3 {
		t.Fatalf("expected 3 CRD contracts, got %d", len(contracts))
	}

	expectedVersions := map[string]string{
		"wasmplugins.extensions.higress.io": "v1alpha1",
		"http2rpcs.networking.higress.io":   "v1",
		"mcpbridges.networking.higress.io":  "v1",
	}

	for _, contract := range contracts {
		wantVersion, ok := expectedVersions[contract.Name]
		if !ok {
			t.Fatalf("unexpected CRD contract loaded: %s", contract.Name)
		}
		if contract.ExpectedVersion != wantVersion {
			t.Fatalf("CRD %s expected version %s, got %s", contract.Name, wantVersion, contract.ExpectedVersion)
		}
	}
}
