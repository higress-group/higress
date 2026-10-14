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

	"github.com/alibaba/higress/hgctl/pkg/helm"
)

type DockerInstaller struct {
	started      bool
	standalone   *StandaloneComponent
	profile      *helm.Profile
	writer       io.Writer
	profileStore ProfileStore
	upgradeFunc  func() error
}

func (d *DockerInstaller) Install() error {
	fmt.Fprintf(d.writer, "\n⌛️ Processing installation... \n\n")

	if err := d.standalone.Install(); err != nil {
		return err
	}

	profileName, err1 := d.profileStore.Save(d.profile)
	if err1 != nil {
		return err1
	}
	fmt.Fprintf(d.writer, "\n✔️ Wrote Profile: \"%s\" \n", profileName)

	fmt.Fprintf(d.writer, "\n🎊 Install All Resources Complete!\n")
	return nil
}

func (d *DockerInstaller) UnInstall() error {
	fmt.Fprintf(d.writer, "\n⌛️ Processing uninstallation... \n\n")

	if err := d.standalone.UnInstall(); err != nil {
		return err
	}

	profileName, err1 := d.profileStore.Delete(d.profile)
	if err1 != nil {
		return err1
	}
	fmt.Fprintf(d.writer, "\n✔️ Removed Profile: \"%s\" \n", profileName)

	fmt.Fprintf(d.writer, "\n🎊 Uninstall All Resources Complete!\n")
	return nil
}

func (d *DockerInstaller) Upgrade() error {
	fmt.Fprintf(d.writer, "\n⌛️ Processing upgrade... \n\n")

	upgrade := d.standalone.Upgrade
	if d.upgradeFunc != nil {
		upgrade = d.upgradeFunc
	}
	if err := upgrade(); err != nil {
		return err
	}

	if err := d.syncStoredVersion(); err != nil {
		return err
	}

	fmt.Fprintf(d.writer, "\n🎊 Install All Resources Complete!\n")
	return nil
}

func (d *DockerInstaller) syncStoredVersion() error {
	version, err := d.standalone.agent.strictVersion()
	if err != nil {
		return err
	}

	profiles, err := d.profileStore.List()
	if err != nil {
		return fmt.Errorf("list stored profiles: %w", err)
	}

	var storedProfile *helm.Profile
	for _, profileContext := range profiles {
		if profileContext == nil || profileContext.Profile == nil || profileContext.Install != helm.InstallLocalDocker {
			continue
		}
		if storedProfile != nil {
			return errors.New("multiple stored local-docker profiles found")
		}
		storedProfile = profileContext.Profile
	}
	if storedProfile == nil {
		return errors.New("stored local-docker profile not found")
	}

	updatedProfile := *storedProfile
	updatedProfile.HigressVersion = version
	if _, err := d.profileStore.Save(&updatedProfile); err != nil {
		return fmt.Errorf("save stored local-docker profile: %w", err)
	}
	if d.profile != nil {
		d.profile.HigressVersion = version
	}
	return nil
}

func NewDockerInstaller(profile *helm.Profile, writer io.Writer, quiet bool) (*DockerInstaller, error) {
	if profile == nil {
		return nil, errors.New("install profile is empty")
	}
	// initialize components
	opts := []ComponentOption{
		WithComponentVersion(profile.Charts.Standalone.Version),
		WithComponentRepoURL(profile.Charts.Standalone.Url),
		WithComponentChartName(profile.Charts.Standalone.Name),
	}
	if quiet {
		opts = append(opts, WithQuiet())
	}
	standaloneComponent, err := NewStandaloneComponent(profile, writer, opts...)
	if err != nil {
		return nil, fmt.Errorf("NewStandaloneComponent failed, err: %s", err)
	}

	profileInstalledPath, err1 := GetProfileInstalledPath()
	if err1 != nil {
		return nil, err1
	}
	profileStore, err2 := NewFileDirProfileStore(profileInstalledPath)
	if err2 != nil {
		return nil, err2
	}
	op := &DockerInstaller{
		profile:      profile,
		standalone:   standaloneComponent,
		writer:       writer,
		profileStore: profileStore,
	}
	return op, nil
}
