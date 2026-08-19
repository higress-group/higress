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

package hgctl

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/alibaba/higress/hgctl/pkg/helm"
	"github.com/alibaba/higress/hgctl/pkg/installer"
	"github.com/alibaba/higress/hgctl/pkg/kubernetes"
	"github.com/alibaba/higress/hgctl/pkg/util"
	"github.com/alibaba/higress/v2/pkg/cmd/options"
	"github.com/spf13/cobra"
)

type upgradeArgs struct {
	*InstallArgs
}

var (
	getAllProfilesForUpgrade = getAllProfiles
	promptUpgradeForUpgrade  = promptUpgrade
	newInstallerForUpgrade   = installer.NewInstaller
)

func addUpgradeFlags(cmd *cobra.Command, args *upgradeArgs) {
	cmd.PersistentFlags().StringSliceVarP(&args.InFilenames, "filename", "f", nil, filenameFlagHelpStr)
	cmd.PersistentFlags().StringArrayVarP(&args.Set, "set", "s", nil, setFlagHelpStr)
	cmd.PersistentFlags().StringVarP(&args.ManifestsPath, "manifests", "d", "", manifestsFlagHelpStr)
	cmd.PersistentFlags().BoolVar(&args.Devel, "devel", false, "use development versions (alpha, beta, and release candidate releases), If version is set, this is ignored")
}

// newUpgradeCmd upgrades Istio control plane in-place with eligibility checks.
func newUpgradeCmd() *cobra.Command {
	upgradeArgs := &upgradeArgs{
		InstallArgs: &InstallArgs{},
	}
	upgradeCmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade Higress in-place",
		Long: "The upgrade command is an alias for the install command" +
			" that performs additional upgrade-related checks.",
		RunE: func(cmd *cobra.Command, args []string) (e error) {
			return upgrade(cmd.OutOrStdout(), upgradeArgs.InstallArgs)
		},
	}
	addUpgradeFlags(upgradeCmd, upgradeArgs)
	flags := upgradeCmd.Flags()
	options.AddKubeConfigFlags(flags)
	return upgradeCmd
}

// upgrade upgrade higress resources from the cluster.
func upgrade(writer io.Writer, iArgs *InstallArgs) error {
	setFlags := applyFlagAliases(iArgs.Set, iArgs.ManifestsPath)
	fmt.Fprintf(writer, "⌛️ Checking higress installed profiles...\n")
	profileContexts, _ := getAllProfilesForUpgrade()
	if len(profileContexts) == 0 {
		fmt.Fprintf(writer, "\nHigress hasn't been installed yet!\n")
		return nil
	}

	valuesOverlay, err := helm.GetValuesOverylayFromFiles(iArgs.InFilenames)
	if err != nil {
		return err
	}
	overlayYAML, err := helm.GetProfileOverlay(valuesOverlay, setFlags)
	if err != nil {
		return err
	}

	profileContext := promptProfileContexts(writer, profileContexts)

	_, profile, err := helm.GenProfileFromProfileContent(util.ToYAML(profileContext.Profile), valuesOverlay, setFlags)
	if err != nil {
		return err
	}
	if profileContext.Profile.Global.Install == helm.InstallLocalDocker {
		if err := validateLocalDockerUpgradeOverlay(profileContext.Profile, overlayYAML); err != nil {
			return err
		}
	}

	fmt.Fprintf(writer, "\n🧐 Validating Profile: \"%s\" \n", profileContext.PathOrName)
	err = profile.Validate()
	if err != nil {
		return err
	}

	if !promptUpgradeForUpgrade(writer) {
		return nil
	}

	err = upgradeManifests(profile, writer, iArgs.Devel)
	if err != nil {
		return err
	}

	// Remove "~/.hgctl/profiles/install.yaml"
	if oldProfileName, isExisted := installer.GetInstalledYamlPath(); isExisted {
		_ = os.Remove(oldProfileName)
	}

	return nil
}

func validateLocalDockerUpgradeOverlay(baseline *helm.Profile, overlayYAML string) error {
	unsupported, err := helm.UnsupportedLocalDockerUpgradeOverlayPaths(baseline, overlayYAML)
	if err != nil {
		return err
	}
	if len(unsupported) > 0 {
		return fmt.Errorf("local-docker upgrade does not support overlay fields: %s", strings.Join(unsupported, ", "))
	}
	return nil
}

func promptUpgrade(writer io.Writer) bool {
	answer := ""
	for {
		fmt.Fprintf(writer, "All Higress resources will be upgrade from the cluster. \nProceed? (y/N)")
		fmt.Scanln(&answer)
		if strings.TrimSpace(answer) == "y" {
			fmt.Fprintf(writer, "\n")
			return true
		}
		if strings.TrimSpace(answer) == "N" {
			fmt.Fprintf(writer, "Cancelled.\n")
			return false
		}
	}
}

func upgradeManifests(profile *helm.Profile, writer io.Writer, devel bool) error {
	installer, err := newInstallerForUpgrade(profile, writer, false, devel, installer.UpgradeInstallerMode)
	if err != nil {
		return err
	}

	err = installer.Upgrade()
	if err != nil {
		return err
	}

	return nil
}

func getAllProfiles() ([]*installer.ProfileContext, error) {
	profileContexts := make([]*installer.ProfileContext, 0)
	profileInstalledPath, err := installer.GetProfileInstalledPath()
	if err != nil {
		return profileContexts, nil
	}
	fileProfileStore, err := installer.NewFileDirProfileStore(profileInstalledPath)
	if err != nil {
		return profileContexts, nil
	}
	fileProfileContexts, err := fileProfileStore.List()
	if err == nil {
		profileContexts = append(profileContexts, fileProfileContexts...)
	}

	cliClient, err := kubernetes.NewCLIClient(options.DefaultConfigFlags.ToRawKubeConfigLoader())
	if err != nil {
		return profileContexts, nil
	}
	configmapProfileStore, err := installer.NewConfigmapProfileStore(cliClient)
	if err != nil {
		return profileContexts, nil
	}

	configmapProfileContexts, err := configmapProfileStore.List()
	if err == nil {
		profileContexts = append(profileContexts, configmapProfileContexts...)
	}
	return profileContexts, nil
}

func promptProfileContexts(writer io.Writer, profileContexts []*installer.ProfileContext) *installer.ProfileContext {
	if len(profileContexts) == 1 {
		fmt.Fprintf(writer, "\nFound a profile::  ")
	} else {
		fmt.Fprintf(writer, "\nPlease select higress installed configuration profiles:\n")
	}
	index := 1
	for _, profileContext := range profileContexts {
		if len(profileContexts) > 1 {
			fmt.Fprintf(writer, "\n%d: ", index)
		}
		fmt.Fprintf(writer, "install mode: %s, profile location: %s", profileContext.Install, profileContext.PathOrName)
		if len(profileContext.Namespace) > 0 {
			fmt.Fprintf(writer, ", namespace: %s", profileContext.Namespace)
		}
		if len(profileContext.HigressVersion) > 0 {
			fmt.Fprintf(writer, ", version: %s", profileContext.HigressVersion)
		}
		fmt.Fprintf(writer, "\n")
		index++
	}

	if len(profileContexts) == 1 {
		return profileContexts[0]
	}

	answer := ""
	for {
		fmt.Fprintf(writer, "\nPlease input 1 to %d select, input your selection:", len(profileContexts))
		fmt.Scanln(&answer)
		index, err := strconv.Atoi(answer)
		if err == nil && index >= 1 && index <= len(profileContexts) {
			return profileContexts[index-1]
		}
	}
}
