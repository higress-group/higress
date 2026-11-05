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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/higress/hgctl/pkg/kubernetes"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8skubernetes "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

func TestStrictFileProfileCollisionsIncludesLegacyInstallYAML(t *testing.T) {
	profilesPath := t.TempDir()
	legacyPath := filepath.Join(profilesPath, InstalledYamlFileName)
	writeTestProfile(t, legacyPath, "prod")

	collisions, err := StrictFileProfileCollisions(profilesPath, "prod")
	if err != nil {
		t.Fatalf("StrictFileProfileCollisions() error = %v", err)
	}
	if len(collisions) != 1 {
		t.Fatalf("collisions = %d, want 1", len(collisions))
	}
	if collisions[0].PathOrName != legacyPath {
		t.Fatalf("collision path = %q, want %q", collisions[0].PathOrName, legacyPath)
	}
}

func TestStrictFileProfileCollisionsFailsClosedOnMalformedFile(t *testing.T) {
	profilesPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(profilesPath, "install-prod.yaml"), []byte("global:\n  namespace: ["), 0o644); err != nil {
		t.Fatalf("write malformed profile: %v", err)
	}

	_, err := StrictFileProfileCollisions(profilesPath, "prod")
	if err == nil || !strings.Contains(err.Error(), "parse profile file") {
		t.Fatalf("StrictFileProfileCollisions() error = %v, want parse failure", err)
	}
}

func TestStrictFileProfileCollisionsFailsClosedOnUnclassifiableProfile(t *testing.T) {
	profilesPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(profilesPath, "install-prod.yaml"), []byte("global:\n  install: unsupported\n  namespace: prod\n"), 0o644); err != nil {
		t.Fatalf("write unclassifiable profile: %v", err)
	}

	_, err := StrictFileProfileCollisions(profilesPath, "prod")
	if err == nil || !strings.Contains(err.Error(), "parse profile file") {
		t.Fatalf("StrictFileProfileCollisions() error = %v, want parse failure", err)
	}
}

func TestStrictFileProfileCollisionsAllowsLocalDockerWithoutNamespace(t *testing.T) {
	profilesPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(profilesPath, "install-local-docker.yaml"), []byte("global:\n  install: local-docker\n"), 0o644); err != nil {
		t.Fatalf("write local-docker profile: %v", err)
	}

	collisions, err := StrictFileProfileCollisions(profilesPath, "prod")
	if err != nil {
		t.Fatalf("StrictFileProfileCollisions() error = %v", err)
	}
	if len(collisions) != 0 {
		t.Fatalf("collisions = %d, want 0", len(collisions))
	}
}

func TestConfigmapProfileStoreStrictCollisionsChecksLaterPages(t *testing.T) {
	kube := fake.NewSimpleClientset()
	listCalls := 0
	kube.Fake.PrependReactor("list", "configmaps", func(action clienttesting.Action) (bool, runtime.Object, error) {
		if _, ok := action.(clienttesting.ListActionImpl); !ok {
			t.Fatalf("list action type = %T, want ListActionImpl", action)
		}
		listCalls++
		if listCalls == 1 {
			return true, &corev1.ConfigMapList{
				ListMeta: metav1.ListMeta{Continue: "next"},
				Items: []corev1.ConfigMap{
					*newTestProfileConfigMap("other-ns", "other"),
				},
			}, nil
		}
		return true, &corev1.ConfigMapList{
			Items: []corev1.ConfigMap{
				*newTestProfileConfigMap("prod", "prod"),
			},
		}, nil
	})
	store := &ConfigmapProfileStore{kubeCli: profileStoreTestCLI{client: kube}}

	collisions, err := store.StrictCollisions("prod")
	if err != nil {
		t.Fatalf("StrictCollisions() error = %v", err)
	}
	if len(collisions) != 1 {
		t.Fatalf("collisions = %d, want 1", len(collisions))
	}
	if collisions[0].PathOrName != "prod/higress-profile" {
		t.Fatalf("collision path = %q, want prod/higress-profile", collisions[0].PathOrName)
	}
	if listCalls != 2 {
		t.Fatalf("list calls = %d, want 2", listCalls)
	}
}

func TestConfigmapProfileStoreStrictCollisionsFailsClosedOnUnclassifiableProfile(t *testing.T) {
	kube := fake.NewSimpleClientset(newRawProfileConfigMap("prod", "global:\n  install: k8s\n"))
	store := &ConfigmapProfileStore{kubeCli: profileStoreTestCLI{client: kube}}

	_, err := store.StrictCollisions("prod")
	if err == nil || !strings.Contains(err.Error(), "parse profile configmap") {
		t.Fatalf("StrictCollisions() error = %v, want parse failure", err)
	}
}

func writeTestProfile(t *testing.T, path, namespace string) {
	t.Helper()
	content := "global:\n  install: k8s\n  namespace: " + namespace + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}
}

func newTestProfileConfigMap(namespace, profileNamespace string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ProfileConfigmapName,
			Namespace: namespace,
		},
		Data: map[string]string{
			ProfileConfigmapKey: "global:\n  install: k8s\n  namespace: " + profileNamespace + "\n",
		},
	}
}

func newRawProfileConfigMap(namespace, profile string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ProfileConfigmapName,
			Namespace: namespace,
		},
		Data: map[string]string{
			ProfileConfigmapKey: profile,
		},
	}
}

type profileStoreTestCLI struct {
	kubernetes.CLIClient
	client k8skubernetes.Interface
}

func (c profileStoreTestCLI) KubernetesInterface() k8skubernetes.Interface {
	return c.client
}
