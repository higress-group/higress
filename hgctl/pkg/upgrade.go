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
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/alibaba/higress/hgctl/pkg/helm"
	"github.com/alibaba/higress/hgctl/pkg/installer"
	"github.com/alibaba/higress/hgctl/pkg/kubernetes"
	"github.com/alibaba/higress/hgctl/pkg/util"
	"github.com/alibaba/higress/v2/pkg/cmd/options"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type upgradeArgs struct {
	*InstallArgs
}

var (
	getAllProfilesForUpgrade = getAllProfiles
	promptUpgradeForUpgrade  = promptUpgrade
	newInstallerForUpgrade   = installer.NewInstaller
	newRecoveredInstaller    = installer.NewInstallerWithOptions
	newHelmReaderForUpgrade  = func() installer.HelmReleaseReader {
		return installer.NewHelmReleaseReader()
	}
	strictProfileCollisionCheckForUpgrade           = checkRecoveredProfileCollisions
	upgradeStdin                          io.Reader = os.Stdin
	isUpgradeTerminal                               = func() bool { return term.IsTerminal(int(os.Stdin.Fd())) }
)

var errRecoveredHelmCancelled = errors.New("recovered Helm upgrade cancelled before mutation")

func addUpgradeFlags(cmd *cobra.Command, args *upgradeArgs) {
	cmd.PersistentFlags().StringSliceVarP(&args.InFilenames, "filename", "f", nil, filenameFlagHelpStr)
	cmd.PersistentFlags().StringArrayVarP(&args.Set, "set", "s", nil, setFlagHelpStr)
	cmd.PersistentFlags().StringVarP(&args.ManifestsPath, "manifests", "d", "", manifestsFlagHelpStr)
	cmd.PersistentFlags().BoolVar(&args.Devel, "devel", false, "use development versions (alpha, beta, and release candidate releases), If version is set, this is ignored")
	cmd.PersistentFlags().BoolVar(&args.FromHelm, "from-helm", false, "recover current upgrade state from an existing Higress Helm release")
	cmd.PersistentFlags().StringVar(&args.HelmRelease, "helm-release", "", "Helm release name to recover when using --from-helm")
	cmd.PersistentFlags().StringVar(&args.HelmNamespace, "helm-namespace", "", "Helm release namespace to recover when using --from-helm")
	cmd.PersistentFlags().BoolVar(&args.Yes, "yes", false, "approve the recovered Helm upgrade after validation; required for non-interactive --from-helm")
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
	if iArgs.FromHelm {
		return upgradeFromHelm(writer, iArgs)
	}
	if iArgs.HelmRelease != "" || iArgs.HelmNamespace != "" || iArgs.Yes {
		return errors.New("--helm-release, --helm-namespace, and --yes are only valid with --from-helm")
	}
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

func upgradeFromHelm(writer io.Writer, iArgs *InstallArgs) error {
	setFlags := applyFlagAliases(iArgs.Set, iArgs.ManifestsPath)
	if helm.GetValueForSetFlag(setFlags, "profile") != "" {
		return errors.New("--from-helm does not allow --set profile=...; the source release determines the recovered profile")
	}

	reader := newHelmReaderForUpgrade()
	candidates, err := reader.ListReleases(installer.HelmReleaseSelector{
		Name:      iArgs.HelmRelease,
		Namespace: iArgs.HelmNamespace,
	})
	if err != nil {
		return err
	}
	sortHelmCandidates(candidates)
	input := bufio.NewReader(upgradeStdin)
	selected, err := selectRecoveredHelmRelease(writer, input, candidates, isUpgradeTerminal())
	if err != nil {
		if errors.Is(err, errRecoveredHelmCancelled) {
			return nil
		}
		return err
	}

	retainedValues, err := reader.GetRetainedValues(selected)
	if err != nil {
		return err
	}
	profile, diagnostics, err := helm.ReconstructProfileFromHelm(selected, retainedValues)
	if err != nil {
		return err
	}
	for _, diagnostic := range diagnostics {
		fmt.Fprintf(writer, "⚠️  Recovered Helm diagnostic: %s\n", diagnostic.String())
	}

	if err := strictProfileCollisionCheckForUpgrade(selected.Namespace); err != nil {
		return err
	}

	originalProfileName := profile.Profile
	originalInstall := profile.Global.Install
	originalNamespace := profile.Global.Namespace

	for _, filename := range iArgs.InFilenames {
		fileOverlay, err := helm.GetValuesOverylayFromFiles([]string{filename})
		if err != nil {
			return err
		}
		if err := helm.OverlayRecoveredProfile(profile, fileOverlay); err != nil {
			return err
		}
	}
	setOverlay, err := helm.GetProfileOverlay("", setFlags)
	if err != nil {
		return err
	}
	if err := helm.OverlayRecoveredProfile(profile, setOverlay); err != nil {
		return err
	}
	if err := validateRecoveredProtectedFields(profile, originalProfileName, originalInstall, originalNamespace); err != nil {
		return err
	}

	fmt.Fprintf(writer, "\n🧐 Validating recovered Helm release: \"%s/%s\" \n", selected.Namespace, selected.Name)
	if err := profile.Validate(); err != nil {
		return err
	}

	if !iArgs.Yes {
		if !isUpgradeTerminal() {
			return errors.New("--from-helm requires --yes in non-interactive mode after validation succeeds")
		}
		ok, err := promptRecoveredHelmUpgrade(writer, input, selected)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
	}

	return upgradeRecoveredHelmManifests(profile, selected, writer, iArgs.Devel)
}

func sortHelmCandidates(candidates []installer.HelmReleaseCandidate) {
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Namespace != candidates[j].Namespace {
			return candidates[i].Namespace < candidates[j].Namespace
		}
		return candidates[i].Name < candidates[j].Name
	})
}

func selectRecoveredHelmRelease(writer io.Writer, input *bufio.Reader, candidates []installer.HelmReleaseCandidate, terminal bool) (installer.HelmReleaseCandidate, error) {
	if len(candidates) == 0 {
		return installer.HelmReleaseCandidate{}, errors.New("no deployed Higress Helm release with parent chart \"higress\" was found")
	}
	if len(candidates) == 1 {
		fmt.Fprintf(writer, "\nFound Higress Helm release: %s/%s (chart %s %s)\n", candidates[0].Namespace, candidates[0].Name, candidates[0].ChartName, candidates[0].ChartVersion)
		return candidates[0], nil
	}
	if !terminal {
		return installer.HelmReleaseCandidate{}, errors.New("multiple Higress Helm releases matched; set --helm-release and --helm-namespace to select exactly one in non-interactive mode")
	}
	fmt.Fprintf(writer, "\nPlease select Higress Helm release to recover:\n")
	for idx, candidate := range candidates {
		fmt.Fprintf(writer, "\n%d: namespace: %s, release: %s, chart: %s %s", idx+1, candidate.Namespace, candidate.Name, candidate.ChartName, candidate.ChartVersion)
	}
	for {
		fmt.Fprintf(writer, "\nPlease input 1 to %d to select, or n to cancel:", len(candidates))
		line, err := input.ReadString('\n')
		if err != nil && len(line) == 0 {
			return installer.HelmReleaseCandidate{}, errRecoveredHelmCancelled
		}
		answer := strings.TrimSpace(line)
		if isCancelAnswer(answer) {
			fmt.Fprintf(writer, "Cancelled.\n")
			return installer.HelmReleaseCandidate{}, errRecoveredHelmCancelled
		}
		index, err := strconv.Atoi(answer)
		if err == nil && index >= 1 && index <= len(candidates) {
			return candidates[index-1], nil
		}
	}
}

func promptRecoveredHelmUpgrade(writer io.Writer, input *bufio.Reader, release installer.HelmReleaseCandidate) (bool, error) {
	for {
		fmt.Fprintf(writer, "Recovered Helm release %s/%s will be upgraded without saving an hgctl Profile. \nProceed? (y/N)", release.Namespace, release.Name)
		line, err := input.ReadString('\n')
		if err != nil && len(line) == 0 {
			return false, errors.New("recovered Helm upgrade confirmation cancelled before mutation")
		}
		answer := strings.TrimSpace(line)
		if answer == "y" {
			fmt.Fprintf(writer, "\n")
			return true, nil
		}
		if answer == "" || answer == "N" || isCancelAnswer(answer) {
			fmt.Fprintf(writer, "Cancelled.\n")
			return false, nil
		}
	}
}

func isCancelAnswer(answer string) bool {
	switch strings.ToLower(answer) {
	case "n", "no", "q", "quit", "cancel", "c":
		return true
	default:
		return false
	}
}

func checkRecoveredProfileCollisions(namespace string) error {
	home, err := installer.GetHomeDir()
	if err != nil {
		return err
	}
	profilesPath := filepath.Join(home, installer.HgctlHomeDirPath, installer.ProfileInstalledPath)
	fileContexts, err := installer.StrictFileProfileCollisions(profilesPath, namespace)
	if err != nil {
		return fmt.Errorf("strict profile file collision check failed: %w", err)
	}
	if len(fileContexts) > 0 {
		return fmt.Errorf("stored hgctl Profile targets namespace %q at %s; choose normal Profile upgrade or --from-helm, not both", namespace, fileContexts[0].PathOrName)
	}

	cliClient, err := kubernetes.NewCLIClient(options.DefaultConfigFlags.ToRawKubeConfigLoader())
	if err != nil {
		return fmt.Errorf("strict ConfigMap profile collision check failed: %w", err)
	}
	configmapProfileStore, err := installer.NewConfigmapProfileStore(cliClient)
	if err != nil {
		return err
	}
	strictStore, ok := configmapProfileStore.(*installer.ConfigmapProfileStore)
	if !ok {
		return errors.New("strict ConfigMap profile collision check unavailable")
	}
	configmapContexts, err := strictStore.StrictCollisions(namespace)
	if err != nil {
		return fmt.Errorf("strict ConfigMap profile collision check failed: %w", err)
	}
	if len(configmapContexts) > 0 {
		return fmt.Errorf("ConfigMap hgctl Profile targets namespace %q at %s; choose normal Profile upgrade or --from-helm, not both", namespace, configmapContexts[0].PathOrName)
	}
	return nil
}

func validateRecoveredProtectedFields(profile *helm.Profile, originalProfileName string, originalInstall helm.InstallMode, originalNamespace string) error {
	if profile.Profile != originalProfileName {
		return fmt.Errorf("--from-helm overlays may not change recovered profile from %q to %q", originalProfileName, profile.Profile)
	}
	if profile.Global.Install != originalInstall {
		return fmt.Errorf("--from-helm overlays may not change recovered global.install from %q to %q", originalInstall, profile.Global.Install)
	}
	if profile.Global.Namespace != originalNamespace {
		return fmt.Errorf("--from-helm overlays may not change selected release namespace from %q to %q", originalNamespace, profile.Global.Namespace)
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

func upgradeRecoveredHelmManifests(profile *helm.Profile, release installer.HelmReleaseCandidate, writer io.Writer, devel bool) error {
	installer, err := newRecoveredInstaller(profile, writer, false, devel, installer.UpgradeInstallerMode, installer.ExecutionOptions{
		ProfilePersistence: installer.DoNotPersistProfile,
		SourceMode:         installer.RecoveredHelmExecutionSource,
		ReleaseName:        release.Name,
		ReleaseNamespace:   release.Namespace,
	})
	if err != nil {
		return err
	}
	if err := installer.Upgrade(); err != nil {
		return fmt.Errorf("recovered Helm upgrade failed; resources may be partially applied and no rollback was performed. Retry with the same selected release, pinned target chart version, ordered -f files, and --set values: %w", err)
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
