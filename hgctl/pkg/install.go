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
	"github.com/alibaba/higress/v2/pkg/cmd/options"
	"github.com/spf13/cobra"
)

const (
	setFlagHelpStr = `Override an higress profile value, e.g. to choose a profile
(--set profile=local-k8s), or override profile values (--set gateway.replicas=2), or override helm values (--set values.global.proxy.resources.requests.cpu=500m).`
	// manifestsFlagHelpStr is the command line description for --manifests
	manifestsFlagHelpStr = `Specify a path to a directory of profiles
(e.g. ~/Downloads/higress/manifests).`
	filenameFlagHelpStr = "Path to file containing helm custom values"
	outputHelpstr       = "Specify a file to write profile yaml"

	profileNameK8s         = "k8s"
	profileNameLocalK8s    = "local-k8s"
	profileNameLocalDocker = "local-docker"
)

type InstallArgs struct {
	// InFilenames is a filename to helm custom values
	InFilenames []string
	// KubeConfigPath is the path to kube config file.
	KubeConfigPath string
	// Context is the cluster context in the kube config
	Context string
	// Set is a string with element format "path=value" where path is an profile path and the value is a
	// value to set the node at that path to.
	Set []string
	// ManifestsPath is a path to a ManifestsPath and profiles directory in the local filesystem with a release tgz.
	ManifestsPath string
	// Devel if set true when version is latest, it will get latest version, otherwise it will get latest stable version
	Devel bool
	// FromHelm recovers current upgrade state from an existing Higress Helm release.
	FromHelm bool
	// HelmRelease selects the Helm release name for --from-helm.
	HelmRelease string
	// HelmNamespace selects the Helm release namespace for --from-helm.
	HelmNamespace string
	// Yes approves non-interactive recovered Helm mutation after validation.
	Yes bool
}

func (a *InstallArgs) String() string {
	var b strings.Builder
	b.WriteString("KubeConfigPath:   " + a.KubeConfigPath + "\n")
	b.WriteString("Context:          " + a.Context + "\n")
	b.WriteString("Set:              " + fmt.Sprint(a.Set) + "\n")
	b.WriteString("ManifestsPath:    " + a.ManifestsPath + "\n")
	return b.String()
}

func addInstallFlags(cmd *cobra.Command, args *InstallArgs) {
	cmd.PersistentFlags().StringSliceVarP(&args.InFilenames, "filename", "f", nil, filenameFlagHelpStr)
	cmd.PersistentFlags().StringArrayVarP(&args.Set, "set", "s", nil, setFlagHelpStr)
	cmd.PersistentFlags().StringVarP(&args.ManifestsPath, "manifests", "d", "", manifestsFlagHelpStr)
	cmd.PersistentFlags().BoolVar(&args.Devel, "devel", false, "use development versions (alpha, beta, and release candidate releases), If version is set, this is ignored")
}

// --manifests is an alias for --set installPackagePath=
func applyFlagAliases(flags []string, manifestsPath string) []string {
	if manifestsPath != "" {
		flags = append(flags, fmt.Sprintf("installPackagePath=%s", manifestsPath))
	}
	return flags
}

// newInstallCmd generates a higress install manifest and applies it to a cluster
func newInstallCmd() *cobra.Command {
	iArgs := &InstallArgs{}
	installCmd := &cobra.Command{
		Use:   "install",
		Short: "Applies an higress manifest, installing or reconfiguring higress on a cluster.",
		Long:  "The install command generates an higress install manifest and applies it to a cluster.",
		// nolint: lll
		Example: `  # Apply a default higress installation
  hgctl install

  # Install higress on local kubernetes cluster
  hgctl install --set profile=local-k8s

  # Install higress on local docker environment with specific gateway port
  hgctl install --set profile=local-docker --set gateway.httpPort=80 --set gateway.httpsPort=443

  # To override profile setting
  hgctl install --set profile=local-k8s  --set global.enableIstioAPI=true --set gateway.replicas=2"

  # To override helm setting
  hgctl install --set profile=local-k8s  --set values.global.proxy.resources.requests.cpu=500m"


`,
		Args: cobra.ExactArgs(0),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return install(cmd.OutOrStdout(), iArgs)
		},
	}
	addInstallFlags(installCmd, iArgs)
	flags := installCmd.Flags()
	options.AddKubeConfigFlags(flags)
	return installCmd
}

func install(writer io.Writer, iArgs *InstallArgs) error {
	setFlags := applyFlagAliases(iArgs.Set, iArgs.ManifestsPath)

	// check profileName
	psf := helm.GetValueForSetFlag(setFlags, "profile")
	if len(psf) == 0 {
		psf = promptProfileName(writer)
		setFlags = append(setFlags, fmt.Sprintf("profile=%s", psf))
	}

	if !promptInstall(writer, psf) {
		return nil
	}

	_, profile, profileName, err := helm.GenerateConfig(iArgs.InFilenames, setFlags)
	if err != nil {
		return fmt.Errorf("generate config: %v", err)
	}

	fmt.Fprintf(writer, "\n🧐 Validating Profile: \"%s\" \n", profileName)
	err = profile.Validate()
	if err != nil {
		return err
	}

	err = installManifests(profile, writer, iArgs.Devel)
	if err != nil {
		return fmt.Errorf("failed to install manifests: %v", err)
	}

	// Remove "~/.hgctl/profiles/install.yaml"
	if oldProfileName, isExisted := installer.GetInstalledYamlPath(); isExisted {
		_ = os.Remove(oldProfileName)
	}

	return nil
}

func promptInstall(writer io.Writer, profileName string) bool {
	answer := ""
	for {
		fmt.Fprintf(writer, "\nThis will install Higress \"%s\" profile into the cluster. \nProceed? (y/N)", profileName)
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

func promptProfileName(writer io.Writer) string {
	answer := ""
	fmt.Fprintf(writer, "\nPlease select higress install configuration profile:\n")
	fmt.Fprintf(writer, "\n1.Install higress to local kubernetes cluster like kind etc.\n")
	fmt.Fprintf(writer, "\n2.Install higress to kubernetes cluster\n")
	fmt.Fprintf(writer, "\n3.Install higress to local docker environment\n")
	for {
		fmt.Fprintf(writer, "\nPlease input 1, 2 or 3 to select, input your selection:")
		fmt.Scanln(&answer)
		if strings.TrimSpace(answer) == "1" {
			return profileNameLocalK8s
		}
		if strings.TrimSpace(answer) == "2" {
			return profileNameK8s
		}
		if strings.TrimSpace(answer) == "3" {
			return profileNameLocalDocker
		}
	}
}

func installManifests(profile *helm.Profile, writer io.Writer, devel bool) error {
	installer, err := installer.NewInstaller(profile, writer, false, devel, installer.InstallInstallerMode)
	if err != nil {
		return err
	}

	err = installer.Install()
	if err != nil {
		return err
	}

	return nil
}
