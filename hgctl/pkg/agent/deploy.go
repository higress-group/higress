// Copyright (c) 2025 Alibaba Group Holding Ltd.
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

package agent

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/alibaba/higress/hgctl/pkg/util"
	"github.com/spf13/cobra"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

type DeployType string

const (
	AgentRun DeployType = "agent-run"
	Local    DeployType = "local"
)

var (
	AddAccessKeyCmd   = fmt.Sprintf("s config add -a %s", DefaultServerLessAccessKey)
	CheckAccessKeyCmd = fmt.Sprintf("s config get -a %s", DefaultServerLessAccessKey)
	DeployAgentRunCmd = fmt.Sprintf("s deploy -a %s", DefaultServerLessAccessKey)
)

const (
	InstallServerlessCmd = "npm install @serverless-devs/s -g"
	BuildAgentCmd        = "s build"
	ServerlessCliDocs    = "https://serverless-devs.com/docs/user-guide/install"
)

type DeployHandler struct {
	Name     string
	AgentDir string
	Type     DeployType
}

func deployAgentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy [name]",
		Short: "Deploy the specified agent locally or to the cloud",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			handler := &DeployHandler{
				Name: args[0],
			}
			cmdutil.CheckErr(handler.Deploy())
		},
	}

	var cloud = false
	cmd.PersistentFlags().BoolVar(&cloud, "agentrun", false, "deploy agent using agentrun")

	return cmd
}

func (h *DeployHandler) validate() error {
	if err := h.checkRequiredEnvironment(); err != nil {
		return fmt.Errorf("failed to get required environment: %s", err)
	}
	return nil
}

func (h *DeployHandler) RunCmd(showOutput bool, cmd string, targetDir string) (string, error) {
	runCmd := exec.Command("bash", "-c", cmd)

	if targetDir != "" {
		runCmd.Dir = targetDir
	}

	if showOutput {
		runCmd.Stderr = os.Stderr
		runCmd.Stdout = os.Stdout
		if err := runCmd.Run(); err != nil {
			return "", err
		}

		return "", nil
	}
	output, err := runCmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func (h *DeployHandler) RunPythonCmd(showOutput bool, args ...string) error {
	cmd := exec.Command("python3", args...)

	if showOutput {
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stdout
	}

	if err := cmd.Run(); err != nil {
		return err
	}

	return nil
}

func (h *DeployHandler) checkAgentRunEnvironment() error {
	if _, err := h.RunCmd(false, "s --version", ""); err != nil {
		fmt.Println("Serverless dev cli not installed, install it automatically..")
		if _, err := h.RunCmd(true, InstallServerlessCmd, ""); err != nil {
			return fmt.Errorf("failed to install serverless dev cli automatically, details refers to %s", ServerlessCliDocs)
		}
	}

	if _, err := h.RunCmd(false, "docker --version", ""); err != nil {
		return fmt.Errorf("docker is required to deploy agent to agentRun: %s", err)
	}

	return nil
}

func (h *DeployHandler) checkLocalEnvironment() error {
	pyVenv, err := util.GetPythonVersion()
	if err != nil {
		fmt.Printf("Python environment not found, you need Python environment to run your agent\n")
		return err
	}

	if util.CompareVersions(pyVenv, MinPythonVersion) == -1 {
		fmt.Printf("Current Python: %s need Python %s+", MinPythonVersion, pyVenv)
		return fmt.Errorf("unsupport python version")
	}

	missingDeps := []string{}
	if err := h.RunPythonCmd(false, "-c", "import agentscope; print(agentscope.__version__)"); err != nil {
		missingDeps = append(missingDeps, "agentscope")
	}

	if err := h.RunPythonCmd(false, "-c", "import agentscope_runtime; print(agentscope_runtime.__version__)"); err != nil {
		missingDeps = append(missingDeps, "agentscope-runtime==1.0.0")
	}

	if len(missingDeps) != 0 {
		venvDir := filepath.Join(util.GetHomeHgctlDir(), ".venv")
		if _, err := os.Stat(venvDir); err == nil {
			// check again
			missingDeps := []string{}
			if err := h.RunPythonCmd(false, "-c", "import agentscope; print(agentscope.__version__)"); err != nil {
				fmt.Println("agentscope not installed, installing...")
				missingDeps = append(missingDeps, "agentscope")
			}
			if err := h.RunPythonCmd(false, "-c", "import agentscope_runtime; print(agentscope_runtime.__version__)"); err != nil {
				fmt.Println("agentscope-runtime not installed, installing...")
				missingDeps = append(missingDeps, "agentscope-runtime==1.0.0")
			}
			// This means ~/.hgctl/.venv/ has already installed the deps before
			if len(missingDeps) == 0 {
				if err := h.activateLocalPythonVenv(); err != nil {
					return err
				}
			}
		}

		if err := h.installLocalRequiredDeps(missingDeps); err != nil {
			return fmt.Errorf("failed to install missing deps: %s", err)
		}

	}

	return nil
}

func (h *DeployHandler) createLocalPyVenv() error {
	venvDir := filepath.Join(util.GetHomeHgctlDir(), ".venv")
	cmd := exec.Command("python3", "-m", "venv", venvDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println("failed to create python virtual environment", string(output))
		return err
	}
	return nil
}

func (h *DeployHandler) installLocalRequiredDeps(missingDeps []string) error {
	if err := h.RunPythonCmd(true, "-m", "pip", "--version"); err != nil {
		fmt.Printf("Pip not installed, you need install pip to deploy your agent\n")
		return err
	}

	fmt.Println("This may takes a few minutes, you can install missing deps by yourself: ")
	for _, deps := range missingDeps {
		fmt.Println("- ", deps)
	}

	if err := h.createLocalPyVenv(); err != nil {
		return fmt.Errorf("failed to create local venv (~/.hgctl/.venv): %s", err)
	}

	if err := h.activateLocalPythonVenv(); err != nil {
		return fmt.Errorf("failed to activateLocalPythonVenv: %s", err)
	}

	for _, deps := range missingDeps {
		if err := h.RunPythonCmd(true, "-m", "pip", "install", deps); err != nil {
			fmt.Printf("failed to install missing deps: %s\n", deps)
			return err
		}
	}

	venvDir := filepath.Join(util.GetHomeHgctlDir(), ".venv")
	fmt.Println("Missing deps installed successfully, target python venv path: ", venvDir)

	return nil
}

func (h *DeployHandler) activateLocalPythonVenv() error {
	venvDir := filepath.Join(util.GetHomeHgctlDir(), ".venv")
	path := os.Getenv("PATH")
	newPath := venvDir + "/bin:" + path
	err := os.Setenv("PATH", newPath)
	if err != nil {
		fmt.Println("Failed to set PATH:", err)
		return err
	}
	err = os.Setenv("VIRTUAL_ENV", venvDir)
	if err != nil {
		fmt.Println("Failed to set VIRTUAL_ENV:", err)
		return err
	}

	return nil
}

func (h *DeployHandler) checkRequiredEnvironment() error {
	if h.Type == AgentRun {
		return h.checkAgentRunEnvironment()
	}

	if h.Type == Local {
		return h.checkLocalEnvironment()
	}
	return nil
}

func (h *DeployHandler) GetRequiredDeps() ([]string, error) {
	switch h.Type {
	case AgentRun:
		return []string{
			"agentrun-sdk[agentscope,server] >= 0.0.3",
		}, nil
	case Local:
		return []string{
			"agentscope", "agentscope-runtime==1.0.0",
		}, nil
	default:
		return nil, fmt.Errorf("unsupported deploy target type: %s", h.Type)
	}
}

// Quick and simple to get type by examine the existence of `requirements.txt` file
func (h *DeployHandler) getAgentType() error {
	path, err := util.GetSpecificAgentDir(h.Name)
	if err != nil {
		fmt.Printf("invalid agent: %s", err)
		return err
	}
	h.AgentDir = path

	filePath := filepath.Join(h.AgentDir, "requirements.txt")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		h.Type = Local
		return nil
	}
	h.Type = AgentRun
	return nil
}

func (h *DeployHandler) Deploy() error {
	if err := h.getAgentType(); err != nil {
		return err
	}

	if err := h.validate(); err != nil {
		return err
	}

	switch h.Type {
	case AgentRun:
		if err := h.HandleAgentRun(); err != nil {
			return err
		}

	case Local:
		if err := h.HandleLocal(); err != nil {
			return err
		}

	default:
		return fmt.Errorf("unsupported deploy target type: %s", h.Type)
	}

	if h.Type == AgentRun {
		fmt.Printf("\n🌟 Agent deploy to agentRun successfully! Refers to https://functionai.console.aliyun.com/cn-hangzhou/agent/runtime to get it")
		fmt.Printf("You can publish it to Higress and Himarket by using `hgctl agent add %s <endpoints-url> -t model --as-product `\n", h.Name)
	}
	return nil
}

// details see: https://github.com/Serverless-Devs/agentrun-sdk-python
func (h *DeployHandler) HandleAgentRun() error {
	if err := h.CheckServerlessAccessKey(); err != nil {
		return fmt.Errorf("failed to set access key automatically: %s", err)
	}

	if _, err := h.RunCmd(true, BuildAgentCmd, h.AgentDir); err != nil {
		return fmt.Errorf("failed to build agent: %s", err)
	}

	if _, err := h.RunCmd(true, DeployAgentRunCmd, h.AgentDir); err != nil {
		return fmt.Errorf("failed to deploy agent: %s", err)
	}

	return nil
}

// Set Serverless's Access Key in s.yaml, details see: https://github.com/Serverless-Devs/agentrun-sdk-python
// Example:
// $ s config get -a default

// You have not yet been found to have configured key information.
// You can use [s config add] for key configuration, or use [s config add -h] to view configuration help.
// If you already used [s config add], please check the permission of file [{HOMEPATH}/.s/access.yaml].
// If you have questions, please tell us: https://github.com/Serverless-Devs/Serverless-Devs/issues
//
// s version: @serverless-devs/s: 3.1.10
func (h *DeployHandler) CheckServerlessAccessKey() error {
	notFoundMessage := "You have not yet been found to have configured key information"
	output, err := h.RunCmd(false, CheckAccessKeyCmd, "")
	if err != nil {
		return fmt.Errorf("failed to run %s command to check access key: %s", CheckAccessKeyCmd, err)
	}
	if strings.Contains(output, notFoundMessage) {
		fmt.Fprintf(os.Stderr, `
🔑 **ACTION REQUIRED**: Please configure your Alibaba Cloud credentials first.
Copy and run the command below to set up your Access Key:
> %s

`, AddAccessKeyCmd)
		return fmt.Errorf("access key not found")
	}

	return nil
}

func (h *DeployHandler) HandleLocal() error {
	if _, err := os.Stat(h.AgentDir); os.IsNotExist(err) {
		return fmt.Errorf("agent source file not found: %s", h.AgentDir)
	}

	if err := h.startAgentProcess(); err != nil {
		return err
	}

	return nil
}

func (h *DeployHandler) startAgentProcess() error {
	switch runtime.GOOS {
	case "windows":
		return h.runWindowsAgent()
	case "darwin", "linux":
		return h.runUnixAgent()
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

func (h *DeployHandler) runUnixAgent() error {
	agentFile := filepath.Join(h.AgentDir, ASRuntimeMainPyFile)
	if err := h.RunPythonCmd(true, agentFile); err != nil {
		fmt.Println("failed to start agent, exiting...")
		return err
	}
	return nil
}

func (h *DeployHandler) runWindowsAgent() error {
	agentFile := filepath.Join(h.AgentDir, ASRuntimeMainPyFile)
	if err := h.RunPythonCmd(true, agentFile); err != nil {
		fmt.Println("failed to start agent, exiting...")
		return err
	}
	return nil
}
