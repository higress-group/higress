// Copyright (c) 2026 Alibaba Group Holding Ltd.
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

package installer

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alibaba/higress/hgctl/pkg/helm"
)

var errTestUpgrade = errors.New("test upgrade failed")

type dockerVersionProfileStore struct {
	profiles  []*ProfileContext
	listErr   error
	saveErr   error
	saved     []*helm.Profile
	listCalls int
}

func (s *dockerVersionProfileStore) Save(profile *helm.Profile) (string, error) {
	s.saved = append(s.saved, profile)
	return "stored", s.saveErr
}

func (s *dockerVersionProfileStore) List() ([]*ProfileContext, error) {
	s.listCalls++
	return s.profiles, s.listErr
}

func (s *dockerVersionProfileStore) Delete(*helm.Profile) (string, error) {
	return "", nil
}

func newDockerVersionProfile(version string) *helm.Profile {
	return &helm.Profile{
		Profile:            "local-docker",
		InstallPackagePath: "/stored/higress",
		HigressVersion:     version,
		Global: helm.ProfileGlobal{
			Install:      helm.InstallLocalDocker,
			IngressClass: "stored-class",
		},
		Console: helm.ProfileConsole{Port: 19000},
		Gateway: helm.ProfileGateway{
			HttpPort:    18080,
			HttpsPort:   18443,
			MetricsPort: 18090,
		},
		Storage: helm.ProfileStorage{
			Url: "file:///stored/nacos",
			Ns:  "stored",
		},
		Values: map[string]any{
			"stored": map[string]any{"key": "value"},
		},
	}
}

func newDockerVersionInstaller(t *testing.T, stored, caller *helm.Profile, versionContent string, upgrade func() error) (*DockerInstaller, *dockerVersionProfileStore) {
	t.Helper()
	installPath := t.TempDir()
	versionPath := filepath.Join(installPath, "higress", "VERSION")
	if versionContent != "missing" {
		if err := os.MkdirAll(filepath.Dir(versionPath), 0o755); err != nil {
			t.Fatalf("create version directory: %v", err)
		}
		if versionContent != "directory" {
			if err := os.WriteFile(versionPath, []byte(versionContent), 0o644); err != nil {
				t.Fatalf("write VERSION: %v", err)
			}
		} else if err := os.Mkdir(versionPath, 0o755); err != nil {
			t.Fatalf("create VERSION directory: %v", err)
		}
	}
	store := &dockerVersionProfileStore{
		profiles: []*ProfileContext{{
			Profile:        stored,
			Install:        helm.InstallLocalDocker,
			SourceType:     "file",
			PathOrName:     filepath.Join(t.TempDir(), "install-local-docker.yaml"),
			HigressVersion: stored.HigressVersion,
		}},
	}
	return &DockerInstaller{
		standalone:   &StandaloneComponent{agent: &Agent{versionPath: versionPath}},
		profile:      caller,
		writer:       io.Discard,
		profileStore: store,
		upgradeFunc:  upgrade,
	}, store
}

func TestDockerInstallerUpgradeSynchronizesStoredVersionWithoutOverlayLeak(t *testing.T) {
	stored := newDockerVersionProfile("v1.0.0")
	caller := *stored
	caller.Gateway.HttpPort = 18081
	caller.Values = map[string]any{"caller": map[string]any{"overlay": true}}

	installer, store := newDockerVersionInstaller(t, stored, &caller, "\n v2.0.0 \n", func() error { return nil })
	if err := installer.Upgrade(); err != nil {
		t.Fatalf("Upgrade() error = %v", err)
	}

	if len(store.saved) != 1 {
		t.Fatalf("saved profiles = %d, want 1", len(store.saved))
	}
	want := *stored
	want.HigressVersion = "v2.0.0"
	if !reflect.DeepEqual(store.saved[0], &want) {
		t.Fatalf("saved profile = %#v, want %#v", store.saved[0], &want)
	}
	if caller.Gateway.HttpPort != 18081 || caller.Values["caller"] == nil {
		t.Fatalf("caller overlay was unexpectedly mutated: %#v", caller)
	}
	if caller.HigressVersion != "v2.0.0" {
		t.Fatalf("caller version = %q, want v2.0.0", caller.HigressVersion)
	}
}

func TestDockerInstallerUpgradeFailureDoesNotSaveProfile(t *testing.T) {
	stored := newDockerVersionProfile("v1.0.0")
	installer, store := newDockerVersionInstaller(t, stored, stored, "v2.0.0", func() error { return errTestUpgrade })

	err := installer.Upgrade()
	if !errors.Is(err, errTestUpgrade) {
		t.Fatalf("Upgrade() error = %v, want %v", err, errTestUpgrade)
	}
	if len(store.saved) != 0 {
		t.Fatalf("saved profiles = %d, want 0", len(store.saved))
	}
}

func TestDockerInstallerUpgradeRejectsInvalidInstalledVersion(t *testing.T) {
	tests := []struct {
		name           string
		versionContent string
		wantError      string
	}{
		{name: "missing", versionContent: "missing", wantError: "read installed Higress version"},
		{name: "unreadable", versionContent: "directory", wantError: "read installed Higress version"},
		{name: "empty", versionContent: " \n\t", wantError: "installed Higress version is empty"},
		{name: "malformed", versionContent: "not-a-version\n", wantError: "invalid installed Higress version"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stored := newDockerVersionProfile("v1.0.0")
			installer, store := newDockerVersionInstaller(t, stored, stored, tt.versionContent, func() error { return nil })

			err := installer.Upgrade()
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("Upgrade() error = %v, want substring %q", err, tt.wantError)
			}
			if store.listCalls != 0 {
				t.Fatalf("profile List calls = %d, want 0", store.listCalls)
			}
			if len(store.saved) != 0 {
				t.Fatalf("saved profiles = %d, want 0", len(store.saved))
			}
		})
	}
}

func TestDockerInstallerUpgradeReturnsProfileSaveFailure(t *testing.T) {
	stored := newDockerVersionProfile("v1.0.0")
	installer, store := newDockerVersionInstaller(t, stored, stored, "v2.0.0", func() error { return nil })
	store.saveErr = errors.New("profile save failed")

	err := installer.Upgrade()
	if err == nil || !strings.Contains(err.Error(), "profile save failed") {
		t.Fatalf("Upgrade() error = %v, want profile save failure", err)
	}
	if stored.HigressVersion != "v1.0.0" {
		t.Fatalf("stored profile was mutated before save: %q", stored.HigressVersion)
	}
}

func TestAgentVersionRemainsTolerantForDisplayReads(t *testing.T) {
	agent := &Agent{versionPath: filepath.Join(t.TempDir(), "missing", "VERSION")}

	version, err := agent.Version()
	if err != nil {
		t.Fatalf("Version() error = %v, want nil for display read", err)
	}
	if version != "" {
		t.Fatalf("Version() = %q, want empty version for missing display read", version)
	}
}
