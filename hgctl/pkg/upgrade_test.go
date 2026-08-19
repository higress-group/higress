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
	"fmt"
	"io"
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
