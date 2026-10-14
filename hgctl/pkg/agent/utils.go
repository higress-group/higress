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
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/alibaba/higress/hgctl/pkg/agent/services"
	"github.com/alibaba/higress/hgctl/pkg/helm"
	"github.com/alibaba/higress/hgctl/pkg/installer"
	"github.com/alibaba/higress/hgctl/pkg/kubernetes"
	"github.com/alibaba/higress/hgctl/pkg/util"
	"github.com/alibaba/higress/v2/pkg/cmd/options"
	"github.com/braydonk/yaml"
	"github.com/fatih/color"
	"github.com/higress-group/openapi-to-mcpserver/pkg/converter"
	"github.com/higress-group/openapi-to-mcpserver/pkg/models"
	"github.com/higress-group/openapi-to-mcpserver/pkg/parser"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	SecretConsoleUser = "adminUsername"
	SecretConsolePwd  = "adminPassword"
)

var (
	purple = color.New(color.FgMagenta, color.Bold)
	cyan   = color.New(color.FgCyan)
	yellow = color.New(color.FgYellow)
	green  = color.New(color.FgGreen)
)

// ------ cmd related  ------
func addHigressConsoleAuthFlag(cmd *cobra.Command, arg *HigressConsoleAuthArg) {
	cmd.PersistentFlags().StringVar(&arg.hgURL, HIGRESS_CONSOLE_URL, "", "The BaseURL of higress console")
	cmd.PersistentFlags().StringVar(&arg.hgUser, HIGRESS_CONSOLE_USER, "", "The username of higress console")
	cmd.PersistentFlags().StringVar(&arg.hgPassword, HIGRESS_CONSOLE_PASSWORD, "", "The password of higress console")

	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()
}

func addHimarketAdminAuthFlag(cmd *cobra.Command, arg *HimarketAdminAuthArg) {
	cmd.PersistentFlags().StringVar(&arg.hmURL, HIMARKET_ADMIN_URL, "", "The BaseURL of himarket")
	cmd.PersistentFlags().StringVar(&arg.hmUser, HIMARKET_ADMIN_USER, "", "The username of himarket")
	cmd.PersistentFlags().StringVar(&arg.hmPassword, HIMARKET_ADMIN_PASSWORD, "", "The password of himarket")

	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()
}

// ------ MCP convert utils function  ------
func parseOpenapi2MCP(arg MCPAddArg) *models.MCPConfig {
	path := arg.spec
	serverName := arg.name

	// Create a new parser
	p := parser.NewParser()

	p.SetValidation(true)

	// Parse the OpenAPI specification
	err := p.ParseFile(path)
	if err != nil {
		fmt.Printf("Error parsing OpenAPI specification: %v\n", err)
		os.Exit(1)
	}

	c := converter.NewConverter(p, models.ConvertOptions{
		ServerName:     serverName,
		ToolNamePrefix: "",
		TemplatePath:   "",
	})

	// Convert the OpenAPI specification to an MCP configuration
	config, err := c.Convert()
	if err != nil {
		fmt.Printf("Error converting OpenAPI specification: %v\n", err)
		os.Exit(1)
	}

	return config
}

func convertMCPConfigToStr(cfg *models.MCPConfig) string {
	var data []byte
	var buffer bytes.Buffer
	encoder := yaml.NewEncoder(&buffer)
	encoder.SetIndent(2)

	if err := encoder.Encode(cfg); err != nil {
		fmt.Printf("Error encoding YAML: %v\n", err)
		os.Exit(1)
	}
	data = buffer.Bytes()
	str := string(data)

	// fmt.Println("Successfully converted OpenAPI specification to MCP Server")
	// fmt.Printf("Get MCP server config string: %v", str)
	return str

	// if err != nil {
	// 	fmt.Printf("Error marshaling MCP configuration: %v\n", err)
	// 	os.Exit(1)
	// }

	// err = os.WriteFile(*outputFile, data, 0644)
	// if err != nil {
	// 	fmt.Printf("Error writing MCP configuration: %v\n", err)
	// 	os.Exit(1)
	// }

}

func GetHigressGatewayServiceIP() (string, error) {
	color.Cyan("🚀 Adding openapi MCP Server from higress to agent, checking Higress Gateway Pod status...")

	defaultKubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
	config, err := clientcmd.BuildConfigFromFlags("", defaultKubeconfig)
	if err != nil {
		color.Yellow("⚠️ Failed to load default kubeconfig: %v", err)
		return promptForServiceKubeSettingsAndRetry()
	}

	clientset, err := k8s.NewForConfig(config)
	if err != nil {
		color.Yellow("⚠️ Failed to create Kubernetes client: %v", err)
		return promptForServiceKubeSettingsAndRetry()
	}

	namespace := "higress-system"
	svc, err := clientset.CoreV1().Services(namespace).Get(context.Background(), "higress-gateway", metav1.GetOptions{})
	if err != nil || svc == nil {
		color.Yellow("⚠️ Could not find Higress Gateway Service in namespace '%s'.", namespace)
		return promptForServiceKubeSettingsAndRetry()
	}

	ip, err := extractServiceIP(clientset, namespace, svc)
	if err != nil {
		return "", err
	}

	color.Green("✅ Found Higress Gateway Service IP: %s (namespace: %s)", ip, namespace)
	return ip, nil
}

// higress-gateway should always be LoadBalancer
func extractServiceIP(clientset *k8s.Clientset, namespace string, svc *v1.Service) (string, error) {
	return svc.Spec.ClusterIP, nil

	// // fallback to Pod IP
	// if len(svc.Spec.Selector) > 0 {
	// 	selector := metav1.FormatLabelSelector(&metav1.LabelSelector{MatchLabels: svc.Spec.Selector})
	// 	pods, err := clientset.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{
	// 		LabelSelector: selector,
	// 	})
	// 	if err != nil {
	// 		return "", fmt.Errorf("failed to list pods for selector: %v", err)
	// 	}
	// 	if len(pods.Items) > 0 {
	// 		return pods.Items[0].Status.PodIP, nil
	// 	}
	// }

}

// prompt fallback for user input
func promptForServiceKubeSettingsAndRetry() (string, error) {
	color.Cyan("Let's fix it manually 👇")

	kubeconfigPrompt := promptui.Prompt{
		Label:   "Enter kubeconfig path",
		Default: filepath.Join(os.Getenv("HOME"), ".kube", "config"),
	}
	kubeconfigPath, err := kubeconfigPrompt.Run()
	if err != nil {
		return "", fmt.Errorf("aborted: %v", err)
	}

	nsPrompt := promptui.Prompt{
		Label:   "Enter Higress namespace",
		Default: "higress-system",
	}
	namespace, err := nsPrompt.Run()
	if err != nil {
		return "", err
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return "", fmt.Errorf("failed to load kubeconfig: %v", err)
	}

	clientset, err := k8s.NewForConfig(config)
	if err != nil {
		return "", fmt.Errorf("failed to create kubernetes client: %v", err)
	}

	svc, err := clientset.CoreV1().Services(namespace).Get(context.Background(), "higress-gateway", metav1.GetOptions{})
	if err != nil || svc == nil {
		color.Red("❌ Higress Gateway Service not found in namespace '%s'", namespace)
		return "", fmt.Errorf("service not found")
	}

	ip, err := extractServiceIP(clientset, namespace, svc)
	if err != nil {
		return "", err
	}

	color.Green("✅ Found Higress Gateway Service IP: %s (namespace: %s)", ip, namespace)
	return ip, nil
}

func getConsoleCredentials(profile *helm.Profile) (username, password string, err error) {
	cliClient, err := kubernetes.NewCLIClient(options.DefaultConfigFlags.ToRawKubeConfigLoader())

	if err != nil {
		return "", "", fmt.Errorf("failed to build kubernetes client: %w", err)
	}

	secret, err := cliClient.KubernetesInterface().CoreV1().Secrets(profile.Global.Namespace).Get(context.Background(), "higress-console", metav1.GetOptions{})
	if err != nil {
		return "", "", err
	}
	return string(secret.Data[SecretConsoleUser]), string(secret.Data[SecretConsolePwd]), nil
}

// This function will do following things:
// 1. read the profile from local-file
// 2. read the profile from k8s' configMap
// 3. combine the two type profiles together and return
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

func getAgentConfig(config *AgentConfig) error {
	options := []string{
		"create step by step",
		fmt.Sprintf("import existing one from current agentcore (%s)", viper.GetString(HGCTL_AGENT_CORE)),
	}

	var response string
	prompt := &survey.Select{
		Message: "How would you like to create a agent",
		Options: options,
	}

	if err := survey.AskOne(prompt, &response); err != nil {
		fmt.Println(err)
		return err
	}

	switch response {
	case options[0]:
		return createAgentStepByStep(config)
	case options[1]:
		return importAgentFromCore(config)
	}
	return fmt.Errorf("Unsupported way to create a agent")
}

func getAgentCoreSubAgents() (map[string]string, []string, error) {
	home, _ := os.UserHomeDir()
	core, err := getCore()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get core: %s", err)
	}
	coreAgentsDir := filepath.Join(home, core.GetCoreDirName(), "agents")

	files, err := os.ReadDir(coreAgentsDir)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read core agents directory (%s): %w", coreAgentsDir, err)
	}

	var agentNames []string
	agentContentMap := make(map[string]string)

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		filename := file.Name()
		if !strings.HasSuffix(filename, ".md") {
			continue // Only process markdown files
		}

		agentName := strings.TrimSuffix(filename, ".md")

		filePath := filepath.Join(coreAgentsDir, filename)
		contentBytes, err := os.ReadFile(filePath)
		if err != nil {
			fmt.Printf("⚠️ Warning: Failed to read content of agent file %s: %v\n", filename, err)
			continue
		}

		agentNames = append(agentNames, agentName)
		agentContentMap[agentName] = string(contentBytes)
	}
	return agentContentMap, agentNames, nil
}

func importAgentFromCore(config *AgentConfig) error {
	agentContentMap, agentNames, err := getAgentCoreSubAgents()
	if err != nil {
		return err
	}

	if len(agentNames) == 0 {
		return fmt.Errorf("no agent files (*.md) found in the core's subagent directory")
	}

	var selectedAgentName string
	prompt := &survey.Select{
		Message: "Select an Agent to import:",
		Options: agentNames,
	}

	err = survey.AskOne(prompt, &selectedAgentName, survey.WithIcons(func(icons *survey.IconSet) {
		icons.SelectFocus.Text = "»"
	}))

	if err != nil {
		return fmt.Errorf("agent selection failed or was interrupted: %w", err)
	}

	promptContent, ok := agentContentMap[selectedAgentName]
	if !ok {
		return fmt.Errorf("internal error: could not find prompt for selected agent: %s", selectedAgentName)
	}

	// Set the selected agent name in the config
	config.AgentName = selectedAgentName

	config.SysPromptPath = filepath.Join(util.GetHomeHgctlDir(), "agents", selectedAgentName)
	if err := writeAgentPromptFile(config.SysPromptPath, selectedAgentName, promptContent); err != nil {
		fmt.Println("❌ failed to write prompt to target file: ", config.SysPromptPath)
		return err
	}

	if err := queryAgentModel(config); err != nil {
		return fmt.Errorf("failed to get agent's model: %s", err)
	}

	if err := queryAgentMCP(config); err != nil {
		return fmt.Errorf("failed to get agent's mcp servers: %s", err)
	}

	if err := queryDeploySettings(config); err != nil {
		return fmt.Errorf("failed to get agent's mcp servers: %s", err)
	}

	fmt.Println("  How the agent responds to user input")
	promptStreaming := &survey.Confirm{
		Message: "Enable streaming responses?",
		Default: true,
	}
	if err := survey.AskOne(promptStreaming, &config.EnableStreaming); err != nil {
		return err
	}

	return nil
}

func queryAgentSysPrompt(config *AgentConfig) error {
	purple.Println("📝 System Prompt")
	fmt.Println("  This defines the agent's personality and behavior")

	options := []string{
		"input directly",
		"use existing markdown file",
		"use LLM to generate",
	}

	var response string
	prompt := &survey.Select{
		Message: "How would you like to set the agent's SysPrompt",
		Options: options,
	}
	if err := survey.AskOne(prompt, &response); err != nil {
		fmt.Println(err)
		return err
	}

	var finalPromptStr string
	switch response {
	case options[0]:
		var prompt string
		sysPromptDefault := fmt.Sprintf("You're a helpful assistant named %s.", config.AgentName)
		promptSysPrompt := &survey.Input{
			Message: "What is the system prompt for this agent?",
			Default: sysPromptDefault,
		}
		if err := survey.AskOne(promptSysPrompt, &prompt); err != nil {
			return err
		}

		finalPromptStr = prompt

	case options[1]:
		var target string
		promptSysPrompt := &survey.Input{
			Message: "Enter the target prompt file path:",
		}
		if err := survey.AskOne(promptSysPrompt, &target); err != nil {
			return err
		}
		content, err := os.ReadFile(target)

		if err != nil {
			fmt.Printf("❌ Failed to read the target file (%s): %v\n", target, err)
			return fmt.Errorf("failed to read source file: %w", err)
		}

		finalPromptStr = string(content)

	case options[2]:
		var desc string
		descPrompt := &survey.Input{
			Message: "Describe what this agent should do (be comprehensive for best results)",
			Default: "Help me write unit tests for my code...",
		}
		if err := survey.AskOne(descPrompt, &desc); err != nil {
			return err
		}

		fmt.Println("generating...(this may take a few minutes, depends on your model)")
		prompt, err := generateAgentPromptByCore(desc)
		fmt.Printf("Generate Prompt for agent %s:\n", config.AgentName)
		fmt.Println(prompt)

		if err != nil {
			fmt.Printf("failed to generate prompt use agent core: %s\n", err)
			return err
		}

		finalPromptStr = prompt
	}

	config.SysPromptPath = filepath.Join(util.GetHomeHgctlDir(), "agents", config.AgentName)
	if err := writeAgentPromptFile(config.SysPromptPath, config.AgentName, finalPromptStr); err != nil {
		fmt.Println("failed to write prompt to target file: ", config.SysPromptPath)
		return err
	}
	return nil
}

func queryAgentTools(config *AgentConfig) error {
	fmt.Println()
	purple.Println("🔧 Available Tools")
	fmt.Println("  Select the tools this agent can use")
	for _, tool := range ASAvailiableTools {
		yellow.Printf("   • %s\n", tool)
	}
	fmt.Println()

	promptTools := &survey.MultiSelect{
		Message: "Which tools to enable? (Space to select, Enter to confirm)",
		Options: ASAvailiableTools,
	}
	if err := survey.AskOne(promptTools, &config.AvailableTools); err != nil {
		return err

	}
	return nil
}

func queryAgentModel(config *AgentConfig) error {
	switch config.Type {
	case AgentRun:
		return queryAgentRunModel(config)
	case Local:
		return queryLocalModel(config)
	default:
		return fmt.Errorf("unsupported deploy type")
	}
}

func queryAgentRunModel(config *AgentConfig) error {
	config.ChatModel = viper.GetString(AGENTRUN_MODEL_NAME)
	fmt.Println()
	purple.Println("🤖 AI Model")
	fmt.Println("  Enter the model name that you've already created on your agentRun dashboard")
	message := "Which model to use?"
	if config.ChatModel != "" {
		message = fmt.Sprintf("Detected from configuration: %s. (Enter to continue)", config.ChatModel)
	}
	promptModelName := &survey.Input{
		Message: message,
		Default: config.ChatModel,
	}
	if err := survey.AskOne(promptModelName, &config.ChatModel); err != nil {
		return err
	}
	return nil
}

func queryLocalModel(config *AgentConfig) error {
	type providerSpec struct {
		InternalName string
		DefaultModel string
		DefaultKey   string
	}

	providerMap := map[string]providerSpec{
		"DashScope": {InternalName: "DashScopeChat", DefaultModel: "qwen-plus", DefaultKey: "DASHSCOPE_API_KEY"},
		"OpenAI":    {InternalName: "OpenAIChat", DefaultModel: "gpt-4o", DefaultKey: "OPENAI_API_KEY"},
		"Anthropic": {InternalName: "AnthropicChat", DefaultModel: "claude-3-5-sonnet-latest", DefaultKey: "ANTHROPIC_API_KEY"},
		"Ollama":    {InternalName: "OllamaChat", DefaultModel: "llama3", DefaultKey: "OLLAMA_API_KEY"},
		"Gemini":    {InternalName: "GeminiChat", DefaultModel: "gemini-1.5-pro", DefaultKey: "GEMINI_API_KEY"},
		"Trinity":   {InternalName: "TrinityChat", DefaultModel: "trinity-model", DefaultKey: "TRINITY_API_KEY"},
	}

	options := []string{"DashScope", "OpenAI", "Anthropic", "Ollama", "Gemini", "Trinity"}

	defaultProvider := options[0]

	if envProvider := viper.GetString(AGENT_MODEL_PROVIDER); envProvider != "" {
		defaultProvider = envProvider
	}

	purple.Println("🏢 AI Provider")
	var selectedDisplayName string
	promptProvider := &survey.Select{
		Message: fmt.Sprintf("Choose the AI provider (%s):", defaultProvider),
		Options: options,
		Default: defaultProvider,
	}
	if err := survey.AskOne(promptProvider, &selectedDisplayName); err != nil {
		return err
	}

	spec := providerMap[selectedDisplayName]
	config.Provider = spec.InternalName

	purple.Println("🤖 AI Model")
	defaultModel := spec.DefaultModel
	if envModel := viper.GetString(AGENT_CHAT_MODEL); envModel != "" {
		defaultModel = envModel
	}

	promptModelName := &survey.Input{
		Message: fmt.Sprintf("Which model to use? (%s)", defaultModel),
		Default: defaultModel,
	}
	if err := survey.AskOne(promptModelName, &config.ChatModel); err != nil {
		return err
	}

	purple.Println("🔑 API Key Configuration")
	promptAPIKey := &survey.Input{
		Message: "Environment variable name for API key:",
		Default: spec.DefaultKey,
	}
	if err := survey.AskOne(promptAPIKey, &config.APIKeyEnvVar); err != nil {
		return err
	}

	return nil
}

func queryAgentMCP(config *AgentConfig) error {
	purple.Println("🔗 MCP Server Configuration")
	cyan.Println("  Configure multiple MCP servers if you want to use external tools")
	config.MCPServers = []MCPServerConfig{}

	// Show Himarket's existing mcp servers
	existServers, names, err := getHimarketMCPServer()
	if err == nil && len(existServers) != 0 {
		yellow.Println("🔗 Get existing MCP Servers from Himarket: ")
		chosedNames := []string{}
		hgServerPrompt := survey.MultiSelect{
			Message: fmt.Sprintf("Choose MCP Server from Current Himarket(%s)", viper.GetString(HIMARKET_DEVELOPER_URL)),
			Options: names,
		}
		if err := survey.AskOne(&hgServerPrompt, &chosedNames); err != nil {
			return err
		}

		for _, name := range chosedNames {
			config.MCPServers = append(config.MCPServers, MCPServerConfig{
				Name:      name,
				URL:       existServers[name],
				Transport: "streamable_http",
			})
		}
	}

	// Show Higress's existing mcp servers
	existServers, names, err = getHigressMCPServers()
	if err == nil && len(existServers) != 0 {
		yellow.Println("🔗 Get existing MCP Servers from Higress: ")
		chosedNames := []string{}
		hgServerPrompt := survey.MultiSelect{
			Message: fmt.Sprintf("Choose MCP Server from Current Higress(%s)", viper.GetString(HIGRESS_CONSOLE_URL)),
			Options: names,
		}
		if err := survey.AskOne(&hgServerPrompt, &chosedNames); err != nil {
			return err
		}

		for _, name := range chosedNames {
			config.MCPServers = append(config.MCPServers, MCPServerConfig{
				Name:      name,
				URL:       existServers[name],
				Transport: "streamable_http",
			})
		}
	}

	fmt.Println()
	purple.Println("Add MCP Servers manually...")

	for {
		var mcpserver MCPServerConfig

		promptMCPServer := &survey.Input{
			Message: "MCP Server URL (or press Enter to finish):",
			Default: "",
		}
		if err := survey.AskOne(promptMCPServer, &mcpserver.URL); err != nil || mcpserver.URL == "" {
			break
		}

		promptMCPTransport := &survey.Input{
			Message: "transport:",
			Default: "streamable_http",
		}
		if err := survey.AskOne(promptMCPTransport, &mcpserver.Transport); err != nil || mcpserver.Transport == "" {
			break
		}

		mcpserver.URL = strings.TrimSpace(mcpserver.URL)

		mcpNameDefault := fmt.Sprintf("%s-mcp-%d", config.AgentName, len(config.MCPServers)+1)
		promptMCPName := &survey.Input{
			Message: "MCP Client Name:",
			Default: mcpNameDefault,
		}
		if err := survey.AskOne(promptMCPName, &mcpserver.Name); err != nil {
			return err
		}

		yellow.Printf("📋 HTTP Headers for '%s' (optional)\n", mcpserver.Name)
		cyan.Println("  Add custom headers for MCP server requests")
		yellow.Println("  Press Enter to finish adding headers")

		mcpserver.Headers = make(map[string]string)

		for {
			var headerKey, headerValue string

			promptKey := &survey.Input{
				Message: "Header name (or press Enter to finish):",
				Default: "",
			}
			if err := survey.AskOne(promptKey, &headerKey); err != nil || headerKey == "" {
				break
			}

			promptValue := &survey.Input{
				Message: fmt.Sprintf("Value for '%s':", headerKey),
				Default: "",
			}
			if err := survey.AskOne(promptValue, &headerValue); err != nil {
				return err
			}

			if headerValue != "" {
				mcpserver.Headers[headerKey] = headerValue
			}
		}

		config.MCPServers = append(config.MCPServers, mcpserver)

		green.Printf("✅ Added MCP server: %s\n", mcpserver.Name)
		fmt.Println()
	}

	return nil
}

func queryDeploySettings(config *AgentConfig) error {
	switch config.Type {
	case AgentRun:
		return queryAgentRunDeploySettings(config)
	case Local:
		return queryLocalDeploySettings(config)
	default:
		return fmt.Errorf("unsupported deploy type")
	}
}

func queryAgentRunDeploySettings(config *AgentConfig) error {
	purple.Println("☁️  AgentRun Deployment Settings")
	fmt.Println("   Configure the settings for deploying to AgentRun/FC")

	promptResourceName := &survey.Input{
		Message: "Resource Name:",
		Default: "my-agent-resource",
		Help:    "A unique name for the deployed resource.",
	}
	if err := survey.AskOne(promptResourceName, &config.ServerlessCfg.ResourceName); err != nil {
		return err
	}

	promptRegion := &survey.Select{
		Message: "Region:",
		Options: []string{"cn-hangzhou", "cn-shanghai", "cn-beijing", "ap-southeast-1"},
		Default: viper.GetString(AGENTRUN_REGION),
		Help:    "The region where the agent will be deployed.",
	}
	if err := survey.AskOne(promptRegion, &config.ServerlessCfg.Region); err != nil {
		return err
	}

	promptAgentDesc := &survey.Input{
		Message: "Agent Description:",
		Default: "My Agent Runtime created by dev",
		Help:    "A brief description of the agent.",
	}
	if err := survey.AskOne(promptAgentDesc, &config.ServerlessCfg.AgentDesc); err != nil {
		return err
	}

	promptPort := &survey.Input{
		Message: "Service Port:",
		Default: "9000",
		Help:    "The port the agent service listens on inside the container/runtime.",
	}
	var portStr string
	if err := survey.AskOne(promptPort, &portStr); err != nil {
		return err
	}

	if portNum, err := strconv.ParseUint(portStr, 10, 32); err == nil {
		config.ServerlessCfg.Port = uint(portNum)
	}

	promptDiskSize := &survey.Input{
		Message: "Disk Size (MB) (Optional, default 500 MB):",
		Default: "512",
		Help:    "Disk size allocated to the agent runtime (MB).",
	}
	var diskSizeStr string
	if err := survey.AskOne(promptDiskSize, &diskSizeStr); err != nil {
		return err
	}
	if diskSizeNum, err := strconv.ParseUint(diskSizeStr, 10, 32); err == nil {
		config.ServerlessCfg.DiskSize = uint(diskSizeNum)
	}

	promptTimeout := &survey.Input{
		Message: "Timeout (seconds) (Optional, default 600s):",
		Default: "600",
		Help:    "The maximum request processing time (seconds).",
	}
	var timeoutStr string
	if err := survey.AskOne(promptTimeout, &timeoutStr); err != nil {
		return err
	}
	if timeoutNum, err := strconv.ParseUint(timeoutStr, 10, 32); err == nil {
		config.ServerlessCfg.Timeout = uint(timeoutNum)
	}

	config.ServerlessCfg.AgentName = config.AgentName

	return nil
}

func queryLocalDeploySettings(config *AgentConfig) error {
	purple.Println("🌐 Deployment Settings")
	fmt.Println("  Network configuration for the agent")
	promptPort := &survey.Input{
		Message: "Deployment port:",
		Default: "8090",
	}
	var portStr string

	if err := survey.AskOne(promptPort, &portStr); err != nil {
		return err
	}

	if portNum, err := strconv.Atoi(portStr); err == nil {
		config.DeploymentPort = portNum
	} else {
		config.DeploymentPort = 8090 // 默认值
	}

	promptHost := &survey.Input{
		Message: "Host binding:",
		Default: "0.0.0.0",
	}
	if err := survey.AskOne(promptHost, &config.HostBinding); err != nil {
		return err
	}
	return nil
}

func createAgentStepByStep(config *AgentConfig) error {
	name := ""
	namePrompt := &survey.Input{
		Message: "What is the agent's name?",
		Default: "",
	}
	if err := survey.AskOne(namePrompt, &name); err != nil {
		return err
	}

	config.AgentName = name
	config.AppName = name

	cyan.Printf("🤖 Let's configure your agent '%s'\n", name)

	fmt.Println()
	purple.Println("📋 App Description")
	fmt.Println("  A brief description of what this agent does")
	promptAppDescription := &survey.Input{
		Message: "What is the app description?",
		Default: "A helpful assistant and useful agent",
	}
	if err := survey.AskOne(promptAppDescription, &config.AppDescription); err != nil {
		return err
	}

	if err := queryAgentSysPrompt(config); err != nil {
		return fmt.Errorf("failed to get agent's sysPrompt: %s", err)
	}

	if err := queryAgentModel(config); err != nil {
		return fmt.Errorf("failed to get agent's model: %s", err)
	}

	if err := queryAgentTools(config); err != nil {
		return fmt.Errorf("failed to get agent's tools: %s", err)
	}

	if err := queryAgentMCP(config); err != nil {
		return fmt.Errorf("failed to get agent's mcp servers: %s", err)
	}

	if err := queryDeploySettings(config); err != nil {
		return fmt.Errorf("failed to get agent's mcp servers: %s", err)
	}

	fmt.Println("  How the agent responds to user input")
	promptStreaming := &survey.Confirm{
		Message: "Enable streaming responses?",
		Default: true,
	}
	if err := survey.AskOne(promptStreaming, &config.EnableStreaming); err != nil {
		return err
	}

	showConfigSummary(config)

	return nil
}

// Write given prompt to ~/.hgctl/agents/<name>/<prompt.md>
func writeAgentPromptFile(dir, name, prompt string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create agent directory %s: %w", dir, err)
	}
	filePath := filepath.Join(dir, "prompt.md")

	if err := os.WriteFile(filePath, []byte(prompt), 0644); err != nil {
		return fmt.Errorf("failed to write prompt file %s: %w", filePath, err)
	}
	return nil
}

func getHimarketMCPServer() (map[string]string, []string, error) {
	conURL := viper.GetString(HIMARKET_DEVELOPER_URL)
	conUser := viper.GetString(HIMARKET_DEVELOPER_USER)
	conPwd := viper.GetString(HIMARKET_DEVELOPER_PASSWORD)

	if conURL == "" || conUser == "" || conPwd == "" {
		return nil, nil, fmt.Errorf("empty env, can not get Himarket's MCP Servers")
	}

	client := services.NewHimarketClient(
		conURL,
		conUser,
		conPwd,
	)
	resultMap, err := client.GetDevMCPServerProduct()
	if err != nil {
		return nil, nil, err
	}

	keys := make([]string, 0, len(resultMap))
	for k := range resultMap {
		keys = append(keys, k)
	}

	return resultMap, keys, nil
}

func getHigressMCPServers() (map[string]string, []string, error) {
	conURL := viper.GetString(HIGRESS_CONSOLE_URL)
	conUser := viper.GetString(HIGRESS_CONSOLE_USER)
	conPwd := viper.GetString(HIGRESS_CONSOLE_PASSWORD)
	gwURL := viper.GetString(HIGRESS_GATEWAY_URL)

	if conURL == "" || conUser == "" || conPwd == "" || gwURL == "" {
		return nil, nil, fmt.Errorf("empty env, can not get Higress's MCP Servers")
	}

	client := services.NewHigressClient(
		conURL,
		conUser,
		conPwd,
	)
	resultMap, err := services.GetExistingMCPServers(client)
	if err != nil {
		return nil, nil, err
	}
	for k := range resultMap {
		resultMap[k] = fmt.Sprintf("%s/mcp-servers/%s", gwURL, k)
	}

	keys := make([]string, 0, len(resultMap))
	for k := range resultMap {
		keys = append(keys, k)
	}

	return resultMap, keys, nil
}

// Print agent config summary to user
func showConfigSummary(config *AgentConfig) {
	summaryColor := color.New(color.FgBlue, color.Bold)
	summaryColor.Println("📊 Agent Configuration Summary:")
	fmt.Printf("  📝 Name: %s\n", config.AgentName)
	fmt.Printf("  🏢 Provider: %s\n", config.Provider)
	fmt.Printf("  🤖 Model: %s\n", config.ChatModel)
	fmt.Printf("  🔧 Tools: %d selected\n", len(config.AvailableTools))
	fmt.Printf("  🌐 Port: %d\n", config.DeploymentPort)
	fmt.Printf("  📍 Host: %s\n", config.HostBinding)
	fmt.Printf("  ✨ Streaming: %t\n", config.EnableStreaming)

	if len(config.MCPServers) > 0 {
		fmt.Printf("  🔗 MCP Servers: %d\n", len(config.MCPServers))
		for i, mcp := range config.MCPServers {
			fmt.Printf("    %d. %s - %s\n", i+1, mcp.Name, mcp.URL)
			if len(mcp.Headers) > 0 {
				fmt.Printf("       Headers: %d\n", len(mcp.Headers))
			}
		}
	}
	fmt.Println()
}
