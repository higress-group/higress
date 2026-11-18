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
	"strings"

	"github.com/alibaba/higress/hgctl/pkg/helm"
	"github.com/alibaba/higress/hgctl/pkg/installer"
	"github.com/alibaba/higress/hgctl/pkg/util"
	"github.com/alibaba/higress/v2/pkg/cmd/options"
	"github.com/spf13/cobra"
)

type uninstallArgs struct {
	// purgeResources delete  all of installed resources.
	purgeResources bool
}

func addUninstallFlags(cmd *cobra.Command, args *uninstallArgs) {
	cmd.PersistentFlags().BoolVarP(&args.purgeResources, "purge-resources", "", false,
		"Delete  all of IstioAPI,GatewayAPI resources")
}

// newUninstallCmd command uninstalls Istio from a cluster
func newUninstallCmd() *cobra.Command {
	uiArgs := &uninstallArgs{}
	uninstallCmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Uninstall higress from a cluster",
		Long:  "The uninstall command uninstalls higress from a cluster or local environment",
		Example: `# Uninstall higress
  hgctl uninstal

  # Uninstall higress, istioAPI and GatewayAPI from a cluster
  hgctl uninstall --purge-resources
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return uninstall(cmd.OutOrStdout(), uiArgs)
		},
	}
	addUninstallFlags(uninstallCmd, uiArgs)
	flags := uninstallCmd.Flags()
	options.AddKubeConfigFlags(flags)
	return uninstallCmd
}

// uninstall uninstalls control plane by either pruning by target revision or deleting specified manifests.
func uninstall(writer io.Writer, uiArgs *uninstallArgs) error {
	fmt.Fprintf(writer, "⌛️ Checking higress installed profiles...\n")
	profileContexts, _ := getAllProfiles()
	if len(profileContexts) == 0 {
		fmt.Fprintf(writer, "\nHigress hasn't been installed yet!\n")
		return nil
	}

	setFlags := make([]string, 0)

	profileContext := promptProfileContexts(writer, profileContexts)
	_, profile, err := helm.GenProfileFromProfileContent(util.ToYAML(profileContext.Profile), "", setFlags)
	if err != nil {
		return err
	}

	fmt.Fprintf(writer, "\n🧐 Validating Profile: \"%s\" \n", profileContext.PathOrName)
	err = profile.Validate()
	if err != nil {
		return err
	}

	if !promptUninstall(writer) {
		return nil
	}

	if profile.Global.Install == helm.InstallK8s || profile.Global.Install == helm.InstallLocalK8s {
		if profile.Global.EnableIstioAPI {
			profile.Global.EnableIstioAPI = uiArgs.purgeResources
		}
		if profile.Global.EnableGatewayAPI {
			profile.Global.EnableGatewayAPI = uiArgs.purgeResources
		}
	}

	err = uninstallManifests(profile, writer, uiArgs)
	if err != nil {
		return err
	}

	// Remove "~/.hgctl/profiles/install.yaml"
	if oldProfileName, isExisted := installer.GetInstalledYamlPath(); isExisted {
		_ = os.Remove(oldProfileName)
	}

	return nil
}

func promptUninstall(writer io.Writer) bool {
	answer := ""
	for {
		fmt.Fprintf(writer, "All Higress resources will be uninstalled from the cluster. \nProceed? (y/N)")
		fmt.Scanln(&answer)
		val := strings.TrimSpace(answer)
		if val == "y" || val == "Y" {
			fmt.Fprintf(writer, "\n")
			return true
		}
		if val == "" || val == "n" || val == "N" {
			fmt.Fprintf(writer, "Cancelled.\n")
			return false
		}
	}
}


func uninstallManifests(profile *helm.Profile, writer io.Writer, uiArgs *uninstallArgs) error {
	installer, err := installer.NewInstaller(profile, writer, false, false, installer.UninstallInstallerMode)
	if err != nil {
		return err
	}

	err = installer.UnInstall()
	if err != nil {
		return err
	}

	return nil
}
