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
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/alibaba/higress/hgctl/pkg/helm"
)

type RunSudoState string

const (
	NoSudo              RunSudoState = "NoSudo"
	SudoWithoutPassword RunSudoState = "SudoWithoutPassword"
	SudoWithPassword    RunSudoState = "SudoWithPassword"
)

type Agent struct {
	profile            *helm.Profile
	writer             io.Writer
	shutdownBinaryName string
	resetBinaryName    string
	startupBinaryName  string
	installBinaryName  string
	installPath        string
	configuredPath     string
	higressPath        string
	versionPath        string
	quiet              bool
	runSudoState       RunSudoState
}

func NewAgent(profile *helm.Profile, writer io.Writer, quiet bool) *Agent {
	installPath := profile.InstallPackagePath
	return &Agent{
		profile:            profile,
		writer:             writer,
		installPath:        installPath,
		higressPath:        filepath.Join(installPath, "higress"),
		installBinaryName:  filepath.Join(installPath, "get-higress.sh"),
		shutdownBinaryName: filepath.Join(installPath, "higress", "bin", "shutdown.sh"),
		resetBinaryName:    filepath.Join(installPath, "higress", "bin", "reset.sh"),
		startupBinaryName:  filepath.Join(installPath, "higress", "bin", "startup.sh"),
		configuredPath:     filepath.Join(installPath, "higress", "compose", ".configured"),
		versionPath:        filepath.Join(installPath, "higress", "VERSION"),
		quiet:              quiet,
		runSudoState:       NoSudo,
	}
}

func (a *Agent) profileArgs() []string {
	args := []string{
		fmt.Sprintf("--nacos-ns=%s", a.profile.Storage.Ns),
		fmt.Sprintf("--config-url=%s", a.profile.Storage.Url),
		fmt.Sprintf("--nacos-ns=%s", a.profile.Storage.Ns),
		fmt.Sprintf("--nacos-password=%s", a.profile.Storage.Password),
		fmt.Sprintf("--nacos-username=%s", a.profile.Storage.Username),
		fmt.Sprintf("--data-enc-key=%s", a.profile.Storage.DataEncKey),
		fmt.Sprintf("--console-port=%d", a.profile.Console.Port),
		fmt.Sprintf("--gateway-http-port=%d", a.profile.Gateway.HttpPort),
		fmt.Sprintf("--gateway-https-port=%d", a.profile.Gateway.HttpsPort),
		fmt.Sprintf("--gateway-metrics-port=%d", a.profile.Gateway.MetricsPort),
	}
	return args
}

func (a *Agent) run(binaryName string, args []string, autoSudo bool) error {
	var cmd *exec.Cmd
	if !autoSudo || a.runSudoState == NoSudo {
		if !a.quiet {
			fmt.Fprintf(a.writer, "\n📦 Running command: %s  %s\n\n", binaryName, strings.Join(args, "  "))
		}
		cmd = exec.Command(binaryName, args...)
	} else {
		newArgs := make([]string, 0)
		newArgs = append(newArgs, binaryName)
		newArgs = append(newArgs, args...)
		if !a.quiet {
			fmt.Fprintf(a.writer, "\n📦 Running command: %s  %s\n\n", "sudo", strings.Join(newArgs, "  "))
		}
		cmd = exec.Command("sudo", newArgs...)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = a.installPath
	if err := cmd.Start(); err != nil {
		return err
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		return err
	}

	return nil
}

func (a *Agent) checkSudoPermission() error {
	if !a.quiet {
		fmt.Fprintf(a.writer, "\n⌛️ Checking docker command sudo permission... ")
	}
	// check docker ps command
	cmd := exec.Command("docker", "ps")
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	cmd.Dir = a.installPath

	if err := cmd.Start(); err != nil {
		return err
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		if err == nil {
			if !a.quiet {
				fmt.Fprintf(a.writer, "checked result: no need sudo permission\n")
			}
			a.runSudoState = NoSudo
			return nil
		}
	}

	// check sudo docker ps command
	cmd2 := exec.Command("sudo", "-S", "docker", "ps")
	var out2 bytes.Buffer
	var stderr2 bytes.Buffer
	cmd2.Stdout = &out2
	cmd2.Stderr = &stderr2
	cmd2.Dir = a.installPath
	stdin, _ := cmd2.StdinPipe()
	defer stdin.Close()

	if err := cmd2.Start(); err != nil {
		return err
	}

	done2 := make(chan error, 1)
	go func() {
		done2 <- cmd2.Wait()
	}()

	select {
	case <-time.After(5 * time.Second):
		cmd2.Process.Signal(os.Interrupt)
		if !a.quiet {
			fmt.Fprintf(a.writer, "checked result: timeout exceed and need sudo with password\n")
		}
		a.runSudoState = SudoWithPassword

	case err := <-done2:
		if err == nil {
			if !a.quiet {
				fmt.Fprintf(a.writer, "checked result: need sudo without password\n")
			}
			a.runSudoState = SudoWithoutPassword
		} else {
			if !a.quiet {
				fmt.Fprintf(a.writer, "checked result: need sudo with password\n")
			}
			a.runSudoState = SudoWithPassword
		}
	}

	return nil
}

func (a *Agent) Install() error {
	a.checkSudoPermission()
	if a.runSudoState == SudoWithPassword {
		if !a.promptSudo() {
			return errors.New("cancel installation")
		}
	}

	if a.hasConfigured() {
		a.Reset()
	}

	if !a.quiet {
		fmt.Fprintf(a.writer, "\n⌛️ Starting to install higress.. \n")
	}
	args := []string{"./higress"}
	args = append(args, a.profileArgs()...)
	return a.run(a.installBinaryName, args, true)

	return nil
}

func (a *Agent) Uninstall() error {
	a.checkSudoPermission()
	if a.runSudoState == SudoWithPassword {
		if !a.promptSudo() {
			return errors.New("cancel uninstall")
		}
	}

	if !a.quiet {
		fmt.Fprintf(a.writer, "\n⌛️ Starting to uninstall higress... \n")
	}

	if err := a.Reset(); err != nil {
		return err
	}

	return nil
}

func (a *Agent) Upgrade() error {
	a.checkSudoPermission()
	if a.runSudoState == SudoWithPassword {
		if !a.promptSudo() {
			return errors.New("cancel upgrade")
		}
	}

	currentVersion := ""
	newVersion := ""
	if !a.quiet {
		fmt.Fprintf(a.writer, "\n⌛️ Checking current higress version... ")
		currentVersion, _ = a.Version()
		fmt.Fprintf(a.writer, "%s\n", currentVersion)
	}

	if !a.quiet {
		fmt.Fprintf(a.writer, "\n⌛️ Starting to upgrade higress... \n")
	}

	if err := a.run(a.installBinaryName, []string{"-u"}, true); err != nil {
		return err
	}

	if !a.quiet {
		fmt.Fprintf(a.writer, "\n⌛️ Checking new higress version... ")
		newVersion, _ = a.Version()
		fmt.Fprintf(a.writer, "%s\n", newVersion)
	}

	if currentVersion == newVersion {
		return nil
	}

	if !a.promptRestart() {
		return nil
	}

	if err := a.Shutdown(); err != nil {
		return err
	}

	if err := a.Startup(); err != nil {
		return err
	}
	return nil
}

func (a *Agent) Version() (string, error) {
	version := ""
	content, err := os.ReadFile(a.versionPath)
	if err != nil {
		return version, nil
	}
	return string(content), nil
}

func (a *Agent) strictVersion() (string, error) {
	content, err := os.ReadFile(a.versionPath)
	if err != nil {
		return "", fmt.Errorf("read installed Higress version: %w", err)
	}

	version := strings.TrimSpace(string(content))
	if version == "" {
		return "", errors.New("installed Higress version is empty")
	}

	normalized := strings.TrimPrefix(version, "v")
	if _, err := semver.StrictNewVersion(normalized); err != nil {
		return "", fmt.Errorf("invalid installed Higress version %q: %w", version, err)
	}
	return version, nil
}

func (a *Agent) promptSudo() bool {
	answer := ""
	for {
		fmt.Fprintf(a.writer, "\nThis need sudo permission and input root password to continue installation, Proceed? (y/N)")
		fmt.Scanln(&answer)
		if strings.TrimSpace(answer) == "y" {
			fmt.Fprintf(a.writer, "\n")
			return true
		}
		if strings.TrimSpace(answer) == "N" {
			fmt.Fprintf(a.writer, "Cancelled.\n")
			return false
		}
	}
}

func (a *Agent) promptRestart() bool {
	answer := ""
	for {
		fmt.Fprintf(a.writer, "\nThis need to restart higress, Proceed? (y/N)")
		fmt.Scanln(&answer)
		if strings.TrimSpace(answer) == "y" {
			fmt.Fprintf(a.writer, "\n")
			return true
		}
		if strings.TrimSpace(answer) == "N" {
			fmt.Fprintf(a.writer, "Cancelled.\n")
			return false
		}
	}
}

func (a *Agent) Startup() error {
	if !a.quiet {
		fmt.Fprintf(a.writer, "\n⌛️ Starting higress... \n")
	}
	return a.run(a.startupBinaryName, []string{}, true)
}

func (a *Agent) Shutdown() error {
	if !a.quiet {
		fmt.Fprintf(a.writer, "\n⌛️ Shutdowning higress... \n")
	}
	return a.run(a.shutdownBinaryName, []string{}, true)
}

func (a *Agent) Reset() error {
	if !a.quiet {
		fmt.Fprintf(a.writer, "\n⌛️ Resetting higress....\n")
	}
	return a.run(a.resetBinaryName, []string{}, true)
}

func (a *Agent) hasConfigured() bool {
	if _, err := os.Stat(a.configuredPath); os.IsNotExist(err) {
		return false
	}
	return true
}
