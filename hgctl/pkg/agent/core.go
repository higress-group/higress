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
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/alibaba/higress/hgctl/pkg/manifests"
	"github.com/alibaba/higress/hgctl/pkg/util"
	"github.com/fatih/color"
	"github.com/manifoldco/promptui"
	"github.com/spf13/viper"
)

type AgenticCore struct {
	binaryName string
}

func NewAgenticCore() *AgenticCore {
	core := &AgenticCore{
		binaryName: viper.GetString(HGCTL_AGENT_CORE),
	}
	core.Setup()
	return core
}

func (c *AgenticCore) GetPromptFileName() string {
	switch c.binaryName {
	case string(CORE_CLAUDE):
		return "CLAUDE.md"
	case string(CORE_QODERCLI):
		return "AGENTS.md"
	}
	return ""
}

func (c *AgenticCore) GetCoreDirName() string {
	switch c.binaryName {
	case string(CORE_CLAUDE):
		return ".claude"
	case string(CORE_QODERCLI):
		return ".qoder"
	}
	return ""
}

// This will use core to test and improve created agent
func (c *AgenticCore) ImproveNewAgent(config *AgentConfig) error {
	agentDir, err := util.GetSpecificAgentDir(config.AgentName)
	if err != nil {
		return fmt.Errorf("failed to get agent directory: %s", agentDir)
	}
	return c.runInTargetDir(agentDir)
}

func (c *AgenticCore) runInTargetDir(dir string, args ...string) error {
	cmd := exec.Command(c.binaryName, args...)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()

}

func (c *AgenticCore) runWithResult(args ...string) (string, error) {
	cmd := exec.Command(c.binaryName, args...)

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("agent execution failed with exit code %d: %s\nStderr: %s",
				exitErr.ExitCode(), err.Error(), exitErr.Stderr)
		}
		return "", fmt.Errorf("failed to run agent: %w", err)
	}

	return string(output), nil
}

func (c *AgenticCore) run(args ...string) error {
	cmd := exec.Command(c.binaryName, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// setup additional prequisite environment and plugins manifest to user's profile
// e.g. ../manifest/agent
func (c *AgenticCore) Setup() {
	// Check if this is the first time, otherwise directly return (TODO: this is a simple check)
	homeDir, _ := os.UserHomeDir()
	targetCtlDir := filepath.Join(homeDir, ".hgctl")
	if _, err := os.Stat(targetCtlDir); err == nil {
		return
	}

	targetCoreDir := filepath.Join(homeDir, c.GetCoreDirName())

	// setup subagent plugins file
	embedFS := manifests.BuiltinOrDir("")
	if err := manifests.ExtractEmbedFiles(embedFS, "agent", targetCtlDir); err != nil {
		fmt.Println(err)
		fmt.Println("failed to init plugins for agent core")
		os.Exit(1)
	}

	// Setup predefined files like: command.md
	if err := manifests.ExtractEmbedFiles(embedFS, "agent", targetCoreDir); err != nil {
		fmt.Println(err)
		fmt.Println("failed to init commands for agent core")
		os.Exit(1)
	}

	// Add Predefined MCP Server
	if err := c.addPredefinedMCP(); err != nil {
		fmt.Printf("Warning: failed to add needed mcp server: %s\n", err)
	}

	if err := c.addHigressAPIMCP(); err != nil {
		fmt.Println("failed to init higress-api mcp server (you may need to add it manually):", err)
		fmt.Println("Details information on Higress-api MCP server refers to https://github.com/alibaba/higress/blob/main/plugins/golang-filter/mcp-server/servers/higress/higress-api/README_en.md")
		return
	}
	// fmt.Println("Higress-api MCP server added successfully")
}

func (c *AgenticCore) addPredefinedMCP() error {
	// deepwikiArg := MCPAddArg{
	// 	name:      "deepwiki",
	// 	url:       "https://mcp.deepwiki.com/mcp",
	// 	typ:       "",
	// 	transport: STREAMABLE,
	// 	scope:     "user",
	// }
	// if err := c.AddMCPServer(deepwikiArg); err != nil {
	// 	return fmt.Errorf("deepwiki")
	// }

	return nil
}

func (c *AgenticCore) addHigressAPIMCP() error {
	arg := &HigressConsoleAuthArg{
		hgURL:      viper.GetString(HIGRESS_GATEWAY_URL),
		hgUser:     viper.GetString(HIGRESS_CONSOLE_USER),
		hgPassword: viper.GetString(HIGRESS_CONSOLE_PASSWORD),
	}
	fmt.Println("Initializing...Add prequisite MCP server (Higress-api MCP server) automatically")

	if arg.hgURL == "" {
		gatewayPrompt := promptui.Prompt{
			Label:   "Enter higress gateway URL",
			Default: "http://127.0.0.1:80",
		}
		gateway, err := gatewayPrompt.Run()
		if err != nil {
			fmt.Println("failed to run gateway prompt: ", err)
			return err
		}
		arg.hgURL = gateway

	}

	if arg.hgURL == "" || arg.hgPassword == "" {
		if err := tryToGetLocalCredential(arg); err != nil || arg.hgUser == "" || arg.hgPassword == "" {
			// fallback: interact with user to provide password & username
			color.Red("failed to get higress-console credential automatically (Requires higress installed by hgctl). Let's do it manually")
			userPrompt := promptui.Prompt{
				Label:   "Enter higress console username",
				Default: "admin",
			}
			username, err := userPrompt.Run()
			if err != nil {
				return fmt.Errorf("aborted: %v", err)
			}
			pwdPrompt := promptui.Prompt{
				Label:   "Enter higress console password",
				Default: "admin",
			}
			pwd, err := pwdPrompt.Run()
			if err != nil {
				return fmt.Errorf("aborted: %v", err)
			}
			arg.hgUser = username
			arg.hgPassword = pwd
		}
	}

	if arg.hgUser == "" || arg.hgPassword == "" {
		return fmt.Errorf("Empty higress console username and password, aborting")
	}

	rawByte := fmt.Appendf(nil, "%s:%s", arg.hgUser, arg.hgPassword)

	resStr := base64.StdEncoding.EncodeToString(rawByte)

	authHeader := fmt.Sprintf("Authorization: Basic %s", resStr)

	return c.AddMCPServer(MCPAddArg{
		name:      "higress-api",
		url:       fmt.Sprintf("%s/higress-api", arg.hgURL),
		transport: HTTP,
		typ:       HTTP,
		scope:     "user",
		header: []string{
			authHeader,
		},
	})
}

// ------- Initialization  -------
func (c *AgenticCore) Start() error {
	return c.run()
}

// ------- MCP  -------
func (c *AgenticCore) AddMCPServer(arg MCPAddArg) error {
	// adapt the field
	if arg.transport == STREAMABLE {
		arg.transport = HTTP
	}
	args := []string{
		"mcp", "add", "--transport", arg.transport, arg.name, arg.url,
	}
	if arg.scope != "" {
		scopeArg := []string{"--scope", arg.scope}
		args = append(args, scopeArg...)
	}
	if len(arg.env) != 0 {
		for _, e := range arg.env {
			envArg := []string{"-e", e}
			args = append(args, envArg...)
		}
	}
	if len(arg.header) != 0 {
		for _, h := range arg.header {
			headerArg := []string{"-H", h}
			args = append(args, headerArg...)
		}
	}
	err := c.run(args...)

	// Allow adding a duplicate mcp server name (core will return error)
	if err == nil || strings.Contains(err.Error(), "already exists") {
		return nil
	}
	return err
}
