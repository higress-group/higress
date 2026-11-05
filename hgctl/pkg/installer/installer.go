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
	"runtime"

	"github.com/alibaba/higress/hgctl/pkg/helm"
	"github.com/alibaba/higress/hgctl/pkg/kubernetes"
	"github.com/alibaba/higress/v2/pkg/cmd/options"
	"k8s.io/client-go/util/homedir"
)

type InstallerMode int32

const (
	HgctlHomeDirPath           = ".hgctl"
	StandaloneInstalledPath    = "higress-standalone"
	ProfileInstalledPath       = "profiles"
	InstalledYamlFileName      = "install.yaml"
	DefaultGatewayAPINamespace = "gateway-system"
	DefaultIstioNamespace      = "istio-system"
)

const (
	InstallInstallerMode InstallerMode = iota
	UpgradeInstallerMode
	UninstallInstallerMode
)

type Installer interface {
	Install() error
	UnInstall() error
	Upgrade() error
}

type ProfilePersistenceMode int

const (
	PersistProfile ProfilePersistenceMode = iota
	DoNotPersistProfile
)

type ExecutionSourceMode int

const (
	NormalExecutionSource ExecutionSourceMode = iota
	RecoveredHelmExecutionSource
)

type ExecutionOptions struct {
	ProfilePersistence ProfilePersistenceMode
	SourceMode         ExecutionSourceMode
	ReleaseName        string
	ReleaseNamespace   string
}

func NewInstaller(profile *helm.Profile, writer io.Writer, quiet bool, devel bool, installerMode InstallerMode) (Installer, error) {
	return NewInstallerWithOptions(profile, writer, quiet, devel, installerMode, ExecutionOptions{ProfilePersistence: PersistProfile})
}

func NewInstallerWithOptions(profile *helm.Profile, writer io.Writer, quiet bool, devel bool, installerMode InstallerMode, execOptions ExecutionOptions) (Installer, error) {
	switch profile.Global.Install {
	case helm.InstallK8s, helm.InstallLocalK8s:
		cliClient, err := kubernetes.NewCLIClient(options.DefaultConfigFlags.ToRawKubeConfigLoader())
		if err != nil {
			return nil, fmt.Errorf("failed to build kubernetes client: %w", err)
		}
		installer, err := NewK8sInstaller(profile, cliClient, writer, quiet, devel, installerMode, execOptions)
		return installer, err
	case helm.InstallLocalDocker:
		installer, err := NewDockerInstaller(profile, writer, quiet)
		return installer, err
	default:
		return nil, errors.New("install is not supported")
	}
}

func GetHomeDir() (string, error) {
	home := homedir.HomeDir()
	if home == "" {
		return "", fmt.Errorf("No user home environment variable found for OS %s", runtime.GOOS)
	}

	return home, nil
}

func GetHgctlPath() (string, error) {
	home, err := GetHomeDir()
	if err != nil {
		return "", err
	}

	hgctlPath := filepath.Join(home, HgctlHomeDirPath)
	if _, err := os.Stat(hgctlPath); os.IsNotExist(err) {
		if err = os.MkdirAll(hgctlPath, os.ModePerm); err != nil {
			return "", err
		}
	}

	return hgctlPath, nil
}

func GetDefaultInstallPackagePath() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	path := filepath.Join(dir, StandaloneInstalledPath)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err = os.MkdirAll(path, os.ModePerm); err != nil {
			return "", err
		}
	}

	return path, err
}

func GetProfileInstalledPath() (string, error) {
	hgctlPath, err := GetHgctlPath()
	if err != nil {
		return "", err
	}

	profilesPath := filepath.Join(hgctlPath, ProfileInstalledPath)
	if _, err := os.Stat(profilesPath); os.IsNotExist(err) {
		if err = os.MkdirAll(profilesPath, os.ModePerm); err != nil {
			return "", err
		}
	}

	return profilesPath, nil
}

func GetInstalledYamlPath() (string, bool) {
	profileInstalledPath, err := GetProfileInstalledPath()
	if err != nil {
		return "", false
	}
	installedYamlFile := filepath.Join(profileInstalledPath, InstalledYamlFileName)
	if _, err := os.Stat(installedYamlFile); os.IsNotExist(err) {
		return installedYamlFile, false
	}
	return installedYamlFile, true
}
