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
	"io/fs"
	"os"
	"path/filepath"
	"text/template"

	"github.com/AlecAivazis/survey/v2"
	"github.com/alibaba/higress/hgctl/pkg/agent/prompt"
	"github.com/alibaba/higress/hgctl/pkg/manifests"
	"github.com/alibaba/higress/hgctl/pkg/util"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	ASRuntimeMainPyFile = "as_runtime_main.py"
	AgentRunMainPyFile  = "agentrun_main.py"
	ToolKitPyFile       = "toolkit.py"
	AgentClassFile      = "agent.py"
	CorePromptFile      = "claude.md" // TODO: support qoder AGENTS.md
	SConfigYAML         = "s.yaml"

	ARTemplate         = "agentrun.tmpl"
	ASTemplate         = "agentscope.tmpl"
	AgentClassTemplate = "agent.tmpl"
	ToolKitTemplate    = "toolkit.tmpl"
	SConfigTemplate    = "agentrun_s.tmpl"
)

var ASAvailiableTools = []string{
	"execute_python_code",
	"execute_shell_command",
	"view_text_file",
	"write_text_file",
	"insert_text_file",
	"dashscope_text_to_image",
	"dashscope_text_to_audio",
	"dashscope_image_to_text",
	"openai_text_to_image",
	"openai_text_to_audio",
	"openai_edit_image",
	"openai_create_image_variation",
	"openai_image_to_text",
	"openai_audio_to_text",
}

const (
	MinPythonVersion = "3.12"

	DefaultServerLessAccessKey = "hgctl-credential"
)

// Callback type for post-agent-creation actions
type PostAgentAction func(config *AgentConfig) error

type MCPServerConfig struct {
	Name      string            // MCP Client Name
	URL       string            // MCP Server URL
	Transport string            // transport `streamable_http` or `see` or `stdio`
	Headers   map[string]string // HTTP Headers
}

type ServerlessConfig struct {
	AccessKey    string
	ResourceName string
	Region       string
	AgentName    string
	AgentDesc    string
	Port         uint

	DiskSize uint
	Timeout  uint

	GlobalConfig HgctlAgentConfig
}

type AgentConfig struct {
	AppName         string   //  "app"
	AppDescription  string   //  "A helpful assistant and useful agent"
	AgentName       string   //  "Friday"
	AvailableTools  []string //   available tools (built-in agentscope)
	SysPromptPath   string   //  "You are a helpful assistant"
	ChatModel       string   //  "qwen-max"
	Provider        string   //  "Aliyun"
	APIKeyEnvVar    string   //  DASHCOPE_API_KEY
	DeploymentPort  int      //  8090
	HostBinding     string   //  0.0.0.0
	EnableStreaming bool     //  true
	EnableThinking  bool     //  true
	MCPServers      []MCPServerConfig

	Type          DeployType
	ServerlessCfg ServerlessConfig
}

func createAgentCmd() *cobra.Command {
	agentRun := false
	deployDirect := false

	var createAgentCmd = &cobra.Command{
		Use:   "new",
		Short: "Create a new agent or import one from core",
		Args:  cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			config := &AgentConfig{
				Type: Local,
			}
			if agentRun {
				config.Type = AgentRun
				config.ServerlessCfg = ServerlessConfig{
					AccessKey: DefaultServerLessAccessKey,
					Port:      9000,
					DiskSize:  512,
					Timeout:   600,

					GlobalConfig: GlobalConfig,
				}
			}

			if err := getAgentConfig(config); err != nil {
				fmt.Printf("Error get Agent config: %v\n", err)
				os.Exit(1)
			}

			if err := createAgentTemplate(config); err != nil {
				fmt.Printf("Error creating agent: %v\n", err)
				os.Exit(1)
			}

			if err := afterCreatedAgent(config); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
		},
	}

	createAgentCmd.PersistentFlags().BoolVar(&agentRun, "agent-run", false, "Use agentRun to deploy to Alibaba cloud, default is false")
	createAgentCmd.PersistentFlags().BoolVar(&deployDirect, "deploy", false, "After agent creation, deploy it directly")
	return createAgentCmd

}

func afterCreatedAgent(config *AgentConfig) error {
	options := []string{
		"Deploy it directly",
		fmt.Sprintf("Improve and test it using agentic core (%s)", viper.GetString(HGCTL_AGENT_CORE)),
		"Do nothing and quit",
	}
	callbacks := map[string]PostAgentAction{
		options[0]: func(cfg *AgentConfig) error {
			handler := &DeployHandler{Name: cfg.AgentName}
			return handler.Deploy()
		},
		options[1]: func(cfg *AgentConfig) error {
			return runAgenticCoreImprovement(cfg)
		},
	}

	if err := promptAfterCreatedAgent(options, config, callbacks); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to handle post-creation action: %v\n", err)
		return nil
	}
	return nil
}

func runAgenticCoreImprovement(cfg *AgentConfig) error {
	core, err := getCore()
	if err != nil {
		return fmt.Errorf("failed to invoke agent core: %s", err)
	}

	if err := core.ImproveNewAgent(cfg); err != nil {
		return fmt.Errorf("failed to use core to improve new agent: %s", err)
	}
	return nil
}

func promptAfterCreatedAgent(options []string, config *AgentConfig, callbacks map[string]PostAgentAction) error {

	promptChoice := &survey.Select{
		Message: "What's next?:",
		Options: options,
		Help:    "Choose an action to perform after agent creation.",
	}

	var response string
	if err := survey.AskOne(promptChoice, &response); err != nil {
		return fmt.Errorf("failed to read user choice: %w", err)
	}

	if callback, ok := callbacks[response]; ok {
		return callback(config)
	}

	if response == options[2] {
		os.Exit(1)
	}

	return fmt.Errorf("unknown action selected: %q", response)
}

func createAgentTemplate(config *AgentConfig) error {
	agentsDir := util.GetHomeHgctlDir() + "/agents"
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		return fmt.Errorf("failed to create agents directory: %v", err)
	}

	agentDir := filepath.Join(agentsDir, config.AgentName)
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		return fmt.Errorf("failed to create agent directory: %v", err)
	}

	switch config.Type {
	case Local:
		// parse agentscope file
		asMain := filepath.Join(agentDir, ASRuntimeMainPyFile)
		asTemplateStr, err := get_template(ASTemplate)
		if err != nil {
			return fmt.Errorf("failed to read agentscope template: %v", err)
		}
		if err := renderTemplateFile(asTemplateStr, asMain, config); err != nil {
			return fmt.Errorf("failed to render agentscope runtime's file: %s", err)
		}
	case AgentRun:
		// Details see: https://github.com/Serverless-Devs/agentrun-sdk-python

		// parse agentrun file
		arMain := filepath.Join(agentDir, AgentRunMainPyFile)
		arTemplateStr, err := get_template(ARTemplate)
		if err != nil {
			return fmt.Errorf("failed to read agentrun template: %v", err)
		}
		if err := renderTemplateFile(arTemplateStr, arMain, config); err != nil {
			return fmt.Errorf("failed to render agentscope runtime's file: %s", err)
		}

		// parse s.yaml
		s := filepath.Join(agentDir, SConfigYAML)
		STmplStr, err := get_template(SConfigTemplate)
		if err != nil {
			return fmt.Errorf("failed to read agentrun's serverless config file template: %v", err)
		}
		if err := renderTemplateFile(STmplStr, s, config.ServerlessCfg); err != nil {
			return fmt.Errorf("failed to render agentscope runtime's file: %s", err)
		}

		// write requirements
		fileContent := "agentrun-sdk[agentscope,server]>=0.0.3"
		targetFilePath := filepath.Join(agentDir, "requirements.txt")
		if err := util.WriteFileString(targetFilePath, fileContent, os.ModePerm); err != nil {
			return fmt.Errorf("failed to write requirements.txt file to target agent directory: %s", err)
		}
	}

	// parse toolkitPath
	toolkitPath := filepath.Join(agentDir, ToolKitPyFile)
	toolkitTmpl, err := get_template(ToolKitTemplate)
	if err != nil {
		return fmt.Errorf("failed to read toolkit template: %v", err)
	}
	if err := renderTemplateFile(toolkitTmpl, toolkitPath, config); err != nil {
		return fmt.Errorf("failed to render toolkit file: %s", err)
	}

	// write agent.py
	agentPath := filepath.Join(agentDir, AgentClassFile)
	agentTmpl, err := get_template(AgentClassTemplate)
	if err != nil {
		return fmt.Errorf("failed to read agent class template: %v", err)
	}
	if err := renderTemplateFile(agentTmpl, agentPath, config); err != nil {
		return fmt.Errorf("failed to render agent class file: %s", err)
	}

	// write core_prompt.md
	if core, err := getCore(); err == nil {
		corePromptPath := filepath.Join(agentDir, core.GetPromptFileName())
		if err := util.WriteFileString(corePromptPath, prompt.AgentDevelopmentGuide, os.ModePerm); err != nil {
			return fmt.Errorf("failed to write %s file to target agent directory: %s", core.GetPromptFileName(), err)
		}
		return nil
	} else {
		return fmt.Errorf("failed to add instruction file in agent dir: %s", err)
	}
}

func renderTemplateFile(templateStr string, targetPath string, data interface{}) error {
	// sync with python
	funcMap := template.FuncMap{
		"boolToPython": func(b bool) string {
			if b {
				return "True"
			}
			return "False"
		},
	}

	tmpl, err := template.New("agent").Funcs(funcMap).Parse(templateStr)
	if err != nil {
		return fmt.Errorf("failed to parse template: %v", err)
	}
	file, err := os.Create(targetPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %v", err)
	}
	defer file.Close()

	if err := tmpl.Execute(file, data); err != nil {
		return fmt.Errorf("failed to render template: %v", err)
	}

	return nil
}

func get_template(templatePath string) (string, error) {
	f := manifests.BuiltinOrDir("")
	templatePath = "agent/template/" + templatePath
	data, err := fs.ReadFile(f, templatePath)
	if err != nil {
		return "", fmt.Errorf("failed to read template: %w", err)
	}

	return string(data), nil
}
