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

package installer

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/alibaba/higress/hgctl/pkg/helm"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type failingHelmOwnershipChecker struct{ err error }

func (f failingHelmOwnershipChecker) IsHigressInstalled() (bool, error) { return false, f.err }

type recordingComponent struct{ run bool }

func (c *recordingComponent) ComponentName() ComponentName    { return Higress }
func (c *recordingComponent) Namespace() string               { return "higress-system" }
func (c *recordingComponent) Enabled() bool                   { return true }
func (c *recordingComponent) Run() error                      { c.run = true; return nil }
func (c *recordingComponent) RenderManifest() (string, error) { return "", nil }

type manifestComponent struct {
	namespace string
	manifest  string
}

func (c manifestComponent) ComponentName() ComponentName    { return Higress }
func (c manifestComponent) Namespace() string               { return c.namespace }
func (c manifestComponent) Enabled() bool                   { return true }
func (c manifestComponent) Run() error                      { return nil }
func (c manifestComponent) RenderManifest() (string, error) { return c.manifest, nil }

type recordingProfileStore struct{ saved bool }

func (s *recordingProfileStore) Save(*helm.Profile) (string, error)   { s.saved = true; return "", nil }
func (s *recordingProfileStore) List() ([]*ProfileContext, error)     { return nil, nil }
func (s *recordingProfileStore) Delete(*helm.Profile) (string, error) { return "", nil }

type noOpKubeCLI struct{}

func (noOpKubeCLI) RESTConfig() *rest.Config { return &rest.Config{} }
func (noOpKubeCLI) Pod(types.NamespacedName) (*corev1.Pod, error) {
	return nil, nil
}
func (noOpKubeCLI) PodsForSelector(string, ...string) (*corev1.PodList, error) {
	return nil, nil
}
func (noOpKubeCLI) PodExec(types.NamespacedName, string, string) (string, string, error) {
	return "", "", nil
}
func (noOpKubeCLI) ApplyObject(*unstructured.Unstructured) error  { return nil }
func (noOpKubeCLI) DeleteObject(*unstructured.Unstructured) error { return nil }
func (noOpKubeCLI) CreateNamespace(string) error                  { return nil }
func (noOpKubeCLI) KubernetesInterface() kubernetes.Interface     { return nil }

type recordingKubeCLI struct {
	noOpKubeCLI
	applied    int
	successful int
	failName   string
	err        error
}

func (c *recordingKubeCLI) ApplyObject(obj *unstructured.Unstructured) error {
	c.applied++
	if obj.GetName() == c.failName {
		return c.err
	}
	c.successful++
	return nil
}

type failingRenderer struct{ err error }

func (r failingRenderer) Init() error                           { return nil }
func (r failingRenderer) RenderManifest(string) (string, error) { return "", r.err }
func (r failingRenderer) SetVersion(string)                     {}

func TestK8sInstallerOwnershipCheckFailureStopsMutation(t *testing.T) {
	for _, upgrade := range []bool{false, true} {
		t.Run(map[bool]string{false: "install", true: "upgrade"}[upgrade], func(t *testing.T) {
			component := &recordingComponent{}
			store := &recordingProfileStore{}
			installer := &K8sInstaller{
				profile:      &helm.Profile{Global: helm.ProfileGlobal{Install: helm.InstallK8s}},
				writer:       &bytes.Buffer{},
				components:   map[ComponentName]Component{Higress: component},
				profileStore: store,
				helmChecker:  failingHelmOwnershipChecker{err: errors.New("helm unavailable")},
			}

			var err error
			if upgrade {
				err = installer.Upgrade()
			} else {
				err = installer.Install()
			}
			if err == nil {
				t.Fatal("Install() error = nil, want ownership-check failure")
			}
			if component.run {
				t.Fatal("component Run() was called after ownership-check failure")
			}
			if store.saved {
				t.Fatal("ProfileStore.Save() was called after ownership-check failure")
			}
		})
	}
}

func TestK8sInstallerRecoveredHelmModeBypassesOwnershipCheckAndSkipsPersistence(t *testing.T) {
	store := &recordingProfileStore{}
	installer := &K8sInstaller{
		profile: &helm.Profile{
			Global: helm.ProfileGlobal{Install: helm.InstallK8s, Namespace: "prod"},
		},
		writer:       &bytes.Buffer{},
		components:   map[ComponentName]Component{},
		profileStore: store,
		helmChecker:  failingHelmOwnershipChecker{err: errors.New("helm guard should be bypassed")},
		kubeCli:      noOpKubeCLI{},
		execOptions: ExecutionOptions{
			ProfilePersistence: DoNotPersistProfile,
			SourceMode:         RecoveredHelmExecutionSource,
		},
	}

	if err := installer.Install(); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if store.saved {
		t.Fatal("ProfileStore.Save() was called in recovered Helm mode")
	}
}

func TestK8sInstallerRecoveredHelmRenderFailureFailsClosedWithoutLeakingValues(t *testing.T) {
	const canary = "SECRET-CANARY-DO-NOT-PRINT"
	writer := &bytes.Buffer{}
	store := &recordingProfileStore{}
	kube := &recordingKubeCLI{}
	profile := &helm.Profile{
		Profile: "k8s",
		Global: helm.ProfileGlobal{
			Install:      helm.InstallK8s,
			Namespace:    "prod",
			IngressClass: "higress",
		},
		Console:    helm.ProfileConsole{Replicas: 1},
		Gateway:    helm.ProfileGateway{Replicas: 1},
		Controller: helm.ProfileController{Replicas: 1},
		Values: map[string]any{
			"custom": map[string]any{"sensitive": canary},
		},
	}
	installer := &K8sInstaller{
		profile: profile,
		writer:  writer,
		components: map[ComponentName]Component{
			Higress: &HigressComponent{
				profile:  profile,
				opts:     &ComponentOptions{Namespace: "prod", Version: "2.2.4", Quiet: true},
				renderer: failingRenderer{err: errors.New("helm template failed: " + canary)},
				writer:   writer,
			},
		},
		profileStore: store,
		kubeCli:      kube,
		execOptions: ExecutionOptions{
			ProfilePersistence: DoNotPersistProfile,
			SourceMode:         RecoveredHelmExecutionSource,
		},
	}

	err := installer.Upgrade()
	if err == nil {
		t.Fatal("Upgrade() error = nil, want render failure")
	}
	if strings.Contains(err.Error(), canary) {
		t.Fatalf("Upgrade() leaked retained value in error: %v", err)
	}
	if strings.Contains(writer.String(), canary) {
		t.Fatalf("Upgrade() leaked retained value in output: %s", writer.String())
	}
	if strings.Contains(writer.String(), "Install All Resources Complete") {
		t.Fatalf("Upgrade() printed success after render failure: %s", writer.String())
	}
	if kube.applied != 0 {
		t.Fatalf("ApplyObject calls = %d, want 0", kube.applied)
	}
	if store.saved {
		t.Fatal("ProfileStore.Save() was called after render failure")
	}
}

func TestK8sInstallerRecoveredHelmApplyFailureFailsClosedWithoutLeakingValues(t *testing.T) {
	const canary = "SECRET-CANARY-DO-NOT-PRINT"
	writer := &bytes.Buffer{}
	store := &recordingProfileStore{}
	kube := &recordingKubeCLI{
		failName: "rejected-second",
		err:      errors.New("admission rejected object containing " + canary),
	}
	installer := &K8sInstaller{
		profile: &helm.Profile{
			Global: helm.ProfileGlobal{Install: helm.InstallK8s, Namespace: "prod"},
		},
		writer: writer,
		components: map[ComponentName]Component{
			Higress: manifestComponent{
				namespace: "prod",
				manifest: `
apiVersion: v1
kind: ConfigMap
metadata:
  name: applied-first
data:
  ok: "true"
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: rejected-second
data:
  sensitive: SECRET-CANARY-DO-NOT-PRINT
`,
			},
		},
		profileStore: store,
		kubeCli:      kube,
		execOptions: ExecutionOptions{
			ProfilePersistence: DoNotPersistProfile,
			SourceMode:         RecoveredHelmExecutionSource,
		},
	}

	err := installer.Upgrade()
	if err == nil {
		t.Fatal("Upgrade() error = nil, want apply failure")
	}
	if strings.Contains(err.Error(), canary) {
		t.Fatalf("Upgrade() leaked retained value in error: %v", err)
	}
	if !strings.Contains(err.Error(), "partially applied") {
		t.Fatalf("Upgrade() error = %v, want partial-apply guidance", err)
	}
	if strings.Contains(writer.String(), canary) {
		t.Fatalf("Upgrade() leaked retained value in output: %s", writer.String())
	}
	if strings.Contains(writer.String(), "Install All Resources Complete") {
		t.Fatalf("Upgrade() printed success after apply failure: %s", writer.String())
	}
	if kube.successful == 0 {
		t.Fatal("ApplyObject had no successful object before failure; want partial apply semantics covered")
	}
	if store.saved {
		t.Fatal("ProfileStore.Save() was called after apply failure")
	}
}
