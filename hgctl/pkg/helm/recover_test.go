// Copyright (c) 2022 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helm

import (
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

func TestClassifyRecoveredHelmSourceVersion(t *testing.T) {
	tests := []struct {
		version string
		want    CompatibilityClass
	}{
		{version: "2.1.0", want: CompatibilityGuaranteed},
		{version: "2.1.9", want: CompatibilityGuaranteed},
		{version: "2.2.4", want: CompatibilityGuaranteed},
		{version: "2.0.9", want: CompatibilityBestEffort},
		{version: "2.1.0-rc.2", want: CompatibilityBestEffort},
		{version: "2.3.0", want: CompatibilityBestEffort},
		{version: "not-semver", want: CompatibilityBestEffort},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			if got := ClassifyRecoveredHelmSourceVersion(tt.version); got != tt.want {
				t.Fatalf("ClassifyRecoveredHelmSourceVersion(%q) = %s, want %s", tt.version, got, tt.want)
			}
		})
	}
}

func TestNormalizeRecoveredProfileValuesConsumesReleaseNamespace(t *testing.T) {
	profile := &Profile{
		Global: ProfileGlobal{
			Install:   InstallK8s,
			Namespace: "selected-ns",
		},
		Values: map[string]any{
			"global": map[string]any{
				"namespace": "overlay-ns",
			},
		},
	}

	if err := NormalizeRecoveredProfileValues(profile); err != nil {
		t.Fatalf("NormalizeRecoveredProfileValues() error = %v", err)
	}
	if profile.Global.Namespace != "overlay-ns" {
		t.Fatalf("Global.Namespace = %q, want overlay-ns", profile.Global.Namespace)
	}

	if _, ok := getNested(profile.Values, "global.namespace"); ok {
		t.Fatalf("NormalizeRecoveredProfileValues() left a raw global.namespace duplicate: %#v", profile.Values)
	}
}

func TestReconstructProfileFromHelmNormalizesTypedOwnedValues(t *testing.T) {
	profile, diagnostics, err := ReconstructProfileFromHelm(HelmReleaseCandidate{
		Name:         "custom-higress",
		Namespace:    "prod",
		ChartName:    "higress",
		ChartVersion: "2.2.4",
	}, map[string]any{
		"global": map[string]any{
			"local":            false,
			"ingressClass":     "custom-class",
			"enableIstioAPI":   true,
			"enableGatewayAPI": true,
			"pluginNamespace":  "plugins-retained",
		},
		"higress-core": map[string]any{
			"gateway": map[string]any{"replicas": float64(2)},
		},
	})
	if err != nil {
		t.Fatalf("ReconstructProfileFromHelm() error = %v", err)
	}
	if profile.Global.Install != InstallK8s || profile.Profile != "k8s" {
		t.Fatalf("install/profile = %s/%s, want k8s/k8s", profile.Global.Install, profile.Profile)
	}
	if profile.Global.IngressClass != "custom-class" {
		t.Fatalf("ingress class = %q, want custom-class", profile.Global.IngressClass)
	}
	if profile.Gateway.Replicas != 2 {
		t.Fatalf("gateway replicas = %d, want 2", profile.Gateway.Replicas)
	}
	if _, ok := getNested(profile.Values, "higress-core.gateway.replicas"); ok {
		t.Fatal("owned raw gateway replicas survived normalization")
	}
	if got, ok := getNested(profile.Values, "global.pluginNamespace"); !ok || got != "plugins-retained" {
		t.Fatalf("unowned retained value = %v/%t, want plugins-retained/true", got, ok)
	}
	for _, diagnostic := range diagnostics {
		if strings.Contains(diagnostic.String(), "plugins-retained") {
			t.Fatalf("diagnostic leaked retained value: %s", diagnostic.String())
		}
	}
}

func TestOverlayRecoveredProfileLaterLayersWinAcrossTypedAndRawForms(t *testing.T) {
	profile, _, err := ReconstructProfileFromHelm(HelmReleaseCandidate{
		Name:         "custom-higress",
		Namespace:    "prod",
		ChartName:    "higress",
		ChartVersion: "2.2.4",
	}, map[string]any{
		"global":       map[string]any{"local": false},
		"higress-core": map[string]any{"gateway": map[string]any{"replicas": float64(2)}},
	})
	if err != nil {
		t.Fatalf("ReconstructProfileFromHelm() error = %v", err)
	}
	if err := OverlayRecoveredProfile(profile, "gateway:\n  replicas: 4\n"); err != nil {
		t.Fatalf("OverlayRecoveredProfile(typed) error = %v", err)
	}
	if profile.Gateway.Replicas != 4 {
		t.Fatalf("typed overlay replicas = %d, want 4", profile.Gateway.Replicas)
	}
	if err := OverlayRecoveredProfile(profile, "values:\n  higress-core:\n    gateway:\n      replicas: 3\n"); err != nil {
		t.Fatalf("OverlayRecoveredProfile(raw) error = %v", err)
	}
	if profile.Gateway.Replicas != 3 {
		t.Fatalf("raw overlay replicas = %d, want 3", profile.Gateway.Replicas)
	}
	valuesYAML, err := profile.ValuesYaml()
	if err != nil {
		t.Fatalf("ValuesYaml() error = %v", err)
	}
	values := make(map[string]any)
	if err := yaml.Unmarshal([]byte(valuesYAML), &values); err != nil {
		t.Fatalf("parse ValuesYaml() = %v\n%s", err, valuesYAML)
	}
	got, ok := getNested(values, "higress-core.gateway.replicas")
	if !ok || got != float64(3) {
		t.Fatalf("rendered gateway replicas = %#v/%t, want 3/true\n%s", got, ok, valuesYAML)
	}
}

func TestOverlayRecoveredProfileTypeErrorDoesNotExposeRetainedValues(t *testing.T) {
	const canary = "SECRET-CANARY-DO-NOT-PRINT"

	profile, _, err := ReconstructProfileFromHelm(HelmReleaseCandidate{
		Name:         "custom-higress",
		Namespace:    "prod",
		ChartName:    "higress",
		ChartVersion: "2.2.4",
	}, map[string]any{
		"global": map[string]any{"local": false},
		"custom": map[string]any{"sensitive": canary},
	})
	if err != nil {
		t.Fatalf("ReconstructProfileFromHelm() error = %v", err)
	}

	err = OverlayRecoveredProfile(profile, "gateway:\n  replicas: not-a-number\n")
	if err == nil {
		t.Fatal("OverlayRecoveredProfile() error = nil, want type failure")
	}
	if strings.Contains(err.Error(), canary) {
		t.Fatalf("OverlayRecoveredProfile() leaked retained value in error: %v", err)
	}
}

func TestReconstructProfileFromHelmRejectsAmbiguousInstallSignals(t *testing.T) {
	_, _, err := ReconstructProfileFromHelm(HelmReleaseCandidate{
		Name:         "custom-higress",
		Namespace:    "prod",
		ChartName:    "higress",
		ChartVersion: "2.2.4",
	}, map[string]any{
		"global": map[string]any{"local": true, "kind": false},
	})
	if err == nil || !strings.Contains(err.Error(), RecoveryDiagnosticAmbiguousInstall) {
		t.Fatalf("ReconstructProfileFromHelm() error = %v, want ambiguous install", err)
	}
}

func TestReconstructProfileFromHelmKeepsSelectedReleaseNamespaceAuthoritative(t *testing.T) {
	profile, _, err := ReconstructProfileFromHelm(HelmReleaseCandidate{
		Name:         "custom-higress",
		Namespace:    "selected-ns",
		ChartName:    "higress",
		ChartVersion: "2.2.4",
	}, map[string]any{
		"global": map[string]any{
			"local":     false,
			"namespace": "retained-ns",
		},
	})
	if err != nil {
		t.Fatalf("ReconstructProfileFromHelm() error = %v", err)
	}
	if profile.Global.Namespace != "selected-ns" {
		t.Fatalf("Global.Namespace = %q, want selected release namespace", profile.Global.Namespace)
	}
	if _, ok := getNested(profile.Values, "global.namespace"); ok {
		t.Fatalf("ReconstructProfileFromHelm() left raw global.namespace duplicate: %#v", profile.Values)
	}
}

func TestReconstructProfileFromHelmDiagnosticsDoNotExposeRetainedValues(t *testing.T) {
	const canary = "SECRET-CANARY-DO-NOT-PRINT"

	_, diagnostics, err := ReconstructProfileFromHelm(HelmReleaseCandidate{
		Name:         "custom-higress",
		Namespace:    "prod",
		ChartName:    "higress",
		ChartVersion: "2.3.0",
	}, map[string]any{
		"global": map[string]any{
			"local": false,
		},
		"custom": map[string]any{
			"sensitive": canary,
		},
	})
	if err != nil {
		t.Fatalf("ReconstructProfileFromHelm() error = %v", err)
	}
	if len(diagnostics) == 0 {
		t.Fatal("ReconstructProfileFromHelm() diagnostics empty, want best-effort and unsupported-value diagnostics")
	}
	for _, diagnostic := range diagnostics {
		if strings.Contains(diagnostic.String(), canary) {
			t.Fatalf("diagnostic leaked retained value: %s", diagnostic.String())
		}
	}
}
