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

package installer

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/alibaba/higress/hgctl/pkg/helm"
	"github.com/alibaba/higress/hgctl/pkg/helm/object"
	"github.com/alibaba/higress/hgctl/pkg/kubernetes"
	"github.com/alibaba/higress/hgctl/pkg/util"
)

type K8sInstaller struct {
	started      bool
	components   map[ComponentName]Component
	kubeCli      kubernetes.CLIClient
	profile      *helm.Profile
	writer       io.Writer
	profileStore ProfileStore
	helmChecker  helmOwnershipChecker
	execOptions  ExecutionOptions
}

type helmOwnershipChecker interface {
	IsHigressInstalled() (bool, error)
}

func (o *K8sInstaller) Install() error {
	// check if higress is installed by helm
	if o.execOptions.SourceMode != RecoveredHelmExecutionSource {
		fmt.Fprintf(o.writer, "\n⌛️ Detecting higress installed by helm or not... \n\n")
		helmChecker := o.helmChecker
		if helmChecker == nil {
			helmChecker = NewHelmAgent(o.profile, o.writer, false)
		}
		helmInstalled, err := helmChecker.IsHigressInstalled()
		if err != nil {
			return fmt.Errorf("check Higress Helm ownership: %w", err)
		}
		if helmInstalled {
			fmt.Fprintf(o.writer, "\n🧐 You have already installed higress by helm, please use \"helm upgrade\" to upgrade higress!\n")
			return nil
		}
	}

	if err := o.Run(); err != nil {
		return err
	}

	manifestMap, err := o.RenderManifests()
	if err != nil {
		return o.sanitizeRecoveredHelmError("render recovered Helm target manifests failed", err)
	}

	fmt.Fprintf(o.writer, "\n⌛️ Processing installation... \n\n")
	if err := o.ApplyManifests(manifestMap); err != nil {
		return o.sanitizeRecoveredHelmError("apply recovered Helm target manifests failed; resources may be partially applied and no rollback was performed", err)
	}

	if o.execOptions.ProfilePersistence != DoNotPersistProfile {
		profileName, err1 := o.profileStore.Save(o.profile)
		if err1 != nil {
			return err1
		}
		fmt.Fprintf(o.writer, "\n✔️ Wrote Profile in kubernetes configmap: \"%s\" \n", profileName)
		fmt.Fprintf(o.writer, "\n   Use below kubectl command to edit profile for upgrade. \n")
		fmt.Fprintf(o.writer, "   ================================================================================== \n")
		names := strings.Split(profileName, "/")
		fmt.Fprintf(o.writer, "   kubectl edit configmap %s -n %s \n", names[1], names[0])
		fmt.Fprintf(o.writer, "   ================================================================================== \n")
	}

	fmt.Fprintf(o.writer, "\n🎊 Install All Resources Complete!\n")

	return nil
}

func (o *K8sInstaller) sanitizeRecoveredHelmError(message string, err error) error {
	if o.execOptions.SourceMode != RecoveredHelmExecutionSource {
		return err
	}
	return recoveredHelmSanitizedError{
		message: message,
		cause:   err,
	}
}

type recoveredHelmSanitizedError struct {
	message string
	cause   error
}

func (e recoveredHelmSanitizedError) Error() string {
	return e.message
}

func (e recoveredHelmSanitizedError) Unwrap() error {
	return e.cause
}

func (o *K8sInstaller) UnInstall() error {
	if _, err := GetProfileInstalledPath(); err != nil {
		return err
	}

	if err := o.Run(); err != nil {
		return err
	}

	manifestMap, err := o.RenderManifests()
	if err != nil {
		return err
	}

	fmt.Fprintf(o.writer, "\n⌛️ Processing uninstallation... \n\n")
	if err := o.DeleteManifests(manifestMap); err != nil {
		return err
	}

	profileName, err1 := o.profileStore.Delete(o.profile)
	if err1 != nil {
		return err1
	}
	fmt.Fprintf(o.writer, "\n✔️ Removed Profile: \"%s\" \n", profileName)

	fmt.Fprintf(o.writer, "\n🎊 Uninstall All Resources Complete!\n")

	return nil
}

func (o *K8sInstaller) Upgrade() error {
	return o.Install()
}

// Run must be invoked before invoking other functions.
func (o *K8sInstaller) Run() error {
	for name, component := range o.components {
		if !component.Enabled() {
			continue
		}
		if err := component.Run(); err != nil {
			return fmt.Errorf("component %s run failed, err: %s", name, err)
		}
	}
	o.started = true
	return nil
}

// RenderManifests renders component manifests specified by profile.
func (o *K8sInstaller) RenderManifests() (map[ComponentName]string, error) {
	if !o.started {
		return nil, errors.New("higress installer is not running")
	}
	res := make(map[ComponentName]string)
	for name, component := range o.components {
		if !component.Enabled() {
			continue
		}
		manifest, err := component.RenderManifest()
		if err != nil {
			return nil, fmt.Errorf("component %s RenderManifest err: %v", name, err)
		}
		res[name] = manifest
	}
	return res, nil
}

// GenerateManifests generates component manifests to k8s cluster
func (o *K8sInstaller) GenerateManifests(manifestMap map[ComponentName]string) error {
	if o.kubeCli == nil {
		return errors.New("no injected k8s cli into K8sInstaller")
	}
	for _, manifest := range manifestMap {
		fmt.Fprint(o.writer, manifest)
	}
	return nil
}

// ApplyManifests apply component manifests to k8s cluster
func (o *K8sInstaller) ApplyManifests(manifestMap map[ComponentName]string) error {
	if o.kubeCli == nil {
		return errors.New("no injected k8s cli into K8sInstaller")
	}
	for name, manifest := range manifestMap {
		namespace := o.components[name].Namespace()
		if err := o.applyManifest(manifest, namespace); err != nil {
			return fmt.Errorf("component %s ApplyManifest err: %v", name, err)
		}
	}
	return nil
}

func (o *K8sInstaller) applyManifest(manifest string, ns string) error {
	if err := o.kubeCli.CreateNamespace(ns); err != nil {
		return err
	}
	objs, err := object.ParseK8sObjectsFromYAMLManifest(manifest)
	if err != nil {
		return err
	}
	for _, obj := range objs {
		// check namespaced object if namespace property has been existed
		if obj.Namespace == "" && o.isNamespacedObject(obj) {
			obj.Namespace = ns
			obj.UnstructuredObject().SetNamespace(ns)
		}
		if o.isNamespacedObject(obj) {
			fmt.Fprintf(o.writer, "✔️ Installed %s:%s:%s.\n", obj.Kind, obj.Name, obj.Namespace)
		} else {
			fmt.Fprintf(o.writer, "✔️ Installed %s::%s.\n", obj.Kind, obj.Name)
		}
		if err := o.kubeCli.ApplyObject(obj.UnstructuredObject()); err != nil {
			return err
		}
	}
	return nil
}

// DeleteManifests delete component manifests to k8s cluster
func (o *K8sInstaller) DeleteManifests(manifestMap map[ComponentName]string) error {
	if o.kubeCli == nil {
		return errors.New("no injected k8s cli into K8sInstaller")
	}
	for name, manifest := range manifestMap {
		namespace := o.components[name].Namespace()
		if err := o.deleteManifest(manifest, namespace); err != nil {
			return fmt.Errorf("component %s DeleteManifest err: %v", name, err)
		}
	}
	return nil
}

// WriteManifests write component manifests to local files
func (o *K8sInstaller) WriteManifests(manifestMap map[ComponentName]string) error {
	if o.kubeCli == nil {
		return errors.New("no injected k8s cli into K8sInstaller")
	}
	rootPath, _ := os.Getwd()
	for name, manifest := range manifestMap {
		fileName := filepath.Join(rootPath, string(name)+".yaml")
		util.WriteFileString(fileName, manifest, 0o644)
	}
	return nil
}

// deleteManifest delete manifest to certain namespace
func (o *K8sInstaller) deleteManifest(manifest string, ns string) error {
	objs, err := object.ParseK8sObjectsFromYAMLManifest(manifest)
	if err != nil {
		return err
	}
	for _, obj := range objs {
		// check namespaced object if namespace property has been existed
		if obj.Namespace == "" && o.isNamespacedObject(obj) {
			obj.Namespace = ns
			obj.UnstructuredObject().SetNamespace(ns)
		}
		if o.isNamespacedObject(obj) {
			fmt.Fprintf(o.writer, "✔️ Removed %s:%s:%s.\n", obj.Kind, obj.Name, obj.Namespace)
		} else {
			fmt.Fprintf(o.writer, "✔️ Removed %s::%s.\n", obj.Kind, obj.Name)
		}
		if err := o.kubeCli.DeleteObject(obj.UnstructuredObject()); err != nil {
			return err
		}
	}

	return nil
}

func (o *K8sInstaller) isNamespacedObject(obj *object.K8sObject) bool {
	if obj.Kind != "CustomResourceDefinition" && obj.Kind != "ClusterRole" && obj.Kind != "ClusterRoleBinding" {
		return true
	}

	return false
}

func NewK8sInstaller(profile *helm.Profile, cli kubernetes.CLIClient, writer io.Writer, quiet bool, devel bool, installerMode InstallerMode, execOptions ...ExecutionOptions) (*K8sInstaller, error) {
	if profile == nil {
		return nil, errors.New("install profile is empty")
	}
	options := ExecutionOptions{ProfilePersistence: PersistProfile}
	if len(execOptions) > 0 {
		options = execOptions[0]
	}
	releaseName := options.ReleaseName
	if releaseName == "" {
		releaseName = profile.Charts.Higress.Name
	}
	releaseNamespace := options.ReleaseNamespace
	if releaseNamespace == "" {
		releaseNamespace = profile.Global.Namespace
	}
	// initialize server info
	serverInfo, _ := NewServerInfo(cli)
	fmt.Fprintf(writer, "\n⌛️ Detecting kubernetes version ... ")
	capabilities, err := serverInfo.GetCapabilities()
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(writer, "%s\n", capabilities.KubeVersion.Version)
	// initialize components
	higressVersion := profile.Charts.Higress.Version
	if installerMode == UninstallInstallerMode {
		// uninstall
		higressVersion = profile.HigressVersion
	}
	components := make(map[ComponentName]Component)
	opts := []ComponentOption{
		WithComponentName(releaseName),
		WithComponentNamespace(releaseNamespace),
		WithComponentChartPath(profile.InstallPackagePath),
		WithComponentVersion(higressVersion),
		WithComponentRepoURL(profile.Charts.Higress.Url),
		WithComponentChartName(profile.Charts.Higress.Name),
		WithComponentCapabilities(capabilities),
		WithDevel(devel),
	}
	if quiet {
		opts = append(opts, WithQuiet())
	}
	higressComponent, err := NewHigressComponent(cli, profile, writer, opts...)
	if err != nil {
		return nil, fmt.Errorf("NewHigressComponent failed, err: %s", err)
	}
	components[Higress] = higressComponent

	if profile.IstioEnabled() {
		istioNamespace := profile.GetIstioNamespace()
		if len(istioNamespace) == 0 {
			istioNamespace = DefaultIstioNamespace
		}
		opts := []ComponentOption{
			WithComponentNamespace(istioNamespace),
			WithComponentVersion("1.18.2"),
			WithComponentRepoURL("embed://istiobase"),
			WithComponentChartName("istio"),
			WithComponentCapabilities(capabilities),
		}
		if quiet {
			opts = append(opts, WithQuiet())
		}

		istioCRDComponent, err := NewIstioCRDComponent(cli, profile, writer, opts...)
		if err != nil {
			return nil, fmt.Errorf("NewIstioCRDComponent failed, err: %s", err)
		}
		components[Istio] = istioCRDComponent
	}

	if profile.GatewayAPIEnabled() {
		opts := []ComponentOption{
			WithComponentNamespace(DefaultGatewayAPINamespace),
			WithComponentVersion("1.0.0"),
			WithComponentRepoURL("embed://gatewayapi"),
			WithComponentChartName("gatewayAPI"),
			WithComponentCapabilities(capabilities),
		}
		if quiet {
			opts = append(opts, WithQuiet())
		}

		gatewayAPIComponent, err := NewGatewayAPIComponent(cli, profile, writer, opts...)
		if err != nil {
			return nil, fmt.Errorf("NewGatewayAPIComponent failed, err: %s", err)
		}
		components[GatewayAPI] = gatewayAPIComponent
	}

	profileStore, err := NewConfigmapProfileStore(cli)
	if err != nil {
		return nil, err
	}

	op := &K8sInstaller{
		profile:      profile,
		components:   components,
		kubeCli:      cli,
		writer:       writer,
		profileStore: profileStore,
		execOptions:  options,
	}
	return op, nil
}
