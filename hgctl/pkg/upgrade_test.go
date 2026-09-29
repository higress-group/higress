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

package hgctl

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/alibaba/higress/hgctl/pkg/helm"
	"github.com/alibaba/higress/hgctl/pkg/installer"
)

func TestValidateLocalDockerUpgradeOverlay(t *testing.T) {
	baseline := &helm.Profile{
		Global:  helm.ProfileGlobal{Install: helm.InstallLocalDocker},
		Gateway: helm.ProfileGateway{HttpPort: 80},
	}

	if err := validateLocalDockerUpgradeOverlay(baseline, "gateway:\n  httpPort: 18080\n"); err == nil || err.Error() != "local-docker upgrade does not support overlay fields: gateway.httpPort" {
		t.Fatalf("validateLocalDockerUpgradeOverlay() error = %v", err)
	}
	if err := validateLocalDockerUpgradeOverlay(baseline, "charts:\n  standalone:\n    url: https://example.com/get-higress.sh\n"); err != nil {
		t.Fatalf("validateLocalDockerUpgradeOverlay() allowed URL error = %v", err)
	}
}

type recordingUpgradeInstaller struct {
	upgraded bool
}

func (i *recordingUpgradeInstaller) Install() error   { return nil }
func (i *recordingUpgradeInstaller) UnInstall() error { return nil }
func (i *recordingUpgradeInstaller) Upgrade() error {
	i.upgraded = true
	return nil
}

type fakeHelmReleaseReader struct {
	candidates []installer.HelmReleaseCandidate
	values     map[string]any
}

func (r *fakeHelmReleaseReader) ListReleases(installer.HelmReleaseSelector) ([]installer.HelmReleaseCandidate, error) {
	return r.candidates, nil
}

func (r *fakeHelmReleaseReader) GetRetainedValues(installer.HelmReleaseCandidate) (map[string]any, error) {
	return r.values, nil
}

type recordingRecoveredInstaller struct {
	profile  *helm.Profile
	options  installer.ExecutionOptions
	upgraded bool
}

func (i *recordingRecoveredInstaller) Install() error   { return nil }
func (i *recordingRecoveredInstaller) UnInstall() error { return nil }
func (i *recordingRecoveredInstaller) Upgrade() error {
	i.upgraded = true
	return nil
}

func withRecoveredUpgradeFakes(t *testing.T, reader *fakeHelmReleaseReader, recovered *recordingRecoveredInstaller) {
	t.Helper()
	originalReader := newHelmReaderForUpgrade
	originalCollision := strictProfileCollisionCheckForUpgrade
	originalRecoveredInstaller := newRecoveredInstaller
	originalStdin := upgradeStdin
	originalTerminal := isUpgradeTerminal
	t.Cleanup(func() {
		newHelmReaderForUpgrade = originalReader
		strictProfileCollisionCheckForUpgrade = originalCollision
		newRecoveredInstaller = originalRecoveredInstaller
		upgradeStdin = originalStdin
		isUpgradeTerminal = originalTerminal
	})
	newHelmReaderForUpgrade = func() installer.HelmReleaseReader { return reader }
	strictProfileCollisionCheckForUpgrade = func(string) error { return nil }
	newRecoveredInstaller = func(profile *helm.Profile, writer io.Writer, quiet bool, devel bool, mode installer.InstallerMode, options installer.ExecutionOptions) (installer.Installer, error) {
		recovered.profile = profile
		recovered.options = options
		return recovered, nil
	}
}

func newLocalDockerProfile(installPackagePath string) *helm.Profile {
	return &helm.Profile{
		InstallPackagePath: installPackagePath,
		Global:             helm.ProfileGlobal{Install: helm.InstallLocalDocker},
		Console:            helm.ProfileConsole{Port: 8001},
		Gateway: helm.ProfileGateway{
			HttpPort:    80,
			HttpsPort:   443,
			MetricsPort: 15020,
		},
		Storage: helm.ProfileStorage{
			Url: "file:///tmp/higress-storage",
			Ns:  "higress-system",
		},
	}
}

func TestUpgradeRejectsLocalDockerOverlayBeforeInstallerConstruction(t *testing.T) {
	tests := []struct {
		name      string
		set       string
		wantField string
	}{
		{name: "gateway setting", set: "gateway.httpPort=18080", wantField: "gateway.httpPort"},
		{name: "higress version", set: "higressVersion=2.2.5", wantField: "higressVersion"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalProfiles := getAllProfilesForUpgrade
			originalPrompt := promptUpgradeForUpgrade
			originalNewInstaller := newInstallerForUpgrade
			t.Cleanup(func() {
				getAllProfilesForUpgrade = originalProfiles
				promptUpgradeForUpgrade = originalPrompt
				newInstallerForUpgrade = originalNewInstaller
			})

			baseline := newLocalDockerProfile(t.TempDir())
			getAllProfilesForUpgrade = func() ([]*installer.ProfileContext, error) {
				return []*installer.ProfileContext{{Profile: baseline}}, nil
			}

			prompted := false
			promptUpgradeForUpgrade = func(io.Writer) bool {
				prompted = true
				return true
			}
			constructed := false
			newInstallerForUpgrade = func(*helm.Profile, io.Writer, bool, bool, installer.InstallerMode) (installer.Installer, error) {
				constructed = true
				return &recordingUpgradeInstaller{}, nil
			}

			err := upgrade(io.Discard, &InstallArgs{Set: []string{tt.set}})
			if err == nil || !strings.Contains(err.Error(), tt.wantField) {
				t.Fatalf("upgrade() error = %v, want unsupported field %q", err, tt.wantField)
			}
			if prompted {
				t.Fatal("upgrade confirmation was reached after overlay rejection")
			}
			if constructed {
				t.Fatal("installer was constructed after overlay rejection")
			}
		})
	}
}

func TestUpgradeAllowsLocalDockerOperationalInputs(t *testing.T) {
	tests := []struct {
		name string
		set  []string
	}{
		{name: "no overlay"},
		{name: "install package path", set: []string{"installPackagePath=%s"}},
		{name: "standalone URL", set: []string{"charts.standalone.url=https://example.com/get-higress.sh"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalProfiles := getAllProfilesForUpgrade
			originalPrompt := promptUpgradeForUpgrade
			originalNewInstaller := newInstallerForUpgrade
			t.Cleanup(func() {
				getAllProfilesForUpgrade = originalProfiles
				promptUpgradeForUpgrade = originalPrompt
				newInstallerForUpgrade = originalNewInstaller
			})

			baseline := newLocalDockerProfile(t.TempDir())
			getAllProfilesForUpgrade = func() ([]*installer.ProfileContext, error) {
				return []*installer.ProfileContext{{Profile: baseline}}, nil
			}
			promptUpgradeForUpgrade = func(io.Writer) bool { return true }
			fakeInstaller := &recordingUpgradeInstaller{}
			newInstallerForUpgrade = func(*helm.Profile, io.Writer, bool, bool, installer.InstallerMode) (installer.Installer, error) {
				return fakeInstaller, nil
			}

			set := tt.set
			if tt.name == "install package path" {
				set = []string{fmt.Sprintf(tt.set[0], t.TempDir())}
			}
			if err := upgrade(io.Discard, &InstallArgs{Set: set}); err != nil {
				t.Fatalf("upgrade() error = %v", err)
			}
			if !fakeInstaller.upgraded {
				t.Fatal("installer Upgrade() was not called for an allowed local-docker input")
			}
		})
	}
}

func TestUpgradeFromHelmRequiresYesInNonTerminalAfterValidation(t *testing.T) {
	reader := &fakeHelmReleaseReader{
		candidates: []installer.HelmReleaseCandidate{newRecoveredCandidate()},
		values:     map[string]any{"global": map[string]any{"local": false}},
	}
	recovered := &recordingRecoveredInstaller{}
	withRecoveredUpgradeFakes(t, reader, recovered)
	isUpgradeTerminal = func() bool { return false }

	err := upgrade(io.Discard, &InstallArgs{FromHelm: true})
	if err == nil || !strings.Contains(err.Error(), "requires --yes") {
		t.Fatalf("upgrade() error = %v, want --yes requirement", err)
	}
	if recovered.upgraded {
		t.Fatal("recovered installer was invoked without --yes")
	}
}

func TestUpgradeFromHelmAppliesOrderedFileAndFinalSetWithNoPersistenceOptions(t *testing.T) {
	valuesFile := writeTempValuesFile(t, "higress-core:\n  gateway:\n    replicas: 4\n")
	reader := &fakeHelmReleaseReader{
		candidates: []installer.HelmReleaseCandidate{newRecoveredCandidate()},
		values: map[string]any{
			"global":       map[string]any{"local": false},
			"higress-core": map[string]any{"gateway": map[string]any{"replicas": float64(2)}},
		},
	}
	recovered := &recordingRecoveredInstaller{}
	withRecoveredUpgradeFakes(t, reader, recovered)
	isUpgradeTerminal = func() bool { return false }

	err := upgrade(io.Discard, &InstallArgs{
		FromHelm:    true,
		Yes:         true,
		InFilenames: []string{valuesFile},
		Set:         []string{"gateway.replicas=3"},
	})
	if err != nil {
		t.Fatalf("upgrade() error = %v", err)
	}
	if !recovered.upgraded {
		t.Fatal("recovered installer Upgrade() was not called")
	}
	if recovered.profile.Gateway.Replicas != 3 {
		t.Fatalf("gateway replicas = %d, want final --set value 3", recovered.profile.Gateway.Replicas)
	}
	if _, ok := recovered.profile.Values["higress-core"]; ok {
		t.Fatalf("owned raw higress-core value survived normalization: %#v", recovered.profile.Values["higress-core"])
	}
	if recovered.options.ProfilePersistence != installer.DoNotPersistProfile {
		t.Fatalf("ProfilePersistence = %v, want DoNotPersistProfile", recovered.options.ProfilePersistence)
	}
	if recovered.options.SourceMode != installer.RecoveredHelmExecutionSource {
		t.Fatalf("SourceMode = %v, want RecoveredHelmExecutionSource", recovered.options.SourceMode)
	}
	if recovered.options.ReleaseName != "custom-higress" || recovered.options.ReleaseNamespace != "prod" {
		t.Fatalf("release identity = %s/%s, want prod/custom-higress", recovered.options.ReleaseNamespace, recovered.options.ReleaseName)
	}
}

func TestUpgradeFromHelmRejectsProtectedNamespaceOverlayBeforeInstaller(t *testing.T) {
	tests := []struct {
		name string
		set  string
	}{
		{name: "typed profile path", set: "global.namespace=other"},
		{name: "raw Helm values path", set: "values.global.namespace=other"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &fakeHelmReleaseReader{
				candidates: []installer.HelmReleaseCandidate{newRecoveredCandidate()},
				values:     map[string]any{"global": map[string]any{"local": false}},
			}
			recovered := &recordingRecoveredInstaller{}
			withRecoveredUpgradeFakes(t, reader, recovered)
			isUpgradeTerminal = func() bool { return false }

			err := upgrade(io.Discard, &InstallArgs{
				FromHelm: true,
				Yes:      true,
				Set:      []string{tt.set},
			})
			if err == nil || !strings.Contains(err.Error(), "selected release namespace") {
				t.Fatalf("upgrade() error = %v, want protected namespace rejection", err)
			}
			if recovered.upgraded {
				t.Fatal("recovered installer was invoked after protected-field rejection")
			}
		})
	}
}

func TestUpgradeFromHelmRejectsProfileSetFlag(t *testing.T) {
	err := upgrade(io.Discard, &InstallArgs{FromHelm: true, Set: []string{"profile=k8s"}})
	if err == nil || !strings.Contains(err.Error(), "--set profile") {
		t.Fatalf("upgrade() error = %v, want --set profile rejection", err)
	}
}

func TestUpgradeFromHelmMultipleCandidatesNeedSelectorInNonTerminal(t *testing.T) {
	reader := &fakeHelmReleaseReader{
		candidates: []installer.HelmReleaseCandidate{
			{Name: "b", Namespace: "prod", ChartName: "higress", ChartVersion: "2.2.4", Status: "deployed"},
			{Name: "a", Namespace: "prod", ChartName: "higress", ChartVersion: "2.2.4", Status: "deployed"},
		},
	}
	recovered := &recordingRecoveredInstaller{}
	withRecoveredUpgradeFakes(t, reader, recovered)
	isUpgradeTerminal = func() bool { return false }

	err := upgrade(io.Discard, &InstallArgs{FromHelm: true, Yes: true})
	if err == nil || !strings.Contains(err.Error(), "multiple Higress Helm releases") {
		t.Fatalf("upgrade() error = %v, want multiple-release selector error", err)
	}
	if recovered.upgraded {
		t.Fatal("installer was invoked after ambiguous non-terminal selection")
	}
}

func TestUpgradeFromHelmTerminalSelectionCancelExitsWithoutMutation(t *testing.T) {
	reader := &fakeHelmReleaseReader{
		candidates: []installer.HelmReleaseCandidate{
			{Name: "b", Namespace: "prod", ChartName: "higress", ChartVersion: "2.2.4", Status: "deployed"},
			{Name: "a", Namespace: "prod", ChartName: "higress", ChartVersion: "2.2.4", Status: "deployed"},
		},
	}
	recovered := &recordingRecoveredInstaller{}
	withRecoveredUpgradeFakes(t, reader, recovered)
	isUpgradeTerminal = func() bool { return true }
	upgradeStdin = bytes.NewBufferString("n\n")

	err := upgrade(io.Discard, &InstallArgs{FromHelm: true, Yes: true})
	if err != nil {
		t.Fatalf("upgrade() error = %v, want nil cancellation", err)
	}
	if recovered.upgraded {
		t.Fatal("installer was invoked after terminal cancellation")
	}
}

func newRecoveredCandidate() installer.HelmReleaseCandidate {
	return installer.HelmReleaseCandidate{
		Name:         "custom-higress",
		Namespace:    "prod",
		ChartName:    "higress",
		ChartVersion: "2.2.4",
		Status:       "deployed",
	}
}

func writeTempValuesFile(t *testing.T, content string) string {
	t.Helper()
	name := t.TempDir() + "/values.yaml"
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatalf("write values file: %v", err)
	}
	return name
}
